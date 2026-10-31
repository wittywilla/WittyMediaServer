package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// fakeGameSource stands in for an outside service.
type fakeGameSource struct {
	id   string
	test func() (string, error)
}

func (f *fakeGameSource) About() gamemeta.About {
	return gamemeta.About{ID: f.id, Name: "Fake", Site: "https://example.test", Fields: []gamemeta.KeyField{{Setting: "rawgKey", Label: "API key"}}, Cost: "Free", Credit: "From fake", License: "Test"}
}
func (f *fakeGameSource) Lookup(ctx context.Context, q gamemeta.Query) (*gamemeta.Info, error) {
	return nil, gamemeta.ErrNotFound
}
func (f *fakeGameSource) Search(ctx context.Context, console, title string) ([]gamemeta.Candidate, error) {
	return []gamemeta.Candidate{{Source: f.id, ID: "7", Title: "Found " + title, Score: 90}}, nil
}
func (f *fakeGameSource) Fetch(ctx context.Context, console, id string) (*gamemeta.Info, error) {
	age, _ := gamemeta.NewAge("ESRB", "M")
	return &gamemeta.Info{ID: id, Title: "Chosen Game (USA)", Developer: "Maker", Ages: []gamemeta.AgeRating{age}}, nil
}
func (f *fakeGameSource) Test(ctx context.Context) (string, error) { return f.test() }

func gameItem(t *testing.T, ts *testServer, title string, rating string, poster string) *library.Item {
	t.Helper()
	lib := ts.library(t, "games", t.TempDir())
	return ts.item(t, &library.Item{LibraryID: lib, Kind: "game", Path: t.TempDir() + "/" + title + ".sfc", Title: title, SortTitle: title, Console: "snes", ContentRating: rating, Poster: poster})
}

func TestGameKeysAreSecrets(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"igdbClientSecret": "very-secret-value", "rawgKey": "another-secret", "ssPassword": "pw-secret"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if got := ts.DB.GetSetting(context.Background(), "rawgKey", ""); got != "another-secret" {
		t.Errorf("not saved: %q", got)
	}
	for _, path := range []string{"/api/settings", "/api/gamemeta/sources", "/api/gamemeta/status"} {
		w := ts.do(t, "GET", path, admin, nil)
		if w.Code != 200 {
			t.Fatalf("%s: %d", path, w.Code)
		}
		for _, secret := range []string{"very-secret-value", "another-secret", "pw-secret"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Errorf("%s gives away a key: %s", path, w.Body)
			}
		}
	}
	// Sent back masked, a saved key stays what it was.
	w := ts.do(t, "GET", "/api/settings", admin, nil)
	var s map[string]string
	json.Unmarshal(w.Body.Bytes(), &s)
	if s["rawgKeySet"] != "1" || !strings.Contains(s["rawgKey"], "•") {
		t.Errorf("%+v", s)
	}
	ts.do(t, "PUT", "/api/settings", admin, map[string]string{"rawgKey": s["rawgKey"]})
	if got := ts.DB.GetSetting(context.Background(), "rawgKey", ""); got != "another-secret" {
		t.Errorf("overwritten with its mask: %q", got)
	}
	// The sources list says what's saved, and nothing more.
	w = ts.do(t, "GET", "/api/gamemeta/sources", admin, nil)
	var list struct {
		Sources []struct {
			ID    string   `json:"id"`
			Saved []string `json:"saved"`
			Ready bool     `json:"ready"`
			On    bool     `json:"on"`
		} `json:"sources"`
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	found := map[string]bool{}
	for _, x := range list.Sources {
		found[x.ID] = true
		if x.ID == "rawg" && (!x.Ready || len(x.Saved) != 1 || x.Saved[0] != "rawgKey") {
			t.Errorf("%+v", x)
		}
		if x.ID == "igdb" && x.Ready {
			t.Errorf("igdb with half its details: %+v", x)
		}
	}
	for _, id := range []string{"libretro", "libretroart", "wikidata", "igdb", "rawg", "tgdb", "screenscraper"} {
		if !found[id] {
			t.Errorf("%s isn't listed", id)
		}
	}
}

func TestGameSourceTestRoute(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	var err error
	ts.GameMeta.Built = func() []gamemeta.Source {
		return []gamemeta.Source{&fakeGameSource{id: "rawg", test: func() (string, error) { return "It works.", err }}}
	}
	// No key: says what's missing, and asks nobody.
	w := ts.do(t, "POST", "/api/gamemeta/sources/rawg/test", admin, map[string]any{"fields": map[string]string{}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":false`) || !strings.Contains(w.Body.String(), "API key") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	// A key typed in, not saved.
	w = ts.do(t, "POST", "/api/gamemeta/sources/rawg/test", admin, map[string]any{"fields": map[string]string{"rawgKey": "typed"}})
	if !strings.Contains(w.Body.String(), `"ok":true`) || !strings.Contains(w.Body.String(), "It works.") {
		t.Errorf("%s", w.Body)
	}
	if got := ts.DB.GetSetting(context.Background(), "rawgKey", ""); got != "" {
		t.Errorf("a test saved %q", got)
	}
	err = gamemeta.ErrKey
	w = ts.do(t, "POST", "/api/gamemeta/sources/rawg/test", admin, map[string]any{"fields": map[string]string{"rawgKey": "typed"}})
	if !strings.Contains(w.Body.String(), `"ok":false`) || !strings.Contains(w.Body.String(), "didn't accept") {
		t.Errorf("%s", w.Body)
	}
	if w := ts.do(t, "POST", "/api/gamemeta/sources/nothing/test", admin, map[string]any{}); w.Code != 404 {
		t.Errorf("%d", w.Code)
	}
}

// A game a profile can't see doesn't exist for it, whichever way it asks. Its rating
// decides that, as a film's does.
func TestBlockedGameBehavesLikeABlockedFilm(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, kid := ts.user(t, "kid", "kids", false, "")
	mature := gameItem(t, ts, "Mature Game", "ESRB M", "")
	young := gameItem(t, ts, "Young Game", "ESRB E", "")
	unrated := gameItem(t, ts, "Unknown Game", "", "")
	for _, target := range []string{
		fmt.Sprintf("/api/items/%d", mature.ID), fmt.Sprintf("/api/items/%d/poster", mature.ID), fmt.Sprintf("/api/games/%d/metadata", mature.ID), fmt.Sprintf("/api/games/%d/art/0", mature.ID),
		fmt.Sprintf("/api/games/%d/metadata", unrated.ID),
	} {
		if w := ts.do(t, "GET", target, kid, nil); w.Code != 404 {
			t.Errorf("%s as a kid: %d", target, w.Code)
		}
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/games/%d/play", mature.ID), kid, map[string]any{}); w.Code != 404 {
		t.Errorf("playing as a kid: %d", w.Code)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/games/%d/metadata", young.ID), kid, nil); w.Code != 200 {
		t.Errorf("a game they may see: %d", w.Code)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/games/%d/metadata", mature.ID), admin, nil); w.Code != 200 {
		t.Errorf("as the owner: %d", w.Code)
	}
	w := ts.do(t, "GET", fmt.Sprintf("/api/items?library=%d&kind=game", mature.LibraryID), kid, nil)
	if strings.Contains(w.Body.String(), "Mature Game") {
		t.Errorf("listed: %s", w.Body)
	}
	w = ts.do(t, "GET", fmt.Sprintf("/api/games/facets?library=%d", mature.LibraryID), kid, nil)
	if w.Code != 200 {
		t.Errorf("facets: %d", w.Code)
	}
	// What a game's file is identified by is for administrators.
	if _, err := ts.Store.DB.Exec(`INSERT INTO game_meta(item_id, library_id, crcs, serial, status) VALUES(?,?,?,?,'done')`, young.ID, young.LibraryID, "deadbeef", "SLUS-00001"); err != nil {
		t.Fatal(err)
	}
	if b := ts.do(t, "GET", fmt.Sprintf("/api/games/%d/metadata", young.ID), kid, nil).Body.String(); strings.Contains(b, "deadbeef") || strings.Contains(b, "SLUS") {
		t.Errorf("a kid sees %s", b)
	}
	if b := ts.do(t, "GET", fmt.Sprintf("/api/games/%d/metadata", young.ID), admin, nil).Body.String(); !strings.Contains(b, "deadbeef") {
		t.Errorf("the owner sees %s", b)
	}
}

func TestFixingAMatch(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	src := &fakeGameSource{id: "rawg", test: func() (string, error) { return "", nil }}
	ts.GameMeta.Built = func() []gamemeta.Source { return []gamemeta.Source{src} }
	ts.DB.SetSetting(context.Background(), "rawgKey", "abc")
	ts.DB.SetSetting(context.Background(), "gameSources", `[{"id":"rawg","on":true}]`)
	it := gameItem(t, ts, "Wrong Name", "", "")
	w := ts.do(t, "GET", fmt.Sprintf("/api/games/%d/metadata/search?q=Right", it.ID), admin, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Found Right") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = ts.do(t, "PUT", fmt.Sprintf("/api/games/%d/metadata/match", it.ID), admin, map[string]any{"source": "rawg", "id": "7"})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"locked":true`) || !strings.Contains(w.Body.String(), `"matchMethod":"manual"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got, _ := ts.Store.Item(context.Background(), it.ID)
	if got.Title != "Chosen Game" || got.ContentRating != "ESRB M" {
		t.Errorf("%+v", got)
	}
	// The rating is live: this game is now hidden from a Kids profile.
	_, kid := ts.user(t, "kid", "kids", false, "")
	if w := ts.do(t, "GET", fmt.Sprintf("/api/games/%d/metadata", it.ID), kid, nil); w.Code != 404 {
		t.Errorf("as a kid: %d", w.Code)
	}
	// Unlock.
	lock := false
	w = ts.do(t, "PUT", fmt.Sprintf("/api/games/%d/metadata/match", it.ID), admin, map[string]any{"lock": &lock})
	if !strings.Contains(w.Body.String(), `"locked":false`) {
		t.Errorf("%s", w.Body)
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/games/%d/metadata/refresh", it.ID), admin, nil); w.Code != 202 {
		t.Errorf("refresh: %d", w.Code)
	}
	if w := ts.do(t, "DELETE", fmt.Sprintf("/api/games/%d/metadata/match", it.ID), admin, nil); w.Code != 200 {
		t.Errorf("no match: %d", w.Code)
	}
	if got, _ := ts.Store.Item(context.Background(), it.ID); got.ContentRating != "" || got.Provider != "" {
		t.Errorf("%+v", got)
	}
	// These are only for games.
	film := ts.item(t, &library.Item{LibraryID: it.LibraryID, Kind: "video", Path: t.TempDir() + "/f.mkv", Title: "Film", SortTitle: "Film"})
	if w := ts.do(t, "GET", fmt.Sprintf("/api/games/%d/metadata", film.ID), admin, nil); w.Code != 404 {
		t.Errorf("a film: %d", w.Code)
	}
}

func TestHandEditedGameFieldsStay(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	it := gameItem(t, ts, "Game", "", "")
	if _, err := ts.Store.DB.Exec(`INSERT INTO game_meta(item_id, library_id, applied) VALUES(?,?,'overview,studio')`, it.ID, it.LibraryID); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "PUT", fmt.Sprintf("/api/items/%d", it.ID), admin, map[string]any{"overview": "Mine"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var edited, applied string
	ts.Store.DB.QueryRow(`SELECT edited, applied FROM game_meta WHERE item_id = ?`, it.ID).Scan(&edited, &applied)
	if edited != "overview" || applied != "studio" {
		t.Errorf("edited %q, applied %q", edited, applied)
	}
}

func TestItemFiltersForGames(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	lib := ts.library(t, "games", t.TempDir())
	add := func(title string, rating float64, year, players int) {
		it := ts.item(t, &library.Item{LibraryID: lib, Kind: "game", Path: t.TempDir() + "/" + title, Title: title, SortTitle: title, Console: "snes", Rating: &rating, Year: &year})
		if _, err := ts.Store.DB.Exec(`INSERT INTO game_meta(item_id, library_id, players, status) VALUES(?,?,?,'done')`, it.ID, lib, players); err != nil {
			t.Fatal(err)
		}
	}
	add("Alpha", 9, 1991, 1)
	add("Beta", 6, 1999, 4)
	add("Gamma", 8, 2004, 2)
	titles := func(q string) []string {
		w := ts.do(t, "GET", fmt.Sprintf("/api/items?library=%d&kind=game&%s", lib, q), admin, nil)
		var out struct {
			Items []library.Item `json:"items"`
		}
		json.Unmarshal(w.Body.Bytes(), &out)
		var t []string
		for _, it := range out.Items {
			t = append(t, it.Title)
		}
		return t
	}
	for q, want := range map[string]string{
		"minRating=8&sort=rating&desc=1": "Alpha,Gamma",
		"decade=1990&sort=year":          "Alpha,Beta",
		"players=2&sort=title":           "Beta,Gamma",
		"sort=year&desc=1":               "Gamma,Beta,Alpha",
	} {
		if got := strings.Join(titles(q), ","); got != want {
			t.Errorf("%s: %s, want %s", q, got, want)
		}
	}
}

// Fixed pictures come through the server; a browser's never sent an outside address.
func TestPictureIsServedByTheServer(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 32)))
	}))
	defer srv.Close()
	ts.GameMeta.Pictures = srv.Client()
	it := gameItem(t, ts, "Game", "", "")
	art, _ := json.Marshal([]gamemeta.ArtRef{{Ref: srv.URL + "/secret-address.png", Kind: "screenshot"}})
	if _, err := ts.Store.DB.Exec(`INSERT INTO game_meta(item_id, library_id, art, status) VALUES(?,?,?,'done')`, it.ID, it.LibraryID, string(art)); err != nil {
		t.Fatal(err)
	}
	w := ts.do(t, "GET", fmt.Sprintf("/api/games/%d/metadata", it.ID), admin, nil)
	if strings.Contains(w.Body.String(), "secret-address") || !strings.Contains(w.Body.String(), fmt.Sprintf("/api/games/%d/art/0", it.ID)) {
		t.Errorf("%s", w.Body)
	}
	for i := 0; i < 2; i++ {
		w = ts.do(t, "GET", fmt.Sprintf("/api/games/%d/art/0", it.ID), admin, nil)
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "image/") {
			t.Fatalf("%d %s", w.Code, w.Header())
		}
	}
	if hits != 1 {
		t.Errorf("fetched %d times", hits)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/games/%d/art/x", it.ID), admin, nil); w.Code != 400 {
		t.Errorf("%d", w.Code)
	}
}
