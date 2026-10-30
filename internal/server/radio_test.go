package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

func TestRadioRelay(t *testing.T) {
	ts := newGalleryServer(t)
	ctx := context.Background()
	lib, err := ts.s.Store.CreateLibrary(ctx, "Radio", "radio", nil)
	if err != nil {
		t.Fatal(err)
	}
	station := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/audio":
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("mp3 bytes"))
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<script>alert(1)</script>"))
		default:
			w.WriteHeader(404)
		}
	}))
	defer station.Close()
	add := func(path string) string {
		t.Helper()
		it, err := ts.s.Store.AddRadioStation(ctx, lib.ID, library.RadioStation{Name: path, URL: station.URL + path})
		if err != nil {
			t.Fatal(err)
		}
		return itoa(it.ID)
	}
	audio, page, gone := add("/audio"), add("/page"), add("/gone")

	// The real client refuses this machine and its network.
	if w := ts.do("GET", "/api/radio/"+audio+"/stream", ts.adult, nil); w.Code != 502 {
		t.Errorf("a station on this machine was reached: %d %s", w.Code, w.Body)
	}
	real := library.SafeClient
	library.SafeClient = station.Client() // from here on, the test's station is reachable
	defer func() { library.SafeClient = real }()

	w := ts.do("GET", "/api/radio/"+audio+"/stream", ts.adult, nil)
	if w.Code != 200 || w.Body.String() != "mp3 bytes" || w.Header().Get("Content-Type") != "audio/mpeg" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("relay: %d %q %v", w.Code, w.Body, w.Header())
	}
	if w := ts.do("GET", "/api/radio/"+page+"/stream", ts.adult, nil); w.Code != 502 || strings.Contains(w.Body.String(), "<script>") {
		t.Errorf("a web page came through: %d %s", w.Code, w.Body)
	}
	if w := ts.do("GET", "/api/radio/"+gone+"/stream", ts.adult, nil); w.Code != 502 {
		t.Errorf("a station that answers 404: %d", w.Code)
	}
	// Who may do what.
	if w := ts.do("GET", "/api/radio/"+audio+"/stream", "", nil); w.Code != 401 {
		t.Errorf("no account: %d", w.Code)
	}
	if w := ts.do("GET", "/api/radio/"+audio+"/stream", ts.outside, nil); w.Code != 404 {
		t.Errorf("an account without this library: %d", w.Code)
	}
	if w := ts.do("POST", "/api/radio", ts.adult, map[string]any{"libraryId": lib.ID, "url": "https://x.example/"}); w.Code != 403 {
		t.Errorf("a non-admin added a station: %d", w.Code)
	}
	if w := ts.do("DELETE", "/api/radio/"+audio, ts.adult, nil); w.Code != 403 {
		t.Errorf("a non-admin removed a station: %d", w.Code)
	}
	// Listening counts for Your year.
	var plays int
	_ = ts.s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM item_stats WHERE item_id = ?`, audio).Scan(&plays)
	if plays == 0 {
		t.Error("listening wasn't recorded")
	}
}
