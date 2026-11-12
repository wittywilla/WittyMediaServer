// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/bluray"
	"github.com/wittywilla/wittymediaserver/internal/dvd"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
)

// What the disc engines' rankings say about a disc, in the one shape the server keeps and
// shows for both kinds: which title is the film and why, with how sure the engine is; what
// exactly plays for it; other versions, a TV disc's episodes, and the other candidates;
// and what was wrong with the copy. "ttn" is a DVD's title number or a Blu-ray's playlist
// number, as everywhere else. It is kept with the item (disc_json), so nothing has to ask
// a helper again to show it, and it is short: lists are cut to what a page shows.

// How much of a ranking is kept.
const (
	rankCandidates = 16
	rankReasons    = 6
	rankSegments   = 128
	rankEpisodes   = 99
	rankNotes      = 12
)

// DiscRank is the engine's ranking of a disc, as kept and shown.
type DiscRank struct {
	// Engine is the module and version that gave it, such as "wittydvd 0.4.0".
	Engine string `json:"engine"`
	// Main is the engine's pick, or -1 when it found nothing that could be the film.
	Main       int      `json:"main"`
	Confidence float64  `json:"confidence"` // 0 to 1
	Level      string   `json:"level"`      // high, medium or low
	Why        string   `json:"why"`        // one sentence
	Reasons    []string `json:"reasons"`
	Against    []string `json:"against,omitempty"`
	// RunnerUp is the next best candidate, or -1.
	RunnerUp int `json:"runnerUp"`
	// Feature is what plays for the film on a DVD, when that is more than one title's
	// chapter table says: several titles in a row, or chains only commands join.
	Feature  *DiscFeature  `json:"feature,omitempty"`
	Versions []DiscVersion `json:"versions,omitempty"`
	Episodes []DiscEpisode `json:"episodes,omitempty"`
	// PlayAll is the title that plays every episode, or -1.
	PlayAll    int             `json:"playAll"`
	Candidates []DiscCandidate `json:"candidates"`
	// Decoys counts the titles the engine found to be decoys, and Hidden the candidates
	// that aren't listed here.
	Decoys   int      `json:"decoys"`
	Hidden   int      `json:"hidden"`
	Warnings []string `json:"warnings,omitempty"`
	// Repairs are what the engine mended or read from a backup copy, and Issues where the
	// copy's structures still disagree. The counts are of all of them.
	Repairs     []string    `json:"repairs,omitempty"`
	RepairCount int         `json:"repairCount"`
	Issues      []DiscIssue `json:"issues,omitempty"`
	IssueCount  int         `json:"issueCount"`
	// PaddedSectors are sectors that couldn't be read while ranking and were taken as
	// zeros; ReadRetries how many reads were tried again.
	PaddedSectors int64 `json:"paddedSectors"`
	ReadRetries   int64 `json:"readRetries"`
	// Protected: the video is encrypted. The structure was still read; nothing plays.
	Protected     bool               `json:"protected,omitempty"`
	SecondOpinion *DiscSecondOpinion `json:"secondOpinion,omitempty"`
	At            int64              `json:"at"`
}

// DiscFeature is what plays for the film on a DVD.
type DiscFeature struct {
	Titles      []int   `json:"titles"`
	DurationSec float64 `json:"durationSec"`
	Source      string  `json:"source"` // chapters, commands or chain
	// Assembled: more than the main title's own chapter table. It then plays as one
	// stream in the engine's order.
	Assembled bool                 `json:"assembled"`
	Segments  []dvd.FeatureSegment `json:"segments,omitempty"`
}

// Engine is the feature in the form the engine gave it, for laying it out to play.
func (f *DiscFeature) Engine() *dvd.Feature {
	return &dvd.Feature{Titles: f.Titles, Duration: f.DurationSec, Source: f.Source, Segments: f.Segments}
}

// DiscVersion is another way to see the film.
type DiscVersion struct {
	Kind        string  `json:"kind"` // angle, parental, cut or edition
	TTN         int     `json:"ttn"`
	Angle       int     `json:"angle,omitempty"`
	Levels      []int   `json:"parentalLevels,omitempty"`
	DurationSec float64 `json:"durationSec"`
	Label       string  `json:"label"`
	Main        bool    `json:"main,omitempty"`
}

// DiscEpisode is one episode of a TV disc, in viewing order. One that has no title of its
// own is a stretch of the play-all title: Part is then true, and it starts at StartSec, at
// Chapter (DVD) or play item Item (Blu-ray) of that title.
type DiscEpisode struct {
	Index       int     `json:"index"`
	TTN         int     `json:"ttn"`
	DurationSec float64 `json:"durationSec"`
	StartSec    float64 `json:"startSec"`
	Chapter     int     `json:"chapter,omitempty"`
	Item        int     `json:"item,omitempty"`
	Part        bool    `json:"part,omitempty"`
}

// DiscCandidate is one title as the engine scored it.
type DiscCandidate struct {
	TTN         int      `json:"ttn"`
	DurationSec float64  `json:"durationSec"`
	Chapters    int      `json:"chapters"`
	Class       string   `json:"class"`
	Score       float64  `json:"score"`
	Decoy       bool     `json:"decoy,omitempty"`
	Reasons     []string `json:"reasons,omitempty"`
}

// DiscIssue is one place where a copy's structures disagree.
type DiscIssue struct {
	Severity string `json:"severity"` // error, warn or info
	Kind     string `json:"kind"`
	TTN      int    `json:"ttn"` // -1 when it is about no one title
	What     string `json:"what"`
}

// DiscSecondOpinion is how a libbluray on this machine read the same disc.
type DiscSecondOpinion struct {
	Source      string   `json:"source"`
	Agrees      bool     `json:"agrees"`
	Main        int      `json:"main"`
	Differences []string `json:"differences,omitempty"`
	More        int      `json:"more,omitempty"`
	// Note says why none was taken, when one was asked for.
	Note string `json:"note,omitempty"`
}

// DiscChoice is the title someone with the right to chose as this disc's film. It beats
// the engine's pick everywhere, until it is cleared or the disc no longer has that title.
type DiscChoice struct {
	TTN int   `json:"ttn"`
	By  int64 `json:"by"` // the account that chose
	At  int64 `json:"at"`
	// Was is the main title before the choice, for when it is cleared and no ranking is
	// kept to go back to.
	Was int `json:"was"`
}

func firstTexts(list []string, n int) []string {
	out := []string{}
	for _, s := range list {
		if s = strings.TrimSpace(s); s != "" && len(out) < n {
			out = append(out, s)
		}
	}
	return out
}

func details(list []dvd.Evidence, n int) []string {
	var out []string
	for _, e := range list {
		if d := strings.TrimSpace(e.Detail); d != "" && len(out) < n {
			out = append(out, d)
		}
	}
	return out
}

// DVDRank puts a DVD's analysis in the kept form. nil when there is no ranking.
func DVDRank(a *dvdsession.Analysis) *DiscRank {
	if a == nil || a.Ranking == nil {
		return nil
	}
	r := a.Ranking
	out := &DiscRank{Engine: "wittydvd " + a.Engine, Main: r.Main, Confidence: r.Confidence, Level: r.Level, RunnerUp: -1, PlayAll: -1,
		Reasons: []string{}, Candidates: []DiscCandidate{}, Decoys: len(r.Decoys), Warnings: firstTexts(r.Warnings, rankNotes),
		PaddedSectors: a.Stats.BadSectors, ReadRetries: a.Stats.Retries, Protected: a.Protected, At: time.Now().Unix()}
	if r.Main == 0 {
		out.Main = -1
	}
	if r.PlayAll != 0 {
		out.PlayAll = r.PlayAll
	}
	if w := r.Why; w != nil {
		out.Why = w.Summary
		out.Reasons = append(out.Reasons, details(w.Because, rankReasons)...)
		out.Against = details(w.Against, rankReasons)
		if w.RunnerUp != 0 {
			out.RunnerUp = w.RunnerUp
		}
		if out.Level == "" {
			out.Level = w.Level
		}
	}
	decoy := map[int]bool{}
	for _, n := range r.Decoys {
		decoy[n] = true
	}
	// The candidates a person might want instead: the best that aren't decoys.
	for _, c := range r.Candidates {
		if decoy[c.TitleNum] || c.Has("decoy") {
			continue
		}
		if len(out.Candidates) >= rankCandidates {
			out.Hidden++
			continue
		}
		out.Candidates = append(out.Candidates, DiscCandidate{TTN: c.TitleNum, DurationSec: c.Duration, Chapters: c.Chapters, Class: c.Class, Score: c.Score, Reasons: firstTexts(c.Reasons, 3)})
	}
	if f := r.Feature; f != nil && len(f.Titles) > 0 {
		df := &DiscFeature{Titles: f.Titles, DurationSec: f.Duration, Source: f.Source, Assembled: len(f.Titles) > 1 || f.Source != "chapters"}
		if len(f.Segments) <= rankSegments {
			df.Segments = f.Segments
		}
		out.Feature = df
	}
	for _, v := range r.Versions {
		if len(out.Versions) >= rankCandidates {
			break
		}
		out.Versions = append(out.Versions, DiscVersion{Kind: v.Kind, TTN: v.Title, Angle: v.Angle, Levels: v.Levels, DurationSec: v.Duration, Label: v.Label, Main: v.Title == r.Main && v.Kind != "cut" && v.Angle <= 1 && len(v.Levels) == 0})
	}
	for _, e := range r.Episodes {
		if len(out.Episodes) >= rankEpisodes {
			break
		}
		part := e.Source != "titles" && e.Source != ""
		ep := DiscEpisode{Index: e.Index, TTN: e.Title, DurationSec: e.Duration, StartSec: e.StartSec, Part: part}
		if part {
			ep.Chapter = max(e.FirstChapter, 1)
		}
		out.Episodes = append(out.Episodes, ep)
	}
	for _, rep := range a.Repairs {
		out.RepairCount++
		if len(out.Repairs) < rankNotes && strings.TrimSpace(rep.Note) != "" {
			where := "the disc's index"
			if rep.VTS > 0 {
				where = fmt.Sprintf("title set %d", rep.VTS)
			}
			done := "noted"
			if rep.Fixed {
				done = "mended"
			}
			out.Repairs = append(out.Repairs, fmt.Sprintf("%s (%s, %s)", strings.TrimSpace(rep.Note), where, done))
		}
	}
	return out
}

// BlurayRank puts a Blu-ray's analysis in the kept form. nil when there is no ranking.
func BlurayRank(a *bdsession.Analysis) *DiscRank {
	if a == nil || a.Ranked == nil || a.Ranked.Ranking == nil {
		return nil
	}
	r := a.Ranked.Ranking
	out := &DiscRank{Engine: "wittybluray " + a.Engine, Main: r.Main, Confidence: r.Confidence, Level: r.ConfidenceLabel, RunnerUp: -1, PlayAll: r.PlayAll,
		Reasons: []string{}, Candidates: []DiscCandidate{}, Decoys: len(r.Decoys), Warnings: firstTexts(r.Warnings, rankNotes), Protected: a.Protected, At: time.Now().Unix()}
	if why := firstTexts(r.Why, rankReasons+1); len(why) > 0 {
		out.Why, out.Reasons = why[0], why[1:]
	}
	decoy := map[int]bool{}
	for _, n := range r.Decoys {
		decoy[n] = true
	}
	listed := 0
	for _, c := range r.Candidates {
		// A menu's background is nothing to choose as the film.
		if decoy[c.PlaylistNum] || c.Class == "decoy" || c.Class == "menu" {
			continue
		}
		listed++
		if len(out.Candidates) >= rankCandidates {
			continue
		}
		if c.PlaylistNum != r.Main && out.RunnerUp < 0 {
			out.RunnerUp = c.PlaylistNum
		}
		out.Candidates = append(out.Candidates, DiscCandidate{TTN: c.PlaylistNum, DurationSec: c.Duration, Chapters: c.Chapters, Class: c.Class, Score: c.Score, Reasons: firstTexts(c.Reasons, 3)})
	}
	// What the helper didn't send is hidden too: it sends its best.
	out.Hidden = max(listed-len(out.Candidates), 0) + max(a.Ranked.Total-len(r.Candidates)-max(len(r.Decoys)-countDecoys(r), 0), 0)
	for _, e := range r.Editions {
		if len(out.Versions) >= rankCandidates {
			break
		}
		out.Versions = append(out.Versions, DiscVersion{Kind: "edition", TTN: e.Playlist, DurationSec: e.Duration, Label: e.Label, Main: e.Main})
	}
	for _, e := range r.Episodes {
		if len(out.Episodes) < rankEpisodes {
			out.Episodes = append(out.Episodes, DiscEpisode{Index: e.Index, TTN: e.Playlist, DurationSec: e.Duration})
		}
	}
	// No playlist per episode: the parts of the playlist that plays them all are the
	// episodes. That playlist is the play-all one, or with none the film itself, when the
	// engine found it made of episode-length clips.
	whole := r.PlayAll
	if whole < 0 {
		whole = r.Main
	}
	if len(out.Episodes) == 0 && whole >= 0 {
		for _, p := range r.PlayAllParts {
			if len(out.Episodes) < rankEpisodes {
				out.Episodes = append(out.Episodes, DiscEpisode{Index: p.Index, TTN: whole, DurationSec: p.Duration, StartSec: p.Start, Item: p.FirstItem, Part: true})
			}
		}
	}
	if c := a.Consistency; c != nil {
		for _, name := range c.FromBackup {
			out.RepairCount++
			if len(out.Repairs) < rankNotes {
				out.Repairs = append(out.Repairs, name+" was read from the disc's backup copy")
			}
		}
		for _, n := range c.Notes {
			out.RepairCount++
			if len(out.Repairs) < rankNotes {
				out.Repairs = append(out.Repairs, n)
			}
		}
		// The worst first: what can't play, then what plays differently.
		for _, sev := range []string{"error", "warn"} {
			for _, x := range c.Issues {
				if x.Severity != sev {
					continue
				}
				out.IssueCount++
				if len(out.Issues) < rankNotes {
					out.Issues = append(out.Issues, DiscIssue{Severity: x.Severity, Kind: x.Kind, TTN: x.Playlist, What: x.What})
				}
			}
		}
	}
	if so := r.SecondOpinion; so != nil {
		d := &DiscSecondOpinion{Source: so.Source, Agrees: so.AgreesOnMain, Main: so.Main, More: so.More}
		for _, x := range so.Disagreements {
			if len(d.Differences) >= rankNotes {
				d.More++
				continue
			}
			where := "the disc"
			if x.Playlist >= 0 {
				where = fmt.Sprintf("playlist %05d", x.Playlist)
			}
			line := fmt.Sprintf("%s, %s: %s here, %s there", where, x.Field, x.Ours, x.Theirs)
			if x.Note != "" {
				line += " (" + x.Note + ")"
			}
			d.Differences = append(d.Differences, line)
		}
		out.SecondOpinion = d
	} else if a.SecondOpinion {
		note := a.Ranked.Note
		if note == "" {
			note = a.Ranked.Libbluray.Reason
		}
		out.SecondOpinion = &DiscSecondOpinion{Main: -1, Note: note}
	}
	return out
}

// countDecoys is how many of the candidates a helper sent are decoys.
func countDecoys(r *bluray.Ranking) int {
	n := 0
	for _, c := range r.Candidates {
		if c.Class == "decoy" {
			n++
		}
	}
	return n
}

// title finds a disc's title by number.
func (d *DiscInfo) title(ttn int) *DiscTitle {
	for i := range d.Titles {
		if d.Titles[i].Number == ttn {
			return &d.Titles[i]
		}
	}
	return nil
}

// HasTitle says whether the disc has a title of that number to play.
func (d *DiscInfo) HasTitle(ttn int) bool { return d != nil && d.title(ttn) != nil }

// EngineMain is the main title without anyone's choice: the engine's pick, or what the
// older rule found before a choice was made.
func (d *DiscInfo) EngineMain() int {
	switch {
	case d.Rank != nil && d.Rank.Main >= 0:
		return d.Rank.Main
	case d.Chosen != nil:
		return d.Chosen.Was
	}
	return d.MainTitle
}

// FeaturePlays is the assembled feature to play for a title, when that title is the
// engine's film and the film is more than the title's own chapter table.
func (d *DiscInfo) FeaturePlays(ttn int) *DiscFeature {
	if d == nil || d.Rank == nil || d.Rank.Feature == nil || d.Rank.Main != ttn {
		return nil
	}
	if f := d.Rank.Feature; f.Assembled && len(f.Segments) > 0 {
		return f
	}
	return nil
}

// applyMain makes ttn the disc's main title in what the item says about itself: how long
// it is and which languages it has. It reports whether the disc has such a title.
func applyMain(it *Item, ttn int) bool {
	d := it.Disc
	t := d.title(ttn)
	if t == nil {
		return false
	}
	d.MainTitle = ttn
	it.RuntimeSec = t.DurationSec
	if f := d.FeaturePlays(ttn); f != nil && f.DurationSec > 0 {
		it.RuntimeSec = f.DurationSec
	}
	langs := func(list []string) []string {
		out := make([]string, 0, len(list))
		for _, s := range list {
			if f := strings.Fields(s); len(f) > 0 {
				out = append(out, f[0])
			}
		}
		return out
	}
	it.AudioLangs, it.SubLangs = langs(t.Audio), langs(t.Subtitles)
	return true
}

// ChooseDiscMain remembers ttn as the disc's film, chosen by an account, or forgets the
// choice when ttn is negative. The item's own length and languages follow. It refuses a
// title the disc doesn't have.
func (s *Store) ChooseDiscMain(ctx context.Context, it *Item, ttn int, by int64) error {
	d := it.Disc
	if d == nil {
		return fmt.Errorf("this item is no disc")
	}
	if ttn < 0 {
		if d.Chosen == nil {
			return nil
		}
		back := d.EngineMain()
		d.Chosen = nil
		if !applyMain(it, back) {
			d.MainTitle = back
		}
		return s.saveDiscMain(ctx, it)
	}
	if !d.HasTitle(ttn) {
		return fmt.Errorf("the disc has no title %d", ttn)
	}
	was := d.MainTitle
	if d.Chosen != nil {
		was = d.Chosen.Was
	}
	d.Chosen = &DiscChoice{TTN: ttn, By: by, At: time.Now().Unix(), Was: was}
	applyMain(it, ttn)
	return s.saveDiscMain(ctx, it)
}

func (s *Store) saveDiscMain(ctx context.Context, it *Item) error {
	b, err := json.Marshal(it.Disc)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE items SET disc_json = ?, runtime_sec = ?, audio_langs = ?, sub_langs = ? WHERE id = ?`,
		string(b), it.RuntimeSec, joinList(it.AudioLangs), joinList(it.SubLangs), it.ID)
	return err
}

// SetDiscRank keeps a ranking worked out after the scan, such as for a disc scanned
// before its module could rank. The main title follows it unless someone chose another.
func (s *Store) SetDiscRank(ctx context.Context, it *Item, rank *DiscRank) error {
	if it.Disc == nil || rank == nil {
		return nil
	}
	it.Disc.Rank = rank
	if it.Disc.Chosen == nil && rank.Main >= 0 && it.Disc.HasTitle(rank.Main) {
		applyMain(it, rank.Main)
	}
	return s.saveDiscMain(ctx, it)
}

// rankDVD asks the engine which title of an opened disc is the film, keeps its answer in
// info, and returns that title laid out to play: the engine's own cell list when the film
// is more than one title's chapter table. With no ranking (an older WittyDVD, or an
// analysis that failed) it is the older rule's pick.
func (s *Scanner) rankDVD(ctx context.Context, path string, d *dvd.Disc, info *DiscInfo) *dvd.TitleSummary {
	m := d.MainTitle()
	an, err := dvdsession.Analyze(path)
	if err != nil {
		s.Log.InfoContext(ctx, "this DVD's titles weren't ranked, so the older rule picks its film", "cat", "discs", "file", path, "err", err)
		return m
	}
	rank := DVDRank(an)
	if rank == nil {
		return m
	}
	info.Rank = rank
	if rank.Main < 1 {
		return m
	}
	if t := d.Title(rank.Main); t != nil {
		m = t
	}
	if f := rank.Feature; f != nil {
		ft, _, ok := d.FeatureTitle(an.Ranking.Feature, 1)
		switch {
		case !ok:
			// It can't be laid out as one title here, so the disc's own commands carry
			// it from part to part when it plays.
			f.Segments = nil
		case f.Assembled:
			m = ft
		}
		if dt := info.title(rank.Main); dt != nil && f.Assembled {
			dt.DurationSec = f.DurationSec
			if ok {
				dt.Chapters = ft.Chapters
			}
		}
	}
	return m
}

// rankProtectedDVD reads what the engine can say of an encrypted disc's structure. Nothing
// of the disc plays; its page can still say what is on it.
func (s *Scanner) rankProtectedDVD(ctx context.Context, it *Item) {
	an, err := dvdsession.Analyze(it.Path)
	if err != nil {
		// Per disc in a scan, and usually no WittyDVD: the page says the disc is protected anyway.
		s.Log.DebugContext(ctx, "an encrypted DVD's structure couldn't be read", "cat", "discs", "file", it.Path, "err", err)
		return
	}
	rank := DVDRank(an)
	if rank == nil {
		return
	}
	rank.Protected = true
	if rank.Feature != nil {
		rank.Feature.Segments = nil
	}
	it.Disc.Rank = rank
	for _, c := range an.Ranking.Candidates {
		if c.Has("decoy") || c.Duration < 1 {
			continue
		}
		it.Disc.Titles = append(it.Disc.Titles, DiscTitle{Number: c.TitleNum, DurationSec: c.Duration, Angles: max(c.Angles, 1), Chapters: []float64{}, Audio: []string{}, Subtitles: []string{}})
	}
	if c := an.Ranking.Candidate(an.Ranking.Main); c != nil {
		it.Disc.MainTitle = c.TitleNum
		it.RuntimeSec = c.Duration
		if f := rank.Feature; f != nil && f.DurationSec > 0 {
			it.RuntimeSec = f.DurationSec
		}
	}
}

// DiscMainTitle is the title an item's disc counts as its film, laid out to read: the
// engine's assembled feature, someone's choice, or the older rule's pick.
func DiscMainTitle(d *dvd.Disc, info *DiscInfo) *dvd.TitleSummary {
	if info != nil && info.MainTitle > 0 {
		if f := info.FeaturePlays(info.MainTitle); f != nil {
			if ft, _, ok := d.FeatureTitle(f.Engine(), 1); ok {
				return ft
			}
		}
		if t := d.Title(info.MainTitle); t != nil && len(t.Cells) > 0 {
			return t
		}
	}
	return d.MainTitle()
}

// rankBluray asks the engine which playlist of an opened disc is the film, keeps its
// answer in info, and returns that playlist. With no ranking it is the engine's own
// MainPlaylist, which is an older WittyBluray's rule.
func (s *Scanner) rankBluray(ctx context.Context, path string, d bdsession.DiscEngine, info *DiscInfo) *bluray.Playlist {
	main := d.MainPlaylist()
	an, err := bdsession.Analyze(path, false, s.Log)
	if err != nil {
		s.Log.InfoContext(ctx, "this Blu-ray's playlists weren't ranked, so the engine's older rule picks its film", "cat", "discs", "file", path, "err", err)
		return main
	}
	rank := BlurayRank(an)
	if rank == nil {
		return main
	}
	info.Rank = rank
	if rank.Main >= 0 {
		if pl := d.Playlist(rank.Main); pl != nil && len(pl.Items) > 0 && len(pl.Items[0].Video) > 0 {
			main = pl
		}
	}
	return main
}

// rankProtectedBluray reads what the engine can say of an encrypted disc's structure.
// Nothing of the disc plays; its page can still say what is on it.
func (s *Scanner) rankProtectedBluray(ctx context.Context, it *Item) {
	an, err := bdsession.Analyze(it.Path, false, s.Log)
	if err != nil {
		s.Log.DebugContext(ctx, "an encrypted Blu-ray's structure couldn't be read", "cat", "discs", "file", it.Path, "err", err)
		return
	}
	rank := BlurayRank(an)
	if rank == nil {
		return
	}
	rank.Protected = true
	it.Disc.Rank = rank
	for _, c := range an.Ranked.Ranking.Candidates {
		if c.Class == "decoy" || c.Class == "menu" || c.Duration < 30 {
			continue
		}
		it.Disc.Titles = append(it.Disc.Titles, DiscTitle{Number: c.PlaylistNum, DurationSec: c.Duration, Angles: max(c.Angles, 1), Wide: true, Chapters: []float64{}, Audio: []string{}, Subtitles: []string{}})
	}
	if c := an.Ranked.Ranking.Candidate(rank.Main); c != nil {
		it.Disc.MainTitle = c.PlaylistNum
		it.RuntimeSec = c.Duration
	}
}
