// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package watchxref answers "where can I watch this?" for one title: whether it's in
// this library (and in what quality and versions), next to where the outside services
// offer it in the viewer's region, to stream, rent or buy.
//
// The outside part comes from a Provider. Only TMDB's watch providers (whose data is
// JustWatch's) is built in, through the TMDB client the listings already use; another
// source (Watchmode, say) is a new Provider and nothing else. Answers are kept as long as
// streaming listings are, and forgotten when the listings are checked again or turned off.
//
// Nothing here plays anything from those services or uses an account with them: it says
// where a title is, and links to the service's own page.
package watchxref

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Offer types, the groups an answer is sorted into.
const (
	TypeStream = "stream"
	TypeFree   = "free"
	TypeAds    = "ads"
	TypeRent   = "rent"
	TypeBuy    = "buy"
)

// Reasons for having no providers.
const (
	ReasonNoAPIKey    = "no-api-key"
	ReasonNotMatched  = "not-matched"
	ReasonUnavailable = "unavailable" // the source couldn't be reached; try again later
)

// Ref names a title at the outside source: kind is movie or tv, id TMDB's.
type Ref struct {
	Kind string `json:"type"`
	ID   string `json:"tmdbId"`
}

var tmdbID = regexp.MustCompile(`^[0-9]{1,10}$`)

// Valid says the ref can be asked about.
func (r Ref) Valid() bool { return (r.Kind == "movie" || r.Kind == "tv") && tmdbID.MatchString(r.ID) }

var regionRe = regexp.MustCompile(`^[A-Z]{2}$`)

// CleanRegion is a two-letter region in capitals, or def.
func CleanRegion(s, def string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if regionRe.MatchString(s) {
		return s
	}
	return def
}

// Offer is one service offering a title one way.
type Offer struct {
	ProviderID int    `json:"providerId"`
	Key        string `json:"key"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Logo       string `json:"logo,omitempty"`
	Priority   int    `json:"priority"`
}

// Offers is what a Provider found.
type Offers struct {
	Link   string  `json:"link,omitempty"`
	Offers []Offer `json:"offers"`
}

// Provider is an outside source of where titles can be watched.
type Provider interface {
	// Name is a short id, used to keep its answers apart: "tmdb".
	Name() string
	// Attribution is the credit the source asks for, shown wherever its data is.
	Attribution() string
	// Available says whether it can be asked now (has its key); reason says why not.
	Available() (ok bool, reason string)
	// Offers reads where a title can be watched in a region.
	Offers(ctx context.Context, ref Ref, region string) (*Offers, error)
}

// Cache keeps answers, by provider, title and region.
type Cache interface {
	Get(ctx context.Context, source, kind, id, region string) (body string, fetchedAt int64, ok bool)
	Put(ctx context.Context, source, kind, id, region, body string) error
}

// Version is one copy of a title in the library.
type Version struct {
	ItemID    int64  `json:"itemId"`
	Label     string `json:"label,omitempty"`
	Kind      string `json:"kind"`
	Quality   string `json:"quality,omitempty"` // 4K, 1080p, 720p, SD, DVD, Blu-ray
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Container string `json:"container,omitempty"`
	Codec     string `json:"videoCodec,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
}

// Local is what this library has of the title.
type Local struct {
	ItemID   int64     `json:"itemId,omitempty"`
	ShowID   int64     `json:"showId,omitempty"`
	Title    string    `json:"title,omitempty"`
	Quality  string    `json:"quality,omitempty"` // the best of the versions
	Versions []Version `json:"versions"`
}

// ProviderGroup is the offers of one type.
type ProviderGroup struct {
	Type      string  `json:"type"`
	Providers []Offer `json:"providers"`
}

// Answer is what the routes send.
type Answer struct {
	InLibrary bool            `json:"inLibrary"`
	ItemID    *int64          `json:"itemId,omitempty"`
	ShowID    *int64          `json:"showId,omitempty"`
	Local     *Local          `json:"local,omitempty"`
	Region    string          `json:"region"`
	TMDB      *Ref            `json:"tmdb,omitempty"`
	Source    string          `json:"source,omitempty"`
	Link      string          `json:"link,omitempty"`
	Groups    []ProviderGroup `json:"groups"`
	Providers []Offer         `json:"providers"`
	// Reason says why there are no providers: no-api-key, not-matched, unavailable.
	Reason      string `json:"reason,omitempty"`
	Attribution string `json:"attribution,omitempty"`
	// FetchedAt is when the outside answer was read (Unix seconds); 0 when there's none.
	FetchedAt int64 `json:"fetchedAt"`
	Cached    bool  `json:"cached"`
}

// Service merges the library with the providers.
type Service struct {
	Providers []Provider
	Cache     Cache
	MaxAge    time.Duration
	Now       func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// typeOrder is how the groups are listed: included with what someone already pays for
// first, then free, then paid.
var typeOrder = map[string]int{TypeStream: 0, TypeFree: 1, TypeAds: 2, TypeRent: 3, TypeBuy: 4}

// Lookup builds the answer for a title. local is what the library has (nil for nothing);
// ref is the title at the outside source (zero when not known).
func (s *Service) Lookup(ctx context.Context, local *Local, ref Ref, region string) Answer {
	a := Answer{Region: region, Groups: []ProviderGroup{}, Providers: []Offer{}}
	if local != nil && (local.ItemID != 0 || local.ShowID != 0) {
		a.InLibrary, a.Local = true, local
		if local.ItemID != 0 {
			id := local.ItemID
			a.ItemID = &id
		}
		if local.ShowID != 0 {
			id := local.ShowID
			a.ShowID = &id
		}
		if local.Versions == nil {
			local.Versions = []Version{}
		}
	}
	if len(s.Providers) == 0 {
		a.Reason = ReasonNoAPIKey
		return a
	}
	if !ref.Valid() {
		a.Reason = ReasonNotMatched
		if ok, why := s.Providers[0].Available(); !ok {
			a.Reason = why
		}
		return a
	}
	r := ref
	a.TMDB = &r
	var merged []Offer
	reason := ""
	for _, p := range s.Providers {
		if ok, why := p.Available(); !ok {
			if reason == "" {
				reason = why
			}
			continue
		}
		got, at, cached, err := s.fetch(ctx, p, ref, region)
		if err != nil {
			reason = ReasonUnavailable
			continue
		}
		if a.Source == "" {
			a.Source, a.Attribution, a.Link, a.FetchedAt, a.Cached = p.Name(), p.Attribution(), got.Link, at, cached
		}
		merged = append(merged, got.Offers...)
	}
	if a.Source == "" {
		a.Reason = reason
		return a
	}
	a.Providers, a.Groups = group(merged)
	return a
}

// fetch reads a provider's answer, from the cache when it's fresh enough.
func (s *Service) fetch(ctx context.Context, p Provider, ref Ref, region string) (*Offers, int64, bool, error) {
	if s.Cache != nil {
		if body, at, ok := s.Cache.Get(ctx, p.Name(), ref.Kind, ref.ID, region); ok && (s.MaxAge <= 0 || s.now().Sub(time.Unix(at, 0)) < s.MaxAge) {
			var o Offers
			if json.Unmarshal([]byte(body), &o) == nil {
				if o.Offers == nil {
					o.Offers = []Offer{}
				}
				return &o, at, true, nil
			}
		}
	}
	o, err := p.Offers(ctx, ref, region)
	if err != nil {
		return nil, 0, false, err
	}
	if o == nil {
		o = &Offers{}
	}
	if o.Offers == nil {
		o.Offers = []Offer{}
	}
	at := s.now().Unix()
	if s.Cache != nil {
		if b, err := json.Marshal(o); err == nil {
			_ = s.Cache.Put(ctx, p.Name(), ref.Kind, ref.ID, region, string(b))
		}
	}
	return o, at, false, nil
}

// group drops repeats (one service, one way) and sorts the offers into their groups, in
// the order the source ranks them.
func group(all []Offer) ([]Offer, []ProviderGroup) {
	seen := map[string]bool{}
	flat := []Offer{}
	for _, o := range all {
		if _, ok := typeOrder[o.Type]; !ok || o.Key == "" || seen[o.Type+"|"+o.Key] {
			continue
		}
		seen[o.Type+"|"+o.Key] = true
		flat = append(flat, o)
	}
	sort.SliceStable(flat, func(i, j int) bool {
		if typeOrder[flat[i].Type] != typeOrder[flat[j].Type] {
			return typeOrder[flat[i].Type] < typeOrder[flat[j].Type]
		}
		return flat[i].Priority < flat[j].Priority
	})
	groups := []ProviderGroup{}
	for _, o := range flat {
		if n := len(groups); n == 0 || groups[n-1].Type != o.Type {
			groups = append(groups, ProviderGroup{Type: o.Type, Providers: []Offer{}})
		}
		groups[len(groups)-1].Providers = append(groups[len(groups)-1].Providers, o)
	}
	return flat, groups
}

// Quality names a picture's size the way people do.
func Quality(kind string, width, height int) string {
	switch {
	case strings.HasPrefix(kind, "bluray"):
		if width >= 3200 || height >= 1800 {
			return "4K"
		}
		return "Blu-ray"
	case strings.HasPrefix(kind, "dvd"):
		return "DVD"
	case width >= 3200 || height >= 1800:
		return "4K"
	case width >= 1700 || height >= 1000:
		return "1080p"
	case width >= 1200 || height >= 700:
		return "720p"
	case width > 0 || height > 0:
		return "SD"
	}
	return ""
}

var qualityRank = map[string]int{"": 0, "SD": 1, "DVD": 2, "720p": 3, "1080p": 4, "Blu-ray": 5, "4K": 6}

// Best is the better of two qualities.
func Best(a, b string) string {
	if qualityRank[b] > qualityRank[a] {
		return b
	}
	return a
}

// ErrNoProvider: nothing to ask.
var ErrNoProvider = errors.New("no where-to-watch source is set up")
