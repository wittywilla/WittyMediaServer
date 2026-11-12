// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type setsAnswer struct {
	Sets []struct {
		ID       string `json:"id"`
		Kind     string `json:"kind"`
		Name     string `json:"name"`
		Count    int    `json:"count"`
		Shared   bool   `json:"shared"`
		Editable bool   `json:"editable"`
	} `json:"sets"`
	CanShare bool `json:"canShare"`
}

type guideAnswer struct {
	Total    int    `json:"total"`
	Offset   int    `json:"offset"`
	Located  *int   `json:"located"`
	Set      string `json:"set"`
	Channels []struct {
		Key      string          `json:"key"`
		Number   string          `json:"number"`
		Name     string          `json:"name"`
		Security json.RawMessage `json:"security"`
	} `json:"channels"`
}

func (ts *testServer) guide(t *testing.T, tok, query string) guideAnswer {
	t.Helper()
	w := ts.do(t, "GET", "/api/guide?"+query, tok, nil)
	if w.Code != 200 {
		t.Fatalf("guide %s: %d %s", query, w.Code, w.Body.String())
	}
	return decode[guideAnswer](t, w)
}

func keys(g guideAnswer) string {
	var out []string
	for _, c := range g.Channels {
		out = append(out, c.Key)
	}
	return strings.Join(out, ",")
}

func TestChannelSets(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "sam", "adult", false, "")
	bigLineup(t, ts.Server, 40, 1)

	// A set of one's own, in one's own order.
	w := ts.do(t, "POST", "/api/tv-sets", adult, map[string]any{"name": " Sports  ", "channels": []string{"tun-30", "tun-2", "tun-30", "nonsense", "tun-11"}})
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	set := decode[setReply](t, w)
	if set.Name != "Sports" || strings.Join(set.Channels, ",") != "tun-30,tun-2,tun-11" || !set.Editable || set.Shared {
		t.Fatalf("made %+v", set)
	}
	if g := ts.guide(t, adult, "set="+set.ID+"&list=1"); keys(g) != "tun-30,tun-2,tun-11" || g.Total != 3 || g.Set != set.ID {
		t.Fatalf("a custom set keeps its order: %s", keys(g))
	}
	// Another account can't see it, change it, or name it in the guide.
	if w := ts.do(t, "GET", "/api/tv-sets/"+strings.TrimPrefix(set.ID, "set:"), admin, nil); w.Code != 404 {
		t.Fatalf("someone else's set: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/guide?set="+set.ID, admin, nil); w.Code != 404 {
		t.Fatalf("someone else's set in the guide: %d", w.Code)
	}
	if w := ts.do(t, "DELETE", "/api/tv-sets/"+set.ID, admin, nil); w.Code != 404 {
		t.Fatalf("deleting someone else's set: %d", w.Code)
	}
	// Only an administrator shares.
	if w := ts.do(t, "PUT", "/api/tv-sets/"+set.ID, adult, map[string]any{"shared": true}); w.Code != 403 {
		t.Fatalf("an adult sharing: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/tv-sets", adult, map[string]any{"name": "Kids", "shared": true}); w.Code != 403 {
		t.Fatalf("an adult making a shared set: %d", w.Code)
	}
	w = ts.do(t, "POST", "/api/tv-sets", admin, map[string]any{"name": "News", "channels": []string{"tun-1", "tun-9"}, "shared": true})
	if w.Code != 201 {
		t.Fatalf("shared: %d %s", w.Code, w.Body.String())
	}
	shared := decode[setReply](t, w)
	// Names are unique among what one account sees, shared sets included.
	if w := ts.do(t, "POST", "/api/tv-sets", adult, map[string]any{"name": "news"}); w.Code != 400 {
		t.Fatalf("a name already shown: %d", w.Code)
	}
	// Everyone sees a shared set; only an administrator changes it.
	if w := ts.do(t, "POST", "/api/tv-sets/"+shared.ID+"/channels", adult, map[string]any{"add": []string{"tun-3"}}); w.Code != 403 {
		t.Fatalf("an adult changing a shared set: %d", w.Code)
	}
	w = ts.do(t, "POST", "/api/tv-sets/"+shared.ID+"/channels", admin, map[string]any{"add": []string{"tun-3"}, "remove": []string{"tun-1"}})
	if got := decode[setReply](t, w); strings.Join(got.Channels, ",") != "tun-9,tun-3" {
		t.Fatalf("add and remove: %v", got.Channels)
	}
	list := decode[setsAnswer](t, ts.do(t, "GET", "/api/tv-sets", adult, nil))
	var ids []string
	for _, x := range list.Sets {
		ids = append(ids, x.ID)
	}
	want := "all," + set.ID + "," + shared.ID + ",src:tun-1,src:tun-2"
	if !strings.HasPrefix(strings.Join(ids, ","), want) || list.CanShare {
		t.Fatalf("sets %v, canShare %v; want them to start %s", ids, list.CanShare, want)
	}
	for _, x := range list.Sets {
		if x.ID == shared.ID && (x.Editable || !x.Shared || x.Count != 2) {
			t.Fatalf("shared set as an adult sees it: %+v", x)
		}
		if x.ID == "grp:News" && x.Count != 5 {
			t.Fatalf("group count: %+v", x)
		}
	}
	// The adult's own order of the sets it sees.
	if w := ts.do(t, "PUT", "/api/tv-sets/order", adult, map[string]any{"ids": []string{shared.ID, set.ID}}); w.Code != 200 {
		t.Fatalf("order: %d", w.Code)
	}
	list = decode[setsAnswer](t, ts.do(t, "GET", "/api/tv-sets", adult, nil))
	if list.Sets[1].ID != shared.ID || list.Sets[2].ID != set.ID {
		t.Fatalf("after ordering: %+v", list.Sets[:3])
	}
	// Renaming, and replacing the channels.
	w = ts.do(t, "PUT", "/api/tv-sets/"+set.ID, adult, map[string]any{"name": "Match day", "channels": []string{"tun-5", "tun-4"}})
	if got := decode[setReply](t, w); got.Name != "Match day" || strings.Join(got.Channels, ",") != "tun-5,tun-4" {
		t.Fatalf("edit: %d %s", w.Code, w.Body.String())
	}
	if w := ts.do(t, "DELETE", "/api/tv-sets/"+set.ID, adult, nil); w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/guide?set="+set.ID, adult, nil); w.Code != 404 {
		t.Fatalf("a deleted set: %d", w.Code)
	}
}

type setReply struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Shared   bool     `json:"shared"`
	Editable bool     `json:"editable"`
	Channels []string `json:"channels"`
}

// A profile kept from tuner channels can't put them in a set, and never sees them in one.
func TestChannelSetsFollowLimits(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, kid := ts.user(t, "kid", "kids", false, `{"maxAge":7,"liveTv":true,"libraryChannels":true}`)
	bigLineup(t, ts.Server, 5, 1)
	if w := ts.do(t, "POST", "/api/tv-sets", kid, map[string]any{"name": "Mine", "channels": []string{"tun-1"}}); w.Code != 400 {
		t.Fatalf("a kid putting a tuner channel in a set: %d %s", w.Code, w.Body.String())
	}
	w := ts.do(t, "POST", "/api/tv-sets", admin, map[string]any{"name": "Everyone", "channels": []string{"tun-1", "tun-2"}, "shared": true})
	shared := decode[setReply](t, w)
	if g := ts.guide(t, kid, "set="+shared.ID); g.Total != 0 {
		t.Fatalf("a kid sees %d tuner channels through a shared set", g.Total)
	}
	list := decode[setsAnswer](t, ts.do(t, "GET", "/api/tv-sets", kid, nil))
	for _, x := range list.Sets {
		if x.Count != 0 {
			t.Fatalf("a kid's set picker counts tuner channels: %+v", x)
		}
	}
}

func TestGuideFindsAndPages(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	bigLineup(t, ts.Server, 120, 1)
	// A page in a source, by number.
	if g := ts.guide(t, tok, "set=src:tun-2&limit=25&offset=25&list=1"); g.Total != 60 || g.Offset != 25 || g.Channels[0].Number != "51" {
		t.Fatalf("second page of a source: total %d offset %d first %s", g.Total, g.Offset, g.Channels[0].Number)
	}
	// Searching names and numbers.
	if g := ts.guide(t, tok, "q=channel%2000011&list=1"); keys(g) != "tun-11" {
		t.Fatalf("by name: %s", keys(g))
	}
	if g := ts.guide(t, tok, "q=11&list=1&limit=100"); keys(g) != "tun-11,tun-110,tun-111,tun-112,tun-113,tun-114,tun-115,tun-116,tun-117,tun-118,tun-119" {
		t.Fatalf("by number: %s", keys(g))
	}
	// Locating a channel moves to the page holding it.
	g := ts.guide(t, tok, "locate=77&limit=25&list=1")
	if g.Located == nil || *g.Located != 76 || g.Offset != 75 || g.Channels[1].Key != "tun-77" {
		t.Fatalf("locate: %+v offset %d", g.Located, g.Offset)
	}
	if g := ts.guide(t, tok, "locate=077&limit=25&list=1"); g.Located == nil || *g.Located != 76 {
		t.Fatal("a dialled 077 is 77")
	}
	if g := ts.guide(t, tok, "set=grp:News&locate=77&limit=25&list=1"); g.Located == nil || *g.Located != -1 {
		t.Fatal("not in this set")
	}
	// The light list carries what a picker needs, and nothing like the security notes.
	g = ts.guide(t, tok, "list=1&lite=1&limit=20000")
	if g.Total != 120 || len(g.Channels) != 120 || g.Channels[0].Security != nil || g.Channels[0].Name == "" {
		t.Fatalf("lite: %d rows, %s", len(g.Channels), g.Channels[0].Security)
	}
	if w := ts.do(t, "GET", "/api/guide?set=bogus", tok, nil); w.Code != 404 {
		t.Fatalf("no such set: %d", w.Code)
	}
}

func TestTunerBulkAndBackups(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "sam", "adult", false, "")
	bigLineup(t, ts.Server, 10, 1)
	// The same station twice: one guide id across both tuners.
	if _, err := ts.DB.Exec(`UPDATE tuner_channels SET epg_id = 'same.test', name = 'Same Station' WHERE id IN (3, 4)`); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "GET", "/api/tuners/duplicates", adult, nil); w.Code != 403 {
		t.Fatalf("duplicates for an adult: %d", w.Code)
	}
	d := decode[dupAnswer](t, ts.do(t, "GET", "/api/tuners/duplicates", admin, nil))
	if len(d.Groups) != 1 || d.Groups[0].Reason != "same guide id" || len(d.Groups[0].Channels) != 2 {
		t.Fatalf("duplicates: %+v", d)
	}
	// Merging hides the backup from the guide; unmerging brings it back.
	if w := ts.do(t, "POST", "/api/tuner-channels/bulk", admin, map[string]any{"ids": []int64{4}, "action": "merge", "into": 3}); w.Code != 200 {
		t.Fatalf("merge: %d %s", w.Code, w.Body.String())
	}
	if g := ts.guide(t, admin, "list=1"); g.Total != 9 || strings.Contains(","+keys(g)+",", ",tun-4,") {
		t.Fatalf("after merging: %s", keys(g))
	}
	if w := ts.do(t, "POST", "/api/tuner-channels/bulk", admin, map[string]any{"ids": []int64{3}, "action": "merge", "into": 4}); w.Code != 400 {
		t.Fatalf("merging into a backup: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/tuner-channels/bulk", admin, map[string]any{"ids": []int64{4}, "action": "unmerge"}); w.Code != 200 {
		t.Fatalf("unmerge: %d", w.Code)
	}
	// Renumbering from 500 in the order given; switching some off.
	if w := ts.do(t, "POST", "/api/tuner-channels/bulk", admin, map[string]any{"ids": []int64{9, 1}, "action": "renumber", "start": "500"}); w.Code != 200 {
		t.Fatalf("renumber: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/tuner-channels/bulk", admin, map[string]any{"ids": []int64{9}, "action": "renumber", "start": "5.1"}); w.Code != 400 {
		t.Fatalf("renumber from a decimal: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/tuner-channels/bulk", admin, map[string]any{"ids": []int64{2}, "action": "disable"}); w.Code != 200 {
		t.Fatalf("disable: %d", w.Code)
	}
	g := ts.guide(t, admin, "list=1")
	last := g.Channels[len(g.Channels)-2:]
	if g.Total != 9 || last[0].Key != "tun-9" || last[0].Number != "500" || last[1].Key != "tun-1" || last[1].Number != "501" {
		t.Fatalf("after renumbering: %s", keys(g))
	}
	if w := ts.do(t, "POST", "/api/tuner-channels/bulk", admin, map[string]any{"ids": []int64{1}, "action": "explode"}); w.Code != 400 {
		t.Fatalf("unknown action: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/tuner-channels/bulk", adult, map[string]any{"ids": []int64{1}, "action": "disable"}); w.Code != 403 {
		t.Fatalf("bulk for an adult: %d", w.Code)
	}
	// A backup whose main channel has gone comes back by itself.
	if w := ts.do(t, "POST", "/api/tuner-channels/bulk", admin, map[string]any{"ids": []int64{4}, "action": "merge", "into": 3}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := ts.do(t, "DELETE", "/api/tuners/"+fmt.Sprint(1+3%2), admin, nil); w.Code != 200 {
		t.Fatalf("delete tuner: %d %s", w.Code, w.Body.String())
	}
	if g := ts.guide(t, admin, "list=1"); !strings.Contains(","+keys(g)+",", ",tun-4,") {
		t.Fatalf("the backup stayed hidden: %s", keys(g))
	}
}

type dupAnswer struct {
	Groups []struct {
		Reason    string `json:"reason"`
		Suggested int64  `json:"suggested"`
		Channels  []struct {
			ID int64 `json:"id"`
		} `json:"channels"`
	} `json:"groups"`
}
