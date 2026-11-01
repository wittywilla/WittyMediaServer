package modules

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/cd"
	"github.com/wittywilla/wittymodule"
)

const cdID = "wittycd"

const (
	// cdCallTime is how long a helper has for one read: long enough for a slow share.
	cdCallTime = 60 * time.Second
	// cdChunk is how much is asked for at once: a few seconds of sound, so a read costs
	// little and playing never waits long.
	cdChunk       = 512 << 10
	maxCDTracks   = 512
	maxCDWarnings = 64
)

type pathParams struct {
	Path string `json:"path"`
}

// CDs reaches WittyCD: a helper for each open image, which an idle image lets go of.
type CDs struct{ M *Manager }

// Open opens an image. It is what cd.Open is set to.
func (c CDs) Open(path string) (*cd.Disc, error) {
	a := c.M.Attach(cdID, cdCallTime)
	if a == nil {
		return nil, c.M.Missing(cdID)
	}
	var d cd.Disc
	if err := openCD(a, path, &d); err != nil {
		a.Close()
		return nil, err
	}
	if err := checkCD(&d); err != nil {
		a.Close()
		a.failedNow(err)
		return nil, err
	}
	d.Engine = &cdDisc{a: a}
	return &d, nil
}

// openCD opens an image. A cue sheet over a sound file WittyCD can't read comes back naming it, and
// it's asked again with FLAC made here, where FFmpeg is. It may only name files beside the sheet.
func openCD(a *Attached, path string, d *cd.Disc) error {
	foreign := map[string]string{}
	for {
		_, err := a.Call("open", cdOpen{Path: path, Foreign: foreign}, d)
		var ce *wittymodule.CallError
		if err == nil || !errors.As(err, &ce) || ce.Code != "needs-flac" {
			return cdError(err)
		}
		if cd.ForeignAudio == nil || len(foreign) >= maxForeign {
			return fmt.Errorf("%w: its audio is in a format this server can't convert", cd.ErrUnsupported)
		}
		asked := strings.Split(ce.Message, "\n")
		for _, f := range asked {
			if _, done := foreign[f]; done || len(foreign) >= maxForeign || !besideSheet(path, f) {
				return fmt.Errorf("%w: it asked for a file it can't have", cd.ErrUnsupported)
			}
			flac, err := cd.ForeignAudio(f)
			if err != nil {
				return fmt.Errorf("%w: its audio can't be read: %v", cd.ErrUnsupported, err)
			}
			foreign[f] = flac
		}
	}
}

// maxForeign is more sound files than a sheet has any use for.
const maxForeign = 256

type cdOpen struct {
	Path    string            `json:"path"`
	Foreign map[string]string `json:"foreign,omitempty"`
}

// besideSheet: a file in the sheet's own folder, or below it.
func besideSheet(sheet, f string) bool {
	rel, err := filepath.Rel(filepath.Dir(sheet), f)
	return err == nil && filepath.IsAbs(f) && filepath.IsLocal(rel)
}

// cdError puts back the errors the server acts on.
func cdError(err error) error {
	var ce *wittymodule.CallError
	if errors.As(err, &ce) {
		switch ce.Code {
		case "not-cd":
			return cd.ErrNotCD
		case "no-audio":
			return cd.ErrNoAudio
		case "unsupported":
			return cd.ErrUnsupported
		}
	}
	return err
}

// checkCD refuses what can't be right. All of this goes to pages and a database as words and
// numbers, so what matters is that it stays a sensible size.
func checkCD(d *cd.Disc) error {
	if len(d.Tracks) > maxCDTracks || len(d.Warnings) > maxCDWarnings || d.TotalSize < 0 || !(d.Duration >= 0) {
		return errors.New("its answer about a disc can't be right")
	}
	return nil
}

// cdDisc is one open image in a helper.
type cdDisc struct {
	a *Attached
}

func (c *cdDisc) Close() error {
	c.a.Close()
	return nil
}

// ReadGraphics reads a run of a karaoke disc's subcode.
func (c *cdDisc) ReadGraphics(from, sectors int) ([]byte, error) {
	return c.a.Call("graphics", struct {
		From    int `json:"from"`
		Sectors int `json:"sectors"`
	}{from, sectors}, nil)
}

// NewStream makes a program, which the helper keeps until the disc closes.
func (c *cdDisc) NewStream(tracks []int, loops int, deemphasis bool) (cd.Stream, error) {
	var ans struct {
		ID      int   `json:"id"`
		Samples int64 `json:"samples"`
		Size    int64 `json:"size"`
	}
	if _, err := c.a.Call("stream", struct {
		Tracks     []int `json:"tracks"`
		Whole      bool  `json:"whole"`
		Loops      int   `json:"loops"`
		Deemphasis bool  `json:"deemphasis"`
	}{tracks, tracks == nil, loops, deemphasis}, &ans); err != nil {
		return nil, cdError(err)
	}
	if ans.Samples < 0 || ans.Size < 0 {
		return nil, errors.New("WittyCD's answer about a program can't be right")
	}
	return &cdStream{a: c.a, id: ans.ID, samples: ans.Samples, size: ans.Size}, nil
}

// cdStream reads a program a chunk at a time.
type cdStream struct {
	a       *Attached
	id      int
	samples int64
	size    int64
}

func (s *cdStream) Samples() int64 { return s.samples }
func (s *cdStream) Size() int64    { return s.size }

// ReadAt reads samples. Asking for more than a chunk just takes more calls.
func (s *cdStream) ReadAt(p []byte, off int64) (int, error) {
	total := 0
	for total < len(p) {
		if off+int64(total) >= s.size {
			return total, io.EOF
		}
		n := min(len(p)-total, cdChunk)
		b, err := s.a.Call("read", struct {
			ID  int   `json:"id"`
			Off int64 `json:"off"`
			N   int   `json:"n"`
		}{s.id, off + int64(total), n}, nil)
		if err != nil {
			return total, err
		}
		if len(b) == 0 {
			return total, io.EOF
		}
		if len(b) > n {
			return total, errors.New("WittyCD sent more than was asked for")
		}
		copy(p[total:], b)
		total += len(b)
	}
	return total, nil
}

// wavHeader is 44 bytes in front of the samples.
const wavHeader = 44

type cdWAV struct {
	s    *cdStream
	from int64
}

// WAV is the program from a sample on, as a WAV file. The samples come from the helper, and
// what's in front of them says only how long they are.
func (s *cdStream) WAV(startSample int64) cd.WAV {
	from := min(startSample*cd.SampleBytes, s.size)
	return &cdWAV{s: s, from: from}
}

func (w *cdWAV) dataSize() int64 { return min(w.s.size-w.from, 0xffffffff-36) }

func (w *cdWAV) Size() int64 { return wavHeader + w.dataSize() }

func (w *cdWAV) ReadAt(p []byte, off int64) (int, error) {
	n := 0
	if off < wavHeader {
		h := make([]byte, wavHeader)
		copy(h, "RIFF")
		binary.LittleEndian.PutUint32(h[4:], uint32(36+w.dataSize()))
		copy(h[8:], "WAVEfmt ")
		binary.LittleEndian.PutUint32(h[16:], 16)
		binary.LittleEndian.PutUint16(h[20:], 1)
		binary.LittleEndian.PutUint16(h[22:], 2)
		binary.LittleEndian.PutUint32(h[24:], cd.SampleRate)
		binary.LittleEndian.PutUint32(h[28:], cd.SampleRate*cd.SampleBytes)
		binary.LittleEndian.PutUint16(h[32:], cd.SampleBytes)
		binary.LittleEndian.PutUint16(h[34:], 16)
		copy(h[36:], "data")
		binary.LittleEndian.PutUint32(h[40:], uint32(w.dataSize()))
		n = copy(p, h[off:])
		p = p[n:]
		off = wavHeader
	}
	if len(p) == 0 {
		return n, nil
	}
	at := off - wavHeader
	if at >= w.dataSize() {
		if n > 0 {
			return n, nil
		}
		return 0, io.EOF
	}
	if rest := w.dataSize() - at; int64(len(p)) > rest {
		p = p[:rest]
	}
	got, err := w.s.ReadAt(p, w.from+at)
	return n + got, err
}

// WriteFLAC has the helper encode, a chunk at a time, so nothing but a helper's own memory
// is at stake whatever an image holds.
func (s *cdStream) WriteFLAC(w io.Writer, startSample int64) error {
	var start struct {
		ID int `json:"id"`
	}
	if _, err := s.a.Call("flac", struct {
		ID    int   `json:"id"`
		Start int64 `json:"start"`
	}{s.id, startSample}, &start); err != nil {
		return err
	}
	for {
		b, err := s.a.Call("flacnext", struct {
			ID int `json:"id"`
		}{start.ID}, nil)
		if err != nil {
			return fmt.Errorf("WittyCD: %w", err)
		}
		if len(b) == 0 {
			return nil
		}
		if _, err := w.Write(b); err != nil {
			_, _ = s.a.Call("flacstop", struct {
				ID int `json:"id"`
			}{start.ID}, nil)
			return err
		}
	}
}
