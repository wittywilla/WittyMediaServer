// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/pace"
)

// Thumbnails made the first time they are asked for: scanning a library of a
// hundred thousand photos costs no FFmpeg at all, and pictures nobody opens
// cost nothing. A few run at a time, each once however many ask for it, and the
// result stays in the cache, named after the file's identity and size.

// LazyPoster in an item's poster field means: made on demand by Thumb.
const LazyPoster = "lazy:"

// ThumbWidths are the sizes that can be asked for (the longer side, in pixels).
// A fixed list keeps the cache from filling with every width someone could type.
var ThumbWidths = []int{160, 320, 480, 640, 1280, 1920, 2560}

// SnapThumbWidth is the smallest listed size that covers w.
func SnapThumbWidth(w int) int {
	for _, t := range ThumbWidths {
		if w <= t {
			return t
		}
	}
	return ThumbWidths[len(ThumbWidths)-1]
}

type thumbCall struct {
	done chan struct{}
	err  error
}

// A picture that can't be made isn't tried again for this long. Some damaged files keep FFmpeg
// working at full speed until it's stopped, and a page of them would otherwise do that again on
// every visit, in the slots everyone else's pictures wait for. The key holds the file's date and
// size, so a replaced file is tried at once; the list has a limit, and when it's full the oldest
// half goes, which at worst costs those files one more try.
const (
	thumbFailMemory = time.Hour
	maxThumbFails   = 4096
)

var (
	thumbMu     sync.Mutex
	thumbCalls  = map[string]*thumbCall{}
	thumbFails  = map[string]thumbFailure{}
	thumbSlotsO sync.Once
	thumbSlots  chan struct{}
)

type thumbFailure struct {
	err error
	at  time.Time
}

// noteThumbFailure remembers a failure. Call with thumbMu held.
func noteThumbFailure(key string, err error) {
	if len(thumbFails) >= maxThumbFails {
		oldest := make([]time.Time, 0, len(thumbFails))
		for _, f := range thumbFails {
			oldest = append(oldest, f.at)
		}
		slices.SortFunc(oldest, time.Time.Compare)
		cut := oldest[len(oldest)/2]
		for k, f := range thumbFails {
			if !f.at.After(cut) {
				delete(thumbFails, k)
			}
		}
	}
	thumbFails[key] = thumbFailure{err, time.Now()}
}

func thumbSlot() chan struct{} {
	thumbSlotsO.Do(func() {
		thumbSlots = make(chan struct{}, pace.Slots(pace.Thumbs))
	})
	return thumbSlots
}

// Thumb returns the path of a JPEG of a picture or video, at most width pixels
// along its longer side (a video's width).
func (s *Scanner) Thumb(ctx context.Context, it *Item, width int) (string, error) {
	width = SnapThumbWidth(width)
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d|%d", it.Path, it.MTime, it.SizeBytes, width)))
	key := hex.EncodeToString(sum[:10])
	dir := filepath.Join(s.CacheDir, "thumbs", key[:2])
	out := filepath.Join(dir, key+".jpg")
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out, nil
	}
	// A link swapped in after the scan doesn't get read.
	if !s.Store.LibraryFile(ctx, it.Path) {
		return "", ErrNotFound
	}
	thumbMu.Lock()
	if f, ok := thumbFails[key]; ok {
		if time.Since(f.at) < thumbFailMemory {
			thumbMu.Unlock()
			return "", f.err
		}
		delete(thumbFails, key)
	}
	if c, ok := thumbCalls[key]; ok {
		thumbMu.Unlock()
		select {
		case <-c.done:
			return out, c.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	c := &thumbCall{done: make(chan struct{})}
	thumbCalls[key] = c
	thumbMu.Unlock()
	// The work outlives a viewer who scrolled past: the next one gets it from the cache.
	go func() {
		defer func() {
			if p := recover(); p != nil {
				logging.ReportPanic("thumbnail", p, "cat", "library")
				c.err = fmt.Errorf("could not read this file (%v)", p)
			}
			if c.err != nil {
				s.Log.Warn("could not make a thumbnail", "file", it.Path, "err", c.err)
			}
			thumbMu.Lock()
			delete(thumbCalls, key)
			if c.err != nil {
				noteThumbFailure(key, c.err)
			}
			thumbMu.Unlock()
			close(c.done)
		}()
		slot := thumbSlot()
		slot <- struct{}{}
		defer func() { <-slot }()
		wctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			c.err = err
			return
		}
		tmp := filepath.Join(dir, key+".tmp.jpg")
		defer os.Remove(tmp)
		if c.err = s.makeThumb(wctx, it, width, tmp); c.err != nil {
			return
		}
		c.err = os.Rename(tmp, out)
	}()
	select {
	case <-c.done:
		return out, c.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *Scanner) makeThumb(ctx context.Context, it *Item, width int, out string) error {
	if it.Kind == "video" {
		at := it.RuntimeSec * 0.1
		if at > 30 {
			at = 30
		}
		return s.Tools.Poster(ctx, ffmpeg.FileInput(it.Path), at, width, out)
	}
	orientation, _ := s.Store.photoExtras(ctx, it.ID)
	src, longSide := it.Path, it.Width
	if it.Height > longSide {
		longSide = it.Height
	}
	if photos().IsRaw(it.Path) {
		// A camera's own development of the picture, stored in the file.
		preview, err := photos().RawPreview(it.Path)
		if err == nil {
			src = out + ".raw.jpg"
			if err := os.WriteFile(src, preview, 0o644); err != nil {
				return err
			}
			defer os.Remove(src)
			longSide = 0 // the preview's size isn't the sensor's
		} else if !errors.Is(err, ErrNoRawPreview) {
			return err
		}
	}
	err := s.Tools.ImageThumb(ctx, src, orientation, width, jpegShrink(src, longSide, width), out)
	if err != nil && strings.EqualFold(filepath.Ext(it.Path), ".psd") {
		// Photoshop keeps a small JPEG in the file, which is better than nothing.
		if preview := psdPreview(it.Path); preview != nil {
			tmp := out + ".psd.jpg"
			if werr := os.WriteFile(tmp, preview, 0o644); werr != nil {
				return werr
			}
			defer os.Remove(tmp)
			return s.Tools.ImageThumb(ctx, tmp, orientation, width, 0, out)
		}
	}
	return err
}

// psdPreview finds the JPEG in a Photoshop file's resources. Every length in it comes
// from the file, so each is checked against what's left.
func psdPreview(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	const maxHead = 16 << 20
	b := make([]byte, maxHead)
	n, _ := io.ReadFull(f, b)
	b = b[:n]
	be32 := func(at int) int {
		if at < 0 || at+4 > len(b) {
			return -1
		}
		return int(binary.BigEndian.Uint32(b[at:]))
	}
	if len(b) < 26 || string(b[:4]) != "8BPS" {
		return nil
	}
	colorLen := be32(26)
	if colorLen < 0 {
		return nil
	}
	at := 30 + colorLen
	resLen := be32(at)
	if resLen < 0 {
		return nil
	}
	end := min(at+4+resLen, len(b))
	for p := at + 4; p+12 <= end; {
		if string(b[p:p+4]) != "8BIM" {
			return nil
		}
		id := int(binary.BigEndian.Uint16(b[p+4:]))
		nameLen := int(b[p+6])
		pad := (nameLen + 2) &^ 1
		sizeAt := p + 6 + pad
		size := be32(sizeAt)
		if size < 0 || sizeAt+4+size > end {
			return nil
		}
		if (id == 1036 || id == 1033) && size > 28 {
			// A thumbnail resource: a 28-byte header, then the JPEG.
			j := b[sizeAt+4+28 : sizeAt+4+size]
			if len(j) > 3 && j[0] == 0xFF && j[1] == 0xD8 {
				return j
			}
		}
		p = sizeAt + 4 + (size+1)&^1
	}
	return nil
}

// jpegShrink: how many times a JPEG can halve while decoding and still cover the
// width, which saves most of the time and memory on large photos.
func jpegShrink(path string, longSide, width int) int {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".jpe" {
		return 0
	}
	if longSide <= 0 || width <= 0 || longSide < width*2 {
		return 0
	}
	n := bits.Len(uint(longSide/width)) - 1
	if n > 3 {
		n = 3
	}
	return n
}
