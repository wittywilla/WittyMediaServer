// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

// A person's saves, controller layouts, profiles and controller-screen picture are WittyPlay's
// own (module API 5): it hears of an account going from the host's events.

// removeUserGameFiles is for when an account goes: what its rows don't cover. An account's
// number can be used again by the next one made, which mustn't find any of this.
func (s *Server) removeUserGameFiles(userID int64) {
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
