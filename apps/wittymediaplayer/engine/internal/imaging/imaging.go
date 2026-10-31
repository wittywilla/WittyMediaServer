// Package imaging copies a disc to an .iso or .img file, sector by sector, for a
// WittyMediaServer library or for playing later.
//
// It only ever copies what the drive hands over. A protected disc is refused
// before anything is written, and any scrambled sector met on the way stops the
// copy and removes the partial file: nothing here unlocks, decrypts or works
// around copy protection.
package imaging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittymediaplayer/engine/internal/disc"
)

const (
	sector = 2048
	// chunk is read at a time: a whole number of DVD (16 sector) and Blu-ray (32
	// sector) error-correction blocks, so a damaged block costs one retry.
	chunkSectors = 64
	// retries is how often an unreadable chunk is tried again before going sector
	// by sector.
	retries = 2
	// maxBad stops a copy of a disc that has clearly failed, at about 100 MB.
	maxBad = 51200
)

// Source is a disc to read.
type Source interface {
	io.ReaderAt
	Size() int64
}

// Progress reports how a copy is going.
type Progress struct {
	Event       string  `json:"event"` // progress | done | error
	Done        int64   `json:"done"`
	Total       int64   `json:"total"`
	BadSectors  int64   `json:"badSectors,omitempty"`
	BytesPerSec float64 `json:"bytesPerSec,omitempty"`
	Path        string  `json:"path,omitempty"`
	Kind        string  `json:"kind,omitempty"`
	Verified    bool    `json:"verified,omitempty"`
	Code        string  `json:"code,omitempty"`
	Message     string  `json:"message,omitempty"`
}

// Options say what to copy where.
type Options struct {
	Out string
	// Kind is what the disc is, from disc.Classify. A DVD's sectors are watched
	// for scrambling as they are copied.
	Kind     string
	Progress func(Progress)
	// Every is how often progress is reported.
	Every time.Duration
}

// Refusal reasons for the destination, in plain words.
var (
	ErrExists = errors.New("a file with that name already exists, and copying a disc never overwrites anything")
	ErrFormat = errors.New("the image's name has to end in .iso or .img")
)

// CheckOut looks at a destination before any reading starts.
func CheckOut(out string, size int64) error {
	switch strings.ToLower(filepath.Ext(out)) {
	case ".iso", ".img":
	default:
		return ErrFormat
	}
	if _, err := os.Lstat(out); err == nil {
		return ErrExists
	}
	dir := filepath.Dir(out)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return fmt.Errorf("the folder %s doesn't exist", dir)
	}
	if free, ok := freeSpace(dir); ok && free < uint64(size)+(64<<20) {
		return fmt.Errorf("there isn't enough room: the disc needs %s and the folder's drive has %s free", humanBytes(uint64(size)), humanBytes(free))
	}
	return nil
}

func humanBytes(n uint64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	default:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	}
}

// Copy reads the whole source into Out. It writes to a temporary name beside the
// destination and takes that name only when it has finished, so a stopped or
// refused copy leaves nothing a library would pick up.
func Copy(ctx context.Context, src Source, opt Options) (res Progress, err error) {
	total := src.Size()
	res = Progress{Event: "done", Total: total, Path: opt.Out, Kind: opt.Kind}
	if total <= 0 || total%sector != 0 {
		return res, fmt.Errorf("the disc reports an unusable size (%d bytes)", total)
	}
	if err := CheckOut(opt.Out, total); err != nil {
		return res, err
	}
	part := opt.Out + ".part"
	// O_EXCL: never write over a partial copy someone else may be making.
	f, err := os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return res, err
	}
	done := false
	defer func() {
		f.Close()
		if !done {
			_ = os.Remove(part)
		}
	}()
	every := opt.Every
	if every <= 0 {
		every = 500 * time.Millisecond
	}
	buf := make([]byte, chunkSectors*sector)
	started, last := time.Now(), time.Now()
	for at := int64(0); at < total; {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		n := min(int64(len(buf)), total-at)
		bad, err := readChunk(ctx, src, buf[:n], at)
		if err != nil {
			return res, err
		}
		res.BadSectors += bad
		if res.BadSectors > maxBad {
			return res, fmt.Errorf("too much of this disc can't be read (over %d sectors), so the copy has stopped", maxBad)
		}
		if opt.Kind == disc.KindDVD {
			for s := int64(0); s < n/sector; s++ {
				if wittydvd.PackScrambled(buf[s*sector : (s+1)*sector]) {
					return res, &disc.RefusalError{Code: disc.CodeProtected, Message: disc.MsgDVDProtected}
				}
			}
		}
		if _, err := f.Write(buf[:n]); err != nil {
			return res, fmt.Errorf("the image couldn't be written: %w", err)
		}
		at += n
		res.Done = at
		if opt.Progress != nil && time.Since(last) >= every {
			last = time.Now()
			p := res
			p.Event = "progress"
			p.BytesPerSec = float64(at) / time.Since(started).Seconds()
			opt.Progress(p)
		}
	}
	if err := f.Sync(); err != nil {
		return res, fmt.Errorf("the image couldn't be written: %w", err)
	}
	if err := f.Close(); err != nil {
		return res, fmt.Errorf("the image couldn't be written: %w", err)
	}
	// Final check of the name: something may have appeared there meanwhile.
	if _, err := os.Lstat(opt.Out); err == nil {
		return res, ErrExists
	}
	if err := os.Rename(part, opt.Out); err != nil {
		return res, err
	}
	done = true
	res.Verified = Verify(opt.Out, opt.Kind) == nil
	return res, nil
}

// readChunk fills buf from the source. A chunk that won't read is retried, then
// read sector by sector; sectors that stay unreadable become zeros, counted so
// the viewer can be told the copy isn't perfect.
func readChunk(ctx context.Context, src Source, buf []byte, at int64) (bad int64, err error) {
	for try := 0; try <= retries; try++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		n, err := src.ReadAt(buf, at)
		if n == len(buf) {
			return 0, nil
		}
		if err == nil || errors.Is(err, io.EOF) {
			break // a short read with no error: count what is missing below
		}
	}
	for off := int64(0); off < int64(len(buf)); off += sector {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		s := buf[off : off+sector]
		if n, _ := src.ReadAt(s, at+off); n != len(s) {
			clear(s)
			bad++
		}
	}
	return bad, nil
}

// Verify opens a finished image with the same engines WittyMediaServer uses, so
// a library will read it too.
func Verify(path, kind string) error {
	got, err := disc.Classify(path)
	if err != nil {
		return err
	}
	if kind != "" && got != kind {
		return fmt.Errorf("the image reads as %s, not %s", got, kind)
	}
	if got == disc.KindDVD || got == disc.KindBluray {
		return disc.CheckProtected(path, got, false)
	}
	return nil
}
