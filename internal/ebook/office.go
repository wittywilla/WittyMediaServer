package ebook

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"io"
	"path"
	"strings"

	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/charmap"
)

func decodeSingle1252(v byte) string {
	out, err := charmap.Windows1252.NewDecoder().Bytes([]byte{v})
	if err != nil {
		return ""
	}
	return string(out)
}

// xmlDecoder is strict about nothing, resolves no outside entities (Go's decoder has
// none to resolve), and reads whatever encoding the file declares.
func xmlDecoder(r io.Reader) *xml.Decoder {
	d := xml.NewDecoder(r)
	d.Strict = false
	d.CharsetReader = charset.NewReaderLabel
	d.Entity = xml.HTMLEntity
	return d
}

// ---------------------------------------------------------------- FictionBook

// fromFB2 reads FictionBook, as XML or zipped. Elements map to chapters by section.
func fromFB2(data []byte) (*Book, error) {
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		zr, err := readZip(data)
		if err != nil {
			return nil, err
		}
		var inner *zip.File
		for _, f := range zr.File {
			if strings.HasSuffix(strings.ToLower(f.Name), ".fb2") {
				inner = f
				break
			}
		}
		if inner == nil {
			return nil, errors.New("no FictionBook in this archive")
		}
		if data, err = readEntry(inner); err != nil {
			return nil, err
		}
	}
	b := &Book{}
	// Pictures are at the end of the file and chapters refer to them, so they go first.
	images := fb2Images(data, b)
	dec := xmlDecoder(bytes.NewReader(data))
	var body strings.Builder
	var stack []string
	var chapterTitle strings.Builder
	inTitle, inInfo, inBody := false, false, false
	sectionDepth := 0
	var coverHref string
	field := ""
	var first, last strings.Builder
	flush := func() {
		b.addChapter(strings.TrimSpace(chapterTitle.String()), body.String())
		body.Reset()
		chapterTitle.Reset()
	}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break // keep what was read from a truncated book
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(stack) > 512 {
				return nil, ErrTooBig
			}
			name := t.Name.Local
			stack = append(stack, name)
			switch name {
			case "title-info":
				inInfo = true
			case "body":
				inBody = true
			case "section":
				if inBody {
					sectionDepth++
					if sectionDepth == 1 {
						flush()
					}
				}
			case "book-title", "first-name", "last-name", "annotation", "date", "lang":
				if inInfo {
					field = name
				}
			case "image":
				href := ""
				for _, a := range t.Attr {
					if a.Name.Local == "href" {
						href = strings.TrimPrefix(a.Value, "#")
					}
				}
				switch {
				case inInfo:
					coverHref = href
				case inBody:
					if n := images[href]; n != "" {
						body.WriteString(`<img src="images/` + xmlText(n) + `" alt=""/>`)
					}
				}
			case "title":
				if inBody {
					inTitle = true
					body.WriteString("<h2>")
				}
			case "p":
				if inBody && !inTitle {
					body.WriteString("<p>")
				}
			case "subtitle":
				if inBody {
					body.WriteString("<h3>")
				}
			case "emphasis":
				if inBody {
					body.WriteString("<em>")
				}
			case "strong":
				if inBody {
					body.WriteString("<strong>")
				}
			case "cite", "epigraph":
				if inBody {
					body.WriteString("<blockquote>")
				}
			case "v":
				if inBody {
					body.WriteString("<p>")
				}
			case "empty-line":
				if inBody {
					body.WriteString("<br/>")
				}
			}
		case xml.EndElement:
			name := t.Name.Local
			if n := len(stack); n > 0 {
				stack = stack[:n-1]
			}
			switch name {
			case "title-info":
				inInfo = false
			case "body":
				flush()
				inBody = false
			case "section":
				if inBody {
					sectionDepth--
					if sectionDepth == 0 {
						flush()
					}
				}
			case "book-title", "first-name", "last-name", "annotation", "date", "lang":
				field = ""
			case "title":
				if inBody {
					inTitle = false
					body.WriteString("</h2>")
				}
			case "p":
				if inBody && !inTitle {
					body.WriteString("</p>")
				} else if inTitle {
					body.WriteString(" ")
					chapterTitle.WriteString(" ")
				}
			case "subtitle":
				if inBody {
					body.WriteString("</h3>")
				}
			case "emphasis":
				if inBody {
					body.WriteString("</em>")
				}
			case "strong":
				if inBody {
					body.WriteString("</strong>")
				}
			case "cite", "epigraph":
				if inBody {
					body.WriteString("</blockquote>")
				}
			case "v":
				if inBody {
					body.WriteString("</p>")
				}
			}
		case xml.CharData:
			text := string(t)
			switch {
			case inInfo && field == "book-title":
				b.Title += text
			case inInfo && field == "first-name":
				first.WriteString(text)
			case inInfo && field == "last-name":
				last.WriteString(text)
			case inInfo && field == "date":
				b.Date += text
			case inInfo && field == "lang":
				b.Language += text
			case inInfo && len(stack) > 0 && hasParent(stack, "annotation"):
				if len(b.Description) < 8000 {
					b.Description += text
				}
			case inBody && len(stack) > 0 && stack[len(stack)-1] != "binary":
				if body.Len() < maxBookText {
					body.WriteString(xmlText(text))
				}
				if inTitle {
					chapterTitle.WriteString(text)
				}
			}
		}
	}
	flush()
	b.Title = strings.TrimSpace(b.Title)
	b.Date = strings.TrimSpace(b.Date)
	b.Language = strings.TrimSpace(b.Language)
	b.Description = strings.TrimSpace(b.Description)
	b.Author = strings.TrimSpace(strings.TrimSpace(first.String()) + " " + strings.TrimSpace(last.String()))
	if n := images[coverHref]; n != "" {
		b.Cover = n
	}
	// Balance whatever a damaged file left open.
	for i := range b.Chapters {
		b.Chapters[i].Body = sanitizeHTML(b.Chapters[i].Body, htmlOptions{image: passImage})
	}
	return b, nil
}

// passImage keeps an image already stored for this book.
func passImage(src string) string {
	name, ok := strings.CutPrefix(src, "images/")
	if !ok || strings.ContainsAny(name, `/\`) {
		return ""
	}
	return name
}

func hasParent(stack []string, want string) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == want {
			return true
		}
	}
	return false
}

// fb2Images stores the book's pictures, and maps their ids to stored names.
func fb2Images(data []byte, b *Book) map[string]string {
	out := map[string]string{}
	dec := xmlDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "binary" {
			continue
		}
		id := ""
		for _, a := range se.Attr {
			if a.Name.Local == "id" {
				id = a.Value
			}
		}
		var raw strings.Builder
		for raw.Len() < maxImage*2 {
			t, err := dec.Token()
			if err != nil {
				return out
			}
			if cd, ok := t.(xml.CharData); ok {
				raw.Write(cd)
				continue
			}
			break
		}
		clean := strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
				return -1
			}
			return r
		}, raw.String())
		if img, err := base64.StdEncoding.DecodeString(clean); err == nil && id != "" {
			if name := b.addImage(img); name != "" {
				out[id] = name
			}
		}
	}
}

// ---------------------------------------------------------------- Word (DOCX)

type docxRels map[string]string

func readRels(zr *zip.Reader, name string) docxRels {
	out := docxRels{}
	raw, err := zipBytes(zr, name)
	if err != nil {
		return out
	}
	var r struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if xmlDecoder(bytes.NewReader(raw)).Decode(&r) == nil {
		for _, it := range r.Items {
			out[it.ID] = it.Target
			if it.Mode == "External" {
				out[it.ID] = "ext:" + it.Target
			}
		}
	}
	return out
}

func fromDOCX(data []byte) (*Book, error) {
	zr, err := readZip(data)
	if err != nil {
		if bytes.HasPrefix(data, []byte("\xD0\xCF\x11\xE0")) {
			return nil, ErrEncrypted // password-protected DOCX is an encrypted Office container
		}
		return nil, err
	}
	doc, err := zipBytes(zr, "word/document.xml")
	if err != nil {
		return nil, err
	}
	b := &Book{}
	if raw, err := zipBytes(zr, "docProps/core.xml"); err == nil {
		var core struct {
			Title   string `xml:"title"`
			Creator string `xml:"creator"`
			Desc    string `xml:"description"`
		}
		if xmlDecoder(bytes.NewReader(raw)).Decode(&core) == nil {
			b.Title, b.Author, b.Description = strings.TrimSpace(core.Title), strings.TrimSpace(core.Creator), strings.TrimSpace(core.Desc)
		}
	}
	rels := readRels(zr, "word/_rels/document.xml.rels")
	if raw, err := zipBytes(zr, "docProps/thumbnail.jpeg"); err == nil {
		b.Cover = b.addImage(raw)
	}
	images := map[string]string{}
	imageFor := func(rid string) string {
		if n, ok := images[rid]; ok {
			return n
		}
		target := rels[rid]
		n := ""
		if target != "" && !strings.HasPrefix(target, "ext:") {
			p := path.Clean(path.Join("word", target))
			if raw, err := zipBytes(zr, p); err == nil {
				n = b.addImage(raw)
			}
		}
		images[rid] = n
		return n
	}
	dec := xmlDecoder(bytes.NewReader(doc))
	var body strings.Builder
	var para strings.Builder
	style := ""
	bold, italic := false, false
	inRPr := false
	depth := 0
	chapterTitle := ""
	flushChapter := func() {
		b.addChapter(chapterTitle, body.String())
		body.Reset()
		chapterTitle = ""
	}
	endPara := func() {
		text := strings.TrimSpace(para.String())
		para.Reset()
		lvl := headingLevel(style)
		style = ""
		if text == "" {
			return
		}
		switch {
		case lvl == 1:
			flushChapter()
			chapterTitle = strings.TrimSpace(tagRe.ReplaceAllString(text, ""))
			body.WriteString("<h1>" + text + "</h1>")
		case lvl > 1:
			tag := "h" + string(rune('0'+min(lvl, 6)))
			body.WriteString("<" + tag + ">" + text + "</" + tag + ">")
		default:
			body.WriteString("<p>" + text + "</p>")
		}
	}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth++; depth > 512 {
				return nil, ErrTooBig
			}
			switch t.Name.Local {
			case "pStyle":
				style = attrLocal(t, "val")
			case "rPr":
				inRPr = true
				bold, italic = false, false
			case "b":
				if inRPr && attrLocal(t, "val") != "0" && attrLocal(t, "val") != "false" {
					bold = true
				}
			case "i":
				if inRPr && attrLocal(t, "val") != "0" && attrLocal(t, "val") != "false" {
					italic = true
				}
			case "br", "cr":
				para.WriteString("<br/>")
			case "tab":
				if !inRPr {
					para.WriteString(" ")
				}
			case "blip":
				if n := imageFor(attrLocal(t, "embed")); n != "" {
					para.WriteString(`<img src="images/` + xmlText(n) + `" alt=""/>`)
				}
			case "r":
				bold, italic = false, false
			}
		case xml.EndElement:
			depth--
			switch t.Name.Local {
			case "rPr":
				inRPr = false
			case "p":
				endPara()
			case "r":
				bold, italic = false, false
			}
		case xml.CharData:
			if len(t) == 0 || inRPr {
				continue
			}
			text := xmlText(string(t))
			if strings.TrimSpace(text) == "" && !strings.Contains(string(t), " ") {
				continue
			}
			if bold {
				text = "<b>" + text + "</b>"
			}
			if italic {
				text = "<i>" + text + "</i>"
			}
			if para.Len() < 1<<24 {
				para.WriteString(text)
			}
		}
	}
	flushChapter()
	for i := range b.Chapters {
		b.Chapters[i].Body = sanitizeHTML(b.Chapters[i].Body, htmlOptions{image: passImage})
	}
	return b, nil
}

func attrLocal(t xml.StartElement, name string) string {
	for _, a := range t.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func headingLevel(style string) int {
	low := strings.ToLower(style)
	if !strings.HasPrefix(low, "heading") && !strings.HasPrefix(low, "titel") && low != "title" {
		return 0
	}
	if low == "title" || low == "titel" {
		return 1
	}
	n := 0
	for _, c := range low {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	if n == 0 || n > 9 {
		return 0
	}
	return n
}

// ---------------------------------------------------------------- OpenDocument

func fromODT(data []byte) (*Book, error) {
	zr, err := readZip(data)
	if err != nil {
		return nil, err
	}
	content, err := zipBytes(zr, "content.xml")
	if err != nil {
		return nil, err
	}
	b := &Book{}
	if raw, err := zipBytes(zr, "meta.xml"); err == nil {
		var m struct {
			Meta struct {
				Title   string `xml:"title"`
				Creator string `xml:"creator"`
			} `xml:"meta"`
		}
		if xmlDecoder(bytes.NewReader(raw)).Decode(&m) == nil {
			b.Title, b.Author = strings.TrimSpace(m.Meta.Title), strings.TrimSpace(m.Meta.Creator)
		}
	}
	dec := xmlDecoder(bytes.NewReader(content))
	var body, para strings.Builder
	chapterTitle := ""
	inText := false
	stack := []string{}
	headLevel := 0
	flush := func() {
		b.addChapter(chapterTitle, body.String())
		body.Reset()
		chapterTitle = ""
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(stack) > 512 {
				return nil, ErrTooBig
			}
			stack = append(stack, t.Name.Local)
			switch t.Name.Local {
			case "text":
				inText = true
			case "h":
				headLevel = 1
				if v := attrLocal(t, "outline-level"); v != "" && v[0] >= '1' && v[0] <= '6' {
					headLevel = int(v[0] - '0')
				}
				para.Reset()
			case "p":
				para.Reset()
			case "line-break":
				para.WriteString("<br/>")
			case "s", "tab":
				para.WriteString(" ")
			case "image":
				if raw, err := zipBytes(zr, path.Clean(attrLocal(t, "href"))); err == nil {
					if n := b.addImage(raw); n != "" {
						para.WriteString(`<img src="images/` + xmlText(n) + `" alt=""/>`)
					}
				}
			}
		case xml.EndElement:
			if n := len(stack); n > 0 {
				stack = stack[:n-1]
			}
			switch t.Name.Local {
			case "h":
				text := strings.TrimSpace(para.String())
				if text != "" {
					if headLevel == 1 {
						flush()
						chapterTitle = strings.TrimSpace(tagRe.ReplaceAllString(text, ""))
					}
					tag := "h" + string(rune('0'+headLevel))
					body.WriteString("<" + tag + ">" + text + "</" + tag + ">")
				}
				headLevel = 0
				para.Reset()
			case "p":
				if text := strings.TrimSpace(para.String()); text != "" {
					body.WriteString("<p>" + text + "</p>")
				}
				para.Reset()
			}
		case xml.CharData:
			if inText && para.Len() < 1<<24 {
				para.WriteString(xmlText(string(t)))
			}
		}
	}
	flush()
	for i := range b.Chapters {
		b.Chapters[i].Body = sanitizeHTML(b.Chapters[i].Body, htmlOptions{image: passImage})
	}
	return b, nil
}

// ---------------------------------------------------------------- Word 97-2003

const (
	oleFree = 0xFFFFFFFF
	oleEnd  = 0xFFFFFFFE
)

// oleFile reads streams from the compound-file container old Office files use. Every
// sector number and chain comes from the file: each is checked, and each chain has a
// length it can't walk past, so a loop in a chain just ends.
type oleFile struct {
	data    []byte
	sector  int
	fat     []uint32
	entries map[string]oleEntry
}

type oleEntry struct {
	start uint32
	size  uint64
}

func openOLE(d []byte) (*oleFile, error) {
	if len(d) < 512 || !bytes.HasPrefix(d, []byte("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1")) {
		return nil, errors.New("not an Office file")
	}
	shift := binary.LittleEndian.Uint16(d[30:])
	if shift != 9 && shift != 12 {
		return nil, errors.New("this Office file is damaged")
	}
	o := &oleFile{data: d, sector: 1 << shift, entries: map[string]oleEntry{}}
	fatSectors := int(binary.LittleEndian.Uint32(d[44:]))
	dirStart := binary.LittleEndian.Uint32(d[48:])
	if fatSectors <= 0 || fatSectors > len(d)/o.sector+1 {
		return nil, errors.New("this Office file is damaged")
	}
	// The first 109 FAT sector numbers are in the header, the rest in a chain.
	var difat []uint32
	for i := 0; i < 109 && len(difat) < fatSectors; i++ {
		difat = append(difat, binary.LittleEndian.Uint32(d[76+i*4:]))
	}
	next := binary.LittleEndian.Uint32(d[68:])
	for hops := 0; next != oleEnd && next != oleFree && len(difat) < fatSectors && hops < 1<<16; hops++ {
		s := o.sectorAt(next)
		if s == nil {
			break
		}
		for i := 0; i+4 < len(s); i += 4 {
			difat = append(difat, binary.LittleEndian.Uint32(s[i:]))
		}
		next = binary.LittleEndian.Uint32(s[len(s)-4:])
	}
	for _, n := range difat[:min(len(difat), fatSectors)] {
		s := o.sectorAt(n)
		if s == nil {
			continue
		}
		for i := 0; i+4 <= len(s); i += 4 {
			o.fat = append(o.fat, binary.LittleEndian.Uint32(s[i:]))
		}
	}
	dir, err := o.chain(dirStart, 1<<30)
	if err != nil {
		return nil, err
	}
	for i := 0; i+128 <= len(dir) && i < 128*65536; i += 128 {
		e := dir[i : i+128]
		nameLen := int(binary.LittleEndian.Uint16(e[64:]))
		if nameLen < 2 || nameLen > 64 {
			continue
		}
		var name strings.Builder
		for k := 0; k+1 < nameLen-2; k += 2 {
			name.WriteRune(rune(binary.LittleEndian.Uint16(e[k:])))
		}
		o.entries[name.String()] = oleEntry{start: binary.LittleEndian.Uint32(e[116:]), size: binary.LittleEndian.Uint64(e[120:])}
	}
	return o, nil
}

func (o *oleFile) sectorAt(n uint32) []byte {
	off := (int64(n) + 1) * int64(o.sector)
	if n >= 0xFFFFFFFA || off+int64(o.sector) > int64(len(o.data)) {
		return nil
	}
	return o.data[off : off+int64(o.sector)]
}

func (o *oleFile) chain(start uint32, limit int) ([]byte, error) {
	var out []byte
	seen := 0
	for n := start; n != oleEnd && n != oleFree; {
		s := o.sectorAt(n)
		if s == nil || int(n) >= len(o.fat) {
			if len(out) > 0 {
				return out, nil
			}
			return nil, errors.New("this Office file is damaged")
		}
		out = append(out, s...)
		if len(out) > limit {
			return nil, ErrTooBig
		}
		if seen++; seen > len(o.data)/o.sector+1 {
			return nil, errors.New("this Office file is damaged")
		}
		n = o.fat[n]
	}
	return out, nil
}

func (o *oleFile) stream(name string) ([]byte, error) {
	e, ok := o.entries[name]
	if !ok {
		return nil, errors.New("no " + name + " in this file")
	}
	if e.size > maxEntry {
		return nil, ErrTooBig
	}
	b, err := o.chain(e.start, maxEntry)
	if err != nil {
		return nil, err
	}
	if uint64(len(b)) > e.size {
		b = b[:e.size]
	}
	return b, nil
}

// fromDOC reads the text of a Word 97-2003 file through its piece table. Short
// streams live in a separate mini stream, but a Word document's main streams are
// always large enough to be read directly.
func fromDOC(d []byte) (*Book, error) {
	o, err := openOLE(d)
	if err != nil {
		return nil, err
	}
	word, err := o.stream("WordDocument")
	if err != nil {
		return nil, err
	}
	if len(word) < 0x200 || binary.LittleEndian.Uint16(word) != 0xA5EC {
		return nil, errors.New("this isn't a Word document")
	}
	flags := binary.LittleEndian.Uint16(word[0x0A:])
	if flags&0x0100 != 0 {
		return nil, ErrEncrypted
	}
	tableName := "0Table"
	if flags&0x0200 != 0 {
		tableName = "1Table"
	}
	table, err := o.stream(tableName)
	if err != nil {
		return nil, err
	}
	clxOff := int(binary.LittleEndian.Uint32(word[0x01A2:]))
	clxLen := int(binary.LittleEndian.Uint32(word[0x01A6:]))
	if clxOff < 0 || clxLen <= 0 || clxOff+clxLen > len(table) || clxOff+clxLen < clxOff {
		return nil, errors.New("this Word document is damaged")
	}
	clx := table[clxOff : clxOff+clxLen]
	// Formatting runs may come first; the piece table starts with 2.
	i := 0
	for i < len(clx) && clx[i] == 1 {
		if i+3 > len(clx) {
			return nil, errors.New("this Word document is damaged")
		}
		i += 3 + int(binary.LittleEndian.Uint16(clx[i+1:]))
	}
	if i+5 > len(clx) || clx[i] != 2 {
		return nil, errors.New("this Word document is damaged")
	}
	size := int(binary.LittleEndian.Uint32(clx[i+1:]))
	plc := clx[i+5:]
	if size < 16 || size > len(plc) {
		return nil, errors.New("this Word document is damaged")
	}
	plc = plc[:size]
	n := (size - 4) / 12
	var text strings.Builder
	for p := 0; p < n; p++ {
		cpA := int(binary.LittleEndian.Uint32(plc[p*4:]))
		cpB := int(binary.LittleEndian.Uint32(plc[(p+1)*4:]))
		pcd := plc[(n+1)*4+p*8:]
		fc := binary.LittleEndian.Uint32(pcd[2:])
		compressed := fc&0x40000000 != 0
		fc &= 0x3FFFFFFF
		count := cpB - cpA
		if count <= 0 || count > maxBookText {
			continue
		}
		if compressed {
			start := int(fc) / 2
			if start < 0 || start+count > len(word) {
				continue
			}
			out, err := charmap.Windows1252.NewDecoder().Bytes(word[start : start+count])
			if err == nil {
				text.Write(out)
			}
		} else {
			start := int(fc)
			if start < 0 || start+count*2 > len(word) {
				continue
			}
			for k := 0; k < count; k++ {
				text.WriteRune(rune(binary.LittleEndian.Uint16(word[start+k*2:])))
			}
		}
		if text.Len() > maxBookText {
			return nil, ErrTooBig
		}
	}
	b := &Book{}
	var body strings.Builder
	depth := 0
	var para strings.Builder
	flush := func() {
		if s := strings.TrimSpace(para.String()); s != "" {
			body.WriteString("<p>" + xmlText(s) + "</p>")
			if body.Len() > textChunk {
				b.addChapter("", body.String())
				body.Reset()
			}
		}
		para.Reset()
	}
	for _, r := range text.String() {
		switch r {
		case 0x13: // the start of a field: its code is not text
			depth++
		case 0x14:
			if depth > 0 {
				depth--
			}
		case 0x15:
			if depth > 0 {
				depth--
			}
		case '\r', 0x07, 0x0C:
			if depth == 0 {
				flush()
			}
		case 0x0B:
			if depth == 0 {
				para.WriteString(" ")
			}
		default:
			if depth == 0 && r >= 0x20 {
				para.WriteRune(r)
			}
		}
	}
	flush()
	b.addChapter("", body.String())
	return b, nil
}
