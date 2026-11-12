// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyremote

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
	"sort"
	"sync"
	"time"
)

// Limits bound what the hub holds, so that a Raspberry Pi with a house full of devices,
// or one misbehaving app, costs a known amount of memory.
type Limits struct {
	MaxDevices            int // every account's together
	MaxDevicesPerUser     int
	MaxSubscribers        int // controllers watching, every account's together
	MaxSubscribersPerUser int
	// QueueLen: commands waiting to be written to one device. One more fails as "busy".
	QueueLen int
	// StateEvery: a device's reports closer together than this are refused, apart from a
	// short burst. CommandEvery: the same for one account's commands.
	StateEvery   time.Duration
	StateBurst   int
	CommandEvery time.Duration
	CommandBurst int
	// AnnounceGrace: how long a device that announced itself may take to open its channel,
	// and how long after its channel dropped it may come back as the same device.
	AnnounceGrace time.Duration
	// IdleAfter: a device whose channel is open but which has reported nothing for this
	// long is taken to be gone. HeartbeatEvery is what it is told to keep to.
	IdleAfter      time.Duration
	HeartbeatEvery time.Duration
	// AckTimeout: how long a command waits for the device to answer.
	AckTimeout time.Duration
}

// DefaultLimits suit a home: a handful of people, a few devices each.
func DefaultLimits() Limits {
	return Limits{
		MaxDevices: 128, MaxDevicesPerUser: 16, MaxSubscribers: 64, MaxSubscribersPerUser: 8,
		QueueLen:   16,
		StateEvery: 500 * time.Millisecond, StateBurst: 6,
		CommandEvery: 200 * time.Millisecond, CommandBurst: 20,
		AnnounceGrace: 30 * time.Second, IdleAfter: 75 * time.Second, HeartbeatEvery: 30 * time.Second,
		AckTimeout: 8 * time.Second,
	}
}

// Why a command didn't happen. Code is for programs, Message for people.
type CommandError struct {
	Code    string
	Message string
}

func (e *CommandError) Error() string { return e.Message }

// Codes a CommandError carries.
const (
	CodeOffline     = "offline"     // the device isn't here (any more)
	CodeBusy        = "busy"        // too many commands are already waiting for it
	CodeTimeout     = "timeout"     // it didn't answer in time
	CodeUnsupported = "unsupported" // it never said it could do this
	CodeRefused     = "refused"     // it answered, and said no
)

var (
	ErrNoDevice    = errors.New("no such device")
	ErrNotYours    = errors.New("that device belongs to another sign-in")
	ErrTooMany     = errors.New("too many devices are announced already")
	ErrTooManySubs = errors.New("too many controllers are watching already")
	ErrVersion     = errors.New("this app speaks an older Remote Protocol than the server accepts: update the app")
	ErrRate        = errors.New("too many messages: slow down")
	ErrNoCommand   = errors.New("no such command is waiting")
)

// Announce is a client saying it can be controlled.
type Announce struct {
	UserID    int64
	UserName  string
	SessionID string // the sign-in it belongs to: auth.SessionID
	// DeviceID: the id it had before, to come back as the same device. "" for a new one.
	DeviceID string
	V        int
	Name     string
	Kind     string
	Caps     Capabilities
}

// Device is one controllable client, as anyone allowed to control it sees it.
type Device struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Kind     string       `json:"kind"`
	UserID   int64        `json:"userId"`
	UserName string       `json:"userName"`
	V        int          `json:"v"`
	Caps     Capabilities `json:"capabilities"`
	// Online: its channel is open, so a command can reach it.
	Online bool `json:"online"`
	// State is what it last reported, and StateAt when, in server milliseconds. A
	// controller works out where a playing device is now from the two.
	State   State `json:"state"`
	StateAt int64 `json:"stateAt"`

	SessionID string `json:"-"`
}

type bucket struct {
	tokens float64
	at     time.Time
}

// take spends one token of a bucket that refills one every `every`, up to burst.
func (b *bucket) take(now time.Time, every time.Duration, burst int) bool {
	if b.at.IsZero() {
		b.tokens = float64(burst)
	} else if every > 0 {
		b.tokens = min(float64(burst), b.tokens+float64(now.Sub(b.at))/float64(every))
	}
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

type device struct {
	Device
	conn     *Conn
	seen     time.Time // announced, attached or reported
	lostAt   time.Time // when its channel dropped; zero while open
	reports  bucket
	announce bucket
}

// Conn is a device's open channel: the server's handler reads commands from C and writes
// them to the client, until Done closes.
type Conn struct {
	C    chan Command
	done chan struct{}
	once sync.Once
	why  string
}

// Done closes when the channel should end: the device was removed, announced itself
// again elsewhere, or went quiet.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Why says what ended it, for the client's last message.
func (c *Conn) Why() string { return c.why }

func (c *Conn) end(why string) {
	c.once.Do(func() { c.why = why; close(c.done) })
}

type pending struct {
	device string
	done   chan *CommandError // buffered: the answer never blocks whoever gives it
}

// Sub is a controller watching devices come, go and change. It is told only that
// something changed and which devices; it then reads what they are now. A slow controller
// therefore misses the steps in between, never the latest, and never makes anyone wait.
type Sub struct {
	hub    *Hub
	userID int64
	wake   chan struct{}
	dirty  map[string]struct{}
}

// C says Take has something.
func (s *Sub) C() <-chan struct{} { return s.wake }

// Take returns the ids of devices that changed since the last Take.
func (s *Sub) Take() []string {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	out := make([]string, 0, len(s.dirty))
	for id := range s.dirty {
		out = append(out, id)
	}
	clear(s.dirty)
	sort.Strings(out)
	return out
}

// Close stops watching.
func (s *Sub) Close() {
	h := s.hub
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[s]; ok {
		delete(h.subs, s)
		if h.subsByUser[s.userID]--; h.subsByUser[s.userID] <= 0 {
			delete(h.subsByUser, s.userID)
		}
	}
}

// Hub is who is here. The zero value isn't usable: New makes one.
type Hub struct {
	Limits Limits
	// OnDrop, when set, is told each device the hub forgets and why: "expired" (it never
	// opened its channel), "idle" (it went quiet), "removed" or "signed-out". It is called
	// with the hub's lock held, so it must not call the hub.
	OnDrop func(d Device, why string)
	now    func() time.Time
	newID  func() string

	mu         sync.Mutex
	devices    map[string]*device
	byUser     map[int64]int
	subs       map[*Sub]struct{}
	subsByUser map[int64]int
	pending    map[string]*pending
	callers    map[int64]*bucket
}

// New makes a hub. now is the clock (nil: the real one), so tests can move time.
func New(l Limits, now func() time.Time) *Hub {
	if now == nil {
		now = time.Now
	}
	return &Hub{Limits: l, now: now, newID: randomID,
		devices: map[string]*device{}, byUser: map[int64]int{}, subs: map[*Sub]struct{}{}, subsByUser: map[int64]int{},
		pending: map[string]*pending{}, callers: map[int64]*bucket{}}
}

func randomID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// touched tells every watcher that a device changed. Called with the lock held.
func (h *Hub) touched(id string) {
	for s := range h.subs {
		s.dirty[id] = struct{}{}
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}

// failPending answers every command still waiting for a device. With the lock held.
func (h *Hub) failPending(id string, e *CommandError) {
	for cid, p := range h.pending {
		if p.device == id {
			delete(h.pending, cid)
			p.done <- e
		}
	}
}

func (h *Hub) drop(d *device, why string) {
	if d.conn != nil {
		d.conn.end(why)
		d.conn = nil
	}
	h.failPending(d.ID, &CommandError{CodeOffline, "That device has gone."})
	if h.OnDrop != nil {
		h.OnDrop(d.Device, why)
	}
	delete(h.devices, d.ID)
	if h.byUser[d.UserID]--; h.byUser[d.UserID] <= 0 {
		delete(h.byUser, d.UserID)
	}
	h.touched(d.ID)
}

// sweep forgets devices that never opened their channel, didn't come back, or went quiet.
// It runs inside everything else that takes the lock, so nothing needs a timer: a device
// nobody looks for costs nothing by staying a little longer.
func (h *Hub) sweep(now time.Time) {
	for _, d := range h.devices {
		switch {
		case d.conn == nil && now.Sub(d.seen) > h.Limits.AnnounceGrace:
			h.drop(d, "expired")
		case d.conn != nil && now.Sub(d.seen) > h.Limits.IdleAfter:
			h.drop(d, "idle")
		}
	}
	if len(h.callers) > 4*h.Limits.MaxDevices {
		for id, b := range h.callers {
			if now.Sub(b.at) > time.Minute {
				delete(h.callers, id)
			}
		}
	}
}

// Sweep is sweep for whoever keeps a channel open and so has a clock ticking anyway.
func (h *Hub) Sweep() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sweep(h.now())
}

// Announce registers a device, or brings one back under the id it had. The device then
// has AnnounceGrace to open its channel.
func (h *Hub) Announce(a Announce) (Device, error) {
	if a.V == 0 {
		a.V = Version
	}
	if a.V < MinVersion {
		return Device{}, ErrVersion
	}
	if a.V > Version {
		a.V = Version // a newer client speaks our version to us
	}
	if !slices.Contains(Kinds, a.Kind) {
		a.Kind = "other"
	}
	if a.Name = Clip(a.Name, MaxNameLen); a.Name == "" {
		a.Name = "A " + a.Kind + " device"
	}
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sweep(now)
	d := h.devices[a.DeviceID]
	if d != nil && (d.UserID != a.UserID || d.SessionID != a.SessionID) {
		d = nil // somebody else's id: this is a new device, not theirs
	}
	if d == nil {
		if len(h.devices) >= h.Limits.MaxDevices || h.byUser[a.UserID] >= h.Limits.MaxDevicesPerUser {
			return Device{}, ErrTooMany
		}
		d = &device{Device: Device{ID: h.newID(), UserID: a.UserID, SessionID: a.SessionID, State: State{V: Version, Status: "idle", Rate: 1}}}
		h.devices[d.ID] = d
		h.byUser[a.UserID]++
	} else if !d.announce.take(now, 2*time.Second, 5) {
		return Device{}, ErrRate
	}
	d.UserName, d.Name, d.Kind, d.V, d.Caps = a.UserName, a.Name, a.Kind, a.V, a.Caps.Negotiate()
	d.seen = now
	h.touched(d.ID)
	return d.Device, nil
}

// own finds a device for the sign-in that announced it. With the lock held.
func (h *Hub) own(id string, userID int64, session string) (*device, error) {
	d := h.devices[id]
	if d == nil {
		return nil, ErrNoDevice
	}
	if d.UserID != userID || d.SessionID != session {
		return nil, ErrNotYours
	}
	return d, nil
}

// Attach opens a device's channel. An earlier channel of the same device ends: the app
// reconnected, and one device has one channel.
func (h *Hub) Attach(id string, userID int64, session string) (*Conn, Device, error) {
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sweep(now)
	d, err := h.own(id, userID, session)
	if err != nil {
		return nil, Device{}, err
	}
	if d.conn != nil {
		d.conn.end("replaced")
		h.failPending(id, &CommandError{CodeOffline, "That device reconnected before it answered."})
	}
	d.conn = &Conn{C: make(chan Command, h.Limits.QueueLen), done: make(chan struct{})}
	d.Online, d.seen, d.lostAt = true, now, time.Time{}
	h.touched(id)
	return d.conn, d.Device, nil
}

// Detach is the channel's handler saying it has ended. The device is offline at once:
// nothing more can be sent to it, and whatever was waiting fails now, not after a timeout.
// It has AnnounceGrace to come back as the same device.
func (h *Hub) Detach(id string, c *Conn) {
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	c.end("closed")
	d := h.devices[id]
	if d == nil || d.conn != c {
		return
	}
	d.conn, d.Online, d.seen, d.lostAt = nil, false, now, now
	h.failPending(id, &CommandError{CodeOffline, "That device has gone."})
	h.touched(id)
}

// Remove is a device saying goodbye, or its sign-in ending.
func (h *Hub) Remove(id string, userID int64, session string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	d, err := h.own(id, userID, session)
	if err != nil {
		return err
	}
	h.drop(d, "removed")
	return nil
}

// RemoveSession forgets every device of a sign-in that has ended.
func (h *Hub) RemoveSession(session string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, d := range h.devices {
		if d.SessionID == session {
			h.drop(d, "signed-out")
		}
	}
}

// RemoveUser forgets every device of an account, except those of the sign-in keep ("" for
// none): "sign out everywhere else", a new password, an account deleted.
func (h *Hub) RemoveUser(userID int64, keep string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, d := range h.devices {
		if d.UserID == userID && (keep == "" || d.SessionID != keep) {
			h.drop(d, "signed-out")
		}
	}
}

// Report is a device saying what it is doing. It is also its heartbeat.
func (h *Hub) Report(id string, userID int64, session string, st State) error {
	st = st.Clean()
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	d, err := h.own(id, userID, session)
	if err != nil {
		return err
	}
	if !d.reports.take(now, h.Limits.StateEvery, h.Limits.StateBurst) {
		return ErrRate
	}
	d.seen = now
	d.State, d.StateAt = st, now.UnixMilli()
	h.touched(id)
	return nil
}

// Devices is everyone here, by name. The caller shows each account only what it may see.
func (h *Hub) Devices() []Device {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sweep(h.now())
	out := make([]Device, 0, len(h.devices))
	for _, d := range h.devices {
		out = append(out, d.Device)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Device is one device, if it is here.
func (h *Hub) Device(id string) (Device, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sweep(h.now())
	d := h.devices[id]
	if d == nil {
		return Device{}, false
	}
	return d.Device, true
}

// Subscribe starts watching for changes.
func (h *Hub) Subscribe(userID int64) (*Sub, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.subs) >= h.Limits.MaxSubscribers || h.subsByUser[userID] >= h.Limits.MaxSubscribersPerUser {
		return nil, ErrTooManySubs
	}
	s := &Sub{hub: h, userID: userID, wake: make(chan struct{}, 1), dirty: map[string]struct{}{}}
	h.subs[s] = struct{}{}
	h.subsByUser[userID]++
	return s, nil
}

// AllowCommand counts one command against an account's rate, and says whether it may go.
func (h *Hub) AllowCommand(userID int64) bool {
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sweep(now)
	b := h.callers[userID]
	if b == nil {
		b = &bucket{}
		h.callers[userID] = b
	}
	return b.take(now, h.Limits.CommandEvery, h.Limits.CommandBurst)
}

// Send gives a device a command and waits for its answer. It returns nil when the device
// said it did it, and a *CommandError otherwise. It never waits on a device that isn't
// here, and never leaves a command in a queue nobody is reading: a full queue is "busy",
// said at once. ownerID is whose device the caller was allowed to control; a device that
// has since become someone else's is "offline" to them.
func (h *Hub) Send(ctx context.Context, id string, ownerID int64, cmd Command) error {
	now := h.now()
	h.mu.Lock()
	h.sweep(now)
	d := h.devices[id]
	if d == nil || d.UserID != ownerID || d.conn == nil {
		h.mu.Unlock()
		return &CommandError{CodeOffline, "That device isn't connected."}
	}
	if need := cmd.Needs(); need != "" && !d.Caps.Has(need) {
		h.mu.Unlock()
		return &CommandError{CodeUnsupported, "That device can't do that."}
	}
	cmd.V, cmd.ID, cmd.SentAt = d.V, h.newID(), now.UnixMilli()
	p := &pending{device: id, done: make(chan *CommandError, 1)}
	select {
	case d.conn.C <- cmd:
		h.pending[cmd.ID] = p
	default:
		h.mu.Unlock()
		return &CommandError{CodeBusy, "That device has too many commands waiting. Try again in a moment."}
	}
	h.mu.Unlock()

	wait := time.NewTimer(h.Limits.AckTimeout)
	defer wait.Stop()
	var timeout *CommandError
	select {
	case e := <-p.done:
		if e == nil {
			return nil
		}
		return e
	case <-wait.C:
		timeout = &CommandError{CodeTimeout, "That device didn't answer."}
	case <-ctx.Done():
		timeout = &CommandError{CodeTimeout, "The request ended before that device answered."}
	}
	h.mu.Lock()
	delete(h.pending, cmd.ID)
	h.mu.Unlock()
	// It may have answered just as the wait ended.
	select {
	case e := <-p.done:
		if e == nil {
			return nil
		}
		return e
	default:
	}
	return timeout
}

// Ack is a device answering a command: done, or not and why.
func (h *Hub) Ack(id string, userID int64, session, commandID string, ok bool, why string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	d, err := h.own(id, userID, session)
	if err != nil {
		return err
	}
	d.seen = h.now()
	p := h.pending[commandID]
	if p == nil || p.device != id {
		return ErrNoCommand
	}
	delete(h.pending, commandID)
	if ok {
		p.done <- nil
		return nil
	}
	if why = Clip(why, 200); why == "" {
		why = "That device couldn't do that."
	}
	p.done <- &CommandError{CodeRefused, why}
	return nil
}

// Counts is how much the hub is holding, for tests and the statistics.
func (h *Hub) Counts() (devices, subs, pending int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.devices), len(h.subs), len(h.pending)
}
