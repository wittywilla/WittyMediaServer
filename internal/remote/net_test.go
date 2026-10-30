package remote

import (
	"crypto/tls"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func nets(t *testing.T, s string) []netip.Prefix {
	t.Helper()
	p, err := ParseNets(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func noV6() []netip.Prefix { return nil }

func TestClassify(t *testing.T) {
	cases := []struct {
		ip   string
		want Where
	}{
		{"127.0.0.1", Machine}, {"::1", Machine}, {"::ffff:127.0.0.1", Machine},
		{"192.168.1.20", Home}, {"10.1.2.3", Home}, {"172.16.0.9", Home}, {"fe80::1%eth0", Home}, {"fd00::5", Home}, {"::ffff:192.168.1.20", Home},
		{"100.64.0.1", VPN}, {"100.101.102.103", VPN}, {"fd7a:115c:a1e0::1", VPN},
		{"8.8.8.8", Internet}, {"2001:db8::1", Internet}, {"172.32.0.1", Internet}, {"100.128.0.1", Internet},
	}
	p := &Policy{LocalV6: noV6}
	for _, c := range cases {
		if got := p.Classify(netip.MustParseAddr(c.ip)); got != c.want {
			t.Errorf("%s: got %s, want %s", c.ip, got, c.want)
		}
	}
	if got := p.Classify(netip.Addr{}); got != Internet {
		t.Errorf("an unknown address must count as the internet, got %s", got)
	}
}

func TestClassifyOwnerLists(t *testing.T) {
	p := &Policy{Home: nets(t, "203.0.113.0/24"), Away: nets(t, "10.8.0.0/24"), LocalV6: noV6}
	if got := p.Classify(netip.MustParseAddr("203.0.113.9")); got != Home {
		t.Errorf("owner's home network: got %s", got)
	}
	if got := p.Classify(netip.MustParseAddr("10.8.0.2")); got != VPN {
		t.Errorf("a VPN of the owner's own must count as away: got %s", got)
	}
	if got := p.Classify(netip.MustParseAddr("10.9.0.2")); got != Home {
		t.Errorf("other private addresses are unaffected: got %s", got)
	}
}

func TestClassifyHomeIPv6(t *testing.T) {
	p := &Policy{LocalV6: func() []netip.Prefix { return nets(t, "2a02:1234:5678:1::/64") }}
	if got := p.Classify(netip.MustParseAddr("2a02:1234:5678:1::abcd")); got != Home {
		t.Errorf("same /64: got %s", got)
	}
	if got := p.Classify(netip.MustParseAddr("2a02:1234:5678:2::abcd")); got != Internet {
		t.Errorf("another /64: got %s", got)
	}
}

func TestHomeSubnetsOnlyWithPrivateIPv4(t *testing.T) {
	home := []Iface{
		{"eth0", netip.MustParsePrefix("192.168.1.5/24")},
		{"eth0", netip.MustParsePrefix("2a02:1234::5/64")},
		{"eth0", netip.MustParsePrefix("fe80::5/64")},
	}
	if got := homeSubnetsOf(home); len(got) != 1 || got[0].String() != "2a02:1234::/64" {
		t.Errorf("home machine: %v", got)
	}
	rented := []Iface{
		{"eth0", netip.MustParsePrefix("203.0.113.5/24")},
		{"eth0", netip.MustParsePrefix("2a02:1234::5/64")},
	}
	if got := homeSubnetsOf(rented); len(got) != 0 {
		t.Errorf("a rented server's neighbours must not count as home: %v", got)
	}
}

func TestResolveForwardedHeaders(t *testing.T) {
	proxy := &Policy{Trusted: nets(t, "10.0.0.2, 127.0.0.1"), LocalV6: noV6}
	cases := []struct {
		name       string
		p          *Policy
		remote     string
		xff        []string
		proto      string
		wantIP     string
		wantWhere  Where
		proxied    bool
		unverified bool
		secure     bool
	}{
		{name: "no proxy, no headers", remote: "192.168.1.9:5", wantIP: "192.168.1.9", wantWhere: Home},
		{name: "nothing is trusted by default: spoofed home address", remote: "8.8.8.8:5", xff: []string{"192.168.1.9"}, proto: "https",
			wantIP: "8.8.8.8", wantWhere: Internet, unverified: true},
		{name: "nothing is trusted by default: loopback peer", remote: "127.0.0.1:5", xff: []string{"192.168.1.9"},
			wantIP: "127.0.0.1", wantWhere: Internet, unverified: true},
		{name: "attacker not in the list spoofs", p: proxy, remote: "8.8.4.4:5", xff: []string{"127.0.0.1"},
			wantIP: "8.8.4.4", wantWhere: Internet, unverified: true},
		{name: "trusted proxy names the client", p: proxy, remote: "10.0.0.2:5", xff: []string{"8.8.8.8"}, proto: "https",
			wantIP: "8.8.8.8", wantWhere: Internet, proxied: true, secure: true},
		{name: "client's made-up entries on the left are ignored", p: proxy, remote: "10.0.0.2:5", xff: []string{"192.168.1.9, 127.0.0.1, 8.8.8.8"},
			wantIP: "8.8.8.8", wantWhere: Internet, proxied: true},
		{name: "two proxies in a row", p: proxy, remote: "127.0.0.1:5", xff: []string{"8.8.8.8, 10.0.0.2"},
			wantIP: "8.8.8.8", wantWhere: Internet, proxied: true},
		{name: "several header lines read as one list", p: proxy, remote: "10.0.0.2:5", xff: []string{"192.168.1.9", "8.8.8.8"},
			wantIP: "8.8.8.8", wantWhere: Internet, proxied: true},
		{name: "a person at home through the proxy", p: proxy, remote: "10.0.0.2:5", xff: []string{"192.168.1.9"},
			wantIP: "192.168.1.9", wantWhere: Home, proxied: true},
		{name: "broken entry stops the walk", p: proxy, remote: "10.0.0.2:5", xff: []string{"127.0.0.1, nonsense"},
			wantIP: "10.0.0.2", wantWhere: Home, proxied: true},
		{name: "last proto wins", p: proxy, remote: "10.0.0.2:5", xff: []string{"8.8.8.8"}, proto: "http, https",
			wantIP: "8.8.8.8", wantWhere: Internet, proxied: true, secure: true},
		{name: "the proxy's own machine without headers", p: proxy, remote: "127.0.0.1:5", wantIP: "127.0.0.1", wantWhere: Machine},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = c.remote
			for _, v := range c.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if c.proto != "" {
				r.Header.Set("X-Forwarded-Proto", c.proto)
			}
			p := c.p
			if p == nil {
				p = &Policy{LocalV6: noV6}
			}
			got := p.Resolve(r)
			if got.IP.String() != c.wantIP || got.Where != c.wantWhere || got.Proxied != c.proxied || got.Unverified != c.unverified || got.Secure != c.secure {
				t.Errorf("got ip=%s where=%s proxied=%v unverified=%v secure=%v", got.IP, got.Where, got.Proxied, got.Unverified, got.Secure)
			}
		})
	}
}

func TestResolveNilPolicyTrustsNobody(t *testing.T) {
	var p *Policy
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:5"
	r.Header.Set("X-Forwarded-For", "192.168.1.9")
	if got := p.Resolve(r); got.Where != Internet || !got.Unverified {
		t.Errorf("got %+v", got)
	}
	r.Header.Del("X-Forwarded-For")
	if got := p.Resolve(r); got.Where != Machine || !got.Direct() {
		t.Errorf("got %+v", got)
	}
}

// The Forwarded header is never believed, even from a trusted proxy: such a
// request must not look like it comes from the proxy's own machine.
func TestResolveIgnoresForwardedHeader(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:5"
	r.Header.Set("Forwarded", "for=192.168.1.9;proto=https")
	for _, p := range []*Policy{{LocalV6: noV6}, {Trusted: nets(t, "127.0.0.1"), LocalV6: noV6}} {
		got := p.Resolve(r)
		if got.Where != Internet || !got.Unverified || got.Secure {
			t.Errorf("trusted=%v: got %+v", p.Trusted, got)
		}
	}
}

func TestSecureFlags(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "8.8.8.8:5"
	r.Header.Set("X-Forwarded-Proto", "https")
	var p *Policy
	if p.Resolve(r).Secure {
		t.Error("an untrusted header must not count as encrypted")
	}
	if !SecureHint(r) {
		t.Error("the hint follows the header")
	}
	r.Header.Del("X-Forwarded-Proto")
	if SecureHint(r) {
		t.Error("no hint without a header")
	}
	r.TLS = &tls.ConnectionState{}
	if !p.Resolve(r).Secure || !SecureHint(r) {
		t.Error("direct TLS is encrypted")
	}
}

func TestParseNets(t *testing.T) {
	got, err := ParseNets(" 10.0.0.2, 192.168.1.0/24\n2001:db8::/48;::ffff:172.16.0.1 ")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"10.0.0.2/32", "192.168.1.0/24", "2001:db8::/48", "172.16.0.1/32"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i, w := range want {
		if got[i].String() != w {
			t.Errorf("%d: got %s, want %s", i, got[i], w)
		}
	}
	for _, bad := range []string{"0.0.0.0/0", "::/0", "1.0.0.0/7", "2000::/16", "example.com", "10.0.0.1/99", "::ffff:0.0.0.0/96"} {
		if _, err := ParseNets(bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
	if got, err := ParseNets(""); err != nil || len(got) != 0 {
		t.Errorf("empty: %v %v", got, err)
	}
}

func TestLimitKey(t *testing.T) {
	a := LimitKey(netip.MustParseAddr("2001:db8:1:2:aaaa::1"))
	b := LimitKey(netip.MustParseAddr("2001:db8:1:2:bbbb::9"))
	if a != b {
		t.Errorf("one /64 is one client: %s, %s", a, b)
	}
	if LimitKey(netip.MustParseAddr("2001:db8:1:3::1")) == a {
		t.Error("another /64 is another client")
	}
	if LimitKey(netip.MustParseAddr("192.0.2.1")) == LimitKey(netip.MustParseAddr("192.0.2.2")) {
		t.Error("IPv4 addresses count one by one")
	}
	if LimitKey(netip.MustParseAddr("::ffff:192.0.2.1")) != "192.0.2.1" {
		t.Error("an IPv4 address in IPv6 form is the same client")
	}
}

func TestFindVPNs(t *testing.T) {
	got := FindVPNs([]Iface{
		{"Tailscale", netip.MustParsePrefix("100.100.1.2/32")},
		{"wt0", netip.MustParsePrefix("100.90.1.2/16")},
		{"utun4", netip.MustParsePrefix("100.70.1.2/32")},
		{"Ethernet", netip.MustParsePrefix("100.64.8.8/24")}, // a carrier's address on a modem
		{"Wi-Fi", netip.MustParsePrefix("192.168.1.5/24")},
	})
	if len(got) != 3 || got[0].Kind != "Tailscale" || got[1].Kind != "NetBird" || got[2].Kind != "VPN" {
		t.Errorf("got %+v", got)
	}
}
