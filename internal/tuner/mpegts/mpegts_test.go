// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package mpegts

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"
	"unicode/utf8"
)

// packet makes one 188-byte packet: payload only, or an adaptation field of adapt bytes first.
func packet(pid int, start bool, scrambled bool, adapt int, payload []byte) []byte {
	p := make([]byte, PacketSize)
	p[0] = 0x47
	p[1] = byte(pid >> 8 & 0x1f)
	if start {
		p[1] |= 0x40
	}
	p[2] = byte(pid)
	ctl := byte(1)
	off := 4
	if adapt >= 0 {
		ctl |= 2
		p[4] = byte(adapt)
		off += 1 + adapt
	}
	p[3] = ctl << 4
	if scrambled {
		p[3] |= 0x80
	}
	n := copy(p[off:], payload)
	for i := off + n; i < PacketSize; i++ {
		p[i] = 0xff
	}
	return p
}

// section makes a long-form section of table id with body, its length and CRC filled in.
func section(table byte, body []byte) []byte {
	n := len(body) + 4 // and the CRC
	s := append([]byte{table, 0xb0 | byte(n>>8&0x0f), byte(n)}, body...)
	return binary.BigEndian.AppendUint32(s, CRC32(s))
}

func TestParsePacket(t *testing.T) {
	good := packet(0x100, true, false, -1, []byte{1, 2, 3})
	for _, tc := range []struct {
		name    string
		p       []byte
		ok      bool
		pid     int
		start   bool
		scr     bool
		payload int
	}{
		{"payload only", good, true, 0x100, true, false, PacketSize - 4},
		{"too short", good[:PacketSize-1], false, 0, false, false, 0},
		{"out of sync", append([]byte{0x46}, good[1:]...), false, 0, false, false, 0},
		{"transport error", func() []byte { p := bytes.Clone(good); p[1] |= 0x80; return p }(), false, 0, false, false, 0},
		{"adaptation field first", packet(0x1ffb, false, false, 10, []byte{9}), true, 0x1ffb, false, false, PacketSize - 4 - 11},
		{"adaptation field filling the packet", packet(0x12, false, false, 183, nil), true, 0x12, false, false, 0},
		{"adaptation field longer than the packet", func() []byte { p := packet(0x12, false, false, 0, nil); p[4] = 250; return p }(), true, 0x12, false, false, 0},
		{"adaptation only", func() []byte { p := packet(0x12, false, false, 0, nil); p[3] = 2 << 4; return p }(), true, 0x12, false, false, 0},
		{"scrambled", packet(0x44, false, true, -1, nil), true, 0x44, false, true, PacketSize - 4},
		{"longer than a packet", append(bytes.Clone(good), 1, 2, 3), true, 0x100, true, false, PacketSize - 4},
	} {
		h, ok := ParsePacket(tc.p)
		if ok != tc.ok {
			t.Errorf("%s: ok %v", tc.name, ok)
			continue
		}
		if !ok {
			continue
		}
		if h.PID != tc.pid || h.Start != tc.start || h.Scrambled != tc.scr || len(h.Payload) != tc.payload || h.HasPayload != (tc.payload > 0) {
			t.Errorf("%s: %+v (payload %d)", tc.name, h, len(h.Payload))
		}
	}
}

func TestCRC32(t *testing.T) {
	// The check value of CRC-32/MPEG-2.
	if got := CRC32([]byte("123456789")); got != 0x0376e6e7 {
		t.Errorf("CRC32 of 123456789 is %08x", got)
	}
	// A section with its CRC after it checks to zero.
	if s := section(0x42, []byte("hello")); CRC32(s) != 0 || !validSection(s) {
		t.Errorf("a section with its CRC doesn't check: %08x", CRC32(s))
	}
	bad := section(0x42, []byte("hello"))
	bad[5] ^= 1
	if validSection(bad) {
		t.Error("a damaged section passed")
	}
	// Short sections have no CRC to check.
	if !validSection([]byte{0x70, 0x00, 0x05, 1, 2, 3, 4, 5}) {
		t.Error("a short-form section was refused")
	}
}

func TestDemux(t *testing.T) {
	sec := section(0x4e, bytes.Repeat([]byte{0xab}, 20))
	long := section(0x4e, bytes.Repeat([]byte{0xcd}, 300)) // over two packets
	var got [][]byte
	follow := map[int]bool{PIDEIT: true}
	d := NewDemux(PIDEIT)
	d.OnSec = func(pid int, s []byte) {
		if !follow[pid] {
			t.Errorf("section from PID %d", pid)
		}
		got = append(got, s)
	}
	feed := func(p []byte) {
		h, ok := ParsePacket(p)
		if !ok {
			t.Fatal("a test packet didn't parse")
		}
		d.Feed(h)
	}

	// One section in one packet, after a pointer field of 0.
	feed(packet(PIDEIT, true, false, -1, append([]byte{0}, sec...)))
	// A long one across two packets.
	first := append([]byte{0}, long[:PacketSize-5]...)
	feed(packet(PIDEIT, true, false, -1, first))
	feed(packet(PIDEIT, false, false, -1, long[PacketSize-5:]))
	if len(got) != 2 || !bytes.Equal(got[0], sec) || !bytes.Equal(got[1], long) {
		t.Fatalf("got %d sections", len(got))
	}

	// The tail of one section and the start of the next in a packet, said by the pointer.
	got = nil
	feed(packet(PIDEIT, true, false, -1, append([]byte{0}, long[:PacketSize-5]...)))
	rest := long[PacketSize-5:]
	feed(packet(PIDEIT, true, false, -1, append(append([]byte{byte(len(rest))}, rest...), sec...)))
	if len(got) != 2 || !bytes.Equal(got[0], long) || !bytes.Equal(got[1], sec) {
		t.Fatalf("pointer field: got %d sections", len(got))
	}

	// What's refused: a damaged section, another PID, a scrambled packet, a continuation with
	// no start, a pointer past the payload, a section longer than the standard allows.
	got = nil
	bad := bytes.Clone(sec)
	bad[10] ^= 0xff
	feed(packet(PIDEIT, true, false, -1, append([]byte{0}, bad...)))
	feed(packet(PIDSDT, true, false, -1, append([]byte{0}, sec...)))
	feed(packet(PIDEIT, true, true, -1, append([]byte{0}, sec...)))
	d2 := NewDemux(PIDEIT)
	d2.OnSec = func(int, []byte) { t.Error("a continuation with no start gave a section") }
	h, _ := ParsePacket(packet(PIDEIT, false, false, -1, sec))
	d2.Feed(h)
	feed(packet(PIDEIT, true, false, -1, []byte{250}))
	huge := []byte{0x4e, 0xbf, 0xff} // 4098 bytes said
	feed(packet(PIDEIT, true, false, -1, append([]byte{0}, huge...)))
	if len(got) != 0 {
		t.Fatalf("%d sections from what should be refused", len(got))
	}
	// Following a PID added later.
	follow[PIDSDT] = true
	d.Add(PIDSDT)
	feed(packet(PIDSDT, true, false, -1, append([]byte{0}, sec...)))
	if len(got) != 1 {
		t.Fatalf("an added PID: %d sections", len(got))
	}
}

func TestTimes(t *testing.T) {
	// EN 300 468 annex C's example: 93/10/13 12:45:00 is MJD 0xC079, time 0x124500.
	if got, want := mjdTime([]byte{0xc0, 0x79, 0x12, 0x45, 0x00}), time.Date(1993, 10, 13, 12, 45, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("mjdTime: %v, want %v", got, want)
	}
	if got := mjdTime([]byte{0xff, 0xff, 0, 0, 0}); !got.IsZero() {
		t.Errorf("an undefined start: %v", got)
	}
	if got := bcdDuration([]byte{0x01, 0x45, 0x30}); got != time.Hour+45*time.Minute+30*time.Second {
		t.Errorf("bcdDuration: %v", got)
	}
	if itoa(7) != "7" || itoa(18) != "18" {
		t.Errorf("itoa: %q %q", itoa(7), itoa(18))
	}
}

// eitEvent is one event of a DVB EIT: start, duration and descriptors.
func eitEvent(id int, start []byte, dur []byte, desc ...[]byte) []byte {
	var d []byte
	for _, x := range desc {
		d = append(d, x...)
	}
	e := []byte{byte(id >> 8), byte(id)}
	e = append(e, start...)
	e = append(e, dur...)
	return append(append(e, 0x80|byte(len(d)>>8&0x0f), byte(len(d))), d...)
}

func shortEvent(title, desc string) []byte {
	body := append([]byte("eng"), byte(len(title)))
	body = append(body, title...)
	body = append(body, byte(len(desc)))
	body = append(body, desc...)
	return append([]byte{0x4d, byte(len(body))}, body...)
}

func eit(service int, events ...[]byte) []byte {
	body := []byte{byte(service >> 8), byte(service), 0xc1, 0, 0, 0, 0, 0, 0, 0, 0x4e}
	for _, e := range events {
		body = append(body, e...)
	}
	return section(0x4e, body)
}

func TestParseEIT(t *testing.T) {
	start := []byte{0xc0, 0x79, 0x12, 0x45, 0x00}
	s := eit(0x0203,
		eitEvent(1, start, []byte{0x00, 0x30, 0x00}, shortEvent("News", "The day's news"), []byte{0x55, 4, 'G', 'B', 'R', 0x09}),
		eitEvent(2, []byte{0xff, 0xff, 0, 0, 0}, []byte{0, 0, 0}, shortEvent("No start", "")), // left out: no time
		eitEvent(3, start, []byte{0x01, 0x00, 0x00}),                                          // left out: no title
	)
	got := ParseEIT(s)
	want := []Event{{ServiceID: 0x0203, Start: time.Date(1993, 10, 13, 12, 45, 0, 0, time.UTC), Duration: 30 * time.Minute, Title: "News", Desc: "The day's news", Rating: "12"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseEIT:\n got %+v\nwant %+v", got, want)
	}
	for name, bad := range map[string][]byte{
		"not an EIT":      section(0x42, s[3:len(s)-4]),
		"too short":       s[:17],
		"length past end": s[:len(s)-10],
	} {
		if ev := ParseEIT(bad); len(ev) != 0 {
			t.Errorf("%s: %+v", name, ev)
		}
	}
	// A descriptor loop longer than the section stops the reading, it doesn't run past it.
	cut := eit(1, eitEvent(1, start, []byte{0, 0, 0}, shortEvent("A", "")))
	cut[3+11+10] = 0x8f // descriptors_loop_length high bits: far past the end
	if ev := ParseEIT(cut); len(ev) != 0 {
		t.Errorf("a loop past the end: %+v", ev)
	}
}

func TestDVBText(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []byte
		want string
	}{
		{"empty", nil, ""},
		{"default table", []byte("Film night"), "Film night"},
		{"Latin-1 by default", []byte{'C', 'a', 'f', 0xe9}, "Café"},
		{"UTF-8", append([]byte{0x15}, "Café ✓"...), "Café ✓"},
		{"ISO 8859 by number", append([]byte{0x10, 0x00, 0x05}, "Abc"...), "Abc"},
		{"UCS-2", []byte{0x11, 0x00, 'H', 0x00, 'i', 0x26, 0x3a}, "Hi☺"},
		{"a table byte dropped", append([]byte{0x05}, "Greek"...), "Greek"},
		{"line break and controls", []byte{'a', 0x8a, 'b', 0x86, 'c', 0x01}, "a\nbc"},
		{"trailing spaces and breaks", []byte{'x', ' ', 0x8a, ' '}, "x"},
	} {
		if got := DVBText(tc.in); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// multiple makes an ATSC multiple_string_structure with one string of one segment.
func multiple(comp, mode byte, data []byte) []byte {
	return append([]byte{1, 'e', 'n', 'g', 1, comp, mode, byte(len(data))}, data...)
}

func TestMultipleString(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []byte
		want string
	}{
		{"empty", nil, ""},
		{"no strings", []byte{0}, ""},
		{"cut short", []byte{1, 'e', 'n'}, ""},
		{"Latin", multiple(0, 0, []byte("Evening news")), "Evening news"},
		{"UTF-16", multiple(0, 0x3f, []byte{0x00, 'O', 0x00, 'K'}), "OK"},
		{"another page", multiple(0, 0x01, []byte{0x00}), "Ā"},
		{"compressed, which isn't read", multiple(1, 0, []byte("xx")), ""},
		{"a mode not read", multiple(0, 0x08, []byte("xx")), ""},
		{"segment past the end", append(multiple(0, 0, []byte("abc"))[:9], 'a'), ""},
	} {
		if got := MultipleString(tc.in); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestATSCTables(t *testing.T) {
	// MGT: two tables, an EIT-0 at 0x1d00 and a channel table at 0x1ffb (not an EIT).
	mgtBody := []byte{0, 0, 0xc1, 0, 0, 0, 0, 2}
	mgtBody = append(mgtBody, 0x01, 0x00, 0xe0|0x1d, 0x00, 0xe0, 0, 0, 0, 0, 0xf0, 0x00)
	mgtBody = append(mgtBody, 0x00, 0x02, 0xff, 0xfb, 0xe0, 0, 0, 0, 0, 0xf0, 0x00)
	mgtBody = append(mgtBody, 0xf0, 0x00)
	if got := ParseMGT(section(0xc7, mgtBody)); !reflect.DeepEqual(got, []int{0x1d00}) {
		t.Errorf("ParseMGT: %v", got)
	}
	if got := ParseMGT(section(0xc8, mgtBody)); got != nil {
		t.Errorf("not an MGT: %v", got)
	}

	// EIT: one event with a title, one without.
	title := multiple(0, 0, []byte("Weather"))
	ev := func(id int, start uint32, secs int, title []byte) []byte {
		e := []byte{0xc0 | byte(id>>8), byte(id)}
		e = binary.BigEndian.AppendUint32(e, start)
		e = append(e, 0xc0|byte(secs>>16&0x0f), byte(secs>>8), byte(secs), byte(len(title)))
		e = append(e, title...)
		return append(e, 0xf0, 0x00)
	}
	body := []byte{0x00, 0x05, 0xc1, 0, 0, 0, 2}
	body = append(body, ev(1, 1_000_000_000, 1800, title)...)
	body = append(body, ev(2, 1_000_001_800, 600, nil)...)
	got := ParseATSCEIT(section(0xcb, body))
	want := []Event{{ServiceID: 5, ATSC: true, Start: time.Unix(1_000_000_000+gpsEpoch, 0).UTC(), Duration: 30 * time.Minute, Title: "Weather"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseATSCEIT:\n got %+v\nwant %+v", got, want)
	}
	if got := ParseATSCEIT(section(0xcb, body)[:20]); len(got) != 0 {
		t.Errorf("cut short: %+v", got)
	}
}

// stream is a reader that hands out data in pieces of the given sizes, then the rest.
type stream struct {
	data   []byte
	pieces []int
	closed bool
}

func (s *stream) Read(p []byte) (int, error) {
	if len(s.data) == 0 {
		return 0, io.EOF
	}
	n := len(s.data)
	if len(s.pieces) > 0 {
		n, s.pieces = min(s.pieces[0], n), s.pieces[1:]
	}
	n = copy(p[:min(n, len(p))], s.data)
	s.data = s.data[n:]
	return n, nil
}

func (s *stream) Close() error { s.closed = true; return nil }

func TestGuardPassesAClearStreamAndReadsItsGuide(t *testing.T) {
	var ts []byte
	// Some noise before the first packet: the guard finds the sync byte again. Noise that comes
	// in a piece shorter than a packet costs the packet after it, which a broadcast's guide,
	// sent again and again, can spare: the second copy is read.
	ts = append(ts, 0x00, 0x13, 0x99)
	sec := eit(7, eitEvent(1, []byte{0xc0, 0x79, 0x12, 0x45, 0x00}, []byte{0, 0x30, 0}, shortEvent("Quiz", "")))
	guide := packet(PIDEIT, true, false, -1, append([]byte{0}, sec...))
	ts = append(ts, guide...)
	for i := 0; i < scrambleWindow; i++ {
		ts = append(ts, packet(0x100, i == 0, false, -1, []byte{byte(i)})...)
	}
	ts = append(ts, guide...)
	// A few scrambled packets don't stop a clear channel.
	for i := 0; i < 10; i++ {
		ts = append(ts, packet(0x100, false, true, -1, nil)...)
	}
	src := &stream{data: bytes.Clone(ts), pieces: []int{1, 100, 187, 188, 189, 1000, 7}}
	g := NewGuard(src)
	var events []Event
	g.OnEvent = func(e Event) { events = append(events, e) }
	var out bytes.Buffer
	buf := make([]byte, 333)
	for {
		n, err := g.Read(buf)
		out.Write(buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(out.Bytes(), ts) {
		t.Error("the guard changed the stream")
	}
	if g.Fatal() != nil || !g.Verified() {
		t.Errorf("a clear stream: fatal %v, verified %v", g.Fatal(), g.Verified())
	}
	if len(events) == 0 || len(events) > 2 || events[0].Title != "Quiz" || events[0].ServiceID != 7 {
		t.Errorf("events: %+v", events)
	}
	if g.Underneath() != src {
		t.Error("Underneath isn't the stream")
	}
	if err := g.Close(); err != nil || !src.closed {
		t.Errorf("Close: %v, closed %v", err, src.closed)
	}
}

func TestGuardStopsAScrambledStream(t *testing.T) {
	var ts []byte
	for i := 0; i < scrambleLimit+10; i++ {
		ts = append(ts, packet(0x100, false, true, -1, nil)...)
	}
	g := NewGuard(&stream{data: ts})
	_, err := io.Copy(io.Discard, g)
	if !errors.Is(err, ErrScrambled) || !errors.Is(g.Fatal(), ErrScrambled) || g.Verified() {
		t.Errorf("copy: %v, fatal %v, verified %v", err, g.Fatal(), g.Verified())
	}
	if n, err := g.Read(make([]byte, 10)); n != 0 || !errors.Is(err, ErrScrambled) {
		t.Errorf("reading on: %d %v", n, err)
	}
}

// Null packets (PID 0x1fff) are padding: they count for nothing either way.
func TestGuardIgnoresNullPackets(t *testing.T) {
	var ts []byte
	for i := 0; i < scrambleWindow; i++ {
		ts = append(ts, packet(0x1fff, false, true, -1, nil)...)
	}
	g := NewGuard(&stream{data: ts})
	if _, err := io.Copy(io.Discard, g); err != nil {
		t.Fatal(err)
	}
	if g.Fatal() != nil || g.Verified() {
		t.Errorf("null packets: fatal %v, verified %v", g.Fatal(), g.Verified())
	}
}

// Whatever a tuner sends, the table readers don't panic, and what they give back is sane.
func FuzzTables(f *testing.F) {
	start := []byte{0xc0, 0x79, 0x12, 0x45, 0x00}
	f.Add(eit(1, eitEvent(1, start, []byte{0, 0x30, 0}, shortEvent("News", "Today"), []byte{0x55, 4, 'G', 'B', 'R', 0x09})))
	f.Add(section(0xc7, []byte{0, 0, 0xc1, 0, 0, 0, 0, 1, 0x01, 0x00, 0xfd, 0x00, 0xe0, 0, 0, 0, 0, 0xf0, 0x00, 0xf0, 0x00}))
	f.Add(section(0xcb, append([]byte{0, 5, 0xc1, 0, 0, 0, 1, 0xc0, 1, 0, 0, 0, 1, 0xc0, 0, 60, 8}, append(multiple(0, 0, []byte("abc")), 0xf0, 0)...)))
	f.Add([]byte{0x11, 0x00, 'H'})
	f.Add(multiple(0, 0x3f, []byte{0xd8, 0x00}))
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, events := range [][]Event{ParseEIT(b), ParseATSCEIT(b)} {
			for _, e := range events {
				if e.Title == "" || !utf8.ValidString(e.Title) || !utf8.ValidString(e.Desc) {
					t.Fatalf("an event with a bad title: %+v", e)
				}
			}
		}
		for _, pid := range ParseMGT(b) {
			if pid < 0 || pid > 0x1fff {
				t.Fatalf("PID %d", pid)
			}
		}
		_ = DVBText(b)
		_ = MultipleString(b)
		if len(b) >= 3 {
			_ = validSection(b)
		}
	})
}

// Whatever comes down the wire, in whatever pieces, the guard passes it on unchanged until it
// stops, and doesn't panic.
func FuzzGuard(f *testing.F) {
	f.Add(packet(PIDEIT, true, false, -1, append([]byte{0}, eit(1, eitEvent(1, []byte{0xc0, 0x79, 0, 0, 0}, []byte{0, 1, 0}, shortEvent("A", "")))...)), uint8(7))
	f.Add(append(packet(0x1ffb, true, false, 3, []byte{0, 0xc7, 0xb0, 0x20}), packet(0x100, false, true, -1, nil)...), uint8(188))
	f.Add([]byte{0x47, 0x47, 0x47}, uint8(1))
	f.Fuzz(func(t *testing.T, data []byte, piece uint8) {
		size := int(piece) + 1
		var pieces []int
		for i := 0; i < len(data); i += size {
			pieces = append(pieces, size)
		}
		g := NewGuard(&stream{data: bytes.Clone(data), pieces: pieces})
		g.OnEvent = func(e Event) {
			if e.Title == "" {
				t.Fatalf("an event with no title: %+v", e)
			}
		}
		out, err := io.ReadAll(g)
		if err != nil && !errors.Is(err, ErrScrambled) {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(data, out) || (err == nil && !bytes.Equal(out, data)) {
			t.Fatalf("passed %d of %d bytes, changed or short (%v)", len(out), len(data), err)
		}
	})
}
