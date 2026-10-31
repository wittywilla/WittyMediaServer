// Package bdremote runs a Blu-ray session's engine in a helper process when a
// downloaded WittyBluray is in use. To the session it looks like the built-in engine.
//
// A helper may be someone's fork, so nothing it says is taken on trust: where files
// are, how large things are and what its structures hold are all checked here before
// the session sees them, and whether a disc is encrypted is the server's own finding
// (bdsession.openEngine). Video never passes through a helper, and FFmpeg reads only
// the disc the viewer opened.
//
// A helper that fails, or that sends something those checks refuse, is replaced by
// the built-in copy for the rest of the session, which goes back to the disc's first
// play. The session doesn't know: a call in progress is simply answered by the
// built-in copy.
package bdremote

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittybluray/helper"
	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymodule"
)

const (
	moduleID = "wittybluray"
	// callLimit is how long one call may take before the helper counts as hung: as long
	// as any helper gets, since a disc can be on a slow network share.
	callLimit = 60 * time.Second
	// Limits on what a helper may answer with, far beyond any real disc.
	maxExtents    = 1 << 16
	maxEffectList = 1 << 16
	maxSubPicture = 8192 // on either side
	maxReason     = 500
)

// restarted is what the viewer is told when the helper stops working.
const restarted = "The downloaded WittyBluray stopped working, so this disc started again on the built-in copy."

// Use makes Blu-ray sessions use the version of WittyBluray the manager has in use.
func Use(m *modules.Manager) {
	bdsession.RemoteEngine = func(path string, o wittybluray.NavOptions, log *slog.Logger) *bdsession.Remote {
		a := m.Attach(moduleID, callLimit)
		if a == nil {
			return nil
		}
		return open(a, path, o, log, func(reason string) { _ = m.ReportFailure(moduleID, a.Version(), reason) })
	}
}

// process is the helper as this package uses it: modules.Attached, or a test's.
type process interface {
	Call(method string, params, out any) ([]byte, error)
	Close()
}

// engine is one session's disc and navigator in a helper. mu is held through each
// call, and the protocol has one call at a time anyway.
type engine struct {
	mu     sync.Mutex
	h      process // nil once the built-in copy has taken over
	path   string
	image  bool   // an image file; otherwise a folder
	root   string // the folder extents must lie in
	real   string // the same with its links followed
	opts   wittybluray.NavOptions
	log    *slog.Logger
	report func(reason string) // a failure the helper's own calls can't have counted
	angle  int

	// What the session reads as often as it likes. The registers stay the same object:
	// a session may hold them across a call that changes them.
	regs     *wittybluray.Registers
	timer    *wittybluray.NavTimer
	timerSeq uint64

	// Each structure is fetched once. A playlist is the same pointer every time.
	playlists map[int]*wittybluray.Playlist
	clips     map[string]*wittybluray.Clip
	extents   map[string][]wittybluray.FileExtent
	main      *int
	sounds    *wittybluray.Sounds
	hasSounds bool

	// The built-in copy, once it has taken over. restart: its first step is first play.
	disc    *wittybluray.Disc
	nav     *wittybluray.Nav
	restart bool
}

// badAnswer is a helper saying something that can't be right. The protocol can't have
// counted that as a failure, as it was a good answer as far as the protocol could tell.
type badAnswer struct{ err error }

func (b *badAnswer) Error() string { return "it sent a bad answer: " + b.err.Error() }

// open asks a helper to open a disc. nil means the built-in copy will have it: a
// helper that can't read the disc may be limited by its sandbox, or just be broken.
func open(h process, path string, o wittybluray.NavOptions, log *slog.Logger, report func(string)) *bdsession.Remote {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	e, err := newEngine(h, path, o, log, report)
	if err == nil {
		var info helper.Opened
		if _, err = h.Call("open", helper.Params{Path: e.path, Options: o}, &info); err == nil {
			e.regs.PSR = info.PSR
			return &bdsession.Remote{Disc: e, Nav: e, Encrypted: info.Encrypted, Stereo3D: info.Stereo3D}
		}
	}
	log.Info("WittyBluray: using the built-in copy for this disc", "cat", "dvd", "err", err)
	h.Close()
	return nil
}

func newEngine(h process, path string, o wittybluray.NavOptions, log *slog.Logger, report func(string)) (*engine, error) {
	abs, err := filepath.Abs(path) // the helper runs in a folder of its own
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	e := &engine{h: h, path: abs, image: !st.IsDir(), opts: o, log: log, report: report, regs: wittybluray.NewRegisters(o.Registers),
		playlists: map[int]*wittybluray.Playlist{}, clips: map[string]*wittybluray.Clip{}, extents: map[string][]wittybluray.FileExtent{}}
	if st.IsDir() {
		e.root = abs
		if b := filepath.Base(abs); strings.EqualFold(b, "BDMV") || strings.EqualFold(b, "BDAV") {
			e.root = filepath.Dir(abs) // as the engine reads it
		}
		if e.real, err = filepath.EvalSymlinks(e.root); err != nil {
			return nil, err
		}
	}
	return e, nil
}

// ---------------------------------------------------------------- failure

// fail hands the disc to the built-in copy. Call with e.mu held and a helper in use.
func (e *engine) fail(err error) {
	e.log.Warn("WittyBluray: the helper failed; the built-in copy takes over this disc", "cat", "dvd", "err", err)
	var bad *badAnswer
	var ce *wittymodule.CallError
	switch {
	case errors.As(err, &bad):
		e.report(err.Error())
	case errors.As(err, &ce) && ce.Code != "denied": // "denied" may be its limits, not its fault
		e.report(ce.Error())
	}
	e.h.Close()
	e.h, e.restart = nil, true
	e.playlists, e.clips, e.extents, e.sounds = nil, nil, nil, nil
	d, oerr := wittybluray.Open(e.path)
	if oerr == nil && d.Encrypted {
		_ = d.Close()
		oerr = errors.New("this disc is encrypted")
	}
	if oerr != nil {
		e.log.Warn("WittyBluray: the built-in copy can't open this disc", "cat", "dvd", "err", oerr)
		return
	}
	e.disc, e.nav = d, wittybluray.NewNav(d, e.opts)
	if e.angle > 0 {
		e.nav.SetAngle(e.angle)
	}
}

// local is a step from the built-in copy, which has first play to do first.
func (e *engine) local(do func(n *wittybluray.Nav) *wittybluray.Step) *wittybluray.Step {
	if e.nav == nil {
		return &wittybluray.Step{Kind: wittybluray.StepStop, Reason: "the disc can't be read any more", Object: -1}
	}
	if e.restart {
		e.restart = false
		st := e.nav.Start()
		if st.Reason == "" {
			st.Reason = restarted
		}
		return st
	}
	return do(e.nav)
}

// builtinPlaylist finds, in the built-in copy, a playlist the helper handed out earlier.
func (e *engine) builtinPlaylist(pl *wittybluray.Playlist) *wittybluray.Playlist {
	if e.disc != nil && pl != nil {
		if same := e.disc.Playlist(pl.Number); same != nil {
			return same
		}
	}
	return pl
}

// answeredNo: a helper in good order, saying this one thing can't be done, as the
// built-in copy would by returning an error.
func answeredNo(err error) bool {
	var ce *wittymodule.CallError
	return errors.As(err, &ce) && ce.Code == ""
}

// decode reads gob from a helper. Gob's decoder takes care with sizes, the protocol
// has already limited the whole answer, and this keeps a decoder's panic from
// becoming the server's.
func decode(blob []byte, v any) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%v", p)
		}
	}()
	return gob.NewDecoder(bytes.NewReader(blob)).Decode(v)
}

// fetch asks for a structure. nil with no error means the helper says there is none.
func fetch[T any](e *engine, method string, p helper.Params, check func(*T) error) (*T, error) {
	blob, err := e.h.Call(method, p, nil)
	if err != nil {
		return nil, err
	}
	if len(blob) == 0 {
		return nil, nil
	}
	v := new(T)
	if err := decode(blob, v); err != nil {
		return nil, &badAnswer{err}
	}
	if check != nil {
		if err := check(v); err != nil {
			return nil, &badAnswer{err}
		}
	}
	return v, nil
}

// maxClipLead is more than any disc stores sound ahead of its picture.
const maxClipLead = 30

// must is fetch for what a helper must answer: a refusal is passed on as the built-in
// copy's own error would be, and anything else goes to the built-in copy. Call with
// e.mu held.
func must[T any](e *engine, method string, p helper.Params, check func(*T) error, builtin func(d *wittybluray.Disc) (*T, error)) (*T, error) {
	if e.h != nil {
		v, err := fetch(e, method, p, check)
		if err == nil && v != nil {
			return v, nil
		}
		if err == nil {
			err = &badAnswer{fmt.Errorf("%s came back empty", method)}
		}
		if answeredNo(err) {
			return nil, err
		}
		e.fail(err)
	}
	if e.disc == nil {
		return nil, errors.New("the disc can't be read any more")
	}
	return builtin(e.disc)
}

// ---------------------------------------------------------------- the disc

func (e *engine) Playlist(n int) *wittybluray.Playlist {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.playlist(n)
}

func (e *engine) playlist(n int) *wittybluray.Playlist {
	if e.h != nil {
		if pl, ok := e.playlists[n]; ok {
			return pl
		}
		pl, err := fetch(e, "playlist", helper.Params{N: n}, func(pl *wittybluray.Playlist) error { return checkPlaylist(pl, n) })
		if err == nil {
			if pl != nil {
				e.playlists[n] = pl
			}
			return pl
		}
		e.fail(err)
	}
	if e.disc == nil {
		return nil
	}
	return e.disc.Playlist(n)
}

func (e *engine) MainPlaylist() *wittybluray.Playlist {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h != nil {
		if e.main == nil {
			var m helper.Main
			if _, err := e.h.Call("main", helper.Params{}, &m); err != nil {
				e.fail(err)
				return e.builtinMain()
			}
			if !m.OK {
				m.N = -1
			}
			e.main = &m.N
		}
		if *e.main < 0 {
			return nil
		}
		pl := e.playlist(*e.main)
		if pl == nil && e.h != nil {
			e.fail(&badAnswer{fmt.Errorf("its main playlist %d doesn't exist", *e.main)})
		}
		if e.h != nil {
			return pl
		}
	}
	return e.builtinMain()
}

func (e *engine) builtinMain() *wittybluray.Playlist {
	if e.disc == nil {
		return nil
	}
	return e.disc.MainPlaylist()
}

// PlaylistDuration is arithmetic on the playlist, which needs no disc.
func (e *engine) PlaylistDuration(pl *wittybluray.Playlist) float64 {
	return (&wittybluray.Disc{}).PlaylistDuration(pl)
}

func (e *engine) Clip(name string) *wittybluray.Clip {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h != nil {
		if c, ok := e.clips[name]; ok {
			return c
		}
		c, err := fetch(e, "clip", helper.Params{Name: name}, func(c *wittybluray.Clip) error {
			if c.Name != name {
				return fmt.Errorf("asked for clip %q, it sent %q", name, c.Name)
			}
			return nil
		})
		if err == nil {
			if c != nil {
				e.clips[name] = c
			}
			return c
		}
		e.fail(err)
	}
	if e.disc == nil {
		return nil
	}
	return e.disc.Clip(name)
}

func (e *engine) ClipExtents(name string) ([]wittybluray.FileExtent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h != nil {
		if out, ok := e.extents[name]; ok {
			return out, nil
		}
	}
	got, err := must(e, "extents", helper.Params{Name: name}, func(ext *[]wittybluray.FileExtent) error {
		checked, err := e.checkExtents(*ext)
		*ext = checked
		return err
	}, func(d *wittybluray.Disc) (*[]wittybluray.FileExtent, error) {
		out, err := d.ClipExtents(name)
		return &out, err
	})
	if err != nil {
		return nil, err
	}
	if e.h != nil {
		e.extents[name] = *got
	}
	return *got, nil
}

// ClipLead says how far ahead of a clip's first picture its first packet is. Zero is
// always safe: the join is only as exact as it was before, so a helper from before this
// question doesn't lose the disc over it.
func (e *engine) ClipLead(clip string, video uint16, from uint32, only ...uint16) float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h != nil {
		got, err := fetch(e, "cliplead", helper.Params{Name: clip, PID: video, From: from, Only: only}, func(l *helper.Lead) error {
			if !(l.Seconds >= 0 && l.Seconds <= maxClipLead) {
				return fmt.Errorf("a clip's lead of %v seconds", l.Seconds)
			}
			return nil
		})
		if err == nil && got != nil {
			return got.Seconds
		}
		var ce *wittymodule.CallError
		if errors.As(err, &ce) && ce.Code == "unknown-method" {
			return 0
		}
		if err == nil {
			err = &badAnswer{errors.New("cliplead came back empty")}
		}
		if answeredNo(err) {
			return 0
		}
		e.fail(err)
	}
	if e.disc == nil {
		return 0
	}
	return e.disc.ClipLead(clip, video, from, only...)
}

func (e *engine) Sounds() *wittybluray.Sounds {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h != nil {
		if e.hasSounds {
			return e.sounds
		}
		s, err := fetch[wittybluray.Sounds](e, "sounds", helper.Params{}, nil)
		if err == nil {
			e.sounds, e.hasSounds = s, true
			return s
		}
		e.fail(err)
	}
	if e.disc == nil {
		return nil
	}
	return e.disc.Sounds()
}

func (e *engine) LoadMenu(clip string, pid uint16, limit uint32) (*wittybluray.Menu, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return must(e, "menu", helper.Params{Name: clip, PID: pid, Limit: limit}, nil, func(d *wittybluray.Disc) (*wittybluray.Menu, error) {
		return d.LoadMenu(clip, pid, limit)
	})
}

func (e *engine) BDJObject(name string) (*wittybluray.BDJObject, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return must(e, "bdjobject", helper.Params{Name: name}, nil, func(d *wittybluray.Disc) (*wittybluray.BDJObject, error) {
		return d.BDJObject(name)
	})
}

func (e *engine) SubtitlesIn(clip string, pid uint16, stc int, from, to uint32) (*wittybluray.SubtitleWindow, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return must(e, "subtitles", helper.Params{Name: clip, PID: pid, STC: stc, From: from, To: to}, checkSubtitles, func(d *wittybluray.Disc) (*wittybluray.SubtitleWindow, error) {
		return d.SubtitlesIn(clip, pid, stc, from, to)
	})
}

func (e *engine) TextSubtitles(clip string, pid uint16, charCode int) ([]wittybluray.TextCue, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	got, err := must(e, "text", helper.Params{Name: clip, PID: pid, CharCode: charCode}, nil, func(d *wittybluray.Disc) (*[]wittybluray.TextCue, error) {
		out, err := d.TextSubtitles(clip, pid, charCode)
		return &out, err
	})
	if err != nil {
		return nil, err
	}
	return *got, nil
}

// Close stops the helper, or the built-in copy.
func (e *engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h != nil {
		e.h.Close()
		e.h = nil
	}
	if e.disc != nil {
		err := e.disc.Close()
		e.disc, e.nav = nil, nil
		return err
	}
	return nil
}

// ---------------------------------------------------------------- the navigator

// ask makes one navigator call and brings the copy of its state up to date. Call with
// e.mu held and a helper in use.
func (e *engine) ask(method string, p helper.Params) (*helper.Answer, error) {
	blob, err := e.h.Call(method, p, nil)
	if err != nil {
		return nil, err
	}
	a := new(helper.Answer)
	if err := json.Unmarshal(blob, a); err != nil {
		return nil, &badAnswer{err}
	}
	e.regs.PSR = a.PSR
	if a.TimerSeq != e.timerSeq {
		e.timer, e.timerSeq = a.Timer, a.TimerSeq
	}
	return a, nil
}

// checkedStep is an answer's step with its playlist filled in. Call with e.mu held.
func (e *engine) checkedStep(a *helper.Answer) (*wittybluray.Step, error) {
	st := a.Step
	if st == nil {
		return nil, &badAnswer{errors.New("no step")}
	}
	switch st.Kind {
	case wittybluray.StepPlay, wittybluray.StepStop, wittybluray.StepUnsupported, wittybluray.StepStay:
	default:
		return nil, &badAnswer{fmt.Errorf("a step of kind %q", st.Kind)}
	}
	if st.Item < 0 || !safeName(st.IGClip) || !safeName(st.BDJ) {
		return nil, &badAnswer{errors.New("a step that can't be right")}
	}
	if fx := st.Effects; fx != nil && (len(fx.EnableButton) > maxEffectList || len(fx.DisableButton) > maxEffectList) {
		return nil, &badAnswer{errors.New("a step with too many effects")}
	}
	if len(st.Reason) > maxReason {
		st.Reason = st.Reason[:maxReason]
	}
	if a.Playlist != nil {
		n := *a.Playlist
		if st.Playlist = e.playlist(n); st.Playlist == nil && e.h != nil {
			return nil, &badAnswer{fmt.Errorf("it asked to play playlist %d, which it doesn't have", n)}
		}
	}
	if st.Kind == wittybluray.StepPlay && st.Playlist == nil && e.h != nil {
		return nil, &badAnswer{errors.New("nothing to play in a step to play")}
	}
	return st, nil
}

// step asks for the next step, from the helper or the built-in copy.
func (e *engine) step(method string, p helper.Params, builtin func(n *wittybluray.Nav) *wittybluray.Step) *wittybluray.Step {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h != nil {
		a, err := e.ask(method, p)
		var st *wittybluray.Step
		if err == nil {
			st, err = e.checkedStep(a)
		}
		if err == nil && e.h != nil {
			return st
		}
		if e.h != nil { // fetching the step's playlist may have failed and handed over already
			e.fail(err)
		}
	}
	return e.local(builtin)
}

// tell tells the navigator something.
func (e *engine) tell(method string, p helper.Params, builtin func(n *wittybluray.Nav)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h != nil {
		_, err := e.ask(method, p)
		if err == nil {
			return
		}
		e.fail(err)
	}
	if e.nav != nil {
		builtin(e.nav)
	}
}

func (e *engine) Registers() *wittybluray.Registers {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.nav != nil {
		return e.nav.Registers()
	}
	return e.regs
}

func (e *engine) Timer() *wittybluray.NavTimer {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.nav != nil {
		return e.nav.Timer()
	}
	if e.h == nil {
		return nil
	}
	return e.timer
}

func (e *engine) Start() *wittybluray.Step {
	return e.step("start", helper.Params{}, func(n *wittybluray.Nav) *wittybluray.Step { return n.Start() })
}

func (e *engine) Resume() *wittybluray.Step {
	return e.step("resume", helper.Params{}, func(n *wittybluray.Nav) *wittybluray.Step { return n.Resume() })
}

func (e *engine) MenuCall() *wittybluray.Step {
	return e.step("menucall", helper.Params{}, func(n *wittybluray.Nav) *wittybluray.Step { return n.MenuCall() })
}

func (e *engine) PlaylistEnded() *wittybluray.Step {
	return e.step("playlistended", helper.Params{}, func(n *wittybluray.Nav) *wittybluray.Step { return n.PlaylistEnded() })
}

func (e *engine) NavTimerFired() *wittybluray.Step {
	return e.step("navtimerfired", helper.Params{}, func(n *wittybluray.Nav) *wittybluray.Step { return n.NavTimerFired() })
}

func (e *engine) PlayTitle(t int) *wittybluray.Step {
	return e.step("playtitle", helper.Params{N: t}, func(n *wittybluray.Nav) *wittybluray.Step { return n.PlayTitle(t) })
}

func (e *engine) PressButton(cmds []wittybluray.Command) *wittybluray.Step {
	return e.step("pressbutton", helper.Params{Commands: cmds}, func(n *wittybluray.Nav) *wittybluray.Step { return n.PressButton(cmds) })
}

func (e *engine) PlayPlaylist(pl *wittybluray.Playlist, item int, t uint32) *wittybluray.Step {
	return e.step("playplaylist", helper.Params{N: pl.Number, Item: item, Time: t}, func(n *wittybluray.Nav) *wittybluray.Step {
		return n.PlayPlaylist(e.builtinPlaylist(pl), item, t)
	})
}

func (e *engine) SkipItem(delta int) (*wittybluray.Step, bool) {
	ok := false
	st := func() *wittybluray.Step {
		e.mu.Lock()
		defer e.mu.Unlock()
		builtin := func(n *wittybluray.Nav) *wittybluray.Step {
			st, can := n.SkipItem(delta)
			ok = can
			return st
		}
		if e.h != nil {
			a, err := e.ask("skipitem", helper.Params{N: delta})
			if err == nil && !a.OK {
				return nil
			}
			var st *wittybluray.Step
			if err == nil {
				st, err = e.checkedStep(a)
			}
			if err == nil && e.h != nil {
				ok = true
				return st
			}
			if e.h != nil {
				e.fail(err)
			}
		}
		if e.restart || e.nav == nil {
			ok = true // first play comes next instead, or the stop that says why it can't
		}
		return e.local(builtin)
	}()
	return st, ok
}

func (e *engine) SetSubtitle(s int, shown bool) {
	e.tell("setsubtitle", helper.Params{N: s, Shown: shown}, func(n *wittybluray.Nav) { n.SetSubtitle(s, shown) })
}

func (e *engine) SetAudio(s int) {
	e.tell("setaudio", helper.Params{N: s}, func(n *wittybluray.Nav) { n.SetAudio(s) })
}

func (e *engine) SetAngle(a int) {
	e.mu.Lock()
	e.angle = a
	e.mu.Unlock()
	e.tell("setangle", helper.Params{N: a}, func(n *wittybluray.Nav) { n.SetAngle(a) })
}

func (e *engine) SetTime(item int, t uint32) {
	e.tell("settime", helper.Params{Item: item, Time: t}, func(n *wittybluray.Nav) { n.SetTime(item, t) })
}

func (e *engine) SetChapter(c int) {
	e.tell("setchapter", helper.Params{N: c}, func(n *wittybluray.Nav) { n.SetChapter(c) })
}

// SetMenuState runs on every action a viewer takes, and usually changes nothing.
func (e *engine) SetMenuState(page, button int) {
	e.mu.Lock()
	unchanged := e.h != nil && e.regs.PSR[wittybluray.PSRPage] == uint32(page) && e.regs.PSR[wittybluray.PSRButton] == uint32(button)
	e.mu.Unlock()
	if unchanged {
		return
	}
	e.tell("setmenustate", helper.Params{Page: page, Button: button}, func(n *wittybluray.Nav) { n.SetMenuState(page, button) })
}

func (e *engine) LoopsAtEnd(st *wittybluray.Step) bool {
	if st == nil || st.Playlist == nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h != nil {
		a, err := e.ask("loopsatend", helper.Params{N: st.Playlist.Number, Item: st.Item, Time: st.Time, ItemOnly: st.ItemOnly})
		if err == nil {
			return a.OK
		}
		e.fail(err)
	}
	if e.nav == nil {
		return false
	}
	c := *st
	c.Playlist = e.builtinPlaylist(st.Playlist)
	return e.nav.LoopsAtEnd(&c)
}

func (e *engine) Forbids(uo uint64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h != nil {
		a, err := e.ask("forbids", helper.Params{UO: uo})
		if err == nil {
			return a.OK
		}
		e.fail(err)
	}
	return e.nav != nil && e.nav.Forbids(uo)
}

// ---------------------------------------------------------------- checks

// checkExtents makes sure FFmpeg will only be pointed at the disc that was opened,
// and at no more of it than is there. The paths in the result are the server's own.
func (e *engine) checkExtents(in []wittybluray.FileExtent) ([]wittybluray.FileExtent, error) {
	if len(in) > maxExtents {
		return nil, fmt.Errorf("%d extents", len(in))
	}
	out := make([]wittybluray.FileExtent, 0, len(in))
	sizes := map[string]int64{}
	for _, x := range in {
		if x.Offset < 0 || x.Length < 0 || x.Length > math.MaxInt64-x.Offset {
			return nil, fmt.Errorf("an extent of %d bytes at %d", x.Length, x.Offset)
		}
		path, err := e.inside(x.Path)
		if err != nil {
			return nil, err
		}
		size, known := sizes[path]
		if !known {
			st, err := os.Stat(path)
			if err != nil {
				return nil, err
			}
			if !st.Mode().IsRegular() {
				return nil, fmt.Errorf("%s isn't a file", path)
			}
			size = st.Size()
			sizes[path] = size
		}
		// A damaged image can stop short of what it says it holds. There is nothing to
		// read past its end, and that isn't the helper's doing.
		if x.Offset > size {
			x.Offset = size
		}
		if x.Length > size-x.Offset {
			x.Length = size - x.Offset
		}
		out = append(out, wittybluray.FileExtent{Path: path, Offset: x.Offset, Length: x.Length})
	}
	return out, nil
}

// inside says whether a path is the opened image, or a file in the opened folder, and
// returns it as the server knows it.
func (e *engine) inside(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("the path %q isn't on the disc", p)
	}
	clean := filepath.Clean(p)
	if e.image {
		if clean == e.path || (runtime.GOOS == "windows" && strings.EqualFold(clean, e.path)) {
			return e.path, nil
		}
		return "", fmt.Errorf("the path %q isn't on the disc", p)
	}
	rel, err := filepath.Rel(e.root, clean)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("the path %q isn't on the disc", p)
	}
	path := filepath.Join(e.root, rel)
	// A link inside the folder is the same thing as a path outside it.
	followed, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if rel, err := filepath.Rel(e.real, followed); err != nil || rel == "." || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("the path %q leads outside the disc", p)
	}
	return path, nil
}

// safeName: the names a disc's tables use for clips and objects, which end up in
// addresses and in the engine's own lookups. Empty means none.
func safeName(s string) bool {
	if len(s) > 16 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// checkPlaylist checks what the session indexes by or uses as a name without looking.
func checkPlaylist(pl *wittybluray.Playlist, n int) error {
	if pl.Number != n {
		return fmt.Errorf("asked for playlist %d, it sent %d", n, pl.Number)
	}
	for i := range pl.Items {
		it := &pl.Items[i]
		if !safeName(it.Clip) {
			return fmt.Errorf("a play item names the clip %q", it.Clip)
		}
		for _, a := range it.Angles {
			if !safeName(a.Clip) {
				return fmt.Errorf("an angle names the clip %q", a.Clip)
			}
		}
		for _, list := range [][]wittybluray.Stream{it.Video, it.Audio, it.PG, it.IG, it.SecAudio, it.SecVideo, it.PiPPG, it.DV} {
			for j := range list {
				if list[j].SubPath < 0 || list[j].SubClip < 0 {
					return errors.New("a stream names a sub-path that can't exist")
				}
			}
		}
	}
	for i := range pl.SubPaths {
		for j := range pl.SubPaths[i].Items {
			sp := &pl.SubPaths[i].Items[j]
			if !safeName(sp.Clip) {
				return fmt.Errorf("a sub play item names the clip %q", sp.Clip)
			}
			for _, a := range sp.MultiClips {
				if !safeName(a.Clip) {
					return fmt.Errorf("a sub play item names the clip %q", a.Clip)
				}
			}
		}
	}
	return nil
}

// checkSubtitles checks the pictures, which are encoded here.
func checkSubtitles(w *wittybluray.SubtitleWindow) error {
	for i := range w.Subtitles {
		m := w.Subtitles[i].Image
		if m == nil {
			return errors.New("a subtitle without a picture")
		}
		dx, dy := m.Rect.Dx(), m.Rect.Dy()
		if m.Rect.Min.X != 0 || m.Rect.Min.Y != 0 || dx <= 0 || dy <= 0 || dx > maxSubPicture || dy > maxSubPicture || m.Stride != dx*4 || len(m.Pix) != m.Stride*dy {
			return fmt.Errorf("a subtitle picture of the wrong size (%v)", m.Rect)
		}
	}
	return nil
}
