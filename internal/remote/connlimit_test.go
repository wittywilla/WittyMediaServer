package remote

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"syscall"
	"testing"
	"time"
)

// stranger counts every address as somewhere on the internet.
var stranger = func() *Policy {
	return &Policy{Away: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}
}

func limitedServer(t *testing.T, l *ConnLimit) *net.TCPAddr {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if l.Log == nil {
		l.Log = slog.New(slog.DiscardHandler)
	}
	lim := l.Wrap(ln)
	t.Cleanup(func() { _ = lim.Close() })
	go func() {
		for {
			c, err := lim.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = c.Write([]byte("x"))
				_, _ = io.Copy(io.Discard, c)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr)
}

// dial says whether the server took this connection: it greets the ones it keeps.
func dial(t *testing.T, addr *net.TCPAddr) (net.Conn, bool) {
	t.Helper()
	c, err := net.DialTCP("tcp", nil, addr)
	if errors.Is(err, syscall.ECONNRESET) { // refused before the call came back
		return nil, false
	}
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	b := make([]byte, 1)
	if _, err := io.ReadFull(c, b); err != nil {
		_ = c.Close()
		return nil, false
	}
	return c, true
}

func TestOneAddressCantTakeAllTheConnections(t *testing.T) {
	addr := limitedServer(t, &ConnLimit{Total: 100, PerAddress: 5, Policy: stranger})
	var open []net.Conn
	defer func() {
		for _, c := range open {
			_ = c.Close()
		}
	}()
	for i := range 5 {
		c, ok := dial(t, addr)
		if !ok {
			t.Fatalf("connection %d was refused", i)
		}
		open = append(open, c)
	}
	if c, ok := dial(t, addr); ok {
		_ = c.Close()
		t.Fatal("past the limit for one address")
	}
	// Closing one makes room.
	_ = open[0].Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if c, ok := dial(t, addr); ok {
			open[0] = c
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("a closed connection left no room")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestTheTotalHoldsAcrossAddresses(t *testing.T) {
	addr := limitedServer(t, &ConnLimit{Total: 3, PerAddress: 100, Policy: stranger})
	for i := range 3 {
		c, ok := dial(t, addr)
		if !ok {
			t.Fatalf("connection %d was refused", i)
		}
		defer c.Close()
	}
	if c, ok := dial(t, addr); ok {
		_ = c.Close()
		t.Fatal("past the total")
	}
}

// Nobody on the internet can fill the place up so that the owner can't get in at home,
// and a reverse proxy isn't one visitor.
func TestHomeAndProxiesAreCountedFairly(t *testing.T) {
	var l ConnLimit
	l.Total, l.PerAddress = 2, 1
	l.Policy = func() *Policy {
		return &Policy{
			Trusted: []netip.Prefix{netip.MustParsePrefix("192.168.1.2/32")},
			Away:    []netip.Prefix{netip.MustParsePrefix("100.64.0.0/10")},
		}
	}
	addr := func(s string) netip.Addr { return netip.MustParseAddr(s) }
	if !l.enter(addr("203.0.113.1")) || !l.enter(addr("203.0.113.2")) {
		t.Fatal("within the total")
	}
	if l.enter(addr("203.0.113.3")) {
		t.Error("a third stranger: past the total")
	}
	if !l.enter(addr("192.168.1.20")) {
		t.Error("someone at home was kept out by strangers")
	}
	for range homeReserve - 2 {
		if !l.enter(addr("192.168.1.2")) {
			t.Fatal("a trusted proxy isn't one visitor")
		}
	}
	if !l.enter(addr("127.0.0.1")) {
		t.Error("this machine isn't one visitor either")
	}
	if l.enter(addr("192.168.1.21")) {
		t.Error("the reserve ends too")
	}
	l.leave(addr("203.0.113.1"))
	if l.perAddr["203.0.113.1"] != 0 {
		t.Errorf("%v", l.perAddr)
	}
}

// An IPv6 network counts as one address, as for sign-in limits.
func TestConnLimitGroupsIPv6Networks(t *testing.T) {
	l := ConnLimit{Total: 100, PerAddress: 2, Policy: func() *Policy { return nil }}
	a := netip.MustParseAddr("2001:db8:1:2::1")
	b := netip.MustParseAddr("2001:db8:1:2::2")
	c := netip.MustParseAddr("2001:db8:1:2::3")
	if !l.enter(a) || !l.enter(b) {
		t.Fatal("within the limit")
	}
	if l.enter(c) {
		t.Error("another address in the same network got past it")
	}
	if !l.enter(netip.MustParseAddr("2001:db8:1:3::1")) {
		t.Error("another network")
	}
}

// Sending a file through this listener still uses the system's own copy, and the
// standard server still sees what it looks for on a connection.
func TestLimitedConnKeepsWhatTheServerUses(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l := &ConnLimit{Total: 10, PerAddress: 10, Policy: stranger, Log: slog.New(slog.DiscardHandler)}
	lim := l.Wrap(ln)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	srv.Listener = lim
	srv.Start()
	defer srv.Close()
	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(b) != "ok" {
		t.Fatalf("%q", b)
	}
	bare, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer bare.Close()
	c, err := net.Dial("tcp", bare.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	in, err := l.Wrap(bare).Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	if _, ok := in.(io.ReaderFrom); !ok {
		t.Error("no ReadFrom: files would be copied through memory")
	}
	if _, ok := in.(interface{ CloseWrite() error }); !ok {
		t.Error("no CloseWrite")
	}
}

func TestDefaultLimitsLeaveARoomFullOfDevices(t *testing.T) {
	// A browser opens 6 connections per server at most, plus a socket for each tab. A home of
	// ten devices behind one address, on the way in from elsewhere, is under this.
	if DefaultPerAddress < 100 {
		t.Errorf("%d", DefaultPerAddress)
	}
}
