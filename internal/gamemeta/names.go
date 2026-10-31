package gamemeta

import (
	"regexp"
	"strings"
	"unicode"
)

// Working out what a file name means, and how alike two names are.

var (
	tagRe     = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)
	discRe    = regexp.MustCompile(`(?i)[\s_-]*\b(?:dis[ck]|cd|side)[\s_-]*(?:[0-9]+|[a-d])(?:\s*of\s*[0-9]+)?\b`)
	discNumRe = regexp.MustCompile(`(?i)\bdis[ck]\s*([0-9]+)`)
	spacesRe  = regexp.MustCompile(`\s+`)
	// A product code left in a file name: SLUS-00594, scus_941.63, SLES 12345.
	serialNameRe = regexp.MustCompile(`(?i)\b(SL[UEPK][SM]|SC[UEPK][SM]|SCED|SLED|SLPM|PAPX|PBPX|ULUS|ULES|ULJS|ULJM|UCUS|UCES|UCJS|NPUG|NPEG|NPJG)[-_ ]?([0-9]{3})[-_. ]?([0-9]{2})\b`)
)

// CleanName splits a file's base name into the game's title and what else the name
// says, e.g. "Mega Game, The (USA) (Rev 1)" -> "The Mega Game", "USA, Rev 1".
func CleanName(base string) (title, tags string) {
	var found []string
	for _, m := range tagRe.FindAllString(base, -1) {
		t := strings.Trim(strings.TrimSpace(m), "()[]")
		if t != "" && t != "!" {
			found = append(found, t)
		}
	}
	title = strings.TrimSpace(tagRe.ReplaceAllString(base, ""))
	title = strings.TrimSpace(strings.ReplaceAll(title, "_", " "))
	for _, art := range []string{"The", "A", "An"} {
		if s, ok := strings.CutSuffix(title, ", "+art); ok {
			title = art + " " + s
			break
		}
		if a, b, ok := strings.Cut(title, ", "+art+" - "); ok {
			title = art + " " + a + " - " + b
			break
		}
	}
	if title == "" {
		title = base
	}
	return title, strings.Join(found, ", ")
}

// SearchTitle takes off what no database calls a game: which disc this is, and a
// product code left in the name. "Dashes-for-spaces" names become words.
func SearchTitle(title string) string {
	t := discRe.ReplaceAllString(title, "")
	t = serialNameRe.ReplaceAllString(t, "")
	if !strings.Contains(t, " ") && strings.Count(t, "-") > 0 {
		t = strings.ReplaceAll(t, "-", " ")
	}
	t = spacesRe.ReplaceAllString(t, " ")
	t = strings.Trim(t, " -_.,")
	if t == "" {
		return title
	}
	return t
}

// DiscNumber says which disc of a set a name is, or 0.
func DiscNumber(name string) int {
	m := discNumRe.FindStringSubmatch(name)
	if m == nil {
		return 0
	}
	n := 0
	for _, c := range m[1] {
		n = n*10 + int(c-'0')
		if n > 99 {
			return 0
		}
	}
	return n
}

// SerialInName finds a product code in a file name, in the form databases use.
func SerialInName(name string) string {
	m := serialNameRe.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	return strings.ToUpper(m[1]) + "-" + m[2] + m[3]
}

// foldTable covers what game names actually use. Anything else is kept as it is, so
// a Japanese title still matches itself.
var foldTable = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a", "æ", "ae", "ç", "c", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i", "ñ", "n", "ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o", "ø", "o", "œ", "oe",
	"ß", "ss", "ú", "u", "ù", "u", "û", "u", "ü", "u", "ý", "y", "ÿ", "y", "ō", "o", "ū", "u", "ā", "a", "ē", "e", "ī", "i",
	"&", " and ", "+", " and ", "'", "", "’", "", "`", "", ".", "")

var romans = map[string]string{"ii": "2", "iii": "3", "iv": "4", "v": "5", "vi": "6", "vii": "7", "viii": "8", "ix": "9", "x": "10",
	"xi": "11", "xii": "12", "xiii": "13", "xiv": "14", "xv": "15", "xvi": "16"}

// words are a name's words in a form that doesn't care about punctuation, case,
// articles, or whether the sequel is written II or 2.
func words(name string) []string {
	s := strings.ToLower(tagRe.ReplaceAllString(name, ""))
	s = foldTable.Replace(s)
	fields := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	out := fields[:0]
	for i, w := range fields {
		if (w == "the" || w == "a" || w == "an") && (i == 0 || i == len(fields)-1) && len(fields) > 1 {
			continue
		}
		if r, ok := romans[w]; ok && i > 0 {
			w = r
		}
		out = append(out, w)
	}
	return out
}

// NameKey is what two names of the same game have in common.
func NameKey(name string) string { return strings.Join(words(name), " ") }

// bigrams counts a name's pairs of letters, ignoring spaces.
func bigrams(s string) map[string]int {
	r := []rune(strings.ReplaceAll(s, " ", ""))
	out := make(map[string]int, len(r))
	for i := 0; i+1 < len(r); i++ {
		out[string(r[i:i+2])]++
	}
	return out
}

func numbers(ws []string) []string {
	var out []string
	for _, w := range ws {
		if w[0] >= '0' && w[0] <= '9' {
			out = append(out, w)
		}
	}
	return out
}

func sameList(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// NameScore says how alike two names are, from 0 to 100. Different numbers mean
// different games however alike the rest is: Mega Game 2 isn't Mega Game 3.
func NameScore(a, b string) int {
	wa, wb := words(a), words(b)
	if len(wa) == 0 || len(wb) == 0 {
		return 0
	}
	ka, kb := strings.Join(wa, " "), strings.Join(wb, " ")
	if ka == kb {
		return 100
	}
	ba, bb := bigrams(ka), bigrams(kb)
	total, shared := 0, 0
	for g, n := range ba {
		total += n
		shared += min(n, bb[g])
	}
	for _, n := range bb {
		total += n
	}
	if total == 0 {
		return 0
	}
	score := 200 * shared / total
	if !sameList(numbers(wa), numbers(wb)) {
		score = min(score, 50)
	}
	// A subtitle one name leaves out: "Mega Game" and "Mega Game - The Quest". Close,
	// but not close enough to be sure, since sequels are named this way too.
	if strings.HasPrefix(kb, ka+" ") || strings.HasPrefix(ka, kb+" ") {
		score = max(score, 76)
		if !sameList(numbers(wa), numbers(wb)) {
			score = 50
		}
	}
	return score
}

// Best picks the highest-scoring of a list of names.
func Best(want string, names []string) (index, score int) {
	index = -1
	for i, n := range names {
		if s := NameScore(want, n); s > score {
			index, score = i, s
		}
	}
	return index, score
}

// regionRank orders a game's regions for someone with no other preference.
func regionRank(tags string) int {
	t := strings.ToLower(tags)
	for i, r := range []string{"usa", "world", "europe", "australia", "uk", "canada"} {
		if strings.Contains(t, r) {
			return i
		}
	}
	if strings.Contains(t, "japan") {
		return 7
	}
	return 6
}

// firstYear reads the year from a date in any of the forms sources use.
func firstYear(date string) int {
	if len(date) < 4 {
		return 0
	}
	y := 0
	for _, c := range date[:4] {
		if c < '0' || c > '9' {
			return 0
		}
		y = y*10 + int(c-'0')
	}
	if y < 1950 || y > 2100 {
		return 0
	}
	return y
}

// Year reads the year from a Released date, or 0.
func Year(date string) int { return firstYear(date) }

// nameConfidence turns how alike two names are into how sure a match is: an exact
// name can still be a different game of the same name.
func nameConfidence(score int) int {
	if score >= 100 {
		return 92
	}
	return score * 9 / 10
}
