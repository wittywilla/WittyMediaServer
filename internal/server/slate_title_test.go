// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/channels"
)

// A stretch with nothing scheduled is named as one in the guide, not "We'll be right back", which reads like a
// title that is missing (#634).
func TestSlateTitle(t *testing.T) {
	short := fromSchedule(channels.Programme{Start: 0, End: 600, Kind: channels.KindSlate})
	if short.Title != "Short break" {
		t.Fatalf("a 10-minute gap is %q", short.Title)
	}
	long := fromSchedule(channels.Programme{Start: 0, End: 7200, Kind: channels.KindSlate})
	if long.Title != "Off air" {
		t.Fatalf("a 2-hour gap is %q", long.Title)
	}
	show := fromSchedule(channels.Programme{Start: 0, End: 600, Kind: channels.KindShow, Title: "Bee Movie"})
	if show.Title != "Bee Movie" {
		t.Fatalf("a show keeps its title, got %q", show.Title)
	}
}
