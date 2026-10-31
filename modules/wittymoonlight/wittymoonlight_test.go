package wittymoonlight

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func target() Target {
	return Target{RigName: "Living Room", Address: "192.168.1.20", UUID: "abc", AppID: 7, AppName: "Space Game"}
}

func kinds(ms []Method) string {
	var k []string
	for _, m := range ms {
		k = append(k, m.Kind)
	}
	return strings.Join(k, ",")
}

func TestPlanPerDevice(t *testing.T) {
	for _, c := range []struct {
		d      Device
		bridge string
		want   string
	}{
		{Device{"desktop", "windows"}, "", "command,steps"},
		{Device{"desktop", "linux"}, "https://x/", "bridge,command,steps"},
		{Device{"browser", "windows"}, "", "command,steps"},
		{Device{"browser", "mac"}, "https://x/", "bridge,command,steps"},
		{Device{"browser", "android"}, "", "steps"},
		{Device{"browser", "ios"}, "https://x/", "bridge,steps"},
		{Device{"android", "android"}, "", "intent,steps"},
		{Device{"android", "androidtv"}, "", "intent,steps"},
		{Device{"android", "firetv"}, "", "intent,steps"},
		{Device{"apple", "ios"}, "", "steps"},
		{Device{"apple", "tvos"}, "", "steps"},
	} {
		if got := kinds(Plan(c.d, target(), c.bridge, "")); got != c.want {
			t.Errorf("%+v: %s, want %s", c.d, got, c.want)
		}
	}
	ms := Plan(Device{"android", "androidtv"}, target(), "", "")
	if ms[0].Package != "com.limelight" || ms[0].Activity != "com.limelight.ShortcutTrampoline" || ms[0].Extras["UUID"] != "abc" || ms[0].Extras["AppId"] != "7" {
		t.Errorf("%+v", ms[0])
	}
	away := target()
	away.Address, away.Away = "", true
	if got := kinds(Plan(Device{"desktop", "windows"}, away, "", "")); got != "" {
		t.Errorf("away with nothing to connect to: %s", got)
	}
	if got := kinds(Plan(Device{"browser", "windows"}, away, "https://x/", "")); got != "bridge" {
		t.Errorf("away with the bridge: %s", got)
	}
}

func TestHostileAppNames(t *testing.T) {
	for _, name := range []string{"--quit-after", "-x", "a\nb", ""} {
		tg := target()
		tg.AppName = name
		if MoonlightArgs(tg) != nil {
			t.Errorf("%q would go on a command line", name)
		}
		if got := kinds(Plan(Device{"desktop", "windows"}, tg, "", "")); strings.Contains(got, "command") {
			t.Errorf("%q: %s", name, got)
		}
	}
	tg := target()
	tg.AppName = `It's; rm -rf / $(x) "y"`
	ms := Plan(Device{"browser", "linux"}, tg, "", "")
	want := `moonlight 'stream' '192.168.1.20' 'It''s; rm -rf / $(x) "y"'`
	if ms[0].Text != want {
		t.Errorf("command line: %s", ms[0].Text)
	}
	tg = target()
	tg.Port = 48000
	if a := MoonlightArgs(tg); a[1] != "192.168.1.20:48000" {
		t.Errorf("%v", a)
	}
	tg.Address = "fd00::1"
	if a := MoonlightArgs(tg); a[1] != "[fd00::1]:48000" {
		t.Errorf("%v", a)
	}
	if PairArgs("-x", 0, "1234") != nil || PairArgs("pc", 0, "12") != nil {
		t.Error("bad pair arguments")
	}
}

func TestInstallLinksAreTheProjects(t *testing.T) {
	for _, os := range []string{"windows", "mac", "linux", "android", "androidtv", "firetv", "ios", "tvos", "chromeos"} {
		i := InstallFor(os)
		if i == nil || i.License != "GPL-3.0" || !strings.HasPrefix(i.Source, "https://github.com/moonlight-stream/") {
			t.Errorf("%s: %+v", os, i)
		}
	}
	if InstallFor("toaster") != nil {
		t.Error("toaster")
	}
}

// A stand-in for the bridge: it records what reaches it.
type upstream struct {
	srv  *httptest.Server
	seen []*http.Request
	body []string
}

func newUpstream(t *testing.T) *upstream {
	u := &upstream{}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		u.seen = append(u.seen, r)
		u.body = append(u.body, string(b))
		w.Header().Set("Set-Cookie", "mlSession=secret")
		_, _ = w.Write([]byte("from bridge"))
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *upstream) port() int {
	_, p, _ := net.SplitHostPort(u.srv.Listener.Addr().String())
	n, _ := strconv.Atoi(p)
	return n
}

type client struct {
	t      *testing.T
	base   string
	cookie *http.Cookie
}

func (c *client) do(method, path, body string, hdr ...string) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest(method, c.base+path, strings.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	tr := http.Transport{}
	res, err := tr.RoundTrip(req)
	if err != nil {
		c.t.Fatal(err)
	}
	c.t.Cleanup(func() { res.Body.Close() })
	return res
}

func proxied(t *testing.T, hooks Hooks) (*Proxy, *upstream, *client) {
	u := newUpstream(t)
	p := NewProxy(u.port, hooks)
	srv := httptest.NewServer(p)
	t.Cleanup(srv.Close)
	c := &client{t: t, base: srv.URL}
	ticket, err := p.NewTicket(Session{User: 5, RigID: 1, AppID: 7, HostID: "42"})
	if err != nil {
		t.Fatal(err)
	}
	res := c.do("GET", "/wms-ticket/"+ticket, "")
	if res.StatusCode != http.StatusFound || !strings.HasPrefix(res.Header.Get("Location"), "/stream.html?") {
		t.Fatalf("redeem: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	loc, _ := url.Parse(res.Header.Get("Location"))
	if loc.Query().Get("hostId") != "42" || loc.Query().Get("appId") != "7" || loc.Query().Get("dataTransport") != "webrtc" {
		t.Errorf("redirect %s", loc)
	}
	if len(res.Cookies()) != 1 || !res.Cookies()[0].HttpOnly || res.Cookies()[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie %+v", res.Cookies())
	}
	c.cookie = res.Cookies()[0]
	// A ticket works once.
	again := &client{t: t, base: srv.URL}
	if res := again.do("GET", "/wms-ticket/"+ticket, ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("a ticket was used twice: %d", res.StatusCode)
	}
	return p, u, c
}

const goodOffer = "v=0\r\na=x-moonlight-host-id:42\r\na=x-moonlight-app-id:7\r\na=x-moonlight-mode:1920x1080x60\r\n"

func TestProxyRefusesWithoutASession(t *testing.T) {
	u := newUpstream(t)
	srv := httptest.NewServer(NewProxy(u.port, Hooks{}))
	defer srv.Close()
	c := &client{t: t, base: srv.URL}
	for _, p := range []string{"/", "/stream.html", "/api/hosts", "/api/login", "/wms-ticket/nonsense"} {
		if res := c.do("GET", p, ""); res.StatusCode != http.StatusForbidden {
			t.Errorf("%s: %d", p, res.StatusCode)
		}
	}
	c.cookie = &http.Cookie{Name: cookieName, Value: "guess"}
	if res := c.do("GET", "/stream.html", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("a made-up cookie: %d", res.StatusCode)
	}
	if len(u.seen) != 0 {
		t.Errorf("%d requests reached the bridge", len(u.seen))
	}
}

func TestProxyOnlyPassesTheStream(t *testing.T) {
	_, u, c := proxied(t, Hooks{})
	// What the stream page needs.
	for _, r := range [][2]string{{"GET", "/stream.html"}, {"GET", "/assets/a.js"}, {"GET", "/api/authenticate"}, {"GET", "/api/settings/permissions"},
		{"GET", "/api/user"}, {"GET", "/api/host?host_id=42"}, {"GET", "/api/apps?host_id=42"}, {"OPTIONS", "/api/host/stream/webrtc"},
		{"PATCH", "/api/host/stream/webrtc/abc"}, {"DELETE", "/api/host/stream/webrtc/abc"}} {
		if res := c.do(r[0], r[1], ""); res.StatusCode != 200 {
			t.Errorf("%v: %d", r, res.StatusCode)
		}
	}
	n := len(u.seen)
	// What it doesn't.
	for _, r := range [][2]string{{"GET", "/api/hosts"}, {"GET", "/api/host?host_id=43"}, {"GET", "/api/apps?host_id=43"}, {"GET", "/api/user?name=admin"},
		{"GET", "/api/users"}, {"GET", "/api/roles"}, {"POST", "/api/host"}, {"POST", "/api/pair"}, {"POST", "/api/login"}, {"PATCH", "/api/user"},
		{"PATCH", "/api/role"}, {"DELETE", "/api/host?host_id=42"}, {"POST", "/api/host/cancel"}, {"POST", "/api/host/wake"},
		{"GET", "/api/host/stream/web_socket"}, {"POST", "/api/host/stream/webrtc/abc/def"}, {"PATCH", "/api/host"}} {
		if res := c.do(r[0], r[1], ""); res.StatusCode != http.StatusForbidden {
			t.Errorf("%v: %d", r, res.StatusCode)
		}
	}
	// The bridge's other pages show a page of ours instead.
	for _, p := range []string{"/", "/index.html", "/admin.html"} {
		if res := c.do("GET", p, ""); res.StatusCode != 200 {
			t.Errorf("%s: %d", p, res.StatusCode)
		}
	}
	if len(u.seen) != n {
		t.Errorf("%d refused requests reached the bridge", len(u.seen)-n)
	}
}

func TestProxyChecksTheOffer(t *testing.T) {
	var started int
	var allow error
	_, u, c := proxied(t, Hooks{
		MayStart: func(_ context.Context, s Session) error { return allow },
		Started:  func(Session) { started++ },
	})
	post := func(body string) int {
		return c.do("POST", "/api/host/stream/webrtc", body, "Content-Type", "application/sdp").StatusCode
	}
	if code := post(goodOffer); code != 200 {
		t.Errorf("good offer: %d", code)
	}
	if u.body[len(u.body)-1] != goodOffer {
		t.Errorf("the offer changed on the way: %q", u.body[len(u.body)-1])
	}
	if started != 1 {
		t.Errorf("started %d", started)
	}
	n := len(u.seen)
	for name, offer := range map[string]string{
		"another game": strings.Replace(goodOffer, "app-id:7", "app-id:881448767", 1),
		"another rig":  strings.Replace(goodOffer, "host-id:42", "host-id:43", 1),
		"two games":    goodOffer + "a=x-moonlight-app-id:881448767\r\n",
		"two rigs":     "a=x-moonlight-host-id:43\r\n" + goodOffer,
		"no game":      "v=0\r\na=x-moonlight-host-id:42\r\n",
		"no rig":       "v=0\r\na=x-moonlight-app-id:7\r\n",
		"other case":   goodOffer + "A=X-MOONLIGHT-APP-ID:881448767\r\n",
		"lone CR":      "a=x-moonlight-host-id:42\ra=x-moonlight-app-id:7\ra=x-moonlight-app-id:1\r",
		"spaces":       goodOffer + "  a=x-moonlight-app-id:881448767\r\n",
		"huge":         goodOffer + strings.Repeat("x", maxOffer),
		"empty":        "",
		"no value":     "a=x-moonlight-host-id:42\r\na=x-moonlight-app-id\r\n",
	} {
		if code := post(offer); code != http.StatusConflict {
			t.Errorf("%s: %d", name, code)
		}
	}
	allow = errors.New("someone else is playing")
	if code := post(goodOffer); code != http.StatusConflict {
		t.Errorf("hook refused: %d", code)
	}
	if len(u.seen) != n {
		t.Errorf("%d refused offers reached the bridge", len(u.seen)-n)
	}
}

func TestProxyStripsWhatIdentifiesPeople(t *testing.T) {
	_, u, c := proxied(t, Hooks{})
	res := c.do("GET", "/api/authenticate", "", "Authorization", "Bearer stolen", BridgeUserHeader, "wms-bridge-admin", "X-Forwarded-For", "1.2.3.4", "X-Forwarded-User", "root")
	if res.Header.Get("Set-Cookie") != "" {
		t.Error("the bridge's cookie reached the browser")
	}
	if !strings.Contains(res.Header.Get("Content-Security-Policy"), "wasm-unsafe-eval") {
		t.Error("no CSP")
	}
	got := u.seen[len(u.seen)-1].Header
	if got.Get(BridgeUserHeader) != "wms-u5" {
		t.Errorf("user header %q", got.Get(BridgeUserHeader))
	}
	if got.Get("Authorization") != "" || got.Get("Cookie") != "" || got.Get("X-Forwarded-For") != "" || got.Get("X-Forwarded-User") != "" {
		t.Errorf("leaked: %v", got)
	}
}

func TestProxyChecksOrigin(t *testing.T) {
	_, u, c := proxied(t, Hooks{})
	n := len(u.seen)
	if res := c.do("POST", "/api/host/stream/webrtc", goodOffer, "Origin", "https://evil.example"); res.StatusCode != http.StatusForbidden {
		t.Errorf("another site: %d", res.StatusCode)
	}
	if len(u.seen) != n {
		t.Error("it reached the bridge")
	}
	if res := c.do("POST", "/api/host/stream/webrtc", goodOffer, "Origin", c.base); res.StatusCode != 200 {
		t.Errorf("same site: %d", res.StatusCode)
	}
}

func TestProxyRevoke(t *testing.T) {
	p, _, c := proxied(t, Hooks{})
	p.RevokeUser(99)
	if res := c.do("GET", "/stream.html", ""); res.StatusCode != 200 {
		t.Errorf("someone else's revocation: %d", res.StatusCode)
	}
	p.RevokeUser(5)
	if res := c.do("GET", "/stream.html", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("after revoking: %d", res.StatusCode)
	}
	ticket, _ := p.NewTicket(Session{User: 5, RigID: 1, AppID: 7, HostID: "42"})
	p.RevokeRig(1)
	if res := c.do("GET", "/wms-ticket/"+ticket, ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("a ticket for a removed rig: %d", res.StatusCode)
	}
}

func TestSecondTicketReplacesTheFirst(t *testing.T) {
	p, _, c := proxied(t, Hooks{})
	ticket, _ := p.NewTicket(Session{User: 5, RigID: 1, AppID: 8, HostID: "42"})
	second := &client{t: t, base: c.base}
	res := second.do("GET", "/wms-ticket/"+ticket, "")
	second.cookie = res.Cookies()[0]
	if res := c.do("GET", "/stream.html", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("an old page still works: %d", res.StatusCode)
	}
	if res := second.do("GET", "/stream.html", ""); res.StatusCode != 200 {
		t.Errorf("the new page: %d", res.StatusCode)
	}
}

func tarGz(t *testing.T, files map[string]string, link bool) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	}
	if link {
		_ = tw.WriteHeader(&tar.Header{Name: "package/ln", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func asset(data []byte, name string) bridgeAsset {
	s := sha256.Sum256(data)
	return bridgeAsset{name, hex.EncodeToString(s[:]), int64(len(data))}
}

func TestInstallChecksEverything(t *testing.T) {
	data := tarGz(t, map[string]string{"package/web-server": "#!", "package/web-server.exe": "#!", "package/static/index.html": "x"}, false)
	b := &Bridge{Dir: filepath.Join(t.TempDir(), "bridge")}
	if err := b.installFrom(data, asset(data, "a.tar.gz")); err != nil {
		t.Fatal(err)
	}
	if !b.Installed() {
		t.Error("not installed")
	}
	bad := asset(data, "a.tar.gz")
	bad.sha256 = strings.Repeat("0", 64)
	if err := (&Bridge{Dir: t.TempDir()}).installFrom(data, bad); err == nil {
		t.Error("a wrong digest was accepted")
	}
	bad = asset(data, "a.tar.gz")
	bad.size++
	if err := (&Bridge{Dir: t.TempDir()}).installFrom(data, bad); err == nil {
		t.Error("a wrong size was accepted")
	}
	for name, d := range map[string][]byte{
		"climbs out": tarGz(t, map[string]string{"../../evil": "x"}, false),
		"absolute":   tarGz(t, map[string]string{"/tmp/evil": "x"}, false),
		"has a link": tarGz(t, map[string]string{"package/a": "x"}, true),
	} {
		dir := t.TempDir()
		if err := (&Bridge{Dir: filepath.Join(dir, "b")}).installFrom(d, asset(d, "a.tar.gz")); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if _, err := os.Stat(filepath.Join(dir, "evil")); err == nil {
			t.Errorf("%s: wrote outside", name)
		}
	}
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	w, _ := zw.Create("../evil")
	_, _ = w.Write([]byte("x"))
	zw.Close()
	if err := (&Bridge{Dir: filepath.Join(t.TempDir(), "b")}).installFrom(zbuf.Bytes(), asset(zbuf.Bytes(), "a.zip")); err == nil {
		t.Error("a zip climbed out")
	}
}

func TestEveryPublishedBuildHasADigest(t *testing.T) {
	for k, a := range bridgeAssets {
		if len(a.sha256) != 64 || a.size < 1<<20 || a.size > bridgeMaxSize {
			t.Errorf("%s: %+v", k, a)
		}
	}
	if !BridgeAvailable("windows", "amd64") || BridgeAvailable("freebsd", "amd64") || BridgeAvailable("linux", "arm") {
		t.Error("availability")
	}
}

func TestConfigKeepsItOffTheNetwork(t *testing.T) {
	b := &Bridge{Dir: t.TempDir(), UDPMin: 40000, UDPMax: 40010}
	raw, err := b.config(1234)
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Web struct {
			Bind      string `json:"bind_address"`
			Create    *bool  `json:"first_login_create_admin"`
			Assign    *bool  `json:"first_login_assign_global_hosts"`
			Forwarded struct {
				Header string `json:"username_header"`
			} `json:"forwarded_header"`
		} `json:"web_server"`
		WebRTC struct {
			Ice   []any `json:"ice_servers"`
			Range struct {
				Min int `json:"min"`
			} `json:"port_range"`
		} `json:"webrtc"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if c.Web.Bind != "127.0.0.1:1234" {
		t.Errorf("it would listen on %q", c.Web.Bind)
	}
	if c.Web.Create == nil || c.Web.Assign == nil {
		t.Error("required settings are missing, which stops it starting")
	}
	if c.Web.Forwarded.Header != BridgeUserHeader {
		t.Errorf("%q", c.Web.Forwarded.Header)
	}
	if c.WebRTC.Ice == nil || len(c.WebRTC.Ice) != 0 {
		t.Errorf("STUN servers: %v", c.WebRTC.Ice)
	}
	if c.WebRTC.Range.Min != 40000 {
		t.Errorf("%+v", c.WebRTC.Range)
	}
}
