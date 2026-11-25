// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package ffmpeg

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/i18n"
)

// Graphics is what this server can do with its graphics chip, at a glance: the devices it sees
// and whether it may open them, and for each use of a chip whether it's ready, with the reason
// when it isn't. Transcoding uses VA-API (and Quick Sync on Intel); games draw with OpenGL
// through EGL, or with Vulkan. A NAS owner whose chip isn't used sees here which part is missing.
type Graphics struct {
	Devices   []GPUDevice   `json:"devices"`
	Container bool          `json:"container"`
	VAAPI     GraphicsCheck `json:"vaapi"`  // video encoding on the chip (VA-API or Quick Sync)
	EGL       GraphicsCheck `json:"egl"`    // OpenGL for games, through EGL
	Vulkan    GraphicsCheck `json:"vulkan"` // Vulkan for games
}

// GraphicsCheck is one use of the chip: ready or not, and in words which driver or why not.
type GraphicsCheck struct {
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// GraphicsStatus puts together what the last hardware test found and which graphics drivers are
// installed. EGL and Vulkan are only looked up, not run: WittyPlay's own check runs them, in its
// sandbox, when it starts a game.
func (t *Tools) GraphicsStatus() Graphics {
	g := Graphics{Devices: GPUs(), Container: InContainer()}
	var usable *GPUDevice
	for i := range g.Devices {
		if g.Devices[i].Usable && usable == nil {
			usable = &g.Devices[i]
		}
	}
	noDevice := ""
	switch {
	case usable == nil && len(g.Devices) > 0:
		d := g.Devices[0]
		noDevice = strings.TrimSpace(d.Problem + " " + d.Fix)
	case usable == nil && unixGPU:
		noDevice = whyNoRender(nil, "", "", g.Container)
	case usable == nil:
		noDevice = i18n.T("No graphics device was found.")
	}

	// Video encoding: what the encoder tests said.
	list, done := t.HardwareStatus()
	var why []string
	for _, h := range list {
		if h.Vendor != "vaapi" && h.Vendor != "intel" && h.Vendor != "amd" && h.Vendor != "nvidia" {
			continue
		}
		if h.Working {
			g.VAAPI = GraphicsCheck{OK: true, Detail: h.Label}
			break
		}
		if h.Reason != "" {
			why = append(why, h.Label+": "+h.Reason)
		}
	}
	if !g.VAAPI.OK {
		switch {
		case !done:
			g.VAAPI.Detail = i18n.T("Still being tested.")
		case noDevice != "":
			g.VAAPI.Detail = noDevice
		case len(why) > 0:
			g.VAAPI.Detail = why[0]
		default:
			g.VAAPI.Detail = i18n.T("No hardware encoder works here.")
		}
	}

	if runtime.GOOS != "linux" {
		// Windows and macOS have their own graphics libraries with the system; games check them.
		g.EGL = GraphicsCheck{OK: true, Detail: i18n.T("The system's own")}
		g.Vulkan = GraphicsCheck{OK: true, Detail: i18n.T("The system's own")}
		return g
	}
	vendor := ""
	if usable != nil {
		vendor = usable.Vendor
	}
	g.EGL = eglCheck(libDirs(), vendor, noDevice)
	g.Vulkan = vulkanCheck(libDirs(), vulkanICDDirs, vendor, noDevice)
	return g
}

// The folders shared libraries are looked for in, as the dynamic loader would on Debian and the
// common distributions, with LD_LIBRARY_PATH first (WSL's /usr/lib/wsl/lib, for one).
func libDirs() []string {
	triplet := map[string]string{"amd64": "x86_64-linux-gnu", "arm64": "aarch64-linux-gnu", "arm": "arm-linux-gnueabihf", "386": "i386-linux-gnu"}[runtime.GOARCH]
	var dirs []string
	for _, d := range filepath.SplitList(os.Getenv("LD_LIBRARY_PATH")) {
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	if triplet != "" {
		dirs = append(dirs, "/usr/lib/"+triplet, "/lib/"+triplet)
	}
	return append(dirs, "/usr/lib64", "/usr/lib", "/lib64", "/lib", "/usr/local/lib")
}

var vulkanICDDirs = []string{"/usr/share/vulkan/icd.d", "/etc/vulkan/icd.d", "/usr/local/share/vulkan/icd.d"}

// findLib is the first folder that has one of these files, or "".
func findLib(dirs []string, names ...string) string {
	for _, d := range dirs {
		for _, n := range names {
			if _, err := os.Stat(filepath.Join(d, n)); err == nil {
				return filepath.Join(d, n)
			}
		}
	}
	return ""
}

// The Mesa drivers each make of chip draws with: OpenGL's DRI drivers and Vulkan's driver files.
var (
	mesaGL = map[string][]string{
		"intel": {"iris_dri.so", "crocus_dri.so", "i915_dri.so"},
		"amd":   {"radeonsi_dri.so", "r600_dri.so"},
	}
	mesaVulkan = map[string][]string{
		"intel": {"intel_icd", "intel_hasvk_icd"},
		"amd":   {"radeon_icd"},
	}
)

func eglCheck(dirs []string, vendor, noDevice string) GraphicsCheck {
	if findLib(dirs, "libEGL.so.1") == "" {
		return GraphicsCheck{Detail: i18n.T("No OpenGL library (libEGL) is installed. In Docker, pull a newer image: 0.8.5-alpha and later have it.")}
	}
	if vendor == "nvidia" {
		if findLib(dirs, "libEGL_nvidia.so.0") == "" {
			return GraphicsCheck{Detail: i18n.T("NVIDIA's OpenGL library isn't here. In Docker, the NVIDIA Container Toolkit adds it when NVIDIA_DRIVER_CAPABILITIES includes graphics.")}
		}
		return GraphicsCheck{OK: true, Detail: "NVIDIA"}
	}
	if findLib(dirs, "libEGL_mesa.so.0") == "" {
		return GraphicsCheck{Detail: i18n.T("Mesa's OpenGL library (libEGL_mesa) isn't installed.")}
	}
	var dri []string
	for _, d := range dirs {
		dri = append(dri, filepath.Join(d, "dri"))
	}
	if noDevice != "" {
		if findLib(dri, "swrast_dri.so", "kms_swrast_dri.so") != "" {
			return GraphicsCheck{Detail: i18n.F("Only on the processor (llvmpipe): {why}", "why", noDevice)}
		}
		return GraphicsCheck{Detail: noDevice}
	}
	if names := mesaGL[vendor]; len(names) > 0 {
		p := findLib(dri, names...)
		if p == "" {
			return GraphicsCheck{Detail: i18n.F("Mesa has no OpenGL driver for this chip ({names}).", "names", strings.Join(names, ", "))}
		}
		return GraphicsCheck{OK: true, Detail: "Mesa " + strings.TrimSuffix(filepath.Base(p), "_dri.so")}
	}
	return GraphicsCheck{OK: true, Detail: "Mesa"}
}

func vulkanCheck(dirs, icdDirs []string, vendor, noDevice string) GraphicsCheck {
	if findLib(dirs, "libvulkan.so.1") == "" {
		return GraphicsCheck{Detail: i18n.T("The Vulkan loader (libvulkan) isn't installed.")}
	}
	icd := func(prefixes ...string) string {
		for _, d := range icdDirs {
			entries, _ := os.ReadDir(d)
			for _, e := range entries {
				for _, p := range prefixes {
					if strings.HasPrefix(e.Name(), p) && strings.HasSuffix(e.Name(), ".json") {
						return strings.TrimSuffix(e.Name(), ".json")
					}
				}
			}
		}
		return ""
	}
	if noDevice != "" {
		if icd("lvp_icd") != "" {
			return GraphicsCheck{Detail: i18n.F("Only on the processor (lavapipe): {why}", "why", noDevice)}
		}
		return GraphicsCheck{Detail: noDevice}
	}
	if vendor == "nvidia" {
		if icd("nvidia_icd", "nvidia_layers") == "" {
			return GraphicsCheck{Detail: i18n.T("NVIDIA's Vulkan driver isn't here. In Docker, the NVIDIA Container Toolkit adds it when NVIDIA_DRIVER_CAPABILITIES includes graphics.")}
		}
		return GraphicsCheck{OK: true, Detail: "NVIDIA"}
	}
	if names := mesaVulkan[vendor]; len(names) > 0 {
		n := icd(names...)
		if n == "" {
			return GraphicsCheck{Detail: i18n.T("Mesa's Vulkan driver for this chip isn't installed.")}
		}
		return GraphicsCheck{OK: true, Detail: "Mesa " + strings.TrimSuffix(n, "_icd")}
	}
	if n := icd(""); n != "" {
		return GraphicsCheck{OK: true, Detail: n}
	}
	return GraphicsCheck{Detail: i18n.T("No Vulkan driver is installed.")}
}

// Line is the status in one line, for the log and the hardware report.
func (g Graphics) Line() string {
	word := func(c GraphicsCheck) string {
		if c.OK {
			return "yes (" + c.Detail + ")"
		}
		return "no: " + c.Detail
	}
	dev := "none"
	for _, d := range g.Devices {
		if d.Usable {
			dev = d.Path + " (" + strings.Trim(d.Vendor+" "+d.Driver, " ") + ")"
			break
		}
	}
	if dev == "none" && len(g.Devices) > 0 {
		dev = g.Devices[0].Path + ", which can't be opened"
	}
	return "device " + dev + "; video encoding " + word(g.VAAPI) + "; OpenGL (EGL) " + word(g.EGL) + "; Vulkan " + word(g.Vulkan)
}
