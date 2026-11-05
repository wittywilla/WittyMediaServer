package library

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// A Title is a film (an item) or a show, handled the same way for editing: one
// list of fields, each with its value, where the value came from, and whether a
// person owns it. A locked field is never changed by a scan, a refresh, a new
// match, an NFO or a sidecar, matched or not. Only a person can change it.

// Targets.
const (
	TargetItem = "item"
	TargetShow = "show"
)

// What's been decided about a title's match.
const (
	MatchAuto   = ""       // whatever the scan found
	MatchManual = "manual" // a person chose it: lookups leave it alone
	MatchNone   = "none"   // a person said it has no match: nothing is looked up for it
)

// SourceYou marks what a person typed.
const SourceYou = "you"

// Where a value can come from besides a metadata service.
const (
	SourceFile  = "file"
	SourceNFO   = "nfo"
	SourceLocal = "local"
	SourceFrame = "frame"
	SourceProbe = "probe"
	// SourceSaved: an NFO this server wrote itself earlier. It says what some match said
	// back then, so it's no more a person's work than that match was.
	SourceSaved = "saved"
)

// TitleMeta is a title's row in title_meta.
type TitleMeta struct {
	Locks       map[string]bool
	Sources     map[string]string
	Baseline    map[string]Baseline
	MatchState  string
	Suggestions []MatchCandidate
	SuggestedAt int64
	Director    string
	Edition     []string
	Tags        []string
	Banner      string
	Logo        string
	AudioFormat string
}

// Baseline is what a source said before someone typed over it, for Revert.
type Baseline struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

func newTitleMeta() *TitleMeta {
	return &TitleMeta{Locks: map[string]bool{}, Sources: map[string]string{}, Baseline: map[string]Baseline{}, Edition: []string{}, Tags: []string{}}
}

// TitleMeta reads a title's row (never nil).
func (s *Store) TitleMeta(ctx context.Context, target string, id int64) *TitleMeta {
	return scanTitleMeta(s.DB.QueryRowContext(ctx, `SELECT `+titleMetaCols+` FROM title_meta WHERE target = ? AND target_id = ?`, target, id))
}

const titleMetaCols = `locks, sources, baseline, match_state, suggestions, suggested_at, director, edition, tags, banner, logo, audio_format`

func scanTitleMeta(row interface{ Scan(...any) error }, first ...any) *TitleMeta {
	m := newTitleMeta()
	var locks, sources, baseline, suggestions, edition, tags string
	err := row.Scan(append(first, &locks, &sources, &baseline, &m.MatchState, &suggestions, &m.SuggestedAt, &m.Director, &edition, &tags, &m.Banner, &m.Logo, &m.AudioFormat)...)
	if err != nil {
		return m
	}
	for _, f := range splitList(locks) {
		m.Locks[f] = true
	}
	_ = json.Unmarshal([]byte(sources), &m.Sources)
	_ = json.Unmarshal([]byte(baseline), &m.Baseline)
	if suggestions != "" {
		_ = json.Unmarshal([]byte(suggestions), &m.Suggestions)
	}
	m.Edition, m.Tags = splitList(edition), splitList(tags)
	if m.Sources == nil {
		m.Sources = map[string]string{}
	}
	if m.Baseline == nil {
		m.Baseline = map[string]Baseline{}
	}
	return m
}

func (s *Store) saveTitleMeta(ctx context.Context, target string, id int64, m *TitleMeta) error {
	locks := make([]string, 0, len(m.Locks))
	for f, on := range m.Locks {
		if on {
			locks = append(locks, f)
		}
	}
	sort.Strings(locks)
	sources, _ := json.Marshal(m.Sources)
	baseline, _ := json.Marshal(m.Baseline)
	suggestions := ""
	if len(m.Suggestions) > 0 {
		b, _ := json.Marshal(m.Suggestions)
		suggestions = string(b)
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO title_meta(target, target_id, locks, sources, baseline, match_state, suggestions, suggested_at, director, edition, tags, banner, logo, audio_format)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(target, target_id) DO UPDATE SET locks = excluded.locks, sources = excluded.sources, baseline = excluded.baseline,
		match_state = excluded.match_state, suggestions = excluded.suggestions, suggested_at = excluded.suggested_at, director = excluded.director, edition = excluded.edition,
		tags = excluded.tags, banner = excluded.banner, logo = excluded.logo, audio_format = excluded.audio_format`,
		target, id, joinList(locks), string(sources), string(baseline), m.MatchState, suggestions, m.SuggestedAt, m.Director, joinList(m.Edition), joinList(m.Tags), m.Banner, m.Logo, m.AudioFormat)
	return err
}

// Title is one film or show with what's kept about it.
type Title struct {
	Target  string
	ID      int64
	Item    *Item
	Show    *Show
	Meta    *TitleMeta
	Studios []string
	// Series is the film series it belongs to, which makes a collection by itself.
	Series string
}

// LoadTitle reads a title, whichever kind it is.
func (s *Store) LoadTitle(ctx context.Context, target string, id int64) (*Title, error) {
	t := &Title{Target: target, ID: id}
	switch target {
	case TargetItem:
		it, err := s.Item(ctx, id)
		if err != nil {
			return nil, err
		}
		t.Item = it
	case TargetShow:
		sh, err := s.Show(ctx, id, 0)
		if err != nil {
			return nil, err
		}
		t.Show = sh
	default:
		return nil, ErrNotFound
	}
	t.Meta = s.TitleMeta(ctx, target, id)
	t.Studios = s.TitleBrands(ctx, target, id, LogoStudio)
	if len(t.Studios) == 0 && t.Item != nil && t.Item.Studio != "" {
		t.Studios = []string{t.Item.Studio}
	}
	if list := s.TitleBrands(ctx, target, id, BrandSeries); len(list) > 0 {
		t.Series = list[0]
	}
	return t, nil
}

// LibraryID is the library a title is in.
func (t *Title) LibraryID() int64 {
	if t.Item != nil {
		return t.Item.LibraryID
	}
	return t.Show.LibraryID
}

// Name is what the title is called now.
func (t *Title) Name() string {
	if t.Item != nil {
		return t.Item.Title
	}
	return t.Show.Title
}

// Provider is the service and id of its match.
func (t *Title) Provider() (string, string) {
	if t.Item != nil {
		return t.Item.Provider, t.Item.ProviderID
	}
	return t.Show.Provider, t.Show.ProviderID
}

func (t *Title) setProvider(source, id string) {
	if t.Item != nil {
		t.Item.Provider, t.Item.ProviderID = source, id
		return
	}
	t.Show.Provider, t.Show.ProviderID = source, id
}

// Path is the file or folder of the title.
func (t *Title) Path() string {
	if t.Item != nil {
		return t.Item.Path
	}
	return t.Show.Path
}

// The fields a person can see and edit, in the order the edit dialog shows them.
var (
	titleItemFields = []string{"title", "sortTitle", "year", "director", "edition", "series", "tagline", "overview", "genres", "tags", "contentRating", "rating", "studios", "poster", "backdrop", "banner", "logo", "introEnd"}
	titleShowFields = []string{"title", "sortTitle", "year", "director", "overview", "genres", "tags", "contentRating", "rating", "network", "studios", "status", "poster", "backdrop", "banner", "logo", "introStart", "introEnd"}
	// A season and episode number belong to an episode alone.
	titleEpisodeFields = []string{"title", "sortTitle", "overview", "contentRating", "rating", "poster", "season", "episode", "episodeEnd", "aired", "introStart", "introEnd"}
)

// FieldNames lists a title's fields.
func (t *Title) FieldNames() []string {
	if t.Show != nil {
		return titleShowFields
	}
	if t.Item.ShowID != nil {
		return titleEpisodeFields
	}
	return titleItemFields
}

func (t *Title) hasField(f string) bool {
	for _, n := range t.FieldNames() {
		if n == f {
			return true
		}
	}
	return false
}

func intStr(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

func floatStr(p *float64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatFloat(*p, 'f', -1, 64)
}

// Field reads a field as text; a list is joined with "|".
func (t *Title) Field(f string) string {
	m := t.Meta
	switch f {
	case "director":
		return m.Director
	case "edition":
		return joinList(m.Edition)
	case "tags":
		return joinList(m.Tags)
	case "banner":
		return m.Banner
	case "logo":
		return m.Logo
	case "studios":
		return joinList(t.Studios)
	case "series":
		return t.Series
	}
	if it := t.Item; it != nil {
		switch f {
		case "title":
			return it.Title
		case "sortTitle":
			return it.SortTitle
		case "year":
			return intStr(it.Year)
		case "overview":
			return it.Overview
		case "tagline":
			return it.Tagline
		case "genres":
			return joinList(it.Genres)
		case "contentRating":
			return it.ContentRating
		case "rating":
			return floatStr(it.Rating)
		case "poster":
			return it.Poster
		case "backdrop":
			return it.Backdrop
		case "introStart":
			return floatStr(it.IntroStart)
		case "introEnd":
			return floatStr(it.IntroEnd)
		case "season":
			return intStr(it.Season)
		case "episode":
			return intStr(it.Episode)
		case "episodeEnd":
			return intStr(it.EpisodeEnd)
		case "aired":
			return it.Aired
		}
		return ""
	}
	sh := t.Show
	switch f {
	case "title":
		return sh.Title
	case "sortTitle":
		return sh.SortTitle
	case "year":
		return intStr(sh.Year)
	case "overview":
		return sh.Overview
	case "genres":
		return joinList(sh.Genres)
	case "contentRating":
		return sh.ContentRating
	case "rating":
		return floatStr(sh.Rating)
	case "network":
		return sh.Network
	case "status":
		return sh.Status
	case "poster":
		return sh.Poster
	case "backdrop":
		return sh.Backdrop
	case "introStart":
		return floatStr(sh.IntroStart)
	case "introEnd":
		return floatStr(sh.IntroEnd)
	}
	return ""
}

// cleanText takes control characters out of text from outside (line breaks stay
// where a field can have them) and limits its length.
func cleanText(s string, n int, lines bool) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' && lines {
			return r
		}
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			if r == '\t' || r == '\n' || r == '\r' {
				return ' '
			}
			return -1
		}
		return r
	}, s)
	return clip(strings.TrimSpace(s), n)
}

// cleanList tidies a list: no empties, no repeats (whatever the case), bounded.
func cleanList(items []string, maxItems, maxLen int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range items {
		// "|" separates a list where it's stored.
		v = strings.ReplaceAll(cleanText(v, maxLen, false), "|", "/")
		k := strings.ToLower(v)
		if v == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
		if len(out) == maxItems {
			break
		}
	}
	return out
}

func parseOptInt(v string, lo, hi int) (*int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		return nil, errors.New("not a number in range: " + v)
	}
	return &n, nil
}

func parseOptFloat(v string, lo, hi float64) (*float64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || n != n || n < lo || n > hi {
		return nil, errors.New("not a number in range: " + v)
	}
	return &n, nil
}

// SetField writes a field from text. Everything going into a field passes through
// here, from a person or a source, so nothing unbounded or odd reaches the database.
func (t *Title) SetField(f, v string) error {
	list := func(maxItems, maxLen int) []string { return cleanList(strings.Split(v, "|"), maxItems, maxLen) }
	text := func(n int) string { return cleanText(v, n, false) }
	m := t.Meta
	switch f {
	case "director":
		m.Director = text(300)
		return nil
	case "edition":
		m.Edition = list(8, 60)
		return nil
	case "tags":
		m.Tags = list(50, 60)
		return nil
	case "series":
		t.Series = text(120)
		return nil
	case "banner":
		m.Banner = text(2000)
		return nil
	case "logo":
		m.Logo = text(2000)
		return nil
	case "studios":
		t.Studios = list(12, 120)
		if t.Item != nil {
			t.Item.Studio = ""
			if len(t.Studios) > 0 {
				t.Item.Studio = t.Studios[0]
			}
		}
		return nil
	case "title":
		n := text(300)
		if n == "" {
			return errors.New("a title can't be empty")
		}
		v = n
	case "sortTitle":
		v = text(300)
		if v == "" {
			v = SortTitle(t.Field("title"))
		}
	}
	if it := t.Item; it != nil {
		switch f {
		case "title":
			it.Title = v
		case "sortTitle":
			it.SortTitle = v
		case "year":
			y, err := parseOptInt(v, 1800, 3000)
			if err != nil {
				return errors.New("a year must be between 1800 and 3000")
			}
			it.Year = y
		case "overview":
			it.Overview = cleanText(v, 20000, true)
		case "tagline":
			it.Tagline = text(500)
		case "genres":
			it.Genres = list(30, 60)
		case "contentRating":
			it.ContentRating = text(20)
		case "rating":
			r, err := parseOptFloat(v, 0, 10)
			if err != nil {
				return errors.New("a rating must be from 0 to 10")
			}
			it.Rating = r
		case "poster":
			it.Poster = text(2000)
		case "backdrop":
			it.Backdrop = text(2000)
		case "introStart":
			p, err := parseOptFloat(v, 0, 86400)
			if err != nil {
				return errors.New("that isn't a time")
			}
			it.IntroStart = p
		case "introEnd":
			p, err := parseOptFloat(v, 0, 86400)
			if err != nil {
				return errors.New("that isn't a time")
			}
			it.IntroEnd = p
			if p != nil && it.IntroStart == nil {
				zero := 0.0
				it.IntroStart = &zero
			}
			if p == nil {
				it.IntroStart = nil
			}
		case "season":
			p, err := parseOptInt(v, 0, 9999)
			if err != nil {
				return errors.New("a season must be a number")
			}
			it.Season = p
		case "episode":
			p, err := parseOptInt(v, 0, 99999)
			if err != nil {
				return errors.New("an episode must be a number")
			}
			it.Episode = p
		case "episodeEnd":
			p, err := parseOptInt(v, 0, 99999)
			if err != nil {
				return errors.New("an episode must be a number")
			}
			it.EpisodeEnd = p
		case "aired":
			it.Aired = text(20)
		default:
			return errors.New("unknown field: " + f)
		}
		return nil
	}
	sh := t.Show
	switch f {
	case "title":
		sh.Title = v
	case "sortTitle":
		sh.SortTitle = v
	case "year":
		y, err := parseOptInt(v, 1800, 3000)
		if err != nil {
			return errors.New("a year must be between 1800 and 3000")
		}
		sh.Year = y
	case "overview":
		sh.Overview = cleanText(v, 20000, true)
	case "genres":
		sh.Genres = list(30, 60)
	case "contentRating":
		sh.ContentRating = text(20)
	case "rating":
		r, err := parseOptFloat(v, 0, 10)
		if err != nil {
			return errors.New("a rating must be from 0 to 10")
		}
		sh.Rating = r
	case "network":
		sh.Network = text(120)
	case "status":
		sh.Status = text(40)
	case "poster":
		sh.Poster = text(2000)
	case "backdrop":
		sh.Backdrop = text(2000)
	case "introStart":
		p, err := parseOptFloat(v, 0, 86400)
		if err != nil {
			return errors.New("that isn't a time")
		}
		sh.IntroStart = p
	case "introEnd":
		p, err := parseOptFloat(v, 0, 86400)
		if err != nil {
			return errors.New("that isn't a time")
		}
		sh.IntroEnd = p
		if p != nil && sh.IntroStart == nil {
			zero := 0.0
			sh.IntroStart = &zero
		}
		if p == nil {
			sh.IntroStart = nil
		}
	default:
		return errors.New("unknown field: " + f)
	}
	return nil
}

// SaveTitle stores a title and everything kept about it.
func (s *Store) SaveTitle(ctx context.Context, t *Title) error {
	var err error
	if t.Item != nil {
		err = s.upsertItem(ctx, t.Item)
		if err == nil && t.Item.ShowID == nil {
			err = s.setTitleBrands(ctx, TargetItem, t.ID, t.Item.LibraryID, LogoStudio, t.Studios)
		}
		if err == nil && t.Item.ShowID == nil {
			err = s.setTitleBrands(ctx, TargetItem, t.ID, t.Item.LibraryID, BrandSeries, []string{t.Series})
		}
	} else {
		err = s.UpsertShow(ctx, t.Show)
		if err == nil {
			err = s.setTitleBrands(ctx, TargetShow, t.ID, t.Show.LibraryID, LogoStudio, t.Studios)
		}
		if err == nil {
			err = s.setTitleBrands(ctx, TargetShow, t.ID, t.Show.LibraryID, LogoNetwork, []string{t.Show.Network})
		}
	}
	if err != nil {
		return err
	}
	return s.saveTitleMeta(ctx, t.Target, t.ID, t.Meta)
}

// sourceOf says where a field's value came from. Titles scanned before this was kept
// say what can be worked out: their match, or the file's own name.
func (t *Title) sourceOf(f string) string {
	if v, ok := t.Meta.Sources[f]; ok {
		return v
	}
	v := t.Field(f)
	if v == "" {
		return ""
	}
	provider, _ := t.Provider()
	switch f {
	case "poster", "backdrop", "banner", "logo":
		switch {
		case strings.HasPrefix(v, "cache:thumb-") || v == LazyPoster:
			return SourceFrame
		case strings.HasPrefix(v, "cache:made-") || strings.HasPrefix(v, "cache:auto-"):
			return SourceMade
		case isRemote(v) || strings.HasPrefix(v, "cache:"):
			if provider != "" {
				return provider
			}
			return ""
		}
		return SourceLocal
	case "title", "sortTitle", "year":
		if provider == "" {
			return SourceFile
		}
	case "season", "episode", "episodeEnd":
		return SourceFile
	}
	return provider
}

// IsServiceSource: the value came from a metadata service, so a new match replaces it.
func isServiceSource(src string) bool {
	switch src {
	case "", SourceYou, SourceFile, SourceNFO, SourceLocal, SourceFrame, SourceMade, SourceProbe:
		return false
	}
	return true
}

// EditFields applies what a person typed. Each changed field becomes theirs: it
// remembers what the source said, for Revert, and is locked unless they say not to.
func (t *Title) EditFields(changes map[string]string, lock bool) error {
	names := make([]string, 0, len(changes))
	for f := range changes {
		names = append(names, f)
	}
	sort.Strings(names)
	for _, f := range names {
		if !t.hasField(f) {
			return errors.New("this title has no field called " + f)
		}
		old := t.Field(f)
		if err := t.SetField(f, changes[f]); err != nil {
			return err
		}
		if t.Field(f) == old {
			continue
		}
		src := t.sourceOf(f)
		if _, has := t.Meta.Baseline[f]; !has && src != SourceYou {
			// sourceOf reads the current value, so ask about the old one.
			t.Meta.Baseline[f] = Baseline{Value: old, Source: t.oldSource(f, old)}
		}
		t.Meta.Sources[f] = SourceYou
		if lock {
			t.Meta.Locks[f] = true
		}
	}
	// A new title files under its new name, unless someone has said otherwise.
	if _, set := changes["sortTitle"]; !set && changes["title"] != "" && !t.Meta.Locks["sortTitle"] {
		_ = t.SetField("sortTitle", SortTitle(t.Field("title")))
	}
	return nil
}

func (t *Title) oldSource(f, old string) string {
	if v, ok := t.Meta.Sources[f]; ok {
		return v
	}
	cur := t.Field(f)
	if err := t.SetField(f, old); err != nil {
		return ""
	}
	src := t.sourceOf(f)
	_ = t.SetField(f, cur)
	return src
}

// RevertField goes back to what the source said, and unlocks the field.
func (t *Title) RevertField(f string) error {
	if !t.hasField(f) {
		return errors.New("this title has no field called " + f)
	}
	delete(t.Meta.Locks, f)
	b, ok := t.Meta.Baseline[f]
	if !ok {
		return nil
	}
	if err := t.SetField(f, b.Value); err != nil {
		return err
	}
	if b.Source == "" {
		delete(t.Meta.Sources, f)
	} else {
		t.Meta.Sources[f] = b.Source
	}
	delete(t.Meta.Baseline, f)
	return nil
}

// ---------------------------------------------------------------------------
// Keeping what's locked through a scan
// ---------------------------------------------------------------------------

// keepLockedItem puts locked fields back on an item a scan has rebuilt, and the whole
// match if a person chose it or said there is none. The scan may have read an NFO, or
// a sidecar, or asked a service.
func (s *Store) keepLockedItem(ctx context.Context, it, prev *Item, m *TitleMeta) {
	if prev == nil {
		return
	}
	from := &Title{Target: TargetItem, ID: prev.ID, Item: prev, Meta: m}
	to := &Title{Target: TargetItem, ID: prev.ID, Item: it, Meta: m}
	for f, on := range m.Locks {
		if on {
			_ = to.SetField(f, from.Field(f))
		}
	}
	switch m.MatchState {
	case MatchManual:
		it.Provider, it.ProviderID = prev.Provider, prev.ProviderID
		// A better copy of the same film: everything the match gave stays.
		for f, src := range m.Sources {
			if isServiceSource(src) {
				_ = to.SetField(f, from.Field(f))
			}
		}
	case MatchNone:
		it.Provider, it.ProviderID = "", ""
	}
}

// keepLockedShow does the same for a show.
func keepLockedShow(sh, prev *Show, m *TitleMeta) {
	from := &Title{Target: TargetShow, ID: prev.ID, Show: prev, Meta: m}
	to := &Title{Target: TargetShow, ID: prev.ID, Show: sh, Meta: m}
	for f, on := range m.Locks {
		if on {
			_ = to.SetField(f, from.Field(f))
		}
	}
	switch m.MatchState {
	case MatchManual:
		sh.Provider, sh.ProviderID = prev.Provider, prev.ProviderID
		for f, src := range m.Sources {
			if isServiceSource(src) {
				_ = to.SetField(f, from.Field(f))
			}
		}
	case MatchNone:
		sh.Provider, sh.ProviderID = "", ""
	}
}

// provenance works out where each field of an item came from while a scan fills it
// in: after each step, whatever changed came from that step.
type provenance struct {
	last    map[string]string
	sources map[string]string
}

var scanFields = []string{"title", "sortTitle", "year", "overview", "tagline", "genres", "contentRating", "rating", "studios", "poster", "backdrop", "introStart", "introEnd", "season", "episode", "episodeEnd", "aired"}

func newProvenance() *provenance {
	return &provenance{last: map[string]string{}, sources: map[string]string{}}
}

func (p *provenance) note(it *Item, source string) {
	t := &Title{Target: TargetItem, Item: it, Meta: newTitleMeta()}
	if it.Studio != "" {
		t.Studios = []string{it.Studio}
	}
	for _, f := range scanFields {
		v := t.Field(f)
		if v != p.last[f] {
			p.last[f] = v
			if v == "" {
				delete(p.sources, f)
			} else {
				p.sources[f] = source
			}
		}
	}
}

// recordScan keeps where a scan's values came from, in the title's row, leaving a
// person's own fields as they are.
func (s *Store) recordScan(ctx context.Context, id int64, m *TitleMeta, p *provenance) {
	if id == 0 {
		return
	}
	for f, src := range p.sources {
		if m.Locks[f] || m.Sources[f] == SourceYou {
			continue
		}
		if m.MatchState == MatchManual && isServiceSource(m.Sources[f]) {
			continue
		}
		m.Sources[f] = src
	}
	_ = s.saveTitleMeta(ctx, TargetItem, id, m)
}

// ---------------------------------------------------------------------------
// History, for Undo
// ---------------------------------------------------------------------------

// titleSnapshot is everything a change to a title could touch.
type titleSnapshot struct {
	Fields     map[string]string   `json:"fields"`
	Provider   string              `json:"provider"`
	ProviderID string              `json:"providerId"`
	MatchState string              `json:"matchState"`
	Locks      map[string]bool     `json:"locks"`
	Sources    map[string]string   `json:"sources"`
	Baseline   map[string]Baseline `json:"baseline"`
}

func (t *Title) snapshot() titleSnapshot {
	fields := map[string]string{}
	for _, f := range t.FieldNames() {
		fields[f] = t.Field(f)
	}
	provider, id := t.Provider()
	locks := map[string]bool{}
	for k, v := range t.Meta.Locks {
		locks[k] = v
	}
	sources := map[string]string{}
	for k, v := range t.Meta.Sources {
		sources[k] = v
	}
	baseline := map[string]Baseline{}
	for k, v := range t.Meta.Baseline {
		baseline[k] = v
	}
	return titleSnapshot{Fields: fields, Provider: provider, ProviderID: id, MatchState: t.Meta.MatchState, Locks: locks, Sources: sources, Baseline: baseline}
}

func (t *Title) restore(snap titleSnapshot) {
	for f, v := range snap.Fields {
		_ = t.SetField(f, v)
	}
	t.setProvider(snap.Provider, snap.ProviderID)
	t.Meta.MatchState, t.Meta.Locks, t.Meta.Sources, t.Meta.Baseline = snap.MatchState, snap.Locks, snap.Sources, snap.Baseline
}

// NewBatch names a group of changes that are undone together.
func NewBatch() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// maxHistory bounds how much is kept for Undo.
const maxHistory = 5000

// Remember keeps a title's state before a change.
func (s *Store) Remember(ctx context.Context, batch, user, action string, t *Title, before titleSnapshot) error {
	b, err := json.Marshal(before)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO meta_history(batch, at, user_name, action, target, target_id, title, before_json) VALUES(?,?,?,?,?,?,?,?)`,
		batch, db.Now(), user, action, t.Target, t.ID, t.Name(), string(b))
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM meta_history WHERE id <= (SELECT MAX(id) FROM meta_history) - ?`, maxHistory)
	return err
}

// HistoryEntry is a group of changes that can be undone.
type HistoryEntry struct {
	Batch  string `json:"batch"`
	At     int64  `json:"at"`
	User   string `json:"user"`
	Action string `json:"action"`
	Title  string `json:"title"`
	Count  int    `json:"count"`
	Undone bool   `json:"undone"`
}

// History lists recent groups of changes, newest first.
func (s *Store) History(ctx context.Context, limit int) ([]HistoryEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT batch, MAX(at), MIN(user_name), MIN(action), MIN(title), COUNT(*), MIN(undone) FROM meta_history GROUP BY batch ORDER BY MAX(id) DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoryEntry{}
	for rows.Next() {
		var e HistoryEntry
		var undone int
		if err := rows.Scan(&e.Batch, &e.At, &e.User, &e.Action, &e.Title, &e.Count, &undone); err != nil {
			return nil, err
		}
		e.Undone = undone != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// ErrAlreadyUndone: this group of changes has been undone.
var ErrAlreadyUndone = errors.New("that has already been undone")

// Undo puts back a group of changes, newest first. It returns how many titles
// changed. Anything since deleted is skipped.
func (s *Store) Undo(ctx context.Context, batch string) (int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT target, target_id, before_json, undone FROM meta_history WHERE batch = ? ORDER BY id DESC`, batch)
	if err != nil {
		return 0, err
	}
	type entry struct {
		target string
		id     int64
		before string
	}
	var list []entry
	undone := false
	for rows.Next() {
		var e entry
		var u int
		if err := rows.Scan(&e.target, &e.id, &e.before, &u); err != nil {
			rows.Close()
			return 0, err
		}
		undone = undone || u != 0
		list = append(list, e)
	}
	rows.Close()
	if len(list) == 0 {
		return 0, ErrNotFound
	}
	if undone {
		return 0, ErrAlreadyUndone
	}
	n := 0
	for _, e := range list {
		t, err := s.LoadTitle(ctx, e.target, e.id)
		if err != nil {
			continue
		}
		var snap titleSnapshot
		if err := json.Unmarshal([]byte(e.before), &snap); err != nil {
			continue
		}
		t.restore(snap)
		if err := s.SaveTitle(ctx, t); err != nil {
			return n, err
		}
		s.forgetListing(ctx, e.target, e.id)
		n++
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE meta_history SET undone = 1 WHERE batch = ?`, batch)
	return n, err
}

// ---------------------------------------------------------------------------
// Links: extras and versions
// ---------------------------------------------------------------------------

// The kinds of attachment.
const (
	LinkExtra   = "extra"
	LinkVersion = "version"
)

// ExtraTypes are the kinds of bonus feature.
var ExtraTypes = []string{"trailer", "featurette", "behind-the-scenes", "deleted-scene", "interview", "short", "other"}

// ItemLink says what an item is attached to.
type ItemLink struct {
	Link       string `json:"link"`
	ExtraType  string `json:"extraType,omitempty"`
	ParentItem *int64 `json:"parentItem,omitempty"`
	ParentShow *int64 `json:"parentShow,omitempty"`
	Label      string `json:"label,omitempty"`
}

// LinkOf reads an item's attachment, or nil.
func (s *Store) LinkOf(ctx context.Context, id int64) *ItemLink {
	var l ItemLink
	var pi, ps sql.NullInt64
	if err := s.DB.QueryRowContext(ctx, `SELECT link, extra_type, parent_item, parent_show, label FROM item_links WHERE item_id = ?`, id).Scan(&l.Link, &l.ExtraType, &pi, &ps, &l.Label); err != nil {
		return nil
	}
	if pi.Valid {
		l.ParentItem = &pi.Int64
	}
	if ps.Valid {
		l.ParentShow = &ps.Int64
	}
	return &l
}

// AttachItem makes an item an extra or a version of a film or show. A title can't
// attach to itself, or to something that's attached itself: there's one level.
func (s *Store) AttachItem(ctx context.Context, id int64, link, extraType, label string, parentItem, parentShow *int64) error {
	if (parentItem == nil) == (parentShow == nil) {
		return errors.New("choose one film or show to attach it to")
	}
	it, err := s.Item(ctx, id)
	if err != nil {
		return err
	}
	if it.ShowID != nil && link == LinkVersion {
		return errors.New("an episode can't be a version of a film")
	}
	switch link {
	case LinkExtra:
		ok := false
		for _, e := range ExtraTypes {
			ok = ok || e == extraType
		}
		if !ok {
			extraType = "other"
		}
	case LinkVersion:
		extraType = ""
		if parentShow != nil {
			return errors.New("a version belongs to a film")
		}
	default:
		return errors.New("unknown link")
	}
	if parentItem != nil {
		if *parentItem == id {
			return errors.New("a title can't be attached to itself")
		}
		p, err := s.Item(ctx, *parentItem)
		if err != nil {
			return err
		}
		if p.ShowID != nil {
			return errors.New("attach it to the show, not to an episode")
		}
		if s.LinkOf(ctx, p.ID) != nil {
			return errors.New("that title is attached to something itself")
		}
	}
	if parentShow != nil {
		if _, err := s.ShowFacts(ctx, *parentShow); err != nil {
			return err
		}
	}
	var children int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM item_links WHERE parent_item = ?`, id).Scan(&children); err != nil {
		return err
	}
	if children > 0 {
		return errors.New("this title has its own extras or versions. Move those first")
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO item_links(item_id, link, extra_type, parent_item, parent_show, label, created_at) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(item_id) DO UPDATE SET link = excluded.link, extra_type = excluded.extra_type, parent_item = excluded.parent_item, parent_show = excluded.parent_show, label = excluded.label`,
		id, link, extraType, parentItem, parentShow, cleanText(label, 80, false), db.Now())
	return err
}

// DetachItem makes an attached item a title of its own again (a split).
func (s *Store) DetachItem(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM item_links WHERE item_id = ?`, id)
	return err
}

// AttachedTo lists what's attached to a film or a show.
func (s *Store) AttachedTo(ctx context.Context, target string, id int64, link string) ([]*Item, []ItemLink, error) {
	col := "parent_item"
	if target == TargetShow {
		col = "parent_show"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT item_id, link, extra_type, label FROM item_links WHERE `+col+` = ? AND link = ? ORDER BY created_at, item_id`, id, link)
	if err != nil {
		return nil, nil, err
	}
	var idsOut []int64
	links := map[int64]ItemLink{}
	for rows.Next() {
		var i int64
		var l ItemLink
		if err := rows.Scan(&i, &l.Link, &l.ExtraType, &l.Label); err != nil {
			rows.Close()
			return nil, nil, err
		}
		idsOut = append(idsOut, i)
		links[i] = l
	}
	rows.Close()
	if len(idsOut) == 0 {
		return []*Item{}, []ItemLink{}, nil
	}
	items, _, err := s.Items(ctx, ItemQuery{IDs: idsOut, IncludeAttached: true, Limit: len(idsOut), NoTotal: true, Brief: true})
	if err != nil {
		return nil, nil, err
	}
	out := make([]ItemLink, len(items))
	for i, it := range items {
		out[i] = links[it.ID]
	}
	return items, out, nil
}

// ---------------------------------------------------------------------------
// Moving between libraries
// ---------------------------------------------------------------------------

// libraryFamilies are the kinds of library that can hold the same files.
func sameLibraryFamily(a, b string) bool {
	fam := func(t string) string {
		switch t {
		case "movies", "discs", "videos", "commercials":
			return "film"
		case "shows", "anime":
			return "tv"
		}
		return t
	}
	return fam(a) == fam(b)
}

// MoveInfo says where a title can go, or why it can't.
type MoveInfo struct {
	Can     bool          `json:"can"`
	Why     string        `json:"why,omitempty"`
	Options []MoveOption  `json:"options"`
	Moved   *MoveFromInfo `json:"moved,omitempty"`
}

// MoveOption is a library a title can move to.
type MoveOption struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// MoveFromInfo: it was moved here, and from where.
type MoveFromInfo struct {
	From int64  `json:"from"`
	Name string `json:"name"`
}

func libTypeWords(t string) string {
	switch t {
	case "movies", "discs", "videos", "commercials":
		return "film or video"
	case "shows", "anime":
		return "TV"
	}
	return t
}

// MoveOptions works out where a title can move to.
func (s *Store) MoveOptions(ctx context.Context, t *Title) MoveInfo {
	out := MoveInfo{Options: []MoveOption{}}
	if t.Item != nil && t.Item.ShowID != nil {
		out.Why = "This is an episode, so it stays with its show. Move the show instead."
		return out
	}
	if t.Item != nil && (t.Item.Kind == "cd-image" || t.Item.Kind == "game" || isRemote(t.Item.Path)) {
		out.Why = "This kind of title stays in the library that found it."
		return out
	}
	if t.Item != nil && s.LinkOf(ctx, t.ID) != nil {
		out.Why = "This is attached to another title, so it goes where that goes."
		return out
	}
	libs, err := s.Libraries(ctx)
	if err != nil {
		out.Why = "The libraries couldn't be read."
		return out
	}
	var cur *Library
	for i := range libs {
		if libs[i].ID == t.LibraryID() {
			cur = &libs[i]
		}
	}
	if cur == nil {
		out.Why = "The library it's in has gone."
		return out
	}
	for _, l := range libs {
		if l.ID == cur.ID || !sameLibraryFamily(l.Type, cur.Type) {
			continue
		}
		// A library of discs only holds discs.
		if l.Type == "discs" && t.Item != nil && !IsDiscKind(t.Item.Kind) {
			continue
		}
		out.Options = append(out.Options, MoveOption{ID: l.ID, Name: l.Name, Type: l.Type})
	}
	var from int64
	if s.DB.QueryRowContext(ctx, `SELECT from_library FROM library_moves WHERE path = ? AND to_library = ?`, t.Path(), cur.ID).Scan(&from) == nil {
		for _, l := range libs {
			if l.ID == from {
				out.Moved = &MoveFromInfo{From: from, Name: l.Name}
			}
		}
	}
	out.Can = len(out.Options) > 0
	if !out.Can {
		out.Why = "There's no other " + libTypeWords(cur.Type) + " library to move it to."
	}
	return out
}

// MoveTitle puts a title in another library. Its file doesn't move: the library it
// was found in is told to leave it, and the new one to look after it, through scans.
func (s *Store) MoveTitle(ctx context.Context, t *Title, to int64) error {
	info := s.MoveOptions(ctx, t)
	ok := false
	for _, o := range info.Options {
		ok = ok || o.ID == to
	}
	if !ok {
		if info.Why != "" {
			return errors.New(info.Why)
		}
		return errors.New("it can't move to that library")
	}
	from := t.LibraryID()
	// If it's going back to where a scan would find it anyway, no note is needed.
	roots, err := s.libraryPaths(ctx, to)
	if err != nil {
		return err
	}
	if t.Item != nil {
		if _, err := s.DB.ExecContext(ctx, `UPDATE items SET library_id = ? WHERE id = ?`, to, t.ID); err != nil {
			return err
		}
	} else {
		if _, err := s.DB.ExecContext(ctx, `UPDATE shows SET library_id = ? WHERE id = ?`, to, t.ID); err != nil {
			return err
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE items SET library_id = ? WHERE show_id = ?`, to, t.ID); err != nil {
			return err
		}
	}
	col := "item_id"
	if t.Show != nil {
		col = "show_id"
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE title_brands SET library_id = ? WHERE `+col+` = ?`, to, t.ID); err != nil {
		return err
	}
	if underAny(t.Path(), roots) {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM library_moves WHERE path = ?`, t.Path())
		return err
	}
	var orig int64
	if s.DB.QueryRowContext(ctx, `SELECT from_library FROM library_moves WHERE path = ?`, t.Path()).Scan(&orig) == nil {
		from = orig // it stays a note about where it was really found
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO library_moves(path, from_library, to_library, title, moved_at) VALUES(?,?,?,?,?)
		ON CONFLICT(path) DO UPDATE SET to_library = excluded.to_library, title = excluded.title, moved_at = excluded.moved_at`, t.Path(), from, to, t.Name(), db.Now())
	return err
}

// RemovedTitle is something taken out of a library that scans leave out.
type RemovedTitle struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Library int64  `json:"libraryId"`
	At      int64  `json:"at"`
}

// RemoveTitle takes a title out of its library without touching its file: it's hidden,
// and scans don't add it back until it's restored.
func (s *Store) RemoveTitle(ctx context.Context, t *Title) error {
	if t.Item != nil {
		if _, err := s.DB.ExecContext(ctx, `UPDATE items SET missing = 1 WHERE id = ?`, t.ID); err != nil {
			return err
		}
	} else {
		if _, err := s.DB.ExecContext(ctx, `UPDATE shows SET missing = 1 WHERE id = ?`, t.ID); err != nil {
			return err
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE items SET missing = 1 WHERE show_id = ?`, t.ID); err != nil {
			return err
		}
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO library_moves(path, from_library, to_library, title, moved_at) VALUES(?,?,0,?,?)
		ON CONFLICT(path) DO UPDATE SET to_library = 0, title = excluded.title, moved_at = excluded.moved_at`, t.Path(), t.LibraryID(), t.Name(), db.Now())
	return err
}

// RemovedTitles lists what was taken out of libraries.
func (s *Store) RemovedTitles(ctx context.Context, libraryID int64) ([]RemovedTitle, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT path, title, from_library, moved_at FROM library_moves WHERE to_library = 0 AND (? = 0 OR from_library = ?) ORDER BY moved_at DESC LIMIT 500`, libraryID, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RemovedTitle{}
	for rows.Next() {
		var r RemovedTitle
		if err := rows.Scan(&r.Path, &r.Title, &r.Library, &r.At); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RestoreRemoved brings something back. It shows again at once, and the note is gone.
func (s *Store) RestoreRemoved(ctx context.Context, path string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM library_moves WHERE path = ? AND to_library = 0`, path)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE items SET missing = 0 WHERE path = ?`, path); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE items SET missing = 0 WHERE show_id IN (SELECT id FROM shows WHERE path = ?)`, path); err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE shows SET missing = 0 WHERE path = ?`, path)
	return err
}

// libraryNotes says what a scan of this library should leave out, and what it should
// look after that its folders don't hold.
func (s *Store) libraryNotes(ctx context.Context, libraryID int64) (leave []string, adopt []string, err error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT path, from_library, to_library FROM library_moves WHERE from_library = ? OR to_library = ?`, libraryID, libraryID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		var from, to int64
		if err := rows.Scan(&p, &from, &to); err != nil {
			return nil, nil, err
		}
		if to == libraryID {
			adopt = append(adopt, p)
		} else if from == libraryID {
			leave = append(leave, p)
		}
	}
	return leave, adopt, rows.Err()
}
