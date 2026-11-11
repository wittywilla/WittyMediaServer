// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package introdetect

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"testing"
)

// music makes sec seconds of made-up "music": a few notes at a time that
// change twice a second, from a seed. The same seed is the same tune.
func music(seed int64, sec float64) []float64 {
	r := rand.New(rand.NewSource(seed))
	n := int(sec * SampleRate)
	out := make([]float64, n)
	var freqs [4]float64
	for i := 0; i < n; i++ {
		if i%(SampleRate/2) == 0 {
			for k := range freqs {
				freqs[k] = 300 * math.Pow(2, r.Float64()*2.7) // 300 Hz to about 2 kHz
			}
		}
		t := float64(i) / SampleRate
		for k, f := range freqs {
			out[i] += math.Sin(2*math.Pi*f*t) / float64(k+1)
		}
	}
	return out
}

// speech makes sec seconds of noise shaped like talk: different every time.
func speech(seed int64, sec float64) []float64 {
	r := rand.New(rand.NewSource(seed))
	n := int(sec * SampleRate)
	out := make([]float64, n)
	low := 0.0
	for i := range out {
		low = 0.9*low + 0.1*(r.Float64()*2-1)
		env := 0.5 + 0.5*math.Sin(2*math.Pi*3*float64(i)/SampleRate)
		out[i] = low * env * 4
	}
	return out
}

// pcm mixes the parts end to end into 16-bit sound at the given loudness,
// with a little noise over everything, as a different encode would have.
func pcm(seed int64, gain float64, parts ...[]float64) []byte {
	r := rand.New(rand.NewSource(seed))
	var out []byte
	for _, p := range parts {
		for _, s := range p {
			v := int(math.Round((s*gain + (r.Float64()*2-1)*0.02) * 6000))
			v = max(-32768, min(32767, v))
			out = append(out, byte(v), byte(v>>8))
		}
	}
	return out
}

func silence(sec float64) []float64 { return make([]float64, int(sec*SampleRate)) }

func near(got, want, tol float64) bool { return math.Abs(got-want) <= tol }

func TestLongestFindsTheSharedIntro(t *testing.T) {
	theme := music(1, 30)
	// Episode A: 20 s of talk, the theme, more talk. Episode B: 65 s of talk
	// first, the theme quieter, then other talk.
	a := FromPCM(pcm(10, 1.0, speech(100, 20), theme, speech(101, 40)))
	b := FromPCM(pcm(11, 0.5, speech(200, 65), theme, speech(201, 25)))
	m, ok := Longest(a, b, MinIntroSec*FramesPerSec)
	if !ok {
		t.Fatal("the shared theme wasn't found")
	}
	sa, ea, sb, eb := m.Seconds()
	if !near(sa, 20, 1.5) || !near(ea, 50, 1.5) || !near(sb, 65, 1.5) || !near(eb, 95, 1.5) {
		t.Errorf("found %.1f-%.1f and %.1f-%.1f, want 20-50 and 65-95", sa, ea, sb, eb)
	}
	if m.Score < 0.85 {
		t.Errorf("score %.2f for the same recording", m.Score)
	}
}

func TestLongestRefusesWhatIsNotShared(t *testing.T) {
	cases := map[string][2][]byte{
		"two different episodes": {pcm(1, 1, speech(1, 60), music(5, 30)), pcm(2, 1, speech(2, 60), music(6, 30))},
		"silence in both":        {pcm(1, 0, silence(60)), pcm(2, 0, silence(60))},
		"a five-second sting":    {pcm(1, 1, speech(3, 30), music(7, 5), speech(4, 30)), pcm(2, 1, speech(5, 40), music(7, 5), speech(6, 20))},
		"silence then talk":      {pcm(1, 1, silence(30), speech(7, 30)), pcm(2, 1, silence(30), speech(8, 30))},
	}
	for name, c := range cases {
		if m, ok := Longest(FromPCM(c[0]), FromPCM(c[1]), MinIntroSec*FramesPerSec); ok {
			t.Errorf("%s: matched %+v", name, m)
		}
	}
	if _, ok := Longest(nil, FromPCM(pcm(1, 1, music(1, 30))), 10); ok {
		t.Error("matched nothing against something")
	}
	if FromPCM(make([]byte, 100)) != nil {
		t.Error("a fingerprint from less than one frame of sound")
	}
}

func TestLongestSurvivesTalkOverTheTitles(t *testing.T) {
	theme := music(3, 40)
	spoiled := append([]float64(nil), theme...)
	// One second of an episode's own sound over the middle of the titles.
	for i, s := range speech(9, 1) {
		spoiled[15*SampleRate+i] += s * 6
	}
	a := FromPCM(pcm(1, 1, speech(1, 10), theme, speech(2, 10)))
	b := FromPCM(pcm(2, 1, speech(3, 10), spoiled, speech(4, 10)))
	m, ok := Longest(a, b, MinIntroSec*FramesPerSec)
	if !ok || m.Len < 37*FramesPerSec {
		t.Errorf("a second of talk split the intro: %+v %v", m, ok)
	}
}

func TestSeason(t *testing.T) {
	theme, credits := music(1, 25), music(2, 20)
	head := func(seed int64, before float64) Fingerprint {
		return FromPCM(pcm(seed, 1, speech(seed*10, before), theme, speech(seed*10+1, 30)))
	}
	tail := func(seed int64) Fingerprint {
		return FromPCM(pcm(seed, 1, speech(seed*10+2, 20), credits))
	}
	eps := []Episode{
		{ID: 1, Head: head(1, 5), Tail: tail(1), TailStart: 1300},
		{ID: 2, Head: head(2, 40), Tail: tail(2), TailStart: 1250},
		// A special with titles and credits of its own.
		{ID: 3, Head: FromPCM(pcm(3, 1, speech(30, 30), music(50, 25))), Tail: FromPCM(pcm(3, 1, speech(31, 40)))},
		{ID: 4, Head: head(4, 12), Tail: tail(4), TailStart: 1280},
	}
	calls := 0
	got, err := Season(context.Background(), eps, func(context.Context) error { calls++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Error("the run never gave way between comparisons")
	}
	for id, before := range map[int64]float64{1: 5, 2: 40, 4: 12} {
		f := got[id]
		if f.Intro == nil || !near(f.Intro.Start, before, 1.5) || !near(f.Intro.End, before+25, 1.5) {
			t.Errorf("episode %d intro: %+v, want %.0f-%.0f", id, f.Intro, before, before+25)
		}
	}
	if f := got[3]; f.Intro != nil || f.Credits != nil {
		t.Errorf("the special shares nothing, but got %+v %+v", f.Intro, f.Credits)
	}
	if f := got[1]; f.Credits == nil || !near(f.Credits.Start, 1320, 1.5) || !near(f.Credits.End, 1340, 1.5) {
		t.Errorf("episode 1 credits: %+v, want 1320-1340", f.Credits)
	}
	// Stopping part-way keeps what was found and says why it stopped.
	stop := errors.New("stopped")
	n := 0
	part, err := Season(context.Background(), eps, func(context.Context) error {
		if n++; n > 1 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || part[1].Intro == nil {
		t.Errorf("stopped run: %v %+v", err, part)
	}
	// Two copies of one episode share everything: that isn't an intro.
	same := []Episode{{ID: 1, Head: FromPCM(pcm(1, 1, music(8, 300)))}, {ID: 2, Head: FromPCM(pcm(2, 1, music(8, 300)))}}
	if dup, _ := Season(context.Background(), same, func(context.Context) error { return nil }); dup[1].Intro != nil {
		t.Errorf("five shared minutes were taken for an intro: %+v", dup[1].Intro)
	}
}

func BenchmarkLongestTenMinutes(b *testing.B) {
	r := rand.New(rand.NewSource(1))
	mk := func() Fingerprint {
		f := make(Fingerprint, HeadSec*FramesPerSec)
		for i := range f {
			f[i] = uint32(r.Intn(1 << 16))
		}
		return f
	}
	x, y := mk(), mk()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Longest(x, y, MinIntroSec*FramesPerSec)
	}
}
