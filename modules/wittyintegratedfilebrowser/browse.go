// Package wittyintegratedfilebrowser lists a computer's drives, folders and
// files for a folder browser shown in a web page, so folders can be chosen
// from another device than the one holding them. It only reads: nothing is
// created, renamed or deleted.
package wittyintegratedfilebrowser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Entry is one folder or file in a listing.
type Entry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Dir      bool   `json:"dir"`
	Kind     string `json:"kind,omitempty"` // files: what Options.Kind says (video, audio, book, disc, subtitle, image, other)
	Disc     string `json:"disc,omitempty"` // folders holding a DVD (VIDEO_TS) or Blu-ray (BDMV): "dvd" or "bluray"
	Size     int64  `json:"size,omitempty"`
	Modified int64  `json:"modified,omitempty"` // Unix seconds
	Hidden   bool   `json:"hidden,omitempty"`
}

// Place is somewhere to start browsing: a drive, a usual folder, or one the
// host adds (such as folders already in use).
type Place struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Group string `json:"group"` // drives, folders, or the host's own group name
	Label string `json:"label,omitempty"`
	Free  uint64 `json:"free,omitempty"`
	Total uint64 `json:"total,omitempty"`
}

// Listing is a folder's contents.
type Listing struct {
	Path      string         `json:"path"`
	Parent    *string        `json:"parent"` // nil at a drive's root
	Sep       string         `json:"sep"`
	Entries   []Entry        `json:"entries"`
	Counts    map[string]int `json:"counts"` // "folder" and each file kind
	Truncated bool           `json:"truncated"`
}

// Options change a listing.
type Options struct {
	ShowHidden bool                     // include dot-files and system folders
	MaxEntries int                      // 0 = 3000
	Kind       func(name string) string // classifies files; nil = FileKind
}

var (
	// ErrNotFound: the path does not exist (or cannot be seen).
	ErrNotFound = errors.New("folder not found")
	// ErrDenied: the folder exists but cannot be opened.
	ErrDenied = errors.New("not allowed to open this folder")
)

// List returns a folder's folders and files, folders first, by name. A path
// to a file lists the folder holding it; "D:" means the drive's root.
func List(path string, o Options) (*Listing, error) {
	p := filepath.Clean(strings.TrimSpace(path))
	if runtime.GOOS == "windows" && len(p) == 2 && p[1] == ':' {
		p += `\`
	}
	st, err := os.Stat(p)
	if err != nil {
		return nil, ErrNotFound
	}
	if !st.IsDir() {
		p = filepath.Dir(p)
	}
	des, err := os.ReadDir(p)
	if err != nil && len(des) == 0 {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, ErrDenied
	}
	max := o.MaxEntries
	if max <= 0 {
		max = 3000
	}
	kind := o.Kind
	if kind == nil {
		kind = FileKind
	}
	l := &Listing{Path: p, Sep: string(filepath.Separator), Entries: []Entry{}, Counts: map[string]int{}}
	for _, de := range des {
		name := de.Name()
		hidden := IsHidden(name)
		if hidden && !o.ShowHidden {
			continue
		}
		if len(l.Entries) >= max {
			l.Truncated = true
			break
		}
		full := filepath.Join(p, name)
		e := Entry{Name: name, Path: full, Dir: de.IsDir(), Hidden: hidden}
		if info, err := de.Info(); err == nil {
			e.Modified = info.ModTime().Unix()
			if !e.Dir {
				e.Size = info.Size()
			}
		}
		if e.Dir {
			l.Counts["folder"]++
			e.Disc = DiscFolder(full)
		} else {
			e.Kind = kind(name)
			l.Counts[e.Kind]++
		}
		l.Entries = append(l.Entries, e)
	}
	sort.SliceStable(l.Entries, func(i, j int) bool {
		a, b := l.Entries[i], l.Entries[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	if d := filepath.Dir(p); d != p {
		l.Parent = &d
	}
	return l, nil
}

// DiscFolder says whether a folder holds a disc's structure: "dvd" for a
// VIDEO_TS folder (or one holding it), "bluray" for BDMV, else "".
func DiscFolder(p string) string {
	name := filepath.Base(p)
	switch {
	case strings.EqualFold(name, "VIDEO_TS") || exists(filepath.Join(p, "VIDEO_TS")):
		return "dvd"
	case strings.EqualFold(name, "BDMV") || exists(filepath.Join(p, "BDMV")):
		return "bluray"
	}
	return ""
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// IsHidden: dot-files, $-folders and Windows system folders nobody keeps
// media in.
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

// Places lists where to start: the drives (with their free space), the
// user's usual folders, then extra in the order given. Only places that
// exist are returned.
func Places(extra ...Place) []Place {
	out := []Place{}
	add := func(pl Place) {
		if st, err := os.Stat(pl.Path); err == nil && st.IsDir() {
			if pl.Group == "drives" {
				pl.Label, pl.Free, pl.Total = diskInfo(pl.Path)
			}
			out = append(out, pl)
		}
	}
	if runtime.GOOS == "windows" {
		for c := 'A'; c <= 'Z'; c++ {
			add(Place{Name: string(c) + ":", Path: string(c) + `:\`, Group: "drives"})
		}
	} else {
		add(Place{Name: "/", Path: "/", Group: "drives"})
		for _, base := range []string{"/media", "/mnt", "/Volumes", "/run/media"} {
			des, _ := os.ReadDir(base)
			for _, d := range des {
				full := filepath.Join(base, d.Name())
				if base == "/run/media" { // /run/media/<user>/<volume>
					sub, _ := os.ReadDir(full)
					for _, v := range sub {
						add(Place{Name: v.Name(), Path: filepath.Join(full, v.Name()), Group: "drives"})
					}
					continue
				}
				add(Place{Name: d.Name(), Path: full, Group: "drives"})
			}
		}
	}
	if h, err := os.UserHomeDir(); err == nil {
		add(Place{Name: "Home", Path: h, Group: "folders"})
		for _, d := range []string{"Desktop", "Videos", "Movies", "Music", "Pictures", "Documents", "Downloads"} {
			add(Place{Name: d, Path: filepath.Join(h, d), Group: "folders"})
		}
	}
	for _, pl := range extra {
		add(pl)
	}
	return out
}

var kinds = map[string]string{}

func init() {
	for kind, exts := range map[string]string{
		"video":    ".mkv .mp4 .m4v .avi .mov .wmv .webm .ts .m2ts .mts .mpg .mpeg .vob .flv .ogv .3gp",
		"audio":    ".mp3 .flac .m4a .aac .ogg .opus .wav .wma .alac .aiff .ape .m4b",
		"book":     ".epub .pdf .cbz .cbr .cb7 .mobi .azw3",
		"disc":     ".iso .img",
		"subtitle": ".srt .ass .ssa .vtt .sub .idx .sup",
		"image":    ".jpg .jpeg .png .webp .gif",
	} {
		for _, e := range strings.Fields(exts) {
			kinds[e] = kind
		}
	}
}

// FileKind classifies a file by its extension: video, audio, book, disc,
// subtitle, image or other.
func FileKind(name string) string {
	if k := kinds[strings.ToLower(filepath.Ext(name))]; k != "" {
		return k
	}
	return "other"
}
