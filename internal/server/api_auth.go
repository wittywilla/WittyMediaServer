package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
)

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	u := s.resolveUser(r)
	ctx := r.Context()
	// The setup wizard continues after the admin account exists, until it is
	// finished or skipped (existing servers with libraries never see it).
	onboarding := false
	if u != nil && u.IsAdmin && s.DB.GetSetting(ctx, "setupDone", "") != "1" {
		libs, _ := s.Store.Libraries(ctx)
		onboarding = len(libs) == 0
	}
	writeJSON(w, 200, map[string]any{
		"onboarding":    onboarding,
		"pickerLocal":   onThisMachine(r),
		"name":          "WittyMediaServer",
		"version":       s.Version,
		"setupRequired": s.Auth.UserCount(r.Context()) == 0,
		"user":          u,
		"brandName":     s.DB.GetSetting(r.Context(), "brandName", ""),
		"theme":         s.DB.GetSetting(r.Context(), "theme", "default"),
	})
}

type credentials struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Device   string `json:"device"`
}

func (s *Server) setCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: auth.CookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"), Expires: time.Now().Add(90 * 24 * time.Hour),
	})
}

// setupMu makes "is there an account yet?" and creating the first admin one
// step, so two browsers racing through setup cannot both become admin.
var setupMu sync.Mutex

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	setupMu.Lock()
	defer setupMu.Unlock()
	if s.Auth.UserCount(r.Context()) > 0 {
		writeErr(w, 409, "already set up")
		return
	}
	var c credentials
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if _, err := s.Auth.CreateUser(r.Context(), c.Name, c.Password, true); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	tok, u, err := s.Auth.Login(r.Context(), c.Name, c.Password, c.Device)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.setCookie(w, r, tok)
	writeJSON(w, 200, map[string]any{"token": tok, "user": u})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	k := s.logins.key(r, c.Name)
	if d := s.logins.wait(k); d > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
		writeErr(w, 429, fmt.Sprintf("Too many wrong passwords. Try again in %d seconds.", int(d.Seconds())+1))
		return
	}
	tok, u, err := s.Auth.Login(r.Context(), c.Name, c.Password, c.Device)
	if err != nil {
		s.logins.fail(k)
		writeErr(w, 401, err.Error())
		return
	}
	s.logins.ok(k)
	s.setCookie(w, r, tok)
	writeJSON(w, 200, map[string]any{"token": tok, "user": u})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.Auth.Logout(r.Context(), auth.TokenFrom(r))
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, userFrom(r))
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.Auth.Users(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, users)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var c struct {
		credentials
		IsAdmin bool `json:"isAdmin"`
	}
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	u, err := s.Auth.CreateUser(r.Context(), c.Name, c.Password, c.IsAdmin)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, u)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok || id == userFrom(r).ID {
		writeErr(w, 400, "cannot delete that user")
		return
	}
	if err := s.Auth.DeleteUser(r.Context(), id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	u := userFrom(r)
	if !ok || (id != u.ID && !u.IsAdmin) {
		writeErr(w, 403, "not allowed")
		return
	}
	var c struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.Auth.SetPassword(r.Context(), id, c.Password); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
