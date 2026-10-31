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
	case MsgReady, MsgEvent, MsgError, CmdLoad:
		return MaxJSON
	case MsgState, MsgSaveRAM, CmdSaveRAM, CmdState:
		return MaxBlob
	case MsgDupe, CmdSaveState, CmdReset, CmdPause, CmdQuit, CmdInput, CmdDisc:
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
	Disc       int     `json:"disc,omitempty"`  // the disc in the drive, from 0
	Discs      int     `json:"discs,omitempty"` // how many the game has; 0 or 1 is nothing to choose
}

// Event is something a player should be told about.
type Event struct {
	Type     string  `json:"type"` // rumble | message | geometry | rotation | log | shutdown | avinfo | statefail | trace | disc
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
// right sticks' X and Y. A browser sends the same bytes.
const InputSize = 11

// ParseInput reads one player's state.
func ParseInput(p []byte) (port int, buttons uint16, lx, ly, rx, ry int16, ok bool) {
	if len(p) != InputSize {
		return 0, 0, 0, 0, 0, 0, false
	}
	return int(p[0]), binary.LittleEndian.Uint16(p[1:]), int16(binary.LittleEndian.Uint16(p[3:])), int16(binary.LittleEndian.Uint16(p[5:])),
		int16(binary.LittleEndian.Uint16(p[7:])), int16(binary.LittleEndian.Uint16(p[9:])), true
}
