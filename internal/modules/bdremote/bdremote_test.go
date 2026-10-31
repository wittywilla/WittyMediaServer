package bdremote

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"errors"
	"image"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittybluray/helper"
	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymodule"
)

// ---------------------------------------------------------------- a disc

// A small disc in the real file formats, so these tests run the real parsers.

func be16(v int) []byte    { return []byte{byte(v >> 8), byte(v)} }
func be32(v uint32) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func indexEntry(o wittybluray.IndexObject) []byte {
	b := make([]byte, 12)
	b[0] = byte(o.Kind) << 6
	switch o.Kind {
	case wittybluray.ObjectHDMV:
		binary.BigEndian.PutUint16(b[6:], uint16(o.MovieObject))
	case wittybluray.ObjectBDJ:
		b[4] = 3 << 6
		copy(b[6:], o.BDJName)
	}
	return b
}

func buildIndex(first, top wittybluray.IndexObject, titles ...wittybluray.IndexObject) []byte {
	head := make([]byte, 78)
	copy(head, "INDX0200")
	binary.BigEndian.PutUint32(head[8:], 78)
	body := cat(make([]byte, 4), indexEntry(first), indexEntry(top), be16(len(titles)))
	for _, t := range titles {
		body = append(body, indexEntry(t)...)
	}
	return append(head, body...)
}

func buildMobj(objs ...[]wittybluray.Command) []byte {
	b := make([]byte, 40)
	copy(b, "MOBJ0200")
	b = append(b, make([]byte, 8)...)
	b = append(b, be16(len(objs))...)
	for _, cmds := range objs {
		b = append(b, be16(0x8000)...) // it can be resumed
		b = append(b, be16(len(cmds))...)
		for _, c := range cmds {
			b = append(b, cat(be32(c.Op), be32(c.Dst), be32(c.Src))...)
		}
	}
	return b
}

func buildMpls(clip string, in, out uint32, marks ...wittybluray.Mark) []byte {
	head := make([]byte, 58)
	copy(head, "MPLS0200")
	head[45] = 1
	binary.BigEndian.PutUint32(head[8:], 58)
	var pi []byte
	pi = append(pi, clip+"M2TS"...)
	pi = append(pi, 0, 1, 0)
	pi = append(pi, cat(be32(in), be32(out))...)
	pi = append(pi, make([]byte, 8)...)
	pi = append(pi, 0, 0, 0, 0)
	stn := make([]byte, 16)
	stn[4], stn[5] = 1, 2 // a picture and two sound tracks
	stn = append(stn, 9, 1, 0x10, 0x11, 0, 0, 0, 0, 0, 0, 5, 0x1b, 0x61, 0, 0, 0)
	for _, pid := range []uint16{0x1100, 0x1101} {
		stn = append(stn, 9, 1, byte(pid>>8), byte(pid), 0, 0, 0, 0, 0, 0, 5, 0x81, 0x31, 'e', 'n', 'g')
	}
	binary.BigEndian.PutUint16(stn, uint16(len(stn)-2))
	pi = append(pi, stn...)
	list := cat(make([]byte, 6), be16(1), []byte{0, 0}, be16(len(pi)), pi)
	binary.BigEndian.PutUint32(head[12:], uint32(58+len(list)))
	b := append(head, list...)
	b = append(b, make([]byte, 4)...)
	b = append(b, be16(len(marks))...)
	for _, m := range marks {
		e := make([]byte, 14)
		e[1] = byte(m.Type)
		binary.BigEndian.PutUint16(e[2:], uint16(m.Item))
		binary.BigEndian.PutUint32(e[4:], m.Time)
		b = append(b, e...)
	}
	return b
}

func buildClip(packets, start, end uint32) []byte {
	b := make([]byte, 60)
	copy(b, "HDMV0200")
	b[46], b[47] = 1, 1
	binary.BigEndian.PutUint32(b[56:], packets)
	binary.BigEndian.PutUint32(b[8:], 60)
	seq := cat(make([]byte, 5), []byte{1}, make([]byte, 4), []byte{1, 0}, be16(0), be32(0), be32(start), be32(end))
	b = append(b, seq...)
	binary.BigEndian.PutUint32(b[12:], uint32(60+len(seq)))
	return append(b, make([]byte, 6)...)
}

const packetSize = 192

func clearStream(packets int) []byte {
	b := make([]byte, packets*packetSize)
	for i := 0; i < packets; i++ {
		b[i*packetSize+4] = 0x47
	}
	return b
}

// scrambled looks as an AACS-encrypted stream does: units of 32 packets of noise,
// whose first header survives.
func scrambled(units int) []byte {
	const unit = 32 * packetSize
	b := make([]byte, units*unit)
	for i := range b {
		b[i] = byte(i*7 + i>>8)
	}
	for u := 0; u < units; u++ {
		b[u*unit], b[u*unit+4] = 0xc0, 0x47
	}
	return b
}

func hdmv(n int) wittybluray.IndexObject {
	return wittybluray.IndexObject{Kind: wittybluray.ObjectHDMV, MovieObject: n}
}

var (
	play      = func(n uint32) wittybluray.Command { return wittybluray.Command{Op: 0x22800000, Dst: n} }
	jumpTitle = func(n uint32) wittybluray.Command { return wittybluray.Command{Op: 0x21810000, Dst: n} }
)

// discFiles: first play shows an intro and goes to title 1, the feature; the top menu
// plays its own playlist and then the feature.
func discFiles(stream []byte) map[string][]byte {
	packets := uint32(len(stream) / packetSize)
	feature := buildMpls("00001", 45000, 45000*600, wittybluray.Mark{Type: 1, Time: 45000}, wittybluray.Mark{Type: 1, Time: 45000 * 300})
	return map[string][]byte{
		"BDMV/index.bdmv": buildIndex(hdmv(0), hdmv(1), hdmv(2)),
		"BDMV/MovieObject.bdmv": buildMobj(
			[]wittybluray.Command{play(2), jumpTitle(1)},
			[]wittybluray.Command{play(3), jumpTitle(1)},
			[]wittybluray.Command{play(1), jumpTitle(0)},
		),
		"BDMV/PLAYLIST/00001.mpls": feature,
		"BDMV/PLAYLIST/00002.mpls": buildMpls("00002", 45000, 45000*5),
		"BDMV/PLAYLIST/00003.mpls": buildMpls("00002", 45000, 45000*9),
		"BDMV/CLIPINF/00001.clpi":  buildClip(packets, 45000, 45000*600),
		"BDMV/CLIPINF/00002.clpi":  buildClip(32, 45000, 45000*10),
		"BDMV/STREAM/00001.m2ts":   stream,
		"BDMV/STREAM/00002.m2ts":   clearStream(32),
	}
}

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

func clearDisc(t *testing.T) string {
	t.Helper()
	return writeDisc(t, discFiles(clearStream(32*8)))
}

// ---------------------------------------------------------------- a helper

// inProc is the real helper's code without the process and the pipe, with a way to
// lie or die at any call. The built helper is driven further down.
type inProc struct {
	h      helper.Helper
	dead   bool
	closed int
	calls  map[string]int
	// tamper changes an answer; it sees what the real helper said.
	tamper func(method string, blob []byte, err error) ([]byte, error)
	opened func(o *helper.Opened)
}

func (p *inProc) Call(method string, params, out any) ([]byte, error) {
	if p.dead {
		return nil, io.ErrClosedPipe
	}
	if p.calls == nil {
		p.calls = map[string]int{}
	}
	p.calls[method]++
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	res, blob, err := p.h.Handle(method, raw)
	if p.tamper != nil {
		blob, err = p.tamper(method, blob, err)
	}
	if err != nil {
		return nil, err
	}
	if o, ok := res.(helper.Opened); ok && p.opened != nil {
		p.opened(&o)
		res = o
	}
	if res != nil && out != nil {
		b, err := json.Marshal(res)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, out); err != nil {
			return nil, err
		}
	}
	return blob, nil
}

func (p *inProc) Close() { p.closed++ }

type world struct {
	t       *testing.T
	m       *bdsession.Manager
	reports []string
	engine  *engine
	opens   int
}

// newWorld is a manager with its own stream and cache folders. make, when given, is the
// helper of every disc opened; nil leaves the hook unset.
func newWorld(t *testing.T, make func() process) *world {
	t.Helper()
	tools := &ffmpeg.Tools{FFmpeg: "ffmpeg", FFprobe: "ffprobe", Encoders: map[string]bool{}}
	dir := t.TempDir()
	log := slog.New(slog.DiscardHandler)
	w := &world{t: t, m: bdsession.NewManager(hls.NewManager(tools, filepath.Join(dir, "hls"), 2, log), tools, dir, "/api", log)}
	old := bdsession.RemoteEngine
	t.Cleanup(func() { bdsession.RemoteEngine = old })
	bdsession.RemoteEngine = nil
	if make != nil {
		bdsession.RemoteEngine = func(path string, o wittybluray.NavOptions, l *slog.Logger) *bdsession.Remote {
			w.opens++
			r := open(make(), path, o, l, func(reason string) { w.reports = append(w.reports, reason) })
			if r != nil {
				w.engine = r.Disc.(*engine)
			}
			return r
		}
	}
	return w
}

func (w *world) open(path string) *bdsession.Session {
	w.t.Helper()
	s, err := w.m.Open(1, path, bdsession.Prefs{AudioLanguage: "en"})
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(s.Close)
	return s
}

// lists are the input lists FFmpeg would be given, which say what it would read.
func (w *world) lists() map[string]string {
	w.t.Helper()
	out := map[string]string{}
	root := filepath.Join(w.m.CacheDir, "lists")
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				w.t.Fatal(rerr)
			}
			rel, _ := filepath.Rel(root, p)
			// A list can name another list, in this world's own cache folder.
			out[rel] = strings.ReplaceAll(string(b), filepath.ToSlash(w.m.CacheDir), "CACHE")
		}
		return nil
	})
	return out
}

// view is a presentation as JSON, without what differs between any two sessions.
func view(t *testing.T, s *bdsession.Session, p *bdsession.Presentation) string {
	t.Helper()
	if p == nil {
		return "nil"
	}
	c := *p
	c.TimerSec = 0
	b, err := json.Marshal(&c)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), s.ID, "SESSION")
}

// walk is a viewer going around a disc, and what they see.
func walk(t *testing.T, s *bdsession.Session) []string {
	t.Helper()
	ctx := context.Background()
	var out []string
	see := func(p *bdsession.Presentation) { out = append(out, view(t, s, p)) }
	see(s.Start(ctx))
	for _, a := range []bdsession.Action{
		{Type: "tick"},
		{Type: "ended"}, // the intro ends: the feature
		{Type: "tick", PositionSec: 12},
		{Type: "audio", Stream: 1, PositionSec: 12},
		{Type: "subtitle", Stream: 0, On: true},
		{Type: "angle", Stream: 1, PositionSec: 12},
		{Type: "menu", PositionSec: 30}, // the top menu, and back
		{Type: "resume"},
		{Type: "title", TTN: 3},
		{Type: "ended"},
		{Type: "title", TTN: 1, Chapter: 2},
		{Type: "key", Key: "next"},
		{Type: "title", TTN: 99},
		{Type: "ended"},
		{Type: "key", Key: "menu"},
		{Type: "ended"},
		{Type: "ended"},
	} {
		see(s.Do(ctx, a))
	}
	see(s.OpenMenu(ctx, "root", 0))
	see(s.PlayTitle(ctx, 1, 2))
	return out
}

func same(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d presentations, want %d", what, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s, step %d:\n got %s\nwant %s", what, i, got[i], want[i])
		}
	}
}

func sameLists(t *testing.T, got, want map[string]string) {
	t.Helper()
	if len(want) == 0 {
		t.Error("nothing would have been played")
	}
	if len(got) != len(want) {
		t.Errorf("%d input lists, want %d", len(got), len(want))
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("input list %s:\n got %q\nwant %q", name, got[name], w)
		}
	}
}

func inHelper() process { return &inProc{} }

// ---------------------------------------------------------------- the tests

// Through the helper, a viewer sees just what the built-in engine shows them, and
// FFmpeg is pointed at just the same bytes.
func TestSameThroughTheHelper(t *testing.T) {
	path := clearDisc(t)
	built := newWorld(t, nil)
	want := walk(t, built.open(path))
	wantLists := built.lists()

	w := newWorld(t, inHelper)
	s := w.open(path)
	same(t, "through the helper", walk(t, s), want)
	sameLists(t, w.lists(), wantLists)
	if w.opens != 1 || w.engine == nil || w.engine.h == nil {
		t.Fatalf("the helper isn't in use: %d opens, %+v", w.opens, w.engine)
	}
	if len(w.reports) != 0 {
		t.Errorf("failures reported: %v", w.reports)
	}
	if w.engine.h.(*inProc).calls["playlist"] == 0 {
		t.Error("the helper was never asked for a playlist")
	}
}

// The built-in copy in use: the hook has nothing to offer, and nothing changes.
func TestBuiltinPathUnchanged(t *testing.T) {
	path := clearDisc(t)
	none := newWorld(t, nil)
	want := walk(t, none.open(path))
	wantLists := none.lists()

	w := newWorld(t, nil)
	calls := 0
	bdsession.RemoteEngine = func(string, wittybluray.NavOptions, *slog.Logger) *bdsession.Remote { calls++; return nil }
	same(t, "with the hook set", walk(t, w.open(path)), want)
	sameLists(t, w.lists(), wantLists)
	if calls != 1 {
		t.Errorf("asked %d times", calls)
	}
}

// A playlist is one pointer however it is reached, which the session relies on, and
// the registers are one object that follows the navigator.
func TestPointersHold(t *testing.T) {
	r := open(&inProc{}, clearDisc(t), wittybluray.NavOptions{}, nil, func(string) {})
	if r == nil {
		t.Fatal("not opened")
	}
	defer r.Disc.Close()
	st := r.Nav.Start()
	if st.Kind != wittybluray.StepPlay || st.Playlist == nil || st.Playlist != r.Disc.Playlist(2) {
		t.Fatalf("%+v", st)
	}
	if r.Disc.MainPlaylist() != r.Disc.Playlist(1) || r.Disc.MainPlaylist() == nil {
		t.Error("the main playlist isn't the same playlist")
	}
	if r.Nav.LoopsAtEnd(st) || r.Nav.LoopsAtEnd(nil) {
		t.Error("first play moves on by itself, and doesn't loop")
	}
	regs := r.Nav.Registers()
	r.Nav.SetAudio(2)
	if regs.PSR[wittybluray.PSRAudio] != 2 {
		t.Error("registers held across a call didn't follow it")
	}
	if r.Nav.Registers() != regs {
		t.Error("the registers became another object")
	}
	// Telling it what it already knows costs no call.
	p := r.Disc.(*engine).h.(*inProc)
	r.Nav.SetMenuState(3, 4)
	r.Nav.SetMenuState(3, 4)
	r.Nav.SetMenuState(3, 4)
	if p.calls["setmenustate"] != 1 {
		t.Errorf("%d calls", p.calls["setmenustate"])
	}
}

// A timer the disc starts again is a new timer even with the same values, because
// that is what restarts the player's clock.
func TestTimerIdentity(t *testing.T) {
	seq, secs := uint64(0), uint32(9)
	p := &inProc{tamper: func(method string, blob []byte, err error) ([]byte, error) {
		var a helper.Answer
		if err := json.Unmarshal(blob, &a); err != nil {
			return blob, nil // not a navigator's answer
		}
		a.TimerSeq, a.Timer = seq, &wittybluray.NavTimer{Seconds: secs}
		return json.Marshal(&a)
	}}
	r := open(p, clearDisc(t), wittybluray.NavOptions{}, nil, func(string) {})
	defer r.Disc.Close()
	seq = 1
	r.Nav.SetChapter(1)
	first := r.Nav.Timer()
	if first == nil || first.Seconds != 9 {
		t.Fatalf("%+v", first)
	}
	r.Nav.SetChapter(2)
	if r.Nav.Timer() != first {
		t.Error("the same timer became a new one, which would restart the clock")
	}
	seq = 2
	r.Nav.SetChapter(3)
	if r.Nav.Timer() == first {
		t.Error("a timer started again wasn't noticed")
	}
}

// "Stills off" in a step's effects is a pointer to false, and has to arrive as one.
func TestStepEffectsSurvive(t *testing.T) {
	off := false
	p := &inProc{tamper: func(method string, blob []byte, err error) ([]byte, error) {
		if method != "pressbutton" {
			return blob, err
		}
		var a helper.Answer
		_ = json.Unmarshal(blob, &a)
		a.Step = &wittybluray.Step{Kind: wittybluray.StepStay, Object: -1, Effects: &wittybluray.Effects{Still: &off, PopupOff: true,
			ButtonPage: &wittybluray.ButtonPage{SetPage: true}, EnableButton: []uint32{3}}}
		return json.Marshal(&a)
	}}
	r := open(p, clearDisc(t), wittybluray.NavOptions{}, nil, func(string) {})
	defer r.Disc.Close()
	st := r.Nav.PressButton([]wittybluray.Command{{Op: 1, Dst: 2, Src: 3}})
	fx := st.Effects
	if fx == nil || fx.Still == nil || *fx.Still || !fx.PopupOff || fx.ButtonPage == nil || !fx.ButtonPage.SetPage || len(fx.EnableButton) != 1 {
		t.Fatalf("%+v", fx)
	}
}

// A helper dying mid-session doesn't take the session with it: the built-in copy goes
// to first play and the disc goes on from there.
func TestCrashMidSession(t *testing.T) {
	ctx := context.Background()
	path := clearDisc(t)
	first := newWorld(t, nil).open(path).Start(ctx)

	for _, a := range []bdsession.Action{
		{Type: "ended"},
		{Type: "menu", PositionSec: 10},
		{Type: "resume"},
		{Type: "title", TTN: 3},
		{Type: "key", Key: "menu"},
		{Type: "key", Key: "next"},
	} {
		w := newWorld(t, inHelper)
		s := w.open(path)
		s.Start(ctx)
		cur := s.Do(ctx, bdsession.Action{Type: "ended"})
		if cur.Kind != "title" {
			t.Fatalf("not at the feature: %+v", cur)
		}
		p := w.engine.h.(*inProc)
		p.dead = true
		got := s.Do(ctx, a)
		if got == nil || got.Kind == "stop" {
			t.Fatalf("%+v: the session stopped: %+v", a, got)
		}
		if got.Reason != restarted {
			t.Errorf("%+v: the viewer isn't told: %q", a, got.Reason)
		}
		if got.Kind != first.Kind || got.Media == nil || got.Media.URL != first.Media.URL {
			t.Errorf("%+v: not first play:\n got %+v\nwant %+v", a, got.Media, first.Media)
		}
		if w.engine.h != nil || p.closed != 1 {
			t.Errorf("%+v: the helper is still in use (%d closes)", a, p.closed)
		}
		// The crash was counted where it was seen, by the call itself.
		if len(w.reports) != 0 {
			t.Errorf("%+v: counted twice: %v", a, w.reports)
		}
		// The disc plays on from there, on the built-in copy.
		if p := s.Do(ctx, bdsession.Action{Type: "ended"}); p.Kind != "title" || p.Title == nil || p.Title.TTN != 1 {
			t.Errorf("%+v: after first play: %+v", a, p)
		}
		p.calls = nil
		if s.Do(ctx, bdsession.Action{Type: "audio", Stream: 1}); p.calls != nil {
			t.Error("the dead helper was asked again")
		}
		s.Close()
	}
}

// A crash found by something that isn't a step (a change of sound track here) doesn't
// interrupt what is playing. The next step is first play.
func TestCrashFoundBetweenSteps(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t, inHelper)
	s := w.open(clearDisc(t))
	s.Start(ctx)
	film := s.Do(ctx, bdsession.Action{Type: "ended"})
	w.engine.h.(*inProc).dead = true
	got := s.Do(ctx, bdsession.Action{Type: "audio", Stream: 1, PositionSec: 40})
	if got.Kind != "title" || got.Title == nil || got.Title.TTN != film.Title.TTN || got.Title.AudioStream != 1 {
		t.Fatalf("the film was interrupted: %+v", got)
	}
	if got := s.Do(ctx, bdsession.Action{Type: "ended"}); got.Reason != restarted {
		t.Errorf("after: %+v", got)
	}
}

// With the disc gone too, there is nothing to play, and nothing hangs or panics.
func TestCrashWithTheDiscGone(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t, inHelper)
	path := clearDisc(t)
	s := w.open(path)
	s.Start(ctx)
	s.Do(ctx, bdsession.Action{Type: "ended"})
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	w.engine.h.(*inProc).dead = true
	if got := s.Do(ctx, bdsession.Action{Type: "ended"}); got.Kind != "stop" {
		t.Fatalf("%+v", got)
	}
	for _, a := range []bdsession.Action{{Type: "audio"}, {Type: "subtitle"}, {Type: "angle"}, {Type: "menu"}, {Type: "resume"}, {Type: "title", TTN: 1}, {Type: "ended"}, {Type: "key", Key: "next"}, {Type: "tick"}} {
		s.Do(ctx, a)
	}
	s.PlayTitle(ctx, 1, 0)
	if _, err := s.Subtitles(1, 0, 0, 10); err == nil {
		t.Error("subtitles from a disc that is gone")
	}
	if _, ok := s.SoundWAV(1); ok {
		t.Error("a sound from a disc that is gone")
	}
}

func gobbed(t *testing.T, v any) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// Extents that aren't the disc, or can't be right, never reach FFmpeg. The session
// gets the real ones from the built-in copy, and the version is marked down.
func TestBadExtentsAreRefused(t *testing.T) {
	ctx := context.Background()
	path := clearDisc(t)
	stream := filepath.Join(path, "BDMV", "STREAM", "00001.m2ts")
	outside := filepath.Join(t.TempDir(), "secret.m2ts")
	if err := os.WriteFile(outside, clearStream(32), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]wittybluray.FileExtent{
		"another file":       {{Path: outside, Length: 100}},
		"climbing out":       {{Path: filepath.Join(path, "..", filepath.Base(filepath.Dir(outside)), "secret.m2ts"), Length: 100}},
		"climbing out plain": {{Path: path + string(filepath.Separator) + ".." + string(filepath.Separator) + "x", Length: 100}},
		"relative":           {{Path: filepath.Join("BDMV", "STREAM", "00001.m2ts"), Length: 100}},
		"the folder itself":  {{Path: path, Length: 100}},
		"a folder on it":     {{Path: filepath.Join(path, "BDMV"), Length: 100}},
		"negative offset":    {{Path: stream, Offset: -192, Length: 192}},
		"negative length":    {{Path: stream, Offset: 192, Length: -192}},
		"overflowing":        {{Path: stream, Offset: math.MaxInt64 - 5, Length: 100}},
		"no such file":       {{Path: filepath.Join(path, "BDMV", "STREAM", "99999.m2ts"), Length: 100}},
		"one bad among good": {{Path: stream, Length: 192}, {Path: outside, Length: 192}},
		"too many":           make([]wittybluray.FileExtent, maxExtents+1),
	}
	for i := range cases["too many"] {
		cases["too many"][i] = wittybluray.FileExtent{Path: stream, Length: 192}
	}
	built := newWorld(t, nil)
	want := built.open(path)
	want.Start(ctx)
	want.Do(ctx, bdsession.Action{Type: "ended"})
	wantLists := built.lists()

	for name, ext := range cases {
		w := newWorld(t, func() process {
			return &inProc{tamper: func(method string, blob []byte, err error) ([]byte, error) {
				if method == "extents" {
					return gobbed(t, ext), nil
				}
				return blob, err
			}}
		})
		s := w.open(path)
		// The first thing played asks for extents: refused, and the built-in copy gives
		// the true ones. The next step is first play again, and then the feature.
		if first := s.Start(ctx); first.Kind == "stop" {
			t.Errorf("%s: the session stopped: %+v", name, first)
		}
		if w.engine.h != nil {
			t.Errorf("%s: the helper is still in use", name)
		}
		if len(w.reports) != 1 || !strings.Contains(w.reports[0], "bad answer") {
			t.Errorf("%s: reported %v", name, w.reports)
		}
		if got := s.Do(ctx, bdsession.Action{Type: "ended"}); got.Reason != restarted {
			t.Errorf("%s: %+v", name, got)
		}
		if got := s.Do(ctx, bdsession.Action{Type: "ended"}); got.Kind != "title" {
			t.Errorf("%s: %+v", name, got)
		}
		sameLists(t, w.lists(), wantLists)
		s.Close()
	}
}

// The checks by themselves, for an image and a folder.
func TestExtentChecks(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "disc.iso")
	if err := os.WriteFile(img, make([]byte, 1000), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other.iso")
	if err := os.WriteFile(other, make([]byte, 1000), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := newEngine(&inProc{}, img, wittybluray.NavOptions{}, slog.New(slog.DiscardHandler), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	// An untidy spelling of the image is the image, and comes back as the server's own.
	sep := string(filepath.Separator)
	got, err := e.checkExtents([]wittybluray.FileExtent{{Path: dir + sep + "x" + sep + ".." + sep + "disc.iso", Offset: 100, Length: 200}})
	if err != nil || got[0].Path != img || got[0].Offset != 100 || got[0].Length != 200 {
		t.Errorf("%+v %v", got, err)
	}
	if _, err := e.checkExtents([]wittybluray.FileExtent{{Path: other, Length: 1}}); err == nil {
		t.Error("another image was accepted")
	}
	// An image cut short has nothing past its end.
	got, err = e.checkExtents([]wittybluray.FileExtent{{Path: img, Offset: 900, Length: 500}, {Path: img, Offset: 5000, Length: 500}})
	if err != nil || got[0].Length != 100 || got[1].Offset != 1000 || got[1].Length != 0 {
		t.Errorf("past the end: %+v %v", got, err)
	}

	// A folder, opened by way of its BDMV: the disc is the folder above.
	disc := filepath.Join(dir, "disc")
	stream := filepath.Join(disc, "BDMV", "STREAM", "00001.m2ts")
	if err := os.MkdirAll(filepath.Dir(stream), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stream, make([]byte, 1000), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err = newEngine(&inProc{}, filepath.Join(disc, "BDMV"), wittybluray.NavOptions{}, slog.New(slog.DiscardHandler), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := e.checkExtents([]wittybluray.FileExtent{{Path: stream, Length: 10}}); err != nil || got[0].Path != stream {
		t.Errorf("%+v %v", got, err)
	}
	for _, bad := range []string{other, disc, filepath.Join(disc, "..", "other.iso"), stream + ":hidden", filepath.Join(disc, "..", "disc2", "x")} {
		if _, err := e.checkExtents([]wittybluray.FileExtent{{Path: bad, Length: 10}}); err == nil {
			t.Errorf("%s was accepted", bad)
		}
	}
	// A link inside the folder that leads out of it is outside it. Making one needs a
	// privilege Windows doesn't always give.
	link := filepath.Join(disc, "BDMV", "STREAM", "00002.m2ts")
	if err := os.Symlink(other, link); err == nil {
		if _, err := e.checkExtents([]wittybluray.FileExtent{{Path: link, Length: 10}}); err == nil {
			t.Error("a link out of the disc was accepted")
		}
	}
}

// Encryption is the server's own finding. A helper saying otherwise changes nothing,
// and isn't even asked.
func TestEncryptedFlagIsNotBelieved(t *testing.T) {
	ctx := context.Background()
	path := writeDisc(t, discFiles(scrambled(8)))
	w := newWorld(t, func() process {
		return &inProc{opened: func(o *helper.Opened) { o.Encrypted = false }}
	})
	s := w.open(path)
	for name, p := range map[string]*bdsession.Presentation{
		"start": s.Start(ctx),
		"title": s.PlayTitle(ctx, 1, 0),
		"menu":  s.OpenMenu(ctx, "root", 0),
	} {
		if p.Kind != "stop" || !p.Scrambled {
			t.Errorf("%s played an encrypted disc: %+v", name, p)
		}
	}
	if w.opens != 0 {
		t.Errorf("a helper was started for an encrypted disc")
	}
	if got := w.lists(); len(got) != 0 {
		t.Errorf("FFmpeg would have read it: %v", got)
	}
}

// A helper can only add to the refusal: one that finds a disc encrypted is listened to,
// and the disc goes to the built-in copy, which refuses it.
func TestHelperThatFindsEncryptionIsHeard(t *testing.T) {
	ctx := context.Background()
	var p *inProc
	w := newWorld(t, func() process {
		p = &inProc{opened: func(o *helper.Opened) { o.Encrypted = true }}
		return p
	})
	s := w.open(clearDisc(t))
	if got := s.Start(ctx); got.Kind != "stop" || !got.Scrambled {
		t.Fatalf("%+v", got)
	}
	if p.closed != 1 {
		t.Errorf("the helper was left running: %d", p.closed)
	}
}

// Opening: a helper that can't, for whatever reason, leaves the disc to the built-in
// copy.
func TestOpeningRefusedUsesTheBuiltinCopy(t *testing.T) {
	ctx := context.Background()
	path := clearDisc(t)
	for name, fail := range map[string]error{
		"denied":      &wittymodule.CallError{Code: "denied", Message: "access is denied"},
		"can't read":  &wittymodule.CallError{Message: "unreadable"},
		"crashed":     io.ErrUnexpectedEOF,
		"won't start": errors.New("could not start"),
	} {
		var p *inProc
		w := newWorld(t, func() process {
			p = &inProc{tamper: func(method string, blob []byte, err error) ([]byte, error) { return nil, fail }}
			return p
		})
		s := w.open(path)
		if got := s.Start(ctx); got.Kind == "stop" {
			t.Errorf("%s: %+v", name, got)
		}
		if p.closed != 1 || p.calls["open"] != 1 || len(p.calls) != 1 {
			t.Errorf("%s: the helper went on being used: %d closes, %v", name, p.closed, p.calls)
		}
		if w.engine != nil {
			t.Errorf("%s: an engine came of it", name)
		}
		s.Close()
	}
}

// A disc with Java programs on a server that runs them stays with the built-in copy.
// With that turned off, the helper plays it from its playlists like any other.
func TestJavaStaysBuiltin(t *testing.T) {
	ctx := context.Background()
	files := discFiles(clearStream(32 * 8))
	files["BDMV/index.bdmv"] = buildIndex(wittybluray.IndexObject{Kind: wittybluray.ObjectBDJ, BDJName: "00000"}, hdmv(1), hdmv(2))
	path := writeDisc(t, files)

	w := newWorld(t, inHelper)
	w.m.BDJ = func() (bool, string) { return true, filepath.Join(t.TempDir(), "no-java") }
	s := w.open(path)
	if w.opens != 0 {
		t.Error("a helper was given a disc whose Java is to run")
	}
	if got := s.Start(ctx); got.Kind == "stop" {
		t.Errorf("with Java on: %+v", got)
	}
	s.Close()

	w = newWorld(t, inHelper)
	enabled := false
	w.m.BDJ = func() (bool, string) { return enabled, filepath.Join(t.TempDir(), "no-java") }
	s = w.open(path)
	if w.opens != 1 || w.engine == nil {
		t.Fatal("the helper wasn't used with Java off")
	}
	enabled = true // turned on with this disc open: its session doesn't change hands
	got := s.Start(ctx)
	if got.Kind != "title" || !strings.Contains(got.Reason, "BD-J") {
		t.Errorf("%+v", got)
	}
	if w.engine.h == nil {
		t.Error("the helper was dropped")
	}
}

// What the session would index by, without looking, is checked as it arrives. Each of
// these is otherwise a panic, or a name that isn't one.
func TestBadStructuresAreRefused(t *testing.T) {
	ctx := context.Background()
	path := clearDisc(t)
	d, err := wittybluray.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	spoiled := func(spoil func(pl *wittybluray.Playlist)) []byte {
		// Decoding its own bytes makes a copy to spoil.
		var pl wittybluray.Playlist
		if err := gob.NewDecoder(bytes.NewReader(gobbed(t, d.Playlist(2)))).Decode(&pl); err != nil {
			t.Fatal(err)
		}
		spoil(&pl)
		return gobbed(t, &pl)
	}
	navAnswer := func(blob []byte, spoil func(a *helper.Answer)) []byte {
		var a helper.Answer
		if err := json.Unmarshal(blob, &a); err != nil {
			t.Fatal(err)
		}
		spoil(&a)
		b, err := json.Marshal(&a)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for name, c := range map[string]struct {
		method string
		blob   func(old []byte) []byte
	}{
		"garbage for a playlist": {"playlist", func([]byte) []byte { return []byte("this is not gob") }},
		"another playlist":       {"playlist", func([]byte) []byte { return gobbed(t, d.Playlist(1)) }},
		"a sub-path below zero": {"playlist", func([]byte) []byte {
			return spoiled(func(pl *wittybluray.Playlist) { pl.Items[0].Audio[0].SubPath = -1 })
		}},
		"a clip that is a path": {"playlist", func([]byte) []byte {
			return spoiled(func(pl *wittybluray.Playlist) { pl.Items[0].Clip = `..\..\x` })
		}},
		"another clip":      {"clip", func([]byte) []byte { return gobbed(t, d.Clip("00001")) }},
		"garbage for steps": {"start", func([]byte) []byte { return []byte("{") }},
		"a step before the first item": {"start", func(old []byte) []byte {
			return navAnswer(old, func(a *helper.Answer) { a.Step.Item = -1 })
		}},
		"a step of an unknown kind": {"start", func(old []byte) []byte {
			return navAnswer(old, func(a *helper.Answer) { a.Step.Kind = "reboot" })
		}},
		"a step to play nothing": {"start", func(old []byte) []byte {
			return navAnswer(old, func(a *helper.Answer) { a.Playlist = nil })
		}},
		"a playlist it doesn't have": {"start", func(old []byte) []byte {
			return navAnswer(old, func(a *helper.Answer) { n := 777; a.Playlist = &n })
		}},
		"a menu clip that is a path": {"start", func(old []byte) []byte {
			return navAnswer(old, func(a *helper.Answer) { a.Step.IGClip = "../x" })
		}},
		"no step":          {"start", func(old []byte) []byte { return navAnswer(old, func(a *helper.Answer) { a.Step = nil }) }},
		"nothing for play": {"start", func([]byte) []byte { return nil }},
		"too many effects": {"start", func(old []byte) []byte {
			return navAnswer(old, func(a *helper.Answer) {
				a.Step.Effects = &wittybluray.Effects{EnableButton: make([]uint32, maxEffectList+1)}
			})
		}},
	} {
		w := newWorld(t, func() process {
			return &inProc{tamper: func(method string, blob []byte, err error) ([]byte, error) {
				if method == c.method {
					return c.blob(blob), nil
				}
				return blob, err
			}}
		})
		s := w.open(path)
		got := s.Start(ctx)
		if got == nil || got.Kind == "stop" {
			t.Errorf("%s: %+v", name, got)
		}
		if w.engine.h != nil || len(w.reports) != 1 {
			t.Errorf("%s: still in use: reports %v", name, w.reports)
		}
		s.Close()
	}
}

// Subtitle pictures are encoded on the server, so they are checked first.
func TestBadSubtitlePictures(t *testing.T) {
	good := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	if err := checkSubtitles(&wittybluray.SubtitleWindow{Subtitles: []wittybluray.Subtitle{{Image: good}}}); err != nil {
		t.Fatal(err)
	}
	for name, m := range map[string]*image.NRGBA{
		"none":        nil,
		"short":       {Pix: good.Pix[:3], Stride: 16, Rect: good.Rect},
		"wide stride": {Pix: good.Pix, Stride: 1 << 20, Rect: good.Rect},
		"backwards":   {Pix: good.Pix, Stride: 16, Rect: image.Rectangle{Min: image.Pt(4, 2)}},
		"huge":        {Pix: good.Pix, Stride: 1 << 24, Rect: image.Rect(0, 0, 1<<22, 2)},
		"elsewhere":   {Pix: good.Pix, Stride: 16, Rect: image.Rect(1<<40, 0, 1<<40+4, 2)},
	} {
		if err := checkSubtitles(&wittybluray.SubtitleWindow{Subtitles: []wittybluray.Subtitle{{Image: m}}}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// A helper in good order that can't do one thing says so, and the session carries on.
func TestARefusalIsJustARefusal(t *testing.T) {
	ctx := context.Background()
	var refusal error = &wittymodule.CallError{Message: "the clip can't be read"}
	w := newWorld(t, func() process {
		return &inProc{tamper: func(method string, blob []byte, err error) ([]byte, error) {
			if method == "subtitles" {
				return nil, refusal
			}
			return blob, err
		}}
	})
	s := w.open(clearDisc(t))
	s.Start(ctx)
	if p := s.PlayTitle(ctx, 1, 0); p.Kind != "title" {
		t.Fatalf("%+v", p)
	}
	if _, err := w.engine.SubtitlesIn("00001", 0x1200, -1, 0, 100); err == nil || !strings.Contains(err.Error(), "can't be read") {
		t.Errorf("%v", err)
	}
	if w.engine.h == nil || len(w.reports) != 0 {
		t.Errorf("a refusal ended the helper's use: %v", w.reports)
	}
	// "denied" may be its limits, so the built-in copy takes over, and it doesn't count.
	refusal = &wittymodule.CallError{Code: "denied", Message: "access is denied"}
	if win, err := w.engine.SubtitlesIn("00001", 0x1200, -1, 0, 100); err != nil || win == nil {
		t.Errorf("%v %v", win, err)
	}
	if w.engine.h != nil || len(w.reports) != 0 {
		t.Errorf("still in use, or counted: %v", w.reports)
	}
}

// Everything a session can ask of a disc gets the built-in engine's answer.
func TestDiscAnswersMatch(t *testing.T) {
	path := clearDisc(t)
	d, err := wittybluray.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	r := open(&inProc{}, path, wittybluray.NavOptions{}, nil, func(string) { t.Error("a failure") })
	defer r.Disc.Close()
	equal := func(what string, a, b any) {
		t.Helper()
		x, _ := json.Marshal(a)
		y, _ := json.Marshal(b)
		if !bytes.Equal(x, y) {
			t.Errorf("%s:\n got %s\nwant %s", what, x, y)
		}
	}
	var nums []int
	for n := range d.Playlists {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	for _, n := range append(nums, 12345) {
		equal("playlist", r.Disc.Playlist(n), d.Playlist(n))
	}
	equal("main", r.Disc.MainPlaylist(), d.MainPlaylist())
	if a, b := r.Disc.PlaylistDuration(r.Disc.Playlist(1)), d.PlaylistDuration(d.Playlist(1)); a != b || a == 0 {
		t.Errorf("duration %v, want %v", a, b)
	}
	for _, name := range []string{"00001", "00002", "99999"} {
		equal("clip", r.Disc.Clip(name), d.Clip(name))
		a, aerr := r.Disc.ClipExtents(name)
		b, berr := d.ClipExtents(name)
		equal("extents", a, b)
		if (aerr == nil) != (berr == nil) {
			t.Errorf("extents of %s: %v, want %v", name, aerr, berr)
		}
	}
	if a, b := r.Disc.Sounds(), d.Sounds(); (a == nil) != (b == nil) {
		t.Errorf("sounds: %v, want %v", a, b)
	}
	if _, err := r.Disc.BDJObject("00000"); err == nil {
		t.Error("a Java object the disc doesn't have")
	}
	if _, err := r.Disc.LoadMenu("00002", 0x1400, 0); err == nil {
		t.Error("a menu the disc doesn't have")
	}
	if a, err := r.Disc.TextSubtitles("00002", 0x1800, 0); err != nil || len(a) != 0 {
		t.Errorf("text subtitles: %v %v", a, err)
	}
	if w, err := r.Disc.SubtitlesIn("00002", 0x1200, -1, 0, 45000); err != nil || w == nil || len(w.Subtitles) != 0 {
		t.Errorf("subtitles: %v %v", w, err)
	}
}
