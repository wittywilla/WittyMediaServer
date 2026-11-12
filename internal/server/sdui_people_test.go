// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

type creditsFixture struct {
	ts                *testServer
	adult, kid        string
	film, grownUp, ep *library.Item
	show              *library.Show
	ana, grown, guest int64
}

func newCreditsFixture(t *testing.T) *creditsFixture {
	t.Helper()
	ts := newTestServer(t)
	f := &creditsFixture{ts: ts}
	ctx := t.Context()
	root := t.TempDir()
	films, tv := ts.library(t, "movies", root), ts.library(t, "shows", filepath.Join(root, "tv"))
	y1, y2 := 2001, 2010
	f.film = ts.item(t, &library.Item{LibraryID: films, Kind: "video", Path: filepath.Join(root, "robot.mkv"), Title: "Robot Friend", Year: &y1, ContentRating: "G"})
	f.grownUp = ts.item(t, &library.Item{LibraryID: films, Kind: "video", Path: filepath.Join(root, "heist.mkv"), Title: "The Heist", Year: &y2, ContentRating: "R"})
	f.show = &library.Show{LibraryID: tv, Path: filepath.Join(root, "tv", "Harbour"), Title: "Harbour", SortTitle: "Harbour", ContentRating: "TV-G"}
	if err := ts.Store.UpsertShow(ctx, f.show); err != nil {
		t.Fatal(err)
	}
	s2, e1 := 2, 1
	f.ep = ts.item(t, &library.Item{LibraryID: tv, Kind: "video", Path: filepath.Join(root, "tv", "Harbour", "s02e01.mkv"), Title: "Storm", ShowID: &f.show.ID, Season: &s2, Episode: &e1})
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(ts.Store.SetCredits(ctx, library.TargetItem, f.film.ID, library.CreditsTMDB, []library.CreditIn{
		{Name: "Ana Lima", IDs: library.PersonIDs{TMDB: "77"}, Role: library.RoleCast, Character: "Bolt (voice)", Photo: "https://image.tmdb.org/t/p/w185/ana.jpg"},
		{Name: "Sam Reed", Role: library.RoleCast, Character: "Max", Order: 1},
		{Name: "Dee Signer", Role: library.RoleCrew, Department: "Art", Job: "Set Decoration"},
		{Name: "Ana Lima", IDs: library.PersonIDs{TMDB: "77"}, Role: library.RoleCrew, Department: "Directing", Job: "Director"},
	}))
	must(ts.Store.SetCredits(ctx, library.TargetItem, f.grownUp.ID, library.CreditsNFO, []library.CreditIn{
		{Name: "Ana Lima", IDs: library.PersonIDs{TMDB: "77"}, Role: library.RoleCast, Character: "The Boss"},
		{Name: "Only Grown Ups", Role: library.RoleCast, Character: "Henchman"},
	}))
	must(ts.Store.SetCredits(ctx, library.TargetShow, f.show.ID, library.CreditsTVDB, []library.CreditIn{{Name: "Captain Regular", Role: library.RoleCast, Character: "Captain"}}))
	must(ts.Store.SetCredits(ctx, library.TargetItem, f.ep.ID, library.CreditsTMDB, []library.CreditIn{{Name: "Guest Diver", Role: library.RoleGuest, Character: "Diver"}}))
	ts.user(t, "boss", "adult", true, "")
	_, f.adult = ts.user(t, "bob", "adult", false, "")
	_, f.kid = ts.user(t, "kid", "kids", false, "")
	for name, into := range map[string]*int64{"ana lima": &f.ana, "only grown ups": &f.grown, "guest diver": &f.guest} {
		if err := ts.DB.QueryRowContext(ctx, `SELECT id FROM people WHERE key = ?`, name).Scan(into); err != nil {
			t.Fatal(name, err)
		}
	}
	return f
}

func TestCreditsRoutes(t *testing.T) {
	f := newCreditsFixture(t)
	ts := f.ts
	var c library.Credits
	w := ts.do(t, "GET", fmt.Sprintf("/api/v1/items/%d/credits", f.film.ID), f.kid, nil)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &c) != nil {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if len(c.Cast) != 2 || c.Cast[0].Name != "Ana Lima" || !c.Cast[0].HasPhoto || len(c.Crew) != 2 || c.Crew[0].Department != "directing" || c.Crew[1].Label != "Art" {
		t.Errorf("credits: %+v", c)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/v1/items/%d/credits", f.grownUp.ID), f.kid, nil); w.Code != 404 {
		t.Errorf("a hidden film's credits: %d", w.Code)
	}
	w = ts.do(t, "GET", fmt.Sprintf("/api/v1/shows/%d/credits?season=2", f.show.ID), f.adult, nil)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &c) != nil || len(c.Cast) != 1 || len(c.Guests) != 1 || c.Guests[0].Name != "Guest Diver" {
		t.Errorf("season credits: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/v1/shows/%d/credits?season=x", f.show.ID), f.adult, nil); w.Code != 400 {
		t.Errorf("a season that isn't a number: %d", w.Code)
	}
	w = ts.do(t, "GET", fmt.Sprintf("/api/v1/items/%d/credits", f.ep.ID), f.adult, nil)
	if json.Unmarshal(w.Body.Bytes(), &c) != nil || len(c.Cast) != 1 || c.Cast[0].Name != "Captain Regular" || len(c.Guests) != 1 {
		t.Errorf("an episode: the show's cast and its own guest: %s", w.Body)
	}

	// A person: their titles, within the limits.
	var p struct {
		Person      library.Person      `json:"person"`
		Filmography library.Filmography `json:"filmography"`
	}
	w = ts.do(t, "GET", fmt.Sprintf("/api/v1/people/%d", f.ana), f.adult, nil)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &p) != nil {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if p.Person.Name != "Ana Lima" || p.Person.IDs.TMDB != "77" || len(p.Filmography.Groups) != 2 || len(p.Filmography.Groups[0].Titles) != 2 {
		t.Errorf("person: %s", w.Body)
	}
	if strings.Contains(w.Body.String(), "image.tmdb.org") {
		t.Error("where a picture comes from is never sent: it is served through /people/{id}/photo")
	}
	w = ts.do(t, "GET", fmt.Sprintf("/api/v1/people/%d", f.ana), f.kid, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "Heist") || strings.Contains(w.Body.String(), "The Boss") {
		t.Errorf("a kid sees Ana's R film: %s", w.Body)
	}
	for _, path := range []string{fmt.Sprintf("/api/v1/people/%d", f.grown), fmt.Sprintf("/api/v1/people/%d/photo", f.grown), "/api/v1/people/999999"} {
		if w := ts.do(t, "GET", path, f.kid, nil); w.Code != 404 {
			t.Errorf("%s for a kid: %d", path, w.Code)
		}
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/v1/people/%d/photo", f.guest), f.adult, nil); w.Code != 404 {
		t.Errorf("a person with no photo: %d", w.Code)
	}
	var found struct {
		People []library.FoundPerson `json:"people"`
	}
	w = ts.do(t, "GET", "/api/v1/people?search=grown", f.kid, nil)
	if json.Unmarshal(w.Body.Bytes(), &found) != nil || len(found.People) != 0 {
		t.Errorf("search finds a person only in hidden titles: %s", w.Body)
	}
	w = ts.do(t, "GET", "/api/v1/people?search=lima", f.adult, nil)
	if json.Unmarshal(w.Body.Bytes(), &found) != nil || len(found.People) != 1 || found.People[0].Titles != 2 {
		t.Errorf("search: %s", w.Body)
	}
	// The library, filtered by a person.
	w = ts.do(t, "GET", fmt.Sprintf("/api/v1/browse?person=%d", f.ana), f.kid, nil)
	if !strings.Contains(w.Body.String(), "Robot Friend") || strings.Contains(w.Body.String(), "Heist") {
		t.Errorf("browse by person for a kid: %s", w.Body)
	}
}

// The person's photo is served from the image cache like a poster: a picture from the web,
// once fetched, is kept there.
func TestPersonPhoto(t *testing.T) {
	f := newCreditsFixture(t)
	const u = "https://image.tmdb.org/t/p/w185/ana.jpg"
	p := f.ts.remoteArtPath(u)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, logoPicture(t), 0o644); err != nil {
		t.Fatal(err)
	}
	w := f.ts.do(t, "GET", fmt.Sprintf("/api/v1/people/%d/photo", f.ana), f.kid, nil)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "image/") {
		t.Fatalf("%d %s %s", w.Code, w.Header().Get("Content-Type"), w.Body)
	}
}

func TestSDUICredits(t *testing.T) {
	f := newCreditsFixture(t)
	ts := f.ts
	doc := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/v1/sdui/pages/detail?catalog=3&formFactor=web&viewport=1440x900&item=%d", f.film.ID), f.kid, nil))
	if len(doc.Root.Children) < 2 || doc.Root.Children[1].Type != "Credits" {
		t.Fatalf("the credits come right after the header: %+v", doc.Root.Children)
	}
	cr := doc.Root.Children[1]
	groups, _ := cr.Props["groups"].([]any)
	if len(cr.Children) != 4 || len(groups) != 3 {
		t.Fatalf("credits node: %d cards, %d groups: %+v", len(cr.Children), len(groups), cr.Props)
	}
	first := cr.Children[0]
	if first.Props["group"] != "cast" || first.Props["subtitle"] != "Bolt (voice)" || first.Action == nil || first.Action.To != fmt.Sprintf("/person/%d", f.ana) {
		t.Errorf("first card: %+v %+v", first.Props, first.Action)
	}
	if img, _ := first.Props["image"].(map[string]any); img == nil || !strings.HasPrefix(img["url"].(string), fmt.Sprintf("/api/people/%d/photo?w=", f.ana)) {
		t.Errorf("the photo is the server's own address: %+v", first.Props["image"])
	}
	// A client of catalogue revision 2 gets a row of person cards.
	old := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/v1/sdui/pages/detail?catalog=2&formFactor=phone&item=%d", f.film.ID), f.kid, nil))
	if n := findNode(&old.Root, "credits"); n == nil || n.Type != "Shelf" || len(n.Children) != 4 || n.Props["cardStyle"] != "person" {
		t.Errorf("catalogue 2: %+v", n)
	}
	// A show's page opened for a season has its guests.
	sh := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/v1/sdui/pages/detail?catalog=3&formFactor=tv&show=%d&season=2", f.show.ID), f.adult, nil))
	if n := findNode(&sh.Root, "credits"); n == nil || len(n.Children) != 2 || n.Children[1].Props["group"] != "guests" {
		t.Errorf("season credits: %+v", n)
	}
	// The person's page: the header, then a row per part of their work.
	pp := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/v1/sdui/pages/detail?catalog=3&formFactor=desktop&person=%d", f.ana), f.adult, nil))
	if pp.Page.ID != "detail-person" || pp.Page.Route != fmt.Sprintf("/person/%d", f.ana) {
		t.Fatalf("page: %+v", pp.Page)
	}
	acting, directing := findNode(&pp.Root, "titles-acting"), findNode(&pp.Root, "titles-directing")
	if acting == nil || directing == nil || len(acting.Children) != 2 || len(directing.Children) != 1 || acting.Props["title"] != "Acting" {
		t.Fatalf("rows: %+v %+v", acting, directing)
	}
	if acting.Children[0].Props["subtitle"] != "2010 · as The Boss" {
		t.Errorf("newest first, with the part: %+v", acting.Children[0].Props)
	}
	kidPage := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/v1/sdui/pages/detail?catalog=3&formFactor=desktop&person=%d", f.ana), f.kid, nil))
	if n := findNode(&kidPage.Root, "titles-acting"); n == nil || len(n.Children) != 1 {
		t.Errorf("a kid's view of Ana: %+v", n)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/v1/sdui/pages/detail?catalog=3&person=%d", f.grown), f.kid, nil); w.Code != 404 {
		t.Errorf("a person only in hidden titles, for a kid: %d", w.Code)
	}
	// Search finds people.
	sr := sduiDoc(t, ts.do(t, "GET", "/api/v1/sdui/pages/search?catalog=3&formFactor=desktop&q=lima", f.adult, nil))
	if n := findNode(&sr.Root, "people"); n == nil || len(n.Children) != 1 || n.Children[0].Props["subtitle"] != "2 titles" {
		t.Errorf("people in search: %+v", n)
	}
	// Show all: everyone, in place of the first few.
	w := ts.do(t, "GET", fmt.Sprintf("/api/v1/sdui/pages/detail/sections/credits/items?cursor=0&item=%d", f.film.ID), f.kid, nil)
	var all sdui.SectionItems
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &all) != nil || !all.Replace || len(all.Items) != 4 {
		t.Errorf("show all: %d %s", w.Code, w.Body)
	}
}

// The departments the library files credits under are the groups the catalogue names.
func TestCreditGroupsMatchTheLibrary(t *testing.T) {
	keys := library.DepartmentKeys()
	if len(sdui.CreditGroups) != len(keys)+2 {
		t.Fatalf("%d groups for %d departments", len(sdui.CreditGroups), len(keys))
	}
	for i, k := range keys {
		g := sdui.CreditGroups[i+2]
		if g.Value != k || g.Label != library.Departments[i].Label {
			t.Errorf("group %d: %+v, department %+v", i, g, library.Departments[i])
		}
	}
}
