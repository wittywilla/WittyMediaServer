package wittybluray

import (
	"errors"
	"fmt"
	"io"
	"os"
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
				if payload[7]&0x80 != 0 && len(payload) >= 14 {
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
