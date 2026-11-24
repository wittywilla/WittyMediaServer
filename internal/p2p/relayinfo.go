// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	pbv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/pb"
)

// A relay that carries whole connections (a community relay, or a WittyRelay)
// says what it allows before anyone sends anything through it: how fast, how
// much, for how long, and what picture size streams through it should keep
// to. Servers show this in Settings before it is used, and keep to it.
//
// The relay can enforce speed, amount and time itself. It can't enforce a
// picture size: what it carries is encrypted end to end and it can't see
// inside. The server keeps to the size the relay asks for, for every stream
// it sends through that relay, and the speed limit holds anyone who doesn't.

// ProtoRelayInfo is answered with one RelayOffer, in enrolment's framing, and
// the stream ends.
const ProtoRelayInfo protocol.ID = "/wittymedia/relay-info/1.0.0"

// RelayOffer is what a relay allows.
type RelayOffer struct {
	V        int    `json:"v"`
	Software string `json:"software,omitempty"` // "WittyMediaServer 0.8.1-alpha", "WittyRelay 0.1.0"
	Name     string `json:"name,omitempty"`
	// Note is the relay's operator's own words: who runs it, how to reach them.
	Note string `json:"note,omitempty"`
	// Access: "open" (any server may hold a place) or "key" (servers its
	// operator gave a key to).
	Access string `json:"access"`
	// Kbps is the most one connection may use, each way; TotalKbps the most
	// the relay gives everyone together. 0: no limit.
	Kbps      int `json:"kbps,omitempty"`
	TotalKbps int `json:"totalKbps,omitempty"`
	// MaxHeight is the picture height, in lines, that streams through the relay
	// should keep to (720 for 720p). 0: any.
	MaxHeight int `json:"maxHeight,omitempty"`
	// DataMiB and Minutes end one connection; a new one starts at once.
	DataMiB int64 `json:"dataMiB,omitempty"`
	Minutes int   `json:"minutes,omitempty"`
	// Servers is how many servers may hold a place at once, Circuits how many
	// connections each may have through it.
	Servers  int `json:"servers,omitempty"`
	Circuits int `json:"circuits,omitempty"`
	// Health is how the relay is doing now, when it says (WittyRelay 0.2 and
	// later): told with its offer and with every registration's answer.
	Health *RelayHealth `json:"health,omitempty"`
}

// RelayHealth is how full a relay is. It is coarse: the relay tells anyone.
type RelayHealth struct {
	// State: ok, busy (most of what it allows is in use: expect it to be
	// slower), full (it takes no new servers; registered ones keep their
	// place) or starting.
	State string `json:"state"`
	// Load is the fullest of what it allows, as a percentage in steps of 10.
	Load int `json:"load"`
}

// Clean holds an offer read from someone else's relay to sane values: it is
// shown to people and decides limits, so nothing in it is taken as it comes.
func (o RelayOffer) Clean() RelayOffer {
	o.V = 1
	o.Software, o.Name, o.Note = CleanName(o.Software, 60), CleanName(o.Name, 60), CleanName(o.Note, 280)
	if o.Access != "key" {
		o.Access = "open"
	}
	rng := func(v, lo, hi int) int {
		if v <= 0 {
			return 0
		}
		return max(lo, min(v, hi))
	}
	o.Kbps = rng(o.Kbps, 100, 10_000_000)
	o.TotalKbps = rng(o.TotalKbps, 100, 100_000_000)
	o.MaxHeight = rng(o.MaxHeight, 144, 4320)
	o.Minutes = rng(o.Minutes, 1, 7*24*60)
	o.Servers = rng(o.Servers, 1, 1_000_000)
	o.Circuits = rng(o.Circuits, 1, 1024)
	if o.DataMiB < 0 {
		o.DataMiB = 0
	}
	o.DataMiB = min(o.DataMiB, 1<<30)
	if h := o.Health; h != nil {
		switch h.State {
		case "ok", "busy", "full", "starting":
			o.Health = &RelayHealth{State: h.State, Load: max(0, min(100, (h.Load+5)/10*10))}
		default:
			o.Health = nil
		}
	}
	return o
}

// Words says what the offer allows, for people.
func (o RelayOffer) Words() string {
	var parts []string
	switch {
	case o.Kbps > 0:
		parts = append(parts, "up to "+rateWords(o.Kbps)+" per connection")
	default:
		parts = append(parts, "no speed limit per connection")
	}
	if o.TotalKbps > 0 {
		parts = append(parts, rateWords(o.TotalKbps)+" for everyone together")
	}
	if o.MaxHeight > 0 {
		parts = append(parts, fmt.Sprintf("pictures up to %dp", o.MaxHeight))
	}
	if o.DataMiB > 0 || o.Minutes > 0 {
		var each []string
		if o.DataMiB > 0 {
			each = append(each, sizeWords(o.DataMiB))
		}
		if o.Minutes > 0 {
			each = append(each, minutesWords(o.Minutes))
		}
		parts = append(parts, strings.Join(each, " and ")+" per connection before it starts a new one")
	}
	s := strings.Join(parts, ", ")
	return strings.ToUpper(s[:1]) + s[1:] + "."
}

func rateWords(kbps int) string {
	if kbps >= 1000 {
		if kbps%1000 == 0 {
			return fmt.Sprintf("%d Mbit/s", kbps/1000)
		}
		return fmt.Sprintf("%.1f Mbit/s", float64(kbps)/1000)
	}
	return fmt.Sprintf("%d kbit/s", kbps)
}

func sizeWords(mib int64) string {
	if mib >= 1024 && mib%1024 == 0 {
		return fmt.Sprintf("%d GiB", mib/1024)
	}
	return fmt.Sprintf("%d MiB", mib)
}

func minutesWords(m int) string {
	switch {
	case m%60 == 0 && m >= 120:
		return fmt.Sprintf("%d hours", m/60)
	case m == 60:
		return "an hour"
	}
	return fmt.Sprintf("%d minutes", m)
}

// offerOf turns a node's own relay limits into what it says to others.
func offerOf(l RelayLimits) RelayOffer {
	o := RelayOffer{V: 1, Software: "WittyMediaServer", Access: "open", Note: l.Note,
		Kbps: l.Kbps, TotalKbps: l.TotalKbps, MaxHeight: l.MaxHeight,
		Servers: l.MaxReservations, Circuits: l.MaxCircuits}
	if l.Data > 0 {
		o.DataMiB = max(1, l.Data>>20)
	}
	if l.Duration > 0 {
		o.Minutes = max(1, int(l.Duration/time.Minute))
	}
	return o.Clean()
}

// serveOffer answers ProtoRelayInfo.
func serveOffer(o RelayOffer) network.StreamHandler {
	return func(s network.Stream) {
		defer s.Close()
		_ = s.SetDeadline(time.Now().Add(10 * time.Second))
		_ = writeMessage(s, o)
	}
}

// FetchRelayOffer asks a relay what it allows. A relay that doesn't answer
// the question (a public one, or a server older than this) gives an error.
func (h *Host) FetchRelayOffer(ctx context.Context, relay peer.ID) (RelayOffer, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	s, err := h.NewStream(network.WithAllowLimitedConn(ctx, "wittymedia"), relay, ProtoRelayInfo)
	if err != nil {
		return RelayOffer{}, err
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(10 * time.Second))
	var o RelayOffer
	if err := readMessage(s, &o); err != nil {
		return RelayOffer{}, err
	}
	o = o.Clean()
	h.offersMu.Lock()
	if h.offers == nil {
		h.offers = map[peer.ID]keptOffer{}
	}
	if len(h.offers) >= 64 {
		for k := range h.offers {
			delete(h.offers, k)
			break
		}
	}
	h.offers[relay] = keptOffer{offer: o, at: time.Now()}
	h.offersMu.Unlock()
	return o, nil
}

type keptOffer struct {
	offer RelayOffer
	at    time.Time
}

// offerKeep is how long an offer is believed before it is asked for again.
const offerKeep = time.Hour

// RelayOfferOf is what a relay last said it allows, if it said.
func (h *Host) RelayOfferOf(relay peer.ID) (RelayOffer, bool) {
	h.offersMu.Lock()
	defer h.offersMu.Unlock()
	k, ok := h.offers[relay]
	return k.offer, ok
}

// learnOffer asks a relay for its offer in the background unless it is known
// and fresh, then calls done (if not nil) when it was learnt.
func (h *Host) learnOffer(relay peer.ID, done func()) {
	h.offersMu.Lock()
	k, ok := h.offers[relay]
	busy := h.asking[relay]
	if (ok && time.Since(k.at) < offerKeep) || busy {
		h.offersMu.Unlock()
		return
	}
	if h.asking == nil {
		h.asking = map[peer.ID]bool{}
	}
	h.asking[relay] = true
	h.offersMu.Unlock()
	Spawn("p2p relay offer", func() {
		_, err := h.FetchRelayOffer(h.ctx, relay)
		h.offersMu.Lock()
		delete(h.asking, relay)
		h.offersMu.Unlock()
		if err == nil && done != nil {
			done()
		}
	})
}

// pacer is a community relay's limit on all it passes on together: libp2p's
// relay reports every piece it copies, from the goroutine copying it, so
// waiting here paces the copy.
type pacer struct{ b *bucket }

func (p pacer) RelayStatus(bool)                      {}
func (p pacer) ConnectionOpened()                     {}
func (p pacer) ConnectionClosed(time.Duration)        {}
func (p pacer) ConnectionRequestHandled(pbv2.Status)  {}
func (p pacer) ReservationAllowed(bool)               {}
func (p pacer) ReservationClosed(int)                 {}
func (p pacer) ReservationRequestHandled(pbv2.Status) {}
func (p pacer) BytesTransferred(n int)                { p.b.wait(n) }
