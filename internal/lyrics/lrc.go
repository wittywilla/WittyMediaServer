// Package lyrics reads song lyrics: timed LRC text (with word timing when it
// has it), plain text, and the frames an MP3 can carry them in. It knows nothing
// about the server: files and answers from the internet are untrusted, so every
// parser here has limits on what it will read.
package lyrics

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// MaxBytes is the most lyrics text worth reading. The longest real songs come
// nowhere near it.
const MaxBytes = 256 << 10

// MaxLines keeps a hostile file from making a very long list.
const MaxLines = 5000

// Word is one timed word of a line, for lyrics with word timing.
type Word struct {
	T    int64  `json:"t"` // milliseconds from the start of the track
	Text string `json:"text"`
}

// Line is one timed line. End is when the next line starts, or 0 for the last.
type Line struct {
	T     int64  `json:"t"`
	End   int64  `json:"end,omitempty"`
	Text  string `json:"text"`
	Words []Word `json:"words,omitempty"`
}

// Doc is lyrics, read. Timed holds the lines when they have times; Plain always
// holds the words alone.
type Doc struct {
	Timed []Line
	Plain string
}

// Synced reports whether there are times to follow.
func (d Doc) Synced() bool { return len(d.Timed) > 0 }

// Empty reports whether there are no words at all.
func (d Doc) Empty() bool { return strings.TrimSpace(d.Plain) == "" && len(d.Timed) == 0 }

var (
	stampRe = regexp.MustCompile(`\[(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?\]`)
	wordRe  = regexp.MustCompile(`<(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?>`)
	metaRe  = regexp.MustCompile(`^\[([a-zA-Z]+):(.*)\]\s*$`)
	// A speaker mark some files put in front of a word-timed line: "v1:" or "F:".
	markRe = regexp.MustCompile(`^(?:v\d|[FMD]):\s*`)
)

func stampMS(min, sec, frac string) int64 {
	m, _ := strconv.ParseInt(min, 10, 64)
	s, _ := strconv.ParseInt(sec, 10, 64)
	ms := m*60000 + s*1000
	switch len(frac) {
	case 1:
		f, _ := strconv.ParseInt(frac, 10, 64)
		ms += f * 100
	case 2:
		f, _ := strconv.ParseInt(frac, 10, 64)
		ms += f * 10
	case 3:
		f, _ := strconv.ParseInt(frac, 10, 64)
		ms += f
	}
	return ms
}

// Parse reads lyrics, timed or not. The file's own [offset:] tag is applied here,
// so what comes out is always in the track's time.
func Parse(text string) Doc {
	if len(text) > MaxBytes {
		text = text[:MaxBytes]
	}
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	var timed []Line
	var plain []string
	var offset int64
	for _, raw := range strings.Split(text, "\n") {
		if len(timed) >= MaxLines || len(plain) >= MaxLines {
			break
		}
		line := strings.TrimSpace(raw)
		if !stampRe.MatchString(line) {
			if m := metaRe.FindStringSubmatch(line); m != nil {
				if strings.EqualFold(m[1], "offset") {
					offset, _ = strconv.ParseInt(strings.TrimSpace(m[2]), 10, 64)
				}
				continue // [ar:], [ti:], [length:] and the like
			}
			plain = append(plain, line)
			continue
		}
		// "[00:12.00][00:45.50] words" is one line sung twice.
		var stamps []int64
		rest := line
		for {
			loc := stampRe.FindStringSubmatchIndex(rest)
			if loc == nil || loc[0] != 0 {
				break
			}
			m := stampRe.FindStringSubmatch(rest)
			stamps = append(stamps, stampMS(m[1], m[2], m[3]))
			rest = rest[loc[1]:]
		}
		rest = strings.TrimSpace(rest)
		words, shown := parseWords(rest)
		for _, t := range stamps {
			l := Line{T: t, Text: shown}
			if len(words) > 0 {
				l.Words = words
			}
			timed = append(timed, l)
		}
	}
	if len(timed) == 0 {
		return Doc{Plain: strings.TrimSpace(strings.Join(plain, "\n"))}
	}
	// The LRC offset moves lyrics earlier when positive: a line stamped 10.000 with
	// offset 500 is sung at 9.500.
	if offset != 0 {
		for i := range timed {
			timed[i].T = max(0, timed[i].T-offset)
			if timed[i].Words != nil {
				w := append([]Word(nil), timed[i].Words...)
				for j := range w {
					w[j].T = max(0, w[j].T-offset)
				}
				timed[i].Words = w
			}
		}
	}
	sort.SliceStable(timed, func(a, b int) bool { return timed[a].T < timed[b].T })
	for i := range timed {
		if i+1 < len(timed) {
			timed[i].End = timed[i+1].T
		}
	}
	var words []string
	for _, l := range timed {
		words = append(words, l.Text)
	}
	return Doc{Timed: timed, Plain: strings.TrimSpace(strings.Join(words, "\n"))}
}

// parseWords reads "<00:01.00> one <00:01.50> two" into timed words and the line
// without its marks. A line with no marks has no words.
func parseWords(s string) ([]Word, string) {
	s = markRe.ReplaceAllString(s, "")
	locs := wordRe.FindAllStringSubmatchIndex(s, -1)
	if locs == nil {
		return nil, s
	}
	var words []Word
	var shown strings.Builder
	shown.WriteString(s[:locs[0][0]])
	for i, loc := range locs {
		end := len(s)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		frac := ""
		if loc[6] >= 0 {
			frac = s[loc[6]:loc[7]]
		}
		t := stampMS(s[loc[2]:loc[3]], s[loc[4]:loc[5]], frac)
		text := s[loc[1]:end]
		shown.WriteString(text)
		if word := strings.TrimSpace(text); word != "" {
			// Written with the space after it, if any, so the words join up as sung.
			if strings.HasSuffix(text, " ") {
				word += " "
			}
			words = append(words, Word{T: t, Text: word})
		}
	}
	return words, strings.Join(strings.Fields(shown.String()), " ")
}

// Format writes timed lines as LRC, the form saved when someone retimes a song.
func Format(lines []Line) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(stamp(l.T))
		b.WriteString(strings.ReplaceAll(strings.ReplaceAll(l.Text, "\r", " "), "\n", " "))
		b.WriteByte('\n')
	}
	return b.String()
}

func stamp(ms int64) string {
	ms = max(0, ms)
	cs := (ms % 1000) / 10
	return "[" + pad(ms/60000) + ":" + pad((ms/1000)%60) + "." + pad(cs) + "]"
}

func pad(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

// Decode turns a lyrics file's bytes into text: UTF-8, or UTF-16 with a byte
// order mark, or Latin-1 for the older files that are neither.
func Decode(b []byte) string {
	if len(b) > MaxBytes {
		b = b[:MaxBytes]
	}
	switch {
	case len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF:
		return strings.ToValidUTF8(string(b[3:]), "")
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		return utf16Text(b[2:], false)
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		return utf16Text(b[2:], true)
	}
	if utf8.Valid(b) {
		return string(b)
	}
	return latin1(b)
}

func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

func utf16Text(b []byte, bigEndian bool) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if bigEndian {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			u = append(u, uint16(b[i+1])<<8|uint16(b[i]))
		}
	}
	return string(utf16.Decode(u))
}
