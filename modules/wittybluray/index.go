package wittybluray

import (
	"errors"
	"fmt"
)

// index.bdmv: what the disc does when inserted (first play), what its top
// menu is, and the list of titles. Each points at an HDMV movie object (a
// small program of navigation commands) or a BD-J object (a Java program,
// which this engine cannot run).

// ObjectKind says what kind of program an index entry runs.
type ObjectKind int

const (
	ObjectNone ObjectKind = iota
	ObjectHDMV            // a movie object in MovieObject.bdmv
	ObjectBDJ             // a BD-J (Java) object: not supported
)

func (k ObjectKind) String() string {
	switch k {
	case ObjectHDMV:
		return "HDMV"
	case ObjectBDJ:
		return "BD-J"
	}
	return "none"
}

// IndexObject is one entry of the index: first play, top menu or a title.
type IndexObject struct {
	Kind        ObjectKind
	Interactive bool   // the object has a menu (an IG stream or BD-J UI) rather than just playing
	MovieObject int    // HDMV: the movie object to run
	BDJName     string // BD-J: the object's name (BDMV/BDJO/<name>.bdjo)
}

// Title is a disc title as the player's title list shows it.
type Title struct {
	Number int // 1-based, as JUMP_TITLE names them
	// Access says what the user may do while this title plays: 0 = anything,
	// 1 = no title search, 2 = no title search and no menu call.
	Access int
	Object IndexObject
}

// Index is BDMV/index.bdmv.
type Index struct {
	Version    string
	Stereo3D   bool // the disc prefers 3D output when the player can
	HasContent bool
	// VideoFormat and FrameRate are the disc's main video, as its menus
	// expect: 1 = 480i, 2 = 576i, 3 = 480p, 4 = 1080i, 5 = 720p, 6 = 1080p,
	// 7 = 576p; frame rates 1 = 23.976, 2 = 24, 3 = 25, 4 = 29.97, 6 = 50, 7 = 59.94.
	VideoFormat, FrameRate int
	FirstPlay, TopMenu     IndexObject
	Titles                 []Title
}

// ParseIndex parses index.bdmv.
func ParseIndex(b []byte) (*Index, error) {
	r := &rd{b: b}
	if r.str(0, 4) != "INDX" {
		return nil, errors.New("index.bdmv: not an index file")
	}
	ix := &Index{Version: r.str(4, 4)}
	start := int(r.u32(8))
	// AppInfoBDMV
	flags := r.u8(44)
	ix.Stereo3D = flags&0x40 != 0
	ix.HasContent = flags&0x20 != 0
	ix.VideoFormat = int(r.u8(45) >> 4)
	ix.FrameRate = int(r.u8(45) & 15)
	// Indexes
	o := start + 4
	var obj func(title bool) IndexObject
	obj = func(title bool) IndexObject {
		kind := ObjectKind(r.u8(o) >> 6)
		var out IndexObject
		switch kind {
		case ObjectHDMV:
			out = IndexObject{Kind: kind, Interactive: r.u8(o+4)>>6 == 1, MovieObject: int(r.u16(o + 6))}
			o += 12
		case ObjectBDJ:
			out = IndexObject{Kind: kind, Interactive: r.u8(o+4)>>6 == 3, BDJName: r.str(o+6, 5)}
			o += 16
		default:
			out = IndexObject{Kind: ObjectNone}
			o += 12
		}
		return out
	}
	ix.FirstPlay = obj(false)
	ix.TopMenu = obj(false)
	n := int(r.u16(o))
	o += 2
	if n > 1000 {
		return nil, fmt.Errorf("index.bdmv: %d titles is more than a disc can have", n)
	}
	for i := 0; i < n && r.err == nil; i++ {
		access := int(r.u8(o)>>4) & 3
		t := Title{Number: i + 1, Access: access}
		t.Object = obj(true)
		ix.Titles = append(ix.Titles, t)
	}
	if r.err != nil {
		return nil, fmt.Errorf("index.bdmv: %w", r.err)
	}
	return ix, nil
}
