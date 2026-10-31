package bdsession

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittybluray/bdj"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
)

// BD-J titles: the disc's applications run in a Java process (package bdj),
// drawing into a picture the player lays over the video and asking for
// playlists, which play here like any other. The navigator's registers are the
// truth; the applications keep a copy.

// BDJConfig says whether BD-J is turned on, and which Java to use ("" = find it).
type BDJConfig func() (enabled bool, javaPath string)

// BDJPoll is what the player waits for: a new picture, or something else the
// applications did.
type BDJPoll struct {
	Change       int           `json:"change"`
	FrameURL     string        `json:"frameUrl,omitempty"`
	Presentation *Presentation `json:"presentation,omitempty"`
	Sounds       []int         `json:"sounds,omitempty"`
	Pause        *bool         `json:"pause,omitempty"`
	Seek         *float64      `json:"seek,omitempty"`
	Gone         bool          `json:"gone,omitempty"`
}

// bdjState is a session's BD-J side.
type bdjState struct {
	host   *bdj.Host
	obj    *wittybluray.BDJObject
	events chan bdj.Event
	failed int
	// One-shot messages for the next poll.
	sounds []int
	pause  *bool
	seek   *float64
	video  []int
	active bool // the current presentation belongs to BD-J
	// A disc can ask for playlists and titles far faster than they can start, so
	// only the newest request of each kind is carried out, at a steady pace.
	pending map[string]*atomic.Int32
	lastGo  time.Time
	limit   string // why the process was ended, if it passed a limit
	// How many of the title's applications have ended themselves, and when the
	// disc last went back to its menu because of that.
	ended     int
	menuExits []time.Time
}

const (
	bdjMenuExits      = 3
	bdjMenuExitWindow = time.Minute
)

// newPending counts the waiting requests of the kinds that start something expensive.
func newPending() map[string]*atomic.Int32 {
	return map[string]*atomic.Int32{"play": {}, "title": {}}
}

// bdjPace is how often a disc's requests may start something expensive.
const bdjPace = 150 * time.Millisecond

// bdjIdle is how long a session with Java running may go unasked. A player showing
// the applications polls every few seconds, so this means the viewer has gone, and
// a Java process is too much to leave running for the usual half hour.
const bdjIdle = 3 * time.Minute

func (s *Session) idleTooLong() bool {
	s.mu.Lock()
	running := s.bd != nil
	s.mu.Unlock()
	idle := time.Since(s.lastUsed())
	return idle > 30*time.Minute || (running && idle > bdjIdle)
}

// storageKey names a disc's storage: the same however the disc is reached and from
// one server start to the next, and different for a different disc. Scores and
// bookmarks belong to the disc's program, so that is what it is worked out from.
func storageKey(d *wittybluray.Disc) string {
	h := sha256.New()
	_, _ = io.WriteString(h, d.Label)
	for _, f := range []string{"index.bdmv", "MovieObject.bdmv"} {
		if b, err := d.ReadProgramFile(f, 4<<20); err == nil {
			_, _ = h.Write(b)
		}
	}
	add := func(o wittybluray.IndexObject) {
		if o.Kind == wittybluray.ObjectBDJ {
			if b, err := d.BDJObject(o.BDJName); err == nil {
				_, _ = fmt.Fprintf(h, "%s %v %+v;", b.Name, b.Playlists, b.Apps)
			}
		}
	}
	add(d.Index.FirstPlay)
	add(d.Index.TopMenu)
	for _, t := range d.Index.Titles {
		add(t.Object)
	}
	return hex.EncodeToString(h.Sum(nil)[:12])
}

// ensureHost starts Java for this session, once. nil means BD-J is off or
// cannot run, and the disc plays from its playlists instead.
func (s *Session) ensureHost(ctx context.Context) *bdj.Host {
	if s.bd != nil {
		return s.bd.host
	}
	if s.m.BDJ == nil {
		return nil
	}
	enabled, path := s.m.BDJ()
	if !enabled {
		return nil
	}
	java, err := bdj.Find(path)
	if err != nil {
		s.warn("BD-J", err)
		return nil
	}
	st := &bdjState{events: make(chan bdj.Event, 4096), pending: newPending()}
	host, err := bdj.Start(ctx, s.disc, bdj.Options{
		Java:       java,
		CacheDir:   filepath.Join(s.m.CacheDir, "bdj"),
		WorkDir:    filepath.Join(s.m.CacheDir, "bdj", "sessions", s.ID),
		PersistDir: filepath.Join(s.m.CacheDir, "bdj", "discs", storageKey(s.disc)),
		Log: func(l string) {
			if s.m.Log != nil {
				s.m.Log.Debug("bd-j", "cat", "dvd", "session", s.ID, "msg", l)
			}
		},
		OnEvent: func(e bdj.Event) {
			if e.Kind == "exit" {
				st.events <- e // never lost: the loop below is waiting for it to end
				return
			}
			p := st.pending[e.Kind]
			if p != nil {
				p.Add(1)
			}
			select {
			case st.events <- e:
			default: // a disc flooding the link loses its own messages
				if p != nil {
					p.Add(-1)
				}
			}
		},
	})
	if err != nil {
		s.warn("BD-J", err)
		return nil
	}
	st.host = host
	s.bd = st
	go s.bdjLoop(st)
	return host
}

func (s *Session) warn(what string, err error) {
	if s.m.Log != nil {
		s.m.Log.Warn(what, "cat", "dvd", "session", s.ID, "err", err)
	}
}

func (s *Session) bdjLoop(st *bdjState) {
	for e := range st.events {
		if p := st.pending[e.Kind]; p != nil {
			if p.Add(-1) > 0 {
				continue // a newer one is on its way
			}
			if wait := bdjPace - time.Since(st.lastGo); wait > 0 {
				time.Sleep(wait)
			}
			st.lastGo = time.Now()
		}
		s.mu.Lock()
		if s.bd == st {
			s.bdjEvent(st, e)
		}
		s.mu.Unlock()
		if e.Kind == "exit" {
			return
		}
	}
}

func argInt(a []string, i int) int {
	if i >= len(a) {
		return -1
	}
	v, err := strconv.Atoi(a[i])
	if err != nil {
		return -1
	}
	return v
}

func argFloat(a []string, i int) float64 {
	if i >= len(a) {
		return 0
	}
	v, _ := strconv.ParseFloat(a[i], 64)
	return v
}

// bdjEvent carries out what an application asked for. Everything here came
// from disc code: numbers are checked like any other disc data.
func (s *Session) bdjEvent(st *bdjState, e bdj.Event) {
	ctx := context.Background()
	r := s.nav.Registers()
	switch e.Kind {
	case "play":
		s.bdjPlay(ctx, st, argInt(e.Args, 0), argInt(e.Args, 1), argInt(e.Args, 2), argFloat(e.Args, 3))
	case "stop":
		s.current = s.bdjView(nil)
		st.host.Kick()
	case "pause", "resume":
		v := e.Kind == "pause"
		st.pause = &v
		st.host.Kick()
	case "seek":
		v := argFloat(e.Args, 0)
		st.seek = &v
		st.host.Kick()
	case "video":
		if len(e.Args) == 4 {
			x, y, w, h := argInt(e.Args, 0), argInt(e.Args, 1), argInt(e.Args, 2), argInt(e.Args, 3)
			if x >= 0 && y >= 0 && w > 0 && h > 0 && x+w <= bdj.Width && y+h <= bdj.Height {
				st.video = []int{x, y, w, h}
				if s.current != nil && s.current.BDJ != nil {
					p, v := *s.current, *s.current.BDJ
					v.Video = st.video
					p.BDJ = &v
					s.current = &p
				}
				st.host.Kick()
			}
		}
	case "sound":
		if id := argInt(e.Args, 0); id >= 0 && len(st.sounds) < 16 {
			st.sounds = append(st.sounds, id)
			st.host.Kick()
		}
	case "gpr":
		if n := argInt(e.Args, 0); n >= 0 && n < len(r.GPR) && len(e.Args) > 1 {
			if v, err := strconv.ParseUint(e.Args[1], 10, 32); err == nil {
				r.GPR[n] = uint32(v)
			}
		}
	case "psr":
		// The same registers a disc's own commands may not change stay out of reach.
		if n := argInt(e.Args, 0); n >= 0 && n < 128 && !playerSetting(n) && len(e.Args) > 1 {
			if v, err := strconv.ParseUint(e.Args[1], 10, 32); err == nil {
				changed := r.PSR[n] != uint32(v)
				r.PSR[n] = uint32(v)
				if changed && (n == wittybluray.PSRAudio || n == wittybluray.PSRAngle) && s.step != nil && s.current != nil && s.current.Kind == "title" {
					s.current = s.bdjOver(s.present(ctx, s.step, true), st)
					st.host.Kick()
				}
			}
		}
	case "title":
		s.show(ctx, s.nav.PlayTitle(argInt(e.Args, 0)))
		st.host.Kick()
	case "appfailed":
		st.failed++
		s.warn("BD-J application", fmt.Errorf("%s", strings.Join(e.Args, " ")))
		// A title whose applications will not run is what happens without
		// Java, so it ends the same way: the disc plays from its playlists.
		if st.obj != nil && st.failed >= len(st.obj.Autostart()) && st.host.Version() == 0 {
			s.bdjFallback(ctx, st, "This disc's menu could not run ("+shortReason(e.Args)+"). Playing the main feature.")
		}
	case "appdestroyed":
		st.ended++
		if st.obj != nil && st.active && st.ended >= len(st.obj.Autostart()) {
			s.bdjAppsEnded(ctx, st)
		}
	case "limit":
		st.limit = strings.Join(e.Args, " ")
		s.warn("BD-J application", fmt.Errorf("stopped: %s", st.limit))
	case "exit":
		st.active = false
		s.bd = nil
		if s.current != nil && s.current.BDJ != nil {
			why := "This disc's menu stopped unexpectedly. Playing the main feature."
			if st.limit != "" {
				why = "This disc's menu was stopped because " + st.limit + ". Playing the main feature."
			}
			s.bdjFallbackStep(ctx, why)
		}
	}
}

// bdjAppsEnded is a title whose applications have all finished by themselves. A
// film they started goes on; with nothing playing the disc goes back to its menu.
// A menu that ends itself each time it starts would loop for ever, so after a few
// goes the disc plays its feature instead.
func (s *Session) bdjAppsEnded(ctx context.Context, st *bdjState) {
	st.active = false
	if s.current != nil && s.current.Kind == "title" {
		p := *s.current
		p.BDJ = nil
		s.serial++
		p.Serial = s.serial
		s.current = &p
		st.host.Kick()
		return
	}
	now := time.Now()
	kept := st.menuExits[:0]
	for _, t := range st.menuExits {
		if now.Sub(t) < bdjMenuExitWindow {
			kept = append(kept, t)
		}
	}
	st.menuExits = append(kept, now)
	if len(st.menuExits) > bdjMenuExits {
		s.bdjFallback(ctx, st, "This disc's menu keeps closing. Playing the main feature.")
		return
	}
	s.show(ctx, s.nav.MenuCall())
	st.host.Kick()
}

func shortReason(a []string) string {
	if len(a) < 2 {
		return "unknown error"
	}
	r := strings.Join(a[1:], " ")
	if len(r) > 120 {
		r = r[:120]
	}
	return r
}

func playerSetting(n int) bool {
	return n == 13 || (n >= 15 && n <= 21) || (n >= 23 && n <= 31) || (n >= 48 && n <= 61) || n >= 96
}

// bdjFallback ends the applications and plays the feature.
func (s *Session) bdjFallback(ctx context.Context, st *bdjState, why string) {
	st.host.Send("stopapps")
	st.active = false
	s.bdjFallbackStep(ctx, why)
	st.host.Kick()
}

func (s *Session) bdjFallbackStep(ctx context.Context, why string) {
	pl := s.disc.MainPlaylist()
	if pl == nil {
		s.stop(why, false)
		return
	}
	stp := s.nav.PlayPlaylist(pl, 0, 0)
	stp.Reason = why
	s.present(ctx, stp, false)
}

// bdjView is the presentation of the applications' graphics over whatever plays.
func (s *Session) bdjView(media *Media) *Presentation {
	s.serial++
	if media == nil {
		media = &Media{Type: "none"}
	}
	p := &Presentation{Kind: "menu", Serial: s.serial, Media: media, Menu: &Menu{Width: bdj.Width, Height: bdj.Height, Aspect: "16:9", Buttons: []ButtonView{}}}
	return s.bdjOver(p, s.bd)
}

// bdjOver marks a presentation as having the graphics plane over it.
func (s *Session) bdjOver(p *Presentation, st *bdjState) *Presentation {
	if p == nil || st == nil {
		return p
	}
	p.BDJ = &dvdsession.BDJView{PollURL: s.url("bdj"), Video: st.video}
	return p
}

// startBDJTitle starts a BD-J object's applications; ok is false when they
// cannot run here, and the caller falls back.
func (s *Session) startBDJTitle(ctx context.Context, st *wittybluray.Step) (*Presentation, bool) {
	host := s.ensureHost(ctx)
	if host == nil {
		return nil, false
	}
	obj, err := s.disc.BDJObject(st.BDJ)
	if err != nil || len(obj.Autostart()) == 0 {
		if err != nil {
			s.warn("BD-J object", err)
		}
		return nil, false
	}
	s.bd.obj, s.bd.failed, s.bd.ended, s.bd.active, s.bd.video = obj, 0, 0, true, nil
	host.Sync(s.nav.Registers())
	host.StartTitle(st.Title, obj)
	s.menu, s.playlist = nil, nil
	s.current = s.bdjView(nil)
	if obj.AutostartFirst && len(obj.Playlists) > 0 {
		s.bdjPlay(ctx, s.bd, obj.Playlists[0], -1, -1, 0)
	}
	return s.current, true
}

// bdjPlay plays what an application asked for. Long playlists present as a
// title, with chapters and subtitles; short ones (a menu's moving background)
// present as a clip with nothing to press but the application's own graphics.
func (s *Session) bdjPlay(ctx context.Context, st *bdjState, n, item, mark int, at float64) {
	pl := s.disc.Playlist(n)
	if pl == nil || len(pl.Items) == 0 {
		st.host.Send("playfailed no-such-playlist")
		return
	}
	if item < 0 || item >= len(pl.Items) {
		item = 0
	}
	t := uint32(0)
	if mark >= 0 && mark < len(pl.Marks) && pl.Marks[mark].Item < len(pl.Items) {
		item, t = pl.Marks[mark].Item, pl.Marks[mark].Time
	}
	step := s.nav.PlayPlaylist(pl, item, t)
	step.ItemOnly, step.StillSec = false, 0
	step.Menu = false
	s.seekTo = at
	s.serial++
	s.step = step
	p := &Presentation{Serial: s.serial}
	var err error
	if s.disc.PlaylistDuration(pl) >= 300 {
		err = s.buildTitle(ctx, step, p)
	} else {
		s.menu = nil
		err = s.buildMenu(ctx, step, p)
	}
	if err != nil {
		st.host.Send("playfailed %s", strings.ReplaceAll(err.Error(), " ", "-"))
		return
	}
	s.current = s.bdjOver(p, st)
	laid := s.layout(pl, -1)
	marks := make([]string, 0, len(pl.Marks))
	for _, m := range pl.Marks {
		if pi := laid.find(m.Item); pi != nil {
			marks = append(marks, fmt.Sprintf("%.3f", pi.start+float64(int64(m.Time)-int64(pi.startPTS))/45000))
		}
	}
	items := make([]string, 0, len(laid.items))
	for _, pi := range laid.items {
		items = append(items, fmt.Sprintf("%.3f", pi.start))
	}
	st.host.Send("plinfo %d %.3f %s %s", pl.Number, laid.duration, orDash(strings.Join(marks, ",")), orDash(strings.Join(items, ",")))
	st.host.Kick()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// The viewer's keys, as the applications know them.
var bdjKeys = map[string]int{"up": 38, "down": 40, "left": 37, "right": 39, "enter": 10, "ok": 10, "select": 10,
	"red": 403, "green": 404, "yellow": 405, "blue": 406, "play": 415, "stop": 413, "pause": 19, "ff": 417, "rew": 412, "next": 425, "prev": 424,
	"popup": 461, "menu": 461, "digit0": 48, "digit1": 49, "digit2": 50, "digit3": 51, "digit4": 52, "digit5": 53, "digit6": 54, "digit7": 55, "digit8": 56, "digit9": 57}

// bdjAction handles a client action while the applications are on screen;
// handled is false for what the navigator should do instead.
func (s *Session) bdjAction(ctx context.Context, a Action) (p *Presentation, handled bool) {
	st := s.bd
	if st == nil || !st.active || s.current == nil || s.current.BDJ == nil {
		return nil, false
	}
	switch a.Type {
	case "tick":
		if a.PositionSec > 0 {
			st.host.Send("time %.3f", a.PositionSec)
		}
		return s.current, true
	case "ended":
		st.host.Send("ended")
		return s.current, true
	case "key":
		if code, ok := bdjKeys[a.Key]; ok {
			st.host.Send("key %d", code)
		}
		return s.current, true
	case "menu":
		if st.obj != nil && st.obj.MenuCallMask {
			return s.current, true
		}
	}
	return nil, false
}

// leaveBDJ ends the applications when the disc moves on to something else.
func (s *Session) leaveBDJ() {
	if s.bd != nil && s.bd.active {
		s.bd.active = false
		s.bd.host.Send("stopapps")
		s.bd.host.Kick()
	}
}

// PollBDJ waits for a change to report to the player.
func (s *Session) PollBDJ(ctx context.Context, since int) *BDJPoll {
	s.mu.Lock()
	st := s.bd
	s.mu.Unlock()
	if st == nil {
		return &BDJPoll{Gone: true, Presentation: s.Current()}
	}
	change := st.host.WaitChange(ctx, since, 8*time.Second)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = time.Now()
	out := &BDJPoll{Change: change, Presentation: s.current, Sounds: st.sounds, Pause: st.pause, Seek: st.seek}
	st.sounds, st.pause, st.seek = nil, nil, nil
	if st.active {
		out.FrameURL = s.url("bdj", "frame", fmt.Sprintf("%d.png", st.host.Version()))
	}
	return out
}

// BDJFrame is the applications' graphics now.
func (s *Session) BDJFrame() ([]byte, bool) {
	s.mu.Lock()
	st := s.bd
	s.mu.Unlock()
	if st == nil {
		return nil, false
	}
	b, _ := st.host.PNG()
	return b, true
}

// closeBDJ ends Java with the session and removes its working files.
func (s *Session) closeBDJ() {
	s.mu.Lock()
	st := s.bd
	s.bd = nil
	s.mu.Unlock()
	if st != nil {
		st.host.Close()
	}
	_ = os.RemoveAll(filepath.Join(s.m.CacheDir, "bdj", "sessions", s.ID))
}

// JavaStatus reports whether this machine can run BD-J, for the settings page.
func JavaStatus(ctx context.Context, cacheDir, path string) (version, where string, err error) {
	j, err := bdj.Find(path)
	if err != nil {
		return "", "", err
	}
	v, err := j.Probe(ctx, filepath.Join(cacheDir, "bdj"))
	return v, j.Path, err
}
