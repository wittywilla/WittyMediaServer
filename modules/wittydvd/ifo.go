// Package dvd implements DVD-Video navigation: IFO parsing, the command
// virtual machine, the navigator state machine, NAV-pack (PCI/DSI) parsing and
// sub-picture (SPU) decoding for menu highlights.
//
// It is written from the public DVD-Video structure (the same layout
// documented by libdvdread/libdvdnav) and never requires a mounted disc: the
// caller provides IFO bytes and sector-range readers.
package wittydvd

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

const Sector = 2048

// Limits that keep hostile or broken discs from exhausting memory.
const (
	maxCommands    = 1024
	maxPGCs        = 1000
	maxLangUnits   = 100
	maxTitleSets   = 99
	maxPTTEntries  = 100000
	maxCellsPerPGC = 255
)

// Time is a DVD time stamp converted to seconds.
func bcd(b byte) int { return int(b>>4)*10 + int(b&0x0f) }

func dvdTime(b []byte) float64 {
	if len(b) < 4 {
		return 0
	}
	fb := b[3]
	fps := 0.0
	switch fb >> 6 {
	case 1:
		fps = 25
	case 3:
		fps = 30000.0 / 1001.0
	}
	sec := float64(bcd(b[0])*3600 + bcd(b[1])*60 + bcd(b[2]))
	if fps > 0 {
		sec += float64(bcd(fb&0x3f)) / fps
	}
	return sec
}

func langCode(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	a, c := b[0], b[1]
	lower := func(x byte) byte {
		if x >= 'A' && x <= 'Z' {
			return x + 32
		}
		return x
	}
	a, c = lower(a), lower(c)
	if a < 'a' || a > 'z' || c < 'a' || c > 'z' {
		return ""
	}
	return string([]byte{a, c})
}

// VideoAttr describes a video stream.
type VideoAttr struct {
	MPEG        int    `json:"mpeg"`
	Standard    string `json:"standard"` // ntsc | pal
	Wide        bool   `json:"wide"`     // 16:9
	Letterbox   bool   `json:"letterbox"`
	PictureSize int    `json:"pictureSize"`
	PermittedDF int    `json:"permittedDf"`
}

func parseVideoAttr(v uint16) VideoAttr {
	a := VideoAttr{MPEG: 1, Standard: "ntsc"}
	if (v>>14)&3 == 1 {
		a.MPEG = 2
	}
	if (v>>12)&3 == 1 {
		a.Standard = "pal"
	}
	a.Wide = (v>>10)&3 == 3
	a.PermittedDF = int(v>>8) & 3
	a.PictureSize = int(v>>3) & 7
	a.Letterbox = v&4 != 0
	return a
}

// Width/Height of the coded picture.
func (a VideoAttr) Size() (int, int) {
	h := 480
	if a.Standard == "pal" {
		h = 576
	}
	switch a.PictureSize {
	case 1:
		return 704, h
	case 2:
		return 352, h
	case 3:
		return 352, h / 2
	}
	return 720, h
}

// AudioAttr describes an audio stream.
type AudioAttr struct {
	Codec     string `json:"codec"`
	Language  string `json:"language,omitempty"`
	Channels  int    `json:"channels"`
	Extension int    `json:"extension"`
	AppMode   int    `json:"appMode"`
}

var audioCodecs = []string{"ac3", "", "mp1", "mp2", "lpcm", "", "dts", "sdds"}

// Karaoke says the stream is a karaoke mix: backing on the front channels, the
// guide melody and vocals on the others.
func (a AudioAttr) Karaoke() bool { return a.AppMode == 1 && a.Channels > 2 }

// VideoStreamID is the MPEG-PS stream id of a DVD's video, as ffmpeg reports it.
const VideoStreamID = 0x1e0

// StreamID is the MPEG-PS stream id ffmpeg gives physical audio stream n of
// this format (0 when the format has no fixed id).
func (a AudioAttr) StreamID(n int) int {
	switch a.Codec {
	case "ac3":
		return 0x80 + n
	case "dts":
		return 0x88 + n
	case "lpcm":
		return 0xa0 + n
	case "mp1", "mp2":
		return 0x1c0 + n
	}
	return 0
}

func parseAudioAttr(b []byte) AudioAttr {
	a := AudioAttr{Codec: audioCodecs[b[0]>>5], Channels: int(b[1]&7) + 1, Extension: int(b[5]), AppMode: int(b[0] & 3)}
	if a.Codec == "" {
		a.Codec = "unknown"
	}
	if (b[0]>>2)&3 == 1 {
		a.Language = langCode(b[2:4])
	}
	return a
}

// SubpAttr describes a sub-picture stream.
type SubpAttr struct {
	Language  string `json:"language,omitempty"`
	Extension int    `json:"extension"`
}

func parseSubpAttr(b []byte) SubpAttr {
	a := SubpAttr{Extension: int(b[5])}
	if b[0]&3 == 1 {
		a.Language = langCode(b[2:4])
	}
	return a
}

// Cell is one cell of a PGC (cell playback info + position).
type Cell struct {
	BlockMode        int     `json:"blockMode"` // 0 normal, 1 first of angle block, 2 middle, 3 last
	BlockType        int     `json:"blockType"` // 1 = angle block
	Seamless         bool    `json:"seamless"`
	Interleaved      bool    `json:"interleaved"`
	STCDiscontinuity bool    `json:"stcDiscontinuity"`
	SeamlessAngle    bool    `json:"seamlessAngle"`
	Restricted       bool    `json:"restricted"`
	StillTime        int     `json:"stillTime"` // seconds, 0xff = infinite
	Command          int     `json:"command"`   // 1-based index into PGC cell commands, 0 none
	Duration         float64 `json:"durationSec"`
	FirstSector      uint32  `json:"firstSector"`
	FirstILVUEnd     uint32  `json:"firstIlvuEndSector"`
	LastVOBUStart    uint32  `json:"lastVobuStartSector"`
	LastSector       uint32  `json:"lastSector"`
	VOBID            int     `json:"vobId"`
	CellID           int     `json:"cellId"`
}

// SubpControl says which sub-picture stream to use per display mode.
type SubpControl struct {
	Narrow, Wide, Letterbox, PanScan int
}

// PGC is a program chain.
type PGC struct {
	Duration     float64          `json:"durationSec"`
	Programs     []int            `json:"programs"` // entry cell number (1-based) per program
	Cells        []Cell           `json:"cells"`
	AudioControl [8]int           `json:"audioControl"` // stream number or -1
	SubpControl  [32]*SubpControl `json:"-"`
	Palette      [16]uint32       `json:"palette"` // 0x00YYCrCb
	UOPs         uint32           `json:"uops"`
	NextPGCN     int              `json:"nextPgcn"`
	PrevPGCN     int              `json:"prevPgcn"`
	GoUpPGCN     int              `json:"goUpPgcn"`
	StillTime    int              `json:"stillTime"`
	PlaybackMode int              `json:"playbackMode"`
	Pre          []Command        `json:"-"`
	Post         []Command        `json:"-"`
	CellCmds     []Command        `json:"-"`
}

// Command is one 8-byte VM instruction.
type Command [8]byte

// PGCEntry is a PGCIT row: a PGC plus its category.
type PGCEntry struct {
	Entry       bool `json:"entry"`
	TitleNumber int  `json:"titleNumber"` // VTS title number (title domain)
	MenuType    int  `json:"menuType"`    // menu domain: 2 title,3 root,4 subp,5 audio,6 angle,7 chapter
	BlockMode   int  `json:"blockMode"`   // parental block: 1 first, 2 middle, 3 last
	BlockType   int  `json:"blockType"`   // 1 = parental block (alternative versions of a scene)
	// ParentalMask names the parental IDs this PGC is for; inside a parental
	// block the player plays the PGC whose mask matches PTL_MAIT for its level.
	ParentalMask int  `json:"parentalMask"`
	PGC          *PGC `json:"-"`
}

// LangUnit is a menu language unit.
type LangUnit struct {
	Language string     `json:"language"`
	Exists   int        `json:"exists"`
	PGCs     []PGCEntry `json:"pgcs"`
}

// TitleSearch is one TT_SRPT row.
type TitleSearch struct {
	Number    int `json:"number"`
	VTS       int `json:"vts"`
	VTSTitle  int `json:"vtsTitle"`
	Angles    int `json:"angles"`
	Chapters  int `json:"chapters"`
	Parental  int `json:"parental"`
	PlayType  int `json:"playType"`
	VTSSector uint32
}

// PTT is one chapter entry: PGC number and program number.
type PTT struct {
	PGCN int `json:"pgcn"`
	PGN  int `json:"pgn"`
}

// VMG is VIDEO_TS.IFO.
type VMG struct {
	Version      int
	RegionMask   int // bit set = region prohibited
	NumTitleSets int
	Provider     string
	FirstPlay    *PGC
	Titles       []TitleSearch
	Menus        []LangUnit
	MenuVideo    VideoAttr
	MenuAudio    []AudioAttr
	MenuSubp     []SubpAttr
	MenuVOBSec   uint32
	// MenuVOBSectors is the size of VIDEO_TS.VOB in sectors according to
	// the IFO (0 when the IFO does not say).
	MenuVOBSectors uint32
	// Parental is the parental management table (PTL_MAIT), per country.
	Parental []ParentalCountry
}

// ParentalCountry is one country's parental management masks: Masks[level-1]
// holds one parental ID mask for the VMG (index 0) and each title set.
type ParentalCountry struct {
	Country string
	Masks   [8][]uint16
}

// VTS is VTS_xx_0.IFO.
type VTS struct {
	Number      int
	Version     int
	Menus       []LangUnit
	PGCs        []PGCEntry
	PTT         [][]PTT // per VTS title (1-based title - 1)
	MenuVideo   VideoAttr
	MenuAudio   []AudioAttr
	MenuSubp    []SubpAttr
	Video       VideoAttr
	Audio       []AudioAttr
	Subp        []SubpAttr
	MenuVOBSec  uint32
	TitleVOBSec uint32
	// Sizes of the menu and title VOBs in sectors according to the IFO
	// (0 when unknown). Cells that start beyond them are junk.
	MenuVOBSectors  uint32
	TitleVOBSectors uint32
}

var ErrBadIFO = errors.New("dvd: malformed IFO")

type reader struct {
	b []byte
}

func (r reader) ok(off, n int) bool { return off >= 0 && n >= 0 && off+n <= len(r.b) }
func (r reader) u8(off int) int {
	if !r.ok(off, 1) {
		return 0
	}
	return int(r.b[off])
}
func (r reader) u16(off int) int {
	if !r.ok(off, 2) {
		return 0
	}
	return int(binary.BigEndian.Uint16(r.b[off:]))
}
func (r reader) u32(off int) uint32 {
	if !r.ok(off, 4) {
		return 0
	}
	return binary.BigEndian.Uint32(r.b[off:])
}
func (r reader) bytes(off, n int) []byte {
	if !r.ok(off, n) {
		return make([]byte, n)
	}
	return r.b[off : off+n]
}

func parsePGC(r reader, base int) (*PGC, error) {
	if !r.ok(base, 0xec) {
		return nil, fmt.Errorf("%w: PGC at %#x runs past end", ErrBadIFO, base)
	}
	p := &PGC{
		Duration:     dvdTime(r.bytes(base+4, 4)),
		UOPs:         r.u32(base + 8),
		NextPGCN:     r.u16(base + 0x9c),
		PrevPGCN:     r.u16(base + 0x9e),
		GoUpPGCN:     r.u16(base + 0xa0),
		StillTime:    r.u8(base + 0xa2),
		PlaybackMode: r.u8(base + 0xa3),
	}
	nPrograms := r.u8(base + 2)
	nCells := r.u8(base + 3)
	for i := 0; i < 8; i++ {
		v := r.u16(base + 0x0c + i*2)
		p.AudioControl[i] = -1
		if v&0x8000 != 0 {
			p.AudioControl[i] = (v >> 8) & 7
		}
	}
	for i := 0; i < 32; i++ {
		v := r.u32(base + 0x1c + i*4)
		if v&0x80000000 != 0 {
			p.SubpControl[i] = &SubpControl{Narrow: int(v>>24) & 31, Wide: int(v>>16) & 31, Letterbox: int(v>>8) & 31, PanScan: int(v) & 31}
		}
	}
	for i := 0; i < 16; i++ {
		p.Palette[i] = r.u32(base+0xa4+i*4) & 0xffffff
	}
	cmdOff := r.u16(base + 0xe4)
	progOff := r.u16(base + 0xe6)
	cellPbOff := r.u16(base + 0xe8)
	cellPosOff := r.u16(base + 0xea)

	if progOff != 0 {
		for i := 0; i < nPrograms; i++ {
			p.Programs = append(p.Programs, r.u8(base+progOff+i))
		}
	}
	if cellPbOff != 0 {
		for i := 0; i < nCells && i < maxCellsPerPGC; i++ {
			o := base + cellPbOff + i*24
			if !r.ok(o, 24) {
				return nil, fmt.Errorf("%w: cell playback table truncated", ErrBadIFO)
			}
			cat := r.u8(o)
			c := Cell{
				BlockMode:        (cat >> 6) & 3,
				BlockType:        (cat >> 4) & 3,
				Seamless:         cat&8 != 0,
				Interleaved:      cat&4 != 0,
				STCDiscontinuity: cat&2 != 0,
				SeamlessAngle:    cat&1 != 0,
				Restricted:       r.u8(o+1)&0x40 != 0,
				StillTime:        r.u8(o + 2),
				Command:          r.u8(o + 3),
				Duration:         dvdTime(r.bytes(o+4, 4)),
				FirstSector:      r.u32(o + 8),
				FirstILVUEnd:     r.u32(o + 12),
				LastVOBUStart:    r.u32(o + 16),
				LastSector:       r.u32(o + 20),
			}
			p.Cells = append(p.Cells, c)
		}
	}
	if cellPosOff != 0 {
		for i := range p.Cells {
			o := base + cellPosOff + i*4
			p.Cells[i].VOBID = r.u16(o)
			p.Cells[i].CellID = r.u8(o + 3)
		}
	}
	if cmdOff != 0 {
		c := base + cmdOff
		nPre, nPost, nCell := r.u16(c), r.u16(c+2), r.u16(c+4)
		take := func(first, n int) []Command {
			var out []Command
			for i := 0; i < n && i < maxCommands; i++ {
				o := c + 8 + (first+i)*8
				if !r.ok(o, 8) {
					break
				}
				var cmd Command
				copy(cmd[:], r.b[o:o+8])
				out = append(out, cmd)
			}
			return out
		}
		p.Pre = take(0, nPre)
		p.Post = take(nPre, nPost)
		p.CellCmds = take(nPre+nPost, nCell)
	}
	return p, nil
}

func parsePGCIT(r reader, base int) ([]PGCEntry, error) {
	if !r.ok(base, 8) {
		return nil, fmt.Errorf("%w: PGCIT outside file", ErrBadIFO)
	}
	n := r.u16(base)
	if n > maxPGCs {
		n = maxPGCs
	}
	out := make([]PGCEntry, 0, n)
	for i := 0; i < n; i++ {
		o := base + 8 + i*8
		cat := r.u32(o)
		at := base + int(r.u32(o+4))
		pgc, err := parsePGC(r, at)
		if err != nil {
			return nil, err
		}
		out = append(out, PGCEntry{
			Entry:        cat&0x80000000 != 0,
			TitleNumber:  int(cat>>24) & 0x7f,
			MenuType:     int(cat>>24) & 0x0f,
			BlockMode:    int(cat>>22) & 3,
			BlockType:    int(cat>>20) & 3,
			ParentalMask: int(cat & 0xffff),
			PGC:          pgc,
		})
	}
	return out, nil
}

func parsePGCIUT(r reader, base int) []LangUnit {
	if base == 0 || !r.ok(base, 8) {
		return nil
	}
	n := r.u16(base)
	var units []LangUnit
	for i := 0; i < n && i < maxLangUnits; i++ {
		o := base + 8 + i*8
		if !r.ok(o, 8) {
			break
		}
		lu := LangUnit{Language: langCode(r.bytes(o, 2)), Exists: r.u8(o + 3)}
		pgcs, err := parsePGCIT(r, base+int(r.u32(o+4)))
		if err == nil {
			lu.PGCs = pgcs
		}
		units = append(units, lu)
	}
	return units
}

// ParseVMG parses VIDEO_TS.IFO.
func ParseVMG(b []byte) (*VMG, error) {
	if len(b) < 0x200 || string(b[:12]) != "DVDVIDEO-VMG" {
		return nil, fmt.Errorf("%w: not a VMG", ErrBadIFO)
	}
	r := reader{b}
	v := &VMG{
		Version:      r.u8(0x20),
		RegionMask:   int(r.u32(0x22)>>16) & 0xff,
		NumTitleSets: r.u16(0x3e),
		Provider:     string(trimZero(r.bytes(0x40, 32))),
		MenuVOBSec:   r.u32(0xc0),
	}
	// The VMG is laid out as IFO, menu VOB, BUP: the menu VOB ends where the
	// backup copy of the IFO (as long as the IFO) begins.
	if last, ifoLast := r.u32(0x0c), r.u32(0x1c); v.MenuVOBSec != 0 && last > ifoLast+v.MenuVOBSec {
		v.MenuVOBSectors = last - ifoLast - v.MenuVOBSec
	}
	if sec := int(r.u32(0xcc)); sec != 0 {
		v.Parental = parsePTLMAIT(r, sec*Sector)
	}
	if fp := int(r.u32(0x84)); fp != 0 {
		if pgc, err := parsePGC(r, fp); err == nil {
			v.FirstPlay = pgc
		}
	}
	// TT_SRPT
	if sec := int(r.u32(0xc4)); sec != 0 {
		base := sec * Sector
		n := r.u16(base)
		for i := 0; i < n && i < 99; i++ {
			o := base + 8 + i*12
			if !r.ok(o, 12) {
				break
			}
			v.Titles = append(v.Titles, TitleSearch{
				Number:    i + 1,
				PlayType:  r.u8(o),
				Angles:    r.u8(o + 1),
				Chapters:  r.u16(o + 2),
				Parental:  r.u16(o + 4),
				VTS:       r.u8(o + 6),
				VTSTitle:  r.u8(o + 7),
				VTSSector: r.u32(o + 8),
			})
		}
	}
	if sec := int(r.u32(0xc8)); sec != 0 {
		v.Menus = parsePGCIUT(r, sec*Sector)
	}
	v.MenuVideo = parseVideoAttr(uint16(r.u16(0x100)))
	for i := 0; i < r.u16(0x102) && i < 1; i++ {
		v.MenuAudio = append(v.MenuAudio, parseAudioAttr(r.bytes(0x104+i*8, 8)))
	}
	for i := 0; i < r.u16(0x154) && i < 1; i++ {
		v.MenuSubp = append(v.MenuSubp, parseSubpAttr(r.bytes(0x156+i*6, 6)))
	}
	return v, nil
}

// ParseVTS parses VTS_xx_0.IFO.
func ParseVTS(b []byte, number int) (*VTS, error) {
	if len(b) < 0x300 || string(b[:12]) != "DVDVIDEO-VTS" {
		return nil, fmt.Errorf("%w: not a VTS", ErrBadIFO)
	}
	r := reader{b}
	v := &VTS{Number: number, Version: r.u8(0x20), MenuVOBSec: r.u32(0xc0), TitleVOBSec: r.u32(0xc4)}
	// A title set is laid out as IFO, menu VOB, title VOBs, BUP.
	if last, ifoLast := r.u32(0x0c), r.u32(0x1c); v.TitleVOBSec != 0 && last > ifoLast+v.TitleVOBSec {
		v.TitleVOBSectors = last - ifoLast - v.TitleVOBSec
		if v.MenuVOBSec != 0 && v.MenuVOBSec < v.TitleVOBSec {
			v.MenuVOBSectors = v.TitleVOBSec - v.MenuVOBSec
		}
	}
	// PTT_SRPT
	if sec := int(r.u32(0xc8)); sec != 0 {
		base := sec * Sector
		n := r.u16(base)
		last := base + int(r.u32(base+4)) + 1
		if last > len(b) {
			last = len(b)
		}
		offs := make([]int, 0, n)
		for i := 0; i < n && i < 99; i++ {
			offs = append(offs, base+int(r.u32(base+8+i*4)))
		}
		for i, start := range offs {
			end := last
			if i+1 < len(offs) {
				end = offs[i+1]
			}
			var list []PTT
			for o := start; o+4 <= end && len(list) < maxPTTEntries; o += 4 {
				list = append(list, PTT{PGCN: r.u16(o), PGN: r.u16(o + 2)})
			}
			v.PTT = append(v.PTT, list)
		}
	}
	if sec := int(r.u32(0xcc)); sec != 0 {
		pgcs, err := parsePGCIT(r, sec*Sector)
		if err != nil {
			return nil, err
		}
		v.PGCs = pgcs
	}
	if sec := int(r.u32(0xd0)); sec != 0 {
		v.Menus = parsePGCIUT(r, sec*Sector)
	}
	v.MenuVideo = parseVideoAttr(uint16(r.u16(0x100)))
	for i := 0; i < r.u16(0x102) && i < 1; i++ {
		v.MenuAudio = append(v.MenuAudio, parseAudioAttr(r.bytes(0x104+i*8, 8)))
	}
	for i := 0; i < r.u16(0x154) && i < 1; i++ {
		v.MenuSubp = append(v.MenuSubp, parseSubpAttr(r.bytes(0x156+i*6, 6)))
	}
	v.Video = parseVideoAttr(uint16(r.u16(0x200)))
	for i := 0; i < r.u16(0x202) && i < 8; i++ {
		v.Audio = append(v.Audio, parseAudioAttr(r.bytes(0x204+i*8, 8)))
	}
	for i := 0; i < r.u16(0x254) && i < 32; i++ {
		v.Subp = append(v.Subp, parseSubpAttr(r.bytes(0x256+i*6, 6)))
	}
	return v, nil
}

// parsePTLMAIT parses the VMG parental management table: per country, eight
// blocks (level 8 first, level 1 last) of one mask for the VMG and each title
// set.
func parsePTLMAIT(r reader, base int) []ParentalCountry {
	if !r.ok(base, 8) {
		return nil
	}
	countries, sets := r.u16(base), r.u16(base+2)
	if sets > maxTitleSets {
		return nil
	}
	var out []ParentalCountry
	for i := 0; i < countries && i < 256; i++ {
		e := base + 8 + i*8
		if !r.ok(e, 8) {
			break
		}
		pc := ParentalCountry{Country: strings.ToUpper(langCode(r.bytes(e, 2)))}
		at := base + r.u16(e+4)
		if !r.ok(at, 16*(sets+1)) {
			continue
		}
		for level := 8; level >= 1; level-- {
			blk := at + 2*(8-level)*(sets+1)
			masks := make([]uint16, sets+1)
			for k := range masks {
				masks[k] = uint16(r.u16(blk + 2*k))
			}
			pc.Masks[level-1] = masks
		}
		out = append(out, pc)
	}
	return out
}

func trimZero(b []byte) []byte {
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}

// MenuTypes maps menu names to the entry PGC menu type codes.
var MenuTypes = map[string]int{"title": 2, "root": 3, "subpicture": 4, "audio": 5, "angle": 6, "chapter": 7}

// Disc bundles the parsed IFOs of one DVD.
type Disc struct {
	VMG *VMG
	VTS map[int]*VTS
}

// vobSectors is the size in sectors of the VOB a domain plays from, as the
// IFO states it (0 when unknown).
func (d *Disc) vobSectors(domain Domain, vts int) uint32 {
	switch domain {
	case DomainFP, DomainVMGM:
		return d.VMG.MenuVOBSectors
	case DomainVTSM:
		if v := d.VTS[vts]; v != nil {
			return v.MenuVOBSectors
		}
	case DomainVTS:
		if v := d.VTS[vts]; v != nil {
			return v.TitleVOBSectors
		}
	}
	return 0
}

// ParentalMask is the parental ID mask PTL_MAIT gives title set vts for a
// country (SPRM12) and parental level (SPRM13). Without parental control
// (level 15) the player acts at level 8, which permits everything; a country
// the table does not list uses the first entry. 0 means the disc has no
// table.
func (d *Disc) ParentalMask(country uint16, level, vts int) uint16 {
	t := d.VMG.Parental
	if len(t) == 0 {
		return 0
	}
	if level < 1 || level > 8 {
		level = 8
	}
	pc := t[0]
	for _, c := range t {
		if len(c.Country) == 2 && code2(c.Country) == country {
			pc = c
			break
		}
	}
	m := pc.Masks[level-1]
	if vts < 0 || vts >= len(m) {
		return 0
	}
	return m[vts]
}

// Region returns the lowest region number the disc allows (1-8), or 1 if all-region.
func (d *Disc) Region() int {
	mask := d.VMG.RegionMask
	for i := 0; i < 8; i++ {
		if mask&(1<<i) == 0 {
			return i + 1
		}
	}
	return 1
}
