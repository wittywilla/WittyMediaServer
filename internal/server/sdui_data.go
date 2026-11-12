// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// The server's answers to the pages' data sources (sdui.Resolver). Every
// source is read the way the ordinary routes read it: with the account's
// limits in the query (library.Access), or through visibleItem, visibleShow,
// visibleLibraries and Channels.Visible. A title an account may not see never
// reaches a page sent to it, whatever an administrator wrote into the page.
// docs/SDUI.md, "Data sources", lists them.
//
// Each answer is one or two of the library store's own indexed queries, asked
// for no more than the device's page of items: nothing here walks a library.

// sduiResolver answers for one request.
type sduiResolver struct {
	s    *Server
	r    *http.Request
	ctx  context.Context
	u    *auth.User
	acc  *library.Access
	vars map[string]string
	now  time.Time

	libs    []library.Library // the libraries the account can open, once asked for
	libsErr error
	gotLibs bool
	subject *sdui.Item
	gotSubj bool
	// noPlay: the profile's screen time is over, so nothing offers to play.
	noPlay bool
	// What the device said it is, for the modules' sources.
	formFactor, platform, locale string
}

func (s *Server) newSDUIResolver(r *http.Request, vars map[string]string) *sduiResolver {
	return &sduiResolver{s: s, r: r, ctx: r.Context(), u: userFrom(r), acc: s.access(r), vars: vars, now: time.Now()}
}

// libraries are the libraries the account can open and the menus show.
func (rv *sduiResolver) libraries() []library.Library {
	if !rv.gotLibs {
		rv.gotLibs = true
		all, err := rv.s.visibleLibraries(rv.ctx, rv.acc)
		rv.libsErr = err
		for _, l := range all {
			if !l.Hidden() {
				rv.libs = append(rv.libs, l)
			}
		}
	}
	return rv.libs
}

// library is one library the account can open, by the ID a page names it by.
func (rv *sduiResolver) library(id string) (*library.Library, bool) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 || !rv.acc.AllowsLibrary(n) {
		return nil, false
	}
	for _, l := range rv.libraries() {
		if l.ID == n {
			return &l, true
		}
	}
	// A hidden library (commercials off the menus) can still be opened by number.
	l, err := rv.s.Store.Library(rv.ctx, n)
	if err != nil || (l.Type == "games" && !rv.acc.Can(library.FeatureGames)) {
		return nil, false
	}
	return l, true
}

func (rv *sduiResolver) can(what string) []string {
	out := []string{what}
	if rv.acc.Can(library.FeatureDownloads) {
		out = append(out, "download")
	}
	if rv.acc.Can(library.FeatureSharing) {
		out = append(out, "share")
	}
	// What plays can be sent to another device and watched together, unless the profile
	// has those turned off (the server's own switches are the dialogs' to explain).
	if what == "play" && rv.acc.Can(library.FeatureRemoteControl) {
		out = append(out, "playOn")
	}
	if what == "play" && rv.acc.Can(library.FeatureSyncplay) {
		out = append(out, "watchParty")
	}
	return out
}

// item is one library item as a page shows it.
func (rv *sduiResolver) item(it *library.Item) sdui.Item {
	id := itoa(it.ID)
	out := sdui.Item{ID: id, Title: it.Title, Overview: it.Overview, Tagline: it.Tagline, ContentRating: it.ContentRating, RuntimeSec: it.RuntimeSec,
		Genres: it.Genres, Studio: it.Studio, Route: "/item/" + id, Images: map[string]string{}}
	if it.Year != nil && *it.Year > 0 {
		out.Year = *it.Year
		out.Subtitle = strconv.Itoa(*it.Year)
	}
	if it.Rating != nil {
		out.Rating = *it.Rating
	}
	if it.Poster != "" {
		out.Images["poster"] = "/api/items/" + id + "/poster"
	}
	if it.Backdrop != "" {
		out.Images["backdrop"] = "/api/items/" + id + "/backdrop"
	}
	playable := false
	switch {
	case it.Kind == "audio":
		out.Kind, out.Artist, out.Subtitle = "track", firstNonEmpty(it.Artist, it.AlbumArtist), firstNonEmpty(it.Artist, it.AlbumArtist)
		if it.Track != nil {
			out.Track = *it.Track
		}
		if p := out.Images["poster"]; p != "" {
			out.Images["square"] = p
		}
		playable = true
	case it.Kind == "book":
		out.Kind, out.Subtitle = "book", firstNonEmpty(it.AlbumArtist, it.Artist, out.Subtitle)
	case it.Kind == "game":
		out.Kind, out.Route = "game", "/game/"+id
	case it.Kind == "photo":
		out.Kind = "photo"
	case strings.HasPrefix(it.Kind, "dvd") || strings.HasPrefix(it.Kind, "bluray") || it.Kind == library.CDKind:
		out.Kind, playable = "disc", true
	case it.ShowID != nil:
		out.Kind, out.ShowTitle, out.ShowID, playable = "episode", it.ShowTitle, itoa(*it.ShowID), true
		if it.Season != nil {
			out.Season = *it.Season
		}
		if it.Episode != nil {
			out.Episode = *it.Episode
		}
		out.Subtitle = fmt.Sprintf("S%d E%d", out.Season, out.Episode)
		if it.ShowTitle != "" {
			out.Subtitle = it.ShowTitle + " · " + out.Subtitle
		}
	default:
		out.Kind, playable = "movie", true
	}
	if p := it.Progress; p != nil {
		out.Watched = p.Watched
		if !p.Watched && p.DurationSec > 0 && p.PositionSec > 0 {
			out.Progress = min(1, p.PositionSec/p.DurationSec)
			out.ResumeSec = p.ResumeSec
		}
	}
	if w, h := it.Width, it.Height; out.Kind == "movie" || out.Kind == "episode" {
		switch {
		case w >= 3800 || h >= 2100:
			out.Badges = append(out.Badges, "4K")
		case w >= 1900 || h >= 1000:
			out.Badges = append(out.Badges, "HD")
		}
	}
	if playable {
		out.PlayID = id
		out.Can = append(rv.can("play"), "watchLater", "watched")
		if rv.acc.Can(library.FeatureRemoteControl) {
			out.Can = append(out.Can, "playOn")
		}
		if rv.acc.Can(library.FeatureSyncplay) {
			out.Can = append(out.Can, "watchParty")
		}
	}
	if rv.noPlay {
		rv.unplayable(&out)
	}
	return out
}

// unplayable takes what plays out of an item: the profile's screen time is over.
func (rv *sduiResolver) unplayable(it *sdui.Item) {
	it.PlayID, it.NextID = "", ""
	it.Can = slices.DeleteFunc(it.Can, func(c string) bool { return c == "play" || c == "playOn" || c == "watchParty" })
}

func (rv *sduiResolver) show(sh *library.Show) sdui.Item {
	id := itoa(sh.ID)
	out := sdui.Item{ID: "show-" + id, Kind: "show", Title: sh.Title, Overview: sh.Overview, ContentRating: sh.ContentRating, Genres: sh.Genres, Studio: sh.Network,
		Unwatched: sh.Unwatched, Count: sh.EpisodeCount, Route: "/show/" + id, Images: map[string]string{}}
	if sh.Year != nil && *sh.Year > 0 {
		out.Year, out.Subtitle = *sh.Year, strconv.Itoa(*sh.Year)
	}
	if sh.Rating != nil {
		out.Rating = *sh.Rating
	}
	if sh.Poster != "" {
		out.Images["poster"] = "/api/shows/" + id + "/art/poster"
	}
	if sh.Backdrop != "" {
		out.Images["backdrop"] = "/api/shows/" + id + "/art/backdrop"
	}
	return out
}

// isArtistPage: a detail page opened for an artist (/artist/{library}?name=), not for
// one of their albums or a title.
func isArtistPage(v map[string]string) bool {
	return v["artist"] != "" && v["album"] == "" && v["item"] == "" && v["show"] == "" && v["collection"] == ""
}

// artist is a music artist as a card and as an artist page's subject. i is the artist's
// place in the library's list, which names it among that library's artists.
func (rv *sduiResolver) artist(lib int64, i int, a library.Artist) sdui.Item {
	it := sdui.Item{ID: "artist-" + strconv.Itoa(i), Kind: "artist", Title: a.Name, Artist: a.Name, Count: a.Albums, Route: "/artist/" + itoa(lib) + "?name=" + url.QueryEscape(a.Name), Images: map[string]string{}}
	noun := "albums"
	if a.Albums == 1 {
		noun = "album"
	}
	it.Subtitle = strconv.Itoa(a.Albums) + " " + noun
	if a.CoverItem != 0 {
		it.Images["square"] = "/api/items/" + itoa(a.CoverItem) + "/poster"
	}
	return it
}

func (rv *sduiResolver) album(lib int64, a library.Album) sdui.Item {
	key := a.CoverItem
	if key == 0 {
		key = a.FirstItem
	}
	out := sdui.Item{ID: fmt.Sprintf("album-%d-%d", lib, key), Kind: "album", Title: a.Title, Subtitle: a.Artist, Artist: a.Artist, Count: a.Tracks, RuntimeSec: a.DurationSec,
		Route: "/album/" + itoa(lib) + "?artist=" + url.QueryEscape(a.Artist) + "&album=" + url.QueryEscape(a.Title), Images: map[string]string{}}
	if a.Year != nil {
		out.Year = *a.Year
	}
	if a.Genre != "" {
		out.Genres = []string{a.Genre}
	}
	if a.CoverItem != 0 {
		out.Images["square"] = "/api/items/" + itoa(a.CoverItem) + "/poster"
	}
	return out
}

func (rv *sduiResolver) items(list []*library.Item) []sdui.Item {
	out := make([]sdui.Item, 0, len(list))
	for _, it := range list {
		out = append(out, rv.item(it))
	}
	return out
}

// page cuts a list that was read whole (a short one) to the page asked for.
func page(all []sdui.Item, req sdui.SourceRequest) sdui.SourceResult {
	total := len(all)
	if req.Offset >= total {
		return sdui.SourceResult{Total: total}
	}
	all = all[req.Offset:]
	more := len(all) > req.Limit
	if more {
		all = all[:req.Limit]
	}
	return sdui.SourceResult{Items: all, More: more, Total: total}
}

// libraryRows are a library's things in an order: shows for a TV library, albums for music
// and audiobooks, items for the rest.
func (rv *sduiResolver) libraryRows(l *library.Library, sortBy string, desc bool, req sdui.SourceRequest, search string) sdui.SourceResult {
	s := rv.s
	limit := req.Limit + 1
	switch l.Type {
	case "shows", "anime":
		sq := library.ShowQuery{LibraryID: l.ID, Sort: sortBy, Desc: desc, Limit: req.Offset + limit, UserID: rv.u.ID, Access: rv.acc, Search: search}
		s.animeQuery(rv.ctx, &sq)
		shows, err := s.Store.Shows(rv.ctx, sq)
		if err != nil {
			return sdui.SourceResult{}
		}
		all := make([]sdui.Item, 0, len(shows))
		for _, sh := range shows {
			all = append(all, rv.show(sh))
		}
		res := page(all, req)
		res.Total = -1
		return res
	case "music", "audiobooks", "podcasts":
		albumSort := "added"
		if sortBy == "title" {
			albumSort = "title"
		}
		albums, err := s.Store.AlbumsFiltered(rv.ctx, l.ID, library.AlbumFilter{Search: search, Sort: albumSort, Limit: req.Offset + limit}, rv.acc)
		if err != nil {
			return sdui.SourceResult{}
		}
		all := make([]sdui.Item, 0, len(albums))
		for _, a := range albums {
			all = append(all, rv.album(l.ID, a))
		}
		res := page(all, req)
		res.Total = -1
		return res
	}
	items, _, err := s.Store.Items(rv.ctx, library.ItemQuery{LibraryID: l.ID, Sort: sortBy, Desc: desc, Limit: limit, Offset: req.Offset, Access: rv.acc, NoTotal: true, Brief: true, Search: search})
	if err != nil {
		return sdui.SourceResult{}
	}
	_ = s.Store.AttachProgress(rv.ctx, rv.u.ID, items)
	res := sdui.SourceResult{Items: rv.items(items), Total: -1}
	if len(res.Items) > req.Limit {
		res.Items, res.More = res.Items[:req.Limit], true
	}
	return res
}

// browse answers a source that is a filter: the library page's own engine.
func (rv *sduiResolver) browse(f library.BrowseFilter, req sdui.SourceRequest, exclude string) (sdui.SourceResult, error) {
	if f.Library != 0 && !rv.acc.AllowsLibrary(f.Library) {
		return sdui.SourceResult{}, nil
	}
	f.Limit, f.Offset = req.Limit+1, req.Offset
	res, err := rv.s.Store.Browse(rv.ctx, f, rv.u.ID, rv.acc)
	if err != nil {
		return sdui.SourceResult{}, err
	}
	out := sdui.SourceResult{Total: res.Total}
	for _, row := range res.Results {
		var it sdui.Item
		switch {
		case row.Item != nil:
			it = rv.item(row.Item)
		case row.Show != nil:
			it = rv.show(row.Show)
		default:
			continue
		}
		if it.ID == exclude {
			continue
		}
		out.Items = append(out.Items, it)
	}
	if len(out.Items) > req.Limit {
		out.Items, out.More = out.Items[:req.Limit], true
	}
	return out, nil
}

// seed changes when a shuffled row should: every hour, day or week.
func (rv *sduiResolver) seed(reshuffle string) int {
	hours := rv.now.Unix() / 3600
	switch reshuffle {
	case "hourly":
		return int(hours) + int(rv.u.ID)
	case "weekly":
		return int(hours/24/7) + int(rv.u.ID)
	}
	return int(hours/24) + int(rv.u.ID)
}

// filterOf is a browse filter from a source's parameters and the page's own.
func filterOf(src sdui.Source, page map[string]string) library.BrowseFilter {
	q := url.Values{}
	for _, k := range []string{"genre", "studio", "network", "tag", "series", "year", "decade", "content", "minRating", "resolution", "watched", "sort"} {
		if v := src.Str(k); v != "" {
			q.Set(k, v)
		}
	}
	if v := src.Str("titles"); v != "" {
		q.Set("kind", v)
	}
	if src.Bool("desc") {
		q.Set("desc", "1")
	}
	if v := src.Str("service"); v != "" {
		q.Set("platform", v)
	}
	for k, v := range page { // the page's filter bar wins over the source's own
		if k == "service" {
			k = "platform" // the library's name for a streaming service
		}
		q.Set(k, v)
	}
	f := library.ParseBrowseFilter(q)
	f.Library = int64(src.Int("library"))
	return f
}

// Items answers one data source.
func (rv *sduiResolver) Items(req sdui.SourceRequest) (sdui.SourceResult, error) {
	s, src := rv.s, req.Source
	if req.Limit <= 0 {
		return sdui.SourceResult{}, nil
	}
	req.Limit = min(req.Limit, 500)
	if mod := sdui.ModuleOf(src.Type()); mod != "" {
		return rv.moduleItems(mod, req)
	}
	var lib *library.Library
	if id := src.Str("library"); id != "" {
		l, ok := rv.library(id)
		if !ok {
			return sdui.SourceResult{}, nil // not this account's to see: the same as empty
		}
		lib = l
	}
	switch src.Type() {
	case "continueWatching":
		cont, err := s.Store.ContinueWatching(rv.ctx, rv.u.ID, 60)
		if err != nil {
			return sdui.SourceResult{}, err
		}
		cont = s.filterItems(rv.ctx, rv.acc, cont)
		_ = s.Store.AttachProgress(rv.ctx, rv.u.ID, cont)
		var all []sdui.Item
		for _, it := range cont {
			media := "watch"
			switch it.Kind {
			case "book":
				media = "read"
			case "audio":
				media = "listen"
			}
			if m := src.Str("media"); m == "" || m == "all" || m == media {
				all = append(all, rv.item(it))
			}
		}
		return page(all, req), nil
	case "nextUp":
		return page(rv.items(s.nextUp(rv.r, rv.libraries())), req), nil
	case "recommended":
		items, shows, _, err := s.Store.RecommendForYou(rv.ctx, rv.u.ID, rv.acc, int64(rv.seed("daily")), min(req.Offset+req.Limit, 60))
		if err != nil {
			return sdui.SourceResult{}, err
		}
		all := rv.items(items)
		for _, sh := range shows {
			all = append(all, rv.show(sh))
		}
		return page(all, req), nil
	case "watchLater", "collection":
		var c *library.Collection
		if src.Type() == "collection" {
			id, _ := strconv.ParseInt(src.Str("collection"), 10, 64)
			got, err := s.Store.CollectionByID(rv.ctx, id, rv.u.ID, rv.u.IsAdmin)
			if err != nil {
				return sdui.SourceResult{}, nil
			}
			c = got
		} else {
			list, err := s.Store.Collections(rv.ctx, rv.u.ID, rv.acc)
			if err != nil {
				return sdui.SourceResult{}, err
			}
			for _, x := range list {
				if x.System == "watchlater" && x.OwnerID == rv.u.ID {
					c = x
				}
			}
			if c == nil {
				return sdui.SourceResult{}, nil
			}
		}
		items, shows, err := s.Store.CollectionItems(rv.ctx, c, rv.u.ID, rv.acc, req.Offset+req.Limit+1, 0)
		if err != nil {
			return sdui.SourceResult{}, err
		}
		_ = s.Store.AttachProgress(rv.ctx, rv.u.ID, items)
		all := rv.items(items)
		for _, sh := range shows {
			all = append(all, rv.show(sh))
		}
		return page(all, req), nil
	case "liked":
		return rv.liked(src, req, lib)
	case "statistics":
		return rv.statistics(src, req, lib)
	case "books":
		return rv.books(src, req, lib)
	case "savedView":
		id, _ := strconv.ParseInt(src.Str("view"), 10, 64)
		views, err := s.Store.SavedViews(rv.ctx, rv.u.ID)
		if err != nil {
			return sdui.SourceResult{}, err
		}
		for _, v := range views {
			if v.ID != id {
				continue
			}
			q, err := url.ParseQuery(v.Query)
			if err != nil {
				return sdui.SourceResult{}, nil
			}
			f := library.ParseBrowseFilter(q)
			f.Library = v.LibraryID
			return rv.browse(f, req, "")
		}
		return sdui.SourceResult{}, nil
	case "recentlyAdded":
		if lib != nil {
			return rv.libraryRows(lib, "added", true, req, ""), nil
		}
		if kind := src.Str("kind"); kind != "" {
			var all []sdui.Item
			for _, l := range rv.libraries() {
				if l.Type == kind && len(all) < req.Offset+req.Limit+1 {
					all = append(all, rv.libraryRows(&l, "added", true, sdui.SourceRequest{Limit: req.Offset + req.Limit + 1}, "").Items...)
				}
			}
			return page(all, req), nil
		}
		items, _, err := s.Store.Items(rv.ctx, library.ItemQuery{Sort: "added", Desc: true, Limit: req.Limit + 1, Offset: req.Offset, Access: rv.acc, NoTotal: true, Brief: true, NoEpisodes: true, NoGallery: true, Watchable: true})
		if err != nil {
			return sdui.SourceResult{}, err
		}
		_ = s.Store.AttachProgress(rv.ctx, rv.u.ID, items)
		return page(rv.items(items), sdui.SourceRequest{Limit: req.Limit}), nil
	case "recentlyReleased":
		f := library.BrowseFilter{Sort: "newest"}
		if lib != nil {
			f.Library = lib.ID
		}
		return rv.browse(f, req, "")
	case "topRated":
		f := library.BrowseFilter{Sort: "rating", MinRating: float64(src.Int("minRating"))}
		if lib != nil {
			f.Library = lib.ID
		}
		return rv.browse(f, req, "")
	case "random":
		f := library.BrowseFilter{Sort: "shuffle", Seed: rv.seed(src.Str("reshuffle")), Watched: src.Str("watched")}
		if lib != nil {
			f.Library = lib.ID
		}
		res, err := rv.browse(f, req, "")
		res.More = false // a shuffle has no next page worth having
		return res, err
	case "browse", "genre", "studio", "tag":
		return rv.browse(filterOf(src, req.Filter), req, "")
	case "collections":
		list, err := s.Store.Collections(rv.ctx, rv.u.ID, rv.acc)
		if err != nil {
			return sdui.SourceResult{}, err
		}
		var all []sdui.Item
		for _, c := range list {
			it := sdui.Item{ID: "collection-" + itoa(c.ID), Kind: "collection", Title: c.Name, Subtitle: c.Description, Count: c.Count, Icon: "collection", Route: "/collections/" + itoa(c.ID), Images: map[string]string{}}
			switch {
			case c.CoverID != 0:
				it.Images["backdrop"] = "/api/items/" + itoa(c.CoverID) + "/backdrop"
			case c.CoverShow != 0:
				it.Images["backdrop"] = "/api/shows/" + itoa(c.CoverShow) + "/art/backdrop"
			}
			all = append(all, it)
		}
		return page(all, req), nil
	case "featured":
		q := library.ItemQuery{Sort: "added", Desc: true, Limit: 40, NoEpisodes: true, NoGallery: true, HasArt: true, Access: rv.acc, NoTotal: true, Brief: true}
		if lib != nil {
			q.LibraryID = lib.ID
		}
		all, _, err := s.Store.Items(rv.ctx, q)
		if err != nil {
			return sdui.SourceResult{}, err
		}
		var picked []*library.Item
		for _, it := range all {
			if it.Backdrop != "" && len(picked) < req.Offset+req.Limit {
				picked = append(picked, it)
			}
		}
		if picked == nil {
			picked = all[:min(len(all), req.Offset+req.Limit)]
		}
		_ = s.Store.AttachProgress(rv.ctx, rv.u.ID, picked)
		res := page(rv.items(picked), req)
		res.More = false
		return res, nil
	case "libraries":
		var all []sdui.Item
		for _, l := range rv.libraries() {
			if k := src.Str("kind"); k != "" && l.Type != k {
				continue
			}
			all = append(all, sdui.Item{ID: "lib" + itoa(l.ID), Kind: "library", Title: l.Name, Count: l.ItemCount, Icon: sdui.LibraryIcon(l.Type), Route: "/library/" + itoa(l.ID)})
		}
		return page(all, req), nil
	case "genres":
		if lib == nil {
			return sdui.SourceResult{}, nil
		}
		genres, err := s.Store.Genres(rv.ctx, lib.ID, rv.acc)
		if err != nil {
			return sdui.SourceResult{}, err
		}
		var all []sdui.Item
		for _, g := range genres {
			all = append(all, sdui.Item{ID: "genre-" + g, Kind: "genre", Title: g, Route: "/library/" + itoa(lib.ID) + "?genre=" + url.QueryEscape(g)})
		}
		return page(all, req), nil
	case "liveNow":
		return page(rv.channels(src.Bool("favourites"), "", req.Offset+req.Limit+1, true), req), nil
	case "recordings":
		if s.TV == nil || !s.mayRecord(rv.r) || !rv.acc.Can(library.FeatureRecordings) {
			return sdui.SourceResult{}, nil
		}
		list, err := s.TV.Recordings(rv.ctx)
		if err != nil {
			return sdui.SourceResult{}, err
		}
		var all []sdui.Item
		for _, x := range list {
			if x.Status != "done" || (!rv.u.IsAdmin && x.UserID != rv.u.ID) {
				continue
			}
			all = append(all, sdui.Item{ID: "recording-" + itoa(x.ID), Kind: "recording", Title: x.Title, Subtitle: firstNonEmpty(x.Sub, x.Channel), RuntimeSec: float64(x.Stop - x.Start), Icon: "record",
				Route: "/tv/recordings", Scope: "wittytv"})
		}
		sort.SliceStable(all, func(i, j int) bool { return all[i].ID > all[j].ID })
		return page(all, req), nil
	case "albums":
		if lib == nil {
			return sdui.SourceResult{}, nil
		}
		albums, err := s.Store.AlbumsFiltered(rv.ctx, lib.ID, library.AlbumFilter{Sort: firstNonEmpty(src.Str("sort"), "added"), Artist: src.Str("artist"), Genre: src.Str("genre"), UserID: rv.u.ID, Limit: req.Offset + req.Limit + 1}, rv.acc)
		if err != nil {
			return sdui.SourceResult{}, err
		}
		all := make([]sdui.Item, 0, len(albums))
		for _, a := range albums {
			it := rv.album(lib.ID, a)
			// An album's own page doesn't offer it again under "More by this artist".
			if rv.subject != nil && rv.subject.Kind == "album" && rv.subject.ID == it.ID {
				continue
			}
			all = append(all, it)
		}
		return page(all, req), nil
	case "artists":
		if lib == nil {
			return sdui.SourceResult{}, nil
		}
		artists, err := s.Store.Artists(rv.ctx, lib.ID, rv.acc)
		if err != nil {
			return sdui.SourceResult{}, err
		}
		all := make([]sdui.Item, 0, len(artists))
		for i, a := range artists {
			if i >= req.Offset+req.Limit+1 {
				break
			}
			all = append(all, rv.artist(lib.ID, i, a))
		}
		res := page(all, req)
		res.Total = len(artists)
		return res, nil
	case "continuePlaying":
		if !rv.acc.Can(library.FeatureGames) {
			return sdui.SourceResult{}, nil
		}
		rows, err := s.Store.GameList(rv.ctx, rv.u.ID, false, 2*(req.Offset+req.Limit))
		if err != nil {
			return sdui.SourceResult{}, err
		}
		var all []sdui.Item
		for _, e := range rows {
			it, err := s.visibleItem(rv.r, e.ItemID)
			if err != nil || it.Kind != "game" || (lib != nil && it.LibraryID != lib.ID) {
				continue
			}
			all = append(all, rv.item(it))
		}
		return page(all, req), nil
	case "episodes":
		return rv.episodes(src, req)
	case "tracks":
		if lib == nil {
			return sdui.SourceResult{}, nil
		}
		album, artist := src.Str("album"), src.Str("artist")
		q := library.ItemQuery{LibraryID: lib.ID, Kind: "audio", Sort: "track", Access: rv.acc, Brief: true, Album: &album, NoTotal: true}
		if artist != "" {
			q.AlbumArtist = &artist
		}
		if _, one := src["album"]; !one {
			// An artist's page: every album's songs, by title, a page at a time.
			q.Album, q.Sort, q.NoTotal, q.Offset, q.Limit = nil, "", false, req.Offset, req.Limit
			items, total, err := s.Store.Items(rv.ctx, q)
			if err != nil {
				return sdui.SourceResult{}, err
			}
			_ = s.Store.AttachProgress(rv.ctx, rv.u.ID, items)
			return sdui.SourceResult{Items: rv.items(items), More: req.Offset+len(items) < total, Total: total}, nil
		}
		if lib.Type == "podcasts" {
			q.Sort, q.Desc = "aired", true
		}
		items, _, err := s.Store.Items(rv.ctx, q)
		if err != nil {
			return sdui.SourceResult{}, err
		}
		_ = s.Store.AttachProgress(rv.ctx, rv.u.ID, items)
		return page(rv.items(items), req), nil
	case "similar":
		return rv.similar(src, req)
	case "credits":
		return rv.credits(src, req)
	case "filmography":
		return rv.filmography(src, req)
	case "search":
		term := strings.TrimSpace(src.Str("query"))
		if len(term) > 100 {
			term = term[:100]
		}
		if len([]rune(term)) < 2 {
			return sdui.SourceResult{}, nil
		}
		if src.Str("in") == "channels" {
			return page(rv.channels(false, term, req.Offset+req.Limit+1, false), req), nil
		}
		if src.Str("in") == "people" {
			return rv.searchPeople(term, req)
		}
		if lib == nil {
			return sdui.SourceResult{}, nil
		}
		return rv.libraryRows(lib, "title", false, req, term), nil
	}
	return sdui.SourceResult{}, nil
}

// channels are the channels the account may watch, with what is on now and next when asked.
func (rv *sduiResolver) channels(favouritesOnly bool, search string, limit int, programmes bool) []sdui.Item {
	s := rv.s
	if s.Channels == nil || (rv.acc != nil && !rv.acc.LiveTV) {
		return nil
	}
	rows := s.libraryRows(rv.ctx, rv.r)
	if s.TV != nil && rv.acc.CanWatchChannels("tuner") {
		rows = append(rows, s.tunerRows(rv.ctx, rv.r)...)
	}
	favs, _ := s.Channels.Favourites(rv.ctx, rv.u.ID)
	search = strings.ToLower(search)
	var out []sdui.Item
	for i := range rows {
		c := &rows[i]
		if len(out) >= limit {
			break
		}
		if (favouritesOnly && !favs[c.Key]) || (search != "" && !strings.Contains(strings.ToLower(c.Name), search)) || c.Encrypted != "" {
			continue
		}
		it := sdui.Item{ID: c.Key, Kind: "channel", Title: c.Name, Number: c.Number, Subtitle: c.Number, Icon: "live", Route: "/tv/watch/" + c.Key, Scope: "wittytv", Images: map[string]string{}}
		if strings.HasPrefix(c.Logo, "/api/") {
			it.Images["logo"] = c.Logo
		}
		if programmes {
			for _, p := range s.rowProgrammes(rv.ctx, c, rv.now, rv.now.Add(3*time.Hour)) {
				pg := &sdui.Programme{Title: p.Title, Start: p.Start, End: p.End}
				if p.Start <= rv.now.Unix() && p.End > rv.now.Unix() {
					it.Now = pg
				} else if p.Start > rv.now.Unix() && it.Next == nil {
					it.Next = pg
					break
				}
			}
		}
		out = append(out, it)
	}
	return out
}

// episodes are one season of a show: the one asked for, else the season of the next episode
// to watch, else the first.
func (rv *sduiResolver) episodes(src sdui.Source, req sdui.SourceRequest) (sdui.SourceResult, error) {
	s := rv.s
	id, _ := strconv.ParseInt(strings.TrimPrefix(src.Str("show"), "show-"), 10, 64)
	sh, err := s.visibleShow(rv.r, id)
	if err != nil {
		return sdui.SourceResult{}, nil
	}
	eps, _, err := s.Store.Items(rv.ctx, library.ItemQuery{ShowID: sh.ID, Access: rv.acc, Limit: 3000, NoTotal: true, Brief: true})
	if err != nil {
		return sdui.SourceResult{}, err
	}
	season := -1
	if v := src.Str("season"); v != "" {
		season, _ = strconv.Atoi(v)
	} else if next, _ := s.Store.NextUpItems(rv.ctx, rv.u.ID, []int64{sh.ID}, rv.acc, 1); len(next) > 0 && next[0].Season != nil {
		season = *next[0].Season
	}
	var kept []*library.Item
	for _, e := range eps {
		if e.Season == nil {
			continue
		}
		if season < 0 {
			season = *e.Season
			for _, x := range eps { // the lowest season that isn't specials
				if x.Season != nil && *x.Season > 0 && (*x.Season < season || season == 0) {
					season = *x.Season
				}
			}
		}
		if *e.Season == season {
			kept = append(kept, e)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		a, b := 0, 0
		if kept[i].Episode != nil {
			a = *kept[i].Episode
		}
		if kept[j].Episode != nil {
			b = *kept[j].Episode
		}
		return a < b
	})
	_ = s.Store.AttachProgress(rv.ctx, rv.u.ID, kept)
	res := page(rv.items(kept), req)
	if season >= 0 {
		res.Selected = strconv.Itoa(season)
	}
	return res, nil
}

// similar are titles in the same library sharing the title's first genre, best scored first.
func (rv *sduiResolver) similar(src sdui.Source, req sdui.SourceRequest) (sdui.SourceResult, error) {
	s := rv.s
	ref := src.Str("item")
	var libID int64
	var genres []string
	exclude := ref
	if id, ok := strings.CutPrefix(ref, "show-"); ok || rv.vars["show"] == ref && rv.vars["item"] == "" {
		n, _ := strconv.ParseInt(id, 10, 64)
		sh, err := s.visibleShow(rv.r, n)
		if err != nil {
			return sdui.SourceResult{}, nil
		}
		libID, genres, exclude = sh.LibraryID, sh.Genres, "show-"+itoa(sh.ID)
	} else {
		n, _ := strconv.ParseInt(ref, 10, 64)
		it, err := s.visibleItem(rv.r, n)
		if err != nil {
			return sdui.SourceResult{}, nil
		}
		libID, genres = it.LibraryID, it.Genres
		if it.ShowID != nil {
			return sdui.SourceResult{}, nil // an episode's neighbours are its season
		}
	}
	if len(genres) == 0 {
		return sdui.SourceResult{}, nil
	}
	res, err := rv.browse(library.BrowseFilter{Library: libID, Genre: genres[0], Sort: "rating"}, sdui.SourceRequest{Offset: req.Offset, Limit: req.Limit + 1}, exclude)
	if len(res.Items) > req.Limit {
		res.Items, res.More = res.Items[:req.Limit], true
	}
	if res.Total > 0 {
		res.Total--
	}
	return res, err
}

// Each lists what a repeated section is repeated over.
func (rv *sduiResolver) Each(over string, kinds []string, limit int) ([]sdui.Each, error) {
	var out []sdui.Each
	switch over {
	case "libraries":
		for _, l := range rv.libraries() {
			if len(kinds) > 0 && !containsStr(kinds, l.Type) {
				continue
			}
			out = append(out, sdui.Each{Key: itoa(l.ID), Label: l.Name, Kind: l.Type})
		}
		return out, rv.libsErr
	case "pinnedViews":
		views, err := rv.s.Store.SavedViews(rv.ctx, rv.u.ID)
		if err != nil {
			return nil, err
		}
		for _, v := range views {
			if v.Pinned && (v.LibraryID == 0 || rv.acc.AllowsLibrary(v.LibraryID)) {
				out = append(out, sdui.Each{Key: itoa(v.ID), Label: v.Name})
			}
		}
	case "collections":
		list, err := rv.s.Store.Collections(rv.ctx, rv.u.ID, rv.acc)
		if err != nil {
			return nil, err
		}
		for _, c := range list {
			if c.System == "" && c.Count > 0 {
				out = append(out, sdui.Each{Key: itoa(c.ID), Label: c.Name})
			}
		}
	case "personParts":
		list, err := rv.personParts()
		if err != nil {
			return nil, err
		}
		out = list
	case "bookSeries", "bookAuthors":
		by := "series"
		if over == "bookAuthors" {
			by = "author"
		}
		list, err := rv.bookGroups(by)
		if err != nil {
			return nil, err
		}
		out = list
	}
	if len(out) > limit && limit > 0 {
		out = out[:limit]
	}
	return out, nil
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Subject is the title a detail page is about, read with the account's limits.
func (rv *sduiResolver) Subject() (*sdui.Item, error) {
	if rv.gotSubj {
		if rv.subject == nil {
			return nil, sdui.ErrNotFound
		}
		return rv.subject, nil
	}
	rv.gotSubj = true
	s, v := rv.s, rv.vars
	switch {
	case v["person"] != "":
		rv.subject = rv.personSubject(v["person"])
	case v["collection"] != "":
		id, _ := strconv.ParseInt(v["collection"], 10, 64)
		c, err := s.Store.CollectionByID(rv.ctx, id, rv.u.ID, rv.u.IsAdmin)
		if err != nil {
			return nil, sdui.ErrNotFound
		}
		it := sdui.Item{ID: "collection-" + itoa(c.ID), Kind: "collection", Title: c.Name, Overview: c.Description, Count: c.Count, Route: "/collections/" + itoa(c.ID), Images: map[string]string{}}
		if rv.acc.Can(library.FeatureSharing) {
			it.Can = []string{"share"}
		}
		if c.CoverID != 0 {
			it.Images["backdrop"], it.Images["poster"] = "/api/items/"+itoa(c.CoverID)+"/backdrop", "/api/items/"+itoa(c.CoverID)+"/poster"
		}
		rv.subject = &it
	case v["album"] != "":
		lib, ok := rv.library(v["library"])
		if !ok {
			return nil, sdui.ErrNotFound
		}
		albums, err := s.Store.AlbumsFiltered(rv.ctx, lib.ID, library.AlbumFilter{Artist: v["artist"], Sort: "title"}, rv.acc)
		if err != nil {
			return nil, sdui.ErrNotFound
		}
		for _, a := range albums {
			if a.Title == v["album"] && (v["artist"] == "" || a.Artist == v["artist"]) {
				it := rv.album(lib.ID, a)
				// Play starts at the first track; the album page plays on from there.
				if first := firstNonZero(a.FirstItem, a.CoverItem); first != 0 {
					it.PlayID, it.Can = itoa(first), rv.can("play")
				}
				if rv.noPlay {
					rv.unplayable(&it)
				}
				rv.subject = &it
				break
			}
		}
	case isArtistPage(v):
		lib, ok := rv.library(v["library"])
		if !ok {
			return nil, sdui.ErrNotFound
		}
		artists, err := s.Store.Artists(rv.ctx, lib.ID, rv.acc)
		if err != nil {
			return nil, sdui.ErrNotFound
		}
		for i, a := range artists {
			if a.Name == v["artist"] {
				it := rv.artist(lib.ID, i, a)
				if a.CoverItem != 0 {
					// Play starts with one of the artist's songs, as an album's does with its first.
					it.PlayID, it.Can = itoa(a.CoverItem), []string{"play"}
				}
				if rv.noPlay {
					rv.unplayable(&it)
				}
				rv.subject = &it
				break
			}
		}
	case v["show"] != "" && v["item"] == "":
		id, _ := strconv.ParseInt(v["show"], 10, 64)
		sh, err := s.visibleShow(rv.r, id)
		if err != nil {
			return nil, sdui.ErrNotFound
		}
		it := rv.show(sh)
		// watched.set and watched.clear take the show's ID too (show_watched.go).
		it.Can = []string{"watchLater", "watched"}
		it.Watched = sh.EpisodeCount > 0 && sh.Unwatched == 0
		if rv.acc.Can(library.FeatureSharing) {
			it.Can = append(it.Can, "share")
		}
		// Play starts the next episode to watch, or the first.
		if next, _ := s.Store.NextUpItems(rv.ctx, rv.u.ID, []int64{sh.ID}, rv.acc, 1); len(next) > 0 {
			it.NextID = itoa(next[0].ID)
		} else if first, _, _ := s.Store.Items(rv.ctx, library.ItemQuery{ShowID: sh.ID, Access: rv.acc, Limit: 1, NoTotal: true, Brief: true}); len(first) > 0 {
			it.NextID = itoa(first[0].ID)
		}
		if it.NextID != "" {
			it.Can = append([]string{"play"}, it.Can...)
		}
		if rv.noPlay {
			rv.unplayable(&it)
		}
		rv.subject = &it
	case v["item"] != "":
		id, _ := strconv.ParseInt(v["item"], 10, 64)
		raw, err := s.visibleItem(rv.r, id)
		if err != nil {
			return nil, sdui.ErrNotFound
		}
		_ = s.Store.AttachProgress(rv.ctx, rv.u.ID, []*library.Item{raw})
		it := rv.item(raw)
		rv.subject = &it
	}
	if rv.subject == nil {
		return nil, sdui.ErrNotFound
	}
	return rv.subject, nil
}

// Seasons lists a show's seasons.
func (rv *sduiResolver) Seasons(show string) ([]sdui.Item, error) {
	id, _ := strconv.ParseInt(strings.TrimPrefix(show, "show-"), 10, 64)
	if _, err := rv.s.visibleShow(rv.r, id); err != nil {
		return nil, nil
	}
	seasons, err := rv.s.Store.Seasons(rv.ctx, id, rv.u.ID, rv.acc)
	if err != nil {
		return nil, err
	}
	out := make([]sdui.Item, 0, len(seasons))
	for _, se := range seasons {
		title := se.Title
		if title == "" {
			title = "Season " + strconv.Itoa(se.Number)
			if se.Number == 0 {
				title = "Specials"
			}
		}
		out = append(out, sdui.Item{ID: "season-" + strconv.Itoa(se.Number), Kind: "season", Season: se.Number, Title: title, Count: se.EpisodeCount, Unwatched: se.Unwatched})
	}
	return out, nil
}

// Facets gives a library's filter choices, from the library page's own facets.
func (rv *sduiResolver) Facets(lib string, fields []string) ([]sdui.Facet, error) {
	l, ok := rv.library(lib)
	if !ok {
		return nil, nil
	}
	f, err := rv.s.Store.LibraryFacets(rv.ctx, l.ID, rv.acc)
	if err != nil || f == nil {
		return nil, err
	}
	options := func(list []library.Facet, max int) []sdui.EnumOption {
		var out []sdui.EnumOption
		for i, x := range list {
			if i >= max || x.Value == "" {
				break
			}
			out = append(out, sdui.EnumOption{Value: x.Value, Label: firstNonEmpty(x.Label, x.Value)})
		}
		return out
	}
	var out []sdui.Facet
	for _, field := range fields {
		switch field {
		case "genre":
			out = append(out, sdui.Facet{ID: field, Label: "Genre", Options: options(f.Genres, 60)})
		case "decade":
			opts := options(f.Decades, 20)
			for i := range opts {
				if opts[i].Label == opts[i].Value {
					opts[i].Label += "s"
				}
			}
			out = append(out, sdui.Facet{ID: field, Label: "Decade", Options: opts})
		case "year":
			out = append(out, sdui.Facet{ID: field, Label: "Year", Options: options(f.Years, 80)})
		case "content":
			out = append(out, sdui.Facet{ID: field, Label: "Rating", Options: options(f.Content, 30)})
		case "resolution":
			out = append(out, sdui.Facet{ID: field, Label: "Resolution", Options: options(f.Resolutions, 8)})
		case "studio":
			out = append(out, sdui.Facet{ID: field, Label: "Studio", Options: options(f.Studios, 60)})
		case "network":
			out = append(out, sdui.Facet{ID: field, Label: "Network", Options: options(f.Networks, 60)})
		case "tag":
			out = append(out, sdui.Facet{ID: field, Label: "Tag", Options: options(f.Tags, 60)})
		case "watched":
			out = append(out, sdui.Facet{ID: field, Label: "Watched", Options: []sdui.EnumOption{{Value: "no", Label: "Unwatched"}, {Value: "yes", Label: "Watched"}}})
		case "service":
			// The streaming services that list titles of this library this profile may see,
			// most first; none while listings are off, and then the filter is left out.
			ps, err := rv.s.Store.ListingPlatforms(rv.ctx, l.ID, rv.acc)
			if err != nil || len(ps) == 0 {
				continue
			}
			var opts []sdui.EnumOption
			for i, p := range ps {
				if i >= 30 {
					break
				}
				opts = append(opts, sdui.EnumOption{Value: p.Key, Label: firstNonEmpty(p.Name, p.Key)})
			}
			out = append(out, sdui.Facet{ID: field, Label: "Streaming on", Options: opts})
		}
	}
	return out, nil
}

// sduiTopShelfRows are the rows a tvOS Top Shelf shows: what this account was in the
// middle of, then what's new in the first libraries it may open.
func (s *Server) sduiTopShelfRows(r *http.Request) []sdui.TopShelfRow {
	rv := s.newSDUIResolver(r, map[string]string{})
	limits := &sdui.RenderContext{}
	s.sduiLimits(r, limits)
	rv.noPlay = limits.Flags["screenTimeUp"]
	var rows []sdui.TopShelfRow
	if res, _ := rv.Items(sdui.SourceRequest{Source: sdui.Source{"type": "continueWatching", "media": "watch"}, Limit: sdui.MaxTopShelfItems}); len(res.Items) > 0 {
		rows = append(rows, sdui.TopShelfRow{Title: "Continue watching", Items: res.Items})
	}
	rows = append(rows, s.topShelfModuleRows(r, rv)...)
	for _, lib := range rv.libraries() {
		if len(rows) >= sdui.MaxTopShelfSections {
			break
		}
		if res := rv.libraryRows(&lib, "added", true, sdui.SourceRequest{Limit: sdui.MaxTopShelfItems}, ""); len(res.Items) > 0 {
			rows = append(rows, sdui.TopShelfRow{Title: "New in " + lib.Name, Items: res.Items})
		}
	}
	return rows
}

func firstNonZero(v ...int64) int64 {
	for _, x := range v {
		if x != 0 {
			return x
		}
	}
	return 0
}
