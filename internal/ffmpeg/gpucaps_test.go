// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package ffmpeg

import (
	"context"
	"os"
	"strings"
	"testing"
)

// The probe on this machine's own FFmpeg and chips: every path is either tested or says why not,
// and nothing claims to work that wasn't tested. WMS_FFMPEG names the FFmpeg; without it this is
// skipped, since it starts FFmpeg a dozen times. WMS_EXPECT_GPU_PATHS lists paths that must work.
func TestProbeGPUPathsForReal(t *testing.T) {
	ff := os.Getenv("WMS_FFMPEG")
	if ff == "" {
		t.Skip("set WMS_FFMPEG to test this machine's graphics chips")
	}
	tools, err := Locate(ff, os.Getenv("WMS_FFPROBE"))
	if err != nil {
		t.Fatal(err)
	}
	tools.ProbeHardware(context.Background())
	list, _ := tools.HardwareStatus()
	for _, e := range list {
		t.Logf("encoder %-18s compiled=%v working=%v %s", e.Name, e.Compiled, e.Working, e.Reason)
	}
	works := map[string]bool{}
	for _, p := range tools.GPUPaths() {
		t.Logf("path %-22s tested=%v working=%v device=%q %s %s", p.ID, p.Tested, p.Working, p.Device, p.Reason, p.Error)
		if p.Working && !p.Tested {
			t.Errorf("%s works without a test", p.ID)
		}
		if !p.Working && p.Reason == "" {
			t.Errorf("%s doesn't work and doesn't say why", p.ID)
		}
		works[p.ID] = p.Working
	}
	for _, id := range strings.Split(os.Getenv("WMS_EXPECT_GPU_PATHS"), ",") {
		if id != "" && !works[id] {
			t.Errorf("%s should work on this machine", id)
		}
	}
}
