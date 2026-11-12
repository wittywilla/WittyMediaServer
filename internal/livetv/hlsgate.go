// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/remote"
	"github.com/wittywilla/wittymediaserver/internal/tuner"
	"github.com/wittywilla/wittymediaserver/internal/tuner/mpegts"
)

// A live stream over HTTP is read by FFmpeg, and left to itself FFmpeg asks for the key of an
// encrypted HLS playlist the moment it sees one: no option of its HLS demuxer turns that off. It
// also makes its own connections, so it would go wherever a playlist or a redirect sends it.
//
// So FFmpeg is pointed at this gate instead, a small server on this machine's loopback address, for
// that one stream. Every playlist FFmpeg reads has been read here first and written out again, naming
// only addresses of the gate: a key is refused before FFmpeg sees it, and a playlist can't lead FFmpeg
// anywhere else. FFmpeg's command line no longer holds the stream's address either, which can hold a
// login.

const (
	// maxGatePlaylist is more than any real live playlist.
	maxGatePlaylist = 2 << 20
	// maxGateNames is how many addresses of one stream are remembered: minutes of a live stream,
	// and a playlist can't grow memory past it.
	maxGateNames = 4096
	// gateSniff is how much of a segment is looked at before any of it is passed on.
	gateSniff = 32 << 10
)

type hlsGate struct {
	client  *http.Client
	ua      string
	headers http.Header
	log     *slog.Logger
	refuse  func(why string)
	refused atomic.Bool

	ln     net.Listener
	srv    *http.Server
	secret string
	entry  string

	mu     sync.Mutex
	names  map[string]gateAddress
	byAddr map[string]string
	order  []string
	serial int
}

type gateAddress struct {
	addr string
	kind gateKind
}

type gateKind int

const (
	gateSegment  gateKind = iota
	gatePlaylist          // read and rewritten
	gateEntry             // what the channel names: a playlist if it reads as one, else a stream
)

// gateInput finds the address in FFmpeg's options for a stream worth putting behind the gate: an
// HTTP stream that isn't a DASH manifest, which has its own way of being checked on opening and
// which FFmpeg resolves in ways a playlist rewrite can't follow.
func gateInput(st *tuner.Stream) (string, bool) {
	if st == nil || st.Reader != nil || (st.Verified && st.Recheck == nil) {
		return "", false
	}
	for i, a := range st.Input {
		if a == "-i" && i+1 < len(st.Input) {
			u, err := url.Parse(st.Input[i+1])
			if err != nil {
				return "", false
			}
			s := strings.ToLower(u.Scheme)
			return st.Input[i+1], s == "http" || s == "https"
		}
	}
	return "", false
}

// newHLSGate starts a gate for one stream. home says the stream was named by the owner or a playlist
// on the home network, so it may lead to the home network; anything else stays on the internet.
func newHLSGate(addr string, input []string, home bool, log *slog.Logger, refuse func(string)) (*hlsGate, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("the stream can't be checked as it plays: %w", err)
	}
	g := &hlsGate{client: gateClient(home), log: log, refuse: refuse, ln: ln, secret: rand.Text(), names: map[string]gateAddress{}, byAddr: map[string]string{}}
	for i := 0; i+1 < len(input); i++ {
		switch input[i] {
		case "-user_agent":
			g.ua = input[i+1]
		case "-headers":
			g.headers = parseFFmpegHeaders(input[i+1])
		}
	}
	g.entry = g.name(addr, gateEntry)
	g.srv = &http.Server{Handler: g, ReadHeaderTimeout: 10 * time.Second, MaxHeaderBytes: 16 << 10}
	logging.Go("live stream gate", func() { _ = g.srv.Serve(ln) }, "cat", "tv")
	return g, nil
}

// Close ends the gate, and any request still going through it.
func (g *hlsGate) Close() {
	if g != nil {
		_ = g.srv.Close()
	}
}

// args is FFmpeg's input options with the gate in place of the stream: it needs nothing but plain
// HTTP to this machine, and what the stream's server wants to see stays here.
func (g *hlsGate) args(input []string) []string {
	out := make([]string, 0, len(input))
	for i := 0; i < len(input); i++ {
		a := input[i]
		switch {
		case (a == "-user_agent" || a == "-headers") && i+1 < len(input):
			i++
		case a == "-protocol_whitelist" && i+1 < len(input):
			out = append(out, a, "http,tcp")
			i++
		case a == "-i" && i+1 < len(input):
			out = append(out, a, g.url(g.entry))
			i++
		default:
			out = append(out, a)
		}
	}
	return out
}

func (g *hlsGate) url(name string) string {
	return "http://" + g.ln.Addr().String() + "/" + g.secret + "/" + name
}

// name gives an address its name at the gate. The same address keeps its name, so FFmpeg sees a live
// playlist as the same playlist each time it reads it.
func (g *hlsGate) name(addr string, kind gateKind) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if n, ok := g.byAddr[addr]; ok {
		if kind != gateSegment && g.names[n].kind == gateSegment {
			g.names[n] = gateAddress{addr, kind}
		}
		return n
	}
	g.serial++
	n := strconv.Itoa(g.serial) + gateExt(addr, kind)
	g.names[n], g.byAddr[addr] = gateAddress{addr, kind}, n
	g.order = append(g.order, n)
	// The oldest go first, and only segments: a live stream's old segments are never asked for again.
	if len(g.order) > maxGateNames {
		for i, old := range g.order {
			if g.names[old].kind == gateSegment {
				delete(g.byAddr, g.names[old].addr)
				delete(g.names, old)
				g.order = append(g.order[:i], g.order[i+1:]...)
				break
			}
		}
	}
	return n
}

// gateExt keeps a name's usual ending, as FFmpeg looks at it, and only ever one from this list.
func gateExt(addr string, kind gateKind) string {
	ext := ""
	if u, err := url.Parse(addr); err == nil {
		ext = strings.ToLower(path.Ext(u.Path))
	}
	switch ext {
	case ".m3u8", ".ts", ".m4s", ".mp4", ".m4a", ".m4v", ".aac", ".ac3", ".eac3", ".mp3", ".mkv", ".mpg", ".mpeg", ".vtt", ".webvtt", ".ogg", ".wav", ".flac":
		return ext
	}
	switch kind {
	case gateSegment:
		return ".ts"
	case gatePlaylist:
		return ".m3u8"
	}
	return ""
}

func (g *hlsGate) lookup(name string) (gateAddress, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	a, ok := g.names[name]
	return a, ok
}

func (g *hlsGate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name, ok := strings.CutPrefix(r.URL.Path, "/"+g.secret+"/")
	if !ok || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		http.NotFound(w, r)
		return
	}
	if g.refused.Load() {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	a, ok := g.lookup(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := g.pass(w, r, a); err != nil && r.Context().Err() == nil {
		g.log.DebugContext(r.Context(), "a live stream's server gave something that wasn't passed on", "topic", "tuner", "err", err)
	}
}

// pass asks the stream's server, then hands on what it may.
func (g *hlsGate) pass(w http.ResponseWriter, r *http.Request, a gateAddress) error {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, a.addr, nil)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return errors.New("that isn't an address")
	}
	for k, vs := range g.headers {
		req.Header[k] = vs
	}
	if g.ua != "" {
		req.Header.Set("User-Agent", g.ua)
	}
	if v := r.Header.Get("Range"); v != "" {
		req.Header.Set("Range", v)
	}
	res, err := g.client.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return gateError(err)
	}
	defer res.Body.Close()
	head := make([]byte, gateSniff)
	n, rerr := io.ReadFull(res.Body, head)
	head = head[:n]
	if a.kind == gatePlaylist || looksLikeHLS(head) {
		if a.kind == gateSegment {
			w.WriteHeader(http.StatusBadGateway)
			return errors.New("a segment turned out to be a playlist")
		}
		return g.passPlaylist(w, res, head, rerr)
	}
	if a.kind == gateSegment && looksLikeManifest(head) {
		w.WriteHeader(http.StatusBadGateway)
		return errors.New("a segment turned out to be a text file")
	}
	if isScrambled(head) {
		g.stop(tuner.ScrambledReason)
		w.WriteHeader(http.StatusForbidden)
		return nil
	}
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := res.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(res.StatusCode)
	if r.Method == http.MethodHead {
		return nil
	}
	if _, err := w.Write(head); err != nil || rerr != nil {
		return nil // the end of a short answer, or FFmpeg has gone
	}
	_, _ = io.Copy(w, res.Body)
	return nil
}

func (g *hlsGate) passPlaylist(w http.ResponseWriter, res *http.Response, head []byte, rerr error) error {
	body := head
	if rerr == nil {
		rest, err := io.ReadAll(io.LimitReader(res.Body, maxGatePlaylist+1-int64(len(head))))
		if err != nil {
			rerr = err
		}
		body = append(body, rest...)
	}
	// What arrived is read whatever came with it: a key in a playlist that was cut short, or that
	// came with an error, is still a key.
	out, why, err := g.rewrite(res.Request.URL, body)
	if why != "" {
		g.stop(why)
		w.WriteHeader(http.StatusForbidden)
		return nil
	}
	switch {
	case err != nil:
		w.WriteHeader(http.StatusBadGateway)
		return err
	case res.StatusCode != http.StatusOK:
		w.WriteHeader(res.StatusCode)
		return nil
	case len(body) > maxGatePlaylist:
		w.WriteHeader(http.StatusBadGateway)
		return errors.New("a playlist too long to read")
	case rerr != nil && !errors.Is(rerr, io.EOF) && !errors.Is(rerr, io.ErrUnexpectedEOF):
		w.WriteHeader(http.StatusBadGateway)
		return rerr
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(out)
	return nil
}

func (g *hlsGate) stop(why string) {
	if g.refused.CompareAndSwap(false, true) {
		g.refuse(why)
	}
}

// rewrite reads a playlist. It gives back the playlist FFmpeg may see, or why the stream may not be
// played. Lines are built again rather than copied, so what FFmpeg reads is only what was read here.
func (g *hlsGate) rewrite(base *url.URL, b []byte) (out []byte, why string, err error) {
	var buf bytes.Buffer
	next := gateSegment
	mapped := false
	for line := range strings.Lines(strings.TrimPrefix(string(b), "\xef\xbb\xbf")) {
		l := strings.TrimSpace(line)
		switch {
		case l == "":
			continue
		case strings.HasPrefix(l, "#EXT-X-KEY:"), strings.HasPrefix(l, "#EXT-X-SESSION-KEY:"):
			if why := library.HLSKeyProtection(l); why != "" {
				return nil, why, nil
			}
			if strings.HasPrefix(l, "#EXT-X-KEY:") {
				buf.WriteString("#EXT-X-KEY:METHOD=NONE\n")
			}
			continue
		case strings.HasPrefix(l, "#EXT-X-MAP:"):
			mapped = true
			fixed, err := g.rewriteAttrs(base, l, "#EXT-X-MAP:", gateSegment, ".mp4")
			if err != nil {
				return nil, "", err
			}
			buf.WriteString(fixed + "\n")
			continue
		case strings.HasPrefix(l, "#EXT-X-MEDIA:"):
			fixed, err := g.rewriteAttrs(base, l, "#EXT-X-MEDIA:", gatePlaylist, "")
			if err != nil {
				return nil, "", err
			}
			buf.WriteString(fixed + "\n")
			continue
		case strings.HasPrefix(l, "#EXT-X-STREAM-INF:"):
			next = gatePlaylist
			buf.WriteString(l + "\n")
			continue
		case strings.HasPrefix(l, "#"):
			if gateKeepsTag(l) {
				buf.WriteString(l + "\n")
			}
			continue
		}
		addr, err := gateResolve(base, l)
		if err != nil {
			return nil, "", err
		}
		name := g.name(addr, next)
		if mapped && next == gateSegment && strings.HasSuffix(name, ".ts") {
			name = g.rename(name, ".m4s")
		}
		buf.WriteString(g.url(name) + "\n")
		next = gateSegment
	}
	return buf.Bytes(), "", nil
}

// gateKeepsTag: the tags a live stream needs that name no address. Whatever else a playlist
// holds stays here: FFmpeg has no use for the others it knows, and what it doesn't know has
// no business in a playlist it reads.
func gateKeepsTag(l string) bool {
	tag, _, _ := strings.Cut(l, ":")
	switch tag {
	case "#EXTM3U", "#EXTINF", "#EXT-X-VERSION", "#EXT-X-TARGETDURATION", "#EXT-X-MEDIA-SEQUENCE", "#EXT-X-DISCONTINUITY",
		"#EXT-X-DISCONTINUITY-SEQUENCE", "#EXT-X-ENDLIST", "#EXT-X-PLAYLIST-TYPE", "#EXT-X-PROGRAM-DATE-TIME",
		"#EXT-X-INDEPENDENT-SEGMENTS", "#EXT-X-START", "#EXT-X-BYTERANGE", "#EXT-X-ALLOW-CACHE":
		return true
	}
	return false
}

// rename changes a segment's ending, for a stream whose segments are fragments of an MP4 whatever
// their server calls them.
func (g *hlsGate) rename(name, ext string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	a, ok := g.names[name]
	if !ok {
		return name
	}
	n := strings.TrimSuffix(name, path.Ext(name)) + ext
	g.names[n], g.byAddr[a.addr] = a, n
	delete(g.names, name)
	for i, old := range g.order {
		if old == name {
			g.order[i] = n
			break
		}
	}
	return n
}

// rewriteAttrs builds a tag again with its URI at the gate.
func (g *hlsGate) rewriteAttrs(base *url.URL, l, tag string, kind gateKind, ext string) (string, error) {
	attrs := library.ParseHLSAttrs(strings.TrimPrefix(l, tag))
	var b strings.Builder
	b.WriteString(tag)
	for i, a := range attrs {
		if i > 0 {
			b.WriteByte(',')
		}
		if a.Key != "URI" {
			b.WriteString(a.Raw)
			continue
		}
		addr, err := gateResolve(base, a.Value)
		if err != nil {
			return "", err
		}
		name := g.name(addr, kind)
		if ext != "" && strings.HasSuffix(name, ".ts") {
			name = g.rename(name, ext)
		}
		b.WriteString(`URI="` + g.url(name) + `"`)
	}
	return b.String(), nil
}

// gateResolve finds what a playlist's line means, for web addresses only.
func gateResolve(base *url.URL, ref string) (string, error) {
	u, err := base.Parse(ref)
	if err != nil {
		return "", errors.New("a playlist names something that isn't an address")
	}
	if s := strings.ToLower(u.Scheme); (s != "http" && s != "https") || u.Host == "" {
		return "", errors.New("a playlist names something that isn't a web address")
	}
	u.Fragment = ""
	return u.String(), nil
}

func looksLikeHLS(b []byte) bool {
	return bytes.HasPrefix(bytes.TrimLeft(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), " \t\r\n"), []byte("#EXTM3U"))
}

// looksLikeManifest: text a demuxer could follow to somewhere else, handed over as a segment.
func looksLikeManifest(b []byte) bool {
	t := bytes.TrimLeft(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), " \t\r\n")
	for _, p := range []string{"#EXT", "ffconcat", "<", "[Script Info]", "file ", "file\t"} {
		if bytes.HasPrefix(t, []byte(p)) {
			return true
		}
	}
	return false
}

// isScrambled reads the start of a transport stream, as the tuner guard reads a whole one.
func isScrambled(b []byte) bool {
	if len(b) < mpegts.PacketSize || b[0] != 0x47 {
		return false
	}
	seen, bad := 0, 0
	for ; len(b) >= mpegts.PacketSize; b = b[mpegts.PacketSize:] {
		h, ok := mpegts.ParsePacket(b[:mpegts.PacketSize])
		if !ok || h.PID == 0x1fff {
			continue
		}
		seen++
		if h.Scrambled {
			bad++
		}
	}
	return seen >= 32 && bad*2 >= seen
}

func parseFFmpegHeaders(s string) http.Header {
	h := http.Header{}
	for l := range strings.Lines(s) {
		k, v, ok := strings.Cut(strings.TrimSpace(l), ":")
		if ok && k != "" {
			h.Add(strings.TrimSpace(k), strings.TrimSpace(v))
		}
	}
	return h
}

// gateError keeps an address, which can hold a login, out of what is logged.
func gateError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// gateClient is how the gate reaches a stream. library.SafeClient holds a stream from the internet to
// the internet, checking each connection as it is made, so a name that changes what it points at
// can't get around it. A stream at home can go anywhere but the addresses a cloud host answers
// questions about itself on.
func gateClient(home bool) *http.Client {
	if !home {
		return library.SafeClient
	}
	d := &net.Dialer{Timeout: 15 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip == nil || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("refusing to connect to %s", host)
		}
		return nil
	}}
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				return d.DialContext(ctx, network, address)
			},
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConnsPerHost:   4,
			IdleConnTimeout:       90 * time.Second,
		},
		CheckRedirect: library.SafeClient.CheckRedirect,
	}
}

// mayLeadHome: the owner named this tuner's address, or its playlist is a file or on the home
// network, going by the address the owner typed: one a home network or VPN uses, or a name only a
// local network can resolve. Asking DNS instead would let a public name point home when it suits it.
// A playlist from the internet stays on the internet.
func mayLeadHome(t *Tuner) bool {
	var p string
	switch t.Driver {
	case "iptv":
		p = t.Options["playlist"]
	case "xtream":
		// A provider's panel: on the internet unless its own address says home.
		p = t.Options["url"]
	default:
		return true
	}
	if !strings.Contains(p, "://") {
		return true
	}
	u, err := url.Parse(p)
	if err != nil {
		return false
	}
	return remote.LocalAddress(u.Hostname())
}
