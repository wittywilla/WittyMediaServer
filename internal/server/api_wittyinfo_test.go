// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
	"github.com/wittywilla/wittymediaserver/pkg/wittyinfo"
)

func encodeInfo(t *testing.T, x *wittyinfo.Info) []byte {
	t.Helper()
	b, err := x.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func itemInfo(duration float64, segs ...wittyinfo.Segment) *wittyinfo.Info {
	x := wittyinfo.New(wittyinfo.ScopeItem)
	x.Identity = &wittyinfo.Identity{DurationSec: wittyinfo.Seconds(duration)}
	x.Segments = segs
	return x
}

func seg(typ string, start, end float64, action string) wittyinfo.Segment {
	return wittyinfo.Segment{Type: typ, Start: wittyinfo.Seconds(start), End: wittyinfo.Sec(end), Action: action}
}

// The .wittyinfo of a video is the most trusted source; a season's and a show's come
// after everything beside the video and before what is guessed.
func TestInfoPrecedence(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	ctx := context.Background()
	tv := ts.library(t, "shows", filepath.Join(f.root, "tv"))
	showDir := filepath.Join(f.root, "tv", "A Show")
	sh := &library.Show{LibraryID: tv, Title: "A Show", SortTitle: "A Show", Path: showDir}
	if err := ts.Store.UpsertShow(ctx, sh); err != nil {
		t.Fatal(err)
	}
	one := 1
	epPath := filepath.Join(showDir, "Season 1", "A Show S01E01.mkv")
	put(t, epPath, []byte("not really an episode"))
	ep := ts.item(t, &library.Item{LibraryID: tv, Kind: "video", Path: epPath, ShowID: &sh.ID, Season: &one, Episode: &one, RuntimeSec: 1400})
	markers := func() mediaextras.Markers {
		return decode[mediaextras.Markers](t, ts.do(t, "GET", f.url("/api/media/%d/markers", ep), f.adult, nil))
	}
	// Chapters only.
	if _, err := ts.DB.ExecContext(ctx, `INSERT INTO media_markers(item_id, scanned) VALUES(?, ?)`, ep.ID, `{"intro":{"start":30,"end":90},"credits":{"start":1300,"end":1400}}`); err != nil {
		t.Fatal(err)
	}
	// The show says the intro is at 0:45 and there's a preview at the end of every episode.
	show := wittyinfo.New(wittyinfo.ScopeShow)
	show.Segments = []wittyinfo.Segment{seg("intro", 45, 90, ""), seg("preview", 1380, 1400, "auto")}
	put(t, filepath.Join(showDir, "show.wittyinfo"), encodeInfo(t, show))
	m := markers()
	if m.Intro.StartSec != 45 || m.Sources["intro"] != mediaextras.SourceShowInfo || m.Credits.StartSec != 1300 || m.Sources["credits"] != mediaextras.SourceChapters {
		t.Fatalf("the show's defaults beat the chapters: %+v %v", m.Intro, m.Sources)
	}
	if m.Preview == nil || m.Preview.Action != "auto" {
		t.Errorf("preview: %+v", m.Preview)
	}
	// The season beats the show; it may sit in the season's folder.
	season := wittyinfo.New(wittyinfo.ScopeSeason)
	season.Season = &one
	season.Segments = []wittyinfo.Segment{seg("intro", 50, 95, "")}
	put(t, filepath.Join(showDir, "Season 1", "season.wittyinfo"), encodeInfo(t, season))
	if m := markers(); m.Intro.StartSec != 50 || m.Sources["intro"] != mediaextras.SourceSeasonInfo || m.Sources["preview"] != mediaextras.SourceShowInfo {
		t.Fatalf("season: %+v %v", m.Intro, m.Sources)
	}
	// A .skip.json beside the episode beats both.
	put(t, epPath+".skip.json", []byte(`{"intro":{"start":40,"end":100}}`))
	if m := markers(); m.Intro.StartSec != 40 || m.Sources["intro"] != mediaextras.SourceSkipJSON {
		t.Fatalf("skip.json: %v", m.Sources)
	}
	// So does what was set by hand.
	if err := ts.Store.SetManualMarkers(ctx, ep.ID, &mediaextras.Markers{Intro: &mediaextras.Range{StartSec: 41, EndSec: 99}}); err != nil {
		t.Fatal(err)
	}
	// And the episode's own .wittyinfo beats everything; it can also say there is no intro.
	own := itemInfo(1400, seg("credits", 1290, 1400, ""), seg("custom", 600, 630, "auto"), seg("postcredits", 1395, 1400, ""))
	own.Absent = []string{"intro"}
	put(t, strings.TrimSuffix(epPath, ".mkv")+".wittyinfo", encodeInfo(t, own))
	m = markers()
	if m.Intro != nil || m.Sources["intro"] != mediaextras.SourceWittyinfo || m.Credits.StartSec != 1290 || m.Source != mediaextras.SourceWittyinfo {
		t.Fatalf("own file: %+v %v", m.Intro, m.Sources)
	}
	if len(m.Custom) != 1 || m.PostCredits == nil || m.PostCredits.Action != "" && m.PostCredits.Action != "mark" {
		t.Errorf("custom and post-credits: %+v %+v", m.Custom, m.PostCredits)
	}
	kinds := []string{}
	for _, s := range m.Segments {
		kinds = append(kinds, s.Kind+":"+s.Action+":"+s.Source)
	}
	if strings.Join(kinds, " ") != "custom:auto:wittyinfo credits:button:wittyinfo preview:auto:wittyinfo-show postcredits:mark:wittyinfo" {
		t.Errorf("segments in time order with sources: %v", kinds)
	}
	// The playback answer carries them too.
	var pb playbackResponse
	ts.fillMarkers(ctx, ep, &pb)
	if len(pb.Segments) != 4 || pb.Info == nil || pb.Info.Origin != "file" || !pb.Info.Defaults {
		t.Errorf("playback: %+v %+v", pb.Segments, pb.Info)
	}
}

// A file made for another cut is flagged and not used, until someone says to use it.
func TestInfoWrongCut(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	ctx := context.Background()
	if _, err := ts.DB.ExecContext(ctx, `INSERT INTO media_markers(item_id, scanned) VALUES(?, ?)`, f.video.ID, `{"intro":{"start":30,"end":90}}`); err != nil {
		t.Fatal(err)
	}
	// Made for the extended cut, 30 minutes longer.
	put(t, strings.TrimSuffix(f.video.Path, ".mkv")+".wittyinfo", encodeInfo(t, itemInfo(3200, seg("intro", 120, 200, ""))))
	m := decode[mediaextras.Markers](t, ts.do(t, "GET", f.url("/api/media/%d/markers", f.video), f.adult, nil))
	if m.Intro.StartSec != 30 || m.Sources["intro"] != mediaextras.SourceChapters {
		t.Fatalf("times for another cut were used: %+v %v", m.Intro, m.Sources)
	}
	a := decode[infoAnswer](t, ts.do(t, "GET", f.url("/api/wittyinfo/item/%d", f.video), f.admin, nil))
	if a.Match == nil || a.Match.Result != wittyinfo.MatchOtherCut || a.Match.Usable || !strings.Contains(a.Match.Message, "53:20") || !strings.Contains(a.Match.Message, "aren't used") || a.Origin != "file" {
		t.Fatalf("not flagged: %+v", a.Match)
	}
	if w := ts.do(t, "POST", f.url("/api/wittyinfo/item/%d/use-anyway", f.video), f.admin, map[string]bool{"use": true}); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	m = decode[mediaextras.Markers](t, ts.do(t, "GET", f.url("/api/media/%d/markers", f.video), f.adult, nil))
	if m.Intro.StartSec != 120 || m.Sources["intro"] != mediaextras.SourceWittyinfo {
		t.Errorf("use anyway: %+v", m.Intro)
	}
	// The same cut, a frame longer, is fine.
	put(t, strings.TrimSuffix(f.video.Path, ".mkv")+".wittyinfo", encodeInfo(t, itemInfo(1400.04, seg("intro", 60, 70, ""))))
	ts.do(t, "POST", f.url("/api/wittyinfo/item/%d/use-anyway", f.video), f.admin, map[string]bool{"use": false})
	if a := decode[infoAnswer](t, ts.do(t, "GET", f.url("/api/wittyinfo/item/%d", f.video), f.admin, nil)); a.Match.Result != wittyinfo.MatchSameCut {
		t.Errorf("%+v", a.Match)
	}
}

// A bad file beside the media is reported, not used, and nothing else breaks.
func TestInfoBadFile(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	put(t, strings.TrimSuffix(f.video.Path, ".mkv")+".wittyinfo", []byte(`{"format":"wittyinfo","version":1,"segments":[{"type":"intro","start":90,"end":10}],"artwork":{"poster":"http://example.com/x.jpg"}}`))
	a := decode[infoAnswer](t, ts.do(t, "GET", f.url("/api/wittyinfo/item/%d", f.video), f.admin, nil))
	if a.Info != nil || !a.FileFound || len(a.Problems) != 2 || a.Problems[0].Where != "segments[0].end" {
		t.Fatalf("%+v", a)
	}
	if w := ts.do(t, "GET", f.url("/api/media/%d/markers", f.video), f.adult, nil); w.Code != 200 {
		t.Error(w.Code)
	}
	// Viewers can't read or change it.
	if w := ts.do(t, "GET", f.url("/api/wittyinfo/item/%d", f.video), f.adult, nil); w.Code != 403 {
		t.Error("a viewer read the editor's answer", w.Code)
	}
	if w := ts.do(t, "PUT", f.url("/api/wittyinfo/item/%d", f.video), f.adult, map[string]any{"info": map[string]any{"format": "wittyinfo", "version": 1}}); w.Code != 403 {
		t.Error("a viewer changed it", w.Code)
	}
}

// Edits go beside the media only when writing there is allowed, and are kept by the server
// otherwise; export, import and pictures work either way.
func TestInfoEditWriteExportImport(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	ctx := context.Background()
	beside := strings.TrimSuffix(f.video.Path, ".mkv") + ".wittyinfo"
	u := f.url("/api/wittyinfo/item/%d", f.video)
	manifest := map[string]any{"format": "wittyinfo", "version": 1,
		"segments": []any{map[string]any{"type": "intro", "start": "1:00", "end": 95, "action": "auto"}},
		"scenes":   []any{map[string]any{"title": "Opening", "start": 0}, map[string]any{"title": "The chase", "start": 600, "thumbnailAt": 610}},
		"metadata": map[string]any{"title": "Film (Director's Cut)", "overview": "Longer."}}

	// Writing beside the media is off: kept by the server.
	if err := ts.DB.SetSetting(ctx, "saveMetadata", "0"); err != nil {
		t.Fatal(err)
	}
	w := ts.do(t, "PUT", u, f.admin, map[string]any{"info": manifest})
	a := decode[infoAnswer](t, w)
	if w.Code != 200 || a.Origin != "stored" || a.CanWrite || a.Info.Authorship.Author != "boss" || a.Info.Authorship.Tool.Name != "WittyMediaServer" ||
		a.Info.Identity == nil || a.Info.Identity.DurationSec != 1400 || a.Info.Identity.FileName != "Film.mkv" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(beside); err == nil {
		t.Fatal("written beside the media although that is off")
	}
	if it, _ := ts.Store.Item(ctx, f.video.ID); it.Title != "Film (Director's Cut)" || it.Overview != "Longer." {
		t.Errorf("metadata not taken in: %q %q", it.Title, it.Overview)
	}
	var pb playbackResponse
	ts.fillMarkers(ctx, f.video, &pb)
	if len(pb.Chapters) != 2 || pb.Chapters[1].Title != "The chase" || *pb.Chapters[1].ThumbnailAt != 610 {
		t.Errorf("scenes are the chapters: %+v", pb.Chapters)
	}

	// A picture.
	var pic bytes.Buffer
	png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 8, 12)))
	if w := ts.do(t, "PUT", u+"/images/poster", f.admin, pic.Bytes()); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	a = decode[infoAnswer](t, ts.do(t, "GET", u, f.admin, nil))
	if len(a.Files) != 1 || a.Files[0].Name != "images/poster.png" || a.Info.Artwork.First("poster") != "images/poster.png" {
		t.Fatalf("%+v", a.Files)
	}
	if w := ts.do(t, "GET", a.Files[0].URL, f.adult, nil); w.Code != 200 || !bytes.Equal(w.Body.Bytes(), pic.Bytes()) || w.Header().Get("Content-Type") != "image/png" {
		t.Errorf("picture: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	if it, _ := ts.Store.Item(ctx, f.video.ID); !strings.HasPrefix(it.Poster, "cache:wi-") {
		t.Errorf("poster not taken in: %q", it.Poster)
	}
	if w := ts.do(t, "PUT", u+"/images/poster", f.admin, []byte("<svg onload=alert(1)>")); w.Code != 400 {
		t.Errorf("not a picture: %d", w.Code)
	}
	if w := ts.do(t, "GET", u+"/files/manifest.json", f.adult, nil); w.Code != 404 {
		t.Errorf("only pictures are served: %d", w.Code)
	}
	// Editing the times keeps the picture.
	manifest["segments"] = []any{map[string]any{"type": "intro", "start": 61, "end": 96}}
	manifest["artwork"] = map[string]string{"poster": "images/poster.png"}
	if w := ts.do(t, "PUT", u, f.admin, map[string]any{"info": manifest}); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}

	// Export.
	w = ts.do(t, "GET", u+"/export", f.admin, nil)
	if w.Code != 200 || !wittyinfo.IsArchive(w.Body.Bytes()) || !strings.Contains(w.Header().Get("Content-Disposition"), "Film.wittyinfo") {
		t.Fatalf("export: %d %v", w.Code, w.Header())
	}
	exported := w.Body.Bytes()
	x, err := wittyinfo.Parse(exported)
	if err != nil || x.Segments[0].Start != 61 || len(x.Files["images/poster.png"]) == 0 {
		t.Fatalf("export: %v %+v", err, x)
	}

	// Writing is on: the next edit goes beside the media and the kept copy goes.
	if err := ts.DB.SetSetting(ctx, "saveMetadata", "1"); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "DELETE", u, f.admin, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if body, _, _ := ts.Store.StoredInfo(ctx, library.InfoTarget{Kind: library.InfoItem, ID: f.video.ID}); len(body) != 0 {
		t.Fatal("the kept copy wasn't forgotten")
	}
	w = ts.do(t, "POST", u+"/import", f.admin, exported)
	a = decode[infoAnswer](t, w)
	if w.Code != 200 || a.Origin != "file" || !a.CanWrite {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	b, err := os.ReadFile(beside)
	if err != nil {
		t.Fatal("not written beside the media:", err)
	}
	if y, err := wittyinfo.Parse(b); err != nil || y.Segments[0].Start != 61 || len(y.Files["images/poster.png"]) == 0 {
		t.Errorf("written file: %v", err)
	}
	// Turning writing off for .wittyinfo alone keeps the next edit in the server.
	if err := ts.DB.SetSetting(ctx, "saveWittyinfo", "0"); err != nil {
		t.Fatal(err)
	}
	manifest["segments"] = []any{map[string]any{"type": "intro", "start": 62, "end": 96}}
	a = decode[infoAnswer](t, ts.do(t, "PUT", u, f.admin, map[string]any{"info": manifest}))
	if a.Origin != "stored" || a.Info.Segments[0].Start != 62 {
		t.Errorf("%+v", a.Origin)
	}
	if b2, _ := os.ReadFile(beside); !bytes.Equal(b, b2) {
		t.Error("the file beside the media changed although writing .wittyinfo files is off")
	}
	// Removing the file there is refused while writing is off.
	if w := ts.do(t, "DELETE", u+"?file=1", f.admin, nil); w.Code != 409 {
		t.Errorf("%d", w.Code)
	}

	// Bad imports are refused with what is wrong.
	w = ts.do(t, "POST", u+"/import", f.admin, []byte(`{"format":"wittyinfo","version":1,"segments":[{"type":"intro","start":5}],"scope":"show"}`))
	if w.Code != 400 {
		t.Errorf("a show's file imported for a film: %d", w.Code)
	}
	w = ts.do(t, "PUT", u, f.admin, map[string]any{"info": map[string]any{"format": "wittyinfo", "version": 1, "segments": []any{map[string]any{"type": "intro", "start": 9, "end": 3}}}})
	if w.Code != 400 || !strings.Contains(w.Body.String(), "segments[0].end") {
		t.Errorf("%d %s", w.Code, w.Body.String())
	}
}

func TestInfoPaths(t *testing.T) {
	for _, c := range []struct {
		it   library.Item
		want string
	}{
		{library.Item{Kind: "video", Path: filepath.FromSlash("/m/Film (2023)/Film (2023).mkv")}, filepath.FromSlash("/m/Film (2023)/Film (2023).wittyinfo")},
		{library.Item{Kind: "dvd-folder", Path: filepath.FromSlash("/m/Disc")}, filepath.FromSlash("/m/Disc.wittyinfo")},
		{library.Item{Kind: "video", Path: "http://x/y.mkv"}, ""},
	} {
		got := library.InfoPaths(&c.it)
		if (len(got) == 0 && c.want != "") || (len(got) > 0 && got[0] != c.want) {
			t.Errorf("%s: %v", c.it.Path, got)
		}
	}
	if got := library.ShowInfoPaths(filepath.FromSlash("/tv/S"), 1, filepath.FromSlash("/tv/S/Season 1")); len(got) != 2 || filepath.Base(got[0]) != "season01.wittyinfo" {
		t.Errorf("%v", got)
	}
}
