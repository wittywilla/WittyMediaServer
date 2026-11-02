package server

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Small samples made when the test runs, never kept. Each is a second of a test pattern
// and a tone, or a page of made-up words: nothing here is real media.

// sampleTools is what's on this machine for making samples.
type sampleTools struct {
	t      *testing.T
	ffmpeg string
	dir    string
	office *officeProfile
}

// officeProfile: LibreOffice builds a profile the first time, which takes seconds, and refuses to
// share one between copies running at once.
type officeProfile struct {
	mu  sync.Mutex
	dir string
}

var errNoTool = fmt.Errorf("no tool here to make this sample")

func (st *sampleTools) tool(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%w (%s)", errNoTool, name)
	}
	return p, nil
}

func (st *sampleTools) run(name string, args ...string) error {
	p, err := st.tool(name)
	if err != nil {
		return err
	}
	cmd := exec.Command(p, args...)
	cmd.Dir = st.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		tail := string(out)
		if len(tail) > 400 {
			tail = tail[len(tail)-400:]
		}
		return fmt.Errorf("%s: %w: %s", name, err, tail)
	}
	return nil
}

// ff runs FFmpeg with a test pattern and a tone as inputs 0 and 1.
func (st *sampleTools) ff(out string, args ...string) error {
	base := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=1", "-f", "lavfi", "-i", "sine=frequency=440:duration=1:sample_rate=48000"}
	return st.run("ffmpeg", append(append(base, args...), out)...)
}

func (st *sampleTools) ffvideo(out string, args ...string) error {
	base := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=1"}
	return st.run("ffmpeg", append(append(base, args...), out)...)
}

func (st *sampleTools) ffaudio(out string, args ...string) error {
	base := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=1:sample_rate=48000"}
	return st.run("ffmpeg", append(append(base, args...), out)...)
}

// ffimage writes one frame of the test pattern.
func (st *sampleTools) ffimage(out string, args ...string) error {
	base := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=1:duration=1", "-frames:v", "1"}
	return st.run("ffmpeg", append(append(base, args...), out)...)
}

func smallJPEGBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 5), 120, 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// rawTIFF makes a TIFF-based RAW file: a header, and a directory pointing at a JPEG.
func rawTIFF(t *testing.T, magic []byte, extra []byte) []byte {
	t.Helper()
	j := smallJPEGBytes(t)
	var b bytes.Buffer
	b.Write(magic)
	binary.Write(&b, binary.LittleEndian, uint32(8+len(extra)))
	b.Write(extra)
	tags := [][3]uint32{{0x0103, 3, 6}, {0x0111, 4, 0}, {0x0117, 4, uint32(len(j))}, {0x0201, 4, 0}, {0x0202, 4, uint32(len(j))}}
	dataAt := uint32(8+len(extra)) + 2 + uint32(len(tags))*12 + 4
	tags[1][2], tags[3][2] = dataAt, dataAt
	binary.Write(&b, binary.LittleEndian, uint16(len(tags)))
	for _, e := range tags {
		binary.Write(&b, binary.LittleEndian, uint16(e[0]))
		binary.Write(&b, binary.LittleEndian, uint16(e[1]))
		binary.Write(&b, binary.LittleEndian, uint32(1))
		binary.Write(&b, binary.LittleEndian, e[2])
	}
	binary.Write(&b, binary.LittleEndian, uint32(0))
	b.Write(j)
	return b.Bytes()
}

// rawRAF makes the Fujifilm layout: offsets at a fixed place in the header.
func rawRAF(t *testing.T) []byte {
	t.Helper()
	j := smallJPEGBytes(t)
	head := make([]byte, 148)
	copy(head, "FUJIFILMCCD-RAW 0201FF129502")
	binary.BigEndian.PutUint32(head[84:], 148)
	binary.BigEndian.PutUint32(head[88:], uint32(len(j)))
	return append(head, j...)
}

// rawCR3 makes the Canon layout: MP4 boxes, with the JPEG somewhere inside.
func rawCR3(t *testing.T) []byte {
	t.Helper()
	j := smallJPEGBytes(t)
	var b bytes.Buffer
	b.Write([]byte{0, 0, 0, 24})
	b.WriteString("ftypcrx \x00\x00\x00\x01crx isom")
	binary.Write(&b, binary.BigEndian, uint32(8+len(j)))
	b.WriteString("mdat")
	b.Write(j)
	return b.Bytes()
}

func zipFiles(t *testing.T, files map[string][]byte, order ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, n := range order {
		f, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		f.Write(files[n])
	}
	zw.Close()
	return b.Bytes()
}

// pageImages makes a few page pictures in a folder of their own.
func (st *sampleTools) pageImages(dir string) ([]string, error) {
	if err := os.MkdirAll(filepath.Join(st.dir, dir), 0o755); err != nil {
		return nil, err
	}
	names := []string{"page1.jpg", "page2.jpg", "page10.jpg"}
	j := smallJPEGBytes(st.t)
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(st.dir, dir, n), j, 0o644); err != nil {
			return nil, err
		}
	}
	return names, nil
}

func (st *sampleTools) cbz(out string) error {
	j := smallJPEGBytes(st.t)
	b := zipFiles(st.t, map[string][]byte{"page1.jpg": j, "page2.jpg": j, "page10.jpg": j}, "page1.jpg", "page2.jpg", "page10.jpg")
	return os.WriteFile(out, b, 0o644)
}

func (st *sampleTools) cbr(out string) error {
	names, err := st.pageImages("rarpages")
	if err != nil {
		return err
	}
	abs, _ := filepath.Abs(out)
	cmd := exec.Command("rar", append([]string{"a", "-idq", "-ep", abs + ".rar"}, names...)...)
	cmd.Dir = filepath.Join(st.dir, "rarpages")
	if _, err := exec.LookPath("rar"); err != nil {
		return errNoTool
	}
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rar: %w: %s", err, b)
	}
	return os.Rename(abs+".rar", abs)
}

func (st *sampleTools) cb7(out string) error {
	names, err := st.pageImages("7zpages")
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("7z"); err != nil {
		return errNoTool
	}
	abs, _ := filepath.Abs(out)
	cmd := exec.Command("7z", append([]string{"a", "-bd", "-t7z", abs + ".7z"}, names...)...)
	cmd.Dir = filepath.Join(st.dir, "7zpages")
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("7z: %w: %s", err, b)
	}
	return os.Rename(abs+".7z", abs)
}

func (st *sampleTools) cbt(out string) error {
	j := smallJPEGBytes(st.t)
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	tw := tar.NewWriter(f)
	for _, n := range []string{"page1.jpg", "page2.jpg", "page10.jpg"} {
		if err := tw.WriteHeader(&tar.Header{Name: n, Mode: 0o644, Size: int64(len(j))}); err != nil {
			return err
		}
		tw.Write(j)
	}
	return tw.Close()
}

const sampleBook = `Made-Up Tale

Chapter 1

The lamp on the quay burned low, and Tessa counted the boats again. Nothing had changed since morning.

The harbour master looked up from his ledger and said nothing at all.

Chapter 2

By noon the wind had turned, and the boats began to come home, one after another, low in the water.
`

// bookSource is the same small book as Markdown, which pandoc and Calibre can read.
func bookSource() string {
	return "---\ntitle: Made-Up Tale\nauthor: A. Writer\n---\n\n# Chapter 1\n\nThe lamp on the quay burned low, and Tessa counted the boats again.\n\nThe harbour master looked up and said **nothing** at *all*.\n\n# Chapter 2\n\nBy noon the wind had turned, and the boats began to come home.\n"
}

func (st *sampleTools) pandoc(out string, args ...string) error {
	src := filepath.Join(st.dir, "book.md")
	if err := os.WriteFile(src, []byte(bookSource()), 0o644); err != nil {
		return err
	}
	return st.run("pandoc", append([]string{"-s", "-o", out, src}, args...)...)
}

func (st *sampleTools) calibre(out string, args ...string) error {
	src := filepath.Join(st.dir, "book-for-calibre.epub")
	if _, err := os.Stat(src); err != nil {
		if err := st.pandoc(src); err != nil {
			return err
		}
	}
	p, err := st.tool("ebook-convert")
	if err != nil {
		return err
	}
	cmd := exec.Command(p, append([]string{src, out}, args...)...)
	cmd.Env = append(os.Environ(), "HOME="+st.dir, "QT_QPA_PLATFORM=offscreen", "CALIBRE_CONFIG_DIRECTORY="+filepath.Join(st.dir, "calibre"))
	if b, err := cmd.CombinedOutput(); err != nil {
		if len(b) > 300 {
			b = b[len(b)-300:]
		}
		return fmt.Errorf("ebook-convert: %w: %s", err, b)
	}
	return nil
}

func (st *sampleTools) soffice(out, format string) error {
	src := filepath.Join(st.dir, "book-for-office.docx")
	if err := st.pandoc(src); err != nil {
		return err
	}
	outdir := filepath.Join(st.dir, "office-out")
	p, err := st.tool("soffice")
	if err != nil {
		return err
	}
	st.office.mu.Lock()
	defer st.office.mu.Unlock()
	cmd := exec.Command(p, "--headless", "-env:UserInstallation=file://"+st.office.dir, "--convert-to", format, "--outdir", outdir, src)
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("soffice: %w: %s", err, b)
	}
	return os.Rename(filepath.Join(outdir, "book-for-office."+format), out)
}

func (st *sampleTools) djvu(out string) error {
	pnm := filepath.Join(st.dir, "page.pgm")
	if err := st.ffimage(pnm, "-pix_fmt", "gray"); err != nil {
		return err
	}
	return st.run("c44", pnm, out)
}

// palmDocBytes is a Palm eBook: a database with a header record and plain text records.
func palmDocBytes() []byte {
	body := []byte(strings.Repeat(sampleBook, 4))
	hdr := make([]byte, 16)
	binary.BigEndian.PutUint16(hdr[0:], 1) // no compression
	binary.BigEndian.PutUint32(hdr[4:], uint32(len(body)))
	binary.BigEndian.PutUint16(hdr[8:], 1)
	binary.BigEndian.PutUint16(hdr[10:], 4096)
	pdb := make([]byte, 78)
	copy(pdb, "Made Up Tale")
	copy(pdb[60:], "TEXtREAd")
	binary.BigEndian.PutUint16(pdb[76:], 2)
	dir := make([]byte, 16+2)
	at := uint32(78 + len(dir))
	binary.BigEndian.PutUint32(dir[0:], at)
	binary.BigEndian.PutUint32(dir[8:], at+uint32(len(hdr)))
	return append(append(append(pdb, dir...), hdr...), body...)
}

// ---------------------------------------------------------------- MIDI

func midiBytes() []byte {
	var b bytes.Buffer
	b.WriteString("MThd")
	binary.Write(&b, binary.BigEndian, uint32(6))
	binary.Write(&b, binary.BigEndian, []uint16{0, 1, 96})
	var tr bytes.Buffer
	tr.Write([]byte{0, 0xFF, 0x03, 9})
	tr.WriteString("Test Song")
	tr.Write([]byte{0, 0xC0, 0}) // a piano
	for _, n := range []byte{60, 64, 67, 72} {
		tr.Write([]byte{0, 0x90, n, 100})
		tr.Write([]byte{48, 0x80, n, 0})
	}
	tr.Write([]byte{0, 0x99, 38, 100, 24, 0x89, 38, 0})
	tr.Write([]byte{0, 0xFF, 0x2F, 0})
	b.WriteString("MTrk")
	binary.Write(&b, binary.BigEndian, uint32(tr.Len()))
	b.Write(tr.Bytes())
	return b.Bytes()
}

// ---------------------------------------------------------------- Tracker and game music

// modBytes is a four-channel ProTracker song: one sample, one pattern of a few notes.
func modBytes() []byte {
	b := make([]byte, 1084)
	copy(b, "made up song")
	sample := make([]byte, 2000)
	for i := range sample {
		if i%40 < 20 {
			sample[i] = 40
		} else {
			sample[i] = 0xD8
		}
	}
	// The first sample's description: name, length in words, volume, loop.
	copy(b[20:], "tone")
	binary.BigEndian.PutUint16(b[42:], uint16(len(sample)/2))
	b[45] = 64
	binary.BigEndian.PutUint16(b[46:], 0)
	binary.BigEndian.PutUint16(b[48:], 1)
	b[950] = 1 // the song is one pattern long
	b[951] = 127
	copy(b[1080:], "M.K.")
	pattern := make([]byte, 64*4*4)
	for row := 0; row < 64; row += 4 {
		// Sample 1, a C at period 428.
		pattern[row*16] = 0x01
		pattern[row*16+1] = 0xAC
		pattern[row*16+2] = 0x10
	}
	return append(append(b, pattern...), sample...)
}

// xmBytes is the smallest FastTracker 2 song libopenmpt takes.
func xmBytes() []byte {
	var b bytes.Buffer
	b.WriteString("Extended Module: ")
	name := make([]byte, 20)
	copy(name, "made up song")
	b.Write(name)
	b.WriteByte(0x1A)
	tracker := make([]byte, 20)
	copy(tracker, "FastTracker v2.00")
	b.Write(tracker)
	binary.Write(&b, binary.LittleEndian, uint16(0x0104))
	binary.Write(&b, binary.LittleEndian, uint32(276)) // header size
	binary.Write(&b, binary.LittleEndian, uint16(1))   // song length
	binary.Write(&b, binary.LittleEndian, uint16(0))   // restart
	binary.Write(&b, binary.LittleEndian, uint16(2))   // channels
	binary.Write(&b, binary.LittleEndian, uint16(1))   // patterns
	binary.Write(&b, binary.LittleEndian, uint16(1))   // instruments
	binary.Write(&b, binary.LittleEndian, uint16(1))   // flags
	binary.Write(&b, binary.LittleEndian, uint16(6))   // tempo
	binary.Write(&b, binary.LittleEndian, uint16(125)) // BPM
	order := make([]byte, 256)
	b.Write(order)
	// One empty pattern of 16 rows: each cell is the "nothing" byte.
	rows := make([]byte, 16*2)
	for i := range rows {
		rows[i] = 0x80
	}
	binary.Write(&b, binary.LittleEndian, uint32(9))
	b.WriteByte(0)
	binary.Write(&b, binary.LittleEndian, uint16(16))
	binary.Write(&b, binary.LittleEndian, uint16(len(rows)))
	b.Write(rows)
	// One instrument with no samples.
	inst := make([]byte, 29)
	binary.LittleEndian.PutUint32(inst, 29)
	b.Write(inst)
	return b.Bytes()
}

// s3mBytes is a Scream Tracker song with no samples and one empty pattern.
func s3mBytes() []byte {
	h := make([]byte, 96)
	copy(h, "made up song")
	h[28] = 0x1A
	h[29] = 16
	binary.LittleEndian.PutUint16(h[32:], 1) // orders
	binary.LittleEndian.PutUint16(h[34:], 0) // instruments
	binary.LittleEndian.PutUint16(h[36:], 1) // patterns
	binary.LittleEndian.PutUint16(h[40:], 0x1320)
	binary.LittleEndian.PutUint16(h[42:], 2)
	copy(h[44:], "SCRM")
	h[48] = 64
	h[49] = 6
	h[50] = 125
	h[51] = 0xB0
	for i := 64; i < 96; i++ {
		h[i] = 0xFF
	}
	h[64], h[65] = 0, 8
	var b bytes.Buffer
	b.Write(h)
	b.WriteByte(0)                                   // order 0: pattern 0
	binary.Write(&b, binary.LittleEndian, uint16(7)) // paragraph where the pattern is
	for b.Len()%16 != 0 {
		b.WriteByte(0)
	}
	for b.Len() < 7*16 {
		b.WriteByte(0)
	}
	pattern := append([]byte{66, 0}, make([]byte, 64)...) // 64 rows ending at once
	b.Write(pattern)
	return b.Bytes()
}

// itBytes is an Impulse Tracker song with one empty pattern.
func itBytes() []byte {
	h := make([]byte, 192)
	copy(h, "IMPM")
	copy(h[4:], "made up song")
	binary.LittleEndian.PutUint16(h[32:], 1) // orders
	binary.LittleEndian.PutUint16(h[38:], 1) // patterns
	binary.LittleEndian.PutUint16(h[40:], 0x0214)
	binary.LittleEndian.PutUint16(h[42:], 0x0200)
	h[48] = 128
	h[49] = 48
	h[50] = 6
	h[51] = 125
	h[52] = 128
	for i := 64; i < 128; i++ {
		h[i] = 32 // channel pan
	}
	for i := 128; i < 192; i++ {
		h[i] = 64 // channel volume
	}
	var b bytes.Buffer
	b.Write(h)
	b.WriteByte(0) // order 0
	patternAt := uint32(b.Len() + 4)
	binary.Write(&b, binary.LittleEndian, patternAt)
	binary.Write(&b, binary.LittleEndian, uint16(1)) // data length
	binary.Write(&b, binary.LittleEndian, uint16(32))
	b.Write(make([]byte, 4))
	b.WriteByte(0)
	return b.Bytes()
}

// spcBytes is a Super Nintendo sound file: the header, and silent memory.
func spcBytes() []byte {
	b := make([]byte, 0x10200)
	copy(b, "SNES-SPC700 Sound File Data v0.30\x1A\x1A\x1A\x1E")
	copy(b[0x2E:], "Made Up Song")
	b[0xA9] = '1' // seconds to play
	b[0xAC] = '0'
	return b
}

// vgmBytes is a Video Game Music file: a few notes for the PSG chip.
func vgmBytes() []byte {
	var cmds bytes.Buffer
	for i := 0; i < 8; i++ {
		cmds.Write([]byte{0x50, 0x8E, 0x50, 0x0F, 0x50, 0x90}) // a tone on the first channel
		cmds.Write([]byte{0x61, 0x00, 0x56})                   // about half a second
		cmds.Write([]byte{0x50, 0x9F})                         // quiet
		cmds.Write([]byte{0x61, 0x00, 0x56})
	}
	cmds.WriteByte(0x66)
	h := make([]byte, 0x40)
	copy(h, "Vgm ")
	binary.LittleEndian.PutUint32(h[4:], uint32(0x40+cmds.Len()-4))
	binary.LittleEndian.PutUint32(h[8:], 0x150)
	binary.LittleEndian.PutUint32(h[0x0C:], 3579545) // the PSG's clock
	samples := uint32(8 * 2 * 0x5600)
	binary.LittleEndian.PutUint32(h[0x18:], samples)
	binary.LittleEndian.PutUint32(h[0x34:], 0x0C)
	return append(h, cmds.Bytes()...)
}

// nsfBytes is a Nintendo sound file whose code just returns.
func nsfBytes() []byte {
	h := make([]byte, 128)
	copy(h, "NESM\x1A")
	h[5] = 1
	h[6] = 1
	h[7] = 1
	binary.LittleEndian.PutUint16(h[8:], 0x8000)
	binary.LittleEndian.PutUint16(h[10:], 0x8000)
	binary.LittleEndian.PutUint16(h[12:], 0x8000)
	copy(h[14:], "Made Up Song")
	binary.LittleEndian.PutUint16(h[110:], 16666)
	code := make([]byte, 4096)
	code[0] = 0x60 // RTS
	return append(h, code...)
}

// ---------------------------------------------------------------- DSD

// dsdTone turns a tone into one-bit samples, the way a simple modulator would.
func dsdTone(bytesPerChannel int) []byte {
	out := make([]byte, bytesPerChannel)
	acc := 0.0
	for i := 0; i < bytesPerChannel*8; i++ {
		phase := float64(i) * 440.0 / 2822400.0
		s := 0.4 * sine(phase)
		if acc < s {
			acc += 1 - s
			out[i/8] |= 1 << (i % 8)
		} else {
			acc -= 1 + s
		}
	}
	return out
}

func sine(turns float64) float64 {
	turns -= float64(int(turns))
	// A triangle does: this only has to be sound, not good sound.
	if turns < .5 {
		return 4*turns - 1
	}
	return 3 - 4*turns
}

func dsfBytes() []byte {
	const block = 4096
	ch := dsdTone(block * 4)
	var data bytes.Buffer
	for i := 0; i+block <= len(ch); i += block {
		data.Write(ch[i : i+block])
		data.Write(ch[i : i+block])
	}
	var b bytes.Buffer
	b.WriteString("DSD ")
	binary.Write(&b, binary.LittleEndian, uint64(28))
	binary.Write(&b, binary.LittleEndian, uint64(28+52+12+data.Len()))
	binary.Write(&b, binary.LittleEndian, uint64(0))
	b.WriteString("fmt ")
	binary.Write(&b, binary.LittleEndian, uint64(52))
	binary.Write(&b, binary.LittleEndian, uint32(1))       // version
	binary.Write(&b, binary.LittleEndian, uint32(0))       // format: raw
	binary.Write(&b, binary.LittleEndian, uint32(2))       // channel type: stereo
	binary.Write(&b, binary.LittleEndian, uint32(2))       // channels
	binary.Write(&b, binary.LittleEndian, uint32(2822400)) // sample rate
	binary.Write(&b, binary.LittleEndian, uint32(1))       // bits per sample
	binary.Write(&b, binary.LittleEndian, uint64(len(ch)*8))
	binary.Write(&b, binary.LittleEndian, uint32(block))
	binary.Write(&b, binary.LittleEndian, uint32(0))
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint64(12+data.Len()))
	b.Write(data.Bytes())
	return b.Bytes()
}

func dffBytes() []byte {
	ch := dsdTone(4096 * 4)
	data := make([]byte, 0, len(ch)*2)
	for _, c := range ch {
		data = append(data, c, c)
	}
	chunk := func(id string, body []byte) []byte {
		var b bytes.Buffer
		b.WriteString(id)
		binary.Write(&b, binary.BigEndian, uint64(len(body)))
		b.Write(body)
		if len(body)%2 == 1 {
			b.WriteByte(0)
		}
		return b.Bytes()
	}
	var prop bytes.Buffer
	prop.WriteString("SND ")
	fs := make([]byte, 4)
	binary.BigEndian.PutUint32(fs, 2822400)
	prop.Write(chunk("FS  ", fs))
	prop.Write(chunk("CHNL", append([]byte{0, 2}, "SLFTSRGT"...)))
	prop.Write(chunk("CMPR", append([]byte("DSD "), make([]byte, 16)...)))
	var form bytes.Buffer
	form.WriteString("DSD ")
	form.Write(chunk("FVER", []byte{1, 5, 0, 0}))
	form.Write(chunk("PROP", prop.Bytes()))
	form.Write(chunk("DSD ", data))
	return chunk("FRM8", form.Bytes())
}

// ---------------------------------------------------------------- Picture subtitles

// supBytes is a Blu-ray subtitle: one small picture on screen from second 0 to 1.
func supBytes() []byte {
	segment := func(pts uint32, kind byte, body []byte) []byte {
		var b bytes.Buffer
		b.WriteString("PG")
		binary.Write(&b, binary.BigEndian, pts)
		binary.Write(&b, binary.BigEndian, uint32(0))
		b.WriteByte(kind)
		binary.Write(&b, binary.BigEndian, uint16(len(body)))
		b.Write(body)
		return b.Bytes()
	}
	const w, h = 32, 8
	var rle bytes.Buffer
	for y := 0; y < h; y++ {
		rle.Write([]byte{0x00, 0x80 | w, 1}) // w pixels of colour 1
		rle.Write([]byte{0x00, 0x00})        // end of line
	}
	pcs := func(state byte, objects int) []byte {
		var p bytes.Buffer
		binary.Write(&p, binary.BigEndian, []uint16{320, 240})
		p.WriteByte(0x10)
		binary.Write(&p, binary.BigEndian, uint16(0))
		p.WriteByte(state)
		p.WriteByte(0)
		p.WriteByte(0)
		p.WriteByte(byte(objects))
		if objects > 0 {
			binary.Write(&p, binary.BigEndian, uint16(0))
			p.WriteByte(0)
			p.WriteByte(0)
			binary.Write(&p, binary.BigEndian, []uint16{10, 10})
		}
		return p.Bytes()
	}
	var wds bytes.Buffer
	wds.WriteByte(1)
	wds.WriteByte(0)
	binary.Write(&wds, binary.BigEndian, []uint16{10, 10, w, h})
	pds := []byte{0, 0, 1, 235, 128, 128, 255}
	var ods bytes.Buffer
	binary.Write(&ods, binary.BigEndian, uint16(0))
	ods.WriteByte(0)
	ods.WriteByte(0xC0)
	size := rle.Len() + 4
	ods.Write([]byte{byte(size >> 16), byte(size >> 8), byte(size)})
	binary.Write(&ods, binary.BigEndian, []uint16{w, h})
	ods.Write(rle.Bytes())
	var out bytes.Buffer
	out.Write(segment(0, 0x16, pcs(0x80, 1)))
	out.Write(segment(0, 0x17, wds.Bytes()))
	out.Write(segment(0, 0x14, pds))
	out.Write(segment(0, 0x15, ods.Bytes()))
	out.Write(segment(0, 0x80, nil))
	out.Write(segment(90000, 0x16, pcs(0x00, 0)))
	out.Write(segment(90000, 0x17, wds.Bytes()))
	out.Write(segment(90000, 0x80, nil))
	return out.Bytes()
}

// vobSub writes an .idx and a .sub: one small DVD subtitle picture.
func vobSub(idx, sub string) error {
	// A subtitle unit: size, then the picture's two fields, then its commands.
	rle := []byte{0x0F, 0x01, 0x00, 0x00} // pixels of colour 1, then the line's end
	var fields bytes.Buffer
	for i := 0; i < 4; i++ {
		fields.Write(rle)
	}
	data := fields.Bytes()
	ctrlAt := 4 + len(data)
	var ctrl bytes.Buffer
	binary.Write(&ctrl, binary.BigEndian, uint16(0)) // when
	binary.Write(&ctrl, binary.BigEndian, uint16(ctrlAt))
	ctrl.WriteByte(0x03) // colours
	ctrl.Write([]byte{0x01, 0x23})
	ctrl.WriteByte(0x04) // contrast
	ctrl.Write([]byte{0xFF, 0xFF})
	ctrl.WriteByte(0x05) // where
	ctrl.Write([]byte{0x00, 0x00, 0x7F, 0x00, 0x00, 0x07})
	ctrl.WriteByte(0x06) // the fields
	binary.Write(&ctrl, binary.BigEndian, []uint16{4, 4})
	ctrl.WriteByte(0x01) // show
	ctrl.WriteByte(0xFF)
	var spu bytes.Buffer
	binary.Write(&spu, binary.BigEndian, uint16(ctrlAt+ctrl.Len()))
	binary.Write(&spu, binary.BigEndian, uint16(ctrlAt))
	spu.Write(data)
	spu.Write(ctrl.Bytes())
	// In a pack: a private stream, with a time.
	var pes bytes.Buffer
	pes.Write([]byte{0x00, 0x00, 0x01, 0xBD})
	binary.Write(&pes, binary.BigEndian, uint16(3+5+1+spu.Len()))
	pes.Write([]byte{0x81, 0x80, 0x05})
	pes.Write([]byte{0x21, 0x00, 0x01, 0x00, 0x01}) // time zero
	pes.WriteByte(0x20)                             // subtitle stream 0
	pes.Write(spu.Bytes())
	pack := []byte{0x00, 0x00, 0x01, 0xBA, 0x44, 0x00, 0x04, 0x00, 0x04, 0x01, 0x00, 0x00, 0x03, 0xF8}
	var out bytes.Buffer
	out.Write(pack)
	out.Write(pes.Bytes())
	for out.Len()%2048 != 0 {
		out.WriteByte(0xFF)
	}
	if err := os.WriteFile(sub, out.Bytes(), 0o644); err != nil {
		return err
	}
	text := "# VobSub index file, v7 (do not modify this line!)\nsize: 320x240\npalette: 000000, ffffff, 808080, 404040, 000000, 000000, 000000, 000000, 000000, 000000, 000000, 000000, 000000, 000000, 000000, 000000\nid: en, index: 0\ntimestamp: 00:00:00:000, filepos: 000000000\n"
	return os.WriteFile(idx, []byte(text), 0o644)
}

// cueFor writes a two-track sheet for one sound file.
func cueFor(sheet, audio, kind string) error {
	text := "PERFORMER \"The Made-Up Band\"\nTITLE \"Made-Up Album\"\nFILE \"" + filepath.Base(audio) + "\" " + kind + "\n  TRACK 01 AUDIO\n    TITLE \"First Song\"\n    INDEX 01 00:00:00\n  TRACK 02 AUDIO\n    TITLE \"Second Song\"\n    INDEX 01 00:00:50\n"
	return os.WriteFile(sheet, []byte(text), 0o644)
}

// dvdFolder authors a real if tiny DVD, and returns its folder.
func (st *sampleTools) dvdFolder(name string) (string, error) {
	vob := filepath.Join(st.dir, name+"-src.mpg")
	base := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=720x480:rate=30000/1001:duration=2", "-f", "lavfi", "-i", "sine=frequency=440:duration=2:sample_rate=48000",
		"-target", "ntsc-dvd", "-aspect", "4:3", vob}
	if err := st.run("ffmpeg", base...); err != nil {
		return "", err
	}
	out := filepath.Join(st.dir, name)
	p, err := st.tool("dvdauthor")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", err
	}
	spec := filepath.Join(st.dir, name+".xml")
	xml := `<dvdauthor dest="` + out + `"><vmgm/><titleset><titles><video format="ntsc" aspect="4:3"/><pgc><vob file="` + vob + `"/></pgc></titles></titleset></dvdauthor>`
	if err := os.WriteFile(spec, []byte(xml), 0o644); err != nil {
		return "", err
	}
	cmd := exec.Command(p, "-x", spec)
	cmd.Env = append(os.Environ(), "VIDEO_FORMAT=NTSC")
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("dvdauthor: %w: %s", err, b)
	}
	return out, nil
}

func (st *sampleTools) dvdISO(out string) error {
	dir, err := st.dvdFolder("dvdsrc-" + strings.ReplaceAll(filepath.Base(out), ".", "-"))
	if err != nil {
		return err
	}
	return st.run("genisoimage", "-quiet", "-dvd-video", "-V", "MADE_UP_DISC", "-o", out, dir)
}

func copyFile(dst, src string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// scrambleVOBs marks the video and sound of a DVD image as encrypted, as CSS does.
// Nothing here encrypts anything: the mark is what the scan looks for.
func scrambleVOBs(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	const sector = 2048
	for off := 0; off+sector <= len(b); off += sector {
		pack := b[off : off+sector]
		if pack[0] != 0 || pack[1] != 0 || pack[2] != 1 || pack[3] != 0xBA {
			continue
		}
		p := 14 + int(pack[13]&7)
		for p+9 <= len(pack) && pack[p] == 0 && pack[p+1] == 0 && pack[p+2] == 1 {
			id := pack[p+3]
			l := int(binary.BigEndian.Uint16(pack[p+4:]))
			if id != 0xBB && id != 0xBE && id != 0xBF {
				pack[p+6] |= 0x30
			}
			p += 6 + l
		}
	}
	return os.WriteFile(path, b, 0o644)
}
