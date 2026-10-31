package relay

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/wittywilla/wittycouch/relay/fakehost"
)

func TestMain(m *testing.M) {
	if os.Getenv(fakehost.Env) != "" {
		fakehost.Run()
		return
	}
	os.Exit(m.Run())
}

func fakeOptions(t *testing.T, mode string) Options {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Options{Program: self, Prepare: func(c *exec.Cmd) { c.Env = append(c.Env, fakehost.Env+"="+mode) },
		Start: Start{Address: "127.0.0.1", AppVersion: "7.1.431.-1", RTSP: "rtsp://127.0.0.1:48010", Width: 1280, Height: 720, FPS: 60, BitrateKbps: 5000, PacketSize: 1024}}
}

func TestStreamStartsAndEnds(t *testing.T) {
	var ended atomic.Bool
	opts := fakeOptions(t, "ok")
	opts.OnEnd = func(string) { ended.Store(true) }
	s, err := New(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if v, a := s.Counts(); v > 3 && a > 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if v, a := s.Counts(); v == 0 || a == 0 {
		t.Errorf("frames %d, audio %d", v, a)
	}
	if info := s.Info(); info.Width != 1280 || info.FPS != 60 {
		t.Errorf("info %+v", info)
	}
	s.Close()
	select {
	case <-s.Ended():
	case <-time.After(5 * time.Second):
		t.Fatal("it didn't end")
	}
	if !ended.Load() {
		t.Error("nobody was told")
	}
	select {
	case <-s.host.Done():
	case <-time.After(5 * time.Second):
		t.Error("the program is still running")
	}
}

func TestStreamFailures(t *testing.T) {
	for mode, want := range map[string]string{"fail": "RTSP handshake", "die": "stopped before", "flood": "stopped before"} {
		_, err := New(context.Background(), fakeOptions(t, mode))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", mode, err)
		}
	}
	opts := fakeOptions(t, "ok")
	opts.Program = ""
	if _, err := New(context.Background(), opts); err != ErrNoProgram {
		t.Errorf("no program: %v", err)
	}
	opts.Program = "/no/such/program"
	if _, err := New(context.Background(), opts); err == nil {
		t.Error("a missing program started")
	}
}

func TestOnlyInputMessagesPass(t *testing.T) {
	good := [][]byte{
		append([]byte{CmdPad}, make([]byte, 18)...), {CmdMouseMove, 1, 0, 2, 0}, {CmdMouseButton, 7, 1}, {CmdKey, 0x41, 0x80, 3, 0}, {CmdScroll, 120, 0}, {CmdIDR},
		append([]byte{CmdMousePosition}, make([]byte, 8)...), {CmdHScroll, 1, 0}, {CmdText, 'h', 'i'},
	}
	for _, m := range good {
		if !ValidInput(m) {
			t.Errorf("refused %v", m)
		}
	}
	bad := [][]byte{
		nil, {}, {cmdStart, 'x', '=', '1'}, {cmdQuit}, {0}, {1}, {255}, {CmdPad}, append([]byte{CmdPad}, make([]byte, 19)...), {CmdMouseMove, 1}, {CmdKey, 1, 2, 3}, {CmdIDR, 1},
		{CmdText}, append([]byte{CmdText}, make([]byte, 257)...),
	}
	for _, m := range bad {
		if ValidInput(m) {
			t.Errorf("allowed %v", m)
		}
	}
}

func FuzzValidInput(f *testing.F) {
	f.Add([]byte{CmdPad, 1, 2})
	f.Fuzz(func(t *testing.T, m []byte) {
		if ValidInput(m) && (m[0] == cmdStart || m[0] == cmdQuit) {
			t.Fatal("a control message passed as input")
		}
	})
}

func TestStartMessageHasWhatItNeeds(t *testing.T) {
	s := Start{Address: "192.168.1.2", AppVersion: "7", RTSP: "rtspenc://192.168.1.2:48010", Width: 1280, Height: 720, FPS: 60, BitrateKbps: 1000, PacketSize: 1392, Remote: true}
	s.Key[0], s.IV[15] = 0xab, 0xcd
	m := string(s.message())
	for _, want := range []string{"address=192.168.1.2\n", "rtsp=rtspenc://192.168.1.2:48010\n", "remote=1\n", "formats=1\n", "rikey=ab000000000000000000000000000000\n", "riiv=000000000000000000000000000000cd\n"} {
		if !strings.Contains(m, want) {
			t.Errorf("no %q in %q", want, m)
		}
	}
}

// A viewer, as a real WebRTC client: media arrives, and input goes back through to the program.
func TestViewerGetsMediaAndSendsInput(t *testing.T) {
	s, err := New(context.Background(), fakeOptions(t, "ok"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		t.Fatal(err)
	}
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		t.Fatal(err)
	}
	control, err := pc.CreateDataChannel("control", nil)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	got := map[string]int{}
	var rumbles atomic.Int32
	control.OnMessage(func(m webrtc.DataChannelMessage) {
		if strings.Contains(string(m.Data), `"rumble"`) {
			rumbles.Add(1)
		}
	})
	opened := make(chan struct{})
	control.OnOpen(func() { close(opened) })
	pc.OnTrack(func(tr *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		for {
			if _, _, err := tr.ReadRTP(); err != nil {
				return
			}
			mu.Lock()
			got[tr.Kind().String()]++
			mu.Unlock()
		}
	})
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	<-gathered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	answer, err := s.Offer(ctx, pc.LocalDescription().SDP)
	if err != nil {
		t.Fatal(err)
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-opened:
	case <-ctx.Done():
		t.Fatal("no connection")
	}
	// A message that's fine goes through; one that isn't goes nowhere.
	_ = control.Send(append([]byte{CmdPad}, make([]byte, 18)...))
	_ = control.Send([]byte{cmdQuit})
	_ = control.Send([]byte{cmdStart, 'x'})
	for ctx.Err() == nil {
		mu.Lock()
		done := got["video"] > 2 && got["audio"] > 2 && rumbles.Load() > 0
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if got["video"] == 0 || got["audio"] == 0 {
		t.Errorf("media %v", got)
	}
	if rumbles.Load() == 0 {
		t.Error("input didn't reach the program, or its answer didn't come back")
	}
	select {
	case <-s.Ended():
		t.Error("a message from a browser ended the stream")
	default:
	}
}
