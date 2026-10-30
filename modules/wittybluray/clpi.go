package wittybluray

import (
	"errors"
	"fmt"
	"sort"
)

// BDMV/CLIPINF/xxxxx.clpi describes one stream file (an .m2ts clip): its
// programs and their streams, its time bases, and the entry-point map that
// turns a time into a byte position, which is how a player starts a play
// item at its IN time or seeks to a chapter.

// Clip is one .clpi file.
type Clip struct {
	Name    string // five digits
	Version string
	// StreamType: 1 = a normal transport stream. ApplicationType: 1 = main
	// movie, 2 = time-based slideshow, 3 = browsable slideshow, 4 = sub-path
	// (secondary audio or text subtitles), 5 = IG menu, 6 = ...
	StreamType, ApplicationType int
	TSRate                      int    // bytes per second of the transport stream
	Packets                     uint32 // source packets (192 bytes each)
	STC                         []STCSequence
	Programs                    []Program
	EPMaps                      []EPMap
}

// STCSequence is a stretch of the clip with one continuous clock.
type STCSequence struct {
	PCRPID     uint16
	StartSPN   uint32
	Start, End uint32 // presentation times, 45 kHz
}

// Program is one program of the transport stream and its streams.
type Program struct {
	StartSPN uint32
	PMTPID   uint16
	Streams  []ClipStream
}

// ClipStream is one elementary stream as the clip describes it.
type ClipStream struct {
	PID    uint16
	Coding byte
	Format int
	Rate   int
	Aspect int // video: 2 = 4:3, 3 = 16:9
	Lang   string
}

// EPMap maps presentation times to source packet numbers for one stream.
type EPMap struct {
	PID     uint16
	Entries []EPEntry // in time order
}

// EPEntry is one entry point: a packet at which decoding can start.
type EPEntry struct {
	PTS uint32 // 45 kHz
	SPN uint32 // source packet number; byte offset = SPN * 192
	// AngleChange marks the packet as a point where a seamless angle change
	// may happen.
	AngleChange bool
}

// Size is the clip's length in bytes.
func (c *Clip) Size() int64 { return int64(c.Packets) * 192 }

// Stream finds a stream by PID.
func (c *Clip) Stream(pid uint16) *ClipStream {
	for i := range c.Programs {
		for j := range c.Programs[i].Streams {
			if c.Programs[i].Streams[j].PID == pid {
				return &c.Programs[i].Streams[j]
			}
		}
	}
	return nil
}

// VideoPID is the clip's main video stream, or 0.
func (c *Clip) VideoPID() uint16 {
	for _, p := range c.Programs {
		for _, s := range p.Streams {
			switch s.Coding {
			case 0x01, 0x02, 0x1b, 0x24, 0xea:
				return s.PID
			}
		}
	}
	return 0
}

// Duration is the clip's length in seconds from its time bases.
func (c *Clip) Duration() float64 {
	var d float64
	for _, s := range c.STC {
		if s.End > s.Start {
			d += ticks45(s.End - s.Start)
		}
	}
	return d
}

// SPNAt returns the entry point at or before the time (45 kHz) in the
// stream, so a player can start reading there. ok is false when the clip has
// no entry-point map for that stream.
func (c *Clip) SPNAt(pid uint16, pts uint32) (e EPEntry, ok bool) {
	for i := range c.EPMaps {
		m := &c.EPMaps[i]
		if m.PID != pid || len(m.Entries) == 0 {
			continue
		}
		k := sort.Search(len(m.Entries), func(j int) bool { return m.Entries[j].PTS > pts })
		if k == 0 {
			return m.Entries[0], true
		}
		return m.Entries[k-1], true
	}
	return EPEntry{}, false
}

// ParseClip parses one .clpi file.
func ParseClip(name string, b []byte) (*Clip, error) {
	r := &rd{b: b}
	if r.str(0, 4) != "HDMV" {
		return nil, errors.New("not a clip information file")
	}
	c := &Clip{Name: name, Version: r.str(4, 4)}
	seqStart, progStart, cpiStart := int(r.u32(8)), int(r.u32(12)), int(r.u32(16))
	// ClipInfo
	c.StreamType = int(r.u8(46))
	c.ApplicationType = int(r.u8(47))
	c.TSRate = int(r.u32(52))
	c.Packets = r.u32(56)
	// SequenceInfo: ATC sequences, each with STC sequences.
	o := seqStart + 5
	nATC := int(r.u8(o))
	o++
	for i := 0; i < nATC && i < 32 && r.err == nil; i++ {
		nSTC := int(r.u8(o + 4))
		o += 6
		for j := 0; j < nSTC && j < 255 && r.err == nil; j++ {
			c.STC = append(c.STC, STCSequence{PCRPID: r.u16(o), StartSPN: r.u32(o + 2), Start: r.u32(o + 6), End: r.u32(o + 10)})
			o += 14
		}
	}
	// ProgramInfo
	o = progStart + 5
	nProg := int(r.u8(o))
	o++
	for i := 0; i < nProg && i < 255 && r.err == nil; i++ {
		p := Program{StartSPN: r.u32(o), PMTPID: r.u16(o + 4)}
		ns := int(r.u8(o + 6))
		o += 8
		for j := 0; j < ns && j < 255 && r.err == nil; j++ {
			s := ClipStream{PID: r.u16(o)}
			l := int(r.u8(o + 2))
			s.Coding = r.u8(o + 3)
			a := o + 4
			switch s.Coding {
			case 0x01, 0x02, 0x1b, 0x20, 0x24, 0xea:
				s.Format, s.Rate = int(r.u8(a)>>4), int(r.u8(a)&15)
				s.Aspect = int(r.u8(a+1) >> 4)
			case 0x03, 0x04, 0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0xa1, 0xa2:
				s.Format, s.Rate = int(r.u8(a)>>4), int(r.u8(a)&15)
				s.Lang = r.str(a+1, 3)
			case 0x90, 0x91:
				s.Lang = r.str(a, 3)
			case 0x92:
				s.Lang = r.str(a+1, 3)
			}
			p.Streams = append(p.Streams, s)
			o += 3 + l
		}
		c.Programs = append(c.Programs, p)
	}
	// CPI: the entry-point map.
	if cpiStart > 0 && r.u32(cpiStart) > 0 && r.u16(cpiStart+4)&15 == 1 {
		base := cpiStart + 6 // EP_map starts after the length and type
		n := int(r.u8(base + 1))
		o = base + 2
		for i := 0; i < n && i < 32 && r.err == nil; i++ {
			m := EPMap{PID: r.u16(o)}
			// 48 bits: 10 reserved, 4 stream type, 16 coarse entries, 18 fine entries.
			v := uint64(r.u16(o+2))<<32 | uint64(r.u32(o+4))
			nCoarse := int((v >> 18) & 0xffff)
			nFine := int(v & 0x3ffff)
			start := base + int(r.u32(o+8))
			o += 12
			if nCoarse > 100000 || nFine > 1<<18 {
				return nil, fmt.Errorf("entry-point map for PID %d is too large", m.PID)
			}
			fineStart := start + int(r.u32(start))
			coarse := start + 4
			type cEntry struct {
				fineID uint32
				pts    uint32
				spn    uint32
			}
			cs := make([]cEntry, 0, nCoarse)
			for k := 0; k < nCoarse && r.err == nil; k++ {
				w := r.u32(coarse + k*8)
				cs = append(cs, cEntry{fineID: w >> 14, pts: w & 0x3fff, spn: r.u32(coarse + k*8 + 4)})
			}
			for k := range cs {
				end := nFine
				if k+1 < len(cs) {
					end = int(cs[k+1].fineID)
				}
				base := cs[k].spn &^ 0x1ffff
				prevFine := uint32(0)
				for f := int(cs[k].fineID); f < end && f < nFine && r.err == nil; f++ {
					w := r.u32(fineStart + f*4)
					fineSPN := w & 0x1ffff
					if fineSPN < prevFine {
						base += 0x20000 // the 17-bit packet number wrapped within this coarse entry
					}
					prevFine = fineSPN
					// PTS: the coarse entry holds bits 32..19 of the 90 kHz time,
					// the fine entry bits 19..9; the engine keeps 45 kHz.
					pts90 := uint64(cs[k].pts)<<19 | uint64((w>>17)&0x3ff)<<9
					m.Entries = append(m.Entries, EPEntry{PTS: uint32(pts90 / 2), SPN: base | fineSPN, AngleChange: w>>31 != 0})
				}
			}
			c.EPMaps = append(c.EPMaps, m)
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	return c, nil
}
