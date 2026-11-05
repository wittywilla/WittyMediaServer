package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/livetv"
)

// logoChannels adds an IPTV tuner with one channel per logo address, numbered from 1.
func logoChannels(t *testing.T, s *Server, logos ...string) {
	t.Helper()
	s.Cfg.ImagesDir = t.TempDir()
	if _, err := s.DB.Exec(`INSERT INTO tuners(id, driver, name, options, created_at) VALUES(1, 'iptv', 'Test', '{}', 1)`); err != nil {
		t.Fatal(err)
	}
	for i, logo := range logos {
		id := i + 1
		if _, err := s.DB.Exec(`INSERT INTO tuner_channels(id, tuner_id, source_id, number, name, logo) VALUES(?, 1, ?, ?, ?, ?)`,
			id, fmt.Sprint("s", id), fmt.Sprint(id), fmt.Sprint("Channel ", id), logo); err != nil {
			t.Fatal(err)
		}
	}
}

func logoPicture(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// A playlist decides what a logo's address is. It must not be able to aim the
// server at this machine or at devices on the home network, even by a name
// that leads there.
func TestTunerLogoRefusesPrivateAddresses(t *testing.T) {
	h, admin, _, s := tvServer(t)
	var hits atomic.Int32
	device := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write(logoPicture(t))
	}))
	defer device.Close()
	port := device.URL[strings.LastIndex(device.URL, ":"):]
	logos := []string{
		device.URL + "/logo.png",
		"http://localhost" + port + "/logo.png",
		"http://[::1]" + port + "/logo.png",
		"http://192.168.1.1/logo.png",
		"http://10.0.0.1/logo.png",
		"http://169.254.169.254/latest/meta-data",
	}
	logoChannels(t, s, logos...)
	for i, logo := range logos {
		w := tvCall(h, "GET", fmt.Sprintf("/api/tv/tun-%d/logo", i+1), admin, "")
		if w.Code != 502 {
			t.Errorf("%s: %d %s", logo, w.Code, w.Body)
		}
		if strings.Contains(w.Body.String(), "logo.png") || strings.Contains(w.Body.String(), "169.254") {
			t.Errorf("the refusal gives the address away: %s", w.Body)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the device on this machine was reached %d times", n)
	}
}

// Only a web address is ever used: not a cache name, a path, or one from the request.
func TestTunerLogoOnlyServesThePlaylistsAddress(t *testing.T) {
	h, admin, _, s := tvServer(t)
	var hits atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/one.png" {
			t.Errorf("fetched %s", r.URL.Path)
		}
		if r.Header.Get("User-Agent") != "WittyMediaServer" { // some logo sites refuse the default one
			t.Errorf("User-Agent %q", r.Header.Get("User-Agent"))
		}
		_, _ = w.Write(logoPicture(t))
	}))
	defer remote.Close()
	saved := library.SafeClient
	library.SafeClient = remote.Client() // the test server is on this machine, which the real client refuses
	defer func() { library.SafeClient = saved }()
	logoChannels(t, s, remote.URL+"/one.png", "cache:secret.jpg", "C:/Users/someone/picture.png", "/etc/passwd", "file:///etc/passwd", "ftp://logo.example/x.png", "")

	for i := 2; i <= 7; i++ {
		if w := tvCall(h, "GET", fmt.Sprintf("/api/tv/tun-%d/logo", i), admin, ""); w.Code != 404 {
			t.Errorf("channel %d: %d", i, w.Code)
		}
	}
	for _, p := range []string{"/api/tv/lib-1/logo", "/api/tv/nope/logo", "/api/tv/tun-0/logo", "/api/tv/tun-99/logo"} {
		if w := tvCall(h, "GET", p, admin, ""); w.Code != 404 {
			t.Errorf("%s: %d", p, w.Code)
		}
	}

	// Asking for another address changes nothing, and the second time comes from the cache.
	for _, q := range []string{"", "?url=" + remote.URL + "/two.png", "?src=" + remote.URL + "/two.png"} {
		w := tvCall(h, "GET", "/api/tv/tun-1/logo"+q, admin, "")
		if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
			t.Fatalf("%q: %d %v", q, w.Code, w.Header())
		}
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("fetched %d times", n)
	}
}

// An SVG can hold scripts, and a site can call anything a PNG.
func TestTunerLogoMustBeAPicture(t *testing.T) {
	h, admin, _, s := tvServer(t)
	var hits atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png") // whatever it says
		_, _ = w.Write([]byte("<svg xmlns='http://www.w3.org/2000/svg'><script>alert(1)</script></svg>"))
	}))
	defer remote.Close()
	saved := library.SafeClient
	library.SafeClient = remote.Client()
	defer func() { library.SafeClient = saved }()
	logoChannels(t, s, remote.URL+"/page.png")
	for range 3 {
		if w := tvCall(h, "GET", "/api/tv/tun-1/logo", admin, ""); w.Code != 502 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	// One failure is remembered, so a broken logo isn't fetched on every look at the guide.
	if n := hits.Load(); n != 1 {
		t.Errorf("fetched %d times", n)
	}
}

// A site that doesn't answer is left alone for a while, whichever of its logos
// comes next: each try would hold one of the browser's connections to this server.
func TestTunerLogoRemembersASiteThatIsDown(t *testing.T) {
	h, admin, _, s := tvServer(t)
	logoChannels(t, s, "http://192.168.1.1/one.png", "http://192.168.1.1/two.png")
	if w := tvCall(h, "GET", "/api/tv/tun-1/logo", admin, ""); w.Code != 502 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !s.logoMissed("192.168.1.1") {
		t.Error("the site wasn't remembered")
	}
	if w := tvCall(h, "GET", "/api/tv/tun-2/logo", admin, ""); w.Code != 502 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

// Whoever may watch a channel may load its logo, and nobody else.
func TestTunerLogoFollowsWhoMayWatch(t *testing.T) {
	h, admin, kid, s := tvServer(t)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(logoPicture(t)) }))
	defer remote.Close()
	saved := library.SafeClient
	library.SafeClient = remote.Client()
	defer func() { library.SafeClient = saved }()
	logoChannels(t, s, remote.URL+"/one.png", remote.URL+"/off.png")
	if _, err := s.DB.Exec(`UPDATE tuner_channels SET enabled = 0 WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := s.Auth.CreateUser(ctx, "alex", "correct horse", false); err != nil {
		t.Fatal(err)
	}
	adult, _, err := s.Auth.Login(ctx, "alex", "correct horse", "t", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, token, path string
		want              int
	}{
		{"admin", admin, "/api/tv/tun-1/logo", 200},
		{"adult", adult, "/api/tv/tun-1/logo", 200},
		{"kids profile", kid, "/api/tv/tun-1/logo", 404},
		{"signed out", "", "/api/tv/tun-1/logo", 401},
		{"adult, a channel that is off", adult, "/api/tv/tun-2/logo", 404},
	} {
		if w := tvCall(h, "GET", c.path, c.token, ""); w.Code != c.want {
			t.Errorf("%s: %d, want %d", c.name, w.Code, c.want)
		}
	}
}

// The address of a logo reaches no client, in any answer about the channel.
func TestLogoAddressNeverReachesClients(t *testing.T) {
	h, admin, _, s := tvServer(t)
	logoChannels(t, s, "https://logo.example/one.png?login=secret")

	w := tvCall(h, "GET", "/api/guide", admin, "")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var g struct {
		Channels []struct {
			Key  string `json:"key"`
			Logo string `json:"logo"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Channels) != 1 || !strings.HasPrefix(g.Channels[0].Logo, "/api/tv/tun-1/logo?v=") {
		t.Fatalf("%+v", g.Channels)
	}
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/api/guide", ""},
		{"GET", "/api/guide?list=1", ""},
		{"GET", "/api/tuners/1/channels", ""},
		{"PUT", "/api/tuner-channels/1", `{"number":"7"}`},
	} {
		w := tvCall(h, c.method, c.path, admin, c.body)
		if w.Code != 200 {
			t.Errorf("%s %s: %d %s", c.method, c.path, w.Code, w.Body)
		}
		if strings.Contains(w.Body.String(), "logo.example") || strings.Contains(w.Body.String(), "secret") {
			t.Errorf("%s %s gives the address away: %s", c.method, c.path, w.Body)
		}
	}
}

// A tuner channel's logo: what a person chose first, then what its list names, then a match on its name.
func TestTunerChannelLogo(t *testing.T) {
	c := &livetv.Channel{ID: 7, Name: "NBC HD"}
	if u, b := tunerLogo(c); u != "" || b == nil || b.Kind != library.LogoNetwork || b.Name != "NBC" {
		t.Errorf("found by name: %q %+v", u, b)
	}
	c.Logo = "https://logos.example/nbc.png"
	if u, b := tunerLogo(c); u == "" || b != nil {
		t.Errorf("the list's own comes before a match: %q %+v", u, b)
	}
	c.LogoSet = "studio|Open Pictures"
	if u, b := tunerLogo(c); u != "" || b == nil || b.Kind != "studio" || b.Name != "Open Pictures" {
		t.Errorf("a person's correction comes first: %q %+v", u, b)
	}
	c.LogoSet = "none"
	if u, b := tunerLogo(c); u != "" || b != nil {
		t.Errorf("none means none, even with the list's: %q %+v", u, b)
	}
	c = &livetv.Channel{Name: "Movies HD"}
	if _, b := tunerLogo(c); b != nil {
		t.Errorf("a word like that would only ever match the wrong thing: %+v", b)
	}
	c = &livetv.Channel{Name: "Open Radio", Radio: true}
	if _, b := tunerLogo(c); b != nil {
		t.Errorf("radio isn't a TV network: %+v", b)
	}
	for in, want := range map[string]string{"": "", " none ": "none", "network|NBC": "network|NBC", "studio| Open Pictures ": "studio|Open Pictures"} {
		if got, err := checkTunerLogoChoice(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	for _, in := range []string{"NBC", "evil|NBC", "network|", "network|---", "network|" + strings.Repeat("x", 121)} {
		if _, err := checkTunerLogoChoice(in); err == nil {
			t.Errorf("%q should be refused", in)
		}
	}
}

// A person can put a channel's logo right through the API, a list's address stays hidden, and the
// credits say where every picture on a screen came from, for whoever can see it.
func TestTunerChannelLogoChoiceAndCredits(t *testing.T) {
	h, admin, kid, s := tvServer(t)
	logoChannels(t, s, "https://logos.example/secret.png", "")
	if _, err := s.DB.Exec(`UPDATE tuner_channels SET name = 'NBC HD', enabled = 1 WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE tuner_channels SET enabled = 1 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	row := func(id int) map[string]any {
		var list []map[string]any
		w := tvCall(h, "GET", "/api/tuners/1/channels", admin, "")
		if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(w.Body.String(), "secret.png") {
			t.Errorf("the list's address reached a client: %s", w.Body)
		}
		for _, c := range list {
			if int(c["id"].(float64)) == id {
				return c
			}
		}
		t.Fatalf("no channel %d", id)
		return nil
	}
	if b, _ := row(2)["brand"].(map[string]any); b["name"] != "NBC" || b["kind"] != "network" {
		t.Errorf("found by name: %v", row(2))
	}
	if u, _ := row(1)["logoUrl"].(string); !strings.HasPrefix(u, "/api/tv/tun-1/logo") {
		t.Errorf("the list's logo comes through this server: %v", row(1))
	}
	put := func(id int, logo string) int {
		return tvCall(h, "PUT", fmt.Sprintf("/api/tuner-channels/%d", id), admin, fmt.Sprintf(`{"logo":%q}`, logo)).Code
	}
	if c := put(2, "evil|x"); c != 400 {
		t.Errorf("a kind that isn't one: %d", c)
	}
	if c := put(2, "studio|Open Pictures"); c != 200 {
		t.Errorf("a person's correction: %d", c)
	}
	if b, _ := row(2)["brand"].(map[string]any); b["name"] != "Open Pictures" || b["kind"] != "studio" {
		t.Errorf("the correction: %v", row(2))
	}
	if g, err := s.Store.LogoFor(context.Background(), "studio", "Open Pictures"); err != nil || g == nil {
		t.Errorf("the service should know the name someone gave: %v %v", g, err)
	}
	if c := put(1, "none"); c != 200 {
		t.Errorf("none: %d", c)
	}
	if r := row(1); r["logoUrl"] != nil || r["brand"] != nil {
		t.Errorf("none means none: %v", r)
	}
	if c := put(1, ""); c != 200 {
		t.Errorf("back to automatic: %d", c)
	}
	if w := tvCall(h, "PUT", "/api/tuner-channels/2", kid, `{"logo":"none"}`); w.Code != 403 {
		t.Errorf("only an administrator: %d", w.Code)
	}
	// A guide row and the credits say the same thing, and nothing about the list's address.
	w := tvCall(h, "GET", "/api/logos/credits", admin, "")
	if !strings.Contains(w.Body.String(), `"name":"Channel 1","source":"list"`) || strings.Contains(w.Body.String(), "secret.png") {
		t.Errorf("credits: %s", w.Body)
	}
	if w := tvCall(h, "GET", "/api/logos", kid, ""); w.Code != 403 {
		t.Errorf("the whole list is for an administrator: %d", w.Code)
	}
	if w := tvCall(h, "GET", "/api/logos?kind=nonsense", admin, ""); w.Code != 400 {
		t.Errorf("a kind that isn't one: %d", w.Code)
	}
	if w := tvCall(h, "GET", "/api/logos?kind=studio&q=open", admin, ""); !strings.Contains(w.Body.String(), "Open Pictures") {
		t.Errorf("found by name: %s", w.Body)
	}
}

// The guided setup's two questions: a check that can't be read is refused, and a test of a
// tuner that isn't there says so rather than tuning anything.
func TestTunerCheckAndTestAnswers(t *testing.T) {
	h, admin, _, _ := tvServer(t)
	if w := tvCall(h, "POST", "/api/tuners/check", admin, "not json"); w.Code != 400 {
		t.Errorf("unreadable check: %d %s", w.Code, w.Body)
	}
	if w := tvCall(h, "POST", "/api/tuners/9/test", admin, "{}"); w.Code != 404 {
		t.Errorf("test of a tuner that isn't there: %d %s", w.Code, w.Body)
	}
	if w := tvCall(h, "POST", "/api/tuners/x/test", admin, "{}"); w.Code != 400 {
		t.Errorf("bad id: %d", w.Code)
	}
	// A playlist file elsewhere on the server is refused before anything is asked of WittyTuner.
	w := tvCall(h, "POST", "/api/tuners/check", admin, `{"driver":"iptv","name":"x","options":{"playlist":"/nowhere/at/all.m3u"}}`)
	var got struct {
		OK      bool           `json:"ok"`
		Problem livetv.Problem `json:"problem"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != 200 || got.OK || got.Problem.Kind != "settings" {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}
