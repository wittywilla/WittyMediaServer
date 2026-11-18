// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyinfo

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/wittywilla/wittymediaserver/pkg/langtag"
)

// Languages. A file says which language its own text is in ("language"), and can carry the
// same text in any number of others ("translations"), keyed by language tag: ISO 639-1, 639-2
// or 639-3 codes and BCP 47 tags with a script or region are all read (pkg/langtag), and
// written in their usual form ("ger" becomes "de", "pt_br" becomes "pt-BR"). A viewer gets
// the translation closest to their language: the same tag, else the same language.
//
// A translation names the parts it translates by their ids, which writers fill in (Tidy),
// so translations stay with their part however the parts are reordered.

// MaxTranslations is the most languages one file carries.
const MaxTranslations = 200

// Translation is a file's text in one language. Anything left out stays as the file has it.
type Translation struct {
	Segments map[string]TextPart `json:"segments,omitempty"`
	Scenes   map[string]TextPart `json:"scenes,omitempty"`
	Filters  map[string]TextPart `json:"filters,omitempty"`
	Metadata *Metadata           `json:"metadata,omitempty"`
	Notes    string              `json:"notes,omitempty"`
	// Lyrics: a song's words in this language (a .wittyaudio), timed like the song's own.
	Lyrics *Lyrics `json:"lyrics,omitempty"`
	Extra
}

func validTag(s string) bool { return langtag.Valid(s) }

// TextPart is the text of one segment, scene or filter in another language.
type TextPart struct {
	Label       string `json:"label,omitempty"`
	Title       string `json:"title,omitempty"`
	Note        string `json:"note,omitempty"`
	Description string `json:"description,omitempty"`
	Extra
}

func (x *Translation) UnmarshalJSON(b []byte) error {
	type a Translation
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *Translation) MarshalJSON() ([]byte, error) {
	type a Translation
	return encodeKeeping((*a)(x), x.Unknown)
}

func (x *TextPart) UnmarshalJSON(b []byte) error {
	type a TextPart
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x TextPart) MarshalJSON() ([]byte, error) {
	type a TextPart
	return encodeKeeping((*a)(&x), x.Unknown)
}

// checkLanguages checks every language tag in a file and the translations' text.
func (x *Info) checkLanguages(c *checker) {
	tag := func(where, v string) {
		if v != "" && !langtag.Valid(v) {
			c.bad(where, "%q is not a language code (use one such as en, deu, pt-BR or zh-Hant)", v)
		}
	}
	tag("language", x.Language)
	if p := x.Playback; p != nil {
		tag("playback.audioLanguage", p.AudioLanguage)
		tag("playback.subtitleLanguage", p.SubtitleLanguage)
	}
	for i, s := range x.Subtitles {
		tag(fmt.Sprintf("subtitles[%d].language", i), s.Language)
	}
	if m := x.Metadata; m != nil {
		for i, l := range m.Languages {
			tag(fmt.Sprintf("metadata.languages[%d]", i), l)
		}
	}
	if len(x.Translations) > MaxTranslations {
		c.bad("translations", "more than %d languages", MaxTranslations)
	}
	ids := func(n int, id func(int) string) map[string]bool {
		out := map[string]bool{}
		for i := 0; i < n; i++ {
			if v := id(i); v != "" {
				out[v] = true
			}
		}
		return out
	}
	segs := ids(len(x.Segments), func(i int) string { return x.Segments[i].ID })
	scenes := ids(len(x.Scenes), func(i int) string { return x.Scenes[i].ID })
	filters := ids(len(x.Filters), func(i int) string { return x.Filters[i].ID })
	seen := map[string]string{}
	for _, k := range sortedKeys(x.Translations) {
		w := "translations." + k
		canon, ok := langtag.Canonical(k)
		if !ok {
			c.bad(w, "%q is not a language code", k)
			continue
		}
		if other, dup := seen[canon]; dup {
			c.bad(w, "the same language as %s", other)
		}
		seen[canon] = k
		tr := x.Translations[k]
		if tr == nil {
			continue
		}
		parts := func(kind string, m map[string]TextPart, known map[string]bool) {
			for _, id := range sortedKeys(m) {
				pw := fmt.Sprintf("%s.%s.%s", w, kind, id)
				if !known[id] {
					c.bad(pw, "names a %s that isn't in the file", kind[:len(kind)-1])
				}
				p := m[id]
				c.text(pw+".label", p.Label, MaxShortText)
				c.text(pw+".title", p.Title, MaxShortText)
				c.text(pw+".note", p.Note, 2000)
				c.text(pw+".description", p.Description, 2000)
			}
		}
		parts("segments", tr.Segments, segs)
		parts("scenes", tr.Scenes, scenes)
		parts("filters", tr.Filters, filters)
		if m := tr.Metadata; m != nil {
			for where, v := range map[string]string{"title": m.Title, "originalTitle": m.OriginalTitle, "sortTitle": m.SortTitle, "edition": m.Edition, "tagline": m.Tagline, "collection": m.Collection} {
				c.text(w+".metadata."+where, v, MaxShortText)
			}
			c.text(w+".metadata.overview", m.Overview, MaxLongText)
			c.list(w+".metadata.genres", m.Genres, 64)
			c.list(w+".metadata.tags", m.Tags, 64)
			c.list(w+".metadata.trivia", m.Trivia, 2000)
		}
		c.text(w+".notes", tr.Notes, MaxLongText)
		checkLyrics(c, w+".lyrics", tr.Lyrics)
	}
}

// tidyLanguages writes every tag in its usual form, and merges translations that turn out
// to be the same language (the later one's text wins).
func (x *Info) tidyLanguages() {
	canon := func(v string) string {
		if c, ok := langtag.Canonical(v); ok {
			return c
		}
		return v
	}
	x.Language = canon(x.Language)
	if p := x.Playback; p != nil {
		p.AudioLanguage, p.SubtitleLanguage = canon(p.AudioLanguage), canon(p.SubtitleLanguage)
	}
	for i := range x.Subtitles {
		x.Subtitles[i].Language = canon(x.Subtitles[i].Language)
	}
	if m := x.Metadata; m != nil {
		for i := range m.Languages {
			m.Languages[i] = canon(m.Languages[i])
		}
	}
	if len(x.Translations) == 0 {
		x.Translations = nil
		return
	}
	out := map[string]*Translation{}
	for _, k := range sortedKeys(x.Translations) {
		if tr := x.Translations[k]; tr != nil && !tr.empty() {
			out[canon(k)] = tr
		}
	}
	if len(out) == 0 {
		out = nil
	}
	x.Translations = out
}

func (t *Translation) empty() bool {
	b, _ := json.Marshal(t)
	return string(b) == "{}"
}

// TranslationLanguages lists the languages a file has text in: its own first (if it says),
// then its translations in tag order.
func (x *Info) TranslationLanguages() []string {
	var out []string
	if x.Language != "" {
		out = append(out, x.Language)
	}
	keys := make([]string, 0, len(x.Translations))
	for k := range x.Translations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return append(out, keys...)
}

// Localize is the file with its text in the language closest to want (a tag in any form),
// or the file as it is when it has nothing closer than its own text. The file itself is
// not changed. The result's Language says which language its text is now in.
func (x *Info) Localize(want string) *Info {
	if want == "" || len(x.Translations) == 0 {
		return x
	}
	cands := sortedKeys(x.Translations)
	if x.Language != "" {
		cands = append([]string{x.Language}, cands...) // its own text wins a tie
	}
	key := langtag.Best(want, cands)
	if key == "" || key == x.Language {
		return x
	}
	tr := x.Translations[key]
	if key == "" || tr == nil {
		return x
	}
	y := x.Clone()
	y.Language = key
	for i := range y.Segments {
		if p, ok := tr.Segments[y.Segments[i].ID]; ok {
			y.Segments[i].Label = firstNonEmpty(p.Label, y.Segments[i].Label)
			y.Segments[i].Note = firstNonEmpty(p.Note, y.Segments[i].Note)
		}
	}
	for i := range y.Scenes {
		if p, ok := tr.Scenes[y.Scenes[i].ID]; ok {
			y.Scenes[i].Title = firstNonEmpty(p.Title, p.Label, y.Scenes[i].Title)
			y.Scenes[i].Description = firstNonEmpty(p.Description, y.Scenes[i].Description)
		}
	}
	for i := range y.Filters {
		if p, ok := tr.Filters[y.Filters[i].ID]; ok {
			y.Filters[i].Label = firstNonEmpty(p.Label, y.Filters[i].Label)
		}
	}
	if m := tr.Metadata; m != nil {
		if y.Metadata == nil {
			y.Metadata = &Metadata{}
		}
		d := y.Metadata
		d.Title, d.OriginalTitle = firstNonEmpty(m.Title, d.Title), firstNonEmpty(m.OriginalTitle, d.OriginalTitle)
		d.SortTitle, d.Edition = firstNonEmpty(m.SortTitle, d.SortTitle), firstNonEmpty(m.Edition, d.Edition)
		d.Overview, d.Tagline = firstNonEmpty(m.Overview, d.Overview), firstNonEmpty(m.Tagline, d.Tagline)
		d.Collection = firstNonEmpty(m.Collection, d.Collection)
		if len(m.Genres) > 0 {
			d.Genres = m.Genres
		}
		if len(m.Tags) > 0 {
			d.Tags = m.Tags
		}
		if len(m.Trivia) > 0 {
			d.Trivia = m.Trivia
		}
	}
	y.Notes = firstNonEmpty(tr.Notes, y.Notes)
	if tr.Lyrics != nil && (len(tr.Lyrics.Lines) > 0 || tr.Lyrics.Plain != "") && y.Audio != nil {
		l := *tr.Lyrics
		if l.Language == "" {
			l.Language = key
		}
		y.Audio.Lyrics = &l
	}
	return y
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
