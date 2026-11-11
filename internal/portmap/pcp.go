// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package portmap

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"
)

// TCP and UDP name what a mapping carries.
const (
	TCP = "tcp"
	UDP = "udp"
)

// errNotPCP: the gateway answered, but as NAT-PMP only. The caller then asks
// it in NAT-PMP instead.
var errNotPCP = errors.New("the router speaks NAT-PMP, not PCP")

// pcpClient speaks the Port Control Protocol (RFC 6887) to one gateway. PCP
// follows NAT-PMP on the same port, and adds two things this server uses:
// mappings of UDP as well as TCP in one design, and IPv6, where a "mapping"
// opens the router's firewall for one address and port and translates nothing.
type pcpClient struct {
	addr    string // host:port of the gateway
	timeout time.Duration

	mu     sync.Mutex
	nonces map[string][12]byte // a mapping is renewed and removed with the nonce that made it
}

func (c *pcpClient) Protocol() string { return "PCP" }

func protoNumber(proto string) byte {
	if proto == UDP {
		return 17
	}
	return 6
}

// nonce returns the mapping's nonce, making one the first time.
func (c *pcpClient) nonce(key string) [12]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nonces == nil {
		c.nonces = map[string][12]byte{}
	}
	n, ok := c.nonces[key]
	if !ok {
		_, _ = rand.Read(n[:])
		c.nonces[key] = n
	}
	return n
}

// pcpAnswer is what a MAP answer says.
type pcpAnswer struct {
	lease    time.Duration
	port     int
	external netip.Addr
}

// mapOnce sends one MAP request and reads its answer. lease 0 removes.
func (c *pcpClient) mapOnce(ctx context.Context, proto string, internal, external int, lease uint32) (pcpAnswer, error) {
	return c.mapFrom(ctx, netip.Addr{}, proto, internal, external, lease)
}

// mapFrom is mapOnce leaving from one of this machine's addresses: PCP opens
// the firewall for the address a request came from.
func (c *pcpClient) mapFrom(ctx context.Context, from netip.Addr, proto string, internal, external int, lease uint32) (pcpAnswer, error) {
	var d net.Dialer
	if from.IsValid() {
		d.LocalAddr = &net.UDPAddr{IP: from.AsSlice()}
	}
	conn, err := d.DialContext(ctx, "udp", c.addr) // connected: only this address can answer
	if err != nil {
		return pcpAnswer{}, err
	}
	defer conn.Close()
	local, ok := netip.AddrFromSlice(conn.LocalAddr().(*net.UDPAddr).IP)
	if !ok {
		return pcpAnswer{}, errors.New("no local address")
	}
	nonce := c.nonce(fmt.Sprintf("%s/%d/%s", proto, internal, local.Unmap()))
	req := make([]byte, 60)
	req[0], req[1] = 2, 1 // version 2, MAP
	binary.BigEndian.PutUint32(req[4:], lease)
	ip16 := local.As16()
	copy(req[8:24], ip16[:])
	copy(req[24:36], nonce[:])
	req[36] = protoNumber(proto)
	binary.BigEndian.PutUint16(req[40:], uint16(internal))
	binary.BigEndian.PutUint16(req[42:], uint16(external))
	// The suggested outside address: "any", in the family being asked about.
	if local.Unmap().Is4() {
		req[54], req[55] = 0xff, 0xff
	}
	buf := make([]byte, 1100)
	for try := range 2 {
		_ = conn.SetDeadline(time.Now().Add(c.timeout << try))
		if _, err := conn.Write(req); err != nil {
			return pcpAnswer{}, err
		}
		n, err := conn.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return pcpAnswer{}, ctx.Err()
			}
			continue
		}
		if n >= 4 && buf[0] == 0 {
			return pcpAnswer{}, errNotPCP // a NAT-PMP router saying it doesn't know version 2
		}
		if n < 60 || buf[0] != 2 || buf[1] != 0x81 || [12]byte(buf[24:36]) != nonce {
			continue // not the answer to this question
		}
		if code := buf[3]; code != 0 {
			if code == 1 {
				return pcpAnswer{}, errNotPCP
			}
			return pcpAnswer{}, fmt.Errorf("the router refused (PCP result %d: %s)", code, pcpResult(code))
		}
		ext, _ := netip.AddrFromSlice(buf[44:60])
		return pcpAnswer{
			lease:    time.Duration(binary.BigEndian.Uint32(buf[4:8])) * time.Second,
			port:     int(binary.BigEndian.Uint16(buf[42:44])),
			external: ext.Unmap(),
		}, nil
	}
	return pcpAnswer{}, errors.New("no answer")
}

func pcpResult(code byte) string {
	switch code {
	case 2:
		return "not allowed by the router's settings"
	case 7:
		return "the router can't reach its own network"
	case 8:
		return "the router has no room for more"
	case 11:
		return "that outside port can't be given"
	case 12:
		return "too many mappings for this device"
	}
	return "see RFC 6887"
}

// ExternalIP asks for a short mapping of a throwaway port to learn the outside
// address, as PCP has no question that only asks for it.
func (c *pcpClient) ExternalIP(ctx context.Context) (netip.Addr, error) {
	a, err := c.mapOnce(ctx, UDP, 9, 0, 0) // lease 0: nothing is opened
	if err != nil {
		return netip.Addr{}, err
	}
	return a.external, nil
}

func (c *pcpClient) Map(ctx context.Context, external, internal int) (int, time.Duration, error) {
	return c.MapProto(ctx, TCP, external, internal)
}

func (c *pcpClient) Unmap(ctx context.Context, external, internal int) error {
	return c.UnmapProto(ctx, TCP, external, internal)
}

func (c *pcpClient) MapProto(ctx context.Context, proto string, external, internal int) (int, time.Duration, error) {
	a, err := c.mapOnce(ctx, proto, internal, external, leaseSeconds)
	if err != nil {
		return 0, 0, err
	}
	return a.port, a.lease, nil
}

func (c *pcpClient) UnmapProto(ctx context.Context, proto string, _, internal int) error {
	_, err := c.mapOnce(ctx, proto, internal, 0, 0)
	return err
}

// Open opens the firewall for one of this machine's IPv6 addresses (the
// request leaves from that address, which is how PCP knows whose port it is).
func (c *pcpClient) Open(ctx context.Context, proto string, addr netip.Addr, port int) (string, time.Duration, error) {
	a, err := c.mapFrom(ctx, addr, proto, port, port, leaseSeconds)
	if err != nil {
		return "", 0, err
	}
	if a.port != port {
		// A firewall that "opens" another port has opened nothing anyone will try.
		_, _ = c.mapFrom(ctx, addr, proto, port, 0, 0)
		return "", 0, errors.New("the router offered a different port")
	}
	return "pcp", a.lease, nil
}

func (c *pcpClient) Shut(ctx context.Context, proto string, addr netip.Addr, port int, _ string) error {
	_, err := c.mapFrom(ctx, addr, proto, port, 0, 0)
	return err
}
