package server

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
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
