// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package portmap

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// A Holder keeps several things open on the home router at once: the same
// port for UDP and TCP (peer-to-peer connections use both), and, where this
// machine has IPv6, an opening in the router's IPv6 firewall for that port.
// Like the Keeper it renews, retries and removes what it made. It speaks PCP
// as well, and is built around one step at a time so tests can drive it with
// a clock of their own.

// Clock is the time, as the Holder sees it.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Want is one port to keep open.
type Want struct {
	Proto string // TCP or UDP
	Port  int    // on this machine; the same port is asked for outside
}

// protoRouter is a router that can forward UDP as well as TCP.
type protoRouter interface {
	router
	MapProto(ctx context.Context, proto string, external, internal int) (int, time.Duration, error)
	UnmapProto(ctx context.Context, proto string, external, internal int) error
}

// pinholer opens a router's IPv6 firewall for one address and port.
type pinholer interface {
	Open(ctx context.Context, proto string, addr netip.Addr, port int) (id string, lease time.Duration, err error)
	Shut(ctx context.Context, proto string, addr netip.Addr, port int, id string) error
}

// MappingStatus is one forwarded port.
type MappingStatus struct {
	Proto        string `json:"proto"`
	InternalPort int    `json:"internalPort"`
	ExternalPort int    `json:"externalPort,omitempty"`
	Working      bool   `json:"working"`
	Error        string `json:"error,omitempty"`
}

// PinholeStatus is one opening in the IPv6 firewall.
type PinholeStatus struct {
	Proto   string `json:"proto"`
	Addr    string `json:"addr"`
	Port    int    `json:"port"`
	Working bool   `json:"working"`
	Error   string `json:"error,omitempty"`
}

// HolderStatus is everything a Holder holds, for Settings.
type HolderStatus struct {
	On         bool   `json:"on"`
	Protocol   string `json:"protocol,omitempty"`
	ExternalIP string `json:"externalIp,omitempty"`
	// Shared: the router's own outside address is not a public one (a second
	// router, or an internet provider that shares addresses), so nothing
	// forwarded here can be reached from the internet.
	Shared   bool            `json:"shared,omitempty"`
	Error    string          `json:"error,omitempty"`
	Mappings []MappingStatus `json:"mappings,omitempty"`
	Pinholes []PinholeStatus `json:"pinholes,omitempty"`
	Renewed  int64           `json:"renewed,omitempty"`
}

// Usable says the IPv4 mappings lead somewhere: every port is forwarded and
// the router's outside address is a public one.
func (s HolderStatus) Usable() bool {
	if !s.On || s.Shared || s.ExternalIP == "" || len(s.Mappings) == 0 {
		return false
	}
	for _, m := range s.Mappings {
		if !m.Working {
			return false
		}
	}
	return true
}

// Holder keeps Wants open. The zero value is usable after setting Log.
type Holder struct {
	Log *slog.Logger
	// IPv6 also asks the router to open its IPv6 firewall for the same ports.
	IPv6 bool
	// OnChange is called (from the Holder's goroutine) when the status changed.
	OnChange func()

	clock Clock                                     // nil: the real one
	find  func(ctx context.Context) (router, error) // nil: discoverAny
	addr6 func() []netip.Addr                       // nil: globalIPv6s
	find6 func(ctx context.Context) pinholer        // nil: discoverPinholer

	mu     sync.Mutex
	st     HolderStatus
	cancel context.CancelFunc
	done   chan struct{}

	// Only the Holder's goroutine (or a test calling step) touches these.
	r        router
	p6       pinholer
	held     map[Want]int // want → outside port given
	holes    map[hole]string
	failures int
}

type hole struct {
	proto string
	addr  netip.Addr
	port  int
}

func (h *Holder) now() Clock {
	if h.clock != nil {
		return h.clock
	}
	return realClock{}
}

// Start begins holding wants. A Holder already running is stopped first.
func (h *Holder) Start(wants []Want) {
	h.Stop()
	if len(wants) == 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	h.mu.Lock()
	h.cancel, h.done = cancel, done
	h.st = HolderStatus{On: true}
	h.mu.Unlock()
	logging.Go("port mapping", func() {
		defer close(done)
		h.run(ctx, wants)
	}, "cat", "remote")
}

// Stop removes everything and waits until that has been tried.
func (h *Holder) Stop() {
	h.mu.Lock()
	cancel, done := h.cancel, h.done
	h.cancel, h.done = nil, nil
	h.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	h.mu.Lock()
	h.st = HolderStatus{}
	h.mu.Unlock()
}

// Status reports what is held.
func (h *Holder) Status() HolderStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.st
	st.Mappings = append([]MappingStatus(nil), st.Mappings...)
	st.Pinholes = append([]PinholeStatus(nil), st.Pinholes...)
	return st
}

func (h *Holder) setStatus(st HolderStatus) {
	h.mu.Lock()
	changed := fmt.Sprint(h.st) != fmt.Sprint(st)
	h.st = st
	h.mu.Unlock()
	if changed && h.OnChange != nil {
		h.OnChange()
	}
}

func (h *Holder) run(ctx context.Context, wants []Want) {
	defer h.release()
	for ctx.Err() == nil {
		wait := h.step(ctx, wants)
		select {
		case <-h.now().After(wait):
		case <-ctx.Done():
			return
		}
	}
}

// Waiting after a failure doubles from half a minute to ten: routers rarely
// change their mind quickly, and this must not keep a small machine busy.
const (
	holderFirstRetry = 30 * time.Second
	holderMaxRetry   = 10 * time.Minute
	holderCheck      = 20 * time.Minute // for mappings that never run out
)

func retryAfter(failures int) time.Duration {
	d := holderFirstRetry
	for i := 1; i < failures && d < holderMaxRetry; i++ {
		d *= 2
	}
	return min(d, holderMaxRetry)
}

// step asks for (or renews) everything once, and says how long to wait before
// the next step.
func (h *Holder) step(ctx context.Context, wants []Want) time.Duration {
	st := HolderStatus{On: true}
	if h.held == nil {
		h.held, h.holes = map[Want]int{}, map[hole]string{}
	}
	next := time.Duration(0)
	sooner := func(lease time.Duration) {
		d := lease / 2
		if d <= 0 {
			d = holderCheck
		}
		if next == 0 || d < next {
			next = d
		}
	}
	failed := false

	// IPv4: forwarded ports.
	if h.r == nil {
		find := h.find
		if find == nil {
			find = discoverAny
		}
		dctx, cancel := context.WithTimeout(ctx, 6*time.Second)
		r, err := find(dctx)
		cancel()
		if err != nil {
			st.Error = err.Error()
			failed = true
		} else {
			h.r = r
		}
	}
	if h.r != nil {
		st.Protocol = h.r.Protocol()
		mctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		for _, w := range wants {
			ms := MappingStatus{Proto: w.Proto, InternalPort: w.Port}
			port, lease, err := mapProto(mctx, h.r, w)
			switch {
			case err != nil:
				ms.Error = err.Error()
				failed = true
			case port != w.Port:
				// Apps are told one port. A different one outside would be a mapping
				// nobody ever tries, so it is given back.
				_ = unmapProto(mctx, h.r, w, port)
				ms.Error = fmt.Sprintf("the router offered port %d instead; another device may be using %d", port, w.Port)
				failed = true
			default:
				ms.Working, ms.ExternalPort = true, port
				h.held[w] = port
				sooner(lease)
			}
			st.Mappings = append(st.Mappings, ms)
		}
		if ip, err := h.r.ExternalIP(mctx); err == nil && ip.IsValid() {
			st.ExternalIP, st.Shared = ip.String(), sharedAddr(ip)
		}
		cancel()
		if failed {
			h.r = nil // look for the router again next time
		}
	}

	// IPv6: openings in the router's firewall.
	if h.IPv6 {
		addrs := h.addr6
		if addrs == nil {
			addrs = globalIPv6s
		}
		list := addrs()
		if len(list) > 0 && h.p6 == nil {
			if p, ok := h.r.(pinholer); ok && canPinhole(h.r) {
				h.p6 = p
			} else {
				find := h.find6
				if find == nil {
					find = discoverPinholer
				}
				pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
				h.p6 = find(pctx)
				cancel()
			}
		}
		want := map[hole]bool{}
		for _, a := range list {
			for _, w := range wants {
				k := hole{w.Proto, a, w.Port}
				want[k] = true
				ps := PinholeStatus{Proto: w.Proto, Addr: a.String(), Port: w.Port}
				if h.p6 == nil {
					ps.Error = "the router offers no way to open its IPv6 firewall"
				} else {
					pctx, cancel := context.WithTimeout(ctx, 4*time.Second)
					if old := h.holes[k]; old != "" && old != "pcp" {
						_ = h.p6.Shut(pctx, k.proto, k.addr, k.port, old) // UPnP: a fresh one rather than guessing what is left of the old
					}
					id, lease, err := h.p6.Open(pctx, w.Proto, a, w.Port)
					cancel()
					if err != nil {
						ps.Error = err.Error()
						delete(h.holes, k)
					} else {
						ps.Working = true
						h.holes[k] = id
						sooner(lease)
					}
				}
				st.Pinholes = append(st.Pinholes, ps)
			}
		}
		// An address this machine no longer has: close what was opened for it.
		for k, id := range h.holes {
			if !want[k] && h.p6 != nil {
				pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
				_ = h.p6.Shut(pctx, k.proto, k.addr, k.port, id)
				cancel()
				delete(h.holes, k)
			}
		}
	}

	if failed {
		h.failures++
		if d := retryAfter(h.failures); next == 0 || d < next {
			next = d
		}
		if h.Log != nil && h.failures == 1 {
			h.Log.Warn("automatic port mapping did not work; trying again later", "err", st.Error, "mappings", fmt.Sprint(st.Mappings))
		}
	} else {
		if h.Log != nil && (h.failures > 0 || h.Status().Renewed == 0) {
			h.Log.Info("ports mapped on the router", "protocol", st.Protocol, "mappings", fmt.Sprint(st.Mappings), "shared", st.Shared)
		}
		h.failures = 0
		st.Renewed = h.now().Now().Unix()
	}
	if next == 0 {
		next = holderCheck
	}
	h.setStatus(st)
	return next
}

// release removes everything made. It runs once the Holder's context is over,
// so it works to a short deadline of its own.
func (h *Holder) release() {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	for k, id := range h.holes {
		if h.p6 != nil {
			_ = h.p6.Shut(ctx, k.proto, k.addr, k.port, id)
		}
	}
	for w, port := range h.held {
		if h.r == nil {
			break
		}
		if err := unmapProto(ctx, h.r, w, port); err != nil && h.Log != nil {
			h.Log.Info("a port mapping could not be removed; it ends by itself within the hour", "proto", w.Proto, "port", w.Port, "err", err)
		}
	}
	h.held, h.holes, h.r, h.p6 = nil, nil, nil, nil
}

func mapProto(ctx context.Context, r router, w Want) (int, time.Duration, error) {
	if pr, ok := r.(protoRouter); ok {
		return pr.MapProto(ctx, w.Proto, w.Port, w.Port)
	}
	if w.Proto == TCP {
		return r.Map(ctx, w.Port, w.Port)
	}
	return 0, 0, fmt.Errorf("the router can't forward %s", strings.ToUpper(w.Proto))
}

func unmapProto(ctx context.Context, r router, w Want, external int) error {
	if pr, ok := r.(protoRouter); ok {
		return pr.UnmapProto(ctx, w.Proto, external, w.Port)
	}
	if w.Proto == TCP {
		return r.Unmap(ctx, external, w.Port)
	}
	return nil
}

// canPinhole: a router of this kind that really has the firewall service.
func canPinhole(r router) bool {
	if u, ok := r.(*upnpClient); ok {
		return u.fw6 != ""
	}
	_, ok := r.(pinholer)
	return ok
}

// discoverAny finds the router: PCP first, NAT-PMP where the router says it
// only knows that, then UPnP.
var discoverAny = func(ctx context.Context) (router, error) {
	for _, gw := range gatewayGuesses() {
		addr := net.JoinHostPort(gw.String(), "5351")
		p := &pcpClient{addr: addr, timeout: 300 * time.Millisecond}
		_, err := p.ExternalIP(ctx)
		if err == nil {
			return p, nil
		}
		n := &natpmpClient{addr: addr, timeout: 300 * time.Millisecond}
		if _, err := n.ExternalIP(ctx); err == nil {
			return n, nil
		}
	}
	return discoverUPnP(ctx)
}

// discoverPinholer finds a way to open the IPv6 firewall when the IPv4 router
// offers none: PCP to the IPv6 router, where the system says which that is.
func discoverPinholer(ctx context.Context) pinholer {
	b, err := os.ReadFile("/proc/net/ipv6_route")
	if err != nil {
		return nil
	}
	for _, gw := range parseRoute6(string(b)) {
		p := &pcpClient{addr: net.JoinHostPort(gw, "5351"), timeout: 300 * time.Millisecond}
		if _, err := p.mapOnce(ctx, UDP, 9, 0, 0); err == nil {
			return p
		}
	}
	return nil
}

// parseRoute6 reads the default routes of Linux's /proc/net/ipv6_route and
// returns their gateways as "address%interface".
func parseRoute6(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		// destination, its prefix length, source, its length, next hop, ..., interface
		if len(f) < 10 || f[0] != strings.Repeat("0", 32) || f[1] != "00" || len(f[4]) != 32 || f[4] == strings.Repeat("0", 32) {
			continue
		}
		b, err := hex.DecodeString(f[4])
		if err != nil {
			continue
		}
		raw := [16]byte(b)
		a := netip.AddrFrom16(raw)
		if !a.IsLinkLocalUnicast() && !a.IsGlobalUnicast() {
			continue
		}
		s := a.String()
		if a.IsLinkLocalUnicast() {
			s += "%" + f[len(f)-1]
		}
		out = append(out, s)
	}
	return out
}

// globalIPv6s lists this machine's IPv6 addresses the internet can route to
// (at most four: a machine with privacy addresses has many, and each takes a
// place in the router's table).
func globalIPv6s() []netip.Addr {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []netip.Addr
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip, ok := netip.AddrFromSlice(ipn.IP)
		if !ok || ip.Is4() || ip.Is4In6() {
			continue
		}
		if ip.IsGlobalUnicast() && !ip.IsPrivate() && len(out) < 4 {
			out = append(out, ip)
		}
	}
	return out
}
