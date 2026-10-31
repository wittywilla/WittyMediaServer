package disc

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"strings"
	"time"

	"github.com/wittywilla/wittydvd"
)

// dvdSession is one opened DVD.
type dvdSession struct {
	opened *wittydvd.Opened
	nav    *wittydvd.Nav
	prefs  Prefs
	drive  bool // read straight from a disc drive

	lastTick time.Time
	current  *Presentation
	menus    map[int]*dvdArt
	titles   map[int]*wittydvd.TitleSummary
	playing  *dvdPlaying
	noVocals bool
	// resumeAt is the offset into the resume cell where the viewer left a title
	// for a menu, so Resume continues from the same frame.
	resumeAt float64
	// seekTo is the exact title position to restart a rebuilt stream at.
	seekTo float64
}

type dvdPlaying struct {
	ts  *wittydvd.TitleSummary
	idx int // index into ts.Cells of the cell playback started from
}

type dvdArt struct {
	pic        *wittydvd.MenuPicture
	overlay    []byte
	highlights map[int][]byte
}

// karaokeOff marks a karaoke audio stream played without its vocals.
const karaokeOff = 0x100

// stillMaxSec: a cell this short is a still picture with no sound.
const stillMaxSec = 1.5

// openDVD opens a disc and refuses it if it is protected.
func openDVD(path string, drive bool, prefs Prefs) (*dvdSession, error) {
	o, err := wittydvd.Open(path)
	if err != nil {
		if drive {
			return nil, &RefusalError{Code: CodeUnreadable, Message: MsgDriveUnreadable}
		}
		return nil, err
	}
	if err := checkDVDProtection(o, drive); err != nil {
		_ = o.Close()
		return nil, err
	}
	s := &dvdSession{opened: o, prefs: prefs, drive: drive, lastTick: time.Now(), menus: map[int]*dvdArt{}, titles: map[int]*wittydvd.TitleSummary{}}
	s.nav = wittydvd.NewNav(o.Disc, wittydvd.NavOptions{RegisterOptions: wittydvd.RegisterOptions{MenuLanguage: prefs.MenuLanguage, AudioLanguage: prefs.AudioLanguage, SubtitleLanguage: prefs.SubtitleLanguage}})
	return s, nil
}

func (s *dvdSession) Close() error { return s.opened.Close() }

func (s *dvdSession) Info() Info {
	in := Info{Kind: "dvd", Label: s.opened.Label, Titles: []TitleInfo{}}
	if m := s.opened.Disc.MainTitle(); m != nil {
		in.Main = m.Number
	}
	for _, t := range s.opened.Disc.Titles() {
		in.Titles = append(in.Titles, TitleInfo{N: t.Number, Duration: t.Duration, Chapters: len(t.Chapters), Main: t.Number == in.Main})
	}
	return in
}

func (s *dvdSession) Start() *Presentation { return s.show(s.nav.Start()) }

func (s *dvdSession) PlayTitle(ttn, chapter int) *Presentation {
	return s.show(s.nav.PlayTitle(ttn, chapter))
}

// Do applies an action and returns the new presentation.
func (s *dvdSession) Do(a Action) *Presentation {
	if st := s.tick(); st != nil {
		return s.show(st)
	}
	cur := s.current
	switch a.Type {
	case "tick":
		return cur
	case "press":
		return s.press(a.Button)
	case "select":
		return s.show(s.nav.SelectButton(a.Button))
	case "key":
		if a.Key == "menu" || a.Key == "root" || a.Key == "title" || a.Key == "return" {
			s.syncPosition(a.PositionSec)
		}
		return s.keyPress(a.Key)
	case "ended":
		return s.ended()
	case "title":
		return s.show(s.nav.PlayTitle(a.TTN, a.Chapter))
	case "menu":
		kind := a.Menu
		if kind == "" {
			kind = "root"
		}
		s.syncPosition(a.PositionSec)
		return s.show(s.nav.MenuKey(kind))
	case "resume":
		return s.show(s.nav.Resume())
	case "chapter":
		if a.Chapter > 0 {
			return s.show(s.nav.NextChapter())
		}
		return s.show(s.nav.PrevChapter())
	case "audio":
		// A stream number with 0x100 is a karaoke stream without its vocals.
		s.noVocals = a.Stream&karaokeOff != 0
		s.nav.SetAudio(a.Stream &^ karaokeOff)
		if cur != nil && cur.Kind == "title" {
			s.syncPosition(a.PositionSec)
			s.seekTo = a.PositionSec
			return s.present(s.nav.Current(), true)
		}
		return cur
	case "angle":
		return s.switchAngle(a.Stream, a.PositionSec)
	case "subtitle":
		// Subtitles are decoded with the stream: no new stream, no restart.
		s.nav.SetSubpicture(a.Stream, a.On)
		if cur != nil && cur.Kind == "title" && cur.Title != nil {
			p, tv := *cur, *cur.Title
			tv.SubN = -1
			if a.On {
				tv.SubN = a.Stream
			}
			tv.ForcedStream = a.Stream & 31
			p.Title = &tv
			s.current = &p
			return &p
		}
		return cur
	}
	return cur
}

func (s *dvdSession) press(n int) *Presentation {
	cur := s.current
	if cur == nil || cur.Menu == nil {
		return s.show(s.nav.PressButton(n, nil))
	}
	art := s.menus[cur.Serial]
	var cmd *wittydvd.Command
	if art != nil {
		for _, b := range art.pic.Buttons {
			if b.N == n {
				c := b.Cmd
				cmd = &c
			}
		}
	}
	return s.show(s.nav.PressButton(n, cmd))
}

func (s *dvdSession) keyPress(key string) *Presentation {
	cur := s.current
	switch key {
	case "menu", "root":
		return s.show(s.nav.MenuKey("root"))
	case "title":
		return s.show(s.nav.MenuKey("title"))
	case "resume":
		return s.show(s.nav.Resume())
	case "return":
		return s.show(s.nav.GoUp())
	case "next":
		return s.show(s.nav.NextChapter())
	case "prev":
		return s.show(s.nav.PrevChapter())
	}
	if cur == nil || cur.Menu == nil || len(cur.Menu.Buttons) == 0 {
		return cur
	}
	sel := cur.Menu.Selected
	var selBtn *ButtonView
	for i := range cur.Menu.Buttons {
		if cur.Menu.Buttons[i].N == sel {
			selBtn = &cur.Menu.Buttons[i]
		}
	}
	if selBtn == nil {
		selBtn = &cur.Menu.Buttons[0]
		sel = selBtn.N
	}
	next := sel
	switch key {
	case "up":
		next = selBtn.Up
	case "down":
		next = selBtn.Down
	case "left":
		next = selBtn.Left
	case "right":
		next = selBtn.Right
	case "enter", "ok", "select":
		return s.press(sel)
	default:
		return cur
	}
	if next == 0 {
		next = sel
	}
	for _, b := range cur.Menu.Buttons {
		if b.N == next && b.Auto {
			return s.press(next)
		}
	}
	return s.show(s.nav.SelectButton(next))
}

// tick advances the disc clocks by the wall time since the last action. It
// returns a step if the navigation timer fired.
func (s *dvdSession) tick() *wittydvd.Step {
	now := time.Now()
	el := now.Sub(s.lastTick).Seconds()
	s.lastTick = now
	return s.nav.Tick(el)
}

// syncPosition maps the viewer's position in a title back to the cell the
// navigator should consider current.
func (s *dvdSession) syncPosition(pos float64) {
	cur := s.current
	if s.playing == nil || cur == nil || cur.Kind != "title" || pos <= 0 {
		return
	}
	cells := s.playing.ts.Cells
	k := -1
	for i, c := range cells {
		if c.Start <= pos+0.01 {
			k = i
		}
	}
	if k < 0 || !s.nav.SyncTitleCell(cells[k].PGCN, cells[k].CellN) {
		return
	}
	s.playing.idx = k
	s.resumeAt = pos - cells[k].Start
}

// ended is called when the player finished the clip and any still time.
func (s *dvdSession) ended() *Presentation {
	cur := s.current
	if pl := s.playing; cur != nil && cur.Kind == "title" && cur.Title != nil && pl != nil {
		// The player played the rest of the flattened title in one stream; walk
		// the navigator through exactly those cells so cell and post commands
		// run, and stop as soon as the disc goes somewhere else.
		st := s.nav.CellEnded()
		for i := pl.idx + 1; i < len(pl.ts.Cells); i++ {
			c := pl.ts.Cells[i]
			if st.Kind != wittydvd.StepTitle || st.PGCN != c.PGCN || st.CellN != c.CellN || st.Title == nil || st.Title.TTN != cur.Title.TTN {
				break
			}
			st = s.nav.CellEnded()
		}
		return s.show(st)
	}
	return s.show(s.nav.CellEnded())
}

func (s *dvdSession) show(st *wittydvd.Step) *Presentation { return s.present(st, false) }

func stopPresentation(serial int, reason string) *Presentation {
	return &Presentation{Kind: "stop", Serial: serial, Reason: reason, Media: &Media{Type: "none"}}
}

// present turns a step into a presentation. A step with the same serial as the
// current presentation is the same screen with a moved highlight (or a message),
// so it reuses everything unless rebuild is set (stream changes).
func (s *dvdSession) present(st *wittydvd.Step, rebuild bool) *Presentation {
	if st == nil {
		return s.current
	}
	if cur := s.current; !rebuild && cur != nil && cur.Serial == st.Serial && cur.Kind != "stop" && st.Kind != wittydvd.StepStop {
		p := *cur
		p.Reason = st.Reason
		if p.Menu != nil {
			m := *p.Menu
			for _, b := range m.Buttons {
				if b.N == st.DefaultButton {
					m.Selected = b.N
				}
			}
			p.Menu = &m
		}
		s.current = &p
		return &p
	}
	p := &Presentation{Serial: st.Serial, Reason: st.Reason}
	prev := s.playing
	s.playing = nil
	fail := func(err error) {
		p.Kind, p.Reason = "stop", plainReason(err)
		p.Media = &Media{Type: "none"}
		p.Protected = IsProtected(err)
	}
	switch st.Kind {
	case wittydvd.StepStop:
		p.Kind = "stop"
		p.Media = &Media{Type: "none"}
	case wittydvd.StepTitle:
		// A rebuild (audio change) keeps the cell the viewer has reached.
		pl := prev
		if !rebuild || pl == nil || st.Title == nil || pl.ts.Number != st.Title.TTN || pl.ts != s.title(st.Title.TTN) {
			pl = s.linearTitle(st)
		}
		if pl != nil {
			p.Kind = "title"
			if err := s.buildTitle(st, pl, p); err != nil {
				fail(err)
			} else {
				s.playing = pl
			}
			break
		}
		// Interactive or branching title cells (games, quizzes, stills) play
		// cell by cell like a menu so their buttons and commands work.
		p.Kind = "menu"
		if err := s.buildMenu(st, p); err != nil {
			fail(err)
		}
	default:
		p.Kind = "menu"
		if err := s.buildMenu(st, p); err != nil {
			fail(err)
		}
	}
	if t := s.nav.Registers().SPRM[wittydvd.SprmNavTimer]; t > 0 && p.Kind != "stop" {
		p.TimerSec = float64(t)
	}
	s.current = p
	return p
}

// plainReason words a read failure for the viewer; refusals are already worded.
func plainReason(err error) string {
	var r *RefusalError
	if errors.As(err, &r) {
		return r.Message
	}
	return "This part of the disc could not be read: " + err.Error()
}

// title returns the flattened title (cached per session).
func (s *dvdSession) title(ttn int) *wittydvd.TitleSummary {
	angle := s.angle(ttn)
	key := ttn*16 + angle
	if ts, ok := s.titles[key]; ok {
		return ts
	}
	ts := s.opened.Disc.TitleAngle(ttn, angle)
	s.titles[key] = ts
	return ts
}

// angle is the camera angle to play for a title, 1 when it has only one.
func (s *dvdSession) angle(ttn int) int {
	n := 1
	if ttn >= 1 && ttn <= len(s.opened.Disc.VMG.Titles) {
		n = s.opened.Disc.VMG.Titles[ttn-1].Angles
	}
	a := int(s.nav.Registers().SPRM[wittydvd.SprmAngle])
	if a < 1 || a > n {
		return 1
	}
	return a
}

// switchAngle changes the camera angle of the title playing and continues at
// the same moment.
func (s *dvdSession) switchAngle(angle int, pos float64) *Presentation {
	cur := s.current
	if cur == nil || cur.Kind != "title" || cur.Title == nil || s.playing == nil {
		s.nav.SetAngle(angle)
		return cur
	}
	s.nav.SetAngle(angle)
	ts := s.title(cur.Title.TTN)
	if ts == nil || len(ts.Cells) == 0 {
		return cur
	}
	k := 0
	for i, c := range ts.Cells {
		if c.Start <= pos+0.01 {
			k = i
		}
	}
	s.nav.SyncTitleCell(ts.Cells[k].PGCN, ts.Cells[k].CellN)
	s.playing = &dvdPlaying{ts: ts, idx: k}
	s.seekTo = pos
	return s.present(s.nav.Current(), true)
}

// linearTitle decides whether a title step can play as one continuous stream
// from its current cell: the cell must be part of the title's normal chain,
// nothing in that chain may pause (stills) or loop, and the cell must not carry
// buttons. Anything else is played cell by cell.
func (s *dvdSession) linearTitle(st *wittydvd.Step) *dvdPlaying {
	if st.Title == nil || st.Loop || st.Hold || st.StillSec != 0 {
		return nil
	}
	ts := s.title(st.Title.TTN)
	if ts == nil || len(ts.Cells) == 0 {
		return nil
	}
	idx := -1
	for i, c := range ts.Cells {
		if c.PGCN == st.PGCN && c.CellN == st.CellN {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	v := s.opened.Disc.VTS[ts.VTS]
	if v == nil {
		return nil
	}
	for _, c := range ts.Cells[idx:] {
		if c.Cell.StillTime != 0 || c.PGCN < 1 || c.PGCN > len(v.PGCs) {
			return nil
		}
		// Short chains that set a highlight are interactive screens; a feature
		// film never is, whatever its commands do for the menu afterwards.
		// Random and shuffle chains pick their programs as they play.
		if p := v.PGCs[c.PGCN-1].PGC; p.StillTime != 0 || p.PlaybackMode != 0 || (p.SetsHighlight() && p.Duration < 600) {
			return nil
		}
	}
	// Look for in-title buttons. Short cells (extras, games) are scanned in full
	// because their buttons often appear only near the end; long feature cells
	// only at the start. Scans are cached.
	hops := 48
	if st.CellDuration <= 300 {
		hops = 0
	}
	pic, err := s.opened.Menu(wittydvd.SpaceFor(st.Domain, st.VTS), st.FirstSector, st.LastSector, s.scanOptions(st, hops))
	if err == nil && interactive(pic) {
		return nil
	}
	return &dvdPlaying{ts: ts, idx: idx}
}

func interactive(pic *wittydvd.MenuPicture) bool {
	for _, b := range pic.Buttons {
		if !b.Inert {
			return true
		}
	}
	return false
}

func (s *dvdSession) videoAttr(st *wittydvd.Step) wittydvd.VideoAttr {
	switch st.Domain {
	case wittydvd.DomainVTSM:
		if v := s.opened.Disc.VTS[st.VTS]; v != nil {
			return v.MenuVideo
		}
	case wittydvd.DomainVTS:
		if v := s.opened.Disc.VTS[st.VTS]; v != nil {
			return v.Video
		}
	}
	return s.opened.Disc.VMG.MenuVideo
}

// scanOptions builds the menu scan options for a step. Inside a title the
// highlight belongs to the sub-picture stream the disc selected.
func (s *dvdSession) scanOptions(st *wittydvd.Step, maxHops int) wittydvd.MenuScanOptions {
	o := wittydvd.MenuScanOptions{Palette: st.Palette, Video: s.videoAttr(st), Wide: true, NavButton: st.DefaultButton, MaxHops: maxHops}
	if pgc := s.nav.StepPGC(st); pgc != nil {
		n := 0
		if st.Domain == wittydvd.DomainVTS {
			n = int(s.nav.Registers().SPRM[wittydvd.SprmSubp] & 31)
		}
		o.SubpControl = pgc.SubpControl[n]
		if o.SubpControl == nil {
			o.SubpControl = pgc.SubpControl[0]
		}
	}
	return o
}

func (s *dvdSession) buildMenu(st *wittydvd.Step, p *Presentation) error {
	sp := wittydvd.SpaceFor(st.Domain, st.VTS)
	pic, err := s.opened.Menu(sp, st.FirstSector, st.LastSector, s.scanOptions(st, 0))
	if errors.Is(err, wittydvd.ErrNoNav) {
		// The cell does not start with a NAV pack (damaged sectors, junk cells):
		// play what is there without buttons rather than stop.
		v := s.videoAttr(st)
		pic, err = &wittydvd.MenuPicture{Aspect: 4.0 / 3.0, NoButtons: true}, nil
		pic.FrameW, pic.FrameH = v.Size()
		if v.Wide {
			pic.Aspect = 16.0 / 9.0
		}
	}
	if err != nil {
		if s.drive {
			return &RefusalError{Code: CodeUnreadable, Message: MsgDriveUnreadable}
		}
		return err
	}
	if pic.Scrambled {
		return &RefusalError{Code: CodeProtected, Message: MsgDVDProtected}
	}
	s.menus[st.Serial] = &dvdArt{pic: pic, highlights: map[int][]byte{}}
	for k := range s.menus {
		if k < st.Serial-4 {
			delete(s.menus, k)
		}
	}
	still := st.StillSec
	loop := st.Loop
	isStill := !loop && (st.CellDuration <= stillMaxSec || (math.IsInf(still, 1) && st.CellDuration <= 3))
	segs, err := s.stepExtents(sp, st)
	if err != nil {
		return err
	}
	m := newMedia("mpeg", "", segs, []float64{st.CellDuration})
	m.Token = fmt.Sprintf("m-%s-%d-%d-%d", st.DomainName, st.VTS, st.PGCN, st.CellN)
	m.Video = wittydvd.VideoStreamID
	m.Loop, m.Duration, m.Deinterlace = loop, st.CellDuration, true
	if math.IsInf(still, 1) {
		m.StillSec = -1
	} else {
		m.StillSec = still
	}
	if !isStill {
		if st.Domain == wittydvd.DomainVTS {
			if id := s.titleAudio(s.opened.Disc.VTS[st.VTS], s.nav.StepPGC(st)); id > 0 {
				m.Audio = id
				m.Token += fmt.Sprintf("-a%x", id)
			}
		} else {
			m.AudioFirst = true
		}
	}
	p.Media = m
	menu := &Menu{Width: pic.FrameW, Height: pic.FrameH, Aspect: "4:3", Selected: pic.DefaultButton, Art: st.Serial, ButtonsFromSec: pic.ButtonsFromSec, HasHighlights: pic.HasHighlights, Buttons: []ButtonView{},
		ForcedAction: pic.ForcedAction, ActionAtSec: pic.ActionAtSec, InTitle: st.Domain == wittydvd.DomainVTS}
	if pic.Aspect > 1.5 {
		menu.Aspect = "16:9"
	}
	m.Aspect = menu.Aspect
	// Most discs end the highlight with the cell (then a still holds it); only
	// an earlier end takes the buttons away.
	if u := pic.ButtonsUntilSec; u > 0 && !isStill && !loop && u < st.CellDuration-0.5 {
		menu.ButtonsUntilSec = u
	}
	for _, b := range pic.Buttons {
		if b.Inert {
			continue
		}
		menu.Buttons = append(menu.Buttons, ButtonView{N: b.N, X1: b.X0, Y1: b.Y0, X2: b.X1, Y2: b.Y1, Up: b.Up, Down: b.Down, Left: b.Left, Right: b.Right, Auto: b.Auto, Lit: b.Select.Opaque()})
	}
	menu.HasOverlay = pic.Normal != nil
	p.Menu = menu
	return nil
}

// stepExtents is the video of the step's cell, following only that cell's VOBUs
// when it is interleaved (multi-angle blocks, seamless branching).
func (s *dvdSession) stepExtents(sp wittydvd.Space, st *wittydvd.Step) ([][]wittydvd.FileExtent, error) {
	if p := s.nav.StepPGC(st); p != nil && st.CellN >= 1 && st.CellN <= len(p.Cells) {
		c := p.Cells[st.CellN-1]
		if c.Interleaved || c.BlockType == 1 {
			ext, err := s.opened.CellExtents(sp, c)
			if err != nil {
				return nil, err
			}
			if len(ext) > 0 {
				return [][]wittydvd.FileExtent{ext}, nil
			}
		}
	}
	ext, err := s.opened.RangeExtents(sp, st.FirstSector, st.LastSector)
	if err != nil {
		return nil, err
	}
	if len(ext) == 0 {
		return nil, errors.New("the video of this menu is missing from the disc")
	}
	return [][]wittydvd.FileExtent{ext}, nil
}

// newMedia lays stretches of bytes out as a stream, each with its duration.
func newMedia(container, token string, parts [][]wittydvd.FileExtent, durations []float64) *Media {
	m := &Media{Type: "clip", Container: container, Token: token, StartPTS: -1}
	files := map[string]int{}
	var at float64
	for i, part := range parts {
		var n int64
		for _, e := range part {
			k, ok := files[e.File]
			if !ok {
				k = len(m.Files)
				files[e.File] = k
				m.Files = append(m.Files, e.File)
			}
			m.Extents = append(m.Extents, [3]int64{int64(k), e.Offset, e.Length})
			n += e.Length
		}
		d := 0.0
		if i < len(durations) {
			d = durations[i]
		}
		m.Segments = append(m.Segments, Segment{Bytes: n, StartSec: at, Duration: d, BasePTS: -1})
		at += d
	}
	return m
}

// titleAudio returns the MPEG-PS stream id of the audio the viewer hears in a
// title-domain step, or 0 when the title set has no audio.
func (s *dvdSession) titleAudio(v *wittydvd.VTS, pgc *wittydvd.PGC) int {
	if v == nil || len(v.Audio) == 0 {
		return 0
	}
	n := int(s.nav.Registers().SPRM[wittydvd.SprmAudio]) & 7
	if n >= len(v.Audio) {
		n = 0
	}
	phys := n
	if pgc != nil && pgc.AudioControl[n] >= 0 {
		phys = pgc.AudioControl[n]
	}
	return v.Audio[n].StreamID(phys)
}

func (s *dvdSession) buildTitle(st *wittydvd.Step, pl *dvdPlaying, p *Presentation) error {
	ti := st.Title
	ts := pl.ts
	sp := wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: ts.VTS}
	start := ts.Cells[pl.idx].Start
	switch {
	case s.seekTo > 0:
		start = s.seekTo // stream change: continue exactly where the viewer was
	case st.Resumed && s.resumeAt > 0 && s.resumeAt < ts.Cells[pl.idx].Cell.Duration:
		start += s.resumeAt
	}
	s.seekTo, s.resumeAt = 0, 0
	v := s.opened.Disc.VTS[ts.VTS]
	if v == nil {
		return errors.New("this title's data is missing from the disc")
	}
	pgc := s.nav.StepPGC(st)
	s.pickDefaultStreams(v, pgc)
	regs := s.nav.Registers()
	audioN := int(regs.SPRM[wittydvd.SprmAudio]) & 7
	subRaw := int(regs.SPRM[wittydvd.SprmSubp])
	subN := -1
	if subRaw&0x40 != 0 {
		subN = subRaw & 31
	}
	audioID := s.titleAudio(v, pgc)
	var parts [][]wittydvd.FileExtent
	var durations []float64
	for _, c := range ts.Cells {
		ext, err := s.opened.CellExtents(sp, c.Cell)
		if err != nil {
			return err
		}
		parts = append(parts, ext)
		durations = append(durations, c.Cell.Duration)
	}
	angle := s.angle(ti.TTN)
	m := newMedia("mpeg", fmt.Sprintf("t%d-x%x-g%d", ti.TTN, audioID, angle), parts, durations)
	m.Video, m.Audio, m.Duration, m.Deinterlace = wittydvd.VideoStreamID, audioID, ts.Duration, true
	if s.noVocals && audioN < len(v.Audio) && v.Audio[audioN].Karaoke() {
		m.Token += "-nv"
		m.FrontOnly = true
	}
	fw, fh := ts.Video.Size()
	m.Aspect = "4:3"
	if ts.Video.Wide {
		m.Aspect = "16:9"
	}
	if start > 0 {
		if err := s.placeStart(m, ts, start); err != nil {
			return err
		}
	}
	p.Media = m
	tv := &TitleView{TTN: ti.TTN, Chapter: ti.PTTN, Duration: ts.Duration, StartSec: start, Chapters: ts.Chapters, AudioN: audioN, SubN: subN,
		Audio: []Track{}, Subtitles: []Track{}, FrameW: fw, FrameH: fh, Aspect: m.Aspect, ForcedStream: subRaw & 31, Angle: angle, Angles: 1}
	if m.FrontOnly {
		tv.AudioN = audioN | karaokeOff
	}
	if pgc != nil {
		tv.Locks = uopLocks(pgc.UOPs)
		for _, c := range pgc.Palette {
			r, g, b := wittydvd.YCrCbToRGB(int(c>>16&0xff), int(c>>8&0xff), int(c&0xff))
			tv.Palette = append(tv.Palette, hex.EncodeToString([]byte{r, g, b}))
		}
	}
	if ti.TTN >= 1 && ti.TTN <= len(s.opened.Disc.VMG.Titles) && s.opened.Disc.VMG.Titles[ti.TTN-1].Angles > 1 {
		tv.Angles = s.opened.Disc.VMG.Titles[ti.TTN-1].Angles
	}
	for i, a := range v.Audio {
		if pgc != nil && i < 8 && pgc.AudioControl[i] < 0 {
			continue // not present in this title
		}
		purpose := a.Purpose()
		if a.Karaoke() {
			purpose = "Karaoke with vocals"
		}
		tv.Audio = append(tv.Audio, Track{N: i, Language: a.Language, Codec: a.Codec, Channels: a.Channels, Purpose: purpose})
		if a.Karaoke() {
			tv.Audio = append(tv.Audio, Track{N: i | karaokeOff, Language: a.Language, Codec: a.Codec, Channels: 2, Purpose: "Karaoke, sing along"})
		}
	}
	for i, sub := range v.Subp {
		if pgc != nil && pgc.SubpControl[i] == nil {
			continue
		}
		tv.Subtitles = append(tv.Subtitles, Track{N: i, Language: sub.Language, Purpose: sub.Purpose(), ID: 0x20 + physicalSub(pgc, i, ts.Video.Wide)})
	}
	if tv.Chapters == nil {
		tv.Chapters = []float64{}
	}
	p.Title = tv
	return nil
}

// physicalSub maps a logical sub-picture stream to the one in the VOBs, for a
// wide screen.
func physicalSub(pgc *wittydvd.PGC, n int, wide bool) int {
	if pgc == nil || pgc.SubpControl[n] == nil {
		return n
	}
	if sc := pgc.SubpControl[n]; wide {
		return sc.Wide
	}
	return pgc.SubpControl[n].Narrow
}

// pickDefaultStreams applies the viewer's language preferences when the disc
// has not chosen streams itself.
func (s *dvdSession) pickDefaultStreams(v *wittydvd.VTS, pgc *wittydvd.PGC) {
	regs := s.nav.Registers()
	avail := func(i int) bool { return pgc == nil || (i < 8 && pgc.AudioControl[i] >= 0) }
	if a := int(regs.SPRM[wittydvd.SprmAudio]); a >= len(v.Audio) || !avail(a) {
		pick := -1
		want := s.prefs.AudioLanguage
		for i, at := range v.Audio {
			if avail(i) && at.Purpose() == "" && want != "" && strings.EqualFold(at.Language, want) {
				pick = i
				break
			}
		}
		for i := range v.Audio {
			if pick < 0 && avail(i) {
				pick = i
			}
		}
		if pick >= 0 {
			s.nav.SetAudio(pick)
		}
	}
	if sub := int(regs.SPRM[wittydvd.SprmSubp]); sub&31 >= len(v.Subp) || (pgc != nil && pgc.SubpControl[sub&31] == nil) {
		if want := s.prefs.SubtitleLanguage; want != "" {
			for i, sa := range v.Subp {
				if (pgc == nil || pgc.SubpControl[i] != nil) && sa.Purpose() == "" && strings.EqualFold(sa.Language, want) {
					s.nav.SetSubpicture(i, true)
					break
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Seeking
// ---------------------------------------------------------------------------

// placeStart makes a new stream begin at a title position.
func (s *dvdSession) placeStart(m *Media, ts *wittydvd.TitleSummary, sec float64) error {
	loc, err := s.locateIn(ts, sec)
	if err != nil {
		return err
	}
	m.StartSec, m.StartOffset, m.StartAt, m.StartPTS = sec, loc.Offset, loc.Sec, loc.PTS
	return nil
}

// Locate finds the VOBU to start decoding from for a title position.
func (s *dvdSession) Locate(sec float64) (Location, error) {
	if s.playing == nil {
		return Location{Sec: 0, PTS: -1}, nil
	}
	return s.locateIn(s.playing.ts, sec)
}

// navAt reads the first NAV pack at or after a position (in sectors of the
// cell's runs), reading little: on a real drive every seek costs.
func navAt(rd wittydvd.SectorReader, runs []wittydvd.SectorRange, index int64) (at int64, ptm uint32, ok bool) {
	const chunk = 32
	for hops := 0; hops < 32; hops++ {
		r, off, found := runAt(runs, index)
		if !found {
			return 0, 0, false
		}
		n := int64(r.Last-r.First+1) - off
		if n > chunk {
			n = chunk
		}
		buf, err := rd.ReadSectors(r.First+uint32(off), int(n))
		if err != nil || int64(len(buf)) < wittydvd.Sector {
			return 0, 0, false
		}
		for _, nv := range wittydvd.ScanSectors(buf, uint32(index)).Navs {
			if nv.PCI != nil {
				return int64(nv.Sector), nv.PCI.VOBUStartPTM, true
			}
		}
		index += n
	}
	return 0, 0, false
}

func runAt(runs []wittydvd.SectorRange, index int64) (wittydvd.SectorRange, int64, bool) {
	for _, r := range runs {
		n := int64(r.Last - r.First + 1)
		if index < n {
			return r, index, true
		}
		index -= n
	}
	return wittydvd.SectorRange{}, 0, false
}

func (s *dvdSession) locateIn(ts *wittydvd.TitleSummary, sec float64) (Location, error) {
	sp := wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: ts.VTS}
	var base int64
	k := 0
	for i, c := range ts.Cells {
		if c.Start > sec+0.01 {
			break
		}
		k = i
	}
	for _, c := range ts.Cells[:k] {
		ext, err := s.opened.CellExtents(sp, c.Cell)
		if err != nil {
			return Location{}, err
		}
		for _, e := range ext {
			base += e.Length
		}
	}
	cell := ts.Cells[k]
	here := Location{Offset: base, Sec: cell.Start, PTS: -1}
	into := sec - cell.Start
	if into < 1 {
		return here, nil
	}
	runs, err := s.opened.CellRanges(sp, cell.Cell)
	if err != nil {
		return Location{}, err
	}
	var sectors int64
	for _, r := range runs {
		sectors += int64(r.Last - r.First + 1)
	}
	rd, err := s.opened.Reader(sp)
	if err != nil {
		return Location{}, err
	}
	_, first, ok := navAt(rd, runs, 0)
	if !ok || sectors <= 0 || cell.Cell.Duration <= 0 {
		return here, nil
	}
	target := float64(first) + into*90000
	// Narrow in on the VOBU by interpolating between what is known: a few reads
	// are enough, since bit rates change slowly. Interpolation alone can creep
	// towards the answer from one side, so a repeat on the same side halves.
	lo, loPTM := int64(0), float64(first)
	hi, hiPTM := sectors, float64(first)+cell.Cell.Duration*90000
	best := Location{Offset: base, Sec: cell.Start, PTS: int64(first)}
	side := 0
	for i := 0; i < 14 && hi-lo > 64 && hiPTM > loPTM; i++ {
		guess := lo + int64(float64(hi-lo)*(target-loPTM)/(hiPTM-loPTM))
		if side > 1 || side < -1 {
			guess = lo + (hi-lo)/2
		}
		if guess <= lo {
			guess = lo + 1
		}
		if guess >= hi {
			guess = hi - 1
		}
		at, ptm, found := navAt(rd, runs, guess)
		if !found || at >= hi {
			hi, hiPTM = guess, loPTM+(hiPTM-loPTM)*float64(guess-lo)/float64(hi-lo)
			side = min(side, 0) - 1
			continue
		}
		if float64(ptm) <= target {
			lo, loPTM = at, float64(ptm)
			side = max(side, 0) + 1
			best = Location{Offset: base + at*wittydvd.Sector, Sec: cell.Start + (float64(ptm)-float64(first))/90000, PTS: int64(ptm)}
			if target-float64(ptm) < 90000 {
				break // under a second to decode through
			}
		} else {
			hi, hiPTM = at, float64(ptm)
			side = min(side, 0) - 1
		}
	}
	return best, nil
}

// ---------------------------------------------------------------------------
// Pictures
// ---------------------------------------------------------------------------

var pngEnc = png.Encoder{CompressionLevel: png.BestSpeed}

func encodePNG(r *wittydvd.RGBA, w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	if r != nil {
		// Copy whole rows (clipped to the frame) instead of per-pixel sets.
		for y := 0; y < r.Height; y++ {
			fy := r.Y + y
			if fy < 0 || fy >= h {
				continue
			}
			x0, x1, sx := r.X, r.X+r.Width, 0
			if x0 < 0 {
				sx, x0 = -x0, 0
			}
			if x1 > w {
				x1 = w
			}
			if x1 <= x0 {
				continue
			}
			copy(img.Pix[fy*img.Stride+x0*4:], r.Pix[(y*r.Width+sx)*4:(y*r.Width+sx+x1-x0)*4])
		}
	}
	var buf bytes.Buffer
	_ = pngEnc.Encode(&buf, img)
	return buf.Bytes()
}

// Art renders the normal picture of a menu, or one button's selected picture.
func (s *dvdSession) Art(key int, kind string, n, _ int) ([]byte, bool) {
	art := s.menus[key]
	if art == nil {
		return nil, false
	}
	switch kind {
	case "overlay":
		if art.pic.Normal == nil {
			return nil, false
		}
		if art.overlay == nil {
			art.overlay = encodePNG(art.pic.Normal, art.pic.FrameW, art.pic.FrameH)
		}
		return art.overlay, true
	case "highlight":
		if b, ok := art.highlights[n]; ok {
			return b, true
		}
		for _, b := range art.pic.Buttons {
			if b.N == n {
				out := encodePNG(b.Select, art.pic.FrameW, art.pic.FrameH)
				art.highlights[n] = out
				return out, true
			}
		}
	}
	return nil, false
}

// Sound: DVDs have no button sounds.
func (s *dvdSession) Sound(int) ([]byte, bool) { return nil, false }

// Subtitles: a DVD's are all in the stream, decoded as it plays.
func (s *dvdSession) Subtitles(int, int, float64, float64) ([]SubEvent, error) { return nil, nil }

func (s *dvdSession) SubPic(string) ([]byte, bool) { return nil, false }

// uopLocks names what a program chain's prohibited-operation bits forbid.
func uopLocks(m uint32) []string {
	var out []string
	add := func(name string, bits ...uint) {
		for _, b := range bits {
			if m>>b&1 == 1 {
				out = append(out, name)
				return
			}
		}
	}
	add("seek", 0, 5, 8, 9)
	add("chapter", 1)
	add("skip", 6, 7)
	add("pause", 19)
	add("audio", 20)
	add("subtitle", 21)
	add("angle", 22)
	return out
}
