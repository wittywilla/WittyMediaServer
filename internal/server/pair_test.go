package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type pairStarted struct {
	Code         string `json:"code"`
	DeviceSecret string `json:"deviceSecret"`
	ExpiresInSec int    `json:"expiresInSec"`
	IntervalSec  int    `json:"intervalSec"`
	ApproveURL   string `json:"approveUrl"`
}

func startPair(t *testing.T, ts *testServer) pairStarted {
	t.Helper()
	w := ts.do(t, "POST", "/api/pair/start", "", map[string]string{"device": "Living room TV", "platform": "tvOS"})
	if w.Code != 200 {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	return decode[pairStarted](t, w)
}

// pollPair polls, stepping past the too-fast check.
func pollPair(t *testing.T, ts *testServer, secret string) (int, map[string]any) {
	t.Helper()
	ts.pair.mu.Lock()
	if p := ts.pair.bySecret[hashSecret(secret)]; p != nil {
		p.lastPoll = time.Time{}
	}
	ts.pair.mu.Unlock()
	w := ts.do(t, "POST", "/api/pair/poll", "", map[string]string{"deviceSecret": secret})
	return w.Code, decode[map[string]any](t, w)
}

func TestPairingHappyPathIsSingleUse(t *testing.T) {
	ts := newTestServer(t)
	me, session := ts.account(t, "Alex", "adult", false)
	p := startPair(t, ts)
	if len(p.Code) != 9 || p.Code[4] != '-' || p.ExpiresInSec != 600 || p.IntervalSec == 0 || len(p.DeviceSecret) != 64 {
		t.Fatalf("start: %+v", p)
	}
	if !strings.Contains(p.ApproveURL, "/pair?code=") {
		t.Errorf("approve url: %q", p.ApproveURL)
	}
	if strings.ContainsAny(p.Code, "AEIOU01") {
		t.Errorf("code %q has letters that read as digits or spell words", p.Code)
	}
	if c, b := pollPair(t, ts, p.DeviceSecret); c != 200 || b["status"] != "pending" {
		t.Fatalf("before approval: %d %v", c, b)
	}

	// What's asking is shown first, and people type codes loosely.
	loose := strings.ToLower(strings.ReplaceAll(p.Code, "-", " "))
	w := ts.do(t, "POST", "/api/pair/info", session, map[string]string{"code": loose})
	if w.Code != 200 {
		t.Fatalf("info: %d %s", w.Code, w.Body.String())
	}
	info := decode[map[string]any](t, w)
	if info["device"] != "Living room TV" || info["platform"] != "tvOS" || info["ip"] != "192.168.1.50" {
		t.Errorf("info: %v", info)
	}

	if w := ts.do(t, "POST", "/api/pair/approve", session, map[string]string{"code": p.Code}); w.Code != 200 {
		t.Fatalf("approve: %d %s", w.Code, w.Body.String())
	}
	// A code works once.
	if w := ts.do(t, "POST", "/api/pair/approve", session, map[string]string{"code": p.Code}); w.Code != 404 {
		t.Errorf("second approve: %d", w.Code)
	}

	c, b := pollPair(t, ts, p.DeviceSecret)
	if c != 200 || b["status"] != "approved" {
		t.Fatalf("after approval: %d %v", c, b)
	}
	tok, _ := b["token"].(string)
	if len(tok) != 64 {
		t.Fatalf("token: %v", b)
	}
	// That token signs in as the approver.
	w = ts.do(t, "GET", "/api/me", tok, nil)
	if w.Code != 200 || decode[map[string]any](t, w)["name"] != me.Name {
		t.Errorf("me: %d %s", w.Code, w.Body.String())
	}
	// And the sign-in is collected only once.
	if c, b := pollPair(t, ts, p.DeviceSecret); c != 410 || b["status"] != "expired" || b["token"] != nil {
		t.Errorf("second collection: %d %v", c, b)
	}
}

func TestPairingNeedsASignedInAdult(t *testing.T) {
	ts := newTestServer(t)
	_, adult := ts.account(t, "Alex", "adult", false)
	_, kid := ts.account(t, "Sam", "kids", false)
	_, guest := ts.account(t, "Visitor", "guest", false)
	p := startPair(t, ts)
	if w := ts.do(t, "POST", "/api/pair/approve", "", map[string]string{"code": p.Code}); w.Code != 401 {
		t.Errorf("anonymous approve: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/pair/info", "", map[string]string{"code": p.Code}); w.Code != 401 {
		t.Errorf("anonymous info: %d", w.Code)
	}
	for name, tok := range map[string]string{"kids": kid, "guest": guest} {
		if w := ts.do(t, "POST", "/api/pair/approve", tok, map[string]string{"code": p.Code}); w.Code != 403 {
			t.Errorf("%s approve: %d", name, w.Code)
		}
	}
	if c, b := pollPair(t, ts, p.DeviceSecret); c != 200 || b["status"] != "pending" {
		t.Errorf("still waiting: %d %v", c, b)
	}
	// A TV can't see or use the code with its secret, nor approve itself.
	if w := ts.do(t, "POST", "/api/pair/approve", p.DeviceSecret, map[string]string{"code": p.Code}); w.Code != 401 {
		t.Errorf("secret as a token: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/pair/deny", adult, map[string]string{"code": p.Code}); w.Code != 200 {
		t.Errorf("deny: %d", w.Code)
	}
	if c, b := pollPair(t, ts, p.DeviceSecret); c != 403 || b["status"] != "denied" {
		t.Errorf("denied: %d %v", c, b)
	}
	if c, _ := pollPair(t, ts, p.DeviceSecret); c != 410 {
		t.Errorf("after denial: %d", c)
	}
}

func TestPairingAsAManagedProfile(t *testing.T) {
	ts := newTestServer(t)
	admin, adminTok := ts.account(t, "Boss", "adult", true)
	_, adult := ts.account(t, "Alex", "adult", false)
	kid, _ := ts.account(t, "Sam", "kids", false)
	other, _ := ts.account(t, "Robin", "adult", false)

	p := startPair(t, ts)
	// An adult can put a Kids profile on the TV, but no other adult or the admin.
	for _, id := range []int64{admin.ID, other.ID} {
		if w := ts.do(t, "POST", "/api/pair/approve", adult, map[string]any{"code": p.Code, "userId": id}); w.Code != 403 {
			t.Errorf("as account %d: %d", id, w.Code)
		}
	}
	w := ts.do(t, "POST", "/api/pair/info", adult, map[string]string{"code": p.Code})
	if profiles, _ := decode[map[string]any](t, w)["profiles"].([]any); len(profiles) != 2 {
		t.Errorf("an adult may choose themselves and the Kids profile, got %v", profiles)
	}
	if w := ts.do(t, "POST", "/api/pair/approve", adult, map[string]any{"code": p.Code, "userId": kid.ID}); w.Code != 200 {
		t.Fatalf("as the Kids profile: %d %s", w.Code, w.Body.String())
	}
	_, b := pollPair(t, ts, p.DeviceSecret)
	tok, _ := b["token"].(string)
	me := decode[map[string]any](t, ts.do(t, "GET", "/api/me", tok, nil))
	if me["name"] != "Sam" {
		t.Errorf("signed in as %v", me["name"])
	}
	_ = adminTok
}

func TestPairingCodesExpire(t *testing.T) {
	ts := newTestServer(t)
	_, session := ts.account(t, "Alex", "adult", false)
	p := startPair(t, ts)
	ts.pair.mu.Lock()
	ts.pair.byCode[cleanPairCode(p.Code)].expires = time.Now().Add(-time.Second)
	ts.pair.mu.Unlock()
	if w := ts.do(t, "POST", "/api/pair/approve", session, map[string]string{"code": p.Code}); w.Code != 404 {
		t.Errorf("approve an expired code: %d", w.Code)
	}
	if c, b := pollPair(t, ts, p.DeviceSecret); c != 410 || b["status"] != "expired" {
		t.Errorf("poll an expired code: %d %v", c, b)
	}
}

func TestPairingIsRateLimited(t *testing.T) {
	ts := newTestServer(t)
	_, session := ts.account(t, "Alex", "adult", false)
	// Codes open at once from one address.
	for i := 0; i < pairMaxPerIP; i++ {
		startPair(t, ts)
	}
	w := ts.do(t, "POST", "/api/pair/start", "", map[string]string{"device": "x"})
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("too many open codes: %d %v", w.Code, w.Header())
	}
	// Starts per window, even when the codes are finished with.
	ts.pair.mu.Lock()
	for c, p := range ts.pair.byCode {
		delete(ts.pair.byCode, c)
		delete(ts.pair.bySecret, p.secret)
	}
	now := time.Now()
	ts.pair.starts["192.168.1.50"] = nil
	for i := 0; i < pairStartsPerIP; i++ {
		ts.pair.starts["192.168.1.50"] = append(ts.pair.starts["192.168.1.50"], now)
	}
	ts.pair.mu.Unlock()
	if w := ts.do(t, "POST", "/api/pair/start", "", map[string]string{"device": "x"}); w.Code != 429 {
		t.Errorf("too many starts: %d", w.Code)
	}
	// Other addresses aren't affected, and the window passes.
	ts.pair.mu.Lock()
	ts.pair.starts["192.168.1.50"] = []time.Time{now.Add(-pairStartsWindow - time.Second)}
	ts.pair.mu.Unlock()
	p := startPair(t, ts)

	// Polling too fast is told to slow down, and doesn't lose the code.
	first := ts.do(t, "POST", "/api/pair/poll", "", map[string]string{"deviceSecret": p.DeviceSecret})
	second := ts.do(t, "POST", "/api/pair/poll", "", map[string]string{"deviceSecret": p.DeviceSecret})
	if first.Code != 200 || second.Code != 429 || decode[map[string]any](t, second)["status"] != "slow-down" {
		t.Errorf("fast polling: %d then %d", first.Code, second.Code)
	}

	// Guessing codes: after a few wrong ones the account has to wait.
	for i := 0; i < 5; i++ {
		if w := ts.do(t, "POST", "/api/pair/approve", session, map[string]string{"code": "BBBB-BBBB"}); w.Code != 404 {
			t.Fatalf("wrong code %d: %d", i, w.Code)
		}
	}
	if w := ts.do(t, "POST", "/api/pair/approve", session, map[string]string{"code": p.Code}); w.Code != 429 {
		t.Errorf("after 5 wrong codes even a right one waits: %d", w.Code)
	}
	// Someone else on the same network can still approve.
	_, other := ts.account(t, "Robin", "adult", false)
	if w := ts.do(t, "POST", "/api/pair/approve", other, map[string]string{"code": p.Code}); w.Code != 200 {
		t.Errorf("another account: %d %s", w.Code, w.Body.String())
	}
}

func TestPairingStartValidates(t *testing.T) {
	ts := newTestServer(t)
	// Before there is any account nobody could approve.
	if w := ts.do(t, "POST", "/api/pair/start", "", map[string]string{"device": "x"}); w.Code != 409 {
		t.Errorf("before setup: %d", w.Code)
	}
	ts.account(t, "Alex", "adult", false)
	if w := ts.do(t, "POST", "/api/pair/start", "", "{not json"); w.Code != 400 {
		t.Errorf("bad body: %d", w.Code)
	}
	long := strings.Repeat("é", 200) + "\x07\n"
	w := ts.do(t, "POST", "/api/pair/start", "", map[string]string{"device": long})
	if w.Code != 200 {
		t.Fatalf("long name: %d", w.Code)
	}
	code := decode[pairStarted](t, w).Code
	ts.pair.mu.Lock()
	dev := ts.pair.byCode[cleanPairCode(code)].device
	ts.pair.mu.Unlock()
	if len(dev) > 60 || strings.ContainsAny(dev, "\x07\n") {
		t.Errorf("device label kept as %q", dev)
	}
	for _, b := range []any{map[string]string{}, map[string]string{"deviceSecret": "short"}} {
		if w := ts.do(t, "POST", "/api/pair/poll", "", b); w.Code != 400 {
			t.Errorf("bad poll %v: %d", b, w.Code)
		}
	}
	if w := ts.do(t, "POST", "/api/pair/poll", "", map[string]string{"deviceSecret": strings.Repeat("0", 64)}); w.Code != 410 {
		t.Errorf("unknown secret: %d", w.Code)
	}
}

// The TV's secret and the session it collects must never reach the log.
func TestPairingLogsNoSecrets(t *testing.T) {
	ts := newTestServer(t)
	var log strings.Builder
	ts.Log = slog.New(slog.NewTextHandler(io.MultiWriter(&log), nil))
	_, session := ts.account(t, "Alex", "adult", false)
	p := startPair(t, ts)
	ts.do(t, "POST", "/api/pair/approve", session, map[string]string{"code": p.Code})
	_, b := pollPair(t, ts, p.DeviceSecret)
	raw, _ := json.Marshal(b)
	if s := log.String(); strings.Contains(s, p.DeviceSecret) || strings.Contains(s, p.Code) || strings.Contains(s, b["token"].(string)) {
		t.Errorf("secrets in the log: %s (%s)", s, raw)
	}
}
