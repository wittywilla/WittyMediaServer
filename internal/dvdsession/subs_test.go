// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package dvdsession

import (
	"os"
	"strings"
	"testing"
)

// TestRealDiscSubtitles lists the main feature's first subtitle events from
// an unencrypted DVD image when WMS_TEST_ISO points at one (skipped otherwise)
// and fetches each picture by the name in its URL.
func TestRealDiscSubtitles(t *testing.T) {
	p := os.Getenv("WMS_TEST_ISO")
	if p == "" {
		t.Skip("set WMS_TEST_ISO to an unencrypted DVD image")
	}
	m := &Manager{BaseURL: "/api", sessions: map[string]*Session{}}
	s, err := m.Open(1, p, Prefs{Wide: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	main := s.opened.Disc.MainTitle()
	evs, err := s.Subtitles(main.Number, 0, 0, 600)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) == 0 {
		t.Skip("no subtitles in the first ten minutes")
	}
	for _, ev := range evs {
		name := ev.URL[strings.LastIndexByte(ev.URL, '/')+1:]
		if b, ok := s.SubPNG(name); !ok || len(b) == 0 || ev.End <= ev.Start {
			t.Fatalf("event %+v: picture %q missing", ev, name)
		}
	}
	t.Logf("%d subtitle events in the first ten minutes of title %d", len(evs), main.Number)
}
