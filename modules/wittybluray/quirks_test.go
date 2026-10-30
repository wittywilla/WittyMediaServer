package wittybluray

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Synthetic disc structures: each builder writes the real file format, so
// these tests run the same parsers a disc does.

func be16(v int) []byte { return []byte{byte(v >> 8), byte(v)} }
func be32(v uint32) []byte {
	return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
}

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func indexEntry(o IndexObject, access int) []byte {
	b := make([]byte, 12)
	b[0] = byte(o.Kind)<<6 | byte(access)<<4
	switch o.Kind {
	case ObjectHDMV:
		binary.BigEndian.PutUint16(b[6:], uint16(o.MovieObject))
	case ObjectBDJ:
		b[4] = 3 << 6
		copy(b[6:], o.BDJName)
	}
	return b
}

func buildIndex(version string, first, top IndexObject, titles ...IndexObject) []byte {
	head := make([]byte, 78)
	copy(head, "INDX"+version)
	binary.BigEndian.PutUint32(head[8:], 78)
	body := cat(make([]byte, 4), indexEntry(first, 0), indexEntry(top, 0), be16(len(titles)))
	for _, t := range titles {
		body = append(body, indexEntry(t, 0)...)
	}
	return append(head, body...)
}

func buildMobj(objs ...MovieObject) []byte {
	b := make([]byte, 40)
	copy(b, "MOBJ0200")
	b = append(b, make([]byte, 8)...)
	b = append(b, be16(len(objs))...)
	for _, o := range objs {
		f := 0
		if o.ResumeIntention {
			f |= 0x8000
		}
		if o.MenuCallMask {
			f |= 0x4000
		}
		b = append(b, be16(f)...)
		b = append(b, be16(len(o.Commands))...)
		for _, c := range o.Commands {
			b = append(b, cat(be32(c.Op), be32(c.Dst), be32(c.Src))...)
		}
	}
	return b
}

type testItem struct {
	clip    string
	in, out uint32
	still   int
	secs    int
	audio   []uint16
	stc     int
}

func buildMpls(playback, count int, items []testItem, marks ...Mark) []byte {
	// Header and AppInfoPlayList, 58 bytes; the playlist follows.
	head := make([]byte, 58)
	copy(head, "MPLS0200")
	head[45] = byte(playback)
	binary.BigEndian.PutUint16(head[46:], uint16(count))
	binary.BigEndian.PutUint32(head[8:], 58)
	var list []byte
	list = append(list, make([]byte, 6)...)
	list = append(list, be16(len(items))...)
	list = append(list, 0, 0)
	for _, it := range items {
		var pi []byte
		pi = append(pi, it.clip+"M2TS"...)
		pi = append(pi, 0, 1, byte(it.stc))
		pi = append(pi, cat(be32(it.in), be32(it.out))...)
		pi = append(pi, make([]byte, 8)...) // UO mask
		pi = append(pi, 0, byte(it.still))
		pi = append(pi, be16(it.secs)...)
		// STN table: a video stream and the audio streams.
		stn := make([]byte, 16)
		stn[4], stn[5] = 1, byte(len(it.audio))
		stn = append(stn, 9, 1, 0x10, 0x11, 0, 0, 0, 0, 0, 0, 5, 0x1b, 0x61, 0, 0, 0)
		for _, pid := range it.audio {
			stn = append(stn, 9, 1, byte(pid>>8), byte(pid), 0, 0, 0, 0, 0, 0, 5, 0x81, 0x31, 'e', 'n', 'g')
		}
		binary.BigEndian.PutUint16(stn, uint16(len(stn)-2))
		pi = append(pi, stn...)
		list = append(list, be16(len(pi))...)
		list = append(list, pi...)
	}
	markStart := 58 + len(list)
	binary.BigEndian.PutUint32(head[12:], uint32(markStart))
	out := append(head, list...)
	out = append(out, make([]byte, 4)...)
	out = append(out, be16(len(marks))...)
	for _, m := range marks {
		e := make([]byte, 14)
		e[1] = byte(m.Type)
		binary.BigEndian.PutUint16(e[2:], uint16(m.Item))
		binary.BigEndian.PutUint32(e[4:], m.Time)
		out = append(out, e...)
	}
	return out
}

// buildClip writes clip information with its time bases.
func buildClip(packets uint32, stcs ...STCSequence) []byte {
	b := make([]byte, 60)
	copy(b, "HDMV0200")
	b[46], b[47] = 1, 1
	binary.BigEndian.PutUint32(b[56:], packets)
	binary.BigEndian.PutUint32(b[8:], 60)
	seq := cat(make([]byte, 5), []byte{1}, make([]byte, 4), []byte{byte(len(stcs)), 0})
	for _, s := range stcs {
		seq = append(seq, cat(be16(int(s.PCRPID)), be32(s.StartSPN), be32(s.Start), be32(s.End))...)
	}
	b = append(b, seq...)
	prog := 60 + len(seq)
	binary.BigEndian.PutUint32(b[12:], uint32(prog))
	return append(b, make([]byte, 6)...)
}

// writeDisc lays files out in a temporary folder.
func writeDisc(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	for name, data := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func stream(n int) []byte {
	b := make([]byte, n*packetSize)
	for i := 0; i < n; i++ {
		b[i*packetSize+4] = 0x47
	}
	return b
}

func hdmv(n int) IndexObject { return IndexObject{Kind: ObjectHDMV, MovieObject: n} }

func play(n uint32) Command       { return Command{0x22800000, n, 0} }
func jumpTitle(n uint32) Command  { return Command{0x21810000, n, 0} }
func jumpObject(n uint32) Command { return Command{0x21800000, n, 0} }
func resume() Command             { return Command{0x01040000 | 0x20000000&0, 0, 0} }
func move(gpr, v uint32) Command  { return Command{0x50400001, gpr, v} }
func simpleDisc(t *testing.T, objs []MovieObject, pls map[string][]byte, titles ...IndexObject) *Disc {
	t.Helper()
	files := map[string][]byte{
		"BDMV/index.bdmv":       buildIndex("0200", hdmv(0), hdmv(1), titles...),
		"BDMV/MovieObject.bdmv": buildMobj(objs...),
	}
	for n, b := range pls {
		files["BDMV/PLAYLIST/"+n+".mpls"] = b
	}
	files["BDMV/CLIPINF/00001.clpi"] = buildClip(32, STCSequence{ID: 0, Start: 45000, End: 45000 * 100})
	files["BDMV/CLIPINF/00002.clpi"] = buildClip(32, STCSequence{ID: 0, Start: 45000, End: 45000 * 100})
	files["BDMV/STREAM/00001.m2ts"] = stream(32)
	files["BDMV/STREAM/00002.m2ts"] = stream(32)
	d, err := Open(writeDisc(t, files))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

var oneItem = []testItem{{clip: "00001", in: 45000, out: 45000 * 11, audio: []uint16{0x1100, 0x1101}}}

func TestArithmeticEdges(t *testing.T) {
	run := func(op, a, b uint32) uint32 {
		r := NewRegisters(RegisterOptions{})
		r.GPR[1] = a
		r.Run([]Command{{0x50400000 | op, 1, b}}, 0, RunOptions{}, nil)
		return r.GPR[1]
	}
	cases := []struct {
		name          string
		op, a, b, out uint32
	}{
		{"add saturates", 3, 0xfffffff0, 0x100, 0xffffffff},
		{"sub stops at zero", 4, 3, 5, 0},
		{"mul saturates", 5, 0x10000, 0x10000, 0xffffffff},
		{"div by zero", 6, 9, 0, 0xffffffff},
		{"mod by zero", 7, 9, 0, 0xffffffff},
		{"bit set out of range", 12, 1, 40, 1},
		{"shift out of range", 14, 1, 40, 0},
	}
	for _, c := range cases {
		if got := run(c.op, c.a, c.b); got != c.out {
			t.Errorf("%s: %#x", c.name, got)
		}
	}
	// RND is 1..n.
	r := NewRegisters(RegisterOptions{})
	r.Rand = func() float64 { return 0.999 }
	r.Run([]Command{{0x50400008, 1, 6}}, 0, RunOptions{}, nil)
	if r.GPR[1] != 6 {
		t.Errorf("RND %d", r.GPR[1])
	}
	// A bit compare is true for a bit of the first operand the second lacks.
	r.GPR[1] = 6
	res := r.Run([]Command{{0x48400100, 1, 2}, jumpTitle(7), jumpTitle(8)}, 0, RunOptions{}, nil)
	if res.Title != 7 {
		t.Errorf("BC 6,2 should be true: title %d", res.Title)
	}
	res = r.Run([]Command{{0x48400100, 1, 6}, jumpTitle(7), jumpTitle(8)}, 0, RunOptions{}, nil)
	if res.Title != 8 {
		t.Errorf("BC 6,6 should be false: title %d", res.Title)
	}
	// A register reference with stray bits is no register.
	r.GPR[1] = 5
	r.Run([]Command{{0x50000001, 2, 0x00100001}}, 0, RunOptions{}, nil)
	if r.GPR[2] != 0 {
		t.Errorf("stray reference read %d", r.GPR[2])
	}
}

func TestRegisterDefaults(t *testing.T) {
	r := NewRegisters(RegisterOptions{})
	want := map[int]uint32{PSRAudio: 0xff, PSRPG: 0x0fff0fff, PSRButton: 0xffff, PSRStyle: 0xff, PSRSecondary: 0xffff, PSRAudioCap: 0xaaaa, PSRVideoCap: 3, PSRProfile: 0x00030200, 48: 0xffffffff}
	for k, v := range want {
		if r.PSR[k] != v {
			t.Errorf("PSR%d = %#x, want %#x", k, r.PSR[k], v)
		}
	}
	if s := NewRegisters(RegisterOptions{Stereo3D: true}); s.PSR[PSRProfile] != Profile5v24 || s.PSR[PSROutputPref] != 1 {
		t.Errorf("3D: %#x", s.PSR[PSRProfile])
	}
	if u := NewRegisters(RegisterOptions{UHD: true}); u.PSR[PSRProfile] != Profile6v30 || u.PSR[PSRUHDCap] == 0 {
		t.Errorf("UHD: %#x", u.PSR[PSRProfile])
	}
	// 3D output only on a 3D player.
	var fx Effects
	r.Run([]Command{{0x3180000a, 1, 0}}, 0, RunOptions{}, &fx)
	if r.PSR[PSRStereoStatus] != 0 || fx.OutputMode3D != nil {
		t.Error("a 2D player turned on 3D output")
	}
	s := NewRegisters(RegisterOptions{Stereo3D: true})
	s.Run([]Command{{0x3180000a, 1, 0}}, 0, RunOptions{}, &fx)
	if s.PSR[PSRStereoStatus] != 1 {
		t.Error("a 3D player did not turn on 3D output")
	}
}

func TestSystemCommands(t *testing.T) {
	r := NewRegisters(RegisterOptions{})
	var fx Effects
	// SET_NV_TIMER: object first, seconds second.
	r.Run([]Command{{0x51c00002, 4, 30}}, 0, RunOptions{}, &fx)
	if fx.NavTimer == nil || fx.NavTimer.Object != 4 || fx.NavTimer.Seconds != 30 || r.PSR[PSRNavTimer] != 30 {
		t.Fatalf("timer %+v", fx.NavTimer)
	}
	fx = Effects{}
	r.Run([]Command{{0x51c00002, 4, 301}}, 0, RunOptions{}, &fx)
	if fx.NavTimer != nil {
		t.Fatal("a timer over 300 seconds is ignored")
	}
	r.Run([]Command{{0x51c00002, 4, 0}}, 0, RunOptions{}, &fx)
	if !fx.NavTimerOff || r.PSR[PSRNavTimer] != 0 {
		t.Fatal("0 cancels the timer")
	}
	// The high bit of a set option counts: 0x10 is not SET_STREAM's 0x00.
	if in := Decode(Command{Op: 0x51c00011}); in.Opt != 0x11 {
		t.Fatalf("option %#x", in.Opt)
	}
	// SET_STREAM leaves the display flag alone when it does not set subtitles,
	// and SET_SEC_STREAM fills its own half of the registers.
	r = NewRegisters(RegisterOptions{})
	r.Run([]Command{{0x51c00001, 0x8000 | 0x4000 | 3, 0}}, 0, RunOptions{}, nil)
	r.Run([]Command{{0x51c00001, 0x80020000, 0}}, 0, RunOptions{}, nil)
	if r.PSR[PSRPG]&0xfff != 3 || r.PSR[PSRPG]>>31 != 1 || r.PSR[PSRAudio] != 2 || r.PSR[PSRPG]>>16&0xfff != 0xfff {
		t.Fatalf("SET_STREAM: pg %#x audio %d", r.PSR[PSRPG], r.PSR[PSRAudio])
	}
	r.Run([]Command{{0x51c00006, 0x80000005 | 0x40000000, 0x80070000 | 0x8000 | 2}}, 0, RunOptions{}, nil)
	if v := r.PSR[PSRSecondary]; v&0xff != 7 || v>>8&0xff != 5 || v>>31 != 1 {
		t.Fatalf("SET_SEC_STREAM %#x", v)
	}
	if r.PSR[PSRPG]>>16&0xfff != 2 {
		t.Fatalf("PiP subtitles %#x", r.PSR[PSRPG])
	}
	// In a button, changing the page ends the commands; in a movie object it
	// only writes the registers.
	cmds := []Command{{0x51c00003, 0x80000002, 0x80000001}, move(1, 9)}
	r = NewRegisters(RegisterOptions{})
	fx = Effects{}
	r.Run(cmds, 0, RunOptions{Button: true}, &fx)
	if r.GPR[1] != 0 || fx.ButtonPage == nil || r.PSR[PSRButton] != 2 || r.PSR[PSRPage] != 1 {
		t.Fatalf("button: GPR1 %d %+v", r.GPR[1], fx.ButtonPage)
	}
	r = NewRegisters(RegisterOptions{})
	r.Run(cmds, 0, RunOptions{}, nil)
	if r.GPR[1] != 9 {
		t.Fatal("movie object stopped at SET_BUTTON_PAGE")
	}
	// LINK and TERMINATE mean nothing outside a button.
	res := r.Run([]Command{{0x22830000, 0, 0}, jumpTitle(3)}, 0, RunOptions{}, nil)
	if res.Kind != ResultJumpTitle {
		t.Fatalf("TERMINATE_PL in a movie object: %+v", res)
	}
}

func TestIndexWithBDJ(t *testing.T) {
	b := buildIndex("0200", IndexObject{Kind: ObjectBDJ, BDJName: "00000"}, IndexObject{Kind: ObjectBDJ, BDJName: "00001"}, hdmv(3), IndexObject{Kind: ObjectBDJ, BDJName: "00002"})
	ix, err := ParseIndex(b)
	if err != nil {
		t.Fatal(err)
	}
	if ix.FirstPlay.BDJName != "00000" || ix.TopMenu.BDJName != "00001" || len(ix.Titles) != 2 || ix.Titles[0].Object.MovieObject != 3 || ix.Titles[1].Object.BDJName != "00002" {
		t.Fatalf("%+v", ix)
	}
}

func TestResumeKeepsChoices(t *testing.T) {
	objs := []MovieObject{
		{Commands: []Command{jumpTitle(1)}},                                 // first play
		{Commands: []Command{play(2)}},                                      // top menu
		{ResumeIntention: true, Commands: []Command{play(1), jumpTitle(0)}}, // the film
		{Commands: []Command{play(1), jumpTitle(0)}},                        // a film with no resume
		{MenuCallMask: true, Commands: []Command{play(1), jumpTitle(0)}},    // a warning
	}
	d := simpleDisc(t, objs, map[string][]byte{"00001": buildMpls(1, 0, oneItem), "00002": buildMpls(1, 0, oneItem)}, hdmv(2), hdmv(3), hdmv(4))
	n := NewNav(d, NavOptions{})
	st := n.Start()
	if st.Playlist == nil || st.Playlist.Number != 1 || st.Title != 1 {
		t.Fatalf("start: %+v", st)
	}
	n.SetTime(0, 45000*5)
	n.Registers().GPR[7] = 1
	st = n.MenuCall()
	if st.Playlist.Number != 2 || st.Title != 0 {
		t.Fatalf("menu: %+v", st)
	}
	// A setup menu changes a general register and the audio stream.
	n.Registers().GPR[7] = 2
	n.SetAudio(2)
	st = n.Resume()
	if st.Playlist.Number != 1 || st.Time != 45000*5 || st.Title != 1 {
		t.Fatalf("resume: %+v", st)
	}
	r := n.Registers()
	if r.GPR[7] != 2 || r.PSR[PSRAudio] != 2 || r.PSR[PSRTitle] != 1 || r.PSR[PSRBackupTitle] != 0xffff {
		t.Fatalf("after resume: GPR7 %d audio %d title %d", r.GPR[7], r.PSR[PSRAudio], r.PSR[PSRTitle])
	}
	// No resume intention: the film ends at the menu call.
	n.PlayTitle(2)
	n.MenuCall()
	if st := n.Resume(); st.Kind != StepStay {
		t.Fatalf("resume without intention: %+v", st)
	}
	// A masked menu key does nothing.
	n.PlayTitle(3)
	if st := n.MenuCall(); st.Kind != StepStay || n.Registers().PSR[PSRTitle] != 3 {
		t.Fatalf("masked menu: %+v", st)
	}
}

func TestButtonJumpsAndCalls(t *testing.T) {
	objs := []MovieObject{
		{Commands: []Command{jumpTitle(0)}},
		{Commands: []Command{play(2)}},
		{Commands: []Command{play(1), jumpTitle(0)}},
		{Commands: []Command{{0x01040000, 0, 0}}},
	}
	d := simpleDisc(t, objs, map[string][]byte{"00001": buildMpls(1, 0, oneItem), "00002": buildMpls(1, 0, oneItem)}, hdmv(2))
	n := NewNav(d, NavOptions{})
	n.Start()
	st := n.PressButton([]Command{jumpObject(2)})
	if st.Kind != StepPlay || st.Playlist.Number != 1 {
		t.Fatalf("JUMP_OBJECT from a button: %+v", st)
	}
	// Jumping to a title forgets what could be resumed.
	n = NewNav(d, NavOptions{})
	n.Start()
	n.PressButton([]Command{jumpTitle(1)})
	if st := n.Resume(); st.Kind != StepStay {
		t.Fatalf("resume after a jump: %+v", st)
	}
}

func TestLoopDetection(t *testing.T) {
	objs := []MovieObject{
		{Commands: []Command{jumpTitle(0)}},
		{Commands: []Command{play(1), {0x20810000, 0, 0}}}, // a looping menu
		{Commands: []Command{play(1), play(2)}},            // a menu that moves on
	}
	d := simpleDisc(t, objs, map[string][]byte{"00001": buildMpls(1, 0, oneItem), "00002": buildMpls(1, 0, oneItem)}, hdmv(2))
	n := NewNav(d, NavOptions{})
	st := n.Start()
	if !n.LoopsAtEnd(st) {
		t.Fatal("a menu that plays itself again should loop")
	}
	st = n.PlayTitle(1)
	if n.LoopsAtEnd(st) {
		t.Fatal("a playlist followed by something else should not loop")
	}
}

func TestPlaylistModes(t *testing.T) {
	items := []testItem{
		{clip: "00001", in: 45000, out: 45000 * 2, audio: []uint16{0x1100}},
		{clip: "00002", in: 45000, out: 45000 * 2, audio: []uint16{0x1100}},
		{clip: "00001", in: 45000 * 2, out: 45000 * 3, audio: []uint16{0x1100}},
	}
	objs := []MovieObject{{Commands: []Command{play(1), jumpTitle(0xffff)}}, {Commands: []Command{play(1)}}}
	for _, c := range []struct {
		name     string
		mode, n  int
		distinct bool
		plays    int
	}{{"random", 2, 5, false, 5}, {"shuffle", 3, 3, true, 3}} {
		d := simpleDisc(t, objs, map[string][]byte{"00001": buildMpls(c.mode, c.n, items)})
		n := NewNav(d, NavOptions{})
		seed := 0.0
		n.Registers().Rand = func() float64 { seed += 0.37; return seed - float64(int(seed)) }
		st := n.Start()
		seen := map[int]bool{}
		got := 0
		for st.Kind == StepPlay && st.Playlist.Number == 1 && got < 50 && st.ItemOnly {
			got++
			seen[st.Item] = true
			if got == c.plays {
				break
			}
			st = n.PlaylistEnded()
		}
		if got != c.plays || (c.distinct && len(seen) != 3) {
			t.Fatalf("%s: %d plays, items %v", c.name, got, seen)
		}
	}
	// Stills: a held item plays alone and says for how long.
	still := []testItem{
		{clip: "00001", in: 45000, out: 45000 * 2, still: 1, secs: 7, audio: []uint16{0x1100}},
		{clip: "00001", in: 45000 * 2, out: 45000 * 3, still: 2, audio: []uint16{0x1100}},
	}
	d := simpleDisc(t, objs, map[string][]byte{"00001": buildMpls(1, 0, still)})
	n := NewNav(d, NavOptions{})
	st := n.Start()
	if !st.ItemOnly || st.StillSec != 7 {
		t.Fatalf("timed still: %+v", st)
	}
	st = n.PlaylistEnded()
	if st.Item != 1 || st.StillSec != -1 {
		t.Fatalf("held still: %+v", st)
	}
	if back, ok := n.SkipItem(-1); !ok || back.Item != 0 {
		t.Fatalf("slideshow back: %+v", back)
	}
	// Items with different streams cannot share one stream.
	mixed := []testItem{
		{clip: "00001", in: 45000, out: 45000 * 2, audio: []uint16{0x1100}},
		{clip: "00002", in: 45000, out: 45000 * 2, audio: []uint16{0x1101}},
	}
	d = simpleDisc(t, objs, map[string][]byte{"00001": buildMpls(1, 0, mixed)})
	if st := NewNav(d, NavOptions{}).Start(); !st.ItemOnly {
		t.Fatal("items with different streams should play one at a time")
	}
	d = simpleDisc(t, objs, map[string][]byte{"00001": buildMpls(1, 0, oneItem)})
	if st := NewNav(d, NavOptions{}).Start(); st.ItemOnly {
		t.Fatal("a plain playlist plays whole")
	}
}

func TestStreamSelectionAndChapters(t *testing.T) {
	objs := []MovieObject{{Commands: []Command{play(1)}}, {Commands: []Command{play(1)}}}
	marks := []Mark{{Type: 1, Item: 0, Time: 45000}, {Type: 2, Item: 0, Time: 45000 * 2}, {Type: 1, Item: 0, Time: 45000 * 3}}
	d := simpleDisc(t, objs, map[string][]byte{"00001": buildMpls(1, 0, oneItem, marks...)})
	n := NewNav(d, NavOptions{Registers: RegisterOptions{AudioLang: "eng"}})
	n.Start()
	r := n.Registers()
	if r.PSR[PSRAudio] != 1 || r.PSR[PSRChapter] != 1 {
		t.Fatalf("audio %d chapter %d", r.PSR[PSRAudio], r.PSR[PSRChapter])
	}
	// A chapter counts entry marks only.
	n.PlayPlaylist(d.Playlist(1), 0, 45000*4)
	if r.PSR[PSRChapter] != 2 {
		t.Fatalf("chapter %d", r.PSR[PSRChapter])
	}
	// A stream number the item lacks is replaced.
	n.SetAudio(9)
	n.PlayPlaylist(d.Playlist(1), 0, 0)
	if r.PSR[PSRAudio] != 1 {
		t.Fatalf("audio %d", r.PSR[PSRAudio])
	}
}

func TestMainPlaylistDecoys(t *testing.T) {
	real := []testItem{
		{clip: "00001", in: 45000, out: 45000 * 500, audio: []uint16{0x1100}},
		{clip: "00002", in: 45000 * 500, out: 45000 * 1000, audio: []uint16{0x1100}},
	}
	shuffled := []testItem{real[1], real[0]}
	looped := []testItem{real[0], real[0], real[0], real[0], real[0], real[0]}
	missing := []testItem{{clip: "00009", in: 45000, out: 45000 * 9000, audio: []uint16{0x1100}}}
	past := []testItem{{clip: "00001", in: 45000, out: 45000 * 5000, audio: []uint16{0x1100}}}
	files := map[string][]byte{
		"BDMV/index.bdmv":          buildIndex("0200", hdmv(0), hdmv(0)),
		"BDMV/MovieObject.bdmv":    buildMobj(MovieObject{Commands: []Command{play(5)}}),
		"BDMV/PLAYLIST/00001.mpls": buildMpls(1, 0, shuffled),
		"BDMV/PLAYLIST/00002.mpls": buildMpls(1, 0, looped),
		"BDMV/PLAYLIST/00003.mpls": buildMpls(1, 0, missing),
		"BDMV/PLAYLIST/00004.mpls": buildMpls(1, 0, past),
		"BDMV/PLAYLIST/00005.mpls": buildMpls(1, 0, real),
		"BDMV/CLIPINF/00001.clpi":  buildClip(32, STCSequence{Start: 45000, End: 45000 * 500}),
		"BDMV/CLIPINF/00002.clpi":  buildClip(32, STCSequence{Start: 45000 * 500, End: 45000 * 1000}),
		"BDMV/STREAM/00001.m2ts":   stream(32),
		"BDMV/STREAM/00002.m2ts":   stream(32),
	}
	d, err := Open(writeDisc(t, files))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if pl := d.MainPlaylist(); pl == nil || pl.Number != 5 {
		t.Fatalf("main playlist %+v", pl)
	}
	// With no program to name it, the clips that meet in time win.
	files["BDMV/PLAYLIST/00001.mpls"], files["BDMV/PLAYLIST/00005.mpls"] = buildMpls(1, 0, real), buildMpls(1, 0, shuffled)
	files["BDMV/MovieObject.bdmv"] = buildMobj()
	d2, err := Open(writeDisc(t, files))
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	if pl := d2.MainPlaylist(); pl == nil || pl.Number != 1 {
		t.Fatalf("seams: %+v", pl)
	}
}

func TestAudioOnlyDisc(t *testing.T) {
	// A playlist with no video stream is a music disc's feature.
	p := buildMpls(1, 0, oneItem)
	pl, err := ParsePlaylist(1, p)
	if err != nil {
		t.Fatal(err)
	}
	pl.Items[0].Video = nil
	d := &Disc{Playlists: map[int]*Playlist{1: pl}, Clips: map[string]*Clip{"00001": {Name: "00001"}}, Index: &Index{}}
	if d.MainPlaylist() != pl {
		t.Fatal("no feature on an audio-only disc")
	}
}

func TestBackupAndLayouts(t *testing.T) {
	good := buildMpls(1, 0, oneItem)
	base := func() map[string][]byte {
		return map[string][]byte{
			"BDMV/index.bdmv":          buildIndex("0200", hdmv(0), hdmv(0)),
			"BDMV/MovieObject.bdmv":    buildMobj(MovieObject{Commands: []Command{play(1)}}),
			"BDMV/PLAYLIST/00001.mpls": good,
			"BDMV/CLIPINF/00001.clpi":  buildClip(32, STCSequence{Start: 45000, End: 45000 * 100}),
			"BDMV/STREAM/00001.m2ts":   stream(32),
		}
	}
	// A damaged index, and a playlist only the backup still has.
	f := base()
	f["BDMV/BACKUP/index.bdmv"] = f["BDMV/index.bdmv"]
	f["BDMV/index.bdmv"] = []byte("damaged")
	delete(f, "BDMV/PLAYLIST/00001.mpls")
	f["BDMV/BACKUP/PLAYLIST/00001.mpls"] = good
	d, err := Open(writeDisc(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if d.Playlist(1) == nil || d.Index.FirstPlay.Kind != ObjectHDMV || len(d.Damaged) != 0 {
		t.Fatalf("backup: %+v damaged %v", d.Index, d.Damaged)
	}
	d.Close()
	// Everything damaged and no backup: the clip still plays.
	f = base()
	f["BDMV/index.bdmv"] = []byte("damaged")
	f["BDMV/MovieObject.bdmv"] = []byte("damaged")
	f["BDMV/PLAYLIST/00001.mpls"] = []byte("damaged")
	d, err = Open(writeDisc(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if d.Playlist(1) == nil || len(d.Damaged) == 0 {
		t.Fatalf("recovery: %+v", d.Playlists)
	}
	d.Close()
	// AVCHD on a camcorder card, with 8.3 names under PRIVATE.
	f = map[string][]byte{
		"PRIVATE/AVCHD/BDMV/INDEX.BDM":          buildIndex("0100", hdmv(0), hdmv(0)),
		"PRIVATE/AVCHD/BDMV/MOVIEOBJ.BDM":       buildMobj(MovieObject{Commands: []Command{play(1)}}),
		"PRIVATE/AVCHD/BDMV/PLAYLIST/00001.MPL": good,
		"PRIVATE/AVCHD/BDMV/CLIPINF/00001.CPI":  buildClip(32, STCSequence{Start: 45000, End: 45000 * 100}),
		"PRIVATE/AVCHD/BDMV/STREAM/00001.MTS":   stream(32),
	}
	d, err = Open(writeDisc(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if d.Playlist(1) == nil || d.Clip("00001") == nil {
		t.Fatalf("AVCHD: %v %v", d.Playlists, d.Clips)
	}
	if ext, err := d.ClipExtents("00001"); err != nil || len(ext) != 1 || !strings.HasSuffix(strings.ToUpper(ext[0].Path), ".MTS") {
		t.Fatalf("AVCHD stream: %+v %v", ext, err)
	}
	d.Close()
	// A BDAV recording has clips and no program.
	f = map[string][]byte{
		"BDAV/info.bdav":          []byte("BDAV"),
		"BDAV/CLIPINF/00007.clpi": buildClip(32, STCSequence{Start: 45000, End: 45000 * 100}),
		"BDAV/STREAM/00007.m2ts":  stream(32),
	}
	d, err = Open(writeDisc(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if !d.Recording || d.Playlist(7) == nil || d.MainPlaylist() != nil && d.MainPlaylist().Number != 7 {
		t.Fatalf("BDAV: %+v", d.Playlists)
	}
	d.Close()
	// Names in the wrong case, as a copy from another system leaves them.
	f = map[string][]byte{}
	for k, v := range base() {
		f[strings.ToLower(k)] = v
	}
	d, err = Open(writeDisc(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if d.Playlist(1) == nil {
		t.Fatalf("lower case: %+v", d.Playlists)
	}
	d.Close()
}

func encryptedStream(units int) []byte {
	b := make([]byte, units*alignedUnit)
	for i := range b {
		b[i] = byte(i*7 + i>>8) // noise; never mind the odd 0x47
	}
	for u := 0; u < units; u++ {
		o := u * alignedUnit
		b[o] = 0xc0
		b[o+4] = 0x47
	}
	return b
}

func TestEncryptionDetection(t *testing.T) {
	clear := stream(32 * 8)
	mkfiles := func(data []byte, aacs bool) map[string][]byte {
		f := map[string][]byte{
			"BDMV/index.bdmv":          buildIndex("0200", hdmv(0), hdmv(0)),
			"BDMV/MovieObject.bdmv":    buildMobj(MovieObject{Commands: []Command{play(1)}}),
			"BDMV/PLAYLIST/00001.mpls": buildMpls(1, 0, oneItem),
			"BDMV/CLIPINF/00001.clpi":  buildClip(uint32(len(data)/packetSize), STCSequence{Start: 45000, End: 45000 * 100}),
			"BDMV/STREAM/00001.m2ts":   data,
		}
		if aacs {
			f["AACS/Unit_Key_RO.inf"] = []byte{1}
		}
		return f
	}
	for _, c := range []struct {
		name      string
		data      []byte
		aacs      bool
		encrypted bool
	}{
		{"clear", clear, false, false},
		{"decrypted copy that kept the AACS folder", clear, true, false},
		{"encrypted", encryptedStream(8), true, true},
		{"encrypted with its AACS folder removed", encryptedStream(8), false, true},
	} {
		d, err := Open(writeDisc(t, mkfiles(c.data, c.aacs)))
		if err != nil {
			t.Fatal(c.name, err)
		}
		if d.Encrypted != c.encrypted {
			t.Errorf("%s: encrypted = %v", c.name, d.Encrypted)
		}
		d.Close()
	}
	// A clear start in front of an encrypted middle is still encrypted.
	mix := append(stream(32*64), encryptedStream(64)...)
	d, err := Open(writeDisc(t, mkfiles(mix, false)))
	if err != nil {
		t.Fatal(err)
	}
	if !d.Encrypted {
		t.Error("clear intro, encrypted film")
	}
	d.Close()
	// A BD+ program is noted.
	f := mkfiles(clear, false)
	f["BDSVM/00000.svm"] = []byte{1}
	d, _ = Open(writeDisc(t, f))
	if !d.BDPlus || d.Encrypted {
		t.Errorf("BD+: %v %v", d.BDPlus, d.Encrypted)
	}
	d.Close()
}

func TestMultipleTimeBases(t *testing.T) {
	c := &Clip{Packets: 1000, STC: []STCSequence{{ID: 0, StartSPN: 0, Start: 45000, End: 45000 * 10}, {ID: 1, StartSPN: 500, Start: 45000, End: 45000 * 10}},
		EPMaps: []EPMap{{PID: 0x1011, Entries: []EPEntry{{PTS: 45000, SPN: 0}, {PTS: 45000 * 5, SPN: 100}, {PTS: 45000, SPN: 500}, {PTS: 45000 * 5, SPN: 600}}}}}
	if e, ok := c.EntryAt(0x1011, 1, 45000*6); !ok || e.SPN != 600 {
		t.Fatalf("second time base: %+v", e)
	}
	if e, ok := c.EntryAt(0x1011, 0, 45000*6); !ok || e.SPN != 100 {
		t.Fatalf("first time base: %+v", e)
	}
	// A stream with no map of its own falls back on the clip's map.
	if e, ok := c.EntryAt(0x1100, 1, 45000*6); !ok || e.SPN != 600 {
		t.Fatalf("audio only: %+v", e)
	}
	if c.EndPTS(1) != 45000*10 || (&Clip{}).EndPTS(0) != 0 {
		t.Fatal("EndPTS")
	}
}

func TestBDJObject(t *testing.T) {
	var b []byte
	b = append(b, "BDJO0200"...)
	b = append(b, make([]byte, 40)...)
	b = append(b, cat(be32(10), []byte("*****"), []byte{0x3c}, make([]byte, 4))...) // terminal info: config 3, both masks
	b = append(b, cat(be32(14), []byte{1, 0}, []byte{1}, []byte("00000"), []byte("*.*"), make([]byte, 3))...)
	b = append(b, cat(be32(10), be32(2<<21|1<<19), []byte("00004"), []byte{0}, []byte("00007"), []byte{0})...)
	// One application.
	app := cat([]byte{1, 0x10}, be32(0x56789abc), be16(0x4001), make([]byte, 10), be16(1<<12), []byte{0, 1, 1, 0, 0, 0}, []byte{200, 0x70})
	app = append(app, cat(be16(8), []byte("eng"), []byte{4}, []byte("Menu"))...)
	app = append(app, 0, 0) // no icon (length 0, then padding)
	app = append(app, 0, 0) // icon flags
	app = append(app, 5)
	app = append(app, "00000"...)
	app = append(app, 0, 0) // classpath
	app = append(app, 7)
	app = append(app, "a.b.Foo"...)
	app = append(app, 3, 2, 'h', 'i')
	b = append(b, cat(be32(uint32(len(app)+2)), []byte{1, 0}, app)...)
	b = append(b, be32(0xffe00000)...)
	b = append(b, be16(3)...)
	b = append(b, "a;b"...)
	o, err := ParseBDJObject("00000", b)
	if err != nil {
		t.Fatal(err)
	}
	if o.DefaultFont != "" || o.HAViConfig != 3 || !o.MenuCallMask || !o.TitleSearchMask {
		t.Fatalf("terminal: %+v", o)
	}
	if len(o.Cache) != 1 || o.Cache[0].Name != "00000" || o.Cache[0].Lang != "*.*" || o.Cache[0].Dir {
		t.Fatalf("cache: %+v", o.Cache)
	}
	if len(o.Playlists) != 2 || o.Playlists[1] != 7 || o.AccessToAll || !o.AutostartFirst {
		t.Fatalf("playlists: %+v", o)
	}
	if len(o.Apps) != 1 {
		t.Fatalf("apps: %+v", o.Apps)
	}
	a := o.Apps[0]
	if a.Control != 1 || a.Type != 1 || a.Org != 0x56789abc || a.ID != 0x4001 || a.Priority != 200 || a.Names["eng"] != "Menu" || a.BaseDir != "00000" || a.InitialClass != "a.b.Foo" || len(a.Params) != 1 || a.Params[0] != "hi" {
		t.Fatalf("app: %+v", a)
	}
	if o.KeyInterest != 0xffe00000 || len(o.FileAccess) != 2 || len(o.Autostart()) != 1 {
		t.Fatalf("rest: %+v", o)
	}
	if safeName("../x") || safeName("") || !safeName("00000") {
		t.Fatal("safeName")
	}
}

func TestSounds(t *testing.T) {
	b := make([]byte, 40)
	copy(b, "BCLK0200")
	binary.BigEndian.PutUint32(b[8:], 66)
	b = append(b, 0, 0, 0, 26, 0, 2)
	b = append(b, cat([]byte{0x31, 0x10}, be32(0), be32(4))...)
	b = append(b, cat([]byte{0x11, 0x10}, be32(4), be32(2))...)
	b = append(b, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc)
	s, err := ParseSounds(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Clips) != 2 || s.Clips[0].Channels != 2 || s.Clips[1].Channels != 1 {
		t.Fatalf("%+v", s.Clips)
	}
	w, ok := s.WAV(0)
	if !ok || string(w[:4]) != "RIFF" || len(w) != 48 || w[44] != 0x34 || w[45] != 0x12 || w[22] != 2 {
		t.Fatalf("WAV %v %v", w, ok)
	}
	if _, ok := s.WAV(9); ok {
		t.Fatal("no such sound")
	}
}

func TestTextSubtitles(t *testing.T) {
	// The style segment: one region style and a palette.
	st := []byte{0x80, 0, 1, 0}
	st = append(st, 1)
	st = append(st, cat(be16(100), be16(900), be16(1720), be16(150))...)
	st = append(st, 2, 0)
	st = append(st, cat(be16(0), be16(0), be16(1720), be16(150))...)
	st = append(st, 1, 2, 3, 0, 0, 2, 40, 3, 4, 1)
	st = append(st, be16(10)...)
	st = append(st, 3, 235, 128, 128, 255)
	st = append(st, 4, 16, 128, 128, 255)
	styles, _ := parseDialogStyle(st)
	if len(styles) != 1 || styles[0].X != 100 || styles[0].Y != 900 || styles[0].HAlign != 2 || styles[0].VAlign != 3 || !styles[0].Italic || styles[0].FontSize != 40 || styles[0].Color.R != 255 || styles[0].OutlineColor.R != 0 {
		t.Fatalf("style %+v", styles)
	}
	// A dialog: forced, two lines, with a font change between them.
	inline := cat([]byte{0x1b, 1, 5}, []byte("Hello"), []byte{0x1b, 4, 1, 50}, []byte{0x1b, 0x0a, 0}, []byte{0x1b, 1, 5}, []byte("there"))
	dl := []byte{0, 0, 1, 0x5f, 0x90}
	dl = append(dl, 0, 0, 2, 0xbf, 0x20)
	dl = append(dl, 0, 1)
	dl = append(dl, 0x40, 1)
	dl = append(dl, be16(len(inline))...)
	dl = append(dl, inline...)
	var pal Palette
	cues := parseDialog(dl, styles, &pal, 1)
	if len(cues) != 1 || cues[0].Text != "Hello\nthere" || !cues[0].Forced || cues[0].Start != 45000 || cues[0].End != 90000 || cues[0].Style.X != 100 {
		t.Fatalf("cue %+v", cues)
	}
	// UTF-16 text, and a coding the engine has no table for.
	if s := decodeText([]byte{0, 'H', 0, 'i'}, 2); s != "Hi" {
		t.Fatalf("UTF-16 %q", s)
	}
	if s := decodeText([]byte{0x82, 0xa0}, 3); s != "" {
		t.Fatalf("Shift-JIS %q", s)
	}
}

func TestMenuTimeouts(t *testing.T) {
	// An in-mux menu: selection ends at 5 s, composition at 9 s.
	pts := func(sec uint64) []byte {
		v := sec * 90000
		return []byte{byte(v >> 32 & 1), byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
	}
	b := []byte{0, 0, 0}
	b = append(b, 0x40) // pop-up, in-mux
	b = append(b, pts(9)...)
	b = append(b, pts(5)...)
	b = append(b, 0, 0x2b, 0xf2, 0) // user timeout: 32 s
	b = b[:len(b)-1]
	b = append(b, 0) // no pages
	m := &Menu{}
	if err := m.parseIC(b); err != nil {
		t.Fatal(err)
	}
	if !m.Popup || m.Timeout != 9 || m.SelectionTimeout != 5 {
		t.Fatalf("%+v", m)
	}
}

func TestPGWindowsAndForced(t *testing.T) {
	pic := &Object{ID: 1, Width: 10, Height: 4, Pixels: make([]byte, 40)}
	for i := range pic.Pixels {
		pic.Pixels[i] = 1
	}
	pics := map[int]*Object{1: pic, 2: {ID: 2, Width: 2, Height: 2, Pixels: []byte{1, 1, 1, 1}}}
	var pal Palette
	pal[1].A, pal[1].R = 255, 200
	// A window narrower than the picture cuts it.
	sub := compose([]compObject{{object: 1, window: 0, x: 100, y: 100}}, []Window{{ID: 0, X: 103, Y: 100, W: 4, H: 4}}, pal, pics, 1920, 1080)
	if sub == nil || sub.X != 103 || sub.Image.Rect.Dx() != 4 || sub.Image.Pix[3] != 255 {
		t.Fatalf("window: %+v", sub)
	}
	// An object off the frame's edge is cut there.
	if sub := compose([]compObject{{object: 1, x: 1915, y: 0}}, nil, pal, pics, 1920, 1080); sub == nil || sub.Image.Rect.Dx() != 5 {
		t.Fatalf("edge: %+v", sub)
	}
	// Palette colours follow the picture's size.
	hd, uhd := yCrCbA(100, 200, 100, 255, matrix709), yCrCbA(100, 200, 100, 255, matrix2020)
	if hd.R == uhd.R || matrixFor(2160) != matrix2020 || matrixFor(1080) != matrix709 || matrixFor(576) != matrix601 {
		t.Fatalf("matrices: %v %v", hd, uhd)
	}
}
