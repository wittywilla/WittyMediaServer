// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package dvd

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// What WittyDVD's title ranking says about a disc, as the server reads it: which title is
// the film and why, what exactly to play for it, the episodes of a TV disc, other versions,
// and the decoys. A copy of the module's side of the same answer, which travels as JSON.
//
// A helper may be a fork, so the answer is checked like any other: its size before it is
// read, then every number against the disc the same helper described and against what a
// disc can hold. An answer that fails is the helper's fault. A WittyDVD from before the
// ranking has no such call, and says so with CodeUnknownCall: the server then falls back
// on its own older rule (Disc.MainTitle).

const (
	// MethodAnalyze answers the title ranking of the open disc as JSON (Ranking).
	MethodAnalyze = "analyze"
	// MethodReadStats answers how many unreadable sectors were padded so far (ReadStats).
	MethodReadStats = "readStats"

	maxRankingAnswer = 4 << 20
	// What a ranking may hold. A DVD has at most 99 titles; the lists that aren't per
	// title are bounded by what the module itself sends, with room to spare.
	maxRankCandidates = 99
	maxRankList       = 256
	maxRankSegments   = 2048
	maxRankText       = 600
	maxRankSeconds    = 24 * 60 * 60
)

// ErrNoRanking: the WittyDVD in use is from before the ranking.
var ErrNoRanking = errors.New("this WittyDVD has no title ranking")

// Evidence is one signal's part in a score or a decision.
type Evidence struct {
	Signal string  `json:"signal"`
	Points float64 `json:"points"`
	Detail string  `json:"detail"`
}

// Candidate is one title as the engine scored it.
type Candidate struct {
	TitleNum      int        `json:"title"`
	VTS           int        `json:"vts"`
	VTSTitle      int        `json:"vtsTitle"`
	PGC           int        `json:"pgc"`
	Duration      float64    `json:"durationSec"`
	Distinct      float64    `json:"distinctSec"`
	Chapters      int        `json:"chapters"`
	Cells         int        `json:"cells"`
	Score         float64    `json:"score"`
	Class         string     `json:"class"`
	Flags         []string   `json:"flags"`
	Reasons       []string   `json:"reasons"`
	Evidence      []Evidence `json:"evidence,omitempty"`
	Path          string     `json:"path,omitempty"`
	ChapterSource string     `json:"chapterSource,omitempty"`
	Angles        int        `json:"angles,omitempty"`
	Continues     int        `json:"continues,omitempty"`
	PartOf        int        `json:"partOf,omitempty"`
}

// Has says whether the candidate carries a flag, by its name.
func (c *Candidate) Has(flag string) bool {
	for _, f := range c.Flags {
		if f == flag {
			return true
		}
	}
	return false
}

// RankEpisode is one episode of a TV disc, in viewing order. When one title holds them all,
// StartSec and FirstChapter say where in it this one begins.
type RankEpisode struct {
	Index        int     `json:"index"`
	Title        int     `json:"title"`
	VTS          int     `json:"vts"`
	PGC          int     `json:"pgc"`
	Duration     float64 `json:"durationSec"`
	StartSec     float64 `json:"startSec"`
	FirstChapter int     `json:"firstChapter"`
	Source       string  `json:"source"`
}

// Decision is one thing the engine decided, with how sure it is and why.
type Decision struct {
	Kind       string     `json:"kind"`
	Titles     []int      `json:"titles,omitempty"`
	Confidence float64    `json:"confidence"`
	Level      string     `json:"level"`
	Summary    string     `json:"summary"`
	Because    []Evidence `json:"because,omitempty"`
	Against    []Evidence `json:"against,omitempty"`
	RunnerUp   int        `json:"runnerUp,omitempty"`
	Margin     float64    `json:"margin,omitempty"`
}

// Version is another way to see the film: a camera angle, a cut chosen by parental level,
// or another title sharing its video.
type Version struct {
	Kind     string  `json:"kind"` // angle, parental or cut
	Title    int     `json:"title"`
	Angle    int     `json:"angle,omitempty"`
	Levels   []int   `json:"parentalLevels,omitempty"`
	Duration float64 `json:"durationSec"`
	Cells    int     `json:"cells"`
	Shared   float64 `json:"shared,omitempty"`
	Label    string  `json:"label"`
}

// Feature is exactly what to play for the film, in order.
type Feature struct {
	Titles   []int            `json:"titles"`
	Duration float64          `json:"durationSec"`
	Source   string           `json:"source"` // chapters, commands or chain
	Segments []FeatureSegment `json:"segments"`
}

// FeatureSegment is a run of cells of one program chain.
type FeatureSegment struct {
	Title       int     `json:"title"`
	VTS         int     `json:"vts"`
	PGC         int     `json:"pgc"`
	FirstCell   int     `json:"firstCell"`
	LastCell    int     `json:"lastCell"`
	StartSec    float64 `json:"startSec"`
	Duration    float64 `json:"durationSec"`
	FirstSector uint32  `json:"firstSector"`
	LastSector  uint32  `json:"lastSector"`
}

// Ranking is the engine's whole answer for a disc.
type Ranking struct {
	Candidates []Candidate   `json:"candidates"`
	Main       int           `json:"main"` // 0 when the disc has no title
	PlayAll    int           `json:"playAll,omitempty"`
	Episodes   []RankEpisode `json:"episodes,omitempty"`
	Decoys     []int         `json:"decoys,omitempty"`
	FirstPlay  int           `json:"firstPlay,omitempty"`
	MenuPlay   int           `json:"menuPlay,omitempty"`
	Warnings   []string      `json:"warnings,omitempty"`
	Confidence float64       `json:"confidence"`
	Level      string        `json:"level,omitempty"`
	Why        *Decision     `json:"why,omitempty"`
	Decisions  []Decision    `json:"decisions,omitempty"`
	Feature    *Feature      `json:"feature,omitempty"`
	Versions   []Version     `json:"versions,omitempty"`
}

// Candidate returns the scored title by number, or nil.
func (r *Ranking) Candidate(title int) *Candidate {
	if r == nil {
		return nil
	}
	for i := range r.Candidates {
		if r.Candidates[i].TitleNum == title {
			return &r.Candidates[i]
		}
	}
	return nil
}

// ReadStats says how much of a disc could not be read and was passed on as zeros.
type ReadStats struct {
	BadSectors int64 `json:"badSectors"`
	Retries    int64 `json:"retries"`
}

// Repair is one place where a disc's tables disagreed, and what was done about it.
type Repair struct {
	VTS   int
	Title int
	Table string
	From  string
	Fixed bool
	Note  string
}

func clipRank(s string) string {
	if len(s) > maxRankText {
		return s[:maxRankText]
	}
	return s
}

func seconds(v float64) bool { return v >= 0 && v <= maxRankSeconds } // false for NaN

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func tidyEvidence(list []Evidence) ([]Evidence, error) {
	if len(list) > maxRankList {
		list = list[:maxRankList]
	}
	for i := range list {
		if !finite(list[i].Points) {
			return nil, errors.New("a reason has an impossible weight")
		}
		list[i].Signal, list[i].Detail = clipRank(list[i].Signal), clipRank(list[i].Detail)
	}
	return list, nil
}

func tidyTexts(list []string, most int) []string {
	if len(list) > most {
		list = list[:most]
	}
	for i := range list {
		list[i] = clipRank(list[i])
	}
	return list
}

// DecodeRanking reads the answer to MethodAnalyze and checks it against the disc it is
// about, which has that many titles. What it lets through is safe to show and to act on:
// every title it names exists, and every time is one a disc can have.
func DecodeRanking(b []byte, d *Disc) (*Ranking, error) {
	if len(b) > maxRankingAnswer {
		return nil, fmt.Errorf("the title ranking is too large: %d bytes", len(b))
	}
	var r Ranking
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("the title ranking is unreadable: %w", err)
	}
	titles := 0
	if d != nil && d.VMG != nil {
		titles = len(d.VMG.Titles)
	}
	title := func(n int) bool { return n >= 1 && n <= titles }
	none := func(n int) bool { return n == 0 || title(n) }
	bad := func(what string) (*Ranking, error) { return nil, errors.New("the title ranking " + what) }
	if len(r.Candidates) > maxRankCandidates || len(r.Episodes) > maxRankList || len(r.Decoys) > maxRankCandidates ||
		len(r.Versions) > maxRankList || len(r.Decisions) > maxRankList {
		return bad("lists more than a disc can hold")
	}
	if !none(r.Main) || !none(r.PlayAll) || !none(r.FirstPlay) || !none(r.MenuPlay) {
		return bad("names a title the disc doesn't have")
	}
	if !(r.Confidence >= 0 && r.Confidence <= 1) {
		return bad("has an impossible confidence")
	}
	r.Level = clipRank(r.Level)
	r.Warnings = tidyTexts(r.Warnings, maxRankList)
	seen := map[int]bool{}
	for i := range r.Candidates {
		c := &r.Candidates[i]
		if !title(c.TitleNum) || seen[c.TitleNum] || !none(c.Continues) || !none(c.PartOf) {
			return bad("names a title the disc doesn't have")
		}
		seen[c.TitleNum] = true
		if !seconds(c.Duration) || !seconds(c.Distinct) || !finite(c.Score) || c.Chapters < 0 || c.Cells < 0 || c.Angles < 0 {
			return bad("has an impossible title")
		}
		var err error
		if c.Evidence, err = tidyEvidence(c.Evidence); err != nil {
			return nil, err
		}
		c.Class, c.Path, c.ChapterSource = clipRank(c.Class), clipRank(c.Path), clipRank(c.ChapterSource)
		c.Reasons, c.Flags = tidyTexts(c.Reasons, maxRankList), tidyTexts(c.Flags, 64)
	}
	if r.Main != 0 && !seen[r.Main] {
		return bad("picks a title it didn't score")
	}
	for _, n := range r.Decoys {
		if !title(n) {
			return bad("names a title the disc doesn't have")
		}
	}
	for i := range r.Episodes {
		e := &r.Episodes[i]
		if !title(e.Title) || !seconds(e.Duration) || !seconds(e.StartSec) || e.FirstChapter < 0 || e.FirstChapter > 999 {
			return bad("has an impossible episode")
		}
		e.Source = clipRank(e.Source)
	}
	for i := range r.Versions {
		v := &r.Versions[i]
		if !title(v.Title) || !seconds(v.Duration) || v.Angle < 0 || v.Angle > 9 || len(v.Levels) > 8 || !finite(v.Shared) {
			return bad("has an impossible version")
		}
		v.Kind, v.Label = clipRank(v.Kind), clipRank(v.Label)
	}
	decisions := r.Decisions
	if r.Why != nil {
		decisions = append([]Decision{*r.Why}, decisions...)
	}
	for i := range decisions {
		dc := &decisions[i]
		if !(dc.Confidence >= 0 && dc.Confidence <= 1) || !none(dc.RunnerUp) || !finite(dc.Margin) || len(dc.Titles) > maxRankCandidates {
			return bad("has an impossible decision")
		}
		for _, n := range dc.Titles {
			if !title(n) {
				return bad("names a title the disc doesn't have")
			}
		}
		var err error
		if dc.Because, err = tidyEvidence(dc.Because); err != nil {
			return nil, err
		}
		if dc.Against, err = tidyEvidence(dc.Against); err != nil {
			return nil, err
		}
		dc.Kind, dc.Level, dc.Summary = clipRank(dc.Kind), clipRank(dc.Level), clipRank(dc.Summary)
	}
	if r.Why != nil {
		*r.Why, r.Decisions = decisions[0], decisions[1:]
	}
	if f := r.Feature; f != nil {
		if len(f.Titles) == 0 || len(f.Titles) > maxRankCandidates || len(f.Segments) > maxRankSegments || !seconds(f.Duration) {
			return bad("has an impossible feature")
		}
		in := map[int]bool{}
		for _, n := range f.Titles {
			if !title(n) {
				return bad("names a title the disc doesn't have")
			}
			in[n] = true
		}
		for _, s := range f.Segments {
			if !in[s.Title] || s.FirstCell < 1 || s.LastCell < s.FirstCell || s.LastCell > 255 || s.LastSector < s.FirstSector || !seconds(s.StartSec) || !seconds(s.Duration) {
				return bad("has an impossible feature")
			}
			// A segment is cells of a chain the disc has, in the title's own title set.
			v := d.VTS[d.VMG.Titles[s.Title-1].VTS]
			if s.VTS != d.VMG.Titles[s.Title-1].VTS || v == nil || s.PGC < 1 || s.PGC > len(v.PGCs) || v.PGCs[s.PGC-1].PGC == nil || s.LastCell > len(v.PGCs[s.PGC-1].PGC.Cells) {
				return bad("plays cells the disc doesn't have")
			}
		}
		f.Source = clipRank(f.Source)
	}
	return &r, nil
}

// DecodeReadStats reads the answer to MethodReadStats.
func DecodeReadStats(b []byte) (ReadStats, error) {
	var s ReadStats
	if len(b) > 4<<10 {
		return s, errors.New("the read count is too large")
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("the read count is unreadable: %w", err)
	}
	if s.BadSectors < 0 || s.Retries < 0 {
		return s, errors.New("the read count is impossible")
	}
	return s, nil
}

// FeatureTitle lays a feature out as one title: the cells of its segments in order, on one
// timeline, with chapters where the titles it runs through have theirs. It is how a film
// that only commands put together, or that is cut into titles playing on into each other,
// plays as one stream. ok is false when that can't be done from what the server holds: the
// parts lie in different title sets, or a segment names cells the disc doesn't have, and the
// disc's own commands then carry the film from part to part instead. titles names the
// title each cell belongs to.
func (d *Disc) FeatureTitle(f *Feature, angle int) (ts *TitleSummary, titles []int, ok bool) {
	if d == nil || d.VMG == nil || f == nil || len(f.Titles) == 0 || len(f.Segments) == 0 {
		return nil, nil, false
	}
	first := f.Titles[0]
	if first < 1 || first > len(d.VMG.Titles) {
		return nil, nil, false
	}
	t := d.VMG.Titles[first-1]
	v := d.VTS[t.VTS]
	if v == nil {
		return nil, nil, false
	}
	ts = &TitleSummary{Number: first, VTS: t.VTS, VTSTitle: t.VTSTitle, Angles: t.Angles, Audio: v.Audio, Subp: v.Subp, Video: v.Video}
	// Where chapters start: every chapter of every title the feature runs through.
	chapterAt := map[[2]int]bool{}
	for _, n := range f.Titles {
		if n < 1 || n > len(d.VMG.Titles) || d.VMG.Titles[n-1].VTS != t.VTS {
			return nil, nil, false
		}
		vt := d.VMG.Titles[n-1].VTSTitle
		if vt < 1 || vt > len(v.PTT) {
			continue
		}
		for _, p := range v.PTT[vt-1] {
			chapterAt[[2]int{p.PGCN, p.PGN}] = true
		}
	}
	for _, s := range f.Segments {
		if s.VTS != t.VTS || s.PGC < 1 || s.PGC > len(v.PGCs) || v.PGCs[s.PGC-1].PGC == nil {
			return nil, nil, false
		}
		p := v.PGCs[s.PGC-1].PGC
		if s.FirstCell < 1 || s.LastCell > len(p.Cells) {
			return nil, nil, false
		}
		var last uint32
		taken := false
		for n := s.FirstCell; n >= 1 && n <= s.LastCell && n <= len(p.Cells); n = nextCellN(p, n) {
			k := angleCell(p, n, angle)
			c := p.Cells[k-1]
			// The engine leaves out cells that point nowhere, and so does this: within
			// a segment the cells it kept go forward through the segment's sectors.
			if c.LastSector < c.FirstSector || c.FirstSector < s.FirstSector || (angle <= 1 && c.LastSector > s.LastSector) || (taken && c.FirstSector <= last) || !seconds(c.Duration) {
				continue
			}
			pg := programOf(p, n)
			if chapterAt[[2]int{s.PGC, pg}] && (len(p.Programs) == 0 || p.Programs[pg-1] == n) {
				ts.Chapters = append(ts.Chapters, ts.Duration)
			}
			ts.Cells = append(ts.Cells, TitleCell{PGCN: s.PGC, CellN: k, Cell: c, Start: ts.Duration})
			titles = append(titles, s.Title)
			ts.Duration += c.Duration
			last, taken = c.LastSector, true
		}
	}
	if len(ts.Cells) == 0 {
		return nil, nil, false
	}
	if len(ts.Chapters) == 0 || ts.Chapters[0] > 0.5 {
		ts.Chapters = append([]float64{0}, ts.Chapters...)
	}
	return ts, titles, true
}
