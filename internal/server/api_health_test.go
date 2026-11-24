// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

func TestLibraryHealthRoutes(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "sam", "adult", false, "")
	dir := t.TempDir()
	lib := ts.library(t, "movies", dir)
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	y := 2001
	big := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: write("a.mkv", "x"), Title: "Harbour", Year: &y, Provider: "tmdb", ProviderID: "1", Height: 2160, Container: "matroska", Poster: "cache:a.jpg"})
	small := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: write("b.mkv", "x"), Title: "Harbour", Year: &y, Provider: "tmdb", ProviderID: "1", Height: 720, Container: "matroska", Poster: "cache:b.jpg"})
	broken := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: write("c.mkv", "not a video"), Title: "Broken", Provider: "tmdb", ProviderID: "2", Poster: "cache:c.jpg"})
	write("c.nfo", "<movie><title>Fish and Chips</movie>")

	for _, c := range []struct{ method, path string }{
		{"GET", "/api/health/library"}, {"POST", "/api/health/library/check"}, {"DELETE", "/api/health/library/check"}, {"POST", "/api/health/library/fix"},
	} {
		if rec := ts.do(t, c.method, c.path, "", nil); rec.Code != 401 {
			t.Errorf("%s %s signed out: %d", c.method, c.path, rec.Code)
		}
		if rec := ts.do(t, c.method, c.path, adult, nil); rec.Code != 403 {
			t.Errorf("%s %s not an administrator: %d", c.method, c.path, rec.Code)
		}
	}

	view := func(q string) healthView {
		t.Helper()
		rec := ts.do(t, "GET", "/api/health/library"+q, admin, nil)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", q, rec.Code, rec.Body)
		}
		var v healthView
		if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	count := func(v healthView, id string) int {
		for _, s := range v.Sections {
			if s.ID == id {
				return s.Count
			}
		}
		t.Fatalf("no section %s", id)
		return 0
	}
	v := view("")
	if count(v, library.HealthDuplicates) != 1 || count(v, library.HealthUnreadable) != 1 || count(v, library.HealthNFO) != 0 {
		t.Fatalf("%+v", v.Sections)
	}
	if rec := ts.do(t, "GET", "/api/health/library?section=nope", admin, nil); rec.Code != 400 {
		t.Errorf("an unknown section: %d", rec.Code)
	}
	if rec := ts.do(t, "GET", "/api/health/library?library=999", admin, nil); rec.Code != 404 {
		t.Errorf("an unknown library: %d", rec.Code)
	}

	// The NFO check runs in the background and is found on the page.
	if rec := ts.do(t, "POST", "/api/health/library/check", admin, nil); rec.Code != 202 {
		t.Fatalf("check: %d %s", rec.Code, rec.Body)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if v = view(""); !v.Check.Running && v.Check.At != 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if v = view(""); count(v, library.HealthNFO) != 1 {
		t.Fatalf("nfo: %+v %+v", v.Check, v.Sections)
	}

	// Copies as versions of the better one.
	rec := ts.do(t, "POST", "/api/health/library/fix", admin, map[string]any{"action": "versions", "ids": []int64{big.ID, small.ID}})
	if rec.Code != 200 {
		t.Fatalf("versions: %d %s", rec.Code, rec.Body)
	}
	if count(view(""), library.HealthDuplicates) != 0 {
		t.Fatal("still a duplicate")
	}

	// A file the fake FFprobe can't read stays unreadable, and says so.
	if rec := ts.do(t, "POST", "/api/health/library/fix", admin, map[string]any{"action": "probe", "id": broken.ID}); rec.Code != 422 {
		t.Fatalf("probe: %d %s", rec.Code, rec.Body)
	}

	// Set aside, then brought back.
	v = view("?section=unreadable")
	if len(v.Sections) != 1 || len(v.Sections[0].Rows) != 1 {
		t.Fatalf("%+v", v.Sections)
	}
	key := v.Sections[0].Rows[0].Key
	if rec := ts.do(t, "POST", "/api/health/library/fix", admin, map[string]any{"action": "setaside", "key": key}); rec.Code != 200 {
		t.Fatalf("set aside: %d %s", rec.Code, rec.Body)
	}
	if v = view(""); count(v, library.HealthUnreadable) != 0 || v.SetAside != 1 {
		t.Fatalf("after setting aside: %+v", v)
	}
	if rec := ts.do(t, "POST", "/api/health/library/fix", admin, map[string]any{"action": "bringback"}); rec.Code != 200 {
		t.Fatalf("bring back: %d", rec.Code)
	}
	if v = view(""); count(v, library.HealthUnreadable) != 1 || v.SetAside != 0 {
		t.Fatalf("after bringing back: %+v", v)
	}

	// Forgetting is only for what's gone.
	if rec := ts.do(t, "POST", "/api/health/library/fix", admin, map[string]any{"action": "forget", "target": "item", "id": broken.ID}); rec.Code != 409 {
		t.Fatalf("forgot a title that's there: %d", rec.Code)
	}
	if rec := ts.do(t, "POST", "/api/health/library/fix", admin, map[string]any{"action": "nope"}); rec.Code != 400 {
		t.Errorf("an unknown fix: %d", rec.Code)
	}

	// TV mode's card says the counts, to the owner only.
	rec = ts.do(t, "GET", "/api/sdui/tv-summary/libraryhealth", admin, nil)
	var sum tvSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &sum); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(sum.Lines, "|"), "1 file can't be read") {
		t.Errorf("summary: %v", sum.Lines)
	}
	rec = ts.do(t, "GET", "/api/sdui/tv-summary/libraryhealth", adult, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &sum); err != nil || len(sum.Lines) != 0 {
		t.Errorf("summary for someone else: %v", sum.Lines)
	}
}
