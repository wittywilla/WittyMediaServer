package wittytuner

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SAT>IP (SES specification 1.2.2): servers announce themselves over SSDP and
// tune from an RTSP address such as
//
//	rtsp://server/?src=1&freq=11362&pol=h&msys=dvbs2&mtype=8psk&sr=22000&fec=23&pids=0,17,18
//
// FFmpeg speaks the RTSP. A relay copies its output to MPEG-TS, so the guard
// sees every packet.

func init() {
	register(Driver{
		ID: "satip", Name: "SAT>IP", Status: "untested",
		Description: "Satellite, terrestrial and cable from a SAT>IP server. Built from the published specification and not yet tried with a real server.",
		Scans:       true,
		Fields: []Field{
			{Key: "server", Label: "Server address", Kind: "text", Required: true, Help: "Such as 192.168.1.60."},
			{Key: "playlist", Label: "Channel list address (M3U)", Kind: "url", Help: "Optional. Many servers offer one."},
			{Key: "transponders", Label: "Transponders to scan", Kind: "textarea", Help: "One tuning per line, as SAT>IP parameters: src=1&freq=11362&pol=h&msys=dvbs2&mtype=8psk&sr=22000&fec=23. No lists are included: see your satellite's or provider's published details."},
			{Key: "transport", Label: "Transport", Kind: "select", Options: []string{"udp", "tcp"}, Default: "udp"},
		},
		build: func(c Config, e Env) (Source, error) {
			host := strings.TrimSpace(c.Options["server"])
			if host == "" || strings.ContainsAny(host, "/?#@ ") {
				return nil, errors.New("the server is an address such as 192.168.1.60")
			}
			return &satip{cfg: c, env: e, host: host}, nil
		},
	})
}

type satip struct {
	cfg  Config
	env  Env
	host string
}

func (s *satip) Info() Info {
	return Info{Driver: "satip", Name: s.cfg.Name, Detail: s.host}
}

// Channels reads the server's M3U when there is one. Otherwise channels come
// from a scan.
func (s *satip) Channels(ctx context.Context) ([]Channel, error) {
	addr := strings.TrimSpace(s.cfg.Options["playlist"])
	if addr == "" {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", addr, nil)
	if err != nil {
		return nil, err
	}
	res, err := s.env.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	m, err := ParseM3U(io.LimitReader(res.Body, maxPlaylist))
	if err != nil {
		return nil, err
	}
	out := make([]Channel, 0, len(m.Entries))
	for _, e := range m.Entries {
		out = append(out, e.Channel)
	}
	return out, nil
}

// checkTuning accepts only SAT>IP's own parameters, so a line in the settings
// cannot add other options to the address.
func checkTuning(line string) (url.Values, error) {
	q, err := url.ParseQuery(strings.TrimSpace(strings.TrimPrefix(line, "?")))
	if err != nil {
		return nil, fmt.Errorf("could not read %q", line)
	}
	allowed := map[string]bool{"src": true, "fe": true, "freq": true, "pol": true, "ro": true, "msys": true, "mtype": true, "plts": true, "sr": true, "fec": true, "bw": true, "tmode": true, "gi": true, "plp": true, "t2id": true, "sm": true, "c2tft": true, "ds": true, "specinv": true}
	for k := range q {
		if !allowed[strings.ToLower(k)] {
			return nil, fmt.Errorf("%q is not a SAT>IP tuning setting", k)
		}
	}
	if q.Get("freq") == "" {
		return nil, fmt.Errorf("%q has no freq", line)
	}
	return q, nil
}

func (s *satip) tuneURL(tuning url.Values, pids string) string {
	u := url.URL{Scheme: "rtsp", Host: s.host, Path: "/"}
	// Commas stay literal: servers expect pids=0,17,18.
	u.RawQuery = tuning.Encode() + "&pids=" + pids
	return u.String()
}

func (s *satip) input(addr string) []string {
	return []string{"-rtsp_transport", firstNonEmpty(s.cfg.Options["transport"], "udp"), "-protocol_whitelist", "rtsp,rtp,udp,tcp", "-i", addr}
}

// Scan tunes each transponder in turn and reads what is on it.
func (s *satip) Scan(ctx context.Context, _ ScanOptions, progress func(Progress)) ([]Channel, error) {
	var lines []string
	for _, l := range strings.Split(s.cfg.Options["transponders"], "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		ch, err := s.Channels(ctx)
		if err == nil && ch == nil {
			return nil, errors.New("add transponders or a channel list address first")
		}
		return ch, err
	}
	if s.env.FFmpeg == "" {
		return nil, errors.New("FFmpeg is needed to scan")
	}
	var out []Channel
	for i, line := range lines {
		q, err := checkTuning(line)
		if err != nil {
			return nil, err
		}
		if progress != nil {
			progress(Progress{Percent: i * 100 / len(lines), Found: len(out), Message: "Transponder " + q.Get("freq")})
		}
		st, err := relay(ctx, s.env, s.input(s.tuneURL(q, "all")))
		if err != nil {
			return nil, err
		}
		found := scanMux(ctx, st.Reader, 12*time.Second)
		_ = st.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		for _, sv := range found {
			pids := append([]int{0, 17, 18}, sv.PIDs...)
			sort.Ints(pids)
			ps := make([]string, 0, len(pids))
			for k, p := range pids {
				if k == 0 || p != pids[k-1] {
					ps = append(ps, strconv.Itoa(p))
				}
			}
			out = append(out, Channel{ID: q.Get("freq") + "-" + strconv.Itoa(sv.Program), Number: sv.number(strconv.Itoa(len(out) + 1)), Name: sv.Name, Group: sv.Provider,
				HD: sv.HD, Radio: sv.Radio, Encrypted: sv.Encrypted, Program: sv.Program, Locator: s.tuneURL(q, strings.Join(ps, ","))})
		}
	}
	if progress != nil {
		progress(Progress{Percent: 100, Found: len(out)})
	}
	return out, nil
}

func (s *satip) Open(ctx context.Context, ch Channel) (*Stream, error) {
	if ch.Encrypted != "" {
		return nil, &EncryptedError{ch.Name, ch.Encrypted}
	}
	u, err := url.Parse(ch.Locator)
	if err != nil || u.Scheme != "rtsp" {
		return nil, errors.New("this channel's tuning address is not usable")
	}
	return relay(ctx, s.env, s.input(ch.Locator))
}

func (s *satip) Close() error { return nil }

// discoverSATIP finds servers on the network.
func discoverSATIP(ctx context.Context, env Env) []Found {
	var out []Found
	for _, loc := range ssdpSearch(ctx, "urn:ses-com:device:SatIPServer:1", 1500*time.Millisecond) {
		u, err := url.Parse(loc)
		if err != nil || !lanHost(u.Hostname()) {
			continue
		}
		name, caps := satipDescribe(ctx, env, loc)
		out = append(out, Found{Driver: "satip", Name: firstNonEmpty(name, "SAT>IP server"), Detail: strings.TrimSpace(caps + " " + u.Hostname()), Options: map[string]string{"server": u.Hostname()}})
	}
	return out
}

func satipDescribe(ctx context.Context, env Env, loc string) (name, caps string) {
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(c, "GET", loc, nil)
	if err != nil {
		return "", ""
	}
	res, err := env.http().Do(req)
	if err != nil {
		return "", ""
	}
	defer res.Body.Close()
	var d struct {
		Device struct {
			Name string `xml:"friendlyName"`
			Caps string `xml:"X_SATIPCAP"`
		} `xml:"device"`
	}
	if xml.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&d) != nil {
		return "", ""
	}
	return d.Device.Name, d.Device.Caps
}
