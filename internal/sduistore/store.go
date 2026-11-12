// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package sduistore keeps the server's UI configuration, ui_config.json in the
// data folder: the pages and navigation an administrator changed (pkg/sdui;
// what nobody changed comes from the built-in pages) and the theme overrides
// the Master Theme Engine applies. It is the one place both are stored.
//
// Beside it sits ui_config.draft.json while an administrator is working on a
// change they haven't published: the same thing, shown only to someone
// holding a preview token.
//
// Every save is a new revision. The file is written atomically, a copy of each
// revision is kept in ui_config.history/ (the newest MaxHistory), and an
// administrator can go back to any of them. A file that can't be read or
// doesn't validate is set aside as ui_config.json.bad and the last good
// configuration is used instead: the newest valid revision in the history, or
// the built-in pages. Someone may edit the file by hand while the server runs:
// it is read again when its modification time or size changes, checked on
// every read (no file watcher).
package sduistore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

const (
	// FileName is the configuration's name in the data folder.
	FileName = "ui_config.json"
	// HistoryDir holds a copy of each recent revision, beside FileName.
	HistoryDir = "ui_config.history"
	// MaxHistory is how many revisions are kept for going back.
	MaxHistory = 10
	// DraftName is the unpublished draft's name in the data folder.
	DraftName = "ui_config.draft.json"
	// FormatVersion is the version of the file's own layout. Version 1 held
	// "screens": trees of low-level nodes.
	FormatVersion = 2
	// MaxBytes is the largest configuration accepted, from disk or from a request.
	MaxBytes = 16 << 20
	// MaxPages bounds how many pages a configuration may hold.
	MaxPages = sdui.MaxPages
)

// Config is the whole of ui_config.json.
type Config struct {
	// Version is FormatVersion.
	Version int `json:"version"`
	// Revision counts saves; set by the store.
	Revision int64 `json:"revision"`
	// UpdatedAt and UpdatedBy say when and by whom the revision was saved.
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
	UpdatedBy string    `json:"updatedBy,omitempty"`
	// Theme holds the theme engine's overrides.
	Theme Theme `json:"theme"`
	// Navigation is the main menu; nil means the built-in one.
	Navigation *sdui.Navigation `json:"navigation,omitempty"`
	// Pages are the pages an administrator changed or added, by ID. A page
	// with a built-in page's ID replaces it; removing it brings the built-in
	// one back.
	Pages map[string]sdui.Page `json:"pages"`
	// Personalisation says whether profiles may arrange pages for themselves.
	Personalisation Personalisation `json:"personalisation,omitzero"`
	// Experiences are the layout options each form factor starts with and the ones
	// profiles may choose (docs/SDUI.md, "Layout options"); nil: the defaults, all
	// allowed.
	Experiences *sdui.ExperiencePolicy `json:"experiences,omitempty"`
	// BaseRevision is set on the draft only: the published revision it was
	// begun from. Publishing applies what the draft changed since that
	// revision on top of what is published now (see Merge). Ignored, and
	// never stored, in the published configuration.
	BaseRevision int64 `json:"baseRevision,omitempty"`
	// ETag identifies this revision; set by the store, never stored.
	ETag string `json:"etag,omitempty"`
}

// Personalisation is the server-wide switch for profiles' own arrangements.
type Personalisation struct {
	// Disabled turns every profile's own arrangement off: pages are shown as
	// authored. What profiles stored is kept for when it is turned back on.
	Disabled bool `json:"disabled,omitempty"`
}

// Theme is the theme engine's part of the file: colour tokens overridden for
// every screen, and per module (or other theme scope).
type Theme struct {
	// Tokens overrides colour tokens everywhere: token name → colour.
	Tokens map[string]string `json:"tokens,omitempty"`
	// Modules overrides them inside one module's parts of the UI, or one
	// page's or section's scope: scope → token → colour.
	Modules map[string]map[string]string `json:"modules,omitempty"`
}

// Revision describes one kept revision.
type Revision struct {
	Revision  int64     `json:"revision"`
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
	UpdatedBy string    `json:"updatedBy,omitempty"`
	ETag      string    `json:"etag"`
	Pages     []string  `json:"pages"`
	Size      int64     `json:"size"`
	Current   bool      `json:"current"`
}

// Errors.
var (
	// ErrConflict: the configuration changed since the caller read it.
	ErrConflict = errors.New("the UI configuration changed since it was read")
	// ErrNoRevision: that revision isn't kept (any more).
	ErrNoRevision = errors.New("no such revision is kept")
)

// MergeConflictError: publishing the draft would undo a change made to the
// published configuration after the draft was begun, because the draft
// changed the same items.
type MergeConflictError struct {
	// Base is the published revision the draft was begun from; Current the
	// one published now.
	Base, Current int64
	// Items are the paths both sides changed differently: /pages/<id>,
	// /navigation, /theme, /personalisation. "/" when the base revision is
	// no longer kept, so nothing can be told apart.
	Items []string
}

func (e *MergeConflictError) Error() string {
	if len(e.Items) == 1 && e.Items[0] == "/" {
		return fmt.Sprintf("the draft was begun from revision %d, which is no longer kept, and revision %d is published now: what each changed can't be told apart", e.Base, e.Current)
	}
	return fmt.Sprintf("since the draft was begun from revision %d, revision %d was published with other changes to %s, which the draft changes too",
		e.Base, e.Current, strings.Join(e.Items, ", "))
}

// InvalidError lists everything wrong with a configuration.
type InvalidError struct {
	Issues []sdui.Issue
}

func (e *InvalidError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "the UI configuration has %d problem(s):", len(e.Issues))
	for i, is := range e.Issues {
		if i == 5 {
			fmt.Fprintf(&b, " …")
			break
		}
		b.WriteString(" ")
		b.WriteString(is.String())
		b.WriteString(";")
	}
	return strings.TrimSuffix(b.String(), ";")
}

func (e *InvalidError) add(path, format string, args ...any) {
	e.Issues = append(e.Issues, sdui.Issue{Path: path, Severity: "error", Message: fmt.Sprintf(format, args...)})
}

// Options configure a Store.
type Options struct {
	// Log receives the warnings about a file that had to be replaced.
	Log *slog.Logger
	// CheckTheme validates the theme section beyond the basic rules (token
	// names known, colours valid). nil: only the basic rules.
	CheckTheme func(Theme) []sdui.Issue
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// Store is ui_config.json, safe for concurrent use.
type Store struct {
	dir, path, histDir string
	log                *slog.Logger
	checkTheme         func(Theme) []sdui.Issue
	now                func() time.Time

	mu    sync.RWMutex
	cur   *Config
	draft *Config   // the unpublished draft, nil when there is none
	mod   time.Time // the file's modification time and size as last read or written
	size  int64
}

// Open loads dir/ui_config.json, creating an empty one (every page the
// built-in one) on the first run, or recovering as the package documentation describes.
func Open(dir string, opt Options) (*Store, error) {
	if dir == "" {
		return nil, errors.New("sduistore: no data folder")
	}
	s := &Store{dir: dir, path: filepath.Join(dir, FileName), histDir: filepath.Join(dir, HistoryDir),
		log: opt.Log, checkTheme: opt.CheckTheme, now: opt.Now}
	if s.log == nil {
		s.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if s.now == nil {
		s.now = time.Now
	}
	if err := os.MkdirAll(s.histDir, 0o750); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.loadDraft()
	raw, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if c := s.newestGoodRevision(); c != nil {
			s.log.Warn("ui_config.json was missing: restored the newest kept revision", "cat", "sdui", "revision", c.Revision)
			return s, s.adopt(c)
		}
		c := s.defaults()
		c.Revision = 1
		if err := s.write(c); err != nil {
			return nil, err
		}
		s.cur = c
		return s, nil
	case err != nil:
		return nil, err
	}
	c, perr := s.parseAndValidate(raw)
	if perr == nil {
		s.cur = c
		s.cur.ETag = ETag(c)
		s.remember()
		// A revision the history doesn't have yet (written by hand while stopped).
		if _, err := os.Stat(s.historyPath(c.Revision)); err != nil {
			_ = s.keepHistory(c)
		}
		return s, nil
	}
	s.setAside(raw, perr)
	c = s.newestGoodRevision()
	if c == nil {
		s.log.Warn("ui_config.json could not be used and no kept revision could either: using the built-in pages", "cat", "sdui", "err", perr)
		c = s.defaults()
		c.Revision = s.nextRevision()
		c.UpdatedBy = "defaults (recovered)"
		if err := s.write(c); err != nil {
			return nil, err
		}
		s.cur = c
		return s, nil
	}
	s.log.Warn("ui_config.json could not be used: went back to the newest good revision", "cat", "sdui", "err", perr, "revision", c.Revision)
	return s, s.adopt(c)
}

// adopt makes a recovered revision current and writes it back as the file.
// c may be the configuration readers hold, so it is copied, never changed.
func (s *Store) adopt(c *Config) error {
	cp := *c
	cp.ETag = ""
	b, err := encode(&cp)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(s.path, b); err != nil {
		return err
	}
	cp.ETag = ETag(&cp)
	s.cur = &cp
	s.remember()
	return nil
}

func (s *Store) defaults() *Config {
	return &Config{Version: FormatVersion, UpdatedAt: s.now().UTC(), UpdatedBy: "defaults", Pages: map[string]sdui.Page{}}
}

// Path is the file's location.
func (s *Store) Path() string { return s.path }

// Current is the configuration now in force. It is shared: don't modify it;
// Clone it first.
func (s *Store) Current() *Config {
	s.maybeReload()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Update applies fn to a copy of the current configuration and saves the
// result as a new revision. ifMatch, when not empty, must be the current
// ETag (ErrConflict otherwise). The result is validated (*InvalidError).
func (s *Store) Update(ifMatch, by string, fn func(*Config) error) (*Config, error) {
	s.maybeReload()
	s.mu.Lock()
	defer s.mu.Unlock()
	if ifMatch != "" && !SameETag(ifMatch, s.cur.ETag) {
		return nil, ErrConflict
	}
	next := s.cur.Clone()
	if err := fn(next); err != nil {
		return nil, err
	}
	return s.saveLocked(next, by)
}

// Replace saves c, a whole configuration, as a new revision when ifMatch is
// the current ETag. Revision, UpdatedAt, UpdatedBy and ETag in c are ignored.
func (s *Store) Replace(c *Config, ifMatch, by string) (*Config, error) {
	if c == nil {
		return nil, errors.New("no configuration")
	}
	return s.Update(ifMatch, by, func(next *Config) error {
		cp := c.Clone()
		*next = *cp
		return nil
	})
}

// Rollback makes a kept revision current again, as a new revision. ifMatch
// works as in Update.
func (s *Store) Rollback(rev int64, ifMatch, by string) (*Config, error) {
	s.maybeReload()
	s.mu.Lock()
	defer s.mu.Unlock()
	if ifMatch != "" && !SameETag(ifMatch, s.cur.ETag) {
		return nil, ErrConflict
	}
	raw, err := os.ReadFile(s.historyPath(rev))
	if err != nil {
		return nil, ErrNoRevision
	}
	old, err := s.parseAndValidate(raw)
	if err != nil {
		return nil, fmt.Errorf("revision %d can't be used: %w", rev, err)
	}
	if by == "" {
		by = "rollback"
	}
	return s.saveLocked(old, by+" (rollback to "+strconv.FormatInt(rev, 10)+")")
}

// History lists the kept revisions, newest first.
func (s *Store) History() ([]Revision, error) {
	s.maybeReload()
	s.mu.RLock()
	cur := s.cur.Revision
	s.mu.RUnlock()
	revs := s.historyRevisions()
	out := make([]Revision, 0, len(revs))
	for i := len(revs) - 1; i >= 0; i-- {
		p := s.historyPath(revs[i])
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		c, err := Parse(raw)
		if err != nil {
			continue
		}
		c.Revision = revs[i]
		out = append(out, Revision{Revision: revs[i], UpdatedAt: c.UpdatedAt, UpdatedBy: c.UpdatedBy, ETag: ETag(c),
			Pages: sortedKeys(c.Pages), Size: int64(len(raw)), Current: revs[i] == cur})
	}
	return out, nil
}

// saveLocked validates next, numbers it and writes it. s.mu is held.
func (s *Store) saveLocked(next *Config, by string) (*Config, error) {
	next.Version = FormatVersion
	next.BaseRevision = 0
	if err := s.validate(next); err != nil {
		return nil, err
	}
	next.Revision = s.nextRevision()
	next.UpdatedAt = s.now().UTC()
	next.UpdatedBy = by
	next.ETag = ""
	if err := s.write(next); err != nil {
		return nil, err
	}
	s.cur = next
	return next, nil
}

// write puts c in the file and the history and remembers it. s.mu is held.
func (s *Store) write(c *Config) error {
	c.ETag = ""
	b, err := encode(c)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(s.path, b); err != nil {
		return err
	}
	if err := writeFileAtomic(s.historyPath(c.Revision), b); err != nil {
		s.log.Warn("could not keep a copy of the UI configuration", "cat", "sdui", "err", err)
	}
	s.prune()
	c.ETag = ETag(c)
	s.cur = c
	s.remember()
	return nil
}

func (s *Store) keepHistory(c *Config) error {
	cp := *c
	cp.ETag = ""
	b, err := encode(&cp)
	if err != nil {
		return err
	}
	err = writeFileAtomic(s.historyPath(c.Revision), b)
	s.prune()
	return err
}

// remember notes the file's state as just read or written. s.mu is held.
func (s *Store) remember() {
	if st, err := os.Stat(s.path); err == nil {
		s.mod, s.size = st.ModTime(), st.Size()
	}
}

// maybeReload reads the file again if it changed on disk since.
func (s *Store) maybeReload() {
	st, err := os.Stat(s.path)
	s.mu.RLock()
	same := err == nil && st.ModTime().Equal(s.mod) && st.Size() == s.size
	s.mu.RUnlock()
	if same {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reloadLocked()
}

func (s *Store) reloadLocked() {
	st, err := os.Stat(s.path)
	if err == nil && st.ModTime().Equal(s.mod) && st.Size() == s.size {
		return // another reader got here first
	}
	if err != nil {
		s.log.Warn("ui_config.json disappeared: writing the current configuration back", "cat", "sdui", "err", err)
		_ = s.adopt(s.cur)
		return
	}
	raw, err := os.ReadFile(s.path)
	if err == nil && len(raw) > MaxBytes {
		err = fmt.Errorf("larger than %d bytes", MaxBytes)
	}
	var c *Config
	if err == nil {
		c, err = s.parseAndValidate(raw)
	}
	if err != nil {
		s.setAside(raw, err)
		s.log.Warn("ui_config.json was changed on disk and can't be used: keeping the last good configuration", "cat", "sdui", "err", err, "revision", s.cur.Revision)
		_ = s.adopt(s.cur)
		return
	}
	if contentHash(c) == contentHash(s.cur) {
		s.remember() // touched, or saved unchanged
		return
	}
	s.log.Info("ui_config.json was changed on disk: using it", "cat", "sdui")
	if _, err := s.saveLocked(c, "file on disk"); err != nil {
		s.log.Warn("could not save the configuration changed on disk", "cat", "sdui", "err", err)
		s.remember()
	}
}

// setAside keeps an unusable file as ui_config.json.bad for its author to look at.
func (s *Store) setAside(raw []byte, why error) {
	if len(raw) == 0 {
		return
	}
	if err := writeFileAtomic(s.path+".bad", raw); err != nil {
		s.log.Warn("could not keep the unusable ui_config.json", "cat", "sdui", "err", err)
		return
	}
	s.log.Warn("the unusable ui_config.json was kept as ui_config.json.bad", "cat", "sdui", "why", why)
}

// ---- history files ----

var histName = regexp.MustCompile(`^([0-9]{1,18})\.json$`)

func (s *Store) historyPath(rev int64) string {
	return filepath.Join(s.histDir, fmt.Sprintf("%010d.json", rev))
}

// historyRevisions are the kept revision numbers, oldest first.
func (s *Store) historyRevisions() []int64 {
	ents, err := os.ReadDir(s.histDir)
	if err != nil {
		return nil
	}
	var out []int64
	for _, e := range ents {
		if m := histName.FindStringSubmatch(e.Name()); m != nil && e.Type().IsRegular() {
			if n, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				out = append(out, n)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *Store) nextRevision() int64 {
	n := int64(0)
	if s.cur != nil {
		n = s.cur.Revision
	}
	if revs := s.historyRevisions(); len(revs) > 0 && revs[len(revs)-1] > n {
		n = revs[len(revs)-1]
	}
	return n + 1
}

func (s *Store) prune() {
	revs := s.historyRevisions()
	for len(revs) > MaxHistory {
		_ = os.Remove(s.historyPath(revs[0]))
		revs = revs[1:]
	}
}

func (s *Store) newestGoodRevision() *Config {
	revs := s.historyRevisions()
	for i := len(revs) - 1; i >= 0; i-- {
		raw, err := os.ReadFile(s.historyPath(revs[i]))
		if err != nil {
			continue
		}
		if c, err := s.parseAndValidate(raw); err == nil {
			c.Revision = revs[i]
			return c
		}
	}
	return nil
}

// ---- parsing and validation ----

// Parse decodes a configuration strictly: no unknown fields, no trailing
// data. It does not validate.
func Parse(b []byte) (*Config, error) {
	if len(b) > MaxBytes {
		return nil, fmt.Errorf("the UI configuration is larger than %d bytes", MaxBytes)
	}
	c := &Config{}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(c); err != nil {
		return nil, fmt.Errorf("the UI configuration isn't valid JSON: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("the UI configuration has something after its end")
	}
	if c.Pages == nil {
		c.Pages = map[string]sdui.Page{}
	}
	c.ETag = ""
	return c, nil
}

func (s *Store) parseAndValidate(b []byte) (*Config, error) {
	c, err := Parse(b)
	if err != nil {
		return nil, err
	}
	if err := s.validate(c); err != nil {
		return nil, err
	}
	return c, nil
}

var (
	moduleID  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	tokenName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)
	colour    = regexp.MustCompile(`^(#[0-9A-Fa-f]{3,4}|#[0-9A-Fa-f]{6}|#[0-9A-Fa-f]{8}|(rgb|rgba|hsl|hsla)\([0-9., %/]{1,60}\))$`)
)

// ValidModuleID reports whether id is a well-formed module id (or theme scope).
func ValidModuleID(id string) bool { return moduleID.MatchString(id) }

// Validate checks c with the basic theme rules only.
func Validate(c *Config) error { return (&Store{}).validate(c) }

// Check answers everything wrong with c, warnings included, each located
// from the top of the file (/pages/home/sections/2/props/title).
func (s *Store) Check(c *Config) []sdui.Issue {
	ve := &InvalidError{}
	if c.Version != FormatVersion {
		ve.add("/version", "unsupported version %d (want %d)", c.Version, FormatVersion)
	}
	if len(c.Pages) > MaxPages {
		ve.add("/pages", "%d pages; at most %d", len(c.Pages), MaxPages)
	}
	for _, id := range sortedKeys(c.Pages) {
		pg := c.Pages[id]
		p := "/pages/" + id
		if pg.ID != id {
			ve.Issues = append(ve.Issues, sdui.Issue{Path: p + "/id", Severity: "error", Message: fmt.Sprintf("the page is stored as %q but calls itself %q", id, pg.ID), Fix: "Make the two the same."})
		}
		if b, ok := sdui.StockPage(id); ok && pg.Kind != b.Kind {
			ve.Issues = append(ve.Issues, sdui.Issue{Path: p + "/kind", Severity: "error", Message: fmt.Sprintf("%q is the built-in %s page; a page that replaces it must be a %s page too", id, b.Kind, b.Kind),
				Fix: "Give your page another ID, or make its kind " + b.Kind + "."})
		}
		for _, is := range sdui.ValidatePage(&pg) {
			is.Path = p + is.Path
			ve.Issues = append(ve.Issues, is)
		}
	}
	if c.Navigation != nil {
		exists := func(id string) bool {
			if _, ok := c.Pages[id]; ok {
				return true
			}
			_, ok := sdui.StockPage(id)
			return ok
		}
		// A page that is neither the file's nor built in may be a module's: the store doesn't
		// know which modules are in use, and a module turned off mustn't make a good file
		// bad. The editor's routes check it against the modules in use when it is saved;
		// here it is a warning, and the menu leaves the entry out while there is no page.
		missing := map[string]bool{}
		for _, it := range c.Navigation.Items {
			if it.Type == "page" && sdui.ValidID(it.Page) && !exists(it.Page) {
				missing[it.Page] = true
			}
		}
		for _, is := range sdui.ValidateNavigation(c.Navigation, func(id string) bool { return exists(id) || missing[id] }) {
			is.Path = "/navigation" + is.Path
			ve.Issues = append(ve.Issues, is)
		}
		for i, it := range c.Navigation.Items {
			if missing[it.Page] {
				ve.Issues = append(ve.Issues, sdui.Issue{Path: fmt.Sprintf("/navigation/items/%d/page", i), Severity: "warning",
					Message: fmt.Sprintf("there is no page called %q here, unless a module brings it; while there is none, the entry is left out", it.Page),
					Fix:     "Turn on the module whose page it is, create the page, or remove the entry."})
			}
		}
	}
	checkTokens := func(p string, toks map[string]string) {
		for _, k := range sortedKeys(toks) {
			if !tokenName.MatchString(k) {
				ve.add(p+"/"+k, "invalid token name")
			}
			if !colour.MatchString(toks[k]) {
				ve.add(p+"/"+k, "not a colour: %q", toks[k])
			}
		}
	}
	checkTokens("/theme/tokens", c.Theme.Tokens)
	for _, mod := range sortedKeys(c.Theme.Modules) {
		if !moduleID.MatchString(mod) {
			ve.add("/theme/modules/"+mod, "invalid module id")
		}
		checkTokens("/theme/modules/"+mod, c.Theme.Modules[mod])
	}
	for _, is := range sdui.ValidateExperiencePolicy(c.Experiences) {
		is.Path = "/experiences" + is.Path
		ve.Issues = append(ve.Issues, is)
	}
	if s.checkTheme != nil {
		for _, is := range s.checkTheme(c.Theme) {
			if is.Severity == "" {
				is.Severity = "error"
			}
			ve.Issues = append(ve.Issues, is)
		}
	}
	return ve.Issues
}

func (s *Store) validate(c *Config) error {
	var errs []sdui.Issue
	for _, is := range s.Check(c) {
		if is.Severity != "warning" {
			errs = append(errs, is)
		}
	}
	if len(errs) > 0 {
		return &InvalidError{Issues: errs}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---- encoding, hashing ----

func encode(c *Config) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Clone is a deep copy.
func (c *Config) Clone() *Config {
	cp := *c
	cp.ETag = ""
	b, err := encode(&cp)
	if err == nil {
		if out, err := Parse(b); err == nil {
			out.ETag = c.ETag
			return out
		}
	}
	// Not reachable for a configuration that came from Parse; keep the copy shallow.
	return &cp
}

// contentHash identifies what a configuration says, whatever its revision.
func contentHash(c *Config) string {
	cp := *c
	cp.Revision, cp.UpdatedAt, cp.UpdatedBy, cp.ETag = 0, time.Time{}, "", ""
	b, _ := json.Marshal(&cp) // map keys sorted
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:24]
}

// ETag is the weak entity tag of a revision: its number and what it holds.
func ETag(c *Config) string {
	return `W/"ui-` + strconv.FormatInt(c.Revision, 10) + "-" + contentHash(c) + `"`
}

// SameETag compares entity tags weakly, as If-Match and If-None-Match do.
// header may be a list, or "*".
func SameETag(header, etag string) bool {
	if etag == "" {
		return false
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || strings.TrimPrefix(part, "W/") == want {
			return true
		}
	}
	return false
}

// writeFileAtomic writes through a temporary file in the same folder, synced
// and renamed into place, so a crash leaves the old file or the new one.
func writeFileAtomic(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".ui_config-*.tmp")
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
	if err := os.Chmod(tmp, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ---- the draft ----

func (s *Store) draftPath() string { return filepath.Join(s.dir, DraftName) }

// loadDraft reads the draft at start. One that can't be used is set aside.
// s.mu is held.
func (s *Store) loadDraft() {
	raw, err := os.ReadFile(s.draftPath())
	if err != nil {
		return
	}
	c, err := s.parseAndValidate(raw)
	if err != nil {
		s.log.Warn("the UI draft could not be used: kept as "+DraftName+".bad", "cat", "sdui", "err", err)
		_ = os.Rename(s.draftPath(), s.draftPath()+".bad")
		return
	}
	if c.BaseRevision <= 0 && s.cur != nil {
		// A draft from before drafts remembered their base: it is taken to be
		// made from what is published, as publishing it used to assume.
		c.BaseRevision = s.cur.Revision
	}
	c.ETag = draftETag(c)
	s.draft = c
}

func draftETag(c *Config) string {
	return `W/"draft-` + strconv.FormatInt(c.Revision, 10) + "-" + contentHash(c) + `"`
}

// Draft is the unpublished draft, nil when there is none. Shared: Clone it
// before changing it.
func (s *Store) Draft() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.draft
}

// SaveDraft keeps c as the draft. ifMatch, when not empty, must be the
// draft's ETag, or the current configuration's when there is no draft yet
// (ErrConflict otherwise). The draft is validated like a configuration; its
// Revision counts its own saves. Its BaseRevision is the published revision
// it was begun from: the current one for a new draft, kept as it was for a
// draft saved again (c.BaseRevision is ignored).
func (s *Store) SaveDraft(c *Config, ifMatch, by string) (*Config, error) {
	if c == nil {
		return nil, errors.New("no configuration")
	}
	s.maybeReload()
	s.mu.Lock()
	defer s.mu.Unlock()
	base, n, from := s.cur.ETag, int64(0), s.cur.Revision
	if s.draft != nil {
		base, n, from = s.draft.ETag, s.draft.Revision, s.draft.BaseRevision
	}
	if ifMatch != "" && !SameETag(ifMatch, base) {
		return nil, ErrConflict
	}
	next := c.Clone()
	next.Version = FormatVersion
	if err := s.validate(next); err != nil {
		return nil, err
	}
	next.Revision, next.UpdatedAt, next.UpdatedBy, next.ETag, next.BaseRevision = n+1, s.now().UTC(), by, "", from
	b, err := encode(next)
	if err != nil {
		return nil, err
	}
	if err := writeFileAtomic(s.draftPath(), b); err != nil {
		return nil, err
	}
	next.ETag = draftETag(next)
	s.draft = next
	return next, nil
}

// DiscardDraft forgets the draft.
func (s *Store) DiscardDraft() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.draft = nil
	if err := os.Remove(s.draftPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Publish makes what the draft changed part of the configuration, as a new
// revision, and forgets the draft. ifMatch, when not empty, must be the
// draft's ETag.
//
// The draft is not simply copied over: what was published since the draft was
// begun would be lost. Instead the draft is compared with the revision it was
// begun from, and only what it changed (page by page, the navigation, the
// theme, the personalisation switch) is applied on top of what is published
// now (see Merge). When something the draft changed was also changed, the
// other way, in what is published, the answer is a *MergeConflictError and
// nothing changes, unless prefer says which side wins for those items:
// "draft" or "published".
func (s *Store) Publish(ifMatch, by string) (*Config, error) {
	return s.PublishPreferring(ifMatch, by, "")
}

// PublishPreferring is Publish, with prefer ("", "draft" or "published")
// settling the items both sides changed.
func (s *Store) PublishPreferring(ifMatch, by, prefer string) (*Config, error) {
	switch prefer {
	case "", "draft", "published":
	default:
		return nil, errors.New("prefer: draft or published")
	}
	s.maybeReload()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return nil, ErrNoRevision
	}
	if ifMatch != "" && !SameETag(ifMatch, s.draft.ETag) {
		return nil, ErrConflict
	}
	var next *Config
	if s.draft.BaseRevision == s.cur.Revision {
		next = s.draft.Clone() // nothing was published since it was begun
	} else {
		base, err := s.revisionLocked(s.draft.BaseRevision)
		if err != nil {
			if prefer != "draft" {
				return nil, &MergeConflictError{Base: s.draft.BaseRevision, Current: s.cur.Revision, Items: []string{"/"}}
			}
			next = s.draft.Clone()
		} else {
			merged, conflicts := Merge(base, s.draft, s.cur, prefer)
			if len(conflicts) > 0 && prefer == "" {
				return nil, &MergeConflictError{Base: s.draft.BaseRevision, Current: s.cur.Revision, Items: conflicts}
			}
			next = merged
		}
	}
	saved, err := s.saveLocked(next, by)
	if err != nil {
		return nil, err
	}
	s.draft = nil
	_ = os.Remove(s.draftPath())
	return saved, nil
}

// Merge is a three-way merge of the configuration: base is what the draft was
// begun from, draft what it is now, published what is published now. Each
// item (every page by ID, the navigation, the theme, the personalisation
// switch) the draft left as it was in base is taken from published; one the
// draft changed is taken from the draft, unless published changed it too, to
// something else: then it is a conflict, named by its path (/pages/<id>,
// /navigation, /theme, /personalisation), and the result keeps what prefer
// says ("draft", "published"; with "" the result is not to be used). A page
// added or removed counts as changed. The result is a new configuration; none
// of the three is modified.
func Merge(base, draft, published *Config, prefer string) (*Config, []string) {
	out := published.Clone()
	out.ETag = ""
	var conflicts []string
	// pick says which side an item comes from: the draft's (true) or published's.
	pick := func(path string, b, d, p any) bool {
		if sameValue(b, d) {
			return false
		}
		if sameValue(b, p) || sameValue(d, p) {
			return true
		}
		conflicts = append(conflicts, path)
		return prefer == "draft"
	}
	ids := map[string]bool{}
	for _, c := range []*Config{base, draft, published} {
		for id := range c.Pages {
			ids[id] = true
		}
	}
	if out.Pages == nil {
		out.Pages = map[string]sdui.Page{}
	}
	for _, id := range sortedKeys(ids) {
		b, d, p := pageOrNil(base, id), pageOrNil(draft, id), pageOrNil(published, id)
		if !pick("/pages/"+id, b, d, p) {
			continue
		}
		if d == nil {
			delete(out.Pages, id)
		} else {
			out.Pages[id] = sdui.ClonePage(*d)
		}
	}
	if pick("/navigation", base.Navigation, draft.Navigation, published.Navigation) {
		out.Navigation = draft.Clone().Navigation
	}
	if pick("/personalisation", base.Personalisation, draft.Personalisation, published.Personalisation) {
		out.Personalisation = draft.Personalisation
	}
	if pick("/theme", base.Theme, draft.Theme, published.Theme) {
		out.Theme = draft.Clone().Theme
	}
	if pick("/experiences", base.Experiences, draft.Experiences, published.Experiences) {
		out.Experiences = draft.Clone().Experiences
	}
	return out, conflicts
}

func pageOrNil(c *Config, id string) *sdui.Page {
	if p, ok := c.Pages[id]; ok {
		return &p
	}
	return nil
}

// sameValue compares two items of a configuration by what they encode to (an
// absent item and an empty one are the same: both are as built in).
func sameValue(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(normaliseEmpty(x), normaliseEmpty(y))
}

func normaliseEmpty(b []byte) []byte {
	switch string(b) {
	case "{}", "null":
		return []byte("null")
	}
	return b
}

// revisionLocked reads a kept revision, or the current one. s.mu is held.
func (s *Store) revisionLocked(rev int64) (*Config, error) {
	if s.cur != nil && rev == s.cur.Revision {
		return s.cur, nil
	}
	return s.RevisionConfig(rev)
}

// RevisionConfig reads one kept revision.
func (s *Store) RevisionConfig(rev int64) (*Config, error) {
	raw, err := os.ReadFile(s.historyPath(rev))
	if err != nil {
		return nil, ErrNoRevision
	}
	c, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	c.Revision = rev
	c.ETag = ETag(c)
	return c, nil
}
