package stream

import (
	"bytes"
	"context"
	"encoding/binary"
	"image/jpeg"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/wittywilla/wittyplay/ipc"
)

// These tests run the real thing: the wittyplay-host program with its built-in
// test core, a real FFmpeg, and a real WebRTC peer on the other end.

func buildRunner(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "wittyplay-host")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "../cmd/wittyplay-host")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the host: %v\n%s", err, b)
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
	return NewManager(Config{Runner: buildRunner(t), FFmpeg: ff, HaveEncoder: haveEncoder(ff), Log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))})
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
