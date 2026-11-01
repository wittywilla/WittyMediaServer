package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittyplay/cores"
)

// What a limited account can't open doesn't show on the way there either.
func TestSmallThingsFollowTheLimits(t *testing.T) {
	ts := newTestServer(t)
	root := t.TempDir()
	tv := ts.library(t, "shows", root)
	if _, err := ts.DB.Exec(`INSERT INTO shows(id, library_id, path, title, sort_title, content_rating, added_at, updated_at) VALUES(5, ?, '/s', 'Show', 'show', 'G', 1, 1)`, tv); err != nil {
		t.Fatal(err)
	}
	show := int64(5)
	one, two, three := 1, 2, 3
	ep1 := ts.item(t, &library.Item{LibraryID: tv, Kind: "video", Path: filepath.Join(root, "s1e1.mkv"), Title: "Open", ShowID: &show, Season: &one, Episode: &one, ContentRating: "G"})
	hidden := ts.item(t, &library.Item{LibraryID: tv, Kind: "video", Path: filepath.Join(root, "s1e2.mkv"), Title: "Hidden", ShowID: &show, Season: &one, Episode: &two, ContentRating: "R", Poster: filepath.Join(root, "hidden.jpg")})
	ep3 := ts.item(t, &library.Item{LibraryID: tv, Kind: "video", Path: filepath.Join(root, "s1e3.mkv"), Title: "Open too", ShowID: &show, Season: &one, Episode: &three, ContentRating: "G"})
	ts.item(t, &library.Item{LibraryID: tv, Kind: "video", Path: filepath.Join(root, "s2e1.mkv"), Title: "Hidden season", ShowID: &show, Season: &two, Episode: &one, ContentRating: "R"})
	ts.user(t, "boss", "adult", true, "")
	_, kid := ts.user(t, "kid", "kids", false, "")
	_, adult := ts.user(t, "bob", "adult", false, "")

	// b: nothing about hidden seasons or episodes on a show's page.
	var page struct {
		Seasons []struct {
			Number, EpisodeCount int
		} `json:"seasons"`
	}
	w := ts.do(t, "GET", "/api/shows/5", kid, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if len(page.Seasons) != 1 || page.Seasons[0].Number != 1 || page.Seasons[0].EpisodeCount != 2 {
		t.Errorf("a Kids profile's seasons: %+v", page.Seasons)
	}
	w = ts.do(t, "GET", "/api/shows/5", adult, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Seasons) != 2 {
		t.Errorf("an adult's: %+v %v", page.Seasons, err)
	}

	// d: the next episode is the next one this account can open.
	var item struct {
		NextID *int64 `json:"nextId"`
	}
	w = ts.do(t, "GET", fmt.Sprintf("/api/items/%d", ep1.ID), kid, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &item); err != nil || item.NextID == nil || *item.NextID != ep3.ID {
		t.Errorf("next for a Kids profile: %v (hidden is %d) %v", item.NextID, hidden.ID, err)
	}
	item.NextID = nil
	w = ts.do(t, "GET", fmt.Sprintf("/api/items/%d", ep1.ID), adult, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &item); err != nil || item.NextID == nil || *item.NextID != hidden.ID {
		t.Errorf("next for an adult: %v %v", item.NextID, err)
	}

	// c: a show with no picture doesn't borrow one from an episode this account can't see.
	if err := os.WriteFile(filepath.Join(root, "hidden.jpg"), logoPicture(t), 0o644); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "GET", "/api/shows/5/art/poster", kid, nil); w.Code == 200 {
		t.Errorf("a Kids profile got a hidden episode's picture")
	}
	if w := ts.do(t, "GET", "/api/shows/5/art/poster", adult, nil); w.Code != 200 {
		t.Errorf("an adult: %d %s", w.Code, w.Body)
	}

	// a: the year in review keeps to what can still be opened.
	film := ts.item(t, &library.Item{LibraryID: tv, Kind: "video", Path: filepath.Join(root, "film.mkv"), Title: "Hidden film", ContentRating: "R"})
	if _, err := ts.DB.Exec(`INSERT INTO play_log(user_id, item_id, day, seconds) VALUES(?, ?, 20260101, 500)`, ts.userID(t, kid), film.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.DB.Exec(`INSERT INTO play_log(user_id, item_id, day, seconds) VALUES(?, ?, 20260102, 400)`, ts.userID(t, kid), ep1.ID); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "GET", "/api/me/review?year=2026", kid, nil); w.Code != 200 || strings.Contains(w.Body.String(), "Hidden") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}

func TestPhotoAlbumCoverFollowsTheLimits(t *testing.T) {
	ts := newTestServer(t)
	root := t.TempDir()
	lib := ts.library(t, "photos", root)
	hidden := ts.item(t, &library.Item{LibraryID: lib, Kind: "photo", Path: filepath.Join(root, "h.jpg"), Title: "h", ContentRating: "R"})
	ts.user(t, "boss", "adult", true, "")
	_, alice := ts.user(t, "alice", "adult", false, "")
	_, kid := ts.user(t, "kid", "kids", false, "")
	w := ts.do(t, "POST", "/api/photos/albums", alice, map[string]any{"libraryId": lib, "name": "A", "shared": true})
	var alb struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &alb); err != nil || alb.ID == 0 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/photos/albums/%d/items", alb.ID), alice, map[string]any{"ids": []int64{hidden.ID}}); w.Code >= 300 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var list []struct {
		CoverID *int64 `json:"coverId"`
		Count   int    `json:"count"`
	}
	w = ts.do(t, "GET", fmt.Sprintf("/api/photos/albums?library=%d", lib), kid, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, a := range list {
		if a.CoverID != nil && *a.CoverID == hidden.ID {
			t.Errorf("the hidden picture is its cover: %+v", a)
		}
	}
}

// A channel's logo is for whoever may see the channel.
func TestChannelLogoFollowsWhoMaySeeIt(t *testing.T) {
	h, admin, kid, s := tvServer(t)
	w := tvCall(h, "POST", "/api/channels", admin, `{"name":"Grown-ups","number":"5","visibility":{"mode":"adults"}}`)
	var c struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil || c.ID == 0 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	dir := s.channelLogoDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for format := range logoTypes {
		if err := os.WriteFile(channelLogoPath(dir, c.ID, format), logoPicture(t), 0o644); err != nil {
			t.Fatal(err)
		}
		break
	}
	if got := tvCall(h, "GET", fmt.Sprintf("/api/channels/%d/logo", c.ID), admin, ""); got.Code != http.StatusOK {
		t.Fatalf("an administrator: %d %s", got.Code, got.Body)
	}
	if got := tvCall(h, "GET", fmt.Sprintf("/api/channels/%d/logo", c.ID), kid, ""); got.Code != 404 {
		t.Errorf("someone who can't see the channel: %d", got.Code)
	}
}

// A core an administrator added isn't described to everyone.
func TestConsoleListKeepsCustomCoresToAdministrators(t *testing.T) {
	v := []consoleView{{}}
	v[0].Installed = append(v[0].Installed, cores.Core{ID: "mine", Source: cores.SourceCustom, SHA256: "abc"}, cores.Core{ID: "stock", Source: "installed"})
	hideCustom(v)
	if len(v[0].Installed) != 1 || v[0].Installed[0].ID != "stock" {
		t.Errorf("%+v", v[0].Installed)
	}
}

