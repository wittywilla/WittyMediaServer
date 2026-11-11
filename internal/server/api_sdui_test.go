// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/sduistore"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

func sduiAccounts(t *testing.T) (ts *testServer, admin, adult, kid string) {
	t.Helper()
	ts = newTestServer(t)
	_, admin = ts.user(t, "owner", auth.KindAdult, true, "")
	_, adult = ts.user(t, "alex", auth.KindAdult, false, "")
	_, kid = ts.user(t, "kit", auth.KindKids, false, "")
	return
}

// sduiAdminRoutes are the routes only an administrator may call.
var sduiAdminOnly = []struct{ method, path string }{
	{"GET", "/api/sdui/config"}, {"PUT", "/api/sdui/config"}, {"GET", "/api/sdui/config/history"}, {"GET", "/api/sdui/config/diff?from=1"},
	{"POST", "/api/sdui/config/rollback/1"}, {"GET", "/api/sdui/config/pages"}, {"GET", "/api/sdui/config/pages/home"},
	{"PUT", "/api/sdui/config/pages/home"}, {"DELETE", "/api/sdui/config/pages/home"}, {"GET", "/api/sdui/config/navigation"},
	{"PUT", "/api/sdui/config/navigation"}, {"DELETE", "/api/sdui/config/navigation"}, {"POST", "/api/sdui/validate"},
	{"GET", "/api/sdui/presets"}, {"POST", "/api/sdui/presets/home-minimal/apply"}, {"GET", "/api/sdui/draft"}, {"PUT", "/api/sdui/draft"},
	{"DELETE", "/api/sdui/draft"}, {"POST", "/api/sdui/draft/publish"}, {"POST", "/api/sdui/draft/preview"}, {"GET", "/api/sdui/export"},
	{"POST", "/api/v1/sdui/import"},
	{"PUT", "/api/theme/global"}, {"GET", "/api/theme/modules/wittyplay"}, {"PUT", "/api/theme/modules/wittyplay"}, {"DELETE", "/api/v1/theme/modules/wittyplay"},
}

var sduiForAnyone = []string{"/api/sdui/catalog", "/api/v1/sdui/navigation", "/api/sdui/navigation/personal", "/api/sdui/pages/home", "/api/sdui/pages/home/personal",
	"/api/sdui/pages/home/sections/jump/items", "/api/sdui/topshelf", "/api/theme"}

func TestSDUIRoutesNeedTheRightAccount(t *testing.T) {
	ts, _, adult, kid := sduiAccounts(t)
	for _, c := range sduiAdminOnly {
		if w := ts.do(t, c.method, c.path, "", nil); w.Code != 401 {
			t.Errorf("%s %s signed out: %d", c.method, c.path, w.Code)
		}
		for _, tok := range []string{adult, kid} {
			if w := ts.do(t, c.method, c.path, tok, nil); w.Code != 403 {
				t.Errorf("%s %s as a non-admin: %d", c.method, c.path, w.Code)
			}
		}
	}
	for _, p := range append([]string{"/api/v1/sdui/pages/home/diff?since=x"}, sduiForAnyone...) {
		if w := ts.do(t, "GET", p, "", nil); w.Code != 401 {
			t.Errorf("GET %s signed out: %d", p, w.Code)
		}
	}
	for _, tok := range []string{adult, kid} {
		for _, p := range sduiForAnyone {
			if w := ts.do(t, "GET", p, tok, nil); w.Code != 200 {
				t.Errorf("GET %s for any account: %d %s", p, w.Code, w.Body)
			}
		}
	}
}

func sduiDoc(t *testing.T, w *httptest.ResponseRecorder) sdui.Document {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	doc, err := sdui.Unmarshal(w.Body.Bytes())
	if err != nil {
		t.Fatalf("%v: %s", err, w.Body)
	}
	return doc
}

func TestSDUIPageRendersFor304AndContext(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	w := ts.do(t, "GET", "/api/v1/sdui/pages/home", adult, nil)
	doc := sduiDoc(t, w)
	etag := w.Header().Get("ETag")
	if doc.Page.ID != "home" || doc.Page.Kind != "home" || doc.Root.Type != "Page" || doc.Version != sdui.SchemaVersion || doc.Catalog != sdui.CatalogRevision ||
		etag == "" || doc.Meta.ETag != etag || doc.Meta.Source != "config" || doc.Meta.ConfigRevision != 1 {
		t.Fatalf("home: %+v %+v, ETag %q", doc.Page, doc.Meta, etag)
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "private") {
		t.Errorf("Cache-Control %q", cc)
	}
	if doc.Layout == nil || doc.Layout.FormFactor != "web" || doc.Personal == nil || !doc.Personal.Editable {
		t.Errorf("layout %+v, personal %+v", doc.Layout, doc.Personal)
	}
	// With no libraries, Home is the welcome notice, whose button opens the app's own Settings.
	if len(doc.Root.Children) != 1 || doc.Root.Children[0].Type != "Notice" || doc.Root.Children[0].Action.Type != sdui.ActNative || doc.Root.Children[0].Action.To != "settings" {
		t.Errorf("home on an empty server: %+v", doc.Root.Children)
	}
	if w := ts.do(t, "GET", "/api/sdui/pages/home", adult, nil, "If-None-Match", etag); w.Code != 304 || w.Body.Len() != 0 {
		t.Errorf("If-None-Match: %d, %d bytes", w.Code, w.Body.Len())
	}
	// What the client says it is: the header, and the query over the header.
	tv := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/home", adult, nil, "X-WMS-Platform", "tvos", "X-WMS-Device-Class", "4k"))
	if tv.Layout.FormFactor != "tv" || tv.Layout.Class != "4k" || tv.Layout.Scale != 2 || !tv.Layout.FocusEngine {
		t.Errorf("tvOS by header: %+v", tv.Layout)
	}
	phone := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/home?platform=android&formFactor=tablet&viewport=700x900&capabilities=reducedMotion,touch", adult, nil, "X-WMS-Platform", "tvos"))
	if phone.Layout.FormFactor != "tablet" || phone.Layout.Breakpoint != "medium" || !phone.Layout.ReducedMotion {
		t.Errorf("the query wins over the header: %+v", phone.Layout)
	}
	// An administrator who hasn't finished setting up gets the nudge too.
	_ = ts.DB.SetSetting(t.Context(), "setupDone", "0")
	if got := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/home", admin, nil)); len(got.Root.Children) != 2 || got.Root.Children[0].ID != "setup" {
		t.Errorf("home for an administrator mid-setup: %+v", got.Root.Children)
	}
	for _, bad := range []string{"platform=Windows!", "formFactor=watch", "deviceClass=8k", "catalog=0", "catalog=x", "viewport=wide", "locale=!!", "capabilities=a%20b", "components=%3Cx%3E", "views=" + strings.Repeat("a,", 20)} {
		if w := ts.do(t, "GET", "/api/sdui/pages/home?"+bad, adult, nil); w.Code != 400 {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	for _, p := range []string{"/api/sdui/pages/nosuch", "/api/sdui/pages/library", "/api/sdui/pages/library?library=99", "/api/sdui/pages/detail?item=99", "/api/sdui/pages/home/sections/nosuch/items"} {
		if w := ts.do(t, "GET", p, adult, nil); w.Code != 404 {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	if w := ts.do(t, "GET", "/api/sdui/pages/Bad%20Name", adult, nil); w.Code != 400 {
		t.Errorf("a page name that isn't one: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/sdui/pages/home?preview=nonsense", admin, nil); w.Code != 403 {
		t.Errorf("a preview token nobody made: %d", w.Code)
	}
}

func urlq(s string) string { return strings.NewReplacer(`"`, "%22", "/", "%2F", " ", "%20").Replace(s) }

func TestSDUIDiff(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	first := ts.do(t, "GET", "/api/sdui/pages/home", adult, nil)
	etag := first.Header().Get("ETag")
	if w := ts.do(t, "GET", "/api/sdui/pages/home/diff", adult, nil); w.Code != 400 {
		t.Errorf("diff without since: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/sdui/pages/home/diff?since="+urlq(etag), adult, nil); w.Code != 304 {
		t.Errorf("diff against the current render: %d", w.Code)
	}
	// The page changes: a notice is added to Home.
	home, _ := sdui.StockPage("home")
	home.Sections = append([]sdui.Section{{ID: "news", Component: "Notice", Props: map[string]any{"text": "Film night is on Friday."}}}, home.Sections...)
	cfgTag := ts.do(t, "GET", "/api/sdui/config", admin, nil).Header().Get("ETag")
	if w := ts.do(t, "PUT", "/api/sdui/config/pages/home", admin, home, "If-Match", cfgTag); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	w := ts.do(t, "GET", "/api/sdui/pages/home/diff?since="+urlq(etag), adult, nil)
	d := decode[sduiDiff](t, w)
	if w.Code != 200 || d.Document != nil || len(d.Patches) == 0 || d.Since != etag || d.ETag == etag {
		t.Fatalf("diff: %d %s", w.Code, w.Body)
	}
	old := sduiDoc(t, first)
	patched, err := sdui.Apply(old, d.Patches)
	if err != nil {
		t.Fatal(err)
	}
	now := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/home", adult, nil))
	if sdui.ETag(patched) != now.Meta.ETag {
		a, _ := json.Marshal(patched)
		b, _ := json.Marshal(now)
		t.Errorf("the patches don't give the page as it is now:\n%s\n%s", a, b)
	}
	// A render the server never gave this account: the whole document.
	d = decode[sduiDiff](t, ts.do(t, "GET", "/api/sdui/pages/home/diff?since=W%2F%22sha256-nope%22", adult, nil))
	if d.Document == nil || d.Patches != nil {
		t.Errorf("diff against an unknown render: %+v", d)
	}
}

// The catalogue describes itself well enough to build an editor from: every component an
// administrator can place has a label, a description, an icon, described properties and
// an example that the validator accepts.
func TestSDUICatalogDescribesItself(t *testing.T) {
	ts, _, adult, _ := sduiAccounts(t)
	w := ts.do(t, "GET", "/api/v1/sdui/catalog", adult, nil)
	if w.Code != 200 || w.Header().Get("ETag") == "" {
		t.Fatalf("%d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/sdui/catalog", adult, nil, "If-None-Match", w.Header().Get("ETag")); w.Code != 304 {
		t.Errorf("If-None-Match: %d", w.Code)
	}
	c := decode[sdui.Catalogue](t, w)
	if c.Revision != sdui.CatalogRevision || len(c.Components) < 20 || len(c.Sources) < 20 || len(c.CardStyles) < 12 || len(c.Icons) < 30 || c.Profiles["tv"] == nil {
		t.Fatalf("catalogue: %d components, %d sources, %d card styles", len(c.Components), len(c.Sources), len(c.CardStyles))
	}
	for _, comp := range c.Components {
		if comp.Label == "" || comp.Description == "" || comp.Icon == "" || comp.Category == "" || comp.A11y == "" {
			t.Errorf("%s isn't fully described", comp.Type)
		}
		if !comp.Required && comp.FallbackNote == "" {
			t.Errorf("%s is optional but says nothing of its fallback", comp.Type)
		}
		for _, p := range comp.Props {
			if p.Label == "" || p.Type == "" {
				t.Errorf("%s.%s has no label or type", comp.Type, p.Name)
			}
		}
		if comp.Authored && comp.Example == nil && comp.Type != "Tab" && comp.Type != "ListItem" {
			t.Errorf("%s has no example", comp.Type)
		}
	}
}

func TestSDUINavigation(t *testing.T) {
	f := newSDUIFixture(t)
	ts := f.ts
	nav := func(tok string, hdr ...string) sdui.NavDocument {
		t.Helper()
		w := ts.do(t, "GET", "/api/sdui/navigation", tok, nil, hdr...)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		return decode[sdui.NavDocument](t, w)
	}
	find := func(d sdui.NavDocument, id string) *sdui.NavEntry {
		for i := range d.Items {
			if d.Items[i].ID == id {
				return &d.Items[i]
			}
		}
		return nil
	}
	web := nav(f.admin)
	if e := find(web, "settings"); e == nil || e.Kind != "native" || e.Screen != "settings" || e.Route != "/settings" {
		t.Errorf("Settings is the app's own screen: %+v", e)
	}
	if e := find(web, "home"); e == nil || e.Kind != "page" || e.Page != "home" || e.Route != "/" || e.Label != "Home" || e.Icon != "home" {
		t.Errorf("home: %+v", e)
	}
	if find(web, "lib-1") == nil || find(web, "lib-2") == nil {
		t.Errorf("an entry per library: %+v", web.Items)
	}
	// A library an account may not open isn't in its menu.
	if one := nav(f.oneLibrary); find(one, "lib-1") == nil || find(one, "lib-2") != nil {
		t.Errorf("one library's account: %+v", one.Items)
	}
	// A television has one Libraries entry, not one per library.
	tv := nav(f.admin, "X-WMS-Platform", "tvos")
	if tv.FormFactor != "tv" || find(tv, "lib-1") != nil || find(tv, "all-libraries") == nil || find(tv, "settings").Placement != "end" {
		t.Errorf("tv: %+v", tv.Items)
	}
	// An administrator's navigation can't lose Settings, whatever it says.
	etag := ts.do(t, "GET", "/api/sdui/config", f.admin, nil).Header().Get("ETag")
	custom := sdui.Navigation{Items: []sdui.NavItem{{ID: "home", Type: "page", Page: "home"}, {ID: "films", Type: "library", Library: f.open, Label: "Films"}}}
	w := ts.do(t, "PUT", "/api/sdui/config/navigation", f.admin, custom, "If-Match", etag)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got := nav(f.oneLibrary)
	if len(got.Items) != 3 || got.Items[1].Label != "Films" || got.Items[1].Route != "/library/1" || got.Items[2].Screen != "settings" {
		t.Errorf("a custom navigation: %+v", got.Items)
	}
	hide := sdui.Navigation{Items: []sdui.NavItem{{ID: "settings", Type: "native", Screen: "settings", Placement: map[string]string{"phone": "hidden"}}}}
	if w := ts.do(t, "PUT", "/api/sdui/config/navigation", f.admin, hide, "If-Match", w.Header().Get("ETag")); w.Code != 400 || !strings.Contains(w.Body.String(), "Settings can't be hidden") {
		t.Errorf("hiding Settings: %d %s", w.Code, w.Body)
	}
	// A profile's own order and hidden entries; Settings stays.
	if w := ts.do(t, "PUT", "/api/sdui/navigation/personal", f.oneLibrary, sdui.Personal{Order: []string{"films", "home"}, Hidden: []string{"settings", "home"}}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	mine := nav(f.oneLibrary)
	if len(mine.Items) != 2 || mine.Items[0].ID != "films" || mine.Items[1].ID != "settings" {
		t.Errorf("a profile's own navigation: %+v", mine.Items)
	}
	if other := nav(f.admin); len(other.Items) != 3 {
		t.Errorf("another profile's navigation changed: %+v", other.Items)
	}
}

func TestSDUIPageEditingNeedsIfMatchAndValidates(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	w := ts.do(t, "GET", "/api/sdui/config", admin, nil)
	cfg := decode[sduistore.Config](t, w)
	etag := w.Header().Get("ETag")
	if etag == "" || cfg.ETag != etag || cfg.Revision != 1 || len(cfg.Pages) != 0 || cfg.Version != sduistore.FormatVersion {
		t.Fatalf("config: revision %d, %d pages, etag %q/%q", cfg.Revision, len(cfg.Pages), cfg.ETag, etag)
	}
	night := sdui.Page{ID: "night", Kind: "custom", Title: "Film night", Icon: "star", Sections: []sdui.Section{
		{ID: "hello", Component: "Text", Props: map[string]any{"text": "{greeting}, {user}", "role": "pageTitle"}},
		{ID: "later", Component: "Shelf", Props: map[string]any{"title": "Watch later", "empty": "message", "emptyText": "Nothing saved yet."}, Source: sdui.Source{"type": "watchLater"}}}}
	if w := ts.do(t, "PUT", "/api/sdui/config/pages/night", admin, night); w.Code != 428 {
		t.Errorf("no If-Match: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/sdui/config/pages/night", admin, night, "If-Match", `W/"ui-0-nope"`); w.Code != 412 {
		t.Errorf("stale If-Match: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/sdui/config/pages/other", admin, night, "If-Match", etag); w.Code != 400 {
		t.Errorf("a page saved under another name: %d", w.Code)
	}
	// What is wrong is said in words, with where and what to do about it.
	bad := night
	bad.Sections = []sdui.Section{{ID: "row", Component: "Shelf", Props: map[string]any{"cardStyle": "postr", "titel": "x"}, Source: sdui.Source{"type": "recentlyAdded", "library": "$item"}},
		{ID: "go", Component: "Button", Props: map[string]any{"label": "Out", "to": "https://evil.example/"}}}
	w = ts.do(t, "PUT", "/api/sdui/config/pages/night", admin, bad, "If-Match", etag)
	var refused struct {
		Issues []sdui.Issue `json:"issues"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &refused)
	if w.Code != 400 || len(refused.Issues) != 4 {
		t.Fatalf("an invalid page: %d %s", w.Code, w.Body)
	}
	text := w.Body.String()
	for _, want := range []string{"/pages/night/sections/0/props/cardStyle", `Did you mean \"poster\"?`, `Did you mean \"title\"?`, "$item means nothing on a custom page", "is not a place in the app"} {
		if !strings.Contains(text, want) {
			t.Errorf("the refusal doesn't say %s: %s", want, text)
		}
	}
	if w := ts.do(t, "POST", "/api/sdui/validate", admin, map[string]any{"page": bad}); w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":false`) || !strings.Contains(w.Body.String(), "Did you mean") {
		t.Errorf("validate: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/sdui/validate", admin, map[string]any{"page": night}); w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Errorf("validate a good page: %d %s", w.Code, w.Body)
	}
	w = ts.do(t, "PUT", "/api/sdui/config/pages/night", admin, night, "If-Match", etag)
	saved := decode[sduistore.Config](t, w)
	if w.Code != 200 || saved.Revision != 2 || saved.UpdatedBy != "owner" || w.Header().Get("ETag") != saved.ETag {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	doc := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/night", adult, nil))
	if doc.Page.Route != "/page/night" || doc.Root.Children[0].Props["text"] == "{greeting}, {user}" || !strings.HasSuffix(doc.Root.Children[0].Props["text"].(string), ", alex") ||
		doc.Root.Children[1].State != "empty" || doc.Root.Children[1].Props["emptyText"] != "Nothing saved yet." {
		t.Errorf("the new page: %+v", doc.Root.Children)
	}
	// The editor's list says where each page comes from; deleting a changed page resets it.
	home, _ := sdui.StockPage("home")
	home.Title = "Our Home"
	w = ts.do(t, "PUT", "/api/sdui/config/pages/home", admin, home, "If-Match", saved.ETag)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	list := ts.do(t, "GET", "/api/sdui/config/pages", admin, nil).Body.String()
	for _, want := range []string{`"id":"night","kind":"custom","title":"Film night","icon":"star","source":"custom"`, `"title":"Our Home","icon":"home"`, `"source":"changed"`, `"id":"library-music"`, `"source":"builtin"`} {
		if !strings.Contains(list, want) {
			t.Errorf("the list of pages lacks %s: %s", want, list)
		}
	}
	one := ts.do(t, "GET", "/api/sdui/config/pages/home", admin, nil).Body.String()
	if !strings.Contains(one, `"source":"changed"`) || !strings.Contains(one, `"builtin":{"id":"home"`) || !strings.Contains(one, `"issues":[]`) {
		t.Errorf("one page: %s", one)
	}
	if got := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/home", adult, nil)); got.Page.Title != "Our Home" {
		t.Errorf("the changed home: %q", got.Page.Title)
	}
	w = ts.do(t, "DELETE", "/api/sdui/config/pages/home", admin, nil, "If-Match", w.Header().Get("ETag"))
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	if got := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/home?x=1", adult, nil)); got.Page.Title != "Home" {
		t.Errorf("home after a reset: %q", got.Page.Title)
	}
	if w := ts.do(t, "DELETE", "/api/sdui/config/pages/nosuch", admin, nil, "If-Match", w.Header().Get("ETag")); w.Code != 404 {
		t.Errorf("deleting a page that isn't there: %d", w.Code)
	}
	// The whole configuration at once, strictly.
	for name, body := range map[string]string{
		"not JSON":      `{"version":`,
		"unknown field": `{"version":2,"pages":{},"extra":true}`,
		"script URL":    `{"version":2,"pages":{"x":{"id":"x","kind":"custom","title":"X","sections":[{"id":"i","component":"Image","props":{"src":"javascript:alert(1)"}}]}}}`,
		"bad colour":    `{"version":2,"pages":{},"theme":{"tokens":{"accent":"url(x)"}}}`,
		"unknown token": `{"version":2,"pages":{},"theme":{"tokens":{"sparkle":"#fff"}}}`,
	} {
		if w := ts.do(t, "PUT", "/api/sdui/config", admin, []byte(body), "If-Match", w.Header().Get("ETag")); w.Code != 400 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
}

func TestSDUIDraftPreviewAndPublish(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	_, admin2 := ts.user(t, "second", auth.KindAdult, true, "")
	if w := ts.do(t, "GET", "/api/sdui/draft", admin, nil); w.Code != 404 {
		t.Errorf("no draft yet: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/sdui/draft/preview", admin, nil); w.Code != 404 {
		t.Errorf("a preview of no draft: %d", w.Code)
	}
	// A preset goes into the draft; nobody sees it.
	w := ts.do(t, "POST", "/api/sdui/presets/home-minimal/apply?target=draft", admin, nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	draftTag := w.Header().Get("ETag")
	hello := func(tok, query string) bool {
		t.Helper()
		w := ts.do(t, "GET", "/api/sdui/pages/home"+query, tok, nil)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", query, w.Code, w.Body)
		}
		return strings.Contains(w.Body.String(), `"id":"hello"`)
	}
	if hello(adult, "") || hello(admin, "") {
		t.Error("an unpublished draft is being served")
	}
	if c := decode[sduistore.Config](t, ts.do(t, "GET", "/api/sdui/config", admin, nil)); len(c.Pages) != 0 {
		t.Error("the draft changed the published configuration")
	}
	// The administrator who asks gets a token that shows the draft to them alone.
	var pv struct{ Token, ExpiresAt string }
	_ = json.Unmarshal(ts.do(t, "POST", "/api/sdui/draft/preview", admin, nil).Body.Bytes(), &pv)
	if len(pv.Token) != 32 || pv.ExpiresAt == "" {
		t.Fatalf("preview: %+v", pv)
	}
	if !hello(admin, "?preview="+pv.Token) {
		t.Error("the preview doesn't show the draft")
	}
	if w := ts.do(t, "GET", "/api/sdui/pages/home", admin, nil, "X-WMS-SDUI-Preview", pv.Token); !strings.Contains(w.Body.String(), `"source":"draft"`) {
		t.Errorf("the preview by header: %s", w.Body)
	}
	for _, tok := range []string{adult, admin2} {
		if w := ts.do(t, "GET", "/api/sdui/pages/home?preview="+pv.Token, tok, nil); w.Code != 403 {
			t.Errorf("someone else's preview token: %d", w.Code)
		}
		if w := ts.do(t, "GET", "/api/sdui/navigation?preview="+pv.Token, tok, nil); w.Code != 403 {
			t.Errorf("someone else's preview token, navigation: %d", w.Code)
		}
	}
	// Editing one page of the draft, then publishing it.
	if w := ts.do(t, "GET", "/api/sdui/config/pages/home?target=draft", admin, nil); !strings.Contains(w.Body.String(), `"id":"hello"`) {
		t.Errorf("the draft's page: %s", w.Body)
	}
	if w := ts.do(t, "GET", "/api/sdui/config/diff?from=current&to=draft", admin, nil); !strings.Contains(w.Body.String(), `{"path":"/pages/home","change":"added"}`) {
		t.Errorf("what the draft changes: %s", w.Body)
	}
	if w := ts.do(t, "POST", "/api/sdui/draft/publish", admin, nil, "If-Match", `W/"draft-0-x"`); w.Code != 412 {
		t.Errorf("publishing a draft that moved on: %d", w.Code)
	}
	w = ts.do(t, "POST", "/api/sdui/draft/publish", admin, nil, "If-Match", draftTag)
	if w.Code != 200 || decode[sduistore.Config](t, w).Revision != 2 {
		t.Fatalf("publish: %d %s", w.Code, w.Body)
	}
	if !hello(adult, "?after=publish") {
		t.Error("the published draft isn't served")
	}
	if w := ts.do(t, "GET", "/api/sdui/pages/home?preview="+pv.Token, admin, nil); w.Code != 403 {
		t.Errorf("a preview token after publishing: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/sdui/draft", admin, nil); w.Code != 404 {
		t.Errorf("the draft after publishing: %d", w.Code)
	}
	// A draft can be thrown away.
	if w := ts.do(t, "DELETE", "/api/sdui/config/pages/home?target=draft", admin, nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "DELETE", "/api/sdui/draft", admin, nil); w.Code != 200 {
		t.Errorf("discard: %d", w.Code)
	}
	if !hello(adult, "?after=discard") {
		t.Error("discarding a draft changed what is published")
	}
}

func TestSDUIPresetsExportAndImport(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	w := ts.do(t, "GET", "/api/sdui/presets", admin, nil)
	var gallery struct{ Presets []sdui.Preset }
	_ = json.Unmarshal(w.Body.Bytes(), &gallery)
	pages, sections := 0, 0
	for _, p := range gallery.Presets {
		if p.Label == "" || p.Description == "" || p.Icon == "" {
			t.Errorf("preset %s isn't described", p.ID)
		}
		if p.Page != nil {
			pages++
		}
		if p.Section != nil {
			sections++
		}
	}
	if pages < 6 || sections < 15 {
		t.Errorf("%d page presets and %d section presets", pages, sections)
	}
	if w := ts.do(t, "POST", "/api/sdui/presets/continue-watching/apply?target=draft", admin, nil); w.Code != 400 {
		t.Errorf("applying a section preset: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/sdui/presets/nosuch/apply?target=draft", admin, nil); w.Code != 404 {
		t.Errorf("a preset that isn't there: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/sdui/presets/home-streaming/apply", admin, nil); w.Code != 428 {
		t.Errorf("applying to what is published without If-Match: %d", w.Code)
	}
	etag := ts.do(t, "GET", "/api/sdui/config", admin, nil).Header().Get("ETag")
	if w := ts.do(t, "POST", "/api/sdui/presets/page-photo-frame/apply", admin, nil, "If-Match", etag); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	// One page out, and into the draft of (here, the same) server.
	w = ts.do(t, "GET", "/api/sdui/export?page=photo-frame", admin, nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Disposition"), "wittymedia-page-photo-frame.json") {
		t.Fatalf("export: %d %v", w.Code, w.Header())
	}
	file := w.Body.Bytes()
	renamed := []byte(strings.Replace(string(file), `"id":"photo-frame"`, `"id":"frame-two"`, 1))
	dry := ts.do(t, "POST", "/api/sdui/import?dryRun=1", admin, renamed)
	if dry.Code != 200 || !strings.Contains(dry.Body.String(), `"ok":true`) || !strings.Contains(dry.Body.String(), `{"path":"/pages/frame-two","change":"added"}`) {
		t.Errorf("dry run: %d %s", dry.Code, dry.Body)
	}
	if w := ts.do(t, "GET", "/api/sdui/draft", admin, nil); w.Code != 404 {
		t.Error("a dry run made a draft")
	}
	if w := ts.do(t, "POST", "/api/sdui/import", admin, renamed); w.Code != 200 {
		t.Fatalf("import: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/sdui/pages/frame-two", adult, nil); w.Code != 404 {
		t.Errorf("an imported page is live before it is published: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/sdui/draft/publish", admin, nil); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if w := ts.do(t, "GET", "/api/sdui/pages/frame-two", adult, nil); w.Code != 200 {
		t.Errorf("the imported page: %d %s", w.Code, w.Body)
	}
	// The whole configuration out and back: the same pages.
	whole := ts.do(t, "GET", "/api/sdui/export", admin, nil).Body.Bytes()
	etag = ts.do(t, "GET", "/api/sdui/config", admin, nil).Header().Get("ETag")
	if w := ts.do(t, "DELETE", "/api/sdui/config/pages/frame-two", admin, nil, "If-Match", etag); w.Code != 200 {
		t.Fatal(w.Body)
	}
	w = ts.do(t, "POST", "/api/sdui/import?target=published", admin, whole)
	if w.Code != 428 {
		t.Errorf("importing over what is published without If-Match: %d", w.Code)
	}
	etag = ts.do(t, "GET", "/api/sdui/config", admin, nil).Header().Get("ETag")
	w = ts.do(t, "POST", "/api/sdui/import?target=published", admin, whole, "If-Match", etag)
	if c := decode[sduistore.Config](t, w); w.Code != 200 || len(c.Pages) != 2 {
		t.Errorf("import of a configuration: %d, %d pages", w.Code, len(c.Pages))
	}
	for name, body := range map[string]string{"another program's file": `{"format":"something-else","page":{}}`, "not JSON": `[`, "nothing in it": `{"format":"wittymedia-ui"}`,
		"an invalid page": `{"format":"wittymedia-ui","page":{"id":"x","kind":"custom","title":"","sections":[]}}`} {
		if w := ts.do(t, "POST", "/api/sdui/import", admin, []byte(body)); w.Code != 400 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
}

func TestSDUIHistoryDiffAndRollback(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	etag := ts.do(t, "GET", "/api/sdui/config", admin, nil).Header().Get("ETag")
	home, _ := sdui.StockPage("home")
	home.Title = "Second"
	home.Sections = append(home.Sections[1:], sdui.Section{ID: "extra", Component: "Divider"})
	home.Sections[1].Props["title"] = "Spotlight"
	w := ts.do(t, "PUT", "/api/sdui/config/pages/home", admin, home, "If-Match", etag)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	var h struct {
		Current   int64
		Keep      int
		Revisions []sduistore.Revision
	}
	_ = json.Unmarshal(ts.do(t, "GET", "/api/sdui/config/history", admin, nil).Body.Bytes(), &h)
	if h.Current != 2 || len(h.Revisions) != 2 || !h.Revisions[0].Current || h.Revisions[0].Revision != 2 || len(h.Revisions[0].Pages) != 1 || h.Keep != sduistore.MaxHistory {
		t.Errorf("history: %+v", h)
	}
	d := ts.do(t, "GET", "/api/sdui/config/diff?from=1&to=2", admin, nil).Body.String()
	if !strings.Contains(d, `{"path":"/pages/home","change":"added"}`) {
		t.Errorf("diff 1 to 2: %s", d)
	}
	home.Sections = home.Sections[:len(home.Sections)-1]
	home.Sections[0], home.Sections[1] = home.Sections[1], home.Sections[0]
	if w := ts.do(t, "PUT", "/api/sdui/config/pages/home", admin, home, "If-Match", w.Header().Get("ETag")); w.Code != 200 {
		t.Fatal(w.Body)
	}
	d = ts.do(t, "GET", "/api/sdui/config/diff?from=2", admin, nil).Body.String()
	if !strings.Contains(d, `"removed":["extra"]`) || !strings.Contains(d, `"reordered":true`) {
		t.Errorf("diff 2 to current: %s", d)
	}
	for _, bad := range []string{"from=99", "from=draft"} {
		if w := ts.do(t, "GET", "/api/sdui/config/diff?"+bad, admin, nil); w.Code != 404 {
			t.Errorf("diff %s: %d", bad, w.Code)
		}
	}
	if w := ts.do(t, "GET", "/api/sdui/config/diff?from=x", admin, nil); w.Code != 400 {
		t.Errorf("diff from nonsense: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/sdui/config/rollback/99", admin, nil); w.Code != 404 {
		t.Errorf("rollback to nowhere: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/sdui/config/rollback/1", admin, nil, "If-Match", `W/"stale"`); w.Code != 412 {
		t.Errorf("rollback with a stale If-Match: %d", w.Code)
	}
	back := decode[sduistore.Config](t, ts.do(t, "POST", "/api/sdui/config/rollback/1", admin, nil))
	if back.Revision != 4 || len(back.Pages) != 0 {
		t.Errorf("rollback: revision %d, %d pages", back.Revision, len(back.Pages))
	}
	if got := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/home", adult, nil)); got.Page.Title != "Home" {
		t.Errorf("home after rollback: %q", got.Page.Title)
	}
}

func TestSDUIConfigFileIsInTheDataFolder(t *testing.T) {
	ts, admin, _, _ := sduiAccounts(t)
	ts.do(t, "GET", "/api/sdui/config", admin, nil)
	if _, err := os.Stat(filepath.Join(ts.Cfg.DataDir, sduistore.FileName)); err != nil {
		t.Error(err)
	}
}
