package wittybluray

import (
	"fmt"
	"math/rand/v2"
)

// The HDMV virtual machine: the registers a Blu-ray player keeps and the
// interpreter for the navigation commands in movie objects and menu
// buttons. Written from the public description of the HDMV command set;
// it shares no code with other players.
//
// A command is 12 bytes: an operation word, a destination operand and a
// source operand. Operands are immediate values or register numbers: a
// general-purpose register (0..4095) or, with the top bit set, a player
// status register (0..127).

// Registers: 4096 general-purpose registers the disc may set freely, and
// 128 player status registers (PSRs) that describe the player and the
// playback state, which the disc reads and the player and SET_SYSTEM
// commands write.
type Registers struct {
	GPR [4096]uint32
	PSR [128]uint32
}

// Player status registers the engine uses.
const (
	PSRIGStream       = 0  // selected IG (menu) stream, 1-based
	PSRAudio          = 1  // selected primary audio stream, 1-based
	PSRPG             = 2  // bits 0..11: PG/text subtitle stream, 1-based; bit 31: subtitles shown
	PSRAngle          = 3  // selected angle, 1-based
	PSRTitle          = 4  // current title, 1-based (0xffff while first play runs)
	PSRChapter        = 5  // current chapter, 1-based (0xffff when unknown)
	PSRPlaylist       = 6  // current playlist number
	PSRPlayItem       = 7  // current play item
	PSRTime           = 8  // presentation time, 45 kHz
	PSRNavTimer       = 9  // navigation timer, seconds
	PSRButton         = 10 // selected button id
	PSRPage           = 11 // current menu page id
	PSRStyle          = 12 // user style number
	PSRParental       = 13 // parental level
	PSRSecondary      = 14 // secondary audio/video selection
	PSRVideoCap       = 15 // player video capability
	PSRAudioLang      = 16 // preferred audio language, three letters packed
	PSRPGLang         = 17 // preferred subtitle language
	PSRMenuLang       = 18 // preferred menu language
	PSRCountry        = 19 // country code
	PSRRegion         = 20 // region: 1 = A, 2 = B, 4 = C
	PSROutputPref     = 21 // bit 0: the user prefers 3D output
	PSRStereoStatus   = 22 // bit 0: 3D output is on now
	PSRDisplayCap     = 23
	PSR3DCap          = 24
	PSRProfile        = 29 // player profile (bit 0: BD-ROM, bit 4: 3D, ...)
	PSRVersion        = 30
	PSRProfileVer     = 31
	PSRBackupTitle    = 36
	PSRBackupChapter  = 37
	PSRBackupPlaylist = 38
	PSRBackupItem     = 39
	PSRBackupTime     = 40
	PSRBackupButton   = 42
	PSRBackupPage     = 43
	PSRBackupStyle    = 44
)

// RegisterOptions set the player's identity: languages, region, 3D.
type RegisterOptions struct {
	AudioLang, SubtitleLang, MenuLang string // three-letter codes, e.g. "eng"
	Country                           string // two letters, e.g. "US"
	Region                            int    // 1 = A, 2 = B, 4 = C; 0 = A
	Parental                          int    // 0 = no limit
	Stereo3D                          bool   // the display can show 3D
}

// NewRegisters makes a player's register file for the options.
func NewRegisters(o RegisterOptions) *Registers {
	r := &Registers{}
	r.PSR[PSRIGStream] = 1
	r.PSR[PSRAudio] = 1
	r.PSR[PSRPG] = 1 | 1<<31
	r.PSR[PSRAngle] = 1
	r.PSR[PSRTitle] = 0xffff
	r.PSR[PSRChapter] = 0xffff
	r.PSR[PSRAudioLang] = packLang(orDefault(o.AudioLang, "eng"))
	r.PSR[PSRPGLang] = packLang(orDefault(o.SubtitleLang, "eng"))
	r.PSR[PSRMenuLang] = packLang(orDefault(o.MenuLang, "eng"))
	c := orDefault(o.Country, "US")
	r.PSR[PSRCountry] = uint32(c[0])<<8 | uint32(c[1])
	r.PSR[PSRRegion] = uint32(orDefaultInt(o.Region, 1))
	r.PSR[PSRParental] = uint32(orDefaultInt(o.Parental, 255))
	// Capabilities: a profile 5 (3D) player when the display can show 3D,
	// else profile 2; 1080p; every common audio and subtitle format.
	r.PSR[PSRVideoCap] = 0x1ff
	r.PSR[PSRDisplayCap] = 0x1
	r.PSR[PSRProfile] = 0x03
	r.PSR[PSRVersion] = 0x0240
	r.PSR[PSRProfileVer] = 0x02<<16 | 0x0240
	if o.Stereo3D {
		r.PSR[PSROutputPref] = 1
		r.PSR[PSR3DCap] = 1
		r.PSR[PSRProfile] |= 1 << 4
		r.PSR[PSRProfileVer] = 0x05<<16 | 0x0240
	}
	return r
}

// Clone copies the registers.
func (r *Registers) Clone() *Registers { c := *r; return &c }

func packLang(s string) uint32 {
	if len(s) < 3 {
		return 0
	}
	return uint32(s[0])<<16 | uint32(s[1])<<8 | uint32(s[2])
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func orDefaultInt(v, d int) int {
	if v == 0 {
		return d
	}
	return v
}

// ---------------------------------------------------------------- decoding

// Group is a command's group.
type Group int

const (
	GroupBranch Group = iota
	GroupCompare
	GroupSet
)

// Instr is a decoded command.
type Instr struct {
	Raw      Command
	Group    Group
	Sub      int // branch: 0 goto, 1 jump, 2 play; set: 0 set, 1 set system
	Opt      int // branch option, compare option or set option
	Operands int
	ImmDst   bool
	ImmSrc   bool
	Dst, Src uint32
}

// Decode decodes a command.
func Decode(c Command) Instr {
	w := c.Op
	in := Instr{Raw: c, Operands: int(w >> 29), Group: Group((w >> 27) & 3), Sub: int((w >> 24) & 7), ImmDst: w&(1<<23) != 0, ImmSrc: w&(1<<22) != 0, Dst: c.Dst, Src: c.Src}
	switch in.Group {
	case GroupBranch:
		in.Opt = int((w >> 16) & 15)
	case GroupCompare:
		in.Opt = int((w >> 8) & 15)
	case GroupSet:
		in.Opt = int(w & 15)
	}
	return in
}

// Name is the command's mnemonic.
func (in Instr) Name() string {
	switch in.Group {
	case GroupBranch:
		switch in.Sub {
		case 0:
			return pick(in.Opt, "NOP", "GOTO", "BREAK")
		case 1:
			return pick(in.Opt, "JUMP_OBJECT", "JUMP_TITLE", "CALL_OBJECT", "CALL_TITLE", "RESUME")
		case 2:
			return pick(in.Opt, "PLAY_PL", "PLAY_PL_PI", "PLAY_PL_PM", "TERMINATE_PL", "LINK_PI", "LINK_MK")
		}
	case GroupCompare:
		return pick(in.Opt, "?", "BC", "EQ", "NE", "GE", "GT", "LE", "LT")
	case GroupSet:
		switch in.Sub {
		case 0:
			return pick(in.Opt, "?", "MOVE", "SWAP", "ADD", "SUB", "MUL", "DIV", "MOD", "RND", "AND", "OR", "XOR", "BITSET", "BITCLR", "SHL", "SHR")
		case 1:
			switch in.Opt {
			case 0x10:
				return "SETSYSTEM_0x10"
			}
			return pick(in.Opt, "?", "SET_STREAM", "SET_NV_TIMER", "SET_BUTTON_PAGE", "ENABLE_BUTTON", "DISABLE_BUTTON", "SET_SEC_STREAM", "POPUP_OFF", "STILL_ON", "STILL_OFF", "SET_OUTPUT_MODE", "SET_STREAM_SS")
		}
	}
	return fmt.Sprintf("op%08x", in.Raw.Op)
}

func pick(i int, names ...string) string {
	if i >= 0 && i < len(names) {
		return names[i]
	}
	return fmt.Sprintf("?%d", i)
}

func operand(v uint32, imm bool) string {
	if imm {
		return fmt.Sprintf("%d", v)
	}
	if v&0x80000000 != 0 {
		return fmt.Sprintf("PSR%d", v&0x7f)
	}
	return fmt.Sprintf("GPR%d", v&0xfff)
}

// String is a readable form, e.g. "EQ GPR4094, 3".
func (in Instr) String() string {
	s := in.Name()
	switch in.Operands {
	case 1:
		s += " " + operand(in.Dst, in.ImmDst)
	case 2:
		s += " " + operand(in.Dst, in.ImmDst) + ", " + operand(in.Src, in.ImmSrc)
	}
	return s
}

// ---------------------------------------------------------------- running

// ResultKind says why a run of commands stopped.
type ResultKind int

const (
	ResultEnd        ResultKind = iota // ran off the end (or BREAK)
	ResultJumpObject                   // run another movie object
	ResultJumpTitle                    // go to a title (0 = top menu, 0xffff = first play)
	ResultCallObject                   // run another movie object, resuming here after
	ResultCallTitle                    // a title, resuming here after
	ResultResume                       // back to what was suspended by a call
	ResultPlay                         // play a playlist (Playlist; Item or Mark when set)
	ResultTerminate                    // stop the playing playlist
	ResultLinkItem                     // jump within the playing playlist to a play item
	ResultLinkMark                     // jump within the playing playlist to a mark
	ResultError
)

// Result is what a run of commands asks the player to do.
type Result struct {
	Kind     ResultKind
	Object   int // JUMP/CALL_OBJECT
	Title    int // JUMP/CALL_TITLE
	Playlist int
	Item     int // PLAY_PL_PI, LINK_PI; -1 otherwise
	Mark     int // PLAY_PL_PM, LINK_MK; -1 otherwise
	PC       int // the command after the one that stopped the run
	Err      error
}

// Effects collects what SET_SYSTEM commands asked for during a run; the
// player applies them.
type Effects struct {
	NavTimer      *NavTimer
	ButtonPage    *ButtonPage
	EnableButton  []uint32
	DisableButton []uint32
	PopupOff      bool
	Still         *bool
	OutputMode3D  *bool
	StreamChange  bool // audio, subtitle, IG or angle registers changed
}

// NavTimer is SET_NV_TIMER: run a movie object after some seconds.
type NavTimer struct {
	Seconds uint32
	Object  int
}

// ButtonPage is SET_BUTTON_PAGE: change the menu's page and/or button.
type ButtonPage struct {
	Button, Page  uint32
	SetButton     bool
	SetPage       bool
	PageEffectOff bool
}

// RunOptions limit a run.
type RunOptions struct {
	MaxSteps int // 0 = 100000; a disc's program cannot loop for ever
}

// Run executes commands from pc until one changes the player's course, or
// the end. Compare commands that are false skip the next command.
func (r *Registers) Run(cmds []Command, pc int, o RunOptions, fx *Effects) Result {
	max := o.MaxSteps
	if max <= 0 {
		max = 100000
	}
	for steps := 0; ; steps++ {
		if pc < 0 || pc >= len(cmds) {
			return Result{Kind: ResultEnd, PC: pc, Item: -1, Mark: -1}
		}
		if steps > max {
			return Result{Kind: ResultError, PC: pc, Err: fmt.Errorf("the disc's program ran %d commands without playing anything", max)}
		}
		in := Decode(cmds[pc])
		dst, src := in.Dst, in.Src
		if !in.ImmDst {
			dst = r.get(in.Dst)
		}
		if !in.ImmSrc {
			src = r.get(in.Src)
		}
		next := pc + 1
		switch in.Group {
		case GroupBranch:
			switch in.Sub {
			case 0:
				switch in.Opt {
				case 1: // GOTO
					next = int(dst)
				case 2: // BREAK
					return Result{Kind: ResultEnd, PC: len(cmds), Item: -1, Mark: -1}
				}
			case 1:
				switch in.Opt {
				case 0:
					return Result{Kind: ResultJumpObject, Object: int(dst), PC: next, Item: -1, Mark: -1}
				case 1:
					return Result{Kind: ResultJumpTitle, Title: int(dst), PC: next, Item: -1, Mark: -1}
				case 2:
					return Result{Kind: ResultCallObject, Object: int(dst), PC: next, Item: -1, Mark: -1}
				case 3:
					return Result{Kind: ResultCallTitle, Title: int(dst), PC: next, Item: -1, Mark: -1}
				case 4:
					return Result{Kind: ResultResume, PC: next, Item: -1, Mark: -1}
				}
			case 2:
				switch in.Opt {
				case 0:
					return Result{Kind: ResultPlay, Playlist: int(dst), Item: -1, Mark: -1, PC: next}
				case 1:
					return Result{Kind: ResultPlay, Playlist: int(dst), Item: int(src), Mark: -1, PC: next}
				case 2:
					return Result{Kind: ResultPlay, Playlist: int(dst), Item: -1, Mark: int(src), PC: next}
				case 3:
					return Result{Kind: ResultTerminate, PC: next, Item: -1, Mark: -1}
				case 4:
					return Result{Kind: ResultLinkItem, Item: int(dst), Mark: -1, PC: next}
				case 5:
					return Result{Kind: ResultLinkMark, Mark: int(dst), Item: -1, PC: next}
				}
			}
		case GroupCompare:
			var ok bool
			switch in.Opt {
			case 1:
				ok = dst&src != 0
			case 2:
				ok = dst == src
			case 3:
				ok = dst != src
			case 4:
				ok = dst >= src
			case 5:
				ok = dst > src
			case 6:
				ok = dst <= src
			case 7:
				ok = dst < src
			default:
				ok = true
			}
			if !ok {
				next = pc + 2
			}
		case GroupSet:
			switch in.Sub {
			case 0:
				if in.ImmDst {
					break // the destination must be a register
				}
				var v uint32
				switch in.Opt {
				case 1:
					v = src
				case 2: // SWAP
					if !in.ImmSrc {
						r.set(in.Src, dst)
					}
					v = src
				case 3:
					v = dst + src
				case 4:
					v = dst - src
				case 5:
					v = dst * src
				case 6:
					if src != 0 {
						v = dst / src
					} else {
						v = 0xffffffff
					}
				case 7:
					if src != 0 {
						v = dst % src
					} else {
						v = 0xffffffff
					}
				case 8:
					if src > 0 {
						v = rand.Uint32N(src) + 1
					} else {
						v = 1
					}
				case 9:
					v = dst & src
				case 10:
					v = dst | src
				case 11:
					v = dst ^ src
				case 12:
					v = dst | 1<<(src&31)
				case 13:
					v = dst &^ (1 << (src & 31))
				case 14:
					v = dst << (src & 31)
				case 15:
					v = dst >> (src & 31)
				default:
					v = dst
				}
				r.set(in.Dst, v)
			case 1:
				r.setSystem(in.Opt, dst, src, fx)
			}
		}
		pc = next
	}
}

// get reads an operand register. PSRs the disc may not see read as zero.
func (r *Registers) get(ref uint32) uint32 {
	if ref&0x80000000 != 0 {
		return r.PSR[ref&0x7f]
	}
	return r.GPR[ref&0xfff]
}

// set writes a general-purpose register; player status registers can only
// change through SET_SYSTEM commands, so writes to them are ignored.
func (r *Registers) set(ref, v uint32) {
	if ref&0x80000000 != 0 {
		return
	}
	r.GPR[ref&0xfff] = v
}

// setSystem applies a SET_SYSTEM command: the stream registers it may
// change directly, and effects the player carries out.
func (r *Registers) setSystem(opt int, dst, src uint32, fx *Effects) {
	if fx == nil {
		fx = &Effects{}
	}
	switch opt {
	case 1: // SET_STREAM: dst = audio (bit 31 valid, bits 16..27) and PG (bit 15 valid, bits 0..11, bit 14 shown); src = IG (bit 31 valid, bits 16..23) and angle (bit 15 valid, bits 0..7)
		if dst&0x80000000 != 0 {
			r.PSR[PSRAudio] = (dst >> 16) & 0xfff
		}
		if dst&0x8000 != 0 {
			r.PSR[PSRPG] = r.PSR[PSRPG]&^0xfff | dst&0xfff
		}
		if dst&0x4000 != 0 {
			r.PSR[PSRPG] |= 1 << 31
		} else if dst&0x8000 != 0 {
			r.PSR[PSRPG] &^= 1 << 31
		}
		if src&0x80000000 != 0 {
			r.PSR[PSRIGStream] = (src >> 16) & 0xff
		}
		if src&0x8000 != 0 {
			r.PSR[PSRAngle] = src & 0xff
		}
		fx.StreamChange = true
	case 2: // SET_NV_TIMER
		fx.NavTimer = &NavTimer{Seconds: dst, Object: int(src)}
	case 3: // SET_BUTTON_PAGE: dst = button (bit 31 valid, bits 0..15); src = page (bit 31 valid, bit 30 page effect off, bits 0..7)
		bp := &ButtonPage{}
		if dst&0x80000000 != 0 {
			bp.SetButton, bp.Button = true, dst&0xffff
		}
		if src&0x80000000 != 0 {
			bp.SetPage, bp.Page = true, src&0xff
			bp.PageEffectOff = src&0x40000000 != 0
		}
		fx.ButtonPage = bp
	case 4:
		fx.EnableButton = append(fx.EnableButton, dst)
	case 5:
		fx.DisableButton = append(fx.DisableButton, dst)
	case 6: // SET_SEC_STREAM
		r.PSR[PSRSecondary] = dst
		fx.StreamChange = true
	case 7:
		fx.PopupOff = true
	case 8:
		on := true
		fx.Still = &on
	case 9:
		off := false
		fx.Still = &off
	case 10: // SET_OUTPUT_MODE: dst bit 0 asks for 3D
		want := dst&1 != 0 && r.PSR[PSR3DCap]&1 != 0
		r.PSR[PSRStereoStatus] = r.PSR[PSRStereoStatus]&^1 | b2u(want)
		fx.OutputMode3D = &want
	case 11: // SET_STREAM_SS: as SET_STREAM, for 3D (stereoscopic) stream numbers
		if dst&0x80000000 != 0 {
			r.PSR[PSRAudio] = (dst >> 16) & 0xfff
		}
		if dst&0x8000 != 0 {
			r.PSR[PSRPG] = r.PSR[PSRPG]&^0xfff | dst&0xfff
		}
		if src&0x80000000 != 0 {
			r.PSR[PSRIGStream] = (src >> 16) & 0xff
		}
		fx.StreamChange = true
	}
}

func b2u(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}
