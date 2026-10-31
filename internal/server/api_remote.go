package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/portmap"
	"github.com/wittywilla/wittymediaserver/internal/remote"
)

// Settings → Remote Access. Everything here is off until the owner turns it
// on, and the server contacts nobody for it, with two exceptions the page
// spells out: asking Let's Encrypt for a certificate (only if chosen) and
// asking the router on the home network to forward a port (only if chosen).

func (s *Server) remoteRoutes(m *http.ServeMux) {
	a, adm := s.requireUser, s.requireAdmin
	m.HandleFunc("GET /api/reach/{nonce}", s.handleReach) // public: this is how the outside test gets in
	m.Handle("GET /api/remote", adm(s.handleRemote))
	m.Handle("PUT /api/remote", adm(s.handlePutRemote))
	m.Handle("POST /api/remote/test", adm(s.handleStartTest))
	m.Handle("POST /api/remote/selfcheck", adm(s.handleSelfCheck))
	m.Handle("POST /api/remote/router", adm(s.handleLookRouter))
	m.Handle("POST /api/remote/obtain", adm(s.handleObtain))
	m.Handle("DELETE /api/remote/lockouts", adm(s.handleClearLockouts))
	m.Handle("GET /api/sessions", a(s.handleSessions))
	m.Handle("DELETE /api/sessions", a(s.handleEndOtherSessions))
	m.Handle("DELETE /api/sessions/{id}", a(s.handleEndSession))
	m.Handle("GET /api/me/2fa", a(s.handleTwoFactor))
	m.Handle("POST /api/me/2fa/setup", a(s.handleTwoFactorSetup))
	m.Handle("POST /api/me/2fa/confirm", a(s.handleTwoFactorConfirm))
	m.Handle("POST /api/me/2fa/disable", a(s.handleTwoFactorDisable))
	m.Handle("DELETE /api/users/{id}/2fa", adm(s.handleUserTwoFactorReset))
}

// remoteSettings is the owner's choices, saved as one setting.
type remoteSettings struct {
	Enabled        bool   `json:"enabled"`
	Method         string `json:"method"` // tailscale | netbird | router | proxy | vps
	PublicURL      string `json:"publicUrl"`
	TrustedProxies string `json:"trustedProxies"`
	HomeNets       string `json:"homeNets"`
	AwayNets       string `json:"awayNets"`
	MaxHeight      int    `json:"maxHeight"`  // 0 = no limit
	MaxBitrate     string `json:"maxBitrate"` // "" = no limit, e.g. "4M"
	HTTPSMode      string `json:"httpsMode"`  // off | acme | cert
	HTTPSPort      int    `json:"httpsPort"`
	Domain         string `json:"domain"`
	Email          string `json:"email"`
	AcceptTerms    bool   `json:"acceptTerms"` // Let's Encrypt's subscriber agreement
	CertFile       string `json:"certFile"`
	KeyFile        string `json:"keyFile"`
	PortMapping    bool   `json:"portMapping"`
	ExternalPort   int    `json:"externalPort"`
}

const remoteSettingKey = "remoteAccess"

func defaultRemote() remoteSettings {
	return remoteSettings{HTTPSMode: "off", HTTPSPort: 9443, ExternalPort: 443}
}

func (s *Server) loadRemote(ctx context.Context) remoteSettings {
	r := defaultRemote()
	if raw := s.DB.GetSetting(ctx, remoteSettingKey, ""); raw != "" {
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			s.Log.WarnContext(ctx, "remote access settings are unreadable; treating them as off", "cat", "remote", "err", err)
			return defaultRemote()
		}
	}
	return r
}

var (
	emailRe   = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	heightsOK = map[int]bool{0: true, 360: true, 480: true, 720: true, 1080: true, 1440: true, 2160: true}
)

// validate checks and tidies the settings.
func (r *remoteSettings) validate(listen string) error {
	switch r.Method {
	case "", "tailscale", "netbird", "router", "proxy", "vps":
	default:
		return errors.New("unknown way of connecting")
	}
	r.PublicURL = strings.TrimRight(strings.TrimSpace(r.PublicURL), "/")
	if r.PublicURL != "" {
		u, err := url.Parse(r.PublicURL)
		if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("the outside address should look like https://media.example.com")
		}
		if u.Scheme != "https" {
			return errors.New("the outside address must start with https://: passwords must not cross the internet unencrypted")
		}
	}
	for _, n := range []struct{ label, list string }{{"Trusted proxies", r.TrustedProxies}, {"Home networks", r.HomeNets}, {"Away networks", r.AwayNets}} {
		if _, err := remote.ParseNets(n.list); err != nil {
			return fmt.Errorf("%s: %w", n.label, err)
		}
	}
	if !heightsOK[r.MaxHeight] {
		return errors.New("unknown resolution limit")
	}
	r.MaxBitrate = strings.TrimSpace(r.MaxBitrate)
	if r.MaxBitrate != "" {
		if b := ffmpeg.ParseRate(r.MaxBitrate); b < 250000 || b > 1000000000 {
			return errors.New("the bitrate limit should be between 250k and 1000M")
		}
	}
	if r.HTTPSPort == 0 {
		r.HTTPSPort = 9443
	}
	if r.HTTPSPort < 1 || r.HTTPSPort > 65535 {
		return errors.New("the HTTPS port should be a number from 1 to 65535")
	}
	if _, p, err := net.SplitHostPort(listen); err == nil && p == strconv.Itoa(r.HTTPSPort) {
		return errors.New("the HTTPS port can't be the port the server already uses for home")
	}
	if r.ExternalPort == 0 {
		r.ExternalPort = 443
	}
	if r.ExternalPort < 1 || r.ExternalPort > 65535 {
		return errors.New("the outside port should be a number from 1 to 65535")
	}
	r.Domain = strings.ToLower(strings.TrimSpace(r.Domain))
	r.Email = strings.TrimSpace(r.Email)
	if r.Email != "" && !emailRe.MatchString(r.Email) {
		return errors.New("that email address doesn't look right")
	}
	switch r.HTTPSMode {
	case "", "off":
		r.HTTPSMode = "off"
	case "acme":
		if !remote.ValidDomain(r.Domain) {
			return errors.New("Let's Encrypt needs a domain name, such as media.example.com")
		}
		if !r.AcceptTerms {
			return errors.New("Let's Encrypt asks you to accept its subscriber agreement first")
		}
	case "cert":
		r.CertFile, r.KeyFile = strings.TrimSpace(r.CertFile), strings.TrimSpace(r.KeyFile)
		if !filepath.IsAbs(r.CertFile) || !filepath.IsAbs(r.KeyFile) {
			return errors.New("give the full path of both the certificate and its key")
		}
	default:
		return errors.New("unknown HTTPS mode")
	}
	if r.PortMapping && r.HTTPSMode == "off" {
		return errors.New("turn on HTTPS first: port mapping only ever opens the encrypted port")
	}
	return nil
}

// reloadNet rebuilds what requests read. Unusable lists count as empty, which
// trusts less, never more.
func (s *Server) reloadNet(ctx context.Context) *netState {
	cfg := s.loadRemote(ctx)
	st := &netState{cfg: cfg, policy: &remote.Policy{}}
	trusted := cfg.TrustedProxies
	if s.Cfg != nil {
		st.forced = s.Cfg.RemoteAccess || s.Cfg.PublicURL != ""
		trusted += "," + s.Cfg.TrustedProxies
	}
	st.on = cfg.Enabled || st.forced
	var err error
	if st.policy.Trusted, err = remote.ParseNets(trusted); err != nil {
		s.Log.Warn("could not read the trusted proxies", "cat", "remote", "err", err)
	}
	if st.policy.Home, err = remote.ParseNets(cfg.HomeNets); err != nil {
		s.Log.Warn("could not read the home networks", "cat", "remote", "err", err)
	}
	if st.policy.Away, err = remote.ParseNets(cfg.AwayNets); err != nil {
		s.Log.Warn("could not read the away networks", "cat", "remote", "err", err)
	}
	st.publicURL = cfg.PublicURL
	if st.publicURL == "" && s.Cfg != nil {
		st.publicURL = strings.TrimRight(s.Cfg.PublicURL, "/")
	}
	if st.publicURL == "" && cfg.HTTPSMode == "acme" && cfg.Domain != "" {
		st.publicURL = "https://" + cfg.Domain
		if cfg.ExternalPort != 443 {
			st.publicURL += ":" + strconv.Itoa(cfg.ExternalPort)
		}
	}
	if u, err := url.Parse(st.publicURL); err == nil {
		st.publicHost = u.Host
	}
	st.capBits = ffmpeg.ParseRate(cfg.MaxBitrate)
	if cfg.MaxHeight > 0 || st.capBits > 0 {
		st.capTag = fmt.Sprintf("-cap%d-%d", cfg.MaxHeight, st.capBits/1000)
	}
	s.netState.Store(st)
	return st
}

// StartRemote opens what the saved settings ask for. handler is what the HTTPS
// port serves.
func (s *Server) StartRemote(ctx context.Context, handler http.Handler) {
	s.HTTPS = remote.NewHTTPS(handler, s.Cfg.DataDir, s.Log)
	s.Mapper = &portmap.Keeper{Log: s.Log.With("cat", "remote")}
	if err := s.applyNet(ctx); err != nil {
		s.Log.WarnContext(ctx, "could not set up remote access", "cat", "remote", "err", err)
	}
}

// StopRemote closes the HTTPS port and removes any port mapping.
func (s *Server) StopRemote(ctx context.Context) {
	if s.Mapper != nil {
		s.Mapper.Stop()
	}
	if s.HTTPS != nil {
		s.HTTPS.Shutdown(ctx)
	}
}

// applyNet makes the running server match the saved settings. Whatever could
// not start is reported; the rest is still applied.
func (s *Server) applyNet(ctx context.Context) error {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	st := s.reloadNet(ctx)
	if s.HTTPS == nil {
		return nil
	}
	c := remote.HTTPSConfig{Mode: "off"}
	if st.on && st.cfg.HTTPSMode != "off" {
		host, _, _ := net.SplitHostPort(s.Cfg.Listen)
		c = remote.HTTPSConfig{Mode: st.cfg.HTTPSMode, Host: host, Port: st.cfg.HTTPSPort, Domain: st.cfg.Domain, Email: st.cfg.Email, CertFile: st.cfg.CertFile, KeyFile: st.cfg.KeyFile}
	}
	err := s.HTTPS.Apply(c)
	if st.on && st.cfg.PortMapping && s.HTTPS.Running() {
		s.Mapper.Set(st.cfg.ExternalPort, st.cfg.HTTPSPort)
	} else {
		s.Mapper.Stop()
	}
	return err
}

// PUT /api/remote: changes any of the settings; the rest stay.
func (s *Server) handlePutRemote(w http.ResponseWriter, r *http.Request) {
	prev := s.loadRemote(r.Context())
	cur := prev
	if err := readJSON(r, &cur); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := cur.validate(s.Cfg.Listen); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	raw, _ := json.Marshal(cur)
	if err := s.DB.SetSetting(r.Context(), remoteSettingKey, string(raw)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// The outside test proved the old way in, not this one.
	if cur.Method != prev.Method || (prev.Enabled && !cur.Enabled) {
		if err := s.DB.SetSetting(r.Context(), "remoteLastProof", ""); err != nil {
			s.Log.WarnContext(r.Context(), "could not forget the last outside test", "cat", "remote", "err", err)
		}
	}
	s.Log.InfoContext(r.Context(), "remote access settings changed", "cat", "remote", "user", userFrom(r).Name, "enabled", cur.Enabled, "method", cur.Method, "https", cur.HTTPSMode, "mapping", cur.PortMapping)
	if err := s.applyNet(r.Context()); err != nil {
		s.Log.WarnContext(r.Context(), "could not apply the remote access settings", "cat", "remote", "err", err)
		// Saved and reported: the status shows what is wrong and how it stands.
		writeJSON(w, 200, s.remoteStatus(r, err.Error()))
		return
	}
	writeJSON(w, 200, s.remoteStatus(r, ""))
}

// ---------------------------------------------------------------- status

type route struct {
	Kind     string   `json:"kind"`  // home | vpn | https | proxy
	Label    string   `json:"label"` // plain words
	State    string   `json:"state"` // ok | waiting | problem
	URLs     []string `json:"urls"`
	Detail   string   `json:"detail,omitempty"`
	LastSeen int64    `json:"lastSeen,omitempty"`
}

type notice struct {
	Level string `json:"level"` // warn | info
	Text  string `json:"text"`
	// Trust names a proxy that could be added to the trusted list.
	Trust string `json:"trust,omitempty"`
	Link  string `json:"link,omitempty"`
}

type remoteView struct {
	Settings remoteSettings `json:"settings"`
	Forced   bool           `json:"forced"`
	State    string         `json:"state"` // off | ok | waiting | problem
	Headline string         `json:"headline"`
	Detail   string         `json:"detail"`
	Routes   []route        `json:"routes"`
	Notices  []notice       `json:"notices"`
	You      youView        `json:"you"`
	// VPNs are the overlay networks found on this machine.
	VPNs []remote.VPNAddr `json:"vpns"`
	// PublicURL is where the server is reached from outside, as far as known.
	PublicURL   string             `json:"publicUrl"`
	HTTPS       remote.HTTPSStatus `json:"https"`
	Mapping     portmap.Status     `json:"mapping"`
	Router      *portmap.Probe     `json:"router,omitempty"`
	Test        *testView          `json:"test,omitempty"`
	LastProof   *reachHit          `json:"lastProof,omitempty"`
	Addresses   []string           `json:"addresses"` // what the outside test can try
	HomePort    string             `json:"homePort"`
	Lockouts    []lockout          `json:"lockouts"`
	Failures    int                `json:"failures"`
	WeakAdmins  []auth.WeakAccount `json:"weakAccounts"`
	No2FAAdmins int                `json:"adminsWithout2fa"`
	Error       string             `json:"error,omitempty"`
}

type youView struct {
	Where   string `json:"where"`
	Via     string `json:"via"`
	IP      string `json:"ip"`
	Through string `json:"through,omitempty"` // the proxy in between
}

func (s *Server) handleRemote(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.remoteStatus(r, ""))
}

// homeURLs are the addresses of this server on the home network.
func (s *Server) homeURLs() []string {
	host, port, err := net.SplitHostPort(s.Cfg.Listen)
	if err != nil {
		return []string{}
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return []string{}
	}
	out := []string{}
	for _, i := range remote.Interfaces() {
		if a := i.Addr.Addr(); a.Is4() && a.IsPrivate() {
			out = append(out, "http://"+net.JoinHostPort(a.String(), port))
		}
	}
	return out
}

func (s *Server) remoteStatus(r *http.Request, applyErr string) remoteView {
	ctx := r.Context()
	st := s.state()
	cfg := st.cfg
	c := clientFrom(r)
	v := remoteView{Settings: cfg, Forced: st.forced, Routes: []route{}, Notices: []notice{}, PublicURL: st.publicURL, Addresses: []string{}, Error: applyErr}
	v.You = youView{Where: c.Where.String(), Via: viaName(c, r), IP: c.IP.String()}
	if c.Proxied {
		v.You.Through = c.Peer.String()
	}
	if s.HTTPS != nil {
		v.HTTPS = s.HTTPS.Status(ctx)
	} else {
		v.HTTPS = remote.HTTPSStatus{Mode: "off"}
	}
	if s.Mapper != nil {
		v.Mapping = s.Mapper.Status()
	}
	s.routerMu.Lock()
	v.Router = s.router
	s.routerMu.Unlock()
	v.Lockouts, v.Failures = s.logins.snapshot()
	v.WeakAdmins, _ = s.Auth.WeakAccounts(ctx)
	if v.WeakAdmins == nil {
		v.WeakAdmins = []auth.WeakAccount{}
	}
	v.No2FAAdmins = s.Auth.AdminsWithoutTwoFactor(ctx)
	v.VPNs = remote.FindVPNs(remote.Interfaces())
	if v.VPNs == nil {
		v.VPNs = []remote.VPNAddr{}
	}
	v.Test = s.reach.view()
	v.LastProof = s.lastProof(ctx)

	s.outside.mu.Lock()
	seenNet, seenVPN, seenProxy, seenPlain := s.outside.internet, s.outside.vpn, s.outside.proxied, s.outside.plain
	unverPeer, unverAt := s.outside.unverPeer, s.outside.unverAt
	s.outside.mu.Unlock()

	homeHost, homePort, _ := net.SplitHostPort(s.Cfg.Listen)
	v.HomePort = homePort
	localOnly := false
	if ip := net.ParseIP(homeHost); ip != nil && ip.IsLoopback() {
		localOnly = true
	}

	v.Routes = append(v.Routes, route{Kind: "home", Label: "Home network", State: "ok", URLs: s.homeURLs()})

	if st.on {
		for _, n := range v.VPNs {
			rt := route{Kind: "vpn", Label: n.Kind, State: "waiting", URLs: []string{"http://" + net.JoinHostPort(n.IP, homePort)}, LastSeen: unixOr0(seenVPN),
				Detail: "Encrypted by " + n.Kind + ". Only your own signed-in devices can use this address."}
			if !seenVPN.IsZero() {
				rt.State = "ok"
			}
			if localOnly {
				rt.State, rt.Detail = "problem", "The server only listens on this computer, so devices on your VPN can't reach it. Start it without a -listen limit."
			}
			v.Routes = append(v.Routes, rt)
			v.Addresses = append(v.Addresses, rt.URLs...)
		}
		if st.publicURL != "" || cfg.HTTPSMode != "off" {
			rt := route{Kind: "https", Label: "The internet, over HTTPS", State: "waiting", URLs: []string{}, LastSeen: unixOr0(seenNet)}
			if st.publicURL != "" {
				rt.URLs = append(rt.URLs, st.publicURL)
				v.Addresses = append(v.Addresses, st.publicURL)
			}
			switch {
			case cfg.HTTPSMode != "off" && !v.HTTPS.Running:
				rt.State, rt.Detail = "problem", "The HTTPS port isn't open."
				if v.HTTPS.Error != "" {
					rt.Detail += " " + v.HTTPS.Error
				}
			case v.Mapping.On && v.Mapping.Shared:
				rt.State, rt.Detail = "problem", "Your router's outside address is shared with others, so nothing from the internet can reach it. Use a VPN instead, or ask your internet provider for a public address."
			case v.Mapping.On && !v.Mapping.Working && v.Mapping.Error != "":
				rt.State, rt.Detail = "problem", "The router didn't open the port: "+v.Mapping.Error
			case v.HTTPS.Running && v.HTTPS.Error != "":
				rt.State, rt.Detail = "problem", "The certificate isn't ready: "+v.HTTPS.Error
			case !seenNet.IsZero() || (v.LastProof != nil && v.LastProof.Where == "internet"):
				rt.State = "ok"
			}
			v.Routes = append(v.Routes, rt)
		}
		if st.policy != nil && len(st.policy.Trusted) > 0 {
			rt := route{Kind: "proxy", Label: "Your reverse proxy", State: "waiting", URLs: []string{}, LastSeen: unixOr0(seenProxy), Detail: "Forwarded addresses are believed only from the proxies you listed."}
			if !seenProxy.IsZero() {
				rt.State = "ok"
			}
			v.Routes = append(v.Routes, rt)
		}
	}

	proofed := v.LastProof != nil && time.Since(time.Unix(v.LastProof.At, 0)) < 7*24*time.Hour
	switch {
	case !st.on:
		v.State = "off"
		v.Headline = "Only your home network"
		v.Detail = "Devices on your Wi-Fi or cable can use this server. Connections from anywhere else are refused, because Remote Access is off."
	case len(v.Routes) == 1:
		v.State = "waiting"
		v.Headline = "Turned on, but no way in yet"
		v.Detail = "Remote Access is on, and nothing can reach the server from outside yet. Choose a way of connecting below and follow its steps."
	default:
		problems, oks := 0, 0
		var okLabels []string
		for _, rt := range v.Routes[1:] {
			switch rt.State {
			case "problem":
				problems++
			case "ok":
				oks++
				okLabels = append(okLabels, rt.Label)
			}
		}
		switch {
		case problems > 0 && oks == 0:
			v.State = "problem"
			v.Headline = "Something is in the way"
			v.Detail = "See the problem below."
		case oks > 0 || proofed:
			v.State = "ok"
			v.Headline = "Reachable from outside your home"
			if len(okLabels) > 0 {
				v.Detail = "Through " + strings.Join(okLabels, " and ") + "."
			}
			if proofed {
				v.Detail = strings.TrimSpace(v.Detail + " Confirmed by the outside test.")
			}
		default:
			v.State = "waiting"
			v.Headline = "Ready, not confirmed yet"
			v.Detail = "Everything is set up here, but no connection from outside has been seen yet. Run the outside test below."
		}
	}

	// What the owner should know.
	warn := func(text string) { v.Notices = append(v.Notices, notice{Level: "warn", Text: text}) }
	if localOnly && st.on {
		warn("The server is set to listen on this computer only (the listen setting), so nothing from outside can reach it.")
	}
	if st.on {
		for _, w := range v.WeakAdmins {
			if w.IsAdmin && !w.Unknown {
				v.Notices = append(v.Notices, notice{Level: "warn", Text: "The administrator account “" + w.Name + "” has a weak password. Anyone on the internet can try to guess it, so choose a longer one before relying on remote access.", Link: "/settings/users"})
			}
		}
		for _, w := range v.WeakAdmins {
			if !w.IsAdmin && !w.Unknown {
				v.Notices = append(v.Notices, notice{Level: "info", Text: "The account “" + w.Name + "” has a weak password.", Link: "/settings/users"})
			}
		}
		if s.DB.GetSetting(ctx, "profilesRemote", "0") == "1" {
			v.Notices = append(v.Notices, notice{Level: "warn", Text: "“Show profiles outside your home” is on, so anyone who finds this server sees everyone's names.", Link: "/settings/users"})
		}
		if cert := v.HTTPS.Cert; cert != nil && v.HTTPS.Running && cert.DaysLeft < 14 {
			if cfg.HTTPSMode == "cert" {
				warn(fmt.Sprintf("Your HTTPS certificate ends in %d days. Renew it, and the server picks up the new files by itself.", cert.DaysLeft))
			} else if cert.DaysLeft < 7 {
				warn(fmt.Sprintf("The Let's Encrypt certificate ends in %d days and has not been renewed. Check that port %d still reaches this computer.", cert.DaysLeft, cfg.ExternalPort))
			}
		}
		if cert := v.HTTPS.Cert; cert != nil && cert.SelfSigned && v.HTTPS.Running {
			v.Notices = append(v.Notices, notice{Level: "info", Text: "Your certificate is self-signed, so browsers will warn everyone who visits."})
		}
	}
	if !unverAt.IsZero() && time.Since(unverAt) < 24*time.Hour {
		v.Notices = append(v.Notices, notice{Level: "warn", Trust: unverPeer,
			Text: "Requests are arriving through a reverse proxy at " + unverPeer + " that isn't on your trusted list, so everyone behind it looks like one visitor from the internet, and their addresses are unknown. If that is your proxy, trust it."})
	}
	if !seenPlain.IsZero() && time.Since(seenPlain) < 24*time.Hour {
		warn("Someone on the internet reached the server's plain, unencrypted port, and was turned away. If you opened that port on your router, close it: only the HTTPS port should be open.")
	}
	return v
}

// ---------------------------------------------------------------- the outside test

// reachTest is one outstanding test: a secret address, good for a little while.
// A phone with Wi-Fi off opens it, and the server sees for itself where that
// came from. No outside service is involved.
type reachTest struct {
	mu      sync.Mutex
	nonce   string
	url     string
	created time.Time
	hit     *reachHit
	self    string // the server asking itself (never counts as proof)
}

type reachHit struct {
	At    int64  `json:"at"`
	Where string `json:"where"` // machine | home | vpn | internet
	Via   string `json:"via"`
	IP    string `json:"ip"`
	Proof bool   `json:"proof"` // it really came from outside
}

type testView struct {
	URL     string    `json:"url"`
	Expires int64     `json:"expires"`
	Hit     *reachHit `json:"hit,omitempty"`
}

const reachTTL = 15 * time.Minute

func newNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (t *reachTest) view() *testView {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.nonce == "" || time.Since(t.created) > reachTTL {
		return nil
	}
	return &testView{URL: t.url, Expires: t.created.Add(reachTTL).Unix(), Hit: t.hit}
}

// POST /api/remote/test {address}: starts a test of one of the addresses the
// status offers.
func (s *Server) handleStartTest(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Address string `json:"address"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	known := false
	for _, a := range s.remoteStatus(r, "").Addresses {
		known = known || a == b.Address
	}
	if !known {
		writeErr(w, 400, "there's no outside address to test yet")
		return
	}
	s.reach.mu.Lock()
	s.reach.nonce, s.reach.created, s.reach.hit = newNonce(), time.Now(), nil
	s.reach.url = strings.TrimRight(b.Address, "/") + "/api/reach/" + s.reach.nonce
	s.reach.mu.Unlock()
	writeJSON(w, 200, s.reach.view())
}

// reachPage is everything the outside test shows: nothing about the server.
const reachPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Connection test</title>` +
	`<style>body{margin:0;min-height:100vh;display:grid;place-items:center;padding:24px;box-sizing:border-box;font:18px/1.5 system-ui,sans-serif;background:#0f1115;color:#e8eaf0}main{max-width:32em}h1{font-size:1.5em;margin:0 0 .4em}</style>` +
	`</head><body><main role="main"><h1>%s</h1><p>%s</p></main></body></html>`

// GET /api/reach/{nonce} (public). An unknown or old address looks like any
// other missing page.
func (s *Server) handleReach(w http.ResponseWriter, r *http.Request) {
	n := r.PathValue("nonce")
	t := &s.reach
	t.mu.Lock()
	self := n != "" && n == t.self
	live := n != "" && n == t.nonce && time.Since(t.created) <= reachTTL
	t.mu.Unlock()
	if !self && !live {
		writeErr(w, 404, "not found")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if self {
		fmt.Fprintf(w, reachPage, "OK", "self-check")
		return
	}
	c := clientFrom(r)
	hit := &reachHit{At: time.Now().Unix(), Where: c.Where.String(), Via: viaName(c, r), IP: c.IP.String(), Proof: !c.Where.AtHome()}
	t.mu.Lock()
	// A later test from home doesn't undo one from outside.
	if t.hit == nil || !t.hit.Proof || hit.Proof {
		t.hit = hit
	}
	t.mu.Unlock()
	if hit.Proof {
		if raw, err := json.Marshal(hit); err == nil {
			if err := s.DB.SetSetting(r.Context(), "remoteLastProof", string(raw)); err != nil {
				s.Log.WarnContext(r.Context(), "could not save the outside test", "cat", "remote", "err", err)
			}
		}
		fmt.Fprintf(w, reachPage, "It works!", "Your server answered from outside your home. You can close this page and go back to Settings.")
		return
	}
	fmt.Fprintf(w, reachPage, "This device is at home", "Your server answered, but this device is on your home network, so that proves nothing yet. Turn off Wi-Fi on your phone so it uses mobile data, and open this address again.")
}

func (s *Server) lastProof(ctx context.Context) *reachHit {
	raw := s.DB.GetSetting(ctx, "remoteLastProof", "")
	if raw == "" {
		return nil
	}
	var h reachHit
	if json.Unmarshal([]byte(raw), &h) != nil {
		return nil
	}
	return &h
}

// checkStep is one step of the inside check.
type checkStep struct {
	Name   string `json:"name"`
	State  string `json:"state"` // ok | warn | fail
	Detail string `json:"detail"`
}

// POST /api/remote/selfcheck: what can be learned from inside the house: does
// the name lead to this home, is the port open on this computer, and does the
// address answer when the server asks it itself. Only the name server the
// computer already uses is asked. A router that won't loop this back to its
// own network proves nothing, so that is never reported as a failure.
func (s *Server) handleSelfCheck(w http.ResponseWriter, r *http.Request) {
	st := s.state()
	steps := []checkStep{}
	add := func(name, state, detail string) { steps = append(steps, checkStep{name, state, detail}) }
	base := st.publicURL
	if base == "" {
		add("Outside address", "fail", "There is no outside address yet. Enter your domain name above first.")
		writeJSON(w, 200, map[string]any{"steps": steps})
		return
	}
	u, err := url.Parse(base)
	if err != nil {
		add("Outside address", "fail", "The outside address can't be read.")
		writeJSON(w, 200, map[string]any{"steps": steps})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	host := u.Hostname()
	var ips []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{ip}
	} else {
		found, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil || len(found) == 0 {
			add("Domain name", "fail", "“"+host+"” doesn't lead anywhere yet. Check the name at your domain or dynamic DNS provider; changes can take a while to spread.")
		}
		ips = found
	}
	router := s.knownOutsideIP()
	if len(ips) > 0 {
		var list []string
		private := false
		matches := false
		for _, a := range ips {
			a = a.Unmap()
			list = append(list, a.String())
			private = private || a.IsPrivate() || a.IsLoopback()
			matches = matches || (router != "" && a.String() == router)
		}
		switch {
		case private:
			add("Domain name", "fail", host+" points to an address inside a home network ("+strings.Join(list, ", ")+"). It should point to your router's outside address.")
		case router != "" && !matches:
			add("Domain name", "warn", host+" points to "+strings.Join(list, ", ")+", but your router says its outside address is "+router+". If you use a reverse proxy elsewhere that's fine; otherwise update the name.")
		case router != "":
			add("Domain name", "ok", host+" points to your router's outside address, "+router+".")
		default:
			add("Domain name", "ok", host+" points to "+strings.Join(list, ", ")+".")
		}
	}
	if st.cfg.HTTPSMode != "off" && s.HTTPS != nil {
		if a := s.HTTPS.Addr(); a != nil {
			add("HTTPS port", "ok", "Port "+strconv.Itoa(a.(*net.TCPAddr).Port)+" is open on this computer.")
		} else {
			add("HTTPS port", "fail", "The HTTPS port isn't open. "+s.HTTPS.Status(ctx).Error)
		}
	}

	nonce := newNonce()
	s.reach.mu.Lock()
	s.reach.self = nonce
	s.reach.mu.Unlock()
	defer func() {
		s.reach.mu.Lock()
		s.reach.self = ""
		s.reach.mu.Unlock()
	}()
	client := &http.Client{
		Timeout:       6 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{TLSHandshakeTimeout: 5 * time.Second, DisableKeepAlives: true},
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/api/reach/"+nonce, nil)
	res, err := client.Do(req)
	switch {
	case err == nil && res.StatusCode == 200:
		add("Answer", "ok", "Asking "+base+" from this computer reached this very server, with a valid certificate.")
	case err == nil:
		add("Answer", "warn", fmt.Sprintf("Something answers at %s, but it isn't this server (status %d).", base, res.StatusCode))
	case strings.Contains(err.Error(), "certificate"):
		add("Answer", "warn", "The address answers, but its certificate isn't accepted: "+certProblem(err))
	default:
		add("Answer", "warn", "This computer couldn't reach "+base+". Many routers don't allow that from inside the house, so this doesn't mean it fails from outside: run the test with your phone.")
	}
	if res != nil {
		res.Body.Close()
	}
	writeJSON(w, 200, map[string]any{"steps": steps})
}

func certProblem(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}

// knownOutsideIP is the router's outside address, when it has told us.
func (s *Server) knownOutsideIP() string {
	if s.Mapper != nil {
		if ip := s.Mapper.Status().ExternalIP; ip != "" {
			return ip
		}
	}
	s.routerMu.Lock()
	defer s.routerMu.Unlock()
	if s.router != nil {
		return s.router.ExternalIP
	}
	return ""
}

// POST /api/remote/router: looks for the router on the home network and asks
// what it can do. It changes nothing on the router.
func (s *Server) handleLookRouter(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	p := portmap.Look(ctx)
	s.routerMu.Lock()
	s.router = p
	s.routerMu.Unlock()
	writeJSON(w, 200, p)
}

// POST /api/remote/obtain: asks Let's Encrypt for the certificate right away.
func (s *Server) handleObtain(w http.ResponseWriter, r *http.Request) {
	if s.HTTPS == nil || !s.HTTPS.Running() || s.state().cfg.HTTPSMode != "acme" {
		writeErr(w, 400, "turn on HTTPS with Let's Encrypt first")
		return
	}
	s.HTTPS.Obtain()
	writeJSON(w, 200, s.remoteStatus(r, ""))
}

// DELETE /api/remote/lockouts: lets everyone try signing in again.
func (s *Server) handleClearLockouts(w http.ResponseWriter, r *http.Request) {
	s.logins.clear()
	writeJSON(w, 200, map[string]any{"ok": true})
}
