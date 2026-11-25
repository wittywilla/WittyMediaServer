// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package ffmpeg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// What games can draw with, from the libraries and drivers that are installed: the 0.8.4 image's
// missing libEGL, Mesa's drivers for an Intel chip, and the processor's when no chip can be used.
func TestGraphicsChecks(t *testing.T) {
	lib, icd := t.TempDir(), t.TempDir()
	dirs, icds := []string{lib}, []string{icd}

	if c := eglCheck(dirs, "intel", ""); c.OK || !strings.Contains(c.Detail, "libEGL") {
		t.Errorf("no libEGL: %+v", c)
	}
	if c := vulkanCheck(dirs, icds, "intel", ""); c.OK || !strings.Contains(c.Detail, "libvulkan") {
		t.Errorf("no Vulkan loader: %+v", c)
	}

	touch(t, filepath.Join(lib, "libEGL.so.1"))
	touch(t, filepath.Join(lib, "libEGL_mesa.so.0"))
	touch(t, filepath.Join(lib, "libvulkan.so.1"))
	if c := eglCheck(dirs, "intel", ""); c.OK || !strings.Contains(c.Detail, "iris_dri.so") {
		t.Errorf("no driver for the chip: %+v", c)
	}
	touch(t, filepath.Join(lib, "dri", "iris_dri.so"))
	touch(t, filepath.Join(lib, "dri", "swrast_dri.so"))
	touch(t, filepath.Join(icd, "intel_icd.x86_64.json"))
	touch(t, filepath.Join(icd, "lvp_icd.x86_64.json"))
	if c := eglCheck(dirs, "intel", ""); !c.OK || c.Detail != "Mesa iris" {
		t.Errorf("Intel's OpenGL: %+v", c)
	}
	if c := vulkanCheck(dirs, icds, "intel", ""); !c.OK || !strings.HasPrefix(c.Detail, "Mesa intel") {
		t.Errorf("Intel's Vulkan: %+v", c)
	}

	why := "No graphics device was passed to the container."
	if c := eglCheck(dirs, "", why); c.OK || !strings.Contains(c.Detail, "llvmpipe") || !strings.Contains(c.Detail, why) {
		t.Errorf("no chip, OpenGL: %+v", c)
	}
	if c := vulkanCheck(dirs, icds, "", why); c.OK || !strings.Contains(c.Detail, "lavapipe") {
		t.Errorf("no chip, Vulkan: %+v", c)
	}

	if c := eglCheck(dirs, "nvidia", ""); c.OK || !strings.Contains(c.Detail, "graphics") {
		t.Errorf("NVIDIA without its libraries: %+v", c)
	}
}

func TestGraphicsLine(t *testing.T) {
	g := Graphics{
		Devices: []GPUDevice{{Path: "/dev/dri/renderD128", Vendor: "intel", Driver: "i915", Usable: true}},
		VAAPI:   GraphicsCheck{OK: true, Detail: "VAAPI"},
		EGL:     GraphicsCheck{OK: true, Detail: "Mesa iris"},
		Vulkan:  GraphicsCheck{Detail: "Mesa's Vulkan driver for this chip isn't installed."},
	}
	want := "device /dev/dri/renderD128 (intel i915); video encoding yes (VAAPI); OpenGL (EGL) yes (Mesa iris); Vulkan no: Mesa's Vulkan driver for this chip isn't installed."
	if got := g.Line(); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// Software drivers are never a yes: llvmpipe (OpenGL) and lavapipe (Vulkan) are the processor's. With no chip they're
// named as software; with a chip of another make that only they could serve, the same. Transcoding mustn't take them for
// a chip (lavapipe once made HDR tone mapping run on the processor of a NAS, which killed it), and nor should a person.
func TestGraphicsSoftwareIsPlainlyNo(t *testing.T) {
	lib, icd := t.TempDir(), t.TempDir()
	dirs, icds := []string{lib}, []string{icd}
	for _, f := range []string{"libEGL.so.1", "libEGL_mesa.so.0", "libvulkan.so.1", "dri/swrast_dri.so", "dri/kms_swrast_dri.so", "dri/libdril_dri.so"} {
		touch(t, filepath.Join(lib, f))
	}
	touch(t, filepath.Join(icd, "lvp_icd.x86_64.json"))
	why := "No graphics device was passed to the container."
	for _, c := range []struct {
		name string
		got  GraphicsCheck
	}{
		{"OpenGL, no chip", eglCheck(dirs, "", why)},
		{"Vulkan, no chip", vulkanCheck(dirs, icds, "", why)},
		{"OpenGL, a chip of another make with only llvmpipe", eglCheck(dirs, "other", "")},
		{"Vulkan, a chip of another make with only lavapipe", vulkanCheck(dirs, icds, "other", "")},
	} {
		if c.got.OK || !strings.HasPrefix(c.got.Detail, "Software (no graphics chip)") {
			t.Errorf("%s: %+v", c.name, c.got)
		}
	}
	// A real driver for that chip is a yes, and lavapipe beside it doesn't change that.
	touch(t, filepath.Join(lib, "dri", "virtio_gpu_dri.so"))
	touch(t, filepath.Join(icd, "virtio_icd.x86_64.json"))
	if c := eglCheck(dirs, "other", ""); !c.OK || c.Detail != "Mesa virtio_gpu" {
		t.Errorf("OpenGL with a chip's driver: %+v", c)
	}
	if c := vulkanCheck(dirs, icds, "other", ""); !c.OK || !strings.HasPrefix(c.Detail, "virtio_icd") {
		t.Errorf("Vulkan with a chip's driver: %+v", c)
	}
}

// Nothing in transcoding uses Vulkan but HDR tone mapping, whose own test refuses a software device: no encoder, no
// decode-and-resize pipeline, and no decoder asked for by name. "-hwaccel auto" only goes with NVENC, where CUDA comes first;
// lavapipe can't decode video at all (it has no video decode queue), so FFmpeg's auto choice never settles on it.
func TestOnlyToneMappingUsesVulkan(t *testing.T) {
	for _, e := range HWEncoders {
		if strings.Contains(e.Name, "vulkan") {
			t.Errorf("a Vulkan encoder is among the hardware encoders: %s", e.Name)
		}
	}
	for _, p := range GPUPathList {
		if strings.Contains(strings.ToLower(p.Label+p.ID), "vulkan") && p.ID != "tonemap:libplacebo" {
			t.Errorf("a path other than tone mapping uses Vulkan: %s", p.ID)
		}
	}
	for _, f := range []string{"../hls/hls.go", "../hls/live.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(src), "\n")
		for i, l := range lines {
			if strings.Contains(l, `"-hwaccel", "vulkan"`) {
				t.Errorf("%s:%d asks for Vulkan decoding", f, i+1)
			}
			if strings.Contains(l, `"-hwaccel", "auto"`) && (i == 0 || !strings.Contains(lines[i-1], "h264_nvenc")) {
				t.Errorf("%s:%d lets FFmpeg choose a decoder (auto) outside NVENC", f, i+1)
			}
		}
	}
}
