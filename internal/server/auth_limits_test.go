package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func requestFrom(addr string) *http.Request {
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = addr + ":1"
	return r
}

// Filling the table used to wipe every running count, so a stranger could guess at one name
// without ever being limited, by sending made-up names in between.
func TestFloodCantEraseTheLimits(t *testing.T) {
	var l loginLimiter
	guesser, flood := requestFrom("203.0.113.1"), requestFrom("203.0.113.2")
	for i := range 4 {
		if d := l.attempt(guesser, "Owner"); d != 0 {
			t.Fatalf("locked after %d", i)
		}
	}
	for i := range 25000 {
		l.attempt(flood, fmt.Sprint("made-up-", i))
	}
	if len(l.m) > maxLimiterEntries {
		t.Fatalf("%d entries", len(l.m))
	}
	if d := l.attempt(guesser, "Owner"); d != 0 {
		t.Fatalf("the fifth try: %v", d)
	}
	if d := l.attempt(guesser, "Owner"); d == 0 {
		t.Fatal("the count was forgotten: the sixth try went through")
	}
}

// With many addresses the table can fill. What goes is the oldest per-name counts, never the limits
// for an address or an account that are still running.
func TestFullTableKeepsAddressAndAccountCounts(t *testing.T) {
	var l loginLimiter
	guesser := requestFrom("203.0.113.1")
	for range 3 {
		l.attempt(guesser, "Owner")
	}
	for a := range 3000 {
		r := requestFrom(fmt.Sprintf("198.18.%d.%d", a/250, a%250))
		for n := range 20 {
			l.attempt(r, fmt.Sprintf("name-%d-%d", a, n))
		}
	}
	if len(l.m) > maxLimiterEntries {
		t.Fatalf("%d entries", len(l.m))
	}
	for _, k := range []string{"addr|203.0.113.1", "acct|owner"} {
		if _, ok := l.m[k]; !ok {
			t.Errorf("%s was forgotten", k)
		}
	}
}

// One address can't grow the table without limit, even at home, where nothing else limits it. Names
// past the cap share one count, whether or not they exist, so this says nothing about who does.
func TestOneAddressHasACapOnNames(t *testing.T) {
	var l loginLimiter
	home := requestFrom("192.168.1.20")
	for range 3 {
		l.attempt(home, "Owner")
	}
	for i := range 5000 {
		l.attempt(home, fmt.Sprint("made-up-", i))
	}
	if n := len(l.m); n > maxNamesPerAddress+1 {
		t.Fatalf("%d entries from one address", n)
	}
	if d := l.attempt(home, "Owner"); d != 0 {
		t.Fatalf("a name it was already counting: %v", d)
	}
	if d := l.attempt(home, "Owner"); d != 0 {
		t.Fatalf("the fifth try: %v", d)
	}
	if d := l.attempt(home, "Owner"); d == 0 {
		t.Fatal("the count for a known name was lost")
	}
	if d := l.attempt(home, "someone-new"); d == 0 {
		t.Fatal("a new name past the cap should share the locked count")
	}
	if d := l.attempt(requestFrom("192.168.1.21"), "someone-new"); d != 0 {
		t.Fatalf("another device isn't held back: %v", d)
	}
}

// A person whose address is locked counts nothing further, so one address can't lock a name for
// everyone else.
func TestOneAddressCantLockANameForEveryone(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	putRemote(t, h, tok, map[string]any{"enabled": true})
	body := map[string]string{"name": "Owner", "password": "wrong password"}
	for range 40 {
		call(h, "POST", "/api/login", public, body, "TLS: yes")
	}
	right := map[string]string{"name": "Owner", "password": pass}
	if r := call(h, "POST", "/api/login", "203.0.113.51", right, "TLS: yes"); r.code != 200 {
		t.Fatalf("the owner, from somewhere else: %d %s", r.code, r.body)
	}
}

// Quiet for half an hour used to start every count over, so a limit of hours was minutes.
func TestQuietDoesntStartCountsOverSoon(t *testing.T) {
	var l loginLimiter
	r := requestFrom("203.0.113.1")
	for range pairFails {
		l.attempt(r, "Owner")
	}
	if d := l.attempt(r, "Owner"); d == 0 {
		t.Fatal("not locked")
	}
	for _, a := range l.m {
		a.until = time.Time{}
		a.last = time.Now().Add(-31 * time.Minute)
	}
	if d := l.attempt(r, "Owner"); d != 0 {
		t.Fatalf("the wait is over: %v", d)
	}
	if d := l.attempt(r, "Owner"); d == 0 {
		t.Fatal("31 quiet minutes started the count over")
	}
	for _, a := range l.m {
		a.until = time.Time{}
		a.last = time.Now().Add(-failMemory - time.Minute)
	}
	if d := l.attempt(r, "Owner"); d != 0 {
		t.Fatalf("after failMemory: %v", d)
	}
	if d := l.attempt(r, "Owner"); d != 0 {
		t.Fatalf("after failMemory the count does start over: %v", d)
	}
}
