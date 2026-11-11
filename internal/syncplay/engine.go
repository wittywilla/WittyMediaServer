// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package syncplay is watch parties: people in different places watching the same thing
// at the same moment. A room holds one authoritative state ("item 12 was at 61.5 seconds
// at server time T, playing at normal speed"), and every member's player works out from
// that where it should be now. Nobody's player is the leader, so nobody's bad connection
// drags the others about.
//
// The engine is plain data behind one lock. It doesn't know about HTTP or the library and
// never reads the real clock: the server hands it a clock, a way to ask whether an account
// may see an item, and calls Tick about once a second while rooms exist. That is what
// makes every rule here something a test can walk through second by second.
// docs/SYNCPLAY.md describes it for people writing clients.
package syncplay

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// Config is the engine's rules and its view of the world outside.
type Config struct {
	// Now is the clock. Nil: the real one.
	Now func() time.Time
	// CanSee says whether an account may see an item: its library and rating limits, asked
	// fresh each time. Nil: everyone sees everything (tests).
	CanSee func(userID, itemID int64) bool
	// NewID and NewCode make room and member ids, and the short codes people type.
	NewID   func() string
	NewCode func() string

	MaxRooms        int
	MaxRoomsPerUser int // rooms one account may host at once
	MaxMembers      int // in one room
	MaxQueue        int
	MaxInvites      int

	// MinLead and MaxLead bound how far in the future a start is set, so that the news of
	// it reaches everyone first. Between them it follows the slowest member's round trip.
	MinLead, MaxLead time.Duration
	// WaitTimeout: how long the room waits for members who are still loading before
	// going on without them. WaitCooldown: how long a member the room stopped waiting for
	// can't make it wait again.
	WaitTimeout, WaitCooldown time.Duration
	// AwayAfter: a member not heard from for this long doesn't hold the room up.
	// SeatGrace: after this long their seat is given up. EmptyClose: a room with nobody in
	// it closes after this. MaxAge: and any room after this.
	AwayAfter, SeatGrace, EmptyClose, MaxAge time.Duration
}

// DefaultConfig suits a home server.
func DefaultConfig() Config {
	return Config{
		MaxRooms: 32, MaxRoomsPerUser: 2, MaxMembers: 16, MaxQueue: 200, MaxInvites: 32,
		MinLead: 300 * time.Millisecond, MaxLead: 2500 * time.Millisecond,
		WaitTimeout: 15 * time.Second, WaitCooldown: time.Minute,
		AwayAfter: 20 * time.Second, SeatGrace: 2 * time.Minute, EmptyClose: 5 * time.Minute, MaxAge: 24 * time.Hour,
	}
}

// Who may play, pause and seek.
const (
	PolicyHost     = "host"
	PolicyEveryone = "everyone"
)

// Commands.
const (
	CmdPlay     = "play"
	CmdPause    = "pause"
	CmdSeek     = "seek"
	CmdNext     = "next"
	CmdPrevious = "previous"
	CmdSelect   = "select" // a place in the queue
	CmdRate     = "rate"
	CmdQueue    = "queue" // a new queue
)

// What a member's player says about itself.
const (
	ReportReady     = "ready"     // loaded at the room's position, able to play
	ReportBuffering = "buffering" // stalled, or still loading
)

// How a member appears to the others.
const (
	MemberReady     = "ready"
	MemberLoading   = "loading"   // hasn't said it's ready since the last change
	MemberBuffering = "buffering" // said it stalled
	MemberLagging   = "lagging"   // the room stopped waiting for it
	MemberAway      = "away"      // not heard from lately; its seat is kept for a while
	MemberBlocked   = "blocked"   // may not see what's playing
)

var (
	ErrNoRoom      = errors.New("no such watch party")
	ErrNotMember   = errors.New("you're not in that watch party")
	ErrNotHost     = errors.New("only the host can do that")
	ErrNotAllowed  = errors.New("only the host controls this watch party")
	ErrCantSee     = errors.New("this watch party is playing something this account can't watch")
	ErrFull        = errors.New("that watch party is full")
	ErrTooMany     = errors.New("too many watch parties are open already")
	ErrBadQueue    = errors.New("a watch party needs between one and the allowed number of things to watch")
	ErrEndOfQueue  = errors.New("there's nothing more in the queue that way")
	ErrBadCommand  = errors.New("that isn't a command a watch party knows")
	ErrBadValue    = errors.New("that value doesn't make sense")
	ErrStale       = errors.New("someone else changed the watch party first")
	ErrNoMember    = errors.New("no such member")
	ErrTooManyInvs = errors.New("too many people are invited already")
)

// User is who is acting: an account, and the device it is on. Seat is what makes a seat
// theirs when they come back: the same account from the same sign-in gets the same seat.
type User struct {
	ID   int64
	Name string
	Seat string
}

type member struct {
	id       string
	user     User
	joined   time.Time
	seen     time.Time
	ready    bool
	stalled  bool // said "buffering" rather than merely not yet "ready"
	lagging  bool
	away     bool // its stream closed; cleared when it is heard from
	blocked  bool
	cooldown time.Time
	rttMs    float64
	sees     []bool // by queue place: may this account see it
	sub      chan struct{}
}

type room struct {
	id, code, name string
	policy         string
	waitForAll     bool
	host           string
	members        map[string]*member
	invited        map[int64]bool
	queue          []int64
	index          int
	created        time.Time
	emptySince     time.Time

	// The state. While playing, the position at time t is pos + (t - at) * rate, and before
	// `at` it is pos: a start is set a little ahead so everyone begins together.
	pos       float64
	at        time.Time
	paused    bool
	waiting   bool
	waitSince time.Time
	rate      float64
	rev       uint64 // goes up with every change anyone can see
	seq       uint64 // goes up with every command, for "I was looking at seq N when I pressed"
	loadRev   uint64 // a "ready" for an older rev than this is about something that has since changed
	closed    string
}

// Engine holds the rooms. They live in memory only: a restart ends every watch party,
// which is right for something that lasts an evening and whose members' players would
// all have lost the server anyway.
type Engine struct {
	cfg    Config
	mu     sync.Mutex
	rooms  map[string]*room
	byCode map[string]string
}

// New makes an engine. Missing parts of cfg take DefaultConfig's.
func New(cfg Config) *Engine {
	d := DefaultConfig()
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.NewID == nil {
		cfg.NewID = randomID
	}
	if cfg.NewCode == nil {
		cfg.NewCode = randomCode
	}
	pick := func(v *int, def int) {
		if *v <= 0 {
			*v = def
		}
	}
	pickD := func(v *time.Duration, def time.Duration) {
		if *v <= 0 {
			*v = def
		}
	}
	pick(&cfg.MaxRooms, d.MaxRooms)
	pick(&cfg.MaxRoomsPerUser, d.MaxRoomsPerUser)
	pick(&cfg.MaxMembers, d.MaxMembers)
	pick(&cfg.MaxQueue, d.MaxQueue)
	pick(&cfg.MaxInvites, d.MaxInvites)
	pickD(&cfg.MinLead, d.MinLead)
	pickD(&cfg.MaxLead, d.MaxLead)
	pickD(&cfg.WaitTimeout, d.WaitTimeout)
	pickD(&cfg.WaitCooldown, d.WaitCooldown)
	pickD(&cfg.AwayAfter, d.AwayAfter)
	pickD(&cfg.SeatGrace, d.SeatGrace)
	pickD(&cfg.EmptyClose, d.EmptyClose)
	pickD(&cfg.MaxAge, d.MaxAge)
	return &Engine{cfg: cfg, rooms: map[string]*room{}, byCode: map[string]string{}}
}

// Config is the rules in force.
func (e *Engine) Config() Config { return e.cfg }

func randomID() string {
	var b [10]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// codeLetters leaves out what is misread aloud or on a TV across the room: 0 and O, 1, I
// and L.
const codeLetters = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// CodeLen is how long a room's code is: 31^6 is about 900 million, and tries are counted.
const CodeLen = 6

func randomCode() string {
	var b [CodeLen]byte
	_, _ = rand.Read(b[:])
	for i := range b {
		// 31 letters from a byte: the few values past the last whole run are folded in, which
		// tilts the odds by under 1%: nothing a code that is also rate-limited minds.
		b[i] = codeLetters[int(b[i])%len(codeLetters)]
	}
	return string(b[:])
}

// NormalCode is a code as people type it, made comparable: case, spaces and dashes don't
// matter.
func NormalCode(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' {
			return -1
		}
		return r
	}, s)
}

// ---------------------------------------------------------------------------
// The state
// ---------------------------------------------------------------------------

func (r *room) playing() bool { return !r.paused && !r.waiting }

// posAt is where the room is at a moment.
func (r *room) posAt(now time.Time) float64 {
	if !r.playing() || !now.After(r.at) {
		return r.pos
	}
	return r.pos + now.Sub(r.at).Seconds()*r.rate
}

// freeze pins the position to where it is now.
func (r *room) freeze(now time.Time) { r.pos, r.at = r.posAt(now), now }

// counts: whether the room waits for this member.
func (m *member) counts() bool { return !m.away && !m.blocked && !m.lagging }

func (r *room) gateOpen() bool {
	for _, m := range r.members {
		if m.counts() && !m.ready {
			return false
		}
	}
	return true
}

func (e *Engine) lead(r *room) time.Duration {
	worst := 0.0
	for _, m := range r.members {
		if m.counts() && m.rttMs > worst {
			worst = m.rttMs
		}
	}
	d := time.Duration(worst)*time.Millisecond + 200*time.Millisecond
	return min(max(d, e.cfg.MinLead), e.cfg.MaxLead)
}

// resume sets the room going, a moment from now. The position is already pinned.
func (e *Engine) resume(r *room, now time.Time) {
	r.waiting = false
	r.at = now.Add(e.lead(r))
}

// settle is run after anything that could open or shut the gate: it starts a room that
// was waiting for people who are now all ready, and holds one that should be playing
// while somebody it waits for isn't.
func (e *Engine) settle(r *room, now time.Time) {
	if r.paused {
		return
	}
	open := !r.waitForAll || r.gateOpen()
	switch {
	case r.waiting && open:
		e.resume(r, now)
		r.changed()
	case !r.waiting && !open:
		r.freeze(now)
		r.waiting, r.waitSince = true, now
		r.changed()
	}
}

// changed marks something everyone should hear about.
func (r *room) changed() {
	r.rev++
	for _, m := range r.members {
		if m.sub != nil {
			select {
			case m.sub <- struct{}{}:
			default:
			}
		}
	}
}

// reload is for a change every player must load again: a seek, another item. Everyone is
// "loading" until they say otherwise, and a "ready" still on its way about the old place
// doesn't count.
func (r *room) reload(now time.Time) {
	r.loadRev = r.rev + 1
	for _, m := range r.members {
		m.ready, m.stalled = false, false
		// A member the room gave up on gets another chance with each new thing to load,
		// unless it has only just been given up on.
		if m.lagging && !now.Before(m.cooldown) {
			m.lagging = false
		}
	}
}

func (e *Engine) seesAll(userID int64, queue []int64) []bool {
	out := make([]bool, len(queue))
	for i, id := range queue {
		out[i] = e.cfg.CanSee == nil || e.cfg.CanSee(userID, id)
	}
	return out
}

// reblock works out again who may see what is playing.
func (r *room) reblock() {
	for _, m := range r.members {
		m.blocked = r.index >= len(m.sees) || !m.sees[r.index]
	}
}

// ---------------------------------------------------------------------------
// What people see
// ---------------------------------------------------------------------------

// State is the room's state as sent to clients. Everything a player needs to know where
// to be is here: see PositionAt.
type State struct {
	// Rev goes up with every change. Seq goes up with every command; send it back as ifSeq
	// to have a command refused when someone else got in first.
	Rev uint64 `json:"rev"`
	Seq uint64 `json:"seq"`
	// ItemID is what's playing: 0 for a member who may not see it.
	ItemID int64 `json:"itemId"`
	Index  int   `json:"index"`
	// PositionSec was (or will be) the position at AtMs, in server milliseconds.
	PositionSec float64 `json:"positionSec"`
	AtMs        int64   `json:"atMs"`
	Paused      bool    `json:"paused"`
	// Waiting: the room wants to play, and is holding for members who are still loading.
	Waiting bool    `json:"waiting"`
	Rate    float64 `json:"rate"`
}

// PositionAt is where a player should be at a server time, in milliseconds.
func (s State) PositionAt(serverMs int64) float64 {
	if s.Paused || s.Waiting || serverMs <= s.AtMs {
		return s.PositionSec
	}
	return s.PositionSec + float64(serverMs-s.AtMs)/1000*s.Rate
}

// MemberView is one person in the room.
type MemberView struct {
	ID     string `json:"id"`
	UserID int64  `json:"userId"`
	Name   string `json:"name"`
	Host   bool   `json:"host"`
	You    bool   `json:"you,omitempty"`
	State  string `json:"state"`
	// RTTMs: their round trip to the server, as their last ping measured it.
	RTTMs int `json:"rttMs"`
}

// View is a room as one member sees it. What they may not see isn't in it.
type View struct {
	ID         string       `json:"id"`
	Code       string       `json:"code"`
	Name       string       `json:"name"`
	Policy     string       `json:"policy"`
	WaitForAll bool         `json:"waitForAll"`
	HostID     string       `json:"hostId"`
	You        string       `json:"you"`
	CanControl bool         `json:"canControl"`
	Blocked    bool         `json:"blocked"` // you may not see what's playing now
	Members    []MemberView `json:"members"`
	// Queue holds item ids, with 0 where this member may not see the item.
	Queue        []int64 `json:"queue"`
	State        State   `json:"state"`
	ServerTimeMs int64   `json:"serverTimeMs"`
	LeadMs       int64   `json:"leadMs"`
}

func (m *member) state() string {
	switch {
	case m.blocked:
		return MemberBlocked
	case m.away:
		return MemberAway
	case m.lagging:
		return MemberLagging
	case m.ready:
		return MemberReady
	case m.stalled:
		return MemberBuffering
	}
	return MemberLoading
}

func (r *room) mayControl(m *member) bool {
	return !m.blocked && (r.policy == PolicyEveryone || m.id == r.host)
}

func (e *Engine) view(r *room, me *member, now time.Time) View {
	v := View{ID: r.id, Code: r.code, Name: r.name, Policy: r.policy, WaitForAll: r.waitForAll, HostID: r.host, You: me.id,
		CanControl: r.mayControl(me), Blocked: me.blocked, ServerTimeMs: now.UnixMilli(), LeadMs: e.lead(r).Milliseconds(),
		State: State{Rev: r.rev, Seq: r.seq, Index: r.index, PositionSec: r.pos, AtMs: r.at.UnixMilli(), Paused: r.paused, Waiting: r.waiting, Rate: r.rate}}
	v.Queue = make([]int64, len(r.queue))
	for i, id := range r.queue {
		if i < len(me.sees) && me.sees[i] {
			v.Queue[i] = id
		}
	}
	if !me.blocked {
		v.State.ItemID = r.queue[r.index]
	}
	v.Members = make([]MemberView, 0, len(r.members))
	for _, m := range r.sorted() {
		v.Members = append(v.Members, MemberView{ID: m.id, UserID: m.user.ID, Name: m.user.Name, Host: m.id == r.host, You: m == me, State: m.state(), RTTMs: int(m.rttMs)})
	}
	return v
}

// sorted is the members in the order they sat down.
func (r *room) sorted() []*member {
	out := make([]*member, 0, len(r.members))
	for _, m := range r.members {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].joined.Equal(out[j].joined) {
			return out[i].joined.Before(out[j].joined)
		}
		return out[i].id < out[j].id
	})
	return out
}

// ---------------------------------------------------------------------------
// Rooms and seats
// ---------------------------------------------------------------------------

// Options are what a room is made with. Items is the queue; the first plays first.
type Options struct {
	Name        string
	Items       []int64
	Index       int
	Policy      string
	WaitForAll  *bool // nil: yes
	PositionSec float64
}

func cleanName(s, def string) string {
	s = strings.ToValidUTF8(strings.TrimSpace(s), "")
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	if r := []rune(s); len(r) > 60 {
		s = string(r[:60])
	}
	if s == "" {
		return def
	}
	return s
}

// Create opens a room with its maker as host. The room starts paused at the start (or
// where Options says), so the host can wait for people before pressing play. The maker
// must be able to see everything in the queue: a queue is theirs to choose, and choosing
// what they can't see would be a way to learn it exists.
func (e *Engine) Create(by User, o Options) (View, error) {
	now := e.cfg.Now()
	if len(o.Items) == 0 || len(o.Items) > e.cfg.MaxQueue || o.Index < 0 || o.Index >= len(o.Items) {
		return View{}, ErrBadQueue
	}
	if math.IsNaN(o.PositionSec) || math.IsInf(o.PositionSec, 0) || o.PositionSec < 0 {
		return View{}, ErrBadValue
	}
	for _, id := range o.Items {
		if id <= 0 {
			return View{}, ErrBadQueue
		}
	}
	sees := e.seesAll(by.ID, o.Items)
	if slices.Contains(sees, false) {
		return View{}, ErrCantSee
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	hosting := 0
	for _, r := range e.rooms {
		if h := r.members[r.host]; h != nil && h.user.ID == by.ID {
			hosting++
		}
	}
	if len(e.rooms) >= e.cfg.MaxRooms || hosting >= e.cfg.MaxRoomsPerUser {
		return View{}, ErrTooMany
	}
	r := &room{id: e.cfg.NewID(), name: cleanName(o.Name, cleanName(by.Name, "Someone")+"'s watch party"), policy: PolicyHost, waitForAll: true,
		members: map[string]*member{}, invited: map[int64]bool{}, queue: slices.Clone(o.Items), index: o.Index,
		created: now, pos: o.PositionSec, at: now, paused: true, rate: 1, rev: 1}
	if o.Policy == PolicyEveryone {
		r.policy = PolicyEveryone
	}
	if o.WaitForAll != nil {
		r.waitForAll = *o.WaitForAll
	}
	for range 8 { // a code nobody else has
		r.code = e.cfg.NewCode()
		if _, taken := e.byCode[r.code]; !taken {
			break
		}
		r.code = ""
	}
	if r.code == "" {
		return View{}, ErrTooMany
	}
	m := &member{id: e.cfg.NewID(), user: by, joined: now, seen: now, sees: sees}
	r.members[m.id], r.host = m, m.id
	e.rooms[r.id], e.byCode[r.code] = r, r.id
	return e.view(r, m, now), nil
}

// find is a room by its id or its code. With the lock held.
func (e *Engine) find(key string) *room {
	if r := e.rooms[key]; r != nil {
		return r
	}
	return e.rooms[e.byCode[NormalCode(key)]]
}

func (r *room) seat(by User) *member {
	for _, m := range r.members {
		if m.user.ID == by.ID && m.user.Seat == by.Seat {
			return m
		}
	}
	return nil
}

// Join seats someone. key is the room's code, or its id for someone invited or coming
// back to their seat. A person who may not see what is playing is refused, and told only
// that: not what it is.
//
// Coming back from the same sign-in within the grace period is the same seat, host's
// included. A late joiner doesn't hold a playing room up: it gets the state and catches
// up, and counts towards "wait for everyone" once it has said it's ready.
func (e *Engine) Join(by User, key string) (View, error) {
	now := e.cfg.Now()
	e.mu.Lock()
	r := e.find(key)
	if r == nil {
		e.mu.Unlock()
		return View{}, ErrNoRoom
	}
	if m := r.seat(by); m != nil {
		m.seen, m.user.Name = now, by.Name
		if m.away {
			m.away = false
			r.changed()
		}
		v := e.view(r, m, now)
		e.mu.Unlock()
		return v, nil
	}
	byCode := r.id != key
	if !byCode && !r.invited[by.ID] {
		e.mu.Unlock()
		return View{}, ErrNoRoom // an id alone opens nothing
	}
	queue, id := slices.Clone(r.queue), r.id
	e.mu.Unlock()

	// Asked without the lock: it reads the database.
	sees := e.seesAll(by.ID, queue)

	e.mu.Lock()
	defer e.mu.Unlock()
	r = e.rooms[id]
	if r == nil {
		return View{}, ErrNoRoom
	}
	if !slices.Equal(queue, r.queue) {
		sees = e.seesAll(by.ID, r.queue) // the queue changed meanwhile; rare enough to ask under the lock
	}
	if r.index >= len(sees) || !sees[r.index] {
		return View{}, ErrCantSee
	}
	if m := r.seat(by); m != nil { // two joins at once
		return e.view(r, m, now), nil
	}
	if len(r.members) >= e.cfg.MaxMembers {
		return View{}, ErrFull
	}
	m := &member{id: e.cfg.NewID(), user: by, joined: now, seen: now, sees: sees}
	// Arriving while it plays: catch up without stopping it for the others.
	m.lagging = r.playing()
	r.members[m.id] = m
	delete(r.invited, by.ID)
	r.emptySince = time.Time{}
	if r.members[r.host] == nil {
		r.host = m.id // an empty room's first arrival runs it
	}
	r.changed()
	e.settle(r, now)
	return e.view(r, m, now), nil
}

// member finds a seat and checks it is this account's. With the lock held.
func (e *Engine) member(roomID, memberID string, userID int64) (*room, *member, error) {
	r := e.rooms[roomID]
	if r == nil {
		return nil, nil, ErrNoRoom
	}
	m := r.members[memberID]
	if m == nil || m.user.ID != userID {
		return nil, nil, ErrNotMember
	}
	return r, m, nil
}

// heard notes that a member is here. With the lock held.
func (e *Engine) heard(r *room, m *member, now time.Time) {
	m.seen = now
	if m.away {
		m.away = false
		r.changed()
		e.settle(r, now)
	}
}

// View is the room as a member sees it now.
func (e *Engine) View(roomID, memberID string, userID int64) (View, error) {
	now := e.cfg.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	r, m, err := e.member(roomID, memberID, userID)
	if err != nil {
		return View{}, err
	}
	return e.view(r, m, now), nil
}

// removeMember takes a seat away and, if it was the host's, hands the room on to whoever
// has been there longest and is still around. With the lock held.
func (e *Engine) removeMember(r *room, m *member, now time.Time) {
	if m.sub != nil {
		close(m.sub)
		m.sub = nil
	}
	delete(r.members, m.id)
	if r.host == m.id {
		r.host = ""
		for _, c := range r.sorted() {
			if !c.away {
				r.host = c.id
				break
			}
		}
		if r.host == "" {
			if s := r.sorted(); len(s) > 0 {
				r.host = s[0].id
			}
		}
	}
	if len(r.members) == 0 {
		r.emptySince = now
		// Nobody is watching: it stops where it was, for whoever comes back.
		r.freeze(now)
		r.paused, r.waiting = true, false
	}
	r.changed()
	e.settle(r, now)
}

// Leave gives a seat up.
func (e *Engine) Leave(roomID, memberID string, userID int64) error {
	now := e.cfg.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	r, m, err := e.member(roomID, memberID, userID)
	if err != nil {
		return err
	}
	e.removeMember(r, m, now)
	return nil
}

func (e *Engine) closeRoom(r *room, why string) {
	r.closed = why
	for _, m := range r.members {
		if m.sub != nil {
			close(m.sub)
			m.sub = nil
		}
	}
	delete(e.rooms, r.id)
	delete(e.byCode, r.code)
}

// Close ends a room for everyone. The host may; so may an administrator, who passes
// admin and needs no seat.
func (e *Engine) Close(roomID, memberID string, userID int64, admin bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	r := e.rooms[roomID]
	if r == nil {
		return ErrNoRoom
	}
	if !admin {
		m := r.members[memberID]
		if m == nil || m.user.ID != userID {
			return ErrNotMember
		}
		if m.id != r.host {
			return ErrNotHost
		}
	}
	e.closeRoom(r, "closed")
	return nil
}

// Transfer makes another member the host.
func (e *Engine) Transfer(roomID, memberID string, userID int64, to string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, m, err := e.member(roomID, memberID, userID)
	if err != nil {
		return err
	}
	if m.id != r.host {
		return ErrNotHost
	}
	if r.members[to] == nil {
		return ErrNoMember
	}
	if r.host != to {
		r.host = to
		r.changed()
	}
	return nil
}

// Kick takes another member's seat away. They can come back with the code; a host who
// minds closes the room and opens another.
func (e *Engine) Kick(roomID, memberID string, userID int64, whom string) error {
	now := e.cfg.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	r, m, err := e.member(roomID, memberID, userID)
	if err != nil {
		return err
	}
	if m.id != r.host {
		return ErrNotHost
	}
	t := r.members[whom]
	if t == nil || t == m {
		return ErrNoMember
	}
	e.removeMember(r, t, now)
	return nil
}

// Settings are what the host may change about a running room. Nil leaves a thing alone.
type Settings struct {
	Name       *string
	Policy     *string
	WaitForAll *bool
}

// Set changes a room's settings.
func (e *Engine) Set(roomID, memberID string, userID int64, s Settings) error {
	now := e.cfg.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	r, m, err := e.member(roomID, memberID, userID)
	if err != nil {
		return err
	}
	if m.id != r.host {
		return ErrNotHost
	}
	if s.Policy != nil {
		if *s.Policy != PolicyHost && *s.Policy != PolicyEveryone {
			return ErrBadValue
		}
		r.policy = *s.Policy
	}
	if s.Name != nil {
		r.name = cleanName(*s.Name, r.name)
	}
	if s.WaitForAll != nil {
		r.waitForAll = *s.WaitForAll
	}
	r.changed()
	e.settle(r, now)
	return nil
}

// Invite lets accounts join by the room's id, from their own list of invitations, with
// no code to type. It doesn't let them see anything: joining still asks.
func (e *Engine) Invite(roomID, memberID string, userID int64, users []int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, m, err := e.member(roomID, memberID, userID)
	if err != nil {
		return err
	}
	if m.id != r.host {
		return ErrNotHost
	}
	for _, u := range users {
		if u <= 0 || r.invited[u] {
			continue
		}
		if len(r.invited) >= e.cfg.MaxInvites {
			return ErrTooManyInvs
		}
		r.invited[u] = true
	}
	return nil
}

// Listing is a room in someone's list: one they sit in, or are invited to. It names the
// room and its host, never what is playing.
type Listing struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	HostName string `json:"hostName"`
	Members  int    `json:"members"`
	// MemberID is their seat, when they have one from this sign-in.
	MemberID string `json:"memberId,omitempty"`
	Invited  bool   `json:"invited,omitempty"`
}

// RoomsFor lists an account's rooms and invitations.
func (e *Engine) RoomsFor(by User) []Listing {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := []Listing{}
	for _, r := range e.rooms {
		l := Listing{ID: r.id, Name: r.name, Members: len(r.members), Invited: r.invited[by.ID]}
		if h := r.members[r.host]; h != nil {
			l.HostName = h.user.Name
		}
		if m := r.seat(by); m != nil {
			l.MemberID = m.id
		}
		if l.MemberID != "" || l.Invited {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Rooms is how many are open.
func (e *Engine) Rooms() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.rooms)
}

// ---------------------------------------------------------------------------
// Commands, reports and time
// ---------------------------------------------------------------------------

// Command is play, pause, seek and the rest.
type Command struct {
	Type        string
	PositionSec float64 // seek
	Index       int     // select; and queue: where in the new queue to start
	Rate        float64 // rate
	Items       []int64 // queue
	// IfSeq, when not nil, refuses the command unless the room is still at that seq: the
	// way two people pressing "next" at the end of an episode skip one episode, not two.
	IfSeq *uint64
}

// Do applies a command to the room. Commands are taken one at a time, in the order they
// reach the server; that order is the truth, whatever order they were pressed in. A
// command that would change nothing (play while playing) is accepted and changes nothing.
func (e *Engine) Do(roomID, memberID string, userID int64, c Command) (View, error) {
	now := e.cfg.Now()
	var sees map[string][]bool
	if c.Type == CmdQueue {
		// A new queue is checked for everyone before the lock is taken: it reads the database.
		if len(c.Items) == 0 || len(c.Items) > e.cfg.MaxQueue || c.Index < 0 || c.Index >= len(c.Items) {
			return View{}, ErrBadQueue
		}
		for _, id := range c.Items {
			if id <= 0 {
				return View{}, ErrBadQueue
			}
		}
		if slices.Contains(e.seesAll(userID, c.Items), false) {
			return View{}, ErrCantSee
		}
		e.mu.Lock()
		users := map[string]int64{}
		if r := e.rooms[roomID]; r != nil {
			for id, m := range r.members {
				users[id] = m.user.ID
			}
		}
		e.mu.Unlock()
		sees = map[string][]bool{}
		for id, u := range users {
			sees[id] = e.seesAll(u, c.Items)
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	r, m, err := e.member(roomID, memberID, userID)
	if err != nil {
		return View{}, err
	}
	e.heard(r, m, now)
	if !r.mayControl(m) {
		if m.blocked {
			return View{}, ErrCantSee
		}
		return View{}, ErrNotAllowed
	}
	if c.IfSeq != nil && *c.IfSeq != r.seq {
		return e.view(r, m, now), ErrStale
	}
	bad := func(f float64) bool { return math.IsNaN(f) || math.IsInf(f, 0) }
	kept := false // a new queue that leaves what is playing as it is
	// move goes to another place in the queue, from its start.
	move := func(i int) error {
		if i < 0 || i >= len(r.queue) {
			return ErrEndOfQueue
		}
		r.index, r.pos, r.at = i, 0, now
		r.reblock()
		r.reload(now)
		return nil
	}
	switch c.Type {
	case CmdPlay:
		if !r.paused {
			return e.view(r, m, now), nil
		}
		r.paused = false
		if r.waitForAll && !r.gateOpen() {
			r.waiting, r.waitSince = true, now
		} else {
			e.resume(r, now)
		}
	case CmdPause:
		if r.paused {
			return e.view(r, m, now), nil
		}
		r.freeze(now)
		r.paused, r.waiting = true, false
	case CmdSeek:
		if bad(c.PositionSec) || c.PositionSec < 0 {
			return View{}, ErrBadValue
		}
		r.pos, r.at = c.PositionSec, now
		r.reload(now)
	case CmdNext:
		if err := move(r.index + 1); err != nil {
			return View{}, err
		}
	case CmdPrevious:
		if err := move(r.index - 1); err != nil {
			return View{}, err
		}
	case CmdSelect:
		if err := move(c.Index); err != nil {
			return View{}, err
		}
	case CmdRate:
		if bad(c.Rate) || c.Rate < 0.5 || c.Rate > 2 {
			return View{}, ErrBadValue
		}
		r.freeze(now)
		r.rate = c.Rate
	case CmdQueue:
		// A queue changed around what is playing (something added, taken out or moved, with
		// index still pointing at the same item) leaves it playing where it is: nobody loads again.
		kept = c.Items[c.Index] == r.queue[r.index]
		r.queue = slices.Clone(c.Items)
		for id, mm := range r.members {
			if s, ok := sees[id]; ok {
				mm.sees = s
			} else {
				mm.sees = make([]bool, len(r.queue)) // joined in the instant between: sees nothing until they rejoin
			}
		}
		if kept {
			r.index = c.Index
			r.reblock()
		} else {
			_ = move(c.Index)
		}
	default:
		return View{}, ErrBadCommand
	}
	// After a reload a playing room holds until its members have loaded, or starts again
	// a moment from now when it doesn't wait for them.
	if c.Type != CmdPlay && c.Type != CmdPause && c.Type != CmdRate && !kept && !r.paused {
		if r.waitForAll && !r.gateOpen() {
			r.waiting, r.waitSince = true, now
		} else {
			e.resume(r, now)
		}
	}
	r.seq++
	r.changed()
	return e.view(r, m, now), nil
}

// Report is a member's player saying how it is doing. rev is the room's rev it has
// caught up with: "ready" for anything older than the last seek or change of item is
// about a place the room has since left, and is ignored.
func (e *Engine) Report(roomID, memberID string, userID int64, what string, rev uint64) error {
	if what != ReportReady && what != ReportBuffering {
		return ErrBadValue
	}
	now := e.cfg.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	r, m, err := e.member(roomID, memberID, userID)
	if err != nil {
		return err
	}
	e.heard(r, m, now)
	was := m.state()
	switch what {
	case ReportReady:
		if rev < r.loadRev {
			return nil
		}
		m.ready, m.stalled, m.lagging = true, false, false
	case ReportBuffering:
		m.ready, m.stalled = false, true
		// Someone the room has just stopped waiting for doesn't get to stop it again
		// straight away: that is one bad connection freezing everyone, a little at a time.
		if now.Before(m.cooldown) {
			m.lagging = true
		}
	}
	if m.state() != was {
		r.changed()
	}
	e.settle(r, now)
	return nil
}

// Ping is a member's heartbeat, with the round trip its last ping measured (in ms; 0 or
// less when it hasn't one yet). The caller answers with the server's time; see ClockOffset.
func (e *Engine) Ping(roomID, memberID string, userID int64, rttMs float64) error {
	now := e.cfg.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	r, m, err := e.member(roomID, memberID, userID)
	if err != nil {
		return err
	}
	e.heard(r, m, now)
	if rttMs > 0 && !math.IsNaN(rttMs) {
		m.rttMs = min(rttMs, 10000)
	}
	return nil
}

// Listen is a member's stream opening. The channel says "look again" and closes when the
// seat or the room is gone; View then says which. One seat has one stream: a second one
// closes the first.
func (e *Engine) Listen(roomID, memberID string, userID int64) (<-chan struct{}, error) {
	now := e.cfg.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	r, m, err := e.member(roomID, memberID, userID)
	if err != nil {
		return nil, err
	}
	if m.sub != nil {
		close(m.sub)
	}
	m.sub = make(chan struct{}, 1)
	e.heard(r, m, now)
	return m.sub, nil
}

// Unlisten is that stream closing. The member is "away" at once, so the room doesn't
// wait for someone who has gone, but keeps the seat for SeatGrace.
func (e *Engine) Unlisten(roomID, memberID string, ch <-chan struct{}) {
	now := e.cfg.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	r := e.rooms[roomID]
	if r == nil {
		return
	}
	m := r.members[memberID]
	if m == nil || m.sub == nil || (<-chan struct{})(m.sub) != ch {
		return
	}
	m.sub = nil
	m.away = true
	r.changed()
	e.settle(r, now)
}

// Tick moves time on: it stops waiting for members who have taken too long, notices
// who has gone quiet, gives up seats nobody came back to, hands on a room whose host
// left, and closes rooms that are empty or too old. The server calls it about once a
// second while there are rooms.
func (e *Engine) Tick() {
	now := e.cfg.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range e.rooms {
		if now.Sub(r.created) > e.cfg.MaxAge {
			e.closeRoom(r, "expired")
			continue
		}
		if len(r.members) == 0 {
			if !r.emptySince.IsZero() && now.Sub(r.emptySince) > e.cfg.EmptyClose {
				e.closeRoom(r, "empty")
			}
			continue
		}
		for _, m := range r.sorted() {
			quiet := now.Sub(m.seen)
			switch {
			case quiet > e.cfg.SeatGrace:
				e.removeMember(r, m, now)
			case quiet > e.cfg.AwayAfter && !m.away:
				m.away = true
				r.changed()
			}
		}
		if len(r.members) == 0 {
			continue
		}
		// A host who has gone away for good is replaced when their seat goes, above. One who
		// is merely away keeps the room: they are probably walking between rooms.
		if r.waiting && now.Sub(r.waitSince) >= e.cfg.WaitTimeout {
			for _, m := range r.members {
				if m.counts() && !m.ready {
					m.lagging, m.cooldown = true, now.Add(e.cfg.WaitCooldown)
				}
			}
			r.changed()
		}
		e.settle(r, now)
	}
}
