// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/dvd"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// standInAnalyzer puts a disc analysis of the test's own in place of WittyDVD's, with a
// module version in use, and puts both back after.
func standInAnalyzer(t *testing.T, version string, analyze func(path string) (*dvdsession.Analysis, error)) {
	t.Helper()
	prev, prevVersion := dvdsession.CurrentAnalyzer(), moduleVersion
	dvdsession.SetAnalyzer(analyze)
	moduleVersion = func(*Server, string) string { return version }
	t.Cleanup(func() {
		moduleVersion = prevVersion
		if prev == nil {
			prev = func(string) (*dvdsession.Analysis, error) { return nil, dvdsession.ErrNoEngine }
		}
		dvdsession.SetAnalyzer(prev)
	})
}

// rankedTwo is an analysis that makes title 2 the film.
func rankedTwo() *dvdsession.Analysis {
	return &dvdsession.Analysis{Engine: "0.4.0", Ranking: &dvd.Ranking{Main: 2, Confidence: 0.9, Level: "high",
		Why:        &dvd.Decision{Summary: "Title 2 is the film."},
		Candidates: []dvd.Candidate{{TitleNum: 2, Duration: 5400}, {TitleNum: 1, Duration: 300}}}}
}

func discItem(t *testing.T, ts *testServer) *library.Item {
	t.Helper()
	lib := ts.library(t, "discs", t.TempDir())
	path := filepath.Join(t.TempDir(), "Film.iso")
	if err := os.WriteFile(path, []byte("not read"), 0o644); err != nil {
		t.Fatal(err)
	}
	return ts.item(t, &library.Item{LibraryID: lib, Kind: "dvd-iso", Path: path, Title: "Film", Disc: &library.DiscInfo{Format: "dvd", MainTitle: 1,
		Titles: []library.DiscTitle{{Number: 1, DurationSec: 300}, {Number: 2, DurationSec: 5400}}}})
}

func TestAFailedRankingIsNotAskedForOnEveryVisit(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	_, viewer := ts.user(t, "viewer", "adult", false, "")
	var calls atomic.Int32
	fail := atomic.Bool{}
	fail.Store(true)
	standInAnalyzer(t, "0.4.0", func(string) (*dvdsession.Analysis, error) {
		calls.Add(1)
		if fail.Load() {
			return nil, errors.New("the helper stopped answering")
		}
		return rankedTwo(), nil
	})
	it := discItem(t, ts)
	u := fmt.Sprintf("/api/items/%d/disc/analysis", it.ID)
	for range 3 {
		if v := analysisOf(t, ts, viewer, it.ID); v.Ranked || v.RankError == "" || v.Main != 1 {
			t.Fatalf("after a failure: %+v", v)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("three visits asked the helper %d times, want once", n)
	}
	// An admin's "Rank again" asks at once, and a success is used from then on.
	fail.Store(false)
	if w := ts.do(t, "POST", u, admin, map[string]any{}); w.Code != 200 {
		t.Fatalf("ranking again: %d %s", w.Code, w.Body)
	}
	if v := analysisOf(t, ts, viewer, it.ID); !v.Ranked || v.Main != 2 || v.RankError != "" {
		t.Errorf("after ranking again: %+v", v)
	}
	// A new version of the module isn't held back by the old one's failure.
	fail.Store(true)
	ts.rankFailed(it, errors.New("old"))
	moduleVersion = func(*Server, string) string { return "0.4.1" }
	before := calls.Load()
	analysisOf(t, ts, viewer, it.ID)
	if calls.Load() != before+1 {
		t.Error("a new module version didn't rank the disc again")
	}
}

func TestLowPowerRanksInTheBackground(t *testing.T) {
	ts := newTestServer(t)
	_, viewer := ts.user(t, "viewer", "adult", false, "")
	release := make(chan struct{})
	standInAnalyzer(t, "0.4.0", func(string) (*dvdsession.Analysis, error) {
		<-release // the page mustn't wait for this
		return rankedTwo(), nil
	})
	if err := ts.DB.SetSetting(context.Background(), "lowPower", "1"); err != nil {
		t.Fatal(err)
	}
	it := discItem(t, ts)
	start := time.Now()
	v := analysisOf(t, ts, viewer, it.ID)
	if !v.Pending || v.Ranked || time.Since(start) > 5*time.Second {
		t.Fatalf("the first visit: %+v after %v, want pending at once", v, time.Since(start))
	}
	// Another visit while it works starts nothing more: one disc at a time.
	if v := analysisOf(t, ts, viewer, it.ID); !v.Pending {
		t.Errorf("the second visit: %+v", v)
	}
	close(release)
	deadline := time.Now().Add(10 * time.Second)
	for {
		got, err := ts.Store.Item(context.Background(), it.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Disc.Rank != nil {
			if got.Disc.MainTitle != 2 || got.RuntimeSec != 5400 {
				t.Errorf("kept main %d, %vs", got.Disc.MainTitle, got.RuntimeSec)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the background ranking never finished")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if v := analysisOf(t, ts, viewer, it.ID); v.Pending || !v.Ranked || v.Main != 2 {
		t.Errorf("once ranked: %+v", v)
	}
}
