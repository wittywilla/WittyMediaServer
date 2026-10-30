package wittyphotos

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// tiffBuilder writes a small little-endian TIFF with the tags the tests need.
type tiffBuilder struct{ b bytes.Buffer }

type rawTag struct {
	id, typ uint16
	count   uint32
	data    []byte // written after the directory when longer than 4 bytes
}

// dir writes a directory at the buffer's end and returns its offset. extra is
// where out-of-line values start.
func (t *tiffBuilder) dir(tags []rawTag) uint32 {
	at := uint32(t.b.Len())
	data := at + 2 + uint32(len(tags))*12 + 4
	le := binary.LittleEndian
	binary.Write(&t.b, le, uint16(len(tags)))
	var extra bytes.Buffer
	for _, x := range tags {
		binary.Write(&t.b, le, x.id)
		binary.Write(&t.b, le, x.typ)
		binary.Write(&t.b, le, x.count)
		if len(x.data) <= 4 {
			v := make([]byte, 4)
			copy(v, x.data)
			t.b.Write(v)
		} else {
			binary.Write(&t.b, le, data+uint32(extra.Len()))
			extra.Write(x.data)
		}
	}
	binary.Write(&t.b, le, uint32(0))
	t.b.Write(extra.Bytes())
	return at
}

func u32le(v uint32) []byte { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); return b }
func u16le(v uint16) []byte { b := make([]byte, 2); binary.LittleEndian.PutUint16(b, v); return b }
func rat(pairs ...uint32) []byte {
	var b []byte
	for _, p := range pairs {
		b = append(b, u32le(p)...)
	}
	return b
}
func ascii(s string) []byte { return append([]byte(s), 0) }

// sampleTIFF: a first directory pointing at EXIF and GPS directories.
func sampleTIFF() []byte {
	t := &tiffBuilder{}
	t.b.Write([]byte{'I', 'I', 42, 0, 8, 0, 0, 0})
	// The sizes of the first directory are known, so the others can follow it.
	mk, md := ascii("TestCam"), ascii("Model X")
	first := []rawTag{{tagMake, 2, uint32(len(mk)), mk}, {tagModel, 2, uint32(len(md)), md}, {tagOrientation, 3, 1, u16le(6)}, {tagExifIFD, 4, 1, u32le(0)}, {tagGPSIFD, 4, 1, u32le(0)}}
	firstAt := t.b.Len()
	t.dir(first)
	exifAt := uint32(t.b.Len())
	date, off, lens := ascii("2023:04:12 15:30:45"), ascii("+02:00"), ascii("50mm f/1.8")
	t.dir([]rawTag{
		{tagExposure, 5, 1, rat(1, 250)}, {tagFNumber, 5, 1, rat(18, 10)}, {tagISO, 3, 1, u16le(400)},
		{tagDateOriginal, 2, uint32(len(date)), date}, {tagOffsetOrig, 2, uint32(len(off)), off},
		{tagFocal, 5, 1, rat(50, 1)}, {tagPixelX, 4, 1, u32le(6000)}, {tagPixelY, 4, 1, u32le(4000)},
		{tagLens, 2, uint32(len(lens)), lens},
	})
	gpsAt := uint32(t.b.Len())
	t.dir([]rawTag{
		{1, 2, 2, []byte("S\x00")}, {2, 5, 3, rat(33, 1, 51, 1, 0, 1)},
		{3, 2, 2, []byte("E\x00")}, {4, 5, 3, rat(151, 1, 12, 1, 36, 1)},
		{5, 1, 1, []byte{0}}, {6, 5, 1, rat(58, 1)},
	})
	b := t.b.Bytes()
	// Point the first directory at the others (entries 3 and 4).
	le := binary.LittleEndian
	le.PutUint32(b[firstAt+2+3*12+8:], exifAt)
	le.PutUint32(b[firstAt+2+4*12+8:], gpsAt)
	return b
}

func TestParseEXIF(t *testing.T) {
	in, err := Parse(sampleTIFF())
	if err != nil {
		t.Fatal(err)
	}
	if in.Make != "TestCam" || in.Model != "Model X" || in.Lens != "50mm f/1.8" {
		t.Errorf("camera: %+v", in)
	}
	if in.Orientation != 6 || in.Width != 6000 || in.Height != 4000 {
		t.Errorf("shape: %+v", in)
	}
	if in.Exposure != "1/250" || in.FNumber != 1.8 || in.ISO != 400 || in.FocalMM != 50 {
		t.Errorf("exposure: %+v", in)
	}
	want := time.Date(2023, 4, 12, 13, 30, 45, 0, time.UTC)
	if !in.HasOffset || !in.Taken.Equal(want) {
		t.Errorf("taken %v, want %v", in.Taken, want)
	}
	if !in.HasGPS || math.Abs(in.Lat+33.85) > 1e-6 || math.Abs(in.Lon-151.21) > 1e-6 || in.Alt != 58 {
		t.Errorf("gps: %+v", in)
	}
}

func TestContainers(t *testing.T) {
	tiff := sampleTIFF()
	exif := append([]byte("Exif\x00\x00"), tiff...)
	// JPEG: APP1 right after SOI.
	var j bytes.Buffer
	j.Write([]byte{0xFF, 0xD8, 0xFF, 0xE1})
	binary.Write(&j, binary.BigEndian, uint16(len(exif)+2))
	j.Write(exif)
	j.Write([]byte{0xFF, 0xD9})
	// PNG: an eXIf chunk before the picture data.
	var p bytes.Buffer
	p.WriteString("\x89PNG\r\n\x1a\n")
	binary.Write(&p, binary.BigEndian, uint32(len(tiff)))
	p.WriteString("eXIf")
	p.Write(tiff)
	p.Write(make([]byte, 4))
	// WebP: an EXIF chunk.
	var w bytes.Buffer
	w.WriteString("RIFF\x00\x00\x00\x00WEBPEXIF")
	binary.Write(&w, binary.LittleEndian, uint32(len(tiff)))
	w.Write(tiff)
	// HEIC: a box file with the marker somewhere inside.
	var h bytes.Buffer
	h.Write([]byte{0, 0, 0, 24})
	h.WriteString("ftypheic")
	h.Write(make([]byte, 12))
	h.Write(make([]byte, 40))
	h.Write(exif)
	for name, b := range map[string][]byte{"jpeg": j.Bytes(), "png": p.Bytes(), "webp": w.Bytes(), "heic": h.Bytes()} {
		in, err := Parse(b)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if in.Make != "TestCam" || !in.HasGPS {
			t.Errorf("%s: %+v", name, in)
		}
	}
}

// Files that lie: no panic, and no result worth believing.
func TestMalformed(t *testing.T) {
	cases := [][]byte{nil, {0xFF}, {0xFF, 0xD8, 0xFF, 0xE1, 0xFF, 0xFF}, []byte("II*\x00\xff\xff\xff\xff"), []byte("MM\x00*\x00\x00\x00\x08\xff\xff"),
		append([]byte("II*\x00\x08\x00\x00\x00\xff\xff"), make([]byte, 40)...)}
	for _, c := range cases {
		_, _ = Parse(c)
		_, _ = previewRange(c)
	}
	// A value that claims to be gigantic stays out of reach.
	b := sampleTIFF()
	binary.LittleEndian.PutUint32(b[8+2+4:], 0xFFFFFFFF) // Make's count
	in, err := Parse(b)
	if err == nil && in.Make != "" {
		t.Errorf("oversized value was read: %q", in.Make)
	}
}

func smallJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestRawPreview(t *testing.T) {
	small, big := smallJPEG(t, 16, 16), smallJPEG(t, 160, 120)
	// A TIFF "RAW": a thumbnail in the first directory, a larger preview in a sub-directory.
	tb := &tiffBuilder{}
	tb.b.Write([]byte{'I', 'I', 42, 0, 8, 0, 0, 0})
	firstAt := tb.b.Len()
	tb.dir([]rawTag{{0x0201, 4, 1, u32le(0)}, {0x0202, 4, 1, u32le(uint32(len(small)))}, {tagSubIFDs, 4, 1, u32le(0)}})
	subAt := uint32(tb.b.Len())
	tb.dir([]rawTag{{0x0103, 3, 1, u16le(6)}, {0x0111, 4, 1, u32le(0)}, {0x0117, 4, 1, u32le(uint32(len(big)))}})
	smallAt := uint32(tb.b.Len())
	tb.b.Write(small)
	bigAt := uint32(tb.b.Len())
	tb.b.Write(big)
	b := tb.b.Bytes()
	le := binary.LittleEndian
	le.PutUint32(b[firstAt+2+0*12+8:], smallAt)
	le.PutUint32(b[firstAt+2+2*12+8:], subAt)
	le.PutUint32(b[int(subAt)+2+1*12+8:], bigAt)
	p := filepath.Join(t.TempDir(), "a.nef")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := RawPreview(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, big) {
		t.Errorf("got %d bytes, want the %d byte preview", len(got), len(big))
	}
	// RAF: offset and length in the header.
	raf := make([]byte, 148)
	copy(raf, "FUJIFILMCCD-RAW 0201")
	binary.BigEndian.PutUint32(raf[84:], 148)
	binary.BigEndian.PutUint32(raf[88:], uint32(len(big)))
	raf = append(raf, big...)
	p2 := filepath.Join(t.TempDir(), "a.raf")
	if err := os.WriteFile(p2, raf, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := RawPreview(p2); err != nil || !bytes.Equal(got, big) {
		t.Errorf("raf: %d bytes, %v", len(got), err)
	}
	// A box format: found by scanning.
	box := append(append([]byte{0, 0, 0, 24}, "ftypcrx "...), make([]byte, 64)...)
	box = append(append(box, small...), big...)
	p3 := filepath.Join(t.TempDir(), "a.cr3")
	if err := os.WriteFile(p3, box, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := RawPreview(p3); err != nil || !bytes.Equal(got, big) {
		t.Errorf("cr3: %d bytes, %v", len(got), err)
	}
	// No preview at all.
	p4 := filepath.Join(t.TempDir(), "b.nef")
	_ = os.WriteFile(p4, []byte("II*\x00\x08\x00\x00\x00\x00\x00\x00\x00\x00\x00"), 0o644)
	if _, err := RawPreview(p4); err == nil {
		t.Error("expected no preview")
	}
}

func TestDimensions(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if w, h := Dimensions(write("a.jpg", smallJPEG(t, 40, 30))); w != 40 || h != 30 {
		t.Errorf("jpeg %dx%d", w, h)
	}
	webp := make([]byte, 32)
	copy(webp, "RIFF\x00\x00\x00\x00WEBPVP8X")
	webp[24], webp[27] = 99, 49 // canvas size minus one
	if w, h := Dimensions(write("a.webp", webp)); w != 100 || h != 50 {
		t.Errorf("webp %dx%d", w, h)
	}
	bmp := make([]byte, 54)
	copy(bmp, "BM")
	binary.LittleEndian.PutUint32(bmp[14:], 40)
	binary.LittleEndian.PutUint32(bmp[18:], 64)
	binary.LittleEndian.PutUint32(bmp[22:], uint32(0xFFFFFFE0)) // -32: top-down
	if w, h := Dimensions(write("a.bmp", bmp)); w != 64 || h != 32 {
		t.Errorf("bmp %dx%d", w, h)
	}
	if w, _ := Dimensions(write("a.txt", []byte("not a picture"))); w != 0 {
		t.Errorf("text has a width: %d", w)
	}
}

func FuzzParse(f *testing.F) {
	f.Add(sampleTIFF())
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x10, 'E', 'x', 'i', 'f', 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = Parse(b)
		_, _ = previewRange(b)
	})
}

// The manifest tells the server which files are RAW without starting the helper,
// so it must list exactly what IsRaw knows.
func TestManifestListsRawExtensions(t *testing.T) {
	var m struct {
		Capabilities struct {
			RawExtensions []string `json:"rawExtensions"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(ManifestJSON, &m); err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, e := range m.Capabilities.RawExtensions {
		listed[e] = true
		if !rawExts[e] {
			t.Errorf("the manifest lists %s, which IsRaw doesn't know", e)
		}
	}
	for e := range rawExts {
		if !listed[e] {
			t.Errorf("IsRaw knows %s, which the manifest doesn't list", e)
		}
	}
}
