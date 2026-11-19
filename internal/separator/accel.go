// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package separator

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Accel is what Demucs runs on: a graphics card when there's one it can use, else the
// processor. It's chosen when Demucs is installed (each needs its own PyTorch build) and used
// for every song after, and a song that fails on the card is separated again on the processor.
//
// The order, fastest first: CUDA (NVIDIA, Windows and Linux), ROCm (AMD, Linux) and Metal
// (Apple silicon) through PyTorch; then DirectML (Intel and AMD cards on Windows) through ONNX
// Runtime, with the model converted on this machine (demucs_onnx.py); then the processor. Intel
// Macs use the processor: Metal through PyTorch needs Apple silicon, and CoreML would mean a
// second conversion for a few old machines.
type Accel struct {
	// ID: "cuda" (NVIDIA), "rocm" (AMD, on Linux), "mps" (Apple silicon), "dml" (DirectML,
	// Windows) or "cpu".
	ID string `json:"id"`
	// Name says it for people: "NVIDIA graphics (CUDA)".
	Name string `json:"name"`
	// Device is what Demucs is told to use (its -d): "cuda" for both CUDA and ROCm builds. For
	// DirectML it's "cpu": PyTorch only does the spectrogram steps, ONNX Runtime the rest.
	Device string `json:"device"`
	// Index is the PyTorch package index holding the build for it.
	Index string `json:"-"`
	// DownloadMB is roughly how much the install downloads with this build.
	DownloadMB int `json:"downloadMB"`
	// Why says, for "cpu", why no card is used.
	Why string `json:"why,omitempty"`
}

const torchIndex = "https://download.pytorch.org/whl/"

var cpuAccel = Accel{ID: "cpu", Name: "the processor", Device: "cpu", Index: torchIndex + "cpu", DownloadMB: 1100}

// dmlAccel is an Intel or AMD card on Windows, through DirectML: PyTorch's processor build,
// and ONNX Runtime's DirectML build on top (about 40 MB more).
var dmlAccel = Accel{ID: "dml", Name: "DirectX 12 graphics (DirectML)", Device: "cpu", Index: torchIndex + "cpu", DownloadMB: 1150}

// DirectMLOn says whether ONNX Runtime's DirectML build is made for a system: 64-bit Windows on
// Intel or AMD processors only (none for Windows on ARM).
func DirectMLOn(goos, goarch string) bool { return goos == "windows" && goarch == "amd64" }

// probe is what DetectAccel looks at, so tests can say what a machine has.
type probe struct {
	goos, goarch string
	nvidia       bool   // an NVIDIA driver is installed (nvidia-smi, or the kernel module)
	rocm         bool   // AMD's ROCm is installed (/opt/rocm and /dev/kfd)
	otherGPU     string // another card's maker, "AMD" or "Intel" (on Windows), which only DirectML reaches
}

// DetectAccel looks at this machine.
func DetectAccel() Accel { return pickAccel(probeMachine()) }

func probeMachine() probe {
	p := probe{goos: runtime.GOOS, goarch: runtime.GOARCH}
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		p.nvidia = true
	} else if runtime.GOOS == "windows" {
		if _, err := os.Stat(filepath.Join(os.Getenv("SystemRoot"), "System32", "nvidia-smi.exe")); err == nil {
			p.nvidia = true
		}
	} else if _, err := os.Stat("/proc/driver/nvidia/version"); err == nil {
		p.nvidia = true
	}
	if runtime.GOOS == "linux" {
		_, a := os.Stat("/opt/rocm")
		_, b := os.Stat("/dev/kfd")
		p.rocm = a == nil && b == nil
	}
	if runtime.GOOS == "windows" && !p.nvidia {
		// Intel and AMD cards are reached through DirectML. Their drivers are how they're found; a
		// small test run at install decides whether one is really used.
		if m, _ := filepath.Glob(filepath.Join(os.Getenv("SystemRoot"), "System32", "DriverStore", "FileRepository", "*", "amdxx64.dll")); len(m) > 0 {
			p.otherGPU = "AMD"
		} else if m, _ := filepath.Glob(filepath.Join(os.Getenv("SystemRoot"), "System32", "DriverStore", "FileRepository", "iigd*")); len(m) > 0 {
			p.otherGPU = "Intel"
		}
	}
	return p
}

func pickAccel(p probe) Accel {
	switch {
	case p.goos == "darwin" && p.goarch == "arm64":
		// The ordinary build already has Metal; nothing more to download.
		return Accel{ID: "mps", Name: "Apple graphics (Metal)", Device: "mps", Index: torchIndex + "cpu", DownloadMB: 900}
	case p.nvidia && p.goarch == "amd64" && (p.goos == "windows" || p.goos == "linux"):
		return Accel{ID: "cuda", Name: "NVIDIA graphics (CUDA)", Device: "cuda", Index: torchIndex + "cu121", DownloadMB: 3200}
	case p.rocm && p.goos == "linux" && p.goarch == "amd64":
		return Accel{ID: "rocm", Name: "AMD graphics (ROCm)", Device: "cuda", Index: torchIndex + "rocm6.2", DownloadMB: 4200}
	case p.otherGPU != "" && DirectMLOn(p.goos, p.goarch):
		a := dmlAccel
		a.Name = p.otherGPU + " graphics (DirectML)"
		return a
	}
	a := cpuAccel
	switch {
	case p.goos == "darwin":
		a.Why = "Intel Macs have no graphics PyTorch can use, so it uses the processor."
	default:
		a.Why = "No graphics card Demucs can use was found, so it uses the processor."
	}
	return a
}

// AccelByID is the accelerator an install was made for, from installed.json.
func AccelByID(id string) Accel {
	switch id {
	case "cuda":
		return pickAccel(probe{goos: "linux", goarch: "amd64", nvidia: true})
	case "rocm":
		return pickAccel(probe{goos: "linux", goarch: "amd64", rocm: true})
	case "mps":
		return pickAccel(probe{goos: "darwin", goarch: "arm64"})
	case "dml":
		return dmlAccel
	}
	return cpuAccel
}

// OnProcessor is words with Demucs told to use the processor instead: for a song that failed
// on the graphics card. Nil when it already was. A song that failed through DirectML is done
// again by Demucs itself, the same as an install for the processor runs it.
func OnProcessor(words []string) []string {
	for i := 0; i+1 < len(words); i++ {
		if words[i] == "--provider" && words[i+1] == "dml" && len(words) > 0 {
			return []string{words[0], "-m", "demucs", "-n", Model, "-d", "cpu", "-o", "{output}", "{input}"}
		}
	}
	for i := 0; i+1 < len(words); i++ {
		if words[i] == "-d" && words[i+1] != "cpu" {
			out := append([]string(nil), words...)
			out[i+1] = "cpu"
			return out
		}
	}
	return nil
}
