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
// something sensitive, counted like any other sign-in attempt. It answers the
// request itself when the password is refused.
func (s *Server) checkOwnPassword(w http.ResponseWriter, r *http.Request, u *auth.User, password string) bool {
	release, ok := s.loginAllowed(w, r, u.Name)
	if !ok {
		return false
	}
	defer release()
	if !s.Auth.CheckPassword(r.Context(), u.ID, password) {
		s.signInFailed(r, u.Name)
		writeErr(w, 403, "the current password is not right")
		return false
	}
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
	if !s.loginWaitOver(w, r, u.Name) {
		return
	}
	codes, err := s.Auth.ConfirmTwoFactor(r.Context(), u.ID, b.Code)
	if err != nil {
		if errors.Is(err, auth.ErrCodeWrong) {
			s.signInFailed(r, u.Name)
		}
		writeErr(w, 400, err.Error())
		return
	}
	_ = s.Auth.EndSessions(r.Context(), u.ID, currentToken(r))
	s.Log.Info("two-factor sign-in turned on", "cat", "signin", "name", u.Name)
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
	if !s.Auth.CheckTwoFactor(r.Context(), u, b.Code) {
		s.signInFailed(r, u.Name)
		writeErr(w, 403, auth.ErrCodeWrong.Error())
		return
	}
	if err := s.Auth.DisableTwoFactor(r.Context(), u.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.Info("two-factor sign-in turned off", "cat", "signin", "name", u.Name)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/users/{id}/2fa (admin): for someone who lost their phone and
// their recovery codes. Administrators use `wms reset-2fa` on the server.
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
		writeErr(w, 403, "another administrator's two-factor sign-in can only be reset on the server itself: wms reset-2fa")
		return
	}
	if err := s.Auth.DisableTwoFactor(r.Context(), id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.Auth.EndSessions(r.Context(), id, "")
	s.Log.Info("two-factor sign-in reset by an administrator", "cat", "signin", "name", target.Name, "by", userFrom(r).Name)
	writeJSON(w, 200, map[string]any{"ok": true})
}
