// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"

	"github.com/wittywilla/wittymediaserver/internal/p2p"
	"github.com/wittywilla/wittymediaserver/internal/remote"
)

// Settings → Remote Access → Peer-to-peer connections: reaching the server
// from away with nothing set up on the router and nothing rented. Off until
// the owner turns it on. While off, no node runs, nothing is listened on,
// nobody is contacted and the router is asked for nothing.
//
// What a peer-to-peer connection may do is decided in three places, and each
// stands on its own:
//   - here, by the list of devices: a key that isn't on it can't open an HTTP
//     stream at all (unless the owner chose otherwise);
//   - in internal/remote, where such a request always counts as coming from
//     the internet, whatever address its connection has;
//   - by everything the server already does for requests from the internet:
//     sign-in, its limits, two-factor codes, stream limits.
//
// docs/P2P.md has the design, docs/REMOTE.md the guide for owners.

func init() {
	// A panic in the node's own goroutines is reported like any other.
	p2p.Spawn = func(name string, fn func()) { logging.Go(name, fn, "cat", "p2p") }
}

func (s *Server) p2pRoutes(m *http.ServeMux) {
	a, adm := s.requireUser, s.requireAdmin
	m.Handle("GET /api/p2p", a(s.handleP2PInfo))
	m.Handle("GET /api/p2p/devices", a(s.handleP2PDevices))
	m.Handle("POST /api/p2p/devices", a(s.handleP2PAddDevice))
	m.Handle("DELETE /api/p2p/devices/{id}", a(s.handleP2PRemoveDevice))
	m.Handle("POST /api/p2p/invite", a(s.handleP2PInvite))
	m.Handle("POST /api/p2p/join", a(s.handleP2PJoin))
	m.Handle("GET /api/p2p/settings", adm(s.handleP2PSettings))
	m.Handle("PUT /api/p2p/settings", adm(s.handlePutP2PSettings))
	m.Handle("GET /api/p2p/status", adm(s.handleP2PStatus))
	m.Handle("POST /api/p2p/test", adm(s.handleP2PTest))
	m.Handle("POST /api/p2p/rotate", adm(s.handleP2PRotate))
	m.Handle("POST /api/p2p/relay-check", adm(s.handleP2PRelayCheck))
}

// ---------------------------------------------------------------- settings

// p2pSettings is the owner's choices, saved as one setting.
type p2pSettings struct {
	Enabled bool `json:"enabled"`
	// Port is listened on for UDP and TCP. Chosen once, when first turned on,
	// and kept: apps remember addresses, and so does the router's mapping.
	Port   int        `json:"port"`
	Layers p2p.Layers `json:"layers"`
	// DevicesOnly: only devices on the list may connect. Off, anyone who has
	// the server's connection ID reaches the sign-in page.
	DevicesOnly bool `json:"devicesOnly"`
	// RelayKbps limits what is sent through a relay (0: no limit).
	RelayKbps int `json:"relayKbps"`
	// RelayForOthers offers this server as a community relay while it can be
	// reached from outside.
	RelayForOthers bool     `json:"relayForOthers"`
	StaticRelays   []string `json:"staticRelays"`
	// ShareKbps and ShareMaxHeight are what this server allows others while
	// it helps as a community relay: its upload for everyone together, and
	// the picture size their streams keep to. Both are told to other servers
	// before they use it.
	ShareKbps      int `json:"shareKbps"`
	ShareMaxHeight int `json:"shareMaxHeight"`
	// OwnRelays are WittyRelays: one the owner runs, or one they were given a
	// key to. Used before any other relay.
	OwnRelays []p2pOwnRelay `json:"ownRelays"`
}

// p2pOwnRelay is a WittyRelay and its access key. The key is never sent back
// to the page: HasKey says there is one, and a key left empty keeps it.
type p2pOwnRelay struct {
	Addr   string `json:"addr"`
	Key    string `json:"key,omitempty"`
	HasKey bool   `json:"hasKey,omitempty"`
}

const (
	p2pSettingKey = "p2p"
	p2pSeedKey    = "p2pKey"

	p2pMaxDevicesEach = 32  // per account
	p2pMaxDevices     = 512 // on the whole server
	p2pInviteTTL      = 15 * time.Minute
	p2pMaxInvites     = 16
)

func defaultP2P() p2pSettings {
	return p2pSettings{Layers: p2p.AllLayers(), DevicesOnly: true, RelayKbps: 4000, StaticRelays: []string{},
		ShareKbps: 8000, ShareMaxHeight: 720, OwnRelays: []p2pOwnRelay{}}
}

func (s *Server) loadP2P(ctx context.Context) p2pSettings {
	c := defaultP2P()
	if raw := s.DB.GetSetting(ctx, p2pSettingKey, ""); raw != "" {
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			s.Log.WarnContext(ctx, "the peer-to-peer settings are unreadable; treating them as off", "cat", "p2p", "err", err)
			return defaultP2P()
		}
	}
	if c.StaticRelays == nil {
		c.StaticRelays = []string{}
	}
	if c.OwnRelays == nil {
		c.OwnRelays = []p2pOwnRelay{}
	}
	for _, r := range c.OwnRelays {
		if r.Key != "" {
			logging.KeepSecret(r.Key)
		}
	}
	return c
}

// scrubP2PSetting is the saved settings without the WittyRelays' access keys,
// for a backup with no password. Unreadable, nothing of it is kept.
func scrubP2PSetting(raw string) string {
	var c p2pSettings
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return ""
	}
	for i := range c.OwnRelays {
		c.OwnRelays[i].Key, c.OwnRelays[i].HasKey = "", false
	}
	out, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(out)
}

func (c *p2pSettings) validate() error {
	if c.Port != 0 && (c.Port < 1024 || c.Port > 65535) {
		return errors.New("the port should be a number from 1024 to 65535")
	}
	if c.RelayKbps != 0 && (c.RelayKbps < 250 || c.RelayKbps > 1_000_000) {
		return errors.New("the limit through relays should be between 250 and 1000000 kbit/s, or 0 for none")
	}
	if len(c.StaticRelays) > 8 {
		return errors.New("at most 8 relays of your own")
	}
	clean := []string{}
	for _, r := range c.StaticRelays {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		// A relay is named by where it is and who it is: /ip4/…/tcp/…/p2p/<its ID>.
		i := strings.LastIndex(r, "/p2p/")
		if i <= 0 || len(r) > 300 {
			return fmt.Errorf("%q isn't a relay's address: it should end in /p2p/ and the relay's ID", r)
		}
		if _, err := p2p.CheckID(r[i+5:]); err != nil {
			return fmt.Errorf("%q isn't a relay's address: it should end in /p2p/ and the relay's ID", r)
		}
		clean = append(clean, r)
	}
	c.StaticRelays = clean
	if c.ShareKbps != 0 && (c.ShareKbps < 500 || c.ShareKbps > 1_000_000) {
		return errors.New("what you share as a relay should be between 500 and 1000000 kbit/s, or 0 for no limit")
	}
	if c.ShareMaxHeight != 0 && (c.ShareMaxHeight < 144 || c.ShareMaxHeight > 4320) {
		return errors.New("the picture size through your relay should be between 144 and 4320 lines, or 0 for any")
	}
	if len(c.OwnRelays) > 4 {
		return errors.New("at most 4 WittyRelays")
	}
	own := []p2pOwnRelay{}
	for _, r := range c.OwnRelays {
		r.Addr, r.Key = strings.TrimSpace(r.Addr), strings.TrimSpace(r.Key)
		if r.Addr == "" {
			continue
		}
		if _, err := p2p.ParseRelayAddr(r.Addr); err != nil {
			return fmt.Errorf("WittyRelay %q: %v", r.Addr, err)
		}
		if len(r.Key) > 200 {
			return errors.New("that access key is too long")
		}
		r.HasKey = r.Key != ""
		own = append(own, r)
	}
	c.OwnRelays = own
	if !c.Layers.Relay {
		c.Layers.HolePunch = false // a hole punch is arranged through a relay
	}
	return nil
}

// p2pState is the running side of the feature.
type p2pState struct {
	on      atomic.Bool
	mu      sync.Mutex // one change of the node at a time
	node    *p2p.Node
	started p2pSettings
	handler http.Handler
	devices p2pDevices
	invites p2pInvites
	testing sync.Mutex
	// tune changes the node's configuration before it starts, for tests.
	tune func(*p2p.NodeConfig)
	// nodeRef is node, for what must not wait on mu (the device list, whose
	// own lock is taken while mu is held).
	nodeRef atomic.Pointer[p2p.Node]
	// pairTries limits how often one account makes invites or types in codes:
	// each makes a way in that waits fifteen minutes.
	pairTries     *p2p.TryLimit
	pairTriesOnce sync.Once
	// relayCheckOpts are added to the relay check's node, for tests.
	relayCheckOpts []libp2p.Option
}

// p2pPairAllowed counts a try at making an invite or typing in a code, and
// answers 429 when the account has made too many lately.
func (s *Server) p2pPairAllowed(w http.ResponseWriter, r *http.Request) bool {
	s.p2p.pairTriesOnce.Do(func() { s.p2p.pairTries = p2p.NewTryLimit(10, 10*time.Minute, 4096) })
	u := userFrom(r)
	if u == nil || s.p2p.pairTries.Allow(fmt.Sprint(u.ID)) {
		return true
	}
	s.Log.WarnContext(r.Context(), "too many invites or codes from one account", "cat", "p2p", "user", u.Name)
	writeErr(w, 429, "Too many invites and codes in a short time. Wait a few minutes and try again.")
	return false
}

// p2pDevicesChanged tells the node the device list changed, so its
// WittyRelays hear at once which devices may come through them.
func (s *Server) p2pDevicesChanged() {
	if n := s.p2p.nodeRef.Load(); n != nil {
		n.DevicesChanged()
	}
}

// p2pRelayCaps are the limits a relay puts on streams through it: the
// picture height it asks for, and a data rate (bits/s) that fits through the
// pacing a stream through it gets, with room for sound and overhead.
func (s *Server) p2pRelayCaps(relay string) (maxHeight int, bps int64) {
	n := s.p2p.nodeRef.Load()
	if n == nil {
		return 0, 0
	}
	if o, ok := n.RelayOffer(relay); ok {
		maxHeight = o.MaxHeight
	}
	if limit := n.RelayStreamLimit(relay); limit > 0 {
		bps = limit * 85 / 100
	}
	return maxHeight, bps
}

func (s *Server) p2pOn() bool { return s.p2p.on.Load() }

// p2pNode is the running node, or nil.
func (s *Server) p2pNode() *p2p.Node {
	s.p2p.mu.Lock()
	defer s.p2p.mu.Unlock()
	return s.p2p.node
}

// p2pSeed reads the server's connection key, making it the first time.
func (s *Server) p2pSeed(ctx context.Context) ([]byte, error) {
	raw := s.DB.GetSetting(ctx, p2pSeedKey, "")
	if seed, err := p2p.DecodeSeed(raw); err == nil {
		logging.KeepSecret(raw)
		return seed, nil
	}
	seed, err := p2p.NewSeed()
	if err != nil {
		return nil, err
	}
	enc := p2p.EncodeSeed(seed)
	logging.KeepSecret(enc)
	if err := s.DB.SetSetting(ctx, p2pSeedKey, enc); err != nil {
		return nil, fmt.Errorf("saving the connection key: %w", err)
	}
	return seed, nil
}

// freeP2PPort finds a port that is free for both UDP and TCP.
func freeP2PPort() (int, error) {
	for range 40 {
		var b [2]byte
		if _, err := rand.Read(b[:]); err != nil {
			return 0, err
		}
		port := 20000 + (int(b[0])<<8|int(b[1]))%40000
		t, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			continue
		}
		u, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
		_ = t.Close()
		if err != nil {
			continue
		}
		_ = u.Close()
		return port, nil
	}
	return 0, errors.New("no free port was found")
}

// StartP2P starts peer-to-peer connections if the saved settings ask for
// them. handler is what requests arriving that way are served by: the same
// one as every other request.
func (s *Server) StartP2P(ctx context.Context, handler http.Handler) {
	s.p2p.mu.Lock()
	s.p2p.handler = handler
	s.p2p.mu.Unlock()
	if !s.loadP2P(ctx).Enabled {
		return // nothing is loaded, started or contacted
	}
	if err := s.applyP2P(ctx); err != nil {
		s.Log.WarnContext(ctx, "peer-to-peer connections could not start", "cat", "p2p", "err", err)
	}
}

// StopP2P ends them: connections close, and the router's mappings are removed.
func (s *Server) StopP2P() {
	s.p2p.mu.Lock()
	defer s.p2p.mu.Unlock()
	s.stopP2PLocked()
}

func (s *Server) stopP2PLocked() {
	s.p2p.on.Store(false)
	s.p2p.nodeRef.Store(nil)
	if s.p2p.node != nil {
		s.p2p.node.Close()
		s.p2p.node = nil
	}
}

// applyP2P makes the running node match the saved settings.
func (s *Server) applyP2P(ctx context.Context) error {
	s.p2p.mu.Lock()
	defer s.p2p.mu.Unlock()
	cfg := s.loadP2P(ctx)
	if !cfg.Enabled || s.p2p.handler == nil {
		s.stopP2PLocked()
		return nil
	}
	if s.p2p.node != nil && fmt.Sprint(s.p2p.started) == fmt.Sprint(cfg) {
		return nil
	}
	s.stopP2PLocked()
	seed, err := s.p2pSeed(ctx)
	if err != nil {
		return err
	}
	if cfg.Port == 0 {
		if cfg.Port, err = freeP2PPort(); err != nil {
			return err
		}
		raw, _ := json.Marshal(cfg)
		if err := s.DB.SetSetting(ctx, p2pSettingKey, string(raw)); err != nil {
			return err
		}
	}
	if err := s.p2p.devices.load(ctx, s); err != nil {
		return err
	}
	nc := p2p.NodeConfig{
		Seed: seed, Port: cfg.Port, Layers: cfg.Layers, StaticRelays: cfg.StaticRelays,
		DevicesOnly: cfg.DevicesOnly, RelayBitrate: int64(cfg.RelayKbps) * 1000,
		Devices: &s.p2p.devices, Enrol: s.p2pEnrol, Handler: s.p2p.handler,
		Log: s.Log.With("cat", "p2p"),
		ConnInfo: func(ctx context.Context, a *p2p.Addr) context.Context {
			return remote.WithP2P(ctx, remote.P2PConn{Addr: a.IP, Device: a.Peer, Relayed: a.Relayed, Relay: a.Relay, Known: a.Known})
		},
	}
	if s.Logs != nil {
		nc.ErrorLog = s.Logs.StdLogger("http", slog.LevelInfo)
	}
	if s.Conns != nil {
		// The same count as every other connection: the limits on how many may
		// be open cover these too.
		nc.Wrap = s.Conns.Wrap
	}
	if cfg.RelayForOthers {
		l := p2p.DefaultRelayLimits()
		l.TotalKbps, l.Kbps, l.MaxHeight = cfg.ShareKbps, cfg.ShareKbps, cfg.ShareMaxHeight
		l.Note = "Another WittyMediaServer owner's home connection."
		nc.RelayService = &l
	}
	for _, r := range cfg.OwnRelays {
		nc.OwnRelays = append(nc.OwnRelays, p2p.OwnRelay{Addr: r.Addr, Key: r.Key})
	}
	nc.Name = s.ServerName(ctx)
	nc.DeviceIDs = func() []string {
		var ids []string
		for _, d := range s.p2p.devices.list() {
			ids = append(ids, d.ID)
		}
		return ids
	}
	if s.p2p.tune != nil {
		s.p2p.tune(&nc)
	}
	node, err := p2p.StartNode(nc)
	if err != nil {
		return fmt.Errorf("starting peer-to-peer connections: %w", err)
	}
	s.p2p.node, s.p2p.started = node, cfg
	s.p2p.nodeRef.Store(node)
	s.p2p.on.Store(true)
	return nil
}

// ---------------------------------------------------------------- devices

// p2pDevice is one device that may connect.
type p2pDevice struct {
	ID       string `json:"id"`
	UserID   int64  `json:"userId"`
	User     string `json:"user,omitempty"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	How      string `json:"how"` // signed-in | invite
	Created  int64  `json:"created"`
	LastSeen int64  `json:"lastSeen"`
	LastPath string `json:"lastPath,omitempty"`
	// Connected and Path are filled in from the running node.
	Connected bool   `json:"connected"`
	Path      string `json:"path,omitempty"`
	// Mine: the device belongs to whoever asked.
	Mine bool `json:"mine"`

	saved int64 // when LastSeen was last written to the database
}

// p2pDevices is the list, kept in memory so that asking "may this key
// connect?" costs no database lookup, however often a stranger asks.
type p2pDevices struct {
	mu     sync.RWMutex
	byID   map[string]*p2pDevice
	srv    *Server
	loaded bool
}

// ensure reads the list from the database the first time it is wanted.
func (d *p2pDevices) ensure(ctx context.Context, s *Server) error {
	d.mu.RLock()
	ok := d.loaded
	d.mu.RUnlock()
	if ok {
		return nil
	}
	return d.load(ctx, s)
}

func (d *p2pDevices) load(ctx context.Context, s *Server) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, name, platform, how, created_at, last_seen, last_path FROM p2p_devices`)
	if err != nil {
		return err
	}
	defer rows.Close()
	m := map[string]*p2pDevice{}
	for rows.Next() {
		v := &p2pDevice{}
		if err := rows.Scan(&v.ID, &v.UserID, &v.Name, &v.Platform, &v.How, &v.Created, &v.LastSeen, &v.LastPath); err != nil {
			return err
		}
		v.saved = v.LastSeen
		m[v.ID] = v
	}
	if err := rows.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	d.byID, d.srv, d.loaded = m, s, true
	d.mu.Unlock()
	return nil
}

// Allowed is asked for every stream a peer opens.
func (d *p2pDevices) Allowed(id string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.byID[id] != nil
}

// Seen notes a connection. The database hears of it at most once a minute
// per device, and not on the goroutine that is serving the stream.
func (d *p2pDevices) Seen(id, path string) {
	now := time.Now().Unix()
	d.mu.Lock()
	v := d.byID[id]
	if v == nil {
		d.mu.Unlock()
		return
	}
	v.LastSeen, v.LastPath = now, path
	write := now-v.saved >= 60
	if write {
		v.saved = now
	}
	srv := d.srv
	d.mu.Unlock()
	if write && srv != nil {
		srv.background("p2p last seen", func(ctx context.Context) {
			if _, err := srv.DB.ExecContext(ctx, `UPDATE p2p_devices SET last_seen = ?, last_path = ? WHERE id = ?`, now, path, id); err != nil {
				srv.Log.WarnContext(ctx, "could not note when a device last connected", "cat", "p2p", "err", err)
			}
		}, "cat", "p2p")
	}
}

func (d *p2pDevices) list() []p2pDevice {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]p2pDevice, 0, len(d.byID))
	for _, v := range d.byID {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Created != out[j].Created {
			return out[i].Created > out[j].Created
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (d *p2pDevices) get(id string) (p2pDevice, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if v := d.byID[id]; v != nil {
		return *v, true
	}
	return p2pDevice{}, false
}

var errP2PTooMany = errors.New("too many devices are on the list already; remove one you no longer use first")

// add puts a device on the list for an account. A key already listed stays
// with the account that has it: nobody takes over someone else's device by
// naming its key.
func (s *Server) p2pAddDevice(ctx context.Context, userID int64, id, name, platform, how string) (p2pDevice, error) {
	id, err := p2p.CheckID(id)
	if err != nil {
		return p2pDevice{}, errors.New("that isn't a device's connection ID")
	}
	if node := s.p2pNode(); node != nil && node.ID() == id {
		return p2pDevice{}, errors.New("that is this server's own ID")
	}
	d := &s.p2p.devices
	if err := d.ensure(ctx, s); err != nil {
		return p2pDevice{}, err
	}
	defer s.p2pDevicesChanged() // after d.mu is let go: deferred calls run last first
	// Names come from the device itself, by invite or code: shown as text, never as anything else.
	name, platform = p2p.CleanName(name, 60), p2p.CleanName(platform, 24)
	if name == "" {
		name = "A device"
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if have := d.byID[id]; have != nil {
		if have.UserID != userID {
			return p2pDevice{}, errors.New("that device is already on the list for another account")
		}
		have.Name, have.Platform = name, platform
		if _, err := s.DB.ExecContext(ctx, `UPDATE p2p_devices SET name = ?, platform = ? WHERE id = ?`, name, platform, id); err != nil {
			return p2pDevice{}, err
		}
		return *have, nil
	}
	mine := 0
	for _, v := range d.byID {
		if v.UserID == userID {
			mine++
		}
	}
	if mine >= p2pMaxDevicesEach || len(d.byID) >= p2pMaxDevices {
		return p2pDevice{}, errP2PTooMany
	}
	v := &p2pDevice{ID: id, UserID: userID, Name: name, Platform: platform, How: how, Created: time.Now().Unix()}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO p2p_devices(id, user_id, name, platform, how, created_at) VALUES(?,?,?,?,?,?)`,
		v.ID, v.UserID, v.Name, v.Platform, v.How, v.Created); err != nil {
		return p2pDevice{}, err
	}
	d.byID[id] = v
	return *v, nil
}

// p2pRemoveDevice takes a device off the list and closes its connections.
func (s *Server) p2pRemoveDevice(ctx context.Context, id string) error {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM p2p_devices WHERE id = ?`, id); err != nil {
		return err
	}
	d := &s.p2p.devices
	d.mu.Lock()
	delete(d.byID, id)
	d.mu.Unlock()
	if node := s.p2pNode(); node != nil {
		node.Kick(id)
	}
	s.p2pDevicesChanged()
	// The device's sign-ins go with it: revoked means out, not out until it
	// finds another way to send the same token.
	if n, err := s.Auth.EndP2PDevice(ctx, id); err != nil {
		s.Log.WarnContext(ctx, "a device was taken off the peer-to-peer list, but its sign-ins couldn't be ended", "cat", "p2p", "device", p2p.ShortID(id), "err", err)
	} else if n > 0 {
		logging.Audit(ctx, s.Log, "the sign-ins of a device taken off the peer-to-peer list were ended", "cat", "p2p", "session", p2p.ConnSession(id), "device", p2p.ShortID(id), "sessions", n)
	}
	return nil
}

// p2pForgetGone drops devices whose account has been deleted (the database
// did so already) from the list in memory, and closes their connections.
func (s *Server) p2pForgetGone(ctx context.Context) {
	d := &s.p2p.devices
	d.mu.RLock()
	loaded := d.loaded
	d.mu.RUnlock()
	if !loaded {
		return
	}
	before := d.list()
	if err := d.load(ctx, s); err != nil {
		return
	}
	node := s.p2pNode()
	for _, v := range before {
		if !d.Allowed(v.ID) && node != nil {
			node.Kick(v.ID)
		}
	}
}

// ---------------------------------------------------------------- invites

// p2pInvite is a link that puts one device on the list, once.
type p2pInvite struct {
	by      int64
	session string
	userID  int64
	expires time.Time
	used    bool
	// spent is shared with the code shown beside this link (api_connect.go):
	// the link, the code over the web port and the code from away are one way
	// in, and whichever is used first ends the others. nil for a link alone.
	spent *atomic.Bool
}

type p2pInvites struct {
	mu   sync.Mutex
	open map[string]*p2pInvite // by the SHA-256 of the secret, which is never kept
}

func p2pSecretHash(secret string) string {
	sum := sha256.Sum256([]byte("wittymedia p2p invite\n" + secret))
	return hex.EncodeToString(sum[:])
}

func (in *p2pInvites) make(by, userID int64, session string) (string, error) {
	return in.makeShared(by, userID, session, nil)
}

// makeShared is make, for a link that is one of several forms of the same way in.
func (in *p2pInvites) makeShared(by, userID int64, session string, spent *atomic.Bool) (string, error) {
	in.mu.Lock()
	defer in.mu.Unlock()
	now := time.Now()
	if in.open == nil {
		in.open = map[string]*p2pInvite{}
	}
	for k, v := range in.open {
		if now.After(v.expires.Add(time.Minute)) {
			delete(in.open, k)
		}
	}
	if len(in.open) >= p2pMaxInvites {
		return "", errPairBusy
	}
	raw := make([]byte, 16) // 128 bits: nothing to guess
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	in.open[p2pSecretHash(secret)] = &p2pInvite{by: by, session: session, userID: userID, expires: now.Add(p2pInviteTTL), spent: spent}
	return secret, nil
}

// take uses an invite, once. It answers the invite, or why not.
func (in *p2pInvites) take(secret string) (*p2pInvite, string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	v := in.open[p2pSecretHash(secret)]
	switch {
	case v == nil:
		return nil, "wrong"
	case v.used:
		return nil, "used"
	case time.Now().After(v.expires):
		return nil, "expired"
	case v.spent != nil && !v.spent.CompareAndSwap(false, true):
		v.used = true
		return nil, "used"
	}
	v.used = true
	return v, ""
}

// p2pEnrol answers a device that shows an invite over the peer-to-peer
// connection: the one thing an unlisted device may ask. A good invite puts
// its key on the list and hands it a sign-in code, which it then redeems at
// /api/v1/pair/redeem like any code typed on a TV, with the same limits.
func (s *Server) p2pEnrol(ctx context.Context, id string, req p2p.EnrolRequest, from netip.Addr) p2p.EnrolResponse {
	fail := func(reason, msg string) p2p.EnrolResponse {
		s.Log.InfoContext(ctx, "peer-to-peer invite refused", "cat", "p2p", "session", p2p.ConnSession(id), "why", reason, "device", p2p.ShortID(id), "from", from.String())
		return p2p.EnrolResponse{Reason: reason, Error: msg}
	}
	in, reason := s.p2p.invites.take(req.Secret)
	switch reason {
	case "wrong":
		return fail(reason, "That invite isn't right. Ask for a new one.")
	case "used":
		return fail(reason, "That invite has already been used. Ask for a new one.")
	case "expired":
		return fail(reason, "That invite has expired. Invites last fifteen minutes: ask for a new one.")
	}
	if !s.stillSignedIn(ctx, in.session, in.by) {
		return fail("expired", "Whoever made that invite has signed out. Ask for a new one.")
	}
	name := p2p.CleanName(req.Name, 60)
	dev, err := s.p2pAddDevice(ctx, in.userID, id, name, req.Platform, "invite")
	if err != nil {
		return fail("refused", err.Error())
	}
	code, err := s.pair.newInvite(in.by, in.userID, in.session)
	if err != nil {
		// On the list, but with no code: the device can still sign in by hand.
		s.Log.WarnContext(ctx, "a device joined by invite, but no sign-in code could be made for it", "cat", "p2p", "session", p2p.ConnSession(id), "err", err)
	}
	u, _ := s.Auth.User(ctx, in.userID)
	who := ""
	if u != nil {
		who = u.Name
	}
	logging.Audit(ctx, s.Log, "a device was added to the peer-to-peer list by invite", "cat", "p2p", "session", p2p.ConnSession(id), "device", p2p.ShortID(id), "name", dev.Name,
		"platform", dev.Platform, "account", who, "from", from.String())
	res := p2p.EnrolResponse{OK: true, ServerName: s.ServerName(ctx)}
	if code != nil {
		res.Code = code.code
	}
	return res
}

// ---------------------------------------------------------------- handlers for every account

// GET /api/p2p: whether peer-to-peer connections are on, and what an app
// needs to use them: the server's connection ID and where it was last
// reachable. Only for signed-in accounts: the ID is how the server is found
// from outside, and nobody needs it who can't sign in. ?device=<id> also says
// whether that device is on the list.
func (s *Server) handleP2PInfo(w http.ResponseWriter, r *http.Request) {
	node := s.p2pNode()
	if node == nil {
		writeJSON(w, 200, map[string]any{"enabled": false})
		return
	}
	cfg := s.loadP2P(r.Context())
	out := map[string]any{"enabled": true, "serverId": node.ID(), "addrs": node.Addrs(), "devicesOnly": cfg.DevicesOnly,
		"layers": cfg.Layers, "staticRelays": cfg.StaticRelays, "serverName": s.ServerName(r.Context())}
	if id := r.URL.Query().Get("device"); id != "" {
		d, ok := s.p2p.devices.get(id)
		out["enrolled"] = ok && d.UserID == userFrom(r).ID
	}
	writeJSON(w, 200, out)
}

func (s *Server) p2pDeviceViews(ctx context.Context, me *auth.User, all bool) []p2pDevice {
	if err := s.p2p.devices.ensure(ctx, s); err != nil {
		return []p2pDevice{}
	}
	live := map[string]string{}
	if node := s.p2pNode(); node != nil {
		for _, c := range node.Status().Devices {
			if live[c.ID] != "direct" {
				live[c.ID] = c.Path
			}
		}
	}
	names := map[int64]string{}
	if all {
		if users, err := s.Auth.Users(ctx); err == nil {
			for _, u := range users {
				names[u.ID] = u.Name
			}
		}
	}
	out := []p2pDevice{}
	for _, d := range s.p2p.devices.list() {
		if d.UserID != me.ID && !all {
			continue
		}
		d.Mine = d.UserID == me.ID
		d.User = names[d.UserID]
		d.Path, d.Connected = live[d.ID], live[d.ID] != ""
		out = append(out, d)
	}
	return out
}

// GET /api/p2p/devices: your devices that may connect; ?all=1 everyone's (admin).
func (s *Server) handleP2PDevices(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	all := qBool(r, "all")
	if all && !me.IsAdmin {
		writeErr(w, 403, "admin only")
		return
	}
	writeJSON(w, 200, s.p2pDeviceViews(r.Context(), me, all))
}

// POST /api/p2p/devices {id, name, platform}: puts the device you are signed
// in on onto the list, under your account. An app does this once, while it
// can reach the server some other way (at home, usually).
func (s *Server) handleP2PAddDevice(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	// A profile kept at home has no use for a way in from away, and isn't given one.
	if !accessFor(me).Can(library.FeatureRemote) {
		writeErr(w, 403, featureRefusal[library.FeatureRemote])
		return
	}
	if auth.IsStreamToken(tokenFrom(r)) || currentToken(r) == "" {
		writeErr(w, 403, "sign in on the device first")
		return
	}
	var b struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Platform string `json:"platform"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON body")
		return
	}
	d, err := s.p2pAddDevice(r.Context(), me.ID, b.ID, b.Name, b.Platform, "signed-in")
	switch {
	case errors.Is(err, errP2PTooMany):
		writeErr(w, 409, err.Error())
		return
	case err != nil:
		writeErr(w, 400, err.Error())
		return
	}
	c := clientFrom(r)
	logging.Audit(r.Context(), s.Log, "a device was added to the peer-to-peer list", "cat", "p2p", "session", p2p.ConnSession(d.ID), "device", p2p.ShortID(d.ID), "name", d.Name,
		"platform", d.Platform, "account", me.Name, "from", c.IP.String(), "where", c.Where.String())
	d.Mine = true
	writeJSON(w, 200, d)
}

// DELETE /api/p2p/devices/{id}: takes a device off the list and closes its
// connections at once. Your own; anyone's for an admin.
func (s *Server) handleP2PRemoveDevice(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	if err := s.p2p.devices.ensure(r.Context(), s); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	d, ok := s.p2p.devices.get(r.PathValue("id"))
	if !ok || (d.UserID != me.ID && !me.IsAdmin) {
		writeErr(w, 404, "no such device") // someone else's looks the same as none
		return
	}
	if err := s.p2pRemoveDevice(r.Context(), d.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	logging.Audit(r.Context(), s.Log, "a device was removed from the peer-to-peer list", "cat", "p2p", "session", p2p.ConnSession(d.ID), "device", p2p.ShortID(d.ID), "name", d.Name, "by", me.Name)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/p2p/invite {userId}: a link (and so a QR code) that puts one
// device on the list and signs it in, for this account or a Kids or Guest
// profile it manages. It works once, for fifteen minutes, and only while
// whoever made it stays signed in.
func (s *Server) handleP2PInvite(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	// A profile kept at home has no use for a way in from away, and isn't given one.
	if !accessFor(me).Can(library.FeatureRemote) {
		writeErr(w, 403, featureRefusal[library.FeatureRemote])
		return
	}
	if !mayPair(me) {
		writeErr(w, 403, "this profile can't sign devices in")
		return
	}
	if !s.p2pPairAllowed(w, r) {
		return
	}
	var b struct {
		UserID int64 `json:"userId"`
		// The inviter's own password (and two-factor code), when the device will be
		// signed in as an administrator: as for /api/pair/approve and /api/p2p/join.
		Password string `json:"password"`
		OTP      string `json:"otp"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON body")
		return
	}
	target, ok := s.pairTarget(r, me, b.UserID)
	if !ok {
		writeErr(w, 403, "you can't sign a device in as that profile")
		return
	}
	node := s.p2pNode()
	if node == nil {
		writeErr(w, 409, "Peer-to-peer connections are off. An administrator can turn them on in Settings, Remote Access.")
		return
	}
	// An invite signs a device in as the target wherever it is: as an administrator only
	// with that administrator's password again, the same check /api/pair/approve makes.
	if !s.confirmAdminGrant(w, r, target, b.Password, b.OTP) {
		return
	}
	secret, err := s.p2p.invites.make(me.ID, target.ID, tokenFrom(r))
	if errors.Is(err, errPairBusy) {
		w.Header().Set("Retry-After", "60")
		writeErr(w, 429, "Too many invites are waiting to be used. Try again in a few minutes.")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	link := p2p.Invite{ServerID: node.ID(), Secret: secret, Name: s.ServerName(r.Context()), Addrs: inviteAddrs(node.Addrs())}.String()
	// The link is a way in for fifteen minutes: it must not sit in a log.
	logging.AddSecret(secret)
	logging.Audit(r.Context(), s.Log, "a peer-to-peer invite was made", "cat", "p2p", "by", me.Name, "for", target.Name)
	writeJSON(w, 200, map[string]any{"link": link, "expiresInSec": int(p2pInviteTTL.Seconds()), "user": target})
}

// inviteAddrs picks the addresses worth the room in a QR code: ones the
// internet can reach first, then relayed ones; never this machine's own.
func inviteAddrs(addrs []string) []string {
	var public, relayed []string
	for _, a := range addrs {
		switch {
		case strings.Contains(a, "/p2p-circuit"):
			relayed = append(relayed, a)
		case strings.HasPrefix(a, "/ip4/127.") || strings.HasPrefix(a, "/ip6/::1/"):
		default:
			public = append(public, a)
		}
	}
	out := append(public, relayed...)
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

// ---------------------------------------------------------------- handlers for the owner

// p2pThirdParty is one outside party the feature talks to, for the page that
// turns it on: who, what they learn, and which switch stops it.
type p2pThirdParty struct {
	Layer string `json:"layer"`
	Who   string `json:"who"`
	Sees  string `json:"sees"`
}

var p2pThirdParties = []p2pThirdParty{
	{"dht", "The public IPFS network: its starting points (bootstrap.libp2p.io, run by the libp2p project's sponsors) and then some dozens of other people's computers that keep its shared address book",
		"This server's connection ID, its IP addresses and port, and that it runs this software. Anyone who has the ID can ask the address book where the server is."},
	{"relay", "Public relays: other people's computers on that network, or other WittyMediaServer owners who offered theirs",
		"The IP addresses of this server and of the device connecting, when, and how much is sent. Not what is sent: that is encrypted between the device and this server."},
	{"portMap", "Your own router, on your home network", "A request to forward one port to this computer. Nothing leaves your home for it."},
	{"ipv6", "Your own router, on your home network", "A request to let connections in to this computer's IPv6 address on one port."},
	{"holePunch", "The device that is connecting, and the relay that introduces the two", "Each side's public IP address and port, which they need to reach each other."},
}

type p2pView struct {
	Settings     p2pSettings     `json:"settings"`
	Running      bool            `json:"running"`
	ServerID     string          `json:"serverId,omitempty"`
	Error        string          `json:"error,omitempty"`
	Status       *p2p.Status     `json:"status,omitempty"`
	Steps        []checkStep     `json:"steps"`
	Verdict      *p2pVerdict     `json:"verdict,omitempty"`
	ThirdParties []p2pThirdParty `json:"thirdParties"`
	Devices      []p2pDevice     `json:"devices"`
}

func (s *Server) p2pView(r *http.Request, applyErr string) p2pView {
	cfg := s.loadP2P(r.Context())
	cfg.OwnRelays = maskRelayKeys(cfg.OwnRelays)
	v := p2pView{Settings: cfg, Error: applyErr, ThirdParties: p2pThirdParties, Steps: []checkStep{}}
	v.Devices = s.p2pDeviceViews(r.Context(), userFrom(r), true)
	if node := s.p2pNode(); node != nil {
		st := node.Status()
		v.Running, v.ServerID, v.Status = true, st.ID, &st
		v.Steps = describeP2P(st, len(v.Devices))
		vd := verdictP2P(st)
		v.Verdict = &vd
	}
	return v
}

// GET /api/p2p/settings: the owner's choices and how every layer is doing.
func (s *Server) handleP2PSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.p2pView(r, ""))
}

// GET /api/p2p/status: only how every layer is doing, for a page that is open.
func (s *Server) handleP2PStatus(w http.ResponseWriter, r *http.Request) {
	node := s.p2pNode()
	if node == nil {
		writeJSON(w, 200, map[string]any{"running": false, "steps": []checkStep{}})
		return
	}
	st := node.Status()
	writeJSON(w, 200, map[string]any{"running": true, "status": st, "steps": describeP2P(st, len(s.p2p.devices.list())), "verdict": verdictP2P(st)})
}

// PUT /api/p2p/settings: changes any of the settings; the rest stay. Turning
// the feature on takes "accept": the page lists who will be contacted, and
// this is the owner saying they read it.
func (s *Server) handlePutP2PSettings(w http.ResponseWriter, r *http.Request) {
	prev := s.loadP2P(r.Context())
	b := struct {
		p2pSettings
		Accept bool `json:"accept"`
	}{p2pSettings: prev}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cur := b.p2pSettings
	if cur.Port == 0 {
		cur.Port = prev.Port
	}
	// A WittyRelay's key isn't sent to the page, so one that comes back empty
	// for the same relay is the one already kept.
	for i, r := range cur.OwnRelays {
		if r.Key != "" {
			continue
		}
		for _, p := range prev.OwnRelays {
			if strings.TrimSpace(p.Addr) == strings.TrimSpace(r.Addr) && r.HasKey {
				cur.OwnRelays[i].Key = p.Key
			}
		}
	}
	if err := cur.validate(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if cur.Enabled && !prev.Enabled && !b.Accept {
		writeErr(w, 400, "turning this on contacts computers outside your home; accept that first")
		return
	}
	if !cur.DevicesOnly && prev.DevicesOnly && !b.Accept {
		writeErr(w, 400, "letting any device reach the sign-in page needs you to accept what that risks first")
		return
	}
	raw, _ := json.Marshal(cur)
	if err := s.DB.SetSetting(r.Context(), p2pSettingKey, string(raw)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	logging.Audit(r.Context(), s.Log, "peer-to-peer settings changed", "cat", "p2p", "by", userFrom(r).Name, "enabled", cur.Enabled,
		"devicesOnly", cur.DevicesOnly, "dht", cur.Layers.DHT, "relay", cur.Layers.Relay, "holePunch", cur.Layers.HolePunch,
		"portMap", cur.Layers.PortMap, "ipv6", cur.Layers.IPv6, "relayForOthers", cur.RelayForOthers, "ownRelays", len(cur.StaticRelays))
	errText := ""
	if err := s.applyP2P(r.Context()); err != nil {
		s.Log.WarnContext(r.Context(), "could not apply the peer-to-peer settings", "cat", "p2p", "err", err)
		errText = err.Error()
	}
	writeJSON(w, 200, s.p2pView(r, errText))
}

// POST /api/p2p/rotate {password, code}: gives the server a new connection
// key, and so a new ID. Every device has to learn the new ID before it can
// connect from away again (apps do when they next see the server at home; an
// invite works too). For when the ID has gone somewhere it shouldn't have.
func (s *Server) handleP2PRotate(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON body")
		return
	}
	if !s.confirmOwner(w, r, b.Password, b.Code) {
		return
	}
	seed, err := p2p.NewSeed()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	enc := p2p.EncodeSeed(seed)
	logging.KeepSecret(enc)
	if err := s.DB.SetSetting(r.Context(), p2pSeedKey, enc); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	logging.Audit(r.Context(), s.Log, "the server's peer-to-peer key was replaced: devices must learn its new ID before they can connect from away", "cat", "p2p", "by", userFrom(r).Name)
	s.p2p.mu.Lock()
	s.stopP2PLocked()
	s.p2p.mu.Unlock()
	errText := ""
	if err := s.applyP2P(r.Context()); err != nil {
		errText = err.Error()
	}
	writeJSON(w, 200, s.p2pView(r, errText))
}

// maskRelayKeys leaves the keys out, saying only whether there is one.
func maskRelayKeys(list []p2pOwnRelay) []p2pOwnRelay {
	out := make([]p2pOwnRelay, 0, len(list))
	for _, r := range list {
		out = append(out, p2pOwnRelay{Addr: r.Addr, HasKey: r.Key != ""})
	}
	return out
}

// ---------------------------------------------------------------- in plain words

func shortAddrs(list []string, n int) string {
	var out []string
	for _, a := range list {
		if strings.Contains(a, "/p2p-circuit") {
			continue
		}
		out = append(out, a)
	}
	if len(out) > n {
		out = append(out[:n], fmt.Sprintf("and %d more", len(out)-n))
	}
	return strings.Join(out, ", ")
}

func hasIPv6(list []string) bool {
	for _, a := range list {
		if strings.HasPrefix(a, "/ip6/") && !strings.Contains(a, "/p2p-circuit") && !strings.HasPrefix(a, "/ip6/::1/") && !strings.HasPrefix(a, "/ip6/fe80") {
			return true
		}
	}
	return false
}

// describeP2P says how each layer is doing, in the order an app tries them.
func describeP2P(st p2p.Status, devices int) []checkStep {
	steps := []checkStep{}
	add := func(name, state, detail string) { steps = append(steps, checkStep{name, state, detail}) }

	add("At home", "ok", "On your home network apps keep connecting the way they always have. Nothing here changes that.")

	// Reached from outside: what other computers proved by connecting back.
	switch {
	case len(st.Reachable) > 0:
		add("Reachable from outside", "ok", "Other computers on the internet connected straight to this server at "+shortAddrs(st.Reachable, 3)+". Apps can connect directly.")
	case st.Reachability == "private" || len(st.Unreachable) > 0:
		add("Reachable from outside", "warn", "Computers on the internet tried to connect straight to this server and couldn't. That is normal behind a home router. Apps then connect by hole punching, which works with most routers.")
	default:
		add("Reachable from outside", "wait", "Not known yet. It takes a few minutes after turning this on for other computers to try.")
	}

	// IPv6.
	switch {
	case !st.Layers.IPv6:
		add("IPv6", "off", "Turned off: the router isn't asked to let IPv6 connections in.")
	case !hasIPv6(st.Addrs):
		add("IPv6", "off", "This computer has no IPv6 address the internet can reach, so there is nothing to open.")
	default:
		open, why := 0, ""
		for _, p := range st.Mapping.Pinholes {
			if p.Working {
				open++
			} else if why == "" {
				why = p.Error
			}
		}
		switch {
		case open > 0:
			add("IPv6", "ok", "This computer has an IPv6 address, and your router agreed to let connections in to it on port "+fmt.Sprint(st.Port)+".")
		case hasIPv6(st.Reachable):
			add("IPv6", "ok", "This computer has an IPv6 address, and it was reached from outside.")
		default:
			if why == "" {
				why = "the router offers no way to ask"
			}
			add("IPv6", "warn", "This computer has an IPv6 address, but your router's firewall wasn't opened for it ("+why+"). Devices that also have IPv6 can usually still get through by hole punching.")
		}
	}

	// The router's port mapping.
	m := st.Mapping
	switch {
	case !st.Layers.PortMap:
		add("Port on your router", "off", "Turned off: the router isn't asked to forward a port.")
	case m.Shared:
		add("Port on your router", "warn", "Your router forwarded the port, but its own outside address ("+m.ExternalIP+") isn't a public one. Either your internet provider shares one address between customers (CGNAT), or there is a second router in front of this one. A forwarded port can't be reached like that, so it isn't used. Hole punching is.")
	case m.Usable():
		add("Port on your router", "ok", fmt.Sprintf("Your router forwards port %d (UDP and TCP) to this computer, asked by %s. It is renewed every half hour and removed when you turn this off. Outside address: %s.", st.Port, m.Protocol, m.ExternalIP))
	case m.On && m.Renewed == 0 && m.Error == "" && len(m.Mappings) == 0:
		add("Port on your router", "wait", "Asking your router.")
	default:
		why := m.Error
		for _, x := range m.Mappings {
			if why == "" && x.Error != "" {
				why = x.Error
			}
		}
		if why == "" {
			why = "no answer"
		}
		add("Port on your router", "warn", "Your router didn't forward a port ("+why+"). Many routers have this (UPnP, NAT-PMP or PCP) turned off, which is a fair choice. Hole punching doesn't need it.")
	}

	// The kind of router, which decides whether hole punching can work.
	switch {
	case !st.Layers.HolePunch:
		add("Hole punching", "off", "Turned off. Apps can then only connect directly (IPv6, or a forwarded port) or through a community relay.")
	case st.NATUDP == "open" || st.NATTCP == "open":
		add("Hole punching", "ok", "Your router uses the same outside port whoever it talks to, which is what hole punching needs. It works when the network the app is on does the same, as most home and mobile networks do.")
	case st.NATUDP == "strict" && st.NATTCP != "open":
		add("Hole punching", "warn", "Your router picks a different outside port for every destination (a \"symmetric\" NAT). Hole punching usually fails with those. A forwarded port, IPv6 or a community relay is needed instead.")
	default:
		add("Hole punching", "wait", "On. What kind of router this is isn't known yet; it shows after a few connections.")
	}

	// The address book.
	switch {
	case !st.Layers.DHT:
		add("Public address book", "off", "Turned off: this server isn't announced, and apps can only use addresses they already know.")
	case st.DHTPeers > 0:
		add("Public address book", "ok", fmt.Sprintf("Connected to the public IPFS network (%d of its computers known). Apps find this server there by its ID.", st.DHTPeers))
	default:
		add("Public address book", "warn", "The public IPFS network hasn't been reached yet. If this stays, something blocks outgoing connections. Apps that already know an address still connect.")
	}

	// Relays.
	community := 0
	for _, r := range st.Relays {
		if r.Community {
			community++
		}
	}
	switch {
	case !st.Layers.Relay:
		add("Relays", "off", "Turned off: no relay is used, so no hole punching either. Only direct connections work.")
	case len(st.Reachable) > 0 && len(st.Relays) == 0:
		add("Relays", "ok", "Not needed: this server can be reached directly.")
	case community > 0:
		add("Relays", "ok", fmt.Sprintf("Holding a place on %d relay(s), %d of them community relays that can carry a whole connection when no direct one can be made. What goes through is encrypted; a relay sees only addresses and amounts.", len(st.Relays), community))
	case len(st.Relays) > 0:
		add("Relays", "ok", fmt.Sprintf("Holding a place on %d public relay(s). They introduce an app to this server so the two can connect directly; they carry too little for anything else. No community relay was found, so where hole punching fails there is no way through.", len(st.Relays)))
	default:
		add("Relays", "wait", "Looking for a relay. This takes up to a minute after turning on.")
	}
	if st.RelayService {
		add("Helping others", "ok", "This server offers itself as a community relay while it can be reached from outside: up to 16 other servers, 2 GiB per connection.")
	}

	connected := map[string]bool{}
	for _, d := range st.Devices {
		connected[d.ID] = true
	}
	switch {
	case devices == 0:
		add("Devices", "warn", "No device is on the list yet. Open the app on a device while it is at home and signed in, or make an invite below.")
	default:
		add("Devices", "ok", fmt.Sprintf("%d on the list, %d connected now.", devices, len(connected)))
	}
	return steps
}

// ---------------------------------------------------------------- the connection test

// POST /api/p2p/test: connects to this server the way an app away from home
// would, from a second node made for the test: finds it by its ID, connects
// through whatever path there is, and sends a request. The test node sits on
// this same computer, so a direct connection it makes says little; what
// counts is that the server was found and answered, and what other computers
// said about reaching it (the first lines).
func (s *Server) handleP2PTest(w http.ResponseWriter, r *http.Request) {
	node := s.p2pNode()
	if node == nil {
		writeErr(w, 409, "Peer-to-peer connections are off.")
		return
	}
	if !s.p2p.testing.TryLock() {
		writeErr(w, 429, "A test is already running.")
		return
	}
	defer s.p2p.testing.Unlock()
	// One session for the whole test, so the viewer can follow it with what the
	// node said while the test's own device dialled in.
	ctx := logging.WithSession(r.Context(), logging.NewID("p2p"))
	s.Log.InfoContext(ctx, "peer-to-peer connection test started", "cat", "p2p", "by", userFrom(r).Name)
	steps := s.p2pSelfTest(ctx, node)
	st := node.Status()
	failed := 0
	for _, step := range steps {
		lv := slog.LevelInfo
		if step.State != "ok" {
			lv = slog.LevelWarn
			if step.State == "fail" {
				failed++
			}
		}
		s.Log.Log(ctx, lv, "peer-to-peer test step", "cat", "p2p", "step", step.Name, "state", step.State, "detail", step.Detail)
	}
	s.Log.InfoContext(ctx, "peer-to-peer connection test finished", "cat", "p2p", "steps", len(steps), "failed", failed,
		"reachability", st.Reachability, "natUdp", st.NATUDP, "natTcp", st.NATTCP, "relays", len(st.Relays), "portMapped", st.Mapping.On, "mappingProtocol", st.Mapping.Protocol, "sharedAddress", st.Mapping.Shared)
	writeJSON(w, 200, map[string]any{"steps": steps, "status": st, "layers": describeP2P(st, len(s.p2p.devices.list())), "verdict": verdictP2P(st)})
}

func (s *Server) p2pSelfTest(ctx context.Context, node *p2p.Node) []checkStep {
	steps := []checkStep{}
	add := func(name, state, detail string) { steps = append(steps, checkStep{name, state, detail}) }
	cfg := s.loadP2P(ctx)
	seed, err := p2p.NewSeed()
	if err != nil {
		add("Test", "fail", err.Error())
		return steps
	}
	cc := p2p.ClientConfig{Seed: seed, ServerID: node.ID(), Layers: cfg.Layers, StaticRelays: cfg.StaticRelays, UpgradeWait: 12 * time.Second}
	if !cfg.Layers.DHT {
		// With no address book, an app only has the addresses it was given.
		cc.Hints = node.Addrs()
	}
	if s.p2p.tune != nil {
		// A test server's node lives on this machine only, and so must this one.
		nc := p2p.NodeConfig{}
		s.p2p.tune(&nc)
		cc.Local, cc.Bootstrap, cc.Listen, cc.Hints = nc.Local, nc.Bootstrap, nc.Listen, node.Addrs()
	}
	client, err := p2p.NewClient(cc)
	if err != nil {
		add("Test", "fail", "The test couldn't start: "+err.Error())
		return steps
	}
	defer client.Close()
	// The test's key is let in for two minutes and then forgotten.
	node.AllowBriefly(client.ID(), 2*time.Minute)

	deadline := time.Now().Add(75 * time.Second)
	var state p2p.State
	for time.Now().Before(deadline) && ctx.Err() == nil {
		if state = client.State(); state.Connected() {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	how := "the address book"
	if !cfg.Layers.DHT {
		how = "its known addresses"
	}
	if !state.Connected() {
		why := state.Error
		if why == "" {
			why = "it wasn't found in time"
		}
		add("Finding this server", "fail", "A second node on this computer looked for this server by its ID through "+how+" and couldn't connect: "+why+". If you only just turned this on, wait two minutes and try again.")
		return steps
	}
	add("Finding this server", "ok", "A second node on this computer found this server by its ID through "+how+".")
	switch state.Path {
	case p2p.PathRelayed:
		kind := "a public relay"
		if state.Community {
			kind = "a community relay"
		}
		add("Connecting", "ok", "It connected through "+kind+", encrypted to this server's key. This is how an app away from home starts, before it tries for a direct connection.")
	default:
		add("Connecting", "ok", "It connected directly ("+state.Transport+"). Being on the same computer, that says nothing about your router.")
	}
	// A request through it, if the path can carry one.
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://server/api/health", nil)
	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	res, err := client.Transport().RoundTrip(req.WithContext(rctx))
	switch {
	case err == nil:
		res.Body.Close()
		add("A request", "ok", fmt.Sprintf("The server answered a request over the connection (%d), and treated it as coming from away from home.", res.StatusCode))
	case errors.Is(err, p2p.ErrNoDirectPath):
		add("A request", "warn", "Through a public relay only the introduction is possible, and from this same computer a direct connection usually can't be made (most routers don't loop back). So this part can't be tested from here: try the app on mobile data.")
	default:
		add("A request", "fail", "The server was reached, but a request over the connection failed: "+err.Error())
	}
	return steps
}

// ---------------------------------------------------------------- the answer in one line

// p2pVerdict is the connection test's answer for someone who doesn't want
// to read about NAT: what will happen when a device is away from home, and
// what they can do about it.
type p2pVerdict struct {
	State  string   `json:"state"` // ok | warn | fail | wait
	Title  string   `json:"title"`
	Detail string   `json:"detail"`
	Todo   []string `json:"todo"`
}

// verdictP2P reads the node's status. Nothing in it needs the network.
func verdictP2P(st p2p.Status) p2pVerdict {
	var dataRelay *p2p.RelayStatus
	for i, r := range st.Relays {
		if r.Community {
			dataRelay = &st.Relays[i]
			break
		}
	}
	ownOK, ownBad := 0, ""
	for _, r := range st.OwnRelays {
		if r.State == "ok" {
			ownOK++
		} else if r.Error != "" && ownBad == "" {
			ownBad = r.Error
		}
	}
	relayWords := func() string {
		if dataRelay != nil && dataRelay.Allows != "" {
			return " It allows: " + strings.ToLower(dataRelay.Allows[:1]) + dataRelay.Allows[1:]
		}
		return ""
	}
	direct := len(st.Reachable) > 0 || (st.Mapping.Usable() && len(st.Unreachable) == 0)
	punch := st.NATUDP == "open" || st.NATTCP == "open"
	strict := (st.NATUDP == "strict" && st.NATTCP != "open") || (st.Mapping.Shared && !punch)
	haveRelay := dataRelay != nil || ownOK > 0
	const addRelay = `For those, run your own WittyRelay (a few minutes with Docker: see Remote access, "Your own WittyRelay") and add it under "Your own WittyRelay" below.`
	v := p2pVerdict{Todo: []string{}}
	switch {
	case !st.Layers.Relay && !direct:
		v = p2pVerdict{State: "warn", Title: "Only direct connections are allowed, and none has worked yet",
			Detail: "Relays are turned off, so hole punching is too. Devices away from home can only connect if your router forwards a port or lets IPv6 in.",
			Todo:   []string{"Turn Relays on under Peer-to-peer options. Public relays carry only the introduction; a relay of your own carries more."}}
	case direct:
		v = p2pVerdict{State: "ok", Title: "Direct connection works",
			Detail: "Other computers on the internet connected straight to this server. Your devices will too, from any network.", Todo: []string{}}
	case punch:
		v = p2pVerdict{State: "ok", Title: "Direct connection works", Todo: []string{},
			Detail: "Your router lets a device and this server connect straight to each other (hole punching). That works from most home and mobile networks. A strict network on the device's side (some offices, schools and hotels) can still stop it."}
		if haveRelay {
			v.Detail += " On those, your devices go through your relay." + relayWords()
		} else {
			v.Todo = append(v.Todo, addRelay)
		}
	case strict && haveRelay:
		v = p2pVerdict{State: "ok", Title: "Connections will go through your relay",
			Detail: "Your network blocks direct connections from outside, so devices away from home connect through your relay. It can't see what it carries: that's encrypted between the device and this server." + relayWords(),
			Todo:   []string{"For direct connections as well, turn on UPnP or NAT-PMP in your router's settings, then test again."}}
	case strict:
		v = p2pVerdict{State: "warn", Title: "Your network blocks direct connections; we'll use a relay when WittyRelay is available",
			Detail: "Your router (or your internet provider, if it shares one address between customers) stops devices outside from connecting straight in, and hole punching can't get through it. The public relays only introduce devices; they don't carry films.",
			Todo: []string{
				"Look for UPnP or NAT-PMP in your router's settings and turn it on, then test again.",
				`Or run your own WittyRelay and add it under "Your own WittyRelay" below: see Remote access, "Your own WittyRelay".`,
				`Or use a private network instead: see Remote access, "The recommended way".`,
			}}
	case !st.Layers.DHT && len(st.Relays) == 0 && ownOK == 0:
		v = p2pVerdict{State: "warn", Title: "Devices can only use addresses they already know",
			Detail: "The public address book is off and no relay is held, so a device that is away finds this server only at an address it remembers.",
			Todo:   []string{"Turn the Public address book on under Peer-to-peer options."}}
	case st.DHTPeers == 0 && len(st.Relays) == 0 && ownOK == 0 && time.Since(time.Unix(st.Started, 0)) > 3*time.Minute:
		v = p2pVerdict{State: "fail", Title: "This server can't reach the internet for peer-to-peer connections",
			Detail: "Nothing outside answered: not the public address book, not a relay. A firewall on this computer or the network may block outgoing UDP and TCP.",
			Todo:   []string{"Check that this computer's firewall lets WittyMediaServer connect out.", "On an office or school network this may not be allowed: use a private network instead."}}
	default:
		v = p2pVerdict{State: "wait", Title: "Still finding out", Todo: []string{},
			Detail: "It takes a few minutes after turning this on for other computers to try connecting to this server. Test again in five minutes."}
	}
	if ownBad != "" {
		v.Todo = append(v.Todo, "Your WittyRelay isn't working: "+ownBad)
	}
	return v
}

// POST /api/p2p/relay-check {addr}: what a WittyRelay allows, before the
// owner adds it. It asks the relay and nobody else, from a key made for
// the purpose, and registers nothing.
func (s *Server) handleP2PRelayCheck(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Addr string `json:"addr"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON body")
		return
	}
	if _, err := p2p.ParseRelayAddr(b.Addr); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !s.p2p.testing.TryLock() {
		writeErr(w, 429, "A test is already running.")
		return
	}
	defer s.p2p.testing.Unlock()
	o, err := p2p.CheckRelay(r.Context(), b.Addr, s.p2p.relayCheckOpts...)
	s.Log.InfoContext(r.Context(), "a WittyRelay was checked", "cat", "p2p", "by", userFrom(r).Name, "ok", err == nil)
	if err != nil {
		res := map[string]any{"ok": false, "error": err.Error()}
		if o.V != 0 {
			res["offer"], res["allows"] = o, o.Words()
		}
		writeJSON(w, 200, res)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "offer": o, "allows": o.Words()})
}
