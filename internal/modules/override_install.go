// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/appcontrol"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

// Installing what was held for a go-ahead, installing from a file, and taking an override back.

// MaxUpload bounds a module archive uploaded by hand.
const MaxUpload = 512 << 20

// installHeldLocked installs a held version with a go-ahead. Call with m.mu held; the work goes on
// in the background like any install, and the report shows it.
func (m *Manager) installHeldLocked(h *held, accept *OverrideAccept) error {
	if h.refusal != "" {
		return errors.New(h.refusal)
	}
	if err := accept.covers(h.needs, h.id, h.man); err != nil {
		return err
	}
	if m.closed {
		return errors.New("the server is stopping")
	}
	e := m.mods[h.id]
	if e == nil {
		if !h.custom {
			return ErrUnknown
		}
		e = customEntry(h.man)
		m.mods[h.id] = e
		m.order = append(m.order, h.id)
		sort.Strings(m.order)
	}
	if e.busy {
		return errors.New("an update of this module is already being installed")
	}
	// It is this install's now: nothing else may drop it.
	delete(m.held, h.token)
	if e.pending == h.token {
		e.pending = ""
	}
	ov := &Override{Kinds: h.needs.Kinds, Version: h.version, By: accept.By, At: time.Now().Unix(), Grants: sortedKinds(accept.Grants), Source: h.source, Key: h.key}
	e.busy, e.phase, e.lastError = true, PhaseInstalling, ""
	task := taskRegistry.Load().Start(tasks.Spec{Type: tasks.TypeModule, Title: "Installing " + h.man.Name + " " + h.version, Phase: "Installing", AdminOnly: true})
	e.task = task
	session := logging.NewID("mod")
	log := m.log.With("session", session, "module", h.id, "version", h.version)
	log.Warn("modules: installing with an override", "kinds", strings.Join(ov.Kinds, ","), "by", accept.By)
	logging.Go("module install", func() {
		err := m.finishHeld(e, h, ov)
		m.mu.Lock()
		defer m.mu.Unlock()
		e.busy, e.task = false, nil
		_ = os.RemoveAll(filepath.Join(m.heldDir(), h.token))
		if err != nil {
			task.Fail(sentence(err.Error()), "Nothing changed: the version in use is still in use.", err)
			e.phase, e.lastError = PhaseFailed, sentence(err.Error())
			e.note("failed", h.version, "the install failed: "+err.Error())
			if h.custom && e.dir == "" {
				// A module this server didn't know, which never got in: not listed at all.
				delete(m.mods, h.id)
				m.order = slices.DeleteFunc(m.order, func(x string) bool { return x == h.id })
				_ = os.RemoveAll(m.modDir(h.id))
			}
			_ = m.saveState()
			log.Error("modules: install with an override failed; nothing changed", "err", err)
			return
		}
		task.Result(h.man.Name + " " + h.version + " is installed, overriding " + strings.Join(ov.Kinds, ", ") + ".")
		task.Finish(nil)
		e.phase = PhaseIdle
		log.Info("modules: installed with an override")
		m.ready(h.id)
	}, "cat", "modules", "session", session)
	return nil
}

// finishHeld puts a held version in place: signed by this server when it came unsigned, started
// once with the limits it will have, then made the version in use with its override.
func (m *Manager) finishHeld(e *entry, h *held, ov *Override) error {
	dir := h.dir
	if ov.has(OverrideUnsigned) {
		key, err := m.signLocally(dir, h.man)
		if err != nil {
			return err
		}
		ov.Key = key
	}
	m.mu.Lock()
	prev := e.state.Override
	e.state.Override = ov // the limits it will run with, for the try below
	keys := m.keysFor(e)
	m.mu.Unlock()
	restore := func() {
		m.mu.Lock()
		e.state.Override = prev
		m.mu.Unlock()
	}
	if err := verifyInstalled(dir, h.man, keys); err != nil {
		restore()
		return err
	}
	if h.man.PerPlatform() {
		if err := appcontrol.CheckInstall(dir, "this module", SignedFix); err != nil {
			restore()
			return err
		}
		if h.man.Delivery != wittymodule.DeliveryProgram {
			m.mu.Lock()
			p := m.newProcFor(h.id, e, h.man, dir)
			m.mu.Unlock()
			p.verify = nil
			if err := p.Try(); err != nil {
				restore()
				return fmt.Errorf("it didn't start on this server with the limits it would have: %w", err)
			}
		}
	}
	dst, err := m.versionDir(h.id, h.version)
	if err != nil {
		restore()
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		restore()
		return err
	}
	_ = os.RemoveAll(dst)
	if err := renameRetry(dir, dst); err != nil {
		restore()
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if h.custom {
		e.state.Removed = false
	}
	if err := m.activate(h.id, e, h.man, h.older); err != nil {
		e.state.Override = prev
		_ = os.RemoveAll(dst)
		return err
	}
	return nil
}

// customEntry is a module this server doesn't know, from what its own manifest says: nothing of
// it is built in, and nothing of it was checked by this project.
func customEntry(man *wittymodule.Manifest) *entry {
	b := *man
	b.Version, b.MinServer, b.MaxServer, b.Core = absentVersion, absentVersion, "0.x", false
	b.Libraries, b.Requires = nil, nil
	return &entry{builtin: &b, man: &b, absent: true, phase: PhaseIdle, known: map[string]*knownRelease{}}
}

// loadCustom brings back the modules an administrator installed that this server doesn't know,
// from what is on disk. Their files are checked like any others' before they're used.
func (m *Manager) loadCustom(saved map[string]moduleState) {
	for id, st := range saved {
		if m.mods[id] != nil || !st.Override.has(OverrideCustom) || st.Active == "" || !wittymodule.Valid(st.Active) {
			continue
		}
		dir, err := m.versionDir(id, st.Active)
		if err != nil {
			continue
		}
		man, err := readManifestFile(dir)
		if err != nil || man.ID != id {
			m.log.Warn("modules: a module installed by hand is unreadable", "module", id, "err", err)
			continue
		}
		e := customEntry(man)
		e.state = st
		m.mods[id] = e
		m.order = append(m.order, id)
	}
	sort.Strings(m.order)
}

// ---------------------------------------------------------------------------
// From a file.
// ---------------------------------------------------------------------------

// Upload is a module release someone chose from their computer.
type Upload struct {
	Archive     string // the archive, saved by the caller, with its name's extension (.tar.gz or .zip)
	Sums, Sig   []byte // the checksum list and its signature, when they came with it
	Key         string // a publisher key, base64, to check the signature against
	By          int64  // the administrator who uploaded it
	ArchiveName string
}

// UploadProblem is one thing about an upload that needs an override.
type UploadProblem struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// UploadInfo says what an uploaded file is and what installing it would take.
type UploadInfo struct {
	Token         string            `json:"token,omitempty"`
	Expires       int64             `json:"expiresAt,omitempty"`
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Delivery      string            `json:"delivery"`
	API           int               `json:"api"`
	ReleaseStatus string            `json:"releaseStatus"`
	StatusNote    string            `json:"statusNote,omitempty"`
	Signed        bool              `json:"signed"`
	Trusted       bool              `json:"trusted"`
	Fingerprint   string            `json:"fingerprint,omitempty"`
	Permissions   []string          `json:"permissions"`
	Problems      []UploadProblem   `json:"problems"`
	NeedsOverride *ErrNeedsOverride `json:"needsOverride,omitempty"`
	Refusal       string            `json:"refusal,omitempty"`
	Known         bool              `json:"known"` // a module this server knows
}

// StageUpload unpacks and checks an uploaded release, and holds it for an administrator's
// go-ahead. A file that can't be installed at all answers its refusal, and nothing is kept.
func (m *Manager) StageUpload(u Upload) (*UploadInfo, error) {
	if err := os.MkdirAll(m.heldDir(), 0o700); err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp(m.heldDir(), ".up-")
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(work)
		}
	}()
	name := strings.ToLower(u.ArchiveName)
	if !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".tgz") && !strings.HasSuffix(name, ".zip") {
		return &UploadInfo{Refusal: "Choose a module's release archive: a .tar.gz or .zip.", Problems: []UploadProblem{}, Permissions: []string{}}, nil
	}
	stage := filepath.Join(work, "new")
	if err := update.Extract(u.Archive, stage, maxUnpacked); err != nil {
		return &UploadInfo{Refusal: "It can't be unpacked: " + sentence(err.Error()), Problems: []UploadProblem{}, Permissions: []string{}}, nil
	}
	raw, err := os.ReadFile(filepath.Join(stage, wittymodule.ManifestFile))
	if err != nil {
		return &UploadInfo{Refusal: "It has no module.json, so it isn't a module's release.", Problems: []UploadProblem{}, Permissions: []string{}}, nil
	}
	man, err := wittymodule.ParseManifest(raw)
	if err != nil {
		return &UploadInfo{Refusal: "Its module.json can't be used: " + sentence(err.Error()), Problems: []UploadProblem{}, Permissions: []string{}}, nil
	}
	status, note := manifestStatus(raw)
	info := &UploadInfo{ID: man.ID, Name: man.Name, Version: man.Version, Delivery: man.Delivery, API: man.API, ReleaseStatus: status, StatusNote: note,
		Permissions: slices.Clone(man.Permissions), Problems: []UploadProblem{}}
	if info.Permissions == nil {
		info.Permissions = []string{}
	}
	refuse := func(why string) (*UploadInfo, error) {
		info.Refusal = sentence(why)
		return info, nil
	}

	m.mu.Lock()
	e := m.mods[man.ID]
	known := e != nil
	var trusted []ed25519.PublicKey
	var builtinDelivery string
	dataWhy := ""
	if known {
		trusted = m.keysFor(e)
		builtinDelivery = e.builtin.Delivery
		if r := dataRefusal(e.builtin.Name, man, e.state.DataHigh); r != "" {
			dataWhy = r
		}
		if e.busy {
			m.mu.Unlock()
			return refuse("An update of this module is being installed.")
		}
	}
	m.mu.Unlock()
	info.Known = known

	// What no override passes.
	switch {
	case man.API < MinAPI || man.API > API:
		return refuse(fmt.Sprintf("It is built for module API %d, and this server speaks %d to %d: there is no way to run it here.", man.API, MinAPI, API))
	case man.Delivery == wittymodule.DeliveryLibrary || man.Delivery == wittymodule.DeliveryBuiltin:
		return refuse("It has nothing to install on its own: it reaches a server inside other modules.")
	case known && !e.absent && man.Delivery != builtinDelivery:
		return refuse(fmt.Sprintf("It is delivered as %q, and this server expects %q for %s.", man.Delivery, builtinDelivery, man.Name))
	case dataWhy != "":
		return refuse(dataWhy)
	}
	if man.PerPlatform() {
		if _, err := os.Stat(filepath.Join(stage, man.EntryFile(runtime.GOOS))); err != nil {
			return refuse(fmt.Sprintf("This file is for another kind of computer: it has no %s for %s.", man.EntryFile(runtime.GOOS), runtime.GOOS+"/"+runtime.GOARCH))
		}
	}

	// Who made it.
	var kinds []string
	signer := ""
	switch {
	case len(u.Sums) > 0 && len(u.Sig) > 0:
		if err := os.WriteFile(filepath.Join(stage, signedList), u.Sums, 0o644); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(stage, signedSig), u.Sig, 0o644); err != nil {
			return nil, err
		}
		if len(trusted) > 0 && update.VerifySums(u.Sums, u.Sig, trusted) == nil {
			info.Signed, info.Trusted = true, true
			if err := verifyInstalled(stage, man, trusted); err != nil {
				return refuse("Its files don't match its signed checksum list: " + err.Error())
			}
			break
		}
		if strings.TrimSpace(u.Key) == "" {
			return refuse("Its signature doesn't match this project's key for it. If someone else published it, give their public key, and it can be installed as theirs.")
		}
		k, err := parseSourceKey(u.Key)
		if err != nil {
			return refuse(err.Error())
		}
		if update.VerifySums(u.Sums, u.Sig, []ed25519.PublicKey{k}) != nil {
			return refuse("Its signature doesn't match the key you gave: it may have been changed, or the key is someone else's.")
		}
		if err := verifyInstalled(stage, man, []ed25519.PublicKey{k}); err != nil {
			return refuse("Its files don't match its signed checksum list: " + err.Error())
		}
		info.Signed, info.Fingerprint = true, Fingerprint(k)
		signer = strings.TrimSpace(u.Key)
		kinds = append(kinds, OverrideUntrustedKey)
	default:
		kinds = append(kinds, OverrideUnsigned)
		if len(u.Sums) > 0 {
			info.Problems = append(info.Problems, UploadProblem{Kind: OverrideUnsigned, Text: "A checksum list came without its signature, which proves nothing about who made it, so it was set aside."})
		}
	}
	if !known {
		kinds = append(kinds, OverrideCustom)
	}
	kinds = append(kinds, OverrideLocalFile)
	ovProbe := &Override{Kinds: kinds}
	if ovProbe.forkLike() {
		if why, ok := noFork[man.ID]; ok {
			return refuse(why)
		}
		if r := overrideRefusal(); r != "" {
			return refuse(r)
		}
		if othersNeedWeakSandbox() && man.PerPlatform() {
			// A program of its own runs on this server; a web bundle runs in browsers.
			kinds = append(kinds, OverrideWeakSandbox)
		}
	}
	if err := supports(man, m.cfg.Server); err != nil {
		if !rangeOnly(man, m.cfg.Server) {
			return refuse(err.Error())
		}
		kinds = append(kinds, OverrideOutOfRange)
		info.Problems = append(info.Problems, UploadProblem{Kind: OverrideOutOfRange, Text: sentence(err.Error())})
	} else if madeFor, ok := olderServer(man, m.cfg.Server); ok {
		// Made for an older server: from a file too, never without the go-ahead for that, which
		// the out-of-range override is, with these words.
		kinds = append(kinds, OverrideOutOfRange)
		info.Problems = append(info.Problems, UploadProblem{Kind: OverrideOutOfRange, Text: OlderServerWarning(man.Name, man.Version, madeFor, m.cfg.Server)})
	}
	if st := status; st == StatusExperimental || st == StatusIncomplete {
		kinds = append(kinds, OverrideIncomplete)
		text := "Its publisher marks it " + st + "."
		if note = strings.TrimSpace(note); note != "" {
			text += " " + note
		}
		info.Problems = append(info.Problems, UploadProblem{Kind: OverrideIncomplete, Text: text})
	}
	m.mu.Lock()
	checks := m.cfg.UIChecks
	m.mu.Unlock()
	if _, err := readUI(stage, man, checks); err != nil {
		kinds = append(kinds, OverrideIncomplete)
		info.Problems = append(info.Problems, UploadProblem{Kind: OverrideIncomplete, Text: sentence(err.Error()) + " It would run with nothing of its own in any app."})
	}
	for _, k := range kinds {
		if !slices.ContainsFunc(info.Problems, func(p UploadProblem) bool { return p.Kind == k }) {
			info.Problems = append(info.Problems, UploadProblem{Kind: k, Text: OverrideWarnings[k]})
		}
	}
	n := needsOverride(man.Version, kinds, man)
	info.NeedsOverride = n
	h := &held{token: newToken(), id: man.ID, version: man.Version, man: man, raw: raw, needs: n, source: "upload", key: signer, signed: info.Signed, by: u.By, custom: !known}
	m.mu.Lock()
	if known {
		h.older = wittymodule.Compare(man.Version, e.man.Version) < 0
	}
	m.mu.Unlock()
	if err := m.holdStage(stage, h); err != nil {
		return nil, err
	}
	info.Token, info.Expires = h.token, h.expires.Unix()
	return info, nil
}

// ErrUnknownUpload: no upload has that token, or it isn't this administrator's, or it ran out.
var ErrUnknownUpload = errors.New("that upload isn't waiting any more: choose the file again")

// InstallUpload installs an uploaded release with an administrator's go-ahead.
func (m *Manager) InstallUpload(token string, by int64, accept *OverrideAccept) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropExpired()
	h := m.held[token]
	if h == nil || h.source != "upload" || h.by != by {
		return ErrUnknownUpload
	}
	return m.installHeldLocked(h, accept)
}

// UploadOf names the module and version an administrator's held upload would install, for the
// audit log; empty once it has gone.
func (m *Manager) UploadOf(token string, by int64) (id, version string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h := m.held[token]; h != nil && h.source == "upload" && h.by == by {
		return h.id, h.version
	}
	return "", ""
}

// DiscardUpload forgets an upload.
func (m *Manager) DiscardUpload(token string, by int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.held[token]
	if h == nil || h.source != "upload" || h.by != by {
		return ErrUnknownUpload
	}
	m.dropHeld(token)
	return nil
}

// ---------------------------------------------------------------------------
// Taking an override back.
// ---------------------------------------------------------------------------

// RemoveOverride ends an override: the module goes back to the version before, when that one
// needed none, or off this server. A module this server didn't know goes altogether.
func (m *Manager) RemoveOverride(id string, libraries map[string][]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil {
		return ErrUnknown
	}
	ov := e.state.Override
	if ov == nil {
		return errors.New(e.builtin.Name + " isn't overridden")
	}
	if e.busy {
		return errors.New("an update of this module is being installed")
	}
	if ov.has(OverrideCustom) {
		if by := m.requiredBy(id); len(by) > 0 {
			return fmt.Errorf("%s can't be removed while %s is built on it", e.builtin.Name, joinNames(by))
		}
		old := e.state
		e.state = moduleState{Removed: true}
		m.resolve(id, e)
		if err := m.saveState(); err != nil {
			e.state = old
			m.resolve(id, e)
			return err
		}
		delete(m.mods, id)
		m.order = slices.DeleteFunc(m.order, func(x string) bool { return x == id })
		_ = os.RemoveAll(m.modDir(id))
		m.uiGen++
		m.log.Info("module override taken back: the module is gone", "cat", "modules-store", "module", id, "version", ov.Version, "kinds", ov.Kinds)
		return m.saveState()
	}
	if prev := e.state.Previous; prev != "" && prev != ov.Version {
		old := e.state
		e.state.Override = nil
		if _, _, err := m.usable(id, e, prev); err == nil {
			e.state.Active, e.state.Previous, e.state.Pin, e.state.Chosen = prev, "", "", true
			e.note("override-removed", prev, "the override of "+ov.Version+" was taken back")
			if err := m.saveState(); err != nil {
				e.state = old
				return err
			}
			m.resolve(id, e)
			m.prune(id, e)
			m.log.Info("module override taken back: the version before is in use", "cat", "modules-store", "module", id, "version", ov.Version, "to", prev, "kinds", ov.Kinds)
			return nil
		}
		e.state = old
	}
	e.state.Override = nil
	if err := m.removeModule(id, libraries); err != nil {
		return err
	}
	m.log.Info("module override taken back: the module is off this server", "cat", "modules-store", "module", id, "version", ov.Version, "kinds", ov.Kinds)
	return nil
}
