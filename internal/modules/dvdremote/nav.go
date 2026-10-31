package dvdremote

import (
	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittydvd/wire"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
)

// nav is a navigator in the helper. Once the helper has gone it is the built-in
// copy's, started again from the disc's first-play.
type nav struct {
	r     *remote
	clone bool // the what-if copy of the navigator; see Clone
	local dvdsession.Navigator
	regs  wittydvd.Registers // as the helper last said
	pgcs  map[pgcKey]*wittydvd.PGC
}

type pgcKey struct {
	domain    wittydvd.Domain
	vts, pgcn int
}

// ask makes one navigator call. The registers and the highest serial follow every answer.
func (n *nav) ask(method string, params any) (*wire.NavAnswer, error) {
	blob, err := n.r.h.Call(method, params, nil)
	if err != nil {
		return nil, err
	}
	a, err := wire.DecodeNav(blob)
	if err != nil {
		return nil, &badAnswer{err}
	}
	if err := checkStep(a.Step); err != nil {
		return nil, err
	}
	n.regs = a.Regs
	clear(n.pgcs)
	if a.Step != nil && a.Step.Serial > n.r.lastSerial {
		n.r.lastSerial = a.Step.Serial
	}
	return a, nil
}

// fromHere keeps a step's serial past every one the helper gave: a session takes the
// same serial to mean the same screen.
func (n *nav) fromHere(st *wittydvd.Step) *wittydvd.Step {
	if st == nil {
		return nil
	}
	c := *st
	c.Serial += n.r.lastSerial + 1
	return &c
}

// step makes a call that moves the disc on. If the helper fails, the viewer goes to
// the disc's first-play instead, with the built-in copy.
func (n *nav) step(method string, c wire.NavCall, local func(dvdsession.Navigator) *wittydvd.Step) *wittydvd.Step {
	if n.r.local == nil {
		c.Clone = n.clone
		a, err := n.ask(method, c)
		if err == nil {
			return a.Step
		}
		n.r.giveUp(err)
	}
	if n.clone {
		if n.local == nil || n.r.restart {
			return nil // a look ahead that went with the helper
		}
		return n.fromHere(local(n.local))
	}
	if n.r.restart {
		n.r.restart = false
		return n.fromHere(n.local.Start())
	}
	return n.fromHere(local(n.local))
}

// set makes a call that only changes a setting.
func (n *nav) set(method string, c wire.NavCall, local func(dvdsession.Navigator)) {
	if n.r.local == nil {
		c.Clone = n.clone
		_, err := n.ask(method, c)
		if err == nil {
			return
		}
		n.r.giveUp(err)
	}
	if n.local != nil {
		local(n.local)
	}
}

// Start runs first-play.
func (n *nav) Start() *wittydvd.Step {
	return n.step(wire.MethodStart, wire.NavCall{}, dvdsession.Navigator.Start)
}

// PlayTitle jumps to a title and chapter.
func (n *nav) PlayTitle(ttn, pttn int) *wittydvd.Step {
	return n.step(wire.MethodPlayTitle, wire.NavCall{N: ttn, M: pttn}, func(l dvdsession.Navigator) *wittydvd.Step { return l.PlayTitle(ttn, pttn) })
}

// OpenMenu starts in one of the disc's menus.
func (n *nav) OpenMenu(kind string, vts int) *wittydvd.Step {
	return n.step(wire.MethodOpenMenu, wire.NavCall{Kind: kind, N: vts}, func(l dvdsession.Navigator) *wittydvd.Step { return l.OpenMenu(kind, vts) })
}

// SelectButton moves the highlight.
func (n *nav) SelectButton(btn int) *wittydvd.Step {
	return n.step(wire.MethodSelect, wire.NavCall{N: btn}, func(l dvdsession.Navigator) *wittydvd.Step { return l.SelectButton(btn) })
}

// PressButton activates a button.
func (n *nav) PressButton(btn int, cmd *wittydvd.Command) *wittydvd.Step {
	return n.step(wire.MethodPress, wire.NavCall{N: btn, Cmd: cmd}, func(l dvdsession.Navigator) *wittydvd.Step { return l.PressButton(btn, cmd) })
}

// MenuKey is the remote control's menu keys.
func (n *nav) MenuKey(kind string) *wittydvd.Step {
	return n.step(wire.MethodMenuKey, wire.NavCall{Kind: kind}, func(l dvdsession.Navigator) *wittydvd.Step { return l.MenuKey(kind) })
}

// Resume goes back to the title a menu was called from.
func (n *nav) Resume() *wittydvd.Step {
	return n.step(wire.MethodResume, wire.NavCall{}, dvdsession.Navigator.Resume)
}

// NextChapter skips forward.
func (n *nav) NextChapter() *wittydvd.Step {
	return n.step(wire.MethodNext, wire.NavCall{}, dvdsession.Navigator.NextChapter)
}

// PrevChapter skips back.
func (n *nav) PrevChapter() *wittydvd.Step {
	return n.step(wire.MethodPrev, wire.NavCall{}, dvdsession.Navigator.PrevChapter)
}

// GoUp is the remote control's return key.
func (n *nav) GoUp() *wittydvd.Step {
	return n.step(wire.MethodGoUp, wire.NavCall{}, dvdsession.Navigator.GoUp)
}

// CellEnded says the shown cell has finished.
func (n *nav) CellEnded() *wittydvd.Step {
	return n.step(wire.MethodCellEnded, wire.NavCall{}, dvdsession.Navigator.CellEnded)
}

// Tick advances the disc's clocks.
func (n *nav) Tick(seconds float64) *wittydvd.Step {
	return n.step(wire.MethodTick, wire.NavCall{Sec: seconds}, func(l dvdsession.Navigator) *wittydvd.Step { return l.Tick(seconds) })
}

// Current is the last step.
func (n *nav) Current() *wittydvd.Step {
	return n.step(wire.MethodCurrent, wire.NavCall{}, dvdsession.Navigator.Current)
}

// SetAudio chooses an audio stream.
func (n *nav) SetAudio(stream int) {
	n.set(wire.MethodSetAudio, wire.NavCall{N: stream}, func(l dvdsession.Navigator) { l.SetAudio(stream) })
}

// SetSubpicture chooses a subtitle stream.
func (n *nav) SetSubpicture(stream int, on bool) {
	n.set(wire.MethodSetSubs, wire.NavCall{N: stream, On: on}, func(l dvdsession.Navigator) { l.SetSubpicture(stream, on) })
}

// SetAngle chooses a camera angle.
func (n *nav) SetAngle(a int) {
	n.set(wire.MethodSetAngle, wire.NavCall{N: a}, func(l dvdsession.Navigator) { l.SetAngle(a) })
}

// SyncTitleCell says which cell of the title the viewer has reached.
func (n *nav) SyncTitleCell(pgcn, cellN int) bool {
	if n.r.local == nil {
		a, err := n.ask(wire.MethodSyncCell, wire.NavCall{Clone: n.clone, N: pgcn, M: cellN})
		if err == nil {
			return a.Synced
		}
		n.r.giveUp(err)
	}
	return n.local != nil && n.local.SyncTitleCell(pgcn, cellN)
}

// Registers are as the helper last said, so reading them costs nothing. The pointer
// stays good, as the built-in copy's does.
func (n *nav) Registers() *wittydvd.Registers {
	if n.local != nil {
		return n.local.Registers()
	}
	return &n.regs
}

// StepPGC is the program chain a step plays, as the navigator resolves it. A session
// asks several times for each step, and the answer holds until the disc moves on.
func (n *nav) StepPGC(st *wittydvd.Step) *wittydvd.PGC {
	if st == nil {
		return nil
	}
	if n.r.local == nil {
		k := pgcKey{st.Domain, st.VTS, st.PGCN}
		if p, ok := n.pgcs[k]; ok {
			return p
		}
		p, err := n.askPGC(st)
		if err == nil {
			if n.pgcs == nil {
				n.pgcs = map[pgcKey]*wittydvd.PGC{}
			}
			n.pgcs[k] = p
			return p
		}
		n.r.giveUp(err)
	}
	if n.local != nil {
		return n.local.StepPGC(st)
	}
	return nil
}

func (n *nav) askPGC(st *wittydvd.Step) (*wittydvd.PGC, error) {
	blob, err := n.r.h.Call(wire.MethodStepPGC, wire.NavCall{Clone: n.clone, Domain: st.Domain, N: st.VTS, M: st.PGCN}, nil)
	if err != nil {
		return nil, err
	}
	p, err := wire.DecodePGC(blob)
	if err != nil {
		return nil, &badAnswer{err}
	}
	if p != nil {
		if err := checkPGC(p); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// Clone is a copy for looking ahead. The helper keeps only the latest, as a session
// asks one question of each and then forgets it, with nothing to say so.
func (n *nav) Clone() dvdsession.Navigator {
	c := &nav{r: n.r, clone: true}
	if n.r.local == nil {
		a, err := n.ask(wire.MethodClone, wire.NavCall{})
		if err == nil {
			c.regs = a.Regs
			return c
		}
		n.r.giveUp(err)
	}
	if n.local != nil {
		c.local = n.local.Clone()
	}
	return c
}
