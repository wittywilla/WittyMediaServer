// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package ffmpeg

import "testing"

// Vulkan on the processor alone (the Docker image's lavapipe, since 0.8.5) is not a graphics chip:
// HDR films mapped through it never started on a NAS. A real chip's device is left to its own test.
func TestSoftwareVulkanIsNoGraphicsChip(t *testing.T) {
	for log, want := range map[string]string{
		"[AVHWDeviceContext @ 0x1] GPU listing:\n[AVHWDeviceContext @ 0x1]     0: llvmpipe (LLVM 19.1.7, 256 bits) (software) (0x0)\n[AVHWDeviceContext @ 0x1] Device 0 selected: llvmpipe (LLVM 19.1.7, 256 bits) (software) (0x0)\n": "llvmpipe",
		"[AVHWDeviceContext @ 0x1] Device 0 selected: SwiftShader Device (Subzero) (cpu) (0x0)\n":                                                                                                                                      "SwiftShader Device",
		"[AVHWDeviceContext @ 0x1] Device 0 selected: Intel(R) Graphics (ADL-N) (integrated) (0x46d1)\n":                                                                                                                               "",
		"[AVHWDeviceContext @ 0x1] Device 0 selected: NVIDIA GeForce RTX 3070 (discrete) (0x2484)\n":                                                                                                                                   "",
		"Failed to set value 'vulkan=vk' for option 'init_hw_device'":                                                                                                                                                                  "",
	} {
		if got := softwareVulkanDevice(log); got != want {
			t.Errorf("%q: %q, want %q", log, got, want)
		}
	}
}
