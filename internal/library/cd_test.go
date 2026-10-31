package library

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wittywilla/wittycd"
	"github.com/wittywilla/wittymediaserver/internal/db"
)

// A generated tone: no audio is kept in the repository.
func cdTone(sectors int) []byte {
	out := make([]byte, sectors*2352)
	for i := range sectors * 588 {
		v := int16(9000 * math.Sin(2*math.Pi*440*float64(i)/44100))
		binary.LittleEndian.PutUint16(out[i*4:], uint16(v))
		binary.LittleEndian.PutUint16(out[i*4+2:], uint16(v))
	}
	return out
}

func writeCD(t *testing.T, dir, base, title string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, base+".bin"), cdTone(450), 0o644); err != nil {
		t.Fatal(err)
	}
	cue := "FILE \"" + base + ".bin\" BINARY\nTRACK 01 AUDIO\n" + title + "INDEX 01 00:00:00\nTRACK 02 AUDIO\nINDEX 01 00:03:00\n"
	p := filepath.Join(dir, base+".cue")
	if err := os.WriteFile(p, []byte(cue), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func cdScanner(t *testing.T) (*Scanner, *Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	st := &Store{DB: d}
	off := func() bool { return false }
	return &Scanner{Store: st, ImagesDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), SaveMeta: off, MusicBrainzOn: off}, st, ctx
}

func TestDiscoverAudioFindsImagesAndClaimsTheirFiles(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "Artist", "Album")
	writeCD(t, album, "CDImage", "")
	// A .wav named by a sheet isn't a song as well.
	wavDir := filepath.Join(root, "Wavs")
	os.MkdirAll(wavDir, 0o755)
	os.WriteFile(filepath.Join(wavDir, "a.wav"), []byte("RIFF"), 0o644)
	os.WriteFile(filepath.Join(wavDir, "w.cue"), []byte("FILE \"a.wav\" WAVE\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"), 0o644)
	// A rip's notes beside tagged per-track files are left alone: those stay songs.
	rip := filepath.Join(root, "Rip")
	os.MkdirAll(rip, 0o755)
	os.WriteFile(filepath.Join(rip, "01.flac"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(rip, "02.flac"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(rip, "rip.cue"), []byte("FILE \"01.flac\" WAVE\nTRACK 01 AUDIO\nINDEX 01 00:00:00\nFILE \"02.flac\" WAVE\nTRACK 02 AUDIO\nINDEX 01 00:00:00\n"), 0o644)
	mp3 := filepath.Join(root, "Mp3")
	os.MkdirAll(mp3, 0o755)
	os.WriteFile(filepath.Join(mp3, "a.mp3"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(mp3, "a.cue"), []byte("FILE \"a.mp3\" MP3\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"), 0o644)

	c, err := discoverAudio(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, x := range c {
		rel, _ := filepath.Rel(root, x.path)
		got[filepath.ToSlash(rel)] = x.kind
	}
	want := map[string]string{"Artist/Album/CDImage.cue": CDKind, "Wavs/w.cue": CDKind, "Rip/01.flac": "audio", "Rip/02.flac": "audio", "Mp3/a.mp3": "audio"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: kind %q, want %q (found %v)", k, got[k], v, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("found %v, want only %v", got, want)
	}
}

func TestIndexCDInMusicAndDiscs(t *testing.T) {
	s, st, ctx := cdScanner(t)
	root := t.TempDir()
	album := filepath.Join(root, "The Band", "First Album")
	cue := writeCD(t, album, "CDImage", "TITLE \"Song One\"\n")
	info, _ := os.Stat(cue)
	lib, err := st.CreateLibrary(ctx, "Music", "music", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	c := candidate{path: cue, kind: CDKind, size: info.Size(), mtime: info.ModTime().Unix()}
	if err := s.indexCD(ctx, lib, c); err != nil {
		t.Fatal(err)
	}
	disc, err := st.itemByPath(ctx, lib.ID, cue)
	if err != nil {
		t.Fatal(err)
	}
	// A generic file name: the folders say what it is.
	if disc.Title != "First Album" || disc.Artist != "The Band" || disc.Kind != CDKind {
		t.Fatalf("disc = %+v", disc)
	}
	if cd := cdInfoOf(disc); cd == nil || cd.DiscID == "" || cd.Lookup != "off" {
		t.Fatalf("cd info = %+v", cdInfoOf(disc))
	}
	// Its tracks are songs of an album.
	albums, err := st.Albums(ctx, lib.ID, "", "", "", nil)
	if err != nil || len(albums) != 1 || albums[0].Tracks != 2 || albums[0].Title != "First Album" {
		t.Fatalf("albums = %+v, %v", albums, err)
	}
	track, err := st.itemByPath(ctx, lib.ID, cue+"#track=1")
	if err != nil {
		t.Fatal(err)
	}
	if track.Title != "Song One" || track.Kind != "audio" || track.CDDisc == nil || *track.CDDisc != disc.ID || track.VideoCodec != "pcm_s16le" {
		t.Fatalf("track = %+v", track)
	}
	if two, _ := st.itemByPath(ctx, lib.ID, cue+"#track=2"); two == nil || two.Title != "Track 02" {
		t.Fatalf("track 2 = %+v", two)
	}
	if image, n, ok := CDTrackNumber(track.Path); !ok || n != 1 || image != cue {
		t.Fatalf("track path %q", track.Path)
	}
	// Scanning again changes nothing, and keeping the image keeps its tracks.
	if err := s.indexCD(ctx, lib, c); err != nil {
		t.Fatal(err)
	}
	if err := st.markMissing(ctx, lib.ID, map[string]bool{cue: true}, nil); err != nil {
		t.Fatal(err)
	}
	if again, _ := st.itemByPath(ctx, lib.ID, cue+"#track=2"); again == nil || again.Missing {
		t.Fatalf("a track went missing with its image still there: %+v", again)
	}
	if err := st.markMissing(ctx, lib.ID, map[string]bool{}, nil); err != nil {
		t.Fatal(err)
	}
	if gone, _ := st.itemByPath(ctx, lib.ID, cue+"#track=2"); gone == nil || !gone.Missing {
		t.Fatalf("a track stayed after its image went: %+v", gone)
	}

	// On a shelf a disc is only a disc, and shows with kind=disc.
	shelf, err := st.CreateLibrary(ctx, "Discs", "discs", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.indexCD(ctx, shelf, c); err != nil {
		t.Fatal(err)
	}
	items, total, err := st.Items(ctx, ItemQuery{LibraryID: shelf.ID})
	if err != nil || total != 1 || items[0].Kind != CDKind {
		t.Fatalf("shelf = %+v, %d, %v", items, total, err)
	}
	if _, total, _ := st.Items(ctx, ItemQuery{LibraryID: shelf.ID, Kind: "disc"}); total != 1 {
		t.Fatalf("kind=disc found %d", total)
	}
	if _, total, _ := st.Items(ctx, ItemQuery{LibraryID: shelf.ID, DiscContent: "music"}); total != 1 {
		t.Fatalf("discContent=music found %d", total)
	}
	// Any position is worth coming back to on a disc.
	if got := resumePoint(CDKind, &Progress{PositionSec: 12}); got != 12 {
		t.Fatalf("resume = %v", got)
	}
}

func TestCDNames(t *testing.T) {
	lib := &Library{Paths: []string{filepath.FromSlash("/m")}}
	cases := []struct{ path, album, artist string }{
		{"/m/Artist/Album/CDImage.cue", "Album", "Artist"},
		{"/m/Artist - Album/image.cue", "Album", "Artist"},
		{"/m/Artist - Album.cue", "Album", "Artist"},
		{"/m/Album Only.cue", "Album Only", ""},
		{"/m/Artist/Album/CD1/disc.cue", "Album", "Artist"},
	}
	for _, c := range cases {
		a, b := cdNames(filepath.FromSlash(c.path), lib)
		if a != c.album || (c.artist != "" && b != c.artist) {
			t.Errorf("%s: %q by %q, want %q by %q", c.path, a, b, c.album, c.artist)
		}
	}
}

// mbServer is a stand-in for MusicBrainz and the Cover Art Archive. It records
// what it was asked, so a test can say exactly what left the server.
func mbServer(t *testing.T, reply func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *[]string) {
	t.Helper()
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.String())
		if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "WittyMediaServer/") || !strings.Contains(ua, "(") {
			t.Errorf("no proper User-Agent: %q", ua)
		}
		reply(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &asked
}

const testRelease = "11111111-2222-3333-4444-555555555555"

func releaseJSON(id, title, discID string, tracks ...string) map[string]any {
	ts := []map[string]any{}
	for i, n := range tracks {
		ts = append(ts, map[string]any{"position": i + 1, "title": n})
	}
	discs := []map[string]any{}
	if discID != "" {
		discs = append(discs, map[string]any{"id": discID})
	}
	return map[string]any{"id": id, "title": title, "date": "1999-03-02", "country": "GB", "artist-credit": []map[string]any{{"name": "The Real Band", "joinphrase": ""}},
		"cover-art-archive": map[string]any{"front": true},
		"media":             []map[string]any{{"position": 1, "track-count": len(tracks), "discs": discs, "tracks": ts}}}
}

func TestLookupUsesOnlyTheTableOfContentsAndAppliesOneAnswer(t *testing.T) {
	s, st, ctx := cdScanner(t)
	s.MusicBrainzOn = func() bool { return true }
	root := t.TempDir()
	cue := writeCD(t, filepath.Join(root, "Secret Folder Name"), "My Private Rip", "")
	info, _ := os.Stat(cue)
	lib, _ := st.CreateLibrary(ctx, "Music", "music", []string{root})
	var discID atomic.Value
	srv, asked := mbServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/release/") {
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0, 1, 1})
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/discid/")
		discID.Store(id)
		json.NewEncoder(w).Encode(map[string]any{"id": id, "releases": []any{releaseJSON(testRelease, "Real Album", id, "Real One", "Real Two")}})
	})
	s.MusicBrainzBase, s.CoverArtBase = srv.URL, srv.URL
	oldClient := *SafeClient
	*SafeClient = *http.DefaultClient
	defer func() { *SafeClient = oldClient }()
	c := candidate{path: cue, kind: CDKind, size: info.Size(), mtime: info.ModTime().Unix()}
	if err := s.indexCD(ctx, lib, c); err != nil {
		t.Fatal(err)
	}
	for _, u := range *asked {
		for _, leak := range []string{"Secret", "Private", "Rip", "My%20"} {
			if strings.Contains(u, leak) {
				t.Errorf("%q left the server in %s", leak, u)
			}
		}
	}
	if len(*asked) != 2 || !strings.Contains((*asked)[0], "toc=1+2+600+150+375") || !strings.Contains((*asked)[0], "fmt=json") {
		t.Fatalf("asked %v", *asked)
	}
	disc, err := st.itemByPath(ctx, lib.ID, cue)
	if err != nil {
		t.Fatal(err)
	}
	if disc.Title != "Real Album" || disc.Artist != "The Real Band" || disc.Year == nil || *disc.Year != 1999 || disc.Provider != "musicbrainz" || disc.ProviderID != testRelease {
		t.Fatalf("disc = %+v", disc)
	}
	if !strings.HasPrefix(disc.Poster, "cache:cd-") {
		t.Fatalf("poster = %q", disc.Poster)
	}
	track, _ := st.itemByPath(ctx, lib.ID, cue+"#track=2")
	if track == nil || track.Title != "Real Two" || track.Album != "Real Album" || track.Poster != disc.Poster {
		t.Fatalf("track = %+v", track)
	}
	// Nothing was written beside the owner's files with saving turned off.
	entries, _ := os.ReadDir(filepath.Dir(cue))
	if len(entries) != 2 {
		t.Fatalf("files beside the image: %v", entries)
	}
}

func TestSeveralDifferentMatchesWait(t *testing.T) {
	s, st, ctx := cdScanner(t)
	s.MusicBrainzOn = func() bool { return true }
	root := t.TempDir()
	cue := writeCD(t, filepath.Join(root, "Album"), "x", "TITLE \"Own Song\"\n")
	info, _ := os.Stat(cue)
	lib, _ := st.CreateLibrary(ctx, "Music", "music", []string{root})
	srv, _ := mbServer(t, func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/discid/")
		json.NewEncoder(w).Encode(map[string]any{"releases": []any{
			releaseJSON(testRelease, "One Album", id, "A", "B"),
			releaseJSON("99999999-2222-3333-4444-555555555555", "Another Album", id, "C", "D"),
		}})
	})
	s.MusicBrainzBase, s.CoverArtBase = srv.URL, srv.URL
	oldClient := *SafeClient
	*SafeClient = *http.DefaultClient
	defer func() { *SafeClient = oldClient }()
	if err := s.indexCD(ctx, lib, candidate{path: cue, kind: CDKind, size: info.Size(), mtime: info.ModTime().Unix()}); err != nil {
		t.Fatal(err)
	}
	disc, _ := st.itemByPath(ctx, lib.ID, cue)
	cd := cdInfoOf(disc)
	if cd.Lookup != "ask" || cd.Matches != 2 || disc.Provider != "" {
		t.Fatalf("cd = %+v, provider %q", cd, disc.Provider)
	}
	if track, _ := st.itemByPath(ctx, lib.ID, cue+"#track=1"); track == nil || track.Title != "Own Song" {
		t.Fatalf("track = %+v", track)
	}
}

func TestSameAlbumPressingsNeedNoAsking(t *testing.T) {
	a := CDMatch{Title: "Same", Artist: "Band", Exact: true, Tracks: []CDTrackName{{Number: 1, Title: "One"}}}
	b := a
	b.ReleaseID = "other"
	if !sameAlbum([]CDMatch{a, b}) {
		t.Fatal("two pressings of one album should need no asking")
	}
	c := a
	c.Exact = false
	if sameAlbum([]CDMatch{a, c}) {
		t.Fatal("a close match isn't good enough")
	}
	d := a
	d.Tracks = []CDTrackName{{Number: 1, Title: "Different"}}
	if sameAlbum([]CDMatch{a, d}) {
		t.Fatal("different tracks are a different album")
	}
}

func TestSidecarOnlyWhenSavingIsOn(t *testing.T) {
	s, st, ctx := cdScanner(t)
	s.SaveMeta = func() bool { return true }
	root := t.TempDir()
	cue := writeCD(t, filepath.Join(root, "Album"), "x", "")
	lib, _ := st.CreateLibrary(ctx, "Music", "music", []string{root})
	info, _ := os.Stat(cue)
	if err := s.indexCD(ctx, lib, candidate{path: cue, kind: CDKind, size: info.Size(), mtime: info.ModTime().Unix()}); err != nil {
		t.Fatal(err)
	}
	disc, _ := st.itemByPath(ctx, lib.ID, cue)
	if err := s.ApplyCDMatch(ctx, lib, disc, mustOpenCD(t, cue), &CDMatch{ReleaseID: testRelease, Title: "Saved", Artist: "Band", Tracks: []CDTrackName{{Number: 1, Title: "S1"}}}); err != nil {
		t.Fatal(err)
	}
	sc := readCDSidecar(cue)
	if sc == nil || sc.Title != "Saved" || sc.ReleaseID != testRelease {
		t.Fatalf("sidecar = %+v", sc)
	}
	// A fresh scan of the disc reads it back without asking anyone.
	os.Chtimes(cue, info.ModTime().Add(1e9), info.ModTime().Add(1e9))
	info, _ = os.Stat(cue)
	if err := s.indexCD(ctx, lib, candidate{path: cue, kind: CDKind, size: info.Size(), mtime: info.ModTime().Unix()}); err != nil {
		t.Fatal(err)
	}
	disc, _ = st.itemByPath(ctx, lib.ID, cue)
	if disc.Title != "Saved" || cdInfoOf(disc).Lookup != "sidecar" {
		t.Fatalf("disc = %+v", disc)
	}
}

func mustOpenCD(t *testing.T, path string) *wittycd.Disc {
	t.Helper()
	d, err := wittycd.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}
