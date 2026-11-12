// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"strings"
	"time"
)

// The computer's own zone, kept as it was at start: the zone the server runs in when the
// administrator hasn't chosen one (the timeZone setting), and the one it goes back to.
var (
	computerLoc  = time.Local
	computerZone = time.Local.String()
)

// validTimeZone: an IANA name such as Europe/London or America/New_York (or UTC); the empty
// value follows the computer. "Local" is the computer's, so it is the empty value.
func validTimeZone(v string) bool {
	if v == "" || strings.EqualFold(v, "local") {
		return false
	}
	_, err := time.LoadLocation(v)
	return err == nil
}

// applyTimeZone makes the server run in the zone the administrator chose, or the computer's:
// the TV guide's days, schedules, recordings and the times in the logs all follow time.Local.
// Called at start and when the setting is saved.
func (s *Server) applyTimeZone(ctx context.Context) {
	name := s.DB.GetSetting(ctx, "timeZone", "")
	if name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			time.Local = loc
			return
		}
	}
	time.Local = computerLoc
}
