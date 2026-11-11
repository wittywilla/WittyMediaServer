// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"math"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/channels"
	"github.com/wittywilla/wittymediaserver/internal/hls"
)

// A long break of commercials, each a little shorter than its slot (which is
// whole seconds), keeps to the schedule: the stream never gets so far ahead
// that a card goes up in the middle of the break, and every commercial plays
// from its start.
func TestFeedKeepsABreakToTheSchedule(t *testing.T) {
	t0 := time.Unix(1_800_000_000, 0)
	// Sixty 15-second slots of 14.4-second commercials.
	var progs []channels.Programme
	for i := range int64(60) {
		progs = append(progs, channels.Programme{Start: t0.Unix() + 15*i, End: t0.Unix() + 15*(i+1), Kind: channels.KindFiller, ItemID: i + 1})
	}
	at := func(when time.Time) *channels.Programme {
		for i := range progs {
			if p := &progs[i]; p.Start <= when.Unix() && when.Unix() < p.End {
				return p
			}
		}
		return nil
	}
	// The first run starts with its head start already played out.
	now := t0.Add(-hls.LiveHeadStart)
	var lastEnd int64
	var waited time.Duration
	for range 59 {
		from, wait, card := feedPlan(now, lastEnd)
		if card > 0 {
			t.Fatalf("a %.0f s card at %v", card, now.Sub(t0))
		}
		if wait > 0 {
			waited += wait
			now = now.Add(wait)
		}
		if lastEnd == 0 {
			from = t0
		}
		p := at(from)
		off, remain := runSpan(p, from)
		if off != 0 {
			t.Fatalf("commercial %d joined %.2f s in", p.ItemID, off)
		}
		if math.Abs(remain-float64(p.End-p.Start)) > 1e-9 {
			t.Fatalf("commercial %d runs %.2f s", p.ItemID, remain)
		}
		if ahead := time.Unix(p.Start, 0).Sub(now); ahead > hls.LiveHeadStart+feedHold+time.Second {
			t.Fatalf("%v ahead of the guide by commercial %d", ahead, p.ItemID)
		}
		lastEnd = p.End
		// It ends 0.6 s before its slot does, and FFmpeg took 0.3 s to start.
		now = now.Add(time.Duration((remain - 0.6 + 0.3) * float64(time.Second)))
	}
	if waited == 0 {
		t.Error("never waited, so the stream drifted ahead")
	}
}

// Joining a programme part way: it plays from there to its end exactly, and a
// file that ended far too soon gets a card for the rest.
func TestFeedPlanAndRunSpan(t *testing.T) {
	now := time.Unix(1000, 500_000_000)
	p := &channels.Programme{Start: 990, End: 1031}
	off, remain := runSpan(p, now)
	if off != 10.5 || remain != 30.5 {
		t.Errorf("off %v remain %v", off, remain)
	}
	if off, _ := runSpan(p, time.Unix(990, 900_000_000)); off != 0 {
		t.Errorf("under a second in should start from the start, got %v", off)
	}
	if at, wait, card := feedPlan(now, 999); !at.Equal(now) || wait != 0 || card != 0 {
		t.Errorf("behind: %v %v %v", at, wait, card)
	}
	if at, wait, card := feedPlan(now, 1010); at.Unix() != 1010 || wait != 0 || card != 0 {
		t.Errorf("a head start: %v %v %v", at, wait, card)
	}
	if _, wait, _ := feedPlan(now, 1016); wait != 0 {
		t.Errorf("a little past the head start waits %v", wait)
	}
	// A file that ended well early: the stream gets back a few seconds a run, so
	// the player's buffer never runs dry.
	if _, wait, _ := feedPlan(now, 1025); wait != feedHold {
		t.Errorf("far ahead waits %v", wait)
	}
	if _, _, card := feedPlan(now, 1060); card != 59.5 {
		t.Errorf("a file that ran out early: card %v", card)
	}
}
