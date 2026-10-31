package gamemeta

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/gamearchive"
	"github.com/wittywilla/wittyplay"
)

// Working out which exact game a file is, as cheaply as possible, and without ever
// sending any of it anywhere. An archive already records its files' checksums, so only
// its headers are read, in the worker process that reads all archives. A cartridge is
// small enough to read through once. A disc isn't: its product code is on the disc, a
// few sectors in, and that's all that's read of it.

// Identity is what a file says about itself.
type Identity struct {
	CRCs   []uint32
	Size   int64
	Serial string
}

// maxHashed is the biggest file read whole to take its checksum. Every cartridge game
// is well under this, and a slow disk shouldn't have to read a disc image for nothing.
const maxHashed = 128 << 20

var hashBuf = make(chan []byte, 1)

// notGameExts are files inside an archive that aren't the game.
var notGameExts = map[string]bool{".cue": true, ".m3u": true, ".gdi": true, ".ccd": true, ".toc": true, ".sub": true, ".sbi": true,
	".txt": true, ".nfo": true, ".diz": true, ".pdf": true, ".url": true, ".htm": true, ".html": true, ".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".bmp": true, ".sfv": true, ".md5": true, ".sha1": true, ".dat": true}

// Identifier looks at game files one after another, keeping one archive worker going
// between them. It isn't for use by two goroutines at once.
type Identifier struct {
	lister *gamearchive.Lister
}

// Close ends the archive worker, if one was started.
func (x *Identifier) Close() {
	if x.lister != nil {
		x.lister.Close()
		x.lister = nil
	}
}

// Identify looks at a game file. It always returns something: at worst, nothing known.
func (x *Identifier) Identify(p, console string) Identity {
	name := filepath.Base(p)
	if gamearchive.IsArchive(name) {
		if x.lister == nil {
			x.lister = gamearchive.NewLister()
		}
		l, err := x.lister.List(p, gamearchive.ScanBudget)
		if err != nil {
			return Identity{}
		}
		return identifyListing(l, console)
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".cue", ".m3u", ".gdi":
		if first := firstTrack(p); first != "" {
			return identifyDisc(first, console)
		}
		return Identity{}
	case ".chd", ".cso", ".pbp", ".rvz", ".gcz", ".wbfs", ".cdi", ".xiso":
		// Compressed discs: nothing cheap to read, so these go by name.
		return Identity{}
	case ".iso", ".img", ".bin", ".gcm":
		st, err := os.Stat(p)
		if err == nil && st.Size() > maxHashed {
			return identifyDisc(p, console)
		}
	}
	st, err := os.Stat(p)
	if err != nil || !st.Mode().IsRegular() || st.Size() > maxHashed {
		return Identity{}
	}
	return Identity{CRCs: fileCRCs(p, st.Size(), strings.ToLower(filepath.Ext(name))), Size: st.Size(), Serial: discSerial(p, console)}
}

func identifyDisc(p, console string) Identity {
	id := Identity{Serial: discSerial(p, console)}
	if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
		id.Size = st.Size()
		if st.Size() <= maxHashed {
			id.CRCs = fileCRCs(p, st.Size(), strings.ToLower(filepath.Ext(p)))
		}
	}
	return id
}

func identifyListing(l *gamearchive.Listing, console string) Identity {
	c := wittyplay.ByID(console)
	var picks []gamearchive.Entry
	for _, e := range l.Entries {
		x := strings.ToLower(path.Ext(e.Name))
		if e.CRC == 0 || notGameExts[x] {
			continue
		}
		if c == nil || c.Accepts(x) || wittyplay.KnownExtension(x) {
			picks = append(picks, e)
		}
	}
	// For discs, track 1 holds the data and is what databases key on; the name sorts it
	// first. A lone game is the only entry anyway. Biggest next: a game's main file.
	sort.SliceStable(picks, func(a, b int) bool {
		ta, tb := isFirstTrack(picks[a].Name), isFirstTrack(picks[b].Name)
		if ta != tb {
			return ta
		}
		return picks[a].Size > picks[b].Size
	})
	id := Identity{}
	for i, e := range picks {
		if i >= 6 {
			break
		}
		id.CRCs = append(id.CRCs, e.CRC)
		if i == 0 {
			id.Size = e.Size
		}
	}
	return id
}

var firstTrackRe = regexp.MustCompile(`(?i)track\s*0*1\b`)

func isFirstTrack(name string) bool { return firstTrackRe.MatchString(path.Base(name)) }

var (
	cueFileRe = regexp.MustCompile(`(?i)^\s*FILE\s+"?(.+?)"?\s+[A-Z0-9]+\s*$`)
	gdiLineRe = regexp.MustCompile(`^\s*[0-9]+\s+[0-9]+\s+[0-9]+\s+[0-9]+\s+"?([^"]+?)"?\s+[0-9]+\s*$`)
)

// firstTrack finds the first file a cue sheet, playlist or GD-ROM description names,
// staying inside its own folder.
func firstTrack(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	x := strings.ToLower(filepath.Ext(p))
	dir := filepath.Dir(p)
	sc := bufio.NewScanner(io.LimitReader(f, 1<<20))
	for lines := 0; sc.Scan() && lines < 2000; lines++ {
		line := strings.TrimSpace(sc.Text())
		name := ""
		switch x {
		case ".cue":
			if m := cueFileRe.FindStringSubmatch(line); m != nil {
				name = m[1]
			}
		case ".gdi":
			if m := gdiLineRe.FindStringSubmatch(line); m != nil {
				name = m[1]
			}
		default:
			if line != "" && !strings.HasPrefix(line, "#") {
				name = line
			}
		}
		if name == "" {
			continue
		}
		full := filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(name, `\`, "/")))
		if rel, err := filepath.Rel(dir, full); err != nil || strings.HasPrefix(rel, "..") {
			return ""
		}
		if x == ".m3u" {
			return firstInList(full)
		}
		return full
	}
	return ""
}

// firstInList: a playlist's first entry can itself be a cue sheet.
func firstInList(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".cue", ".gdi":
		return firstTrack(p)
	}
	return p
}

// copierHeader says how many bytes of a cartridge dump aren't the cartridge. Databases
// list cartridges without them, and old dumps commonly have them.
func copierHeader(size int64, ext string, head []byte) int64 {
	switch {
	case len(head) >= 16 && bytes.HasPrefix(head, []byte("NES\x1a")):
		return 16
	case len(head) >= 16 && bytes.HasPrefix(head, []byte("FDS\x1a")):
		return 16
	case len(head) >= 128 && bytes.Equal(head[1:10], []byte("ATARI7800")):
		return 128
	case len(head) >= 64 && bytes.HasPrefix(head, []byte("LYNX")):
		return 64
	}
	switch ext {
	case ".smc", ".sfc", ".fig", ".swc":
		if size%1024 == 512 {
			return 512
		}
	}
	return 0
}

// fileCRCs takes a file's checksum, and again without a copier's header if it has one.
// Databases list some dumps one way and some the other.
func fileCRCs(p string, size int64, ext string) []uint32 {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	var buf []byte
	select {
	case buf = <-hashBuf:
	default:
		buf = make([]byte, 256<<10)
	}
	defer func() {
		select {
		case hashBuf <- buf:
		default:
		}
	}()
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil
	}
	head := buf[:n]
	skip := copierHeader(size, ext, head)
	whole := crc32.NewIEEE()
	stripped := crc32.NewIEEE()
	whole.Write(head)
	if skip > 0 && int64(len(head)) >= skip {
		stripped.Write(head[skip:])
	}
	for n == len(buf) {
		n, err = io.ReadFull(f, buf)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return nil
		}
		whole.Write(buf[:n])
		if skip > 0 {
			stripped.Write(buf[:n])
		}
	}
	if skip > 0 {
		return []uint32{whole.Sum32(), stripped.Sum32()}
	}
	return []uint32{whole.Sum32()}
}

// ---------------------------------------------------------------- disc serials

// discSerial reads a disc's product code. It looks only a few sectors into the file and
// gives up quietly on anything it doesn't recognise.
func discSerial(p, console string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	switch console {
	case "ps1", "ps2":
		return playStationSerial(f)
	case "psp":
		return umdSerial(f)
	case "gc", "wii":
		return gameCubeSerial(f)
	case "saturn":
		return segaSerial(f, "SEGA SEGASATURN")
	case "dc":
		return segaSerial(f, "SEGA SEGAKATANA")
	case "segacd":
		return segaSerial(f, "SEGADISCSYSTEM")
	}
	return ""
}

// isoLayout finds how a disc image lays out its sectors, from where the ISO 9660 volume
// descriptor is: plain 2048-byte sectors, or raw 2352-byte ones with a header before
// the data.
func isoLayout(f io.ReaderAt) (sector, offset int64, ok bool) {
	var tag [6]byte
	for _, l := range [][2]int64{{2048, 0}, {2352, 24}, {2352, 16}, {2336, 8}} {
		if _, err := f.ReadAt(tag[:], 16*l[0]+l[1]); err != nil {
			continue
		}
		if string(tag[1:6]) == "CD001" {
			return l[0], l[1], true
		}
	}
	return 0, 0, false
}

// isoFile finds a file in a disc's root folder, and reads up to limit bytes of it.
func isoFile(f io.ReaderAt, want string, limit int) []byte {
	sector, offset, ok := isoLayout(f)
	if !ok {
		return nil
	}
	pvd := make([]byte, 2048)
	if _, err := f.ReadAt(pvd, 16*sector+offset); err != nil {
		return nil
	}
	rootLBA := int64(binary.LittleEndian.Uint32(pvd[156+2:]))
	rootLen := int64(binary.LittleEndian.Uint32(pvd[156+10:]))
	rootLen = min(rootLen, 16*2048)
	for s := int64(0); s*2048 < rootLen; s++ {
		block := make([]byte, 2048)
		if _, err := f.ReadAt(block, (rootLBA+s)*sector+offset); err != nil {
			return nil
		}
		for i := 0; i+33 < len(block); {
			n := int(block[i])
			if n == 0 {
				break
			}
			if i+n > len(block) {
				break
			}
			nameLen := int(block[i+32])
			if 33+nameLen <= n {
				name := strings.ToUpper(string(block[i+33 : i+33+nameLen]))
				name, _, _ = strings.Cut(name, ";")
				if name == want {
					lba := int64(binary.LittleEndian.Uint32(block[i+2:]))
					size := int(binary.LittleEndian.Uint32(block[i+10:]))
					size = min(size, limit)
					if size <= 0 || size > 1<<20 {
						return nil
					}
					out := make([]byte, 0, size)
					for read := int64(0); len(out) < size; read++ {
						chunk := make([]byte, 2048)
						if _, err := f.ReadAt(chunk, (lba+read)*sector+offset); err != nil {
							break
						}
						out = append(out, chunk[:min(2048, size-len(out))]...)
					}
					return out
				}
			}
			i += n
		}
	}
	return nil
}

var bootLineRe = regexp.MustCompile(`(?i)BOOT2?\s*=\s*cdrom0?:\\?([A-Z]{4})[_-]?([0-9]{3})\.?([0-9]{2})`)

// playStationSerial reads SYSTEM.CNF, which names the program to start after the disc.
func playStationSerial(f io.ReaderAt) string {
	cnf := isoFile(f, "SYSTEM.CNF", 2048)
	if m := bootLineRe.FindSubmatch(cnf); m != nil {
		return strings.ToUpper(string(m[1])) + "-" + string(m[2]) + string(m[3])
	}
	return ""
}

var umdRe = regexp.MustCompile(`^([A-Z]{4})-?([0-9]{5})`)

// umdSerial reads a PSP disc's UMD_DATA.BIN.
func umdSerial(f io.ReaderAt) string {
	if m := umdRe.FindSubmatch(isoFile(f, "UMD_DATA.BIN", 64)); m != nil {
		return string(m[1]) + "-" + string(m[2])
	}
	return ""
}

var gcIDRe = regexp.MustCompile(`^[A-Z0-9]{6}$`)

// gameCubeSerial is the first six bytes of a GameCube or Wii disc.
func gameCubeSerial(f io.ReaderAt) string {
	var b [32]byte
	if _, err := f.ReadAt(b[:], 0); err != nil {
		return ""
	}
	if !gcIDRe.Match(b[:6]) {
		return ""
	}
	// A Wii disc says so at 0x18 and a GameCube one at 0x1c.
	if binary.BigEndian.Uint32(b[0x18:]) == 0x5d1c9ea3 || binary.BigEndian.Uint32(b[0x1c:]) == 0xc2339f3d {
		return string(b[:6])
	}
	return ""
}

// segaSerial reads the product code from the IP header at the start of a Sega disc,
// which is at a fixed place after the name of the system.
func segaSerial(f io.ReaderAt, magic string) string {
	for _, off := range []int64{0, 16, 24} {
		b := make([]byte, 0x200)
		if _, err := f.ReadAt(b, off); err != nil {
			continue
		}
		if !bytes.HasPrefix(b, []byte(magic)) {
			continue
		}
		field := b[0x20:0x29]
		if magic == "SEGA SEGAKATANA" {
			field = b[0x40:0x4a]
		} else if magic == "SEGADISCSYSTEM" {
			field = b[0x183:0x18e]
		}
		s := strings.TrimSpace(string(field))
		if s != "" && isSerialText(s) {
			return s
		}
	}
	return ""
}

func isSerialText(s string) bool {
	for _, r := range s {
		if r < 0x20 || r > 0x7e {
			return false
		}
	}
	return true
}
