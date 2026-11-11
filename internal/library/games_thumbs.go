// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"github.com/wittywilla/wittymediaserver/internal/pace"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // pictures come in these three kinds, and WebP is served as it is
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Small posters and covers for lists. Box art and posters come at full size, and a page
// of 120 at a few hundred kilobytes each is a lot for a phone or an old TV. A smaller copy
// is made the first time one is asked for, and kept beside the original in the image
// cache. (The files are still named gamethumb-, from when only games had them.)

const (
	// A picture bigger than this isn't a cover, and decoding it could take all the memory
	// a small machine has.
	maxThumbPixels = 16 << 20
	// Posters are asked for up to 640 wide, backdrops up to a screen's width.
	thumbMaxWidth = 1920
)

var (
	gameThumbOnce  sync.Once
	gameThumbSlots chan struct{}
)

func gameThumbSlot() chan struct{} {
	// Each holds a decoded picture twice over, up to 128 MB for the largest allowed, so
	// a page of new posters mustn't decode a dozen at once on a small machine.
	gameThumbOnce.Do(func() { gameThumbSlots = make(chan struct{}, min(2, pace.Slots(pace.Thumbs))) })
	return gameThumbSlots
}

// PosterThumb returns a smaller copy of a picture in the image cache, src a "cache:"
// reference, as a "cache:" reference, or src when it can't or needn't be made smaller.
// A picture beside the media goes to SmallCopyOf, opened inside its library.
func (g *GameMeta) PosterThumb(ctx context.Context, src string, width int) string {
	name, ok := strings.CutPrefix(src, "cache:")
	if !ok || name == "" || name != filepath.Base(name) || width <= 0 || width > thumbMaxWidth {
		return src
	}
	f, err := os.Open(filepath.Join(g.ImagesDir, name))
	if err != nil {
		return src
	}
	defer f.Close()
	return g.SmallCopyOf(ctx, f, src, width)
}

// SmallCopyOf is PosterThumb for a picture already open, which is what's read. A picture
// beside the media is opened by Store.OpenLibraryFile, which checks it's inside a library
// in one open, where checking it by name again looks at every folder on the way (37 looks
// for each poster on the WSL test machine's media drive). src names it, for the copy's name.
func (g *GameMeta) SmallCopyOf(ctx context.Context, f *os.File, src string, width int) string {
	if width <= 0 || width > thumbMaxWidth || src == "" {
		return src
	}
	width = SnapThumbWidth(width)
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		return src
	}
	h := sha1.Sum([]byte(src + "|" + st.ModTime().String()))
	name := "gamethumb-" + hex.EncodeToString(h[:12]) + "-" + strconv.Itoa(width) + ".jpg"
	out := filepath.Join(g.ImagesDir, name)
	if st, err := os.Stat(out); err == nil && !st.IsDir() {
		return "cache:" + name
	}
	// Before waiting for a turn: a picture already small enough, or one that can't be
	// read, mustn't queue behind the large ones being decoded.
	if !worthShrinking(f, width) {
		return src
	}
	select {
	case gameThumbSlot() <- struct{}{}:
		defer func() { <-gameThumbSlot() }()
	case <-ctx.Done():
		return src
	}
	if err := makeGameThumb(f, out, width); err != nil {
		return src
	}
	return "cache:" + name
}

var errThumbNotNeeded = errors.New("no smaller copy needed")

// worthShrinking reads a picture's header: it's one this can decode, wider than width,
// and not so large that decoding it could take all of a small machine's memory. f is left
// at its start, for whoever reads it next.
func worthShrinking(f io.ReadSeeker, width int) bool {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return false
	}
	cfg, _, err := image.DecodeConfig(f)
	if _, serr := f.Seek(0, io.SeekStart); serr != nil {
		return false
	}
	return err == nil && cfg.Width > width && cfg.Height > 0 && cfg.Width*cfg.Height <= maxThumbPixels
}

func makeGameThumb(f io.ReadSeeker, out string, width int) error {
	if !worthShrinking(f, width) {
		return errThumbNotNeeded
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	src, _, err := image.Decode(f)
	if err != nil {
		return err
	}
	dst := shrink(src, width)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	// Named like the copies, so that one a crash leaves behind is trimmed with them.
	tmp, err := os.CreateTemp(filepath.Dir(out), "gamethumb-part-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := jpeg.Encode(tmp, dst, &jpeg.Options{Quality: 82}); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), out)
}

// shrink makes a picture narrower by averaging the pixels each new one covers, which keeps
// fine print on box art readable. See-through parts become dark, as JPEG has no way to
// say otherwise.
func shrink(src image.Image, width int) *image.RGBA {
	b := src.Bounds()
	height := max(1, b.Dy()*width/b.Dx())
	flat := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(flat, flat.Bounds(), image.NewUniform(color.RGBA{16, 16, 20, 255}), image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), src, b.Min, draw.Over)
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		y0, y1 := y*b.Dy()/height, max((y+1)*b.Dy()/height, y*b.Dy()/height+1)
		for x := 0; x < width; x++ {
			x0, x1 := x*b.Dx()/width, max((x+1)*b.Dx()/width, x*b.Dx()/width+1)
			var r, g, bl, n uint32
			for sy := y0; sy < y1; sy++ {
				row := flat.Pix[sy*flat.Stride+x0*4 : sy*flat.Stride+x1*4]
				for i := 0; i+3 < len(row); i += 4 {
					r += uint32(row[i])
					g += uint32(row[i+1])
					bl += uint32(row[i+2])
					n++
				}
			}
			i := y*dst.Stride + x*4
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/n), uint8(g/n), uint8(bl/n), 255
		}
	}
	return dst
}
