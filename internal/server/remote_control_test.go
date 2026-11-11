// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// sse is one open event stream, read an event at a time.
type sse struct {
	t      *testing.T
	events chan [2]string
	close  func()
}

// openSSE opens a stream and fails the test if it isn't one.
func openSSE(t *testing.T, srv *httptest.Server, path, tok string) *sse {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	ctx, cancel := context.WithCancel(context.Background())
	res, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		cancel()
		t.Fatalf("%s: %d %s", path, res.StatusCode, res.Header.Get("Content-Type"))
	}
	s := &sse{t: t, events: make(chan [2]string, 64), close: func() { cancel(); res.Body.Close() }}
	t.Cleanup(s.close)
	go func() {
		defer close(s.events)
		r := bufio.NewReader(res.Body)
		var event, data string
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\n")
			switch {
			case strings.HasPrefix(line, "event: "):
				event = line[7:]
			case strings.HasPrefix(line, "data: "):
				data = line[6:]
			case line == "" && event != "":
				s.events <- [2]string{event, data}
				event, data = "", ""
			}
		}
	}()
	return s
}

// next is the next event, which must be of this kind.
func (s *sse) next(want string) string {
	s.t.Helper()
	select {
	case e, ok := <-s.events:
		if !ok {
			s.t.Fatalf("the stream ended while waiting for %q", want)
		}
		if e[0] != want {
			s.t.Fatalf("event %q (%s), want %q", e[0], e[1], want)
		}
		return e[1]
	case <-time.After(20 * time.Second):
		s.t.Fatalf("no %q event came", want)
	}
	return ""
}

// until skips events until one of this kind satisfies ok.
func (s *sse) until(want string, ok func(data string) bool) string {
	s.t.Helper()
	deadline := time.After(20 * time.Second)
	for {
		select {
		case e, open := <-s.events:
			if !open {
				s.t.Fatalf("the stream ended while waiting for %q", want)
			}
			if e[0] == want && ok(e[1]) {
				return e[1]
			}
		case <-deadline:
			s.t.Fatalf("no %q event that fits came", want)
		}
	}
}

func body(w *httptest.ResponseRecorder) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out
}

// remoteWorld is a house: a film for grown-ups and one for everyone, an administrator, a
// parent and a child.
type remoteWorld struct {
	*testServer
	srv                *httptest.Server
	admin, parent, kid string
	parentID, kidID    int64
	adultFilm, kidFilm int64
	song               int64
}

func newRemoteWorld(t *testing.T) *remoteWorld {
	ts := newTestServer(t)
	root := t.TempDir()
	lib := ts.library(t, "movies", root)
	w := &remoteWorld{testServer: ts}
	w.adultFilm = ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(root, "a.mkv"), Title: "The Night Shift", ContentRating: "R"}).ID
	w.kidFilm = ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(root, "b.mkv"), Title: "Puddle Day", ContentRating: "G"}).ID
	w.song = ts.item(t, &library.Item{LibraryID: lib, Kind: "audio", Path: filepath.Join(root, "c.flac"), Title: "Tune", ContentRating: "G"}).ID
	_, w.admin = ts.user(t, "boss", "adult", true, "")
	p, ptok := ts.user(t, "parent", "adult", false, "")
	k, ktok := ts.user(t, "kid", "kids", false, "")
	w.parent, w.kid, w.parentID, w.kidID = ptok, ktok, p.ID, k.ID
	w.srv = httptest.NewServer(ts.h)
	t.Cleanup(w.srv.Close)
	return w
}

// device announces a TV for a sign-in and opens its channel.
func (w *remoteWorld) device(t *testing.T, tok, name string) (string, *sse) {
	t.Helper()
	res := w.do(t, "POST", "/api/remote-control/devices", tok, map[string]any{
		"v": 1, "name": name, "kind": "tv",
		"capabilities": map[string]any{"mediaKinds": []string{"video"}, "features": []string{"seek", "volume", "queue", "teleport"}},
	})
	if res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	id, _ := body(res)["deviceId"].(string)
	ch := openSSE(t, w.srv, "/api/remote-control/devices/"+id+"/channel", tok)
	ch.next("hello")
	return id, ch
}

// obey answers the next command on a channel, and returns it.
func (w *remoteWorld) obey(t *testing.T, tok, id string, ch *sse, ok bool, why string) map[string]any {
	t.Helper()
	var cmd map[string]any
	if err := json.Unmarshal([]byte(ch.next("command")), &cmd); err != nil {
		t.Fatal(err)
	}
	res := w.do(t, "POST", "/api/remote-control/devices/"+id+"/ack", tok, map[string]any{"id": cmd["id"], "ok": ok, "error": why})
	if res.Code != 200 {
		t.Fatalf("ack: %d %s", res.Code, res.Body)
	}
	return cmd
}

// send posts a command and reports the answer, without waiting for it here.
func (w *remoteWorld) send(t *testing.T, tok, id string, cmd map[string]any) chan *httptest.ResponseRecorder {
	out := make(chan *httptest.ResponseRecorder, 1)
	go func() { out <- w.do(t, "POST", "/api/remote-control/devices/"+id+"/commands", tok, cmd) }()
	return out
}

func (w *remoteWorld) allow(t *testing.T, user int64, yes bool) {
	t.Helper()
	if res := w.do(t, "PUT", fmt.Sprintf("/api/remote-control/accounts/%d", user), w.admin, map[string]any{"controlOthers": yes}); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
}

func deviceIDs(w *httptest.ResponseRecorder) []string {
	var out struct {
		Devices []struct {
			ID string `json:"id"`
		} `json:"devices"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	ids := []string{}
	for _, d := range out.Devices {
		ids = append(ids, d.ID)
	}
	return ids
}

func TestRemoteControlACommandReachesTheDeviceAndIsAnswered(t *testing.T) {
	w := newRemoteWorld(t)
	// The same account on a second sign-in: its phone.
	tok2, _, err := w.Auth.Login(context.Background(), "parent", "correct-horse-1", "phone", false)
	if err != nil {
		t.Fatal(err)
	}
	tv, ch := w.device(t, w.parent, "Living room")

	// Capabilities were negotiated: what the server doesn't know was dropped.
	list := w.do(t, "GET", "/api/remote-control/devices", tok2, nil)
	if !strings.Contains(list.Body.String(), `"features":["seek","volume","queue"]`) || !strings.Contains(list.Body.String(), `"online":true`) || !strings.Contains(list.Body.String(), `"mine":true`) {
		t.Fatalf("%s", list.Body)
	}
	if strings.Contains(list.Body.String(), `"thisSession":true`) {
		t.Fatal("the phone was told the TV is itself")
	}

	answer := w.send(t, tok2, tv, map[string]any{"v": 1, "type": "play", "itemId": w.kidFilm, "positionSec": 30.5, "somethingNew": true})
	cmd := w.obey(t, w.parent, tv, ch, true, "")
	if cmd["type"] != "play" || cmd["itemId"] != float64(w.kidFilm) || cmd["positionSec"] != 30.5 || cmd["v"] != float64(1) || cmd["id"] == "" || cmd["sentAt"] == nil {
		t.Fatalf("%v", cmd)
	}
	if s, _ := cmd["sender"].(map[string]any); s["userName"] != "parent" {
		t.Fatalf("%v", cmd["sender"])
	}
	if res := <-answer; res.Code != 200 || body(res)["ok"] != true {
		t.Fatalf("%d %s", res.Code, res.Body)
	}

	// The device says no: the controller is told why.
	answer = w.send(t, tok2, tv, map[string]any{"type": "seek", "bySec": -10})
	w.obey(t, w.parent, tv, ch, false, "Nothing is playing.")
	if res := <-answer; res.Code != 409 || body(res)["code"] != "refused" || body(res)["error"] != "Nothing is playing." {
		t.Fatalf("%d %s", res.Code, res.Body)
	}

	// The device reports; a controller's list shows it, with the title.
	if res := w.do(t, "POST", "/api/remote-control/devices/"+tv+"/state", w.parent, map[string]any{"v": 1, "status": "playing", "itemId": w.kidFilm, "positionSec": 31, "durationSec": 5400, "rate": 1, "volume": 0.5,
		"audioTracks": []map[string]any{{"id": "0", "label": "English", "lang": "en"}}}); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	list = w.do(t, "GET", "/api/remote-control/devices", tok2, nil)
	for _, want := range []string{`"title":"Puddle Day"`, `"status":"playing"`, `"positionSec":31`, `"itemKind":"video"`, `"label":"English"`} {
		if !strings.Contains(list.Body.String(), want) {
			t.Errorf("no %s in %s", want, list.Body)
		}
	}
}

func TestRemoteControlRefusals(t *testing.T) {
	w := newRemoteWorld(t)
	tv, ch := w.device(t, w.parent, "Living room")
	path := "/api/remote-control/devices/" + tv

	for name, c := range map[string]struct {
		cmd  map[string]any
		code int
		what string
	}{
		"unknown command":            {map[string]any{"type": "explode"}, 400, "bad-command"},
		"volume out of range":        {map[string]any{"type": "volume", "level": 4}, 400, "bad-command"},
		"play with nothing":          {map[string]any{"type": "play"}, 400, "bad-command"},
		"something it never said":    {map[string]any{"type": "navigate", "key": "up"}, 409, "unsupported"},
		"a kind it doesn't play":     {map[string]any{"type": "play", "itemId": w.song}, 409, "unsupported"},
		"an item that doesn't exist": {map[string]any{"type": "play", "itemId": 99999}, 404, "no-item"},
		"cast from nowhere":          {map[string]any{"type": "cast", "from": "nope"}, 404, "no-device"},
		"cast from itself":           {map[string]any{"type": "cast", "from": tv}, 400, "bad-command"},
	} {
		res := w.do(t, "POST", path+"/commands", w.parent, c.cmd)
		if res.Code != c.code || body(res)["code"] != c.what {
			t.Errorf("%s: %d %s", name, res.Code, res.Body)
		}
	}
	if res := w.do(t, "POST", path+"/commands", w.parent, []byte(`{"type":"pause","pad":"`+strings.Repeat("x", 9000)+`"}`)); res.Code != 413 {
		t.Errorf("a long command: %d", res.Code)
	}
	if res := w.do(t, "POST", path+"/state", w.parent, []byte(`{"status":"idle","screen":"`+strings.Repeat("x", 40000)+`"}`)); res.Code != 413 {
		t.Errorf("a long state: %d", res.Code)
	}
	if res := w.do(t, "POST", path+"/commands", w.parent, []byte(`not json`)); res.Code != 400 {
		t.Errorf("not JSON: %d", res.Code)
	}
	// None of that reached the device.
	select {
	case e := <-ch.events:
		t.Fatalf("a refused command reached the device: %v", e)
	case <-time.After(100 * time.Millisecond):
	}

	// A device that isn't there: said at once, never waited for.
	began := time.Now()
	if res := w.do(t, "POST", "/api/remote-control/devices/nosuchdevice/commands", w.parent, map[string]any{"type": "pause"}); res.Code != 404 {
		t.Errorf("%d %s", res.Code, res.Body)
	}
	ch.close()
	deadline := time.Now().Add(10 * time.Second)
	for {
		res := w.do(t, "POST", path+"/commands", w.parent, map[string]any{"type": "pause"})
		if res.Code == 409 && body(res)["code"] == "offline" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("a command to a device whose channel closed: %d %s", res.Code, res.Body)
		}
		time.Sleep(20 * time.Millisecond) // the server hasn't noticed the channel close yet
	}
	if time.Since(began) > 9*time.Second {
		t.Error("an absent device was waited for")
	}

	// Only the sign-in that announced a device speaks for it. Everyone else is told it isn't there.
	_, other := w.user(t, "lodger", "adult", false, "")
	again, _, _ := w.Auth.Login(context.Background(), "parent", "correct-horse-1", "tablet", false)
	for _, tok := range []string{other, again, w.admin} {
		for _, r := range [][2]string{{"POST", path + "/state"}, {"POST", path + "/ack"}, {"GET", path + "/channel"}, {"DELETE", path}} {
			if res := w.do(t, r[0], r[1], tok, map[string]any{}); res.Code != 404 {
				t.Errorf("%s %s from another sign-in: %d %s", r[0], r[1], res.Code, res.Body)
			}
		}
	}

	// The owner's switch.
	if res := w.do(t, "PUT", "/api/settings", w.admin, map[string]string{"remoteControl": "0"}); res.Code != 200 {
		t.Fatal(res.Code)
	}
	for _, r := range [][2]string{{"GET", "/api/remote-control/devices"}, {"POST", "/api/remote-control/devices"}, {"POST", path + "/commands"}, {"GET", "/api/remote-control/events"}, {"POST", path + "/state"}} {
		if res := w.do(t, r[0], r[1], w.parent, map[string]any{"type": "pause"}); res.Code != 403 || body(res)["code"] != "disabled" {
			t.Errorf("%s %s with remote control off: %d %s", r[0], r[1], res.Code, res.Body)
		}
	}
	if res := w.do(t, "GET", "/api/remote-control", w.parent, nil); res.Code != 200 || body(res)["enabled"] != false {
		t.Errorf("%d %s", res.Code, res.Body)
	}
}

// By default an account controls its own devices and no others. An administrator may let
// an account control other accounts'; even then nobody's limits are widened.
func TestRemoteControlAcrossAccounts(t *testing.T) {
	w := newRemoteWorld(t)
	tv, tvCh := w.device(t, w.parent, "Living room")
	tablet, tabletCh := w.device(t, w.kid, "Kid's tablet")

	// Each sees their own, and can't touch the other's: it isn't there.
	if ids := deviceIDs(w.do(t, "GET", "/api/remote-control/devices", w.kid, nil)); len(ids) != 1 || ids[0] != tablet {
		t.Fatalf("%v", ids)
	}
	if ids := deviceIDs(w.do(t, "GET", "/api/remote-control/devices", w.admin, nil)); len(ids) != 0 {
		t.Fatalf("an administrator sees other accounts' devices without being given that: %v", ids)
	}
	if res := w.do(t, "POST", "/api/remote-control/devices/"+tv+"/commands", w.kid, map[string]any{"type": "pause"}); res.Code != 404 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	if res := w.do(t, "POST", "/api/remote-control/devices/"+tablet+"/commands", w.parent, map[string]any{"type": "cast", "from": tv}); res.Code != 404 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}

	// Only an administrator changes that.
	if res := w.do(t, "PUT", fmt.Sprintf("/api/remote-control/accounts/%d", w.kidID), w.parent, map[string]any{"controlOthers": true}); res.Code != 403 {
		t.Fatalf("%d", res.Code)
	}
	if res := w.do(t, "PUT", "/api/remote-control/accounts/9999", w.admin, map[string]any{"controlOthers": true}); res.Code != 404 {
		t.Fatalf("%d", res.Code)
	}
	if res := w.do(t, "PUT", fmt.Sprintf("/api/remote-control/accounts/%d", w.kidID), w.admin, map[string]any{}); res.Code != 400 {
		t.Fatalf("%d", res.Code)
	}
	w.allow(t, w.kidID, true)
	w.allow(t, w.parentID, true)
	if res := w.do(t, "GET", "/api/remote-control", w.kid, nil); body(res)["controlOthers"] != true {
		t.Fatalf("%s", res.Body)
	}
	accounts := w.do(t, "GET", "/api/remote-control/accounts", w.admin, nil).Body.String()
	if !strings.Contains(accounts, `"name":"kid","displayName":"","kind":"kids","isAdmin":false,"controlOthers":true`) || !strings.Contains(accounts, `"name":"boss","displayName":"","kind":"adult","isAdmin":true,"controlOthers":false`) {
		t.Fatalf("%s", accounts)
	}

	// The child may now pause the living room TV...
	answer := w.send(t, w.kid, tv, map[string]any{"type": "pause"})
	w.obey(t, w.parent, tv, tvCh, true, "")
	if res := <-answer; res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	// ...and put on something they may watch...
	answer = w.send(t, w.kid, tv, map[string]any{"type": "play", "itemId": w.kidFilm})
	w.obey(t, w.parent, tv, tvCh, true, "")
	if res := <-answer; res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	// ...but not something they may not see, alone or tucked into a queue: to them it doesn't exist.
	for _, cmd := range []map[string]any{
		{"type": "play", "itemId": w.adultFilm},
		{"type": "play", "queue": []int64{w.kidFilm, w.adultFilm}, "queueIndex": 0},
	} {
		if res := w.do(t, "POST", "/api/remote-control/devices/"+tv+"/commands", w.kid, cmd); res.Code != 404 || body(res)["code"] != "no-item" {
			t.Fatalf("a Kids profile played what it can't see on a grown-up's TV: %d %s", res.Code, res.Body)
		}
	}
	// The parent may not make the child's tablet play what the child's account may not see.
	for _, cmd := range []map[string]any{
		{"type": "play", "itemId": w.adultFilm},
		{"type": "play", "queue": []int64{w.kidFilm, w.adultFilm}, "queueIndex": 0},
	} {
		if res := w.do(t, "POST", "/api/remote-control/devices/"+tablet+"/commands", w.parent, cmd); res.Code != 403 || body(res)["code"] != "not-allowed-there" {
			t.Fatalf("a restricted account's device was made to play what it can't: %d %s", res.Code, res.Body)
		}
	}
	// Nor by handing over what the TV is playing.
	if res := w.do(t, "POST", "/api/remote-control/devices/"+tv+"/state", w.parent, map[string]any{"status": "playing", "itemId": w.adultFilm, "positionSec": 600, "durationSec": 6000, "rate": 1}); res.Code != 200 {
		t.Fatal(res.Code)
	}
	if res := w.do(t, "POST", "/api/remote-control/devices/"+tablet+"/commands", w.parent, map[string]any{"type": "cast", "from": tv}); res.Code != 403 {
		t.Fatalf("cast got round the limits: %d %s", res.Code, res.Body)
	}
	for _, ch := range []*sse{tvCh, tabletCh} {
		select {
		case e := <-ch.events:
			t.Fatalf("a refused command reached a device: %v", e)
		case <-time.After(100 * time.Millisecond):
		}
	}

	// What the TV is playing is not the child's to know: playing, and nothing about what.
	seen := w.do(t, "GET", "/api/remote-control/devices", w.kid, nil).Body.String()
	if strings.Contains(seen, "Night Shift") || strings.Contains(seen, fmt.Sprintf(`"itemId":%d`, w.adultFilm)) || strings.Contains(seen, `"positionSec":600`) {
		t.Fatalf("a Kids profile was shown what a grown-up is watching: %s", seen)
	}
	if !strings.Contains(seen, `"hidden":true`) || !strings.Contains(seen, `"status":"playing"`) {
		t.Fatalf("%s", seen)
	}
	// The parent sees it.
	if seen := w.do(t, "GET", "/api/remote-control/devices", w.parent, nil).Body.String(); !strings.Contains(seen, "The Night Shift") {
		t.Fatalf("%s", seen)
	}

	// Taken away again, it is taken away.
	w.allow(t, w.kidID, false)
	if res := w.do(t, "POST", "/api/remote-control/devices/"+tv+"/commands", w.kid, map[string]any{"type": "pause"}); res.Code != 404 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
}

func TestRemoteControlCastHandsOver(t *testing.T) {
	w := newRemoteWorld(t)
	tok2, _, _ := w.Auth.Login(context.Background(), "parent", "correct-horse-1", "bedroom", false)
	tv, tvCh := w.device(t, w.parent, "Living room")
	bedroom, bedCh := w.device(t, tok2, "Bedroom")

	// Nothing playing: nothing to hand over.
	if res := w.do(t, "POST", "/api/remote-control/devices/"+bedroom+"/commands", w.parent, map[string]any{"type": "cast", "from": tv}); res.Code != 409 || body(res)["code"] != "nothing-playing" {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	w.do(t, "POST", "/api/remote-control/devices/"+tv+"/state", w.parent, map[string]any{"status": "paused", "itemId": w.kidFilm, "positionSec": 1234.5, "durationSec": 5400, "rate": 1,
		"audio": "2", "subtitle": "off", "queue": []int64{w.kidFilm, w.adultFilm}, "queueIndex": 0})

	answer := w.send(t, w.parent, bedroom, map[string]any{"type": "cast", "from": tv})
	play := w.obey(t, tok2, bedroom, bedCh, true, "")
	if play["type"] != "play" || play["itemId"] != float64(w.kidFilm) || play["positionSec"] != 1234.5 || play["audio"] != "2" || play["subtitle"] != "off" {
		t.Fatalf("%v", play)
	}
	if q, _ := play["queue"].([]any); len(q) != 2 {
		t.Fatalf("the queue didn't come along: %v", play)
	}
	// Once the new device has it, the old one is told to stop.
	stop := w.obey(t, w.parent, tv, tvCh, true, "")
	if stop["type"] != "stop" {
		t.Fatalf("%v", stop)
	}
	if res := <-answer; res.Code != 200 || body(res)["sourceStopped"] != true {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
}

func TestRemoteControlEventsForControllers(t *testing.T) {
	w := newRemoteWorld(t)
	tv, tvCh := w.device(t, w.parent, "Living room")
	events := openSSE(t, w.srv, "/api/remote-control/events", w.parent)
	if snap := events.next("snapshot"); !strings.Contains(snap, tv) {
		t.Fatalf("%s", snap)
	}
	kidEvents := openSSE(t, w.srv, "/api/remote-control/events", w.kid)
	if snap := kidEvents.next("snapshot"); strings.Contains(snap, tv) {
		t.Fatalf("another account's stream shows the TV: %s", snap)
	}

	w.do(t, "POST", "/api/remote-control/devices/"+tv+"/state", w.parent, map[string]any{"status": "playing", "itemId": w.kidFilm, "positionSec": 12, "durationSec": 100, "rate": 1})
	got := events.until("device", func(d string) bool { return strings.Contains(d, `"status":"playing"`) })
	if !strings.Contains(got, `"title":"Puddle Day"`) || !strings.Contains(got, `"stateAt":`) {
		t.Fatalf("%s", got)
	}
	// The device goes: its channel closing is enough.
	tvCh.close()
	events.until("device", func(d string) bool { return strings.Contains(d, `"online":false`) })
	// And says goodbye properly.
	tv2, _ := w.device(t, w.parent, "Kitchen")
	events.until("device", func(d string) bool { return strings.Contains(d, tv2) })
	if res := w.do(t, "DELETE", "/api/remote-control/devices/"+tv2, w.parent, nil); res.Code != 200 {
		t.Fatal(res.Code)
	}
	events.until("gone", func(d string) bool { return strings.Contains(d, tv2) })
	// None of it went to the other account.
	select {
	case e := <-kidEvents.events:
		t.Fatalf("another account was told: %v", e)
	case <-time.After(400 * time.Millisecond):
	}
}

// quickBeat makes the streams here look at their sign-in often, for one test.
func quickBeat(t *testing.T) {
	was := remoteStreamBeat
	remoteStreamBeat = 50 * time.Millisecond
	t.Cleanup(func() { remoteStreamBeat = was })
}

// Signing a device out ends what it had open, at the next keep-alive.
func TestRemoteControlChannelEndsWithItsSignIn(t *testing.T) {
	quickBeat(t)
	w := newRemoteWorld(t)
	tv, ch := w.device(t, w.parent, "Living room")
	events := openSSE(t, w.srv, "/api/remote-control/events", w.parent)
	events.next("snapshot")
	w.Auth.Logout(context.Background(), w.parent)
	if bye := ch.next("bye"); !strings.Contains(bye, "signed-out") {
		t.Fatal(bye)
	}
	if _, here := w.remoteHub().Device(tv); here {
		t.Fatal("a signed-out device is still listed")
	}
	// The controller's stream, from the same sign-in, ends too.
	deadline := time.After(10 * time.Second)
	for {
		select {
		case _, open := <-events.events:
			if !open {
				return
			}
		case <-deadline:
			t.Fatal("a signed-out controller's stream stayed open")
		}
	}
}
