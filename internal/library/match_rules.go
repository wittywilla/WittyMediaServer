package library

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Letting names guide matching. Two things:
//
//   - An id in a folder or file name, in the usual forms: "My Film (2001) [tmdbid-123]",
//     "{imdb-tt0000001}", "[tvdb-456]". This always wins over a search.
//   - A library's rules, for folders with a naming of their own: a pattern such as
//     "Studio Ghibli/{year} - {title}" says how to read a path, and a rule can also say
//     where to look, add tags, or mark everything it matches as a bonus feature.

// A tag names an id in a file or folder name.
var idTagRe = regexp.MustCompile(`(?i)[\[{(]\s*(tmdbid|tmdb|imdbid|imdb|tvdbid|tvdb|tvmazeid|tvmaze|anilistid|anilist)[-= :]\s*([a-z0-9]{1,16})\s*[\]})]`)

// IDTag reads an id out of a name.
func IDTag(name string) (MatchRef, bool) {
	m := idTagRe.FindStringSubmatch(name)
	if m == nil {
		return MatchRef{}, false
	}
	src := strings.TrimSuffix(strings.ToLower(m[1]), "id")
	id := strings.ToLower(m[2])
	if src == "imdb" && !imdbRe.MatchString(id) {
		return MatchRef{}, false
	}
	return MatchRef{src, id}, true
}

// stripIDTags takes ids out of a name, so they don't end up in a title.
func stripIDTags(name string) string {
	return strings.TrimSpace(idTagRe.ReplaceAllString(name, ""))
}

// MatchRule is one rule of a library.
type MatchRule struct {
	ID        int64    `json:"id"`
	LibraryID int64    `json:"libraryId"`
	Position  int      `json:"position"`
	Pattern   string   `json:"pattern"`
	Lookup    string   `json:"lookup"` // auto | none | a source
	Tags      []string `json:"tags"`
	Extra     string   `json:"extra"` // "" or an extra type, to mark as a bonus feature
	Enabled   bool     `json:"enabled"`
}

// The words a pattern can use for a part of a name.
var ruleWords = map[string]string{"title": `.+?`, "year": `(?:19|20)\d{2}`, "edition": `.+?`, "tmdb": `\d+`, "imdb": `tt\d+`, "tvdb": `\d+`, "any": `.*?`}

var ruleToken = regexp.MustCompile(`\{([a-z]+)\}`)

// compileRule turns a pattern into something to test a path with. A pattern is plain
// words with {title}, {year}, {edition}, {tmdb}, {imdb}, {tvdb}, {any}, and * for
// anything within a folder name. Nothing else is special, and Go's regular
// expressions can't take long however they're fed.
func compileRule(pattern string) (*regexp.Regexp, error) {
	p := strings.TrimSpace(strings.ReplaceAll(pattern, `\`, "/"))
	if p == "" || len(p) > 300 {
		return nil, errors.New("a pattern needs from 1 to 300 characters")
	}
	var b strings.Builder
	b.WriteString(`(?i)^(?:.*/)?`)
	last := 0
	seen := map[string]bool{}
	var failure error
	write := func(lit string) {
		for i, part := range strings.Split(lit, "*") {
			if i > 0 {
				b.WriteString(`[^/]*`)
			}
			b.WriteString(regexp.QuoteMeta(part))
		}
	}
	for _, m := range ruleToken.FindAllStringSubmatchIndex(p, -1) {
		write(p[last:m[0]])
		word := p[m[2]:m[3]]
		re, ok := ruleWords[word]
		if !ok {
			failure = errors.New("a pattern can't use {" + word + "}. It can use {title}, {year}, {edition}, {tmdb}, {imdb}, {tvdb} and {any}")
			break
		}
		if word == "any" || seen[word] {
			b.WriteString(`(?:` + re + `)`)
		} else {
			seen[word] = true
			b.WriteString(`(?P<` + word + `>` + re + `)`)
		}
		last = m[1]
	}
	if failure != nil {
		return nil, failure
	}
	write(p[last:])
	b.WriteString(`$`)
	return regexp.Compile(b.String())
}

// RuleResult is what a rule says about a path.
type RuleResult struct {
	Matched bool      `json:"matched"`
	Rule    int64     `json:"rule,omitempty"`
	Title   string    `json:"title,omitempty"`
	Year    *int      `json:"year,omitempty"`
	Edition []string  `json:"edition,omitempty"`
	Ref     *MatchRef `json:"-"`
	Source  string    `json:"source,omitempty"`
	ID      string    `json:"id,omitempty"`
	Lookup  string    `json:"lookup,omitempty"`
	Tags    []string  `json:"tags,omitempty"`
	Extra   string    `json:"extra,omitempty"`
}

// applyRule tests one rule on a path relative to its library's folder, without its extension.
func applyRule(r *MatchRule, re *regexp.Regexp, rel string) (RuleResult, bool) {
	m := re.FindStringSubmatch(rel)
	if m == nil {
		return RuleResult{}, false
	}
	out := RuleResult{Matched: true, Rule: r.ID, Lookup: r.Lookup, Tags: r.Tags, Extra: r.Extra}
	for i, name := range re.SubexpNames() {
		v := strings.TrimSpace(m[i])
		if v == "" {
			continue
		}
		switch name {
		case "title":
			out.Title, _ = ParseName(v)
		case "year":
			if y, err := strconv.Atoi(v); err == nil {
				out.Year = &y
			}
		case "edition":
			out.Edition = []string{cleanText(strings.NewReplacer(".", " ", "_", " ").Replace(v), 60, false)}
		case "tmdb":
			out.Source, out.ID = "tmdb", v
		case "imdb":
			out.Source, out.ID = "imdb", strings.ToLower(v)
		case "tvdb":
			out.Source, out.ID = "tvdb", v
		}
	}
	return out, true
}

// MatchRules lists a library's rules, in order.
func (s *Store) MatchRules(ctx context.Context, libraryID int64) ([]MatchRule, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, library_id, position, pattern, lookup, tags, extra, enabled FROM match_rules WHERE library_id = ? ORDER BY position, id`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MatchRule{}
	for rows.Next() {
		var r MatchRule
		var tags string
		var on int
		if err := rows.Scan(&r.ID, &r.LibraryID, &r.Position, &r.Pattern, &r.Lookup, &tags, &r.Extra, &on); err != nil {
			return nil, err
		}
		r.Tags, r.Enabled = splitList(tags), on != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// maxRules bounds a library's rules: each is tested on each file of a scan.
const maxRules = 100

// SetMatchRules replaces a library's rules, in the order given.
func (s *Store) SetMatchRules(ctx context.Context, libraryID int64, rules []MatchRule) ([]MatchRule, error) {
	if len(rules) > maxRules {
		return nil, errors.New("a library can have at most 100 rules")
	}
	for i := range rules {
		if _, err := compileRule(rules[i].Pattern); err != nil {
			return nil, err
		}
		switch rules[i].Lookup {
		case "", "auto":
			rules[i].Lookup = "auto"
		case "none", "tmdb", "tvdb", "tvmaze", "wikidata", "anilist":
		default:
			return nil, errors.New("a rule can look in auto, none, tmdb, tvdb, tvmaze, wikidata or anilist")
		}
		if e := rules[i].Extra; e != "" {
			ok := false
			for _, x := range ExtraTypes {
				ok = ok || x == e
			}
			if !ok {
				return nil, errors.New("unknown kind of bonus feature: " + e)
			}
		}
		rules[i].Tags = cleanList(rules[i].Tags, 20, 60)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM match_rules WHERE library_id = ?`, libraryID); err != nil {
		return nil, err
	}
	for i, r := range rules {
		on := 0
		if r.Enabled {
			on = 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO match_rules(library_id, position, pattern, lookup, tags, extra, enabled) VALUES(?,?,?,?,?,?,?)`,
			libraryID, i, strings.TrimSpace(r.Pattern), r.Lookup, joinList(r.Tags), r.Extra, on); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.MatchRules(ctx, libraryID)
}

// ruleSet is a library's rules, ready to test.
type ruleSet struct {
	rules []MatchRule
	res   []*regexp.Regexp
	roots []string
}

func (s *Store) loadRuleSet(ctx context.Context, lib *Library) *ruleSet {
	out := &ruleSet{roots: lib.Paths}
	rules, err := s.MatchRules(ctx, lib.ID)
	if err != nil {
		return out
	}
	for i := range rules {
		if !rules[i].Enabled {
			continue
		}
		re, err := compileRule(rules[i].Pattern)
		if err != nil {
			continue
		}
		out.rules = append(out.rules, rules[i])
		out.res = append(out.res, re)
	}
	return out
}

// test finds the first rule that fits a file.
func (r *ruleSet) test(path string) (RuleResult, bool) {
	if r == nil || len(r.rules) == 0 {
		return RuleResult{}, false
	}
	root := rootFor(r.roots, path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return RuleResult{}, false
	}
	rel = filepath.ToSlash(rel)
	rel = strings.TrimSuffix(rel, filepath.Ext(rel))
	for i := range r.rules {
		if out, ok := applyRule(&r.rules[i], r.res[i], rel); ok {
			return out, true
		}
	}
	return RuleResult{}, false
}

// RuleTry is one file in a try of some rules.
type RuleTry struct {
	File   string     `json:"file"`
	Result RuleResult `json:"result"`
}

// TryRules says what some rules would do to a library's files, without changing
// anything, so a pattern can be got right first.
func (s *Store) TryRules(ctx context.Context, libraryID int64, rules []MatchRule) ([]RuleTry, error) {
	lib, err := s.Library(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	set := &ruleSet{roots: lib.Paths}
	for i := range rules {
		re, err := compileRule(rules[i].Pattern)
		if err != nil {
			return nil, err
		}
		set.rules = append(set.rules, rules[i])
		set.res = append(set.res, re)
	}
	items, _, err := s.Items(ctx, ItemQuery{LibraryID: libraryID, Limit: 200, NoTotal: true, Brief: true, IncludeAttached: true})
	if err != nil {
		return nil, err
	}
	out := []RuleTry{}
	for _, it := range items {
		if r, ok := set.test(it.Path); ok {
			rel, _ := filepath.Rel(rootFor(lib.Paths, it.Path), it.Path)
			out = append(out, RuleTry{File: filepath.ToSlash(rel), Result: r})
		}
	}
	return out, nil
}
