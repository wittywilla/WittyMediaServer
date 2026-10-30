package wittytuner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// External backends: TV servers that own the tuners and offer them over an
// open protocol. Two are spoken here: an HTTP API with an M3U channel list and
// XMLTV guide, and SVDRP (text commands) with an HTTP streaming plugin.

func init() {
	register(Driver{
		ID: "backend", Name: "External TV backend", Status: "mock",
		Description: "A TV server you already run, over an open protocol: an HTTP API with an M3U channel list and XMLTV guide, or SVDRP. Tried against stand-in servers only.",
		Fields: []Field{
			{Key: "protocol", Label: "Protocol", Kind: "select", Required: true, Default: "http-m3u", Options: []string{"http-m3u", "svdrp"}},
			{Key: "address", Label: "Server address", Kind: "text", Required: true, Help: "http://host:port for HTTP; host for SVDRP."},
			{Key: "username", Label: "User name", Kind: "text"},
			{Key: "password", Label: "Password", Kind: "password"},
			{Key: "playlistPath", Label: "Channel list path", Kind: "text", Default: "/playlist/channels.m3u", Help: "HTTP only."},
			{Key: "guidePath", Label: "Guide path", Kind: "text", Default: "/xmltv/channels", Help: "HTTP only."},
			{Key: "svdrpPort", Label: "SVDRP port", Kind: "number", Default: "6419"},
			{Key: "streamPort", Label: "Streaming port", Kind: "number", Default: "3000", Help: "SVDRP only: the HTTP streaming plugin's port."},
			{Key: "streams", Label: "Streams at once", Kind: "number"},
		},
		build: func(c Config, e Env) (Source, error) {
			switch c.Options["protocol"] {
			case "svdrp":
				host := strings.TrimSpace(c.Options["address"])
				if host == "" || strings.ContainsAny(host, "/?#@ ") {
					return nil, errors.New("the SVDRP address is a host name or address")
				}
				return &backend{cfg: c, env: e, host: host}, nil
			default:
				base, err := deviceBase(c.Options["address"])
				if err != nil {
					return nil, err
				}
				if p := c.Options["playlistPath"]; !strings.HasPrefix(p, "/") {
					return nil, errors.New("the channel list path starts with /")
				}
				return &backend{cfg: c, env: e, base: base}, nil
			}
		},
	})
}

type backend struct {
	cfg  Config
	env  Env
	base string
	host string
}

func (b *backend) Info() Info {
	i := Info{Driver: "backend", Name: b.cfg.Name, Detail: firstNonEmpty(b.base, b.host)}
	i.MaxStreams, _ = strconv.Atoi(b.cfg.Options["streams"])
	if b.base != "" {
		i.GuideURL = b.base + b.cfg.Options["guidePath"]
	}
	return i
}

func (b *backend) headers() http.Header {
	return basicAuth(b.cfg.Options["username"], b.cfg.Options["password"])
}

// GuideHeaders are what a guide request needs.
func (b *backend) GuideHeaders() http.Header { return b.headers() }

func (b *backend) Channels(ctx context.Context) ([]Channel, error) {
	if b.host != "" {
		return b.svdrpChannels(ctx)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", b.base+b.cfg.Options["playlistPath"], nil)
	if err != nil {
		return nil, err
	}
	for k, v := range b.headers() {
		req.Header[k] = v
	}
	res, err := b.env.http().Do(req)
	if err != nil {
		return nil, fmt.Errorf("the backend could not be reached: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return nil, errors.New("the backend refused the user name and password")
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("the backend answered %s", res.Status)
	}
	m, err := ParseM3U(io.LimitReader(res.Body, maxPlaylist))
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(b.base)
	out := make([]Channel, 0, len(m.Entries))
	for _, e := range m.Entries {
		// Stream addresses must stay on the backend's own host; those from a
		// playlist with embedded passwords are not kept in that form.
		u, err := url.Parse(e.URL)
		if err != nil || u.Hostname() != base.Hostname() {
			continue
		}
		u.User = nil
		e.Locator = u.String()
		out = append(out, e.Channel)
	}
	if len(out) == 0 {
		return nil, errors.New("no channels came from the backend")
	}
	return out, nil
}

func (b *backend) Scan(ctx context.Context, _ ScanOptions, p func(Progress)) ([]Channel, error) {
	return noScan(b, ctx, p)
}

func (b *backend) Close() error { return nil }

func (b *backend) Open(ctx context.Context, ch Channel) (*Stream, error) {
	if ch.Encrypted != "" {
		return nil, &EncryptedError{ch.Name, ch.Encrypted}
	}
	return openHTTPStream(ctx, b.env, ch, "", b.headers())
}

// svdrpChannels asks for the channel list: LSTC answers lines such as
// 250-1 Name;Provider:freq:params:src:rate:vpid:apid:tpid:ca:sid:nid:tid:rid
func (b *backend) svdrpChannels(ctx context.Context) ([]Channel, error) {
	port := firstNonEmpty(b.cfg.Options["svdrpPort"], "6419")
	d := net.Dialer{Timeout: 8 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(b.host, port))
	if err != nil {
		return nil, fmt.Errorf("the backend could not be reached: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	r := bufio.NewReader(conn)
	if line, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(line, "220") {
		return nil, errors.New("that does not answer like an SVDRP server")
	}
	if _, err := io.WriteString(conn, "LSTC\r\n"); err != nil {
		return nil, err
	}
	streamPort := firstNonEmpty(b.cfg.Options["streamPort"], "3000")
	var out []Channel
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if len(line) < 4 {
			continue
		}
		if !strings.HasPrefix(line, "250") {
			return nil, fmt.Errorf("the server said: %s", line)
		}
		if c, ok := parseSVDRPChannel(line[4:], b.host, streamPort); ok {
			out = append(out, c)
		}
		if line[3] == ' ' {
			break
		}
	}
	_, _ = io.WriteString(conn, "QUIT\r\n")
	return out, nil
}

func parseSVDRPChannel(s, host, port string) (Channel, bool) {
	number, rest, ok := strings.Cut(s, " ")
	if !ok {
		return Channel{}, false
	}
	f := strings.Split(rest, ":")
	if len(f) < 13 {
		return Channel{}, false
	}
	name := f[0]
	group := ""
	if n, p, ok := strings.Cut(name, ";"); ok {
		name, group = n, p
	}
	if n, _, ok := strings.Cut(name, ","); ok {
		name = n
	}
	// source-nid-tid-sid(-rid) identifies a channel in VDR's own notation.
	id := f[3] + "-" + f[10] + "-" + f[11] + "-" + f[9]
	if f[12] != "0" && f[12] != "" {
		id += "-" + f[12]
	}
	c := Channel{ID: id, Number: number, Name: name, Group: group}
	c.Program, _ = strconv.Atoi(f[9])
	u := url.URL{Scheme: "http", Host: net.JoinHostPort(host, port), Path: "/TS/" + id}
	c.Locator = u.String()
	if ca := f[8]; ca != "0" && ca != "" {
		c.Encrypted = "the backend lists a conditional access system for it"
	}
	if f[5] == "0" {
		c.Radio = true
	}
	return c, true
}
