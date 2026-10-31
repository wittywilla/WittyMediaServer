package wittycd

import (
	"encoding/binary"
	"fmt"
	"io"
	"path/filepath"
)

// A Nero image is its sectors followed by a chain of chunks, with a footer in the
// last few bytes saying where the chain starts. A disc-at-once image has a cue table
// saying where tracks are on the disc and a track table saying where they are in the
// file. A track-at-once image may have only the second. Each session has its own
// tables, in order.
//
// Offsets all come from the file, so each is held against the size of the part of
// the file that holds sectors before it is used.

const (
	// maxNrgChunks is far more chunks than any real image has.
	maxNrgChunks = 4096
	// maxNrgRead bounds the chunks read into memory, however many there are.
	maxNrgRead = 8 * maxSheet
	// maxNrgTables: a disc has no more sessions than tracks.
	maxNrgTables = MaxTracks

	nrgDaoHeader = 22
	nrgDaoXEntry = 42
	nrgDaoIEntry = 30
	nrgEtn2Entry = 32
	nrgEtnfEntry = 20
	nrgTinfEntry = 12
)

type nrgChunk struct {
	id   string
	data []byte
}

// nrgCue is what the cue tables say about a track.
type nrgCue struct {
	table             int
	control           byte
	pregap, start     int
	hasPregap, hasLBA bool
	indexes           []Index
}

// nrgFile is where the track tables say a track is in the file.
type nrgFile struct {
	stride             int64
	mode               byte
	index0, index1, to int64
	isrc               string
	lba                int // from a track-at-once table, -1 when it doesn't say
}

// nrgFooter finds the chain of chunks. limit is where the footer itself starts,
// which is as far as the chain or any sectors can reach.
func nrgFooter(r io.ReaderAt, size int64) (start, limit int64, err error) {
	var tail [12]byte
	if size >= 12 {
		if _, err := r.ReadAt(tail[:], size-12); err != nil {
			return 0, 0, fmt.Errorf("can't read this Nero image: %w", err)
		}
	} else if size >= 8 {
		if _, err := r.ReadAt(tail[4:], size-8); err != nil {
			return 0, 0, fmt.Errorf("can't read this Nero image: %w", err)
		}
	}
	var at uint64
	switch {
	case string(tail[:4]) == "NER5":
		at, limit = binary.BigEndian.Uint64(tail[4:]), size-12
	case string(tail[4:8]) == "NERO":
		at, limit = uint64(binary.BigEndian.Uint32(tail[8:])), size-8
	default:
		return 0, 0, fmt.Errorf("%w: this Nero image has no footer, so it is damaged or cut short", ErrNotCD)
	}
	if at > uint64(limit) || int64(at)+8 > limit {
		return 0, 0, fmt.Errorf("%w: this Nero image's footer points outside the file", ErrNotCD)
	}
	return int64(at), limit, nil
}

// readNrgChunks walks the chain and reads the chunks that matter. A chain that goes
// bad keeps what was found before it.
func (d *Disc) readNrgChunks(r io.ReaderAt, start, limit int64) []nrgChunk {
	var out []nrgChunk
	spent := int64(0)
	pos := start
	for steps := 0; ; steps++ {
		if steps == maxNrgChunks || pos+8 > limit {
			d.warn("nrg-chain", "This Nero image's list of contents doesn't end properly, so it may be damaged.")
			return out
		}
		var h [8]byte
		if _, err := r.ReadAt(h[:], pos); err != nil {
			d.warn("nrg-chain", "This Nero image's list of contents can't be read to its end, so it may be damaged.")
			return out
		}
		id, n := string(h[:4]), int64(binary.BigEndian.Uint32(h[4:]))
		if id == "END!" {
			return out
		}
		if n > maxSheet || pos+8+n > limit {
			d.warn("nrg-chain", "This Nero image's list of contents has a part that is larger than it can be, so everything after it was left out.")
			return out
		}
		switch id {
		case "CUEX", "CUES", "DAOX", "DAOI", "ETN2", "ETNF", "TINF", "CDTX", "SINF":
			if spent+n > maxNrgRead {
				d.warn("nrg-chain", "This Nero image's list of contents is larger than it can be, so everything after that point was left out.")
				return out
			}
			spent += n
			b := make([]byte, n)
			if n > 0 {
				if _, err := r.ReadAt(b, pos+8); err != nil {
					d.warn("nrg-chain", "This Nero image's list of contents can't be read to its end, so it may be damaged.")
					return out
				}
			}
			out = append(out, nrgChunk{id, b})
		}
		pos += 8 + n
	}
}

func bcdByte(b byte) (int, bool) {
	return int(b>>4)*10 + int(b&0x0f), b>>4 <= 9 && b&0x0f <= 9
}

// parseNrgCue reads a cue table. Most tables write track numbers in BCD and some as
// plain numbers, and one table is all one or all the other. A plain table of ten or
// more tracks shows itself with a value BCD can't have; below ten they read the same.
func (d *Disc) parseNrgCue(b []byte, wide bool, table int, cues *[MaxTracks + 1]*nrgCue, leadouts []int) {
	plain := false
	for i := 0; i+8 <= len(b); i += 8 {
		if v := b[i+1]; v != 0xaa && (v>>4 > 9 || v&0x0f > 9) {
			plain = true
			break
		}
	}
	number := func(v byte) (int, bool) {
		if plain {
			return int(v), true
		}
		return bcdByte(v)
	}
	bad := false
	for i := 0; i+8 <= len(b); i += 8 {
		e := b[i : i+8]
		if e[1] == 0 {
			continue // the lead-in
		}
		var lba int
		if wide {
			lba = int(int32(binary.BigEndian.Uint32(e[4:])))
		} else {
			m, ok1 := bcdByte(e[5])
			s, ok2 := bcdByte(e[6])
			f, ok3 := bcdByte(e[7])
			if !ok1 || !ok2 || !ok3 || s > 59 || f >= FramesPerSecond {
				bad = true
				continue
			}
			lba = (m*60+s)*FramesPerSecond + f - LeadIn
		}
		if lba < -LeadIn || lba > MaxSectors {
			bad = true
			continue
		}
		if e[1] == 0xaa {
			if lba > 0 && leadouts[table] < 0 {
				leadouts[table] = lba
			}
			continue
		}
		track, ok1 := number(e[1])
		index, ok2 := number(e[2])
		if !ok1 || !ok2 || track > MaxTracks || index > 99 {
			bad = true
			continue
		}
		c := cues[track]
		if c == nil {
			c = &nrgCue{table: table, control: e[0] >> 4}
			cues[track] = c
		} else if c.table != table {
			bad = true
			continue
		}
		switch {
		case index == 0 && !c.hasPregap:
			c.pregap, c.hasPregap = lba, true
		case index == 1 && !c.hasLBA:
			if lba < 0 || lba >= MaxSectors {
				bad = true
				continue
			}
			c.start, c.hasLBA = lba, true
			c.control = e[0] >> 4
		case index >= 2 && len(c.indexes) < maxIndexes:
			c.indexes = append(c.indexes, Index{index, lba})
		}
	}
	if bad {
		d.warn("bad-tracks", "Some entries in this Nero image's cue table made no sense and were left out.")
	}
}

// addNrgFile checks a track's offsets against the file and records it. The offsets
// are unsigned here, so one far too large for the file can't become a negative one.
func (d *Disc) addNrgFile(files *[MaxTracks + 1]*nrgFile, n int, f nrgFile, i0, i1, end uint64, limit int64) {
	bad := func() {
		d.warn("bad-tracks", "Track %d in this Nero image's track table points outside the file and was left out.", n)
	}
	if n < 1 || n > MaxTracks || files[n] != nil {
		return
	}
	if i1 >= uint64(limit) || end < i1 || !nrgStrideOK(f.stride) {
		bad()
		return
	}
	if end > uint64(limit) {
		d.warn("truncated", "This image is shorter than its track table says: track %d ends early.", n)
		end = uint64(limit)
	}
	if i0 > i1 {
		i0 = i1
	}
	f.index0, f.index1, f.to = int64(i0), int64(i1), int64(end)
	files[n] = &f
}

func nrgStrideOK(s int64) bool {
	switch s {
	case 2048, 2324, 2336, 2352, 2448:
		return true
	}
	return false
}

// nrgModeStride is the sector size a track-at-once table's mode means, 0 for a mode
// nobody knows.
func nrgModeStride(mode uint32) int64 {
	switch mode {
	case 0, 2:
		return 2048
	case 3:
		return 2336
	case 5, 6, 7:
		return 2352
	case 0x0f, 0x10, 0x11:
		return 2448
	}
	return 0
}

func nrgModeIsAudio(mode byte) bool { return mode == 7 || mode == 0x10 || mode == 1 }

// parseNrgDao reads a disc-at-once track table.
func (d *Disc) parseNrgDao(b []byte, wide bool, files *[MaxTracks + 1]*nrgFile, limit int64) {
	if len(b) < nrgDaoHeader {
		d.warn("bad-tracks", "This Nero image's track table is cut short and was left out.")
		return
	}
	if d.Catalog == "" {
		d.Catalog = printable(string(b[4:17]))
	}
	first, last := int(b[20]), int(b[21])
	if first == 0 {
		first = nrgNextFree(files)
	}
	size := nrgDaoIEntry
	if wide {
		size = nrgDaoXEntry
	}
	count := (len(b) - nrgDaoHeader) / size
	if last >= first {
		count = min(count, last-first+1)
	}
	count = min(count, MaxTracks)
	for i := range count {
		e := b[nrgDaoHeader+i*size : nrgDaoHeader+(i+1)*size]
		// The mode is written one byte or two wide, with the other byte zero.
		mode := e[14]
		if mode == 0 {
			mode = e[15]
		}
		f := nrgFile{stride: int64(binary.BigEndian.Uint16(e[12:])), mode: mode, isrc: printable(string(e[:12])), lba: -1}
		var i0, i1, end uint64
		if wide {
			i0, i1, end = binary.BigEndian.Uint64(e[18:]), binary.BigEndian.Uint64(e[26:]), binary.BigEndian.Uint64(e[34:])
		} else {
			i0, i1, end = uint64(binary.BigEndian.Uint32(e[18:])), uint64(binary.BigEndian.Uint32(e[22:])), uint64(binary.BigEndian.Uint32(e[26:]))
		}
		d.addNrgFile(files, first+i, f, i0, i1, end, limit)
	}
}

func nrgNextFree(files *[MaxTracks + 1]*nrgFile) int {
	for n := MaxTracks; n >= 1; n-- {
		if files[n] != nil {
			return n + 1
		}
	}
	return 1
}

// parseNrgEtn reads a track-at-once track table, whose tracks carry on from the
// numbers of the tables before it.
func (d *Disc) parseNrgEtn(id string, b []byte, files *[MaxTracks + 1]*nrgFile, limit int64) {
	size := nrgEtnfEntry
	switch id {
	case "ETN2":
		size = nrgEtn2Entry
	case "TINF":
		size = nrgTinfEntry
	}
	n := nrgNextFree(files)
	for i := 0; i+size <= len(b) && n <= MaxTracks; i += size {
		e := b[i : i+size]
		var off, length uint64
		var mode uint32
		lba := int64(-1)
		if id == "ETN2" {
			off, length, mode = binary.BigEndian.Uint64(e), binary.BigEndian.Uint64(e[8:]), binary.BigEndian.Uint32(e[16:])
			lba = int64(binary.BigEndian.Uint32(e[20:]))
		} else {
			off, length, mode = uint64(binary.BigEndian.Uint32(e)), uint64(binary.BigEndian.Uint32(e[4:])), binary.BigEndian.Uint32(e[8:])
			if id == "ETNF" {
				lba = int64(binary.BigEndian.Uint32(e[12:]))
			}
		}
		stride := nrgModeStride(mode)
		if stride == 0 || length > uint64(limit) {
			d.warn("bad-tracks", "Track %d in this Nero image's track table made no sense and was left out.", n)
			n++
			continue
		}
		if lba >= MaxSectors {
			lba = -1
		}
		d.addNrgFile(files, n, nrgFile{stride: stride, mode: byte(mode), lba: int(lba)}, off, off, off+length, limit)
		n++
	}
}

func (d *Disc) readNRG(path string) error {
	d.Format = "nrg"
	r, size, err := d.file(path)
	if err != nil {
		return fmt.Errorf("can't read %q: %w", filepath.Base(path), err)
	}
	start, limit, err := nrgFooter(r, size)
	if err != nil {
		return err
	}
	chunks := d.readNrgChunks(r, start, limit)

	var cues [MaxTracks + 1]*nrgCue
	var files [MaxTracks + 1]*nrgFile
	leadouts := make([]int, 0, maxNrgTables)
	var sinf []int
	var cdtext []byte
	// Disc-at-once tables say their own track numbers, so they go first.
	for _, c := range chunks {
		switch c.id {
		case "DAOX", "DAOI":
			d.parseNrgDao(c.data, c.id == "DAOX", &files, limit)
		case "CUEX", "CUES":
			if len(leadouts) < maxNrgTables {
				leadouts = append(leadouts, -1)
				d.parseNrgCue(c.data, c.id == "CUEX", len(leadouts)-1, &cues, leadouts)
			}
		case "CDTX":
			if cdtext == nil {
				cdtext = c.data
			}
		case "SINF":
			for i := 0; i+4 <= len(c.data) && len(sinf) <= MaxTracks; i += 4 {
				sinf = append(sinf, int(binary.BigEndian.Uint32(c.data[i:])))
			}
		}
	}
	for _, c := range chunks {
		switch c.id {
		case "ETN2", "ETNF", "TINF":
			d.parseNrgEtn(c.id, c.data, &files, limit)
		}
	}

	d.layNrgTracks(r, &cues, &files, leadouts)
	if len(d.Tracks) == 0 {
		return fmt.Errorf("%w: this Nero image has no usable track table", ErrNotCD)
	}
	d.nrgSessions(sinf)
	if cdtext != nil {
		d.parseCDText(cdtext)
	}
	d.graphicsInSectors()
	return nil
}

// layNrgTracks places each track on the disc. The cue table is believed where there
// is one, and otherwise tracks follow one another. Track 1's usual 2-second pregap
// sits before LBA 0 and is no part of the program, like the same 2 seconds on any
// other kind of image. A longer one is real sectors from LBA 0 up.
func (d *Disc) layNrgTracks(r io.ReaderAt, cues *[MaxTracks + 1]*nrgCue, files *[MaxTracks + 1]*nrgFile, leadouts []int) {
	d.Tracks = make([]Track, 0, MaxTracks)
	cursor := 0
	for n := 1; n <= MaxTracks; n++ {
		f, c := files[n], cues[n]
		if f == nil {
			if c != nil && c.hasLBA {
				d.warn("bad-tracks", "Track %d is in this Nero image's cue table but not in its track table, so it was left out.", n)
			}
			continue
		}
		body := int((f.to - f.index1) / f.stride)
		inFile := int((f.index1 - f.index0) / f.stride)
		tr := Track{Number: n, ISRC: f.isrc, Session: 1}
		tr.Audio = nrgModeIsAudio(f.mode)
		if c != nil && c.hasLBA {
			tr.Audio = c.control&0x04 == 0
			tr.PreEmphasis, tr.CopyAllowed, tr.FourChannel = c.control&0x01 != 0, c.control&0x02 != 0, c.control&0x08 != 0
			tr.Session = c.table + 1
			tr.Start, tr.Pregap = c.start, c.start
			if c.hasPregap && c.pregap <= c.start {
				tr.Pregap = max(c.pregap, 0)
			}
			tr.End = nrgTrackEnd(n, cues, leadouts, c.start+body)
			tr.Indexes = c.indexes
		} else {
			switch {
			case f.lba >= cursor:
				tr.Start, tr.Pregap = f.lba, f.lba
			case len(d.Tracks) == 0:
				tr.Pregap, tr.Start = 0, max(inFile-LeadIn, 0)
			default:
				tr.Pregap, tr.Start = cursor, cursor+inFile
			}
			tr.End = tr.Start + body
		}
		if tr.Audio && f.stride != SectorBytes && f.stride != SectorBytes+96 {
			tr.Audio = false // a sector that small can't hold audio, whatever the table says
		}
		tr.Mode = nrgModeLabel(tr.Audio, f.stride, f.mode)

		// A pregap can be in the file or only in the table of contents.
		pregap := tr.Start - tr.Pregap
		held := min(pregap, inFile)
		d.addExtent(extent{lba: tr.Pregap, sectors: pregap - held, data: !tr.Audio})
		d.addExtent(extent{lba: tr.Start - held, sectors: held, src: r, offset: f.index1 - int64(held)*f.stride, stride: f.stride, data: !tr.Audio})
		d.addExtent(extent{lba: tr.Start, sectors: min(tr.End-tr.Start, body), src: r, offset: f.index1, stride: f.stride, data: !tr.Audio})
		cursor = max(cursor, tr.End)
		d.Tracks = append(d.Tracks, tr)
	}
}

// nrgTrackEnd is where the next thing in a track's own table starts: the next
// track's pregap, or the table's lead-out. A session's last track with neither runs
// as far as its file does.
func nrgTrackEnd(n int, cues *[MaxTracks + 1]*nrgCue, leadouts []int, fallback int) int {
	c := cues[n]
	for m := n + 1; m <= MaxTracks; m++ {
		next := cues[m]
		if next == nil || !next.hasLBA || next.table != c.table {
			continue
		}
		if next.hasPregap && next.pregap <= next.start {
			return max(next.pregap, 0)
		}
		return next.start
	}
	if c.table < len(leadouts) && leadouts[c.table] > c.start {
		return leadouts[c.table]
	}
	return fallback
}

func nrgModeLabel(audio bool, stride int64, mode byte) string {
	switch {
	case audio && stride == SectorBytes+96:
		return "CDG"
	case audio:
		return "AUDIO"
	case stride == 2048 && mode == 2:
		return "MODE2/2048"
	case stride == 2048:
		return "MODE1/2048"
	case stride == 2324:
		return "MODE2/2324"
	case stride == 2336:
		return "MODE2/2336"
	case stride == 2352 && mode == 6:
		return "MODE2/2352"
	case stride == 2352:
		return "MODE1/2352"
	}
	return "MODE1/2448"
}

// nrgSessions settles the session count. A session table is only believed when it
// accounts for exactly the tracks there are; otherwise each cue table is a session.
func (d *Disc) nrgSessions(sinf []int) {
	sum := 0
	for _, n := range sinf {
		if n < 0 || n > MaxTracks {
			sum = -1
			break
		}
		sum += n
	}
	if sum == len(d.Tracks) && len(sinf) > 0 {
		i := 0
		for s, n := range sinf {
			for range n {
				d.Tracks[i].Session = s + 1
				i++
			}
		}
	}
	for i := range d.Tracks {
		d.Sessions = max(d.Sessions, d.Tracks[i].Session)
	}
}
