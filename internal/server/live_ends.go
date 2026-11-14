// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"strings"
	"sync"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/syncplay"
)

// What a sign-in has open, and ending it the moment the sign-in ends.
//
// A device announced for remote control, a controller's stream of events, a seat in a
// watch party and a game in an emulator all go on after the request that started them.
// Each looks at its sign-in again every so often, but "every so often" is 25 seconds in
// which a device somebody just signed out of, or switched to another profile on, would go
// on acting as the profile before. So the places sign-ins are ended (internal/auth) tell
// the server, and the server ends these at once. The same is done when an account's limits
// change so that it may no longer do what it has open.

// Kinds of stream kept here.
const (
	openRemoteChannel = "remote-channel" // a device's own channel
	openRemoteEvents  = "remote-events"  // a controller watching devices
	openParty         = "syncplay"       // a seat's stream in a watch party
)

// Why a stream was ended, as its last message says it.
const (
	endedSignedOut  = "signed-out"
	endedFeatureOff = "feature-off"
)

// openStream is one long answer that is being written.
type openStream struct {
	kind    string
	session string
	user    int64
	// end is closed when the stream must stop; why then says the reason.
	end chan struct{}
	// poke asks the stream to look at its account again now, not at its next keep-alive.
	poke chan struct{}
	why  string
}

// reason is why it was ended. Only read after end is closed.
func (o *openStream) reason() string {
	select {
	case <-o.end:
		return o.why
	default:
		return ""
	}
}

// openStreams is every such stream. It can't outgrow the streams there may be: each is
// counted against its own limit (taskStreamCounts) before it is added here.
type openStreams struct {
	mu sync.Mutex
	m  map[*openStream]struct{}
}

func (o *openStreams) add(kind, session string, user int64) *openStream {
	st := &openStream{kind: kind, session: session, user: user, end: make(chan struct{}), poke: make(chan struct{}, 1)}
	o.mu.Lock()
	if o.m == nil {
		o.m = map[*openStream]struct{}{}
	}
	o.m[st] = struct{}{}
	o.mu.Unlock()
	return st
}

func (o *openStreams) remove(st *openStream) {
	o.mu.Lock()
	delete(o.m, st)
	o.mu.Unlock()
}

// stop ends every stream that match picks, and says how many.
func (o *openStreams) stop(why string, match func(*openStream) bool) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := 0
	for st := range o.m {
		if match(st) {
			st.why = why
			close(st.end)
			delete(o.m, st)
			n++
		}
	}
	return n
}

// nudge has every stream that match picks look at its account again now.
func (o *openStreams) nudge(match func(*openStream) bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for st := range o.m {
		if match(st) {
			select {
			case st.poke <- struct{}{}:
			default:
			}
		}
	}
}

func (o *openStreams) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.m)
}

// watchSignIns has internal/auth tell the server when sign-ins end.
func (s *Server) watchSignIns() {
	if s.Auth != nil && s.Auth.OnEnd == nil {
		s.Auth.OnEnd = s.signInsEnded
	}
}

// seatSession is the sign-in a watch party seat belongs to (see syncUser).
func seatSession(seat string) string {
	sess, _, _ := strings.Cut(seat, ":")
	return sess
}

// signInsEnded ends everything the sign-ins had open: one sign-in, or all of an account
// but one. It is called for signing out, switching profile, "sign out everywhere else", a
// new password, a device signed out from Settings, and an account deleted.
func (s *Server) signInsEnded(e auth.SessionEnd) {
	gone := func(session string, user int64) bool {
		if e.Session != "" {
			return session == e.Session
		}
		return user == e.UserID && (e.Keep == "" || session != e.Keep)
	}
	// The streams first, so that each says why it ended rather than that its seat went.
	s.open.stop(endedSignedOut, func(o *openStream) bool { return gone(o.session, o.user) })
	// Both are made when first wanted, and are empty and cheap until something is in them.
	if e.Session != "" {
		s.remoteHub().RemoveSession(e.Session)
	} else {
		s.remoteHub().RemoveUser(e.UserID, e.Keep)
	}
	s.syncEngine().DropSeats(func(u syncplay.User) bool { return gone(seatSession(u.Seat), u.ID) })
	s.emus.stop(s, func(g *emulatorGame) bool { return gone(g.session, g.user) })
	signIn := s.signInHandle(e.Session)
	keep := s.signInHandle(e.Keep)
	s.stopModuleSessions("signed-out", func(v *moduleSession) bool {
		if e.Session != "" {
			return v.SignIn == signIn
		}
		return v.User == e.UserID && (e.Keep == "" || v.SignIn != keep)
	})
}

// accountGone is signInsEnded for an account that no longer exists: what it was invited to
// goes too. Its sign-ins are already announced by internal/auth.
func (s *Server) accountGone(userID int64) {
	s.syncEngine().Uninvite(userID)
}

// limitsChanged is called when an account's limits were just saved, or it was given more
// time. What it has open is made to follow them now: a watch party asks again what it may
// see, a controller's list of devices is sent again, and what it may no longer do at all
// is ended.
func (s *Server) limitsChanged(ctx context.Context, userID int64) {
	u, err := s.Auth.User(ctx, userID)
	if err != nil {
		return
	}
	a := accessFor(u)
	mine := func(kinds ...string) func(*openStream) bool {
		return func(o *openStream) bool {
			if o.user != userID {
				return false
			}
			for _, k := range kinds {
				if o.kind == k {
					return true
				}
			}
			return false
		}
	}
	if !a.Can(library.FeatureRemoteControl) {
		s.open.stop(endedFeatureOff, mine(openRemoteEvents))
	}
	if !a.Can(library.FeatureGames) {
		s.emus.stop(s, func(g *emulatorGame) bool { return g.user == userID })
		s.stopModuleSessions("feature-off", func(v *moduleSession) bool { return v.User == userID })
	}
	if e := s.syncEngine(); !a.Can(library.FeatureSyncplay) {
		s.open.stop(endedFeatureOff, mine(openParty))
		e.DropSeats(func(m syncplay.User) bool { return m.ID == userID })
		e.Uninvite(userID)
	} else {
		e.Recheck(userID)
		e.SetTimeUp(userID, s.timeIsUp(ctx, u, a))
	}
	// Whoever is watching devices may now see more or less of what they play.
	s.open.nudge(mine(openRemoteEvents))
	// And so may whoever watches this account's devices, which is anyone allowed to.
	s.open.nudge(func(o *openStream) bool { return o.kind == openRemoteEvents && o.user != userID })
}

// timeIsUp: the account has time rules, and by them may not play now.
func (s *Server) timeIsUp(ctx context.Context, u *auth.User, a *library.Access) bool {
	if a == nil || a.Time.Empty() {
		return false
	}
	return !a.Time.Check(s.clock(), s.parentalInit().tracker.Usage(ctx, u.ID)).Allowed
}

// watchPartyMembers looks at everyone seated in a watch party: an account that is gone or
// may no longer use watch parties loses its seats, and one whose screen time is over sits
// out, so that a room which waits for everyone doesn't wait for someone who can't play.
// The server calls it every few seconds while there are rooms.
//
// It costs one read of each seated account, and nothing at all while nobody is in a party.
func (s *Server) watchPartyMembers(ctx context.Context) {
	e := s.syncEngine()
	recheck := s.watch.looks.Add(1)%partyRecheckEvery == 0
	for _, id := range e.Seated() {
		u, err := s.Auth.User(ctx, id)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			e.DropSeats(func(m syncplay.User) bool { return m.ID == id })
			e.Uninvite(id)
			continue
		}
		a := accessFor(u)
		if !a.Can(library.FeatureSyncplay) {
			s.open.stop(endedFeatureOff, func(o *openStream) bool { return o.kind == openParty && o.user == id })
			e.DropSeats(func(m syncplay.User) bool { return m.ID == id })
			e.Uninvite(id)
			continue
		}
		e.SetTimeUp(id, s.timeIsUp(ctx, u, a))
		// Limits that go by a collection or a tag follow the collection as it changes, which
		// nobody announces. So for those accounts, and only those, the party asks again now
		// and then what they may see.
		if recheck && a != nil && len(a.AllowCollections)+len(a.BlockCollections)+len(a.BlockTags) > 0 {
			e.Recheck(id)
		}
	}
}
