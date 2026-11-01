package server

import (
	"context"
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
)

const noLibraryForYou = `{"maxAge":-1,"allowUnrated":true,"libraries":[999]}`

// Liked before a limit came, or in a library out of reach: still not to be named.
func TestLikedSongsKeepToWhatAnAccountMaySee(t *testing.T) {
	f := newMusicFixture(t)
	u, tok := f.user(t, "limited", "adult", false, noLibraryForYou)
	if _, err := f.DB.ExecContext(context.Background(), `INSERT INTO item_reactions(user_id, item_id, value, created_at) VALUES(?,?,1,1)`, u.ID, f.song.ID); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	w := f.do(t, "GET", "/api/music/likes", tok, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &ids); err != nil {
		t.Fatalf("%v %s", err, w.Body)
	}
	if len(ids) != 0 {
		t.Errorf("a limited account was told about %v", ids)
	}
	if _, err := f.DB.ExecContext(context.Background(), `INSERT INTO item_reactions(user_id, item_id, value, created_at) VALUES((SELECT id FROM users WHERE name = 'adult'),?,1,1)`, f.song.ID); err != nil {
		t.Fatal(err)
	}
	if w := f.do(t, "GET", "/api/music/likes", f.adult, nil); !strings.Contains(w.Body.String(), fmt.Sprint(f.song.ID)) {
		t.Errorf("an account with no limits: %s", w.Body)
	}
}

// The credits name every logo there is, and a limited account shouldn't learn of a studio that
// way, or how much is on the server from the totals.
func TestLogoFactsKeepToWhatAnAccountMaySee(t *testing.T) {
	f := newMusicFixture(t)
	ctx := context.Background()
	for _, n := range []string{"Hidden Pictures", "Open Pictures"} {
		if _, err := f.DB.ExecContext(ctx, `INSERT INTO logos(kind, key, name, status, file) VALUES('studio',?,?,'found','x')`, library.BrandKey(n), n); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.DB.ExecContext(ctx, `INSERT INTO logos(kind, key, name, status, file) VALUES('console','a console','A Console','found','x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.DB.ExecContext(ctx, `INSERT INTO title_brands(kind, key, name, library_id, item_id) VALUES('studio',?,'Open Pictures',?,?)`, library.BrandKey("Open Pictures"), f.lib, f.song.ID); err != nil {
		t.Fatal(err)
	}
	kid := f.do(t, "GET", "/api/logos/credits", f.kid, nil).Body.String()
	if strings.Contains(kid, "Hidden Pictures") {
		t.Errorf("a Kids profile read: %s", kid)
	}
	if !strings.Contains(kid, "Open Pictures") || !strings.Contains(kid, "A Console") {
		t.Errorf("what it may see should still be credited: %s", kid)
	}
	if w := f.do(t, "GET", "/api/logos/credits", f.adult, nil); !strings.Contains(w.Body.String(), "Hidden Pictures") {
		t.Errorf("an account with no limits should see them: %s", w.Body)
	}
	status := func(path, tok string) map[string]any {
		var st map[string]any
		if err := json.Unmarshal(f.do(t, "GET", path, tok, nil).Body.Bytes(), &st); err != nil {
			t.Fatal(err)
		}
		return st
	}
	if st := status("/api/logos/status", f.kid); st["counts"] != nil {
		t.Errorf("how many logos there are, to anyone: %v", st)
	}
	if st := status("/api/logos/status", f.admin); st["counts"] == nil {
		t.Errorf("an administrator's settings need them: %v", st)
	}
	if _, err := f.DB.ExecContext(ctx, `INSERT INTO listing_titles(target, target_id, region, checked_at) VALUES('item',?,?,5)`, f.song.ID, f.Store.ListingsRegion(ctx)); err != nil {
		t.Fatal(err)
	}
	if st := status("/api/listings/status", f.kid); st["titles"] != float64(0) || st["attribution"] == "" {
		t.Errorf("how many titles were checked, to anyone: %v", st)
	}
	if st := status("/api/listings/status", f.admin); st["titles"] != float64(1) {
		t.Errorf("an administrator's settings need them: %v", st)
	}
}

// Fetching a chooser's picture contacts Wikimedia, so it waits for the agreement the rest does.
func TestLogoPreviewNeedsAgreement(t *testing.T) {
	f := newMusicFixture(t)
	if w := f.do(t, "GET", "/api/logos/preview?src=https%3A%2F%2Fupload.wikimedia.org%2Fx%2Fnone.png", f.admin, nil); w.Code != 409 {
		t.Errorf("before agreeing: %d %s", w.Code, w.Body)
	}
}

// A picture set on many titles at once is held to what a single edit is.
func TestBulkEditChecksPictures(t *testing.T) {
	f := newMusicFixture(t)
	outside := filepath.Join(t.TempDir(), "secret.jpg")
	body := map[string]any{"titles": []map[string]any{{"target": "item", "id": f.song.ID}}, "set": map[string]string{"poster": outside}}
	if w := f.do(t, "POST", "/api/meta/bulk", f.admin, body); w.Code != 400 {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	it, err := f.Store.Item(context.Background(), f.song.ID)
	if err != nil {
		t.Fatal(err)
	}
	if it.Poster == outside {
		t.Error("it was kept")
	}
}

// The lyrics source an owner names may be at home, and only that one address may be.
func TestLyricsMirrorStaysOnItsHost(t *testing.T) {
	var reached atomic.Int64
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Add(1) }))
	defer other.Close()
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+"/secret", http.StatusFound)
	}))
	defer mirror.Close()
	res, err := lyricsMirrorClient.Get(mirror.URL)
	if err == nil {
		res.Body.Close()
	}
	if reached.Load() != 0 {
		t.Error("a redirect led away from the address the owner named")
	}
	if _, err := lyricsMirrorClient.Get("http://169.254.169.254/latest/meta-data/"); err == nil || !strings.Contains(err.Error(), "lyrics server") {
		t.Errorf("a cloud host's own address: %v", err)
	}
	// Its own redirect, on the machine or the network, still works.
	home := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/api", http.StatusFound)
			return
		}
		w.WriteHeader(204)
	}))
	defer home.Close()
	res, err = lyricsMirrorClient.Get(home.URL)
	if err != nil || res.StatusCode != 204 {
		t.Fatalf("%v %v", res, err)
	}
	res.Body.Close()
}

// What a limit hides stays hidden on a parent's page, by the extra's show too.
func TestAttachedExtrasGoByTheirShow(t *testing.T) {
	f := newMusicFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	lib := f.library(t, "shows", root)
	if _, err := f.DB.ExecContext(ctx, `INSERT INTO shows(id, library_id, path, title, sort_title, content_rating, added_at, updated_at) VALUES(55,?,'/s','A Show','a show','R',1,1)`, lib); err != nil {
		t.Fatal(err)
	}
	show := int64(55)
	parent := f.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(root, "parent.mkv"), Title: "Parent", ContentRating: "G"})
	extra := f.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(root, "extra.mkv"), Title: "Hidden extra", ShowID: &show})
	if err := f.Store.AttachItem(ctx, extra.ID, library.LinkExtra, library.ExtraTypes[0], "", &parent.ID, nil); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/meta/item/%d/attached", parent.ID)
	if w := f.do(t, "GET", path, f.kid, nil); w.Code != 200 || strings.Contains(w.Body.String(), "Hidden extra") {
		t.Errorf("a Kids profile: %d %s", w.Code, w.Body)
	}
	if w := f.do(t, "GET", path, f.adult, nil); !strings.Contains(w.Body.String(), "Hidden extra") {
		t.Errorf("an account with no limits: %s", w.Body)
	}
}

// A credit is a link on the page, and what's kept came from a service's answer.
func TestArtistCreditIsALinkToTheArticle(t *testing.T) {
	f := newMusicFixture(t)
	path := f.artistCache("An Artist", ".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(artistFacts{Name: "An Artist", Title: "An Artist", Bio: strings.Repeat("words ", 100000), URL: "javascript:alert(1)", At: 1})
	if err := os.WriteFile(path, b, 0o640); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Bio    string `json:"bio"`
		BioURL string `json:"bioUrl"`
	}
	w := f.do(t, "GET", fmt.Sprintf("/api/music/artist?library=%d&name=An%%20Artist", f.lib), f.adult, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("%v %s", err, w.Body)
	}
	if !strings.HasPrefix(out.BioURL, "https://") {
		t.Errorf("credit link %q", out.BioURL)
	}
	if len(out.Bio) > 8<<10 {
		t.Errorf("%d bytes of biography", len(out.Bio))
	}
}
