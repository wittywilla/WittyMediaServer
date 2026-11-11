// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package watchxref

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeProvider struct {
	name   string
	ok     bool
	why    string
	offers *Offers
	err    error
	calls  int
}

func (f *fakeProvider) Name() string              { return f.name }
func (f *fakeProvider) Attribution() string       { return "from " + f.name }
func (f *fakeProvider) Available() (bool, string) { return f.ok, f.why }
func (f *fakeProvider) Offers(_ context.Context, _ Ref, _ string) (*Offers, error) {
	f.calls++
	return f.offers, f.err
}

type memCache map[string]struct {
	body string
	at   int64
}

func (m memCache) Get(_ context.Context, source, kind, id, region string) (string, int64, bool) {
	v, ok := m[source+kind+id+region]
	return v.body, v.at, ok
}
func (m memCache) Put(_ context.Context, source, kind, id, region, body string) error {
	m[source+kind+id+region] = struct {
		body string
		at   int64
	}{body, time.Now().Unix()}
	return nil
}

func fake() *fakeProvider {
	return &fakeProvider{name: "fake", ok: true, offers: &Offers{Link: "https://example.invalid/title", Offers: []Offer{
		{Key: "buyit", Name: "Buy It", Type: TypeBuy, Priority: 1},
		{Key: "streamb", Name: "Stream B", Type: TypeStream, Priority: 5},
		{Key: "streama", Name: "Stream A", Type: TypeStream, Priority: 1, Logo: "https://image.tmdb.org/t/p/w92/a.png"},
		{Key: "streama", Name: "Stream A", Type: TypeStream, Priority: 1}, // repeated
		{Key: "rentit", Name: "Rent It", Type: TypeRent, Priority: 2},
		{Key: "freeit", Name: "Free It", Type: TypeFree, Priority: 3},
		{Key: "odd", Name: "Odd", Type: "lease"}, // a type nobody knows
		{Key: "", Name: "Nameless", Type: TypeStream},
	}}}
}

func TestLookupMerges(t *testing.T) {
	p := fake()
	s := &Service{Providers: []Provider{p}, Cache: memCache{}, MaxAge: time.Hour}
	local := &Local{ItemID: 7, Title: "Film", Versions: []Version{{ItemID: 7, Quality: "1080p"}, {ItemID: 8, Quality: "4K"}}, Quality: "4K"}
	a := s.Lookup(context.Background(), local, Ref{Kind: "movie", ID: "603"}, "GB")
	if !a.InLibrary || a.ItemID == nil || *a.ItemID != 7 || a.Local.Quality != "4K" || len(a.Local.Versions) != 2 {
		t.Errorf("local: %+v", a)
	}
	if a.Region != "GB" || a.Source != "fake" || a.Attribution != "from fake" || a.Link != "https://example.invalid/title" || a.Reason != "" {
		t.Errorf("answer: %+v", a)
	}
	var order []string
	for _, g := range a.Groups {
		for _, o := range g.Providers {
			order = append(order, g.Type+":"+o.Key)
		}
	}
	want := []string{"stream:streama", "stream:streamb", "free:freeit", "rent:rentit", "buy:buyit"}
	if len(order) != len(want) {
		t.Fatalf("groups %v", order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("groups %v, want %v", order, want)
			break
		}
	}
	if len(a.Providers) != 5 || len(a.Groups) != 4 {
		t.Errorf("providers %d groups %d", len(a.Providers), len(a.Groups))
	}
	if a.Cached || a.FetchedAt == 0 {
		t.Errorf("first answer cached=%v at=%d", a.Cached, a.FetchedAt)
	}
	// Asked again: from the cache, without asking the provider.
	b := s.Lookup(context.Background(), local, Ref{Kind: "movie", ID: "603"}, "GB")
	if !b.Cached || p.calls != 1 || len(b.Providers) != 5 {
		t.Errorf("second: cached=%v calls=%d", b.Cached, p.calls)
	}
	// Another region is asked for itself.
	s.Lookup(context.Background(), local, Ref{Kind: "movie", ID: "603"}, "US")
	if p.calls != 2 {
		t.Errorf("calls %d", p.calls)
	}
	// An old answer is asked for again.
	s.Now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	s.Lookup(context.Background(), local, Ref{Kind: "movie", ID: "603"}, "GB")
	if p.calls != 3 {
		t.Errorf("stale answer used: calls %d", p.calls)
	}
}

func TestLookupWithoutKey(t *testing.T) {
	s := &Service{Providers: []Provider{&fakeProvider{name: "fake", why: ReasonNoAPIKey}}}
	a := s.Lookup(context.Background(), &Local{ItemID: 3, Versions: []Version{}}, Ref{Kind: "movie", ID: "1"}, "US")
	if !a.InLibrary || a.Reason != ReasonNoAPIKey || a.Providers == nil || len(a.Providers) != 0 || a.Groups == nil {
		t.Errorf("%+v", a)
	}
	// Not in the library, not matched, no key: the key is the reason given.
	a = s.Lookup(context.Background(), nil, Ref{}, "US")
	if a.InLibrary || a.Reason != ReasonNoAPIKey || a.Local != nil {
		t.Errorf("%+v", a)
	}
	// With no provider at all.
	a = (&Service{}).Lookup(context.Background(), nil, Ref{Kind: "tv", ID: "1"}, "US")
	if a.Reason != ReasonNoAPIKey {
		t.Errorf("%+v", a)
	}
}

func TestLookupNotMatchedAndFailing(t *testing.T) {
	p := fake()
	s := &Service{Providers: []Provider{p}}
	if a := s.Lookup(context.Background(), &Local{ItemID: 1}, Ref{}, "US"); a.Reason != ReasonNotMatched || p.calls != 0 {
		t.Errorf("%+v", a)
	}
	if a := s.Lookup(context.Background(), nil, Ref{Kind: "movie", ID: "12; DROP"}, "US"); a.Reason != ReasonNotMatched || p.calls != 0 {
		t.Errorf("a bad id was asked about: %+v", a)
	}
	p.err = errors.New("down")
	a := s.Lookup(context.Background(), nil, Ref{Kind: "movie", ID: "5"}, "US")
	if a.Reason != ReasonUnavailable || len(a.Providers) != 0 {
		t.Errorf("%+v", a)
	}
}

func TestSecondProviderAdds(t *testing.T) {
	one := fake()
	two := &fakeProvider{name: "two", ok: true, offers: &Offers{Offers: []Offer{
		{Key: "streama", Name: "Stream A", Type: TypeStream}, // the same as the first's
		{Key: "streamc", Name: "Stream C", Type: TypeStream, Priority: 9},
	}}}
	a := (&Service{Providers: []Provider{one, two}}).Lookup(context.Background(), nil, Ref{Kind: "tv", ID: "9"}, "US")
	if a.Source != "fake" || len(a.Groups[0].Providers) != 3 {
		t.Errorf("%+v", a.Groups)
	}
}

func TestQuality(t *testing.T) {
	for _, c := range []struct {
		kind string
		w, h int
		want string
	}{
		{"video", 3840, 2160, "4K"}, {"video", 3840, 1600, "4K"}, {"video", 1920, 1080, "1080p"}, {"video", 1920, 800, "1080p"},
		{"video", 1280, 720, "720p"}, {"video", 720, 480, "SD"}, {"video", 0, 0, ""}, {"dvd-iso", 720, 480, "DVD"},
		{"bluray-folder", 1920, 1080, "Blu-ray"}, {"bluray-iso", 3840, 2160, "4K"},
	} {
		if got := Quality(c.kind, c.w, c.h); got != c.want {
			t.Errorf("%s %dx%d: %q", c.kind, c.w, c.h, got)
		}
	}
	if Best("1080p", "4K") != "4K" || Best("4K", "SD") != "4K" || Best("", "DVD") != "DVD" {
		t.Error("best")
	}
	if CleanRegion("gb", "US") != "GB" || CleanRegion("GBR", "US") != "US" || CleanRegion("", "US") != "US" || CleanRegion("1A", "US") != "US" {
		t.Error("region")
	}
}
