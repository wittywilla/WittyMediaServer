// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wittywilla/wittymediaserver/internal/auth"
)

// Signing a device in with a code. A device on the network can't attach by
// itself: someone already signed in on a phone or computer has to agree, in one
// of two ways, for whichever end is easier to type on:
//
//   - the device shows a code, and it's typed at /pair, where the person sees
//     which device is asking and approves. The device polls with a long secret
//     only it knows, and collects its sign-in once (start, poll, info, approve);
//   - /pair shows 6 digits, and they're typed on the device (invite, redeem).
//
// Everything is held in memory for ten minutes at most: a restart simply asks
// for a new code. Codes use 20 consonants (no vowels, so no words; nothing that
// looks like a digit), about 34 bits for 8 letters, and guessing is limited
// and needs a signed-in account.

const (
	pairTTL          = 10 * time.Minute
	pairInterval     = 3 * time.Second // how often a TV should poll
	pairMaxPending   = 256             // on the whole server
	pairMaxPerIP     = 4               // codes open at once from one address
	pairStartsPerIP  = 12              // codes started from one address per window
	pairStartsWindow = 10 * time.Minute
	pairAlphabet     = "BCDFGHJKLMNPQRSTVWXZ"

	inviteTTL = 5 * time.Minute
	// inviteKeep: how long a finished code is remembered, to say why it failed.
	inviteKeep = 10 * time.Minute
	// Six digits are few, so guessing is limited twice: by address (s.logins),
	// and for the whole server, where passing the limit also cancels open codes.
	inviteWrongLimit  = 30
	inviteWrongWindow = 5 * time.Minute
	inviteMaxOpen     = 16
	// pairHomeShare: places at the table only the home network can take, so strangers
	// can't stop a TV in the house from asking for a code.
	pairHomeShare = 64
)

// invite is a code made to be typed on the device.
type invite struct {
	code    string
	by      int64  // who asked for it
	session string // the sign-in that asked: the code lasts only as long as that does
	userID  int64  // who the device signs in as
	expires time.Time
	used    bool
}

type pairing struct {
	code     string
	secret   string // sha256 of what the TV holds
	device   string
	platform string
	ip       string
	key      string // what limits count it under: one address, or one IPv6 network
	created  time.Time
	expires  time.Time
	lastPoll time.Time
	userID   int64 // who approved; 0 = waiting
	approver int64
	session  string // the sign-in that approved: the answer lasts only as long as that does
	denied   bool

	// Pairing v2 (/api/auth/pair) only; empty for a plain code.
	id      string // the session's public ID, in its URLs and QR code
	pinKey  []byte // the TOTP secret behind the rotating PIN: never leaves the server
	pinUsed int64  // the last time step a PIN was accepted for, so none works twice
	tries   int    // wrong PINs typed for this session
	locked  bool   // too many wrong PINs: the session is over
	qrCheck string // the k= in its QR code
}

type pairings struct {
	mu       sync.Mutex
	byCode   map[string]*pairing
	bySecret map[string]*pairing
	byID     map[string]*pairing // v2 sessions, by their public ID
	starts   map[string][]time.Time
	invites  map[string]*invite
	// clock is what time it is for pairing; nil means the real time. Tests set it, holding mu.
	clock func() time.Time
	qrKey []byte // signs the check in QR codes; made when first needed, lost on restart
	// Wrong codes typed lately. Those from away from home and those at home are kept apart,
	// so strangers can't stop anyone at home using a code.
	wrongAway, wrongHome []time.Time
}

// newInvite makes a 6-digit code; a person has one open at a time.
func (ps *pairings) newInvite(by, userID int64, session string) (*invite, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	now := time.Now()
	if ps.invites == nil {
		ps.invites = map[string]*invite{}
	}
	open := 0
	for c, in := range ps.invites {
		switch {
		case now.After(in.expires.Add(inviteKeep)):
			delete(ps.invites, c)
		case !in.used && now.Before(in.expires) && in.by == by:
			delete(ps.invites, c)
		case !in.used && now.Before(in.expires):
			open++
		}
	}
	if open >= inviteMaxOpen {
		return nil, errPairBusy
	}
	in := &invite{by: by, session: session, userID: userID, expires: now.Add(inviteTTL)}
	for {
		n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
		if err != nil {
			return nil, err
		}
		in.code = fmt.Sprintf("%06d", n.Int64())
		if ps.invites[in.code] == nil {
			break
		}
	}
	ps.invites[in.code] = in
	return in, nil
}

// Why a typed code didn't work. Part of the API: apps may word it their own way.
const (
	redeemWrong   = "wrong-code"
	redeemExpired = "expired"
	redeemUsed    = "used"
	redeemTooMany = "too-many-tries"
)

// redeem uses a code, once. It answers the code, or why not. away says the try comes from
// away from home.
func (ps *pairings) redeem(code string, away bool) (*invite, string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	now := time.Now()
	wrong := &ps.wrongHome
	if away {
		wrong = &ps.wrongAway
	}
	keep := (*wrong)[:0]
	for _, t := range *wrong {
		if now.Sub(t) < inviteWrongWindow {
			keep = append(keep, t)
		}
	}
	*wrong = keep
	if len(*wrong) >= inviteWrongLimit {
		return nil, redeemTooMany
	}
	in := ps.invites[code]
	switch {
	case in == nil:
		*wrong = append(*wrong, now)
		// Someone at home working through the numbers: no open code stays guessable. Away
		// from home the limit itself does that, and takes nothing from anyone at home.
		if !away && len(*wrong) >= inviteWrongLimit {
			for _, x := range ps.invites {
				x.expires = now.Add(-time.Second)
			}
		}
		return nil, redeemWrong
	case in.used:
		return nil, redeemUsed
	case now.After(in.expires):
		return nil, redeemExpired
	}
	in.used = true
	return in, ""
}

// stillSignedIn: a code or an approval still stands only while the sign-in behind it does. Without
// this, someone with a stolen session could keep one open, and be back in after the owner changed
// the password or signed everything out.
func (s *Server) stillSignedIn(ctx context.Context, session string, who int64) bool {
	u, err := s.Auth.Resolve(ctx, session)
	return err == nil && u.ID == who
}

var redeemFailures = map[string]struct {
	status int
	msg    string
}{
	redeemWrong:   {400, "That code isn't right. Check the number on your phone or computer and try again."},
	redeemExpired: {410, "That code has expired. Codes last five minutes: get a new one on your phone or computer."},
	redeemUsed:    {410, "That code has already been used. Get a new one on your phone or computer."},
	redeemTooMany: {429, "Too many wrong codes. Wait a few minutes and try again."},
}

func redeemFailed(w http.ResponseWriter, reason string, wait time.Duration) {
	f := redeemFailures[reason]
	body := map[string]any{"error": f.msg, "reason": reason}
	if reason == redeemTooMany {
		secs := int(wait.Seconds()) + 1
		if wait <= 0 {
			secs = int(inviteWrongWindow.Seconds())
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		body["retryAfterSec"] = secs
	}
	writeJSON(w, f.status, body)
}

// cleanInviteCode undoes how people type digits; "" if it can't be a code.
func cleanInviteCode(s string) string {
	s = strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(s))
	if len(s) != 6 {
		return ""
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return s
}

// POST /api/pair/invite {userId}: 6 digits to type on the device, for this
// account or a Kids or Guest profile it manages.
func (s *Server) handlePairInvite(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	if !mayPair(me) {
		writeErr(w, 403, "this profile can't sign devices in")
		return
	}
	var b struct {
		UserID   int64  `json:"userId"`
		Password string `json:"password"`
		OTP      string `json:"otp"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON body")
		return
	}
	target, ok := s.pairTarget(r, me, b.UserID)
	if !ok {
		writeErr(w, 403, "you can't sign a device in as that profile")
		return
	}
	if !s.confirmAdminGrant(w, r, target, b.Password, b.OTP) {
		return
	}
	in, err := s.pair.newInvite(me.ID, target.ID, tokenFrom(r))
	if errors.Is(err, errPairBusy) {
		w.Header().Set("Retry-After", "60")
		writeErr(w, 429, "Too many codes are waiting to be used. Try again in a few minutes.")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"code": in.code, "expiresInSec": int(inviteTTL.Seconds()), "user": target})
}

// POST /api/pair/redeem {code, device, platform} (public): a device typing the
// digits gets its sign-in straight away.
func (s *Server) handlePairRedeem(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Code     string `json:"code"`
		Device   string `json:"device"`
		Platform string `json:"platform"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON body")
		return
	}
	k := s.logins.key(r, "redeem")
	if d := s.logins.wait(k); d > 0 {
		s.Log.WarnContext(r.Context(), "code sign-in locked after repeated failures", "cat", "auth", "from", ipOf(r), "wait", d.Round(time.Second).String())
		redeemFailed(w, redeemTooMany, d)
		return
	}
	code := cleanInviteCode(b.Code)
	if code == "" { // not a guess at anything
		s.Log.InfoContext(r.Context(), "code sign-in failed", "cat", "auth", "from", ipOf(r), "why", "not a code")
		redeemFailed(w, redeemWrong, 0)
		return
	}
	in, reason := s.pair.redeem(code, !clientFrom(r).Where.AtHome())
	if reason != "" {
		if reason == redeemWrong {
			s.logins.fail(k)
		}
		s.Log.InfoContext(r.Context(), "code sign-in failed", "cat", "auth", "from", ipOf(r), "why", reason)
		redeemFailed(w, reason, 0)
		return
	}
	s.logins.ok(k)
	if !s.stillSignedIn(r.Context(), in.session, in.by) {
		s.Log.InfoContext(r.Context(), "code sign-in failed", "cat", "auth", "from", ipOf(r), "why", "whoever made the code has signed out")
		redeemFailed(w, redeemExpired, 0)
		return
	}
	device := cleanLabel(b.Device, 60)
	if device == "" {
		device = "TV app"
	}
	tok, u, err := s.Auth.IssueSession(r.Context(), in.userID, device)
	if err != nil { // the account was deleted in the meantime
		redeemFailed(w, redeemExpired, 0)
		return
	}
	s.Log.InfoContext(r.Context(), "device signed in with a code", "cat", "auth", "device", device, "user", u.Name, "from", ipOf(r))
	s.notePlainSession(r, tok)
	writeJSON(w, 200, map[string]any{"status": "approved", "token": tok, "user": u})
}

// pairTarget is the account a device will sign in as: this one, or a Kids or
// Guest profile it manages.
func (s *Server) pairTarget(r *http.Request, me *auth.User, id int64) (*auth.User, bool) {
	if id == 0 || id == me.ID {
		return me, true
	}
	t, err := s.Auth.User(r.Context(), id)
	if err != nil || t.IsAdmin || !s.canManage(r, me, t.Kind) {
		return nil, false
	}
	return t, true
}

var errPairBusy = errors.New("too many sign-in codes were asked for from this device; wait a few minutes")

// now is the time as pairing sees it. Call with mu held.
func (ps *pairings) now() time.Time {
	if ps.clock != nil {
		return ps.clock()
	}
	return time.Now()
}

// forgetLocked drops a pairing from every index.
func (ps *pairings) forgetLocked(p *pairing) {
	delete(ps.byCode, p.code)
	delete(ps.bySecret, p.secret)
	if p.id != "" {
		delete(ps.byID, p.id)
	}
}

func (ps *pairings) sweep(now time.Time) {
	for _, p := range ps.byCode {
		if now.After(p.expires) {
			ps.forgetLocked(p)
		}
	}
	for ip, ts := range ps.starts {
		keep := ts[:0]
		for _, t := range ts {
			if now.Sub(t) < pairStartsWindow {
				keep = append(keep, t)
			}
		}
		if len(keep) == 0 {
			delete(ps.starts, ip)
		} else {
			ps.starts[ip] = keep
		}
	}
}

// start opens a pairing and returns its code and the TV's secret.
func (ps *pairings) start(device, platform, ip, key string, away bool) (*pairing, string, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.startLocked(device, platform, ip, key, away)
}

// startLocked is start, for v1 and v2 alike: the same limits, the same code and secret.
func (ps *pairings) startLocked(device, platform, ip, key string, away bool) (*pairing, string, error) {
	now := ps.now()
	if ps.byCode == nil {
		ps.byCode, ps.bySecret, ps.starts = map[string]*pairing{}, map[string]*pairing{}, map[string][]time.Time{}
	}
	if ps.byID == nil {
		ps.byID = map[string]*pairing{}
	}
	ps.sweep(now)
	open := 0
	for _, p := range ps.byCode {
		if p.key == key {
			open++
		}
	}
	room := pairMaxPending
	if away {
		room -= pairHomeShare
	}
	if open >= pairMaxPerIP || len(ps.starts[key]) >= pairStartsPerIP || len(ps.byCode) >= room {
		return nil, "", errPairBusy
	}
	ps.starts[key] = append(ps.starts[key], now)
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", err
	}
	secret := hex.EncodeToString(raw)
	p := &pairing{secret: hashSecret(secret), device: device, platform: platform, ip: ip, key: key, created: now, expires: now.Add(pairTTL)}
	for {
		code, err := newPairCode()
		if err != nil {
			return nil, "", err
		}
		if ps.byCode[code] == nil {
			p.code = code
			break
		}
	}
	ps.byCode[p.code] = p
	ps.bySecret[p.secret] = p
	return p, secret, nil
}

func hashSecret(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func newPairCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = pairAlphabet[int(b[i])%len(pairAlphabet)]
	}
	return string(b), nil
}

// cleanPairCode undoes how people type codes: lower case, spaces, the dash.
func cleanPairCode(s string) string {
	return strings.NewReplacer("-", "", " ", "").Replace(strings.ToUpper(strings.TrimSpace(s)))
}

// showPairCode is the code the way a TV should show it: ABCD-EFGH.
func showPairCode(c string) string {
	if len(c) == 8 {
		return c[:4] + "-" + c[4:]
	}
	return c
}

// waiting returns the live, unanswered pairing for a code.
func (ps *pairings) waiting(code string) *pairing {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.waitingLocked(code)
}

func (ps *pairings) waitingLocked(code string) *pairing {
	return ps.stillWaitingLocked(ps.byCode[cleanPairCode(code)])
}

// stillWaitingLocked is p if it is live and unanswered, else nil.
func (ps *pairings) stillWaitingLocked(p *pairing) *pairing {
	if p == nil || ps.now().After(p.expires) || p.userID != 0 || p.denied || p.locked {
		return nil
	}
	return p
}

// answer approves (userID > 0) or denies a waiting pairing, once.
func (ps *pairings) answer(code string, userID, approver int64, session string) *pairing {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	p := ps.waitingLocked(code)
	if p == nil {
		return nil
	}
	ps.answerLocked(p, userID, approver, session)
	return p
}

// answerLocked answers a pairing known to be waiting.
func (ps *pairings) answerLocked(p *pairing, userID, approver int64, session string) {
	if userID > 0 {
		p.userID, p.approver, p.session = userID, approver, session
	} else {
		p.denied = true
	}
	// The TV has a little time left to collect the answer, whatever remained.
	if until := ps.now().Add(2 * time.Minute); until.Before(p.expires) {
		p.expires = until
	}
}

// How a poll ended.
const (
	pollPending = iota
	pollTooFast
	pollGone
	pollDenied
	pollApproved
	pollLockedOut // v2: too many wrong PINs
)

// poll answers a TV. An approved or denied pairing is finished by the answer:
// the next poll with the same secret finds nothing, so a sign-in is collected
// exactly once.
func (ps *pairings) poll(secret string) (int, *pairing) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.pollLocked(ps.bySecret[hashSecret(secret)])
}

func (ps *pairings) pollLocked(p *pairing) (int, *pairing) {
	now := ps.now()
	if p == nil || now.After(p.expires) {
		return pollGone, nil
	}
	if !p.lastPoll.IsZero() && now.Sub(p.lastPoll) < pairInterval/2 {
		return pollTooFast, p
	}
	p.lastPoll = now
	switch {
	case p.locked:
		ps.forgetLocked(p)
		return pollLockedOut, p
	case p.userID != 0:
		ps.forgetLocked(p)
		return pollApproved, p
	case p.denied:
		ps.forgetLocked(p)
		return pollDenied, p
	}
	return pollPending, p
}

func cleanLabel(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	for len(s) > n {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s
}

func ipOf(r *http.Request) string {
	if p := clientIP(r); p != nil {
		return p.String()
	}
	return "?"
}

// POST /api/pair/start {device, platform} (public): a code to show on screen.
func (s *Server) handlePairStart(w http.ResponseWriter, r *http.Request) {
	device, platform, ok := s.pairStartRequest(w, r)
	if !ok {
		return
	}
	// An administrator can turn the code-only way off: every device then has to show a PIN.
	if s.DB.GetSetting(r.Context(), "pairRequirePin", "0") == "1" {
		writeJSON(w, 426, map[string]any{"error": "This server signs devices in with a PIN. Update the app, or sign in with a password.", "reason": "pin-required", "use": "/api/v1/auth/pair"})
		return
	}
	p, secret, err := s.pair.start(device, platform, ipOf(r), addressKey(r), !clientFrom(r).Where.AtHome())
	if !pairStartOK(w, err) {
		return
	}
	writeJSON(w, 200, map[string]any{
		"code":         showPairCode(p.code),
		"deviceSecret": secret,
		"expiresInSec": int(pairTTL.Seconds()),
		"intervalSec":  int(pairInterval.Seconds()),
		"approvePath":  "/pair",
		"approveUrl":   "/pair?code=" + showPairCode(p.code),
	})
}

// pairStartRequest reads what a device starting a pairing says about itself, for v1 and v2.
func (s *Server) pairStartRequest(w http.ResponseWriter, r *http.Request) (device, platform string, ok bool) {
	var b struct {
		Device   string `json:"device"`
		Platform string `json:"platform"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON body")
		return "", "", false
	}
	device = cleanLabel(b.Device, 60)
	if device == "" {
		device = "TV app"
	}
	if s.Auth.NoAccounts(r.Context()) {
		writeErr(w, 409, "this server has not been set up yet")
		return "", "", false
	}
	return device, cleanLabel(b.Platform, 20), true
}

// pairStartOK answers a failed start; true when there was nothing to answer.
func pairStartOK(w http.ResponseWriter, err error) bool {
	if errors.Is(err, errPairBusy) {
		w.Header().Set("Retry-After", "60")
		writeErr(w, 429, err.Error())
		return false
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return false
	}
	return true
}

// POST /api/pair/poll {deviceSecret} (public): has someone answered? The
// secret travels in the body so it never lands in logs.
func (s *Server) handlePairPoll(w http.ResponseWriter, r *http.Request) {
	var b struct {
		DeviceSecret string `json:"deviceSecret"`
	}
	if err := readJSON(r, &b); err != nil || len(b.DeviceSecret) != 64 {
		writeErr(w, 400, "deviceSecret is required")
		return
	}
	state, p := s.pair.poll(b.DeviceSecret)
	s.writePoll(w, r, state, p)
}

// writePoll answers a device asking whether its pairing was answered, the same way for v1 and v2.
func (s *Server) writePoll(w http.ResponseWriter, r *http.Request, state int, p *pairing) {
	switch state {
	case pollTooFast:
		w.Header().Set("Retry-After", strconv.Itoa(int(pairInterval.Seconds())))
		writeJSON(w, 429, map[string]any{"status": "slow-down", "error": "polling too fast", "intervalSec": int(pairInterval.Seconds())})
	case pollGone:
		writeJSON(w, 410, map[string]any{"status": "expired", "error": "this code has expired or was already used; ask for a new one"})
	case pollDenied:
		writeJSON(w, 403, map[string]any{"status": "denied", "error": "the sign-in was refused"})
	case pollLockedOut:
		writeJSON(w, 410, map[string]any{"status": "locked", "error": "too many wrong PINs were typed; start again"})
	case pollApproved:
		if !s.stillSignedIn(r.Context(), p.session, p.approver) {
			writeJSON(w, 410, map[string]any{"status": "expired", "error": "this code has expired or was already used; ask for a new one"})
			return
		}
		tok, u, err := s.Auth.IssueSession(r.Context(), p.userID, p.device)
		if err != nil { // the account was deleted in the meantime
			writeJSON(w, 410, map[string]any{"status": "expired", "error": "this code has expired or was already used; ask for a new one"})
			return
		}
		s.Log.InfoContext(r.Context(), "device signed in by pairing", "cat", "auth", "device", p.device, "user", u.Name)
		s.notePlainSession(r, tok)
		writeJSON(w, 200, map[string]any{"status": "approved", "token": tok, "user": u})
	default:
		writeJSON(w, 200, map[string]any{"status": "pending", "intervalSec": int(pairInterval.Seconds())})
	}
}

// pairCodeFrom reads a code from the body, counting wrong ones: signed-in
// people may try a few, not all of them.
func (s *Server) pairCodeFrom(w http.ResponseWriter, r *http.Request, v any, code func() string) (*pairing, string, bool) {
	k := s.logins.key(r, "pair:"+userFrom(r).Name)
	if d := s.logins.wait(k); d > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
		s.Log.WarnContext(r.Context(), "pairing locked after repeated wrong codes", "cat", "auth", "from", ipOf(r))
		writeErr(w, 429, "Too many wrong codes. Try again in a little while.")
		return nil, "", false
	}
	if err := readJSON(r, v); err != nil {
		writeErr(w, 400, "expected a JSON body")
		return nil, "", false
	}
	p := s.pair.waiting(code())
	if p == nil {
		s.logins.fail(k)
		s.Log.InfoContext(r.Context(), "pairing failed: wrong code", "cat", "auth", "from", ipOf(r))
		writeErr(w, 404, "That code isn't valid. Check it on the TV: codes last ten minutes and work once.")
		return nil, "", false
	}
	return p, k, true
}

// mayPair: only adults sign devices in. A Kids or Guest profile may have
// no password at all, and must not be able to hand out lasting sign-ins.
func mayPair(u *auth.User) bool { return u.IsAdmin || u.Kind == auth.KindAdult }

// POST /api/pair/info {code}: what is asking, before anyone agrees to it.
func (s *Server) handlePairInfo(w http.ResponseWriter, r *http.Request) {
	if !mayPair(userFrom(r)) {
		writeErr(w, 403, "this profile can't sign devices in")
		return
	}
	var b struct {
		Code string `json:"code"`
	}
	// Looking at a code clears nothing: that would let someone with a code of their own go on
	// guessing at everyone else's.
	p, _, ok := s.pairCodeFrom(w, r, &b, func() string { return b.Code })
	if !ok {
		return
	}
	writeJSON(w, 200, s.pairInfo(r, userFrom(r), p))
}

// pairInfo is what someone about to approve a pairing is shown: the device, and the
// profiles they can put on it instead of their own.
func (s *Server) pairInfo(r *http.Request, me *auth.User, p *pairing) map[string]any {
	type choice struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Kind string `json:"kind"`
		// NeedsPassword: signing the device in as this profile asks for the approver's
		// password again (an administrator's own account).
		NeedsPassword bool `json:"needsPassword,omitempty"`
	}
	profiles := []choice{{me.ID, me.Name, me.Kind, me.IsAdmin}}
	if users, err := s.Auth.Users(r.Context()); err == nil {
		for _, u := range users {
			if u.ID != me.ID && !u.IsAdmin && s.canManage(r, me, u.Kind) {
				profiles = append(profiles, choice{u.ID, u.Name, u.Kind, false})
			}
		}
	}
	out := map[string]any{
		"code": showPairCode(p.code), "device": p.device, "platform": p.platform, "ip": p.ip,
		"createdAt": p.created.Unix(), "expiresAt": p.expires.Unix(), "profiles": profiles,
		// Started with /api/auth/pair: approving takes the PIN the device shows.
		"needsPin": p.pinKey != nil,
		// The approver has two-factor sign-in: confirming their password takes a code too.
		"twoFactor": me.TwoFactor,
	}
	if p.id != "" {
		out["sessionId"] = p.id
	}
	return out
}

// POST /api/pair/approve {code, userId}: signs the device in, as this account
// or a Kids or Guest profile this account manages.
func (s *Server) handlePairApprove(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	if !mayPair(me) {
		writeErr(w, 403, "this profile can't sign devices in")
		return
	}
	var b struct {
		Code   string `json:"code"`
		UserID int64  `json:"userId"`
		PIN    string `json:"pin"` // for a device that started with /api/auth/pair
		// The approver's own password (and two-factor code), when the device will be
		// signed in as an administrator.
		Password string `json:"password"`
		OTP      string `json:"otp"`
	}
	waiting, k, ok := s.pairCodeFrom(w, r, &b, func() string { return b.Code })
	if !ok {
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
	// A device with a PIN is never signed in by its code alone, whichever way it is approved.
	if waiting.pinKey != nil {
		s.approveByPIN(w, r, me, target, k, b.Code, true, b.PIN, "")
		return
	}
	p := s.pair.answer(b.Code, target.ID, me.ID, tokenFrom(r))
	if p == nil { // answered a moment ago, on another phone
		writeErr(w, 404, "That code has already been used.")
		return
	}
	s.logins.ok(k)
	s.Log.InfoContext(r.Context(), "pairing approved", "cat", "auth", "device", p.device, "account", target.Name, "user", me.Name, "ip", p.ip)
	writeJSON(w, 200, map[string]any{"ok": true, "device": p.device, "user": target})
}

// POST /api/pair/deny {code}: refuses it; the TV is told and stops asking.
func (s *Server) handlePairDeny(w http.ResponseWriter, r *http.Request) {
	if !mayPair(userFrom(r)) {
		writeErr(w, 403, "this profile can't sign devices in")
		return
	}
	var b struct {
		Code string `json:"code"`
	}
	_, k, ok := s.pairCodeFrom(w, r, &b, func() string { return b.Code })
	if !ok {
		return
	}
	if p := s.pair.answer(b.Code, 0, 0, ""); p != nil {
		s.Log.InfoContext(r.Context(), "pairing refused", "cat", "auth", "device", p.device, "user", userFrom(r).Name, "ip", p.ip)
	}
	s.logins.ok(k)
	writeJSON(w, 200, map[string]any{"ok": true})
}
