// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
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
//
// By then the log and more are running, so time.Local is only written when the zone really
// differs: EarlyTimeZone has normally set it already, before any of them started.
func (s *Server) applyTimeZone(ctx context.Context) {
	if want := zoneFor(s.DB.GetSetting(ctx, "timeZone", "")); time.Local.String() != want.String() {
		time.Local = want
	}
}

// zoneFor is the zone a timeZone setting names, or the computer's.
func zoneFor(name string) *time.Location {
	if name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	return computerLoc
}

// EarlyTimeZone sets the saved zone before anything else runs, from a read-only look at the
// database in dataDir: the log's goroutine reads the clock from the moment it starts, and Go
// has no safe way to change time.Local under it. No database yet means the computer's zone.
func EarlyTimeZone(dataDir string) {
	d, err := db.OpenExisting(dataDir, true)
	if err != nil {
		return
	}
	defer d.Close()
	if want := zoneFor(d.GetSetting(context.Background(), "timeZone", "")); time.Local.String() != want.String() {
		time.Local = want
	}
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
