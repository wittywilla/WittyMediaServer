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
		year := 1999
		return ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: p, Title: title, ContentRating: rating, RuntimeSec: 6000, Poster: "cache:x.jpg", Backdrop: "cache:b.jpg", Year: &year, Genres: []string{"Adventure"},
			Overview: "What " + title + " is about."})
	}
	f.family = mk(f.open, "open", "Family Film", "G")
	f.grownUp = mk(f.open, "open", "Grown-up Film", "R")
	f.secretOne = mk(f.secret, "secret", "Secret Film", "G")
	_, f.admin = ts.user(t, "owner", auth.KindAdult, true, "")
	_, f.oneLibrary = ts.user(t, "alex", auth.KindAdult, false, fmt.Sprintf(`{"maxAge":-1,"allowUnrated":true,"libraries":[%d]}`, f.open))
	_, f.kid = ts.user(t, "kit", auth.KindKids, false, "")
	return f
}

// A page sent to an account holds only what that account may see: the same check, on
// every built-in page, on a section's own pages of items and on Top Shelf, for a library
// limit and for a rating limit.
func TestSDUIPagesHonourLimits(t *testing.T) {
	f := newSDUIFixture(t)
	ts := f.ts
	paths := []string{
		"/api/sdui/pages/home", "/api/sdui/pages/home?formFactor=tv", "/api/sdui/pages/libraries", "/api/sdui/pages/collections",
		"/api/sdui/pages/search?q=Film", "/api/sdui/pages/music", "/api/sdui/pages/livetv", "/api/sdui/pages/photos", "/api/sdui/pages/books", "/api/sdui/pages/games",
		fmt.Sprintf("/api/sdui/pages/library?library=%d", f.open), fmt.Sprintf("/api/sdui/pages/library-movies?library=%d", f.open),
		fmt.Sprintf("/api/sdui/pages/home/sections/recent-%d/items", f.open), fmt.Sprintf("/api/sdui/pages/library/sections/all/items?library=%d&cursor=0", f.open),
		fmt.Sprintf("/api/sdui/pages/detail?item=%d", f.family.ID), "/api/sdui/topshelf", "/api/sdui/navigation",
	}
	for _, p := range paths {
		w := ts.do(t, "GET", p, f.admin, nil)
		if w.Code != 200 {
			t.Fatalf("%s as the administrator: %d %s", p, w.Code, w.Body)
		}
		wantAll := strings.Contains(p, "pages/home") && !strings.Contains(p, "sections") || strings.Contains(p, "search") || strings.Contains(p, "topshelf")
		if wantAll && (!strings.Contains(w.Body.String(), "Secret Film") || !strings.Contains(w.Body.String(), "Grown-up Film")) {
			t.Errorf("%s: the administrator's page is missing titles: %s", p, w.Body)
		}
		w = ts.do(t, "GET", p, f.oneLibrary, nil)
		if w.Code != 200 {
			t.Fatalf("%s with one library: %d %s", p, w.Code, w.Body)
		}
		if strings.Contains(w.Body.String(), "Secret") || strings.Contains(w.Body.String(), fmt.Sprintf("/library/%d", f.secret)) {
			t.Errorf("%s shows an account something from a library it may not open", p)
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
	// Asking for the title or the library by number is "not found", as everywhere else:
	// the page, its diff and its sections alike.
	for _, p := range []string{
		fmt.Sprintf("/api/sdui/pages/detail?item=%d", f.secretOne.ID),
		fmt.Sprintf("/api/sdui/pages/library?library=%d", f.secret),
		fmt.Sprintf("/api/sdui/pages/library-movies?library=%d", f.secret),
		fmt.Sprintf("/api/sdui/pages/library/diff?since=x&library=%d", f.secret),
		fmt.Sprintf("/api/sdui/pages/library/sections/all/items?library=%d", f.secret),
		fmt.Sprintf("/api/sdui/pages/detail/sections/similar/items?item=%d", f.secretOne.ID),
	} {
		if w := ts.do(t, "GET", p, f.oneLibrary, nil); w.Code != 404 || strings.Contains(w.Body.String(), "Secret") {
			t.Errorf("%s with one library: %d %s", p, w.Code, w.Body)
		}
	}
	// A row repeated over libraries has no copy for a library the account can't open, and
	// asking for that copy by name finds nothing.
	if w := ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/home/sections/recent-%d/items", f.secret), f.oneLibrary, nil); w.Code != 404 {
		t.Errorf("a repeated row's copy for a library that isn't theirs: %d %s", w.Code, w.Body)
	}
	// A page an administrator wrote can name a library outright; the account's limits
	// still decide what comes back.
	etag := ts.do(t, "GET", "/api/sdui/config", f.admin, nil).Header().Get("ETag")
	leak := sdui.Page{ID: "leak", Kind: "custom", Title: "Everything", Sections: []sdui.Section{
		{ID: "theirs", Component: "Shelf", Props: map[string]any{"title": "Theirs", "empty": "message"}, Source: sdui.Source{"type": "recentlyAdded", "library": f.secret}},
		{ID: "all", Component: "Grid", Props: map[string]any{"empty": "message"}, Source: sdui.Source{"type": "browse", "sort": "title"}}}}
	if w := ts.do(t, "PUT", "/api/sdui/config/pages/leak", f.admin, leak, "If-Match", etag); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if w := ts.do(t, "GET", "/api/sdui/pages/leak", f.oneLibrary, nil); w.Code != 200 || strings.Contains(w.Body.String(), "Secret") || !strings.Contains(w.Body.String(), "Family Film") {
		t.Errorf("a page naming a library the account can't open: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/sdui/pages/leak", f.kid, nil); strings.Contains(w.Body.String(), "Grown-up") || !strings.Contains(w.Body.String(), "Secret Film") {
		t.Errorf("the same page for a Kids profile: %s", w.Body)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/detail?item=%d", f.grownUp.ID), f.kid, nil); w.Code != 404 {
		t.Errorf("an R-rated title's page for a Kids profile: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/sdui/topshelf", "", nil); w.Code != 401 {
		t.Errorf("Top Shelf signed out: %d", w.Code)
	}
	// A diff is against the account's own renders only: another account's ETag gets the
	// whole document, made for the one asking.
	adminTag := ts.do(t, "GET", "/api/sdui/pages/home", f.admin, nil).Header().Get("ETag")
	w := ts.do(t, "GET", "/api/sdui/pages/home/diff?since="+strings.ReplaceAll(adminTag, `"`, "%22"), f.oneLibrary, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "Secret") || !strings.Contains(w.Body.String(), `"document"`) {
		t.Errorf("diff against another account's render: %d %s", w.Code, w.Body)
	}
}

func findNode(n *sdui.Node, id string) *sdui.Node {
	if n.ID == id {
		return n
	}
	for i := range n.Children {
		if got := findNode(&n.Children[i], id); got != nil {
			return got
		}
	}
	return nil
}

// Home and a film's page, as an Apple TV and a browser get them from a real library.
func TestSDUIBuiltInPagesFromALibrary(t *testing.T) {
	f := newSDUIFixture(t)
	ts := f.ts
	tv := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/home", f.admin, nil, "X-WMS-Platform", "tvos"))
	if tv.Layout.SafeArea.Left != 80 || tv.Layout.Budget.ShelfItems != 12 || tv.Layout.Text["body"] != 29 {
		t.Errorf("the tv profile: %+v", tv.Layout)
	}
	row := findNode(&tv.Root, fmt.Sprintf("recent-%d", f.open))
	if row == nil || row.Type != "Shelf" || row.Props["title"] != "Recently added in movies" || len(row.Children) != 2 {
		t.Fatalf("the row for the first library: %+v", row)
	}
	card := row.Children[0]
	im, _ := card.Props["image"].(map[string]any)
	if card.Type != "Card" || card.Props["style"] != "poster" || card.Action == nil || card.Action.Type != sdui.ActNavigate || !strings.HasPrefix(card.Action.To, "/item/") ||
		im == nil || !strings.HasSuffix(im["url"].(string), "/poster?w=320") || card.Focus == nil || card.Focus.Group != row.ID {
		t.Errorf("a card on a television: %+v focus %+v", card, card.Focus)
	}
	if findNode(&tv.Root, "libraries") != nil {
		t.Error("the libraries row is for devices without a Libraries tab")
	}
	web := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/home", f.admin, nil))
	if tiles := findNode(&web.Root, "libraries"); tiles == nil || len(tiles.Children) != 2 || tiles.Children[0].Props["style"] != "tile" || tiles.Children[0].Props["icon"] != "movie" {
		t.Errorf("the libraries row in a browser: %+v", tiles)
	}
	if hero := findNode(&web.Root, "hero"); hero == nil || len(hero.Children) != 3 || hero.Children[0].Props["style"] != "hero" {
		t.Errorf("the spotlight: %+v", hero)
	}
	// A film's page: the header and what it offers this account.
	detail := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/detail?item=%d", f.family.ID), f.kid, nil, "X-WMS-Platform", "ios"))
	h := findNode(&detail.Root, "header")
	if detail.Page.ID != "detail" || detail.Page.Title != "Family Film" || detail.Page.Route != fmt.Sprintf("/item/%d", f.family.ID) || h == nil || h.Type != "DetailHeader" ||
		h.Props["title"] != "Family Film" || h.Props["overview"] != "What Family Film is about." || h.Focus == nil || !h.Focus.Default {
		t.Fatalf("a film's page: %+v %+v", detail.Page, h)
	}
	acts, _ := json.Marshal(h.Props["actions"])
	if !strings.Contains(string(acts), `"id":"play"`) || !strings.Contains(string(acts), `"primary":true`) || !strings.Contains(string(acts), `"op":"watched.set"`) {
		t.Errorf("the header's buttons: %s", acts)
	}
	// A profile that may not download or share is offered neither.
	_, strict := ts.user(t, "sam", auth.KindAdult, false, `{"maxAge":-1,"allowUnrated":true,"off":["downloads","sharing"]}`)
	sd := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/detail?item=%d", f.family.ID), strict, nil))
	a2, _ := json.Marshal(findNode(&sd.Root, "header").Props["actions"])
	if strings.Contains(string(a2), "download") || strings.Contains(string(a2), "share") || !strings.Contains(string(acts), `"id":"download"`) {
		t.Errorf("buttons for a profile without downloads or sharing: %s (with: %s)", a2, acts)
	}
	// More like this: the other film of the same genre in the same library, not itself,
	// and not the one in the other library.
	asAdmin := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/detail?item=%d", f.family.ID), f.admin, nil))
	sim := findNode(&asAdmin.Root, "similar")
	if sim == nil || len(sim.Children) != 1 || sim.Children[0].Props["title"] != "Grown-up Film" {
		t.Errorf("more like this: %+v", sim)
	}
	// A film library: the filter bar's choices come from the library, and choosing one
	// narrows the grid.
	lib := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/library?library=%d", f.open), f.admin, nil))
	bar, grid := findNode(&lib.Root, "filters"), findNode(&lib.Root, "all")
	if lib.Page.ID != "library-movies" || lib.Page.Title != "movies" || bar == nil || grid == nil || len(grid.Children) != 2 || !strings.Contains(fmt.Sprint(bar.Props["fields"]), "Adventure") {
		t.Fatalf("a film library: %+v bar %+v", lib.Page, bar)
	}
	if n, _ := bar.Props["count"].(float64); n != 2 {
		t.Errorf("the filter bar's count: %v", bar.Props["count"])
	}
	none := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/library?library=%d&genre=Western", f.open), f.admin, nil))
	if g := findNode(&none.Root, "all"); g == nil || g.State != "empty" || none.Page.Params["genre"] != "Western" {
		t.Errorf("a filter that matches nothing: %+v", g)
	}
	// Search finds by title, in each library the account can open.
	found := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/search?q=grown", f.admin, nil))
	if r := findNode(&found.Root, fmt.Sprintf("found-%d", f.open)); r == nil || len(r.Children) != 1 || findNode(&found.Root, "q").Props["query"] != "grown" {
		t.Errorf("search: %+v", found.Root.Children)
	}
}

// A row past the device's budget arrives empty with where to ask; asking gives its cards,
// and the page after them.
func TestSDUISectionsPageByCursor(t *testing.T) {
	f := newSDUIFixture(t)
	ts := f.ts
	root := t.TempDir()
	for i := range 40 {
		p := filepath.Join(root, fmt.Sprintf("Film %02d.mkv", i))
		put(t, p, []byte("film"))
		ts.item(t, &library.Item{LibraryID: f.open, Kind: "video", Path: p, Title: fmt.Sprintf("Film %02d", i), ContentRating: "G", RuntimeSec: 6000})
	}
	doc := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/library?library=%d&sort=title", f.open), f.oneLibrary, nil, "X-WMS-Platform", "tvos"))
	grid := findNode(&doc.Root, "all")
	more, _ := grid.Props["more"].(map[string]any)
	if len(grid.Children) != 36 || more == nil || grid.Props["columns"] != float64(6) {
		t.Fatalf("the first page: %d cards, more %v, columns %v", len(grid.Children), more, grid.Props["columns"])
	}
	next, _ := more["url"].(string)
	if !strings.HasPrefix(next, "/api/v1/sdui/pages/library-movies/sections/all/items?") || !strings.Contains(next, "cursor=36") || !strings.Contains(next, "sort=title") {
		t.Fatalf("where the next page is: %q", next)
	}
	w := ts.do(t, "GET", next, f.oneLibrary, nil, "X-WMS-Platform", "tvos")
	var page sdui.SectionItems
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if len(page.Items) != 6 || page.Next != "" || page.Items[0].ID == grid.Children[35].ID || page.Section != "all" {
		t.Errorf("the second page: %d items, next %q", len(page.Items), page.Next)
	}
	if w := ts.do(t, "GET", next, f.oneLibrary, nil, "X-WMS-Platform", "tvos", "If-None-Match", w.Header().Get("ETag")); w.Code != 304 {
		t.Errorf("a page of items the client has: %d", w.Code)
	}
	for _, bad := range []string{"cursor=-1", "cursor=x", "season=x"} {
		if w := ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/library/sections/all/items?library=%d&%s", f.open, bad), f.oneLibrary, nil); w.Code != 400 && bad != "cursor=x" {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	// A page of many rows: the ones past the budget are deferred.
	var secs []sdui.Section
	for i := range 8 {
		secs = append(secs, sdui.Section{ID: fmt.Sprintf("row%d", i), Component: "Shelf", Props: map[string]any{"title": fmt.Sprintf("Row %d", i)}, Source: sdui.Source{"type": "recentlyAdded", "library": f.open}})
	}
	etag := ts.do(t, "GET", "/api/sdui/config", f.admin, nil).Header().Get("ETag")
	if w := ts.do(t, "PUT", "/api/sdui/config/pages/rows", f.admin, sdui.Page{ID: "rows", Kind: "custom", Title: "Rows", Sections: secs}, "If-Match", etag); w.Code != 200 {
		t.Fatal(w.Body)
	}
	rows := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/rows", f.oneLibrary, nil, "X-WMS-Platform", "tvos"))
	if a, b := rows.Root.Children[3], rows.Root.Children[4]; a.State != "" || len(a.Children) != 12 || b.State != "deferred" || len(b.Children) != 0 {
		t.Fatalf("rows 4 and 5 on an Apple TV HD: %q with %d cards, %q with %d", a.State, len(a.Children), b.State, len(b.Children))
	}
	if n := len(rows.Root.Children); n != 8 {
		t.Errorf("%d rows", n)
	}
	first, _ := rows.Root.Children[4].Props["more"].(map[string]any)["url"].(string)
	w = ts.do(t, "GET", first, f.oneLibrary, nil, "X-WMS-Platform", "tvos")
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	if w.Code != 200 || len(page.Items) != 12 || page.Props["title"] != "Row 4" || !strings.Contains(page.Next, "cursor=12") {
		t.Errorf("a deferred row's first page: %d %d items, props %v, next %q", w.Code, len(page.Items), page.Props, page.Next)
	}
}

// A profile arranges a page for itself; nobody else's changes, a profile that may not
// can't, and the order the web app's Home kept before carries over.
func TestSDUIPersonalisation(t *testing.T) {
	f := newSDUIFixture(t)
	ts := f.ts
	order := func(tok string) string {
		t.Helper()
		doc := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/home?n="+tok[:6], tok, nil))
		var ids []string
		for _, n := range doc.Root.Children {
			ids = append(ids, n.ID)
		}
		return strings.Join(ids, " ")
	}
	before := order(f.oneLibrary)
	if !strings.HasPrefix(before, "hero libraries recent-1 shuffle") {
		t.Fatalf("home as shipped: %s", before)
	}
	mine := sdui.Personal{Order: []string{"shuffle", "recent-1"}, Hidden: []string{"hero"}, Pinned: []string{"libraries"}, Density: "compact", CardSize: map[string]string{"recent-1": "l"}}
	if w := ts.do(t, "PUT", "/api/sdui/pages/home/personal", f.oneLibrary, mine); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if got := order(f.oneLibrary); got != "libraries shuffle recent-1" {
		t.Errorf("home as arranged: %s", got)
	}
	doc := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/home", f.oneLibrary, nil))
	if doc.Layout.Density != "compact" || findNode(&doc.Root, "recent-1").Props["cardSize"] != "l" || !doc.Personal.Sections[0].Pinned || doc.Personal.Density != "compact" {
		t.Errorf("density and card size: %+v", doc.Personal)
	}
	hidden := false
	for _, s := range doc.Personal.Sections {
		hidden = hidden || (s.ID == "hero" && s.Hidden)
	}
	if !hidden {
		t.Errorf("a hidden section isn't listed so it can be shown again: %+v", doc.Personal.Sections)
	}
	if got := order(f.admin); !strings.HasPrefix(got, "hero libraries") {
		t.Errorf("another profile's home changed: %s", got)
	}
	if w := ts.do(t, "GET", "/api/sdui/pages/home/personal", f.oneLibrary, nil); !strings.Contains(w.Body.String(), `"editable":true`) || !strings.Contains(w.Body.String(), `"pinned":["libraries"]`) {
		t.Errorf("reading it back: %s", w.Body)
	}
	// It lives with the profile's other interface choices, which the web app still reads.
	if ui := ts.do(t, "GET", "/api/me/ui", f.oneLibrary, nil).Body.String(); !strings.Contains(ui, `"sdui":{"pages":{"home"`) {
		t.Errorf("/api/me/ui: %s", ui)
	}
	for name, body := range map[string]string{"a density that isn't one": `{"density":"huge"}`, "an ID that isn't one": `{"hidden":["a b"]}`, "unknown fields": `{"colour":"red"}`,
		"too much": `{"order":["` + strings.Repeat("x", 9000) + `"]}`} {
		if w := ts.do(t, "PUT", "/api/sdui/pages/home/personal", f.oneLibrary, []byte(body)); w.Code != 400 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	if w := ts.do(t, "PUT", "/api/sdui/pages/nosuch/personal", f.oneLibrary, mine); w.Code != 404 {
		t.Errorf("a page that isn't there: %d", w.Code)
	}
	if w := ts.do(t, "DELETE", "/api/sdui/pages/home/personal", f.oneLibrary, nil); w.Code != 200 || order(f.oneLibrary) != before {
		t.Errorf("reset: %d, %s", w.Code, order(f.oneLibrary))
	}
	// A profile whose "rearrange pages" switch is off.
	_, fixed := ts.user(t, "robin", auth.KindKids, false, `{"maxAge":18,"allowUnrated":true,"off":["editLayout"]}`)
	if w := ts.do(t, "PUT", "/api/sdui/pages/home/personal", fixed, mine); w.Code != 403 {
		t.Errorf("a profile that may not rearrange pages: %d", w.Code)
	}
	if w := ts.do(t, "DELETE", "/api/sdui/navigation/personal", fixed, nil); w.Code != 403 {
		t.Errorf("nor its navigation: %d", w.Code)
	}
	if d := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/home", fixed, nil)); d.Personal == nil || d.Personal.Editable || len(d.Personal.Allow) != 0 {
		t.Errorf("what the page says it may change: %+v", d.Personal)
	}
	// What the web app's Home saved before pages were server-driven still applies.
	if w := ts.do(t, "PUT", "/api/me/ui", f.kid, map[string]any{"home": map[string]any{"v": 2, "order": []string{"shuffle", "hero"}, "hidden": []string{"libraries"}}}); w.Code >= 300 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if got := order(f.kid); !strings.HasPrefix(got, "shuffle hero recent-1") || strings.Contains(got, "libraries") {
		t.Errorf("home with the old saved order: %s", got)
	}
	// A page can say what may be changed, a section can be locked, and the server can turn
	// it all off.
	home, _ := sdui.StockPage("home")
	home.Personal = &sdui.PersonalPolicy{Allow: []string{"reorder"}}
	for i := range home.Sections {
		home.Sections[i].Locked = home.Sections[i].ID == "hero"
	}
	etag := ts.do(t, "GET", "/api/sdui/config", f.admin, nil).Header().Get("ETag")
	w := ts.do(t, "PUT", "/api/sdui/config/pages/home", f.admin, home, "If-Match", etag)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	if got := order(f.kid); !strings.HasPrefix(got, "hero shuffle libraries") {
		t.Errorf("a locked spotlight and no hiding: %s", got)
	}
	cfg := decode[sduistore.Config](t, w)
	cfg.Personalisation.Disabled = true
	if w := ts.do(t, "PUT", "/api/sdui/config", f.admin, cfg, "If-Match", cfg.ETag); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if got := order(f.kid); !strings.HasPrefix(got, "hero libraries") {
		t.Errorf("with personalisation off: %s", got)
	}
	if w := ts.do(t, "PUT", "/api/sdui/pages/home/personal", f.oneLibrary, mine); w.Code != 403 {
		t.Errorf("saving an arrangement with personalisation off: %d", w.Code)
	}
}

// A client that draws less is sent less: fallbacks by catalogue revision and by the
// components it names.
func TestSDUICapabilityNegotiation(t *testing.T) {
	f := newSDUIFixture(t)
	ts := f.ts
	types := func(query string) string {
		t.Helper()
		doc := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/home?"+query, f.admin, nil))
		var out []string
		for _, n := range doc.Root.Children {
			out = append(out, n.Type+":"+n.ID)
		}
		return strings.Join(out, " ")
	}
	if got := types(""); !strings.HasPrefix(got, "Hero:hero") || !strings.HasSuffix(got, "Banner:shuffle") {
		t.Errorf("everything: %s", got)
	}
	for _, q := range []string{"components=none", "catalog=1"} {
		if got := types(q); !strings.HasPrefix(got, "Shelf:hero") || !strings.HasSuffix(got, "Notice:shuffle") {
			t.Errorf("%s: %s", q, got)
		}
	}
	if got := types("components=Hero"); !strings.HasPrefix(got, "Hero:hero") || !strings.HasSuffix(got, "Notice:shuffle") {
		t.Errorf("only Hero: %s", got)
	}
	if doc := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/home?catalog=1", f.admin, nil)); doc.Catalog != 1 {
		t.Errorf("document.catalog for an older client: %d", doc.Catalog)
	}
	if doc := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/home?catalog=99", f.admin, nil)); doc.Catalog != sdui.CatalogRevision {
		t.Errorf("document.catalog for a newer client: %d", doc.Catalog)
	}
}

// The renders kept for diffs are bounded for the server and for each account, so neither a
// busy client nor many accounts can make the server hold pages without end.
func TestSDUIKeptRendersAreBounded(t *testing.T) {
	ts := newTestServer(t)
	doc := func(n int) sdui.Document {
		d := sdui.Document{Version: 1, Page: sdui.PageInfo{ID: "x"}, Root: sdui.Node{Type: "Page", ID: fmt.Sprint("x", n)}}
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
	// The few seconds of finished renders are bounded too.
	_, tok := ts.user(t, "owner", auth.KindAdult, true, "")
	for i := range 400 {
		if w := ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/home?viewport=%dx900", 800+i), tok, nil); w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
	if n, limit := len(ts.sdui.recent), keptRendersFor(pace.Machine()); n > limit {
		t.Errorf("%d recent renders kept, limit %d", n, limit)
	}
}
