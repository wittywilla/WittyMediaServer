package server

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// The built-in folder browser: the server's own drives and folders, for
// choosing library folders from any device (the browser may be on another
// computer than the files). Read-only: it lists, it never changes anything.
// Administrators only.

type fsEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Dir      bool   `json:"dir"`
	Kind     string `json:"kind,omitempty"` // files: video, audio, book, disc, subtitle, image, other
	Disc     string `json:"disc,omitempty"` // folders holding a DVD (VIDEO_TS) or Blu-ray (BDMV)
	Size     int64  `json:"size,omitempty"`
	Modified int64  `json:"modified,omitempty"`
	Hidden   bool   `json:"hidden,omitempty"`
}

type fsPlace struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Group string `json:"group"` // drives, folders, libraries
	Label string `json:"label,omitempty"`
	Free  uint64 `json:"free,omitempty"`
	Total uint64 `json:"total,omitempty"`
}

// At most this many entries are listed; very large folders say so.
const fsMaxEntries = 3000

// GET /api/fs?path=&hidden=1
// Without a path: the places to start from (drives, your folders, folders
// already in libraries). With a path: its folders and files, and a count of
// the media in it.
func (s *Server) handleFS(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	showHidden := r.URL.Query().Get("hidden") == "1"
	sep := string(filepath.Separator)
	if p == "" {
		writeJSON(w, 200, map[string]any{"path": "", "parent": nil, "sep": sep, "places": s.fsPlaces(r)})
		return
	}
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" && len(p) == 2 && p[1] == ':' {
		p += `\` // "D:" means the drive's root
	}
	st, err := os.Stat(p)
	if err != nil {
		writeErr(w, 404, "That folder can't be found, or the server isn't allowed to open it.")
		return
	}
	if !st.IsDir() {
		p = filepath.Dir(p)
	}
	des, err := os.ReadDir(p)
	if err != nil {
		writeErr(w, 403, "The server isn't allowed to open this folder.")
		return
	}
	entries := []fsEntry{}
	counts := map[string]int{}
	truncated := false
	for _, de := range des {
		name := de.Name()
		hidden := strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") || isHiddenSystem(name)
		if hidden && !showHidden {
			continue
		}
		if len(entries) >= fsMaxEntries {
			truncated = true
			break
		}
		full := filepath.Join(p, name)
		e := fsEntry{Name: name, Path: full, Dir: de.IsDir(), Hidden: hidden}
		if info, err := de.Info(); err == nil {
			e.Modified = info.ModTime().Unix()
			if !e.Dir {
				e.Size = info.Size()
			}
		}
		if e.Dir {
			counts["folder"]++
			switch {
			case exists(filepath.Join(full, "VIDEO_TS")) || strings.EqualFold(name, "VIDEO_TS"):
				e.Disc = "dvd"
			case exists(filepath.Join(full, "BDMV")) || strings.EqualFold(name, "BDMV"):
				e.Disc = "bluray"
			}
		} else {
			e.Kind = library.FileKind(name)
			counts[e.Kind]++
		}
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Dir != entries[j].Dir {
			return entries[i].Dir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	var parent *string
	if d := filepath.Dir(p); d != p {
		parent = &d
	}
	writeJSON(w, 200, map[string]any{"path": p, "parent": parent, "sep": sep, "entries": entries, "counts": counts, "truncated": truncated})
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// isHiddenSystem: Windows folders nobody keeps media in.
func isHiddenSystem(name string) bool {
	switch strings.ToLower(name) {
	case "system volume information", "recovery", "config.msi", "pagefile.sys", "hiberfil.sys", "swapfile.sys", "dumpstack.log.tmp":
		return true
	}
	return false
}

func (s *Server) fsPlaces(r *http.Request) []fsPlace {
	var out []fsPlace
	add := func(name, path, group string) {
		if st, err := os.Stat(path); err == nil && st.IsDir() {
			pl := fsPlace{Name: name, Path: path, Group: group}
			if group == "drives" {
				pl.Label, pl.Free, pl.Total = diskInfo(path)
			}
			out = append(out, pl)
		}
	}
	if runtime.GOOS == "windows" {
		for c := 'A'; c <= 'Z'; c++ {
			add(string(c)+":", string(c)+`:\`, "drives")
		}
	} else {
		add("/", "/", "drives")
		for _, base := range []string{"/media", "/mnt", "/Volumes", "/run/media"} {
			des, _ := os.ReadDir(base)
			for _, d := range des {
				full := filepath.Join(base, d.Name())
				if base == "/run/media" { // /run/media/<user>/<volume>
					sub, _ := os.ReadDir(full)
					for _, v := range sub {
						add(v.Name(), filepath.Join(full, v.Name()), "drives")
					}
					continue
				}
				add(d.Name(), full, "drives")
			}
		}
	}
	if h, err := os.UserHomeDir(); err == nil {
		add("Home", h, "folders")
		for _, d := range []string{"Desktop", "Videos", "Movies", "Music", "Pictures", "Documents", "Downloads"} {
			add(d, filepath.Join(h, d), "folders")
		}
	}
	if libs, err := s.Store.Libraries(r.Context()); err == nil {
		seen := map[string]bool{}
		for _, l := range libs {
			for _, lp := range l.Paths {
				if !seen[lp] {
					seen[lp] = true
					add(l.Name+": "+filepath.Base(lp), lp, "libraries")
				}
			}
		}
	}
	if out == nil {
		out = []fsPlace{}
	}
	return out
}
