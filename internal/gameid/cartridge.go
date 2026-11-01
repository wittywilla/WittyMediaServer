package gameid

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// Cartridges carry a header because the console itself needed one: a logo it checks
// before it will start, a checksum, a magic number. Those are the same in every real
// game and in nothing else, which is what makes them proof.

var (
	gbLogoStart = []byte{0xCE, 0xED, 0x66, 0x66, 0xCC, 0x0D, 0x00, 0x0B, 0x03, 0x73, 0x00, 0x83, 0x00, 0x0C, 0x00, 0x0D}
	armLogo     = []byte{0x24, 0xFF, 0xAE, 0x51}
)

func sniffCartridge(r io.ReaderAt, size int64, head []byte) (Finding, bool) {
	switch {
	case bytes.HasPrefix(head, []byte("NES\x1a")):
		return Finding{Console: "nes", Kind: KindGame, Proof: "It starts with the NES file header."}, true
	case bytes.HasPrefix(head, []byte("UNIF")):
		return Finding{Console: "nes", Kind: KindGame, Proof: "It starts with a UNIF cartridge header."}, true
	case bytes.HasPrefix(head, []byte("FDS\x1a")), bytes.HasPrefix(head, []byte("\x01*NINTENDO-HVC*")):
		return Finding{Console: "nes", Kind: KindGame, Proof: "It starts with the Famicom Disk System's header."}, true
	case bytes.HasPrefix(head, []byte("NESM\x1a")), bytes.HasPrefix(head, []byte("SNESM")), bytes.HasPrefix(head, []byte("SNES-SPC700")), bytes.HasPrefix(head, []byte("GBS\x01")), bytes.HasPrefix(head, []byte("PSF")):
		return Finding{Kind: KindAudio, Proof: "It's a file of console music, not a game."}, true
	case bytes.HasPrefix(head, []byte("BANDAI SFC-ADX")):
		return Finding{Console: "snes", Kind: KindGame, Proof: "It starts with the Sufami Turbo cartridge's mark."}, true
	case bytes.HasPrefix(head, []byte("LYNX")):
		return Finding{Console: "lynx", Kind: KindGame, Proof: "It starts with the Lynx cartridge header."}, true
	case len(head) >= 10 && bytes.Equal(head[1:10], []byte("ATARI7800")):
		f := Finding{Console: "a7800", Kind: KindGame, Proof: "It starts with the Atari 7800 header."}
		if len(head) >= 0x31 {
			if t := clean(head[0x11:0x31]); usableTitle(t) {
				f.Title = t
			}
		}
		return f, true
	}
	if f, ok := sniffN64(head); ok {
		return f, true
	}
	if len(head) >= 0x150 && bytes.Equal(head[0x104:0x114], gbLogoStart) {
		f := Finding{Console: "gb", Kind: KindGame, Proof: "It holds the Nintendo logo the Game Boy checks before starting."}
		if t := clean(head[0x134:0x144]); usableTitle(t) {
			f.Title = t
		}
		return f, true
	}
	if len(head) >= 0xC0 && head[0xB2] == 0x96 && bytes.Equal(head[4:8], armLogo) {
		f := Finding{Console: "gba", Kind: KindGame, Proof: "It holds the Nintendo logo and the fixed value the Game Boy Advance checks before starting."}
		if t := clean(head[0xA0:0xAC]); usableTitle(t) {
			f.Title = t
		}
		if code := clean(head[0xAC:0xB0]); len(code) == 4 {
			f.Serial, f.Region = code, cartRegion(code[3])
		}
		return f, true
	}
	if len(head) >= 0x160 && (binary.LittleEndian.Uint16(head[0x15C:]) == 0xCF56 || bytes.Equal(head[0xC0:0xC4], armLogo)) {
		f := Finding{Console: "nds", Kind: KindGame, Proof: "Its header has the Nintendo logo and checksum a DS checks before starting."}
		if t := clean(head[0:12]); usableTitle(t) {
			f.Title = t
		}
		if code := clean(head[0x0C:0x10]); len(code) == 4 && code != "####" {
			f.Serial, f.Region = code, cartRegion(code[3])
		}
		return f, true
	}
	if f, ok := sniffMegaDrive(head, size); ok {
		return f, true
	}
	if f, ok := sniffSegaMaster(r, size); ok {
		return f, true
	}
	if len(head) >= 0x30 && (bytes.HasPrefix(head, []byte("COPYRIGHT BY SNK")) || bytes.HasPrefix(head, []byte(" LICENSED BY SNK"))) {
		f := Finding{Console: "ngp", Kind: KindGame, Proof: "It starts with the copyright line a Neo Geo Pocket checks."}
		if t := clean(head[0x24:0x30]); usableTitle(t) {
			f.Title = t
		}
		return f, true
	}
	return Finding{}, false
}

// cartRegion reads the last letter of a Nintendo cartridge code.
func cartRegion(c byte) string {
	switch c {
	case 'E':
		return "USA"
	case 'P':
		return "Europe"
	case 'J':
		return "Japan"
	case 'K':
		return "Korea"
	case 'D':
		return "Germany"
	case 'F':
		return "France"
	case 'S':
		return "Spain"
	case 'I':
		return "Italy"
	}
	return ""
}

// sniffN64 reads the Nintendo 64's first word, which sets up its cartridge bus and is
// the same in every game. Dumps come in three byte orders, and that word shows which.
func sniffN64(head []byte) (Finding, bool) {
	if len(head) < 0x40 {
		return Finding{}, false
	}
	var b []byte
	order := ""
	switch {
	case head[0] == 0x80 && head[3] == 0x40 && (head[1] == 0x37 || head[1] == 0x27 || head[1] == 0x20):
		b, order = head[:0x40], "big-endian"
	case head[0] == 0x37 && head[1] == 0x80 && head[2] == 0x40:
		b = make([]byte, 0x40)
		for i := 0; i+1 < 0x40; i += 2 {
			b[i], b[i+1] = head[i+1], head[i]
		}
		order = "byte-swapped"
	case head[0] == 0x40 && head[1] == 0x12 && head[2] == 0x37 && head[3] == 0x80:
		b = make([]byte, 0x40)
		for i := 0; i+3 < 0x40; i += 4 {
			b[i], b[i+1], b[i+2], b[i+3] = head[i+3], head[i+2], head[i+1], head[i]
		}
		order = "little-endian"
	default:
		return Finding{}, false
	}
	f := Finding{Console: "n64", Kind: KindGame, Proof: fmt.Sprintf("It starts with the Nintendo 64's cartridge settings, in %s order.", order)}
	if t := clean(b[0x20:0x34]); usableTitle(t) {
		f.Title = t
	}
	if code := clean(b[0x3B:0x3F]); len(code) == 4 {
		f.Serial, f.Region = code, cartRegion(code[3])
	}
	return f, true
}

func sniffMegaDrive(head []byte, size int64) (Finding, bool) {
	if len(head) >= 0x200 && bytes.Contains(head[0x100:0x105], []byte("SEGA")) {
		if bytes.HasPrefix(head[0x100:], []byte("SEGA 32X")) {
			return Finding{Console: Sega32X, Kind: KindGame, Proof: "Its header says SEGA 32X."}, true
		}
		f := Finding{Console: "md", Kind: KindGame, Proof: "Its header says " + clean(head[0x100:0x110]) + "."}
		// The overseas title reads better for most people, and is what lists use.
		for _, t := range []string{clean(head[0x150:0x180]), clean(head[0x120:0x150])} {
			if usableTitle(t) {
				f.Title = t
				break
			}
		}
		if code := clean(head[0x180:0x18E]); code != "" {
			f.Serial = code
		}
		return f, true
	}
	// A copier's interleaved dump has a header of its own, and the real one scrambled.
	if size%16384 == 512 && len(head) > 10 && head[8] == 0xAA && head[9] == 0xBB {
		return Finding{Console: "md", Kind: KindGame, Proof: "It has the header of a Mega Drive copier's dump."}, true
	}
	return Finding{}, false
}

// sniffSegaMaster looks for the Master System's TMR SEGA mark, which sits near the end
// of the first 8, 16 or 32 KB. A digit beside it says Master System or Game Gear. Many
// Japanese cartridges don't have it, so its absence means nothing.
func sniffSegaMaster(r io.ReaderAt, size int64) (Finding, bool) {
	for _, off := range []int64{0x7FF0, 0x3FF0, 0x1FF0} {
		if off+16 > size {
			continue
		}
		b := at(r, off, 16)
		if b == nil || !bytes.Equal(b[:8], []byte("TMR SEGA")) {
			continue
		}
		switch b[15] >> 4 {
		case 3, 4:
			return Finding{Console: "sms", Kind: KindGame, Proof: "It holds the TMR SEGA mark, set to Master System."}, true
		case 5, 6, 7:
			return Finding{Console: "gg", Kind: KindGame, Proof: "It holds the TMR SEGA mark, set to Game Gear."}, true
		}
		return Finding{Console: "sms", Kind: KindGame, Proof: "It holds the TMR SEGA mark."}, true
	}
	return Finding{}, false
}

// sniffSNES finds the Super Nintendo's internal header. It has no magic number, so its
// fields are scored at each place it can be, with a copier's 512 bytes or without.
// Only a score no other file would reach by chance counts as proof.
func sniffSNES(r io.ReaderAt, size int64) (Finding, bool) {
	if size < 0x8000 || size > 16<<20 {
		return Finding{}, false
	}
	copier := int64(0)
	if size%1024 == 512 {
		copier = 512
	}
	best, bestAt := 0, int64(-1)
	for _, base := range []int64{0x7FB0, 0xFFB0, 0x40FFB0} {
		if s := snesScore(r, copier+base, size, base); s > best {
			best, bestAt = s, copier+base
		}
	}
	if best < 8 {
		return Finding{}, false
	}
	f := Finding{Console: "snes", Kind: KindGame, Proof: "Its internal header holds the Super Nintendo's title, memory map and checksum."}
	if h := at(r, bestAt, 0x50); h != nil {
		if t := clean(h[0x10:0x25]); usableTitle(t) {
			f.Title = t
		}
		if h[0x29] < 0x14 {
			f.Region = snesRegion(h[0x29])
		}
	}
	return f, true
}

func snesRegion(c byte) string {
	switch c {
	case 0:
		return "Japan"
	case 1:
		return "USA"
	case 2, 3, 4, 5, 6, 7, 8, 9, 0xA, 0xB, 0xC, 0x11:
		return "Europe"
	case 0xD:
		return "Korea"
	}
	return ""
}

func snesScore(r io.ReaderAt, off, size, base int64) int {
	if off+0x50 > size {
		return 0
	}
	h := at(r, off, 0x50)
	if h == nil {
		return 0
	}
	score := 0
	mode := h[0x25]
	if mode&0xE0 == 0x20 {
		score += 2
		// The map mode has to fit where its header is.
		hi := mode&1 == 1 || mode&0x0F == 5
		if (base != 0x7FB0) == hi {
			score += 2
		}
	}
	if h[0x27] >= 7 && h[0x27] <= 13 {
		score++
	}
	if h[0x28] <= 8 {
		score++
	}
	if h[0x29] < 0x14 {
		score++
	}
	complement := binary.LittleEndian.Uint16(h[0x2C:])
	checksum := binary.LittleEndian.Uint16(h[0x2E:])
	if complement^checksum == 0xFFFF && checksum != 0 && checksum != 0xFFFF {
		score += 4
	}
	if reset := binary.LittleEndian.Uint16(h[0x4C:]); reset >= 0x8000 && reset != 0xFFFF {
		score += 2
	}
	return score
}

// sniffBIOS recognises the console start-up files most often left among games.
func sniffBIOS(r io.ReaderAt, size int64) (Finding, bool) {
	// The PlayStation 2's lists its own modules right at the start, and names itself.
	if size >= 4<<20 && size <= 32<<20 {
		if b := at(r, 0x2000, 0x1000); b != nil && bytes.Contains(b, []byte("RESET\x00")) {
			return Finding{Console: "ps2", Kind: KindBIOS, Proof: "It holds the module list of a PlayStation 2 BIOS."}, true
		}
	}
	if size == 512<<10 {
		if b := at(r, 0x100, 0x40); b != nil && bytes.Contains(b, []byte("Sony Computer Entertainment")) {
			return Finding{Console: "ps1", Kind: KindBIOS, Proof: "It's the size of a PlayStation BIOS, and holds Sony's copyright line."}, true
		}
	}
	return Finding{}, false
}

// CheckFor is for consoles whose cartridges have no header at all. It can't prove a
// file is for one of these, only say whether a file a folder says is for one could be.
func CheckFor(console string, r io.ReaderAt, size int64) (fits bool, why string) {
	switch console {
	case "nes":
		// A dump with no header at all. Most cores want one, and Mesen can find what it needs
		// from a checksum, so it's worth listing if it could be a cartridge's memory.
		b := at(r, 0, 16)
		same := b != nil
		for _, c := range b {
			same = same && c == b[0]
		}
		if size >= 16<<10 && size%(8<<10) == 0 && !same {
			return true, "It has no NES header, so only some cores will play it. Mesen, if it's installed, works it out from its checksum."
		}
		return false, "It has no NES header, and doesn't look like a cartridge's memory either."
	case "a2600":
		// Bank-switched carts come in a fixed list of sizes.
		switch size {
		case 2 << 10, 4 << 10, 6 << 10, 8 << 10, 8192 + 256, 10 << 10, 10495, 12 << 10, 16 << 10, 24 << 10, 28 << 10, 29 << 10, 32 << 10, 64 << 10, 128 << 10, 256 << 10, 512 << 10:
			return true, "It's the size of an Atari 2600 cartridge."
		}
		return false, fmt.Sprintf("At %d bytes it isn't the size of any Atari 2600 cartridge.", size)
	case "pce":
		if size > 0 && (size%8192 == 0 || size%8192 == 512) {
			return true, "It's the size of a PC Engine card."
		}
		return false, "It isn't the size of a PC Engine card."
	case "vb":
		if size >= 64<<10 && size <= 16<<20 && size&(size-1) == 0 {
			return true, "It's the size of a Virtual Boy cartridge."
		}
		return false, "It isn't the size of a Virtual Boy cartridge."
	case "ws":
		// A WonderSwan cartridge ends in a far jump to where it starts.
		if size >= 128<<10 && size <= 32<<20 && size%(64<<10) == 0 {
			if b := at(r, size-16, 1); b != nil && b[0] == 0xEA {
				return true, "It ends with the start-up jump a WonderSwan cartridge has."
			}
		}
		return false, "It doesn't end the way a WonderSwan cartridge does."
	}
	return true, ""
}
