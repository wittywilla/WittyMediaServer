package wittycd

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// A few short looks at the audio settle what no sheet says: which way round its
// bytes are, whether anything is hiding before track 1, and whether what is there
// is music at all. Scanning stays cheap because this reads at most a few
// megabytes of a disc.

const (
	inspectWindows = 12
	inspectSectors = 16
)

// inspectAudio looks at samples spread over the disc.
func (d *Disc) inspectAudio() {
	audio := d.AudioTracks()
	buf := make([]byte, inspectSectors*SectorBytes)
	type look struct{ le, be, level float64 }
	var looks []look
	total := 0
	for _, t := range audio {
		total += t.End - t.Start
	}
	dts := 0
	for w := 0; w < inspectWindows && total > 0; w++ {
		at := (w*2 + 1) * total / (inspectWindows * 2)
		lba := -1
		for _, t := range audio {
			if n := t.End - t.Start; at < n {
				lba = t.Start + at
				break
			} else {
				at -= n
			}
		}
		if lba < 0 {
			continue
		}
		e := d.extentAt(lba)
		if e == nil || e.src == nil {
			continue
		}
		// Read as stored, so both byte orders can be judged.
		n := len(buf)
		if err := d.readRawWindow(e, lba, buf[:n]); err != nil {
			continue
		}
		le, be, level := roughness(buf[:n])
		if level == 0 {
			continue
		}
		looks = append(looks, look{le, be, level})
		if hasDTSSync(buf[:n]) {
			dts++
		}
	}
	if len(looks) == 0 {
		return
	}
	var le, be float64
	for _, l := range looks {
		le += l.le
		be += l.be
	}
	// A file read the wrong way round is many times rougher than it should be.
	if be*6 < le && d.flipUndeclared(true) {
		d.warn("byte-order", "This image's audio is stored big-endian, though its sheet doesn't say so. It is being played the right way round.")
		le, be = be, le
	} else if le*6 < be && d.flipUndeclared(false) {
		d.warn("byte-order", "This image's audio is stored little-endian, though its sheet says otherwise. It is being played the right way round.")
		le, be = be, le
	}
	_ = be
	if dts*2 > len(looks) {
		d.Problem = "dts"
		d.warn("dts", "This is a DTS surround disc: its tracks hold encoded surround sound rather than plain CD audio, and would play as loud noise.")
		return
	}
	noisy := 0
	for _, l := range looks {
		r := l.le
		if l.be < r {
			r = l.be
		}
		if r > 1.15 {
			noisy++
		}
	}
	if noisy*4 >= len(looks)*3 {
		d.Problem = "noise"
		d.warn("noise", "This image's audio looks like noise rather than music. It may be damaged, scrambled or copy protected. WittyCD doesn't unscramble anything, so a clean copy is the way to play it.")
	}
	d.findHiddenTrack()
}

// readRawWindow reads sectors as they are stored, for judging the byte order.
func (d *Disc) readRawWindow(e *extent, lba int, p []byte) error {
	for len(p) > 0 {
		e = d.extentAt(lba)
		if e == nil || e.src == nil {
			clear(p)
			return nil
		}
		n := min(len(p), SectorBytes)
		if err := readFull(e.src, p[:n], e.offset+int64(lba-e.lba)*e.stride); err != nil {
			return err
		}
		p = p[n:]
		lba++
	}
	return nil
}

// roughness is the average change from one sample to the next, over the average
// level, read each way round. Music is smooth; bytes read backwards, or white
// noise, aren't.
func roughness(b []byte) (le, be, level float64) {
	var lePrev, bePrev [2]int32
	var leDiff, beDiff, leAbs, beAbs float64
	n := 0
	for i := 0; i+3 < len(b); i += 4 {
		for c := 0; c < 2; c++ {
			p := b[i+c*2:]
			l, r := int32(int16(binary.LittleEndian.Uint16(p))), int32(int16(binary.BigEndian.Uint16(p)))
			leDiff += abs32(l - lePrev[c])
			beDiff += abs32(r - bePrev[c])
			leAbs += abs32(l)
			beAbs += abs32(r)
			lePrev[c], bePrev[c] = l, r
		}
		n += 2
	}
	if n == 0 || leAbs == 0 || beAbs == 0 {
		return 0, 0, 0
	}
	return leDiff / leAbs, beDiff / beAbs, leAbs / float64(n)
}

func abs32(v int32) float64 {
	if v < 0 {
		return float64(-v)
	}
	return float64(v)
}

// hasDTSSync looks for the sync words of DTS in PCM, 16-bit and 14-bit forms.
func hasDTSSync(b []byte) bool {
	for i := 0; i+6 <= len(b); i += 2 {
		switch {
		case b[i] == 0xfe && b[i+1] == 0x7f && b[i+2] == 0x01 && b[i+3] == 0x80:
			return true
		case b[i] == 0xff && b[i+1] == 0x1f && b[i+2] == 0x00 && b[i+3] == 0xe8 && b[i+4]&0xf0 == 0xf0 && b[i+5] == 0x07:
			return true
		}
	}
	return false
}

// flipUndeclared turns round the file extents whose byte order was a guess.
// It returns whether anything could be.
func (d *Disc) flipUndeclared(big bool) bool {
	changed := false
	for i := range d.extents {
		e := &d.extents[i]
		if e.src != nil && !e.declared && !e.data && e.bigEndian != big {
			e.bigEndian = big
			changed = true
		}
	}
	return changed
}

// findHiddenTrack checks whether track 1's pregap holds real audio. Pregaps often
// hold a few stray bits of dither, so it takes a real level to count.
func (d *Disc) findHiddenTrack() {
	audio := d.AudioTracks()
	if len(audio) == 0 || audio[0].Start-audio[0].Pregap < FramesPerSecond {
		return
	}
	first := audio[0]
	buf := make([]byte, inspectSectors*SectorBytes)
	span := first.Start - first.Pregap
	for w := 0; w < inspectWindows; w++ {
		lba := first.Pregap + (w*2+1)*span/(inspectWindows*2)
		n := min(first.Start-lba, inspectSectors) * SectorBytes
		if n <= 0 || d.readAudio(lba, 0, buf[:n]) != nil {
			continue
		}
		for i := 0; i+1 < n; i += 2 {
			v := int16(binary.LittleEndian.Uint16(buf[i:]))
			if v > 256 || v < -256 {
				d.HiddenTrack = true
				return
			}
		}
	}
}

// readRaw takes bare audio with no sheet: an .iso, .img or .bin that isn't a data
// disc. It is one long track, as nothing says where any others start.
func (d *Disc) readRaw(path string) error {
	d.Format = "raw"
	f, size, err := d.file(path)
	if err != nil {
		return fmt.Errorf("can't read %q: %w", filepath.Base(path), err)
	}
	if size < SectorBytes*FramesPerSecond || size%SectorBytes != 0 || size > MaxSectors*SectorBytes {
		return ErrNotCD
	}
	if looksLikeData(f, size) {
		return ErrNotCD
	}
	sectors := int(size / SectorBytes)
	// Bare files have no words to go by, so it has to sound like audio. A sheetless
	// data file of the right size is far likelier than bare audio.
	buf := make([]byte, inspectSectors*SectorBytes)
	smooth, seen := 0, 0
	for w := 0; w < inspectWindows; w++ {
		off := int64((w*2+1)*sectors/(inspectWindows*2)) * SectorBytes
		if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
			continue
		}
		le, be, level := roughness(buf)
		if level == 0 {
			continue
		}
		seen++
		if min(le, be) < 0.9 {
			smooth++
		}
	}
	if seen == 0 || smooth*2 < seen {
		return ErrNotCD
	}
	d.addExtent(extent{lba: 0, sectors: sectors, src: f})
	d.Tracks = []Track{{Number: 1, Audio: true, Mode: "AUDIO", Start: 0, End: sectors}}
	d.Text.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	d.warn("no-sheet", "This image has no sheet saying where its tracks are, so it plays as one long track.")
	return nil
}

// looksLikeData spots a file system or a CD data sector at the places they must be.
func looksLikeData(r io.ReaderAt, size int64) bool {
	var b [32]byte
	for _, at := range []int64{16 * 2048, 16 * 2352, 16*2352 + 16, 16*2352 + 24, 256 * 2048} {
		if at+int64(len(b)) > size {
			continue
		}
		if _, err := r.ReadAt(b[:], at); err != nil {
			continue
		}
		if string(b[1:6]) == "CD001" || string(b[1:6]) == "BEA01" || string(b[1:6]) == "NSR02" || string(b[1:6]) == "NSR03" {
			return true
		}
	}
	if _, err := r.ReadAt(b[:12], 0); err == nil && isDataSync(b[:12]) {
		return true
	}
	if _, err := r.ReadAt(b[:12], 16*2352); err == nil && isDataSync(b[:12]) {
		return true
	}
	return false
}

func isDataSync(b []byte) bool {
	if b[0] != 0 || b[11] != 0 {
		return false
	}
	for _, c := range b[1:11] {
		if c != 0xff {
			return false
		}
	}
	return true
}

// IsSilentRange reports whether a run of sectors is digital silence. The player
// uses it to tell a real pause from an empty pregap.
func (d *Disc) IsSilentRange(from, to int) bool {
	buf := make([]byte, 64*SectorBytes)
	for lba := from; lba < to; lba += 64 {
		n := min(to-lba, 64) * SectorBytes
		if d.readAudio(lba, 0, buf[:n]) != nil {
			return false
		}
		for _, c := range buf[:n] {
			if c != 0 {
				return false
			}
		}
	}
	return true
}

var _ = os.ErrNotExist
