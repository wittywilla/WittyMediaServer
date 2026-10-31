package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// Timings for a stream that nobody is watching: a brief dropout or a trip to another app
// should cost nothing, and a tab closed for good shouldn't hold a PC's encoder for long.
const (
	noViewerLimit = 60 * time.Second
	startLimit    = 30 * time.Second
	idrEvery      = 300 * time.Millisecond
)

// Options set up a session.
type Options struct {
	Program  string
	Prepare  func(*exec.Cmd)
	Start    Start
	Log      *slog.Logger
	STUN     []string
	PublicIP []string
	// OnEnd is told once, when the stream has ended for any reason.
	OnEnd func(reason string)
}

// Session is one stream from one PC, to one viewer at a time.
type Session struct {
	opts Options
	host *Host
	log  *slog.Logger

	mu       sync.Mutex
	pc       *webrtc.PeerConnection
	video    *webrtc.TrackLocalStaticSample
	audio    *webrtc.TrackLocalStaticSample
	control  *webrtc.DataChannel
	ws       func(kind byte, flags byte, ts uint32, data []byte)
	wsEvents func([]byte)
	viewerAt time.Time
	viewing  bool
	info     Event
	started  chan error
	startOne sync.Once
	ended    chan struct{}
	endOne   sync.Once

	lastTS    atomic.Uint32
	lastIDR   atomic.Int64
	frameDur  atomic.Int64
	audioDur  atomic.Int64
	frames    atomic.Uint64
	audioPkts atomic.Uint64
}

// New starts the host program and connects it to the PC. It returns when the stream is
// flowing, or says why it isn't.
func New(ctx context.Context, opts Options) (*Session, error) {
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	s := &Session{opts: opts, log: opts.Log, started: make(chan error, 1), ended: make(chan struct{})}
	if opts.Start.FPS > 0 {
		s.frameDur.Store(int64(time.Second) / int64(opts.Start.FPS))
	}
	s.audioDur.Store(int64(5 * time.Millisecond))
	host, err := StartHost(opts.Program, opts.Prepare, opts.Log, s)
	if err != nil {
		return nil, err
	}
	s.host = host
	go func() {
		<-host.Done()
		s.startOne.Do(func() { s.started <- errors.New("the Moonlight host program stopped before the stream began") })
		s.end("the stream ended")
	}()
	if err := host.Begin(opts.Start); err != nil {
		s.Close()
		return nil, err
	}
	wait, cancel := context.WithTimeout(ctx, startLimit)
	defer cancel()
	select {
	case err := <-s.started:
		if err != nil {
			s.Close()
			return nil, err
		}
	case <-wait.Done():
		s.Close()
		return nil, errors.New("the PC didn't start streaming in time")
	}
	s.mu.Lock()
	s.viewerAt = time.Now()
	s.mu.Unlock()
	go s.watchViewer()
	return s, nil
}

func (s *Session) watchViewer() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.ended:
			return
		case <-t.C:
		}
		s.mu.Lock()
		gone := !s.viewing && time.Since(s.viewerAt) > noViewerLimit
		s.mu.Unlock()
		if gone {
			s.log.Info("moonlight stream ended: nobody was watching")
			s.Close()
			return
		}
	}
}

// Ended closes when the stream has ended.
func (s *Session) Ended() <-chan struct{} { return s.ended }

func (s *Session) end(reason string) {
	s.endOne.Do(func() {
		close(s.ended)
		s.mu.Lock()
		pc := s.pc
		s.mu.Unlock()
		if pc != nil {
			_ = pc.Close()
		}
		if s.opts.OnEnd != nil {
			s.opts.OnEnd(reason)
		}
	})
}

// Close ends the stream.
func (s *Session) Close() {
	if s.host != nil {
		s.host.Close()
	}
	s.end("the stream was closed")
}

// Counts says how many frames and audio packets have come through.
func (s *Session) Counts() (video, audio uint64) { return s.frames.Load(), s.audioPkts.Load() }

// Info is the latest thing known about the picture.
func (s *Session) Info() Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info
}

// Input passes on a message from the viewer.
func (s *Session) Input(m []byte) { _ = s.host.Input(m) }

// Video is the host program's next frame.
func (s *Session) Video(keyframe bool, ts uint32, annexB []byte) {
	s.frames.Add(1)
	s.mu.Lock()
	track, ws := s.video, s.ws
	s.mu.Unlock()
	dur := time.Duration(s.frameDur.Load())
	if last := s.lastTS.Swap(ts); last != 0 && ts > last {
		if d := time.Duration(ts-last) * time.Second / 90000; d > 0 && d < time.Second {
			dur = d
		}
	}
	if track != nil {
		_ = track.WriteSample(media.Sample{Data: annexB, Duration: dur})
	}
	if ws != nil {
		flags := byte(0)
		if keyframe {
			flags = 1
		}
		ws(1, flags, ts, annexB)
	}
}

// Audio is the host program's next Opus packet.
func (s *Session) Audio(opus []byte) {
	s.audioPkts.Add(1)
	s.mu.Lock()
	track, ws := s.audio, s.ws
	s.mu.Unlock()
	if track != nil {
		_ = track.WriteSample(media.Sample{Data: opus, Duration: time.Duration(s.audioDur.Load())})
	}
	if ws != nil {
		ws(2, 0, 0, opus)
	}
}

// Rumble goes on to the viewer's controller.
func (s *Session) Rumble(controller, low, high uint16) {
	s.tell(map[string]any{"type": "rumble", "controller": controller, "low": low, "high": high})
}

// Event is something from the host program.
func (s *Session) Event(e Event) {
	switch e.Type {
	case "started":
		s.startOne.Do(func() { s.started <- nil })
	case "failed":
		s.startOne.Do(func() { s.started <- fmt.Errorf("the connection to the PC failed at its %s step (%d)", e.Stage, e.Code) })
	case "terminated":
		s.log.Info("moonlight stream terminated", "code", e.Code)
		s.tell(e)
	case "video":
		s.mu.Lock()
		s.info = e
		s.mu.Unlock()
		if e.FPS > 0 {
			s.frameDur.Store(int64(time.Second) / int64(e.FPS))
		}
		s.tell(e)
	case "audio":
		if e.Rate > 0 && e.Samples > 0 {
			s.audioDur.Store(int64(e.Samples) * int64(time.Second) / int64(e.Rate))
		}
	case "status":
		s.tell(e)
	}
}

func (s *Session) tell(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.mu.Lock()
	dc, ws := s.control, s.wsEvents
	s.mu.Unlock()
	if dc != nil && dc.ReadyState() == webrtc.DataChannelStateOpen {
		_ = dc.SendText(string(b))
	}
	if ws != nil {
		ws(b)
	}
}

func (s *Session) requestIDR() {
	now := time.Now().UnixNano()
	if now-s.lastIDR.Load() < int64(idrEvery) {
		return
	}
	s.lastIDR.Store(now)
	_ = s.host.Input([]byte{CmdIDR})
}

// Offer takes a browser's WebRTC offer and answers it. A new one replaces any earlier
// viewer, so a reload or another device simply carries on.
func (s *Session) Offer(ctx context.Context, offer string) (string, error) {
	videoCap := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"}
	audioCap := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "minptime=5;useinbandfec=1;stereo=1"}
	me := &webrtc.MediaEngine{}
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
	if len(s.opts.PublicIP) > 0 {
		se.SetNAT1To1IPs(s.opts.PublicIP, webrtc.ICECandidateTypeHost)
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithInterceptorRegistry(ir), webrtc.WithSettingEngine(se))
	var servers []webrtc.ICEServer
	if len(s.opts.STUN) > 0 {
		servers = append(servers, webrtc.ICEServer{URLs: s.opts.STUN})
	}
	pc, err := api.NewPeerConnection(webrtc.Configuration{ICEServers: servers})
	if err != nil {
		return "", err
	}
	video, err := webrtc.NewTrackLocalStaticSample(videoCap, "video", "wittymoonlight")
	if err != nil {
		_ = pc.Close()
		return "", err
	}
	audio, err := webrtc.NewTrackLocalStaticSample(audioCap, "audio", "wittymoonlight")
	if err != nil {
		_ = pc.Close()
		return "", err
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer}); err != nil {
		_ = pc.Close()
		return "", errors.New("that offer couldn't be read")
	}
	for i, track := range []*webrtc.TrackLocalStaticSample{video, audio} {
		sender, err := pc.AddTrack(track)
		if err != nil {
			_ = pc.Close()
			return "", err
		}
		go func() {
			for {
				pkts, _, err := sender.ReadRTCP()
				if err != nil {
					return
				}
				if i != 0 {
					continue
				}
				for _, p := range pkts {
					switch p.(type) {
					case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
						s.requestIDR()
					}
				}
			}
		}()
	}
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() == "control" {
			dc.OnOpen(func() {
				s.mu.Lock()
				if s.pc == pc {
					s.control = dc
				}
				s.mu.Unlock()
				if info := s.Info(); info.Type != "" {
					s.tell(info)
				}
			})
		}
		dc.OnMessage(func(m webrtc.DataChannelMessage) {
			if m.IsString {
				return
			}
			s.mu.Lock()
			current := s.pc == pc
			s.mu.Unlock()
			if current {
				s.Input(m.Data)
			}
		})
	})
	pc.OnConnectionStateChange(func(st webrtc.PeerConnectionState) {
		s.mu.Lock()
		current := s.pc == pc
		switch {
		case !current:
		case st == webrtc.PeerConnectionStateConnected:
			s.viewing = true
		case st == webrtc.PeerConnectionStateFailed, st == webrtc.PeerConnectionStateClosed, st == webrtc.PeerConnectionStateDisconnected:
			s.viewing = false
			s.viewerAt = time.Now()
		}
		s.mu.Unlock()
		if current && st == webrtc.PeerConnectionStateConnected {
			s.requestIDR()
		}
	})
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = pc.Close()
		return "", err
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		_ = pc.Close()
		return "", err
	}
	select {
	case <-gathered:
	case <-ctx.Done():
		_ = pc.Close()
		return "", ctx.Err()
	case <-time.After(5 * time.Second):
	}
	s.mu.Lock()
	old := s.pc
	s.pc, s.video, s.audio, s.control = pc, video, audio, nil
	s.viewerAt = time.Now()
	s.mu.Unlock()
	if old != nil {
		go old.Close()
	}
	s.requestIDR()
	return pc.LocalDescription().SDP, nil
}
