package server

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
	"github.com/wittywilla/wittymediaserver/internal/watchxref"
)

var (
	testMP4  = append([]byte("\x00\x00\x00\x20ftypisom\x00\x00\x02\x00isomiso2mp41"), bytes.Repeat([]byte("v"), 200)...)
	testFLAC = append([]byte("fLaC\x00\x00\x00\x22"), bytes.Repeat([]byte("a"), 300)...)
)

func put(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

type extrasFixture struct {
	ts                    *testServer
	admin, adult, blocked string
	root                  string
	song, video           *library.Item
}

func newExtrasFixture(t *testing.T) *extrasFixture {
	t.Helper()
	ts := newTestServer(t)
	root := t.TempDir()
	music := ts.library(t, "music", filepath.Join(root, "music"))
	movies := ts.library(t, "movies", filepath.Join(root, "movies"))
	songPath := filepath.Join(root, "music", "Album", "01 Song.flac")
	put(t, songPath, testFLAC)
	videoPath := filepath.Join(root, "movies", "Film (2001)", "Film.mkv")
	put(t, videoPath, []byte("not really a film"))
	song := ts.item(t, &library.Item{LibraryID: music, Kind: "audio", Path: songPath, Title: "Song", VideoCodec: "flac", Container: "flac"})
	video := ts.item(t, &library.Item{LibraryID: movies, Kind: "video", Path: videoPath, Title: "Film", RuntimeSec: 1400, Width: 1920, Height: 1080, Provider: "tmdb", ProviderID: "603"})
	_, admin := ts.user(t, "boss", "adult", true, "")
	_, adult := ts.user(t, "alex", "adult", false, "")
	_, blocked := ts.user(t, "kept", "adult", false, noLibraryForYou)
	return &extrasFixture{ts: ts, admin: admin, adult: adult, blocked: blocked, root: root, song: song, video: video}
}

func (f *extrasFixture) url(format string, it *library.Item) string {
	return fmt.Sprintf(format, it.ID)
}

func TestCanvasFromSidecarWithRange(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	if c := decode[canvasInfo](t, ts.do(t, "GET", f.url("/api/media/%d/canvas", f.song), f.adult, nil)); c.Available || !c.Loop {
		t.Fatalf("nothing there yet: %+v", c)
	}
	put(t, filepath.Join(filepath.Dir(f.song.Path), "canvas.mp4"), testMP4)
	w := ts.do(t, "GET", f.url("/api/media/%d/canvas", f.song), f.adult, nil)
	c := decode[canvasInfo](t, w)
	if w.Code != 200 || !c.Available || c.MIME != "video/mp4" || c.Source != "sidecar" || !strings.HasPrefix(c.URL, f.url("/api/media/%d/canvas/file?v=", f.song)) {
		t.Fatalf("%d %+v", w.Code, c)
	}
	if alias := decode[canvasInfo](t, ts.do(t, "GET", f.url("/api/items/%d/canvas", f.song), f.adult, nil)); alias != c {
		t.Errorf("alias %+v", alias)
	}
	// The song's own beats the album's.
	put(t, filepath.Join(filepath.Dir(f.song.Path), "01 Song.canvas.webm"), append([]byte("\x1a\x45\xdf\xa3\x9f\x42\x82\x84webm"), bytes.Repeat([]byte("w"), 50)...))
	if c := decode[canvasInfo](t, ts.do(t, "GET", f.url("/api/media/%d/canvas", f.song), f.adult, nil)); c.MIME != "video/webm" {
		t.Errorf("own canvas: %+v", c)
	}
	os.Remove(filepath.Join(filepath.Dir(f.song.Path), "01 Song.canvas.webm"))

	full := ts.do(t, "GET", f.url("/api/media/%d/canvas/file", f.song), f.adult, nil)
	if full.Code != 200 || !bytes.Equal(full.Body.Bytes(), testMP4) || full.Header().Get("ETag") == "" || !strings.Contains(full.Header().Get("Cache-Control"), "max-age") {
		t.Fatalf("file: %d %v", full.Code, full.Header())
	}
	if ct := full.Header().Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("type %s", ct)
	}
	part := ts.do(t, "GET", f.url("/api/media/%d/canvas/file", f.song), f.adult, nil, "Range", "bytes=4-11")
	if part.Code != 206 || part.Body.String() != "ftypisom" || part.Header().Get("Content-Range") != fmt.Sprintf("bytes 4-11/%d", len(testMP4)) {
		t.Errorf("range: %d %q %s", part.Code, part.Body, part.Header().Get("Content-Range"))
	}
	if again := ts.do(t, "GET", f.url("/api/media/%d/canvas/file", f.song), f.adult, nil, "If-None-Match", full.Header().Get("ETag")); again.Code != 304 {
		t.Errorf("etag: %d", again.Code)
	}
	// A stream token for the item opens it; one for another item doesn't.
	tok := decode[map[string]any](t, ts.do(t, "POST", f.url("/api/items/%d/stream-token", f.song), f.adult, nil))["token"].(string)
	if w := ts.do(t, "GET", f.url("/api/media/%d/canvas/file", f.song)+"?st="+tok, "", nil); w.Code != 200 {
		t.Errorf("stream token: %d", w.Code)
	}
	other := decode[map[string]any](t, ts.do(t, "POST", f.url("/api/items/%d/stream-token", f.video), f.adult, nil))["token"].(string)
	if w := ts.do(t, "GET", f.url("/api/media/%d/canvas/file", f.song)+"?st="+other, "", nil); w.Code != 401 {
		t.Errorf("another item's token: %d", w.Code)
	}
	if w := ts.do(t, "GET", f.url("/api/media/%d/canvas", f.song)+"?st="+tok, "", nil); w.Code != 401 {
		t.Errorf("a stream token opened the listing: %d", w.Code)
	}
}

func TestCanvasLinkOutsideIsNotFollowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("links")
	}
	f := newExtrasFixture(t)
	outside := filepath.Join(t.TempDir(), "secret.mp4")
	put(t, outside, append([]byte("\x00\x00\x00\x20ftypisom"), []byte("TOP-SECRET")...))
	if err := os.Symlink(outside, filepath.Join(filepath.Dir(f.video.Path), "canvas.mp4")); err != nil {
		t.Fatal(err)
	}
	if c := decode[canvasInfo](t, f.ts.do(t, "GET", f.url("/api/media/%d/canvas", f.video), f.admin, nil)); c.Available {
		t.Errorf("a link out of the library: %+v", c)
	}
	if w := f.ts.do(t, "GET", f.url("/api/media/%d/canvas/file", f.video), f.admin, nil); w.Code != 404 || strings.Contains(w.Body.String(), "TOP-SECRET") {
		t.Errorf("%d", w.Code)
	}
}

func TestCanvasFromMetadataFolder(t *testing.T) {
	f := newExtrasFixture(t)
	put(t, filepath.Join(f.ts.metaFolderDir(), "Movies", fmt.Sprintf("Film (2001) [item-%d]-canvas.mp4", f.video.ID)), testMP4)
	c := decode[canvasInfo](t, f.ts.do(t, "GET", f.url("/api/media/%d/canvas", f.video), f.adult, nil))
	if !c.Available || c.Source != "metadata-folder" {
		t.Fatalf("%+v", c)
	}
	if w := f.ts.do(t, "GET", f.url("/api/media/%d/canvas/file", f.video), f.adult, nil); w.Code != 200 || !bytes.Equal(w.Body.Bytes(), testMP4) {
		t.Errorf("%d", w.Code)
	}
}

func TestCanvasUpload(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	u := f.url("/api/media/%d/canvas", f.video)
	if w := ts.do(t, "PUT", u, f.adult, testMP4); w.Code != 403 {
		t.Errorf("non-admin upload: %d", w.Code)
	}
	if w := ts.do(t, "PUT", u, f.admin, []byte("<html><script>alert(1)</script></html>")); w.Code != 415 {
		t.Errorf("html: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", u, f.admin, append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 100)...)); w.Code != 415 {
		t.Errorf("png: %d", w.Code)
	}
	big := append(append([]byte{}, testMP4...), make([]byte, mediaextras.MaxCanvas)...)
	if w := ts.do(t, "PUT", u, f.admin, big); w.Code != 413 {
		t.Errorf("too big: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", u, f.admin, testMP4); w.Code != 200 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	c := decode[canvasInfo](t, ts.do(t, "GET", u, f.adult, nil))
	if !c.Available || c.Source != "upload" {
		t.Fatalf("%+v", c)
	}
	// Kept in the server's folder, and nothing was written beside the media.
	entries, _ := os.ReadDir(filepath.Dir(f.video.Path))
	if len(entries) != 1 {
		t.Errorf("written beside the media: %v", entries)
	}
	if !strings.HasPrefix(mediaextras.FirstPlain(ts.extrasRoot(), mediaextras.Key(f.video.ID, f.video.Path)+"/canvas", mediaextras.CanvasExts), ts.Cfg.ImagesDir) {
		t.Error("not in the images folder")
	}
	if w := ts.do(t, "GET", f.url("/api/media/%d/canvas/file", f.video), f.adult, nil, "Range", "bytes=0-3"); w.Code != 206 || w.Body.Len() != 4 {
		t.Errorf("range on an upload: %d", w.Code)
	}
	if w := ts.do(t, "DELETE", u, f.adult, nil); w.Code != 403 {
		t.Errorf("non-admin delete: %d", w.Code)
	}
	if w := ts.do(t, "DELETE", u, f.admin, nil); w.Code != 200 {
		t.Errorf("delete: %d", w.Code)
	}
	if w := ts.do(t, "DELETE", u, f.admin, nil); w.Code != 404 {
		t.Errorf("delete again: %d", w.Code)
	}
	if c := decode[canvasInfo](t, ts.do(t, "GET", u, f.adult, nil)); c.Available {
		t.Errorf("still there: %+v", c)
	}
	// An upload can't go to something a canvas doesn't suit.
	book := ts.item(t, &library.Item{LibraryID: f.video.LibraryID, Kind: "book", Path: filepath.Join(f.root, "movies", "b.pdf")})
	if w := ts.do(t, "PUT", f.url("/api/media/%d/canvas", book), f.admin, testMP4); w.Code != 400 {
		t.Errorf("book: %d", w.Code)
	}
}

func TestMediaExtrasRespectLimits(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	put(t, filepath.Join(filepath.Dir(f.song.Path), "canvas.mp4"), testMP4)
	put(t, filepath.Join(filepath.Dir(f.song.Path), "01 Song.vocals.flac"), testFLAC)
	for _, p := range []string{
		f.url("/api/media/%d/canvas", f.song), f.url("/api/items/%d/canvas", f.song), f.url("/api/media/%d/canvas/file", f.song),
		f.url("/api/media/%d/stems", f.song), f.url("/api/media/%d/stems/vocals", f.song),
		f.url("/api/media/%d/markers", f.video), f.url("/api/media/%d/where-to-watch", f.video),
	} {
		w := ts.do(t, "GET", p, f.blocked, nil)
		if w.Code != 404 && w.Code != 403 {
			t.Errorf("%s for an account without the library: %d", p, w.Code)
		}
		if strings.Contains(w.Body.String(), "fLaC") || strings.Contains(w.Body.String(), "ftyp") {
			t.Errorf("%s leaked the file", p)
		}
		if w := ts.do(t, "GET", p, "", nil); w.Code != 401 {
			t.Errorf("%s signed out: %d", p, w.Code)
		}
	}
	if w := ts.do(t, "GET", "/api/media/999999/canvas", f.admin, nil); w.Code != 404 {
		t.Errorf("no such item: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/media/abc/canvas", f.admin, nil); w.Code != 400 {
		t.Errorf("bad id: %d", w.Code)
	}
}

func TestStems(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	dir := filepath.Dir(f.song.Path)
	put(t, filepath.Join(dir, "01 Song.vocals.flac"), testFLAC)
	put(t, filepath.Join(dir, "01 Song.stems", "accompaniment.flac"), testFLAC)
	put(t, filepath.Join(dir, "01 Song.stem.mp4"), testMP4)
	type list struct {
		Stems []stemView `json:"stems"`
		Names []string   `json:"names"`
	}
	got := decode[list](t, ts.do(t, "GET", f.url("/api/media/%d/stems", f.song), f.adult, nil))
	if len(got.Stems) != 3 || got.Stems[0].Name != "vocals" || got.Stems[1].Name != "instrumental" || !got.Stems[2].Multitrack || len(got.Names) != 5 {
		t.Fatalf("%+v", got)
	}
	if got.Stems[0].URL != f.url("/api/media/%d/stems/vocals", f.song) || got.Stems[0].MIME != "audio/flac" || got.Stems[0].Size != int64(len(testFLAC)) {
		t.Errorf("%+v", got.Stems[0])
	}
	w := ts.do(t, "GET", f.url("/api/media/%d/stems/accompaniment", f.song), f.adult, nil, "Range", "bytes=0-3")
	if w.Code != 206 || w.Body.String() != "fLaC" {
		t.Errorf("range: %d %q", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", f.url("/api/media/%d/stems/drums", f.song), f.adult, nil); w.Code != 404 {
		t.Errorf("missing stem: %d", w.Code)
	}
	// Names that try to lead elsewhere. A bare ".." is cleaned away by the router, which
	// answers with a redirect to the listing and never reaches a stem at all.
	refused := func(code int) bool { return code == 404 || code == 400 || code == 307 }
	for _, bad := range []string{"..", "%2e%2e", "..%2Fsecret", "vocals.flac", "piano", "%2F..%2F..%2Fetc%2Fpasswd", "vocals%00", "..%5C..%5Cwin.ini"} {
		if w := ts.do(t, "GET", f.url("/api/media/%d/stems/", f.song)+bad, f.admin, nil); !refused(w.Code) || strings.Contains(w.Body.String(), "fLaC") {
			t.Errorf("GET %q: %d", bad, w.Code)
		}
		if w := ts.do(t, "PUT", f.url("/api/media/%d/stems/", f.song)+bad, f.admin, testFLAC); !refused(w.Code) {
			t.Errorf("PUT %q: %d", bad, w.Code)
		}
		if w := ts.do(t, "DELETE", f.url("/api/media/%d/stems/", f.song)+bad, f.admin, nil); !refused(w.Code) {
			t.Errorf("DELETE %q: %d", bad, w.Code)
		}
	}
	if entries, _ := os.ReadDir(ts.extrasRoot()); len(entries) != 0 {
		t.Errorf("a refused name left something: %v", entries)
	}
	// Uploads: a drum part, wrong kinds refused, and the upload is listed first.
	if w := ts.do(t, "PUT", f.url("/api/media/%d/stems/drums", f.song), f.adult, testFLAC); w.Code != 403 {
		t.Errorf("non-admin: %d", w.Code)
	}
	if w := ts.do(t, "PUT", f.url("/api/media/%d/stems/drums", f.song), f.admin, []byte("#!/bin/sh\nrm -rf /\n")); w.Code != 415 {
		t.Errorf("script: %d", w.Code)
	}
	if w := ts.do(t, "PUT", f.url("/api/media/%d/stems/multitrack", f.song), f.admin, testFLAC); w.Code != 415 {
		t.Errorf("flac as multitrack: %d", w.Code)
	}
	if w := ts.do(t, "PUT", f.url("/api/media/%d/stems/drums", f.song), f.admin, testFLAC); w.Code != 200 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", f.url("/api/media/%d/stems/vocals", f.video), f.admin, testFLAC); w.Code != 400 {
		t.Errorf("stems for a film: %d", w.Code)
	}
	got = decode[list](t, ts.do(t, "GET", f.url("/api/media/%d/stems", f.song), f.adult, nil))
	if len(got.Stems) != 4 || got.Stems[2].Name != "drums" || got.Stems[2].Source != "upload" {
		t.Errorf("after upload: %+v", got.Stems)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 4 {
		t.Errorf("written beside the song: %v", entries)
	}
	if w := ts.do(t, "DELETE", f.url("/api/media/%d/stems/drums", f.song), f.admin, nil); w.Code != 200 {
		t.Errorf("delete: %d", w.Code)
	}
	if w := ts.do(t, "DELETE", f.url("/api/media/%d/stems/vocals", f.song), f.admin, nil); w.Code != 404 {
		t.Errorf("a sidecar isn't deleted: %d", w.Code)
	}
	// The player gets them with its stream token.
	pb := decode[playbackResponse](t, ts.do(t, "POST", f.url("/api/items/%d/playback", f.song), f.adult, map[string]any{"profile": "web"}))
	if len(pb.Stems) != 3 || !strings.Contains(pb.Stems[0].URL, "st="+pb.StreamToken) {
		t.Errorf("playback stems: %+v", pb.Stems)
	}
	if w := ts.do(t, "GET", pb.Stems[0].URL, "", nil); w.Code != 200 {
		t.Errorf("stem by stream token: %d", w.Code)
	}
}

func TestMarkers(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	ctx := context.Background()
	u := f.url("/api/media/%d/markers", f.video)
	m := decode[mediaextras.Markers](t, ts.do(t, "GET", u, f.adult, nil))
	if !m.Empty() || m.Source != mediaextras.SourceNone || m.Commercials == nil {
		t.Fatalf("nothing: %+v", m)
	}
	// What a scan read from chapters.
	if err := ts.Store.SetManualMarkers(ctx, f.video.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.DB.ExecContext(ctx, `UPDATE media_markers SET scanned = ? WHERE item_id = ?`, `{"intro":{"start":30,"end":90},"credits":{"start":1300,"end":1400}}`, f.video.ID); err != nil {
		t.Fatal(err)
	}
	m = decode[mediaextras.Markers](t, ts.do(t, "GET", u, f.adult, nil))
	if m.Intro.StartSec != 30 || m.Credits.StartSec != 1300 || m.Source != mediaextras.SourceChapters {
		t.Fatalf("chapters: %+v", m)
	}
	// An EDL beside the film adds adverts and a recap, and its intro wins over the chapters'.
	put(t, strings.TrimSuffix(f.video.Path, ".mkv")+".edl", []byte("0 45 3 recap\n35 95 3 intro\n600 700 3\n800 900 0\n"))
	m = decode[mediaextras.Markers](t, ts.do(t, "GET", u, f.adult, nil))
	if m.Recap == nil || m.Recap.EndSec != 45 || m.Intro.StartSec != 35 || len(m.Commercials) != 2 || m.Source != mediaextras.SourceEDL || m.Sources["credits"] != mediaextras.SourceChapters {
		t.Fatalf("edl: %+v", m)
	}
	// A skip.json beats the EDL.
	put(t, f.video.Path+".skip.json", []byte(`{"intro":{"start":40,"end":100},"credits":{"start":1320}}`))
	m = decode[mediaextras.Markers](t, ts.do(t, "GET", u, f.adult, nil))
	if m.Intro.StartSec != 40 || m.Credits.StartSec != 1320 || m.Credits.EndSec != 1400 || m.Source != mediaextras.SourceSkipJSON {
		t.Fatalf("skip.json: %+v", m)
	}
	// An administrator's edit beats everything, and a scan never replaces it.
	if w := ts.do(t, "PUT", u, f.adult, map[string]any{"intro": map[string]any{"start": 1, "end": 2}}); w.Code != 403 {
		t.Errorf("non-admin: %d", w.Code)
	}
	for _, bad := range []any{
		map[string]any{"intro": map[string]any{"start": 50, "end": 10}},
		map[string]any{"credits": map[string]any{"start": -5}},
		map[string]any{"commercials": []any{map[string]any{"start": 5, "end": 5}}},
	} {
		if w := ts.do(t, "PUT", u, f.admin, bad); w.Code != 400 {
			t.Errorf("%v: %d", bad, w.Code)
		}
	}
	w := ts.do(t, "PUT", u, f.admin, map[string]any{"intro": map[string]any{"start": 50, "end": 110}, "recap": map[string]any{"start": 0, "end": 49}})
	m = decode[mediaextras.Markers](t, w)
	if w.Code != 200 || m.Intro.StartSec != 50 || m.Recap.EndSec != 49 || m.Source != mediaextras.SourceManual || m.Sources["credits"] != mediaextras.SourceSkipJSON {
		t.Fatalf("manual: %d %+v", w.Code, m)
	}
	if err := ts.Store.SaveScannedMarkers(ctx, f.video.ID, &mediaextras.Markers{Intro: &mediaextras.Range{StartSec: 7, EndSec: 8}}); err != nil {
		t.Fatal(err)
	}
	if m := decode[mediaextras.Markers](t, ts.do(t, "GET", u, f.adult, nil)); m.Intro.StartSec != 50 {
		t.Errorf("a scan replaced a manual edit: %+v", m.Intro)
	}
	if st := ts.Store.Markers(ctx, f.video.ID); st.Manual == nil || st.Scanned == nil || st.Scanned.Intro.StartSec != 7 {
		t.Errorf("stored %+v", st)
	}
	// The player gets them beside the intro.
	pb := playbackResponse{}
	ts.fillMarkers(ctx, f.video, &pb)
	if pb.Intro == nil || pb.Intro.StartSec != 50 || pb.Recap == nil || pb.Credits == nil || pb.Credits.StartSec != 1320 || len(pb.Commercials) != 2 {
		t.Errorf("playback: %+v %+v %+v %+v", pb.Intro, pb.Recap, pb.Credits, pb.Commercials)
	}
	// Reset goes back to the files.
	m = decode[mediaextras.Markers](t, ts.do(t, "PUT", u, f.admin, map[string]any{"reset": true}))
	if m.Intro.StartSec != 40 || m.Source != mediaextras.SourceSkipJSON {
		t.Errorf("reset: %+v", m)
	}
	// A damaged skip.json is ignored, and the EDL counts again.
	put(t, f.video.Path+".skip.json", []byte(`{"intro":{"start":100,"end":5}}`))
	if m := decode[mediaextras.Markers](t, ts.do(t, "GET", u, f.adult, nil)); m.Source != mediaextras.SourceEDL {
		t.Errorf("damaged skip.json: %+v", m)
	}
}

func TestShowIntroIsAMarker(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	ctx := context.Background()
	tv := ts.library(t, "shows", filepath.Join(f.root, "tv"))
	start, end := 5.0, 65.0
	sh := &library.Show{LibraryID: tv, Title: "A Show", SortTitle: "A Show", Path: filepath.Join(f.root, "tv", "A Show"), IntroStart: &start, IntroEnd: &end}
	if err := ts.Store.UpsertShow(ctx, sh); err != nil {
		t.Fatal(err)
	}
	one := 1
	ep := ts.item(t, &library.Item{LibraryID: tv, Kind: "video", Path: filepath.Join(f.root, "tv", "A Show", "S01E01.mkv"), ShowID: &sh.ID, Season: &one, Episode: &one})
	m := decode[mediaextras.Markers](t, ts.do(t, "GET", f.url("/api/media/%d/markers", ep), f.adult, nil))
	if m.Intro == nil || m.Intro.EndSec != 65 || m.Source != mediaextras.SourceShow {
		t.Errorf("%+v", m)
	}
}

type fakeWatch struct{ calls int }

func (*fakeWatch) Name() string              { return "fake" }
func (*fakeWatch) Attribution() string       { return "fake data" }
func (*fakeWatch) Available() (bool, string) { return true, "" }
func (f *fakeWatch) Offers(_ context.Context, ref watchxref.Ref, region string) (*watchxref.Offers, error) {
	f.calls++
	return &watchxref.Offers{Link: "https://www.themoviedb.org/movie/" + ref.ID + "/watch?locale=" + region, Offers: []watchxref.Offer{
		{Key: "streamer", Name: "Streamer", Type: watchxref.TypeStream, Priority: 1},
		{Key: "shop", Name: "Shop", Type: watchxref.TypeBuy, Priority: 2},
		{Key: "shop", Name: "Shop", Type: watchxref.TypeRent, Priority: 2},
	}}, nil
}

func TestWhereToWatch(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	// Without a TMDB key it says so, and still says what's here.
	w := ts.do(t, "GET", f.url("/api/media/%d/where-to-watch", f.video), f.adult, nil)
	a := decode[watchxref.Answer](t, w)
	if w.Code != 200 || !a.InLibrary || a.Reason != watchxref.ReasonNoAPIKey || len(a.Providers) != 0 || a.Region != "US" || a.Local.Quality != "1080p" {
		t.Fatalf("no key: %d %+v", w.Code, a)
	}
	if w := ts.do(t, "GET", "/api/discover/where-to-watch?tmdb=603&type=movie", f.adult, nil); decode[watchxref.Answer](t, w).Reason != watchxref.ReasonNoAPIKey {
		t.Errorf("discover without a key: %s", w.Body)
	}

	fake := &fakeWatch{}
	watchProvidersFor = func(*Server) []watchxref.Provider { return []watchxref.Provider{fake} }
	t.Cleanup(func() { watchProvidersFor = nil })

	a = decode[watchxref.Answer](t, ts.do(t, "GET", f.url("/api/media/%d/where-to-watch", f.video)+"?region=gb", f.adult, nil))
	if a.Region != "GB" || a.Source != "fake" || len(a.Groups) != 3 || a.Groups[0].Type != "stream" || a.TMDB == nil || a.TMDB.ID != "603" || a.ItemID == nil || *a.ItemID != f.video.ID {
		t.Fatalf("%+v", a)
	}
	a = decode[watchxref.Answer](t, ts.do(t, "GET", f.url("/api/media/%d/where-to-watch", f.video)+"?region=gb", f.adult, nil))
	if !a.Cached || fake.calls != 1 {
		t.Errorf("cache: %v %d", a.Cached, fake.calls)
	}
	// Checking the listings again forgets the kept answers.
	ts.Scanner.RecheckListings()
	ts.do(t, "GET", f.url("/api/media/%d/where-to-watch", f.video)+"?region=gb", f.adult, nil)
	if fake.calls != 2 {
		t.Errorf("after a recheck: %d", fake.calls)
	}

	// Discover: in the library, and not.
	a = decode[watchxref.Answer](t, ts.do(t, "GET", "/api/discover/where-to-watch?tmdb=603&type=movie", f.adult, nil))
	if !a.InLibrary || a.ItemID == nil || *a.ItemID != f.video.ID || len(a.Providers) != 3 {
		t.Errorf("discover here: %+v", a)
	}
	a = decode[watchxref.Answer](t, ts.do(t, "GET", "/api/discover/where-to-watch?tmdb=999&type=movie", f.adult, nil))
	if a.InLibrary || a.ItemID != nil || len(a.Providers) != 3 {
		t.Errorf("discover elsewhere: %+v", a)
	}
	// Someone who can't see the film isn't told it's here.
	a = decode[watchxref.Answer](t, ts.do(t, "GET", "/api/discover/where-to-watch?tmdb=603&type=movie", f.blocked, nil))
	if a.InLibrary || a.ItemID != nil {
		t.Errorf("leaked to a limited account: %+v", a)
	}
	for _, bad := range []string{"tmdb=abc", "tmdb=1&type=game", "tmdb=", "tmdb=12345678901"} {
		if w := ts.do(t, "GET", "/api/discover/where-to-watch?"+bad, f.adult, nil); w.Code != 400 {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
}

func TestMediaUploadPath(t *testing.T) {
	for p, want := range map[string]bool{
		"/api/media/5/canvas": true, "/api/v1/media/5/canvas": true, "/api/media/5/stems/vocals": true,
		"/api/media/5/markers": false, "/api/items/5/canvas": false, "/api/media/5/canvas/file": false,
	} {
		if got := mediaUploadPath(p); got != want {
			t.Errorf("%s: %v", p, got)
		}
	}
}
