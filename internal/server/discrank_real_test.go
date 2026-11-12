// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
)

// What the server does with the disc modules' rankings, with the real modules: the
// synthetic discs WittyDVD and WittyBluray write out for other projects' tests, each with
// its right answer, scanned into a library and played through the API. The server has none
// of the modules' code, so the discs come from the test catalogue
// (scripts/make-test-catalog.sh writes them to <catalogue>/synth/<module>), and the test is
// skipped without it.

type synthDVD struct {
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	Main          int       `json:"main"`
	Feature       []int     `json:"feature"`
	FeatureSec    float64   `json:"featureSec"`
	Chapters      int       `json:"chapters"`
	Episodes      []int     `json:"episodes"`
	EpisodeStarts []float64 `json:"episodeStartsSec"`
	PlayAll       int       `json:"playAll"`
	MinDecoys     int       `json:"minDecoys"`
	Versions      []string  `json:"versions"`
}

type synthBD struct {
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	Main          int       `json:"main"`
	FeatureSec    float64   `json:"featureSec"`
	Chapters      int       `json:"chapters"`
	Editions      []int     `json:"editions"`
	Episodes      []int     `json:"episodes"`
	PlayAll       int       `json:"playAll"`
	PlayAllStarts []float64 `json:"playAllStartsSec"`
	Decoys        int       `json:"decoys"`
}

// synthList reads a module's list of synthetic discs, or skips.
func synthList(t *testing.T, module string, into any) string {
	t.Helper()
	dir := filepath.Join(os.Getenv(modtest.EnvCatalog), "synth", module)
	b, err := os.ReadFile(filepath.Join(dir, "expected.json"))
	if err != nil {
		t.Skipf("no synthetic discs from %s in the test catalogue: %v", module, err)
	}
	if err := json.Unmarshal(b, into); err != nil {
		t.Fatal(err)
	}
	return dir
}

// scanDiscs makes a discs library of a folder, scans it and returns its items by path.
func scanDiscs(t *testing.T, ts *testServer, dir string) (map[string]*library.Item, int64) {
	t.Helper()
	no := func() bool { return false }
	ts.Scanner.SaveMeta, ts.Scanner.WikidataOn, ts.Scanner.TVMazeOn, ts.Scanner.MusicBrainzOn = no, no, no, no
	id := ts.library(t, "discs", dir)
	return scanAgain(t, ts, id), id
}

// scanAgain scans a library and returns its items by path.
func scanAgain(t *testing.T, ts *testServer, id int64) map[string]*library.Item {
	t.Helper()
	ctx := context.Background()
	if !ts.Scanner.Start(id) {
		t.Fatal("the scan didn't start")
	}
	deadline := time.Now().Add(5 * time.Minute)
	for ts.Scanner.Running(id) {
		if time.Now().After(deadline) {
			t.Fatal("the scan took too long")
		}
		time.Sleep(50 * time.Millisecond)
	}
	items, _, err := ts.Store.Items(ctx, library.ItemQuery{LibraryID: id, Limit: 500, NoTotal: true})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*library.Item{}
	for _, it := range items {
		full, err := ts.Store.Item(ctx, it.ID)
		if err != nil {
			t.Fatal(err)
		}
		out[full.Path] = full
	}
	return out
}

func near(a, b, within float64) bool { return math.Abs(a-b) <= within }

// openDisc opens a disc session through the API and returns what it shows.
func openDisc(t *testing.T, ts *testServer, tok string, id int64, body map[string]any) (string, *struct {
	Kind   string
	Reason string
	Title  *struct {
		TTN         int       `json:"ttn"`
		DurationSec float64   `json:"durationSec"`
		StartSec    float64   `json:"startSec"`
		Chapters    []float64 `json:"chapters"`
	}
}) {
	t.Helper()
	w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/dvd", id), tok, body)
	if w.Code != 200 {
		t.Fatalf("opening the disc: %d %s", w.Code, w.Body)
	}
	var got struct {
		SessionID    string `json:"sessionId"`
		Presentation *struct {
			Kind   string
			Reason string
			Title  *struct {
				TTN         int       `json:"ttn"`
				DurationSec float64   `json:"durationSec"`
				StartSec    float64   `json:"startSec"`
				Chapters    []float64 `json:"chapters"`
			}
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Presentation == nil {
		t.Fatalf("the disc's answer: %v %s", err, w.Body)
	}
	t.Cleanup(func() { ts.do(t, "DELETE", "/api/dvd/"+got.SessionID, tok, nil) })
	return got.SessionID, got.Presentation
}

func analysisOf(t *testing.T, ts *testServer, tok string, id int64) discAnalysisView {
	t.Helper()
	w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/disc/analysis", id), tok, nil)
	if w.Code != 200 {
		t.Fatalf("the analysis: %d %s", w.Code, w.Body)
	}
	var v discAnalysisView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestRankedDVDsWithTheRealModule(t *testing.T) {
	m := modtest.Need(t, "wittydvd")
	var list []synthDVD
	dir := synthList(t, "wittydvd", &list)
	ts := newTestServer(t)
	ts.Modules = m
	_, admin := ts.user(t, "admin", "adult", true, "")
	_, viewer := ts.user(t, "viewer", "adult", false, "")
	items, lib := scanDiscs(t, ts, dir)
	for _, want := range list {
		t.Run(want.Name, func(t *testing.T) {
			it := items[filepath.Join(dir, want.Path)]
			if it == nil || it.Disc == nil {
				t.Fatalf("the scan found no disc at %s", want.Path)
			}
			d := it.Disc
			if d.Rank == nil || !strings.HasPrefix(d.Rank.Engine, "wittydvd ") {
				t.Fatalf("the scan kept no ranking: %+v", d.Rank)
			}
			if d.MainTitle != want.Main || d.Rank.Main != want.Main {
				t.Errorf("main title %d (ranked %d), want %d", d.MainTitle, d.Rank.Main, want.Main)
			}
			if !(d.Rank.Confidence > 0 && d.Rank.Confidence <= 1) || d.Rank.Why == "" {
				t.Errorf("no reason or confidence: %q %v", d.Rank.Why, d.Rank.Confidence)
			}
			if d.Rank.Decoys < want.MinDecoys {
				t.Errorf("%d decoys, want at least %d", d.Rank.Decoys, want.MinDecoys)
			}
			if want.FeatureSec > 0 && !near(it.RuntimeSec, want.FeatureSec, 2) {
				t.Errorf("the item is %.0fs long, want %.0fs", it.RuntimeSec, want.FeatureSec)
			}
			if len(want.Feature) > 1 {
				f := d.Rank.Feature
				if f == nil || !slices.Equal(f.Titles, want.Feature) || !f.Assembled || len(f.Segments) == 0 {
					t.Fatalf("the feature kept is %+v, want titles %v laid out to play", f, want.Feature)
				}
			}
			if len(want.Episodes) > 0 && len(d.Rank.Episodes) != len(want.Episodes) {
				t.Errorf("%d episodes, want %d", len(d.Rank.Episodes), len(want.Episodes))
			}
			if len(want.Versions) > 0 && len(d.Rank.Versions) == 0 {
				t.Errorf("no versions, want %v", want.Versions)
			}

			// Anyone who may see the disc sees why; only an admin may choose.
			v := analysisOf(t, ts, viewer, it.ID)
			if !v.Ranked || v.Main != want.Main || v.EngineMain != want.Main || v.CanChoose || v.Kind != "dvd" {
				t.Errorf("the viewer's analysis: %+v", v)
			}
			if a := analysisOf(t, ts, admin, it.ID); !a.CanChoose {
				t.Error("an admin can't choose")
			}

			// Play plays the film: the whole of it, through every title it runs through.
			_, p := openDisc(t, ts, viewer, it.ID, map[string]any{"mode": "title"})
			if p.Kind != "title" || p.Title == nil {
				t.Fatalf("Play shows %q (%s)", p.Kind, p.Reason)
			}
			if p.Title.TTN != want.Main {
				t.Errorf("Play plays title %d, want %d", p.Title.TTN, want.Main)
			}
			if want.FeatureSec > 0 && !near(p.Title.DurationSec, want.FeatureSec, 2) {
				t.Errorf("Play plays %.0fs, want %.0fs", p.Title.DurationSec, want.FeatureSec)
			}
			if want.Chapters > 0 && len(p.Title.Chapters) != want.Chapters {
				t.Errorf("Play has %d chapters, want %d", len(p.Title.Chapters), want.Chapters)
			}

			// An episode of a title that plays them all starts where it starts.
			if len(want.EpisodeStarts) > 2 && len(d.Rank.Episodes) == len(want.EpisodeStarts) {
				_, p := openDisc(t, ts, viewer, it.ID, map[string]any{"mode": "title", "episode": 3})
				if p.Title == nil || !near(p.Title.StartSec, want.EpisodeStarts[2], 2) {
					t.Errorf("episode 3 starts at %+v, want %.0fs", p.Title, want.EpisodeStarts[2])
				}
			}
			if w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/dvd", it.ID), viewer, map[string]any{"mode": "title", "episode": 99}); w.Code != 400 {
				t.Errorf("an episode the disc hasn't: %d", w.Code)
			}
		})
	}

	// A remembered choice beats the engine, for everyone, and outlives a rescan.
	var pick synthDVD
	for _, x := range list {
		if x.Path == "simple" {
			pick = x
		}
	}
	it := items[filepath.Join(dir, "simple")]
	if it == nil {
		t.Fatal("no simple disc")
	}
	other := -1
	for _, ti := range it.Disc.Titles {
		if ti.Number != pick.Main {
			other = ti.Number
			break
		}
	}
	if other < 0 {
		t.Fatal("the simple disc has one title")
	}
	if w := ts.do(t, "PUT", fmt.Sprintf("/api/items/%d/disc/choice", it.ID), viewer, map[string]any{"ttn": other}); w.Code != 403 {
		t.Errorf("a viewer chose the film: %d", w.Code)
	}
	if w := ts.do(t, "PUT", fmt.Sprintf("/api/items/%d/disc/choice", it.ID), admin, map[string]any{"ttn": 77}); w.Code != 400 {
		t.Errorf("a title the disc hasn't was chosen: %d", w.Code)
	}
	if w := ts.do(t, "PUT", fmt.Sprintf("/api/items/%d/disc/choice", it.ID), admin, map[string]any{"ttn": other}); w.Code != 200 {
		t.Fatalf("choosing: %d %s", w.Code, w.Body)
	}
	v := analysisOf(t, ts, viewer, it.ID)
	if v.Main != other || v.EngineMain != pick.Main || v.Chosen == nil || v.ChosenBy != "" {
		t.Errorf("after the choice the viewer sees %+v", v)
	}
	if a := analysisOf(t, ts, admin, it.ID); a.ChosenBy != "admin" {
		t.Errorf("the admin isn't told who chose: %q", a.ChosenBy)
	}
	if _, p := openDisc(t, ts, viewer, it.ID, map[string]any{"mode": "title"}); p.Title == nil || p.Title.TTN != other {
		t.Errorf("Play doesn't play the chosen title: %+v", p.Title)
	}
	again := scanAgain(t, ts, lib)[it.Path]
	if again == nil || again.Disc == nil {
		t.Fatal("the disc is gone after a scan")
	}
	if again.Disc.MainTitle != other || again.Disc.Chosen == nil {
		t.Errorf("the choice is gone after a scan: main %d", again.Disc.MainTitle)
	}
	if w := ts.do(t, "DELETE", fmt.Sprintf("/api/items/%d/disc/choice", it.ID), admin, nil); w.Code != 200 {
		t.Fatalf("clearing: %d %s", w.Code, w.Body)
	}
	if v := analysisOf(t, ts, viewer, it.ID); v.Main != pick.Main || v.Chosen != nil {
		t.Errorf("after clearing: %+v", v)
	}
}

func TestRankedBlurayWithTheRealModule(t *testing.T) {
	m := modtest.Need(t, "wittybluray")
	var list []synthBD
	dir := synthList(t, "wittybluray", &list)
	ts := newTestServer(t)
	ts.Modules = m
	_, admin := ts.user(t, "admin", "adult", true, "")
	_, viewer := ts.user(t, "viewer", "adult", false, "")
	items, _ := scanDiscs(t, ts, dir)
	for _, want := range list {
		t.Run(want.Name, func(t *testing.T) {
			it := items[filepath.Join(dir, want.Path)]
			if it == nil || it.Disc == nil {
				t.Fatalf("the scan found no disc at %s", want.Path)
			}
			d := it.Disc
			if d.Rank == nil || !strings.HasPrefix(d.Rank.Engine, "wittybluray ") {
				t.Fatalf("the scan kept no ranking: %+v", d.Rank)
			}
			if d.MainTitle != want.Main || d.Rank.Main != want.Main {
				t.Errorf("main playlist %d (ranked %d), want %d", d.MainTitle, d.Rank.Main, want.Main)
			}
			if d.Rank.Why == "" || !(d.Rank.Confidence > 0) {
				t.Errorf("no reason or confidence: %q %v", d.Rank.Why, d.Rank.Confidence)
			}
			if want.FeatureSec > 0 && !near(it.RuntimeSec, want.FeatureSec, 2) {
				t.Errorf("the item is %.0fs long, want %.0fs", it.RuntimeSec, want.FeatureSec)
			}
			if len(want.Editions) > 1 {
				var got []int
				for _, v := range d.Rank.Versions {
					if v.Kind == "edition" {
						got = append(got, v.TTN)
					}
				}
				if !slices.Equal(got, want.Editions) {
					t.Errorf("editions %v, want %v", got, want.Editions)
				}
			}
			if len(want.Episodes) > 0 {
				var got []int
				for _, e := range d.Rank.Episodes {
					got = append(got, e.TTN)
				}
				if !slices.Equal(got, want.Episodes) {
					t.Errorf("episodes %v, want %v", got, want.Episodes)
				}
			}
			if want.PlayAll >= 0 && d.Rank.PlayAll != want.PlayAll {
				t.Errorf("play-all %d, want %d", d.Rank.PlayAll, want.PlayAll)
			}

			// The disc page lists the film with the engine's chapters, and not the decoys.
			w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/disc", it.ID), viewer, nil)
			var page struct {
				Main *struct {
					TTN      int `json:"ttn"`
					Chapters []struct{ N int }
				} `json:"main"`
				Extras []struct{ TTN int } `json:"extras"`
				Rank   map[string]any      `json:"rank"`
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Main == nil {
				t.Fatalf("the disc page: %d %s", w.Code, w.Body)
			}
			if page.Main.TTN != want.Main || (want.Chapters > 0 && len(page.Main.Chapters) != want.Chapters) {
				t.Errorf("the page's film is %d with %d chapters, want %d with %d", page.Main.TTN, len(page.Main.Chapters), want.Main, want.Chapters)
			}
			if want.Decoys > 0 && len(page.Extras) > 50 {
				t.Errorf("the page lists %d extras on a disc of %d decoys", len(page.Extras), want.Decoys)
			}
			if page.Rank == nil || page.Rank["why"] == "" {
				t.Errorf("the page doesn't say why: %v", page.Rank)
			}

			_, p := openDisc(t, ts, viewer, it.ID, map[string]any{"mode": "title"})
			if p.Kind != "title" || p.Title == nil || p.Title.TTN != want.Main {
				t.Fatalf("Play shows %q %+v (%s), want playlist %d", p.Kind, p.Title, p.Reason, want.Main)
			}
			if want.FeatureSec > 0 && !near(p.Title.DurationSec, want.FeatureSec, 2) {
				t.Errorf("Play plays %.0fs, want %.0fs", p.Title.DurationSec, want.FeatureSec)
			}
			// A play-all playlist's parts are episodes, each played from its own start.
			if len(want.PlayAllStarts) > 2 && len(want.Episodes) == 0 {
				_, p := openDisc(t, ts, viewer, it.ID, map[string]any{"mode": "title", "episode": 2})
				whole := want.PlayAll
				if whole < 0 {
					whole = want.Main
				}
				if p.Title == nil || p.Title.TTN != whole || !near(p.Title.StartSec, want.PlayAllStarts[1], 2) {
					t.Errorf("episode 2 plays %+v, want playlist %d from %.0fs", p.Title, whole, want.PlayAllStarts[1])
				}
			}
		})
	}
	// A second opinion is the owner's to turn on, and needs a libbluray.
	for _, it := range items {
		if it.Disc == nil {
			continue
		}
		u := fmt.Sprintf("/api/items/%d/disc/analysis", it.ID)
		if w := ts.do(t, "POST", u, viewer, map[string]any{"secondOpinion": true}); w.Code != 403 {
			t.Errorf("a viewer asked for a second opinion: %d", w.Code)
		}
		if w := ts.do(t, "POST", u, admin, map[string]any{"secondOpinion": true}); w.Code != 403 {
			t.Errorf("a second opinion with the setting off: %d %s", w.Code, w.Body)
		}
		if w := ts.do(t, "POST", u, admin, map[string]any{}); w.Code != 200 {
			t.Errorf("ranking again: %d %s", w.Code, w.Body)
		}
		w := ts.do(t, "GET", "/api/bluray/capabilities", admin, nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"libbluray"`) {
			t.Errorf("capabilities: %d %s", w.Code, w.Body)
		}
		break
	}
}
