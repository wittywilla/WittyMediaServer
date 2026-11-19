// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package mediaextras

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Markers say where the parts of a video are that people skip: the intro, a recap of
// earlier episodes, the credits, adverts left in a recording, and from a .wittyinfo also a
// preview of the next episode, a scene after the credits and anything else someone marked.
// They come from, in order of trust: the video's own .wittyinfo, what an administrator set
// by hand, a <file>.skip.json, a <file>.edl, the season's and then the show's .wittyinfo,
// the file's chapter names, what comparing the season's episodes by ear found, and the
// intro set for the whole show.

// Kinds of range.
const (
	KindIntro      = "intro"
	KindRecap      = "recap"
	KindCredits    = "credits"
	KindCommercial = "commercial"
	// Only a .wittyinfo gives these.
	KindPreview     = "preview"
	KindPostCredits = "postcredits"
	KindCustom      = "custom"
)

// Actions: what the player does with a range.
const (
	ActButton = "button" // a skip button while it plays
	ActAuto   = "auto"   // skipped by itself when the viewer allows that
	ActMark   = "mark"   // only shown where it is
)

// Sources, most trusted first.
const (
	// SourceWittyinfo: the video's own .wittyinfo (or the copy kept by the server).
	SourceWittyinfo = "wittyinfo"
	SourceManual    = "manual"
	SourceSkipJSON  = "skip.json"
	SourceEDL       = "edl"
	SourceChapters  = "chapters"
	// SourceDetected: found by comparing the sound of a season's episodes.
	SourceDetected = "detected"
	SourceShow     = "show"
	SourceNone     = "none"
	// The season's and the show's .wittyinfo: defaults for every episode.
	SourceSeasonInfo = "wittyinfo-season"
	SourceShowInfo   = "wittyinfo-show"
)

// Range is a stretch of a video, in seconds.
type Range struct {
	StartSec float64 `json:"startSec"`
	EndSec   float64 `json:"endSec"`
	Label    string  `json:"label,omitempty"`
	// Action: button, auto or mark; empty is a button.
	Action string `json:"action,omitempty"`
}

// Markers is what's known about one video.
type Markers struct {
	Intro       *Range  `json:"intro"`
	Recap       *Range  `json:"recap"`
	Credits     *Range  `json:"credits"`
	Commercials []Range `json:"commercials"`
	// From a .wittyinfo only.
	Preview     *Range  `json:"preview,omitempty"`
	PostCredits *Range  `json:"postCredits,omitempty"`
	Custom      []Range `json:"custom,omitempty"`
	// Absent: kinds this video is known to have none of; less trusted sources are not
	// asked about them.
	Absent []string `json:"absent,omitempty"`
	// Segments is every range above as one list in time order, with its kind and source,
	// for players and editors that show them all.
	Segments []Segment `json:"segments"`
	// Source is where the most trusted of these came from (see Sources), "none" for nothing.
	Source string `json:"source"`
	// Sources says where each one came from: intro, recap, credits, commercials.
	Sources map[string]string `json:"sources"`
}

// Segment is one range of Markers.Segments.
type Segment struct {
	Kind     string  `json:"kind"`
	StartSec float64 `json:"startSec"`
	EndSec   float64 `json:"endSec"`
	Label    string  `json:"label,omitempty"`
	Action   string  `json:"action"`
	Source   string  `json:"source"`
}

// Empty says nothing is marked.
func (m *Markers) Empty() bool {
	return m == nil || (m.Intro == nil && m.Recap == nil && m.Credits == nil && len(m.Commercials) == 0 &&
		m.Preview == nil && m.PostCredits == nil && len(m.Custom) == 0 && len(m.Absent) == 0)
}

// ------------------------------------------------------------------ chapter names

var (
	// recapRe: "Previously on...", "Recap", and the same in German, French, Spanish and Japanese.
	recapRe = regexp.MustCompile(`(?i)(\bpreviously\b|\brecap\b|\brecapitulation\b|\bzuvor\b|was bisher geschah|\bbisher bei\b|\brückblick\b|\bruckblick\b|précédemment|precedemment|\banteriormente\b|\bpreviamente\b|lo que pasó|lo que paso|\bresumen\b|\barasuji\b|あらすじ|前回)`)
	// openingCreditsRe names an intro that also says "credits".
	openingCreditsRe = regexp.MustCompile(`(?i)(opening credits|générique de début|generique de debut|générique d'ouverture|generique d'ouverture|créditos iniciales|creditos iniciales|créditos de apertura)`)
	// creditsRe: end credits and endings, in the same languages. "ED" only in capitals.
	creditsRe = regexp.MustCompile(`(?i)(\bcredits\b|\bend credits\b|\bclosing\b|\boutro\b|\bending\b|\bend titles\b|\babspann\b|\bnachspann\b|\babschluss\b|générique de fin|generique de fin|générique fin|\bcrédits\b|\bcréditos\b|\bcreditos\b|\bcréditos finales\b|\bcierre\b|\bstaff ?roll\b|\bsutaffu\b|エンディング|スタッフロール)`)
	edRe      = regexp.MustCompile(`(^|[^A-Za-z])ED([^A-Za-z]|$)`)
	// introRe: what the scan has always taken as an intro, and the same in other languages.
	introRe = regexp.MustCompile(`(?i)(\bintro\b|\bopening\b|\bop\b|\btheme\b|\btitle sequence\b|\bvorspann\b|\bgénérique\b|\bgenerique\b|\bintroducción\b|\bintroduccion\b|\bapertura\b|\bcabecera\b|\bopuningu\b|オープニング)`)
)

// ChapterKind says what a chapter's name marks: intro, recap, credits, or "" for an
// ordinary chapter. "Opening Credits" is an intro, "Ending Theme" credits.
func ChapterKind(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return ""
	}
	switch {
	case recapRe.MatchString(t):
		return KindRecap
	case openingCreditsRe.MatchString(t):
		return KindIntro
	case creditsRe.MatchString(t) || edRe.MatchString(t):
		return KindCredits
	case introRe.MatchString(t):
		return KindIntro
	}
	return ""
}

// Chapter is a chapter as ffprobe reads it.
type Chapter struct {
	Start, End float64
	Title      string
}

// Longest each kind of chapter can be and still be taken for what its name says: a
// chapter called "Intro" that runs twenty minutes is something else.
var maxChapter = map[string]float64{KindIntro: 300, KindRecap: 300, KindCredits: 900}

// FromChapters reads the markers chapter names give. The first chapter of each kind
// counts, except credits, where the last does (a cold open can have "credits" too).
func FromChapters(chs []Chapter) Markers {
	var m Markers
	for _, c := range chs {
		kind := ChapterKind(c.Title)
		if kind == "" || !(c.End > c.Start) || c.Start < 0 || c.End-c.Start > maxChapter[kind] {
			continue
		}
		r := &Range{StartSec: c.Start, EndSec: c.End}
		switch kind {
		case KindIntro:
			if m.Intro == nil {
				m.Intro = r
			}
		case KindRecap:
			if m.Recap == nil {
				m.Recap = r
			}
		case KindCredits:
			m.Credits = r
		}
	}
	return m
}

// ------------------------------------------------------------------ EDL

// MaxSidecar is the most read of a .edl or .skip.json.
const MaxSidecar = 256 << 10

// maxRanges is the most ranges kept from one file.
const maxRanges = 200

// ParseEDL reads an edit decision list in the MPlayer and Kodi form: one range a line,
// "start end type", in seconds or as [hh:]mm:ss[.fff]. Type 3 is an advert break and type 0
// a cut, which are both listed as commercials; 1 (mute) and 2 (scene marker) are left
// out. The type may also be a word (intro, recap, credits, commercial), and anything
// after the type names the range, so "0 85.5 3 intro" marks the intro. Kodi's frame
// numbers (#1234) can't be read without the frame rate, so those lines are skipped, as
// are comments (lines starting with "#", "//" or ";") and anything else unreadable.
func ParseEDL(data []byte) Markers {
	var m Markers
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 4096), 4096)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		start, ok1 := parseTime(f[0])
		end, ok2 := parseTime(f[1])
		if !ok1 || !ok2 || !(end > start) {
			continue
		}
		typ, label := "3", ""
		if len(f) >= 3 {
			typ = strings.ToLower(f[2])
		}
		if len(f) >= 4 {
			label = strings.Join(f[3:], " ")
		}
		kind := ""
		switch typ {
		case "0", "3", "cut", "commercial", "ad", "ads", "advert":
			kind = KindCommercial
		case "1", "2":
		default:
			kind = rangeKind(typ)
		}
		if k := rangeKind(label); k != "" {
			kind = k
		}
		if kind == "" {
			continue
		}
		r := Range{StartSec: start, EndSec: end, Label: cleanLabel(label)}
		m.put(kind, r)
	}
	return m
}

// rangeKind reads a word naming a range: intro, recap, credits (or a chapter name that
// says so), commercial.
func rangeKind(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "":
		return ""
	case "intro", "opening", "op":
		return KindIntro
	case "recap", "previously":
		return KindRecap
	case "credits", "outro", "ending", "ed", "end":
		return KindCredits
	case "commercial", "commercials", "ad", "ads", "advert", "adverts", "advertisement", "break", "cut":
		return KindCommercial
	}
	return ChapterKind(s)
}

func cleanLabel(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 80 {
		s = strings.ToValidUTF8(s[:80], "")
	}
	return s
}

// parseTime reads seconds ("85.5") or [hh:]mm:ss[.fff].
func parseTime(s string) (float64, bool) {
	if s == "" || strings.HasPrefix(s, "#") {
		return 0, false
	}
	parts := strings.Split(s, ":")
	if len(parts) > 3 {
		return 0, false
	}
	total := 0.0
	for i, p := range parts {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) || (i > 0 && v >= 60) {
			return 0, false
		}
		total = total*60 + v
	}
	if total > 7*24*3600 {
		return 0, false
	}
	return total, true
}

// ------------------------------------------------------------------ skip.json

// SkipFile is the form of <file>.skip.json, and of an administrator's edit:
//
//	{"intro": {"start": 62, "end": 151.5}, "recap": {"start": 0, "end": 60},
//	 "credits": {"start": 1290}, "commercials": [{"start": 600, "end": 780}]}
//
// "startSec" and "endSec" are read too. A credits range without an end runs to the end.
type SkipFile struct {
	Intro       *SkipRange  `json:"intro,omitempty"`
	Recap       *SkipRange  `json:"recap,omitempty"`
	Credits     *SkipRange  `json:"credits,omitempty"`
	Commercials []SkipRange `json:"commercials,omitempty"`
}

// SkipRange is one range of a SkipFile.
type SkipRange struct {
	Start    *float64 `json:"start,omitempty"`
	End      *float64 `json:"end,omitempty"`
	StartSec *float64 `json:"startSec,omitempty"`
	EndSec   *float64 `json:"endSec,omitempty"`
	Label    string   `json:"label,omitempty"`
}

// ErrBadMarkers: the ranges don't make sense.
var ErrBadMarkers = errors.New("a range has to start at 0 or later and end after it starts")

func (r *SkipRange) times() (start, end float64, hasEnd bool) {
	switch {
	case r.Start != nil:
		start = *r.Start
	case r.StartSec != nil:
		start = *r.StartSec
	}
	switch {
	case r.End != nil:
		end, hasEnd = *r.End, true
	case r.EndSec != nil:
		end, hasEnd = *r.EndSec, true
	}
	return
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// rangeOf checks one range. openEnd lets the end be left out: it is then durationSec, or
// 0 ("to the end") when that isn't known.
func (r *SkipRange) rangeOf(openEnd bool, durationSec float64) (*Range, error) {
	start, end, hasEnd := r.times()
	if !hasEnd && openEnd {
		end = durationSec
		if end <= 0 {
			if !finite(start) || start < 0 {
				return nil, ErrBadMarkers
			}
			return &Range{StartSec: start, EndSec: 0, Label: cleanLabel(r.Label)}, nil
		}
	}
	if !finite(start) || !finite(end) || start < 0 || end <= start || end > 7*24*3600 {
		return nil, ErrBadMarkers
	}
	return &Range{StartSec: start, EndSec: end, Label: cleanLabel(r.Label)}, nil
}

// Markers turns a SkipFile into markers, refusing any range that doesn't make sense.
func (f *SkipFile) Markers(durationSec float64) (Markers, error) {
	var m Markers
	var err error
	if f.Intro != nil {
		if m.Intro, err = f.Intro.rangeOf(false, durationSec); err != nil {
			return m, err
		}
	}
	if f.Recap != nil {
		if m.Recap, err = f.Recap.rangeOf(false, durationSec); err != nil {
			return m, err
		}
	}
	if f.Credits != nil {
		if m.Credits, err = f.Credits.rangeOf(true, durationSec); err != nil {
			return m, err
		}
	}
	if len(f.Commercials) > maxRanges {
		return m, errors.New("too many commercial breaks")
	}
	for i := range f.Commercials {
		r, err := f.Commercials[i].rangeOf(false, durationSec)
		if err != nil {
			return m, err
		}
		m.Commercials = append(m.Commercials, *r)
	}
	return m, nil
}

// ParseSkipJSON reads a <file>.skip.json. A file that can't be read, or holds a range
// that doesn't make sense, gives nothing: it's a sidecar anyone could have left.
func ParseSkipJSON(data []byte, durationSec float64) (Markers, error) {
	var f SkipFile
	if err := json.Unmarshal(data, &f); err != nil {
		return Markers{}, err
	}
	return f.Markers(durationSec)
}

// ToSkipFile is the other way: markers as they are stored and edited.
func (m *Markers) ToSkipFile() SkipFile {
	conv := func(r *Range) *SkipRange {
		if r == nil {
			return nil
		}
		s, e := r.StartSec, r.EndSec
		out := &SkipRange{Start: &s, Label: r.Label}
		if e > 0 {
			out.End = &e
		}
		return out
	}
	f := SkipFile{Intro: conv(m.Intro), Recap: conv(m.Recap), Credits: conv(m.Credits)}
	for i := range m.Commercials {
		f.Commercials = append(f.Commercials, *conv(&m.Commercials[i]))
	}
	return f
}

// ------------------------------------------------------------------ merging

func (m *Markers) put(kind string, r Range) {
	switch kind {
	case KindIntro:
		if m.Intro == nil {
			m.Intro = &r
		}
	case KindRecap:
		if m.Recap == nil {
			m.Recap = &r
		}
	case KindCredits:
		m.Credits = &r // the last one: see FromChapters
	case KindCommercial:
		if len(m.Commercials) < maxRanges {
			m.Commercials = append(m.Commercials, r)
		}
	}
}

// Layer is one source of markers, for Merge.
type Layer struct {
	Source  string
	Markers Markers
}

// Merge takes each marker from the most trusted layer that has it: give the layers
// most trusted first. Lists (commercials, custom) come whole from one layer, never mixed.
// A layer that says a kind is absent settles it: nothing less trusted gives one. Source is
// the most trusted source anything came from.
func Merge(layers ...Layer) Markers {
	out := Markers{Commercials: []Range{}, Sources: map[string]string{}, Source: SourceNone}
	settled := map[string]bool{}
	one := func(kind string, dst **Range, src *Range, layer string) {
		if settled[kind] || src == nil {
			return
		}
		r := *src
		*dst, out.Sources[kind], settled[kind] = &r, layer, true
	}
	many := func(kind string, dst *[]Range, src []Range, layer string) {
		if settled[kind] || len(src) == 0 {
			return
		}
		*dst = append([]Range{}, src...)
		sort.Slice(*dst, func(i, j int) bool { return (*dst)[i].StartSec < (*dst)[j].StartSec })
		out.Sources[kind], settled[kind] = layer, true
	}
	for _, l := range layers {
		m := l.Markers
		one(KindIntro, &out.Intro, m.Intro, l.Source)
		one(KindRecap, &out.Recap, m.Recap, l.Source)
		one(KindCredits, &out.Credits, m.Credits, l.Source)
		one(KindPreview, &out.Preview, m.Preview, l.Source)
		one(KindPostCredits, &out.PostCredits, m.PostCredits, l.Source)
		many("commercials", &out.Commercials, m.Commercials, l.Source)
		many(KindCustom, &out.Custom, m.Custom, l.Source)
		for _, k := range m.Absent {
			key := k
			if k == KindCommercial {
				key = "commercials"
			}
			if !settled[key] {
				settled[key] = true
				out.Absent = append(out.Absent, k)
				out.Sources[key] = l.Source
			}
		}
	}
	out.Segments = out.segmentList()
	for _, l := range layers { // the first layer that gave anything
		for _, src := range out.Sources {
			if src == l.Source {
				out.Source = l.Source
				return out
			}
		}
	}
	return out
}

// segmentList puts every range in one list in time order.
func (m *Markers) segmentList() []Segment {
	out := []Segment{}
	add := func(kind, key string, r *Range) {
		if r == nil {
			return
		}
		act := r.Action
		if act == "" {
			act = ActButton
			if kind == KindPostCredits {
				act = ActMark
			}
		}
		out = append(out, Segment{Kind: kind, StartSec: r.StartSec, EndSec: r.EndSec, Label: r.Label, Action: act, Source: m.Sources[key]})
	}
	add(KindRecap, KindRecap, m.Recap)
	add(KindIntro, KindIntro, m.Intro)
	add(KindCredits, KindCredits, m.Credits)
	add(KindPreview, KindPreview, m.Preview)
	add(KindPostCredits, KindPostCredits, m.PostCredits)
	for i := range m.Commercials {
		add(KindCommercial, "commercials", &m.Commercials[i])
	}
	for i := range m.Custom {
		add(KindCustom, KindCustom, &m.Custom[i])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartSec < out[j].StartSec })
	return out
}

// FillEnds gives a range that runs "to the end" (an end of 0) the video's length.
func (m *Markers) FillEnds(durationSec float64) {
	if durationSec <= 0 {
		return
	}
	fix := func(r *Range) {
		if r != nil && r.EndSec <= 0 && durationSec > r.StartSec {
			r.EndSec = durationSec
		}
	}
	fix(m.Credits)
	fix(m.Preview)
	fix(m.PostCredits)
	for i := range m.Custom {
		fix(&m.Custom[i])
	}
	m.Segments = m.segmentList()
}
