// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"os"
	"path/filepath"
	"strconv"
)

// A person's controller layouts, profiles and controller-screen picture are WittyPlay's own
// (module API 5: its routes at /api/modules/wittyplay/consoles/..., /game-profiles, /game-background).

// removeUserGameFiles is for when an account goes: its rows go with it, and this is what
// they don't cover. An account's number can be used again by the next one made, which
// mustn't find any of this.
func (s *Server) removeUserGameFiles(userID int64) {
	g := s.games()
	if sess := g.mgr.ForUser(userID); sess != nil {
		sess.Stop() // it would save into what's about to go
	}
	dir := "u" + strconv.FormatInt(userID, 10)
	_ = os.RemoveAll(filepath.Join(g.dir, "saves", dir))
	_ = os.RemoveAll(filepath.Join(g.dir, "bezels", dir))
	s.forgetCouchUser(userID)
	r := s.rigs()
	r.mu.Lock()
	for id, st := range r.starter {
		if st.user == userID {
			delete(r.starter, id)
		}
	}
	r.mu.Unlock()
}
