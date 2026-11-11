// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package mpegts reads the parts of an MPEG transport stream a TV tuner needs:
// the scrambling bits, what the stream says it carries (PAT, PMT, SDT, ATSC
// VCT) and the programme guide inside it (DVB EIT, ATSC PSIP).
//
// It never decrypts anything. Scrambled packets are only counted, so that a
// stream can be refused.
package mpegts

import (
	"encoding/binary"
	"errors"
	"io"
	"sync/atomic"
	"time"
	"unicode/utf16"
)

// PacketSize is the size of one transport stream packet.
const PacketSize = 188

// Well-known PIDs.
const (
	PIDPAT  = 0x0000
	PIDSDT  = 0x0011
	PIDEIT  = 0x0012
	PIDPSIP = 0x1FFB
)

// ErrScrambled: the stream is encrypted, and the packets say so.
var ErrScrambled = errors.New("the stream is scrambled")

// Header is what a packet's first four bytes say.
type Header struct {
	PID        int
	Start      bool
	Scrambled  bool
	HasPayload bool
	Payload    []byte
}

// ParsePacket reads one packet. ok is false for anything out of sync.
func ParsePacket(p []byte) (h Header, ok bool) {
	if len(p) < PacketSize || p[0] != 0x47 {
		return h, false
	}
	if p[1]&0x80 != 0 { // transport error
		return h, false
	}
	h.Start = p[1]&0x40 != 0
	h.PID = int(p[1]&0x1f)<<8 | int(p[2])
	h.Scrambled = p[3]&0xc0 != 0
	ctl := p[3] >> 4 & 3
	off := 4
	if ctl&2 != 0 {
		off += 1 + int(p[4])
	}
	if ctl&1 != 0 && off < PacketSize {
		h.HasPayload = true
		h.Payload = p[off:PacketSize]
	}
	return h, true
}

// CRC32 is the MPEG-2 section checksum.
func CRC32(b []byte) uint32 {
	crc := uint32(0xffffffff)
	for _, c := range b {
		crc ^= uint32(c) << 24
		for i := 0; i < 8; i++ {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04c11db7
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// maxSection is the longest private section allowed by the standard.
const maxSection = 4096

// Demux puts sections back together from packets of the PIDs it is asked for.
type Demux struct {
	wanted  map[int]bool
	partial map[int][]byte
	OnSec   func(pid int, section []byte)
}

// NewDemux follows these PIDs.
func NewDemux(pids ...int) *Demux {
	d := &Demux{wanted: map[int]bool{}, partial: map[int][]byte{}}
	for _, p := range pids {
		d.wanted[p] = true
	}
	return d
}

// Add starts following another PID.
func (d *Demux) Add(pid int) { d.wanted[pid] = true }

// Feed takes one parsed packet.
func (d *Demux) Feed(h Header) {
	if !d.wanted[h.PID] || !h.HasPayload || h.Scrambled {
		return
	}
	data := h.Payload
	if h.Start {
		if len(data) == 0 {
			return
		}
		ptr := int(data[0])
		data = data[1:]
		if ptr > len(data) {
			delete(d.partial, h.PID)
			return
		}
		if prev := d.partial[h.PID]; len(prev) > 0 {
			d.append(h.PID, data[:ptr])
		}
		delete(d.partial, h.PID)
		data = data[ptr:]
	} else if _, going := d.partial[h.PID]; !going {
		return
	}
	d.append(h.PID, data)
}

func (d *Demux) append(pid int, data []byte) {
	buf := append(d.partial[pid], data...)
	for len(buf) >= 3 {
		if buf[0] == 0xff { // stuffing to the end of the packet
			buf = nil
			break
		}
		n := 3 + (int(buf[1]&0x0f)<<8 | int(buf[2]))
		if n > maxSection {
			buf = nil
			break
		}
		if len(buf) < n {
			break
		}
		if d.OnSec != nil && validSection(buf[:n]) {
			d.OnSec(pid, append([]byte(nil), buf[:n]...))
		}
		buf = buf[n:]
	}
	if len(buf) == 0 {
		// A later section may still start in this packet's stream; forget the
		// buffer but keep following once the next unit starts.
		delete(d.partial, pid)
		return
	}
	d.partial[pid] = append([]byte(nil), buf...)
}

// validSection checks the checksum when the section has one. Long-form
// sections end in a CRC; the table ids for DVB's TDT/TOT and a few others are
// short and have none.
func validSection(b []byte) bool {
	if b[1]&0x80 == 0 {
		return true
	}
	return len(b) >= 12 && CRC32(b) == 0
}

// --------------------------------------------------------------------------
// Tables
// --------------------------------------------------------------------------

// Program is one entry of the PAT.
type Program struct {
	Number int
	PMTPID int
}

// ES is an elementary stream in a PMT.
type ES struct {
	Type int
	PID  int
}

// PMT says what a programme is made of.
type PMT struct {
	Program int
	PCR     int
	Streams []ES
	CA      bool // a conditional access descriptor: the programme may be encrypted
}

// Service is an entry of DVB's SDT.
type Service struct {
	ID       int
	Name     string
	Provider string
	Type     int
	FreeCA   bool // conditional access: encrypted
}

func forDescriptors(b []byte, f func(tag byte, d []byte)) {
	for len(b) >= 2 {
		n := int(b[1])
		if 2+n > len(b) {
			return
		}
		f(b[0], b[2:2+n])
		b = b[2+n:]
	}
}

// Event is a programme from the broadcast's own guide.
type Event struct {
	ServiceID int // DVB service, or the ATSC source id when ATSC is set
	ATSC      bool
	Start     time.Time
	Duration  time.Duration
	Title     string
	Desc      string
	Rating    string
}

// ParseEIT reads a DVB Event Information Table section.
func ParseEIT(s []byte) []Event {
	if len(s) < 18 || s[0] < 0x4e || s[0] > 0x6f {
		return nil
	}
	end := 3 + (int(s[1]&0x0f)<<8 | int(s[2])) - 4
	if end > len(s) {
		return nil
	}
	service := int(s[3])<<8 | int(s[4])
	var out []Event
	for i := 14; i+12 <= end; {
		e := Event{ServiceID: service}
		e.Start = mjdTime(s[i+2 : i+7])
		e.Duration = bcdDuration(s[i+7 : i+10])
		n := int(s[i+10]&0x0f)<<8 | int(s[i+11])
		i += 12
		if i+n > end {
			break
		}
		forDescriptors(s[i:i+n], func(tag byte, d []byte) {
			switch tag {
			case 0x4d: // short event
				if len(d) >= 5 {
					tl := int(d[3])
					if 4+tl+1 <= len(d) {
						e.Title = DVBText(d[4 : 4+tl])
						dl := int(d[4+tl])
						if 5+tl+dl <= len(d) {
							e.Desc = DVBText(d[5+tl : 5+tl+dl])
						}
					}
				}
			case 0x55: // parental rating: minimum age = value + 3
				if len(d) >= 4 && d[3] > 0 && d[3] <= 0x0f {
					e.Rating = itoa(int(d[3]) + 3)
				}
			}
		})
		i += n
		if e.Title != "" && !e.Start.IsZero() {
			out = append(out, e)
		}
	}
	return out
}

func itoa(n int) string {
	if n >= 10 {
		return string(rune('0'+n/10)) + string(rune('0'+n%10))
	}
	return string(rune('0' + n))
}

func mjdTime(b []byte) time.Time {
	mjd := int(b[0])<<8 | int(b[1])
	if mjd == 0xffff {
		return time.Time{}
	}
	h, m, s := bcd(b[2]), bcd(b[3]), bcd(b[4])
	// MJD 40587 is 1970-01-01.
	return time.Unix(int64(mjd-40587)*86400+int64(h)*3600+int64(m)*60+int64(s), 0).UTC()
}

func bcd(b byte) int { return int(b>>4)*10 + int(b&0x0f) }

func bcdDuration(b []byte) time.Duration {
	return time.Duration(bcd(b[0]))*time.Hour + time.Duration(bcd(b[1]))*time.Minute + time.Duration(bcd(b[2]))*time.Second
}

// DVBText decodes a DVB text field (EN 300 468 annex A). UTF-8 and the
// Latin-1 family are read; the default table is close enough to Latin-1 for
// names and titles. Other character tables come through with the unknown
// characters dropped.
func DVBText(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	switch c := b[0]; {
	case c == 0x15:
		return cleanText(string(b[1:]))
	case c == 0x10 && len(b) >= 3:
		b = b[3:]
	case c == 0x11 && len(b) >= 1:
		b = b[1:]
		u := make([]uint16, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		}
		return cleanText(string(utf16.Decode(u)))
	case c < 0x20:
		b = b[1:]
	}
	r := make([]rune, 0, len(b))
	for _, c := range b {
		switch {
		case c == 0x8a:
			r = append(r, '\n')
		case c >= 0x80 && c < 0xa0:
		default:
			r = append(r, rune(c))
		}
	}
	return cleanText(string(r))
}

func cleanText(s string) string {
	out := make([]rune, 0, len(s))
	for _, c := range s {
		if c >= 0x20 || c == '\n' {
			out = append(out, c)
		}
	}
	for len(out) > 0 && (out[len(out)-1] == ' ' || out[len(out)-1] == '\n') {
		out = out[:len(out)-1]
	}
	return string(out)
}

// --------------------------------------------------------------------------
// ATSC PSIP
// --------------------------------------------------------------------------

// VirtualChannel is an entry of ATSC's channel table.
type VirtualChannel struct {
	Name      string
	Major     int
	Minor     int
	Program   int
	SourceID  int
	Encrypted bool // access_controlled
	Hidden    bool
	Type      int
}

// ParseMGT lists the PIDs of the ATSC event tables (EIT-0, EIT-1…).
func ParseMGT(s []byte) []int {
	if len(s) < 17 || s[0] != 0xc7 {
		return nil
	}
	end := 3 + (int(s[1]&0x0f)<<8 | int(s[2])) - 4
	if end > len(s) {
		return nil
	}
	n := int(s[9])<<8 | int(s[10])
	var out []int
	i := 11
	for k := 0; k < n && i+11 <= end; k++ {
		typ := int(s[i])<<8 | int(s[i+1])
		pid := int(s[i+2]&0x1f)<<8 | int(s[i+3])
		dl := int(s[i+9]&0x0f)<<8 | int(s[i+10])
		i += 11 + dl
		if typ >= 0x0100 && typ <= 0x017f {
			out = append(out, pid)
		}
	}
	return out
}

// gpsEpoch is 1980-01-06. ATSC times count seconds from it, ahead of UTC by
// the leap seconds since then (18 today; the broadcast also states the number
// in the STT, and a few seconds matter little in a guide).
const gpsEpoch = 315964800 - 18

// ParseATSCEIT reads an ATSC Event Information Table section.
func ParseATSCEIT(s []byte) []Event {
	if len(s) < 14 || s[0] != 0xcb {
		return nil
	}
	end := 3 + (int(s[1]&0x0f)<<8 | int(s[2])) - 4
	if end > len(s) {
		return nil
	}
	source := int(s[3])<<8 | int(s[4])
	n := int(s[9])
	var out []Event
	i := 10
	for k := 0; k < n && i+10 <= end; k++ {
		start := binary.BigEndian.Uint32(s[i+2:])
		length := int(s[i+6]&0x0f)<<16 | int(s[i+7])<<8 | int(s[i+8])
		tl := int(s[i+9])
		i += 10
		if i+tl+2 > end {
			break
		}
		title := MultipleString(s[i : i+tl])
		i += tl
		dl := int(s[i]&0x0f)<<8 | int(s[i+1])
		i += 2 + dl
		if title != "" {
			out = append(out, Event{ServiceID: source, ATSC: true, Start: time.Unix(int64(start)+gpsEpoch, 0).UTC(), Duration: time.Duration(length) * time.Second, Title: title})
		}
	}
	return out
}

// MultipleString reads ATSC's multiple_string_structure: the first string, in
// the modes that are plain text. Huffman-compressed strings need tables this
// reader does not carry, and give an empty result.
func MultipleString(b []byte) string {
	if len(b) < 1 || b[0] == 0 {
		return ""
	}
	i := 1
	if i+4 > len(b) {
		return ""
	}
	segs := int(b[i+3])
	i += 4
	var out []rune
	for k := 0; k < segs && i+3 <= len(b); k++ {
		comp, mode, n := b[i], b[i+1], int(b[i+2])
		i += 3
		if i+n > len(b) {
			break
		}
		data := b[i : i+n]
		i += n
		if comp != 0 {
			continue
		}
		switch {
		case mode == 0x3f:
			u := make([]uint16, 0, n/2)
			for j := 0; j+1 < n; j += 2 {
				u = append(u, uint16(data[j])<<8|uint16(data[j+1]))
			}
			out = append(out, utf16.Decode(u)...)
		case mode <= 0x07 || mode >= 0x09 && mode <= 0x10 || mode >= 0x20 && mode <= 0x27 || mode >= 0x30 && mode <= 0x33:
			for _, c := range data {
				out = append(out, rune(mode)<<8|rune(c))
			}
		}
	}
	return cleanText(string(out))
}

// --------------------------------------------------------------------------
// Guard: the stream's gatekeeper
// --------------------------------------------------------------------------

// How many scrambled packets end a stream, and in how many. A few stray bits
// must not stop a clear channel; a real encrypted one sets them on nearly all
// of its audio and video.
const (
	scrambleWindow = 2048
	scrambleLimit  = 512
)

// Guard passes a transport stream through unchanged, while watching it. When
// the stream turns out to be encrypted it stops with ErrScrambled. It also
// collects the guide data going by, which costs almost nothing.
type Guard struct {
	r       io.ReadCloser
	OnEvent func(Event)
	dem     *Demux
	carry   []byte
	seen    int
	bad     int
	fatal   atomic.Bool
	packets atomic.Int64
}

// NewGuard watches r.
func NewGuard(r io.ReadCloser) *Guard {
	g := &Guard{r: r}
	g.dem = NewDemux(PIDEIT, PIDPSIP)
	g.dem.OnSec = g.section
	return g
}

func (g *Guard) section(pid int, s []byte) {
	switch {
	case pid == PIDPSIP && s[0] == 0xc7:
		for _, p := range ParseMGT(s) {
			g.dem.Add(p)
		}
	case g.OnEvent == nil:
	case s[0] == 0xcb:
		for _, e := range ParseATSCEIT(s) {
			g.OnEvent(e)
		}
	case pid == PIDEIT:
		for _, e := range ParseEIT(s) {
			g.OnEvent(e)
		}
	}
}

// Read implements io.Reader.
func (g *Guard) Read(p []byte) (int, error) {
	if g.fatal.Load() {
		return 0, ErrScrambled
	}
	n, err := g.r.Read(p)
	if n > 0 {
		g.inspect(p[:n])
		if g.fatal.Load() {
			return 0, ErrScrambled
		}
	}
	return n, err
}

func (g *Guard) inspect(b []byte) {
	if len(g.carry) > 0 {
		need := PacketSize - len(g.carry)
		if len(b) < need {
			g.carry = append(g.carry, b...)
			return
		}
		g.carry = append(g.carry, b[:need]...)
		g.packet(g.carry)
		g.carry = g.carry[:0]
		b = b[need:]
	}
	for len(b) >= PacketSize {
		if b[0] != 0x47 {
			// Lost sync: look for the next packet start.
			i := 1
			for i < len(b) && b[i] != 0x47 {
				i++
			}
			b = b[i:]
			continue
		}
		g.packet(b[:PacketSize])
		b = b[PacketSize:]
	}
	if len(b) > 0 {
		g.carry = append(g.carry[:0], b...)
	}
}

func (g *Guard) packet(p []byte) {
	h, ok := ParsePacket(p)
	if !ok || h.PID == 0x1fff {
		return
	}
	g.seen++
	g.packets.Add(1)
	if h.Scrambled {
		g.bad++
		if g.bad >= scrambleLimit {
			g.fatal.Store(true)
			return
		}
	}
	if g.seen >= scrambleWindow {
		g.seen, g.bad = 0, 0
	}
	if h.PID == PIDEIT || h.PID == PIDPSIP || (g.OnEvent != nil && g.dem.wanted[h.PID]) {
		g.dem.Feed(h)
	}
}

// Fatal says why the guard stopped the stream, if it did.
func (g *Guard) Fatal() error {
	if g.fatal.Load() {
		return ErrScrambled
	}
	return nil
}

// Verified says enough of the stream has gone by, unscrambled, to call it clear.
func (g *Guard) Verified() bool { return !g.fatal.Load() && g.packets.Load() >= scrambleWindow }

// Close closes the stream underneath.
func (g *Guard) Close() error { return g.r.Close() }

// Underneath is the stream being watched, for a helper handing it to a server that
// watches it for itself: looking twice would cost twice, and only the server's look counts.
func (g *Guard) Underneath() io.ReadCloser { return g.r }

// --------------------------------------------------------------------------
// Picking one programme out of a whole multiplex
// --------------------------------------------------------------------------
