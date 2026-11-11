// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/syncplay"
	"github.com/wittywilla/wittymediaserver/internal/wittyremote"
)

// SyncPlay: watch parties. internal/syncplay is the rooms and their one true state;
// this file is the HTTP around it. docs/SYNCPLAY.md is the guide for people writing a
// client.
//
// What matters here:
//
//   - A room never lets anyone see more than their account may. Creating, joining and
//     changing the queue all ask about the account's own limits, and a member who may not
//     see what's playing is told only that.
//   - Nothing but the state goes through a room. Each member's player fetches the video
//     itself, through the ordinary playback routes, so the limits on quality away from
//     home, the transcoder's limits and every check on those routes apply to each member
//     as they would alone.
//   - Rooms are kept in memory. A restart ends them.

func init() {
	settingKeys = append(settingKeys, "syncplay", "syncplayCreate")
	// syncplay: the owner's switch. syncplayCreate: who may start a watch party; anyone
	// signed in may join one they have the code for.
	settingDefaults["syncplay"] = "1"
	settingDefaults["syncplayCreate"] = "everyone"
	settingChecks["syncplay"] = oneOf("0", "1")
	settingChecks["syncplayCreate"] = oneOf("everyone", "adults", "admins")
}

const syncplayOff = "Watch parties are turned off on this server."

type watchParties struct {
	once    sync.Once
	engine  *syncplay.Engine
	ticking atomic.Bool
	streams taskStreamCounts
	rates   rateKeys
}

// rateKeys is a small set of token buckets by name: one for each member's commands,
// reports and pings, and each account's wrong codes.
type rateKeys struct {
	mu sync.Mutex
	m  map[string]*rateKey
}

type rateKey struct {
	tokens float64
	at     time.Time
}

// take spends one from the bucket named key, which refills one every `every` up to burst.
func (k *rateKeys) take(key string, every time.Duration, burst int) bool {
	now := time.Now()
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.m == nil {
		k.m = map[string]*rateKey{}
	}
	if len(k.m) > 2048 { // far more than members there can be: forget the idle ones
		for name, b := range k.m {
			if now.Sub(b.at) > time.Minute {
				delete(k.m, name)
			}
		}
	}
	b := k.m[key]
	if b == nil {
		b = &rateKey{tokens: float64(burst), at: now}
		k.m[key] = b
	}
	b.tokens = min(float64(burst), b.tokens+float64(now.Sub(b.at))/float64(every))
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// give returns one: for a try that turned out not to count.
func (k *rateKeys) give(key string, burst int) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if b := k.m[key]; b != nil {
		b.tokens = min(float64(burst), b.tokens+1)
	}
}

func (s *Server) syncEngine() *syncplay.Engine {
	s.watch.once.Do(func() {
		s.watch.engine = syncplay.New(syncplay.Config{CanSee: func(userID, itemID int64) bool {
			// Asked fresh, so that limits an administrator has just set count from the next
			// item on. Not tied to any request: the engine asks on behalf of every member.
			ctx := context.Background()
			u, err := s.Auth.User(ctx, userID)
			if err != nil {
				return false
			}
			_, ok := s.itemFor(ctx, u, itemID)
			return ok
		}})
	})
	return s.watch.engine
}

// syncTick keeps the rooms' clocks moving while there are rooms, and stops when there
// are none, so a server nobody is having a watch party on does nothing at all.
func (s *Server) syncTick() {
	if !s.watch.ticking.CompareAndSwap(false, true) {
		return
	}
	ok := s.background("syncplay", func(ctx context.Context) {
		defer s.watch.ticking.Store(false)
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				e := s.syncEngine()
				e.Tick()
				if e.Rooms() == 0 {
					return
				}
			}
		}
	})
	if !ok {
		s.watch.ticking.Store(false)
	}
}

func (s *Server) syncplayRoutes(m *http.ServeMux) {
	a := s.requireUser
	m.Handle("GET /api/syncplay", a(s.handleSyncplay))
	m.Handle("POST /api/syncplay/rooms", a(s.handleSyncCreate))
	m.Handle("POST /api/syncplay/join", a(s.handleSyncJoin))
	m.Handle("GET /api/syncplay/rooms/{id}", a(s.handleSyncRoom))
	m.Handle("PUT /api/syncplay/rooms/{id}", a(s.handleSyncSet))
	m.Handle("DELETE /api/syncplay/rooms/{id}", a(s.handleSyncClose))
	m.Handle("GET /api/syncplay/rooms/{id}/events", a(s.handleSyncEvents))
	m.Handle("POST /api/syncplay/rooms/{id}/leave", a(s.handleSyncLeave))
	m.Handle("POST /api/syncplay/rooms/{id}/host", a(s.handleSyncHost))
	m.Handle("POST /api/syncplay/rooms/{id}/kick", a(s.handleSyncKick))
	m.Handle("POST /api/syncplay/rooms/{id}/invite", a(s.handleSyncInvite))
	m.Handle("POST /api/syncplay/rooms/{id}/command", a(s.handleSyncCommand))
	m.Handle("POST /api/syncplay/rooms/{id}/report", a(s.handleSyncReport))
	m.Handle("POST /api/syncplay/rooms/{id}/ping", a(s.handleSyncPing))
}

func (s *Server) syncplayOn(ctx context.Context) bool {
	return s.DB.GetSetting(ctx, "syncplay", "1") != "0"
}

func (s *Server) mayCreateParty(ctx context.Context, u *auth.User) bool {
	switch s.DB.GetSetting(ctx, "syncplayCreate", "everyone") {
	case "admins":
		return u.IsAdmin
	case "adults":
		return u.IsAdmin || u.Kind == auth.KindAdult || u.Kind == ""
	}
	return true
}

// syncGate starts every route: the feature is on.
func (s *Server) syncGate(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	if !s.syncplayOn(r.Context()) {
		remoteError(w, 403, "disabled", syncplayOff)
		return nil, false
	}
	return userFrom(r), true
}

// syncUser is the caller as the engine knows them. The seat is the sign-in and, within
// it, whatever the app calls this window: two tabs of one browser are two seats.
func syncUser(r *http.Request, device string) (syncplay.User, bool) {
	sess := sessionOf(r)
	if sess == "" {
		return syncplay.User{}, false
	}
	u := userFrom(r)
	return syncplay.User{ID: u.ID, Name: displayName(u), Seat: sess + ":" + wittyremote.Clip(device, 40)}, true
}

func syncError(w http.ResponseWriter, err error) {
	code, status := "error", 400
	switch {
	case errors.Is(err, syncplay.ErrNoRoom):
		code, status = "no-room", 404
	case errors.Is(err, syncplay.ErrNotMember):
		code, status = "not-member", 404 // as if the room weren't there
	case errors.Is(err, syncplay.ErrNoMember):
		code, status = "no-member", 404
	case errors.Is(err, syncplay.ErrNotHost):
		code, status = "not-host", 403
	case errors.Is(err, syncplay.ErrNotAllowed):
		code, status = "host-only", 403
	case errors.Is(err, syncplay.ErrCantSee):
		code, status = "cant-watch", 403
	case errors.Is(err, syncplay.ErrFull):
		code, status = "full", 409
	case errors.Is(err, syncplay.ErrTooMany), errors.Is(err, syncplay.ErrTooManyInvs):
		code, status = "too-many", 429
	case errors.Is(err, syncplay.ErrStale):
		code, status = "stale", 409
	case errors.Is(err, syncplay.ErrEndOfQueue):
		code, status = "end-of-queue", 409
	case errors.Is(err, syncplay.ErrBadQueue), errors.Is(err, syncplay.ErrBadValue), errors.Is(err, syncplay.ErrBadCommand):
		code = "bad-request"
	}
	remoteError(w, status, code, err.Error())
}

// ---------------------------------------------------------------------------
// What a member is shown
// ---------------------------------------------------------------------------

// syncItem is a queue entry with enough to show it. An entry the member may not see is
// an id of 0 and nothing else.
type syncItem struct {
	ID         int64   `json:"id"`
	Title      string  `json:"title,omitempty"`
	Kind       string  `json:"kind,omitempty"`
	RuntimeSec float64 `json:"runtimeSec,omitempty"`
}

// syncRoomView is the engine's view plus the titles of what this member may see.
type syncRoomView struct {
	syncplay.View
	Items []syncItem `json:"items"`
}

// syncTitles remembers titles for one stream, so a room that changes ten times a minute
// doesn't read its queue from the database ten times a minute for every member.
type syncTitles struct {
	s    *Server
	user int64
	seen map[int64]syncItem
}

func (t *syncTitles) view(ctx context.Context, v syncplay.View) syncRoomView {
	out := syncRoomView{View: v, Items: make([]syncItem, len(v.Queue))}
	if t.seen == nil {
		t.seen = map[int64]syncItem{}
	}
	var u *auth.User
	for i, id := range v.Queue {
		if id == 0 {
			continue
		}
		it, ok := t.seen[id]
		if !ok {
			if u == nil {
				var err error
				if u, err = t.s.Auth.User(ctx, t.user); err != nil {
					continue
				}
			}
			// The engine said this account may see it; asked again here because this is
			// where the title leaves the server.
			if li, visible := t.s.itemFor(ctx, u, id); visible {
				it = syncItem{ID: id, Title: li.Title, Kind: li.Kind, RuntimeSec: li.RuntimeSec}
			}
			if len(t.seen) < 512 {
				t.seen[id] = it
			}
		}
		out.Items[i] = it
	}
	return out
}

func (s *Server) syncView(r *http.Request, v syncplay.View) syncRoomView {
	return (&syncTitles{s: s, user: userFrom(r).ID}).view(r.Context(), v)
}

// GET /api/syncplay: whether watch parties are on, the rules clients keep in step by, and
// this account's rooms and invitations.
func (s *Server) handleSyncplay(w http.ResponseWriter, r *http.Request) {
	ctx, u := r.Context(), userFrom(r)
	cfg := s.syncEngine().Config()
	out := map[string]any{
		"enabled":   s.syncplayOn(ctx),
		"canCreate": s.syncplayOn(ctx) && s.mayCreateParty(ctx, u),
		"drift": map[string]any{
			"stillSec": syncplay.DriftStill, "settledSec": syncplay.DriftSettled, "seekSec": syncplay.DriftSeek, "rate": syncplay.DriftRate,
		},
		"limits": map[string]any{
			"members": cfg.MaxMembers, "queue": cfg.MaxQueue, "roomsPerAccount": cfg.MaxRoomsPerUser, "invites": cfg.MaxInvites,
			"codeLength": syncplay.CodeLen, "pingEverySec": 5, "awaySec": int(cfg.AwayAfter.Seconds()), "seatGraceSec": int(cfg.SeatGrace.Seconds()),
			"waitTimeoutSec": int(cfg.WaitTimeout.Seconds()), "emptyCloseSec": int(cfg.EmptyClose.Seconds()),
			"minLeadMs": cfg.MinLead.Milliseconds(), "maxLeadMs": cfg.MaxLead.Milliseconds(),
		},
		"rooms":        []syncplay.Listing{},
		"serverTimeMs": time.Now().UnixMilli(),
	}
	if su, ok := syncUser(r, r.URL.Query().Get("device")); ok && s.syncplayOn(ctx) {
		out["rooms"] = s.syncEngine().RoomsFor(su)
	}
	writeJSON(w, 200, out)
}

// POST /api/syncplay/rooms {itemId | items, index?, positionSec?, name?, policy?,
// waitForAll?, device?}: starts a watch party, with the caller as its host.
func (s *Server) handleSyncCreate(w http.ResponseWriter, r *http.Request) {
	u, ok := s.syncGate(w, r)
	if !ok {
		return
	}
	if !s.mayCreateParty(r.Context(), u) {
		remoteError(w, 403, "not-allowed", "This account can't start a watch party. It can join one with a code.")
		return
	}
	var b struct {
		ItemID      int64   `json:"itemId"`
		Items       []int64 `json:"items"`
		Index       int     `json:"index"`
		PositionSec float64 `json:"positionSec"`
		Name        string  `json:"name"`
		Policy      string  `json:"policy"`
		WaitForAll  *bool   `json:"waitForAll"`
		Device      string  `json:"device"`
	}
	if !readSmall(w, r, &b, maxRemoteCommandBody) {
		return
	}
	su, ok := syncUser(r, b.Device)
	if !ok {
		remoteError(w, 403, "no-session", "This needs a signed-in device, not a media address.")
		return
	}
	if len(b.Items) == 0 && b.ItemID > 0 {
		b.Items = []int64{b.ItemID}
	}
	if b.Policy != "" && b.Policy != syncplay.PolicyHost && b.Policy != syncplay.PolicyEveryone {
		remoteError(w, 400, "bad-request", `policy is "host" or "everyone"`)
		return
	}
	v, err := s.syncEngine().Create(su, syncplay.Options{Name: b.Name, Items: b.Items, Index: b.Index, Policy: b.Policy, WaitForAll: b.WaitForAll, PositionSec: b.PositionSec})
	if err != nil {
		if errors.Is(err, syncplay.ErrCantSee) {
			// Hidden and missing are one answer.
			remoteError(w, 404, "no-item", "no such item")
			return
		}
		syncError(w, err)
		return
	}
	s.syncTick()
	writeJSON(w, 200, s.syncView(r, v))
}

// POST /api/syncplay/join {code | roomId, device?}: takes a seat, or takes back the one
// this sign-in had. A room's id works only for someone invited or already seated.
func (s *Server) handleSyncJoin(w http.ResponseWriter, r *http.Request) {
	u, ok := s.syncGate(w, r)
	if !ok {
		return
	}
	var b struct {
		Code   string `json:"code"`
		RoomID string `json:"roomId"`
		Device string `json:"device"`
	}
	if !readSmall(w, r, &b, maxRemoteCommandBody) {
		return
	}
	su, ok := syncUser(r, b.Device)
	if !ok {
		remoteError(w, 403, "no-session", "This needs a signed-in device, not a media address.")
		return
	}
	key := b.RoomID
	if key == "" {
		key = syncplay.NormalCode(b.Code)
		if len(key) != syncplay.CodeLen {
			remoteError(w, 404, "no-room", syncplay.ErrNoRoom.Error())
			return
		}
	}
	// Codes are short so that they can be read out; wrong ones are counted so that they
	// can't be guessed.
	const tries, every = 10, 6 * time.Second
	limit := "join:" + strconv.FormatInt(u.ID, 10)
	if !s.watch.rates.take(limit, every, tries) {
		w.Header().Set("Retry-After", "10")
		remoteError(w, 429, "rate", "Too many tries. Wait a little and try again.")
		return
	}
	v, err := s.syncEngine().Join(su, key)
	if err != nil {
		syncError(w, err)
		return
	}
	s.watch.rates.give(limit, tries)
	s.syncTick()
	writeJSON(w, 200, s.syncView(r, v))
}

// seatBody is what every route for a seated member takes: which seat.
type seatBody struct {
	Member string `json:"member"`
}

// GET /api/syncplay/rooms/{id}?member=: the room, for one of its members.
func (s *Server) handleSyncRoom(w http.ResponseWriter, r *http.Request) {
	u, ok := s.syncGate(w, r)
	if !ok {
		return
	}
	v, err := s.syncEngine().View(r.PathValue("id"), r.URL.Query().Get("member"), u.ID)
	if err != nil {
		syncError(w, err)
		return
	}
	writeJSON(w, 200, s.syncView(r, v))
}

// PUT /api/syncplay/rooms/{id} {member, name?, policy?, waitForAll?}: the host's settings.
func (s *Server) handleSyncSet(w http.ResponseWriter, r *http.Request) {
	u, ok := s.syncGate(w, r)
	if !ok {
		return
	}
	var b struct {
		seatBody
		Name       *string `json:"name"`
		Policy     *string `json:"policy"`
		WaitForAll *bool   `json:"waitForAll"`
	}
	if !readSmall(w, r, &b, maxRemoteCommandBody) {
		return
	}
	if err := s.syncEngine().Set(r.PathValue("id"), b.Member, u.ID, syncplay.Settings{Name: b.Name, Policy: b.Policy, WaitForAll: b.WaitForAll}); err != nil {
		syncError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/syncplay/rooms/{id}?member=: ends the party for everyone. Its host, or an
// administrator.
func (s *Server) handleSyncClose(w http.ResponseWriter, r *http.Request) {
	u, ok := s.syncGate(w, r)
	if !ok {
		return
	}
	if err := s.syncEngine().Close(r.PathValue("id"), r.URL.Query().Get("member"), u.ID, u.IsAdmin); err != nil {
		syncError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/syncplay/rooms/{id}/leave {member}
func (s *Server) handleSyncLeave(w http.ResponseWriter, r *http.Request) {
	u, ok := s.syncGate(w, r)
	if !ok {
		return
	}
	var b seatBody
	if !readSmall(w, r, &b, maxRemoteCommandBody) {
		return
	}
	if err := s.syncEngine().Leave(r.PathValue("id"), b.Member, u.ID); err != nil {
		syncError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/syncplay/rooms/{id}/host {member, to}: the host hands the room to another member.
func (s *Server) handleSyncHost(w http.ResponseWriter, r *http.Request) {
	u, ok := s.syncGate(w, r)
	if !ok {
		return
	}
	var b struct {
		seatBody
		To string `json:"to"`
	}
	if !readSmall(w, r, &b, maxRemoteCommandBody) {
		return
	}
	if err := s.syncEngine().Transfer(r.PathValue("id"), b.Member, u.ID, b.To); err != nil {
		syncError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/syncplay/rooms/{id}/kick {member, whom}: the host takes a seat away.
func (s *Server) handleSyncKick(w http.ResponseWriter, r *http.Request) {
	u, ok := s.syncGate(w, r)
	if !ok {
		return
	}
	var b struct {
		seatBody
		Whom string `json:"whom"`
	}
	if !readSmall(w, r, &b, maxRemoteCommandBody) {
		return
	}
	if err := s.syncEngine().Kick(r.PathValue("id"), b.Member, u.ID, b.Whom); err != nil {
		syncError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/syncplay/rooms/{id}/invite {member, userIds}: the host invites accounts, who
// then find the room in their list. An invitation is a shortcut past the code, not past
// anyone's limits.
func (s *Server) handleSyncInvite(w http.ResponseWriter, r *http.Request) {
	u, ok := s.syncGate(w, r)
	if !ok {
		return
	}
	var b struct {
		seatBody
		UserIDs []int64 `json:"userIds"`
	}
	if !readSmall(w, r, &b, maxRemoteCommandBody) {
		return
	}
	if len(b.UserIDs) == 0 || len(b.UserIDs) > 64 {
		remoteError(w, 400, "bad-request", "userIds: between 1 and 64 accounts")
		return
	}
	// Only accounts that exist: an id nobody has yet mustn't be an invitation waiting for
	// whoever gets it.
	known := []int64{}
	for _, id := range b.UserIDs {
		if _, err := s.Auth.User(r.Context(), id); err == nil {
			known = append(known, id)
		}
	}
	if err := s.syncEngine().Invite(r.PathValue("id"), b.Member, u.ID, known); err != nil {
		syncError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "invited": len(known)})
}

// POST /api/syncplay/rooms/{id}/command {member, type, positionSec?, index?, rate?,
// items?, ifSeq?}: play, pause, seek, next, previous, select, rate, queue. The answer is
// the room after it. A command refused as stale (409) carries the room as it is now.
func (s *Server) handleSyncCommand(w http.ResponseWriter, r *http.Request) {
	u, ok := s.syncGate(w, r)
	if !ok {
		return
	}
	var b struct {
		seatBody
		Type        string  `json:"type"`
		PositionSec float64 `json:"positionSec"`
		Index       int     `json:"index"`
		Rate        float64 `json:"rate"`
		Items       []int64 `json:"items"`
		IfSeq       *uint64 `json:"ifSeq"`
	}
	if !readSmall(w, r, &b, maxRemoteCommandBody) {
		return
	}
	if !s.watch.rates.take("cmd:"+b.Member, 250*time.Millisecond, 12) {
		w.Header().Set("Retry-After", "1")
		remoteError(w, 429, "rate", "too many commands: slow down")
		return
	}
	v, err := s.syncEngine().Do(r.PathValue("id"), b.Member, u.ID, syncplay.Command{Type: b.Type, PositionSec: b.PositionSec, Index: b.Index, Rate: b.Rate, Items: b.Items, IfSeq: b.IfSeq})
	if err != nil {
		if errors.Is(err, syncplay.ErrStale) {
			writeJSON(w, 409, map[string]any{"error": err.Error(), "code": "stale", "room": s.syncView(r, v)})
			return
		}
		syncError(w, err)
		return
	}
	writeJSON(w, 200, s.syncView(r, v))
}

// POST /api/syncplay/rooms/{id}/report {member, state, rev}: a member's player says it
// is "ready" (loaded where the room is) or "buffering".
func (s *Server) handleSyncReport(w http.ResponseWriter, r *http.Request) {
	u, ok := s.syncGate(w, r)
	if !ok {
		return
	}
	var b struct {
		seatBody
		State string `json:"state"`
		Rev   uint64 `json:"rev"`
	}
	if !readSmall(w, r, &b, maxRemoteCommandBody) {
		return
	}
	if !s.watch.rates.take("report:"+b.Member, 250*time.Millisecond, 12) {
		w.Header().Set("Retry-After", "1")
		remoteError(w, 429, "rate", "too many reports: slow down")
		return
	}
	if err := s.syncEngine().Report(r.PathValue("id"), b.Member, u.ID, b.State, b.Rev); err != nil {
		syncError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func msNow() float64 { return float64(time.Now().UnixMicro()) / 1000 }

// POST /api/syncplay/rooms/{id}/ping {member, t0, rttMs?}: the heartbeat and the clock.
// The answer is {t0, t1, t2}: the client's own send time back, the server's time when the
// ping arrived and when the answer left. With its own time of arrival the client has the
// four numbers of syncplay.ClockOffset.
func (s *Server) handleSyncPing(w http.ResponseWriter, r *http.Request) {
	t1 := msNow()
	u, ok := s.syncGate(w, r)
	if !ok {
		return
	}
	var b struct {
		seatBody
		T0    float64 `json:"t0"`
		RTTMs float64 `json:"rttMs"`
	}
	if !readSmall(w, r, &b, maxRemoteCommandBody) {
		return
	}
	if !s.watch.rates.take("ping:"+b.Member, 500*time.Millisecond, 10) {
		w.Header().Set("Retry-After", "1")
		remoteError(w, 429, "rate", "too many pings: slow down")
		return
	}
	if err := s.syncEngine().Ping(r.PathValue("id"), b.Member, u.ID, b.RTTMs); err != nil {
		syncError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"t0": b.T0, "t1": t1, "t2": msNow()})
}

// GET /api/syncplay/rooms/{id}/events?member=: the room, as server-sent events. "room"
// first and again whenever anything changes, whole each time, so a member who reads
// slowly or reconnects is simply told how things stand. "closed" is the last word, with
// why: the room ended, the seat was taken away, or another stream took this seat.
func (s *Server) handleSyncEvents(w http.ResponseWriter, r *http.Request) {
	u, ok := s.syncGate(w, r)
	if !ok {
		return
	}
	if !s.watch.streams.take(u.ID) {
		remoteError(w, 429, "too-many", "too many watch party streams are open")
		return
	}
	defer s.watch.streams.give(u.ID)
	e, room, member := s.syncEngine(), r.PathValue("id"), r.URL.Query().Get("member")
	ch, err := e.Listen(room, member, u.ID)
	if err != nil {
		syncError(w, err)
		return
	}
	defer e.Unlisten(room, member, ch)
	s.syncTick()
	ctx := r.Context()
	titles := &syncTitles{s: s, user: u.ID}
	es := newEventStream(w)
	send := func() bool {
		v, err := e.View(room, member, u.ID)
		if err != nil {
			return false
		}
		return es.send("room", titles.view(ctx, v))
	}
	closed := func() {
		why := "replaced"
		if _, err := e.View(room, member, u.ID); errors.Is(err, syncplay.ErrNoRoom) {
			why = "closed"
		} else if err != nil {
			why = "removed"
		}
		es.send("closed", map[string]any{"reason": why})
	}
	if !send() {
		closed()
		return
	}
	beat := time.NewTicker(remoteStreamBeat)
	defer beat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.streamsEnding():
			es.send("closed", map[string]any{"reason": "server-stopping"})
			return
		case _, open := <-ch:
			if !open || !send() {
				closed()
				return
			}
		case <-beat.C:
			if !s.sessionStillGood(r, u) || !s.syncplayOn(ctx) {
				_ = e.Leave(room, member, u.ID)
				es.send("closed", map[string]any{"reason": "signed-out"})
				return
			}
			clear(titles.seen) // limits may have changed
			if !es.raw([]byte(": still here\n\n")) {
				return
			}
		}
	}
}
