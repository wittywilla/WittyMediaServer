package server

import (
	"net/http"
	"path/filepath"

	fb "github.com/wittywilla/wittyintegratedfilebrowser"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// The built-in folder browser (WittyIntegratedFileBrowser): the server's own
// drives and folders, for choosing library folders from any device (the
// browser may be on another computer than the files). Read-only.
// Administrators only.

// GET /api/fs?path=&hidden=1
// Without a path: the places to start from (drives, your folders, folders
// already in libraries). With a path: its folders and files, and a count of
// the media in it.
func (s *Server) handleFS(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if p == "" {
		var libs []fb.Place
		if ls, err := s.Store.Libraries(r.Context()); err == nil {
			seen := map[string]bool{}
			for _, l := range ls {
				for _, lp := range l.Paths {
					if !seen[lp] {
						seen[lp] = true
						libs = append(libs, fb.Place{Name: l.Name + ": " + filepath.Base(lp), Path: lp, Group: "libraries"})
					}
				}
			}
		}
		writeJSON(w, 200, map[string]any{"path": "", "parent": nil, "sep": string(filepath.Separator), "places": fb.Places(libs...)})
		return
	}
	l, err := fb.List(p, fb.Options{ShowHidden: r.URL.Query().Get("hidden") == "1", Kind: library.FileKind})
	switch err {
	case nil:
		writeJSON(w, 200, l)
	case fb.ErrNotFound:
		writeErr(w, 404, "That folder can't be found, or the server isn't allowed to open it.")
	default:
		writeErr(w, 403, "The server isn't allowed to open this folder.")
	}
}
