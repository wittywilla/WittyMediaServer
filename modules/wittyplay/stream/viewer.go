package stream

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/wittywilla/wittyplay/ipc"
)

// viewer is one browser watching a session.
type viewer struct {
	s       *Session
	codec   string // h264 | vp8 | jpeg
	waitKey atomic.Bool

	// WebRTC
	pc     *webrtc.PeerConnection
	video  *webrtc.TrackLocalStaticSample
	audio  *webrtc.TrackLocalStaticSample
	events atomic.Pointer[webrtc.DataChannel]

	// WebSocket
	ws  *websocket.Conn
	out chan wsMessage

	closeOnce sync.Once
}

type wsMessage struct {
	typ  websocket.MessageType
	data []byte
}

// What a WebSocket viewer receives; the first byte says which.
const (
	wsH264 = 1 // then one byte, 1 for a keyframe, then Annex B
	wsJPEG = 2
	wsPCM  = 3 // then a u32 sample rate, then s16le stereo
)

// attach counts a viewer in and wakes the game up if it was the first.
func (s *Session) attach(v *viewer) {
	s.viewMu.Lock()
	s.viewers[v] = true
	s.lastInput = time.Now()
	// A running encoder's next keyframe may be a second away, and this viewer can
	// show nothing until then. Starting it again sends one straight away.
	switch {
	case v.codec == "h264" && s.h264 != nil:
		s.h264.stop()
		s.h264 = nil
	case v.codec == "vp8" && s.vp8 != nil:
		s.vp8.stop()
		s.vp8 = nil
	}
	if v.pc != nil {
		s.startOpusLocked()
	}
	s.applyPauseLocked()
	s.viewMu.Unlock()
	// A paused game makes no new frames, but a new viewer still needs a picture.
	s.signalFrame()
}

// applyPauseLocked tells the runner whether to run: only with someone watching,
// and not while a menu has paused it. It says so only when that has changed.
func (s *Session) applyPauseLocked() {
	paused := s.menuPaused || len(s.viewers) == 0
	if paused == s.paused {
		return
	}
	s.paused = paused
	b := byte(0)
	if paused {
		b = 1
	}
	_ = s.command(ipc.CmdPause, []byte{b})
}

// detach counts a viewer out; with none left the game pauses and encoders stop.
func (s *Session) detach(v *viewer) {
	s.viewMu.Lock()
	if !s.viewers[v] {
		s.viewMu.Unlock()
		return
	}
	delete(s.viewers, v)
	if len(s.viewers) == 0 {
		s.noViewerSince = time.Now()
		s.stopEncodersLocked()
	} else if !s.anyWebRTCLocked() {
		s.stopOpusLocked()
	}
	s.applyPauseLocked()
	s.viewMu.Unlock()
}

func (s *Session) anyWebRTCLocked() bool {
	for v := range s.viewers {
		if v.pc != nil {
			return true
		}
	}
	return false
}

func (v *viewer) close() {
	v.closeOnce.Do(func() {
		v.s.detach(v)
		if v.pc != nil {
			_ = v.pc.Close()
		}
		if v.ws != nil {
			_ = v.ws.Close(websocket.StatusNormalClosure, "the game has ended")
		}
	})
}

func (s *Session) snapshotViewers() []*viewer {
	s.viewMu.Lock()
	defer s.viewMu.Unlock()
	out := make([]*viewer, 0, len(s.viewers))
	for v := range s.viewers {
		out = append(out, v)
	}
	return out
}

func (s *Session) deliverVideo(codec string, au AU) {
	dur := time.Duration(float64(time.Second) / s.info.FPS)
	for _, v := range s.snapshotViewers() {
		if v.codec != codec {
			continue
		}
		if v.video != nil {
			_ = v.video.WriteSample(media.Sample{Data: au.Data, Duration: dur})
			continue
		}
		// A viewer that fell behind waits for the next keyframe, because its
		// decoder can't use anything before that.
		if v.waitKey.Load() && !au.Key {
			continue
		}
		flags := byte(0)
		if au.Key {
			flags = 1
		}
		msg := append([]byte{wsH264, flags}, au.Data...)
		if v.send(websocket.MessageBinary, msg) {
			v.waitKey.Store(false)
		} else {
			v.waitKey.Store(true)
		}
	}
}

func (s *Session) deliverJPEG(b []byte) {
	for _, v := range s.snapshotViewers() {
		if v.codec == "jpeg" {
			v.send(websocket.MessageBinary, append([]byte{wsJPEG}, b...))
		}
	}
}

func (s *Session) deliverOpus(p []byte) {
	for _, v := range s.snapshotViewers() {
		if v.audio != nil {
			_ = v.audio.WriteSample(media.Sample{Data: p, Duration: 10 * time.Millisecond})
		}
	}
}

func (s *Session) deliverPCM(b []byte) {
	vs := s.snapshotViewers()
	var msg []byte
	for _, v := range vs {
		if v.ws == nil {
			continue
		}
		if msg == nil {
			msg = make([]byte, 5, 5+len(b))
			msg[0] = wsPCM
			s.viewMu.Lock()
			binary.LittleEndian.PutUint32(msg[1:], uint32(s.rate))
			s.viewMu.Unlock()
			msg = append(msg, b...)
		}
		v.send(websocket.MessageBinary, msg)
	}
}

func (s *Session) broadcastEvent(b []byte) {
	for _, v := range s.snapshotViewers() {
		if dc := v.events.Load(); dc != nil {
			_ = dc.SendText(string(b))
		} else if v.ws != nil {
			v.send(websocket.MessageText, b)
		}
	}
}

// send queues a message without waiting, and reports whether it fit.
func (v *viewer) send(t websocket.MessageType, b []byte) bool {
	select {
	case v.out <- wsMessage{t, b}:
		return true
	default:
		return false
	}
}

// ---- WebRTC ----

// udpShared is the one UDP port all sessions share when one is configured.
type udpShared struct {
	mux  ice.UDPMux
	conn net.PacketConn
	port int
}

func (m *Manager) sharedUDP(port int) (ice.UDPMux, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.udp != nil && m.udp.port == port {
		return m.udp.mux, nil
	}
	if m.udp != nil {
		_ = m.udp.mux.Close()
		m.udp = nil
	}
	conn, err := net.ListenPacket("udp", net.JoinHostPort("", itoa(port)))
	if err != nil {
		return nil, err
	}
	mux := webrtc.NewICEUDPMux(nil, conn)
	m.udp = &udpShared{mux: mux, conn: conn, port: port}
	return mux, nil
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// MaxOfferSize bounds an SDP offer; a real one is a few kilobytes.
const MaxOfferSize = 64 << 10

// Answer connects a browser over WebRTC: it takes the browser's offer and returns
// the answer. prefer is "" or "vp8". Which codec is used depends on what the
// browser offered and what FFmpeg here can encode.
func (s *Session) Answer(ctx context.Context, offer, prefer string) (string, error) {
	if len(offer) > MaxOfferSize {
		return "", errors.New("the connection offer is too large")
	}
	select {
	case <-s.done:
		return "", ErrStopped
	default:
	}
	codec := s.pickCodec(offer, prefer)
	if codec == "" {
		return "", errors.New("this browser and this server have no video format in common")
	}
	me := &webrtc.MediaEngine{}
	videoCap := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"}
	if codec == "vp8" {
		videoCap = webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000}
	}
	audioCap := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "minptime=10;useinbandfec=1"}
	if err := me.RegisterCodec(webrtc.RTPCodecParameters{RTPCodecCapability: videoCap, PayloadType: 102}, webrtc.RTPCodecTypeVideo); err != nil {
		return "", err
	}
	if err := me.RegisterCodec(webrtc.RTPCodecParameters{RTPCodecCapability: audioCap, PayloadType: 111}, webrtc.RTPCodecTypeAudio); err != nil {
		return "", err
	}
	ir := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(me, ir); err != nil {
		return "", err
	}
	var se webrtc.SettingEngine
	cfg := s.m.config()
	if cfg.UDPPort > 0 {
		mux, err := s.m.sharedUDP(cfg.UDPPort)
		if err != nil {
			s.log.Warn("the WebRTC UDP port is unavailable; using any free port", "port", cfg.UDPPort, "err", err)
		} else {
			se.SetICEUDPMux(mux)
		}
	}
	if len(cfg.PublicIPs) > 0 {
		se.SetNAT1To1IPs(cfg.PublicIPs, webrtc.ICECandidateTypeHost)
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithInterceptorRegistry(ir), webrtc.WithSettingEngine(se))
	var ice []webrtc.ICEServer
	if len(cfg.STUN) > 0 {
		ice = append(ice, webrtc.ICEServer{URLs: cfg.STUN})
	}
	pc, err := api.NewPeerConnection(webrtc.Configuration{ICEServers: ice})
	if err != nil {
		return "", err
	}
	v := &viewer{s: s, codec: codec, pc: pc}
	fail := func(err error) (string, error) { _ = pc.Close(); return "", err }

	video, err := webrtc.NewTrackLocalStaticSample(videoCap, "video", "wittyplay")
	if err != nil {
		return fail(err)
	}
	audio, err := webrtc.NewTrackLocalStaticSample(audioCap, "audio", "wittyplay")
	if err != nil {
		return fail(err)
	}
	v.video, v.audio = video, audio
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer}); err != nil {
		return fail(err)
	}
	for _, track := range []*webrtc.TrackLocalStaticSample{video, audio} {
		sender, err := pc.AddTrack(track)
		if err != nil {
			return fail(err)
		}
		go func() { // the interceptors act on what arrives here
			buf := make([]byte, 1500)
			for {
				if _, _, err := sender.Read(buf); err != nil {
					return
				}
			}
		}()
	}
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		switch dc.Label() {
		case "input":
			dc.OnMessage(func(m webrtc.DataChannelMessage) {
				if !m.IsString {
					s.Input(m.Data)
				}
			})
		case "events":
			dc.OnOpen(func() { v.events.Store(dc) })
		}
	})
	pc.OnConnectionStateChange(func(st webrtc.PeerConnectionState) {
		switch st {
		case webrtc.PeerConnectionStateConnected:
			s.attach(v)
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			v.close()
		}
	})
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return fail(err)
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		return fail(err)
	}
	select {
	case <-gathered:
	case <-ctx.Done():
		return fail(ctx.Err())
	case <-time.After(6 * time.Second): // answer with what has been found so far
	}
	go func() { // a browser that never connects must not hold this forever
		select {
		case <-time.After(30 * time.Second):
			if pc.ConnectionState() != webrtc.PeerConnectionStateConnected {
				_ = pc.Close()
			}
		case <-s.done:
			_ = pc.Close()
		}
	}()
	return pc.LocalDescription().SDP, nil
}

func (s *Session) pickCodec(offer, prefer string) string {
	hasH264 := strings.Contains(offer, "H264/90000") && len(s.chain("h264")) > 0
	hasVP8 := strings.Contains(offer, "VP8/90000") && len(s.chain("vp8")) > 0
	switch {
	case prefer == "vp8" && hasVP8:
		return "vp8"
	case hasH264:
		return "h264"
	case hasVP8:
		return "vp8"
	}
	return ""
}

// ---- WebSocket: the path that works wherever HTTP does ----

// ServeWS is the fallback transport. mode is "h264" (decoded with WebCodecs) or
// "jpeg". Origin checks are done by Accept, as for the rest of the API.
func (s *Session) ServeWS(w http.ResponseWriter, r *http.Request, mode string) {
	if mode != "h264" {
		mode = "jpeg"
	}
	if mode == "h264" && len(s.chain("h264")) == 0 {
		mode = "jpeg"
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	c.SetReadLimit(4096)
	v := &viewer{s: s, codec: mode, ws: c, out: make(chan wsMessage, 16)}
	v.waitKey.Store(true)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer v.close()
	hello, _ := json.Marshal(map[string]any{"type": "hello", "video": mode})
	v.send(websocket.MessageText, hello)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.done:
				_ = c.Close(websocket.StatusNormalClosure, "the game has ended")
				return
			case m := <-v.out:
				wctx, wcancel := context.WithTimeout(ctx, 5*time.Second)
				err := c.Write(wctx, m.typ, m.data)
				wcancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	s.attach(v)
	for {
		t, b, err := c.Read(ctx)
		if err != nil {
			return
		}
		if t == websocket.MessageBinary {
			s.Input(b)
		}
	}
}
