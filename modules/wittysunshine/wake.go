package wittysunshine

import (
	"errors"
	"net"
	"net/netip"
	"strconv"
)

// Waking a sleeping PC: the "magic packet" of Wake-on-LAN. The packet has no answer and
// proves nothing, so anyone on a network can send one; this server only ever sends it for
// a PC an admin registered, to that PC's own MAC address.

// wakePorts are the usual one, and the one a rig's router may have forwarded.
var wakePorts = []int{9, 47009}

// ParseMAC checks a MAC address. All zeroes is what Sunshine says when it won't name it,
// and a group address would wake a whole network.
func ParseMAC(s string) (net.HardwareAddr, error) {
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return nil, errors.New("That doesn't look like a MAC address")
	}
	if hw[0]&1 != 0 {
		return nil, errors.New("That is a group address, not one computer's")
	}
	zero := true
	for _, b := range hw {
		zero = zero && b == 0
	}
	if zero {
		return nil, errors.New("The PC hasn't told us its MAC address")
	}
	return hw, nil
}

// MagicPacket is six bytes of 0xFF and then the MAC address sixteen times.
func MagicPacket(hw net.HardwareAddr) []byte {
	p := make([]byte, 0, 102)
	for range 6 {
		p = append(p, 0xff)
	}
	for range 16 {
		p = append(p, hw...)
	}
	return p
}

// Wake sends the packet to every network this machine is on, and straight to the PC's last
// known address, which is the only way that crosses to another subnet or a VPN.
func Wake(mac, lastAddress string) error {
	hw, err := ParseMAC(mac)
	if err != nil {
		return err
	}
	packet := MagicPacket(hw)
	targets := broadcastTargets()
	if ip, err := netip.ParseAddr(lastAddress); err == nil && Classify(ip) == Private {
		targets = append(targets, ip.String())
	}
	sent := 0
	var last error
	for _, t := range targets {
		for _, port := range wakePorts {
			c, err := net.Dial("udp", net.JoinHostPort(t, strconv.Itoa(port)))
			if err != nil {
				last = err
				continue
			}
			if _, err := c.Write(packet); err != nil {
				last = err
			} else {
				sent++
			}
			c.Close()
		}
	}
	if sent == 0 {
		if last == nil {
			last = errors.New("this machine isn't on any network")
		}
		return last
	}
	return nil
}

// broadcastTargets are the broadcast addresses of this machine's private IPv4 networks.
func broadcastTargets() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return []string{"255.255.255.255"}
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil || !n.IP.IsPrivate() || len(n.Mask) != 4 {
				continue
			}
			ip := n.IP.To4()
			b := make(net.IP, 4)
			for i := range b {
				b[i] = ip[i] | ^n.Mask[i]
			}
			out = append(out, b.String())
		}
	}
	return append(out, "255.255.255.255")
}
