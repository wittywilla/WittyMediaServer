package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
)

func TestLoginLimiter(t *testing.T) {
	var l loginLimiter
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = "192.168.1.20:5000"
	k := l.key(r, "Alice")
	if l.key(r, " alice ") != k {
		t.Fatal("names should match regardless of case and spaces")
	}
	for i := 0; i < 5; i++ {
		if d := l.begin(k, freeTriesPerName); d != 0 {
			t.Fatalf("locked after %d attempts, want 5", i)
		}
	}
	if l.begin(k, freeTriesPerName) == 0 {
		t.Fatal("not locked after 5 failures")
	}
	other := httptest.NewRequest("POST", "/api/login", nil)
	other.RemoteAddr = "192.168.1.21:5000"
	if l.begin(l.key(other, "alice"), freeTriesPerName) != 0 {
		t.Fatal("another device should not be locked out")
	}
	l.ok(k)
	if l.begin(k, freeTriesPerName) != 0 {
		t.Fatal("a successful sign-in should clear the lock")
	}
}

// Guesses sent at the same time used to all pass the check, because a failure
// was only counted when its password check had finished.
func TestLoginLimiterHoldsUnderParallelGuesses(t *testing.T) {
	ts := newTestServer(t)
	ts.user(t, "victim", "adult", false, "")
	var wg sync.WaitGroup
	var checked atomic.Int64
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := ts.do(t, "POST", "/api/login", "", map[string]string{"name": "victim", "password": fmt.Sprint("guess", i)}); w.Code == 401 {
				checked.Add(1)
			}
		}()
	}
	wg.Wait()
	if n := checked.Load(); n > freeTriesPerName {
		t.Fatalf("%d passwords were checked in parallel, want at most %d", n, freeTriesPerName)
	}
}

// Trying one password against many names from one address is limited too.
func TestLoginLimiterStopsSpraying(t *testing.T) {
	var l loginLimiter
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = "8.8.8.8:1"
	a := addressKey(r)
	for i := 0; i < freeTriesPerAddress; i++ {
		if l.begin(a, freeTriesPerAddress) != 0 {
			t.Fatalf("locked after %d", i)
		}
	}
	if l.begin(a, freeTriesPerAddress) == 0 {
		t.Fatal("one address may try any number of names")
	}
	six := httptest.NewRequest("POST", "/api/login", nil)
	six.RemoteAddr = "[2001:4860:4860:1::1]:1"
	six2 := httptest.NewRequest("POST", "/api/login", nil)
	six2.RemoteAddr = "[2001:4860:4860:1:ffff::2]:1"
	if addressKey(six) != addressKey(six2) {
		t.Fatal("addresses of one IPv6 network must share a count")
	}
}

// A sign-in's name becomes a key kept in memory: without a limit, repeating
// a 4 MB name filled the server's memory with no account at all.
func TestLoginLimiterMemoryIsBounded(t *testing.T) {
	var l loginLimiter
	r := httptest.NewRequest("POST", "/api/login", nil)
	if k := l.key(r, strings.Repeat("x", 1<<20)); len(k) > 200 {
		t.Fatalf("key of %d bytes", len(k))
	}
	for i := 0; i < maxLimiterEntries+500; i++ {
		l.begin(fmt.Sprint("k", i), freeTriesPerName)
	}
	if len(l.m) > maxLimiterEntries {
		t.Fatalf("%d entries kept", len(l.m))
	}
	ts := newTestServer(t)
	body := []byte(`{"name":"` + strings.Repeat("x", 1<<20) + `","password":"x"}`)
	if w := ts.do(t, "POST", "/api/login", "", body); w.Code != 400 {
		t.Fatalf("a 1 MB sign-in: status %d", w.Code)
	}
}

// DNS rebinding: a page on the internet points its own domain at this server.
// It arrives from a local address but names a host only the internet resolves.
func TestRebindingIsNeverLocal(t *testing.T) {
	for host, want := range map[string]bool{
		"127.0.0.1:9797": true, "[::1]:9797": true, "192.168.1.5:9797": true, "localhost:9797": true, "mediabox:9797": true, "mediabox.local": true,
		"media.lan:9797": true, "media.home.arpa": true, "nas.fritz.box": true, "x.localhost": true, "evil.example.com:9797": false, "evil.com": false, "127.0.0.1.nip.io:9797": false, "localhost.evil.com": false, "": false,
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "192.168.1.20:1"
		r.Host = host
		if got := onLocalNetwork(r); got != want {
			t.Errorf("Host %q: onLocalNetwork = %v, want %v", host, got, want)
		}
	}
}

func TestSetupClaim(t *testing.T) {
	creds := map[string]string{"name": "owner", "password": "a long password"}
	// A website rebinding its domain to a fresh server in the visitor's home.
	ts := newTestServer(t)
	if w := ts.do(t, "POST", "/api/setup", "", creds, "Host", "evil.example.com:9797", "RemoteAddr", "192.168.1.9:1"); w.Code != 403 {
		t.Fatalf("rebinding page claimed a fresh server: %d", w.Code)
	}
	// Someone on the internet finding a server still waiting for setup.
	if w := ts.do(t, "POST", "/api/setup", "", creds, "Host", "203.0.113.5:9797", "RemoteAddr", "198.51.100.7:1"); w.Code != 403 {
		t.Fatalf("a stranger claimed a fresh server: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/setup", "", creds, "X-Forwarded-For", "198.51.100.7"); w.Code != 403 {
		t.Fatalf("through a proxy, without the code: %d", w.Code)
	}
	if ts.Auth.UserCount(context.Background()) != 0 {
		t.Fatal("an account was made")
	}
	var st map[string]any
	w := ts.do(t, "GET", "/api/status", "", nil, "Host", "evil.example.com:9797")
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil || st["setupCodeRequired"] != true {
		t.Fatalf("status: %s", w.Body)
	}
	// The owner, from another network, with the code from the log.
	withCode := map[string]string{"name": "owner", "password": "a long password", "setupCode": strings.ToLower(ts.SetupCode())}
	if w := ts.do(t, "POST", "/api/setup", "", withCode, "Host", "media.example.com", "X-Forwarded-For", "198.51.100.7"); w.Code != 200 {
		t.Fatalf("with the code: %d %s", w.Code, w.Body)
	}
	// Setup can't run twice.
	if w := ts.do(t, "POST", "/api/setup", "", creds); w.Code != 409 {
		t.Fatalf("second setup: %d", w.Code)
	}
}

func TestSetupFromThisNetwork(t *testing.T) {
	ts := newTestServer(t)
	if w := ts.do(t, "POST", "/api/setup", "", map[string]string{"name": "owner", "password": "a long password"}); w.Code != 200 {
		t.Fatalf("local setup: %d %s", w.Code, w.Body)
	}
}

// A password-free Kids profile signs in with a tap at home, not from a page
// that rebinds its domain to this server.
func TestPasswordFreeProfileNeedsRealLocalHost(t *testing.T) {
	ts := newTestServer(t)
	ts.user(t, "owner", "adult", true, "")
	if _, err := ts.Auth.CreateProfile(context.Background(), "kid", "", "kids", false, ""); err != nil {
		t.Fatal(err)
	}
	body := map[string]string{"name": "kid"}
	if w := ts.do(t, "POST", "/api/login", "", body); w.Code != 200 {
		t.Fatalf("at home: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/login", "", body, "Host", "evil.example.com:9797"); w.Code != 401 {
		t.Fatalf("from a rebinding page: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/profiles", "", nil, "Host", "evil.example.com:9797"); strings.Contains(w.Body.String(), "owner") {
		t.Fatalf("names listed to a rebinding page: %s", w.Body)
	}
}

func TestLocalNetwork(t *testing.T) {
	cases := map[string]bool{"127.0.0.1:1": true, "[::1]:1": true, "192.168.0.5:1": true, "10.1.2.3:1": true, "8.8.8.8:1": false, "[2001:db8::1]:1": false}
	for addr, want := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = addr
		r.Host = "192.168.0.2:9797"
		if got := onLocalNetwork(r); got != want {
			t.Errorf("%s: got %v want %v", addr, got, want)
		}
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1"
	r.Host = "127.0.0.1:9797"
	r.Header.Set("X-Forwarded-For", "8.8.8.8")
	if onLocalNetwork(r) || onThisMachine(r) {
		t.Error("a proxied request must count as remote")
	}
}

func TestAccessFor(t *testing.T) {
	if accessFor(&auth.User{Kind: "adult", IsAdmin: true, Restrictions: `{"maxAge":0}`}) != nil {
		t.Error("administrators are never limited")
	}
	if accessFor(&auth.User{Kind: "adult"}) != nil {
		t.Error("an adult nobody restricted has no limits")
	}
	if a := accessFor(&auth.User{Kind: "kids"}); a == nil || a.MaxAge != 7 {
		t.Errorf("a Kids profile without saved limits gets the defaults, got %+v", a)
	}
	if a := accessFor(&auth.User{Kind: "adult", Restrictions: "{broken"}); a == nil || a.MaxAge != 0 {
		t.Errorf("unreadable limits must fail closed, got %+v", a)
	}
}
