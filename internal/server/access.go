// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Per-account access: which libraries, ratings and titles an account may
// see and play. Administrators and accounts nobody restricted get nil (no
// limits, no extra SQL). Hidden titles answer "not found", so an account
// cannot even learn that they exist.

// accessFor returns a user's limits (nil = unrestricted).
func accessFor(u *auth.User) *library.Access {
	if u == nil || u.IsAdmin {
		return nil
	}
	if u.Restrictions == "" && (u.Kind == auth.KindAdult || u.Kind == "") {
		return nil
	}
	a := library.DefaultAccess(u.Kind) // Kids/Guest are never unrestricted
	if u.Restrictions != "" {
		if err := json.Unmarshal([]byte(u.Restrictions), &a); err != nil {
			a = library.DefaultAccess(u.Kind)
			if u.Kind == auth.KindAdult {
				a.MaxAge = 0 // unreadable limits must fail closed
			}
		}
	}
	a.ForKind(u.Kind)
	return &a
}

// access is the signed-in account's limits, ready to answer for one title.
func (s *Server) access(r *http.Request) *library.Access { return s.accessOf(r.Context(), userFrom(r)) }

// visibleItem loads an item the signed-in account may see.
func (s *Server) visibleItem(r *http.Request, id int64) (*library.Item, error) {
	it, err := s.Store.Item(r.Context(), id)
	if err != nil {
		return nil, err
	}
	a := s.access(r)
	if a == nil {
		return it, nil
	}
	var sh *library.Show
	if it.ShowID != nil {
		sh, _ = s.Store.ShowFacts(r.Context(), *it.ShowID)
	}
	if !a.AllowsItem(it, sh) {
		s.noteHidden(r, it, sh) // whoever manages this profile can see what it asked for
		return nil, library.ErrNotFound
	}
	return it, nil
}

// visibleShow loads a show the signed-in account may see.
func (s *Server) visibleShow(r *http.Request, id int64) (*library.Show, error) {
	sh, err := s.Store.Show(r.Context(), id, userFrom(r).ID)
	if err != nil {
		return nil, err
	}
	if !s.access(r).AllowsShow(sh) {
		return nil, library.ErrNotFound
	}
	return sh, nil
}

// forViewer takes out what only an administrator needs: where a file is on
// this machine (folder names, often an account name), and the web addresses of
// radio stations and podcast episodes, which can hold a private key. Both play
// through the server.
func (s *Server) forViewer(r *http.Request, items ...*library.Item) []*library.Item {
	if !userFrom(r).IsAdmin {
		for _, it := range items {
			it.Path = ""
			it.Poster, it.Backdrop = artForViewer(it.Poster), artForViewer(it.Backdrop)
		}
	}
	return items
}

func (s *Server) showsForViewer(r *http.Request, shows ...*library.Show) []*library.Show {
	if !userFrom(r).IsAdmin {
		for _, sh := range shows {
			sh.Path = ""
			sh.Poster, sh.Backdrop = artForViewer(sh.Poster), artForViewer(sh.Backdrop)
		}
	}
	return shows
}

func isWeb(s string) bool { return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") }

// artForViewer: the page asks the server for pictures and only needs to know
// whether there is one, so a picture beside the media doesn't name its folder.
func artForViewer(v string) string {
	if v == "" || isWeb(v) || strings.HasPrefix(v, "cache:") {
		return v
	}
	return "local"
}

// fileFailed is what someone who doesn't manage this server is told when a file can't be read.
const fileFailed = "This file can't be read right now. It may have moved, or be damaged."

// viewerError is what to tell the person asking about a file that failed. Errors from the
// system and from FFmpeg name where the file is, which is for administrators; a refusal such
// as "this disc is encrypted" names nothing and helps, so it stays. The full text is in the log.
func (s *Server) viewerError(r *http.Request, err error) string {
	msg := err.Error()
	if u := userFrom(r); u != nil && u.IsAdmin {
		return msg
	}
	var pe *fs.PathError
	if errors.As(err, &pe) || (s.Cfg.DataDir != "" && strings.Contains(msg, s.Cfg.DataDir)) {
		s.Log.WarnContext(r.Context(), "a file couldn't be read", "cat", "library", "err", err)
		return fileFailed
	}
	for _, root := range s.Store.Roots(r.Context()) {
		if strings.Contains(msg, root) {
			s.Log.WarnContext(r.Context(), "a file couldn't be read", "cat", "library", "err", err)
			return fileFailed
		}
	}
	return msg
}

// visibleLibraries drops libraries an account may not open: those its limits
// leave out, and the games libraries of an account that may not play games.
func (s *Server) visibleLibraries(ctx context.Context, a *library.Access) ([]library.Library, error) {
	libs, err := s.Store.Libraries(ctx)
	if err != nil || a == nil || (len(a.Libraries) == 0 && a.Can(library.FeatureGames)) {
		return libs, err
	}
	out := libs[:0]
	for _, l := range libs {
		if a.AllowsLibrary(l.ID) && (l.Type != "games" || a.Can(library.FeatureGames)) {
			out = append(out, l)
		}
	}
	return out, nil
}

// hidePaths: where files live on the server is for administrators (the
// folders can name people), and so are episode addresses, which can hold a key.
func hidePaths(r *http.Request, items ...*library.Item) {
	if u := userFrom(r); u != nil && u.IsAdmin {
		return
	}
	for _, it := range items {
		if it != nil {
			it.Path = ""
		}
	}
}

// filterItems drops items an account may not see (small lists only, such
// as Continue Watching; large lists are filtered in SQL).
func (s *Server) filterItems(ctx context.Context, a *library.Access, items []*library.Item) []*library.Item {
	if a == nil {
		return items
	}
	out := items[:0]
	for _, it := range items {
		var sh *library.Show
		if it.ShowID != nil {
			sh, _ = s.Store.ShowFacts(ctx, *it.ShowID)
		}
		if a.AllowsItem(it, sh) {
			out = append(out, it)
		}
	}
	return out
}
