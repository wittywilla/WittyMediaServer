// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sduistore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFirstRunSeedsDefaults(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	c := s.Current()
	if c.Revision != 1 || len(c.Pages) != 0 || c.Navigation != nil || c.Version != FormatVersion {
		t.Fatalf("revision %d, %d pages: a new configuration changes nothing", c.Revision, len(c.Pages))
	}
	if c.ETag == "" {
		t.Error("no etag")
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
		t.Error(err)
	}
	h, _ := s.History()
	if len(h) != 1 || !h[0].Current {
		t.Errorf("history %+v", h)
	}
	// Opened again: the same revision, not a new one.
	if c2 := open(t, dir).Current(); c2.Revision != 1 || c2.ETag != c.ETag {
		t.Errorf("reopened: %d %s, want 1 %s", c2.Revision, c2.ETag, c.ETag)
	}
}

func TestUpdateIfMatchAndValidation(t *testing.T) {
	s := open(t, t.TempDir())
	c0 := s.Current()
	c1, err := s.Update(c0.ETag, "owner", func(c *Config) error {
		c.Theme.Tokens = map[string]string{"accent": "#ff0000"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if c1.Revision != 2 || c1.ETag == c0.ETag || c1.UpdatedBy != "owner" {
		t.Errorf("after update: %+v", c1)
	}
	if _, err := s.Update(c0.ETag, "owner", func(*Config) error { return nil }); !errors.Is(err, ErrConflict) {
		t.Errorf("stale If-Match: %v", err)
	}
	_, err = s.Update("", "owner", func(c *Config) error {
		c.Theme.Modules = map[string]map[string]string{"Bad Module": {"accent": "url(x)"}}
		return nil
	})
	var ie *InvalidError
	if !errors.As(err, &ie) || len(ie.Issues) < 2 {
		t.Errorf("invalid theme: %v", err)
	}
	_, err = s.Update("", "owner", func(c *Config) error {
		c.Pages["home"] = sdui.Page{ID: "home", Kind: "home", Title: "Home", Sections: []sdui.Section{{ID: "row", Component: "Shelff"}}}
		return nil
	})
	if !errors.As(err, &ie) || !strings.HasPrefix(ie.Issues[0].Path, "/pages/home/sections/0") || ie.Issues[0].Fix == "" {
		t.Errorf("invalid page: %v", err)
	}
	_, err = s.Update("", "owner", func(c *Config) error {
		c.Pages["home"] = sdui.Page{ID: "elsewhere", Kind: "home", Title: "Home"}
		return nil
	})
	if !errors.As(err, &ie) {
		t.Errorf("a page stored under another name: %v", err)
	}
	// A menu entry for a page the file doesn't hold may be for a module's page, which the
	// store can't know: it is a warning, never a reason to refuse or set the file aside.
	// (The editor's route refuses one for a page that isn't there at all.)
	nav := &sdui.Navigation{Items: []sdui.NavItem{{ID: "x", Type: "page", Page: "nowhere"}, {ID: "settings", Type: "native", Screen: "settings"}}}
	warned := false
	for _, is := range s.Check(&Config{Version: FormatVersion, Navigation: nav, Pages: map[string]sdui.Page{}}) {
		warned = warned || (is.Path == "/navigation/items/0/page" && is.Severity == "warning")
		if is.Severity == "error" {
			t.Errorf("an entry for a page that may be a module's is an error: %+v", is)
		}
	}
	if !warned {
		t.Error("no warning for an entry whose page isn't in the file")
	}
	if s.Current().Revision != 2 {
		t.Errorf("a refused save changed the revision: %d", s.Current().Revision)
	}
	// The shared current configuration isn't changed by an Update's function.
	_, _ = s.Update("", "x", func(c *Config) error {
		c.Theme.Tokens["accent"] = "#00ff00"
		return errors.New("changed my mind")
	})
	if s.Current().Theme.Tokens["accent"] != "#ff0000" {
		t.Error("an abandoned update leaked into the current configuration")
	}
}

func TestHistoryKeepsTenAndRollback(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	for i := range 14 {
		if _, err := s.Update("", "owner", func(c *Config) error {
			c.Theme.Tokens = map[string]string{"accent": fmt.Sprintf("#0000%02x", i)}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	h, err := s.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != MaxHistory || h[0].Revision != 15 || !h[0].Current || h[len(h)-1].Revision != 6 {
		t.Fatalf("history: %d entries, newest %d, oldest %d", len(h), h[0].Revision, h[len(h)-1].Revision)
	}
	if _, err := s.Rollback(2, "", "owner"); !errors.Is(err, ErrNoRevision) {
		t.Errorf("pruned revision: %v", err)
	}
	if _, err := s.Rollback(10, "W/\"ui-1-x\"", "owner"); !errors.Is(err, ErrConflict) {
		t.Errorf("rollback with a stale If-Match: %v", err)
	}
	c, err := s.Rollback(10, s.Current().ETag, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if c.Revision != 16 || c.Theme.Tokens["accent"] != "#000008" {
		t.Errorf("rollback: revision %d accent %s", c.Revision, c.Theme.Tokens["accent"])
	}
}

func TestCorruptFileFallsBackToLastGoodRevision(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	if _, err := s.Update("", "owner", func(c *Config) error {
		c.Theme.Tokens = map[string]string{"accent": "#123456"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s2 := open(t, dir)
	c := s2.Current()
	if c.Revision != 2 || c.Theme.Tokens["accent"] != "#123456" {
		t.Errorf("recovered revision %d accent %q", c.Revision, c.Theme.Tokens["accent"])
	}
	if b, err := os.ReadFile(filepath.Join(dir, FileName+".bad")); err != nil || string(b) != "{not json" {
		t.Errorf("the bad file wasn't kept: %v %q", err, b)
	}
	if _, err := Parse(mustRead(t, filepath.Join(dir, FileName))); err != nil {
		t.Errorf("the file wasn't repaired: %v", err)
	}
}

func TestCorruptFileAndHistoryFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	open(t, dir)
	_ = os.WriteFile(filepath.Join(dir, FileName), []byte(`{"version":2,"pages":{"home":{"id":"home","kind":"nonsense","title":"x"}}}`), 0o644)
	ents, _ := os.ReadDir(filepath.Join(dir, HistoryDir))
	for _, e := range ents {
		_ = os.WriteFile(filepath.Join(dir, HistoryDir, e.Name()), []byte("garbage"), 0o644)
	}
	c := open(t, dir).Current()
	if len(c.Pages) != 0 || c.Revision < 2 || c.Version != FormatVersion {
		t.Errorf("defaults: %d pages, revision %d", len(c.Pages), c.Revision)
	}
}

func TestEditOnDiskIsPickedUp(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	c := s.Current().Clone()
	c.Theme.Tokens = map[string]string{"bg": "#101010"}
	c.ETag = ""
	b, err := encode(c)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, FileName)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(p, later, later)
	got := s.Current()
	if got.Theme.Tokens["bg"] != "#101010" || got.Revision != 2 || got.UpdatedBy != "file on disk" {
		t.Errorf("edit on disk: revision %d by %q tokens %v", got.Revision, got.UpdatedBy, got.Theme.Tokens)
	}
	// A broken edit leaves the last good configuration in force.
	_ = os.WriteFile(p, []byte("]"), 0o644)
	later = later.Add(2 * time.Second)
	_ = os.Chtimes(p, later, later)
	if got := s.Current(); got.Revision != 2 || got.Theme.Tokens["bg"] != "#101010" {
		t.Errorf("after a broken edit: revision %d", got.Revision)
	}
	if _, err := Parse(mustRead(t, p)); err != nil {
		t.Errorf("the broken edit wasn't replaced: %v", err)
	}
}

func TestConcurrentUpdatesAndReads(t *testing.T) {
	s := open(t, t.TempDir())
	var wg sync.WaitGroup
	const writers, each = 8, 5
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range each {
				if _, err := s.Update("", "w", func(c *Config) error {
					if c.Theme.Modules == nil {
						c.Theme.Modules = map[string]map[string]string{}
					}
					c.Theme.Modules[fmt.Sprintf("m%d", w)] = map[string]string{"accent": fmt.Sprintf("#00%02x00", i)}
					return nil
				}); err != nil {
					t.Error(err)
				}
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each * 4 {
				c := s.Current()
				_ = c.ETag
				_ = len(c.Theme.Modules)
				_, _ = s.History()
			}
		}()
	}
	wg.Wait()
	c := s.Current()
	if c.Revision != 1+writers*each || len(c.Theme.Modules) != writers {
		t.Errorf("revision %d, %d modules", c.Revision, len(c.Theme.Modules))
	}
	// Concurrent If-Match: exactly one of two writers holding the same tag wins.
	tag := c.ETag
	var ok, conflict int
	var mu sync.Mutex
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Update(tag, "w", func(c *Config) error { c.Theme.Tokens = map[string]string{"fg": "#ffffff"}; return nil })
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				ok++
			} else if errors.Is(err, ErrConflict) {
				conflict++
			}
		}()
	}
	wg.Wait()
	if ok != 1 || conflict != 1 {
		t.Errorf("same If-Match twice: %d won, %d conflicted", ok, conflict)
	}
}

func TestParseIsStrict(t *testing.T) {
	for _, in := range []string{`{"version":2,"pages":{},"extra":1}`, `{"version":2,"pages":{}} {}`, `{"version":2,"pages":{"home":{"id":"home","kind":"home","title":"Home","x":1}}}`} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("accepted %s", in)
		}
	}
}

func TestSameETag(t *testing.T) {
	if !SameETag(`"a", W/"b"`, `W/"b"`) || !SameETag("*", `W/"x"`) || SameETag(`"c"`, `W/"b"`) || SameETag("*", "") {
		t.Error("SameETag")
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A configuration in the first format becomes one in this: stock screens nobody changed
// are dropped for the built-in pages, changed and added ones are converted, the theme is
// kept, and the original file stays beside the new one.
func TestLegacyConfigurationIsConverted(t *testing.T) {
	raw := mustRead(t, filepath.Join("testdata", "legacy_ui_config.json"))
	if !IsLegacy(raw) {
		t.Fatal("the first format wasn't recognised")
	}
	// Unchanged: nothing but the theme survives, with no notes.
	c, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Pages) != 0 || len(c.Notes) != 0 || c.Theme.Tokens["accent"] != "#FF8800" || c.Theme.Modules["wittyplay"]["accent"] != "#3AD29F" {
		t.Errorf("unchanged stock screens: %d pages, notes %q, theme %+v", len(c.Pages), c.Notes, c.Theme)
	}
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}

	// An administrator moved Continue watching to the top of Home, renamed it, and made
	// a page of their own.
	var old map[string]any
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	screens := old["screens"].(map[string]any)
	home := screens["home"].(map[string]any)
	kids := home["root"].(map[string]any)["children"].([]any)
	var row map[string]any
	rest := []any{}
	for _, k := range kids {
		if n := k.(map[string]any); n["id"] == "home-continue" {
			row = n
		} else {
			rest = append(rest, k)
		}
	}
	row["props"].(map[string]any)["title"] = "Keep watching"
	home["root"].(map[string]any)["children"] = append([]any{row}, rest...)
	screens["family"] = map[string]any{"version": 1, "screen": "family", "meta": map[string]any{"title": "Family"},
		"actions": map[string]any{"films": map[string]any{"type": "navigate", "target": "/library/1"}, "site": map[string]any{"type": "external", "target": "https://example.org"}},
		"root": map[string]any{"type": "Screen", "id": "family", "children": []any{
			map[string]any{"type": "Text", "id": "t", "props": map[string]any{"text": "Friday night", "variant": "heading"}, "style": map[string]any{"foreground": "#ff0000"}},
			map[string]any{"type": "Button", "id": "b", "props": map[string]any{"label": "Films"}, "action": "films", "visibility": map[string]any{"role": "kids"}},
			map[string]any{"type": "Button", "id": "b2", "props": map[string]any{"label": "Site"}, "action": "site", "visibility": map[string]any{"platform": "tv"}},
			map[string]any{"type": "Video", "id": "v", "props": map[string]any{"src": "/api/x"}},
		}}}
	changed, _ := json.Marshal(old)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), changed, 0o644); err != nil {
		t.Fatal(err)
	}
	s := open(t, dir)
	got := s.Current()
	if got.Version != FormatVersion || got.Revision != 4 || got.UpdatedBy != "converted from the earlier format" {
		t.Errorf("converted: version %d revision %d by %q", got.Version, got.Revision, got.UpdatedBy)
	}
	if string(mustRead(t, filepath.Join(dir, LegacyBackup))) != string(changed) {
		t.Error("the original file wasn't kept as it was")
	}
	if _, err := Parse(mustRead(t, filepath.Join(dir, FileName))); err != nil || IsLegacy(mustRead(t, filepath.Join(dir, FileName))) {
		t.Errorf("the file wasn't rewritten in the new format: %v", err)
	}
	h, ok := got.Pages["home"]
	if !ok || len(got.Pages) != 2 {
		t.Fatalf("pages %v: want the changed home and family, and nothing for the untouched screens", sortedKeys(got.Pages))
	}
	first := h.Sections[0]
	if first.Component != "Shelf" || first.Props["title"] != "Keep watching" || first.Source.Type() != "continueWatching" || first.Props["cardStyle"] != "landscape" {
		t.Errorf("home's first section: %+v", first)
	}
	var comps []string
	for _, sec := range h.Sections {
		comps = append(comps, sec.Component+":"+sec.Source.Type())
	}
	joined := strings.Join(comps, " ")
	for _, want := range []string{"Hero:featured", "Shelf:nextUp", "Shelf:recentlyAdded", "Shelf:recommended", "Shelf:liveNow", "Stack:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("home lost %s: %s", want, joined)
		}
	}
	for _, sec := range h.Sections {
		if sec.ID == "home-libraries" && (sec.Repeat == nil || sec.Repeat.Over != "libraries" || sec.Source["library"] != "$each") {
			t.Errorf("the row per library: %+v", sec)
		}
		if sec.ID == "home-admin-setup" && (sec.Props["native"] != "settings" || sec.When == nil || len(sec.When.All) != 2) {
			t.Errorf("the settings button and its condition: %+v %+v", sec.Props, sec.When)
		}
	}
	f := got.Pages["family"]
	if f.Kind != "custom" || f.Title != "Family" || len(f.Sections) != 3 {
		t.Fatalf("family: %+v", f)
	}
	if f.Sections[0].Props["role"] != "heading" || f.Sections[1].Props["to"] != "/library/1" || f.Sections[1].When.ProfileKind[0] != "kids" ||
		f.Sections[2].Props["url"] != "https://example.org" || f.Sections[2].When.FormFactor[0] != "tv" {
		t.Errorf("family's sections: %+v", f.Sections)
	}
	if !strings.Contains(f.Description, "Video") || !strings.Contains(f.Description, "colours and sizes") {
		t.Errorf("what was dropped isn't named: %q", f.Description)
	}
	if len(got.Notes) < 3 {
		t.Errorf("notes: %q", got.Notes)
	}
	// Opened again, nothing is converted twice and the backup isn't overwritten.
	again := open(t, dir).Current()
	if again.Revision != got.Revision || again.ETag != got.ETag {
		t.Errorf("reopened: revision %d, was %d", again.Revision, got.Revision)
	}
	// Every page of the result is one the renderer accepts.
	for id, p := range got.Pages {
		if issues := sdui.ValidatePage(&p); sdui.HasErrors(issues) {
			t.Errorf("%s: %v", id, issues)
		}
	}
}

func TestDraftIsSeparateUntilPublished(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	if s.Draft() != nil {
		t.Fatal("a draft out of nowhere")
	}
	c := s.Current().Clone()
	c.Pages["night"] = sdui.Page{ID: "night", Kind: "custom", Title: "Film night", Sections: []sdui.Section{{ID: "hello", Component: "Text", Props: map[string]any{"text": "Hello"}}}}
	if _, err := s.SaveDraft(c, "nonsense", "owner"); !errors.Is(err, ErrConflict) {
		t.Errorf("a first draft against a configuration that has moved on: %v", err)
	}
	d, err := s.SaveDraft(c, s.Current().ETag, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Current().Pages) != 0 || s.Current().Revision != 1 {
		t.Error("saving a draft changed what is published")
	}
	bad := c.Clone()
	bad.Pages["night"] = sdui.Page{ID: "night", Kind: "custom", Title: ""}
	var ie *InvalidError
	if _, err := s.SaveDraft(bad, "", "owner"); !errors.As(err, &ie) {
		t.Errorf("an invalid draft: %v", err)
	}
	// It survives a restart.
	s2 := open(t, dir)
	if got := s2.Draft(); got == nil || got.ETag != d.ETag || got.Pages["night"].Title != "Film night" {
		t.Fatalf("the draft after a restart: %+v", got)
	}
	if _, err := s2.Publish("stale", "owner"); !errors.Is(err, ErrConflict) {
		t.Errorf("publishing a draft that changed: %v", err)
	}
	pub, err := s2.Publish(d.ETag, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if pub.Revision != 2 || pub.Pages["night"].Title != "Film night" || s2.Draft() != nil {
		t.Errorf("published: revision %d, draft still there: %v", pub.Revision, s2.Draft() != nil)
	}
	if _, err := os.Stat(filepath.Join(dir, DraftName)); err == nil {
		t.Error("the draft's file is still there")
	}
	if _, err := s2.Publish("", "owner"); !errors.Is(err, ErrNoRevision) {
		t.Errorf("publishing with no draft: %v", err)
	}
	if _, err := s2.SaveDraft(pub.Clone(), "", "owner"); err != nil {
		t.Fatal(err)
	}
	if err := s2.DiscardDraft(); err != nil || s2.Draft() != nil {
		t.Errorf("discard: %v", err)
	}
	if rc, err := s2.RevisionConfig(1); err != nil || len(rc.Pages) != 0 {
		t.Errorf("revision 1: %v", err)
	}
}

// What a migration can't read must not stop the server.
func FuzzParse(f *testing.F) {
	f.Add(mustReadF(f, filepath.Join("testdata", "legacy_ui_config.json")))
	f.Add([]byte(`{"version":2,"pages":{"home":{"id":"home","kind":"home","title":"Home","sections":[{"id":"a","component":"Shelf","source":{"type":"nextUp"}}]}}}`))
	f.Add([]byte(`{"version":1,"screens":{"x":{"root":{"type":"Stack","id":"r","children":[{"type":"PosterRow","id":"p","bindings":{"items":{"path":"$.home.nextUp"}}}]}}}}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		c, err := Parse(b)
		if err != nil {
			return
		}
		if IsLegacy(b) {
			// Whatever a conversion produces is valid, or Open would set the file aside.
			if err := Validate(c); err != nil {
				var ie *InvalidError
				if errors.As(err, &ie) {
					for _, is := range ie.Issues {
						if strings.HasPrefix(is.Path, "/pages/") {
							t.Fatalf("a converted page doesn't validate: %v", is)
						}
					}
				}
			}
		}
		_ = c.Clone()
	})
}

func mustReadF(f *testing.F, p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		f.Fatal(err)
	}
	return b
}
