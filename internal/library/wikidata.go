// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Wikidata fills in basic facts about films when there is no TMDB key: the
// year, genres, the US rating (so parental controls work) and a short plot
// from the film's English Wikipedia article. No key is needed. Wikidata is
// public domain (CC0); Wikipedia text is CC BY-SA 4.0 and credited in the
// app. A poster comes only from Wikimedia Commons, where a film has a free one,
// credited with its licence and author (commons_posters.go).
type Wikidata struct {
	Client     *http.Client
	API        string // tests
	CommonsAPI string // tests
}

const wikiUA = "WittyMediaServer/0.4 (https://github.com/wittywilla/WittyMediaServer)"

// One request at a time with a short gap, as the Wikimedia API asks.
var wikiGate = struct {
	sync.Mutex
	next time.Time
}{}

func (w *Wikidata) get(ctx context.Context, u string, out any) error {
	wikiGate.Lock()
	wait := time.Until(wikiGate.next)
	wikiGate.next = time.Now().Add(max(wait, 0) + 200*time.Millisecond)
	wikiGate.Unlock()
	if wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c := w.Client
	if c == nil {
		c = catalogClient(15 * time.Second)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", wikiUA)
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 200))
		return fmt.Errorf("wikidata: %s: %s", res.Status, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(io.LimitReader(res.Body, maxAnswer)).Decode(out)
}

type wdEntity struct {
	ID           string                                 `json:"id"`
	Labels       map[string]struct{ Value string }      `json:"labels"`
	Descriptions map[string]struct{ Value string }      `json:"descriptions"`
	Sitelinks    map[string]struct{ Title string }      `json:"sitelinks"`
	Claims       map[string][]struct{ Mainsnak wdSnak } `json:"claims"`
}

type wdSnak struct {
	Datavalue struct {
		Value json.RawMessage `json:"value"`
	} `json:"datavalue"`
}

func (s wdSnak) itemID() string {
	var v struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(s.Datavalue.Value, &v)
	return v.ID
}

func (s wdSnak) time() string {
	var v struct {
		Time string `json:"time"`
	}
	_ = json.Unmarshal(s.Datavalue.Value, &v)
	return strings.TrimPrefix(v.Time, "+")
}

func (s wdSnak) amount() float64 {
	var v struct {
		Amount string `json:"amount"`
	}
	_ = json.Unmarshal(s.Datavalue.Value, &v)
	f, _ := strconv.ParseFloat(strings.TrimPrefix(v.Amount, "+"), 64)
	return f
}

const (
	wdPublication  = "P577"
	wdGenre        = "P136"
	wdMPARating    = "P1657"
	wdDuration     = "P2047"
	wdInstanceOf   = "P31"
	wdPartOfSeries = "P179"
)

func (w *Wikidata) entities(ctx context.Context, ids []string, props string) (map[string]wdEntity, error) {
	var res struct {
		Entities map[string]wdEntity `json:"entities"`
	}
	q := url.Values{"action": {"wbgetentities"}, "ids": {strings.Join(ids, "|")}, "props": {props}, "languages": {"en"}, "sitefilter": {"enwiki"}, "format": {"json"}}
	err := w.get(ctx, w.api()+"?"+q.Encode(), &res)
	return res.Entities, err
}

// Enrich fills empty fields of a movie item. It changes nothing when no
// film of that name (and year) is found.
func (w *Wikidata) Enrich(ctx context.Context, it *Item) error {
	var search struct {
		Search []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
		} `json:"search"`
	}
	q := url.Values{"action": {"wbsearchentities"}, "search": {it.Title}, "language": {"en"}, "type": {"item"}, "limit": {"10"}, "format": {"json"}}
	if err := w.get(ctx, w.api()+"?"+q.Encode(), &search); err != nil {
		return err
	}
	var cands []string
	byText := false // found by the full-text search, which finds series too
	for _, r := range search.Search {
		d := strings.ToLower(r.Description)
		if (strings.Contains(d, "film") || strings.Contains(d, "movie")) &&
			!containsAny(d, "series", "franchise", "trilogy", "soundtrack", "character", "festival", "studio", "company", "book") {
			cands = append(cands, r.ID)
		}
	}
	if len(cands) == 0 {
		// A name that doesn't start the way the film's entry does, such as "The Cabinet of Dr Caligari" for
		// "The Cabinet of Dr. Caligari": everything films are called is searched instead.
		ids, err := w.searchTextIDs(ctx, it.Title)
		if err != nil {
			return err
		}
		cands, byText = ids, true
	}
	if len(cands) == 0 {
		return ErrNotFound
	}
	ents, err := w.entities(ctx, cands, "claims|descriptions|sitelinks")
	if err != nil {
		return err
	}
	// The first film in search order whose year matches (or the first film
	// when the file name has no year).
	var best *wdEntity
	for _, id := range cands {
		e, ok := ents[id]
		if !ok {
			continue
		}
		if byText && !isA(e, wdFilm) {
			continue
		}
		if it.Year == nil || releasedNear(e, *it.Year) {
			best = &e
			break
		}
	}
	if best == nil {
		return ErrNotFound
	}
	if c := best.Claims[wdPublication]; len(c) > 0 && it.Year == nil {
		if y, err := strconv.Atoi(firstN(c[0].Mainsnak.time(), 4)); err == nil {
			it.Year = &y
		}
	}
	if c := best.Claims[wdDuration]; len(c) > 0 && it.RuntimeSec == 0 {
		it.RuntimeSec = c[0].Mainsnak.amount() * 60
	}
	// Genres and the rating are other Wikidata items: fetch their names.
	var refs []string
	for _, c := range best.Claims[wdGenre] {
		if id := c.Mainsnak.itemID(); id != "" && len(refs) < 8 {
			refs = append(refs, id)
		}
	}
	rating := ""
	if c := best.Claims[wdMPARating]; len(c) > 0 {
		rating = c[0].Mainsnak.itemID()
		refs = append(refs, rating)
	}
	series := map[string]bool{}
	for _, c := range best.Claims[wdPartOfSeries] {
		if id := c.Mainsnak.itemID(); id != "" && !series[id] && len(series) < 8 {
			series[id] = true
			refs = append(refs, id)
		}
	}
	if len(refs) > 0 {
		if names, err := w.entities(ctx, refs, "labels"); err == nil {
			var genres, seriesNames []string
			for _, id := range refs {
				l := names[id].Labels["en"].Value
				if l == "" {
					continue
				}
				if id == rating {
					if it.ContentRating == "" {
						it.ContentRating = strings.TrimSuffix(l, " rating") // e.g. "PG-13"
					}
					continue
				}
				if series[id] {
					seriesNames = append(seriesNames, l)
					continue
				}
				genres = append(genres, titleCase(strings.TrimSuffix(strings.TrimSuffix(l, " film"), " movie")))
			}
			it.series = pickSeries(seriesNames, it.Title)
			if len(it.Genres) == 0 {
				it.Genres = genres
			}
		}
	}
	if it.Overview == "" {
		if page := best.Sitelinks["enwiki"].Title; page != "" {
			var sum struct {
				Extract string `json:"extract"`
			}
			if w.get(ctx, "https://en.wikipedia.org/api/rest_v1/page/summary/"+url.PathEscape(strings.ReplaceAll(page, " ", "_")), &sum) == nil {
				it.Overview = sum.Extract
			}
		}
		if it.Overview == "" {
			it.Overview = best.Descriptions["en"].Value
		}
	}
	if it.Poster == "" {
		it.Poster = w.commonsPoster(ctx, *best)
	}
	if it.Provider == "" {
		it.Provider, it.ProviderID = "wikidata", best.ID
	}
	return nil
}

// isA: whether an entity is an instance of this kind of thing.
func isA(e wdEntity, kind string) bool {
	for _, c := range e.Claims[wdInstanceOf] {
		if c.Mainsnak.itemID() == kind {
			return true
		}
	}
	return false
}

// releasedNear: whether any of a film's release dates is within a year of this one. A film can have several:
// Plan 9 from Outer Space's first is its premiere in 1957, and a file names 1959, when it came out.
func releasedNear(e wdEntity, year int) bool {
	for _, c := range e.Claims[wdPublication] {
		if y, err := strconv.Atoi(firstN(c.Mainsnak.time(), 4)); err == nil && y != 0 && abs(y-year) <= 1 {
			return true
		}
	}
	return false
}

func firstN(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

func containsAny(s string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}
