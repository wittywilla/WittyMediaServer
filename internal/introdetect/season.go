// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package introdetect

import (
	"context"
	"sort"
)

// Limits on what is looked at and what is believed.
const (
	// HeadSec is how much of the front of an episode is listened to for the
	// intro, and TailSec how much of the back for the credits.
	HeadSec = 600
	TailSec = 420
	// MinIntroSec and MaxIntroSec bound a believable intro; a shared stretch
	// outside them is something else (a five-second sting, or two copies of
	// the same episode).
	MinIntroSec = 15
	MaxIntroSec = 180
	// MinCreditsSec and MaxCreditsSec, the same for credits.
	MinCreditsSec = 15
	MaxCreditsSec = 400
	// MaxEpisodes is the most episodes compared in one go.
	MaxEpisodes = 60
)

// Range is a stretch of one episode, in seconds from its start.
type Range struct{ Start, End float64 }

// Found is what was worked out for one episode. A nil range was not found.
type Found struct {
	Intro, Credits *Range
}

// Episode is one episode to compare: Head is the fingerprint of its first
// HeadSec seconds, Tail of its last TailSec, and TailStart where Tail begins
// in the episode. Either may be nil, and then that end isn't looked at.
type Episode struct {
	ID        int64
	Head      Fingerprint
	Tail      Fingerprint
	TailStart float64
}

// Season compares each episode with the next (and with the one after that, if
// the next shares nothing: a clip show, a special), and gives each the range
// it shares with a neighbour. When an episode shares a range with both its
// neighbours, the longer is kept. Episodes are compared in the order given.
// between is called between comparisons, so the caller can give way to other
// work or stop: an error from it ends the run with what was found so far.
func Season(ctx context.Context, eps []Episode, between func(context.Context) error) (map[int64]Found, error) {
	if len(eps) > MaxEpisodes {
		eps = eps[:MaxEpisodes]
	}
	out := make(map[int64]Found, len(eps))
	keep := func(id int64, intro bool, r Range) {
		f := out[id]
		slot := &f.Credits
		if intro {
			slot = &f.Intro
		}
		if *slot == nil || r.End-r.Start > (*slot).End-(*slot).Start {
			*slot = &r
		}
		out[id] = f
	}
	compare := func(a, b Episode) (bool, error) {
		if err := between(ctx); err != nil {
			return false, err
		}
		hit := false
		if m, ok := Longest(a.Head, b.Head, MinIntroSec*FramesPerSec); ok && m.Len <= MaxIntroSec*FramesPerSec {
			sa, ea, sb, eb := m.Seconds()
			keep(a.ID, true, Range{sa, ea})
			keep(b.ID, true, Range{sb, eb})
			hit = true
		}
		if m, ok := Longest(a.Tail, b.Tail, MinCreditsSec*FramesPerSec); ok && m.Len <= MaxCreditsSec*FramesPerSec {
			sa, ea, sb, eb := m.Seconds()
			keep(a.ID, false, Range{a.TailStart + sa, a.TailStart + ea})
			keep(b.ID, false, Range{b.TailStart + sb, b.TailStart + eb})
		}
		return hit, nil
	}
	for i := 0; i+1 < len(eps); i++ {
		hit, err := compare(eps[i], eps[i+1])
		if err != nil {
			return out, err
		}
		if !hit && i+2 < len(eps) {
			if _, err := compare(eps[i], eps[i+2]); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

// SortedIDs lists the episodes something was found for, for stable output.
func SortedIDs(m map[int64]Found) []int64 {
	ids := make([]int64, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
