// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/totp"
)

// Pairing v2 (/api/auth/pair): the same pairing as /api/pair, one store and one
// path through it, plus a PIN that proves the person approving is looking at the
// device's screen right now.
//
// The device starts a session and shows a QR code and a PIN. The PIN is a TOTP
// (RFC 6238: HMAC-SHA1, 6 digits, 30-second steps) from a 160-bit secret that
// never leaves the server's memory, so it changes every 30 seconds. The person
// approving scans the QR code, or opens the page, and types the PIN they see.
// The PIN shown now and the one before it are accepted (someone may still be
// typing when it changes; the server made both, so there is no clock drift to
// allow for and a PIN not yet shown is never right). A PIN works once, and five
// wrong ones end the session. The whole session lasts five minutes.
//
// What it defends against, and how:
//
//   - someone guessing: a session is found by its 128-bit ID or its 34-bit code,
//     both needing a signed-in adult; then 2 PINs in 1,000,000 are right and 5
//     tries end the session, so 1 in 100,000 per session, with the person's own
//     limit on wrong codes on top;
//   - someone who saw the screen earlier (a photo of the QR code): the PIN has
//     changed, and the QR code alone approves nothing;
//   - someone replaying an approval: a session is answered once, a PIN works
//     for one step once, and the sign-in is collected once;
//   - someone else collecting the sign-in: only the device that started the
//     session holds its secret (256 bits, kept hashed), compared in constant time;
//   - a stolen session approving a device for good: the approval stands only
//     while the approving sign-in does, and signing a device in as an
//     administrator asks for that administrator's password again;
//   - handing out more than the approver has: the device signs in as the approver
//     or a Kids or Guest profile they manage, never another adult or administrator.

const (
	authPairTTL      = 5 * time.Minute
	authPairMaxTries = 5
	// pairSecretHeader carries the device's secret on GET requests, which have no
	// body: a header, so it stays out of addresses and logs.
	pairSecretHeader = "X-WMS-Pair-Secret"
	// qrCheckBytes: how much of the HMAC the QR code carries, base64url.
	qrCheckBytes = 12
)

// Why a PIN approval failed. Part of the API: apps may word it their own way.
const (
	pinWrong    = "wrong-pin"
	pinBadCheck = "bad-check"
	pinLocked   = "locked"
	pinNotFound = "not-found"
	pinExpired  = "expired"
	pinUsed     = "used"
)

var pinFailures = map[string]struct {
	status int
	msg    string
}{
	pinWrong:    {400, "That PIN isn't right. Type the PIN the device shows now."},
	pinBadCheck: {400, "That QR code doesn't belong to this sign-in. Scan the code on the device again."},
	pinLocked:   {410, "Too many wrong PINs. Start again on the device."},
	pinNotFound: {404, "That sign-in wasn't found. Start again on the device."},
	pinExpired:  {410, "That sign-in has expired. Start again on the device."},
	pinUsed:     {410, "That sign-in has already been answered."},
}

// startV2 opens a pairing with a PIN. It goes through the same limits as a plain code.
func (ps *pairings) startV2(device, platform, ip, key string, away bool, serverID, base string) (*pairing, string, error) {
	pinKey, err := totp.NewSecret()
	if err != nil {
		return nil, "", err
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", err
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.qrKey == nil {
		ps.qrKey = make([]byte, 32)
		if _, err := rand.Read(ps.qrKey); err != nil {
			ps.qrKey = nil
			return nil, "", err
		}
	}
	p, secret, err := ps.startLocked(device, platform, ip, key, away)
	if err != nil {
		return nil, "", err
	}
	id := hex.EncodeToString(raw)
	for ps.byID[id] != nil { // 128 bits: never, but cheap to be sure
		if _, err := rand.Read(raw); err != nil {
			ps.forgetLocked(p)
			return nil, "", err
		}
		id = hex.EncodeToString(raw)
	}
	p.id, p.pinKey, p.pinUsed = id, pinKey, -1
	p.expires = p.created.Add(authPairTTL)
	p.qrCheck = qrCheck(ps.qrKey, serverID, base, id)
	ps.byID[id] = p
	return p, secret, nil
}

// qrCheck is the k= in a QR code: an HMAC over what the code says, under a key only this
// server holds. An app sends it back when approving, and an altered code is refused.
func qrCheck(key []byte, serverID, base, id string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("wms-pair-v1\n" + serverID + "\n" + base + "\n" + id))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)[:qrCheckBytes])
}

// qrPayload is what the QR code says. Fields in a fixed order, values escaped.
func qrPayload(serverID, base, id, check string) string {
	return "wms://pair?v=1&s=" + url.QueryEscape(serverID) + "&u=" + url.QueryEscape(base) + "&c=" + url.QueryEscape(id) + "&k=" + url.QueryEscape(check)
}

// ownedLocked is the v2 session id if secret is its device's secret, else nil.
func (ps *pairings) ownedLocked(id, secret string) *pairing {
	p := ps.byID[id]
	if p == nil || subtle.ConstantTimeCompare([]byte(p.secret), []byte(hashSecret(secret))) != 1 {
		return nil
	}
	return p
}

// pinNow is the PIN to show for a session, for its own device only.
func (ps *pairings) pinNow(id, secret string) (pin string, step int64, p *pairing, ok bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	p = ps.ownedLocked(id, secret)
	if p == nil {
		return "", 0, nil, false
	}
	if ps.stillWaitingLocked(p) == nil {
		return "", 0, p, false
	}
	now := ps.now()
	step = totp.Step(now)
	return totp.At(p.pinKey, step), step, p, true
}

// pollV2 is poll, by session and secret.
func (ps *pairings) pollV2(id, secret string) (int, *pairing) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.pollLocked(ps.ownedLocked(id, secret))
}

// approveWithPIN answers a waiting pairing if the PIN (and the QR check, when given) is
// right, all at once: nobody can slip in between the check and the answer. lookup is a
// session ID, or a plain code when byCode. It answers why not, and how many tries are left.
func (ps *pairings) approveWithPIN(lookup string, byCode bool, pin, check string, userID, approver int64, session string) (*pairing, string, int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	var p *pairing
	if byCode {
		p = ps.byCode[cleanPairCode(lookup)]
	} else {
		p = ps.byID[lookup]
	}
	now := ps.now()
	switch {
	case p == nil:
		return nil, pinNotFound, 0
	case p.locked:
		return nil, pinLocked, 0
	case now.After(p.expires):
		return nil, pinExpired, 0
	case p.userID != 0 || p.denied:
		return nil, pinUsed, 0
	}
	if p.pinKey != nil {
		reason := ""
		if check != "" && !hmac.Equal([]byte(check), []byte(p.qrCheck)) {
			reason = pinBadCheck
		} else if step, ok := totp.VerifyWindow(p.pinKey, cleanInviteCode(pin), now, p.pinUsed, 1, 0); ok {
			p.pinUsed = step
		} else {
			reason = pinWrong
		}
		if reason != "" {
			p.tries++
			if p.tries >= authPairMaxTries {
				p.locked = true
				// The device has a little time left to be told.
				if until := now.Add(2 * time.Minute); until.Before(p.expires) {
					p.expires = until
				}
				return nil, pinLocked, 0
			}
			return nil, reason, authPairMaxTries - p.tries
		}
	}
	ps.answerLocked(p, userID, approver, session)
	return p, "", 0
}

// waitingByID is a live, unanswered v2 session.
func (ps *pairings) waitingByID(id string) *pairing {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.stillWaitingLocked(ps.byID[id])
}

// denyByID refuses a waiting v2 session.
func (ps *pairings) denyByID(id string) *pairing {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	p := ps.stillWaitingLocked(ps.byID[id])
	if p != nil {
		ps.answerLocked(p, 0, 0, "")
	}
	return p
}

// pairBaseURL is where this server is, as the device starting a pairing reached it: the
// address it used at home, or the server's outside address when that is known and it is
// away. "" when no usable address is known; then only relative links are given.
func (s *Server) pairBaseURL(r *http.Request) string {
	c := clientFrom(r)
	public := s.state().publicURL
	if !c.Where.AtHome() && public != "" {
		return public
	}
	host := r.Host
	if c.ForwardedHost != "" {
		host = c.ForwardedHost
	}
	if host == "" || strings.ContainsAny(host, "/?#@\\ \t\r\n\"'<>") {
		return public
	}
	scheme := "http"
	if c.Secure || r.TLS != nil {
		scheme = "https"
	}
	u, err := url.Parse(scheme + "://" + host)
	if err != nil || u.Host != host {
		return public
	}
	return scheme + "://" + host
}

func pairSecretFrom(r *http.Request) string {
	if s := r.Header.Get(pairSecretHeader); len(s) == 64 {
		return s
	}
	return ""
}

// POST /api/auth/pair {device, platform} (public): start a session, to show as a QR code
// and a PIN.
func (s *Server) handleAuthPairStart(w http.ResponseWriter, r *http.Request) {
	device, platform, ok := s.pairStartRequest(w, r)
	if !ok {
		return
	}
	id, err := s.Identity()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	base := s.pairBaseURL(r)
	p, secret, err := s.pair.startV2(device, platform, ipOf(r), addressKey(r), !clientFrom(r).Where.AtHome(), id.ID, base)
	if !pairStartOK(w, err) {
		return
	}
	s.Log.InfoContext(r.Context(), "pairing started", "cat", "auth", "session", p.id, "device", p.device, "platform", platform, "from", ipOf(r), "v", 2)
	pin, step, _, _ := s.pair.pinNow(p.id, secret)
	writeJSON(w, 200, map[string]any{
		"sessionId":    p.id,
		"deviceSecret": secret,
		"pin":          pin,
		"pinExpiresAt": totp.StepEnds(step).Unix(),
		"period":       int(totp.Period.Seconds()),
		"digits":       totp.Digits,
		"qr":           qrPayload(id.ID, base, p.id, p.qrCheck),
		"code":         showPairCode(p.code),
		"approvePath":  "/pair",
		"approveUrl":   base + "/pair?code=" + showPairCode(p.code),
		"expiresAt":    p.expires.Unix(),
		"expiresInSec": int(authPairTTL.Seconds()),
		"intervalSec":  int(pairInterval.Seconds()),
	})
}

// GET /api/auth/pair/{session}/pin (public, the device's secret): the PIN to show now.
func (s *Server) handleAuthPairPIN(w http.ResponseWriter, r *http.Request) {
	secret := pairSecretFrom(r)
	if secret == "" {
		writeErr(w, 400, pairSecretHeader+" is required")
		return
	}
	pin, step, p, ok := s.pair.pinNow(r.PathValue("session"), secret)
	switch {
	case p == nil:
		writeJSON(w, 404, map[string]any{"status": "expired", "error": "this sign-in has expired or was never started; start again"})
	case !ok:
		writeJSON(w, 410, map[string]any{"status": "answered", "error": "this sign-in is no longer waiting for a PIN; check its status"})
	default:
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{
			"pin":          pin,
			"pinExpiresAt": totp.StepEnds(step).Unix(),
			"period":       int(totp.Period.Seconds()),
			"digits":       totp.Digits,
			"expiresAt":    p.expires.Unix(),
		})
	}
}

// GET /api/auth/pair/{session} (public, the device's secret): has someone answered? As
// /api/pair/poll: an approval hands out the sign-in once.
func (s *Server) handleAuthPairPoll(w http.ResponseWriter, r *http.Request) {
	secret := pairSecretFrom(r)
	if secret == "" {
		writeErr(w, 400, pairSecretHeader+" is required")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	state, p := s.pair.pollV2(r.PathValue("session"), secret)
	s.writePoll(w, r, state, p)
}

// pinGate checks the caller may answer pairings, and isn't locked out for wrong codes.
func (s *Server) pinGate(w http.ResponseWriter, r *http.Request) (*auth.User, string, bool) {
	me := userFrom(r)
	if !mayPair(me) {
		writeErr(w, 403, "this profile can't sign devices in")
		return nil, "", false
	}
	k := s.logins.key(r, "pair:"+me.Name)
	if d := s.logins.wait(k); d > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
		s.Log.WarnContext(r.Context(), "pairing locked after repeated wrong codes", "cat", "auth", "user", me.Name, "from", ipOf(r))
		writeErr(w, 429, "Too many wrong codes. Try again in a little while.")
		return nil, "", false
	}
	return me, k, true
}

// GET /api/auth/pair/{session}/info?k=: what is asking, for an app that scanned the QR
// code, before anyone agrees to it. k is the check from the code; a wrong one is refused.
func (s *Server) handleAuthPairInfo(w http.ResponseWriter, r *http.Request) {
	me, k, ok := s.pinGate(w, r)
	if !ok {
		return
	}
	p := s.pair.waitingByID(r.PathValue("session"))
	if p == nil {
		s.logins.fail(k)
		s.Log.InfoContext(r.Context(), "pairing failed", "cat", "auth", "session", r.PathValue("session"), "from", ipOf(r), "why", pinNotFound)
		writeJSON(w, 404, map[string]any{"error": pinFailures[pinNotFound].msg, "reason": pinNotFound})
		return
	}
	if check := r.URL.Query().Get("k"); check != "" && !hmac.Equal([]byte(check), []byte(p.qrCheck)) {
		s.logins.fail(k)
		s.Log.InfoContext(r.Context(), "pairing failed", "cat", "auth", "session", p.id, "user", me.Name, "from", ipOf(r), "why", pinBadCheck)
		writeJSON(w, 400, map[string]any{"error": pinFailures[pinBadCheck].msg, "reason": pinBadCheck})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, s.pairInfo(r, me, p))
}

// POST /api/auth/pair/{session}/approve {pin, userId, check, password, otp}: sign the
// device in, with the PIN it shows now. password (and otp, with two-factor sign-in) are
// the approver's own, needed when the device will be signed in as an administrator.
func (s *Server) handleAuthPairApprove(w http.ResponseWriter, r *http.Request) {
	me, k, ok := s.pinGate(w, r)
	if !ok {
		return
	}
	var b struct {
		PIN      string `json:"pin"`
		UserID   int64  `json:"userId"`
		Check    string `json:"check"`
		Password string `json:"password"`
		OTP      string `json:"otp"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON body")
		return
	}
	target, allowed := s.pairTarget(r, me, b.UserID)
	if !allowed {
		writeErr(w, 403, "you can't sign a device in as that profile")
		return
	}
	if !s.confirmAdminGrant(w, r, target, b.Password, b.OTP) {
		return
	}
	s.approveByPIN(w, r, me, target, k, r.PathValue("session"), false, b.PIN, b.Check)
}

// confirmAdminGrant: a device signed in as an administrator can do everything on the
// server for as long as its sign-in lasts, so approving that asks for the administrator's
// own password again (and code, with two-factor sign-in). A session left open on a phone,
// or stolen, can then sign a device in as a Kids profile at most. It answers the request
// itself when it refuses.
func (s *Server) confirmAdminGrant(w http.ResponseWriter, r *http.Request, target *auth.User, password, otp string) bool {
	if !target.IsAdmin {
		return true
	}
	if password == "" {
		writeJSON(w, 403, map[string]any{"error": "Signing a device in as an administrator needs your password.", "reason": pairNeedsPassword, "twoFactor": target.TwoFactor})
		return false
	}
	if !s.confirmOwner(w, r, password, otp) {
		s.Log.WarnContext(r.Context(), "pairing refused: the administrator's password wasn't confirmed", "cat", "auth", "session", r.PathValue("session"), "user", target.Name, "from", ipOf(r))
		return false
	}
	return true
}

// pairNeedsPassword is the reason an approval is refused for want of the password.
const pairNeedsPassword = "password-required"

// approveByPIN approves a pairing that has a PIN, for v2 and for /api/pair/approve alike.
func (s *Server) approveByPIN(w http.ResponseWriter, r *http.Request, me, target *auth.User, k, lookup string, byCode bool, pin, check string) {
	p, reason, left := s.pair.approveWithPIN(lookup, byCode, pin, check, target.ID, me.ID, tokenFrom(r))
	// The session is the pairing's public id; a code is never logged.
	sess := ""
	if !byCode {
		sess = lookup
	}
	if reason == "" {
		s.logins.ok(k)
		s.Log.InfoContext(r.Context(), "pairing approved with a PIN", "cat", "auth", "session", p.id, "device", p.device, "account", target.Name, "user", me.Name, "from", p.ip)
		writeJSON(w, 200, map[string]any{"ok": true, "device": p.device, "user": target})
		return
	}
	switch reason {
	case pinWrong, pinBadCheck, pinLocked, pinNotFound:
		s.logins.fail(k)
	}
	if reason == pinLocked {
		s.Log.WarnContext(r.Context(), "pairing locked after repeated wrong PINs", "cat", "auth", "session", sess, "user", me.Name, "from", ipOf(r))
	} else {
		s.Log.InfoContext(r.Context(), "pairing failed", "cat", "auth", "session", sess, "user", me.Name, "from", ipOf(r), "why", reason, "triesLeft", left)
	}
	f := pinFailures[reason]
	body := map[string]any{"error": f.msg, "reason": reason}
	if reason == pinWrong || reason == pinBadCheck {
		body["triesLeft"] = left
	}
	writeJSON(w, f.status, body)
}

// POST /api/auth/pair/{session}/deny: refuse; the device is told and stops asking.
func (s *Server) handleAuthPairDeny(w http.ResponseWriter, r *http.Request) {
	_, k, ok := s.pinGate(w, r)
	if !ok {
		return
	}
	p := s.pair.denyByID(r.PathValue("session"))
	if p == nil {
		s.logins.fail(k)
		writeErr(w, 404, pinFailures[pinNotFound].msg)
		return
	}
	s.logins.ok(k)
	s.Log.InfoContext(r.Context(), "pairing refused", "cat", "auth", "session", p.id, "device", p.device, "user", userFrom(r).Name, "from", p.ip)
	writeJSON(w, 200, map[string]any{"ok": true})
}
