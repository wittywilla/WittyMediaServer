package watchxref

import (
	"context"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// TMDB is the built-in Provider: TMDB's watch providers, which are JustWatch's data,
// through the same client and key the streaming listings use.
type TMDB struct {
	// Client gives the client to ask with, or nil when there's no key.
	Client func() *library.TMDB
}

// Name implements Provider.
func (TMDB) Name() string { return "tmdb" }

// Attribution implements Provider: the credit TMDB requires.
func (TMDB) Attribution() string { return library.ListingAttribution }

// Available implements Provider.
func (t TMDB) Available() (bool, string) {
	if t.Client == nil || t.Client() == nil {
		return false, ReasonNoAPIKey
	}
	return true, ""
}

// Offers implements Provider.
func (t TMDB) Offers(ctx context.Context, ref Ref, region string) (*Offers, error) {
	c := t.Client()
	if c == nil {
		return nil, ErrNoProvider
	}
	got, err := c.WatchOffers(ctx, ref.Kind, ref.ID, region)
	if err != nil {
		return nil, err
	}
	out := &Offers{Link: got.Link, Offers: make([]Offer, 0, len(got.Offers))}
	for _, o := range got.Offers {
		out.Offers = append(out.Offers, Offer{ProviderID: o.ProviderID, Key: o.Key, Name: o.Name, Type: o.Type, Logo: o.Logo, Priority: o.Priority})
	}
	return out, nil
}

// StoreCache keeps answers in the library's database, beside the listings.
type StoreCache struct{ Store *library.Store }

// Get implements Cache.
func (c StoreCache) Get(ctx context.Context, source, kind, id, region string) (string, int64, bool) {
	return c.Store.WatchCacheGet(ctx, source, kind, id, region)
}

// Put implements Cache.
func (c StoreCache) Put(ctx context.Context, source, kind, id, region, body string) error {
	return c.Store.WatchCachePut(ctx, source, kind, id, region, body)
}
