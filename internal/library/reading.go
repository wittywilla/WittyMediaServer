package library

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"regexp"
	"strings"
	"unicode"
)

// Reading tools for EPUBs: per-account annotations (bookmarks, highlights,
// notes and handwriting), word counts for "time left", and full-text search.

// Annotation is a bookmark, highlight, note or ink stroke in a book. A
// highlight is anchored by its quoted text plus a little text before and
// after it, so it is found again whatever the font size or screen. An ink
// stroke keeps its points in Data, relative to the word it was drawn by.
// In PDFs, where pages never reflow, a highlight also keeps its boxes on
// the page in Data and an ink stroke is in page units.
type Annotation struct {
	ID      int64   `json:"id"`
	ItemID  int64   `json:"itemId"`
	Kind    string  `json:"kind"` // bookmark | highlight | ink
	Chapter int     `json:"chapter"`
	Pos     float64 `json:"pos"` // chapter + fraction of the chapter
	Quote   string  `json:"quote"`
	Prefix  string  `json:"prefix"`
	Suffix  string  `json:"suffix"`
	Color   string  `json:"color"` // a named colour or #rrggbb
	Note    string  `json:"note"`
	Data    string  `json:"data,omitempty"` // ink: the stroke; PDF highlights: their boxes (JSON)
	Created int64   `json:"created"`
}

var (
	annColors = map[string]bool{"yellow": true, "green": true, "blue": true, "pink": true, "": true}
	hexColor  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// maxInk caps one stroke's JSON (a long, dense pen stroke is ~20 KB).
const maxInk = 256 << 10

// maxAnnotationsPerBook keeps one account from filling the disk with notes.
const maxAnnotationsPerBook = 20000

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// Annotations lists one account's annotations in a book, in reading order.
func (s *Store) Annotations(ctx context.Context, userID, itemID int64) ([]Annotation, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, item_id, kind, chapter, pos, quote, prefix, suffix, color, note, data, created_at
		FROM annotations WHERE user_id = ? AND item_id = ? ORDER BY pos, id`, userID, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Annotation{}
	for rows.Next() {
		var a Annotation
		if err := rows.Scan(&a.ID, &a.ItemID, &a.Kind, &a.Chapter, &a.Pos, &a.Quote, &a.Prefix, &a.Suffix, &a.Color, &a.Note, &a.Data, &a.Created); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SaveAnnotation adds (ID 0) or updates one of the account's annotations.
func (s *Store) SaveAnnotation(ctx context.Context, userID int64, a *Annotation) error {
	if a.Kind != "bookmark" && a.Kind != "highlight" && a.Kind != "ink" {
		return errors.New("kind must be bookmark, highlight or ink")
	}
	if !annColors[a.Color] && !hexColor.MatchString(a.Color) {
		return errors.New("unknown colour")
	}
	if a.Kind == "bookmark" || (a.Kind == "highlight" && a.Data == "") {
		a.Data = ""
	} else if len(a.Data) > maxInk || !json.Valid([]byte(a.Data)) {
		return errors.New("bad annotation data")
	}
	a.Quote, a.Prefix, a.Suffix, a.Note = clip(a.Quote, 4000), clip(a.Prefix, 200), clip(a.Suffix, 200), clip(a.Note, 10000)
	if a.ID == 0 {
		var n int
		_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM annotations WHERE user_id = ? AND item_id = ?`, userID, a.ItemID).Scan(&n)
		if n >= maxAnnotationsPerBook {
			return errors.New("this book has as many notes as it can hold")
		}
		res, err := s.DB.ExecContext(ctx, `INSERT INTO annotations(user_id, item_id, kind, chapter, pos, quote, prefix, suffix, color, note, data, created_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,strftime('%s','now'))`, userID, a.ItemID, a.Kind, a.Chapter, a.Pos, a.Quote, a.Prefix, a.Suffix, a.Color, a.Note, a.Data)
		if err != nil {
			return err
		}
		a.ID, _ = res.LastInsertId()
		return nil
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE annotations SET color = ?, note = ? WHERE id = ? AND user_id = ?`, a.Color, a.Note, a.ID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAnnotation removes one of the account's annotations.
func (s *Store) DeleteAnnotation(ctx context.Context, userID, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM annotations WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

var spaceRe = regexp.MustCompile(`\s+`)

// stripTags takes the tags out of XHTML, and the insides of script, style and head. It keeps
// its place in the text rather than using a pattern: "<script>" with no closing tag makes a
// pattern look to the end of the text at each one, so a crafted chapter takes days.
func stripTags(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	closers := []string{"</script>", "</style>", "</head>"}
	next := []int{-2, -2, -2} // where each closer is next seen, -1 for no more of them
	for i := 0; i < len(s); {
		lt := strings.IndexByte(s[i:], '<')
		if lt < 0 {
			out.WriteString(s[i:])
			break
		}
		lt += i
		out.WriteString(s[i:lt])
		if lt+1 < len(s) && s[lt+1] == '>' { // "<>" isn't a tag
			out.WriteByte('<')
			i = lt + 1
			continue
		}
		gt := strings.IndexByte(s[lt+1:], '>')
		if gt < 0 { // no more tags can end: the rest is text
			out.WriteString(s[lt:])
			break
		}
		end := lt + 1 + gt + 1
		out.WriteByte(' ')
		i = end
		if !hidesItsInsides(s[lt+1 : end-1]) {
			continue
		}
		closeAt, closeLen := -1, 0
		for c, name := range closers {
			if next[c] != -1 && next[c] < end {
				next[c] = -1
				if at := strings.Index(s[end:], name); at >= 0 {
					next[c] = end + at
				}
			}
			if next[c] >= 0 && (closeAt < 0 || next[c] < closeAt) {
				closeAt, closeLen = next[c], len(name)
			}
		}
		if closeAt >= 0 {
			i = closeAt + closeLen
		}
	}
	return out.String()
}

// hidesItsInsides: a tag whose contents aren't text. tag is what's between < and >.
func hidesItsInsides(tag string) bool {
	for _, name := range []string{"script", "style", "head"} {
		if rest, ok := strings.CutPrefix(tag, name); ok {
			if rest == "" {
				return true
			}
			c := rest[0]
			return !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == ':')
		}
	}
	return false
}

// bookWork is how many books are being read through at once: counting a book's words or
// searching it is up to 64 MB of text, and anyone who can open a book can ask for it again.
var bookWork = make(chan struct{}, 2)

// bookBudget is how much text one request unpacks from a book. A real book
// comes nowhere near it; a crafted archive can repeat a tiny, highly compressed
// chapter thousands of times and otherwise unpack hundreds of gigabytes.
const bookBudget = 64 << 20

// chapterText is the readable text of an XHTML chapter. It takes what it reads
// off budget, and is empty once that has run out.
func chapterText(zr *zip.Reader, name string, budget *int64) string {
	if *budget <= 0 {
		return ""
	}
	f, err := zr.Open(name)
	if err != nil {
		return ""
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, min(16<<20, *budget)))
	*budget -= int64(len(b))
	return strings.TrimSpace(spaceRe.ReplaceAllString(html.UnescapeString(stripTags(string(b))), " "))
}

// WordCounts is the number of words in each chapter (for "time left").
func WordCounts(zr *zip.Reader, spine []string) []int {
	bookWork <- struct{}{}
	defer func() { <-bookWork }()
	out := make([]int, len(spine))
	budget := int64(bookBudget)
	for i, name := range spine {
		out[i] = len(strings.FieldsFunc(chapterText(zr, name, &budget), unicode.IsSpace))
	}
	return out
}

// SearchHit is one match of a search in a book.
type SearchHit struct {
	Chapter int    `json:"chapter"`
	Snippet string `json:"snippet"`
	Quote   string `json:"quote"` // the matched text as written in the book
	Prefix  string `json:"prefix"`
}

// SearchBook finds text in an EPUB (case-insensitive, at most 200 hits).
func SearchBook(zr *zip.Reader, spine []string, q string) []SearchHit {
	q = strings.TrimSpace(q)
	if len([]rune(q)) < 2 {
		return []SearchHit{}
	}
	bookWork <- struct{}{}
	defer func() { <-bookWork }()
	lq := strings.ToLower(q)
	hits := []SearchHit{}
	budget := int64(bookBudget)
	for ci, name := range spine {
		if len(hits) >= 200 {
			break
		}
		text := chapterText(zr, name, &budget)
		low := strings.ToLower(text)
		from := 0
		for len(hits) < 200 {
			i := strings.Index(low[from:], lq)
			if i < 0 {
				break
			}
			at := from + i
			// lower-casing can change byte lengths for some scripts; only use
			// positions when they line up.
			if len(low) != len(text) {
				hits = append(hits, SearchHit{Chapter: ci, Snippet: q, Quote: q})
				break
			}
			start, end := max(0, at-60), min(len(text), at+len(lq)+60)
			hits = append(hits, SearchHit{Chapter: ci, Quote: text[at : at+len(lq)], Prefix: text[max(0, at-40):at],
				Snippet: "…" + strings.ToValidUTF8(text[start:end], "") + "…"})
			from = at + len(lq)
		}
	}
	return hits
}
