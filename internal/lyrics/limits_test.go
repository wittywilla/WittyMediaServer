// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package lyrics

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// What comes out of Parse is sent as one answer, built in memory. A small file saying "this line,
// with these 18000 timed words, is sung at each of these 18000 moments" came out as 18000 x 18000
// words: 256 KB in, gigabytes out, from any account opening the lyrics.
func TestParseCantMultiplyWhatItRead(t *testing.T) {
	stamps := strings.Repeat("[00:01]", 18000)
	words := strings.Repeat("<00:01>a", 18000)
	for name, text := range map[string]string{
		"stamps times words":  stamps + words,
		"with an offset too":  "[offset:5]\n" + stamps + words,
		"stamps times a line": strings.Repeat("[00:01]", 1000) + strings.Repeat("x", MaxBytes-7000),
		"many lines each":     strings.Repeat("[00:01][00:02][00:03][00:04] <00:01>a<00:02>b\n", 6000),
	} {
		d := Parse(text)
		start := time.Now()
		b, err := json.Marshal(d.Timed)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) > 16<<20 {
			t.Errorf("%s: %d bytes of input became %d of answer", name, len(text), len(b))
		}
		if len(d.Timed) > MaxLines {
			t.Errorf("%s: %d lines", name, len(d.Timed))
		}
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("%s: took %v", name, d)
		}
	}
	// A real song still has all of its lines and words.
	d := Parse("[00:12.00][00:45.50]<00:12.00>one <00:12.50>two\n[00:20.00]three")
	if len(d.Timed) != 3 || len(d.Timed[0].Words) != 2 || d.Timed[1].Text != "three" || len(d.Timed[2].Words) != 2 {
		t.Errorf("%+v", d.Timed)
	}
}

func TestOffsetCantWrapAround(t *testing.T) {
	d := Parse("[offset:-9223372036854775808]\n[00:10.00]a\n[00:20.00]b")
	if len(d.Timed) != 2 || d.Timed[0].T < 0 || d.Timed[1].T < d.Timed[0].T {
		t.Errorf("%+v", d.Timed)
	}
}

func limitTag(frames ...[]byte) []byte {
	body := bytes.Join(frames, nil)
	n := len(body)
	return append([]byte{'I', 'D', '3', 3, 0, 0, byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}, body...)
}

func limitFrame(id string, body []byte) []byte {
	var h [10]byte
	copy(h[:4], id)
	binary.BigEndian.PutUint32(h[4:8], uint32(len(body)))
	return append(h[:], body...)
}

// Lying sizes and a tag with nothing but frames must end quickly with nothing to show.
func TestID3HoldsTogether(t *testing.T) {
	big := make([]byte, 10)
	copy(big, "SYLT")
	binary.BigEndian.PutUint32(big[4:8], 0xFFFFFFFF)
	sylt := append([]byte{0, 'e', 'n', 'g', 2, 1, 0}, append([]byte("line"), 0, 0, 0, 0, 1)...)
	for name, tag := range map[string][]byte{
		"a frame larger than the tag": limitTag(big),
		"an empty USLT":               limitTag(limitFrame("USLT", nil)),
		"a short SYLT":                limitTag(limitFrame("SYLT", []byte{0, 'e', 'n', 'g', 2, 1})),
		"truncated":                   limitTag(limitFrame("USLT", []byte{1, 'e', 'n', 'g', 0xFF}))[:18],
		"many frames":                 limitTag(bytes.Repeat(limitFrame("XXXX", nil), 100000)),
	} {
		start := time.Now()
		ReadID3(bytes.NewReader(tag))
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("%s took %v", name, d)
		}
	}
	if d := ReadID3(bytes.NewReader(limitTag(limitFrame("SYLT", sylt)))); !d.Synced() || d.Timed[0].Text != "line" {
		t.Errorf("a good tag: %+v", d)
	}
}

func FuzzLyrics(f *testing.F) {
	for _, s := range []string{
		"[ar:x]\n[offset:500]\n[00:10.00]a\n", "[00:01][00:02]<00:01>a <00:02>b", "plain words\nand more", "[offset:9223372036854775807]\n[00:01]a",
		"v1: <00:01>a", "[999:99.999]", "\xff\xfe[\x000\x000\x00", "\xef\xbb\xbf[00:01]a",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		start := time.Now()
		d := Parse(Decode([]byte(s)))
		b, err := json.Marshal(d.Timed)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Timed) > MaxLines || len(b) > 16<<20 {
			t.Fatalf("%d bytes became %d lines and %d bytes", len(s), len(d.Timed), len(b))
		}
		if el := time.Since(start); el > 5*time.Second {
			t.Fatalf("took %v", el)
		}
		_ = Format(d.Timed)
	})
}

func FuzzReadID3(f *testing.F) {
	sylt := append([]byte{0, 'e', 'n', 'g', 2, 1, 0}, append([]byte("line"), 0, 0, 0, 0, 1)...)
	f.Add(limitTag(limitFrame("SYLT", sylt)))
	f.Add(limitTag(limitFrame("USLT", append([]byte{0, 'e', 'n', 'g', 0}, []byte("words")...))))
	f.Add(limitTag(limitFrame("USLT", append([]byte{1, 'e', 'n', 'g', 0xFF, 0xFE, 0, 0}, []byte{'a', 0}...))))
	f.Add([]byte("ID3\x04\x40\x00\x00\x00\x00\x7f\x00\x00\x00\x06"))
	f.Fuzz(func(t *testing.T, b []byte) {
		start := time.Now()
		d := ReadID3(bytes.NewReader(b))
		if len(d.Timed) > MaxLines {
			t.Fatalf("%d lines", len(d.Timed))
		}
		if el := time.Since(start); el > 5*time.Second {
			t.Fatalf("took %v", el)
		}
	})
}
