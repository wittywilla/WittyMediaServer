// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package separator

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPickAccel(t *testing.T) {
	for _, c := range []struct {
		name string
		p    probe
		want string
	}{
		{"Windows with NVIDIA", probe{goos: "windows", goarch: "amd64", nvidia: true}, "cuda"},
		{"Linux with NVIDIA", probe{goos: "linux", goarch: "amd64", nvidia: true}, "cuda"},
		{"Linux with ROCm", probe{goos: "linux", goarch: "amd64", rocm: true}, "rocm"},
		{"NVIDIA wins over ROCm", probe{goos: "linux", goarch: "amd64", nvidia: true, rocm: true}, "cuda"},
		{"Apple silicon", probe{goos: "darwin", goarch: "arm64"}, "mps"},
		{"Intel Mac", probe{goos: "darwin", goarch: "amd64"}, "cpu"},
		{"Windows with AMD graphics", probe{goos: "windows", goarch: "amd64", otherGPU: "AMD"}, "dml"},
		{"Windows with Intel graphics", probe{goos: "windows", goarch: "amd64", otherGPU: "Intel"}, "dml"},
		{"NVIDIA wins over DirectML", probe{goos: "windows", goarch: "amd64", nvidia: true, otherGPU: "Intel"}, "cuda"},
		{"Windows on ARM (no DirectML build)", probe{goos: "windows", goarch: "arm64", otherGPU: "Intel"}, "cpu"},
		{"Linux with an Intel card", probe{goos: "linux", goarch: "amd64", otherGPU: "Intel"}, "cpu"},
		{"Linux on ARM with NVIDIA (no CUDA build here)", probe{goos: "linux", goarch: "arm64", nvidia: true}, "cpu"},
		{"no card", probe{goos: "linux", goarch: "amd64"}, "cpu"},
	} {
		a := pickAccel(c.p)
		if a.ID != c.want {
			t.Errorf("%s: %s, want %s", c.name, a.ID, c.want)
		}
		if a.ID == "cpu" && a.Why == "" {
			t.Errorf("%s: the processor needs a reason", c.name)
		}
		if !strings.HasPrefix(a.Index, torchIndex) || a.Device == "" || a.DownloadMB <= 0 || a.Name == "" {
			t.Errorf("%s: %+v", c.name, a)
		}
	}
	if AccelByID("dml").Device != "cpu" || !strings.Contains(pickAccel(probe{goos: "windows", goarch: "amd64", otherGPU: "AMD"}).Name, "AMD") {
		t.Error("DirectML: PyTorch on the processor, and the card named")
	}
	if AccelByID("rocm").Device != "cuda" || AccelByID("mps").Device != "mps" || AccelByID("nonsense").ID != "cpu" || AccelByID("").ID != "cpu" {
		t.Error("AccelByID")
	}
}

func TestOnProcessorFallback(t *testing.T) {
	gpu := []string{"py", "-m", "demucs", "-n", "htdemucs", "-d", "cuda", "-o", "{output}", "{input}"}
	cpu := OnProcessor(gpu)
	if cpu == nil || cpu[6] != "cpu" || gpu[6] != "cuda" {
		t.Fatalf("%v (and the original must be left alone: %v)", cpu, gpu)
	}
	if OnProcessor(cpu) != nil {
		t.Error("already on the processor: no second try")
	}
	if OnProcessor([]string{"demucs", "-o", "{output}", "{input}"}) != nil {
		t.Error("no -d: nothing to change")
	}
	dml := []string{"py", "demucs_onnx.py", "separate", "--onnx", "m.onnx", "--provider", "dml", "-o", "{output}", "{input}"}
	back := OnProcessor(dml)
	if strings.Join(back, " ") != "py -m demucs -n htdemucs -d cpu -o {output} {input}" {
		t.Errorf("a song failing through DirectML is done by Demucs on the processor: %v", back)
	}
}

func TestInstalledChoices(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "separator")
	py := venvPython(dir)
	os.MkdirAll(filepath.Dir(py), 0o755)
	os.WriteFile(py, []byte("x"), 0o755)
	write := func(js string) { os.WriteFile(filepath.Join(dir, infoFile), []byte(js), 0o644) }
	ids := func(in *Info) (out []string) {
		for _, a := range Choices(in) {
			out = append(out, a.ID)
		}
		return
	}
	write(`{"version":"4.0.1","model":"htdemucs","accel":"cuda","directml":true}`)
	if got := ids(ReadInfo(dir)); !slices.Equal(got, []string{"cuda", "dml", "cpu"}) {
		t.Errorf("choices %v", got)
	}
	if f := InstalledWith(dir, "auto"); f.Accel.ID != "cuda" {
		t.Errorf("auto: %v", f.Accel.ID)
	}
	if f := InstalledWith(dir, "cpu"); f.Accel.ID != "cpu" || f.Words[slices.Index(f.Words, "-d")+1] != "cpu" {
		t.Errorf("cpu: %v", f.Words)
	}
	// DirectML chosen, but the converted model is missing: Demucs on the processor.
	if f := InstalledWith(dir, "dml"); f.Accel.ID != "cpu" || slices.Contains(f.Words, "--provider") {
		t.Errorf("dml without its model: %v", f.Words)
	}
	os.MkdirAll(filepath.Join(dir, "models"), 0o755)
	os.WriteFile(filepath.Join(dir, "models", onnxModelFile), []byte("x"), 0o644)
	if f := InstalledWith(dir, "dml"); f.Accel.ID != "dml" || !slices.Contains(f.Words, "--provider") || !slices.Contains(f.Words, "{input}") {
		t.Errorf("dml: %v", f.Words)
	}
	// A choice the install can't run is the install's own.
	write(`{"version":"4.0.1","model":"htdemucs","accel":"cpu"}`)
	if got := ids(ReadInfo(dir)); !slices.Equal(got, []string{"cpu"}) {
		t.Errorf("choices %v", got)
	}
	if f := InstalledWith(dir, "dml"); f.Accel.ID != "cpu" {
		t.Errorf("dml not installed: %v", f.Accel.ID)
	}
}

func TestInstalledRunsOnItsCard(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "separator")
	py := venvPython(dir)
	os.MkdirAll(filepath.Dir(py), 0o755)
	os.WriteFile(py, []byte("x"), 0o755)
	for id, dev := range map[string]string{"cuda": "cuda", "mps": "mps", "cpu": "cpu", "": "cpu"} {
		os.WriteFile(filepath.Join(dir, infoFile), []byte(`{"version":"4.0.1","model":"htdemucs","accel":"`+id+`"}`), 0o644)
		f := InstalledAt(dir)
		i := slices.Index(f.Words, "-d")
		if i < 0 || f.Words[i+1] != dev {
			t.Errorf("%q: %v", id, f.Words)
		}
	}
}
