package wittysunshine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"syscall"
)

// Reach is what kind of place an address is.
type Reach int

const (
	Unusable Reach = iota // nothing a computer could be: multicast, link-local, unspecified
	Loopback              // this machine
	Private               // the home network, or a private overlay network
	Public                // the internet
)

// cgnat is 100.64.0.0/10, which private overlay networks hand out.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// Classify says what kind of place an address is. Link-local addresses count as
// unusable: 169.254.169.254 is where cloud machines keep their credentials.
func Classify(ip netip.Addr) Reach {
	ip = ip.Unmap()
	switch {
	case !ip.IsValid(), ip.IsUnspecified(), ip.IsMulticast(), ip.IsLinkLocalUnicast(), ip == netip.AddrFrom4([4]byte{255, 255, 255, 255}):
		return Unusable
	case ip.IsLoopback():
		return Loopback
	case ip.IsPrivate(), cgnat.Contains(ip):
		return Private
	}
	return Public
}

// Policy decides which addresses connections may go to. It is checked when the
// connection is made, not only when an address is typed, so a name that later
// resolves somewhere else is caught.
type Policy struct {
	AllowPublic bool // the owner confirmed that this rig is on the internet
}

// Allow reports whether a connection may go to ip.
func (p Policy) Allow(ip netip.Addr) error {
	switch Classify(ip) {
	case Loopback, Private:
		return nil
	case Public:
		if p.AllowPublic {
			return nil
		}
		return fmt.Errorf("%w: %s is on the internet, not on your network", ErrBlocked, ip)
	}
	return fmt.Errorf("%w: %s can't be a computer on your network", ErrBlocked, ip)
}

func (p Policy) control(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	return p.Allow(ip)
}

// ErrBadAddress: what was typed isn't a computer's name or address.
var ErrBadAddress = errors.New("Type just the PC's name or IP address, like 192.168.1.20 or gaming-pc")

// CleanAddress checks an address someone typed: a name or an IP and nothing else, with
// a port if one follows. There is no scheme, path, user name or anything else that could
// point a request somewhere unexpected.
func CleanAddress(s string) (host string, port int, err error) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 253 {
		return "", 0, ErrBadAddress
	}
	host = s
	if h, p, e := net.SplitHostPort(s); e == nil {
		host = h
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' || n > 65535 {
				return "", 0, ErrBadAddress
			}
			n = n*10 + int(c-'0')
		}
		if n < 1 || n > 65535 {
			return "", 0, ErrBadAddress
		}
		port = n
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if ip, e := netip.ParseAddr(host); e == nil {
		if ip.Zone() != "" {
			return "", 0, ErrBadAddress
		}
		return ip.String(), port, nil
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", 0, ErrBadAddress
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return "", 0, ErrBadAddress
			}
		}
	}
	// An all-numeric last label is a mistyped IP, and a browser would read it as one.
	last := host[strings.LastIndexByte(host, '.')+1:]
	if strings.Trim(last, "0123456789") == "" {
		return "", 0, ErrBadAddress
	}
	return host, port, nil
}

// CheckAddress resolves a cleaned address and says what kind of place it is. When a
// name has several addresses, the least safe one decides, as a connection may use any
// of them.
func CheckAddress(ctx context.Context, host string, lookup func(context.Context, string) ([]netip.Addr, error)) (Reach, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		r := Classify(ip)
		if r == Unusable {
			return r, fmt.Errorf("%s can't be a computer on your network", host)
		}
		return r, nil
	}
	if lookup == nil {
		lookup = func(ctx context.Context, h string) ([]netip.Addr, error) { return net.DefaultResolver.LookupNetIP(ctx, "ip", h) }
	}
	ips, err := lookup(ctx, host)
	if err != nil || len(ips) == 0 {
		return Unusable, fmt.Errorf("Couldn't find a computer called %s on your network. Try its IP address", host)
	}
	worst := Loopback
	for _, ip := range ips {
		switch Classify(ip) {
		case Unusable:
			return Unusable, fmt.Errorf("%s points at %s, which can't be a computer on your network", host, ip)
		case Public:
			worst = Public
		case Private:
			if worst == Loopback {
				worst = Private
			}
		}
	}
	return worst, nil
}
