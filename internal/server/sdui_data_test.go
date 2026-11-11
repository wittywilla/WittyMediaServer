// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/pace"
	"github.com/wittywilla/wittymediaserver/internal/sduistore"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

type sduiFixture struct {
	ts                         *testServer
	admin, oneLibrary, kid     string
	open, secret               int64 // libraries
	family, grownUp, secretOne *library.Item
}

// Two film libraries. "Family Film" (G) and "Grown-up Film" (R) are in the first, "Secret
// Film" in the second. One account may open only the first library; a Kids profile may
// open both but only see what is rated for it.
func newSDUIFixture(t *testing.T) *sduiFixture {
	t.Helper()
	ts := newTestServer(t)
	root := t.TempDir()
	f := &sduiFixture{ts: ts}
	f.open = ts.library(t, "movies", filepath.Join(root, "open"))
	f.secret = ts.library(t, "movies", filepath.Join(root, "secret"))
	mk := func(lib int64, dir, title, rating string) *library.Item {
		p := filepath.Join(root, dir, title+".mkv")
		put(t, p, []byte("film"))
		return ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: p, Title: title, ContentRating: rating, RuntimeSec: 6000, Poster: "cache:x.jpg"})
	}
	f.family = mk(f.open, "open", "Family Film", "G")
	f.grownUp = mk(f.open, "open", "Grown-up Film", "R")
	f.secretOne = mk(f.secret, "secret", "Secret Film", "G")
	_, f.admin = ts.user(t, "owner", auth.KindAdult, true, "")
	_, f.oneLibrary = ts.user(t, "alex", auth.KindAdult, false, fmt.Sprintf(`{"maxAge":-1,"allowUnrated":true,"libraries":[%d]}`, f.open))
	_, f.kid = ts.user(t, "kit", auth.KindKids, false, "")
	return f
}

// A screen sent to an account holds only what that account may see: the same check, on
// every stock screen and on Top Shelf, for a library limit and for a rating limit.
func TestSDUIScreensHonourLimits(t *testing.T) {
	f := newSDUIFixture(t)
	ts := f.ts
	paths := []string{
		"/api/sdui/screens/home", "/api/sdui/screens/home?formFactor=tv", "/api/sdui/screens/libraries",
		"/api/sdui/screens/search?q=Film", "/api/sdui/screens/music", "/api/sdui/screens/livetv",
		fmt.Sprintf("/api/sdui/screens/library?library=%d", f.open), "/api/sdui/topshelf",
	}
	for _, p := range paths {
		w := ts.do(t, "GET", p, f.admin, nil)
		if w.Code != 200 {
			t.Fatalf("%s as the administrator: %d %s", p, w.Code, w.Body)
		}
		wantAll := strings.Contains(p, "home") || strings.Contains(p, "search") || strings.Contains(p, "topshelf")
		if wantAll && (!strings.Contains(w.Body.String(), "Secret Film") || !strings.Contains(w.Body.String(), "Grown-up Film")) {
			t.Errorf("%s: the administrator's screen is missing titles: %s", p, w.Body)
		}
		w = ts.do(t, "GET", p, f.oneLibrary, nil)
		if w.Code != 200 {
			t.Fatalf("%s with one library: %d %s", p, w.Code, w.Body)
		}
		if strings.Contains(w.Body.String(), "Secret") {
			t.Errorf("%s shows an account a title from a library it may not open", p)
		}
		if wantAll && !strings.Contains(w.Body.String(), "Family Film") {
			t.Errorf("%s: an allowed title is missing: %s", p, w.Body)
		}
		w = ts.do(t, "GET", p, f.kid, nil)
		if w.Code != 200 {
			t.Fatalf("%s as a Kids profile: %d %s", p, w.Code, w.Body)
		}
		if strings.Contains(w.Body.String(), "Grown-up") {
			t.Errorf("%s shows a Kids profile an R-rated title", p)
		}
	}
	// Asking for the title or the library by number is "not found", as everywhere else.
	for _, p := range []string{
		fmt.Sprintf("/api/sdui/screens/detail?item=%d", f.secretOne.ID),
		fmt.Sprintf("/api/sdui/screens/library?library=%d", f.secret),
		fmt.Sprintf("/api/sdui/screens/detail?item=%d/diff?since=x", f.secretOne.ID),
	} {
		if w := ts.do(t, "GET", p, f.oneLibrary, nil); w.Code != 404 || strings.Contains(w.Body.String(), "Secret") {
			t.Errorf("%s with one library: %d %s", p, w.Code, w.Body)
		}
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/sdui/screens/detail?item=%d", f.grownUp.ID), f.kid, nil); w.Code != 404 {
		t.Errorf("an R-rated title's detail screen for a Kids profile: %d", w.Code)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/sdui/screens/detail?item=%d", f.family.ID), f.kid, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "Family Film") {
		t.Errorf("an allowed title's detail screen: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/sdui/screens/detail?item=999999", f.admin, nil); w.Code != 404 {
		t.Errorf("no such item: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/sdui/screens/library", f.admin, nil); w.Code != 404 {
		t.Errorf("library screen with no library: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/sdui/topshelf", "", nil); w.Code != 401 {
		t.Errorf("Top Shelf signed out: %d", w.Code)
	}
	// A diff is against the account's own renders only: another account's ETag gets the
	// whole document, made for the one asking.
	adminTag := ts.do(t, "GET", "/api/sdui/screens/home", f.admin, nil).Header().Get("ETag")
	w := ts.do(t, "GET", "/api/sdui/screens/home/diff?since="+strings.ReplaceAll(adminTag, `"`, "%22"), f.oneLibrary, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "Secret") || !strings.Contains(w.Body.String(), `"document"`) {
		t.Errorf("diff against another account's render: %d %s", w.Code, w.Body)
	}
}

func TestSDUIForAnAppleTV(t *testing.T) {
	f := newSDUIFixture(t)
	ts := f.ts
	w := ts.do(t, "GET", "/api/sdui/screens/home", f.admin, nil, "X-WMS-Platform", "tvos")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	doc := decode[sdui.Document](t, w)
	if doc.Layout == nil || doc.Layout.FormFactor != "tv" || doc.Layout.Class != "hd" || doc.Layout.SafeArea.Left != 80 || !doc.Layout.FocusEngine {
		t.Fatalf("layout for tvOS: %+v", doc.Layout)
	}
	var card *sdui.Node
	var defaults int
	var walk func(n *sdui.Node)
	walk = func(n *sdui.Node) {
		if n.Type == sdui.Card && card == nil && n.Props["title"] == "Family Film" && strings.HasPrefix(n.ID, "home-libraries:") {
			card = n
		}
		if n.Focus != nil && n.Focus.Default {
			defaults++
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(&doc.Root)
	if card == nil || card.Focus == nil || !card.Focus.Focusable || card.Focus.Order < 1 || card.Action != "openItem" || card.Props["route"] != fmt.Sprintf("/item/%d", f.family.ID) {
		t.Fatalf("a film's card: %+v", card)
	}
	img, _ := card.Props["image"].(map[string]any)
	if img == nil || img["url"] != fmt.Sprintf("/api/items/%d/poster?w=250", f.family.ID) || img["aspect"] != "2/3" {
		t.Errorf("the card's picture for a 1080p Apple TV: %v", img)
	}
	if defaults != 1 {
		t.Errorf("%d nodes with default focus", defaults)
	}
	// A 4K one gets pictures twice the size, and the two renders have different ETags.
	w4 := ts.do(t, "GET", "/api/sdui/screens/home?formFactor=tv&deviceClass=4k", f.admin, nil)
	if !strings.Contains(w4.Body.String(), "poster?w=500") || w4.Header().Get("ETag") == w.Header().Get("ETag") {
		t.Errorf("4K render: %s", w4.Body)
	}
	// The same render again is 304; what the client holds is still right.
	if w := ts.do(t, "GET", "/api/sdui/screens/home", f.admin, nil, "X-WMS-Platform", "tvos", "If-None-Match", w.Header().Get("ETag")); w.Code != 304 {
		t.Errorf("unchanged screen: %d", w.Code)
	}
	// Top Shelf: sectioned, sized for the device, each picture leading into the app.
	top := decode[sdui.TopShelf](t, ts.do(t, "GET", "/api/sdui/topshelf?deviceClass=4k", f.admin, nil))
	if top.Style != "sectioned" || len(top.Sections) == 0 || len(top.Sections) > sdui.MaxTopShelfSections {
		t.Fatalf("top shelf: %+v", top)
	}
	it := top.Sections[0].Items[0]
	if it.Image == nil || it.Image.Width != 808 || !strings.HasPrefix(it.DisplayRoute, "/item/") || !strings.HasPrefix(it.PlayRoute, "/watch/") {
		t.Errorf("top shelf item: %+v %+v", it, it.Image)
	}
	for _, bad := range []string{"/api/sdui/screens/home?deviceClass=8k", "/api/sdui/topshelf?deviceClass=8k", "/api/sdui/screens/home?catalog=0", "/api/sdui/screens/home?catalog=x"} {
		if w := ts.do(t, "GET", bad, f.admin, nil); w.Code != 400 {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
}

// An administrator's edit can't take a stock screen away from clients, or save a screen
// that a TV couldn't move around or that names a colour no theme has.
func TestSDUIConfigCannotBrickClients(t *testing.T) {
	f := newSDUIFixture(t)
	ts := f.ts
	w := ts.do(t, "GET", "/api/sdui/config", f.admin, nil)
	cfg := decode[sduistore.Config](t, w)
	etag := cfg.ETag
	edit := func(fn func(c *sduistore.Config)) []byte {
		c := cfg
		c.Screens = map[string]sdui.Document{}
		for k, v := range cfg.Screens {
			c.Screens[k] = v
		}
		fn(&c)
		b, _ := json.Marshal(c)
		return b
	}
	withHome := func(fn func(root *sdui.Node, d *sdui.Document)) []byte {
		return edit(func(c *sduistore.Config) {
			d := c.Screens["home"]
			d.Root.Children = append([]sdui.Node(nil), d.Root.Children...)
			fn(&d.Root, &d)
			c.Screens["home"] = d
		})
	}
	for name, body := range map[string][]byte{
		"a colour token no theme has": withHome(func(r *sdui.Node, _ *sdui.Document) { r.Style = &sdui.Style{Background: "token:sparkle"} }),
		"a button inside a card": withHome(func(r *sdui.Node, _ *sdui.Document) {
			r.Children = append(r.Children, sdui.Node{Type: sdui.Card, ID: "c", Children: []sdui.Node{{Type: sdui.Text, ID: "t", Props: map[string]any{"text": "x"}, Action: "goHome"}}})
		}),
		"a shelf inside a shelf": withHome(func(r *sdui.Node, _ *sdui.Document) {
			r.Children = append(r.Children, sdui.Node{Type: sdui.Shelf, ID: "a", Props: map[string]any{"title": "A"}, Children: []sdui.Node{{Type: sdui.Shelf, ID: "b", Props: map[string]any{"title": "B"}}}})
		}),
		"a link that leaves the server": withHome(func(_ *sdui.Node, d *sdui.Document) {
			acts := map[string]sdui.Action{}
			for k, v := range d.Actions {
				acts[k] = v
			}
			acts["goHome"] = sdui.Action{Type: sdui.Fetch, Target: "https://evil.example/collect"}
			d.Actions = acts
		}),
	} {
		w := ts.do(t, "PUT", "/api/sdui/config", f.admin, body, "If-Match", etag)
		if w.Code != 400 || !strings.Contains(w.Body.String(), `"issues"`) {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	// Removing home is allowed, and home is still served: from the stock template.
	w = ts.do(t, "PUT", "/api/sdui/config", f.admin, edit(func(c *sduistore.Config) { delete(c.Screens, "home") }), "If-Match", etag)
	if w.Code != 200 {
		t.Fatalf("saving without home: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/sdui/screens/home", f.oneLibrary, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "Family Film") {
		t.Errorf("home after it was removed from the configuration: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/sdui/screens/nosuch", f.admin, nil); w.Code != 404 {
		t.Errorf("a screen nobody made: %d", w.Code)
	}
	// A home whose root is hidden from everyone but administrators can't be rendered for
	// the rest; they get the stock one instead of an error.
	cur := decode[sduistore.Config](t, ts.do(t, "GET", "/api/sdui/config", f.admin, nil))
	home := sdui.HomeTemplate(sdui.HomeData{})
	home.Root.Visibility = &sdui.Condition{Role: "admin"}
	cur.Screens["home"] = home
	body, _ := json.Marshal(cur)
	if w := ts.do(t, "PUT", "/api/sdui/config", f.admin, body, "If-Match", cur.ETag); w.Code != 200 {
		t.Fatalf("saving the admin-only home: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/sdui/screens/home", f.kid, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "Family Film") {
		t.Errorf("home for an account the configured one hides from: %d %s", w.Code, w.Body)
	}
	// And going back a revision restores the first configuration.
	if w := ts.do(t, "POST", "/api/sdui/config/rollback/1", f.admin, nil); w.Code != 200 || len(decode[sduistore.Config](t, w).Screens) != len(sdui.DefaultScreens()) {
		t.Errorf("rollback: %d %s", w.Code, w.Body)
	}
}

// The renders kept for diffs are bounded for the server and for each account, so neither a
// busy client nor many accounts can make the server hold screens without end.
func TestSDUIKeptRendersAreBounded(t *testing.T) {
	ts := newTestServer(t)
	doc := func(n int) sdui.Document {
		d := sdui.Document{Version: 1, Screen: "x", Root: sdui.Node{Type: sdui.Screen, ID: fmt.Sprint("x", n)}}
		d.Meta.ETag = sdui.ETag(d)
		return d
	}
	for i := 0; i < 50; i++ {
		ts.keepRender(1, doc(i))
	}
	if n := ts.sdui.order.Len(); n != maxKeptPerAccount {
		t.Errorf("one account keeps %d renders, want %d", n, maxKeptPerAccount)
	}
	if _, ok := ts.keptRenderFor(1, doc(49).Meta.ETag); !ok {
		t.Error("the newest render was dropped")
	}
	if _, ok := ts.keptRenderFor(1, doc(0).Meta.ETag); ok {
		t.Error("the oldest render was kept")
	}
	for uid := int64(2); uid < 400; uid++ {
		ts.keepRender(uid, doc(int(uid)))
	}
	if n, limit := ts.sdui.order.Len(), keptRendersFor(pace.Machine()); n > limit || n != len(ts.sdui.renders) {
		t.Errorf("%d renders kept (index %d), limit %d", n, len(ts.sdui.renders), limit)
	}
	if _, ok := ts.keptRenderFor(2, doc(399).Meta.ETag); ok {
		t.Error("an account was given another account's render")
	}
}
