// Package dvd is what the server knows of a DVD: the shapes WittyDVD answers in. WittyDVD itself
// is a module, reached through a helper process (internal/modules/dvdremote). These travel as gob,
// which goes by field name, so a field a newer version adds is simply not read here.

package dvd

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
)

const Sector = 2048

// VideoAttr describes a video stream.
type VideoAttr struct {
	MPEG        int    `json:"mpeg"`
	Standard    string `json:"standard"` // ntsc | pal
	Wide        bool   `json:"wide"`     // 16:9
	Letterbox   bool   `json:"letterbox"`
	PictureSize int    `json:"pictureSize"`
	PermittedDF int    `json:"permittedDf"`
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

// SubpAttr describes a sub-picture stream.
type SubpAttr struct {
	Language  string `json:"language,omitempty"`
	Extension int    `json:"extension"`
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

// MenuTypes maps menu names to the entry PGC menu type codes.
var MenuTypes = map[string]int{"title": 2, "root": 3, "subpicture": 4, "audio": 5, "angle": 6, "chapter": 7}

// Disc bundles the parsed IFOs of one DVD.
type Disc struct {
	VMG *VMG
	VTS map[int]*VTS
}

// MenuButton is a button in frame coordinates with its highlight artwork.
type MenuButton struct {
	N       int     `json:"n"`
	X0      int     `json:"x0"`
	Y0      int     `json:"y0"`
	X1      int     `json:"x1"`
	Y1      int     `json:"y1"`
	Up      int     `json:"up"`
	Down    int     `json:"down"`
	Left    int     `json:"left"`
	Right   int     `json:"right"`
	Auto    bool    `json:"auto"`
	Cmd     Command `json:"-"`
	CmdText string  `json:"cmd"`
	Select  *RGBA   `json:"-"`
	Action  *RGBA   `json:"-"`
	Inert   bool    `json:"inert,omitempty"` // no visible highlight and no command
}

// MenuPicture is everything a client needs to draw and operate a menu.
type MenuPicture struct {
	FrameW, FrameH int
	Aspect         float64 // 4/3 or 16/9
	Buttons        []MenuButton
	DefaultButton  int
	Normal         *RGBA // sub-picture as shown with nothing selected
	ButtonsFromSec float64
	// ButtonsUntilSec is when the buttons stop working, measured like
	// ButtonsFromSec: the earlier of the highlight end (HLI_E_PTM) and the
	// end of button selection (BTN_SL_E_PTM). 0 = until the cell ends.
	ButtonsUntilSec float64
	HasHighlights   bool
	Scrambled       bool
	NoButtons       bool
	// ForcedSelect is the button the disc highlights when the menu appears
	// (FOSL_BTNN); it wins over SPRM8. 0 = none.
	ForcedSelect int
	// ForcedAction is the button the disc activates by itself when button
	// selection ends (FOAC_BTNN) at ActionAtSec into the cell. 0 = none.
	ForcedAction int
	ActionAtSec  float64
}

// MenuScanOptions tune ScanMenu.
type MenuScanOptions struct {
	Palette     [16]uint32
	Video       VideoAttr
	SubpControl *SubpControl // from the PGC, picks the SPU stream per display mode
	Wide        bool         // viewer wants 16:9 presentation
	NavButton   int          // SPRM8 highlight to prefer as default
	MaxHops     int
	ReadAhead   int // sectors to read once buttons are found
}

var ErrNoNav = errors.New("dvd: no NAV pack at cell start")

// StepKind is what the navigator wants shown.
type StepKind string

const (
	StepMenu  StepKind = "menu"
	StepTitle StepKind = "title"
	StepStop  StepKind = "stop"
)

// TitleInfo describes the title being played in a title step.
type TitleInfo struct {
	TTN      int     `json:"ttn"`
	VTSTTN   int     `json:"vtsTtn"`
	PTTN     int     `json:"pttn"`
	Cells    []Cell  `json:"-"`
	CellNums []int   `json:"cellNums"`
	Duration float64 `json:"durationSec"`
}

// Step is the navigator's instruction to the presentation layer.
type Step struct {
	Kind          StepKind   `json:"kind"`
	Domain        Domain     `json:"-"`
	DomainName    string     `json:"domain"`
	VTS           int        `json:"vts"`
	PGCN          int        `json:"pgcn"`
	CellN         int        `json:"cellN"`
	FirstSector   uint32     `json:"firstSector"`
	LastSector    uint32     `json:"lastSector"`
	StillSec      float64    `json:"stillSec"` // +Inf = wait for the viewer
	Loop          bool       `json:"loop"`
	Palette       [16]uint32 `json:"palette"`
	DefaultButton int        `json:"defaultButton"`
	Serial        int        `json:"serial"`
	Reason        string     `json:"reason,omitempty"`
	Hold          bool       `json:"hold,omitempty"`
	Resumed       bool       `json:"resumed,omitempty"`
	Title         *TitleInfo `json:"title,omitempty"`
	CellDuration  float64    `json:"cellDurationSec"`
}

// NavOptions configure a navigator.
type NavOptions struct {
	RegisterOptions
}

// angleCell maps cell n to the cell for the current angle inside an angle block.
func angleCell(p *PGC, n, angle int) int {
	if n < 1 || n > len(p.Cells) {
		return n
	}
	c := p.Cells[n-1]
	if c.BlockType != 1 || c.BlockMode != 1 {
		return n
	}
	k := n
	for a := 1; a < angle; a++ {
		if k >= len(p.Cells) || p.Cells[k-1].BlockMode == 3 {
			break
		}
		k++
	}
	return k
}

// nextCellN is the cell after n, skipping the rest of an angle block.
func nextCellN(p *PGC, n int) int {
	if n < 1 || n > len(p.Cells) {
		return n + 1
	}
	c := p.Cells[n-1]
	if c.BlockType != 1 {
		return n + 1
	}
	k := n
	for k <= len(p.Cells) && p.Cells[k-1].BlockType == 1 && p.Cells[k-1].BlockMode != 3 {
		k++
	}
	return k + 1
}

// programOf is the 1-based program that contains cell n.
func programOf(p *PGC, n int) int {
	pg := 1
	for i, entry := range p.Programs {
		if entry <= n {
			pg = i + 1
		}
	}
	return pg
}

const packStart = 0x000001ba

// Button is one highlight button from a PCI.
type Button struct {
	N           int     `json:"n"`
	Group       int     `json:"group"`
	X0, Y0      int     `json:"-"`
	X1, Y1      int     `json:"-"`
	Up, Down    int     `json:"-"`
	Left, Right int     `json:"-"`
	Auto        bool    `json:"auto"`
	Cmd         Command `json:"-"`
	ColorScheme int     `json:"-"`
	Select      Look    `json:"-"`
	Action      Look    `json:"-"`
}

// Look is a color+contrast table for pixel codes 0..3.
type Look struct {
	Color    [4]int
	Contrast [4]int
}

// PackScrambled reports CSS scrambling on the first PES of a pack.
func PackScrambled(pack []byte) bool {
	if len(pack) < 20 || binary.BigEndian.Uint32(pack) != packStart || pack[4]&0xc0 != 0x40 {
		return false
	}
	p := 14 + int(pack[13]&7)
	for p+9 <= len(pack) {
		if pack[p] != 0 || pack[p+1] != 0 || pack[p+2] != 1 {
			return false
		}
		id := pack[p+3]
		l := int(binary.BigEndian.Uint16(pack[p+4:]))
		if id == 0xbb || id == 0xbe || id == 0xbf { // system header, padding, nav: no scrambling flag
			p += 6 + l
			continue
		}
		return pack[p+6]&0x30 != 0
	}
	return false
}

// SPUUnit is a complete sub-picture unit with its presentation time.
type SPUUnit struct {
	Stream int
	Sector uint32
	PTS    int64
	Bytes  []byte
}

// Space names a VOB address space.
type Space struct {
	Domain Domain
	VTS    int
}

// FileExtent is a byte range in a named file.
type FileExtent struct {
	File   string `json:"file"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
}

// SpaceFor maps a step to its VOB address space.
func SpaceFor(domain Domain, vts int) Space {
	switch domain {
	case DomainFP, DomainVMGM:
		return Space{Domain: DomainVMGM}
	case DomainVTSM:
		return Space{Domain: DomainVTSM, VTS: vts}
	}
	return Space{Domain: DomainVTS, VTS: vts}
}

// TitleSummary is a flattened view of one title.
type TitleSummary struct {
	Number   int         `json:"number"`
	VTS      int         `json:"vts"`
	VTSTitle int         `json:"vtsTitle"`
	Duration float64     `json:"durationSec"`
	Chapters []float64   `json:"chapters"` // chapter start offsets in seconds
	Angles   int         `json:"angles"`
	Audio    []AudioAttr `json:"audio"`
	Subp     []SubpAttr  `json:"subtitles"`
	Video    VideoAttr   `json:"video"`
	Cells    []TitleCell `json:"-"`
}

// TitleCell is one cell of a flattened title with its PGC.
type TitleCell struct {
	PGCN  int
	CellN int
	Cell  Cell
	Start float64
}

// Titles flattens every title (following PGC chains in program order).
func (d *Disc) Titles() []TitleSummary {
	var out []TitleSummary
	for _, t := range d.VMG.Titles {
		if ts := d.Title(t.Number); ts != nil {
			out = append(out, *ts)
		}
	}
	return out
}

// Title flattens one title by number.
func (d *Disc) Title(number int) *TitleSummary { return d.TitleAngle(number, 1) }

// TitleAngle flattens a title for one camera angle (1-based); cells inside
// angle blocks are replaced by that angle's cell.
func (d *Disc) TitleAngle(number, angle int) *TitleSummary {
	if number < 1 || number > len(d.VMG.Titles) {
		return nil
	}
	t := d.VMG.Titles[number-1]
	v := d.VTS[t.VTS]
	if v == nil || t.VTSTitle < 1 || t.VTSTitle > len(v.PTT) {
		return nil
	}
	ts := &TitleSummary{Number: number, VTS: t.VTS, VTSTitle: t.VTSTitle, Angles: t.Angles, Audio: v.Audio, Subp: v.Subp, Video: v.Video}
	ptts := v.PTT[t.VTSTitle-1]
	if len(ptts) == 0 {
		return ts
	}
	// Walk PGCs starting from the first chapter's PGC, following next_pgcn.
	seen := map[int]bool{}
	pgcn := ptts[0].PGCN
	chapterAt := map[[2]int]bool{}
	for _, p := range ptts {
		chapterAt[[2]int{p.PGCN, p.PGN}] = true
	}
	for pgcn >= 1 && pgcn <= len(v.PGCs) && !seen[pgcn] {
		seen[pgcn] = true
		p := v.PGCs[pgcn-1].PGC
		for n := 1; n >= 1 && n <= len(p.Cells); n = nextCellN(p, n) {
			k := angleCell(p, n, angle)
			c := p.Cells[k-1]
			pg := programOf(p, n)
			if chapterAt[[2]int{pgcn, pg}] && (len(p.Programs) == 0 || p.Programs[pg-1] == n) {
				ts.Chapters = append(ts.Chapters, ts.Duration)
			}
			ts.Cells = append(ts.Cells, TitleCell{PGCN: pgcn, CellN: k, Cell: c, Start: ts.Duration})
			ts.Duration += c.Duration
		}
		if p.NextPGCN == 0 {
			break
		}
		pgcn = p.NextPGCN
	}
	return ts
}

// MainTitle guesses the feature: the title with the most distinct video.
// Structural copy protection adds decoy titles that replay the feature's
// cells many times over or in scrambled order, and cells that point outside
// the title VOB; counting each cell once, skipping junk cells and preferring
// the title that plays its cells in disc order picks the real one.
func (d *Disc) MainTitle() *TitleSummary {
	type cand struct {
		ts       TitleSummary
		distinct float64
		jumps    int
	}
	var cs []cand
	for _, ts := range d.Titles() {
		v := d.VTS[ts.VTS]
		c := cand{ts: ts}
		seen := map[[2]uint32]bool{}
		var prev uint32
		for i, tc := range ts.Cells {
			cell := tc.Cell
			if cell.LastSector < cell.FirstSector || (v != nil && v.TitleVOBSectors != 0 && cell.FirstSector >= v.TitleVOBSectors) {
				continue
			}
			k := [2]uint32{cell.FirstSector, cell.LastSector}
			if !seen[k] {
				seen[k] = true
				c.distinct += cell.Duration
			}
			if i > 0 && cell.FirstSector < prev {
				c.jumps++
			}
			prev = cell.FirstSector
		}
		cs = append(cs, c)
	}
	if len(cs) == 0 {
		return nil
	}
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		// Within a minute of each other, decoys and the feature tie on
		// content: the fewest jumps back on the disc wins.
		if math.Abs(a.distinct-b.distinct) > 60 {
			return a.distinct > b.distinct
		}
		if a.jumps != b.jumps {
			return a.jumps < b.jumps
		}
		return a.ts.Duration > b.ts.Duration
	})
	return &cs[0].ts
}

// TitleRun is a stretch of a title that can be read straight through.
type TitleRun struct {
	Extents []FileExtent
	// Duration is how long the run plays, in seconds.
	Duration float64
	// StartPTS is the presentation time of its first picture (90 kHz); 0 when
	// the disc doesn't say.
	StartPTS uint32
	// Lead is how long before its first picture the run's earliest packet lies,
	// in seconds: the sound stored ahead of the picture (see scanLead).
	Lead float64
	// FirstCell and Cells say which of the given cells it plays.
	FirstCell, Cells int
}

// RGBA is a premultiplied-free 8-bit RGBA bitmap.
type RGBA struct {
	X, Y          int
	Width, Height int
	Pix           []byte // 4 bytes per pixel
}

// Opaque reports whether any pixel is visible.
func (r *RGBA) Opaque() bool {
	if r == nil {
		return false
	}
	for i := 3; i < len(r.Pix); i += 4 {
		if r.Pix[i] != 0 {
			return true
		}
	}
	return false
}

// CellSubs are the sub-picture units of one cell, all streams.
type CellSubs struct {
	// BasePTM is the presentation time of the cell's first VOBU; unit PTS
	// minus BasePTM is the offset into the cell.
	BasePTM uint32
	Units   []SPUUnit
}

// Purpose names an audio stream's code extension (empty for plain audio).
func (a AudioAttr) Purpose() string {
	switch a.Extension {
	case 2:
		return "Visually impaired"
	case 3:
		return "Commentary"
	case 4:
		return "Commentary 2"
	}
	return ""
}

// Purpose names a sub-picture stream's code extension.
func (s SubpAttr) Purpose() string {
	switch s.Extension {
	case 2:
		return "Large"
	case 3:
		return "Children"
	case 5:
		return "Captions"
	case 6:
		return "Large captions"
	case 7:
		return "Children's captions"
	case 9:
		return "Forced"
	case 13:
		return "Commentary"
	case 14:
		return "Large commentary"
	case 15:
		return "Children's commentary"
	}
	return ""
}

// Register file of the DVD virtual machine.
type Registers struct {
	GPRM      [16]uint16
	Counter   [16]bool // GPRM counter mode (increments once per second)
	SPRM      [24]uint16
	Rand      func() float64
}

// SPRM indexes.
const (
	SprmMenuLang  = 0
	SprmAudio     = 1
	SprmSubp      = 2
	SprmAngle     = 3
	SprmTTN       = 4
	SprmVTSTTN    = 5
	SprmTTPGCN    = 6
	SprmPTTN      = 7
	SprmHLBTNN    = 8
	SprmNavTimer  = 9
	SprmNavPGCN   = 10
	SprmAudioMix  = 11
	SprmCountry   = 12
	SprmParental  = 13
	SprmVideoMode = 14
	SprmAudioCap  = 15
	SprmAudioLang = 16
	SprmAudioExt  = 17
	SprmSubpLang  = 18
	SprmSubpExt   = 19
	SprmRegion    = 20
)

// RegisterOptions seeds player preferences.
type RegisterOptions struct {
	MenuLanguage     string
	AudioLanguage    string
	SubtitleLanguage string
	Region           int    // 1-8
	Parental         int    // 1-8; 0 = no limit
	Country          string // two letters; "" = US
	Angle            int    // initial camera angle (0 = 1)
	Rand             func() float64
}

type operand struct {
	isReg bool
	reg   int
	val   uint16
}

func regOp(r byte) operand { return operand{isReg: true, reg: int(r)} }

func valOp(v uint16) operand { return operand{val: v} }

func validReg(o operand) bool {
	return !o.isReg || o.reg < 16 || (o.reg >= 0x80 && o.reg < 0x80+24)
}

// LinkOp enumerates link commands.
type LinkOp int

const (
	LinkNone LinkOp = iota
	LinkTopCell
	LinkNextCell
	LinkPrevCell
	LinkTopPG
	LinkNextPG
	LinkPrevPG
	LinkTopPGC
	LinkNextPGC
	LinkPrevPGC
	LinkGoUpPGC
	LinkTailPGC
	LinkRSM
	LinkPGCN
	LinkPTTN
	LinkPGN
	LinkCN
)

var linkSubset = map[int]LinkOp{
	0: LinkNone, 1: LinkTopCell, 2: LinkNextCell, 3: LinkPrevCell, 5: LinkTopPG, 6: LinkNextPG, 7: LinkPrevPG,
	9: LinkTopPGC, 10: LinkNextPGC, 11: LinkPrevPGC, 12: LinkGoUpPGC, 13: LinkTailPGC, 16: LinkRSM,
}

// Link is a decoded link target.
type Link struct {
	Op     LinkOp
	N      int // PGCN / PTTN / PGN / CN
	Button int // button to highlight (0 = unchanged)
}

// JumpOp enumerates jump/call commands.
type JumpOp int

const (
	JumpNone JumpOp = iota
	JumpExit
	JumpTT
	JumpVTSTT
	JumpVTSPTT
	JumpSS
	CallSS
)

// Domain of playback.
type Domain int

const (
	DomainFP Domain = iota
	DomainVMGM
	DomainVTSM
	DomainVTS
)

func (d Domain) String() string {
	return [...]string{"fp", "vmgm", "vtsm", "vts"}[d]
}

// Jump is a decoded jump/call target.
type Jump struct {
	Op      JumpOp
	TTN     int
	PTTN    int
	Domain  Domain
	Menu    int // menu type for JumpSS/CallSS
	VTS     int
	PGCN    int
	HasPGCN bool
	RsmCell int
}

type setOp int

type cmpOp int

const (
	cmpNone cmpOp = iota
	cmpBC
	cmpEQ
	cmpNE
	cmpGE
	cmpGT
	cmpLE
	cmpLT
)

type sysOp int

const (
	sysNone sysOp = iota
	sysSetSTN
	sysSetNVTMR
	sysSetGPRMMD
	sysSetAMXMD
	sysSetHLBTNN
)

type specialOp int

// when controls how compare, set and link interact (command types 4-6).
type when int

const (
	whenAll        when = iota // cmp gates everything
	whenLink                   // set always; cmp gates link
	whenSetAndLink             // cmp gates set+link
	whenSet                    // cmp gates set; link always
)

// Instr is a decoded VM instruction.
type Instr struct {
	Raw     Command
	Type    int
	Invalid string

	cmp   cmpOp
	cmpA  operand
	cmpB  operand
	when  when
	set   setOp
	setD  int
	setS  operand
	sys   sysOp
	sysA  operand // audio / src
	sysB  operand // subpicture
	sysC  operand // angle
	sysN  int     // pgcn / dst register
	sysCt bool    // counter mode
	sp    specialOp
	spLvl int
	spLin int
	link  *Link
	jump  *Jump
}

// Decode decodes one 8-byte command.
func Decode(c Command) Instr {
	b := c[:]
	in := Instr{Raw: c, Type: int(b[0] >> 5)}
	u16 := func(i int) uint16 { return uint16(b[i])<<8 | uint16(b[i+1]) }
	direct := (b[0]>>4)&1 == 1
	setField := int(b[0] & 0x0f)
	cmpDirect := b[1]>>7 == 1
	cop := cmpOp((b[1] >> 4) & 7)
	cmd4 := int(b[1] & 0x0f)
	bad := func(why string) Instr { in.Invalid = why; return in }
	cmpB := func(regIdx int, valIdx int) operand {
		if cmpDirect {
			return valOp(u16(valIdx))
		}
		return regOp(b[regIdx])
	}

	linkOf := func(cmd int) (*Link, string) {
		button := int(b[6] >> 2)
		switch cmd {
		case 1:
			code := int(b[7] & 0x1f)
			op, ok := linkSubset[code]
			if !ok {
				return nil, fmt.Sprintf("link subset %#x invalid", code)
			}
			if op == LinkNone {
				return nil, ""
			}
			return &Link{Op: op, Button: button}, ""
		case 4:
			return &Link{Op: LinkPGCN, N: int(u16(6) & 0x7fff)}, ""
		case 5:
			return &Link{Op: LinkPTTN, N: int(b[6]&3)<<8 | int(b[7]), Button: button}, ""
		case 6:
			return &Link{Op: LinkPGN, N: int(b[7] & 0x7f), Button: button}, ""
		case 7:
			return &Link{Op: LinkCN, N: int(b[7]), Button: button}, ""
		}
		return nil, fmt.Sprintf("link command %d invalid", cmd)
	}
	jumpOf := func(cmd int) (*Jump, string) {
		sel := b[5] >> 6
		menu := int(b[5] & 0x0f)
		// Title, program and PGC numbers are 7 and 15 bits wide; the top bit is
		// reserved and some discs leave junk in it.
		pgcn := int(u16(2) & 0x7fff)
		ttn := int(b[5] & 0x7f)
		switch cmd {
		case 1:
			return &Jump{Op: JumpExit}, ""
		case 2:
			return &Jump{Op: JumpTT, TTN: ttn}, ""
		case 3:
			return &Jump{Op: JumpVTSTT, TTN: ttn}, ""
		case 5:
			return &Jump{Op: JumpVTSPTT, TTN: ttn, PTTN: int(b[2]&3)<<8 | int(b[3])}, ""
		case 6, 8:
			j := &Jump{Op: JumpSS}
			if cmd == 8 {
				j.Op = CallSS
				j.RsmCell = int(b[4])
			}
			switch sel {
			case 0:
				j.Domain = DomainFP
			case 1:
				j.Domain, j.Menu = DomainVMGM, menu
			case 2:
				j.Domain, j.Menu = DomainVTSM, menu
				if cmd == 6 {
					j.VTS, j.TTN = int(b[4]), int(b[3]&0x7f)
				}
			default:
				j.Domain, j.PGCN, j.HasPGCN = DomainVMGM, pgcn, true
			}
			return j, ""
		}
		return nil, fmt.Sprintf("jump command %d invalid", cmd)
	}

	switch {
	case in.Type == 0:
		if cmd4 > 3 {
			return bad("special command invalid")
		}
		if cop != cmpNone {
			in.cmp, in.cmpA, in.cmpB = cop, regOp(b[3]), cmpB(5, 4)
		}
		in.sp = specialOp(cmd4 + 1)
		in.spLvl, in.spLin = int(b[6]&0x0f), int(b[7])
	case in.Type == 1 && !direct:
		if cop != cmpNone {
			in.cmp, in.cmpA, in.cmpB = cop, regOp(b[3]), cmpB(5, 4)
		}
		if cmd4 != 0 {
			l, why := linkOf(cmd4)
			if why != "" {
				return bad(why)
			}
			in.link = l
		}
	case in.Type == 1:
		if cop != cmpNone {
			in.cmp, in.cmpA, in.cmpB = cop, regOp(b[6]), regOp(b[7])
		}
		if cmd4 != 0 {
			j, why := jumpOf(cmd4)
			if why != "" {
				return bad(why)
			}
			in.jump = j
		}
	case in.Type == 2 || in.Type == 3:
		if cop != cmpNone && cmd4 != 0 {
			return bad("compare and link together are illegal")
		}
		if cop == cmpNone && cmd4 != 0 {
			l, why := linkOf(cmd4)
			if why != "" {
				return bad(why)
			}
			in.link = l
		}
		if in.Type == 2 {
			if cop != cmpNone {
				in.cmp, in.cmpA, in.cmpB = cop, regOp(b[6]), regOp(b[7])
			}
			pick := func(x byte) (operand, bool) {
				if x&0x80 == 0 {
					return operand{}, false
				}
				if direct {
					return valOp(uint16(x & 0x7f)), true
				}
				return regOp(x & 0x0f), true
			}
			src := func(hi, lo int) operand {
				if direct {
					return valOp(u16(hi))
				}
				return regOp(b[lo] & 0x0f)
			}
			switch setField {
			case 0:
			case 1:
				in.sys = sysSetSTN
				var ok bool
				if in.sysA, ok = pick(b[3]); !ok {
					in.sysA = operand{reg: -1, isReg: true}
				}
				if in.sysB, ok = pick(b[4]); !ok {
					in.sysB = operand{reg: -1, isReg: true}
				}
				if in.sysC, ok = pick(b[5]); !ok {
					in.sysC = operand{reg: -1, isReg: true}
				}
			case 2:
				in.sys, in.sysN = sysSetNVTMR, int(u16(4))
				if direct {
					in.sysA = valOp(u16(2))
				} else {
					in.sysA = regOp(b[3])
				}
			case 3:
				in.sys, in.sysN, in.sysCt = sysSetGPRMMD, int(b[5]&0x0f), b[5]&0x80 != 0
				if direct {
					in.sysA = valOp(u16(2))
				} else {
					in.sysA = regOp(b[3])
				}
			case 4:
				in.sys, in.sysA = sysSetAMXMD, src(4, 5)
			case 6:
				in.sys, in.sysA = sysSetHLBTNN, src(4, 5)
			default:
				return bad("SetSystem operation invalid")
			}
		} else {
			if setField > 11 {
				return bad("set operation invalid")
			}
			if cop != cmpNone {
				in.cmp, in.cmpA, in.cmpB = cop, regOp(b[2]), cmpB(7, 6)
			}
			if setField != 0 {
				in.set, in.setD = setOp(setField), int(b[3]&0x0f)
				if direct {
					in.setS = valOp(u16(4))
				} else {
					in.setS = regOp(b[5])
				}
			}
		}
	case in.Type >= 4 && in.Type <= 6:
		if setField > 11 {
			return bad("set operation invalid")
		}
		if in.Type != 4 && direct && cmpDirect {
			return bad("immediate set with immediate compare is illegal")
		}
		dst := int(b[1] & 0x0f)
		l, why := linkOf(1)
		if why != "" {
			return bad(why)
		}
		in.link = l
		if in.Type == 4 {
			if setField != 0 {
				in.set, in.setD = setOp(setField), dst
				if direct {
					in.setS = valOp(u16(2))
				} else {
					in.setS = regOp(b[3])
				}
			}
			if cop != cmpNone {
				in.cmp, in.cmpA, in.cmpB = cop, regOp(byte(dst)), cmpB(5, 4)
			}
			in.when = whenLink
		} else {
			if setField != 0 {
				in.set, in.setD = setOp(setField), dst
				if direct {
					in.setS = valOp(u16(2))
				} else {
					in.setS = regOp(b[2])
				}
			}
			if cop != cmpNone {
				a := b[3]
				if direct {
					a = b[4]
				}
				in.cmp, in.cmpA, in.cmpB = cop, regOp(a), cmpB(5, 4)
			}
			if in.Type == 5 {
				in.when = whenSetAndLink
			} else {
				in.when = whenSet
			}
		}
	default:
		return bad("command type 7 unknown")
	}
	for _, o := range []operand{in.cmpA, in.cmpB, in.setS, in.sysA, in.sysB, in.sysC} {
		if o.isReg && o.reg >= 0 && !validReg(o) {
			return bad(fmt.Sprintf("register %#x does not exist", o.reg))
		}
	}
	return in
}

// Decoded instructions are cached: a disc only has a few thousand distinct
// commands but the navigator re-runs them constantly (every cell, button and
// loop check), and decoding allocates. Decoded instructions are immutable.
var decoded = struct {
	sync.RWMutex
	m map[Command]*Instr
}{m: map[Command]*Instr{}}

func decodeCached(c Command) *Instr {
	decoded.RLock()
	in := decoded.m[c]
	decoded.RUnlock()
	if in != nil {
		return in
	}
	d := Decode(c)
	decoded.Lock()
	if len(decoded.m) >= 1<<14 {
		decoded.m = map[Command]*Instr{}
	}
	decoded.m[c] = &d
	decoded.Unlock()
	return &d
}

// SetsHighlight reports whether the PGC's pre-commands select a button
// (SetHL_BTNN) before its cells play, the mark of an interactive screen
// inside a title (voting, games). Links that merely carry a button number
// for the menu the title returns to (common at the end of features) don't
// count.
func (p *PGC) SetsHighlight() bool {
	for _, c := range p.Pre {
		if decodeCached(c).sys == sysSetHLBTNN {
			return true
		}
	}
	return false
}
