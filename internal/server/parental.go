// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/parental"
)

// Parental controls beyond which titles an account sees (that is
// library.Access, applied by each handler): what an account may do at all, and
// when and for how long it may play. Both are decided in one place, before any
// handler runs, from the route being called. So a handler can't forget them,
// and a route nobody classed fails a test (parental_routes_test.go).

// How a route counts towards screen time.
const (
	timeNone = iota
	// timeTick: the profile is watching (a progress report). It counts, and
	// is let through when time is up, so where they got to is still saved.
	timeTick
	// timePlay: it plays or starts something. Refused when time is up.
	timePlay
)

// routeClass is what parental controls need to know about a route.
type routeClass struct {
	feature string // the library.Feature it is part of, "" for none
	time    int
}

func (c routeClass) String() string {
	out := c.feature
	switch c.time {
	case timeTick:
		out += " tick"
	case timePlay:
		out += " play"
	}
	return strings.TrimSpace(out)
}

// classify says which feature a route belongs to and whether calling it is
// playing something. pattern is the route as it was registered, "GET /api/...".
func classify(pattern string) routeClass {
	method, path, _ := strings.Cut(pattern, " ")
	under := func(prefixes ...string) bool {
		for _, p := range prefixes {
			if path == p || strings.HasPrefix(path, p+"/") {
				return true
			}
		}
		return false
	}
	ends := func(suffixes ...string) bool {
		for _, x := range suffixes {
			if strings.HasSuffix(path, x) {
				return true
			}
		}
		return false
	}
	stops := method == "DELETE" // leaving a stream is always allowed
	var c routeClass
	switch {
	case under("/api/games", "/api/play", "/api/couch", "/api/rigs", "/api/modules/wittyplay", "/api/game-bezels"),
		strings.HasPrefix(path, "/api/items/") && (ends("/achievements") || strings.Contains(path, "/achievements/")):
		// A game's RetroAchievements belong to games, though they hang off the item.
		c.feature = library.FeatureGames
		// Under WittyPlay's own routes everything is a game being played, except an
		// emulator's settings, the list of one's games, and pausing one.
		emulator := under("/api/modules/wittyplay") && !ends("/settings", "/docker/sessions", "/pause")
		playing := under("/api/play", "/api/couch/sessions") || emulator ||
			(method == "POST" && ends("/play", "/play/emulator", "/prepare", "/start", "/stream", "/webrtc"))
		if playing && !stops {
			c.time = timePlay
		}
	case under("/api/recordings", "/api/recording-rules"):
		c.feature = library.FeatureRecordings
	case under("/api/syncplay/rooms", "/api/syncplay/join") && !stops && !ends("/leave"):
		// Watch parties. GET /api/syncplay itself stays open: it is how an app learns that
		// they are off for this profile. Starting or joining one is starting to watch, so it
		// is refused when time is up; what is watched then counts through the playback
		// routes each member's own player uses. Leaving and ending one are always allowed.
		c.feature = library.FeatureSyncplay
		if method == "POST" && (path == "/api/syncplay/rooms" || path == "/api/syncplay/join") {
			c.time = timePlay
		}
	case path == "/api/remote-control/devices" && method == "GET", path == "/api/remote-control/events",
		path == "/api/remote-control/devices/{id}/commands":
		// The controller's side of WittyRemote. A device's own side (announcing itself,
		// its channel, its state, its answers) is no feature: a profile kept from steering
		// other devices can still be played on by whoever may. Whether a command plays
		// something when time is up is decided where the command is read (checkPlay).
		c.feature = library.FeatureRemoteControl
	case path == "/api/shares" && method == "POST":
		c.feature = library.FeatureSharing
	case path == "/api/items/{id}/comments", under("/api/comments"):
		c.feature = library.FeatureComments
	case path == "/api/items/{id}/original":
		c.feature = library.FeatureDownloads
	case under("/api/hls", "/api/dvd"):
		if !stops {
			c.time = timePlay
		}
	case path == "/api/items/{id}/progress" && method == "POST", path == "/api/items/{id}/playback", path == "/api/items/{id}/view":
		c.time = timeTick
	case method == "GET" && under("/api/items/{id}/stream", "/api/items/{id}/file", "/api/items/{id}/play", "/api/cd/{id}/audio",
		"/api/radio/{id}/stream", "/api/radio/{id}/hls", "/api/tv/{key}/live", "/api/tv/{key}/tune", "/api/tv/{key}/catchup", "/api/media/{id}/canvas/file",
		"/api/media/{id}/stems/{stem}", "/api/music/extras/{id}/file"):
		c.time = timePlay
	case method == "POST" && under("/api/items/{id}/dvd", "/api/items/{id}/stream-token"):
		c.time = timePlay
	}
	return c
}

// What someone is told when a feature is off for their profile.
var featureRefusal = map[string]string{
	library.FeatureGames:         "Games are turned off for this profile.",
	library.FeatureRecordings:    "Recording is turned off for this profile.",
	library.FeatureSharing:       "Share links are turned off for this profile.",
	library.FeatureComments:      "Comments are turned off for this profile.",
	library.FeatureDownloads:     "Downloads are turned off for this profile.",
	library.FeatureRemote:        "This profile can only be used at home.",
	library.FeatureSyncplay:      "Watch parties are turned off for this profile.",
	library.FeatureRemoteControl: "Playing on other devices is turned off for this profile.",
}

// parentalState is what the server remembers between requests.
type parentalState struct {
	once    sync.Once
	tracker *parental.Tracker
	// now is the clock, which tests turn.
	now func() time.Time

	mu    sync.Mutex
	rules map[string]resolvedRules // an account's collections and tags, looked up lately
	seen  map[string]time.Time     // refusals written down lately, so one isn't written a hundred times
}

type resolvedRules struct {
	a     *library.Access
	until time.Time
}

const (
	rulesFor     = 15 * time.Second // how long a look-up of collections and tags is reused
	maxRules     = 256
	sameRefusal  = 10 * time.Minute // one refusal of one title is written down once in this long
	maxSeen      = 2048
	eventsKept   = 300 // per profile
	watchKeptFor = 400 // days of what a profile watched
)

func (s *Server) parentalInit() *parentalState {
	p := &s.parental
	p.once.Do(func() {
		p.tracker = &parental.Tracker{Store: screenStore{s.DB}, Now: func() time.Time { return s.clock() }}
	})
	return p
}

// clock is the time parental controls go by.
func (s *Server) clock() time.Time {
	if s.parental.now != nil {
		return s.parental.now()
	}
	return time.Now()
}

// accessOf is accessFor with the account's collections and tags looked up, so
// the limits can answer for one title. Every check of one title uses this.
func (s *Server) accessOf(ctx context.Context, u *auth.User) *library.Access {
	a := accessFor(u)
	if a == nil || a.Resolved() {
		return a
	}
	p := s.parentalInit()
	key := strconv.FormatInt(u.ID, 10) + "|" + u.Restrictions
	now := time.Now()
	p.mu.Lock()
	if e, ok := p.rules[key]; ok && now.Before(e.until) {
		p.mu.Unlock()
		return e.a
	}
	p.mu.Unlock()
	r, err := s.Store.Resolve(ctx, a)
	if err != nil {
		if ctx.Err() == nil {
			s.Log.WarnContext(ctx, "could not look up an account's limits by collection", "cat", "profiles", "account", u.ID, "err", err)
		}
		return a // unresolved: allows no single title
	}
	p.mu.Lock()
	if p.rules == nil || len(p.rules) >= maxRules {
		p.rules = map[string]resolvedRules{}
	}
	p.rules[key] = resolvedRules{r, now.Add(rulesFor)}
	p.mu.Unlock()
	return r
}

// forgetRules drops what was looked up, after limits or a collection changed.
func (s *Server) forgetRules() {
	s.parental.mu.Lock()
	s.parental.rules = nil
	s.parental.mu.Unlock()
	// A watch party asked what each member may see when they sat down. A collection that
	// limits go by has just changed, or someone's limits have, so it asks again.
	e := s.syncEngine()
	for _, id := range e.Seated() {
		e.Recheck(id)
	}
}

// remoteAllowed: an account kept to the home network isn't signed in anywhere else.
func remoteAllowed(r *http.Request, u *auth.User) bool {
	if clientFrom(r).Where.AtHome() {
		return true
	}
	return accessFor(u).Can(library.FeatureRemote)
}

// parentalGate decides, before a handler runs, whether this account may call
// this route now. It answers the request itself when it refuses. The writer it
// returns stops a long answer (a film sent in one piece) when time runs out.
func (s *Server) parentalGate(w http.ResponseWriter, r *http.Request, u *auth.User) (http.ResponseWriter, *http.Request, func(), bool) {
	a := accessFor(u)
	if a == nil {
		return w, r, nil, true
	}
	c := classify(r.Pattern)
	if mc, ok := r.Context().Value(classKey{}).(routeClass); ok {
		c = mc // a module's own route, classed by its integration.json (module_routes.go)
	}
	if c.feature != "" && !a.Can(c.feature) {
		s.noteRefused(r.Context(), u, "feature", 0, "", c.feature)
		writeJSON(w, 403, map[string]any{"error": featureRefusal[c.feature], "code": "feature-off", "feature": c.feature})
		return w, r, nil, false
	}
	if c.time == timeNone {
		return w, r, nil, true
	}
	p := s.parentalInit()
	if a.Time.Empty() {
		p.tracker.Tick(r.Context(), u.ID) // counted for the report, though nothing limits it
		return w, r, nil, true
	}
	v := a.Time.Check(s.clock(), p.tracker.Usage(r.Context(), u.ID))
	if !v.Allowed {
		w.Header().Set("X-WMS-Screen-Time-Left", "0")
		if c.time == timePlay {
			s.noteRefused(r.Context(), u, "time", 0, "", v.Reason)
			writeJSON(w, 403, map[string]any{"error": v.Message(), "code": "screen-time", "screenTime": screenTimeView(v)})
			return w, r, nil, false
		}
		return w, r, nil, true
	}
	p.tracker.Tick(r.Context(), u.ID)
	w.Header().Set("X-WMS-Screen-Time-Left", strconv.Itoa(v.RemainingSec))
	if c.time != timePlay || v.Until.IsZero() {
		return w, r, nil, true
	}
	// Something that goes on for a long time is stopped when the time runs out:
	// the answer stops being written, and a connection held open is told to end.
	ctx, cancel := context.WithCancel(r.Context())
	pw := &playWatch{s: s, userID: u.ID, name: u.Name, rules: a.Time, cancel: cancel}
	pw.until.Store(v.Until.UnixNano())
	pw.arm()
	return &timedWriter{ResponseWriter: w, watch: pw}, r.WithContext(ctx), pw.stop, true
}

// playWatch follows one request that is playing something for a profile with time rules.
type playWatch struct {
	s      *Server
	userID int64
	name   string // the profile's, for the log
	rules  *parental.TimeRules
	cancel context.CancelFunc
	until  atomic.Int64 // when to look again, in nanoseconds
	over   atomic.Bool
	ticked atomic.Int64

	mu    sync.Mutex
	timer *time.Timer
	done  bool
}

// arm sets the timer for the moment time runs out. A parent may have given
// more by then, so the timer asks again instead of stopping blindly.
func (p *playWatch) arm() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return
	}
	d := time.Until(time.Unix(0, p.until.Load()))
	if p.s.parental.now != nil { // a test's clock says how long, not ours
		d = time.Unix(0, p.until.Load()).Sub(p.s.clock())
	}
	p.timer = time.AfterFunc(max(d, 0)+time.Second, func() {
		if p.expired() {
			p.s.Log.Info("playback stopped: the profile's screen time ran out", "cat", "profiles", "profile", p.name)
			p.cancel()
			return
		}
		p.arm()
	})
}

// expired asks whether the profile's time is up now, and remembers a later moment if not.
func (p *playWatch) expired() bool {
	if p.over.Load() {
		return true
	}
	now := p.s.clock()
	if now.UnixNano() < p.until.Load() {
		return false
	}
	v := p.rules.Check(now, p.s.parentalInit().tracker.Usage(context.Background(), p.userID))
	if !v.Allowed || v.Until.IsZero() {
		if !v.Allowed {
			p.over.Store(true)
			return true
		}
		p.until.Store(now.Add(time.Hour).UnixNano())
		return false
	}
	p.until.Store(v.Until.UnixNano())
	return false
}

func (p *playWatch) stop() {
	p.mu.Lock()
	p.done = true
	if p.timer != nil {
		p.timer.Stop()
	}
	p.mu.Unlock()
	p.cancel()
}

var errScreenTime = errors.New("this profile's screen time is over")

// timedWriter stops writing when the profile's time is up, and counts the
// time the answer takes as watching.
type timedWriter struct {
	http.ResponseWriter
	watch *playWatch
}

func (t *timedWriter) Write(b []byte) (int, error) {
	now := t.watch.s.clock().UnixNano()
	if last := t.watch.ticked.Load(); now-last > int64(5*time.Second) && t.watch.ticked.CompareAndSwap(last, now) {
		if t.watch.expired() {
			return 0, errScreenTime
		}
		t.watch.s.parentalInit().tracker.Tick(context.Background(), t.watch.userID)
	} else if t.watch.over.Load() {
		return 0, errScreenTime
	}
	return t.ResponseWriter.Write(b)
}

func (t *timedWriter) Unwrap() http.ResponseWriter { return t.ResponseWriter }

func (t *timedWriter) Flush() {
	if f, ok := t.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (t *timedWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(t.ResponseWriter).Hijack()
}

// screenTimeView is a verdict as the API gives it, with its moments as Unix seconds.
func screenTimeView(v parental.Verdict) map[string]any {
	out := map[string]any{"allowed": v.Allowed, "reason": v.Reason, "allowanceSec": v.AllowanceSec, "usedSec": v.UsedSec,
		"extraSec": v.ExtraSec, "remainingSec": v.RemainingSec, "message": v.Message()}
	if !v.Until.IsZero() {
		out["until"] = v.Until.Unix()
	}
	if !v.NextAt.IsZero() {
		out["nextAt"] = v.NextAt.Unix()
	}
	return out
}

// noteRefused writes down that a profile with limits asked for something and
// was refused, for whoever manages it. kind is "blocked" (a title), "time" or
// "feature". The same refusal isn't written again for a while.
func (s *Server) noteRefused(ctx context.Context, u *auth.User, kind string, itemID int64, title, detail string) {
	if u == nil || u.IsAdmin {
		return
	}
	p := s.parentalInit()
	key := fmt.Sprintf("%d|%s|%d|%s", u.ID, kind, itemID, detail)
	now := time.Now()
	p.mu.Lock()
	if at, ok := p.seen[key]; ok && now.Sub(at) < sameRefusal {
		p.mu.Unlock()
		return
	}
	if p.seen == nil || len(p.seen) >= maxSeen {
		p.seen = map[string]time.Time{}
	}
	p.seen[key] = now
	p.mu.Unlock()
	ctx = context.WithoutCancel(ctx)
	// Once per refusal while it keeps being asked for, as it is written down.
	s.Log.InfoContext(ctx, "profile refused", "cat", "profiles", "profile", u.Name, "kind", kind, "detail", detail, "item", itemID)
	if len(title) > 200 {
		title = title[:200]
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO profile_events(user_id, at, kind, item_id, title, detail) VALUES(?,?,?,?,?,?)`, u.ID, s.clock().Unix(), kind, itemID, title, detail); err != nil {
		s.Log.WarnContext(ctx, "could not write down a refusal", "cat", "profiles", "err", err)
		return
	}
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM profile_events WHERE user_id = ? AND id <= (SELECT id FROM profile_events WHERE user_id = ? ORDER BY id DESC LIMIT 1 OFFSET ?)`, u.ID, u.ID, eventsKept)
}

// noteHidden is noteRefused for a title the account's limits hide.
func (s *Server) noteHidden(r *http.Request, it *library.Item, sh *library.Show) {
	rating := it.ContentRating
	title := it.Title
	if sh != nil {
		if rating == "" {
			rating = sh.ContentRating
		}
		title = sh.Title + ": " + title
	}
	s.noteRefused(r.Context(), userFrom(r), "blocked", it.ID, title, rating)
}

// screenStore keeps screen time in the database.
type screenStore struct {
	db interface {
		QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
		ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	}
}

func (st screenStore) Load(ctx context.Context, userID int64, day int) (parental.Usage, error) {
	var u parental.Usage
	var until int64
	err := st.db.QueryRowContext(ctx, `SELECT seconds, extra_seconds, extra_until FROM screen_time WHERE user_id = ? AND day = ?`, userID, day).Scan(&u.UsedSec, &u.ExtraSec, &until)
	if errors.Is(err, sql.ErrNoRows) {
		// The first look of a day: a good moment to drop what is too old to show.
		old := parental.DayNumber(time.Now().AddDate(0, 0, -watchKeptFor))
		_, _ = st.db.ExecContext(ctx, `DELETE FROM screen_time WHERE user_id = ? AND day < ?`, userID, old)
		_, _ = st.db.ExecContext(ctx, `DELETE FROM profile_watch WHERE user_id = ? AND day < ?`, userID, old)
		return u, nil
	}
	if until > 0 {
		u.ExtraUntil = time.Unix(until, 0)
	}
	return u, err
}

func (st screenStore) Add(ctx context.Context, userID int64, day int, seconds int) error {
	_, err := st.db.ExecContext(ctx, `INSERT INTO screen_time(user_id, day, seconds) VALUES(?,?,?)
		ON CONFLICT(user_id, day) DO UPDATE SET seconds = seconds + excluded.seconds`, userID, day, seconds)
	return err
}

// grantExtra gives a profile more time today: minutes more of its allowance,
// and that long past its hours from now.
func (s *Server) grantExtra(ctx context.Context, userID int64, minutes int, by string) error {
	now := s.clock()
	until := now.Add(time.Duration(minutes) * time.Minute).Unix()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO screen_time(user_id, day, extra_seconds, extra_until) VALUES(?,?,?,?)
		ON CONFLICT(user_id, day) DO UPDATE SET extra_seconds = extra_seconds + excluded.extra_seconds, extra_until = MAX(extra_until, excluded.extra_until)`,
		userID, parental.DayNumber(now), minutes*60, until)
	if err != nil {
		return err
	}
	s.parentalInit().tracker.Forget(ctx, userID)
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO profile_events(user_id, at, kind, title, detail) VALUES(?,?,?,?,?)`, userID, now.Unix(), "extra", by, strconv.Itoa(minutes))
	s.limitsChanged(ctx, userID) // a watch party it was sitting out of has it back now
	return nil
}
