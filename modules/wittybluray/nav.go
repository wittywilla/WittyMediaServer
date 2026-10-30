package wittybluray

import "fmt"

// The navigator: it runs the disc's program (first play, titles, menus)
// through the VM and tells the player what to play, the way a hardware
// player's firmware does. The player reports back when a playlist ends or
// the user presses a menu button, and gets the next step.
//
// A Step is "play this playlist from here" (with its menu, if it has one),
// "stop", or "this is a BD-J program". The player plays a playlist's items in
// order unless the step says ItemOnly (chapters are the playlist's marks).

// StepKind is what the player should do.
type StepKind string

const (
	StepPlay        StepKind = "play"
	StepStop        StepKind = "stop"
	StepUnsupported StepKind = "unsupported" // a BD-J object (Step.BDJ names it): run it if the player can, else fall back to the main feature
	StepStay        StepKind = "stay"        // nothing changes (a button only changed menu state)
)

// Step is the navigator's instruction to the player.
type Step struct {
	Kind     StepKind
	Reason   string
	Playlist *Playlist
	Item     int    // where to start: the play item index
	Time     uint32 // and the time in it (45 kHz ticks, in the clip's time base); 0 = the item's IN
	// ItemOnly: play just this item, then report PlaylistEnded; the navigator
	// says which item comes next. Set for random and shuffle playlists, items
	// with a still, and playlists whose items do not share one set of streams.
	ItemOnly bool
	// StillSec is how long the picture holds after what this step plays: 0 =
	// not at all, -1 = until the viewer acts.
	StillSec float64
	// Menu is set when the playlist has an IG (menu graphics) stream: its
	// PID, and the clip it comes from when it is in a sub-path.
	Menu    bool
	IGPID   uint16
	IGClip  string // "" when the IG stream is in the play item's own clip
	Title   int    // PSR4 at this step (0xffff during first play)
	Object  int    // the movie object that asked for this
	Effects *Effects
	// BDJ names the BD-J object of a StepUnsupported step.
	BDJ string
}

// NavOptions configure the player the navigator simulates.
type NavOptions struct {
	Registers RegisterOptions
}

// frame is the one thing a disc can suspend: a movie object to come back to.
// playing says it was suspended inside a PLAY_PL (a menu call during the
// film), so coming back continues the playlist rather than the next command.
type frame struct {
	object  int
	pc      int
	playing bool
}

type navState struct {
	regs      *Registers
	obj       int // the movie object whose command is suspended by a playing playlist (-1 = none)
	pc        int
	suspended *frame // CALL_OBJECT, CALL_TITLE or a menu call: one level only
	title     int
	timer     *NavTimer
	// The playlist playing, for random and item-by-item playback.
	pl       *Playlist
	item     int
	played   []bool
	left     int
	itemOnly bool
}

// Nav runs a disc.
type Nav struct {
	d  *Disc
	st *navState
}

// NewNav makes a navigator for the disc. An Ultra HD disc gets an Ultra HD
// player: such discs ask, and refuse to start on anything else.
func NewNav(d *Disc, o NavOptions) *Nav {
	if d.Index != nil && d.Index.Version >= "0300" {
		o.Registers.UHD = true
	}
	return &Nav{d: d, st: &navState{regs: NewRegisters(o.Registers), obj: -1, title: 0xffff}}
}

// Registers exposes the player's registers (read, or set by the player for
// stream choices through SetAudio and friends).
func (n *Nav) Registers() *Registers { return n.st.regs }

// Clone copies the navigator, to explore a disc without disturbing it.
func (n *Nav) Clone() *Nav {
	c := *n.st
	c.regs = n.st.regs.Clone()
	c.played = append([]bool(nil), n.st.played...)
	if n.st.suspended != nil {
		f := *n.st.suspended
		c.suspended = &f
	}
	if n.st.timer != nil {
		t := *n.st.timer
		c.timer = &t
	}
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
	if st := n.nextItem(); st != nil {
		return st
	}
	n.st.pl = nil
	if n.st.obj < 0 {
		return &Step{Kind: StepStop, Reason: "the playlist ended and nothing follows it"}
	}
	return n.run(n.st.obj, n.st.pc)
}

// LoopsAtEnd says whether the disc's program starts this same playlist again
// when it ends. Such a menu plays as one seamless loop; anything else
// (a menu that moves on by itself) must be reported as ended.
func (n *Nav) LoopsAtEnd(st *Step) bool {
	if st == nil || st.Playlist == nil || st.ItemOnly {
		return false
	}
	next := n.Clone().PlaylistEnded()
	return next.Kind == StepPlay && next.Playlist == st.Playlist && next.Item == st.Item && next.Time == st.Time
}

// SkipItem moves within an item-by-item playlist (a slideshow), one way or
// the other; ok is false when the playlist does not play that way or has no
// such item.
func (n *Nav) SkipItem(delta int) (st *Step, ok bool) {
	pl := n.st.pl
	if pl == nil || !n.st.itemOnly || pl.PlaybackType >= 2 {
		return nil, false
	}
	i := n.st.item + delta
	if i < 0 || i >= len(pl.Items) {
		return nil, false
	}
	return n.playStep(pl, i, 0, ""), true
}

// PressButton runs a menu button's commands (from the IG stream). A button
// may change the menu (page, selected button), start another playlist, end
// the menu (TERMINATE_PL) or jump elsewhere.
func (n *Nav) PressButton(cmds []Command) *Step {
	fx := &Effects{}
	res := n.st.regs.Run(cmds, 0, RunOptions{Button: true}, fx)
	if res.Kind == ResultEnd {
		// A button's commands ran out (or it had none): the menu stays as it is.
		n.noteTimer(fx)
		return &Step{Kind: StepStay, Object: -1, Effects: fx}
	}
	st := n.apply(res, n.st.obj, fx)
	if st.Kind == StepStay && st.Object >= 0 {
		// JUMP_OBJECT or CALL_OBJECT: the disc's program goes on there.
		st = n.run(st.Object, st.Item)
	}
	if st.Kind == StepStay || st.Effects == nil {
		st.Effects = fx
	}
	return st
}

// SetMenuState records the page and button on show, which a disc's commands
// read to find out what was pressed.
func (n *Nav) SetMenuState(page, button int) {
	n.st.regs.PSR[PSRPage] = uint32(page)
	n.st.regs.PSR[PSRButton] = uint32(button)
}

// MenuAllowed says whether the disc lets the viewer call the menu now: the
// movie object or the playing playlist and item can forbid it.
func (n *Nav) MenuAllowed() bool {
	if o := n.st.obj; o >= 0 && o < len(n.d.MovieObjects) && n.d.MovieObjects[o].MenuCallMask {
		return false
	}
	return !n.Forbids(UOMenuCall)
}

// Forbids says whether the playing playlist or item forbids a user operation
// (a UO constant).
func (n *Nav) Forbids(uo uint64) bool {
	pl := n.st.pl
	if pl == nil {
		return false
	}
	if pl.UOMask&uo != 0 {
		return true
	}
	i := int(n.st.regs.PSR[PSRPlayItem])
	return i < len(pl.Items) && pl.Items[i].UOMask&uo != 0
}

// MenuCall is the user's Menu key: the top menu. When the disc's program
// means the playing object to be resumed (its resume-intention flag), it is
// suspended so RESUME can return; otherwise it ends here. A disc that masks
// the key sees nothing happen.
func (n *Nav) MenuCall() *Step {
	if !n.MenuAllowed() {
		return &Step{Kind: StepStay, Object: -1, Reason: "the disc does not allow the menu here"}
	}
	if o := n.st.obj; o >= 0 && n.st.pl != nil {
		n.st.suspended = nil
		if o < len(n.d.MovieObjects) && n.d.MovieObjects[o].ResumeIntention {
			n.st.suspended = &frame{object: o, pc: n.st.pc, playing: true}
			n.backup()
		}
		n.st.obj = -1
	}
	n.st.pl = nil
	return n.titleZero()
}

// Resume returns to what a call or menu call suspended.
func (n *Nav) Resume() *Step {
	f := n.st.suspended
	if f == nil {
		return &Step{Kind: StepStay, Object: -1, Reason: "nothing to resume"}
	}
	n.st.suspended = nil
	n.restore()
	if !f.playing {
		return n.run(f.object, f.pc)
	}
	n.st.obj, n.st.pc = f.object, f.pc
	r := n.st.regs
	pl := n.d.Playlist(int(r.PSR[PSRPlaylist]))
	if pl == nil {
		return n.run(f.object, f.pc)
	}
	item := int(r.PSR[PSRPlayItem])
	if item >= len(pl.Items) {
		item = 0
	}
	return n.playStep(pl, item, r.PSR[PSRTime], "resume")
}

// backup and restore keep the playback position across a suspension. General
// registers and the stream choices are not touched: a setup menu changes
// them, and they must stay changed on the way back.
func (n *Nav) backup() {
	r := &n.st.regs.PSR
	copy(r[PSRBackupTitle:PSRBackupTime+1], r[PSRTitle:PSRTime+1])
	copy(r[PSRBackupButton:PSRBackupStyle+1], r[PSRButton:PSRStyle+1])
}

func (n *Nav) restore() {
	r := &n.st.regs.PSR
	copy(r[PSRTitle:PSRTime+1], r[PSRBackupTitle:PSRBackupTime+1])
	copy(r[PSRButton:PSRStyle+1], r[PSRBackupButton:PSRBackupStyle+1])
	n.st.title = int(r[PSRTitle])
	n.clearBackup()
}

func (n *Nav) clearBackup() {
	r := &n.st.regs.PSR
	r[PSRBackupTitle], r[PSRBackupChapter], r[PSRBackupPlaylist], r[PSRBackupItem], r[PSRBackupTime] = 0xffff, 0xffff, 0, 0, 0
	r[PSRBackupButton], r[PSRBackupPage], r[PSRBackupStyle] = 0xffff, 0, 0xff
}

// PlayTitle starts a title from the title list (1-based), as the user's
// title search does.
func (n *Nav) PlayTitle(t int) *Step {
	n.st.suspended = nil
	n.st.obj = -1
	n.st.pl = nil
	n.clearBackup()
	return n.jumpTitle(t)
}

// PlayPlaylist plays a playlist directly, outside the disc's program: for
// "play the movie" from a library, and for discs whose program needs BD-J.
// When the playlist ends, nothing follows (PlaylistEnded stops).
func (n *Nav) PlayPlaylist(pl *Playlist, item int, t uint32) *Step {
	n.st.obj = -1
	n.st.suspended = nil
	n.st.pl = nil
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
	n.st.regs.setPG(uint32(s), shown)
}
func (n *Nav) SetAngle(a int) { n.st.regs.PSR[PSRAngle] = uint32(a) }

// Timer is the navigation timer running now, or nil. The player starts its
// clock when this changes and calls NavTimerFired when it runs out.
func (n *Nav) Timer() *NavTimer { return n.st.timer }

// NavTimerFired is called by the player when the SET_NV_TIMER timeout of
// the last Step has passed with no user action.
func (n *Nav) NavTimerFired() *Step {
	t := n.st.timer
	n.st.timer = nil
	n.st.regs.PSR[PSRNavTimer] = 0
	if t == nil {
		return &Step{Kind: StepStay, Object: -1}
	}
	n.st.obj = -1
	n.st.pl = nil
	return n.run(t.Object, 0)
}

// ---------------------------------------------------------------- internals

func (n *Nav) runIndexObject(o IndexObject, what string) *Step {
	switch o.Kind {
	case ObjectHDMV:
		return n.run(o.MovieObject, 0)
	case ObjectBDJ:
		return &Step{Kind: StepUnsupported, BDJ: o.BDJName, Reason: fmt.Sprintf("%s is a BD-J (Java) program (%s)", what, o.BDJName), Title: n.st.title}
	}
	return &Step{Kind: StepStop, Reason: what + " has no program"}
}

// titleZero is the top menu, keeping what a menu call suspended.
func (n *Nav) titleZero() *Step {
	n.st.title = 0
	n.st.regs.PSR[PSRTitle] = 0
	return n.runIndexObject(n.d.Index.TopMenu, "the top menu")
}

func (n *Nav) jumpTitle(t int) *Step {
	ix := n.d.Index
	switch {
	case t == 0:
		return n.titleZero()
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

func (n *Nav) noteTimer(fx *Effects) {
	if fx.NavTimer != nil {
		n.st.timer = fx.NavTimer
	} else if fx.NavTimerOff {
		n.st.timer = nil
	}
}

// apply turns a VM result into a step. A StepStay with Object >= 0 means
// "continue running that object at Item (as pc)".
func (n *Nav) apply(res Result, obj int, fx *Effects) *Step {
	n.noteTimer(fx)
	switch res.Kind {
	case ResultError:
		return &Step{Kind: StepStop, Reason: res.Err.Error()}
	case ResultEnd:
		// An object that ends without jumping ends playback (discs always
		// jump; a button's command list ending just leaves the menu as is).
		// What is suspended stays suspended: nothing resumes by itself.
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
		n.st.pl = nil
		n.st.suspended = nil
		n.clearBackup()
		return n.jumpTitle(res.Title)
	case ResultCallObject:
		n.suspend(obj, res.PC)
		n.st.obj = -1
		return &Step{Kind: StepStay, Object: res.Object, Item: 0}
	case ResultCallTitle:
		n.suspend(obj, res.PC)
		n.st.obj = -1
		return n.jumpTitle(res.Title)
	case ResultResume:
		if n.st.suspended != nil {
			return n.Resume()
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
		if res.Mark >= 0 && res.Mark < len(pl.Marks) && pl.Marks[res.Mark].Item < len(pl.Items) {
			m := pl.Marks[res.Mark]
			item, t = m.Item, m.Time
		}
		n.st.pl = nil // a new playlist, even if it is the same one again
		return n.playStep(pl, item, t, "")
	case ResultTerminate:
		// The menu's playlist stops; the suspended object continues.
		n.st.pl = nil
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
		if res.Kind == ResultLinkMark && res.Mark >= 0 && res.Mark < len(pl.Marks) && pl.Marks[res.Mark].Item < len(pl.Items) {
			m := pl.Marks[res.Mark]
			item, t = m.Item, m.Time
		}
		return n.playStep(pl, item, t, "")
	}
	return &Step{Kind: StepStop, Reason: "unknown result"}
}

// suspend remembers one object to come back to. A second call replaces the
// first, as the format has only one level; if a playlist is playing the
// call goes ahead without a way back.
func (n *Nav) suspend(obj, pc int) {
	if n.st.obj >= 0 && n.st.pl != nil {
		return
	}
	n.st.suspended = &frame{object: obj, pc: pc}
	n.backup()
}

// playsItemByItem says whether a playlist must be presented one item at a
// time: random order, a still inside it, or items whose streams differ (one
// continuous stream can only carry one set).
func playsItemByItem(pl *Playlist) bool {
	if pl.PlaybackType >= 2 {
		return true
	}
	for i := range pl.Items {
		it := &pl.Items[i]
		if it.StillMode != 0 {
			return true
		}
		if i > 0 && !sameStreams(&pl.Items[0], it) {
			return true
		}
	}
	return false
}

func sameStreams(a, b *PlayItem) bool {
	if len(a.Video) != len(b.Video) || len(a.Audio) != len(b.Audio) {
		return false
	}
	for i := range a.Video {
		if a.Video[i].PID != b.Video[i].PID || a.Video[i].Coding != b.Video[i].Coding || a.Video[i].Format != b.Video[i].Format {
			return false
		}
	}
	for i := range a.Audio {
		if a.Audio[i].PID != b.Audio[i].PID || a.Audio[i].Coding != b.Audio[i].Coding {
			return false
		}
	}
	return true
}

// nextItem is what follows an item played alone, or nil when the playlist is
// finished.
func (n *Nav) nextItem() *Step {
	pl := n.st.pl
	if pl == nil || !n.st.itemOnly {
		return nil
	}
	if pl.PlaybackType < 2 {
		if n.st.item+1 < len(pl.Items) {
			return n.playStep(pl, n.st.item+1, 0, "")
		}
		return nil
	}
	if n.st.left <= 0 {
		return nil
	}
	return n.playStep(pl, n.pickItem(pl), 0, "")
}

// pickItem draws the next item of a random playlist; a shuffle plays each
// item once before any repeats.
func (n *Nav) pickItem(pl *Playlist) int {
	num := uint32(len(pl.Items))
	if pl.PlaybackType == 3 {
		free := 0
		for _, p := range n.st.played {
			if !p {
				free++
			}
		}
		if free == 0 {
			n.st.played = make([]bool, num)
			free = int(num)
		}
		k := int(n.st.regs.random(uint32(free))) - 1
		for i, p := range n.st.played {
			if p {
				continue
			}
			if k == 0 {
				return i
			}
			k--
		}
	}
	return int(n.st.regs.random(num)) - 1
}

// chapterAt numbers the entry mark at or before a place in the playlist.
func chapterAt(pl *Playlist, item int, t uint32) uint32 {
	c, cur := uint32(0), uint32(0xffff)
	for _, m := range pl.Marks {
		if m.Type != 1 {
			continue
		}
		c++
		if m.Item < item || (m.Item == item && m.Time <= t) {
			cur = c
		}
	}
	return cur
}

// selectStreams applies the player's own stream choice when a play item
// starts: a stream number the item does not have (or none yet) is replaced
// by the viewer's preferred language, else the first stream. Whether
// subtitles show is the disc's or the viewer's decision and stays as it is.
func (n *Nav) selectStreams(it *PlayItem) {
	r := n.st.regs
	if len(it.Audio) > 0 {
		if a := int(r.PSR[PSRAudio]); a < 1 || a > len(it.Audio) {
			r.PSR[PSRAudio] = uint32(pickLang(it.Audio, r.PSR[PSRAudioLang]) + 1)
		}
	}
	if len(it.PG) > 0 {
		if g := int(r.PSR[PSRPG] & 0xfff); g < 1 || g > len(it.PG) {
			r.PSR[PSRPG] = r.PSR[PSRPG]&^0xfff | uint32(pickLang(it.PG, r.PSR[PSRPGLang])+1)
		}
	}
	if len(it.IG) > 0 {
		if g := int(r.PSR[PSRIGStream]); g < 1 || g > len(it.IG) {
			r.PSR[PSRIGStream] = 1
		}
	}
	if a := int(r.PSR[PSRAngle]); a < 1 || a > len(it.Angles) {
		r.PSR[PSRAngle] = 1
	}
}

func pickLang(streams []Stream, lang uint32) int {
	for i := range streams {
		if packLang(streams[i].Lang) == lang {
			return i
		}
	}
	return 0
}

func (n *Nav) playStep(pl *Playlist, item int, t uint32, reason string) *Step {
	r := n.st.regs
	if n.st.pl != pl {
		// A playlist starts: an item the disc named plays first, then the rest.
		n.st.played = make([]bool, len(pl.Items))
		n.st.left = pl.PlaybackCount
		if n.st.left <= 0 {
			n.st.left = len(pl.Items)
		}
		if pl.PlaybackType >= 2 && item == 0 && t == 0 && len(pl.Items) > 0 {
			item = n.pickItem(pl)
		}
	}
	n.st.pl, n.st.item, n.st.itemOnly = pl, item, playsItemByItem(pl)
	if item < len(n.st.played) {
		n.st.played[item] = true
	}
	n.st.left--
	r.PSR[PSRPlaylist] = uint32(pl.Number)
	r.PSR[PSRPlayItem] = uint32(item)
	r.PSR[PSRTime] = t
	at := t
	if item < len(pl.Items) && at < pl.Items[item].In {
		at = pl.Items[item].In // time 0 means the item's start
	}
	r.PSR[PSRChapter] = chapterAt(pl, item, at)
	st := &Step{Kind: StepPlay, Reason: reason, Playlist: pl, Item: item, Time: t, Title: n.st.title, Object: n.st.obj, ItemOnly: n.st.itemOnly}
	if item < len(pl.Items) {
		it := &pl.Items[item]
		n.selectStreams(it)
		switch it.StillMode {
		case 1:
			st.StillSec = float64(it.StillTime)
		case 2:
			st.StillSec = -1
		}
		if len(it.IG) > 0 {
			s := it.IG[r.PSR[PSRIGStream]-1]
			st.Menu, st.IGPID = true, s.PID
			if s.EntryType != 1 {
				st.IGClip = pl.SubClip(s)
			}
		}
	}
	return st
}
