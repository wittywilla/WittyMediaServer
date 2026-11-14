// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/dvd"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymodule/host"
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
	admin, _ := ts.user(t, "admin", "adult", true, "")
	viewer, _ := ts.user(t, "viewer", "adult", false, "")
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
	u := fmt.Sprintf("/discs/%d/rank", it.ID)
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
	if w := discHost(t, ts, admin, "POST", u, map[string]any{}); w.Code != 200 {
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
	viewer, _ := ts.user(t, "viewer", "adult", false, "")
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

// discHostModules is the disc modules, each holding its own secret, as the host API sees them.
type discHostModules struct{ fakeHostModules }

func (discHostModules) HostModule(t string) (string, bool) {
	switch t {
	case wittyDVDID, wittyBlurayID, wittyCDID:
		return t, true
	}
	return "", false
}
func (discHostModules) Libraries(string) []string { return []string{"discs", "movies", "music"} }

// discHostAs calls the host API as a module, on a person's behalf.
func discHostAs(t *testing.T, ts *Server, module string, u *auth.User, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	ts.hostModsForTest = discHostModules{}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, host.Base+path, rd)
	req.Header.Set(host.HeaderToken, module)
	if u != nil {
		req.Header.Set(host.HeaderOnUser, strconv.FormatInt(u.ID, 10))
	}
	w := httptest.NewRecorder()
	ts.HostAPI().ServeHTTP(w, req)
	return w
}

// discHost calls the host API as the module of the disc a path names (/discs/{id}...).
func discHost(t *testing.T, ts *testServer, u *auth.User, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var id int64
	fmt.Sscanf(strings.TrimPrefix(path, "/discs/"), "%d", &id)
	it, err := ts.Store.Item(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return discHostAs(t, ts.Server, discModuleOf(it), u, method, path, body)
}

// analysisView is what a disc module is told, with whether there is a ranking.
type analysisView struct {
	hostDisc
	Ranked bool
}

// analysisOf is what a disc module is told of a disc's ranking for a person, ranking it first
// when the kept ranking is stale.
func analysisOf(t *testing.T, ts *testServer, u *auth.User, id int64) analysisView {
	t.Helper()
	w := discHost(t, ts, u, "GET", fmt.Sprintf("/discs/%d?rank=1", id), nil)
	if w.Code != 200 {
		t.Fatalf("the analysis: %d %s", w.Code, w.Body)
	}
	var v analysisView
	if err := json.Unmarshal(w.Body.Bytes(), &v.hostDisc); err != nil {
		t.Fatal(err)
	}
	v.Ranked = v.Rank != nil
	return v
}

// discHostPage is the disc's page as its module is told it.
func discHostPage(t *testing.T, ts *testServer, u *auth.User, id int64) hostDiscPage {
	t.Helper()
	w := discHost(t, ts, u, "GET", fmt.Sprintf("/discs/%d?page=1", id), nil)
	var v hostDisc
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &v) != nil || v.Page == nil {
		t.Fatalf("the disc page: %d %s", w.Code, w.Body)
	}
	return *v.Page
}

// The host API's disc calls: only the disc's own module, for a person who may see it; choosing
// the film is an administrator's; nothing a module sends is a title the disc hasn't.
func TestHostDiscCalls(t *testing.T) {
	ts := newTestServer(t)
	admin, _ := ts.user(t, "admin", "adult", true, "")
	viewer, _ := ts.user(t, "viewer", "adult", false, "")
	kid, _ := ts.user(t, "kid", "kids", false, `{"maxAge":7,"allowUnrated":false}`)
	standInAnalyzer(t, "0.5.0", func(string) (*dvdsession.Analysis, error) { return rankedTwo(), nil })
	it := discItem(t, ts)
	path := fmt.Sprintf("/discs/%d", it.ID)
	if w := discHostAs(t, ts.Server, wittyBlurayID, viewer, "GET", path, nil); w.Code != 404 {
		t.Errorf("WittyBluray asked about a DVD: %d", w.Code)
	}
	if w := discHostAs(t, ts.Server, wittyDVDID, nil, "GET", path, nil); w.Code != 403 {
		t.Errorf("asked on nobody's behalf: %d", w.Code)
	}
	if w := discHostAs(t, ts.Server, wittyDVDID, kid, "GET", path, nil); w.Code != 404 {
		t.Errorf("a profile that may not see it: %d", w.Code)
	}
	if v := analysisOf(t, ts, viewer, it.ID); !v.Ranked || v.Main != 2 || v.EngineMain != 2 || v.Kind != "dvd" || v.Rank.Feature != nil && v.Rank.Feature.Segments != nil {
		t.Errorf("the viewer is told %+v", v)
	}
	if w := discHost(t, ts, viewer, "PUT", path+"/choice", map[string]any{"ttn": 1}); w.Code != 403 {
		t.Errorf("a viewer chose the film: %d", w.Code)
	}
	if w := discHost(t, ts, admin, "PUT", path+"/choice", map[string]any{"ttn": 9}); w.Code != 400 {
		t.Errorf("a title the disc hasn't: %d", w.Code)
	}
	if w := discHost(t, ts, admin, "PUT", path+"/choice", map[string]any{"ttn": 1}); w.Code != 200 {
		t.Fatalf("choosing: %d %s", w.Code, w.Body)
	}
	if v := analysisOf(t, ts, admin, it.ID); v.Main != 1 || v.EngineMain != 2 || v.Chosen == nil || v.ChosenBy != "admin" {
		t.Errorf("after the choice: %+v", v)
	}
	if v := analysisOf(t, ts, viewer, it.ID); v.ChosenBy != "" {
		t.Errorf("a viewer is told who chose: %q", v.ChosenBy)
	}
	if w := discHost(t, ts, admin, "PUT", path+"/choice", map[string]any{"ttn": -1}); w.Code != 200 {
		t.Fatalf("clearing: %d %s", w.Code, w.Body)
	}
	if v := analysisOf(t, ts, viewer, it.ID); v.Main != 2 || v.Chosen != nil {
		t.Errorf("after clearing: %+v", v)
	}
	if w := discHostAs(t, ts.Server, wittyDVDID, admin, "GET", "/discs/libbluray", nil); w.Code != 403 {
		t.Errorf("WittyDVD asked about libbluray: %d", w.Code)
	}
	if w := discHostAs(t, ts.Server, wittyDVDID, admin, "GET", fmt.Sprintf("/cds/%d/matches", it.ID), nil); w.Code != 403 {
		t.Errorf("WittyDVD asked about a CD: %d", w.Code)
	}
}
