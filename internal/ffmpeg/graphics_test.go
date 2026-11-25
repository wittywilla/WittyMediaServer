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
