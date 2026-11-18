// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package separator

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Accel is what Demucs runs on: a graphics card when there's one PyTorch can use, else the
// processor. It's chosen when Demucs is installed (each needs its own PyTorch build) and used
// for every song after, and a song that fails on the card is separated again on the processor.
type Accel struct {
	// ID: "cuda" (NVIDIA), "rocm" (AMD, on Linux), "mps" (Apple silicon) or "cpu".
	ID string `json:"id"`
	// Name says it for people: "NVIDIA graphics (CUDA)".
	Name string `json:"name"`
	// Device is what Demucs is told to use (its -d): "cuda" for both CUDA and ROCm builds.
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

// probe is what DetectAccel looks at, so tests can say what a machine has.
type probe struct {
	goos, goarch string
	nvidia       bool // an NVIDIA driver is installed (nvidia-smi, or the kernel module)
	rocm         bool // AMD's ROCm is installed (/opt/rocm and /dev/kfd)
	otherGPU     bool // some other card is there (Intel or AMD on Windows), which PyTorch here can't use
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
		// DirectML would reach Intel and AMD cards, but Demucs's spectrogram steps aren't supported
		// by PyTorch's DirectML build, so those stay on the processor; the line in Settings says so.
		if m, _ := filepath.Glob(filepath.Join(os.Getenv("SystemRoot"), "System32", "DriverStore", "FileRepository", "*", "amdxx64.dll")); len(m) > 0 {
			p.otherGPU = true
		} else if m, _ := filepath.Glob(filepath.Join(os.Getenv("SystemRoot"), "System32", "DriverStore", "FileRepository", "iigd*")); len(m) > 0 {
			p.otherGPU = true
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
	}
	a := cpuAccel
	switch {
	case p.otherGPU:
		a.Why = "This graphics card can't run Demucs (PyTorch's DirectML build lacks what Demucs needs), so it uses the processor."
	case p.goos == "darwin":
		a.Why = "Intel Macs have no graphics PyTorch can use, so it uses the processor."
	default:
		a.Why = "No NVIDIA card (CUDA) or AMD card with ROCm was found, so it uses the processor."
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
	}
	return cpuAccel
}

// OnProcessor is words with Demucs told to use the processor instead: for a song that failed
// on the graphics card. Nil when it already was.
func OnProcessor(words []string) []string {
	for i := 0; i+1 < len(words); i++ {
		if words[i] == "-d" && words[i+1] != "cpu" {
			out := append([]string(nil), words...)
			out[i+1] = "cpu"
			return out
		}
	}
	return nil
}
