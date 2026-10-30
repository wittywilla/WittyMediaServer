package wittybluray

import (
	"encoding/binary"
	"testing"
	"time"
)

// The parsers read whatever is on a disc image, so no input may crash them.
// `go test -fuzz=FuzzParsers` explores further; the seeds run with every test.

func FuzzParsers(f *testing.F) {
	f.Add([]byte("INDX0200"), uint8(0))
	f.Add([]byte("MOBJ0200\x00\x00\x00\x00"), uint8(1))
	f.Add([]byte("MPLS0200\x00\x00\x00\x3a\x00\x00\x00\x00"), uint8(2))
	f.Add([]byte("HDMV0200\x00\x00\x00\x00\x00\x00\x00\x00"), uint8(3))
	f.Add([]byte{0x16, 0x00, 0x10}, uint8(4))
	f.Add([]byte{0x15, 0x00, 0x05, 0x00, 0x01}, uint8(5))
	f.Fuzz(func(t *testing.T, b []byte, which uint8) {
		switch which % 7 {
		case 0:
			_, _ = ParseIndex(b)
		case 1:
			_, _ = ParseMovieObjects(b)
		case 2:
			_, _ = ParsePlaylist(1, b)
		case 3:
			_, _ = ParseClip("00001", b)
		case 4:
			m := &Menu{}
			_ = m.parseIC(b)
		case 5:
			_, _ = parsePDS(b, true)
			_, _ = parseCompObject(b)
			_ = parseWindows(b)
		case 6:
			_, _ = decodeRLE(b, 64, 64)
		}
	})
}

func navDisc(objs []MovieObject) *Disc {
	first := IndexObject{Kind: ObjectHDMV, MovieObject: 0}
	return &Disc{
		Index:        &Index{FirstPlay: first, TopMenu: first, Titles: []Title{{Object: first}}},
		MovieObjects: objs,
		Playlists:    map[int]*Playlist{1: {Number: 1, Items: []PlayItem{{Clip: "00001"}}}},
		Clips:        map[string]*Clip{},
	}
}

// A disc's program jumping to its own first-play title recursed until the Go
// stack overflowed, which ends the whole server: unlike a panic it can't be
// recovered, so a single crafted disc image took everything down.
func TestNavProgramsCantRecurse(t *testing.T) {
	for name, cmd := range map[string]Command{
		"jump to first play": {0x21810000, 0xffff, 0},
		"jump to top menu":   {0x21810000, 0, 0},
		"jump to title 1":    {0x21810000, 1, 0},
		"call title 1":       {0x21830000, 1, 0},
		"call first play":    {0x21830000, 0xffff, 0},
	} {
		n := NewNav(navDisc([]MovieObject{{Commands: []Command{cmd}}}), NavOptions{})
		if st := n.Start(); st == nil || st.Kind != StepStop {
			t.Errorf("%s: %+v, want a stop", name, st)
		}
	}
}

// Running a disc's programs must always come back, however they are written.
func FuzzNav(f *testing.F) {
	f.Add([]byte{0x21, 0x81, 0, 0, 0, 0, 0xff, 0xff, 0, 0, 0, 0, 0x20, 0x81, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, uint8(0))
	f.Add([]byte{0x22, 0x80, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0x21, 0x84, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, uint8(3))
	f.Fuzz(func(t *testing.T, b []byte, script uint8) {
		var cmds []Command
		for i := 0; i+12 <= len(b) && len(cmds) < 64; i += 12 {
			cmds = append(cmds, Command{binary.BigEndian.Uint32(b[i:]), binary.BigEndian.Uint32(b[i+4:]), binary.BigEndian.Uint32(b[i+8:])})
		}
		n := NewNav(navDisc([]MovieObject{{Commands: cmds}, {Commands: cmds}}), NavOptions{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			n.Start()
			for i := uint8(0); i < script%8; i++ {
				n.PressButton(cmds)
				n.PlaylistEnded()
				n.MenuCall()
				n.Resume()
				n.NavTimerFired()
				n.PlayTitle(int(script))
			}
		}()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Fatal("a disc's program never came back")
		}
	})
}
