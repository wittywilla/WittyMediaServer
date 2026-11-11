// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/syncplay"
)

// What a sign-in has open ends when the sign-in does: at once, not at the stream's next
// keep-alive. None of these tests shortens the keep-alive (quickBeat), so a stream that
// only ended by looking at its sign-in again would fail them by taking 25 seconds.

// lastWord waits for a stream's last word and answers it, failing if it takes long.
func lastWord(t *testing.T, s *sse, event string) string {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case e, open := <-s.events:
			if !open {
				t.Fatalf("the stream ended without saying %q", event)
			}
			if e[0] == event {
				return e[1]
			}
		case <-deadline:
			t.Fatalf("the stream was still open five seconds after its sign-in ended (waiting for %q)", event)
		}
	}
}

// house is one sign-in with everything open that can be: a device with its channel, a
// controller's stream, and a seat in a watch party with its stream.
type house struct {
	tv                    string
	channel, events, room *sse
	seat                  partyRoom
}

func (w *limitsWorld) openEverything(t *testing.T, tok string, party_ partyRoom) house {
	t.Helper()
	h := house{}
	h.tv, h.channel = w.tv(t, tok, "TV")
	h.events = openSSE(t, w.srv, "/api/remote-control/events", tok)
	h.events.next("snapshot")
	h.seat = party(t, w.do(t, "POST", "/api/syncplay/join", tok, map[string]any{"code": party_.Code}))
	h.room = openSSE(t, w.srv, "/api/syncplay/rooms/"+h.seat.ID+"/events?member="+h.seat.You, tok)
	h.room.next("room")
	return h
}

// closed checks that all of it ended, and said the sign-in was the reason.
func (w *limitsWorld) closed(t *testing.T, what string, h house, userID int64) {
	t.Helper()
	if bye := lastWord(t, h.channel, "bye"); !strings.Contains(bye, "signed-out") {
		t.Errorf("%s: the device's channel: %s", what, bye)
	}
	if bye := lastWord(t, h.events, "bye"); !strings.Contains(bye, "signed-out") {
		t.Errorf("%s: the controller's stream: %s", what, bye)
	}
	if bye := lastWord(t, h.room, "closed"); !strings.Contains(bye, "signed-out") {
		t.Errorf("%s: the watch party's stream: %s", what, bye)
	}
	if _, here := w.remoteHub().Device(h.tv); here {
		t.Errorf("%s: the device is still listed", what)
	}
	if _, err := w.syncEngine().View(h.seat.ID, h.seat.You, userID); err == nil {
		t.Errorf("%s: the seat in the watch party is still theirs", what)
	}
}

// stillOpen checks that a sign-in that didn't end kept what it had.
func (w *limitsWorld) stillOpen(t *testing.T, what string, h house, userID int64) {
	t.Helper()
	if _, here := w.remoteHub().Device(h.tv); !here {
		t.Errorf("%s: the device of a sign-in that didn't end is gone", what)
	}
	if _, err := w.syncEngine().View(h.seat.ID, h.seat.You, userID); err != nil {
		t.Errorf("%s: the seat of a sign-in that didn't end is gone: %v", what, err)
	}
	select {
	case e, open := <-h.channel.events:
		t.Errorf("%s: the channel of a sign-in that didn't end said %v (open: %v)", what, e, open)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestSigningOutEndsWhatWasOpen(t *testing.T) {
	w := newLimitsWorld(t)
	night := party(t, w.do(t, "POST", "/api/syncplay/rooms", w.admin, map[string]any{"itemId": w.fine.ID}))
	login := func(name string) string {
		tok, _, err := w.Auth.Login(context.Background(), name, "correct-horse-1", "another device", false)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}

	// Signing out.
	u, tok := w.user(t, "anna", "adult", false, "")
	h := w.openEverything(t, tok, night)
	if res := w.do(t, "POST", "/api/logout", tok, nil); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	w.closed(t, "signing out", h, u.ID)

	// A device signed out from another one, in Settings; the one that did it keeps its own.
	phone, tv := login("anna"), login("anna")
	onPhone, onTV := w.openEverything(t, phone, night), w.openEverything(t, tv, night)
	if res := w.do(t, "DELETE", "/api/sessions/"+auth.SessionID(tv), phone, nil); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	w.closed(t, "signed out from another device", onTV, u.ID)
	w.stillOpen(t, "signed out from another device", onPhone, u.ID)

	// "Sign out everywhere else".
	tv = login("anna")
	onTV = w.openEverything(t, tv, night)
	if res := w.do(t, "DELETE", "/api/sessions", phone, nil); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	w.closed(t, "signed out everywhere else", onTV, u.ID)
	w.stillOpen(t, "signed out everywhere else", onPhone, u.ID)

	// An administrator signs a device out.
	if res := w.do(t, "DELETE", "/api/sessions/"+auth.SessionID(phone), w.admin, nil); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	w.closed(t, "signed out by an administrator", onPhone, u.ID)

	// A new password signs every other device out.
	phone, tv = login("anna"), login("anna")
	onPhone, onTV = w.openEverything(t, phone, night), w.openEverything(t, tv, night)
	if res := w.do(t, "PUT", fmt.Sprintf("/api/users/%d/password", u.ID), phone, map[string]any{"current": "correct-horse-1", "password": "another-horse-2"}); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	w.closed(t, "a new password", onTV, u.ID)
	w.stillOpen(t, "a new password", onPhone, u.ID)

	// The account is deleted: everything of it goes, and what it was invited to.
	other := party(t, w.do(t, "POST", "/api/syncplay/rooms", w.parent, map[string]any{"itemId": w.fine.ID}))
	if res := w.seat(t, w.parent, other, "invite", map[string]any{"userIds": []int64{u.ID}}); res.Code != 200 {
		t.Fatal(res.Code, res.Body)
	}
	if res := w.do(t, "DELETE", fmt.Sprintf("/api/users/%d", u.ID), w.admin, nil); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	w.closed(t, "the account deleted", onPhone, u.ID)
	// Whoever is given that id next doesn't inherit the invitation.
	if n := len(w.syncEngine().RoomsFor(syncplayUser(u.ID))); n != 0 {
		t.Errorf("a deleted account is still invited to %d watch parties", n)
	}
	if n := w.open.count(); n != 0 {
		t.Errorf("%d streams are still counted as open", n)
	}
}

// Switching profile on a device: everything the device had open as the profile before
// ends, so nothing goes on acting as it, and the device starts again as the new one.
func TestSwitchingProfileEndsWhatTheDeviceHadOpen(t *testing.T) {
	w := newLimitsWorld(t)
	kidU, _ := w.user(t, "kid", "kids", false, `{"maxAge":7,"off":["remoteControl"]}`)
	w.allowOthers(t, w.parentID)
	night := party(t, w.do(t, "POST", "/api/syncplay/rooms", w.admin, map[string]any{"itemId": w.fine.ID}))
	h := w.openEverything(t, w.parent, night)
	// The parent's TV is playing something for grown-ups when the child takes it over.
	grownUp := w.hidden["a rating for films only"]
	if res := w.do(t, "POST", "/api/remote-control/devices/"+h.tv+"/state", w.parent, map[string]any{"v": 1, "status": "playing", "itemId": grownUp.ID}); res.Code != 200 {
		t.Fatal(res.Code, res.Body)
	}
	res := w.do(t, "POST", "/api/profiles/switch", w.parent, map[string]any{"name": "kid", "password": "correct-horse-1", "device": "Living room TV"})
	if res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	kid, _ := body(res)["token"].(string)
	w.closed(t, "switching profile", h, w.parentID)

	// The old sign-in can do nothing more with the device or the seat.
	for _, r := range [][2]string{
		{"POST", "/api/remote-control/devices/" + h.tv + "/state"}, {"GET", "/api/remote-control/devices/" + h.tv + "/channel"},
		{"POST", "/api/syncplay/rooms/" + h.seat.ID + "/command"},
	} {
		if res := w.do(t, r[0], r[1], w.parent, map[string]any{"v": 1, "status": "idle", "member": h.seat.You, "type": "play"}); res.Code != 401 {
			t.Errorf("%s %s with the sign-in from before the switch: %d", r[0], r[1], res.Code)
		}
	}
	// The new one can't take over the old one's device id or seat: it is a new device, and a
	// new member, as the child.
	if res := w.do(t, "POST", "/api/remote-control/devices/"+h.tv+"/state", kid, map[string]any{"v": 1, "status": "idle"}); res.Code != 404 {
		t.Errorf("the new profile reported as the old one's device: %d %s", res.Code, res.Body)
	}
	if res := w.do(t, "POST", "/api/syncplay/rooms/"+h.seat.ID+"/command", kid, map[string]any{"member": h.seat.You, "type": "play"}); res.Code != 404 {
		t.Errorf("the new profile used the old one's seat: %d %s", res.Code, res.Body)
	}
	tv, ch := w.tv(t, kid, "Living room TV")
	d, here := w.remoteHub().Device(tv)
	if !here || d.UserID != kidU.ID || tv == h.tv {
		t.Fatalf("the device after the switch: %+v", d)
	}
	// It is the child's device now: its limits decide what plays there, whoever asks.
	if res := w.command(t, w.admin, kid, tv, ch, map[string]any{"type": "play", "itemId": grownUp.ID}); res.Code == 200 {
		t.Error("the owner played a film for grown-ups on a device that had switched to a Kids profile")
	}
	// And the child, kept from steering devices, steers none from it.
	if res := w.do(t, "GET", "/api/remote-control/devices", kid, nil); res.Code != 403 {
		t.Errorf("%d %s", res.Code, res.Body)
	}
	mine := party(t, w.do(t, "POST", "/api/syncplay/join", kid, map[string]any{"code": night.Code}))
	if mine.You == h.seat.You {
		t.Error("the new profile was given the old one's seat")
	}
	for _, m := range w.room(t, kid, mine).Members {
		if m.Name == "parent" {
			t.Error("the profile the device left is still seated in the watch party")
		}
	}
}

// Taking away an account's leave to control other accounts' devices shows at once in the
// stream it is watching devices through.
func TestRevokingControlOfOthersShowsAtOnce(t *testing.T) {
	w := newLimitsWorld(t)
	_, kid := w.user(t, "kid", "kids", false, "")
	w.allowOthers(t, w.parentID)
	tv, _ := w.tv(t, kid, "Kid's TV")
	events := openSSE(t, w.srv, "/api/remote-control/events", w.parent)
	if first := events.next("snapshot"); !strings.Contains(first, tv) {
		t.Fatalf("with leave: %s", first)
	}
	if res := w.do(t, "PUT", fmt.Sprintf("/api/remote-control/accounts/%d", w.parentID), w.admin, map[string]any{"controlOthers": false}); res.Code != 200 {
		t.Fatal(res.Code, res.Body)
	}
	if next := lastWord(t, events, "snapshot"); strings.Contains(next, tv) {
		t.Errorf("after leave was taken away: %s", next)
	}
	// A device's own limits changing shows at once to whoever is watching it, too.
	w.allowOthers(t, w.parentID)
	lastWord(t, events, "snapshot")
	if res := w.do(t, "POST", "/api/remote-control/devices/"+tv+"/state", kid, map[string]any{"v": 1, "status": "playing", "itemId": w.fine.ID}); res.Code != 200 {
		t.Fatal(res.Code, res.Body)
	}
	events.until("device", func(d string) bool { return strings.Contains(d, w.fine.Title) })
	parent, _ := w.Auth.User(context.Background(), w.parentID)
	w.setLimits(t, w.admin, parent.ID, `{`+w.noLimits+`,"blockItems":[`+fmt.Sprint(w.fine.ID)+`]}`)
	if next := lastWord(t, events, "snapshot"); strings.Contains(next, w.fine.Title) || !strings.Contains(next, `"hidden":true`) {
		t.Errorf("after the watcher's limits changed: %s", next)
	}
}

// ---------------------------------------------------------------------------
// Games in standalone emulators
// ---------------------------------------------------------------------------

// heldEmulators stands in for WittyPlay, with games that go on until the test ends them.
type heldEmulators struct {
	mu    sync.Mutex
	over  map[string]chan struct{}
	calls *wittyPlayCalls
}

func holdEmulators(t *testing.T) *heldEmulators {
	t.Helper()
	h := &heldEmulators{over: map[string]chan struct{}{}, calls: fakeWittyPlayCalls(t)}
	inner := callWittyPlay
	callWittyPlay = func(ctx context.Context, name string, args []any, out []any) error {
		if name != "stream.Session.Wait" {
			return inner(ctx, name, args, out)
		}
		select {
		case <-h.ended(args[0].(string)):
			*out[0].(*bool) = true
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}
	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for id, c := range h.over {
			select {
			case <-c:
			default:
				close(c)
			}
			delete(h.over, id)
		}
	})
	return h
}

func (h *heldEmulators) ended(id string) chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.over[id] == nil {
		h.over[id] = make(chan struct{})
	}
	return h.over[id]
}

// start has an account start a game, which WittyPlay names id.
func (h *heldEmulators) start(t *testing.T, ts *testServer, tok string, game int64, id string) {
	t.Helper()
	h.calls.mu.Lock()
	h.calls.result = emulatorResult{Session: &emulatorSession{ID: id, Backend: "docker+kasm", Emulator: "pcsx2", Name: "PCSX2",
		KasmURL: "/kasm/" + id + "/" + kasmTicket + "/", Stop: "/docker/sessions/" + id, Pause: "/docker/sessions/" + id + "/pause", Resume: "/docker/sessions/" + id + "/resume"}}
	h.calls.mu.Unlock()
	if w := ts.do(t, "POST", fmt.Sprintf("/api/games/%d/play/emulator", game), tok, map[string]string{"emulator": "pcsx2"}); w.Code != 201 {
		t.Fatalf("starting a game: %d %s", w.Code, w.Body)
	}
}

// stopped waits for WittyPlay to be asked to end a game, as the account whose it is.
func stopped(t *testing.T, got func() []seen, id string, user int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, s := range got() {
			if s.Path == "/docker/sessions/"+id && s.User == fmt.Sprint(user) && s.Admin != "1" {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("WittyPlay was never asked to end game %s as account %d: %+v", id, user, got())
}

// A game in an emulator belongs to the sign-in that started it and follows its profile's
// limits for as long as it runs, though its picture reaches the player by a ticket alone.
func TestEmulatorGamesFollowTheProfile(t *testing.T) {
	t.Setenv("WMS_WITTYPLAY_RUNNER", "/nonexistent/runner")
	got := fakeWittyPlayService(t)
	held := holdEmulators(t)
	was := emulatorWatchEvery
	emulatorWatchEvery = 20 * time.Millisecond
	t.Cleanup(func() { emulatorWatchEvery = was })
	ts := newTestServer(t)
	game, _ := ps2Game(t, ts, "Open Game", "ESRB E")
	_, admin := ts.user(t, "boss", "adult", true, "")
	grown, adult := ts.user(t, "grown", "adult", false, "")
	kidU, kid := ts.user(t, "kid", "kids", false, `{"maxAge":10,"time":{"schoolDayMinutes":1,"weekendMinutes":1}}`)
	clock := parentalClock(ts, 2, "16:00")
	desktop := func(id string) int {
		return ts.do(t, "GET", "/api/modules/wittyplay/kasm/"+id+"/"+kasmTicket+"/vnc.html", "", nil).Code
	}
	const a, b, c, d = "aaaaaaaa01", "bbbbbbbb02", "cccccccc03", "dddddddd04"

	// Signing out ends the game that sign-in started, and its desktop is no longer passed on.
	held.start(t, ts, adult, game.ID, a)
	if code := desktop(a); code != 200 {
		t.Fatalf("the desktop of a game that is going: %d", code)
	}
	if code := desktop(b); code != 404 {
		t.Fatalf("the desktop of a game nobody started: %d", code)
	}
	ts.Auth.Logout(context.Background(), adult)
	stopped(t, got, a, grown.ID)
	close(held.ended(a)) // WittyPlay says it has ended
	eventually(t, "an ended game's desktop to stop being passed on", func() bool { return desktop(a) == 404 })

	// A profile with screen time: playing counts, and the game is ended when the time is up.
	held.start(t, ts, kid, game.ID, b)
	if code := desktop(b); code != 200 {
		t.Fatalf("the child's desktop with time left: %d", code)
	}
	eventually(t, "the game to be counted as screen time", func() bool {
		clock.add(10 * time.Second)
		time.Sleep(30 * time.Millisecond)
		var st struct {
			UsedSec int `json:"usedSec"`
		}
		readBody(t, ts.do(t, "GET", "/api/me/screen-time", kid, nil).Body.Bytes(), &st)
		return st.UsedSec >= 20
	})
	eventually(t, "the time to run out", func() bool {
		clock.add(10 * time.Second)
		time.Sleep(30 * time.Millisecond)
		return desktop(b) == 404
	})
	stopped(t, got, b, kidU.ID)
	close(held.ended(b))
	// With time up, another can't be started.
	if w := ts.do(t, "POST", fmt.Sprintf("/api/games/%d/play/emulator", game.ID), kid, map[string]string{"emulator": "pcsx2"}); w.Code != 403 || body(w)["code"] != "screen-time" {
		t.Errorf("starting a game with time up: %d %s", w.Code, w.Body)
	}

	// Games turned off for a profile while it plays: the game is ended.
	_, teen := ts.user(t, "teen", "guest", false, `{"maxAge":13}`)
	teenID := ts.userID(t, teen)
	held.start(t, ts, teen, game.ID, c)
	ts.setLimits(t, admin, teenID, `{"maxAge":13,"off":["games"]}`)
	stopped(t, got, c, teenID)
	if code := desktop(c); code != 404 {
		t.Errorf("the desktop of a profile with games off: %d", code)
	}
	close(held.ended(c))
	for _, r := range [][2]string{{"GET", fmt.Sprintf("/api/games/%d/emulators", game.ID)}, {"POST", fmt.Sprintf("/api/games/%d/play/emulator", game.ID)},
		{"GET", "/api/modules/wittyplay/docker/sessions"}, {"POST", "/api/modules/wittyplay/docker/sessions/" + c + "/resume"}} {
		if w := ts.do(t, r[0], r[1], teen, map[string]string{"emulator": "pcsx2"}); w.Code != 403 || body(w)["code"] != "feature-off" {
			t.Errorf("%s %s with games off: %d %s", r[0], r[1], w.Code, w.Body)
		}
	}

	// Switching profile ends the game the device was playing as the profile before.
	_, mum := ts.user(t, "mum", "adult", false, "")
	mumID := ts.userID(t, mum)
	held.start(t, ts, mum, game.ID, d)
	if w := ts.do(t, "POST", "/api/profiles/switch", mum, map[string]any{"name": "teen", "password": "correct-horse-1"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	stopped(t, got, d, mumID)
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("waited ten seconds for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ---------------------------------------------------------------------------
// Pairing, and the extras of a title
// ---------------------------------------------------------------------------

// Signing a TV in from a phone follows the rules for who manages whom: an adult who has
// limits of any kind (a feature turned off is one) signs a device in as themselves only,
// and the device then has those limits; a Kids profile signs no device in at all.
func TestPairingFollowsWhoManagesWhom(t *testing.T) {
	ts := newTestServer(t)
	film := playableFilm(t, ts, "G")
	ts.user(t, "owner", "adult", true, "")
	kidU, kid := ts.user(t, "sam", "kids", false, oneMinuteADay)
	_, mum := ts.user(t, "mum", "adult", false, "")
	_, limited := ts.user(t, "uncle", "adult", false, `{"maxAge":-1,"allowUnrated":true,"liveTv":true,"libraryChannels":true,"tunerChannels":true,"off":["syncplay"]}`)
	c := withClock(ts)
	approve := func(tok string, as int64) (int, string) {
		t.Helper()
		p := startAuthPair(t, ts)
		b := map[string]any{"pin": pinAt(ts, c, p.SessionID, 0)}
		if as != 0 {
			b["userId"] = as
		}
		w := ts.do(t, "POST", "/api/auth/pair/"+p.SessionID+"/approve", tok, b)
		if w.Code != 200 {
			return w.Code, ""
		}
		_, got := pollAuthPair(t, ts, p.SessionID, p.DeviceSecret)
		tv, _ := got["token"].(string)
		return 200, tv
	}
	if code, _ := approve(kid, 0); code != 403 {
		t.Errorf("a Kids profile signed a device in: %d", code)
	}
	if code, _ := approve(limited, kidU.ID); code != 403 {
		t.Errorf("an adult with limits signed a device in as a Kids profile: %d", code)
	}
	// As themselves: the device has their limits.
	code, tv := approve(limited, 0)
	if code != 200 || tv == "" {
		t.Fatalf("an adult with limits signed a device in as themselves: %d", code)
	}
	if w := ts.do(t, "POST", "/api/syncplay/rooms", tv, map[string]any{"itemId": film.ID}); w.Code != 403 || body(w)["code"] != "feature-off" {
		t.Errorf("the paired device started a watch party: %d %s", w.Code, w.Body)
	}
	// A parent signs the TV in as the child: no password asked, and it is the child's device,
	// with the child's screen time.
	code, tv = approve(mum, kidU.ID)
	if code != 200 || tv == "" {
		t.Fatalf("a parent signed a device in as a Kids profile: %d", code)
	}
	if w := ts.do(t, "GET", "/api/me", tv, nil); !strings.Contains(w.Body.String(), `"name":"sam"`) {
		t.Fatalf("the device is %s", w.Body)
	}
	clock := parentalClock(ts, 2, "16:00")
	spend(t, ts, clock, tv, film.ID, 61*time.Second)
	if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/file", film.ID), tv, nil); w.Code != 403 || body(w)["code"] != "screen-time" {
		t.Errorf("the paired TV playing with the child's time up: %d %s", w.Code, w.Body)
	}
	// And remote control knows it as the child's: the phone that paired it can't steer it
	// without leave, and it announces itself as the child.
	res := ts.do(t, "POST", "/api/remote-control/devices", tv, map[string]any{"v": 1, "name": "TV", "kind": "tv", "capabilities": map[string]any{"mediaKinds": []string{"video"}}})
	id, _ := body(res)["deviceId"].(string)
	if d, here := ts.remoteHub().Device(id); !here || d.UserID != kidU.ID {
		t.Fatalf("the paired TV as a device: %+v", d)
	}
	if w := ts.do(t, "POST", "/api/remote-control/devices/"+id+"/commands", mum, map[string]any{"v": 1, "type": "pause"}); w.Code != 404 {
		t.Errorf("a parent with no leave steered the child's TV: %d %s", w.Code, w.Body)
	}
}

// A title's extras (its canvas, stems, skip markers, where to watch it) follow every kind
// of limit, and the ones that play are screen time.
func TestMediaExtrasFollowEveryKindOfLimit(t *testing.T) {
	w := newLimitsWorld(t)
	toks, _ := w.profiles(t)
	extras := []string{"/api/media/%d/canvas", "/api/items/%d/canvas", "/api/media/%d/canvas/file", "/api/media/%d/stems", "/api/media/%d/stems/vocals",
		"/api/media/%d/markers", "/api/media/%d/where-to-watch"}
	for _, kind := range limitKinds {
		hidden := w.hidden[kind]
		for _, e := range extras {
			target := fmt.Sprintf(e, hidden.ID)
			if res := w.do(t, "GET", target, toks[kind], nil); res.Code != 404 || strings.Contains(res.Body.String(), "Zebra") {
				t.Errorf("%s: GET %s for the hidden title: %d %s", kind, target, res.Code, res.Body)
			}
			// The owner is refused nothing: whatever the answer, it isn't "hidden".
			if res := w.do(t, "GET", fmt.Sprintf("/api/media/%d/markers", hidden.ID), w.admin, nil); res.Code != 200 {
				t.Errorf("%s: the owner's markers: %d", kind, res.Code)
			}
		}
		if res := w.do(t, "POST", fmt.Sprintf("/api/media/%d/stems/approximate", hidden.ID), toks[kind], nil); res.Code != 404 {
			t.Errorf("%s: asked for stems of the hidden title: %d %s", kind, res.Code, res.Body)
		}
		if res := w.do(t, "GET", fmt.Sprintf("/api/media/%d/markers", w.fine.ID), toks[kind], nil); res.Code != 200 {
			t.Errorf("%s: the markers of a title it may see: %d", kind, res.Code)
		}
	}
	// The ones that are media are refused when time is up; the ones that describe aren't.
	_, kid := w.user(t, "kid", "kids", false, oneMinuteADay)
	clock := parentalClock(w.testServer, 2, "16:00")
	spend(t, w.testServer, clock, kid, w.fine.ID, 61*time.Second)
	for _, e := range []string{"/api/media/%d/canvas/file", "/api/media/%d/stems/vocals"} {
		if res := w.do(t, "GET", fmt.Sprintf(e, w.fine.ID), kid, nil); res.Code != 403 || body(res)["code"] != "screen-time" {
			t.Errorf("GET %s with time up: %d %s", e, res.Code, res.Body)
		}
	}
	for _, e := range []string{"/api/media/%d/canvas", "/api/media/%d/stems", "/api/media/%d/markers"} {
		if res := w.do(t, "GET", fmt.Sprintf(e, w.fine.ID), kid, nil); res.Code != 200 {
			t.Errorf("GET %s with time up: %d %s", e, res.Code, res.Body)
		}
	}
}

// syncplayUser is an account as the watch party engine is asked about it, with no seat.
func syncplayUser(id int64) syncplay.User { return syncplay.User{ID: id} }
