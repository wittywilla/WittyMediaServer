// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/search"
)

// searchState is the one search's index and what it was built from. Each library's
// part is rebuilt when its titles change: after a scan, or found by a look at the
// libraries' stamps at most once a minute while people are searching.
type searchState struct {
	once    sync.Once
	idx     *search.Index
	readyCh chan struct{} // closed once every library has been read
	ready   atomic.Bool
	running atomic.Bool
	again   atomic.Bool // asked again while running
	mu      sync.Mutex
	stamps  map[int64]library.SearchStamp
	checked time.Time
}

// searchStampsEvery is how often a search looks for changes no scan announced: an
// edit, a match, a library removed.
const searchStampsEvery = time.Minute

// searchIndexWait is how long a search waits for the index on a server that has
// just started, before giving up for now.
const searchIndexWait = 10 * time.Second

func (st *searchState) init() {
	st.once.Do(func() { st.idx, st.readyCh = search.New(), make(chan struct{}) })
}

// searchIndex is the index once it holds every library, else nil. It also asks for
// a look at the stamps when the last was long enough ago.
func (s *Server) searchIndex() *search.Index {
	st := &s.search
	st.init()
	st.mu.Lock()
	due := time.Since(st.checked) > searchStampsEvery
	st.mu.Unlock()
	if due {
		s.kickSearch()
	}
	if !st.ready.Load() {
		select {
		case <-st.readyCh:
		case <-time.After(searchIndexWait):
			return nil
		}
	}
	return st.idx
}

// kickSearch brings the index up to date in the background.
func (s *Server) kickSearch() {
	st := &s.search
	st.init()
	if !st.running.CompareAndSwap(false, true) {
		st.again.Store(true)
		return
	}
	logging.Go("search index", func() {
		defer st.running.Store(false)
		for {
			st.again.Store(false)
			if err := s.refreshSearch(context.Background()); err != nil {
				s.Log.Warn("could not update the search index", "cat", "library", "err", err)
			}
			if !st.again.Load() {
				return
			}
		}
	}, "cat", "library")
}

// refreshSearch rebuilds the parts of the index whose stamps changed.
func (s *Server) refreshSearch(ctx context.Context) error {
	st := &s.search
	st.mu.Lock()
	st.checked = time.Now()
	old := st.stamps
	st.mu.Unlock()
	stamps, err := s.Store.SearchStamps(ctx)
	if err != nil {
		return err
	}
	libs, err := s.Store.Libraries(ctx)
	if err != nil {
		return err
	}
	start, built, docs := time.Now(), 0, 0
	keep := map[int64]bool{0: true}
	for i := range libs {
		l := &libs[i]
		keep[l.ID] = true
		if old != nil && old[l.ID] == stamps[l.ID] && st.idx.Has(l.ID) {
			continue
		}
		list, err := s.Store.SearchDocs(ctx, l)
		if err != nil {
			return err
		}
		st.idx.Replace(l.ID, list)
		built++
		docs += len(list)
	}
	if old == nil || old[0] != stamps[0] || !st.idx.Has(0) {
		people, err := s.Store.PeopleDocs(ctx)
		if err != nil {
			return err
		}
		st.idx.Replace(0, people)
		built++
		docs += len(people)
	}
	for lib := range old {
		if !keep[lib] {
			st.idx.Drop(lib)
		}
	}
	st.mu.Lock()
	st.stamps = stamps
	st.mu.Unlock()
	if !st.ready.Swap(true) {
		close(st.readyCh)
	}
	if built > 0 {
		s.Log.Info("search index updated", "cat", "library", "parts", built, "docs", docs, "total", st.idx.Len(), "took", time.Since(start).Round(time.Millisecond))
	}
	return nil
}

// Recent searches are kept for each profile, newest first, in a setting of its own.
const recentSearchesKept = 10

func recentSearchesKey(user int64) string { return "searchRecent." + itoa(user) }

func (s *Server) recentSearches(ctx context.Context, user int64) []string {
	var out []string
	_ = json.Unmarshal([]byte(s.DB.GetSetting(ctx, recentSearchesKey(user), "[]")), &out)
	return out
}

// noteSearch keeps a search among the profile's recent ones. Typing asks again every
// few letters, so a search that carries on from the newest one ("inter", then
// "interstellar") takes its place rather than adding to the list.
func (s *Server) noteSearch(ctx context.Context, user int64, q string) {
	q = strings.Join(strings.Fields(q), " ")
	if user == 0 || len([]rune(q)) < 2 || len(q) > 100 {
		return
	}
	s.search.mu.Lock() // one change to the list at a time
	defer s.search.mu.Unlock()
	list := s.recentSearches(ctx, user)
	if len(list) > 0 {
		newest := strings.ToLower(list[0])
		lq := strings.ToLower(q)
		if newest == lq {
			return
		}
		if strings.HasPrefix(lq, newest) || strings.HasPrefix(newest, lq) {
			list = list[1:]
		}
	}
	kept := []string{q}
	for _, v := range list {
		if !strings.EqualFold(v, q) && len(kept) < recentSearchesKept {
			kept = append(kept, v)
		}
	}
	b, _ := json.Marshal(kept)
	_ = s.DB.SetSetting(ctx, recentSearchesKey(user), string(b))
}

// handleRecentSearches answers GET (the profile's recent searches) and DELETE
// (forget them, or with ?q= just that one).
func (s *Server) handleRecentSearches(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if r.Method == http.MethodDelete {
		list := []string{}
		if q := r.URL.Query().Get("q"); q != "" {
			for _, v := range s.recentSearches(r.Context(), u.ID) {
				if v != q {
					list = append(list, v)
				}
			}
		}
		b, _ := json.Marshal(list)
		if err := s.DB.SetSetting(r.Context(), recentSearchesKey(u.ID), string(b)); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recent": s.recentSearches(r.Context(), u.ID)})
}
