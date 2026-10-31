package library

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"math/bits"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittyphotos"
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

var (
	thumbMu     sync.Mutex
	thumbCalls  = map[string]*thumbCall{}
	thumbSlotsO sync.Once
	thumbSlots  chan struct{}
)

func thumbSlot() chan struct{} {
	thumbSlotsO.Do(func() {
		n := runtime.NumCPU() / 2
		if n < 1 {
			n = 1
		}
		if n > 3 {
			n = 3
		}
		thumbSlots = make(chan struct{}, n)
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
	thumbMu.Lock()
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
				c.err = fmt.Errorf("could not read this file (%v)", p)
			}
			thumbMu.Lock()
			delete(thumbCalls, key)
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
		return s.Tools.Still(ctx, ffmpeg.FileInput(it.Path), at, width, out)
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
		} else if !errors.Is(err, wittyphotos.ErrNoPreview) {
			return err
		}
	}
	return s.Tools.ImageThumb(ctx, src, orientation, width, jpegShrink(src, longSide, width), out)
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
