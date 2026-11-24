// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/channels"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// A programme's breaks are the runs of bumpers and commercials inside it and after it, up to
// the next programme's own pre-roll.
func TestBreaksInAProgramme(t *testing.T) {
	show := func(start, end, item, off int64) channels.Programme {
		return channels.Programme{Start: start, End: end, Kind: channels.KindShow, ItemID: item, Offset: off}
	}
	filler := func(start, end int64, role string) channels.Programme {
		return channels.Programme{Start: start, End: end, Kind: channels.KindFiller, Role: role}
	}
	list := []channels.Programme{
		filler(0, 10, channels.RolePre),
		show(10, 600, 1, 0),
		filler(600, 630, channels.RoleBreak), filler(630, 660, channels.RoleBreak),
		show(660, 1200, 1, 590),
		filler(1200, 1260, channels.RoleAd), {Start: 1260, End: 1290, Kind: channels.KindSlate},
		filler(1290, 1300, channels.RolePre),
		show(1300, 2000, 2, 0),
		filler(2000, 2030, channels.RoleBreak),
	}
	cur := &channels.Programme{Start: 0, End: 1200, Kind: channels.KindShow, ItemID: 1}
	got := breaksIn(list, cur)
	want := []span{{0, 10}, {600, 660}, {1200, 1290}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("breaks %v, want %v", got, want)
	}
	// The next programme's breaks are its own.
	next := &channels.Programme{Start: 1290, End: 2000, Kind: channels.KindShow, ItemID: 2}
	if got := breaksIn(list, next); fmt.Sprint(got) != fmt.Sprint([]span{{1290, 1300}, {2000, 2030}}) {
		t.Fatalf("next programme's breaks %v", got)
	}
}

func TestStartOverRoutes(t *testing.T) {
	h, admin, kid, s := tvServer(t)
	_, video, _ := tvWorld(t, s)
	for _, p := range []string{"/api/tv/lib-1/startover", "/api/tv/lib-1/timeline", "/api/tv/lib-1/shift/v/playlist.m3u8", "/api/tv/lib-1/shift/v/seg/x.ts"} {
		if w := tvCall(h, "GET", p, "", ""); w.Code != 401 {
			t.Errorf("GET %s signed out: %d", p, w.Code)
		}
	}
	body := fmt.Sprintf(`{"name":"Films","number":"7","definition":{"mode":"simple","rules":{"items":[%d]},"order":{"mode":"sequential"},"filler":{},"alignMin":0}}`, video)
	w := tvCall(h, "POST", "/api/channels", admin, body)
	if w.Code != 200 {
		t.Fatalf("channel: %d %s", w.Code, w.Body)
	}
	var ch struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &ch)
	key := "lib-" + strconv.FormatInt(ch.ID, 10)

	w = tvCall(h, "GET", "/api/tv/"+key+"/timeline", admin, "")
	if w.Code != 200 {
		t.Fatalf("timeline: %d %s", w.Code, w.Body)
	}
	var tl timeline
	_ = json.Unmarshal(w.Body.Bytes(), &tl)
	now := time.Now().Unix()
	if tl.Programme.Start > now || tl.Programme.End <= now || tl.Live < now-2 || tl.Earliest >= tl.Live {
		t.Fatalf("timeline: %s", w.Body)
	}

	if w := tvCall(h, "GET", "/api/tv/"+key+"/startover", admin, ""); w.Code != 400 {
		t.Errorf("a player that doesn't name itself: %d", w.Code)
	}
	if w := tvCall(h, "GET", "/api/tv/"+key+"/startover?viewer=tv1&from="+strconv.FormatInt(now, 10), admin, ""); w.Code != 409 {
		t.Errorf("starting over from now is live: %d", w.Code)
	}
	if w := tvCall(h, "GET", "/api/tv/tun-1/startover?viewer=tv1", admin, ""); w.Code != 404 {
		t.Errorf("a tuner channel with no archive: %d", w.Code)
	}

	from := now - 600
	w = tvCall(h, "GET", "/api/tv/"+key+"/startover?viewer=tv1&from="+strconv.FormatInt(from, 10), admin, "")
	if w.Code != 200 {
		t.Fatalf("start over: %d %s", w.Code, w.Body)
	}
	var info tuneInfo
	_ = json.Unmarshal(w.Body.Bytes(), &info)
	if !info.StartOver || info.Timeline == nil || info.Timeline.From != from || !strings.Contains(info.URL, "/api/tv/"+key+"/shift/tv1/playlist.m3u8") {
		t.Fatalf("start over answer: %s", w.Body)
	}
	if info.CanRecord {
		t.Error("a start-over stream isn't recorded")
	}
	// The stream is this account's: another with the same player name has none.
	if w := tvCall(h, "GET", "/api/tv/"+key+"/shift/tv1/seg/x.ts", kid, ""); w.Code != 410 && w.Code != 404 {
		t.Errorf("another account reached the stream: %d", w.Code)
	}
	sid := ""
	for _, id := range []string{s.shiftIDFor(ch.ID, 1, "tv1")} {
		sid = id
	}
	if s.HLS.GetLive(sid) == nil {
		t.Fatalf("no shifted stream %s", sid)
	}
	if got := s.HLS.GetLive(sid).Spec.Shift; got < 599*time.Second || got > 605*time.Second {
		t.Errorf("shift %v", got)
	}
	// The shared channel isn't touched: nobody tuned it.
	if s.HLS.GetLive(key) != nil {
		t.Error("starting over started the shared stream")
	}
	// Back to live stops it.
	if w := tvCall(h, "POST", "/api/tv/"+key+"/leave?viewer=tv1&shift=1", admin, ""); w.Code != 200 {
		t.Fatalf("leave: %d", w.Code)
	}
	if s.HLS.GetLive(sid) != nil {
		t.Error("the start-over stream should stop when its player goes back to live")
	}
	if w := tvCall(h, "GET", "/api/tv/"+key+"/shift/tv1/playlist.m3u8", admin, ""); w.Code != 410 {
		t.Errorf("a stopped start-over stream: %d", w.Code)
	}

	// The owner can turn it off.
	if w := tvCall(h, "PUT", "/api/tv/settings", admin, `{"tvStartOver":"0"}`); w.Code != 200 {
		t.Fatalf("setting: %d %s", w.Code, w.Body)
	}
	if w := tvCall(h, "GET", "/api/tv/"+key+"/startover?viewer=tv1", admin, ""); w.Code != 403 {
		t.Errorf("start over turned off: %d", w.Code)
	}
}

// Start over and the stream it makes keep to the profile's limits: a title the profile may
// not watch can't be started over, and a shifted stream stops at its next part once what it
// plays is no longer allowed (parental_live.go, asked at the stream's moment of the schedule).
func TestStartOverKeepsTheProfilesLimits(t *testing.T) {
	h, admin, _, s := tvServer(t)
	_, video, _ := tvWorld(t, s)
	ctx := context.Background()
	rate := func(r string) {
		t.Helper()
		if _, err := s.DB.Exec(`UPDATE items SET content_rating = ?, min_age = ? WHERE id = ?`, r, *library.AgeFor(r), video); err != nil {
			t.Fatal(err)
		}
		s.forgetRules()
	}
	rate("G")
	if _, err := s.Auth.CreateProfile(ctx, "teen", "", auth.KindKids, false, `{"maxAge":13,"liveTv":true,"libraryChannels":true,"tunerChannels":true}`); err != nil {
		t.Fatal(err)
	}
	teen, _, err := s.Auth.Login(ctx, "teen", "", "t", true)
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"name":"Films","number":"7","definition":{"mode":"simple","rules":{"items":[%d]},"order":{"mode":"sequential"},"filler":{},"alignMin":0}}`, video)
	w := tvCall(h, "POST", "/api/channels", admin, body)
	if w.Code != 200 {
		t.Fatalf("channel: %d %s", w.Code, w.Body)
	}
	var ch struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &ch)
	key := "lib-" + strconv.FormatInt(ch.ID, 10)
	from := strconv.FormatInt(time.Now().Unix()-600, 10)

	if w := tvCall(h, "GET", "/api/tv/"+key+"/startover?viewer=tv2&from="+from, teen, ""); w.Code != 200 {
		t.Fatalf("start over of a G title by a profile limited to 13: %d %s", w.Code, w.Body)
	}
	rate("R")
	for _, p := range []string{"/shift/tv2/playlist.m3u8", "/shift/tv2/seg/x.ts", "/startover?viewer=tv2&from=" + from} {
		if w := tvCall(h, "GET", "/api/tv/"+key+p, teen, ""); w.Code != 403 && w.Code != 404 {
			t.Errorf("%s once the title is R: %d %s", p, w.Code, w.Body)
		}
	}
	// Nothing changes for an account with no limits.
	if w := tvCall(h, "GET", "/api/tv/"+key+"/startover?viewer=tv1&from="+from, admin, ""); w.Code != 200 {
		t.Errorf("start over, no limits: %d %s", w.Code, w.Body)
	}
}
