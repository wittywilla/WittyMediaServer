package gamestream

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymoonlight/gamestream/fake"
	"golang.org/x/net/dns/dnsmessage"
)

func rigFor(t *testing.T, s *fake.Sunshine) *Rig {
	t.Helper()
	id, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return &Rig{Address: "127.0.0.1", Port: s.Port(), Identity: id}
}

// pair runs pairing with a person typing typed into Sunshine.
func pair(t *testing.T, s *fake.Sunshine, r *Rig, pin, typed string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		if s.WaitForPairing(5 * time.Second) {
			s.TypePIN(typed)
		}
	}()
	return r.Pair(ctx, pin)
}

func startFake(t *testing.T) *fake.Sunshine {
	t.Helper()
	s, err := fake.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func paired(t *testing.T) (*fake.Sunshine, *Rig) {
	t.Helper()
	s := startFake(t)
	r := rigFor(t, s)
	cert, err := pair(t, s, r, "1234", "1234")
	if err != nil {
		t.Fatal(err)
	}
	r.Cert = cert
	return s, r
}

func TestPairing(t *testing.T) {
	s := startFake(t)
	r := rigFor(t, s)
	cert, err := pair(t, s, r, "1234", "1234")
	if err != nil {
		t.Fatal(err)
	}
	if !s.IsPaired(r.Identity.CertPEM) {
		t.Error("the rig didn't record this server")
	}
	if CertFingerprint(cert) != CertFingerprint(s.CertPEM()) {
		t.Error("the pinned certificate isn't the rig's")
	}
	r.Cert = cert
	info, err := r.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.MAC != s.MAC || info.Name != s.Name || info.UUID != s.UUID {
		t.Errorf("status: %+v", info)
	}
}

func TestWrongPIN(t *testing.T) {
	s := startFake(t)
	r := rigFor(t, s)
	if _, err := pair(t, s, r, "1234", "9999"); !errors.Is(err, ErrWrongPIN) {
		t.Fatalf("err = %v", err)
	}
	if s.IsPaired(r.Identity.CertPEM) {
		t.Error("a wrong PIN paired")
	}
}

func TestImposterInPairing(t *testing.T) {
	s := startFake(t)
	s.WrongCertInPairing = true
	r := rigFor(t, s)
	if _, err := pair(t, s, r, "1234", "1234"); !errors.Is(err, ErrImposter) {
		t.Fatalf("err = %v", err)
	}
	if s.IsPaired(r.Identity.CertPEM) {
		t.Error("it paired with an imposter")
	}
}

func TestRigThatDoesntKnowPIN(t *testing.T) {
	s := startFake(t)
	s.WrongHash = true
	r := rigFor(t, s)
	if _, err := pair(t, s, r, "1234", "1234"); !errors.Is(err, ErrWrongPIN) {
		t.Fatalf("err = %v", err)
	}
}

func TestPairNobodyTypes(t *testing.T) {
	s := startFake(t)
	s.HoldPairFor = 200 * time.Millisecond
	r := rigFor(t, s)
	if _, err := r.Pair(context.Background(), "1234"); !errors.Is(err, ErrPairTimedOut) {
		t.Fatalf("err = %v", err)
	}
	// And when our own time runs out first.
	s.HoldPairFor = time.Minute
	r = rigFor(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := r.Pair(ctx, "1234"); !errors.Is(err, ErrPairTimedOut) {
		t.Fatalf("err = %v", err)
	}
}

func TestPairOldHost(t *testing.T) {
	s := startFake(t)
	s.Version = "3.20.0.1"
	r := rigFor(t, s)
	if _, err := r.Pair(context.Background(), "1234"); !errors.Is(err, ErrTooOld) {
		t.Fatalf("err = %v", err)
	}
}

func TestBadPINsNeverLeave(t *testing.T) {
	s := startFake(t)
	r := rigFor(t, s)
	for _, pin := range []string{"", "123", "12345", "12a4", "١٢٣٤"} {
		if _, err := r.Pair(context.Background(), pin); !errors.Is(err, ErrWrongPIN) {
			t.Errorf("pin %q: %v", pin, err)
		}
	}
	for range 200 {
		p, err := NewPIN()
		if err != nil || !ValidPIN(p) {
			t.Fatalf("NewPIN: %q %v", p, err)
		}
	}
}

func TestAppsAndArt(t *testing.T) {
	s, r := paired(t)
	s.Apps = append(s.Apps, App2("Tom & <Jerry>"))
	apps, err := r.Apps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 4 || apps[0].Title != "Desktop" || apps[3].Title != "Tom & <Jerry>" {
		t.Fatalf("apps: %+v", apps)
	}
	data, ext, err := r.AppImage(context.Background(), 7)
	if err != nil || ext != ".png" || len(data) == 0 {
		t.Fatalf("art: %d %q %v", len(data), ext, err)
	}
	s.NotImage = true
	if _, _, err := r.AppImage(context.Background(), 7); !errors.Is(err, ErrBadAnswer) {
		t.Errorf("a script was taken for art: %v", err)
	}
	s.NotImage, s.HugeImage = false, true
	if _, _, err := r.AppImage(context.Background(), 7); !errors.Is(err, ErrBadAnswer) {
		t.Errorf("huge art: %v", err)
	}
}

func TestLaunchAndQuit(t *testing.T) {
	s, r := paired(t)
	ctx := context.Background()
	if err := r.StartOnly(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if s.Running() != 7 {
		t.Errorf("running %d", s.Running())
	}
	if err := r.StartOnly(ctx, 7); !errors.Is(err, ErrBusy) {
		t.Errorf("second launch: %v", err)
	}
	info, err := r.Status(ctx)
	if err != nil || info.CurrentGame != 7 {
		t.Errorf("status: %+v %v", info, err)
	}
	if err := r.Quit(ctx); err != nil {
		t.Fatal(err)
	}
	if s.Running() != 0 {
		t.Error("still running after quit")
	}
	var se *StatusError
	if err := r.StartOnly(ctx, 12345); !errors.As(err, &se) || se.Code != 404 {
		t.Errorf("unknown app: %v", err)
	}
}

func TestPinning(t *testing.T) {
	s, r := paired(t)
	// Another PC (or someone on the network) answering at the same address.
	other := startFake(t)
	impostor := &Rig{Address: "127.0.0.1", Port: other.Port(), Identity: r.Identity, Cert: r.Cert}
	if _, err := impostor.Status(context.Background()); !errors.Is(err, ErrCertChanged) {
		t.Errorf("a different certificate was accepted: %v", err)
	}
	if _, err := impostor.Apps(context.Background()); !errors.Is(err, ErrCertChanged) {
		t.Errorf("apps from a different certificate: %v", err)
	}
	if err := impostor.SubmitPIN(context.Background(), "admin", "secret", "4321", ""); !errors.Is(err, ErrCertChanged) {
		t.Errorf("Sunshine's sign-in went to a different certificate: %v", err)
	}
	_ = s
}

func TestUnpairedAndForgotten(t *testing.T) {
	s, r := paired(t)
	s.Forget()
	if _, err := r.Apps(context.Background()); !errors.Is(err, ErrNotPaired) {
		t.Errorf("after being removed from the rig: %v", err)
	}
	stranger := rigFor(t, s)
	stranger.Cert = s.CertPEM()
	if _, err := stranger.Status(context.Background()); !errors.Is(err, ErrNotPaired) {
		t.Errorf("a stranger: %v", err)
	}
	noCert := rigFor(t, s)
	if _, err := noCert.Apps(context.Background()); !errors.Is(err, ErrNotPaired) {
		t.Errorf("no pin at all: %v", err)
	}
}

func TestFailures(t *testing.T) {
	s := startFake(t)
	r := rigFor(t, s)
	s.Garbage = true
	if _, err := r.Hello(context.Background()); !errors.Is(err, ErrBadAnswer) {
		t.Errorf("garbage: %v", err)
	}
	s.Garbage, s.Huge = false, true
	if _, err := r.Hello(context.Background()); !errors.Is(err, ErrBadAnswer) {
		t.Errorf("endless answer: %v", err)
	}
	s.Close()
	if _, err := r.Hello(context.Background()); !errors.Is(err, ErrRefused) {
		t.Errorf("closed port: %v", err)
	}
}

func TestPolicyStopsConnections(t *testing.T) {
	// A public address is refused while connecting, before anything is sent.
	r := &Rig{Address: "93.184.216.34", Port: 47989}
	if _, err := r.Hello(context.Background()); !errors.Is(err, ErrBlocked) {
		t.Errorf("public: %v", err)
	}
	for _, addr := range []string{"169.254.169.254", "224.0.0.251", "0.0.0.0", "::", "fe80::1"} {
		r := &Rig{Address: addr, Port: 47989, Policy: Policy{AllowPublic: true}}
		if _, err := r.Hello(context.Background()); !errors.Is(err, ErrBlocked) {
			t.Errorf("%s: %v", addr, err)
		}
	}
}

func TestClassify(t *testing.T) {
	want := map[string]Reach{
		"192.168.1.20": Private, "10.0.0.2": Private, "172.16.0.9": Private, "100.100.1.2": Private, "fd00::1": Private,
		"127.0.0.1": Loopback, "::1": Loopback, "::ffff:127.0.0.1": Loopback, "::ffff:192.168.0.2": Private,
		"8.8.8.8": Public, "2606:4700::1": Public, "100.128.0.1": Public,
		"169.254.169.254": Unusable, "fe80::1": Unusable, "224.0.0.1": Unusable, "0.0.0.0": Unusable, "::": Unusable, "255.255.255.255": Unusable, "ff02::1": Unusable,
		"::ffff:169.254.169.254": Unusable,
	}
	for s, r := range want {
		if got := Classify(netip.MustParseAddr(s)); got != r {
			t.Errorf("%s: %d, want %d", s, got, r)
		}
	}
}

func TestCleanAddress(t *testing.T) {
	good := map[string]struct {
		host string
		port int
	}{
		"192.168.1.20": {"192.168.1.20", 0}, " 192.168.1.20:47989 ": {"192.168.1.20", 47989}, "gaming-pc": {"gaming-pc", 0},
		"gaming-pc.local": {"gaming-pc.local", 0}, "[fd00::1]:47989": {"fd00::1", 47989}, "fd00::1": {"fd00::1", 0}, "PC2:1234": {"PC2", 1234},
	}
	for in, w := range good {
		h, p, err := CleanAddress(in)
		if err != nil || h != w.host || p != w.port {
			t.Errorf("%q: %q %d %v", in, h, p, err)
		}
	}
	for _, in := range []string{"", "http://192.168.1.2", "192.168.1.2/admin", "user@pc", "pc?x=1", "pc#a", "a b", "pc:0", "pc:99999", "pc:abc", "-pc", "pc-", "a..b", "fe80::1%eth0",
		"file:///etc/passwd", "127.1", "2130706433", "0x7f.1", "pc\nHost: evil", strings.Repeat("a", 300)} {
		if h, _, err := CleanAddress(in); err == nil {
			t.Errorf("%q was accepted as %q", in, h)
		}
	}
}

func TestCheckAddress(t *testing.T) {
	ctx := context.Background()
	lookup := func(ips ...string) func(context.Context, string) ([]netip.Addr, error) {
		return func(context.Context, string) ([]netip.Addr, error) {
			var out []netip.Addr
			for _, s := range ips {
				out = append(out, netip.MustParseAddr(s))
			}
			return out, nil
		}
	}
	if r, err := CheckAddress(ctx, "pc", lookup("192.168.1.2")); err != nil || r != Private {
		t.Errorf("%d %v", r, err)
	}
	if r, err := CheckAddress(ctx, "pc", lookup("192.168.1.2", "8.8.8.8")); err != nil || r != Public {
		t.Errorf("mixed: %d %v", r, err)
	}
	if _, err := CheckAddress(ctx, "pc", lookup("192.168.1.2", "169.254.169.254")); err == nil {
		t.Error("a name that also points at the metadata address was accepted")
	}
	if _, err := CheckAddress(ctx, "pc", func(context.Context, string) ([]netip.Addr, error) { return nil, errors.New("nope") }); err == nil {
		t.Error("an unknown name was accepted")
	}
	if r, err := CheckAddress(ctx, "8.8.8.8", nil); err != nil || r != Public {
		t.Errorf("%d %v", r, err)
	}
	if _, err := CheckAddress(ctx, "169.254.169.254", nil); err == nil {
		t.Error("the metadata address was accepted")
	}
}

func TestSubmitPIN(t *testing.T) {
	s, r := paired(t)
	ctx := context.Background()
	if err := r.SubmitPIN(ctx, "admin", "secret", "4321", ""); !errors.Is(err, ErrNoPending) {
		t.Errorf("nothing waiting: %v", err)
	}
	s.PendingDevice("Living Room TV")
	if err := r.SubmitPIN(ctx, "admin", "wrong", "4321", ""); !errors.Is(err, ErrWebSignIn) {
		t.Errorf("wrong password: %v", err)
	}
	if err := r.SubmitPIN(ctx, "admin", "secret", "12", ""); err == nil {
		t.Error("a bad PIN was sent")
	}
	if err := r.SubmitPIN(ctx, "admin", "secret", "1111", "Living Room TV"); !errors.Is(err, ErrWrongPIN) {
		t.Errorf("wrong pin: %v", err)
	}
	s.PendingDevice("Living Room TV")
	if err := r.SubmitPIN(ctx, "admin", "secret", "4321", ""); err != nil {
		t.Errorf("good: %v", err)
	}
}

func TestMagicPacket(t *testing.T) {
	hw, err := ParseMAC("aa:bb:cc:dd:ee:02")
	if err != nil {
		t.Fatal(err)
	}
	p := MagicPacket(hw)
	if len(p) != 102 || p[0] != 0xff || p[5] != 0xff || p[6] != 0xaa || p[101] != 0x02 {
		t.Errorf("packet: %x", p)
	}
	for _, bad := range []string{"", "00:00:00:00:00:00", "ff:ff:ff:ff:ff:ff", "01:00:5e:00:00:01", "aa:bb:cc:dd:ee", "aa:bb:cc:dd:ee:ff:00:11", "hello"} {
		if _, err := ParseMAC(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if err := Wake("00:00:00:00:00:00", ""); err == nil {
		t.Error("woke nothing without saying so")
	}
}

func TestWakeSends(t *testing.T) {
	// Straight to the last known address, on a port the test can listen on.
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	old := wakePorts
	wakePorts = []int{c.LocalAddr().(*net.UDPAddr).Port}
	defer func() { wakePorts = old }()
	// Loopback isn't "private", so this goes through the same dial with 127.0.0.1 directly.
	hw, _ := ParseMAC("aa:bb:cc:dd:ee:02")
	d, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(wakePorts[0])))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Write(MagicPacket(hw)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 200)
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if n, _, err := c.ReadFromUDP(buf); err != nil || n != 102 {
		t.Errorf("%d %v", n, err)
	}
}

func TestDiscoveryParsing(t *testing.T) {
	svc := dnsmessage.MustNewName(serviceName)
	inst := dnsmessage.MustNewName("Living Room." + serviceName)
	target := dnsmessage.MustNewName("rig.local.")
	msg := dnsmessage.Message{
		Header: dnsmessage.Header{Response: true},
		Answers: []dnsmessage.Resource{{
			Header: dnsmessage.ResourceHeader{Name: svc, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET},
			Body:   &dnsmessage.PTRResource{PTR: inst},
		}},
		Additionals: []dnsmessage.Resource{
			{Header: dnsmessage.ResourceHeader{Name: inst, Type: dnsmessage.TypeSRV, Class: dnsmessage.ClassINET}, Body: &dnsmessage.SRVResource{Port: 48000, Target: target}},
			{Header: dnsmessage.ResourceHeader{Name: target, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}, Body: &dnsmessage.AResource{A: [4]byte{192, 168, 1, 20}}},
		},
	}
	b, err := msg.Pack()
	if err != nil {
		t.Fatal(err)
	}
	got := parseAnswer(b, netip.MustParseAddr("192.168.1.99"))
	if len(got) != 1 || got[0].Address != "192.168.1.20" || got[0].Port != 48000 || got[0].Name != "Living Room" {
		t.Fatalf("%+v", got)
	}
	// An answer claiming a public address falls back to who sent it.
	msg.Additionals[1].Body = &dnsmessage.AResource{A: [4]byte{8, 8, 8, 8}}
	b, _ = msg.Pack()
	if got := parseAnswer(b, netip.MustParseAddr("192.168.1.99")); len(got) != 1 || got[0].Address != "192.168.1.99" {
		t.Errorf("%+v", got)
	}
	// From the internet: dropped.
	if got := parseAnswer(b, netip.MustParseAddr("8.8.4.4")); len(got) != 0 {
		t.Errorf("%+v", got)
	}
	// Junk: nothing, and no panic.
	for _, junk := range [][]byte{nil, {1}, []byte("not dns at all, not even close")} {
		if got := parseAnswer(junk, netip.MustParseAddr("192.168.1.99")); len(got) != 0 {
			t.Errorf("%+v", got)
		}
	}
	if q, err := browseQuery(); err != nil || len(q) < 12 {
		t.Errorf("query %x %v", q, err)
	}
}

func FuzzParseAnswer(f *testing.F) {
	f.Add([]byte{0, 0, 0x84, 0, 0, 0, 0, 1, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) { parseAnswer(b, netip.MustParseAddr("192.168.1.99")) })
}

func TestIdentityKeptAndReused(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rigs")
	a, err := LoadOrCreateIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadOrCreateIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID || string(a.CertPEM) != string(b.CertPEM) {
		t.Error("a new identity was made instead of reusing the old one")
	}
	st, err := os.Stat(filepath.Join(dir, "client.key"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o077 != 0 && !onWindows {
		t.Errorf("key mode %v", st.Mode())
	}
	// The whole round: pair with a saved identity, then use it after "restarting".
	s := startFake(t)
	r := &Rig{Address: "127.0.0.1", Port: s.Port(), Identity: a}
	cert, err := pair(t, s, r, "1234", "1234")
	if err != nil {
		t.Fatal(err)
	}
	again := &Rig{Address: "127.0.0.1", Port: s.Port(), Identity: b, Cert: cert}
	if _, err := again.Apps(context.Background()); err != nil {
		t.Errorf("after reloading: %v", err)
	}
}

// App2 makes a fake app with a hard title.
func App2(title string) fake.App { return fake.App{ID: 55, Title: title} }
