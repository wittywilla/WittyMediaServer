package wittytuner

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func init() {
	register(Driver{
		ID: "iptv", Name: "IPTV playlist (M3U)", Status: "tested",
		Description: "Channels from an M3U or M3U8 playlist, from an address or a file. Bring your own playlist: none is included.",
		Fields: []Field{
			{Key: "playlist", Label: "Playlist address or file", Kind: "url", Required: true, Help: "http(s):// address, or the full path of a file on the server."},
			{Key: "guide", Label: "Guide address (XMLTV)", Kind: "url", Help: "Optional. If empty, the guide address inside the playlist is used."},
			{Key: "userAgent", Label: "User agent", Kind: "text", Help: "Optional. A few providers ask for a particular one."},
			{Key: "streams", Label: "Streams at once", Kind: "number", Help: "Optional. Many providers allow only one or two."},
		},
		build: func(c Config, e Env) (Source, error) {
			p := strings.TrimSpace(c.Options["playlist"])
			if err := checkPlaylistPlace(p); err != nil {
				return nil, err
			}
			return &iptv{cfg: c, env: e, playlist: p}, nil
		},
	})
	register(Driver{
		ID: "stream", Name: "IPTV single stream", Status: "tested",
		Description: "One channel from one address: HTTP, HLS, RTSP, RTP or UDP.",
		Fields: []Field{
			{Key: "url", Label: "Stream address", Kind: "url", Required: true},
			{Key: "number", Label: "Channel number", Kind: "text"},
			{Key: "userAgent", Label: "User agent", Kind: "text"},
		},
		build: func(c Config, e Env) (Source, error) {
			u := strings.TrimSpace(c.Options["url"])
			if err := checkStreamURL(u, true); err != nil {
				return nil, err
			}
			return &iptv{cfg: c, env: e, single: u}, nil
		},
	})
}

const maxPlaylist = 64 << 20

func checkPlaylistPlace(p string) error {
	if strings.Contains(p, "://") {
		u, err := url.Parse(p)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("playlists come from http(s) addresses or files on the server")
		}
		return nil
	}
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		return errors.New("that playlist file was not found on the server")
	}
	return nil
}

// checkStreamURL allows the kinds of address FFmpeg should open for a channel.
// A playlist can come from anywhere, so nothing it lists may make the server
// read its own files or reach the cloud metadata addresses. local allows this
// machine (the owner typed the address, or the playlist itself is local).
func checkStreamURL(raw string, local bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("that is not a stream address")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "rtsp", "rtsps", "rtp", "udp", "rtmp", "rtmps", "srt":
	default:
		return fmt.Errorf("%s addresses are not allowed for channels", u.Scheme)
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLinkLocalUnicast() {
		return errors.New("that address is not allowed for a channel")
	}
	if !local {
		if strings.EqualFold(host, "localhost") || (ip != nil && (ip.IsLoopback() || ip.IsUnspecified())) {
			return errors.New("a playlist from elsewhere may not point at this server")
		}
	}
	return nil
}

// M3UEntry is one channel of a playlist.
type M3UEntry struct {
	Channel
	URL string
}

// M3U is a parsed playlist.
type M3U struct {
	GuideURL string
	Entries  []M3UEntry
	Skipped  int
}

// ParseM3U reads an extended M3U playlist. Comments it does not understand are
// skipped, so one odd line does not lose a channel list.
func ParseM3U(r io.Reader) (*M3U, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 2<<20)
	out := &M3U{}
	var cur *M3UEntry
	drm := ""
	num := 0
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if first {
			line = strings.TrimPrefix(line, "\xef\xbb\xbf")
			first = false
		}
		switch {
		case line == "":
		case strings.HasPrefix(line, "#EXTM3U"):
			attrs := parseAttrs(line[len("#EXTM3U"):])
			for _, k := range []string{"url-tvg", "x-tvg-url", "tvg-url"} {
				if v := attrs[k]; v != "" && out.GuideURL == "" {
					out.GuideURL, _, _ = strings.Cut(v, ",")
				}
			}
		case strings.HasPrefix(line, "#EXTINF:"):
			head, title := splitExtinf(line[len("#EXTINF:"):])
			attrs := parseAttrs(head)
			cur = &M3UEntry{Channel: Channel{
				Name: firstNonEmpty(title, attrs["tvg-name"]), EPGID: attrs["tvg-id"], Logo: attrs["tvg-logo"],
				Group: attrs["group-title"], Number: firstNonEmpty(attrs["tvg-chno"], attrs["channel-number"]),
			}}
		case strings.HasPrefix(line, "#EXTGRP:"):
			if cur != nil && cur.Group == "" {
				cur.Group = strings.TrimSpace(line[len("#EXTGRP:"):])
			}
		case strings.HasPrefix(line, "#"):
			l := strings.ToLower(line)
			// Licence lines mean a DRM-protected stream.
			if strings.Contains(l, "license_key") || strings.Contains(l, "license_type") || strings.Contains(l, "widevine") || strings.Contains(l, "playready") || strings.Contains(l, "fairplay") || strings.Contains(l, "clearkey") || strings.Contains(l, "drm") {
				drm = "the playlist lists a DRM licence for it"
			}
		default:
			if cur == nil {
				cur = &M3UEntry{}
			}
			cur.URL = line
			if cur.Name == "" {
				cur.Name = line
			}
			if cur.Number == "" {
				num++
				cur.Number = strconv.Itoa(num)
			}
			// Addresses often carry a login token that changes, and two entries
			// can share one (SD and HD names), so the name is part of the id.
			cur.ID = idOf(firstNonEmpty(cur.EPGID, line) + "|" + cur.Name)
			cur.Locator = line
			cur.Encrypted = drm
			out.Entries = append(out.Entries, *cur)
			cur = nil
			drm = ""
		}
	}
	return out, sc.Err()
}

func idOf(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:8])
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// splitExtinf separates the duration and attributes from the title: the title
// follows the first comma outside quotes.
func splitExtinf(s string) (head, title string) {
	quoted := false
	for i, c := range s {
		switch c {
		case '"':
			quoted = !quoted
		case ',':
			if !quoted {
				return s[:i], strings.TrimSpace(s[i+1:])
			}
		}
	}
	return s, ""
}

// parseAttrs reads key="value" pairs.
func parseAttrs(s string) map[string]string {
	out := map[string]string{}
	for {
		i := strings.Index(s, `="`)
		if i < 0 {
			return out
		}
		start := strings.LastIndexAny(s[:i], " \t") + 1
		key := strings.ToLower(s[start:i])
		rest := s[i+2:]
		j := strings.IndexByte(rest, '"')
		if j < 0 {
			return out
		}
		out[key] = rest[:j]
		s = rest[j+1:]
	}
}

type iptv struct {
	cfg      Config
	env      Env
	playlist string
	single   string
	guide    string
}

func (s *iptv) Info() Info {
	i := Info{Driver: "iptv", Name: s.cfg.Name, GuideURL: firstNonEmpty(s.cfg.Options["guide"], s.guide)}
	i.MaxStreams, _ = strconv.Atoi(s.cfg.Options["streams"])
	if s.single != "" {
		i.Driver = "stream"
	}
	return i
}

func (s *iptv) localPlaylist() bool {
	if !strings.Contains(s.playlist, "://") {
		return true
	}
	u, err := url.Parse(s.playlist)
	if err != nil {
		return false
	}
	h := u.Hostname()
	ip := net.ParseIP(h)
	return strings.EqualFold(h, "localhost") || (ip != nil && (ip.IsLoopback() || ip.IsPrivate()))
}

func (s *iptv) load(ctx context.Context) (*M3U, error) {
	var r io.ReadCloser
	if strings.Contains(s.playlist, "://") {
		req, err := http.NewRequestWithContext(ctx, "GET", s.playlist, nil)
		if err != nil {
			return nil, err
		}
		if ua := s.cfg.Options["userAgent"]; ua != "" {
			req.Header.Set("User-Agent", ua)
		}
		res, err := s.env.http().Do(req)
		if err != nil {
			return nil, fmt.Errorf("the playlist could not be fetched: %w", err)
		}
		if res.StatusCode != 200 {
			res.Body.Close()
			return nil, fmt.Errorf("the playlist could not be fetched: the server answered %s", res.Status)
		}
		r = res.Body
	} else {
		f, err := os.Open(s.playlist)
		if err != nil {
			return nil, err
		}
		r = f
	}
	defer r.Close()
	body := bufio.NewReader(io.LimitReader(r, maxPlaylist))
	var src io.Reader = body
	if m, _ := body.Peek(2); len(m) == 2 && m[0] == 0x1f && m[1] == 0x8b {
		gz, err := gzip.NewReader(body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		src = io.LimitReader(gz, maxPlaylist)
	}
	return ParseM3U(src)
}

func (s *iptv) Channels(ctx context.Context) ([]Channel, error) {
	if s.single != "" {
		name := firstNonEmpty(s.cfg.Name, "Stream")
		return []Channel{{ID: idOf(s.single), Number: firstNonEmpty(s.cfg.Options["number"], "1"), Name: name, Locator: s.single}}, nil
	}
	m, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	s.guide = m.GuideURL
	local := s.localPlaylist()
	out := make([]Channel, 0, len(m.Entries))
	for _, e := range m.Entries {
		if checkStreamURL(e.URL, local) != nil {
			continue
		}
		out = append(out, e.Channel)
	}
	if len(out) == 0 {
		return nil, errors.New("no usable channels in that playlist")
	}
	return out, nil
}

func (s *iptv) Scan(ctx context.Context, _ ScanOptions, p func(Progress)) ([]Channel, error) {
	return noScan(s, ctx, p)
}

func (s *iptv) Close() error { return nil }

// Open starts a stream. Plain MPEG-TS over HTTP comes through here, so the
// guard can watch it; HLS is checked for encryption first and handed to
// FFmpeg, without the crypto protocol it would need to decrypt anything.
func (s *iptv) Open(ctx context.Context, ch Channel) (*Stream, error) {
	if ch.Encrypted != "" {
		return nil, &EncryptedError{ch.Name, ch.Encrypted}
	}
	local := s.single != "" || s.localPlaylist()
	if err := checkStreamURL(ch.Locator, local); err != nil {
		return nil, err
	}
	u, _ := url.Parse(ch.Locator)
	ua := s.cfg.Options["userAgent"]
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return openHTTPStream(ctx, s.env, ch, ua, nil)
	// A relay turns the rest into MPEG-TS, so the guard can read them too.
	case "rtp", "udp":
		return relay(ctx, s.env, []string{"-protocol_whitelist", "udp,rtp", "-i", ch.Locator})
	case "rtmp", "rtmps":
		return relay(ctx, s.env, []string{"-protocol_whitelist", "rtmp,rtmps,tcp,tls", "-i", ch.Locator})
	case "srt":
		// A passphrase here would have FFmpeg decrypting the stream.
		if u.Query().Has("passphrase") {
			return nil, &EncryptedError{ch.Name, "its address asks for a decryption passphrase"}
		}
		return relay(ctx, s.env, []string{"-protocol_whitelist", "srt,udp", "-i", ch.Locator})
	default:
		return relay(ctx, s.env, []string{"-rtsp_transport", "tcp", "-protocol_whitelist", "rtsp,rtsps,rtp,udp,tcp,tls", "-i", ch.Locator})
	}
}

// httpInput builds FFmpeg's options for an HTTP(S) address, leaving out the
// crypto protocol so encrypted HLS segments cannot be read.
func httpInput(addr, ua string, headers http.Header) []string {
	in := []string{"-protocol_whitelist", "http,https,tcp,tls", "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5"}
	if ua != "" {
		in = append(in, "-user_agent", ua)
	}
	if len(headers) > 0 {
		var b strings.Builder
		for k, vs := range headers {
			for _, v := range vs {
				b.WriteString(k + ": " + v + "\r\n")
			}
		}
		in = append(in, "-headers", b.String())
	}
	return append(in, "-i", addr)
}

// openHTTPStream reads the start of an address to see what it is.
func openHTTPStream(ctx context.Context, env Env, ch Channel, ua string, headers http.Header) (*Stream, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", ch.Locator, nil)
	if err != nil {
		return nil, err
	}
	for k, vs := range headers {
		req.Header[k] = vs
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	res, err := env.stream().Do(req)
	if err != nil {
		return nil, fmt.Errorf("the stream could not be reached: %w", err)
	}
	if res.StatusCode == 503 || res.StatusCode == 429 {
		res.Body.Close()
		return nil, ErrBusy
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		return nil, fmt.Errorf("the stream answered %s", res.Status)
	}
	head := make([]byte, 4096)
	n, _ := io.ReadFull(res.Body, head)
	head = head[:n]
	ct := strings.ToLower(res.Header.Get("Content-Type"))
	// DTCP-IP: a licensed device sending a stream that only a licensed player
	// may open. It reached us still protected, so it stays that way.
	if strings.Contains(ct, "dtcp") {
		res.Body.Close()
		return nil, &EncryptedError{ch.Name, "the device sends it copy protected"}
	}
	if bytes.Contains(head, []byte("<MPD")) {
		rest, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if bytes.Contains(append(head, rest...), []byte("ContentProtection")) {
			return nil, &EncryptedError{ch.Name, "its DASH manifest uses DRM"}
		}
		return &Stream{Input: httpInput(ch.Locator, ua, headers), Verified: true}, nil
	}
	isHLS := bytes.HasPrefix(bytes.TrimPrefix(head, []byte("\xef\xbb\xbf")), []byte("#EXTM3U")) || strings.Contains(ct, "mpegurl")
	if isHLS {
		rest, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if why := hlsProtection(append(head, rest...)); why != "" {
			return nil, &EncryptedError{ch.Name, why}
		}
		if v := firstVariant(append(head, rest...)); v != "" {
			if why := checkVariant(ctx, env, ch.Locator, v, ua, headers); why != "" {
				return nil, &EncryptedError{ch.Name, why}
			}
		}
		return &Stream{Input: httpInput(ch.Locator, ua, headers), Verified: true}, nil
	}
	if n >= 188 && head[0] == 0x47 && (n < 376 || head[188] == 0x47) {
		// Raw MPEG-TS: read on, starting with what was already read.
		return guarded(&prefixed{Reader: io.MultiReader(bytes.NewReader(head), res.Body), c: res.Body}), nil
	}
	res.Body.Close()
	return &Stream{Input: httpInput(ch.Locator, ua, headers)}, nil
}

type prefixed struct {
	io.Reader
	c io.Closer
}

func (p *prefixed) Close() error { return p.c.Close() }

// hlsProtection says why an HLS playlist cannot be played, or "".
func hlsProtection(b []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(l, "#EXT-X-KEY:") && !strings.HasPrefix(l, "#EXT-X-SESSION-KEY:") && !strings.HasPrefix(l, "#EXT-X-MAP:") {
			continue
		}
		if strings.HasPrefix(l, "#EXT-X-MAP:") {
			continue
		}
		attrs := strings.ToUpper(l)
		if strings.Contains(attrs, "METHOD=NONE") {
			continue
		}
		if strings.Contains(attrs, "KEYFORMAT") || strings.Contains(attrs, "SAMPLE-AES") {
			return "its HLS playlist uses DRM"
		}
		return "its HLS playlist is encrypted"
	}
	return ""
}

// firstVariant finds a master playlist's first stream.
func firstVariant(b []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	next := false
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(l, "#EXT-X-STREAM-INF") {
			next = true
			continue
		}
		if next && l != "" && !strings.HasPrefix(l, "#") {
			return l
		}
	}
	return ""
}

func checkVariant(ctx context.Context, env Env, base, v, ua string, headers http.Header) string {
	bu, err := url.Parse(base)
	if err != nil {
		return ""
	}
	ru, err := bu.Parse(v)
	if err != nil || (ru.Scheme != "http" && ru.Scheme != "https") {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, "GET", ru.String(), nil)
	if err != nil {
		return ""
	}
	for k, vs := range headers {
		req.Header[k] = vs
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	res, err := env.http().Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	return hlsProtection(b)
}

// relay runs FFmpeg to bring a stream to MPEG-TS without converting anything.
func relay(ctx context.Context, env Env, input []string) (*Stream, error) {
	if env.FFmpeg == "" {
		return nil, errors.New("FFmpeg is needed for this stream")
	}
	args := append([]string{"-hide_banner", "-loglevel", "error", "-nostdin"}, input...)
	args = append(args, "-map", "0", "-c", "copy", "-f", "mpegts", "pipe:1")
	ctx, cancel := context.WithCancel(ctx)
	cmd := env.command(ctx, env.FFmpeg, args...)
	stderr := &stderrTail{}
	cmd.Stderr = stderr
	cmd.WaitDelay = 2 * time.Second // a child holding stderr open mustn't hold Close
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	return guarded(&procReader{ReadCloser: out, cancel: cancel, wait: cmd.Wait, ctx: ctx, log: env.logger(), what: "ffmpeg", stderr: stderr, started: time.Now()}), nil
}

type procReader struct {
	io.ReadCloser
	cancel context.CancelFunc
	wait   func() error

	ctx      context.Context
	log      *slog.Logger
	what     string
	stderr   *stderrTail
	started  time.Time
	ended    atomic.Bool // what it was writing ran out before anyone closed it
	waitOnce sync.Once
}

func (p *procReader) Read(b []byte) (int, error) {
	n, err := p.ReadCloser.Read(b)
	if err != nil {
		p.ended.Store(true)
	}
	return n, err
}

// Close stops the process, and says how it ended if that wasn't this.
func (p *procReader) Close() error {
	ownAccord := p.ended.Load() && p.ctx.Err() == nil
	p.cancel()
	err := p.ReadCloser.Close()
	p.waitOnce.Do(func() {
		werr := p.wait()
		lv := slog.LevelDebug
		if ownAccord && werr != nil {
			lv = slog.LevelWarn
		}
		p.log.Log(p.ctx, lv, "process ended", "topic", "tuner", "process", p.what, "err", werr, "stderr", p.stderr.String(),
			"ran", time.Since(p.started).Round(time.Millisecond).String())
	})
	return err
}

// stderrTail keeps the end of what a process prints, whatever it prints.
type stderrTail struct {
	mu sync.Mutex
	b  []byte
}

const stderrKeep = 8 << 10

func (t *stderrTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := len(p)
	if len(p) >= stderrKeep {
		t.b, p = t.b[:0], p[len(p)-stderrKeep:]
	}
	t.b = append(t.b, p...)
	if len(t.b) > 2*stderrKeep {
		t.b = append(t.b[:0], t.b[len(t.b)-stderrKeep:]...)
	}
	return n, nil
}

func (t *stderrTail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	b := t.b
	if len(b) > stderrKeep {
		b = b[len(b)-stderrKeep:]
	}
	return strings.TrimSpace(string(b))
}

// basicAuth is the header for a login.
func basicAuth(user, pass string) http.Header {
	if user == "" {
		return nil
	}
	return http.Header{"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))}}
}
