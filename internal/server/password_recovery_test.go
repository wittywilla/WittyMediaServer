package server

import (
	"context"
	"fmt"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// Whoever manages a profile can give it a new password or PIN, which is the way out of a
// forgotten one. It signs them out, and nobody else can do it.
func TestSettingSomeoneElsesPassword(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()
	_, owner := ts.user(t, "owner", "adult", true, "")
	parent, parentTok := ts.user(t, "parent", "adult", false, "")
	kid, kidTok := ts.user(t, "kid", "kids", false, "")
	guest, _ := ts.user(t, "guest", "guest", false, "")
	adult, adultTok := ts.user(t, "adult", "adult", false, "")

	set := func(tok string, id int64, body map[string]string) int {
		t.Helper()
		ts.logins.clear()
		return ts.do(t, "PUT", fmt.Sprintf("/api/users/%d/password", id), tok, body).Code
	}
	// A Kids PIN, set by an adult who manages Kids profiles, asks for nothing more.
	if c := set(parentTok, kid.ID, map[string]string{"password": "4321"}); c != 200 {
		t.Fatalf("an adult setting a Kids PIN: %d", c)
	}
	if !ts.Auth.CheckPassword(ctx, kid.ID, "4321") {
		t.Fatal("the PIN wasn't set")
	}
	if w := ts.do(t, "GET", "/api/me", kidTok, nil); w.Code != 401 {
		t.Errorf("the profile should be signed out: %d", w.Code)
	}
	// An administrator can do a Guest and an adult.
	if c := set(owner, guest.ID, map[string]string{"password": "9876"}); c != 200 {
		t.Errorf("an administrator setting a Guest PIN: %d", c)
	}
	if c := set(owner, adult.ID, map[string]string{"password": "a good new password"}); c != 200 {
		t.Errorf("an administrator setting an adult's password: %d", c)
	}
	if w := ts.do(t, "GET", "/api/me", adultTok, nil); w.Code != 401 {
		t.Errorf("the adult should be signed out: %d", w.Code)
	}
	// An adult's rules stay an adult's: a PIN isn't enough.
	if c := set(owner, parent.ID, map[string]string{"password": "1234"}); c != 400 {
		t.Errorf("a PIN for an adult: %d", c)
	}
	// Nobody else.
	_, kid2 := ts.user(t, "kid2", "kids", false, "")
	_, adult2 := ts.user(t, "adult2", "adult", false, "")
	if c := set(kid2, kid.ID, map[string]string{"password": "1111"}); c != 403 {
		t.Errorf("a Kids profile changing another's: %d", c)
	}
	if c := set(adult2, parent.ID, map[string]string{"password": "someone else's"}); c != 403 {
		t.Errorf("an adult changing another adult's: %d", c)
	}
	if c := set("", kid.ID, map[string]string{"password": "1111"}); c != 401 {
		t.Errorf("signed out: %d", c)
	}
	if !ts.Auth.CheckPassword(ctx, kid.ID, "4321") {
		t.Error("someone changed the PIN who shouldn't have")
	}
	// Taking over the administrator takes the administrator's own proof, from any address.
	admin, _ := ts.user(t, "second", "adult", true, "")
	if c := set(parentTok, admin.ID, map[string]string{"password": "attacker chosen"}); c != 403 {
		t.Errorf("an adult changing an administrator's: %d", c)
	}
	if c := set(owner, admin.ID, map[string]string{"password": "attacker chosen"}); c != 403 {
		t.Errorf("another administrator's, without the caller's own password: %d", c)
	}
}

// A request hung up on before its database read must not look like a server with no accounts.
// Nor must a table that can't be read: first-run rules open a good deal.
func TestUnreadableAccountsAreNotAFirstRun(t *testing.T) {
	ts := newTestServer(t)
	ts.user(t, "owner", "adult", true, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ts.Auth.NoAccounts(ctx) {
		t.Error("a cancelled request saw no accounts")
	}
	r := httptest.NewRequest("GET", "/api/backup", nil).WithContext(ctx)
	r.RemoteAddr = "192.168.1.50:1000"
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	if w.Code == 200 {
		t.Errorf("a cancelled request from home got the backups: %d %s", w.Code, w.Body)
	}
	if _, err := ts.DB.ExecContext(context.Background(), `ALTER TABLE users RENAME TO users_away`); err != nil {
		t.Fatal(err)
	}
	if ts.Auth.NoAccounts(context.Background()) {
		t.Error("a table that can't be read counted as no accounts")
	}
	if _, err := ts.DB.ExecContext(context.Background(), `ALTER TABLE users_away RENAME TO users`); err != nil {
		t.Fatal(err)
	}
}

// Making a restore's first account takes the same proof as making any other first account.
func TestFirstRunRestoreAsksForTheSetupCodeFromHome(t *testing.T) {
	ts := newTestServer(t)
	r := httptest.NewRequest("POST", "/api/backup/restore/start", strings.NewReader(`{"confirm":"RESTORE"}`))
	r.RemoteAddr, r.Host = "192.168.1.50:1000", "192.168.1.2:9797"
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	if w.Code != 403 || !strings.Contains(w.Body.String(), "setup code") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

// The recovery on the machine has no address. The only route that sets a password is the one
// above, which takes a signed-in session whichever address it comes from.
func TestNoAddressRecoversAPassword(t *testing.T) {
	ts := newTestServer(t)
	user, _ := ts.user(t, "owner", "adult", true, "")
	// A server reset asks for the administrator's password, and two-factor's routes for a code:
	// neither sets a password.
	sets := regexp.MustCompile(`(?i)password|recover|forgot`)
	known := regexp.MustCompile(`^(PUT /api/users/\{id\}/password|(GET|POST|PUT|DELETE) /api/(reset|me/2fa)\b.*)$`)
	for route := range routeRules {
		if sets.MatchString(route) && !known.MatchString(route) {
			t.Errorf("%s: a route with this name needs looking at: recovering a password must stay off the network", route)
		}
	}
	// Looking as if it came from the server's own machine gets nowhere either.
	for _, hdr := range [][]string{nil, {"X-Forwarded-For", "127.0.0.1"}, {"X-Real-IP", "127.0.0.1"}, {"Host", "localhost"}} {
		w := ts.do(t, "PUT", fmt.Sprintf("/api/users/%d/password", user.ID), "", map[string]string{"password": "attacker chosen"}, hdr...)
		if w.Code != 401 && w.Code != 403 {
			t.Errorf("%v: %d %s", hdr, w.Code, w.Body)
		}
	}
	for _, path := range []string{"/api/reset-password", "/api/recover", "/api/users/recover", "/api/repair/reset-password", "/api/auth/recover"} {
		if w := ts.do(t, "POST", path, "", map[string]string{"name": "owner", "password": "attacker chosen"}); w.Code != 404 && w.Code != 405 {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
	if !ts.Auth.CheckPassword(context.Background(), user.ID, "correct-horse-1") {
		t.Error("the password changed")
	}
}
