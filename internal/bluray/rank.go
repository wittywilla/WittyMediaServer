// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package bluray

import (
	"errors"
	"math"
)

// What WittyBluray's playlist ranking says about a disc, as the server reads it: which
// playlist is the film and why, the cuts of a film that has several, a TV disc's episodes,
// chapters on a playlist's playing time, and where a copy's structures disagree. Copies of
// the module's side of the same answers, which travel as JSON in a call's result.
//
// A helper may be a fork, so each answer is checked before anything uses it: every
// playlist it names is one the same helper listed, and every time is one a disc can have.
// A WittyBluray from before these calls refuses them with "unknown-method", and the server
// then uses the helper's own older pick ("main") and reads chapters from the marks itself.

const (
	maxRankCandidates = 400
	maxRankList       = 512
	maxRankText       = 600
	maxRankSeconds    = 48 * 60 * 60
	// RankingLimit is how many candidates the server asks for: enough for every real
	// playlist of a disc, while a disc with thousands of decoys sends only its best.
	RankingLimit = 60
)

// ErrNoRanking: the WittyBluray in use is from before the ranking.
var ErrNoRanking = errors.New("this WittyBluray has no playlist ranking")

// Evidence is one signal's part in a candidate's score.
type Evidence struct {
	Signal string  `json:"signal"`
	Points float64 `json:"points"`
	Detail string  `json:"detail"`
}

// Candidate is one playlist as the engine scored it.
type Candidate struct {
	PlaylistNum       int        `json:"playlist"`
	Duration          float64    `json:"durationSec"`
	Distinct          float64    `json:"distinctSec"`
	Chapters          int        `json:"chapters"`
	Items             int        `json:"items"`
	Clips             int        `json:"clips"`
	Size              int64      `json:"sizeBytes"`
	Audio             int        `json:"audioStreams"`
	Subtitles         int        `json:"subtitleStreams"`
	Languages         []string   `json:"audioLanguages,omitempty"`
	Angles            int        `json:"angles"`
	Score             float64    `json:"score"`
	Class             string     `json:"class"`
	Flags             []string   `json:"flags"`
	Reasons           []string   `json:"reasons"`
	Evidence          []Evidence `json:"evidence,omitempty"`
	Video             string     `json:"video,omitempty"`
	SubtitleLanguages []string   `json:"subtitleLanguages,omitempty"`
	Continuity        float64    `json:"continuity"`
	Cluster           int        `json:"cluster"`
	ClusterSize       int        `json:"clusterSize"`
}

// RankEpisode is one episode of a TV disc, in viewing order.
type RankEpisode struct {
	Index    int     `json:"index"`
	Playlist int     `json:"playlist"`
	Duration float64 `json:"durationSec"`
	Chapters int     `json:"chapters"`
}

// Edition is one cut of a film with several.
type Edition struct {
	Playlist  int     `json:"playlist"`
	Duration  float64 `json:"durationSec"`
	Shared    float64 `json:"sharedSec"`
	Own       float64 `json:"ownSec"`
	Label     string  `json:"label"`
	Chapters  int     `json:"chapters"`
	Main      bool    `json:"main,omitempty"`
	ByProgram bool    `json:"byProgram,omitempty"`
}

// Cluster is a group of playlists that play the same segments.
type Cluster struct {
	Kept       int     `json:"kept"`
	Size       int     `json:"size"`
	Shuffled   int     `json:"shuffled"`
	Duplicates int     `json:"duplicates"`
	Duration   float64 `json:"durationSec"`
	Ambiguous  bool    `json:"ambiguous,omitempty"`
}

// Part is one stretch of a play-all playlist. Episode is the playlist of the episode it
// matches, or -1.
type Part struct {
	Index     int     `json:"index"`
	FirstItem int     `json:"firstItem"`
	LastItem  int     `json:"lastItem"`
	Start     float64 `json:"startSec"`
	Duration  float64 `json:"durationSec"`
	Episode   int     `json:"episode"`
}

// Chapter is one chapter of a playlist, on its playing time.
type Chapter struct {
	Number   int     `json:"number"`
	Mark     int     `json:"mark"`
	Item     int     `json:"item"`
	Clip     string  `json:"clip"`
	ClipTime uint32  `json:"clipTime"`
	Start    float64 `json:"startSec"`
	Duration float64 `json:"durationSec"`
	Adjusted bool    `json:"adjusted,omitempty"`
}

// Disagreement is one difference between the engine and a second opinion.
type Disagreement struct {
	Playlist int    `json:"playlist"`
	Field    string `json:"field"`
	Ours     string `json:"ours"`
	Theirs   string `json:"theirs"`
	Note     string `json:"note,omitempty"`
}

// SecondOpinionReport is how another reader's account of the disc compares.
type SecondOpinionReport struct {
	Source        string         `json:"source"`
	Main          int            `json:"main"`
	AgreesOnMain  bool           `json:"agreesOnMain"`
	Disagreements []Disagreement `json:"disagreements,omitempty"`
	More          int            `json:"more,omitempty"`
}

// Ranking is the engine's whole answer for a disc. Playlist numbers start at 0, so "none"
// is -1.
type Ranking struct {
	Candidates      []Candidate          `json:"candidates"`
	Main            int                  `json:"main"`
	PlayAll         int                  `json:"playAll"`
	Episodes        []RankEpisode        `json:"episodes,omitempty"`
	Decoys          []int                `json:"decoys,omitempty"`
	FirstPlay       int                  `json:"firstPlay"`
	TitleOne        int                  `json:"titleOne"`
	BDJ             bool                 `json:"bdj,omitempty"`
	Warnings        []string             `json:"warnings,omitempty"`
	Confidence      float64              `json:"confidence"`
	ConfidenceLabel string               `json:"confidenceLabel"`
	Why             []string             `json:"why,omitempty"`
	Editions        []Edition            `json:"editions,omitempty"`
	Clusters        []Cluster            `json:"clusters,omitempty"`
	PlayAllParts    []Part               `json:"playAllParts,omitempty"`
	Chapters        []Chapter            `json:"chapters,omitempty"`
	Stereo3D        bool                 `json:"stereo3d,omitempty"`
	UHD             bool                 `json:"uhd,omitempty"`
	HDR             string               `json:"hdr,omitempty"`
	MovieOnly       bool                 `json:"movieOnly,omitempty"`
	Inconsistencies map[string]int       `json:"inconsistencies,omitempty"`
	SecondOpinion   *SecondOpinionReport `json:"secondOpinion,omitempty"`
}

// Candidate returns the scored playlist by number, or nil.
func (r *Ranking) Candidate(playlist int) *Candidate {
	if r == nil {
		return nil
	}
	for i := range r.Candidates {
		if r.Candidates[i].PlaylistNum == playlist {
			return &r.Candidates[i]
		}
	}
	return nil
}

// Cap says whether a libbluray the machine's owner installed can give a second opinion.
type Cap struct {
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
	Path      string `json:"path,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// Ranked answers "ranking". Total is how many playlists were ranked; the ranking holds the
// best of them. Note says why no second opinion was taken, when one was asked for.
type Ranked struct {
	Ranking   *Ranking `json:"ranking"`
	Total     int      `json:"total"`
	Note      string   `json:"note,omitempty"`
	Libbluray Cap      `json:"libbluray"`
}

// ChapterList answers "chapters".
type ChapterList struct {
	Playlist int       `json:"playlist"`
	Duration float64   `json:"durationSec"`
	Chapters []Chapter `json:"chapters"`
}

// Inconsistency is one place where a copy's playlists, clip information and stream files
// disagree.
type Inconsistency struct {
	Severity    string  `json:"severity"` // error, warn or info
	Kind        string  `json:"kind"`
	Playlist    int     `json:"playlist"`
	Clip        string  `json:"clip,omitempty"`
	What        string  `json:"what"`
	PlayableSec float64 `json:"playableSec,omitempty"`
}

// DiscMeta is the disc library metadata a disc carries about itself.
type DiscMeta struct {
	Language  string         `json:"language,omitempty"`
	Name      string         `json:"name,omitempty"`
	Titles    map[int]string `json:"titles,omitempty"`
	Languages []string       `json:"languages,omitempty"`
}

// Consistency answers "consistency".
type Consistency struct {
	Issues     []Inconsistency `json:"issues"`
	FromBackup []string        `json:"fromBackup,omitempty"`
	Notes      []string        `json:"notes,omitempty"`
	Damaged    []string        `json:"damaged,omitempty"`
	MovieOnly  bool            `json:"movieOnly,omitempty"`
	Meta       *DiscMeta       `json:"meta,omitempty"`
}

// Capabilities answers "capabilities".
type Capabilities struct {
	Libbluray Cap `json:"libbluray"`
}

func clipRank(s string) string {
	if len(s) > maxRankText {
		return s[:maxRankText]
	}
	return s
}

func rankSeconds(v float64) bool { return v >= 0 && v <= maxRankSeconds } // false for NaN

func rankFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func tidyRankTexts(list []string, most int) []string {
	if len(list) > most {
		list = list[:most]
	}
	for i := range list {
		list[i] = clipRank(list[i])
	}
	return list
}

// Tidy clips a capability's words: they are shown on a settings page.
func (c *Cap) Tidy() {
	c.Version, c.Path, c.Reason = clipRank(c.Version), clipRank(c.Path), clipRank(c.Reason)
}

// Check makes a ranking safe to show and to act on. known says whether the disc has a
// playlist of that number, by the same helper's account of it.
func (a *Ranked) Check(known func(int) bool) error {
	r := a.Ranking
	if r == nil {
		return errors.New("the playlist ranking is empty")
	}
	bad := func(what string) error { return errors.New("the playlist ranking " + what) }
	none := func(n int) bool { return n == -1 || known(n) }
	if len(r.Candidates) > maxRankCandidates || len(r.Episodes) > maxRankList || len(r.Editions) > maxRankList ||
		len(r.Clusters) > maxRankList || len(r.PlayAllParts) > maxRankList || len(r.Chapters) > 2*maxRankList || len(r.Inconsistencies) > maxRankList {
		return bad("lists more than a disc can hold")
	}
	// The decoys of an obfuscated disc run to thousands. Only their number is used.
	if len(r.Decoys) > 100000 {
		return bad("lists more than a disc can hold")
	}
	if !none(r.Main) || !none(r.PlayAll) || !none(r.FirstPlay) || !none(r.TitleOne) {
		return bad("names a playlist the disc doesn't have")
	}
	if !(r.Confidence >= 0 && r.Confidence <= 1) || a.Total < 0 {
		return bad("has an impossible confidence")
	}
	a.Note = clipRank(a.Note)
	a.Libbluray.Tidy()
	r.ConfidenceLabel, r.HDR = clipRank(r.ConfidenceLabel), clipRank(r.HDR)
	r.Why, r.Warnings = tidyRankTexts(r.Why, 32), tidyRankTexts(r.Warnings, 64)
	seen := map[int]bool{}
	for i := range r.Candidates {
		c := &r.Candidates[i]
		if !known(c.PlaylistNum) || seen[c.PlaylistNum] {
			return bad("names a playlist the disc doesn't have")
		}
		seen[c.PlaylistNum] = true
		if !rankSeconds(c.Duration) || !rankSeconds(c.Distinct) || !rankFinite(c.Score) || !rankFinite(c.Continuity) || c.Chapters < 0 || c.Items < 0 || c.Angles < 0 || c.Size < 0 {
			return bad("has an impossible playlist")
		}
		if len(c.Evidence) > 64 {
			c.Evidence = c.Evidence[:64]
		}
		for j := range c.Evidence {
			e := &c.Evidence[j]
			if !rankFinite(e.Points) {
				return bad("has an impossible reason")
			}
			e.Signal, e.Detail = clipRank(e.Signal), clipRank(e.Detail)
		}
		c.Class, c.Video = clipRank(c.Class), clipRank(c.Video)
		c.Flags, c.Reasons = tidyRankTexts(c.Flags, 64), tidyRankTexts(c.Reasons, 64)
		c.Languages, c.SubtitleLanguages = tidyRankTexts(c.Languages, 64), tidyRankTexts(c.SubtitleLanguages, 64)
	}
	if r.Main != -1 && !seen[r.Main] {
		return bad("picks a playlist it didn't send")
	}
	for i := range r.Episodes {
		if e := &r.Episodes[i]; !known(e.Playlist) || !rankSeconds(e.Duration) || e.Chapters < 0 {
			return bad("has an impossible episode")
		}
	}
	for i := range r.Editions {
		e := &r.Editions[i]
		if !known(e.Playlist) || !rankSeconds(e.Duration) || !rankSeconds(e.Shared) || !rankSeconds(e.Own) || e.Chapters < 0 {
			return bad("has an impossible edition")
		}
		e.Label = clipRank(e.Label)
	}
	for i := range r.PlayAllParts {
		p := &r.PlayAllParts[i]
		if !none(p.Episode) || !rankSeconds(p.Start) || !rankSeconds(p.Duration) || p.FirstItem < 0 || p.LastItem < p.FirstItem {
			return bad("has an impossible part")
		}
	}
	for i := range r.Clusters {
		if c := &r.Clusters[i]; !known(c.Kept) || c.Size < 0 || !rankSeconds(c.Duration) {
			return bad("has an impossible group")
		}
	}
	if err := checkChapters(r.Chapters); err != nil {
		return err
	}
	if so := r.SecondOpinion; so != nil {
		so.Source = clipRank(so.Source)
		if len(so.Disagreements) > 64 {
			so.More += len(so.Disagreements) - 64
			so.Disagreements = so.Disagreements[:64]
		}
		if so.More < 0 {
			so.More = 0
		}
		for i := range so.Disagreements {
			d := &so.Disagreements[i]
			d.Field, d.Ours, d.Theirs, d.Note = clipRank(d.Field), clipRank(d.Ours), clipRank(d.Theirs), clipRank(d.Note)
		}
	}
	return nil
}

func checkChapters(list []Chapter) error {
	prev := -1.0
	for i := range list {
		c := &list[i]
		if !rankSeconds(c.Start) || !rankSeconds(c.Duration) || c.Item < 0 || c.Mark < 0 || c.Start < prev {
			return errors.New("the chapters are impossible")
		}
		prev = c.Start
		c.Clip = clipRank(c.Clip)
	}
	return nil
}

// Check makes a chapter list safe to use for a playlist of that many play items.
func (c *ChapterList) Check(items int) error {
	if len(c.Chapters) > 2*maxRankList || !rankSeconds(c.Duration) {
		return errors.New("the chapters are impossible")
	}
	for _, ch := range c.Chapters {
		if ch.Item >= items {
			return errors.New("a chapter starts in a play item the playlist doesn't have")
		}
	}
	return checkChapters(c.Chapters)
}

// Tidy keeps a consistency report to what is worth showing.
func (c *Consistency) Tidy() {
	if len(c.Issues) > maxRankList {
		c.Issues = c.Issues[:maxRankList]
	}
	for i := range c.Issues {
		x := &c.Issues[i]
		x.Severity, x.Kind, x.Clip, x.What = clipRank(x.Severity), clipRank(x.Kind), clipRank(x.Clip), clipRank(x.What)
		if !rankSeconds(x.PlayableSec) {
			x.PlayableSec = 0
		}
	}
	c.FromBackup, c.Notes, c.Damaged = tidyRankTexts(c.FromBackup, 64), tidyRankTexts(c.Notes, 64), tidyRankTexts(c.Damaged, 64)
	if m := c.Meta; m != nil {
		m.Language, m.Name = clipRank(m.Language), clipRank(m.Name)
		m.Languages = tidyRankTexts(m.Languages, 64)
		if len(m.Titles) > maxRankList {
			m.Titles = nil
		}
		for k, v := range m.Titles {
			m.Titles[k] = clipRank(v)
		}
	}
}

// MarkChapters maps a playlist's entry marks onto its playing time, the way the server
// did before the engine answered "chapters": for a WittyBluray older than that call.
func MarkChapters(pl *Playlist) []Chapter {
	if pl == nil {
		return nil
	}
	starts := make([]float64, len(pl.Items))
	var t float64
	for i := range pl.Items {
		starts[i] = t
		t += pl.Items[i].Duration()
	}
	var out []Chapter
	for i, m := range pl.Marks {
		if m.Type != 1 || m.Item < 0 || m.Item >= len(pl.Items) {
			continue
		}
		at := starts[m.Item] + float64(int64(m.Time)-int64(pl.Items[m.Item].In))/45000
		out = append(out, Chapter{Number: len(out) + 1, Mark: i, Item: m.Item, Clip: pl.Items[m.Item].Clip, ClipTime: m.Time, Start: at})
	}
	for i := range out {
		end := t
		if i+1 < len(out) {
			end = out[i+1].Start
		}
		out[i].Duration = max(end-out[i].Start, 0)
	}
	return out
}
