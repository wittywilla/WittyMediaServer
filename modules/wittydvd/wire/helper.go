package wire

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"

	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittymodule"
)

// Helper is the helper process's side: one open disc and its navigator. The protocol
// makes one call at a time, so it needs no lock.
type Helper struct {
	opened *wittydvd.Opened
	nav    *wittydvd.Nav
	clone  *wittydvd.Nav // the latest what-if copy; nothing holds on to the one before
}

// Close lets go of the disc. A helper process has no need to: it ends with its stdin.
func (h *Helper) Close() {
	if h.opened != nil {
		_ = h.opened.Close()
		h.opened, h.nav, h.clone = nil, nil, nil
	}
}

func refuse(code, format string, a ...any) error {
	return &wittymodule.CallError{Code: code, Message: fmt.Sprintf(format, a...)}
}

// coded names the errors the server acts on.
func coded(err error) error {
	code := ""
	switch {
	case errors.Is(err, wittydvd.ErrNoNav):
		code = CodeNoNav
	case errors.Is(err, fs.ErrPermission):
		code = CodeDenied // the helper's limits may be why, so the server tries itself
	}
	return &wittymodule.CallError{Code: code, Message: err.Error()}
}

// Handle answers one call. It is a wittymodule.Handler.
func (h *Helper) Handle(method string, params json.RawMessage) (any, []byte, error) {
	blob, err := h.answer(method, params)
	return nil, blob, err
}

func (h *Helper) answer(method string, params json.RawMessage) ([]byte, error) {
	switch method {
	case MethodOpen:
		var p OpenParams
		if err := json.Unmarshal(params, &p); err != nil || p.Path == "" {
			return nil, refuse(CodeBadCall, "no disc was named")
		}
		if h.opened != nil {
			return nil, refuse(CodeBadCall, "a disc is already open")
		}
		o, err := wittydvd.Open(p.Path)
		if err != nil {
			return nil, coded(err)
		}
		h.opened = o
		return EncodeDisc(o.Disc)
	}
	if h.opened == nil {
		return nil, refuse(CodeBadCall, "no disc is open")
	}
	switch method {
	case MethodNav:
		var o NavOptions
		if err := json.Unmarshal(params, &o); err != nil {
			return nil, refuse(CodeBadCall, "unreadable options")
		}
		ro := wittydvd.RegisterOptions{MenuLanguage: o.MenuLanguage, AudioLanguage: o.AudioLanguage, SubtitleLanguage: o.SubtitleLanguage,
			Region: o.Region, Parental: o.Parental, Country: o.Country, Angle: o.Angle}
		if o.Seed != 0 {
			ro.Rand = rand.New(rand.NewSource(o.Seed)).Float64
		}
		h.nav = wittydvd.NewNav(h.opened.Disc, wittydvd.NavOptions{RegisterOptions: ro})
		h.clone = nil
		return Encode(&NavAnswer{Regs: *h.nav.Registers()})
	case MethodMenu:
		var c MenuCall
		if err := json.Unmarshal(params, &c); err != nil {
			return nil, refuse(CodeBadCall, "unreadable call")
		}
		pic, err := h.opened.Menu(c.Space, c.First, c.Last, c.Options)
		if err != nil {
			return nil, coded(err)
		}
		return EncodeMenu(pic)
	case MethodRange:
		var c RangeCall
		if err := json.Unmarshal(params, &c); err != nil {
			return nil, refuse(CodeBadCall, "unreadable call")
		}
		ext, err := h.opened.RangeExtents(c.Space, c.First, c.Last)
		if err != nil {
			return nil, coded(err)
		}
		return Encode(ext)
	case MethodRuns:
		var c RunsCall
		if err := json.Unmarshal(params, &c); err != nil {
			return nil, refuse(CodeBadCall, "unreadable call")
		}
		runs, err := h.opened.TitleRuns(c.Space, c.Cells)
		if err != nil {
			return nil, coded(err)
		}
		return Encode(runs)
	case MethodCell, MethodSubs:
		var c CellCall
		if err := json.Unmarshal(params, &c); err != nil {
			return nil, refuse(CodeBadCall, "unreadable call")
		}
		if method == MethodSubs {
			subs, err := h.opened.CellSubpictures(c.Space, c.Cell)
			if err != nil {
				return nil, coded(err)
			}
			return EncodeSubs(subs), nil
		}
		ext, err := h.opened.CellExtents(c.Space, c.Cell)
		if err != nil {
			return nil, coded(err)
		}
		return Encode(ext)
	}
	return h.navCall(method, params)
}

func (h *Helper) navCall(method string, params json.RawMessage) ([]byte, error) {
	var c NavCall
	if len(params) > 0 {
		if err := json.Unmarshal(params, &c); err != nil {
			return nil, refuse(CodeBadCall, "unreadable call")
		}
	}
	n := h.nav
	if c.Clone {
		n = h.clone
	}
	if n == nil {
		return nil, refuse(CodeBadCall, "that navigator isn't running")
	}
	var a NavAnswer
	switch method {
	case MethodStart:
		a.Step = n.Start()
	case MethodPlayTitle:
		a.Step = n.PlayTitle(c.N, c.M)
	case MethodOpenMenu:
		a.Step = n.OpenMenu(c.Kind, c.N)
	case MethodSelect:
		a.Step = n.SelectButton(c.N)
	case MethodPress:
		a.Step = n.PressButton(c.N, c.Cmd)
	case MethodMenuKey:
		a.Step = n.MenuKey(c.Kind)
	case MethodResume:
		a.Step = n.Resume()
	case MethodNext:
		a.Step = n.NextChapter()
	case MethodPrev:
		a.Step = n.PrevChapter()
	case MethodGoUp:
		a.Step = n.GoUp()
	case MethodCellEnded:
		a.Step = n.CellEnded()
	case MethodTick:
		a.Step = n.Tick(c.Sec)
	case MethodCurrent:
		a.Step = n.Current()
	case MethodSetAudio:
		n.SetAudio(c.N)
	case MethodSetSubs:
		n.SetSubpicture(c.N, c.On)
	case MethodSetAngle:
		n.SetAngle(c.N)
	case MethodSyncCell:
		a.Synced = n.SyncTitleCell(c.N, c.M)
	case MethodClone:
		h.clone = h.nav.Clone()
		n = h.clone
	case MethodStepPGC:
		return EncodePGC(n.StepPGC(&wittydvd.Step{Domain: c.Domain, VTS: c.N, PGCN: c.M}))
	default:
		return nil, refuse(CodeUnknownCall, "this version has no %q", method)
	}
	a.Regs = *n.Registers()
	return Encode(&a)
}
