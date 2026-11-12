// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Cast and crew (docs/METADATA.md, "Cast and crew"; docs/CLIENT-API.md, "People"). A title's
// credits are read only after the title itself was found visible to the account; a person, their
// picture and their titles only when they are credited on something the account may see, and
// their filmography names nothing else. A person credited only on hidden titles is not found.

func (s *Server) peopleRoutes(m *http.ServeMux) {
	a := s.requireUser
	m.Handle("GET /api/items/{id}/credits", a(s.handleItemCredits))
	m.Handle("GET /api/shows/{id}/credits", a(s.handleShowCredits))
	m.Handle("GET /api/people", a(s.handlePeopleSearch))
	m.Handle("GET /api/people/{id}", a(s.handlePerson))
	m.Handle("GET /api/people/{id}/photo", a(s.handlePersonPhoto))
}

// GET /api/items/{id}/credits: a film's or an episode's cast, guest stars and crew.
func (s *Server) handleItemCredits(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil {
		writeErr(w, 404, "item not found")
		return
	}
	c, err := s.Store.ItemCredits(r.Context(), it)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, c)
}

// GET /api/shows/{id}/credits?season=: a show's cast and crew, with that season's guest stars.
func (s *Server) handleShowCredits(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if _, err := s.visibleShow(r, id); err != nil {
		writeErr(w, 404, "show not found")
		return
	}
	season := -1
	if v := r.URL.Query().Get("season"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 10000 {
			writeErr(w, 400, "season must be a season's number")
			return
		}
		season = n
	}
	c, err := s.Store.ShowCredits(r.Context(), id, season, s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, c)
}

// GET /api/people?search=&limit=: people credited on titles the account may see, by name.
func (s *Server) handlePeopleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("search"))
	if q == "" {
		q = strings.TrimSpace(r.URL.Query().Get("q"))
	}
	if len(q) > 100 {
		q = q[:100]
	}
	found, err := s.Store.SearchPeople(r.Context(), q, min(max(qInt(r, "limit", 20), 1), 100), s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"people": found})
}

// visiblePerson loads a person the account may know of.
func (s *Server) visiblePerson(r *http.Request, id int64) (*library.Person, error) {
	p, err := s.Store.Person(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if !s.Store.PersonVisible(r.Context(), id, s.access(r)) {
		return nil, library.ErrNotFound
	}
	return p, nil
}

// personFresh fills a person in from TMDB first when that's due, for at most a few seconds:
// the page is shown with what is known if the answer is slow.
func (s *Server) personFresh(r *http.Request, p *library.Person) *library.Person {
	if s.Scanner == nil {
		return p
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	asked, err := s.Scanner.LookUpPerson(ctx, p)
	if err != nil {
		s.Log.DebugContext(r.Context(), "person lookup failed", "person", p.ID, "err", err)
	}
	if !asked {
		return p
	}
	if fresh, err := s.Store.Person(r.Context(), p.ID); err == nil {
		return fresh
	}
	return p
}

// GET /api/people/{id}: a person and their titles here, grouped by what they did.
func (s *Server) handlePerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	p, err := s.visiblePerson(r, id)
	if err != nil {
		writeErr(w, 404, "person not found")
		return
	}
	p = s.personFresh(r, p)
	films, err := s.Store.Filmography(r.Context(), id, s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"person": p, "filmography": films})
}

// GET /api/people/{id}/photo?w=: the person's picture, made smaller when asked.
func (s *Server) handlePersonPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	p, err := s.visiblePerson(r, id)
	if err != nil || p.Photo == "" {
		writeErr(w, 404, "no photo")
		return
	}
	s.serveArtSized(w, r, p.Photo)
}
