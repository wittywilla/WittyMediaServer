package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// What only an administrator may see stays that way through a collection.
func TestCollectionsDontGiveAwayAddresses(t *testing.T) {
	ts, lib, st := radioFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("audio"))
	})
	it := addStation(t, ts, lib, "Private", st.URL+"/stream?key=SECRETKEY")
	w := ts.do("POST", "/api/collections", ts.adult, map[string]any{"name": "Mine"})
	var c struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &c); w.Code >= 300 || err != nil {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	path := fmt.Sprintf("/api/collections/%d", c.ID)
	if w := ts.do("POST", path+"/items", ts.adult, map[string]any{"items": []int64{it.ID}}); w.Code >= 300 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got := ts.do("GET", path, ts.adult, nil)
	if got.Code != 200 || !strings.Contains(got.Body.String(), "Private") {
		t.Fatalf("%d %s", got.Code, got.Body)
	}
	if strings.Contains(got.Body.String(), "SECRETKEY") || strings.Contains(got.Body.String(), st.URL) {
		t.Errorf("the station's address came through: %s", got.Body)
	}
}

func TestRecentGamesHideTheFolder(t *testing.T) {
	g := newGameServer(t)
	item := runnerGameItem(t, g)
	if w := g.do("PUT", fmt.Sprintf("/api/games/%d/meta", item), g.other, map[string]any{"favourite": true}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w := g.do("GET", "/api/games/recent?favourites=1", g.other, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"id"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "Game.wittytest") {
		t.Errorf("the path came through: %s", w.Body)
	}
}

// A station's address can hold a private key: listeners never get it, and nor does the log.
func TestRadioPlaylistsDontGiveAwayAddresses(t *testing.T) {
	var ts *galleryServer
	var lib *library.Library
	var st *httptest.Server
	ts, lib, st = radioFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/live.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			fmt.Fprint(w, "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nSECRETPATH/seg1.aac?key=SECRETKEY\n")
		default:
			w.Header().Set("Content-Type", "audio/aac")
			_, _ = w.Write([]byte("segment"))
		}
	})
	it := addStation(t, ts, lib, "Live", st.URL+"/live.m3u8")
	w := ts.do("GET", fmt.Sprintf("/api/radio/%d/stream", it.ID), ts.adult, nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	body := w.Body.String()
	m := regexp.MustCompile(`hls\?u=([A-Za-z0-9_-]+)`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no part in %s", body)
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "SECRET") {
			t.Errorf("the address shows: %s", line)
		}
	}
	if strings.Contains(m[1], "aHR0c") { // the start of "http" in base64
		t.Errorf("that's only encoded: %s", m[1])
	}
	seg := ts.do("GET", fmt.Sprintf("/api/radio/%d/hls?u=%s", it.ID, m[1]), ts.adult, nil)
	if seg.Code != 200 || seg.Body.String() != "segment" {
		t.Errorf("the part doesn't play: %d %s", seg.Code, seg.Body)
	}
	// It means nothing for another station, and can't be altered.
	other := addStation(t, ts, lib, "Other", st.URL+"/other")
	if w := ts.do("GET", fmt.Sprintf("/api/radio/%d/hls?u=%s", other.ID, m[1]), ts.adult, nil); w.Code != 403 {
		t.Errorf("another station: %d", w.Code)
	}
	if w := ts.do("GET", fmt.Sprintf("/api/radio/%d/hls?u=A%s", it.ID, m[1]), ts.adult, nil); w.Code != 403 {
		t.Errorf("altered: %d", w.Code)
	}
	if got := redactQuery("u=" + m[1] + "&st=abc"); strings.Contains(got, m[1]) || strings.Contains(got, "abc") {
		t.Errorf("the log keeps it: %s", got)
	}
}

func TestStationErrorsDontQuoteTheAddress(t *testing.T) {
	_, err := library.OpenStation(context.Background(), "http://127.0.0.1:1/SECRETPATH?key=SECRETKEY")
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("%v", err)
	}
}

func wantImport(t *testing.T, g *gameServer, method, path string, body []byte, want int, words string) {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	r.RemoteAddr = "192.168.1.50:4000"
	r.Header.Set("Authorization", "Bearer "+g.other)
	w := httptest.NewRecorder()
	g.h.ServeHTTP(w, r)
	if w.Code != want || !strings.Contains(w.Body.String(), words) {
		t.Errorf("%s %s: %d %s", method, path, w.Code, w.Body)
	}
}

// What the limit says is the limit: larger than the server's general limit on a request.
func TestImportsTakeWhatTheirLimitSays(t *testing.T) {
	g := newGameServer(t)
	item := runnerGameItem(t, g)
	big := make([]byte, 33<<20)
	copy(big, "PATCH")
	copy(big[len(big)-3:], "EOF")
	wantImport(t, g, "PUT", fmt.Sprintf("/api/games/%d/saves/1/file", item), big, 200, "")
	wantImport(t, g, "PUT", fmt.Sprintf("/api/games/%d/battery", item), big, 200, "")
	if !bigUploadPath(fmt.Sprintf("/api/games/%d/patches", item)) {
		t.Error("patches are held to the general limit")
	}
	// The general limit still holds everywhere else.
	if bigUploadPath("/api/items/1/file") || bigUploadPath("/api/games/1/meta") {
		t.Error("that's too wide")
	}
}

func TestAccountsHaveALimitAcrossTheirGames(t *testing.T) {
	g := newGameServer(t)
	item := runnerGameItem(t, g)
	old := maxAccountGameBytes
	maxAccountGameBytes = 2 << 30
	defer func() { maxAccountGameBytes = old }()

	u := g.do("GET", "/api/me", g.other, nil)
	var me struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(u.Body.Bytes(), &me); err != nil || me.ID == 0 {
		t.Fatalf("%d %s", u.Code, u.Body)
	}
	other := filepath.Join(g.s.games().saveDir(me.ID, 99999), "auto.state")
	if err := os.MkdirAll(filepath.Dir(other), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(other)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(2 << 30); err != nil {
		t.Fatal(err)
	}
	f.Close()
	wantImport(t, g, "PUT", fmt.Sprintf("/api/games/%d/battery", item), []byte("data"), 409, "2 GB")
	wantImport(t, g, "PUT", fmt.Sprintf("/api/games/%d/saves/1/file", item), []byte("data"), 409, "2 GB")
	wantImport(t, g, "POST", fmt.Sprintf("/api/games/%d/patches", item), []byte("PATCHEOF"), 409, "2 GB")
}

// Requests arriving together can't all take the last place.
func TestPatchLimitHoldsWithManyAtOnce(t *testing.T) {
	g := newGameServer(t)
	item := runnerGameItem(t, g)
	ctx := context.Background()
	var wg sync.WaitGroup
	var mu sync.Mutex
	added, refused := 0, 0
	for i := 0; i < maxGamePatches*2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := g.s.Store.AddGamePatch(ctx, 1, item, library.GamePatch{Name: "p", Kind: "ips"}, maxGamePatches)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				added++
			case errors.Is(err, library.ErrTooManyPatches):
				refused++
			default:
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if added != maxGamePatches || refused != maxGamePatches {
		t.Errorf("%d added, %d refused", added, refused)
	}
}
