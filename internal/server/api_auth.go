package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
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
	setupRequired := s.Auth.UserCount(r.Context()) == 0
	// Admins see a notice when a newer version was found.
	newer := ""
	if u != nil && u.IsAdmin && s.Update != nil {
		newer = s.Update.Available()
	}
	writeJSON(w, 200, map[string]any{
		"update":        newer,
		"onboarding":    onboarding,
		"pickerLocal":   onThisMachine(r),
		"name":          "WittyMediaServer",
		"version":       s.Version,
		"setupRequired": setupRequired,
		// Setting up from outside this network takes the code from the log.
		"setupCodeRequired": setupRequired && !onLocalNetwork(r),
		"user":              u,
		"brandName":         s.DB.GetSetting(r.Context(), "brandName", ""),
		"brandLogo":         s.DB.GetSetting(r.Context(), "brandLogo", ""),
		"theme":             s.DB.GetSetting(r.Context(), "theme", "default"),
	})
}

type credentials struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Device   string `json:"device"`
	// SetupCode: first-run setup from outside this network.
	SetupCode string `json:"setupCode"`
}

// maxCredentialBytes caps sign-in and setup requests (no name or password
// comes near this); they are the only ones anyone can send without an account.
const maxCredentialBytes = 16 << 10

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
	r.Body = http.MaxBytesReader(w, r.Body, maxCredentialBytes)
	var c credentials
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !onLocalNetwork(r) {
		if d := s.logins.begin("setup|"+addressKey(r), freeTriesPerName); d > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
			writeErr(w, 429, "Too many wrong setup codes. Try again later.")
			return
		}
		if !s.setupCodeOK(c.SetupCode) {
			s.Log.Warn("setup refused: wrong setup code", "from", addressKey(r))
			writeErr(w, 403, "Setting up this server from another network takes its setup code. You'll find it in the server's log.")
			return
		}
	}
	if _, err := s.Auth.CreateUser(r.Context(), c.Name, c.Password, true); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	tok, u, err := s.Auth.Login(r.Context(), c.Name, c.Password, c.Device, false)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.setCookie(w, r, tok)
	writeJSON(w, 200, map[string]any{"token": tok, "user": u})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCredentialBytes)
	var c credentials
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	tooMany := func(d time.Duration) {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
		writeErr(w, 429, fmt.Sprintf("Too many wrong passwords. Try again in %d seconds.", int(d.Seconds())+1))
	}
	k := s.logins.key(r, c.Name)
	if d := s.logins.begin(k, freeTriesPerName); d > 0 {
		tooMany(d)
		return
	}
	// Behind a reverse proxy every visitor has the proxy's address, so one
	// address's count would lock everybody out.
	addr := ""
	if !proxied(r) {
		addr = addressKey(r)
		if d := s.logins.begin(addr, freeTriesPerAddress); d > 0 {
			tooMany(d)
			return
		}
	}
	if !acquirePasswordSlot(r.Context()) {
		w.Header().Set("Retry-After", "5")
		writeErr(w, 503, "The server is busy. Try again in a moment.")
		return
	}
	tok, u, err := s.Auth.Login(r.Context(), c.Name, c.Password, c.Device, s.profilesVisible(r))
	<-passwordSlots
	if err != nil {
		writeErr(w, 401, err.Error())
		return
	}
	s.logins.ok(k)
	if addr != "" {
		s.logins.refund(addr)
	}
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

// GET /api/me/ui: the account's interface choices (home widget order...).
func (s *Server) handleGetUI(w http.ResponseWriter, r *http.Request) {
	raw := s.Auth.UI(r.Context(), userFrom(r).ID)
	if raw == "" {
		raw = "{}"
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(raw))
}

// PUT /api/me/ui: replaces them. Any JSON object up to 16 KB.
func (s *Server) handlePutUI(w http.ResponseWriter, r *http.Request) {
	var v map[string]any
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		writeErr(w, 400, "expected a JSON object of at most 16 KB")
		return
	}
	b, _ := json.Marshal(v)
	if err := s.Auth.SetUI(r.Context(), userFrom(r).ID, string(b)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, v)
}

// userView is an account as the Users page sees it, with its limits.
type userView struct {
	auth.User
	Access *library.Access `json:"access"` // nil = no limits
}

// canManage reports whether me may change account target: the admin
// always; adults may manage Kids profiles unless the admin locked that.
func (s *Server) canManage(r *http.Request, me *auth.User, targetKind string) bool {
	if me.IsAdmin {
		return true
	}
	return me.Kind == auth.KindAdult && targetKind == auth.KindKids && !accessFor(me).Limited() && s.DB.GetSetting(r.Context(), "kidsManagedBy", "adults") == "adults"
}

// GET /api/users: everyone for the admin; the Kids profiles for adults who
// may manage them.
func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	users, err := s.Auth.Users(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := []userView{}
	for _, u := range users {
		if !s.canManage(r, me, u.Kind) {
			continue
		}
		v := userView{User: u, Access: accessFor(&u)}
		if v.Access == nil && !u.IsAdmin {
			a := library.DefaultAccess(u.Kind)
			v.Access = &a
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var c struct {
		credentials
		IsAdmin bool            `json:"isAdmin"`
		Kind    string          `json:"kind"`
		Access  *library.Access `json:"access"`
	}
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if c.Kind == "" {
		c.Kind = auth.KindAdult
	}
	me := userFrom(r)
	if !s.canManage(r, me, c.Kind) {
		writeErr(w, 403, "only the administrator can add this kind of profile")
		return
	}
	if !me.IsAdmin {
		c.IsAdmin = false
	}
	restr := ""
	if c.Access != nil || c.Kind != auth.KindAdult {
		a := library.DefaultAccess(c.Kind)
		if c.Access != nil {
			a = *c.Access
		}
		b, _ := json.Marshal(a)
		restr = string(b)
	}
	u, err := s.Auth.CreateProfile(r.Context(), c.Name, c.Password, c.Kind, c.IsAdmin, restr)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, u)
}

// PUT /api/users/{id}/access {access, clearPassword}: an account's limits.
// Kids profiles: the admin, or adults when allowed. Everyone else: admin.
func (s *Server) handleUserAccess(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	target, err := s.Auth.User(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "no such account")
		return
	}
	me := userFrom(r)
	if target.ID == me.ID || target.IsAdmin || !s.canManage(r, me, target.Kind) {
		writeErr(w, 403, "not allowed")
		return
	}
	var b struct {
		Access        *library.Access `json:"access"`
		ClearPassword bool            `json:"clearPassword"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.Access != nil {
		raw, _ := json.Marshal(b.Access)
		if err := s.Auth.SetRestrictions(r.Context(), id, string(raw)); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if b.ClearPassword {
		if err := s.Auth.ClearPassword(r.Context(), id); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	u, _ := s.Auth.User(r.Context(), id)
	writeJSON(w, 200, userView{User: *u, Access: accessFor(u)})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok || id == userFrom(r).ID {
		writeErr(w, 400, "cannot delete that user")
		return
	}
	if target, err := s.Auth.User(r.Context(), id); err != nil || !s.canManage(r, userFrom(r), target.Kind) {
		writeErr(w, 403, "not allowed")
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
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	// Your own password (adults), or one you manage. Kids and Guest
	// profiles cannot change theirs: the people who manage them do.
	allowed := id == u.ID && u.Kind == auth.KindAdult
	if id != u.ID {
		if t, err := s.Auth.User(r.Context(), id); err == nil && !t.IsAdmin && s.canManage(r, u, t.Kind) {
			allowed = true
		}
	}
	if !allowed && !(u.IsAdmin && id != u.ID) {
		writeErr(w, 403, "not allowed")
		return
	}
	var c struct {
		Password string `json:"password"`
		Current  string `json:"current"`
	}
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// Your own password needs the current one, so a session left open on
	// another device can't lock you out of your account.
	if id == u.ID && !s.Auth.CheckPassword(r.Context(), id, c.Current) {
		writeErr(w, 403, "the current password is not right")
		return
	}
	if err := s.Auth.SetPassword(r.Context(), id, c.Password); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// Every other session of that account ends (this one stays signed in).
	keep := ""
	if id == u.ID {
		keep = auth.TokenFrom(r)
	}
	_ = s.Auth.EndSessions(r.Context(), id, keep)
	writeJSON(w, 200, map[string]any{"ok": true})
}
