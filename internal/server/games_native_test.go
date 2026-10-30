package server

import (
	"context"
	"encoding/binary"
	"encoding/json"
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
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittyplay"
)

// A native app, as docs/GAMING-API.md describes one: everything over the documented
// routes with a bearer token, a WebRTC stack of its own, and no browser anywhere.
func TestNativeClientPlaysAGame(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no FFmpeg on PATH")
	}
	runner := filepath.Join(t.TempDir(), "wittyplay-host")
	if runtime.GOOS == "windows" {
		runner += ".exe"
	}
	build := exec.Command("go", "build", "-o", runner, "./cmd/wittyplay-host")
	build.Dir = filepath.Join("..", "..", "modules", "wittyplay")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	t.Setenv("WMS_WITTYPLAY_RUNNER", runner)

	g := newGameServer(t)
	defer g.s.StopGames()
	g.s.Tools.FFmpeg = ffmpegPath
	out, _ := exec.Command(ffmpegPath, "-hide_banner", "-encoders").Output()
	for _, n := range []string{"libx264", "libvpx", "libopus"} {
		g.s.Tools.Encoders[n] = strings.Contains(string(out), " "+n+" ")
	}
	srv := httptest.NewServer(g.h)
	defer srv.Close()

	call := func(method, path string, body any, into any) int {
		t.Helper()
		w := g.do(method, path, g.admin, body)
		if into != nil {
			if err := json.Unmarshal(w.Body.Bytes(), into); err != nil {
				t.Fatalf("%s %s: %v: %s", method, path, err, w.Body.String())
			}
		}
		return w.Code
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Game (World).wittytest"), []byte("game"), 0o644); err != nil {
		t.Fatal(err)
	}
	var lib library.Library
	if c := call("POST", "/api/libraries", map[string]any{"name": "Test", "type": "games", "console": "wittytest", "paths": []string{dir}}, &lib); c != 201 {
		t.Fatalf("library: %d", c)
	}
	var item *library.Item
	for i := 0; i < 200 && item == nil; i++ {
		var r struct {
			Items []*library.Item `json:"items"`
		}
		call("GET", "/api/items?kind=game&library="+itoa64(lib.ID), nil, &r)
		if len(r.Items) > 0 {
			item = r.Items[0]
		}
		time.Sleep(50 * time.Millisecond)
	}
	if item == nil || item.Console != "wittytest" {
		t.Fatalf("no game found: %+v", item)
	}

	// The controller comes as data.
	var consoles []struct {
		ID    string              `json:"id"`
		Ready bool                `json:"ready"`
		Pad   wittyplay.PadLayout `json:"pad"`
	}
	call("GET", "/api/games/consoles", nil, &consoles)
	for _, c := range consoles {
		if c.ID == "snes" && (len(c.Pad.Face) != 4 || c.Pad.Face[0].Label != "Y") {
			t.Errorf("snes pad: %+v", c.Pad)
		}
		if c.ID == "ps1" && !c.Pad.Sticks {
			t.Errorf("the PlayStation has sticks: %+v", c.Pad)
		}
	}
	type controls struct {
		Controls []wittyplay.Control `json:"controls"`
		Defaults struct {
			Gamepad map[string]wittyplay.Binding `json:"gamepad"`
			Remote  map[string]string            `json:"remote"`
			Touch   map[string]wittyplay.TouchLayout
		} `json:"defaults"`
		Saved struct {
			Gamepad  map[string]map[string]wittyplay.Binding `json:"gamepad"`
			Keyboard map[string]string                       `json:"keyboard"`
			Touch    map[string]wittyplay.TouchLayout        `json:"touch"`
		} `json:"saved"`
	}
	var ctl controls
	if c := call("GET", "/api/consoles/snes/controls?device=appletv-1", nil, &ctl); c != 200 {
		t.Fatalf("controls: %d", c)
	}
	if ctl.Defaults.Remote["8"] != "Enter" || ctl.Defaults.Gamepad["8"].Button == nil || *ctl.Defaults.Gamepad["8"].Button != 1 || ctl.Defaults.Touch["landscape"]["dpad"].Size == 0 {
		t.Fatalf("defaults: %+v", ctl.Defaults)
	}
	if ctl.Saved.Keyboard != nil || len(ctl.Saved.Gamepad) != 0 {
		t.Fatalf("nothing is saved yet: %+v", ctl.Saved)
	}

	// Layouts are saved on the server, per account, and touch ones per device.
	if c := call("PUT", "/api/consoles/snes/controls", map[string]any{"kind": "gamepad", "key": "Pad Model", "data": map[string]any{"8": map[string]int{"b": 5}}}, nil); c != 200 {
		t.Fatalf("saving a gamepad layout: %d", c)
	}
	if c := call("PUT", "/api/consoles/snes/controls", map[string]any{"kind": "touch", "key": "phone-1", "orientation": "portrait", "data": map[string]any{"dpad": map[string]float64{"x": 0.3, "y": 0.6, "size": 0.3, "opacity": 0.5}}}, nil); c != 200 {
		t.Fatalf("saving a touch layout: %d", c)
	}
	ctl = controls{}
	call("GET", "/api/consoles/snes/controls?device=phone-1", nil, &ctl)
	if b := ctl.Saved.Gamepad["Pad Model"]["8"].Button; b == nil || *b != 5 {
		t.Errorf("the saved gamepad layout: %+v", ctl.Saved.Gamepad)
	}
	if ctl.Saved.Touch["portrait"]["dpad"].X != 0.3 {
		t.Errorf("the saved touch layout: %+v", ctl.Saved.Touch)
	}
	ctl = controls{}
	call("GET", "/api/consoles/snes/controls?device=another-device", nil, &ctl)
	if len(ctl.Saved.Touch) != 0 || len(ctl.Saved.Gamepad) != 1 {
		t.Errorf("touch stays with its device, gamepads follow the account: %+v", ctl.Saved)
	}
	w := g.do("GET", "/api/consoles/snes/controls", g.other, nil)
	if strings.Contains(w.Body.String(), "Pad Model") {
		t.Error("another account must not see these layouts")
	}
	for name, body := range map[string]map[string]any{
		"bad kind":            {"kind": "wheel", "data": map[string]any{}},
		"bad layout":          {"kind": "gamepad", "key": "x", "data": map[string]any{"99": map[string]int{"b": 1}}},
		"no device":           {"kind": "touch", "orientation": "portrait", "data": map[string]any{}},
		"path in device name": {"kind": "touch", "key": "../x", "orientation": "portrait", "data": map[string]any{}},
		"bad orientation":     {"kind": "touch", "key": "d", "orientation": "sideways", "data": map[string]any{}},
		"no gamepad model":    {"kind": "gamepad", "data": map[string]any{}},
	} {
		if c := call("PUT", "/api/consoles/snes/controls", body, nil); c != 400 {
			t.Errorf("%s: got %d", name, c)
		}
	}
	if c := call("PUT", "/api/consoles/nope/controls", map[string]any{"kind": "keyboard", "data": map[string]any{}}, nil); c != 404 {
		t.Errorf("an unknown console: %d", c)
	}
	if c := call("PUT", "/api/consoles/snes/controls", map[string]any{"kind": "gamepad", "key": "Pad Model", "data": nil}, nil); c != 200 {
		t.Errorf("resetting: %d", c)
	}

	// The modes this server offers.
	var modes struct {
		Client string `json:"client"`
		Modes  []struct {
			ID          string   `json:"id"`
			Video       []string `json:"video"`
			BrowserOnly bool     `json:"browserOnly"`
		} `json:"modes"`
	}
	call("GET", "/api/games/modes", nil, &modes)
	if modes.Client != "native" || len(modes.Modes) == 0 || modes.Modes[0].ID != "webrtc" {
		t.Fatalf("modes: %+v", modes)
	}
	for _, m := range modes.Modes {
		if m.BrowserOnly {
			t.Errorf("%s is browser-only and was offered to an app", m.ID)
		}
	}

	// Start, and connect with nothing but the documented steps.
	var sess struct {
		ID      string `json:"id"`
		Console string `json:"console"`
		Modes   []struct {
			ID string `json:"id"`
		} `json:"modes"`
	}
	if c := call("POST", "/api/games/"+itoa64(item.ID)+"/play", map[string]any{"resume": false}, &sess); c != 201 {
		t.Fatalf("play: %d", c)
	}
	if sess.Console != "wittytest" || len(sess.Modes) == 0 {
		t.Fatalf("session: %+v", sess)
	}
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	for _, k := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeVideo, webrtc.RTPCodecTypeAudio} {
		if _, err := pc.AddTransceiverFromKind(k, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
			t.Fatal(err)
		}
	}
	ordered, retries := false, uint16(0)
	input, err := pc.CreateDataChannel("input", &webrtc.DataChannelInit{Ordered: &ordered, MaxRetransmits: &retries})
	if err != nil {
		t.Fatal(err)
	}
	events, err := pc.CreateDataChannel("events", nil)
	if err != nil {
		t.Fatal(err)
	}
	rumbled := make(chan struct{}, 8)
	events.OnMessage(func(m webrtc.DataChannelMessage) {
		var e struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(m.Data, &e) == nil && e.Type == "rumble" {
			rumbled <- struct{}{}
		}
	})
	var video atomic.Int64
	pc.OnTrack(func(tr *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		buf := make([]byte, 1500)
		for {
			if _, _, err := tr.Read(buf); err != nil {
				return
			}
			if tr.Kind() == webrtc.RTPCodecTypeVideo {
				video.Add(1)
			}
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
	var answer struct {
		SDP string `json:"sdp"`
	}
	if c := call("POST", "/api/play/"+sess.ID+"/webrtc", map[string]any{"sdp": pc.LocalDescription().SDP}, &answer); c != 200 {
		t.Fatalf("webrtc: %d", c)
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer.SDP}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(25 * time.Second)
	for video.Load() < 20 || input.ReadyState() != webrtc.DataChannelStateOpen {
		if time.Now().After(deadline) {
			t.Fatalf("no picture: %d packets, input %s", video.Load(), input.ReadyState())
		}
		time.Sleep(50 * time.Millisecond)
	}

	// The input message exactly as documented: A pressed on player 0 rumbles in the
	// test game, and the event comes back on its channel.
	state := make([]byte, 11)
	binary.LittleEndian.PutUint16(state[1:], 1<<wittyplay.RetroA)
	if err := input.Send(state); err != nil {
		t.Fatal(err)
	}
	select {
	case <-rumbled:
	case <-time.After(10 * time.Second):
		t.Fatal("the button press did not reach the game, or the event did not come back")
	}

	// The WebSocket mode with a bearer header, as an app would use it.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, srv.URL+"/api/play/"+sess.ID+"/ws?video=jpeg", &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + g.admin}}})
	if err != nil {
		t.Fatal(err)
	}
	c.SetReadLimit(1 << 24)
	typ, b, err := c.Read(ctx)
	if err != nil || typ != websocket.MessageText || !strings.Contains(string(b), `"hello"`) {
		t.Fatalf("first message: %v %s", err, b)
	}
	c.Close(websocket.StatusNormalClosure, "")

	if c := call("DELETE", "/api/play/"+sess.ID, nil, nil); c != 200 {
		t.Fatalf("stop: %d", c)
	}
	var saves []struct {
		Slot int `json:"slot"`
	}
	call("GET", "/api/games/"+itoa64(item.ID)+"/saves", nil, &saves)
	if len(saves) != 1 || saves[0].Slot != 0 {
		t.Fatalf("stopping should leave the automatic save: %+v", saves)
	}
}
