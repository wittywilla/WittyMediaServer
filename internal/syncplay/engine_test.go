// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package syncplay

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

// world is an engine with a clock the test moves, ids that count up, and a list of what
// each account may not see.
type world struct {
	t      *testing.T
	e      *Engine
	mu     sync.Mutex
	now    time.Time
	n      int
	hidden map[int64]map[int64]bool // user -> items it may not see
}

func newWorld(t *testing.T, change func(*Config)) *world {
	w := &world{t: t, now: time.Unix(1_800_000_000, 0), hidden: map[int64]map[int64]bool{}}
	cfg := Config{
		Now: func() time.Time { w.mu.Lock(); defer w.mu.Unlock(); return w.now },
		NewID: func() string {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.n++
			return fmt.Sprintf("id%d", w.n)
		},
		NewCode: func() string {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.n++
			return fmt.Sprintf("CDE%03d", w.n)
		},
		CanSee: func(u, item int64) bool { w.mu.Lock(); defer w.mu.Unlock(); return !w.hidden[u][item] },
	}
	if change != nil {
		change(&cfg)
	}
	w.e = New(cfg)
	return w
}

func (w *world) pass(d time.Duration) {
	w.mu.Lock()
	w.now = w.now.Add(d)
	w.mu.Unlock()
}

func (w *world) ms() int64 { w.mu.Lock(); defer w.mu.Unlock(); return w.now.UnixMilli() }

func (w *world) hide(user, item int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.hidden[user] == nil {
		w.hidden[user] = map[int64]bool{}
	}
	w.hidden[user][item] = true
}

func person(id int64) User { return User{ID: id, Name: fmt.Sprintf("p%d", id), Seat: "s"} }

func (w *world) create(u User, items ...int64) View {
	w.t.Helper()
	v, err := w.e.Create(u, Options{Items: items})
	if err != nil {
		w.t.Fatal(err)
	}
	return v
}

func (w *world) join(u User, key string) View {
	w.t.Helper()
	v, err := w.e.Join(u, key)
	if err != nil {
		w.t.Fatal(err)
	}
	return v
}

func (w *world) do(v View, user int64, c Command) View {
	w.t.Helper()
	out, err := w.e.Do(v.ID, v.You, user, c)
	if err != nil {
		w.t.Fatal(err)
	}
	return out
}

func (w *world) ready(v View, user int64) {
	w.t.Helper()
	cur, err := w.e.View(v.ID, v.You, user)
	if err != nil {
		w.t.Fatal(err)
	}
	if err := w.e.Report(v.ID, v.You, user, ReportReady, cur.State.Rev); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) view(v View, user int64) View {
	w.t.Helper()
	out, err := w.e.View(v.ID, v.You, user)
	if err != nil {
		w.t.Fatal(err)
	}
	return out
}

func stateOf(v View, member string) string {
	for _, m := range v.Members {
		if m.ID == member {
			return m.State
		}
	}
	return "missing"
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestARoomStartsPausedAndPlaysALittleInTheFuture(t *testing.T) {
	w := newWorld(t, nil)
	host := w.create(person(1), 10)
	if !host.State.Paused || host.State.ItemID != 10 || host.Policy != PolicyHost || !host.WaitForAll || !host.CanControl {
		t.Fatalf("%+v", host)
	}
	guest := w.join(person(2), host.Code)
	if guest.State.ItemID != 10 || guest.CanControl || guest.HostID != host.You {
		t.Fatalf("%+v", guest)
	}
	w.ready(host, 1)
	w.ready(guest, 2)
	v := w.do(host, 1, Command{Type: CmdPlay})
	if v.State.Paused || v.State.Waiting {
		t.Fatalf("%+v", v.State)
	}
	// It starts MinLead from now: everyone hears of it before it happens.
	if lead := v.State.AtMs - w.ms(); lead != 300 {
		t.Fatalf("lead %d", lead)
	}
	if p := v.State.PositionAt(w.ms()); p != 0 {
		t.Fatalf("before the start it is at %v", p)
	}
	w.pass(10*time.Second + 300*time.Millisecond)
	if p := w.view(guest, 2).State.PositionAt(w.ms()); !near(p, 10) {
		t.Fatalf("after ten seconds it is at %v", p)
	}
	// Pausing pins it where the server says it is, not where the pauser's player was.
	p := w.do(host, 1, Command{Type: CmdPause})
	if !p.State.Paused || !near(p.State.PositionSec, 10) {
		t.Fatalf("%+v", p.State)
	}
	w.pass(time.Minute)
	if got := w.view(guest, 2).State.PositionAt(w.ms()); !near(got, 10) {
		t.Fatalf("paused, and it moved to %v", got)
	}
}

func TestTheLeadFollowsTheSlowestMember(t *testing.T) {
	w := newWorld(t, nil)
	host := w.create(person(1), 10)
	guest := w.join(person(2), host.Code)
	w.ready(host, 1)
	w.ready(guest, 2)
	for _, c := range []struct {
		rtt  float64
		want int64
	}{{40, 300}, {600, 800}, {9000, 2500}, {math.NaN(), 2500}} {
		if err := w.e.Ping(guest.ID, guest.You, 2, c.rtt); err != nil {
			t.Fatal(err)
		}
		w.do(host, 1, Command{Type: CmdPause})
		v := w.do(host, 1, Command{Type: CmdPlay})
		if got := v.State.AtMs - w.ms(); got != c.want {
			t.Errorf("rtt %v: lead %d, want %d", c.rtt, got, c.want)
		}
	}
}

func TestHostOnlyAndEveryone(t *testing.T) {
	w := newWorld(t, nil)
	host := w.create(person(1), 10)
	guest := w.join(person(2), host.Code)
	if _, err := w.e.Do(guest.ID, guest.You, 2, Command{Type: CmdPlay}); !errors.Is(err, ErrNotAllowed) {
		t.Fatal(err)
	}
	// Another account using the guest's seat id is nobody.
	if _, err := w.e.Do(guest.ID, guest.You, 3, Command{Type: CmdPlay}); !errors.Is(err, ErrNotMember) {
		t.Fatal(err)
	}
	everyone := PolicyEveryone
	if err := w.e.Set(guest.ID, guest.You, 2, Settings{Policy: &everyone}); !errors.Is(err, ErrNotHost) {
		t.Fatal(err)
	}
	if err := w.e.Set(host.ID, host.You, 1, Settings{Policy: &everyone}); err != nil {
		t.Fatal(err)
	}
	if v := w.do(guest, 2, Command{Type: CmdSeek, PositionSec: 30}); !near(v.State.PositionSec, 30) || !v.CanControl {
		t.Fatalf("%+v", v)
	}
	nonsense := "nobody"
	if err := w.e.Set(host.ID, host.You, 1, Settings{Policy: &nonsense}); !errors.Is(err, ErrBadValue) {
		t.Fatal(err)
	}
}

func TestBadCommandsAreRefused(t *testing.T) {
	w := newWorld(t, nil)
	host := w.create(person(1), 10, 11)
	for name, c := range map[string]Command{
		"unknown":       {Type: "dance"},
		"seek NaN":      {Type: CmdSeek, PositionSec: math.NaN()},
		"seek negative": {Type: CmdSeek, PositionSec: -1},
		"seek infinite": {Type: CmdSeek, PositionSec: math.Inf(1)},
		"rate zero":     {Type: CmdRate},
		"rate huge":     {Type: CmdRate, Rate: 16},
		"previous":      {Type: CmdPrevious},
		"select":        {Type: CmdSelect, Index: 2},
		"empty queue":   {Type: CmdQueue},
		"bad queue":     {Type: CmdQueue, Items: []int64{0}},
	} {
		before := w.view(host, 1).State
		if _, err := w.e.Do(host.ID, host.You, 1, c); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if after := w.view(host, 1).State; after != before {
			t.Errorf("%s: a refused command changed the room: %+v", name, after)
		}
	}
}

// Commands are applied in the order the server gets them, and each is a new seq.
func TestOrderingAndIfSeq(t *testing.T) {
	w := newWorld(t, func(c *Config) { c.MaxQueue = 10 })
	host := w.create(person(1), 10, 11, 12)
	everyone := PolicyEveryone
	_ = w.e.Set(host.ID, host.You, 1, Settings{Policy: &everyone})
	guest := w.join(person(2), host.Code)

	a := w.do(host, 1, Command{Type: CmdSeek, PositionSec: 100})
	b := w.do(guest, 2, Command{Type: CmdSeek, PositionSec: 200})
	if b.State.Seq != a.State.Seq+1 || b.State.Rev <= a.State.Rev || !near(b.State.PositionSec, 200) {
		t.Fatalf("%+v then %+v", a.State, b.State)
	}
	// The episode ends on both screens at once and both press next, each having seen seq N.
	seq := b.State.Seq
	first, err := w.e.Do(host.ID, host.You, 1, Command{Type: CmdNext, IfSeq: &seq})
	if err != nil || first.State.ItemID != 11 {
		t.Fatal(err, first.State)
	}
	second, err := w.e.Do(guest.ID, guest.You, 2, Command{Type: CmdNext, IfSeq: &seq})
	if !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	// The refusal carries the room as it is, so the loser's player follows it.
	if second.State.ItemID != 11 || second.State.Seq != first.State.Seq {
		t.Fatalf("%+v", second.State)
	}
	// Playing while playing is accepted and changes nothing.
	w.ready(host, 1)
	w.ready(guest, 2)
	p1 := w.do(host, 1, Command{Type: CmdPlay})
	p2 := w.do(guest, 2, Command{Type: CmdPlay})
	if p2.State.Seq != p1.State.Seq || p2.State.Rev != p1.State.Rev {
		t.Fatalf("%+v then %+v", p1.State, p2.State)
	}
	if _, err := w.e.Do(host.ID, host.You, 1, Command{Type: CmdNext}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.e.Do(host.ID, host.You, 1, Command{Type: CmdNext}); !errors.Is(err, ErrEndOfQueue) {
		t.Fatal(err)
	}
}

// Many people pressing things at once: every command lands, one at a time, and the room
// ends in a state one of them asked for. Run with -race.
func TestConcurrentCommands(t *testing.T) {
	w := newWorld(t, nil)
	host := w.create(person(1), 10)
	everyone := PolicyEveryone
	_ = w.e.Set(host.ID, host.You, 1, Settings{Policy: &everyone})
	views := []View{host}
	for u := int64(2); u <= 8; u++ {
		views = append(views, w.join(person(u), host.Code))
	}
	start := w.view(host, 1).State
	const each = 50
	var wg sync.WaitGroup
	for i, v := range views {
		wg.Add(1)
		go func(i int, v View) {
			defer wg.Done()
			for n := range each {
				_, err := w.e.Do(v.ID, v.You, int64(i+1), Command{Type: CmdSeek, PositionSec: float64(i*1000 + n)})
				if err != nil {
					t.Error(err)
				}
				_ = w.e.Report(v.ID, v.You, int64(i+1), ReportReady, 0)
				_ = w.e.Ping(v.ID, v.You, int64(i+1), 20)
				w.e.Tick()
			}
		}(i, v)
	}
	wg.Wait()
	end := w.view(host, 1).State
	if got := end.Seq - start.Seq; got != uint64(len(views)*each) {
		t.Fatalf("%d commands were counted, of %d", got, len(views)*each)
	}
	if n := int(end.PositionSec) % 1000; n != each-1 {
		t.Fatalf("it ended at %v, which is nobody's last seek", end.PositionSec)
	}
}

func TestWaitForEveryoneAndItsTimeout(t *testing.T) {
	w := newWorld(t, nil)
	host := w.create(person(1), 10)
	a := w.join(person(2), host.Code)
	b := w.join(person(3), host.Code)
	w.ready(host, 1)
	w.ready(a, 2)
	// b hasn't loaded yet: play holds.
	v := w.do(host, 1, Command{Type: CmdPlay})
	if !v.State.Waiting || v.State.Paused || stateOf(v, b.You) != MemberLoading {
		t.Fatalf("%+v", v)
	}
	w.pass(5 * time.Second)
	w.e.Tick()
	if v := w.view(host, 1); !v.State.Waiting || v.State.PositionAt(w.ms()) != 0 {
		t.Fatalf("it went on without them after 5s: %+v", v.State)
	}
	// b loads: it starts, a moment from now.
	w.ready(b, 3)
	v = w.view(host, 1)
	if v.State.Waiting || v.State.AtMs-w.ms() != 300 {
		t.Fatalf("%+v", v.State)
	}
	// b stalls mid-film: everyone holds, where the room was.
	w.pass(20*time.Second + 300*time.Millisecond)
	if err := w.e.Report(b.ID, b.You, 3, ReportBuffering, v.State.Rev); err != nil {
		t.Fatal(err)
	}
	v = w.view(a, 2)
	if !v.State.Waiting || !near(v.State.PositionSec, 20) || stateOf(v, b.You) != MemberBuffering {
		t.Fatalf("%+v", v)
	}
	// b never recovers. After the timeout the room goes on without them.
	for range 14 {
		w.pass(time.Second)
		w.e.Tick()
	}
	if v := w.view(a, 2); !v.State.Waiting {
		t.Fatal("it gave up before the timeout")
	}
	w.pass(time.Second)
	w.e.Tick()
	v = w.view(a, 2)
	if v.State.Waiting || stateOf(v, b.You) != MemberLagging || !near(v.State.PositionSec, 20) {
		t.Fatalf("%+v", v)
	}
	// Still stalling: for the next minute that can't stop the room again.
	w.pass(5 * time.Second)
	_ = w.e.Report(b.ID, b.You, 3, ReportBuffering, v.State.Rev)
	if v := w.view(a, 2); v.State.Waiting {
		t.Fatal("one stalled member froze the room twice in a row")
	}
	// Once they catch up they count again, and after the cooldown can hold the room again.
	w.ready(b, 3)
	if s := stateOf(w.view(a, 2), b.You); s != MemberReady {
		t.Fatal(s)
	}
	w.pass(2 * time.Minute)
	for _, x := range []struct {
		v View
		u int64
	}{{host, 1}, {a, 2}, {b, 3}} {
		_ = w.e.Ping(x.v.ID, x.v.You, x.u, 30) // nobody went away meanwhile
	}
	_ = w.e.Report(b.ID, b.You, 3, ReportBuffering, v.State.Rev)
	if v := w.view(a, 2); !v.State.Waiting {
		t.Fatal("after the cooldown a stall should hold the room")
	}
}

func TestWithoutWaitForEveryoneNobodyHoldsTheRoom(t *testing.T) {
	w := newWorld(t, nil)
	no := false
	host, err := w.e.Create(person(1), Options{Items: []int64{10}, WaitForAll: &no})
	if err != nil {
		t.Fatal(err)
	}
	guest := w.join(person(2), host.Code)
	v := w.do(host, 1, Command{Type: CmdPlay})
	if v.State.Waiting {
		t.Fatal("it waited")
	}
	_ = w.e.Report(guest.ID, guest.You, 2, ReportBuffering, v.State.Rev)
	if w.view(host, 1).State.Waiting {
		t.Fatal("it waited")
	}
	// Turning it on while someone is stalled holds it now.
	yes := true
	if err := w.e.Set(host.ID, host.You, 1, Settings{WaitForAll: &yes}); err != nil {
		t.Fatal(err)
	}
	if !w.view(host, 1).State.Waiting {
		t.Fatal("it didn't wait")
	}
}

// A "ready" that was sent before a seek is about the old place.
func TestAStaleReadyDoesntCount(t *testing.T) {
	w := newWorld(t, nil)
	host := w.create(person(1), 10)
	guest := w.join(person(2), host.Code)
	w.ready(host, 1)
	w.ready(guest, 2)
	playing := w.do(host, 1, Command{Type: CmdPlay})
	w.pass(5 * time.Second)
	seeked := w.do(host, 1, Command{Type: CmdSeek, PositionSec: 600})
	if !seeked.State.Waiting || !near(seeked.State.PositionSec, 600) {
		t.Fatalf("%+v", seeked.State)
	}
	// The guest's "ready" from before the seek arrives late.
	_ = w.e.Report(guest.ID, guest.You, 2, ReportReady, playing.State.Rev)
	w.ready(host, 1)
	if v := w.view(host, 1); !v.State.Waiting || stateOf(v, guest.You) != MemberLoading {
		t.Fatalf("%+v", v)
	}
	w.ready(guest, 2)
	if v := w.view(host, 1); v.State.Waiting || !near(v.State.PositionSec, 600) {
		t.Fatalf("%+v", v.State)
	}
}

func TestALateJoinerGetsTheStateAndDoesntStopTheRoom(t *testing.T) {
	w := newWorld(t, nil)
	host := w.create(person(1), 10)
	w.ready(host, 1)
	w.do(host, 1, Command{Type: CmdPlay})
	w.pass(90*time.Second + 300*time.Millisecond)
	_ = w.e.Ping(host.ID, host.You, 1, 20)

	late := w.join(person(2), host.Code)
	if late.State.Paused || late.State.Waiting || late.State.ItemID != 10 {
		t.Fatalf("%+v", late.State)
	}
	if p := late.State.PositionAt(late.ServerTimeMs); !near(p, 90) {
		t.Fatalf("the late joiner is told %v", p)
	}
	if s := stateOf(late, late.You); s != MemberLagging {
		t.Fatal(s)
	}
	if w.view(host, 1).State.Waiting {
		t.Fatal("joining stopped the room")
	}
	w.ready(late, 2)
	if s := stateOf(w.view(host, 1), late.You); s != MemberReady {
		t.Fatal(s)
	}
	// From now on they are waited for like anyone.
	_ = w.e.Report(late.ID, late.You, 2, ReportBuffering, w.view(late, 2).State.Rev)
	if !w.view(host, 1).State.Waiting {
		t.Fatal("a member who had caught up stalled, and the room didn't wait")
	}
}

func TestReconnectKeepsTheSeatWithinTheGrace(t *testing.T) {
	w := newWorld(t, nil)
	host := w.create(person(1), 10)
	guest := w.join(person(2), host.Code)
	w.ready(host, 1)
	w.ready(guest, 2)
	ch, err := w.e.Listen(guest.ID, guest.You, 2)
	if err != nil {
		t.Fatal(err)
	}
	w.do(host, 1, Command{Type: CmdPlay})

	// The guest's connection drops: away at once, so nobody waits for them.
	w.e.Unlisten(guest.ID, guest.You, ch)
	if s := stateOf(w.view(host, 1), guest.You); s != MemberAway {
		t.Fatal(s)
	}
	w.pass(time.Minute)
	_ = w.e.Ping(host.ID, host.You, 1, 20)
	w.e.Tick()
	// Back within the grace, from the same sign-in: the same seat.
	back := w.join(person(2), host.Code)
	if back.You != guest.You || len(back.Members) != 2 {
		t.Fatalf("%+v", back)
	}
	if s := stateOf(back, guest.You); s == MemberAway {
		t.Fatal("still away after coming back")
	}
	// The same account on another device is another seat.
	other := w.join(User{ID: 2, Name: "p2", Seat: "phone"}, host.Code)
	if other.You == guest.You || len(other.Members) != 3 {
		t.Fatalf("%+v", other)
	}
	// Past the grace the seat is gone, and the old seat id is nobody.
	w.pass(3 * time.Minute)
	_ = w.e.Ping(host.ID, host.You, 1, 20)
	w.e.Tick()
	if _, err := w.e.View(guest.ID, guest.You, 2); !errors.Is(err, ErrNotMember) {
		t.Fatal(err)
	}
	if v := w.view(host, 1); len(v.Members) != 1 {
		t.Fatalf("%+v", v.Members)
	}
}

func TestAStreamReplacedOrASeatLostClosesTheChannel(t *testing.T) {
	w := newWorld(t, nil)
	host := w.create(person(1), 10)
	guest := w.join(person(2), host.Code)
	first, _ := w.e.Listen(guest.ID, guest.You, 2)
	second, _ := w.e.Listen(guest.ID, guest.You, 2)
	if _, open := <-first; open {
		t.Fatal("the first stream should have been closed by the second")
	}
	// The first stream's handler ending mustn't make the member away: the second is open.
	w.e.Unlisten(guest.ID, guest.You, first)
	if s := stateOf(w.view(host, 1), guest.You); s == MemberAway {
		t.Fatal("away, with a stream open")
	}
	if err := w.e.Kick(host.ID, host.You, 1, guest.You); err != nil {
		t.Fatal(err)
	}
	for range second { // drains what was queued; ends because it was closed
	}
	if _, err := w.e.View(guest.ID, guest.You, 2); !errors.Is(err, ErrNotMember) {
		t.Fatal(err)
	}
	if err := w.e.Kick(guest.ID, guest.You, 2, host.You); err == nil {
		t.Fatal("a guest kicked the host")
	}
}

func TestHostTransfer(t *testing.T) {
	w := newWorld(t, nil)
	host := w.create(person(1), 10)
	a := w.join(person(2), host.Code)
	w.pass(time.Second)
	b := w.join(person(3), host.Code)
	if err := w.e.Transfer(a.ID, a.You, 2, b.You); !errors.Is(err, ErrNotHost) {
		t.Fatal(err)
	}
	if err := w.e.Transfer(host.ID, host.You, 1, "nobody"); !errors.Is(err, ErrNoMember) {
		t.Fatal(err)
	}
	if err := w.e.Transfer(host.ID, host.You, 1, b.You); err != nil {
		t.Fatal(err)
	}
	v := w.view(host, 1)
	if v.HostID != b.You || v.CanControl {
		t.Fatalf("%+v", v)
	}
	if _, err := w.e.Do(b.ID, b.You, 3, Command{Type: CmdSeek, PositionSec: 5}); err != nil {
		t.Fatal(err)
	}
	// The host leaves: whoever has been there longest takes over.
	if err := w.e.Leave(b.ID, b.You, 3); err != nil {
		t.Fatal(err)
	}
	if v := w.view(a, 2); v.HostID != host.You {
		t.Fatalf("the room went to %s", v.HostID)
	}
	// The host vanishes without leaving: once their seat is given up, the next takes over.
	w.pass(3 * time.Minute)
	_ = w.e.Ping(a.ID, a.You, 2, 20)
	w.e.Tick()
	if v := w.view(a, 2); v.HostID != a.You || len(v.Members) != 1 || !v.CanControl {
		t.Fatalf("%+v", v)
	}
}

func TestOnlyTheHostClosesAndAnEmptyRoomClosesItself(t *testing.T) {
	w := newWorld(t, nil)
	host := w.create(person(1), 10)
	guest := w.join(person(2), host.Code)
	if err := w.e.Close(guest.ID, guest.You, 2, false); !errors.Is(err, ErrNotHost) {
		t.Fatal(err)
	}
	if err := w.e.Close(host.ID, "not-a-seat", 9, false); !errors.Is(err, ErrNotMember) {
		t.Fatal(err)
	}
	ch, _ := w.e.Listen(guest.ID, guest.You, 2)
	if err := w.e.Close(host.ID, host.You, 1, false); err != nil {
		t.Fatal(err)
	}
	for range ch {
	}
	if _, err := w.e.Join(person(2), host.Code); !errors.Is(err, ErrNoRoom) {
		t.Fatal(err)
	}
	// An administrator closes any room, with no seat in it.
	other := w.create(person(1), 10)
	if err := w.e.Close(other.ID, "", 99, true); err != nil || w.e.Rooms() != 0 {
		t.Fatal(err, w.e.Rooms())
	}

	// Everyone leaves: the room and its code last a while, then go.
	room := w.create(person(1), 10)
	w.ready(room, 1)
	w.do(room, 1, Command{Type: CmdPlay})
	w.pass(30 * time.Second)
	_ = w.e.Leave(room.ID, room.You, 1)
	w.pass(4 * time.Minute)
	w.e.Tick()
	back := w.join(person(3), room.Code)
	// It stopped where it was when the last person left, and its first arrival runs it.
	if !back.State.Paused || back.State.PositionSec < 29 || back.HostID != back.You {
		t.Fatalf("%+v", back)
	}
	_ = w.e.Leave(back.ID, back.You, 3)
	w.pass(5*time.Minute + time.Second)
	w.e.Tick()
	if w.e.Rooms() != 0 {
		t.Fatal("an empty room stayed open")
	}
	// No room outlives a day, whoever is in it.
	old := w.create(person(1), 10)
	for range 25 {
		w.pass(time.Hour)
		_ = w.e.Ping(old.ID, old.You, 1, 20)
	}
	w.e.Tick()
	if w.e.Rooms() != 0 {
		t.Fatal("a day-old room stayed open")
	}
}

// A room never widens what an account may see.
func TestPermissionRefusals(t *testing.T) {
	w := newWorld(t, nil)
	w.hide(2, 10) // person 2 may not see item 10
	w.hide(2, 12)
	// Making a room for something you can't see.
	if _, err := w.e.Create(person(2), Options{Items: []int64{11, 10}}); !errors.Is(err, ErrCantSee) {
		t.Fatal(err)
	}
	host := w.create(person(1), 10, 11, 12)
	// Joining while it plays something you can't see: refused, with nothing about the room.
	v, err := w.e.Join(person(2), host.Code)
	if !errors.Is(err, ErrCantSee) || v.ID != "" || len(v.Queue) != 0 {
		t.Fatalf("%v %+v", err, v)
	}
	if got := w.view(host, 1); len(got.Members) != 1 {
		t.Fatal("the refused account got a seat")
	}
	// On to something they can see: now they may join, and see only that.
	w.do(host, 1, Command{Type: CmdNext})
	guest := w.join(person(2), host.Code)
	if guest.State.ItemID != 11 || guest.Blocked {
		t.Fatalf("%+v", guest)
	}
	if guest.Queue[0] != 0 || guest.Queue[1] != 11 || guest.Queue[2] != 0 {
		t.Fatalf("the queue shows what they may not see: %v", guest.Queue)
	}
	w.ready(host, 1)
	w.ready(guest, 2)
	// And on to something they can't: they stay seated, see no item, and hold nobody up.
	w.do(host, 1, Command{Type: CmdPlay})
	w.do(host, 1, Command{Type: CmdNext})
	g := w.view(guest, 2)
	if !g.Blocked || g.State.ItemID != 0 || g.CanControl || stateOf(g, guest.You) != MemberBlocked {
		t.Fatalf("%+v", g)
	}
	w.ready(host, 1)
	if h := w.view(host, 1); h.State.Waiting || h.State.ItemID != 12 {
		t.Fatalf("the room waits for someone who may not watch: %+v", h.State)
	}
	// Even where everyone may control, someone who can't see it can't steer it.
	everyone := PolicyEveryone
	_ = w.e.Set(host.ID, host.You, 1, Settings{Policy: &everyone})
	if _, err := w.e.Do(guest.ID, guest.You, 2, Command{Type: CmdSeek, PositionSec: 9}); !errors.Is(err, ErrCantSee) {
		t.Fatal(err)
	}
	// Nor put something in the queue that they can't see, or that the server says isn't theirs.
	w.do(host, 1, Command{Type: CmdPrevious})
	if _, err := w.e.Do(guest.ID, guest.You, 2, Command{Type: CmdQueue, Items: []int64{11, 12}}); !errors.Is(err, ErrCantSee) {
		t.Fatal(err)
	}
	// A new queue from the host is worked out again for everyone.
	w.do(host, 1, Command{Type: CmdQueue, Items: []int64{12, 11}, Index: 1})
	if g := w.view(guest, 2); g.Blocked || g.Queue[0] != 0 || g.Queue[1] != 11 || g.State.ItemID != 11 {
		t.Fatalf("%+v", g)
	}
}

// Adding to the queue, or moving things in it, around what is playing doesn't start it again.
func TestQueueAroundWhatIsPlaying(t *testing.T) {
	w := newWorld(t, func(c *Config) { c.MaxQueue = 10 })
	host := w.create(person(1), 10, 11)
	guest := w.join(person(2), host.Code)
	w.ready(host, 1)
	w.ready(guest, 2)
	w.do(host, 1, Command{Type: CmdPlay})
	w.pass(30 * time.Second)
	before := w.view(guest, 2)
	// 12 is put first and 13 last: 10 is now second, and still playing.
	v := w.do(host, 1, Command{Type: CmdQueue, Items: []int64{12, 10, 11, 13}, Index: 1})
	if v.State.ItemID != 10 || v.State.Index != 1 || v.State.Seq != before.State.Seq+1 || v.State.Paused || v.State.Waiting {
		t.Fatalf("%+v", v.State)
	}
	if !near(v.State.PositionAt(w.ms()), before.State.PositionAt(w.ms())) || v.State.AtMs != before.State.AtMs {
		t.Fatalf("the place moved: %+v, was %+v", v.State, before.State)
	}
	for _, m := range w.view(guest, 2).Members {
		if m.State != MemberReady {
			t.Fatalf("%s has to load again: %s", m.Name, m.State)
		}
	}
	// Pointing at something else is a change of item, from its start, as before.
	v = w.do(host, 1, Command{Type: CmdQueue, Items: []int64{12, 10, 11, 13}, Index: 2})
	if v.State.ItemID != 11 || !near(v.State.PositionSec, 0) || !v.State.Waiting {
		t.Fatalf("%+v", v.State)
	}
}

func TestInvitesAndLists(t *testing.T) {
	w := newWorld(t, func(c *Config) { c.MaxInvites = 2 })
	w.hide(4, 10)
	host := w.create(person(1), 10)
	// The id alone opens nothing.
	if _, err := w.e.Join(person(2), host.ID); !errors.Is(err, ErrNoRoom) {
		t.Fatal(err)
	}
	if _, err := w.e.Join(person(2), "NOSUCH"); !errors.Is(err, ErrNoRoom) {
		t.Fatal(err)
	}
	guest := w.join(person(3), host.Code)
	if err := w.e.Invite(guest.ID, guest.You, 3, []int64{2}); !errors.Is(err, ErrNotHost) {
		t.Fatal(err)
	}
	if err := w.e.Invite(host.ID, host.You, 1, []int64{2, 4, 2}); err != nil {
		t.Fatal(err)
	}
	if err := w.e.Invite(host.ID, host.You, 1, []int64{5}); !errors.Is(err, ErrTooManyInvs) {
		t.Fatal(err)
	}
	list := w.e.RoomsFor(person(2))
	if len(list) != 1 || !list[0].Invited || list[0].HostName != "p1" || list[0].MemberID != "" {
		t.Fatalf("%+v", list)
	}
	if len(w.e.RoomsFor(person(9))) != 0 {
		t.Fatal("a stranger sees the room")
	}
	in := w.join(person(2), host.ID)
	if in.State.ItemID != 10 {
		t.Fatalf("%+v", in)
	}
	if l := w.e.RoomsFor(person(2)); len(l) != 1 || l[0].Invited || l[0].MemberID != in.You {
		t.Fatalf("%+v", l)
	}
	// An invitation is not permission to see it.
	if _, err := w.e.Join(person(4), host.ID); !errors.Is(err, ErrCantSee) {
		t.Fatal(err)
	}
	// Codes are forgiving about how they're typed.
	code := host.Code
	typed := " " + strings.ToLower(code[:3]) + "-" + code[3:] + " "
	if NormalCode(typed) != code {
		t.Fatalf("%q became %q", typed, NormalCode(typed))
	}
}

func TestLimits(t *testing.T) {
	w := newWorld(t, func(c *Config) { c.MaxRooms, c.MaxRoomsPerUser, c.MaxMembers, c.MaxQueue = 3, 2, 2, 3 })
	if _, err := w.e.Create(person(1), Options{Items: []int64{1, 2, 3, 4}}); !errors.Is(err, ErrBadQueue) {
		t.Fatal(err)
	}
	if _, err := w.e.Create(person(1), Options{}); !errors.Is(err, ErrBadQueue) {
		t.Fatal(err)
	}
	if _, err := w.e.Create(person(1), Options{Items: []int64{1}, Index: 1}); !errors.Is(err, ErrBadQueue) {
		t.Fatal(err)
	}
	if _, err := w.e.Create(person(1), Options{Items: []int64{1}, PositionSec: math.Inf(1)}); !errors.Is(err, ErrBadValue) {
		t.Fatal(err)
	}
	a := w.create(person(1), 1)
	w.create(person(1), 1)
	if _, err := w.e.Create(person(1), Options{Items: []int64{1}}); !errors.Is(err, ErrTooMany) {
		t.Fatal(err)
	}
	w.create(person(2), 1)
	if _, err := w.e.Create(person(3), Options{Items: []int64{1}}); !errors.Is(err, ErrTooMany) {
		t.Fatal(err)
	}
	w.join(person(4), a.Code)
	if _, err := w.e.Join(person(5), a.Code); !errors.Is(err, ErrFull) {
		t.Fatal(err)
	}
	// A long name is cut, and one that is only control characters is replaced.
	name := strings.Repeat("é", 100)
	_ = w.e.Set(a.ID, a.You, 1, Settings{Name: &name})
	long, _ := w.e.View(a.ID, a.You, 1)
	if n := len([]rune(long.Name)); n != 60 {
		t.Fatalf("name of %d", n)
	}
}

func TestRandomCodesAreTypeable(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		c := randomCode()
		if len(c) != CodeLen || NormalCode(c) != c {
			t.Fatal(c)
		}
		for _, r := range c {
			if r == '0' || r == 'O' || r == '1' || r == 'I' || r == 'L' {
				t.Fatalf("%s holds a letter that is misread", c)
			}
		}
		seen[c] = true
	}
	if len(seen) < 199 {
		t.Fatal("codes repeat")
	}
}

func TestRate(t *testing.T) {
	w := newWorld(t, nil)
	host := w.create(person(1), 10)
	w.ready(host, 1)
	w.do(host, 1, Command{Type: CmdPlay})
	w.pass(10*time.Second + 300*time.Millisecond)
	v := w.do(host, 1, Command{Type: CmdRate, Rate: 2})
	if !near(v.State.PositionSec, 10) || v.State.Rate != 2 {
		t.Fatalf("%+v", v.State)
	}
	w.pass(5 * time.Second)
	if p := w.view(host, 1).State.PositionAt(w.ms()); !near(p, 20) {
		t.Fatalf("at double speed it reached %v", p)
	}
}
