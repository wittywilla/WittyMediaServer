// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymodule"
)

// fakeScans stands in for a module that scans games libraries: it keeps every call, and answers
// the last identify call and each describe call with what the test says.
type fakeScans struct {
	mu       sync.Mutex
	on       bool
	identify []wittymodule.ScanIdentifyArgs
	describe []wittymodule.ScanDescribeArgs
	// done answers the last identify call, given every file the scan sent.
	done func(files []wittymodule.ScanFile) wittymodule.ScanIdentified
	// described answers a describe call.
	described func(a wittymodule.ScanDescribeArgs) wittymodule.ScanDescribed
	// onIdentify runs at each identify call (a test stops the scan there).
	onIdentify func(n int)
}

func (f *fakeScans) ScanModule(kind string) (ScanModule, bool) {
	if kind != "games" {
		return ScanModule{}, false
	}
	return ScanModule{ID: "fake", Name: "Fake Play", Version: "1.0.0", On: f.on, Describe: true, Facet: "console"}, true
}

func (f *fakeScans) Call(ctx context.Context, module, call string, args, out any) error {
	if module != "fake" {
		return errors.New("no such module")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Through JSON, as the service's pipe does.
	b, _ := json.Marshal(args)
	var answer any
	switch call {
	case wittymodule.CallScanIdentify:
		var a wittymodule.ScanIdentifyArgs
		_ = json.Unmarshal(b, &a)
		f.mu.Lock()
		f.identify = append(f.identify, a)
		n := len(f.identify)
		var all []wittymodule.ScanFile
		for _, c := range f.identify {
			if c.Scan == a.Scan {
				all = append(all, c.Files...)
			}
		}
		f.mu.Unlock()
		if f.onIdentify != nil {
			f.onIdentify(n)
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		answer = struct{}{}
		if a.Done && f.done != nil {
			answer = f.done(all)
		}
	case wittymodule.CallScanDescribe:
		var a wittymodule.ScanDescribeArgs
		_ = json.Unmarshal(b, &a)
		f.mu.Lock()
		f.describe = append(f.describe, a)
		f.mu.Unlock()
		answer = wittymodule.ScanDescribed{}
		if f.described != nil {
			answer = f.described(a)
		}
	default:
		return errors.New("no such call")
	}
	b, _ = json.Marshal(answer)
	return json.Unmarshal(b, out)
}

type hookFixture struct {
	s    *Scanner
	mod  *fakeScans
	lib  *Library
	root string
	ctx  context.Context
}

func newHookFixture(t *testing.T, roots int) *hookFixture {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	st := &Store{DB: d}
	var paths []string
	for range roots {
		paths = append(paths, t.TempDir())
	}
	lib, err := st.CreateLibrary(context.Background(), "Games", "games", paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetLibraryConsole(context.Background(), lib.ID, "auto"); err != nil {
		t.Fatal(err)
	}
	s := NewScanner(st, &ffmpeg.Tools{Encoders: map[string]bool{}}, t.TempDir(), func() string { return "" }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.ImagesDir = t.TempDir()
	s.slot = nil
	mod := &fakeScans{on: true}
	s.Modules = mod
	lib, _ = st.Library(context.Background(), lib.ID)
	return &hookFixture{s: s, mod: mod, lib: lib, root: paths[0], ctx: context.Background()}
}

func (f *hookFixture) write(t *testing.T, rel string) string {
	t.Helper()
	p := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(rel), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *hookFixture) itemAt(t *testing.T, p string) *Item {
	t.Helper()
	it, err := f.s.Store.itemByPath(f.ctx, f.lib.ID, p)
	if err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return it
}

// The walk's files go to identify in batches of at most 256, hidden ones left out, then one
// call with Done; what that answers is kept: a game on two discs as one item with both files,
// a version under the game it's a version of, and nothing outside the library's folders.
func TestModuleScanKeepsWhatTheModuleSays(t *testing.T) {
	f := newHookFixture(t, 1)
	var roms []string
	for i := range 600 {
		roms = append(roms, f.write(t, fmt.Sprintf("NES/game%03d.nes", i)))
	}
	disc1, disc2 := f.write(t, "PS1/Quest (Disc 1).chd"), f.write(t, "PS1/Quest (Disc 2).chd")
	f.write(t, ".hidden/secret.nes")
	f.write(t, "$RECYCLE.BIN/old.nes")
	f.write(t, "NES/.DS_Store")
	f.mod.done = func(files []wittymodule.ScanFile) wittymodule.ScanIdentified {
		return wittymodule.ScanIdentified{Problems: 1, Items: []wittymodule.ScanItem{
			{Files: []string{disc1, disc2}, Kind: "game", Title: "Quest", Year: 1999, Container: "chd", Size: 10, MTime: 20, Facets: map[string]string{"console": "ps1"}},
			{Files: []string{roms[0]}, Kind: "game", Title: "Game Zero", Tagline: "USA", Facets: map[string]string{"console": "nes"}},
			{Files: []string{roms[1]}, Kind: "game", Title: "Game Zero", Tagline: "Europe", Facets: map[string]string{"console": "nes"}, VersionOf: roms[0], VersionLabel: "Europe"},
			{Files: []string{"/etc/passwd"}, Kind: "game", Title: "Not ours"},
			{Files: []string{roms[2]}, Kind: "Not A Kind!", Title: "Bad kind"},
		}}
	}
	if err := f.s.scan(f.ctx, f.lib.ID); err != nil {
		t.Fatal(err)
	}
	calls := f.mod.identify
	if len(calls) != 4 { // 602 files: 256 + 256 + 90, then Done
		t.Fatalf("%d identify calls", len(calls))
	}
	sent := 0
	for i, c := range calls {
		if c.Scan != calls[0].Scan || c.Library.ID != f.lib.ID || c.Library.Kind != "games" || c.Library.Facet != "auto" || c.Total != 602 {
			t.Errorf("call %d: %+v", i, c)
		}
		if len(c.Files) > identifyBatch || c.Done != (i == len(calls)-1) || (c.Done && len(c.Files) != 0) {
			t.Errorf("call %d: %d files, done %v", i, len(c.Files), c.Done)
		}
		for _, file := range c.Files {
			if strings.Contains(file.Path, "hidden") || strings.Contains(file.Path, "RECYCLE") || strings.Contains(file.Path, "DS_Store") || file.Root != f.root || file.Size == 0 {
				t.Errorf("sent %+v", file)
			}
		}
		sent += len(c.Files)
	}
	if sent != 602 {
		t.Errorf("sent %d files", sent)
	}
	quest := f.itemAt(t, disc1)
	if quest.Kind != "game" || quest.Title != "Quest" || quest.Console != "ps1" || quest.Year == nil || *quest.Year != 1999 || quest.Container != "chd" {
		t.Errorf("%+v", quest)
	}
	if files, _ := f.s.Store.ItemFiles(f.ctx, quest.ID); !slices.Equal(files, []string{disc1, disc2}) {
		t.Errorf("files %v", files)
	}
	if _, err := f.s.Store.itemByPath(f.ctx, f.lib.ID, disc2); !errors.Is(err, ErrNotFound) {
		t.Errorf("the second disc is an item of its own: %v", err)
	}
	zero, europe := f.itemAt(t, roms[0]), f.itemAt(t, roms[1])
	link := f.s.Store.LinkOf(f.ctx, europe.ID)
	if link == nil || link.Link != LinkVersion || link.ParentItem == nil || *link.ParentItem != zero.ID || link.Label != "Europe" {
		t.Errorf("version link %+v", link)
	}
	if _, err := f.s.Store.itemByPath(f.ctx, f.lib.ID, "/etc/passwd"); err == nil {
		t.Error("kept an item outside the library")
	}
	if _, err := f.s.Store.itemByPath(f.ctx, f.lib.ID, roms[2]); err == nil {
		t.Error("kept an item of a kind that isn't one")
	}
	// The new ones are described, with their files and facets.
	if len(f.mod.describe) != 1 || len(f.mod.describe[0].Items) != 3 {
		t.Fatalf("describe calls %+v", f.mod.describe)
	}
	for _, d := range f.mod.describe[0].Items {
		if d.ID == quest.ID && (!slices.Equal(d.Files, []string{disc1, disc2}) || d.Facets["console"] != "ps1") {
			t.Errorf("%+v", d)
		}
	}

	// Scanned again with nothing changed, nothing is described again.
	f.mod.describe = nil
	if err := f.s.scan(f.ctx, f.lib.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.mod.describe) != 0 {
		t.Errorf("described again: %+v", f.mod.describe)
	}
}

// What the module no longer lists is missing, except what was under a folder that couldn't be
// reached this time; a stopped scan sends no Done and changes nothing.
func TestModuleScanMissingOfflineAndStopped(t *testing.T) {
	f := newHookFixture(t, 2)
	away := f.lib.Paths[1]
	a, b := f.write(t, "a.nes"), f.write(t, "b.nes")
	far := filepath.Join(away, "far.nes")
	if err := os.WriteFile(far, []byte("far"), 0o644); err != nil {
		t.Fatal(err)
	}
	list := []string{a, b, far}
	f.mod.done = func([]wittymodule.ScanFile) wittymodule.ScanIdentified {
		var out []wittymodule.ScanItem
		for _, p := range list {
			out = append(out, wittymodule.ScanItem{Files: []string{p}, Kind: "game", Title: filepath.Base(p)})
		}
		return wittymodule.ScanIdentified{Items: out}
	}
	if err := f.s.scan(f.ctx, f.lib.ID); err != nil {
		t.Fatal(err)
	}
	// b is gone from what the module lists, and the second folder is away (empty).
	if err := os.Remove(far); err != nil {
		t.Fatal(err)
	}
	list = []string{a}
	if err := f.s.scan(f.ctx, f.lib.ID); err != nil {
		t.Fatal(err)
	}
	last := f.mod.identify[len(f.mod.identify)-1]
	if !last.Done || !slices.Equal(last.Offline, []string{away}) {
		t.Errorf("offline %v", last.Offline)
	}
	if f.itemAt(t, a).Missing || !f.itemAt(t, b).Missing || f.itemAt(t, far).Missing {
		t.Errorf("missing: a %v b %v far %v", f.itemAt(t, a).Missing, f.itemAt(t, b).Missing, f.itemAt(t, far).Missing)
	}

	// Stopped at its second batch: no Done, and nothing kept from it.
	for i := range 300 {
		f.write(t, fmt.Sprintf("more/%03d.nes", i))
	}
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	f.mod.onIdentify = func(n int) {
		if n == 2 { // 302 files: the second of two batches
			cancel()
		}
	}
	f.mod.identify = nil
	list = nil
	if err := f.s.scan(ctx, f.lib.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("a stopped scan: %v", err)
	}
	for _, c := range f.mod.identify {
		if c.Done {
			t.Error("a stopped scan sent Done")
		}
	}
	if f.itemAt(t, a).Missing {
		t.Error("a stopped scan changed what the library holds")
	}
}

// Without its module the library's items are kept as they are, and nothing is asked.
func TestModuleScanWithoutTheModuleKeepsItems(t *testing.T) {
	f := newHookFixture(t, 1)
	a := f.write(t, "a.nes")
	f.mod.done = func([]wittymodule.ScanFile) wittymodule.ScanIdentified {
		return wittymodule.ScanIdentified{Items: []wittymodule.ScanItem{{Files: []string{a}, Kind: "game", Title: "A"}}}
	}
	if err := f.s.scan(f.ctx, f.lib.ID); err != nil {
		t.Fatal(err)
	}
	f.mod.on = false
	f.mod.identify = nil
	if err := os.Remove(a); err != nil {
		t.Fatal(err)
	}
	if err := f.s.scan(f.ctx, f.lib.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.mod.identify) != 0 || f.itemAt(t, a).Missing {
		t.Errorf("calls %d, missing %v", len(f.mod.identify), f.itemAt(t, a).Missing)
	}
	f.s.Modules = nil // no modules at all: a games library is no less kept
	if err := f.s.scan(f.ctx, f.lib.ID); err != nil || f.itemAt(t, a).Missing {
		t.Errorf("%v %v", err, f.itemAt(t, a).Missing)
	}
}

// What a module describes is applied, except a field a person typed; a picture must be one the
// module kept through the host API, and becomes the item's own copy in the image cache.
func TestDescribeKeepsWhatAPersonTyped(t *testing.T) {
	f := newHookFixture(t, 1)
	a := f.write(t, "a.nes")
	f.mod.done = func([]wittymodule.ScanFile) wittymodule.ScanIdentified {
		return wittymodule.ScanIdentified{Items: []wittymodule.ScanItem{{Files: []string{a}, Kind: "game", Title: "a"}}}
	}
	if err := os.MkdirAll(filepath.Join(f.s.ImagesDir, "modules", "fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.s.ImagesDir, "modules", "fake", "0123abcd.png"), pngHead, 0o644); err != nil {
		t.Fatal(err)
	}
	rating := 8.5
	f.mod.described = func(d wittymodule.ScanDescribeArgs) wittymodule.ScanDescribed {
		var out wittymodule.ScanDescribed
		for _, it := range d.Items {
			out.Items = append(out.Items, wittymodule.ItemFields{ID: it.ID, Title: "Module Title", Overview: "What it is.", Year: 1991,
				Genres: []string{"Platform"}, Rating: &rating, ContentRating: "ESRB T", Poster: "/api/module-images/fake/0123abcd.png",
				Titles: &wittymodule.ItemTitles{Native: "モジュール"}})
		}
		out.Items = append(out.Items, wittymodule.ItemFields{ID: 999999, Title: "Not asked about"})
		return out
	}
	if err := f.s.scan(f.ctx, f.lib.ID); err != nil {
		t.Fatal(err)
	}
	it := f.itemAt(t, a)
	if it.Title != "Module Title" || it.Overview != "What it is." || it.Rating == nil || *it.Rating != 8.5 || it.Poster != "cache:mod-fake-0123abcd.png" || it.Titles == nil || it.Titles.Native != "モジュール" {
		t.Fatalf("%+v %+v", it, it.Titles)
	}
	if got := AgeFor(it.ContentRating); got == nil || *got != 13 {
		t.Errorf("an ESRB T is for 13 and up: %v", got)
	}
	if _, err := os.Stat(filepath.Join(f.s.ImagesDir, "mod-fake-0123abcd.png")); err != nil {
		t.Error(err)
	}

	// A person types a title and an overview; Refresh asks the module again, which keeps them.
	if err := f.s.Store.SetItemEdited(f.ctx, it.ID, "title", "overview"); err != nil {
		t.Fatal(err)
	}
	it.Title, it.Overview = "Typed", "Typed too."
	if err := f.s.Store.UpdateItem(f.ctx, it); err != nil {
		t.Fatal(err)
	}
	f.mod.describe = nil
	if err := f.s.DescribeItems(f.ctx, f.lib.ID, []int64{it.ID}, true); err != nil {
		t.Fatal(err)
	}
	if len(f.mod.describe) != 1 || !f.mod.describe[0].Refresh || !slices.Equal(f.mod.describe[0].Items[0].Edited, []string{"overview", "title"}) {
		t.Errorf("%+v", f.mod.describe)
	}
	if it = f.itemAt(t, a); it.Title != "Typed" || it.Overview != "Typed too." || it.Year == nil || *it.Year != 1991 {
		t.Errorf("%q %q %v", it.Title, it.Overview, it.Year)
	}

	// Another module's picture, or a file of the server's, isn't one this module may name.
	for _, bad := range []string{"/api/module-images/other/0123abcd.png", "/api/module-images/fake/../../wms.db", "cache:anything.png", "/etc/passwd"} {
		if err := f.s.ApplyItemFields(f.ctx, "fake", wittymodule.ItemFields{ID: it.ID, Poster: bad}); !errors.Is(err, ErrModuleArt) {
			t.Errorf("%s: %v", bad, err)
		}
	}
	// Clear empties what it names, unless it was typed.
	if err := f.s.ApplyItemFields(f.ctx, "fake", wittymodule.ItemFields{ID: it.ID, Clear: []string{"overview", "genres", "poster"}}); err != nil {
		t.Fatal(err)
	}
	if it = f.itemAt(t, a); it.Overview != "Typed too." || len(it.Genres) != 0 || it.Poster != "" {
		t.Errorf("%q %v %q", it.Overview, it.Genres, it.Poster)
	}
}
