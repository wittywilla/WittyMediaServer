// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/wittyinfo"
)

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 20, 30)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// A film's NFO, poster, skips and subtitles go into its .wittyinfo, and come back out again.
func TestConvertGatherAndSpread(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	ctx := context.Background()
	dir := filepath.Dir(f.video.Path)
	stem := strings.TrimSuffix(f.video.Path, ".mkv")
	put(t, stem+".nfo", []byte(`<?xml version="1.0"?><movie><title>Film From NFO</title><year>2001</year><plot>What it is about.</plot><genre>Drama</genre><uniqueid type="tmdb" default="true">603</uniqueid></movie>`))
	put(t, filepath.Join(dir, "poster.jpg"), jpegBytes(t))
	put(t, f.video.Path+".skip.json", []byte(`{"intro":{"start":30,"end":90},"credits":{"start":1300}}`))
	put(t, stem+".de.forced.srt", []byte("1\n00:00:05,000 --> 00:00:06,000\nHallo\n"))

	// Counting first, as setup does.
	c := decode[map[string]any](t, ts.do(t, "GET", "/api/wittyinfo/convert/counts", f.admin, nil))
	counts := c["counts"].(map[string]any)
	if counts["withNfo"].(float64) != 1 || counts["withArt"].(float64) != 1 || counts["withSkips"].(float64) != 1 || counts["withInfo"].(float64) != 0 {
		t.Fatalf("counts: %v", counts)
	}
	if w := ts.do(t, "GET", "/api/wittyinfo/convert/counts", f.adult, nil); w.Code != 403 {
		t.Error("a viewer counted", w.Code)
	}

	libs := []int64{f.video.LibraryID}
	o := library.ConvertOptions{Subtitles: true, Retire: true, Beside: true, Tool: "test"}
	rep, err := ts.Scanner.GatherInfo(ctx, libs, o, nil)
	if err != nil || rep.Converted != 1 || rep.Failed != 0 {
		t.Fatalf("%+v %v", rep, err)
	}
	b, err := os.ReadFile(stem + ".wittyinfo")
	if err != nil {
		t.Fatal("no .wittyinfo beside the film:", err)
	}
	x, err := wittyinfo.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if x.Metadata.Title != "Film From NFO" || *x.Metadata.Year != 2001 || x.Metadata.ExternalIDs["tmdb"] != "603" || x.Artwork.First("poster") != "images/poster.jpg" ||
		len(x.Segments) != 2 || len(x.Subtitles) != 1 || x.Subtitles[0].Language != "de" || !x.Subtitles[0].Forced || x.Identity == nil {
		t.Fatalf("gathered: %+v %+v %+v %+v", x.Metadata, x.Artwork, x.Segments, x.Subtitles)
	}
	for _, p := range []string{stem + ".nfo", filepath.Join(dir, "poster.jpg"), f.video.Path + ".skip.json", stem + ".de.forced.srt"} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s wasn't set aside", filepath.Base(p))
		}
		if _, err := os.Stat(p + ".bak"); err != nil {
			t.Errorf("%s.bak missing", filepath.Base(p))
		}
	}
	if rep.Retired != 4 {
		t.Errorf("retired %d", rep.Retired)
	}
	// Films and episodes, and shows, each counted as what they are (#535).
	if rep.Titles != 1 || rep.TitlesConverted != 1 || rep.Songs != 0 || rep.Albums != 0 {
		t.Errorf("counted: %+v", rep)
	}
	if got := convertResult("wittyinfo", rep); got != "1 of 1 film or episode converted; 4 old files renamed to .bak." {
		t.Errorf("result: %q", got)
	}
	// Gathering again finds nothing new.
	if rep, _ := ts.Scanner.GatherInfo(ctx, libs, o, nil); rep.Converted != 0 || rep.Unchanged != 1 {
		t.Errorf("again: %+v", rep)
	}

	// And back out: NFO, poster, .skip.json and subtitles from the .wittyinfo.
	if err := ts.DB.SetSetting(ctx, "saveMetadata", "1"); err != nil {
		t.Fatal(err)
	}
	rep, err = ts.Scanner.SpreadInfo(ctx, libs, library.ConvertOptions{Subtitles: true, Retire: true}, nil)
	if err != nil || rep.Converted != 1 || rep.Failed != 0 {
		t.Fatalf("spread: %+v %v", rep, err)
	}
	nfo, _ := os.ReadFile(stem + ".nfo")
	if !strings.Contains(string(nfo), "Film From NFO") {
		t.Errorf("nfo: %s", nfo)
	}
	skip, _ := os.ReadFile(stem + ".skip.json")
	if !strings.Contains(string(skip), `"start": 30`) {
		t.Errorf("skip.json: %s", skip)
	}
	if _, err := os.Stat(stem + ".de.forced.srt"); err != nil {
		t.Error("subtitle not written back")
	}
	poster, _, _ := library.ArtPaths(f.video)
	if _, err := os.Stat(poster); err != nil {
		t.Errorf("poster not written back at %s", poster)
	}
	if _, err := os.Stat(stem + ".wittyinfo.bak"); err != nil {
		t.Error("the .wittyinfo wasn't set aside")
	}

	// The API starts it as a task; a viewer can't.
	if w := ts.do(t, "POST", "/api/wittyinfo/convert", f.adult, map[string]any{"direction": "gather"}); w.Code != 403 {
		t.Error("a viewer converted", w.Code)
	}
	if w := ts.do(t, "POST", "/api/wittyinfo/convert", f.admin, map[string]any{"direction": "sideways"}); w.Code != 400 {
		t.Error(w.Code)
	}
}
