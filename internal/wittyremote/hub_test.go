// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyremote

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *clock) pass(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func newHub(change func(*Limits)) (*Hub, *clock) {
	c := &clock{now: time.Unix(1_800_000_000, 0)}
	l := DefaultLimits()
	l.AckTimeout = 200 * time.Millisecond
	if change != nil {
		change(&l)
	}
	return New(l, c.Now), c
}

var tv = Capabilities{MediaKinds: []string{"video", "audio"}, Features: []string{"seek", "volume", "queue"}}

func announce(t *testing.T, h *Hub, user int64, session string) Device {
	t.Helper()
	d, err := h.Announce(Announce{UserID: user, UserName: "someone", SessionID: session, V: 1, Name: "Living room", Kind: "tv", Caps: tv})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func code(err error) string {
	var ce *CommandError
	if errors.As(err, &ce) {
		return ce.Code
	}
	if err == nil {
		return ""
	}
	return "error: " + err.Error()
}

func f(v float64) *float64 { return &v }
func s(v string) *string   { return &v }

func TestAnnounceNegotiates(t *testing.T) {
	h, _ := newHub(nil)
	d, err := h.Announce(Announce{UserID: 1, SessionID: "a", V: 7, Name: "  Kitchen\x00 tablet  ", Kind: "fridge",
		Caps: Capabilities{MediaKinds: []string{"video", "hologram"}, Features: []string{"seek", "smell", "rate"}}})
	if err != nil {
		t.Fatal(err)
	}
	// A newer client is spoken to in the server's version; what the server doesn't know is dropped.
	if d.V != Version || d.Kind != "other" || d.Name != "Kitchen tablet" || d.Online {
		t.Fatalf("%+v", d)
	}
	if strings.Join(d.Caps.MediaKinds, ",") != "video" || strings.Join(d.Caps.Features, ",") != "seek,rate" {
		t.Fatalf("%+v", d.Caps)
	}
	if _, err := h.Announce(Announce{UserID: 1, SessionID: "a", V: -1}); !errors.Is(err, ErrVersion) {
		t.Fatal(err)
	}
	// Announcing again with its id is the same device; with someone else's id, a new one.
	again, _ := h.Announce(Announce{UserID: 1, SessionID: "a", DeviceID: d.ID, Name: "Renamed", Kind: "tablet"})
	if again.ID != d.ID || again.Name != "Renamed" {
		t.Fatalf("%+v", again)
	}
	thief, _ := h.Announce(Announce{UserID: 2, SessionID: "b", DeviceID: d.ID, Name: "Mine now"})
	if thief.ID == d.ID {
		t.Fatal("another account took over a device by naming its id")
	}
	if got, _ := h.Device(d.ID); got.UserID != 1 || got.Name != "Renamed" {
		t.Fatalf("%+v", got)
	}
}

func TestPresenceExpires(t *testing.T) {
	h, c := newHub(nil)
	never := announce(t, h, 1, "a")
	d := announce(t, h, 1, "b")
	conn, _, err := h.Attach(d.ID, 1, "b")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := h.Device(d.ID); !got.Online {
		t.Fatal("attached, and not online")
	}
	// One that never opens its channel is forgotten.
	c.pass(31 * time.Second)
	_ = h.Report(d.ID, 1, "b", State{})
	if _, ok := h.Device(never.ID); ok {
		t.Fatal("a device that never opened its channel is still listed")
	}
	// The channel drops: offline at once, gone after the grace.
	h.Detach(d.ID, conn)
	if got, ok := h.Device(d.ID); !ok || got.Online {
		t.Fatalf("%+v %v", got, ok)
	}
	if err := h.Send(context.Background(), d.ID, 1, Command{Type: CmdPause}); code(err) != CodeOffline {
		t.Fatal(err)
	}
	// It may come back as itself within the grace.
	c.pass(20 * time.Second)
	if _, _, err := h.Attach(d.ID, 1, "b"); err != nil {
		t.Fatal(err)
	}
	// Open but silent: gone once it has missed its heartbeats.
	c.pass(76 * time.Second)
	if _, ok := h.Device(d.ID); ok {
		t.Fatal("a silent device is still listed")
	}
	if n, _, _ := h.Counts(); n != 0 {
		t.Fatal(n)
	}
}

func TestOnlyItsOwnSignInSpeaksForADevice(t *testing.T) {
	h, _ := newHub(nil)
	d := announce(t, h, 1, "a")
	for name, err := range map[string]error{
		"attach, other account":  third(h.Attach(d.ID, 2, "a")),
		"attach, other sign-in":  third(h.Attach(d.ID, 1, "b")),
		"report, other account":  h.Report(d.ID, 2, "a", State{}),
		"ack, other sign-in":     h.Ack(d.ID, 1, "b", "x", true, ""),
		"remove, other account":  h.Remove(d.ID, 2, "a"),
		"remove, other sign-in":  h.Remove(d.ID, 1, "b"),
		"report, no such device": h.Report("nope", 1, "a", State{}),
	} {
		if !errors.Is(err, ErrNotYours) && !errors.Is(err, ErrNoDevice) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := h.Remove(d.ID, 1, "a"); err != nil {
		t.Fatal(err)
	}
}

func third(_ *Conn, _ Device, err error) error { return err }

func TestCommandsAreDeliveredAndAnswered(t *testing.T) {
	h, _ := newHub(func(l *Limits) { l.AckTimeout = 5 * time.Second })
	d := announce(t, h, 1, "a")
	conn, _, _ := h.Attach(d.ID, 1, "a")
	// The device: does what it's told, except rewinding.
	go func() {
		for cmd := range conn.C {
			if cmd.Type == CmdSeek && cmd.BySec != nil && *cmd.BySec < 0 {
				_ = h.Ack(d.ID, 1, "a", cmd.ID, false, "This is a live programme.")
				continue
			}
			if cmd.ID == "" || cmd.V != 1 || cmd.SentAt == 0 {
				t.Errorf("%+v", cmd)
			}
			_ = h.Ack(d.ID, 1, "a", cmd.ID, true, "")
		}
	}()
	ctx := context.Background()
	if err := h.Send(ctx, d.ID, 1, Command{Type: CmdPlay, ItemID: 5}); err != nil {
		t.Fatal(err)
	}
	err := h.Send(ctx, d.ID, 1, Command{Type: CmdSeek, BySec: f(-10)})
	if code(err) != CodeRefused || err.Error() != "This is a live programme." {
		t.Fatal(err)
	}
	// It never said it could change sound tracks, so it is never asked to.
	if err := h.Send(ctx, d.ID, 1, Command{Type: CmdAudio, Audio: s("2")}); code(err) != CodeUnsupported {
		t.Fatal(err)
	}
	// The caller was allowed to control account 1's device; this id is no longer that.
	if err := h.Send(ctx, d.ID, 2, Command{Type: CmdPause}); code(err) != CodeOffline {
		t.Fatal(err)
	}
	if err := h.Send(ctx, "nope", 1, Command{Type: CmdPause}); code(err) != CodeOffline {
		t.Fatal(err)
	}
	if _, _, p := h.Counts(); p != 0 {
		t.Fatalf("%d commands left waiting", p)
	}
	if err := h.Ack(d.ID, 1, "a", "long-gone", true, ""); !errors.Is(err, ErrNoCommand) {
		t.Fatal(err)
	}
}

// Whatever happens to a command, whoever sent it is told, and promptly.
func TestACommandIsNeverLostSilently(t *testing.T) {
	h, _ := newHub(func(l *Limits) { l.QueueLen = 2 })
	d := announce(t, h, 1, "a")
	conn, _, _ := h.Attach(d.ID, 1, "a")
	ctx := context.Background()

	// The device reads nothing: two wait, and time out; the third is refused at once.
	results := make(chan error, 3)
	for range 2 {
		go func() { results <- h.Send(ctx, d.ID, 1, Command{Type: CmdPause}) }()
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(conn.C) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("the commands never reached the queue")
		}
		time.Sleep(time.Millisecond)
	}
	began := time.Now()
	if err := h.Send(ctx, d.ID, 1, Command{Type: CmdPause}); code(err) != CodeBusy {
		t.Fatal(err)
	}
	if time.Since(began) > 100*time.Millisecond {
		t.Fatal("a full queue made the caller wait")
	}
	for range 2 {
		if err := <-results; code(err) != CodeTimeout {
			t.Fatal(err)
		}
	}
	if _, _, p := h.Counts(); p != 0 {
		t.Fatalf("%d commands left waiting", p)
	}

	// The channel drops while one waits: it fails now, not at the timeout.
	h2, _ := newHub(func(l *Limits) { l.AckTimeout = time.Minute })
	d2 := announce(t, h2, 1, "a")
	conn2, _, _ := h2.Attach(d2.ID, 1, "a")
	go func() { results <- h2.Send(ctx, d2.ID, 1, Command{Type: CmdStop}) }()
	<-conn2.C
	began = time.Now()
	h2.Detach(d2.ID, conn2)
	if err := <-results; code(err) != CodeOffline || time.Since(began) > 5*time.Second {
		t.Fatal(err)
	}

	// The device reconnects while one waits: the old channel ends and the command fails.
	conn3, _, _ := h2.Attach(d2.ID, 1, "a")
	go func() { results <- h2.Send(ctx, d2.ID, 1, Command{Type: CmdStop}) }()
	<-conn3.C
	conn4, _, _ := h2.Attach(d2.ID, 1, "a")
	if err := <-results; code(err) != CodeOffline {
		t.Fatal(err)
	}
	<-conn3.Done()
	if conn3.Why() != "replaced" {
		t.Fatal(conn3.Why())
	}
	// The old channel's handler finishing doesn't take the new one down.
	h2.Detach(d2.ID, conn3)
	if got, _ := h2.Device(d2.ID); !got.Online {
		t.Fatal("the old channel ending put the device offline")
	}

	// The caller gives up: the command stops waiting.
	gone, cancel := context.WithCancel(ctx)
	go func() { results <- h2.Send(gone, d2.ID, 1, Command{Type: CmdStop}) }()
	<-conn4.C
	cancel()
	if err := <-results; code(err) != CodeTimeout {
		t.Fatal(err)
	}

	// The device is removed while one waits.
	go func() { results <- h2.Send(ctx, d2.ID, 1, Command{Type: CmdStop}) }()
	<-conn4.C
	h2.RemoveSession("a")
	if err := <-results; code(err) != CodeOffline {
		t.Fatal(err)
	}
	<-conn4.Done()
	if conn4.Why() != "signed-out" {
		t.Fatal(conn4.Why())
	}
}

func TestStateIsTidiedAndRateLimited(t *testing.T) {
	h, c := newHub(nil)
	d := announce(t, h, 1, "a")
	vol := 3.0
	long := strings.Repeat("x", 500)
	tracks := make([]Track, 100)
	for i := range tracks {
		tracks[i] = Track{ID: "t", Label: long}
	}
	err := h.Report(d.ID, 1, "a", State{Status: "playing", ItemID: 9, PositionSec: math.NaN(), DurationSec: math.Inf(1), Rate: -2,
		Volume: &vol, Audio: long, AudioTracks: tracks, Queue: []int64{9, -1}, Screen: long})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := h.Device(d.ID)
	st := got.State
	if st.PositionSec != 0 || math.IsInf(st.DurationSec, 0) || st.Rate != 1 || *st.Volume != 1 || len(st.Audio) != 64 ||
		len(st.AudioTracks) != MaxTracks || len(st.AudioTracks[0].Label) != 80 || st.Queue != nil || len(st.Screen) != 40 || got.StateAt == 0 {
		t.Fatalf("%+v", st)
	}
	// Idle is idle: no leftovers from what played before.
	c.pass(time.Second)
	_ = h.Report(d.ID, 1, "a", State{Status: "dancing", ItemID: 9, PositionSec: 50})
	if got, _ := h.Device(d.ID); got.State.Status != "idle" || got.State.ItemID != 0 || got.State.PositionSec != 0 {
		t.Fatalf("%+v", got.State)
	}
	// A burst is allowed, a flood is not, and it recovers.
	refused := 0
	for range 20 {
		if errors.Is(h.Report(d.ID, 1, "a", State{}), ErrRate) {
			refused++
		}
	}
	if refused < 10 || refused == 20 {
		t.Fatalf("%d of 20 refused", refused)
	}
	c.pass(time.Second)
	if err := h.Report(d.ID, 1, "a", State{}); err != nil {
		t.Fatal(err)
	}
}

func TestCommandRate(t *testing.T) {
	h, c := newHub(func(l *Limits) { l.CommandBurst = 3 })
	for i := range 3 {
		if !h.AllowCommand(1) {
			t.Fatal(i)
		}
	}
	if h.AllowCommand(1) {
		t.Fatal("a fourth in the same instant")
	}
	if !h.AllowCommand(2) {
		t.Fatal("one account's flood slowed another")
	}
	c.pass(time.Second)
	if !h.AllowCommand(1) {
		t.Fatal("it never recovered")
	}
}

func TestLimitsOnDevicesAndWatchers(t *testing.T) {
	h, _ := newHub(func(l *Limits) {
		l.MaxDevices, l.MaxDevicesPerUser, l.MaxSubscribers, l.MaxSubscribersPerUser = 3, 2, 3, 2
	})
	announce(t, h, 1, "a")
	announce(t, h, 1, "b")
	if _, err := h.Announce(Announce{UserID: 1, SessionID: "c"}); !errors.Is(err, ErrTooMany) {
		t.Fatal(err)
	}
	announce(t, h, 2, "d")
	if _, err := h.Announce(Announce{UserID: 3, SessionID: "e"}); !errors.Is(err, ErrTooMany) {
		t.Fatal(err)
	}
	a, _ := h.Subscribe(1)
	if _, err := h.Subscribe(1); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Subscribe(1); !errors.Is(err, ErrTooManySubs) {
		t.Fatal(err)
	}
	if _, err := h.Subscribe(2); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Subscribe(3); !errors.Is(err, ErrTooManySubs) {
		t.Fatal(err)
	}
	a.Close()
	a.Close() // twice is once
	if _, err := h.Subscribe(3); err != nil {
		t.Fatal(err)
	}
}

// A controller that isn't reading holds nobody up, and when it looks it sees the latest.
func TestASlowWatcherMissesStepsNotTheLatest(t *testing.T) {
	h, c := newHub(nil)
	sub, _ := h.Subscribe(1)
	defer sub.Close()
	d := announce(t, h, 1, "a")
	other := announce(t, h, 1, "b")
	for i := range 500 {
		c.pass(time.Second)
		if err := h.Report(d.ID, 1, "a", State{Status: "playing", ItemID: 1, PositionSec: float64(i)}); err != nil {
			t.Fatal(err)
		}
		_ = h.Report(other.ID, 1, "b", State{}) // keeps it from going quiet
	}
	select {
	case <-sub.C():
	default:
		t.Fatal("nothing to say after 500 changes")
	}
	ids := sub.Take()
	if len(ids) != 2 {
		t.Fatalf("500 changes to 2 devices came out as %d entries", len(ids))
	}
	if got, _ := h.Device(d.ID); got.State.PositionSec != 499 {
		t.Fatal(got.State.PositionSec)
	}
	if len(sub.Take()) != 0 {
		t.Fatal("the same changes twice")
	}
	// A device going is a change too.
	_ = h.Remove(other.ID, 1, "b")
	if ids := sub.Take(); len(ids) != 1 || ids[0] != other.ID {
		t.Fatal(ids)
	}
}

func TestCommandCheck(t *testing.T) {
	yes := true
	good := []Command{
		{Type: CmdPlay, ItemID: 4},
		{Type: CmdPlay, Queue: []int64{4, 5}, QueueIndex: 1, PositionSec: f(12.5), Audio: s("1"), Subtitle: s("off")},
		{Type: CmdPlay, ItemID: 5, Queue: []int64{4, 5}, QueueIndex: 1},
		{Type: CmdPause}, {Type: CmdResume}, {Type: CmdStop}, {Type: CmdNext}, {Type: CmdPrevious},
		{Type: CmdSeek, PositionSec: f(0)}, {Type: CmdSeek, BySec: f(-30)},
		{Type: CmdVolume, Level: f(0.4)}, {Type: CmdMute, Muted: &yes},
		{Type: CmdAudio, Audio: s("2")}, {Type: CmdSubtitle, Subtitle: s("off")},
		{Type: CmdRate, Rate: f(1.5)}, {Type: CmdNavigate, Key: "select"}, {Type: CmdCast, From: "abc"},
		{V: 1, Type: CmdPause},
	}
	for _, c := range good {
		if err := c.Check(); err != nil {
			t.Errorf("%+v: %v", c, err)
		}
	}
	bad := []Command{
		{}, {Type: "explode"}, {V: -3, Type: CmdPause},
		{Type: CmdPlay}, {Type: CmdPlay, ItemID: 4, QueueIndex: 2}, {Type: CmdPlay, Queue: []int64{4}, QueueIndex: 1},
		{Type: CmdPlay, Queue: []int64{4, 0}}, {Type: CmdPlay, ItemID: 9, Queue: []int64{4, 5}},
		{Type: CmdPlay, Queue: make([]int64, MaxQueueItems+1)},
		{Type: CmdPlay, ItemID: 4, PositionSec: f(-1)}, {Type: CmdPlay, ItemID: 4, PositionSec: f(math.NaN())},
		{Type: CmdSeek}, {Type: CmdSeek, PositionSec: f(1), BySec: f(1)}, {Type: CmdSeek, BySec: f(math.Inf(-1))},
		{Type: CmdVolume}, {Type: CmdVolume, Level: f(1.5)}, {Type: CmdMute},
		{Type: CmdAudio}, {Type: CmdAudio, Audio: s(strings.Repeat("a", 65))}, {Type: CmdSubtitle, Subtitle: s("")},
		{Type: CmdRate, Rate: f(0)}, {Type: CmdRate, Rate: f(100)},
		{Type: CmdNavigate}, {Type: CmdNavigate, Key: "eject"}, {Type: CmdCast},
	}
	for _, c := range bad {
		if c.Check() == nil {
			t.Errorf("accepted: %+v", c)
		}
	}
	for typ, want := range map[string]string{CmdSeek: "seek", CmdNext: "queue", CmdMute: "volume", CmdAudio: "audio", CmdSubtitle: "subtitles", CmdRate: "rate", CmdNavigate: "navigate", CmdPause: "", CmdStop: ""} {
		c := Command{Type: typ}
		if got := c.Needs(); got != want {
			t.Errorf("%s needs %q, want %q", typ, got, want)
		}
	}
	if (&Command{Type: CmdPlay, Queue: []int64{1, 2}}).Needs() != "queue" || (&Command{Type: CmdPlay, ItemID: 1}).Needs() != "" {
		t.Error("a queue of several needs the queue feature; one item doesn't")
	}
}

// Everything at once, for the race detector.
func TestHubUnderLoad(t *testing.T) {
	h, _ := newHub(func(l *Limits) { l.StateEvery, l.CommandEvery = 0, 0; l.AckTimeout = 5 * time.Second })
	const devices = 6
	var wg sync.WaitGroup
	ctx, stop := context.WithCancel(context.Background())
	var ids []string
	for i := range devices {
		session := string(rune('a' + i))
		d := announce(t, h, int64(i%2+1), session)
		ids = append(ids, d.ID)
		conn, _, _ := h.Attach(d.ID, d.UserID, session)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case cmd := <-conn.C:
					_ = h.Ack(d.ID, d.UserID, session, cmd.ID, true, "")
					_ = h.Report(d.ID, d.UserID, session, State{Status: "playing", ItemID: 1})
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	sub, _ := h.Subscribe(1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-sub.C():
				sub.Take()
				h.Devices()
			case <-ctx.Done():
				return
			}
		}
	}()
	var senders sync.WaitGroup
	for i := range devices {
		senders.Add(1)
		go func() {
			defer senders.Done()
			for range 40 {
				h.AllowCommand(int64(i))
				if err := h.Send(ctx, ids[i], int64(i%2+1), Command{Type: CmdPause}); err != nil {
					t.Error(err)
				}
				h.Sweep()
			}
		}()
	}
	senders.Wait()
	stop()
	wg.Wait()
	sub.Close()
	if _, subs, p := h.Counts(); subs != 0 || p != 0 {
		t.Fatalf("%d watchers and %d commands left", subs, p)
	}
}
