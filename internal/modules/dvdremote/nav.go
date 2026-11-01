package dvdremote

import (
	"github.com/wittywilla/wittymediaserver/internal/dvd"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
)

// nav is a navigator in the helper. Once the helper has gone, the disc has stopped: the
// viewer sees that, and opening the disc again starts a new helper.
type nav struct {
	r     *remote
	clone bool // the what-if copy of the navigator; see Clone
	regs  dvd.Registers // as the helper last said
	pgcs  map[pgcKey]*dvd.PGC
}

type pgcKey struct {
	domain    dvd.Domain
	vts, pgcn int
}

// ask makes one navigator call. The registers and the highest serial follow every answer.
func (n *nav) ask(method string, params any) (*dvd.NavAnswer, error) {
	blob, err := n.r.h.Call(method, params, nil)
	if err != nil {
		return nil, err
	}
	a, err := dvd.DecodeNav(blob)
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

// step makes a call that moves the disc on. If the helper fails, the disc stops.
func (n *nav) step(method string, c dvd.NavCall) *dvd.Step {
	if n.r.dead == nil {
		c.Clone = n.clone
		a, err := n.ask(method, c)
		if err == nil {
			return a.Step
		}
		n.r.giveUp(err)
	}
	if n.clone {
		return nil // a look ahead that went with the helper
	}
	n.r.lastSerial++
	return &dvd.Step{Kind: dvd.StepStop, Serial: n.r.lastSerial}
}

// set makes a call that only changes a setting.
func (n *nav) set(method string, c dvd.NavCall) {
	if n.r.dead == nil {
		c.Clone = n.clone
		if _, err := n.ask(method, c); err != nil {
			n.r.giveUp(err)
		}
	}
}

// Start runs first-play.
func (n *nav) Start() *dvd.Step { return n.step(dvd.MethodStart, dvd.NavCall{}) }

// PlayTitle jumps to a title and chapter.
func (n *nav) PlayTitle(ttn, pttn int) *dvd.Step {
	return n.step(dvd.MethodPlayTitle, dvd.NavCall{N: ttn, M: pttn})
}

// OpenMenu starts in one of the disc's menus.
func (n *nav) OpenMenu(kind string, vts int) *dvd.Step {
	return n.step(dvd.MethodOpenMenu, dvd.NavCall{Kind: kind, N: vts})
}

// SelectButton moves the highlight.
func (n *nav) SelectButton(btn int) *dvd.Step {
	return n.step(dvd.MethodSelect, dvd.NavCall{N: btn})
}

// PressButton activates a button.
func (n *nav) PressButton(btn int, cmd *dvd.Command) *dvd.Step {
	return n.step(dvd.MethodPress, dvd.NavCall{N: btn, Cmd: cmd})
}

// MenuKey is the remote control's menu keys.
func (n *nav) MenuKey(kind string) *dvd.Step {
	return n.step(dvd.MethodMenuKey, dvd.NavCall{Kind: kind})
}

// Resume goes back to the title a menu was called from.
func (n *nav) Resume() *dvd.Step { return n.step(dvd.MethodResume, dvd.NavCall{}) }

// NextChapter skips forward.
func (n *nav) NextChapter() *dvd.Step { return n.step(dvd.MethodNext, dvd.NavCall{}) }

// PrevChapter skips back.
func (n *nav) PrevChapter() *dvd.Step { return n.step(dvd.MethodPrev, dvd.NavCall{}) }

// GoUp is the remote control's return key.
func (n *nav) GoUp() *dvd.Step { return n.step(dvd.MethodGoUp, dvd.NavCall{}) }

// CellEnded says the shown cell has finished.
func (n *nav) CellEnded() *dvd.Step { return n.step(dvd.MethodCellEnded, dvd.NavCall{}) }

// Tick advances the disc's clocks.
func (n *nav) Tick(seconds float64) *dvd.Step {
	return n.step(dvd.MethodTick, dvd.NavCall{Sec: seconds})
}

// Current is the last step.
func (n *nav) Current() *dvd.Step { return n.step(dvd.MethodCurrent, dvd.NavCall{}) }

// SetAudio chooses an audio stream.
func (n *nav) SetAudio(stream int) { n.set(dvd.MethodSetAudio, dvd.NavCall{N: stream}) }

// SetSubpicture chooses a subtitle stream.
func (n *nav) SetSubpicture(stream int, on bool) {
	n.set(dvd.MethodSetSubs, dvd.NavCall{N: stream, On: on})
}

// SetAngle chooses a camera angle.
func (n *nav) SetAngle(a int) { n.set(dvd.MethodSetAngle, dvd.NavCall{N: a}) }

// SyncTitleCell says which cell of the title the viewer has reached.
func (n *nav) SyncTitleCell(pgcn, cellN int) bool {
	if n.r.dead != nil {
		return false
	}
	a, err := n.ask(dvd.MethodSyncCell, dvd.NavCall{Clone: n.clone, N: pgcn, M: cellN})
	if err != nil {
		n.r.giveUp(err)
		return false
	}
	return a.Synced
}

// Registers are as the helper last said, so reading them costs nothing. The pointer
// stays good.
func (n *nav) Registers() *dvd.Registers { return &n.regs }

// StepPGC is the program chain a step plays, as the navigator resolves it. A session
// asks several times for each step, and the answer holds until the disc moves on.
func (n *nav) StepPGC(st *dvd.Step) *dvd.PGC {
	if st == nil || n.r.dead != nil {
		return nil
	}
	k := pgcKey{st.Domain, st.VTS, st.PGCN}
	if p, ok := n.pgcs[k]; ok {
		return p
	}
	p, err := n.askPGC(st)
	if err != nil {
		n.r.giveUp(err)
		return nil
	}
	if n.pgcs == nil {
		n.pgcs = map[pgcKey]*dvd.PGC{}
	}
	n.pgcs[k] = p
	return p
}

func (n *nav) askPGC(st *dvd.Step) (*dvd.PGC, error) {
	blob, err := n.r.h.Call(dvd.MethodStepPGC, dvd.NavCall{Clone: n.clone, Domain: st.Domain, N: st.VTS, M: st.PGCN}, nil)
	if err != nil {
		return nil, err
	}
	p, err := dvd.DecodePGC(blob)
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
	if n.r.dead == nil {
		a, err := n.ask(dvd.MethodClone, dvd.NavCall{})
		if err == nil {
			c.regs = a.Regs
			return c
		}
		n.r.giveUp(err)
	}
	return c
}
