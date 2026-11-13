// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package wittyplayfile reads and writes .wittyplay files: a small, open JSON format that says how
// games are pictured in WittyPlay (box art fitted or filled, or drawn in a cartridge or case, which
// case, and what goes on its label), for everything, per console and per game. The format's open
// specification is docs/specs/wittyplay-file.md (CC0) with its JSON Schema beside it.
//
// A reader keeps what it doesn't know: unknown fields, at any level, are carried through a read and
// a write unchanged, so a file made by a newer program loses nothing passing through an older one.
package wittyplayfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// The format's identity.
const (
	// Version is the one version this reader understands; a file says it in its "wittyplay" field.
	Version = 1
	// Ext is the file extension, MediaType its registered-style media type.
	Ext       = ".wittyplay"
	MediaType = "application/vnd.wittyplay+json"
	// MaxSize is the largest file read, in bytes.
	MaxSize = 64 << 10
	// Limits on what one file holds.
	MaxGames    = 2000
	MaxConsoles = 200
	MaxName     = 200
	MaxText     = 1000
)

// Picture values: how the art is shown.
const (
	PictureFitted = "fitted" // the whole box art, over a blurred, dimmed copy of itself
	PictureFilled = "filled" // the box art fills the tile, cropped to it
	PictureCase   = "case"   // drawn in a cartridge, disc case or the like
)

// Label values: what goes on a case's label.
const (
	LabelBoxArt = "boxart"
	LabelLogo   = "logo"
	LabelTitle  = "title"
)

// CaseAuto lets the console decide the case.
const CaseAuto = "auto"

// CaseStyles are the case drawings this version names. A file may name another (a newer program's);
// a reader that doesn't know it draws the console's own.
var CaseStyles = []string{
	"cart-nes", "cart-atari", "cart-super", "cart-64", "cart-mega", "cart-pocket", "cart-advance", "card-slim",
	"case-jewel", "case-keep", "case-mini", "case-umd", "disk-floppy", "tape-cassette", "arcade-marquee", "generic-box",
}

var (
	pictures  = map[string]bool{PictureFitted: true, PictureFilled: true, PictureCase: true}
	labels    = map[string]bool{LabelBoxArt: true, LabelLogo: true, LabelTitle: true}
	caseID    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	consoleID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
)

// Errors a caller can tell apart.
var (
	ErrTooBig = fmt.Errorf("a .wittyplay file is at most %d KB", MaxSize>>10)
	ErrNewer  = errors.New("this .wittyplay file was made for a newer version of the format")
)

// Extra holds the fields a reader didn't know, as they were.
type Extra map[string]json.RawMessage

// Look is how pictures are shown at one level. Every field is optional: an empty one is decided by
// the level above.
type Look struct {
	Picture string `json:"picture,omitempty"`
	Case    string `json:"case,omitempty"`
	Label   string `json:"label,omitempty"`
	Extra   Extra  `json:"-"`
}

// Empty says nothing is set.
func (l Look) Empty() bool {
	return l.Picture == "" && l.Case == "" && l.Label == "" && len(l.Extra) == 0
}

// Over is l with every field it leaves empty taken from under.
func (l Look) Over(under Look) Look {
	if l.Picture == "" {
		l.Picture = under.Picture
	}
	if l.Case == "" {
		l.Case = under.Case
	}
	if l.Label == "" {
		l.Label = under.Label
	}
	return l
}

// Match says which game an entry is for. Item is a server's own id, meaningful only on the server
// that wrote it; File a ROM's file name; Title and Console a game's name on a console.
type Match struct {
	Item    int64  `json:"item,omitempty"`
	File    string `json:"file,omitempty"`
	Title   string `json:"title,omitempty"`
	Console string `json:"console,omitempty"`
	Extra   Extra  `json:"-"`
}

// Game is one game's look.
type Game struct {
	Match Match `json:"match"`
	Look
}

// Doc is a whole file.
type Doc struct {
	Version     int             `json:"wittyplay"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Generator   string          `json:"generator,omitempty"`
	Default     *Look           `json:"default,omitempty"`
	Consoles    map[string]Look `json:"consoles,omitempty"`
	Games       []Game          `json:"games,omitempty"`
	Extra       Extra           `json:"-"`
}

// Read reads one file from r, refusing anything over MaxSize.
func Read(r io.Reader) (*Doc, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxSize+1))
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Parse reads and checks a file's bytes.
func Parse(b []byte) (*Doc, error) {
	if len(b) > MaxSize {
		return nil, ErrTooBig
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(b) {
		return nil, errors.New("a .wittyplay file is UTF-8 text")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil || top == nil {
		return nil, errors.New("a .wittyplay file is one JSON object")
	}
	d := &Doc{}
	if err := d.fromFields(top); err != nil {
		return nil, err
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return d, nil
}

// Validate checks a document against the format's rules.
func (d *Doc) Validate() error {
	switch {
	case d.Version > Version:
		return ErrNewer
	case d.Version < 1:
		return errors.New(`"wittyplay" must be 1`)
	}
	if utf8.RuneCountInString(d.Name) > MaxName {
		return fmt.Errorf(`"name" is at most %d characters`, MaxName)
	}
	if utf8.RuneCountInString(d.Description) > MaxText || utf8.RuneCountInString(d.Generator) > MaxName {
		return errors.New(`"description" or "generator" is too long`)
	}
	if d.Default != nil {
		if err := d.Default.check("default"); err != nil {
			return err
		}
	}
	if len(d.Consoles) > MaxConsoles {
		return fmt.Errorf("at most %d consoles", MaxConsoles)
	}
	for id, l := range d.Consoles {
		if !consoleID.MatchString(id) {
			return fmt.Errorf("%q is not a console id (lower-case letters, digits, - and _)", id)
		}
		if err := l.check("consoles." + id); err != nil {
			return err
		}
	}
	if len(d.Games) > MaxGames {
		return fmt.Errorf("at most %d games", MaxGames)
	}
	for i, g := range d.Games {
		where := fmt.Sprintf("games[%d]", i)
		if err := g.Match.check(where); err != nil {
			return err
		}
		if err := g.Look.check(where); err != nil {
			return err
		}
	}
	return nil
}

func (l Look) check(where string) error {
	if l.Picture != "" && !pictures[l.Picture] {
		return fmt.Errorf(`%s: "picture" is fitted, filled or case`, where)
	}
	if l.Label != "" && !labels[l.Label] {
		return fmt.Errorf(`%s: "label" is boxart, logo or title`, where)
	}
	if l.Case != "" && !caseID.MatchString(l.Case) {
		return fmt.Errorf(`%s: "case" is a case style id`, where)
	}
	return nil
}

func (m Match) check(where string) error {
	if m.Item == 0 && m.File == "" && m.Title == "" {
		return fmt.Errorf(`%s: "match" names an item, a file or a title`, where)
	}
	if m.Item < 0 {
		return fmt.Errorf(`%s: "item" is a positive number`, where)
	}
	if strings.ContainsAny(m.File, `/\`) || m.File == "." || m.File == ".." {
		return fmt.Errorf(`%s: "file" is a file name, without a folder`, where)
	}
	if utf8.RuneCountInString(m.File) > MaxName || utf8.RuneCountInString(m.Title) > MaxName {
		return fmt.Errorf(`%s: a name is at most %d characters`, where, MaxName)
	}
	if m.Console != "" && !consoleID.MatchString(m.Console) {
		return fmt.Errorf(`%s: "console" is a console id`, where)
	}
	return nil
}

// KnownCase says the case style is one this version draws.
func KnownCase(id string) bool {
	for _, c := range CaseStyles {
		if c == id {
			return true
		}
	}
	return false
}

// For is the look a document gives one game: its default, then its console's, then each entry
// that matches, a later entry over an earlier one. A file name or title is matched
// without regard to case.
func (d *Doc) For(item int64, console, file, title string) Look {
	var out Look
	for i := len(d.Games) - 1; i >= 0; i-- {
		if d.Games[i].Match.Matches(item, console, file, title) {
			out = out.Over(d.Games[i].Look)
		}
	}
	if c, ok := d.Consoles[console]; ok && console != "" {
		out = out.Over(c)
	}
	if d.Default != nil {
		out = out.Over(*d.Default)
	}
	out.Extra = nil
	return out
}

// Matches says the entry is for this game: every field it gives agrees.
func (m Match) Matches(item int64, console, file, title string) bool {
	if m.Item == 0 && m.File == "" && m.Title == "" {
		return false
	}
	if m.Item != 0 && m.Item != item {
		return false
	}
	if m.File != "" && !strings.EqualFold(m.File, file) {
		return false
	}
	if m.Title != "" && !strings.EqualFold(strings.TrimSpace(m.Title), strings.TrimSpace(title)) {
		return false
	}
	return m.Console == "" || m.Console == console
}

// ---- reading and writing, keeping what isn't known ----

func take[T any](f map[string]json.RawMessage, key string, into *T) error {
	raw, ok := f[key]
	if !ok {
		return nil
	}
	delete(f, key)
	if string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("%q has the wrong type", key)
	}
	return nil
}

func rest(f map[string]json.RawMessage) Extra {
	if len(f) == 0 {
		return nil
	}
	return Extra(f)
}

func (d *Doc) fromFields(f map[string]json.RawMessage) error {
	raw, ok := f["wittyplay"]
	if !ok {
		return errors.New(`a .wittyplay file starts with "wittyplay": 1`)
	}
	var v json.Number
	if len(raw) == 0 || raw[0] == '"' {
		return errors.New(`"wittyplay" is the format's version number`)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return errors.New(`"wittyplay" is the format's version number`)
	}
	n, err := v.Int64()
	if err != nil || n > 1<<20 {
		return errors.New(`"wittyplay" is a whole number`)
	}
	delete(f, "wittyplay")
	d.Version = int(n)
	if d.Version > Version {
		return ErrNewer
	}
	for _, k := range []struct {
		key string
		to  *string
	}{{"name", &d.Name}, {"description", &d.Description}, {"generator", &d.Generator}} {
		if err := take(f, k.key, k.to); err != nil {
			return err
		}
	}
	if err := take(f, "default", &d.Default); err != nil {
		return err
	}
	if err := take(f, "consoles", &d.Consoles); err != nil {
		return err
	}
	if err := take(f, "games", &d.Games); err != nil {
		return err
	}
	d.Extra = rest(f)
	return nil
}

func (l *Look) UnmarshalJSON(b []byte) error {
	var f map[string]json.RawMessage
	if err := json.Unmarshal(b, &f); err != nil {
		return errors.New("a look is an object")
	}
	*l = Look{}
	for _, k := range []struct {
		key string
		to  *string
	}{{"picture", &l.Picture}, {"case", &l.Case}, {"label", &l.Label}} {
		if err := take(f, k.key, k.to); err != nil {
			return err
		}
	}
	l.Extra = rest(f)
	return nil
}

func (l Look) fields() map[string]any {
	out := map[string]any{}
	for k, v := range l.Extra {
		out[k] = v
	}
	put(out, "picture", l.Picture)
	put(out, "case", l.Case)
	put(out, "label", l.Label)
	return out
}

func (l Look) MarshalJSON() ([]byte, error) { return marshalSorted(l.fields()) }

func (m *Match) UnmarshalJSON(b []byte) error {
	var f map[string]json.RawMessage
	if err := json.Unmarshal(b, &f); err != nil {
		return errors.New(`"match" is an object`)
	}
	*m = Match{}
	if err := take(f, "item", &m.Item); err != nil {
		return err
	}
	for _, k := range []struct {
		key string
		to  *string
	}{{"file", &m.File}, {"title", &m.Title}, {"console", &m.Console}} {
		if err := take(f, k.key, k.to); err != nil {
			return err
		}
	}
	m.Extra = rest(f)
	return nil
}

func (m Match) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	for k, v := range m.Extra {
		out[k] = v
	}
	if m.Item != 0 {
		out["item"] = m.Item
	}
	put(out, "file", m.File)
	put(out, "title", m.Title)
	put(out, "console", m.Console)
	return marshalSorted(out)
}

func (g *Game) UnmarshalJSON(b []byte) error {
	var f map[string]json.RawMessage
	if err := json.Unmarshal(b, &f); err != nil {
		return errors.New("a game is an object")
	}
	*g = Game{}
	raw, ok := f["match"]
	if !ok {
		return errors.New(`a game has a "match"`)
	}
	delete(f, "match")
	if err := json.Unmarshal(raw, &g.Match); err != nil {
		return err
	}
	rest, _ := json.Marshal(f)
	return json.Unmarshal(rest, &g.Look)
}

func (g Game) MarshalJSON() ([]byte, error) {
	out := g.Look.fields()
	out["match"] = g.Match
	return marshalSorted(out)
}

// MarshalJSON writes the document with "wittyplay" first, so a person opening it sees what it is.
func (d Doc) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	for k, v := range d.Extra {
		out[k] = v
	}
	put(out, "name", d.Name)
	put(out, "description", d.Description)
	put(out, "generator", d.Generator)
	if d.Default != nil {
		out["default"] = *d.Default
	}
	if len(d.Consoles) > 0 {
		out["consoles"] = d.Consoles
	}
	if len(d.Games) > 0 {
		out["games"] = d.Games
	}
	body, err := marshalSorted(out)
	if err != nil {
		return nil, err
	}
	head := fmt.Sprintf(`{"wittyplay":%d`, d.Version)
	if len(body) > 2 {
		return []byte(head + "," + string(body[1:])), nil
	}
	return []byte(head + "}"), nil
}

// Encode writes the document as a file: indented, ending in a new line.
func (d *Doc) Encode() ([]byte, error) {
	b, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, b, "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

func put(m map[string]any, k, v string) {
	if v != "" {
		m[k] = v
	}
}

// marshalSorted writes an object's keys in order, so the same document is always the same bytes.
func marshalSorted(m map[string]any) ([]byte, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		b.Write(kb)
		b.WriteByte(':')
		vb, err := json.Marshal(m[k])
		if err != nil {
			return nil, err
		}
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}
