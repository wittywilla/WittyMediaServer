// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multihash"
	"golang.org/x/crypto/argon2"
)

// Joining by a code is an invite the other way round, for a device with no
// keyboard and no camera: a TV away from home. An invite link is long, and a
// TV has no way to take one in. So the TV makes up a short code and shows it;
// someone signed in types it into the server (POST /api/v1/p2p/join); and the
// two find each other through the address book:
//
//  1. Both turn the code into two keys with Argon2id: one to look each other
//     up by, one to prove they know the code.
//  2. The server announces itself in the address book under the first key
//     (a provider record, as community relays do under theirs). The TV asks
//     the address book who is under that key, and so learns the server's
//     connection ID and where it is.
//  3. The TV connects and each proves to the other, with the second key and
//     both their IDs, that it knows the code, without sending it. Only then
//     does the server put the TV's key on its list, exactly as for an invite.
//
// What a stranger can do: the address book's own computers see the first
// key, and anyone can announce themselves under it. Neither gets them past
// step 3: a made-up server can't prove it knows the code, and the TV sends
// such a one nothing but a random number. Guessing the code from what is
// seen costs an Argon2id run per guess (40 bits of code, tens of milliseconds
// and megabytes each: centuries), and the code is good for fifteen minutes
// and once. Guessing at the server itself is held to a few tries.

// ProtoJoin is how a device that shows a code gets onto the server's list.
const ProtoJoin protocol.ID = "/wittymedia/join/1.0.0"

// joinAlphabet has no I, L, O or U: nothing that reads as a digit, or as
// another letter, across a room.
const joinAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// JoinCodeLen is how many symbols a code has: 40 bits.
const JoinCodeLen = 8

// NewJoinCode makes the code a device shows, as it is typed: "K7QM-2XHD".
func NewJoinCode() (string, error) {
	raw := make([]byte, JoinCodeLen)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	var b strings.Builder
	for i, v := range raw {
		if i == JoinCodeLen/2 {
			b.WriteByte('-')
		}
		b.WriteByte(joinAlphabet[int(v)%len(joinAlphabet)]) // 32 symbols: no bias
	}
	return b.String(), nil
}

// CleanJoinCode reads a code as a person typed it: either case, with or
// without the dash or spaces, and the letters that look like digits taken for
// them. An error means it can't be one.
func CleanJoinCode(typed string) (string, error) {
	var b strings.Builder
	for _, r := range strings.ToUpper(typed) {
		switch r {
		case ' ', '-', '\t':
			continue
		case 'O':
			r = '0'
		case 'I', 'L':
			r = '1'
		}
		if r > 127 || !strings.ContainsRune(joinAlphabet, r) {
			return "", errors.New("that isn't a code a device shows: it has eight letters and digits")
		}
		b.WriteRune(r)
	}
	if b.Len() != JoinCodeLen {
		return "", errors.New("that isn't a code a device shows: it has eight letters and digits")
	}
	return b.String(), nil
}

// FormatJoinCode writes a clean code as it is shown.
func FormatJoinCode(code string) string {
	if len(code) != JoinCodeLen {
		return code
	}
	return code[:JoinCodeLen/2] + "-" + code[JoinCodeLen/2:]
}

// joinKeys are what a code becomes.
type joinKeys struct {
	find  []byte // what the two look each other up by; the address book sees it
	proof []byte // what proves knowing the code; never sent
}

// deriveJoin costs about 32 MB and some tens of milliseconds, once at each
// end, and the same for every guess anyone makes.
func deriveJoin(code string) joinKeys {
	out := argon2.IDKey([]byte(code), []byte("wittymedia-join-v1"), 2, 32<<10, 1, 64)
	return joinKeys{find: out[:32], proof: out[32:]}
}

func joinCID(find []byte) cid.Cid {
	h, _ := multihash.Sum(append([]byte("wittymedia-join-v1\n"), find...), multihash.SHA2_256, -1)
	return cid.NewCidV1(cid.Raw, h)
}

// joinProof binds the code to this one conversation: both random numbers and
// both ends' IDs, which libp2p has already proved. A proof heard on one
// connection is no use on another.
func joinProof(key []byte, role string, deviceNonce, serverNonce []byte, server, device peer.ID) []byte {
	m := hmac.New(sha256.New, key)
	for _, part := range [][]byte{[]byte("wittymedia join 1"), []byte(role), deviceNonce, serverNonce, []byte(server), []byte(device)} {
		var n [2]byte
		n[0], n[1] = byte(len(part)>>8), byte(len(part))
		m.Write(n[:])
		m.Write(part)
	}
	return m.Sum(nil)
}

// On the wire: the same framing as enrolment, four messages.
type joinHello struct {
	V        int    `json:"v"`
	Find     string `json:"find"`  // the lookup key, in hex: which code this is about
	Nonce    string `json:"nonce"` // 16 random bytes, base64
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

type joinChallenge struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
	Nonce  string `json:"nonce,omitempty"`
	Proof  string `json:"proof,omitempty"` // the server's, first: it has to show it knows the code before the device shows anything
}

type joinAnswer struct {
	Proof string `json:"proof"`
}

const (
	joinNonceLen = 16
	// JoinTTL is how long a code typed into the server waits for its device.
	JoinTTL = 15 * time.Minute
	// maxJoins is how many codes can be waiting at once.
	maxJoins     = 16
	maxJoinWrong = 3
)

// ErrJoinNeedsDHT: without the address book there is nowhere for a device
// that knows only a code to look the server up.
var ErrJoinNeedsDHT = errors.New("joining by a code needs the address book, which is turned off on this server")

// ErrJoinBusy: too many codes are waiting.
var ErrJoinBusy = errors.New("too many codes are waiting for their devices")

type pendingJoin struct {
	proof   []byte
	secret  string // the invite this code stands for
	expires time.Time
	wrong   int
}

type joins struct {
	mu   sync.Mutex
	open map[string]*pendingJoin // by the lookup key, in hex
}

// OfferJoin makes the server findable by a device showing this code, for
// JoinTTL. secret is an invite's: what the code is worth is exactly what that
// invite is worth, and whatever ends the invite ends the code. It returns as
// soon as the code is waiting; telling the address book goes on behind.
func (n *Node) OfferJoin(code, secret string) error {
	clean, err := CleanJoinCode(code)
	if err != nil {
		return err
	}
	if n.host.DHT == nil {
		return ErrJoinNeedsDHT
	}
	keys := deriveJoin(clean)
	name := hex.EncodeToString(keys.find)
	now := time.Now()
	n.joins.mu.Lock()
	if n.joins.open == nil {
		n.joins.open = map[string]*pendingJoin{}
	}
	for k, v := range n.joins.open {
		if now.After(v.expires) {
			delete(n.joins.open, k)
		}
	}
	if _, again := n.joins.open[name]; !again && len(n.joins.open) >= maxJoins {
		n.joins.mu.Unlock()
		return ErrJoinBusy
	}
	n.joins.open[name] = &pendingJoin{proof: keys.proof, secret: secret, expires: now.Add(JoinTTL)}
	n.joins.mu.Unlock()

	n.wg.Add(1)
	Spawn("p2p join announce", func() {
		defer n.wg.Done()
		// The address book takes a while to be told, and a node that has only
		// just started may not have found it yet: a few tries, while the code
		// is still waiting.
		for try, wait := 0, time.Duration(0); try < 4; try, wait = try+1, 20*time.Second {
			select {
			case <-n.ctx.Done():
				return
			case <-time.After(wait):
			}
			if !n.joinWaiting(name) {
				return
			}
			ctx, cancel := context.WithTimeout(n.ctx, 90*time.Second)
			err := n.host.DHT.Provide(ctx, joinCID(keys.find), true)
			cancel()
			if err == nil {
				return
			}
			n.log.Info("could not tell the address book about a code that is waiting; trying again", "err", err)
		}
	})
	return nil
}

// TakesCodes says a device showing a code could find this server: the address
// book is on.
func (n *Node) TakesCodes() bool { return n.host.DHT != nil }

func (n *Node) joinWaiting(name string) bool {
	n.joins.mu.Lock()
	defer n.joins.mu.Unlock()
	v := n.joins.open[name]
	return v != nil && time.Now().Before(v.expires)
}

// handleJoin answers a device that says it shows a code. Nothing is done for
// it, and nothing about the code is said to it beyond the server's own proof,
// until its proof has been checked.
func (n *Node) handleJoin(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(20 * time.Second))
	device := s.Conn().RemotePeer()
	addr := n.remoteOf(s, false)
	refuse := func(reason, msg string) { _ = writeMessage(s, joinChallenge{Reason: reason, Error: msg}) }
	// The same counters as invites: a stranger gets no more tries by asking another way.
	okAll, okPeer, okAddr := n.enrolAll.allow("all"), n.enrolPeer.allow(device.String()), n.enrolAddr.allow(addr.IP.String())
	if !okAll || !okPeer || !okAddr {
		n.noteRefused(s, "too many tries at a code")
		refuse("too-many-tries", "Too many tries. Wait a minute and try again.")
		return
	}
	var hello joinHello
	if err := readMessage(s, &hello); err != nil || hello.V != 1 {
		refuse("wrong", "That isn't a code this server understands.")
		return
	}
	deviceNonce, err := base64.StdEncoding.DecodeString(hello.Nonce)
	if err != nil || len(deviceNonce) != joinNonceLen {
		refuse("wrong", "That isn't a code this server understands.")
		return
	}
	n.joins.mu.Lock()
	waiting := n.joins.open[hello.Find]
	var proofKey []byte
	if waiting != nil && time.Now().Before(waiting.expires) {
		proofKey = waiting.proof
	}
	n.joins.mu.Unlock()
	if proofKey == nil || n.cfg.Enrol == nil {
		refuse("wrong", "No such code is waiting on this server.")
		return
	}
	serverNonce := make([]byte, joinNonceLen)
	if _, err := rand.Read(serverNonce); err != nil {
		refuse("refused", "The server couldn't answer.")
		return
	}
	me := n.host.ID()
	if err := writeMessage(s, joinChallenge{OK: true, Nonce: base64.StdEncoding.EncodeToString(serverNonce),
		Proof: base64.StdEncoding.EncodeToString(joinProof(proofKey, "server", deviceNonce, serverNonce, me, device))}); err != nil {
		return
	}
	var answer joinAnswer
	if err := readMessage(s, &answer); err != nil {
		return
	}
	got, _ := base64.StdEncoding.DecodeString(answer.Proof)
	n.joins.mu.Lock()
	waiting = n.joins.open[hello.Find]
	good := waiting != nil && time.Now().Before(waiting.expires) && hmac.Equal(got, joinProof(waiting.proof, "device", deviceNonce, serverNonce, me, device))
	secret := ""
	switch {
	case good:
		secret = waiting.secret
		delete(n.joins.open, hello.Find) // once
	case waiting != nil:
		// Someone who found the lookup key and is guessing at the code: the
		// code is given up after a few, and its device shows another.
		if waiting.wrong++; waiting.wrong >= maxJoinWrong {
			delete(n.joins.open, hello.Find)
		}
	}
	n.joins.mu.Unlock()
	if !good {
		n.noteRefused(s, "a wrong proof for a code")
		_ = writeMessage(s, EnrolResponse{Reason: "wrong", Error: "That code isn't right."})
		return
	}
	ctx, cancel := context.WithTimeout(n.ctx, 10*time.Second)
	defer cancel()
	res := n.cfg.Enrol(ctx, device.String(), EnrolRequest{V: 1, Secret: secret, Name: hello.Name, Platform: hello.Platform}, addr.IP)
	if res.OK {
		res.Addrs = n.Addrs()
	}
	_ = writeMessage(s, res)
}

// ---------------------------------------------------------------- the device's side

// JoinConfig is what a device that shows a code looks for its server with.
type JoinConfig struct {
	Seed     []byte // this device's key
	Code     string // as NewJoinCode made it
	Name     string // what to call the device in the server's list
	Platform string
	// Layers: the address book can't be left out, as it is how the server is
	// found; relays and hole punching are how a server behind a router is reached.
	Layers       Layers
	StaticRelays []string

	// For tests: see HostConfig. Every is how long to wait between looks.
	Bootstrap []peer.AddrInfo
	Local     bool
	Listen    []string
	Every     time.Duration
}

// JoinResult is the server found, and its answer.
type JoinResult struct {
	ServerID string `json:"serverId"`
	EnrolResponse
}

// Join waits until a server has been told this device's code, finds it, and
// has this device's key put on its list. It returns when that has happened,
// when the server refused, or when ctx ends (give it the code's fifteen
// minutes). The node it runs is its own and is gone when it returns: what
// comes back is what a connection is then started with.
func Join(ctx context.Context, cfg JoinConfig) (JoinResult, error) {
	code, err := CleanJoinCode(cfg.Code)
	if err != nil {
		return JoinResult{}, err
	}
	layers := cfg.Layers
	layers.DHT, layers.PortMap, layers.IPv6 = true, false, false
	h, err := NewHost(HostConfig{Seed: cfg.Seed, Layers: layers, StaticRelays: cfg.StaticRelays, Bootstrap: cfg.Bootstrap, Local: cfg.Local, Listen: cfg.Listen})
	if err != nil {
		return JoinResult{}, err
	}
	defer h.Close()
	keys := deriveJoin(code)
	key := joinCID(keys.find)
	every := cfg.Every
	if every <= 0 {
		every = 4 * time.Second
	}
	// A node that announced itself under the key and couldn't prove it knows
	// the code is not asked twice.
	liars := map[peer.ID]bool{h.ID(): true}
	for {
		fctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		for found := range h.DHT.FindProvidersAsync(fctx, key, 8) {
			if liars[found.ID] {
				continue
			}
			res, verdict := joinWith(fctx, h, found, keys, cfg)
			switch verdict {
			case joinDone:
				cancel()
				return res, nil
			case joinNotIt:
				liars[found.ID] = true
			case joinLater:
			}
		}
		cancel()
		select {
		case <-ctx.Done():
			return JoinResult{}, errors.New("no server took the code in time")
		case <-time.After(every):
		}
		if every < 10*time.Second {
			every += time.Second
		}
	}
}

type joinVerdict int

const (
	joinDone  joinVerdict = iota // the server answered: yes, or a no that is final
	joinNotIt                    // whoever this is, it doesn't know the code
	joinLater                    // couldn't be reached or asked just now
)

func joinWith(ctx context.Context, h *Host, found peer.AddrInfo, keys joinKeys, cfg JoinConfig) (JoinResult, joinVerdict) {
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	if len(found.Addrs) == 0 {
		if ai, err := h.DHT.FindPeer(cctx, found.ID); err == nil {
			found.Addrs = ai.Addrs
		}
	}
	h.Peerstore().AddAddrs(found.ID, found.Addrs, peerstore.TempAddrTTL)
	if err := h.Connect(cctx, peer.AddrInfo{ID: found.ID}); err != nil {
		return JoinResult{}, joinLater
	}
	s, err := h.NewStream(network.WithAllowLimitedConn(cctx, "wittymedia"), found.ID, ProtoJoin)
	if err != nil {
		return JoinResult{}, joinNotIt // not a WittyMediaServer that takes codes
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(20 * time.Second))
	deviceNonce := make([]byte, joinNonceLen)
	if _, err := rand.Read(deviceNonce); err != nil {
		return JoinResult{}, joinLater
	}
	if err := writeMessage(s, joinHello{V: 1, Find: hex.EncodeToString(keys.find), Nonce: base64.StdEncoding.EncodeToString(deviceNonce),
		Name: cfg.Name, Platform: cfg.Platform}); err != nil {
		return JoinResult{}, joinLater
	}
	var challenge joinChallenge
	if err := readMessage(s, &challenge); err != nil {
		return JoinResult{}, joinLater
	}
	if !challenge.OK {
		if challenge.Reason == "too-many-tries" {
			return JoinResult{}, joinLater
		}
		return JoinResult{}, joinNotIt
	}
	serverNonce, err1 := base64.StdEncoding.DecodeString(challenge.Nonce)
	theirs, err2 := base64.StdEncoding.DecodeString(challenge.Proof)
	if err1 != nil || err2 != nil || len(serverNonce) != joinNonceLen ||
		!hmac.Equal(theirs, joinProof(keys.proof, "server", deviceNonce, serverNonce, found.ID, h.ID())) {
		// It announced itself under the code's key without knowing the code.
		// It is told nothing more.
		_ = s.Reset()
		return JoinResult{}, joinNotIt
	}
	if err := writeMessage(s, joinAnswer{Proof: base64.StdEncoding.EncodeToString(joinProof(keys.proof, "device", deviceNonce, serverNonce, found.ID, h.ID()))}); err != nil {
		return JoinResult{}, joinLater
	}
	var res EnrolResponse
	if err := readMessage(s, &res); err != nil {
		return JoinResult{}, joinLater
	}
	// The server that knows the code has spoken: its yes or its no is the answer.
	if len(res.Addrs) == 0 {
		res.Addrs = addrStrings(h.Peerstore().Addrs(found.ID))
	}
	return JoinResult{ServerID: found.ID.String(), EnrolResponse: res}, joinDone
}
