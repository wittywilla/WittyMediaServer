// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package portmap

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakePCP is a PCP gateway on a local UDP port. It gives the outside port it
// is told to (0: the one asked for) and remembers what is open.
type fakePCP struct {
	conn     net.PacketConn
	mu       sync.Mutex
	open     map[string]int // "proto/internal" → outside port
	givePort int
	result   byte
	natpmp   bool // answer as a router that only knows NAT-PMP
	requests int
}

func newFakePCP(t *testing.T) *fakePCP {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakePCP{conn: conn, open: map[string]int{}}
	t.Cleanup(func() { conn.Close() })
	go f.serve()
	return f
}

func (f *fakePCP) serve() {
	buf := make([]byte, 1100)
	for {
		n, from, err := f.conn.ReadFrom(buf)
		if err != nil {
			return
		}
		req := append([]byte(nil), buf[:n]...)
		f.mu.Lock()
		f.requests++
		if f.natpmp {
			f.mu.Unlock()
			res := make([]byte, 8)
			res[1], res[3] = 0x80|req[1], 1 // version 0, "unsupported version"
			_, _ = f.conn.WriteTo(res, from)
			continue
		}
		if n < 60 || req[0] != 2 || req[1] != 1 {
			f.mu.Unlock()
			continue
		}
		res := make([]byte, 60)
		res[0], res[1], res[3] = 2, 0x81, f.result
		lease := binary.BigEndian.Uint32(req[4:8])
		binary.BigEndian.PutUint32(res[4:], lease)
		copy(res[24:44], req[24:44]) // nonce, protocol, internal port
		internal := binary.BigEndian.Uint16(req[40:42])
		port := binary.BigEndian.Uint16(req[42:44])
		if f.givePort != 0 {
			port = uint16(f.givePort)
		}
		key := "tcp/"
		if req[36] == 17 {
			key = "udp/"
		}
		key += itoa(int(internal))
		if f.result == 0 {
			if lease == 0 {
				delete(f.open, key)
			} else {
				f.open[key] = int(port)
			}
		}
		binary.BigEndian.PutUint16(res[42:], port)
		ext := netip.MustParseAddr("::ffff:203.0.113.7").As16()
		copy(res[44:], ext[:])
		f.mu.Unlock()
		_, _ = f.conn.WriteTo(res, from)
	}
}

func (f *fakePCP) client() *pcpClient {
	return &pcpClient{addr: f.conn.LocalAddr().String(), timeout: 200 * time.Millisecond}
}

func (f *fakePCP) opened() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int{}
	for k, v := range f.open {
		out[k] = v
	}
	return out
}

func TestPCPMapsBothProtocolsAndRemoves(t *testing.T) {
	f := newFakePCP(t)
	c := f.client()
	ctx := context.Background()
	ip, err := c.ExternalIP(ctx)
	if err != nil || ip.String() != "203.0.113.7" {
		t.Fatalf("outside address = %v, %v", ip, err)
	}
	for _, proto := range []string{UDP, TCP} {
		port, lease, err := c.MapProto(ctx, proto, 4100, 4100)
		if err != nil || port != 4100 || lease != leaseSeconds*time.Second {
			t.Fatalf("%s: got port %d, lease %v, err %v", proto, port, lease, err)
		}
	}
	if got := f.opened(); got["udp/4100"] != 4100 || got["tcp/4100"] != 4100 {
		t.Fatalf("open on the router: %v", got)
	}
	if err := c.UnmapProto(ctx, UDP, 4100, 4100); err != nil {
		t.Fatal(err)
	}
	if got := f.opened(); len(got) != 1 || got["tcp/4100"] != 4100 {
		t.Fatalf("after removing UDP: %v", got)
	}
}

func TestPCPRefusalAndNATPMPOnly(t *testing.T) {
	f := newFakePCP(t)
	f.mu.Lock()
	f.result = 2
	f.mu.Unlock()
	if _, _, err := f.client().MapProto(context.Background(), UDP, 4100, 4100); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("a refusal should say why: %v", err)
	}
	f.mu.Lock()
	f.result, f.natpmp = 0, true
	f.mu.Unlock()
	if _, err := f.client().ExternalIP(context.Background()); !errors.Is(err, errNotPCP) {
		t.Fatalf("a NAT-PMP router should be told apart: %v", err)
	}
}

func TestPCPIgnoresAnswersToOtherQuestions(t *testing.T) {
	// A gateway that answers with another nonce: not our mapping.
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	go func() {
		buf := make([]byte, 1100)
		for {
			_, from, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			res := make([]byte, 60)
			res[0], res[1] = 2, 0x81
			_, _ = conn.WriteTo(res, from)
		}
	}()
	c := &pcpClient{addr: conn.LocalAddr().String(), timeout: 20 * time.Millisecond}
	if _, _, err := c.MapProto(context.Background(), UDP, 1, 1); err == nil {
		t.Fatal("an answer with the wrong nonce was believed")
	}
}

func TestNATPMPMapsUDP(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ops := make(chan byte, 4)
	go func() {
		buf := make([]byte, 64)
		for {
			n, from, err := conn.ReadFrom(buf)
			if err != nil || n < 12 {
				return
			}
			ops <- buf[1]
			res := make([]byte, 16)
			res[1] = buf[1] + 128
			copy(res[8:10], buf[4:6])
			copy(res[10:12], buf[6:8])
			copy(res[12:], buf[8:12])
			_, _ = conn.WriteTo(res, from)
		}
	}()
	c := &natpmpClient{addr: conn.LocalAddr().String(), timeout: 200 * time.Millisecond}
	port, _, err := c.MapProto(context.Background(), UDP, 4100, 4100)
	if err != nil || port != 4100 {
		t.Fatalf("port %d, err %v", port, err)
	}
	if op := <-ops; op != 1 {
		t.Fatalf("UDP is NAT-PMP operation 1, sent %d", op)
	}
}

// fakeFirewall is a router with UPnP's IPv6 firewall service beside its
// forwarding one.
type fakeFirewall struct {
	fakeIGD
	mu2    sync.Mutex
	next   int
	holes  map[string]string // id → body
	refuse bool
}

const igd6XML = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0"><device>
  <deviceList><device>
    <deviceList><device>
      <serviceList>
        <service><serviceType>urn:schemas-upnp-org:service:WANIPConnection:2</serviceType><controlURL>/ctl/ip</controlURL></service>
        <service><serviceType>urn:schemas-upnp-org:service:WANIPv6FirewallControl:1</serviceType><controlURL>/ctl/fw6</controlURL></service>
      </serviceList>
    </device></deviceList>
  </device></deviceList>
</device></root>`

func (f *fakeFirewall) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET" && r.URL.Path == "/desc.xml":
		_, _ = io.WriteString(w, igd6XML)
	case r.Method == "POST" && r.URL.Path == "/ctl/fw6":
		action := r.Header.Get("SOAPAction")
		action = strings.Trim(action[strings.Index(action, "#")+1:], `"`)
		b, _ := io.ReadAll(r.Body)
		f.mu2.Lock()
		defer f.mu2.Unlock()
		if f.holes == nil {
			f.holes = map[string]string{}
		}
		switch action {
		case "AddPinhole":
			if f.refuse {
				w.WriteHeader(500)
				_, _ = io.WriteString(w, `<s:Fault><errorCode>606</errorCode></s:Fault>`)
				return
			}
			f.next++
			id := itoa(f.next)
			f.holes[id] = string(b)
			_, _ = io.WriteString(w, `<r><UniqueID>`+id+`</UniqueID></r>`)
		case "DeletePinhole":
			m := pinholeIDRe.FindSubmatch(b)
			if m != nil {
				delete(f.holes, string(m[1]))
			}
			_, _ = io.WriteString(w, `<r/>`)
		}
	default:
		f.fakeIGD.ServeHTTP(w, r)
	}
}

func TestUPnPIPv6Pinhole(t *testing.T) {
	f := &fakeFirewall{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c, err := openRouter(context.Background(), srv.URL+"/desc.xml", netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("192.168.1.5"))
	if err != nil {
		t.Fatal(err)
	}
	if !canPinhole(c) {
		t.Fatal("the firewall service was not found")
	}
	addr := netip.MustParseAddr("2001:db8:1::5")
	id, lease, err := c.Open(context.Background(), UDP, addr, 4100)
	if err != nil || id != "1" || lease <= 0 {
		t.Fatalf("id %q, lease %v, err %v", id, lease, err)
	}
	body := f.holes["1"]
	for _, want := range []string{"<InternalClient>2001:db8:1::5</InternalClient>", "<InternalPort>4100</InternalPort>", "<Protocol>17</Protocol>", "<RemotePort>0</RemotePort>"} {
		if !strings.Contains(body, want) {
			t.Errorf("the request lacks %s: %s", want, body)
		}
	}
	if err := c.Shut(context.Background(), UDP, addr, 4100, id); err != nil || len(f.holes) != 0 {
		t.Fatalf("closing: %v, left open: %v", err, f.holes)
	}
	if err := c.Shut(context.Background(), UDP, addr, 4100, "1</UniqueID><x>"); err == nil {
		t.Fatal("an id that isn't a number must not reach the router")
	}
	// A router without the service says so, and is never asked.
	plain := openFake(t, &fakeIGD{})
	if canPinhole(plain) {
		t.Fatal("a router without the service can't open its firewall")
	}
	if _, _, err := plain.Open(context.Background(), UDP, addr, 4100); err == nil {
		t.Fatal("expected a refusal")
	}
}

// ---------------------------------------------------------------- the Holder

type fakeClock struct {
	mu     sync.Mutex
	t      time.Time
	waits  []time.Duration
	wake   chan time.Time
	waited chan time.Duration
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Unix(1_800_000_000, 0), wake: make(chan time.Time), waited: make(chan time.Duration, 16)}
}
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.waits = append(c.waits, d)
	c.t = c.t.Add(d)
	c.mu.Unlock()
	c.waited <- d
	return c.wake
}

// multiRouter is a router that forwards both protocols and opens pinholes.
type multiRouter struct {
	mu       sync.Mutex
	external string
	lease    time.Duration
	give     map[string]int // proto → another port than asked
	fail     map[string]error
	maps     map[string]int
	open     map[string]bool
	holes    map[string]bool
	holeErr  error
}

func newMultiRouter() *multiRouter {
	return &multiRouter{external: "203.0.113.9", lease: time.Hour, give: map[string]int{}, fail: map[string]error{}, maps: map[string]int{}, open: map[string]bool{}, holes: map[string]bool{}}
}
func (f *multiRouter) Protocol() string { return "test" }
func (f *multiRouter) ExternalIP(context.Context) (netip.Addr, error) {
	return netip.MustParseAddr(f.external), nil
}
func (f *multiRouter) Map(ctx context.Context, e, i int) (int, time.Duration, error) {
	return f.MapProto(ctx, TCP, e, i)
}
func (f *multiRouter) Unmap(ctx context.Context, e, i int) error {
	return f.UnmapProto(ctx, TCP, e, i)
}
func (f *multiRouter) MapProto(_ context.Context, proto string, ext, _ int) (int, time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail[proto]; err != nil {
		return 0, 0, err
	}
	f.maps[proto]++
	if p := f.give[proto]; p != 0 {
		ext = p
	}
	f.open[proto+"/"+itoa(ext)] = true
	return ext, f.lease, nil
}
func (f *multiRouter) UnmapProto(_ context.Context, proto string, ext, _ int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.open, proto+"/"+itoa(ext))
	return nil
}
func (f *multiRouter) Open(_ context.Context, proto string, a netip.Addr, port int) (string, time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.holeErr != nil {
		return "", 0, f.holeErr
	}
	f.holes[proto+"/"+a.String()+"/"+itoa(port)] = true
	return "pcp", f.lease, nil
}
func (f *multiRouter) Shut(_ context.Context, proto string, a netip.Addr, port int, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.holes, proto+"/"+a.String()+"/"+itoa(port))
	return nil
}

var bothWants = []Want{{UDP, 4100}, {TCP, 4100}}

func TestHolderMapsRenewsAtHalfTheLease(t *testing.T) {
	r := newMultiRouter()
	h := &Holder{clock: newFakeClock(), find: func(context.Context) (router, error) { return r, nil }}
	wait := h.step(context.Background(), bothWants)
	if wait != 30*time.Minute {
		t.Fatalf("an hour's lease is renewed after half of it, not %v", wait)
	}
	st := h.Status()
	if !st.Usable() || st.ExternalIP != "203.0.113.9" || len(st.Mappings) != 2 || st.Renewed == 0 {
		t.Fatalf("status: %+v", st)
	}
	h.step(context.Background(), bothWants)
	if r.maps[UDP] != 2 || r.maps[TCP] != 2 {
		t.Fatalf("a second step renews both: %v", r.maps)
	}
	h.release()
	if len(r.open) != 0 {
		t.Fatalf("left open on the router after stopping: %v", r.open)
	}
}

func TestHolderSharedAddressIsNotUsable(t *testing.T) {
	for _, ip := range []string{"100.72.3.4", "192.168.0.2", "10.0.0.9"} {
		r := newMultiRouter()
		r.external = ip
		h := &Holder{clock: newFakeClock(), find: func(context.Context) (router, error) { return r, nil }}
		h.step(context.Background(), bothWants)
		if st := h.Status(); !st.Shared || st.Usable() {
			t.Errorf("%s is behind another router or a shared address, and must not count as working: %+v", ip, st)
		}
	}
}

func TestHolderBacksOffAndRecovers(t *testing.T) {
	r := newMultiRouter()
	var findErr error = errNoRouter
	h := &Holder{clock: newFakeClock(), find: func(context.Context) (router, error) {
		if findErr != nil {
			return nil, findErr
		}
		return r, nil
	}}
	var waits []time.Duration
	for range 7 {
		waits = append(waits, h.step(context.Background(), bothWants))
	}
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 10 * time.Minute, 10 * time.Minute}
	for i := range want {
		if waits[i] != want[i] {
			t.Fatalf("waits after failures = %v, want %v", waits, want)
		}
	}
	if st := h.Status(); st.Usable() || st.Error == "" {
		t.Fatalf("no router must say so: %+v", st)
	}
	findErr = nil
	if wait := h.step(context.Background(), bothWants); wait != 30*time.Minute || !h.Status().Usable() {
		t.Fatalf("after the router came back: wait %v, %+v", wait, h.Status())
	}
	// One protocol refused: not usable, the router is looked for again, and the wait starts short.
	r.fail[UDP] = errors.New("refused")
	if wait := h.step(context.Background(), bothWants); wait != 30*time.Second || h.Status().Usable() {
		t.Fatalf("UDP refused: wait %v, %+v", wait, h.Status())
	}
}

func TestHolderGivesBackAnotherPort(t *testing.T) {
	r := newMultiRouter()
	r.give[UDP] = 5000
	h := &Holder{clock: newFakeClock(), find: func(context.Context) (router, error) { return r, nil }}
	h.step(context.Background(), bothWants)
	st := h.Status()
	if st.Usable() || st.Mappings[0].Working || !strings.Contains(st.Mappings[0].Error, "5000") {
		t.Fatalf("a different outside port is not the mapping that was asked for: %+v", st)
	}
	if r.open["udp/5000"] {
		t.Fatal("the unwanted mapping was left on the router")
	}
}

func TestHolderPinholes(t *testing.T) {
	r := newMultiRouter()
	addrs := []netip.Addr{netip.MustParseAddr("2001:db8::5")}
	h := &Holder{IPv6: true, clock: newFakeClock(), find: func(context.Context) (router, error) { return r, nil },
		addr6: func() []netip.Addr { return addrs }}
	h.step(context.Background(), bothWants)
	st := h.Status()
	if len(st.Pinholes) != 2 || !st.Pinholes[0].Working || !r.holes["udp/2001:db8::5/4100"] || !r.holes["tcp/2001:db8::5/4100"] {
		t.Fatalf("pinholes: %+v, on the router: %v", st.Pinholes, r.holes)
	}
	// The machine's address changed: the old opening is closed, the new one made.
	addrs = []netip.Addr{netip.MustParseAddr("2001:db8::6")}
	h.step(context.Background(), bothWants)
	if r.holes["udp/2001:db8::5/4100"] || !r.holes["udp/2001:db8::6/4100"] {
		t.Fatalf("after the address changed: %v", r.holes)
	}
	h.release()
	if len(r.holes) != 0 {
		t.Fatalf("left open after stopping: %v", r.holes)
	}
	// A router with no way to open its firewall is reported, not assumed.
	h2 := &Holder{IPv6: true, clock: newFakeClock(), find: func(context.Context) (router, error) { return &fakeRouter{lease: time.Hour}, nil },
		addr6: func() []netip.Addr { return addrs }, find6: func(context.Context) pinholer { return nil }}
	h2.step(context.Background(), []Want{{TCP, 4100}})
	if st := h2.Status(); len(st.Pinholes) != 1 || st.Pinholes[0].Working || st.Pinholes[0].Error == "" {
		t.Fatalf("no firewall control: %+v", st.Pinholes)
	}
}

func TestHolderRunsAndStops(t *testing.T) {
	r := newMultiRouter()
	clock := newFakeClock()
	changed := make(chan struct{}, 8)
	h := &Holder{clock: clock, find: func(context.Context) (router, error) { return r, nil }, OnChange: func() { changed <- struct{}{} }}
	h.Start(bothWants)
	if d := <-clock.waited; d != 30*time.Minute {
		t.Fatalf("first wait %v", d)
	}
	<-changed
	clock.wake <- clock.Now() // the lease is half over
	<-clock.waited
	r.mu.Lock()
	maps := r.maps[UDP]
	r.mu.Unlock()
	if maps != 2 {
		t.Fatalf("renewed %d times", maps)
	}
	h.Stop()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.open) != 0 {
		t.Fatalf("Stop left mappings on the router: %v", r.open)
	}
	if h.Status().On {
		t.Fatal("stopped, but the status says on")
	}
}

func TestParseRoute6(t *testing.T) {
	text := "00000000000000000000000000000000 00 00000000000000000000000000000000 00 fe800000000000000000000000000001 00000400 00000001 00000000 00450003     eth0\n" +
		"20010db8000000000000000000000000 40 00000000000000000000000000000000 00 00000000000000000000000000000000 00000100 00000001 00000000 00000001     eth0\n"
	got := parseRoute6(text)
	if len(got) != 1 || got[0] != "fe80::1%eth0" {
		t.Fatalf("got %v", got)
	}
}
