package wittybluray

import (
	"errors"
	"fmt"
)

// MovieObject.bdmv: the disc's HDMV programs. A movie object is a list of
// navigation commands the player runs when the index or a JUMP/CALL names
// it; its commands start playlists, branch, compare and set registers. The
// commands themselves are decoded and run in vm.go.

// Command is one 12-byte HDMV navigation command, as stored.
type Command struct {
	Op       uint32
	Dst, Src uint32
}

// MovieObject is one program.
type MovieObject struct {
	ResumeIntention bool // playback resumes in this object after a menu call
	MenuCallMask    bool // the user's menu key is ignored while it runs
	TitleSearchMask bool // the user's title search is ignored while it runs
	Commands        []Command
}

// ParseMovieObjects parses MovieObject.bdmv.
func ParseMovieObjects(b []byte) ([]MovieObject, error) {
	r := &rd{b: b}
	if r.str(0, 4) != "MOBJ" {
		return nil, errors.New("MovieObject.bdmv: not a movie object file")
	}
	o := 40
	n := int(r.u16(o + 8))
	o += 10
	if n > 10000 {
		return nil, fmt.Errorf("MovieObject.bdmv: %d objects is more than a disc can have", n)
	}
	out := make([]MovieObject, 0, n)
	for i := 0; i < n && r.err == nil; i++ {
		flags := r.u16(o)
		nc := int(r.u16(o + 2))
		o += 4
		if nc > 100000 {
			return nil, fmt.Errorf("MovieObject.bdmv: object %d has %d commands", i, nc)
		}
		m := MovieObject{ResumeIntention: flags&0x8000 != 0, MenuCallMask: flags&0x4000 != 0, TitleSearchMask: flags&0x2000 != 0, Commands: make([]Command, 0, nc)}
		for c := 0; c < nc && r.err == nil; c++ {
			m.Commands = append(m.Commands, Command{Op: r.u32(o), Dst: r.u32(o + 4), Src: r.u32(o + 8)})
			o += 12
		}
		out = append(out, m)
	}
	if r.err != nil {
		return nil, fmt.Errorf("MovieObject.bdmv: %w", r.err)
	}
	return out, nil
}
