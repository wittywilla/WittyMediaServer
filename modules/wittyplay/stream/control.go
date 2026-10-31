package stream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittyplay/ipc"
)

// How a running game is steered, and what it can report. Only the session's owner gets
// here, and everything arriving is held to the limits the server set when the game
// started.

// Save slots. 0 is automatic, 1 to MaxSlot are a person's own, and two more are kept
// for mistakes: the state from before the last load, and the automatic save that the
// last "start over" would otherwise have lost.
const (
	MaxSlot      = ipc.MaxStateSlot
	SlotUndo     = ipc.SlotUndo
	SlotPrevAuto = ipc.SlotPrevAuto
)

// MaxSaveBytes is how much one account may keep for one game.
const MaxSaveBytes = 4 << 30

// ErrSaveFull: this game's saves have reached their limit.
var ErrSaveFull = errors.New("this game's saved states have used all the space they may; delete some first")

// controlState is the session's side of the runner's controls.
type controlState struct {
	ctlMu      sync.Mutex
	speed      float64
	want       ipc.Control // what the settings last asked for
	options    []ipc.Option
	optionsSet chan struct{} // closed at the first list
	optionOnce sync.Once
	run        ipc.RunStats

	noAutoSave atomic.Bool
	framesIn   atomic.Int64
	framesFed  atomic.Int64
	dropped    atomic.Int64
	framePend  atomic.Bool
	bytesOut   atomic.Int64
	lastAUH264 atomic.Int64
	lastAUVP8  atomic.Int64
	statPrev   statSample
}

type statSample struct {
	at                   time.Time
	frames, bytes, enc   int64
	encNs                int64
	fed                  int64
	dropped              int64
	fps, kbps, encMs     float64
	shownPerSec, gamePer float64
}

func (s *Session) noteFrame(sizeChanged bool, hdr ipc.VideoHeader) {
	s.framesIn.Add(1)
	if sizeChanged {
		// What's drawn on the viewer's device needs the game's real pixels, which aren't
		// always what the core first said, and the video may be scaled on its way there.
		b, _ := json.Marshal(ipc.Event{Type: "frame", Width: hdr.Width, Height: hdr.Height})
		s.broadcastEvent(b)
	}
	if s.framePend.Swap(true) {
		s.dropped.Add(1) // the last one was replaced before it reached the encoder
	}
}

// Stats is what the player's on-screen numbers show. Reading it does no work beyond
// a few counters, and only a client that shows them asks.
type Stats struct {
	GameFPS       float64 `json:"gameFps"`       // frames the game ran each second
	SentFPS       float64 `json:"sentFps"`       // frames the runner sent each second
	Skipped       int     `json:"skipped"`       // frames left out in the last second
	EncodedFPS    float64 `json:"encodedFps"`    // frames the encoder finished each second
	EncodeMs      float64 `json:"encodeMs"`      // time to encode a frame
	Kbps          float64 `json:"kbps"`          // what's being sent, over the last second
	DroppedFrames int64   `json:"droppedFrames"` // frames an encoder never got, since the start
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	Core          string  `json:"core"`
	Encoder       string  `json:"encoder,omitempty"`
	Speed         float64 `json:"speed"`
	Paused        bool    `json:"paused"`
	RewindBytes   int64   `json:"rewindBytes,omitempty"`
	RewindFrames  int     `json:"rewindFrames,omitempty"`
}

// Stats reports the session's numbers since the last time anyone asked.
func (s *Session) Stats() Stats {
	s.ctlMu.Lock()
	defer s.ctlMu.Unlock()
	now := time.Now()
	p := &s.statPrev
	frames, enc, encNs, b := s.framesFed.Load(), s.stats.frames.Load(), s.stats.totalUS.Load()*1000, s.bytesOut.Load()
	if dt := now.Sub(p.at).Seconds(); dt >= 0.25 && !p.at.IsZero() {
		p.fps = float64(enc-p.enc) / dt
		p.kbps = float64(b-p.bytes) * 8 / 1000 / dt
		if n := enc - p.enc; n > 0 {
			p.encMs = float64(encNs-p.encNs) / float64(n) / 1e6
		}
	}
	p.at, p.frames, p.enc, p.encNs, p.bytes = now, frames, enc, encNs, b
	s.viewMu.Lock()
	hdrW, hdrH, paused, core, enc0 := s.info.Width, s.info.Height, s.paused, s.info.Name, s.encName
	s.viewMu.Unlock()
	s.frameMu.Lock()
	if s.frameHdr.Width > 0 {
		hdrW, hdrH = s.frameHdr.Width, s.frameHdr.Height
	}
	s.frameMu.Unlock()
	speed := s.speed
	if speed == 0 {
		speed = 1
	}
	run := s.run
	if paused { // a paused runner reports nothing, and what it last said is out of date
		run.Ran, run.Shown, run.Skipped = 0, 0, 0
	}
	return Stats{GameFPS: float64(run.Ran), SentFPS: float64(run.Shown), Skipped: run.Skipped, EncodedFPS: p.fps, EncodeMs: p.encMs, Kbps: p.kbps,
		DroppedFrames: s.dropped.Load(), Width: hdrW, Height: hdrH, Core: core, Encoder: enc0, Speed: speed, Paused: paused, RewindBytes: run.RewindBytes, RewindFrames: run.RewindFrames}
}

// auDuration is how long a frame lasts on the wire: the real time since the last one.
// Frame skip and fast-forward change the rate, and a stream whose clock runs slow
// makes a browser's buffer grow.
func (s *Session) auDuration(codec string) time.Duration {
	last := &s.lastAUH264
	if codec == "vp8" {
		last = &s.lastAUVP8
	}
	now := time.Now().UnixNano()
	prev := last.Swap(now)
	nominal := time.Duration(float64(time.Second) / s.info.FPS)
	if prev == 0 {
		return nominal
	}
	d := time.Duration(now - prev)
	if d < time.Millisecond || d > 250*time.Millisecond {
		return nominal
	}
	return d
}

func (s *Session) control(c ipc.Control) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return s.command(ipc.CmdControl, b)
}

// MaxSpeed is the fastest this session may run.
func (s *Session) maxSpeed() float64 {
	if s.opt.MaxSpeed >= 1 {
		return min(s.opt.MaxSpeed, ipc.SpeedMax)
	}
	return 1
}

// SetSpeed changes how fast the game runs, and says what it was held to.
func (s *Session) SetSpeed(v float64) (float64, error) {
	if v != v || v <= 0 {
		return 0, errors.New("that isn't a speed")
	}
	v = min(max(v, ipc.SpeedMin), s.maxSpeed())
	s.ctlMu.Lock()
	s.speed = v
	s.ctlMu.Unlock()
	return v, s.control(ipc.Control{Speed: &v})
}

// SetRewinding goes backwards while on. It's refused when rewind isn't turned on, so
// that a button doing nothing can say why.
func (s *Session) SetRewinding(on bool) error {
	s.ctlMu.Lock()
	enabled := s.want.RewindBytes != nil && *s.want.RewindBytes > 0
	s.ctlMu.Unlock()
	if on && !enabled {
		return errors.New("rewind is off; turn it on in this game's settings")
	}
	return s.control(ipc.Control{Rewinding: &on})
}

// Advance runs a paused game for a few frames.
func (s *Session) Advance(frames int) error {
	frames = min(max(frames, 1), ipc.AdvanceMax)
	return s.control(ipc.Control{Advance: &frames})
}

// Apply brings a running game into line with new settings. Only what differs from
// the last time is sent, and a safe-mode game takes nothing: it's running with
// defaults so that someone can get out of a mess.
func (s *Session) Apply(c ipc.Control, coreOptions map[string]string) error {
	if s.opt.SafeMode {
		return nil
	}
	s.ctlMu.Lock()
	send := diffControl(s.want, c)
	s.want = c
	opts := s.options
	s.ctlMu.Unlock()
	if send != (ipc.Control{}) {
		if err := s.control(send); err != nil {
			return err
		}
	}
	// An option nobody sets any more goes back to the core's own default. One set to
	// something the core doesn't list is left as it is: it would be ignored anyway.
	for _, o := range opts {
		want, ok := coreOptions[o.Key]
		if !ok {
			want = o.Default
		}
		if want != o.Value && slices.Contains(o.Choices, want) {
			if err := s.sendOption(o.Key, want); err != nil {
				return err
			}
		}
	}
	return nil
}

// SetAutoSave says whether leaving writes the resume state.
func (s *Session) SetAutoSave(on bool) { s.noAutoSave.Store(!on) }

func diffControl(a, b ipc.Control) ipc.Control {
	var d ipc.Control
	if b.FrameSkip != nil && (a.FrameSkip == nil || *a.FrameSkip != *b.FrameSkip) {
		d.FrameSkip = b.FrameSkip
	}
	if b.RunAhead != nil && (a.RunAhead == nil || *a.RunAhead != *b.RunAhead) {
		d.RunAhead = b.RunAhead
	}
	if b.RewindBytes != nil && (a.RewindBytes == nil || *a.RewindBytes != *b.RewindBytes) {
		d.RewindBytes = b.RewindBytes
	}
	if b.RewindSeconds != nil && (a.RewindSeconds == nil || *a.RewindSeconds != *b.RewindSeconds) {
		d.RewindSeconds = b.RewindSeconds
	}
	return d
}

// SetOption changes one core option for this session. It's refused here when it can be,
// so that a client gets an answer. The runner checks again whatever it's sent.
func (s *Session) SetOption(key, value string) error {
	s.ctlMu.Lock()
	var offered *ipc.Option
	for i := range s.options {
		if s.options[i].Key == key {
			offered = &s.options[i]
		}
	}
	ok := offered != nil && slices.Contains(offered.Choices, value)
	s.ctlMu.Unlock()
	if !ok {
		return errors.New("this core doesn't offer that option or value")
	}
	return s.sendOption(key, value)
}

func (s *Session) sendOption(key, value string) error {
	b, err := json.Marshal(ipc.OptionSet{Key: key, Value: value})
	if err != nil {
		return err
	}
	return s.command(ipc.CmdOption, b)
}

// SetCheats replaces the list of cheats.
func (s *Session) SetCheats(list []ipc.Cheat) error {
	if s.opt.SafeMode {
		return nil
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return s.command(ipc.CmdCheats, b)
}

// CoreOptions is what the core offers and what each is set to.
func (s *Session) CoreOptions() []ipc.Option {
	s.ctlMu.Lock()
	defer s.ctlMu.Unlock()
	return append([]ipc.Option(nil), s.options...)
}

// Safe says this game started with defaults, whatever its settings say.
func (s *Session) Safe() bool { return s.opt.SafeMode }

// WaitOptions waits a moment for the core's list, for a client asking right after a start.
func (s *Session) WaitOptions(ctx context.Context) {
	select {
	case <-s.optionsSet:
	case <-ctx.Done():
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}

// Limits on what a runner may say about its options. It can't be trusted on this any
// more than on anything else, and these get stored and shown.
const (
	maxOffered       = 1024
	maxOfferedChoice = 512
	maxOfferedText   = 300
	maxOfferedInfo   = 1000
)

func cleanOffered(list []ipc.Option) []ipc.Option {
	if len(list) > maxOffered {
		list = list[:maxOffered]
	}
	out := make([]ipc.Option, 0, len(list))
	for _, o := range list {
		if o.Key == "" || len(o.Key) > ipc.MaxOptionKey || len(o.Choices) == 0 {
			continue
		}
		if len(o.Choices) > maxOfferedChoice {
			o.Choices = o.Choices[:maxOfferedChoice]
		}
		clean := make([]string, 0, len(o.Choices))
		names := make([]string, 0, len(o.Choices))
		for i, c := range o.Choices {
			if len(c) <= ipc.MaxOptionValue {
				clean = append(clean, c)
				name := c
				if i < len(o.Labels) && o.Labels[i] != "" {
					name = truncate(o.Labels[i], maxOfferedText)
				}
				names = append(names, name)
			}
		}
		if len(clean) == 0 {
			continue
		}
		o.Choices, o.Labels = clean, names
		if !slices.Contains(clean, o.Default) {
			o.Default = clean[0]
		}
		o.Label = truncate(o.Label, maxOfferedText)
		o.Info = truncate(o.Info, maxOfferedInfo)
		o.Category = truncate(o.Category, maxOfferedText)
		o.Value = truncate(o.Value, ipc.MaxOptionValue)
		out = append(out, o)
	}
	return out
}

func (s *Session) noteEvent(e ipc.Event) (handled bool) {
	switch e.Type {
	case "stats":
		if e.Stats != nil {
			s.ctlMu.Lock()
			s.run = *e.Stats
			s.ctlMu.Unlock()
		}
		return true
	case "options":
		list := cleanOffered(e.Options)
		s.ctlMu.Lock()
		s.options = list
		s.ctlMu.Unlock()
		s.optionOnce.Do(func() { close(s.optionsSet) })
		if s.opt.OnCoreOptions != nil {
			go s.opt.OnCoreOptions(list)
		}
		return true
	case "statefail":
		e.Text = "That saved state couldn’t be loaded. The game is still as it was, or use Undo load."
		e.Type = "message"
		b, _ := json.Marshal(e)
		s.broadcastEvent(b)
		return true
	}
	return false
}

// sendStart gives a runner what it should have from the first frame.
func (s *Session) sendStart() {
	s.noAutoSave.Store(s.opt.NoAutoSave)
	if s.opt.SafeMode {
		return
	}
	if s.opt.Control != nil {
		s.ctlMu.Lock()
		s.want = *s.opt.Control
		s.ctlMu.Unlock()
		_ = s.control(*s.opt.Control)
	}
	if len(s.opt.Cheats) > 0 {
		_ = s.SetCheats(s.opt.Cheats)
	}
}

// saveDirSize adds up what's in this game's folder.
func saveDirSize(dir string) int64 {
	var n int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !info.IsDir() {
			n += info.Size()
		}
	}
	return n
}

// keepUndo saves the game as it is now, so that a load can be taken back.
func (s *Session) keepUndo(ctx context.Context) {
	b, err := s.requestState(ctx)
	if err != nil || len(b) == 0 {
		return
	}
	if saveDirSize(s.opt.SaveDir)+int64(len(b)) > MaxSaveBytes {
		return
	}
	name, _ := SlotName(SlotUndo)
	if err := writeAtomic(filepath.Join(s.opt.SaveDir, name+".state"), b); err == nil {
		s.writeThumb(name)
	}
}

// Undo goes back to before the last load, and the game as it stands becomes the undo,
// so pressing it again goes forward.
func (s *Session) Undo(ctx context.Context) error {
	name, _ := SlotName(SlotUndo)
	path := filepath.Join(s.opt.SaveDir, name+".state")
	b, err := readLimited(path)
	if err != nil {
		return errors.New("there's nothing to undo")
	}
	s.keepUndo(ctx)
	return s.command(ipc.CmdState, b)
}

// Screenshot is the current picture as a PNG, without any effects: those only exist on
// the device showing them.
func (s *Session) Screenshot() ([]byte, error) {
	s.frameMu.Lock()
	if len(s.frame) == 0 {
		s.frameMu.Unlock()
		return nil, errors.New("there's no picture yet")
	}
	hdr, data := s.frameHdr, append([]byte(nil), s.frame...)
	s.frameMu.Unlock()
	var out bytes.Buffer
	if err := png.Encode(&out, toRGBA(hdr, data)); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// SlotMeta is written beside a save state: which core made it.
type SlotMeta struct {
	Core string `json:"core"`
	At   int64  `json:"at"`
}

func (s *Session) writeSlotMeta(name string) {
	s.viewMu.Lock()
	core := s.info.Name
	s.viewMu.Unlock()
	b, _ := json.Marshal(SlotMeta{Core: core, At: time.Now().Unix()})
	_ = writeAtomic(filepath.Join(s.opt.SaveDir, name+".json"), b)
}

// keepPrevAuto moves a resume state out of the way when the session that's about to
// replace it didn't start from it, which would otherwise lose it for good.
func (s *Session) keepPrevAuto() {
	if s.resumed {
		return
	}
	auto, _ := SlotName(0)
	prev, _ := SlotName(SlotPrevAuto)
	for _, ext := range []string{".state", ".png", ".json"} {
		from := filepath.Join(s.opt.SaveDir, auto+ext)
		if _, err := os.Stat(from); err == nil {
			_ = os.Rename(from, filepath.Join(s.opt.SaveDir, prev+ext))
		}
	}
}

// ProbeOptions runs a core with no game for long enough to ask what options it offers.
// Most cores say as soon as they load; a core that only says with a game loaded gives
// an empty list, and the list comes at the next play. The runner is boxed in as it
// always is, in a throwaway folder.
func (m *Manager) ProbeOptions(ctx context.Context, core, systemDir, tmp string) ([]ipc.Option, error) {
	cfg := m.config()
	if cfg.Runner == "" {
		return nil, errors.New("the game runner is missing from this install")
	}
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(tmp, "probe-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	args := append(append([]string(nil), cfg.RunnerArgs...), "-save", dir, "-memory-mb", strconv.FormatInt(cfg.MemoryMB, 10))
	cmd := exec.CommandContext(ctx, cfg.Runner, args...)
	cmd.Dir = dir
	cmd.Env = runnerEnv()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	prepareSandbox(cmd, dir, m.log)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting the game runner: %w", err)
	}
	box := lockDown(cmd, cfg.MemoryMB, m.log)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if box != nil {
			box()
		}
	}()
	load, _ := json.Marshal(ipc.Load{Core: core, System: systemDir, Save: dir, Probe: true})
	if err := ipc.Write(stdin, ipc.CmdLoad, load); err != nil {
		return nil, err
	}
	rd := ipc.NewReader(stdout)
	for {
		t, p, err := rd.Next()
		if err != nil {
			return nil, errors.New("the core could not be started to ask for its options")
		}
		switch t {
		case ipc.MsgEvent:
			var e ipc.Event
			if json.Unmarshal(p, &e) == nil && e.Type == "options" {
				return cleanOffered(e.Options), nil
			}
		case ipc.MsgError:
			return nil, errors.New(truncate(string(p), 300))
		}
	}
}
