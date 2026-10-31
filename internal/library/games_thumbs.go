package library

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // pictures come in these three kinds, and WebP is served as it is
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// Small covers for lists. Box art comes at full size, and a page of 120 games at a few
// hundred kilobytes each is a lot for a phone or an old TV. A smaller copy is made the
// first time one is asked for, and kept beside the original in the image cache.

const (
	// A picture bigger than this isn't a cover, and decoding it could take all the memory
	// a small machine has.
	maxThumbPixels = 16 << 20
	thumbMaxWidth  = 640
)

var (
	gameThumbOnce  sync.Once
	gameThumbSlots chan struct{}
)

func gameThumbSlot() chan struct{} {
	gameThumbOnce.Do(func() { gameThumbSlots = make(chan struct{}, max(1, runtime.NumCPU()/2)) })
	return gameThumbSlots
}

// PosterThumb returns a smaller copy of a game's cover as a "cache:" reference, or the
// reference it was given when it can't or needn't be made smaller. src is a "cache:"
// reference or a picture beside a game, already known to be inside a library.
func (g *GameMeta) PosterThumb(ctx context.Context, src string, width int) string {
	if width <= 0 || width > thumbMaxWidth || src == "" {
		return src
	}
	width = SnapThumbWidth(width)
	path := src
	if strings.HasPrefix(src, "cache:") {
		name := strings.TrimPrefix(src, "cache:")
		if name == "" || name != filepath.Base(name) {
			return src
		}
		path = filepath.Join(g.ImagesDir, name)
	} else if isRemote(src) {
		return src
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return src
	}
	h := sha1.Sum([]byte(src + "|" + st.ModTime().String()))
	name := "gamethumb-" + hex.EncodeToString(h[:12]) + "-" + strconv.Itoa(width) + ".jpg"
	out := filepath.Join(g.ImagesDir, name)
	if st, err := os.Stat(out); err == nil && !st.IsDir() {
		return "cache:" + name
	}
	select {
	case gameThumbSlot() <- struct{}{}:
		defer func() { <-gameThumbSlot() }()
	case <-ctx.Done():
		return src
	}
	if err := makeGameThumb(path, out, width); err != nil {
		return src
	}
	return "cache:" + name
}

var errThumbNotNeeded = errors.New("no smaller copy needed")

func makeGameThumb(path, out string, width int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return err
	}
	if cfg.Width <= width || cfg.Height <= 0 || cfg.Width*cfg.Height > maxThumbPixels {
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
	tmp, err := os.CreateTemp(filepath.Dir(out), ".gamethumb-*")
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
