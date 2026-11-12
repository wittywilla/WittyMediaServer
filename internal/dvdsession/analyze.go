// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package dvdsession

import (
	"sync/atomic"

	"github.com/wittywilla/wittymediaserver/internal/dvd"
)

// Analysis is what the WittyDVD in use makes of a disc beyond its structure: which title
// is the film and why. It is worked out in a helper of its own, apart from any session's,
// so an analysis that fails or runs long costs no viewer their disc.
type Analysis struct {
	// Ranking is nil when the WittyDVD in use is from before the ranking: callers then
	// fall back on Disc.MainTitle.
	Ranking *dvd.Ranking
	// Repairs are where the disc's tables disagreed and what was mended.
	Repairs []dvd.Repair
	// Stats count the sectors the analysis couldn't read and passed on as zeros.
	Stats dvd.ReadStats
	// Engine is the version of WittyDVD that gave the answer.
	Engine string
	// Protected: the server's own look found encrypted video. The structure is still
	// read, since the index files of a disc are never encrypted; nothing of it plays.
	Protected bool
}

var analyzer atomic.Pointer[func(path string) (*Analysis, error)]

// SetAnalyzer says how discs are analysed, once at start.
func SetAnalyzer(analyze func(path string) (*Analysis, error)) { analyzer.Store(&analyze) }

// Analyze asks the WittyDVD in use about a disc. The answer is remembered for as long as
// the disc on disk stays the same, so asking again is cheap.
func Analyze(path string) (*Analysis, error) {
	if a := analyzer.Load(); a != nil {
		return (*a)(path)
	}
	return nil, ErrNoEngine
}
