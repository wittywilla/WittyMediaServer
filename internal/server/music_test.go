package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/lyrics"
)

// A music library on a real folder: one song file with whatever sits beside it.
func musicSong(t *testing.T, ts *testServer, files map[string]string) (*library.Item, string) {
	t.Helper()
	dir := t.TempDir()
	song := filepath.Join(dir, "01 A Made Up Song.mp3")
	if err := os.WriteFile(song, []byte("not really sound"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lib := ts.library(t, "music", dir)
	it := ts.item(t, &library.Item{LibraryID: lib, Kind: "audio", Path: song, Title: "A Made Up Song", Artist: "An Artist", AlbumArtist: "An Artist", Album: "An Album", RuntimeSec: 200, Genres: []string{"Indie"}})
	return it, dir
}

func decodeLyrics(t *testing.T, w *httptest.ResponseRecorder) lyricsReply {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("code %d: %s", w.Code, w.Body)
	}
	var out lyricsReply
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// A stand-in for the public service, counting what it's asked.
func fakeLRCLIB(t *testing.T, handle http.HandlerFunc) *atomic.Int32 {
	t.Helper()
	var asked atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		handle(w, r)
	}))
	t.Cleanup(srv.Close)
	savedBase, savedClient := lyricsPublicBase, library.SafeClient
	lyricsPublicBase, library.SafeClient = srv.URL, srv.Client()
	t.Cleanup(func() { lyricsPublicBase, library.SafeClient = savedBase, savedClient })
	return &asked
}

func TestLyricsSidecarComesFirstAndNeverAsksOnline(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	it, _ := musicSong(t, ts, map[string]string{"01 A Made Up Song.LRC": "[00:01.00]First\n[00:02.50]Second\n"})
	asked := fakeLRCLIB(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) })
	ts.do(t, "PUT", "/api/settings", admin, map[string]string{"lyricsOnline": "1"})
	got := decodeLyrics(t, ts.do(t, "GET", fmt.Sprintf("/api/music/lyrics/%d?fetch=1", it.ID), admin, nil))
	if got.Source != "sidecar" || !got.Synced || len(got.Lines) != 2 || got.Lines[1].T != 2500 {
		t.Fatalf("%+v", got)
	}
	if asked.Load() != 0 {
		t.Fatalf("asked online %d times with lyrics beside the song", asked.Load())
	}
}

func TestLyricsOnlineIsOffUntilTurnedOn(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	it, _ := musicSong(t, ts, nil)
	var query string
	asked := fakeLRCLIB(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"id":7,"trackName":"A Made Up Song","artistName":"An Artist","duration":200,"syncedLyrics":"[00:03.00]From online","plainLyrics":"From online"}`))
	})
	url := fmt.Sprintf("/api/music/lyrics/%d?fetch=1", it.ID)
	got := decodeLyrics(t, ts.do(t, "GET", url, admin, nil))
	if got.Online || got.Source != "" || asked.Load() != 0 {
		t.Fatalf("online before anyone agreed: %+v, asked %d", got, asked.Load())
	}
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"lyricsOnline": "1"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got = decodeLyrics(t, ts.do(t, "GET", url, admin, nil))
	if got.Source != "lrclib" || got.SourceName != "LRCLIB" || got.CreditURL == "" || got.Lines[0].Text != "From online" {
		t.Fatalf("%+v", got)
	}
	for _, field := range []string{"artist_name=An+Artist", "track_name=A+Made+Up+Song", "album_name=An+Album", "duration=200"} {
		if !strings.Contains(query, field) {
			t.Errorf("%s not sent: %s", field, query)
		}
	}
	if strings.Contains(query, "mp3") || strings.Contains(query, "owner") {
		t.Errorf("sent more than it should: %s", query)
	}
	// Asked once: the second time comes from the cache, which is not in the data folder.
	decodeLyrics(t, ts.do(t, "GET", url, admin, nil))
	if asked.Load() != 1 {
		t.Fatalf("asked %d times", asked.Load())
	}
	if !strings.HasPrefix(ts.lyricsCachePath(it.ID), ts.Cfg.CacheDir) {
		t.Fatalf("kept at %s", ts.lyricsCachePath(it.ID))
	}
}

func TestLyricsOnlyAsksWhenOpened(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	it, _ := musicSong(t, ts, nil)
	asked := fakeLRCLIB(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) })
	ts.do(t, "PUT", "/api/settings", admin, map[string]string{"lyricsOnline": "1"})
	decodeLyrics(t, ts.do(t, "GET", fmt.Sprintf("/api/music/lyrics/%d", it.ID), admin, nil))
	if asked.Load() != 0 {
		t.Fatal("asked online without fetch=1")
	}
}

func TestLyricsNothingFoundIsRemembered(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	it, _ := musicSong(t, ts, nil)
	asked := fakeLRCLIB(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/search" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(404)
	})
	ts.do(t, "PUT", "/api/settings", admin, map[string]string{"lyricsOnline": "1"})
	url := fmt.Sprintf("/api/music/lyrics/%d?fetch=1", it.ID)
	got := decodeLyrics(t, ts.do(t, "GET", url, admin, nil))
	if !got.Tried || got.Source != "" {
		t.Fatalf("%+v", got)
	}
	n := asked.Load()
	decodeLyrics(t, ts.do(t, "GET", url, admin, nil))
	if asked.Load() != n {
		t.Fatal("asked again straight away")
	}
}

func TestLyricsWhoMayDoWhat(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, person := ts.user(t, "person", "adult", false, "")
	it, _ := musicSong(t, ts, nil)
	base := fmt.Sprintf("/api/music/lyrics/%d", it.ID)
	lrc := "[00:01.00]Corrected\n"
	if w := ts.do(t, "GET", base, "", nil); w.Code != 401 {
		t.Errorf("no sign-in: %d", w.Code)
	}
	for _, c := range []struct {
		method, path string
		body         any
	}{{"PUT", base, map[string]any{"text": lrc}}, {"DELETE", base, nil}, {"POST", base + "/search", map[string]string{"q": "x"}}, {"DELETE", "/api/music/lyrics-cache", nil}} {
		if w := ts.do(t, c.method, c.path, person, c.body); w.Code != 403 {
			t.Errorf("%s %s by a person: %d", c.method, c.path, w.Code)
		}
	}
	if w := ts.do(t, "PUT", base, admin, map[string]any{"text": lrc}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got := decodeLyrics(t, ts.do(t, "GET", base, person, nil))
	if got.Source != "override" || got.Lines[0].Text != "Corrected" || got.CanEdit {
		t.Fatalf("%+v", got)
	}
	// Each person's offset is their own.
	if w := ts.do(t, "PUT", base+"/offset", person, map[string]int{"offsetMs": 750}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if decodeLyrics(t, ts.do(t, "GET", base, person, nil)).OffsetMS != 750 || decodeLyrics(t, ts.do(t, "GET", base, admin, nil)).OffsetMS != 0 {
		t.Fatal("offset wasn't kept per person")
	}
	if w := ts.do(t, "PUT", base+"/offset", person, map[string]int{"offsetMs": 999999}); w.Code != 400 {
		t.Errorf("wild offset: %d", w.Code)
	}
	if w := ts.do(t, "PUT", base, admin, map[string]any{"text": "   "}); w.Code != 400 {
		t.Errorf("empty text: %d", w.Code)
	}
	if w := ts.do(t, "PUT", base, admin, map[string]any{"text": strings.Repeat("x", lyrics.MaxBytes*3)}); w.Code != 400 {
		t.Errorf("huge text: %d", w.Code)
	}
	if w := ts.do(t, "DELETE", base, admin, nil); w.Code != 204 {
		t.Fatalf("%d", w.Code)
	}
	if decodeLyrics(t, ts.do(t, "GET", base, admin, nil)).Source != "" {
		t.Fatal("correction wasn't forgotten")
	}
}

func TestLyricsLimitedProfileCantReachHiddenSong(t *testing.T) {
	ts := newTestServer(t)
	it, _ := musicSong(t, ts, map[string]string{"01 A Made Up Song.lrc": "[00:01.00]Secret"})
	_, admin := ts.user(t, "owner", "adult", true, "")
	other := ts.library(t, "movies", t.TempDir())
	restrict := fmt.Sprintf(`{"maxAge":-1,"allowUnrated":true,"libraries":[%d]}`, other)
	_, kid := ts.user(t, "kid", "adult", false, restrict)
	if w := ts.do(t, "GET", fmt.Sprintf("/api/music/lyrics/%d", it.ID), kid, nil); w.Code != 404 {
		t.Fatalf("code %d: %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/music/lyrics/%d", it.ID), admin, nil); w.Code != 200 {
		t.Fatalf("code %d", w.Code)
	}
}

func TestMusicExtrasStayInTheFolder(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	it, dir := musicSong(t, ts, map[string]string{"Booklet.pdf": "%PDF-1.4", "Notes.txt": "Liner notes", "01 A Made Up Song.txt": "words of the song", "cover.jpg": "x", "rip.log": "x"})
	outside := filepath.Join(filepath.Dir(dir), "secret.txt")
	_ = os.WriteFile(outside, []byte("secret"), 0o644)
	defer os.Remove(outside)
	w := ts.do(t, "GET", fmt.Sprintf("/api/music/extras/%d", it.ID), admin, nil)
	var list []musicExtra
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, e := range list {
		names[e.Name] = e.Kind
	}
	if names["Booklet.pdf"] != "booklet" || names["Notes.txt"] != "text" || len(names) != 2 {
		t.Fatalf("%v", names)
	}
	w = ts.do(t, "GET", fmt.Sprintf("/api/music/extras/%d/file?name=Notes.txt", it.ID), admin, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Liner notes") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, name := range []string{"../secret.txt", "..%2Fsecret.txt", "cover.jpg", "rip.log", "01 A Made Up Song.mp3"} {
		if w := ts.do(t, "GET", fmt.Sprintf("/api/music/extras/%d/file?name=%s", it.ID, strings.ReplaceAll(name, " ", "%20")), admin, nil); w.Code != 404 {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
}

func TestMusicHomeSongsAndRadio(t *testing.T) {
	ts := newTestServer(t)
	u, admin := ts.user(t, "owner", "adult", true, "")
	it, dir := musicSong(t, ts, nil)
	for i, title := range []string{"Second Song", "Third Song"} {
		ts.item(t, &library.Item{LibraryID: it.LibraryID, Kind: "audio", Path: filepath.Join(dir, title+".mp3"), Title: title, Artist: "An Artist", AlbumArtist: "An Artist", Album: "An Album", Track: new(int), RuntimeSec: float64(100 + i), Genres: []string{"Indie"}})
	}
	if w := ts.do(t, "PUT", fmt.Sprintf("/api/items/%d/reaction", it.ID), admin, map[string]int{"value": 1}); w.Code >= 300 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w := ts.do(t, "GET", fmt.Sprintf("/api/music/home?library=%d", it.LibraryID), admin, nil)
	var home musicHome
	if err := json.Unmarshal(w.Body.Bytes(), &home); err != nil {
		t.Fatalf("%v: %s", err, w.Body)
	}
	if home.Counts["songs"] != 3 || home.Counts["albums"] != 1 || home.Counts["artists"] != 1 || home.Counts["liked"] != 1 || len(home.Hero) != 1 || home.Hero[0].Genre != "Indie" || len(home.Songs) != 3 {
		t.Fatalf("%+v", home)
	}
	w = ts.do(t, "GET", fmt.Sprintf("/api/music/songs?library=%d&view=liked", it.LibraryID), admin, nil)
	var liked []library.Item
	_ = json.Unmarshal(w.Body.Bytes(), &liked)
	if len(liked) != 1 || liked[0].ID != it.ID {
		t.Fatalf("%s", w.Body)
	}
	w = ts.do(t, "GET", fmt.Sprintf("/api/music/likes?library=%d", it.LibraryID), admin, nil)
	if strings.TrimSpace(w.Body.String()) != fmt.Sprintf("[%d]", it.ID) {
		t.Fatalf("%s", w.Body)
	}
	w = ts.do(t, "GET", fmt.Sprintf("/api/music/radio/%d", it.ID), admin, nil)
	var radio []library.Item
	_ = json.Unmarshal(w.Body.Bytes(), &radio)
	if len(radio) != 3 || radio[0].ID != it.ID {
		t.Fatalf("%s", w.Body)
	}
	// A song someone disliked stays off their station.
	second := radio[1].ID
	ts.do(t, "PUT", fmt.Sprintf("/api/items/%d/reaction", second), admin, map[string]int{"value": -1})
	w = ts.do(t, "GET", fmt.Sprintf("/api/music/radio/%d", it.ID), admin, nil)
	if strings.Contains(w.Body.String(), fmt.Sprintf(`"id":%d,`, second)) {
		t.Fatalf("disliked song on the station: %s", w.Body)
	}
	// Played songs come first in Most played.
	if _, err := ts.DB.Exec(`INSERT INTO item_stats(user_id, item_id, plays, seconds, last_at) VALUES(?,?,3,300,1)`, u.ID, second); err != nil {
		t.Fatal(err)
	}
	w = ts.do(t, "GET", fmt.Sprintf("/api/music/songs?library=%d&view=played", it.LibraryID), admin, nil)
	var played []library.Item
	_ = json.Unmarshal(w.Body.Bytes(), &played)
	if len(played) != 1 || played[0].ID != second {
		t.Fatalf("%s", w.Body)
	}
	// Plain lists: by genre, and by artist with no match.
	if w := ts.do(t, "GET", fmt.Sprintf("/api/music/albums?library=%d&genre=Rock", it.LibraryID), admin, nil); strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("%s", w.Body)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/music/artist?library=%d&name=Nobody", it.LibraryID), admin, nil); w.Code != 404 {
		t.Fatalf("%d", w.Code)
	}
	w = ts.do(t, "GET", fmt.Sprintf("/api/music/artist?library=%d&name=An%%20Artist", it.LibraryID), admin, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"online":false`) || strings.Contains(w.Body.String(), `"bio"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestArtistPhotoPrefersTheFolderThenACover(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	it, dir := musicSong(t, ts, map[string]string{"artist.jpg": "\xff\xd8\xff fake"})
	w := ts.do(t, "GET", fmt.Sprintf("/api/music/artist-photo?library=%d&name=An%%20Artist", it.LibraryID), admin, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "fake") {
		t.Fatalf("%d", w.Code)
	}
	if err := os.Remove(filepath.Join(dir, "artist.jpg")); err != nil {
		t.Fatal(err)
	}
	w = ts.do(t, "GET", fmt.Sprintf("/api/music/artist-photo?library=%d&name=An%%20Artist", it.LibraryID), admin, nil)
	if w.Code != 404 { // this song has no cover either
		t.Fatalf("%d", w.Code)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/music/artist-photo?library=%d&name=%s", it.LibraryID, strings.Repeat("x", 400)), admin, nil); w.Code != 400 {
		t.Fatalf("%d", w.Code)
	}
}

func TestSettingsCheckTheLyricsMirrorAndMaskTheKey(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	for _, bad := range []string{"file:///etc/passwd", "gopher://x", "http://user:pass@host/", "nonsense"} {
		if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"lyricsMirror": bad}); w.Code != 400 {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"lyricsMirror": "http://192.168.1.20:3300", "lyricsMusixmatchKey": "secret-key"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w := ts.do(t, "GET", "/api/settings", admin, nil)
	if strings.Contains(w.Body.String(), "secret-key") || !strings.Contains(w.Body.String(), "192.168.1.20") {
		t.Fatalf("%s", w.Body)
	}
}

// A source that can't be reached is left alone for a while: with one look-up at a time for
// the whole server, every song would otherwise wait on it in turn.
func TestLyricsSourceThatsDownIsLeftAlone(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	first, _ := musicSong(t, ts, nil)
	second, _ := musicSong(t, ts, nil)
	asked := fakeLRCLIB(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) })
	ts.do(t, "PUT", "/api/settings", admin, map[string]string{"lyricsOnline": "1"})
	got := decodeLyrics(t, ts.do(t, "GET", fmt.Sprintf("/api/music/lyrics/%d?fetch=1", first.ID), admin, nil))
	if got.Failed == "" || asked.Load() != 1 {
		t.Fatalf("%+v, asked %d", got, asked.Load())
	}
	got = decodeLyrics(t, ts.do(t, "GET", fmt.Sprintf("/api/music/lyrics/%d?fetch=1", second.ID), admin, nil))
	if got.Failed == "" || asked.Load() != 1 {
		t.Fatalf("asked a source that was down a moment ago: %+v, asked %d", got, asked.Load())
	}
	// Someone asking again by hand is asked for.
	decodeLyrics(t, ts.do(t, "GET", fmt.Sprintf("/api/music/lyrics/%d?fetch=1&again=1", second.ID), admin, nil))
	if asked.Load() != 2 {
		t.Fatalf("asked %d times", asked.Load())
	}
}
