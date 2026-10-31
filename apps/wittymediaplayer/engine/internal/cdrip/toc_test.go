package cdrip

import (
	"errors"
	"testing"
)

func tocEntry(number int, control byte, lba int) []byte {
	lba += LeadIn
	return []byte{0, 0x10 | control, byte(number), 0, 0, byte(lba / 75 / 60), byte(lba / 75 % 60), byte(lba % 75)}
}

func TestDriveTOC(t *testing.T) {
	b := []byte{0, 0, 1, 3}
	b = append(b, tocEntry(1, 0, 0)...)
	b = append(b, tocEntry(2, 1, 15000)...)
	b = append(b, tocEntry(3, 0, 30000)...)
	b = append(b, tocEntry(0xAA, 0, 45000)...)
	info, err := parseTOC(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Tracks) != 3 || info.Tracks[1].Start != 15000 || info.Tracks[1].End != 30000 || !info.Tracks[1].PreEmphasis || !info.Tracks[0].Audio || info.Tracks[0].PreEmphasis {
		t.Errorf("%+v", info.Tracks)
	}
	if info.DiscID == "" || info.TOC == "" {
		t.Errorf("%q %q", info.DiscID, info.TOC)
	}
	// Music, then a data session: the gap between them isn't music, nor readable.
	b = []byte{0, 0, 1, 2}
	b = append(b, tocEntry(1, 0, 0)...)
	b = append(b, tocEntry(2, 4, 60000)...)
	b = append(b, tocEntry(0xAA, 4, 90000)...)
	info, err = parseTOC(b)
	if err != nil {
		t.Fatal(err)
	}
	if a := info.AudioTracks(); len(a) != 1 || a[0].End != 60000-sessionGap {
		t.Errorf("%+v", info.Tracks)
	}
	if info.Tracks[1].Audio {
		t.Error("track 2 is data")
	}
	// Nothing but data, and nonsense.
	b = []byte{0, 0, 1, 1}
	b = append(b, tocEntry(1, 4, 0)...)
	b = append(b, tocEntry(0xAA, 4, 9000)...)
	if _, err := parseTOC(b); err == nil {
		t.Error("a data disc isn't an audio CD")
	}
	if _, err := parseTOC([]byte{1, 2}); !errors.Is(err, ErrNoRead) {
		t.Error(err)
	}
}

// An image keeps only the audio of a disc with a data track, counted from its own start.
func TestImageOfMixedDisc(t *testing.T) {
	info := &Info{Tracks: []Track{{Number: 1, Pregap: 0, Start: 0, End: 100}, {Number: 2, Audio: true, Pregap: 100, Start: 150, End: 300}, {Number: 3, Audio: true, Pregap: 300, Start: 300, End: 500}}}
	got := audioOnly(info, 100)
	if len(got.Tracks) != 2 || got.Tracks[0].Pregap != 0 || got.Tracks[0].Start != 50 || got.Tracks[1].End != 400 || len(info.Tracks) != 3 {
		t.Errorf("%+v", got.Tracks)
	}
}
