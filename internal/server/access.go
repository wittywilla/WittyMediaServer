package server

import (
	"context"
	"encoding/json"
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
	if u.Restrictions == "" {
		if u.Kind == auth.KindAdult || u.Kind == "" {
			return nil
		}
		a := library.DefaultAccess(u.Kind) // Kids/Guest are never unrestricted
		return &a
	}
	a := library.DefaultAccess(u.Kind)
	if err := json.Unmarshal([]byte(u.Restrictions), &a); err != nil {
		a = library.DefaultAccess(u.Kind)
		if u.Kind == auth.KindAdult {
			a.MaxAge = 0 // unreadable limits must fail closed
		}
	}
	return &a
}

func (s *Server) access(r *http.Request) *library.Access { return accessFor(userFrom(r)) }

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
// this machine (folder names, often an account name). A podcast episode's
// path is a web address and stays: players and the page need it.
func (s *Server) forViewer(r *http.Request, items ...*library.Item) []*library.Item {
	if !userFrom(r).IsAdmin {
		for _, it := range items {
			if !isWeb(it.Path) {
				it.Path = ""
			}
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

// visibleLibraries drops libraries an account may not open.
func (s *Server) visibleLibraries(ctx context.Context, a *library.Access) ([]library.Library, error) {
	libs, err := s.Store.Libraries(ctx)
	if err != nil || a == nil || len(a.Libraries) == 0 {
		return libs, err
	}
	out := libs[:0]
	for _, l := range libs {
		for _, id := range a.Libraries {
			if l.ID == id {
				out = append(out, l)
				break
			}
		}
	}
	return out, nil
}

// hidePaths: where files live on the server is for administrators (the
// folders can name people). Web addresses of podcast episodes stay: the
// player and the download button read them.
func hidePaths(r *http.Request, items ...*library.Item) {
	if u := userFrom(r); u != nil && u.IsAdmin {
		return
	}
	for _, it := range items {
		if it != nil && !strings.HasPrefix(it.Path, "http://") && !strings.HasPrefix(it.Path, "https://") {
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
