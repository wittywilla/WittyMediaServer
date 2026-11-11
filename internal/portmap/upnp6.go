// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package portmap

import (
	"context"
	"encoding/xml"
	"errors"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// IPv6 needs no translation, but home routers still refuse connections from
// outside to the devices behind them. UPnP's WANIPv6FirewallControl service
// opens that firewall for one address and port: a "pinhole".

// pinholeLease is how long a pinhole is asked for. Routers allow up to a day;
// an hour means one left behind by a crash doesn't stay long.
const pinholeLease = leaseSeconds

var pinholeIDRe = regexp.MustCompile(`UniqueID>\s*(\d+)\s*<`)

// firewallService finds the IPv6 firewall service in a router's description.
func firewallService(d upnpDevice) *upnpService {
	for i, s := range d.Services {
		if strings.Contains(s.Type, ":WANIPv6FirewallControl:") {
			return &d.Services[i]
		}
	}
	for _, c := range d.Devices {
		if s := firewallService(c); s != nil {
			return s
		}
	}
	return nil
}

// firewall is the same router, asked about its IPv6 firewall.
func (c *upnpClient) firewall() *upnpClient {
	if c.fw6 == "" {
		return nil
	}
	return &upnpClient{control: c.fw6, service: c.fw6Service, localIP: c.localIP, http: c.http}
}

// Open opens the router's IPv6 firewall for addr:port, from anywhere.
func (c *upnpClient) Open(ctx context.Context, proto string, addr netip.Addr, port int) (string, time.Duration, error) {
	fw := c.firewall()
	if fw == nil {
		return "", 0, errors.New("the router has no IPv6 firewall control")
	}
	var b strings.Builder
	b.WriteString(`<RemoteHost></RemoteHost><RemotePort>0</RemotePort><InternalClient>`)
	_ = xml.EscapeText(&b, []byte(addr.String()))
	b.WriteString(`</InternalClient><InternalPort>` + strconv.Itoa(port) + `</InternalPort><Protocol>` + strconv.Itoa(int(protoNumber(proto))) + `</Protocol>`)
	b.WriteString(`<LeaseTime>` + strconv.Itoa(pinholeLease) + `</LeaseTime>`)
	res, err := fw.soap(ctx, "AddPinhole", b.String())
	if err != nil {
		return "", 0, err
	}
	m := pinholeIDRe.FindSubmatch(res)
	if m == nil {
		return "", 0, errors.New("the router did not name the opening it made")
	}
	return string(m[1]), pinholeLease * time.Second, nil
}

// Shut closes an opening made by Open.
func (c *upnpClient) Shut(ctx context.Context, _ string, _ netip.Addr, _ int, id string) error {
	fw := c.firewall()
	if fw == nil || id == "" {
		return nil
	}
	if _, err := strconv.Atoi(id); err != nil {
		return errors.New("not an opening this router made")
	}
	_, err := fw.soap(ctx, "DeletePinhole", `<UniqueID>`+id+`</UniqueID>`)
	return err
}
