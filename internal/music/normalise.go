// Package music is the music library's own framework: what ties a song to its music
// video, and how the two are matched. It speaks only to the database, so it can move
// with the music library wherever that ends up.
package music

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// What a video's title often carries that the song's doesn't. A bracket holding any of
// these words is dropped before comparing: "Lights (Official Music Video) [HD]" is "lights".
var dropBracket = regexp.MustCompile(`(?i)[\(\[\{][^\)\]\}]*\b(official|video|videoclip|clip|lyrics?|audio|visuali[sz]er|hd|hq|4k|1080p|720p|remaster(ed)?|explicit|clean|mv|m/v)\b[^\)\]\}]*[\)\]\}]`)

// Words that make a recording a different take of the song: a match that differs only by
// these is never sure.
var takeWords = regexp.MustCompile(`(?i)\b(live|acoustic|unplugged|demo|remix|mix|edit|version|instrumental|karaoke|cover|session|rehearsal)\b`)

// "feat. X", "ft X", "featuring X", "with X" at the end of a title or an artist.
var featuring = regexp.MustCompile(`(?i)[\(\[]?\s*\b(feat\.?|ft\.?|featuring)\s+[^\)\]]*[\)\]]?`)

// A trailing " - Official Video" or " | Official Music Video".
var dropTail = regexp.MustCompile(`(?i)\s+[-|–—]\s+(official|lyric|music video|video|audio|visuali[sz]er)\b.*$`)

// Key folds a name for comparing: accents and case gone, "&" read as "and", anything that
// isn't a letter or digit made a single space.
func Key(s string) string {
	s = norm.NFKD.String(strings.ToLower(s))
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue // the accent of a letter already written
		case r == '&' || r == '+':
			if b.Len() > 0 && !space {
				b.WriteByte(' ')
			}
			b.WriteString("and ")
			space = true
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			space = false
		default:
			if b.Len() > 0 && !space {
				b.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// TitleKey is a song or video title for comparing, with what a video adds taken off.
func TitleKey(title string) string {
	t := dropBracket.ReplaceAllString(title, " ")
	t = dropTail.ReplaceAllString(t, "")
	t = featuring.ReplaceAllString(t, " ")
	return Key(t)
}

// Any bracket, and anything after " - ".
var anyBracket = regexp.MustCompile(`[\(\[\{][^\)\]\}]*[\)\]\}]|\s+[-|–—]\s+.*$`)

// baseKey is a title with every bracket and tail gone, so "Autumn (Live)" and
// "Autumn - Acoustic" are both "autumn". Only good for suggestions.
func baseKey(title string) string {
	return Key(anyBracket.ReplaceAllString(featuring.ReplaceAllString(title, " "), " "))
}

// ArtistKey is the main artist for comparing: "A feat. B" and "A ft B" are "a". "The" at
// the start doesn't count either.
func ArtistKey(artist string) string {
	a := featuring.ReplaceAllString(artist, " ")
	k := Key(a)
	return strings.TrimPrefix(k, "the ")
}

// IsTake says whether a title names another take of a song: live, acoustic, a remix.
func IsTake(title string) bool {
	return takeWords.MatchString(dropBracket.ReplaceAllString(title, " "))
}

// CleanISRC is an ISRC as twelve letters and digits, or "" when it isn't one.
func CleanISRC(s string) string {
	s = strings.ToUpper(strings.NewReplacer("-", "", " ", "", "_", "").Replace(strings.TrimSpace(s)))
	if len(s) != 12 {
		return ""
	}
	for i, r := range s {
		letter := r >= 'A' && r <= 'Z'
		digit := r >= '0' && r <= '9'
		if (i < 2 && !letter) || (i >= 2 && i < 5 && !letter && !digit) || (i >= 5 && !digit) {
			return ""
		}
	}
	return s
}

// CleanMBID is a MusicBrainz identifier in its usual form, or "" when it isn't one.
func CleanMBID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) != 36 {
		return ""
	}
	for i, r := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return ""
			}
			continue
		}
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return ""
		}
	}
	return s
}

// similar is how alike two keys are, from 0 to 1, by edit distance.
func similar(a, b string) float64 {
	if a == b {
		return 1
	}
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 || len(rb) == 0 || len(ra) > 200 || len(rb) > 200 {
		return 0
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return 1 - float64(prev[len(rb)])/float64(max(len(ra), len(rb)))
}
