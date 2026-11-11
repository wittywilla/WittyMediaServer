// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package syncplay

import "math"

// The arithmetic every client does, written once here so there is something to test and
// to point at. web/src/player/sync.ts is the same rules in TypeScript; docs/SYNCPLAY.md
// spells them out for apps in other languages.

// ClockOffset is NTP's sum for one ping. t0: the client's clock when it sent; t1: the
// server's when that arrived; t2: the server's when it answered; t3: the client's when the
// answer arrived. All in milliseconds. offset is how far the server's clock is ahead of
// the client's, and rtt the time spent on the wire there and back.
func ClockOffset(t0, t1, t2, t3 float64) (offset, rtt float64) {
	return ((t1 - t0) + (t2 - t3)) / 2, (t3 - t0) - (t2 - t1)
}

// Sample is one ping's four times.
type Sample struct{ T0, T1, T2, T3 float64 }

// BestOffset picks, from several pings, the offset of the one with the shortest round
// trip: it was held up least on the way, so its two halves are most nearly equal, which is
// what the sum assumes. A ping that came back before it left (a clock that jumped) is
// ignored. ok is false with nothing usable.
func BestOffset(samples []Sample) (offset, rtt float64, ok bool) {
	rtt = math.Inf(1)
	for _, s := range samples {
		o, r := ClockOffset(s.T0, s.T1, s.T2, s.T3)
		if r < 0 || math.IsNaN(r) || math.IsNaN(o) {
			continue
		}
		if r < rtt {
			offset, rtt, ok = o, r, true
		}
	}
	if !ok {
		return 0, 0, false
	}
	return offset, rtt, true
}

// Drift thresholds, in seconds, and the rate change used to close a small gap. They are
// the ones the web player already keeps a live channel in step with, and every client is
// told them by GET /api/v1/syncplay, so none has to copy the numbers.
const (
	DriftStill   = 0.12 // closer than this: leave it alone
	DriftSettled = 0.04 // once nudging, carry on until this close
	DriftSeek    = 3.0  // this far off, or more: jump
	DriftRate    = 0.03 // play this much faster or slower to catch up or fall back
)

// Step is what a player should do about being out of step: play at Rate, or jump by
// SeekBy seconds (and then play at 1).
type Step struct {
	Rate   float64
	SeekBy float64
}

// Correct is the drift rule. behind: seconds this player is behind where the room is
// (negative: ahead). nudging: whether it is already playing faster or slower, which
// carries on until it's settled rather than stopping the moment it is merely close.
func Correct(behind float64, nudging bool) Step {
	off := math.Abs(behind)
	if off >= DriftSeek {
		return Step{Rate: 1, SeekBy: behind}
	}
	limit := DriftStill
	if nudging {
		limit = DriftSettled
	}
	if off < limit {
		return Step{Rate: 1}
	}
	if behind > 0 {
		return Step{Rate: 1 + DriftRate}
	}
	return Step{Rate: 1 - DriftRate}
}
