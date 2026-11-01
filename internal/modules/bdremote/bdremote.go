// Package bdremote runs a Blu-ray session's engine in a helper process, of the WittyBluray
// version in use.
//
// A helper may be someone's fork, so nothing it says is taken on trust: where files
// are, how large things are and what its structures hold are all checked here before
// the session sees them, and whether a disc is encrypted is the server's own finding
// (bdsession.openEngine). Video never passes through a helper, and FFmpeg reads only
// the disc the viewer opened.
//
// A helper that fails, or that sends something those checks refuse, ends the disc: the viewer
// sees why, and opening it again starts a new helper.
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

	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/bluray"
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

// Use makes Blu-ray sessions use the version of WittyBluray the manager has in use.
func Use(m *modules.Manager) {
	bdsession.RemoteEngine = func(path string, o bluray.NavOptions, log *slog.Logger) (*bdsession.Remote, error) {
		a := m.Attach(moduleID, callLimit)
		if a == nil {
			return nil, m.Missing(moduleID)
		}
		return open(a, path, o, log, func(reason string) { _ = m.ReportFailure(moduleID, a.Version(), reason) })
	}
	bdsession.JavaProbe = func(path string) (version, where string, err error) {
		a := m.Attach(moduleID, javaProbeLimit)
		if a == nil {
			return "", "", m.Missing(moduleID)
		}
		return probe(a, path)
	}
}

// javaProbeLimit is how long Java may take to say whether it can run a disc's programs.
const javaProbeLimit = 30 * time.Second

// process is the helper as this package uses it: modules.Attached, or a test's.
type process interface {
	Call(method string, params, out any) ([]byte, error)
	Close()
}

// engine is one session's disc and navigator in a helper. mu is held through each
// call, and the protocol has one call at a time anyway.
type engine struct {
	mu     sync.Mutex
	h      process // nil once the helper has gone
	path   string
	image  bool   // an image file; otherwise a folder
	root   string // the folder extents must lie in
	real   string // the same with its links followed
	opts   bluray.NavOptions
	log    *slog.Logger
	report func(reason string) // a failure the helper's own calls can't have counted
	angle  int

	// What the session reads as often as it likes. The registers stay the same object:
	// a session may hold them across a call that changes them.
	regs     *bluray.Registers
	timer    *bluray.NavTimer
	timerSeq uint64

	// Each structure is fetched once. A playlist is the same pointer every time.
	playlists map[int]*bluray.Playlist
	clips     map[string]*bluray.Clip
	extents   map[string][]bluray.FileExtent
	main      *int
	sounds    *bluray.Sounds
	hasSounds bool
}

// badAnswer is a helper saying something that can't be right. The protocol can't have
// counted that as a failure, as it was a good answer as far as the protocol could tell.
type badAnswer struct{ err error }

func (b *badAnswer) Error() string { return "it sent a bad answer: " + b.err.Error() }

// open asks a helper to open a disc.
func open(h process, path string, o bluray.NavOptions, log *slog.Logger, report func(string)) (*bdsession.Remote, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	e, err := newEngine(h, path, o, log, report)
	if err == nil {
		var info bluray.Opened
		if _, err = h.Call("open", bluray.Params{Path: e.path, Options: o}, &info); err == nil {
			e.regs.PSR = info.PSR
			return &bdsession.Remote{Disc: e, Nav: e, Encrypted: info.Encrypted, Stereo3D: info.Stereo3D, Label: clipText(info.Label), Index: info.Index, BDJ: info.BDJ, Playlists: info.Playlists}, nil
		}
	}
	h.Close()
	return nil, err
}

func clipText(s string) string {
	if len(s) > 256 {
		return s[:256]
	}
	return s
}

func newEngine(h process, path string, o bluray.NavOptions, log *slog.Logger, report func(string)) (*engine, error) {
	abs, err := filepath.Abs(path) // the helper runs in a folder of its own
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	e := &engine{h: h, path: abs, image: !st.IsDir(), opts: o, log: log, report: report, regs: bluray.NewRegisters(o.Registers),
		playlists: map[int]*bluray.Playlist{}, clips: map[string]*bluray.Clip{}, extents: map[string][]bluray.FileExtent{}}
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

// fail ends the disc. Call with e.mu held and a helper in use.
func (e *engine) fail(err error) {
	e.log.Warn("WittyBluray: the helper failed; this disc has stopped", "cat", "dvd", "err", err)
	var bad *badAnswer
	var ce *wittymodule.CallError
	switch {
	case errors.As(err, &bad):
		e.report(err.Error())
	case errors.As(err, &ce) && ce.Code != "denied": // "denied" may be its limits, not its fault
		e.report(ce.Error())
	}
	e.h.Close()
	e.h = nil
	e.playlists, e.clips, e.extents, e.sounds = nil, nil, nil, nil
}

// errGone: the helper has gone, and with it the disc.
var errGone = errors.New("WittyBluray stopped working on this disc. Open it again to start over")

// stopped is the step that says so.
func stopped() *bluray.Step {
	return &bluray.Step{Kind: bluray.StepStop, Reason: errGone.Error(), Object: -1}
}

// answeredNo: a helper in good order, saying this one thing can't be done.
func answeredNo(err error) bool {
	var ce *wittymodule.CallError
	return errors.As(err, &ce) && ce.Code == ""
}

// answerLimits are what the server reads of an answer before decoding it. Gob isn't
// hardened against a stranger's bytes: a playlist of empty play items grows 1500 times
// as it decodes, which the protocol's own limit doesn't come close to stopping. They are
// far above what real discs need (the largest playlist measured is 6 KB, the largest clip
// 75 KB and its extents 88 KB); beyond them the helper is at fault. Menus, sounds and
// subtitles hold pictures and samples, so they keep the protocol's limit.
var answerLimits = map[string]int{
	"playlist":  512 << 10,
	"clip":      4 << 20,
	"extents":   4 << 20,
	"cliplead":  4 << 10,
	"bdjobject": 1 << 20,
	"text":      16 << 20,
}

// decode reads gob from a helper. The limits above and the protocol's have already
// limited the answer, and this keeps a decoder's panic from becoming the server's.
func decode(blob []byte, v any) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%v", p)
		}
	}()
	return gob.NewDecoder(bytes.NewReader(blob)).Decode(v)
}

// fetch asks for a structure. nil with no error means the helper says there is none.
func fetch[T any](e *engine, method string, p bluray.Params, check func(*T) error) (*T, error) {
	blob, err := e.h.Call(method, p, nil)
	if err != nil {
		return nil, err
	}
	if len(blob) == 0 {
		return nil, nil
	}
	if limit, ok := answerLimits[method]; ok && len(blob) > limit {
		return nil, &badAnswer{fmt.Errorf("its answer to %s is too large: %d bytes", method, len(blob))}
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

// must is fetch for what a helper must answer: a refusal is passed on as an error, and
// anything else ends the disc. Call with e.mu held.
func must[T any](e *engine, method string, p bluray.Params, check func(*T) error) (*T, error) {
	if e.h == nil {
		return nil, errGone
	}
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
	return nil, errGone
}

// ---------------------------------------------------------------- the disc

func (e *engine) Playlist(n int) *bluray.Playlist {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.playlist(n)
}

func (e *engine) playlist(n int) *bluray.Playlist {
	if e.h == nil {
		return nil
	}
	if pl, ok := e.playlists[n]; ok {
		return pl
	}
	pl, err := fetch(e, "playlist", bluray.Params{N: n}, func(pl *bluray.Playlist) error { return checkPlaylist(pl, n) })
	if err != nil {
		e.fail(err)
		return nil
	}
	if pl != nil {
		e.playlists[n] = pl
	}
	return pl
}

func (e *engine) MainPlaylist() *bluray.Playlist {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == nil {
		return nil
	}
	if e.main == nil {
		var m bluray.Main
		if _, err := e.h.Call("main", bluray.Params{}, &m); err != nil {
			e.fail(err)
			return nil
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
	return pl
}

// PlaylistDuration is arithmetic on the playlist, which needs no disc.
func (e *engine) PlaylistDuration(pl *bluray.Playlist) float64 {
	return bluray.PlaylistDuration(pl)
}

func (e *engine) Clip(name string) *bluray.Clip {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == nil {
		return nil
	}
	if c, ok := e.clips[name]; ok {
		return c
	}
	c, err := fetch(e, "clip", bluray.Params{Name: name}, func(c *bluray.Clip) error {
		if c.Name != name {
			return fmt.Errorf("asked for clip %q, it sent %q", name, c.Name)
		}
		return nil
	})
	if err != nil {
		e.fail(err)
		return nil
	}
	if c != nil {
		e.clips[name] = c
	}
	return c
}

func (e *engine) ClipExtents(name string) ([]bluray.FileExtent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h != nil {
		if out, ok := e.extents[name]; ok {
			return out, nil
		}
	}
	got, err := must(e, "extents", bluray.Params{Name: name}, func(ext *[]bluray.FileExtent) error {
		checked, err := e.checkExtents(*ext)
		*ext = checked
		return err
	})
	if err != nil {
		return nil, err
	}
	e.extents[name] = *got
	return *got, nil
}

// ClipLead says how far ahead of a clip's first picture its first packet is. Zero is
// always safe: the join is only as exact as it was before.
func (e *engine) ClipLead(clip string, video uint16, from uint32, only ...uint16) float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == nil {
		return 0
	}
	got, err := fetch(e, "cliplead", bluray.Params{Name: clip, PID: video, From: from, Only: only}, func(l *bluray.Lead) error {
		if !(l.Seconds >= 0 && l.Seconds <= maxClipLead) {
			return fmt.Errorf("a clip's lead of %v seconds", l.Seconds)
		}
		return nil
	})
	if err == nil && got != nil {
		return got.Seconds
	}
	if err == nil {
		err = &badAnswer{errors.New("cliplead came back empty")}
	}
	if !answeredNo(err) {
		e.fail(err)
	}
	return 0
}

func (e *engine) Sounds() *bluray.Sounds {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == nil {
		return nil
	}
	if e.hasSounds {
		return e.sounds
	}
	s, err := fetch[bluray.Sounds](e, "sounds", bluray.Params{}, nil)
	if err != nil {
		e.fail(err)
		return nil
	}
	e.sounds, e.hasSounds = s, true
	return s
}

func (e *engine) LoadMenu(clip string, pid uint16, limit uint32) (*bluray.Menu, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return must[bluray.Menu](e, "menu", bluray.Params{Name: clip, PID: pid, Limit: limit}, nil)
}

func (e *engine) BDJObject(name string) (*bluray.BDJObject, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return must[bluray.BDJObject](e, "bdjobject", bluray.Params{Name: name}, nil)
}

func (e *engine) SubtitlesIn(clip string, pid uint16, stc int, from, to uint32) (*bluray.SubtitleWindow, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return must(e, "subtitles", bluray.Params{Name: clip, PID: pid, STC: stc, From: from, To: to}, checkSubtitles)
}

func (e *engine) TextSubtitles(clip string, pid uint16, charCode int) ([]bluray.TextCue, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	got, err := must[[]bluray.TextCue](e, "text", bluray.Params{Name: clip, PID: pid, CharCode: charCode}, nil)
	if err != nil {
		return nil, err
	}
	return *got, nil
}

// Close stops the helper.
func (e *engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h != nil {
		e.h.Close()
		e.h = nil
	}
	return nil
}

// ---------------------------------------------------------------- the navigator

// ask makes one navigator call and brings the copy of its state up to date. Call with
// e.mu held and a helper in use.
func (e *engine) ask(method string, p bluray.Params) (*bluray.Answer, error) {
	blob, err := e.h.Call(method, p, nil)
	if err != nil {
		return nil, err
	}
	a := new(bluray.Answer)
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
func (e *engine) checkedStep(a *bluray.Answer) (*bluray.Step, error) {
	st := a.Step
	if st == nil {
		return nil, &badAnswer{errors.New("no step")}
	}
	switch st.Kind {
	case bluray.StepPlay, bluray.StepStop, bluray.StepUnsupported, bluray.StepStay:
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
	// Only by its number, through the checks: a playlist written into the step has had none.
	st.Playlist = nil
	if a.Playlist != nil {
		n := *a.Playlist
		if st.Playlist = e.playlist(n); st.Playlist == nil && e.h != nil {
			return nil, &badAnswer{fmt.Errorf("it asked to play playlist %d, which it doesn't have", n)}
		}
	}
	if st.Kind == bluray.StepPlay && st.Playlist == nil && e.h != nil {
		return nil, &badAnswer{errors.New("nothing to play in a step to play")}
	}
	return st, nil
}

// step asks for the next step. With the helper gone, it's the stop that says why.
func (e *engine) step(method string, p bluray.Params) *bluray.Step {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == nil {
		return stopped()
	}
	a, err := e.ask(method, p)
	var st *bluray.Step
	if err == nil {
		st, err = e.checkedStep(a)
	}
	if err == nil && e.h != nil {
		return st
	}
	if e.h != nil { // fetching the step's playlist may have ended it already
		e.fail(err)
	}
	return stopped()
}

// tell tells the navigator something.
func (e *engine) tell(method string, p bluray.Params) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == nil {
		return
	}
	if _, err := e.ask(method, p); err != nil {
		e.fail(err)
	}
}

func (e *engine) Registers() *bluray.Registers { return e.regs }

func (e *engine) Timer() *bluray.NavTimer {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == nil {
		return nil
	}
	return e.timer
}

func (e *engine) Start() *bluray.Step { return e.step("start", bluray.Params{}) }

func (e *engine) Resume() *bluray.Step { return e.step("resume", bluray.Params{}) }

func (e *engine) MenuCall() *bluray.Step { return e.step("menucall", bluray.Params{}) }

func (e *engine) PlaylistEnded() *bluray.Step { return e.step("playlistended", bluray.Params{}) }

func (e *engine) NavTimerFired() *bluray.Step { return e.step("navtimerfired", bluray.Params{}) }

func (e *engine) PlayTitle(t int) *bluray.Step { return e.step("playtitle", bluray.Params{N: t}) }

func (e *engine) PressButton(cmds []bluray.Command) *bluray.Step {
	return e.step("pressbutton", bluray.Params{Commands: cmds})
}

func (e *engine) PlayPlaylist(pl *bluray.Playlist, item int, t uint32) *bluray.Step {
	return e.step("playplaylist", bluray.Params{N: pl.Number, Item: item, Time: t})
}

func (e *engine) SkipItem(delta int) (*bluray.Step, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == nil {
		return stopped(), true
	}
	a, err := e.ask("skipitem", bluray.Params{N: delta})
	if err == nil && !a.OK {
		return nil, false
	}
	var st *bluray.Step
	if err == nil {
		st, err = e.checkedStep(a)
	}
	if err == nil && e.h != nil {
		return st, true
	}
	if e.h != nil {
		e.fail(err)
	}
	return stopped(), true
}

func (e *engine) SetSubtitle(s int, shown bool) {
	e.tell("setsubtitle", bluray.Params{N: s, Shown: shown})
}

func (e *engine) SetAudio(s int) { e.tell("setaudio", bluray.Params{N: s}) }

func (e *engine) SetAngle(a int) {
	e.mu.Lock()
	e.angle = a
	e.mu.Unlock()
	e.tell("setangle", bluray.Params{N: a})
}

func (e *engine) SetTime(item int, t uint32) { e.tell("settime", bluray.Params{Item: item, Time: t}) }

func (e *engine) SetChapter(c int) { e.tell("setchapter", bluray.Params{N: c}) }

// SetMenuState runs on every action a viewer takes, and usually changes nothing.
func (e *engine) SetMenuState(page, button int) {
	e.mu.Lock()
	unchanged := e.h != nil && e.regs.PSR[bluray.PSRPage] == uint32(page) && e.regs.PSR[bluray.PSRButton] == uint32(button)
	e.mu.Unlock()
	if unchanged {
		return
	}
	e.tell("setmenustate", bluray.Params{Page: page, Button: button})
}

func (e *engine) LoopsAtEnd(st *bluray.Step) bool {
	if st == nil || st.Playlist == nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == nil {
		return false
	}
	a, err := e.ask("loopsatend", bluray.Params{N: st.Playlist.Number, Item: st.Item, Time: st.Time, ItemOnly: st.ItemOnly})
	if err != nil {
		e.fail(err)
		return false
	}
	return a.OK
}

func (e *engine) Forbids(uo uint64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == nil {
		return false
	}
	a, err := e.ask("forbids", bluray.Params{UO: uo})
	if err != nil {
		e.fail(err)
		return false
	}
	return a.OK
}

// ---------------------------------------------------------------- checks

// checkExtents makes sure FFmpeg will only be pointed at the disc that was opened,
// and at no more of it than is there. The paths in the result are the server's own.
func (e *engine) checkExtents(in []bluray.FileExtent) ([]bluray.FileExtent, error) {
	if len(in) > maxExtents {
		return nil, fmt.Errorf("%d extents", len(in))
	}
	out := make([]bluray.FileExtent, 0, len(in))
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
		out = append(out, bluray.FileExtent{Path: path, Offset: x.Offset, Length: x.Length})
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
func checkPlaylist(pl *bluray.Playlist, n int) error {
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
		for _, list := range [][]bluray.Stream{it.Video, it.Audio, it.PG, it.IG, it.SecAudio, it.SecVideo, it.PiPPG, it.DV} {
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
func checkSubtitles(w *bluray.SubtitleWindow) error {
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
