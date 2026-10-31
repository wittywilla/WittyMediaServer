package wittycd

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// A CloneCD image is an INI file describing the table of contents as it came off
// the disc, and an .img of raw sectors with no sheet-like structure of its own. An
// LBA there is already counted from index 1 of a normal first track, so sector n of
// the .img is LBA n, until a second session: dumps differ over whether the gap
// between sessions is in the file, which only its size can settle.
//
// A scrambled data track makes no difference here, as data is never read, and
// nothing is ever unscrambled. The .sub file isn't needed for anything played.

// ccdEntry is an [Entry n] section as it is read.
type ccdEntry struct {
	session, point, adr, control int
	plba, pmin, psec, pframe     int
	hasPoint, hasPLBA, hasMSF    bool
	bad                          bool
}

// lba says where an entry points, working it out from the minutes, seconds and
// frames when PLBA is missing. Those count the 2-second lead-in, and PLBA doesn't.
func (e *ccdEntry) lba() (int, bool) {
	if e.hasPLBA {
		return e.plba, e.plba >= 0 && e.plba <= MaxSectors
	}
	if !e.hasMSF || e.pmin < 0 || e.pmin > 99 || e.psec < 0 || e.psec > 59 || e.pframe < 0 || e.pframe >= FramesPerSecond {
		return 0, false
	}
	v := (e.pmin*60+e.psec)*FramesPerSecond + e.pframe - LeadIn
	return v, v >= 0 && v <= MaxSectors
}

// ccdTrack is what a [TRACK n] section adds to the table of contents.
type ccdTrack struct {
	mode    int // -1 when it doesn't say
	index0  int // -1 when it doesn't say
	indexes []Index
}

type ccdPoint struct {
	session, control, lba int
}

// ccdNumber reads a number the way these files write them: 0x41 or 41.
func ccdNumber(s string) (int, bool) {
	s = strings.TrimSpace(s)
	var v int64
	var err error
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, err = strconv.ParseInt(s[2:], 16, 32)
	} else {
		v, err = strconv.ParseInt(s, 10, 32)
	}
	return int(v), err == nil
}

// ccdPack reads a line of hex bytes as one CD-TEXT pack. Packs written without their
// checksum get one, so parseCDText can treat them all alike.
func ccdPack(s string) ([]byte, bool) {
	fields := strings.Fields(s)
	if len(fields) != packBytes && len(fields) != packBytes-2 {
		return nil, false
	}
	p := make([]byte, 0, packBytes)
	for _, f := range fields {
		v, err := strconv.ParseUint(f, 16, 8)
		if err != nil {
			return nil, false
		}
		p = append(p, byte(v))
	}
	if len(p) == packBytes-2 {
		crc := ^crcCCITT(p)
		p = append(p, byte(crc>>8), byte(crc))
	}
	return p, true
}

func (d *Disc) readCCD(path string) error {
	raw, err := readSheet(path)
	if err != nil {
		return err
	}
	d.Format = "ccd"
	var points [MaxTracks + 1]*ccdPoint
	var info [MaxTracks + 1]ccdTrack
	for i := range info {
		info[i] = ccdTrack{mode: -1, index0: -1}
	}
	leadouts := map[int]int{}
	var packs []byte
	badText := false

	// Sections are read in whatever order they come. An entry is only known to be
	// complete when the next section starts.
	section, sectionNum := "", 0
	var entry *ccdEntry
	commit := func() {
		if entry == nil {
			return
		}
		e := entry
		entry = nil
		if e.bad || !e.hasPoint {
			d.warn("bad-tracks", "An entry in this CloneCD image's table of contents made no sense and was left out.")
			return
		}
		if e.adr != 0 && e.adr != 1 {
			return // other kinds of entry describe the lead-in, not a track
		}
		if e.session < 1 || e.session > MaxTracks {
			e.session = 1
		}
		lba, ok := e.lba()
		switch {
		case e.point >= 1 && e.point <= MaxTracks:
			if !ok || lba >= MaxSectors {
				d.warn("bad-tracks", "Track %d in this CloneCD image's table of contents has no usable position and was left out.", e.point)
				return
			}
			if points[e.point] == nil {
				points[e.point] = &ccdPoint{e.session, e.control, lba}
			}
		case e.point == 0xa2:
			if _, seen := leadouts[e.session]; !seen && ok {
				leadouts[e.session] = lba
			}
		}
	}
	for line := range strings.SplitSeq(sheetText(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == ';' {
			continue
		}
		if line[0] == '[' {
			commit()
			name := strings.ToLower(strings.TrimSpace(strings.Trim(line, "[]")))
			section, sectionNum = name, 0
			if f := strings.Fields(name); len(f) == 2 {
				n, ok := ccdNumber(f[1])
				if !ok {
					n = -1
				}
				section, sectionNum = f[0], n
			}
			if section == "entry" {
				entry = &ccdEntry{session: 1}
			}
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch section {
		case "disc":
			if key == "catalog" {
				d.Catalog = printable(value)
			}
		case "cdtext":
			if !strings.HasPrefix(key, "entry") {
				continue
			}
			if p, ok := ccdPack(value); ok && len(packs) < 256*packBytes*8 {
				packs = append(packs, p...)
			} else {
				badText = true
			}
		case "entry":
			v, ok := ccdNumber(value)
			switch key {
			case "session":
				entry.session, entry.bad = v, entry.bad || !ok
			case "point":
				entry.point, entry.hasPoint = v, ok
				entry.bad = entry.bad || !ok
			case "adr":
				entry.adr, entry.bad = v, entry.bad || !ok
			case "control":
				entry.control, entry.bad = v, entry.bad || !ok
			case "plba":
				entry.plba, entry.hasPLBA = v, ok
			case "pmin":
				entry.pmin, entry.hasMSF = v, entry.hasMSF || ok
			case "psec":
				entry.psec, entry.hasMSF = v, entry.hasMSF || ok
			case "pframe":
				entry.pframe, entry.hasMSF = v, entry.hasMSF || ok
			}
		case "track":
			if sectionNum < 1 || sectionNum > MaxTracks {
				continue
			}
			t := &info[sectionNum]
			if key == "mode" {
				if v, ok := ccdNumber(value); ok {
					t.mode = v
				}
				continue
			}
			f := strings.Fields(key)
			if len(f) != 2 || f[0] != "index" {
				continue
			}
			n, ok1 := ccdNumber(f[1])
			at, ok2 := ccdNumber(value)
			if !ok1 || !ok2 || n < 0 || n > 99 {
				continue
			}
			if n == 0 {
				t.index0 = at
			} else if n >= 2 && len(t.indexes) < maxIndexes {
				t.indexes = append(t.indexes, Index{n, at})
			}
		}
	}
	commit()
	if badText {
		d.warn("cdtext-damaged", "Some of this CloneCD image's CD-TEXT is damaged, so a few names may be missing.")
	}

	// The tracks, in the order they are on the disc.
	d.Tracks = make([]Track, 0, MaxTracks)
	for n := 1; n <= MaxTracks; n++ {
		p := points[n]
		if p == nil {
			continue
		}
		audio := p.control&0x04 == 0
		mode := "AUDIO"
		if !audio {
			mode = "MODE1/2352"
			if info[n].mode == 2 {
				mode = "MODE2/2352"
			}
		}
		d.Tracks = append(d.Tracks, Track{Number: n, Audio: audio, Mode: mode, Pregap: p.lba, Start: p.lba, Indexes: info[n].indexes,
			PreEmphasis: p.control&0x01 != 0, CopyAllowed: p.control&0x02 != 0, FourChannel: p.control&0x08 != 0, Session: p.session})
	}
	if len(d.Tracks) == 0 {
		return fmt.Errorf("%w: this CloneCD file lists no tracks", ErrNotCD)
	}
	slices.SortStableFunc(d.Tracks, func(a, b Track) int { return a.Start - b.Start })

	// The sectors themselves.
	f, size, err := d.ccdImage(path)
	if err != nil {
		return err
	}
	inFile := int((size + SectorBytes - 1) / SectorBytes)

	// Everything before the first track belongs to it, as a pregap. A later track
	// has a pregap only when its own section says so, and the sectors before it are
	// the end of the track before.
	for i := range d.Tracks {
		t := &d.Tracks[i]
		x := info[t.Number].index0
		switch {
		case i == 0:
			t.Pregap = 0
			if x >= 0 && x <= t.Start {
				t.Pregap = x
			}
		case x >= 0 && x <= t.Start && x > d.Tracks[i-1].Start:
			t.Pregap = x
		}
	}
	gaps := 0
	for i := range d.Tracks {
		t := &d.Tracks[i]
		leadout, known := leadouts[t.Session]
		if i+1 == len(d.Tracks) {
			switch {
			case known && leadout > t.Start:
				t.End = leadout
			case inFile > t.Start:
				t.End = inFile
			default:
				t.End = inFile + gaps
			}
			break
		}
		next := &d.Tracks[i+1]
		t.End = next.Pregap
		if next.Session == t.Session {
			continue
		}
		switch {
		case known && leadout > t.Start && leadout <= next.Pregap:
			t.End = leadout
		case next.Pregap-sessionGap > t.Start:
			t.End = next.Pregap - sessionGap
		}
		gaps += next.Pregap - t.End
	}

	// The file holds the gap if it is long enough to: a file no longer than the disc
	// without its gaps has left them out, and later sessions sit that much earlier.
	last := d.Tracks[len(d.Tracks)-1].End
	gapless := gaps > 0 && inFile <= last-gaps
	shift := 0
	for i := range d.Tracks {
		t := &d.Tracks[i]
		if i > 0 && gapless {
			shift += t.Pregap - d.Tracks[i-1].End
		}
		off := int64(t.Pregap-shift) * SectorBytes
		if off >= size || t.End <= t.Pregap {
			continue
		}
		have := int((size - off + SectorBytes - 1) / SectorBytes)
		d.addExtent(extent{lba: t.Pregap, sectors: min(t.End-t.Pregap, have), src: f, offset: off, declared: true, data: !t.Audio})
	}
	for i := range d.Tracks {
		d.Sessions = max(d.Sessions, d.Tracks[i].Session)
	}
	if len(packs) > 0 {
		d.parseCDText(packs)
	}
	return nil
}

// ccdImage finds the sectors beside the sheet, by the sheet's own name.
func (d *Disc) ccdImage(path string) (io.ReaderAt, int64, error) {
	dir := filepath.Dir(path)
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	p, err := plainFileBeside(dir, base+".img")
	if err != nil {
		var err2 error
		if p, err2 = plainFileBeside(dir, base+".bin"); err2 != nil {
			return nil, 0, err
		}
	}
	f, size, err := d.file(p)
	if err != nil {
		return nil, 0, fmt.Errorf("can't open %q: %w", filepath.Base(p), err)
	}
	return f, size, nil
}
