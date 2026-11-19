// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package separator

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSupportedOn(t *testing.T) {
	for _, c := range []struct {
		goos, arch string
		musl, ok   bool
	}{
		{"windows", "amd64", false, true},
		{"linux", "amd64", false, true},
		{"linux", "arm64", false, true},
		{"darwin", "arm64", false, true},
		{"darwin", "amd64", false, true},
		{"linux", "amd64", true, false}, // Alpine
		{"freebsd", "amd64", false, false},
		{"windows", "arm64", false, false},
		{"linux", "386", false, false},
	} {
		ok, why := supportedOn(c.goos, c.arch, c.musl)
		if ok != c.ok || (!ok && why == "") {
			t.Errorf("%s/%s musl=%v: %v %q", c.goos, c.arch, c.musl, ok, why)
		}
	}
	for k, a := range uvAssets {
		if len(a.sha256) != 64 || !strings.Contains(a.name, "uv-") {
			t.Errorf("%s: %+v", k, a)
		}
	}
}

func TestPackagesArePinned(t *testing.T) {
	for _, p := range [][2]string{{"windows", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}} {
		pk := Packages(p[0], p[1])
		var demucs, torch bool
		for _, s := range pk {
			demucs = demucs || strings.HasPrefix(s, "demucs==")
			torch = torch || strings.HasPrefix(s, "torch==")
		}
		if !demucs || !torch {
			t.Errorf("%v: %v", p, pk)
		}
	}
	if strings.Join(Packages("darwin", "amd64"), " ") == strings.Join(Packages("darwin", "arm64"), " ") {
		t.Error("Intel Macs need the older PyTorch")
	}
}

func TestInstalledOnlyWhenFinished(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "separator")
	py := venvPython(dir)
	if err := os.MkdirAll(filepath.Dir(py), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(py, []byte("x"), 0o755)
	if InstalledAt(dir) != nil {
		t.Fatal("no installed.json: an unfinished install isn't one")
	}
	os.WriteFile(filepath.Join(dir, infoFile), []byte(`{"version":"4.0.1","model":"htdemucs","python":"3.11","uv":"0.13.5"}`), 0o644)
	f := InstalledAt(dir)
	if f == nil || !f.Installed || f.Version != "4.0.1" || f.Words[0] != py || f.Words[1] != "-m" || f.Words[2] != "demucs" {
		t.Fatalf("%+v", f)
	}
	var torchHome bool
	for _, e := range f.Env {
		torchHome = torchHome || e == "TORCH_HOME="+filepath.Join(dir, "models")
	}
	if !torchHome {
		t.Errorf("env %v", f.Env)
	}
	if err := Remove(filepath.Dir(dir)); err == nil {
		t.Error("Remove must only take a separator folder")
	}
	if err := Remove(dir); err != nil || InstalledAt(dir) != nil {
		t.Errorf("remove: %v", err)
	}
}

func TestLicensesFromPackages(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "separator")
	site := filepath.Join(dir, "venv", "Lib", "site-packages")
	if runtime.GOOS != "windows" {
		site = filepath.Join(dir, "venv", "lib", "python3.11", "site-packages")
	}
	di := filepath.Join(site, "demucs-4.0.1.dist-info")
	os.MkdirAll(filepath.Join(di, "licenses"), 0o755)
	os.WriteFile(filepath.Join(di, "METADATA"), []byte("Metadata-Version: 2.1\nName: demucs\nVersion: 4.0.1\nHome-page: https://github.com/facebookresearch/demucs\nClassifier: License :: OSI Approved :: MIT License\n\nLong description. License: not this\n"), 0o644)
	os.WriteFile(filepath.Join(di, "licenses", "LICENSE"), []byte("MIT License\nCopyright (c) Meta Platforms"), 0o644)
	ti := filepath.Join(site, "torch-2.5.1.dist-info")
	os.MkdirAll(ti, 0o755)
	os.WriteFile(filepath.Join(ti, "METADATA"), []byte("Name: torch\nVersion: 2.5.1\nLicense: BSD-3-Clause\n\n"), 0o644)
	os.WriteFile(filepath.Join(ti, "LICENSE"), []byte("BSD"), 0o644)
	os.MkdirAll(filepath.Join(dir, "python", "cpython-3.11"), 0o755)
	os.WriteFile(filepath.Join(dir, "python", "cpython-3.11", "LICENSE.txt"), []byte("PYTHON SOFTWARE FOUNDATION LICENSE VERSION 2"), 0o644)

	if Licenses(dir) != nil {
		t.Fatal("nothing is listed until the install is finished")
	}
	os.WriteFile(filepath.Join(dir, infoFile), []byte(`{"version":"4.0.1","python":"3.11","uv":"0.13.5"}`), 0o644)
	ls := Licenses(dir)
	names := []string{}
	for _, l := range ls {
		names = append(names, l.Name+" "+l.Version+" "+l.License)
	}
	want := "uv 0.13.5 MIT OR Apache-2.0|Python 3.11 PSF-2.0|demucs 4.0.1 MIT License|torch 2.5.1 BSD-3-Clause"
	if got := strings.Join(names, "|"); got != want {
		t.Fatalf("got %s", got)
	}
	if !strings.Contains(ls[2].Text, "Meta Platforms") || ls[2].URL == "" || ls[3].Text != "BSD" {
		t.Errorf("%+v", ls[2:])
	}
}

func TestWithPath(t *testing.T) {
	env := withPath([]string{"A=1", "PATH=/usr/bin"}, "/ff")
	if env[1] != "PATH=/ff"+string(os.PathListSeparator)+"/usr/bin" {
		t.Errorf("%v", env)
	}
	if env := withPath([]string{"A=1"}, "/ff"); env[1] != "PATH=/ff" {
		t.Errorf("%v", env)
	}
}
