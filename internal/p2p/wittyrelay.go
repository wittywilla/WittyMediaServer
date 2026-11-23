// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	ma "github.com/multiformats/go-multiaddr"
)

// WittyRelay is a relay anyone can run in Docker (its own project) for the
// times no direct connection can be made: both ends behind strict routers,
// a network that lets no UDP out, a provider's shared address on each side.
// It is libp2p's circuit relay v2, the same relays this node already uses,
// with three things added that a relay run for strangers needs:
//
//   - A server registers before it may hold a place: it proves its ID (libp2p
//     does that on every connection) and, if the relay's operator wants, shows
//     a key the operator gave out.
//   - With the registration the server says which devices may reach it
//     through the relay. That list is the server's word, renewed every few
//     minutes and forgotten by the relay a quarter of an hour after it last
//     heard it, so a device removed on the server loses the relay soon too.
//     A device that isn't on its server's list can't use the relay at all.
//   - Limits, said before use (ProtoRelayInfo): speed per connection and in
//     all, data and time per connection, picture size.
//
// What is relayed stays encrypted between the device and the server: the
// relay passes on bytes it can't read, as every relay here does.

// ProtoRelayRegister is how a server registers with a WittyRelay: one
// RegisterRequest, one RegisterResponse, in enrolment's framing.
const ProtoRelayRegister protocol.ID = "/wittyrelay/register/1.0.0"

// RegisterRequest is what a server tells a WittyRelay.
type RegisterRequest struct {
	V   int    `json:"v"`
	Key string `json:"key,omitempty"` // the access key the relay's operator gave, if it needs one
	// Name is the server's name, for the relay's operator to recognise it.
	Name string `json:"name,omitempty"`
	// Devices are the IDs that may reach this server through the relay.
	Devices []string `json:"devices"`
}

// RegisterResponse is the relay's answer.
type RegisterResponse struct {
	OK bool `json:"ok"`
	// Reason: key (wrong or missing key) | full | busy | bad | off.
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
	// TTLSec is how long the registration holds without being renewed.
	TTLSec int        `json:"ttlSec,omitempty"`
	Offer  RelayOffer `json:"offer"`
}

// MaxRegisterDevices is the most devices one registration lists: the most a
// server keeps (512), with room to spare.
const MaxRegisterDevices = 1024

// maxRegisterMessage: a registration with every device listed is about 54 kB.
const maxRegisterMessage = 64 << 10

// OwnRelay is a WittyRelay the owner pointed this server at.
type OwnRelay struct {
	Addr string // a multiaddr ending /p2p/<the relay's ID>
	Key  string
}

// OwnRelayStatus says how registering with one is going.
type OwnRelayStatus struct {
	Addr  string `json:"addr"`
	ID    string `json:"id"`
	State string `json:"state"` // ok | refused | unreachable | wait
	Error string `json:"error,omitempty"`
	// Offer is what it said it allows, and Allows the same in words.
	Offer   *RelayOffer `json:"offer,omitempty"`
	Allows  string      `json:"allows,omitempty"`
	Renewed int64       `json:"renewed,omitempty"`
}

// ParseRelayAddr reads a relay's address as the owner pastes it: a multiaddr
// ending in /p2p/<ID>, or the same with "wittyrelay://" before it, which is
// how a WittyRelay prints it, so that it reads as an address.
func ParseRelayAddr(s string) (peer.AddrInfo, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "wittyrelay://")
	if len(s) > 300 {
		return peer.AddrInfo{}, errors.New("that is too long to be a relay's address")
	}
	a, err := ma.NewMultiaddr(s)
	if err != nil {
		return peer.AddrInfo{}, errors.New("that isn't a relay's address: it looks like /dns4/relay.example.com/udp/4001/quic-v1/p2p/12D3KooW…")
	}
	ai, err := peer.AddrInfoFromP2pAddr(a)
	if err != nil || len(ai.Addrs) == 0 {
		return peer.AddrInfo{}, errors.New("that address doesn't end in /p2p/ and the relay's ID")
	}
	if _, err := CheckID(ai.ID.String()); err != nil {
		return peer.AddrInfo{}, errors.New("that address doesn't end in a relay's ID")
	}
	return *ai, nil
}

// registerEvery is how often a registration is renewed; the relay forgets
// one after registerTTL by default.
const registerEvery = 5 * time.Minute

// registerRetry is how soon a registration the relay didn't take is tried again: just over its
// one-minute window for registrations.
const registerRetry = 65 * time.Second

// Register tells a WittyRelay about this server once and returns its answer.
func (h *Host) Register(ctx context.Context, relay peer.AddrInfo, req RegisterRequest) (RegisterResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	h.Peerstore().AddAddrs(relay.ID, relay.Addrs, peerstore.PermanentAddrTTL)
	if err := h.Connect(ctx, relay); err != nil {
		return RegisterResponse{}, fmt.Errorf("the relay couldn't be reached: %w", plainDialError(err))
	}
	s, err := h.NewStream(ctx, relay.ID, ProtoRelayRegister)
	if err != nil {
		return RegisterResponse{}, errors.New("that is a relay, but not a WittyRelay (it doesn't take registrations)")
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(20 * time.Second))
	req.V = 1
	if len(req.Devices) > MaxRegisterDevices {
		req.Devices = req.Devices[:MaxRegisterDevices]
	}
	if err := writeFrame(s, req, maxRegisterMessage); err != nil {
		return RegisterResponse{}, err
	}
	var res RegisterResponse
	if err := readFrame(s, &res, maxEnrolMessage); err != nil {
		return RegisterResponse{}, errors.New("the relay didn't answer")
	}
	res.Offer = res.Offer.Clean()
	if res.OK {
		h.offersMu.Lock()
		if h.offers == nil {
			h.offers = map[peer.ID]keptOffer{}
		}
		h.offers[relay.ID] = keptOffer{offer: res.Offer, at: time.Now()}
		h.offersMu.Unlock()
	}
	return res, nil
}

// keepRegistered registers with each of the owner's WittyRelays now and
// every few minutes, and at once when the list of devices changes.
func (n *Node) keepRegistered() {
	defer n.wg.Done()
	if len(n.cfg.OwnRelays) == 0 {
		close(n.registered)
		return
	}
	first := true
	for {
		wait := registerEvery
		for i, r := range n.cfg.OwnRelays {
			if !n.registerOnce(i, r) {
				// A relay that said "busy" (too many registrations from this network in a minute) keeps
				// the list it had: an invite opened just now isn't on it. Try again soon, not in minutes.
				wait = registerRetry
			}
		}
		if first {
			first = false
			close(n.registered)
		}
		select {
		case <-n.ctx.Done():
			return
		case <-time.After(wait):
		case <-n.devicesChanged:
		}
	}
}

// registerOnce registers with one relay, and says whether it took.
func (n *Node) registerOnce(i int, r OwnRelay) bool {
	st := OwnRelayStatus{Addr: r.Addr, State: "unreachable"}
	ai, err := ParseRelayAddr(r.Addr)
	if err != nil {
		st.State, st.Error = "refused", err.Error()
		n.setOwnRelay(i, st)
		return true // a setting that can't work: trying sooner changes nothing
	}
	st.ID = ai.ID.String()
	var devices []string
	if n.cfg.DeviceIDs != nil {
		devices = n.cfg.DeviceIDs()
	}
	res, err := n.host.Register(n.ctx, ai, RegisterRequest{Key: r.Key, Name: n.cfg.Name, Devices: devices})
	if n.ctx.Err() != nil {
		return true
	}
	n.count.registrations.Add(1)
	if err != nil || !res.OK {
		n.count.registrationsBad.Add(1)
	}
	switch {
	case err != nil:
		st.Error = err.Error()
	case !res.OK:
		st.State, st.Error = "refused", res.Error
		if st.Error == "" {
			st.Error = "the relay said no (" + res.Reason + ")"
		}
	default:
		st.State, st.Renewed = "ok", time.Now().Unix()
		o := res.Offer
		st.Offer, st.Allows = &o, o.Words()
		// A place on it, asked for here rather than left to libp2p's search
		// for relays, which only starts once other computers have told this
		// one it can't be reached: with few of them about (the address book
		// off, a quiet network) that may never happen, and the owner's relay
		// would go unused. Renewed with every registration.
		if err := n.reserve(i, ai); err != nil {
			st.State, st.Error = "refused", "registered, but the relay gave no place: "+err.Error()
		}
	}
	if st.State != "ok" {
		n.mu.Lock()
		delete(n.ownCircuits, i)
		n.mu.Unlock()
	}
	prev := n.ownRelay(i)
	if prev.State != st.State || prev.Error != st.Error {
		if st.State == "ok" {
			n.log.Info("peer-to-peer: registered with your WittyRelay", "relay", ShortID(st.ID), "devices", len(devices), "allows", st.Allows)
		} else {
			n.log.Warn("peer-to-peer: couldn't register with your WittyRelay", "relay", ShortID(st.ID), "state", st.State, "err", st.Error)
		}
	}
	n.setOwnRelay(i, st)
	return st.State == "ok"
}

// reserve holds a place on a relay and keeps this server's addresses
// through it, to announce.
func (n *Node) reserve(i int, ai peer.AddrInfo) error {
	ctx, cancel := context.WithTimeout(n.ctx, 30*time.Second)
	defer cancel()
	res, err := client.Reserve(ctx, n.host, ai)
	if err != nil {
		return err
	}
	addrs := res.Addrs
	if len(addrs) == 0 {
		addrs = ai.Addrs
	}
	var circuits []ma.Multiaddr
	for _, a := range addrs {
		if _, err := a.ValueForProtocol(ma.P_P2P); err != nil {
			a = a.Encapsulate(ma.StringCast("/p2p/" + ai.ID.String()))
		}
		circuits = append(circuits, a.Encapsulate(ma.StringCast("/p2p-circuit")))
	}
	n.mu.Lock()
	if n.ownCircuits == nil {
		n.ownCircuits = map[int][]ma.Multiaddr{}
	}
	n.ownCircuits[i] = circuits
	n.mu.Unlock()
	n.host.ConnManager().Protect(ai.ID, "wittyrelay")
	return nil
}

func (n *Node) ownRelay(i int) OwnRelayStatus {
	n.mu.Lock()
	defer n.mu.Unlock()
	if i < len(n.ownRelays) {
		return n.ownRelays[i]
	}
	return OwnRelayStatus{}
}

func (n *Node) setOwnRelay(i int, st OwnRelayStatus) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if i < len(n.ownRelays) {
		n.ownRelays[i] = st
	}
}

// DevicesChanged tells the node the list of devices changed, so its
// WittyRelays hear the new list now rather than at the next renewal.
func (n *Node) DevicesChanged() { poke(n.devicesChanged) }

// RelayStreamLimit is the rate a stream through this relay is paced to: the
// owner's own limit or the relay's, whichever is lower (0: none).
func (n *Node) RelayStreamLimit(relay string) int64 {
	limit := n.cfg.RelayBitrate
	if o, ok := n.RelayOffer(relay); ok && o.Kbps > 0 {
		if r := int64(o.Kbps) * 1000; limit == 0 || r < limit {
			limit = r
		}
	}
	return limit
}

// ---------------------------------------------------------------- framing

// writeFrame and readFrame are enrolment's framing with a size of their own.
func writeFrame(s network.Stream, v any, limit int) error {
	return writeMessageN(s, v, limit)
}

func readFrame(s network.Stream, v any, limit int) error {
	return readMessageN(s, v, limit)
}

// CheckRelay asks a relay what it allows, from a node of its own with a key
// made for the purpose, before the owner chooses to use it. Nothing is
// registered and nobody else is contacted. opts are for tests.
func CheckRelay(ctx context.Context, addr string, opts ...libp2p.Option) (RelayOffer, error) {
	ai, err := ParseRelayAddr(addr)
	if err != nil {
		return RelayOffer{}, err
	}
	seed, err := NewSeed()
	if err != nil {
		return RelayOffer{}, err
	}
	h, err := NewHost(HostConfig{Seed: seed, Listen: []string{}, Options: opts})
	if err != nil {
		return RelayOffer{}, err
	}
	defer h.Close()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := h.Connect(ctx, ai); err != nil {
		return RelayOffer{}, fmt.Errorf("the relay couldn't be reached: %w", plainDialError(err))
	}
	o, err := h.FetchRelayOffer(ctx, ai.ID)
	if err != nil {
		return RelayOffer{}, errors.New("something answered at that address, but it isn't a WittyRelay (it didn't say what it allows)")
	}
	if got, _ := h.Peerstore().SupportsProtocols(ai.ID, ProtoRelayRegister); len(got) == 0 {
		return o, errors.New("that relay says what it allows, but takes no registrations: it is another server's community relay, not a WittyRelay")
	}
	return o, nil
}
