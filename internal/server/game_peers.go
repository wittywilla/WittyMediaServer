// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/http"
	"sync"
	"time"
)

// A connection to a game session holds sockets, goroutines and memory, and a WebRTC offer
// holds them for half a minute whether or not anything comes of it. A session is one
// person's game, so it only ever needs a few: a page that reloaded, a new network, a second
// tab. These limits keep anyone allowed to see a game from running the machine out.
const (
	// peerOfferBurst offers at once, then one more each peerOfferEvery. The player tries
	// WebRTC once before falling back to a socket, so this is many reloads in a row.
	peerOfferBurst = 8
	peerOfferEvery = time.Minute
	// peerSocketsAtOnce is how many WebSocket viewers a session can have.
	peerSocketsAtOnce = 4
	// peerSessionsKept is more than can run at once. The rest is forgotten, oldest first.
	peerSessionsKept = 64
)

type sessionPeers struct {
	offers  int // left to use
	refill  time.Time
	sockets int
	last    time.Time
}

type peerLimits struct {
	mu sync.Mutex
	m  map[string]*sessionPeers
}

func (p *peerLimits) entry(id string, now time.Time) *sessionPeers {
	if p.m == nil {
		p.m = map[string]*sessionPeers{}
	}
	e := p.m[id]
	if e == nil {
		if len(p.m) >= peerSessionsKept {
			p.forgetOldest()
		}
		e = &sessionPeers{offers: peerOfferBurst, refill: now}
		p.m[id] = e
	}
	e.last = now
	return e
}

func (p *peerLimits) forgetOldest() {
	var old string
	var at time.Time
	for id, e := range p.m {
		if e.sockets == 0 && (old == "" || e.last.Before(at)) {
			old, at = id, e.last
		}
	}
	delete(p.m, old)
}

// offer says whether a session may make another WebRTC offer now.
func (p *peerLimits) offer(id string, now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.entry(id, now)
	if n := int(now.Sub(e.refill) / peerOfferEvery); n > 0 {
		e.offers = min(e.offers+n, peerOfferBurst)
		e.refill = e.refill.Add(time.Duration(n) * peerOfferEvery)
	}
	if e.offers == 0 {
		return false
	}
	if e.offers == peerOfferBurst {
		e.refill = now
	}
	e.offers--
	return true
}

// socket takes one of a session's places for a WebSocket viewer, until release.
func (p *peerLimits) socket(id string, now time.Time) (release func(), ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.entry(id, now)
	if e.sockets >= peerSocketsAtOnce {
		return nil, false
	}
	e.sockets++
	var once sync.Once
	return func() {
		once.Do(func() {
			p.mu.Lock()
			e.sockets--
			p.mu.Unlock()
		})
	}, true
}

func tooManyPeers(w http.ResponseWriter, retry string) {
	w.Header().Set("Retry-After", retry)
	writeErr(w, 429, "Too many connections to this game. Try again in a minute.")
}
