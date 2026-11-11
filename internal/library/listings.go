// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Streaming listings: which of the titles on this server a streaming service currently
// lists, so someone used to a service can find what they have that it also has. Nothing
// ever plays from those services and no account with them is used: it only sorts local
// media. It's off until the owner has read and accepted the disclosures, it needs the
// owner's own TMDB key, and every screen showing it says so and dates it.
//
// The data is TMDB's watch providers, which come from JustWatch. TMDB requires JustWatch
// to be credited as the source, and answers are kept for at most a few weeks, well inside
// the six months TMDB's terms allow.

// ListingsVersion goes up if what the owner accepted changes enough to ask again.
const ListingsVersion = "1"

// ListingAcknowledgements are the separate things the owner has to accept. Each one has
// to be sent back to turn the feature on.
var ListingAcknowledgements = []string{"not-streaming", "no-affiliation", "may-be-wrong", "sends-titles"}

// listingMaxAge is how old an answer can be before it's asked again.
const listingMaxAge = 14 * 24 * time.Hour

var regionRe = regexp.MustCompile(`^[A-Z]{2}$`)

// ListingsEnabled reports whether the owner has turned it on.
func (s *Store) ListingsEnabled(ctx context.Context) bool {
	return s.DB.GetSetting(ctx, "listingsConsent", "") == ListingsVersion
}

// ListingsRegion is the country listings are for.
func (s *Store) ListingsRegion(ctx context.Context) string {
	return s.DB.GetSetting(ctx, "listingsRegion", "US")
}

// ListingStatus says how the feature is doing.
type ListingStatus struct {
	Enabled bool   `json:"enabled"`
	Region  string `json:"region"`
	// AsOf is when the oldest answer was fetched and Newest the latest; both are for showing a date.
	AsOf             int64    `json:"asOf"`
	Newest           int64    `json:"newest"`
	HasKey           bool     `json:"hasKey"`
	Titles           int      `json:"titles"`
	Listed           int      `json:"listed"`
	Running          bool     `json:"running"`
	Done             int      `json:"done"`
	Total            int      `json:"total"`
	Source           string   `json:"source"`
	Attribution      string   `json:"attribution"`
	Acknowledgements []string `json:"acknowledgements"`
}

// ListingAttribution is the credit TMDB requires, in the words shown everywhere.
const ListingAttribution = "Listings from JustWatch, through TMDB. This product uses the TMDB API but is not endorsed or certified by TMDB."

// Status reads the state of the feature.
func (s *Scanner) ListingStatus(ctx context.Context) ListingStatus {
	st := ListingStatus{Enabled: s.Store.ListingsEnabled(ctx), Region: s.Store.ListingsRegion(ctx), Source: "JustWatch, through TMDB", Attribution: ListingAttribution, Acknowledgements: ListingAcknowledgements}
	st.HasKey = s.TMDBKey != nil && s.TMDBKey() != ""
	_ = s.Store.DB.QueryRowContext(ctx, `SELECT IFNULL(MIN(checked_at), 0), IFNULL(MAX(checked_at), 0), COUNT(*) FROM listing_titles WHERE region = ?`, st.Region).Scan(&st.AsOf, &st.Newest, &st.Titles)
	_ = s.Store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT DISTINCT target, target_id FROM listings WHERE region = ?)`, st.Region).Scan(&st.Listed)
	listingJob.Lock()
	st.Running, st.Done, st.Total = listingJob.running, listingJob.done, listingJob.total
	listingJob.Unlock()
	return st
}

// ErrListingsNotAccepted: every disclosure has to be accepted first.
var ErrListingsNotAccepted = errors.New("every statement has to be accepted before listings can be turned on")

// SetListings turns the feature on or off. Turning it off drops all its data.
func (s *Scanner) SetListings(ctx context.Context, on bool, region string, accepted []string) error {
	if region != "" {
		region = strings.ToUpper(strings.TrimSpace(region))
		if !regionRe.MatchString(region) {
			return errors.New("a region is two letters, such as US")
		}
	}
	// Once they're on, what was accepted stands: changing the region doesn't ask again.
	already := s.Store.ListingsEnabled(ctx)
	if on {
		have := map[string]bool{}
		for _, a := range accepted {
			have[a] = true
		}
		for _, a := range ListingAcknowledgements {
			if !have[a] && !already {
				return ErrListingsNotAccepted
			}
		}
		if s.TMDBKey == nil || s.TMDBKey() == "" {
			return errors.New("listings need a free TMDB key first: add one under Built-in agents")
		}
	}
	if region != "" && region != s.Store.ListingsRegion(ctx) {
		if err := s.Store.DB.SetSetting(ctx, "listingsRegion", region); err != nil {
			return err
		}
	}
	if !on {
		StopListings()
		if err := s.Store.DB.SetSetting(ctx, "listingsConsent", "0"); err != nil {
			return err
		}
		if err := s.Store.ClearWatchCache(ctx); err != nil {
			return err
		}
		if _, err := s.Store.DB.ExecContext(ctx, `DELETE FROM listings`); err != nil {
			return err
		}
		_, err := s.Store.DB.ExecContext(ctx, `DELETE FROM listing_titles`)
		return err
	}
	if !already {
		if err := s.Store.DB.SetSetting(ctx, "listingsConsent", ListingsVersion); err != nil {
			return err
		}
		if err := s.Store.DB.SetSetting(ctx, "listingsConsentAt", strconv.FormatInt(db.Now(), 10)); err != nil {
			return err
		}
	}
	s.KickListings()
	return nil
}

// ListingPlatform is a service with what it lists from a library.
type ListingPlatform struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ListingPlatforms lists the services with titles here, most first, counting only what
// this viewer may see.
func (s *Store) ListingPlatforms(ctx context.Context, libraryID int64, access *Access) ([]ListingPlatform, error) {
	if !s.ListingsEnabled(ctx) {
		return []ListingPlatform{}, nil
	}
	region := s.ListingsRegion(ctx)
	ic, ia := access.ItemClause()
	sc, sa := access.ShowClause()
	args := []any{region, libraryID, libraryID}
	args = append(args, ia...)
	args = append(args, region, libraryID, libraryID)
	args = append(args, sa...)
	rows, err := s.DB.QueryContext(ctx, `SELECT platform_key, MIN(platform), COUNT(*) FROM (
		SELECT l.platform_key, l.platform FROM listings l JOIN items i ON l.target = 'item' AND i.id = l.target_id
		  WHERE l.region = ? AND (? = 0 OR i.library_id = ?) AND i.missing = 0 AND `+hideLinked+` AND `+ic+`
		UNION ALL
		SELECT l.platform_key, l.platform FROM listings l JOIN shows s ON l.target = 'show' AND s.id = l.target_id
		  WHERE l.region = ? AND (? = 0 OR s.library_id = ?) AND s.missing = 0 AND `+sc+`
		) GROUP BY platform_key`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ListingPlatform{}
	for rows.Next() {
		var p ListingPlatform
		if err := rows.Scan(&p.Key, &p.Name, &p.Count); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, rows.Err()
}

func (s *Store) platformNames(ctx context.Context) map[string]string {
	out := map[string]string{}
	rows, err := s.DB.QueryContext(ctx, `SELECT platform_key, MIN(platform) FROM listings GROUP BY platform_key`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, n string
		if rows.Scan(&k, &n) == nil {
			out[k] = n
		}
	}
	return out
}

// TitleListings says which services list one title.
func (s *Store) TitleListings(ctx context.Context, target string, id int64) ([]ListingPlatform, int64) {
	out := []ListingPlatform{}
	if !s.ListingsEnabled(ctx) {
		return out, 0
	}
	region := s.ListingsRegion(ctx)
	rows, err := s.DB.QueryContext(ctx, `SELECT platform_key, platform FROM listings WHERE target = ? AND target_id = ? AND region = ? ORDER BY platform COLLATE NOCASE`, target, id, region)
	if err != nil {
		return out, 0
	}
	defer rows.Close()
	for rows.Next() {
		var p ListingPlatform
		if rows.Scan(&p.Key, &p.Name) == nil {
			out = append(out, p)
		}
	}
	var at int64
	_ = s.DB.QueryRowContext(ctx, `SELECT checked_at FROM listing_titles WHERE target = ? AND target_id = ? AND region = ?`, target, id, region).Scan(&at)
	return out, at
}

var listingJob = struct {
	sync.Mutex
	running     bool
	again       bool // asked for while running: go round once more, for what was forgotten since
	done, total int
	cancel      context.CancelFunc
}{}

// forgetListing drops what's known of one title's listings when its match changes. They were
// read for whatever it was matched with before, and would otherwise be shown for weeks.
func (s *Store) forgetListing(ctx context.Context, target string, id int64) {
	for _, table := range []string{"listings", "listing_titles"} {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM `+table+` WHERE target = ? AND target_id = ?`, target, id); err != nil {
			return
		}
	}
}

// StopListings stops the background job.
func StopListings() {
	listingJob.Lock()
	defer listingJob.Unlock()
	if listingJob.cancel != nil {
		listingJob.cancel()
	}
}

// KickListings checks whatever hasn't been checked lately, in the background and
// gently. It does nothing if the owner hasn't turned listings on.
func (s *Scanner) KickListings() { s.kickListings(false) }

// RecheckListings checks everything again now, however lately it was checked, and forgets
// the kept where-to-watch answers so that they're asked for again too.
func (s *Scanner) RecheckListings() {
	_ = s.Store.ClearWatchCache(context.Background())
	s.kickListings(true)
}

func (s *Scanner) kickListings(all bool) {
	ctx := context.Background()
	if !s.Store.ListingsEnabled(ctx) || s.TMDBKey == nil || s.TMDBKey() == "" {
		return
	}
	listingJob.Lock()
	defer listingJob.Unlock()
	if listingJob.running {
		listingJob.again = true
		return
	}
	jctx, cancel := context.WithCancel(ctx)
	listingJob.running, listingJob.done, listingJob.total, listingJob.cancel = true, 0, 0, cancel
	t := s.Tasks.Start(tasks.Spec{Type: tasks.TypeMetadata, Title: "Updating streaming listings", Unit: "titles", Cancel: cancel, Quiet: true})
	jctx = context.WithValue(jctx, taskKey{}, t)
	logging.Go("streaming listings", func() {
		var err error
		defer func() {
			t.Finish(err)
			cancel()
			listingJob.Lock()
			listingJob.running = false
			again := listingJob.again
			listingJob.again = false
			listingJob.Unlock()
			if again {
				s.KickListings()
			}
		}()
		if err = s.syncListings(jctx, all); err != nil && !errors.Is(err, context.Canceled) {
			s.Log.WarnContext(jctx, "could not update streaming listings", "err", err)
		}
	}, "cat", "library")
}

type listingTodo struct {
	target string
	id     int64
}

func (s *Scanner) syncListings(ctx context.Context, all bool) error {
	region := s.Store.ListingsRegion(ctx)
	cutoff := time.Now().Add(-listingMaxAge).Unix()
	if all {
		cutoff = time.Now().Unix() + 1
	}
	var todo []listingTodo
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT 'item', i.id FROM items i LEFT JOIN listing_titles t ON t.target = 'item' AND t.target_id = i.id AND t.region = ?1
		WHERE i.missing = 0 AND i.show_id IS NULL AND i.kind IN ('video', 'dvd-iso', 'dvd-folder', 'bluray-iso', 'bluray-folder') AND i.library_id IN (SELECT id FROM libraries WHERE type IN ('movies', 'discs'))
		  AND `+hideLinked+` AND (t.checked_at IS NULL OR t.checked_at < ?2)
		UNION ALL
		SELECT 'show', s.id FROM shows s LEFT JOIN listing_titles t ON t.target = 'show' AND t.target_id = s.id AND t.region = ?1
		WHERE s.missing = 0 AND (t.checked_at IS NULL OR t.checked_at < ?2)`, region, cutoff)
	if err != nil {
		return err
	}
	for rows.Next() {
		var t listingTodo
		if err := rows.Scan(&t.target, &t.id); err != nil {
			rows.Close()
			return err
		}
		todo = append(todo, t)
	}
	rows.Close()
	listingJob.Lock()
	listingJob.total = len(todo)
	listingJob.Unlock()
	tm := &TMDB{Key: s.TMDBKey(), Base: s.TMDBBase}
	for i, t := range todo {
		if err := ctx.Err(); err != nil {
			return err
		}
		listingJob.Lock()
		listingJob.done = i
		listingJob.Unlock()
		scanTask(ctx).Progress(int64(i), int64(len(todo)))
		if err := s.checkListing(ctx, tm, t, region); err != nil {
			s.Log.DebugContext(ctx, "no listing", "target", t.target, "id", t.id, "err", err)
		}
		select {
		case <-time.After(300 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	listingJob.Lock()
	listingJob.done = len(todo)
	listingJob.Unlock()
	return nil
}

// checkListing finds a title on TMDB and reads which services list it. A title that
// isn't matched on TMDB is looked for by name, and only taken if that's nearly certain:
// a wrong guess here would say something on a service when it isn't.
func (s *Scanner) checkListing(ctx context.Context, tm *TMDB, td listingTodo, region string) error {
	t, err := s.Store.LoadTitle(ctx, td.target, td.id)
	if err != nil {
		return err
	}
	kind, id := "", ""
	provider, pid := t.Provider()
	switch provider {
	case "tmdb":
		kind, id = "movie", pid
	case "tmdb-tv":
		kind, id = "tv", pid
	}
	if id == "" && t.Meta.MatchState != MatchNone {
		var c []MatchCandidate
		var year *int
		if t.Item != nil {
			year = t.Item.Year
			c, err = (&tmdbMovies{*tm}).Search(ctx, t.Name(), year)
		} else {
			year = t.Show.Year
			c, err = (&tmdbShows{*tm}).Search(ctx, t.Name(), year)
		}
		if err != nil {
			return err
		}
		best, bestScore := "", 0
		for _, x := range c {
			sc, _ := scoreMatch(t.Name(), year, "", x.Title, x.Year, x.Kind)
			if sc > bestScore {
				best, bestScore = x.ID, sc
			}
		}
		if bestScore >= 90 {
			id = best
			kind = "movie"
			if t.Show != nil {
				kind = "tv"
			}
		}
	}
	var offers []listingOffer
	if id != "" {
		if offers, err = tm.watchProviders(ctx, kind, id, region); err != nil {
			return err
		}
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM listings WHERE target = ? AND target_id = ? AND region = ?`, td.target, td.id, region); err != nil {
		return err
	}
	for _, o := range offers {
		if _, err := tx.ExecContext(ctx, `INSERT INTO listings(target, target_id, region, platform_key, platform, offer) VALUES(?,?,?,?,?,?) ON CONFLICT DO NOTHING`, td.target, td.id, region, o.key, o.name, o.offer); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO listing_titles(target, target_id, region, checked_at, tmdb_kind, tmdb_id) VALUES(?,?,?,?,?,?)
		ON CONFLICT(target, target_id, region) DO UPDATE SET checked_at = excluded.checked_at, tmdb_kind = excluded.tmdb_kind, tmdb_id = excluded.tmdb_id`, td.target, td.id, region, db.Now(), kind, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, o := range offers {
		_, _ = s.Store.EnsureLogo(ctx, LogoPlatform, o.name)
	}
	return nil
}

type listingOffer struct{ key, name, offer string }

// watchProviders reads which services include a title with a subscription, for free or with
// adverts. Rentals and purchases aren't "what's on" a service, so they're left out.
func (t *TMDB) watchProviders(ctx context.Context, kind, id, region string) ([]listingOffer, error) {
	all, err := t.WatchOffers(ctx, kind, id, region)
	if err != nil || all == nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []listingOffer
	for _, o := range all.Offers {
		offer := ""
		switch o.Type {
		case WatchStream:
			offer = "subscription"
		case WatchFree, WatchAds:
			offer = o.Type
		default:
			continue
		}
		if !seen[o.Key] && len(out) < 50 {
			seen[o.Key] = true
			out = append(out, listingOffer{o.Key, o.Name, offer})
		}
	}
	return out, nil
}

type tmdbProvider struct {
	ID       int    `json:"provider_id"`
	Name     string `json:"provider_name"`
	Logo     string `json:"logo_path"`
	Priority int    `json:"display_priority"`
}

// Kinds of watch offer.
const (
	WatchStream = "stream" // with a subscription
	WatchFree   = "free"
	WatchAds    = "ads"
	WatchRent   = "rent"
	WatchBuy    = "buy"
)

// WatchOffer is one service offering a title one way.
type WatchOffer struct {
	ProviderID int    `json:"providerId"`
	Key        string `json:"key"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Logo       string `json:"logo,omitempty"` // a full address on TMDB's image host
	Priority   int    `json:"priority"`
}

// WatchOffers is everything TMDB says about a title in one region.
type WatchOffers struct {
	// Link is TMDB's page for the title's offers in the region, which links on to each service.
	Link   string       `json:"link,omitempty"`
	Offers []WatchOffer `json:"offers"`
}

// WatchOffers reads every way a title can be watched in a region: kind is movie or tv, id
// TMDB's. Nothing known in the region is an empty answer, not an error. The data is
// JustWatch's, and has to be credited as such (ListingAttribution).
func (t *TMDB) WatchOffers(ctx context.Context, kind, id, region string) (*WatchOffers, error) {
	if kind != "movie" && kind != "tv" {
		return nil, errors.New("kind is movie or tv")
	}
	var r struct {
		Results map[string]struct {
			Link     string         `json:"link"`
			Flatrate []tmdbProvider `json:"flatrate"`
			Free     []tmdbProvider `json:"free"`
			Ads      []tmdbProvider `json:"ads"`
			Rent     []tmdbProvider `json:"rent"`
			Buy      []tmdbProvider `json:"buy"`
		} `json:"results"`
	}
	if err := t.get(ctx, "/"+kind+"/"+url.PathEscape(id)+"/watch/providers", url.Values{}, &r); err != nil {
		return nil, err
	}
	out := &WatchOffers{Offers: []WatchOffer{}}
	res, ok := r.Results[region]
	if !ok {
		return out, nil
	}
	if u, err := url.Parse(res.Link); err == nil && u.Scheme == "https" && (u.Host == "www.themoviedb.org" || u.Host == "themoviedb.org") {
		out.Link = res.Link
	}
	seen := map[string]bool{}
	add := func(list []tmdbProvider, typ string) {
		for _, p := range list {
			name := cleanBrandName(p.Name)
			k := BrandKey(name)
			if k == "" || seen[typ+"|"+k] || len(out.Offers) >= 200 {
				continue
			}
			seen[typ+"|"+k] = true
			o := WatchOffer{ProviderID: p.ID, Key: k, Name: name, Type: typ, Priority: p.Priority}
			if strings.HasPrefix(p.Logo, "/") && !strings.Contains(p.Logo, "..") && len(p.Logo) < 200 {
				o.Logo = tmdbImage + "w92" + p.Logo
			}
			out.Offers = append(out.Offers, o)
		}
	}
	add(res.Flatrate, WatchStream)
	add(res.Free, WatchFree)
	add(res.Ads, WatchAds)
	add(res.Rent, WatchRent)
	add(res.Buy, WatchBuy)
	return out, nil
}

// TMDBClient is a TMDB client with the scanner's key, or nil without one.
func (s *Scanner) TMDBClient() *TMDB {
	if s.TMDBKey == nil || s.TMDBKey() == "" {
		return nil
	}
	return &TMDB{Key: s.TMDBKey(), Base: s.TMDBBase}
}

// ListingTMDB says which TMDB title the listings matched a local title with, if any.
func (s *Store) ListingTMDB(ctx context.Context, target string, id int64) (kind, tmdbID string) {
	_ = s.DB.QueryRowContext(ctx, `SELECT tmdb_kind, tmdb_id FROM listing_titles WHERE target = ? AND target_id = ? AND tmdb_id != '' ORDER BY checked_at DESC LIMIT 1`, target, id).Scan(&kind, &tmdbID)
	return
}

// ------------------------------------------------------------------ where to watch cache

// WatchCacheMaxAge is how long a where-to-watch answer is kept: the same as listings.
const WatchCacheMaxAge = listingMaxAge

// WatchCacheGet reads a kept answer, and when it was fetched.
func (s *Store) WatchCacheGet(ctx context.Context, source, kind, id, region string) (string, int64, bool) {
	var body string
	var at int64
	err := s.DB.QueryRowContext(ctx, `SELECT body, fetched_at FROM watch_offers_cache WHERE source = ? AND kind = ? AND tmdb_id = ? AND region = ?`, source, kind, id, region).Scan(&body, &at)
	return body, at, err == nil
}

// WatchCachePut keeps an answer. Old answers are dropped on the way.
func (s *Store) WatchCachePut(ctx context.Context, source, kind, id, region, body string) error {
	now := db.Now()
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM watch_offers_cache WHERE fetched_at < ?`, now-int64(WatchCacheMaxAge/time.Second)); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO watch_offers_cache(source, kind, tmdb_id, region, fetched_at, body) VALUES(?,?,?,?,?,?)
		ON CONFLICT(source, kind, tmdb_id, region) DO UPDATE SET fetched_at = excluded.fetched_at, body = excluded.body`, source, kind, id, region, now, body)
	return err
}

// ClearWatchCache forgets every kept answer, as checking the listings again does.
func (s *Store) ClearWatchCache(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM watch_offers_cache`)
	return err
}

// ByTMDB finds the titles here matched with a TMDB id: films (and discs) for movie, shows
// for tv. Missing ones are left out. Who may see them is for the caller to check.
func (s *Store) ByTMDB(ctx context.Context, kind, id string) (items []int64, shows []int64) {
	q := `SELECT id FROM items WHERE provider = 'tmdb' AND provider_id = ? AND missing = 0 AND show_id IS NULL ORDER BY id LIMIT 20`
	if kind == "tv" {
		q = `SELECT id FROM shows WHERE provider = 'tmdb-tv' AND provider_id = ? AND missing = 0 ORDER BY id LIMIT 20`
	}
	rows, err := s.DB.QueryContext(ctx, q, id)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()
	for rows.Next() {
		var n int64
		if rows.Scan(&n) == nil {
			if kind == "tv" {
				shows = append(shows, n)
			} else {
				items = append(items, n)
			}
		}
	}
	return items, shows
}
