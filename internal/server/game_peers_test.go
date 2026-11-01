package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/webrtc/v4"
)

// Each offer on a game session holds sockets, goroutines and memory for half a minute
// whether or not anything connects, so a session gets only so many. Through the real
// handlers with the test game, as anyone allowed to see a game could do it.
func TestGameSessionConnectionsAreLimited(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no FFmpeg on PATH")
	}
	runner := filepath.Join(t.TempDir(), "wittyplay-host")
	if runtime.GOOS == "windows" {
		runner += ".exe"
	}
	build := exec.Command("go", "build", "-o", runner, "github.com/wittywilla/wittyplay/cmd/wittyplay-host")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	t.Setenv("WMS_WITTYPLAY_RUNNER", runner)
	g := newGameServer(t)
	defer g.s.StopGames()
	g.s.Tools.FFmpeg = ffmpegPath
	g.s.Tools.Encoders = map[string]bool{"libx264": true, "libvpx": true}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Game.wittytest"), []byte("game"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := g.do("POST", "/api/libraries", g.admin, map[string]any{"name": "Test", "type": "games", "console": "wittytest", "paths": []string{dir}})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var lib struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &lib)
	var item int64
	for i := 0; i < 200 && item == 0; i++ {
		var list struct {
			Items []struct {
				ID int64 `json:"id"`
			} `json:"items"`
		}
		_ = json.Unmarshal(g.do("GET", "/api/items?library="+itoa64(lib.ID), g.admin, nil).Body.Bytes(), &list)
		if len(list.Items) > 0 {
			item = list.Items[0].ID
		}
		time.Sleep(50 * time.Millisecond)
	}
	if item == 0 {
		t.Fatal("the scan did not finish in time")
	}
	w = g.do("POST", "/api/games/"+itoa64(item)+"/play", g.admin, map[string]any{})
	if w.Code != 201 {
		t.Fatalf("play: %d %s", w.Code, w.Body)
	}
	var sess struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &sess)

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
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
	body := map[string]any{"sdp": pc.LocalDescription().SDP}
	answered, refused := 0, 0
	for range peerOfferBurst + 12 {
		w := g.do("POST", "/api/play/"+sess.ID+"/webrtc", g.admin, body)
		switch w.Code {
		case 200:
			answered++
		case 429:
			refused++
			if w.Header().Get("Retry-After") == "" {
				t.Errorf("no Retry-After: %v", w.Header())
			}
		default:
			t.Logf("offer: %d %s", w.Code, w.Body)
		}
	}
	if answered == 0 || answered > peerOfferBurst || refused == 0 {
		t.Errorf("%d answered and %d refused, with a limit of %d", answered, refused, peerOfferBurst)
	}

	srv := httptest.NewServer(g.h)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dial := func() (*websocket.Conn, int) {
		c, res, err := websocket.Dial(ctx, srv.URL+"/api/play/"+sess.ID+"/ws?video=jpeg", &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + g.admin}}})
		if err != nil {
			if res != nil {
				return nil, res.StatusCode
			}
			t.Fatal(err)
		}
		return c, 101
	}
	var open []*websocket.Conn
	for i := range peerSocketsAtOnce {
		c, code := dial()
		if code != 101 {
			t.Fatalf("socket %d: %d", i, code)
		}
		open = append(open, c)
	}
	c, code := dial()
	if code != 429 {
		if c != nil {
			c.CloseNow()
		}
		t.Errorf("one socket too many: %d", code)
	}
	// Closing one makes room.
	open[0].CloseNow()
	deadline := time.Now().Add(5 * time.Second)
	for {
		c, code := dial()
		if code == 101 {
			c.CloseNow()
			break
		}
		if time.Now().After(deadline) {
			t.Errorf("a closed socket left no room: %d", code)
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, c := range open[1:] {
		c.CloseNow()
	}
}

func TestPeerLimitsRefillAndForget(t *testing.T) {
	var p peerLimits
	now := time.Unix(1000, 0)
	for range peerOfferBurst {
		if !p.offer("a", now) {
			t.Fatal("within the burst")
		}
	}
	if p.offer("a", now) {
		t.Error("past the burst")
	}
	if !p.offer("b", now) {
		t.Error("another session isn't touched")
	}
	if p.offer("a", now.Add(peerOfferEvery-time.Second)) {
		t.Error("too soon")
	}
	if !p.offer("a", now.Add(peerOfferEvery)) {
		t.Error("one more after the wait")
	}
	if p.offer("a", now.Add(peerOfferEvery)) {
		t.Error("only one more")
	}
	// Sessions come and go, and the list doesn't keep them.
	for i := range peerSessionsKept * 3 {
		p.offer(itoa64(int64(i)), now.Add(time.Duration(i)*time.Second))
	}
	if len(p.m) > peerSessionsKept {
		t.Errorf("%d kept", len(p.m))
	}
	release, ok := p.socket("x", now)
	if !ok {
		t.Fatal("first socket")
	}
	release()
	release() // once is what counts
	if p.m["x"].sockets != 0 {
		t.Errorf("%d", p.m["x"].sockets)
	}
}
