package server

import (
	"errors"
	"net/http"
	"path/filepath"

	"github.com/wittywilla/wittymediaserver/internal/library"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
)

// The folder browser (WittyIntegratedFileBrowser): the server's own drives and folders, for
// choosing library folders from any device (the browser may be on another computer than the
// files). Read-only. Administrators only. Without the module, the page asks for a path.

// GET /api/fs?path=&hidden=1
// Without a path: the places to start from (drives, your folders, folders
// already in libraries). With a path: its folders and files, and a count of
// the media in it.
func (s *Server) handleFS(w http.ResponseWriter, r *http.Request) {
	browser := modmgr.FileBrowser{M: s.Modules}
	p := r.URL.Query().Get("path")
	if p == "" {
		var libs []modmgr.Place
		if ls, err := s.Store.Libraries(r.Context()); err == nil {
			seen := map[string]bool{}
			for _, l := range ls {
				for _, lp := range l.Paths {
					if !seen[lp] {
						seen[lp] = true
						libs = append(libs, modmgr.Place{Name: l.Name + ": " + filepath.Base(lp), Path: lp, Group: "libraries"})
					}
				}
			}
		}
		places, err := browser.Places(libs...)
		// The folders already in libraries are still worth showing, along with what's missing.
		var missing *modmgr.ErrMissing
		errors.As(err, &missing)
		out := map[string]any{"path": "", "parent": nil, "sep": string(filepath.Separator), "places": places}
		if missing != nil {
			out["module"], out["moduleName"] = missing.ID, missing.Name
		}
		writeJSON(w, 200, out)
		return
	}
	if err := s.checkAdminPaths(r, []string{p}, nil); err != nil {
		writePathErr(w, 403, err)
		return
	}
	l, err := browser.List(p, modmgr.ListOptions{ShowHidden: r.URL.Query().Get("hidden") == "1", Kind: library.FileKind})
	switch {
	case err == nil:
		writeJSON(w, 200, l)
	case errors.Is(err, modmgr.ErrFolderNotFound):
		writeErr(w, 404, "That folder can't be found, or the server isn't allowed to open it.")
	case writeModuleMissing(w, err):
	default:
		writeErr(w, 403, "The server isn't allowed to open this folder.")
	}
}
