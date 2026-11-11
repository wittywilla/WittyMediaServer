// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package parental

import (
	"context"
	"sync"
	"time"
)

// Screen time is counted by the server's own clock, from the requests that
// playing makes (a stream's parts, a progress report), never from what an
// app says it played. Requests less than a gap apart are one stretch of
// watching and the time between them counts; a longer silence is a pause.
// Two devices playing at once on one profile count once.

// Gap is the longest silence that still counts as watching. Apps report
// progress every ten seconds and a stream asks for a part every few.
const Gap = 90 * time.Second

// flushAfter is how much time is held in memory before it is written down.
const flushAfter = 5 * time.Second

// maxTracked bounds the table. One entry is a profile that played lately.
const maxTracked = 512

// Store keeps each profile's day. Add is called with whole seconds.
type Store interface {
	Load(ctx context.Context, userID int64, day int) (Usage, error)
	Add(ctx context.Context, userID int64, day int, seconds int) error
}

type tracked struct {
	day     int
	loaded  bool
	usage   Usage         // as stored
	pending time.Duration // counted, not yet stored
	mark    time.Time     // the last request that counted
}

// Tracker counts screen time for the profiles that have limits.
type Tracker struct {
	Store Store
	Now   func() time.Time // the clock, so tests can turn it

	mu sync.Mutex
	m  map[int64]*tracked
}

func (t *Tracker) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

// entry returns a profile's day, loading it the first time and when the day
// has turned. Called with the lock held.
func (t *Tracker) entry(ctx context.Context, userID int64, now time.Time) *tracked {
	if t.m == nil {
		t.m = map[int64]*tracked{}
	}
	e := t.m[userID]
	day := DayNumber(now)
	if e == nil {
		if len(t.m) >= maxTracked {
			for id, x := range t.m { // whoever stopped a while ago has nothing held here
				if now.Sub(x.mark) > Gap && x.pending < time.Second {
					delete(t.m, id)
				}
			}
			if len(t.m) >= maxTracked {
				clear(t.m)
			}
		}
		e = &tracked{}
		t.m[userID] = e
	}
	if e.day != day || !e.loaded {
		t.flush(ctx, userID, e) // what was held belongs to the day it was played on
		e.day, e.pending = day, 0
		u, err := t.Store.Load(ctx, userID, day)
		e.usage, e.loaded = u, err == nil
	}
	return e
}

func (t *Tracker) flush(ctx context.Context, userID int64, e *tracked) {
	sec := int(e.pending / time.Second)
	if sec <= 0 || e.day == 0 {
		return
	}
	if err := t.Store.Add(context.WithoutCancel(ctx), userID, e.day, sec); err != nil {
		return // kept in memory, and tried again with the next
	}
	e.pending -= time.Duration(sec) * time.Second
	e.usage.UsedSec += sec
}

// Tick notes that the profile is playing something now.
func (t *Tracker) Tick(ctx context.Context, userID int64) {
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.entry(ctx, userID, now)
	if d := now.Sub(e.mark); !e.mark.IsZero() && d > 0 && d <= Gap {
		e.pending += d
	}
	e.mark = now
	if e.pending >= flushAfter {
		t.flush(ctx, userID, e)
	}
}

// Usage is what the profile has had today, with what isn't written down yet.
func (t *Tracker) Usage(ctx context.Context, userID int64) Usage {
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.entry(ctx, userID, now)
	u := e.usage
	u.UsedSec += int(e.pending / time.Second)
	return u
}

// Forget drops what is remembered of a profile, after its extra time or its
// stored day changed. What was counted and not yet stored is written first.
func (t *Tracker) Forget(ctx context.Context, userID int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e := t.m[userID]; e != nil {
		t.flush(ctx, userID, e)
		e.loaded = false
	}
}

// Flush writes down everything held, when the server stops.
func (t *Tracker) Flush(ctx context.Context) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, e := range t.m {
		t.flush(ctx, id, e)
	}
}
