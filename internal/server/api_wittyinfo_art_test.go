// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/wittyinfo"
)

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// A title's file holds several posters: added, put first, offered by the artwork chooser
// and taken out one at a time.
func TestInfoSeveralPictures(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	ctx := context.Background()
	if err := ts.DB.SetSetting(ctx, "saveMetadata", "0"); err != nil {
		t.Fatal(err)
	}
	u := fmt.Sprintf("/api/wittyinfo/item/%d", f.video.ID)
	if w := ts.do(t, "PUT", u+"/images/poster", f.admin, pngOf(t, 8, 12)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := ts.do(t, "PUT", u+"/images/poster?add=1", f.admin, pngOf(t, 9, 12)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	a := decode[infoAnswer](t, ts.do(t, "GET", u, f.admin, nil))
	if got := a.Info.Artwork["poster"]; len(got) != 2 || got[0] != "images/poster.png" || got[1] != "images/poster-2.png" || a.Version != wittyinfo.Version || a.Newer {
		t.Fatalf("posters %v, version %d newer %v", got, a.Version, a.Newer)
	}
	first, _ := ts.Store.Item(ctx, f.video.ID)

	// The chooser offers both, and the second can be shown first.
	c := decode[struct {
		Candidates []artCandidate `json:"candidates"`
	}](t, ts.do(t, "GET", fmt.Sprintf("/api/items/%d/artwork?kind=poster", f.video.ID), f.admin, nil))
	n := 0
	for _, x := range c.Candidates {
		if x.Source == "wittyinfo" && strings.HasPrefix(x.URL, "cache:wi-") {
			n++
		}
	}
	if n != 1 { // the first is the poster in use, offered as "current"
		t.Errorf("the chooser offers %d pictures from the file: %+v", n, c.Candidates)
	}
	// So does the chooser the title's page uses.
	mc := decode[struct {
		Candidates []struct {
			URL   string `json:"url"`
			Label string `json:"sourceLabel"`
		} `json:"candidates"`
	}](t, ts.do(t, "GET", fmt.Sprintf("/api/meta/item/%d/artwork?kind=poster", f.video.ID), f.admin, nil))
	n = 0
	for _, x := range mc.Candidates {
		if x.Label == "The title’s .wittyinfo" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the title's chooser offers %d pictures from the file: %+v", n, mc.Candidates)
	}
	if w := ts.do(t, "PUT", u+"/images/poster?first=images/poster-2.png", f.admin, nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	a = decode[infoAnswer](t, ts.do(t, "GET", u, f.admin, nil))
	if a.Info.Artwork.First("poster") != "images/poster-2.png" {
		t.Fatalf("not put first: %v", a.Info.Artwork["poster"])
	}
	if now, _ := ts.Store.Item(ctx, f.video.ID); now.Poster == first.Poster || !strings.HasPrefix(now.Poster, "cache:wi-") {
		t.Errorf("the poster shown didn't follow: %q then %q", first.Poster, now.Poster)
	}
	if w := ts.do(t, "PUT", u+"/images/poster?first=images/none.png", f.admin, nil); w.Code != 400 {
		t.Errorf("a picture it doesn't have put first: %d", w.Code)
	}
	if w := ts.do(t, "PUT", u+"/images/poster?add=1", f.adult, pngOf(t, 9, 12)); w.Code != 403 {
		t.Errorf("a viewer added a picture: %d", w.Code)
	}

	// One out, then the rest.
	if w := ts.do(t, "DELETE", u+"/images/poster?name=images/poster.png", f.admin, nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	a = decode[infoAnswer](t, ts.do(t, "GET", u, f.admin, nil))
	if got := a.Info.Artwork["poster"]; len(got) != 1 || got[0] != "images/poster-2.png" || len(a.Files) != 1 {
		t.Fatalf("after taking one out: %v %+v", got, a.Files)
	}
	if w := ts.do(t, "DELETE", u+"/images/poster", f.admin, nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	a = decode[infoAnswer](t, ts.do(t, "GET", u, f.admin, nil))
	if a.Info != nil && len(a.Info.Artwork) != 0 {
		t.Errorf("all out: %v", a.Info.Artwork)
	}
}

// Further pictures beside a film (Film-poster-2.jpg, Film-fanart2.png) go into its
// .wittyinfo, and come back out under the same kind of name.
func TestConvertSeveralPictures(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	ctx := context.Background()
	dir := filepath.Dir(f.video.Path)
	stem := strings.TrimSuffix(f.video.Path, ".mkv")
	put(t, filepath.Join(dir, "poster.jpg"), jpegBytes(t))
	put(t, stem+"-poster-2.png", pngOf(t, 10, 15))
	put(t, stem+"-poster-3.png", pngOf(t, 11, 15))
	put(t, stem+"-fanart2.png", pngOf(t, 16, 9))
	put(t, stem+"-posterboy.png", pngOf(t, 4, 4)) // not a numbered poster

	libs := []int64{f.video.LibraryID}
	rep, err := ts.Scanner.GatherInfo(ctx, libs, library.ConvertOptions{Retire: true, Beside: true, Tool: "test"}, nil)
	if err != nil || rep.Converted != 1 || rep.Failed != 0 {
		t.Fatalf("%+v %v", rep, err)
	}
	b, err := os.ReadFile(stem + ".wittyinfo")
	if err != nil {
		t.Fatal(err)
	}
	x, err := wittyinfo.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if p := x.Artwork["poster"]; len(p) != 3 || p[0] != "images/poster.jpg" {
		t.Fatalf("posters: %v", p)
	}
	if p := x.Artwork["backdrop"]; len(p) != 1 {
		t.Fatalf("backdrops: %v", p)
	}
	if _, err := os.Stat(stem + "-poster-2.png.bak"); err != nil {
		t.Error("a further poster wasn't set aside")
	}
	if _, err := os.Stat(stem + "-posterboy.png"); err != nil {
		t.Error("a picture that isn't a numbered poster was taken")
	}
	if rep, _ := ts.Scanner.GatherInfo(ctx, libs, library.ConvertOptions{Beside: true}, nil); rep.Converted != 0 {
		t.Errorf("gathering again took more: %+v", rep)
	}

	if err := ts.DB.SetSetting(ctx, "saveMetadata", "1"); err != nil {
		t.Fatal(err)
	}
	rep, err = ts.Scanner.SpreadInfo(ctx, libs, library.ConvertOptions{}, nil)
	if err != nil || rep.Converted != 1 || rep.Failed != 0 {
		t.Fatalf("spread: %+v %v", rep, err)
	}
	for _, name := range []string{"-poster-2.png", "-poster-3.png"} {
		if got, err := os.ReadFile(stem + name); err != nil || len(got) == 0 {
			t.Errorf("%s not written back: %v", name, err)
		}
	}
}
