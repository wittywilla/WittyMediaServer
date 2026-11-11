// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gameid

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Discs are told apart by what's on them. PlayStation 1, PlayStation 2 and PSP discs
// are all ISO 9660, so what proves one is the file that starts the game. Sega's discs
// start with a boot sector that names the console.

// isoImage is an ISO 9660 disc, in whichever sector layout it was dumped.
type isoImage struct {
	r              io.ReaderAt
	sector, offset int64
	pvd            []byte
}

// discLayouts are the sector sizes and data offsets dumps use: plain 2048-byte sectors,
// or raw ones with their headers.
var discLayouts = [][2]int64{{2048, 0}, {2352, 24}, {2352, 16}, {2336, 8}, {2448, 24}, {2448, 16}}

// openISO finds a disc's volume descriptor, the mark of ISO 9660, 16 sectors in.
func openISO(r io.ReaderAt) *isoImage {
	for _, l := range discLayouts {
		b := at(r, 16*l[0]+l[1], 2048)
		if b != nil && b[0] == 1 && string(b[1:6]) == "CD001" {
			return &isoImage{r: r, sector: l[0], offset: l[1], pvd: b}
		}
	}
	return nil
}

func (i *isoImage) block(lba int64) []byte {
	return at(i.r, lba*i.sector+i.offset, 2048)
}

// dirEntry is one file or folder in a disc's folder.
type dirEntry struct {
	name string
	lba  int64
	size int64
	dir  bool
}

// list reads a folder. Records can claim anything, so each is checked against its
// block, and a folder is read up to 256 KB.
func (i *isoImage) list(lba, size int64) []dirEntry {
	size = min(size, 256<<10)
	var out []dirEntry
	for s := int64(0); s*2048 < size; s++ {
		b := i.block(lba + s)
		if b == nil {
			break
		}
		for p := 0; p+33 < len(b); {
			n := int(b[p])
			if n == 0 {
				break
			}
			if n < 34 || p+n > len(b) {
				break
			}
			nameLen := int(b[p+32])
			if 33+nameLen <= n {
				name := strings.ToUpper(string(b[p+33 : p+33+nameLen]))
				if j := strings.IndexByte(name, ';'); j >= 0 {
					name = name[:j]
				}
				out = append(out, dirEntry{name: strings.TrimSuffix(name, "."),
					lba:  int64(binary.LittleEndian.Uint32(b[p+2:])),
					size: int64(binary.LittleEndian.Uint32(b[p+10:])),
					dir:  b[p+25]&2 != 0})
			}
			p += n
		}
	}
	return out
}

// root lists the disc's top folder.
func (i *isoImage) root() []dirEntry {
	return i.list(int64(binary.LittleEndian.Uint32(i.pvd[156+2:])), int64(binary.LittleEndian.Uint32(i.pvd[156+10:])))
}

func findEntry(list []dirEntry, name string) (dirEntry, bool) {
	for _, e := range list {
		if e.name == name {
			return e, true
		}
	}
	return dirEntry{}, false
}

// read gets up to limit bytes of a file on the disc.
func (i *isoImage) read(e dirEntry, limit int64) []byte {
	want := min(e.size, limit)
	out := make([]byte, 0, want)
	for s := e.lba; int64(len(out)) < want; s++ {
		b := i.block(s)
		if b == nil {
			break
		}
		out = append(out, b[:min(2048, want-int64(len(out)))]...)
	}
	return out
}

var bootRe = regexp.MustCompile(`(?im)^\s*(BOOT2?)\s*=\s*cdrom0?:[\\/]*(?:[^\r\n;]*[\\/])?([A-Z]{4})[_-]?([0-9]{3})\.?([0-9]{2})`)

// sniffDisc says whose disc this is.
func sniffDisc(r io.ReaderAt, size int64) (Finding, bool) {
	if f, ok := sniffBootSector(r, size); ok {
		return f, true
	}
	iso := openISO(r)
	if iso == nil {
		return Finding{}, false
	}
	return sniffISO(iso, size), true
}

// sniffISO looks inside a plain ISO 9660 disc.
func sniffISO(iso *isoImage, size int64) Finding {
	root := iso.root()
	system := clean(iso.pvd[8:40])
	if e, ok := findEntry(root, "SYSTEM.CNF"); ok {
		cnf := iso.read(e, 4096)
		if m := bootRe.FindSubmatch(cnf); m != nil {
			serial := strings.ToUpper(string(m[2])) + "-" + string(m[3]) + string(m[4])
			console, name := "ps1", "PlayStation"
			if strings.EqualFold(string(m[1]), "BOOT2") {
				console, name = "ps2", "PlayStation 2"
			}
			return Finding{Console: console, Kind: KindGame, Serial: serial, Region: serialRegion(serial),
				Proof: fmt.Sprintf("Its SYSTEM.CNF starts a %s program, %s.", name, serial)}
		}
	}
	if dir, ok := findEntry(root, "PSP_GAME"); ok && dir.dir {
		f := Finding{Console: "psp", Kind: KindGame, Proof: "It has the PSP_GAME folder of a PSP disc."}
		if sfo, ok := findEntry(iso.list(dir.lba, dir.size), "PARAM.SFO"); ok {
			applySFO(&f, iso.read(sfo, 64<<10))
		}
		if f.Serial == "" {
			if umd, ok := findEntry(root, "UMD_DATA.BIN"); ok {
				if code, _, ok := strings.Cut(string(iso.read(umd, 32)), "|"); ok {
					f.Serial = strings.TrimSpace(code)
				}
			}
		}
		f.Region = serialRegion(f.Serial)
		return f
	}
	if strings.HasPrefix(system, "PSP GAME") {
		return Finding{Console: "psp", Kind: KindGame, Proof: "Its volume says it's a PSP game."}
	}
	if _, ok := findEntry(root, "PSX.EXE"); ok {
		return Finding{Console: "ps1", Kind: KindGame, Proof: "It starts from PSX.EXE, as an early PlayStation disc does."}
	}
	if strings.HasPrefix(system, "PLAYSTATION") {
		// Sony's mark, with nothing to say which console: only the second has DVDs.
		if size > 900<<20 {
			return Finding{Console: "ps2", Kind: KindGame, Proof: "Its volume is marked PLAYSTATION, and it's too big for a CD."}
		}
		return Finding{Console: "ps1", Kind: KindGame, Proof: "Its volume is marked PLAYSTATION."}
	}
	if _, ok := findEntry(root, "IPL.TXT"); ok {
		return Finding{Console: NeoGeoCD, Kind: KindGame, Proof: "It has the IPL.TXT start-up list of a Neo Geo CD."}
	}
	if _, ok := findEntry(root, "VIDEO_TS"); ok {
		return Finding{Kind: KindOther, Proof: "It's a DVD video, not a game."}
	}
	if _, ok := findEntry(root, "BDMV"); ok {
		return Finding{Kind: KindOther, Proof: "It's a Blu-ray video, not a game."}
	}
	for _, name := range []string{"AUTORUN.INF", "SETUP.EXE", "INSTALL.EXE"} {
		if _, ok := findEntry(root, name); ok {
			return Finding{Kind: KindOther, Proof: "It's a PC disc: it has " + name + "."}
		}
	}
	return Finding{Proof: "It's a CD or DVD image, with nothing on it that says which console it's for."}
}

// applySFO fills in what a PSP's PARAM.SFO says.
func applySFO(f *Finding, b []byte) {
	sfo := parseSFO(b)
	if t := sfo["TITLE"]; usableTitle(t) {
		f.Title = t
	}
	if id := sfo["DISC_ID"]; id != "" {
		f.Serial = normalPSP(id)
	}
	switch sfo["CATEGORY"] {
	case "PG":
		f.Kind, f.Proof = KindUpdate, "Its PARAM.SFO marks it as an update for a game, not a game."
	case "ME":
		f.Console = "ps1"
		f.Proof = "Its PARAM.SFO marks it as a PlayStation game packaged for the PSP."
	}
}

var pspIDRe = regexp.MustCompile(`^([A-Z]{4})-?([0-9]{5})$`)

func normalPSP(id string) string {
	if m := pspIDRe.FindStringSubmatch(id); m != nil {
		return m[1] + "-" + m[2]
	}
	return id
}

// parseSFO reads the key and value table of a PARAM.SFO. Every offset in it is checked:
// nothing here trusts a file to be well formed.
func parseSFO(b []byte) map[string]string {
	out := map[string]string{}
	if len(b) < 20 || !bytes.HasPrefix(b, []byte("\x00PSF")) {
		return out
	}
	le := binary.LittleEndian
	keys, data, n := int(le.Uint32(b[8:])), int(le.Uint32(b[12:])), int(le.Uint32(b[16:]))
	if n > 256 || keys <= 0 || data <= 0 || keys > len(b) || data > len(b) {
		return out
	}
	for i := 0; i < n; i++ {
		rec := 20 + i*16
		if rec+16 > len(b) {
			break
		}
		keyAt := keys + int(le.Uint16(b[rec:]))
		format := le.Uint16(b[rec+2:])
		length := int(le.Uint32(b[rec+4:]))
		valueAt := data + int(le.Uint32(b[rec+12:]))
		if keyAt >= len(b) || valueAt < 0 || length < 0 || valueAt+length > len(b) {
			continue
		}
		end := bytes.IndexByte(b[keyAt:], 0)
		if end <= 0 {
			continue
		}
		if format == 0x0204 || format == 0x0004 { // text
			out[string(b[keyAt:keyAt+end])] = clean(b[valueAt : valueAt+length])
		}
	}
	return out
}

// bootSectorAt lists where a boot sector can be: at the very start of a plain image,
// or after the header of a raw sector.
var bootSectorAt = []int64{0, 16, 24}

// sniffBootSector looks for consoles whose discs say so in their first sector.
func sniffBootSector(r io.ReaderAt, size int64) (Finding, bool) {
	for _, off := range bootSectorAt {
		b := at(r, off, 0x200)
		if b == nil {
			continue
		}
		switch {
		case bytes.HasPrefix(b, []byte("SEGA SEGAKATANA")):
			f := Finding{Console: "dc", Kind: KindGame, Proof: "Its boot sector says SEGA SEGAKATANA, as a Dreamcast disc does."}
			f.Serial = clean(b[0x40:0x4A])
			if t := clean(b[0x80:0x100]); usableTitle(t) {
				f.Title = t
			}
			return f, true
		case bytes.HasPrefix(b, []byte("SEGA SEGASATURN")):
			f := Finding{Console: Saturn, Kind: KindGame, Proof: "Its boot sector says SEGA SEGASATURN."}
			f.Serial = clean(b[0x20:0x2A])
			if t := clean(b[0x60:0xD0]); usableTitle(t) {
				f.Title = t
			}
			return f, true
		case bytes.HasPrefix(b, []byte("SEGADISCSYSTEM")), bytes.HasPrefix(b, []byte("SEGABOOTDISC")), bytes.HasPrefix(b, []byte("SEGADISC")):
			f := Finding{Console: SegaCD, Kind: KindGame, Proof: "Its boot sector says SEGADISCSYSTEM, as a Sega CD disc does."}
			f.Serial = clean(b[0x180:0x18E])
			if t := clean(b[0x150:0x180]); usableTitle(t) {
				f.Title = t
			}
			return f, true
		}
	}
	// GameCube and Wii discs carry a magic number in their first bytes.
	if b := at(r, 0, 0x40); b != nil {
		be := binary.BigEndian
		switch {
		case be.Uint32(b[0x18:]) == 0x5D1C9EA3:
			return Finding{Console: Wii, Kind: KindGame, Serial: clean(b[:6]), Title: titleIf(clean(b[0x20:0x40])), Proof: "It has the magic number of a Wii disc."}, true
		case be.Uint32(b[0x1C:]) == 0xC2339F3D:
			return Finding{Console: GameCube, Kind: KindGame, Serial: clean(b[:6]), Title: titleIf(clean(b[0x20:0x40])), Proof: "It has the magic number of a GameCube disc."}, true
		case bytes.HasPrefix(b, []byte("\x01ZZZZZ\x01")):
			return Finding{Console: ThreeDO, Kind: KindGame, Proof: "It starts with the volume mark of a 3DO disc."}, true
		}
	}
	if b := at(r, 0x10000, 20); b != nil && bytes.HasPrefix(b, []byte("MICROSOFT*XBOX*MEDIA")) {
		return Finding{Console: Xbox, Kind: KindGame, Proof: "It has the volume mark of an Xbox disc."}, true
	}
	// A PC Engine CD names itself in the second sector of its data track.
	for _, off := range []int64{2048, 2352 + 16, 2352 + 24} {
		if b := at(r, off, 0x60); b != nil && bytes.Contains(b, []byte("PC Engine CD-ROM SYSTEM")) {
			return Finding{Console: "pce", Kind: KindGame, Proof: "It names the PC Engine CD-ROM system in its second sector."}, true
		}
		if b := at(r, off, 0x20); b != nil && bytes.HasPrefix(b, []byte("PC-FX:Hu_CD-ROM")) {
			return Finding{Console: PCFX, Kind: KindGame, Proof: "It names the PC-FX in its second sector."}, true
		}
	}
	return Finding{}, false
}

func titleIf(s string) string {
	if usableTitle(s) {
		return s
	}
	return ""
}

// sniffPBP reads a PSP eboot. It's a table of the parts inside, the first being its
// PARAM.SFO, which says what it is. A PlayStation game packed for a PSP is one too.
func sniffPBP(r io.ReaderAt, size int64) Finding {
	table := at(r, 8, 8)
	if table == nil {
		return Finding{Proof: "It starts like a PSP eboot, but it's cut short."}
	}
	le := binary.LittleEndian
	sfoAt, next := int64(le.Uint32(table)), int64(le.Uint32(table[4:]))
	f := Finding{Console: "psp", Kind: KindGame, Proof: "It's a PSP eboot."}
	if sfoAt > 0 && next > sfoAt && next-sfoAt <= 64<<10 && next <= size {
		applySFO(&f, at(r, sfoAt, int(next-sfoAt)))
	}
	// The data part of a converted PlayStation game starts with its own mark.
	if parts := at(r, 8, 0x20); parts != nil {
		data := int64(le.Uint32(parts[0x1C:]))
		if b := at(r, data, 10); b != nil && (bytes.HasPrefix(b, []byte("PSISOIMG")) || bytes.HasPrefix(b, []byte("PSTITLEIMG"))) {
			f.Console, f.Proof = "ps1", "It's a PlayStation game packed as a PSP eboot."
		}
	}
	return f
}

// sniffCHD reads a compressed disc image's metadata, which says what kind of disc it
// is. Its contents would take a full set of decompressors, so which console it's for
// is left to where it's kept.
func sniffCHD(r io.ReaderAt, size int64) Finding {
	h := at(r, 0, 64)
	f := Finding{Proof: "It's a compressed disc image, which doesn't say which console it's for."}
	if h == nil {
		return f
	}
	be := binary.BigEndian
	var meta int64
	switch be.Uint32(h[12:]) {
	case 5:
		meta = int64(be.Uint64(h[48:]))
	case 3, 4:
		meta = int64(be.Uint64(h[36:]))
	default:
		return f
	}
	for i := 0; i < 64 && meta > 0 && meta < size; i++ {
		e := at(r, meta, 16)
		if e == nil {
			break
		}
		switch string(e[:4]) {
		case "CHGD", "CHGT":
			f.Media = "gd"
			f.Console, f.Kind, f.Proof = "dc", KindGame, "It's a compressed GD-ROM, which only a Dreamcast uses here."
			return f
		case "CHCD", "CHT2", "CHTR":
			f.Media = "cd"
			f.Proof = "It's a compressed CD image, which doesn't say which console it's for."
			return f
		case "DVD ":
			f.Media = "dvd"
			f.Proof = "It's a compressed DVD image, which doesn't say which console it's for."
			return f
		}
		meta = int64(be.Uint64(e[8:]))
	}
	return f
}
