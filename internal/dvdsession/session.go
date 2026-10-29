// Package dvdsession turns navigator steps into things a client can show:
// HLS streams or still frames for the video, PNG overlays for sub-pictures and
// button highlights, and button geometry for input handling.
package dvdsession

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/disc/dvd"
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
	HasHighlights  bool         `json:"hasHighlights"`
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
}

type Track struct {
	N        int    `json:"n"`
	Language string `json:"language"`
	Codec    string `json:"codec,omitempty"`
	Channels int    `json:"channels,omitempty"`
}

// Prefs are viewer preferences for a session.
type Prefs struct {
	MenuLanguage     string
	AudioLanguage    string
	SubtitleLanguage string
	Wide             bool // viewer has a 16:9 display
	HW               bool
	MaxHeight        int
	AudioCodec       string // aac | mp3
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

// Session is one viewer's DVD player.
type Session struct {
	ID     string
	ItemID int64
	Path   string
	m      *Manager
	opened *dvd.Opened
	nav    *dvd.Nav
	prefs  Prefs
	key    string // stable per disc, for HLS/still cache ids

	mu      sync.Mutex
	used    time.Time
	current *Presentation
	menus   map[int]*menuArt
	probes  map[string]*streamMap
}

type menuArt struct {
	pic *dvd.MenuPicture
}

type streamMap struct {
	audioIdx map[int]int // DVD audio stream n -> ffmpeg a:index
	subIdx   map[int]int // sub-picture stream id (0..31) -> ffmpeg s:index
}

func randomID() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Open starts a session for a disc path.
func (m *Manager) Open(itemID int64, path string, prefs Prefs) (*Session, error) {
	o, err := dvd.Open(path)
	if err != nil {
		return nil, err
	}
	h := sha1.Sum([]byte(path))
	s := &Session{
		ID: randomID(), ItemID: itemID, Path: path, m: m, opened: o, prefs: prefs,
		key: hex.EncodeToString(h[:8]), used: time.Now(), menus: map[int]*menuArt{}, probes: map[string]*streamMap{},
	}
	s.nav = dvd.NewNav(o.Disc, dvd.NavOptions{RegisterOptions: dvd.RegisterOptions{MenuLanguage: prefs.MenuLanguage, AudioLanguage: prefs.AudioLanguage, SubtitleLanguage: prefs.SubtitleLanguage}})
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	return s, nil
}

// Get fetches a session.
func (m *Manager) Get(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sessions[id]; s != nil {
		s.touch()
		return s
	}
	return nil
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

func (s *Session) Close() { _ = s.opened.Close() }

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
	return s.show(ctx, s.nav.PlayTitle(ttn, chapter))
}

// Action is a client input.
type Action struct {
	Type    string `json:"type"` // press | select | key | ended | title | menu | resume | chapter | audio | subtitle
	Button  int    `json:"button"`
	Key     string `json:"key"` // up down left right enter menu title resume next prev
	TTN     int    `json:"ttn"`
	Chapter int    `json:"chapter"`
	Stream  int    `json:"stream"`
	On      bool   `json:"on"`
	Menu    string `json:"menu"` // root | title | audio | subpicture | angle | chapter
}

// Do applies an action and returns the new presentation.
func (s *Session) Do(ctx context.Context, a Action) *Presentation {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.current
	switch a.Type {
	case "press":
		return s.press(ctx, a.Button)
	case "select":
		return s.show(ctx, s.nav.SelectButton(a.Button))
	case "key":
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
		return s.show(ctx, s.nav.MenuKey(kind))
	case "resume":
		return s.show(ctx, s.nav.Resume())
	case "chapter":
		if a.Chapter > 0 {
			return s.show(ctx, s.nav.NextChapter())
		}
		return s.show(ctx, s.nav.PrevChapter())
	case "audio":
		s.nav.SetAudio(a.Stream)
		if cur != nil && cur.Kind == "title" {
			return s.show(ctx, s.nav.Current())
		}
		return cur
	case "subtitle":
		s.nav.SetSubpicture(a.Stream, a.On)
		if cur != nil && cur.Kind == "title" {
			return s.show(ctx, s.nav.Current())
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
	var cmd *dvd.Command
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

// ended is called when the client finished the clip (and any still time).
func (s *Session) ended(ctx context.Context) *Presentation {
	cur := s.current
	if cur != nil && cur.Kind == "title" && cur.Title != nil {
		// The client played the whole flattened title; walk the navigator
		// through its remaining cells so cell/post commands run.
		ttn := cur.Title.TTN
		st := s.nav.CellEnded()
		for i := 0; i < 2000 && st.Kind == dvd.StepTitle && st.Title != nil && st.Title.TTN == ttn; i++ {
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

func (s *Session) show(ctx context.Context, st *dvd.Step) *Presentation {
	if st == nil {
		return s.current
	}
	p := &Presentation{Serial: st.Serial, Reason: st.Reason}
	switch st.Kind {
	case dvd.StepStop:
		p.Kind = "stop"
		p.Media = &Media{Type: "none"}
	case dvd.StepTitle:
		p.Kind = "title"
		if err := s.buildTitle(ctx, st, p); err != nil {
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
	s.current = p
	return p
}

func (s *Session) videoAttr(st *dvd.Step) dvd.VideoAttr {
	switch st.Domain {
	case dvd.DomainVTSM:
		if v := s.opened.Disc.VTS[st.VTS]; v != nil {
			return v.MenuVideo
		}
	case dvd.DomainVTS:
		if v := s.opened.Disc.VTS[st.VTS]; v != nil {
			return v.Video
		}
	}
	return s.opened.Disc.VMG.MenuVideo
}

func (s *Session) pgcOf(st *dvd.Step) *dvd.PGC {
	lang := s.nav.Registers().SPRM[dvd.SprmMenuLang]
	pick := func(units []dvd.LangUnit) []dvd.PGCEntry {
		if len(units) == 0 {
			return nil
		}
		for _, u := range units {
			if len(u.Language) == 2 && uint16(u.Language[0])<<8|uint16(u.Language[1]) == lang {
				return u.PGCs
			}
		}
		return units[0].PGCs
	}
	var list []dvd.PGCEntry
	switch st.Domain {
	case dvd.DomainFP:
		return s.opened.Disc.VMG.FirstPlay
	case dvd.DomainVMGM:
		list = pick(s.opened.Disc.VMG.Menus)
	case dvd.DomainVTSM:
		if v := s.opened.Disc.VTS[st.VTS]; v != nil {
			list = pick(v.Menus)
		}
	case dvd.DomainVTS:
		if v := s.opened.Disc.VTS[st.VTS]; v != nil {
			list = v.PGCs
		}
	}
	if st.PGCN >= 1 && st.PGCN <= len(list) {
		return list[st.PGCN-1].PGC
	}
	return nil
}

const stillMaxSec = 1.5

func (s *Session) buildMenu(ctx context.Context, st *dvd.Step, p *Presentation) error {
	sp := dvd.SpaceFor(st.Domain, st.VTS)
	video := s.videoAttr(st)
	pgc := s.pgcOf(st)
	var subc *dvd.SubpControl
	if pgc != nil {
		subc = pgc.SubpControl[0]
	}
	rd, err := s.opened.Reader(sp)
	if err != nil {
		return err
	}
	pic, err := dvd.ScanMenu(rd, st.FirstSector, st.LastSector, dvd.MenuScanOptions{
		Palette: st.Palette, Video: video, SubpControl: subc, Wide: s.prefs.Wide, NavButton: st.DefaultButton,
	})
	if err != nil {
		return err
	}
	if pic.Scrambled {
		p.Scrambled = true
		return errors.New("this disc is CSS-scrambled; decrypt it before adding it to the library")
	}
	s.menus[st.Serial] = &menuArt{pic: pic}
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
		id, err := s.menuHLS(sp, st)
		if err != nil {
			return err
		}
		m.URL = s.m.BaseURL + "/hls/" + id + "/playlist.m3u8"
	}
	p.Media = m
	// Menu layer
	menu := &Menu{Width: pic.FrameW, Height: pic.FrameH, Aspect: "4:3", Selected: pic.DefaultButton, ButtonsFromSec: pic.ButtonsFromSec, HasHighlights: pic.HasHighlights, Buttons: []ButtonView{}}
	if pic.Aspect > 1.5 {
		menu.Aspect = "16:9"
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

func (s *Session) segmentsFor(sp dvd.Space, first, last uint32, dur float64) ([]ffmpeg.Segment, error) {
	ext, err := s.opened.RangeExtents(sp, first, last)
	if err != nil {
		return nil, err
	}
	if len(ext) == 0 {
		return nil, errors.New("VOB range missing from the disc")
	}
	return []ffmpeg.Segment{{Extents: ext, Duration: dur}}, nil
}

func (s *Session) listDir() string { return filepath.Join(s.m.CacheDir, "lists", s.key) }

const dvdDeinterlace = "bwdif=mode=send_frame:parity=auto:deint=interlaced"

func (s *Session) menuHLS(sp dvd.Space, st *dvd.Step) (string, error) {
	segs, err := s.segmentsFor(sp, st.FirstSector, st.LastSector, st.CellDuration)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("m-%s-%d-%d-%d%s", st.DomainName, st.VTS, st.PGCN, st.CellN, s.codecTag())
	list, err := ffmpeg.WriteConcat(s.listDir(), name, segs)
	if err != nil {
		return "", err
	}
	id := fmt.Sprintf("dvd-%s-%s", s.key, name)
	s.m.HLS.Ensure(hls.Spec{
		ID: id, InputArgs: ffmpeg.ConcatInputArgs(list), Duration: st.CellDuration, VideoFilter: dvdDeinterlace,
		AudioIndex: 0, AudioChannels: 2, SubtitleIndex: -1, LowLatency: true, HW: s.prefs.HW, MaxHeight: s.prefs.MaxHeight, AudioCodec: s.prefs.AudioCodec,
	})
	return id, nil
}

func (s *Session) renderStill(ctx context.Context, sp dvd.Space, st *dvd.Step) (string, error) {
	name := fmt.Sprintf("%s-%s-%d-%d-%d.png", s.key, st.DomainName, st.VTS, st.PGCN, st.CellN)
	out := filepath.Join(s.m.CacheDir, "stills", name)
	if fileNonEmpty(out) {
		return name, nil
	}
	_ = os.MkdirAll(filepath.Dir(out), 0o755)
	segs, err := s.segmentsFor(sp, st.FirstSector, st.LastSector, st.CellDuration)
	if err != nil {
		return "", err
	}
	list, err := ffmpeg.WriteConcat(s.listDir(), "still-"+strings.TrimSuffix(name, ".png"), segs)
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

func (s *Session) buildTitle(ctx context.Context, st *dvd.Step, p *Presentation) error {
	ti := st.Title
	ts := s.opened.Disc.Title(ti.TTN)
	if ts == nil || len(ts.Cells) == 0 {
		return fmt.Errorf("title %d has no cells", ti.TTN)
	}
	sp := dvd.Space{Domain: dvd.DomainVTS, VTS: ts.VTS}
	start := 0.0
	for _, c := range ts.Cells {
		if c.PGCN == st.PGCN && c.CellN == st.CellN {
			start = c.Start
			break
		}
	}
	regs := s.nav.Registers()
	audioN := int(regs.SPRM[dvd.SprmAudio])
	if audioN > 7 {
		audioN = 0
	}
	subRaw := int(regs.SPRM[dvd.SprmSubp])
	subN := -1
	if subRaw&0x40 != 0 {
		subN = subRaw & 31
	}
	v := s.opened.Disc.VTS[ts.VTS]
	sm, err := s.streamMap(ctx, sp, ts)
	if err != nil {
		return err
	}
	audioIdx := -1
	if audioN < len(v.Audio) {
		if idx, ok := sm.audioIdx[audioN]; ok {
			audioIdx = idx
		}
	}
	if audioIdx < 0 && len(sm.audioIdx) > 0 {
		audioIdx = 0
	}
	subIdx := -1
	if subN >= 0 && subN < len(v.Subp) {
		pgc := s.pgcOf(st)
		stream := subN
		if pgc != nil && pgc.SubpControl[subN] != nil {
			sc := pgc.SubpControl[subN]
			switch {
			case ts.Video.Wide && s.prefs.Wide:
				stream = sc.Wide
			case ts.Video.Wide:
				stream = sc.Letterbox
			default:
				stream = sc.Narrow
			}
		}
		if idx, ok := sm.subIdx[stream]; ok {
			subIdx = idx
		}
	}
	channels := 2
	if audioN < len(v.Audio) && v.Audio[audioN].Channels > 2 {
		channels = 6
	}
	var segs []ffmpeg.Segment
	for _, c := range ts.Cells {
		ext, err := s.opened.RangeExtents(sp, c.Cell.FirstSector, c.Cell.LastSector)
		if err != nil {
			return err
		}
		if len(ext) == 0 {
			continue
		}
		segs = append(segs, ffmpeg.Segment{Extents: ext, Duration: c.Cell.Duration})
	}
	name := fmt.Sprintf("t%d-a%d-s%d%s", ti.TTN, audioIdx, subIdx, s.codecTag())
	list, err := ffmpeg.WriteConcat(s.listDir(), name, segs)
	if err != nil {
		return err
	}
	id := fmt.Sprintf("dvd-%s-%s", s.key, name)
	s.m.HLS.Ensure(hls.Spec{
		ID: id, InputArgs: ffmpeg.ConcatInputArgs(list), Duration: ts.Duration, VideoFilter: dvdDeinterlace,
		AudioIndex: audioIdx, AudioChannels: channels, SubtitleIndex: subIdx, HW: s.prefs.HW, MaxHeight: s.prefs.MaxHeight, AudioCodec: s.prefs.AudioCodec,
	})
	p.Media = &Media{Type: "hls", URL: s.m.BaseURL + "/hls/" + id + "/playlist.m3u8", DurationSec: ts.Duration, Audio: audioIdx >= 0}
	tv := &TitleView{TTN: ti.TTN, Chapter: ti.PTTN, DurationSec: ts.Duration, StartSec: start, Chapters: ts.Chapters, AudioStream: audioN, SubStream: subN, Wide: ts.Video.Wide, Audio: []Track{}, Subtitles: []Track{}}
	for i, a := range v.Audio {
		tv.Audio = append(tv.Audio, Track{N: i, Language: a.Language, Codec: a.Codec, Channels: a.Channels})
	}
	for i, sub := range v.Subp {
		tv.Subtitles = append(tv.Subtitles, Track{N: i, Language: sub.Language})
	}
	if tv.Chapters == nil {
		tv.Chapters = []float64{}
	}
	p.Title = tv
	return nil
}

// streamMap probes the start of a title to learn how ffmpeg orders the DVD
// streams (the concat demuxer drops stream ids, so we look at one subfile).
func (s *Session) streamMap(ctx context.Context, sp dvd.Space, ts *dvd.TitleSummary) (*streamMap, error) {
	key := fmt.Sprintf("t%d", ts.Number)
	if sm, ok := s.probes[key]; ok {
		return sm, nil
	}
	c := ts.Cells[0].Cell
	last := c.FirstSector + 8192
	if last > c.LastSector {
		last = c.LastSector
	}
	ext, err := s.opened.RangeExtents(sp, c.FirstSector, last)
	if err != nil || len(ext) == 0 {
		return nil, errors.New("cannot read the title's first cell")
	}
	pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pr, err := s.m.Tools.ProbeInput(pctx, []string{"-probesize", "50M", "-analyzeduration", "20M", "-i", ffmpeg.SubfileURL(ext[0])})
	if err != nil {
		return nil, err
	}
	sm := &streamMap{audioIdx: map[int]int{}, subIdx: map[int]int{}}
	ai, si := 0, 0
	for _, st := range pr.Streams {
		var id int
		fmt.Sscanf(st.ID, "0x%x", &id)
		switch st.CodecType {
		case "audio":
			// 0x80+n AC3, 0x88+n DTS, 0xa0+n LPCM, 0xc0+n MPEG: n is the DVD stream number.
			n := -1
			switch {
			case id >= 0x80 && id <= 0x87:
				n = id - 0x80
			case id >= 0x88 && id <= 0x8f:
				n = id - 0x88
			case id >= 0xa0 && id <= 0xa7:
				n = id - 0xa0
			case id >= 0xc0 && id <= 0xc7:
				n = id - 0xc0
			}
			if n >= 0 {
				sm.audioIdx[n] = ai
			}
			ai++
		case "subtitle":
			if id >= 0x20 && id <= 0x3f {
				sm.subIdx[id-0x20] = si
			}
			si++
		}
	}
	s.probes[key] = sm
	return sm, nil
}

// ---------------------------------------------------------------------------
// PNG artwork
// ---------------------------------------------------------------------------

func encodePNG(r *dvd.RGBA, w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	if r != nil {
		for y := 0; y < r.Height; y++ {
			for x := 0; x < r.Width; x++ {
				i := (y*r.Width + x) * 4
				if r.Pix[i+3] == 0 {
					continue
				}
				img.SetNRGBA(r.X+x, r.Y+y, color.NRGBA{r.Pix[i], r.Pix[i+1], r.Pix[i+2], r.Pix[i+3]})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
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
	return encodePNG(art.pic.Normal, art.pic.FrameW, art.pic.FrameH), true
}

// HighlightPNG renders the selected-state artwork for one button.
func (s *Session) HighlightPNG(serial, n int) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	art := s.menus[serial]
	if art == nil {
		return nil, false
	}
	for _, b := range art.pic.Buttons {
		if b.N == n {
			return encodePNG(b.Select, art.pic.FrameW, art.pic.FrameH), true
		}
	}
	return nil, false
}

func (s *Session) codecTag() string {
	if s.prefs.AudioCodec == "mp3" {
		return "-mp3"
	}
	return ""
}
