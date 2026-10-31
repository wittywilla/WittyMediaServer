package disc

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittydvd"
)

// bluraySession is one opened Blu-ray.
type bluraySession struct {
	prefs Prefs
	disc  *wittybluray.Disc
	nav   *wittybluray.Nav

	current   *Presentation
	serial    int
	step      *wittybluray.Step
	playlist  *laidOut // what a title presentation plays
	menu      *menuState
	menus     map[int]*bdArt
	artSeq    int
	menuCache map[string]*wittybluray.Menu
	subPics   map[string]*image.NRGBA
	textCues  map[string][]wittybluray.TextCue
	seekTo    float64 // seconds into the playlist: continue here after a stream change
	timer     *wittybluray.NavTimer
	timerAt   time.Time
	// The viewer's subtitle language is applied once, to the first title.
	subsApplied bool
}

// laidOut is a playlist laid out on one timeline.
type laidOut struct {
	pl       *wittybluray.Playlist
	only     int // the one item this presents, or -1 for all
	items    []playedItem
	duration float64
	chapters []float64
}

// playedItem is one play item's place on the timeline and in its clip.
type playedItem struct {
	item     int // its index in the playlist
	clip     string
	stc      int
	start    float64 // seconds into the playlist
	duration float64
	startPTS uint32 // the entry point the stream begins at (45 kHz)
	spn      [2]uint32
	base     int64 // byte offset in the stream
}

type menuState struct {
	menu              *wittybluray.Menu
	page              *wittybluray.Page
	selected          int
	enabled, disabled map[int]bool
	skipEffect        bool // the disc changed page with its transition off
}

type bdArt struct {
	menu       *wittybluray.Menu
	page       *wittybluray.Page
	buttons    []*wittybluray.Button
	overlay    []byte
	highlights map[int][]byte
}

func (p *laidOut) find(item int) *playedItem {
	for i := range p.items {
		if p.items[i].item == item {
			return &p.items[i]
		}
	}
	return nil
}

// openBluray opens a disc and refuses it if it is protected. The engine samples
// the streams; on a drive, a stream it can't read at all counts against the disc
// when it has an AACS folder.
func openBluray(path string, drive bool, prefs Prefs) (*bluraySession, error) {
	d, err := wittybluray.Open(path)
	if err != nil {
		if drive {
			return nil, &RefusalError{Code: CodeUnreadable, Message: MsgDriveUnreadable}
		}
		return nil, err
	}
	if d.Encrypted {
		_ = d.Close()
		return nil, &RefusalError{Code: CodeProtected, Message: MsgBlurayProtected}
	}
	s := &bluraySession{prefs: prefs, disc: d, menus: map[int]*bdArt{}, menuCache: map[string]*wittybluray.Menu{}, subPics: map[string]*image.NRGBA{}}
	s.nav = wittybluray.NewNav(d, wittybluray.NavOptions{Registers: wittybluray.RegisterOptions{
		AudioLang: lang3(prefs.AudioLanguage), SubtitleLang: lang3(prefs.SubtitleLanguage), MenuLang: lang3(prefs.MenuLanguage),
	}})
	return s, nil
}

func (s *bluraySession) Close() error { return s.disc.Close() }

func (s *bluraySession) Info() Info {
	in := Info{Kind: "bluray", Label: s.disc.Label, Titles: []TitleInfo{}}
	if m := s.disc.MainPlaylist(); m != nil {
		in.Main = m.Number
	}
	nums := make([]int, 0, len(s.disc.Playlists))
	for n := range s.disc.Playlists {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	for _, n := range nums {
		pl := s.disc.Playlists[n]
		// Very short playlists are logos and menu backgrounds, not titles.
		if d := s.disc.PlaylistDuration(pl); d >= 60 || n == in.Main {
			in.Titles = append(in.Titles, TitleInfo{N: n, Duration: d, Chapters: len(chapterMarks(pl)), Main: n == in.Main})
		}
	}
	if s.disc.BDJ {
		in.Notes = append(in.Notes, "This disc's menus are Java programs, which WittyMediaPlayer doesn't run yet. It plays the disc's titles instead.")
	}
	return in
}

func (s *bluraySession) Start() *Presentation { return s.show(s.nav.Start()) }

// PlayTitle plays a playlist (ttn is its number; 0 = the main feature) from a
// chapter (1-based; 0 = the start).
func (s *bluraySession) PlayTitle(ttn, chapter int) *Presentation {
	pl := s.disc.Playlist(ttn)
	if pl == nil {
		pl = s.disc.MainPlaylist()
	}
	if pl == nil {
		return s.stop("The disc has nothing to play.")
	}
	item, t := 0, uint32(0)
	if marks := chapterMarks(pl); chapter >= 1 && chapter <= len(marks) {
		item, t = marks[chapter-1].Item, marks[chapter-1].Time
	}
	return s.show(s.nav.PlayPlaylist(pl, item, t))
}

// Do applies an action.
func (s *bluraySession) Do(a Action) *Presentation {
	if st := s.tickTimer(); st != nil {
		return s.show(st)
	}
	s.touchMenuState()
	cur := s.current
	switch a.Type {
	case "tick":
		return cur
	case "press":
		s.syncPosition(a.PositionSec) // pop-up menus read the chapter and time registers
		return s.press(a.Button)
	case "select":
		p := s.selectButton(a.Button)
		s.touchMenuState()
		return p
	case "key":
		s.syncPosition(a.PositionSec)
		return s.keyPress(a.Key)
	case "ended":
		return s.show(s.nav.PlaylistEnded())
	case "title":
		return s.PlayTitle(a.TTN, a.Chapter)
	case "menu":
		s.syncPosition(a.PositionSec)
		return s.show(s.nav.MenuCall())
	case "resume":
		return s.show(s.nav.Resume())
	case "chapter":
		return s.keyPress(map[bool]string{true: "next", false: "prev"}[a.Chapter > 0])
	case "audio":
		s.nav.SetAudio(a.Stream + 1)
		if cur != nil && cur.Kind == "title" && s.step != nil && s.playlist != nil {
			s.seekTo = a.PositionSec
			return s.present(s.step, true)
		}
		return cur
	case "angle":
		s.nav.SetAngle(a.Stream)
		if cur != nil && cur.Kind == "title" && s.step != nil {
			s.seekTo = a.PositionSec
			return s.present(s.step, true)
		}
		return cur
	case "subtitle":
		s.nav.SetSubtitle(a.Stream+1, a.On)
		if cur != nil && cur.Kind == "title" && cur.Title != nil {
			p, tv := *cur, *cur.Title
			tv.SubN = -1
			if a.On {
				tv.SubN = a.Stream
			}
			tv.ForcedStream = a.Stream
			p.Title = &tv
			s.current = &p
			return &p
		}
		return cur
	}
	return cur
}

func lang3(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if len(code) == 3 {
		return code
	}
	m := map[string]string{"en": "eng", "fr": "fra", "de": "deu", "es": "spa", "it": "ita", "nl": "nld", "pt": "por", "ja": "jpn", "zh": "zho", "ko": "kor", "ru": "rus", "sv": "swe", "da": "dan", "no": "nor", "fi": "fin", "pl": "pol", "cs": "ces", "hu": "hun", "el": "ell", "tr": "tur", "ar": "ara", "he": "heb", "th": "tha", "ro": "ron", "ca": "cat"}
	return m[code]
}

// syncTimer follows the navigator's timer: a disc's movie objects and buttons
// can both start or stop it.
func (s *bluraySession) syncTimer() {
	t := s.nav.Timer()
	if t == s.timer {
		return
	}
	s.timer = t
	if t != nil {
		s.timerAt = time.Now().Add(time.Duration(t.Seconds) * time.Second)
	}
}

func (s *bluraySession) tickTimer() *wittybluray.Step {
	s.syncTimer()
	if s.timer == nil || time.Now().Before(s.timerAt) {
		return nil
	}
	s.timer = nil
	return s.nav.NavTimerFired()
}

// touchMenuState tells the disc's registers what is on show: its commands read
// the page and button to find out what was pressed.
func (s *bluraySession) touchMenuState() {
	if ms := s.menu; ms != nil {
		s.nav.SetMenuState(ms.page.ID, ms.selected)
	}
}

// press runs a menu button's commands.
func (s *bluraySession) press(n int) *Presentation {
	ms, cur := s.menu, s.current
	if ms == nil || cur == nil || cur.Menu == nil {
		return cur
	}
	b := ms.page.Button(n - 1)
	if b == nil {
		return cur
	}
	ms.selected = b.ID
	s.touchMenuState()
	st := s.nav.PressButton(b.Commands)
	s.syncTimer()
	if st.Kind != wittybluray.StepStay {
		return s.show(st)
	}
	fx := st.Effects
	if fx != nil && fx.Still != nil {
		if q := s.current; q != nil {
			c := *q
			c.Pause = fx.Still
			s.current = &c
			defer func(c *Presentation) { s.current.Pause = nil }(&c)
			cur = &c
		}
	}
	if s.applyEffects(fx) {
		if cur.Kind == "title" && cur.Menu.Popup {
			// The pop-up menu changed page or buttons: same film, new layer.
			p := *cur
			s.menuView(&p, ms.menu, true, s.videoRate(s.step))
			s.current = &p
			return &p
		}
		return s.present(s.step, true) // the page or its buttons changed
	}
	p := s.moveSelection(ms.selected)
	if fx != nil && fx.PopupOff && p != nil && p.Menu != nil {
		// Tell the player to hide the pop-up, without remembering it as closed.
		q, m := *p, *p.Menu
		m.Closed = true
		q.Menu = &m
		return &q
	}
	return p
}

// applyEffects applies a button's effects to the menu state and says whether the
// screen must be rebuilt.
func (s *bluraySession) applyEffects(fx *wittybluray.Effects) bool {
	ms := s.menu
	if fx == nil || ms == nil {
		return false
	}
	rebuild := false
	if bp := fx.ButtonPage; bp != nil {
		if bp.SetPage {
			if p := ms.menu.Page(int(bp.Page)); p != nil && p != ms.page {
				ms.page = p
				ms.enabled, ms.disabled = map[int]bool{}, map[int]bool{}
				ms.selected = p.DefaultSelected
				ms.skipEffect = bp.PageEffectOff
				rebuild = true
			}
		}
		if bp.SetButton {
			ms.selected = int(bp.Button)
		}
	}
	for _, id := range fx.EnableButton {
		// A group shows one button at a time: enabling one replaces the others.
		for i := range ms.page.BOGs {
			g := &ms.page.BOGs[i]
			has := false
			for j := range g.Buttons {
				has = has || g.Buttons[j].ID == int(id)
			}
			if has {
				for j := range g.Buttons {
					delete(ms.enabled, g.Buttons[j].ID)
				}
			}
		}
		ms.enabled[int(id)] = true
		delete(ms.disabled, int(id))
		rebuild = true
	}
	for _, id := range fx.DisableButton {
		ms.disabled[int(id)] = true
		delete(ms.enabled, int(id))
		rebuild = true
	}
	return rebuild
}

func (s *bluraySession) selectButton(n int) *Presentation {
	if s.menu == nil {
		return s.current
	}
	return s.moveSelection(n - 1)
}

func (s *bluraySession) moveSelection(id int) *Presentation {
	ms, cur := s.menu, s.current
	if ms == nil || cur == nil || cur.Menu == nil {
		return cur
	}
	ms.selected = id
	p, m := *cur, *cur.Menu
	m.Selected = id + 1
	p.Menu = &m
	s.current = &p
	return &p
}

func (s *bluraySession) keyPress(key string) *Presentation {
	cur := s.current
	switch key {
	case "menu", "root", "title":
		return s.show(s.nav.MenuCall())
	case "resume":
		return s.show(s.nav.Resume())
	case "return":
		return cur
	case "next", "prev":
		// A slideshow's pictures, one at a time.
		d := 1
		if key == "prev" {
			d = -1
		}
		if st, ok := s.nav.SkipItem(d); ok {
			return s.show(st)
		}
		return cur
	}
	ms := s.menu
	if ms == nil || cur == nil || cur.Menu == nil || len(cur.Menu.Buttons) == 0 {
		return cur
	}
	sel := ms.page.Button(ms.selected)
	if sel == nil {
		if vis := ms.page.Visible(ms.enabled, ms.disabled); len(vis) > 0 {
			sel = vis[0]
		} else {
			return cur
		}
	}
	next := wittybluray.NoButton
	switch key {
	case "up":
		next = sel.Up
	case "down":
		next = sel.Down
	case "left":
		next = sel.Left
	case "right":
		next = sel.Right
	case "enter", "ok", "select":
		return s.press(sel.ID + 1)
	default:
		return cur
	}
	if next == wittybluray.NoButton || ms.page.Button(next) == nil {
		return cur
	}
	// A hidden or disabled neighbour cannot take the selection.
	visible := false
	for _, b := range ms.page.Visible(ms.enabled, ms.disabled) {
		visible = visible || b.ID == next
	}
	if !visible {
		return cur
	}
	if b := ms.page.Button(next); b != nil && b.AutoAction {
		return s.press(next + 1)
	}
	return s.moveSelection(next)
}

func (s *bluraySession) stop(reason string) *Presentation {
	s.serial++
	p := stopPresentation(s.serial, reason)
	s.current, s.menu, s.playlist = p, nil, nil
	return p
}

func (s *bluraySession) show(st *wittybluray.Step) *Presentation { return s.present(st, false) }

// present turns a step into a presentation; rebuild makes a new screen even for
// the same step (stream change, menu page change).
func (s *bluraySession) present(st *wittybluray.Step, rebuild bool) *Presentation {
	if st == nil {
		return s.current
	}
	switch st.Kind {
	case wittybluray.StepStay:
		return s.current
	case wittybluray.StepStop:
		return s.stop(st.Reason)
	case wittybluray.StepUnsupported:
		return s.presentBDJ(st, rebuild)
	}
	if st.Playlist == nil {
		return s.stop("There is nothing to play here.")
	}
	if !rebuild {
		s.seekTo = 0
	}
	// A playlist whose menu is always on is a menu. A playlist with a pop-up menu
	// is a title that carries the menu for the player's Menu key. Playlists that
	// are not a numbered title (first-play intros, menu transitions) present like
	// menus without buttons: they play through.
	var popup *wittybluray.Menu
	if st.Menu {
		switch menu := s.loadMenu(st); {
		case menu == nil:
			st.Menu = false
		case menu.Popup:
			st.Menu, popup = false, menu
		}
	}
	s.serial++
	s.step = st
	p := &Presentation{Serial: s.serial, Reason: st.Reason}
	var err error
	if st.Menu || st.Title == 0xffff || st.Title == 0 {
		if !rebuild {
			s.menu = nil
		}
		err = s.buildMenu(st, p)
	} else {
		if !rebuild || popup == nil {
			s.menu = nil
		}
		if err = s.buildTitle(st, p); err == nil && popup != nil {
			s.menuView(p, popup, true, s.videoRate(st))
		}
	}
	if err != nil {
		s.current = stopPresentation(s.serial, plainReason(err))
		return s.current
	}
	s.syncTimer()
	if s.timer != nil {
		if left := time.Until(s.timerAt).Seconds(); left > 0 {
			p.TimerSec = left
		}
	}
	s.current = p
	return p
}

// presentBDJ plays what a Java title's own object says it plays, else the main
// feature: this player runs no Java.
func (s *bluraySession) presentBDJ(st *wittybluray.Step, rebuild bool) *Presentation {
	var pl *wittybluray.Playlist
	if st.Title >= 1 && st.Title != 0xffff {
		if o, err := s.disc.BDJObject(st.BDJ); err == nil && len(o.Playlists) > 0 {
			pl = s.disc.Playlist(o.Playlists[0])
		}
	}
	if pl == nil {
		pl = s.disc.MainPlaylist()
	}
	if pl == nil {
		return s.stop(st.Reason)
	}
	st2 := s.nav.PlayPlaylist(pl, 0, 0)
	st2.Reason = "This disc's menu is a Java program, which WittyMediaPlayer doesn't run yet. Playing the main feature."
	return s.present(st2, rebuild)
}

// layout puts a playlist's items on one timeline, each starting at the entry
// point at or before its IN time. only picks one item (-1 = all).
func (s *bluraySession) layout(pl *wittybluray.Playlist, only int) *laidOut {
	out := &laidOut{pl: pl, only: only}
	angle := int(s.nav.Registers().PSR[wittybluray.PSRAngle])
	var t float64
	var base int64
	for i := range pl.Items {
		if only >= 0 && i != only {
			continue
		}
		it := &pl.Items[i]
		clipName := it.Clip
		if angle >= 2 && angle <= len(it.Angles) {
			clipName = it.Angles[angle-1].Clip
		}
		c := s.disc.Clip(clipName)
		if c == nil {
			continue
		}
		pi := playedItem{item: i, clip: clipName, stc: it.STCID, start: t, startPTS: it.In, spn: [2]uint32{0, c.Packets}, base: base}
		if f, to, ok := c.STCRange(it.STCID); ok {
			pi.spn = [2]uint32{f, to}
		}
		if e, ok := c.EntryAt(c.VideoPID(), it.STCID, it.In); ok {
			pi.startPTS, pi.spn[0] = e.PTS, e.SPN
		}
		if e, ok := c.EntryAt(c.VideoPID(), it.STCID, it.Out); ok && e.SPN > pi.spn[0] && it.Out < c.EndPTS(it.STCID) {
			pi.spn[1] = e.SPN
		}
		if it.Out > pi.startPTS {
			pi.duration = float64(it.Out-pi.startPTS) / 45000
		}
		if it.StillMode != 0 && pi.duration < 1 {
			pi.duration = 1 // a still is often one frame; a stream needs some length
		}
		out.items = append(out.items, pi)
		t += pi.duration
		base += int64(pi.spn[1]-pi.spn[0]) * 192
	}
	out.duration = t
	for _, m := range chapterMarks(pl) {
		if pi := out.find(m.Item); pi != nil {
			out.chapters = append(out.chapters, pi.start+float64(int64(m.Time)-int64(pi.startPTS))/45000)
		}
	}
	return out
}

func chapterMarks(pl *wittybluray.Playlist) []wittybluray.Mark {
	var out []wittybluray.Mark
	for _, m := range pl.Marks {
		if m.Type == 1 {
			out = append(out, m)
		}
	}
	return out
}

// clipExtents is a stretch of a clip's stream file.
func (s *bluraySession) clipExtents(clip string, fromSPN, toSPN uint32) ([]wittydvd.FileExtent, error) {
	ext, err := s.disc.ClipExtents(clip)
	if err != nil {
		return nil, err
	}
	from, to := int64(fromSPN)*192, int64(toSPN)*192
	var cut []wittydvd.FileExtent
	var off int64
	for _, e := range ext {
		a, b := off, off+e.Length
		off = b
		lo, hi := max(a, from), min(b, to)
		if hi <= lo {
			continue
		}
		cut = append(cut, wittydvd.FileExtent{File: e.Path, Offset: e.Offset + (lo - a), Length: hi - lo})
	}
	if len(cut) == 0 {
		return nil, fmt.Errorf("clip %s is missing from the disc", clip)
	}
	return cut, nil
}

// isInterlaced: 480i, 576i and 1080i discs.
func isInterlaced(it *wittybluray.PlayItem) bool {
	if len(it.Video) == 0 {
		return false
	}
	switch it.Video[0].Format {
	case 1, 2, 4:
		return true
	}
	return false
}

// aspectOf is a play item's picture shape: standard-definition material on a
// Blu-ray can be either.
func (s *bluraySession) aspectOf(it *wittybluray.PlayItem) string {
	if len(it.Video) > 0 {
		if c := s.disc.Clip(it.Clip); c != nil {
			if cs := c.Stream(it.Video[0].PID); cs != nil && cs.Aspect == 2 {
				return "4:3"
			}
		}
	}
	return "16:9"
}

func frameSize(format int) (int, int) {
	switch format {
	case 1, 3:
		return 720, 480
	case 2, 7:
		return 720, 576
	case 5:
		return 1280, 720
	case 8:
		return 3840, 2160
	}
	return 1920, 1080
}

// buildMedia lays out the stream of a playlist.
func (s *bluraySession) buildMedia(laid *laidOut, it *wittybluray.PlayItem, audio *wittybluray.Stream) (*Media, error) {
	if len(it.Video) == 0 && audio == nil {
		return nil, errors.New("this playlist has neither picture nor sound")
	}
	var parts [][]wittydvd.FileExtent
	var durations []float64
	var bases []int64
	for _, pi := range laid.items {
		ext, err := s.clipExtents(pi.clip, pi.spn[0], pi.spn[1])
		if err != nil {
			return nil, err
		}
		parts = append(parts, ext)
		durations = append(durations, pi.duration)
		bases = append(bases, int64(pi.startPTS)*2)
	}
	if len(parts) == 0 {
		return nil, errors.New("this playlist has no clips")
	}
	m := newMedia("mpegts", "", parts, durations)
	for i := range m.Segments {
		m.Segments[i].BasePTS = bases[i]
	}
	angle := s.nav.Registers().PSR[wittybluray.PSRAngle]
	m.Token = fmt.Sprintf("p%05d-g%d", laid.pl.Number, angle)
	if laid.only >= 0 {
		m.Token += fmt.Sprintf("-i%d", laid.only)
	}
	if len(it.Video) > 0 {
		m.Video = int(it.Video[0].PID)
	}
	// A sound kept outside the main stream (a browsable slideshow's music) isn't
	// played yet: the picture carries on without it.
	if audio != nil && audio.EntryType == 1 {
		m.Audio = int(audio.PID)
		m.Token += fmt.Sprintf("-a%x", m.Audio)
	}
	m.Duration, m.Deinterlace = laid.duration, isInterlaced(it)
	m.Aspect = s.aspectOf(it)
	return m, nil
}

// audioFor picks the audio stream for the first item: the register's choice when
// it is in the clip, else the first.
func (s *bluraySession) audioFor(it *wittybluray.PlayItem) (int, *wittybluray.Stream) {
	n := int(s.nav.Registers().PSR[wittybluray.PSRAudio])
	if n >= 1 && n <= len(it.Audio) {
		return n - 1, &it.Audio[n-1]
	}
	if len(it.Audio) > 0 {
		s.nav.SetAudio(1)
		return 0, &it.Audio[0]
	}
	return -1, nil
}

// loadMenu reads (and caches) the step's menu graphics; nil when there are none
// or they cannot be read.
func (s *bluraySession) loadMenu(st *wittybluray.Step) *wittybluray.Menu {
	if !st.Menu || st.Item >= len(st.Playlist.Items) {
		return nil
	}
	clip, limit := st.IGClip, uint32(0)
	if clip == "" {
		clip, limit = st.Playlist.Items[st.Item].Clip, 200000
	}
	if s.disc.Clip(clip) == nil {
		return nil
	}
	key := fmt.Sprintf("%s-%x", clip, st.IGPID)
	if m, ok := s.menuCache[key]; ok {
		return m
	}
	m, err := s.disc.LoadMenu(clip, st.IGPID, limit)
	if err != nil || len(m.Pages) == 0 {
		m = nil
	}
	s.menuCache[key] = m
	return m
}

func onlyItem(st *wittybluray.Step) int {
	if st.ItemOnly {
		return st.Item
	}
	return -1
}

func (s *bluraySession) buildMenu(st *wittybluray.Step, p *Presentation) error {
	pl := st.Playlist
	if st.Item >= len(pl.Items) {
		return errors.New("the menu's playlist is empty")
	}
	it := &pl.Items[st.Item]
	var menu *wittybluray.Menu
	if st.Menu {
		if menu = s.loadMenu(st); menu == nil {
			return errors.New("the menu graphics could not be read")
		}
	} else {
		s.menu = nil
	}
	laid := s.layout(pl, onlyItem(st))
	_, audio := s.audioFor(it)
	m, err := s.buildMedia(laid, it, audio)
	if err != nil {
		return err
	}
	p.Kind = "menu"
	// A menu loops only if the disc's program plays it again when it ends; a menu
	// that moves on by itself (to the film, after a while) must get to.
	m.Loop = st.StillSec == 0 && s.nav.LoopsAtEnd(st)
	m.StillSec = st.StillSec
	p.Media = m
	if menu == nil {
		p.Menu = &Menu{Width: 1920, Height: 1080, Aspect: s.aspectOf(it), Buttons: []ButtonView{}}
		return nil
	}
	s.menuView(p, menu, false, s.videoRate(st))
	if v := p.Menu; v != nil {
		// Selection ends by itself at its time, pressing the default button; the
		// menu's buttons go when the composition ends. The times are the stream's:
		// this stream starts at the item's first entry point.
		base := float64(it.In) / 45000
		if pi := laid.find(st.Item); pi != nil {
			base = float64(pi.startPTS) / 45000
		}
		page := s.menu.page
		if t := menu.SelectionTimeout - base; menu.SelectionTimeout > 0 && t > 0 && t < laid.duration && page.Button(page.DefaultActivated) != nil {
			v.ForcedAction, v.ActionAtSec = page.DefaultActivated+1, t
		}
		if t := menu.Timeout - base; menu.Timeout > 0 && t > 0 && t < laid.duration-0.5 {
			v.ButtonsUntilSec = t
		}
	}
	return nil
}

// menuView fills p.Menu with the current page of a menu. popup marks a menu the
// player shows only on its Menu key. Each view gets its own art key, so a page
// change under the same serial still refreshes the pictures.
func (s *bluraySession) menuView(p *Presentation, menu *wittybluray.Menu, popup bool, rate int) {
	if s.menu == nil || s.menu.menu != menu {
		s.menu = &menuState{menu: menu, page: &menu.Pages[0], selected: menu.Pages[0].DefaultSelected, enabled: map[int]bool{}, disabled: map[int]bool{}}
	}
	ms := s.menu
	buttons := ms.page.Visible(ms.enabled, ms.disabled)
	if ms.page.Button(ms.selected) == nil {
		// A page may name no default: the disc's last selection, if it is here.
		psr := int(s.nav.Registers().PSR[wittybluray.PSRButton])
		ms.selected = wittybluray.NoButton
		for _, b := range buttons {
			if b.ID == psr {
				ms.selected = psr
			}
		}
		if ms.selected == wittybluray.NoButton && len(buttons) > 0 {
			ms.selected = buttons[0].ID
		}
	}
	s.touchMenuState()
	s.artSeq = s.artSeq%999 + 1
	key := s.serial*1000 + s.artSeq
	s.menus[key] = &bdArt{menu: menu, page: ms.page, buttons: buttons, highlights: map[int][]byte{}}
	for k := range s.menus {
		if k < (s.serial-4)*1000 {
			delete(s.menus, k)
		}
	}
	m := &Menu{Width: menu.Width, Height: menu.Height, Aspect: "16:9", Selected: ms.selected + 1, Art: key, HasOverlay: true, HasHighlights: true, Buttons: []ButtonView{}, Popup: popup, InTitle: popup}
	if menu.Width > 0 && menu.Height > 0 && menu.Width*3 == menu.Height*4 {
		m.Aspect = "4:3"
	}
	if st := s.step; st != nil && st.Playlist != nil && st.Item < len(st.Playlist.Items) && s.aspectOf(&st.Playlist.Items[st.Item]) == "4:3" {
		m.Aspect = "4:3"
	}
	if popup && menu.UserTimeout > 0 {
		m.UserTimeoutSec = menu.UserTimeout
	}
	m.HasSounds = s.disc.Sounds() != nil
	if !ms.skipEffect {
		for i := range ms.page.In.Effects {
			m.InEffect = append(m.InEffect, ms.page.In.Effects[i].DurationSec)
		}
	}
	ms.skipEffect = false
	fps := animationFPS(ms.page, rate, s.disc.Index.FrameRate)
	for _, b := range buttons {
		r := menu.Rect(b)
		bv := ButtonView{N: b.ID + 1, X1: r.Min.X, Y1: r.Min.Y, X2: r.Max.X, Y2: r.Max.Y,
			Up: ref(b.Up), Down: ref(b.Down), Left: ref(b.Left), Right: ref(b.Right), Auto: b.AutoAction, Lit: b.Selected.Start != wittybluray.NoButton}
		if b.NumericSelect != wittybluray.NoButton && b.NumericSelect > 0 {
			bv.Num = b.NumericSelect
		}
		if b.SelectedSound != wittybluray.NoSound {
			bv.SoundSelect = b.SelectedSound + 1
		}
		if b.ActivatedSound != wittybluray.NoSound {
			bv.SoundPress = b.ActivatedSound + 1
		}
		if n := b.Selected.Frames(); n > 1 && fps > 0 {
			bv.Frames, bv.FPS = n, fps
		}
		m.Buttons = append(m.Buttons, bv)
	}
	p.Menu = m
}

// animationFPS is how fast a page's button animations run: the video frame rate
// divided by the page's animation rate code (0 = no animation).
func animationFPS(page *wittybluray.Page, rate, indexRate int) float64 {
	if page.AnimationRate <= 0 {
		return 0
	}
	rates := map[int]float64{1: 23.976, 2: 24, 3: 25, 4: 29.97, 6: 50, 7: 59.94}
	fps := rates[rate]
	if fps == 0 {
		fps = rates[indexRate]
	}
	if fps == 0 {
		fps = 24
	}
	return fps / float64(page.AnimationRate)
}

func (s *bluraySession) videoRate(st *wittybluray.Step) int {
	if st != nil && st.Playlist != nil && st.Item < len(st.Playlist.Items) && len(st.Playlist.Items[st.Item].Video) > 0 {
		return st.Playlist.Items[st.Item].Video[0].Rate
	}
	return 0
}

func ref(id int) int {
	if id == wittybluray.NoButton {
		return 0
	}
	return id + 1
}

func (s *bluraySession) buildTitle(st *wittybluray.Step, p *Presentation) error {
	pl := st.Playlist
	if st.Item >= len(pl.Items) {
		return errors.New("the playlist is empty")
	}
	laid := s.layout(pl, onlyItem(st))
	s.playlist = laid
	it := &pl.Items[st.Item]
	audioN, audio := s.audioFor(it)
	m, err := s.buildMedia(laid, it, audio)
	if err != nil {
		return err
	}
	m.StillSec = st.StillSec
	start := 0.0
	if pi := laid.find(st.Item); pi != nil {
		start = pi.start
		if st.Time > pi.startPTS {
			start += float64(st.Time-pi.startPTS) / 45000
		}
	}
	if s.seekTo > 0 {
		start = s.seekTo
	}
	s.seekTo = 0
	if start > 0 {
		if loc, err := s.locateLaid(laid, start); err == nil {
			m.StartSec, m.StartOffset, m.StartAt, m.StartPTS = start, loc.Offset, loc.Sec, loc.PTS
		}
	}
	regs := s.nav.Registers()
	// The viewer's subtitle language turns subtitles on once, for the first
	// title; after that the disc's menus and the viewer's choices rule.
	if want := lang3(s.prefs.SubtitleLanguage); want != "" && !s.subsApplied {
		for i := range it.PG {
			if it.PG[i].Lang == want {
				s.nav.SetSubtitle(i+1, true)
				break
			}
		}
	}
	s.subsApplied = true
	subN := -1
	pg := regs.PSR[wittybluray.PSRPG]
	if g := int(pg & 0xfff); pg>>31 == 1 && g >= 1 && g <= len(it.PG) {
		subN = g - 1
	}
	// Forced text comes from the subtitle stream in the language being heard;
	// failing that, the stream the disc has selected.
	forced := subN
	if forced < 0 {
		if g := int(pg & 0xfff); g >= 1 && g <= len(it.PG) {
			forced = g - 1
		}
		if audio != nil {
			for i := range it.PG {
				if it.PG[i].Lang == audio.Lang {
					forced = i
					break
				}
			}
		}
	}
	chapter := 1
	for i, c := range laid.chapters {
		if c <= start+0.5 {
			chapter = i + 1
		}
	}
	fw, fh := 1920, 1080
	if len(it.Video) > 0 {
		fw, fh = frameSize(it.Video[0].Format)
	}
	p.Kind = "title"
	p.Media = m
	tv := &TitleView{TTN: pl.Number, Chapter: chapter, Duration: laid.duration, StartSec: start, Chapters: laid.chapters, AudioN: audioN, SubN: subN,
		Audio: []Track{}, Subtitles: []Track{}, FrameW: fw, FrameH: fh, Aspect: m.Aspect, ForcedStream: forced, Angle: int(regs.PSR[wittybluray.PSRAngle]), Angles: len(it.Angles), Locks: s.locks(pl)}
	if tv.Chapters == nil {
		tv.Chapters = []float64{}
	}
	for i, a := range it.Audio {
		ch := 2
		switch a.Format {
		case 1:
			ch = 1
		case 6, 12:
			ch = 6
		}
		tv.Audio = append(tv.Audio, Track{N: i, Language: a.Lang, Codec: a.CodingName(), Channels: ch, ID: int(a.PID)})
	}
	for i, sub := range it.PG {
		t := Track{N: i, Language: sub.Lang, ID: int(sub.PID)}
		if sub.Coding == 0x92 || sub.EntryType != 1 {
			t.ID, t.External = 0, true
		}
		tv.Subtitles = append(tv.Subtitles, t)
	}
	p.Title = tv
	return nil
}

// locks names what the disc forbids in this playlist and play item.
func (s *bluraySession) locks(pl *wittybluray.Playlist) []string {
	var out []string
	add := func(name string, uo uint64) {
		if s.nav.Forbids(uo) {
			out = append(out, name)
		}
	}
	add("seek", wittybluray.UOTimeSearch)
	if !pl.RandomAccess && (len(out) == 0 || out[len(out)-1] != "seek") {
		out = append(out, "seek")
	}
	add("chapter", wittybluray.UOChapterSearch)
	add("skip", wittybluray.UOSkipToNext)
	add("pause", wittybluray.UOPauseOn)
	add("audio", wittybluray.UOPrimaryAudio)
	add("subtitle", wittybluray.UOPGChange)
	add("angle", wittybluray.UOAngle)
	return out
}

// syncPosition records where the viewer is in the title, for Resume.
func (s *bluraySession) syncPosition(pos float64) {
	pl := s.playlist
	if pl == nil || s.current == nil || s.current.Kind != "title" || pos <= 0 {
		return
	}
	for i := len(pl.items) - 1; i >= 0; i-- {
		pi := pl.items[i]
		if pi.start <= pos+0.01 {
			s.nav.SetTime(pi.item, pi.startPTS+uint32((pos-pi.start)*45000))
			break
		}
	}
	for i, c := range pl.chapters {
		if c <= pos+0.5 {
			s.nav.SetChapter(i + 1)
		}
	}
}

// Locate finds the entry point to start decoding from for a playlist position.
// The clip's own map has it, so the disc isn't touched.
func (s *bluraySession) Locate(sec float64) (Location, error) {
	if s.playlist == nil {
		return Location{Sec: 0, PTS: -1}, nil
	}
	return s.locateLaid(s.playlist, sec)
}

func (s *bluraySession) locateLaid(laid *laidOut, sec float64) (Location, error) {
	if len(laid.items) == 0 {
		return Location{}, errors.New("the playlist has no clips")
	}
	pi := laid.items[0]
	for _, x := range laid.items {
		if x.start <= sec+0.01 {
			pi = x
		}
	}
	loc := Location{Offset: pi.base, Sec: pi.start, PTS: int64(pi.startPTS) * 2}
	c := s.disc.Clip(pi.clip)
	if c == nil {
		return loc, nil
	}
	want := pi.startPTS + uint32((sec-pi.start)*45000)
	if e, ok := c.EntryAt(c.VideoPID(), pi.stc, want); ok && e.SPN >= pi.spn[0] && e.SPN < pi.spn[1] && e.PTS >= pi.startPTS {
		loc.Offset = pi.base + int64(e.SPN-pi.spn[0])*192
		loc.Sec = pi.start + float64(e.PTS-pi.startPTS)/45000
		loc.PTS = int64(e.PTS) * 2
	}
	return loc, nil
}

func encodeImage(img image.Image) []byte {
	var buf bytes.Buffer
	_ = pngEnc.Encode(&buf, img)
	return buf.Bytes()
}

// Art renders a menu page with every button in its normal state, one button's
// selected state, or one picture of the page's opening transition.
func (s *bluraySession) Art(key int, kind string, n, f int) ([]byte, bool) {
	art := s.menus[key]
	if art == nil {
		return nil, false
	}
	switch kind {
	case "overlay":
		if art.overlay == nil {
			art.overlay = encodeImage(art.menu.Render(art.page, art.buttons, wittybluray.NoButton))
		}
		return art.overlay, true
	case "highlight":
		if f < 0 || f > 256 {
			return nil, false
		}
		k := n*1000 + f
		if b, ok := art.highlights[k]; ok {
			return b, true
		}
		for _, b := range art.buttons {
			if b.ID == n-1 {
				img := image.NewNRGBA(image.Rect(0, 0, art.menu.Width, art.menu.Height))
				art.menu.DrawFrame(img, art.page, b, wittybluray.ButtonSelected, f)
				out := encodeImage(img)
				art.highlights[k] = out
				return out, true
			}
		}
	case "effect":
		if n >= 0 && n < len(art.page.In.Effects) {
			return encodeImage(art.menu.DrawEffect(&art.page.In, n)), true
		}
	}
	return nil, false
}

// Sound is a button sound as a WAV file.
func (s *bluraySession) Sound(id int) ([]byte, bool) {
	snd := s.disc.Sounds()
	if snd == nil {
		return nil, false
	}
	return snd.WAV(id)
}

const maxSubPics = 3000

// Subtitles returns the subtitle events kept outside the main stream (text
// subtitles and sub-paths) of stream n that overlap [from, to] seconds.
func (s *bluraySession) Subtitles(ttn, n int, from, to float64) ([]SubEvent, error) {
	pl := s.disc.Playlist(ttn)
	if pl == nil {
		return nil, errors.New("no such playlist")
	}
	laid := s.playlist
	if laid == nil || laid.pl != pl {
		laid = s.layout(pl, -1)
	}
	out := []SubEvent{}
	for _, pi := range laid.items {
		if pi.start+pi.duration < from || pi.start > to {
			continue
		}
		it := &pl.Items[pi.item]
		if n < 0 || n >= len(it.PG) {
			continue
		}
		st := it.PG[n]
		if st.Coding == 0x92 {
			out = append(out, s.textEvents(pl, pi, &st, from, to)...)
			continue
		}
		// A stream kept in a sub-path has its own clip and its own clock: its IN
		// time meets the main path at the sub play item's sync time.
		clip, stc := pi.clip, pi.stc
		clipBase := pi.start - float64(pi.startPTS)/45000 // playlist seconds at the clip's time 0
		if st.EntryType != 1 {
			sp := subPlayItem(pl, &st, pi.item)
			if sp == nil {
				continue
			}
			clip, stc = pl.SubClip(st), sp.STCID
			clipBase = pi.start + (float64(sp.SyncStart)-float64(pi.startPTS)-float64(sp.In))/45000
		}
		fromPTS := uint32(0)
		if t := from - clipBase; t > 0 {
			fromPTS = uint32(t * 45000)
		}
		end := min(to, pi.start+pi.duration)
		toPTS := uint32(0)
		if t := end - clipBase; t > 0 {
			toPTS = uint32(t * 45000)
		}
		win, err := s.disc.SubtitlesIn(clip, st.PID, stc, fromPTS, toPTS)
		if err != nil {
			return nil, err
		}
		for k, sub := range win.Subtitles {
			start := clipBase + float64(sub.Start)/45000
			stop := start + 5
			if sub.End > sub.Start {
				stop = clipBase + float64(sub.End)/45000
			} else if k+1 < len(win.Subtitles) {
				stop = clipBase + float64(win.Subtitles[k+1].Start)/45000
			}
			stop = min(stop, pi.start+pi.duration)
			if stop < from || start > to {
				continue
			}
			id := fmt.Sprintf("%s-%x-%d-%d", clip, st.PID, sub.Start, k)
			if len(s.subPics) >= maxSubPics {
				s.subPics = map[string]*image.NRGBA{}
			}
			s.subPics[id] = sub.Image
			out = append(out, SubEvent{Start: start, End: stop, X: sub.X, Y: sub.Y, W: sub.Image.Rect.Dx(), H: sub.Image.Rect.Dy(), Forced: sub.Forced, Pic: id})
		}
	}
	return out, nil
}

// subPlayItem finds the sub play item that goes with a main-path item.
func subPlayItem(pl *wittybluray.Playlist, st *wittybluray.Stream, item int) *wittybluray.SubPlayItem {
	if st.SubPath >= len(pl.SubPaths) || len(pl.SubPaths[st.SubPath].Items) == 0 {
		return nil
	}
	items := pl.SubPaths[st.SubPath].Items
	for i := range items {
		if items[i].SyncItem == item {
			return &items[i]
		}
	}
	return &items[0]
}

// textEvents lists the dialogs of a text subtitle stream in a time window.
func (s *bluraySession) textEvents(pl *wittybluray.Playlist, pi playedItem, st *wittybluray.Stream, from, to float64) []SubEvent {
	sp := subPlayItem(pl, st, pi.item)
	if sp == nil {
		return nil
	}
	clip := pl.SubClip(*st)
	key := fmt.Sprintf("%s-%x", clip, st.PID)
	cues, ok := s.textCues[key]
	if !ok {
		cues, _ = s.disc.TextSubtitles(clip, st.PID, st.CharCode)
		if s.textCues == nil {
			s.textCues = map[string][]wittybluray.TextCue{}
		}
		s.textCues[key] = cues
	}
	base := pi.start + (float64(sp.SyncStart)-float64(pi.startPTS)-float64(sp.In))/45000
	var out []SubEvent
	for i := range cues {
		c := &cues[i]
		start, stop := base+float64(c.Start)/45000, base+float64(c.End)/45000
		if stop < from || start > to || start > pi.start+pi.duration || c.Text == "" {
			continue
		}
		y := c.Style
		out = append(out, SubEvent{Start: start, End: stop, X: y.X, Y: y.Y, W: y.W, H: y.H, Forced: c.Forced, Text: c.Text,
			Color: hexColor(y.Color), Size: y.FontSize, HAlign: y.HAlign, VAlign: y.VAlign, Bold: y.Bold, Italic: y.Italic})
	}
	return out
}

func hexColor(c color.RGBA) string {
	if c.A == 0 {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// SubPic is a subtitle picture from a Subtitles call.
func (s *bluraySession) SubPic(key string) ([]byte, bool) {
	img, ok := s.subPics[key]
	if !ok || img == nil {
		return nil, false
	}
	return encodeImage(img), true
}
