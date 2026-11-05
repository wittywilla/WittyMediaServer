package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/remote"
)

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	u := s.resolveUser(r)
	ctx := r.Context()
	// The setup wizard continues after the admin account exists, until its last page. It says it's
	// under way with setupDone "0". A server that never said so, because it's older or was set up
	// from a script, only sees it if it has no libraries.
	onboarding := false
	if u != nil && u.IsAdmin {
		switch s.DB.GetSetting(ctx, "setupDone", "") {
		case "1":
		case "0":
			onboarding = true
		default:
			libs, _ := s.Store.Libraries(ctx)
			onboarding = len(libs) == 0
		}
	}
	setupRequired := s.Auth.NoAccounts(r.Context())
	// Admins see a notice when a newer version was found.
	newer := ""
	if u != nil && u.IsAdmin && s.Update != nil {
		newer = s.Update.Available()
	}
	// Which version runs is nobody's business on the internet until they sign in.
	version := s.Version
	if u == nil && clientFrom(r).Where == remote.Internet {
		version = ""
	}
	// An administrator who never saw the first-run notice sees it once (a server
	// set up before it existed, or one set up from a script).
	legalAck := u == nil || !u.IsAdmin || s.legalAcked(ctx)
	// While the owner lets the internet in unencrypted, administrators see it on every page.
	st := s.state()
	plainWarning := ""
	if u != nil && u.IsAdmin && st.on && st.cfg.AllowPlainAway {
		plainWarning = remote.PlainAwayWarning
	}
	writeJSON(w, 200, map[string]any{
		// plainConnection: this very request crossed the internet unencrypted, so a
		// password typed here can be read on the way. The sign-in page says so.
		"plainConnection":  plainAway(r),
		"plainAwayWarning": plainWarning,
		"legalAck":         legalAck,
		"update":           newer,
		"onboarding":       onboarding,
		"pickerLocal":      onThisMachine(r),
		"name":             "WittyMediaServer",
		"version":          version,
		"setupRequired":    setupRequired,
		// Setting up from another device takes the code the server wrote down.
		"setupCodeRequired": setupRequired && s.setupNeedsCode(r),
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
	Code     string `json:"code"` // two-factor sign-in
	// SetupCode: first-run setup from outside this network.
	SetupCode string `json:"setupCode"`
	// AcknowledgedLegal: the setup wizard's first-run notice was ticked.
	AcknowledgedLegal bool `json:"acknowledgedLegal"`
}

// maxCredentialBytes caps every request anyone can send without an account (no name,
// password or pairing code comes near this). See publicBody.
const maxCredentialBytes = 16 << 10

func (s *Server) setCookie(w http.ResponseWriter, r *http.Request, token string) {
	c := &http.Cookie{
		Name: auth.CookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: remote.SecureHint(r), Expires: time.Now().Add(90 * 24 * time.Hour),
	}
	if plainAway(r) {
		// As safe as a cookie gets without HTTPS: never sent by another site's
		// links either, and gone when the session ends.
		c.SameSite, c.Expires = http.SameSiteStrictMode, time.Now().Add(auth.PlainAwayTTL)
		s.notePlainSession(r, token)
	}
	http.SetCookie(w, c)
}

// setupMu makes "is there an account yet?" and creating the first admin one
// step, so two browsers racing through setup cannot both become admin.
var setupMu sync.Mutex

// setupNeedsCode: whoever makes the first account owns the server. On the server's own
// desktop nobody else can be asking. Anywhere else, the home network included, a flatmate,
// a visitor on the Wi-Fi or a hacked device could get there first, so it takes the code
// that only someone with access to the machine can read. WMS_SETUP_FROM_HOME=1 lets the
// home network go without it.
func (s *Server) setupNeedsCode(r *http.Request) bool {
	if s.Cfg != nil && s.Cfg.SetupFromHome && onLocalNetwork(r) {
		return false
	}
	return !onThisMachine(r)
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCredentialBytes)
	var c credentials
	if err := readJSON(r, &c); err != nil { // before the lock: a slow sender mustn't hold up everyone
		writeErr(w, 400, err.Error())
		return
	}
	setupMu.Lock()
	defer setupMu.Unlock()
	if !s.Auth.NoAccounts(r.Context()) {
		writeErr(w, 409, "already set up")
		return
	}
	if s.setupNeedsCode(r) {
		if d := s.logins.begin("setup|"+addressKey(r), freeTriesPerName); d > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
			writeErr(w, 429, "Too many wrong setup codes. Try again later.")
			return
		}
		if !s.setupCodeOK(c.SetupCode) {
			s.Log.WarnContext(r.Context(), "setup refused: wrong setup code", "cat", "auth", "from", addressKey(r))
			writeErr(w, 403, "Setting up this server from another device takes its setup code. It's on the server's console (for Docker, in docker logs), and in setup-code.txt in its data folder.")
			return
		}
	}
	if _, err := s.Auth.CreateUser(r.Context(), c.Name, c.Password, true); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	tok, u, err := s.Auth.Login(r.Context(), c.Name, c.Password, deviceName(c.Device, r), false)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if c.AcknowledgedLegal {
		if err := s.recordLegalAck(r.Context()); err != nil {
			s.Log.Warn("couldn't record the first-run notice", "err", err)
		}
	}
	s.forgetSetupCode()
	s.setCookie(w, r, tok)
	writeJSON(w, 200, map[string]any{"token": tok, "user": u})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCredentialBytes)
	var c credentials
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	release, ok := s.signInAllowed(w, r, c.Name)
	if !ok {
		return
	}
	defer release()
	// Password-free Kids and Guest profiles sign in only at home, whatever the
	// sign-in screen shows.
	allowNoPassword := onLocalNetwork(r) && s.profilesVisible(r)
	tok, u, err := s.Auth.LoginCode(r.Context(), c.Name, c.Password, c.Code, deviceName(c.Device, r), allowNoPassword)
	switch {
	case errors.Is(err, auth.ErrCodeNeeded):
		s.logins.uncount(r, c.Name) // the password was right: nothing was guessed
		writeJSON(w, 401, map[string]any{"error": err.Error(), "twoFactor": true})
		return
	case errors.Is(err, auth.ErrCodeWrong):
		s.logins.noteFailure()
		s.logSignInFailure(r, c.Name)
		writeJSON(w, 401, map[string]any{"error": err.Error(), "twoFactor": true})
		return
	case err != nil:
		s.logins.noteFailure()
		s.logSignInFailure(r, c.Name)
		writeErr(w, 401, err.Error())
		return
	}
	s.logins.passed(r, c.Name)
	s.Log.InfoContext(r.Context(), "signed in", "cat", "auth", "name", u.Name, "from", clientFrom(r).IP.String(), "where", clientFrom(r).Where.String())
	s.setCookie(w, r, tok)
	writeJSON(w, 200, map[string]any{"token": tok, "user": u})
}

// signInAllowed answers 429 when this client has failed too often, or when too
// many passwords are being checked right now. Otherwise it holds one of the
// places for checking a password until release is called. An attempt counts when
// it starts, not when its password turns out wrong: otherwise hundreds of guesses
// sent at once all pass before the first one ends.
func (s *Server) signInAllowed(w http.ResponseWriter, r *http.Request, name string) (release func(), ok bool) {
	if d := s.logins.attempt(r, name); d > 0 {
		tooManyTries(w, d)
		return nil, false
	}
	release, ok = s.loginSlot(w, r)
	if !ok { // nothing was checked
		s.logins.uncount(r, name)
	}
	return release, ok
}

// loginSlot waits for a place among the password checks running at once.
func (s *Server) loginSlot(w http.ResponseWriter, r *http.Request) (release func(), ok bool) {
	t := time.NewTimer(3 * time.Second)
	defer t.Stop()
	select {
	case s.loginSlots <- struct{}{}:
		return func() { <-s.loginSlots }, true
	case <-t.C:
		w.Header().Set("Retry-After", "5")
		writeErr(w, 429, "The server is busy. Try again in a moment.")
		return nil, false
	case <-r.Context().Done():
		return nil, false
	}
}

func tooManyTries(w http.ResponseWriter, d time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
	writeErr(w, 429, fmt.Sprintf("Too many wrong tries. Try again in %s.", waitText(d)))
}

func waitText(d time.Duration) string {
	if d < 2*time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds())+1)
	}
	return fmt.Sprintf("%d minutes", int(d.Minutes())+1)
}

// endSessions signs an account out everywhere after a change meant to do that. A
// failure means devices that should have gone are still signed in.
func (s *Server) endSessions(r *http.Request, id int64, keep string) {
	if err := s.Auth.EndSessions(r.Context(), id, keep); err != nil {
		s.Log.ErrorContext(r.Context(), "could not sign an account out of its other devices", "cat", "auth", "account", id, "err", err)
	}
}

// logSignInFailure writes down a failure that has already been counted.
func (s *Server) logSignInFailure(r *http.Request, name string) {
	s.Log.InfoContext(r.Context(), "sign-in failed", "cat", "auth", "name", limitName(name), "from", clientFrom(r).IP.String(), "where", clientFrom(r).Where.String())
	if d := s.logins.gate(r, name); d > 0 {
		s.Log.WarnContext(r.Context(), "sign-in locked after repeated failures", "cat", "auth", "name", limitName(name), "from", clientFrom(r).IP.String(), "wait", d.Round(time.Second).String())
	}
}

// deviceName is how a session is named in the list of signed-in devices: what
// the app calls itself, or else the browser and system its address reveals.
func deviceName(device string, r *http.Request) string {
	if strings.HasPrefix(device, "Mozilla/") || device == "" {
		device = browserName(r.UserAgent())
	}
	device = strings.Map(func(c rune) rune {
		if c < 32 || c == 127 {
			return -1
		}
		return c
	}, device)
	if len(device) > 80 {
		device = device[:80]
	}
	return device
}

func browserName(ua string) string {
	has := func(s string) bool { return strings.Contains(ua, s) }
	browser := ""
	switch {
	case has("Edg/"):
		browser = "Edge"
	case has("OPR/"):
		browser = "Opera"
	case has("Firefox/"):
		browser = "Firefox"
	case has("Chrome/"):
		browser = "Chrome"
	case has("Safari/"):
		browser = "Safari"
	}
	system := ""
	switch {
	case has("Android"):
		system = "Android"
	case has("iPhone"), has("iPad"):
		system = "iOS"
	case has("Windows"):
		system = "Windows"
	case has("Mac OS X"):
		system = "macOS"
	case has("Linux"):
		system = "Linux"
	}
	switch {
	case browser != "" && system != "":
		return browser + " on " + system
	case browser != "":
		return browser
	case system != "":
		return system
	}
	return "Unknown device"
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	for _, tok := range auth.Tokens(r) { // a stale token can sit in front of the live one
		s.Auth.Logout(r.Context(), tok)
	}
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: remote.SecureHint(r)})
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
		IsAdmin     bool            `json:"isAdmin"`
		Kind        string          `json:"kind"`
		Access      *library.Access `json:"access"`
		DisplayName string          `json:"displayName"`
		Current     string          `json:"current"` // your own password, for a new administrator
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
	if c.IsAdmin && c.Kind == auth.KindAdult && !s.confirmOwner(w, r, c.Current, c.Code) {
		return
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
	if c.DisplayName != "" {
		if err := s.Auth.SetDisplayName(r.Context(), u.ID, c.DisplayName); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if u, err = s.Auth.User(r.Context(), u.ID); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 201, u)
}

// PUT /api/users/{id}/profile {displayName, username, showUsername}: what an account
// is called. Which name the corner shows is anyone's own choice. The names themselves
// are for grown-up accounts to change for themselves, and for whoever manages the
// account, so Kids and Guest profiles keep the names they were given.
func (s *Server) handleUserProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	var in struct {
		DisplayName  *string `json:"displayName"`
		Username     *string `json:"username"`
		ShowUsername *bool   `json:"showUsername"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	me := userFrom(r)
	target, err := s.Auth.User(ctx, id)
	if err != nil {
		writeErr(w, 404, "no such profile")
		return
	}
	self := me.ID == target.ID
	manages := s.canManage(r, me, target.Kind)
	if !self && !manages {
		writeErr(w, 403, "you can't change this profile")
		return
	}
	if (in.DisplayName != nil || in.Username != nil) && !manages && !(self && me.Kind == auth.KindAdult) {
		writeErr(w, 403, "this profile's names are managed by the adults of this server")
		return
	}
	if in.Username != nil && *in.Username != target.Name {
		if err := s.Auth.SetUsername(ctx, id, *in.Username); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	if in.DisplayName != nil {
		if err := s.Auth.SetDisplayName(ctx, id, *in.DisplayName); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if in.ShowUsername != nil {
		if err := s.Auth.SetShowUsername(ctx, id, *in.ShowUsername); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	u, err := s.Auth.User(ctx, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, u)
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
	target, err := s.Auth.User(r.Context(), id)
	if err != nil || !s.canManage(r, userFrom(r), target.Kind) {
		writeErr(w, 403, "not allowed")
		return
	}
	if target.IsAdmin {
		var b struct {
			Current string `json:"current"`
			Code    string `json:"code"`
		}
		if err := readJSON(r, &b); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if !s.confirmOwner(w, r, b.Current, b.Code) {
			return
		}
	}
	if err := s.Auth.DeleteUser(r.Context(), id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.removeUserGameFiles(id)
	if s.TV != nil {
		if err := s.TV.ForgetUser(r.Context(), id); err != nil {
			s.Log.ErrorContext(r.Context(), "could not hand over a removed account's recordings", "cat", "tv", "err", err)
		}
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
	otherAdmin := false
	if id != u.ID {
		if t, err := s.Auth.User(r.Context(), id); err == nil {
			otherAdmin = t.IsAdmin
			allowed = !t.IsAdmin && s.canManage(r, u, t.Kind)
		}
	}
	if !allowed && !(u.IsAdmin && id != u.ID) {
		writeErr(w, 403, "not allowed")
		return
	}
	var c struct {
		Password string `json:"password"`
		Current  string `json:"current"`
		Code     string `json:"code"`
	}
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// Your own password needs the current one, so a session left open on
	// another device can't lock you out of your account. Wrong guesses count
	// like wrong sign-ins, so that session can't be used to guess it either.
	if id == u.ID && !s.checkOwnPassword(w, r, u, c.Current) {
		return
	}
	// Taking over another administrator is what a stolen session would want.
	if otherAdmin && !s.confirmOwner(w, r, c.Current, c.Code) {
		return
	}
	if err := s.Auth.SetPassword(r.Context(), id, c.Password); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.Log.WarnContext(r.Context(), "a password was changed", "cat", "auth", "account", id, "by", u.Name, "from", clientFrom(r).IP.String())
	// Every other session of that account ends (this one stays signed in).
	keep := ""
	if id == u.ID {
		keep = auth.TokenFrom(r)
	}
	s.endSessions(r, id, keep)
	writeJSON(w, 200, map[string]any{"ok": true})
}
