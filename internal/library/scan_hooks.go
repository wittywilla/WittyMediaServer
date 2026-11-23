// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
	"github.com/wittywilla/wittymodule"
)

// Scan hooks (module API 5, docs/MODULES-API5.md#scan-hooks). A module that owns a kind of
// library (WittyPlay: games) says so in its integration.json's scan part. The server still
// walks the library's folders, keeps the items and checks who may see them; the module says
// what the files are (scan.identify) and what's known about each item (scan.describe). What
// the module learns beyond the generic fields it keeps in its own database.

// ScanModule is the module that scans a kind of library.
type ScanModule struct {
	ID   string
	Name string
	// Version is the version in use, for whoever keeps what it said (scan.choices).
	Version string
	// On: installed and turned on. A module that isn't can't be asked, and the library's
	// items are kept as they are until it is.
	On bool
	// Describe: it answers scan.describe.
	Describe bool
	// Facet is the facet a library of the kind can be held to ("console"), "" for none.
	Facet string
}

// ScanModules is how a scan reaches the modules that scan kinds of library. The server's
// module manager answers it; tests stand in for it.
type ScanModules interface {
	// ScanModule is the module whose integration.json says it scans this kind of library, and
	// whether there is one.
	ScanModule(kind string) (ScanModule, bool)
	// Call asks a module's service one of the scan calls (wittymodule.CallScan…). It ends when
	// ctx does.
	Call(ctx context.Context, module, call string, args, out any) error
}

// ownScans are the kinds of library the server's own walks know what to make of.
var ownScans = map[string]bool{"movies": true, "discs": true, "shows": true, "anime": true, "videos": true, "commercials": true,
	"music": true, "audiobooks": true, "podcasts": true, "books": true, "photos": true, "shorts": true, "channels": true,
	"musicvideos": true, "radio": true}

// OwnScan says whether the server scans a kind of library by itself, without a module.
func OwnScan(kind string) bool { return ownScans[kind] }

// scanModule is the module that scans a library of this kind, if one does. A kind the server
// has no walk of its own for (games) always has one, which may be missing (On false): its
// items are then kept as they are.
func (s *Scanner) scanModule(kind string) (ScanModule, bool) {
	var m ScanModule
	ok := false
	if s.Modules != nil {
		m, ok = s.Modules.ScanModule(kind)
	}
	if ok && m.On {
		return m, true
	}
	if OwnScan(kind) {
		return ScanModule{}, false
	}
	if m.Name == "" {
		m.Name = "The module that scans " + kind + " libraries"
	}
	m.On = false
	return m, true
}

// The scan hooks' batches.
const (
	identifyBatch = 256
	describeBatch = 50
)

// skippedName: hidden files and folders, and the ones systems keep for themselves ($RECYCLE.BIN).
func skippedName(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$")
}

// walkForModule lists every regular file under a root, in walk order, skipping hidden names.
// Symbolic links are never followed. It stops when ctx ends.
func walkForModule(ctx context.Context, root string) ([]wittymodule.ScanFile, error) {
	var out []wittymodule.ScanFile
	var bad unreadable
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			bad.note(err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if p != root && skippedName(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			bad.note(err)
			return nil
		}
		out = append(out, wittymodule.ScanFile{Path: p, Root: root, Size: info.Size(), MTime: info.ModTime().Unix()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, bad.with(nil)
}

// scanLibraryOf is a library as the scan calls name it.
func scanLibraryOf(lib *Library, mod ScanModule) wittymodule.ScanLibrary {
	out := wittymodule.ScanLibrary{ID: lib.ID, Kind: lib.Type, Paths: lib.Paths}
	if mod.Facet != "" {
		out.Facet = lib.Console
		if out.Facet == "" {
			out.Facet = wittymodule.ScanAuto
		}
	}
	if out.Paths == nil {
		out.Paths = []string{}
	}
	return out
}

// scanByModule scans a library through its module's hooks: the walk's files in batches to
// scan.identify, the items the last call answers kept, and the new or changed ones described.
func (s *Scanner) scanByModule(ctx context.Context, lib *Library, mod ScanModule, pausePoint func() error, tally *scanTally, started time.Time) error {
	task := scanTask(ctx)
	live, run := liveOf(ctx), runOf(ctx)
	steps := []tasks.Step{{Name: phaseFinding, Share: 1}, {Name: phaseWorkingOut, Share: 4}, {Name: phaseGrouping, Share: 1}, {Name: phaseListing, Share: 2}, {Name: phaseTidying, Share: 1}}
	if mod.Describe {
		steps = append(steps, tasks.Step{Name: phaseDescribing, Share: 2})
	}
	task.Steps(steps...)
	task.Phase(phaseFinding)
	var files []wittymodule.ScanFile
	var offline []string
	for _, root := range lib.Paths {
		if err := pausePoint(); err != nil {
			return err
		}
		s.progress(lib.ID, func(p *ScanProgress) { p.Phase = "discovering"; p.Current = root })
		task.Item(filepath.Base(root), root)
		// A drive or share that is away (an unmounted mount point reads as an
		// empty folder) is not the same as every file having been deleted.
		if entries, err := os.ReadDir(root); err != nil || len(entries) == 0 {
			s.Log.WarnContext(ctx, "library folder is empty or can't be reached; keeping its items", "library", lib.Name, "dir", root, "err", err)
			task.Problem(root, "This folder is empty or can't be reached, so what it held is being kept for now. If it's on a drive or a share, check that it's connected.")
			offline = append(offline, root)
			continue
		}
		found, err := walkForModule(ctx, root)
		if ctx.Err() != nil {
			return ctx.Err() // before anything half-found is used
		}
		if found == nil && err != nil {
			s.Log.WarnContext(ctx, "could not read a library folder; keeping its items", "library", lib.Name, "dir", root, "err", err)
			words, fix := tasks.Explain(err)
			task.Problem(root, "This folder couldn't be read, so what it held is being kept for now. "+words+" "+fix)
			offline = append(offline, root)
			continue
		}
		if err != nil {
			s.Log.WarnContext(ctx, "could not read all of a library folder", "library", lib.Name, "dir", root, "err", err)
			words, fix := tasks.Explain(err)
			task.Problem(root, "Some of this folder couldn't be read. "+words+" "+fix)
		}
		files = append(files, found...)
	}

	// What each file is: the module's to say.
	task.Phase(phaseWorkingOut)
	task.Unit("files")
	scanLib := scanLibraryOf(lib, mod)
	scanID := strconv.FormatInt(lib.ID, 10) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	s.progress(lib.ID, func(p *ScanProgress) { p.Phase = "indexing"; p.Found = len(files) })
	// What the module reports of its own progress inside a call (the host API's task
	// scan-<library>) moves the bar, in files, with what it's working on now when it says.
	moduleSays := func(done, total int64, detail string) {
		task.Progress(done, total)
		if detail != "" {
			task.Item(detail, "")
		}
		s.progress(lib.ID, func(p *ScanProgress) {
			p.Done, p.Total, p.Detail = int(min(done, total)), int(total), detail
		})
	}
	for i := 0; i < len(files); i += identifyBatch {
		if err := pausePoint(); err != nil {
			return err
		}
		batch := files[i:min(i+identifyBatch, len(files))]
		s.progress(lib.ID, func(p *ScanProgress) { p.Current = filepath.Base(batch[0].Path); p.Done = i; p.Total = len(files) })
		task.Item(filepath.Base(batch[0].Path), batch[0].Path)
		task.Progress(int64(i), int64(len(files)))
		var none struct{}
		args := wittymodule.ScanIdentifyArgs{Scan: scanID, Library: scanLib, Files: batch, Total: len(files), Offline: offline}
		called := live.calling(mod.ID, mod.Name+" was working out what "+filepath.Base(batch[0].Path)+" and the files after it are", moduleSays)
		err := s.Modules.Call(ctx, mod.ID, wittymodule.CallScanIdentify, args, &none)
		called()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("%s couldn't say what the files are: %w", mod.Name, err)
		}
	}
	if err := pausePoint(); err != nil {
		return err
	}
	// The last call puts the files together into items: on a big library, a while.
	task.Phase(phaseGrouping)
	task.Item("", "")
	task.Progress(0, int64(len(files)))
	s.progress(lib.ID, func(p *ScanProgress) {
		p.Phase, p.Current, p.Done, p.Total, p.Detail = "grouping", "", 0, len(files), ""
	})
	var done wittymodule.ScanIdentified
	args := wittymodule.ScanIdentifyArgs{Scan: scanID, Library: scanLib, Total: len(files), Done: true, Offline: offline}
	called := live.calling(mod.ID, mod.Name+" was putting what it found together", moduleSays)
	err := s.Modules.Call(ctx, mod.ID, wittymodule.CallScanIdentify, args, &done)
	called()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%s couldn't say what the files are: %w", mod.Name, err)
	}
	task.Progress(int64(len(files)), int64(len(files)))

	// Keeping what it found.
	task.Phase(phaseListing)
	task.Item("", "")
	s.progress(lib.ID, func(p *ScanProgress) { p.Phase, p.Done, p.Total, p.Detail = "listing", 0, 0, "" })
	changed, kept, refused, err := s.Store.applyScanned(ctx, lib, mod, done.Items)
	if err != nil {
		return err
	}
	if refused > 0 {
		s.Log.WarnContext(ctx, "a module named items the scan can't keep", "library", lib.Name, "module", mod.ID, "count", refused)
		task.ProblemN("", mod.Name+" named items that aren't files in this library's folders, so they were left out.", refused)
	}
	task.Phase(phaseTidying)
	if err := s.Store.markMissing(ctx, lib.ID, kept, offline); err != nil {
		return err
	}

	// What's known about what's new: listed first, so a restart carries on with what's left,
	// and with every item on a Full rescan, read again from the start.
	if mod.Describe {
		if err := s.Store.queueDescribe(ctx, lib.ID, changed); err != nil {
			return err
		}
		if run.full() && !run.queued {
			if err := s.Store.queueAllDescribe(ctx, lib.ID); err != nil {
				return err
			}
			run.queued = true
		}
		todo, err := s.Store.toDescribe(ctx, lib.ID)
		if err != nil {
			return err
		}
		if len(todo) > 0 {
			task.Phase(phaseDescribing)
			task.Unit("items")
			task.Progress(0, int64(len(todo)))
			s.progress(lib.ID, func(p *ScanProgress) { p.Phase, p.Done, p.Total, p.Detail = "describing", 0, len(todo), "" })
			err := s.describe(ctx, lib, mod, todo, run.full(), func(batch []int64, done, total int) error {
				s.Store.described(ctx, batch)
				task.Progress(int64(done), int64(total))
				s.progress(lib.ID, func(p *ScanProgress) { p.Done, p.Total = done, total })
				return pausePoint()
			})
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil && s.stopping.Load() {
				return err // carried on at the next start
			}
			if err != nil {
				s.minorFailure(ctx, "could not find out about what's new", err)
			}
		}
	}
	s.Store.DB.Optimize(ctx)
	problems := tally.report(ctx, s.Log) + done.Problems + refused
	s.Log.InfoContext(ctx, "scan finished", "library", lib.Name, "files", len(files), "items", len(kept), "module", mod.ID,
		"errors", problems, "offline", len(offline), "took", time.Since(started).Round(time.Second).String())
	if r := strings.TrimSpace(done.Result); r != "" {
		task.Result(r)
	} else {
		task.Result(scanResult(len(files), problems, time.Since(started)))
	}
	return nil
}

// validScanKind: an item kind a module names is a short lower-case word.
func validScanKind(k string) bool {
	if k == "" || len(k) > 32 {
		return false
	}
	for _, c := range k {
		if !(c >= 'a' && c <= 'z' || c == '-') {
			return false
		}
	}
	return true
}

// maxItemFiles is the most files one item can have: a game on many discs.
const maxItemFiles = 64

// applyScanned keeps the items a module's scan found: each by its first file, with its files
// in order, and its versions linked to the item they're versions of. It says which items are
// new or changed (to be described), every path kept, and how many items were refused.
func (s *Store) applyScanned(ctx context.Context, lib *Library, mod ScanModule, items []wittymodule.ScanItem) (changed []int64, kept map[string]bool, refused int, err error) {
	kept = map[string]bool{}
	byPath := map[string]int64{}
	facet := mod.Facet
	if facet == "" {
		facet = "console"
	}
	type version struct {
		id    int64
		of    string
		label string
	}
	var versions []version
	for _, si := range items {
		if ctx.Err() != nil {
			return nil, nil, 0, ctx.Err()
		}
		paths, ok := cleanItemFiles(si.Files, lib.Paths)
		if !ok || !validScanKind(si.Kind) || kept[paths[0]] {
			refused++
			continue
		}
		id, isNew, moved, err := s.keepScanned(ctx, lib, si, paths, facet)
		if err != nil {
			return nil, nil, 0, err
		}
		kept[paths[0]] = true
		byPath[paths[0]] = id
		if isNew || moved {
			changed = append(changed, id)
		}
		if si.VersionOf != "" {
			versions = append(versions, version{id, filepath.Clean(si.VersionOf), si.VersionLabel})
		}
	}
	// Versions: what the module says now replaces what it said before.
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM item_links WHERE link = ? AND item_id IN (SELECT id FROM items WHERE library_id = ?)`, LinkVersion, lib.ID); err != nil {
		return nil, nil, 0, err
	}
	for _, v := range versions {
		parent, ok := byPath[v.of]
		if !ok || parent == v.id {
			continue
		}
		label := strings.TrimSpace(v.label)
		if len(label) > 80 {
			label = label[:80]
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO item_links(item_id, link, extra_type, parent_item, parent_show, label, created_at) VALUES(?,?,'',?,NULL,?,?)
			ON CONFLICT(item_id) DO UPDATE SET link = excluded.link, parent_item = excluded.parent_item, parent_show = NULL, label = excluded.label`,
			v.id, LinkVersion, parent, label, db.Now()); err != nil {
			return nil, nil, 0, err
		}
	}
	return changed, kept, refused, tx.Commit()
}

// cleanItemFiles checks an item's files: some, not too many, and each a path under one of the
// library's folders. A module only ever names files the walk could have found.
func cleanItemFiles(files []string, roots []string) ([]string, bool) {
	if len(files) == 0 || len(files) > maxItemFiles {
		return nil, false
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		if f == "" || !filepath.IsAbs(f) {
			return nil, false
		}
		f = filepath.Clean(f)
		if !underAny(f, roots) || slices.Contains(roots, f) {
			return nil, false
		}
		out = append(out, f)
	}
	return out, true
}

// keepScanned adds or updates one item a module's scan found. A field a person typed stays.
func (s *Store) keepScanned(ctx context.Context, lib *Library, si wittymodule.ScanItem, paths []string, facet string) (id int64, isNew, moved bool, err error) {
	it, err := s.itemByPath(ctx, lib.ID, paths[0])
	switch {
	case errors.Is(err, ErrNotFound):
		it = &Item{LibraryID: lib.ID, Path: paths[0]}
		isNew = true
	case err != nil:
		return 0, false, false, err
	}
	edited := map[string]bool{}
	if !isNew {
		for _, f := range s.ItemEdits(ctx, it.ID) {
			edited[f] = true
		}
		old, _ := s.ItemFiles(ctx, it.ID)
		moved = it.Missing || it.SizeBytes != si.Size || it.MTime != si.MTime || !slices.Equal(old, paths) || it.Kind != si.Kind
	}
	was := struct {
		kind, title, sort, tagline, container, console string
		year                                           int
	}{it.Kind, it.Title, it.SortTitle, it.Tagline, it.Container, it.Console, derefInt(it.Year)}
	it.Kind = si.Kind
	if title := strings.TrimSpace(si.Title); !edited["title"] {
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(paths[0]), filepath.Ext(paths[0]))
		}
		it.Title = clipText(title, 500)
		it.SortTitle = clipText(strings.TrimSpace(si.SortTitle), 500)
		if it.SortTitle == "" {
			it.SortTitle = SortTitle(it.Title)
		}
	}
	if !edited["tagline"] {
		it.Tagline = clipText(si.Tagline, 300)
	}
	if !edited["year"] && si.Year > 0 && si.Year < 3000 {
		y := si.Year
		it.Year = &y
	}
	it.Container = clipText(strings.ToLower(si.Container), 32)
	it.SizeBytes, it.MTime = si.Size, si.MTime
	it.Console = clipText(si.Facets[facet], 64)
	if !isNew && !moved && was.kind == it.Kind && was.title == it.Title && was.sort == it.SortTitle && was.tagline == it.Tagline &&
		was.container == it.Container && was.console == it.Console && was.year == derefInt(it.Year) {
		return it.ID, false, false, nil // as it was: nothing to write
	}
	if err := s.upsertItem(ctx, it); err != nil {
		return 0, false, false, err
	}
	if isNew || moved {
		if err := s.setItemFiles(ctx, it.ID, paths); err != nil {
			return 0, false, false, err
		}
	}
	return it.ID, isNew, moved, nil
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func clipText(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		s = s[:n]
	}
	return s
}

// setItemFiles keeps an item's files, in order.
func (s *Store) setItemFiles(ctx context.Context, id int64, paths []string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM item_files WHERE item_id = ?`, id); err != nil {
		return err
	}
	for n, p := range paths {
		if _, err := tx.ExecContext(ctx, `INSERT INTO item_files(item_id, n, path) VALUES(?,?,?)`, id, n, p); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ItemFiles are an item's files in order: what a scan by a module kept (a game's discs), or
// its one path.
func (s *Store) ItemFiles(ctx context.Context, id int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT path FROM item_files WHERE item_id = ? ORDER BY n`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		var p string
		if err := s.DB.QueryRowContext(ctx, `SELECT path FROM items WHERE id = ?`, id).Scan(&p); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		out = []string{p}
	}
	return out, nil
}

// ItemEdits are the fields of an item a person typed in the item editor (wittymodule
// ItemFieldNames' names): nothing that describes the item changes them again.
func (s *Store) ItemEdits(ctx context.Context, id int64) []string {
	rows, err := s.DB.QueryContext(ctx, `SELECT field FROM item_edits WHERE item_id = ? ORDER BY field`, id)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var f string
		if rows.Scan(&f) == nil {
			out = append(out, f)
		}
	}
	return out
}

// SetItemEdited notes fields of an item a person typed.
func (s *Store) SetItemEdited(ctx context.Context, id int64, fields ...string) error {
	for _, f := range fields {
		if _, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO item_edits(item_id, field) VALUES(?, ?)`, id, f); err != nil {
			return err
		}
	}
	return nil
}

// DescribeItems asks the module that scans the items' library about them again (Refresh: an
// administrator asked), or for the first time (a game added to a held library). Nothing
// happens without a module that describes that kind of library.
func (s *Scanner) DescribeItems(ctx context.Context, libraryID int64, ids []int64, refresh bool) error {
	lib, err := s.Store.Library(ctx, libraryID)
	if err != nil {
		return err
	}
	mod, ok := s.scanModule(lib.Type)
	if !ok || !mod.On {
		return ErrNoScanModule
	}
	if !mod.Describe {
		return nil
	}
	return s.describe(ctx, lib, mod, ids, refresh, nil)
}

// ErrNoScanModule: the module that scans this kind of library isn't installed and turned on.
var ErrNoScanModule = errors.New("the module that scans this kind of library isn't installed and turned on")

// DescribesKind says whether items of a library of this kind are described by a module, and
// which: the item editor's Refresh asks it.
func (s *Scanner) DescribesKind(kind string) (ScanModule, bool) {
	mod, ok := s.scanModule(kind)
	return mod, ok && mod.On && mod.Describe
}

// describe sends items to scan.describe in batches, and applies what comes back. between runs
// after each batch (progress, a pause), and stops it with its error.
func (s *Scanner) describe(ctx context.Context, lib *Library, mod ScanModule, ids []int64, refresh bool, between func(batch []int64, done, total int) error) error {
	scanLib := scanLibraryOf(lib, mod)
	var failed error
	task := scanTask(ctx)
	for i := 0; i < len(ids); i += describeBatch {
		batch := ids[i:min(i+describeBatch, len(ids))]
		args := wittymodule.ScanDescribeArgs{Library: scanLib, Refresh: refresh}
		edits := map[int64][]string{}
		for _, id := range batch {
			it, err := s.Store.Item(ctx, id)
			if err != nil || it.LibraryID != lib.ID {
				continue
			}
			files, _ := s.Store.ItemFiles(ctx, id)
			edits[id] = s.Store.ItemEdits(ctx, id)
			d := wittymodule.ScanDescribeItem{ID: id, Kind: it.Kind, Title: it.Title, Files: files, Edited: edits[id]}
			if it.Console != "" && mod.Facet != "" {
				d.Facets = map[string]string{mod.Facet: it.Console}
			}
			args.Items = append(args.Items, d)
		}
		answered := true
		if len(args.Items) > 0 {
			var out wittymodule.ScanDescribed
			base := int64(i)
			called := liveOf(ctx).calling(mod.ID, mod.Name+" was finding out about "+args.Items[0].Title, func(done, _ int64, _ string) {
				task.Progress(base+min(done, int64(len(batch))), int64(len(ids)))
			})
			err := s.Modules.Call(ctx, mod.ID, wittymodule.CallScanDescribe, args, &out)
			called()
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				failed = fmt.Errorf("%s couldn't describe what was found: %w", mod.Name, err)
				if s.stopping.Load() {
					return failed // its service went with the server: the rest waits for the next start
				}
				answered = false
			} else {
				for _, f := range out.Items {
					if _, asked := edits[f.ID]; !asked {
						continue // only what was asked about
					}
					if err := s.ApplyItemFields(ctx, mod.ID, f); err != nil {
						if ctx.Err() != nil {
							return ctx.Err()
						}
						s.minorFailure(ctx, "could not keep what a module said about an item", err, "item", f.ID)
					}
				}
			}
		}
		if between != nil {
			var done []int64 // what was described: a batch the module couldn't answer stays to be asked again
			if answered {
				done = batch
			}
			if err := between(done, i+len(batch), len(ids)); err != nil {
				return err
			}
		}
	}
	return failed
}

// ErrModuleArt: a picture a module names must be one it kept through the host API.
var ErrModuleArt = errors.New("a poster or backdrop must be an address the host API's PutImage gave this module")

// ApplyItemFields sets an item's generic fields from what a module knows about it (scan.describe,
// or host PATCH /items/{id}). A field left empty stays as it is, Clear empties the fields it
// names, and a field a person typed (ItemEdits) is never changed. Whether the module may set
// this item's fields at all is the caller's to check.
func (s *Scanner) ApplyItemFields(ctx context.Context, module string, f wittymodule.ItemFields) error {
	it, err := s.Store.Item(ctx, f.ID)
	if err != nil {
		return err
	}
	edited := map[string]bool{}
	for _, e := range s.Store.ItemEdits(ctx, it.ID) {
		edited[e] = true
	}
	may := func(field string) bool { return !edited[field] }
	poster, backdrop := "", ""
	if f.Poster != "" && may("poster") {
		if poster, err = s.moduleArt(module, f.Poster); err != nil {
			return err
		}
	}
	if f.Backdrop != "" && may("backdrop") {
		if backdrop, err = s.moduleArt(module, f.Backdrop); err != nil {
			return err
		}
	}
	for _, c := range f.Clear {
		if !may(c) {
			continue
		}
		switch c {
		case "title":
			// An item always has a title: clearing it is a mistake, and it stays.
		case "tagline":
			it.Tagline = ""
		case "overview":
			it.Overview = ""
		case "year":
			it.Year = nil
		case "genres":
			it.Genres = nil
		case "rating":
			it.Rating = nil
		case "contentRating":
			it.ContentRating = ""
		case "studio":
			it.Studio = ""
		case "poster":
			it.Poster = ""
		case "backdrop":
			it.Backdrop = ""
		case "provider":
			it.Provider, it.ProviderID = "", ""
		}
	}
	if t := clipText(f.Title, 500); t != "" && may("title") {
		it.Title = t
		it.SortTitle = clipText(f.SortTitle, 500)
		if it.SortTitle == "" {
			it.SortTitle = SortTitle(t)
		}
	}
	if v := clipText(f.Tagline, 300); v != "" && may("tagline") {
		it.Tagline = v
	}
	if v := clipText(f.Overview, 20000); v != "" && may("overview") {
		it.Overview = v
	}
	if f.Year > 0 && f.Year < 3000 && may("year") {
		y := f.Year
		it.Year = &y
	}
	if len(f.Genres) > 0 && may("genres") {
		var g []string
		for _, x := range f.Genres {
			if x = clipText(strings.ReplaceAll(x, "|", "/"), 60); x != "" && !slices.Contains(g, x) && len(g) < 30 {
				g = append(g, x)
			}
		}
		it.Genres = g
	}
	if f.Rating != nil && may("rating") && !math.IsNaN(*f.Rating) && !math.IsInf(*f.Rating, 0) {
		r := min(max(*f.Rating, 0), 10)
		it.Rating = &r
	}
	if v := clipText(f.ContentRating, 32); v != "" && may("contentRating") {
		it.ContentRating = v
	}
	if v := clipText(f.Studio, 200); v != "" && may("studio") {
		it.Studio = v
	}
	if poster != "" {
		it.Poster = poster
	}
	if backdrop != "" {
		it.Backdrop = backdrop
	}
	if v := clipText(f.Provider, 64); v != "" && may("provider") {
		it.Provider, it.ProviderID = v, clipText(f.ProviderID, 128)
	}
	if err := s.Store.upsertItem(ctx, it); err != nil {
		return err
	}
	if may("titles") && may("title") {
		if f.Titles != nil {
			return s.Store.setItemTitles(ctx, it.ID, itemTitlesFor(f.Titles.Native, f.Titles.English, it.Title))
		}
		if slices.Contains(f.Clear, "titles") {
			return s.Store.setItemTitles(ctx, it.ID, "")
		}
	}
	return nil
}

// moduleArtPrefix is where pages show a picture a module kept (host PutImage).
const moduleArtPrefix = "/api/module-images/"

// moduleArt turns the address of a picture a module kept into the item's own copy in the image
// cache, so it's served, made smaller and forgotten as any other artwork is. Only a picture
// this module kept is taken.
func (s *Scanner) moduleArt(module, ref string) (string, error) {
	rest, ok := strings.CutPrefix(ref, moduleArtPrefix+module+"/")
	if !ok || rest == "" || rest != filepath.Base(rest) || strings.Count(rest, ".") != 1 || strings.HasPrefix(rest, ".") {
		return "", ErrModuleArt
	}
	src := filepath.Join(s.ImagesDir, "modules", module, rest)
	st, err := os.Stat(src)
	if err != nil || !st.Mode().IsRegular() {
		return "", ErrModuleArt
	}
	name := "mod-" + module + "-" + rest
	dst := filepath.Join(s.ImagesDir, name)
	if _, err := os.Stat(dst); err == nil {
		return "cache:" + name, nil
	}
	if err := os.Link(src, dst); err != nil {
		if err := copyFile(src, dst); err != nil {
			return "", err
		}
	}
	return "cache:" + name, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".part-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}
