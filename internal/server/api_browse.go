package server

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Browsing films and shows by anything, categories, saved views and streaming listings.

// GET /api/browse?library=&genre=&studio=&network=&year=&decade=&content=&minRating=&resolution=&audio=&subtitle=&format=&tag=&edition=&kind=&watched=&platform=&sort=&desc=&seed=&limit=&offset=
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	f := library.ParseBrowseFilter(r.URL.Query())
	if f.Library != 0 && !s.access(r).AllowsLibrary(f.Library) {
		writeErr(w, 404, "library not found")
		return
	}
	s.writeBrowse(w, r, f)
}

func (s *Server) writeBrowse(w http.ResponseWriter, r *http.Request, f library.BrowseFilter) {
	q := library.ShowQuery{LibraryID: f.Library}
	s.animeQuery(r.Context(), &q)
	f.AnimeOf, f.HideAnime = q.AnimeOf, q.HideAnime
	if f.Platform != "" && !s.Store.ListingsEnabled(r.Context()) {
		writeJSON(w, 200, library.BrowseResult{Results: []library.BrowseRow{}})
		return
	}
	res, err := s.Store.Browse(r.Context(), f, userFrom(r).ID, s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, row := range res.Results {
		if row.Item != nil {
			s.forViewer(r, row.Item)
		}
		if row.Show != nil {
			s.showsForViewer(r, row.Show)
		}
	}
	writeJSON(w, 200, res)
}

// GET /api/browse/facets?library=
func (s *Server) handleBrowseFacets(w http.ResponseWriter, r *http.Request) {
	lib := int64(qInt(r, "library", 0))
	if lib != 0 && !s.access(r).AllowsLibrary(lib) {
		writeErr(w, 404, "library not found")
		return
	}
	out, err := s.Store.LibraryFacets(r.Context(), lib, s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

// GET /api/browse/shelves?library=: what a library's page is made of, in one answer.
func (s *Server) handleBrowseShelves(w http.ResponseWriter, r *http.Request) {
	lib := int64(qInt(r, "library", 0))
	if lib != 0 && !s.access(r).AllowsLibrary(lib) {
		writeErr(w, 404, "library not found")
		return
	}
	ctx, acc := r.Context(), s.access(r)
	cats, err := s.Store.Categories(ctx, lib, acc, qBool(r, "all") && userFrom(r).IsAdmin)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	studios, err := s.Store.Brands(ctx, lib, library.LogoStudio, acc)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	networks, err := s.Store.Brands(ctx, lib, library.LogoNetwork, acc)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	collections, err := s.Store.CollectionsInLibrary(ctx, lib, userFrom(r).ID, acc)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	series, err := s.Store.SeriesInLibrary(ctx, lib, acc)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	collections = append(series, collections...)
	platforms, err := s.Store.ListingPlatforms(ctx, lib, acc)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"categories": cats, "studios": studios, "networks": networks, "collections": collections, "platforms": platforms})
}

// PUT /api/browse/categories {library, changes: [{key, name, hidden, mergeInto, artItem}], order: [key]}
func (s *Server) handleCategoriesPut(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Library int64                    `json:"library"`
		Changes []library.CategoryChange `json:"changes"`
		Order   []string                 `json:"order"`
		Reset   bool                     `json:"reset"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	if _, err := s.Store.Library(ctx, b.Library); err != nil {
		writeErr(w, 404, "library not found")
		return
	}
	var err error
	if b.Reset {
		err = s.Store.ResetCategories(ctx, b.Library)
	} else {
		err = s.Store.ChangeCategories(ctx, b.Library, b.Changes, b.Order)
	}
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cats, err := s.Store.Categories(ctx, b.Library, nil, true)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"categories": cats})
}

// Saved views belong to whoever saved them.

func (s *Server) handleViews(w http.ResponseWriter, r *http.Request) {
	out, err := s.Store.SavedViews(r.Context(), userFrom(r).ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"views": out})
}

func (s *Server) handleViewSave(w http.ResponseWriter, r *http.Request) {
	var v library.SavedView
	if err := readJSON(r, &v); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	v.ID = 0
	if id, ok := pathInt(r, "id"); ok {
		v.ID = id
	}
	if v.LibraryID != 0 && !s.access(r).AllowsLibrary(v.LibraryID) {
		writeErr(w, 404, "library not found")
		return
	}
	if err := s.Store.SaveView(r.Context(), userFrom(r).ID, &v); err != nil {
		if errors.Is(err, library.ErrNotFound) {
			writeErr(w, 404, "no such view")
			return
		}
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, v)
}

func (s *Server) handleViewDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if err := s.Store.DeleteView(r.Context(), userFrom(r).ID, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/views/{id}/titles?limit=&offset=&seed=: what a saved view finds now.
func (s *Server) handleViewTitles(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	views, err := s.Store.SavedViews(r.Context(), userFrom(r).ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, v := range views {
		if v.ID != id {
			continue
		}
		q, err := url.ParseQuery(v.Query)
		if err != nil {
			writeErr(w, 500, "that view can't be read")
			return
		}
		f := library.ParseBrowseFilter(q)
		f.Library = v.LibraryID
		f.Limit, f.Offset, f.Seed = qInt(r, "limit", 24), qInt(r, "offset", 0), qInt(r, "seed", 0)
		s.writeBrowse(w, r, f)
		return
	}
	writeErr(w, 404, "no such view")
}

// Streaming listings.

// GET /api/listings/status
func (s *Server) handleListingStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Scanner.ListingStatus(r.Context()))
}

// PUT /api/listings/settings {enabled, region, accepted: [...]}
func (s *Server) handleListingSettings(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Enabled  bool     `json:"enabled"`
		Region   string   `json:"region"`
		Accepted []string `json:"accepted"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.Scanner.SetListings(r.Context(), b.Enabled, b.Region, b.Accepted); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, s.Scanner.ListingStatus(r.Context()))
}

// POST /api/listings/refresh
func (s *Server) handleListingRefresh(w http.ResponseWriter, r *http.Request) {
	s.Scanner.KickListings()
	writeJSON(w, 202, s.Scanner.ListingStatus(r.Context()))
}

// GET /api/meta/{target}/{id}/listings: which services list one title.
func (s *Server) handleTitleListings(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	target := r.PathValue("target")
	switch target {
	case library.TargetItem:
		if _, err := s.visibleItem(r, id); err != nil {
			writeErr(w, 404, "not found")
			return
		}
	case library.TargetShow:
		if _, err := s.visibleShow(r, id); err != nil {
			writeErr(w, 404, "not found")
			return
		}
	default:
		writeErr(w, 404, "not found")
		return
	}
	list, at := s.Store.TitleListings(r.Context(), target, id)
	writeJSON(w, 200, map[string]any{"platforms": list, "asOf": at, "region": s.Store.ListingsRegion(r.Context()), "attribution": library.ListingAttribution})
}

// GET /api/meta/{target}/{id}/attached: extras and versions, for everyone who can see the title.
func (s *Server) handleTitleAttached(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	target := r.PathValue("target")
	switch target {
	case library.TargetItem:
		if _, err := s.visibleItem(r, id); err != nil {
			writeErr(w, 404, "not found")
			return
		}
	case library.TargetShow:
		if _, err := s.visibleShow(r, id); err != nil {
			writeErr(w, 404, "not found")
			return
		}
	default:
		writeErr(w, 404, "not found")
		return
	}
	ctx := r.Context()
	extras, extraLinks, err := s.Store.AttachedTo(ctx, target, id, library.LinkExtra)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	versions, versionLinks, err := s.Store.AttachedTo(ctx, target, id, library.LinkVersion)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	acc := s.access(r)
	type row struct {
		Item *library.Item    `json:"item"`
		Link library.ItemLink `json:"link"`
	}
	collect := func(items []*library.Item, links []library.ItemLink) []row {
		out := []row{}
		for i, it := range items {
			if acc == nil || acc.AllowsItem(it, nil) {
				s.forViewer(r, it)
				out = append(out, row{it, links[i]})
			}
		}
		return out
	}
	t := &library.Title{Target: target, ID: id, Meta: s.Store.TitleMeta(ctx, target, id)}
	studios := s.Store.TitleBrands(ctx, target, id, library.LogoStudio)
	writeJSON(w, 200, map[string]any{"extras": collect(extras, extraLinks), "versions": collect(versions, versionLinks), "studios": studios,
		"director": t.Meta.Director, "edition": t.Meta.Edition, "tags": t.Meta.Tags, "hasBanner": t.Meta.Banner != "", "hasLogo": t.Meta.Logo != ""})
}

// GET /api/meta/{target}/{id}/art/{kind}: a title's banner or logo.
func (s *Server) handleTitleArt(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	target := r.PathValue("target")
	switch target {
	case library.TargetItem:
		if _, err := s.visibleItem(r, id); err != nil {
			writeErr(w, 404, "not found")
			return
		}
	case library.TargetShow:
		if _, err := s.visibleShow(r, id); err != nil {
			writeErr(w, 404, "not found")
			return
		}
	default:
		writeErr(w, 404, "not found")
		return
	}
	m := s.Store.TitleMeta(r.Context(), target, id)
	switch r.PathValue("kind") {
	case "banner":
		s.serveArtSrc(w, r, m.Banner)
	case "logo":
		s.serveArtSrc(w, r, m.Logo)
	default:
		writeErr(w, 404, "no such artwork")
	}
}
