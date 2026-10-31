package wittycd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A TOC file is a small language rather than a list of lines: statements can share
// a line, and CD-TEXT sits in nested braces. A track is built from pieces (a part of
// a file, or silence) one after another, so a track's place on the disc only comes
// from adding up what is before it, and a piece with no length runs to the end of
// its file. Tracks therefore start at sector 0 with no 2-second lead-in of their own.

// maxTocPieces bounds what a sheet can make the reader do. It is far more pieces
// than any real disc has.
const maxTocPieces = 4096

// tocPiece is one statement that adds to a track. Positions are in bytes of the
// track's sector layout, which is what sample counts and MM:SS:FF both come down to.
type tocPiece struct {
	name   string // "" for generated silence
	swap   bool
	hash   int64 // the #byte offset into the file
	start  int64
	length int64 // 0: as far as the file goes
}

type tocTrack struct {
	label        string
	stride       int64
	audio        bool
	copyOK, pre  bool
	four         bool
	isrc         string
	text         Text
	pieces       []tocPiece
	startPieces  int // pieces before index 1, -1 when START gives no such place
	startSectors int // the pregap START states, -1 when it doesn't
	indexes      []int
}

// tocMode is the layout of a track mode. A mode nobody has heard of is taken as
// data, which is never played, so its sectors still hold their place.
func tocMode(mode, sub string) (label string, stride int64, audio bool) {
	switch mode {
	case "AUDIO":
		label, stride, audio = "AUDIO", SectorBytes, true
	case "MODE1":
		label, stride = "MODE1/2048", 2048
	case "MODE1_RAW":
		label, stride = "MODE1/2352", 2352
	case "MODE2", "MODE2_FORM_MIX":
		label, stride = "MODE2/2336", 2336
	case "MODE2_FORM1":
		label, stride = "MODE2/2048", 2048
	case "MODE2_FORM2":
		label, stride = "MODE2/2324", 2324
	case "MODE2_RAW":
		label, stride = "MODE2/2352", 2352
	default:
		label, stride = "UNKNOWN", 2352
	}
	if sub == "RW" || sub == "RW_RAW" {
		stride += 96 // sub-channel data follows each sector
		if audio {
			label = "CDG"
		}
	}
	return label, stride, audio
}

// tocToken is a word, a quoted string or one of { } and a comma. A string keeps its
// backslashes: a file name wants them as they are, and text wants them decoded.
type tocToken struct {
	text   string
	quoted bool
}

// tocLexer reads tokens as they are asked for, as a sheet of nothing but short words
// would otherwise cost far more memory to hold than to walk.
type tocLexer struct {
	s   string
	pos int
}

func (l *tocLexer) next() (tocToken, bool) {
	s := l.s
	for l.pos < len(s) {
		c := s[l.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			l.pos++
		case strings.HasPrefix(s[l.pos:], "//"):
			nl := strings.IndexByte(s[l.pos:], '\n')
			if nl < 0 {
				l.pos = len(s)
			} else {
				l.pos += nl
			}
		case c == '{' || c == '}' || c == ',':
			l.pos++
			return tocToken{text: s[l.pos-1 : l.pos]}, true
		case c == '"':
			from := l.pos + 1
			i := from
			for i < len(s) && s[i] != '"' && s[i] != '\n' {
				if s[i] == '\\' && i+1 < len(s) && s[i+1] != '\n' {
					i++
				}
				i++
			}
			i = min(i, len(s))
			l.pos = i
			if i < len(s) && s[i] == '"' {
				l.pos++
			}
			return tocToken{text: s[from:i], quoted: true}, true
		default:
			from := l.pos
			l.pos++
			for l.pos < len(s) {
				c := s[l.pos]
				if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '{' || c == '}' || c == ',' || c == '"' || strings.HasPrefix(s[l.pos:], "//") {
					break
				}
				l.pos++
			}
			return tocToken{text: s[from:l.pos]}, true
		}
	}
	return tocToken{}, false
}

func (l *tocLexer) peek() (tocToken, bool) {
	at := l.pos
	t, ok := l.next()
	l.pos = at
	return t, ok
}

// skipBlock passes over the rest of a brace block whose { has been read.
func (l *tocLexer) skipBlock() {
	for depth := 1; depth > 0; {
		t, ok := l.next()
		if !ok {
			return
		}
		if !t.quoted && t.text == "{" {
			depth++
		} else if !t.quoted && t.text == "}" {
			depth--
		}
	}
}

// tocText decodes a string's escapes for showing: \" and \\ and octal bytes, which
// are ISO 8859-1 when they stand for a character.
func tocText(s string) string {
	if !strings.Contains(s, `\`) {
		return printable(s)
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		if i+3 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteRune(rune(v))
				i += 3
				continue
			}
		}
		if s[i+1] == '"' || s[i+1] == '\\' {
			i++
		}
		b.WriteByte(s[i])
	}
	return printable(b.String())
}

// tocName is a string as a file name: only a quote is escaped in one, as a backslash
// is more likely part of a path from the ripper's PC.
func tocName(s string) string { return strings.ReplaceAll(s, `\"`, `"`) }

// tocPos reads a length or position: MM:SS:FF counts sectors, and a plain number
// counts samples in an audio track and bytes in a data track.
func (t *tocTrack) tocPos(s string) (int64, bool) {
	if strings.Contains(s, ":") {
		v, ok := parseMSF(s)
		if !ok || v > MaxSectors {
			return 0, false
		}
		return int64(v) * t.stride, true
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 0 || v > MaxSectors*t.stride {
		return 0, false
	}
	if t.audio {
		if v > MaxSectors*SectorSamples {
			return 0, false
		}
		return v/SectorSamples*t.stride + v%SectorSamples*SampleBytes, true
	}
	return v, true
}

func (t *tocTrack) sectorsOf(n int64) int {
	return int((n + t.stride - 1) / t.stride)
}

func isDigit(s string) bool { return s != "" && s[0] >= '0' && s[0] <= '9' }

// addPiece adds to a track, as long as the sheet hasn't asked for too many in all.
func (d *Disc) addPiece(t *tocTrack, pieces *int, p tocPiece) {
	if *pieces >= maxTocPieces {
		d.warn("too-many-pieces", "This TOC file builds its tracks from more pieces than any real disc has, so the last of them were left out.")
		return
	}
	*pieces++
	t.pieces = append(t.pieces, p)
}

// parseToc reads the statements of a TOC file. Words it doesn't know are passed
// over, so one strange statement doesn't lose the disc.
func (d *Disc) parseToc(text string) []*tocTrack {
	l := &tocLexer{s: text}
	var tracks []*tocTrack
	var cur *tocTrack
	pieces := 0
	badLength := func() {
		d.warn("bad-lengths", "This TOC file gives a length or position that makes no sense, so that part of it was left out.")
	}
	for {
		tok, ok := l.next()
		if !ok {
			break
		}
		if tok.quoted {
			continue
		}
		if tok.text == "{" {
			l.skipBlock()
			continue
		}
		switch word := strings.ToUpper(tok.text); word {
		case "CATALOG":
			if v, ok := l.peek(); ok && v.quoted {
				l.next()
				d.Catalog = printable(v.text)
			}
		case "CD_TEXT":
			target := &d.Text
			if cur != nil {
				target = &cur.text
			}
			if v, ok := l.peek(); ok && !v.quoted && v.text == "{" {
				l.next()
				d.readTocText(l, target, cur)
			}
		case "TRACK":
			mode, ok := l.peek()
			if !ok || mode.quoted || mode.text == "{" || mode.text == "}" {
				continue
			}
			l.next()
			if len(tracks) > MaxTracks {
				d.warn("bad-tracks", "This TOC file lists more tracks than a CD can hold, so the last of them were left out.")
				return tracks
			}
			sub := ""
			if v, ok := l.peek(); ok && !v.quoted {
				if s := strings.ToUpper(v.text); s == "RW" || s == "RW_RAW" {
					l.next()
					sub = s
				}
			}
			cur = &tocTrack{startPieces: -1, startSectors: -1}
			cur.label, cur.stride, cur.audio = tocMode(strings.ToUpper(mode.text), sub)
			tracks = append(tracks, cur)
		case "NO":
			v, ok := l.peek()
			if !ok || v.quoted || cur == nil {
				continue
			}
			switch strings.ToUpper(v.text) {
			case "COPY":
				l.next()
				cur.copyOK = false
			case "PRE_EMPHASIS":
				l.next()
				cur.pre = false
			}
		case "COPY":
			if cur != nil {
				cur.copyOK = true
			}
		case "PRE_EMPHASIS":
			if cur != nil {
				cur.pre = true
			}
		case "TWO_CHANNEL_AUDIO":
			if cur != nil {
				cur.four = false
			}
		case "FOUR_CHANNEL_AUDIO":
			if cur != nil {
				cur.four = true
			}
		case "ISRC":
			if v, ok := l.peek(); ok && v.quoted {
				l.next()
				if cur != nil {
					cur.isrc = printable(v.text)
				}
			}
		case "SILENCE", "ZERO", "FIFO":
			if word == "FIFO" {
				// A sheet doesn't get to read from anything but a plain file. The
				// length is kept as silence so what follows stays in its place.
				d.warn("fifo", "This TOC file asks to read part of its audio from a pipe, which isn't done, so that part plays as silence.")
				if v, ok := l.peek(); ok && v.quoted {
					l.next()
				}
			}
			// ZERO may name the sector layout before its length.
			for {
				v, ok := l.peek()
				if !ok || v.quoted || isDigit(v.text) || !isTocModeWord(v.text) {
					break
				}
				l.next()
			}
			v, ok := l.peek()
			if !ok || v.quoted || !isDigit(v.text) {
				continue
			}
			l.next()
			if cur == nil {
				continue
			}
			n, ok := cur.tocPos(v.text)
			if !ok {
				badLength()
				continue
			}
			if n > 0 {
				d.addPiece(cur, &pieces, tocPiece{length: n})
			}
		case "FILE", "AUDIOFILE", "DATAFILE":
			name, ok := l.peek()
			if !ok || !name.quoted {
				continue
			}
			l.next()
			p := tocPiece{name: tocName(name.text)}
			bad := false
			var pos []string
			for {
				v, ok := l.peek()
				if !ok || v.quoted {
					break
				}
				if strings.EqualFold(v.text, "SWAP") {
					l.next()
					p.swap = true
					continue
				}
				if strings.HasPrefix(v.text, "#") {
					l.next()
					n := v.text[1:]
					if n == "" {
						if w, ok := l.peek(); ok && !w.quoted && isDigit(w.text) {
							l.next()
							n = w.text
						}
					}
					h, err := strconv.ParseInt(n, 10, 64)
					if err != nil || h < 0 || h > 1<<48 {
						bad = true
					} else {
						p.hash = h
					}
					continue
				}
				if !isDigit(v.text) || len(pos) == 2 {
					break
				}
				l.next()
				pos = append(pos, v.text)
			}
			if cur == nil {
				continue
			}
			// A data file has no start of its own: the first number is its length.
			if word == "DATAFILE" && len(pos) > 0 {
				pos = append([]string{"0"}, pos[:1]...)
			}
			if len(pos) > 0 {
				var ok bool
				if p.start, ok = cur.tocPos(pos[0]); !ok {
					bad = true
				}
			}
			if len(pos) > 1 {
				var ok bool
				if p.length, ok = cur.tocPos(pos[1]); !ok {
					bad = true
				}
				if ok && cur.audio && p.length%cur.stride != 0 {
					d.warn("uneven-lengths", "This TOC file cuts its audio part way through a sector. Those pieces were rounded to whole sectors, so a join there may not be exact.")
				}
			}
			if bad {
				badLength()
				continue
			}
			d.addPiece(cur, &pieces, p)
		case "PREGAP":
			v, ok := l.peek()
			if !ok || v.quoted || !isDigit(v.text) {
				continue
			}
			l.next()
			if cur == nil {
				continue
			}
			n, ok := cur.tocPos(v.text)
			if !ok {
				badLength()
				continue
			}
			if n > 0 {
				d.addPiece(cur, &pieces, tocPiece{length: n})
			}
			cur.startPieces, cur.startSectors = len(cur.pieces), -1
		case "START":
			if cur == nil {
				continue
			}
			if v, ok := l.peek(); ok && !v.quoted && isDigit(v.text) {
				l.next()
				n, ok := cur.tocPos(v.text)
				if !ok {
					badLength()
					continue
				}
				cur.startPieces, cur.startSectors = -1, cur.sectorsOf(n)
				continue
			}
			cur.startPieces, cur.startSectors = len(cur.pieces), -1
		case "INDEX":
			v, ok := l.peek()
			if !ok || v.quoted || !isDigit(v.text) {
				continue
			}
			l.next()
			if cur == nil || len(cur.indexes) >= maxIndexes {
				continue
			}
			n, ok := cur.tocPos(v.text)
			if !ok {
				badLength()
				continue
			}
			cur.indexes = append(cur.indexes, cur.sectorsOf(n))
		}
	}
	return tracks
}

func isTocModeWord(s string) bool {
	switch strings.ToUpper(s) {
	case "AUDIO", "MODE0", "MODE1", "MODE1_RAW", "MODE2", "MODE2_FORM1", "MODE2_FORM2", "MODE2_FORM_MIX", "MODE2_RAW", "RW", "RW_RAW":
		return true
	}
	return false
}

// readTocText reads a CD_TEXT block whose { has been read. Only language 0 is used:
// it is what a player without a language setting shows.
func (d *Disc) readTocText(l *tocLexer, target *Text, track *tocTrack) {
	for {
		tok, ok := l.next()
		if !ok || (!tok.quoted && tok.text == "}") {
			return
		}
		if tok.quoted {
			continue
		}
		switch strings.ToUpper(tok.text) {
		case "{", "LANGUAGE_MAP":
			if tok.text != "{" {
				v, ok := l.peek()
				if !ok || v.quoted || v.text != "{" {
					continue
				}
				l.next()
			}
			l.skipBlock()
		case "LANGUAGE":
			num, ok := l.next()
			if !ok {
				return
			}
			if v, ok := l.peek(); !ok || v.quoted || v.text != "{" {
				continue
			}
			l.next()
			if num.text == "0" {
				d.readTocItems(l, target, track)
			} else {
				l.skipBlock()
			}
		}
	}
}

// readTocItems reads the items of a language block. Binary values in braces say
// nothing a person would read, so they are passed over.
func (d *Disc) readTocItems(l *tocLexer, target *Text, track *tocTrack) {
	for {
		key, ok := l.next()
		if !ok || (!key.quoted && key.text == "}") {
			return
		}
		if key.quoted {
			continue
		}
		if key.text == "{" {
			l.skipBlock()
			continue
		}
		v, ok := l.peek()
		if !ok {
			return
		}
		if !v.quoted {
			if v.text == "{" {
				l.next()
				l.skipBlock()
			}
			continue
		}
		l.next()
		s := tocText(v.text)
		switch strings.ToUpper(key.text) {
		case "TITLE":
			target.Title = s
		case "PERFORMER":
			target.Performer = s
		case "SONGWRITER":
			target.Songwriter = s
		case "COMPOSER":
			target.Composer = s
		case "ARRANGER":
			target.Arranger = s
		case "MESSAGE":
			target.Message = s
		case "GENRE":
			if track == nil {
				d.Genre = s
			}
		case "UPC_EAN":
			if track == nil && d.Catalog == "" {
				d.Catalog = printable(s)
			}
		case "ISRC":
			if track != nil && track.isrc == "" {
				track.isrc = strings.ReplaceAll(s, "-", "")
			}
		}
	}
}

// plainFileBeside is resolveBeside for a file that is about to be opened. Only a
// plain file will do, as opening a pipe a sheet happens to name would wait forever.
func plainFileBeside(dir, name string) (string, error) {
	p, err := resolveBeside(dir, name)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("the file %q this image needs isn't an ordinary file", filepath.Base(p))
	}
	return p, nil
}

type tocFileKey struct {
	name string
	swap bool
}

func (d *Disc) readTocFile(path string) error {
	raw, err := readSheet(path)
	if err != nil {
		return err
	}
	d.Format = "toc"
	tracks := d.parseToc(sheetText(raw))
	if len(tracks) == 0 {
		return fmt.Errorf("%w: this TOC file lists no tracks", ErrNotCD)
	}

	// What each file is, once however many pieces name it.
	names := map[string]bool{}
	for _, t := range tracks {
		for i := range t.pieces {
			if n := t.pieces[i].name; n != "" {
				names[n] = true
			}
		}
	}
	dir := filepath.Dir(path)
	opened := map[tocFileKey]*audioFile{}
	var firstErr error
	failed := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
		d.warn("missing-file", "%s", capitalize(err.Error()))
	}
	for _, t := range tracks {
		for i := range t.pieces {
			p := &t.pieces[i]
			key := tocFileKey{p.name, p.swap}
			if _, seen := opened[key]; seen || p.name == "" {
				continue
			}
			opened[key] = nil
			at, err := plainFileBeside(dir, p.name)
			if err != nil && len(names) == 1 {
				// A renamed rip: the sheet's only file under another name.
				if q := sameNameAudio(path); q != "" {
					at, err = plainFileBeside(dir, filepath.Base(q))
				}
			}
			if err != nil {
				failed(err)
				continue
			}
			// Raw audio in a TOC file is big-endian unless it says SWAP. Nothing
			// declares the first, so a wrong guess can still be put right.
			af, err := d.openAudioFile(at, !p.swap, p.swap)
			if err != nil {
				if errors.Is(err, ErrUnsupported) {
					return err
				}
				failed(err)
				continue
			}
			opened[key] = &af
		}
	}

	// Lay the tracks out, one after another.
	d.Tracks = make([]Track, 0, len(tracks))
	lba, fromFiles := 0, false
	for n, t := range tracks {
		tr := Track{Number: n + 1, Audio: t.audio, Mode: t.label, Pregap: lba, Start: lba, CopyAllowed: t.copyOK, PreEmphasis: t.pre, FourChannel: t.four,
			ISRC: t.isrc, Session: 1, Text: t.text}
		for i := range t.pieces {
			if i == t.startPieces {
				tr.Start = lba
			}
			used, real := d.placeTocPiece(t, &t.pieces[i], lba, opened)
			lba += used
			fromFiles = fromFiles || real
		}
		if t.startPieces == len(t.pieces) {
			tr.Start = lba
		}
		if t.startSectors >= 0 {
			tr.Start = tr.Pregap + t.startSectors
		}
		tr.End = lba
		for i, at := range t.indexes {
			tr.Indexes = append(tr.Indexes, Index{i + 2, tr.Start + at})
		}
		d.Tracks = append(d.Tracks, tr)
	}
	if !fromFiles && firstErr != nil {
		return firstErr
	}
	d.findGraphics(path, nil)
	d.graphicsInSectors()
	return nil
}

// placeTocPiece adds a piece's extent at lba and returns the sectors it holds on the
// disc, and whether any came from a file. A file shorter than the sheet says gives
// fewer sectors than its place on the disc, which finish reports as a truncated
// image; later tracks stay where the sheet put them.
func (d *Disc) placeTocPiece(t *tocTrack, p *tocPiece, lba int, opened map[tocFileKey]*audioFile) (int, bool) {
	if p.name == "" {
		n := t.sectorsOf(p.length)
		d.addExtent(extent{lba: lba, sectors: n, data: !t.audio})
		return n, false
	}
	af := opened[tocFileKey{p.name, p.swap}]
	if af == nil {
		return t.sectorsOf(p.length), false
	}
	begin := max(af.offset, p.hash) + p.start
	stop := af.offset + af.size
	if begin >= stop {
		d.warn("truncated", "This image is shorter than its TOC file says: a part of the audio it names isn't there.")
		return t.sectorsOf(p.length), false
	}
	have := stop - begin
	want := p.length
	if want == 0 {
		want = have
	}
	d.addExtent(extent{lba: lba, sectors: t.sectorsOf(min(want, have)), src: af.src, offset: begin, stride: t.stride,
		bigEndian: af.bigEndian, declared: af.declared, data: !t.audio})
	return t.sectorsOf(want), true
}

// graphicsInSectors looks for CD+G in audio sectors that keep their subcode, for the
// images that aren't cue sheets.
func (d *Disc) graphicsInSectors() {
	if d.sub != nil {
		return
	}
	for i := range d.extents {
		e := &d.extents[i]
		if e.src == nil || e.data || e.stride != SectorBytes+96 {
			continue
		}
		sub := &subcode{src: e.src, offset: e.offset + SectorBytes, stride: e.stride, first: e.lba, size: int64(e.sectors) * e.stride}
		if sub.looksLikeGraphics() {
			d.sub, d.HasGraphics = sub, true
		}
		return
	}
}
