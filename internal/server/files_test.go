package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

func linkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("creating links needs developer mode on Windows")
		}
		t.Fatal(err)
	}
}

// dirLink makes a link to a folder: a symbolic link, or a junction on Windows, which any
// account can make there and which Go doesn't report as a link.
func dirLink(t *testing.T, target, link string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if real, err := filepath.EvalSymlinks(target); err == nil {
			target = real // writing through a junction to a short name fails by itself
		}
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
		return
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// A folder swapped for a link to somewhere else, holding a file of the same name. On Windows
// this needs no special rights, and a junction isn't reported as a link.
func TestFolderSwappedForALinkIsNeverServed(t *testing.T) {
	g := newGalleryServer(t)
	outside := t.TempDir()
	dir := filepath.Join(g.lib.Paths[0], "d")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	photo := g.galleryItem(t, filepath.Join("d", "a.jpg"), "photo")
	video := g.galleryItem(t, filepath.Join("d", "b.mp4"), "video")
	for _, n := range []string{"a.jpg", "b.mp4"} {
		if err := os.WriteFile(filepath.Join(outside, n), []byte("SECRETBYTES"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(dir, n)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	dirLink(t, outside, dir)
	for _, p := range []string{
		fmt.Sprintf("/api/items/%d/original", photo.ID),
		fmt.Sprintf("/api/items/%d/image", photo.ID),
		fmt.Sprintf("/api/items/%d/original", video.ID),
		fmt.Sprintf("/api/items/%d/file", video.ID),
	} {
		if w := g.do("GET", p, g.adult, nil); strings.Contains(w.Body.String(), "SECRETBYTES") {
			t.Errorf("%s handed over the file behind the link: %d", p, w.Code)
		}
	}
	// A plain file beside it in the library is still served.
	plain := g.galleryItem(t, "plain.mp4", "video")
	if w := g.do("GET", fmt.Sprintf("/api/items/%d/file", plain.ID), g.adult, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "the real picture") {
		t.Errorf("a plain file: %d %s", w.Code, w.Body)
	}
	w := g.do("GET", fmt.Sprintf("/api/items/%d/original", plain.ID), g.adult, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "the real picture") {
		t.Errorf("a plain file's original: %d %s", w.Code, w.Body)
	}
}

// galleryItem indexes a real file in the gallery's library, as a scan would have.
func (g *galleryServer) galleryItem(t *testing.T, name, kind string) *library.Item {
	t.Helper()
	p := filepath.Join(g.lib.Paths[0], name)
	if err := os.WriteFile(p, []byte("the real picture"), 0o644); err != nil {
		t.Fatal(err)
	}
	it := &library.Item{LibraryID: g.lib.ID, Kind: kind, Path: p, Title: name, SortTitle: name}
	if err := g.s.Store.UpdateItem(context.Background(), it); err != nil {
		t.Fatal(err)
	}
	return it
}

// A file swapped for a link after the scan must not hand over what the link names, on any
// route that sends the file itself.
func TestLinkSwappedInIsNeverServed(t *testing.T) {
	g := newGalleryServer(t)
	outside := filepath.Join(t.TempDir(), "wms.db")
	if err := os.WriteFile(outside, []byte("SECRETBYTES"), 0o600); err != nil {
		t.Fatal(err)
	}
	photo := g.galleryItem(t, "a.jpg", "photo")
	video := g.galleryItem(t, "b.mp4", "video")
	if err := os.Remove(photo.Path); err != nil {
		t.Fatal(err)
	}
	linkOrSkip(t, outside, photo.Path)
	if err := os.Remove(video.Path); err != nil {
		t.Fatal(err)
	}
	linkOrSkip(t, outside, video.Path)
	for _, p := range []string{
		fmt.Sprintf("/api/items/%d/original", photo.ID),
		fmt.Sprintf("/api/items/%d/image", photo.ID),
		fmt.Sprintf("/api/items/%d/original", video.ID),
		fmt.Sprintf("/api/items/%d/file", video.ID),
	} {
		w := g.do("GET", p, g.adult, nil)
		if strings.Contains(w.Body.String(), "SECRETBYTES") {
			t.Errorf("%s handed over the file behind the link: %d", p, w.Code)
		}
	}
}

// Check first and open by name later can be raced. What was checked has to be what is read.
func TestSwappingALinkDuringServingNeverLeaks(t *testing.T) {
	g := newGalleryServer(t)
	dir := g.lib.Paths[0]
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("SECRETBYTES"), 0o600); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(dir, "real.mp4")
	if err := os.WriteFile(real, []byte("the real film"), 0o644); err != nil {
		t.Fatal(err)
	}
	it := g.galleryItem(t, "film.mp4", "video")
	good, bad := filepath.Join(dir, ".good"), filepath.Join(dir, ".bad")
	linkOrSkip(t, real, good)
	linkOrSkip(t, outside, bad)
	stop := make(chan struct{})
	done := make(chan struct{})
	var swaps atomic.Int64
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			tmp := filepath.Join(dir, ".next")
			name := good
			if swaps.Add(1)%2 == 0 {
				name = bad
			}
			target, _ := os.Readlink(name)
			_ = os.Remove(tmp)
			if os.Symlink(target, tmp) == nil {
				_ = os.Rename(tmp, it.Path)
			}
		}
	}()
	leaked := 0
	for range 600 {
		w := g.do("GET", fmt.Sprintf("/api/items/%d/file", it.ID), g.adult, nil)
		if strings.Contains(w.Body.String(), "SECRETBYTES") {
			leaked++
		}
		w = g.do("GET", fmt.Sprintf("/api/items/%d/original", it.ID), g.adult, nil)
		if strings.Contains(w.Body.String(), "SECRETBYTES") {
			leaked++
		}
	}
	close(stop)
	<-done
	if leaked > 0 {
		t.Errorf("the outside file came through %d times in 1200 requests", leaked)
	}
}

// A number that JSON can't say used to blank every list holding it: the answer was 200 with
// nothing in it, and nothing in the log.
func TestNumbersJSONCantSay(t *testing.T) {
	g := newGalleryServer(t)
	inf := math.Inf(1)
	nan := math.NaN()
	it := g.galleryItem(t, "a.mp4", "video")
	it.Rating, it.RuntimeSec, it.IntroStart, it.IntroEnd = &inf, inf, &nan, &inf
	if err := g.s.Store.UpdateItem(context.Background(), it); err != nil {
		t.Fatal(err)
	}
	w := g.do("GET", fmt.Sprintf("/api/items?library=%d&limit=50", g.lib.ID), g.adult, nil)
	var got struct {
		Items []struct {
			ID         int64    `json:"id"`
			Rating     *float64 `json:"rating"`
			RuntimeSec float64  `json:"runtimeSec"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != 200 || len(got.Items) != 1 {
		t.Fatalf("%d %q %v", w.Code, w.Body, err)
	}
	if got.Items[0].Rating != nil || got.Items[0].RuntimeSec != 0 {
		t.Errorf("%+v", got.Items[0])
	}

	rec := httptest.NewRecorder()
	writeJSON(rec, 200, map[string]any{"bad": inf})
	if rec.Code != 500 || !json.Valid(rec.Body.Bytes()) {
		t.Errorf("what can't be written is an error, not an empty answer: %d %q", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	writeJSON(rec, 201, map[string]any{"fine": 1})
	if rec.Code != 201 || !strings.Contains(rec.Body.String(), `"fine":1`) {
		t.Errorf("%d %q", rec.Code, rec.Body)
	}
}

// An upload's folders may have been swapped for a link: nothing lands where it leads.
func TestUploadsDontFollowAFolderSwappedForALink(t *testing.T) {
	g := newGalleryServer(t)
	outside := t.TempDir()
	dirLink(t, outside, filepath.Join(g.lib.Paths[0], "Uploads"))
	w := g.upload(g.adult, "a.jpg", tinyJPEG(t))
	if strings.Contains(w.Body.String(), `"id":`) {
		t.Errorf("the upload went through: %s", w.Body)
	}
	if names, _ := os.ReadDir(outside); len(names) != 0 {
		t.Errorf("wrote through the link: %v", names)
	}
}
