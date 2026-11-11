// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"strings"
	"time"
)

// Evaluate reports whether c holds for ctx. A nil or zero condition holds.
// Every set field must hold; All, Any and Not combine nested conditions.
// {"not": {}} never holds.
func Evaluate(c *Condition, ctx *RenderContext) bool {
	if c == nil {
		return true
	}
	for i := range c.All {
		if !Evaluate(&c.All[i], ctx) {
			return false
		}
	}
	if len(c.Any) > 0 {
		ok := false
		for i := range c.Any {
			if Evaluate(&c.Any[i], ctx) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if c.Not != nil && Evaluate(c.Not, ctx) {
		return false
	}
	ff := ""
	if ctx.Profile != nil {
		ff = ctx.Profile.FormFactor
	}
	if len(c.FormFactor) > 0 && !contains(c.FormFactor, ff) {
		return false
	}
	if len(c.Platform) > 0 && !contains(c.Platform, ctx.Platform) {
		return false
	}
	if len(c.ProfileKind) > 0 && !contains(c.ProfileKind, ctx.ProfileKind) {
		return false
	}
	if c.Admin != nil && *c.Admin != ctx.Admin {
		return false
	}
	if c.Feature != "" && !contains(ctx.Features, c.Feature) {
		return false
	}
	if c.Module != "" && !contains(ctx.Modules, c.Module) {
		return false
	}
	if c.Can != "" && contains(ctx.Cannot, c.Can) {
		return false
	}
	if c.HasLibrary != "" && !contains(ctx.LibraryKinds, c.HasLibrary) {
		return false
	}
	if c.LibraryKind != "" && ctx.Vars["libraryKind"] != c.LibraryKind {
		return false
	}
	if c.Capability != "" && !contains(ctx.Capabilities, c.Capability) {
		return false
	}
	if c.Flag != "" && !ctx.Flags[c.Flag] {
		return false
	}
	if w := ctx.Viewport.Width; w > 0 {
		if c.MinWidth > 0 && w < c.MinWidth {
			return false
		}
		if c.MaxWidth > 0 && w > c.MaxWidth {
			return false
		}
	}
	if c.TimeFrom != "" || c.TimeTo != "" {
		now := ctx.Now.Hour()*60 + ctx.Now.Minute()
		from, _ := parseClock(c.TimeFrom)
		to, ok := parseClock(c.TimeTo)
		if !ok {
			to = 24 * 60
		}
		if from <= to {
			if now < from || now >= to {
				return false
			}
		} else if now < from && now >= to { // a range that passes midnight
			return false
		}
	}
	if len(c.Weekday) > 0 && !contains(c.Weekday, Weekdays[ctx.Now.Weekday()]) {
		return false
	}
	if c.Locale != "" && !localeMatches(ctx.Locale, c.Locale) {
		return false
	}
	return true
}

// parseClock reads "HH:MM" as minutes past midnight.
func parseClock(s string) (int, bool) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

func localeMatches(have, want string) bool {
	have, want = strings.ToLower(have), strings.ToLower(want)
	return have == want || strings.HasPrefix(have, want+"-") || strings.HasPrefix(have, want+"_")
}

// usesClock reports whether a condition depends on the time of day or the
// day of the week, so a render that used it can't be kept for long.
func usesClock(c *Condition) bool {
	if c == nil {
		return false
	}
	if c.TimeFrom != "" || c.TimeTo != "" || len(c.Weekday) > 0 || usesClock(c.Not) {
		return true
	}
	for i := range c.All {
		if usesClock(&c.All[i]) {
			return true
		}
	}
	for i := range c.Any {
		if usesClock(&c.Any[i]) {
			return true
		}
	}
	return false
}
