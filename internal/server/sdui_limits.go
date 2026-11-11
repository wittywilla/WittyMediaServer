// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/http"
	"slices"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/parental"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// Parental controls in server-driven pages.
//
// What a page is filled with is read through the account's limits like everything else
// (sdui_data.go: s.access(r), the same clauses and checks as the list and item routes), so
// every kind of limit on titles holds there without a second mechanism. This file is the
// other half: the parts of a page that aren't titles. A page, built-in or an
// administrator's own, shows a section or a navigation entry on a condition: a feature the
// server has, a module that is installed, a flag, something the profile "can" do. For a
// profile that has been kept from something, the server answers those conditions as if the
// thing weren't there at all:
//
//   - games off: no game features, and WittyPlay and the game-PC modules count as not installed;
//   - Live TV off: no Live TV features, and WittyTV counts as not installed;
//   - recordings off: no "recordings" feature;
//   - watch parties off: no "syncplay" feature; playing on other devices off: no "remote-protocol";
//   - screen time over: no "playback" feature, the flag "screenTimeUp" is set, the document
//     carries screenTime (why, and when there is time again), and nothing in the page
//     offers to play: no Play or Resume button, no play action on a card, track or banner,
//     no playRoute on Top Shelf. The player is the app's own screen now, so there is no
//     page to refuse: the playback routes themselves answer 403 "screen-time" as before.
//
// The same switches are what a "can" condition asks about, and the data sources and
// buttons of a switched-off feature answer nothing (sdui_data.go). A client can't say
// otherwise: these are set after whatever it sent.

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

// profileHidden lists the features and modules a profile's switches take out.
func profileHidden(a *library.Access) (features, modules []string) {
	if a == nil {
		return nil, nil
	}
	for f, h := range profileHides {
		if !a.Can(f) {
			features, modules = append(features, h.features...), append(modules, h.modules...)
		}
	}
	if !a.LiveTV {
		features, modules = append(features, liveTVHides.features...), append(modules, liveTVHides.modules...)
	}
	return features, modules
}

// sduiLimits makes a page's conditions follow the account's limits: ctx's features and
// modules lose what the profile is kept from, and the screenTimeUp flag and ctx.ScreenTime
// say whether its time is over. It answers the verdict when the profile has time rules.
func (s *Server) sduiLimits(r *http.Request, ctx *sdui.RenderContext) *parental.Verdict {
	u := userFrom(r)
	a := accessFor(u)
	if ctx.Flags == nil {
		ctx.Flags = map[string]bool{}
	}
	ctx.Flags["screenTimeUp"] = false
	ctx.ScreenTime = nil
	if a == nil {
		return nil
	}
	features, modules := profileHidden(a)
	var verdict *parental.Verdict
	if !a.Time.Empty() {
		v := a.Time.Check(s.clock(), s.parentalInit().tracker.Usage(r.Context(), u.ID))
		verdict = &v
		if !v.Allowed {
			features = append(features, "playback")
			ctx.Flags["screenTimeUp"] = true
			// Why, and when there is time again: nothing that changes by the second, so a
			// page doesn't count as changed every time it is asked for.
			st := &sdui.ScreenTime{Reason: v.Reason, Message: v.Message()}
			if !v.NextAt.IsZero() {
				st.NextAt = v.NextAt.Unix()
			}
			ctx.ScreenTime = st
		}
	}
	// Copies: the server's own list of features must not be changed for everyone.
	ctx.Features = slices.DeleteFunc(slices.Clone(ctx.Features), func(f string) bool { return slices.Contains(features, f) })
	ctx.Modules = slices.DeleteFunc(slices.Clone(ctx.Modules), func(m string) bool { return slices.Contains(modules, m) })
	return verdict
}
