package modules

import (
	"encoding/json"
	"os"
	"path/filepath"

	fb "github.com/wittywilla/wittyintegratedfilebrowser"
)

const fileBrowserID = "wittyintegratedfilebrowser"

// maxPlaces is far more than any computer has, and stops a helper filling a page.
const maxPlaces = 1000

// FileBrowser lists the server's folders through the WittyIntegratedFileBrowser
// version in use, and through the built-in copy when that is the one in use. Any
// refusal from a helper also goes to the built-in copy, since a helper's limits may
// be why it can't read a folder, so a folder is only ever called missing or closed
// by the server itself. M may be nil.
type FileBrowser struct{ M *Manager }

// Places lists where to start: the helper's drives and usual folders, then extra,
// which is what the server knows itself, such as folders already in libraries.
func (b FileBrowser) Places(extra ...fb.Place) []fb.Place {
	if b.M != nil {
		home, _ := os.UserHomeDir()
		var found []fb.Place
		_, err, handled := b.M.call(fileBrowserID, "places", struct {
			Home string `json:"home"`
		}{home}, &found)
		if handled && err == nil {
			if found = realPlaces(found); len(found) > 0 {
				return append(found, realPlaces(extra)...)
			}
		}
	}
	return fb.Places(extra...)
}

// realPlaces keeps only folders that are there, as the server sees it.
func realPlaces(in []fb.Place) []fb.Place {
	out := make([]fb.Place, 0, len(in))
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
// kind itself, which also keeps the page's words the same whichever copy is in use.
func (b FileBrowser) List(path string, o fb.Options) (*fb.Listing, error) {
	if l := b.fromHelper(path, o); l != nil {
		return l, nil
	}
	return fb.List(path, o)
}

func (b FileBrowser) fromHelper(path string, o fb.Options) *fb.Listing {
	if b.M == nil {
		return nil
	}
	blob, err, handled := b.M.call(fileBrowserID, "list", struct {
		Path       string `json:"path"`
		ShowHidden bool   `json:"showHidden"`
		MaxEntries int    `json:"maxEntries,omitempty"`
	}{path, o.ShowHidden, o.MaxEntries}, nil)
	if !handled || err != nil {
		return nil
	}
	var l fb.Listing
	if json.Unmarshal(blob, &l) != nil {
		return nil
	}
	return checkedListing(path, &l, o)
}

// checkedListing makes what a helper said into what the built-in copy would have
// answered about the same folder, or nil if it can't be. A listing only names things,
// but the page offers those names as paths to open, so they must be the ones asked for.
func checkedListing(asked string, l *fb.Listing, o fb.Options) *fb.Listing {
	if !isFolderOf(asked, l.Path) {
		return nil
	}
	limit := o.MaxEntries
	if limit <= 0 {
		limit = 3000
	}
	kind := o.Kind
	if kind == nil {
		kind = fb.FileKind
	}
	out := &fb.Listing{Path: l.Path, Sep: string(filepath.Separator), Entries: make([]fb.Entry, 0, len(l.Entries)), Counts: map[string]int{}, Truncated: l.Truncated}
	if d := filepath.Dir(l.Path); d != l.Path {
		out.Parent = &d
	}
	for _, e := range l.Entries {
		if e.Name == "" || e.Name == "." || e.Name == ".." || filepath.Base(e.Name) != e.Name || e.Path != filepath.Join(l.Path, e.Name) {
			return nil
		}
		e.Hidden = fb.IsHidden(e.Name)
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
