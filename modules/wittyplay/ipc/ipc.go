// Package ipc is the protocol between the server and a game runner process:
// length-prefixed messages over the runner's stdin and stdout.
//
// A runner is wittyplay-host, or any other program that speaks this protocol,
// such as WittyRetroArch. Talking over pipes is what keeps a GPL runner a
// separate program from the Apache-licensed server, so this package is the
// whole of what they share. docs/GAMING.md has the licensing reasons.
//
// The server treats everything a runner says as hostile. A core may have been
// taken over by a bad ROM, so every message has a size limit, read here before
// any memory is set aside.
package ipc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Messages from the host.
const (
	MsgReady   byte = 1 // JSON: Ready
	MsgVideo   byte = 2 // VideoHeader + tightly packed rows
	MsgDupe    byte = 3 // the previous frame again
	MsgAudio   byte = 4 // s16le stereo
	MsgState   byte = 5 // u32 request id + save state (empty: the core has none)
	MsgSaveRAM byte = 6 // battery save
	MsgEvent   byte = 7 // JSON: Event
	MsgError   byte = 8 // text, then the host exits
)

// Commands to the host.
const (
	CmdLoad      byte = 64 // JSON: Load
	CmdSaveRAM   byte = 65 // battery save to restore; sent before the first unpause
	CmdState     byte = 66 // a save state to restore
	CmdInput     byte = 67 // InputSize bytes
	CmdSaveState byte = 68 // u32 request id
	CmdReset     byte = 69
	CmdPause     byte = 70 // one byte: 1 paused, 0 running
	CmdQuit      byte = 71 // one byte: 1 sends a save state first; battery save is always sent
	CmdDisc      byte = 72 // one byte: the disc to change to, from 0
	CmdPointer   byte = 73 // PointerSize bytes: a touch screen or mouse
	CmdDevice    byte = 80 // DeviceSize bytes: plug a kind of controller into a port
	CmdControl   byte = 96 // JSON: Control; how the game runs
	CmdOption    byte = 97 // JSON: OptionSet; a core option, only ever one the core offered
	CmdCheats    byte = 98 // JSON: []Cheat; the whole list, replacing what was there
)

// Size limits. MaxVideoFrame is far above what a console produces (4K at four
// bytes a pixel), and MaxBlob holds the largest save states.
const (
	MaxVideoFrame = 4096 * 2304 * 4
	MaxAudio      = 1 << 20
	MaxJSON       = 1 << 20
	MaxBlob       = 512 << 20
	MaxDimension  = 4096
)

func limit(t byte) int {
	switch t {
	case MsgVideo:
		return MaxVideoFrame + VideoHeaderSize
	case MsgAudio:
		return MaxAudio
	case MsgReady, MsgEvent, MsgError, CmdLoad, CmdControl, CmdOption, CmdCheats:
		return MaxJSON
	case MsgState, MsgSaveRAM, CmdSaveRAM, CmdState:
		return MaxBlob
	case MsgDupe, CmdSaveState, CmdReset, CmdPause, CmdQuit, CmdInput, CmdDisc, CmdPointer, CmdDevice:
		return 16
	}
	return 0
}

// Reader reads messages; the returned payload is valid until the next call.
type Reader struct {
	r   io.Reader
	buf []byte
	hdr [5]byte
}

func NewReader(r io.Reader) *Reader { return &Reader{r: r} }

// Next reads one message. An unknown type or an oversized message is an error,
// and the connection is finished then.
func (r *Reader) Next() (t byte, payload []byte, err error) {
	if _, err = io.ReadFull(r.r, r.hdr[:]); err != nil {
		return 0, nil, err
	}
	t = r.hdr[0]
	n := binary.LittleEndian.Uint32(r.hdr[1:])
	max := limit(t)
	if max == 0 {
		return 0, nil, fmt.Errorf("ipc: unknown message type %d", t)
	}
	if n > uint32(max) {
		return 0, nil, fmt.Errorf("ipc: message type %d of %d bytes is over its limit", t, n)
	}
	if int(n) > cap(r.buf) {
		// Blobs grow as they arrive, so a false length costs nothing.
		if n > MaxVideoFrame+VideoHeaderSize {
			b, err := io.ReadAll(io.LimitReader(r.r, int64(n)))
			if err != nil {
				return 0, nil, err
			}
			if len(b) != int(n) {
				return 0, nil, io.ErrUnexpectedEOF
			}
			return t, b, nil
		}
		r.buf = make([]byte, n)
	}
	r.buf = r.buf[:n]
	if _, err = io.ReadFull(r.r, r.buf); err != nil {
		return 0, nil, err
	}
	return t, r.buf, nil
}

// Write sends one message. Small messages go out in a single write.
func Write(w io.Writer, t byte, parts ...[]byte) error {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	var hdr [5]byte
	hdr[0] = t
	binary.LittleEndian.PutUint32(hdr[1:], uint32(n))
	if n <= 512 {
		buf := make([]byte, 0, 5+n)
		buf = append(buf, hdr[:]...)
		for _, p := range parts {
			buf = append(buf, p...)
		}
		_, err := w.Write(buf)
		return err
	}
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	for _, p := range parts {
		if _, err := w.Write(p); err != nil {
			return err
		}
	}
	return nil
}

// Load is what a runner needs to start a game. Core TestCore is the built-in
// test core and any other value is a shared library path.
type Load struct {
	Core    string            `json:"core"`
	ROM     string            `json:"rom"`
	System  string            `json:"system"`
	Save    string            `json:"save"`
	Options map[string]string `json:"options,omitempty"`
	Trace   bool              `json:"trace,omitempty"` // send everything the core says as log events; for wittyplay-check
	// PreferDevices names the controller to plug in, by words in its name, best first.
	PreferDevices []string `json:"preferDevices,omitempty"`
	// Probe asks only which options the core offers: the runner sends them and exits
	// without a game. ROM is empty then.
	Probe bool `json:"probe,omitempty"`
}

// TestCore is the Load.Core value for the built-in test core.
const TestCore = "builtin:test"

// Ready says a game has loaded.
type Ready struct {
	Name       string  `json:"name"`
	Version    string  `json:"version"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Aspect     float64 `json:"aspect"`
	FPS        float64 `json:"fps"`
	SampleRate float64 `json:"sampleRate"`
	Disc       int     `json:"disc,omitempty"`     // the disc in the drive, from 0
	Discs      int     `json:"discs,omitempty"`    // how many the game has; 0 or 1 is nothing to choose
	Renderer   string  `json:"renderer,omitempty"` // the graphics card, for a game drawn on one
	// Devices: the kinds of controller this core offers, when it offers a choice.
	Devices []PortDevices `json:"devices,omitempty"`
}

// PortDevices lists what can be plugged into one port.
type PortDevices struct {
	Port    int          `json:"port"`
	Types   []DeviceType `json:"types"`
	Current uint32       `json:"current"`
}

// DeviceType is one kind of controller.
type DeviceType struct {
	ID   uint32 `json:"id"`
	Name string `json:"name"`
}

// Event is something a player should be told about.
type Event struct {
	Type     string  `json:"type"` // rumble | message | geometry | rotation | log | shutdown | avinfo | options | stats | statefail | frame | trace | disc
	Port     int     `json:"port,omitempty"`
	Effect   int     `json:"effect,omitempty"`
	Strength int     `json:"strength,omitempty"`
	Text     string  `json:"text,omitempty"`
	Level    int     `json:"level,omitempty"`
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
	Aspect   float64 `json:"aspect,omitempty"`
	Turns    int     `json:"turns,omitempty"`
	FPS      float64 `json:"fps,omitempty"`
	Rate     float64 `json:"rate,omitempty"`
	Index    int     `json:"index,omitempty"`
	Count    int     `json:"count,omitempty"`
	// Options is every option the core offers, for "options".
	Options []Option `json:"options,omitempty"`
	// Stats comes once a second while a game runs, for "stats".
	Stats *RunStats `json:"stats,omitempty"`
}

// Option is one setting a core offers. The first choice is the core's default.
type Option struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Info     string   `json:"info,omitempty"`
	Category string   `json:"category,omitempty"`
	Choices  []string `json:"choices"`
	Labels   []string `json:"labels,omitempty"` // the name of each choice, when it has one
	Default  string   `json:"default,omitempty"`
	Value    string   `json:"value"`
	Hidden   bool     `json:"hidden,omitempty"` // the core says it doesn't apply just now
}

// OptionSet changes one option. A runner ignores a key the core didn't offer or a
// value it didn't list, so this can't reach anything else.
type OptionSet struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Cheat is one code as a core's cheat interface takes it.
type Cheat struct {
	Code    string `json:"code"`
	Enabled bool   `json:"enabled"`
}

// Control changes how a game runs. Only what is present changes.
type Control struct {
	Speed         *float64 `json:"speed,omitempty"`         // 1 is normal; SpeedMin to SpeedMax
	FrameSkip     *int     `json:"frameSkip,omitempty"`     // FrameSkipAuto, 0 for none, or how many to skip after each shown
	RewindBytes   *int64   `json:"rewindBytes,omitempty"`   // memory rewind may use; 0 turns it off
	RewindSeconds *int     `json:"rewindSeconds,omitempty"` // how far back to keep
	Rewinding     *bool    `json:"rewinding,omitempty"`     // going backwards now
	RunAhead      *int     `json:"runAhead,omitempty"`      // frames; 0 to RunAheadMax
	Advance       *int     `json:"advance,omitempty"`       // frames to run while paused
}

// Save slots: 0 is automatic and 1 to MaxStateSlot belong to a person. The other two
// are kept for mistakes: what was running before the last load, and the automatic save
// that starting over would have replaced.
const (
	MaxStateSlot = 99
	SlotUndo     = 100
	SlotPrevAuto = 101
)

// Limits on what's said about a core's options.
const (
	MaxOptionKey   = 128
	MaxOptionValue = 128
)

// Limits a runner holds to whatever it's told.
const (
	SpeedMin       = 0.1
	SpeedMax       = 16.0
	FrameSkipAuto  = -1
	FrameSkipMax   = 8
	RunAheadMax    = 3
	RewindBytesMax = 512 << 20
	AdvanceMax     = 60
)

// RunStats says what a runner did over the last second.
type RunStats struct {
	Ran          int   `json:"ran"`                    // frames the game ran
	Shown        int   `json:"shown"`                  // of those, how many were sent
	Skipped      int   `json:"skipped"`                // left out to keep up or on request
	RewindBytes  int64 `json:"rewindBytes,omitempty"`  // memory rewind holds
	RewindFrames int   `json:"rewindFrames,omitempty"` // steps back it holds
}

// VideoHeader starts a MsgVideo payload.
type VideoHeader struct {
	Width, Height int
	Format        int
}

// VideoHeaderSize is the encoded size of a VideoHeader.
const VideoHeaderSize = 5

// Append adds the header to b.
func (h VideoHeader) Append(b []byte) []byte {
	b = binary.LittleEndian.AppendUint16(b, uint16(h.Width))
	b = binary.LittleEndian.AppendUint16(b, uint16(h.Height))
	return append(b, byte(h.Format))
}

// ParseVideo checks a MsgVideo payload: the sizes must add up exactly, so what
// goes on to FFmpeg is always a whole frame of the size it was told to expect.
func ParseVideo(p []byte) (VideoHeader, []byte, error) {
	if len(p) < VideoHeaderSize {
		return VideoHeader{}, nil, errors.New("ipc: short video message")
	}
	h := VideoHeader{Width: int(binary.LittleEndian.Uint16(p)), Height: int(binary.LittleEndian.Uint16(p[2:])), Format: int(p[4])}
	if h.Width == 0 || h.Height == 0 || h.Width > MaxDimension || h.Height > MaxDimension || h.Format > 2 {
		return VideoHeader{}, nil, fmt.Errorf("ipc: bad video header %+v", h)
	}
	bpp := 2
	if h.Format == 1 {
		bpp = 4
	}
	if len(p)-VideoHeaderSize != h.Width*h.Height*bpp {
		return VideoHeader{}, nil, errors.New("ipc: video frame size does not match its header")
	}
	return h, p[VideoHeaderSize:], nil
}

// InputSize is the size of a CmdInput payload: port, buttons, then the left and
// right sticks' X and Y. A browser sends the same bytes. InputSizeFull adds how far
// the L2 and R2 triggers are held, 0 to 255, for clients that can tell.
const (
	InputSize     = 11
	InputSizeFull = 13
)

// ParseTriggers reads the analog triggers of a state, which are 0 when it has none.
func ParseTriggers(p []byte) (l2, r2 uint8) {
	if len(p) != InputSizeFull {
		return 0, 0
	}
	return p[11], p[12]
}

// DeviceSize is the size of a CmdDevice payload: port, then a u32 device type.
const DeviceSize = 5

// ParseInput reads one player's state.
func ParseInput(p []byte) (port int, buttons uint16, lx, ly, rx, ry int16, ok bool) {
	if len(p) != InputSize && len(p) != InputSizeFull {
		return 0, 0, 0, 0, 0, 0, false
	}
	return int(p[0]), binary.LittleEndian.Uint16(p[1:]), int16(binary.LittleEndian.Uint16(p[3:])), int16(binary.LittleEndian.Uint16(p[5:])),
		int16(binary.LittleEndian.Uint16(p[7:])), int16(binary.LittleEndian.Uint16(p[9:])), true
}

// PointerSize is the size of a pointer message. Its first byte, PointerMark, can't
// begin a pad message, so a client sends both down the same channel. Then come
// flags, X and Y. X and Y run from -32767 to 32767 across the picture, the way
// libretro counts them.
const (
	PointerSize = 6
	PointerMark = 0xFF
)

// ParsePointer reads a touch screen or mouse state.
func ParsePointer(p []byte) (x, y int16, pressed, ok bool) {
	if len(p) != PointerSize || p[0] != PointerMark {
		return 0, 0, false, false
	}
	return int16(binary.LittleEndian.Uint16(p[2:])), int16(binary.LittleEndian.Uint16(p[4:])), p[1]&1 != 0, true
}
