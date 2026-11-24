// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package channels

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
)

// Blocks that play one programme (Block.Once), as a cloned station's slots do. The programme
// starts with the slot, and the time it leaves goes to commercials: some in breaks through it,
// at its natural break points where they are known (where its own adverts were, the end of its
// opening titles, the start of its credits) and evenly spaced where not, and the rest after it,
// so the next slot starts on the station's clock. A programme longer than its slot is cut.

// minBreakGap keeps breaks this far from each other and from either end of a programme.
const minBreakGap = 120

// eraYears: a commercial within this many years of a programme counts as from its era.
const eraYears = 6

func (d *Definition) adsOff() bool { return d.Clone != nil && d.Clone.Options.AdsOff }

func (d *Definition) density() string {
	if d.Clone == nil {
		return DensityNormal
	}
	return d.Clone.Options.Density
}

func (d *Definition) eraMatch() bool { return d.Clone == nil || !d.Clone.Options.NoEra }

// playOnce fills a Once block from t: one programme, then commercials to its end.
func (g *generator) playOnce(seg segment, t int64, p *pool) int64 {
	e := p.peek()
	if e == nil {
		g.pad(t, seg.end)
		return seg.end
	}
	room := seg.end - t
	dur := e.Dur
	if !seg.soft && dur > room {
		dur = max(room, 1)
	}
	p.advance(e)
	ads := !g.def.adsOff() && len(g.filler) > 0
	slack := max(room-dur, 0)
	var points []int64
	if ads && slack >= 60 {
		halves := int((seg.end - seg.start + 1799) / 1800)
		n := breaksPerHalfHour(g.def.density()) * max(halves, 1)
		n = min(n, int(dur/(2*minBreakGap+60)))
		points = breakPoints(dur, n, e.Breaks)
	}
	if g.def.eraMatch() {
		g.era = e.Year
	}
	defer func() { g.era = 0 }()
	share := slack / int64(len(points)+1)
	standIn := ""
	if seg.block != nil && seg.block.StandIn {
		standIn = seg.block.Name
	}
	var from int64
	for _, at := range points {
		pr := g.place(t, t+at-from, KindShow, e)
		pr.Offset, pr.For = from, standIn
		t += at - from
		from = at
		t = g.fillBreak(t, share)
	}
	pr := g.place(t, t+dur-from, KindShow, e)
	pr.Offset, pr.For, pr.Trimmed = from, standIn, e.Dur-dur
	t += dur - from
	if t >= seg.end {
		return t
	}
	if ads {
		g.pad(t, seg.end)
	} else if seg.end-t >= minSlate {
		g.place(t, seg.end, KindSlate, nil)
	}
	return seg.end
}

// fillBreak plays commercials for up to budget seconds from t, and returns where they end.
func (g *generator) fillBreak(t, budget int64) int64 {
	end := t + budget
	for end-t >= minSlate && len(g.out) < maxPerDay {
		e := g.pickFiller(end - t)
		if e == nil {
			break
		}
		g.place(t, t+e.Dur, KindFiller, e).Role = RoleBreak
		t += e.Dur
	}
	return t
}

// breakPoints chooses n places for breaks in a programme of dur seconds: evenly spaced, each
// moved to a natural break nearby when there is one. They are in order, and keep minBreakGap
// from each other and from both ends.
func breakPoints(dur int64, n int, natural []int64) []int64 {
	if n <= 0 || dur < 2*minBreakGap {
		return nil
	}
	spacing := dur / int64(n+1)
	var out []int64
	last := int64(0)
	for k := 1; k <= n; k++ {
		at := spacing * int64(k)
		best, bestDist := at, spacing/3+1
		for _, b := range natural {
			if d := abs64(b - at); d < bestDist {
				best, bestDist = b, d
			}
		}
		if best-last < minBreakGap || dur-best < minBreakGap {
			continue
		}
		out = append(out, best)
		last = best
	}
	return slices.Compact(out)
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// inEra says whether a commercial is from the years of the programme it plays in. One with no
// year fits any.
func inEra(year, era int) bool {
	return era == 0 || year == 0 || abs64(int64(year-era)) <= eraYears
}

// naturalBreaks are the places in a video where a break fits: where its own adverts began, the
// end of its opening titles and recap, and the start of its credits. What someone set wins over
// what a scan or comparison found.
func (s *Store) naturalBreaks(ctx context.Context, id int64) []int64 {
	var scanned, manual, detected string
	if s.DB.QueryRowContext(ctx, `SELECT scanned, manual, detected FROM media_markers WHERE item_id = ?`, id).Scan(&scanned, &manual, &detected) != nil {
		return nil
	}
	var out []int64
	for _, raw := range []string{manual, scanned, detected} {
		if raw == "" {
			continue
		}
		var f mediaextras.SkipFile
		if json.Unmarshal([]byte(raw), &f) != nil {
			continue
		}
		m, err := f.Markers(0)
		if err != nil {
			continue
		}
		for _, r := range m.Commercials {
			out = append(out, int64(r.StartSec))
		}
		for _, r := range []*mediaextras.Range{m.Intro, m.Recap} {
			if r != nil {
				out = append(out, int64(r.EndSec))
			}
		}
		if m.Credits != nil {
			out = append(out, int64(m.Credits.StartSec))
		}
		if len(out) > 0 {
			break
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
