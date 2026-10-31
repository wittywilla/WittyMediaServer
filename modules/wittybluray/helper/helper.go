// Package helper is WittyBluray's side of running as a helper process: one opened
// disc and its navigator, answering the server's calls. Video never comes through
// here: the server reads the disc itself from the extents it is told about.
//
// The navigator's answers are JSON, and everything else is gob, both sent as the
// protocol's blob. Gob suits the large structures: pictures go as raw bytes, and a
// field one side doesn't know is ignored. It can't carry a pointer to a zero value,
// though, and "stills off" in a step's effects is exactly that.
package helper

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittymodule"
)

// Params is every call's parameters; each method reads the ones it needs.
type Params struct {
	Path     string                 `json:"path,omitempty"`
	Options  wittybluray.NavOptions `json:"options"`
	N        int                    `json:"n,omitempty"`
	Name     string                 `json:"name,omitempty"`
	Item     int                    `json:"item,omitempty"`
	Time     uint32                 `json:"time,omitempty"`
	ItemOnly bool                   `json:"itemOnly,omitempty"`
	Shown    bool                   `json:"shown,omitempty"`
	PID      uint16                 `json:"pid,omitempty"`
	Limit    uint32                 `json:"limit,omitempty"`
	STC      int                    `json:"stc,omitempty"`
	From     uint32                 `json:"from,omitempty"`
	To       uint32                 `json:"to,omitempty"`
	CharCode int                    `json:"charCode,omitempty"`
	UO       uint64                 `json:"uo,omitempty"`
	Page     int                    `json:"page,omitempty"`
	Button   int                    `json:"button,omitempty"`
	Commands []wittybluray.Command  `json:"commands,omitempty"`
}

// Opened is the answer to "open".
type Opened struct {
	Encrypted bool        `json:"encrypted"`
	Stereo3D  bool        `json:"stereo3d"`
	PSR       [128]uint32 `json:"psr"`
}

// Main is the answer to "main": the number of the feature's playlist.
type Main struct {
	N  int  `json:"n"`
	OK bool `json:"ok"`
}

// Answer is what a call to the navigator comes back with. PSR and the timer are
// what the session reads without asking, so every answer brings them up to date.
// General registers stay here: only the Java side reads those, and it isn't run
// through a helper.
type Answer struct {
	PSR [128]uint32 `json:"psr"`
	// TimerSeq changes when the disc starts a timer, even one the same as before:
	// the session restarts its clock when the timer is a new one.
	Timer    *wittybluray.NavTimer `json:"timer,omitempty"`
	TimerSeq uint64                `json:"timerSeq"`
	// Step is without its playlist, which Playlist names: the server keeps each
	// playlist once, and a session compares them as pointers.
	Step     *wittybluray.Step `json:"step,omitempty"`
	Playlist *int              `json:"playlist,omitempty"`
	OK       bool              `json:"ok,omitempty"`
}

// Helper holds one opened disc.
type Helper struct {
	disc     *wittybluray.Disc
	nav      *wittybluray.Nav
	timer    *wittybluray.NavTimer
	timerSeq uint64
}

// Handle answers one call. wittymodule.Serve calls it for one call at a time.
func (h *Helper) Handle(method string, raw json.RawMessage) (any, []byte, error) {
	var p Params
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, nil, &wittymodule.CallError{Code: "bad-call", Message: "the call's parameters are unreadable"}
		}
	}
	if method == "open" {
		return h.open(p)
	}
	if h.disc == nil {
		return nil, nil, &wittymodule.CallError{Code: "not-open", Message: "no disc is open"}
	}
	d, nav := h.disc, h.nav
	switch method {
	case "playlist":
		return send(d.Playlist(p.N), nil)
	case "clip":
		return send(d.Clip(p.Name), nil)
	case "main":
		if pl := d.MainPlaylist(); pl != nil {
			return Main{N: pl.Number, OK: true}, nil, nil
		}
		return Main{}, nil, nil
	case "extents":
		ext, err := d.ClipExtents(p.Name)
		return send(&ext, err)
	case "sounds":
		return send(d.Sounds(), nil)
	case "menu":
		m, err := d.LoadMenu(p.Name, p.PID, p.Limit)
		return send(m, err)
	case "bdjobject":
		o, err := d.BDJObject(p.Name)
		return send(o, err)
	case "subtitles":
		w, err := d.SubtitlesIn(p.Name, p.PID, p.STC, p.From, p.To)
		return send(w, err)
	case "text":
		c, err := d.TextSubtitles(p.Name, p.PID, p.CharCode)
		return send(&c, err)

	case "start":
		return h.step(nav.Start())
	case "resume":
		return h.step(nav.Resume())
	case "menucall":
		return h.step(nav.MenuCall())
	case "playlistended":
		return h.step(nav.PlaylistEnded())
	case "navtimerfired":
		return h.step(nav.NavTimerFired())
	case "playtitle":
		return h.step(nav.PlayTitle(p.N))
	case "pressbutton":
		return h.step(nav.PressButton(p.Commands))
	case "playplaylist":
		pl := d.Playlist(p.N)
		if pl == nil {
			return nil, nil, &wittymodule.CallError{Code: "bad-call", Message: fmt.Sprintf("the disc has no playlist %d", p.N)}
		}
		return h.step(nav.PlayPlaylist(pl, p.Item, p.Time))
	case "skipitem":
		st, ok := nav.SkipItem(p.N)
		return h.answer(st, ok)

	case "setsubtitle":
		nav.SetSubtitle(p.N, p.Shown)
	case "setaudio":
		nav.SetAudio(p.N)
	case "setangle":
		nav.SetAngle(p.N)
	case "settime":
		nav.SetTime(p.Item, p.Time)
	case "setmenustate":
		nav.SetMenuState(p.Page, p.Button)
	case "setchapter":
		nav.SetChapter(p.N)
	case "forbids":
		return h.answer(nil, nav.Forbids(p.UO))
	case "loopsatend":
		st := &wittybluray.Step{Playlist: d.Playlist(p.N), Item: p.Item, Time: p.Time, ItemOnly: p.ItemOnly}
		return h.answer(nil, nav.LoopsAtEnd(st))
	default:
		return nil, nil, &wittymodule.CallError{Code: "unknown-method", Message: fmt.Sprintf("this version has no %q", method)}
	}
	return h.answer(nil, false)
}

func (h *Helper) open(p Params) (any, []byte, error) {
	d, err := wittybluray.Open(p.Path)
	if err != nil {
		return nil, nil, coded(err)
	}
	if h.disc != nil {
		_ = h.disc.Close()
	}
	h.disc, h.nav, h.timer = d, wittybluray.NewNav(d, p.Options), nil
	return Opened{Encrypted: d.Encrypted, Stereo3D: d.Stereo3D, PSR: h.nav.Registers().PSR}, nil, nil
}

func (h *Helper) step(st *wittybluray.Step) (any, []byte, error) { return h.answer(st, false) }

// answer sends a step, with the state the server keeps a copy of.
func (h *Helper) answer(st *wittybluray.Step, ok bool) (any, []byte, error) {
	if t := h.nav.Timer(); t != h.timer {
		h.timer = t
		h.timerSeq++
	}
	a := Answer{PSR: h.nav.Registers().PSR, Timer: h.timer, TimerSeq: h.timerSeq, OK: ok}
	if st != nil {
		c := *st
		if st.Playlist != nil {
			n := st.Playlist.Number
			a.Playlist = &n
			c.Playlist = nil
		}
		a.Step = &c
	}
	b, err := json.Marshal(&a)
	return nil, b, err
}

// send answers with gob, or with nothing when there is nothing to send.
func send[T any](v *T, err error) (any, []byte, error) {
	if err != nil {
		return nil, nil, coded(err)
	}
	if v == nil {
		return nil, nil, nil
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		return nil, nil, &wittymodule.CallError{Code: "panic", Message: "sending the answer: " + err.Error()}
	}
	return nil, buf.Bytes(), nil
}

// coded names the errors the server acts on.
func coded(err error) error {
	code := ""
	if errors.Is(err, fs.ErrPermission) {
		code = "denied" // the helper's limits may be why, so the server tries itself
	}
	return &wittymodule.CallError{Code: code, Message: err.Error()}
}
