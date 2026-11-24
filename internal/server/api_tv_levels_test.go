// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"strings"
	"testing"
)

// The card a television shows in place of an editor says what is set now, and only what the
// person asking may already see.
func TestTVSummary(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "sam", "adult", false, "")
	ts.library(t, "movies", t.TempDir())
	ts.library(t, "shows", t.TempDir())

	get := func(id, tok string) tvSummary {
		t.Helper()
		rec := ts.do(t, "GET", "/api/sdui/tv-summary/"+id, tok, nil)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", id, rec.Code, rec.Body.String())
		}
		var out tvSummary
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.Lines == nil || out.Addresses == nil {
			t.Fatalf("%s: lines and addresses are always lists: %s", id, rec.Body.String())
		}
		return out
	}
	if got := get("libraries", admin); len(got.Lines) < 2 || got.Lines[0] != "2 libraries" || !strings.Contains(got.Lines[1], "movies 1") {
		t.Errorf("libraries for the owner: %v", got.Lines)
	}
	if got := get("backup", admin); len(got.Lines) != 1 || !strings.HasPrefix(got.Lines[0], "Automatic backups:") {
		t.Errorf("backup for the owner: %v", got.Lines)
	}
	if got := get("remote", admin); len(got.Lines) == 0 || (got.Lines[0] != "On" && got.Lines[0] != "Off") {
		t.Errorf("remote for the owner: %v", got.Lines)
	}
	if got := get("data-folder", admin); len(got.Lines) != 1 || got.Lines[0] == "" {
		t.Errorf("data-folder for the owner: %v", got.Lines)
	}
	if got := get("metadata", admin); len(got.Lines) != 1 || !strings.Contains(got.Lines[0], "agent") {
		t.Errorf("metadata for the owner: %v", got.Lines)
	}
	for _, id := range []string{"backup", "modules", "transcoding", "network", "remote", "data-folder", "metadata"} {
		if got := get(id, adult); len(got.Lines) != 0 {
			t.Errorf("%s says %v to someone who isn't an administrator", id, got.Lines)
		}
	}
	if got := get("no-such-screen", admin); len(got.Lines) != 0 {
		t.Errorf("an unknown screen: %v", got.Lines)
	}
	if rec := ts.do(t, "GET", "/api/sdui/tv-summary/libraries", "", nil); rec.Code != 401 {
		t.Errorf("signed out: %d", rec.Code)
	}
}
