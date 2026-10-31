package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittycouch/gamestream"
	"github.com/wittywilla/wittycouch/gamestream/fake"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

func rigFake(t *testing.T) *fake.Sunshine {
	t.Helper()
	f, err := fake.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	return f
}

func rigJSON[T any](t *testing.T, code int, body []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("%d: %s: %v", code, body, err)
	}
	return v
}

// pairedRig adds the fake as a rig through the API and pairs it.
func pairedRig(t *testing.T, ts *testServer, f *fake.Sunshine, admin string) rigView {
	t.Helper()
	w := ts.do(t, "POST", "/api/rigs", admin, map[string]any{"address": fmt.Sprintf("127.0.0.1:%d", f.Port())})
	if w.Code != 201 {
		t.Fatalf("add: %d %s", w.Code, w.Body)
	}
	v := rigJSON[rigView](t, w.Code, w.Body.Bytes())
	w = ts.do(t, "POST", fmt.Sprintf("/api/rigs/%d/pair", v.ID), admin, nil)
	if w.Code != 200 {
		t.Fatalf("pair: %d %s", w.Code, w.Body)
	}
	p := rigJSON[rigPairing](t, w.Code, w.Body.Bytes())
	if p.State != "waiting" || len(p.PIN) != 4 {
		t.Fatalf("pairing: %+v", p)
	}
	if !f.WaitForPairing(5 * time.Second) {
		t.Fatal("nothing started pairing")
	}
	f.TypePIN(p.PIN)
	deadline := time.Now().Add(10 * time.Second)
	for {
		w = ts.do(t, "GET", fmt.Sprintf("/api/rigs/%d/pair", v.ID), admin, nil)
		st := rigJSON[rigPairing](t, w.Code, w.Body.Bytes())
		if st.State == "paired" {
			break
		}
		if st.State == "failed" || time.Now().After(deadline) {
			t.Fatalf("pairing: %+v", st)
		}
		time.Sleep(50 * time.Millisecond)
	}
	// The games arrive a moment after pairing finishes.
	for time.Now().Before(deadline) {
		w = ts.do(t, "GET", fmt.Sprintf("/api/rigs/%d/apps", v.ID), admin, nil)
		if apps := rigJSON[[]rigApp](t, w.Code, w.Body.Bytes()); len(apps) == len(f.Apps) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return v
}

func rigGames(t *testing.T, ts *testServer, token string, lib int64) []*library.Item {
	t.Helper()
	w := ts.do(t, "GET", fmt.Sprintf("/api/items?library=%d&kind=game", lib), token, nil)
	if w.Code != 200 {
		t.Fatalf("items: %d %s", w.Code, w.Body)
	}
	var r struct {
		Items []*library.Item `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	return r.Items
}

func gameNamed(t *testing.T, items []*library.Item, title string) *library.Item {
	t.Helper()
	for _, it := range items {
		if it.Title == title {
			return it
		}
	}
	t.Fatalf("no %q in %d games", title, len(items))
	return nil
}

func TestRigFromAddingToPlaying(t *testing.T) {
	ts := newTestServer(t)
	f := rigFake(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	v := pairedRig(t, ts, f, admin)
	if v.Name != f.Name {
		t.Errorf("name %q", v.Name)
	}

	// The library holds the games, and the whole-computer app stays out of it.
	items := rigGames(t, ts, admin, v.LibraryID)
	if len(items) != 2 {
		t.Fatalf("games: %d", len(items))
	}
	game := gameNamed(t, items, "Space Game")
	if game.Tagline != "Plays on "+f.Name || game.Console != library.RigConsole {
		t.Errorf("game: %+v", game)
	}
	if game.Poster == "" {
		t.Error("the game with its own picture has none")
	}
	// All the default pictures are the same, which means no picture.
	if steam := gameNamed(t, items, "Steam Big Picture"); steam.Poster != "" {
		t.Errorf("default picture kept: %q", steam.Poster)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/poster", game.ID), admin, nil); w.Code != 200 {
		t.Errorf("poster: %d", w.Code)
	}

	// Status, as the PC has told the server its MAC now.
	w := ts.do(t, "GET", "/api/rigs", admin, nil)
	list := rigJSON[[]rigView](t, w.Code, w.Body.Bytes())
	if len(list) != 1 || list[0].State != "online" || !list[0].Paired || !list[0].HasMAC || list[0].Fingerprint == "" {
		t.Fatalf("list: %+v", list)
	}

	// Scanning does nothing to it, and the library can't be turned into something else.
	if w := ts.do(t, "POST", fmt.Sprintf("/api/libraries/%d/scan", v.LibraryID), admin, nil); w.Code != 202 {
		t.Errorf("scan: %d", w.Code)
	}
	time.Sleep(100 * time.Millisecond)
	if n := len(rigGames(t, ts, admin, v.LibraryID)); n != 2 {
		t.Errorf("games after a scan: %d", n)
	}
	if w := ts.do(t, "PUT", fmt.Sprintf("/api/libraries/%d", v.LibraryID), admin, map[string]any{"name": "x", "type": "movies", "paths": []string{"/"}}); w.Code != 409 {
		t.Errorf("edit: %d", w.Code)
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/games/%d/play", game.ID), admin, map[string]any{}); w.Code != 409 {
		t.Errorf("the emulator route: %d", w.Code)
	}

	// A plan for each kind of device, with what the Moonlight app needs.
	w = ts.do(t, "POST", fmt.Sprintf("/api/rigs/games/%d/play", game.ID), admin, map[string]any{"client": "desktop", "os": "windows"})
	if w.Code != 200 {
		t.Fatalf("play: %d %s", w.Code, w.Body)
	}
	plan := rigJSON[rigPlayView](t, w.Code, w.Body.Bytes())
	if !plan.Ready || len(plan.Methods) < 2 || plan.Methods[0].Kind != "command" || plan.Methods[0].Args[2] != "Space Game" || plan.Install == nil {
		t.Errorf("plan: %+v", plan)
	}
	w = ts.do(t, "POST", fmt.Sprintf("/api/rigs/games/%d/play", game.ID), admin, map[string]any{"client": "android", "os": "androidtv"})
	plan = rigJSON[rigPlayView](t, w.Code, w.Body.Bytes())
	if plan.Methods[0].Kind != "intent" || plan.Methods[0].Extras["UUID"] != f.UUID || plan.Methods[0].Extras["AppId"] != "7" {
		t.Errorf("android plan: %+v", plan.Methods[0])
	}

	// Starting, seeing it running, refused a second time, and ended.
	if w := ts.do(t, "POST", fmt.Sprintf("/api/rigs/games/%d/start", game.ID), admin, nil); w.Code != 200 {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	if f.Running() != 7 {
		t.Errorf("running %d", f.Running())
	}
	w = ts.do(t, "GET", "/api/rigs", admin, nil)
	list = rigJSON[[]rigView](t, w.Code, w.Body.Bytes())
	if list[0].State != "busy" || list[0].CurrentGame != "Space Game" || !list[0].CanQuit {
		t.Errorf("while running: %+v", list[0])
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/rigs/games/%d/start", gameNamed(t, items, "Steam Big Picture").ID), admin, nil); w.Code != 409 {
		t.Errorf("second game: %d", w.Code)
	}
	w = ts.do(t, "POST", fmt.Sprintf("/api/rigs/games/%d/play", gameNamed(t, items, "Steam Big Picture").ID), admin, map[string]any{"client": "browser"})
	plan = rigJSON[rigPlayView](t, w.Code, w.Body.Bytes())
	if plan.Ready || !strings.Contains(plan.Message, "Space Game") {
		t.Errorf("plan while another runs: %+v", plan)
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/rigs/%d/quit", v.ID), admin, nil); w.Code != 200 {
		t.Errorf("quit: %d %s", w.Code, w.Body)
	}
	if f.Running() != 0 {
		t.Error("still running")
	}

	// Waking only goes to a registered rig.
	if w := ts.do(t, "POST", fmt.Sprintf("/api/rigs/%d/wake", v.ID), admin, nil); w.Code != 200 {
		t.Errorf("wake: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/rigs/99/wake", admin, nil); w.Code != 404 {
		t.Errorf("wake nothing: %d", w.Code)
	}

	// Showing and hiding.
	if w := ts.do(t, "PUT", fmt.Sprintf("/api/rigs/%d/apps/%d", v.ID, f.Apps[0].ID), admin, map[string]any{"shown": true}); w.Code != 200 {
		t.Errorf("show: %d %s", w.Code, w.Body)
	}
	if n := len(rigGames(t, ts, admin, v.LibraryID)); n != 3 {
		t.Errorf("after showing Desktop: %d", n)
	}
	// A game gone from the PC goes from the library; a PC that doesn't answer changes nothing.
	f.Apps = f.Apps[:2]
	if w := ts.do(t, "POST", fmt.Sprintf("/api/rigs/%d/sync", v.ID), admin, nil); w.Code != 200 {
		t.Errorf("sync: %d %s", w.Code, w.Body)
	}
	if n := len(rigGames(t, ts, admin, v.LibraryID)); n != 2 {
		t.Errorf("after a game left: %d", n)
	}
	f.Close()
	if w := ts.do(t, "POST", fmt.Sprintf("/api/rigs/%d/sync", v.ID), admin, nil); w.Code != 502 {
		t.Errorf("sync with it off: %d", w.Code)
	}
	if n := len(rigGames(t, ts, admin, v.LibraryID)); n != 2 {
		t.Errorf("a sleeping PC lost its games: %d", n)
	}
	ts.forgetRigStatus(v.ID)
	w = ts.do(t, "GET", "/api/rigs", admin, nil)
	if st := rigJSON[[]rigView](t, w.Code, w.Body.Bytes())[0].State; st != "no-sunshine" {
		t.Errorf("state %q", st)
	}

	// Removing it takes the library and games with it.
	w = ts.do(t, "DELETE", fmt.Sprintf("/api/rigs/%d", v.ID), admin, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "paired devices") {
		t.Errorf("remove: %d %s", w.Code, w.Body)
	}
	if _, err := ts.Store.Library(context.Background(), v.LibraryID); err == nil {
		t.Error("its library is still there")
	}
	if rows, _ := ts.rigRows(context.Background()); len(rows) != 0 {
		t.Errorf("rows: %d", len(rows))
	}
}

func TestRigPermissions(t *testing.T) {
	ts := newTestServer(t)
	f := rigFake(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	v := pairedRig(t, ts, f, admin)
	game := gameNamed(t, rigGames(t, ts, admin, v.LibraryID), "Space Game")
	base := fmt.Sprintf("/api/rigs/%d", v.ID)

	_, adult := ts.user(t, "adult1", "adult", false, "")
	_, kid := ts.user(t, "kid1", "kids", false, "")
	_, guest := ts.user(t, "guest1", "guest", false, "")
	// An adult limited to other libraries.
	limited, _ := json.Marshal(library.Access{MaxAge: -1, AllowUnrated: true, Libraries: []int64{v.LibraryID + 1000}})
	_, other := ts.user(t, "adult2", "adult", false, string(limited))
	// A kid who has been given this one game.
	allowed, _ := json.Marshal(library.Access{MaxAge: 7, AllowItems: []int64{game.ID}})
	_, kidOK := ts.user(t, "kid2", "kids", false, string(allowed))

	// Anything that manages a rig or names an address is refused for everyone but admins.
	for _, tok := range []string{adult, kid, guest, other, kidOK, ""} {
		for _, c := range [][2]string{
			{"POST", "/api/rigs"}, {"GET", "/api/rigs/find"}, {"PATCH", base}, {"DELETE", base}, {"POST", base + "/pair"}, {"GET", base + "/pair"}, {"DELETE", base + "/pair"},
			{"POST", base + "/devices"}, {"POST", base + "/sync"}, {"GET", base + "/apps"}, {"PUT", base + fmt.Sprintf("/apps/%d", f.Apps[0].ID)},
		} {
			w := ts.do(t, c[0], c[1], tok, map[string]any{"address": "169.254.169.254", "shown": true})
			if w.Code != 401 && w.Code != 403 {
				t.Errorf("%q %s %s: %d %s", tok[:min(len(tok), 4)], c[0], c[1], w.Code, w.Body)
			}
		}
	}

	seesGames := func(tok string) int { return len(rigGames(t, ts, tok, v.LibraryID)) }
	if seesGames(adult) != 2 {
		t.Errorf("adult sees %d", seesGames(adult))
	}
	// Kids and guests see nothing until given it: a PC's games have no ratings.
	if seesGames(kid) != 0 || seesGames(guest) != 0 {
		t.Errorf("kid %d guest %d", seesGames(kid), seesGames(guest))
	}
	if seesGames(kidOK) != 1 {
		t.Errorf("kid with one game sees %d", seesGames(kidOK))
	}

	// Those without access can't learn it exists or reach it.
	for name, tok := range map[string]string{"kid": kid, "guest": guest, "limited adult": other} {
		w := ts.do(t, "GET", "/api/rigs", tok, nil)
		if got := rigJSON[[]rigView](t, w.Code, w.Body.Bytes()); len(got) != 0 {
			t.Errorf("%s sees %+v", name, got)
		}
		if w := ts.do(t, "POST", fmt.Sprintf("/api/rigs/games/%d/play", game.ID), tok, map[string]any{}); w.Code != 404 {
			t.Errorf("%s play: %d", name, w.Code)
		}
		if w := ts.do(t, "POST", fmt.Sprintf("/api/rigs/games/%d/start", game.ID), tok, nil); w.Code != 404 {
			t.Errorf("%s start: %d", name, w.Code)
		}
		if w := ts.do(t, "POST", base+"/wake", tok, nil); w.Code != 404 {
			t.Errorf("%s wake: %d", name, w.Code)
		}
		if w := ts.do(t, "POST", base+"/quit", tok, nil); w.Code != 404 {
			t.Errorf("%s quit: %d", name, w.Code)
		}
	}
	if launches, _ := f.Counts(); launches != 0 {
		t.Errorf("%d launches got through", launches)
	}

	// A viewer's list says nothing about where the PC is or how it's identified.
	w := ts.do(t, "GET", "/api/rigs", adult, nil)
	body := w.Body.String()
	for _, secret := range []string{"127.0.0.1", f.MAC, "fingerprint", "webUrl", "BEGIN", "address"} {
		if strings.Contains(body, secret) {
			t.Errorf("a viewer's list has %q: %s", secret, body)
		}
	}
	if got := rigJSON[[]rigView](t, w.Code, w.Body.Bytes()); len(got) != 1 {
		t.Errorf("adult sees %d rigs", len(got))
	}

	// A viewer's game hides its made-up path too.
	if it := gameNamed(t, rigGames(t, ts, adult, v.LibraryID), "Space Game"); it.Path != "" {
		t.Errorf("path %q", it.Path)
	}

	// What's unlocked works: the kid can start their game, and only that.
	if w := ts.do(t, "POST", fmt.Sprintf("/api/rigs/games/%d/start", game.ID), kidOK, nil); w.Code != 200 {
		t.Fatalf("kid start: %d %s", w.Code, w.Body)
	}
	steam := gameNamed(t, rigGames(t, ts, admin, v.LibraryID), "Steam Big Picture")
	if w := ts.do(t, "POST", fmt.Sprintf("/api/rigs/games/%d/start", steam.ID), kidOK, nil); w.Code != 404 {
		t.Errorf("kid starting another: %d", w.Code)
	}
	// Someone else can't end what they started, or see that they may.
	if w := ts.do(t, "POST", base+"/quit", adult, nil); w.Code != 403 {
		t.Errorf("another viewer quitting: %d", w.Code)
	}
	if w := ts.do(t, "POST", base+"/quit", kidOK, nil); w.Code != 200 {
		t.Errorf("own quit: %d %s", w.Code, w.Body)
	}
}

func TestRigAddressesAreChecked(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	for _, addr := range []string{"http://192.168.1.2", "192.168.1.2/admin", "user@pc", "169.254.169.254", "224.0.0.1", "0.0.0.0", "fe80::1", "pc?x=y", ""} {
		if w := ts.do(t, "POST", "/api/rigs", admin, map[string]any{"address": addr}); w.Code != 400 {
			t.Errorf("%q: %d %s", addr, w.Code, w.Body)
		}
	}
	// The internet needs saying twice, and is then still only a question of a real answer.
	w := ts.do(t, "POST", "/api/rigs", admin, map[string]any{"address": "8.8.8.8"})
	if w.Code != 409 || !strings.Contains(w.Body.String(), `"public":true`) {
		t.Errorf("public: %d %s", w.Code, w.Body)
	}
	if rows, _ := ts.rigRows(context.Background()); len(rows) != 0 {
		t.Errorf("something was stored: %d", len(rows))
	}
	// Something that isn't Sunshine isn't added.
	f := rigFake(t)
	f.Garbage = true
	w = ts.do(t, "POST", "/api/rigs", admin, map[string]any{"address": fmt.Sprintf("127.0.0.1:%d", f.Port())})
	if w.Code != 502 || !strings.Contains(w.Body.String(), "isn't Sunshine") {
		t.Errorf("a printer: %d %s", w.Code, w.Body)
	}
	f.Garbage = false
	// The same PC twice.
	if w := ts.do(t, "POST", "/api/rigs", admin, map[string]any{"address": fmt.Sprintf("127.0.0.1:%d", f.Port())}); w.Code != 201 {
		t.Fatalf("add: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/rigs", admin, map[string]any{"address": fmt.Sprintf("localhost:%d", f.Port())}); w.Code != 409 {
		t.Errorf("twice: %d %s", w.Code, w.Body)
	}
	rows, _ := ts.rigRows(context.Background())
	id := rows[0].ID
	if w := ts.do(t, "PATCH", fmt.Sprintf("/api/rigs/%d", id), admin, map[string]any{"mac": "ff:ff:ff:ff:ff:ff"}); w.Code != 400 {
		t.Errorf("group MAC: %d", w.Code)
	}
	if w := ts.do(t, "PATCH", fmt.Sprintf("/api/rigs/%d", id), admin, map[string]any{"address": "169.254.169.254"}); w.Code != 400 {
		t.Errorf("metadata address: %d", w.Code)
	}
	if w := ts.do(t, "PATCH", fmt.Sprintf("/api/rigs/%d", id), admin, map[string]any{"remoteAddress": "100.100.1.2"}); w.Code != 200 {
		t.Errorf("VPN address: %d %s", w.Code, w.Body)
	}
}

func TestRigSecretsStayHome(t *testing.T) {
	ts := newTestServer(t)
	f := rigFake(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	v := pairedRig(t, ts, f, admin)
	id, err := ts.rigs().ident()
	if err != nil {
		t.Fatal(err)
	}
	// No answer, even to an admin, has this server's key or certificate or the rig's.
	for _, p := range []string{"/api/rigs", fmt.Sprintf("/api/rigs/%d/pair", v.ID), fmt.Sprintf("/api/rigs/%d/apps", v.ID), "/api/rigs/browser", "/api/settings"} {
		w := ts.do(t, "GET", p, admin, nil)
		if strings.Contains(w.Body.String(), "BEGIN") || strings.Contains(w.Body.String(), id.ID) {
			t.Errorf("%s has a secret: %s", p, w.Body)
		}
	}
	// A finished pairing no longer shows its PIN.
	w := ts.do(t, "GET", fmt.Sprintf("/api/rigs/%d/pair", v.ID), admin, nil)
	if st := rigJSON[rigPairing](t, w.Code, w.Body.Bytes()); st.PIN != "" {
		t.Errorf("PIN still showing: %+v", st)
	}
	// The key isn't in the database.
	var n int
	if err := ts.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM wittyrigs_rigs WHERE cert LIKE '%PRIVATE%'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("%d %v", n, err)
	}
	// A rig answering with another certificate isn't trusted, and viewers get no detail.
	other := rigFake(t)
	if _, err := ts.DB.ExecContext(context.Background(), `UPDATE wittyrigs_rigs SET port = ? WHERE id = ?`, other.Port(), v.ID); err != nil {
		t.Fatal(err)
	}
	ts.forgetRigStatus(v.ID)
	_, adult := ts.user(t, "adult1", "adult", false, "")
	w = ts.do(t, "GET", "/api/rigs", admin, nil)
	if got := rigJSON[[]rigView](t, w.Code, w.Body.Bytes())[0]; got.State != "cert-changed" {
		t.Errorf("admin sees %+v", got)
	}
	w = ts.do(t, "GET", "/api/rigs", adult, nil)
	if got := rigJSON[[]rigView](t, w.Code, w.Body.Bytes())[0]; got.State != "cert-changed" || strings.Contains(got.Message, "certificate") {
		t.Errorf("viewer sees %+v", got)
	}
	if launches, _ := other.Counts(); launches != 0 {
		t.Error("a different PC was used")
	}
}

func TestRigDevicePINIsNeverStored(t *testing.T) {
	ts := newTestServer(t)
	f := rigFake(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	v := pairedRig(t, ts, f, admin)
	f.PendingDevice("Living Room TV")
	url := fmt.Sprintf("/api/rigs/%d/devices", v.ID)
	if w := ts.do(t, "POST", url, admin, map[string]any{"pin": "4321", "username": "admin", "password": "wrong"}); w.Code != 400 || !strings.Contains(w.Body.String(), "user name") {
		t.Errorf("wrong sign-in: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", url, admin, map[string]any{"pin": "4321", "username": "admin", "password": "secret"}); w.Code != 200 {
		t.Errorf("good: %d %s", w.Code, w.Body)
	}
	// Nowhere in the database.
	for _, table := range []string{"wittyrigs_rigs", "settings"} {
		rows, err := ts.DB.QueryContext(context.Background(), "SELECT * FROM "+table)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		vals := make([]any, len(cols))
		for i := range vals {
			vals[i] = new(any)
		}
		for rows.Next() {
			_ = rows.Scan(vals...)
			for _, c := range vals {
				if s, ok := (*c.(*any)).(string); ok && strings.Contains(s, "secret") {
					t.Errorf("%s holds the password", table)
				}
			}
		}
		rows.Close()
	}
}

func TestRigPairingRefusals(t *testing.T) {
	ts := newTestServer(t)
	f := rigFake(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	w := ts.do(t, "POST", "/api/rigs", admin, map[string]any{"address": fmt.Sprintf("127.0.0.1:%d", f.Port())})
	v := rigJSON[rigView](t, w.Code, w.Body.Bytes())
	if v.State != "not-paired" {
		t.Errorf("state %q", v.State)
	}
	url := fmt.Sprintf("/api/rigs/%d/pair", v.ID)
	waitFor := func(state string) rigPairing {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			w := ts.do(t, "GET", url, admin, nil)
			st := rigJSON[rigPairing](t, w.Code, w.Body.Bytes())
			if st.State == state {
				return st
			}
			if time.Now().After(deadline) {
				t.Fatalf("pairing stuck at %+v", st)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	// A wrong PIN.
	w = ts.do(t, "POST", url, admin, nil)
	first := rigJSON[rigPairing](t, w.Code, w.Body.Bytes())
	// While it waits, asking again gives the same PIN rather than a second pairing.
	again := rigJSON[rigPairing](t, 200, ts.do(t, "POST", url, admin, nil).Body.Bytes())
	if again.PIN != first.PIN {
		t.Errorf("a second PIN: %s, %s", first.PIN, again.PIN)
	}
	f.WaitForPairing(5 * time.Second)
	wrong := "0000"
	if first.PIN == wrong {
		wrong = "1111"
	}
	f.TypePIN(wrong)
	if st := waitFor("failed"); !strings.Contains(st.Message, "PIN") {
		t.Errorf("wrong PIN: %+v", st)
	}
	if rows, _ := ts.rigRows(context.Background()); rows[0].Cert != "" {
		t.Error("a certificate was saved")
	}
	// Someone in the middle.
	f.WrongCertInPairing = true
	w = ts.do(t, "POST", url, admin, nil)
	p := rigJSON[rigPairing](t, w.Code, w.Body.Bytes())
	f.WaitForPairing(5 * time.Second)
	f.TypePIN(p.PIN)
	if st := waitFor("failed"); !strings.Contains(st.Message, "pose") {
		t.Errorf("imposter: %+v", st)
	}
	if rows, _ := ts.rigRows(context.Background()); rows[0].Cert != "" {
		t.Error("an imposter's certificate was saved")
	}
	f.WrongCertInPairing = false
	// Cancelling.
	w = ts.do(t, "POST", url, admin, nil)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	f.WaitForPairing(5 * time.Second)
	ts.do(t, "DELETE", url, admin, nil)
	if st := waitFor("failed"); !strings.Contains(st.Message, "cancelled") {
		t.Errorf("cancel: %+v", st)
	}
}

func TestRigAddressDependsOnWhereYouAre(t *testing.T) {
	ts := newTestServer(t)
	ts.Cfg.RemoteAccess = true
	ts.h = ts.Handler()
	f := rigFake(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	v := pairedRig(t, ts, f, admin)
	game := gameNamed(t, rigGames(t, ts, admin, v.LibraryID), "Space Game")
	url := fmt.Sprintf("/api/rigs/games/%d/play", game.ID)
	body := map[string]any{"client": "desktop", "os": "windows"}
	plan := func(hdr ...string) rigPlayView {
		t.Helper()
		w := ts.do(t, "POST", url, admin, body, hdr...)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		return rigJSON[rigPlayView](t, w.Code, w.Body.Bytes())
	}
	// A rig that's this very machine means nothing to another device on the network, which
	// gets the name it reached this server by.
	lan := plan("RemoteAddr", "192.168.1.50:5000", "Host", "192.168.1.10:9797")
	if lan.Away || lan.Methods[0].Kind != "command" || !strings.HasPrefix(lan.Methods[0].Args[1], "192.168.1.10") {
		t.Errorf("at home: %+v", lan)
	}
	// Away with no private address: there's nothing Moonlight could connect to, and it says so.
	away := plan("RemoteAddr", "100.100.1.5:5000", "Host", "pc.example.ts.net")
	if !away.Away || len(away.Methods) != 0 || !strings.Contains(away.Message, "away from home") {
		t.Errorf("away: %+v", away)
	}
	if w := ts.do(t, "PATCH", fmt.Sprintf("/api/rigs/%d", v.ID), admin, map[string]any{"remoteAddress": "100.100.1.2"}); w.Code != 200 {
		t.Fatal(w.Body)
	}
	away = plan("RemoteAddr", "100.100.1.5:5000", "Host", "pc.example.ts.net")
	if !away.Away || len(away.Methods) == 0 || !strings.HasPrefix(away.Methods[0].Args[1], "100.100.1.2") {
		t.Errorf("away with a private address: %+v", away)
	}
}

func TestRigArtStaysWhenThePCCantSay(t *testing.T) {
	ts := newTestServer(t)
	f := rigFake(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	v := pairedRig(t, ts, f, admin)
	sync := func() {
		t.Helper()
		if w := ts.do(t, "POST", fmt.Sprintf("/api/rigs/%d/sync", v.ID), admin, nil); w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	poster := func() string { return gameNamed(t, rigGames(t, ts, admin, v.LibraryID), "Space Game").Poster }
	before := poster()
	if !strings.HasPrefix(before, library.RigArtPrefix) {
		t.Fatalf("poster %q", before)
	}
	f.NotImage = true
	sync()
	if poster() != before {
		t.Errorf("a bad answer cost the game its picture: %q", poster())
	}
	f.NotImage = false
	// A picture chosen by hand stays.
	it := gameNamed(t, rigGames(t, ts, admin, v.LibraryID), "Space Game")
	full, err := ts.Store.Item(context.Background(), it.ID)
	if err != nil {
		t.Fatal(err)
	}
	full.Poster = "cache:mine.png"
	if err := ts.Store.UpdateItem(context.Background(), full); err != nil {
		t.Fatal(err)
	}
	sync()
	if poster() != "cache:mine.png" {
		t.Errorf("a chosen picture was replaced: %q", poster())
	}
}

func TestRigLibraryDeletedInLibrarySettings(t *testing.T) {
	ts := newTestServer(t)
	f := rigFake(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	v := pairedRig(t, ts, f, admin)
	if w := ts.do(t, "DELETE", fmt.Sprintf("/api/libraries/%d", v.LibraryID), admin, nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if rows, _ := ts.rigRows(context.Background()); len(rows) != 0 {
		t.Errorf("%d rigs left", len(rows))
	}
	var n int
	if err := ts.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM wittyrigs_apps`).Scan(&n); err != nil || n != 0 {
		t.Errorf("%d apps left, %v", n, err)
	}
}

var _ = gamestream.DefaultPort
var _ = http.StatusOK
