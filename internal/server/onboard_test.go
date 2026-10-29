package server

import (
	"net/http/httptest"
	"testing"
)

func TestLoginLimiter(t *testing.T) {
	var l loginLimiter
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = "192.168.1.20:5000"
	k := l.key(r, "Alice")
	if l.key(r, " alice ") != k {
		t.Fatal("names should match regardless of case and spaces")
	}
	for i := 0; i < 4; i++ {
		l.fail(k)
		if l.wait(k) != 0 {
			t.Fatalf("locked after %d failures, want 5", i+1)
		}
	}
	l.fail(k)
	if l.wait(k) == 0 {
		t.Fatal("not locked after 5 failures")
	}
	other := httptest.NewRequest("POST", "/api/login", nil)
	other.RemoteAddr = "192.168.1.21:5000"
	if l.wait(l.key(other, "alice")) != 0 {
		t.Fatal("another device should not be locked out")
	}
	l.ok(k)
	if l.wait(k) != 0 {
		t.Fatal("a successful sign-in should clear the lock")
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
