// Package portmap asks the home router to forward a port to this machine, with
// NAT-PMP (RFC 6886) or UPnP IGD. It is off until the owner turns it on, talks
// only to the router on the local network, and removes what it set up when it
// is turned off or the server stops.
package portmap

import (
	"context"
	"errors"
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

const (
	leaseSeconds = 3600
	description  = "WittyMediaServer"
)

// router is one way of talking to a router.
type router interface {
	Protocol() string
	ExternalIP(ctx context.Context) (netip.Addr, error)
	// Map forwards external → internal for a while, and returns the outside
	// port the router really gave and how long it lasts (0 = until removed).
	Map(ctx context.Context, external, internal int) (int, time.Duration, error)
	Unmap(ctx context.Context, external, internal int) error
}

// Probe is what a look at the router found. It changes nothing on the router.
type Probe struct {
	Found      bool   `json:"found"`
	Protocol   string `json:"protocol,omitempty"`
	ExternalIP string `json:"externalIp,omitempty"`
	// Shared: the router's own outside address is not on the public internet,
	// because the internet provider shares addresses between customers or
	// there is a second router in front. Port forwarding then can't work.
	Shared bool   `json:"shared,omitempty"`
	Error  string `json:"error,omitempty"`
	At     int64  `json:"at"`
}

// Status is the state of the keeper, for Settings.
type Status struct {
	On           bool   `json:"on"`
	Working      bool   `json:"working"`
	Protocol     string `json:"protocol,omitempty"`
	ExternalIP   string `json:"externalIp,omitempty"`
	ExternalPort int    `json:"externalPort,omitempty"`
	InternalPort int    `json:"internalPort,omitempty"`
	Shared       bool   `json:"shared,omitempty"`
	Error        string `json:"error,omitempty"`
	Renewed      int64  `json:"renewed,omitempty"`
}

// sharedAddr: an address that can't be reached from the internet.
func sharedAddr(a netip.Addr) bool {
	a = a.Unmap()
	return !a.IsValid() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsUnspecified() || netip.MustParsePrefix("100.64.0.0/10").Contains(a)
}

// discover finds the router, NAT-PMP first (a single quick question).
var discover = func(ctx context.Context) (router, error) {
	gateways := gatewayGuesses()
	for _, gw := range gateways {
		c := &natpmpClient{addr: net.JoinHostPort(gw.String(), "5351"), timeout: 300 * time.Millisecond}
		if _, err := c.ExternalIP(ctx); err == nil {
			return c, nil
		}
	}
	return discoverUPnP(ctx)
}

// Look finds out what the router supports, for the "check my router" button.
func Look(ctx context.Context) *Probe {
	p := &Probe{At: time.Now().Unix()}
	r, err := discover(ctx)
	if err != nil {
		p.Error = err.Error()
		return p
	}
	p.Found, p.Protocol = true, r.Protocol()
	ip, err := r.ExternalIP(ctx)
	if err != nil {
		p.Error = err.Error()
		return p
	}
	p.ExternalIP, p.Shared = ip.String(), sharedAddr(ip)
	return p
}

// Keeper holds one port mapping open while it is wanted: it renews the lease,
// retries when the router has gone away, and removes the mapping when stopped.
type Keeper struct {
	Log *slog.Logger

	mu     sync.Mutex
	st     Status
	cancel context.CancelFunc
	done   chan struct{}
	want   [2]int
}

// Set starts or moves the mapping (internal <= 0 stops it).
func (k *Keeper) Set(external, internal int) {
	k.mu.Lock()
	if internal > 0 && k.want == [2]int{external, internal} && k.cancel != nil {
		k.mu.Unlock()
		return
	}
	cancel, done := k.cancel, k.done
	k.cancel, k.done, k.want = nil, nil, [2]int{}
	if internal <= 0 {
		k.st = Status{}
	}
	k.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done // the old mapping is gone before a new one is asked for
	}
	if internal <= 0 {
		return
	}
	ctx, stop := context.WithCancel(context.Background())
	d := make(chan struct{})
	k.mu.Lock()
	k.cancel, k.done, k.want = stop, d, [2]int{external, internal}
	k.st = Status{On: true, InternalPort: internal}
	k.mu.Unlock()
	logging.Go("port mapping", func() {
		defer close(d)
		k.run(ctx, external, internal)
	}, "cat", "remote")
}

// Stop removes the mapping and waits until that has been tried.
func (k *Keeper) Stop() { k.Set(0, 0) }

// Status reports the mapping.
func (k *Keeper) Status() Status {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.st
}

func (k *Keeper) set(f func(*Status)) {
	k.mu.Lock()
	f(&k.st)
	k.mu.Unlock()
}

// retryWait is how long to wait after a failure: routers rarely change their
// mind quickly, and this must not keep a low-end machine or the network busy.
const retryWait = 5 * time.Minute

func (k *Keeper) run(ctx context.Context, external, internal int) {
	var r router
	got := external
	wait := func(d time.Duration) bool {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
			return true
		case <-ctx.Done():
			return false
		}
	}
	defer func() {
		if r != nil {
			c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := r.Unmap(c, got, internal); err != nil {
				k.Log.Info("port mapping could not be removed; it ends by itself within the hour", "err", err)
			} else {
				k.Log.Info("port mapping removed", "external", got)
			}
		}
	}()
	for ctx.Err() == nil {
		if r == nil {
			dctx, cancel := context.WithTimeout(ctx, 6*time.Second)
			found, err := discover(dctx)
			cancel()
			if err != nil {
				if ctx.Err() == nil {
					k.Log.Warn("no router found for port mapping", "err", err)
				}
				k.set(func(s *Status) { s.Working, s.Error = false, err.Error() })
				if !wait(retryWait) {
					return
				}
				continue
			}
			r = found
		}
		mctx, cancel := context.WithTimeout(ctx, 6*time.Second)
		port, lease, err := r.Map(mctx, external, internal)
		var ip netip.Addr
		if err == nil {
			ip, _ = r.ExternalIP(mctx)
		}
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			k.Log.Warn("port mapping failed", "err", err)
			k.set(func(s *Status) { s.Working, s.Error, s.Protocol = false, err.Error(), r.Protocol() })
			r = nil // look for the router again next time
			if !wait(retryWait) {
				return
			}
			continue
		}
		got = port
		k.set(func(s *Status) {
			s.Working, s.Error, s.Protocol, s.ExternalPort, s.Renewed = true, "", r.Protocol(), port, time.Now().Unix()
			s.ExternalIP, s.Shared = "", false
			if ip.IsValid() {
				s.ExternalIP, s.Shared = ip.String(), sharedAddr(ip)
			}
		})
		k.Log.Info("port mapping", "protocol", r.Protocol(), "external", port, "internal", internal)
		renew := lease / 2
		if renew <= 0 {
			renew = 20 * time.Minute // a permanent mapping: just check it now and then
		}
		if !wait(renew) {
			return
		}
	}
}

// gatewayGuesses lists where the router probably is: the address the system
// says on Linux, and the first and last address of each private network this
// machine is on. Guessing wrong only means one unanswered question.
func gatewayGuesses() []netip.Addr {
	var out []netip.Addr
	seen := map[netip.Addr]bool{}
	add := func(a netip.Addr) {
		if a.IsValid() && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	if b, err := os.ReadFile("/proc/net/route"); err == nil {
		for _, a := range parseRouteTable(string(b)) {
			add(a)
		}
	}
	for _, ip := range privateIPv4s() {
		b := ip.As4()
		add(netip.AddrFrom4([4]byte{b[0], b[1], b[2], 1}))
		add(netip.AddrFrom4([4]byte{b[0], b[1], b[2], 254}))
	}
	return out
}

// parseRouteTable reads the default routes of Linux's /proc/net/route, whose
// addresses are hex in little-endian order.
func parseRouteTable(text string) []netip.Addr {
	var out []netip.Addr
	for i, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if i == 0 || len(f) < 3 || f[1] != "00000000" || len(f[2]) != 8 {
			continue
		}
		var b [4]byte
		if _, err := fmt.Sscanf(f[2], "%02x%02x%02x%02x", &b[3], &b[2], &b[1], &b[0]); err != nil {
			continue
		}
		if a := netip.AddrFrom4(b); a.IsPrivate() {
			out = append(out, a)
		}
	}
	return out
}

// privateIPv4s lists this machine's private addresses (the networks a home
// router can be on; VPN cards, with their 100.64.0.0/10 addresses, are not).
func privateIPv4s() []netip.Addr {
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
		if !ok {
			continue
		}
		if ip = ip.Unmap(); ip.Is4() && ip.IsPrivate() {
			out = append(out, ip)
		}
	}
	return out
}

var errNoRouter = errors.New("no router answered. It may not offer automatic port forwarding, or has it turned off; you can set up the forward by hand")
