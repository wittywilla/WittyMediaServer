// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package ffmpeg

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Finding FFmpeg starts it once, for both its version and its encoders: the server can't
// listen until it's done, and each start is slow on a busy machine.
func TestLocateStartsFFmpegOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("counts the starts with a shell script")
	}
	real := os.Getenv("WMS_FFMPEG")
	if real == "" {
		var err error
		if real, err = exec.LookPath("ffmpeg"); err != nil {
			t.Skip("no ffmpeg here")
		}
	}
	dir := t.TempDir()
	starts := filepath.Join(dir, "starts")
	wrapper := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\necho start >> '" + starts + "'\nexec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	tools, err := Locate(wrapper, filepath.Join(dir, "ffprobe"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tools.Version, "ffmpeg version ") || strings.ContainsAny(tools.Version, "\r\n") {
		t.Errorf("version %q", tools.Version)
	}
	if !tools.Encoders["aac"] || tools.Encoders["Encoders:"] {
		t.Errorf("encoders %v", tools.Encoders)
	}
	b, _ := os.ReadFile(starts)
	if n := strings.Count(string(b), "start"); n != 1 {
		t.Errorf("FFmpeg was started %d times", n)
	}
}
