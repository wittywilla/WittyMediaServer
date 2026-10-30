package wittybluray

import "fmt"

// The navigator: it runs the disc's program (first play, titles, menus)
// through the VM and tells the player what to play, the way a hardware
// player's firmware does. The player reports back when a playlist ends or
// the user presses a menu button, and gets the next step.
//
// A Step is "play this playlist from here" (with its menu, if it has one),
// "stop", or "this needs BD-J, which is not supported". The player plays a
// playlist's items in order; the engine does not need to be told about each
// item (chapters are the playlist's marks).

// StepKind is what the player should do.
type StepKind string

const (
	StepPlay        StepKind = "play"
	StepStop        StepKind = "stop"
	StepUnsupported StepKind = "unsupported" // a BD-J object: the player may fall back to the main feature
	StepStay        StepKind = "stay"        // nothing changes (a button only changed menu state)
)

// Step is the navigator's instruction to the player.
type Step struct {
	Kind     StepKind
	Reason   string
	Playlist *Playlist
	Item     int    // where to start: the play item index
	Time     uint32 // and the time in it (45 kHz ticks, in the clip's time base); 0 = the item's IN
	// Menu is set when the playlist has an IG (menu graphics) stream: its
	// PID, and the clip it comes from when it is in a sub-path.
	Menu    bool
	IGPID   uint16
	IGClip  string // "" when the IG stream is in the play item's own clip
	Title   int    // PSR4 at this step (0xffff during first play)
	Object  int    // the movie object that asked for this
	Effects *Effects
}

// NavOptions configure the player the navigator simulates.
type NavOptions struct {
	Registers RegisterOptions
}

type frame struct {
	object int
	pc     int
}

type navState struct {
	regs   *Registers
	obj    int // the movie object whose command is suspended by a playing playlist (-1 = none)
	pc     int
	stack  []frame // CALL_OBJECT / CALL_TITLE return points
	title  int
	timer  *NavTimer
	resume *navState // saved by a menu call from a title, for RESUME
}

// Nav runs a disc.
type Nav struct {
	d  *Disc
	st *navState
}

// NewNav makes a navigator for the disc.
func NewNav(d *Disc, o NavOptions) *Nav {
	return &Nav{d: d, st: &navState{regs: NewRegisters(o.Registers), obj: -1, title: 0xffff}}
}

// Registers exposes the player's registers (read, or set by the player for
// stream choices through SetAudio and friends).
func (n *Nav) Registers() *Registers { return n.st.regs }

// Clone copies the navigator, to explore a disc without disturbing it.
func (n *Nav) Clone() *Nav {
	c := *n.st
	c.regs = n.st.regs.Clone()
	c.stack = append([]frame(nil), n.st.stack...)
	return &Nav{d: n.d, st: &c}
}

// Start begins as if the disc were just inserted: the first-play object.
func (n *Nav) Start() *Step {
	n.st.title = 0xffff
	n.st.regs.PSR[PSRTitle] = 0xffff
	return n.runIndexObject(n.d.Index.FirstPlay, "first play")
}

// PlaylistEnded is called by the player when the playlist of the last
// Step has played to its end: the disc's program continues after PLAY_PL.
func (n *Nav) PlaylistEnded() *Step {
	if n.st.obj < 0 {
		return &Step{Kind: StepStop, Reason: "the playlist ended and nothing follows it"}
	}
	return n.run(n.st.obj, n.st.pc)
}

// PressButton runs a menu button's commands (from the IG stream). A button
// may change the menu (page, selected button), start another playlist, end
// the menu (TERMINATE_PL) or jump elsewhere.
func (n *Nav) PressButton(cmds []Command) *Step {
	fx := &Effects{}
	res := n.st.regs.Run(cmds, 0, RunOptions{}, fx)
	if res.Kind == ResultEnd {
		// A button's commands ran out (or it had none): the menu stays as it is.
		return &Step{Kind: StepStay, Object: -1, Effects: fx}
	}
	st := n.apply(res, n.st.obj, fx)
	if st.Kind == StepStay || st.Effects == nil {
		st.Effects = fx
	}
	return st
}

// MenuCall is the user's Menu key: the top menu, remembering where the
// title was so RESUME can return.
func (n *Nav) MenuCall() *Step {
	if n.st.title != 0 && n.st.title != 0xffff {
		saved := *n.st
		saved.regs = n.st.regs.Clone()
		n.st.resume = &saved
		r := n.st.regs
		r.PSR[PSRBackupTitle], r.PSR[PSRBackupChapter], r.PSR[PSRBackupPlaylist] = r.PSR[PSRTitle], r.PSR[PSRChapter], r.PSR[PSRPlaylist]
		r.PSR[PSRBackupItem], r.PSR[PSRBackupTime], r.PSR[PSRBackupButton] = r.PSR[PSRPlayItem], r.PSR[PSRTime], r.PSR[PSRButton]
		r.PSR[PSRBackupPage], r.PSR[PSRBackupStyle] = r.PSR[PSRPage], r.PSR[PSRStyle]
	}
	return n.jumpTitle(0)
}

// Resume returns to the title a menu call interrupted, at the saved time.
func (n *Nav) Resume() *Step {
	saved := n.st.resume
	if saved == nil {
		return &Step{Kind: StepStay, Reason: "nothing to resume"}
	}
	n.st = saved
	n.st.resume = nil
	pl := n.d.Playlist(int(saved.regs.PSR[PSRPlaylist]))
	if pl == nil {
		return n.jumpTitle(0)
	}
	item := int(saved.regs.PSR[PSRPlayItem])
	if item >= len(pl.Items) {
		item = 0
	}
	return n.playStep(pl, item, saved.regs.PSR[PSRTime], "resume")
}

// PlayTitle starts a title from the title list (1-based), as the user's
// title search does.
func (n *Nav) PlayTitle(t int) *Step {
	n.st.resume = nil
	return n.jumpTitle(t)
}

// PlayPlaylist plays a playlist directly, outside the disc's program: for
// "play the movie" from a library, and for discs whose program needs BD-J.
// When the playlist ends, nothing follows (PlaylistEnded stops).
func (n *Nav) PlayPlaylist(pl *Playlist, item int, t uint32) *Step {
	n.st.obj = -1
	n.st.resume = nil
	for _, ti := range n.d.Index.Titles {
		if ti.Object.Kind == ObjectHDMV && n.playsPlaylist(ti.Object.MovieObject, pl.Number) {
			n.st.title = ti.Number
			n.st.regs.PSR[PSRTitle] = uint32(ti.Number)
			break
		}
	}
	if item < 0 || item >= len(pl.Items) {
		item = 0
	}
	return n.playStep(pl, item, t, "")
}

// playsPlaylist says whether a movie object's commands name the playlist.
func (n *Nav) playsPlaylist(obj, playlist int) bool {
	if obj < 0 || obj >= len(n.d.MovieObjects) {
		return false
	}
	for _, c := range n.d.MovieObjects[obj].Commands {
		in := Decode(c)
		if in.Group == GroupBranch && in.Sub == 2 && in.Opt <= 2 && in.ImmDst && int(in.Dst) == playlist {
			return true
		}
	}
	return false
}

// SetTime tells the navigator where playback is, for resume and the disc's
// own program (PSR8).
func (n *Nav) SetTime(item int, t uint32) {
	n.st.regs.PSR[PSRPlayItem] = uint32(item)
	n.st.regs.PSR[PSRTime] = t
}

// SetChapter records the chapter the player is in (1-based).
func (n *Nav) SetChapter(c int) { n.st.regs.PSR[PSRChapter] = uint32(c) }

// SetAudio and SetSubtitle record the user's stream choices (1-based; a
// subtitle stream of 0 hides subtitles).
func (n *Nav) SetAudio(s int) { n.st.regs.PSR[PSRAudio] = uint32(s) }
func (n *Nav) SetSubtitle(s int, shown bool) {
	v := uint32(s) & 0xfff
	if shown {
		v |= 1 << 31
	}
	n.st.regs.PSR[PSRPG] = v
}
func (n *Nav) SetAngle(a int) { n.st.regs.PSR[PSRAngle] = uint32(a) }

// NavTimerFired is called by the player when the SET_NV_TIMER timeout of
// the last Step has passed with no user action.
func (n *Nav) NavTimerFired() *Step {
	t := n.st.timer
	n.st.timer = nil
	if t == nil {
		return &Step{Kind: StepStay}
	}
	return n.run(t.Object, 0)
}

// ---------------------------------------------------------------- internals

func (n *Nav) runIndexObject(o IndexObject, what string) *Step {
	switch o.Kind {
	case ObjectHDMV:
		return n.run(o.MovieObject, 0)
	case ObjectBDJ:
		return &Step{Kind: StepUnsupported, Reason: fmt.Sprintf("%s is a BD-J (Java) program (%s), which this engine cannot run", what, o.BDJName), Title: n.st.title}
	}
	return &Step{Kind: StepStop, Reason: what + " has no program"}
}

func (n *Nav) jumpTitle(t int) *Step {
	ix := n.d.Index
	switch {
	case t == 0:
		n.st.title = 0
		n.st.regs.PSR[PSRTitle] = 0
		return n.runIndexObject(ix.TopMenu, "the top menu")
	case t == 0xffff:
		return n.Start()
	case t >= 1 && t <= len(ix.Titles):
		n.st.title = t
		n.st.regs.PSR[PSRTitle] = uint32(t)
		return n.runIndexObject(ix.Titles[t-1].Object, fmt.Sprintf("title %d", t))
	}
	return &Step{Kind: StepStop, Reason: fmt.Sprintf("the disc jumped to title %d, which it does not have", t)}
}

// run executes a movie object from pc, following jumps and calls, until
// something plays or everything stops.
func (n *Nav) run(obj, pc int) *Step {
	for hops := 0; hops < 64; hops++ {
		if obj < 0 || obj >= len(n.d.MovieObjects) {
			return &Step{Kind: StepStop, Reason: fmt.Sprintf("the disc jumped to movie object %d, which it does not have", obj)}
		}
		fx := &Effects{}
		res := n.st.regs.Run(n.d.MovieObjects[obj].Commands, pc, RunOptions{}, fx)
		st := n.apply(res, obj, fx)
		if st.Kind == StepStay && st.Object >= 0 {
			obj, pc = st.Object, st.Item // apply asks to keep running elsewhere
			continue
		}
		return st
	}
	return &Step{Kind: StepStop, Reason: "the disc's program jumped between objects without playing anything"}
}

// apply turns a VM result into a step. A StepStay with Object >= 0 means
// "continue running that object at Item (as pc)".
func (n *Nav) apply(res Result, obj int, fx *Effects) *Step {
	if fx.NavTimer != nil {
		n.st.timer = fx.NavTimer
	}
	switch res.Kind {
	case ResultError:
		return &Step{Kind: StepStop, Reason: res.Err.Error()}
	case ResultEnd:
		// An object that ends without jumping ends playback (discs always
		// jump; a button's command list ending just leaves the menu as is).
		if obj == n.st.obj && n.st.obj >= 0 {
			return &Step{Kind: StepStay, Object: -1, Effects: fx}
		}
		n.st.obj = -1
		return &Step{Kind: StepStop, Reason: "the disc's program ended"}
	case ResultJumpObject:
		n.st.obj = -1
		return &Step{Kind: StepStay, Object: res.Object, Item: 0}
	case ResultJumpTitle:
		n.st.obj = -1
		n.st.stack = nil
		return n.jumpTitle(res.Title)
	case ResultCallObject:
		n.st.stack = append(n.st.stack, frame{object: obj, pc: res.PC})
		n.st.obj = -1
		return &Step{Kind: StepStay, Object: res.Object, Item: 0}
	case ResultCallTitle:
		n.st.stack = append(n.st.stack, frame{object: obj, pc: res.PC})
		n.st.obj = -1
		return n.jumpTitle(res.Title)
	case ResultResume:
		if n.st.resume != nil {
			return n.Resume()
		}
		if k := len(n.st.stack); k > 0 {
			f := n.st.stack[k-1]
			n.st.stack = n.st.stack[:k-1]
			return &Step{Kind: StepStay, Object: f.object, Item: f.pc}
		}
		return &Step{Kind: StepStop, Reason: "RESUME with nothing to resume"}
	case ResultPlay:
		pl := n.d.Playlist(res.Playlist)
		if pl == nil {
			return &Step{Kind: StepStop, Reason: fmt.Sprintf("the disc asked for playlist %05d, which it does not have", res.Playlist)}
		}
		n.st.obj, n.st.pc = obj, res.PC
		item, t := 0, uint32(0)
		if res.Item >= 0 && res.Item < len(pl.Items) {
			item = res.Item
		}
		if res.Mark >= 0 && res.Mark < len(pl.Marks) {
			m := pl.Marks[res.Mark]
			item, t = m.Item, m.Time
		}
		return n.playStep(pl, item, t, "")
	case ResultTerminate:
		// The menu's playlist stops; the suspended object continues.
		if n.st.obj < 0 {
			return &Step{Kind: StepStop, Reason: "the playlist ended"}
		}
		return n.run(n.st.obj, n.st.pc)
	case ResultLinkItem, ResultLinkMark:
		pl := n.d.Playlist(int(n.st.regs.PSR[PSRPlaylist]))
		if pl == nil {
			return &Step{Kind: StepStop, Reason: "LINK with no playlist playing"}
		}
		item, t := 0, uint32(0)
		if res.Kind == ResultLinkItem && res.Item >= 0 && res.Item < len(pl.Items) {
			item = res.Item
		}
		if res.Kind == ResultLinkMark && res.Mark >= 0 && res.Mark < len(pl.Marks) {
			m := pl.Marks[res.Mark]
			item, t = m.Item, m.Time
		}
		return n.playStep(pl, item, t, "")
	}
	return &Step{Kind: StepStop, Reason: "unknown result"}
}

func (n *Nav) playStep(pl *Playlist, item int, t uint32, reason string) *Step {
	r := n.st.regs
	r.PSR[PSRPlaylist] = uint32(pl.Number)
	r.PSR[PSRPlayItem] = uint32(item)
	r.PSR[PSRTime] = t
	st := &Step{Kind: StepPlay, Reason: reason, Playlist: pl, Item: item, Time: t, Title: n.st.title, Object: n.st.obj}
	if item < len(pl.Items) {
		it := pl.Items[item]
		ig := int(r.PSR[PSRIGStream])
		if ig < 1 || ig > len(it.IG) {
			ig = 1
		}
		if len(it.IG) > 0 {
			s := it.IG[ig-1]
			st.Menu, st.IGPID = true, s.PID
			if s.EntryType != 1 && s.SubPath < len(pl.SubPaths) && len(pl.SubPaths[s.SubPath].Items) > 0 {
				st.IGClip = pl.SubPaths[s.SubPath].Items[0].Clip
			}
		}
	}
	return st
}
