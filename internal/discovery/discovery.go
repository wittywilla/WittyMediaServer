// Package discovery lets apps find this server on the home network, by mDNS/DNS-SD
// (what Apple's and Android's own browsers use) and, as an extra for clients with
// raw sockets, by a small UDP question. It tells the network a name, a random ID,
// a version, and where to connect: never accounts, paths or the computer's name.
// Being found opens nothing: connecting still takes a security code or a password.
package discovery

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/ipv4"
)

const (
	// ServiceType is the DNS-SD service apps browse for.
	ServiceType = "_wittymedia._tcp"
	// UDPPort is where the UDP question goes, whatever port the server's web
	// address uses (a client can't know that yet).
	UDPPort = 9797
	// UDPQuestion is the whole of the UDP question.
	UDPQuestion = "WMS-DISCOVER/1"

	mdnsPort = 5353
)

var mdnsGroup = net.IPv4(224, 0, 0, 251)

// Info is everything announced.
type Info struct {
	ID      string
	Name    string
	Version string
	API     int
	Port    int
	TLS     bool
}

// Service announces the server while Enabled says so.
type Service struct {
	Info    func() Info
	Enabled func() bool
	// Bind limits it to one address, when the server listens on only that.
	Bind net.IP
	Log  *slog.Logger

	refresh chan struct{}
	once    sync.Once
}

func (s *Service) init() {
	s.once.Do(func() { s.refresh = make(chan struct{}, 1) })
}

// Refresh looks at the settings again: after the owner turned it off or on, or
// renamed the server.
func (s *Service) Refresh() {
	s.init()
	select {
	case s.refresh <- struct{}{}:
	default:
	}
}

// Reachable says whether a listen address can be reached from other devices,
// and the one address it is limited to, if any.
func Reachable(listen string) (bool, net.IP) {
	host, _, err := net.SplitHostPort(listen)
	if err != nil || host == "" || host == "0.0.0.0" || host == "::" {
		return true, nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return !strings.EqualFold(host, "localhost"), nil
	}
	if ip.IsLoopback() {
		return false, nil
	}
	return true, ip
}

// Run announces until ctx ends, and says goodbye. Networks come and go (a laptop,
// a DHCP change), so it looks again every minute.
func (s *Service) Run(ctx context.Context) {
	s.init()
	var cur *running
	stop := func() {
		if cur != nil {
			cur.stop()
			cur = nil
		}
	}
	defer stop()
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		on := s.Enabled == nil || s.Enabled()
		info := s.Info()
		nets := s.interfaces()
		switch {
		case !on:
			stop()
		case cur == nil || cur.info != info || cur.key != nets.key():
			stop()
			cur = s.start(info, nets)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-s.refresh:
		}
	}
}

// localNet is one network to announce on.
type localNet struct {
	ifi   net.Interface
	nets  []*net.IPNet // its private IPv4 networks
	addrs []net.IP
}

type localNets []localNet

func (l localNets) key() string {
	var b strings.Builder
	for _, n := range l {
		b.WriteString(n.ifi.Name)
		for _, a := range n.addrs {
			b.WriteString(" " + a.String())
		}
		b.WriteString(";")
	}
	return b.String()
}

// interfaces lists the networks to announce on: up, able to multicast, with a
// private address. Not this machine's loopback, and not VPN tunnels, where nobody
// expects a server to announce itself.
func (s *Service) interfaces() localNets {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out localNets
	for _, ifi := range ifs {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 || ifi.Flags&net.FlagLoopback != 0 || ifi.Flags&net.FlagPointToPoint != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		n := localNet{ifi: ifi}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil || !ipn.IP.IsPrivate() {
				continue
			}
			if s.Bind != nil && !s.Bind.Equal(ipn.IP) {
				continue
			}
			n.nets = append(n.nets, ipn)
			n.addrs = append(n.addrs, ipn.IP.To4())
		}
		if len(n.addrs) > 0 {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ifi.Name < out[j].ifi.Name })
	return out
}

// running is one round of announcing, ended when anything about it changes.
type running struct {
	info Info
	key  string
	stop func()
}

func (s *Service) start(info Info, nets localNets) *running {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var conns []*mdnsConn
	for _, n := range nets {
		c, err := openMDNS(n)
		if err != nil {
			s.Log.Debug("mdns", "interface", n.ifi.Name, "err", err)
			continue
		}
		conns = append(conns, c)
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.serve(ctx, info, s.Log)
		}()
	}
	if udp, err := net.ListenUDP("udp4", &net.UDPAddr{Port: UDPPort}); err != nil {
		s.Log.Debug("discovery by UDP is off", "err", err) // something else has the port
	} else {
		wg.Add(1)
		go func() {
			defer wg.Done()
			serveUDP(ctx, udp, info)
		}()
	}
	if len(conns) > 0 {
		s.Log.Info("announcing on the local network", "service", ServiceType, "interfaces", len(conns))
	} else if len(nets) > 0 {
		s.Log.Warn("could not announce on the local network; apps can still connect by address")
	}
	return &running{info: info, key: nets.key(), stop: func() {
		for _, c := range conns {
			c.goodbye(info)
		}
		cancel()
		wg.Wait()
	}}
}

// mdnsConn answers mDNS on one network.
type mdnsConn struct {
	n  localNet
	c  *net.UDPConn
	pc *ipv4.PacketConn
}

func openMDNS(n localNet) (*mdnsConn, error) {
	c, err := net.ListenMulticastUDP("udp4", &n.ifi, &net.UDPAddr{IP: mdnsGroup, Port: mdnsPort})
	if err != nil {
		return nil, err
	}
	pc := ipv4.NewPacketConn(c)
	// RFC 6762: multicast records go out with a hop limit of 255, to this
	// network and no further.
	_ = pc.SetMulticastTTL(255)
	_ = pc.SetMulticastInterface(&n.ifi)
	return &mdnsConn{n: n, c: c, pc: pc}, nil
}

func (m *mdnsConn) send(b []byte, to *net.UDPAddr) {
	if to == nil {
		to = &net.UDPAddr{IP: mdnsGroup, Port: mdnsPort}
	}
	_, _ = m.c.WriteToUDP(b, to)
}

// onThisNetwork: mDNS is for the local link only.
func (m *mdnsConn) onThisNetwork(ip net.IP) bool {
	for _, n := range m.n.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func (m *mdnsConn) serve(ctx context.Context, info Info, log *slog.Logger) {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
		case <-done:
		}
		_ = m.c.Close()
	}()
	// Say we are here, twice, a moment apart, as RFC 6762 asks.
	go func() {
		for _, wait := range []time.Duration{0, time.Second} {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			if b, err := announcement(info, m.n.addrs, false); err == nil {
				m.send(b, nil)
			}
		}
	}()
	buf := make([]byte, 9000)
	for {
		n, src, err := m.c.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if !m.onThisNetwork(src.IP) {
			continue
		}
		resp, unicast := answer(buf[:n], src.Port, info, m.n.addrs)
		if resp == nil {
			continue
		}
		if unicast {
			m.send(resp, src)
		} else {
			m.send(resp, nil)
		}
	}
}

func (m *mdnsConn) goodbye(info Info) {
	if b, err := announcement(info, m.n.addrs, true); err == nil {
		m.send(b, nil)
	}
}

// ---------------------------------------------------------------- UDP question

type udpAnswer struct {
	Service string `json:"service"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	API     int    `json:"api"`
	Port    int    `json:"port"`
	TLS     bool   `json:"tls"`
}

// udpReply is the answer to the UDP question, nil for anything else.
func udpReply(question []byte, from net.IP, info Info) []byte {
	if string(question) != UDPQuestion {
		return nil
	}
	if !from.IsPrivate() && !from.IsLoopback() && !from.IsLinkLocalUnicast() {
		return nil // not from the home network
	}
	b, err := json.Marshal(udpAnswer{Service: "wittymediaserver", ID: info.ID, Name: info.Name, Version: info.Version, API: info.API, Port: info.Port, TLS: info.TLS})
	if err != nil {
		return nil
	}
	return b
}

// udpLimit bounds how much this answers: the answer is bigger than the question,
// so it shouldn't be useful for flooding anyone.
const udpLimit = 40 // answers per 10 seconds

func serveUDP(ctx context.Context, c *net.UDPConn, info Info) {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
		case <-done:
		}
		_ = c.Close()
	}()
	buf := make([]byte, 64)
	window, count := time.Now(), 0
	for {
		n, src, err := c.ReadFromUDP(buf)
		if err != nil {
			return
		}
		r := udpReply(buf[:n], src.IP, info)
		if r == nil {
			continue
		}
		if time.Since(window) > 10*time.Second {
			window, count = time.Now(), 0
		}
		if count++; count > udpLimit {
			continue
		}
		_, _ = c.WriteToUDP(r, src)
	}
}
