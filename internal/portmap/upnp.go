package portmap

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// upnpClient talks to one router's WANIPConnection or WANPPPConnection service.
type upnpClient struct {
	control string // the service's control address
	service string // its full type
	localIP string // this machine, as the router sees it
	http    *http.Client
}

func (c *upnpClient) Protocol() string { return "UPnP" }

// A router's answers are small; anything else is not worth reading.
const upnpMaxBody = 256 << 10

func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// soap calls one action and returns the response's XML. A router's refusal
// comes back as its UPnP error code.
func (c *upnpClient) soap(ctx context.Context, action, body string) ([]byte, error) {
	env := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>` +
		`<u:` + action + ` xmlns:u="` + c.service + `">` + body + `</u:` + action + `></s:Body></s:Envelope>`
	req, err := http.NewRequestWithContext(ctx, "POST", c.control, strings.NewReader(env))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"`+c.service+`#`+action+`"`)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, upnpMaxBody))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		if m := upnpErrRe.FindSubmatch(b); m != nil {
			code, _ := strconv.Atoi(string(m[1]))
			return nil, &upnpError{code}
		}
		return nil, fmt.Errorf("the router answered %d", res.StatusCode)
	}
	return b, nil
}

var (
	upnpErrRe = regexp.MustCompile(`errorCode>\s*(\d+)\s*<`)
	extIPRe   = regexp.MustCompile(`NewExternalIPAddress>\s*([^<\s]+)\s*<`)
)

type upnpError struct{ code int }

func (e *upnpError) Error() string {
	switch e.code {
	case 718:
		return "another device on your network already uses that outside port"
	case 725:
		return "the router only allows permanent mappings"
	case 606:
		return "the router's settings don't allow this; look for a UPnP setting there"
	}
	return "the router refused (UPnP error " + strconv.Itoa(e.code) + ")"
}

func (c *upnpClient) ExternalIP(ctx context.Context) (netip.Addr, error) {
	b, err := c.soap(ctx, "GetExternalIPAddress", "")
	if err != nil {
		return netip.Addr{}, err
	}
	m := extIPRe.FindSubmatch(b)
	if m == nil {
		return netip.Addr{}, errors.New("the router did not say its outside address")
	}
	return netip.ParseAddr(string(m[1]))
}

func (c *upnpClient) add(ctx context.Context, external, internal, lease int) error {
	var b strings.Builder
	b.WriteString(`<NewRemoteHost></NewRemoteHost><NewExternalPort>` + strconv.Itoa(external) + `</NewExternalPort><NewProtocol>TCP</NewProtocol>`)
	b.WriteString(`<NewInternalPort>` + strconv.Itoa(internal) + `</NewInternalPort><NewInternalClient>`)
	_ = xml.EscapeText(&b, []byte(c.localIP))
	b.WriteString(`</NewInternalClient><NewEnabled>1</NewEnabled><NewPortMappingDescription>` + description + `</NewPortMappingDescription>`)
	b.WriteString(`<NewLeaseDuration>` + strconv.Itoa(lease) + `</NewLeaseDuration>`)
	_, err := c.soap(ctx, "AddPortMapping", b.String())
	return err
}

func (c *upnpClient) Map(ctx context.Context, external, internal int) (int, time.Duration, error) {
	err := c.add(ctx, external, internal, leaseSeconds)
	var ue *upnpError
	if errors.As(err, &ue) && ue.code == 725 {
		if err = c.add(ctx, external, internal, 0); err == nil {
			return external, 0, nil
		}
	}
	if err != nil {
		return 0, 0, err
	}
	return external, leaseSeconds * time.Second, nil
}

func (c *upnpClient) Unmap(ctx context.Context, external, internal int) error {
	_, err := c.soap(ctx, "DeletePortMapping", `<NewRemoteHost></NewRemoteHost><NewExternalPort>`+strconv.Itoa(external)+`</NewExternalPort><NewProtocol>TCP</NewProtocol>`)
	return err
}

// ---------------------------------------------------------------- finding the router

const ssdpAddr = "239.255.255.250:1900"

var ssdpTargets = []string{
	"urn:schemas-upnp-org:service:WANIPConnection:2",
	"urn:schemas-upnp-org:service:WANIPConnection:1",
	"urn:schemas-upnp-org:service:WANPPPConnection:1",
	"urn:schemas-upnp-org:device:InternetGatewayDevice:1",
}

// discoverUPnP asks the local network for a router (SSDP) from each private
// network this machine is on, and reads the description of the ones that
// answer.
func discoverUPnP(ctx context.Context) (router, error) {
	ips := privateIPv4s()
	if len(ips) == 0 {
		return nil, errNoRouter
	}
	sctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	var wg sync.WaitGroup
	found := make(chan ssdpAnswer, 16)
	for _, ip := range ips {
		wg.Add(1)
		logging.Go("router search", func() {
			defer wg.Done()
			ssdpSearch(sctx, ip, found)
		}, "cat", "remote")
	}
	logging.Go("router search", func() {
		wg.Wait()
		close(found)
	}, "cat", "remote")
	seen := map[string]bool{}
	for a := range found {
		if seen[a.location] {
			continue
		}
		seen[a.location] = true
		c, err := openRouter(ctx, a.location, a.from, a.local)
		if err == nil {
			cancel()
			for range found { // let the searches finish
			}
			return c, nil
		}
	}
	return nil, errNoRouter
}

type ssdpAnswer struct {
	location string
	from     netip.Addr
	local    netip.Addr
}

func ssdpSearch(ctx context.Context, local netip.Addr, out chan<- ssdpAnswer) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: local.AsSlice()})
	if err != nil {
		return
	}
	defer conn.Close()
	dest, _ := net.ResolveUDPAddr("udp4", ssdpAddr)
	for _, t := range ssdpTargets {
		msg := "M-SEARCH * HTTP/1.1\r\nHOST: " + ssdpAddr + "\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: " + t + "\r\n\r\n"
		_, _ = conn.WriteToUDP([]byte(msg), dest)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
	}
	buf := make([]byte, 2048)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		ip, ok := netip.AddrFromSlice(from.IP)
		if !ok {
			continue
		}
		if loc := ssdpLocation(buf[:n]); loc != "" {
			select {
			case out <- ssdpAnswer{location: loc, from: ip.Unmap(), local: local}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// ssdpLocation reads the LOCATION of an SSDP answer.
func ssdpLocation(b []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if ok && strings.EqualFold(strings.TrimSpace(k), "location") {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// checkLocation: anything on the network can answer a search, so the address
// it names must be the same private machine that answered. Otherwise a device
// could send this server off to fetch addresses elsewhere.
func checkLocation(location string, from netip.Addr) (*url.URL, error) {
	u, err := url.Parse(location)
	if err != nil || u.Scheme != "http" {
		return nil, errors.New("not a router address")
	}
	host, err := netip.ParseAddr(u.Hostname())
	if err != nil || host != from || !(host.IsPrivate() || host.IsLoopback()) {
		return nil, errors.New("not a router address")
	}
	return u, nil
}

type upnpService struct {
	Type    string `xml:"serviceType"`
	Control string `xml:"controlURL"`
}

type upnpDevice struct {
	Services []upnpService `xml:"serviceList>service"`
	Devices  []upnpDevice  `xml:"deviceList>device"`
}

type upnpRoot struct {
	Device upnpDevice `xml:"device"`
}

// wanService finds the service that can forward ports, preferring IP over PPP
// (a few routers list both, and only the first really works).
func wanService(d upnpDevice) *upnpService {
	var ppp *upnpService
	var walk func(upnpDevice) *upnpService
	walk = func(d upnpDevice) *upnpService {
		for i, s := range d.Services {
			if strings.Contains(s.Type, ":WANIPConnection:") {
				return &d.Services[i]
			}
			if ppp == nil && strings.Contains(s.Type, ":WANPPPConnection:") {
				ppp = &d.Services[i]
			}
		}
		for _, c := range d.Devices {
			if s := walk(c); s != nil {
				return s
			}
		}
		return nil
	}
	if s := walk(d); s != nil {
		return s
	}
	return ppp
}

// openRouter reads a router's description and finds its forwarding service.
func openRouter(ctx context.Context, location string, from, local netip.Addr) (*upnpClient, error) {
	u, err := checkLocation(location, from)
	if err != nil {
		return nil, err
	}
	hc := &http.Client{Timeout: 4 * time.Second, CheckRedirect: noRedirect}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var root upnpRoot
	if err := xml.NewDecoder(io.LimitReader(res.Body, upnpMaxBody)).Decode(&root); err != nil {
		return nil, err
	}
	svc := wanService(root.Device)
	if svc == nil {
		return nil, errors.New("this device doesn't forward ports")
	}
	ctl, err := u.Parse(strings.TrimSpace(svc.Control))
	if err != nil || ctl.Host != u.Host || ctl.Scheme != "http" {
		return nil, errors.New("the router's description is not usable")
	}
	return &upnpClient{control: ctl.String(), service: strings.TrimSpace(svc.Type), localIP: local.String(), http: hc}, nil
}
