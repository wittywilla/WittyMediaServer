// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/i18n"
	"github.com/wittywilla/wittymediaserver/internal/p2p"
	"github.com/wittywilla/wittymediaserver/pkg/connectsteps"
)

// Check connection (Settings → Remote access → Peer-to-peer connections): can
// a device away from home reach this server, one thing at a time, each with
// what to do when it can't. Read from what the node already knows, so it is
// instant and asks nothing of anyone outside; "Test my connection" is the one
// that dials in. And each device's tries, so the owner can see a device tried
// and why it was refused. All of it stays on this server.

// reachCheck is one line of Check connection.
type reachCheck struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	State  string `json:"state"` // ok | warn | fail | off
	Detail string `json:"detail"`
	Next   string `json:"next,omitempty"`
}

// checkP2P is Check connection's lines: what a device from away needs, in the
// order it needs them.
func checkP2P(cfg p2pSettings, st p2p.Status, running bool, devices int) []reachCheck {
	out := []reachCheck{}
	add := func(id, title, state, detail, next string) {
		out = append(out, reachCheck{ID: id, Title: title, State: state, Detail: detail, Next: next})
	}
	if !cfg.Enabled {
		add("on", i18n.T("Peer-to-peer connections"), "fail", i18n.T("They're off, so no device can reach this server from away this way."),
			i18n.T("Turn on peer-to-peer connections on this page."))
		return out
	}
	if !running {
		add("on", i18n.T("Peer-to-peer connections"), "fail", i18n.T("They're on, but haven't started."),
			i18n.T("Turn them off and on again. If they still don't start, look in the server's log under peer to peer."))
		return out
	}
	add("on", i18n.T("Peer-to-peer connections"), "ok", i18n.F("On, on port {port} (UDP and TCP).", "port", st.Port), "")

	// Only paired devices.
	if cfg.DevicesOnly {
		add("devicesOnly", i18n.T("Only paired devices"), "ok", i18n.T("Only devices on this server's list can connect. A new device joins with an invite or a code."), "")
	} else {
		add("devicesOnly", i18n.T("Only paired devices"), "warn", i18n.T("Any device with this server's connection ID can reach its sign-in page."),
			i18n.T("Turn on “Only paired devices” below: it is all the apps need."))
	}
	if devices == 0 {
		add("devices", i18n.T("Paired devices"), "warn", i18n.T("No device is paired yet."), i18n.T("Use Connect a device to send an invite or type a device's code."))
	} else {
		add("devices", i18n.T("Paired devices"), "ok", i18n.F("{count} paired.", "count", devices), "")
	}

	// Being found: the address book.
	switch {
	case !st.Layers.DHT:
		add("dht", i18n.T("Address book"), "off", i18n.T("Off. Devices find this server only at addresses they remember, and a TV can't join by a code."),
			i18n.T("Turn on the public address book below, or connect devices with an invite link."))
	case st.DHTPeers > 0:
		add("dht", i18n.T("Address book"), "ok", i18n.F("This server is listed, so devices can find it by its ID ({count} computers in touch).", "count", st.DHTPeers), "")
	case time.Since(time.Unix(st.Started, 0)) < 3*time.Minute:
		add("dht", i18n.T("Address book"), "warn", i18n.T("Still joining. This takes a minute or two after starting."), i18n.T("Check again in two minutes."))
	default:
		add("dht", i18n.T("Address book"), "fail", i18n.T("No computer in the address book answered, so devices away can't find this server by its ID."),
			i18n.T("Check that this computer's firewall lets WittyMediaServer connect out over UDP and TCP."))
	}

	// Relay reservation: a place held on a relay, for devices that can't connect straight in.
	ownOK, ownBad := 0, ""
	for _, r := range st.OwnRelays {
		if r.State == "ok" {
			ownOK++
		} else if ownBad == "" && r.Error != "" {
			ownBad = r.Error
		}
	}
	held := len(st.Relays)
	switch {
	case !st.Layers.Relay:
		add("relay", i18n.T("Relay reservation"), "off", i18n.T("Relays are off, so a device can only connect if this server can be reached straight from the internet."),
			i18n.T("Turn relays on below unless a public address shows as reachable."))
	case ownBad != "":
		f := connectsteps.Classify("", ownBad, connectsteps.Found)
		add("relay", i18n.T("Relay reservation"), "fail", i18n.F("Your WittyRelay didn't give this server a place: {error}", "error", ownBad), f.Next)
	case ownOK > 0:
		add("relay", i18n.T("Relay reservation"), "ok", i18n.F("Your WittyRelay holds a place for this server ({count} relays in all).", "count", max(held, ownOK)), "")
	case held > 0:
		add("relay", i18n.T("Relay reservation"), "ok", i18n.F("{count} relays hold a place for this server. Public relays only introduce a device; a relay of your own carries the connection when a direct one can't be made.", "count", held), "")
	case time.Since(time.Unix(st.Started, 0)) < 3*time.Minute:
		add("relay", i18n.T("Relay reservation"), "warn", i18n.T("Not held yet. This takes a minute or two after starting."), i18n.T("Check again in two minutes."))
	default:
		add("relay", i18n.T("Relay reservation"), "fail", i18n.T("No relay holds a place for this server, so a device can't reach it unless it can be reached straight from the internet."),
			i18n.T("Add a relay of your own below (your own WittyRelay), or check this computer's firewall."))
	}

	// Public addresses: proved reachable from outside, or not.
	public := publicAddrs(st.Addrs)
	switch {
	case len(st.Reachable) > 0:
		add("public", i18n.T("Public addresses"), "ok", i18n.F("Computers outside reached this server at {addrs}.", "addrs", addrKinds(st.Reachable)), "")
	case st.Reachability == "private" || len(st.Unreachable) > 0:
		add("public", i18n.T("Public addresses"), "warn", i18n.T("Computers outside tried this server's addresses and couldn't get in, so devices connect through a relay or by hole punching."),
			i18n.T("For direct connections, turn on UPnP or NAT-PMP in your router, or forward this port to this computer."))
	case len(public) > 0:
		add("public", i18n.T("Public addresses"), "warn", i18n.F("This server has {addrs}, not yet proved reachable from outside.", "addrs", addrKinds(public)),
			i18n.T("Check again in a few minutes."))
	default:
		add("public", i18n.T("Public addresses"), "warn", i18n.T("No public address yet: this server is behind a router, so devices connect through a relay or by hole punching."), "")
	}

	// UPnP / NAT-PMP.
	switch m := st.Mapping; {
	case !st.Layers.PortMap:
		add("upnp", i18n.T("Router port forwarding (UPnP)"), "off", i18n.T("Off: this server doesn't ask the router to forward its port."), "")
	case m.On && m.Shared:
		add("upnp", i18n.T("Router port forwarding (UPnP)"), "warn", i18n.T("The router forwarded the port, but its own outside address is shared (a second router, or the internet provider's), so it can't be reached from the internet."),
			i18n.T("Use a relay of your own, or ask your internet provider for a public address."))
	case m.On:
		proto := m.Protocol
		if proto == "" {
			proto = "UPnP"
		}
		add("upnp", i18n.T("Router port forwarding (UPnP)"), "ok", i18n.F("The router forwards the port ({protocol}).", "protocol", proto), "")
	case m.Error != "":
		add("upnp", i18n.T("Router port forwarding (UPnP)"), "warn", i18n.F("The router didn't forward the port: {error}", "error", m.Error),
			i18n.T("Look for UPnP or NAT-PMP in your router's settings and turn it on. It isn't needed if a relay holds a place."))
	default:
		add("upnp", i18n.T("Router port forwarding (UPnP)"), "warn", i18n.T("No router answered. It isn't needed if a relay holds a place."), "")
	}

	// IPv6.
	six := 0
	for _, a := range public {
		if strings.Contains(a, "/ip6/") {
			six++
		}
	}
	switch {
	case !st.Layers.IPv6:
		add("ipv6", i18n.T("IPv6"), "off", i18n.T("Off: this server doesn't ask the router to let IPv6 connections in."), "")
	case six > 0 && len(st.Mapping.Pinholes) > 0:
		add("ipv6", i18n.T("IPv6"), "ok", i18n.T("This server has a public IPv6 address, and the router lets connections in to it."), "")
	case six > 0:
		add("ipv6", i18n.T("IPv6"), "ok", i18n.T("This server has a public IPv6 address. Devices on IPv6 networks may connect straight to it if the router lets them in."), "")
	default:
		add("ipv6", i18n.T("IPv6"), "warn", i18n.T("No public IPv6 address. That's common, and nothing needs doing."), "")
	}
	return out
}

// publicAddrs are the multiaddrs whose IP is a public one.
func publicAddrs(addrs []string) []string {
	out := []string{}
	for _, a := range addrs {
		parts := strings.Split(a, "/")
		if len(parts) < 3 || (parts[1] != "ip4" && parts[1] != "ip6") || strings.Contains(a, "/p2p-circuit") {
			continue
		}
		ip, err := netip.ParseAddr(parts[2])
		if err != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
			continue
		}
		if ip.Is4() && ip.As4()[0] == 100 && ip.As4()[1]&0xc0 == 64 { // 100.64.0.0/10, shared by providers
			continue
		}
		out = append(out, a)
	}
	return out
}

// addrKinds says which kinds of address a list holds, and on which port, not
// the addresses themselves: a page about this is shown and shared, and a home's
// address is nobody else's business. "a public IPv4 address (port 4001)".
func addrKinds(addrs []string) string {
	four, six, port := false, false, ""
	for _, a := range addrs {
		p := strings.Split(a, "/")
		if len(p) < 5 {
			continue
		}
		four = four || p[1] == "ip4"
		six = six || p[1] == "ip6"
		if port == "" {
			port = p[4]
		}
	}
	kind := i18n.T("a public address")
	switch {
	case four && six:
		kind = i18n.T("public IPv4 and IPv6 addresses")
	case four:
		kind = i18n.T("a public IPv4 address")
	case six:
		kind = i18n.T("a public IPv6 address")
	}
	if port == "" {
		return kind
	}
	return i18n.F("{kind} (port {port})", "kind", kind, "port", port)
}

// POST /api/p2p/check: Check connection.
func (s *Server) handleP2PCheck(w http.ResponseWriter, r *http.Request) {
	cfg := s.loadP2P(r.Context())
	node := s.p2pNode()
	var st p2p.Status
	if node != nil {
		st = node.Status()
	}
	checks := checkP2P(cfg, st, node != nil, len(s.p2p.devices.list()))
	failed := 0
	for _, c := range checks {
		if c.State == "fail" {
			failed++
		}
	}
	s.Log.InfoContext(r.Context(), "peer-to-peer connection checked", "cat", "p2p", "by", userFrom(r).Name, "failed", failed)
	res := map[string]any{"checks": checks, "at": time.Now().Unix()}
	if node != nil {
		res["verdict"] = verdictP2P(st)
	}
	writeJSON(w, 200, res)
}

// p2pAttempt is an attempt with the name the device has on the list.
type p2pAttempt struct {
	p2p.Attempt
	Short  string `json:"short"`  // the device's ID, short
	Listed bool   `json:"listed"` // on the list of devices that may connect
	Title  string `json:"title"`  // the step that failed, in words
}

// GET /api/p2p/attempts[?device=ID]: devices' tries at connecting from away,
// newest first, and the steps they go through (connectsteps).
func (s *Server) handleP2PAttempts(w http.ResponseWriter, r *http.Request) {
	node := s.p2pNode()
	steps := connectsteps.Steps()
	if node == nil {
		writeJSON(w, 200, map[string]any{"attempts": []p2pAttempt{}, "steps": steps})
		return
	}
	names := map[string]string{}
	for _, d := range s.p2p.devices.list() {
		names[d.ID] = d.Name
	}
	out := []p2pAttempt{}
	for _, a := range node.Attempts(r.URL.Query().Get("device")) {
		name, listed := names[a.Device]
		if listed {
			a.Name = name
		}
		pa := p2pAttempt{Attempt: a, Short: p2p.ShortID(a.Device), Listed: listed}
		for _, st := range steps {
			if st.ID == a.Step {
				pa.Title = st.Failed
			}
		}
		out = append(out, pa)
	}
	writeJSON(w, 200, map[string]any{"attempts": out, "steps": steps})
}
