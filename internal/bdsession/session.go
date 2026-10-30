// Package bdsession turns WittyBluray navigator steps into what the disc
// player shows: HLS streams of the playlists, PNG overlays for menu
// buttons and their highlights, button geometry for the remote, and
// subtitle pictures. It produces the same presentation as dvdsession, so
// one player plays both kinds of disc.
package bdsession

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
)

// The presentation types are dvdsession's: the client is the same.
type (
	Presentation = dvdsession.Presentation
	Media        = dvdsession.Media
	Menu         = dvdsession.Menu
	ButtonView   = dvdsession.ButtonView
	TitleView    = dvdsession.TitleView
	Track        = dvdsession.Track
	Prefs        = dvdsession.Prefs
	Action       = dvdsession.Action
	SubEvent     = dvdsession.SubEvent
)

// Manager owns Blu-ray sessions.
type Manager struct {
	HLS      *hls.Manager
	Tools    *ffmpeg.Tools
	CacheDir string
	BaseURL  string
	Log      *slog.Logger
	mu       sync.Mutex
	sessions map[string]*Session
}

// NewManager makes a manager; idle sessions are closed after 30 minutes.
func NewManager(h *hls.Manager, tools *ffmpeg.Tools, cacheDir, baseURL string, log *slog.Logger) *Manager {
	m := &Manager{HLS: h, Tools: tools, CacheDir: cacheDir, BaseURL: baseURL, Log: log, sessions: map[string]*Session{}}
	go m.sweep()
	return m
}

func (m *Manager) sweep() {
	for range time.Tick(time.Minute) {
		m.mu.Lock()
		for id, s := range m.sessions {
			if time.Since(s.lastUsed()) > 30*time.Minute {
				s.Close()
				delete(m.sessions, id)
			}
		}
		m.mu.Unlock()
	}
}

// Session is one viewer's disc.
type Session struct {
	ID     string
	ItemID int64
	m      *Manager
	key    string
	prefs  Prefs
	disc   *wittybluray.Disc
	nav    *wittybluray.Nav

	mu        sync.Mutex
	last      time.Time
	current   *Presentation
	serial    int
	step      *wittybluray.Step
	playlist  *playing // what a title presentation streams
	menu      *menuState
	menus     map[int]*menuArt
	artSeq    int
	menuCache map[string]*wittybluray.Menu
	subPics   map[string]*subPic
	seekTo    float64 // seconds into the playlist: continue here after a stream change
	timer     *wittybluray.NavTimer
	timerAt   time.Time
}

// playing is a playlist laid out on one timeline.
type playing struct {
	pl       *wittybluray.Playlist
	items    []playedItem
	duration float64
	chapters []float64
}

// playedItem is one play item's place on the timeline and in its clip.
type playedItem struct {
	clip     string
	start    float64 // seconds into the playlist
	duration float64
	startPTS uint32 // the entry point the stream begins at (45 kHz)
	spn      [2]uint32
}

type menuState struct {
	menu              *wittybluray.Menu
	page              *wittybluray.Page
	selected          int
	enabled, disabled map[int]bool
}

type menuArt struct {
	menu       *wittybluray.Menu
	page       *wittybluray.Page
	buttons    []*wittybluray.Button
	overlay    []byte
	highlights map[int][]byte
}

// Count is the number of open sessions.
func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

func randomID() string {
	b := make([]byte, 9)
	_, _ = rand.Read(b)
	return "bd" + hex.EncodeToString(b)
}

// Open opens a disc for a viewer.
func (m *Manager) Open(itemID int64, path string, prefs Prefs) (*Session, error) {
	d, err := wittybluray.Open(path)
	if err != nil {
		return nil, err
	}
	s := &Session{ID: randomID(), ItemID: itemID, m: m, key: hls.Key(path), prefs: prefs, disc: d, last: time.Now(),
		menus: map[int]*menuArt{}, menuCache: map[string]*wittybluray.Menu{}, subPics: map[string]*subPic{}}
	s.nav = wittybluray.NewNav(d, wittybluray.NavOptions{Registers: wittybluray.RegisterOptions{
		AudioLang: lang3(prefs.AudioLanguage), SubtitleLang: lang3(prefs.SubtitleLanguage), MenuLang: lang3(prefs.MenuLanguage),
	}})
	if prefs.Angle > 0 {
		s.nav.SetAngle(prefs.Angle)
	}
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	return s, nil
}

// Get finds a session.
func (m *Manager) Get(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s != nil {
		s.touch()
	}
	return s
}

// CloseSession ends a session.
func (m *Manager) CloseSession(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sessions[id]; s != nil {
		s.Close()
		delete(m.sessions, id)
	}
}

func (s *Session) touch() {
	s.mu.Lock()
	s.last = time.Now()
	s.mu.Unlock()
}

func (s *Session) lastUsed() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

// Close releases the disc.
func (s *Session) Close() { _ = s.disc.Close() }

// Current is the presentation now showing.
func (s *Session) Current() *Presentation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// Start plays the disc from insertion: its first-play program.
func (s *Session) Start(ctx context.Context) *Presentation {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disc.Encrypted {
		return s.stop("this disc is encrypted (AACS); make a decrypted copy of a disc you own with a tool that is legal where you live", true)
	}
	return s.show(ctx, s.nav.Start())
}

// PlayTitle plays a playlist (ttn is its number; 0 = the main feature) from
// a chapter (1-based; 0 = the start).
func (s *Session) PlayTitle(ctx context.Context, ttn, chapter int) *Presentation {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disc.Encrypted {
		return s.stop("this disc is encrypted (AACS)", true)
	}
	pl := s.disc.Playlist(ttn)
	if pl == nil {
		pl = s.disc.MainPlaylist()
	}
	if pl == nil {
		return s.stop("the disc has nothing to play", false)
	}
	item, t := 0, uint32(0)
	if marks := chapterMarks(pl); chapter >= 1 && chapter <= len(marks) {
		item, t = marks[chapter-1].Item, marks[chapter-1].Time
	}
	return s.show(ctx, s.nav.PlayPlaylist(pl, item, t))
}

// OpenMenu opens the disc's top menu (Blu-rays have one menu program).
func (s *Session) OpenMenu(ctx context.Context, kind string, vts int) *Presentation {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disc.Encrypted {
		return s.stop("this disc is encrypted (AACS)", true)
	}
	return s.show(ctx, s.nav.PlayTitle(0))
}

// Do applies a client action.
func (s *Session) Do(ctx context.Context, a Action) *Presentation {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.tickTimer(); st != nil {
		return s.show(ctx, st)
	}
	cur := s.current
	switch a.Type {
	case "tick":
		return cur
	case "press":
		s.syncPosition(a.PositionSec) // pop-up menus read the chapter and time registers
		return s.press(ctx, a.Button)
	case "select":
		return s.selectButton(a.Button)
	case "key":
		s.syncPosition(a.PositionSec)
		return s.keyPress(ctx, a.Key)
	case "ended":
		return s.show(ctx, s.nav.PlaylistEnded())
	case "title":
		s.mu.Unlock()
		p := s.PlayTitle(ctx, a.TTN, a.Chapter)
		s.mu.Lock()
		return p
	case "menu":
		s.syncPosition(a.PositionSec)
		return s.show(ctx, s.nav.MenuCall())
	case "resume":
		return s.show(ctx, s.nav.Resume())
	case "audio":
		s.nav.SetAudio(a.Stream + 1)
		if cur != nil && cur.Kind == "title" && s.step != nil {
			s.seekTo = a.PositionSec
			return s.present(ctx, s.step, true)
		}
		return cur
	case "angle":
		s.nav.SetAngle(a.Stream)
		if cur != nil && cur.Kind == "title" && s.step != nil {
			s.seekTo = a.PositionSec
			return s.present(ctx, s.step, true)
		}
		return cur
	case "subtitle":
		s.nav.SetSubtitle(a.Stream+1, a.On)
		if cur != nil && cur.Kind == "title" && cur.Title != nil {
			p, tv := *cur, *cur.Title
			tv.SubStream = -1
			if a.On {
				tv.SubStream = a.Stream
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
	if v, ok := m[code]; ok {
		return v
	}
	return ""
}

// ---------------------------------------------------------------- timer

func (s *Session) tickTimer() *wittybluray.Step {
	if s.timer == nil || time.Now().Before(s.timerAt) {
		return nil
	}
	s.timer = nil
	return s.nav.NavTimerFired()
}

// ---------------------------------------------------------------- menus

// press runs a menu button's commands.
func (s *Session) press(ctx context.Context, n int) *Presentation {
	ms, cur := s.menu, s.current
	if ms == nil || cur == nil || cur.Menu == nil {
		return cur
	}
	b := ms.page.Button(n - 1)
	if b == nil {
		return cur
	}
	ms.selected = b.ID
	st := s.nav.PressButton(b.Commands)
	if st.Kind != wittybluray.StepStay {
		return s.show(ctx, st)
	}
	fx := st.Effects
	if s.applyEffects(fx) {
		if cur.Kind == "title" && cur.Menu.Popup {
			// The pop-up menu changed page or buttons: same film, new layer.
			p := *cur
			if err := s.menuView(&p, ms.menu, true, s.videoRate(s.step)); err != nil {
				return cur
			}
			s.current = &p
			return &p
		}
		return s.present(ctx, s.step, true) // the page or its buttons changed
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

// applyEffects applies a button's SET_SYSTEM effects to the menu state and
// says whether the screen must be rebuilt.
func (s *Session) applyEffects(fx *wittybluray.Effects) bool {
	ms := s.menu
	if fx == nil || ms == nil {
		return false
	}
	rebuild := false
	if fx.NavTimer != nil {
		s.timer, s.timerAt = fx.NavTimer, time.Now().Add(time.Duration(fx.NavTimer.Seconds)*time.Second)
	}
	if bp := fx.ButtonPage; bp != nil {
		if bp.SetPage {
			if p := ms.menu.Page(int(bp.Page)); p != nil && p != ms.page {
				ms.page = p
				ms.enabled, ms.disabled = map[int]bool{}, map[int]bool{}
				ms.selected = p.DefaultSelected
				rebuild = true
			}
		}
		if bp.SetButton {
			ms.selected = int(bp.Button)
		}
	}
	for _, id := range fx.EnableButton {
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

func (s *Session) selectButton(n int) *Presentation {
	if s.menu == nil {
		return s.current
	}
	return s.moveSelection(n - 1)
}

// moveSelection updates the selected button without changing the screen.
func (s *Session) moveSelection(id int) *Presentation {
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

func (s *Session) keyPress(ctx context.Context, key string) *Presentation {
	cur := s.current
	switch key {
	case "menu", "root", "title":
		return s.show(ctx, s.nav.MenuCall())
	case "resume":
		return s.show(ctx, s.nav.Resume())
	case "return", "next", "prev":
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
		return s.press(ctx, sel.ID+1)
	default:
		return cur
	}
	if next == wittybluray.NoButton || ms.page.Button(next) == nil {
		return cur
	}
	if b := ms.page.Button(next); b != nil && b.AutoAction {
		return s.press(ctx, next+1)
	}
	return s.moveSelection(next)
}

// ---------------------------------------------------------------- presenting

func (s *Session) url(parts ...string) string {
	return s.m.BaseURL + "/dvd/" + s.ID + "/" + strings.Join(parts, "/")
}

func (s *Session) stop(reason string, scrambled bool) *Presentation {
	s.serial++
	p := &Presentation{Kind: "stop", Serial: s.serial, Reason: reason, Media: &Media{Type: "none"}, Scrambled: scrambled}
	s.current, s.menu, s.playlist = p, nil, nil
	return p
}

func (s *Session) show(ctx context.Context, st *wittybluray.Step) *Presentation {
	return s.present(ctx, st, false)
}

// present turns a step into a presentation; rebuild makes a new screen even
// for the same step (stream change, menu page change).
func (s *Session) present(ctx context.Context, st *wittybluray.Step, rebuild bool) *Presentation {
	if st == nil {
		return s.current
	}
	if s.m.Log != nil {
		s.m.Log.Info("bluray step", "cat", "dvd", "session", s.ID, "kind", st.Kind, "reason", st.Reason, "menu", st.Menu, "title", st.Title)
	}
	switch st.Kind {
	case wittybluray.StepStay:
		return s.current
	case wittybluray.StepStop:
		return s.stop(st.Reason, false)
	case wittybluray.StepUnsupported:
		// A BD-J program: play the feature instead, and say so.
		pl := s.disc.MainPlaylist()
		if pl == nil {
			return s.stop(st.Reason, false)
		}
		st2 := s.nav.PlayPlaylist(pl, 0, 0)
		st2.Reason = "This disc's menu needs BD-J (Java), which is not supported; playing the main feature."
		return s.present(ctx, st2, rebuild)
	}
	if st.Playlist == nil {
		return s.stop("nothing to play", false)
	}
	if !rebuild {
		s.seekTo = 0
	}
	// A playlist whose menu is always on is a menu. A playlist with a pop-up
	// menu (summoned during the film) is a title that carries the menu for
	// the player's Menu key. Playlists that are not a numbered title
	// (first-play intros, menu transitions) present like menus without
	// buttons: they play through, and no viewing progress is recorded.
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
		err = s.buildMenu(ctx, st, p)
	} else {
		if !rebuild || popup == nil {
			s.menu = nil
		}
		if err = s.buildTitle(ctx, st, p); err == nil && popup != nil {
			err = s.menuView(p, popup, true, s.videoRate(st))
		}
	}
	if err != nil {
		s.current = &Presentation{Kind: "stop", Serial: s.serial, Reason: err.Error(), Media: &Media{Type: "none"}}
		return s.current
	}
	if s.timer != nil {
		if left := time.Until(s.timerAt).Seconds(); left > 0 {
			p.TimerSec = left
		}
	}
	s.current = p
	return p
}

// layout puts a playlist's items on one timeline, each starting at the
// entry point at or before its IN time (the stream begins there).
func (s *Session) layout(pl *wittybluray.Playlist) *playing {
	out := &playing{pl: pl}
	angle := int(s.nav.Registers().PSR[wittybluray.PSRAngle])
	var t float64
	for _, it := range pl.Items {
		clipName := it.Clip
		if angle >= 2 && angle <= len(it.Angles) {
			clipName = it.Angles[angle-1].Clip
		}
		c := s.disc.Clip(clipName)
		if c == nil {
			continue
		}
		pi := playedItem{clip: clipName, start: t, startPTS: it.In, spn: [2]uint32{0, c.Packets}}
		if e, ok := c.SPNAt(c.VideoPID(), it.In); ok {
			pi.startPTS, pi.spn[0] = e.PTS, e.SPN
		}
		if e, ok := c.SPNAt(c.VideoPID(), it.Out); ok && e.SPN > pi.spn[0] && it.Out < c.STC[len(c.STC)-1].End {
			pi.spn[1] = e.SPN
		}
		if it.Out > pi.startPTS {
			pi.duration = float64(it.Out-pi.startPTS) / 45000
		}
		out.items = append(out.items, pi)
		t += pi.duration
	}
	out.duration = t
	for _, m := range chapterMarks(pl) {
		if m.Item < len(out.items) {
			pi := out.items[m.Item]
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

// segments are the FFmpeg inputs for the laid-out playlist.
func (s *Session) segments(pl *playing) ([]ffmpeg.Segment, error) {
	var segs []ffmpeg.Segment
	for _, pi := range pl.items {
		ext, err := s.disc.ClipExtents(pi.clip)
		if err != nil {
			return nil, err
		}
		from, to := int64(pi.spn[0])*192, int64(pi.spn[1])*192
		var cut []wittydvd.FileExtent
		var off int64
		for _, e := range ext {
			a, b := off, off+e.Length
			off = b
			lo, hi := max64(a, from), min64(b, to)
			if hi <= lo {
				continue
			}
			cut = append(cut, wittydvd.FileExtent{File: e.Path, Offset: e.Offset + (lo - a), Length: hi - lo})
		}
		if len(cut) == 0 {
			return nil, fmt.Errorf("clip %s is missing from the disc", pi.clip)
		}
		segs = append(segs, ffmpeg.Segment{Extents: cut, Duration: pi.duration})
	}
	if len(segs) == 0 {
		return nil, errors.New("the playlist has no clips")
	}
	return segs, nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func (s *Session) listDir() string { return filepath.Join(s.m.CacheDir, "lists", s.key) }

// videoFilter deinterlaces interlaced discs (480i, 576i, 1080i) and
// tone-maps HDR (UHD discs: HDR10, HDR10+, Dolby Vision's HDR10 base layer)
// to SDR, since browsers and the H.264 streams they play are SDR.
func (s *Session) videoFilter(it *wittybluray.PlayItem) string {
	var chain []string
	if len(it.Video) > 0 {
		v := it.Video[0]
		switch v.Format {
		case 1, 2, 4:
			if s.prefs.LowPower {
				chain = append(chain, "yadif=mode=send_frame:parity=auto:deint=interlaced")
			} else {
				chain = append(chain, "bwdif=mode=send_frame:parity=auto:deint=interlaced")
			}
		}
		if isHDR(v) {
			if tm := s.m.Tools.ToneMapFilter(); tm != "" {
				chain = append(chain, tm)
			}
		}
	}
	return strings.Join(chain, ",")
}

// isHDR says whether a video stream is high dynamic range: UHD discs flag
// HDR10 and Dolby Vision, and BT.2020 colour means HDR in practice.
func isHDR(v wittybluray.Stream) bool {
	return v.Coding == 0x24 && (v.DynamicRange >= 1 || v.ColorSpace == 2)
}

// maxHeight caps the stream: 2160p sources are sent as 1080p unless the
// settings ask for a height of 2160 or more, since encoding 4K in software
// is beyond most machines and most screens that play it are 1080p.
func (s *Session) maxHeight(it *wittybluray.PlayItem) int {
	if s.prefs.MaxHeight == 0 && len(it.Video) > 0 && it.Video[0].Format == 8 {
		return 1080
	}
	return s.prefs.MaxHeight
}

func (s *Session) preset(menu bool) string {
	switch {
	case s.prefs.LowPower:
		return "ultrafast"
	case menu:
		return "superfast"
	}
	return ""
}

func (s *Session) codecTag() string {
	if s.prefs.AudioCodec == "mp3" {
		return "-mp3"
	}
	return ""
}

// audioFor picks the audio stream for the first item: the register's
// choice when it is in the clip, else the preferred language, else the
// first. Returns the index (0-based) and the stream (nil = no audio).
func (s *Session) audioFor(it *wittybluray.PlayItem) (int, *wittybluray.Stream) {
	inMux := func(st wittybluray.Stream) bool { return st.EntryType == 1 }
	n := int(s.nav.Registers().PSR[wittybluray.PSRAudio])
	if n >= 1 && n <= len(it.Audio) && inMux(it.Audio[n-1]) {
		return n - 1, &it.Audio[n-1]
	}
	if want := lang3(s.prefs.AudioLanguage); want != "" {
		for i := range it.Audio {
			if inMux(it.Audio[i]) && it.Audio[i].Lang == want {
				s.nav.SetAudio(i + 1)
				return i, &it.Audio[i]
			}
		}
	}
	for i := range it.Audio {
		if inMux(it.Audio[i]) {
			s.nav.SetAudio(i + 1)
			return i, &it.Audio[i]
		}
	}
	return -1, nil
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

func (s *Session) hlsID(name string) string { return fmt.Sprintf("bd-%s-%s", s.key, name) }

// ensureHLS starts (or finds) the stream of a laid-out playlist.
func (s *Session) ensureHLS(pl *playing, it *wittybluray.PlayItem, audio *wittybluray.Stream, menu bool) (string, int, error) {
	audioIdx := -1
	ids := []int{}
	if len(it.Video) > 0 {
		ids = append(ids, int(it.Video[0].PID))
	}
	channels := 2
	if audio != nil {
		ids = append(ids, int(audio.PID))
		audioIdx = 0
		if audio.Format >= 6 {
			channels = 6
		}
	}
	angle := s.nav.Registers().PSR[wittybluray.PSRAngle]
	name := fmt.Sprintf("p%05d-a%x-g%d%s", pl.pl.Number, idsKey(ids), angle, s.codecTag())
	id := s.hlsID(name)
	if s.m.HLS.Get(id) != nil {
		return id, audioIdx, nil
	}
	segs, err := s.segments(pl)
	if err != nil {
		return "", -1, err
	}
	list, err := ffmpeg.WriteConcat(s.listDir(), name, segs, ids...)
	if err != nil {
		return "", -1, err
	}
	s.m.HLS.Ensure(hls.Spec{
		ID: id, InputArgs: ffmpeg.ConcatInputArgs(list), Duration: pl.duration, VideoFilter: s.videoFilter(it), Preset: s.preset(menu),
		AudioIndex: audioIdx, AudioChannels: channels, SubtitleIndex: -1, LowLatency: menu, HoldLastFrame: menu,
		Quality: s.prefs.Quality, MaxHeight: s.maxHeight(it), AudioCodec: s.prefs.AudioCodec,
	})
	return id, audioIdx, nil
}

func idsKey(ids []int) int {
	k := 0
	for _, id := range ids {
		k = k*0x10000 + id
	}
	return k
}

// loadMenu reads (and caches) the step's menu graphics; nil when there are
// none or they cannot be read.
func (s *Session) loadMenu(st *wittybluray.Step) *wittybluray.Menu {
	if !st.Menu || st.Item >= len(st.Playlist.Items) {
		return nil
	}
	clip, limit := st.IGClip, uint32(0)
	if clip == "" {
		clip, limit = st.Playlist.Items[st.Item].Clip, 200000
	}
	key := fmt.Sprintf("%s-%x", clip, st.IGPID)
	if m, ok := s.menuCache[key]; ok {
		return m
	}
	m, err := s.disc.LoadMenu(clip, st.IGPID, limit)
	if err != nil || len(m.Pages) == 0 {
		if s.m.Log != nil {
			s.m.Log.Warn("bluray menu", "cat", "dvd", "clip", clip, "err", err)
		}
		m = nil
	}
	s.menuCache[key] = m
	return m
}

func (s *Session) buildMenu(ctx context.Context, st *wittybluray.Step, p *Presentation) error {
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
	laid := s.layout(pl)
	_, audio := s.audioFor(it)
	id, audioIdx, err := s.ensureHLS(laid, it, audio, true)
	if err != nil {
		return err
	}
	p.Kind = "menu"
	p.Media = &Media{Type: "hls", URL: s.m.BaseURL + "/hls/" + id + "/playlist.m3u8", Loop: st.Menu, DurationSec: laid.duration, Audio: audioIdx >= 0}
	if menu == nil {
		p.Menu = &Menu{Width: 1920, Height: 1080, Aspect: "16:9", Buttons: []ButtonView{}}
		return nil
	}
	return s.menuView(p, menu, false, s.videoRate(st))
}

// menuView fills p.Menu with the current page of a menu: its buttons, the
// overlay of their normal pictures and a highlight per button. popup marks
// a menu the player shows only on its Menu key. Each view gets its own
// artwork key, so a page change under the same serial still refreshes the
// pictures.
func (s *Session) menuView(p *Presentation, menu *wittybluray.Menu, popup bool, rate int) error {
	if s.menu == nil || s.menu.menu != menu {
		s.menu = &menuState{menu: menu, page: &menu.Pages[0], selected: menu.Pages[0].DefaultSelected, enabled: map[int]bool{}, disabled: map[int]bool{}}
	}
	ms := s.menu
	buttons := ms.page.Visible(ms.enabled, ms.disabled)
	if ms.page.Button(ms.selected) == nil && len(buttons) > 0 {
		ms.selected = buttons[0].ID
	}
	s.artSeq = s.artSeq%999 + 1
	key := s.serial*1000 + s.artSeq
	s.menus[key] = &menuArt{menu: menu, page: ms.page, buttons: buttons, highlights: map[int][]byte{}}
	for k := range s.menus {
		if k < (s.serial-4)*1000 {
			delete(s.menus, k)
		}
	}
	m := &Menu{Width: menu.Width, Height: menu.Height, Aspect: "16:9", Selected: ms.selected + 1, HasHighlights: true, Buttons: []ButtonView{}, Popup: popup, InTitle: popup}
	if menu.Width > 0 && menu.Height > 0 && menu.Width*3 == menu.Height*4 {
		m.Aspect = "4:3"
	}
	fps := animationFPS(ms.page, rate, s.disc.Index.FrameRate)
	for _, b := range buttons {
		r := menu.Rect(b)
		cmd := ""
		if len(b.Commands) > 0 {
			cmd = wittybluray.Decode(b.Commands[0]).String()
		}
		bv := ButtonView{N: b.ID + 1, X1: r.Min.X, Y1: r.Min.Y, X2: r.Max.X, Y2: r.Max.Y,
			Up: ref(b.Up), Down: ref(b.Down), Left: ref(b.Left), Right: ref(b.Right), Auto: b.AutoAction, Lit: b.Selected.Start != wittybluray.NoButton, Cmd: cmd}
		if n := b.Selected.Frames(); n > 1 && fps > 0 {
			bv.Frames, bv.FPS = n, fps
		}
		m.Buttons = append(m.Buttons, bv)
	}
	m.OverlayURL = s.url("menu", fmt.Sprint(key), "overlay.png")
	m.HighlightURL = s.url("menu", fmt.Sprint(key), "highlight", "{n}.png")
	p.Menu = m
	return nil
}

// animationFPS is how fast a page's button animations run: the video frame
// rate divided by the page's animation rate code (0 = no animation).
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

// videoRate is the frame rate code of the step's video stream.
func (s *Session) videoRate(st *wittybluray.Step) int {
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

func (s *Session) buildTitle(ctx context.Context, st *wittybluray.Step, p *Presentation) error {
	pl := st.Playlist
	if st.Item >= len(pl.Items) {
		return errors.New("the playlist is empty")
	}
	laid := s.layout(pl)
	s.playlist = laid
	it := &pl.Items[st.Item]
	audioN, audio := s.audioFor(it)
	id, audioIdx, err := s.ensureHLS(laid, it, audio, false)
	if err != nil {
		return err
	}
	start := 0.0
	if st.Item < len(laid.items) {
		start = laid.items[st.Item].start
		if st.Time > laid.items[st.Item].startPTS {
			start += float64(st.Time-laid.items[st.Item].startPTS) / 45000
		}
	}
	if s.seekTo > 0 {
		start = s.seekTo
	}
	s.seekTo = 0
	regs := s.nav.Registers()
	subN := -1
	if pg := regs.PSR[wittybluray.PSRPG]; pg>>31 == 1 && int(pg&0xfff) >= 1 && int(pg&0xfff) <= len(it.PG) {
		subN = int(pg&0xfff) - 1
	} else if want := lang3(s.prefs.SubtitleLanguage); want != "" && pg>>31 == 1 {
		for i := range it.PG {
			if it.PG[i].Lang == want {
				subN = i
				s.nav.SetSubtitle(i+1, true)
				break
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
	p.Media = &Media{Type: "hls", URL: s.m.BaseURL + "/hls/" + id + "/playlist.m3u8", DurationSec: laid.duration, Audio: audioIdx >= 0}
	tv := &TitleView{TTN: pl.Number, Chapter: chapter, DurationSec: laid.duration, StartSec: start, Chapters: laid.chapters, AudioStream: audioN, SubStream: subN,
		Wide: true, Audio: []Track{}, Subtitles: []Track{}, FrameW: fw, FrameH: fh, Aspect: "16:9",
		SubtitlesURL: s.url("subs") + fmt.Sprintf("?ttn=%d", pl.Number), ForcedStream: subN, Angle: int(regs.PSR[wittybluray.PSRAngle]), Angles: len(it.Angles)}
	if tv.Chapters == nil {
		tv.Chapters = []float64{}
	}
	for i, a := range it.Audio {
		if a.EntryType != 1 {
			continue
		}
		ch := 2
		switch a.Format {
		case 1:
			ch = 1
		case 6, 12:
			ch = 6
		}
		tv.Audio = append(tv.Audio, Track{N: i, Language: a.Lang, Codec: a.CodingName(), Channels: ch})
	}
	for i, sub := range it.PG {
		tv.Subtitles = append(tv.Subtitles, Track{N: i, Language: sub.Lang})
	}
	p.Title = tv
	return nil
}

// syncPosition records where the viewer is in the title, for Resume.
func (s *Session) syncPosition(pos float64) {
	pl := s.playlist
	if pl == nil || s.current == nil || s.current.Kind != "title" || pos <= 0 {
		return
	}
	for i := len(pl.items) - 1; i >= 0; i-- {
		pi := pl.items[i]
		if pi.start <= pos+0.01 {
			s.nav.SetTime(i, pi.startPTS+uint32((pos-pi.start)*45000))
			break
		}
	}
	for i, c := range pl.chapters {
		if c <= pos+0.5 {
			s.nav.SetChapter(i + 1)
		}
	}
}

// ---------------------------------------------------------------- artwork

var pngEnc = png.Encoder{CompressionLevel: png.BestSpeed}

func encode(img image.Image) []byte {
	var buf bytes.Buffer
	_ = pngEnc.Encode(&buf, img)
	return buf.Bytes()
}

// OverlayPNG is the menu page with every button in its normal state.
func (s *Session) OverlayPNG(serial int) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	art := s.menus[serial]
	if art == nil {
		return nil, false
	}
	if art.overlay == nil {
		art.overlay = encode(art.menu.Render(art.page, art.buttons, wittybluray.NoButton))
	}
	return art.overlay, true
}

// HighlightPNG is one button in its selected state, on a full frame.
func (s *Session) HighlightPNG(serial, n int) ([]byte, bool) {
	return s.HighlightFramePNG(serial, n, 0)
}

// HighlightFramePNG is one picture of a button's selected-state animation.
func (s *Session) HighlightFramePNG(serial, n, frame int) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	art := s.menus[serial]
	if art == nil || frame < 0 || frame > 256 {
		return nil, false
	}
	k := n*1000 + frame
	if b, ok := art.highlights[k]; ok {
		return b, true
	}
	for _, b := range art.buttons {
		if b.ID == n-1 {
			img := image.NewNRGBA(image.Rect(0, 0, art.menu.Width, art.menu.Height))
			art.menu.DrawFrame(img, art.page, b, wittybluray.ButtonSelected, frame)
			out := encode(img)
			art.highlights[k] = out
			return out, true
		}
	}
	return nil, false
}

// StillPath: Blu-ray presentations are always streams.
func (s *Session) StillPath(name string) (string, bool) { return "", false }
