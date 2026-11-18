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
		{"Windows with Intel or AMD graphics", probe{goos: "windows", goarch: "amd64", otherGPU: true}, "cpu"},
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
