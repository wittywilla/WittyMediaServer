package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymoonlight"
	"github.com/wittywilla/wittymoonlight/gamestream"
	"github.com/wittywilla/wittymoonlight/relay"
)

// WittyMoonlight: a Moonlight client in the page, for connecting to any Sunshine server. See
// docs/GAME-RIGS.md. This makes the server connect where a person says, which needs care:
//
//   - The connections only speak this one protocol, on its own ports, to fixed paths. Answers are
//     parsed into a few fields, and nothing a far end says is passed back raw, so this can't be
//     used to read other services or as a general network probe.
//   - What was refused and what timed out read the same to anyone but an admin, as the
//     difference is what a port scanner wants.
//   - Attempts are limited per person, and nobody but an admin reaches this machine itself.
//   - Each person's certificate and pairings are their own, kept apart, and never shown.
//   - Who may use it, and which networks it may reach, are an admin's settings.

const (
	moonlightAttemptsWindow = 10 * time.Minute
	moonlightAttemptsLimit  = 20
	moonlightHostLimit      = 32
)

func init() {
	settingKeys = append(settingKeys, "moonlightAccess", "moonlightPrivate", "moonlightInternet", "moonlightCustomPorts", "moonlightMaxStreams", "moonlightUdpPort", "moonlightPublicIp", "moonlightStun")
	settingDefaults["moonlightAccess"] = "adults"
	settingDefaults["moonlightPrivate"] = "1"
	settingDefaults["moonlightInternet"] = "1"
	settingDefaults["moonlightCustomPorts"] = "0"
	settingDefaults["moonlightMaxStreams"] = "2"
}

func (s *Server) moonlightRoutes(m *http.ServeMux) {
	a := s.requireUser
	m.Handle("GET /api/moonlight", a(s.handleMoonlight))
	m.Handle("GET /api/moonlight/find", a(s.handleMoonlightFind))
	m.Handle("GET /api/moonlight/hosts", a(s.handleMoonlightHosts))
	m.Handle("POST /api/moonlight/hosts", a(s.handleMoonlightAdd))
	m.Handle("PATCH /api/moonlight/hosts/{id}", a(s.handleMoonlightRename))
	m.Handle("DELETE /api/moonlight/hosts/{id}", a(s.handleMoonlightRemove))
	m.Handle("POST /api/moonlight/hosts/{id}/pair", a(s.handleMoonlightPair))
	m.Handle("GET /api/moonlight/hosts/{id}/pair", a(s.handleMoonlightPairStatus))
	m.Handle("DELETE /api/moonlight/hosts/{id}/pair", a(s.handleMoonlightPairCancel))
	m.Handle("GET /api/moonlight/hosts/{id}/apps", a(s.handleMoonlightApps))
	m.Handle("GET /api/moonlight/hosts/{id}/apps/{app}/art", a(s.handleMoonlightArt))
	m.Handle("POST /api/moonlight/hosts/{id}/apps/{app}/play", a(s.handleMoonlightPlay))
	m.Handle("POST /api/moonlight/hosts/{id}/wake", a(s.handleMoonlightWake))
	m.Handle("POST /api/moonlight/hosts/{id}/quit", a(s.handleMoonlightQuit))
	m.Handle("GET /api/moonlight/sessions/{sid}", a(s.handleMoonlightSession))
	m.Handle("POST /api/moonlight/sessions/{sid}/webrtc", a(s.handleMoonlightWebRTC))
	m.Handle("DELETE /api/moonlight/sessions/{sid}", a(s.handleMoonlightSessionEnd))
	m.Handle("POST /api/rigs/games/{id}/stream", a(s.handleRigStream))
}

// moonlightService holds what's in memory, and starts up the first time it's needed.
type moonlightService struct {
	dir string

	mu         sync.Mutex
	identities map[int64]*gamestream.Identity
	attempts   map[int64][]time.Time
	statuses   map[int64]rigCached
	pairs      map[int64]*rigPairing
	sessions   map[string]*moonlightStream
	apps       map[int64]moonlightAppsCache
	wakes      map[int64]time.Time
	finding    map[int64]bool
	checked    string
	checkedAt  time.Time
	checkOK    bool
}

type moonlightAppsCache struct {
	at   time.Time
	apps []moonlightApp
}

// moonlightStream is one stream, and who it belongs to.
type moonlightStream struct {
	ID      string
	User    int64
	HostID  int64
	RigID   int64
	AppID   int
	Title   string
	Client  *gamestream.Rig
	Session *relay.Session
}

func (s *Server) moonlight() *moonlightService {
	s.moonOnce.Do(func() {
		s.moonSvc = &moonlightService{dir: filepath.Join(s.Cfg.DataDir, "wittymoonlight"), identities: map[int64]*gamestream.Identity{}, attempts: map[int64][]time.Time{},
			statuses: map[int64]rigCached{}, pairs: map[int64]*rigPairing{}, sessions: map[string]*moonlightStream{}, apps: map[int64]moonlightAppsCache{}, wakes: map[int64]time.Time{}, finding: map[int64]bool{}}
		s.pruneMoonlightIdentities()
	})
	return s.moonSvc
}

// pruneMoonlightIdentities removes the keys of accounts that no longer exist. A key that nobody
// can use is only a risk.
func (s *Server) pruneMoonlightIdentities() {
	entries, err := os.ReadDir(filepath.Join(s.moonSvc.dir, "identities"))
	if err != nil {
		return
	}
	for _, e := range entries {
		id, err := strconv.ParseInt(strings.TrimPrefix(e.Name(), "u"), 10, 64)
		if err != nil {
			continue
		}
		if _, err := s.Auth.User(context.Background(), id); errors.Is(err, sql.ErrNoRows) {
			_ = os.RemoveAll(filepath.Join(s.moonSvc.dir, "identities", e.Name()))
		}
	}
}

// StopMoonlight ends every stream. The server is shutting down.
func (s *Server) StopMoonlight() {
	if s.moonSvc == nil {
		return
	}
	s.moonSvc.mu.Lock()
	list := make([]*moonlightStream, 0, len(s.moonSvc.sessions))
	for _, st := range s.moonSvc.sessions {
		list = append(list, st)
	}
	for _, p := range s.moonSvc.pairs {
		if p.cancel != nil {
			p.cancel()
		}
	}
	s.moonSvc.mu.Unlock()
	for _, st := range list {
		st.Session.Close()
	}
}

// moonlightAllowed: admins always. Otherwise it's an admin's setting, and an account with limits
// set on it doesn't get it, since a PC's desktop is outside anything those limits can reach.
func (s *Server) moonlightAllowed(ctx context.Context, u *auth.User) bool {
	if u == nil {
		return false
	}
	if u.IsAdmin {
		return true
	}
	switch s.DB.GetSetting(ctx, "moonlightAccess", settingDefaults["moonlightAccess"]) {
	case "everyone":
		return true
	case "adults":
		if u.Kind != auth.KindAdult && u.Kind != "" {
			return false
		}
		a := accessFor(u)
		return a == nil || !a.Limited()
	}
	return false
}

// moonlightUser checks the request and answers for it if it isn't allowed.
func (s *Server) moonlightUser(w http.ResponseWriter, r *http.Request) *auth.User {
	u := userFrom(r)
	if !s.moonlightAllowed(r.Context(), u) {
		writeErr(w, 403, "Connecting to game PCs isn't turned on for this account. Ask whoever runs this server.")
		return nil
	}
	return u
}

// moonlightPolicy is where this person's connections may go.
func (s *Server) moonlightPolicy(ctx context.Context, u *auth.User) gamestream.Policy {
	on := func(k string) bool { return s.DB.GetSetting(ctx, k, settingDefaults[k]) == "1" }
	if u.IsAdmin {
		return gamestream.Policy{AllowPublic: true}
	}
	return gamestream.Policy{AllowPublic: on("moonlightInternet"), DenyPrivate: !on("moonlightPrivate"), DenyLoopback: true}
}

// moonlightAttempt counts a new connection attempt against this person's limit.
func (s *Server) moonlightAttempt(w http.ResponseWriter, u *auth.User) bool {
	if u.IsAdmin {
		return true
	}
	g := s.moonlight()
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	kept := g.attempts[u.ID][:0]
	for _, t := range g.attempts[u.ID] {
		if now.Sub(t) < moonlightAttemptsWindow {
			kept = append(kept, t)
		}
	}
	if len(kept) >= moonlightAttemptsLimit {
		g.attempts[u.ID] = kept
		w.Header().Set("Retry-After", "600")
		writeErr(w, 429, "That's a lot of connection attempts. Try again in a few minutes.")
		return false
	}
	g.attempts[u.ID] = append(kept, now)
	return true
}

func (s *Server) moonlightIdentity(u int64) (*gamestream.Identity, error) {
	g := s.moonlight()
	g.mu.Lock()
	defer g.mu.Unlock()
	if id := g.identities[u]; id != nil {
		return id, nil
	}
	id, err := gamestream.LoadOrCreateIdentity(filepath.Join(g.dir, "identities", "u"+strconv.FormatInt(u, 10)))
	if err != nil {
		return nil, fmt.Errorf("this account's game PC certificate can't be read or made: %w", err)
	}
	g.identities[u] = id
	return id, nil
}

// moonlightProgram finds the host program: named by the environment, in this server's data
// folder, or beside the server. It's a separate GPL-3.0 program, so a server works without it.
func (s *Server) moonlightProgram() string {
	if p := os.Getenv("WMS_WITTYMOONLIGHT_HOST"); p != "" {
		return p
	}
	name := "wittymoonlight-host"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if p := filepath.Join(s.moonlight().dir, "bin", name); fileExists(p) {
		return p
	}
	return findProgram("wittymoonlight-host")
}

// moonlightProgramWorks runs the program to see that it can run here, at most once a minute.
func (s *Server) moonlightProgramWorks(ctx context.Context, program string) bool {
	if program == "" {
		return false
	}
	g := s.moonlight()
	g.mu.Lock()
	cached := time.Since(g.checkedAt) < time.Minute && g.checked == program
	ok := g.checkOK
	g.mu.Unlock()
	if cached {
		return ok
	}
	ok = relay.CheckProgram(ctx, program) == nil
	g.mu.Lock()
	g.checked, g.checkedAt, g.checkOK = program, time.Now(), ok
	g.mu.Unlock()
	return ok
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// moonlightHost is a PC as it's stored, for one person.
type moonlightHost struct {
	ID, UserID int64
	Name       string
	Address    string
	Port       int
	UUID, MAC  string
	Cert       string
	Public     bool
}

const moonlightCols = `id, user_id, name, address, port, uuid, mac, cert, public`

func scanMoonlightHost(row interface{ Scan(...any) error }) (*moonlightHost, error) {
	var h moonlightHost
	if err := row.Scan(&h.ID, &h.UserID, &h.Name, &h.Address, &h.Port, &h.UUID, &h.MAC, &h.Cert, &h.Public); err != nil {
		return nil, err
	}
	return &h, nil
}

// moonlightHostFor loads a PC in the path. Someone else's answers "not found", as if it isn't there.
func (s *Server) moonlightHostFor(w http.ResponseWriter, r *http.Request, u *auth.User) *moonlightHost {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil
	}
	h, err := scanMoonlightHost(s.DB.QueryRowContext(r.Context(), `SELECT `+moonlightCols+` FROM wittymoonlight_hosts WHERE id = ? AND user_id = ?`, id, u.ID))
	if err != nil {
		writeErr(w, 404, "not found")
		return nil
	}
	return h
}

func (s *Server) moonlightClient(ctx context.Context, u *auth.User, h *moonlightHost) (*gamestream.Rig, error) {
	id, err := s.moonlightIdentity(u.ID)
	if err != nil {
		return nil, err
	}
	return &gamestream.Rig{Address: h.Address, Port: h.Port, Identity: id, Cert: []byte(h.Cert), Policy: s.moonlightPolicy(ctx, u)}, nil
}

// moonlightHostView is a PC as its owner sees it.
type moonlightHostView struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	Port        int    `json:"port,omitempty"`
	Paired      bool   `json:"paired"`
	State       string `json:"state"`
	Message     string `json:"message"`
	CurrentGame string `json:"currentGame,omitempty"`
	CanWake     bool   `json:"canWake"`
	Public      bool   `json:"public,omitempty"`
}

func (s *Server) moonlightStatus(ctx context.Context, u *auth.User, h *moonlightHost) rigState {
	g := s.moonlight()
	g.mu.Lock()
	c, ok := g.statuses[h.ID]
	g.mu.Unlock()
	if ok && time.Since(c.at) < rigStatusLife {
		return c.view
	}
	var view rigState
	client, err := s.moonlightClient(ctx, u, h)
	if err != nil {
		view = rigState{State: "error", Message: "This account's certificate couldn't be read."}
	} else if h.Cert == "" {
		info, err := client.Hello(ctx)
		view = statusFor(false, info, err)
	} else {
		info, err := client.Status(ctx)
		view = statusFor(true, info, err)
		if err == nil && info.MAC != h.MAC {
			if _, merr := gamestream.ParseMAC(info.MAC); merr == nil {
				_, _ = s.DB.ExecContext(ctx, `UPDATE wittymoonlight_hosts SET mac = ? WHERE id = ?`, info.MAC, h.ID)
				h.MAC = info.MAC
			}
		}
	}
	if !u.IsAdmin && (view.State == "asleep" || view.State == "no-sunshine") {
		// Whether something refused or never answered is how ports get scanned.
		view = rigState{State: "asleep", Message: "This PC isn't answering. It may be asleep or off, or Sunshine may not be running."}
	}
	if view.currentApp > 0 {
		g.mu.Lock()
		for _, a := range g.apps[h.ID].apps {
			if a.ID == view.currentApp {
				view.CurrentGame = a.Title
			}
		}
		g.mu.Unlock()
	}
	g.mu.Lock()
	g.statuses[h.ID] = rigCached{at: time.Now(), view: view}
	g.mu.Unlock()
	return view
}

func (s *Server) moonlightViewOf(ctx context.Context, u *auth.User, h *moonlightHost) moonlightHostView {
	st := s.moonlightStatus(ctx, u, h)
	return moonlightHostView{ID: h.ID, Name: h.Name, Address: h.Address, Port: h.Port, Paired: h.Cert != "", State: st.State, Message: st.Message,
		CurrentGame: st.CurrentGame, CanWake: h.MAC != "" && st.State == "asleep", Public: h.Public}
}

// GET /api/moonlight: whether this account may use this, whether this server can, and the settings
// a person needs to know.
func (s *Server) handleMoonlight(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(r)
	program := s.moonlightProgram()
	ready := s.moonlightProgramWorks(ctx, program)
	body := map[string]any{"allowed": s.moonlightAllowed(ctx, u), "ready": ready, "credits": wittymoonlight.Credits()}
	if u.IsAdmin {
		body["program"] = program != ""
		body["access"] = s.DB.GetSetting(ctx, "moonlightAccess", settingDefaults["moonlightAccess"])
	}
	writeJSON(w, 200, body)
}

// GET /api/moonlight/find: who on this server's network runs Sunshine.
func (s *Server) handleMoonlightFind(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := s.moonlightUser(w, r)
	if u == nil {
		return
	}
	policy := s.moonlightPolicy(ctx, u)
	if policy.DenyPrivate {
		writeJSON(w, 200, []foundRig{})
		return
	}
	g := s.moonlight()
	g.mu.Lock()
	busy := g.finding[u.ID]
	if !busy {
		g.finding[u.ID] = true
	}
	g.mu.Unlock()
	if busy {
		writeErr(w, 409, "Already looking. Try again in a moment.")
		return
	}
	defer func() {
		g.mu.Lock()
		delete(g.finding, u.ID)
		g.mu.Unlock()
	}()
	if !s.moonlightAttempt(w, u) {
		return
	}
	found, err := gamestream.Browse(ctx, 3*time.Second)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var mine []*moonlightHost
	if rows, err := s.DB.QueryContext(ctx, `SELECT `+moonlightCols+` FROM wittymoonlight_hosts WHERE user_id = ?`, u.ID); err == nil {
		defer rows.Close()
		for rows.Next() {
			if h, err := scanMoonlightHost(rows); err == nil {
				mine = append(mine, h)
			}
		}
	}
	out := []foundRig{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, f := range found {
		wg.Add(1)
		go func() {
			defer wg.Done()
			info, err := (&gamestream.Rig{Address: f.Address, Port: f.Port, Policy: policy}).Hello(ctx)
			if err != nil {
				return
			}
			item := foundRig{Name: info.Name, Address: f.Address, Port: f.Port, Version: info.Version}
			for _, h := range mine {
				if strings.EqualFold(h.UUID, info.UUID) {
					item.Added = true
				}
			}
			mu.Lock()
			out = append(out, item)
			mu.Unlock()
		}()
	}
	wg.Wait()
	writeJSON(w, 200, out)
}

// GET /api/moonlight/hosts: this person's PCs, and how they are.
func (s *Server) handleMoonlightHosts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := s.moonlightUser(w, r)
	if u == nil {
		return
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+moonlightCols+` FROM wittymoonlight_hosts WHERE user_id = ? ORDER BY name COLLATE NOCASE`, u.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var hosts []*moonlightHost
	for rows.Next() {
		h, err := scanMoonlightHost(rows)
		if err != nil {
			rows.Close()
			writeErr(w, 500, err.Error())
			return
		}
		hosts = append(hosts, h)
	}
	rows.Close()
	out := make([]moonlightHostView, len(hosts))
	var wg sync.WaitGroup
	for i, h := range hosts {
		wg.Add(1)
		go func() { defer wg.Done(); out[i] = s.moonlightViewOf(ctx, u, h) }()
	}
	wg.Wait()
	writeJSON(w, 200, out)
}

// moonlightCheckAddress applies this person's rules to an address they typed.
func (s *Server) moonlightCheckAddress(ctx context.Context, u *auth.User, address string) (host string, port int, reach gamestream.Reach, msg string) {
	host, port, err := gamestream.CleanAddress(address)
	if err != nil {
		return "", 0, 0, err.Error()
	}
	reach, err = gamestream.CheckAddress(ctx, host, nil)
	if err != nil {
		return "", 0, 0, err.Error()
	}
	if port != 0 && port != gamestream.DefaultPort && !u.IsAdmin && s.DB.GetSetting(ctx, "moonlightCustomPorts", "0") != "1" {
		return "", 0, 0, "This server only connects to Sunshine's usual port. Ask whoever runs it to allow others."
	}
	policy := s.moonlightPolicy(ctx, u)
	switch {
	case reach == gamestream.Loopback && policy.DenyLoopback:
		return "", 0, 0, "That address is this server itself, which this can't connect to."
	case reach == gamestream.Private && policy.DenyPrivate:
		return "", 0, 0, "This server has been set not to connect to PCs on its own network."
	case reach == gamestream.Public && !policy.AllowPublic:
		return "", 0, 0, "This server has been set not to connect to PCs over the internet."
	}
	return host, port, reach, ""
}

// POST /api/moonlight/hosts {address, name}: add a PC. It has to answer as Sunshine.
func (s *Server) handleMoonlightAdd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := s.moonlightUser(w, r)
	if u == nil {
		return
	}
	var b struct {
		Address string `json:"address"`
		Name    string `json:"name"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	host, port, reach, msg := s.moonlightCheckAddress(ctx, u, b.Address)
	if msg != "" {
		writeErr(w, 400, msg)
		return
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM wittymoonlight_hosts WHERE user_id = ?`, u.ID).Scan(&n); err == nil && n >= moonlightHostLimit {
		writeErr(w, 409, "That's as many PCs as one account can have. Remove one first.")
		return
	}
	if !s.moonlightAttempt(w, u) {
		return
	}
	client := &gamestream.Rig{Address: host, Port: port, Policy: s.moonlightPolicy(ctx, u)}
	info, err := client.Hello(ctx)
	if err != nil {
		s.Log.Info("moonlight: a PC didn't answer", "user", u.Name)
		writeErr(w, 502, moonlightConnectMessage(u, err))
		return
	}
	name := strings.TrimSpace(b.Name)
	if name == "" {
		name = info.Name
	}
	if name == "" {
		name = "Game PC"
	}
	if len(name) > 100 {
		name = name[:100]
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO wittymoonlight_hosts(user_id, name, address, port, uuid, public, created_at) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(user_id, uuid) DO UPDATE SET address = excluded.address, port = excluded.port, public = excluded.public`,
		u.ID, name, host, port, info.UUID, reach == gamestream.Public, db.Now())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h, err := scanMoonlightHost(s.DB.QueryRowContext(ctx, `SELECT `+moonlightCols+` FROM wittymoonlight_hosts WHERE user_id = ? AND uuid = ?`, u.ID, info.UUID))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.Info("moonlight: a PC was added", "user", u.Name, "where", reachName(reach))
	writeJSON(w, 201, s.moonlightViewOf(ctx, u, h))
}

func reachName(r gamestream.Reach) string {
	switch r {
	case gamestream.Public:
		return "internet"
	case gamestream.Private:
		return "private"
	case gamestream.Loopback:
		return "this machine"
	}
	return "unknown"
}

// moonlightConnectMessage: an admin gets the reason, and anyone else gets one sentence.
func moonlightConnectMessage(u *auth.User, err error) string {
	if errors.Is(err, gamestream.ErrBlocked) {
		return "That address isn't allowed here."
	}
	if !u.IsAdmin {
		return "That PC didn't answer as a Sunshine server. Check the address, that it's on, and that Sunshine is running."
	}
	switch {
	case errors.Is(err, gamestream.ErrRefused):
		return "That PC is on, but Sunshine isn't running on it."
	case errors.Is(err, gamestream.ErrBadAnswer):
		return "Something answered at that address, but it isn't Sunshine."
	}
	return "That PC didn't answer. Check the address, that it's on, and that Sunshine is running."
}

// PATCH /api/moonlight/hosts/{id} {name}
func (s *Server) handleMoonlightRename(w http.ResponseWriter, r *http.Request) {
	u := s.moonlightUser(w, r)
	if u == nil {
		return
	}
	h := s.moonlightHostFor(w, r, u)
	if h == nil {
		return
	}
	var b struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	name := strings.TrimSpace(b.Name)
	if name == "" || len(name) > 100 {
		writeErr(w, 400, "A name is required, up to 100 characters.")
		return
	}
	if _, err := s.DB.ExecContext(r.Context(), `UPDATE wittymoonlight_hosts SET name = ? WHERE id = ?`, name, h.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.Name = name
	writeJSON(w, 200, s.moonlightViewOf(r.Context(), u, h))
}

// DELETE /api/moonlight/hosts/{id}: forget a PC. That PC keeps this account on its own list.
func (s *Server) handleMoonlightRemove(w http.ResponseWriter, r *http.Request) {
	u := s.moonlightUser(w, r)
	if u == nil {
		return
	}
	h := s.moonlightHostFor(w, r, u)
	if h == nil {
		return
	}
	g := s.moonlight()
	g.mu.Lock()
	if p := g.pairs[h.ID]; p != nil && p.cancel != nil {
		p.cancel()
	}
	delete(g.pairs, h.ID)
	delete(g.statuses, h.ID)
	delete(g.apps, h.ID)
	delete(g.wakes, h.ID)
	g.mu.Unlock()
	if _, err := s.DB.ExecContext(r.Context(), `DELETE FROM wittymoonlight_hosts WHERE id = ?`, h.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.removeMoonlightArt(u.ID, h.ID)
	writeJSON(w, 200, map[string]any{"ok": true, "note": "To finish, remove this device from the paired devices in Sunshine on that PC."})
}

// POST /api/moonlight/hosts/{id}/pair: start pairing. The answer has the PIN to type into Sunshine.
func (s *Server) handleMoonlightPair(w http.ResponseWriter, r *http.Request) {
	u := s.moonlightUser(w, r)
	if u == nil {
		return
	}
	h := s.moonlightHostFor(w, r, u)
	if h == nil {
		return
	}
	g := s.moonlight()
	g.mu.Lock()
	if p := g.pairs[h.ID]; p != nil && p.State == "waiting" {
		g.mu.Unlock()
		writeJSON(w, 200, p)
		return
	}
	g.mu.Unlock()
	if !s.moonlightAttempt(w, u) {
		return
	}
	pin, err := gamestream.NewPIN()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), gamestream.PairTimeout)
	p := &rigPairing{PIN: pin, State: "waiting", Started: time.Now(), Kind: "moonlight", cancel: cancel}
	g.mu.Lock()
	g.pairs[h.ID] = p
	g.mu.Unlock()
	go func() {
		defer cancel()
		finish := func(state, msg string) {
			g.mu.Lock()
			p.State, p.Message, p.PIN = state, msg, ""
			g.mu.Unlock()
		}
		client, err := s.moonlightClient(ctx, u, h)
		if err != nil {
			finish("failed", err.Error())
			return
		}
		client.Cert = nil
		cert, err := client.Pair(ctx, pin)
		if err != nil {
			finish("failed", pairMessage(err))
			return
		}
		bg := context.Background()
		if _, err := s.DB.ExecContext(bg, `UPDATE wittymoonlight_hosts SET cert = ?, paired_at = ? WHERE id = ?`, string(cert), db.Now(), h.ID); err != nil {
			finish("failed", err.Error())
			return
		}
		g.mu.Lock()
		delete(g.statuses, h.ID)
		g.mu.Unlock()
		finish("paired", "Paired.")
		s.Log.Info("moonlight: a PC was paired", "user", u.Name)
	}()
	writeJSON(w, 200, p)
}

func (s *Server) handleMoonlightPairStatus(w http.ResponseWriter, r *http.Request) {
	u := s.moonlightUser(w, r)
	if u == nil {
		return
	}
	h := s.moonlightHostFor(w, r, u)
	if h == nil {
		return
	}
	g := s.moonlight()
	g.mu.Lock()
	defer g.mu.Unlock()
	if p := g.pairs[h.ID]; p != nil {
		writeJSON(w, 200, p)
		return
	}
	writeJSON(w, 200, rigPairing{State: "none"})
}

func (s *Server) handleMoonlightPairCancel(w http.ResponseWriter, r *http.Request) {
	u := s.moonlightUser(w, r)
	if u == nil {
		return
	}
	h := s.moonlightHostFor(w, r, u)
	if h == nil {
		return
	}
	g := s.moonlight()
	g.mu.Lock()
	if p := g.pairs[h.ID]; p != nil && p.cancel != nil {
		p.cancel()
	}
	g.mu.Unlock()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// moonlightApp is something a PC offers. Sunshine answers with one default picture for any app
// without its own, so a picture that several apps share counts as none.
type moonlightApp struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	HasArt    bool   `json:"hasArt"`
	IsDesktop bool   `json:"isDesktop,omitempty"`
}

func (g *moonlightService) artPath(user, host int64, app int) string {
	return filepath.Join(g.dir, "art", fmt.Sprintf("u%d-h%d-%d.img", user, host, app))
}

func (s *Server) removeMoonlightArt(user, host int64) {
	matches, _ := filepath.Glob(filepath.Join(s.moonlight().dir, "art", fmt.Sprintf("u%d-h%d-*", user, host)))
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

// GET /api/moonlight/hosts/{id}/apps: what a paired PC offers.
func (s *Server) handleMoonlightApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := s.moonlightUser(w, r)
	if u == nil {
		return
	}
	h := s.moonlightHostFor(w, r, u)
	if h == nil {
		return
	}
	g := s.moonlight()
	g.mu.Lock()
	c, ok := g.apps[h.ID]
	g.mu.Unlock()
	if ok && time.Since(c.at) < rigSyncEvery && r.URL.Query().Get("fresh") == "" {
		writeJSON(w, 200, c.apps)
		return
	}
	client, err := s.moonlightClient(ctx, u, h)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	list, err := client.Apps(ctx)
	if err != nil {
		writeErr(w, 502, statusFor(h.Cert != "", nil, err).Message)
		return
	}
	if len(list) > rigMaxApps {
		list = list[:rigMaxApps]
	}
	out := make([]moonlightApp, len(list))
	sums := make([][32]byte, len(list))
	seen := map[[32]byte]int{}
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	if err := os.MkdirAll(filepath.Join(g.dir, "art"), 0o700); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for i, a := range list {
		out[i] = moonlightApp{ID: a.ID, Title: a.Title, IsDesktop: desktopLike.MatchString(a.Title)}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			data, _, err := client.AppImage(ctx, a.ID)
			if err != nil {
				return
			}
			sums[i] = sha256.Sum256(data)
			_ = os.WriteFile(g.artPath(u.ID, h.ID, a.ID), data, 0o600)
		}()
	}
	wg.Wait()
	for _, sum := range sums {
		seen[sum]++
	}
	for i := range out {
		out[i].HasArt = sums[i] != [32]byte{} && seen[sums[i]] == 1
	}
	g.mu.Lock()
	g.apps[h.ID] = moonlightAppsCache{at: time.Now(), apps: out}
	g.mu.Unlock()
	writeJSON(w, 200, out)
}

// GET /api/moonlight/hosts/{id}/apps/{app}/art: a picture kept from the last read of the list, and
// only ever served as a picture, as the bytes came from somebody else's computer.
func (s *Server) handleMoonlightArt(w http.ResponseWriter, r *http.Request) {
	u := s.moonlightUser(w, r)
	if u == nil {
		return
	}
	h := s.moonlightHostFor(w, r, u)
	if h == nil {
		return
	}
	app, err := strconv.Atoi(r.PathValue("app"))
	if err != nil {
		writeErr(w, 400, "bad app")
		return
	}
	data, err := os.ReadFile(s.moonlight().artPath(u.ID, h.ID, app))
	if err != nil {
		writeErr(w, 404, "no artwork")
		return
	}
	ct := "image/png"
	if len(data) > 3 && data[0] == 0xff && data[1] == 0xd8 {
		ct = "image/jpeg"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(data)
}

// moonlightPlayRequest is what a device wants. Numbers are limited to what's sensible, as a
// person's request ends up deciding what an encoder on someone's PC does.
type moonlightPlayRequest struct {
	Width   int `json:"width"`
	Height  int `json:"height"`
	FPS     int `json:"fps"`
	Bitrate int `json:"bitrate"`
}

func (q *moonlightPlayRequest) clean() {
	clamp := func(v, lo, hi, def int) int {
		if v == 0 {
			return def
		}
		return max(lo, min(hi, v))
	}
	q.Width = clamp(q.Width, 640, 3840, 1920) &^ 1
	q.Height = clamp(q.Height, 360, 2160, 1080) &^ 1
	q.FPS = clamp(q.FPS, 24, 120, 60)
	q.Bitrate = clamp(q.Bitrate, 1000, 150000, 20000)
}

var rtspURL = regexp.MustCompile(`^(rtsp|rtspenc)://[^/]*:([0-9]{1,5})/?$`)

// cleanSessionURL rebuilds where the stream connects from what the PC said. The host program
// connects where this says, and a far end has no business naming somewhere else: only its
// scheme and port are used, with the address already checked.
func cleanSessionURL(raw, ip string) (string, error) {
	m := rtspURL.FindStringSubmatch(raw)
	if m == nil {
		return "", errors.New("the PC gave a stream address that doesn't look right")
	}
	port, err := strconv.Atoi(m[2])
	if err != nil || port < 1024 || port > 65535 {
		return "", errors.New("the PC gave a stream address that doesn't look right")
	}
	return m[1] + "://" + net.JoinHostPort(ip, m[2]), nil
}

// resolveAllowed turns an address into the IP the host program should connect to, which is
// checked here, as that program connects where it's told.
func resolveAllowed(ctx context.Context, address string, policy gamestream.Policy) (string, error) {
	if ip, err := netip.ParseAddr(address); err == nil {
		return ip.String(), policy.Allow(ip)
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", address)
	if err != nil || len(ips) == 0 {
		return "", gamestream.ErrNoAnswer
	}
	for _, ip := range ips {
		if err := policy.Allow(ip); err != nil {
			return "", err
		}
	}
	return ips[0].Unmap().String(), nil
}

// startMoonlightStream starts a game and the stream of it, for one person.
func (s *Server) startMoonlightStream(ctx context.Context, u *auth.User, client *gamestream.Rig, appID int, title string, q moonlightPlayRequest, hostID, rigID int64) (*moonlightStream, error) {
	program := s.moonlightProgram()
	if program == "" {
		return nil, relay.ErrNoProgram
	}
	g := s.moonlight()
	limit, _ := strconv.Atoi(s.DB.GetSetting(ctx, "moonlightMaxStreams", settingDefaults["moonlightMaxStreams"]))
	if limit < 1 {
		limit = 2
	}
	g.mu.Lock()
	var mine []*moonlightStream
	for _, st := range g.sessions {
		if st.User == u.ID {
			mine = append(mine, st)
		}
	}
	others := len(g.sessions) - len(mine)
	g.mu.Unlock()
	if others >= limit {
		return nil, errMoonlightFull
	}
	// One stream per person: starting another ends the last one, as with any game here.
	for _, st := range mine {
		st.Session.Close()
	}
	info, err := client.Status(ctx)
	if err != nil {
		return nil, err
	}
	ip, err := resolveAllowed(ctx, client.Address, client.Policy)
	if err != nil {
		return nil, err
	}
	o := gamestream.StreamOptions{AppID: appID, Width: q.Width, Height: q.Height, FPS: q.FPS}
	if _, err := rand.Read(o.Key[:]); err != nil {
		return nil, err
	}
	if _, err := rand.Read(o.IV[:]); err != nil {
		return nil, err
	}
	var launched *gamestream.Launched
	switch {
	case info.CurrentGame == 0:
		launched, err = client.Launch(ctx, o)
	case info.CurrentGame == appID:
		launched, err = client.Resume(ctx, o)
	default:
		return nil, gamestream.ErrBusy
	}
	if err != nil {
		return nil, err
	}
	rtsp, err := cleanSessionURL(launched.SessionURL, ip)
	if err != nil {
		return nil, err
	}
	sidb := make([]byte, 16)
	if _, err := rand.Read(sidb); err != nil {
		return nil, err
	}
	st := &moonlightStream{ID: hex.EncodeToString(sidb), User: u.ID, HostID: hostID, RigID: rigID, AppID: appID, Title: title, Client: client}
	addr, err := netip.ParseAddr(ip)
	remote := err == nil && gamestream.Classify(addr) == gamestream.Public
	packet := 1392
	if remote {
		packet = 1024
	}
	opts := relay.Options{
		Program: program, Prepare: prepareMoonlightHost, Log: s.Log,
		Start: relay.Start{Address: ip, AppVersion: info.Version, GfeVersion: info.GfeVersion, RTSP: rtsp, CodecModes: info.CodecModes,
			Width: q.Width, Height: q.Height, FPS: q.FPS, BitrateKbps: q.Bitrate, PacketSize: packet, Remote: remote, Key: o.Key, IV: o.IV},
		OnEnd: func(reason string) {
			g.mu.Lock()
			delete(g.sessions, st.ID)
			g.mu.Unlock()
			s.Log.Info("moonlight: a stream ended", "user", u.Name)
		},
	}
	if v := strings.TrimSpace(s.DB.GetSetting(ctx, "moonlightPublicIp", "")); v != "" {
		opts.PublicIP = []string{v}
	}
	if v := strings.TrimSpace(s.DB.GetSetting(ctx, "moonlightStun", "")); v != "" {
		opts.STUN = strings.Fields(strings.ReplaceAll(v, ",", " "))
	}
	if p, err := strconv.Atoi(s.DB.GetSetting(ctx, "moonlightUdpPort", "")); err == nil && p > 1024 {
		opts.UDPPort = p
	}
	sess, err := relay.New(ctx, opts)
	if err != nil {
		return nil, err
	}
	st.Session = sess
	g.mu.Lock()
	g.sessions[st.ID] = st
	g.mu.Unlock()
	s.Log.Info("moonlight: a stream started", "user", u.Name, "encrypted", true)
	return st, nil
}

var errMoonlightFull = errors.New("This server is already streaming as many games as it's set to. Try again when one has finished.")

// writeMoonlightStartError says why a stream didn't start.
func writeMoonlightStartError(w http.ResponseWriter, u *auth.User, err error) {
	var se *gamestream.StatusError
	switch {
	case errors.Is(err, relay.ErrNoProgram):
		writeJSON(w, 503, map[string]any{"error": "This server doesn't have its Moonlight program yet. It's a separate download: see Settings, WittyMoonlight.", "needProgram": true})
	case errors.Is(err, errMoonlightFull):
		writeErr(w, 503, err.Error())
	case errors.Is(err, gamestream.ErrBusy):
		writeErr(w, 409, "Another game is running on this PC, so this one can't start until it's closed.")
	case errors.Is(err, gamestream.ErrNotPaired):
		writeJSON(w, 409, map[string]any{"error": "This PC no longer trusts this account. Pair it again.", "needPair": true})
	case errors.Is(err, gamestream.ErrCertChanged):
		writeErr(w, 409, "Something other than the PC you paired with answered, so it wasn't trusted. If you reinstalled Sunshine, pair it again.")
	case errors.As(err, &se):
		writeErr(w, 502, "The PC couldn't start it. "+se.Message)
	default:
		writeErr(w, 502, moonlightConnectMessage(u, err))
	}
}

type moonlightSessionView struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	FPS    int    `json:"fps,omitempty"`
}

func viewOfStream(st *moonlightStream) moonlightSessionView {
	info := st.Session.Info()
	return moonlightSessionView{ID: st.ID, Title: st.Title, Width: info.Width, Height: info.Height, FPS: info.FPS}
}

// POST /api/moonlight/hosts/{id}/apps/{app}/play {width, height, fps, bitrate}
func (s *Server) handleMoonlightPlay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := s.moonlightUser(w, r)
	if u == nil {
		return
	}
	h := s.moonlightHostFor(w, r, u)
	if h == nil {
		return
	}
	appID, err := strconv.Atoi(r.PathValue("app"))
	if err != nil {
		writeErr(w, 400, "bad app")
		return
	}
	var q moonlightPlayRequest
	if err := readJSON(r, &q); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	q.clean()
	client, err := s.moonlightClient(ctx, u, h)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	apps, err := client.Apps(ctx)
	if err != nil {
		writeMoonlightStartError(w, u, err)
		return
	}
	title := ""
	for _, a := range apps {
		if a.ID == appID {
			title = a.Title
		}
	}
	if title == "" {
		writeErr(w, 404, "That PC doesn't offer that any more.")
		return
	}
	st, err := s.startMoonlightStream(ctx, u, client, appID, title, q, h.ID, 0)
	if err != nil {
		writeMoonlightStartError(w, u, err)
		return
	}
	g := s.moonlight()
	g.mu.Lock()
	delete(g.statuses, h.ID)
	g.mu.Unlock()
	writeJSON(w, 200, viewOfStream(st))
}

// POST /api/rigs/games/{id}/stream: play a shared PC's game here. It uses the server's own pairing,
// and the same checks as any other way of playing it.
func (s *Server) handleRigStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	it, rig, appID := s.rigGameFor(w, r)
	if it == nil {
		return
	}
	var q moonlightPlayRequest
	if err := readJSON(r, &q); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	q.clean()
	u := userFrom(r)
	client, err := s.rigClient(rig)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	st := s.rigStatusNow(ctx, rig)
	if !s.rigMayJoin(rig, st, appID, u.ID, u.IsAdmin) {
		writeErr(w, 409, "Something else is running on this PC, so this can't start until it's closed.")
		return
	}
	stream, err := s.startMoonlightStream(ctx, u, client, appID, it.Title, q, 0, rig.ID)
	if err != nil {
		writeMoonlightStartError(w, u, err)
		return
	}
	g := s.rigs()
	g.mu.Lock()
	g.starter[rig.ID] = u.ID
	delete(g.statuses, rig.ID)
	g.mu.Unlock()
	writeJSON(w, 200, viewOfStream(stream))
}

// POST /api/moonlight/hosts/{id}/wake
func (s *Server) handleMoonlightWake(w http.ResponseWriter, r *http.Request) {
	u := s.moonlightUser(w, r)
	if u == nil {
		return
	}
	h := s.moonlightHostFor(w, r, u)
	if h == nil {
		return
	}
	if h.MAC == "" {
		writeErr(w, 409, "This PC hasn't told this account how to wake it yet. It says so when it's paired and on.")
		return
	}
	if h.Public {
		writeErr(w, 409, "A PC over the internet can't be woken from here.")
		return
	}
	g := s.moonlight()
	g.mu.Lock()
	if time.Since(g.wakes[h.ID]) < rigWakeEvery {
		g.mu.Unlock()
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	g.wakes[h.ID] = time.Now()
	delete(g.statuses, h.ID)
	g.mu.Unlock()
	if err := gamestream.Wake(h.MAC, h.Address); err != nil {
		writeErr(w, 500, "The wake-up signal couldn't be sent.")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "note": "A wake-up signal was sent. A PC usually takes under a minute, if Wake-on-LAN is on in its settings."})
}

// POST /api/moonlight/hosts/{id}/quit: end what's running on this person's PC.
func (s *Server) handleMoonlightQuit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := s.moonlightUser(w, r)
	if u == nil {
		return
	}
	h := s.moonlightHostFor(w, r, u)
	if h == nil {
		return
	}
	client, err := s.moonlightClient(ctx, u, h)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := client.Quit(ctx); err != nil {
		writeMoonlightStartError(w, u, err)
		return
	}
	g := s.moonlight()
	g.mu.Lock()
	delete(g.statuses, h.ID)
	g.mu.Unlock()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// moonlightStreamFor finds a stream of this person's.
func (s *Server) moonlightStreamFor(w http.ResponseWriter, r *http.Request) *moonlightStream {
	u := userFrom(r)
	g := s.moonlight()
	g.mu.Lock()
	st := g.sessions[r.PathValue("sid")]
	g.mu.Unlock()
	if st == nil || st.User != u.ID {
		writeErr(w, 404, "That stream has ended.")
		return nil
	}
	return st
}

func (s *Server) handleMoonlightSession(w http.ResponseWriter, r *http.Request) {
	if st := s.moonlightStreamFor(w, r); st != nil {
		writeJSON(w, 200, viewOfStream(st))
	}
}

// POST /api/moonlight/sessions/{sid}/webrtc {sdp}
func (s *Server) handleMoonlightWebRTC(w http.ResponseWriter, r *http.Request) {
	st := s.moonlightStreamFor(w, r)
	if st == nil {
		return
	}
	var b struct {
		SDP string `json:"sdp"`
	}
	if err := readJSON(r, &b); err != nil || len(b.SDP) > 256<<10 {
		writeErr(w, 400, "That offer couldn't be read.")
		return
	}
	answer, err := st.Session.Offer(r.Context(), b.SDP)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"sdp": answer})
}

// DELETE /api/moonlight/sessions/{sid}?quit=1: stop streaming, and with quit, end the game too.
func (s *Server) handleMoonlightSessionEnd(w http.ResponseWriter, r *http.Request) {
	st := s.moonlightStreamFor(w, r)
	if st == nil {
		return
	}
	st.Session.Close()
	if r.URL.Query().Get("quit") == "1" {
		if err := st.Client.Quit(r.Context()); err != nil {
			s.Log.Debug("moonlight: ending the game", "err", err)
		}
		g := s.moonlight()
		g.mu.Lock()
		delete(g.statuses, st.HostID)
		g.mu.Unlock()
		if st.RigID != 0 {
			s.forgetRigStatus(st.RigID)
		}
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// prepareMoonlightHost keeps the host program apart from this server. It parses what a stranger's
// PC sends in C, so it's given as little as it can work with: see the sandbox files.
func prepareMoonlightHost(cmd *exec.Cmd) {
	cmd.Dir = os.TempDir()
}
