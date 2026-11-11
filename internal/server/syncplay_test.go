// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/syncplay"
)

// partyRoom is what the room routes answer with, as far as these tests look.
type partyRoom struct {
	ID         string `json:"id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	You        string `json:"you"`
	HostID     string `json:"hostId"`
	CanControl bool   `json:"canControl"`
	Blocked    bool   `json:"blocked"`
	Members    []struct {
		ID, Name, State string
		Host            bool
	} `json:"members"`
	Queue []int64 `json:"queue"`
	Items []struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	} `json:"items"`
	State        syncplay.State `json:"state"`
	ServerTimeMs int64          `json:"serverTimeMs"`
}

func party(t *testing.T, w *httptest.ResponseRecorder) partyRoom {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var out partyRoom
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (w *remoteWorld) partyDo(t *testing.T, tok string, room partyRoom, what string, b map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	if b == nil {
		b = map[string]any{}
	}
	b["member"] = room.You
	return w.do(t, "POST", "/api/syncplay/rooms/"+room.ID+"/"+what, tok, b)
}

func (w *remoteWorld) partyReady(t *testing.T, tok string, room partyRoom) {
	t.Helper()
	now := party(t, w.do(t, "GET", "/api/syncplay/rooms/"+room.ID+"?member="+room.You, tok, nil))
	if res := w.partyDo(t, tok, room, "report", map[string]any{"state": "ready", "rev": now.State.Rev}); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
}

func TestSyncplayAPartyFromStartToFinish(t *testing.T) {
	w := newRemoteWorld(t)
	_, friend := w.user(t, "friend", "adult", false, "")

	info := body(w.do(t, "GET", "/api/syncplay", w.parent, nil))
	if info["enabled"] != true || info["canCreate"] != true || info["drift"].(map[string]any)["seekSec"] != 3.0 {
		t.Fatalf("%v", info)
	}

	host := party(t, w.do(t, "POST", "/api/syncplay/rooms", w.parent, map[string]any{"items": []int64{w.kidFilm, w.adultFilm}, "name": "Friday film"}))
	if len(host.Code) != syncplay.CodeLen || host.Name != "Friday film" || !host.CanControl || !host.State.Paused || host.State.ItemID != w.kidFilm {
		t.Fatalf("%+v", host)
	}
	if len(host.Items) != 2 || host.Items[0].Title != "Puddle Day" || host.Items[1].Title != "The Night Shift" {
		t.Fatalf("%+v", host.Items)
	}

	// The friend types the code, however they type it.
	guest := party(t, w.do(t, "POST", "/api/syncplay/join", friend, map[string]any{"code": strings.ToLower(host.Code[:3]) + " " + host.Code[3:]}))
	if guest.ID != host.ID || guest.You == host.You || guest.CanControl || len(guest.Members) != 2 || guest.Members[1].Name != "friend" {
		t.Fatalf("%+v", guest)
	}
	// Both lists show it; a stranger's doesn't.
	if rooms := fmt.Sprint(body(w.do(t, "GET", "/api/syncplay", friend, nil))["rooms"]); !strings.Contains(rooms, host.ID) || strings.Contains(rooms, "Puddle") {
		t.Fatalf("%s", rooms)
	}
	if rooms := fmt.Sprint(body(w.do(t, "GET", "/api/syncplay", w.kid, nil))["rooms"]); strings.Contains(rooms, host.ID) {
		t.Fatalf("%s", rooms)
	}

	events := openSSE(t, w.srv, "/api/syncplay/rooms/"+guest.ID+"/events?member="+guest.You, friend)
	events.next("room")

	// The clock: the answer carries the client's own time back and the server's two.
	before := time.Now().UnixMilli()
	ping := body(w.partyDo(t, friend, guest, "ping", map[string]any{"t0": 1234.5, "rttMs": 80}))
	t1, _ := ping["t1"].(float64)
	t2, _ := ping["t2"].(float64)
	if ping["t0"] != 1234.5 || t1 < float64(before) || t2 < t1 || t2 > float64(time.Now().UnixMilli()+1) {
		t.Fatalf("%v", ping)
	}

	// Host-only: the guest can't press play.
	if res := w.partyDo(t, friend, guest, "command", map[string]any{"type": "play"}); res.Code != 403 || body(res)["code"] != "host-only" {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	w.partyReady(t, w.parent, host)
	// The guest hasn't loaded: play holds for them.
	held := party(t, w.partyDo(t, w.parent, host, "command", map[string]any{"type": "play"}))
	if !held.State.Waiting || held.State.Paused {
		t.Fatalf("%+v", held.State)
	}
	events.until("room", func(d string) bool { return strings.Contains(d, `"waiting":true`) })
	w.partyReady(t, friend, guest)
	started := events.until("room", func(d string) bool {
		return strings.Contains(d, `"waiting":false`) && strings.Contains(d, `"paused":false`)
	})
	var room partyRoom
	_ = json.Unmarshal([]byte(started), &room)
	// It starts a little in the future, by the server's clock, for everyone at once.
	if lead := room.State.AtMs - room.ServerTimeMs; lead < 200 || lead > 2600 {
		t.Fatalf("it starts %d ms from now", lead)
	}

	// The host seeks; the guest's stream says so; a late "next" from a stale screen is refused.
	seeked := party(t, w.partyDo(t, w.parent, host, "command", map[string]any{"type": "seek", "positionSec": 600}))
	events.until("room", func(d string) bool { return strings.Contains(d, `"positionSec":600`) })
	stale := w.partyDo(t, w.parent, host, "command", map[string]any{"type": "next", "ifSeq": seeked.State.Seq - 1})
	if stale.Code != 409 || body(stale)["code"] != "stale" || body(stale)["room"] == nil {
		t.Fatalf("%d %s", stale.Code, stale.Body)
	}

	// The host lets everyone control, and hands the room over.
	if res := w.do(t, "PUT", "/api/syncplay/rooms/"+host.ID, friend, map[string]any{"member": guest.You, "policy": "everyone"}); res.Code != 403 {
		t.Fatalf("a guest changed the room's rules: %d", res.Code)
	}
	if res := w.do(t, "PUT", "/api/syncplay/rooms/"+host.ID, w.parent, map[string]any{"member": host.You, "policy": "everyone"}); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	if res := w.partyDo(t, friend, guest, "command", map[string]any{"type": "pause"}); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	if res := w.partyDo(t, friend, guest, "host", map[string]any{"to": guest.You}); res.Code != 403 {
		t.Fatalf("a guest made themselves host: %d", res.Code)
	}
	if res := w.partyDo(t, w.parent, host, "host", map[string]any{"to": guest.You}); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	events.until("room", func(d string) bool { return strings.Contains(d, `"hostId":"`+guest.You+`"`) })

	// Only the host closes it (or an administrator); the stream's last word says so.
	if res := w.do(t, "DELETE", "/api/syncplay/rooms/"+host.ID+"?member="+host.You, w.parent, nil); res.Code != 403 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	if res := w.do(t, "DELETE", "/api/syncplay/rooms/"+host.ID+"?member="+guest.You, friend, nil); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	if last := events.until("closed", func(string) bool { return true }); !strings.Contains(last, `"reason":"closed"`) {
		t.Fatal(last)
	}
	if res := w.do(t, "POST", "/api/syncplay/join", friend, map[string]any{"code": host.Code}); res.Code != 404 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
}

// A room never shows or plays anyone more than their own account may.
func TestSyncplayNeverWidensAccess(t *testing.T) {
	w := newRemoteWorld(t)

	// A Kids profile can't start a party for what it can't see: to it, there is no such thing.
	if res := w.do(t, "POST", "/api/syncplay/rooms", w.kid, map[string]any{"itemId": w.adultFilm}); res.Code != 404 || body(res)["code"] != "no-item" {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	if res := w.do(t, "POST", "/api/syncplay/rooms", w.kid, map[string]any{"items": []int64{w.kidFilm, w.adultFilm}}); res.Code != 404 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}

	host := party(t, w.do(t, "POST", "/api/syncplay/rooms", w.parent, map[string]any{"items": []int64{w.adultFilm, w.kidFilm}, "name": "Late film", "policy": "everyone"}))
	// With the code, and even invited, the child is refused, and learns nothing of what it is.
	if res := w.partyDo(t, w.parent, host, "invite", map[string]any{"userIds": []int64{w.kidID, 424242}}); res.Code != 200 || body(res)["invited"] != 1.0 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	for _, b := range []map[string]any{{"code": host.Code}, {"roomId": host.ID}} {
		res := w.do(t, "POST", "/api/syncplay/join", w.kid, b)
		if res.Code != 403 || body(res)["code"] != "cant-watch" {
			t.Fatalf("%d %s", res.Code, res.Body)
		}
		if s := res.Body.String(); strings.Contains(s, "Night Shift") || strings.Contains(s, "Late film") || strings.Contains(s, fmt.Sprint(w.adultFilm)) {
			t.Fatalf("the refusal says what the room is playing: %s", s)
		}
	}
	// Their list of invitations names the room and its host, not the film.
	list := w.do(t, "GET", "/api/syncplay", w.kid, nil).Body.String()
	if !strings.Contains(list, host.ID) || strings.Contains(list, "Night Shift") || strings.Contains(list, `"itemId"`) {
		t.Fatalf("%s", list)
	}
	// Not a member: every room route says the room isn't there.
	for _, r := range [][2]string{{"GET", ""}, {"GET", "/events"}, {"POST", "/command"}, {"POST", "/report"}, {"POST", "/ping"}, {"POST", "/leave"}, {"POST", "/kick"}, {"POST", "/host"}, {"POST", "/invite"}, {"PUT", ""}, {"DELETE", ""}} {
		res := w.do(t, r[0], "/api/syncplay/rooms/"+host.ID+r[1]+"?member="+host.You, w.kid, map[string]any{"member": host.You, "type": "play", "state": "ready", "userIds": []int64{1}})
		if res.Code != 404 {
			t.Errorf("%s %s with someone else's seat: %d %s", r[0], r[1], res.Code, res.Body)
		}
	}

	// The room moves on to something the child may watch: now they may join.
	party(t, w.partyDo(t, w.parent, host, "command", map[string]any{"type": "next"}))
	kid := party(t, w.do(t, "POST", "/api/syncplay/join", w.kid, map[string]any{"roomId": host.ID}))
	if kid.State.ItemID != w.kidFilm || kid.Queue[0] != 0 || kid.Items[0].Title != "" || kid.Items[1].Title != "Puddle Day" {
		t.Fatalf("%+v", kid)
	}
	events := openSSE(t, w.srv, "/api/syncplay/rooms/"+kid.ID+"/events?member="+kid.You, w.kid)
	events.next("room")
	// Back to the grown-ups' film: the child keeps their seat, and is shown nothing of it.
	party(t, w.partyDo(t, w.parent, host, "command", map[string]any{"type": "previous"}))
	got := events.until("room", func(d string) bool { return strings.Contains(d, `"blocked":true`) })
	if strings.Contains(got, "Night Shift") || strings.Contains(got, fmt.Sprintf(`"itemId":%d`, w.adultFilm)) || strings.Contains(got, fmt.Sprintf("[%d,", w.adultFilm)) {
		t.Fatalf("a member who may not see it was told what is playing: %s", got)
	}
	// Everyone may control here, but not someone who can't see what they'd be steering.
	if res := w.partyDo(t, w.kid, kid, "command", map[string]any{"type": "seek", "positionSec": 10}); res.Code != 403 || body(res)["code"] != "cant-watch" {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	// Nor may they queue what they can't see (they can steer again once it's theirs to see).
	party(t, w.partyDo(t, w.parent, host, "command", map[string]any{"type": "next"}))
	if res := w.partyDo(t, w.kid, kid, "command", map[string]any{"type": "queue", "items": []int64{w.adultFilm}}); res.Code != 403 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	if res := w.partyDo(t, w.kid, kid, "command", map[string]any{"type": "pause"}); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	// A member can't close the room unless they host it; an administrator with no seat can.
	if res := w.do(t, "DELETE", "/api/syncplay/rooms/"+host.ID+"?member="+kid.You, w.kid, nil); res.Code != 403 {
		t.Fatalf("%d", res.Code)
	}
	if res := w.do(t, "DELETE", "/api/syncplay/rooms/"+host.ID, w.admin, nil); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
}

func TestSyncplaySwitchesAndLimits(t *testing.T) {
	w := newRemoteWorld(t)
	// Wrong codes are counted.
	refused := 0
	for range 14 {
		res := w.do(t, "POST", "/api/syncplay/join", w.kid, map[string]any{"code": "ZZZZZZ"})
		switch res.Code {
		case 429:
			refused++
		case 404:
		default:
			t.Fatalf("%d %s", res.Code, res.Body)
		}
	}
	if refused < 3 {
		t.Fatalf("14 wrong codes, %d refused", refused)
	}
	// A code of the wrong length isn't looked up at all.
	if res := w.do(t, "POST", "/api/syncplay/join", w.parent, map[string]any{"code": "ABC"}); res.Code != 404 {
		t.Fatalf("%d", res.Code)
	}
	for name, b := range map[string]map[string]any{
		"nothing to watch": {},
		"a bad policy":     {"itemId": w.kidFilm, "policy": "mob"},
		"a bad position":   {"itemId": w.kidFilm, "positionSec": -4},
		"a bad index":      {"items": []int64{w.kidFilm}, "index": 3},
	} {
		if res := w.do(t, "POST", "/api/syncplay/rooms", w.parent, b); res.Code != 400 {
			t.Errorf("%s: %d %s", name, res.Code, res.Body)
		}
	}
	if res := w.do(t, "POST", "/api/syncplay/rooms", w.parent, []byte(`{"itemId":1,"name":"`+strings.Repeat("x", 9000)+`"}`)); res.Code != 413 {
		t.Errorf("a long body: %d", res.Code)
	}
	host := party(t, w.do(t, "POST", "/api/syncplay/rooms", w.parent, map[string]any{"itemId": w.kidFilm}))
	for name, b := range map[string]map[string]any{
		"an unknown command": {"type": "explode"},
		"a seek to nowhere":  {"type": "seek", "positionSec": -1},
		"a silly rate":       {"type": "rate", "rate": 50},
	} {
		if res := w.partyDo(t, w.parent, host, "command", b); res.Code != 400 {
			t.Errorf("%s: %d %s", name, res.Code, res.Body)
		}
	}
	if res := w.partyDo(t, w.parent, host, "command", map[string]any{"type": "next"}); res.Code != 409 || body(res)["code"] != "end-of-queue" {
		t.Errorf("%d %s", res.Code, res.Body)
	}
	if res := w.partyDo(t, w.parent, host, "report", map[string]any{"state": "napping"}); res.Code != 400 {
		t.Errorf("%d %s", res.Code, res.Body)
	}
	// A flood of commands from one seat is slowed.
	slowed := false
	for range 40 {
		if res := w.partyDo(t, w.parent, host, "command", map[string]any{"type": "seek", "positionSec": 5}); res.Code == 429 {
			slowed = true
		}
	}
	if !slowed {
		t.Error("40 commands at once were all taken")
	}

	// Who may start one.
	if res := w.do(t, "PUT", "/api/settings", w.admin, map[string]string{"syncplayCreate": "adults"}); res.Code != 200 {
		t.Fatal(res.Code)
	}
	if res := w.do(t, "POST", "/api/syncplay/rooms", w.kid, map[string]any{"itemId": w.kidFilm}); res.Code != 403 || body(res)["code"] != "not-allowed" {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	if body(w.do(t, "GET", "/api/syncplay", w.kid, nil))["canCreate"] != false {
		t.Fatal("canCreate")
	}
	// They may still join.
	party(t, w.do(t, "POST", "/api/syncplay/join", w.parent, map[string]any{"code": host.Code, "device": "second window"}))
	if res := w.do(t, "PUT", "/api/settings", w.admin, map[string]string{"syncplayCreate": "sometimes"}); res.Code != 400 {
		t.Fatal(res.Code)
	}

	// The owner's switch.
	if res := w.do(t, "PUT", "/api/settings", w.admin, map[string]string{"syncplay": "0"}); res.Code != 200 {
		t.Fatal(res.Code)
	}
	for _, r := range [][2]string{{"POST", "/api/syncplay/rooms"}, {"POST", "/api/syncplay/join"}, {"GET", "/api/syncplay/rooms/" + host.ID}, {"POST", "/api/syncplay/rooms/" + host.ID + "/command"}} {
		if res := w.do(t, r[0], r[1], w.parent, map[string]any{}); res.Code != 403 || body(res)["code"] != "disabled" {
			t.Errorf("%s %s with watch parties off: %d %s", r[0], r[1], res.Code, res.Body)
		}
	}
	if info := body(w.do(t, "GET", "/api/syncplay", w.parent, nil)); info["enabled"] != false || info["canCreate"] != false {
		t.Errorf("%v", info)
	}
}

// A dropped connection is "away" at once and keeps its seat; signing out gives it up.
func TestSyncplayStreamsAndSeats(t *testing.T) {
	quickBeat(t)
	w := newRemoteWorld(t)
	_, friend := w.user(t, "friend", "adult", false, "")
	host := party(t, w.do(t, "POST", "/api/syncplay/rooms", w.parent, map[string]any{"itemId": w.kidFilm}))
	guest := party(t, w.do(t, "POST", "/api/syncplay/join", friend, map[string]any{"code": host.Code}))
	hostEvents := openSSE(t, w.srv, "/api/syncplay/rooms/"+host.ID+"/events?member="+host.You, w.parent)
	hostEvents.next("room")
	guestEvents := openSSE(t, w.srv, "/api/syncplay/rooms/"+guest.ID+"/events?member="+guest.You, friend)
	guestEvents.next("room")

	guestEvents.close()
	hostEvents.until("room", func(d string) bool { return strings.Contains(d, `"state":"away"`) })
	// Back again, the same seat.
	back := party(t, w.do(t, "POST", "/api/syncplay/join", friend, map[string]any{"code": host.Code}))
	if back.You != guest.You {
		t.Fatalf("a new seat: %s, was %s", back.You, guest.You)
	}
	first := openSSE(t, w.srv, "/api/syncplay/rooms/"+guest.ID+"/events?member="+guest.You, friend)
	first.next("room")
	// A second stream for the same seat ends the first, and says why.
	second := openSSE(t, w.srv, "/api/syncplay/rooms/"+guest.ID+"/events?member="+guest.You, friend)
	second.next("room")
	if last := first.until("closed", func(string) bool { return true }); !strings.Contains(last, "replaced") {
		t.Fatal(last)
	}
	// The host removes them: their stream says so.
	if res := w.partyDo(t, w.parent, host, "kick", map[string]any{"whom": guest.You}); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	if last := second.until("closed", func(string) bool { return true }); !strings.Contains(last, "removed") {
		t.Fatal(last)
	}
	// The host signs out: at the next keep-alive the stream ends and the seat is given up.
	w.Auth.Logout(context.Background(), w.parent)
	if last := hostEvents.until("closed", func(string) bool { return true }); !strings.Contains(last, "signed-out") {
		t.Fatal(last)
	}
	if _, err := w.syncEngine().View(host.ID, host.You, w.parentID); err == nil {
		t.Fatal("a signed-out member still has their seat")
	}
}
