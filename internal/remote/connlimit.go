package remote

import (
	"io"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"time"
)

// Each open connection costs a descriptor and some memory, so a stranger holding open as
// many as they can is a way to stop the server for everyone.
const (
	// DefaultPerAddress is how many one address can hold (an IPv6 /64 counts as one address).
	DefaultPerAddress = 128
	// homeReserve is room kept for the home network once the rest is full, so nobody on the
	// internet can fill the place up and lock the owner out at home.
	homeReserve = 64
)

// ConnLimit counts open connections. One ConnLimit covers every listener wrapped with it.
type ConnLimit struct {
	Total      int
	PerAddress int
	// Policy is the owner's current choices: a trusted reverse proxy is many visitors, so
	// it isn't limited as one address.
	Policy func() *Policy
	Log    *slog.Logger

	mu       sync.Mutex
	total    int
	perAddr  map[string]int
	lastNote time.Time
	refused  int
}

// Wrap counts the connections a listener accepts, and closes those over the limits.
func (l *ConnLimit) Wrap(ln net.Listener) net.Listener { return &limitedListener{Listener: ln, l: l} }

type limitedListener struct {
	net.Listener
	l *ConnLimit
}

func (ln *limitedListener) Accept() (net.Conn, error) {
	for {
		c, err := ln.Listener.Accept()
		if err != nil {
			return nil, err
		}
		a := peerAddr(c.RemoteAddr().String())
		if ln.l.enter(a) {
			return &limitedConn{Conn: c, l: ln.l, addr: a}, nil
		}
		// Reset, not a goodbye: refusing someone shouldn't leave anything waiting here.
		if tc, ok := c.(*net.TCPConn); ok {
			_ = tc.SetLinger(0)
		}
		_ = c.Close()
	}
}

func (l *ConnLimit) policy() *Policy {
	if l.Policy == nil {
		return nil
	}
	return l.Policy()
}

// enter counts a connection, unless it is over a limit.
func (l *ConnLimit) enter(a netip.Addr) bool {
	p := l.policy()
	where := p.Classify(a)
	limit := l.Total
	if where.AtHome() {
		limit += homeReserve
	}
	one := where != Machine && !p.trusts(a)
	key := LimitKey(a)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.total >= limit || (one && l.perAddr[key] >= l.PerAddress) {
		l.noteRefused(a)
		return false
	}
	if l.perAddr == nil {
		l.perAddr = map[string]int{}
	}
	l.total++
	l.perAddr[key]++
	return true
}

func (l *ConnLimit) leave(a netip.Addr) {
	key := LimitKey(a)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.total--
	if l.perAddr[key] <= 1 {
		delete(l.perAddr, key)
		return
	}
	l.perAddr[key]--
}

// noteRefused logs refusals at most once a minute: someone flooding the port shouldn't
// fill the log as well.
func (l *ConnLimit) noteRefused(a netip.Addr) {
	l.refused++
	if l.Log == nil || time.Since(l.lastNote) < time.Minute {
		return
	}
	l.Log.Warn("refused connections: too many open at once", "cat", "remote", "from", a.String(), "refused", l.refused, "open", l.total)
	l.lastNote, l.refused = time.Now(), 0
}

// limitedConn gives its place back when closed.
type limitedConn struct {
	net.Conn
	l    *ConnLimit
	addr netip.Addr
	once sync.Once
}

func (c *limitedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { c.l.leave(c.addr) })
	return err
}

// ReadFrom keeps the system's own way of sending a file, which the standard server only
// uses on a connection that offers this.
func (c *limitedConn) ReadFrom(r io.Reader) (int64, error) { return io.Copy(c.Conn, r) }

// CloseWrite lets the standard server close its side first, as it does after an error,
// so that the client can read why.
func (c *limitedConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}
