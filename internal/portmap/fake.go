// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package portmap

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"time"
)

// FakeRouter is a router that exists only in memory, for the tests of
// packages that hold a Holder: nothing is sent anywhere.
type FakeRouter struct {
	External netip.Addr // the outside address it claims
	Refuse   bool       // refuse every mapping

	mu   sync.Mutex
	open map[Want]bool
}

func (f *FakeRouter) Protocol() string { return "fake" }

func (f *FakeRouter) ExternalIP(context.Context) (netip.Addr, error) { return f.External, nil }

func (f *FakeRouter) Map(ctx context.Context, e, i int) (int, time.Duration, error) {
	return f.MapProto(ctx, TCP, e, i)
}

func (f *FakeRouter) Unmap(ctx context.Context, e, i int) error {
	return f.UnmapProto(ctx, TCP, e, i)
}

func (f *FakeRouter) MapProto(_ context.Context, proto string, external, _ int) (int, time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Refuse {
		return 0, 0, errors.New("refused")
	}
	if f.open == nil {
		f.open = map[Want]bool{}
	}
	f.open[Want{proto, external}] = true
	return external, time.Hour, nil
}

func (f *FakeRouter) UnmapProto(_ context.Context, proto string, external, _ int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.open, Want{proto, external})
	return nil
}

// Open lists what is mapped on it now.
func (f *FakeRouter) Open() []Want {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Want
	for w := range f.open {
		out = append(out, w)
	}
	return out
}

// NewFakeHolder is a Holder whose router is r.
func NewFakeHolder(r *FakeRouter) *Holder {
	return &Holder{find: func(context.Context) (router, error) { return r, nil },
		addr6: func() []netip.Addr { return nil }}
}
