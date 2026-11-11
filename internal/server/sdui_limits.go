// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"errors"
	"net/http"
	"slices"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/parental"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// Parental controls in server-driven screens.
//
// What a screen is filled with is read through the account's limits like everything else
// (sdui_data.go: s.access(r), the same clauses and checks as the list and item routes), so
// every kind of limit on titles holds there without a second mechanism. This file is the
// other half: the parts of a screen that aren't titles. A layout, stock or an
// administrator's own, shows a tab, a row or a button on a condition: a feature the server
// has, a module that is installed, a flag. For a profile that has been kept from something,
// the server answers those conditions as if the thing weren't there at all:
//
//   - games off: no game features, and WittyPlay and the game-PC modules count as not installed;
//   - Live TV off: no Live TV features, and WittyTV counts as not installed;
//   - recordings off: no "recordings" feature;
//   - watch parties off: no "syncplay" feature; playing on other devices off: no "remote-protocol";
//   - screen time over: no "playback" feature (so no Play or Resume button is sent), the
//     flag "screenTimeUp" is set and $.screenTime says why, and when there is time again. The
//     "player" screen itself is refused, with the same answer the playback routes give.
//
// A client can't say otherwise: these are set after whatever it sent.

// profileHides is what a profile's switches take out of a screen's conditions.
var profileHides = map[string]struct{ features, modules []string }{
	library.FeatureGames: {
		features: []string{"game-metadata", "game-controller-profiles", "game-settings", "custom-game-cores", "game-rigs", "standalone-emulators"},
		modules:  []string{"wittyplay", "wittycouch", "wittyrigs"},
	},
	library.FeatureRecordings:    {features: []string{"recordings"}},
	library.FeatureSyncplay:      {features: []string{"syncplay"}},
	library.FeatureRemoteControl: {features: []string{"remote-protocol"}},
}

// liveTVHides is the same for an account that doesn't see Live TV at all.
var liveTVHides = struct{ features, modules []string }{
	features: []string{"live-tv", "live-tv-stream-tokens", "tv-favourites", "tv-reminders", "recordings"},
	modules:  []string{"wittytv", "wittytuner"},
}

// errScreenTimeUp: the screen is for playing, and this profile's time is over.
var errScreenTimeUp = errors.New("this profile's screen time is over")

// sduiLimits makes a screen's conditions follow the account's limits. screen is the screen
// being rendered. It answers the verdict when the profile has time rules.
func (s *Server) sduiLimits(r *http.Request, screen string, ctx *sdui.RenderContext) (*parental.Verdict, error) {
	u := userFrom(r)
	a := accessFor(u)
	ctx.Flags["screenTimeUp"] = false
	if a == nil {
		return nil, nil
	}
	var features, modules []string
	for f, h := range profileHides {
		if !a.Can(f) {
			features, modules = append(features, h.features...), append(modules, h.modules...)
		}
	}
	if !a.LiveTV {
		features, modules = append(features, liveTVHides.features...), append(modules, liveTVHides.modules...)
	}
	var verdict *parental.Verdict
	if !a.Time.Empty() {
		v := a.Time.Check(s.clock(), s.parentalInit().tracker.Usage(r.Context(), u.ID))
		verdict = &v
		if !v.Allowed {
			if screen == "player" {
				return verdict, errScreenTimeUp
			}
			features = append(features, "playback")
			ctx.Flags["screenTimeUp"] = true
			// Why, and when there is time again: nothing that changes by the second, so a
			// screen doesn't count as changed every time it is asked for.
			st := map[string]any{"reason": v.Reason, "message": v.Message()}
			if !v.NextAt.IsZero() {
				st["nextAt"] = v.NextAt.Unix()
			}
			ctx.Data["screenTime"] = st
		}
	}
	ctx.Features = slices.DeleteFunc(ctx.Features, func(f string) bool { return slices.Contains(features, f) })
	ctx.Modules = slices.DeleteFunc(ctx.Modules, func(m string) bool { return slices.Contains(modules, m) })
	return verdict, nil
}

// sduiLibrary is a library this account may open in a screen: within its limits, and not a
// games library when games are off for it.
func (s *Server) sduiLibrary(r *http.Request, id int64) (*library.Library, bool) {
	if id <= 0 {
		return nil, false
	}
	libs, err := s.visibleLibraries(r.Context(), s.access(r))
	if err != nil {
		return nil, false
	}
	for i := range libs {
		if libs[i].ID == id {
			return &libs[i], true
		}
	}
	return nil, false
}
