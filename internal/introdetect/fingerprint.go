// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package introdetect finds the part episodes of a season share: the title
// sequence at the front, the credits music at the back. Nothing marks it in
// the files, so it is found by ear: each episode's sound is turned into a
// fingerprint, the fingerprints of neighbouring episodes are slid over each
// other, and the longest stretch where they agree is the shared part.
//
// The fingerprint is the one Haitsma and Kalker described ("A Highly Robust
// Audio Fingerprinting System", 2002), cut down: sound at 5512 Hz, mono, a
// frame ten times a second, the energy in 17 bands from 300 Hz to 2 kHz, and
// 16 bits per frame that say whether the difference between neighbouring bands
// rose or fell since the frame before. Two recordings of the same music agree
// on most bits whatever their loudness or encoding; two different sounds agree
// on about half. Nothing here needs more than the standard library, a model,
// or a network.
//
// It is sized for a small machine: ten minutes of an episode is 6.6 MB of
// sound and 6000 fingerprints, and comparing two episodes is 36 million
// 16-bit comparisons.
package introdetect

import (
	"math"
	"math/bits"
)

const (
	// SampleRate is what the sound is resampled to, in Hz. Title music is
	// recognisable well below 2 kHz, and less sound is less work.
	SampleRate = 5512
	// frameLen is the samples one frame looks at (0.37 s), hop how far it moves
	// on: FramesPerSec frames a second.
	frameLen = 2048
	hop      = SampleRate / FramesPerSec
	// FramesPerSec is how many fingerprints a second of sound makes.
	FramesPerSec = 10
	bands        = 17
	// Silent marks a frame too quiet to say anything: two silences are alike,
	// and that mustn't count as a match.
	Silent = 1 << 31
)

// Fingerprint is one value per frame: 16 bits, or Silent.
type Fingerprint []uint32

// bandEdges are the FFT bins where each band starts, spaced evenly in pitch
// from 300 Hz to 2 kHz.
var bandEdges = func() [bands + 1]int {
	var e [bands + 1]int
	for i := range e {
		hz := 300 * math.Pow(2000.0/300, float64(i)/bands)
		e[i] = int(math.Round(hz * frameLen / SampleRate))
	}
	return e
}()

var window = func() []float64 {
	w := make([]float64, frameLen)
	for i := range w {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(frameLen-1))
	}
	return w
}()

// FromPCM fingerprints mono 16-bit little-endian sound at SampleRate.
func FromPCM(pcm []byte) Fingerprint {
	n := len(pcm) / 2
	if n < frameLen {
		return nil
	}
	frames := (n-frameLen)/hop + 1
	out := make(Fingerprint, 0, frames)
	re, im := make([]float64, frameLen), make([]float64, frameLen)
	var prev [bands]float64
	for f := 0; f < frames; f++ {
		off := f * hop * 2
		var loud float64
		for i := 0; i < frameLen; i++ {
			s := float64(int16(uint16(pcm[off+2*i]) | uint16(pcm[off+2*i+1])<<8))
			loud += s * s
			re[i], im[i] = s*window[i], 0
		}
		fft(re, im)
		var e [bands]float64
		for b := 0; b < bands; b++ {
			for k := bandEdges[b]; k < bandEdges[b+1]; k++ {
				e[b] += re[k]*re[k] + im[k]*im[k]
			}
		}
		var v uint32
		for b := 0; b < bands-1; b++ {
			if (e[b]-e[b+1])-(prev[b]-prev[b+1]) > 0 {
				v |= 1 << b
			}
		}
		prev = e
		// Quieter than about -60 dB of full scale: nothing to recognise.
		if math.Sqrt(loud/frameLen) < 33 {
			v = Silent
		}
		out = append(out, v)
	}
	return out
}

// fft is an in-place radix-2 transform; len(re) is a power of two.
func fft(re, im []float64) {
	n := len(re)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		ang := -2 * math.Pi / float64(size)
		wr, wi := math.Cos(ang), math.Sin(ang)
		for start := 0; start < n; start += size {
			cr, ci := 1.0, 0.0
			for k := 0; k < size/2; k++ {
				a, b := start+k, start+k+size/2
				tr, ti := re[b]*cr-im[b]*ci, re[b]*ci+im[b]*cr
				re[b], im[b] = re[a]-tr, im[a]-ti
				re[a], im[a] = re[a]+tr, im[a]+ti
				cr, ci = cr*wr-ci*wi, cr*wi+ci*wr
			}
		}
	}
}

// Match is a stretch two fingerprints share, in frames: [StartA, StartA+Len)
// of the first is the same sound as [StartB, StartB+Len) of the second.
type Match struct {
	StartA, StartB, Len int
	// Score is the share of bits that agree over the stretch, 0.5 for chance
	// and 1 for the same recording.
	Score float64
}

// Seconds turns the match into times in each recording.
func (m Match) Seconds() (startA, endA, startB, endB float64) {
	f := float64(FramesPerSec)
	return float64(m.StartA) / f, float64(m.StartA+m.Len) / f, float64(m.StartB) / f, float64(m.StartB+m.Len) / f
}

const (
	// maxBitErrors is how many of a frame's 16 bits may differ and the frames
	// still count as the same sound. Chance gives 8; at most 4 happens by
	// chance for 1 frame in 26.
	maxBitErrors = 4
	// maxGap is how many frames in a row may disagree inside a match: a second
	// and a half of an episode's own sound over the titles (a voice, a door).
	maxGap = 15
	// minDensity: at least this share of a match's frames agree.
	minDensity = 0.6
)

// Longest finds the longest stretch a and b share that is at least minFrames
// long, trying every way of sliding one over the other. ok is false when there
// is none. It is deterministic: of two equally long stretches the earlier in a
// wins.
func Longest(a, b Fingerprint, minFrames int) (best Match, ok bool) {
	if len(a) == 0 || len(b) == 0 {
		return Match{}, false
	}
	for d := -(len(a) - 1); d < len(b); d++ {
		// a[i] against b[i+d]
		i0 := max(0, -d)
		n := min(len(a)-i0, len(b)-(i0+d))
		if n < minFrames || n <= best.Len {
			continue
		}
		start, hits, agree, gap, lastHit := -1, 0, 0, 0, -1
		flush := func() {
			if start < 0 {
				return
			}
			length := lastHit - start + 1
			if length >= minFrames && length > best.Len && float64(hits) >= minDensity*float64(length) {
				best, ok = Match{StartA: start, StartB: start + d, Len: length, Score: float64(agree) / float64(16*hits)}, true
			}
			start, hits, agree, gap = -1, 0, 0, 0
		}
		for i := i0; i < i0+n; i++ {
			x, y := a[i], b[i+d]
			if x&Silent == 0 && y&Silent == 0 {
				if e := bits.OnesCount32(x ^ y); e <= maxBitErrors {
					if start < 0 {
						start = i
					}
					hits++
					agree += 16 - e
					gap, lastHit = 0, i
					continue
				}
			}
			if start >= 0 {
				if gap++; gap > maxGap {
					flush()
				}
			}
		}
		flush()
	}
	if ok {
		best = trim(a, b, best)
		ok = best.Len >= minFrames
	}
	return best, ok
}

// edge is the window, in frames, a match's ends are held to: two seconds.
const edge = 20

// trim pulls a match's ends in to where the agreement is solid. A stray frame
// that happens to agree a second before the music starts would otherwise be
// taken for its start.
func trim(a, b Fingerprint, m Match) Match {
	d := m.StartB - m.StartA
	hit := func(i int) bool {
		x, y := a[i], b[i+d]
		return x&Silent == 0 && y&Silent == 0 && bits.OnesCount32(x^y) <= maxBitErrors
	}
	solid := func(from int) bool { // the window starting at from
		n := 0
		for i := from; i < from+edge; i++ {
			if hit(i) {
				n++
			}
		}
		return float64(n) >= minDensity*edge
	}
	start, end := m.StartA, m.StartA+m.Len
	for end-start >= edge && !(hit(start) && solid(start)) {
		start++
	}
	for end-start >= edge && !(hit(end-1) && solid(end-edge)) {
		end--
	}
	if end-start < edge {
		return Match{}
	}
	m.StartA, m.StartB, m.Len = start, start+d, end-start
	return m
}
