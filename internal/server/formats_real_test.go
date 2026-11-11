// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// TestRealMediaListing scans the owner's own folders, read only, with no lookups and
// nothing written beside the media, and writes what it found to a file, for comparing
// one version of the scan with another. Set WMS_REAL_MEDIA to the media folder and
// WMS_REAL_OUT to the file. It's for people to run; it does nothing otherwise.
func TestRealMediaListing(t *testing.T) {
	root, out := os.Getenv("WMS_REAL_MEDIA"), os.Getenv("WMS_REAL_OUT")
	if root == "" || out == "" {
		t.Skip("set WMS_REAL_MEDIA and WMS_REAL_OUT")
	}
	tools, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("no FFmpeg here")
	}
	ts := realMediaServer(t, tools)
	no := func() bool { return false }
	ts.Scanner.SaveMeta, ts.Scanner.WikidataOn, ts.Scanner.TVMazeOn, ts.Scanner.MusicBrainzOn = no, no, no, no
	_, _ = ts.user(t, "tester", auth.KindAdult, true, "")
	var lines []string
	for _, l := range []struct{ typ, dir string }{
		{"movies", "Movies"}, {"shows", "TV Shows"}, {"music", "Music"}, {"podcasts", "Podcasts"}, {"books", "Ebook"},
		{"discs", "DVD ISOs"}, {"discs", "BluRay ISOs"}, {"discs", "3D BluRay ISOs"},
	} {
		dir := filepath.Join(root, l.dir)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		id := ts.library(t, l.typ, dir)
		scanAndWait(t, ts.Server, id)
		items, _, err := ts.Store.Items(context.Background(), library.ItemQuery{LibraryID: id, NoTotal: true, Limit: 100000})
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			rel, _ := filepath.Rel(root, it.Path)
			art := "no-art"
			if it.Poster != "" {
				art = "art"
			}
			lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%s\t%.0f\t%dx%d\t%s\t%s", filepath.ToSlash(rel), it.Kind, it.Container, it.VideoCodec, it.RuntimeSec, it.Width, it.Height, art, it.Title))
		}
	}
	sort.Strings(lines)
	if err := os.WriteFile(out, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d items", len(lines))
}

// realMediaServer is a server with this machine's FFmpeg, like the format table's.
func realMediaServer(t *testing.T, tools *ffmpeg.Tools) *testServer {
	t.Helper()
	ts := newTestServer(t)
	ts.Tools = tools
	ts.Scanner = library.NewScanner(ts.Store, tools, t.TempDir(), func() string { return "" }, ts.Log)
	if err := os.MkdirAll(ts.Scanner.ImagesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return ts
}
