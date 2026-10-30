package mpegts

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// section adds the length and checksum to a long-form section.
func section(table byte, ext int, body []byte) []byte {
	s := []byte{table, 0xb0, 0, byte(ext >> 8), byte(ext), 0xc1, 0, 0}
	s = append(s, body...)
	n := len(s) - 3 + 4
	s[1] |= byte(n >> 8)
	s[2] = byte(n)
	crc := CRC32(s)
	return append(s, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
}

// packets splits a section into TS packets.
func packets(pid int, sec []byte) []byte {
	var out []byte
	first := true
	cc := byte(0)
	for len(sec) > 0 {
		p := bytes.Repeat([]byte{0xff}, PacketSize)
		p[0], p[1], p[2], p[3] = 0x47, byte(pid>>8), byte(pid), 0x10|cc
		cc = (cc + 1) & 0x0f
		off := 4
		if first {
			p[1] |= 0x40
			p[4] = 0
			off = 5
			first = false
		}
		n := copy(p[off:], sec)
		sec = sec[n:]
		out = append(out, p...)
	}
	return out
}

func TestPAT(t *testing.T) {
	s := section(0, 1, []byte{0, 0, 0xe0, 0x10, 0, 3, 0xe1, 0x00, 0, 4, 0xe1, 0x10})
	got := ParsePAT(s)
	if len(got) != 2 || got[0] != (Program{3, 0x100}) || got[1] != (Program{4, 0x110}) {
		t.Fatalf("%+v", got)
	}
}

func TestDemuxLongSectionAcrossPackets(t *testing.T) {
	body := make([]byte, 400)
	for i := range body {
		body[i] = 0x55
	}
	sec := section(0x42, 1, body)
	var got []byte
	d := NewDemux(0x11)
	d.OnSec = func(pid int, s []byte) { got = s }
	data := packets(0x11, sec)
	for i := 0; i+PacketSize <= len(data); i += PacketSize {
		h, ok := ParsePacket(data[i : i+PacketSize])
		if !ok {
			t.Fatal("bad packet")
		}
		d.Feed(h)
	}
	if !bytes.Equal(got, sec) {
		t.Fatalf("section not put back together: %d vs %d bytes", len(got), len(sec))
	}
}

func TestDemuxRejectsBadChecksum(t *testing.T) {
	sec := section(0, 1, []byte{0, 3, 0xe1, 0x00})
	sec[len(sec)-1] ^= 0xff
	called := false
	d := NewDemux(0)
	d.OnSec = func(int, []byte) { called = true }
	h, _ := ParsePacket(packets(0, sec))
	d.Feed(h)
	if called {
		t.Fatal("a damaged section must be dropped")
	}
}

func pmtBody(pcr int, ca bool, streams ...ES) []byte {
	var prog []byte
	if ca {
		prog = []byte{0x09, 4, 0x01, 0x00, 0xe1, 0x00}
	}
	b := []byte{0xe0 | byte(pcr>>8), byte(pcr), 0xf0 | byte(len(prog)>>8), byte(len(prog))}
	b = append(b, prog...)
	for _, e := range streams {
		b = append(b, byte(e.Type), 0xe0|byte(e.PID>>8), byte(e.PID), 0xf0, 0)
	}
	return b
}

func TestPMTAndCA(t *testing.T) {
	clear := section(2, 5, pmtBody(0x101, false, ES{0x1b, 0x101}, ES{0x0f, 0x102}))
	p, ok := ParsePMT(clear)
	if !ok || p.Program != 5 || p.CA || p.PCR != 0x101 || len(p.Streams) != 2 || !p.HasVideo() || !p.HD() {
		t.Fatalf("%+v", p)
	}
	enc := section(2, 6, pmtBody(0x101, true, ES{0x02, 0x101}))
	p, ok = ParsePMT(enc)
	if !ok || !p.CA {
		t.Fatalf("the CA descriptor was missed: %+v", p)
	}
	radio := section(2, 7, pmtBody(0x101, false, ES{0x03, 0x101}))
	if p, _ := ParsePMT(radio); p.HasVideo() {
		t.Fatal("a radio service has no video")
	}
}

func TestSDT(t *testing.T) {
	name, prov := []byte("Channel One"), []byte("Provider")
	d := []byte{0x48, byte(3 + len(name) + len(prov)), 1, byte(len(prov))}
	d = append(d, prov...)
	d = append(d, byte(len(name)))
	d = append(d, name...)
	entry := []byte{0, 42, 0xfc, 0x80 | 0x10, byte(len(d))}
	entry = append(entry, d...)
	body := append([]byte{0, 1, 0xff}, entry...)
	got := ParseSDT(section(0x42, 1, body))
	if len(got) != 1 || got[0].ID != 42 || got[0].Name != "Channel One" || got[0].Provider != "Provider" || !got[0].FreeCA {
		t.Fatalf("%+v", got)
	}
}

func TestEIT(t *testing.T) {
	title, desc := []byte("News at Ten"), []byte("The day's stories")
	sd := append([]byte{'e', 'n', 'g', byte(len(title))}, title...)
	sd = append(sd, byte(len(desc)))
	sd = append(sd, desc...)
	d := append([]byte{0x4d, byte(len(sd))}, sd...)
	d = append(d, 0x55, 4, 'g', 'b', 'r', 9) // minimum age 12
	// 2026-10-30 is MJD 61343.
	mjd := 40587 + int(time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC).Unix()/86400)
	ev := []byte{0, 1, byte(mjd >> 8), byte(mjd), 0x22, 0x00, 0x00, 0x00, 0x30, 0x00, 0x80 | byte(len(d)>>8), byte(len(d))}
	ev = append(ev, d...)
	body := append([]byte{0, 1, 0, 1, 0, 0x4e}, ev...)
	got := ParseEIT(section(0x4e, 77, body))
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	e := got[0]
	want := time.Date(2026, 10, 30, 22, 0, 0, 0, time.UTC)
	if e.ServiceID != 77 || !e.Start.Equal(want) || e.Duration != 30*time.Minute || e.Title != "News at Ten" || e.Desc != "The day's stories" || e.Rating != "12" {
		t.Fatalf("%+v", e)
	}
}

func mss(text string) []byte {
	b := []byte{1, 'e', 'n', 'g', 1, 0, 0, byte(len(text))}
	return append(b, text...)
}

func TestATSCEIT(t *testing.T) {
	title := mss("Evening News")
	start := time.Date(2026, 10, 30, 22, 0, 0, 0, time.UTC).Unix() - gpsEpoch
	ev := make([]byte, 10)
	binary.BigEndian.PutUint32(ev[2:], uint32(start))
	ev[6], ev[7], ev[8] = 0, 0x07, 0x08 // 1800 seconds
	ev[9] = byte(len(title))
	ev = append(ev, title...)
	ev = append(ev, 0xf0, 0)
	body := append([]byte{0, 1}, ev...)
	got := ParseATSCEIT(section(0xcb, 9, body))
	if len(got) != 1 || got[0].ServiceID != 9 || !got[0].ATSC || got[0].Title != "Evening News" || got[0].Duration != 30*time.Minute {
		t.Fatalf("%+v", got)
	}
	if !got[0].Start.Equal(time.Date(2026, 10, 30, 22, 0, 0, 0, time.UTC)) {
		t.Fatalf("start %v", got[0].Start)
	}
}

func TestMultipleStringSkipsHuffman(t *testing.T) {
	b := []byte{1, 'e', 'n', 'g', 1, 1, 0, 2, 0xaa, 0xbb}
	if got := MultipleString(b); got != "" {
		t.Fatalf("compressed strings should come out empty, got %q", got)
	}
}

func TestVCT(t *testing.T) {
	entry := make([]byte, 32)
	for i, c := range "WKID" {
		binary.BigEndian.PutUint16(entry[2*i:], uint16(c))
	}
	major, minor := 5, 1
	binary.BigEndian.PutUint32(entry[14:], uint32(0xf)<<28|uint32(major)<<18|uint32(minor)<<8|4)
	entry[24], entry[25] = 0, 3
	entry[26] = 0x20
	entry[27] = 2
	entry[28], entry[29] = 0, 9
	body := append([]byte{0, 1}, entry...)
	got := ParseVCT(section(0xc8, 1, body))
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	v := got[0]
	if v.Name != "WKID" || v.Major != 5 || v.Minor != 1 || v.Program != 3 || v.SourceID != 9 || !v.Encrypted {
		t.Fatalf("%+v", v)
	}
}

func tsPacket(pid int, scrambled bool) []byte {
	p := bytes.Repeat([]byte{0x47}, PacketSize)
	p[1], p[2], p[3] = byte(pid>>8), byte(pid), 0x10
	if scrambled {
		p[3] |= 0x80
	}
	return p
}

type readCloser struct{ io.Reader }

func (readCloser) Close() error { return nil }

func TestGuardStopsAnEncryptedStream(t *testing.T) {
	var data []byte
	for i := 0; i < 1000; i++ {
		data = append(data, tsPacket(0x100, true)...)
	}
	g := NewGuard(readCloser{bytes.NewReader(data)})
	_, err := io.Copy(io.Discard, g)
	if !errors.Is(err, ErrScrambled) || !errors.Is(g.Fatal(), ErrScrambled) {
		t.Fatalf("an encrypted stream must be stopped: %v", err)
	}
}

func TestGuardPassesAClearStreamUnchanged(t *testing.T) {
	var data []byte
	for i := 0; i < 3000; i++ {
		data = append(data, tsPacket(0x100, i%100 == 0)...) // the odd stray bit is not encryption
	}
	g := NewGuard(readCloser{bytes.NewReader(data)})
	var out bytes.Buffer
	// Small reads split packets, which must not matter.
	if _, err := io.CopyBuffer(&out, struct{ io.Reader }{g}, make([]byte, 100)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), data) || g.Fatal() != nil {
		t.Fatal("a clear stream must pass untouched")
	}
}

func TestGuardCollectsGuide(t *testing.T) {
	title := []byte("Show")
	sd := append([]byte{'e', 'n', 'g', byte(len(title))}, title...)
	sd = append(sd, 0)
	d := append([]byte{0x4d, byte(len(sd))}, sd...)
	mjd := 61000
	ev := []byte{0, 1, byte(mjd >> 8), byte(mjd), 0x20, 0, 0, 0, 0x30, 0, 0x80, byte(len(d))}
	ev = append(ev, d...)
	body := append([]byte{0, 1, 0, 1, 0, 0x4e}, ev...)
	data := packets(PIDEIT, section(0x4e, 77, body))
	g := NewGuard(readCloser{bytes.NewReader(data)})
	var got []Event
	g.OnEvent = func(e Event) { got = append(got, e) }
	if _, err := io.Copy(io.Discard, g); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Show" || got[0].ServiceID != 77 {
		t.Fatalf("%+v", got)
	}
}

func TestProgramFilter(t *testing.T) {
	pat := section(0, 1, []byte{0, 3, 0xe1, 0x00, 0, 4, 0xe2, 0x00})
	pmt := section(2, 3, pmtBody(0x101, false, ES{0x02, 0x101}, ES{0x04, 0x102}))
	var data []byte
	data = append(data, packets(0, pat)...)
	data = append(data, packets(0x100, pmt)...)
	data = append(data, tsPacket(0x101, false)...)
	data = append(data, tsPacket(0x102, false)...)
	data = append(data, tsPacket(0x201, false)...) // another programme
	data = append(data, packets(0, pat)...)
	f := NewProgramFilter(readCloser{bytes.NewReader(data)}, 3)
	out, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	pids := map[int]int{}
	for i := 0; i+PacketSize <= len(out); i += PacketSize {
		h, ok := ParsePacket(out[i : i+PacketSize])
		if !ok {
			t.Fatal("output out of sync")
		}
		pids[h.PID]++
		if h.PID == 0 {
			dem := NewDemux(0)
			var progs []Program
			dem.OnSec = func(_ int, s []byte) { progs = ParsePAT(s) }
			dem.Feed(h)
			if len(progs) != 1 || progs[0].Number != 3 || progs[0].PMTPID != 0x100 {
				t.Fatalf("the PAT should list only this programme: %+v", progs)
			}
		}
	}
	if pids[0x101] != 1 || pids[0x102] != 1 || pids[0x100] != 1 || pids[0x201] != 0 {
		t.Fatalf("wrong packets kept: %v", pids)
	}
}

func TestDVBText(t *testing.T) {
	cases := map[string]string{
		"\x15caf\xc3\xa9":     "café",
		"Plain name":          "Plain name",
		"\x05Tab\xe9":         "Tabé",
		"Line\x8anext":        "Line\nnext",
		"\x10\x00\x01Tab\xe9": "Tabé",
	}
	for in, want := range cases {
		if got := DVBText([]byte(in)); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func FuzzParsers(f *testing.F) {
	f.Add(section(0, 1, []byte{0, 3, 0xe1, 0}))
	f.Add(section(2, 3, pmtBody(1, true, ES{2, 3})))
	f.Add(section(0x42, 1, []byte{0, 1, 0xff, 0, 1, 0xfc, 0x80, 0}))
	f.Add(section(0x4e, 1, []byte{0, 1, 0, 1, 0, 0x4e}))
	f.Add(section(0xc8, 1, []byte{0, 1}))
	f.Add(section(0xc7, 1, []byte{0, 0, 1}))
	f.Add(section(0xcb, 1, []byte{0, 1}))
	f.Fuzz(func(t *testing.T, b []byte) {
		ParsePAT(b)
		ParsePMT(b)
		ParseSDT(b)
		ParseEIT(b)
		ParseVCT(b)
		ParseMGT(b)
		ParseATSCEIT(b)
		MultipleString(b)
		DVBText(b)
		if len(b) >= PacketSize {
			ParsePacket(b)
		}
		g := NewGuard(readCloser{strings.NewReader(string(b))})
		_, _ = io.Copy(io.Discard, g)
	})
}
