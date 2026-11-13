// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package lyrics

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os/exec"
	"testing"
	"time"
)

// A curve with a voice starting 700 ms after each line says so, whichever way the lines are off.
func TestBestOffsetFindsTheShift(t *testing.T) {
	for _, shift := range []int64{700, -1250, 0, 1900} {
		starts := []int64{2000, 6500, 9800, 14200, 18000, 23500, 27100, 31800}
		curve := make([]float64, 40000/FrameMS)
		rnd := rand.New(rand.NewSource(1))
		for i := range curve {
			curve[i] = 0.3 * rnd.Float64()
		}
		for _, s := range starts {
			at := int((s + shift) / FrameMS)
			for f := at; f < at+30 && f < len(curve); f++ { // two seconds of singing
				if f >= 0 {
					curve[f] += 4
				}
			}
		}
		got, conf, ok := BestOffset(curve, starts)
		if !ok || got != shift {
			t.Errorf("shift %d: got %d (sure %.1f, ok %v)", shift, got, conf, ok)
		}
	}
}

// Noise with no voice in it moves nothing.
func TestBestOffsetLeavesNoiseAlone(t *testing.T) {
	rnd := rand.New(rand.NewSource(2))
	curve := make([]float64, 30000/FrameMS)
	for i := range curve {
		curve[i] = rnd.Float64()
	}
	if got, conf, ok := BestOffset(curve, []int64{1000, 5000, 9000, 13000, 17000, 21000}); ok {
		t.Fatalf("moved noise by %d (sure %.1f)", got, conf)
	}
	if _, _, ok := BestOffset(curve, []int64{1000, 2000}); ok {
		t.Fatal("two lines are too few to go on")
	}
}

// The middle of the mix counts and the sides don't: a sound panned hard left is not a voice.
func TestVoiceCurveHearsTheMiddle(t *testing.T) {
	frame := AutoSyncRate * FrameMS / 1000
	pcm := make([]byte, 0, frame*4*2)
	for i := 0; i < frame; i++ { // centre
		v := int16(8000 * math.Sin(float64(i)))
		pcm = binary.LittleEndian.AppendUint16(pcm, uint16(v))
		pcm = binary.LittleEndian.AppendUint16(pcm, uint16(v))
	}
	for i := 0; i < frame; i++ { // left only
		v := int16(8000 * math.Sin(float64(i)))
		pcm = binary.LittleEndian.AppendUint16(pcm, uint16(v))
		pcm = binary.LittleEndian.AppendUint16(pcm, 0)
	}
	c := VoiceCurve(pcm, AutoSyncRate)
	if len(c) != 2 || c[0] <= 1 || c[1] != 0 {
		t.Fatalf("curve %v", c)
	}
}

// From real audio through FFmpeg: centred bursts 600 ms after each line, over a hum at the sides.
func TestAutoSyncThroughFFmpeg(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg")
	}
	starts := []int64{1000, 4500, 8000, 11500, 15000, 18500}
	enable := ""
	for i, s := range starts {
		if i > 0 {
			enable += "+"
		}
		a := float64(s+600) / 1000
		enable += fmt.Sprintf("between(t,%.2f,%.2f)", a, a+1.5)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dir := t.TempDir()
	song := dir + "/song.wav"
	// A voice-like tone in the middle, switched on and off; a steady hum on the left only.
	gen := exec.CommandContext(ctx, ff, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100:duration=22",
		"-f", "lavfi", "-i", "sine=frequency=1200:sample_rate=44100:duration=22",
		"-filter_complex", "[0]volume=enable='"+enable+"':volume=1,volume=enable='not("+enable+")':volume=0[v];[v]asplit[vl][vr];[1]volume=0.3[h];[vl][h]amix=inputs=2:normalize=0[l];[l][vr]join=inputs=2:channel_layout=stereo[out]",
		"-map", "[out]", song)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("making the song: %v %s", err, out)
	}
	pcm, err := exec.CommandContext(ctx, ff, AutoSyncArgs(song)...).Output()
	if err != nil {
		t.Fatal(err)
	}
	got, conf, ok := BestOffset(VoiceCurve(pcm, AutoSyncRate), starts)
	if !ok || got < 550 || got > 650 {
		t.Fatalf("got %d ms (sure %.1f, ok %v), want about 600", got, conf, ok)
	}
}
