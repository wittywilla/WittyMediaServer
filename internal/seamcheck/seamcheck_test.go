package seamcheck

import (
	"math"
	"testing"
)

// A made-up film: every frame looks different from its neighbours.
func film(n int) *Pictures {
	p := &Pictures{FPS: 25, Frames: make([][thumbSize]byte, n)}
	for i := range p.Frames {
		for k := range p.Frames[i] {
			p.Frames[i][k] = byte((i*7 + k*(i%13+1)) % 251)
		}
	}
	return p
}

func noise(sec float64) []int16 {
	s := make([]int16, int(sec*AudioRate))
	v := uint32(1)
	for i := range s {
		v = v*1664525 + 1013904223
		s[i] = int16(v >> 18)
	}
	return s
}

// The check has to see what it is for: frames left out, frames repeated, and
// sound lost at a boundary. If it can't, a clean report means nothing.
func TestMeasureSeesLostFramesAndSound(t *testing.T) {
	ref := film(500)
	out := &Pictures{FPS: 25}
	out.Frames = append(out.Frames, ref.Frames[:250]...)
	out.Frames = append(out.Frames, ref.Frames[256:]...) // 6 frames gone at 10 s
	sound := noise(20)
	outSound := append(append([]int16{}, sound[:10*AudioRate]...), sound[10*AudioRate+1920:]...) // 240 ms gone
	rows := Measure(Input{Ref: ref, Out: out, RefSound: sound, OutSound: outSound, Boundaries: []float64{10}, Labels: []string{"chapter 2"}})
	r := rows[0]
	if r.Lost != 6 {
		t.Errorf("lost %d frames, want 6", r.Lost)
	}
	if math.Abs(r.ShiftMs+240) > 1 {
		t.Errorf("picture moved %.0f ms, want -240", r.ShiftMs)
	}
	if math.Abs(r.SoundJumpMs+240) > 1 || r.SoundConfidence < 0.9 {
		t.Errorf("sound jumped %.1f ms (confidence %.2f), want -240", r.SoundJumpMs, r.SoundConfidence)
	}
}

func TestMeasureSeesRepeatedFrames(t *testing.T) {
	ref := film(500)
	out := &Pictures{FPS: 25}
	out.Frames = append(out.Frames, ref.Frames[:250]...)
	for range 6 {
		out.Frames = append(out.Frames, ref.Frames[249])
	}
	out.Frames = append(out.Frames, ref.Frames[250:]...)
	r := Measure(Input{Ref: ref, Out: out, Boundaries: []float64{10}})[0]
	if r.Lost != 0 || r.Repeated != 6 || math.Abs(r.MarkMs-240) > 1 {
		t.Errorf("lost %d, repeated %d, mark %.0f ms; want 0, 6, 240", r.Lost, r.Repeated, r.MarkMs)
	}
}

// A short gap in the sound moves nothing after it, but it can be heard.
func TestMeasureSeesASoundDropOut(t *testing.T) {
	ref := film(500)
	sound := noise(20)
	out := append([]int16{}, sound...)
	clear(out[10*AudioRate : 10*AudioRate+AudioRate/25]) // 40 ms of nothing
	r := Measure(Input{Ref: ref, Out: ref, RefSound: sound, OutSound: out, Boundaries: []float64{10}})[0]
	if r.HoleMs < 35 || r.HoleMs > 45 {
		t.Errorf("drop-out %.0f ms, want 40", r.HoleMs)
	}
}

func TestMeasureCleanJoin(t *testing.T) {
	ref := film(500)
	sound := noise(20)
	r := Measure(Input{Ref: ref, Out: ref, RefSound: sound, OutSound: sound, Boundaries: []float64{10}})[0]
	if r.Lost != 0 || r.Repeated != 0 || r.ShiftMs != 0 || r.MarkMs != 0 || r.SoundJumpMs != 0 || r.SyncMs != 0 {
		t.Errorf("a clean join reads as %+v", r)
	}
}
