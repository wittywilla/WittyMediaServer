// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/channels"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// A library channel copies H.264 only when its keyframes are IDR frames (#760). An open-GOP encode,
// joined part way through, would give players no frame to start from: graphics-chip decoders held the
// picture while the sound played. Both are joined part way in, as a tune-in does.
func TestLiveCopiesOnlyClosedGOP(t *testing.T) {
	tools, err := ffmpeg.Locate(os.Getenv("WMS_FFMPEG"), os.Getenv("WMS_FFPROBE"))
	if err != nil || tools.FFmpeg == "" || tools.FFprobe == "" {
		t.Skip("no FFmpeg here: set WMS_FFMPEG")
	}
	ts := formatServer(t, tools)
	dir := t.TempDir()
	lib := ts.library(t, "movies", dir)
	ctx := context.Background()
	for _, c := range []struct {
		name, params string
		copied       bool
	}{
		{"closed", "keyint=50:min-keyint=50:scenecut=0", true},
		{"open", "keyint=50:min-keyint=50:scenecut=0:open-gop=1", false},
	} {
		p := filepath.Join(dir, c.name+".mkv")
		out, err := exec.Command(tools.FFmpeg, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25",
			"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "12", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
			"-bf", "2", "-x264-params", c.params, "-c:a", "aac", "-shortest", p).CombinedOutput()
		if err != nil {
			t.Skipf("can't make a sample: %v %s", err, out)
		}
		it := ts.item(t, &library.Item{LibraryID: lib, Kind: "movie", Path: p, Title: c.name, RuntimeSec: 12})
		in, err := ts.itemLiveInput(ctx, &channels.Programme{ItemID: it.ID}, 5.3, 6.7, ffmpeg.Quality{}, 0, true)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if in.CopyVideo != c.copied {
			t.Errorf("%s-GOP H.264 joined part way in: copied %v, want %v", c.name, in.CopyVideo, c.copied)
		}
	}
}
