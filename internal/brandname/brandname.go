// Package brandname splits the server's name into the parts drawn in the theme
// colour and the parts drawn plain, the way WittyMediaServer is drawn as
// Witty, Media in the theme colour, then Server.
//
// The same rule is in web/src/brandName.ts, and both are tested against
// testdata/cases.json, so every device draws a name the same way. Apps get the
// parts ready-made as brandParts in GET /api/status.
package brandname

import (
	"strconv"
	"strings"
	"unicode"
)

// Default is the name shown when the owner hasn't given one.
const Default = "WittyMediaServer"

// MaxRunes is the longest name the settings accept, so no colouring range can
// reach further.
const MaxRunes = 80

// Part is a run of the name, drawn in the theme colour when Accent is set.
type Part struct {
	Text   string `json:"text"`
	Accent bool   `json:"accent,omitempty"`
}

// The brandColouring setting: Auto (also when empty or unknown), One for the
// whole name in one colour, or Pick followed by the coloured ranges, as
// start-end pairs of character (code point) offsets, end not included:
// "pick:5-10,16-22".
const (
	Auto = "auto"
	One  = "one"
	Pick = "pick:"
)

// Parts splits name by the colouring setting. The parts joined give the name
// back unchanged; neighbouring parts never share the same Accent.
func Parts(name, colouring string) []Part {
	r := []rune(name)
	accent := make([]bool, len(r))
	switch {
	case colouring == One:
	case strings.HasPrefix(colouring, Pick):
		if ranges, ok := parsePick(colouring); ok {
			for _, rg := range ranges {
				for i := rg[0]; i < rg[1] && i < len(r); i++ {
					accent[i] = true
				}
			}
			break
		}
		fallthrough
	default:
		for n, w := range Words(name) {
			if n%2 == 1 {
				for i := w[0]; i < w[1]; i++ {
					accent[i] = true
				}
			}
		}
	}
	var out []Part
	start := 0
	for i := 1; i <= len(r); i++ {
		if i == len(r) || accent[i] != accent[start] {
			out = append(out, Part{Text: string(r[start:i]), Accent: accent[start]})
			start = i
		}
	}
	return out
}

// Words finds the words of a name, as [start, end) code point offsets: split
// at spaces, and inside joined words before a capital that follows a small
// letter (MyHome), before the last capital of a run that starts a word
// (TVRoom: TV, Room; but TVs stays whole), between small letters and digits
// (Movies4K), and before a capital after a dash, underscore, slash, &, +, | or
// colon (Witty-Media). Runs of capitals and digits stay together (TV, 4K, HDTV), and
// scripts without capitals split at spaces only.
func Words(name string) [][2]int {
	r := []rune(name)
	var out [][2]int
	start := -1
	for i, c := range r {
		if unicode.IsSpace(c) {
			if start >= 0 {
				out = append(out, [2]int{start, i})
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
			continue
		}
		if breaksBefore(r, start, i) {
			out = append(out, [2]int{start, i})
			start = i
		}
	}
	if start >= 0 {
		out = append(out, [2]int{start, len(r)})
	}
	return out
}

// breaksBefore says whether a new word starts at r[i], in a word that began at start.
func breaksBefore(r []rune, start, i int) bool {
	c := r[i]
	// Accents written as separate marks belong to the letter before them.
	p := -1
	for j := i - 1; j >= start; j-- {
		if !unicode.Is(unicode.M, r[j]) {
			p = j
			break
		}
	}
	if p < 0 || unicode.Is(unicode.M, c) {
		return false
	}
	prev := r[p]
	switch {
	case unicode.IsUpper(c):
		switch {
		case unicode.IsLower(prev):
			return true
		case unicode.IsUpper(prev) || unicode.IsDigit(prev):
			// The last capital of a run starts the next word when two small
			// letters follow: TVRoom, 4KMovies, not TVs or DVDs.
			return i+2 < len(r) && unicode.IsLower(r[i+1]) && unicode.IsLower(r[i+2])
		case joiner(prev):
			// After a dash or the like, when a letter or digit came before it: not -Home.
			for j := start; j < p; j++ {
				if unicode.IsLetter(r[j]) || unicode.IsDigit(r[j]) {
					return true
				}
			}
		}
	case unicode.IsDigit(c):
		return unicode.IsLower(prev)
	}
	return false
}

// joiner: the marks that join two words into one, where a capital after them
// starts the next word. Apostrophes and full stops don't (O'Brien, J.R.).
func joiner(c rune) bool {
	return unicode.Is(unicode.Pd, c) || strings.ContainsRune("_/&+|·:", c)
}

// Valid says whether v is a brandColouring the settings accept.
func Valid(v string) bool {
	if v == "" || v == Auto || v == One {
		return true
	}
	_, ok := parsePick(v)
	return ok
}

// parsePick reads "pick:5-10,16-22": ranges in order, not overlapping, within MaxRunes.
// "pick:" alone is allowed: nothing coloured, as One.
func parsePick(v string) ([][2]int, bool) {
	body, ok := strings.CutPrefix(v, Pick)
	if !ok {
		return nil, false
	}
	if body == "" {
		return nil, true
	}
	var out [][2]int
	end := 0
	for _, s := range strings.Split(body, ",") {
		a, b, ok := strings.Cut(s, "-")
		if !ok {
			return nil, false
		}
		x, err1 := strconv.Atoi(a)
		y, err2 := strconv.Atoi(b)
		if err1 != nil || err2 != nil || x < end || y <= x || y > MaxRunes || len(a) > 2 || len(b) > 2 {
			return nil, false
		}
		out = append(out, [2]int{x, y})
		end = y
	}
	return out, true
}
