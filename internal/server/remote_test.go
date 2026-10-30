package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// A whole server without FFmpeg or media: enough for sign-in and settings.
func remoteServer(t *testing.T, cfg *config.Config) (*Server, http.Handler) {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if cfg == nil {
		cfg = &config.Config{}
	}
	if cfg.Listen == "" {
		cfg.Listen = ":9797"
	}
	s := &Server{Cfg: cfg, DB: d, Log: slog.New(slog.DiscardHandler), Auth: &auth.Service{DB: d}, Store: &library.Store{DB: d}, Tools: &ffmpeg.Tools{}, Web: fstest.MapFS{"index.html": {Data: []byte("app")}}}
	return s, s.Handler()
}

type reply struct {
	code int
	body string
	h    http.Header
}

func (r reply) json(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(r.body), v); err != nil {
		t.Fatalf("%v: %s", err, r.body)
	}
}

// call sends a request from an address, with "Header: value" strings.
func call(h http.Handler, method, path, from string, body any, hdr ...string) reply {
	var b bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&b).Encode(body)
	}
	r := httptest.NewRequest(method, path, &b)
	r.RemoteAddr = from + ":5000"
	if strings.Contains(from, ":") {
		r.RemoteAddr = "[" + from + "]:5000"
	}
	r.Host = "192.168.1.2:9797" // the address a browser at home has in its bar
	for _, kv := range hdr {
		k, v, _ := strings.Cut(kv, ": ")
		if k == "Host" {
			r.Host = v
			continue
		}
		if k == "TLS" { // a direct encrypted connection
			r.TLS = &tls.ConnectionState{ServerName: "media.example.com"}
			continue
		}
		r.Header.Add(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return reply{w.Code, w.Body.String(), w.Header()}
}

const (
	lan    = "192.168.1.20"
	public = "203.0.113.50"
	tail   = "100.101.102.103"
	pass   = "a long enough passphrase"
)

func adminToken(t *testing.T, s *Server, h http.Handler) string {
	t.Helper()
	if _, err := s.Auth.CreateUser(context.Background(), "Owner", pass, true); err != nil {
		t.Fatal(err)
	}
	r := call(h, "POST", "/api/login", lan, map[string]string{"name": "Owner", "password": pass})
	var out struct {
		Token string `json:"token"`
	}
	r.json(t, &out)
	if out.Token == "" {
		t.Fatalf("no token: %d %s", r.code, r.body)
	}
	return out.Token
}

func putRemote(t *testing.T, h http.Handler, token string, body any) remoteView {
	t.Helper()
	r := call(h, "PUT", "/api/remote", lan, body, "Authorization: Bearer "+token)
	if r.code != 200 {
		t.Fatalf("PUT /api/remote: %d %s", r.code, r.body)
	}
	var v remoteView
	r.json(t, &v)
	return v
}

func TestEverythingIsOffUntilTurnedOn(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	for _, from := range []string{public, tail, "2001:db8::9"} {
		if r := call(h, "GET", "/api/status", from, nil); r.code != 403 {
			t.Errorf("%s while off: got %d, want 403", from, r.code)
		}
		if r := call(h, "GET", "/", from, nil); r.code != 403 || strings.Contains(r.body, "<html") {
			t.Errorf("%s page while off: got %d %q", from, r.code, r.body)
		}
	}
	// A forwarded header claiming to be at home changes nothing.
	if r := call(h, "GET", "/api/status", public, nil, "X-Forwarded-For: 192.168.1.1", "X-Forwarded-Proto: https"); r.code != 403 {
		t.Errorf("spoofed header: got %d", r.code)
	}
	if r := call(h, "GET", "/api/status", "127.0.0.1", nil, "X-Forwarded-For: 192.168.1.1"); r.code != 403 {
		t.Errorf("an unlisted local proxy counts as outside: got %d", r.code)
	}
	// Home is unaffected.
	for _, from := range []string{lan, "127.0.0.1", "::1"} {
		if r := call(h, "GET", "/api/status", from, nil); r.code != 200 {
			t.Errorf("%s: got %d", from, r.code)
		}
	}
	r := call(h, "GET", "/api/remote", lan, nil, "Authorization: Bearer "+tok)
	var v remoteView
	r.json(t, &v)
	if v.State != "off" || v.Settings.Enabled || v.Settings.PortMapping || v.Settings.HTTPSMode != "off" {
		t.Errorf("defaults: %+v", v)
	}
}

func TestSetupStaysAtHome(t *testing.T) {
	_, h := remoteServer(t, nil)
	body := map[string]string{"name": "Mallory", "password": pass}
	if r := call(h, "POST", "/api/setup", public, body); r.code != 403 {
		t.Fatalf("first-run setup from the internet: got %d", r.code)
	}
	if r := call(h, "POST", "/api/setup", lan, body); r.code != 200 {
		t.Fatalf("setup at home: %d %s", r.code, r.body)
	}
}

func TestInternetNeedsHTTPS(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	putRemote(t, h, tok, map[string]any{"enabled": true})

	// VPNs encrypt by themselves.
	if r := call(h, "GET", "/api/status", tail, nil); r.code != 200 {
		t.Errorf("VPN: got %d", r.code)
	}
	// Plain HTTP from the internet: no outside address known, so refused.
	if r := call(h, "POST", "/api/login", public, map[string]string{"name": "Owner", "password": pass}); r.code != 403 {
		t.Errorf("a password over plain HTTP: got %d", r.code)
	}
	// With an outside address, reads are sent there. The target is always the
	// configured address, never what the request says.
	putRemote(t, h, tok, map[string]any{"publicUrl": "https://media.example.com/"})
	r := call(h, "GET", "/settings?x=1", public, nil, "Host: evil.example")
	if r.code != http.StatusPermanentRedirect || r.h.Get("Location") != "https://media.example.com/settings?x=1" {
		t.Errorf("redirect: %d %s", r.code, r.h.Get("Location"))
	}
	if r := call(h, "POST", "/api/login", public, nil); r.code != 403 {
		t.Errorf("sending a password is never redirected: got %d", r.code)
	}
}

func TestTrustedProxy(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	putRemote(t, h, tok, map[string]any{"enabled": true, "trustedProxies": "10.0.0.2"})
	// Encrypted by the proxy, client on the internet.
	if r := call(h, "GET", "/api/status", "10.0.0.2", nil, "X-Forwarded-For: "+public, "X-Forwarded-Proto: https"); r.code != 200 {
		t.Errorf("through the proxy: %d", r.code)
	}
	// The proxy forgot HTTPS: refused, not assumed.
	if r := call(h, "POST", "/api/login", "10.0.0.2", nil, "X-Forwarded-For: "+public, "X-Forwarded-Proto: http"); r.code != 403 {
		t.Errorf("unencrypted through the proxy: %d", r.code)
	}
	// Somebody else sending the same headers is just a visitor.
	if r := call(h, "POST", "/api/login", "10.0.0.3", nil, "X-Forwarded-For: 192.168.1.1"); r.code == 200 {
		t.Errorf("an unlisted machine's header was believed")
	}
	// The status tells the owner which address is suspected of being a proxy.
	call(h, "GET", "/api/status", "10.0.0.9", nil, "X-Forwarded-For: "+public, "X-Forwarded-Proto: https")
	call(h, "GET", "/api/status", "198.51.100.7", nil, "X-Forwarded-For: "+public, "X-Forwarded-Proto: https")
	var v remoteView
	call(h, "GET", "/api/remote", lan, nil, "Authorization: Bearer "+tok).json(t, &v)
	found := ""
	for _, n := range v.Notices {
		if n.Trust != "" {
			found = n.Trust
		}
	}
	if found != "10.0.0.9" {
		t.Errorf("proxy suggestion %q: only machines at home may be suggested, in %+v", found, v.Notices)
	}
}

func TestTrustListRefusesEverything(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	for _, bad := range []map[string]any{
		{"trustedProxies": "0.0.0.0/0"},
		{"homeNets": "::/0"},
		{"publicUrl": "http://media.example.com"},
		{"publicUrl": "https://media.example.com/path"},
		{"httpsMode": "acme", "domain": "media.example.com"}, // terms not accepted
		{"httpsMode": "acme", "acceptTerms": true, "domain": "192.168.1.5"},
		{"httpsMode": "cert", "certFile": "cert.pem", "keyFile": "key.pem"},
		{"portMapping": true}, // not without HTTPS
		{"httpsPort": 9797},   // the home port
		{"maxBitrate": "nonsense"},
		{"maxHeight": 123},
		{"method": "magic"},
		{"email": "not an address"},
	} {
		if r := call(h, "PUT", "/api/remote", lan, bad, "Authorization: Bearer "+tok); r.code != 400 {
			t.Errorf("%v: got %d %s", bad, r.code, r.body)
		}
	}
	var v remoteView
	call(h, "GET", "/api/remote", lan, nil, "Authorization: Bearer "+tok).json(t, &v)
	if v.Settings.TrustedProxies != "" || v.Settings.HTTPSMode != "off" {
		t.Errorf("a refused change must not stick: %+v", v.Settings)
	}
}

func TestRemoteSettingsAreForAdmins(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	_ = tok
	if _, err := s.Auth.CreateUser(context.Background(), "Alex", pass, false); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Token string `json:"token"`
	}
	call(h, "POST", "/api/login", lan, map[string]string{"name": "Alex", "password": pass}).json(t, &out)
	for _, c := range []struct{ m, p string }{{"GET", "/api/remote"}, {"PUT", "/api/remote"}, {"POST", "/api/remote/test"}, {"POST", "/api/remote/router"}, {"POST", "/api/remote/selfcheck"}, {"DELETE", "/api/remote/lockouts"}, {"GET", "/api/sessions?all=1"}} {
		if r := call(h, c.m, c.p, lan, nil, "Authorization: Bearer "+out.Token); r.code != 403 {
			t.Errorf("%s %s as an ordinary adult: %d", c.m, c.p, r.code)
		}
		if r := call(h, c.m, c.p, lan, nil); r.code != 401 {
			t.Errorf("%s %s signed out: %d", c.m, c.p, r.code)
		}
	}
}

func TestLoginLimitsAwayFromHome(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	putRemote(t, h, tok, map[string]any{"enabled": true})
	vpnWrong := func(from, name string) int {
		return call(h, "POST", "/api/login", from, map[string]string{"name": name, "password": "wrong"}).code
	}
	// One address spraying many names gets stopped.
	blocked := false
	for i := range addrFails + 2 {
		if vpnWrong(tail, "user"+string(rune('a'+i))) == 429 {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Fatal("spraying many names from one address should be limited")
	}
	// The same person at home is not affected by that limit.
	if r := call(h, "POST", "/api/login", lan, map[string]string{"name": "Owner", "password": pass}); r.code != 200 {
		t.Errorf("the owner at home: %d %s", r.code, r.body)
	}
	locks, fails := s.logins.snapshot()
	if len(locks) == 0 || fails == 0 {
		t.Errorf("snapshot: %v %d", locks, fails)
	}
	if r := call(h, "DELETE", "/api/remote/lockouts", lan, nil, "Authorization: Bearer "+tok); r.code != 200 {
		t.Error(r.code)
	}
	if locks, _ := s.logins.snapshot(); len(locks) != 0 {
		t.Errorf("after clearing: %v", locks)
	}
}

func TestAccountLockSparesHome(t *testing.T) {
	var l loginLimiter
	req := func(from string) *http.Request {
		r := httptest.NewRequest("POST", "/api/login", nil)
		r.RemoteAddr = from + ":1"
		return r
	}
	// Many addresses guess one name.
	for i := range acctFails {
		l.failed(req("198.51.100."+itoa(int64(i+1))), "owner")
	}
	if l.gate(req("198.51.100.200"), "owner") == 0 {
		t.Error("a new address should also have to wait for this account")
	}
	if l.gate(req(lan), "owner") != 0 {
		t.Error("the owner at home must not be locked out by strangers")
	}
	if l.gate(req("198.51.100.200"), "someone") != 0 {
		t.Error("other accounts are not affected")
	}
}

func TestLimiterIPv6Prefix(t *testing.T) {
	var l loginLimiter
	req := func(from string) *http.Request {
		r := httptest.NewRequest("POST", "/api/login", nil)
		r.RemoteAddr = "[" + from + "]:1"
		return r
	}
	for i := range pairFails {
		l.failed(req("2001:db8:1:1::"+itoa(int64(i+1))), "owner")
	}
	if l.gate(req("2001:db8:1:1::ffff"), "owner") == 0 {
		t.Error("another address in the same /64 is the same client")
	}
	if l.gate(req("2001:db8:1:2::1"), "someone") != 0 {
		t.Error("another network is another client")
	}
}

func TestLimiterStaysBounded(t *testing.T) {
	var l loginLimiter
	for i := range limiterSize + 100 {
		l.bump("x"+itoa(int64(i)), 5, time.Second, time.Second)
	}
	if n := len(l.m); n > limiterSize {
		t.Errorf("%d entries", n)
	}
}

func TestPlainHTTPClaimingHTTPSIsNotEnough(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	putRemote(t, h, tok, map[string]any{"enabled": true})
	if r := call(h, "POST", "/api/login", public, nil, "X-Forwarded-Proto: https"); r.code != 403 {
		t.Errorf("a direct visitor's own header: %d", r.code)
	}
	if r := call(h, "GET", "/api/status", public, nil, "TLS: yes"); r.code != 200 {
		t.Errorf("really encrypted: %d", r.code)
	}
	if r := call(h, "GET", "/", public, nil, "TLS: yes"); r.h.Get("Strict-Transport-Security") == "" {
		t.Error("HTTPS should stay on once used")
	}
	if r := call(h, "GET", "/", lan, nil); r.h.Get("Strict-Transport-Security") != "" {
		t.Error("never over plain HTTP")
	}
}

func TestSecureCookie(t *testing.T) {
	s, h := remoteServer(t, nil)
	if _, err := s.Auth.CreateUser(context.Background(), "Owner", pass, true); err != nil {
		t.Fatal(err)
	}
	login := map[string]string{"name": "Owner", "password": pass}
	cookie := func(r reply) *http.Cookie {
		res := http.Response{Header: r.h}
		for _, c := range res.Cookies() {
			if c.Name == auth.CookieName {
				return c
			}
		}
		t.Fatalf("no cookie: %d %s", r.code, r.body)
		return nil
	}
	c := cookie(call(h, "POST", "/api/login", lan, login))
	if c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("at home over HTTP: %+v", c)
	}
	c = cookie(call(h, "POST", "/api/login", lan, login, "X-Forwarded-Proto: https"))
	if !c.Secure || !c.HttpOnly {
		t.Errorf("over HTTPS: %+v", c)
	}
}

func TestPasswordFreeProfilesOnlyAtHome(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	if _, err := s.Auth.CreateProfile(context.Background(), "Kid", "", auth.KindKids, false, ""); err != nil {
		t.Fatal(err)
	}
	if r := call(h, "PUT", "/api/settings", lan, map[string]string{"profilesRemote": "1"}, "Authorization: Bearer "+tok); r.code != 200 {
		t.Fatal(r.code)
	}
	putRemote(t, h, tok, map[string]any{"enabled": true})
	tap := map[string]string{"name": "Kid", "password": ""}
	if r := call(h, "POST", "/api/login", lan, tap); r.code != 200 {
		t.Errorf("at home: %d %s", r.code, r.body)
	}
	if r := call(h, "POST", "/api/login", tail, tap); r.code != 401 {
		t.Errorf("away from home, even with profiles shown: %d %s", r.code, r.body)
	}
}

func TestStatusHidesVersionFromTheInternet(t *testing.T) {
	s, h := remoteServer(t, nil)
	s.Version = "9.9.9"
	tok := adminToken(t, s, h)
	putRemote(t, h, tok, map[string]any{"enabled": true})
	var st struct {
		Version string `json:"version"`
	}
	call(h, "GET", "/api/status", lan, nil).json(t, &st)
	if st.Version != "9.9.9" {
		t.Errorf("at home: %q", st.Version)
	}
	st.Version = ""
	call(h, "GET", "/api/status", public, nil, "TLS: yes").json(t, &st)
	if st.Version != "" {
		t.Errorf("internet, signed out: %q", st.Version)
	}
	call(h, "GET", "/api/status", public, nil, "TLS: yes", "Authorization: Bearer "+tok).json(t, &st)
	if st.Version != "9.9.9" {
		t.Errorf("internet, signed in: %q", st.Version)
	}
}

func TestCORS(t *testing.T) {
	_, h := remoteServer(t, nil)
	acao := func(r reply) string { return r.h.Get("Access-Control-Allow-Origin") }
	if got := acao(call(h, "GET", "/api/profiles", lan, nil, "Origin: http://evil.test")); got != "" {
		t.Errorf("a page elsewhere must not read the profiles: %q", got)
	}
	if got := acao(call(h, "GET", "/api/me", lan, nil, "Origin: http://tool.test", "Authorization: Bearer x")); got != "http://tool.test" {
		t.Errorf("a tool with a token: %q", got)
	}
	if got := acao(call(h, "OPTIONS", "/api/me", lan, nil, "Origin: http://tool.test", "Access-Control-Request-Headers: authorization")); got != "http://tool.test" {
		t.Errorf("preflight of a tool with a token: %q", got)
	}
	if got := acao(call(h, "OPTIONS", "/api/login", lan, nil, "Origin: http://evil.test", "Access-Control-Request-Headers: content-type")); got != "" {
		t.Errorf("preflight without a token: %q", got)
	}
	if got := acao(call(h, "GET", "/api/status", lan, nil, "Origin: capacitor://localhost")); got != "capacitor://localhost" {
		t.Errorf("the mobile app: %q", got)
	}
	r := call(h, "GET", "/api/status", lan, nil, "Origin: capacitor://localhost")
	if r.h.Get("Access-Control-Allow-Credentials") != "" {
		t.Error("cookies are never honoured across origins")
	}
}

func TestLoginCSRF(t *testing.T) {
	s, h := remoteServer(t, nil)
	if _, err := s.Auth.CreateUser(context.Background(), "Owner", pass, true); err != nil {
		t.Fatal(err)
	}
	body := map[string]string{"name": "Owner", "password": pass}
	if r := call(h, "POST", "/api/login", lan, body, "Origin: http://evil.test", "Sec-Fetch-Site: cross-site"); r.code != 403 {
		t.Errorf("another site signing a browser in: %d", r.code)
	}
	if r := call(h, "POST", "/api/login", lan, body, "Origin: capacitor://localhost", "Sec-Fetch-Site: cross-site"); r.code != 200 {
		t.Errorf("the mobile app: %d %s", r.code, r.body)
	}
}

func TestCSRFForwardedHostNeedsTrust(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	login := map[string]string{"name": "Owner", "password": pass}
	// Without trust a made-up X-Forwarded-Host doesn't make another origin the same.
	r := call(h, "POST", "/api/login", lan, login, "Origin: http://evil.test", "X-Forwarded-Host: evil.test")
	if r.code != 403 {
		t.Errorf("untrusted forwarded host: %d", r.code)
	}
	putRemote(t, h, tok, map[string]any{"enabled": true, "trustedProxies": "10.0.0.2"})
	r = call(h, "POST", "/api/login", "10.0.0.2", login, "Origin: https://media.example.com", "X-Forwarded-For: "+lan, "X-Forwarded-Host: media.example.com", "X-Forwarded-Proto: https")
	if r.code != 200 {
		t.Errorf("a trusted proxy's host: %d %s", r.code, r.body)
	}
	putRemote(t, h, tok, map[string]any{"publicUrl": "https://media.example.com"})
	if r := call(h, "POST", "/api/login", lan, login, "Origin: https://media.example.com"); r.code != 200 {
		t.Errorf("the server's own outside address: %d %s", r.code, r.body)
	}
}

func TestReachTest(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	hdr := "Authorization: Bearer " + tok
	if r := call(h, "POST", "/api/remote/test", lan, map[string]string{"address": "https://elsewhere.example"}, hdr); r.code != 400 {
		t.Errorf("only offered addresses may be tested: %d", r.code)
	}
	putRemote(t, h, tok, map[string]any{"enabled": true, "publicUrl": "https://media.example.com"})
	r := call(h, "POST", "/api/remote/test", lan, map[string]string{"address": "https://media.example.com"}, hdr)
	var tv testView
	r.json(t, &tv)
	if !strings.HasPrefix(tv.URL, "https://media.example.com/api/reach/") {
		t.Fatalf("test: %s", r.body)
	}
	path := strings.TrimPrefix(tv.URL, "https://media.example.com")

	if r := call(h, "GET", "/api/reach/00000000000000000000000000000000", public, nil, "TLS: yes"); r.code != 404 {
		t.Errorf("a wrong address: %d", r.code)
	}
	// From home: answered, but proves nothing.
	r = call(h, "GET", path, lan, nil)
	if r.code != 200 || !strings.Contains(r.body, "at home") || strings.Contains(r.body, "WittyMediaServer") {
		t.Errorf("from home: %d %s", r.code, r.body)
	}
	var v remoteView
	call(h, "GET", "/api/remote", lan, nil, hdr).json(t, &v)
	if v.Test == nil || v.Test.Hit == nil || v.Test.Hit.Proof || v.LastProof != nil {
		t.Errorf("a test from home is no proof: %+v", v.Test)
	}
	// From outside: proof, kept after the test ends.
	r = call(h, "GET", path, public, nil, "TLS: yes")
	if r.code != 200 || !strings.Contains(r.body, "It works") || r.h.Get("Content-Security-Policy") == "" {
		t.Errorf("from outside: %d %s", r.code, r.body)
	}
	call(h, "GET", "/api/remote", lan, nil, hdr).json(t, &v)
	if v.Test.Hit == nil || !v.Test.Hit.Proof || v.Test.Hit.Where != "internet" || v.LastProof == nil || v.State != "ok" {
		t.Errorf("after the outside test: %+v %+v state=%s", v.Test, v.LastProof, v.State)
	}
	// Starting a new test ends the old address.
	call(h, "POST", "/api/remote/test", lan, map[string]string{"address": "https://media.example.com"}, hdr)
	if r := call(h, "GET", path, lan, nil); r.code != 404 {
		t.Errorf("an old test address: %d", r.code)
	}
}

func TestWeakAdminWarning(t *testing.T) {
	s, h := remoteServer(t, nil)
	if _, err := s.Auth.CreateUser(context.Background(), "Owner", "12345678", true); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Token string `json:"token"`
	}
	call(h, "POST", "/api/login", lan, map[string]string{"name": "Owner", "password": "12345678"}).json(t, &out)
	warned := func(v remoteView) bool {
		for _, n := range v.Notices {
			if n.Level == "warn" && strings.Contains(n.Text, "weak password") {
				return true
			}
		}
		return false
	}
	var v remoteView
	call(h, "GET", "/api/remote", lan, nil, "Authorization: Bearer "+out.Token).json(t, &v)
	if warned(v) {
		t.Error("no warning while remote access is off")
	}
	v = putRemote(t, h, out.Token, map[string]any{"enabled": true})
	if !warned(v) {
		t.Errorf("a weak admin password should be warned about: %+v", v.Notices)
	}
}

func TestForcedOnByConfig(t *testing.T) {
	s, h := remoteServer(t, &config.Config{RemoteAccess: true, TrustedProxies: "10.0.0.2"})
	if r := call(h, "GET", "/api/status", "10.0.0.2", nil, "X-Forwarded-For: "+public, "X-Forwarded-Proto: https"); r.code != 200 {
		t.Errorf("config turns it on, and config trusts the proxy: %d", r.code)
	}
	if !s.state().forced {
		t.Error("the status should say it can't be turned off here")
	}
	_, h = remoteServer(t, &config.Config{PublicURL: "https://media.example.com"})
	if r := call(h, "GET", "/api/status", public, nil, "TLS: yes"); r.code != 200 {
		t.Errorf("an outside address in the config means remote access is wanted: %d", r.code)
	}
}

func TestSessionsAPI(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	hdr := "Authorization: Bearer " + tok
	r := call(h, "POST", "/api/login", lan, map[string]string{"name": "Owner", "password": pass, "device": "Mozilla/5.0 (Windows NT 10.0) Chrome/120 Safari/537"}, "User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")
	var second struct {
		Token string `json:"token"`
	}
	r.json(t, &second)
	var list []auth.Session
	call(h, "GET", "/api/sessions", lan, nil, hdr).json(t, &list)
	if len(list) != 2 {
		t.Fatalf("sessions: %+v", list)
	}
	raw := call(h, "GET", "/api/sessions", lan, nil, hdr).body
	if strings.Contains(raw, tok) || strings.Contains(raw, second.Token) {
		t.Fatal("tokens must never be listed")
	}
	var other auth.Session
	for _, v := range list {
		if !v.Current {
			other = v
		}
	}
	if other.Device != "Chrome on Windows" {
		t.Errorf("device = %q", other.Device)
	}
	if r := call(h, "DELETE", "/api/sessions/"+other.ID, lan, nil, hdr); r.code != 200 {
		t.Fatalf("revoke: %d %s", r.code, r.body)
	}
	if r := call(h, "GET", "/api/me", lan, nil, "Authorization: Bearer "+second.Token); r.code != 401 {
		t.Errorf("a revoked session still works: %d", r.code)
	}
	if r := call(h, "DELETE", "/api/sessions/nonsense", lan, nil, hdr); r.code != 404 {
		t.Errorf("unknown session: %d", r.code)
	}
}

func TestTwoFactorAPI(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	hdr := "Authorization: Bearer " + tok
	if r := call(h, "POST", "/api/me/2fa/setup", lan, map[string]string{"password": "wrong"}, hdr); r.code != 403 {
		t.Fatalf("setup needs the password: %d", r.code)
	}
	r := call(h, "POST", "/api/me/2fa/setup", lan, map[string]string{"password": pass}, hdr)
	var setup auth.TwoFactorSetup
	r.json(t, &setup)
	if setup.Secret == "" {
		t.Fatalf("setup: %s", r.body)
	}
	if r := call(h, "POST", "/api/me/2fa/confirm", lan, map[string]string{"code": "000000"}, hdr); r.code != 400 {
		t.Errorf("a wrong code: %d", r.code)
	}
	code := auth.CurrentCode(setup.Secret)
	r = call(h, "POST", "/api/me/2fa/confirm", lan, map[string]string{"code": code}, hdr)
	var conf struct {
		Recovery []string `json:"recovery"`
	}
	r.json(t, &conf)
	if len(conf.Recovery) == 0 {
		t.Fatalf("confirm: %d %s", r.code, r.body)
	}
	login := map[string]string{"name": "Owner", "password": pass}
	r = call(h, "POST", "/api/login", lan, login)
	var need struct {
		TwoFactor bool   `json:"twoFactor"`
		Token     string `json:"token"`
	}
	r.json(t, &need)
	if r.code != 401 || !need.TwoFactor || need.Token != "" {
		t.Errorf("password alone: %d %s", r.code, r.body)
	}
	r = call(h, "POST", "/api/login", lan, map[string]string{"name": "Owner", "password": "wrong"})
	need.TwoFactor = false
	r.json(t, &need)
	if need.TwoFactor {
		t.Errorf("a wrong password must not reveal the second step: %s", r.body)
	}
	login["code"] = conf.Recovery[0]
	if r := call(h, "POST", "/api/login", lan, login); r.code != 200 {
		t.Errorf("recovery code: %d %s", r.code, r.body)
	}
	// Turning it off needs both.
	if r := call(h, "POST", "/api/me/2fa/disable", lan, map[string]string{"password": pass, "code": "000000"}, hdr); r.code != 403 {
		t.Errorf("disable with a wrong code: %d", r.code)
	}
	if r := call(h, "POST", "/api/me/2fa/disable", lan, map[string]string{"password": pass, "code": conf.Recovery[1]}, hdr); r.code != 200 {
		t.Errorf("disable: %d %s", r.code, r.body)
	}
	if r := call(h, "POST", "/api/login", lan, map[string]string{"name": "Owner", "password": pass}); r.code != 200 {
		t.Errorf("after disabling: %d %s", r.code, r.body)
	}
}

func TestStreamCaps(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	putRemote(t, h, tok, map[string]any{"enabled": true, "maxHeight": 720, "maxBitrate": "4M"})
	req := func(from, q string) *http.Request {
		r := httptest.NewRequest("GET", "/api/items/1/play"+q, nil)
		r.RemoteAddr = from + ":1"
		return r.WithContext(context.WithValue(r.Context(), clientKey, s.state().policy.Resolve(r)))
	}
	// At home: nothing changes.
	q, height := s.quality(req(lan, ""))
	if height != 0 || q.CapRate != "" || s.capTag(req(lan, "")) != "" || !s.fitsCap(req(lan, ""), 1<<40, 60) {
		t.Errorf("at home: %+v %d", q, height)
	}
	// Away: limited, even when the player asks for more.
	q, height = s.quality(req(tail, "?maxHeight=2160"))
	if height != 720 {
		t.Errorf("height = %d", height)
	}
	if ffmpeg.ParseRate(q.CapRate) <= 0 || ffmpeg.ParseRate(q.CapRate) >= 4000000 {
		t.Errorf("video limit = %q", q.CapRate)
	}
	// A player asking for less keeps that.
	if _, height = s.quality(req(tail, "?maxHeight=480")); height != 480 {
		t.Errorf("a lower request: %d", height)
	}
	if s.capTag(req(tail, "")) == "" {
		t.Error("disc streams must be kept apart")
	}
	// A file over the limit may not be sent as it is.
	gb := int64(1) << 30
	if s.fitsCap(req(tail, ""), 4*gb, 3600) {
		t.Error("4 GB/hour is about 9.5 Mb/s")
	}
	if !s.fitsCap(req(tail, ""), gb, 3600) {
		t.Error("1 GB/hour is about 2.4 Mb/s")
	}
	w := httptest.NewRecorder()
	if s.capWriter(w, req(tail, "")) == http.ResponseWriter(w) {
		t.Error("what is sent away from home is throttled")
	}
	if s.capWriter(w, req(lan, "")) != http.ResponseWriter(w) {
		t.Error("at home it isn't")
	}
}

func TestOnlyRealConnectionsCountAsEvidence(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	v := putRemote(t, h, tok, map[string]any{"enabled": true, "publicUrl": "https://media.example.com"})
	if v.State != "waiting" {
		t.Fatalf("nothing seen yet: %s", v.State)
	}
	// Someone at home forging headers, and plain requests from the internet.
	call(h, "GET", "/api/status", lan, nil, "X-Forwarded-For: "+public, "X-Forwarded-Proto: https")
	call(h, "GET", "/api/status", public, nil)
	var got remoteView
	call(h, "GET", "/api/remote", lan, nil, "Authorization: Bearer "+tok).json(t, &got)
	if got.State != "waiting" {
		t.Errorf("forged or plain requests must not say it works: %s", got.State)
	}
	call(h, "GET", "/api/status", public, nil, "TLS: yes")
	call(h, "GET", "/api/remote", lan, nil, "Authorization: Bearer "+tok).json(t, &got)
	if got.State != "ok" {
		t.Errorf("a real encrypted visit from the internet: %s", got.State)
	}
}

func TestProofEndsWithTheWayIn(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	hdr := "Authorization: Bearer " + tok
	putRemote(t, h, tok, map[string]any{"enabled": true, "method": "proxy", "publicUrl": "https://media.example.com"})
	var tv testView
	call(h, "POST", "/api/remote/test", lan, map[string]string{"address": "https://media.example.com"}, hdr).json(t, &tv)
	call(h, "GET", strings.TrimPrefix(tv.URL, "https://media.example.com"), public, nil, "TLS: yes")
	var v remoteView
	call(h, "GET", "/api/remote", lan, nil, hdr).json(t, &v)
	if v.LastProof == nil {
		t.Fatal("no proof recorded")
	}
	if got := putRemote(t, h, tok, map[string]any{"method": "tailscale"}); got.LastProof != nil {
		t.Error("the proof was for the old way in")
	}
}

func TestProfilesAwayFromHomeHideWhoHasNoPassword(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	if _, err := s.Auth.CreateProfile(context.Background(), "Kid", "", auth.KindKids, false, ""); err != nil {
		t.Fatal(err)
	}
	call(h, "PUT", "/api/settings", lan, map[string]string{"profilesRemote": "1"}, "Authorization: Bearer "+tok)
	putRemote(t, h, tok, map[string]any{"enabled": true})
	type list struct {
		Profiles []struct {
			Name        string `json:"name"`
			HasPassword bool   `json:"hasPassword"`
		} `json:"profiles"`
	}
	var home, away list
	call(h, "GET", "/api/profiles", lan, nil).json(t, &home)
	call(h, "GET", "/api/profiles", tail, nil).json(t, &away)
	for _, p := range home.Profiles {
		if p.Name == "Kid" && p.HasPassword {
			t.Error("at home the tap works")
		}
	}
	for _, p := range away.Profiles {
		if !p.HasPassword {
			t.Errorf("%s: away from home every profile asks for a password", p.Name)
		}
	}
}

func TestLoginSlotIsHeldUntilDone(t *testing.T) {
	s, h := remoteServer(t, nil)
	_ = h
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = lan + ":1"
	var held []func()
	for range cap(s.loginSlots) {
		release, ok := s.loginAllowed(httptest.NewRecorder(), r, "x")
		if !ok {
			t.Fatal("a place should be free")
		}
		held = append(held, release)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := s.loginAllowed(httptest.NewRecorder(), r.WithContext(ctx), "x"); ok {
		t.Error("all places are taken")
	}
	held[0]()
	if release, ok := s.loginAllowed(httptest.NewRecorder(), r, "x"); !ok {
		t.Error("a released place is free again")
	} else {
		release()
	}
	for _, f := range held[1:] {
		f()
	}
}
