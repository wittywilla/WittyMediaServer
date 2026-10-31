package modules

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittytuner"
	"github.com/wittywilla/wittytuner/mpegts"
)

func tunerManifest(ver string) []byte {
	b, _ := json.Marshal(wittymodule.Manifest{ID: tunerID, Name: "WittyTuner", Version: ver, Repo: "wittywilla/WittyTuner", Delivery: wittymodule.DeliveryHelper,
		Entry: "wittytuner-module", API: 2, MinServer: "0.1.0", MaxServer: "0.x", Permissions: []string{wittymodule.PermissionNetwork}})
	return b
}

// fakeTuners has a version of WittyTuner in use whose helper behaves as a tuner's
// "behave" setting says.
func fakeTuners(t *testing.T) (Tuners, *Manager) {
	t.Helper()
	w := newWorld(t)
	m := withInstalled(t, w, tunerManifest("0.1.0"), tunerManifest("0.2.0"), programBytes(t, "./testdata/faketuner"), nil)
	t.Cleanup(m.Close)
	if s := m.Report().Modules[0]; s.Version != "0.2.0" {
		t.Fatalf("%+v", s)
	}
	return Tuners{m}, m
}

func tsBytes(n int, scrambled bool) []byte {
	p := bytes.Repeat([]byte{0x47}, 188)
	p[1], p[2], p[3] = 0x01, 0x00, 0x10
	if scrambled {
		p[3] |= 0x80
	}
	return bytes.Repeat(p, n)
}

// streamSite is what a built-in copy reaches for itself: /ts is a clear stream, /hls
// clear HLS, and /locked encrypted HLS.
func streamSite(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/ts", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(tsBytes(500, false)) })
	mux.HandleFunc("/hls", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nseg.ts\n")
	})
	mux.HandleFunc("/locked", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key\"\n#EXTINF:6,\nseg.ts\n")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func streamSource(t *testing.T, tn Tuners, addr, behave string, more ...string) (wittytuner.Source, wittytuner.Channel) {
	t.Helper()
	opts := map[string]string{"url": addr, "behave": behave}
	for i := 0; i+1 < len(more); i += 2 {
		opts[more[i]] = more[i+1]
	}
	s, err := tn.New("stream", wittytuner.Config{Name: "Test", Options: opts}, wittytuner.Env{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*tunerProxy); !ok {
		t.Fatalf("not through a helper: %T", s)
	}
	return s, wittytuner.Channel{ID: "x", Number: "1", Name: "Test", Locator: addr}
}

func failures(m *Manager) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mods[tunerID].fails
}

// However a helper describes a stream, the server's own guard reads it and refuses it.
func TestTunerScrambledFromHelperIsRefused(t *testing.T) {
	tn, _ := fakeTuners(t)
	s, ch := streamSource(t, tn, streamSite(t).URL+"/ts", "scrambled")
	st, err := s.Open(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if st.Guard == nil || st.Verified {
		t.Fatalf("%+v", st)
	}
	_, err = io.Copy(io.Discard, st.Reader)
	if !errors.Is(err, mpegts.ErrScrambled) || !wittytuner.IsEncrypted(err) {
		t.Fatalf("it went through: %v", err)
	}
	if st.Guard.Fatal() == nil || st.Guard.Verified() {
		t.Fatal("the guard doesn't know")
	}
}

// A helper saying a stream was checked means nothing. Only what the guard saw does.
func TestTunerVerifiedFromHelperIsIgnored(t *testing.T) {
	tn, _ := fakeTuners(t)
	s, ch := streamSource(t, tn, streamSite(t).URL+"/ts", "clear")
	st, err := s.Open(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if st.Verified || st.Input != nil {
		t.Fatalf("%+v", st)
	}
	if st.Guard == nil || st.Guard.Verified() {
		t.Fatal("clear before anything was read")
	}
	if _, err := io.Copy(io.Discard, st.Reader); err != nil {
		t.Fatal(err)
	}
	if !st.Guard.Verified() {
		t.Fatal("the guard saw it all go by, and should say so")
	}
}

// What a helper offers for FFmpeg to open never gets to FFmpeg. What it offers that
// isn't a plain address counts against it, and the built-in copy opens the channel.
func TestTunerUnsafeInputIsRefused(t *testing.T) {
	site := streamSite(t)
	addr := site.URL + "/hls"
	for _, behave := range []string{"input-options", "input-file", "input-concat", "input-crypto", "input-empty"} {
		tn, m := fakeTuners(t)
		s, ch := streamSource(t, tn, addr, behave)
		before := failures(m)
		st, err := s.Open(context.Background(), ch)
		if err != nil {
			t.Fatalf("%s: %v", behave, err)
		}
		line := strings.Join(st.Input, " ")
		if !st.Verified || strings.Contains(line, "file") || strings.Contains(line, "crypto") || !strings.Contains(line, addr) || !strings.HasSuffix(line, "-i "+addr) {
			t.Fatalf("%s: %+v", behave, st)
		}
		if failures(m) != before+1 {
			t.Fatalf("%s: it wasn't counted against the version", behave)
		}
	}
	tn, m := fakeTuners(t)
	for range failLimit {
		s, ch := streamSource(t, tn, addr, "input-file")
		_, _ = s.Open(context.Background(), ch)
	}
	if st := m.Report().Modules[0]; st.Version != "0.1.0" || !strings.Contains(st.Notice, "refused") {
		t.Fatalf("%+v", st)
	}
}

func TestPlainInputURL(t *testing.T) {
	for _, in := range [][]string{{"-i", "http://a.test/x"}, {"-i", "https://a.test/x?y=1"}, {"-i", "udp://239.0.0.1:1234"}, {"-i", "rtp://239.0.0.1:1234"}} {
		if _, err := plainInputURL(in); err != nil {
			t.Errorf("%v: %v", in, err)
		}
	}
	for _, in := range [][]string{nil, {}, {"-i"}, {"http://a.test/x"}, {"-i", ""}, {"-f", "http://a.test/x"},
		{"-f", "lavfi", "-i", "http://a.test/x"}, {"-i", "http://a.test/x", "-i", "file:///etc/passwd"},
		{"-i", "file:///etc/passwd"}, {"-i", "/etc/passwd"}, {"-i", "concat:http://a/1|file:///x"}, {"-i", "http://a.test/x|file:///etc/passwd"},
		{"-i", "crypto+http://a.test/x"}, {"-i", "hls+http://a.test/x"}, {"-i", "subfile,,start,0,end,0,,:/etc/passwd"}, {"-i", "rtsp://a.test/x"},
		{"-i", "tcp://a.test:1"}, {"-i", "http://user:pass@a.test/x"}, {"-i", "http://a.test/x y"}, {"-i", "data:text/plain,x"}, {"-i", "pipe:0"}} {
		if u, err := plainInputURL(in); err == nil {
			t.Errorf("%v passed as %q", in, u)
		}
	}
}

// A plain address is still opened and checked here, not on the helper's word. What it
// says about an address the built-in copy wouldn't use for this channel isn't taken.
func TestTunerPlainInputIsCheckedHere(t *testing.T) {
	tn, m := fakeTuners(t)
	site := streamSite(t)
	s, ch := streamSource(t, tn, site.URL+"/hls", "input-plain")
	st, err := s.Open(context.Background(), ch)
	if err != nil || !st.Verified || st.Input[len(st.Input)-1] != ch.Locator || !strings.Contains(strings.Join(st.Input, " "), "-protocol_whitelist http,https,tcp,tls") {
		t.Fatalf("%+v %v", st, err)
	}
	if failures(m) != 0 {
		t.Fatal("a plain address isn't a failure")
	}
	// It says "plain, and verified" about encrypted HLS. The server looks for itself.
	s, ch = streamSource(t, tn, site.URL+"/locked", "input-plain")
	if st, err := s.Open(context.Background(), ch); !wittytuner.IsEncrypted(err) {
		t.Fatalf("%+v %v", st, err)
	}
	// An address that isn't this channel's.
	s, ch = streamSource(t, tn, site.URL+"/hls", "input-elsewhere")
	st, err = s.Open(context.Background(), ch)
	if err != nil || st.Input[len(st.Input)-1] != ch.Locator {
		t.Fatalf("%+v %v", st, err)
	}
	if failures(m) != 1 {
		t.Fatal("it wasn't counted against the version")
	}
}

// A helper dying during a stream ends that stream and no other.
func TestTunerHelperCrashEndsOnlyThatStream(t *testing.T) {
	tn, _ := fakeTuners(t)
	site := streamSite(t)
	long, lch := streamSource(t, tn, site.URL+"/ts", "forever")
	a, err := long.Open(context.Background(), lch)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	buf := make([]byte, 188*20)
	if _, err := io.ReadFull(a.Reader, buf); err != nil {
		t.Fatal(err)
	}
	dying, dch := streamSource(t, tn, site.URL+"/ts", "dies-midstream")
	b, err := dying.Open(context.Background(), dch)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, b.Reader)
		done <- err
	}()
	select {
	case <-done: // it ended, and its Release can run as for any stream that stops
	case <-time.After(30 * time.Second):
		t.Fatal("the stream didn't end with its helper")
	}
	for range 5 {
		if _, err := io.ReadFull(a.Reader, buf); err != nil {
			t.Fatalf("the other stream ended too: %v", err)
		}
	}
	// Ending it is quick, and leaves nothing running.
	begin := time.Now()
	a.Close()
	if d := time.Since(begin); d > 5*time.Second {
		t.Fatalf("closing took %v", d)
	}
	_ = b.Close()
	m := tn.M
	m.mu.Lock()
	left := len(m.attached)
	m.mu.Unlock()
	if left != 0 {
		t.Fatalf("%d helpers left", left)
	}
}

// As with the built-in copy, a stream lasts only as long as its context.
func TestTunerStreamEndsWithItsContext(t *testing.T) {
	tn, _ := fakeTuners(t)
	s, ch := streamSource(t, tn, streamSite(t).URL+"/ts", "forever")
	ctx, cancel := context.WithCancel(context.Background())
	st, err := s.Open(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, st.Reader); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("it went on")
	}
}

// A version that fails, or can't do something, is passed over for the built-in copy.
func TestTunerFallsBackToBuiltin(t *testing.T) {
	tn, m := fakeTuners(t)
	addr := streamSite(t).URL + "/ts"
	for _, behave := range []string{"crash", "denied", "garbage", "dies-listing", "empty", "error-doc"} {
		s, _ := streamSource(t, tn, addr, behave)
		list, err := s.Channels(context.Background())
		if err != nil || len(list) != 1 || list[0].Locator != addr || list[0].Name != "Test" {
			t.Errorf("%s: channels: %+v %v", behave, list, err)
		}
		list, err = s.Scan(context.Background(), wittytuner.ScanOptions{}, nil)
		if err != nil || len(list) != 1 {
			t.Errorf("%s: scan: %+v %v", behave, list, err)
		}
		if st := m.Report().Modules[0]; st.Version == "0.1.0" {
			break // gone out of use, as it should once they add up: the rest is the built-in copy
		}
	}
	tn, m = fakeTuners(t)
	for _, behave := range []string{"denied", "dies-opening"} {
		s, ch := streamSource(t, tn, addr, behave)
		st, err := s.Open(context.Background(), ch)
		if err != nil {
			t.Fatalf("%s: %v", behave, err)
		}
		n, err := io.Copy(io.Discard, st.Reader)
		st.Close()
		if err != nil || n != 500*188 {
			t.Fatalf("%s: %d %v", behave, n, err)
		}
	}
	if failures(m) != 1 {
		t.Fatalf("only the one that died counts: %d", failures(m))
	}

	// The built-in copy in use: no helper is started, and the source works just the same.
	w := newWorld(t)
	none := New(Config{Dir: w.dir, Server: w.server, Builtins: [][]byte{tunerManifest("0.1.0")}, Keys: nil})
	defer none.Close()
	s, ch := streamSource(t, Tuners{none}, addr, "crash")
	st, err := s.Open(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if n, _ := io.Copy(io.Discard, st.Reader); n != 500*188 {
		t.Fatal(n)
	}
	none.mu.Lock()
	n := len(none.attached)
	none.mu.Unlock()
	if n != 0 {
		t.Fatal("a helper with the built-in copy in use")
	}
}

// What a helper can say that refuses something is taken as said.
func TestTunerHelperMayRefuse(t *testing.T) {
	tn, m := fakeTuners(t)
	addr := streamSite(t).URL + "/ts"
	s, ch := streamSource(t, tn, addr, "encrypted")
	var enc *wittytuner.EncryptedError
	if _, err := s.Open(context.Background(), ch); !errors.As(err, &enc) || enc.Channel != "Test" || !strings.Contains(enc.Why, "said so") {
		t.Fatalf("%v", err)
	}
	s, ch = streamSource(t, tn, addr, "busy")
	if _, err := s.Open(context.Background(), ch); !errors.Is(err, wittytuner.ErrBusy) {
		t.Fatalf("%v", err)
	}
	if failures(m) != 0 {
		t.Fatal("a working helper saying no isn't a failure")
	}
	// A channel already known to be protected never reaches a helper.
	s, ch = streamSource(t, tn, addr, "crash")
	ch.Encrypted = "it is known"
	if _, err := s.Open(context.Background(), ch); !wittytuner.IsEncrypted(err) || failures(m) != 0 {
		t.Fatalf("%v", err)
	}
}

// A list from a helper is cleaned: nothing it says makes a channel known to be clear.
func TestTunerChannelsAreChecked(t *testing.T) {
	tn, _ := fakeTuners(t)
	addr := streamSite(t).URL + "/ts"
	s, _ := streamSource(t, tn, addr, "", "guideFromHelper", "file:///etc/passwd")
	list, err := s.Channels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != "clear" || list[1].ID != "protected" {
		t.Fatalf("%+v", list)
	}
	if list[0].Checked || list[0].Encrypted != "" || list[0].Logo != "" {
		t.Fatalf("%+v", list[0])
	}
	if list[1].Encrypted == "" {
		t.Fatalf("%+v", list[1])
	}
	i := s.Info()
	if i.Driver != "stream" || i.Name != "Test" || i.Detail != "seen by the helper" || i.MaxStreams != 3 || i.GuideURL != "" {
		t.Fatalf("%+v", i)
	}
}

// A playlist's guide address may come from a helper, if it is an address.
func TestTunerGuideAddress(t *testing.T) {
	tn, _ := fakeTuners(t)
	site := streamSite(t)
	mk := func(guide string) wittytuner.Info {
		s, err := tn.New("iptv", wittytuner.Config{Name: "Test", Options: map[string]string{"playlist": site.URL + "/list", "url": site.URL + "/ts", "guideFromHelper": guide}}, wittytuner.Env{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Channels(context.Background()); err != nil {
			t.Fatal(err)
		}
		return s.Info()
	}
	if i := mk("https://guide.test/x.xml"); i.GuideURL != "https://guide.test/x.xml" {
		t.Fatalf("%+v", i)
	}
	if i := mk("file:///etc/passwd"); i.GuideURL != "" {
		t.Fatalf("%+v", i)
	}
}

// A scan's progress arrives as it goes, tidied up.
func TestTunerScanProgress(t *testing.T) {
	tn, _ := fakeTuners(t)
	addr := streamSite(t).URL + "/ts"
	s, _ := streamSource(t, tn, addr, "")
	var seen []wittytuner.Progress
	list, err := s.Scan(context.Background(), wittytuner.ScanOptions{}, func(p wittytuner.Progress) { seen = append(seen, p) })
	if err != nil || len(list) != 2 {
		t.Fatalf("%+v %v", list, err)
	}
	if len(seen) != 2 || seen[0].Percent != 100 || seen[0].Found != 0 || seen[0].Message != "half" || seen[1].Percent != 100 {
		t.Fatalf("%+v", seen)
	}
}

// A scan under way stops when it is cancelled, and a helper that goes quiet can't hang it.
func TestTunerScanCancels(t *testing.T) {
	tn, m := fakeTuners(t)
	addr := streamSite(t).URL + "/ts"
	s, _ := streamSource(t, tn, addr, "progress-then-hang")
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan wittytuner.Progress, 1)
	done := make(chan error, 1)
	go func() {
		_, err := s.Scan(ctx, wittytuner.ScanOptions{}, func(p wittytuner.Progress) { got <- p })
		done <- err
	}()
	select {
	case p := <-got:
		if p.Percent != 40 || p.Found != 2 {
			t.Fatalf("%+v", p)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no progress")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("it went on")
	}
	if failures(m) != 0 {
		t.Fatal("being cancelled isn't failing")
	}
	// A list that takes too long goes to the built-in copy.
	s, _ = streamSource(t, tn, addr, "hang")
	short, stop := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer stop()
	if _, err := s.Channels(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

// What a helper can't do stays with the built-in copy whichever version is in use.
func TestTunerWhatStaysBuiltin(t *testing.T) {
	tn, m := fakeTuners(t)
	for driver, opts := range map[string]map[string]string{
		"satip":   {"server": "192.168.1.60"},
		"command": {"command": "prog"},
	} {
		s, err := tn.New(driver, wittytuner.Config{Name: "X", Options: opts}, wittytuner.Env{AllowCommands: true})
		if err != nil {
			t.Fatalf("%s: %v", driver, err)
		}
		if _, ok := s.(*tunerProxy); ok {
			t.Errorf("%s goes through a helper", driver)
		}
	}
	// An address that takes FFmpeg never asks the helper, here one that would crash.
	s, _ := streamSource(t, tn, "rtsp://192.168.1.60/x", "crash")
	if _, err := s.Open(context.Background(), wittytuner.Channel{ID: "x", Name: "X", Locator: "rtsp://192.168.1.60/x"}); err == nil || !strings.Contains(err.Error(), "FFmpeg") {
		t.Fatalf("%v", err)
	}
	if failures(m) != 0 {
		t.Fatal("the helper was asked")
	}
	// Settings are checked by the built-in copy, not by what a helper would allow.
	if _, err := tn.New("stream", wittytuner.Config{Options: map[string]string{"url": "file:///etc/passwd"}}, wittytuner.Env{}); err == nil {
		t.Fatal("a file as a stream")
	}
}

// Devices are looked for here and the network through the helper, and it can only
// suggest the kinds of tuner that are on a network.
func TestTunerDiscover(t *testing.T) {
	tn, _ := fakeTuners(t)
	found := tn.Discover(context.Background(), wittytuner.Env{})
	names := map[string]bool{}
	for _, f := range found {
		names[f.Name] = true
		if f.Driver == "command" {
			t.Fatalf("%+v", f)
		}
	}
	if !names["From the helper"] {
		t.Fatalf("%+v", found)
	}
}

// The real helper, in its sandbox, against a stand-in for a network tuner.
func TestRealTunerHelper(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "modules", "wittytuner", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	real, err := wittymodule.ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if real.Delivery != wittymodule.DeliveryHelper || !real.Asks(wittymodule.PermissionNetwork) {
		t.Fatalf("%+v", real)
	}
	old := bytes.Replace(raw, []byte(`"version": "`+real.Version+`"`), []byte(`"version": "0.0.1"`), 1)
	w := newWorld(t)
	w.server = real.MinServer
	m := withInstalled(t, w, old, raw, programBytes(t, "github.com/wittywilla/wittytuner/cmd/wittytuner-module"), nil)
	defer m.Close()
	if s := m.Report().Modules[0]; s.Version != real.Version || s.State != StateInstalled {
		t.Fatalf("%+v", s)
	}

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/discover.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"FriendlyName":"Box","ModelNumber":"M","DeviceID":"1","TunerCount":4}`)
	})
	mux.HandleFunc("/lineup.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"GuideNumber":"5.1","GuideName":"Clear","URL":"%[1]s/ok"},{"GuideNumber":"7.1","GuideName":"Tagged","Tags":"drm","URL":"%[1]s/ok"},
			{"GuideNumber":"9.1","GuideName":"Sneaky","URL":"%[1]s/scrambled"},{"GuideNumber":"11.1","GuideName":"Busy","URL":"%[1]s/busy"},{"GuideNumber":"13.1","GuideName":"Off","URL":"http://other.test/x"}]`, srv.URL)
	})
	mux.HandleFunc("/lineup.post", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/lineup_status.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ScanInProgress":0,"Progress":100,"Found":3}`)
	})
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(tsBytes(3000, false)) })
	mux.HandleFunc("/scrambled", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(tsBytes(3000, true)) })
	mux.HandleFunc("/busy", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })
	mux.HandleFunc("/hls", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "#EXTM3U\n#EXTINF:6,\nseg.ts\n") })
	srv = httptest.NewServer(mux)
	defer srv.Close()

	tn := Tuners{m}
	s, err := tn.New("network", wittytuner.Config{Name: "Box", Options: map[string]string{"address": srv.URL}}, wittytuner.Env{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	list, err := s.Channels(ctx)
	if err != nil || failures(m) != 0 {
		t.Fatalf("%+v %v %d", list, err, failures(m))
	}
	if s.Info().MaxStreams != 4 || !strings.Contains(s.Info().Detail, "Box M") {
		t.Fatalf("what it learned about the device: %+v", s.Info())
	}
	byName := map[string]wittytuner.Channel{}
	for _, c := range list {
		byName[c.Name] = c
		if c.Checked {
			t.Fatalf("%+v", c)
		}
	}
	if len(list) != 4 || byName["Tagged"].Encrypted == "" {
		t.Fatalf("%+v", list)
	}
	var seen int
	if scanned, err := s.Scan(ctx, wittytuner.ScanOptions{}, func(wittytuner.Progress) { seen++ }); err != nil || len(scanned) != 4 || seen < 2 {
		t.Fatalf("%+v %v %d", scanned, err, seen)
	}

	st, err := s.Open(ctx, byName["Clear"])
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(io.Discard, st.Reader)
	if err != nil || n != 3000*188 || !st.Guard.Verified() || st.Verified {
		t.Fatalf("%d %v", n, err)
	}
	st.Close()

	st, err = s.Open(ctx, byName["Sneaky"])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, st.Reader); !errors.Is(err, mpegts.ErrScrambled) || st.Guard.Fatal() == nil {
		t.Fatalf("the helper's stream went through: %v", err)
	}
	st.Close()

	if _, err := s.Open(ctx, byName["Busy"]); !errors.Is(err, wittytuner.ErrBusy) {
		t.Fatal(err)
	}
	// The channel known to be protected is refused without a helper being asked.
	tagged := byName["Tagged"]
	if _, err := s.Open(ctx, tagged); !wittytuner.IsEncrypted(err) {
		t.Fatal(err)
	}

	// HLS is opened by the server, on the real helper's suggestion.
	one, err := tn.New("stream", wittytuner.Config{Name: "One", Options: map[string]string{"url": srv.URL + "/hls"}}, wittytuner.Env{})
	if err != nil {
		t.Fatal(err)
	}
	st, err = one.Open(ctx, wittytuner.Channel{ID: "x", Name: "One", Locator: srv.URL + "/hls"})
	if err != nil || !st.Verified || st.Input[len(st.Input)-1] != srv.URL+"/hls" {
		t.Fatalf("%+v %v", st, err)
	}
	if failures(m) != 0 {
		t.Fatalf("%d failures: %+v", failures(m), m.Report().Modules[0].Events)
	}
	// Nothing is left running between questions.
	m.mu.Lock()
	left := len(m.attached)
	m.mu.Unlock()
	if left != 0 {
		t.Fatalf("%d helpers left", left)
	}
}
