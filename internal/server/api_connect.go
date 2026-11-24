// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/wittywilla/wittymediaserver/internal/i18n"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/p2p"
)

// Connecting another device (Settings, Remote Access; the From away page): one
// way in for one device, shown as a short code and as a link (the QR code holds
// the link). Whichever the device uses first ends the other.
//
//   - The code is typed on a device on the same network, and goes to this
//     server's web port (POST /api/connect/hello, then /api/connect/answer).
//     The device doesn't know which server on the network made the code, so it
//     asks each one it finds; none of them learns the code. Much as joining by a
//     code does (internal/p2p/join.go), both ends turn it into two keys, with
//     scrypt (p2p.DeriveWebJoin): the device sends the first, to say which code it means, and the
//     server has to prove it knows the code before the device proves it does. A
//     server that didn't make the code can't, and gets nothing it could guess
//     the code from in the fifteen minutes it lasts.
//   - The link is a peer-to-peer invite (p2p.Invite), when peer to peer is on:
//     the way in from away. (A code from away would need the public address
//     book to find this server by, which proved too slow and unreliable to
//     offer: see #605.)
//
// What the way in is worth is what an invite is worth: once, for fifteen
// minutes, only while whoever made it stays signed in, and as an administrator
// only with that administrator's password typed again.

const (
	connectTTL      = 15 * time.Minute
	connectMaxOpen  = 16
	connectMaxWrong = 3 // wrong proofs before a code is given up
	connectAsks     = 8 // challenges a code keeps at once
	connectAskTTL   = time.Minute
)

type connectOffer struct {
	code    string // as shown: K7QM-2XHD
	find    string // the lookup key, in hex
	proof   []byte
	by      int64
	session string
	userID  int64
	expires time.Time
	wrong   int
	asks    map[string]time.Time // server nonces handed out, base64, until when they may be answered
	spent   *atomic.Bool         // shared with the link's invite
}

type connectOffers struct {
	mu   sync.Mutex
	open map[string]*connectOffer // by find
}

// sweepLocked drops what has expired or been used. Call with mu held.
func (co *connectOffers) sweepLocked(now time.Time) {
	for k, o := range co.open {
		if now.After(o.expires) || o.spent.Load() {
			delete(co.open, k)
		}
	}
}

// add keeps an offer, ending any other the same person has open: one at a time
// each, so a forgotten one doesn't linger beside a new one.
func (co *connectOffers) add(o *connectOffer) ([]*connectOffer, error) {
	co.mu.Lock()
	defer co.mu.Unlock()
	if co.open == nil {
		co.open = map[string]*connectOffer{}
	}
	now := time.Now()
	co.sweepLocked(now)
	var ended []*connectOffer
	for k, x := range co.open {
		if x.by == o.by {
			x.spent.Store(true)
			ended = append(ended, x)
			delete(co.open, k)
		}
	}
	if len(co.open) >= connectMaxOpen {
		return ended, errPairBusy
	}
	co.open[o.find] = o
	return ended, nil
}

// cancel ends every offer this person has open.
func (co *connectOffers) cancel(by int64) []*connectOffer {
	co.mu.Lock()
	defer co.mu.Unlock()
	var ended []*connectOffer
	for k, x := range co.open {
		if x.by == by {
			x.spent.Store(true)
			ended = append(ended, x)
			delete(co.open, k)
		}
	}
	return ended
}

// ask hands out a challenge for a code: a fresh server nonce. nil when no
// code with that lookup key is open.
func (co *connectOffers) ask(find string) (*connectOffer, []byte) {
	co.mu.Lock()
	defer co.mu.Unlock()
	now := time.Now()
	co.sweepLocked(now)
	o := co.open[find]
	if o == nil {
		return nil, nil
	}
	for k, until := range o.asks {
		if now.After(until) {
			delete(o.asks, k)
		}
	}
	if len(o.asks) >= connectAsks {
		return o, nil // busy: someone is asking a lot about this one code
	}
	nonce := make([]byte, p2pJoinNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return o, nil
	}
	o.asks[base64.StdEncoding.EncodeToString(nonce)] = now.Add(connectAskTTL)
	return o, nonce
}

const p2pJoinNonceLen = 16

// connectQRMax is what the pages' QR codes hold (versions 1 to 10 at level M:
// web/src/ui/qr.ts), less a little.
const connectQRMax = 210

// qrInvite is the same invite, short enough for a QR code: the same server and
// secret, with the addresses most worth trying first, as many as fit (an app
// that has none can still look the server up by its ID). Public addresses come
// first, then the ones this server has on the home network, never a relay's,
// whose addresses are long. The name is cut last, if even that doesn't fit.
func qrInvite(inv p2p.Invite, home []string, max int) string {
	rank := func(a string) int {
		parts := strings.Split(a, "/")
		if len(parts) < 3 || strings.Contains(a, "/p2p-circuit") {
			return -1
		}
		ip, err := netip.ParseAddr(parts[2])
		if err != nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return -1
		}
		if !ip.IsPrivate() {
			return 0
		}
		for _, h := range home {
			if strings.Contains(h, "//"+ip.String()+":") {
				return 1
			}
		}
		return 2
	}
	var addrs []string
	for want := 0; want <= 2; want++ {
		for _, a := range inv.Addrs {
			if rank(a) == want {
				addrs = append(addrs, a)
			}
		}
	}
	short := inv
	for n := len(addrs); n >= 0; n-- {
		short.Addrs = addrs[:n]
		if link := short.String(); len(link) <= max {
			return link
		}
	}
	for r := []rune(short.Name); len(r) > 0; r = r[:len(r)-1] {
		short.Name = string(r[:len(r)-1])
		if link := short.String(); len(link) <= max {
			return link
		}
	}
	return short.String()
}

// answer checks a device's proof for a challenge it was given, once. On a good
// proof the offer is spent here, so nothing else can use it.
func (co *connectOffers) answer(find string, deviceNonce, serverNonce, proof []byte, identityKey string) (*connectOffer, string) {
	co.mu.Lock()
	defer co.mu.Unlock()
	now := time.Now()
	co.sweepLocked(now)
	o := co.open[find]
	if o == nil {
		return nil, redeemWrong
	}
	key := base64.StdEncoding.EncodeToString(serverNonce)
	until, asked := o.asks[key]
	delete(o.asks, key) // a challenge is answered once, right or wrong
	if !asked || now.After(until) {
		return nil, redeemExpired
	}
	if !hmac.Equal(proof, p2p.WebJoinProof(o.proof, "device", deviceNonce, serverNonce, identityKey)) {
		if o.wrong++; o.wrong >= connectMaxWrong {
			o.spent.Store(true)
			delete(co.open, find)
		}
		return nil, redeemWrong
	}
	delete(co.open, find)
	if !o.spent.CompareAndSwap(false, true) {
		return nil, redeemUsed
	}
	return o, ""
}

// connectStatus is what the simple screen says first: who can connect, in words.
type connectStatus struct {
	// Away: off (only at home), checking, yes, partly (some networks), no.
	Away      string `json:"away"`
	Words     string `json:"words"`
	Detail    string `json:"detail,omitempty"`
	P2P       bool   `json:"p2p"`             // peer to peer is on
	CanTurnOn bool   `json:"canTurnOn"`       // this person may turn it on
	Other     string `json:"other,omitempty"` // a way in set up by hand: a private network or the internet
}

func (s *Server) connectStatus(r *http.Request) connectStatus {
	u := userFrom(r)
	st := connectStatus{CanTurnOn: u != nil && u.IsAdmin}
	rv := s.remoteView(r, "", false)
	if rv.State == "ok" {
		st.Other = rv.Away
	}
	node := s.p2pNode()
	cfg := s.loadP2P(r.Context())
	switch {
	case node == nil && cfg.Enabled:
		st.Away, st.Words, st.P2P = "checking", i18n.T("Connections from away are starting…"), true
	case node == nil:
		st.Away, st.Words = "off", i18n.T("Only devices on your home network can connect.")
	default:
		st.P2P = true
		v := verdictP2P(node.Status())
		st.Detail = v.Title
		switch v.State {
		case "ok":
			st.Away, st.Words = "yes", "Devices away from home can connect."
		case "warn":
			st.Away, st.Words = "partly", i18n.T("Devices away from home can connect from some networks, not all.")
		case "fail":
			st.Away, st.Words = "no", i18n.T("Devices away from home can't connect right now.")
		default:
			st.Away, st.Words = "checking", i18n.T("Devices away from home should be able to connect. This server is still checking.")
		}
	}
	if st.Other != "" && st.Away != "yes" {
		st.Away, st.Words = "yes", "Devices away from home can connect."
	}
	return st
}

// GET /api/connect: whether devices can connect from away, in words.
func (s *Server) handleConnectStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.connectStatus(r))
}

// POST /api/connect/offer {userId, password, otp}: a code, and the link the QR
// code holds, for one device to sign in as this account or a Kids or Guest
// profile it manages. An administrator's password is asked again (reason
// password-required) when the device would sign in as an administrator.
func (s *Server) handleConnectOffer(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	if !mayPair(me) {
		writeErr(w, 403, "this profile can't sign devices in")
		return
	}
	if !s.p2pPairAllowed(w, r) {
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
	code, err := p2p.NewJoinCode()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	find, proof, err := p2p.DeriveWebJoin(code)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	o := &connectOffer{code: code, find: hex.EncodeToString(find), proof: proof, by: me.ID, session: tokenFrom(r), userID: target.ID,
		expires: time.Now().Add(connectTTL), asks: map[string]time.Time{}, spent: &atomic.Bool{}}
	ended, err := s.connect.add(o)
	if len(ended) > 0 {
		s.p2pDevicesChanged() // the links beside the codes it ended are spent: their enrol keys go from the relay
	}
	if errors.Is(err, errPairBusy) {
		w.Header().Set("Retry-After", "60")
		writeErr(w, 429, "Too many devices are waiting to connect. Try again in a few minutes.")
		return
	}
	logging.AddSecret(code)
	if clean, err := p2p.CleanJoinCode(code); err == nil {
		logging.AddSecret(clean)
	}
	out := map[string]any{"code": code, "expiresInSec": int(connectTTL.Seconds()), "user": target, "homeAddresses": s.homeURLs()}
	// From away as well: the link. A profile kept at home has no use for it.
	away := false
	if node := s.p2pNode(); node != nil && accessFor(target).Can(library.FeatureRemote) {
		secret, err := s.p2p.invites.makeShared(me.ID, target.ID, tokenFrom(r), o.spent)
		if err == nil {
			s.p2pInviteOpened()
			logging.AddSecret(secret)
			inv := p2p.Invite{ServerID: node.ID(), Secret: secret, Name: s.ServerName(r.Context()), Addrs: inviteAddrs(node.Addrs())}
			out["link"] = inv.String()
			out["qrLink"] = qrInvite(inv, s.homeURLs(), connectQRMax)
			away = true
		} else {
			s.Log.WarnContext(r.Context(), "a code was made for connecting a device, but no link to go with it", "cat", "p2p", "err", err)
		}
	}
	out["away"] = away
	logging.Audit(r.Context(), s.Log, "a code to connect a device was made", "cat", "pair", "by", me.Name, "for", target.Name, "away", away)
	writeJSON(w, 200, out)
}

// DELETE /api/connect/offer: ends the code and link this person has open.
func (s *Server) handleConnectCancel(w http.ResponseWriter, r *http.Request) {
	if len(s.connect.cancel(userFrom(r).ID)) > 0 {
		s.p2pDevicesChanged()
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/connect/hello {v, find, nonce} (public): a device asking whether this
// server made the code it holds. The answer proves the server knows the code,
// bound to its identity key and both nonces; 404 says it didn't make it.
func (s *Server) handleConnectHello(w http.ResponseWriter, r *http.Request) {
	var b struct {
		V     int    `json:"v"`
		Find  string `json:"find"`
		Nonce string `json:"nonce"`
	}
	if err := readJSON(r, &b); err != nil || b.V != 1 || len(b.Find) != 64 {
		writeErr(w, 400, "That isn't a code this server understands.")
		return
	}
	deviceNonce, err := base64.StdEncoding.DecodeString(b.Nonce)
	if err != nil || len(deviceNonce) != p2pJoinNonceLen {
		writeErr(w, 400, "That isn't a code this server understands.")
		return
	}
	k := s.logins.key(r, "connect")
	if d := s.logins.wait(k); d > 0 {
		redeemFailed(w, redeemTooMany, d)
		return
	}
	o, nonce := s.connect.ask(b.Find)
	if o == nil {
		// Another server on the same network may have made it: that is not a guess, so it costs nothing here.
		writeJSON(w, 404, map[string]any{"error": "This server didn't make that code.", "reason": "not-here"})
		return
	}
	if nonce == nil {
		redeemFailed(w, redeemTooMany, time.Minute)
		return
	}
	id, err := s.Identity()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"nonce": base64.StdEncoding.EncodeToString(nonce), "publicKey": id.PublicKey(),
		"proof": base64.StdEncoding.EncodeToString(p2p.WebJoinProof(o.proof, "server", deviceNonce, nonce, id.PublicKey()))})
}

// POST /api/connect/answer {v, find, nonce, serverNonce, proof, device, platform}
// (public): the device's proof. A right one signs the device in.
func (s *Server) handleConnectAnswer(w http.ResponseWriter, r *http.Request) {
	var b struct {
		V           int    `json:"v"`
		Find        string `json:"find"`
		Nonce       string `json:"nonce"`
		ServerNonce string `json:"serverNonce"`
		Proof       string `json:"proof"`
		Device      string `json:"device"`
		Platform    string `json:"platform"`
	}
	if err := readJSON(r, &b); err != nil || b.V != 1 || len(b.Find) != 64 {
		writeErr(w, 400, "That isn't a code this server understands.")
		return
	}
	deviceNonce, err1 := base64.StdEncoding.DecodeString(b.Nonce)
	serverNonce, err2 := base64.StdEncoding.DecodeString(b.ServerNonce)
	proof, err3 := base64.StdEncoding.DecodeString(b.Proof)
	if err1 != nil || err2 != nil || err3 != nil || len(deviceNonce) != p2pJoinNonceLen || len(serverNonce) != p2pJoinNonceLen || len(proof) != 32 {
		writeErr(w, 400, "That isn't a code this server understands.")
		return
	}
	k := s.logins.key(r, "connect")
	if d := s.logins.wait(k); d > 0 {
		redeemFailed(w, redeemTooMany, d)
		return
	}
	id, err := s.Identity()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	o, reason := s.connect.answer(b.Find, deviceNonce, serverNonce, proof, id.PublicKey())
	if reason != "" {
		if reason == redeemWrong {
			s.logins.fail(k)
		}
		s.Log.InfoContext(r.Context(), "a code to connect a device was refused", "cat", "auth", "from", ipOf(r), "why", reason)
		redeemFailed(w, reason, 0)
		return
	}
	s.logins.ok(k)
	s.p2pDevicesChanged() // the link beside the code is spent: its enrol key goes from the relay
	if !s.stillSignedIn(r.Context(), o.session, o.by) {
		s.Log.InfoContext(r.Context(), "a code to connect a device was refused", "cat", "auth", "from", ipOf(r), "why", "whoever made the code has signed out")
		redeemFailed(w, redeemExpired, 0)
		return
	}
	device := cleanLabel(b.Device, 60)
	if device == "" {
		device = "App"
	}
	tok, u, err := s.Auth.IssueSession(r.Context(), o.userID, device)
	if err != nil {
		redeemFailed(w, redeemExpired, 0)
		return
	}
	logging.Audit(r.Context(), s.Log, "a device connected with a code", "cat", "auth", "device", device, "user", u.Name, "from", ipOf(r))
	s.notePlainSession(r, tok)
	s.noteP2PSession(r, tok)
	writeJSON(w, 200, map[string]any{"status": "approved", "token": tok, "user": u, "serverName": s.ServerName(r.Context())})
}
