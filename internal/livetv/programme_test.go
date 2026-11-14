// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"errors"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/tuner/xmltv"
)

func programmeLineup(t *testing.T) *Manager {
	t.Helper()
	m, d := testManager(t)
	add := func(tn int64, number, name, epg string, fn func(*Channel)) {
		c := d.add(tn, Channel{Number: number, Name: name, EPGID: epg}, "x")
		if fn != nil {
			d.change(m, c.ID, func(x *Channel) { fn(x) })
		}
	}
	add(1, "1", "Harbour", "harbour.test", nil)
	add(1, "2", "Harbour backup", "harbour.test", func(c *Channel) { c.MergedInto = 1 })
	add(1, "3", "Pine", "pine.test", nil)
	add(2, "4", "On a tuner turned off", "pine.test", func(c *Channel) { c.TunerEnabled = false })
	add(1, "5", "Turned off", "pine.test", func(c *Channel) { c.Enabled = false })
	return m
}

func TestProgrammeAt(t *testing.T) {
	m := programmeLineup(t)
	ctx := t.Context()
	extra := detailsJSON(xmltv.Details{Year: 2024, Credits: []xmltv.Credit{{Role: "actor", Name: "Mara Voss", Character: "Nell"}}})
	if detailsJSON(xmltv.Details{}) != "" {
		t.Fatal("a programme with nothing more stores something")
	}
	for _, q := range []struct {
		ch, title, extra string
	}{{"harbour.test", "The Quiet Orchard", extra}, {"tuner:1", "What the broadcast said", ""}} {
		if _, err := m.DB.Exec(`INSERT INTO epg_programmes(channel, start, stop, title, season, episode, extra) VALUES(?, 1000, 2000, ?, 1, 3, ?)`, q.ch, q.title, q.extra); err != nil {
			t.Fatal(err)
		}
	}
	c, err := m.Channel(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	p, d, err := m.ProgrammeAt(ctx, c, 1500)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "The Quiet Orchard" || *p.Season != 1 || *p.Episode != 3 || d.Year != 2024 || len(d.Credits) != 1 || d.Credits[0].Character != "Nell" {
		t.Fatalf("%+v %+v", p, d)
	}
	if _, _, err := m.ProgrammeAt(ctx, c, 2000); !errors.Is(err, ErrNoProgramme) {
		t.Fatalf("after it ends: %v", err)
	}
	// With no guide, what the broadcast said.
	directory.(*fakeDirectory).change(m, 1, func(c *Channel) { c.EPGID = "" })
	c, _ = m.Channel(ctx, 1)
	if p, d, err = m.ProgrammeAt(ctx, c, 1000); err != nil || p.Title != "What the broadcast said" || d.Year != 0 {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestAirings(t *testing.T) {
	m := programmeLineup(t)
	ctx := t.Context()
	now := time.Now()
	at := now.Add(time.Hour).Unix()
	for i, ch := range []string{"harbour.test", "pine.test", "pine.test", "tuner:3", "elsewhere.test"} {
		start := at + int64(i)*3600
		if ch == "tuner:3" {
			start = at + 3600 // the broadcast's own copy of an airing the guide has too
		}
		if _, err := m.DB.Exec(`INSERT INTO epg_programmes(channel, start, stop, title, sub) VALUES(?, ?, ?, 'the quiet orchard', ?)`, ch, start, start+1800, ch); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.DB.Exec(`INSERT INTO epg_programmes(channel, start, stop, title) VALUES('pine.test', ?, ?, 'Weather')`, at, at+60); err != nil {
		t.Fatal(err)
	}
	got, err := m.Airings(ctx, " The Quiet Orchard ", now, now.Add(14*24*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	// Harbour's backup, the tuner turned off and the channel turned off aren't in the guide; one airing seen twice is one.
	var where []int64
	for _, a := range got {
		where = append(where, a.ChannelID)
	}
	if len(got) != 3 || where[0] != 1 || where[1] != 3 || where[2] != 3 || got[0].Start > got[1].Start {
		t.Fatalf("%+v", got)
	}
	if got, _ := m.Airings(ctx, "The Quiet Orchard", now, now.Add(14*24*time.Hour), 1); len(got) != 1 {
		t.Fatalf("the limit: %d", len(got))
	}
	if got, _ := m.Airings(ctx, "", now, now.Add(time.Hour), 10); got != nil {
		t.Fatal("no title")
	}
}
