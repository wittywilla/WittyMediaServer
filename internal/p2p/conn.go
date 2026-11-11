// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"crypto/sha256"
	"errors"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Addr is where a peer-to-peer connection comes from, as the HTTP server sees
// it. Its text form is an IP address and port like any other, so everything
// that counts or limits by address works, but that address is never one that
// could be mistaken for the home network or this computer: see presentedIP.
type Addr struct {
	IP      netip.Addr
	Peer    string // the other end's ID
	Relayed bool
	Known   bool // on the server's list of devices
}

func (a *Addr) Network() string { return "p2p" }
func (a *Addr) String() string  { return net.JoinHostPort(a.IP.String(), "0") }

// strangerIP stands for "someone on the same network whose address must not
// count as local": a documentation address no real machine has.
var strangerIP = netip.MustParseAddr("192.0.2.1")

// presentedIP chooses the address a connection is counted under.
//
// The danger this guards against is the oldest one in tunnels: a connection
// that arrives through one looks as if it came from wherever the tunnel ends,
// which is this computer or the home network, and inherits their privileges.
// So an address is only passed on when it is a public one and the connection
// is direct. Otherwise:
//   - a device on the server's list gets an address made from its key, in a
//     range reserved for documentation, so each device is limited on its own
//     wherever it connects from;
//   - a stranger behind a relay is counted under the relay's address, and one
//     on a private network under a fixed one, so making new keys never makes
//     new allowances.
func presentedIP(remote netip.Addr, relayed, known bool, id peer.ID) netip.Addr {
	if !relayed && publicIP(remote) {
		return remote.Unmap()
	}
	if known {
		sum := sha256.Sum256([]byte(id))
		// 2001:db8:xxxx:xxxx::1 — the first 64 bits differ per device, which is
		// the unit limits count IPv6 by.
		return netip.AddrFrom16([16]byte{0x20, 0x01, 0x0d, 0xb8, sum[0], sum[1], sum[2], sum[3], 0, 0, 0, 0, 0, 0, 0, 1})
	}
	if relayed && publicIP(remote) {
		return remote.Unmap()
	}
	return strangerIP
}

// streamConn makes a libp2p stream look like a network connection.
type streamConn struct {
	network.Stream
	local, remote net.Addr
	bucket        *bucket // nil: no limit
	onClose       func()
	once          sync.Once
}

func (c *streamConn) LocalAddr() net.Addr  { return c.local }
func (c *streamConn) RemoteAddr() net.Addr { return c.remote }

func (c *streamConn) Write(b []byte) (int, error) {
	if c.bucket == nil {
		return c.Stream.Write(b)
	}
	// In pieces, so a slow link is paced evenly and a closed stream is noticed
	// within a piece rather than after a megabyte's wait.
	done := 0
	for done < len(b) {
		n := min(len(b)-done, 16<<10)
		c.bucket.wait(n)
		w, err := c.Stream.Write(b[done : done+n])
		done += w
		if err != nil {
			return done, err
		}
	}
	return done, nil
}

func (c *streamConn) Close() error {
	err := c.Stream.Close()
	c.once.Do(func() {
		if c.onClose != nil {
			c.onClose()
		}
	})
	return err
}

// bucket paces writes to a rate in bytes per second, shared by every stream
// of one connection. At most a second's worth is ever saved up.
type bucket struct {
	mu    sync.Mutex
	rate  float64
	have  float64
	last  time.Time
	sleep func(time.Duration)
	now   func() time.Time
}

func newBucket(bitsPerSecond int64) *bucket {
	if bitsPerSecond <= 0 {
		return nil
	}
	r := float64(bitsPerSecond) / 8
	return &bucket{rate: r, have: r / 4, sleep: time.Sleep, now: time.Now}
}

func (b *bucket) wait(n int) {
	b.mu.Lock()
	now := b.now()
	if !b.last.IsZero() {
		b.have = min(b.have+now.Sub(b.last).Seconds()*b.rate, b.rate)
	}
	b.last = now
	b.have -= float64(n)
	var d time.Duration
	if b.have < 0 {
		d = time.Duration(-b.have / b.rate * float64(time.Second))
	}
	b.mu.Unlock()
	if d > 0 {
		b.sleep(min(d, 2*time.Second))
	}
}

// listener hands accepted streams to an HTTP server.
type listener struct {
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
	addr   net.Addr
}

func newListener(addr net.Addr) *listener {
	return &listener{conns: make(chan net.Conn, 16), closed: make(chan struct{}), addr: addr}
}

func (l *listener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *listener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *listener) Addr() net.Addr { return l.addr }

var errBusy = errors.New("too busy to take another connection")

// offer gives the server a connection, waiting a little while it is busy.
func (l *listener) offer(c net.Conn) error {
	t := time.NewTimer(5 * time.Second)
	defer t.Stop()
	select {
	case l.conns <- c:
		return nil
	case <-l.closed:
		return net.ErrClosed
	case <-t.C:
		return errBusy
	}
}
