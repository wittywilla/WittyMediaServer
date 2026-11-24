// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// moduleReportGate lets one browser's reports about a module's version count once an hour.
// Three counted reports in a day take a version out of use for everyone, so one browser going
// over the same failure (a page opened again and again, a tab reloaded) can't be three on its
// own in a few minutes; a bundle that really won't load in it still counts, hour by hour, and
// one that fails in several browsers counts for each.
type moduleReportGate struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// moduleReportEvery is how often one browser's report about one version counts.
var moduleReportEvery = time.Hour

// counts says whether this browser's report about this version counts now, and notes it if so.
// A browser is its sign-in: the session token, kept here only as a hash.
func (g *moduleReportGate) counts(session, module, version string, now time.Time) bool {
	sum := sha256.Sum256([]byte(session))
	key := hex.EncodeToString(sum[:8]) + "|" + module + "@" + version
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.last == nil {
		g.last = map[string]time.Time{}
	}
	if t, ok := g.last[key]; ok && now.Sub(t) < moduleReportEvery {
		return false
	}
	if len(g.last) > 4096 { // forget what no longer matters, so it stays small
		for k, t := range g.last {
			if now.Sub(t) >= moduleReportEvery {
				delete(g.last, k)
			}
		}
	}
	g.last[key] = now
	return true
}
