// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/wittyremote"
)

// WittyRemote: any signed-in client can be played on, and steered, from any other. The
// messages and their rules are internal/wittyremote; docs/REMOTE-PROTOCOL.md is the guide
// for people writing a client. This file is the HTTP around it and every decision about
// who may do what:
//
//   - A device belongs to the sign-in that announced it. Only that sign-in opens its
//     channel, reports its state and answers its commands.
//   - An account controls its own devices. It controls other accounts' only when an
//     administrator has said it may (remoteControlOthers below).
//   - Nothing is played on a device that the device's own account may not see, nor that
//     the caller may not see. What a device is playing is shown to a controller only if
//     that controller may see it.
//   - Parental controls bind both ends. The limits of the account a device is signed in
//     as always decide what plays on it, whoever asks, an administrator included: every
//     kind of limit (libraries, ages by kind of media, genres, tags, collections, games
//     off), and its screen time. The caller's own limits decide what the caller may ask
//     for, and a caller whose screen time is over can pause and stop but not start
//     anything. A profile with "remoteControl" off can't steer devices at all
//     (parental.go), but its own devices can still be steered by whoever may.
//
// The routes live under /api/remote-control: /api/remote is remote access, which is about
// reaching the server from outside, not about one client steering another.

func init() {
	settingKeys = append(settingKeys, "remoteControl")
	// remoteControl: the owner's switch for the whole feature.
	settingDefaults["remoteControl"] = "1"
	settingChecks["remoteControl"] = oneOf("0", "1")
}

// remoteControlOthersKey holds which accounts may control devices signed in as other
// accounts: {"<account id>": <when that account was made>}. The second number is there
// because an account's id can be given out again after it is deleted, and the new holder
// must not inherit this.
const remoteControlOthersKey = "remoteControl.others"

const (
	maxRemoteStateBody   = 32 << 10
	maxRemoteCommandBody = 8 << 10
	remoteOff            = "Remote control is turned off on this server."
)

// remoteStreamBeat is how often a long stream here writes a keep-alive and looks at its
// sign-in again. A variable so that a test needn't wait for it.
var remoteStreamBeat = taskStreamBeat

type remoteCtl struct {
	once sync.Once
	hub  *wittyremote.Hub
	// streams counts open channels and event streams together, like the activity streams.
	streams taskStreamCounts
	// accounts: one change to who may control other accounts' devices at a time.
	accounts sync.Mutex
}

func (s *Server) remoteHub() *wittyremote.Hub {
	s.remoteCtl.once.Do(func() {
		hub := wittyremote.New(wittyremote.DefaultLimits(), nil)
		hub.OnDrop = func(d wittyremote.Device, why string) {
			s.rcLog(d.ID).Info("remote device forgotten", "why", why, "device", d.Name, "user", d.UserName)
		}
		s.remoteCtl.hub = hub
	})
	return s.remoteCtl.hub
}

// rcLog is the log of one controllable device: its records share the session
// "rc-"+its id, so the viewer can follow it. Never the sign-in's: that is a token's hash.
func (s *Server) rcLog(deviceID string) *slog.Logger {
	return s.Log.With("cat", "remotectl", "session", "rc-"+deviceID)
}

func (s *Server) remoteControlRoutes(m *http.ServeMux) {
	a, adm := s.requireUser, s.requireAdmin
	s.watchSignIns()
	m.Handle("GET /api/remote-control", a(s.handleRemoteControl))
	m.Handle("GET /api/remote-control/devices", a(s.handleRemoteDevices))
	m.Handle("POST /api/remote-control/devices", a(s.handleRemoteAnnounce))
	m.Handle("DELETE /api/remote-control/devices/{id}", a(s.handleRemoteForget))
	m.Handle("GET /api/remote-control/devices/{id}/channel", a(s.handleRemoteChannel))
	m.Handle("POST /api/remote-control/devices/{id}/state", a(s.handleRemoteState))
	m.Handle("POST /api/remote-control/devices/{id}/ack", a(s.handleRemoteAck))
	m.Handle("POST /api/remote-control/devices/{id}/commands", a(s.handleRemoteCommand))
	m.Handle("GET /api/remote-control/events", a(s.handleRemoteEvents))
	m.Handle("GET /api/remote-control/accounts", adm(s.handleRemoteAccounts))
	m.Handle("PUT /api/remote-control/accounts/{id}", adm(s.handleRemoteAccountPut))
}

// ---------------------------------------------------------------------------
// Who may do what
// ---------------------------------------------------------------------------

func (s *Server) remoteControlOn(ctx context.Context) bool {
	return s.DB.GetSetting(ctx, "remoteControl", "1") != "0"
}

func (s *Server) remoteOthers(ctx context.Context) map[string]int64 {
	out := map[string]int64{}
	_ = json.Unmarshal([]byte(s.DB.GetSetting(ctx, remoteControlOthersKey, "{}")), &out)
	return out
}

// controlsOthers: an administrator has said this account may control devices signed in
// as other accounts. Nobody has it without being given it, administrators included: what
// someone else's screen is showing is theirs until the household decides otherwise.
func (s *Server) controlsOthers(ctx context.Context, u *auth.User) bool {
	made, ok := s.remoteOthers(ctx)[strconv.FormatInt(u.ID, 10)]
	return ok && made == u.CreatedAt
}

// mayControl: this account may see and steer that device.
func mayControl(u *auth.User, others bool, d wittyremote.Device) bool {
	return d.UserID == u.ID || others
}

// displayName is what other people are shown for an account.
func displayName(u *auth.User) string {
	if u.DisplayName != "" && !u.ShowUsername {
		return u.DisplayName
	}
	return u.Name
}

// sessionOf is the sign-in a request came from, "" when it didn't come from one (a
// stream token, which is for media and has no business here).
func sessionOf(r *http.Request) string {
	tok := tokenFrom(r)
	if tok == "" || auth.IsStreamToken(tok) {
		return ""
	}
	return auth.SessionID(tok)
}

// itemFor loads an item if this account may see it. The account is whoever is given, not
// whoever is asking: it is how one account's limits are checked on another's behalf.
func (s *Server) itemFor(ctx context.Context, u *auth.User, id int64) (*library.Item, bool) {
	it, err := s.Store.Item(ctx, id)
	if err != nil {
		return nil, false
	}
	a := s.accessOf(ctx, u) // with its collections and tags looked up, so every kind of limit counts
	if a == nil {
		return it, true
	}
	var sh *library.Show
	if it.ShowID != nil {
		sh, _ = s.Store.ShowFacts(ctx, *it.ShowID)
	}
	if !a.AllowsItem(it, sh) {
		return nil, false
	}
	return it, true
}

// remoteError is a refusal with a code a program can act on.
func remoteError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": msg, "code": code})
}

func commandStatus(code string) int {
	switch code {
	case wittyremote.CodeBusy:
		return 503
	case wittyremote.CodeTimeout:
		return 504
	}
	return 409 // offline, unsupported, refused: the device's state doesn't allow it
}

// readSmall reads a JSON body that must fit in limit. It answers the request itself when
// it doesn't, or isn't JSON.
func readSmall(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		writeErr(w, 413, "that message is too long")
		return false
	}
	if len(raw) == 0 {
		return true
	}
	if err := json.Unmarshal(raw, v); err != nil {
		writeErr(w, 400, "expected a JSON body")
		return false
	}
	return true
}

// remoteGate is the start of every route here: the feature is on, and the device routes
// are being used from a real sign-in.
func (s *Server) remoteGate(w http.ResponseWriter, r *http.Request, needSession bool) (*auth.User, string, bool) {
	if !s.remoteControlOn(r.Context()) {
		remoteError(w, 403, "disabled", remoteOff)
		return nil, "", false
	}
	sess := sessionOf(r)
	if needSession && sess == "" {
		remoteError(w, 403, "no-session", "This needs a signed-in device, not a media address.")
		return nil, "", false
	}
	return userFrom(r), sess, true
}

func hubError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, wittyremote.ErrNoDevice), errors.Is(err, wittyremote.ErrNotYours):
		// The same answer for both: whether an id exists is not for another sign-in to learn.
		remoteError(w, 404, "no-device", "no such device")
	case errors.Is(err, wittyremote.ErrRate):
		w.Header().Set("Retry-After", "1")
		remoteError(w, 429, "rate", err.Error())
	case errors.Is(err, wittyremote.ErrTooMany), errors.Is(err, wittyremote.ErrTooManySubs):
		remoteError(w, 429, "too-many", err.Error())
	case errors.Is(err, wittyremote.ErrVersion):
		remoteError(w, 400, "version", err.Error())
	case errors.Is(err, wittyremote.ErrNoCommand):
		remoteError(w, 404, "no-command", err.Error())
	default:
		writeErr(w, 500, err.Error())
	}
}

// ---------------------------------------------------------------------------
// What a controller is shown
// ---------------------------------------------------------------------------

// remoteDeviceView is a device as one account sees it.
type remoteDeviceView struct {
	wittyremote.Device
	// Mine: it is signed in as the caller's own account. ThisSession: and from this very
	// sign-in, which is how an app finds itself in the list.
	Mine        bool `json:"mine"`
	ThisSession bool `json:"thisSession"`
	// Title and ItemKind describe what it is playing, when the caller may see that.
	Title    string `json:"title,omitempty"`
	ItemKind string `json:"itemKind,omitempty"`
	// Hidden: it is playing something the caller may not see. The state then says only
	// whether it is playing.
	Hidden bool `json:"hidden,omitempty"`
}

// remoteViewer shows devices to one account, remembering what it has looked up: a
// playing device reports every few seconds, and each report mustn't be a database read
// for every controller watching.
type remoteViewer struct {
	s       *Server
	u       *auth.User
	session string
	others  bool
	seen    map[int64]*library.Item // nil: looked up, and not for this account
}

func (s *Server) remoteViewer(ctx context.Context, u *auth.User, session string) *remoteViewer {
	return &remoteViewer{s: s, u: u, session: session, others: s.controlsOthers(ctx, u), seen: map[int64]*library.Item{}}
}

// refresh looks the account up again: its limits and what it may control can change
// while a stream is open.
func (v *remoteViewer) refresh(ctx context.Context) bool {
	u, err := v.s.Auth.User(ctx, v.u.ID)
	if err != nil {
		return false
	}
	v.u, v.others = u, v.s.controlsOthers(ctx, u)
	clear(v.seen)
	return true
}

func (v *remoteViewer) item(ctx context.Context, id int64) *library.Item {
	if it, ok := v.seen[id]; ok {
		return it
	}
	if len(v.seen) > 64 {
		clear(v.seen)
	}
	it, ok := v.s.itemFor(ctx, v.u, id)
	if !ok {
		it = nil
	}
	v.seen[id] = it
	return it
}

func (v *remoteViewer) view(ctx context.Context, d wittyremote.Device) remoteDeviceView {
	out := remoteDeviceView{Device: d, Mine: d.UserID == v.u.ID, ThisSession: d.UserID == v.u.ID && d.SessionID == v.session}
	if d.State.ItemID == 0 {
		return out
	}
	if it := v.item(ctx, d.State.ItemID); it != nil {
		out.Title, out.ItemKind = it.Title, it.Kind
		// The queue is the device's account's; the caller is shown only their part of it.
		if len(d.State.Queue) > 0 {
			q := make([]int64, len(d.State.Queue))
			for i, id := range d.State.Queue {
				if id == d.State.ItemID || v.item(ctx, id) != nil {
					q[i] = id
				}
			}
			out.State.Queue = q
		}
		return out
	}
	// Not for this account: that it is playing, and nothing about what.
	st := d.State
	out.State = wittyremote.State{V: st.V, Status: st.Status, Rate: 1, Volume: st.Volume, Muted: st.Muted, Screen: st.Screen}
	out.Hidden = true
	return out
}

func (v *remoteViewer) list(ctx context.Context) []remoteDeviceView {
	out := []remoteDeviceView{}
	for _, d := range v.s.remoteHub().Devices() {
		if mayControl(v.u, v.others, d) {
			out = append(out, v.view(ctx, d))
		}
	}
	return out
}

// GET /api/remote-control: the protocol's versions, limits and what this account may do.
func (s *Server) handleRemoteControl(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	l := s.remoteHub().Limits
	writeJSON(w, 200, map[string]any{
		"enabled": s.remoteControlOn(r.Context()),
		// canControl: this profile may play on and steer devices. False when the feature is
		// off for the profile (parental controls), though its own devices can still announce
		// themselves and be steered by whoever may.
		"canControl":    s.remoteControlOn(r.Context()) && accessFor(u).Can(library.FeatureRemoteControl),
		"v":             wittyremote.Version,
		"minV":          wittyremote.MinVersion,
		"controlOthers": s.controlsOthers(r.Context(), u),
		"kinds":         wittyremote.Kinds,
		"features":      wittyremote.Features,
		"mediaKinds":    wittyremote.MediaKinds,
		"navigateKeys":  wittyremote.NavigateKeys,
		"limits": map[string]any{
			"devicesPerAccount":  l.MaxDevicesPerUser,
			"watchersPerAccount": l.MaxSubscribersPerUser,
			"commandQueue":       l.QueueLen,
			"stateBytes":         maxRemoteStateBody,
			"commandBytes":       maxRemoteCommandBody,
			"stateEveryMs":       l.StateEvery.Milliseconds(),
			"commandEveryMs":     l.CommandEvery.Milliseconds(),
			"heartbeatSec":       int(l.HeartbeatEvery.Seconds()),
			"idleSec":            int(l.IdleAfter.Seconds()),
			"reconnectSec":       int(l.AnnounceGrace.Seconds()),
			"ackTimeoutMs":       l.AckTimeout.Milliseconds(),
			"queueItems":         wittyremote.MaxQueueItems,
		},
	})
}

// GET /api/remote-control/devices: the devices this account may control.
func (s *Server) handleRemoteDevices(w http.ResponseWriter, r *http.Request) {
	u, sess, ok := s.remoteGate(w, r, false)
	if !ok {
		return
	}
	writeJSON(w, 200, map[string]any{"devices": s.remoteViewer(r.Context(), u, sess).list(r.Context()), "serverTimeMs": time.Now().UnixMilli()})
}

// ---------------------------------------------------------------------------
// The device's side
// ---------------------------------------------------------------------------

// POST /api/remote-control/devices {v, name, kind, capabilities, deviceId?}: announces
// this client as something that can be controlled. The answer is what was agreed.
func (s *Server) handleRemoteAnnounce(w http.ResponseWriter, r *http.Request) {
	u, sess, ok := s.remoteGate(w, r, true)
	if !ok {
		return
	}
	var b struct {
		V            int                      `json:"v"`
		DeviceID     string                   `json:"deviceId"`
		Name         string                   `json:"name"`
		Kind         string                   `json:"kind"`
		Capabilities wittyremote.Capabilities `json:"capabilities"`
	}
	if !readSmall(w, r, &b, maxRemoteCommandBody) {
		return
	}
	if b.Name == "" {
		b.Name = deviceName("", r)
	}
	hub := s.remoteHub()
	d, err := hub.Announce(wittyremote.Announce{UserID: u.ID, UserName: displayName(u), SessionID: sess, DeviceID: b.DeviceID, V: b.V, Name: b.Name, Kind: b.Kind, Caps: b.Capabilities})
	if err != nil {
		s.Log.InfoContext(r.Context(), "remote device announce refused", "cat", "remotectl", "user", u.Name, "v", b.V, "err", err)
		hubError(w, err)
		return
	}
	s.rcLog(d.ID).InfoContext(r.Context(), "remote device announced", "device", d.Name, "kind", d.Kind, "user", u.Name, "v", d.V)
	writeJSON(w, 200, map[string]any{
		"v": d.V, "minV": wittyremote.MinVersion, "deviceId": d.ID, "name": d.Name, "kind": d.Kind, "capabilities": d.Caps,
		"heartbeatSec": int(hub.Limits.HeartbeatEvery.Seconds()), "openWithinSec": int(hub.Limits.AnnounceGrace.Seconds()),
		"serverTimeMs": time.Now().UnixMilli(),
	})
}

// DELETE /api/remote-control/devices/{id}: this client stops being controllable.
func (s *Server) handleRemoteForget(w http.ResponseWriter, r *http.Request) {
	u, sess, ok := s.remoteGate(w, r, true)
	if !ok {
		return
	}
	if err := s.remoteHub().Remove(r.PathValue("id"), u.ID, sess); err != nil {
		hubError(w, err)
		return
	}
	s.rcLog(r.PathValue("id")).DebugContext(r.Context(), "remote device asked to be forgotten", "user", u.Name)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// sessionStillGood: the token still signs in the same account. Looked at with every
// keep-alive of a long stream, so that signing a device out ends what it had open.
func (s *Server) sessionStillGood(r *http.Request, was *auth.User) bool {
	tok := tokenFrom(r)
	if tok == "" || auth.IsStreamToken(tok) {
		return false
	}
	now, err := s.Auth.Resolve(r.Context(), tok)
	return err == nil && now != nil && now.ID == was.ID
}

// GET /api/remote-control/devices/{id}/channel: the device's channel, as server-sent
// events. "hello" first, then a "command" for each thing it is asked to do, each of which
// it answers with POST .../ack. "bye" says why the server is ending it.
func (s *Server) handleRemoteChannel(w http.ResponseWriter, r *http.Request) {
	u, sess, ok := s.remoteGate(w, r, true)
	if !ok {
		return
	}
	if !s.remoteCtl.streams.take(u.ID) {
		remoteError(w, 429, "too-many", "too many remote control streams are open")
		return
	}
	defer s.remoteCtl.streams.give(u.ID)
	hub, id := s.remoteHub(), r.PathValue("id")
	conn, d, err := hub.Attach(id, u.ID, sess)
	if err != nil {
		s.rcLog(id).DebugContext(r.Context(), "remote device channel refused", "user", u.Name, "err", err)
		hubError(w, err)
		return
	}
	log := s.rcLog(id)
	log.InfoContext(r.Context(), "remote device connected", "device", d.Name, "user", u.Name)
	why := "the device closed it"
	defer func() { log.Info("remote device disconnected", "why", why) }()
	defer hub.Detach(id, conn)
	open := s.open.add(openRemoteChannel, sess, u.ID)
	defer s.open.remove(open)
	es := newEventStream(w)
	if !es.send("hello", map[string]any{"v": d.V, "deviceId": d.ID, "capabilities": d.Caps, "heartbeatSec": int(hub.Limits.HeartbeatEvery.Seconds()), "serverTimeMs": time.Now().UnixMilli()}) {
		return
	}
	beat := time.NewTicker(remoteStreamBeat)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.streamsEnding():
			why = "server-stopping"
			es.send("bye", map[string]any{"reason": why})
			return
		case <-open.end:
			// The sign-in ended (signed out, another profile, the account deleted): the
			// device is gone with it, now rather than at the next keep-alive.
			why = open.reason()
			_ = hub.Remove(id, u.ID, sess)
			es.send("bye", map[string]any{"reason": why})
			return
		case <-conn.Done():
			why = conn.Why()
			es.send("bye", map[string]any{"reason": why})
			return
		case cmd := <-conn.C:
			// A command that can't be written isn't lost quietly: the channel ends, and
			// ending it fails everything still waiting for this device.
			if !es.send("command", cmd) {
				why = "a command could not be written to it"
				return
			}
		case <-beat.C:
			if !s.sessionStillGood(r, u) || !s.remoteControlOn(r.Context()) {
				why = "signed-out"
				_ = hub.Remove(id, u.ID, sess)
				es.send("bye", map[string]any{"reason": why})
				return
			}
			hub.Sweep()
			if !es.raw([]byte(": still here\n\n")) {
				return
			}
		}
	}
}

// POST /api/remote-control/devices/{id}/state: the device says what it is doing. Also
// its heartbeat: one at least every heartbeatSec, playing or not.
func (s *Server) handleRemoteState(w http.ResponseWriter, r *http.Request) {
	u, sess, ok := s.remoteGate(w, r, true)
	if !ok {
		return
	}
	var st wittyremote.State
	if !readSmall(w, r, &st, maxRemoteStateBody) {
		return
	}
	if err := s.remoteHub().Report(r.PathValue("id"), u.ID, sess, st); err != nil {
		hubError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "serverTimeMs": time.Now().UnixMilli()})
}

// POST /api/remote-control/devices/{id}/ack {id, ok, error?}: the device answers a command.
func (s *Server) handleRemoteAck(w http.ResponseWriter, r *http.Request) {
	u, sess, ok := s.remoteGate(w, r, true)
	if !ok {
		return
	}
	var b struct {
		ID    string `json:"id"`
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if !readSmall(w, r, &b, maxRemoteCommandBody) {
		return
	}
	if err := s.remoteHub().Ack(r.PathValue("id"), u.ID, sess, b.ID, b.OK, b.Error); err != nil {
		s.rcLog(r.PathValue("id")).DebugContext(r.Context(), "remote command answer not taken", "command", b.ID, "err", err)
		hubError(w, err)
		return
	}
	s.rcLog(r.PathValue("id")).DebugContext(r.Context(), "remote command answered", "command", b.ID, "ok", b.OK, "error", b.Error)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// The controller's side
// ---------------------------------------------------------------------------

// checkPlay decides whether caller may have a device play these items. Each must be
// something the caller may see, something the device's own account may see, and of a
// kind the device said it plays.
func (s *Server) checkPlay(ctx context.Context, caller *auth.User, d wittyremote.Device, items []int64) (int, string, string) {
	owner := caller
	if d.UserID != caller.ID {
		var err error
		if owner, err = s.Auth.User(ctx, d.UserID); err != nil {
			return 409, wittyremote.CodeOffline, "That device's account no longer exists."
		}
	}
	// Screen time: neither a caller whose time is up nor a device whose profile's time is
	// up starts anything. What plays then counts as the device's profile's time, through
	// the playback routes its player uses.
	if a := accessFor(caller); s.timeIsUp(ctx, caller, a) {
		v := a.Time.Check(s.clock(), s.parentalInit().tracker.Usage(ctx, caller.ID))
		s.noteRefused(ctx, caller, "time", 0, "", v.Reason)
		return 403, "screen-time", v.Message()
	}
	if owner != caller && s.timeIsUp(ctx, owner, accessFor(owner)) {
		return 403, "screen-time-there", "Screen time is over for the profile that device is signed in as."
	}
	for _, id := range items {
		it, ok := s.itemFor(ctx, caller, id)
		if !ok {
			// Hidden and missing are one answer, as everywhere else.
			return 404, "no-item", "no such item"
		}
		if owner != caller {
			if _, ok := s.itemFor(ctx, owner, id); !ok {
				return 403, "not-allowed-there", "The account that device is signed in as isn't allowed to play that."
			}
		}
		if !d.Caps.Plays(it.Kind) {
			return 409, wittyremote.CodeUnsupported, "That device can't play that kind of thing."
		}
	}
	return 0, "", ""
}

// castFrom turns "take over from that device" into a play: what it is playing, where it
// has got to, with the sound and subtitles it has chosen.
func castFrom(src wittyremote.Device, target wittyremote.Device, now time.Time) (wittyremote.Command, bool) {
	st := src.State
	if st.ItemID == 0 || st.Status == "idle" {
		return wittyremote.Command{}, false
	}
	pos := st.PositionSec
	if st.Status == "playing" {
		pos += float64(now.UnixMilli()-src.StateAt) / 1000 * st.Rate
	}
	if st.DurationSec > 0 && pos > st.DurationSec {
		pos = st.DurationSec
	}
	if pos < 0 {
		pos = 0
	}
	cmd := wittyremote.Command{Type: wittyremote.CmdPlay, ItemID: st.ItemID, PositionSec: &pos}
	if len(st.Queue) > 1 && target.Caps.Has(wittyremote.FeatureQueue) {
		cmd.Queue, cmd.QueueIndex = st.Queue, st.QueueIndex
	}
	if st.Audio != "" {
		cmd.Audio = &st.Audio
	}
	if st.Subtitle != "" {
		cmd.Subtitle = &st.Subtitle
	}
	return cmd, true
}

// POST /api/remote-control/devices/{id}/commands: asks a device to do something, and
// waits for its answer. 200 means the device said it did it.
func (s *Server) handleRemoteCommand(w http.ResponseWriter, r *http.Request) {
	u, _, ok := s.remoteGate(w, r, false)
	if !ok {
		return
	}
	ctx := r.Context()
	hub := s.remoteHub()
	var cmd wittyremote.Command
	if !readSmall(w, r, &cmd, maxRemoteCommandBody) {
		return
	}
	// What the server fills in is the server's to fill in.
	cmd.ID, cmd.Sender, cmd.SentAt = "", nil, 0
	if err := cmd.Check(); err != nil {
		remoteError(w, 400, "bad-command", err.Error())
		return
	}
	if !hub.AllowCommand(u.ID) {
		w.Header().Set("Retry-After", "1")
		remoteError(w, 429, "rate", wittyremote.ErrRate.Error())
		return
	}
	others := s.controlsOthers(ctx, u)
	d, found := hub.Device(r.PathValue("id"))
	if !found || !mayControl(u, others, d) {
		remoteError(w, 404, "no-device", "no such device")
		return
	}
	if !d.Online {
		remoteError(w, 409, wittyremote.CodeOffline, "That device isn't connected.")
		return
	}
	var source *wittyremote.Device
	if cmd.Type == wittyremote.CmdCast {
		src, found := hub.Device(cmd.From)
		if !found || !mayControl(u, others, src) {
			remoteError(w, 404, "no-device", "no such device to take over from")
			return
		}
		if src.ID == d.ID {
			remoteError(w, 400, "bad-command", "a device can't take over from itself")
			return
		}
		play, playing := castFrom(src, d, time.Now())
		if !playing {
			remoteError(w, 409, "nothing-playing", "That device isn't playing anything to hand over.")
			return
		}
		cmd, source = play, &src
	}
	switch cmd.Type {
	case wittyremote.CmdPlay, wittyremote.CmdResume, wittyremote.CmdNext, wittyremote.CmdPrevious:
		// Resume, next and previous name no item (the device plays its own queue, which
		// its own limits made), but they start playing, so screen time is asked about.
		if status, code, msg := s.checkPlay(ctx, u, d, cmd.Items()); status != 0 {
			if code == "screen-time" {
				s.Log.InfoContext(ctx, "remote play refused: screen time", "cat", "profiles", "session", "rc-"+d.ID, "profile", u.Name, "command", cmd.Type)
				// The caller's own time: the same answer as from a playback route, so an
				// app shows the same notice.
				v := accessFor(u).Time.Check(s.clock(), s.parentalInit().tracker.Usage(ctx, u.ID))
				writeJSON(w, status, map[string]any{"error": msg, "code": code, "screenTime": screenTimeView(v)})
				return
			}
			remoteError(w, status, code, msg)
			return
		}
	}
	cmd.Sender = &wittyremote.Sender{UserName: displayName(u)}
	log := s.rcLog(d.ID)
	log.DebugContext(ctx, "remote command sent", "command", cmd.Type, "by", u.Name)
	if err := hub.Send(ctx, d.ID, d.UserID, cmd); err != nil {
		var ce *wittyremote.CommandError
		if errors.As(err, &ce) {
			log.WarnContext(ctx, "remote command failed", "command", cmd.Type, "code", ce.Code, "by", u.Name)
			remoteError(w, commandStatus(ce.Code), ce.Code, ce.Message)
			return
		}
		log.ErrorContext(ctx, "remote command failed", "command", cmd.Type, "by", u.Name, "err", err)
		writeErr(w, 500, err.Error())
		return
	}
	out := map[string]any{"ok": true}
	if source != nil {
		// The new device has it; the old one stops. If it won't, the hand-over still
		// happened, and the answer says the old one is still going.
		err := hub.Send(ctx, source.ID, source.UserID, wittyremote.Command{Type: wittyremote.CmdStop, Sender: cmd.Sender})
		out["sourceStopped"] = err == nil
		if err != nil {
			s.rcLog(source.ID).InfoContext(ctx, "remote hand-over: the old device didn't stop", "to", d.ID, "err", err)
		}
	}
	writeJSON(w, 200, out)
}

// GET /api/remote-control/events: for controllers, as server-sent events. "snapshot"
// first: every device this account may control. Then "device" when one appears or changes
// and "gone" when one leaves. A controller that reads slowly gets fewer, later, whole
// updates: never a backlog.
func (s *Server) handleRemoteEvents(w http.ResponseWriter, r *http.Request) {
	u, sess, ok := s.remoteGate(w, r, false)
	if !ok {
		return
	}
	if !s.remoteCtl.streams.take(u.ID) {
		remoteError(w, 429, "too-many", "too many remote control streams are open")
		return
	}
	defer s.remoteCtl.streams.give(u.ID)
	hub := s.remoteHub()
	sub, err := hub.Subscribe(u.ID)
	if err != nil {
		hubError(w, err)
		return
	}
	defer sub.Close()
	open := s.open.add(openRemoteEvents, sess, u.ID)
	defer s.open.remove(open)
	ctx := r.Context()
	v := s.remoteViewer(ctx, u, sess)
	es := newEventStream(w)
	shown := map[string]bool{}
	snapshot := func() bool {
		list := v.list(ctx)
		clear(shown)
		for _, d := range list {
			shown[d.ID] = true
		}
		return es.send("snapshot", map[string]any{"devices": list, "serverTimeMs": time.Now().UnixMilli()})
	}
	if !snapshot() {
		return
	}
	beat := time.NewTicker(remoteStreamBeat)
	defer beat.Stop()
	// No more than a few updates a second, however fast devices report.
	pace := time.NewTicker(250 * time.Millisecond)
	defer pace.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.streamsEnding():
			es.send("bye", map[string]any{"reason": "server-stopping"})
			return
		case <-open.end:
			es.send("bye", map[string]any{"reason": open.reason()})
			return
		case <-open.poke:
			// Limits, or who may control whose devices, just changed: say it all again now.
			if !s.remoteControlOn(ctx) || !v.refresh(ctx) || !accessFor(v.u).Can(library.FeatureRemoteControl) {
				es.send("bye", map[string]any{"reason": endedFeatureOff})
				return
			}
			if !snapshot() {
				return
			}
		case <-beat.C:
			if tokenFrom(r) != "" && !s.sessionStillGood(r, u) {
				es.send("bye", map[string]any{"reason": endedSignedOut})
				return
			}
			if !s.remoteControlOn(ctx) || !v.refresh(ctx) || !accessFor(v.u).Can(library.FeatureRemoteControl) {
				es.send("bye", map[string]any{"reason": endedFeatureOff})
				return
			}
			hub.Sweep()
			// What this account may control, or see, may have changed: say it all again.
			if !snapshot() {
				return
			}
		case <-sub.C():
			select {
			case <-pace.C:
			case <-ctx.Done():
				return
			}
			for _, id := range sub.Take() {
				d, here := hub.Device(id)
				if here && mayControl(v.u, v.others, d) {
					shown[id] = true
					if !es.send("device", v.view(ctx, d)) {
						return
					}
				} else if shown[id] {
					delete(shown, id)
					if !es.send("gone", map[string]any{"id": id}) {
						return
					}
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// The administrator's side
// ---------------------------------------------------------------------------

// GET /api/remote-control/accounts (admin): every account, and whether it may control
// devices signed in as other accounts.
func (s *Server) handleRemoteAccounts(w http.ResponseWriter, r *http.Request) {
	users, err := s.Auth.Users(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	type row struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		DisplayName   string `json:"displayName"`
		Kind          string `json:"kind"`
		IsAdmin       bool   `json:"isAdmin"`
		ControlOthers bool   `json:"controlOthers"`
	}
	out := make([]row, 0, len(users))
	for i := range users {
		u := &users[i]
		out = append(out, row{u.ID, u.Name, u.DisplayName, u.Kind, u.IsAdmin, s.controlsOthers(r.Context(), u)})
	}
	writeJSON(w, 200, map[string]any{"enabled": s.remoteControlOn(r.Context()), "accounts": out})
}

// PUT /api/remote-control/accounts/{id} {controlOthers} (admin).
func (s *Server) handleRemoteAccountPut(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad account id")
		return
	}
	var b struct {
		ControlOthers *bool `json:"controlOthers"`
	}
	if err := readJSON(r, &b); err != nil || b.ControlOthers == nil {
		writeErr(w, 400, "expected {controlOthers: true or false}")
		return
	}
	ctx := r.Context()
	target, err := s.Auth.User(ctx, id)
	if err != nil {
		writeErr(w, 404, "no such account")
		return
	}
	s.remoteCtl.accounts.Lock() // this is read, changed and written back
	defer s.remoteCtl.accounts.Unlock()
	users, err := s.Auth.Users(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// Rewritten from the accounts that exist, so one that was deleted doesn't stay listed.
	was, now := s.remoteOthers(ctx), map[string]int64{}
	for i := range users {
		k := strconv.FormatInt(users[i].ID, 10)
		if made, ok := was[k]; ok && made == users[i].CreatedAt {
			now[k] = made
		}
	}
	k := strconv.FormatInt(target.ID, 10)
	if *b.ControlOthers {
		now[k] = target.CreatedAt
	} else {
		delete(now, k)
	}
	raw, _ := json.Marshal(now)
	if err := s.DB.SetSetting(ctx, remoteControlOthersKey, string(raw)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// Whoever it is sees the difference now, not at their stream's next keep-alive.
	s.open.nudge(func(o *openStream) bool { return o.kind == openRemoteEvents && o.user == target.ID })
	s.Log.InfoContext(ctx, "remote control of other accounts' devices changed", "cat", "remotectl", "user", target.Name, "allowed", *b.ControlOthers, "by", userFrom(r).Name)
	writeJSON(w, 200, map[string]any{"ok": true, "controlOthers": *b.ControlOthers})
}
