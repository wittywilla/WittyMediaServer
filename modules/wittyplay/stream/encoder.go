package stream

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittyplay/engine"
	"github.com/wittywilla/wittyplay/ipc"
)

// AU is one encoded frame in Annex B form (H.264) or as a VP8 frame.
type AU struct {
	Data []byte
	Key  bool
}

func pixFmt(format int) string {
	switch format {
	case engine.FormatXRGB8888:
		return "bgr0"
	case engine.FormatRGB565:
		return "rgb565le"
	}
	return "rgb555le"
}

// bitrate for a quality setting, by how many pixels are being sent.
func bitrate(quality string, w, h int) int {
	px := w * h
	var kbps int
	switch quality {
	case "low":
		kbps = 400 + px/500
	case "high":
		kbps = 2000 + px/40
	default:
		kbps = 1000 + px/120
	}
	return kbps * 1000
}

// scaleFor doubles small frames with nearest-neighbour: it keeps pixel art sharp
// through 4:2:0 chroma. A machine with little to spare sends them as they are,
// unless a hardware encoder is doing the work.
func scaleFor(w int, hardware bool) int {
	if w > 400 {
		return 1
	}
	if !hardware && runtime.NumCPU() < 4 {
		return 1
	}
	return 2
}

func isHardware(name string) bool {
	return strings.HasPrefix(name, "h264_")
}

// encoderArgs are the low-latency settings for each encoder: no B-frames, no
// lookahead, its fastest preset, and a keyframe every second so a lost packet
// heals quickly.
func encoderArgs(name string, bps int, gop int) (args, inputArgs []string, upload string) {
	b := strconv.Itoa(bps)
	g := strconv.Itoa(gop)
	half := strconv.Itoa(bps / 2)
	switch name {
	case "h264_nvenc":
		args = []string{"-preset", "p1", "-tune", "ull", "-zerolatency", "1", "-rc", "cbr", "-b:v", b, "-bf", "0", "-profile:v", "baseline"}
	case "h264_qsv":
		args = []string{"-preset", "veryfast", "-async_depth", "1", "-look_ahead", "0", "-b:v", b, "-bf", "0", "-profile:v", "baseline"}
	case "h264_amf":
		args = []string{"-usage", "ultralowlatency", "-quality", "speed", "-rc", "cbr", "-b:v", b, "-bf", "0", "-profile:v", "constrained_baseline"}
	case "h264_vaapi":
		inputArgs = []string{"-vaapi_device", "/dev/dri/renderD128"}
		upload = "hwupload"
		args = []string{"-rc_mode", "CBR", "-b:v", b, "-bf", "0", "-profile:v", "constrained_baseline"}
	case "h264_videotoolbox":
		args = []string{"-realtime", "1", "-prio_speed", "1", "-b:v", b, "-bf", "0", "-profile:v", "baseline", "-allow_sw", "1"}
	case "libx264":
		args = []string{"-preset", "ultrafast", "-tune", "zerolatency", "-profile:v", "baseline", "-b:v", b, "-maxrate", b, "-bufsize", half,
			"-x264-params", "scenecut=0:rc-lookahead=0:sync-lookahead=0:sliced-threads=0"}
	case "libopenh264":
		args = []string{"-b:v", b, "-profile:v", "constrained_baseline", "-rc_mode", "bitrate"}
	case "libvpx":
		args = []string{"-deadline", "realtime", "-cpu-used", "8", "-lag-in-frames", "0", "-error-resilient", "1", "-b:v", b, "-maxrate", b, "-minrate", half, "-bufsize", half, "-threads", "2"}
	}
	return append(args, "-g", g, "-keyint_min", g), inputArgs, upload
}

// chain is the encoders to try for a codec, best first.
func (s *Session) chain(codec string) []string {
	var out []string
	add := func(name string) {
		s.m.mu.Lock()
		bad := s.m.badEnc[name]
		s.m.mu.Unlock()
		if !bad && s.cfg.HaveEncoder(name) {
			out = append(out, name)
		}
	}
	if codec == "vp8" {
		add("libvpx")
		return out
	}
	for _, n := range s.cfg.HWEncoders {
		add(n)
	}
	add("libx264")
	add("libopenh264")
	return out
}

func (s *Session) markBadEncoder(name string) {
	s.m.mu.Lock()
	s.m.badEnc[name] = true
	s.m.mu.Unlock()
}

// videoEncoder is one FFmpeg process turning raw frames into encoded ones.
type videoEncoder struct {
	name    string
	codec   string
	hdr     ipc.VideoHeader
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	cancel  context.CancelFunc
	got     chan struct{} // closed at the first frame out
	gotOnce sync.Once
	stopped chan struct{}
}

func (s *Session) startVideoEncoder(codec, name string, hdr ipc.VideoHeader, fps float64) (*videoEncoder, error) {
	hardware := isHardware(name)
	scale := scaleFor(hdr.Width, hardware)
	bps := bitrate(s.opt.Quality, hdr.Width*scale, hdr.Height*scale)
	gop := int(fps + 0.5)
	if gop < 10 {
		gop = 10
	}
	encArgs, inputArgs, upload := encoderArgs(name, bps, gop)
	vf := fmt.Sprintf("scale=trunc(iw*%d/2)*2:trunc(ih*%d/2)*2:flags=neighbor,format=%s", scale, scale, encoderPixFmt(name))
	if upload != "" {
		vf += "," + upload
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	args = append(args, inputArgs...)
	args = append(args, "-fflags", "+nobuffer", "-flags", "+low_delay",
		"-f", "rawvideo", "-pix_fmt", pixFmt(hdr.Format), "-video_size", fmt.Sprintf("%dx%d", hdr.Width, hdr.Height),
		"-framerate", strconv.FormatFloat(fps, 'f', 4, 64), "-i", "pipe:0", "-an", "-vf", vf, "-c:v", name)
	args = append(args, encArgs...)
	args = append(args, "-fps_mode", "passthrough", "-flush_packets", "1")
	if codec == "vp8" {
		args = append(args, "-f", "ivf", "pipe:1")
	} else {
		args = append(args, "-f", "flv", "-flvflags", "no_duration_filesize", "pipe:1")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, s.cfg.FFmpeg, args...)
	hideWindow(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr := &tailWriter{max: 500}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	e := &videoEncoder{name: name, codec: codec, hdr: hdr, cmd: cmd, stdin: stdin, cancel: cancel, got: make(chan struct{}), stopped: make(chan struct{})}
	go func() {
		defer close(e.stopped)
		emit := func(au AU) {
			e.gotOnce.Do(func() { close(e.got) })
			s.deliverVideo(codec, au)
		}
		var err error
		if codec == "vp8" {
			err = readIVF(stdout, emit)
		} else {
			err = readFLV(stdout, emit)
		}
		_ = cmd.Wait()
		select {
		case <-e.got:
		default:
			if ctx.Err() == nil { // it never produced a frame, and nobody stopped it
				s.log.Warn("video encoder failed to start; trying the next", "encoder", name, "err", err, "stderr", stderr.String())
				s.markBadEncoder(name)
				s.encoderDied(e)
			}
		}
	}()
	return e, nil
}

func encoderPixFmt(name string) string {
	if name == "h264_vaapi" || name == "h264_qsv" {
		return "nv12"
	}
	return "yuv420p"
}

func (e *videoEncoder) stop() {
	_ = e.stdin.Close()
	go func() {
		select {
		case <-e.stopped:
		case <-time.After(2 * time.Second):
		}
		e.cancel()
	}()
}

// videoLoop feeds the newest frame to whatever needs it. A slow encoder blocks
// only this loop: the frames that arrive in the meantime replace each other, so
// a queue can never build up and add delay.
func (s *Session) videoLoop() {
	var feed []byte
	var hdr ipc.VideoHeader
	for {
		select {
		case <-s.done:
			return
		case <-s.frameReady:
		}
		s.frameMu.Lock()
		if len(s.frame) == 0 {
			s.frameMu.Unlock()
			continue
		}
		feed = append(feed[:0], s.frame...)
		hdr = s.frameHdr
		s.frameMu.Unlock()

		s.viewMu.Lock()
		h264, vp8, jp := s.syncEncodersLocked(hdr)
		s.viewMu.Unlock()
		for _, e := range []*videoEncoder{h264, vp8} {
			if e != nil {
				if _, err := e.stdin.Write(feed); err != nil && !errors.Is(err, os.ErrClosed) {
					s.log.Debug("writing to the encoder", "err", err)
				}
			}
		}
		if jp != nil {
			jp.offer(hdr, feed)
		}
	}
}

// syncEncodersLocked starts what the viewers need and restarts anything whose
// frame size has changed. Consoles change resolution now and then; a restart
// costs a moment, and the decoder picks up the new size at the next keyframe.
func (s *Session) syncEncodersLocked(hdr ipc.VideoHeader) (h264, vp8 *videoEncoder, jp *jpegEncoder) {
	wantH264, wantVP8, wantJPEG := false, false, false
	for v := range s.viewers {
		switch v.codec {
		case "h264":
			wantH264 = true
		case "vp8":
			wantVP8 = true
		case "jpeg":
			wantJPEG = true
		}
	}
	ensure := func(cur *videoEncoder, want bool, codec string) *videoEncoder {
		if cur != nil && (!want || cur.hdr != hdr) {
			cur.stop()
			cur = nil
		}
		if !want || cur != nil {
			return cur
		}
		for _, name := range s.chain(codec) {
			e, err := s.startVideoEncoder(codec, name, hdr, s.info.FPS)
			if err != nil {
				s.log.Warn("starting encoder", "encoder", name, "err", err)
				s.markBadEncoder(name)
				continue
			}
			s.encName = name
			s.log.Info("encoding video", "encoder", name, "size", fmt.Sprintf("%dx%d", hdr.Width, hdr.Height))
			return e
		}
		s.log.Warn("no working encoder for this codec", "codec", codec)
		return nil
	}
	s.h264 = ensure(s.h264, wantH264, "h264")
	s.vp8 = ensure(s.vp8, wantVP8, "vp8")
	if wantJPEG && s.jpeg == nil {
		s.jpeg = newJPEGEncoder(s)
	} else if !wantJPEG && s.jpeg != nil {
		s.jpeg.stop()
		s.jpeg = nil
	}
	return s.h264, s.vp8, s.jpeg
}

// encoderDied clears a failed encoder so the next frame tries the next one.
func (s *Session) encoderDied(e *videoEncoder) {
	s.viewMu.Lock()
	defer s.viewMu.Unlock()
	if s.h264 == e {
		s.h264 = nil
	}
	if s.vp8 == e {
		s.vp8 = nil
	}
}

func (s *Session) stopEncodersLocked() {
	if s.h264 != nil {
		s.h264.stop()
		s.h264 = nil
	}
	if s.vp8 != nil {
		s.vp8.stop()
		s.vp8 = nil
	}
	if s.jpeg != nil {
		s.jpeg.stop()
		s.jpeg = nil
	}
	s.stopOpusLocked()
}

// ---- FLV: frames with exact boundaries, so a finished frame never waits ----

func readFLV(r io.Reader, emit func(AU)) error {
	var head [13]byte // the file header and the first "previous tag size"
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return err
	}
	var sps, pps [][]byte
	lenSize := 4
	var tag [11]byte
	var buf []byte
	for {
		if _, err := io.ReadFull(r, tag[:]); err != nil {
			return err
		}
		n := int(tag[1])<<16 | int(tag[2])<<8 | int(tag[3])
		if cap(buf) < n+4 {
			buf = make([]byte, n+4)
		}
		buf = buf[:n+4]
		if _, err := io.ReadFull(r, buf); err != nil {
			return err
		}
		if tag[0] != 9 || n < 5 || buf[0]&0x0f != 7 { // not AVC video
			continue
		}
		d := buf[5:n]
		switch buf[1] {
		case 0:
			sps, pps, lenSize = parseAVCC(d)
		case 1:
			key := buf[0]>>4 == 1
			out := make([]byte, 0, len(d)+128)
			if key {
				for _, set := range [][][]byte{sps, pps} {
					for _, nal := range set {
						out = append(out, 0, 0, 0, 1)
						out = append(out, nal...)
					}
				}
			}
			for len(d) >= lenSize {
				var l int
				for i := 0; i < lenSize; i++ {
					l = l<<8 | int(d[i])
				}
				d = d[lenSize:]
				if l < 0 || l > len(d) {
					return errors.New("flv: a NAL unit runs past its frame")
				}
				out = append(out, 0, 0, 0, 1)
				out = append(out, d[:l]...)
				d = d[l:]
			}
			emit(AU{Data: out, Key: key})
		}
	}
}

// parseAVCC reads an AVCDecoderConfigurationRecord: the stream's parameter sets.
func parseAVCC(d []byte) (sps, pps [][]byte, lenSize int) {
	lenSize = 4
	if len(d) < 6 {
		return
	}
	lenSize = int(d[4]&3) + 1
	n := int(d[5] & 0x1f)
	d = d[6:]
	read := func(count int, into *[][]byte) bool {
		for i := 0; i < count; i++ {
			if len(d) < 2 {
				return false
			}
			l := int(binary.BigEndian.Uint16(d))
			if len(d) < 2+l {
				return false
			}
			*into = append(*into, append([]byte(nil), d[2:2+l]...))
			d = d[2+l:]
		}
		return true
	}
	if !read(n, &sps) || len(d) < 1 {
		return
	}
	n = int(d[0])
	d = d[1:]
	read(n, &pps)
	return
}

func readIVF(r io.Reader, emit func(AU)) error {
	var head [32]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return err
	}
	var fh [12]byte
	for {
		if _, err := io.ReadFull(r, fh[:]); err != nil {
			return err
		}
		n := binary.LittleEndian.Uint32(fh[:])
		if n > 64<<20 {
			return errors.New("ivf: frame too large")
		}
		b := make([]byte, n)
		if _, err := io.ReadFull(r, b); err != nil {
			return err
		}
		if n > 0 {
			emit(AU{Data: b, Key: b[0]&1 == 0})
		}
	}
}

// ---- audio ----

// audioEncoder turns PCM into Opus packets for WebRTC.
type audioEncoder struct {
	stdin  io.WriteCloser
	cancel context.CancelFunc
}

func (s *Session) startOpusLocked() {
	if s.opus != nil || s.rate <= 0 {
		return
	}
	if !s.cfg.HaveEncoder("libopus") {
		s.log.Warn("this FFmpeg has no Opus encoder; WebRTC will have no sound")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, s.cfg.FFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-fflags", "+nobuffer",
		"-f", "s16le", "-ar", strconv.Itoa(int(s.rate)), "-ac", "2", "-i", "pipe:0", "-vn",
		"-c:a", "libopus", "-ar", "48000", "-b:a", "96k", "-application", "lowdelay", "-frame_duration", "10",
		"-flush_packets", "1", "-page_duration", "10000", "-f", "ogg", "pipe:1")
	hideWindow(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return
	}
	if err := cmd.Start(); err != nil {
		cancel()
		s.log.Warn("starting the audio encoder", "err", err)
		return
	}
	s.opus = &audioEncoder{stdin: stdin, cancel: cancel}
	go func() {
		_ = readOgg(stdout, s.deliverOpus)
		_ = cmd.Wait()
	}()
}

func (s *Session) stopOpusLocked() {
	if s.opus != nil {
		_ = s.opus.stdin.Close()
		s.opus.cancel()
		s.opus = nil
	}
}

// audioLoop hands the game's sound to the encoder and to WebSocket viewers.
func (s *Session) audioLoop() {
	for {
		select {
		case <-s.done:
			return
		case b := <-s.audioCh:
			s.viewMu.Lock()
			var w io.Writer
			if s.opus != nil {
				w = s.opus.stdin
			}
			s.viewMu.Unlock()
			if w != nil {
				_, _ = w.Write(b)
			}
			s.deliverPCM(b)
		}
	}
}

// readOgg calls emit with each Opus packet, skipping the two header packets.
func readOgg(r io.Reader, emit func([]byte)) error {
	var hdr [27]byte
	var packet []byte
	seen := 0
	for {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return err
		}
		if string(hdr[:4]) != "OggS" {
			return errors.New("ogg: lost sync")
		}
		var lacing [255]byte
		segs := lacing[:hdr[26]]
		if _, err := io.ReadFull(r, segs); err != nil {
			return err
		}
		for _, l := range segs {
			start := len(packet)
			packet = append(packet, make([]byte, l)...)
			if _, err := io.ReadFull(r, packet[start:]); err != nil {
				return err
			}
			if l < 255 {
				if seen >= 2 {
					emit(packet)
				}
				seen++
				packet = nil
			}
		}
	}
}

// ---- JPEG, the last resort: encoded here, no FFmpeg needed ----

type jpegEncoder struct {
	s    *Session
	mu   sync.Mutex
	hdr  ipc.VideoHeader
	buf  []byte
	wake chan struct{}
	quit chan struct{}
}

func newJPEGEncoder(s *Session) *jpegEncoder {
	j := &jpegEncoder{s: s, wake: make(chan struct{}, 1), quit: make(chan struct{})}
	go j.loop()
	return j
}

func (j *jpegEncoder) offer(hdr ipc.VideoHeader, data []byte) {
	j.mu.Lock()
	j.hdr, j.buf = hdr, append(j.buf[:0], data...)
	j.mu.Unlock()
	select {
	case j.wake <- struct{}{}:
	default:
	}
}

func (j *jpegEncoder) stop() { close(j.quit) }

func (j *jpegEncoder) loop() {
	var out bytes.Buffer
	var work []byte
	for {
		select {
		case <-j.quit:
			return
		case <-j.wake:
		}
		j.mu.Lock()
		hdr := j.hdr
		work = append(work[:0], j.buf...)
		j.mu.Unlock()
		out.Reset()
		if err := jpeg.Encode(&out, toRGBA(hdr, work), &jpeg.Options{Quality: 75}); err == nil {
			j.s.deliverJPEG(out.Bytes())
		}
	}
}

// toRGBA converts a raw frame. The header and size have already been checked.
func toRGBA(hdr ipc.VideoHeader, data []byte) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, hdr.Width, hdr.Height))
	p := img.Pix
	switch hdr.Format {
	case engine.FormatXRGB8888:
		for i := 0; i+3 < len(data) && i+3 < len(p); i += 4 {
			p[i], p[i+1], p[i+2], p[i+3] = data[i+2], data[i+1], data[i], 0xff
		}
	case engine.FormatRGB565:
		for i := 0; i+1 < len(data) && i*2+3 < len(p); i += 2 {
			v := uint16(data[i]) | uint16(data[i+1])<<8
			r, g, b := byte(v>>11), byte(v>>5)&0x3f, byte(v)&0x1f
			p[i*2], p[i*2+1], p[i*2+2], p[i*2+3] = r<<3|r>>2, g<<2|g>>4, b<<3|b>>2, 0xff
		}
	default:
		for i := 0; i+1 < len(data) && i*2+3 < len(p); i += 2 {
			v := uint16(data[i]) | uint16(data[i+1])<<8
			r, g, b := byte(v>>10)&0x1f, byte(v>>5)&0x1f, byte(v)&0x1f
			p[i*2], p[i*2+1], p[i*2+2], p[i*2+3] = r<<3|r>>2, g<<3|g>>2, b<<3|b>>2, 0xff
		}
	}
	return img
}

// writeThumb saves the current picture beside a save state.
func (s *Session) writeThumb(name string) {
	s.frameMu.Lock()
	if len(s.frame) == 0 {
		s.frameMu.Unlock()
		return
	}
	hdr, data := s.frameHdr, append([]byte(nil), s.frame...)
	s.frameMu.Unlock()
	var out bytes.Buffer
	if err := png.Encode(&out, toRGBA(hdr, data)); err == nil {
		_ = writeAtomic(filepath.Join(s.opt.SaveDir, name+".png"), out.Bytes())
	}
}

// tailWriter keeps the end of an error stream for the log.
type tailWriter struct {
	mu  sync.Mutex
	max int
	b   []byte
}

func (t *tailWriter) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > t.max {
		t.b = t.b[len(t.b)-t.max:]
	}
	return len(p), nil
}

func (t *tailWriter) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}
