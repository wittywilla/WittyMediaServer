// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/sduistore"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

func editorPage(id, title string) sdui.Page {
	return sdui.Page{ID: id, Kind: "custom", Title: title, Sections: []sdui.Section{{ID: "t", Component: "Text", Props: map[string]any{"text": title}}}}
}

// A menu checked for the draft may name the draft's own pages; checked for what is
// published, it may not.
func TestSDUIValidateNavigationAgainstTheTarget(t *testing.T) {
	ts, admin, _, _ := sduiAccounts(t)
	if w := ts.do(t, "PUT", "/api/sdui/config/pages/night?target=draft", admin, editorPage("night", "Night")); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	nav := map[string]any{"navigation": map[string]any{"items": []map[string]any{
		{"id": "night", "type": "page", "page": "night"}, {"id": "settings", "type": "native", "screen": "settings"}}}}
	cases := []struct {
		query string
		ok    bool
	}{
		{"?target=draft", true},
		{"", false},
		{"?target=published", false},
	}
	for _, tc := range cases {
		w := ts.do(t, "POST", "/api/sdui/validate"+tc.query, admin, nav)
		var got struct{ OK bool }
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		if w.Code != 200 || got.OK != tc.ok {
			t.Errorf("%q: %d ok=%v, want %v: %s", tc.query, w.Code, got.OK, tc.ok, w.Body)
		}
	}
	if w := ts.do(t, "POST", "/api/sdui/validate?target=nonsense", admin, nav); w.Code != 400 {
		t.Errorf("an unknown target: %d", w.Code)
	}
}

// Deleting a page that is in no configuration is 404, published or draft; a built-in page
// that was never changed is already as shipped.
func TestSDUIDeleteAPageThatIsntThere(t *testing.T) {
	ts, admin, _, _ := sduiAccounts(t)
	etag := func() string { return ts.do(t, "GET", "/api/sdui/config", admin, nil).Header().Get("ETag") }
	cases := []struct {
		name, path string
		ifMatch    bool
		code       int
	}{
		{"missing, draft", "/api/sdui/config/pages/nosuch?target=draft", false, 404},
		{"missing, published", "/api/sdui/config/pages/nosuch", true, 404},
		{"built-in, draft", "/api/sdui/config/pages/home?target=draft", false, 200},
		{"built-in, published", "/api/sdui/config/pages/home", true, 200},
	}
	for _, tc := range cases {
		var hdr []string
		if tc.ifMatch {
			hdr = []string{"If-Match", etag()}
		}
		w := ts.do(t, "DELETE", tc.path, admin, nil, hdr...)
		if w.Code != tc.code {
			t.Errorf("%s: %d %s", tc.name, w.Code, w.Body)
		}
		if tc.code == 404 && !strings.Contains(w.Body.String(), "no such page") {
			t.Errorf("%s: %s", tc.name, w.Body)
		}
	}
}

// Publishing a draft keeps what was published after it was begun; a page changed on both
// sides is a 409 naming it, settled with prefer=.
func TestSDUIPublishMergesAndReportsConflicts(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	put := func(path string, body any, hdr ...string) string {
		t.Helper()
		w := ts.do(t, "PUT", path, admin, body, hdr...)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		return w.Header().Get("ETag")
	}
	etag := ts.do(t, "GET", "/api/sdui/config", admin, nil).Header().Get("ETag")
	etag = put("/api/sdui/config/pages/shared", editorPage("shared", "Before"), "If-Match", etag) // revision 2
	put("/api/sdui/config/pages/mine?target=draft", editorPage("mine", "Mine"))
	put("/api/sdui/config/pages/shared?target=draft", editorPage("shared", "Draft's"))
	etag = put("/api/sdui/config/pages/theirs", editorPage("theirs", "Theirs"), "If-Match", etag) // revision 3

	// The diff names the revision the draft was made from, and says it is the draft.
	var diff struct {
		From, To           int64
		FromDraft, ToDraft bool
	}
	_ = json.Unmarshal(ts.do(t, "GET", "/api/sdui/config/diff?from=current&to=draft", admin, nil).Body.Bytes(), &diff)
	if diff.From != 3 || diff.To != 2 || diff.FromDraft || !diff.ToDraft {
		t.Errorf("diff current→draft: %+v", diff)
	}

	// Both changed "shared": refused, naming it.
	etag = put("/api/sdui/config/pages/shared", editorPage("shared", "Published's"), "If-Match", etag) // revision 4
	w := ts.do(t, "POST", "/api/sdui/draft/publish", admin, nil)
	var refused struct {
		Error                         string
		Conflicts                     []string
		BaseRevision, CurrentRevision int64
	}
	_ = json.Unmarshal(w.Body.Bytes(), &refused)
	if w.Code != 409 || len(refused.Conflicts) != 1 || refused.Conflicts[0] != "/pages/shared" || refused.BaseRevision != 2 || refused.CurrentRevision != 4 || !strings.Contains(refused.Error, "/pages/shared") {
		t.Fatalf("a conflict: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/sdui/draft/publish?prefer=sideways", admin, nil); w.Code != 400 {
		t.Errorf("an unknown preference: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/sdui/draft/publish?prefer=published", admin, nil, "If-Match", `W/"draft-0-x"`); w.Code != 412 {
		t.Errorf("If-Match still applies: %d", w.Code)
	}
	w = ts.do(t, "POST", "/api/sdui/draft/publish?prefer=published", admin, nil)
	pub := decode[sduistore.Config](t, w)
	if w.Code != 200 || pub.Pages["mine"].Title != "Mine" || pub.Pages["theirs"].Title != "Theirs" || pub.Pages["shared"].Title != "Published's" {
		t.Fatalf("publish preferring what is published: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/sdui/pages/theirs", adult, nil); w.Code != 200 {
		t.Errorf("the page published while the draft was open was lost: %d", w.Code)
	}
}
