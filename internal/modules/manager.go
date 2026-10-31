// Package modules runs the plugins that can be updated without a new server:
// which version is in use, why, and getting a newer or older one. docs/MODULES.md
// has the design.
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
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

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

// PinBuiltin is the pin of a module kept on its built-in copy.
const PinBuiltin = "builtin"

// firstCheck is the pause before the first look for updates after a start.
var firstCheck = 3 * time.Minute

const (
	// failLimit is how many calls in a row may fail before a version is given up on.
	failLimit = 3
	// A browser can't say a bundle worked, so its failures count within this long.
	failWindow = 24 * time.Hour
	maxEvents  = 20
)

// mayUseNetwork lists the plugins whose helpers may have a network, if their signed
// manifest asks. Everything else gets an empty one where the system allows, so a
// file that takes over a parser has nowhere to send anything.
var mayUseNetwork = map[string]bool{"wittytuner": true}

// Report is what Settings shows.
type Report struct {
	Server     string   `json:"server"` // this server's version
	API        int      `json:"api"`    // the newest module API it has
	MinAPI     int      `json:"minApi"` // the oldest it still takes
	CheckedAt  int64    `json:"checkedAt,omitempty"`
	CheckError string   `json:"checkError,omitempty"`
	HoldUntil  int64    `json:"holdUntil,omitempty"` // GitHub asked this network to wait until then
	Modules    []Status `json:"modules"`
	// Libraries is every kind of library a module provides, and whether it can be made.
	Libraries []LibraryType `json:"libraries"`
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

	// What the page needs in order to explain it.
	Group     string                 `json:"group,omitempty"`
	Summary   string                 `json:"summary,omitempty"`
	Needs     []string               `json:"needs,omitempty"`
	Updates   string                 `json:"updates,omitempty"` // what a release of its own changes
	Stays     string                 `json:"stays,omitempty"`   // what only a new server changes
	Pin       string                 `json:"pin,omitempty"`     // a version kept on purpose, or "builtin"
	Withdrawn *wittymodule.Withdrawn `json:"withdrawn,omitempty"`
	LastFail  string                 `json:"lastFailure,omitempty"`
	Events    []Event                `json:"events,omitempty"`

	// What the store needs.
	Installed  bool     `json:"installed"`
	Core       bool     `json:"core,omitempty"`      // other modules are built on it, so it stays
	Libraries  []string `json:"libraries,omitempty"` // the kinds of library it makes possible
	Requires   []string `json:"requires,omitempty"`
	RequiredBy []string `json:"requiredBy,omitempty"`

	// A fork in place of the module's own repository.
	StockRepo string      `json:"stockRepo"`
	Source    *SourceInfo `json:"source,omitempty"`
	NoFork    string      `json:"noFork,omitempty"` // why a fork can't replace it

	// AutoUpdate says this module may update by itself when the server's setting is on.
	AutoUpdate bool `json:"autoUpdate"`
}

// Event is one thing that happened to a module, newest kept, for the page's log.
type Event struct {
	At      int64  `json:"at"` // unix ms
	Kind    string `json:"kind"`
	Version string `json:"version,omitempty"`
	Detail  string `json:"detail,omitempty"`
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
	Pin      string            `json:"pin,omitempty"`      // automatic updates leave this alone
	// NoAuto: someone wants to choose this module's updates by hand. It keeps offering
	// them, and unlike a pin it says nothing about which version stays.
	NoAuto bool `json:"noAuto,omitempty"`
	// Chosen: the active version was picked by hand, so it is used even when the
	// built-in copy is newer.
	Chosen  bool    `json:"chosen,omitempty"`
	Removed bool    `json:"removed,omitempty"` // someone took it off this server
	Source  *Source `json:"source,omitempty"`  // a fork in place of its own repository
	// StockWithdrawn keeps the module's own list while a fork is in use, so going back
	// can't bring an older list in.
	StockWithdrawn *wittymodule.Withdrawals `json:"stockWithdrawn,omitempty"`
	DataHigh       int                      `json:"dataHigh,omitempty"`  // the newest data format used so far
	Withdrawn      *wittymodule.Withdrawals `json:"withdrawn,omitempty"` // the newest signed list seen
	Events         []Event                  `json:"events,omitempty"`
}

type entry struct {
	builtin *wittymodule.Manifest
	state   moduleState

	man      *wittymodule.Manifest // the version in use
	dir      string                // its folder; "" for the built-in copy
	proc     *proc                 // for a helper; nil otherwise
	web      *webFiles             // for a bundle; nil otherwise
	notice   string
	fails    int
	failedAt time.Time

	known      map[string]*knownRelease // verified, by version
	badFiles   map[string]bool          // signed lists that failed, so they aren't fetched each check
	feedLoaded bool
	etag       string
	rel        *release // the newest compatible release
	latest     *Offer
	tooNew     *Offer

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
	// Launcher is this server's own program, which boxes a helper in before it runs
	// (Linux; see RunSandboxed). Without it helpers have the rest of their limits.
	Launcher string
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

	mu        sync.Mutex
	mods      map[string]*entry
	order     []string
	checked   time.Time
	checkErr  string
	holdUntil time.Time
	checking  chan struct{}
	closed    bool
	attached  map[*Attached]bool
	labels    map[string]string // kinds of library someone gave a name of their own
	retiring  sync.WaitGroup    // helpers of versions no longer in use, on their way out
}

// New reads what is installed and picks the version of each module to use. A
// manifest that can't be read is a build error caught by tests, so it is only
// logged here; nothing about modules can stop a server from starting.
func New(cfg Config) *Manager {
	m := &Manager{cfg: cfg, dir: cfg.Dir, keys: cfg.Keys, fetch: cfg.Fetch, log: cfg.Log, mods: map[string]*entry{}, attached: map[*Attached]bool{}}
	if m.log == nil {
		m.log = slog.Default()
	}
	if m.keys == nil {
		m.keys = update.ModuleKeys()
	}
	if m.fetch == nil {
		m.fetch = &update.Fetcher{UserAgent: "WittyMediaServer/" + cfg.Server, Transport: cfg.Transport}
	}
	saved := m.loadState()
	m.loadLabels()
	for _, raw := range cfg.Builtins {
		man, err := wittymodule.ParseManifest(raw)
		if err != nil {
			m.log.Error("modules: a built-in manifest is unreadable", "err", err)
			continue
		}
		e := &entry{builtin: man, man: man, phase: PhaseIdle, state: saved[man.ID], known: map[string]*knownRelease{}}
		m.mods[man.ID] = e
		m.order = append(m.order, man.ID)
	}
	sort.Strings(m.order)
	for _, id := range m.order {
		m.checkSavedSource(id, m.mods[id])
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
		s := e.state
		if s.Active != "" || s.Previous != "" || len(s.Blocked) > 0 || s.Pin != "" || s.NoAuto || s.Removed || s.Source != nil || s.DataHigh > 0 || s.Withdrawn != nil || len(s.Events) > 0 {
			saved[id] = s
		}
	}
	b, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(m.statePath(), b)
}

// writeAtomic replaces a file whole, so a power cut leaves the old or the new one.
func writeAtomic(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".state-*")
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
	return os.Rename(tmp, path)
}

// note keeps a record for the page's log. Call with m.mu held, or before m is shared.
func (e *entry) note(kind, version, detail string) {
	e.state.Events = append(e.state.Events, Event{At: time.Now().UnixMilli(), Kind: kind, Version: version, Detail: firstLine(detail)})
	if n := len(e.state.Events); n > maxEvents {
		e.state.Events = append([]Event(nil), e.state.Events[n-maxEvents:]...)
	}
}

// dataRefusal says why a version can't take over the data that is there, in words
// for the owner; "" if it can. Nothing here ever changes the data.
func dataRefusal(name string, man *wittymodule.Manifest, high int) string {
	switch {
	case high == 0:
		return ""
	case man.DataFormat == 0 && man.DataFrom == 0:
		return "" // it keeps none of its own
	case high > man.DataFormat:
		return fmt.Sprintf("%s %s can't read what a newer version saved (data format %d, and it reads up to %d), so going back to it could lose data.", name, man.Version, high, man.DataFormat)
	case high < man.DataFrom:
		return fmt.Sprintf("%s %s can't read data this old (data format %d, and it needs at least %d). Install a version in between first.", name, man.Version, high, man.DataFrom)
	}
	return ""
}

// usable checks an installed version: signed, unchanged, for this module and
// compatible with this server.
func (m *Manager) usable(id string, e *entry, ver string) (*wittymodule.Manifest, string, error) {
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
	if err := verifyInstalled(dir, man, m.keysFor(e)); err != nil {
		return nil, "", err
	}
	if err := man.Supports(m.cfg.Server, wittymodule.MinAPI, wittymodule.API); err != nil {
		return nil, "", err
	}
	if man.Delivery != e.builtin.Delivery {
		return nil, "", fmt.Errorf("it is delivered as %q, and this server expects %q", man.Delivery, e.builtin.Delivery)
	}
	if !updatable(man) {
		return nil, "", fmt.Errorf("this server can't run a module delivered as %q", man.Delivery)
	}
	if r := protocolRefusal(e, man); r != "" {
		return nil, "", errors.New(r)
	}
	if r := dataRefusal(e.builtin.Name, man, e.state.DataHigh); r != "" {
		return nil, "", errors.New(r)
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
	// With nothing chosen, the built-in copy is the choice: Previous is what the chosen
	// version falls back on, not a choice of its own.
	var candidates []string
	if e.state.Active != "" {
		candidates = []string{e.state.Active, e.state.Previous}
	}
	for i, ver := range candidates {
		if ver == "" {
			continue
		}
		if r := e.state.Blocked[ver]; r != "" {
			why = append(why, name+" "+ver+" is out of use: "+r)
			continue
		}
		if w := e.state.Withdrawn.Find(ver); w != nil && w.Severity == wittymodule.SeveritySecurity {
			why = append(why, name+" "+ver+" was withdrawn for security reasons: "+sentence(w.Reason))
			continue
		}
		// A newer built-in copy simply wins, which isn't a problem to report. The
		// exception is an older version someone chose on purpose.
		if wittymodule.Compare(ver, e.builtin.Version) <= 0 && !(i == 0 && e.state.Chosen) {
			continue
		}
		man, dir, err := m.usable(id, e, ver)
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
		p := e.proc
		m.retiring.Add(1) // a call in progress can hold it up, so it isn't waited for here
		go func() { defer m.retiring.Done(); p.Close() }()
		e.proc = nil
	}
	e.fails, e.web, e.dir = 0, nil, ""
	if pick != nil {
		e.man, e.dir = pick, pickDir
		switch pick.Delivery {
		case wittymodule.DeliveryHelper:
			e.proc = m.newProcFor(id, e, pick, pickDir)
		case wittymodule.DeliveryWeb:
			w, err := loadWebFiles(pickDir, pick, m.keysFor(e))
			if err != nil { // usable has just checked it, so only a change this moment gets here
				m.log.Warn("modules: reading a bundle", "module", id, "err", err)
				e.man, e.dir = e.builtin, ""
				why = append(why, name+" "+pick.Version+" can't be used: "+sentence(err.Error()))
				pick = nil
			} else {
				e.web = w
			}
		}
	} else {
		e.man = e.builtin
	}
	if e.man.DataFormat > e.state.DataHigh {
		e.state.DataHigh = e.man.DataFormat
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

// newProcFor runs a version as a helper with what its signed manifest asks for and
// its kind of plugin may have. A fork never gets a network.
func (m *Manager) newProcFor(id string, e *entry, man *wittymodule.Manifest, dir string) *proc {
	keys := m.keysFor(e)
	p := newProc(man, dir, func() error { return verifyInstalled(dir, man, keys) }, m.log)
	p.network = mayUseNetwork[id] && man.Asks(wittymodule.PermissionNetwork) && e.state.Source == nil
	p.strict = e.state.Source != nil
	p.launch = m.cfg.Launcher
	return p
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
func updatable(man *wittymodule.Manifest) bool {
	switch man.Delivery {
	case wittymodule.DeliveryHelper, wittymodule.DeliveryProgram, wittymodule.DeliveryWeb:
		return true
	}
	return false
}

func (e *entry) status(id string) Status {
	st := Status{ID: id, Name: e.builtin.Name, Version: e.man.Version, BuiltinVersion: e.builtin.Version, Delivery: e.builtin.Delivery,
		Updatable: updatable(e.builtin), MinServer: e.man.MinServer, MaxServer: e.man.MaxServer, API: e.man.API, Repo: e.repo(),
		Notice: e.notice, Latest: e.latest, TooNew: e.tooNew, Phase: e.phase, Done: e.done, Total: e.total, Error: e.lastError, Previous: e.state.Previous,
		Group: e.man.Group, Summary: e.man.Summary, Needs: e.man.Needs, Updates: e.man.Updates, Stays: e.man.Stays, Pin: e.state.Pin,
		Installed: !e.state.Removed, Core: e.builtin.Core, Libraries: e.builtin.Libraries, Requires: e.builtin.Requires,
		StockRepo: e.builtin.Repo, Source: e.sourceInfo(), NoFork: noFork[id]}
	// A release from before a description was written, or a fork's that has none, still
	// gets one: it is the same module.
	if st.Group == "" {
		st.Group = e.builtin.Group
	}
	if st.Summary == "" {
		st.Summary = e.builtin.Summary
	}
	if len(st.Needs) == 0 {
		st.Needs = e.builtin.Needs
	}
	if st.Updates == "" {
		st.Updates = e.builtin.Updates
	}
	if st.Stays == "" {
		st.Stays = e.builtin.Stays
	}
	st.AutoUpdate = updatable(e.builtin) && !e.state.NoAuto && e.state.Source == nil
	inUse := e.dir != ""
	switch {
	case inUse && e.notice == "":
		st.State = StateInstalled
	case e.notice != "":
		st.State = StateFallback
	default:
		st.State = StateBuiltin
	}
	st.CanRollback = inUse && !e.busy
	if st.Previous != "" && (e.state.Blocked[st.Previous] != "" || wittymodule.Compare(st.Previous, e.builtin.Version) <= 0 || st.Previous == e.man.Version) {
		st.Previous = ""
	}
	st.Withdrawn = e.state.Withdrawn.Find(e.man.Version)
	if e.dir == "" {
		st.Withdrawn = nil // the built-in copy is what this server was built with
	}
	for i := len(e.state.Events) - 1; i >= 0; i-- {
		if ev := e.state.Events[i]; ev.Kind == "failed" || ev.Kind == "taken-out-of-use" {
			st.LastFail = ev.Detail
			break
		}
	}
	st.Events = append(st.Events, e.state.Events...)
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
	if time.Now().Before(m.holdUntil) {
		r.HoldUntil = m.holdUntil.UnixMilli()
	}
	for _, id := range m.order {
		m.loadFeed(id, m.mods[id])
		st := m.mods[id].status(id)
		st.RequiredBy = m.requiredBy(id)
		r.Modules = append(r.Modules, st)
	}
	r.Libraries = m.libraryTypes()
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
	m.failed(id, e, p.man.Version, err)
	return nil, nil, false
}

// failed notes a version failing. Changed files end its use at once; anything else
// gets a few tries, since one crash may be chance.
func (m *Manager) failed(id string, e *entry, ver string, err error) {
	m.log.Warn("modules: a version failed; using another for this call", "module", id, "version", ver, "err", err)
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.man.Version != ver || e.dir == "" {
		return
	}
	now := time.Now()
	if !e.failedAt.IsZero() && now.Sub(e.failedAt) > failWindow {
		e.fails = 0
	}
	e.failedAt = now
	e.fails++
	e.note("failed", ver, plainFailure(err))
	if e.fails < failLimit && !errors.Is(err, errUntrusted) {
		if serr := m.saveState(); serr != nil {
			m.log.Warn("modules: saving the state", "err", serr)
		}
		return
	}
	if e.state.Blocked == nil {
		e.state.Blocked = map[string]string{}
	}
	e.state.Blocked[ver] = "it kept failing (" + plainFailure(err) + ")"
	if errors.Is(err, errUntrusted) {
		e.state.Blocked[ver] = sentence(err.Error())
	}
	m.log.Error("modules: taking a version out of use", "module", id, "version", ver, "err", err)
	e.note("taken-out-of-use", ver, e.state.Blocked[ver])
	m.resolve(id, e)
	if err := m.saveState(); err != nil {
		m.log.Warn("modules: saving the state", "err", err)
	}
}

// plainFailure puts how a helper failed in words for the owner. A helper that dies
// mid-call shows up as the end of its pipe, which says nothing to anyone.
func plainFailure(err error) string {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "it stopped without answering"
	}
	return firstLine(err.Error())
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}

// browserReportEvery is how often a browser's report can count.
var browserReportEvery = time.Minute

// ReportBrowserFailure records a browser saying a bundle failed. It only applies to
// bundles: nothing a browser says counts against a helper or a program. Only an admin's
// report can take a version out of use. Anyone else's goes in the module's log for an
// admin to see: their own page has already fallen back, and a profile shouldn't be able to
// take a version, perhaps one with a security fix, away from everyone.
func (m *Manager) ReportBrowserFailure(id, ver, reason string, admin bool) error {
	m.mu.Lock()
	e := m.mods[id]
	if e == nil {
		m.mu.Unlock()
		return ErrUnknown
	}
	if e.builtin.Delivery != wittymodule.DeliveryWeb || e.man.Version != ver || e.dir == "" {
		m.mu.Unlock()
		return nil
	}
	if !admin {
		e.note("reported", ver, reason)
		if err := m.saveState(); err != nil {
			m.log.Warn("modules: saving the state", "err", err)
		}
		m.mu.Unlock()
		return nil
	}
	if !e.failedAt.IsZero() && time.Since(e.failedAt) < browserReportEvery {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	return m.ReportFailure(id, ver, reason)
}

// ReportFailure records a version failing where the server can't see it itself: a
// bundle in someone's browser, or a program during a game. Three within a day and
// the version goes out of use. A report about any other version is ignored, so an
// old tab can't count against what replaced it.
func (m *Manager) ReportFailure(id, ver, reason string) error {
	m.mu.Lock()
	e := m.mods[id]
	m.mu.Unlock()
	if e == nil {
		return ErrUnknown
	}
	if reason == "" {
		reason = "it failed"
	}
	if len(reason) > 500 {
		reason = reason[:500]
	}
	m.failed(id, e, ver, errors.New(reason))
	return nil
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
	if e.dir == "" {
		return errors.New("this module is already using its built-in copy")
	}
	ver := e.man.Version
	old := e.state
	blocked := map[string]string{ver: "an admin rolled it back on " + time.Now().Format("2006-01-02")}
	for k, v := range old.Blocked {
		if k != ver {
			blocked[k] = v
		}
	}
	e.state.Blocked = blocked
	if old.Previous == ver {
		e.state.Active, e.state.Previous = "", ""
	} else {
		e.state.Active, e.state.Previous = old.Previous, ""
	}
	e.state.Chosen = false
	if old.Pin == ver {
		e.state.Pin = ""
	}
	e.note("rolled-back", ver, "")
	if err := m.saveState(); err != nil {
		e.state = old
		return err
	}
	m.resolve(id, e)
	// The rollback is what the admin wanted, so it isn't reported as a problem.
	e.notice = ""
	m.computeOffers(e)
	m.log.Info("modules: rolled back", "module", id, "from", ver, "to", e.man.Version)
	return nil
}

// Pin keeps the version in use: automatic updates leave it alone until Unpin. A
// pin doesn't override safety: a version withdrawn for security reasons, or one that
// stops working, still goes out of use, and the page says so.
func (m *Manager) Pin(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil {
		return ErrUnknown
	}
	if !updatable(e.builtin) {
		return errors.New(e.builtin.Name + " updates with the server, so it has nothing to pin")
	}
	pin := e.man.Version
	if e.dir == "" {
		pin = PinBuiltin
	}
	old := e.state
	e.state.Pin = pin
	e.note("pinned", e.man.Version, "")
	if err := m.saveState(); err != nil {
		e.state = old
		return err
	}
	return nil
}

// Unpin lets automatic updates move the module again.
func (m *Manager) Unpin(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil {
		return ErrUnknown
	}
	old := e.state
	e.state.Pin = ""
	e.note("unpinned", e.man.Version, "")
	if err := m.saveState(); err != nil {
		e.state = old
		return err
	}
	return nil
}

// SetAutoUpdate says whether this module may update by itself while the server's setting
// is on. Off, it still looks for updates and offers them, and an admin installs them by
// hand. A built-in part of the server has no updates of its own to turn off.
func (m *Manager) SetAutoUpdate(id string, on bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil {
		return ErrUnknown
	}
	if !updatable(e.builtin) {
		return errors.New(e.builtin.Name + " is built into the server and updates with it")
	}
	if on && e.state.Source != nil {
		return errors.New("a module from another publisher never updates by itself")
	}
	old := e.state
	e.state.NoAuto = !on
	if err := m.saveState(); err != nil {
		e.state = old
		return err
	}
	return nil
}

// UseBuiltin goes back to the built-in copy without blaming the version in use: it
// stays available to choose again.
func (m *Manager) UseBuiltin(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil {
		return ErrUnknown
	}
	if e.busy {
		return errors.New("an update of this module is being installed")
	}
	if e.dir == "" {
		return nil
	}
	old := e.state
	from := e.man.Version
	e.state.Previous, e.state.Active, e.state.Chosen = from, "", false
	if old.Pin != "" {
		e.state.Pin = PinBuiltin
	}
	e.note("switched", e.builtin.Version, "from "+from+" to the built-in copy")
	if err := m.saveState(); err != nil {
		e.state = old
		return err
	}
	m.resolve(id, e)
	m.computeOffers(e)
	return nil
}

// ToLatest unpins a module and starts moving it to the newest version that fits:
// a release if one is newer than the built-in copy, or else the built-in copy.
func (m *Manager) ToLatest(id string) error {
	if err := m.Unpin(id); err != nil {
		return err
	}
	m.mu.Lock()
	e := m.mods[id]
	m.computeOffers(e)
	rel := e.rel
	inUse := e.dir != ""
	olderThanBuiltin := inUse && wittymodule.Compare(e.man.Version, e.builtin.Version) < 0
	m.mu.Unlock()
	if rel != nil {
		return m.InstallVersion(id, rel.version, true)
	}
	if olderThanBuiltin {
		return m.UseBuiltin(id)
	}
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
		if n := d.Name(); n != e.state.Active && n != e.state.Previous && n != e.state.Pin {
			_ = os.RemoveAll(filepath.Join(dir, "versions", n))
		}
	}
}

// Close stops every helper. A helper in the middle of a call may keep it waiting until
// its time limit, so nothing waits for long, and none of them outlives the server anyway.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	var procs []*proc
	for _, e := range m.mods {
		if e.proc != nil {
			procs = append(procs, e.proc)
		}
	}
	for a := range m.attached {
		procs = append(procs, a.p)
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, p := range procs {
		wg.Add(1)
		go func() { defer wg.Done(); p.Close() }()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); m.retiring.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
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
		if e.latest != nil && e.latest.Blocked == "" && e.rel != nil && !e.busy && e.state.Pin == "" && !e.state.NoAuto && !e.state.Removed && e.state.Source == nil {
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
