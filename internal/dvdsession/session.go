// Package dvdsession turns WittyDVD navigator steps into things a client can
// show: HLS streams or still frames for the video, PNG overlays for
// sub-pictures and button highlights, and button geometry for input handling.
package dvdsession

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
	"maps"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
)

// Presentation is what the client should show right now.
type Presentation struct {
	Kind      string     `json:"kind"` // menu | title | stop
	Serial    int        `json:"serial"`
	Reason    string     `json:"reason,omitempty"`
	Media     *Media     `json:"media,omitempty"`
	Menu      *Menu      `json:"menu,omitempty"`
	Title     *TitleView `json:"title,omitempty"`
	Scrambled bool       `json:"scrambled,omitempty"`
	// TimerSec is set while the disc's navigation timer runs; the client
	// sends a "tick" action when it expires.
	TimerSec float64 `json:"timerSec,omitempty"`
	// Pause is set when a disc's button froze or released the picture
	// (Blu-ray STILL_ON and STILL_OFF): true = hold, false = go on.
	Pause *bool `json:"pause,omitempty"`
	// BDJ is set when a disc's BD-J applications are on screen: their graphics
	// lie over the video and the viewer's keys go to them.
	BDJ *BDJView `json:"bdj,omitempty"`
}

// BDJView tells the player where to follow a disc's BD-J applications.
type BDJView struct {
	PollURL string `json:"pollUrl"`
	// Video is where the applications have placed the video, on a 1920x1080
	// frame (x, y, w, h); empty = full screen.
	Video []int `json:"video,omitempty"`
}

// Media describes the picture/sound for the current step.
type Media struct {
	Type        string  `json:"type"` // hls | still | none
	URL         string  `json:"url,omitempty"`
	Loop        bool    `json:"loop"`
	DurationSec float64 `json:"durationSec"`
	StillSec    float64 `json:"stillSec"` // seconds to hold after the clip; -1 = until the viewer acts
	Audio       bool    `json:"audio"`
}

// Menu carries the interactive layer.
type Menu struct {
	Width          int          `json:"width"`
	Height         int          `json:"height"`
	Aspect         string       `json:"aspect"` // 4:3 | 16:9
	Buttons        []ButtonView `json:"buttons"`
	Selected       int          `json:"selected"`
	OverlayURL     string       `json:"overlayUrl,omitempty"`
	HighlightURL   string       `json:"highlightUrl,omitempty"` // template with {n}
	ButtonsFromSec float64      `json:"buttonsFromSec,omitempty"`
	// ButtonsUntilSec is when the disc's buttons stop working (highlight or
	// button selection ends) before the clip does; 0 = while the clip shows.
	ButtonsUntilSec float64 `json:"buttonsUntilSec,omitempty"`
	HasHighlights   bool    `json:"hasHighlights"`
	// ForcedAction is a button the disc activates by itself ActionAtSec into
	// the clip (when button selection ends). 0 = none.
	ForcedAction int     `json:"forcedAction,omitempty"`
	ActionAtSec  float64 `json:"actionAtSec,omitempty"`
	// InTitle is set for interactive cells inside a title (games, quizzes,
	// branching features) rather than a menu domain.
	InTitle bool `json:"inTitle,omitempty"`
	// Popup marks a menu summoned during a title (Blu-ray pop-up menus): the
	// player shows it on the Menu key. Closed tells the player to hide it (a
	// button ran POPUP_OFF).
	Popup  bool `json:"popup,omitempty"`
	Closed bool `json:"closed,omitempty"`
	// UserTimeoutSec closes a pop-up when no key has been pressed for so long.
	UserTimeoutSec float64 `json:"userTimeoutSec,omitempty"`
	// SoundURL is a template with {id} for the buttons' sounds.
	SoundURL string `json:"soundUrl,omitempty"`
	// InEffect is a page's opening transition, frame by frame.
	InEffect []EffectFrame `json:"inEffect,omitempty"`
}

// EffectFrame is one picture of a page transition.
type EffectFrame struct {
	URL string  `json:"url"`
	Sec float64 `json:"sec"`
}

type ButtonView struct {
	N     int    `json:"n"`
	X1    int    `json:"x1"`
	Y1    int    `json:"y1"`
	X2    int    `json:"x2"`
	Y2    int    `json:"y2"`
	Up    int    `json:"up"`
	Down  int    `json:"down"`
	Left  int    `json:"left"`
	Right int    `json:"right"`
	Auto  bool   `json:"auto"`
	Lit   bool   `json:"lit"`
	Cmd   string `json:"cmd"`
	// Frames and FPS describe an animated selected state (Blu-ray menus):
	// the highlight URL takes ?f=N for frame N.
	Frames int     `json:"frames,omitempty"`
	FPS    float64 `json:"fps,omitempty"`
	// Num is the number key that presses it, when that is not N (Blu-ray
	// buttons have their own numbers).
	Num int `json:"num,omitempty"`
	// SoundSelect and SoundPress are sound ids + 1 (0 = silent).
	SoundSelect int `json:"soundSelect,omitempty"`
	SoundPress  int `json:"soundPress,omitempty"`
}

// TitleView describes feature playback.
type TitleView struct {
	TTN         int       `json:"ttn"`
	Chapter     int       `json:"chapter"`
	DurationSec float64   `json:"durationSec"`
	StartSec    float64   `json:"startSec"`
	Chapters    []float64 `json:"chapters"`
	Audio       []Track   `json:"audio"`
	Subtitles   []Track   `json:"subtitles"`
	AudioStream int       `json:"audioStream"`    // SPRM1
	SubStream   int       `json:"subtitleStream"` // -1 off
	Wide        bool      `json:"wide"`
	// Picture geometry for the client-side subtitle layer.
	FrameW int    `json:"frameW"`
	FrameH int    `json:"frameH"`
	Aspect string `json:"aspect"` // 4:3 | 16:9
	// SubtitlesURL lists sub-picture events: add &stream=N&from=&to=.
	SubtitlesURL string `json:"subtitlesUrl"`
	// ForcedStream is the stream whose forced subtitles show even when
	// subtitles are off.
	ForcedStream int `json:"forcedStream"`
	// Angle is the camera angle playing (1-based) of Angles; angles are
	// alternate versions of the picture (e.g. English or Dutch on-screen text).
	Angle  int `json:"angle"`
	Angles int `json:"angles"`
	// Locks names what the disc forbids here: "seek", "chapter", "pause",
	// "audio", "subtitle", "angle". The player honours them unless the viewer
	// turned that off.
	Locks []string `json:"locks,omitempty"`
}

type Track struct {
	N        int    `json:"n"`
	Language string `json:"language"`
	Codec    string `json:"codec,omitempty"`
	Channels int    `json:"channels,omitempty"`
	Purpose  string `json:"purpose,omitempty"` // Commentary, Captions, Forced, ...
}

// Prefs are viewer preferences for a session.
type Prefs struct {
	MenuLanguage     string
	AudioLanguage    string
	SubtitleLanguage string
	Wide             bool           // viewer has a 16:9 display
	Quality          ffmpeg.Quality // encoder + quality profile from Settings
	MaxHeight        int
	// Tag keeps streams apart in their IDs: a viewer with limits can't be
	// handed the unlimited stream of the same disc.
	Tag        string
	AudioCodec string // aac | mp3
	LowPower   bool   // cheaper deinterlacing and encoder presets
	Angle      int    // remembered camera angle for this disc (0 = disc default)
	// The player's identity, from Settings. 0 and "" leave the defaults: a DVD
	// plays as its own region, a Blu-ray as region A; no parental limit.
	DVDRegion   int
	BDRegion    int    // 1 = A, 2 = B, 4 = C
	DVDParental int    // 1-8
	BDAge       int    // the viewer's age, as Blu-ray parental checks ask it
	Country     string // two letters
}

// Manager owns sessions.
type Manager struct {
	HLS      *hls.Manager
	Tools    *ffmpeg.Tools
	CacheDir string
	BaseURL  string // prefix for generated URLs, e.g. "/api"
	Log      *slog.Logger

	mu       sync.Mutex
	sessions map[string]*Session
}

func NewManager(h *hls.Manager, tools *ffmpeg.Tools, cacheDir, baseURL string, log *slog.Logger) *Manager {
	m := &Manager{HLS: h, Tools: tools, CacheDir: cacheDir, BaseURL: baseURL, Log: log, sessions: map[string]*Session{}}
	go m.sweep()
	return m
}

func (m *Manager) sweep() {
	for range time.Tick(time.Minute) {
		for id, s := range m.list() { // a session busy with a slow step holds its own lock, never the manager's
			if time.Since(s.lastUsed()) > 30*time.Minute {
				m.CloseSession(id)
			}
		}
	}
}

func (m *Manager) list() map[string]*Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return maps.Clone(m.sessions)
}

// maxSessions bounds the open discs (each keeps a file and its parsed
// structure); the one left alone longest makes way.
const maxSessions = 16

func (m *Manager) makeRoom() {
	var oldest *Session
	if all := m.list(); len(all) >= maxSessions {
		for _, o := range all {
			if oldest == nil || o.lastUsed().Before(oldest.lastUsed()) {
				oldest = o
			}
		}
		m.CloseSession(oldest.ID)
	}
}

// Session is one viewer's DVD player.
type Session struct {
	noVocals bool // the karaoke stream plays without its vocals
	ID       string
	ItemID   int64
	Path     string
	m        *Manager
	opened   *wittydvd.Opened
	nav      *wittydvd.Nav
	prefs    Prefs
	key      string // stable per disc, for HLS/still cache ids

	mu       sync.Mutex
	used     time.Time
	lastTick time.Time
	current  *Presentation
	menus    map[int]*menuArt
	titles   map[int]*wittydvd.TitleSummary
	playing  *playingTitle // linear title the client is streaming
	// resumeAt is the offset into the resume cell where the viewer left a
	// title for a menu, so Resume continues from the same frame.
	resumeAt float64
	// seekTo is the exact title position to restart a rebuilt stream at
	// (after an audio change).
	seekTo  float64
	subPics map[string][]*subPic // per sub-picture unit, one per picture it shows
}

type playingTitle struct {
	ts  *wittydvd.TitleSummary
	idx int // index into ts.Cells of the cell playback started from
}

type menuArt struct {
	pic        *wittydvd.MenuPicture
	overlay    []byte
	highlights map[int][]byte
}

// randomID never starts with "bd": that prefix sends a request to the Blu-ray
// sessions, and one DVD in 256 would have opened as already expired.
func randomID() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return "d" + hex.EncodeToString(b)
}

// Open starts a session for a disc path.
func (m *Manager) Open(itemID int64, path string, prefs Prefs) (*Session, error) {
	m.makeRoom()
	o, err := wittydvd.Open(path)
	if err != nil {
		return nil, err
	}
	s := &Session{
		ID: randomID(), ItemID: itemID, Path: path, m: m, opened: o, prefs: prefs,
		key: hls.Key(path), used: time.Now(), lastTick: time.Now(), menus: map[int]*menuArt{}, titles: map[int]*wittydvd.TitleSummary{}, subPics: map[string][]*subPic{},
	}
	s.nav = wittydvd.NewNav(o.Disc, wittydvd.NavOptions{RegisterOptions: wittydvd.RegisterOptions{MenuLanguage: prefs.MenuLanguage, AudioLanguage: prefs.AudioLanguage, SubtitleLanguage: prefs.SubtitleLanguage, Angle: prefs.Angle, Region: prefs.DVDRegion, Parental: prefs.DVDParental, Country: prefs.Country}})
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	return s, nil
}

// Get fetches a session.
func (m *Manager) Get(id string) *Session {
	m.mu.Lock()
	s := m.sessions[id]
	m.mu.Unlock()
	if s != nil {
		s.touch()
	}
	return s
}

// CloseSession ends a session.
func (m *Manager) CloseSession(id string) {
	m.mu.Lock()
	s := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if s != nil {
		s.Close()
	}
}

// Count is the number of open sessions.
func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

func (s *Session) touch() {
	s.mu.Lock()
	s.used = time.Now()
	s.mu.Unlock()
}

func (s *Session) lastUsed() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.used
}

// Close waits for any step in progress: its reads must not hit a closed file.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.opened.Close()
}

// Current returns the last presentation.
func (s *Session) Current() *Presentation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// Start runs first-play.
func (s *Session) Start(ctx context.Context) *Presentation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.show(ctx, s.nav.Start())
}

// PlayTitle jumps to a title/chapter directly (the "Play" button in the UI).
func (s *Session) PlayTitle(ctx context.Context, ttn, chapter int) *Presentation {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.nav.PlayTitle(ttn, chapter)
	// Started directly (not through the disc's menus): the viewer's
	// remembered angle wins over the one the title's own commands picked.
	if a := s.prefs.Angle; a > 1 && st.Kind == wittydvd.StepTitle && st.Title != nil &&
		st.Title.TTN >= 1 && st.Title.TTN <= len(s.opened.Disc.VMG.Titles) && a <= s.opened.Disc.VMG.Titles[st.Title.TTN-1].Angles {
		s.nav.SetAngle(a)
	}
	return s.show(ctx, st)
}

// OpenMenu starts the disc directly in one of its menus.
func (s *Session) OpenMenu(ctx context.Context, kind string, vts int) *Presentation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.show(ctx, s.nav.OpenMenu(kind, vts))
}

// Action is a client input.
type Action struct {
	Type    string `json:"type"` // press | select | key | ended | title | menu | resume | chapter | audio | subtitle | tick
	Button  int    `json:"button"`
	Key     string `json:"key"` // up down left right enter menu title resume next prev
	TTN     int    `json:"ttn"`
	Chapter int    `json:"chapter"`
	Stream  int    `json:"stream"`
	On      bool   `json:"on"`
	Menu    string `json:"menu"` // root | title | audio | subpicture | angle | chapter
	// PositionSec is where the viewer is in the current title stream; sent
	// with menu calls so Resume can continue from the same point.
	PositionSec float64 `json:"positionSec"`
}

// Do applies an action and returns the new presentation.
func (s *Session) Do(ctx context.Context, a Action) *Presentation {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.tick(); st != nil {
		return s.show(ctx, st)
	}
	cur := s.current
	switch a.Type {
	case "tick":
		return cur
	case "press":
		return s.press(ctx, a.Button)
	case "select":
		return s.show(ctx, s.nav.SelectButton(a.Button))
	case "key":
		if a.Key == "menu" || a.Key == "root" || a.Key == "title" || a.Key == "return" {
			s.syncPosition(a.PositionSec)
		}
		return s.keyPress(ctx, a.Key)
	case "ended":
		return s.ended(ctx)
	case "title":
		return s.show(ctx, s.nav.PlayTitle(a.TTN, a.Chapter))
	case "menu":
		kind := a.Menu
		if kind == "" {
			kind = "root"
		}
		s.syncPosition(a.PositionSec)
		return s.show(ctx, s.nav.MenuKey(kind))
	case "resume":
		return s.show(ctx, s.nav.Resume())
	case "chapter":
		if a.Chapter > 0 {
			return s.show(ctx, s.nav.NextChapter())
		}
		return s.show(ctx, s.nav.PrevChapter())
	case "audio":
		// A stream number with 0x100 is a karaoke stream without its vocals.
		s.noVocals = a.Stream&karaokeOff != 0
		s.nav.SetAudio(a.Stream &^ karaokeOff)
		if cur != nil && cur.Kind == "title" {
			s.syncPosition(a.PositionSec)
			s.seekTo = a.PositionSec
			return s.present(ctx, s.nav.Current(), true)
		}
		return cur
	case "angle":
		return s.switchAngle(ctx, a.Stream, a.PositionSec)
	case "subtitle":
		// Subtitles are drawn by the client: no new stream, no restart.
		s.nav.SetSubpicture(a.Stream, a.On)
		if cur != nil && cur.Kind == "title" && cur.Title != nil {
			p, tv := *cur, *cur.Title
			tv.SubStream = -1
			if a.On {
				tv.SubStream = a.Stream
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

func (s *Session) press(ctx context.Context, n int) *Presentation {
	cur := s.current
	if cur == nil || cur.Menu == nil {
		return s.show(ctx, s.nav.PressButton(n, nil))
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
	return s.show(ctx, s.nav.PressButton(n, cmd))
}

func (s *Session) keyPress(ctx context.Context, key string) *Presentation {
	cur := s.current
	switch key {
	case "menu", "root":
		return s.show(ctx, s.nav.MenuKey("root"))
	case "title":
		return s.show(ctx, s.nav.MenuKey("title"))
	case "resume":
		return s.show(ctx, s.nav.Resume())
	case "return":
		return s.show(ctx, s.nav.GoUp())
	case "next":
		return s.show(ctx, s.nav.NextChapter())
	case "prev":
		return s.show(ctx, s.nav.PrevChapter())
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
		return s.press(ctx, sel)
	default:
		return cur
	}
	if next == 0 {
		next = sel
	}
	for _, b := range cur.Menu.Buttons {
		if b.N == next && b.Auto {
			return s.press(ctx, next)
		}
	}
	return s.show(ctx, s.nav.SelectButton(next))
}

// tick advances the disc clocks (counter GPRMs, navigation timer) by the
// wall time since the last action. It returns a step if the timer fired.
func (s *Session) tick() *wittydvd.Step {
	now := time.Now()
	el := now.Sub(s.lastTick).Seconds()
	s.lastTick = now
	return s.nav.Tick(el)
}

// syncPosition maps the viewer's position in a linear title stream back to
// the cell the navigator should consider current.
func (s *Session) syncPosition(pos float64) {
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

// ended is called when the client finished the clip (and any still time).
func (s *Session) ended(ctx context.Context) *Presentation {
	cur := s.current
	if pl := s.playing; cur != nil && cur.Kind == "title" && cur.Title != nil && pl != nil {
		// The client played the rest of the flattened title in one stream;
		// walk the navigator through exactly those cells so cell and post
		// commands run, and stop as soon as the disc goes somewhere else.
		st := s.nav.CellEnded()
		for i := pl.idx + 1; i < len(pl.ts.Cells); i++ {
			c := pl.ts.Cells[i]
			if st.Kind != wittydvd.StepTitle || st.PGCN != c.PGCN || st.CellN != c.CellN || st.Title == nil || st.Title.TTN != cur.Title.TTN {
				break
			}
			st = s.nav.CellEnded()
		}
		return s.show(ctx, st)
	}
	return s.show(ctx, s.nav.CellEnded())
}

// ---------------------------------------------------------------------------
// Building presentations
// ---------------------------------------------------------------------------

func (s *Session) url(parts ...string) string {
	return s.m.BaseURL + "/dvd/" + s.ID + "/" + strings.Join(parts, "/")
}

func (s *Session) show(ctx context.Context, st *wittydvd.Step) *Presentation {
	return s.present(ctx, st, false)
}

// present turns a step into a presentation. A step with the same serial as
// the current presentation is the same screen with a moved highlight (or a
// message), so it reuses everything unless rebuild is set (stream changes).
func (s *Session) present(ctx context.Context, st *wittydvd.Step, rebuild bool) *Presentation {
	if st == nil {
		return s.current
	}
	if s.m.Log != nil {
		s.m.Log.Info("dvd step", "cat", "dvd", "session", s.ID, "kind", st.Kind, "domain", st.DomainName, "vts", st.VTS, "pgc", st.PGCN,
			"cell", st.CellN, "serial", st.Serial, "still", st.StillSec, "loop", st.Loop, "reason", st.Reason)
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
			if err := s.buildTitle(ctx, st, pl, p); err != nil {
				p.Kind, p.Reason = "stop", err.Error()
				p.Media = &Media{Type: "none"}
			} else {
				s.playing = pl
			}
			break
		}
		// Interactive or branching title cells (games, quizzes, stills)
		// play cell by cell like a menu so their buttons and commands work.
		p.Kind = "menu"
		if err := s.buildMenu(ctx, st, p); err != nil {
			p.Kind, p.Reason = "stop", err.Error()
			p.Media = &Media{Type: "none"}
		}
	default:
		p.Kind = "menu"
		if err := s.buildMenu(ctx, st, p); err != nil {
			p.Kind, p.Reason = "stop", err.Error()
			p.Media = &Media{Type: "none"}
		}
	}
	if t := s.nav.Registers().SPRM[wittydvd.SprmNavTimer]; t > 0 && p.Kind != "stop" {
		p.TimerSec = float64(t)
	}
	s.current = p
	if p.Kind == "menu" {
		s.prefetchNext(st, p)
	}
	return p
}

// title returns the flattened title (cached per session).
func (s *Session) title(ttn int) *wittydvd.TitleSummary {
	angle := s.angle(ttn)
	key := ttn*16 + angle
	if ts, ok := s.titles[key]; ok {
		return ts
	}
	ts := s.opened.Disc.TitleAngle(ttn, angle)
	s.titles[key] = ts
	return ts
}

// angle is the camera angle (SPRM3) to play for a title, 1 when the title has
// only one.
func (s *Session) angle(ttn int) int {
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

// switchAngle changes the camera angle of the title being streamed and
// continues at the same moment.
func (s *Session) switchAngle(ctx context.Context, angle int, pos float64) *Presentation {
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
	s.playing = &playingTitle{ts: ts, idx: k}
	s.seekTo = pos
	return s.present(ctx, s.nav.Current(), true)
}

// linearTitle decides whether a title step can be streamed as one continuous
// video from its current cell: the cell must be part of the title's normal
// chain, nothing in that chain may pause (stills) or loop, and the cell must
// not carry buttons. Anything else is played cell by cell.
func (s *Session) linearTitle(st *wittydvd.Step) *playingTitle {
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
	// Look for in-title buttons. Short cells (extras, games) are scanned in
	// full because their buttons often appear only near the end (voting,
	// "play again"); long feature cells only at the start. Scans are cached.
	hops := 48
	if st.CellDuration <= 300 {
		hops = 0
	}
	pic, err := s.opened.Menu(wittydvd.SpaceFor(st.Domain, st.VTS), st.FirstSector, st.LastSector, s.scanOptions(st, hops))
	if err == nil && interactive(pic) {
		return nil
	}
	return &playingTitle{ts: ts, idx: idx}
}

func interactive(pic *wittydvd.MenuPicture) bool {
	for _, b := range pic.Buttons {
		if !b.Inert {
			return true
		}
	}
	return false
}

// prefetchNext starts transcoding the clip that follows the one just shown,
// so menu transitions (intro -> looping menu, sub-menu fades) do not stall.
func (s *Session) prefetchNext(st *wittydvd.Step, p *Presentation) {
	if p.Media == nil || p.Media.Loop || p.Media.StillSec < 0 || st.Kind == wittydvd.StepStop {
		return
	}
	nx := s.nav.Clone().CellEnded()
	if nx == nil || nx.Kind != wittydvd.StepMenu || nx.CellDuration <= stillMaxSec {
		return
	}
	if id, err := s.menuHLS(wittydvd.SpaceFor(nx.Domain, nx.VTS), nx, 0); err == nil {
		if h := s.m.HLS.Get(id); h != nil {
			h.Warm()
		}
	}
}

func (s *Session) videoAttr(st *wittydvd.Step) wittydvd.VideoAttr {
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

// pgcOf is the program chain a step plays, as the navigator resolves it
// (including the menu language unit it settled on).
func (s *Session) pgcOf(st *wittydvd.Step) *wittydvd.PGC { return s.nav.StepPGC(st) }

const stillMaxSec = 1.5

// scanOptions builds the menu scan options for a step. Inside a title the
// highlight belongs to the sub-picture stream the disc selected (SPRM2).
func (s *Session) scanOptions(st *wittydvd.Step, maxHops int) wittydvd.MenuScanOptions {
	o := wittydvd.MenuScanOptions{Palette: st.Palette, Video: s.videoAttr(st), Wide: s.prefs.Wide, NavButton: st.DefaultButton, MaxHops: maxHops}
	if pgc := s.pgcOf(st); pgc != nil {
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

func (s *Session) buildMenu(ctx context.Context, st *wittydvd.Step, p *Presentation) error {
	sp := wittydvd.SpaceFor(st.Domain, st.VTS)
	pic, err := s.opened.Menu(sp, st.FirstSector, st.LastSector, s.scanOptions(st, 0))
	if errors.Is(err, wittydvd.ErrNoNav) {
		// The cell does not start with a NAV pack (damaged sectors, junk
		// cells): play what is there without buttons rather than stop.
		v := s.videoAttr(st)
		pic, err = &wittydvd.MenuPicture{Aspect: 4.0 / 3.0, NoButtons: true}, nil
		pic.FrameW, pic.FrameH = v.Size()
		if v.Wide {
			pic.Aspect = 16.0 / 9.0
		}
	}
	if err != nil {
		return err
	}
	if pic.Scrambled {
		p.Scrambled = true
		return errors.New("this disc is CSS-scrambled; decrypt it before adding it to the library")
	}
	s.menus[st.Serial] = &menuArt{pic: pic, highlights: map[int][]byte{}}
	for k := range s.menus {
		if k < st.Serial-4 {
			delete(s.menus, k)
		}
	}
	// Media
	still := st.StillSec
	loop := st.Loop
	isStill := !loop && (st.CellDuration <= stillMaxSec || (math.IsInf(still, 1) && st.CellDuration <= 3))
	m := &Media{Loop: loop, DurationSec: st.CellDuration, Audio: true}
	if math.IsInf(still, 1) {
		m.StillSec = -1
	} else {
		m.StillSec = still
	}
	if isStill {
		m.Type = "still"
		if !math.IsInf(still, 1) {
			m.StillSec = still + st.CellDuration
		}
		name, err := s.renderStill(ctx, sp, st)
		if err != nil {
			return err
		}
		m.URL = s.url("still", name)
		m.Audio = false
	} else {
		m.Type = "hls"
		audio := 0
		if st.Domain == wittydvd.DomainVTS {
			audio = -1
			if id := s.titleAudio(s.opened.Disc.VTS[st.VTS], s.pgcOf(st)); id > 0 {
				audio = id
			}
			m.Audio = audio >= 0
		}
		id, err := s.menuHLS(sp, st, audio)
		if err != nil {
			return err
		}
		m.URL = s.m.BaseURL + "/hls/" + id + "/playlist.m3u8"
	}
	p.Media = m
	// Menu layer
	menu := &Menu{Width: pic.FrameW, Height: pic.FrameH, Aspect: "4:3", Selected: pic.DefaultButton, ButtonsFromSec: pic.ButtonsFromSec, HasHighlights: pic.HasHighlights, Buttons: []ButtonView{},
		ForcedAction: pic.ForcedAction, ActionAtSec: pic.ActionAtSec, InTitle: st.Domain == wittydvd.DomainVTS}
	if pic.Aspect > 1.5 {
		menu.Aspect = "16:9"
	}
	// Most discs end the highlight with the cell (then a still holds it);
	// only an earlier end takes the buttons away.
	if u := pic.ButtonsUntilSec; u > 0 && m.Type == "hls" && !loop && u < st.CellDuration-0.5 {
		menu.ButtonsUntilSec = u
	}
	for _, b := range pic.Buttons {
		if b.Inert {
			continue
		}
		menu.Buttons = append(menu.Buttons, ButtonView{N: b.N, X1: b.X0, Y1: b.Y0, X2: b.X1, Y2: b.Y1, Up: b.Up, Down: b.Down, Left: b.Left, Right: b.Right, Auto: b.Auto, Lit: b.Select.Opaque(), Cmd: b.CmdText})
	}
	if pic.Normal != nil {
		menu.OverlayURL = s.url("menu", fmt.Sprint(st.Serial), "overlay.png")
	}
	if pic.HasHighlights {
		menu.HighlightURL = s.url("menu", fmt.Sprint(st.Serial), "highlight", "{n}.png")
	}
	p.Menu = menu
	return nil
}

func (s *Session) segmentsFor(sp wittydvd.Space, first, last uint32, dur float64) ([]ffmpeg.Segment, error) {
	ext, err := s.opened.RangeExtents(sp, first, last)
	if err != nil {
		return nil, err
	}
	if len(ext) == 0 {
		return nil, errors.New("VOB range missing from the disc")
	}
	return []ffmpeg.Segment{{Extents: ext, Duration: dur}}, nil
}

// stepSegments is the video of the step's cell, following only that cell's
// VOBUs when it is interleaved (multi-angle blocks, seamless branching).
func (s *Session) stepSegments(sp wittydvd.Space, st *wittydvd.Step) ([]ffmpeg.Segment, error) {
	if p := s.pgcOf(st); p != nil && st.CellN >= 1 && st.CellN <= len(p.Cells) {
		c := p.Cells[st.CellN-1]
		if c.Interleaved || c.BlockType == 1 {
			ext, err := s.opened.CellExtents(sp, c)
			if err != nil {
				return nil, err
			}
			if len(ext) > 0 {
				return []ffmpeg.Segment{{Extents: ext, Duration: st.CellDuration}}, nil
			}
		}
	}
	return s.segmentsFor(sp, st.FirstSector, st.LastSector, st.CellDuration)
}

func (s *Session) listDir() string { return filepath.Join(s.m.CacheDir, "lists", s.key) }

const dvdDeinterlace = "bwdif=mode=send_frame:parity=auto:deint=interlaced"

// Low-power machines get yadif (much cheaper than bwdif) and faster x264
// presets; DVD sources are SD so the quality cost is small.
const dvdDeinterlaceFast = "yadif=mode=send_frame:parity=auto:deint=interlaced"

func (s *Session) deinterlace() string {
	if s.prefs.LowPower {
		return dvdDeinterlaceFast
	}
	return dvdDeinterlace
}

func (s *Session) preset(menu bool) string {
	switch {
	case s.prefs.LowPower:
		return "ultrafast"
	case menu:
		return "superfast" // short clips: start-up latency matters more than bits
	}
	return ""
}

// titleAudio returns the MPEG-PS stream id of the audio the viewer hears in
// a title-domain step (SPRM1 through the PGC's audio control table), or 0
// when the title set has no audio.
func (s *Session) titleAudio(v *wittydvd.VTS, pgc *wittydvd.PGC) int {
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

// menuHLS streams one menu (or interactive title) cell. audio is the ffmpeg
// audio index in menu domains or, inside a title, the audio's stream id
// (>= 0x80), which pins the streams by id (see ffmpeg.WriteConcat).
func (s *Session) menuHLS(sp wittydvd.Space, st *wittydvd.Step, audio int) (string, error) {
	name := fmt.Sprintf("m-%s-%d-%d-%d-a%x%s", st.DomainName, st.VTS, st.PGCN, st.CellN, audio, s.codecTag())
	id := fmt.Sprintf("dvd-%s-%s", s.key, name)
	if s.m.HLS.Get(id) != nil {
		return id, nil
	}
	segs, err := s.stepSegments(sp, st)
	if err != nil {
		return "", err
	}
	var ids []int
	if audio >= 0x80 {
		ids, audio = []int{wittydvd.VideoStreamID, audio}, 0
	}
	list, err := ffmpeg.WriteConcat(s.listDir(), name, segs, ids...)
	if err != nil {
		return "", err
	}
	s.m.HLS.Ensure(hls.Spec{
		ID: id, InputArgs: ffmpeg.ConcatInputArgs(list), Duration: st.CellDuration, VideoFilter: s.deinterlace(), Preset: s.preset(true),
		AudioIndex: audio, AudioChannels: 2, SubtitleIndex: -1, LowLatency: true, HoldLastFrame: true, Quality: s.prefs.Quality, MaxHeight: s.prefs.MaxHeight, AudioCodec: s.prefs.AudioCodec,
	})
	return id, nil
}

func (s *Session) renderStill(ctx context.Context, sp wittydvd.Space, st *wittydvd.Step) (string, error) {
	name := fmt.Sprintf("%s-%s-%d-%d-%d.png", s.key, st.DomainName, st.VTS, st.PGCN, st.CellN)
	out := filepath.Join(s.m.CacheDir, "stills", name)
	if fileNonEmpty(out) {
		return name, nil
	}
	_ = os.MkdirAll(filepath.Dir(out), 0o755)
	segs, err := s.stepSegments(sp, st)
	if err != nil {
		return "", err
	}
	list, err := ffmpeg.WriteConcat(s.listDir(), "still-"+strings.TrimSuffix(name, ".png"), segs, wittydvd.VideoStreamID)
	if err != nil {
		return "", err
	}
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// Short still cells often fade in: prefer a frame near the end, then fall back to the first.
	at := 0.0
	if st.CellDuration > 0.6 {
		at = st.CellDuration - 0.3
	}
	if err := s.m.Tools.Still(tctx, ffmpeg.ConcatInputArgs(list), at, 0, out); err != nil || !fileNonEmpty(out) {
		if err := s.m.Tools.Still(tctx, ffmpeg.ConcatInputArgs(list), 0, 0, out); err != nil {
			return "", err
		}
	}
	return name, nil
}

func fileNonEmpty(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Size() > 0
}

// StillPath resolves a still file name.
func (s *Session) StillPath(name string) (string, bool) {
	if strings.ContainsAny(name, `/\`) || !strings.HasPrefix(name, s.key) {
		return "", false
	}
	p := filepath.Join(s.m.CacheDir, "stills", name)
	return p, fileNonEmpty(p)
}

// ---------------------------------------------------------------------------
// Titles
// ---------------------------------------------------------------------------

func (s *Session) buildTitle(ctx context.Context, st *wittydvd.Step, pl *playingTitle, p *Presentation) error {
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
	pgc := s.pgcOf(st)
	s.pickDefaultStreams(v, pgc)
	regs := s.nav.Registers()
	audioN := int(regs.SPRM[wittydvd.SprmAudio]) & 7
	subRaw := int(regs.SPRM[wittydvd.SprmSubp])
	subN := -1
	if subRaw&0x40 != 0 {
		subN = subRaw & 31
	}
	// SPRM1 is a logical stream number; the PGC's audio control table maps
	// it to the physical substream (0x80+n, ...). Streams are pinned by id,
	// so no probe is needed and cells that order their streams differently
	// (interleaved angle blocks) keep their sound.
	audioID := s.titleAudio(v, pgc)
	audioIdx := -1
	if audioID > 0 {
		audioIdx = 0
	}
	channels := 2
	if audioN < len(v.Audio) && v.Audio[audioN].Channels > 2 {
		channels = 6
	}
	runs, err := s.opened.TitleRuns(sp, ts.Cells)
	if err != nil {
		return err
	}
	var segs []ffmpeg.Segment
	for _, r := range runs {
		segs = append(segs, ffmpeg.Segment{Extents: r.Extents, Duration: r.Duration, Lead: r.Lead})
	}
	// Subtitles are rendered by the client from the disc's sub-pictures, so
	// the stream only depends on the audio track.
	angle := s.angle(ti.TTN)
	name := fmt.Sprintf("t%d-x%x-g%d%s", ti.TTN, audioID, angle, s.codecTag())
	audioFilter := ""
	if s.noVocals && audioN < len(v.Audio) && v.Audio[audioN].Karaoke() {
		name += "-nv"
		audioFilter = "pan=stereo|c0=FL|c1=FR" // the backing track only
	}
	id := fmt.Sprintf("dvd-%s-%s", s.key, name)
	if s.m.HLS.Get(id) == nil {
		ids := []int{wittydvd.VideoStreamID}
		if audioID > 0 {
			ids = append(ids, audioID)
		}
		list, err := ffmpeg.WriteConcat(s.listDir(), name, segs, ids...)
		if err != nil {
			return err
		}
		s.m.HLS.Ensure(hls.Spec{
			ID: id, InputArgs: ffmpeg.ConcatInputArgs(list), Duration: ts.Duration, VideoFilter: s.deinterlace(), Preset: s.preset(false), SeekMargin: 1.5,
			AudioIndex: audioIdx, AudioChannels: channels, AudioFilter: audioFilter, SubtitleIndex: -1, Quality: s.prefs.Quality, MaxHeight: s.prefs.MaxHeight, AudioCodec: s.prefs.AudioCodec,
		})
	}
	p.Media = &Media{Type: "hls", URL: s.m.BaseURL + "/hls/" + id + "/playlist.m3u8", DurationSec: ts.Duration, Audio: audioIdx >= 0}
	fw, fh := ts.Video.Size()
	aspect := "4:3"
	if ts.Video.Wide {
		aspect = "16:9"
	}
	tv := &TitleView{TTN: ti.TTN, Chapter: ti.PTTN, DurationSec: ts.Duration, StartSec: start, Chapters: ts.Chapters, AudioStream: audioN, SubStream: subN,
		Wide: ts.Video.Wide, Audio: []Track{}, Subtitles: []Track{}, FrameW: fw, FrameH: fh, Aspect: aspect,
		SubtitlesURL: s.url("subs") + fmt.Sprintf("?ttn=%d&angle=%d", ti.TTN, angle), ForcedStream: subRaw & 31,
		Angle: angle, Angles: 1}
	if audioFilter != "" {
		tv.AudioStream = audioN | karaokeOff
	}
	if pgc != nil {
		tv.Locks = uopLocks(pgc.UOPs)
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
		tv.Subtitles = append(tv.Subtitles, Track{N: i, Language: sub.Language, Purpose: sub.Purpose()})
	}
	if tv.Chapters == nil {
		tv.Chapters = []float64{}
	}
	p.Title = tv
	return nil
}

// pickDefaultStreams applies the viewer's language preferences when the disc
// has not chosen streams itself (SPRM1/SPRM2 still invalid for this title).
func (s *Session) pickDefaultStreams(v *wittydvd.VTS, pgc *wittydvd.PGC) {
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
// PNG artwork
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

// OverlayPNG renders the normal sub-picture for a menu serial.
func (s *Session) OverlayPNG(serial int) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	art := s.menus[serial]
	if art == nil || art.pic.Normal == nil {
		return nil, false
	}
	if art.overlay == nil {
		art.overlay = encodePNG(art.pic.Normal, art.pic.FrameW, art.pic.FrameH)
	}
	return art.overlay, true
}

// HighlightPNG renders the selected-state artwork for one button.
func (s *Session) HighlightPNG(serial, n int) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	art := s.menus[serial]
	if art == nil {
		return nil, false
	}
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
	return nil, false
}

func (s *Session) codecTag() string {
	if s.prefs.AudioCodec == "mp3" {
		return "-mp3" + s.prefs.Tag
	}
	return s.prefs.Tag
}

// karaokeOff marks a karaoke audio stream played without its vocals.
const karaokeOff = 0x100

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
