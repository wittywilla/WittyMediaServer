// Package uninstall removes WittyMediaServer from a computer. The Windows uninstaller, the
// Mac uninstaller and `wittymediaserver --uninstall` are all this package with a different
// way of asking.
//
// It works from a plan: it looks at the machine, lists everything of ours that it finds,
// and removes only what is on the list. Three rules decide what can get on the list:
//
//   - It is a name we made: a service, a registry key, a known file or folder. Nothing is
//     found by searching for media or by deleting whole folders it can't vouch for, so a
//     file someone put in the data folder stays, and is reported as left there.
//   - Media is never touched. Libraries are read from the database, and a folder that holds
//     one stays whatever it is called. Recorded TV is media too, and goes only when asked
//     for on its own.
//   - A folder someone chose (a cache on another drive, say) is theirs. It's reported, not
//     emptied.
//
// Planning and removing run against an Env, so every system's plan is tested on any system.
package uninstall

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Group is what a person is deciding about.
type Group string

const (
	GroupProgram    Group = "program"    // program files, service, shortcuts, startup entries
	GroupData       Group = "data"       // library, accounts, settings, logs, caches, modules
	GroupRecordings Group = "recordings" // recorded TV
)

// Steps run in this order: stop what runs, let an installer remove what it put there, remove
// what is left, then clear the folders that are now empty.
const (
	stepStop = iota
	stepInstaller
	stepFiles
	stepEmptyFolders
)

// Item is one thing to remove.
type Item struct {
	Kind  string `json:"kind"` // what it is, in plain words: "folder", "service", "startup entry"
	Name  string `json:"name"` // the path, or the name it goes by
	Group Group  `json:"group"`
	Bytes int64  `json:"bytes,omitempty"`
	// Admin says removing it needs an administrator or root.
	Admin bool `json:"admin,omitempty"`

	step   int
	remove func() error
	// gone says there's nothing left to remove, because a step before took it with it.
	gone func() bool
}

// Note is something left in place, and why.
type Note struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// Plan is what removing would do.
type Plan struct {
	Items []Item `json:"items"`
	Kept  []Note `json:"kept,omitempty"`

	seen map[string]bool
}

// Options are a person's answers.
type Options struct {
	RemoveData       bool // library, accounts, settings, logs, caches, modules
	RemoveRecordings bool // recorded TV in the data folder
}

// Failure is something that couldn't be removed.
type Failure struct {
	Item
	Error string `json:"error"`
}

// Result is what happened.
type Result struct {
	Removed []Item    `json:"removed"`
	Failed  []Failure `json:"failed,omitempty"`
	Kept    []Note    `json:"kept,omitempty"`
}

// OK says everything asked for was removed.
func (r Result) OK() bool { return len(r.Failed) == 0 }

// System is what changes a machine other than through its files. Tests use a fake.
type System interface {
	// Run starts a program and waits for it.
	Run(name string, args ...string) (string, error)
	// StopProgramsIn ends every running program whose file is inside dir.
	StopProgramsIn(dir string)
	// The Windows registry. hive is "HKCU" or "HKLM". Elsewhere there are no keys.
	RegKeyExists(hive, key string) bool
	RegValue(hive, key, name string) (string, bool)
	RegDeleteKey(hive, key string) error
	RegDeleteValue(hive, key, name string) error
}

// Env is the machine being looked at.
type Env struct {
	GOOS    string
	Getenv  func(string) string
	Home    string
	IsAdmin bool
	// Prefix goes before every absolute path on Linux, BSD and macOS. Only tests set it.
	Prefix string
	// SelfRoot is the package the running program belongs to, "" when it isn't in one.
	SelfRoot string
	// Sys changes the machine.
	Sys System
	// Libraries reads a data folder's library folders and its chosen folders. An error
	// means they aren't known, which is reported.
	Libraries func(dataDir string) (DataFacts, error)
}

// DataFacts is what a server's database says about folders outside its control.
type DataFacts struct {
	Libraries []string // library folders
	Chosen    []Note   // folders chosen in Settings
}

func (e *Env) p(path string) string { return e.Prefix + filepath.FromSlash(path) }

// MakePlan looks at the machine.
func MakePlan(e *Env) *Plan {
	pl := &Plan{}
	switch e.GOOS {
	case "windows":
		planWindows(e, pl)
	case "darwin":
		planMac(e, pl)
	default:
		planUnix(e, pl)
	}
	pl.sort()
	return pl
}

func (pl *Plan) sort() {
	sort.SliceStable(pl.Items, func(a, b int) bool { return pl.Items[a].step < pl.Items[b].step })
}

func (pl *Plan) add(it Item) { pl.Items = append(pl.Items, it) }

func (pl *Plan) keep(name, reason string) {
	for _, k := range pl.Kept {
		if k.Name == name {
			return
		}
	}
	pl.Kept = append(pl.Kept, Note{name, reason})
}

// first says a folder hasn't been looked through yet.
func (pl *Plan) first(path string) bool {
	if pl.seen == nil {
		pl.seen = map[string]bool{}
	}
	key := strings.ToLower(filepath.Clean(path))
	if pl.seen[key] {
		return false
	}
	pl.seen[key] = true
	return true
}

// Select is the part of a plan a person's answers cover.
func (pl *Plan) Select(o Options) []Item {
	var out []Item
	for _, it := range pl.Items {
		switch it.Group {
		case GroupData:
			if !o.RemoveData {
				continue
			}
		case GroupRecordings:
			if !o.RemoveRecordings {
				continue
			}
		}
		out = append(out, it)
	}
	return out
}

// NeedsAdmin says some of what these answers cover can't be removed by a normal account.
func (pl *Plan) NeedsAdmin(o Options) bool {
	for _, it := range pl.Select(o) {
		if it.Admin {
			return true
		}
	}
	return false
}

// Bytes is the size of everything in a group.
func (pl *Plan) Bytes(g Group) int64 {
	var n int64
	for _, it := range pl.Items {
		if it.Group == g {
			n += it.Bytes
		}
	}
	return n
}

// Has says the group has something in it.
func (pl *Plan) Has(g Group) bool {
	for _, it := range pl.Items {
		if it.Group == g {
			return true
		}
	}
	return false
}

// Execute removes what the answers cover. One thing failing doesn't stop the rest, and what
// the data question didn't cover is reported as kept.
func Execute(pl *Plan, o Options) Result {
	r := Result{Kept: append([]Note(nil), pl.Kept...)}
	for _, it := range pl.Items {
		if (it.Group == GroupData && !o.RemoveData) || (it.Group == GroupRecordings && !o.RemoveRecordings) {
			if it.step != stepEmptyFolders {
				r.Kept = append(r.Kept, Note{it.Name, "you chose to keep it"})
			}
			continue
		}
		if it.gone != nil && it.gone() {
			if it.step != stepEmptyFolders {
				r.Removed = append(r.Removed, it)
			}
			continue
		}
		if err := it.remove(); err != nil {
			r.Failed = append(r.Failed, Failure{it, err.Error()})
			continue
		}
		r.Removed = append(r.Removed, it)
	}
	return r
}

// Report is the result in plain words.
func (r Result) Report() string {
	var b strings.Builder
	if len(r.Removed) > 0 {
		fmt.Fprintf(&b, "Removed:\n")
		for _, it := range r.Removed {
			fmt.Fprintf(&b, "  %s: %s\n", it.Kind, it.Name)
		}
	}
	if len(r.Failed) > 0 {
		fmt.Fprintf(&b, "\nCouldn't be removed:\n")
		for _, f := range r.Failed {
			fmt.Fprintf(&b, "  %s: %s (%s)\n", f.Kind, f.Name, f.Error)
		}
	}
	if len(r.Kept) > 0 {
		fmt.Fprintf(&b, "\nLeft in place:\n")
		for _, k := range r.Kept {
			fmt.Fprintf(&b, "  %s: %s\n", k.Name, k.Reason)
		}
	}
	return b.String()
}

// Describe is what a plan covers, in plain words, for asking.
func (pl *Plan) Describe(g Group) string {
	var b strings.Builder
	for _, it := range pl.Items {
		if it.Group == g && it.step != stepEmptyFolders {
			fmt.Fprintf(&b, "  %s: %s", it.Kind, it.Name)
			if it.Bytes > 0 {
				fmt.Fprintf(&b, " (%s)", Size(it.Bytes))
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// Size is a number of bytes for people.
func Size(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

// ----- files

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func isDir(p string) bool {
	st, err := os.Lstat(p)
	return err == nil && st.IsDir()
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// inside says p is dir or something in it. Paths compare without regard to case, which
// errs on the side of keeping something on a system where case matters.
func inside(p, dir string) bool {
	r, err := filepath.Rel(strings.ToLower(filepath.Clean(dir)), strings.ToLower(filepath.Clean(p)))
	return err == nil && (r == "." || filepath.IsLocal(r))
}

func sizeOf(p string) int64 {
	var n int64
	_ = filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if st, err := d.Info(); err == nil {
				n += st.Size()
			}
		}
		return nil
	})
	return n
}

// fileItem is a file or a whole folder that's entirely ours.
func fileItem(kind, path string, g Group, admin bool) Item {
	return Item{Kind: kind, Name: path, Group: g, Bytes: sizeOf(path), Admin: admin, step: stepFiles,
		remove: func() error { return os.RemoveAll(path) },
		gone:   func() bool { return !exists(path) }}
}

// emptyFolderItem removes a folder once what we put in it has gone. Whatever is left in it
// isn't ours, and then the folder stays.
func emptyFolderItem(path string, g Group, admin bool) Item {
	return Item{Kind: "folder", Name: path, Group: g, Admin: admin, step: stepEmptyFolders,
		remove: func() error {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) && !notEmpty(path) {
				return err
			}
			return nil
		},
		gone: func() bool { return !exists(path) || notEmpty(path) }}
}

func notEmpty(p string) bool {
	entries, err := os.ReadDir(p)
	return err == nil && len(entries) > 0
}
