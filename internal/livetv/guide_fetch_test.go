// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A slow guide host holds nothing up. A transaction open across the fetch stopped every write on
// the server, sessions and progress among them, for as long as the host liked.
func TestSlowGuideHoldsNoLock(t *testing.T) {
	stop := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := w.(http.Flusher)
		fmt.Fprint(w, `<?xml version="1.0"?><tv>`)
		f.Flush()
		for {
			select {
			case <-stop:
				fmt.Fprint(w, `</tv>`)
				return
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
				fmt.Fprint(w, `<channel id="x"><display-name>X</display-name></channel>`)
				f.Flush()
			}
		}
	}))
	defer srv.Close()
	m := testManager(t)
	ctx := context.Background()
	s, err := m.AddSource(ctx, "Slow", srv.URL+"/g.xml")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- m.FetchSource(ctx, s.ID) }()
	time.Sleep(200 * time.Millisecond)
	at := time.Now()
	if _, err := m.DB.Exec(`UPDATE epg_sources SET name = 'Still slow' WHERE id = ?`, s.ID); err != nil {
		t.Errorf("a write failed while the guide was coming in: %v", err)
	}
	if d := time.Since(at); d > time.Second {
		t.Errorf("a write waited %v while the guide was coming in", d)
	}
	close(stop)
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("%v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the fetch never ended")
	}
}

// A host that stops sending is given up on, and leaves the guide as it was.
func TestStalledGuideIsGivenUpOn(t *testing.T) {
	old := guideIdle
	guideIdle = 200 * time.Millisecond
	defer func() { guideIdle = old }()
	var asked atomic.Int64
	release := make(chan struct{})
	defer close(release)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		fmt.Fprint(w, `<?xml version="1.0"?><tv><channel id="x"><display-name>X</display-name></channel>`)
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	m := testManager(t)
	ctx := context.Background()
	s, err := m.AddSource(ctx, "Stuck", srv.URL+"/g.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.DB.Exec(`INSERT INTO epg_channels(source_id, epg_id, name) VALUES(?, 'old', 'Old')`, s.ID); err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	if err := m.FetchSource(ctx, s.ID); err == nil {
		t.Error("a guide cut short counted as read")
	}
	if d := time.Since(at); d > 5*time.Second {
		t.Errorf("took %v", d)
	}
	ids, _ := m.GuideIDs(ctx)
	if !strings.Contains(fmt.Sprint(ids), "old") {
		t.Errorf("what was there is gone: %v", ids)
	}
}

// However much a guide lists, only so much is kept.
func TestGuideCaps(t *testing.T) {
	oldC, oldP := maxGuideChannels, maxGuideProgrammes
	maxGuideChannels, maxGuideProgrammes = 10, 25
	defer func() { maxGuideChannels, maxGuideProgrammes = oldC, oldP }()
	var c *Channel
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<?xml version="1.0"?><tv>`)
		for i := range 40 {
			fmt.Fprintf(w, `<channel id="c%d"><display-name>C%d</display-name></channel>`, i, i)
		}
		fmt.Fprintf(w, `<channel id="%s"><display-name>%s</display-name></channel>`, c.GuideKey(), c.Name)
		start := time.Now().Add(-time.Hour)
		for i := range 60 {
			s := start.Add(time.Duration(i) * time.Hour).Format("20060102150405 -0700")
			e := start.Add(time.Duration(i+1) * time.Hour).Format("20060102150405 -0700")
			fmt.Fprintf(w, `<programme start="%s" stop="%s" channel="%s"><title>Show %d</title></programme>`, s, e, c.GuideKey(), i)
		}
		fmt.Fprint(w, `</tv>`)
	}))
	defer srv.Close()
	m := testManager(t)
	ctx := context.Background()
	list := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXTINF:-1 tvg-id=\"keyed\",Keyed\nhttp://stream.example/a.ts\n")
	}))
	defer list.Close()
	tn, err := m.Create(ctx, &Tuner{Driver: "iptv", Name: "L", Options: map[string]string{"playlist": list.URL + "/l.m3u"}})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := m.Channels(ctx, tn.ID)
	if err != nil || len(ch) != 1 {
		t.Fatalf("%v %v", err, ch)
	}
	c = ch[0]
	s, err := m.AddSource(ctx, "Big", srv.URL+"/g.xml")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.FetchSource(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	var channels, programmes int
	_ = m.DB.QueryRow(`SELECT COUNT(*) FROM epg_channels WHERE source_id = ?`, s.ID).Scan(&channels)
	_ = m.DB.QueryRow(`SELECT COUNT(*) FROM epg_programmes`).Scan(&programmes)
	if channels > 10 || programmes > 25 || programmes == 0 {
		t.Errorf("%d channels, %d programmes", channels, programmes)
	}
}
