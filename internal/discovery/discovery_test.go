package discovery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"
)

var testInfo = Info{ID: "0123456789abcdef0123456789abcdef", Name: "Home.Server", Version: "0.5.0", API: 1, Port: 9797}
var testIPs = []net.IP{net.IPv4(192, 168, 1, 20), net.IPv4(10, 0, 0, 5)}

func question(t *testing.T, id uint16, name string, typ dnsmessage.Type, class dnsmessage.Class) []byte {
	t.Helper()
	n, err := dnsmessage.NewName(name)
	if err != nil {
		t.Fatal(err)
	}
	b, err := (&dnsmessage.Message{Header: dnsmessage.Header{ID: id}, Questions: []dnsmessage.Question{{Name: n, Type: typ, Class: class}}}).Pack()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func parse(t *testing.T, b []byte) dnsmessage.Message {
	t.Helper()
	var m dnsmessage.Message
	if err := m.Unpack(b); err != nil {
		t.Fatalf("not a DNS message: %v", err)
	}
	if !m.Response || !m.Authoritative {
		t.Errorf("header: %+v", m.Header)
	}
	return m
}

func TestBrowseFindsTheServer(t *testing.T) {
	b, unicast := answer(question(t, 0, "_wittymedia._tcp.local.", dnsmessage.TypePTR, dnsmessage.ClassINET), mdnsPort, testInfo, testIPs)
	if b == nil || unicast {
		t.Fatalf("answer: %v unicast=%v", b, unicast)
	}
	m := parse(t, b)
	if len(m.Answers) != 1 {
		t.Fatalf("answers: %+v", m.Answers)
	}
	ptr := m.Answers[0].Body.(*dnsmessage.PTRResource)
	inst := ptr.PTR.String()
	if !strings.HasPrefix(inst, "Home Server (0123)._wittymedia._tcp.local") {
		t.Errorf("instance %q", inst)
	}
	var txt, host string
	var port uint16
	var ips []string
	for _, r := range m.Additionals {
		switch body := r.Body.(type) {
		case *dnsmessage.TXTResource:
			txt = strings.Join(body.TXT, " ")
		case *dnsmessage.SRVResource:
			port, host = body.Port, body.Target.String()
		case *dnsmessage.AResource:
			ips = append(ips, net.IP(body.A[:]).String())
		}
		if r.Header.Class&flushBit == 0 {
			t.Errorf("%v should be marked as unique", r.Header)
		}
	}
	if port != 9797 || host != "wms-01234567.local." {
		t.Errorf("srv: %d %q", port, host)
	}
	if want := "id=" + testInfo.ID + " name=Home.Server v=0.5.0 api=1 tls=0"; txt != want {
		t.Errorf("txt %q, want %q", txt, want)
	}
	if strings.Join(ips, ",") != "192.168.1.20,10.0.0.5" {
		t.Errorf("addresses %v", ips)
	}
}

// Only what the owner agreed to: a name, a random ID, a version, and how to connect.
func TestAnnouncesNothingElse(t *testing.T) {
	keys := []string{}
	for _, kv := range txtRecord(testInfo) {
		k, _, _ := strings.Cut(kv, "=")
		keys = append(keys, k)
	}
	if strings.Join(keys, ",") != "id,name,v,api,tls" {
		t.Errorf("announced keys: %v", keys)
	}
	var reply map[string]any
	if err := json.Unmarshal(udpReply([]byte(UDPQuestion), net.IPv4(192, 168, 1, 9), testInfo), &reply); err != nil {
		t.Fatal(err)
	}
	if len(reply) != 7 {
		t.Errorf("UDP reply: %v", reply)
	}
	for _, k := range []string{"service", "id", "name", "version", "api", "port", "tls"} {
		if _, ok := reply[k]; !ok {
			t.Errorf("no %s in %v", k, reply)
		}
	}
}

func TestOtherQuestionsGetNoAnswer(t *testing.T) {
	for name, q := range map[string][]byte{
		"someone else's service": question(t, 0, "_ipp._tcp.local.", dnsmessage.TypePTR, dnsmessage.ClassINET),
		"wrong class":            question(t, 0, "_wittymedia._tcp.local.", dnsmessage.TypePTR, dnsmessage.ClassCHAOS),
		"nonsense":               []byte("this is not a DNS message at all"),
		"empty":                  {},
	} {
		if b, _ := answer(q, mdnsPort, testInfo, testIPs); b != nil {
			t.Errorf("%s: answered", name)
		}
	}
	// A response is never answered, or two servers would talk forever. With loopback
	// on, the server hears its own announcements and answers too.
	resp, _ := answer(question(t, 0, "_wittymedia._tcp.local.", dnsmessage.TypePTR, dnsmessage.ClassINET), mdnsPort, testInfo, testIPs)
	hello, _ := announcement(testInfo, testIPs, false)
	bye, _ := announcement(testInfo, testIPs, true)
	for name, b := range map[string][]byte{"an answer": resp, "its announcement": hello, "its goodbye": bye} {
		if r, _ := answer(b, mdnsPort, testInfo, testIPs); r != nil {
			t.Errorf("answered %s", name)
		}
	}
}

func TestLegacyAndDirectQueries(t *testing.T) {
	// Asked from an ordinary port: straight back, with its ID and short lifetimes.
	b, unicast := answer(question(t, 77, "_wittymedia._tcp.local.", dnsmessage.TypePTR, dnsmessage.ClassINET), 50123, testInfo, testIPs)
	m := parse(t, b)
	if !unicast || m.ID != 77 || len(m.Questions) != 1 || m.Answers[0].Header.TTL != 10 {
		t.Errorf("legacy: unicast=%v %+v", unicast, m.Header)
	}
	for _, r := range m.Additionals {
		if r.Header.Class&flushBit != 0 {
			t.Error("legacy answers must not set the cache flush bit")
		}
	}
	// An mDNS client asking for a direct answer gets one.
	_, unicast = answer(question(t, 0, "_wittymedia._tcp.local.", dnsmessage.TypePTR, dnsmessage.ClassINET|unicastBit), mdnsPort, testInfo, testIPs)
	if !unicast {
		t.Error("QU question should be answered directly")
	}
	// Following the trail: SRV, TXT and A by name, in any case.
	for _, c := range []struct {
		name string
		typ  dnsmessage.Type
		n    int
	}{
		{"Home Server (0123)._wittymedia._tcp.local.", dnsmessage.TypeSRV, 1},
		{"HOME SERVER (0123)._WITTYMEDIA._TCP.LOCAL.", dnsmessage.TypeTXT, 1},
		{"wms-01234567.local.", dnsmessage.TypeA, 2},
		{"_services._dns-sd._udp.local.", dnsmessage.TypePTR, 1},
	} {
		b, _ := answer(question(t, 0, c.name, c.typ, dnsmessage.ClassINET), mdnsPort, testInfo, testIPs)
		if b == nil {
			t.Errorf("%s: no answer", c.name)
			continue
		}
		if got := len(parse(t, b).Answers); got != c.n {
			t.Errorf("%s: %d answers, want %d", c.name, got, c.n)
		}
	}
}

func TestAnnouncementAndGoodbye(t *testing.T) {
	b, err := announcement(testInfo, testIPs, false)
	if err != nil {
		t.Fatal(err)
	}
	if m := parse(t, b); len(m.Answers) != 5 || m.Answers[0].Header.TTL == 0 {
		t.Errorf("announcement: %+v", m.Answers)
	}
	b, err = announcement(testInfo, testIPs, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range parse(t, b).Answers {
		if r.Header.TTL != 0 {
			t.Errorf("goodbye with a lifetime: %+v", r.Header)
		}
	}
}

func TestStrangeNamesStillWork(t *testing.T) {
	for _, name := range []string{"", "...", "Björn's", strings.Repeat("é", 100), "a\x00b\nc", strings.Repeat("x", 300)} {
		i := testInfo
		i.Name = name
		n, err := namesFor(i)
		if err != nil {
			t.Errorf("%q: %v", name, err)
			continue
		}
		label := instanceLabel(i)
		if len(label) > 63 || strings.Contains(label, ".") {
			t.Errorf("%q gave label %q", name, label)
		}
		b, err := announcement(i, testIPs, false)
		if err != nil || b == nil {
			t.Errorf("%q: no announcement: %v", name, err)
			continue
		}
		parse(t, b)
		_ = n
	}
}

func TestUDPQuestion(t *testing.T) {
	home := net.IPv4(192, 168, 1, 9)
	if udpReply([]byte(UDPQuestion), home, testInfo) == nil {
		t.Error("no answer to the question")
	}
	if udpReply([]byte(UDPQuestion), net.IPv4(8, 8, 8, 8), testInfo) != nil {
		t.Error("answered the internet")
	}
	for _, q := range []string{"", "WMS-DISCOVER/2", UDPQuestion + "x", "GET / HTTP/1.1"} {
		if udpReply([]byte(q), home, testInfo) != nil {
			t.Errorf("answered %q", q)
		}
	}
}

func TestReachable(t *testing.T) {
	for addr, want := range map[string]bool{":9797": true, "0.0.0.0:9797": true, "[::]:9797": true, "192.168.1.2:80": true, "127.0.0.1:9797": false, "[::1]:9797": false, "localhost:9797": false} {
		if got, _ := Reachable(addr); got != want {
			t.Errorf("%s: %v", addr, got)
		}
	}
	if _, ip := Reachable("192.168.1.2:80"); ip == nil || ip.String() != "192.168.1.2" {
		t.Errorf("bound address: %v", ip)
	}
}

func FuzzAnswer(f *testing.F) {
	q, _ := (&dnsmessage.Message{Questions: []dnsmessage.Question{{Name: dnsmessage.MustNewName("_wittymedia._tcp.local."), Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET}}}).Pack()
	f.Add(q, 5353)
	f.Add([]byte{0, 0, 0, 0, 0, 1}, 1234)
	f.Fuzz(func(t *testing.T, b []byte, port int) { answer(b, port, testInfo, testIPs) })
}

// On a real network, only when asked for (WMS_TEST_LAN=1): opening these ports can
// make a desktop firewall ask its owner about the test program. A question by mDNS
// and one by UDP both find the server, and it can be switched off.
func TestOnARealNetwork(t *testing.T) {
	if os.Getenv("WMS_TEST_LAN") != "1" {
		t.Skip("set WMS_TEST_LAN=1 to try this on the network")
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	info := testInfo
	info.Name = "Discovery test"
	var on atomic.Bool
	on.Store(true)
	s := &Service{Info: func() Info { return info }, Enabled: on.Load, Log: log}
	nets := s.interfaces()
	if len(nets) == 0 {
		t.Skip("no network here that multicasts")
	}
	if c, err := openMDNS(nets[0]); err != nil {
		t.Skipf("can't use mDNS here: %v", err)
	} else {
		c.c.Close()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	ask := func(addr *net.UDPAddr, msg []byte) []byte {
		c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: nets[0].addrs[0]})
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		for i := 0; i < 8; i++ {
			_, _ = c.WriteToUDP(msg, addr)
			_ = c.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
			buf := make([]byte, 2000)
			n, _, err := c.ReadFromUDP(buf)
			if err == nil {
				return buf[:n]
			}
		}
		return nil
	}
	b := ask(&net.UDPAddr{IP: mdnsGroup, Port: mdnsPort}, question(t, 5, "_wittymedia._tcp.local.", dnsmessage.TypePTR, dnsmessage.ClassINET))
	if b == nil {
		t.Skip("no mDNS answer on this machine's network (a firewall, perhaps)")
	}
	if m := parse(t, b); !strings.Contains(m.Answers[0].Body.(*dnsmessage.PTRResource).PTR.String(), "Discovery test") {
		t.Errorf("answer: %+v", m.Answers)
	}
	reply := ask(&net.UDPAddr{IP: nets[0].addrs[0], Port: UDPPort}, []byte(UDPQuestion))
	if reply == nil {
		t.Log("no UDP answer: something else holds the port")
	} else if !strings.Contains(string(reply), info.ID) {
		t.Errorf("UDP reply: %s", reply)
	}
	on.Store(false)
	s.Refresh()
	time.Sleep(600 * time.Millisecond)
	if b := ask(&net.UDPAddr{IP: mdnsGroup, Port: mdnsPort}, question(t, 6, "_wittymedia._tcp.local.", dnsmessage.TypePTR, dnsmessage.ClassINET)); b != nil {
		// Another copy of the server on this network may answer; ours has this name.
		if strings.Contains(string(b), "Discovery test") {
			t.Error("still answering after being switched off")
		}
	}
}

// Programs on the server's own machine hear it: the system's mDNS responder there,
// and so apps and simulators beside the server. A neighbour on this machine, in the
// group on the mDNS port as a system responder is, hears the announcement and the
// answer to a question it asks. Only with WMS_TEST_LAN=1, as above.
func TestHeardOnItsOwnMachine(t *testing.T) {
	if os.Getenv("WMS_TEST_LAN") != "1" {
		t.Skip("set WMS_TEST_LAN=1 to try this on the network")
	}
	nets := (&Service{}).interfaces()
	if len(nets) == 0 {
		t.Skip("no network here that multicasts")
	}
	n := nets[0]
	group := &net.UDPAddr{IP: mdnsGroup, Port: mdnsPort}
	l, err := net.ListenMulticastUDP("udp4", &n.ifi, group)
	if err != nil {
		t.Skipf("can't use mDNS here: %v", err)
	}
	defer l.Close()
	// Loopback on, as system responders have it: on Windows that's what lets this
	// socket hear its own machine at all.
	lp := ipv4.NewPacketConn(l)
	_ = lp.SetMulticastInterface(&n.ifi)
	if err := lp.SetMulticastLoopback(true); err != nil {
		t.Skipf("no multicast loopback here: %v", err)
	}
	// A server of its own, so another one on this network can't answer for it.
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	info := testInfo
	info.ID, info.Name = hex.EncodeToString(id), "Loopback test"
	nm, err := namesFor(info)
	if err != nil {
		t.Fatal(err)
	}
	has := func(rs []dnsmessage.Resource, typ dnsmessage.Type) bool {
		for _, r := range rs {
			if r.Header.Type == typ && sameName(r.Header.Name, nm.instance) {
				return true
			}
		}
		return false
	}
	hear := func(ok func(dnsmessage.Message) bool) bool {
		buf := make([]byte, 9000)
		_ = l.SetReadDeadline(time.Now().Add(3 * time.Second))
		for {
			k, _, err := l.ReadFromUDP(buf)
			if err != nil {
				return false
			}
			var m dnsmessage.Message
			if m.Unpack(buf[:k]) == nil && ok(m) {
				return true
			}
		}
	}
	// First, that this machine loops multicast back at all: a question nobody answers.
	check := "loopback-check-" + info.ID + ".local."
	if _, err := l.WriteToUDP(question(t, 0, check, dnsmessage.TypeA, dnsmessage.ClassINET), group); err != nil {
		t.Skipf("can't multicast here: %v", err)
	}
	if !hear(func(m dnsmessage.Message) bool {
		return !m.Response && len(m.Questions) == 1 && strings.EqualFold(m.Questions[0].Name.String(), check)
	}) {
		t.Skip("this machine doesn't loop multicast back to itself")
	}

	c, err := openMDNS(n)
	if err != nil {
		t.Skipf("can't use mDNS here: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = c.serve(ctx, info, slog.New(slog.NewTextHandler(io.Discard, nil)), 0, false); close(done) }()
	defer func() { c.goodbye(info); cancel(); <-done }()

	if !hear(func(m dnsmessage.Message) bool {
		return m.Response && len(m.Answers) > 1 && has(m.Answers, dnsmessage.TypeSRV)
	}) {
		t.Fatal("this machine didn't hear the announcement")
	}
	// The answer has the TXT record alone, where an announcement has them all.
	if _, err := l.WriteToUDP(question(t, 0, nm.instance.String(), dnsmessage.TypeTXT, dnsmessage.ClassINET), group); err != nil {
		t.Fatal(err)
	}
	if !hear(func(m dnsmessage.Message) bool {
		return m.Response && len(m.Answers) == 1 && has(m.Answers, dnsmessage.TypeTXT)
	}) {
		t.Fatal("a question from this machine got no answer it could hear")
	}
}

// Told to listen on one address, the question port doesn't answer on the machine's other networks.
func TestUDPListensWhereTheServerDoes(t *testing.T) {
	if a := (&Service{}).udpAddr(); a == nil || a.IP != nil || a.Port != UDPPort {
		t.Errorf("no address given: %+v", a)
	}
	bound := net.IPv4(192, 168, 1, 5)
	if a := (&Service{Bind: bound}).udpAddr(); a == nil || !a.IP.Equal(bound) || a.Port != UDPPort {
		t.Errorf("one address given: %+v", a)
	}
	if a := (&Service{Bind: net.ParseIP("fd00::5")}).udpAddr(); a != nil {
		t.Errorf("this socket is IPv4 only, so an IPv6 address means no answers: %+v", a)
	}
}

// The tray app asks the server on this computer where it is, whatever port that is.
func TestAskLocal(t *testing.T) {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	info := testInfo
	info.Port = 8123
	go serveUDP(ctx, c, info)
	addr := c.LocalAddr().(*net.UDPAddr)
	if port, _ := ask(addr, 3*time.Second); port != 8123 {
		t.Errorf("port %d", port)
	}
	cancel()
	time.Sleep(50 * time.Millisecond)
	if port, _ := ask(addr, 300*time.Millisecond); port != 0 {
		t.Errorf("nobody's there, but got port %d", port)
	}
}

// ------------------------------------------------- a socket that hears nothing

// testConn is a socket on this machine's loopback address standing in for one
// network's: its "multicast" goes to to, or back to itself when to is nil, as
// with loopback on.
func testConn(t *testing.T, to *net.UDPAddr) *mdnsConn {
	t.Helper()
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Skipf("no loopback address here: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	if to == nil {
		to = c.LocalAddr().(*net.UDPAddr)
	}
	_, lo, _ := net.ParseCIDR("127.0.0.0/8")
	n := localNet{ifi: net.Interface{Name: "test"}, nets: []*net.IPNet{lo}, addrs: []net.IP{net.IPv4(127, 0, 0, 1)}}
	return &mdnsConn{n: n, c: c, group: to}
}

// sink is somewhere to send that never answers (a closed port can, on Windows).
func sink(t *testing.T) *net.UDPAddr {
	t.Helper()
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Skipf("no loopback address here: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c.LocalAddr().(*net.UDPAddr)
}

type lockedBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func TestReopenWait(t *testing.T) {
	var got []string
	for i := 0; i < 7; i++ {
		got = append(got, reopenWait(i).String())
	}
	if s := strings.Join(got, " "); s != "5s 10s 20s 40s 1m0s 1m0s 1m0s" {
		t.Errorf("waits: %s", s)
	}
}

// A socket that hears nothing gives up only when told to; one that hears its
// own announcement never does.
func TestDeafSocket(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	serve := func(c *mdnsConn, deaf time.Duration) (context.CancelFunc, chan error) {
		ctx, cancel := context.WithCancel(context.Background())
		res := make(chan error, 1)
		go func() { res <- c.serve(ctx, testInfo, log, deaf, false) }()
		return cancel, res
	}
	cancel, res := serve(testConn(t, sink(t)), 200*time.Millisecond)
	select {
	case err := <-res:
		if !errors.Is(err, errDeaf) {
			t.Errorf("deaf socket: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("a deaf socket didn't give up")
	}
	cancel()
	for name, c := range map[string]struct {
		conn *mdnsConn
		deaf time.Duration
	}{
		"hearing itself":         {testConn(t, nil), 200 * time.Millisecond},
		"deaf, but told to wait": {testConn(t, sink(t)), 0},
	} {
		cancel, res := serve(c.conn, c.deaf)
		select {
		case err := <-res:
			t.Errorf("%s: gave up: %v", name, err)
		case <-time.After(800 * time.Millisecond):
		}
		cancel()
		if err := <-res; !errors.Is(err, context.Canceled) {
			t.Errorf("%s: ended with %v", name, err)
		}
	}
}

// keep opens a deaf network's socket again, less and less often, until one hears.
func TestKeepReopensUntilHeard(t *testing.T) {
	var logs lockedBuf
	log := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	first, again, hearing := testConn(t, sink(t)), testConn(t, sink(t)), testConn(t, nil)
	var opens atomic.Int32
	open := func(localNet) (*mdnsConn, error) {
		switch opens.Add(1) {
		case 1:
			return nil, errors.New("not yet")
		case 2:
			return again, nil
		}
		return hearing, nil
	}
	var mu sync.Mutex
	var waits []string
	after := func(d time.Duration) <-chan time.Time {
		mu.Lock()
		waits = append(waits, d.String())
		mu.Unlock()
		ch := make(chan time.Time, 1)
		ch <- time.Time{}
		return ch
	}
	slot := &netSlot{n: first.n, c: first}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { keep(ctx, slot, testInfo, log, 300*time.Millisecond, open, after); close(done) }()
	for end := time.Now().Add(10 * time.Second); slot.conn() != hearing; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatalf("never reached a socket that hears; opened %d", opens.Load())
		}
	}
	time.Sleep(time.Second) // it stays with the one that hears
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if n := opens.Load(); n != 3 {
		t.Errorf("opened %d times, want 3", n)
	}
	if s := strings.Join(waits, " "); s != "5s 10s 20s" {
		t.Errorf("waited %s", s)
	}
	out := logs.String()
	if n := strings.Count(out, "can't hear the local network"); n != 1 {
		t.Errorf("warned %d times:\n%s", n, out)
	}
	if !strings.Contains(out, "hearing the local network again") {
		t.Errorf("no word of hearing it again:\n%s", out)
	}
}

// Where deafAfter is off, a quiet network's socket is left alone.
func TestQuietIsFineElsewhere(t *testing.T) {
	if runtime.GOOS != "darwin" && deafAfter != 0 {
		t.Errorf("deafAfter is %v on %s", deafAfter, runtime.GOOS)
	}
	c := testConn(t, sink(t))
	var opens atomic.Int32
	open := func(localNet) (*mdnsConn, error) { opens.Add(1); return nil, errors.New("no") }
	after := func(time.Duration) <-chan time.Time { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		keep(ctx, &netSlot{n: c.n, c: c}, testInfo, slog.New(slog.NewTextHandler(io.Discard, nil)), 0, open, after)
		close(done)
	}()
	select {
	case <-done:
		t.Error("gave up on a quiet network")
	case <-time.After(800 * time.Millisecond):
	}
	cancel()
	<-done
	if opens.Load() != 0 {
		t.Error("opened it again")
	}
}
