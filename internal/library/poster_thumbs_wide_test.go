// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A backdrop asked for wider than the widest copy (a wide screen's 2560, a 4K one's 3840) gets
// the widest copy, not the 3000-pixel original.
func TestAWideAskGetsTheWidestCopyNotTheOriginal(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 3000, 1688)), nil); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "backdrop.jpg")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Scanner{ImagesDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, w := range []int{2560, 3840} {
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		got := s.SmallCopyOf(context.Background(), f, p, w)
		f.Close()
		if !strings.HasPrefix(got, "cache:") || !strings.HasSuffix(got, "-2560.jpg") {
			t.Fatalf("w=%d: %q, want the 2560 copy", w, got)
		}
		c, err := os.Open(filepath.Join(s.ImagesDir, strings.TrimPrefix(got, "cache:")))
		if err != nil {
			t.Fatal(err)
		}
		cfg, _, err := image.DecodeConfig(c)
		c.Close()
		if err != nil || cfg.Width != 2560 {
			t.Fatalf("w=%d: the copy is %dx%d (%v)", w, cfg.Width, cfg.Height, err)
		}
	}
	// One already smaller than what's asked is sent as it is.
	f, _ := os.Open(p)
	defer f.Close()
	if got := s.PosterThumb(context.Background(), "cache:missing.jpg", 3840); got != "cache:missing.jpg" {
		t.Errorf("a cache picture that isn't there: %q", got)
	}
}
