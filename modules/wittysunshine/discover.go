package wittysunshine

import (
	"context"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// Finding PCs on the home network: Sunshine announces itself over mDNS as _nvstream._tcp.
// Answers come from anything on the network, so what they say is only a place to ask next:
// nothing is believed until Hello gets a real reply, and only private addresses are kept.

const serviceName = "_nvstream._tcp.local."

// Found is one PC that answered.
type Found struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
}

func browseQuery() ([]byte, error) {
	name, err := dnsmessage.NewName(serviceName)
	if err != nil {
		return nil, err
	}
	msg := dnsmessage.Message{Questions: []dnsmessage.Question{{Name: name, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET}}}
	return msg.Pack()
}

// parseAnswer reads one mDNS reply into what it says about PCs. An address that doesn't
// come with the reply is filled in from who sent it, as replies often leave it out.
func parseAnswer(packet []byte, from netip.Addr) []Found {
	var m dnsmessage.Message
	if err := m.Unpack(packet); err != nil {
		return nil
	}
	all := append(append(append([]dnsmessage.Resource{}, m.Answers...), m.Additionals...), m.Authorities...)
	addrs := map[string][]netip.Addr{}
	srv := map[string]dnsmessage.SRVResource{}
	var instances []string
	for _, r := range all {
		name := strings.ToLower(r.Header.Name.String())
		switch b := r.Body.(type) {
		case *dnsmessage.PTRResource:
			if name == serviceName {
				instances = append(instances, b.PTR.String())
			}
		case *dnsmessage.SRVResource:
			srv[name] = *b
		case *dnsmessage.AResource:
			addrs[name] = append(addrs[name], netip.AddrFrom4(b.A))
		case *dnsmessage.AAAAResource:
			addrs[name] = append(addrs[name], netip.AddrFrom16(b.AAAA))
		}
	}
	var out []Found
	for _, inst := range instances {
		label := inst
		if len(inst) > len(serviceName)+1 {
			label = inst[:len(inst)-len(serviceName)-1]
		}
		f := Found{Name: clip(strings.ReplaceAll(label, `\ `, " "), 128), Port: DefaultPort}
		ip := from
		if s, ok := srv[strings.ToLower(inst)]; ok {
			if s.Port != 0 {
				f.Port = int(s.Port)
			}
			for _, a := range addrs[strings.ToLower(s.Target.String())] {
				if Classify(a) == Private {
					ip = a
					break
				}
			}
		}
		if Classify(ip) != Private {
			continue
		}
		f.Address = ip.Unmap().String()
		out = append(out, f)
	}
	return out
}

// Browse asks the home network who runs Sunshine, and listens for wait. It costs nothing
// until someone presses Find, and it doesn't keep a socket open afterwards.
func Browse(ctx context.Context, wait time.Duration) ([]Found, error) {
	query, err := browseQuery()
	if err != nil {
		return nil, err
	}
	var (
		mu    sync.Mutex
		found = map[string]Found{}
		wg    sync.WaitGroup
	)
	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	for _, local := range localIPv4s() {
		c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: local})
		if err != nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer c.Close()
			stop := context.AfterFunc(ctx, func() { _ = c.SetReadDeadline(time.Now()) })
			defer stop()
			// Twice, as a single multicast packet is easily lost on Wi-Fi.
			_, _ = c.WriteToUDP(query, group)
			again := time.AfterFunc(wait/3, func() { _, _ = c.WriteToUDP(query, group) })
			defer again.Stop()
			buf := make([]byte, 9000)
			for {
				n, from, err := c.ReadFromUDP(buf)
				if err != nil {
					return
				}
				ip, ok := netip.AddrFromSlice(from.IP)
				if !ok {
					continue
				}
				for _, f := range parseAnswer(buf[:n], ip.Unmap()) {
					mu.Lock()
					found[f.Address] = f
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	out := make([]Found, 0, len(found))
	for _, f := range found {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func localIPv4s() []net.IP {
	var out []net.IP
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagMulticast == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && n.IP.IsPrivate() {
				out = append(out, n.IP)
			}
		}
	}
	return out
}
