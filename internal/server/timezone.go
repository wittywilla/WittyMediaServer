// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
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
// Called once, at start. Go has no safe way to change time.Local while anything else may be
// reading the clock, so a zone chosen later takes effect when the server starts again
// (restartForTimeZone), as a new port does.
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

// timeZoneWaiting: a zone was saved that the server isn't running in yet.
func (s *Server) timeZoneWaiting(ctx context.Context) bool {
	want := s.DB.GetSetting(ctx, "timeZone", "")
	if want == "" {
		return time.Local.String() != computerZone
	}
	return want != time.Local.String()
}

// restartForTimeZone starts the server again, so a zone saved since it started is the one it
// runs in. Not during first-run setup, which would lose its place: finishing setup does it.
// by is who changed it, for the log.
func (s *Server) restartForTimeZone(ctx context.Context, by string) bool {
	if s.Restart == nil || !s.timeZoneWaiting(ctx) || !s.setupComplete(ctx) {
		return false
	}
	logging.NoteStop(logging.StopTimeZone, "a new time zone", by)
	time.AfterFunc(time.Second, s.Restart) // the page has its answer first
	return true
}
