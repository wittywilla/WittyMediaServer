package server

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
)

func TestLoginLimiter(t *testing.T) {
	var l loginLimiter
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = "192.168.1.20:5000"
	k, _ := l.keys(r, "Alice")
	if k2, _ := l.keys(r, " alice "); k2 != k {
		t.Fatal("names should match regardless of case and spaces")
	}
	for i := 0; i < 4; i++ {
		l.fail(k, 5)
		if l.wait(k) != 0 {
			t.Fatalf("locked after %d failures, want 5", i+1)
		}
	}
	l.fail(k, 5)
	if l.wait(k) == 0 {
		t.Fatal("not locked after 5 failures")
	}
	other := httptest.NewRequest("POST", "/api/login", nil)
	other.RemoteAddr = "192.168.1.21:5000"
	if ko, _ := l.keys(other, "alice"); l.wait(ko) != 0 {
		t.Fatal("another device should not be locked out")
	}
	l.ok(k)
	if l.wait(k) != 0 {
		t.Fatal("a successful sign-in should clear the lock")
	}
}

// One address trying one password on many accounts slows down too, and a
// flood of made-up names can't grow the limiter without bound.
func TestLoginLimiterAcrossNames(t *testing.T) {
	var l loginLimiter
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = "203.0.113.9:5000"
	_, ip := l.keys(r, "x")
	for i := 0; i < ipFailLimit-1; i++ {
		l.fail(ip, ipFailLimit)
	}
	if l.wait(ip) != 0 {
		t.Fatal("locked before the limit")
	}
	l.fail(ip, ipFailLimit)
	if l.wait(ip) == 0 {
		t.Fatal("not locked at the limit")
	}
	if k, _ := l.keys(r, strings.Repeat("n", 1<<20)); len(k) > 100 {
		t.Fatalf("a long name makes a key of %d bytes", len(k))
	}
	var flood loginLimiter
	for i := 0; i < 3*maxLoginEntries; i++ {
		flood.fail(strconv.Itoa(i), 5)
	}
	if len(flood.m) > maxLoginEntries+1 {
		t.Fatalf("limiter grew to %d entries", len(flood.m))
	}
}

func TestLocalNetwork(t *testing.T) {
	cases := map[string]bool{"127.0.0.1:1": true, "[::1]:1": true, "192.168.0.5:1": true, "10.1.2.3:1": true, "8.8.8.8:1": false, "[2001:db8::1]:1": false}
	for addr, want := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = addr
		if got := onLocalNetwork(r); got != want {
			t.Errorf("%s: got %v want %v", addr, got, want)
		}
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1"
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
