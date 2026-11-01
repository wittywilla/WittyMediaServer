package modules

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymodule"
)

const fileBrowserID = "wittyintegratedfilebrowser"

// maxPlaces is far more than any computer has, and stops a helper filling a page.
const maxPlaces = 1000

// What WittyIntegratedFileBrowser answers. They travel as JSON.
type (
	// Entry is one thing in a folder.
	Entry struct {
		Name     string `json:"name"`
		Path     string `json:"path"`
		Dir      bool   `json:"dir"`
		Kind     string `json:"kind,omitempty"`
		Disc     string `json:"disc,omitempty"`
		Size     int64  `json:"size,omitempty"`
		Modified int64  `json:"modified,omitempty"`
		Hidden   bool   `json:"hidden,omitempty"`
	}
	// Place is somewhere to start from.
	Place struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		Group string `json:"group"`
		Label string `json:"label,omitempty"`
		Free  uint64 `json:"free,omitempty"`
		Total uint64 `json:"total,omitempty"`
	}
	// Listing is a folder's contents.
	Listing struct {
		Path      string         `json:"path"`
		Parent    *string        `json:"parent"`
		Sep       string         `json:"sep"`
		Entries   []Entry        `json:"entries"`
		Counts    map[string]int `json:"counts"`
		Truncated bool           `json:"truncated"`
	}
	// ListOptions says how to list a folder.
	ListOptions struct {
		ShowHidden bool
		MaxEntries int
		Kind       func(name string) string // names what each file is
	}
)

var (
	// ErrFolderNotFound: there's no such folder.
	ErrFolderNotFound = errors.New("no such folder")
	// ErrFolderDenied: it isn't allowed to open that folder.
	ErrFolderDenied = errors.New("not allowed to open this folder")
)

// ErrMissing: a module this needs isn't on this server, or is turned off. The page says so and
// offers to install it, rather than showing a broken screen.
type ErrMissing struct {
	ID   string
	Name string
}

func (e *ErrMissing) Error() string { return e.Name + " isn't on this server." }

// missing is the ErrMissing for a module.
// Missing is the ErrMissing for a module.
func (m *Manager) Missing(id string) error { return m.missing(id) }

func (m *Manager) missing(id string) error {
	name := id
	if m != nil {
		m.mu.Lock()
		if e := m.mods[id]; e != nil {
			name = e.builtin.Name
		}
		m.mu.Unlock()
	}
	return &ErrMissing{ID: id, Name: name}
}

// FileBrowser lists the server's folders through the WittyIntegratedFileBrowser version in
// use. Without it, someone can still type a folder's path. M may be nil.
type FileBrowser struct{ M *Manager }

// Available says whether there's a version in use.
func (b FileBrowser) Available() bool { return b.M != nil && b.M.Installed(fileBrowserID) }

// Places lists where to start: the helper's drives and usual folders, then extra,
// which is what the server knows itself, such as folders already in libraries.
func (b FileBrowser) Places(extra ...Place) ([]Place, error) {
	if b.M == nil {
		return realPlaces(extra), nil
	}
	home, _ := os.UserHomeDir()
	var found []Place
	_, err, handled := b.M.call(fileBrowserID, "places", struct {
		Home string `json:"home"`
	}{home}, &found)
	if !handled {
		return realPlaces(extra), b.M.missing(fileBrowserID)
	}
	if err != nil {
		return realPlaces(extra), err
	}
	return append(realPlaces(found), realPlaces(extra)...), nil
}

// realPlaces keeps only folders that are there, as the server sees it.
func realPlaces(in []Place) []Place {
	out := make([]Place, 0, len(in))
	for _, p := range in {
		if len(out) == maxPlaces {
			break
		}
		if !filepath.IsAbs(p.Path) {
			continue
		}
		if st, err := os.Stat(p.Path); err == nil && st.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

// List lists a folder. o.Kind can't reach a helper, so the server names each file's
// kind itself, which also keeps the page's words the same whichever version is in use.
func (b FileBrowser) List(path string, o ListOptions) (*Listing, error) {
	if b.M == nil {
		return nil, (*Manager)(nil).missing(fileBrowserID)
	}
	blob, err, handled := b.M.call(fileBrowserID, "list", struct {
		Path       string `json:"path"`
		ShowHidden bool   `json:"showHidden"`
		MaxEntries int    `json:"maxEntries,omitempty"`
	}{path, o.ShowHidden, o.MaxEntries}, nil)
	if !handled {
		return nil, b.M.missing(fileBrowserID)
	}
	var ce *wittymodule.CallError
	if errors.As(err, &ce) {
		switch ce.Code {
		case "not-found":
			return nil, ErrFolderNotFound
		case "denied":
			return nil, ErrFolderDenied
		}
	}
	if err != nil {
		return nil, err
	}
	var l Listing
	if json.Unmarshal(blob, &l) != nil {
		return nil, errors.New("the file browser's answer is unreadable")
	}
	out := checkedListing(path, &l, o)
	if out == nil {
		return nil, errors.New("the file browser's answer doesn't match the folder asked for")
	}
	return out, nil
}

// IsHidden says whether a name is one a folder listing hides by default.
func IsHidden(name string) bool {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") {
		return true
	}
	switch strings.ToLower(name) {
	case "system volume information", "recovery", "config.msi", "pagefile.sys", "hiberfil.sys", "swapfile.sys", "dumpstack.log.tmp":
		return true
	}
	return false
}

// checkedListing makes what a helper said into a true answer about the folder asked
// for, or nil if it can't be. A listing only names things,
// but the page offers those names as paths to open, so they must be the ones asked for.
func checkedListing(asked string, l *Listing, o ListOptions) *Listing {
	if !isFolderOf(asked, l.Path) {
		return nil
	}
	limit := o.MaxEntries
	if limit <= 0 {
		limit = 3000
	}
	kind := o.Kind
	if kind == nil {
		kind = func(string) string { return "" }
	}
	out := &Listing{Path: l.Path, Sep: string(filepath.Separator), Entries: make([]Entry, 0, len(l.Entries)), Counts: map[string]int{}, Truncated: l.Truncated}
	if d := filepath.Dir(l.Path); d != l.Path {
		out.Parent = &d
	}
	for _, e := range l.Entries {
		if e.Name == "" || e.Name == "." || e.Name == ".." || filepath.Base(e.Name) != e.Name || e.Path != filepath.Join(l.Path, e.Name) {
			return nil
		}
		e.Hidden = IsHidden(e.Name)
		if e.Hidden && !o.ShowHidden {
			continue
		}
		if len(out.Entries) >= limit {
			out.Truncated = true
			break
		}
		if e.Disc != "dvd" && e.Disc != "bluray" {
			e.Disc = ""
		}
		if e.Dir {
			e.Kind = ""
			out.Counts["folder"]++
		} else {
			e.Kind, e.Disc = kind(e.Name), ""
			out.Counts[e.Kind]++
		}
		out.Entries = append(out.Entries, e)
	}
	return out
}

// isFolderOf says whether folder is the path asked for, or the folder holding it when
// a file was asked for, as a listing's path may be spelled differently from the request.
func isFolderOf(asked, folder string) bool {
	if !filepath.IsAbs(folder) {
		return false
	}
	a, err := os.Stat(asked)
	if err != nil {
		return false
	}
	if !a.IsDir() {
		if a, err = os.Stat(filepath.Dir(filepath.Clean(asked))); err != nil {
			return false
		}
	}
	f, err := os.Stat(folder)
	return err == nil && f.IsDir() && os.SameFile(a, f)
}
