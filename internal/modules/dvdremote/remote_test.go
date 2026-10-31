package dvdremote

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittydvd/wire"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymodule"
)

var errCrash = errors.New("the helper crashed")

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// fake is a helper in this process: the real engine behind the real wire form, with a
// hook to crash or lie. It is what a fork could do, without building a program for each.
type fake struct {
	h      wire.Helper
	calls  []string
	closed bool
	// hook sees each call, and may answer it itself.
	hook func(method string, params json.RawMessage) (blob []byte, err error, handled bool)
}

func (f *fake) Call(method string, params, out any) ([]byte, error) {
	f.calls = append(f.calls, method)
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if f.hook != nil {
		if b, err, handled := f.hook(method, raw); handled {
			return b, err
		}
	}
	_, b, err := f.h.Handle(method, raw)
	return b, err
}

func (f *fake) Close() {
	f.closed = true
	f.h.Close()
}

// crashOn makes the helper die at a call and for good after it.
func crashOn(methods ...string) func(string, json.RawMessage) ([]byte, error, bool) {
	dead := false
	return func(method string, _ json.RawMessage) ([]byte, error, bool) {
		if dead || slices.Contains(methods, method) {
			dead = true
			return nil, errCrash, true
		}
		return nil, nil, false
	}
}

type reports struct{ got []string }

func (r *reports) add(reason string) { r.got = append(r.got, reason) }

func openFake(t *testing.T, f *fake, path string) (*dvdsession.Engine, *reports) {
	t.Helper()
	r := &reports{}
	e, err := open(f, r.add, quiet, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e, r
}

func isRemote(e *dvdsession.Engine) bool {
	_, ok := e.DiscEngine.(*remote)
	return ok
}

var titleSpace = wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: 1}

func sessions(t *testing.T) *dvdsession.Manager {
	t.Helper()
	m := dvdsession.NewManager(hls.NewManager(&ffmpeg.Tools{}, t.TempDir(), 1, quiet), &ffmpeg.Tools{}, t.TempDir(), "/api", quiet)
	return m
}

// playAs opens a session whose engine is the given one.
func playAs(t *testing.T, path string, f *fake, r *reports) (*dvdsession.Manager, *dvdsession.Session) {
	t.Helper()
	dvdsession.SetEngine(func(p string) (*dvdsession.Engine, error) { return open(f, r.add, quiet, p) })
	t.Cleanup(func() { dvdsession.SetEngine(dvdsession.OpenBuiltin) })
	m := sessions(t)
	s, err := m.Open(1, path, dvdsession.Prefs{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.CloseSession(s.ID) })
	return m, s
}

// shape is what must be the same however a disc is played: URLs hold the session's id.
func shape(p *dvdsession.Presentation) string {
	var c dvdsession.Presentation = *p
	c.Media, c.Menu, c.Title = nil, nil, nil
	type view struct {
		P        dvdsession.Presentation
		Type     string
		Loop     bool
		Dur      float64
		Still    float64
		Buttons  []dvdsession.ButtonView
		Selected int
		Width    int
		Title    *dvdsession.TitleView
	}
	v := view{P: c}
	if p.Media != nil {
		v.Type, v.Loop, v.Dur, v.Still = p.Media.Type, p.Media.Loop, p.Media.DurationSec, p.Media.StillSec
	}
	if p.Menu != nil {
		v.Buttons, v.Selected, v.Width = p.Menu.Buttons, p.Menu.Selected, p.Menu.Width
	}
	if p.Title != nil {
		t := *p.Title
		t.SubtitlesURL = ""
		v.Title = &t
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// The same disc through the session's own code, with the built-in copy and the helper:
// every screen the same.
func TestHelperPlaysLikeTheBuiltinCopy(t *testing.T) {
	path := makeDisc(t)
	script := []dvdsession.Action{{Type: "select", Button: 1}, {Type: "key", Key: "enter"}, {Type: "subtitle", Stream: 0, On: true}, {Type: "audio", Stream: 0, PositionSec: 5},
		{Type: "ended"}, {Type: "title", TTN: 1, Chapter: 1}, {Type: "press", Button: 1}, {Type: "key", Key: "menu"}, {Type: "resume"}, {Type: "chapter", Chapter: 1}}

	play := func(s *dvdsession.Session) []string {
		ctx := context.Background()
		out := []string{shape(s.Start(ctx))}
		for _, a := range script {
			out = append(out, shape(s.Do(ctx, a)))
		}
		return out
	}
	bm := sessions(t)
	bs, err := bm.Open(1, path, dvdsession.Prefs{})
	if err != nil {
		t.Fatal(err)
	}
	defer bm.CloseSession(bs.ID)
	want := play(bs)

	f := &fake{}
	_, s := playAs(t, path, f, &reports{})
	got := play(s)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step %d:\n built-in %s\n helper   %s", i, want[i], got[i])
		}
	}
	if !strings.Contains(want[0], `"Kind":""`) && !strings.Contains(want[0], `"kind":"menu"`) {
		t.Errorf("the disc should start on its button: %s", want[0])
	}
	if !slices.Contains(f.calls, wire.MethodPress) || !slices.Contains(f.calls, wire.MethodClone) || f.closed {
		t.Errorf("the helper wasn't what played it: %v closed=%v", f.calls, f.closed)
	}
}

// With no helper, nothing about a session changes. The default goes straight to the
// built-in copy, without any of this package in between.
func TestBuiltinIsTheDefault(t *testing.T) {
	path := makeDisc(t)
	e, err := dvdsession.OpenBuiltin(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if isRemote(e) {
		t.Fatal("the built-in copy came through the helper code")
	}
	m := sessions(t)
	s, err := m.Open(1, path, dvdsession.Prefs{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.CloseSession(s.ID)
	p := s.Start(context.Background())
	if p.Kind != "menu" || p.Menu == nil || len(p.Menu.Buttons) != 1 {
		t.Fatalf("%+v", p)
	}
	if n := s.Do(context.Background(), dvdsession.Action{Type: "press", Button: 1}); n.Kind != "title" {
		t.Fatalf("%+v", n)
	}
}

// A helper dying as a button is pressed: the viewer is at the disc's first-play, with
// the built-in copy, and the session goes on.
func TestHelperCrashAtAButton(t *testing.T) {
	f := &fake{}
	path := makeDisc(t)
	r := &reports{}
	_, s := playAs(t, path, f, r)
	ctx := context.Background()
	first := s.Start(ctx)
	if first.Kind != "menu" {
		t.Fatalf("%+v", first)
	}
	f.hook = crashOn(wire.MethodPress)
	p := s.Do(ctx, dvdsession.Action{Type: "press", Button: 1})
	if p.Kind != "menu" || p.Menu == nil || len(p.Menu.Buttons) != 1 {
		t.Fatalf("should be back at the start: %+v", p)
	}
	if p.Serial <= first.Serial {
		t.Fatalf("a serial the player has seen means the same screen: %d then %d", first.Serial, p.Serial)
	}
	if !f.closed {
		t.Fatal("the helper was left running")
	}
	if len(r.got) != 0 {
		t.Fatalf("Attached.Call counts a crash; counting it here too makes two: %v", r.got)
	}
	n := len(f.calls)
	// And the disc plays on, with the helper out of it.
	if p = s.Do(ctx, dvdsession.Action{Type: "press", Button: 1}); p.Kind != "title" || p.Title == nil {
		t.Fatalf("%+v", p)
	}
	if len(f.calls) != n {
		t.Fatalf("the dead helper was asked again: %v", f.calls[n:])
	}
}

// Every kind of call can be the one that finds the helper gone.
func TestHelperCrashAnywhere(t *testing.T) {
	for _, method := range []string{wire.MethodNav, wire.MethodStart, wire.MethodTick, wire.MethodMenu, wire.MethodStepPGC, wire.MethodRange, wire.MethodClone, wire.MethodCellEnded, wire.MethodCell, wire.MethodSetAudio, wire.MethodSyncCell, wire.MethodSubs} {
		t.Run(method, func(t *testing.T) {
			path := makeDisc(t)
			f := &fake{hook: crashOn(method)}
			_, s := playAs(t, path, f, &reports{})
			ctx := context.Background()
			p := s.Start(ctx)
			for _, a := range []dvdsession.Action{{Type: "press", Button: 1}, {Type: "audio", Stream: 0, PositionSec: 5}, {Type: "key", Key: "menu", PositionSec: 70}, {Type: "resume"}, {Type: "ended"}, {Type: "ended"}} {
				if p = s.Do(ctx, a); p == nil {
					t.Fatalf("no answer after %+v", a)
				}
			}
			if _, err := s.Subtitles(1, 0, 0, 100); err != nil {
				t.Fatal(err)
			}
			if p = s.Start(ctx); p == nil || p.Kind != "menu" {
				t.Fatalf("the disc should still play: %+v", p)
			}
		})
	}
}

// A call about the disc finds the helper gone: the built-in copy answers it, so what's
// on screen stays, and the next thing the viewer does starts the disc again.
func TestHelperCrashInAMenuScan(t *testing.T) {
	path := makeDisc(t)
	f := &fake{}
	e, _ := openFake(t, f, path)
	n := e.NewNav(wittydvd.NavOptions{})
	st := n.Start()
	f.hook = crashOn(wire.MethodMenu)
	pic, err := e.Menu(titleSpace, st.FirstSector, st.LastSector, wittydvd.MenuScanOptions{})
	if err != nil || len(pic.Buttons) != 1 {
		t.Fatalf("%+v %v", pic, err)
	}
	if !f.closed {
		t.Fatal("the helper was left running")
	}
	c := pic.Buttons[0].Cmd
	got := n.PressButton(1, &c)
	if got.CellN != 1 || got.Serial <= st.Serial {
		t.Fatalf("should be the first cell again, as a new screen: %+v", got)
	}
	if got = n.PressButton(1, &c); got.CellN != 2 {
		t.Fatalf("the built-in copy should carry on: %+v", got)
	}
}

// The helper has gone and now the disc has too. Nothing to do but say so.
func TestNothingLeftToReadTheDisc(t *testing.T) {
	path := makeDisc(t)
	f := &fake{}
	e, _ := openFake(t, f, path)
	n := e.NewNav(wittydvd.NavOptions{})
	n.Start()
	f.hook = crashOn(wire.MethodNext)
	e.DiscEngine.(*remote).closeAll() // a file in use can't be removed on Windows
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if st := n.NextChapter(); st == nil || st.Kind != wittydvd.StepStop {
		t.Fatalf("%+v", st)
	}
	if _, err := e.Menu(titleSpace, 0, 9, wittydvd.MenuScanOptions{}); err == nil {
		t.Fatal("a disc that's gone can't answer")
	}
	if e.Disc == nil || e.Disc.VMG == nil {
		t.Fatal("the session walks the disc's structure")
	}
	if n.Registers() == nil || n.StepPGC(&wittydvd.Step{}) != nil {
		t.Fatal("the navigator must still answer")
	}
}

// Helpers that can't or won't open the disc: the built-in copy opens it. Only a fault
// of the helper's own counts against it.
func TestOpenFallsBack(t *testing.T) {
	path := makeDisc(t)
	for _, c := range []struct {
		name    string
		answer  func() ([]byte, error)
		reports int
	}{
		{"its limits", func() ([]byte, error) { return nil, &wittymodule.CallError{Code: wire.CodeDenied, Message: "denied"} }, 0},
		{"a refusal", func() ([]byte, error) { return nil, &wittymodule.CallError{Message: "no thanks"} }, 0},
		{"a crash", func() ([]byte, error) { return nil, errCrash }, 0},
		{"nonsense", func() ([]byte, error) { return []byte("nonsense"), nil }, 1},
		{"no structure", func() ([]byte, error) { b, _ := wire.Encode(&wittydvd.Disc{}); return b, nil }, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fake{hook: func(method string, _ json.RawMessage) ([]byte, error, bool) {
				if method == wire.MethodOpen {
					b, err := c.answer()
					return b, err, true
				}
				return nil, nil, false
			}}
			e, r := openFake(t, f, path)
			if isRemote(e) || !f.closed || len(r.got) != c.reports {
				t.Fatalf("remote=%v closed=%v reports=%v", isRemote(e), f.closed, r.got)
			}
			if st := e.NewNav(wittydvd.NavOptions{}).Start(); st.Kind != wittydvd.StepTitle {
				t.Fatalf("%+v", st)
			}
		})
	}
}

// answering makes a helper give an answer to one method, then behave.
func answering(method string, blob []byte, err error) func(string, json.RawMessage) ([]byte, error, bool) {
	return func(m string, _ json.RawMessage) ([]byte, error, bool) {
		if m == method {
			return blob, err, true
		}
		return nil, nil, false
	}
}

func extentsOf(t *testing.T, e ...wittydvd.FileExtent) []byte {
	t.Helper()
	b, err := wire.Encode(e)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// FFmpeg reads what the extents name, with the server's rights. A helper pointing
// anywhere else is refused, counted, and replaced by the built-in copy's answer.
func TestExtentsOutsideTheDiscAreRefused(t *testing.T) {
	path := makeDisc(t)
	vob := filepath.Join(path, "VIDEO_TS", "VTS_01_1.VOB")
	secret := filepath.Join(filepath.Dir(path), "secret.txt")
	notes := filepath.Join(path, "VIDEO_TS", "notes.txt")
	for _, p := range []string{secret, notes} {
		if err := os.WriteFile(p, []byte("private"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, ext := range map[string]wittydvd.FileExtent{
		"a file elsewhere":   {File: secret, Length: 7},
		"up and out":         {File: filepath.Join(path, "VIDEO_TS") + string(filepath.Separator) + ".." + string(filepath.Separator) + ".." + string(filepath.Separator) + "secret.txt", Length: 7},
		"not video":          {File: notes, Length: 7},
		"relative":           {File: "secret.vob", Length: 7},
		"the folder":         {File: path, Length: 7},
		"negative offset":    {File: vob, Offset: -1, Length: 7},
		"negative length":    {File: vob, Length: -7},
		"past any disc":      {File: vob, Offset: 1 << 62, Length: 7},
		"an empty name":      {Length: 7},
		"a line in the name": {File: vob + "\nsubfile,,start,0,end,99,,:/etc/passwd\n.vob", Length: 7},
	} {
		t.Run(name, func(t *testing.T) {
			for _, ask := range []func(*dvdsession.Engine) ([]wittydvd.FileExtent, error){
				func(e *dvdsession.Engine) ([]wittydvd.FileExtent, error) { return e.RangeExtents(titleSpace, 0, 9) },
				func(e *dvdsession.Engine) ([]wittydvd.FileExtent, error) {
					return e.CellExtents(titleSpace, wittydvd.Cell{FirstSector: 0, LastSector: 9})
				},
			} {
				f := &fake{}
				e, r := openFake(t, f, path)
				f.hook = func(m string, _ json.RawMessage) ([]byte, error, bool) {
					if m == wire.MethodRange || m == wire.MethodCell {
						return extentsOf(t, ext), nil, true
					}
					return nil, nil, false
				}
				got, err := ask(e)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != 1 || got[0].File != vob || got[0].Offset != 0 || got[0].Length != 10*wittydvd.Sector {
					t.Fatalf("not the built-in copy's answer: %+v", got)
				}
				if !f.closed || len(r.got) != 1 {
					t.Fatalf("closed=%v reports=%v", f.closed, r.got)
				}
			}
		})
	}
}

func TestBoundsOfADiscImage(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "Disc.iso")
	other := filepath.Join(dir, "Other.iso")
	if err := os.WriteFile(img, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := newBounds(img)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.check([]wittydvd.FileExtent{{File: img, Offset: 2048, Length: 2048}}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{other, dir, img + ".vob", filepath.Join(img, "x"), filepath.Join(dir, "..", "Disc.iso")} {
		if err := b.check([]wittydvd.FileExtent{{File: f, Length: 1}}); err == nil {
			t.Errorf("%s was allowed", f)
		}
	}
	if err := b.check(make([]wittydvd.FileExtent, maxExtents+1)); err == nil {
		t.Error("too many extents were allowed")
	}
	if runtime.GOOS == "windows" {
		// The same file as the system sees it, and ways round it.
		for _, f := range []string{strings.ToUpper(img), strings.ReplaceAll(img, `\`, `/`), filepath.Join(dir, ".", "Disc.iso")} {
			if err := b.check([]wittydvd.FileExtent{{File: f, Length: 1}}); err != nil {
				t.Errorf("%s: %v", f, err)
			}
		}
		for _, f := range []string{img + ":secret", img + "::$DATA", `\\?\` + other, `\\.\` + other, `\\localhost\c$\Windows\win.ini`, `C:\Windows\win.ini`, `NUL`, `COM1`} {
			if err := b.check([]wittydvd.FileExtent{{File: f, Length: 1}}); err == nil {
				t.Errorf("%s was allowed", f)
			}
		}
	}
}

func TestBoundsOfADiscFolder(t *testing.T) {
	path := makeDisc(t)
	b, err := newBounds(path)
	if err != nil {
		t.Fatal(err)
	}
	vob := filepath.Join(path, "VIDEO_TS", "VTS_01_1.VOB")
	if err := b.check([]wittydvd.FileExtent{{File: vob, Length: 1}}); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := b.check([]wittydvd.FileExtent{{File: strings.ToLower(vob), Length: 1}}); err != nil {
			t.Error(err)
		}
		for _, f := range []string{vob + ":secret.vob", filepath.Join(path, "VIDEO_TS", "NUL.vob"), filepath.Join(path, "VIDEO_TS", "COM1.vob"), path + `\..\..\x.vob`} {
			if err := b.check([]wittydvd.FileExtent{{File: f, Length: 1}}); err == nil {
				t.Errorf("%s was allowed", f)
			}
		}
	}
	// The folder's name as the start of another's is not inside it.
	sibling := path + "-2.vob"
	if err := os.WriteFile(sibling, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := b.check([]wittydvd.FileExtent{{File: sibling, Length: 1}}); err == nil {
		t.Error("a neighbour with the same start of its name was allowed")
	}
	// A link can't lead out of the disc.
	outside := filepath.Join(t.TempDir(), "outside.vob")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(path, "VIDEO_TS", "LINK.VOB")
	if err := os.Symlink(outside, link); err != nil {
		t.Logf("no links here: %v", err)
		return
	}
	if err := b.check([]wittydvd.FileExtent{{File: link, Length: 1}}); err == nil {
		t.Error("a link out of the disc was allowed")
	}
}

// A fork saying a disc is clear changes nothing. An encrypted feature never gets as far
// as a helper, and the built-in copy refuses it the way it always has.
func TestEncryptedDiscNeverReachesTheHelper(t *testing.T) {
	path := makeDisc(t, 0) // the first cell of the feature
	f := &fake{}
	e, _ := openFake(t, f, path)
	if isRemote(e) || len(f.calls) != 0 || !f.closed {
		t.Fatalf("remote=%v calls=%v closed=%v", isRemote(e), f.calls, f.closed)
	}
	pic, err := e.Menu(titleSpace, 0, 9, wittydvd.MenuScanOptions{})
	if err != nil || !pic.Scrambled {
		t.Fatalf("%+v %v", pic, err)
	}
}

// Encrypted video the feature's check doesn't see, and a helper saying it is clear:
// the server reads the sectors itself before believing anything.
func TestServerChecksTheSectorsAskedFor(t *testing.T) {
	path := makeDisc(t)
	f := &fake{}
	e, _ := openFake(t, f, path)
	if !isRemote(e) {
		t.Fatal("this disc should use the helper")
	}
	// Only now does part of the disc turn out to be encrypted: as a disc with a clear
	// feature and encrypted extras would look, and with the helper's scan none the wiser.
	vob := filepath.Join(path, "VIDEO_TS", "VTS_01_1.VOB")
	w, err := os.OpenFile(vob, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteAt(scrambledPack(), 11*wittydvd.Sector); err != nil {
		t.Fatal(err)
	}
	w.Close()
	clean, err := wire.EncodeMenu(&wittydvd.MenuPicture{FrameW: 720, FrameH: 480, Aspect: 1.3})
	if err != nil {
		t.Fatal(err)
	}
	f.hook = answering(wire.MethodMenu, clean, nil)
	pic, err := e.Menu(titleSpace, 10, 19, wittydvd.MenuScanOptions{})
	if err != nil || !pic.Scrambled {
		t.Fatalf("the helper's word was taken: %+v %v", pic, err)
	}
	if slices.Contains(f.calls, wire.MethodMenu) {
		t.Fatal("the helper was asked about encrypted sectors")
	}
	if subs, err := e.CellSubpictures(titleSpace, wittydvd.Cell{FirstSector: 10, LastSector: 19}); err != nil || len(subs.Units) != 0 {
		t.Fatalf("%+v %v", subs, err)
	}
	// Where FFmpeg would read is checked too, wherever the helper says that is.
	f.hook = answering(wire.MethodRange, extentsOf(t, wittydvd.FileExtent{File: vob, Offset: 10 * wittydvd.Sector, Length: 10 * wittydvd.Sector}), nil)
	if _, err := e.RangeExtents(titleSpace, 0, 9); !errors.Is(err, errScrambled) {
		t.Fatalf("%v", err)
	}
	// The other cell is still fine.
	f.hook = nil
	if pic, err = e.Menu(titleSpace, 0, 9, wittydvd.MenuScanOptions{}); err != nil || pic.Scrambled || len(pic.Buttons) != 1 {
		t.Fatalf("%+v %v", pic, err)
	}
}

// A helper's own refusal is believed.
func TestHelpersRefusalStands(t *testing.T) {
	f := &fake{}
	e, r := openFake(t, f, makeDisc(t))
	b, err := wire.EncodeMenu(&wittydvd.MenuPicture{Scrambled: true})
	if err != nil {
		t.Fatal(err)
	}
	f.hook = answering(wire.MethodMenu, b, nil)
	pic, err := e.Menu(titleSpace, 0, 9, wittydvd.MenuScanOptions{})
	if err != nil || !pic.Scrambled || len(r.got) != 0 || f.closed {
		t.Fatalf("%+v %v %v", pic, err, r.got)
	}
}

// When the built-in copy can't open a disc, a newer version may. The server still
// looks for encryption, through the disc's files and wherever the helper points.
func TestDiscOnlyAHelperCanRead(t *testing.T) {
	broken := func(t *testing.T, scrambled ...int) (string, *wittydvd.Disc) {
		path := makeDisc(t, scrambled...)
		o, err := wittydvd.Open(makeDisc(t))
		if err != nil {
			t.Fatal(err)
		}
		defer o.Close()
		if err := os.WriteFile(filepath.Join(path, "VIDEO_TS", "VIDEO_TS.IFO"), []byte("damaged"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path, o.Disc
	}
	serve := func(t *testing.T, d *wittydvd.Disc, vob string) *fake {
		blob, err := wire.EncodeDisc(d)
		if err != nil {
			t.Fatal(err)
		}
		return &fake{hook: func(m string, _ json.RawMessage) ([]byte, error, bool) {
			switch m {
			case wire.MethodOpen:
				return blob, nil, true
			case wire.MethodRange:
				return extentsOf(t, wittydvd.FileExtent{File: vob, Length: 20 * wittydvd.Sector}), nil, true
			}
			return nil, nil, false
		}}
	}
	t.Run("clear", func(t *testing.T) {
		path, d := broken(t)
		f := serve(t, d, filepath.Join(path, "VIDEO_TS", "VTS_01_1.VOB"))
		r := &reports{}
		e, err := open(f, r.add, quiet, path)
		if err != nil {
			t.Fatal(err)
		}
		defer e.Close()
		if !isRemote(e) {
			t.Fatal("the helper could read this disc")
		}
		if ext, err := e.RangeExtents(titleSpace, 0, 19); err != nil || len(ext) != 1 {
			t.Fatalf("%+v %v", ext, err)
		}
	})
	t.Run("encrypted", func(t *testing.T) {
		path, d := broken(t, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18)
		f := serve(t, d, filepath.Join(path, "VIDEO_TS", "VTS_01_1.VOB"))
		if _, err := open(f, (&reports{}).add, quiet, path); err == nil {
			t.Fatal("the built-in copy can't open it, and the helper mustn't either")
		}
		if slices.Contains(f.calls, wire.MethodOpen) {
			t.Fatal("an encrypted disc reached the helper")
		}
	})
}

// The answers a session walks and allocates from, with values no real disc has.
func TestImpossibleAnswersAreRefused(t *testing.T) {
	step := func(st wittydvd.Step) []byte {
		b, err := wire.Encode(&wire.NavAnswer{Step: &st})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	good := wittydvd.Step{Kind: wittydvd.StepTitle, Domain: wittydvd.DomainVTS, VTS: 1, PGCN: 1, CellN: 1, CellDuration: 60}
	change := func(f func(*wittydvd.Step)) []byte { st := good; f(&st); return step(st) }
	for name, b := range map[string][]byte{
		"garbage":       []byte("garbage"),
		"unknown kind":  change(func(s *wittydvd.Step) { s.Kind = "evil" }),
		"domain":        change(func(s *wittydvd.Step) { s.Domain = 99 }),
		"negative":      change(func(s *wittydvd.Step) { s.Domain = -1 }),
		"title set":     change(func(s *wittydvd.Step) { s.VTS = 100 }),
		"cell":          change(func(s *wittydvd.Step) { s.CellN = -1 }),
		"NaN":           change(func(s *wittydvd.Step) { s.CellDuration = math.NaN() }),
		"forever cell":  change(func(s *wittydvd.Step) { s.CellDuration = math.Inf(1) }),
		"backward":      change(func(s *wittydvd.Step) { s.StillSec = math.Inf(-1) }),
		"serial":        change(func(s *wittydvd.Step) { s.Serial = math.MaxInt }),
		"title's time":  change(func(s *wittydvd.Step) { s.Title = &wittydvd.TitleInfo{Duration: math.NaN()} }),
		"button":        change(func(s *wittydvd.Step) { s.DefaultButton = -3 }),
		"far too large": make([]byte, 1<<20),
	} {
		t.Run(name, func(t *testing.T) {
			f := &fake{}
			e, r := openFake(t, f, makeDisc(t))
			n := e.NewNav(wittydvd.NavOptions{})
			f.hook = answering(wire.MethodStart, b, nil)
			st := n.Start()
			if st == nil || st.Kind != wittydvd.StepTitle || !f.closed || len(r.got) != 1 {
				t.Fatalf("%+v closed=%v reports=%v", st, f.closed, r.got)
			}
		})
	}
}

// A step's domain name goes into the names of cache files. A helper can't choose it.
func TestStepCantNameFiles(t *testing.T) {
	f := &fake{}
	e, _ := openFake(t, f, makeDisc(t))
	n := e.NewNav(wittydvd.NavOptions{})
	b, err := wire.Encode(&wire.NavAnswer{Step: &wittydvd.Step{Kind: wittydvd.StepMenu, Domain: wittydvd.DomainVTSM, DomainName: `..\..\..\evil`, VTS: 1, StillSec: math.Inf(1), Reason: strings.Repeat("x", 10000)}})
	if err != nil {
		t.Fatal(err)
	}
	f.hook = answering(wire.MethodStart, b, nil)
	st := n.Start()
	if st.DomainName != "vtsm" || !math.IsInf(st.StillSec, 1) || len(st.Reason) > maxReason {
		t.Fatalf("%+v", st)
	}
}

func TestImpossibleDiscsAreRefused(t *testing.T) {
	pgc := func() *wittydvd.PGC { return &wittydvd.PGC{} }
	vts := func(v wittydvd.VTS) map[int]*wittydvd.VTS { return map[int]*wittydvd.VTS{1: &v} }
	for name, d := range map[string]*wittydvd.Disc{
		"no structure":     {},
		"too many titles":  {VMG: &wittydvd.VMG{Titles: make([]wittydvd.TitleSearch, maxTitles+1)}},
		"a chain missing":  {VMG: &wittydvd.VMG{}, VTS: vts(wittydvd.VTS{PGCs: []wittydvd.PGCEntry{{}}})},
		"menu chain":       {VMG: &wittydvd.VMG{Menus: []wittydvd.LangUnit{{PGCs: []wittydvd.PGCEntry{{}}}}}},
		"too many streams": {VMG: &wittydvd.VMG{}, VTS: vts(wittydvd.VTS{Subp: make([]wittydvd.SubpAttr, 33)})}, // the session indexes a table of 32
		"too much audio":   {VMG: &wittydvd.VMG{}, VTS: vts(wittydvd.VTS{Audio: make([]wittydvd.AudioAttr, 9)})},
		"no such set":      {VMG: &wittydvd.VMG{}, VTS: map[int]*wittydvd.VTS{100: {}}},
		"empty set":        {VMG: &wittydvd.VMG{}, VTS: map[int]*wittydvd.VTS{1: nil}},
		"impossible time":  {VMG: &wittydvd.VMG{FirstPlay: &wittydvd.PGC{Duration: math.NaN()}}},
		"cell's time":      {VMG: &wittydvd.VMG{FirstPlay: &wittydvd.PGC{Cells: []wittydvd.Cell{{Duration: math.Inf(1)}}}}},
		"too many cells":   {VMG: &wittydvd.VMG{FirstPlay: &wittydvd.PGC{Cells: make([]wittydvd.Cell, maxCells+1)}}},
	} {
		if err := checkDisc(d); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := checkDisc(&wittydvd.Disc{VMG: &wittydvd.VMG{FirstPlay: pgc()}, VTS: vts(wittydvd.VTS{PGCs: []wittydvd.PGCEntry{{PGC: pgc()}}})}); err != nil {
		t.Error(err)
	}
}

func TestImpossibleMenusAreRefused(t *testing.T) {
	for name, p := range map[string]*wittydvd.MenuPicture{
		"huge frame":   {FrameW: 1 << 20, FrameH: 1 << 20}, // a session allocates this much to draw it
		"negative":     {FrameW: -1},
		"many buttons": {Buttons: make([]wittydvd.MenuButton, maxButtons+1)},
		"bad aspect":   {Aspect: math.NaN()},
		"bad time":     {ButtonsFromSec: math.Inf(1)},
		"far away":     {Normal: &wittydvd.RGBA{X: math.MaxInt - 5}}, // X plus width must not wrap
		"button":       {Buttons: []wittydvd.MenuButton{{Select: &wittydvd.RGBA{Y: -math.MaxInt}}}},
	} {
		if err := checkMenu(p); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	f := &fake{}
	e, r := openFake(t, f, makeDisc(t))
	b, err := wire.EncodeMenu(&wittydvd.MenuPicture{FrameW: 1 << 20, FrameH: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	f.hook = answering(wire.MethodMenu, b, nil)
	pic, err := e.Menu(titleSpace, 0, 9, wittydvd.MenuScanOptions{})
	if err != nil || pic.FrameW != 720 || !f.closed || len(r.got) != 1 {
		t.Fatalf("%+v %v %v", pic, err, r.got)
	}
}

// The errors a session acts on get through as themselves, and a helper's plain refusal
// goes to the viewer without costing it anything.
func TestErrorsComeThrough(t *testing.T) {
	f := &fake{}
	e, r := openFake(t, f, makeDisc(t))
	f.hook = answering(wire.MethodMenu, nil, &wittymodule.CallError{Code: wire.CodeNoNav, Message: "no nav"})
	if _, err := e.Menu(titleSpace, 0, 9, wittydvd.MenuScanOptions{}); !errors.Is(err, wittydvd.ErrNoNav) {
		t.Fatal(err)
	}
	f.hook = answering(wire.MethodMenu, nil, &wittymodule.CallError{Message: strings.Repeat("a read went wrong ", 1000)})
	_, err := e.Menu(titleSpace, 1, 9, wittydvd.MenuScanOptions{})
	if err == nil || len(err.Error()) > maxReason || f.closed || len(r.got) != 0 {
		t.Fatalf("%v closed=%v %v", err, f.closed, r.got)
	}
	// A panic in the helper is a fault of its own, even though it still answers.
	f.hook = answering(wire.MethodMenu, nil, &wittymodule.CallError{Code: "panic", Message: "boom"})
	if pic, err := e.Menu(titleSpace, 2, 9, wittydvd.MenuScanOptions{}); err == nil && pic == nil {
		t.Fatal("no answer")
	}
	if !f.closed || len(r.got) != 1 {
		t.Fatalf("closed=%v %v", f.closed, r.got)
	}
}

// A session reads the registers and the program chain constantly; only moving the disc
// should cost a call, and the what-if copy mustn't move the real one.
func TestCallsAreFew(t *testing.T) {
	f := &fake{}
	e, _ := openFake(t, f, makeDisc(t))
	n := e.NewNav(wittydvd.NavOptions{})
	st := n.Start()
	calls := len(f.calls)
	for range 5 {
		_ = n.Registers().SPRM[wittydvd.SprmAudio]
		if n.StepPGC(st) == nil {
			t.Fatal("no chain")
		}
	}
	if got := len(f.calls) - calls; got != 1 {
		t.Fatalf("%d calls: %v", got, f.calls[calls:])
	}
	c := n.Clone()
	if ahead := c.CellEnded(); ahead == nil || ahead.CellN != 2 {
		t.Fatalf("%+v", ahead)
	}
	if cur := n.Current(); cur.CellN != 1 {
		t.Fatalf("looking ahead moved the disc: %+v", cur)
	}
	before := n.Registers()
	n.SetAudio(1)
	if before != n.Registers() || before.SPRM[wittydvd.SprmAudio] != 1 {
		t.Fatalf("a held pointer should see the change: %+v", before.SPRM)
	}
	if ahead := c.CellEnded(); ahead == nil {
		t.Fatal("a look ahead should still answer")
	}
	n.Tick(1)
}

func TestSubsAreRemembered(t *testing.T) {
	f := &fake{}
	e, _ := openFake(t, f, makeDisc(t))
	c := wittydvd.Cell{FirstSector: 0, LastSector: 9}
	for range 3 {
		if _, err := e.CellSubpictures(titleSpace, c); err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	for _, m := range f.calls {
		if m == wire.MethodSubs {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d calls: a session asks again for every window of subtitles", n)
	}
	for i := range maxCachedSubs + 2 {
		c := wittydvd.Cell{FirstSector: uint32(i), LastSector: 9}
		if _, err := e.CellSubpictures(titleSpace, c); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(e.DiscEngine.(*remote).subs); got != maxCachedSubs {
		t.Fatalf("%d kept", got)
	}
}
