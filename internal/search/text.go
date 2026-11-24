// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package search is the server's one search: every title, show, album, artist,
// person and channel name held in memory as three-letter pieces (trigrams), so a
// search costs a few lookups however large the libraries are, and a word typed
// with a letter wrong still finds what was meant.
package search

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Normalize is how both titles and searches are compared: lower case, accents
// gone, apostrophes dropped ("don't" is "dont"), "&" read as "and", and anything
// else that isn't a letter or a digit a space between words.
func Normalize(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	space := true // no space at the start
	for _, r := range norm.NFD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue // the accent of a letter just written
		case r == '\'' || r == '’' || r == '`':
			continue
		case r == '&':
			if !space {
				b.WriteByte(' ')
			}
			b.WriteString("and ")
			space = true
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			space = false
		default:
			if !space {
				b.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// gram is three letters in one number: 21 bits each covers every code point.
type gram uint64

func pack(a, b, c rune) gram { return gram(a)<<42 | gram(b)<<21 | gram(c) }

// grams are the pieces of normalised text, each word with a space before and
// after it, so "ab" is " ab" and "ab ". With open set, the last word has no space
// after it: it's still being typed, and "inter" should find "interstellar".
func grams(text string, open bool, add func(gram)) {
	words := strings.Fields(text)
	if len(words) > 0 { // the name's first two letters once more, as its start: what begins with the search ranks first
		r := []rune(words[0])
		if len(r) >= 2 {
			add(pack(1, r[0], r[1]))
		}
	}
	for wi, w := range words {
		rs := make([]rune, 0, utf8.RuneCountInString(w)+2)
		rs = append(rs, ' ')
		rs = append(rs, []rune(w)...)
		if !open || wi < len(words)-1 {
			rs = append(rs, ' ')
		}
		if len(rs) < 3 { // one letter typed: " a" stands for every word starting with it
			add(pack(rs[0], rs[1], 0))
			continue
		}
		for i := 0; i+2 < len(rs); i++ {
			add(pack(rs[i], rs[i+1], rs[i+2]))
		}
	}
}

// distance is the number of letters added, dropped, changed or swapped with a
// neighbour between a and b, stopping at limit+1 once it's sure to be more. rows
// is room for the table, reused from one call to the next.
func distance(a, b []rune, limit int, rows *[3][]int) int {
	if d := len(a) - len(b); d > limit || -d > limit {
		return limit + 1
	}
	n := len(b) + 1
	for i := range rows {
		if cap(rows[i]) < n {
			rows[i] = make([]int, n, max(n, 32))
		}
	}
	// Three rows of the usual table: two back for a swap.
	prev2, prev, cur := rows[0][:n], rows[1][:n], rows[2][:n]
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		best := cur[0]
		for j := 1; j < n; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			v := min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				v = min(v, prev2[j-2]+1)
			}
			cur[j] = v
			best = min(best, v)
		}
		if best > limit {
			return limit + 1
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(b)]
}

// allowed is how many letters a word may have wrong and still match: none in a
// short word (where one wrong letter makes another word: "love" and "lode"), one
// from five letters, two from nine.
func allowed(n int) int {
	switch {
	case n >= 9:
		return 2
	case n >= 5:
		return 1
	}
	return 0
}

// little are words too common to say what a search is about: a title that matches
// only these and one other word of a longer search isn't what was meant.
var little = map[string]bool{"the": true, "a": true, "an": true, "of": true, "and": true, "to": true, "in": true,
	"on": true, "at": true, "for": true, "la": true, "le": true, "les": true, "el": true, "los": true, "der": true,
	"die": true, "das": true, "de": true, "du": true}
