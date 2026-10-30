package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// A browser can send another server's (or an expired) cookie beside the
// token the page put in a media address: every token is offered, most
// explicit first.
func TestTokensOrder(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/items/1/poster?token=fromquery", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: "stalecookie"})
	if got, want := Tokens(r), []string{"fromquery", "stalecookie"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Tokens = %v, want %v", got, want)
	}
	r.Header.Set("Authorization", "Bearer fromheader")
	if got := TokenFrom(r); got != "fromheader" {
		t.Fatalf("TokenFrom = %q, want the header's token", got)
	}
	if got := Tokens(httptest.NewRequest("GET", "/", nil)); len(got) != 0 {
		t.Fatalf("no credentials: Tokens = %v", got)
	}
}

// A token in an address only works for media, and only to read it.
func TestQueryTokenOnlyOnMediaRoutes(t *testing.T) {
	for target, want := range map[string]bool{
		"/api/items/1/poster?token=x":             true,
		"/api/items/1/file?token=x":               true,
		"/api/items/1/epub/OEBPS/a.xhtml?token=x": true,
		"/api/shows/3/art/poster?token=x":         true,
		"/api/hls/v-1/playlist.m3u8?token=x":      true,
		"/api/dvd/abc/still/x.png?token=x":        true,
		"/api/logs/files/wms-1.log?token=x":       true,
		"/api/users?token=x":                      false,
		"/api/settings?token=x":                   false,
		"/api/me?token=x":                         false,
		"/api/fs?token=x&path=/":                  false,
		"/api/items/1?token=x":                    false,
	} {
		if got := len(Tokens(httptest.NewRequest("GET", target, nil))) == 1; got != want {
			t.Errorf("GET %s: token used = %v, want %v", target, got, want)
		}
	}
	if got := Tokens(httptest.NewRequest("POST", "/api/items/1/stream?token=x", nil)); len(got) != 0 {
		t.Errorf("a token in the address must not authorise a change: %v", got)
	}
}

func testService(t *testing.T) *Service {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return &Service{DB: d}
}

// The database must hold no working sign-in: a stolen copy is not a session.
func TestSessionTokensAreHashedAtRest(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if _, err := s.CreateUser(ctx, "a", "long enough pw", true); err != nil {
		t.Fatal(err)
	}
	tok, _, err := s.Login(ctx, "a", "long enough pw", "", false)
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := s.DB.QueryRowContext(ctx, `SELECT token FROM sessions`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, tok) {
		t.Fatal("the token is stored as is")
	}
	if u, err := s.Resolve(ctx, tok); err != nil || u.Name != "a" {
		t.Fatalf("the token no longer resolves: %v", err)
	}
	if _, err := s.Resolve(ctx, stored); err == nil {
		t.Fatal("the stored value works as a token")
	}
	s.Logout(ctx, tok)
	if _, err := s.Resolve(ctx, tok); err == nil {
		t.Fatal("logged out, still resolves")
	}
}

// Sessions from before hashing keep working after the upgrade.
func TestHashStoredTokensKeepsOldSessions(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	u, err := s.CreateUser(ctx, "a", "long enough pw", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO sessions(token, user_id, created_at, expires_at) VALUES('oldtoken', ?, 1, ?)`, u.ID, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // running twice changes nothing
		if err := s.HashStoredTokens(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Resolve(ctx, "oldtoken"); err != nil {
		t.Fatalf("old session lost: %v", err)
	}
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE token = 'oldtoken'`).Scan(&n)
	if n != 0 {
		t.Fatal("the plain token is still stored")
	}
}

func TestPasswordRules(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if _, err := s.CreateProfile(ctx, "adult", "1234", KindAdult, true, ""); err == nil {
		t.Error("an adult account took a 4-character password")
	}
	if _, err := s.CreateProfile(ctx, "long", strings.Repeat("x", 73), KindAdult, false, ""); err == nil {
		t.Error("a password longer than bcrypt reads was accepted")
	}
	kid, err := s.CreateProfile(ctx, "kid", "1234", KindKids, false, "")
	if err != nil {
		t.Fatalf("a Kids PIN: %v", err)
	}
	if err := s.SetPassword(ctx, kid.ID, "99"); err == nil {
		t.Error("a 2-character PIN was accepted")
	}
	a, err := s.CreateProfile(ctx, "grownup", "long enough pw", KindAdult, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPassword(ctx, a.ID, "short"); err == nil {
		t.Error("an adult's new password can't be short")
	}
}

func streamRequest(method, path, tok string) *http.Request {
	return httptest.NewRequest(method, path+"?token="+tok, nil)
}

// A stream token opens one title or stream, to read it, and nothing else.
func TestStreamTokenScope(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if _, err := s.CreateUser(ctx, "a", "long enough pw", true); err != nil {
		t.Fatal(err)
	}
	session, _, err := s.Login(ctx, "a", "long enough pw", "", false)
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := StreamToken(session, "/api/hls/abc/", time.Hour)
	if strings.Contains(tok, session) {
		t.Fatal("a stream token must not contain the session's token")
	}
	for _, c := range []struct {
		method, path string
		want         bool
	}{
		{"GET", "/api/hls/abc/playlist.m3u8", true},
		{"GET", "/api/hls/abc/seg/1.ts", true},
		{"HEAD", "/api/hls/abc/seg/1.ts", true},
		{"DELETE", "/api/hls/abc/", false},
		{"POST", "/api/hls/abc/seg/1.ts", false},
		{"GET", "/api/hls/other/seg/1.ts", false},
		{"GET", "/api/hls/abc/../other/seg/1.ts", false},
		{"GET", "/api/users", false},
		{"GET", "/api/items/1/file", false},
	} {
		if _, err := s.ResolveStream(ctx, tok, streamRequest(c.method, c.path, tok)); (err == nil) != c.want {
			t.Errorf("%s %s: got %v, want allowed=%v", c.method, c.path, err, c.want)
		}
	}
	// As a session token it is nothing.
	if _, err := s.Resolve(ctx, tok); err == nil {
		t.Error("a stream token worked as a session")
	}
	// Changed in any way, it is nothing.
	for _, bad := range []string{tok[:len(tok)-2] + "aa", "st1." + tok[6:], "st1.x", "st1.", strings.Replace(tok, ".", "", 2)} {
		if _, err := s.ResolveStream(ctx, bad, streamRequest("GET", "/api/hls/abc/seg/1.ts", "x")); err == nil {
			t.Errorf("tampered token %q was accepted", bad)
		}
	}
	// Past its time, it is nothing.
	old, _ := StreamToken(session, "/api/hls/abc/", -time.Minute)
	if _, err := s.ResolveStream(ctx, old, streamRequest("GET", "/api/hls/abc/seg/1.ts", old)); err == nil {
		t.Error("an expired stream token was accepted")
	}
	// Signing out ends it at once.
	s.Logout(ctx, session)
	if _, err := s.ResolveStream(ctx, tok, streamRequest("GET", "/api/hls/abc/seg/1.ts", tok)); err == nil {
		t.Error("a stream token outlived its session")
	}
}

func TestStreamScope(t *testing.T) {
	for path, want := range map[string]string{
		"/api/items/5/stream":          "/api/items/5/",
		"/api/items/5/poster":          "/api/items/5/",
		"/api/shows/3/art/poster":      "/api/shows/3/art/",
		"/api/hls/v-1/playlist.m3u8":   "/api/hls/v-1/",
		"/api/dvd/abc/still/x.png":     "/api/dvd/abc/",
		"/api/items/5":                 "",
		"/api/items/5/annotations":     "",
		"/api/users":                   "",
		"/api/logs/files/wms-1.log":    "",
		"/api/artwork/preview":         "",
		"/api/hls/../users/x":          "",
		"https://evil.test/api/hls/x/": "",
	} {
		if got := StreamScope(path); got != want {
			t.Errorf("%s: %q, want %q", path, got, want)
		}
	}
}
