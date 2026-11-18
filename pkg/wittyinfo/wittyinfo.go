// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package wittyinfo reads and writes .wittyinfo files: what people add by hand to a film,
// an episode, a season or a whole show. Skip segments, scenes, content filters, metadata,
// artwork, subtitles and playback preferences, with who made it and which cut of the video
// it was made for. docs/formats/wittyinfo.md describes the format; this package is the
// reference reader and writer, and the server and the wittyinfo program both use it.
//
// A .wittyinfo is a ZIP archive holding manifest.json and, if wanted, pictures and subtitle
// files. A bare manifest (a JSON file) is read too, so it can be written by hand.
//
// Nothing in a .wittyinfo is ever run, fetched or followed: names inside it refer only to
// other members of the same archive, and every part is checked against the limits below
// before any of it is used.
package wittyinfo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Format is the value of the manifest's "format" field.
const Format = "wittyinfo"

// Version is the newest version of the format this package writes. A file with a higher
// version is still read: what this version knows is used and the rest is kept.
const Version = 1

// Ext is the file extension, dot included.
const Ext = ".wittyinfo"

// Scopes: what a file is about.
const (
	ScopeItem   = "item"   // one film or one episode: Movie (2023).wittyinfo
	ScopeSeason = "season" // every episode of a season: season01.wittyinfo
	ScopeShow   = "show"   // every episode of a show: show.wittyinfo
)

// Segment types.
const (
	SegIntro       = "intro"
	SegRecap       = "recap"
	SegCredits     = "credits"
	SegPreview     = "preview"     // "next time on": a look at the next episode
	SegPostCredits = "postcredits" // a scene after the credits: worth not skipping past
	SegCommercial  = "commercial"  // an advert break left in a recording
	SegCustom      = "custom"      // anything else someone wants to skip
)

// SegmentTypes lists every segment type, in the order editors offer them.
var SegmentTypes = []string{SegIntro, SegRecap, SegCredits, SegPreview, SegPostCredits, SegCommercial, SegCustom}

// Segment actions.
const (
	ActButton = "button" // offer a skip button while it plays
	ActAuto   = "auto"   // skip it by itself, if the viewer allows skipping by itself
	ActMark   = "mark"   // only show where it is
)

// Actions lists the segment actions.
var Actions = []string{ActButton, ActAuto, ActMark}

// DefaultAction is what a segment does when its file doesn't say.
func DefaultAction(segType string) string {
	if segType == SegPostCredits {
		return ActMark
	}
	return ActButton
}

// Filter categories.
var FilterCategories = []string{"violence", "language", "nudity", "sex", "drugs", "frightening", "flashing", "discrimination", "other"}

// Filter actions: what a filter suggests. A viewer's own content settings decide in the end.
var FilterActions = []string{"skip", "mute", "warn"}

// Artwork roles.
var ArtworkRoles = []string{"poster", "backdrop", "logo", "thumb", "banner", "clearart", "disc", "square"}

// Episode kinds, mostly for anime: whether an episode is part of the story.
var EpisodeKinds = []string{"canon", "filler", "mixed", "recap"}

// Info is a whole .wittyinfo: its manifest and the other members of its archive.
type Info struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	Scope   string `json:"scope,omitempty"`
	// Season: the season a season file is for.
	Season *int `json:"season,omitempty"`

	Identity   *Identity   `json:"identity,omitempty"`
	Authorship *Authorship `json:"authorship,omitempty"`

	Segments []Segment `json:"segments,omitempty"`
	// Absent: segment types this video has none of. Defaults and automatic detection are
	// not used for them ("this episode has no intro").
	Absent    []string   `json:"absent,omitempty"`
	Scenes    []Scene    `json:"scenes,omitempty"`
	Filters   []Filter   `json:"filters,omitempty"`
	Metadata  *Metadata  `json:"metadata,omitempty"`
	Artwork   Artwork    `json:"artwork,omitempty"`
	Playback  *Playback  `json:"playback,omitempty"`
	Subtitles []Subtitle `json:"subtitles,omitempty"`
	Notes     string     `json:"notes,omitempty"`
	// Language: the language the file's own text is in. Translations: the same text in
	// others, by language tag (lang.go).
	Language string `json:"language,omitempty"`
	// Audio: music details, in a .wittyaudio only (audio.go).
	Audio        *Audio                  `json:"audio,omitempty"`
	Translations map[string]*Translation `json:"translations,omitempty"`
	// Extensions: anything a tool wants to keep, under a name of its own ("tool.key").
	// Kept as it is and never used by this package.
	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`

	Extra
	// Files: the archive's other members by name ("images/poster.jpg"), checked.
	Files map[string][]byte `json:"-"`
}

// Identity says which video a file was made for, so that times made for one cut are not
// used on another.
type Identity struct {
	DurationSec Seconds `json:"durationSec"`
	// ToleranceSec: how far the length may differ and still be the same cut. Default
	// DefaultTolerance(durationSec).
	ToleranceSec *float64     `json:"toleranceSec,omitempty"`
	FileName     string       `json:"fileName,omitempty"`
	FileSize     int64        `json:"fileSize,omitempty"`
	Edition      string       `json:"edition,omitempty"`
	Fingerprint  *Fingerprint `json:"fingerprint,omitempty"`
	Extra
}

// Fingerprint is a short value that names the exact file. FingerprintKind is the only
// kind this version makes; others are kept and not compared.
type Fingerprint struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Extra
}

// Authorship: who made the file, when, and with what.
type Authorship struct {
	Author  string `json:"author,omitempty"`
	Created string `json:"created,omitempty"` // RFC 3339
	Updated string `json:"updated,omitempty"` // RFC 3339
	Tool    *Tool  `json:"tool,omitempty"`
	License string `json:"license,omitempty"` // an SPDX id, such as CC-BY-4.0
	Source  string `json:"source,omitempty"`  // where the times came from, in words
	Extra
}

// Tool is the program that wrote the file last.
type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Extra
}

// Segment is a stretch of the video to skip, or to know about.
type Segment struct {
	ID    string  `json:"id,omitempty"`
	Type  string  `json:"type"`
	Start Seconds `json:"start"`
	// End: nil runs to the end of the video (credits, mostly).
	End    *Seconds `json:"end,omitempty"`
	Label  string   `json:"label,omitempty"`
	Action string   `json:"action,omitempty"`
	Note   string   `json:"note,omitempty"`
	Extra
}

// Act is the segment's action, its default when the file has none.
func (s *Segment) Act() string {
	if s.Action == "" {
		return DefaultAction(s.Type)
	}
	return s.Action
}

// Scene is a named point in the video, as chapters are.
type Scene struct {
	ID    string  `json:"id,omitempty"`
	Title string  `json:"title"`
	Start Seconds `json:"start"`
	// ThumbnailAt: the moment to show as the scene's picture. Image: a picture in the
	// archive instead ("images/scenes/01.jpg").
	ThumbnailAt *Seconds `json:"thumbnailAt,omitempty"`
	Image       string   `json:"image,omitempty"`
	Description string   `json:"description,omitempty"`
	Extra
}

// Filter marks something a viewer may not want to see or hear.
type Filter struct {
	ID       string  `json:"id,omitempty"`
	Category string  `json:"category"`
	Severity int     `json:"severity,omitempty"` // 1 mild, 2 moderate, 3 strong; 0 not said
	Start    Seconds `json:"start"`
	End      Seconds `json:"end"`
	Action   string  `json:"action,omitempty"` // skip | mute | warn: a suggestion
	Label    string  `json:"label,omitempty"`
	Extra
}

// Metadata replaces what the server found about a title. Every field is optional.
type Metadata struct {
	Title         string            `json:"title,omitempty"`
	OriginalTitle string            `json:"originalTitle,omitempty"`
	SortTitle     string            `json:"sortTitle,omitempty"`
	Edition       string            `json:"edition,omitempty"`
	Year          *int              `json:"year,omitempty"`
	ReleaseDate   string            `json:"releaseDate,omitempty"` // YYYY-MM-DD
	Overview      string            `json:"overview,omitempty"`
	Tagline       string            `json:"tagline,omitempty"`
	Genres        []string          `json:"genres,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	ContentRating string            `json:"contentRating,omitempty"`
	Studios       []string          `json:"studios,omitempty"`
	Countries     []string          `json:"countries,omitempty"`
	Languages     []string          `json:"languages,omitempty"`
	Collection    string            `json:"collection,omitempty"`
	EpisodeKind   string            `json:"episodeKind,omitempty"`
	ExternalIDs   map[string]string `json:"externalIds,omitempty"`
	Trivia        []string          `json:"trivia,omitempty"`
	Extra
}

// Artwork names a picture in the archive for each role: {"poster": "images/poster.jpg"}.
type Artwork map[string]string

// Playback holds how the video is best played. Players follow what they can.
type Playback struct {
	StartAt          *Seconds `json:"startAt,omitempty"` // where playing starts the first time
	AudioLanguage    string   `json:"audioLanguage,omitempty"`
	SubtitleLanguage string   `json:"subtitleLanguage,omitempty"`
	SubtitleMode     string   `json:"subtitleMode,omitempty"` // off | forced | always
	VolumeDB         *float64 `json:"volumeDb,omitempty"`
	AspectRatio      string   `json:"aspectRatio,omitempty"` // "16:9", "2.39:1"
	Extra
}

// Subtitle is a subtitle file in the archive.
type Subtitle struct {
	File     string `json:"file"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Forced   bool   `json:"forced,omitempty"`
	SDH      bool   `json:"sdh,omitempty"`
	Extra
}

// ------------------------------------------------------------------ seconds

// Seconds is a time in the video. It is written as a number of seconds; a string such as
// "1:05", "1:02:03.5" or "85.5" is read too, for files written by hand.
type Seconds float64

// UnmarshalJSON reads a number or a clock string.
func (s *Seconds) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		var t string
		if err := json.Unmarshal(b, &t); err != nil {
			return err
		}
		v, ok := ParseClock(t)
		if !ok {
			return fmt.Errorf("%q is not a time: use seconds or h:mm:ss", t)
		}
		*s = Seconds(v)
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("a time has to be a number of seconds")
	}
	*s = Seconds(f)
	return nil
}

// MarshalJSON writes seconds to the millisecond.
func (s Seconds) MarshalJSON() ([]byte, error) {
	v := math.Round(float64(s)*1000) / 1000
	return []byte(strconv.FormatFloat(v, 'f', -1, 64)), nil
}

// Sec is a pointer for an optional time.
func Sec(v float64) *Seconds { s := Seconds(v); return &s }

// ParseClock reads 85.5, 1:05, 1:02:03 or 1:02:03.250.
func ParseClock(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	parts := strings.Split(s, ":")
	if len(parts) > 3 {
		return 0, false
	}
	total := 0.0
	for i, p := range parts {
		if p == "" || strings.ContainsAny(p, "+-eE") {
			return 0, false
		}
		v, err := strconv.ParseFloat(p, 64)
		if err != nil || v < 0 || (i > 0 && v >= 60) || (i < len(parts)-1 && strings.Contains(p, ".")) {
			return 0, false
		}
		total = total*60 + v
	}
	return total, true
}

// Clock writes seconds as m:ss or h:mm:ss, with milliseconds when there are any.
func Clock(sec float64) string {
	ms := int64(math.Round(sec * 1000))
	h, m, s, f := ms/3600000, ms/60000%60, ms/1000%60, ms%1000
	out := fmt.Sprintf("%d:%02d", m, s)
	if h > 0 {
		out = fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	if f > 0 {
		out += strings.TrimRight(fmt.Sprintf(".%03d", f), "0")
	}
	return out
}

// ------------------------------------------------------------------ unknown fields

// Extra keeps the fields of an object this version doesn't know, so that a file written
// by a newer tool loses nothing when an older one saves it again. They are never used.
type Extra struct {
	Unknown map[string]json.RawMessage `json:"-"`
}

// knownFields caches each struct's JSON field names.
var knownFields sync.Map // reflect.Type -> map[string]bool

func fieldsOf(t reflect.Type) map[string]bool {
	if v, ok := knownFields.Load(t); ok {
		return v.(map[string]bool)
	}
	out := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" || f.Anonymous {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = true
	}
	knownFields.Store(t, out)
	return out
}

// decodeKeeping fills v (a pointer to an alias of a struct with Extra) and returns the
// fields it didn't know.
func decodeKeeping(b []byte, v any) (map[string]json.RawMessage, error) {
	if err := json.Unmarshal(b, v); err != nil {
		return nil, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, err
	}
	known := fieldsOf(reflect.TypeOf(v).Elem())
	var extra map[string]json.RawMessage
	for k, raw := range all {
		if known[k] {
			continue
		}
		if extra == nil {
			extra = map[string]json.RawMessage{}
		}
		extra[k] = raw
	}
	return extra, nil
}

// encodeKeeping writes v (an alias) with the unknown fields put back, known ones first.
func encodeKeeping(v any, extra map[string]json.RawMessage) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil || len(extra) == 0 {
		return b, err
	}
	known := fieldsOf(reflect.TypeOf(v).Elem())
	keys := make([]string, 0, len(extra))
	for k := range extra {
		if !known[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.Write(b[:len(b)-1])
	for _, k := range keys {
		if buf.Len() > 1 {
			buf.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(extra[k])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Each type with Extra reads and writes itself through an alias, which has no methods.

func (x *Info) UnmarshalJSON(b []byte) error {
	type a Info
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *Info) MarshalJSON() ([]byte, error) { type a Info; return encodeKeeping((*a)(x), x.Unknown) }

func (x *Identity) UnmarshalJSON(b []byte) error {
	type a Identity
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *Identity) MarshalJSON() ([]byte, error) {
	type a Identity
	return encodeKeeping((*a)(x), x.Unknown)
}

func (x *Fingerprint) UnmarshalJSON(b []byte) error {
	type a Fingerprint
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *Fingerprint) MarshalJSON() ([]byte, error) {
	type a Fingerprint
	return encodeKeeping((*a)(x), x.Unknown)
}

func (x *Authorship) UnmarshalJSON(b []byte) error {
	type a Authorship
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *Authorship) MarshalJSON() ([]byte, error) {
	type a Authorship
	return encodeKeeping((*a)(x), x.Unknown)
}

func (x *Tool) UnmarshalJSON(b []byte) error {
	type a Tool
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *Tool) MarshalJSON() ([]byte, error) { type a Tool; return encodeKeeping((*a)(x), x.Unknown) }

func (x *Segment) UnmarshalJSON(b []byte) error {
	type a Segment
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *Segment) MarshalJSON() ([]byte, error) {
	type a Segment
	return encodeKeeping((*a)(x), x.Unknown)
}

func (x *Scene) UnmarshalJSON(b []byte) error {
	type a Scene
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *Scene) MarshalJSON() ([]byte, error) { type a Scene; return encodeKeeping((*a)(x), x.Unknown) }

func (x *Filter) UnmarshalJSON(b []byte) error {
	type a Filter
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *Filter) MarshalJSON() ([]byte, error) {
	type a Filter
	return encodeKeeping((*a)(x), x.Unknown)
}

func (x *Metadata) UnmarshalJSON(b []byte) error {
	type a Metadata
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *Metadata) MarshalJSON() ([]byte, error) {
	type a Metadata
	return encodeKeeping((*a)(x), x.Unknown)
}

func (x *Playback) UnmarshalJSON(b []byte) error {
	type a Playback
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *Playback) MarshalJSON() ([]byte, error) {
	type a Playback
	return encodeKeeping((*a)(x), x.Unknown)
}

func (x *Subtitle) UnmarshalJSON(b []byte) error {
	type a Subtitle
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *Subtitle) MarshalJSON() ([]byte, error) {
	type a Subtitle
	return encodeKeeping((*a)(x), x.Unknown)
}
