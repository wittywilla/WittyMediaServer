package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/remote"
	"github.com/wittywilla/wittymoonlight"
	"github.com/wittywilla/wittymoonlight/gamestream"
)

// Game rigs: someone's own gaming PC running Sunshine, as a source of games. See
// docs/GAME-RIGS.md. A PC reached this way may hold everything its owner has, so:
//
//   - Adding, pairing and managing a rig is for admins. Nobody else can name an address.
//   - Everyone else only ever reaches addresses an admin registered, on fixed paths.
//   - This server's key stays in a file, and nothing secret goes to a browser.
//   - What someone may play is decided by the same rules as every other item, and checked
//     again at the moment a stream starts.

const (
	rigStatusLife = 5 * time.Second
	rigSyncEvery  = 10 * time.Minute
	rigWakeEvery  = 10 * time.Second
	rigMaxApps    = 500
)

// rigRoutes registers the game rigs API.
func (s *Server) rigRoutes(m *http.ServeMux) {
	a := s.requireUser
	m.Handle("GET /api/rigs", a(s.handleRigs))
	m.Handle("GET /api/rigs/find", s.requireAdmin(s.handleRigFind))
	m.Handle("POST /api/rigs", s.requireAdmin(s.handleRigAdd))
	m.Handle("PATCH /api/rigs/{id}", s.requireAdmin(s.handleRigUpdate))
	m.Handle("DELETE /api/rigs/{id}", s.requireAdmin(s.handleRigRemove))
	m.Handle("POST /api/rigs/{id}/pair", s.requireAdmin(s.handleRigPair))
	m.Handle("GET /api/rigs/{id}/pair", s.requireAdmin(s.handleRigPairStatus))
	m.Handle("DELETE /api/rigs/{id}/pair", s.requireAdmin(s.handleRigPairCancel))
	m.Handle("POST /api/rigs/{id}/devices", s.requireAdmin(s.handleRigDevicePIN))
	m.Handle("POST /api/rigs/{id}/sync", s.requireAdmin(s.handleRigSync))
	m.Handle("GET /api/rigs/{id}/apps", s.requireAdmin(s.handleRigApps))
	m.Handle("PUT /api/rigs/{id}/apps/{app}", s.requireAdmin(s.handleRigAppShown))
	m.Handle("POST /api/rigs/{id}/wake", a(s.handleRigWake))
	m.Handle("POST /api/rigs/{id}/quit", a(s.handleRigQuit))
	m.Handle("POST /api/rigs/games/{id}/play", a(s.handleRigPlay))
	m.Handle("POST /api/rigs/games/{id}/start", a(s.handleRigStart))
}

// rigService is built the first time anything needs it, so a server with no rigs spends
// nothing on them.
type rigService struct {
	dir string

	mu       sync.Mutex
	identity *gamestream.Identity
	statuses map[int64]rigCached
	pairs    map[int64]*rigPairing
	lastSync map[int64]time.Time
	lastWake map[int64]time.Time
	starter  map[int64]int64 // rig: who last started something on it through this server
	syncing  map[int64]*sync.Mutex
	finding  sync.Mutex
}

type rigCached struct {
	at   time.Time
	view rigState
}

type rigPairing struct {
	PIN     string    `json:"pin,omitempty"`
	State   string    `json:"state"` // waiting, paired, failed
	Message string    `json:"message,omitempty"`
	Started time.Time `json:"-"`
	Kind    string    `json:"kind"`
	cancel  context.CancelFunc
}

func (s *Server) rigs() *rigService {
	s.rigOnce.Do(func() {
		s.rigSvc = &rigService{dir: filepath.Join(s.Cfg.DataDir, "gamerigs"), statuses: map[int64]rigCached{}, pairs: map[int64]*rigPairing{},
			lastSync: map[int64]time.Time{}, lastWake: map[int64]time.Time{}, starter: map[int64]int64{}, syncing: map[int64]*sync.Mutex{}}
	})
	return s.rigSvc
}

// StopRigs ends what's running for rigs; the server is shutting down. Games on the PCs
// themselves carry on, as they're someone's to end.
func (s *Server) StopRigs() {
	if s.rigSvc == nil {
		return
	}
	s.rigSvc.mu.Lock()
	for _, p := range s.rigSvc.pairs {
		if p.cancel != nil {
			p.cancel()
		}
	}
	s.rigSvc.mu.Unlock()
}

func (g *rigService) ident() (*gamestream.Identity, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.identity != nil {
		return g.identity, nil
	}
	id, err := gamestream.LoadOrCreateIdentity(filepath.Join(g.dir, "identity"))
	if err != nil {
		return nil, fmt.Errorf("the rig identity can't be read or made: %w", err)
	}
	g.identity = id
	return id, nil
}

// rigRow is a rig as it's stored. It has no JSON form: rigView decides what's shown to whom.
type rigRow struct {
	ID, LibraryID   int64
	Name, Address   string
	Port            int
	RemoteAddress   string
	AllowPublic     bool
	UUID, MAC, Cert string
	PairedAt        *int64
}

const rigCols = `id, library_id, name, address, port, remote_address, allow_public, uuid, mac, cert, paired_at`

func scanRig(row interface{ Scan(...any) error }) (*rigRow, error) {
	var r rigRow
	if err := row.Scan(&r.ID, &r.LibraryID, &r.Name, &r.Address, &r.Port, &r.RemoteAddress, &r.AllowPublic, &r.UUID, &r.MAC, &r.Cert, &r.PairedAt); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Server) rigRows(ctx context.Context) ([]*rigRow, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+rigCols+` FROM wittysunshine_rigs ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*rigRow
	for rows.Next() {
		r, err := scanRig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Server) rigByID(ctx context.Context, id int64) (*rigRow, error) {
	return scanRig(s.DB.QueryRowContext(ctx, `SELECT `+rigCols+` FROM wittysunshine_rigs WHERE id = ?`, id))
}

// rigForRequest loads the rig in the path. Anyone not allowed to see it gets "not found".
func (s *Server) rigForRequest(w http.ResponseWriter, r *http.Request) *rigRow {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil
	}
	rig, err := s.rigByID(r.Context(), id)
	if err != nil || !s.rigVisible(r.Context(), s.access(r), rig) {
		writeErr(w, 404, "not found")
		return nil
	}
	return rig
}

// rigVisible: someone can see a rig if they can see at least one of its games. A Kids
// profile with nothing unlocked doesn't learn that it exists.
func (s *Server) rigVisible(ctx context.Context, a *library.Access, rig *rigRow) bool {
	if a == nil {
		return true
	}
	if !a.AllowsLibrary(rig.LibraryID) {
		return false
	}
	n, err := s.Store.VisibleRigGames(ctx, rig.LibraryID, a)
	return err == nil && n > 0
}

// client makes the protocol client for a rig. The connection policy is applied to every
// connection it makes, whoever's request caused it.
func (s *Server) rigClient(rig *rigRow) (*gamestream.Rig, error) {
	id, err := s.rigs().ident()
	if err != nil {
		return nil, err
	}
	return &gamestream.Rig{Address: rig.Address, Port: rig.Port, Identity: id, Cert: []byte(rig.Cert), Policy: gamestream.Policy{AllowPublic: rig.AllowPublic}}, nil
}

// rigState is how a rig is doing, in words for people.
type rigState struct {
	State       string `json:"state"` // online, busy, asleep, no-sunshine, not-paired, cert-changed, error
	Message     string `json:"message"`
	CurrentGame string `json:"currentGame,omitempty"`
	currentApp  int
	version     string
}

func statusFor(paired bool, info *gamestream.Info, err error) rigState {
	switch {
	case err == nil && !paired:
		return rigState{State: "not-paired", Message: "This PC is on, and needs pairing.", version: info.Version}
	case err == nil && info.CurrentGame > 0:
		return rigState{State: "busy", Message: "A game is running on this PC.", currentApp: info.CurrentGame, version: info.Version}
	case err == nil:
		return rigState{State: "online", Message: "Ready to play.", version: info.Version}
	case errors.Is(err, gamestream.ErrRefused):
		return rigState{State: "no-sunshine", Message: "This PC is on, but Sunshine isn't running on it. Start Sunshine on that PC."}
	case errors.Is(err, gamestream.ErrNoAnswer):
		return rigState{State: "asleep", Message: "This PC isn't answering. It's asleep, turned off, or not on this network."}
	case errors.Is(err, gamestream.ErrNotPaired):
		return rigState{State: "not-paired", Message: "This PC no longer trusts this server, so it has to be paired again."}
	case errors.Is(err, gamestream.ErrCertChanged):
		return rigState{State: "cert-changed", Message: "Something other than the PC you paired with answered at that address, so it wasn't trusted. If you reinstalled Sunshine, pair it again. If not, check what's at that address."}
	case errors.Is(err, gamestream.ErrBlocked):
		return rigState{State: "error", Message: "This PC's address isn't allowed: " + err.Error()}
	}
	return rigState{State: "error", Message: "This PC answered with something that isn't from Sunshine."}
}

// rigStatus asks a rig how it is, at most every few seconds however many people are looking.
func (s *Server) rigStatus(ctx context.Context, rig *rigRow) rigState {
	g := s.rigs()
	g.mu.Lock()
	c, ok := g.statuses[rig.ID]
	g.mu.Unlock()
	if ok && time.Since(c.at) < rigStatusLife {
		return c.view
	}
	return s.rigStatusNow(ctx, rig)
}

func (s *Server) rigStatusNow(ctx context.Context, rig *rigRow) rigState {
	var view rigState
	c, err := s.rigClient(rig)
	if err != nil {
		view = rigState{State: "error", Message: err.Error()}
	} else if rig.Cert == "" {
		info, err := c.Hello(ctx)
		view = statusFor(false, info, err)
	} else {
		info, err := c.Status(ctx)
		view = statusFor(true, info, err)
		// The PC only says its MAC address to someone it trusts, and it's what wakes it.
		if err == nil && info.MAC != rig.MAC {
			if _, merr := gamestream.ParseMAC(info.MAC); merr == nil {
				_, _ = s.DB.ExecContext(ctx, `UPDATE wittysunshine_rigs SET mac = ? WHERE id = ?`, info.MAC, rig.ID)
				rig.MAC = info.MAC
			}
		}
	}
	if view.currentApp > 0 {
		var title string
		if err := s.DB.QueryRowContext(ctx, `SELECT title FROM wittysunshine_apps WHERE rig_id = ? AND app_id = ?`, rig.ID, view.currentApp).Scan(&title); err == nil {
			view.CurrentGame = title
		}
	}
	g := s.rigs()
	g.mu.Lock()
	g.statuses[rig.ID] = rigCached{at: time.Now(), view: view}
	g.mu.Unlock()
	return view
}

func (s *Server) forgetRigStatus(id int64) {
	g := s.rigs()
	g.mu.Lock()
	delete(g.statuses, id)
	g.mu.Unlock()
}

// rigView is a rig as the API shows it. Where it is and how it's identified are for admins.
type rigView struct {
	ID          int64  `json:"id"`
	LibraryID   int64  `json:"libraryId"`
	Name        string `json:"name"`
	Paired      bool   `json:"paired"`
	State       string `json:"state"`
	Message     string `json:"message"`
	CurrentGame string `json:"currentGame,omitempty"`
	CanWake     bool   `json:"canWake"`
	CanQuit     bool   `json:"canQuit"`
	// Admins only.
	Address       string `json:"address,omitempty"`
	Port          int    `json:"port,omitempty"`
	RemoteAddress string `json:"remoteAddress,omitempty"`
	Version       string `json:"version,omitempty"`
	Fingerprint   string `json:"fingerprint,omitempty"`
	HasMAC        bool   `json:"hasMac,omitempty"`
	Public        bool   `json:"public,omitempty"`
	WebURL        string `json:"webUrl,omitempty"`
}

func (s *Server) rigViewFor(ctx context.Context, r *http.Request, rig *rigRow) rigView {
	st := s.rigStatus(ctx, rig)
	u := userFrom(r)
	g := s.rigs()
	g.mu.Lock()
	starter := g.starter[rig.ID]
	g.mu.Unlock()
	v := rigView{ID: rig.ID, LibraryID: rig.LibraryID, Name: rig.Name, Paired: rig.Cert != "", State: st.State, Message: st.Message, CurrentGame: st.CurrentGame,
		CanWake: rig.MAC != "" && (st.State == "asleep"), CanQuit: st.State == "busy" && (u.IsAdmin || starter == u.ID)}
	if !u.IsAdmin {
		if st.State == "cert-changed" || st.State == "not-paired" || st.State == "error" {
			v.Message = "Something needs sorting out on this PC. Ask whoever runs this server."
		}
		return v
	}
	v.Address, v.Port, v.RemoteAddress, v.Version, v.HasMAC, v.Public = rig.Address, rig.Port, rig.RemoteAddress, st.version, rig.MAC != "", rig.AllowPublic
	if rig.Cert != "" {
		v.Fingerprint = gamestream.CertFingerprint([]byte(rig.Cert))
	}
	if c, err := s.rigClient(rig); err == nil {
		v.WebURL = c.WebURL()
	}
	return v
}

// GET /api/rigs: the rigs this account can see, and how they're doing.
func (s *Server) handleRigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := s.rigRows(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a := s.access(r)
	visible := rows[:0]
	for _, rig := range rows {
		if s.rigVisible(ctx, a, rig) {
			visible = append(visible, rig)
		}
	}
	out := make([]rigView, len(visible))
	var wg sync.WaitGroup
	for i, rig := range visible {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = s.rigViewFor(ctx, r, rig)
			if out[i].Paired && (out[i].State == "online" || out[i].State == "busy") {
				s.syncSoon(rig)
			}
		}()
	}
	wg.Wait()
	writeJSON(w, 200, out)
}

// foundRig is a PC that answered on the network.
type foundRig struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	Version string `json:"version"`
	Added   bool   `json:"added"`
}

// GET /api/rigs/find: who on the home network runs Sunshine. Takes a few seconds.
func (s *Server) handleRigFind(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g := s.rigs()
	if !g.finding.TryLock() {
		writeErr(w, 409, "Already looking. Try again in a moment.")
		return
	}
	defer g.finding.Unlock()
	found, err := gamestream.Browse(ctx, 3*time.Second)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	rigs, err := s.rigRows(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := []foundRig{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, f := range found {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// mDNS can say anything, so it only counts if the PC itself answers.
			info, err := (&gamestream.Rig{Address: f.Address, Port: f.Port}).Hello(ctx)
			if err != nil {
				return
			}
			item := foundRig{Name: info.Name, Address: f.Address, Port: f.Port, Version: info.Version}
			for _, rig := range rigs {
				if rig.UUID != "" && strings.EqualFold(rig.UUID, info.UUID) {
					item.Added = true
					s.rigMoved(ctx, rig, f)
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

// rigMoved follows a paired PC to a new address, when its router gave it another. A name on
// the network proves nothing: only answering with the pinned certificate does.
func (s *Server) rigMoved(ctx context.Context, rig *rigRow, f gamestream.Found) {
	if rig.Cert == "" || rig.Address == f.Address {
		return
	}
	id, err := s.rigs().ident()
	if err != nil {
		return
	}
	moved := &gamestream.Rig{Address: f.Address, Port: f.Port, Identity: id, Cert: []byte(rig.Cert)}
	if _, err := moved.Status(ctx); err != nil {
		return
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE wittysunshine_rigs SET address = ?, port = ? WHERE id = ?`, f.Address, f.Port, rig.ID); err == nil {
		s.Log.Info("rig moved to a new address", "rig", rig.Name)
		s.forgetRigStatus(rig.ID)
	}
}

func rigPortOf(port int) int {
	if port == gamestream.DefaultPort {
		return 0
	}
	return port
}

// POST /api/rigs {address, name, confirmPublic}: add a PC. It has to answer as Sunshine.
func (s *Server) handleRigAdd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var b struct {
		Address       string `json:"address"`
		Name          string `json:"name"`
		ConfirmPublic bool   `json:"confirmPublic"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	host, port, err := gamestream.CleanAddress(b.Address)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	reach, err := gamestream.CheckAddress(ctx, host, nil)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if reach == gamestream.Public && !b.ConfirmPublic {
		writeJSON(w, 409, map[string]any{"public": true, "error": "That address is on the internet, which means this PC's game streaming ports are open to everyone. A VPN is much safer: see the guide. Add it anyway only if you know that's what you want."})
		return
	}
	rig := &gamestream.Rig{Address: host, Port: port, Policy: gamestream.Policy{AllowPublic: reach == gamestream.Public}}
	info, err := rig.Hello(ctx)
	if err != nil {
		code := 502
		msg := "That PC didn't answer. Check the address, that it's on, and that Sunshine is installed and running."
		switch {
		case errors.Is(err, gamestream.ErrRefused):
			msg = "That PC is on, but Sunshine isn't running on it. Install or start Sunshine, then try again."
		case errors.Is(err, gamestream.ErrBadAnswer):
			msg = "Something answered at that address, but it isn't Sunshine."
		case errors.Is(err, gamestream.ErrBlocked):
			msg, code = err.Error(), 400
		}
		writeErr(w, code, msg)
		return
	}
	rows, err := s.rigRows(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, other := range rows {
		if strings.EqualFold(other.UUID, info.UUID) {
			writeErr(w, 409, "That PC has already been added, as "+other.Name+".")
			return
		}
	}
	name := strings.TrimSpace(b.Name)
	if name == "" {
		name = info.Name
	}
	if name == "" {
		name = "Gaming PC"
	}
	if len(name) > 100 {
		name = name[:100]
	}
	lib, err := s.Store.CreateRigLibrary(ctx, name)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO wittysunshine_rigs(library_id, name, address, port, allow_public, uuid, created_at) VALUES(?,?,?,?,?,?,?)`,
		lib.ID, name, host, port, reach == gamestream.Public, info.UUID, db.Now())
	if err != nil {
		_ = s.Store.DeleteLibrary(ctx, lib.ID)
		writeErr(w, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	s.Log.Info("rig added", "rig", name, "by", userFrom(r).Name)
	row, err := s.rigByID(ctx, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, s.rigViewFor(ctx, r, row))
}

// PATCH /api/rigs/{id} {name, address, remoteAddress, mac}: only what's named changes. A new
// address keeps the pinned certificate, so it must still be the same PC there.
func (s *Server) handleRigUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rig := s.rigForRequest(w, r)
	if rig == nil {
		return
	}
	var b struct {
		Name          *string `json:"name"`
		Address       *string `json:"address"`
		RemoteAddress *string `json:"remoteAddress"`
		MAC           *string `json:"mac"`
		ConfirmPublic bool    `json:"confirmPublic"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.Name != nil {
		name := strings.TrimSpace(*b.Name)
		if name == "" || len(name) > 100 {
			writeErr(w, 400, "A name is required, up to 100 characters.")
			return
		}
		if _, err := s.Store.UpdateLibrary(ctx, rig.LibraryID, name, "games", nil); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		rig.Name = name
	}
	if b.Address != nil {
		host, port, err := gamestream.CleanAddress(*b.Address)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		reach, err := gamestream.CheckAddress(ctx, host, nil)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if reach == gamestream.Public && !b.ConfirmPublic {
			writeJSON(w, 409, map[string]any{"public": true, "error": "That address is on the internet. A VPN is much safer: see the guide."})
			return
		}
		rig.Address, rig.Port, rig.AllowPublic = host, port, reach == gamestream.Public
	}
	if b.RemoteAddress != nil {
		rig.RemoteAddress = ""
		if v := strings.TrimSpace(*b.RemoteAddress); v != "" {
			host, _, err := gamestream.CleanAddress(v)
			if err != nil {
				writeErr(w, 400, err.Error())
				return
			}
			rig.RemoteAddress = host
		}
	}
	if b.MAC != nil {
		rig.MAC = ""
		if v := strings.TrimSpace(*b.MAC); v != "" {
			hw, err := gamestream.ParseMAC(v)
			if err != nil {
				writeErr(w, 400, err.Error())
				return
			}
			rig.MAC = hw.String()
		}
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE wittysunshine_rigs SET name=?, address=?, port=?, remote_address=?, allow_public=?, mac=? WHERE id=?`,
		rig.Name, rig.Address, rig.Port, rig.RemoteAddress, rig.AllowPublic, rig.MAC, rig.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.forgetRigStatus(rig.ID)
	writeJSON(w, 200, s.rigViewFor(ctx, r, rig))
}

// forgetRig drops everything held for a rig but its rows, which go with its library.
func (s *Server) forgetRig(rig *rigRow) {
	g := s.rigs()
	g.mu.Lock()
	if p := g.pairs[rig.ID]; p != nil && p.cancel != nil {
		p.cancel()
	}
	delete(g.pairs, rig.ID)
	delete(g.statuses, rig.ID)
	delete(g.lastSync, rig.ID)
	delete(g.lastWake, rig.ID)
	delete(g.starter, rig.ID)
	g.mu.Unlock()
	s.removeRigArt(rig.ID)
}

// rigLibraryGoing is told a library is being deleted from the library settings, in case it's
// a rig's: the rig goes with it.
func (s *Server) rigLibraryGoing(ctx context.Context, libraryID int64) {
	if rig, err := scanRig(s.DB.QueryRowContext(ctx, `SELECT `+rigCols+` FROM wittysunshine_rigs WHERE library_id = ?`, libraryID)); err == nil {
		s.forgetRig(rig)
	}
}

// DELETE /api/rigs/{id}: forget a PC, its library and its pictures. The PC keeps this server
// on its own list until someone removes it there, which the protocol leaves no way to do.
func (s *Server) handleRigRemove(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rig := s.rigForRequest(w, r)
	if rig == nil {
		return
	}
	s.forgetRig(rig)
	if err := s.Store.DeleteLibrary(ctx, rig.LibraryID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.Info("rig removed", "rig", rig.Name, "by", userFrom(r).Name)
	writeJSON(w, 200, map[string]any{"ok": true, "note": "To finish, remove " + gamestream.DeviceName + " from the paired devices in Sunshine on that PC."})
}

// POST /api/rigs/{id}/pair: start pairing. The answer has the PIN to type into Sunshine, and
// the pairing carries on until someone does, or the time runs out.
func (s *Server) handleRigPair(w http.ResponseWriter, r *http.Request) {
	rig := s.rigForRequest(w, r)
	if rig == nil {
		return
	}
	pin, err := gamestream.NewPIN()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	g := s.rigs()
	g.mu.Lock()
	if p := g.pairs[rig.ID]; p != nil && p.State == "waiting" {
		g.mu.Unlock()
		writeJSON(w, 200, p)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), gamestream.PairTimeout)
	p := &rigPairing{PIN: pin, State: "waiting", Started: time.Now(), Kind: "server", cancel: cancel}
	g.pairs[rig.ID] = p
	g.mu.Unlock()
	s.Log.Info("rig pairing started", "rig", rig.Name, "by", userFrom(r).Name)
	go s.runRigPair(ctx, cancel, rig, p, pin)
	writeJSON(w, 200, p)
}

func (s *Server) runRigPair(ctx context.Context, cancel context.CancelFunc, rig *rigRow, p *rigPairing, pin string) {
	defer cancel()
	finish := func(state, msg string) {
		g := s.rigs()
		g.mu.Lock()
		p.State, p.Message, p.PIN = state, msg, ""
		g.mu.Unlock()
	}
	c, err := s.rigClient(rig)
	if err != nil {
		finish("failed", err.Error())
		return
	}
	c.Cert = nil
	cert, err := c.Pair(ctx, pin)
	if err != nil {
		finish("failed", pairMessage(err))
		s.Log.Info("rig pairing failed", "rig", rig.Name, "reason", pairMessage(err))
		return
	}
	// A PC that was already paired comes back as itself. If it's another one, the old
	// certificate going away is what the admin just agreed to by typing a PIN into it.
	bg := context.Background()
	if _, err := s.DB.ExecContext(bg, `UPDATE wittysunshine_rigs SET cert = ?, paired_at = ? WHERE id = ?`, string(cert), db.Now(), rig.ID); err != nil {
		finish("failed", err.Error())
		return
	}
	rig.Cert = string(cert)
	s.forgetRigStatus(rig.ID)
	s.rigStatusNow(bg, rig)
	finish("paired", "Paired. Its games are being added to your library.")
	s.Log.Info("rig paired", "rig", rig.Name, "fingerprint", gamestream.CertFingerprint(cert))
	if _, err := s.syncRig(bg, rig); err != nil {
		s.Log.Warn("rig sync", "rig", rig.Name, "err", err)
	}
}

func pairMessage(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "Pairing was cancelled."
	case errors.Is(err, gamestream.ErrWrongPIN), errors.Is(err, gamestream.ErrPairTimedOut), errors.Is(err, gamestream.ErrAlreadyPairing),
		errors.Is(err, gamestream.ErrImposter), errors.Is(err, gamestream.ErrTooOld):
		return err.Error() + "."
	}
	return "Pairing didn't finish: " + err.Error() + "."
}

// GET /api/rigs/{id}/pair: how pairing is going. The PIN stays visible while it's waiting.
func (s *Server) handleRigPairStatus(w http.ResponseWriter, r *http.Request) {
	rig := s.rigForRequest(w, r)
	if rig == nil {
		return
	}
	g := s.rigs()
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.pairs[rig.ID]
	if p == nil {
		writeJSON(w, 200, rigPairing{State: "none"})
		return
	}
	writeJSON(w, 200, p)
}

// DELETE /api/rigs/{id}/pair: stop waiting.
func (s *Server) handleRigPairCancel(w http.ResponseWriter, r *http.Request) {
	rig := s.rigForRequest(w, r)
	if rig == nil {
		return
	}
	g := s.rigs()
	g.mu.Lock()
	if p := g.pairs[rig.ID]; p != nil && p.cancel != nil {
		p.cancel()
	}
	g.mu.Unlock()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// POST /api/rigs/{id}/devices {pin, device, username, password}: type a PIN into Sunshine for
// another device. Sunshine's sign-in is passed through once to the pinned certificate, and is
// never kept or logged, as it can run any command on that PC.
func (s *Server) handleRigDevicePIN(w http.ResponseWriter, r *http.Request) {
	rig := s.rigForRequest(w, r)
	if rig == nil {
		return
	}
	var b struct {
		PIN      string `json:"pin"`
		Device   string `json:"device"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "That request couldn't be read.")
		return
	}
	if rig.Cert == "" {
		writeErr(w, 409, "Pair this PC with the server first.")
		return
	}
	c, err := s.rigClient(rig)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	err = c.SubmitPIN(r.Context(), b.Username, b.Password, b.PIN, b.Device)
	b.Password = ""
	if err != nil {
		code := 400
		if errors.Is(err, gamestream.ErrNoAnswer) || errors.Is(err, gamestream.ErrRefused) {
			code = 502
		}
		writeErr(w, code, err.Error())
		return
	}
	s.Log.Info("a device was paired with a rig", "rig", rig.Name, "by", userFrom(r).Name)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// desktopLike are apps that hand over the whole computer rather than a game. They stay out of
// the library until an admin says otherwise.
var desktopLike = regexp.MustCompile(`(?i)\b(desktop|terminal|powershell|command prompt|cmd|file explorer|task manager)\b`)

// rigApp is one app in an admin's list.
type rigApp struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Shown  bool   `json:"shown"`
	ItemID *int64 `json:"itemId"`
}

func (s *Server) rigAppList(ctx context.Context, rigID int64) ([]rigApp, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT app_id, title, shown, item_id FROM wittysunshine_apps WHERE rig_id = ? ORDER BY title COLLATE NOCASE`, rigID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rigApp{}
	for rows.Next() {
		var a rigApp
		var item sql.NullInt64
		if err := rows.Scan(&a.ID, &a.Title, &a.Shown, &item); err != nil {
			return nil, err
		}
		if item.Valid {
			a.ItemID = &item.Int64
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// syncRig brings a rig's library in line with what the PC offers. When the PC doesn't answer
// nothing changes, so a sleeping PC keeps its games. Art comes from the PC where it has any.
func (s *Server) syncRig(ctx context.Context, rig *rigRow) ([]rigApp, error) {
	g := s.rigs()
	g.mu.Lock()
	lock := g.syncing[rig.ID]
	if lock == nil {
		lock = &sync.Mutex{}
		g.syncing[rig.ID] = lock
	}
	g.lastSync[rig.ID] = time.Now()
	g.mu.Unlock()
	lock.Lock()
	defer lock.Unlock()

	c, err := s.rigClient(rig)
	if err != nil {
		return nil, err
	}
	apps, err := c.Apps(ctx)
	if err != nil {
		return nil, err
	}
	if len(apps) > rigMaxApps {
		apps = apps[:rigMaxApps]
	}
	known, err := s.rigAppList(ctx, rig.ID)
	if err != nil {
		return nil, err
	}
	before := map[int]rigApp{}
	for _, a := range known {
		before[a.ID] = a
	}
	// A picture shared by several apps is Sunshine's default: that's no art, and a tile with
	// the game's name looks better.
	art := map[int]*string{} // a missing entry means the PC couldn't say, which changes nothing
	sums := map[[32]byte][]int{}
	images := map[int][]byte{}
	exts := map[int]string{}
	for _, app := range apps {
		if data, ext, err := c.AppImage(ctx, app.ID); err == nil {
			images[app.ID], exts[app.ID] = data, ext
			sum := sha256.Sum256(data)
			sums[sum] = append(sums[sum], app.ID)
		}
	}
	if err := os.MkdirAll(s.Cfg.ImagesDir, 0o755); err != nil {
		return nil, err
	}
	for _, ids := range sums {
		if len(ids) > 1 {
			for _, id := range ids {
				art[id] = new(string)
			}
			continue
		}
		name := fmt.Sprintf("rig-%d-%d%s", rig.ID, ids[0], exts[ids[0]])
		p := filepath.Join(s.Cfg.ImagesDir, name)
		tmp := p + ".tmp"
		if err := os.WriteFile(tmp, images[ids[0]], 0o644); err != nil {
			continue
		}
		if err := os.Rename(tmp, p); err == nil {
			poster := "cache:" + name
			art[ids[0]] = &poster
		}
	}
	tagline := "Plays on " + rig.Name
	var fresh []int64
	seen := map[int]bool{}
	for _, app := range apps {
		seen[app.ID] = true
		old, had := before[app.ID]
		shown := !desktopLike.MatchString(app.Title)
		if had {
			shown = old.Shown
		}
		var itemID any
		if shown {
			id, err := s.Store.SaveRigGame(ctx, rig.LibraryID, library.RigPath(rig.ID, app.ID), app.Title, tagline, art[app.ID])
			if err != nil {
				return nil, err
			}
			itemID = id
			if !had || old.ItemID == nil {
				fresh = append(fresh, id)
			}
		} else if had && old.ItemID != nil {
			_ = s.Store.RemoveRigGame(ctx, *old.ItemID)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO wittysunshine_apps(rig_id, app_id, title, shown, item_id) VALUES(?,?,?,?,?)
			ON CONFLICT(rig_id, app_id) DO UPDATE SET title = excluded.title, shown = excluded.shown, item_id = excluded.item_id`,
			rig.ID, app.ID, app.Title, shown, itemID); err != nil {
			return nil, err
		}
	}
	for _, old := range known {
		if seen[old.ID] {
			continue
		}
		if old.ItemID != nil {
			_ = s.Store.RemoveRigGame(ctx, *old.ItemID)
		}
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM wittysunshine_apps WHERE rig_id = ? AND app_id = ?`, rig.ID, old.ID)
	}
	s.enrichRigGames(fresh)
	return s.rigAppList(ctx, rig.ID)
}

// enrichRigGames queues new games for the metadata sources, in the background. Each source
// has its own setting, as for any other game.
func (s *Server) enrichRigGames(items []int64) {
	if len(items) == 0 || s.GameMeta == nil {
		return
	}
	go func() {
		for _, id := range items {
			if err := s.Store.EnrichRigGame(context.Background(), id); err != nil {
				s.Log.Debug("rig game details", "item", id, "err", err)
			}
		}
		s.GameMeta.Kick()
	}()
}

// syncSoon refreshes a rig's games now and then, when someone is looking at them.
func (s *Server) syncSoon(rig *rigRow) {
	g := s.rigs()
	g.mu.Lock()
	due := time.Since(g.lastSync[rig.ID]) > rigSyncEvery
	if due {
		g.lastSync[rig.ID] = time.Now()
	}
	g.mu.Unlock()
	if !due {
		return
	}
	copyOf := *rig
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if _, err := s.syncRig(ctx, &copyOf); err != nil {
			s.Log.Debug("rig sync", "rig", copyOf.Name, "err", err)
		}
	}()
}

func (s *Server) removeRigArt(rigID int64) {
	matches, _ := filepath.Glob(filepath.Join(s.Cfg.ImagesDir, fmt.Sprintf("rig-%d-*", rigID)))
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

// POST /api/rigs/{id}/sync: read the PC's games again.
func (s *Server) handleRigSync(w http.ResponseWriter, r *http.Request) {
	rig := s.rigForRequest(w, r)
	if rig == nil {
		return
	}
	apps, err := s.syncRig(r.Context(), rig)
	if err != nil {
		writeErr(w, 502, statusFor(rig.Cert != "", nil, err).Message)
		return
	}
	writeJSON(w, 200, apps)
}

// GET /api/rigs/{id}/apps: everything the PC offers, shown in the library or not.
func (s *Server) handleRigApps(w http.ResponseWriter, r *http.Request) {
	rig := s.rigForRequest(w, r)
	if rig == nil {
		return
	}
	apps, err := s.rigAppList(r.Context(), rig.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, apps)
}

// PUT /api/rigs/{id}/apps/{app} {shown}: whether an app is in the library.
func (s *Server) handleRigAppShown(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rig := s.rigForRequest(w, r)
	if rig == nil {
		return
	}
	appID, err := strconv.Atoi(r.PathValue("app"))
	if err != nil {
		writeErr(w, 400, "bad app")
		return
	}
	var b struct {
		Shown bool `json:"shown"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var title string
	var item sql.NullInt64
	if err := s.DB.QueryRowContext(ctx, `SELECT title, item_id FROM wittysunshine_apps WHERE rig_id = ? AND app_id = ?`, rig.ID, appID).Scan(&title, &item); err != nil {
		writeErr(w, 404, "not found")
		return
	}
	var itemID any
	if b.Shown {
		id, err := s.Store.SaveRigGame(ctx, rig.LibraryID, library.RigPath(rig.ID, appID), title, "Plays on "+rig.Name, nil)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		itemID = id
	} else if item.Valid {
		_ = s.Store.RemoveRigGame(ctx, item.Int64)
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE wittysunshine_apps SET shown = ?, item_id = ? WHERE rig_id = ? AND app_id = ?`, b.Shown, itemID, rig.ID, appID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	apps, err := s.rigAppList(ctx, rig.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, apps)
}

// POST /api/rigs/{id}/wake: send the wake-up packet, only to a registered rig's own MAC address
// and only for someone who can see that rig. From here it even works while you're away from
// home, as this server is on the rig's network.
func (s *Server) handleRigWake(w http.ResponseWriter, r *http.Request) {
	rig := s.rigForRequest(w, r)
	if rig == nil {
		return
	}
	if rig.MAC == "" {
		writeErr(w, 409, "This PC hasn't told the server how to wake it yet. Turn it on by hand once while it's paired, then this will work.")
		return
	}
	g := s.rigs()
	g.mu.Lock()
	if time.Since(g.lastWake[rig.ID]) < rigWakeEvery {
		g.mu.Unlock()
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	g.lastWake[rig.ID] = time.Now()
	delete(g.statuses, rig.ID)
	g.mu.Unlock()
	if err := gamestream.Wake(rig.MAC, rig.Address); err != nil {
		writeErr(w, 500, "The wake-up signal couldn't be sent: "+err.Error())
		return
	}
	s.Log.Info("rig woken", "rig", rig.Name, "by", userFrom(r).Name)
	writeJSON(w, 200, map[string]any{"ok": true, "note": "A wake-up signal was sent. A PC usually takes under a minute, if Wake-on-LAN is on in its settings."})
}

// POST /api/rigs/{id}/quit: end what's running on the PC. For admins, and for whoever started
// it from here: ending someone else's game isn't something a viewer gets to do.
func (s *Server) handleRigQuit(w http.ResponseWriter, r *http.Request) {
	rig := s.rigForRequest(w, r)
	if rig == nil {
		return
	}
	u := userFrom(r)
	g := s.rigs()
	g.mu.Lock()
	starter := g.starter[rig.ID]
	g.mu.Unlock()
	if !u.IsAdmin && starter != u.ID {
		writeErr(w, 403, "Only whoever started it, or an admin, can end what's running on this PC.")
		return
	}
	c, err := s.rigClient(rig)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := c.Quit(r.Context()); err != nil {
		writeErr(w, 502, statusFor(true, nil, err).Message)
		return
	}
	g.mu.Lock()
	delete(g.starter, rig.ID)
	delete(g.statuses, rig.ID)
	g.mu.Unlock()
	s.Log.Info("rig game ended", "rig", rig.Name, "by", u.Name)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// rigGameFor loads the game in the path, if this account may see it.
func (s *Server) rigGameFor(w http.ResponseWriter, r *http.Request) (*library.Item, *rigRow, int) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil, nil, 0
	}
	it, err := s.visibleItem(r, id)
	if err != nil || it.Kind != "game" || it.Console != library.RigConsole {
		writeErr(w, 404, "not found")
		return nil, nil, 0
	}
	rig, appID, err := s.rigOfItem(r.Context(), it.ID)
	if err != nil {
		writeErr(w, 404, "not found")
		return nil, nil, 0
	}
	return it, rig, appID
}

func (s *Server) rigOfItem(ctx context.Context, itemID int64) (*rigRow, int, error) {
	var rigID int64
	var appID int
	if err := s.DB.QueryRowContext(ctx, `SELECT rig_id, app_id FROM wittysunshine_apps WHERE item_id = ?`, itemID).Scan(&rigID, &appID); err != nil {
		return nil, 0, err
	}
	rig, err := s.rigByID(ctx, rigID)
	return rig, appID, err
}

// rigPlayView is everything a device needs to get to a game.
type rigPlayView struct {
	Rig     rigView                 `json:"rig"`
	Game    rigPlayGame             `json:"game"`
	Away    bool                    `json:"away"`
	Ready   bool                    `json:"ready"`
	Message string                  `json:"message,omitempty"`
	Methods []wittymoonlight.Method `json:"methods"`
	Install *wittymoonlight.Install `json:"install,omitempty"`
	Setup   []string                `json:"setup,omitempty"`
}

type rigPlayGame struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

var (
	rigClients = map[string]bool{"browser": true, "desktop": true, "android": true, "apple": true}
	rigSystems = map[string]bool{"windows": true, "mac": true, "linux": true, "chromeos": true, "android": true, "androidtv": true, "firetv": true, "ios": true, "tvos": true}
)

// rigAddressFor is the address this person's Moonlight should use from where they are, or
// nothing. A PC that's the server itself is known here as this machine, which means nothing to
// anyone else, so they get the name they reached this server by.
func rigAddressFor(r *http.Request, rig *rigRow) (string, bool) {
	where := clientFrom(r).Where
	away := !onLocalNetwork(r)
	if away {
		return rig.RemoteAddress, true
	}
	if isLoopbackHost(rig.Address) && where != remote.Machine {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if clean, _, err := gamestream.CleanAddress(host); err == nil {
			return clean, false
		}
		return "", false
	}
	return rig.Address, false
}

func isLoopbackHost(a string) bool {
	if a == "localhost" {
		return true
	}
	ip := net.ParseIP(a)
	return ip != nil && ip.IsLoopback()
}

// POST /api/rigs/games/{id}/play {client, os}: what this device can do to play this game. It
// starts nothing on the PC. Playing in the page is offered if this server has its Moonlight
// program, and nobody else's game is running there.
func (s *Server) handleRigPlay(w http.ResponseWriter, r *http.Request) {
	it, rig, appID := s.rigGameFor(w, r)
	if it == nil {
		return
	}
	var b wittymoonlight.Device
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !rigClients[b.Client] {
		b.Client = "browser"
	}
	if !rigSystems[b.OS] {
		b.OS = ""
	}
	ctx := r.Context()
	u := userFrom(r)
	st := s.rigStatus(ctx, rig)
	addr, away := rigAddressFor(r, rig)
	t := wittymoonlight.Target{RigName: rig.Name, Address: addr, Port: rig.Port, UUID: rig.UUID, AppID: appID, AppName: it.Title, Away: away}
	view := rigPlayView{Rig: s.rigViewFor(ctx, r, rig), Game: rigPlayGame{ID: it.ID, Title: it.Title}, Away: away, Install: wittymoonlight.InstallFor(b.OS)}
	view.Ready = st.State == "online" || st.State == "busy"
	if st.State == "busy" && st.currentApp != appID {
		view.Ready = false
		who := "Another game"
		if st.CurrentGame != "" {
			who = st.CurrentGame
		}
		view.Message = who + " is running on this PC, so this one can't start until it's closed."
	} else if !view.Ready {
		view.Message = view.Rig.Message
	}
	if addr != "" && b.OS != "" {
		view.Setup = wittymoonlight.SetupSteps(b.OS, rig.Name, addr, rig.Port)
	}
	if away && addr == "" {
		view.Message = strings.TrimSpace(view.Message + " You're away from home, and Moonlight can't reach this PC from here unless you connect both to a private network first: see the guide.")
	}
	inPage := view.Ready && s.moonlightProgram() != "" && s.rigMayJoin(rig, st, appID, u.ID, u.IsAdmin)
	view.Methods = wittymoonlight.Plan(b, t, inPage, "")
	if view.Methods == nil {
		view.Methods = []wittymoonlight.Method{}
	}
	writeJSON(w, 200, view)
}

// rigMayJoin: if something is running, a browser stream would join it rather than start
// anything, so that's only for whoever started it, or an admin.
func (s *Server) rigMayJoin(rig *rigRow, st rigState, appID int, user int64, admin bool) bool {
	if st.currentApp == 0 {
		return true
	}
	if st.currentApp != appID {
		return false
	}
	if admin {
		return true
	}
	g := s.rigs()
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.starter[rig.ID] == user
}

// POST /api/rigs/games/{id}/start: start a game on the PC from here, so a device with no way to
// be handed a game (an Apple TV) can pick it up in Moonlight with Resume.
func (s *Server) handleRigStart(w http.ResponseWriter, r *http.Request) {
	it, rig, appID := s.rigGameFor(w, r)
	if it == nil {
		return
	}
	ctx := r.Context()
	c, err := s.rigClient(rig)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	u := userFrom(r)
	if err := c.StartOnly(ctx, appID); err != nil {
		if errors.Is(err, gamestream.ErrBusy) {
			writeErr(w, 409, "Another game is running on this PC, so this one can't start until it's closed.")
			return
		}
		var se *gamestream.StatusError
		if errors.As(err, &se) {
			writeErr(w, 502, "The PC couldn't start it. "+se.Message)
			return
		}
		writeErr(w, 502, statusFor(true, nil, err).Message)
		return
	}
	g := s.rigs()
	g.mu.Lock()
	g.starter[rig.ID] = u.ID
	delete(g.statuses, rig.ID)
	g.mu.Unlock()
	s.Log.Info("rig game started", "rig", rig.Name, "game", it.Title, "by", u.Name)
	writeJSON(w, 200, map[string]any{"ok": true, "steps": []string{"Open the Moonlight app.", "Choose " + rig.Name + ".", "Choose " + it.Title + ", then Resume."}})
}
