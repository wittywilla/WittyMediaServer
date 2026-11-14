// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/livetv"
	"github.com/wittywilla/wittymediaserver/internal/livetv/livetvtest"
)

// The tuners and their channels are WittyTuner's (module API 5): a test of the server's guide,
// recordings or sets puts channels in a stand-in for it.

// tunersInTest is the stand-in of the test running now, for helpers that change a channel.
var tunersInTest *livetvtest.Directory

// useTuners puts a stand-in for WittyTuner in place for the rest of the test.
func useTuners(t testing.TB) *livetvtest.Directory {
	t.Helper()
	d := livetvtest.Use(t)
	tunersInTest = d
	t.Cleanup(func() { tunersInTest = nil })
	return d
}

// tunerChannel puts a channel on tuner 1 (or c.TunerID), in the guide unless c says otherwise.
func tunerChannel(t testing.TB, c livetv.Channel, addr string) *livetv.Channel {
	t.Helper()
	if tunersInTest == nil {
		useTuners(t)
	}
	return tunersInTest.Add(c, addr)
}

// changeTunerChannel changes a channel as WittyTuner's settings would.
func changeTunerChannel(s *Server, id int64, fn func(*livetv.Channel)) {
	tunersInTest.Change(s.TV, id, fn)
}
