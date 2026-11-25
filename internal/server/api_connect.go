// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/i18n"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/p2p"
	"github.com/wittywilla/wittymediaserver/internal/remote"
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
//   - The QR code holds the link, and for a device at home the address the
//     page was opened at and the code as well (connectHome): scanned at home,
//     the device asks there with the code, as if it had been typed, and needs
//     no peer-to-peer path. In a container on Docker's bridge network that is
//     the only way a device at home finds the server at all: the server's own
//     addresses are the container's, and it can't be discovered.
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

// connectPerPerson is how many offers one person may have open. A new one
// doesn't end the one before (#683): the person may have the screen open in a
// second tab, or on the TV and the phone, and a device may be using the first
// one right now. Past this many, the oldest goes.
const connectPerPerson = 3

// add keeps an offer, ending the oldest of the same person's past connectPerPerson.
func (co *connectOffers) add(o *connectOffer) ([]*connectOffer, error) {
	co.mu.Lock()
	defer co.mu.Unlock()
	if co.open == nil {
		co.open = map[string]*connectOffer{}
	}
	now := time.Now()
	co.sweepLocked(now)
	var ended []*connectOffer
	for {
		var oldest *connectOffer
		mine := 0
		for _, x := range co.open {
			if x.by == o.by {
				mine++
				if oldest == nil || x.expires.Before(oldest.expires) {
					oldest = x
				}
			}
		}
		if mine < connectPerPerson {
			break
		}
		oldest.spent.Store(true)
		ended = append(ended, oldest)
		delete(co.open, oldest.find)
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

// connectQRMax is how long a link the QR code holds: all of version 13 at level
// M (331 bytes; web/src/ui/qr.ts goes to 15), still easy to read at the page's
// size. It fits the address at home, the code and one relay of the owner's
// (#683); when they don't all fit, the relay stays (qrInvite).
const connectQRMax = 330

// connectHome is where another device at home can reach this server's web
// port: the address the page making the code was opened at, when it is one
// only the home network uses (a private IPv4 address, or a name such as
// nas.local) and the browser reached the server directly. bridged says that
// address is none of this computer's own: the server runs in a container on
// Docker's bridge network, whose own addresses nothing else at home can reach,
// and the browser's bar names the host and its published port.
func connectHome(r *http.Request) (home string, ip netip.Addr, bridged bool) {
	c := clientFrom(r)
	if !c.Direct() || (c.Where != remote.Home && c.Where != remote.Machine) {
		return "", netip.Addr{}, false
	}
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		host, port = r.Host, ""
	}
	name := strings.ToLower(strings.TrimSuffix(host, "."))
	if a, err := netip.ParseAddr(host); err == nil {
		if a = a.Unmap(); !a.Is4() || !a.IsPrivate() {
			return "", netip.Addr{}, false // this computer itself, or an address another device at home can't be sure of
		}
		ip = a
	} else if name == "localhost" || strings.HasSuffix(name, ".localhost") || !remote.LocalName(name) {
		return "", netip.Addr{}, false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	at := host
	if port != "" {
		at = net.JoinHostPort(host, port)
	}
	if home = p2p.InviteHome(scheme + "://" + at); home == "" {
		return "", netip.Addr{}, false
	}
	if ip.IsValid() {
		bridged = true
		for _, i := range remote.Interfaces() {
			if i.Addr.Addr() == ip {
				bridged = false
				break
			}
		}
	}
	return home, ip, bridged
}

// hostHints are where this server's peer-to-peer port is on the container's
// host, for a device at home: Docker forwards it there when it is published
// with the same number (deploy/docker), and the invite's other addresses are
// the container's own.
func hostHints(ip netip.Addr, port int) []string {
	if !ip.IsValid() || port <= 0 {
		return nil
	}
	return []string{fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", ip, port), fmt.Sprintf("/ip4/%s/tcp/%d", ip, port)}
}

// qrInvite is the same invite, short enough for a QR code: the same server and
// secret, with the addresses most worth trying first, as many as fit (an app
// that has none can still look the server up by its ID). A circuit through one
// of the owner's own relays comes first: from away it is often the only way in,
// and the public address book is no help to a server behind a router (#683).
// Then public addresses, then another relay's circuit, then the ones this server
// has on the home network, which a device at home doesn't need when the QR code
// holds the web address. The name is cut before the first relayed address goes;
// the home address and code, which a device at home needs most, go only when
// nothing else would make it fit (a very long name for the computer).
func qrInvite(inv p2p.Invite, home []string, own map[string]bool, max int) string {
	noHome := inv
	noHome.Home, noHome.Code = "", ""
	// Everything that matters: the address at home and the code, and the first relayed address.
	if link, ok := qrFit(inv, home, own, max, true); ok {
		return link
	}
	// Then the relayed address before the address at home: it works at home too, and the home address doesn't
	// away from it (the pairing lab's relay-only cases).
	if inv.Home != "" {
		if link, ok := qrFit(noHome, home, own, max, true); ok {
			return link
		}
	}
	if link, ok := qrFit(inv, home, own, max, false); ok {
		return link
	}
	link, _ := qrFit(noHome, home, own, max, false)
	return link
}

// circuitRelay is the relay a /p2p-circuit address goes through, by its ID; "" for any other address.
func circuitRelay(a string) string {
	before, _, ok := strings.Cut(a, "/p2p-circuit")
	if !ok {
		return ""
	}
	if i := strings.LastIndex(before, "/p2p/"); i >= 0 {
		return before[i+len("/p2p/"):]
	}
	return ""
}

// ownRelayIDs are the relays the owner named (their WittyRelays, and relays of their own), by ID.
func (s *Server) ownRelayIDs(ctx context.Context) map[string]bool {
	cfg := s.loadP2P(ctx)
	own := map[string]bool{}
	addrs := append([]string{}, cfg.StaticRelays...)
	for _, r := range cfg.OwnRelays {
		addrs = append(addrs, r.Addr)
	}
	for _, a := range addrs {
		if ai, err := p2p.ParseRelayAddr(a); err == nil {
			own[ai.ID.String()] = true
		}
	}
	return own
}

// qrFit cuts addresses from the end, then the name, until the link fits; with keepRelay it never cuts the first
// relayed address (nor what ranks before it), and says false when that can't fit.
func qrFit(inv p2p.Invite, home []string, own map[string]bool, max int, keepRelay bool) (string, bool) {
	// 0 the owner's relays' circuits, 1 public addresses, 2 any other relay's circuit (an ordinary public relay,
	// which carries the invite and lets the two ends try for a direct connection), 3 this server's addresses at home,
	// 4 other private ones. Away from home the first three are what work; at home the QR code's web address does.
	rank := func(a string) int {
		parts := strings.Split(a, "/")
		if r := circuitRelay(a); r != "" {
			if own[r] {
				return 0
			}
			return 2
		}
		if len(parts) < 3 {
			return -1
		}
		ip, err := netip.ParseAddr(parts[2])
		if err != nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return -1
		}
		if !ip.IsPrivate() {
			return 1
		}
		for _, h := range home {
			if strings.Contains(h, "//"+ip.String()+":") {
				return 3
			}
		}
		return 4
	}
	var addrs []string
	ownCount := 0 // how many addresses, from the start, are kept before the name is cut: up to the first relayed one
	for want := 0; want <= 4; want++ {
		for _, a := range inv.Addrs {
			if rank(a) == want {
				addrs = append(addrs, a)
				if (want == 0 || want == 2) && ownCount == 0 {
					ownCount = len(addrs)
				}
			}
		}
	}
	// Other addresses go first, then the name (only shown), and the owner's relay circuits last: from away they
	// may be the only way in.
	short := inv
	fit := func(keep int) (string, bool) {
		for n := len(addrs); n >= keep; n-- {
			short.Addrs = addrs[:n]
			if link := short.String(); len(link) <= max {
				return link, true
			}
		}
		for r := []rune(short.Name); len(r) > 0; r = r[:len(r)-1] {
			short.Name = string(r[:len(r)-1])
			if link := short.String(); len(link) <= max {
				return link, true
			}
		}
		return "", false
	}
	if !keepRelay {
		ownCount = 0
	}
	if link, ok := fit(ownCount); ok {
		return link, true
	}
	if keepRelay {
		return "", false
	}
	return short.String(), false
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
	// Hint is what to do about it, when this server can tell: in a container, the port to publish.
	Hint string `json:"hint,omitempty"`
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
	// In a container, the peer-to-peer port is reached only if it was published as well as the web port (the
	// stock compose file published only the web port before 0.8.5), and forwarded on the router or helped by a
	// WittyRelay: from inside, the server can't ask the router itself.
	if node != nil && ffmpeg.InContainer() {
		port := node.Port()
		if _, ip, bridged := connectHome(r); bridged && !hostPortOpen(ip, port) {
			// On Docker's bridge network, with the page opened at the host's address: that address's port answers
			// only if Docker publishes it. Nothing answering there is the owner's 0.8.4 setup exactly.
			st.Hint = i18n.F("Port {port} isn't published from Docker, so devices away from home can't connect peer to peer. Add \"{port}:{port}/tcp\" and \"{port}:{port}/udp\" to ports: in your compose file and recreate the container, then forward the port on your router or use your own WittyRelay. Devices at home can scan the QR code.",
				"port", fmt.Sprint(port))
			if st.Away != "yes" {
				st.Away, st.Words = "no", i18n.T("Devices away from home can't connect right now.")
			}
		} else if st.Away != "yes" {
			st.Hint = i18n.F("This server runs in a container. For devices away from home, its peer-to-peer port {port} (UDP and TCP) has to be published as well as the web port, and forwarded on your router, or use your own WittyRelay. Devices at home can scan the QR code.",
				"port", fmt.Sprint(port))
		}
	}
	return st
}

// hostPortOpen says whether the container's host answers on a TCP port: Docker publishes it to this container (it
// answers here, through the host) or nothing does. Quick: refused at once, or a short wait on a network that drops.
func hostPortOpen(ip netip.Addr, port int) bool {
	if !ip.IsValid() || port <= 0 {
		return false
	}
	c, err := net.DialTimeout("tcp", netip.AddrPortFrom(ip, uint16(port)).String(), 700*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
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
	defer s.p2pSavePending() // kept for a restart: p2p_pending.go
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
	// Where devices at home find this server: the address this page was opened at first. In a container on
	// Docker's bridge network the server's own addresses are the container's, which nothing else reaches.
	home, homeIP, bridged := connectHome(r)
	homes := s.homeURLs()
	if bridged {
		homes = []string{}
	}
	if home != "" && !slices.Contains(homes, home) {
		homes = append([]string{home}, homes...)
	}
	out := map[string]any{"code": code, "expiresInSec": int(connectTTL.Seconds()), "user": target, "homeAddresses": homes}
	// From away as well: the link. A profile kept at home has no use for it.
	away := false
	if node := s.p2pNode(); node != nil && accessFor(target).Can(library.FeatureRemote) {
		secret, err := s.p2p.invites.makeShared(me.ID, target.ID, tokenFrom(r), o.spent)
		if err == nil {
			s.p2pInviteOpened()
			logging.AddSecret(secret)
			addrs := node.Addrs()
			if bridged {
				addrs = append(hostHints(homeIP, node.Port()), addrs...)
			}
			own := s.ownRelayIDs(r.Context())
			inv := p2p.Invite{ServerID: node.ID(), Secret: secret, Name: s.ServerName(r.Context()), Addrs: inviteAddrs(addrs, own)}
			out["link"] = inv.String()
			// The QR code is scanned by a device in the same room, so it holds the code shown beside it too.
			qr := inv
			if clean, err := p2p.CleanJoinCode(code); err == nil && home != "" {
				qr.Home, qr.Code = home, clean
			}
			out["qrLink"] = qrInvite(qr, homes, own, connectQRMax)
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
		s.p2pSavePending()
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
		// Codes don't outlive a restart (p2p_pending.go): one shown just before it is told so.
		if s.p2pCodeEndedByRestart(0) {
			writeJSON(w, 404, map[string]any{"error": p2pCodeRestartedWords, "reason": "restarted"})
			return
		}
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
	defer s.p2pSavePending() // used, or one wrong guess nearer being given up
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
