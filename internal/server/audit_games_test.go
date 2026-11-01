package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittycouch/gamestream/fake"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittyplay/testcore/fakecore"
)

// A PC chooses its apps' numbers, so pictures of ones it no longer lists have to go.
func TestCouchArtDoesntPileUp(t *testing.T) {
	testAllowLoopback(t)
	ts := newTestServer(t)
	_ = ts.DB.SetSetting(context.Background(), "couchCustomPorts", "1")
	_, adult := ts.user(t, "adult1", "adult", false, "")
	f := rigFake(t)
	h := addCouch(t, ts, f, adult)
	pairCouch(t, ts, f, adult, h.ID)
	list := func() {
		t.Helper()
		if w := ts.do(t, "GET", fmt.Sprintf("/api/couch/hosts/%d/apps?fresh=1", h.ID), adult, nil); w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	png := []byte("\x89PNG\r\n\x1a\nfirst")
	f.Apps = []fake.App{{ID: 41, Title: "Old", Image: png}}
	list()
	g := ts.couch()
	u := userID(t, ts, adult)
	if _, err := os.Stat(g.artPath(u, h.ID, 41)); err != nil {
		t.Fatal(err)
	}
	f.Apps = []fake.App{{ID: 42, Title: "New", Image: png}}
	list()
	if _, err := os.Stat(g.artPath(u, h.ID, 41)); err == nil {
		t.Error("the old app's picture is still there")
	}
	if _, err := os.Stat(g.artPath(u, h.ID, 42)); err != nil {
		t.Error(err)
	}
	// A fresh list counts as an attempt to connect.
	limited := false
	for range couchAttemptsLimit + 1 {
		if ts.do(t, "GET", fmt.Sprintf("/api/couch/hosts/%d/apps?fresh=1", h.ID), adult, nil).Code == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("asking for a fresh list is never limited")
	}
}

func userID(t *testing.T, ts *testServer, token string) int64 {
	t.Helper()
	type me struct {
		ID int64 `json:"id"`
	}
	v := rigJSON[me](t, 200, ts.do(t, "GET", "/api/me", token, nil).Body.Bytes())
	if v.ID == 0 {
		t.Fatal("no id")
	}
	return v.ID
}

// The next account can be given the same number, so nothing of the last one's may stay.
func TestADeletedAccountLeavesNothingBehind(t *testing.T) {
	testAllowLoopback(t)
	ts := newTestServer(t)
	_ = ts.DB.SetSetting(context.Background(), "couchCustomPorts", "1")
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "adult1", "adult", false, "")
	u := userID(t, ts, adult)
	f := rigFake(t)
	h := addCouch(t, ts, f, adult)
	pairCouch(t, ts, f, adult, h.ID)
	g := ts.games()
	dir := "u" + fmt.Sprint(u)
	files := []string{filepath.Join(g.saveDir(u, 5), "slot1.state"), filepath.Join(g.dir, "bezels", dir, "b1.png"), filepath.Join(g.patchDir(u, 5), "1.patch"),
		ts.couch().artPath(u, h.ID, 7)}
	for _, p := range files {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	identities := filepath.Join(ts.couch().dir, "identities", dir)
	if _, err := os.Stat(identities); err != nil {
		t.Fatalf("no identity to remove: %v", err)
	}
	r := ts.rigs()
	r.mu.Lock()
	r.starter[1] = rigStarter{user: u, app: 7}
	r.mu.Unlock()

	if w := ts.do(t, "DELETE", fmt.Sprintf("/api/users/%d", u), admin, nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, p := range append(files, identities) {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("still there: %s", p)
		}
	}
	r.mu.Lock()
	_, kept := r.starter[1]
	r.mu.Unlock()
	if kept {
		t.Error("the PC still says they started what's on it")
	}
	if ts.couch().identities[u] != nil {
		t.Error("the identity is still in memory")
	}
}

// What someone started stops being theirs when it ends.
func TestRigStarterEndsWithTheGame(t *testing.T) {
	ts := newTestServer(t)
	f := rigFake(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "adult1", "adult", false, "")
	v := pairedRig(t, ts, f, admin)
	r := ts.rigs()
	r.mu.Lock()
	r.starter[v.ID] = rigStarter{user: userID(t, ts, adult), app: 7}
	r.mu.Unlock()
	// Nothing is running on the PC any more.
	if w := ts.do(t, "POST", fmt.Sprintf("/api/rigs/%d/quit", v.ID), adult, nil); w.Code != 403 {
		t.Errorf("quit: %d %s", w.Code, w.Body)
	}
	r.mu.Lock()
	_, kept := r.starter[v.ID]
	r.mu.Unlock()
	if kept {
		t.Error("it's still theirs")
	}
	if !(rigStarter{user: 5, app: 7}).owns(5, 7) {
		t.Error("the game they started")
	}
	for _, c := range [][2]int64{{5, 8}, {6, 7}, {0, 0}} {
		if (rigStarter{user: 5, app: 7}).owns(c[0], int(c[1])) && c[0] != 5 {
			t.Errorf("%v", c)
		}
	}
	if (rigStarter{user: 5, app: 7}).owns(5, 8) {
		t.Error("another game on the same PC")
	}
	if (rigStarter{}).owns(0, 0) {
		t.Error("no one started it")
	}
}

// The limit holds for streams on their way too, which take up to a minute.
func TestCouchLimitCountsStreamsStarting(t *testing.T) {
	testAllowLoopback(t)
	ts := newTestServer(t)
	_ = ts.DB.SetSetting(context.Background(), "couchCustomPorts", "1")
	_ = ts.DB.SetSetting(context.Background(), "couchMaxStreams", "1")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WMS_WITTYCOUCH_HOST", self)
	_, adult := ts.user(t, "adult1", "adult", false, "")
	f := rigFake(t)
	h := addCouch(t, ts, f, adult)
	pairCouch(t, ts, f, adult, h.ID)
	play := fmt.Sprintf("/api/couch/hosts/%d/apps/7/play", h.ID)
	g := ts.couch()
	u := userID(t, ts, adult)

	g.mu.Lock()
	g.starting = map[int64]bool{u: true}
	g.mu.Unlock()
	if w := ts.do(t, "POST", play, adult, map[string]any{}); w.Code != 409 || !strings.Contains(w.Body.String(), "already starting") {
		t.Errorf("twice at once: %d %s", w.Code, w.Body)
	}
	g.mu.Lock()
	g.starting = map[int64]bool{u + 100: true}
	g.mu.Unlock()
	if w := ts.do(t, "POST", play, adult, map[string]any{}); w.Code != 503 {
		t.Errorf("someone else's on its way, and the limit is one: %d %s", w.Code, w.Body)
	}
}

func TestCouchPlayCountsAsAnAttempt(t *testing.T) {
	testAllowLoopback(t)
	ts := newTestServer(t)
	_ = ts.DB.SetSetting(context.Background(), "couchCustomPorts", "1")
	_, adult := ts.user(t, "adult1", "adult", false, "")
	f := rigFake(t)
	h := addCouch(t, ts, f, adult)
	pairCouch(t, ts, f, adult, h.ID)
	limited := false
	for range couchAttemptsLimit + 1 {
		if ts.do(t, "POST", fmt.Sprintf("/api/couch/hosts/%d/apps/999999/play", h.ID), adult, map[string]any{}).Code == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("playing is never limited")
	}
}

func TestRadioRelayHasALimit(t *testing.T) {
	var l radioLive
	for i := 0; i < radioStreamsPerUser; i++ {
		if !l.takeStream(1) {
			t.Fatal("refused too soon")
		}
	}
	if l.takeStream(1) {
		t.Error("one person went over")
	}
	if !l.takeStream(2) {
		t.Error("that stopped someone else")
	}
	l.dropStream(1)
	if !l.takeStream(1) {
		t.Error("a place didn't come back")
	}
	for i := 2; i < radioStreamsAtOnce; i++ {
		l.takeStream(int64(i * 10))
		l.takeStream(int64(i * 10))
	}
	if l.takeStream(9999) {
		t.Error("the server went over")
	}
}

func TestRadioRelayAnswersWhenFull(t *testing.T) {
	ts, lib, st := radioFixture(t, func(http.ResponseWriter, *http.Request) {})
	it := addStation(t, ts, lib, "Live", st.URL+"/a")
	u := userFromToken(t, ts, ts.adult)
	for range radioStreamsPerUser {
		ts.s.radio.takeStream(u)
	}
	w := ts.do("GET", fmt.Sprintf("/api/radio/%d/stream", it.ID), ts.adult, nil)
	if w.Code != 503 || w.Header().Get("Retry-After") == "" {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}

func userFromToken(t *testing.T, ts *galleryServer, token string) int64 {
	t.Helper()
	var v struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(ts.do("GET", "/api/me", token, nil).Body.Bytes(), &v); err != nil || v.ID == 0 {
		t.Fatalf("%v", err)
	}
	return v.ID
}

// Waiting behind another patched game gives up with whoever was waiting.
func TestPatchingOneAtATime(t *testing.T) {
	g := newGameServer(t)
	item := runnerGameItem(t, g)
	gs := g.s.games()
	u := int64(1)
	if _, err := g.s.Store.AddGamePatch(context.Background(), u, item, library.GamePatch{Name: "p", Kind: "ips"}, maxGamePatches); err != nil {
		t.Fatal(err)
	}
	patchSlot <- struct{}{}
	defer func() { <-patchSlot }()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	rom := filepath.Join(t.TempDir(), "game.nes")
	if err := os.WriteFile(rom, []byte("rom"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _, _ := gs.patchROM(ctx, g.s.Store, u, item, rom, nil)
	if got != rom {
		t.Errorf("%s", got)
	}
	if ctx.Err() == nil {
		t.Error("it went ahead without waiting for the other")
	}
}

// Asking a core what it offers loads it, so a changed file is refused here as for a game.
func TestCoreOptionsCheckACustomCore(t *testing.T) {
	buildCustomRunner(t)
	g := newGameServer(t)
	g.enableCustom(t)
	good := fakeCoreFile(t, "probeme", fakecore.Good)
	core := g.addCore(t, "probeme", good, []string{"snes"}, nil)
	if err := g.s.DB.SetSetting(context.Background(), "gamingCores", `{"snes":"probeme"}`); err != nil {
		t.Fatal(err)
	}
	// Approving it may have kept what it offers, which is safe to answer from; this is about asking again.
	if _, err := g.s.DB.ExecContext(context.Background(), `DELETE FROM wittyplay_core_options`); err != nil {
		t.Log(err)
	}
	store := g.s.games().custom.store
	if err := os.WriteFile(store.CorePath(core.ID, core.SHA256), append(append([]byte{}, good...), 1), 0o755); err != nil {
		t.Fatal(err)
	}
	id, _, custom := g.s.coreForConsole(context.Background(), "snes", 0)
	if id != "probeme" || custom == nil {
		t.Fatalf("%q %v", id, custom)
	}
	if w := g.do("POST", "/api/games/settings/validate", g.kid, map[string]any{"document": "{}", "console": "snes"}); w.Code >= 500 {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	if !auditHas(g.customAudit(t), "file-changed", core.ID) {
		t.Error("the changed file wasn't checked before asking it")
	}
}

func TestSilentCoresArentAskedAgain(t *testing.T) {
	g := newGameServer(t)
	gs := g.s.games()
	gs.silentCores.Store("test", time.Now())
	if at, ok := gs.silentCores.Load("test"); !ok || time.Since(at.(time.Time)) >= silentCoreFor {
		t.Error("not kept")
	}
	if cap(gs.probeSlot) != 1 {
		t.Errorf("%d at once", cap(gs.probeSlot))
	}
}

func TestModuleReportsAreLimited(t *testing.T) {
	ts := newTestServer(t)
	ts.Modules = installedBundle(t, `export default 1`, `a{}`)
	_, tok := ts.user(t, "reader", "adult", false, "")
	limited := false
	for range 500 {
		w := ts.do(t, "POST", "/api/modules/wittycomics/failed", tok, map[string]string{"version": "0.2.0", "reason": "x"})
		if w.Code == 429 {
			limited = w.Header().Get("Retry-After") != ""
			break
		}
	}
	if !limited {
		t.Error("reports are never limited")
	}
}
