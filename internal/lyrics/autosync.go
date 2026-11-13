// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package lyrics

import (
	"encoding/binary"
	"math"
	"sort"
)

// Lyrics that come with timing (LRC, from a file or online) are often timed against another
// release of the song, so every line is early or late by the same amount. AutoSync finds that
// amount by listening: it hears where a voice starts singing and moves the lines to meet it.
// It is cheap on purpose: one pass of FFmpeg at 8 kHz gives a curve of twenty numbers a
// second, and lining the lines up with it is a few thousand additions.

// Frame is the length of one step of the voice curve.
const FrameMS = 50

// AutoSyncRate is the sample rate the audio is decoded at for listening. A voice's start is
// plain at telephone quality, and it keeps the decoding quick.
const AutoSyncRate = 8000

// AutoSyncRange is how far the lines may be moved, either way. Timed lyrics are seldom further out
// than this, and a bigger move found by listening was more often a piano than a voice.
const AutoSyncRange = 2000

// AutoSyncArgs are FFmpeg's arguments for the audio AutoSync listens to: the first audio stream
// of path (its first fifteen minutes), at AutoSyncRate, as two channels of 16-bit samples on its output, band-limited to
// where a voice is.
func AutoSyncArgs(path string) []string {
	return []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-t", "900", "-i", path, "-map", "0:a:0", "-vn",
		"-af", "highpass=f=180,lowpass=f=3400", "-ac", "2", "-ar", "8000", "-f", "s16le", "-"}
}

// VoiceCurve is, for each FrameMS of pcm (16-bit little-endian, two channels, at rate), how
// much of what is heard is in the middle of the mix, where a singer almost always is, beyond
// what is at the sides: a rough measure of a voice. One channel is taken as both.
func VoiceCurve(pcm []byte, rate int) []float64 {
	per := rate * FrameMS / 1000
	if per <= 0 {
		return nil
	}
	frames := len(pcm) / 4
	out := make([]float64, 0, frames/per+1)
	var mid, side float64
	n := 0
	for i := 0; i+3 < len(pcm); i += 4 {
		l := float64(int16(binary.LittleEndian.Uint16(pcm[i:])))
		r := float64(int16(binary.LittleEndian.Uint16(pcm[i+2:])))
		m, s := (l+r)/2, (l-r)/2
		mid += m * m
		side += s * s
		n++
		if n == per {
			v := mid - 1.2*side
			if v < 0 {
				v = 0
			}
			out = append(out, math.Log1p(v/float64(per)))
			mid, side, n = 0, 0, 0
		}
	}
	return out
}

// BestOffset is how far the lines whose start times (ms) are starts should move so they begin
// where a voice in curve begins, and how sure it is (how far the best fit stands out from the
// rest, 0 to 1). ok is false when there is too little to go on or no fit
// stands out: then the lines are best left where they were.
func BestOffset(curve []float64, starts []int64) (offsetMS int64, confidence float64, ok bool) {
	if len(curve) < 40 || len(starts) < 4 {
		return 0, 0, false
	}
	scores, steps := offsetScores(curve, starts)
	best, bestAt := math.Inf(-1), 0
	for i, v := range scores {
		if v > best {
			best, bestAt = v, i-steps
		}
	}
	if best <= 0 {
		return 0, 0, false
	}
	sorted := append([]float64(nil), scores...)
	sort.Float64s(sorted)
	median := sorted[len(sorted)/2]
	// How much of the best fit is more than a typical one, and how near the best other fit comes
	// (any more than 300 ms from the best).
	second := 0.0
	for i, v := range scores {
		if k := i - steps; (k-bestAt > 6 || bestAt-k > 6) && v > second {
			second = v
		}
	}
	confidence = (best - median) / best
	return int64(bestAt * FrameMS), confidence, confidence >= AutoSyncSure && second <= AutoSyncRunnerUp*best
}

// AutoSyncSure is how much of the best fit has to be more than a typical one before the lines are
// moved by it, and AutoSyncRunnerUp how near any other fit may come. Both are strict: lines left
// where they were cost nothing, lines moved wrongly spoil the song.
const (
	AutoSyncSure     = 0.5
	AutoSyncRunnerUp = 0.75
)

// LineStarts are the start times of the lines that have words, in order, at most limit of them.
func LineStarts(lines []Line, limit int) []int64 {
	var out []int64
	for _, l := range lines {
		if l.Text == "" {
			continue
		}
		out = append(out, l.T)
		if len(out) == limit {
			break
		}
	}
	return out
}

// offsetScores is, for each move from -AutoSyncRange to +AutoSyncRange in FrameMS steps, how
// strongly a voice starts where the moved lines start; steps is the index of no move at all.
func offsetScores(curve []float64, starts []int64) (scores []float64, steps int) {
	// Where the voice grows: the rise across 150 ms centred on each frame, nothing where it falls.
	rise := make([]float64, len(curve))
	for i := 2; i+1 < len(curve); i++ {
		if d := curve[i+1] - curve[i-2]; d > 0 {
			rise[i] = d
		}
	}
	steps = AutoSyncRange / FrameMS
	scores = make([]float64, 0, 2*steps+1)
	for k := -steps; k <= steps; k++ {
		var sum float64
		for _, t := range starts {
			f := int(t/FrameMS) + k
			// A line's time is rounded by whoever wrote it: the strongest rise within a frame either side.
			var m float64
			for g := f - 1; g <= f+1; g++ {
				if g >= 0 && g < len(rise) && rise[g] > m {
					m = rise[g]
				}
			}
			sum += m
		}
		scores = append(scores, sum)
	}
	return scores, steps
}
