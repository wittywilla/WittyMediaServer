// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/bluray"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/dvd"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
)

func TestBlurayRankEpisodesOfAFilmMadeOfThem(t *testing.T) {
	// No playlist per episode and no play-all playlist: the engine split the film itself.
	a := &bdsession.Analysis{Engine: "0.4.0", Ranked: &bluray.Ranked{Ranking: &bluray.Ranking{Main: 3, PlayAll: -1,
		Candidates:   []bluray.Candidate{{PlaylistNum: 3, Duration: 6650, Class: "main"}},
		PlayAllParts: []bluray.Part{{Index: 1, FirstItem: 0, Duration: 1300}, {Index: 2, FirstItem: 1, Start: 1300, Duration: 1350}},
	}}}
	r := BlurayRank(a)
	if len(r.Episodes) != 2 || r.Episodes[1].TTN != 3 || !r.Episodes[1].Part || r.Episodes[1].Item != 1 || r.Episodes[1].StartSec != 1300 {
		t.Errorf("episodes %+v", r.Episodes)
	}
	// With a play-all playlist, its parts are of that playlist.
	a.Ranked.Ranking.PlayAll = 9
	if r := BlurayRank(a); r.Episodes[0].TTN != 9 || r.PlayAll != 9 {
		t.Errorf("play-all episodes %+v", r.Episodes)
	}
	// Episodes with playlists of their own come first.
	a.Ranked.Ranking.Episodes = []bluray.RankEpisode{{Index: 1, Playlist: 20, Duration: 1300}}
	if r := BlurayRank(a); len(r.Episodes) != 1 || r.Episodes[0].Part {
		t.Errorf("own-playlist episodes %+v", r.Episodes)
	}
	if BlurayRank(&bdsession.Analysis{}) != nil {
		t.Error("a ranking from an engine without one")
	}
}

func TestDVDRankKeepsWhatAPageShows(t *testing.T) {
	a := &dvdsession.Analysis{Engine: "0.4.0", Stats: dvd.ReadStats{BadSectors: 4},
		Repairs: []dvd.Repair{{VTS: 2, Fixed: true, Note: "chapter table from the backup"}, {Note: ""}},
		Ranking: &dvd.Ranking{Main: 7, Confidence: 0.8, Decoys: []int{1, 2},
			Why:        &dvd.Decision{Summary: "Title 7 is the film.", Level: "high", RunnerUp: 3, Because: []dvd.Evidence{{Detail: "the menu's Play button starts it"}}},
			Candidates: []dvd.Candidate{{TitleNum: 7, Duration: 7200}, {TitleNum: 1, Flags: []string{"decoy"}}, {TitleNum: 3, Duration: 600}},
			Feature:    &dvd.Feature{Titles: []int{7, 3, 9}, Duration: 7200, Source: "chain"},
			Episodes:   []dvd.RankEpisode{{Index: 1, Title: 5, Source: "titles"}, {Index: 2, Title: 6, Source: "chapters", FirstChapter: 4, StartSec: 1200}},
		}}
	r := DVDRank(a)
	switch {
	case r.Main != 7 || r.Level != "high" || r.Why == "" || r.RunnerUp != 3 || len(r.Reasons) != 1:
		t.Errorf("the pick: %+v", r)
	case len(r.Candidates) != 2 || r.Decoys != 2:
		t.Errorf("decoys listed: %+v", r.Candidates)
	case r.Feature == nil || !r.Feature.Assembled:
		t.Errorf("feature %+v", r.Feature)
	case r.Episodes[0].Part || !r.Episodes[1].Part || r.Episodes[1].Chapter != 4:
		t.Errorf("episodes %+v", r.Episodes)
	case r.RepairCount != 2 || len(r.Repairs) != 1 || r.PaddedSectors != 4:
		t.Errorf("repairs %v (%d), padded %d", r.Repairs, r.RepairCount, r.PaddedSectors)
	}
}

func TestAChosenFilmBeatsTheEngineUntilCleared(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s := &Store{DB: d}
	ctx := context.Background()
	lib, err := s.CreateLibrary(ctx, "Discs", "discs", []string{"/discs"})
	if err != nil {
		t.Fatal(err)
	}
	it := &Item{LibraryID: lib.ID, Kind: "dvd", Title: "disc", SortTitle: "disc", Path: "/discs/a", Disc: &DiscInfo{Format: "dvd", MainTitle: 1,
		Titles: []DiscTitle{{Number: 1, DurationSec: 5000, Audio: []string{"en ac3 6ch"}}, {Number: 2, DurationSec: 600, Audio: []string{"fr ac3 2ch"}}}}}
	if err := s.UpdateItem(ctx, it); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDiscRank(ctx, it, &DiscRank{Main: 1, Engine: "wittydvd 0.4.0"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ChooseDiscMain(ctx, it, 9, 1); err == nil {
		t.Error("a title the disc hasn't was chosen")
	}
	if err := s.ChooseDiscMain(ctx, it, 2, 1); err != nil {
		t.Fatal(err)
	}
	got, err := s.Item(ctx, it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Disc.MainTitle != 2 || got.Disc.EngineMain() != 1 || got.RuntimeSec != 600 || len(got.AudioLangs) != 1 || got.AudioLangs[0] != "fr" {
		t.Errorf("after choosing: main %d engine %d, %vs %v", got.Disc.MainTitle, got.Disc.EngineMain(), got.RuntimeSec, got.AudioLangs)
	}
	// A new ranking doesn't move a choice.
	if err := s.SetDiscRank(ctx, got, &DiscRank{Main: 1, Engine: "wittydvd 0.4.1"}); err != nil || got.Disc.MainTitle != 2 {
		t.Errorf("a ranking moved the choice: %d %v", got.Disc.MainTitle, err)
	}
	if err := s.ChooseDiscMain(ctx, got, -1, 1); err != nil {
		t.Fatal(err)
	}
	if got.Disc.MainTitle != 1 || got.Disc.Chosen != nil || got.RuntimeSec != 5000 {
		t.Errorf("after clearing: main %d, %vs", got.Disc.MainTitle, got.RuntimeSec)
	}
}
