// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"
)

// Clock is the time, as the connection keeper sees it. Tests bring their own.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// backoff is how long to wait before trying again: a second, doubling to a
// minute, each moved a little so that many apps cut off at once (a server
// restarting) don't all come back in the same instant.
type backoff struct {
	n      int
	jitter func() float64 // 0..1; nil: random
}

const (
	backoffFirst = time.Second
	backoffMax   = time.Minute
)

func (b *backoff) next() time.Duration {
	d := backoffFirst
	for i := 0; i < b.n && d < backoffMax; i++ {
		d *= 2
	}
	d = min(d, backoffMax)
	b.n++
	j := rand.Float64
	if b.jitter != nil {
		j = b.jitter
	}
	return d - d/5 + time.Duration(j()*float64(d)*2/5) // within a fifth either way
}

func (b *backoff) reset() { b.n = 0 }

// Paths a connection can be on.
const (
	PathConnecting = "connecting"
	PathOffline    = "offline"
	PathLocal      = "local"   // direct, on the same network
	PathDirect     = "direct"  // direct, across the internet
	PathRelayed    = "relayed" // through a relay
)

// State is where the connection to the server stands, for the app to show.
type State struct {
	Path string `json:"path"`
	// Addr is the server's address on a direct path, Relay the relay's ID on a
	// relayed one. Community says that relay passes on whole connections.
	Addr      string `json:"addr,omitempty"`
	Transport string `json:"transport,omitempty"` // quic | tcp
	Relay     string `json:"relay,omitempty"`
	Community bool   `json:"community,omitempty"`
	Error     string `json:"error,omitempty"`    // why the last try failed
	Attempts  int    `json:"attempts,omitempty"` // tries since it last worked
	NextTry   int64  `json:"nextTry,omitempty"`  // when the next one is due
	Since     int64  `json:"since,omitempty"`    // when this path began
}

// Connected says requests can be sent.
func (s State) Connected() bool {
	return s.Path == PathLocal || s.Path == PathDirect || s.Path == PathRelayed
}

// link is what a keeper keeps up: the connection to one server.
type link interface {
	// current says how the server is reached right now ("" if it isn't).
	current() State
	// connect tries every way of reaching the server once.
	connect(ctx context.Context) error
	// tidy is called now and then while connected, to drop what a better
	// path has replaced.
	tidy()
	// check finds out whether a standing connection still works, and drops it
	// if not. It is asked when the network changed under it.
	check(ctx context.Context)
}

// keeper holds one connection up: it connects, notices when the path changes
// or is lost, and tries again after a wait that grows. It is one goroutine
// with no timers of its own beyond its clock.
type keeper struct {
	link    link
	clock   Clock
	onState func(State)
	bo      backoff

	changed chan struct{} // the link's connections changed
	kicked  chan struct{} // the network changed: try now, with no wait

	mu    sync.Mutex
	state State
	waits []chan struct{} // closed when the state next changes
}

func newKeeper(l link, clock Clock, onState func(State)) *keeper {
	if clock == nil {
		clock = realClock{}
	}
	return &keeper{link: l, clock: clock, onState: onState, changed: make(chan struct{}, 1), kicked: make(chan struct{}, 1),
		state: State{Path: PathConnecting}}
}

func poke(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// noteChanged tells the keeper the link's connections changed.
func (k *keeper) noteChanged() { poke(k.changed) }

// kick tells the keeper the network changed (Wi-Fi to mobile data, waking
// from sleep): whatever it was waiting for, it looks again now.
func (k *keeper) kick() { poke(k.kicked) }

func (k *keeper) get() State {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.state
}

func (k *keeper) set(s State) {
	k.mu.Lock()
	same := k.state == s
	if s.Path == k.state.Path && s.Addr == k.state.Addr && s.Relay == k.state.Relay && k.state.Since != 0 {
		s.Since = k.state.Since
		same = k.state == s
	}
	k.state = s
	waits := k.waits
	if !same {
		k.waits = nil
	}
	k.mu.Unlock()
	if same {
		return
	}
	for _, w := range waits {
		close(w)
	}
	if k.onState != nil {
		k.onState(s)
	}
}

// wait blocks until ok(state) or ctx ends, and returns the state then.
func (k *keeper) wait(ctx context.Context, ok func(State) bool) (State, error) {
	for {
		k.mu.Lock()
		s := k.state
		if ok(s) {
			k.mu.Unlock()
			return s, nil
		}
		w := make(chan struct{})
		k.waits = append(k.waits, w)
		k.mu.Unlock()
		select {
		case <-w:
		case <-ctx.Done():
			return k.get(), ctx.Err()
		}
	}
}

// tidyEvery is how often a standing connection is looked over.
const tidyEvery = 20 * time.Second

// run keeps the link up until ctx ends.
func (k *keeper) run(ctx context.Context) {
	attempts := 0
	for ctx.Err() == nil {
		if cur := k.link.current(); cur.Connected() {
			attempts = 0
			k.bo.reset()
			cur.Since = k.clock.Now().Unix()
			k.set(cur)
			select {
			case <-ctx.Done():
				return
			case <-k.changed:
			case <-k.kicked:
				k.link.check(ctx)
			case <-k.clock.After(tidyEvery):
				k.link.tidy()
			}
			continue
		}
		prev := k.get()
		k.set(State{Path: PathConnecting, Error: prev.Error, Attempts: attempts})
		err := k.link.connect(ctx)
		if ctx.Err() != nil {
			return
		}
		if err == nil && k.link.current().Connected() {
			continue
		}
		attempts++
		d := k.bo.next()
		why := "the server didn't answer"
		if err != nil {
			why = err.Error()
		}
		k.set(State{Path: PathOffline, Error: why, Attempts: attempts, NextTry: k.clock.Now().Add(d).Unix()})
		// Drain a change that this very attempt caused, or it would cut the wait short.
		select {
		case <-k.changed:
		default:
		}
		select {
		case <-ctx.Done():
			return
		case <-k.clock.After(d):
		case <-k.changed:
		case <-k.kicked:
			k.bo.reset() // a new network deserves a fresh start
		}
	}
}
