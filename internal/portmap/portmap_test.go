package portmap

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

// Nothing here talks to a real router: the routers are fakes on this machine.

// fakeNATPMP is a NAT-PMP gateway on a local UDP port.
func fakeNATPMP(t *testing.T, external string, givePort int) (addr string, requests func() [][]byte) {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	var mu sync.Mutex
	var got [][]byte
	go func() {
		buf := make([]byte, 64)
		for {
			n, from, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			req := append([]byte(nil), buf[:n]...)
			mu.Lock()
			got = append(got, req)
			mu.Unlock()
			var res []byte
			switch req[1] {
			case 0:
				res = make([]byte, 12)
				res[1] = 128
				ip := netip.MustParseAddr(external).As4()
				copy(res[8:], ip[:])
			case 2:
				res = make([]byte, 16)
				res[1] = 130
				copy(res[8:10], req[4:6])
				binary.BigEndian.PutUint16(res[10:], uint16(givePort))
				copy(res[12:], req[8:12])
			}
			_, _ = conn.WriteTo(res, from)
		}
	}()
	return conn.LocalAddr().String(), func() [][]byte {
		mu.Lock()
		defer mu.Unlock()
		return got
	}
}

func TestNATPMP(t *testing.T) {
	addr, requests := fakeNATPMP(t, "203.0.113.9", 8443)
	c := &natpmpClient{addr: addr, timeout: 200 * time.Millisecond}
	ctx := context.Background()
	ip, err := c.ExternalIP(ctx)
	if err != nil || ip.String() != "203.0.113.9" {
		t.Fatalf("external: %v %v", ip, err)
	}
	port, lease, err := c.Map(ctx, 443, 9443)
	if err != nil || port != 8443 || lease != time.Hour {
		t.Fatalf("map: %d %v %v", port, lease, err)
	}
	if err := c.Unmap(ctx, port, 9443); err != nil {
		t.Fatal(err)
	}
	reqs := requests()
	mapReq, unmapReq := reqs[1], reqs[2]
	if mapReq[1] != 2 || binary.BigEndian.Uint16(mapReq[4:]) != 9443 || binary.BigEndian.Uint16(mapReq[6:]) != 443 || binary.BigEndian.Uint32(mapReq[8:]) != leaseSeconds {
		t.Errorf("map request % x", mapReq)
	}
	if binary.BigEndian.Uint16(unmapReq[6:]) != 0 || binary.BigEndian.Uint32(unmapReq[8:]) != 0 {
		t.Errorf("removing takes port 0 and lease 0: % x", unmapReq)
	}
}

func TestNATPMPNoAnswer(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := &natpmpClient{addr: conn.LocalAddr().String(), timeout: 20 * time.Millisecond}
	if _, err := c.ExternalIP(context.Background()); err == nil {
		t.Fatal("silence is not an answer")
	}
}

const igdXML = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0"><device>
  <deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType>
  <serviceList><service><serviceType>urn:schemas-upnp-org:service:Layer3Forwarding:1</serviceType><controlURL>/l3f</controlURL></service></serviceList>
  <deviceList><device><deviceType>urn:schemas-upnp-org:device:WANDevice:1</deviceType>
    <deviceList><device><deviceType>urn:schemas-upnp-org:device:WANConnectionDevice:1</deviceType>
      <serviceList>
        <service><serviceType>urn:schemas-upnp-org:service:WANPPPConnection:1</serviceType><controlURL>/ppp</controlURL></service>
        <service><serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType><controlURL>/ctl/ip</controlURL></service>
      </serviceList>
    </device></deviceList>
  </device></deviceList>
</device></root>`

// fakeIGD is a UPnP router's web service.
type fakeIGD struct {
	mu       sync.Mutex
	actions  []string
	bodies   []string
	failWith map[string]int // action → UPnP error, once
}

func (f *fakeIGD) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET" && r.URL.Path == "/desc.xml":
		_, _ = io.WriteString(w, igdXML)
	case r.Method == "POST" && r.URL.Path == "/ctl/ip":
		action := r.Header.Get("SOAPAction")
		action = strings.Trim(action[strings.Index(action, "#")+1:], `"`)
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.actions = append(f.actions, action)
		f.bodies = append(f.bodies, string(b))
		code, fail := f.failWith[action]
		if fail {
			delete(f.failWith, action)
		}
		f.mu.Unlock()
		if fail {
			w.WriteHeader(500)
			_, _ = io.WriteString(w, `<s:Envelope><s:Body><s:Fault><detail><UPnPError><errorCode>`+itoa(code)+`</errorCode></UPnPError></detail></s:Fault></s:Body></s:Envelope>`)
			return
		}
		if action == "GetExternalIPAddress" {
			_, _ = io.WriteString(w, `<r><NewExternalIPAddress>100.70.1.2</NewExternalIPAddress></r>`)
			return
		}
		_, _ = io.WriteString(w, `<r/>`)
	default:
		w.WriteHeader(404)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for ; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	return s
}

func openFake(t *testing.T, f *fakeIGD) *upnpClient {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := openRouter(context.Background(), srv.URL+"/desc.xml", netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("192.168.1.5"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestUPnP(t *testing.T) {
	f := &fakeIGD{}
	c := openFake(t, f)
	if !strings.HasSuffix(c.control, "/ctl/ip") || !strings.Contains(c.service, "WANIPConnection") {
		t.Fatalf("found %s %s; the IP service should win over PPP", c.control, c.service)
	}
	ctx := context.Background()
	ip, err := c.ExternalIP(ctx)
	if err != nil || ip.String() != "100.70.1.2" {
		t.Fatalf("external: %v %v", ip, err)
	}
	if !sharedAddr(ip) {
		t.Error("a 100.64.0.0/10 address is a shared address")
	}
	port, lease, err := c.Map(ctx, 443, 9443)
	if err != nil || port != 443 || lease != time.Hour {
		t.Fatalf("map: %d %v %v", port, lease, err)
	}
	add := f.bodies[len(f.bodies)-1]
	for _, want := range []string{"<NewExternalPort>443<", "<NewInternalPort>9443<", "<NewInternalClient>192.168.1.5<", "<NewProtocol>TCP<", "<NewLeaseDuration>3600<"} {
		if !strings.Contains(add, want) {
			t.Errorf("AddPortMapping lacks %s: %s", want, add)
		}
	}
	if err := c.Unmap(ctx, 443, 9443); err != nil {
		t.Fatal(err)
	}
	if f.actions[len(f.actions)-1] != "DeletePortMapping" {
		t.Errorf("actions: %v", f.actions)
	}
}

func TestUPnPPermanentOnly(t *testing.T) {
	f := &fakeIGD{failWith: map[string]int{"AddPortMapping": 725}}
	c := openFake(t, f)
	port, lease, err := c.Map(context.Background(), 443, 9443)
	if err != nil || port != 443 || lease != 0 {
		t.Fatalf("got %d %v %v", port, lease, err)
	}
	if !strings.Contains(f.bodies[len(f.bodies)-1], "<NewLeaseDuration>0<") {
		t.Errorf("the retry should ask for a permanent mapping: %s", f.bodies[len(f.bodies)-1])
	}
}

func TestUPnPConflict(t *testing.T) {
	f := &fakeIGD{failWith: map[string]int{"AddPortMapping": 718}}
	c := openFake(t, f)
	if _, _, err := c.Map(context.Background(), 443, 9443); err == nil || !strings.Contains(err.Error(), "already uses") {
		t.Fatalf("got %v", err)
	}
}

func TestCheckLocation(t *testing.T) {
	gw := netip.MustParseAddr("192.168.1.1")
	if _, err := checkLocation("http://192.168.1.1:5000/desc.xml", gw); err != nil {
		t.Error(err)
	}
	for _, bad := range []string{
		"http://192.168.1.2:5000/desc.xml", // another machine than the one that answered
		"https://192.168.1.1/desc.xml",     // no such router
		"http://metadata.example/desc.xml", // a name
		"file:///etc/passwd",               // not the web
		"http://8.8.8.8/desc.xml",          // the internet
		"http://192.168.1.1@8.8.8.8/x.xml", // dressed up
	} {
		from := gw
		if bad == "http://8.8.8.8/desc.xml" {
			from = netip.MustParseAddr("8.8.8.8")
		}
		if _, err := checkLocation(bad, from); err == nil {
			t.Errorf("%s should be refused", bad)
		}
	}
}

func TestControlURLStaysOnTheRouter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<root><device><serviceList><service><serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType><controlURL>http://8.8.8.8/ctl</controlURL></service></serviceList></device></root>`)
	}))
	defer srv.Close()
	if _, err := openRouter(context.Background(), srv.URL, netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("192.168.1.5")); err == nil {
		t.Fatal("a control address elsewhere must be refused")
	}
}

func TestSSDPLocation(t *testing.T) {
	msg := "HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=120\r\nST: x\r\nLocation: http://192.168.1.1:5000/rootDesc.xml\r\n\r\n"
	if got := ssdpLocation([]byte(msg)); got != "http://192.168.1.1:5000/rootDesc.xml" {
		t.Errorf("got %q", got)
	}
	if got := ssdpLocation([]byte("HTTP/1.1 200 OK\r\n\r\n")); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestParseRouteTable(t *testing.T) {
	table := "Iface\tDestination\tGateway \tFlags\n" +
		"eth0\t00000000\t0101A8C0\t0003\n" +
		"eth0\t0001A8C0\t00000000\t0001\n" +
		"wg0\t00000000\t08080808\t0003\n"
	got := parseRouteTable(table)
	if len(got) != 1 || got[0].String() != "192.168.1.1" {
		t.Errorf("got %v", got)
	}
}

func TestSharedAddr(t *testing.T) {
	for ip, want := range map[string]bool{"203.0.113.9": false, "8.8.8.8": false, "192.168.0.2": true, "10.0.0.1": true, "100.64.0.1": true, "100.127.255.255": true, "100.128.0.1": false, "0.0.0.0": true} {
		if got := sharedAddr(netip.MustParseAddr(ip)); got != want {
			t.Errorf("%s: got %v", ip, got)
		}
	}
}

// fakeRouter records what the keeper asks of it.
type fakeRouter struct {
	mu      sync.Mutex
	maps    int
	unmaps  int
	failMap error
	lease   time.Duration
	mapped  chan struct{}
}

func (f *fakeRouter) Protocol() string { return "test" }
func (f *fakeRouter) ExternalIP(context.Context) (netip.Addr, error) {
	return netip.MustParseAddr("203.0.113.9"), nil
}
func (f *fakeRouter) Map(_ context.Context, ext, _ int) (int, time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failMap != nil {
		return 0, 0, f.failMap
	}
	f.maps++
	if f.mapped != nil {
		select {
		case f.mapped <- struct{}{}:
		default:
		}
	}
	return ext, f.lease, nil
}
func (f *fakeRouter) Unmap(context.Context, int, int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unmaps++
	return nil
}

func withRouter(t *testing.T, r router, err error) {
	t.Helper()
	old := discover
	discover = func(context.Context) (router, error) { return r, err }
	t.Cleanup(func() { discover = old })
}

func waitFor(t *testing.T, what string, f func() bool) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if f() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestKeeperRenewsAndRemoves(t *testing.T) {
	f := &fakeRouter{lease: 40 * time.Millisecond}
	withRouter(t, f, nil)
	k := &Keeper{Log: slog.New(slog.DiscardHandler)}
	if k.Status().On {
		t.Fatal("off until asked")
	}
	k.Set(443, 9443)
	waitFor(t, "a working mapping", func() bool { return k.Status().Working })
	st := k.Status()
	if st.ExternalPort != 443 || st.ExternalIP != "203.0.113.9" || st.Shared || st.Protocol != "test" || !st.On {
		t.Errorf("status %+v", st)
	}
	waitFor(t, "a renewal", func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.maps >= 3 })
	k.Stop()
	f.mu.Lock()
	unmaps := f.unmaps
	f.mu.Unlock()
	if unmaps != 1 {
		t.Errorf("unmaps = %d, want 1", unmaps)
	}
	if k.Status().On {
		t.Error("stopped means off")
	}
}

func TestKeeperReportsFailure(t *testing.T) {
	withRouter(t, nil, errNoRouter)
	k := &Keeper{Log: slog.New(slog.DiscardHandler)}
	k.Set(443, 9443)
	waitFor(t, "the error", func() bool { return k.Status().Error != "" })
	if k.Status().Working {
		t.Error("nothing works without a router")
	}
	k.Stop()
}

func TestKeeperMappingRefused(t *testing.T) {
	f := &fakeRouter{failMap: errors.New("no")}
	withRouter(t, f, nil)
	k := &Keeper{Log: slog.New(slog.DiscardHandler)}
	k.Set(443, 9443)
	waitFor(t, "the error", func() bool { return k.Status().Error == "no" })
	k.Stop()
	if f.unmaps != 0 {
		t.Error("nothing was mapped, so nothing to remove")
	}
}

func TestLook(t *testing.T) {
	withRouter(t, &fakeRouter{}, nil)
	p := Look(context.Background())
	if !p.Found || p.ExternalIP != "203.0.113.9" || p.Shared {
		t.Errorf("got %+v", p)
	}
	withRouter(t, nil, errNoRouter)
	if p := Look(context.Background()); p.Found || p.Error == "" {
		t.Errorf("got %+v", p)
	}
}
