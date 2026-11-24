// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Live TV by programme. A channel a profile may watch can still show a programme it may not:
// a broadcast or IPTV channel's guide gives each programme's rating, and a library channel
// plays titles that each have theirs. So tuning in, every live playlist and every part of the
// stream asks what is on at that moment, and refuses when the profile's TV limit doesn't
// allow it. A stream that was allowed stops at the next part once the programme changes to
// one that isn't: the player is told why, and the profile's report says what was refused.
//
// A broadcast programme the guide gives no rating is unrated, which the profile's TV limit
// decides like any unrated title ("allow titles with no rating").

// liveProgramme is what is on a channel at one moment, as far as limits care.
type liveProgramme struct {
	start, end int64
	title      string
	rating     string
	itemID     int64 // a library channel's title, 0 for broadcasts and fillers
}

// liveVerdict is one profile's answer for one programme, kept until the programme ends.
type liveVerdict struct {
	start, end int64
	ok         bool
	asked      time.Time
}

const (
	liveVerdictFor = time.Minute // limits changed since are forgotten by forgetRules anyway
	maxLiveVerdict = 1024
)

// programmeAt looks up what is on. kind is "tun" (tuner and IPTV) or "lib" (library channels).
func (s *Server) programmeAt(r *http.Request, kind string, id int64, at time.Time) (liveProgramme, error) {
	u := at.Unix()
	if kind == "tun" {
		if s.TV == nil {
			return liveProgramme{}, errors.New("no live TV")
		}
		c, err := s.TV.Channel(r.Context(), id)
		if err != nil {
			return liveProgramme{}, err
		}
		list, err := s.TV.Guide(r.Context(), c, at, at.Add(time.Second))
		if err != nil {
			return liveProgramme{}, err
		}
		for _, p := range list {
			if p.Start <= u && p.End > u {
				return liveProgramme{start: p.Start, end: p.End, title: p.Title, rating: p.Rating}, nil
			}
		}
		// Nothing in the guide: an unrated stretch, asked about again in a minute.
		return liveProgramme{start: u, end: u + 60}, nil
	}
	c, err := s.Channels.Get(r.Context(), id)
	if err != nil {
		return liveProgramme{}, err
	}
	cur, _, err := s.Channels.Now(r.Context(), c, at)
	if err != nil || cur == nil {
		return liveProgramme{start: u, end: u + 60}, err
	}
	return liveProgramme{start: cur.Start, end: cur.End, title: cur.Title, rating: cur.Rating, itemID: cur.ItemID}, nil
}

// liveAllowed says whether this profile may watch what is on the channel at at. Accounts with
// no limits never ask.
func (s *Server) liveAllowed(r *http.Request, kind string, id int64, at time.Time) (ok bool, p liveProgramme, err error) {
	u := userFrom(r)
	a := s.access(r)
	if a == nil || u == nil {
		return true, p, nil
	}
	key := fmt.Sprintf("%d|%s|%s|%d", u.ID, u.Restrictions, kind, id)
	now := time.Now()
	st := s.parentalInit()
	st.mu.Lock()
	if v, hit := st.live[key]; hit && v.start <= at.Unix() && at.Unix() < v.end && now.Sub(v.asked) < liveVerdictFor {
		st.mu.Unlock()
		return v.ok, liveProgramme{start: v.start, end: v.end}, nil
	}
	st.mu.Unlock()
	if p, err = s.programmeAt(r, kind, id, at); err != nil {
		return false, p, err
	}
	switch {
	case p.itemID != 0:
		// A library title: every rule the profile has, as anywhere else it is asked for.
		_, ierr := s.visibleItem(r, p.itemID)
		if ierr != nil && !errors.Is(ierr, library.ErrNotFound) {
			return false, p, ierr
		}
		ok = ierr == nil
	default:
		ok = a.AllowsOutside("episode", p.rating, nil)
	}
	st.mu.Lock()
	if st.live == nil || len(st.live) >= maxLiveVerdict {
		st.live = map[string]liveVerdict{}
	}
	st.live[key] = liveVerdict{start: p.start, end: p.end, ok: ok, asked: now}
	st.mu.Unlock()
	return ok, p, nil
}

// liveRefused answers the request itself when what is on isn't for this profile, and says so
// with when it may look again (the programme's end).
func (s *Server) liveRefused(w http.ResponseWriter, r *http.Request, kind string, id int64, at time.Time) bool {
	ok, p, err := s.liveAllowed(r, kind, id, at)
	if err != nil {
		if s.access(r) == nil {
			return false
		}
		// Not knowing what is on is not a reason to show it.
		writeJSON(w, 403, map[string]any{"error": "What is on now can't be checked for this profile.", "code": "programme-rating"})
		return true
	}
	if ok {
		return false
	}
	if p.title != "" || p.rating != "" {
		s.noteRefused(r.Context(), userFrom(r), "blocked", p.itemID, p.title, p.rating)
	}
	w.Header().Set("Retry-After", strconv.FormatInt(max(p.end-time.Now().Unix(), 1), 10))
	writeJSON(w, 403, map[string]any{"error": "What is on now is not available to this profile.", "code": "programme-rating", "until": p.end})
	return true
}
