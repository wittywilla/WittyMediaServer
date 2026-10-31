package wittybluray

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
)

// Reading one elementary stream out of a clip. Blu-ray clips are MPEG
// transport streams in 192-byte source packets (a 4-byte arrival stamp,
// then a 188-byte TS packet). Menu graphics (IG) and subtitles (PG) travel
// in PES packets of private stream 1; this demuxer hands each PES packet of
// one PID, with its presentation time, to a callback. It reads sequentially
// in large chunks and never holds more than one PES packet, so it is light
// even on a 20 GB clip.

const packetSize = 192

// maxPES bounds one PES packet; graphics segments are far smaller.
const maxPES = 4 << 20

// clipReader reads a clip's bytes across its extents.
type clipReader struct {
	ext  []FileExtent
	file *os.File
	path string
}

func (c *clipReader) close() {
	if c.file != nil {
		c.file.Close()
	}
}

// readAt reads clip bytes at a logical offset.
func (c *clipReader) readAt(buf []byte, off int64) (int, error) {
	total := 0
	for _, e := range c.ext {
		if off >= e.Length {
			off -= e.Length
			continue
		}
		if c.file == nil || c.path != e.Path {
			c.close()
			f, err := os.Open(e.Path)
			if err != nil {
				return total, err
			}
			c.file, c.path = f, e.Path
		}
		want := int64(len(buf) - total)
		if avail := e.Length - off; want > avail {
			want = avail
		}
		n, err := c.file.ReadAt(buf[total:total+int(want)], e.Offset+off)
		total += n
		if err != nil && !errors.Is(err, io.EOF) {
			return total, err
		}
		off = 0
		if total >= len(buf) {
			break
		}
	}
	if total < len(buf) {
		return total, io.EOF
	}
	return total, nil
}

// leadPackets is how much of a stretch is searched for its earliest packet: about
// half a second at the highest rate a disc plays at, enough for every stream to
// show up, and little enough to do at the start of a playlist.
const leadPackets = 16384 // 3 MB

// ClipLead says how long before the first picture a stretch of a clip's earliest
// packet lies, in seconds: usually sound, stored ahead of its picture. A demuxer
// puts an input on its timeline by that packet, so whoever joins clips must allow
// for it. video is the picture's PID; 0 when nothing can be told. only, when given,
// limits which streams count.
func (d *Disc) ClipLead(clip string, video uint16, from uint32, only ...uint16) float64 {
	ext, err := d.ClipExtents(clip)
	if err != nil {
		return 0
	}
	rd := &clipReader{ext: ext}
	defer rd.close()
	buf := make([]byte, leadPackets*packetSize)
	got, err := rd.readAt(buf, int64(from)*packetSize)
	if got < packetSize || (err != nil && !errors.Is(err, io.EOF)) {
		return 0
	}
	return scanLead(buf[:got], video, only...)
}

// scanLead is ClipLead on the bytes themselves.
func scanLead(b []byte, video uint16, only ...uint16) float64 {
	seen := map[uint16]bool{}
	var first, earliest, other int64 = -1, -1, -1
	for i := 0; i+packetSize <= len(b); i += packetSize {
		p := b[i+4 : i+packetSize]
		if p[0] != 0x47 || p[1]&0x40 == 0 {
			continue
		}
		pid := uint16(p[1]&0x1f)<<8 | uint16(p[2])
		if seen[pid] || (len(only) > 0 && !slices.Contains(only, pid)) {
			continue
		}
		off := 4
		if p[3]>>4&2 != 0 {
			off += 1 + int(p[4])
		}
		if off+14 > len(p) || p[off] != 0 || p[off+1] != 0 || p[off+2] != 1 || p[off+3] == 0xbf || p[off+7]&0x80 == 0 {
			continue
		}
		q := p[off+9:]
		pts := int64(q[0]>>1&7)<<30 | int64(q[1])<<22 | int64(q[2]>>1)<<15 | int64(q[3])<<7 | int64(q[4]>>1)
		seen[pid] = true
		switch {
		case pid == video:
			first = pts
		case pid >= 0x1200 && pid < 0x1a00: // subtitles and menus
			if other < 0 || pts < other {
				other = pts
			}
			continue
		}
		if earliest < 0 || pts < earliest {
			earliest = pts
		}
	}
	// Subtitles and menus only count when they lie just ahead of the rest.
	if other >= 0 && (earliest < 0 || (other < earliest && earliest-other < 90000)) {
		earliest = other
	}
	if first < 0 || earliest < 0 || earliest >= first {
		return 0
	}
	return float64(first-earliest) / 90000
}

// PESHandler receives one PES packet's payload and 90 kHz presentation
// time; it returns false to stop reading.
type PESHandler func(payload []byte, pts uint64) bool

// ReadPID reads the PES packets of one PID from a clip, from source packet
// `from` up to (not including) `to` (0 = the end).
func (d *Disc) ReadPID(clip string, pid uint16, from, to uint32, fn PESHandler) error {
	ext, err := d.ClipExtents(clip)
	if err != nil {
		return err
	}
	c := d.Clip(clip)
	if c == nil {
		return fmt.Errorf("clip %s: no clip information", clip)
	}
	if to == 0 || to > c.Packets {
		to = c.Packets
	}
	rd := &clipReader{ext: ext}
	defer rd.close()
	const chunkPackets = 16384 // 3 MB
	buf := make([]byte, chunkPackets*packetSize)
	var pes []byte
	var pts uint64
	var pesLen int
	flush := func() bool {
		if len(pes) < 9 {
			pes = pes[:0]
			return true
		}
		hdr := 9 + int(pes[8])
		if pes[3] == 0xbf {
			hdr = 6 // private stream 2 (text subtitles) has no header extension
		}
		if hdr > len(pes) {
			pes = pes[:0]
			return true
		}
		payload := pes[hdr:]
		pes = pes[:0]
		return fn(payload, pts)
	}
	for spn := from; spn < to; {
		n := to - spn
		if n > chunkPackets {
			n = chunkPackets
		}
		got, err := rd.readAt(buf[:n*packetSize], int64(spn)*packetSize)
		if got < packetSize {
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			break
		}
		for i := 0; i+packetSize <= got; i += packetSize {
			p := buf[i+4 : i+packetSize]
			if p[0] != 0x47 {
				continue
			}
			if uint16(p[1]&0x1f)<<8|uint16(p[2]) != pid {
				continue
			}
			pusi := p[1]&0x40 != 0
			afc := p[3] >> 4 & 3
			off := 4
			if afc&2 != 0 { // adaptation field
				off += 1 + int(p[4])
			}
			if afc&1 == 0 || off >= len(p) {
				continue
			}
			payload := p[off:]
			if pusi {
				if len(pes) > 0 && !flush() {
					return nil
				}
				if len(payload) < 9 || payload[0] != 0 || payload[1] != 0 || payload[2] != 1 {
					continue
				}
				pesLen = int(payload[4])<<8 | int(payload[5])
				pts = 0
				if payload[3] != 0xbf && payload[7]&0x80 != 0 && len(payload) >= 14 {
					pts = uint64(payload[9]>>1&7)<<30 | uint64(payload[10])<<22 | uint64(payload[11]>>1)<<15 | uint64(payload[12])<<7 | uint64(payload[13]>>1)
				}
				pes = append(pes[:0], payload...)
			} else if len(pes) > 0 {
				pes = append(pes, payload...)
			}
			if len(pes) > maxPES {
				pes = pes[:0]
			}
			// A PES with a known length is complete as soon as it is all here.
			if pesLen > 0 && len(pes) >= 6+pesLen {
				pes = pes[:6+pesLen]
				if !flush() {
					return nil
				}
			}
		}
		spn += uint32(got / packetSize)
		if err != nil {
			break
		}
	}
	if len(pes) > 0 {
		flush()
	}
	return nil
}
