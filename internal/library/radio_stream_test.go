package library

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// useLocalStations lets SafeClient reach a test's own servers; the real one
// refuses this machine.
func useLocalStations(t *testing.T, ts *httptest.Server) {
	t.Helper()
	real := SafeClient
	SafeClient = ts.Client()
	t.Cleanup(func() { SafeClient = real })
}

func TestSafeClientRefusesPrivateAddresses(t *testing.T) {
	inner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("secret")) }))
	defer inner.Close()
	ctx := context.Background()
	if _, err := OpenStation(ctx, inner.URL); err == nil {
		t.Error("a station on this machine was reached")
	}
	if _, err := ProbeStation(ctx, inner.URL); err == nil {
		t.Error("a check reached this machine")
	}
	for _, a := range []string{"10.0.0.1", "127.0.0.1", "192.168.1.1", "169.254.169.254", "::1", "fe80::1", "fc00::1", "::ffff:10.0.0.1", "64:ff9b::a00:1", "2002:a00:1::"} {
		if internetAddr(net.ParseIP(a)) {
			t.Errorf("%s counted as the internet", a)
		}
	}
}

// A station can't reach the private network by redirecting there, nor through a
// playlist, with the real client: each connection is checked as it's made.
func TestRedirectsAndPlaylistsCantReachThePrivateNetwork(t *testing.T) {
	hit := make(chan string, 8)
	inner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit <- r.URL.Path
		_, _ = w.Write([]byte("secret"))
	}))
	defer inner.Close()
	// The outer server stands in for a station on the internet: only its own
	// address is let through, as a dial to anywhere else goes via the real check.
	var outer *httptest.Server
	outer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, inner.URL+"/admin", http.StatusFound)
		case "/list.pls":
			w.Header().Set("Content-Type", "audio/x-scpls")
			_, _ = w.Write([]byte("[playlist]\nFile1=" + inner.URL + "/admin\n"))
		case "/list.m3u":
			w.Header().Set("Content-Type", "audio/x-mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n" + inner.URL + "/admin\n"))
		}
	}))
	defer outer.Close()
	outerAddr := outer.Listener.Addr().String()
	real := SafeClient
	base := SafeClient.Transport.(*http.Transport).Clone()
	base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address == outerAddr {
			return (&net.Dialer{}).DialContext(ctx, network, address)
		}
		return safeDialer.DialContext(ctx, network, address)
	}
	SafeClient = &http.Client{Transport: base, CheckRedirect: real.CheckRedirect}
	defer func() { SafeClient = real }()

	for _, path := range []string{"/redirect", "/list.pls", "/list.m3u"} {
		if st, err := OpenStation(context.Background(), outer.URL+path); err == nil {
			st.Close()
			t.Errorf("%s: opened", path)
		}
	}
	select {
	case p := <-hit:
		t.Errorf("the private network was reached at %s", p)
	default:
	}
}

func TestOpenStationKinds(t *testing.T) {
	var base string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/audio":
			w.Header().Set("Content-Type", "audio/mpeg; charset=x")
			_, _ = w.Write([]byte("mp3"))
		case "/unlabelled":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("\xff\xfbmp3"))
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>"))
		case "/page-as-bytes":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("<html><script>x</script>"))
		case "/video":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("mp4"))
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
		case "/a.pls":
			w.Header().Set("Content-Type", "audio/x-scpls")
			_, _ = w.Write([]byte("[playlist]\nNumberOfEntries=1\nFile1=" + base + "/audio\nTitle1=A\n"))
		case "/plain.pls":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("[Playlist]\nFile1=" + base + "/audio\n"))
		case "/a.m3u":
			w.Header().Set("Content-Type", "audio/mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1,A\n" + base + "/audio\n"))
		case "/relative.m3u":
			w.Header().Set("Content-Type", "audio/mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\naudio\n"))
		case "/loop.m3u":
			w.Header().Set("Content-Type", "audio/mpegurl")
			_, _ = w.Write([]byte(base + "/loop.m3u\n"))
		case "/empty.pls":
			w.Header().Set("Content-Type", "audio/x-scpls")
			_, _ = w.Write([]byte("[playlist]\nNumberOfEntries=0\n"))
		case "/huge.m3u":
			w.Header().Set("Content-Type", "audio/mpegurl")
			_, _ = w.Write(bytes.Repeat([]byte("#comment line\n"), 300000))
		case "/live.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nseg1.aac\n"))
		case "/bad-metaint":
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Header().Set("icy-metaint", "-5")
		default:
			w.WriteHeader(404)
		}
	}))
	defer ts.Close()
	base = ts.URL
	useLocalStations(t, ts)
	ctx := context.Background()
	for _, tc := range []struct {
		path string
		hls  bool
		ok   bool
	}{
		{"/audio", false, true}, {"/unlabelled", false, true}, {"/a.pls", false, true}, {"/plain.pls", false, true}, {"/a.m3u", false, true}, {"/relative.m3u", false, true}, {"/live.m3u8", true, true},
		{"/page", false, false}, {"/page-as-bytes", false, false}, {"/video", false, false}, {"/json", false, false}, {"/loop.m3u", false, false}, {"/empty.pls", false, false}, {"/huge.m3u", false, false}, {"/gone", false, false}, {"/bad-metaint", false, false},
	} {
		st, err := OpenStation(ctx, ts.URL+tc.path)
		if tc.ok != (err == nil) {
			t.Errorf("%s: %v", tc.path, err)
			continue
		}
		if err != nil {
			continue
		}
		if (st.HLS != nil) != tc.hls {
			t.Errorf("%s: HLS %v", tc.path, st.HLS != nil)
		}
		if !tc.hls {
			b, _ := io.ReadAll(st.Body)
			if !bytes.Contains(b, []byte("mp3")) {
				t.Errorf("%s: got %q", tc.path, b)
			}
		}
		st.Close()
	}
	if _, err := OpenStation(ctx, "file:///etc/passwd"); err == nil {
		t.Error("a file address was opened")
	}
}

func TestIcyMetadataIsTakenOutOfTheAudio(t *testing.T) {
	const metaInt = 10
	block := func(text string) []byte {
		n := (len(text) + 15) / 16
		out := append([]byte{byte(n)}, text...)
		return append(out, make([]byte, n*16-len(text))...)
	}
	var in bytes.Buffer
	in.WriteString("0123456789")
	in.Write(block("StreamTitle='Queen - Don''t Stop';StreamUrl='http://x';"))
	in.WriteString("abcdefghij")
	in.WriteByte(0) // no change
	in.WriteString("ABCDEFGHIJ")
	in.Write(block("StreamTitle='<b>Caf\xe9</b>\x07 evil';"))
	in.WriteString("end")
	var titles []string
	// Reads of awkward sizes must give the same audio.
	for _, size := range []int{1, 3, 7, 4096} {
		titles = nil
		r := NewIcyReader(bytes.NewReader(in.Bytes()), metaInt, func(s string) { titles = append(titles, s) })
		var got bytes.Buffer
		buf := make([]byte, size)
		for {
			n, err := r.Read(buf)
			got.Write(buf[:n])
			if err != nil {
				break
			}
		}
		if got.String() != "0123456789abcdefghijABCDEFGHIJend" {
			t.Errorf("read by %d: %q", size, got.String())
		}
	}
	if len(titles) != 2 || titles[0] != "Queen - Don''t Stop" || titles[1] != "Café evil" {
		t.Errorf("titles: %q", titles)
	}
	// The largest block a station can send, and a cut-off one, are handled.
	big := append([]byte("0123456789"), 255)
	big = append(big, bytes.Repeat([]byte("x"), 255*16)...)
	big = append(big, "tail"...)
	if b, err := io.ReadAll(NewIcyReader(bytes.NewReader(big), metaInt, nil)); err != nil || string(b) != "0123456789tail" {
		t.Errorf("largest block: %q %v", b, err)
	}
	cut := append([]byte("0123456789"), 9, 'x')
	if _, err := io.ReadAll(NewIcyReader(bytes.NewReader(cut), metaInt, nil)); err == nil {
		t.Error("a cut-off block went unnoticed")
	}
	if _, ok := ParseStreamTitle([]byte("StreamUrl='x';")); ok {
		t.Error("a title where there is none")
	}
}

func TestNowPlayingSplit(t *testing.T) {
	if n := SplitNowPlaying("A-ha - Take On Me - Live"); n.Artist != "A-ha" || n.Track != "Take On Me - Live" {
		t.Errorf("%+v", n)
	}
	if n := SplitNowPlaying("Station jingle"); n.Artist != "" || n.Title != "Station jingle" {
		t.Errorf("%+v", n)
	}
	if n := SplitNowPlaying(" - x"); n.Artist != "" {
		t.Errorf("%+v", n)
	}
}

func TestCleanRadioText(t *testing.T) {
	for in, want := range map[string]string{
		"  plain  ":                     "plain",
		"<script>alert(1)</script>Name": "alert(1)Name",
		"two\nlines\t here":             "two lines here",
		"bell\x07\x1b[31m":              "bell[31m",
		"right‮left​":                   "rightleft",
		"bad\xff\xfe":                   "bad",
		"&lt;img src=x onerror=y&gt;":   "<img src=x onerror=y>", // shown as text by the interface
	} {
		if got := CleanRadioText(in, 100); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
	if got := CleanRadioText(strings.Repeat("é", 500), 10); got != strings.Repeat("é", 10) {
		t.Errorf("long: %q", got)
	}
}

// A short line can become a long link, so 2 MB from a station could come to gigabytes.
func TestHLSRewriteStopsGrowing(t *testing.T) {
	base, _ := url.Parse("https://stream.example/live/")
	link := func(string) string { return "/api/radio/1/hls?u=" + strings.Repeat("a", 2500) }
	body := "#EXTM3U\n" + strings.Repeat("a\n", 1<<20)
	if _, _, err := RewriteHLS([]byte(body), base, link); !errors.Is(err, ErrNotAudio) {
		t.Fatalf("%v", err)
	}
	// A long real one still comes through: a day of six-second segments.
	real := "#EXTM3U\n" + strings.Repeat("#EXTINF:6,\nseg.aac\n", 14400)
	if _, _, err := RewriteHLS([]byte(real), base, func(string) string { return "/api/radio/1/hls?u=" + strings.Repeat("a", 200) }); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestHLSRewrite(t *testing.T) {
	base, _ := url.Parse("https://radio.example/live/index.m3u8")
	link := func(abs string) string { return "L(" + abs + ")" }

	// Video variants go; audio stays; every address is rewritten.
	master := `#EXTM3U
#EXT-X-VERSION:4
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="a",URI="audio/index.m3u8"
#EXT-X-MEDIA:TYPE=VIDEO,GROUP-ID="v",URI="v/index.m3u8"
#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=1,URI="iframe.m3u8"
#EXT-X-STREAM-INF:BANDWIDTH=64000,CODECS="mp4a.40.2"
a64.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2000000,CODECS="avc1.64001f,mp4a.40.2",RESOLUTION=1280x720
https://cdn.example/video.m3u8
`
	out, _, err := RewriteHLS([]byte(master), base, link)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{`L(https://radio.example/live/audio/index.m3u8)`, "L(https://radio.example/live/a64.m3u8)\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
	for _, bad := range []string{"video.m3u8", "iframe", "v/index", "avc1"} {
		if strings.Contains(got, bad) {
			t.Errorf("%s came through:\n%s", bad, got)
		}
	}

	// Only video: not a radio station.
	if _, _, err := RewriteHLS([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,RESOLUTION=640x360\nv.m3u8\n"), base, link); !errors.Is(err, ErrNotAudio) {
		t.Errorf("video only: %v", err)
	}
	// Encrypted: refused, never unlocked. A key's address isn't fetched.
	for _, tag := range []string{`#EXT-X-KEY:METHOD=AES-128,URI="k.key"`, `#EXT-X-KEY:METHOD=SAMPLE-AES,URI="skd://x"`, `#EXT-X-SESSION-KEY:METHOD=AES-128,URI="k"`, `#EXT-X-KEY:URI="k"`} {
		if _, _, err := RewriteHLS([]byte("#EXTM3U\n"+tag+"\n#EXTINF:6,\ns.aac\n"), base, link); !errors.Is(err, ErrStationEncrypted) {
			t.Errorf("%s: %v", tag, err)
		}
	}
	if out, _, err := RewriteHLS([]byte("#EXTM3U\n#EXT-X-KEY:METHOD=NONE\n#EXTINF:6,\ns.aac\n"), base, link); err != nil || strings.Contains(string(out), "KEY") {
		t.Errorf("no key: %v %s", err, out)
	}
	// Titles, and addresses that aren't the web.
	media := "#EXTM3U\n#EXTINF:6,title=\"Song <b>One</b>\",artist=\"X\"\ns1.aac\n#EXTINF:6,Artist - Two\ns2.aac\nfile:///etc/passwd\njavascript:alert(1)\n#EXT-X-MAP:URI=\"file:///x\"\n#EXTINF:6,\ns3.aac\n#EXTINF:6,12345\ns4.aac\n"
	out, title, err := RewriteHLS([]byte(media), base, link)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Artist - Two" {
		t.Errorf("title %q", title)
	}
	if strings.Contains(string(out), "file:") || strings.Contains(string(out), "javascript") {
		t.Errorf("unsafe addresses came through:\n%s", out)
	}
	if _, _, err := RewriteHLS([]byte("<html>"), base, link); err == nil {
		t.Error("a page counted as a playlist")
	}
}

func id3Frame(id string, text []byte) []byte {
	var b bytes.Buffer
	b.WriteString(id)
	_ = binary.Write(&b, binary.BigEndian, uint32(len(text)))
	b.Write([]byte{0, 0})
	b.Write(text)
	return b.Bytes()
}

func id3Tag(frames ...[]byte) []byte {
	body := bytes.Join(frames, nil)
	n := len(body)
	head := []byte{'I', 'D', '3', 3, 0, 0, byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}
	return append(head, body...)
}

func TestID3Title(t *testing.T) {
	latin := id3Tag(id3Frame("TPE1", []byte("\x00Caf\xe9")), id3Frame("TIT2", []byte("\x03Song\x00")))
	if got := ID3Title(latin); got != "Café - Song" {
		t.Errorf("%q", got)
	}
	utf16 := id3Tag(id3Frame("TIT2", []byte{1, 0xff, 0xfe, 'H', 0, 'i', 0, 0, 0}))
	if got := ID3Title(utf16); got != "Hi" {
		t.Errorf("%q", got)
	}
	// Sizes that lie, and things that aren't tags at all, give nothing.
	for _, bad := range [][]byte{nil, []byte("ID3"), []byte("not a tag at all"), {'I', 'D', '3', 3, 0, 0, 0x7f, 0x7f, 0x7f, 0x7f}, {'I', 'D', '3', 3, 0, 0, 0, 0, 0, 20, 'T', 'I', 'T', '2', 0xff, 0xff, 0xff, 0xff, 0, 0}, {'I', 'D', '3', 2, 0, 0, 0, 0, 0, 0}} {
		if got := ID3Title(bad); got != "" {
			t.Errorf("%v: %q", bad, got)
		}
	}
}

func TestSegmentTitle(t *testing.T) {
	tag := id3Tag(id3Frame("TPE1", []byte("\x03Band")), id3Frame("TIT2", []byte("\x03Song")))
	emsg := append([]byte{0, 0, 0, 0, 'e', 'm', 's', 'g', 1, 0, 0, 0, 'x', 'y', 'z'}, tag...)
	binary.BigEndian.PutUint32(emsg, uint32(len(emsg)))
	for name, head := range map[string][]byte{"packed": tag, "fragmented": append(emsg, "moofmdat"...)} {
		if got := SegmentTitle(head); got != "Band - Song" {
			t.Errorf("%s: %q", name, got)
		}
	}
	// A box that says it's bigger or smaller than it is gives no trouble.
	for _, bad := range [][]byte{nil, []byte("emsg"), {0, 0, 0, 1, 'e', 'm', 's', 'g'}, {0xff, 0xff, 0xff, 0xff, 'e', 'm', 's', 'g', 'I', 'D', '3'}, []byte("moof............")} {
		if got := SegmentTitle(bad); got != "" {
			t.Errorf("%v: %q", bad, got)
		}
	}
}

func FuzzID3Title(f *testing.F) {
	f.Add(id3Tag(id3Frame("TIT2", []byte("\x03Song"))))
	f.Add([]byte("ID3\x04\x00\x00\x00\x00\x00\x20TIT2\x7f\x7f\x7f\x7f\x00\x00"))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = ID3Title(b), SegmentTitle(b) })
}

func FuzzRewriteHLS(f *testing.F) {
	base, _ := url.Parse("https://radio.example/live/")
	f.Add([]byte("#EXTM3U\n#EXT-X-MEDIA:URI=\"a\"\n#EXTINF:1,t\ns\n"))
	f.Fuzz(func(t *testing.T, b []byte) { _, _, _ = RewriteHLS(b, base, func(s string) string { return s }) })
}

func FuzzParsePlaylist(f *testing.F) {
	f.Add("#EXTM3U\n#EXTINF:-1 group-title=\"x\",n\nhttp://a/\n")
	f.Add("[playlist]\nFile1=http://a/\nTitle1=n\n")
	f.Fuzz(func(t *testing.T, s string) {
		if len(ParsePlaylist(s)) > maxPlaylistStations {
			t.Error("too many")
		}
	})
}

// Old radio servers answer "ICY 200 OK" instead of HTTP.
func TestOldServerAnswers(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				buf := make([]byte, 2048)
				_, _ = c.Read(buf)
				_, _ = c.Write([]byte("ICY 200 OK\r\nicy-name:Old <b>Station</b>\r\nicy-br:128\r\nicy-genre:Rock\r\ncontent-type:audio/mpeg\r\n\r\nmp3 bytes"))
			}()
		}
	}()
	real := SafeClient
	tr := real.Transport.(*http.Transport).Clone()
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		c, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &icyConn{Conn: c}, nil
	}
	SafeClient = &http.Client{Transport: tr}
	defer func() { SafeClient = real }()
	st, err := OpenStation(context.Background(), "http://"+ln.Addr().String()+"/")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	b, _ := io.ReadAll(st.Body)
	if string(b) != "mp3 bytes" {
		t.Errorf("%q", b)
	}
	f := FactsOf(st)
	if f.Name != "Old Station" || f.Bitrate != 128 || f.Genre != "Rock" || f.Codec != "MP3" {
		t.Errorf("%+v", f)
	}
}
