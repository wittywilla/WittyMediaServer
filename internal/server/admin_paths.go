// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"errors"
	"net/http"
	"slices"
	"strings"
)

// Paths an administrator gives from a browser. The account isn't meant to have the machine: on
// Windows, opening \\host\share sends the server's own credentials to that host, the machine's
// when it runs as a service, and nothing about a stolen session should be able to do that. Someone
// at the server's own desktop may, and so may a server whose owner set allowNetworkPaths.

var errOtherMachine = errors.New("That folder is on another machine, and the server would sign in there with its own account, so it can't be chosen from this device. Add it on the server's own desktop, give it a drive letter or mount it on the server, or, if this is your own storage, set allowNetworkPaths in the server's config file.")

// writePathErr answers with why a path was refused. A refusal for being on another machine has a
// code, so a page can lay out the ways forward rather than just repeat the sentence.
func writePathErr(w http.ResponseWriter, status int, err error) {
	if errors.Is(err, errOtherMachine) {
		writeJSON(w, status, map[string]any{"error": err.Error(), "code": "other-machine"})
		return
	}
	writeErr(w, status, err.Error())
}

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
