// Package modules runs the plugins that can be updated without a new server:
// which version is in use, why, and getting a newer one. docs/MODULES.md has the
// design.
//
// A plugin's built-in copy is always there and always works. A downloaded version
// is used only while it is signed, in range for this server and behaving, so
// nothing here can stop the server from running.
package modules

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

// Phases of an install, shown in Settings.
const (
	PhaseIdle        = "idle"
	PhaseDownloading = "downloading"
	PhaseVerifying   = "verifying"
	PhaseInstalling  = "installing"
	PhaseFailed      = "failed"
)

// What a module is doing.
const (
	StateBuiltin   = "builtin"   // its built-in copy, as expected
	StateInstalled = "installed" // a downloaded version
	StateFallback  = "fallback"  // a downloaded version was refused; see Notice
)

// firstCheck is the pause before the first look for updates after a start.
var firstCheck = 3 * time.Minute

// failLimit is how many calls in a row may fail before a version is given up on.
const failLimit = 3

// Report is what Settings shows.
type Report struct {
	Server     string   `json:"server"` // this server's version
	API        int      `json:"api"`    // the newest module API it has
	MinAPI     int      `json:"minApi"` // the oldest it still takes
	CheckedAt  int64    `json:"checkedAt,omitempty"`
	CheckError string   `json:"checkError,omitempty"`
	Modules    []Status `json:"modules"`
}

// Status is one module.
type Status struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Version        string `json:"version"` // the version in use
	BuiltinVersion string `json:"builtinVersion"`
	Delivery       string `json:"delivery"`
	Updatable      bool   `json:"updatable"` // can be updated without a new server
	MinServer      string `json:"minServer"` // of the version in use
	MaxServer      string `json:"maxServer"`
	API            int    `json:"api"`
	Repo           string `json:"repo"`
	State          string `json:"state"`
	Notice         string `json:"notice,omitempty"`   // why a version isn't in use, in plain words
	Previous       string `json:"previous,omitempty"` // an installed version to go back to
	CanRollback    bool   `json:"canRollback"`
	Latest         *Offer `json:"latest,omitempty"`
	TooNew         *Offer `json:"tooNew,omitempty"` // a newer version that doesn't fit this server
	Phase          string `json:"phase"`
	Done           int64  `json:"done,omitempty"` // bytes downloaded
	Total          int64  `json:"total,omitempty"`
	Error          string `json:"error,omitempty"` // why the last install failed
}

// Offer is a release of a module.
type Offer struct {
	Version    string `json:"version"`
	MinServer  string `json:"minServer"`
	MaxServer  string `json:"maxServer"`
	Notes      string `json:"notes,omitempty"`
	URL        string `json:"url,omitempty"`
	Published  int64  `json:"published,omitempty"` // unix ms
	Prerelease bool   `json:"prerelease"`
	Size       int64  `json:"size,omitempty"`
	Reason     string `json:"reason,omitempty"`  // why it doesn't fit
	Blocked    string `json:"blocked,omitempty"` // why it was taken out of use before
}

// moduleState is what is remembered across restarts about one module.
type moduleState struct {
	Active   string            `json:"active,omitempty"`   // the chosen installed version
	Previous string            `json:"previous,omitempty"` // the one before it
	Blocked  map[string]string `json:"blocked,omitempty"`  // versions taken out of use, and why
}

type entry struct {
	builtin *wittymodule.Manifest
	state   moduleState

	man    *wittymodule.Manifest // the version in use
	proc   *proc                 // nil: the built-in copy
	notice string
	fails  int

	rel        *release
	latest     *Offer
	tooNew     *Offer
	checkError string

	busy        bool
	phase       string
	done, total int64
	lastError   string
}

// Config prepares a Manager.
type Config struct {
	Dir       string // <data>/modules
	Server    string // this server's version
	Builtins  [][]byte
	Transport http.RoundTripper // internet-only transport for GitHub
	Log       *slog.Logger
	// CheckEnabled and AutoInstall read the owner's settings. Without them, nothing
	// happens on its own.
	CheckEnabled func() bool
	AutoInstall  func() bool
	// LogEnv is how the owner wants a module logged, as environment for its helper.
	LogEnv func(id string) []string
	// For tests.
	Keys  []ed25519.PublicKey
	Fetch *update.Fetcher
}

// Manager keeps track of every module.
type Manager struct {
	cfg   Config
	dir   string
	keys  []ed25519.PublicKey
	fetch *update.Fetcher
	log   *slog.Logger

	mu       sync.Mutex
	mods     map[string]*entry
	order    []string
	checked  time.Time
	checkErr string
	checking chan struct{}
	closed   bool
}

// New reads what is installed and picks the version of each module to use. A
// manifest that can't be read is a build error caught by tests, so it is only
// logged here; nothing about modules can stop a server from starting.
func New(cfg Config) *Manager {
	m := &Manager{cfg: cfg, dir: cfg.Dir, keys: cfg.Keys, fetch: cfg.Fetch, log: cfg.Log, mods: map[string]*entry{}}
	if m.log == nil {
		m.log = slog.Default()
	}
	m.log = m.log.With("cat", "modules")
	if m.keys == nil {
		m.keys = update.TrustedKeys()
	}
	if m.fetch == nil {
		m.fetch = &update.Fetcher{UserAgent: "WittyMediaServer/" + cfg.Server, Transport: cfg.Transport}
	}
	saved := m.loadState()
	for _, raw := range cfg.Builtins {
		man, err := wittymodule.ParseManifest(raw)
		if err != nil {
			m.log.Error("modules: a built-in manifest is unreadable", "err", err)
			continue
		}
		e := &entry{builtin: man, man: man, phase: PhaseIdle, state: saved[man.ID]}
		m.mods[man.ID] = e
		m.order = append(m.order, man.ID)
	}
	sort.Strings(m.order)
	for _, id := range m.order {
		m.resolve(id, m.mods[id])
		m.prune(id, m.mods[id])
	}
	return m
}

func (m *Manager) modDir(id string) string { return filepath.Join(m.dir, id) }

func (m *Manager) versionDir(id, ver string) (string, error) {
	if !wittymodule.Valid(ver) {
		return "", fmt.Errorf("%q is not a version", ver)
	}
	return filepath.Join(m.modDir(id), "versions", ver), nil
}

func (m *Manager) statePath() string { return filepath.Join(m.dir, "state.json") }

func (m *Manager) loadState() map[string]moduleState {
	saved := map[string]moduleState{}
	b, err := os.ReadFile(m.statePath())
	if err != nil {
		return saved
	}
	if err := json.Unmarshal(b, &saved); err != nil {
		// Only the choice of version is lost: every module falls back to its
		// built-in copy until the next install.
		m.log.Warn("modules: the saved state is unreadable; using built-in copies", "err", err)
		return map[string]moduleState{}
	}
	return saved
}

// saveState writes the state so it is either the old or the new one whole, even
// if the power goes in between. Call with m.mu held.
func (m *Manager) saveState() error {
	saved := map[string]moduleState{}
	for id, e := range m.mods {
		if e.state.Active != "" || e.state.Previous != "" || len(e.state.Blocked) > 0 {
			saved[id] = e.state
		}
	}
	b, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(m.dir, ".state-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, m.statePath())
}

// usable checks an installed version: signed, unchanged, for this module and
// compatible with this server.
func (m *Manager) usable(id, ver string) (*wittymodule.Manifest, string, error) {
	dir, err := m.versionDir(id, ver)
	if err != nil {
		return nil, "", err
	}
	b, err := os.ReadFile(filepath.Join(dir, wittymodule.ManifestFile))
	if err != nil {
		return nil, "", errors.New("it is no longer on disk")
	}
	man, err := wittymodule.ParseManifest(b)
	if err != nil {
		return nil, "", err
	}
	if man.ID != id || man.Version != ver {
		return nil, "", fmt.Errorf("its files say %s %s", man.ID, man.Version)
	}
	if err := verifyInstalled(dir, id, ver, m.keys); err != nil {
		return nil, "", err
	}
	if err := man.Supports(m.cfg.Server, wittymodule.MinAPI, wittymodule.API); err != nil {
		return nil, "", err
	}
	if man.Delivery != wittymodule.DeliveryHelper {
		return nil, "", fmt.Errorf("this server can't run a module delivered as %q yet", man.Delivery)
	}
	return man, dir, nil
}

// resolve picks the version to use: the chosen one, then the one before it, then
// the built-in copy, which is always compatible because it was compiled in. Call
// with m.mu held, or before m is shared.
func (m *Manager) resolve(id string, e *entry) {
	name := e.builtin.Name
	var why []string
	var pick *wittymodule.Manifest
	var pickDir string
	for _, ver := range []string{e.state.Active, e.state.Previous} {
		if ver == "" {
			continue
		}
		if r := e.state.Blocked[ver]; r != "" {
			why = append(why, name+" "+ver+" is out of use: "+r)
			continue
		}
		if wittymodule.Compare(ver, e.builtin.Version) <= 0 {
			why = append(why, "The built-in "+name+" "+e.builtin.Version+" is the same or newer than the installed "+ver+".")
			continue
		}
		man, dir, err := m.usable(id, ver)
		if err != nil {
			m.log.Warn("modules: not using an installed version", "module", id, "version", ver, "err", err)
			msg := sentence(err.Error())
			if !strings.HasPrefix(msg, name+" "+ver) {
				msg = name + " " + ver + " can't be used: " + msg
			}
			why = append(why, msg)
			continue
		}
		pick, pickDir = man, dir
		break
	}
	if e.proc != nil {
		logging.Go("module close", e.proc.Close, "cat", "modules")
		e.proc = nil
	}
	e.fails = 0
	if pick != nil {
		e.man, e.proc = pick, newProc(pick, pickDir, func() error { return verifyInstalled(pickDir, id, pick.Version, m.keys) }, m.log)
		e.proc.logEnv = m.cfg.LogEnv
	} else {
		e.man = e.builtin
	}
	e.notice = ""
	if len(why) > 0 {
		using := "the built-in copy (" + e.builtin.Version + ")"
		if pick != nil {
			using = "version " + pick.Version
		}
		e.notice = strings.Join(why, " ") + " Using " + using + "."
	}
}

// sentence ends a message with a full stop if it needs one.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s != "" && !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// updatable: a release of this module can reach this server on its own.
func updatable(man *wittymodule.Manifest) bool { return man.Delivery == wittymodule.DeliveryHelper }

func (e *entry) status(id string) Status {
	st := Status{ID: id, Name: e.builtin.Name, Version: e.man.Version, BuiltinVersion: e.builtin.Version, Delivery: e.builtin.Delivery,
		Updatable: updatable(e.builtin), MinServer: e.man.MinServer, MaxServer: e.man.MaxServer, API: e.man.API, Repo: e.builtin.Repo,
		Notice: e.notice, Latest: e.latest, TooNew: e.tooNew, Phase: e.phase, Done: e.done, Total: e.total, Error: e.lastError, Previous: e.state.Previous}
	switch {
	case e.proc != nil && e.notice == "":
		st.State = StateInstalled
	case e.notice != "":
		st.State = StateFallback
	default:
		st.State = StateBuiltin
	}
	st.CanRollback = e.proc != nil && !e.busy
	if st.Previous != "" && e.state.Blocked[st.Previous] != "" {
		st.Previous = ""
	}
	return st
}

// Report describes every module.
func (m *Manager) Report() Report {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := Report{Server: m.cfg.Server, API: wittymodule.API, MinAPI: wittymodule.MinAPI, CheckError: m.checkErr, Modules: []Status{}}
	if !m.checked.IsZero() {
		r.CheckedAt = m.checked.UnixMilli()
	}
	for _, id := range m.order {
		r.Modules = append(r.Modules, m.mods[id].status(id))
	}
	return r
}

// ErrUnknown: no module has that id.
var ErrUnknown = errors.New("there is no such module")

// call runs one call on a module's helper. handled is false when the caller
// should use the built-in copy instead: there is no helper in use, or it failed.
func (m *Manager) call(id, method string, params, out any) (blob []byte, err error, handled bool) {
	m.mu.Lock()
	e := m.mods[id]
	var p *proc
	if e != nil {
		p = e.proc
	}
	m.mu.Unlock()
	if p == nil {
		return nil, nil, false
	}
	blob, err = p.Call(method, params, out)
	var ce *wittymodule.CallError
	if err == nil || errors.As(err, &ce) {
		m.mu.Lock()
		if e.proc == p {
			e.fails = 0
		}
		m.mu.Unlock()
		return blob, err, true
	}
	m.failed(id, e, p, err)
	return nil, nil, false
}

// failed notes a helper failing. Changed files end its use at once; anything else
// gets a few tries, since one crash may be chance.
func (m *Manager) failed(id string, e *entry, p *proc, err error) {
	m.log.Warn("modules: a helper failed; using the built-in copy for this call", "module", id, "version", p.man.Version, "err", err)
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.proc != p {
		return
	}
	e.fails++
	if e.fails < failLimit && !errors.Is(err, errUntrusted) {
		return
	}
	ver := p.man.Version
	if e.state.Blocked == nil {
		e.state.Blocked = map[string]string{}
	}
	e.state.Blocked[ver] = "it kept failing (" + firstLine(err.Error()) + ")"
	if errors.Is(err, errUntrusted) {
		e.state.Blocked[ver] = sentence(err.Error())
	}
	m.log.Error("modules: taking a version out of use", "module", id, "version", ver, "err", err)
	if err := m.saveState(); err != nil {
		m.log.Warn("modules: saving the state", "err", err)
	}
	m.resolve(id, e)
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// Rollback takes the version in use out of use, going back to the previous one or
// the built-in copy. It isn't installed on its own again, though an admin may.
func (m *Manager) Rollback(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil {
		return ErrUnknown
	}
	if e.busy {
		return errors.New("an update of this module is being installed")
	}
	if e.proc == nil {
		return errors.New("this module is already using its built-in copy")
	}
	ver := e.man.Version
	if e.state.Blocked == nil {
		e.state.Blocked = map[string]string{}
	}
	e.state.Blocked[ver] = "an admin rolled it back on " + time.Now().Format("2006-01-02")
	old := e.state
	e.state.Active, e.state.Previous = old.Previous, ""
	if err := m.saveState(); err != nil {
		e.state = old
		delete(e.state.Blocked, ver)
		return err
	}
	m.resolve(id, e)
	// The rollback is what the admin wanted, so it isn't reported as a problem.
	e.notice = ""
	m.log.Info("modules: rolled back", "module", id, "from", ver, "to", e.man.Version)
	return nil
}

// prune removes installed versions that are no longer needed and anything left
// from an install that didn't finish. A file still in use stays until next time.
// Call with m.mu held, or before m is shared.
func (m *Manager) prune(id string, e *entry) {
	dir := m.modDir(id)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, d := range entries {
		if strings.HasPrefix(d.Name(), ".work-") && !e.busy {
			_ = os.RemoveAll(filepath.Join(dir, d.Name()))
		}
	}
	versions, err := os.ReadDir(filepath.Join(dir, "versions"))
	if err != nil {
		return
	}
	for _, d := range versions {
		if d.Name() != e.state.Active && d.Name() != e.state.Previous {
			_ = os.RemoveAll(filepath.Join(dir, "versions", d.Name()))
		}
	}
}

// Close stops every helper.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for _, e := range m.mods {
		if e.proc != nil {
			e.proc.Close()
		}
	}
}

// Run looks for updates after a pause at start and then twice a day, while the
// owner's settings allow it, and installs them if the settings say so too.
func (m *Manager) Run(ctx context.Context) {
	wait := firstCheck
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = 12 * time.Hour
		if m.cfg.CheckEnabled == nil || !m.cfg.CheckEnabled() || !wittymodule.Valid(m.cfg.Server) {
			continue
		}
		m.Check(ctx, 6*time.Hour)
		if m.cfg.AutoInstall != nil && m.cfg.AutoInstall() {
			m.autoInstall()
		}
	}
}

func (m *Manager) autoInstall() {
	m.mu.Lock()
	var ids []string
	for _, id := range m.order {
		e := m.mods[id]
		if e.latest != nil && e.latest.Blocked == "" && e.rel != nil && !e.busy {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	for _, id := range ids {
		if err := m.Install(id); err != nil {
			m.log.Warn("modules: automatic install", "module", id, "err", err)
		} else {
			m.log.Info("modules: installing an update automatically", "module", id)
		}
	}
}
