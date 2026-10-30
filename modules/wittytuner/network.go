package wittytuner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Network tuners: boxes on the home network with an HTTP interface, in the
// style of HDHomeRun devices. The interface is documented by the maker:
// discover.json, lineup.json, lineup_status.json, lineup.post and MPEG-TS over
// HTTP. Devices are found by the UDP discovery protocol on port 65001 and, for
// devices that emulate one, SSDP.

func init() {
	register(Driver{
		ID: "network", Name: "Network tuner (HDHomeRun-style)", Status: "mock",
		Description: "Antenna or cable boxes on your network that stream over HTTP. Usually found by themselves.",
		Scans:       true,
		Fields: []Field{
			{Key: "address", Label: "Address", Kind: "text", Required: true, Help: "The device's address, such as 192.168.1.50."},
			{Key: "source", Label: "Signal", Kind: "select", Options: []string{"Antenna", "Cable"}, Default: "Antenna", Help: "Which signal a channel scan looks at."},
		},
		build: func(c Config, e Env) (Source, error) {
			base, err := deviceBase(c.Options["address"])
			if err != nil {
				return nil, err
			}
			return &netTuner{cfg: c, env: e, base: base}, nil
		},
	})
}

// deviceBase turns what someone typed into http://host[:port].
func deviceBase(addr string) (string, error) {
	addr = strings.TrimSuffix(strings.TrimSpace(addr), "/")
	if addr == "" {
		return "", errors.New("fill in the device's address")
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("that is not a device address")
	}
	return u.Scheme + "://" + u.Host, nil
}

type netTuner struct {
	cfg  Config
	env  Env
	base string
	dev  discoverJSON
}

type discoverJSON struct {
	FriendlyName string `json:"FriendlyName"`
	ModelNumber  string `json:"ModelNumber"`
	DeviceID     string `json:"DeviceID"`
	BaseURL      string `json:"BaseURL"`
	LineupURL    string `json:"LineupURL"`
	TunerCount   int    `json:"TunerCount"`
	Firmware     string `json:"FirmwareVersion"`
	Legacy       int    `json:"Legacy"`
}

type lineupEntry struct {
	GuideNumber string `json:"GuideNumber"`
	GuideName   string `json:"GuideName"`
	Tags        string `json:"Tags"` // comma-separated: "drm" is copy protection
	URL         string `json:"URL"`
	// Not in the maker's published guide, but sent by devices in use.
	VideoCodec string `json:"VideoCodec"`
	AudioCodec string `json:"AudioCodec"`
	HD         int    `json:"HD"`
	DRM        int    `json:"DRM"`
	Favorite   int    `json:"Favorite"`
}

// protected: the published marking is a tag, and some firmware sends a field.
func (l lineupEntry) protected() bool {
	if l.DRM != 0 {
		return true
	}
	for _, t := range strings.Split(l.Tags, ",") {
		if strings.EqualFold(strings.TrimSpace(t), "drm") {
			return true
		}
	}
	return false
}

// atsc3: these devices name the codecs of a channel, and only ATSC 3.0 uses these.
func (l lineupEntry) atsc3() bool {
	return strings.EqualFold(l.VideoCodec, "hevc") || strings.EqualFold(l.AudioCodec, "ac4") || strings.EqualFold(l.AudioCodec, "ac-4")
}

func getJSON(ctx context.Context, c *http.Client, addr string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", addr, nil)
	if err != nil {
		return err
	}
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("the device answered %s", res.Status)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(out)
}

func (t *netTuner) Info() Info {
	i := Info{Driver: "network", Name: t.cfg.Name, MaxStreams: t.dev.TunerCount, Detail: t.base}
	if t.dev.ModelNumber != "" {
		i.Detail = strings.TrimSpace(t.dev.FriendlyName + " " + t.dev.ModelNumber + " at " + t.base)
	}
	return i
}

func (t *netTuner) discover(ctx context.Context) error {
	if t.dev.DeviceID != "" || t.dev.ModelNumber != "" {
		return nil
	}
	if err := getJSON(ctx, t.env.http(), t.base+"/discover.json", &t.dev); err != nil {
		return fmt.Errorf("no tuner answered at %s: %w", t.base, err)
	}
	return nil
}

func (t *netTuner) Channels(ctx context.Context) ([]Channel, error) {
	if err := t.discover(ctx); err != nil {
		return nil, err
	}
	var lineup []lineupEntry
	if err := getJSON(ctx, t.env.http(), t.base+"/lineup.json", &lineup); err != nil {
		return nil, fmt.Errorf("the channel list could not be read: %w", err)
	}
	out := make([]Channel, 0, len(lineup))
	for _, l := range lineup {
		if l.GuideNumber == "" {
			continue
		}
		c := Channel{ID: l.GuideNumber, Number: l.GuideNumber, Name: firstNonEmpty(l.GuideName, l.GuideNumber), Locator: l.URL, HD: l.HD != 0,
			Checked: true, ATSC3: l.atsc3(), VideoCodec: l.VideoCodec, AudioCodec: l.AudioCodec}
		if l.protected() {
			c.Encrypted = "the device marks it as copy protected"
		}
		out = append(out, c)
	}
	return out, nil
}

// Scan asks the device to look for channels, and follows its progress.
func (t *netTuner) Scan(ctx context.Context, o ScanOptions, progress func(Progress)) ([]Channel, error) {
	if err := t.discover(ctx); err != nil {
		return nil, err
	}
	source := firstNonEmpty(o.Source, t.cfg.Options["source"], "Antenna")
	if source != "Antenna" && source != "Cable" {
		return nil, errors.New("the signal is Antenna or Cable")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", t.base+"/lineup.post?scan=start&source="+source, nil)
	if err != nil {
		return nil, err
	}
	res, err := t.env.http().Do(req)
	if err != nil {
		return nil, fmt.Errorf("the scan could not start: %w", err)
	}
	res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("the device refused the scan: %s (it may be busy)", res.Status)
	}
	tick := time.NewTicker(scanPoll)
	defer tick.Stop()
	for {
		var st struct {
			ScanInProgress int `json:"ScanInProgress"`
			Progress       int `json:"Progress"`
			Found          int `json:"Found"`
		}
		if err := getJSON(ctx, t.env.http(), t.base+"/lineup_status.json", &st); err != nil {
			return nil, err
		}
		if progress != nil {
			progress(Progress{Percent: st.Progress, Found: st.Found})
		}
		if st.ScanInProgress == 0 {
			return t.Channels(ctx)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tick.C:
		}
	}
}

var scanPoll = 2 * time.Second

func (t *netTuner) Open(ctx context.Context, ch Channel) (*Stream, error) {
	if ch.Encrypted != "" {
		return nil, &EncryptedError{ch.Name, ch.Encrypted}
	}
	addr, err := t.streamURL(ch)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", addr, nil)
	if err != nil {
		return nil, err
	}
	res, err := t.env.stream().Do(req)
	if err != nil {
		return nil, fmt.Errorf("the tuner could not be reached: %w", err)
	}
	if res.StatusCode != 200 {
		defer res.Body.Close()
		// The device explains itself in a header, as its maker's guide lists.
		code := res.Header.Get("X-HDHomeRun-Error")
		num, _, _ := strings.Cut(code, " ")
		switch num {
		case "811":
			return nil, &EncryptedError{ch.Name, "the device says it needs copy protection"}
		case "803", "804", "805", "809":
			return nil, ErrBusy
		case "801":
			return nil, errors.New("the device no longer has this channel: scan again")
		case "806":
			return nil, errors.New("the tuner could not tune this channel: the signal may be too weak")
		case "807":
			return nil, errors.New("the tuner found no picture on this channel: the signal may be too weak")
		}
		if res.StatusCode == 503 {
			return nil, ErrBusy
		}
		return nil, fmt.Errorf("the tuner answered %s %s", res.Status, code)
	}
	return guarded(res.Body), nil
}

// streamURL uses the address from the lineup, but only on the device's own
// host: a lineup is data from the network, not a reason to go elsewhere.
func (t *netTuner) streamURL(ch Channel) (string, error) {
	base, _ := url.Parse(t.base)
	u, err := url.Parse(ch.Locator)
	if err != nil || u.Host == "" {
		return "http://" + net.JoinHostPort(base.Hostname(), "5004") + "/auto/v" + url.PathEscape(ch.Number), nil
	}
	if u.Hostname() != base.Hostname() || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("the device's channel list points at another address")
	}
	return u.String(), nil
}

func (t *netTuner) Close() error { return nil }

// --------------------------------------------------------------------------
// Discovery
// --------------------------------------------------------------------------

const (
	hdhrPort        = 65001
	hdhrDiscoverReq = 0x0002
	hdhrDiscoverRes = 0x0003
	hdhrTagType     = 0x01
	hdhrTagID       = 0x02
	hdhrTagTuners   = 0x10
	hdhrTagBaseURL  = 0x2a
	hdhrTypeTuner   = 0x00000001
	hdhrWildcard    = 0xffffffff
)

// hdhrPacket frames a discovery packet: type, length, tags, little-endian CRC.
func hdhrPacket(typ uint16, tags []byte) []byte {
	b := make([]byte, 4, 8+len(tags))
	binary.BigEndian.PutUint16(b, typ)
	binary.BigEndian.PutUint16(b[2:], uint16(len(tags)))
	b = append(b, tags...)
	return binary.LittleEndian.AppendUint32(b, crc32.ChecksumIEEE(b))
}

func hdhrDiscoverRequest() []byte {
	tags := []byte{hdhrTagType, 4, 0, 0, 0, hdhrTypeTuner, hdhrTagID, 4, 0xff, 0xff, 0xff, 0xff}
	return hdhrPacket(hdhrDiscoverReq, tags)
}

type hdhrReply struct {
	id      uint32
	tuners  int
	baseURL string
}

func parseHDHRReply(b []byte) (r hdhrReply, ok bool) {
	if len(b) < 8 {
		return r, false
	}
	if binary.BigEndian.Uint16(b) != hdhrDiscoverRes {
		return r, false
	}
	n := int(binary.BigEndian.Uint16(b[2:]))
	if 4+n+4 > len(b) || binary.LittleEndian.Uint32(b[4+n:]) != crc32.ChecksumIEEE(b[:4+n]) {
		return r, false
	}
	tags := b[4 : 4+n]
	for len(tags) >= 2 {
		tag := tags[0]
		l := int(tags[1])
		i := 2
		if l&0x80 != 0 {
			if len(tags) < 3 {
				return r, false
			}
			l = l&0x7f | int(tags[2])<<7
			i = 3
		}
		if i+l > len(tags) {
			return r, false
		}
		v := tags[i : i+l]
		switch tag {
		case hdhrTagID:
			if l == 4 {
				r.id = binary.BigEndian.Uint32(v)
			}
		case hdhrTagTuners:
			if l == 1 {
				r.tuners = int(v[0])
			}
		case hdhrTagBaseURL:
			r.baseURL = string(v)
		}
		tags = tags[i+l:]
	}
	return r, r.id != 0
}

// lanHost keeps discovery to the local network: a reply cannot send the
// server off to the internet.
func lanHost(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())
}

func discoverNetworkTuners(ctx context.Context, env Env) []Found {
	hosts := map[string]bool{}
	// The maker's UDP protocol.
	if conn, err := net.ListenPacket("udp4", ":0"); err == nil {
		defer conn.Close()
		req := hdhrDiscoverRequest()
		for _, dst := range broadcastAddrs() {
			_, _ = conn.WriteTo(req, &net.UDPAddr{IP: dst, Port: hdhrPort})
		}
		_ = conn.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
		buf := make([]byte, 2048)
		for {
			n, from, err := conn.ReadFrom(buf)
			if err != nil {
				break
			}
			if _, ok := parseHDHRReply(buf[:n]); ok {
				if u, ok := from.(*net.UDPAddr); ok {
					hosts[u.IP.String()] = true
				}
			}
		}
	}
	// SSDP, which devices that emulate one answer.
	for _, loc := range ssdpSearch(ctx, "urn:schemas-upnp-org:device:MediaServer:1", 1500*time.Millisecond) {
		if u, err := url.Parse(loc); err == nil && lanHost(u.Hostname()) {
			hosts[u.Host] = true
		}
	}
	var out []Found
	for h := range hosts {
		var d discoverJSON
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := getJSON(c, env.http(), "http://"+h+"/discover.json", &d)
		cancel()
		if err != nil || (d.DeviceID == "" && d.ModelNumber == "") {
			continue
		}
		out = append(out, Found{Driver: "network", Name: firstNonEmpty(d.FriendlyName, "Network tuner"),
			Detail: fmt.Sprintf("%s, %d tuners, %s", d.ModelNumber, d.TunerCount, h), Options: map[string]string{"address": h}})
	}
	return out
}

func broadcastAddrs() []net.IP {
	out := []net.IP{net.IPv4bcast}
	ifs, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil {
				continue
			}
			ip := n.IP.To4()
			b := make(net.IP, 4)
			for k := range b {
				b[k] = ip[k] | ^n.Mask[len(n.Mask)-4+k]
			}
			out = append(out, b)
		}
	}
	return out
}

// ssdpSearch multicasts an M-SEARCH and collects the LOCATION of each answer.
func ssdpSearch(ctx context.Context, st string, wait time.Duration) []string {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil
	}
	defer conn.Close()
	msg := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: " + st + "\r\nUSER-AGENT: WittyTuner/1 UPnP/1.1\r\n\r\n"
	dst := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}
	_, _ = conn.WriteTo([]byte(msg), dst)
	deadline := time.Now().Add(wait)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetReadDeadline(deadline)
	seen := map[string]bool{}
	var out []string
	buf := make([]byte, 4096)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return out
		}
		if loc := ssdpLocation(buf[:n]); loc != "" && !seen[loc] {
			seen[loc] = true
			out = append(out, loc)
		}
	}
}

func ssdpLocation(b []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if ok && strings.EqualFold(strings.TrimSpace(k), "location") {
			v = strings.TrimSpace(v)
			if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
				return v
			}
		}
	}
	return ""
}
