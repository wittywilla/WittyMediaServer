package discovery

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
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
	// A response is never answered, or two servers would talk forever.
	resp, _ := answer(question(t, 0, "_wittymedia._tcp.local.", dnsmessage.TypePTR, dnsmessage.ClassINET), mdnsPort, testInfo, testIPs)
	if b, _ := answer(resp, mdnsPort, testInfo, testIPs); b != nil {
		t.Error("answered a response")
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
