package ebook

import (
	"regexp"
	"strings"

	"golang.org/x/net/html/atom"
)

var bodyAtom = atom.Body

// chapterLineRe finds the headings long texts divide themselves by.
var chapterLineRe = regexp.MustCompile(`(?i)^\s*(chapter|part|book|prologue|epilogue|introduction|preface|foreword|contents)\b[\s\w.:,'-]{0,70}$`)

// A chunk of plain text with no headings is cut at about this length, so that opening
// a long file doesn't lay out all of it at once.
const textChunk = 96 << 10

// fromText reads plain text: paragraphs at blank lines, chapters at lines that look
// like headings.
func fromText(data []byte) *Book {
	b := &Book{}
	text := strings.ReplaceAll(strings.ReplaceAll(decodeText(data), "\r\n", "\n"), "\r", "\n")
	var body strings.Builder
	title := ""
	flush := func() {
		b.addChapter(title, body.String())
		body.Reset()
		title = ""
	}
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if !strings.Contains(para, "\n") && chapterLineRe.MatchString(para) {
			flush()
			title = para
			body.WriteString("<h2>" + xmlText(para) + "</h2>")
			continue
		}
		lines := strings.Split(para, "\n")
		body.WriteString("<p>")
		for i, l := range lines {
			if i > 0 {
				body.WriteString(" ")
			}
			body.WriteString(xmlText(strings.TrimSpace(l)))
		}
		body.WriteString("</p>")
		if body.Len() > textChunk {
			flush()
		}
	}
	flush()
	return b
}

var (
	mdHeadingRe = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	mdListRe    = regexp.MustCompile(`^\s*([-*+]|\d{1,4}[.)])\s+(.*)$`)
	mdRuleRe    = regexp.MustCompile(`^\s*([-*_])(\s*\1){2,}\s*$`)
	mdLinkRe    = regexp.MustCompile(`\[([^\]]{1,500})\]\((https?://[^)\s"<>]{1,1000})\)`)
	mdBoldRe    = regexp.MustCompile(`(\*\*|__)([^*_]{1,4000}?)(\*\*|__)`)
	mdItalicRe  = regexp.MustCompile(`(^|[\s(])[*_]([^*_\s][^*_]{0,4000}?)[*_]($|[\s).,;:!?])`)
	mdCodeRe    = regexp.MustCompile("`([^`]{1,2000})`")
	mdImageRe   = regexp.MustCompile(`!\[[^\]]{0,500}\]\([^)]{0,2000}\)`)
)

// fromMarkdown reads the common parts of Markdown. Anything that looks like HTML in
// it is shown as the text it is: it's escaped before any markup is added.
func fromMarkdown(data []byte) *Book {
	b := &Book{}
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(decodeText(data), "\r\n", "\n"), "\r", "\n"), "\n")
	var body, para strings.Builder
	title := ""
	inCode, inList, inQuote := false, "", false
	flushPara := func() {
		if para.Len() > 0 {
			body.WriteString("<p>" + mdInline(strings.TrimSpace(para.String())) + "</p>")
			para.Reset()
		}
	}
	closeList := func() {
		if inList != "" {
			body.WriteString("</" + inList + ">")
			inList = ""
		}
	}
	closeQuote := func() {
		if inQuote {
			flushPara()
			body.WriteString("</blockquote>")
			inQuote = false
		}
	}
	flushChapter := func() {
		flushPara()
		closeList()
		closeQuote()
		b.addChapter(title, body.String())
		body.Reset()
		title = ""
	}
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			flushPara()
			closeList()
			if inCode {
				body.WriteString("</code></pre>")
			} else {
				body.WriteString("<pre><code>")
			}
			inCode = !inCode
			continue
		}
		if inCode {
			body.WriteString(xmlText(line) + "\n")
			continue
		}
		if strings.TrimSpace(line) == "" {
			flushPara()
			closeList()
			closeQuote()
			continue
		}
		if m := mdHeadingRe.FindStringSubmatch(line); m != nil {
			level := len(m[1])
			if level == 1 && body.Len() > 0 {
				flushChapter()
			}
			flushPara()
			closeList()
			closeQuote()
			text := mdInline(m[2])
			if level == 1 && b.Title == "" {
				b.Title = strings.TrimSpace(tagRe.ReplaceAllString(m[2], ""))
			}
			if level == 1 {
				title = strings.TrimSpace(m[2])
			}
			tag := "h" + string(rune('0'+level))
			body.WriteString("<" + tag + ">" + text + "</" + tag + ">")
			continue
		}
		if mdRuleRe.MatchString(line) {
			flushPara()
			closeList()
			body.WriteString("<hr/>")
			continue
		}
		if m := mdListRe.FindStringSubmatch(line); m != nil {
			flushPara()
			want := "ul"
			if m[1][0] >= '0' && m[1][0] <= '9' {
				want = "ol"
			}
			if inList != want {
				closeList()
				body.WriteString("<" + want + ">")
				inList = want
			}
			body.WriteString("<li>" + mdInline(m[2]) + "</li>")
			continue
		}
		if strings.HasPrefix(strings.TrimLeft(line, " "), ">") {
			if !inQuote {
				flushPara()
				body.WriteString("<blockquote>")
				inQuote = true
			}
			para.WriteString(strings.TrimPrefix(strings.TrimLeft(line, " "), ">") + " ")
			continue
		}
		para.WriteString(line + " ")
		if body.Len() > textChunk {
			flushChapter()
		}
	}
	if inCode {
		body.WriteString("</code></pre>")
	}
	flushChapter()
	return b
}

// mdInline adds emphasis, code and web links to text that has been escaped first.
func mdInline(s string) string {
	s = mdImageRe.ReplaceAllString(s, "")
	s = xmlText(s)
	s = mdCodeRe.ReplaceAllString(s, "<code>$1</code>")
	s = mdLinkRe.ReplaceAllString(s, `<a href="$2">$1</a>`)
	s = mdBoldRe.ReplaceAllString(s, "<strong>$2</strong>")
	s = mdItalicRe.ReplaceAllString(s, "$1<em>$2</em>$3")
	return s
}

// fromRTF reads Rich Text: its paragraphs, bold and italics, and the title and author
// from its information block. Everything else in the format (fonts, colours, embedded
// objects and pictures) is passed over.
func fromRTF(data []byte) (*Book, error) {
	r := rtfReader{data: data}
	r.run()
	b := &Book{Title: r.title, Author: r.author}
	b.addChapter("", r.close())
	return b, nil
}

type rtfState struct {
	skip         bool
	bold, italic bool
	dest         string // info field being read
}

type rtfReader struct {
	data          []byte
	body          strings.Builder
	para          strings.Builder
	title, author string
	field         strings.Builder
	cp1252        bool
	stack         []rtfState
	cur           rtfState
	ucSkip        int
	openB, openI  bool
}

// RTF destinations whose contents aren't text of the document.
var rtfSkipped = map[string]bool{"fonttbl": true, "colortbl": true, "stylesheet": true, "pict": true, "object": true, "header": true, "footer": true, "headerl": true,
	"headerr": true, "footerl": true, "footerr": true, "footnote": true, "themedata": true, "generator": true, "datastore": true, "latentstyles": true, "listtable": true, "listoverridetable": true,
	"revtbl": true, "rsidtbl": true, "xmlnstbl": true, "mmathPr": true, "wgrffmtfilter": true, "pgptbl": true, "fldinst": true, "operator": true, "creatim": true, "revtim": true, "printim": true, "buptim": true}

func (r *rtfReader) run() {
	r.ucSkip = 1
	d := r.data
	for i := 0; i < len(d); {
		c := d[i]
		switch c {
		case '{':
			if len(r.stack) < 512 { // deeper nesting is hostile, not a document
				r.stack = append(r.stack, r.cur)
			}
			i++
		case '}':
			r.setBold(false)
			r.setItalic(false)
			if n := len(r.stack); n > 0 {
				if r.cur.dest != "" && (n == 1 || r.stack[n-1].dest != r.cur.dest) {
					r.endField()
				}
				r.cur = r.stack[n-1]
				r.stack = r.stack[:n-1]
			}
			i++
		case '\\':
			i = r.control(i + 1)
		case '\r', '\n':
			i++
		default:
			j := i
			for j < len(d) && d[j] != '{' && d[j] != '}' && d[j] != '\\' && d[j] != '\r' && d[j] != '\n' {
				j++
			}
			r.text(string(d[i:j]))
			i = j
		}
	}
}

func (r *rtfReader) control(i int) int {
	d := r.data
	if i >= len(d) {
		return i
	}
	c := d[i]
	if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
		switch c {
		case '\'':
			if i+2 < len(d) {
				if v, ok := hexPair(d[i+1], d[i+2]); ok {
					r.text(cp1252Rune(v))
				}
				return i + 3
			}
			return len(d)
		case '*':
			r.cur.skip = true
		case '~':
			r.text(" ")
		case '_':
			r.text("-")
		case '\\', '{', '}':
			r.text(string(c))
		case '\r', '\n':
			r.endPara()
		}
		return i + 1
	}
	j := i
	for j < len(d) && (d[j] >= 'a' && d[j] <= 'z' || d[j] >= 'A' && d[j] <= 'Z') && j-i < 32 {
		j++
	}
	word := string(d[i:j])
	neg := j < len(d) && d[j] == '-'
	if neg {
		j++
	}
	n, hasNum := 0, false
	for j < len(d) && d[j] >= '0' && d[j] <= '9' && n < 1<<24 {
		n = n*10 + int(d[j]-'0')
		hasNum = true
		j++
	}
	if neg {
		n = -n
	}
	if j < len(d) && d[j] == ' ' {
		j++
	}
	on := !hasNum || n != 0
	switch {
	case rtfSkipped[word]:
		r.cur.skip = true
	case word == "info":
		r.cur.skip = false
	case word == "title" || word == "author":
		r.cur.dest = word
		r.field.Reset()
	case word == "par" || word == "line" || word == "sect" || word == "page":
		r.endPara()
	case word == "tab":
		r.text("\t")
	case word == "emdash":
		r.text("—")
	case word == "endash":
		r.text("–")
	case word == "lquote":
		r.text("‘")
	case word == "rquote":
		r.text("’")
	case word == "ldblquote":
		r.text("“")
	case word == "rdblquote":
		r.text("”")
	case word == "bullet":
		r.text("•")
	case word == "b":
		r.setBold(on)
	case word == "i":
		r.setItalic(on)
	case word == "plain":
		r.setBold(false)
		r.setItalic(false)
	case word == "uc" && hasNum && n >= 0 && n < 16:
		r.ucSkip = n
	case word == "u" && hasNum:
		if n < 0 {
			n += 65536
		}
		r.text(string(rune(n)))
		// The fallback characters that follow are for readers without Unicode.
		for k := 0; k < r.ucSkip && j < len(d); k++ {
			if d[j] == '\\' && j+3 < len(d) && d[j+1] == '\'' {
				j += 4
			} else if d[j] == '{' || d[j] == '}' || d[j] == '\\' {
				break
			} else {
				j++
			}
		}
	}
	return j
}

func hexPair(a, b byte) (byte, bool) {
	h := func(c byte) (byte, bool) {
		switch {
		case c >= '0' && c <= '9':
			return c - '0', true
		case c >= 'a' && c <= 'f':
			return c - 'a' + 10, true
		case c >= 'A' && c <= 'F':
			return c - 'A' + 10, true
		}
		return 0, false
	}
	x, ok1 := h(a)
	y, ok2 := h(b)
	return x<<4 | y, ok1 && ok2
}

func cp1252Rune(v byte) string { return decodeSingle1252(v) }

func (r *rtfReader) text(s string) {
	if r.cur.dest != "" {
		if r.field.Len() < 1000 {
			r.field.WriteString(s)
		}
		return
	}
	if r.cur.skip {
		return
	}
	if r.para.Len()+r.body.Len() > maxPlainInput {
		return
	}
	r.para.WriteString(xmlText(s))
}

func (r *rtfReader) endField() {
	v := strings.TrimSpace(r.field.String())
	switch r.cur.dest {
	case "title":
		r.title = v
	case "author":
		r.author = v
	}
	r.field.Reset()
}

func (r *rtfReader) setBold(on bool) {
	if on != r.openB {
		if r.cur.dest == "" && !r.cur.skip {
			if on {
				r.para.WriteString("<b>")
			} else {
				r.para.WriteString("</b>")
			}
			r.openB = on
		}
	}
}

func (r *rtfReader) setItalic(on bool) {
	if on != r.openI {
		if r.cur.dest == "" && !r.cur.skip {
			if on {
				r.para.WriteString("<i>")
			} else {
				r.para.WriteString("</i>")
			}
			r.openI = on
		}
	}
}

func (r *rtfReader) endPara() {
	r.closeInline()
	p := strings.TrimSpace(r.para.String())
	r.para.Reset()
	if p != "" {
		r.body.WriteString("<p>" + p + "</p>")
	}
}

func (r *rtfReader) closeInline() {
	if r.openI {
		r.para.WriteString("</i>")
		r.openI = false
	}
	if r.openB {
		r.para.WriteString("</b>")
		r.openB = false
	}
}

func (r *rtfReader) close() string {
	r.endPara()
	// Whatever the markers did, the result goes through the sanitiser, which balances it.
	return sanitizeHTML(r.body.String(), htmlOptions{})
}
