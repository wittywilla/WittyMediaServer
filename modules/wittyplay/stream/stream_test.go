package stream

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image/jpeg"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/wittywilla/wittyplay/ipc"
)

// These tests run the real thing: the wittyplay-host program with its built-in
// test core, a real FFmpeg, and a real WebRTC peer on the other end.

// One build for the whole run. A new program can be held for half a minute the first
// time it starts, by a security scanner on Windows, and that's nothing these test.
var runnerOnce = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "wittyplay-runner-")
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "wittyplay-host")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "../cmd/wittyplay-host")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%w\n%s", err, b)
	}
	return out, nil
})

func buildRunner(t *testing.T) string {
	t.Helper()
	out, err := runnerOnce()
	if err != nil {
		t.Fatalf("building the host: %v", err)
	}
	return out
}

func findFFmpeg(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("WMS_TEST_FFMPEG"); p != "" {
		return p
	}
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no FFmpeg on PATH; set WMS_TEST_FFMPEG to run this")
	}
	return p
}

func haveEncoder(ffmpeg string) func(string) bool {
	out, _ := exec.Command(ffmpeg, "-hide_banner", "-encoders").Output()
	text := string(out)
	return func(name string) bool { return strings.Contains(text, " "+name+" ") }
}

func newManager(t *testing.T) *Manager {
	t.Helper()
	ff := findFFmpeg(t)
	// Normal priority, so a busy machine can't starve a test into a false failure.
	return NewManager(Config{Runner: buildRunner(t), RunnerArgs: []string{"-normal-priority"}, FFmpeg: ff, HaveEncoder: haveEncoder(ff), Log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))})
}

func startGame(t *testing.T, m *Manager, save string, resume bool) *Session {
	t.Helper()
	rom := filepath.Join(t.TempDir(), "game.wittytest")
	if err := os.WriteFile(rom, []byte("a test game"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := m.Start(Options{UserID: 1, ItemID: 7, Title: "Test", Core: ipc.TestCore, ROM: rom, SystemDir: filepath.Dir(rom), SaveDir: save, Resume: resume})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func input(port byte, buttons uint16) []byte {
	b := make([]byte, ipc.InputSize)
	b[0] = port
	binary.LittleEndian.PutUint16(b[1:], buttons)
	return b
}

func TestWebRTCEndToEnd(t *testing.T) {
	m := newManager(t)
	save := t.TempDir()
	s := startGame(t, m, save, false)
	defer s.Stop()
	if info := s.Info(); info.Width != 256 || info.FPS != 60 || info.Core == "" {
		t.Fatalf("unexpected info: %+v", info)
	}

	// The browser's side.
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	for _, kind := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeVideo, webrtc.RTPCodecTypeAudio} {
		if _, err := pc.AddTransceiverFromKind(kind, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
			t.Fatal(err)
		}
	}
	var videoPackets, audioPackets, videoBytes atomic.Int64
	codec := make(chan string, 2)
	pc.OnTrack(func(tr *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		codec <- tr.Codec().MimeType
		buf := make([]byte, 1500)
		for {
			n, _, err := tr.Read(buf)
			if err != nil {
				return
			}
			if tr.Kind() == webrtc.RTPCodecTypeVideo {
				videoPackets.Add(1)
				videoBytes.Add(int64(n))
			} else {
				audioPackets.Add(1)
			}
		}
	})
	inputDC, err := pc.CreateDataChannel("input", &webrtc.DataChannelInit{Ordered: boolp(false), MaxRetransmits: uint16p(0)})
	if err != nil {
		t.Fatal(err)
	}
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	<-gathered
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	answer, err := s.Answer(ctx, pc.LocalDescription().SDP, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 20*time.Second, "video and audio to arrive", func() bool { return videoPackets.Load() > 30 && audioPackets.Load() > 10 })
	t.Logf("codecs: %s %s; video %d packets, %d bytes; audio %d packets; encoder %s", <-codec, <-codec, videoPackets.Load(), videoBytes.Load(), audioPackets.Load(), s.Info().Encoder)

	// Input reaches the game through the data channel: Start counts up in the
	// test core's save memory.
	waitFor(t, 10*time.Second, "the input channel", func() bool { return inputDC.ReadyState() == webrtc.DataChannelStateOpen })
	if err := inputDC.Send(input(0, 1<<3)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := inputDC.Send(input(0, 0)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	if err := s.SaveState(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(save, "state-1.state")); err != nil || st.Size() == 0 {
		t.Fatalf("no state file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(save, "state-1.png")); err != nil {
		t.Fatalf("no state picture: %v", err)
	}
	if err := s.LoadState(1); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadState(5); err == nil {
		t.Fatal("loading an empty slot should fail")
	}

	s.Stop()
	if _, err := os.Stat(filepath.Join(save, "auto.state")); err != nil {
		t.Fatalf("stopping should leave a resume state: %v", err)
	}
	ram, err := os.ReadFile(filepath.Join(save, "save.srm"))
	if err != nil {
		t.Fatalf("stopping should write the battery save: %v", err)
	}
	if binary.LittleEndian.Uint32(ram) != 1 {
		t.Fatalf("Start presses in save memory = %d, want 1: input did not arrive", binary.LittleEndian.Uint32(ram))
	}
	if m.Count() != 0 {
		t.Fatal("the session should be gone")
	}

	// Resuming restores both.
	s2 := startGame(t, m, save, true)
	defer s2.Stop()
	if !s2.Info().Resumed {
		t.Fatal("the second session should have resumed")
	}
}

func boolp(b bool) *bool       { return &b }
func uint16p(n uint16) *uint16 { return &n }

func waitFor(t *testing.T, d time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestWebSocketFallbacks(t *testing.T) {
	m := newManager(t)
	s := startGame(t, m, t.TempDir(), false)
	defer s.Stop()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.ServeWS(w, r, r.URL.Query().Get("mode"))
	}))
	defer srv.Close()

	for _, mode := range []string{"h264", "jpeg"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			c, _, err := websocket.Dial(ctx, srv.URL+"?mode="+mode, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close(websocket.StatusNormalClosure, "")
			c.SetReadLimit(1 << 24)
			var video, audio int
			firstKey := false
			for video < 20 || audio < 5 {
				typ, b, err := c.Read(ctx)
				if err != nil {
					t.Fatalf("after %d video and %d audio messages: %v", video, audio, err)
				}
				if typ != websocket.MessageBinary || len(b) < 2 {
					continue
				}
				switch b[0] {
				case wsH264:
					if video == 0 {
						firstKey = b[1] == 1
						if !bytes.HasPrefix(b[2:], []byte{0, 0, 0, 1}) {
							t.Fatal("expected Annex B")
						}
					}
					video++
				case wsJPEG:
					img, err := jpeg.Decode(bytes.NewReader(b[1:]))
					if err != nil {
						t.Fatal(err)
					}
					if img.Bounds().Dx() != 256 {
						t.Fatalf("JPEG width %d", img.Bounds().Dx())
					}
					firstKey = true
					video++
				case wsPCM:
					if binary.LittleEndian.Uint32(b[1:]) != 44100 {
						t.Fatal("wrong sample rate")
					}
					audio++
				}
			}
			if !firstKey {
				t.Fatal("a viewer's first frame must be one its decoder can start on")
			}
		})
	}
}

// A viewer who connects while a menu has paused the game still gets a picture, and
// the game stays paused.
func TestViewerJoiningWhilePaused(t *testing.T) {
	m := newManager(t)
	s := startGame(t, m, t.TempDir(), false)
	defer s.Stop()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.ServeWS(w, r, "jpeg")
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	read := func(c *websocket.Conn) {
		t.Helper()
		for {
			typ, b, err := c.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if typ == websocket.MessageBinary && b[0] == wsJPEG {
				return
			}
		}
	}
	first, _, err := websocket.Dial(ctx, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	first.SetReadLimit(1 << 24)
	read(first)
	if err := s.Pause(true); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	second, _, err := websocket.Dial(ctx, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	second.SetReadLimit(1 << 24)
	defer second.Close(websocket.StatusNormalClosure, "")
	read(second)
	first.Close(websocket.StatusNormalClosure, "")
	s.viewMu.Lock()
	paused := s.paused
	s.viewMu.Unlock()
	if !paused {
		t.Fatal("a new viewer must not unpause a game a menu has paused")
	}
}

// Repeats of the same state heal lost packets, but don't count as someone playing:
// otherwise an abandoned tab would keep a game running forever.
func TestRepeatedInputIsNotActivity(t *testing.T) {
	m := newManager(t)
	s := startGame(t, m, t.TempDir(), false)
	defer s.Stop()
	s.viewMu.Lock()
	long := time.Now().Add(-time.Hour)
	s.lastInput = long
	s.viewMu.Unlock()
	s.Input(input(0, 0)) // the state it already has
	s.viewMu.Lock()
	if !s.lastInput.Equal(long) {
		t.Error("a repeat should not count as activity")
	}
	s.viewMu.Unlock()
	s.Input(input(0, 1<<3))
	s.viewMu.Lock()
	if s.lastInput.Equal(long) {
		t.Error("a change should count as activity")
	}
	s.viewMu.Unlock()
}

func TestModes(t *testing.T) {
	m := NewManager(Config{HaveEncoder: func(n string) bool { return n == "libvpx" }})
	got := m.Modes(ClientNative)
	if len(got) != 2 || got[0].ID != "webrtc" || got[0].Video[0] != "vp8" || got[1].ID != "ws-jpeg" {
		t.Fatalf("with only VP8: %+v", got)
	}
	m = NewManager(Config{HaveEncoder: func(string) bool { return false }})
	if got := m.Modes(ClientBrowser); len(got) != 1 || got[0].ID != "ws-jpeg" {
		t.Fatalf("with no encoder at all, JPEG still works: %+v", got)
	}
	m = NewManager(Config{HaveEncoder: func(n string) bool { return n == "libx264" }})
	if got := m.Modes(ClientBrowser); len(got) != 3 || got[0].Video[0] != "h264" {
		t.Fatalf("with H.264: %+v", got)
	}
}

func TestBadInputIgnored(t *testing.T) {
	m := newManager(t)
	s := startGame(t, m, t.TempDir(), false)
	defer s.Stop()
	s.Input(nil)
	s.Input(make([]byte, 5))
	s.Input(input(200, 0xffff)) // no such player
	if _, err := s.Answer(context.Background(), strings.Repeat("x", MaxOfferSize+1), ""); err == nil {
		t.Fatal("an oversized offer should be refused")
	}
	if _, err := SlotName(10); err == nil {
		t.Fatal("slot 10 should not exist")
	}
	if _, err := SlotName(-1); err == nil {
		t.Fatal("slot -1 should not exist")
	}
}

func TestSessionLimit(t *testing.T) {
	m := newManager(t)
	m.Configure(func(c *Config) { c.MaxSessions = 1 })
	a := startGame(t, m, t.TempDir(), false)
	defer a.Stop()
	rom := filepath.Join(t.TempDir(), "g.wittytest")
	_ = os.WriteFile(rom, []byte("x"), 0o644)
	if _, err := m.Start(Options{UserID: 2, ItemID: 8, Core: ipc.TestCore, ROM: rom, SaveDir: t.TempDir()}); err != ErrBusy {
		t.Fatalf("got %v, want ErrBusy", err)
	}
	// The same account starting another game replaces its first one.
	b, err := m.Start(Options{UserID: 1, ItemID: 9, Core: ipc.TestCore, ROM: rom, SaveDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Stop()
	if m.Get(a.ID()) != nil {
		t.Fatal("the first session should have ended")
	}
}

func TestBadGameFailsCleanly(t *testing.T) {
	m := newManager(t)
	if _, err := m.Start(Options{UserID: 1, ItemID: 1, Core: ipc.TestCore, ROM: filepath.Join(t.TempDir(), "missing"), SaveDir: t.TempDir()}); err == nil {
		t.Fatal("a missing game should fail")
	}
	if m.Count() != 0 {
		t.Fatal("no session should remain")
	}
}

// A touch goes from a client to the core: the test core draws a white cross where
// the pointer is pressed, and a JPEG viewer sees it.
func TestPointerEndToEnd(t *testing.T) {
	m := newManager(t)
	s := startGame(t, m, t.TempDir(), false)
	defer s.Stop()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.ServeWS(w, r, "jpeg") }))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	// Three quarters across and a quarter down.
	press := []byte{ipc.PointerMark, 1, 0, 0, 0, 0}
	binary.LittleEndian.PutUint16(press[2:], uint16(int16(16383)))
	neg := int16(-16383)
	binary.LittleEndian.PutUint16(press[4:], uint16(neg))
	if err := c.Write(ctx, websocket.MessageBinary, press); err != nil {
		t.Fatal(err)
	}
	white := func(x, y int) bool {
		for {
			_, b, err := c.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(b) < 2 || b[0] != wsJPEG {
				continue
			}
			img, err := jpeg.Decode(bytes.NewReader(b[1:]))
			if err != nil {
				continue
			}
			r, g, b2, _ := img.At(x, y).RGBA()
			return r>>8 > 200 && g>>8 > 200 && b2>>8 > 200
		}
	}
	found := false
	for i := 0; i < 300 && !found; i++ {
		found = white(192, 56)
	}
	if !found {
		t.Fatal("the touch never showed up at the place it was made")
	}
	press[1] = 0
	if err := c.Write(ctx, websocket.MessageBinary, press); err != nil {
		t.Fatal(err)
	}
	gone := false
	for i := 0; i < 300 && !gone; i++ {
		gone = !white(192, 56)
	}
	if !gone {
		t.Fatal("the touch never ended")
	}
}

func TestGPULimit(t *testing.T) {
	ff := findFFmpeg(t)
	m := NewManager(Config{Runner: buildRunner(t), FFmpeg: ff, HaveEncoder: haveEncoder(ff), MaxGPU: 1, MaxSessions: 4})
	rom := filepath.Join(t.TempDir(), "game.wittytest")
	if err := os.WriteFile(rom, []byte("a test game"), 0o644); err != nil {
		t.Fatal(err)
	}
	opt := func(user int64, gpu bool) Options {
		return Options{UserID: user, ItemID: 7, Title: "Test", Core: ipc.TestCore, ROM: rom, SystemDir: filepath.Dir(rom), SaveDir: t.TempDir(), GPU: gpu}
	}
	a, err := m.Start(opt(1, true))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	if _, err := m.Start(opt(2, true)); !errors.Is(err, ErrGPUBusy) {
		t.Fatalf("a second game on the card should be refused, got %v", err)
	}
	b, err := m.Start(opt(3, false))
	if err != nil {
		t.Fatalf("a software-rendered game doesn't use the card: %v", err)
	}
	defer b.Stop()
}

func TestGPUGamesPreferHardwareEncoders(t *testing.T) {
	m := NewManager(Config{HaveEncoder: func(string) bool { return true }, HWEncoders: []string{"h264_qsv"}, GPUEncoders: []string{"h264_nvenc", "h264_qsv"}})
	gpu := &Session{m: m, cfg: m.config(), opt: Options{GPU: true}}
	if got := gpu.chain("h264"); got[0] != "h264_nvenc" || got[1] != "h264_qsv" {
		t.Errorf("GPU game's encoders: %v", got)
	}
	soft := &Session{m: m, cfg: m.config()}
	if got := soft.chain("h264"); got[0] != "h264_qsv" {
		t.Errorf("software game's encoders: %v", got)
	}
}

// A game can change resolution twice in a moment, so an encoder can be stopped
// before it has made a frame. That says nothing against the encoder.
func TestStoppingAnEncoderEarlyIsNoFailure(t *testing.T) {
	ff := findFFmpeg(t)
	has := haveEncoder(ff)
	name := "libx264"
	if !has(name) {
		t.Skip("this FFmpeg has no libx264")
	}
	m := NewManager(Config{FFmpeg: ff, HaveEncoder: has})
	s := &Session{m: m, cfg: m.cfg, log: m.log}
	e, err := s.startVideoEncoder("h264", name, ipc.VideoHeader{Width: 64, Height: 64, Format: 1}, 60)
	if err != nil {
		t.Fatal(err)
	}
	e.stop()
	<-e.stopped
	if m.badEnc[name] {
		t.Fatal("the encoder was blamed for being stopped")
	}
}
