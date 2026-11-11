// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/parental"
)

// Profiles on a shared screen, and parental controls: switching from one
// profile to another without signing out, what a profile's own limits are (so
// an app can hide what is off), screen time and asking a parent for more, and
// what a parent can see of a profile's day. The limits themselves are saved
// through PUT /api/users/{id}/access, as they always were.

func (s *Server) parentalRoutes(m *http.ServeMux) {
	a := s.requireUser
	m.Handle("GET /api/profiles/household", a(s.handleHousehold))
	m.Handle("POST /api/profiles/switch", a(s.handleSwitchProfile))
	m.Handle("GET /api/me/limits", a(s.handleMyLimits))
	m.Handle("GET /api/me/screen-time", a(s.handleMyScreenTime))
	m.Handle("POST /api/me/screen-time/extra", a(s.handleAskParent))
	m.Handle("POST /api/users/{id}/screen-time/extra", a(s.handleGiveTime))
	m.Handle("GET /api/users/{id}/activity", a(s.handleProfileActivity))
	m.Handle("DELETE /api/users/{id}/activity", a(s.handleClearProfileActivity))
	m.Handle("GET /api/parental/ratings", a(s.handleRatingSystems))
	m.Handle("GET /api/parental/pin", s.requireAdmin(s.handleParentPIN))
	m.Handle("PUT /api/parental/pin", s.requireAdmin(s.handleSetParentPIN))
}

// parentPINKey holds the parents' PIN, scrambled as passwords are. It isn't
// one of the settings the settings page reads or writes.
const parentPINKey = "parentalPin"

// Limits on what limits may hold: each is a row or a condition in every query
// this account makes.
const (
	maxRuleIDs   = 2000
	maxRuleWords = 200
	maxExtraMin  = 240 // the most a parent gives in one go
)

// cleanAccess checks limits someone sent before they are saved, and puts them
// in order. kind is the kind of profile they are for.
func (s *Server) cleanAccess(r *http.Request, a *library.Access, kind string) error {
	okAge := func(n int) bool { return n >= library.NoAgeLimit && n <= 21 }
	if !okAge(a.MaxAge) {
		return errors.New("the age limit must be between 0 and 21, or -1 for none")
	}
	for k, l := range a.Kinds {
		if !parental.ValidKind(k) {
			return errors.New("limits can be set for film, tv, game, music and book, not for " + k)
		}
		if !okAge(l.MaxAge) {
			return errors.New("the age limit for " + k + " must be between 0 and 21, or -1 for none")
		}
	}
	for _, list := range [][]int64{a.Libraries, a.AllowItems, a.BlockItems, a.AllowShows, a.BlockShows, a.AllowCollections, a.BlockCollections} {
		if len(list) > maxRuleIDs {
			return errors.New("that is too many titles for one profile's limits")
		}
		for _, id := range list {
			if id <= 0 {
				return errors.New("limits name titles by their numbers")
			}
		}
	}
	for _, list := range []*[]string{&a.BlockGenres, &a.OnlyGenres, &a.BlockTags} {
		if len(*list) > maxRuleWords {
			return errors.New("that is too many genres or tags for one profile's limits")
		}
		out := (*list)[:0]
		for _, w := range *list {
			w = strings.TrimSpace(w)
			if w == "" || len(w) > 100 || strings.ContainsAny(w, "|%_") {
				continue // genres are kept joined with |, and looked for with LIKE
			}
			out = append(out, w)
		}
		*list = out
	}
	off := []string{}
	for _, f := range library.Features {
		if !a.Can(f) {
			off = append(off, f)
		}
	}
	for _, f := range a.Off {
		known := false
		for _, k := range library.Features {
			known = known || k == f
		}
		if !known {
			return errors.New("there is no feature called " + f)
		}
	}
	a.Off = off
	// A Kids or Guest profile has its own name and picture only when given it (see ForKind).
	a.On = nil
	if kind != auth.KindAdult && a.Can(library.FeatureEditProfile) {
		a.On = []string{library.FeatureEditProfile}
	}
	if a.Time.Empty() {
		a.Time = nil
	} else if err := a.Time.Valid(); err != nil {
		return err
	}
	// Only a collection that lists its titles can be a limit: one made from a
	// rule holds whatever matches today.
	for _, list := range []*[]int64{&a.AllowCollections, &a.BlockCollections} {
		out := (*list)[:0]
		for _, id := range *list {
			var kind string
			if err := s.DB.QueryRowContext(r.Context(), `SELECT kind FROM collections WHERE id = ?`, id).Scan(&kind); err == nil && kind == "manual" {
				out = append(out, id)
			}
		}
		*list = out
	}
	return nil
}

// mergeAccess reads limits sent as JSON over the ones an account has, so an
// app written before a limit existed doesn't clear it by not sending it.
func mergeAccess(base library.Access, raw json.RawMessage) (*library.Access, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil, errors.New("limits are a JSON object")
	}
	// What is sent replaces what was there; a map or an object read over an old
	// one would keep the old one's other entries.
	if _, ok := keys["kinds"]; ok {
		base.Kinds = nil
	}
	if _, ok := keys["time"]; ok {
		base.Time = nil
	}
	if err := json.Unmarshal(raw, &base); err != nil {
		return nil, errors.New("those limits can't be read: " + err.Error())
	}
	return &base, nil
}

// GET /api/profiles/household: the profiles this device can switch to. The
// same list as the sign-in screen's, with the same rule for who sees it:
// where that screen shows profiles, and for grown-up accounts anywhere.
func (s *Server) handleHousehold(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	type profile struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Kind        string `json:"kind"`
		HasPassword bool   `json:"hasPassword"`
		Avatar      string `json:"avatar"`
		Current     bool   `json:"current"`
	}
	out := []profile{}
	if s.profilesVisible(r) || me.Kind == auth.KindAdult {
		users, err := s.Auth.Users(r.Context())
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		tap := onLocalNetwork(r) && s.profilesVisible(r)
		for _, u := range users {
			out = append(out, profile{ID: u.ID, Name: u.Name, DisplayName: u.DisplayName, Kind: u.Kind, HasPassword: u.HasPassword || !tap, Avatar: u.Avatar, Current: u.ID == me.ID})
		}
	}
	writeJSON(w, 200, map[string]any{"profiles": out})
}

// POST /api/profiles/switch {name, password, code, device}: this device goes
// from the profile it is signed in to, to another. It is signing in as that
// profile, with everything signing in asks for (its password or PIN, its code,
// the same count of wrong tries), and then signing out of the one before, so a
// device is never signed in to two at once and a profile left behind can't be
// gone back to without its own password.
func (s *Server) handleSwitchProfile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCredentialBytes)
	var c credentials
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	me := userFrom(r)
	if strings.EqualFold(strings.TrimSpace(c.Name), me.Name) {
		writeErr(w, 400, "this device is already signed in to that profile")
		return
	}
	release, ok := s.signInAllowed(w, r, c.Name)
	if !ok {
		return
	}
	defer release()
	allowNoPassword := onLocalNetwork(r) && s.profilesVisible(r)
	tok, u, err := s.Auth.LoginCode(r.Context(), c.Name, c.Password, c.Code, deviceName(c.Device, r), allowNoPassword)
	switch {
	case errors.Is(err, auth.ErrCodeNeeded):
		s.logins.uncount(r, c.Name)
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
		// 403, not 401: the device is still signed in to the profile it had.
		writeErr(w, 403, err.Error())
		return
	}
	s.logins.passed(r, c.Name)
	if !remoteAllowed(r, u) {
		s.Auth.Logout(r.Context(), tok)
		writeErr(w, 403, featureRefusal[library.FeatureRemote])
		return
	}
	if old := tokenFrom(r); old != "" {
		s.Auth.Logout(r.Context(), old)
	}
	s.Log.InfoContext(r.Context(), "switched profile", "cat", "auth", "from_name", me.Name, "name", u.Name, "from", clientFrom(r).IP.String(), "where", clientFrom(r).Where.String())
	s.setCookie(w, r, tok)
	writeJSON(w, 200, map[string]any{"token": tok, "user": u})
}

// myScreenTime is the signed-in profile's time right now.
func (s *Server) myScreenTime(r *http.Request, u *auth.User, a *library.Access) parental.Verdict {
	if a == nil || a.Time.Empty() {
		return parental.Verdict{Allowed: true, AllowanceSec: -1, RemainingSec: -1}
	}
	return a.Time.Check(s.clock(), s.parentalInit().tracker.Usage(r.Context(), u.ID))
}

// GET /api/me/limits: what this profile may do, for an app to hide what is
// off. It names no titles: what is hidden stays unnamed here too.
func (s *Server) handleMyLimits(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	a := accessFor(u)
	features := map[string]bool{}
	for _, f := range library.Features {
		features[f] = a.Can(f)
	}
	// What the kind of profile decides, whatever its limits say: Kids and Guest
	// profiles read comments and don't write them, and make no share links.
	features["writeComments"] = features[library.FeatureComments] && canPost(u)
	features[library.FeatureSharing] = features[library.FeatureSharing] && canPost(u)
	features["liveTv"] = a == nil || a.LiveTV
	out := map[string]any{
		"limited": a != nil, "kind": u.Kind, "features": features, "maxAge": library.NoAgeLimit, "allowUnrated": true,
		"kinds": map[string]library.KindLimit{}, "time": nil, "screenTime": screenTimeView(s.myScreenTime(r, u, a)),
		"parentPin": s.DB.GetSetting(r.Context(), parentPINKey, "") != "",
	}
	if a != nil {
		out["maxAge"], out["allowUnrated"] = a.MaxAge, a.AllowUnrated
		if a.Kinds != nil {
			out["kinds"] = a.Kinds
		}
		if !a.Time.Empty() {
			out["time"] = a.Time
		}
	}
	writeJSON(w, 200, out)
}

// GET /api/me/screen-time: how long this profile may still play today.
func (s *Server) handleMyScreenTime(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	writeJSON(w, 200, screenTimeView(s.myScreenTime(r, u, accessFor(u))))
}

func extraMinutes(n int) (int, bool) { return n, n >= 1 && n <= maxExtraMin }

// POST /api/me/screen-time/extra {minutes, pin} or {minutes, name, password}:
// "ask a parent". Someone who manages this profile types the parents' PIN, or
// their own name and password, on the child's device, and the profile gets
// that many minutes more today. Wrong tries count as wrong sign-ins do, for
// this profile and for the address, so the PIN can't be guessed through.
func (s *Server) handleAskParent(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCredentialBytes)
	var b struct {
		Minutes  int    `json:"minutes"`
		PIN      string `json:"pin"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	me := userFrom(r)
	minutes, ok := extraMinutes(b.Minutes)
	if !ok {
		writeErr(w, 400, "choose between 1 and 240 minutes")
		return
	}
	ctx := r.Context()
	by := ""
	switch {
	case b.PIN != "":
		hash := s.DB.GetSetting(ctx, parentPINKey, "")
		// Counted for the profile and for the address, so neither a new address nor
		// another profile gets a fresh set of tries.
		keys := []string{"parentpin|u" + limitName(me.Name), "parentpin|" + addressKey(r)}
		for _, k := range keys {
			if d := s.logins.wait(k); d > 0 {
				tooManyTries(w, d)
				return
			}
		}
		release, ok := s.loginSlot(w, r)
		if !ok {
			return
		}
		good := auth.CheckSecret(hash, b.PIN)
		release()
		if !good {
			for _, k := range keys {
				s.logins.fail(k)
			}
			s.logins.noteFailure()
			s.Log.InfoContext(ctx, "a wrong parents' PIN", "cat", "auth", "name", me.Name, "from", clientFrom(r).IP.String())
			writeErr(w, 403, "That isn't the parents' PIN.")
			return
		}
		for _, k := range keys {
			s.logins.ok(k)
		}
		by = "PIN"
	case b.Name != "" && b.Password != "":
		release, ok := s.signInAllowed(w, r, b.Name)
		if !ok {
			return
		}
		parent := s.parentNamed(r, b.Name, me)
		good := parent != nil && s.Auth.CheckPassword(ctx, parent.ID, b.Password)
		if parent == nil {
			auth.CheckSecret("", b.Password) // as long as a real check, so names can't be told apart
		}
		release()
		if !good {
			s.logins.noteFailure()
			s.logSignInFailure(r, b.Name)
			writeErr(w, 403, "That name and password aren't those of someone who manages this profile.")
			return
		}
		s.logins.uncount(r, b.Name)
		by = parent.Name
	default:
		writeErr(w, 400, "it takes the parents' PIN, or the name and password of someone who manages this profile")
		return
	}
	if err := s.grantExtra(ctx, me.ID, minutes, by); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.InfoContext(ctx, "extra screen time given", "cat", "auth", "name", me.Name, "minutes", minutes, "by", by)
	writeJSON(w, 200, screenTimeView(s.myScreenTime(r, me, accessFor(me))))
}

// parentNamed finds the account of that name if it manages child.
func (s *Server) parentNamed(r *http.Request, name string, child *auth.User) *auth.User {
	users, err := s.Auth.Users(r.Context())
	if err != nil {
		return nil
	}
	for i := range users {
		p := &users[i]
		if strings.EqualFold(p.Name, strings.TrimSpace(name)) && p.ID != child.ID && !child.IsAdmin && s.canManage(r, p, child.Kind) {
			return p
		}
	}
	return nil
}

// managed loads the profile a route names if the caller manages it, and
// answers the request when they don't.
func (s *Server) managed(w http.ResponseWriter, r *http.Request) *auth.User {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil
	}
	target, err := s.Auth.User(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "no such profile")
		return nil
	}
	me := userFrom(r)
	if target.ID == me.ID || target.IsAdmin || !s.canManage(r, me, target.Kind) {
		writeErr(w, 403, "not allowed")
		return nil
	}
	return target
}

// POST /api/users/{id}/screen-time/extra {minutes}: whoever manages a profile
// gives it more time today, from their own device.
func (s *Server) handleGiveTime(w http.ResponseWriter, r *http.Request) {
	target := s.managed(w, r)
	if target == nil {
		return
	}
	var b struct {
		Minutes int `json:"minutes"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	minutes, ok := extraMinutes(b.Minutes)
	if !ok {
		writeErr(w, 400, "choose between 1 and 240 minutes")
		return
	}
	if err := s.grantExtra(r.Context(), target.ID, minutes, userFrom(r).Name); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, screenTimeView(s.myScreenTime(r, target, accessFor(target))))
}

// GET /api/users/{id}/activity?days=14: what a profile you manage watched and
// when, how much screen time it had each day, and what it asked for and was
// refused. Kept only for profiles with limits.
func (s *Server) handleProfileActivity(w http.ResponseWriter, r *http.Request) {
	target := s.managed(w, r)
	if target == nil {
		return
	}
	ctx := r.Context()
	days := min(max(qInt(r, "days", 14), 1), 90)
	now := s.clock()
	s.parentalInit().tracker.Forget(ctx, target.ID) // write down what is still held, so today is whole
	since := parental.DayNumber(now.AddDate(0, 0, -(days - 1)))
	type dayRow struct {
		Day       int `json:"day"`
		ScreenSec int `json:"screenSec"`
		ExtraSec  int `json:"extraSec"`
	}
	type watchRow struct {
		Day       int     `json:"day"`
		ItemID    int64   `json:"itemId"`
		Title     string  `json:"title"`
		ShowTitle string  `json:"showTitle,omitempty"`
		Kind      string  `json:"kind"`
		Seconds   float64 `json:"seconds"`
		LastAt    int64   `json:"lastAt"`
	}
	type eventRow struct {
		At     int64  `json:"at"`
		Kind   string `json:"kind"`
		ItemID int64  `json:"itemId,omitempty"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}
	out := struct {
		Days       []dayRow       `json:"days"`
		Watched    []watchRow     `json:"watched"`
		Events     []eventRow     `json:"events"`
		ScreenTime map[string]any `json:"screenTime"`
	}{Days: []dayRow{}, Watched: []watchRow{}, Events: []eventRow{}, ScreenTime: screenTimeView(s.myScreenTime(r, target, accessFor(target)))}
	rows, err := s.DB.QueryContext(ctx, `SELECT day, seconds, extra_seconds FROM screen_time WHERE user_id = ? AND day >= ? ORDER BY day DESC`, target.ID, since)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for rows.Next() {
		var d dayRow
		if err := rows.Scan(&d.Day, &d.ScreenSec, &d.ExtraSec); err == nil {
			out.Days = append(out.Days, d)
		}
	}
	rows.Close()
	rows, err = s.DB.QueryContext(ctx, `SELECT w.day, w.item_id, w.title, IFNULL(sh.title, ''), w.kind, w.seconds, w.last_at
		FROM profile_watch w LEFT JOIN items i ON i.id = w.item_id LEFT JOIN shows sh ON sh.id = i.show_id
		WHERE w.user_id = ? AND w.day >= ? ORDER BY w.day DESC, w.last_at DESC LIMIT 2000`, target.ID, since)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for rows.Next() {
		var x watchRow
		if err := rows.Scan(&x.Day, &x.ItemID, &x.Title, &x.ShowTitle, &x.Kind, &x.Seconds, &x.LastAt); err == nil {
			out.Watched = append(out.Watched, x)
		}
	}
	rows.Close()
	rows, err = s.DB.QueryContext(ctx, `SELECT at, kind, item_id, title, detail FROM profile_events WHERE user_id = ? AND at >= ? ORDER BY id DESC LIMIT ?`,
		target.ID, now.AddDate(0, 0, -days).Unix(), eventsKept)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for rows.Next() {
		var e eventRow
		if err := rows.Scan(&e.At, &e.Kind, &e.ItemID, &e.Title, &e.Detail); err == nil {
			out.Events = append(out.Events, e)
		}
	}
	rows.Close()
	writeJSON(w, 200, out)
}

// DELETE /api/users/{id}/activity: forgets what was written down about a
// profile you manage. Today's screen time stays, or clearing would reset it.
func (s *Server) handleClearProfileActivity(w http.ResponseWriter, r *http.Request) {
	target := s.managed(w, r)
	if target == nil {
		return
	}
	ctx := r.Context()
	today := parental.DayNumber(s.clock())
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`DELETE FROM profile_watch WHERE user_id = ?`, []any{target.ID}},
		{`DELETE FROM profile_events WHERE user_id = ?`, []any{target.ID}},
		{`DELETE FROM screen_time WHERE user_id = ? AND day <> ?`, []any{target.ID, today}},
	} {
		if _, err := s.DB.ExecContext(ctx, q.sql, q.args...); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/parental/ratings: the rating systems Settings offers, and what a
// profile can be given or kept from. Nothing here is anyone's own.
func (s *Server) handleRatingSystems(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"kinds": parental.Kinds, "systems": parental.Systems, "features": library.Features,
		"avatars": auth.AvatarNames, "avatarColours": auth.AvatarColours, "maxExtraMinutes": maxExtraMin,
	})
}

// GET /api/parental/pin (admin): whether the parents' PIN is set.
func (s *Server) handleParentPIN(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"set": s.DB.GetSetting(r.Context(), parentPINKey, "") != ""})
}

// PUT /api/parental/pin (admin) {pin, current, code}: sets the PIN a parent
// types on a child's device to give more time, or removes it with pin "". It
// asks for the administrator's own password, as a device left signed in mustn't
// be enough to choose it.
func (s *Server) handleSetParentPIN(w http.ResponseWriter, r *http.Request) {
	var b struct {
		PIN     string `json:"pin"`
		Current string `json:"current"`
		Code    string `json:"code"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.PIN != "" {
		if len(b.PIN) < auth.MinPIN || len(b.PIN) > 12 || strings.Trim(b.PIN, "0123456789") != "" {
			writeErr(w, 400, "the PIN is 4 to 12 digits")
			return
		}
	}
	if !s.confirmOwner(w, r, b.Current, b.Code) {
		return
	}
	hash := ""
	if b.PIN != "" {
		var err error
		if hash, err = auth.HashSecret(b.PIN); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if err := s.DB.SetSetting(r.Context(), parentPINKey, hash); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.WarnContext(r.Context(), "the parents' PIN was changed", "cat", "auth", "by", userFrom(r).Name, "from", clientFrom(r).IP.String(), "set", hash != "")
	writeJSON(w, 200, map[string]any{"set": hash != ""})
}
