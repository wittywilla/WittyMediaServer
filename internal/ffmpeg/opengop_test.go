// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package ffmpeg

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// An open-GOP encode's keyframes aren't IDR frames (#760): a live copy of one would give players
// nothing to start decoding from, so WittyTV converts it.
func TestOpenGOPFrom(t *testing.T) {
	idr := "[trace_headers @ 0x1] nal_unit_type 00101 = 5\n"
	slice := "[trace_headers @ 0x1] nal_unit_type 00001 = 1\n"
	sps := "[trace_headers @ 0x1] nal_unit_type 00111 = 7\n"
	for _, c := range []struct {
		trace       string
		open, known bool
	}{
		{sps + idr, false, true},
		{sps + slice, true, true},
		{sps + idr + slice, false, true},
		{sps, false, false},
		{"", false, false},
	} {
		if open, known := openGOPFrom(c.trace); open != c.open || known != c.known {
			t.Errorf("%q: open %v known %v, want %v %v", c.trace, open, known, c.open, c.known)
		}
	}
}

func TestOpenGOPOnRealEncodes(t *testing.T) {
	bin := os.Getenv("WMS_FFMPEG")
	if bin == "" {
		var err error
		if bin, err = exec.LookPath("ffmpeg"); err != nil {
			t.Skip("no ffmpeg here")
		}
	}
	dir := t.TempDir()
	make := func(name, params string) string {
		p := filepath.Join(dir, name+".mkv")
		out, err := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25", "-t", "8",
			"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-bf", "2", "-x264-params", params, p).CombinedOutput()
		if err != nil {
			t.Skipf("can't make a sample: %v %s", err, out)
		}
		return p
	}
	tools := &Tools{FFmpeg: bin}
	ctx := context.Background()
	if open, known := tools.OpenGOP(ctx, make("closed", "keyint=50:min-keyint=50:scenecut=0"), 0, 4.5); open || !known {
		t.Errorf("a closed-GOP encode: open %v known %v", open, known)
	}
	if open, known := tools.OpenGOP(ctx, make("open", "keyint=50:min-keyint=50:scenecut=0:open-gop=1"), 0, 4.5); !open || !known {
		t.Errorf("an open-GOP encode: open %v known %v", open, known)
	}
}
