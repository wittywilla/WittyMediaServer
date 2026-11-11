// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/http"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// artworkCredit is one picture from Wikimedia Commons on a title, with what its licence asks to be said.
type artworkCredit struct {
	Title  string `json:"title"`
	Target string `json:"target"`
	ID     int64  `json:"id"`
	Kind   string `json:"kind"` // poster | backdrop
	library.CommonsPicture
}

// commonsWiki asks Commons about files. Tests answer in its place.
var commonsWiki = func() *library.Wikidata { return &library.Wikidata{} }

// GET /api/artwork/credits: where each poster or backdrop from Wikimedia Commons came from, its licence
// and author, for the titles this account may see. Commons is asked about the files, a while at a time.
func (s *Server) handleArtworkCredits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	titles, err := s.Store.CommonsArtwork(ctx, s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var addresses []string
	for _, t := range titles {
		addresses = append(addresses, t.Poster, t.Backdrop)
	}
	pics := library.CommonsCredits(ctx, commonsWiki(), addresses)
	out := []artworkCredit{}
	for _, t := range titles {
		for _, a := range []struct{ kind, url string }{{"poster", t.Poster}, {"backdrop", t.Backdrop}} {
			if p, ok := pics[a.url]; ok {
				out = append(out, artworkCredit{Title: t.Title, Target: t.Target, ID: t.ID, Kind: a.kind, CommonsPicture: p})
			}
		}
	}
	writeJSON(w, 200, map[string]any{"pictures": out})
}
