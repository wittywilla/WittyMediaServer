package wittycd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A cue sheet is a list of files, tracks and index points, written by many
// tools with little agreement. Indexes count in sectors from the start of their
// file, and a track's index 0 is often in the file before its index 1.

type cueIndex struct {
	number, file, msf int
}

type cueTrack struct {
	number          int
	mode            string
	indexes         []cueIndex
	pregap, postgap int
	pre, dcp, four  bool
	isrc            string
	text            Text
	session         int
}

type cueFile struct {
	name      string
	path      string
	bigEndian bool
	declared  bool
	audio     audioFile
	segs      []cueSeg
	sectors   int
	base      int // LBA where the file starts, before any inserted silence
}

// cueSeg: from sector start of a file on, sectors are stride bytes long.
type cueSeg struct {
	start  int
	stride int64
	data   bool
}

// sectorStride is the bytes per sector of a track mode. Unknown modes are taken
// as data, which is never played.
func sectorStride(mode string) (stride int64, audio bool) {
	switch strings.ToUpper(mode) {
	case "AUDIO":
		return SectorBytes, true
	case "CDG":
		return 2448, true
	case "MODE1/2048":
		return 2048, false
	case "MODE2/2336", "CDI/2336":
		return 2336, false
	case "MODE2/2048":
		return 2048, false
	case "MODE2/2324":
		return 2324, false
	}
	return SectorBytes, false
}

// parseMSF reads MM:SS:FF as sectors.
func parseMSF(s string) (int, bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, false
	}
	m, e1 := strconv.Atoi(parts[0])
	sec, e2 := strconv.Atoi(parts[1])
	f, e3 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || e3 != nil || m < 0 || m > 999 || sec < 0 || sec > 99 || f < 0 || f >= FramesPerSecond {
		return 0, false
	}
	return (m*60+sec)*FramesPerSecond + f, true
}

// sheetFields splits a line at spaces, keeping quoted text whole.
func sheetFields(line string) []string {
	var out []string
	var cur strings.Builder
	quoted, in := false, false
	for _, r := range line {
		switch {
		case r == '"':
			quoted = !quoted
			in = true
		case !quoted && (r == ' ' || r == '\t'):
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if in {
		out = append(out, cur.String())
	}
	return out
}

func readSheet(path string) ([]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("can't read %q: %w", filepath.Base(path), err)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q isn't a file", ErrNotCD, filepath.Base(path))
	}
	if st.Size() > maxSheet {
		return nil, fmt.Errorf("%w: %q is far too large to be a sheet", ErrNotCD, filepath.Base(path))
	}
	return os.ReadFile(path)
}

func (d *Disc) readCue(path string) error {
	raw, err := readSheet(path)
	if err != nil {
		return err
	}
	d.Format = "cue"
	dir := filepath.Dir(path)
	var files []*cueFile
	var tracks []*cueTrack
	var cur *cueTrack
	session := 1
	cdtext := ""
	for _, line := range strings.Split(sheetText(raw), "\n") {
		f := sheetFields(strings.TrimRight(line, "\r"))
		if len(f) == 0 {
			continue
		}
		arg := func(i int) string {
			if i < len(f) {
				return strings.TrimSpace(f[i])
			}
			return ""
		}
		textTarget := &d.Text
		if cur != nil {
			textTarget = &cur.text
		}
		switch strings.ToUpper(f[0]) {
		case "FILE":
			if len(f) < 2 || len(files) >= MaxTracks {
				continue
			}
			typ := strings.ToUpper(arg(len(f) - 1))
			files = append(files, &cueFile{name: f[1], bigEndian: typ == "MOTOROLA", declared: typ == "MOTOROLA"})
		case "TRACK":
			n, err := strconv.Atoi(arg(1))
			if err != nil || len(tracks) >= MaxTracks+1 {
				cur = nil
				continue
			}
			cur = &cueTrack{number: n, mode: strings.ToUpper(arg(2)), session: session}
			tracks = append(tracks, cur)
		case "INDEX":
			if cur == nil || len(files) == 0 || len(cur.indexes) >= maxIndexes {
				continue
			}
			n, err := strconv.Atoi(arg(1))
			msf, ok := parseMSF(arg(2))
			if err != nil || !ok || n < 0 || n > 99 {
				continue
			}
			cur.indexes = append(cur.indexes, cueIndex{n, len(files) - 1, msf})
		case "PREGAP", "POSTGAP":
			if v, ok := parseMSF(arg(1)); ok && cur != nil {
				if strings.EqualFold(f[0], "PREGAP") {
					cur.pregap = v
				} else {
					cur.postgap = v
				}
			}
		case "FLAGS":
			if cur != nil {
				for _, fl := range f[1:] {
					switch strings.ToUpper(fl) {
					case "PRE":
						cur.pre = true
					case "DCP":
						cur.dcp = true
					case "4CH":
						cur.four = true
					}
				}
			}
		case "ISRC":
			if cur != nil {
				cur.isrc = printable(arg(1))
			}
		case "CATALOG":
			d.Catalog = printable(arg(1))
		case "CDTEXTFILE":
			cdtext = arg(1)
		case "TITLE":
			textTarget.Title = arg(1)
		case "PERFORMER":
			textTarget.Performer = arg(1)
		case "SONGWRITER":
			textTarget.Songwriter = arg(1)
		case "COMPOSER":
			textTarget.Composer = arg(1)
		case "ARRANGER":
			textTarget.Arranger = arg(1)
		case "MESSAGE":
			textTarget.Message = arg(1)
		case "REM":
			switch strings.ToUpper(arg(1)) {
			case "GENRE":
				d.Genre = arg(2)
			case "DATE":
				d.Date = arg(2)
			case "DISCNUMBER":
				d.DiscNumber, _ = strconv.Atoi(arg(2))
			case "SESSION":
				if n, err := strconv.Atoi(arg(2)); err == nil && n >= 1 && n <= 99 {
					session = n
					d.Sessions = max(d.Sessions, n)
				}
			case "COMPOSER", "SONGWRITER":
				if strings.EqualFold(arg(1), "COMPOSER") {
					textTarget.Composer = arg(2)
				} else {
					textTarget.Songwriter = arg(2)
				}
			}
		}
	}
	if len(tracks) == 0 || len(files) == 0 {
		return fmt.Errorf("%w: this cue sheet lists no tracks", ErrNotCD)
	}

	// What each file holds and how it is laid out.
	var firstErr error
	for i, cf := range files {
		p, err := resolveBeside(dir, cf.name)
		if err != nil {
			// A renamed rip: the sheet's only file under another name.
			if len(files) == 1 {
				p = sameNameAudio(path)
			}
			if p == "" {
				if firstErr == nil {
					firstErr = err
				}
				d.warn("missing-file", "%s", capitalize(err.Error()))
				continue
			}
		}
		af, err := d.openAudioFile(p, cf.bigEndian, cf.declared)
		if err != nil {
			if errors.Is(err, ErrUnsupported) {
				return err
			}
			if firstErr == nil {
				firstErr = err
			}
			d.warn("missing-file", "%s", capitalize(err.Error()))
			continue
		}
		cf.path, cf.audio = p, af
		cf.segs = fileSegments(tracks, i)
		cf.sectors = fileSectors(cf)
	}
	base := 0
	for _, cf := range files {
		cf.base = base
		base += cf.sectors
		if cf.sectors > MaxSectors || base > MaxSectors {
			return fmt.Errorf("%w: this cue sheet describes a disc longer than a CD can be", ErrNotCD)
		}
	}
	d.buildCueTracks(tracks, files)
	if cdtext != "" {
		if p, err := resolveBeside(dir, cdtext); err == nil {
			if b, err := readSheet(p); err == nil {
				d.parseCDText(b)
			}
		}
	} else if p := firstExisting(strings.TrimSuffix(path, filepath.Ext(path)) + ".cdt"); p != "" {
		if b, err := readSheet(p); err == nil {
			d.parseCDText(b)
		}
	}
	d.findGraphics(path, files)
	if len(d.extents) == 0 && firstErr != nil {
		return firstErr
	}
	return nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// sameNameAudio finds the audio beside a sheet when the name inside it is stale.
func sameNameAudio(sheet string) string {
	base := strings.TrimSuffix(sheet, filepath.Ext(sheet))
	return firstExisting(base+".bin", base+".wav", base+".img", base+".flac", base+".BIN", base+".WAV", base+".IMG", base+".FLAC")
}

// fileSegments lists where, in one file, the sector size changes.
func fileSegments(tracks []*cueTrack, file int) []cueSeg {
	var segs []cueSeg
	for _, t := range tracks {
		for _, x := range t.indexes {
			if x.file != file {
				continue
			}
			stride, audio := sectorStride(t.mode)
			start := x.msf
			if len(segs) == 0 {
				start = 0
			}
			if len(segs) == 0 || segs[len(segs)-1].stride != stride || segs[len(segs)-1].data == audio {
				if len(segs) == 0 || start > segs[len(segs)-1].start {
					segs = append(segs, cueSeg{start, stride, !audio})
				}
			}
			break
		}
	}
	return segs
}

// fileSectors works out a file's length in sectors from its segments.
func fileSectors(cf *cueFile) int {
	if len(cf.segs) == 0 {
		return 0
	}
	left := cf.audio.size
	sectors := 0
	for i, s := range cf.segs {
		if i+1 < len(cf.segs) {
			n := cf.segs[i+1].start - s.start
			if int64(n)*s.stride >= left {
				return sectors + int(left/s.stride)
			}
			left -= int64(n) * s.stride
			sectors += n
			continue
		}
		sectors += int(left / s.stride)
		if rest := left % s.stride; rest > 0 && s.stride == SectorBytes && !s.data {
			sectors++ // a last sector that isn't whole plays as far as it goes
		}
	}
	return sectors
}

// byteOf is where a sector of a file is.
func (cf *cueFile) byteOf(sector int) (int64, int64, bool) {
	off := cf.audio.offset
	for i, s := range cf.segs {
		if i+1 == len(cf.segs) || sector < cf.segs[i+1].start {
			return off + int64(sector-s.start)*s.stride, s.stride, s.data
		}
		off += int64(cf.segs[i+1].start-s.start) * s.stride
	}
	return off, SectorBytes, false
}

// buildCueTracks lays the tracks out on the disc, adding extents for each file's
// sectors and for any silence the sheet asks to have generated.
func (d *Disc) buildCueTracks(tracks []*cueTrack, files []*cueFile) {
	shift := 0
	d.Tracks = make([]Track, 0, len(tracks))
	type placed struct {
		tr      *Track
		first   cueIndex
		shift   int
		postgap int
	}
	var laid []placed
	for _, t := range tracks {
		one := -1
		for i, x := range t.indexes {
			if x.number == 1 {
				one = i
				break
			}
		}
		if one < 0 {
			d.warn("bad-tracks", "Track %d has no index 1 in its cue sheet and was left out.", t.number)
			continue
		}
		_, audio := sectorStride(t.mode)
		first := t.indexes[0]
		start := t.indexes[one]
		// Sectors before the first index in a file belong to the track that opens
		// it, as a pregap: that is where a hidden track lives.
		if first.number >= 1 && first.msf > 0 && !fileHasEarlierTrack(tracks, t, first.file) {
			first = cueIndex{0, first.file, 0}
		}
		at := func(x cueIndex) int { return files[x.file].base + x.msf + shift }
		pregap := at(first)
		if t.pregap > 0 {
			d.addExtent(extent{lba: pregap, sectors: t.pregap, data: !audio})
			shift += t.pregap
		}
		tr := Track{Number: t.number, Audio: audio, Mode: t.mode, Pregap: pregap, Start: at(start), PreEmphasis: t.pre, CopyAllowed: t.dcp, FourChannel: t.four,
			ISRC: t.isrc, Session: t.session, Text: t.text}
		for _, x := range t.indexes[one+1:] {
			if x.number > 1 {
				tr.Indexes = append(tr.Indexes, Index{x.number, at(x)})
			}
		}
		d.Tracks = append(d.Tracks, tr)
		laid = append(laid, placed{tr: &d.Tracks[len(d.Tracks)-1], first: first, shift: shift})
		laid[len(laid)-1].postgap = t.postgap
		shift += t.postgap
	}
	// Each track's sectors run to the next track's first index, or the end of its
	// file; files between them belong to it too.
	for i := range laid {
		l := &laid[i]
		from := l.first
		endFile, endSector := len(files)-1, files[len(files)-1].sectors
		if i+1 < len(laid) {
			endFile, endSector = laid[i+1].first.file, laid[i+1].first.msf
		}
		lba := files[from.file].base + from.msf + l.shift
		for f := from.file; f <= endFile && f < len(files); f++ {
			cf := files[f]
			start := 0
			if f == from.file {
				start = from.msf
			}
			stop := cf.sectors
			if f == endFile {
				stop = min(endSector, cf.sectors)
			}
			for start < stop && cf.audio.src != nil {
				off, stride, data := cf.byteOf(start)
				run := stop - start
				for _, s := range cf.segs { // a run has one sector size
					if s.start > start && s.start < start+run {
						run = s.start - start
					}
				}
				d.addExtent(extent{lba: lba, sectors: run, src: cf.audio.src, offset: off, stride: stride, bigEndian: cf.audio.bigEndian, declared: cf.audio.declared, data: data || !l.tr.Audio})
				lba += run
				start += run
			}
		}
		l.tr.End = lba
		if l.postgap > 0 {
			// The track's own postgap: generated silence after it.
			d.addExtent(extent{lba: l.tr.End, sectors: l.postgap})
			l.tr.End += l.postgap
		}
	}
}

func fileHasEarlierTrack(tracks []*cueTrack, t *cueTrack, file int) bool {
	for _, o := range tracks {
		if o == t {
			return false
		}
		for _, x := range o.indexes {
			if x.file == file {
				return true
			}
		}
	}
	return false
}
