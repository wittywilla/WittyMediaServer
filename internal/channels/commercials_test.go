package channels

import (
	"fmt"
	"testing"
	"time"
)

// adWorld is a channel's titles beside a library of commercials like a real
// one: many of them, from a few seconds to a minute, mostly 15 and 30. Library
// 60 holds them; items 801 and 802 are short clips from it that a channel may
// also use as bumpers.
func adWorld() PoolLoader {
	shows := append(eps(1, 8, 22*60+13), eps(2, 8, 11*60+7)...)
	var ads []Entry
	lens := []int64{3, 4, 6, 11, 15, 15, 16, 21, 30, 30, 31, 31, 32, 38, 60}
	for i := range 45 {
		ads = append(ads, Entry{ID: 700 + int64(i), Dur: lens[i%len(lens)], Title: fmt.Sprintf("Ad %d", i)})
	}
	ads = append(ads, Entry{ID: 801, Dur: 3, Title: "Ident"}, Entry{ID: 802, Dur: 5, Title: "Back soon"})
	return func(r Rules, _ int64) ([]Entry, error) {
		switch {
		case len(r.Libraries) == 1 && r.Libraries[0] == 60:
			return ads, nil
		case len(r.Items) > 0:
			var out []Entry
			for _, a := range ads {
				for _, id := range r.Items {
					if a.ID == id {
						out = append(out, a)
					}
				}
			}
			return out, nil
		}
		return shows, nil
	}
}

func adDay(t *testing.T, def *Definition) []Programme {
	t.Helper()
	if err := def.Validate(); err != nil {
		t.Fatal(err)
	}
	progs, _, err := GenerateDay(def, 3, 20000, time.UTC, State{}, adWorld(), map[string][]Entry{})
	if err != nil {
		t.Fatal(err)
	}
	noOverlap(t, progs)
	return progs
}

// A channel with commercials after each programme and in breaks inside it,
// with and without cutting them short: they play where they should, every
// programme starts on the grid, nothing is left unaccounted for, and the guide
// has each programme once, from its first moment to the next one's.
func TestScheduleWithCommercials(t *testing.T) {
	for _, trim := range []bool{false, true} {
		t.Run(fmt.Sprintf("trim=%v", trim), func(t *testing.T) {
			def := Definition{Mode: "simple", Order: Order{Mode: "rotate"}, AlignMin: 30,
				Filler:  Filler{Commercials: []CommercialPick{{Library: 60}}, Between: 3, BreakEveryMin: 10, BreakAds: 2},
				Bumpers: BumperPlan{Trim: Trim{On: trim}}}
			progs := adDay(t, &def)
			day := DayStart(20000, time.UTC).Unix()
			// A moment too short for a card is closed by starting a moment early.
			onGrid := func(at int64) bool { d := (at - day) % 1800; return d <= 2 || d >= 1800-2 }
			played := map[int64]int64{} // seconds of each programme
			breaks, after := 0, 0
			for i, p := range progs {
				if i > 0 && p.Start != progs[i-1].End {
					t.Fatalf("a hole: %+v then %+v", progs[i-1], p)
				}
				switch {
				case p.Kind == KindShow:
					if p.Offset == 0 && !onGrid(p.Start) {
						t.Fatalf("a programme off the grid: %+v", p)
					}
					if p.Offset == 0 {
						played[p.ItemID] = 0 // round again
					}
					if p.Offset != played[p.ItemID] {
						t.Fatalf("a stretch from %d s, after %d s had played: %+v", p.Offset, played[p.ItemID], p)
					}
					played[p.ItemID] += p.End - p.Start
				case p.Role == RoleAd && i+1 < len(progs) && progs[i+1].Kind == KindShow && progs[i+1].Offset > 0:
					breaks++
				case p.Role == RoleAd:
					after++
				}
			}
			if breaks == 0 || after == 0 {
				t.Errorf("%d commercials in breaks and %d after programmes", breaks, after)
			}
			for _, p := range GuideCells(progs) {
				if p.Kind == KindShow && !onGrid(p.Start) {
					t.Errorf("the guide has a programme off the grid: %+v", p)
				}
			}
		})
	}
}

// One commercial doesn't come round again until many others have played, and
// when only it fits, it plays again rather than nothing.
func TestCommercialsDontRepeatSoon(t *testing.T) {
	g := &generator{fillerR: rng(9)}
	for i := range int64(45) {
		g.filler = append(g.filler, Entry{ID: 700 + i, Dur: 10 + i})
	}
	last := map[int64]int{}
	for n := range 500 {
		e := g.pickFiller(1 << 40)
		if at, ok := last[e.ID]; ok && n-at <= min(maxRecent, len(g.filler)/2) {
			t.Fatalf("%d again after only %d others", e.ID, n-at-1)
		}
		last[e.ID] = n
	}
	for range 3 {
		if e := g.pickFiller(10); e == nil || e.ID != 700 {
			t.Fatalf("the only one that fits: %+v", e)
		}
	}
	// A handful: each waits for half of the others.
	g = &generator{fillerR: rng(9), filler: g.filler[:4]}
	prev := int64(0)
	for range 50 {
		e := g.pickFiller(1 << 40)
		if e.ID == prev {
			t.Fatalf("%d twice running", e.ID)
		}
		prev = e.ID
	}
}

// A video a bumper rule plays is a bumper on that channel, not a commercial as
// well, though it is in the commercials library; unless bumpers are all there is.
func TestBumpersAreNotCommercialsToo(t *testing.T) {
	def := Definition{Mode: "simple", Order: Order{Mode: "rotate"}, AlignMin: 30,
		Filler: Filler{Commercials: []CommercialPick{{Library: 60}}, Between: 2, BreakEveryMin: 10, BreakAds: 2},
		Bumpers: BumperPlan{Rules: []BumperRule{
			{ID: "in", When: WhenBefore, Source: Rules{Items: []int64{801}}, Mandatory: true},
			{ID: "out", When: WhenAfter, Source: Rules{Items: []int64{802}}},
			{ID: "brk", When: WhenEvery, EveryMin: 10, Source: Rules{Items: []int64{802}}},
		}}}
	progs := adDay(t, &def)
	pre, post, brk := 0, 0, 0
	for i, p := range progs {
		switch p.Role {
		case RolePre:
			pre++
			if p.ItemID != 801 || progs[i+1].Kind != KindShow || progs[i+1].Offset != 0 {
				t.Fatalf("the ident should lead into a programme: %+v / %+v", p, progs[i+1])
			}
		case RolePost:
			post++
		case RoleBreak:
			brk++
		case RoleAd, RolePad:
			if p.ItemID == 801 || p.ItemID == 802 {
				t.Fatalf("a bumper played as a commercial: %+v", p)
			}
		}
	}
	if pre == 0 || post == 0 || brk == 0 {
		t.Errorf("%d before, %d after, %d in breaks", pre, post, brk)
	}

	// The same two clips as the only commercials: they play as both.
	def.Filler.Commercials = []CommercialPick{{Library: 60, Only: true, Items: []int64{801, 802}}}
	ads := 0
	for _, p := range adDay(t, &def) {
		if p.Role == RoleAd {
			ads++
		}
	}
	if ads == 0 {
		t.Error("no commercials when the bumpers are all there is")
	}
}
