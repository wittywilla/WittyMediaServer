package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// A music library with one song for everyone, and an administrator, an adult and a Kids profile.
type musicFixture struct {
	*testServer
	dir               string
	lib               int64
	song              *library.Item
	admin, adult, kid string
}

func newMusicFixture(t *testing.T) *musicFixture {
	t.Helper()
	ts := newTestServer(t)
	root := t.TempDir()
	dir := filepath.Join(root, "An Artist", "An Album")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f := &musicFixture{testServer: ts, dir: dir}
	_, f.admin = ts.user(t, "owner", "adult", true, "")
	_, f.adult = ts.user(t, "adult", "adult", false, "")
	_, f.kid = ts.user(t, "kid", "kids", false, "")
	f.lib = ts.library(t, "music", root)
	path := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(path, []byte("not really a song"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.song = ts.item(t, &library.Item{LibraryID: f.lib, Kind: "audio", Path: path, Title: "A Song", Artist: "An Artist", Album: "An Album", ContentRating: "G"})
	return f
}

func (f *musicFixture) url(format string) string { return fmt.Sprintf(format, f.song.ID) }

func (f *musicFixture) artistPhoto(t *testing.T) (int, string, string) {
	t.Helper()
	w := f.do(t, "GET", "/api/music/artist-photo?library="+strconv.FormatInt(f.lib, 10)+"&name=An%20Artist", f.adult, nil)
	return w.Code, w.Header().Get("Content-Type"), w.Body.String()
}

// A small lyrics file beside a song, opened by anyone, used to make an answer of gigabytes.
func TestLyricsFileCantMakeAHugeAnswer(t *testing.T) {
	f := newMusicFixture(t)
	bomb := strings.Repeat("[00:01]", 18000) + strings.Repeat("<00:01>a", 18000)
	if err := os.WriteFile(filepath.Join(f.dir, "song.lrc"), []byte(bomb), 0o644); err != nil {
		t.Fatal(err)
	}
	w := f.do(t, "GET", f.url("/api/music/lyrics/%d"), f.kid, nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String()[:min(w.Body.Len(), 300)])
	}
	if w.Body.Len() > 16<<20 {
		t.Errorf("%d bytes of file became %d of answer", len(bomb), w.Body.Len())
	}
}

// Whatever is called artist.jpg gets the same check as any other picture.
func TestArtistPictureMustBeAPicture(t *testing.T) {
	f := newMusicFixture(t)
	p := filepath.Join(filepath.Dir(f.dir), "artist.jpg")
	if err := os.WriteFile(p, []byte("<html><script>alert(1)</script>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, body := f.artistPhoto(t); strings.Contains(body, "alert(1)") {
		t.Errorf("%d served as it is: %s", code, body)
	}
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")
	if err := os.WriteFile(p, png, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, ct, _ := f.artistPhoto(t); code != 200 || ct != "image/png" {
		t.Errorf("a real picture: %d %s", code, ct)
	}
}

// A file beside a song is swapped for a link while it's being asked for: the words and the
// booklet were read by name after they were listed.
func TestSwappingFilesBesideASongNeverLeaks(t *testing.T) {
	f := newMusicFixture(t)
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("SECRETBYTES"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, s := range []struct{ name, route string }{
		{"notes.txt", f.url("/api/music/extras/%d/file") + "?name=notes.txt"},
		{"booklet.pdf", f.url("/api/music/extras/%d/file") + "?name=booklet.pdf"},
		{"song.lrc", f.url("/api/music/lyrics/%d")},
	} {
		real := filepath.Join(f.dir, s.name)
		plain := filepath.Join(f.dir, ".plain")
		if err := os.WriteFile(plain, []byte("[00:01]words"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(real, []byte("[00:01]words"), 0o644); err != nil {
			t.Fatal(err)
		}
		probe := filepath.Join(f.dir, ".probe")
		linkOrSkip(t, outside, probe)
		_ = os.Remove(probe)
		stop := make(chan struct{})
		done := make(chan struct{})
		var flips atomic.Int64
		go func() {
			defer close(done)
			tmp := filepath.Join(f.dir, ".next")
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = os.Remove(tmp)
				if flips.Add(1)%2 == 0 {
					_ = os.Symlink(outside, tmp)
				} else {
					_ = os.Link(plain, tmp)
				}
				_ = os.Rename(tmp, real)
			}
		}()
		leaked := 0
		for range 800 {
			if w := f.do(t, "GET", s.route, f.adult, nil); strings.Contains(w.Body.String(), "SECRETBYTES") {
				leaked++
			}
		}
		close(stop)
		<-done
		_ = os.Remove(real)
		if leaked > 0 {
			t.Errorf("%s: the outside file came through %d times in 800 requests", s.name, leaked)
		}
	}
}
