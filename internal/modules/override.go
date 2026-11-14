// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

// Overrides: an administrator may install what the server would otherwise refuse, when what
// stands in the way is a judgement rather than an impossibility, after reading exactly what is
// being bypassed and what could go wrong (docs/MODULE-STORE.md, "Overriding a refusal"). Each
// kind has a plain warning and a severity; the server asks for more the higher it is. What would
// lose data, can't run, or would let a module past a line the server holds for everyone (encrypted
// media, a machine's owner keeping other publishers off) is never overridable.
//
// An overridden version is remembered with what was bypassed, pinned, and never updated by
// itself. One from a publisher this server doesn't trust is held to a fork's rules: the strict
// sandbox, and no permission an administrator didn't tick for it.

// Kinds of override.
const (
	OverrideIncomplete   = "incomplete"
	OverrideOutOfRange   = "out-of-range"
	OverrideLocalFile    = "local-file"
	OverrideCustom       = "custom"
	OverrideUntrustedKey = "untrusted-key"
	OverrideWeakSandbox  = "weak-sandbox"
	OverrideUnsigned     = "unsigned"
)

// overrideKinds are every kind with its severity, 1 to 4: 3 and above also need the module's
// name typed.
var overrideKinds = map[string]int{
	OverrideIncomplete: 1, OverrideOutOfRange: 2, OverrideLocalFile: 2,
	OverrideCustom: 3, OverrideUntrustedKey: 3, OverrideWeakSandbox: 3, OverrideUnsigned: 4,
}

// OverrideWarnings say what each override bypasses and what could go wrong. The server says
// them, so every app that offers an override says the same thing.
var OverrideWarnings = map[string]string{
	OverrideIncomplete: "Its publisher says it isn't finished, or what it adds to the apps doesn't fit this server. " +
		"Parts of it may be missing or not work, and whatever doesn't fit isn't shown in any app.",
	OverrideOutOfRange: "It wasn't made for this version of the server. It speaks a module API this server knows, so it may work, " +
		"but nobody has tried it here: pages, libraries or playback that depend on it may break, and updating the server may break it further.",
	OverrideLocalFile: "It comes from a file you chose, not from a store this server checks for updates and withdrawals. " +
		"You'll be told about nothing that goes wrong with this version later.",
	OverrideCustom: "This server doesn't know this module. Nothing about it has been checked by this project: " +
		"it runs with the strict sandbox and no permissions you don't tick, and what it adds to the apps is its own.",
	OverrideUntrustedKey: "It is signed, but by a key this server doesn't trust for it. The signature proves only that whoever holds that key made it, " +
		"not who they are or that it is safe. Compare the key's fingerprint with its publisher, somewhere other than where you got the file.",
	OverrideWeakSandbox: "This system's sandbox for modules from other publishers is Apple's sandbox-exec, which Apple has deprecated. " +
		"The module is refused if it won't start inside it, but it can still read anything this server's account can read, " +
		"which may include your media and this server's database.",
	OverrideUnsigned: "It isn't signed at all. Nothing proves who made it or that it wasn't changed on its way to you. " +
		"It runs with the strict sandbox and no permissions you don't tick, but it is someone's code running on your server. " +
		"Only install it if you made it yourself or got it from someone you trust, by a route nobody else could change.",
}

// forkLikeKinds are the overrides that make a module someone else's code: held to a fork's rules.
var forkLikeKinds = []string{OverrideUnsigned, OverrideUntrustedKey, OverrideCustom, OverrideWeakSandbox}

// Override is what an administrator bypassed for the version in use, as it is kept.
type Override struct {
	Kinds   []string `json:"kinds"`
	Version string   `json:"version"`
	By      string   `json:"by,omitempty"`
	At      int64    `json:"at"`
	// Key is the publisher key (base64) an untrusted-key version was signed with, or this
	// server's own local key for an unsigned one: what its files are checked against.
	Key string `json:"key,omitempty"`
	// Grants are the permissions the administrator ticked for a fork-like module.
	Grants []string `json:"grants,omitempty"`
	// Source is "catalogue" or "upload".
	Source string `json:"source"`
}

// OverrideInfo is an override as the pages show it.
type OverrideInfo struct {
	Kinds       []string `json:"kinds"`
	Reasons     []string `json:"reasons"`
	Version     string   `json:"version"`
	By          string   `json:"by,omitempty"`
	At          int64    `json:"at"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	Grants      []string `json:"grants,omitempty"`
	Source      string   `json:"source"`
}

func (o *Override) info() *OverrideInfo {
	if o == nil {
		return nil
	}
	info := &OverrideInfo{Kinds: o.Kinds, Version: o.Version, By: o.By, At: o.At, Grants: o.Grants, Source: o.Source}
	for _, k := range o.Kinds {
		info.Reasons = append(info.Reasons, OverrideWarnings[k])
	}
	if slices.Contains(o.Kinds, OverrideUntrustedKey) {
		if k, err := parseSourceKey(o.Key); err == nil {
			info.Fingerprint = Fingerprint(k)
		}
	}
	return info
}

func (o *Override) has(kind string) bool { return o != nil && slices.Contains(o.Kinds, kind) }

func (o *Override) forkLike() bool {
	if o == nil {
		return false
	}
	for _, k := range forkLikeKinds {
		if o.has(k) {
			return true
		}
	}
	return false
}

// ErrNeedsOverride is what installing needs before it goes ahead: an administrator who has read
// each warning and accepted each kind.
type ErrNeedsOverride struct {
	Version     string   `json:"version"`
	Kinds       []string `json:"kinds"`
	Reasons     []string `json:"reasons"`
	Severity    int      `json:"severity"`
	Warning     string   `json:"warning"`
	Permissions []string `json:"permissions,omitempty"` // what it asks for, which may be granted
	// Pending: it has been downloaded and checked, and is held until the answer.
	Pending bool `json:"pending,omitempty"`
}

func (e *ErrNeedsOverride) Error() string {
	return "Installing it needs your go-ahead past what this server would refuse: " + strings.Join(e.Kinds, ", ") + "."
}

func needsOverride(ver string, kinds []string, man *wittymodule.Manifest) *ErrNeedsOverride {
	kinds = sortedKinds(kinds)
	n := &ErrNeedsOverride{Version: ver, Kinds: kinds}
	var words []string
	for _, k := range kinds {
		n.Reasons = append(n.Reasons, OverrideWarnings[k])
		words = append(words, OverrideWarnings[k])
		n.Severity = max(n.Severity, overrideKinds[k])
	}
	n.Warning = strings.Join(words, " ")
	if man != nil {
		n.Permissions = slices.Clone(man.Permissions)
	}
	return n
}

func sortedKinds(kinds []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, k := range kinds {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if overrideKinds[out[i]] != overrideKinds[out[j]] {
			return overrideKinds[out[i]] < overrideKinds[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

// OverrideAccept is an administrator's go-ahead, after the server checked their password (and
// the typed name, where the severity asks for it).
type OverrideAccept struct {
	Accept []string
	Grants []string
	Typed  string
	By     string
}

// covers checks a go-ahead against what is needed: exactly the kinds needed, the name typed
// when the severity asks for it, and only grants the module asks for, for a fork-like one.
// ErrBadOverride is a go-ahead that doesn't fit what installing needs: kinds that aren't exactly
// those needed, the name not typed, a permission that can't be granted. Needs says what would.
type ErrBadOverride struct {
	Reason string
	Needs  *ErrNeedsOverride
}

func (e *ErrBadOverride) Error() string { return e.Reason }

func (a *OverrideAccept) covers(n *ErrNeedsOverride, id string, man *wittymodule.Manifest) error {
	if a == nil {
		return n
	}
	bad := func(why string) error { return &ErrBadOverride{Reason: why, Needs: n} }
	got, want := sortedKinds(a.Accept), n.Kinds
	if !slices.Equal(got, want) {
		return bad(fmt.Sprintf("the go-ahead names %s, and installing it needs exactly %s", strings.Join(got, ", "), strings.Join(want, ", ")))
	}
	if n.Severity >= 3 && !strings.EqualFold(strings.TrimSpace(a.Typed), id) {
		return bad(fmt.Sprintf("type the module's name, %s, to go ahead", id))
	}
	ov := &Override{Kinds: want}
	for _, g := range a.Grants {
		if !ov.forkLike() {
			return bad("permissions are granted only to a module from another publisher; this one keeps the official rules")
		}
		if man == nil || !man.Asks(g) {
			return bad(fmt.Sprintf("it doesn't ask for %q", g))
		}
		if os.Getenv(LockForks) == "1" {
			return bad("whoever runs this machine keeps modules from other publishers without permissions (" + LockForks + ")")
		}
	}
	return nil
}

// rangeOnly reports that a version is refused only for its server range, while its module API is
// one this server speaks: that it could run in compatibility mode.
func rangeOnly(man *wittymodule.Manifest, server string) bool {
	if supports(man, server) == nil || man.API < MinAPI || man.API > API {
		return false
	}
	c := *man
	c.MinServer, c.MaxServer = "0.0.0", "999.x"
	return c.Supports(server, MinAPI, API) == nil
}

// Status of a version, as its manifest says (module API 4; absent in older manifests).
const (
	StatusStable       = "stable"
	StatusBeta         = "beta"
	StatusExperimental = "experimental"
	StatusIncomplete   = "incomplete"
)

// manifestStatus reads a manifest's status and note: fields the wittymodule a server is built with
// may not have yet.
func manifestStatus(raw []byte) (status, note string) {
	var v struct {
		Status     string `json:"status"`
		StatusNote string `json:"statusNote"`
	}
	_ = json.Unmarshal(raw, &v)
	if v.Status == "" {
		v.Status = StatusStable
	}
	if len([]rune(v.StatusNote)) > 500 {
		v.StatusNote = string([]rune(v.StatusNote)[:500])
	}
	return v.Status, strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' {
			return -1
		}
		return r
	}, v.StatusNote)
}

// stagedNeeds are what a downloaded, unpacked version needs beyond its source: incomplete, when
// it says it is, or its SDUI is missing or doesn't fit.
func (m *Manager) stagedNeeds(stage string, man *wittymodule.Manifest, raw []byte) []string {
	var needs []string
	if st, _ := manifestStatus(raw); st == StatusExperimental || st == StatusIncomplete {
		needs = append(needs, OverrideIncomplete)
	}
	m.mu.Lock()
	checks := m.cfg.UIChecks
	m.mu.Unlock()
	if _, err := readUI(stage, man, checks); err != nil {
		needs = append(needs, OverrideIncomplete)
	}
	return needs
}

// overrideFor is the override for a version, nil when it has none.
func (e *entry) overrideFor(ver string) *Override {
	if o := e.state.Override; o != nil && o.Version == ver {
		return o
	}
	return nil
}

// ---------------------------------------------------------------------------
// Downloads and uploads held for a go-ahead.
// ---------------------------------------------------------------------------

// held is a version downloaded or uploaded, unpacked and checked, waiting for an administrator.
type held struct {
	token   string
	id      string
	version string
	dir     string // the unpacked version, its signed list beside it once one exists
	man     *wittymodule.Manifest
	raw     []byte
	needs   *ErrNeedsOverride
	refusal string
	source  string // "catalogue" or "upload"
	key     string // an untrusted signer, base64
	signed  bool   // a signed list came with it
	by      int64  // the administrator who uploaded it; 0 for a download
	expires time.Time
	custom  bool
	older   bool
}

// heldFor is how long a held version waits.
const heldFor = 30 * time.Minute

// maxHeld bounds what is held at once.
const maxHeld = 8

func (m *Manager) heldDir() string { return filepath.Join(m.dir, ".held") }

// keepHeld remembers a held version, dropping the oldest past maxHeld. Call with m.mu held.
func (m *Manager) keepHeld(h *held) {
	if m.held == nil {
		m.held = map[string]*held{}
	}
	m.dropExpired()
	for len(m.held) >= maxHeld {
		var oldest *held
		for _, x := range m.held {
			if oldest == nil || x.expires.Before(oldest.expires) {
				oldest = x
			}
		}
		m.dropHeld(oldest.token)
	}
	m.held[h.token] = h
}

func (m *Manager) dropExpired() {
	now := time.Now()
	for t, h := range m.held {
		if now.After(h.expires) {
			m.dropHeld(t)
		}
	}
}

// dropHeld forgets a held version and its files. Call with m.mu held.
func (m *Manager) dropHeld(token string) {
	h := m.held[token]
	if h == nil {
		return
	}
	delete(m.held, token)
	_ = os.RemoveAll(filepath.Join(m.heldDir(), token))
	if e := m.mods[h.id]; e != nil && e.pending == token {
		e.pending = ""
	}
}

func newToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// holdStage moves a checked stage aside for a go-ahead. Call without m.mu.
func (m *Manager) holdStage(stage string, h *held) error {
	if err := os.MkdirAll(m.heldDir(), 0o700); err != nil {
		return err
	}
	dst := filepath.Join(m.heldDir(), h.token)
	if err := renameRetry(stage, dst); err != nil {
		return err
	}
	h.dir = dst
	h.expires = time.Now().Add(heldFor)
	m.mu.Lock()
	m.keepHeld(h)
	if e := m.mods[h.id]; e != nil && h.source == "catalogue" {
		e.pending = h.token
	}
	m.mu.Unlock()
	return nil
}

// PendingInfo is a download waiting for a go-ahead, as the page shows it.
type PendingInfo struct {
	Version       string            `json:"version"`
	NeedsOverride *ErrNeedsOverride `json:"needsOverride"`
	Expires       int64             `json:"expires"`
}

// DropPending forgets a module's held download.
func (m *Manager) DropPending(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil {
		return ErrUnknown
	}
	if e.pending == "" {
		return errors.New("nothing is waiting for a go-ahead")
	}
	m.dropHeld(e.pending)
	e.phase = PhaseIdle
	return nil
}

// ---------------------------------------------------------------------------
// This server's own key, for unsigned versions an administrator accepted.
// ---------------------------------------------------------------------------

// localKey is this server's own signing key, made the first time it is needed and never leaving
// the modules folder. It signs only the list of an unsigned version an administrator accepted, so
// that a file changed afterwards is refused like any other; it is trusted for nothing else.
func (m *Manager) localKey() (ed25519.PrivateKey, error) {
	p := filepath.Join(m.dir, ".local-key")
	if b, err := os.ReadFile(p); err == nil {
		if k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b))); err == nil && len(k) == ed25519.PrivateKeySize {
			return ed25519.PrivateKey(k), nil
		}
		return nil, errors.New("this server's local module key is unreadable")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return nil, err
	}
	if err := writeFileAtomic(p, []byte(base64.StdEncoding.EncodeToString(priv)), 0o600); err != nil {
		return nil, err
	}
	return priv, nil
}

func writeFileAtomic(p string, b []byte, mode os.FileMode) error {
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// signLocally writes a signed list of every file in dir, under the top folder a release would
// use, signed with this server's own key. It answers the key, base64.
func (m *Manager) signLocally(dir string, man *wittymodule.Manifest) (string, error) {
	key, err := m.localKey()
	if err != nil {
		return "", err
	}
	top := man.Top(runtime.GOOS, runtime.GOARCH)
	var lines []string
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == signedList || rel == signedSig {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is a link or special file, which a module can't hold", rel)
		}
		h, err := update.FileSHA256(p)
		if err != nil {
			return err
		}
		lines = append(lines, h+"  "+top+"/"+rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(lines)
	sums := []byte(strings.Join(lines, "\n") + "\n")
	if err := os.WriteFile(filepath.Join(dir, signedList), sums, 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, signedSig), update.Sign(key, sums), 0o644); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)), nil
}

// overrideRefusal says why someone else's code can't run on this system at all, overridden or not:
// whoever runs the machine keeps other publishers off, or there is nothing to box it in. "" when it
// can, with the weak-sandbox override where the box is Apple's (weakSandboxForOthers).
func overrideRefusal() string {
	switch {
	case os.Getenv(LockForks) == "1":
		return "Whoever runs this machine has kept modules from other publishers off with " + LockForks + "."
	case sandboxAvailable, weakSandboxForOthers():
		return ""
	}
	return "This system has no way to box in a module from another publisher, so it can't be installed here, whatever is overridden."
}

// othersNeedWeakSandbox: on this system a module from another publisher also needs the
// weak-sandbox override.
func othersNeedWeakSandbox() bool { return !sandboxAvailable && weakSandboxForOthers() }

// UIAdds is what a module in use adds to every app, from its sdui.json, for its page in the store.
type UIAdds struct {
	Pages      []UIPage `json:"pages"`
	Navigation []string `json:"navigation"`
	Settings   bool     `json:"settings"`
}

// UIPage is one page a module adds.
type UIPage struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// overrideStatus fills in a module's override, held download, status and what it adds to the
// apps and to this server. Call with
// m.mu held.
func (m *Manager) overrideStatus(e *entry, st *Status) {
	st.Override = e.state.Override.info()
	if h := m.held[e.pending]; h != nil {
		st.Pending = &PendingInfo{Version: h.version, NeedsOverride: h.needs, Expires: h.expires.Unix()}
	}
	if e.dir != "" {
		st.ReleaseStatus, st.StatusNote = e.relStatus, e.relNote
	}
	if ui := e.ui; ui != nil && e.dir != "" && !e.state.Disabled {
		adds := &UIAdds{Pages: []UIPage{}, Navigation: []string{}, Settings: len(ui.Settings) > 0}
		for _, p := range ui.Pages {
			adds.Pages = append(adds.Pages, UIPage{ID: p.ID, Title: p.Title})
		}
		for _, n := range ui.Navigation {
			label := n.Label
			if label == "" {
				for _, p := range ui.Pages {
					if p.ID == n.Page {
						label = p.Title
					}
				}
			}
			if label != "" {
				adds.Navigation = append(adds.Navigation, label)
			}
		}
		st.UIAdds = adds
	}
	if e.integ != nil && e.dir != "" && !e.state.Disabled {
		st.IntegrationAdds = integrationAdds(e.integ)
	}
}
