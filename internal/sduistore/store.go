// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package sduistore keeps the server's UI configuration, ui_config.json in the
// data folder: the server-driven screens (pkg/sdui documents) and the theme
// overrides the Master Theme Engine applies. It is the one place both are
// stored.
//
// Every save is a new revision. The file is written atomically, a copy of each
// revision is kept in ui_config.history/ (the newest MaxHistory), and an
// administrator can go back to any of them. A file that can't be read or
// doesn't validate is set aside as ui_config.json.bad and the last good
// configuration is used instead: the newest valid revision in the history, or
// the stock screens. Someone may edit the file by hand while the server runs:
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
	// FormatVersion is the version of the file's own layout (the screens
	// inside carry sdui.SchemaVersion).
	FormatVersion = 1
	// MaxBytes is the largest configuration accepted, from disk or from a request.
	MaxBytes = 16 << 20
	// MaxScreens bounds how many screens a configuration may hold.
	MaxScreens = 64
)

// Config is the whole of ui_config.json.
type Config struct {
	// Version is FormatVersion.
	Version int `json:"version"`
	// Revision counts saves; set by the store.
	Revision int64 `json:"revision"`
	// UpdatedAt and UpdatedBy say when and by whom (an account name, "defaults",
	// "rollback", "file on disk") this revision was made; set by the store.
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
	UpdatedBy string    `json:"updatedBy,omitempty"`
	// Theme is the theme overrides.
	Theme Theme `json:"theme"`
	// Screens are the authored screens by name; each key equals its
	// document's screen.
	Screens map[string]sdui.Document `json:"screens"`
	// ETag identifies this revision for If-Match. Filled in by the store,
	// never written to the file, ignored when saving.
	ETag string `json:"etag,omitempty"`
}

// Theme is the theme section: colours that replace a theme's own, for the
// whole interface and for the parts belonging to one module. The theme itself
// (its id) is the server's "theme" setting, shared with /api/themes.
type Theme struct {
	// Tokens are global overrides: token name → colour.
	Tokens map[string]string `json:"tokens,omitempty"`
	// Modules are per-module overrides: module id → token name → colour.
	Modules map[string]map[string]string `json:"modules,omitempty"`
}

// Revision describes one kept revision.
type Revision struct {
	Revision  int64     `json:"revision"`
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
	UpdatedBy string    `json:"updatedBy,omitempty"`
	ETag      string    `json:"etag"`
	Screens   []string  `json:"screens"`
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
	e.Issues = append(e.Issues, sdui.Issue{Path: path, Message: fmt.Sprintf(format, args...)})
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

	mu   sync.RWMutex
	cur  *Config
	mod  time.Time // the file's modification time and size as last read or written
	size int64
}

// Open loads dir/ui_config.json, creating it from sdui.DefaultScreens on the
// first run, or recovering as the package documentation describes.
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
	raw, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if c := s.newestGoodRevision(); c != nil {
			s.log.Warn("ui_config.json was missing: restored the newest kept revision", "cat", "ui", "revision", c.Revision)
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
		s.log.Warn("ui_config.json could not be used and no kept revision could either: using the stock screens", "cat", "ui", "err", perr)
		c = s.defaults()
		c.Revision = s.nextRevision()
		c.UpdatedBy = "defaults (recovered)"
		if err := s.write(c); err != nil {
			return nil, err
		}
		s.cur = c
		return s, nil
	}
	s.log.Warn("ui_config.json could not be used: went back to the newest good revision", "cat", "ui", "err", perr, "revision", c.Revision)
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
	return &Config{Version: FormatVersion, UpdatedAt: s.now().UTC(), UpdatedBy: "defaults", Screens: sdui.DefaultScreens()}
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
		names := make([]string, 0, len(c.Screens))
		for k := range c.Screens {
			names = append(names, k)
		}
		sort.Strings(names)
		out = append(out, Revision{Revision: revs[i], UpdatedAt: c.UpdatedAt, UpdatedBy: c.UpdatedBy, ETag: ETag(c),
			Screens: names, Size: int64(len(raw)), Current: revs[i] == cur})
	}
	return out, nil
}

// saveLocked validates next, numbers it and writes it. s.mu is held.
func (s *Store) saveLocked(next *Config, by string) (*Config, error) {
	next.Version = FormatVersion
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
		s.log.Warn("could not keep a copy of the UI configuration", "cat", "ui", "err", err)
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
		s.log.Warn("ui_config.json disappeared: writing the current configuration back", "cat", "ui", "err", err)
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
		s.log.Warn("ui_config.json was changed on disk and can't be used: keeping the last good configuration", "cat", "ui", "err", err, "revision", s.cur.Revision)
		_ = s.adopt(s.cur)
		return
	}
	if contentHash(c) == contentHash(s.cur) {
		s.remember() // touched, or saved unchanged
		return
	}
	s.log.Info("ui_config.json was changed on disk: using it", "cat", "ui")
	if _, err := s.saveLocked(c, "file on disk"); err != nil {
		s.log.Warn("could not save the configuration changed on disk", "cat", "ui", "err", err)
		s.remember()
	}
}

// setAside keeps an unusable file as ui_config.json.bad for its author to look at.
func (s *Store) setAside(raw []byte, why error) {
	if len(raw) == 0 {
		return
	}
	if err := writeFileAtomic(s.path+".bad", raw); err != nil {
		s.log.Warn("could not keep the unusable ui_config.json", "cat", "ui", "err", err)
		return
	}
	s.log.Warn("the unusable ui_config.json was kept as ui_config.json.bad", "cat", "ui", "why", why)
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
// data, every screen through sdui.Unmarshal. It does not validate.
func Parse(b []byte) (*Config, error) {
	if len(b) > MaxBytes {
		return nil, fmt.Errorf("the UI configuration is larger than %d bytes", MaxBytes)
	}
	var raw struct {
		Version   int                        `json:"version"`
		Revision  int64                      `json:"revision"`
		UpdatedAt time.Time                  `json:"updatedAt"`
		UpdatedBy string                     `json:"updatedBy"`
		Theme     Theme                      `json:"theme"`
		Screens   map[string]json.RawMessage `json:"screens"`
		ETag      string                     `json:"etag"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("the UI configuration isn't valid JSON: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("the UI configuration has something after its end")
	}
	c := &Config{Version: raw.Version, Revision: raw.Revision, UpdatedAt: raw.UpdatedAt, UpdatedBy: raw.UpdatedBy, Theme: raw.Theme,
		Screens: make(map[string]sdui.Document, len(raw.Screens))}
	for name, r := range raw.Screens {
		doc, err := sdui.Unmarshal(r)
		if err != nil {
			return nil, fmt.Errorf("screen %q: %w", name, err)
		}
		c.Screens[name] = doc
	}
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

// ScreenName is the form of a screen's name.
var ScreenName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

var (
	moduleID  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	tokenName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)
)

// ValidModuleID reports whether id is a well-formed module id.
func ValidModuleID(id string) bool { return moduleID.MatchString(id) }

// Validate checks c with the basic theme rules only.
func Validate(c *Config) error { return (&Store{}).validate(c) }

func (s *Store) validate(c *Config) error {
	ve := &InvalidError{}
	if c.Version != FormatVersion {
		ve.add("/version", "unsupported version %d (want %d)", c.Version, FormatVersion)
	}
	if len(c.Screens) > MaxScreens {
		ve.add("/screens", "%d screens; at most %d", len(c.Screens), MaxScreens)
	}
	names := make([]string, 0, len(c.Screens))
	for k := range c.Screens {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		doc := c.Screens[name]
		p := "/screens/" + name
		if !ScreenName.MatchString(name) {
			ve.add(p, "screen names are lower-case letters, digits, - and _")
		}
		if doc.Screen != name {
			ve.add(p+"/screen", "is %q but the screen is stored as %q", doc.Screen, name)
		}
		if err := sdui.Validate(&doc); err != nil {
			var sv *sdui.ValidationError
			if errors.As(err, &sv) {
				for _, is := range sv.Issues {
					ve.add(p+is.Path, "%s", is.Message)
				}
			} else {
				ve.add(p, "%v", err)
			}
		}
	}
	checkTokens := func(p string, toks map[string]string) {
		for _, k := range sortedKeys(toks) {
			if !tokenName.MatchString(k) {
				ve.add(p+"/"+k, "invalid token name")
			}
			if err := sdui.ValidateColor(toks[k]); err != nil || strings.HasPrefix(toks[k], "token:") {
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
	if s.checkTheme != nil {
		ve.Issues = append(ve.Issues, s.checkTheme(c.Theme)...)
	}
	if len(ve.Issues) > 0 {
		return ve
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
	h := sha256.New()
	for _, name := range sortedKeys(c.Screens) {
		fmt.Fprintf(h, "screen %s %s\n", name, sdui.ETag(c.Screens[name]))
	}
	th, _ := json.Marshal(c.Theme) // map keys sorted
	fmt.Fprintf(h, "theme %s\n", th)
	return hex.EncodeToString(h.Sum(nil))[:24]
}

// ETag is the weak entity tag of a revision: its number and what it holds
// (each screen's sdui.ETag and the theme section).
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
