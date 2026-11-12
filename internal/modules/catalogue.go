// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

// A server learns which modules exist from a signed catalogue, with nothing compiled in.
// It offers versions and says where their files are. An install still checks the release's
// own signed list and manifest, and refuses one that differs from what the catalogue said,
// so a catalogue's key alone can't make anything run. docs/MODULES.md, "The catalogue".

// knownJSON is what this server knows of its modules before it has seen any catalogue: their
// names and what they're for, with no versions. A copy of WittyModuleCatalog's file, kept so
// a page can name the module it's missing on a server that has never been online.
//
//go:embed known.json
var knownJSON []byte

// A catalogue found here is kept here.
const (
	catalogFile = wittymodule.CatalogName
	catalogSig  = wittymodule.CatalogName + ".sig"
	serialFile  = "catalog-serial.json"
)

// absentVersion is what a module with no version on this server sorts as.
const absentVersion = "0.0.0"

// CatalogInfo is what the store shows about the catalogue itself.
type CatalogInfo struct {
	Name      string `json:"name,omitempty"`
	Serial    int64  `json:"serial,omitempty"`
	Generated string `json:"generated,omitempty"`
	Error     string `json:"error,omitempty"` // why the last look failed, in words
}

// absentManifest is what a module is known as before any version of it is here. It is never
// run: e.dir is what says there's something to run.
func absentManifest(mod *wittymodule.CatalogModule) *wittymodule.Manifest {
	man := &wittymodule.Manifest{ID: mod.ID, Name: mod.Name, Version: absentVersion, Repo: mod.Repo, Delivery: wittymodule.DeliveryHelper,
		API: wittymodule.API, MinServer: absentVersion, MaxServer: "0.x", Group: mod.Group, Summary: mod.Summary, Needs: mod.Needs,
		Updates: mod.Updates, Stays: mod.Stays, Libraries: mod.Libraries, Requires: mod.Requires, Core: mod.Core}
	if len(mod.Versions) > 0 {
		man.Delivery, man.License = mod.Versions[0].Delivery, mod.Versions[0].License
	}
	return man
}

// addAbsent makes an entry for a module this server has no built-in idea of. Call with m.mu
// held, or before m is shared.
func (m *Manager) addAbsent(mod *wittymodule.CatalogModule, saved map[string]moduleState) *entry {
	man := absentManifest(mod)
	e := &entry{builtin: man, man: man, absent: true, phase: PhaseIdle, state: saved[mod.ID], known: map[string]*knownRelease{}, listed: mod}
	m.mods[mod.ID] = e
	m.order = append(m.order, mod.ID)
	sort.Strings(m.order)
	return e
}

// loadKnown brings in what this server knows of its modules without going online: the list
// it was built with, then the catalogue it last saw, or a starter set's, and then anything
// installed that neither of those lists. Before m is shared.
func (m *Manager) loadKnown(saved map[string]moduleState) {
	if !m.cfg.Known {
		// Only what's installed and what a catalogue says.
	} else if known, err := wittymodule.ParseCatalog(knownJSON); err != nil {
		m.log.Error("modules: the list of known modules is unreadable", "err", err)
	} else {
		for i := range known.Modules {
			mod := &known.Modules[i]
			mod.Versions = nil // only the catalogue says what can be installed
			if m.mods[mod.ID] == nil {
				m.addAbsent(mod, saved)
			}
		}
	}
	m.loadCatalog(saved)
	ids, err := os.ReadDir(m.dir)
	if err != nil {
		return
	}
	for _, d := range ids {
		id := d.Name()
		if !d.IsDir() || m.mods[id] != nil {
			continue
		}
		if s, ok := saved[id]; ok && s.Active != "" {
			dir, err := m.versionDir(id, s.Active)
			if err != nil {
				continue
			}
			man, err := readManifestFile(dir)
			if err != nil || man.ID != id {
				continue
			}
			m.addAbsent(&wittymodule.CatalogModule{ID: id, Name: man.Name, Summary: man.Summary, Repo: man.Repo, Group: man.Group, Libraries: man.Libraries,
				Requires: man.Requires, Needs: man.Needs, Updates: man.Updates, Stays: man.Stays, Publisher: wittymodule.Publisher{Name: man.Repo}}, saved)
		}
	}
}

func readManifestFile(dir string) (*wittymodule.Manifest, error) {
	b, err := os.ReadFile(filepath.Join(dir, wittymodule.ManifestFile))
	if err != nil {
		return nil, err
	}
	return wittymodule.ParseManifest(b)
}

// loadCatalog reads the kept catalogue, checking it again, since the folder may be a shared
// one, or the starter set's if there's none yet or the starter's is newer. Before m is shared.
func (m *Manager) loadCatalog(saved map[string]moduleState) {
	m.catHigh = m.readSerial()
	var best *wittymodule.Catalog
	from := ""
	read := func(dir string) (*wittymodule.Catalog, error) {
		raw, err := os.ReadFile(filepath.Join(dir, catalogFile))
		if err != nil {
			return nil, err
		}
		sig, err := os.ReadFile(filepath.Join(dir, catalogSig))
		if err != nil {
			return nil, err
		}
		return verifyCatalog(raw, sig, m.keys)
	}
	if c, err := read(m.dir); err == nil {
		best, from = c, m.cfg.CatalogURL
	} else if !errors.Is(err, os.ErrNotExist) {
		m.log.Warn("modules: the kept catalogue is no good; the next look replaces it", "err", err)
	}
	if m.cfg.Starter != "" {
		c, err := read(m.cfg.Starter)
		switch {
		case errors.Is(err, os.ErrNotExist):
		case err != nil:
			m.log.Warn("modules: the starter set's catalogue is no good", "err", err)
		case best == nil || c.Serial > best.Serial:
			best, from = c, fileURL(filepath.Join(m.cfg.Starter, catalogFile))
		}
	}
	if best == nil {
		return
	}
	if best.Serial < m.catHigh {
		m.log.Warn("modules: ignoring a catalogue older than one already seen", "serial", best.Serial, "seen", m.catHigh)
		return
	}
	m.applyCatalog(best, from, saved)
}

// fileURL is a local file's address, for a starter set's relative paths to resolve against.
func fileURL(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	abs = filepath.ToSlash(abs)
	if !strings.HasPrefix(abs, "/") {
		abs = "/" + abs // a Windows drive
	}
	return (&url.URL{Scheme: "file", Path: abs}).String()
}

func (m *Manager) readSerial() int64 {
	var s struct {
		High int64 `json:"high"`
	}
	if b, err := os.ReadFile(filepath.Join(m.dir, serialFile)); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s.High
}

// applyCatalog makes a verified catalogue what's on offer. Call with m.mu held, or before m is
// shared. saved is the state to give new entries.
func (m *Manager) applyCatalog(c *wittymodule.Catalog, from string, saved map[string]moduleState) {
	m.cat, m.catFrom = c, from
	if c.Serial > m.catHigh {
		m.catHigh = c.Serial
	}
	for i := range c.Modules {
		mod := &c.Modules[i]
		e := m.mods[mod.ID]
		switch {
		case e == nil:
			e = m.addAbsent(mod, saved)
		case !e.absent:
			continue // a part of this server, whatever a catalogue says
		default:
			e.listed = mod
			was := e.builtin
			e.builtin = absentManifest(mod)
			if e.man == was {
				e.man = e.builtin
			}
		}
		if e.state.Source != nil {
			continue // a fork's versions come from the fork
		}
		e.known = map[string]*knownRelease{}
		e.feedLoaded = true
		for j := range mod.Versions {
			if k := listedRelease(mod, &mod.Versions[j], from); k != nil {
				e.known[mod.Versions[j].Version] = k
			}
		}
		m.applyListedWithdrawals(mod.ID, e, mod, c.Serial)
		m.computeOffers(e)
	}
}

// listedRelease is a version as a catalogue describes it, ready to judge and install, or nil
// if it has nothing for this system or can't be made sense of.
func listedRelease(mod *wittymodule.CatalogModule, v *wittymodule.CatalogVersion, from string) *knownRelease {
	f := v.Platform(runtime.GOOS, runtime.GOARCH)
	if f == nil {
		return nil
	}
	asset := func(ref string, size int64) (update.Asset, bool) {
		u, err := wittymodule.ResolveCatalogURL(from, ref)
		if err != nil {
			return update.Asset{}, false
		}
		p, err := url.Parse(u)
		if err != nil {
			return update.Asset{}, false
		}
		return update.Asset{Name: path.Base(p.Path), URL: u, Size: size}, true
	}
	archive, ok1 := asset(f.URL, f.Size)
	sums, ok2 := asset(v.Sums, 0)
	sig, ok3 := asset(v.Sig, 0)
	man, ok4 := asset(v.Manifest, 0)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return nil
	}
	k := &knownRelease{
		rel:   release{version: v.Version, archive: archive, sums: sums, sig: sig, man: man, listed: v, sha256: f.SHA256},
		man:   &wittymodule.Manifest{ID: mod.ID, Name: mod.Name, Version: v.Version, Repo: mod.Repo, License: v.License, Delivery: v.Delivery, API: v.API, MinServer: v.MinServer, MaxServer: v.MaxServer, Permissions: v.Permissions},
		notes: trimNotes(v.Changelog), url: pageURL("https://github.com/" + mod.Repo + "/releases/tag/v" + v.Version), prerelease: v.Prerelease,
	}
	if t, err := time.Parse(time.DateOnly, v.Released); err == nil {
		k.published = t.UnixMilli()
	}
	return k
}

// applyListedWithdrawals takes what a catalogue withdraws, as a module's own signed list
// would be: only if it is newer than what's kept, so an old catalogue can't bring a version
// back. Call with m.mu held, or before m is shared.
func (m *Manager) applyListedWithdrawals(id string, e *entry, mod *wittymodule.CatalogModule, serial int64) {
	if e.state.Withdrawn != nil && int64(e.state.Withdrawn.Serial) >= serial {
		return
	}
	w := &wittymodule.Withdrawals{Module: id, Serial: int(serial)}
	for i := range mod.Versions {
		if x := mod.Versions[i].Withdrawn; x != nil {
			w.Versions = append(w.Versions, wittymodule.Withdrawn{Version: mod.Versions[i].Version, Severity: x.Severity, Reason: x.Reason})
		}
	}
	if len(w.Versions) == 0 && e.state.Withdrawn == nil {
		return
	}
	inUse, ver := e.dir != "", e.man.Version
	e.state.Withdrawn = w
	if v := w.Find(ver); v != nil && inUse {
		m.log.Warn("modules: the version in use was withdrawn", "module", id, "version", ver, "severity", v.Severity, "reason", v.Reason)
		e.note("withdrawn", ver, v.Severity+": "+v.Reason)
		if v.Severity == wittymodule.SeveritySecurity {
			m.resolve(id, e)
		}
	}
}

// checkCatalog brings the catalogue up to date. A catalogue that fails any check changes
// nothing, and the reason shows in the store.
func (m *Manager) checkCatalog(ctx context.Context) error {
	if m.cfg.CatalogURL == "" {
		return nil
	}
	raw, err := m.small(ctx, m.cfg.CatalogURL, wittymodule.MaxCatalog)
	if errors.Is(err, update.ErrNotFound) {
		return nil // not published yet, so there's nothing to learn from it
	}
	if err != nil {
		return fmt.Errorf("the module catalogue: %w", err)
	}
	sig, err := m.small(ctx, m.cfg.CatalogURL+".sig", maxSig)
	if err != nil {
		return fmt.Errorf("the module catalogue's signature: %w", err)
	}
	c, err := verifyCatalog(raw, sig, m.keys)
	if err != nil {
		return fmt.Errorf("the module catalogue was ignored: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if c.Serial < m.catHigh {
		return fmt.Errorf("the module catalogue was ignored: it is older than one already seen (%d, and %d was seen), which can mean someone is holding back updates", c.Serial, m.catHigh)
	}
	if m.cat != nil && c.Serial == m.cat.Serial && m.catFrom == m.cfg.CatalogURL {
		return nil
	}
	if err := writeAtomic(filepath.Join(m.dir, catalogFile), raw); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(m.dir, catalogSig), sig); err != nil {
		return err
	}
	b, _ := json.Marshal(struct {
		High int64 `json:"high"`
	}{c.Serial})
	if err := writeAtomic(filepath.Join(m.dir, serialFile), b); err != nil {
		return err
	}
	m.applyCatalog(c, m.cfg.CatalogURL, nil)
	if err := m.saveState(); err != nil {
		m.log.Warn("modules: saving the state", "err", err)
	}
	return nil
}

// Catalogue reports on the catalogue in use, for the store.
func (m *Manager) catalogInfo() *CatalogInfo {
	info := &CatalogInfo{Error: m.catErr}
	if m.cat != nil {
		info.Name, info.Serial, info.Generated = m.cat.Name, m.cat.Serial, m.cat.Generated
	}
	return info
}

// small reads a small file: from a starter set's folder, or the catalogue's own address.
func (m *Manager) small(ctx context.Context, raw string, limit int64) ([]byte, error) {
	if b, ok, err := m.routeSmall(ctx, raw, limit); ok {
		return b, err
	}
	if p, ok := m.localFile(raw); ok {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, limit+1))
		if err != nil {
			return nil, err
		}
		if int64(len(b)) > limit {
			return nil, fmt.Errorf("%s is larger than expected", filepath.Base(p))
		}
		return b, nil
	}
	return m.fetch.Small(ctx, raw, limit)
}

// openFile opens a release's archive for reading, wherever it is.
func (m *Manager) openFile(ctx context.Context, raw string) (io.ReadCloser, int64, error) {
	if body, size, ok, err := m.routeOpen(ctx, raw); ok {
		return body, size, err
	}
	if p, ok := m.localFile(raw); ok {
		f, err := os.Open(p)
		if err != nil {
			return nil, 0, err
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, 0, err
		}
		return f, st.Size(), nil
	}
	res, err := m.fetch.Get(ctx, raw, "application/octet-stream")
	if err != nil {
		return nil, 0, err
	}
	return res.Body, res.ContentLength, nil
}

// localFile finds the file a file: address means. It only ever answers inside the starter
// set: a catalogue can't name anything else on this machine, and these are checked again
// here because a file: address is the one kind that doesn't come through the checked client.
func (m *Manager) localFile(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "file" || m.cfg.Starter == "" {
		return "", false
	}
	root, err := filepath.Abs(m.cfg.Starter)
	if err != nil {
		return "", false
	}
	p := filepath.FromSlash(strings.TrimPrefix(u.Path, "/"))
	if runtime.GOOS != "windows" {
		p = string(filepath.Separator) + p
	}
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return p, true
}

// checkListed compares what the catalogue said with what the release's signed files say. Any
// difference refuses the install.
func checkListed(id string, r *release, man *wittymodule.Manifest, want string) error {
	if r.listed == nil {
		return nil
	}
	if err := r.listed.MatchesManifest(id, man); err != nil {
		return fmt.Errorf("the catalogue and the release disagree: %w", err)
	}
	if name := man.Archive(runtime.GOOS, runtime.GOARCH); r.archive.Name != name {
		return fmt.Errorf("the catalogue points at %s, and this system's archive is %s", r.archive.Name, name)
	}
	if r.sha256 != want {
		return errors.New("the catalogue and the release's signed list disagree about the archive")
	}
	return nil
}

// maxIconFile is the most a catalogue may say an icon holds.
const maxIconFile = 256 << 10

var pngStart = []byte("\x89PNG\r\n\x1a\n")

// IconFile is where a module's picture from the catalogue is kept, fetched the first time it's
// asked for. It's checked against the signed catalogue, so it can't be swapped for anything else.
func (m *Manager) IconFile(ctx context.Context, id string) (string, error) {
	m.mu.Lock()
	e, from := m.mods[id], m.catFrom
	var icon *wittymodule.CatalogFile
	if e != nil && e.listed != nil {
		icon = e.listed.Icon
	}
	m.mu.Unlock()
	if icon == nil {
		return "", ErrUnknown
	}
	p := filepath.Join(m.dir, "icons", id+"-"+icon.SHA256+".png")
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	u, err := wittymodule.ResolveCatalogURL(from, icon.URL)
	if err != nil {
		return "", err
	}
	b, err := m.small(ctx, u, maxIconFile)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	if int64(len(b)) != icon.Size || !strings.EqualFold(hex.EncodeToString(sum[:]), icon.SHA256) || !bytes.HasPrefix(b, pngStart) {
		return "", fmt.Errorf("%s's picture isn't the one the catalogue lists", id)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	return p, writeAtomic(p, b)
}
