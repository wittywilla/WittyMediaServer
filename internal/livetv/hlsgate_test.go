// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/tuner"
)

func testGate(t *testing.T, addr string, home bool) (*hlsGate, *string) {
	t.Helper()
	why := new(string)
	g, err := newHLSGate(addr, []string{"-i", addr}, home, slog.Default(), func(w string) { *why = w })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	return g, why
}

func gateGet(t *testing.T, addr string) (int, string) {
	t.Helper()
	res, err := http.Get(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestGateRewritesEveryAddress(t *testing.T) {
	g, _ := testGate(t, "http://upstream.invalid/live/master.m3u8", true)
	base, _ := url.Parse("http://upstream.invalid/live/master.m3u8")
	in := `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="a",NAME="English, main",URI="audio/en.m3u8"
#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=1,URI="http://elsewhere.invalid/iframe.m3u8"
#EXT-X-SESSION-DATA:DATA-ID="x",URI="http://elsewhere.invalid/data.json"
#EXT-X-STREAM-INF:BANDWIDTH=1000,AUDIO="a"
low/index.m3u8?t=1
#EXT-X-STREAM-INF:BANDWIDTH=2000
https://cdn.invalid/high.m3u8
`
	out, why, err := g.rewrite(base, []byte(in))
	if err != nil || why != "" {
		t.Fatalf("%q %v", why, err)
	}
	s := string(out)
	if strings.Contains(s, "invalid") || strings.Contains(s, "elsewhere") {
		t.Errorf("an address got through:\n%s", s)
	}
	if !strings.Contains(s, `NAME="English, main"`) {
		t.Errorf("a quoted comma was lost:\n%s", s)
	}
	seen := map[string]bool{}
	for _, a := range g.names {
		seen[a.addr] = a.kind == gatePlaylist
	}
	for _, want := range []string{"http://upstream.invalid/live/audio/en.m3u8", "http://upstream.invalid/live/low/index.m3u8?t=1", "https://cdn.invalid/high.m3u8"} {
		if !seen[want] {
			t.Errorf("%s isn't a playlist at the gate: %v", want, seen)
		}
	}

	media := `#EXTM3U
#EXT-X-TARGETDURATION:6
#EXT-X-MAP:URI="init.bin",BYTERANGE="100@0"
#EXT-X-KEY:METHOD=NONE
#EXTINF:6,
seg1.m4s
#EXT-X-PART:DURATION=1,URI="part.m4s"
#EXT-X-SOMETHING-NEW:Uri = "http://elsewhere.invalid/x"
#EXT-X-BYTERANGE:100@200
#EXTINF:6,
../seg2
`
	out, why, err = g.rewrite(base, []byte(media))
	if err != nil || why != "" {
		t.Fatalf("%q %v", why, err)
	}
	s = string(out)
	if !strings.Contains(s, "#EXT-X-KEY:METHOD=NONE\n") || !strings.Contains(s, "#EXT-X-BYTERANGE:100@200\n") || !strings.Contains(s, `BYTERANGE="100@0"`) {
		t.Errorf("a tag was lost:\n%s", s)
	}
	if strings.Contains(s, "part.m4s") || strings.Contains(s, "#EXT-X-PART") || strings.Contains(s, "elsewhere") {
		t.Errorf("a tag naming an address got through:\n%s", s)
	}
	if !strings.Contains(s, ".mp4\"") || !strings.Contains(s, ".m4s\n") {
		t.Errorf("FFmpeg goes by these endings:\n%s", s)
	}
}

// Each of these is a key, however it is dressed up.
func TestGateRefusesEveryKindOfKey(t *testing.T) {
	g, _ := testGate(t, "http://upstream.invalid/l.m3u8", true)
	base, _ := url.Parse("http://upstream.invalid/l.m3u8")
	for _, l := range []string{
		`#EXT-X-KEY:METHOD=AES-128,URI="key"`,
		`#EXT-X-KEY:METHOD=SAMPLE-AES,URI="key"`,
		`#EXT-X-KEY:METHOD=SAMPLE-AES-CTR,KEYFORMAT="com.widevine",URI="key"`,
		`#EXT-X-SESSION-KEY:METHOD=SAMPLE-AES,URI="key"`,
		// Words that looked like no key to a search of the line.
		`#EXT-X-KEY:METHOD=AES-128,URI="key",IV="METHOD=NONE"`,
		`#EXT-X-KEY:URI="METHOD=NONE",METHOD=AES-128`,
		`#EXT-X-KEY:METHOD=NONE,METHOD=AES-128,URI="key"`,
		`#EXT-X-KEY:URI="key"`,
		`#EXT-X-KEY:`,
		"  #EXT-X-KEY:METHOD=AES-128,URI=\"key\"  ",
	} {
		_, why, err := g.rewrite(base, []byte("#EXTM3U\n"+l+"\n#EXTINF:1,\nseg.ts\n"))
		if err != nil || why == "" {
			t.Errorf("%s: %q %v", l, why, err)
		}
	}
	for _, l := range []string{"#EXT-X-KEY:METHOD=NONE", `#EXT-X-KEY:METHOD="NONE"`} {
		if _, why, err := g.rewrite(base, []byte("#EXTM3U\n"+l+"\n")); err != nil || why != "" {
			t.Errorf("%s: %q %v", l, why, err)
		}
	}
}

// A playlist can't send FFmpeg to a file, or to anything else that isn't the web.
func TestGateRefusesOtherKindsOfAddress(t *testing.T) {
	g, _ := testGate(t, "http://upstream.invalid/l.m3u8", true)
	base, _ := url.Parse("http://upstream.invalid/l.m3u8")
	for _, ref := range []string{"file:///etc/passwd", "concat:a|b", "data:text/plain,x", "ftp://h/x", "http://", "crypto+http://h/x", "pipe:0", "lavfi:testsrc", `file:///c:/x`} {
		for _, pl := range []string{"#EXTM3U\n#EXTINF:1,\n" + ref + "\n", "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n" + ref + "\n", `#EXTM3U` + "\n" + `#EXT-X-MAP:URI="` + ref + `"` + "\n"} {
			if out, why, err := g.rewrite(base, []byte(pl)); err == nil {
				t.Errorf("%q: %q %q", pl, out, why)
			}
		}
	}
}

// The stream's address and login are the gate's business, not on FFmpeg's command line.
func TestGateTakesTheStreamOffTheCommandLine(t *testing.T) {
	addr := "https://user:secret@tv.invalid/live.m3u8?key=hunter2"
	in := []string{"-protocol_whitelist", "http,https,tcp,tls", "-reconnect", "1", "-user_agent", "Box/1", "-headers", "Cookie: id=1\r\nX-Token: abc\r\n", "-i", addr}
	g, err := newHLSGate(addr, in, true, slog.Default(), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	args := strings.Join(g.args(in), " ")
	for _, s := range []string{"secret", "hunter2", "Cookie", "X-Token", "Box/1", "https", "tls", "tv.invalid"} {
		if strings.Contains(args, s) {
			t.Errorf("%q is on the command line: %s", s, args)
		}
	}
	if !strings.Contains(args, "-protocol_whitelist http,tcp ") || !strings.Contains(args, "-reconnect 1") || !strings.HasSuffix(args, "-i "+g.url(g.entry)) {
		t.Errorf("%s", args)
	}
	if g.ua != "Box/1" || g.headers.Get("Cookie") != "id=1" || g.headers.Get("X-Token") != "abc" {
		t.Errorf("%q %v", g.ua, g.headers)
	}
}

func TestGatePassesWhatTheServerWants(t *testing.T) {
	var ua, cookie, rng atomic.Value
	mux := http.NewServeMux()
	mux.HandleFunc("/live.m3u8", func(w http.ResponseWriter, r *http.Request) {
		ua.Store(r.UserAgent())
		cookie.Store(r.Header.Get("Cookie"))
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXTINF:1,\nseg.ts\n")
	})
	mux.HandleFunc("/seg.ts", func(w http.ResponseWriter, r *http.Request) {
		rng.Store(r.Header.Get("Range"))
		w.Header().Set("Content-Range", "bytes 2-4/10")
		w.WriteHeader(206)
		_, _ = w.Write([]byte("abc"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	addr := srv.URL + "/live.m3u8"
	in := []string{"-user_agent", "Box/1", "-headers", "Cookie: id=1\r\n", "-i", addr}
	g, err := newHLSGate(addr, in, true, slog.Default(), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	code, body := gateGet(t, g.url(g.entry))
	if code != 200 || !strings.Contains(body, g.secret) {
		t.Fatalf("%d %s", code, body)
	}
	if ua.Load() != "Box/1" || cookie.Load() != "id=1" {
		t.Errorf("%v %v", ua.Load(), cookie.Load())
	}
	var seg string
	for l := range strings.Lines(body) {
		if strings.HasPrefix(l, "http://") {
			seg = strings.TrimSpace(l)
		}
	}
	req, _ := http.NewRequest("GET", seg, nil)
	req.Header.Set("Range", "bytes=2-4")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 206 || string(b) != "abc" || res.Header.Get("Content-Range") != "bytes 2-4/10" || rng.Load() != "bytes=2-4" {
		t.Errorf("%d %q %v %v", res.StatusCode, b, res.Header, rng.Load())
	}
}

func TestGateNeedsItsSecret(t *testing.T) {
	asked := atomic.Int64{}
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { asked.Add(1) }))
	defer srv.Close()
	g, _ := testGate(t, srv.URL+"/a.ts", true)
	host := "http://" + g.ln.Addr().String()
	for _, p := range []string{"/", "/" + g.entry, "/wrong/" + g.entry, "/" + g.secret + "/none.ts", "/" + g.secret + "/../" + g.entry} {
		if code, _ := gateGet(t, host+p); code != 404 {
			t.Errorf("%s: %d", p, code)
		}
	}
	if asked.Load() != 0 {
		t.Errorf("the stream's server was asked %d times", asked.Load())
	}
}

// A segment that turns out to be a playlist would have FFmpeg following its addresses by itself.
func TestGateRefusesAPlaylistDressedAsASegment(t *testing.T) {
	for _, body := range []string{"#EXTM3U\n#EXTINF:1,\nhttp://192.0.2.1/secret\n", "\xef\xbb\xbf#EXTM3U\n", "ffconcat version 1.0\nfile /etc/passwd\n", "<MPD></MPD>", "<?xml version=\"1.0\"?>"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, body) }))
		g, _ := testGate(t, srv.URL+"/live.m3u8", true)
		base, _ := url.Parse(srv.URL + "/live.m3u8")
		out, _, err := g.rewrite(base, []byte("#EXTM3U\n#EXTINF:1,\nseg.ts\n"))
		if err != nil {
			t.Fatal(err)
		}
		var seg string
		for l := range strings.Lines(string(out)) {
			if strings.HasPrefix(l, "http://") {
				seg = strings.TrimSpace(l)
			}
		}
		code, got := gateGet(t, seg)
		if code != 502 || got != "" {
			t.Errorf("%q: %d %q", body, code, got)
		}
		srv.Close()
	}
}

// The first look saw a video file. A second look at the same address can't turn into an encrypted
// playlist that FFmpeg alone reads.
func TestGateStreamThatTurnsIntoAKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"k\"\n#EXTINF:1,\ns.ts\n")
	}))
	defer srv.Close()
	g, why := testGate(t, srv.URL+"/film.mp4", true)
	code, body := gateGet(t, g.url(g.entry))
	if code != 403 || strings.Contains(body, "EXT-X-KEY") || !strings.Contains(*why, "encrypted") {
		t.Errorf("%d %q %q", code, body, *why)
	}
	if code, _ := gateGet(t, g.url(g.entry)); code != 403 {
		t.Errorf("refused is refused from then on: %d", code)
	}
}

func TestGateRefusesAScrambledSegment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tsData(200, true)) }))
	defer srv.Close()
	g, why := testGate(t, srv.URL+"/s.ts", true)
	if code, _ := gateGet(t, g.url(g.entry)); code != 403 || *why != tuner.ScrambledReason {
		t.Errorf("%d %q", code, *why)
	}
	clear := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tsData(200, false)) }))
	defer clear.Close()
	g, why = testGate(t, clear.URL+"/s.ts", true)
	if code, body := gateGet(t, g.url(g.entry)); code != 200 || len(body) != 200*188 || *why != "" {
		t.Errorf("%d %d %q", code, len(body), *why)
	}
}

// A playlist from the internet can't use the gate to reach this machine or the home network,
// by address, by name, or by a redirect.
func TestGateFromTheInternetStaysThere(t *testing.T) {
	secret := atomic.Int64{}
	home := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { secret.Add(1) }))
	defer home.Close()
	u, _ := url.Parse(home.URL)
	for _, addr := range []string{home.URL + "/a.ts", "http://localhost:" + u.Port() + "/a.ts", "http://[::ffff:127.0.0.1]:" + u.Port() + "/a.ts"} {
		g, _ := testGate(t, addr, false)
		if code, _ := gateGet(t, g.url(g.entry)); code != 502 {
			t.Errorf("%s: %d", addr, code)
		}
	}
	if secret.Load() != 0 {
		t.Errorf("reached the home server %d times", secret.Load())
	}
	if gateClient(false) != library.SafeClient {
		t.Error("a stream from the internet has to use the internet-only client")
	}
}

// Even a stream at home can't ask a cloud host's metadata service.
func TestGateAtHomeRefusesLinkLocal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://169.254.169.254/latest/meta-data/", nil)
	res, err := gateClient(true).Do(req)
	if err == nil {
		res.Body.Close()
		t.Fatal("it connected")
	}
	if !strings.Contains(err.Error(), "refusing") {
		t.Errorf("%v", err)
	}
}

func TestMayLeadHome(t *testing.T) {
	for _, tc := range []struct {
		t    Tuner
		want bool
	}{
		{Tuner{Driver: "network"}, true},
		{Tuner{Driver: "stream"}, true},
		{Tuner{Driver: "iptv", Options: map[string]string{"playlist": "/srv/list.m3u"}}, true},
		{Tuner{Driver: "iptv", Options: map[string]string{"playlist": "http://192.168.1.2/list.m3u"}}, true},
		{Tuner{Driver: "iptv", Options: map[string]string{"playlist": "http://localhost/list.m3u"}}, true},
		{Tuner{Driver: "iptv", Options: map[string]string{"playlist": "https://iptv.example.com/list.m3u"}}, false},
		{Tuner{Driver: "iptv", Options: map[string]string{"playlist": "https://8.8.8.8/list.m3u"}}, false},
	} {
		if got := mayLeadHome(&tc.t); got != tc.want {
			t.Errorf("%+v: %v", tc.t, got)
		}
	}
}

// Addresses a playlist makes up can't grow memory for ever, and playlists are never forgotten.
func TestGateRemembersABoundedNumber(t *testing.T) {
	g, _ := testGate(t, "http://upstream.invalid/l.m3u8", true)
	base, _ := url.Parse("http://upstream.invalid/l.m3u8")
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nvariant.m3u8\n")
	for i := range maxGateNames * 2 {
		fmt.Fprintf(&b, "#EXTINF:1,\nseg%d.ts\n", i)
	}
	if _, _, err := g.rewrite(base, []byte(b.String())); err != nil {
		t.Fatal(err)
	}
	if len(g.names) > maxGateNames || len(g.byAddr) > maxGateNames {
		t.Errorf("%d %d", len(g.names), len(g.byAddr))
	}
	if _, ok := g.lookup(g.entry); !ok {
		t.Error("the entry was forgotten")
	}
	if n, ok := g.byAddr["http://upstream.invalid/variant.m3u8"]; !ok || g.names[n].kind != gatePlaylist {
		t.Error("a playlist was forgotten")
	}
}

// The whole point, with the real FFmpeg: it is reading a live stream when a key appears. It never
// asks for the key, and the stream is refused as soon as FFmpeg next reads the playlist, rather than
// at the next look around.
func TestFFmpegNeverSeesAKeyThatComesLater(t *testing.T) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("needs FFmpeg")
	}
	segFile := filepath.Join(t.TempDir(), "seg.ts")
	gen := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc=s=64x64:r=5:d=1", "-c:v", "mpeg2video", "-f", "mpegts", segFile)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("can't make a segment: %v %s", err, out)
	}
	segment, err := os.ReadFile(segFile)
	if err != nil {
		t.Fatal(err)
	}
	var keyAsked, segments atomic.Int64
	var locked atomic.Bool
	started := time.Now()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/list.m3u", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "#EXTM3U\n#EXTINF:-1,Live\n%s/live.m3u8\n", srv.URL)
	})
	mux.HandleFunc("/live.m3u8", func(w http.ResponseWriter, _ *http.Request) {
		seq := int(time.Since(started) / time.Second)
		fmt.Fprintf(w, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n#EXT-X-MEDIA-SEQUENCE:%d\n", seq)
		if locked.Load() {
			fmt.Fprint(w, "#EXT-X-KEY:METHOD=AES-128,URI=\"/key\"\n")
		}
		for i := range 3 {
			fmt.Fprintf(w, "#EXTINF:1,\nseg%d.ts\n", seq+i)
		}
	})
	mux.HandleFunc("/key", func(w http.ResponseWriter, _ *http.Request) {
		keyAsked.Add(1)
		_, _ = w.Write(make([]byte, 16))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".ts") {
			segments.Add(1)
			_, _ = w.Write(segment)
			return
		}
		w.WriteHeader(404)
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	m := testManager(t)
	ctx := context.Background()
	tn, err := m.Create(ctx, &Tuner{Driver: "iptv", Name: "Live", Options: map[string]string{"playlist": srv.URL + "/list.m3u"}})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := m.Channels(ctx, tn.ID)
	if err != nil || len(ch) != 1 {
		t.Fatalf("%v %+v", err, ch)
	}
	o, err := m.Open(ctx, ch[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Release()
	// The stream's own look is left out of this, so what's measured is the gate.
	o.stopWatch()
	if strings.Contains(strings.Join(o.Stream.Input, " "), srv.URL) {
		t.Fatalf("FFmpeg was given the stream's own address: %v", o.Stream.Input)
	}
	run, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	args := append([]string{"-hide_banner", "-loglevel", "error", "-nostdin"}, o.Stream.Input...)
	cmd := exec.CommandContext(run, bin, append(args, "-map", "0", "-c", "copy", "-f", "null", "-")...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	deadline := time.Now().Add(15 * time.Second)
	for segments.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if segments.Load() < 2 {
		t.Fatal("FFmpeg isn't reading through the gate")
	}
	locked.Store(true)
	at := time.Now()
	refusedIn(t, o, 8*time.Second)
	t.Logf("refused %v after the key appeared", time.Since(at))
	if !tuner.IsEncrypted(o.Refusal()) {
		t.Errorf("%v", o.Refusal())
	}
	time.Sleep(1500 * time.Millisecond)
	if n := keyAsked.Load(); n != 0 {
		t.Errorf("FFmpeg asked for the key %d times", n)
	}
	got, err := m.Channel(ctx, ch[0].ID)
	if err != nil || got.Encrypted == "" {
		t.Errorf("not marked: %+v %v", got, err)
	}

	// What this is here for: left alone with the same playlist, FFmpeg goes straight for the key.
	look, stop := context.WithTimeout(ctx, 3*time.Second)
	defer stop()
	direct := exec.CommandContext(look, bin, "-hide_banner", "-loglevel", "error", "-nostdin", "-protocol_whitelist", "http,https,tcp,tls", "-t", "1", "-i", srv.URL+"/live.m3u8", "-map", "0", "-c", "copy", "-f", "null", "-")
	_ = direct.Run()
	t.Logf("FFmpeg reading the stream itself asked for the key %d times", keyAsked.Load())
}

func FuzzGateRewrite(f *testing.F) {
	for _, s := range []string{
		"#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\na.m3u8\n",
		"#EXTM3U\n#EXT-X-MAP:URI=\"i.mp4\"\n#EXTINF:1,\ns.m4s\n",
		"#EXT-X-MEDIA:URI=\"a\",NAME=\"b",
		"#EXT-X-KEY:METHOD=AES-128,URI=\"k\",IV=\"METHOD=NONE\"\n",
		"\xef\xbb\xbf#EXTM3U\r\n#EXTINF:1,\r\nhttp://h/s.ts\r\n",
		"#EXT-X-MAP:=,=\"\",,URI",
	} {
		f.Add(s)
	}
	g, err := newHLSGate("http://upstream.invalid/l.m3u8", nil, true, slog.New(slog.DiscardHandler), func(string) {})
	if err != nil {
		f.Fatal(err)
	}
	defer g.Close()
	base, _ := url.Parse("http://upstream.invalid/l.m3u8")
	prefix := g.url("")
	f.Fuzz(func(t *testing.T, s string) {
		out, why, err := g.rewrite(base, []byte(s))
		if err != nil || why != "" {
			return
		}
		// Whatever comes out names only the gate: no line FFmpeg would read as an address, and no
		// tag holding one, can point anywhere else.
		for l := range strings.Lines(string(out)) {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			if !strings.HasPrefix(l, "#") {
				if !strings.HasPrefix(l, prefix) {
					t.Fatalf("line %q of %q", l, out)
				}
				continue
			}
			if strings.HasPrefix(l, "#EXT-X-KEY:") && l != "#EXT-X-KEY:METHOD=NONE" {
				t.Fatalf("key %q from %q", l, s)
			}
			tag, rest, _ := strings.Cut(l, ":")
			switch tag {
			case "#EXT-X-MAP", "#EXT-X-MEDIA":
				for _, a := range library.ParseHLSAttrs(rest) {
					if a.Key == "URI" && !strings.HasPrefix(a.Value, prefix) {
						t.Fatalf("uri %q in %q", a.Value, l)
					}
				}
			case "#EXT-X-KEY", "#EXT-X-STREAM-INF":
			default:
				if !gateKeepsTag(l) {
					t.Fatalf("a tag got through: %q", l)
				}
			}
		}
	})
}

// The guide names recordings, and a guide's titles are anyone's.
func TestRecordingNamesAreNeverDevices(t *testing.T) {
	for _, title := range []string{"CON", "con", "PRN", "AUX", "NUL", "COM1", "LPT9", "com¹", "CON.news", "nul.ts", " CON ", "aux..."} {
		if got := safeFileName(title); library.ReservedName(got) {
			t.Errorf("%q gave %q", title, got)
		}
	}
	for title, want := range map[string]string{"Conan": "Conan", "The News": "The News", "../../etc": "etc", "": "Recording"} {
		if got := safeFileName(title); got != want {
			t.Errorf("%q gave %q, wanted %q", title, got, want)
		}
	}
}
