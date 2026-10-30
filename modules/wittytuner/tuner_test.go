package wittytuner

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittytuner/mpegts"
	"github.com/wittywilla/wittytuner/xmltv"
)

func TestMain(m *testing.M) {
	// The custom command tests run this test binary as their "tuner".
	if os.Getenv("WITTYTUNER_TEST_TS") != "" {
		out := bytes.Repeat(tsPacket(0x100, os.Getenv("WITTYTUNER_TEST_TS") == "scrambled"), 3000)
		_, _ = os.Stdout.Write(out)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func tsPacket(pid int, scrambled bool) []byte {
	p := bytes.Repeat([]byte{0x47}, 188)
	p[1], p[2], p[3] = byte(pid>>8), byte(pid), 0x10
	if scrambled {
		p[3] |= 0x80
	}
	return p
}

const samplePlaylist = `#EXTM3U url-tvg="http://guide.test/epg.xml.gz,http://other.test/x"
#EXTINF:-1 tvg-id="one.test" tvg-chno="12" tvg-logo="http://logo.test/1.png" group-title="News, Weather",News One, Live
#EXTVLCOPT:http-user-agent=Thing
http://stream.test/one.m3u8

#EXTINF:-1 tvg-name="Fallback",
http://stream.test/two.ts
#KODIPROP:inputstream.adaptive.license_type=com.widevine.alpha
#EXTINF:-1,Locked Channel
https://stream.test/locked.mpd
#EXTINF:-1,Bad
file:///etc/passwd
rtsp://camera.test/1
`

func TestParseM3U(t *testing.T) {
	m, err := ParseM3U(strings.NewReader(samplePlaylist))
	if err != nil {
		t.Fatal(err)
	}
	if m.GuideURL != "http://guide.test/epg.xml.gz" {
		t.Fatalf("guide %q", m.GuideURL)
	}
	if len(m.Entries) != 5 {
		t.Fatalf("%d entries: %+v", len(m.Entries), m.Entries)
	}
	a := m.Entries[0]
	if a.Name != "News One, Live" || a.EPGID != "one.test" || a.Number != "12" || a.Group != "News, Weather" || a.Logo != "http://logo.test/1.png" || a.URL != "http://stream.test/one.m3u8" {
		t.Fatalf("%+v", a)
	}
	if m.Entries[1].Name != "Fallback" {
		t.Fatalf("%+v", m.Entries[1])
	}
	if m.Entries[2].Encrypted == "" {
		t.Fatal("a stream with a DRM licence line must be marked")
	}
	if m.Entries[1].Encrypted != "" || m.Entries[3].Encrypted != "" {
		t.Fatal("the marker must not carry over to other channels")
	}
}

func TestStreamURLRules(t *testing.T) {
	good := []string{"http://a.test/x", "https://a.test/x", "rtsp://a.test/x", "udp://239.0.0.1:1234", "rtp://239.0.0.1:1234", "http://192.168.1.5/x"}
	for _, u := range good {
		if err := checkStreamURL(u, false); err != nil {
			t.Errorf("%s: %v", u, err)
		}
	}
	bad := []string{"file:///etc/passwd", "concat:a|b", "subfile,,start,0,end,0,,:/etc/passwd", "gopher://a.test", "data:text/plain,hi", "http://169.254.169.254/latest/meta-data", "http://localhost/x", "http://127.0.0.1/x", "http://[::1]/x"}
	for _, u := range bad {
		if checkStreamURL(u, false) == nil {
			t.Errorf("%s should be refused", u)
		}
	}
	if err := checkStreamURL("http://127.0.0.1/x", true); err != nil {
		t.Error("the owner's own local playlist may name this machine")
	}
	if checkStreamURL("http://169.254.169.254/", true) == nil {
		t.Error("metadata addresses are never allowed")
	}
}

func TestIPTVFromAddressAndFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); ua != "Agent/1" {
			t.Errorf("user agent %q", ua)
		}
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		_, _ = gz.Write([]byte(samplePlaylist))
		_ = gz.Close()
		_, _ = w.Write(b.Bytes())
	}))
	defer srv.Close()
	s, err := New("iptv", Config{Name: "P", Options: map[string]string{"playlist": srv.URL, "userAgent": "Agent/1"}}, Env{})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := s.Channels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ch) != 4 { // file:/// is dropped
		t.Fatalf("%+v", ch)
	}
	if s.Info().GuideURL != "http://guide.test/epg.xml.gz" {
		t.Fatalf("guide %q", s.Info().GuideURL)
	}
	path := filepath.Join(t.TempDir(), "p.m3u")
	if err := os.WriteFile(path, []byte(samplePlaylist), 0o600); err != nil {
		t.Fatal(err)
	}
	s2, err := New("iptv", Config{Options: map[string]string{"playlist": path}}, Env{})
	if err != nil {
		t.Fatal(err)
	}
	if ch, err := s2.Channels(context.Background()); err != nil || len(ch) != 4 {
		t.Fatalf("%v %+v", err, ch)
	}
	if _, err := New("iptv", Config{Options: map[string]string{"playlist": "ftp://x.test/a"}}, Env{}); err == nil {
		t.Fatal("ftp playlist")
	}
	if _, err := New("iptv", Config{Options: map[string]string{}}, Env{}); err == nil {
		t.Fatal("a playlist is required")
	}
}

func openStream(t *testing.T, url string) (*Stream, error) {
	t.Helper()
	s, err := New("stream", Config{Name: "S", Options: map[string]string{"url": url}}, Env{})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := s.Channels(context.Background())
	if err != nil || len(ch) != 1 {
		t.Fatalf("%v %+v", err, ch)
	}
	return s.Open(context.Background(), ch[0])
}

func TestOpenHTTPKinds(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/clear.m3u8", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-TARGETDURATION:4\n#EXTINF:4,\nseg1.ts\n")
	})
	mux.HandleFunc("/aes.m3u8", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"k\"\n#EXTINF:4,\nseg1.ts\n")
	})
	mux.HandleFunc("/drm.m3u8", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-KEY:METHOD=SAMPLE-AES,KEYFORMAT=\"com.apple.streamingkeydelivery\",URI=\"skd://x\"\n#EXTINF:4,\nseg1.ts\n")
	})
	mux.HandleFunc("/none.m3u8", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-KEY:METHOD=NONE\n#EXTINF:4,\nseg1.ts\n")
	})
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\naes.m3u8\n")
	})
	mux.HandleFunc("/live.ts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat(tsPacket(0x100, false), 200))
	})
	mux.HandleFunc("/locked.ts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat(tsPacket(0x100, true), 2000))
	})
	mux.HandleFunc("/busy", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })
	mux.HandleFunc("/gone", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/video.mp4", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("....ftypisom")) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	st, err := openStream(t, srv.URL+"/clear.m3u8")
	if err != nil || st.Input == nil {
		t.Fatalf("clear HLS: %v %+v", err, st)
	}
	joined := strings.Join(st.Input, " ")
	if strings.Contains(joined, "crypto") {
		t.Fatalf("FFmpeg must not be able to decrypt: %s", joined)
	}
	if !strings.Contains(joined, "-protocol_whitelist http,https,tcp,tls") {
		t.Fatalf("no whitelist: %s", joined)
	}

	for _, p := range []string{"/aes.m3u8", "/drm.m3u8", "/master.m3u8"} {
		_, err = openStream(t, srv.URL+p)
		var e *EncryptedError
		if !errors.As(err, &e) {
			t.Errorf("%s should be refused as encrypted, got %v", p, err)
		}
	}
	if _, err := openStream(t, srv.URL+"/none.m3u8"); err != nil {
		t.Errorf("METHOD=NONE is not encrypted: %v", err)
	}

	st, err = openStream(t, srv.URL+"/live.ts")
	if err != nil || st.Reader == nil || st.Guard == nil {
		t.Fatalf("raw TS goes through the guard: %v %+v", err, st)
	}
	data, err := io.ReadAll(st.Reader)
	if err != nil || len(data) != 200*188 {
		t.Fatalf("%v, %d bytes", err, len(data))
	}
	st.Close()

	st, err = openStream(t, srv.URL+"/locked.ts")
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(st.Reader)
	if !IsEncrypted(err) {
		t.Fatalf("a scrambled stream must stop: %v", err)
	}
	if !strings.Contains(RefusalMessage("Ch", err).Error(), "does not decrypt") {
		t.Fatal(RefusalMessage("Ch", err))
	}
	st.Close()

	if _, err := openStream(t, srv.URL+"/busy"); !errors.Is(err, ErrBusy) {
		t.Fatalf("503: %v", err)
	}
	if _, err := openStream(t, srv.URL+"/gone"); err == nil {
		t.Fatal("404")
	}
	st, err = openStream(t, srv.URL+"/video.mp4")
	if err != nil || st.Input == nil {
		t.Fatalf("other formats go to FFmpeg: %v", err)
	}
}

func TestDRMEntryRefusedBeforeAnyRequest(t *testing.T) {
	s, _ := New("stream", Config{Options: map[string]string{"url": "http://never.test/x"}}, Env{})
	_, err := s.Open(context.Background(), Channel{Name: "L", Locator: "http://never.test/x", Encrypted: "the playlist lists a DRM licence for it"})
	var e *EncryptedError
	if !errors.As(err, &e) {
		t.Fatalf("%v", err)
	}
}

// mockDevice stands in for a network tuner.
func mockDevice(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	polls := 0
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/discover.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"FriendlyName":"Box","ModelNumber":"M1","DeviceID":"1234ABCD","TunerCount":2,"BaseURL":%q}`, srv.URL)
	})
	mux.HandleFunc("/lineup.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"GuideNumber":"5.1","GuideName":"WKID","HD":1,"VideoCodec":"MPEG2","URL":"%[1]s/auto/v5.1"},
			{"GuideNumber":"7.1","GuideName":"LOCK","DRM":1,"URL":"%[1]s/auto/v7.1"},
			{"GuideNumber":"9.1","GuideName":"BUSY","URL":"%[1]s/auto/v9.1"},
			{"GuideNumber":"11.1","GuideName":"AWAY","URL":"http://203.0.113.9/auto/v11.1"}]`, srv.URL)
	})
	mux.HandleFunc("/lineup.post", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Query().Get("scan") != "start" || r.URL.Query().Get("source") != "Cable" {
			t.Errorf("scan request: %s %s", r.Method, r.URL)
		}
	})
	mux.HandleFunc("/lineup_status.json", func(w http.ResponseWriter, r *http.Request) {
		polls++
		if polls < 3 {
			fmt.Fprintf(w, `{"ScanInProgress":1,"Progress":%d,"Found":%d}`, polls*30, polls)
			return
		}
		fmt.Fprint(w, `{"ScanInProgress":0}`)
	})
	mux.HandleFunc("/auto/v5.1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat(tsPacket(0x100, false), 100))
	})
	mux.HandleFunc("/auto/v9.1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-HDHomeRun-Error", "805 All Tuners In Use")
		w.WriteHeader(503)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &polls
}

func TestNetworkTuner(t *testing.T) {
	srv, polls := mockDevice(t)
	scanPoll = time.Millisecond
	s, err := New("network", Config{Name: "Box", Options: map[string]string{"address": srv.URL}}, Env{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ch, err := s.Channels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ch) != 4 || !ch[0].HD || ch[0].Name != "WKID" || ch[0].Number != "5.1" {
		t.Fatalf("%+v", ch)
	}
	if s.Info().MaxStreams != 2 || !strings.Contains(s.Info().Detail, "M1") {
		t.Fatalf("%+v", s.Info())
	}
	if ch[1].Encrypted == "" {
		t.Fatal("DRM: 1 must mark the channel")
	}
	st, err := s.Open(ctx, ch[0])
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(st.Reader)
	st.Close()
	if len(data) != 100*188 {
		t.Fatalf("%d bytes", len(data))
	}
	_, err = s.Open(ctx, ch[1])
	var e *EncryptedError
	if !errors.As(err, &e) {
		t.Fatalf("copy protected channel: %v", err)
	}
	if _, err = s.Open(ctx, ch[2]); !errors.Is(err, ErrBusy) {
		t.Fatalf("busy: %v", err)
	}
	if _, err = s.Open(ctx, ch[3]); err == nil || !strings.Contains(err.Error(), "another address") {
		t.Fatalf("a lineup must not send the server elsewhere: %v", err)
	}
	var seen []Progress
	got, err := s.Scan(ctx, ScanOptions{Source: "Cable"}, func(p Progress) { seen = append(seen, p) })
	if err != nil || len(got) != 4 || *polls != 3 || len(seen) != 3 || seen[1].Percent != 60 {
		t.Fatalf("scan: %v, %d channels, %d polls, %+v", err, len(got), *polls, seen)
	}
}

func TestDeviceBase(t *testing.T) {
	cases := map[string]string{"192.168.1.5": "http://192.168.1.5", "http://box:5004/": "http://box:5004", "https://x.test/a/b": "https://x.test"}
	for in, want := range cases {
		if got, err := deviceBase(in); err != nil || got != want {
			t.Errorf("%s: %s %v", in, got, err)
		}
	}
	if _, err := deviceBase("ftp://x"); err == nil {
		t.Error("ftp")
	}
}

func TestHDHRDiscoveryPackets(t *testing.T) {
	req := hdhrDiscoverRequest()
	if binary.BigEndian.Uint16(req) != hdhrDiscoverReq {
		t.Fatal("type")
	}
	if binary.LittleEndian.Uint32(req[len(req)-4:]) != crc32.ChecksumIEEE(req[:len(req)-4]) {
		t.Fatal("crc")
	}
	url := "http://192.168.1.5:80"
	tags := []byte{hdhrTagType, 4, 0, 0, 0, 1, hdhrTagID, 4, 0x12, 0x34, 0x56, 0x78, hdhrTagTuners, 1, 3, hdhrTagBaseURL, byte(len(url))}
	tags = append(tags, url...)
	r, ok := parseHDHRReply(hdhrPacket(hdhrDiscoverRes, tags))
	if !ok || r.id != 0x12345678 || r.tuners != 3 || r.baseURL != url {
		t.Fatalf("%+v %v", r, ok)
	}
	bad := hdhrPacket(hdhrDiscoverRes, tags)
	bad[6] ^= 1
	if _, ok := parseHDHRReply(bad); ok {
		t.Fatal("a damaged reply must be dropped")
	}
	if _, ok := parseHDHRReply([]byte{0, 3}); ok {
		t.Fatal("short")
	}
}

func TestSSDPLocation(t *testing.T) {
	msg := "HTTP/1.1 200 OK\r\nST: x\r\nLocation: http://192.168.1.9:8875/desc.xml\r\n\r\n"
	if got := ssdpLocation([]byte(msg)); got != "http://192.168.1.9:8875/desc.xml" {
		t.Fatal(got)
	}
	if got := ssdpLocation([]byte("LOCATION: file:///etc/passwd\r\n")); got != "" {
		t.Fatal(got)
	}
}

func TestSATIP(t *testing.T) {
	s := &satip{host: "192.168.1.60"}
	q, err := checkTuning("src=1&freq=11362&pol=h&msys=dvbs2&sr=22000&fec=23")
	if err != nil {
		t.Fatal(err)
	}
	u := s.tuneURL(q, "0,17,18")
	if !strings.HasPrefix(u, "rtsp://192.168.1.60/?") || !strings.Contains(u, "freq=11362") || !strings.HasSuffix(u, "&pids=0,17,18") {
		t.Fatal(u)
	}
	if _, err := checkTuning("freq=1&x=2"); err == nil {
		t.Fatal("unknown settings must be refused")
	}
	if _, err := checkTuning("src=1"); err == nil {
		t.Fatal("no frequency")
	}
	if _, err := New("satip", Config{Options: map[string]string{"server": "a/b"}}, Env{}); err == nil {
		t.Fatal("bad host")
	}
	_, err = (&satip{}).Open(context.Background(), Channel{Encrypted: "x"})
	var e *EncryptedError
	if !errors.As(err, &e) {
		t.Fatal(err)
	}
}

func TestBandPlans(t *testing.T) {
	atsc := bandPlans["atsc"](0)
	if atsc[0].Frequency != 57000000 || atsc[len(atsc)-1].Frequency != 605000000 {
		t.Fatalf("ATSC RF 2 and 36: %d %d", atsc[0].Frequency, atsc[len(atsc)-1].Frequency)
	}
	// RF 7 and RF 14 are known fixed points.
	if atsc[5].Frequency != 177000000 || atsc[12].Frequency != 473000000 {
		t.Fatalf("%d %d", atsc[5].Frequency, atsc[12].Frequency)
	}
	dvbt := bandPlans["dvbt"](8000000)
	if dvbt[0].Frequency != 474000000 || dvbt[0].Bandwidth != 8000000 {
		t.Fatalf("%+v", dvbt[0])
	}
}

func TestLocatorRoundTrip(t *testing.T) {
	in := Tuning{System: "dvbs2", Frequency: 11362000, SymbolRate: 22000000, Pol: "h"}
	got, prog, err := ParseLocator(in.Locator(7))
	if err != nil || got != in || prog != 7 {
		t.Fatalf("%+v %d %v", got, prog, err)
	}
	if _, _, err := ParseLocator("nonsense"); err == nil {
		t.Fatal("bad locator")
	}
}

func TestChannelsConf(t *testing.T) {
	text := `[CHANNEL]
	# comment
[News One]
	DELIVERY_SYSTEM = ATSC
	VCHANNEL = 5.1
	SERVICE_ID = 3
	FREQUENCY = 177000000
[Locked]
	FREQUENCY = 177000000
	SERVICE_ID = 4
	CA_ID = 256
`
	got := ParseChannelsConf("atsc", text)
	if len(got) != 2 || got[0].Name != "News One" || got[0].Number != "5.1" || got[0].Program != 3 || got[0].Encrypted != "" {
		t.Fatalf("%+v", got)
	}
	if got[1].Encrypted == "" {
		t.Fatal("CA_ID must mark the channel")
	}
	zap := ParseChannelsConf("atsc", "KIDS:177000000:8VSB:49:52:3\n")
	if len(zap) != 1 || zap[0].Name != "KIDS" || zap[0].Program != 3 {
		t.Fatalf("%+v", zap)
	}
	tp, err := parseTransponders("dvbs2", "11362000 h 22000000\n# note\n12187000 v 27500000\n")
	if err != nil || len(tp) != 2 || tp[1].Pol != "v" || tp[1].SymbolRate != 27500000 {
		t.Fatalf("%v %+v", err, tp)
	}
	if _, err := parseTransponders("dvbs2", "abc"); err == nil {
		t.Fatal("bad line")
	}
	if _, err := parseTransponders("dvbc", ""); err == nil {
		t.Fatal("empty list")
	}
}

func section(table byte, ext int, body []byte) []byte {
	s := []byte{table, 0xb0, 0, byte(ext >> 8), byte(ext), 0xc1, 0, 0}
	s = append(s, body...)
	n := len(s) - 3 + 4
	s[2] = byte(n)
	s[1] |= byte(n >> 8)
	crc := mpegts.CRC32(s)
	return append(s, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
}

func secPackets(pid int, sec []byte) []byte {
	p := bytes.Repeat([]byte{0xff}, 188)
	p[0], p[1], p[2], p[3], p[4] = 0x47, 0x40|byte(pid>>8), byte(pid), 0x10, 0
	copy(p[5:], sec)
	return p
}

func TestScanMux(t *testing.T) {
	pat := section(0, 1, []byte{0, 3, 0xe1, 0x00, 0, 4, 0xe2, 0x00})
	clear := section(2, 3, []byte{0xe1, 0x01, 0xf0, 0, 0x1b, 0xe1, 0x01, 0xf0, 0, 0x0f, 0xe1, 0x02, 0xf0, 0})
	enc := section(2, 4, []byte{0xe2, 0x01, 0xf0, 6, 0x09, 4, 1, 0, 0xe3, 0, 0x02, 0xe2, 0x01, 0xf0, 0})
	var data []byte
	for i := 0; i < 2; i++ {
		data = append(data, secPackets(0, pat)...)
		data = append(data, secPackets(0x100, clear)...)
		data = append(data, secPackets(0x200, enc)...)
	}
	got := scanMux(context.Background(), bytes.NewReader(data), time.Second)
	if len(got) != 2 || got[0].Program != 3 || !got[0].HD || got[0].Encrypted != "" || got[1].Encrypted == "" {
		t.Fatalf("%+v", got)
	}
}

func TestCaptureParsing(t *testing.T) {
	v, a := ParseDShowDevices(`[dshow @ 000001] "USB Capture" (video)
[dshow @ 000001]   Alternative name "@device_pnp_x"
[dshow @ 000001] "Line In (USB)" (audio)
`)
	if len(v) != 1 || v[0] != "USB Capture" || len(a) != 1 || a[0] != "Line In (USB)" {
		t.Fatalf("%v %v", v, a)
	}
	v, a = ParseDShowDevices(`[dshow @ 01] DirectShow video devices (some may be both video and audio devices)
[dshow @ 01]  "Old Card"
[dshow @ 01]     Alternative name "@x"
[dshow @ 01] DirectShow audio devices
[dshow @ 01]  "Old Sound"
`)
	if len(v) != 1 || v[0] != "Old Card" || len(a) != 1 || a[0] != "Old Sound" {
		t.Fatalf("old format: %v %v", v, a)
	}
	v, a = ParseAVFDevices(`[AVFoundation indev @ 0x7f] AVFoundation video devices:
[AVFoundation indev @ 0x7f] [0] FaceTime Camera
[AVFoundation indev @ 0x7f] [1] USB Capture
[AVFoundation indev @ 0x7f] AVFoundation audio devices:
[AVFoundation indev @ 0x7f] [0] Built-in Mic
`)
	if len(v) != 2 || v[1] != "USB Capture" || len(a) != 1 {
		t.Fatalf("%v %v", v, a)
	}
	if checkDeviceName("-f lavfi") == nil || checkDeviceName("a\nb") == nil || checkDeviceName(`a"b`) == nil {
		t.Fatal("names that could be read as options must be refused")
	}
	c := &capture{cfg: Config{Options: map[string]string{"video": "USB Capture", "audio": "Line In"}}, driver: "dshow"}
	if got := strings.Join(c.input(), " "); !strings.Contains(got, "-i video=USB Capture:audio=Line In") {
		t.Fatal(got)
	}
	c = &capture{cfg: Config{Options: map[string]string{"video": "USB", "audio": "Mic"}}, driver: "avfoundation"}
	if got := strings.Join(c.input(), " "); !strings.Contains(got, "-i USB:Mic") {
		t.Fatal(got)
	}
}

func TestV4L2(t *testing.T) {
	if _, _, err := frequencyLine("nope"); err == nil {
		t.Fatal("bad line")
	}
	v := &v4l2{cfg: Config{Name: "Card", Options: map[string]string{"device": "/dev/video1", "audio": "hw:1,0", "standard": "ntsc", "channels": "Channel 5=77.25\nChannel 7=175.25"}}}
	ch, err := v.Channels(context.Background())
	if err != nil || len(ch) != 2 || ch[1].Name != "Channel 7" || ch[1].Locator != "175.250" {
		t.Fatalf("%v %+v", err, ch)
	}
	got := strings.Join(v.input(), " ")
	for _, want := range []string{"-f v4l2", "-standard NTSC", "-i /dev/video1", "-f alsa", "-i hw:1,0"} {
		if !strings.Contains(got, want) {
			t.Errorf("%s lacks %s", got, want)
		}
	}
	v = &v4l2{cfg: Config{Options: map[string]string{"device": "/dev/video0"}}}
	if ch, _ := v.Channels(context.Background()); len(ch) != 1 {
		t.Fatalf("a capture input is one channel: %+v", ch)
	}
}

func TestHTTPBackend(t *testing.T) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/playlist/channels.m3u", func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "me" || p != "pw" {
			w.WriteHeader(401)
			return
		}
		fmt.Fprintf(w, "#EXTM3U\n#EXTINF:-1 tvg-id=\"a\",A\nhttp://user:pass@%[1]s/stream/1\n#EXTINF:-1,Elsewhere\nhttp://203.0.113.9/x\n", srv.Listener.Addr())
	})
	mux.HandleFunc("/stream/1", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); !ok {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write(bytes.Repeat(tsPacket(0x100, false), 50))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	s, err := New("backend", Config{Options: map[string]string{"protocol": "http-m3u", "address": srv.URL, "username": "me", "password": "pw"}}, Env{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(s.Info().GuideURL, "/xmltv/channels") {
		t.Fatal(s.Info().GuideURL)
	}
	ch, err := s.Channels(context.Background())
	if err != nil || len(ch) != 1 || ch[0].EPGID != "a" {
		t.Fatalf("%v %+v", err, ch)
	}
	if strings.Contains(ch[0].Locator, "pass") {
		t.Fatalf("passwords must not be kept in addresses: %s", ch[0].Locator)
	}
	st, err := s.Open(context.Background(), ch[0])
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(st.Reader); len(b) != 50*188 {
		t.Fatalf("%d bytes", len(b))
	}
	st.Close()
	bad, _ := New("backend", Config{Options: map[string]string{"protocol": "http-m3u", "address": srv.URL, "username": "me", "password": "wrong"}}, Env{})
	if _, err := bad.Channels(context.Background()); err == nil || !strings.Contains(err.Error(), "password") {
		t.Fatalf("%v", err)
	}
}

func TestSVDRPBackend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no local network")
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		fmt.Fprint(c, "220 host SVDRP VideoDiskRecorder 2.6.0; ready\r\n")
		buf := make([]byte, 64)
		_, _ = c.Read(buf)
		fmt.Fprint(c, "250-1 News;Pack:11362:hC23M2S1:S19.2E:22000:255:256:0:0:10301:1:1019:0\r\n")
		fmt.Fprint(c, "250-2 Locked;Pack:11362:hC23M2S1:S19.2E:22000:255:256:0:1702:10302:1:1019:0\r\n")
		fmt.Fprint(c, "250 3 Radio,R;Pack:11362:hC23M2S1:S19.2E:22000:0:256:0:0:10303:1:1019:0\r\n")
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	s, err := New("backend", Config{Options: map[string]string{"protocol": "svdrp", "address": "127.0.0.1", "svdrpPort": port}}, Env{})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := s.Channels(context.Background())
	if err != nil || len(ch) != 3 {
		t.Fatalf("%v %+v", err, ch)
	}
	if ch[0].Name != "News" || ch[0].Group != "Pack" || ch[0].ID != "S19.2E-1-1019-10301" || !strings.HasSuffix(ch[0].Locator, ":3000/TS/S19.2E-1-1019-10301") {
		t.Fatalf("%+v", ch[0])
	}
	if ch[1].Encrypted == "" {
		t.Fatal("a CA system must mark the channel")
	}
	if !ch[2].Radio || ch[2].Name != "Radio" {
		t.Fatalf("%+v", ch[2])
	}
}

func TestCustomCommand(t *testing.T) {
	cmd := fmt.Sprintf("%q", os.Args[0])
	opts := map[string]string{"command": cmd + " --tuner={number}", "channels": "5, News, news\n7, Other"}
	if _, err := New("command", Config{Options: opts}, Env{}); err == nil {
		t.Fatal("custom commands must be off by default")
	}
	for _, d := range Drivers(Env{}) {
		if d.ID == "command" && (d.Available || !strings.Contains(d.Note, "WMS_TUNER_COMMANDS")) {
			t.Fatalf("%+v", d)
		}
	}
	env := Env{AllowCommands: true}
	s, err := New("command", Config{Options: opts}, env)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := s.Channels(context.Background())
	if err != nil || len(ch) != 2 || ch[0].ID != "news" || ch[1].ID != "7" {
		t.Fatalf("%v %+v", err, ch)
	}
	t.Setenv("WITTYTUNER_TEST_TS", "clear")
	st, err := s.Open(context.Background(), ch[0])
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(st.Reader)
	if err != nil || len(b) != 3000*188 {
		t.Fatalf("%v, %d bytes", err, len(b))
	}
	st.Close()
	t.Setenv("WITTYTUNER_TEST_TS", "scrambled")
	st, err = s.Open(context.Background(), ch[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(st.Reader); !IsEncrypted(err) {
		t.Fatalf("%v", err)
	}
	st.Close()
}

func TestSplitCommand(t *testing.T) {
	got, err := SplitCommand(`prog "a b" 'c d' e`)
	if err != nil || strings.Join(got, "|") != "prog|a b|c d|e" {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := SplitCommand(`prog "open`); err == nil {
		t.Fatal("open quote")
	}
}

func TestDriversDescribeThemselves(t *testing.T) {
	want := map[string]bool{"iptv": false, "stream": false, "network": false, "satip": false, "dvb": false, "v4l2": false, "dshow": false, "avfoundation": false, "bda": false, "backend": false, "command": false}
	for _, d := range Drivers(Env{}) {
		if _, ok := want[d.ID]; !ok {
			t.Errorf("unexpected driver %s", d.ID)
		}
		want[d.ID] = true
		if d.Name == "" || d.Description == "" || d.Status == "" {
			t.Errorf("%s describes itself poorly: %+v", d.ID, d)
		}
	}
	for id, seen := range want {
		if !seen {
			t.Errorf("driver %s missing", id)
		}
	}
	if len(SecretFields("backend")) != 1 {
		t.Fatal("the backend's password is a secret")
	}
	if _, err := New("nope", Config{}, Env{}); err == nil {
		t.Fatal("unknown driver")
	}
	if _, err := New("bda", Config{}, Env{}); err == nil {
		t.Fatal("a planned driver cannot be used")
	}
}

func TestXMLTVTogether(t *testing.T) {
	doc := `<?xml version="1.0"?><tv>
<channel id="one.test"><display-name>News One</display-name><display-name>1</display-name><icon src="http://i.test/1.png"/></channel>
<programme start="20261030200000 +0100" stop="20261030210000 +0100" channel="one.test">
<title lang="en">Evening &amp; News</title><sub-title>Tonight</sub-title><desc>Stories.</desc><category>News</category>
<date>2026</date><episode-num system="xmltv_ns">1 . 4 . 0/1</episode-num><rating system="x"><value>TV-G</value></rating></programme>
<programme start="garbage" channel="one.test"><title>Bad</title></programme>
<programme start="202610302100" channel="one.test"><title>No end</title><episode-num system="onscreen">S02E05</episode-num></programme>
</tv>`
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	_, _ = gz.Write([]byte(doc))
	_ = gz.Close()
	for _, src := range []io.Reader{strings.NewReader(doc), &b} {
		var chs []xmltv.Channel
		var ps []xmltv.Programme
		err := xmltv.Parse(src, func(c xmltv.Channel) bool { chs = append(chs, c); return true }, func(p xmltv.Programme) bool { ps = append(ps, p); return true })
		if err != nil {
			t.Fatal(err)
		}
		if len(chs) != 1 || chs[0].Names[0] != "News One" || chs[0].Icon == "" {
			t.Fatalf("%+v", chs)
		}
		if len(ps) != 2 {
			t.Fatalf("%+v", ps)
		}
		p := ps[0]
		if p.Title != "Evening & News" || p.Sub != "Tonight" || p.Desc != "Stories." || p.Category != "News" || p.Rating != "TV-G" || *p.Season != 2 || *p.Episode != 5 || p.Year != 2026 {
			t.Fatalf("%+v", p)
		}
		if !p.Start.Equal(time.Date(2026, 10, 30, 19, 0, 0, 0, time.UTC)) || !p.Stop.Equal(time.Date(2026, 10, 30, 20, 0, 0, 0, time.UTC)) {
			t.Fatalf("times: %v %v", p.Start, p.Stop)
		}
		if !ps[1].Stop.IsZero() || !ps[1].Start.Equal(time.Date(2026, 10, 30, 21, 0, 0, 0, time.UTC)) || *ps[1].Season != 2 {
			t.Fatalf("%+v", ps[1])
		}
	}
}
