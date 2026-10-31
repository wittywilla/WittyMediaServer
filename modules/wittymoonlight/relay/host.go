// Package relay carries a game stream between the Moonlight host program and a browser. The
// host program holds Moonlight's GPL-3.0 code and is a separate program, reached only by the
// messages in modules/wittymoonlight-host/PROTOCOL.md: this package contains none of it.
package relay

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// What the host program sends.
const (
	msgEvent  = 1
	msgVideo  = 2
	msgAudio  = 3
	msgRumble = 4
)

// What it reads. The numbers from CmdPad up are also what a browser sends.
const (
	cmdStart         = 64
	CmdPad           = 65
	CmdMouseMove     = 66
	CmdMouseButton   = 67
	CmdKey           = 68
	CmdScroll        = 69
	CmdIDR           = 70
	cmdQuit          = 71
	CmdMousePosition = 72
	CmdText          = 73
	CmdHScroll       = 74
)

const (
	maxOutMessage = 16 << 20 // a keyframe at a high bitrate
	// ProtocolVersion is the host program this package speaks to.
	ProtocolVersion = 1
)

// Start is what a host program needs to connect. The PC has already been told to start the
// game, over its own client protocol, by the time this is used.
type Start struct {
	Address     string // an IP, resolved and checked by the caller
	AppVersion  string
	GfeVersion  string
	RTSP        string
	CodecModes  int
	Width       int
	Height      int
	FPS         int
	BitrateKbps int
	PacketSize  int
	Remote      bool
	Key, IV     [16]byte
}

func (s Start) message() []byte {
	remote := 0
	if s.Remote {
		remote = 1
	}
	var b strings.Builder
	line := func(k, v string) { b.WriteString(k + "=" + v + "\n") }
	line("address", s.Address)
	line("appversion", s.AppVersion)
	line("gfeversion", s.GfeVersion)
	line("rtsp", s.RTSP)
	line("scm", strconv.Itoa(s.CodecModes))
	line("width", strconv.Itoa(s.Width))
	line("height", strconv.Itoa(s.Height))
	line("fps", strconv.Itoa(s.FPS))
	line("bitrate", strconv.Itoa(s.BitrateKbps))
	line("packetsize", strconv.Itoa(s.PacketSize))
	line("remote", strconv.Itoa(remote))
	line("formats", "1") // H.264: what every browser can decode
	line("rikey", hex.EncodeToString(s.Key[:]))
	line("riiv", hex.EncodeToString(s.IV[:]))
	return []byte(b.String())
}

// Event is something the host program reports.
type Event struct {
	Type     string `json:"type"`
	Stage    string `json:"stage,omitempty"`
	Code     int    `json:"code,omitempty"`
	Poor     bool   `json:"poor,omitempty"`
	Format   int    `json:"format,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	FPS      int    `json:"fps,omitempty"`
	Rate     int    `json:"rate,omitempty"`
	Channels int    `json:"channels,omitempty"`
	Samples  int    `json:"samples,omitempty"`
	Streams  int    `json:"streams,omitempty"`
	Coupled  int    `json:"coupled,omitempty"`
	Protocol int    `json:"protocol,omitempty"`
}

// Sink receives what the host program produces. Calls come from one goroutine.
type Sink interface {
	Video(keyframe bool, timestamp uint32, annexB []byte)
	Audio(opus []byte)
	Rumble(controller, low, high uint16)
	Event(e Event)
}

// Host is one running host program, for one stream.
type Host struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	out   io.ReadCloser
	log   *slog.Logger
	sink  Sink

	wmu    sync.Mutex
	done   chan struct{}
	closer sync.Once
}

// ErrNoProgram: the program isn't installed here.
var ErrNoProgram = errors.New("the Moonlight host program isn't installed on this server")

// StartHost runs the program. It holds nothing and reads nothing it's not sent: no address,
// key or user name ever goes on its command line, where other programs could read it.
func StartHost(program string, prepare func(*exec.Cmd), log *slog.Logger, sink Sink) (*Host, error) {
	if program == "" {
		return nil, ErrNoProgram
	}
	cmd := exec.Command(program)
	cmd.Env = []string{}
	if root := os.Getenv("SystemRoot"); root != "" {
		cmd.Env = append(cmd.Env, "SystemRoot="+root) // Windows sockets don't work without it
	}
	if prepare != nil {
		prepare(cmd)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("the Moonlight host program couldn't start: %w", err)
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	h := &Host{cmd: cmd, stdin: stdin, out: out, log: log, sink: sink, done: make(chan struct{})}
	go h.read()
	return h, nil
}

// Done closes when the program has ended.
func (h *Host) Done() <-chan struct{} { return h.done }

// Send writes one message. Anything sent after the program ends goes nowhere.
func (h *Host) send(kind byte, payload []byte) error {
	head := [5]byte{kind}
	binary.LittleEndian.PutUint32(head[1:], uint32(len(payload)))
	h.wmu.Lock()
	defer h.wmu.Unlock()
	if _, err := h.stdin.Write(head[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := h.stdin.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// Begin connects to the PC.
func (h *Host) Begin(s Start) error { return h.send(cmdStart, s.message()) }

// inputSizes are the only input messages there are, with their exact sizes: a browser can't
// send anything else, and in particular can't start or end anything this way.
var inputSizes = map[byte]int{CmdPad: 18, CmdMouseMove: 4, CmdMouseButton: 2, CmdKey: 4, CmdScroll: 2, CmdIDR: 0, CmdMousePosition: 8, CmdHScroll: 2}

// ValidInput checks a message from a browser: its type byte, then its body.
func ValidInput(m []byte) bool {
	if len(m) == 0 {
		return false
	}
	if m[0] == CmdText {
		return len(m) > 1 && len(m) <= 257
	}
	n, ok := inputSizes[m[0]]
	return ok && len(m) == n+1
}

// Input passes on one message a browser sent, if it's a valid one.
func (h *Host) Input(m []byte) error {
	if !ValidInput(m) {
		return errors.New("not an input message")
	}
	return h.send(m[0], m[1:])
}

// Close ends the stream, and the program a moment later if it hasn't gone by itself.
func (h *Host) Close() {
	h.closer.Do(func() {
		_ = h.send(cmdQuit, nil)
		_ = h.stdin.Close()
		select {
		case <-h.done:
		case <-time.After(3 * time.Second):
			_ = h.cmd.Process.Kill()
			<-h.done
		}
	})
}

func (h *Host) read() {
	defer func() {
		_ = h.cmd.Process.Kill()
		_ = h.cmd.Wait()
		close(h.done)
	}()
	r := bufio.NewReaderSize(h.out, 1<<20)
	var head [5]byte
	buf := make([]byte, 1<<20)
	for {
		if _, err := io.ReadFull(r, head[:]); err != nil {
			return
		}
		n := binary.LittleEndian.Uint32(head[1:])
		// What this program says is treated as untrusted: it parses what a stranger's PC sent.
		if n > maxOutMessage {
			h.log.Warn("moonlight host sent too much at once", "bytes", n)
			return
		}
		if int(n) > len(buf) {
			buf = make([]byte, n)
		}
		p := buf[:n]
		if _, err := io.ReadFull(r, p); err != nil {
			return
		}
		switch head[0] {
		case msgVideo:
			if n >= 5 {
				h.sink.Video(p[0]&1 != 0, binary.LittleEndian.Uint32(p[1:5]), p[5:])
			}
		case msgAudio:
			h.sink.Audio(p)
		case msgRumble:
			if n == 6 {
				h.sink.Rumble(binary.LittleEndian.Uint16(p), binary.LittleEndian.Uint16(p[2:]), binary.LittleEndian.Uint16(p[4:]))
			}
		case msgEvent:
			var e Event
			if err := json.Unmarshal(p, &e); err == nil {
				h.sink.Event(e)
			}
		}
	}
}

// CheckProgram runs the program once to see that it can run here and speaks this protocol.
func CheckProgram(ctx context.Context, program string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, program, "--version").Output()
	if err != nil {
		return fmt.Errorf("the Moonlight host program couldn't run: %w", err)
	}
	if !strings.Contains(string(out), "protocol "+strconv.Itoa(ProtocolVersion)) {
		return errors.New("the Moonlight host program is a different version from this server")
	}
	return nil
}
