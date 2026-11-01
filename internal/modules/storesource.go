package modules

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/wittywilla/wittymodule"
)

// A store's version of a module is a fork whose list of versions comes from the store rather
// than from a GitHub repository. Everything else about it is a fork's: its own key, a fork's
// limits, and nothing updates by itself. This is the small part that's different.

// storeRoutes says how to fetch an address a store listed, since a store's files aren't on
// GitHub and have that store's own rules. An address not here isn't a store's.
type storeRoutes struct {
	mu sync.RWMutex
	by map[string]*storeFetcher
}

func (r *storeRoutes) set(raw string, f *storeFetcher) {
	r.mu.Lock()
	if r.by == nil {
		r.by = map[string]*storeFetcher{}
	}
	r.by[raw] = f
	r.mu.Unlock()
}

func (r *storeRoutes) get(raw string) *storeFetcher {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.by[raw]
}

// fromStore says this module's versions come from a store's catalogue, not from asking GitHub.
func fromStore(e *entry) bool { return e.state.Source != nil && e.state.Source.Store != "" }

// CheckEnabled says whether the owner allows looking for updates.
func (m *Manager) CheckEnabled() bool { return m.cfg.CheckEnabled != nil && m.cfg.CheckEnabled() }

// routeSmall and routeOpen answer for an address a store listed; ok is false for any other.
func (m *Manager) routeSmall(ctx context.Context, raw string, limit int64) (b []byte, ok bool, err error) {
	f := m.routes.get(raw)
	if f == nil {
		return nil, false, nil
	}
	b, err = f.small(ctx, raw, limit)
	return b, true, err
}

func (m *Manager) routeOpen(ctx context.Context, raw string) (body io.ReadCloser, size int64, ok bool, err error) {
	f := m.routes.get(raw)
	if f == nil {
		return nil, 0, false, nil
	}
	body, size, err = f.open(ctx, raw)
	return body, size, true, err
}

// useStoreSource makes a store the source of a module. What was downloaded from the old source
// goes, since it was signed by someone else.
func (m *Manager) useStoreSource(id string, src *Source, detail string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil {
		return ErrUnknown
	}
	if why := noFork[id]; why != "" {
		return errors.New(why)
	}
	if e.busy {
		return errors.New("an update of this module is being installed")
	}
	return m.changeSource(id, e, src, detail)
}

// relist gives a module back the versions this project's catalogue lists, after it stopped
// coming from somewhere else. Call with m.mu held.
func (m *Manager) relist(id string, e *entry) {
	if e.state.Source != nil || m.cat == nil {
		return
	}
	mod := m.cat.Find(id)
	if mod == nil {
		return
	}
	e.feedLoaded = true
	for i := range mod.Versions {
		if k := listedRelease(mod, &mod.Versions[i], m.catFrom); k != nil {
			e.known[mod.Versions[i].Version] = k
		}
	}
	m.computeOffers(e)
}

// applyStoreListing gives every module using this store the versions it now lists. Each version
// is only an offer, and has its own signed list and manifest to pass when it's installed. A
// store can't reach any module that isn't using it, and can't restore a version it withdrew.
func (m *Manager) applyStoreListing(store string, c *wittymodule.Catalog, from string, f *storeFetcher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range m.order {
		e := m.mods[id]
		if e.state.Source == nil || e.state.Source.Store != store {
			continue
		}
		mod := c.Find(id)
		if mod == nil {
			continue
		}
		e.known = map[string]*knownRelease{}
		e.feedLoaded = true
		for i := range mod.Versions {
			k := listedRelease(mod, &mod.Versions[i], from)
			if k == nil {
				continue
			}
			k.notes = cleanText(k.notes)
			k.url = ""
			e.known[mod.Versions[i].Version] = k
			m.routes.set(k.rel.sums.URL, f)
			m.routes.set(k.rel.sig.URL, f)
			m.routes.set(k.rel.man.URL, f)
			m.routes.set(k.rel.archive.URL, f)
		}
		m.applyListedWithdrawals(id, e, mod, c.Serial)
		m.computeOffers(e)
	}
}
