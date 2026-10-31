package wittycd

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CD+G: a karaoke disc's pictures travel in the subcode, 4 packets of 24 bytes for
// every sector. They come from a .cdg file beside the image, or from CDG sectors
// that keep 96 bytes of subcode after the audio.

const (
	graphicsPacket  = 24
	graphicsPerSect = 4
	graphicsCommand = 0x09
)

type subcode struct {
	src    io.ReaderAt
	offset int64
	stride int64 // bytes from one sector's packets to the next
	first  int   // the sector the packets start at
	size   int64
}

// findGraphics looks for CD+G for a sheet-based image.
func (d *Disc) findGraphics(sheet string, files []*cueFile) {
	base := strings.TrimSuffix(sheet, filepath.Ext(sheet))
	if p := firstExisting(base+".cdg", base+".CDG"); p != "" {
		d.useCDGFile(p)
		return
	}
	for _, cf := range files {
		for _, s := range cf.segs {
			if s.stride == 2448 && cf.audio.src != nil {
				d.sub = &subcode{src: cf.audio.src, offset: cf.audio.offset + SectorBytes, stride: 2448, first: cf.base, size: cf.audio.size}
				d.HasGraphics = d.sub.looksLikeGraphics()
				if !d.HasGraphics {
					d.sub = nil
				}
				return
			}
		}
	}
}

// useCDGFile attaches a .cdg file. Its first packet goes with track 1's index 1,
// which finish has yet to settle, so the first sector is filled in on use.
func (d *Disc) useCDGFile(path string) {
	f, size, err := d.file(path)
	if err != nil || size < graphicsPacket {
		return
	}
	d.sub = &subcode{src: f, stride: graphicsPacket * graphicsPerSect, first: -1, size: size}
	d.HasGraphics = d.sub.looksLikeGraphics()
	if !d.HasGraphics {
		d.sub = nil
	}
}

func (g *subcode) looksLikeGraphics() bool {
	var pack [graphicsPacket * graphicsPerSect]byte
	for at := int64(0); at < g.stride*64 && at < g.size; at += g.stride {
		n, err := g.src.ReadAt(pack[:], g.offset+at)
		if err != nil && !errors.Is(err, io.EOF) {
			return false
		}
		for i := 0; i+graphicsPacket <= n; i += graphicsPacket {
			if pack[i]&0x3f == graphicsCommand {
				return true
			}
		}
	}
	return false
}

// ReadGraphics returns the CD+G packets of a run of sectors, each packet's bytes
// masked to the 6 bits that carry graphics. Sectors with no graphics read as
// zeroes, which a decoder ignores.
func (d *Disc) ReadGraphics(from, sectors int) ([]byte, error) {
	if d.sub == nil {
		return nil, errors.New("this disc has no CD+G graphics")
	}
	if sectors < 0 || sectors > MaxSectors {
		return nil, errors.New("too many sectors of graphics at once")
	}
	first := d.sub.first
	if first < 0 {
		if a := d.AudioTracks(); len(a) > 0 {
			first = a[0].Start
		}
	}
	out := make([]byte, sectors*graphicsPacket*graphicsPerSect)
	for i := range sectors {
		at := from + i - first
		if at < 0 {
			continue
		}
		pack := out[i*graphicsPacket*graphicsPerSect : (i+1)*graphicsPacket*graphicsPerSect]
		off := d.sub.offset + int64(at)*d.sub.stride
		if off >= d.sub.offset+d.sub.size {
			break
		}
		if _, err := d.sub.src.ReadAt(pack, off); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
			return nil, err
		}
	}
	for i := range out {
		out[i] &= 0x3f
	}
	return out, nil
}
