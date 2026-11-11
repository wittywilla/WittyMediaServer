// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"errors"
	"net/http"

	"github.com/wittywilla/wittymediaserver/internal/auth"
)

// Signed-in devices and two-factor sign-in, for every adult.

func currentToken(r *http.Request) string {
	t, _ := r.Context().Value(tokenKey).(string)
	return t
}

// GET /api/sessions: your signed-in devices; ?all=1 everyone's (admin).
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	who := me.ID
	if qBool(r, "all") {
		if !me.IsAdmin {
			writeErr(w, 403, "admin only")
			return
		}
		who = 0
	}
	list, err := s.Auth.Sessions(r.Context(), who, currentToken(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

// DELETE /api/sessions/{id}: signs one device out; the admin may end anyone's.
func (s *Server) handleEndSession(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	who := me.ID
	if me.IsAdmin {
		who = 0
	}
	ok, err := s.Auth.EndSession(r.Context(), who, r.PathValue("id"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !ok {
		writeErr(w, 404, "no such session")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/sessions: signs out all your other devices.
func (s *Server) handleEndOtherSessions(w http.ResponseWriter, r *http.Request) {
	if err := s.Auth.EndSessions(r.Context(), userFrom(r).ID, currentToken(r)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// checkOwnPassword asks for the signed-in person's password again before
// something sensitive, counted like any other sign-in attempt, as it starts. It
// answers the request itself when the password is refused. The right password
// takes back only this attempt, so it can't clear what wrong codes have built up.
func (s *Server) checkOwnPassword(w http.ResponseWriter, r *http.Request, u *auth.User, password string) bool {
	release, ok := s.signInAllowed(w, r, u.Name)
	if !ok {
		return false
	}
	defer release()
	if !s.Auth.CheckPassword(r.Context(), u.ID, password) {
		s.logins.noteFailure()
		s.logSignInFailure(r, u.Name)
		writeErr(w, 403, "the current password is not right")
		return false
	}
	s.logins.uncount(r, u.Name)
	return true
}

// confirmOwner asks for the signed-in person's password, and code if they have two-factor sign-in,
// before something that a stolen session must not be able to do. It answers the request itself when
// they are refused.
func (s *Server) confirmOwner(w http.ResponseWriter, r *http.Request, password, code string) bool {
	u := userFrom(r)
	return s.checkOwnPassword(w, r, u, password) && (!u.TwoFactor || s.checkOwnCode(w, r, u, code))
}

// checkOwnCode is checkOwnPassword for a two-factor code.
func (s *Server) checkOwnCode(w http.ResponseWriter, r *http.Request, u *auth.User, code string) bool {
	if d := s.logins.attempt(r, u.Name); d > 0 {
		tooManyTries(w, d)
		return false
	}
	if !s.Auth.CheckTwoFactor(r.Context(), u, code) {
		s.logins.noteFailure()
		s.logSignInFailure(r, u.Name)
		writeErr(w, 403, auth.ErrCodeWrong.Error())
		return false
	}
	s.logins.uncount(r, u.Name)
	return true
}

// GET /api/me/2fa
func (s *Server) handleTwoFactor(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	writeJSON(w, 200, map[string]any{"on": u.TwoFactor, "recoveryLeft": s.Auth.RecoveryLeft(r.Context(), u.ID), "available": u.Kind == auth.KindAdult})
}

// POST /api/me/2fa/setup {password}: a new secret for an authenticator app.
func (s *Server) handleTwoFactorSetup(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	u := userFrom(r)
	if !s.checkOwnPassword(w, r, u, b.Password) {
		return
	}
	issuer := s.DB.GetSetting(r.Context(), "brandName", "")
	setup, err := s.Auth.StartTwoFactor(r.Context(), u.ID, issuer)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, setup)
}

// POST /api/me/2fa/confirm {code}: turns it on and hands out the recovery
// codes. Other devices are signed out: whoever else had this account, doesn't.
func (s *Server) handleTwoFactorConfirm(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	u := userFrom(r)
	if d := s.logins.attempt(r, u.Name); d > 0 {
		tooManyTries(w, d)
		return
	}
	codes, err := s.Auth.ConfirmTwoFactor(r.Context(), u.ID, b.Code)
	if err != nil {
		if errors.Is(err, auth.ErrCodeWrong) {
			s.logins.noteFailure()
			s.logSignInFailure(r, u.Name)
		} else {
			s.logins.uncount(r, u.Name) // no code was checked
		}
		writeErr(w, 400, err.Error())
		return
	}
	s.logins.uncount(r, u.Name)
	s.endSessions(r, u.ID, currentToken(r))
	s.Log.InfoContext(r.Context(), "two-factor sign-in turned on", "cat", "auth", "name", u.Name)
	writeJSON(w, 200, map[string]any{"recovery": codes})
}

// POST /api/me/2fa/disable {password, code}: needs both, so a device left
// signed in can't take this protection away.
func (s *Server) handleTwoFactorDisable(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	u := userFrom(r)
	if !s.checkOwnPassword(w, r, u, b.Password) {
		return
	}
	if !s.checkOwnCode(w, r, u, b.Code) {
		return
	}
	if err := s.Auth.DisableTwoFactor(r.Context(), u.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.InfoContext(r.Context(), "two-factor sign-in turned off", "cat", "auth", "name", u.Name)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/users/{id}/2fa (admin): for someone who lost their phone and
// their recovery codes. Administrators use `wittymediaserver reset-2fa` on the server.
func (s *Server) handleUserTwoFactorReset(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok || id == userFrom(r).ID {
		writeErr(w, 400, "use your own security page for your account")
		return
	}
	target, err := s.Auth.User(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "no such account")
		return
	}
	if target.IsAdmin {
		writeErr(w, 403, "another administrator's two-factor sign-in can only be reset on the server itself: wittymediaserver reset-2fa")
		return
	}
	if err := s.Auth.DisableTwoFactor(r.Context(), id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.endSessions(r, id, "")
	s.Log.WarnContext(r.Context(), "two-factor sign-in reset by an administrator", "cat", "auth", "name", target.Name, "user", userFrom(r).Name)
	writeJSON(w, 200, map[string]any{"ok": true})
}
