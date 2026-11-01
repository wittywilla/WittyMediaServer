package server

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Paths an administrator gives from a browser. The account isn't meant to have the machine: on
// Windows, opening \\host\share sends the server's own credentials to that host, the machine's
// when it runs as a service, and nothing about a stolen session should be able to do that. Someone
// at the server's own desktop may, and so may a server whose owner set allowNetworkPaths.

var errOtherMachine = errors.New("That path is on another machine, and the server would sign in there with its own account. Use a drive letter or mount it on the server. Or, if this is your own NAS, make the change on the server's own desktop, or set allowNetworkPaths in its config file.")

// otherMachine: a path Windows takes to another machine or a device: \\host\share, //host/share,
// \\?\UNC\..., \\.\pipe\.
func otherMachine(p string) bool {
	return strings.HasPrefix(strings.ReplaceAll(strings.TrimSpace(p), `\`, "/"), "//")
}

// checkAdminPaths refuses paths on another machine, leaving out those already in use.
func (s *Server) checkAdminPaths(r *http.Request, paths, already []string) error {
	if s.Cfg.AllowNetworkPaths || onThisMachine(r) {
		return nil
	}
	for _, p := range paths {
		if otherMachine(p) && !slices.Contains(already, strings.TrimSpace(p)) {
			return errOtherMachine
		}
	}
	return nil
}

// checkSourcePlace is for a playlist or guide named as a file: read and partly shown back, so it has
// to come from where media and the server's own things are, or be chosen at the server itself. Web
// addresses are checked by whatever fetches them.
func (s *Server) checkSourcePlace(r *http.Request, place string) error {
	place = strings.TrimSpace(place)
	if place == "" || (strings.Contains(place, "://") && !otherMachine(place)) {
		return nil
	}
	if err := s.checkAdminPaths(r, []string{place}, nil); err != nil {
		return err
	}
	if otherMachine(place) {
		return nil
	}
	st, err := os.Stat(place)
	if err != nil || !st.Mode().IsRegular() {
		return errors.New("that file was not found on the server")
	}
	if onThisMachine(r) || s.Store.LibraryFile(r.Context(), place) || inDataFolder(s.Cfg.DataDir, place) {
		return nil
	}
	return errors.New("Keep this file in a library folder or the server's data folder, or add it on the server's own desktop.")
}

func inDataFolder(dir, path string) bool {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, real)
	return err == nil && filepath.IsLocal(rel)
}
