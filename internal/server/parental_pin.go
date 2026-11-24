// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/parental"
)

// The parents' PIN on a child's device. It gives more time (handleAskParent), lets a kids'
// profile be left for another one, and lets a parent change the profile's screen time and age
// limit there and then. In each case a parent may type their own name and password instead.
// Wrong PINs are counted for the profile and for the address, like wrong sign-ins, so the PIN
// can't be guessed through any of these routes.

// pinNeededCode is what an app is told when leaving needs the PIN.
const pinNeededCode = "parent-pin"

// parentPINSet reports whether the owner has chosen a parents' PIN.
func (s *Server) parentPINSet(r *http.Request) bool {
	return s.DB.GetSetting(r.Context(), parentPINKey, "") != ""
}

// parentConsent checks that a parent is at the child's device: the parents' PIN, or the name
// and password of someone who manages this profile. It answers the request when they aren't,
// and says who agreed when they are.
func (s *Server) parentConsent(w http.ResponseWriter, r *http.Request, me *auth.User, pin, name, password string) (by string, ok bool) {
	ctx := r.Context()
	switch {
	case pin != "":
		hash := s.DB.GetSetting(ctx, parentPINKey, "")
		// Counted for the profile and for the address, so neither a new address nor
		// another profile gets a fresh set of tries.
		keys := []string{"parentpin|u" + limitName(me.Name), "parentpin|" + addressKey(r)}
		for _, k := range keys {
			if d := s.logins.wait(k); d > 0 {
				tooManyTries(w, d)
				return "", false
			}
		}
		release, ok := s.loginSlot(w, r)
		if !ok {
			return "", false
		}
		good := hash != "" && auth.CheckSecret(hash, pin)
		if hash == "" {
			auth.CheckSecret("", pin) // as long as a real check
		}
		release()
		if !good {
			for _, k := range keys {
				s.logins.fail(k)
			}
			s.logins.noteFailure()
			s.Log.InfoContext(ctx, "a wrong parents' PIN", "cat", "auth", "name", me.Name, "from", clientFrom(r).IP.String())
			writeJSON(w, 403, map[string]any{"error": "That isn't the parents' PIN.", "code": "wrong-pin"})
			return "", false
		}
		for _, k := range keys {
			s.logins.ok(k)
		}
		return "PIN", true
	case name != "" && password != "":
		release, ok := s.signInAllowed(w, r, name)
		if !ok {
			return "", false
		}
		parent := s.parentNamed(r, name, me)
		good := parent != nil && s.Auth.CheckPassword(ctx, parent.ID, password)
		if parent == nil {
			auth.CheckSecret("", password) // as long as a real check, so names can't be told apart
		}
		release()
		if !good {
			s.logins.noteFailure()
			s.logSignInFailure(r, name)
			s.Log.InfoContext(ctx, "refused: not the password of someone who manages this profile", "cat", "profiles", "profile", me.Name, "from", clientFrom(r).IP.String())
			writeJSON(w, 403, map[string]any{"error": "That name and password aren't those of someone who manages this profile.", "code": "wrong-pin"})
			return "", false
		}
		s.logins.uncount(r, name)
		return parent.Name, true
	}
	writeJSON(w, 403, map[string]any{"error": "It takes the parents' PIN, or the name and password of someone who manages this profile.", "code": pinNeededCode})
	return "", false
}

// leaveNeedsPIN: this profile is a kids' (or guest) profile kept on its device until a parent
// says otherwise, and there is a PIN to say it with. Without a PIN set there is nothing to
// ask for, as it always was.
func (s *Server) leaveNeedsPIN(r *http.Request, u *auth.User) bool {
	if u == nil || u.IsAdmin || u.Kind == auth.KindAdult || u.Kind == "" {
		return false
	}
	a := accessFor(u)
	return a != nil && a.PinToLeave && s.parentPINSet(r)
}

// keptProfileOf finds a kids' profile kept on its device among the sessions a request carries.
// Every token is asked about, as signing out ends every one: a second token sent along can't
// carry the kids' one out with it.
func (s *Server) keptProfileOf(r *http.Request) *auth.User {
	for _, tok := range auth.Tokens(r) {
		if auth.IsStreamToken(tok) {
			continue
		}
		if u, err := s.Auth.Resolve(r.Context(), tok); err == nil && s.leaveNeedsPIN(r, u) {
			return u
		}
	}
	return nil
}

// refuseLeaving answers 403 when leaving this profile needs the PIN and none was given, or a
// wrong one was. pin "" asks for it.
func (s *Server) refuseLeaving(w http.ResponseWriter, r *http.Request, u *auth.User, pin, name, password string) bool {
	if !s.leaveNeedsPIN(r, u) {
		return false
	}
	if pin == "" && (name == "" || password == "") {
		s.noteRefused(r.Context(), u, "feature", 0, "", "leave")
		writeJSON(w, 403, map[string]any{"error": "A parent's PIN is needed to leave this profile.", "code": pinNeededCode})
		return true
	}
	by, ok := s.parentConsent(w, r, u, pin, name, password)
	if !ok {
		return true
	}
	logging.Audit(r.Context(), s.Log, "a kids' profile was left with a parent's consent", "cat", "profiles", "profile", u.Name, "by", by)
	return false
}

// PUT /api/me/limits {pin | name, password, time, maxAge}: a parent changes this profile's
// screen time, bedtime and age limit on the child's own device. Only those: everything else
// is changed in Settings by whoever manages the profile.
func (s *Server) handleChangeMyLimits(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var b struct {
		PIN      string               `json:"pin"`
		Name     string               `json:"name"`
		Password string               `json:"password"`
		Time     *parental.TimeRules  `json:"time"`
		MaxAge   *int                 `json:"maxAge"`
		Kinds    map[string]*struct { // the age limit of one kind of media
			MaxAge int `json:"maxAge"`
		} `json:"kinds"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	me := userFrom(r)
	if me.IsAdmin || me.Kind == auth.KindAdult || me.Kind == "" {
		writeErr(w, 403, "this profile's limits are changed in Settings")
		return
	}
	if err := b.Time.Valid(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.MaxAge != nil && (*b.MaxAge < library.NoAgeLimit || *b.MaxAge > 21) {
		writeErr(w, 400, "the age limit must be between 0 and 21, or -1 for none")
		return
	}
	for k, l := range b.Kinds {
		if !parental.ValidKind(k) || l == nil || l.MaxAge < library.NoAgeLimit || l.MaxAge > 21 {
			writeErr(w, 400, "limits can be set for film, tv, game, music and book, between 0 and 21")
			return
		}
	}
	by, ok := s.parentConsent(w, r, me, b.PIN, strings.TrimSpace(b.Name), b.Password)
	if !ok {
		return
	}
	a := accessFor(me)
	if a == nil { // a kids' profile always has limits; this is only belt and braces
		d := library.DefaultAccess(me.Kind)
		a = &d
	}
	if b.Time != nil {
		if b.Time.Empty() {
			a.Time = nil
		} else {
			a.Time = b.Time
		}
	}
	if b.MaxAge != nil {
		a.MaxAge = *b.MaxAge
	}
	for k, l := range b.Kinds {
		kl := a.Kinds[k]
		kl.MaxAge = l.MaxAge
		if a.Kinds == nil {
			a.Kinds = map[string]library.KindLimit{}
		}
		a.Kinds[k] = kl
	}
	if err := s.cleanAccess(r, a, me.Kind); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	raw, err := json.Marshal(a)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.Auth.SetRestrictions(r.Context(), me.ID, string(raw)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.parentalInit().tracker.Forget(r.Context(), me.ID)
	s.forgetRules()
	s.limitsChanged(r.Context(), me.ID)
	logging.Audit(r.Context(), s.Log, "a profile's limits were changed on its own device", "cat", "profiles", "profile", me.Name, "by", by)
	me.Restrictions = string(raw)
	s.handleMyLimits(w, r)
}
