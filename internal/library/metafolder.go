// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/metafolder"
)

// The editable metadata folder: one plain file per title, in <data>/metadata, that can
// be fixed with any text editor. Reading one back goes through the same field code as
// the edit dialog, so it's limited, locked and undone the same way.
//
// A record of what this server last wrote or applied is kept in the folder. A file that
// still matches isn't an edit, so it can't undo a change made since it was written; and
// one that doesn't is someone's work, which an export never writes over.

// Libraries whose titles have a file: what the edit dialog's fields fit.
var metaFolderTypes = map[string]bool{"movies": true, "shows": true, "anime": true, "discs": true, "videos": true, "books": true, "games": true, "musicvideos": true}

// ExportResult is how an export went.
type ExportResult struct {
	Total     int                  `json:"total"`
	Written   int                  `json:"written"`
	Unchanged int                  `json:"unchanged"`
	Kept      int                  `json:"kept"` // files with edits not applied yet, left alone
	Problems  []metafolder.Problem `json:"problems"`
}

// FolderFieldChange is one thing a file would change.
type FolderFieldChange struct {
	Field string `json:"field"`
	Label string `json:"label"`
	Old   string `json:"old"`
	New   string `json:"new"`
	Line  int    `json:"line,omitempty"`
}

// FileChange is what one file would change.
type FileChange struct {
	File   string              `json:"file"`
	Target string              `json:"target"`
	ID     int64               `json:"id"`
	Name   string              `json:"name"`
	Fields []FolderFieldChange `json:"fields"`
}

// FolderPreview is what applying the folder would do.
type FolderPreview struct {
	Dir       string               `json:"dir"`
	Files     int                  `json:"files"`     // files of titles, and pictures
	Unchanged int                  `json:"unchanged"` // nothing to do
	Changes   []FileChange         `json:"changes"`
	Listed    bool                 `json:"listed"` // false: more changes than were listed
	Total     int                  `json:"total"`  // changed files, listed or not
	Problems  []metafolder.Problem `json:"problems"`
}

// FolderResult is how applying it went.
type FolderResult struct {
	Batch    string               `json:"batch,omitempty"` // for undoing all of it
	Applied  int                  `json:"applied"`
	Fields   int                  `json:"fields"`
	Skipped  int                  `json:"skipped"`
	Changes  []FileChange         `json:"changes"`
	Problems []metafolder.Problem `json:"problems"`
}

// FolderApply says who is applying and where things go.
type FolderApply struct {
	User      string
	ImagesDir string
	// Persist writes a title beside its media after a change, if the owner wants that.
	Persist func(context.Context, *Title)
}

const (
	maxListedChanges  = 1000
	maxListedProblems = 1000
	maxShownValue     = 300
)

func fieldLabel(f string) string {
	switch f {
	case "locked":
		return "Locked fields"
	case "match":
		return "Match"
	}
	if l := FieldLabel(f); l != "" {
		return l
	}
	return f
}

// ---------------------------------------------------------------- export

// folderName gives each library a folder, the same one each time. Two names that a file
// system can't tell apart get the library's number.
func folderNames(libs []Library) map[int64]string {
	sort.Slice(libs, func(i, j int) bool { return libs[i].ID < libs[j].ID })
	used := map[string]bool{}
	out := map[int64]string{}
	for _, l := range libs {
		name := metafolder.SafeName(l.Name)
		if used[strings.ToLower(name)] {
			name = fmt.Sprintf("%s [library-%d]", name, l.ID)
		}
		used[strings.ToLower(name)] = true
		out[l.ID] = name
	}
	return out
}

// titleFile is a title's file as an export writes it.
func titleFile(t *Title) ([]byte, error) {
	list := []metafolder.Field{
		{Name: "wittyMetadata", Value: metafolder.Format},
		{Name: "target", Value: t.Target},
		{Name: "id", Value: t.ID},
		{Name: "path", Value: t.Path()},
	}
	for _, f := range t.FieldNames() {
		stored := t.Field(f)
		if v, ok := metafolder.ValueFor(f, stored); ok {
			if f == "title" && v == "" {
				v = "Untitled"
			}
			list = append(list, metafolder.Field{Name: f, Value: v})
		}
	}
	locked := []string{}
	for f, on := range t.Meta.Locks {
		if on && t.hasField(f) {
			locked = append(locked, f)
		}
	}
	sort.Strings(locked)
	list = append(list, metafolder.Field{Name: "locked", Value: locked})
	if p, id := t.Provider(); metafolder.KnownProvider(p) && id != "" {
		if clean, ok := metafolder.ProviderID(id); ok {
			list = append(list, metafolder.Field{Name: "provider", Value: p}, metafolder.Field{Name: "providerId", Value: clean})
		}
	}
	return metafolder.Encode(list)
}

// existingFiles maps the titles that have a file already to where it is. A file stays
// where it is when a title is renamed: the id is what counts, and two files for one
// title would only be trouble.
func existingFiles(dir string) (map[string]string, []metafolder.Problem) {
	out := map[string]string{}
	ps := metafolder.Walk(dir, func(e metafolder.Entry) {
		if e.Kind != "json" && e.Kind != "nfo" {
			return
		}
		base := path.Base(e.Rel)
		target, id, ok := metafolder.IDFromName(base)
		if !ok && base == "show.json" {
			target, id, ok = metafolder.IDFromName(path.Base(path.Dir(e.Rel)))
		}
		if ok {
			key := fmt.Sprintf("%s-%d", target, id)
			if _, has := out[key]; !has {
				out[key] = e.Rel
			}
		}
	})
	return out, ps
}

// ExportMetadata writes the folder: every title of one library, or all of them with
// libraryID 0. A file is only written if it would be different, and never over edits
// that haven't been applied. Nothing is ever deleted.
func (s *Store) ExportMetadata(ctx context.Context, dir string, libraryID int64, progress func(done, total int)) (*ExportResult, error) {
	all, err := s.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	names := folderNames(all)
	res := &ExportResult{Problems: []metafolder.Problem{}}
	if _, err := metafolder.SafeDir(dir, ""); err != nil {
		return nil, err
	}
	state := metafolder.LoadState(dir)
	existing, ps := existingFiles(dir)
	res.Problems = append(res.Problems, ps...)

	type work struct {
		target string
		id     int64
		show   int64
		lib    string
	}
	var todo []work
	for _, l := range all {
		if (libraryID != 0 && l.ID != libraryID) || !metaFolderTypes[l.Type] {
			continue
		}
		rows, err := s.DB.QueryContext(ctx, `SELECT id FROM shows WHERE library_id = ? ORDER BY id`, l.ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			todo = append(todo, work{target: TargetShow, id: id, lib: names[l.ID]})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		rows, err = s.DB.QueryContext(ctx, `SELECT id, IFNULL(show_id, 0) FROM items WHERE library_id = ? AND missing = 0 AND cd_disc IS NULL ORDER BY id`, l.ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, show int64
			if err := rows.Scan(&id, &show); err != nil {
				rows.Close()
				return nil, err
			}
			todo = append(todo, work{target: TargetItem, id: id, show: show, lib: names[l.ID]})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	res.Total = len(todo)

	// showDirs: where a show's episodes go, worked out once per show.
	showDirs := map[int64]string{}
	showDir := func(ctx context.Context, lib string, id int64) (string, error) {
		if d, ok := showDirs[id]; ok {
			return d, nil
		}
		if rel, ok := existing[fmt.Sprintf("%s-%d", TargetShow, id)]; ok {
			showDirs[id] = path.Dir(rel)
			return showDirs[id], nil
		}
		sh, err := s.Show(ctx, id, 0)
		if err != nil {
			return "", err
		}
		d := lib + "/" + metafolder.FolderNameFor(sh.Title, sh.ID)
		showDirs[id] = d
		return d, nil
	}
	for i, w := range todo {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if progress != nil {
			progress(i, len(todo))
		}
		t, err := s.LoadTitle(ctx, w.target, w.id)
		if errors.Is(err, ErrNotFound) {
			continue // removed while this ran
		}
		if err != nil {
			return res, err
		}
		key := fmt.Sprintf("%s-%d", w.target, w.id)
		rel, has := existing[key]
		if !has {
			switch {
			case w.target == TargetShow:
				d, err := showDir(ctx, w.lib, w.id)
				if err != nil {
					return res, err
				}
				rel = d + "/show.json"
			case w.show != 0:
				d, err := showDir(ctx, w.lib, w.show)
				if err != nil {
					if errors.Is(err, ErrNotFound) {
						d = w.lib
					} else {
						return res, err
					}
				}
				rel = d + "/" + metafolder.FileNameFor(episodeLabel(t.Item), TargetItem, w.id)
			default:
				rel = w.lib + "/" + metafolder.FileNameFor(titleLabel(t.Item), TargetItem, w.id)
			}
		}
		if !strings.HasSuffix(strings.ToLower(rel), ".json") {
			res.Kept++ // a hand-written NFO stands in for this title
			continue
		}
		body, err := titleFile(t)
		if err != nil {
			return res, err
		}
		writeOwned(dir, rel, body, state, res)
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}
	if progress != nil {
		progress(len(todo), len(todo))
	}
	writeOwned(dir, "README.txt", []byte(metafolder.Readme), state, &ExportResult{Problems: []metafolder.Problem{}})
	if err := state.Save(dir); err != nil {
		return res, err
	}
	metafolder.SortProblems(res.Problems)
	return res, nil
}

// writeOwned writes a file unless it's the same already, or someone has changed it.
func writeOwned(dir, rel string, body []byte, state metafolder.State, res *ExportResult) {
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	fi, err := os.Lstat(abs)
	switch {
	case err == nil && !metafolder.PlainFile(fi):
		res.Problems = append(res.Problems, metafolder.Problem{File: rel, Msg: "This is a link or a special file, so it was left alone", Severity: metafolder.SevWarning})
		return
	case err == nil:
		cur, err := metafolder.ReadPlain(abs, 8<<20)
		if err != nil {
			res.Problems = append(res.Problems, metafolder.Problem{File: rel, Msg: "This file can't be read, so it was left alone", Severity: metafolder.SevWarning})
			return
		}
		if string(cur) == string(body) {
			state[rel] = metafolder.HashOf(body)
			res.Unchanged++
			return
		}
		if state[rel] != metafolder.HashOf(cur) {
			res.Kept++ // edited, and not applied yet
			return
		}
	case !errors.Is(err, os.ErrNotExist):
		res.Problems = append(res.Problems, metafolder.Problem{File: rel, Msg: "This file can't be read, so it was left alone", Severity: metafolder.SevWarning})
		return
	}
	if err := metafolder.WriteAtomic(dir, rel, body); err != nil {
		res.Problems = append(res.Problems, metafolder.Problem{File: rel, Msg: "This file can't be written: " + err.Error(), Severity: metafolder.SevError})
		return
	}
	state[rel] = metafolder.HashOf(body)
	res.Written++
}

func titleLabel(it *Item) string {
	if it.Year != nil {
		return fmt.Sprintf("%s (%d)", it.Title, *it.Year)
	}
	return it.Title
}

func episodeLabel(it *Item) string {
	if it.Season != nil && it.Episode != nil {
		return fmt.Sprintf("S%02dE%02d %s", *it.Season, *it.Episode, it.Title)
	}
	return it.Title
}

// ---------------------------------------------------------------- reading it back

// folderScan is what's in the folder, and which of it someone has changed.
type folderScan struct {
	docs     []metafolder.Entry
	touched  map[string]bool // files that aren't as last written or applied
	artOnly  map[string]bool // files beside a changed picture: only their pictures count
	pictures map[string]string
	state    metafolder.State
	problems []metafolder.Problem
	files    int
}

func scanFolder(dir string) *folderScan {
	fs := &folderScan{touched: map[string]bool{}, artOnly: map[string]bool{}, pictures: map[string]string{}, state: metafolder.LoadState(dir)}
	var pics []metafolder.Entry
	fs.problems = metafolder.Walk(dir, func(e metafolder.Entry) {
		fs.files++
		if e.Kind == "picture" {
			pics = append(pics, e)
		} else {
			fs.docs = append(fs.docs, e)
		}
	})
	changedDirs := map[string]bool{}
	for _, e := range pics {
		b, err := metafolder.ReadPlain(e.Abs, metafolder.MaxPicture)
		if err != nil {
			fs.problems = append(fs.problems, metafolder.Problem{File: e.Rel, Msg: "This picture can't be used: " + err.Error(), Severity: metafolder.SevWarning})
			continue
		}
		h := metafolder.HashOf(b)
		fs.pictures[e.Rel] = h
		if fs.state[e.Rel] != h {
			changedDirs[path.Dir(e.Rel)] = true
		}
	}
	for _, e := range fs.docs {
		b, err := metafolder.ReadEntry(e)
		if err != nil || fs.state[e.Rel] != metafolder.HashOf(b) {
			fs.touched[e.Rel] = true
		} else if changedDirs[path.Dir(e.Rel)] {
			fs.artOnly[e.Rel] = true
		}
	}
	return fs
}

// plan is one file worked out against the title it's for.
type plan struct {
	change   FileChange
	changes  map[string]string // field to new value, for EditFields
	pictures map[string]string // field to the picture's file, to copy in
	locks    map[string]bool   // the locks it should end with, or nil
	provider *[2]string
	problems []metafolder.Problem
}

func sameBase(a, b string) bool {
	last := func(p string) string {
		p = strings.TrimRight(strings.ReplaceAll(p, `\`, "/"), "/")
		if i := strings.LastIndexByte(p, '/'); i >= 0 {
			p = p[i+1:]
		}
		return strings.ToLower(p)
	}
	return last(a) == last(b)
}

func shortValue(s string) string {
	return clip(strings.ReplaceAll(s, "|", ", "), maxShownValue)
}

func pictureRef(b []byte, ext string) string {
	h := sha1.Sum(b)
	return "cache:up-" + hex.EncodeToString(h[:10]) + ext
}

// planFile works out what a file would change. Nothing is saved: the title it works on
// is thrown away.
func (s *Store) planFile(ctx context.Context, e metafolder.Entry, d *metafolder.Doc, artOnly bool) (*plan, error) {
	p := &plan{changes: map[string]string{}, pictures: map[string]string{}}
	fail := func(line int, field, msg string) {
		p.problems = append(p.problems, metafolder.Problem{File: e.Rel, Line: line, Field: field, Msg: msg, Severity: metafolder.SevError})
	}
	warn := func(line int, field, msg string) {
		p.problems = append(p.problems, metafolder.Problem{File: e.Rel, Line: line, Field: field, Msg: msg, Severity: metafolder.SevWarning})
	}
	t, err := s.LoadTitle(ctx, d.Target, d.ID)
	if errors.Is(err, ErrNotFound) {
		fail(d.Lines["id"], "id", fmt.Sprintf("There's no title with the number %d on this server. It may have been removed, or this file may be from another server. Export the folder again to get a fresh file", d.ID))
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	if d.Path != "" && !sameBase(d.Path, t.Path()) {
		fail(d.Lines["path"], "path", "This file was written for "+shortValue(path.Base(strings.ReplaceAll(d.Path, `\`, "/")))+", but this number is now "+shortValue(path.Base(strings.ReplaceAll(t.Path(), `\`, "/")))+". Export the folder again, or if you're sure, delete the \"path\" line")
		return p, nil
	}
	p.change = FileChange{File: e.Rel, Target: d.Target, ID: d.ID, Name: t.Name(), Fields: []FolderFieldChange{}}

	add := func(field, old, new string) {
		p.change.Fields = append(p.change.Fields, FolderFieldChange{Field: field, Label: fieldLabel(field), Old: shortValue(old), New: shortValue(new), Line: d.Lines[field]})
	}
	if !artOnly {
		names := make([]string, 0, len(d.Fields))
		for f := range d.Fields {
			names = append(names, f)
		}
		// In the order they are in the file, which is the order someone reads it.
		sort.Slice(names, func(i, j int) bool {
			if d.Lines[names[i]] != d.Lines[names[j]] {
				return d.Lines[names[i]] < d.Lines[names[j]]
			}
			return names[i] < names[j]
		})
		for _, f := range names {
			v := d.Fields[f]
			if !t.hasField(f) {
				if v != "" {
					warn(d.Lines[f], f, "This kind of title has no "+strings.ToLower(fieldLabel(f))+", so that was left out")
				}
				continue
			}
			old := t.Field(f)
			if err := t.SetField(f, v); err != nil {
				fail(d.Lines[f], f, "\""+f+"\": "+err.Error())
				return p, nil
			}
			now := t.Field(f)
			if now == old || now == metafolder.Clean(f, old) {
				continue
			}
			p.changes[f] = v
			add(f, old, now)
		}
	}
	dirAbs := filepath.Dir(e.Abs)
	base := path.Base(e.Rel)
	for _, f := range []string{"poster", "backdrop", "banner", "logo"} {
		art, named := d.Art[f]
		// A picture put beside the file is the most deliberate thing there is, so it comes
		// before a web address the file may have had since it was exported.
		if !named || art.File == "" {
			if dropped := metafolder.DroppedPicture(dirAbs, base, f); dropped != "" {
				art, named = metafolder.Art{File: dropped}, true
			}
		}
		if !named || (artOnly && art.File == "") {
			continue
		}
		if !t.hasField(f) {
			warn(d.Lines[f], f, "This kind of title has no "+strings.ToLower(fieldLabel(f))+", so that was left out")
			continue
		}
		old := t.Field(f)
		switch {
		case art.File != "":
			b, ext, err := metafolder.ReadPicture(filepath.Join(dirAbs, art.File))
			if err != nil {
				fail(d.Lines[f], f, "\""+f+"\" names "+art.File+", which can't be used: "+plainErr(err))
				return p, nil
			}
			if pictureRef(b, ext) != old {
				p.pictures[f] = art.File
				add(f, pictureWords(old), "The picture "+art.File)
			}
		case art.URL != old:
			p.changes[f] = art.URL
			add(f, pictureWords(old), art.URL)
		}
	}
	if artOnly {
		return p, nil
	}
	if d.HasLocked {
		want := map[string]bool{}
		for _, f := range d.Locked {
			if t.hasField(f) {
				want[f] = true
			}
		}
		for f := range p.changes {
			want[f] = true
		}
		for f := range p.pictures {
			want[f] = true
		}
		now := map[string]bool{}
		for f, on := range t.Meta.Locks {
			if on && t.hasField(f) {
				now[f] = true
			}
		}
		if !sameSet(want, now) {
			// Fields the file changes lock themselves, so those aren't worth a line.
			if !sameSet(withoutChanged(want, p), withoutChanged(now, p)) {
				add("locked", setWords(now), setWords(want))
			}
			p.locks = want
		}
	}
	_, hasP := d.Lines["provider"]
	_, hasI := d.Lines["providerId"]
	if hasP || hasI {
		cur, curID := t.Provider()
		next, nextID := cur, curID
		if hasP {
			next = d.Provider
		}
		if hasI {
			nextID = d.ProviderID
		}
		if next != cur || nextID != curID {
			if (next == "") != (nextID == "") {
				fail(d.Lines["provider"], "provider", "\"provider\" and \"providerId\" go together: give both, or empty both to remove the match")
				return p, nil
			}
			p.provider = &[2]string{next, nextID}
			add("match", strings.TrimSpace(cur+" "+curID), strings.TrimSpace(next+" "+nextID))
		}
	}
	return p, nil
}

func plainErr(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

func pictureWords(v string) string {
	switch {
	case v == "":
		return "None"
	case isRemote(v):
		return v
	}
	return "The server's own picture"
}

func sameSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func withoutChanged(in map[string]bool, p *plan) map[string]bool {
	out := map[string]bool{}
	for k := range in {
		_, changed := p.changes[k]
		_, picture := p.pictures[k]
		if !changed && !picture {
			out[k] = true
		}
	}
	return out
}

func setWords(in map[string]bool) string {
	if len(in) == 0 {
		return "None"
	}
	list := make([]string, 0, len(in))
	for k := range in {
		list = append(list, fieldLabel(k))
	}
	sort.Strings(list)
	return strings.Join(list, ", ")
}

// readTargets reads the files someone changed, and works out which can't be used
// because two of them claim one title.
func (s *Store) readTargets(fs *folderScan) (map[string]*metafolder.Doc, []metafolder.Problem) {
	docs := map[string]*metafolder.Doc{}
	var ps []metafolder.Problem
	// What the file names say, for all of them: one nobody touched can still be the
	// other half of two files for one title.
	byName := map[string][]string{}
	for _, e := range fs.docs {
		base := path.Base(e.Rel)
		target, id, ok := metafolder.IDFromName(base)
		if !ok && base == "show.json" {
			target, id, ok = metafolder.IDFromName(path.Base(path.Dir(e.Rel)))
		}
		if ok {
			key := fmt.Sprintf("%s-%d", target, id)
			byName[key] = append(byName[key], e.Rel)
		}
	}
	byTitle := map[string]string{}
	for _, e := range fs.docs {
		if !fs.touched[e.Rel] && !fs.artOnly[e.Rel] {
			continue
		}
		d, problems := metafolder.ReadDoc(e)
		ps = append(ps, problems...)
		if metafolder.HasErrors(problems) {
			continue
		}
		key := fmt.Sprintf("%s-%d", d.Target, d.ID)
		other, dup := byTitle[key]
		if !dup {
			for _, rel := range byName[key] {
				if rel != e.Rel {
					other, dup = rel, true
					break
				}
			}
		}
		if dup {
			ps = append(ps, metafolder.Problem{File: e.Rel, Line: d.Lines["id"], Field: "id", Msg: "This file and " + other + " describe the same title. Delete one of them. Neither was used", Severity: metafolder.SevError})
			delete(docs, other)
			continue
		}
		byTitle[key] = e.Rel
		docs[e.Rel] = d
	}
	return docs, ps
}

func tally(ps []metafolder.Problem, n int) []metafolder.Problem {
	metafolder.SortProblems(ps)
	if len(ps) > n {
		more := len(ps) - n
		ps = append(ps[:n:n], metafolder.Problem{File: ".", Msg: fmt.Sprintf("...and %d more. Fix these and check again", more), Severity: metafolder.SevWarning})
	}
	return ps
}

// PreviewMetadataFolder says what applying the folder would change, field by field,
// and what's wrong with any file. Nothing is written.
func (s *Store) PreviewMetadataFolder(ctx context.Context, dir string) (*FolderPreview, error) {
	fs := scanFolder(dir)
	docs, ps := s.readTargets(fs)
	out := &FolderPreview{Dir: dir, Files: fs.files, Changes: []FileChange{}, Listed: true}
	problems := append(append([]metafolder.Problem{}, fs.problems...), ps...)
	for _, e := range fs.docs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		d := docs[e.Rel]
		if d == nil {
			if !fs.touched[e.Rel] && !fs.artOnly[e.Rel] {
				out.Unchanged++
			}
			continue
		}
		p, err := s.planFile(ctx, e, d, fs.artOnly[e.Rel])
		if err != nil {
			return nil, err
		}
		problems = append(problems, p.problems...)
		switch {
		case metafolder.HasErrors(p.problems):
		case len(p.change.Fields) == 0:
			out.Unchanged++
		default:
			out.Total++
			if len(out.Changes) < maxListedChanges {
				out.Changes = append(out.Changes, p.change)
			} else {
				out.Listed = false
			}
		}
	}
	out.Problems = tally(problems, maxListedProblems)
	return out, nil
}

// SavePicture keeps a picture in the image cache under a name that's only ours, and
// says how a title's field refers to it.
func SavePicture(imagesDir string, b []byte, ext string) (string, error) {
	if ext == "" || metafolder.PictureExt(b) != ext {
		return "", errors.New("that isn't a picture")
	}
	ref := pictureRef(b, ext)
	if _, err := metafolder.SafeDir(imagesDir, ""); err != nil {
		return "", err
	}
	if err := metafolder.WriteAtomic(imagesDir, strings.TrimPrefix(ref, "cache:"), b); err != nil {
		return "", err
	}
	return ref, nil
}

// picturesOf says whether a picture in the folder belongs to this file: named in it, or
// beside it under the name that goes with it.
func picturesOf(file string, d *metafolder.Doc, pic string) bool {
	if path.Dir(file) != path.Dir(pic) {
		return false
	}
	name := path.Base(pic)
	for _, a := range d.Art {
		if a.File == name {
			return true
		}
	}
	base := path.Base(file)
	stem := strings.TrimSuffix(name, path.Ext(name))
	if base == "show.json" {
		return stem == "poster" || stem == "backdrop" || stem == "banner" || stem == "logo"
	}
	return strings.HasPrefix(stem, strings.TrimSuffix(base, path.Ext(base))+"-")
}

// ApplyMetadataFolder does what the preview said: each changed file through the same
// code as the edit dialog, in one batch that can be undone as a whole. A bad file is
// listed and left out, and the rest go ahead.
func (s *Store) ApplyMetadataFolder(ctx context.Context, dir string, o FolderApply) (*FolderResult, error) {
	fs := scanFolder(dir)
	docs, ps := s.readTargets(fs)
	res := &FolderResult{Changes: []FileChange{}}
	problems := append(append([]metafolder.Problem{}, fs.problems...), ps...)
	batch := NewBatch()
	state := fs.state
	saveState := func() error {
		if err := state.Save(dir); err != nil {
			return fmt.Errorf("the changes were made, but the folder's record couldn't be saved: %w", err)
		}
		return nil
	}
	for _, e := range fs.docs {
		if err := ctx.Err(); err != nil {
			_ = saveState()
			return res, err
		}
		d := docs[e.Rel]
		if d == nil {
			if fs.touched[e.Rel] || fs.artOnly[e.Rel] {
				res.Skipped++
			}
			continue
		}
		artOnly := fs.artOnly[e.Rel]
		p, err := s.planFile(ctx, e, d, artOnly)
		if err != nil {
			return res, err
		}
		problems = append(problems, p.problems...)
		if metafolder.HasErrors(p.problems) {
			res.Skipped++
			continue
		}
		if len(p.changes) > 0 || len(p.pictures) > 0 || p.locks != nil || p.provider != nil {
			t, err := s.LoadTitle(ctx, d.Target, d.ID)
			if err != nil {
				return res, err
			}
			before := t.Snapshot()
			dirAbs := filepath.Dir(e.Abs)
			bad := false
			for f, name := range p.pictures {
				b, ext, err := metafolder.ReadPicture(filepath.Join(dirAbs, name))
				if err == nil {
					var ref string
					if ref, err = SavePicture(o.ImagesDir, b, ext); err == nil {
						p.changes[f] = ref
						continue
					}
				}
				problems = append(problems, metafolder.Problem{File: e.Rel, Line: d.Lines[f], Field: f, Msg: "\"" + f + "\" names " + name + ", which can't be used: " + plainErr(err), Severity: metafolder.SevError})
				bad = true
			}
			if bad {
				res.Skipped++
				continue
			}
			if err := s.Remember(ctx, batch, o.User, "Edited in the metadata folder", t, before); err != nil {
				return res, err
			}
			if err := t.EditFields(p.changes, true); err != nil {
				problems = append(problems, metafolder.Problem{File: e.Rel, Msg: err.Error(), Severity: metafolder.SevError})
				res.Skipped++
				continue
			}
			if p.locks != nil {
				t.Meta.Locks = map[string]bool{}
				for f := range p.locks {
					t.Meta.Locks[f] = true
				}
			}
			if p.provider != nil {
				t.setProvider(p.provider[0], p.provider[1])
				if p.provider[1] == "" {
					t.Meta.MatchState = MatchNone
				} else {
					t.Meta.MatchState = MatchManual
				}
			}
			if o.Persist != nil && len(p.changes) > 0 {
				o.Persist(ctx, t)
			}
			if err := s.SaveTitle(ctx, t); err != nil {
				return res, err
			}
			res.Applied++
			res.Fields += len(p.change.Fields)
			if len(res.Changes) < maxListedChanges {
				res.Changes = append(res.Changes, p.change)
			}
			res.Batch = batch
		}
		// Done with: it isn't an edit any more, and a later export may bring it up to date.
		if b, err := metafolder.ReadEntry(e); err == nil {
			state[e.Rel] = metafolder.HashOf(b)
		}
		for rel, h := range fs.pictures {
			if picturesOf(e.Rel, d, rel) {
				state[rel] = h
			}
		}
	}
	res.Problems = tally(problems, maxListedProblems)
	if err := saveState(); err != nil {
		return res, err
	}
	return res, nil
}
