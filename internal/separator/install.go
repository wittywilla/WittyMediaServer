// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package separator

// Installing Demucs for the owner, when an administrator presses the button for it. Nothing
// here runs by itself, and nothing goes outside the server's own folder for it:
//
//	<data>/separator/
//	    uv/            the installer (uv, by Astral), its download checked against a SHA-256 kept here
//	    python/        Python 3.11, fetched and checked by uv
//	    venv/          Demucs, PyTorch (for the graphics card when there is one, accel.go) and what they need, pinned
//	    models/        Demucs's model, fetched once from its makers (TORCH_HOME), and on Windows
//	                   htdemucs.onnx, the same model converted here for DirectML
//	    demucs_onnx.py on Windows: runs the converted model through ONNX Runtime (DirectML)
//	    installed.json written last: without it the folder is an unfinished install, and is removed
//
// What's downloaded, and from where: uv from GitHub (astral-sh/uv), Python from GitHub
// (astral-sh/python-build-standalone, chosen and checked by uv), the packages from PyPI and
// download.pytorch.org (the CPU builds, so no graphics card drivers come with them), and the
// model from dl.fbaipublicfiles.com. About 1 GB in all, 3 to 4 GB with a graphics card's build.
// On 64-bit Windows, ONNX Runtime's DirectML build and onnx come too (about 40 MB, from PyPI,
// each checked against a SHA-256 kept here), and the model is converted to ONNX on this machine:
// nothing converted is downloaded from anyone, us included. Removing it deletes the folder.

import (
	_ "embed"

	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// UVVersion is the installer used; its archives and their SHA-256 are below. Changing it means
// changing every hash, from the .sha256 files published beside that release's archives.
const UVVersion = "0.13.5"

// PythonVersion is what Demucs and PyTorch run on.
const PythonVersion = "3.11"

type uvAsset struct{ name, sha256 string }

var uvAssets = map[string]uvAsset{
	"windows/amd64": {"uv-x86_64-pc-windows-msvc.zip", "a38b8d1902b7cfbc76b423ba1bb456f623f42064cdc72628b6075b89afd1019a"},
	"linux/amd64":   {"uv-x86_64-unknown-linux-gnu.tar.gz", "f68d291c75783122dad58ab77def0e13f3233f358731db5d6d7e8715603b928f"},
	"linux/arm64":   {"uv-aarch64-unknown-linux-gnu.tar.gz", "4bb69cf95ed13f23713bb7bc09f241896ea0c75b7c557e615a20604b82f9c403"},
	"darwin/amd64":  {"uv-x86_64-apple-darwin.tar.gz", "b0f52b0e3945b4107d89024ea38e625683011fbfc05a0d98f9fa2d6dcee28c66"},
	"darwin/arm64":  {"uv-aarch64-apple-darwin.tar.gz", "743f85035a1996ecb350659e01cd9faea9709829be09b1b4b03c73eaf3c9cd73"},
}

// Packages are what's installed, pinned. PyTorch stopped making builds for Intel Macs after
// 2.2, which also needs NumPy before 2.
func Packages(goos, goarch string) []string {
	if goos == "darwin" && goarch == "amd64" {
		return []string{"demucs==4.0.1", "torch==2.2.2", "torchaudio==2.2.2", "numpy<2", "soundfile==0.12.1"}
	}
	return []string{"demucs==4.0.1", "torch==2.5.1", "torchaudio==2.5.1", "soundfile==0.12.1"}
}

// Model is the Demucs model used, fetched at install so the first song doesn't wait for it.
const Model = "htdemucs"

// ModelURL is where Demucs fetches the model's weights from: Meta's own server. Demucs, its
// weights included, is released under the MIT license.
const ModelURL = "https://dl.fbaipublicfiles.com/demucs/hybrid_transformer/955717e8-8726e21a.th"

// onnxScript converts the model to ONNX and runs it with ONNX Runtime, for DirectML.
//
//go:embed demucs_onnx.py
var onnxScript []byte

const (
	onnxScriptFile = "demucs_onnx.py"
	onnxModelFile  = "htdemucs.onnx" // in models/
)

// wheel is a Python package fetched by the server itself, refused unless its SHA-256 matches.
type wheel struct{ name, url, sha256 string }

// dmlWheels are the DirectML parts, for Python 3.11 on 64-bit Windows: ONNX Runtime's DirectML
// build, and onnx, which PyTorch's converter needs. Changing one means changing its hash, from
// PyPI's page for that file.
var dmlWheels = []wheel{
	{"onnxruntime_directml-1.24.4-cp311-cp311-win_amd64.whl",
		"https://files.pythonhosted.org/packages/be/90/99566dc6398028e7691a5b12720fd85f757a0901818b84599d28abb3f085/onnxruntime_directml-1.24.4-cp311-cp311-win_amd64.whl",
		"96642a787e5a6f33bf043521c0f06eb1eb663f6b830e5862a2026d03f9c90543"},
	{"onnx-1.17.0-cp311-cp311-win_amd64.whl",
		"https://files.pythonhosted.org/packages/51/a5/19b0dfcb567b62e7adf1a21b08b23224f0c2d13842aee4d0abc6f07f9cf5/onnx-1.17.0-cp311-cp311-win_amd64.whl",
		"95c03e38671785036bb704c30cd2e150825f6ab4763df3a4f1d249da48525957"},
}

// DirectMLPackages are dmlWheels as Settings lists them.
var DirectMLPackages = []string{"onnxruntime-directml==1.24.4", "onnx==1.17.0"}

// Supported says whether this machine can have Demucs installed for it, and why not.
func Supported() (bool, string) {
	return supportedOn(runtime.GOOS, runtime.GOARCH, muslLinux())
}

func supportedOn(goos, goarch string, musl bool) (bool, string) {
	if _, ok := uvAssets[goos+"/"+goarch]; !ok {
		switch {
		case goos == "freebsd":
			return false, "PyTorch, which Demucs needs, isn't made for FreeBSD. Install a separator by hand, or use the quick version."
		case goos == "windows" && goarch == "arm64":
			return false, "PyTorch isn't made for Windows on ARM for this. Install a separator by hand, or use the quick version."
		}
		return false, fmt.Sprintf("There's no ready-made build for %s on %s. Install a separator by hand, or use the quick version.", goarch, goos)
	}
	if musl {
		return false, "This system's C library (musl, as on Alpine) can't run PyTorch's builds. Install a separator by hand, or use the quick version."
	}
	return true, ""
}

func muslLinux() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	m, _ := filepath.Glob("/lib/ld-musl-*")
	return len(m) > 0
}

// Info is what installed.json holds.
type Info struct {
	Version  string   `json:"version"` // Demucs's
	Model    string   `json:"model"`
	UV       string   `json:"uv"`
	Python   string   `json:"python"`
	Packages []string `json:"packages"`
	Accel    string   `json:"accel"` // what it runs on by itself: cuda, rocm, mps, dml or cpu
	// DirectML: the model was converted to ONNX and passed a test run on the card through
	// DirectML, so it can be chosen in Settings even where another card comes first.
	DirectML  bool      `json:"directml,omitempty"`
	Installed time.Time `json:"installed"`
}

const infoFile = "installed.json"

// ReadInfo is the finished install in dir, or nil.
func ReadInfo(dir string) *Info {
	b, err := os.ReadFile(filepath.Join(dir, infoFile))
	if err != nil {
		return nil
	}
	var in Info
	if json.Unmarshal(b, &in) != nil || in.Version == "" {
		return nil
	}
	return &in
}

func venvPython(dir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(dir, "venv", "Scripts", "python.exe")
	}
	return filepath.Join(dir, "venv", "bin", "python")
}

// Env is what the installed Demucs needs to run: where its model is kept, and no reaching for
// anything outside its folder.
func Env(dir string) []string {
	return []string{"TORCH_HOME=" + filepath.Join(dir, "models"), "PYTHONNOUSERSITE=1", "PYTHONDONTWRITEBYTECODE=1"}
}

// InstalledAt is the Demucs installed in dir, or nil when there's none, or it's unfinished.
func InstalledAt(dir string) *Found { return InstalledWith(dir, "auto") }

// Choices are what the installed Demucs can run on, best first: the card it was installed for,
// DirectML when that passed its test, and the processor. Settings offers these, with "auto".
func Choices(in *Info) []Accel {
	if in == nil {
		return nil
	}
	var out []Accel
	if in.Accel != "cpu" && in.Accel != "dml" && in.Accel != "" {
		out = append(out, AccelByID(in.Accel))
	}
	if in.DirectML {
		out = append(out, dmlAccel)
	}
	return append(out, cpuAccel)
}

// InstalledWith is InstalledAt running on choice: "auto" (or "") for what the install chose, or
// one of Choices' IDs. A choice the install can't run falls back to what it chose.
func InstalledWith(dir, choice string) *Found {
	in := ReadInfo(dir)
	if in == nil {
		return nil
	}
	py := venvPython(dir)
	if fi, err := os.Stat(py); err != nil || fi.IsDir() {
		return nil
	}
	acc := AccelByID(in.Accel)
	for _, c := range Choices(in) {
		if c.ID == choice {
			acc = c
		}
	}
	f := &Found{Name: "Demucs", Version: in.Version, Path: py, Installed: true, Env: Env(dir), Accel: acc,
		Words: []string{py, "-m", "demucs", "-n", in.Model, "-d", acc.Device, "-o", "{output}", "{input}"}}
	if acc.ID == "dml" {
		script, model := filepath.Join(dir, onnxScriptFile), filepath.Join(dir, "models", onnxModelFile)
		if _, err := os.Stat(model); err != nil {
			f.Accel = cpuAccel // the converted model is gone: Demucs itself, on the processor
			return f
		}
		f.Words = []string{py, script, "separate", "--onnx", model, "--provider", "dml", "-o", "{output}", "{input}"}
	}
	return f
}

// Step says where an install is: a sentence for people, and how far along, 0 to 100.
type Step func(phase string, pct int)

// Install puts Demucs in dir, built for acc. A failed or stopped install leaves nothing behind.
// ffmpegDir is put first on the PATH of what it runs (Demucs reads songs with FFmpeg).
func Install(ctx context.Context, dir, ffmpegDir string, acc Accel, step Step, logw io.Writer) (err error) {
	if ok, why := Supported(); !ok {
		return errors.New(why)
	}
	asset := uvAssets[runtime.GOOS+"/"+runtime.GOARCH]
	if ReadInfo(dir) != nil {
		return errors.New("Demucs is already installed")
	}
	_ = os.RemoveAll(dir) // an unfinished one from before
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dir)
		}
	}()
	if step == nil {
		step = func(string, int) {}
	}
	if logw == nil {
		logw = io.Discard
	}

	step("Downloading the installer", 2)
	uvDir := filepath.Join(dir, "uv")
	uv, err := fetchUV(ctx, asset, uvDir)
	if err != nil {
		return fmt.Errorf("the installer couldn't be downloaded: %w", err)
	}
	env := append(os.Environ(),
		"UV_CACHE_DIR="+filepath.Join(dir, "cache"),
		"UV_PYTHON_INSTALL_DIR="+filepath.Join(dir, "python"),
		"UV_PYTHON_PREFERENCE=only-managed",
		"UV_NO_CONFIG=1",
		"UV_NO_PROGRESS=1",
		"UV_LINK_MODE=copy",
		// Python stays in the folder: no python3.11 on the user's PATH, nothing in the registry.
		"UV_PYTHON_INSTALL_BIN=0",
		"UV_PYTHON_INSTALL_REGISTRY=0",
	)
	env = withPath(append(env, Env(dir)...), ffmpegDir)
	run := func(phase string, pct int, prog string, args ...string) error {
		step(phase, pct)
		fmt.Fprintf(logw, "> %s %s\n", filepath.Base(prog), strings.Join(args, " "))
		cmd := exec.CommandContext(ctx, prog, args...)
		cmd.Dir = dir
		cmd.Env = env
		cmd.Stdout, cmd.Stderr = logw, logw
		cmd.WaitDelay = 10 * time.Second
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("%s failed: %w", strings.ToLower(phase[:1])+phase[1:], err)
		}
		return nil
	}
	if err := run("Getting Python", 8, uv, "python", "install", PythonVersion); err != nil {
		return err
	}
	if err := run("Making a place for Demucs", 15, uv, "venv", "--python", PythonVersion, filepath.Join(dir, "venv")); err != nil {
		return err
	}
	py := venvPython(dir)
	pkgs := Packages(runtime.GOOS, runtime.GOARCH)
	if acc.Index == "" {
		acc = cpuAccel
	}
	args := append([]string{"pip", "install", "--python", py, "--index-url", acc.Index,
		"--extra-index-url", "https://pypi.org/simple", "--index-strategy", "unsafe-best-match"}, pkgs...)
	if err := run(fmt.Sprintf("Getting Demucs and PyTorch for %s (about %.1f GB)", acc.Name, float64(acc.DownloadMB)/1000), 20, uv, args...); err != nil {
		return err
	}
	if err := run("Getting Demucs's model", 80, py, "-c", "from demucs.pretrained import get_model; get_model('"+Model+"')"); err != nil {
		return err
	}
	step("Checking it", 85)
	ver, err := exec.CommandContext(ctx, py, "-c", "import demucs;print(demucs.__version__)").Output()
	if err != nil {
		return fmt.Errorf("Demucs was installed but won't start: %w", err)
	}
	// DirectML, for Intel and AMD cards on Windows, and as the next choice where another card
	// fails its check. Not when the administrator chose the processor only. A failure here is
	// said in the log and leaves the rest of the install as it is: Demucs works without it.
	pkgs = append([]string(nil), pkgs...)
	dml := false
	if DirectMLOn(runtime.GOOS, runtime.GOARCH) && acc.ID != "cpu" {
		var got bool
		var derr error
		got, dml, derr = installDirectML(ctx, dir, uv, py, env, step, logw)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if derr != nil {
			fmt.Fprintf(logw, "DirectML won't be used: %v\n", derr)
		}
		if got {
			pkgs = append(pkgs, DirectMLPackages...)
		}
	}
	_ = os.RemoveAll(filepath.Join(dir, "cache")) // the downloads, now unpacked: a few hundred MB
	// Whether the card really works with this build; if not, it's kept for DirectML when that
	// passed its test, else the processor, which works anywhere, rather than failing every song first.
	if acc.ID == "dml" && !dml {
		fmt.Fprintf(logw, "%s didn't pass its test, so Demucs will use the processor\n", acc.Name)
		acc = cpuAccel
	}
	if acc.ID != "cpu" && acc.ID != "dml" {
		step("Checking the graphics card", 97)
		probe := "import torch,sys;sys.exit(0 if (torch.backends.mps.is_available() if '" + acc.Device + "'=='mps' else torch.cuda.is_available()) else 1)"
		if err := exec.CommandContext(ctx, py, "-c", probe).Run(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if dml {
				fmt.Fprintf(logw, "%s isn't usable from PyTorch here, so Demucs will use DirectML\n", acc.Name)
				acc = dmlAccel
			} else {
				fmt.Fprintf(logw, "%s isn't usable from PyTorch here, so Demucs will use the processor\n", acc.Name)
				acc = cpuAccel
			}
		}
	}
	in := Info{Version: strings.TrimSpace(string(ver)), Model: Model, UV: UVVersion, Python: PythonVersion, Packages: pkgs, Accel: acc.ID, DirectML: dml, Installed: time.Now().UTC()}
	b, _ := json.MarshalIndent(in, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, infoFile), b, 0o644); err != nil {
		return err
	}
	step("Installed", 100)
	return nil
}

// installDirectML adds ONNX Runtime's DirectML build to the install in dir, converts the model
// to ONNX, checks the conversion against PyTorch on the processor, and runs it once on the card.
// got: the packages went in; ok: the card passed and DirectML can be used.
func installDirectML(ctx context.Context, dir, uv, py string, env []string, step Step, logw io.Writer) (got, ok bool, err error) {
	run := func(timeout time.Duration, prog string, args ...string) error {
		c, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		fmt.Fprintf(logw, "> %s %s\n", filepath.Base(prog), strings.Join(args, " "))
		cmd := exec.CommandContext(c, prog, args...)
		cmd.Dir = dir
		cmd.Env = env
		cmd.Stdout, cmd.Stderr = logw, logw
		cmd.WaitDelay = 10 * time.Second
		return cmd.Run()
	}
	step("Getting ONNX Runtime for DirectML", 87)
	wheels := filepath.Join(dir, "cache", "wheels")
	var paths []string
	for _, w := range dmlWheels {
		p, err := fetchChecked(ctx, w.url, w.sha256, filepath.Join(wheels, w.name), 100<<20)
		if err != nil {
			return false, false, fmt.Errorf("%s couldn't be downloaded: %w", w.name, err)
		}
		paths = append(paths, p)
	}
	if err := run(15*time.Minute, uv, append([]string{"pip", "install", "--python", py}, paths...)...); err != nil {
		return false, false, fmt.Errorf("ONNX Runtime couldn't be installed: %w", err)
	}
	script := filepath.Join(dir, onnxScriptFile)
	if err := os.WriteFile(script, onnxScript, 0o644); err != nil {
		return true, false, err
	}
	model := filepath.Join(dir, "models", onnxModelFile)
	step("Converting the model for DirectML", 90)
	// The script compares the converted model with Demucs on the processor and refuses it below
	// 60 dB (they agree to about 80).
	if err := run(20*time.Minute, py, script, "export", "--out", model); err != nil {
		_ = os.Remove(model)
		return true, false, fmt.Errorf("the model couldn't be converted: %w", err)
	}
	step("Checking the graphics card with DirectML", 94)
	if err := run(5*time.Minute, py, script, "probe", "--onnx", model, "--provider", "dml"); err != nil {
		return true, false, fmt.Errorf("the test run through DirectML failed: %w", err)
	}
	return true, true, nil
}

// Remove deletes everything Install put in dir.
func Remove(dir string) error {
	if dir == "" || filepath.Base(dir) != "separator" {
		return errors.New("not a separator folder")
	}
	return os.RemoveAll(dir)
}

// Size is how much room dir takes.
func Size(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, e := d.Info(); e == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

// withPath puts first on the PATH in env.
func withPath(env []string, first string) []string {
	if first == "" {
		return env
	}
	for i, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if strings.EqualFold(k, "PATH") {
			env[i] = k + "=" + first + string(os.PathListSeparator) + v
			return env
		}
	}
	return append(env, "PATH="+first)
}

// WithPath is withPath for the server, which runs the installed separator the same way.
func WithPath(env []string, first string) []string { return withPath(env, first) }

const uvBase = "https://github.com/astral-sh/uv/releases/download/"

// fetchChecked downloads url to path, at most limit bytes, and refuses it (removing it) unless
// its SHA-256 is want.
func fetchChecked(ctx context.Context, url, want, path string, limit int64) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "WittyMediaServer")
	res, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("%s answered %s", req.URL.Host, res.Status)
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(res.Body, limit))
	f.Close()
	if err == nil {
		if got := hex.EncodeToString(h.Sum(nil)); got != want {
			err = fmt.Errorf("its checksum is wrong (%s), so it wasn't used", got[:12])
		}
	}
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// fetchUV downloads the installer's archive, refuses it unless its SHA-256 is the one kept
// here, and takes the uv program out of it.
func fetchUV(ctx context.Context, a uvAsset, into string) (string, error) {
	archive, err := fetchChecked(ctx, uvBase+UVVersion+"/"+a.name, a.sha256, filepath.Join(into, a.name), 200<<20)
	if err != nil {
		return "", err
	}
	defer os.Remove(archive)
	name := "uv"
	if strings.HasSuffix(a.name, ".zip") {
		name = "uv.exe"
	}
	out := filepath.Join(into, name)
	if strings.HasSuffix(a.name, ".zip") {
		err = unzipOne(archive, name, out)
	} else {
		err = untarOne(archive, name, out)
	}
	return out, err
}

func unzipOne(archive, name, out string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, f := range z.File {
		if filepath.Base(f.Name) != name || f.FileInfo().IsDir() {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		defer r.Close()
		return writeExe(out, io.LimitReader(r, 200<<20))
	}
	return fmt.Errorf("%s isn't in the download", name)
}

func untarOne(archive, name, out string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(bufio.NewReader(f))
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("%s isn't in the download", name)
			}
			return err
		}
		if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == name {
			return writeExe(out, io.LimitReader(tr, 200<<20))
		}
	}
}

func writeExe(out string, r io.Reader) error {
	f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// License is one thing the install brought, as the Licenses page lists it.
type License struct {
	Name, Version, License, URL, Text string
}

// Licenses are everything installed in dir, from what each package carries: its name, version,
// license and license text, and Python's own, and uv's. Empty when nothing is installed.
func Licenses(dir string) []License {
	in := ReadInfo(dir)
	if in == nil {
		return nil
	}
	var out []License
	out = append(out, License{Name: "uv", Version: in.UV, License: "MIT OR Apache-2.0", URL: "https://github.com/astral-sh/uv",
		Text: "uv is made by Astral Software Inc. and offered under the MIT license or the Apache License 2.0, at your choice. See https://github.com/astral-sh/uv/blob/main/LICENSE-MIT and LICENSE-APACHE."})
	if p := pythonLicense(filepath.Join(dir, "python")); p != "" {
		out = append(out, License{Name: "Python", Version: in.Python, License: "PSF-2.0", URL: "https://www.python.org", Text: p})
	}
	first := len(out) // uv and Python stay first; the packages follow by name
	for _, site := range sitePackages(dir) {
		ents, _ := os.ReadDir(site)
		for _, e := range ents {
			if !e.IsDir() || !strings.HasSuffix(e.Name(), ".dist-info") {
				continue
			}
			if l, ok := readDistInfo(filepath.Join(site, e.Name())); ok {
				out = append(out, l)
			}
		}
	}
	pk := out[first:]
	sort.SliceStable(pk, func(i, j int) bool { return strings.ToLower(pk[i].Name) < strings.ToLower(pk[j].Name) })
	// The model's weights: fetched from Meta, not from us, and under Demucs's own license.
	mit := ""
	for _, l := range pk {
		if strings.EqualFold(l.Name, "demucs") {
			mit = l.Text
		}
	}
	model := in.Model
	if model == "" {
		model = Model
	}
	note := "The " + model + " model's weights, as published by Meta Platforms, Inc. with Demucs (" + ModelURL + "), fetched from there by this server when Demucs was installed. Demucs, its pretrained models included, is released under the MIT license."
	if in.DirectML {
		note += " On this machine they were also converted to ONNX, for DirectML; the converted copy stays here and isn't shared."
	}
	out = append(out, License{Name: "Demucs " + model + " model", Version: in.Version, License: "MIT", URL: "https://github.com/facebookresearch/demucs",
		Text: strings.TrimSpace(note + "\n\n" + mit)})
	return out
}

func sitePackages(dir string) []string {
	if runtime.GOOS == "windows" {
		return []string{filepath.Join(dir, "venv", "Lib", "site-packages")}
	}
	m, _ := filepath.Glob(filepath.Join(dir, "venv", "lib", "python*", "site-packages"))
	return m
}

// pythonLicense is Python's LICENSE.txt from the Python uv installed.
func pythonLicense(root string) string {
	var found string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return fs.SkipDir
		}
		if d.IsDir() && strings.Count(strings.TrimPrefix(p, root), string(os.PathSeparator)) > 4 {
			return fs.SkipDir
		}
		if !d.IsDir() && d.Name() == "LICENSE.txt" {
			if b, e := os.ReadFile(p); e == nil && strings.Contains(string(b), "PYTHON SOFTWARE FOUNDATION") {
				found = string(b)
			}
		}
		return nil
	})
	return found
}

// readDistInfo reads a package's METADATA and its license files.
func readDistInfo(di string) (License, bool) {
	b, err := os.ReadFile(filepath.Join(di, "METADATA"))
	if err != nil {
		return License{}, false
	}
	var l License
	var classifier string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			break // the headers end where the description starts
		}
		k, v, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		switch k {
		case "Name":
			l.Name = v
		case "Version":
			l.Version = v
		case "License-Expression":
			l.License = v
		case "License":
			if l.License == "" && len(v) <= 60 && !strings.Contains(v, "\n") {
				l.License = v
			}
		case "Home-page":
			l.URL = v
		case "Project-URL":
			if l.URL == "" {
				if _, u, ok := strings.Cut(v, ", "); ok {
					l.URL = u
				}
			}
		case "Classifier":
			if strings.HasPrefix(v, "License :: ") && classifier == "" {
				parts := strings.Split(v, " :: ")
				classifier = parts[len(parts)-1]
			}
		}
	}
	if l.Name == "" {
		return License{}, false
	}
	if l.License == "" || strings.EqualFold(l.License, "UNKNOWN") {
		l.License = classifier
	}
	if l.License == "" {
		l.License = "See its license text"
	}
	var texts []string
	_ = filepath.WalkDir(di, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		n := strings.ToUpper(d.Name())
		if strings.HasPrefix(n, "LICENSE") || strings.HasPrefix(n, "LICENCE") || strings.HasPrefix(n, "COPYING") || strings.HasPrefix(n, "NOTICE") || strings.HasPrefix(n, "AUTHORS") {
			if fi, e := d.Info(); e == nil && fi.Size() < 2<<20 {
				if t, e := os.ReadFile(p); e == nil {
					texts = append(texts, string(t))
				}
			}
		}
		return nil
	})
	// Some packages (ONNX Runtime) keep their license in the package's own folder instead:
	// the folders top_level.txt names, beside the dist-info, at their top only.
	if len(texts) == 0 {
		if b, err := os.ReadFile(filepath.Join(di, "top_level.txt")); err == nil {
			for _, top := range strings.Fields(string(b)) {
				if top == "" || strings.ContainsAny(top, `/\.`) {
					continue
				}
				ents, _ := os.ReadDir(filepath.Join(filepath.Dir(di), top))
				for _, e := range ents {
					n := strings.ToUpper(e.Name())
					if e.IsDir() || !(strings.HasPrefix(n, "LICENSE") || strings.HasPrefix(n, "LICENCE") || strings.HasPrefix(n, "COPYING") || strings.HasPrefix(n, "NOTICE") || strings.HasPrefix(n, "THIRDPARTYNOTICES")) {
						continue
					}
					if fi, e2 := e.Info(); e2 == nil && fi.Size() < 2<<20 {
						if t, e2 := os.ReadFile(filepath.Join(filepath.Dir(di), top, e.Name())); e2 == nil {
							texts = append(texts, string(t))
						}
					}
				}
			}
		}
	}
	l.Text = strings.Join(texts, "\n\n")
	return l, true
}
