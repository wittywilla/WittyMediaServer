package server

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Who is watching what, right now. Nothing here has a timer or a goroutine while nobody
// is watching anything. Each request for a piece of media costs a map lookup and adding up
// bytes, and a small goroutine tidies up for as long as anyone is playing something.

// A stream with no requests for this long has stopped: a player asks for pieces every few
// seconds, and a phone that has paused may take a while over it.
const streamGone = 60 * time.Second

// StreamView is someone playing something. Paths are never in here. Decision is direct,
// remux, transcode or what a playback answer said; Reasons say why, when it isn't direct.
type StreamView struct {
	ID          string   `json:"id"`
	UserID      int64    `json:"userId"`
	User        string   `json:"user"`
	Device      string   `json:"device,omitempty"`
	Where       string   `json:"where,omitempty"` // home, internet...
	ItemID      int64    `json:"itemId,omitempty"`
	Title       string   `json:"title,omitempty"`
	Decision    string   `json:"decision"`
	Reasons     []string `json:"reasons"`
	Width       int      `json:"width,omitempty"`
	Height      int      `json:"height,omitempty"`
	FileKbps    int64    `json:"fileKbps,omitempty"` // the file's own average
	SendingKbps int64    `json:"sendingKbps"`        // what's actually going out
	Bytes       int64    `json:"bytes"`
	StartedAt   int64    `json:"startedAt"`
}

// GameView is someone playing a game. Latency here is the server's part of it, the time to
// encode a frame: the network's part is only known to the player.
type GameView struct {
	ID            string  `json:"id"`
	UserID        int64   `json:"userId"`
	User          string  `json:"user"`
	Title         string  `json:"title"`
	Core          string  `json:"core,omitempty"`
	Encoder       string  `json:"encoder,omitempty"`
	GameFPS       float64 `json:"gameFps"`
	SentFPS       float64 `json:"sentFps"`
	EncodeMs      float64 `json:"encodeMs"`
	Kbps          float64 `json:"kbps"`
	DroppedFrames int64   `json:"droppedFrames"`
	Width         int     `json:"width,omitempty"`
	Height        int     `json:"height,omitempty"`
	Paused        bool    `json:"paused,omitempty"`
	StartedAt     int64   `json:"startedAt"`
}

type liveStream struct {
	view     StreamView // what doesn't change, guarded by the tracker's lock
	key      string
	bytes    atomic.Int64
	lastSeen atomic.Int64 // Unix seconds
	inFlight atomic.Int32 // requests being answered: one long download counts as going on
	task     *tasks.Handle

	rateAt    time.Time
	rateBytes int64
	rateKbps  int64
}

type liveGame struct {
	user    int64
	name    string
	title   string
	started int64
	stats   func() GameView
}

type streamTracker struct {
	s *Server

	mu      sync.Mutex
	live    map[string]*liveStream
	byHLS   map[string]*liveStream
	games   map[string]*liveGame
	tidying bool
	peak    int
	peakAt  int64
}

func (t *streamTracker) init(s *Server) {
	t.s = s
	t.live, t.byHLS, t.games = map[string]*liveStream{}, map[string]*liveStream{}, map[string]*liveGame{}
}

// hlsKey: two people can share one stream, and each is watching it for themselves.
func hlsKey(user int64, id string) string { return strconv.FormatInt(user, 10) + "|" + id }

func streamKey(user, item int64) string {
	return strconv.FormatInt(user, 10) + "/" + strconv.FormatInt(item, 10)
}

// note says a playback has started, with what the server decided about it.
func (t *streamTracker) note(r *http.Request, it *library.Item, decision string, reasons []string, hlsID string) {
	u := userFrom(r)
	if u == nil || it == nil {
		return
	}
	key := streamKey(u.ID, it.ID)
	v := StreamView{ID: logging.NewID("play"), UserID: u.ID, User: u.Name, ItemID: it.ID, Title: it.Title, Decision: decision, Reasons: append([]string{}, reasons...),
		Width: it.Width, Height: it.Height, StartedAt: time.Now().Unix(), Where: clientFrom(r).Where.String(), Device: t.deviceOf(r)}
	if it.RuntimeSec > 0 && it.SizeBytes > 0 {
		v.FileKbps = int64(float64(it.SizeBytes) * 8 / 1000 / float64(it.RuntimeSec))
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.live[key]
	if st == nil {
		st = &liveStream{key: key}
		t.live[key] = st
	} else {
		st.task.Finish(nil) // moved to another way of playing it, or started over
		st.task = nil
		v.ID, v.StartedAt = st.view.ID, st.view.StartedAt
	}
	st.view = v
	st.lastSeen.Store(time.Now().Unix())
	if hlsID != "" {
		t.byHLS[hlsKey(u.ID, hlsID)] = st
	}
	if decision == "transcode" {
		st.task = t.s.Tasks.Start(tasks.Spec{Type: tasks.TypeTranscode, Title: "Converting " + it.Title, By: byUser(u), Private: true, NoHistory: true})
	}
	t.startedLocked()
}

// deviceOf finds what the signed-in device calls itself. Once per playback, not per piece.
func (t *streamTracker) deviceOf(r *http.Request) string {
	u, tok := userFrom(r), tokenFrom(r)
	if u == nil || tok == "" {
		return ""
	}
	list, err := t.s.Auth.Sessions(r.Context(), u.ID, tok)
	if err != nil {
		return ""
	}
	for _, sess := range list {
		if sess.Current {
			return sess.Device
		}
	}
	return ""
}

func (t *streamTracker) startedLocked() {
	if n := len(t.live) + len(t.games); n > t.peak {
		t.peak, t.peakAt = n, time.Now().Unix()
	}
	if !t.tidying {
		t.tidying = true
		logging.Go("stream tracker", t.tidy, "cat", "playback")
	}
}

// tidy lets go of streams nobody is asking for, for as long as there are any.
func (t *streamTracker) tidy() {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for range tick.C {
		t.mu.Lock()
		cut := time.Now().Add(-streamGone).Unix()
		var ended []*liveStream
		for k, st := range t.live {
			if st.inFlight.Load() == 0 && st.lastSeen.Load() < cut {
				delete(t.live, k)
				for id, x := range t.byHLS {
					if x == st {
						delete(t.byHLS, id)
					}
				}
				st.task.Finish(nil)
				ended = append(ended, st)
			}
		}
		done := len(t.live) == 0 && len(t.games) == 0
		if done {
			t.tidying = false
		}
		t.mu.Unlock()
		for _, st := range ended {
			t.saveBytes(st)
		}
		if done {
			return
		}
	}
}

// saveBytes keeps how much a stream sent, counted per person per day.
func (t *streamTracker) saveBytes(st *liveStream) {
	n := st.bytes.Load()
	if n <= 0 {
		return
	}
	day := time.Now().Unix() / 86400
	_, err := t.s.DB.ExecContext(context.Background(), `INSERT INTO stream_bytes(day, user_id, bytes) VALUES (?, ?, ?)
		ON CONFLICT(day, user_id) DO UPDATE SET bytes = bytes + excluded.bytes`, day, st.view.UserID, n)
	if err != nil {
		t.s.Log.Debug("could not save how much was streamed", "cat", "playback", "err", err)
	}
}

// served counts what a request for a piece of media sends. Whatever it's answering may
// have started without a word to note: a file opened directly, a disc's stream.
func (t *streamTracker) served(w http.ResponseWriter, r *http.Request, itemID int64, hlsID string) (http.ResponseWriter, func()) {
	u := userFrom(r)
	if u == nil {
		return w, func() {}
	}
	key := streamKey(u.ID, itemID)
	if itemID == 0 {
		key = hlsKey(u.ID, hlsID)
	}
	t.mu.Lock()
	st := t.byHLS[hlsKey(u.ID, hlsID)]
	if hlsID == "" || st == nil {
		st = t.live[key]
	}
	if st == nil {
		st = &liveStream{key: key}
		st.view = StreamView{ID: logging.NewID("play"), UserID: u.ID, User: u.Name, ItemID: itemID, Decision: "direct", Reasons: []string{}, StartedAt: time.Now().Unix(), Where: clientFrom(r).Where.String()}
		if hlsID != "" {
			st.view.Decision = "transcode"
			if sess := t.s.HLS.Get(hlsID); sess != nil {
				st.view.Title = filepath.Base(sess.Spec.Source)
				if sess.Spec.CopyVideo {
					st.view.Decision = "remux"
				}
			}
			t.byHLS[hlsKey(u.ID, hlsID)] = st
		}
		t.live[key] = st
		t.startedLocked()
	}
	t.mu.Unlock()
	st.lastSeen.Store(time.Now().Unix())
	st.inFlight.Add(1)
	return &countingWriter{ResponseWriter: w, st: st}, func() {
		st.inFlight.Add(-1)
		st.lastSeen.Store(time.Now().Unix())
	}
}

type countingWriter struct {
	http.ResponseWriter
	st *liveStream
}

func (c *countingWriter) Write(b []byte) (int, error) {
	n, err := c.ResponseWriter.Write(b)
	c.st.bytes.Add(int64(n))
	return n, err
}

// ReadFrom keeps the fast way of sending a file, which is why it's here.
func (c *countingWriter) ReadFrom(r io.Reader) (int64, error) {
	n, err := io.Copy(c.ResponseWriter, r)
	c.st.bytes.Add(n)
	return n, err
}

func (c *countingWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// game notes a game session until it ends. stats asks it for its numbers, and only when
// someone wants them.
func (t *streamTracker) game(id string, user int64, name, title string, done <-chan struct{}, stats func() GameView) {
	t.mu.Lock()
	t.games[id] = &liveGame{user: user, name: name, title: title, started: time.Now().Unix(), stats: stats}
	t.startedLocked()
	t.mu.Unlock()
	logging.Go("game session end", func() {
		<-done
		t.mu.Lock()
		delete(t.games, id)
		t.mu.Unlock()
	}, "cat", "games")
}

// counts says how many of each are going on.
func (t *streamTracker) counts() (streams, transcodes, games int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, st := range t.live {
		streams++
		if st.view.Decision == "transcode" {
			transcodes++
		}
	}
	return streams, transcodes, len(t.games)
}

// streams lists who's playing what. only, if not zero, is the one account to list.
func (t *streamTracker) streams(only int64) []StreamView {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	out := []StreamView{}
	for _, st := range t.live {
		if only != 0 && st.view.UserID != only {
			continue
		}
		v := st.view
		b := st.bytes.Load()
		if dt := now.Sub(st.rateAt).Seconds(); !st.rateAt.IsZero() && dt >= 1 {
			st.rateKbps = int64(float64(b-st.rateBytes) * 8 / 1000 / dt)
			st.rateAt, st.rateBytes = now, b
		} else if st.rateAt.IsZero() {
			st.rateAt, st.rateBytes = now, b
		}
		v.Bytes, v.SendingKbps = b, st.rateKbps
		out = append(out, v)
	}
	return out
}

func (t *streamTracker) gameViews(only int64) []GameView {
	t.mu.Lock()
	list := make([]*liveGame, 0, len(t.games))
	ids := make([]string, 0, len(t.games))
	for id, g := range t.games {
		if only == 0 || g.user == only {
			list = append(list, g)
			ids = append(ids, id)
		}
	}
	t.mu.Unlock()
	out := []GameView{}
	for i, g := range list {
		v := g.stats()
		v.ID, v.UserID, v.User, v.Title, v.StartedAt = ids[i], g.user, g.name, g.title, g.started
		out = append(out, v)
	}
	return out
}

// liveBytes is what's been sent per account by streams still going, not yet saved.
func (t *streamTracker) liveBytes() map[int64]int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := map[int64]int64{}
	for _, st := range t.live {
		out[st.view.UserID] += st.bytes.Load()
	}
	return out
}

func (t *streamTracker) peakNow() (int, int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.peak, t.peakAt
}
