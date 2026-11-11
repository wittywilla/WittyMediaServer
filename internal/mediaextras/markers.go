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
// earlier episodes, the credits, and adverts left in a recording. They come from, in
// order of trust: what an administrator set by hand, a <file>.skip.json, a <file>.edl,
// the file's chapter names, and the intro set for the whole show.

// Kinds of range.
const (
	KindIntro      = "intro"
	KindRecap      = "recap"
	KindCredits    = "credits"
	KindCommercial = "commercial"
)

// Sources, most trusted first.
const (
	SourceManual   = "manual"
	SourceSkipJSON = "skip.json"
	SourceEDL      = "edl"
	SourceChapters = "chapters"
	SourceShow     = "show"
	SourceNone     = "none"
)

// Range is a stretch of a video, in seconds.
type Range struct {
	StartSec float64 `json:"startSec"`
	EndSec   float64 `json:"endSec"`
	Label    string  `json:"label,omitempty"`
}

// Markers is what's known about one video.
type Markers struct {
	Intro       *Range  `json:"intro"`
	Recap       *Range  `json:"recap"`
	Credits     *Range  `json:"credits"`
	Commercials []Range `json:"commercials"`
	// Source is where the most trusted of these came from (see Sources), "none" for nothing.
	Source string `json:"source"`
	// Sources says where each one came from: intro, recap, credits, commercials.
	Sources map[string]string `json:"sources"`
}

// Empty says nothing is marked.
func (m *Markers) Empty() bool {
	return m == nil || (m.Intro == nil && m.Recap == nil && m.Credits == nil && len(m.Commercials) == 0)
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
// most trusted first. Commercials come whole from one layer, never mixed. Source is the
// most trusted source anything came from.
func Merge(layers ...Layer) Markers {
	out := Markers{Commercials: []Range{}, Sources: map[string]string{}, Source: SourceNone}
	for _, l := range layers {
		m := l.Markers
		if out.Intro == nil && m.Intro != nil {
			r := *m.Intro
			out.Intro, out.Sources[KindIntro] = &r, l.Source
		}
		if out.Recap == nil && m.Recap != nil {
			r := *m.Recap
			out.Recap, out.Sources[KindRecap] = &r, l.Source
		}
		if out.Credits == nil && m.Credits != nil {
			r := *m.Credits
			out.Credits, out.Sources[KindCredits] = &r, l.Source
		}
		if len(out.Commercials) == 0 && len(m.Commercials) > 0 {
			out.Commercials = append([]Range{}, m.Commercials...)
			sort.Slice(out.Commercials, func(i, j int) bool { return out.Commercials[i].StartSec < out.Commercials[j].StartSec })
			out.Sources["commercials"] = l.Source
		}
	}
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
