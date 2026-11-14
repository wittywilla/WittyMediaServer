// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/wittywilla/wittymediaserver/internal/library"
	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
)

// A native app, as docs/GAMING-API.md describes one: everything over the documented
// routes with a bearer token, a WebRTC stack of its own, and no browser anywhere.
func TestNativeClientPlaysAGame(t *testing.T) {
	modtest.Need(t, "wittyplay")
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no FFmpeg on PATH")
	}
	runner := modtest.Runner(t)
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
	// Controller layouts are WittyPlay's own now (its api package tests them).

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
			ID     string `json:"id"`
			Stream int    `json:"stream"`
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
	if c := call("POST", "/api/play/"+sess.ID+"/stream", map[string]any{"hello": map[string]any{"v": 2, "app": "test"}, "sdp": pc.LocalDescription().SDP}, &answer); c != 200 {
		t.Fatalf("stream: %d", c)
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

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	{
		// A second device, as docs/GAMING-STREAM.md has it: the hello and the offer through one route.
		pc2, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		if err != nil {
			t.Fatal(err)
		}
		defer pc2.Close()
		for _, kind := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeVideo, webrtc.RTPCodecTypeAudio} {
			if _, err := pc2.AddTransceiverFromKind(kind, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := pc2.CreateDataChannel("events", nil); err != nil {
			t.Fatal(err)
		}
		offer2, err := pc2.CreateOffer(nil)
		if err != nil {
			t.Fatal(err)
		}
		gathered2 := webrtc.GatheringCompletePromise(pc2)
		if err := pc2.SetLocalDescription(offer2); err != nil {
			t.Fatal(err)
		}
		<-gathered2
		var connected struct {
			Stream struct {
				V         int    `json:"v"`
				Conn      string `json:"conn"`
				Transport string `json:"transport"`
				Audio     string `json:"audio"`
				Away      bool   `json:"away"`
				Seats     []struct {
					Port int  `json:"port"`
					You  bool `json:"you"`
				} `json:"seats"`
			} `json:"stream"`
			SDP string `json:"sdp"`
		}
		hello := map[string]any{"v": 2, "app": "test", "device": "appletv-1", "seats": []int{1}, "display": map[string]any{"hz": 60}}
		if c := call("POST", "/api/play/"+sess.ID+"/stream", map[string]any{"hello": hello, "sdp": pc2.LocalDescription().SDP}, &connected); c != 200 {
			t.Fatalf("stream: %d", c)
		}
		st := connected.Stream
		if st.V != 2 || st.Conn == "" || st.Transport != "webrtc" || st.Audio != "opus" || st.Away || len(st.Seats) != 1 || st.Seats[0].Port != 1 || !st.Seats[0].You || connected.SDP == "" {
			t.Fatalf("version 2's answer: %+v", connected)
		}
		if err := pc2.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: connected.SDP}); err != nil {
			t.Fatal(err)
		}

		// Its WebSocket, with a bearer header: the hello first, and the stream's description back.
		c2, _, err := websocket.Dial(ctx, srv.URL+"/api/play/"+sess.ID+"/stream", &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + g.admin}}})
		if err != nil {
			t.Fatal(err)
		}
		c2.SetReadLimit(1 << 24)
		if err := c2.Write(ctx, websocket.MessageText, []byte(`{"type":"hello","v":2,"device":"phone-1","watch":false,"seats":[2]}`)); err != nil {
			t.Fatal(err)
		}
		typ, b, err := c2.Read(ctx)
		if err != nil || typ != websocket.MessageText || !strings.Contains(string(b), `"type":"stream"`) || !strings.Contains(string(b), `"transport":"websocket"`) {
			t.Fatalf("version 2's first message: %v %s", err, b)
		}
		c2.Close(websocket.StatusNormalClosure, "")
	}

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
