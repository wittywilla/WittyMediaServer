// Package remote holds what remote access is built on: deciding who a request
// really comes from (and whether forwarded headers may be believed), HTTPS
// certificates, and the numbers behind the Remote Access settings page.
package remote

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"
)

// Where a client is, from least to most trusted. The zero value is the least
// trusted one, so anything unknown fails closed.
type Where int

const (
	Internet Where = iota // anywhere else
	VPN                   // a private overlay network (Tailscale, NetBird, or a network the owner named)
	Home                  // the home network
	Machine               // this computer
)

func (w Where) String() string {
	switch w {
	case Machine:
		return "machine"
	case Home:
		return "home"
	case VPN:
		return "vpn"
	}
	return "internet"
}

// AtHome: the client is on this computer or the home network.
func (w Where) AtHome() bool { return w >= Home }

// Client is what a request says about its sender, after deciding how much of
// it can be believed.
type Client struct {
	IP    netip.Addr // the client's address as far as it can be known
	Peer  netip.Addr // who opened the connection to this server
	Where Where
	// Proxied: a trusted proxy passed this request on and named the client.
	Proxied bool
	// Unverified: the request carries forwarded headers but did not come from
	// a trusted proxy. They are ignored, and it counts as coming from the
	// internet, which is the safe direction for everything that uses this.
	Unverified bool
	// Secure: the connection from the browser is encrypted. Direct TLS, or a
	// trusted proxy says so.
	Secure bool
	// ForwardedHost is the address the browser used, from a trusted proxy only.
	ForwardedHost string
}

// Direct: nothing stands between this server and the client.
func (c Client) Direct() bool { return !c.Proxied && !c.Unverified }

// Policy is the owner's choices about networks. A nil Policy trusts no proxy
// and knows only the built-in ranges.
type Policy struct {
	Trusted []netip.Prefix // reverse proxies whose forwarded headers are believed
	Home    []netip.Prefix // extra networks that count as the home network
	Away    []netip.Prefix // networks that count as away, even if private (a VPN of your own)
	// LocalV6 lists this machine's own IPv6 home subnets; nil uses HomeSubnets.
	LocalV6 func() []netip.Prefix
}

var (
	cgnat     = netip.MustParsePrefix("100.64.0.0/10")       // Tailscale and NetBird addresses
	tailscale = netip.MustParsePrefix("fd7a:115c:a1e0::/48") // Tailscale's IPv6 range
)

func clean(a netip.Addr) netip.Addr { return a.Unmap().WithZone("") }

func within(list []netip.Prefix, a netip.Addr) bool {
	for _, p := range list {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// Classify says where an address is. The owner's lists win over the built-in
// ranges, and "away" wins over everything: a private VPN address must not
// count as the home network if the owner says it doesn't.
func (p *Policy) Classify(a netip.Addr) Where {
	if !a.IsValid() {
		return Internet
	}
	a = clean(a)
	if p != nil {
		if within(p.Away, a) {
			return VPN
		}
		if within(p.Home, a) {
			return Home
		}
	}
	switch {
	case a.IsLoopback():
		return Machine
	case cgnat.Contains(a), tailscale.Contains(a):
		return VPN
	case a.IsPrivate(), a.IsLinkLocalUnicast():
		return Home
	}
	if a.Is6() {
		local := HomeSubnets
		if p != nil && p.LocalV6 != nil {
			local = p.LocalV6
		}
		if within(local(), a) {
			return Home
		}
	}
	return Internet
}

func (p *Policy) trusts(a netip.Addr) bool {
	return p != nil && a.IsValid() && within(p.Trusted, clean(a))
}

// Resolve works out who sent a request. Forwarded headers are read only when
// the connection itself comes from a trusted proxy, and then from the right
// (the entry that proxy added) until the first address that is not a trusted
// proxy: everything further left was written by the client and may be made up.
//
// RFC 7239's Forwarded header is never read: proxies that only maintain
// X-Forwarded-For pass a client's made-up Forwarded through untouched.
func (p *Policy) Resolve(r *http.Request) Client {
	peer := peerAddr(r.RemoteAddr)
	c := Client{IP: peer, Peer: peer, Secure: r.TLS != nil}
	xff := r.Header.Values("X-Forwarded-For")
	forwarded := len(xff) > 0 || r.Header.Get("Forwarded") != ""
	if !p.trusts(peer) {
		if forwarded {
			c.Unverified = true
			c.Where = Internet
			return c
		}
		c.Where = p.Classify(peer)
		return c
	}
	if len(xff) == 0 {
		if forwarded {
			// A trusted proxy that only speaks Forwarded: which client it means
			// can't be told, so it must not look like the proxy itself (which
			// may well be this machine).
			c.Unverified = true
			c.Where = Internet
			return c
		}
		c.Where = p.Classify(peer)
		return c
	}
	c.Proxied = true
	client := peer
	list := strings.Split(strings.Join(xff, ","), ",")
	for i := len(list) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(list[i]))
		if err != nil {
			break // nothing left of a broken entry can be believed
		}
		a = clean(a)
		client = a
		if !p.trusts(a) {
			break
		}
	}
	c.IP = client
	c.Where = p.Classify(client)
	c.Secure = c.Secure || lastValue(r.Header.Get("X-Forwarded-Proto")) == "https"
	c.ForwardedHost = lastValue(r.Header.Get("X-Forwarded-Host"))
	return c
}

// SecureHint: the connection is encrypted, or a header claims a proxy in front
// of this server encrypts it. The header is only good enough for choices that
// can harm nobody but whoever forged it, such as marking a cookie Secure.
func SecureHint(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(lastValue(r.Header.Get("X-Forwarded-Proto")), "https")
}

func lastValue(h string) string {
	if i := strings.LastIndexByte(h, ','); i >= 0 {
		h = h[i+1:]
	}
	return strings.ToLower(strings.TrimSpace(h))
}

func peerAddr(remote string) netip.Addr {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return clean(a)
}

// ParseNets reads a list of addresses and networks (separated by commas,
// spaces or new lines). A network wider than /8 (IPv4) or /32 (IPv6) is
// refused: trusting that much of the internet is never what anyone means.
func ParseNets(text string) ([]netip.Prefix, error) {
	fields := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\r' || r == '\t' || r == ';' })
	if len(fields) > 64 {
		return nil, errors.New("too many networks: at most 64")
	}
	var out []netip.Prefix
	for _, f := range fields {
		var pre netip.Prefix
		if strings.Contains(f, "/") {
			p, err := netip.ParsePrefix(f)
			if err != nil {
				return nil, fmt.Errorf("%q is not an address or network", f)
			}
			pre = p
		} else {
			a, err := netip.ParseAddr(f)
			if err != nil {
				return nil, fmt.Errorf("%q is not an address or network", f)
			}
			a = clean(a)
			pre = netip.PrefixFrom(a, a.BitLen())
		}
		if a := pre.Addr(); a.Is4In6() {
			pre = netip.PrefixFrom(a.Unmap(), max(pre.Bits()-96, 0))
		}
		if (pre.Addr().Is4() && pre.Bits() < 8) || (pre.Addr().Is6() && pre.Bits() < 32) {
			return nil, fmt.Errorf("%q covers too much of the internet", f)
		}
		out = append(out, pre.Masked())
	}
	return out, nil
}

// LimitKey is the unit a limit applies to: one IPv4 address, or one IPv6 /64
// (a single home or server is usually given a whole /64, so counting single
// IPv6 addresses would limit nobody).
func LimitKey(a netip.Addr) string {
	if !a.IsValid() {
		return "?"
	}
	a = clean(a)
	if a.Is6() {
		if p, err := a.Prefix(64); err == nil {
			return p.String()
		}
	}
	return a.String()
}

// ---------------------------------------------------------------------------
// This machine's networks
// ---------------------------------------------------------------------------

// Iface is one address of one of this machine's network cards.
type Iface struct {
	Name string
	Addr netip.Prefix
}

// Interfaces lists the addresses of the network cards that are up.
func Interfaces() []Iface {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []Iface
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(ipn.IP)
			if !ok {
				continue
			}
			bits, _ := ipn.Mask.Size()
			ip = clean(ip)
			if ip.Is4() && bits > 32 {
				bits -= 96
			}
			out = append(out, Iface{Name: i.Name, Addr: netip.PrefixFrom(ip, bits)})
		}
	}
	return out
}

// VPNAddr is an address of this machine on a private overlay network.
type VPNAddr struct {
	Kind string `json:"kind"` // Tailscale | NetBird | VPN
	IP   string `json:"ip"`
}

// FindVPNs finds overlay networks from what the network cards look like. No
// program is run and nothing is sent. An address in 100.64.0.0/10 on a card
// with no telling name could also be a modem handing out a carrier's address,
// so only cards recognised by name count.
func FindVPNs(list []Iface) []VPNAddr {
	var out []VPNAddr
	for _, i := range list {
		a := i.Addr.Addr()
		name := strings.ToLower(i.Name)
		kind := ""
		switch {
		case strings.Contains(name, "tailscale"):
			kind = "Tailscale"
		case strings.Contains(name, "netbird"), strings.HasPrefix(name, "wt") && len(name) <= 4:
			kind = "NetBird"
		case strings.HasPrefix(name, "utun") && (cgnat.Contains(a) || tailscale.Contains(a)):
			kind = "VPN" // on macOS every overlay is called utunN
		}
		if kind == "" || !(cgnat.Contains(a) || tailscale.Contains(a) || a.IsPrivate()) || !a.Is4() {
			continue
		}
		out = append(out, VPNAddr{Kind: kind, IP: a.String()})
	}
	return out
}

// homeSubnetsOf finds the IPv6 networks that are really the home network: a
// /64 on a card that also has a private IPv4 address. Devices at home with
// global IPv6 addresses then still count as at home, while a rented server
// (public IPv4, possibly sharing its /64 with strangers) is left out.
func homeSubnetsOf(list []Iface) []netip.Prefix {
	private := map[string]bool{}
	for _, i := range list {
		if a := i.Addr.Addr(); a.Is4() && a.IsPrivate() {
			private[i.Name] = true
		}
	}
	var out []netip.Prefix
	for _, i := range list {
		a := i.Addr.Addr()
		if private[i.Name] && a.Is6() && a.IsGlobalUnicast() && !a.IsPrivate() && i.Addr.Bits() == 64 {
			out = append(out, i.Addr.Masked())
		}
	}
	return out
}

type subnetCache struct {
	at   time.Time
	nets []netip.Prefix
}

var homeSubnets atomic.Pointer[subnetCache]

// HomeSubnets returns homeSubnetsOf for this machine, looked up at most once a
// minute (and only ever for clients with global IPv6 addresses).
func HomeSubnets() []netip.Prefix {
	if c := homeSubnets.Load(); c != nil && time.Since(c.at) < time.Minute {
		return c.nets
	}
	c := &subnetCache{at: time.Now(), nets: homeSubnetsOf(Interfaces())}
	homeSubnets.Store(c)
	return c.nets
}
