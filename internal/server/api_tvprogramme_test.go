// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"net/http/httptest"

	"encoding/json"
	"fmt"
	"github.com/wittywilla/wittymediaserver/internal/auth"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/tuner/xmltv"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// programmeFixture is the credits fixture's library with a tuner channel whose guide airs one of
// its films, then a programme the library doesn't have.
func programmeFixture(t *testing.T) (*creditsFixture, int64, int64) {
	t.Helper()
	f := newCreditsFixture(t)
	db := f.ts.DB
	now := time.Now().Truncate(time.Minute).Unix()
	film, other := now-600, now+6000
	guideOnly, _ := json.Marshal(xmltv.Details{Year: 2024, Credits: []xmltv.Credit{
		{Role: "director", Name: "Ana Ruiz"}, {Role: "actor", Name: "Mara Voss", Character: "Nell"},
		{Role: "guest", Name: "Jon Ellis"}, {Role: "presenter", Name: "Kim Ode"}, {Role: "writer", Name: "Lee Park"},
	}, Stars: []xmltv.Star{{Value: "8.1/10", System: "IMDB"}}, IMDb: "tt0123456", TMDb: "tv/4242", Subtitles: []string{"EN"}, HD: true})
	filmDetails, _ := json.Marshal(xmltv.Details{Year: 2001, Credits: []xmltv.Credit{{Role: "actor", Name: "Someone The Guide Says"}}})
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tuners(id, driver, name, options, created_at) VALUES(1, 'stream', 'Test', '{}', 1)`, nil},
		{`INSERT INTO tuner_channels(id, tuner_id, source_id, number, name, locator, epg_id) VALUES(1, 1, 's1', '4', 'Harbour Network', 'http://provider.invalid/1.ts', 'harbour.test'), (2, 1, 's2', '5', 'Pine', 'http://provider.invalid/2.ts', 'pine.test')`, nil},
		{`INSERT INTO epg_programmes(channel, start, stop, title, descr, category, rating, extra) VALUES('harbour.test', ?, ?, 'Robot Friend', 'A robot.', 'Family', 'G', ?)`, []any{film, film + 5400, string(filmDetails)}},
		{`INSERT INTO epg_programmes(channel, start, stop, title, sub, descr, category, rating, season, episode, extra) VALUES('harbour.test', ?, ?, 'The Quiet Orchard', 'The Ledger', 'Frost.', 'Drama', 'TV-PG', 1, 3, ?)`, []any{other, other + 3600, string(guideOnly)}},
		{`INSERT INTO epg_programmes(channel, start, stop, title, sub) VALUES('pine.test', ?, ?, 'The Quiet Orchard', 'Again')`, []any{other + 86400, other + 90000}},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	return f, film, other
}

func getProgramme(t *testing.T, f *creditsFixture, token, key string, at int64) (int, programmeInfo) {
	t.Helper()
	w := f.ts.do(t, "GET", fmt.Sprintf("/api/v1/tv/%s/programme?start=%d", key, at), token, nil)
	var info programmeInfo
	if w.Code == 200 {
		if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, info
}

// A programme the library doesn't have: everything the guide says, the guide's own credits, and
// where else it airs.
func TestProgrammeFromTheGuide(t *testing.T) {
	f, _, other := programmeFixture(t)
	code, info := getProgramme(t, f, f.adult, "tun-1", other+60)
	if code != 200 {
		t.Fatalf("%d", code)
	}
	p := info.Programme
	if info.Channel.Key != "tun-1" || info.Channel.Number != "4" || p.Title != "The Quiet Orchard" || p.Sub != "The Ledger" || p.Desc != "Frost." ||
		p.Rating != "TV-PG" || p.Year != 2024 || *p.Season != 1 || *p.Episode != 3 || p.LengthMin != 60 || !p.HD || fmt.Sprint(p.Genres) != "[Drama]" {
		t.Fatalf("facts %+v", p)
	}
	if len(p.Scores) != 1 || p.Scores[0].Value != "8.1/10" || info.IDs["imdb"] != "tt0123456" || info.IDs["tmdb"] != "tv/4242" {
		t.Errorf("scores and ids %+v %+v", p.Scores, info.IDs)
	}
	a := info.Actions
	if a.Watch || !a.Remind || a.Reminded || a.Open || a.WatchLater || info.Library != nil {
		t.Errorf("a programme later, not in the library: %+v %+v", a, info.Library)
	}
	c := info.Credits
	if c == nil || len(c.Cast) != 2 || c.Cast[0].Name != "Mara Voss" || c.Cast[0].Character != "Nell" || c.Cast[1].Character != "Presenter" ||
		len(c.Guests) != 1 || c.Cast[0].Source != "guide" || c.Cast[0].PersonID != 0 {
		t.Fatalf("cast %+v", c)
	}
	if len(c.Crew) != 2 || c.Crew[0].Department != "directing" || c.Crew[0].People[0].Name != "Ana Ruiz" || c.Crew[1].Department != "writing" {
		t.Errorf("crew %+v", c.Crew)
	}
	if len(info.Schedule) != 2 || !info.Schedule[0].This || info.Schedule[0].Key != "tun-1" || info.Schedule[1].Key != "tun-2" || info.Schedule[1].Sub != "Again" {
		t.Errorf("schedule %+v", info.Schedule)
	}
	if info.Trailers == nil {
		t.Error("trailers is always a list")
	}
}

// A title the library has: its credits, its page and Watch later, over what the guide says.
func TestProgrammeFromTheLibrary(t *testing.T) {
	f, film, _ := programmeFixture(t)
	code, info := getProgramme(t, f, f.adult, "tun-1", film)
	if code != 200 {
		t.Fatalf("%d", code)
	}
	if info.Library == nil || info.Library.ItemID != f.film.ID || !info.Actions.Open || !info.Actions.WatchLater || info.Actions.InList || !info.Actions.Watch || info.Actions.Remind {
		t.Fatalf("library %+v actions %+v", info.Library, info.Actions)
	}
	if c := info.Credits; c == nil || len(c.Cast) != 2 || c.Cast[0].Name != "Ana Lima" || c.Cast[0].PersonID != f.ana {
		t.Fatalf("the library's credits: %+v", info.Credits)
	}
	if w := f.ts.do(t, "POST", "/api/v1/collections/watch-later/items", f.adult, map[string]any{"items": []int64{f.film.ID}}); w.Code != 200 && w.Code != 204 {
		t.Fatalf("watch later: %d %s", w.Code, w.Body)
	}
	if _, info = getProgramme(t, f, f.adult, "tun-1", film); !info.Actions.InList {
		t.Errorf("on Watch later: %+v", info.Actions)
	}
}

func TestProgrammeRefusals(t *testing.T) {
	f, film, other := programmeFixture(t)
	for _, c := range []struct {
		token, key string
		at         int64
		want       int
	}{
		{f.adult, "tun-1", other + 7200, 404},                         // nothing on then
		{f.adult, "tun-9", other, 404},                                // no such channel
		{f.adult, "nope", other, 400},                                 // not a channel key
		{f.adult, "tun-1", 0, 400},                                    // no start
		{f.adult, "tun-1", time.Now().AddDate(0, 0, -40).Unix(), 404}, // older than any guide keeps
		{f.kid, "tun-1", film, 404},                                   // a profile without tuner channels
		{"", "tun-1", film, 401},
	} {
		if code, _ := getProgramme(t, f, c.token, c.key, c.at); code != c.want {
			t.Errorf("%s @%d: %d, want %d", c.key, c.at, code, c.want)
		}
	}
}

// The credits source draws a programme's cast and crew, the same as the route answers.
func TestSDUICreditsOfAProgramme(t *testing.T) {
	f, _, other := programmeFixture(t)
	ts := f.ts
	src := sdui.Source{"type": "credits", "programme": fmt.Sprintf("tun-1@%d", other)}
	ask := func(u *auth.User) []sdui.Item {
		r := httptest.NewRequest("GET", "/", nil)
		r = r.WithContext(context.WithValue(r.Context(), userKey, u))
		res, err := ts.newSDUIResolver(r, nil).credits(src, sdui.SourceRequest{Source: src, Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		return res.Items
	}
	viewer, _ := ts.user(t, "viewer", "adult", false, "")
	if got := ask(viewer); len(got) != 5 || got[0].Title != "Mara Voss" {
		t.Fatalf("%+v", got)
	}
	child, _ := ts.user(t, "child", "kids", false, "")
	if got := ask(child); len(got) != 0 {
		t.Errorf("a profile without tuner channels: %+v", got)
	}
}
