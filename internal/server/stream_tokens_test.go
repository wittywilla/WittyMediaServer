package server

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// mintFor asks the API for a stream token.
func mintFor(t *testing.T, ts *testServer, session string, id int64) string {
	t.Helper()
	w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/stream-token", id), session, nil)
	if w.Code != 200 {
		t.Fatalf("mint: %d %s", w.Code, w.Body.String())
	}
	b := decode[map[string]any](t, w)
	if b["param"] != "st" || b["expiresInSec"].(float64) <= 0 {
		t.Errorf("mint answer: %v", b)
	}
	return b["token"].(string)
}

func tempMedia(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStreamTokenOpensOnlyItsOwnItem(t *testing.T) {
	ts := newTestServer(t)
	_, session := ts.account(t, "Alex", "adult", false)
	a := ts.item(t, "video", tempMedia(t, "a.mp4", "AAAA"))
	b := ts.item(t, "video", tempMedia(t, "b.mp4", "BBBB"))
	tok := mintFor(t, ts, session, a)
	if !strings.HasPrefix(tok, "st_") {
		t.Errorf("token %q", tok)
	}

	w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/file?st=%s", a, tok), "", nil)
	if w.Code != 200 || w.Body.String() != "AAAA" {
		t.Errorf("own file: %d %q", w.Code, w.Body.String())
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/file?st=%s", b, tok), "", nil); w.Code != 401 {
		t.Errorf("another item's file: %d", w.Code)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/file?st=nonsense", a), "", nil); w.Code != 401 {
		t.Errorf("made up token: %d", w.Code)
	}
	// Ranges work: native players seek with them.
	r := httptest.NewRequest("GET", fmt.Sprintf("/api/items/%d/file?st=%s", a, tok), nil)
	r.Header.Set("Range", "bytes=1-2")
	rec := httptest.NewRecorder()
	ts.h.ServeHTTP(rec, r)
	if rec.Code != 206 || rec.Body.String() != "AA" {
		t.Errorf("range: %d %q", rec.Code, rec.Body.String())
	}
}

// Whatever a stream token is used on, only the listed media routes answer. The
// routes are read back from the docs, so a new route is covered by default.
func TestStreamTokenRefusedEverywhereElse(t *testing.T) {
	ts := newTestServer(t)
	_, session := ts.account(t, "Boss", "adult", true) // an admin's ticket is no admin
	id := ts.item(t, "video", tempMedia(t, "a.mp4", "AAAA"))
	tok := mintFor(t, ts, session, id)
	for _, route := range documentedRoutes(t) {
		method, path := route.method, fillPath(route.path, id)
		if ts.streamRoutes(route.pattern()) != nil {
			continue
		}
		if route.public {
			continue
		}
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		w := ts.do(t, method, path+sep+"st="+tok, "", nil)
		if w.Code != 401 {
			t.Errorf("%s %s with a stream token: %d, want 401", method, path, w.Code)
		}
	}
	// Nor does it make a new ticket, whatever route it reaches.
	w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/stream-token?st=%s", id, tok), "", nil)
	if w.Code != 401 {
		t.Errorf("a ticket buying a ticket: %d", w.Code)
	}
}

func TestStreamTokenFollowsTheSignIn(t *testing.T) {
	ts := newTestServer(t)
	_, session := ts.account(t, "Alex", "adult", false)
	id := ts.item(t, "video", tempMedia(t, "a.mp4", "AAAA"))
	tok := mintFor(t, ts, session, id)
	url := fmt.Sprintf("/api/items/%d/file?st=%s", id, tok)
	if w := ts.do(t, "GET", url, "", nil); w.Code != 200 {
		t.Fatalf("before sign-out: %d", w.Code)
	}
	ts.do(t, "POST", "/api/logout", session, nil)
	if w := ts.do(t, "GET", url, "", nil); w.Code != 401 {
		t.Errorf("after sign-out: %d", w.Code)
	}
}

func TestStreamTokenFollowsAccessRules(t *testing.T) {
	ts := newTestServer(t)
	u, session := ts.account(t, "Alex", "adult", false)
	path := tempMedia(t, "a.mp4", "AAAA")
	id := ts.item(t, "video", path)
	tok := mintFor(t, ts, session, id)
	sid := "v-" + hls.Key(path) + "-test"
	ts.HLS.Ensure(hls.Spec{ID: sid, Duration: 8, AudioIndex: -1, SubtitleIndex: -1})
	playlist := "/api/hls/" + sid + "/playlist.m3u8?st=" + tok
	if w := ts.do(t, "GET", playlist, "", nil); w.Code != 200 {
		t.Fatalf("before blocking: %d", w.Code)
	}
	// Taking the title away from the account closes the ticket's door too,
	// on routes that never look at the item themselves as well.
	if err := ts.Auth.SetRestrictions(context.Background(), u.ID, fmt.Sprintf(`{"maxAge":-1,"blockItems":[%d]}`, id)); err != nil {
		t.Fatal(err)
	}
	for _, addr := range []string{fmt.Sprintf("/api/items/%d/file?st=%s", id, tok), playlist} {
		if w := ts.do(t, "GET", addr, "", nil); w.Code != 401 {
			t.Errorf("%s after blocking the title: %d", addr, w.Code)
		}
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/stream-token", id), session, nil); w.Code != 404 {
		t.Errorf("mint for a hidden title: %d", w.Code)
	}
}

func TestStreamTokenExpiry(t *testing.T) {
	ts := newTestServer(t)
	_, session := ts.account(t, "Alex", "adult", false)
	id := ts.item(t, "video", tempMedia(t, "a.mp4", "AAAA"))
	tok := mintFor(t, ts, session, id)
	url := fmt.Sprintf("/api/items/%d/file?st=%s", id, tok)

	// Use keeps a ticket alive...
	ts.stream.mu.Lock()
	ts.stream.m[tok].idle = time.Now().Add(time.Minute)
	ts.stream.mu.Unlock()
	ts.do(t, "GET", url, "", nil)
	ts.stream.mu.Lock()
	idle := ts.stream.m[tok].idle
	ts.stream.mu.Unlock()
	if time.Until(idle) < streamIdle-time.Minute {
		t.Errorf("use didn't extend it: %v", time.Until(idle))
	}
	// ...but not past its hard limit.
	ts.stream.mu.Lock()
	ts.stream.m[tok].hard = time.Now().Add(time.Minute)
	ts.stream.mu.Unlock()
	ts.do(t, "GET", url, "", nil)
	ts.stream.mu.Lock()
	idle = ts.stream.m[tok].idle
	ts.stream.mu.Unlock()
	if time.Until(idle) > time.Minute {
		t.Errorf("extended past the limit: %v", time.Until(idle))
	}
	// Idle too long, it's gone.
	ts.stream.mu.Lock()
	ts.stream.m[tok].idle = time.Now().Add(-time.Second)
	ts.stream.mu.Unlock()
	if w := ts.do(t, "GET", url, "", nil); w.Code != 401 {
		t.Errorf("expired: %d", w.Code)
	}
}

func TestStreamTokensDoNotPileUp(t *testing.T) {
	var st streamTokens
	it := &library.Item{ID: 1, Path: "/a.mp4"}
	a, _, _ := st.mint("session", 1, it)
	b, _, _ := st.mint("session", 1, it)
	if a != b {
		t.Error("asking twice should give the same ticket")
	}
	for i := int64(2); i < 200; i++ {
		if _, _, err := st.mint("session", 1, &library.Item{ID: i, Path: fmt.Sprint("/", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(st.m); n > streamMaxPerUser {
		t.Errorf("one account holds %d tickets", n)
	}
}

func TestStreamTokenInHLSAddresses(t *testing.T) {
	ts := newTestServer(t)
	_, session := ts.account(t, "Alex", "adult", false)
	path := tempMedia(t, "a.mkv", "x")
	id := ts.item(t, "video", path)
	other := ts.item(t, "video", tempMedia(t, "b.mkv", "x"))
	tok := mintFor(t, ts, session, id)
	sid := "v-" + hls.Key(path) + "-test"
	ts.HLS.Ensure(hls.Spec{ID: sid, Duration: 8, AudioIndex: -1, SubtitleIndex: -1})

	w := ts.do(t, "GET", "/api/hls/"+sid+"/playlist.m3u8?st="+tok, "", nil)
	if w.Code != 200 {
		t.Fatalf("playlist: %d %s", w.Code, w.Body.String())
	}
	// Every segment address carries it, since relative addresses lose the query.
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") && !strings.Contains(line, "st="+tok) {
			t.Errorf("segment without its ticket: %q", line)
		}
	}
	// With a header there is no token to pass on.
	if w := ts.do(t, "GET", "/api/hls/"+sid+"/playlist.m3u8", session, nil); w.Code != 200 || strings.Contains(w.Body.String(), "st=") {
		t.Errorf("plain playlist: %d %s", w.Code, w.Body.String())
	}
	// Someone else's item's ticket doesn't open this stream.
	wrong := mintFor(t, ts, session, other)
	if w := ts.do(t, "GET", "/api/hls/"+sid+"/playlist.m3u8?st="+wrong, "", nil); w.Code != 401 {
		t.Errorf("another item's ticket: %d", w.Code)
	}
	// It's read-only: stopping a stream needs a sign-in.
	if w := ts.do(t, "DELETE", "/api/hls/"+sid+"?st="+tok, "", nil); w.Code != 401 {
		t.Errorf("delete with a ticket: %d", w.Code)
	}
}

func TestLogsHideStreamTokens(t *testing.T) {
	got := redactQuery("start=10&st=st_secret")
	if strings.Contains(got, "st_secret") || !strings.Contains(got, "start=10") {
		t.Errorf("redactQuery = %q", got)
	}
	if got := streamTokenRe.ReplaceAllString(`GET /api/x?st=st_abc&f=1 failed`, "st=REDACTED"); strings.Contains(got, "st_abc") {
		t.Errorf("client log: %q", got)
	}
}

func TestAPIVersioning(t *testing.T) {
	ts := newTestServer(t)
	_, session := ts.account(t, "Alex", "adult", false)
	for _, p := range []string{"/api/me", "/api/v1/me"} {
		w := ts.do(t, "GET", p, session, nil)
		if w.Code != 200 || w.Header().Get("X-WMS-API-Version") != "1" {
			t.Errorf("%s: %d %v", p, w.Code, w.Header())
		}
	}
	if w := ts.do(t, "GET", "/api/v1/me", "", nil); w.Code != 401 {
		t.Errorf("v1 is no way around sign-in: %d", w.Code)
	}
	w := ts.do(t, "GET", "/api/v2/me", session, nil)
	if w.Code != 404 || decode[map[string]any](t, w)["apiVersions"] == nil {
		t.Errorf("unknown version: %d %s", w.Code, w.Body.String())
	}
	// The web app's own pages aren't touched by any of this.
	if w := ts.do(t, "GET", "/library/5", "", nil); w.Code != 200 || !strings.Contains(w.Body.String(), "app") {
		t.Errorf("page: %d", w.Code)
	}
	// Unknown API paths are errors, not the web page.
	if w := ts.do(t, "GET", "/api/v1/nothing", session, nil); w.Code != 404 || strings.Contains(w.Body.String(), "<html") {
		t.Errorf("unknown path: %d %s", w.Code, w.Body.String())
	}
}

func TestCapabilities(t *testing.T) {
	ts := newTestServer(t)
	for _, p := range []string{"/api/capabilities", "/api/v1/capabilities"} {
		w := ts.do(t, "GET", p, "", nil)
		if w.Code != 200 {
			t.Fatalf("%s: %d", p, w.Code)
		}
		c := decode[map[string]any](t, w)
		if c["apiVersion"].(float64) != 1 || c["setupRequired"] != true || c["version"] != "test" {
			t.Errorf("%v", c)
		}
		feats := fmt.Sprint(c["features"])
		for _, want := range []string{"pairing", "stream-tokens", "hls-remux", "playback-profiles"} {
			if !strings.Contains(feats, want) {
				t.Errorf("no feature %s in %s", want, feats)
			}
		}
	}
}
