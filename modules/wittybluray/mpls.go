package wittybluray

import (
	"errors"
	"fmt"
)

// BDMV/PLAYLIST/xxxxx.mpls: a playlist is what a title or a menu plays: a
// sequence of play items, each a stretch of one clip (an .m2ts stream file)
// with the streams the user may choose from; sub-paths for streams kept in
// other clips (menu graphics, secondary audio, 3D dependent views); and
// marks, which are the chapters.

// Playlist is one .mpls file.
type Playlist struct {
	Number       int
	Version      string
	PlaybackType int // 1 = sequential, 2 = random, 3 = shuffle
	// PlaybackCount is how many items a random or shuffle playlist plays.
	PlaybackCount int
	UOMask        uint64 // user operations the disc forbids (see UO constants)
	RandomAccess  bool   // the user may not jump around (chapter search, time search) when false
	Items         []PlayItem
	SubPaths      []SubPath
	Marks         []Mark
}

// PlayItem is one stretch of a clip.
type PlayItem struct {
	Clip  string // the clip's five-digit name, e.g. "00005"
	Codec string // "M2TS"
	// ConnectionCondition says how this item joins the previous one: 1 =
	// seamless (same clip continues), 5 = seamless across clips, 6 = a break
	// (the player may pause briefly).
	ConnectionCondition int
	STCID               int
	In, Out             uint32 // 45 kHz ticks into the clip
	UOMask              uint64
	RandomAccess        bool
	// StillMode: 0 = none, 1 = a still of StillTime seconds after the item,
	// 2 = a still until the user acts.
	StillMode int
	StillTime int
	// Angles lists the clips of a multi-angle item; the first is the main
	// angle (the same as Clip).
	Angles           []AngleClip
	SeamlessAngles   bool
	DifferentAudios  bool
	Video, Audio, PG []Stream
	IG, SecAudio     []Stream
	SecVideo, PiPPG  []Stream
}

// Duration is the item's length in seconds.
func (p *PlayItem) Duration() float64 {
	if p.Out <= p.In {
		return 0
	}
	return ticks45(p.Out - p.In)
}

// AngleClip is one angle's clip.
type AngleClip struct {
	Clip  string
	Codec string
	STCID int
}

// Stream is one selectable stream of a play item.
type Stream struct {
	Kind string // video, audio, pg (subtitles), ig (menu), sec-audio, sec-video, pip-pg
	// EntryType says where the stream is: 1 = in this item's clip, 2 = in a
	// sub-path's clip, 3 = in a sub-path (text subtitles), 4 = in a sub-path
	// clip with its own PID.
	EntryType int
	PID       uint16
	SubPath   int
	SubClip   int
	Coding    byte // MPEG-TS stream type: 0x1b H.264, 0x24 HEVC, 0xea VC-1, 0x02 MPEG-2, 0x20 MVC; 0x80 LPCM, 0x81 AC-3, 0x82 DTS, 0x83 TrueHD, 0x84 E-AC-3, 0x85 DTS-HD, 0x86 DTS-HD MA; 0x90 PG, 0x91 IG, 0x92 text
	Format    int  // video: 1 480i 2 576i 3 480p 4 1080i 5 720p 6 1080p 7 576p 8 2160p; audio: 1 mono 3 stereo 6 multi-channel 12 stereo+multi
	Rate      int  // video frame rate (see Index.FrameRate); audio: 1 48 kHz, 4 96 kHz, 5 192 kHz, 12 48/192, 14 48/96
	Lang      string
	CharCode  int // text subtitles: character coding
}

// CodingName is a readable name for the stream's coding.
func (s Stream) CodingName() string {
	switch s.Coding {
	case 0x01:
		return "MPEG-1 video"
	case 0x02:
		return "MPEG-2 video"
	case 0x1b:
		return "H.264"
	case 0x20:
		return "H.264 MVC (3D)"
	case 0x24:
		return "HEVC"
	case 0xea:
		return "VC-1"
	case 0x03, 0x04:
		return "MPEG audio"
	case 0x80:
		return "LPCM"
	case 0x81:
		return "AC-3"
	case 0x82:
		return "DTS"
	case 0x83:
		return "TrueHD"
	case 0x84:
		return "E-AC-3"
	case 0x85:
		return "DTS-HD"
	case 0x86:
		return "DTS-HD Master Audio"
	case 0xa1:
		return "E-AC-3 (secondary)"
	case 0xa2:
		return "DTS-HD (secondary)"
	case 0x90:
		return "subtitles (PG)"
	case 0x91:
		return "menu graphics (IG)"
	case 0x92:
		return "text subtitles"
	}
	return fmt.Sprintf("type 0x%02x", s.Coding)
}

// SubPath is a sequence of sub play items with streams that play alongside
// the main path: out-of-mux menu graphics, secondary audio or video,
// picture-in-picture, text subtitles, or a 3D dependent view.
type SubPath struct {
	// Type: 2 = primary audio of a browsable slideshow, 3 = IG menu, 4 = text
	// subtitles, 5 = out-of-mux synchronous (secondary audio/video), 6 =
	// out-of-mux asynchronous PiP, 7 = in-mux synchronous PiP, 8 = 3D
	// dependent view (MVC), 9 = 3D dependent view with PiP.
	Type   int
	Repeat bool
	Items  []SubPlayItem
}

// SubPlayItem is one stretch of a clip in a sub-path.
type SubPlayItem struct {
	Clip                string
	Codec               string
	ConnectionCondition int
	STCID               int
	In, Out             uint32
	SyncItem            int    // the main path's play item this one is synchronised with
	SyncStart           uint32 // and the time in it (45 kHz)
	MultiClips          []AngleClip
}

// Mark is a chapter (Type 1, "entry mark") or a link point (Type 2).
type Mark struct {
	Type     int
	Item     int    // index into Items
	Time     uint32 // 45 kHz ticks into the clip, in the item's time base
	PID      uint16 // the stream it applies to, 0xffff for all
	Duration uint32
}

// User operation mask bits (UOMask). A set bit means the operation is
// forbidden while the playlist or item plays.
const (
	UOMenuCall       = 1 << 63
	UOTitleSearch    = 1 << 62
	UOChapterSearch  = 1 << 61
	UOTimeSearch     = 1 << 60
	UOSkipToNext     = 1 << 59
	UOSkipBack       = 1 << 58
	UOPlay           = 1 << 57
	UOStop           = 1 << 56
	UOPauseOn        = 1 << 55
	UOStill          = 1 << 53
	UOForward        = 1 << 52
	UOBackward       = 1 << 51
	UOResume         = 1 << 50
	UOMoveUp         = 1 << 49
	UOMoveDown       = 1 << 48
	UOMoveLeft       = 1 << 47
	UOMoveRight      = 1 << 46
	UOSelect         = 1 << 45
	UOActivate       = 1 << 44
	UOSelectActivate = 1 << 43
	UOPrimaryAudio   = 1 << 42
	UOAngle          = 1 << 40
	UOPopup          = 1 << 39
	UOPGSelect       = 1 << 38
	UOPGEnable       = 1 << 37
	UOSecVideo       = 1 << 36
	UOSecAudio       = 1 << 34
	UOPiPPG          = 1 << 32
)

// ParsePlaylist parses one .mpls file.
func ParsePlaylist(number int, b []byte) (*Playlist, error) {
	r := &rd{b: b}
	if r.str(0, 4) != "MPLS" {
		return nil, errors.New("not a playlist file")
	}
	pl := &Playlist{Number: number, Version: r.str(4, 4)}
	plStart, markStart := int(r.u32(8)), int(r.u32(12))
	// AppInfoPlayList
	pl.PlaybackType = int(r.u8(45))
	pl.PlaybackCount = int(r.u16(46))
	pl.UOMask = r.u64(48)
	pl.RandomAccess = r.u16(56)&0x8000 == 0
	// PlayList
	o := plStart
	nItems, nSub := int(r.u16(o+6)), int(r.u16(o+8))
	o += 10
	if nItems > 1000 || nSub > 255 {
		return nil, fmt.Errorf("%d play items and %d sub-paths is more than a playlist can have", nItems, nSub)
	}
	for i := 0; i < nItems && r.err == nil; i++ {
		l := int(r.u16(o))
		it := parsePlayItem(r, o+2)
		pl.Items = append(pl.Items, it)
		o += 2 + l
	}
	for i := 0; i < nSub && r.err == nil; i++ {
		l := int(r.u32(o))
		sp := parseSubPath(r, o+4)
		pl.SubPaths = append(pl.SubPaths, sp)
		o += 4 + l
	}
	// PlayListMark
	o = markStart
	nMarks := int(r.u16(o + 4))
	o += 6
	if nMarks > 10000 {
		return nil, fmt.Errorf("%d marks is more than a playlist can have", nMarks)
	}
	for i := 0; i < nMarks && r.err == nil; i++ {
		pl.Marks = append(pl.Marks, Mark{Type: int(r.u8(o + 1)), Item: int(r.u16(o + 2)), Time: r.u32(o + 4), PID: r.u16(o + 8), Duration: r.u32(o + 10)})
		o += 14
	}
	if r.err != nil {
		return nil, r.err
	}
	return pl, nil
}

func parsePlayItem(r *rd, o int) PlayItem {
	it := PlayItem{Clip: r.str(o, 5), Codec: r.str(o+5, 4)}
	flags := r.u16(o + 9)
	multi := flags&0x10 != 0
	it.ConnectionCondition = int(flags & 15)
	it.STCID = int(r.u8(o + 11))
	it.In, it.Out = r.u32(o+12), r.u32(o+16)
	it.UOMask = r.u64(o + 20)
	it.RandomAccess = r.u8(o+28)&0x80 == 0
	it.StillMode = int(r.u8(o + 29))
	it.StillTime = int(r.u16(o + 30))
	it.Angles = []AngleClip{{Clip: it.Clip, Codec: it.Codec, STCID: it.STCID}}
	p := o + 32
	if multi {
		n := int(r.u8(p))
		af := r.u8(p + 1)
		it.DifferentAudios = af&2 != 0
		it.SeamlessAngles = af&1 != 0
		p += 2
		for k := 1; k < n && k < 10 && r.err == nil; k++ {
			it.Angles = append(it.Angles, AngleClip{Clip: r.str(p, 5), Codec: r.str(p+5, 4), STCID: int(r.u8(p + 9))})
			p += 10
		}
	}
	// STN table
	counts := [7]int{}
	for k := range counts {
		counts[k] = int(r.u8(p + 4 + k))
	}
	q := p + 16
	read := func(kind string, n int) []Stream {
		var out []Stream
		for k := 0; k < n && r.err == nil; k++ {
			s, next := parseStream(r, q, kind)
			out = append(out, s)
			q = next
		}
		return out
	}
	it.Video = read("video", counts[0])
	it.Audio = read("audio", counts[1])
	it.PG = read("pg", counts[2])
	it.IG = read("ig", counts[3])
	it.SecAudio = read("sec-audio", counts[4])
	it.SecVideo = read("sec-video", counts[5])
	it.PiPPG = read("pip-pg", counts[6])
	return it
}

// parseStream reads a stream entry and its attributes; returns the stream
// and the offset after them.
func parseStream(r *rd, o int, kind string) (Stream, int) {
	s := Stream{Kind: kind}
	el := int(r.u8(o))
	s.EntryType = int(r.u8(o + 1))
	switch s.EntryType {
	case 1:
		s.PID = r.u16(o + 2)
	case 2, 4:
		s.SubPath, s.SubClip, s.PID = int(r.u8(o+2)), int(r.u8(o+3)), r.u16(o+4)
	case 3:
		s.SubPath, s.PID = int(r.u8(o+2)), r.u16(o+3)
	}
	o += 1 + el
	al := int(r.u8(o))
	s.Coding = r.u8(o + 1)
	a := o + 2
	switch s.Coding {
	case 0x01, 0x02, 0x1b, 0x20, 0x24, 0xea:
		s.Format, s.Rate = int(r.u8(a)>>4), int(r.u8(a)&15)
	case 0x03, 0x04, 0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0xa1, 0xa2:
		s.Format, s.Rate = int(r.u8(a)>>4), int(r.u8(a)&15)
		s.Lang = r.str(a+1, 3)
	case 0x90, 0x91:
		s.Lang = r.str(a, 3)
	case 0x92:
		s.CharCode = int(r.u8(a))
		s.Lang = r.str(a+1, 3)
	}
	return s, o + 1 + al
}

func parseSubPath(r *rd, o int) SubPath {
	sp := SubPath{Type: int(r.u8(o + 1)), Repeat: r.u16(o+2)&1 != 0}
	n := int(r.u8(o + 5))
	p := o + 6
	for i := 0; i < n && i < 255 && r.err == nil; i++ {
		l := int(r.u16(p))
		q := p + 2
		it := SubPlayItem{Clip: r.str(q, 5), Codec: r.str(q+5, 4)}
		w := r.u32(q + 9)
		it.ConnectionCondition = int(w>>1) & 15
		multi := w&1 != 0
		it.STCID = int(r.u8(q + 13))
		it.In, it.Out = r.u32(q+14), r.u32(q+18)
		it.SyncItem = int(r.u16(q + 22))
		it.SyncStart = r.u32(q + 24)
		if multi {
			m := int(r.u8(q + 28))
			e := q + 29
			for k := 1; k < m && k < 10 && r.err == nil; k++ {
				it.MultiClips = append(it.MultiClips, AngleClip{Clip: r.str(e, 5), Codec: r.str(e+5, 4), STCID: int(r.u8(e + 9))})
				e += 10
			}
		}
		sp.Items = append(sp.Items, it)
		p += 2 + l
	}
	return sp
}
