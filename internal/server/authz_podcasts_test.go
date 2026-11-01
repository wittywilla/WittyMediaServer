package server

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// A subscription none of whose episodes this account may see stays out of sight, as they do.
func TestPodcastsListFollowsItsEpisodes(t *testing.T) {
	ts := newTestServer(t)
	lib := ts.library(t, "podcasts")
	if _, err := ts.DB.Exec(`INSERT INTO podcasts(library_id, url, title, author, description, created_at) VALUES(?,?,?,?,?,1)`, lib, "https://feeds.example.net/a?auth=SECRET", "Late Show", "Someone", "Not for children"); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.DB.Exec(`INSERT INTO podcasts(library_id, url, title, author, description, created_at) VALUES(?,?,?,?,?,1)`, lib, "https://feeds.example.net/b", "Story Time", "Someone", "For everyone"); err != nil {
		t.Fatal(err)
	}
	ts.item(t, &library.Item{LibraryID: lib, Kind: "audio", Path: "https://feeds.example.net/late.mp3", Title: "Late 1", Album: "Late Show"})
	ts.item(t, &library.Item{LibraryID: lib, Kind: "audio", Path: "https://feeds.example.net/story.mp3", Title: "Story 1", Album: "Story Time", ContentRating: "G"})
	_, admin := ts.user(t, "boss", "adult", true, "")
	_, kid := ts.user(t, "kid", "kids", false, "")
	_, adult := ts.user(t, "bob", "adult", false, "")

	if w := ts.do(t, "GET", fmt.Sprintf("/api/items?library=%d", lib), kid, nil); strings.Contains(w.Body.String(), "Late 1") {
		t.Fatalf("the episode itself: %s", w.Body)
	}
	w := ts.do(t, "GET", "/api/podcasts", kid, nil)
	if strings.Contains(w.Body.String(), "Late Show") || strings.Contains(w.Body.String(), "SECRET") {
		t.Errorf("a Kids profile sees a podcast whose episodes it can't: %s", w.Body)
	}
	if !strings.Contains(w.Body.String(), "Story Time") {
		t.Errorf("one it may see went too: %s", w.Body)
	}
	if w := ts.do(t, "GET", "/api/podcasts", adult, nil); !strings.Contains(w.Body.String(), "Late Show") || strings.Contains(w.Body.String(), "SECRET") {
		t.Errorf("an adult with no limits: %s", w.Body)
	}
	if w := ts.do(t, "GET", "/api/podcasts", admin, nil); !strings.Contains(w.Body.String(), "Late Show") || !strings.Contains(w.Body.String(), "SECRET") {
		t.Errorf("an administrator: %s", w.Body)
	}
}
