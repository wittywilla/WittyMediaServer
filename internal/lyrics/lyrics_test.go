// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package lyrics

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseTimed(t *testing.T) {
	d := Parse("[ar:Someone]\n[ti:A Made Up Song]\n[00:01.50]First line\n[00:03.25][00:09.00]Sung twice\n\n[00:05]Third\n")
	if !d.Synced() || len(d.Timed) != 4 {
		t.Fatalf("got %+v", d)
	}
	want := []struct {
		t    int64
		text string
	}{{1500, "First line"}, {3250, "Sung twice"}, {5000, "Third"}, {9000, "Sung twice"}}
	for i, w := range want {
		if d.Timed[i].T != w.t || d.Timed[i].Text != w.text {
			t.Errorf("line %d = %+v, want %+v", i, d.Timed[i], w)
		}
	}
	if d.Timed[0].End != 3250 || d.Timed[3].End != 0 {
		t.Errorf("ends: %+v", d.Timed)
	}
	if strings.Contains(d.Plain, "[") || !strings.Contains(d.Plain, "Third") {
		t.Errorf("plain: %q", d.Plain)
	}
}

func TestParseFrom(t *testing.T) {
	for name, c := range map[string]struct {
		in     string
		synced bool
	}{
		"plain":         {"Just words\nand more words", false},
		"crlf":          {"[00:01.00]One\r\n[00:02.00]Two\r\n", true},
		"three digits":  {"[00:01.123]One", true},
		"colon frac":    {"[00:01:12]One", true},
		"meta only":     {"[ar:Nobody]\n[length:03:00]", false},
		"not a stamp":   {"[Chorus]\nWords", false},
		"bad minutes":   {"[aa:bb]Words", false},
		"long stamp":    {"[1234:00.00]Words", false},
		"empty":         {"", false},
		"only newlines": {"\n\n\n", false},
	} {
		if got := Parse(c.in); got.Synced() != c.synced {
			t.Errorf("%s: synced = %v, doc %+v", name, got.Synced(), got)
		}
	}
}

func TestOffsetMovesLinesEarlier(t *testing.T) {
	d := Parse("[offset:+500]\n[00:10.00]Line\n[00:00.20]Early")
	if d.Timed[1].T != 9500 || d.Timed[0].T != 0 {
		t.Fatalf("got %+v", d.Timed)
	}
}

func TestWordTiming(t *testing.T) {
	d := Parse("[00:01.00] v1: <00:01.00> Hello <00:01.60> there <00:02.20> friend")
	if len(d.Timed) != 1 {
		t.Fatalf("got %+v", d)
	}
	l := d.Timed[0]
	if l.Text != "Hello there friend" {
		t.Errorf("text %q", l.Text)
	}
	if len(l.Words) != 3 || l.Words[1].T != 1600 || strings.TrimSpace(l.Words[2].Text) != "friend" {
		t.Errorf("words %+v", l.Words)
	}
}

func TestLimits(t *testing.T) {
	var b strings.Builder
	for b.Len() < MaxBytes*2 {
		b.WriteString("[00:01.00]x\n")
	}
	if d := Parse(b.String()); len(d.Timed) > MaxLines {
		t.Fatalf("%d lines", len(d.Timed))
	}
}

func TestFormatRoundTrip(t *testing.T) {
	in := []Line{{T: 1000, Text: "One"}, {T: 62340, Text: "Two\nwith a break"}, {T: 3600000, Text: "Three"}}
	d := Parse(Format(in))
	if len(d.Timed) != 3 || d.Timed[1].T != 62340 || d.Timed[1].Text != "Two with a break" || d.Timed[2].T != 3600000 {
		t.Fatalf("got %+v", d.Timed)
	}
}

func TestDecode(t *testing.T) {
	if Decode([]byte{0xEF, 0xBB, 0xBF, 'h', 'i'}) != "hi" {
		t.Error("utf-8 with a mark")
	}
	if Decode([]byte{0xFF, 0xFE, 'h', 0, 'i', 0}) != "hi" {
		t.Error("utf-16 little endian")
	}
	if Decode([]byte{0xFE, 0xFF, 0, 'h', 0, 'i'}) != "hi" {
		t.Error("utf-16 big endian")
	}
	if Decode([]byte{'c', 'a', 'f', 0xE9}) != "café" {
		t.Error("latin-1")
	}
}

// id3Tag builds a tag with these frames, v2.3 sizes.
func id3Tag(frames ...[]byte) []byte {
	body := bytes.Join(frames, nil)
	n := len(body)
	head := []byte{'I', 'D', '3', 3, 0, 0, byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}
	return append(head, body...)
}

func id3Frame(id string, body []byte) []byte {
	h := make([]byte, 10)
	copy(h, id)
	binary.BigEndian.PutUint32(h[4:], uint32(len(body)))
	return append(h, body...)
}

func TestID3Plain(t *testing.T) {
	uslt := append([]byte{3, 'e', 'n', 'g', 0}, []byte("Words of a song\nAnother line")...)
	d := ReadID3(bytes.NewReader(id3Tag(id3Frame("APIC", make([]byte, 5000)), id3Frame("USLT", uslt))))
	if d.Synced() || !strings.Contains(d.Plain, "Another line") {
		t.Fatalf("got %+v", d)
	}
}

func TestID3Timed(t *testing.T) {
	ms := func(n uint32) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, n); return b }
	sylt := []byte{3, 'e', 'n', 'g', 2, 1, 0}
	sylt = append(append(append(sylt, "One\x00"...), ms(1500)...), append([]byte("Two\x00"), ms(4000)...)...)
	d := ReadID3(bytes.NewReader(id3Tag(id3Frame("SYLT", sylt))))
	if !d.Synced() || len(d.Timed) != 2 || d.Timed[1].T != 4000 || d.Timed[0].End != 4000 {
		t.Fatalf("got %+v", d)
	}
}

func TestID3Hostile(t *testing.T) {
	for name, b := range map[string][]byte{
		"nothing":      {},
		"short":        []byte("ID3"),
		"not a tag":    []byte("OggS and so on and so on"),
		"frame beyond": id3Tag([]byte{'U', 'S', 'L', 'T', 0xff, 0xff, 0xff, 0xff, 0, 0, 1, 2, 3}),
		"empty uslt":   id3Tag(id3Frame("USLT", nil)),
		"short sylt":   id3Tag(id3Frame("SYLT", []byte{1, 2})),
		"sylt frames":  id3Tag(id3Frame("SYLT", []byte{3, 'e', 'n', 'g', 1, 1, 0, 'a', 0, 0, 0, 0, 1})),
		"odd utf16":    id3Tag(id3Frame("USLT", []byte{1, 'e', 'n', 'g', 0xff, 0xfe, 0, 0, 'a'})),
	} {
		if d := ReadID3(bytes.NewReader(b)); d.Synced() {
			t.Errorf("%s: read lines from it: %+v", name, d)
		}
	}
}

func lrclibServer(t *testing.T, handle func(w http.ResponseWriter, r *http.Request)) *LRCLIB {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(handle))
	t.Cleanup(ts.Close)
	return &LRCLIB{Base: ts.URL, Client: ts.Client(), Label: "test"}
}

func TestLRCLIBSendsOnlyTheFourFields(t *testing.T) {
	l := lrclibServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/get" {
			t.Errorf("path %s", r.URL.Path)
		}
		q := r.URL.Query()
		for k := range q {
			if k != "artist_name" && k != "track_name" && k != "album_name" && k != "duration" {
				t.Errorf("sent %s as well", k)
			}
		}
		if q.Get("duration") != "228" || q.Get("artist_name") != "An Artist" {
			t.Errorf("query %v", q)
		}
		if !strings.HasPrefix(r.UserAgent(), "WittyMediaServer") {
			t.Errorf("agent %q", r.UserAgent())
		}
		_ = json.NewEncoder(w).Encode(lrclibRecord{ID: 9, TrackName: "T", ArtistName: "An Artist", SyncedLyrics: "[00:01.00]Hi", PlainLyrics: "Hi"})
	})
	f, err := l.Get(context.Background(), Query{Artist: "An Artist", Title: "T", Album: "A", DurationSec: 227.6})
	if err != nil || !f.HasSynced || f.ID != "9" {
		t.Fatalf("%+v %v", f, err)
	}
}

func TestLRCLIBFallsBackOnSearchByLength(t *testing.T) {
	l := lrclibServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/get" {
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode([]lrclibRecord{
			{ID: 1, Duration: 300, SyncedLyrics: "[00:01.00]A long live version"},
			{ID: 2, Duration: 201, PlainLyrics: "Right length, no times"},
			{ID: 3, Duration: 202, SyncedLyrics: "[00:01.00]Right length, timed"},
		})
	})
	f, err := l.Get(context.Background(), Query{Title: "T", DurationSec: 200})
	if err != nil || f.ID != "3" {
		t.Fatalf("%+v %v", f, err)
	}
}

func TestLRCLIBNothingFits(t *testing.T) {
	l := lrclibServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/get" {
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode([]lrclibRecord{{ID: 1, Duration: 300, SyncedLyrics: "[00:01.00]No"}})
	})
	if _, err := l.Get(context.Background(), Query{Title: "T", DurationSec: 200}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestMusixmatch(t *testing.T) {
	refused := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := 200
		body := `{}`
		switch {
		case refused:
			code = 401
		case strings.HasSuffix(r.URL.Path, "matcher.subtitle.get"):
			body = `{"subtitle":{"subtitle_body":"[00:01.00]Timed"}}`
		}
		_, _ = w.Write([]byte(`{"message":{"header":{"status_code":` + map[int]string{200: "200", 401: "401"}[code] + `},"body":` + body + `}}`))
	}))
	defer ts.Close()
	m := &Musixmatch{Key: "k", Base: ts.URL, Client: ts.Client()}
	f, err := m.Get(context.Background(), Query{Title: "T", Artist: "A"})
	if err != nil || !f.HasSynced {
		t.Fatalf("%+v %v", f, err)
	}
	// A result under Other versions can be chosen again by its id.
	found, err := m.Search(context.Background(), Query{Title: "T & co", Artist: "A", DurationSec: 201.4}, "")
	if err != nil || len(found) != 1 || found[0].ID == "" {
		t.Fatalf("%+v %v", found, err)
	}
	again, err := m.ByID(context.Background(), found[0].ID)
	if err != nil || !again.HasSynced || again.Provider != m.Provider() || again.Title != "T & co" || again.ID != found[0].ID || again.DurationSec != 201 {
		t.Fatalf("%+v %v", again, err)
	}
	if _, err := m.ByID(context.Background(), "made-up"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an id that was never given out: %v", err)
	}
	refused = true
	if _, err := m.Get(context.Background(), Query{Title: "T"}); !errors.Is(err, ErrNeedsPlan) {
		t.Fatalf("got %v", err)
	}
}

func FuzzParse(f *testing.F) {
	f.Add("[00:01.00]x\n[offset:9]\n[00:01.00]<00:01.00>y")
	f.Fuzz(func(t *testing.T, s string) { _ = Parse(s) })
}

func FuzzID3(f *testing.F) {
	f.Add(id3Tag(id3Frame("USLT", []byte{1, 'e', 'n', 'g', 0xff, 0xfe, 0, 0, 'a'})))
	f.Fuzz(func(t *testing.T, b []byte) { _ = ReadID3(bytes.NewReader(b)) })
}
