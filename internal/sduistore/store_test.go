// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sduistore

import (
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

// Whatever is read must not stop the server.
func FuzzParse(f *testing.F) {
	f.Add([]byte(`{"version":2,"pages":{"home":{"id":"home","kind":"home","title":"Home","sections":[{"id":"a","component":"Shelf","source":{"type":"nextUp"}}]}}}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		c, err := Parse(b)
		if err != nil {
			return
		}
		_ = c.Clone()
	})
}
