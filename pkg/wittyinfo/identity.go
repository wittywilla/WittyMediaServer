// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyinfo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
)

// FingerprintKind is the fingerprint this version makes: SHA-256 over the file's size in
// decimal, a newline, its first MiB and its last MiB. It names one exact file cheaply (two
// reads, whatever its size); a remux or another encode of the same cut has another one, and
// the length check decides for those.
const FingerprintKind = "sha256-headtail-v1"

const fingerprintChunk = 1 << 20

// FingerprintFile works out FingerprintKind for a file.
func FingerprintFile(path string) (*Fingerprint, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	fp, err := FingerprintReader(f, st.Size())
	return fp, st.Size(), err
}

// FingerprintReader works out FingerprintKind for size bytes read through r.
func FingerprintReader(r io.ReaderAt, size int64) (*Fingerprint, error) {
	h := sha256.New()
	fmt.Fprintf(h, "%d\n", size)
	head := min(size, fingerprintChunk)
	if _, err := io.Copy(h, io.NewSectionReader(r, 0, head)); err != nil {
		return nil, err
	}
	tailFrom := max(size-fingerprintChunk, head)
	if _, err := io.Copy(h, io.NewSectionReader(r, tailFrom, size-tailFrom)); err != nil {
		return nil, err
	}
	return &Fingerprint{Kind: FingerprintKind, Value: hex.EncodeToString(h.Sum(nil))}, nil
}

// DefaultTolerance is how far a video's length may be from the one a file was made for
// and still be the same cut: two seconds, or half a percent for anything over 400 seconds.
// Another encode or a remux moves the length by a frame or two; a different cut by minutes.
func DefaultTolerance(durationSec float64) float64 { return math.Max(2, durationSec*0.005) }

// Match results.
const (
	MatchExact     = "exact"     // the fingerprint is the same: this very file
	MatchSameCut   = "same-cut"  // the length agrees: another copy of the same cut
	MatchNoCheck   = "unchecked" // the file doesn't say, or the video's length isn't known
	MatchOtherCut  = "other-cut" // the length is wrong: made for another cut, not used
	MatchForced    = "forced"    // made for another cut, used anyway because someone said so
	MatchNotNeeded = "defaults"  // a season or show file: no one video to compare with
)

// Match says whether a file made for one video fits another.
type Match struct {
	Result string `json:"result"`
	// Usable: the file's times can be used on this video.
	Usable bool `json:"usable"`
	// Message says it in words, for a person.
	Message string `json:"message,omitempty"`
	// DeltaSec: the video's length less the one the file was made for.
	DeltaSec float64 `json:"deltaSec,omitempty"`
}

// MatchVideo compares a file with the video it is about to be used on. durationSec is the
// video's length (0 if not known) and fp its fingerprint (nil if not worked out).
func (x *Info) MatchVideo(durationSec float64, fp *Fingerprint) Match {
	if x.ScopeOr() != ScopeItem {
		return Match{Result: MatchNotNeeded, Usable: true}
	}
	id := x.Identity
	if id == nil || id.DurationSec <= 0 {
		return Match{Result: MatchNoCheck, Usable: true, Message: "The file doesn't say which cut it was made for."}
	}
	if fp != nil && id.Fingerprint != nil && id.Fingerprint.Kind == fp.Kind && id.Fingerprint.Value == fp.Value {
		return Match{Result: MatchExact, Usable: true}
	}
	if durationSec <= 0 {
		return Match{Result: MatchNoCheck, Usable: true, Message: "The video's length isn't known yet, so the cut wasn't checked."}
	}
	tol := DefaultTolerance(float64(id.DurationSec))
	if id.ToleranceSec != nil {
		tol = *id.ToleranceSec
	}
	delta := durationSec - float64(id.DurationSec)
	if math.Abs(delta) <= tol {
		return Match{Result: MatchSameCut, Usable: true, DeltaSec: round3(delta)}
	}
	return Match{Result: MatchOtherCut, Usable: false, DeltaSec: round3(delta),
		Message: fmt.Sprintf("Made for a %s cut; this video is %s long.", Clock(math.Round(float64(id.DurationSec))), Clock(math.Round(durationSec)))}
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
