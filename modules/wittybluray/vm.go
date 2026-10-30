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
	// Rand returns a number in [0,1) for RND; tests and diagnostic tools set
	// it to make a run repeatable. nil uses the system generator.
	Rand func() float64
}

// Player status registers the engine uses.
const (
	PSRIGStream       = 0  // selected IG (menu) stream, 1-based
	PSRAudio          = 1  // selected primary audio stream, 1-based (0xff = none chosen yet)
	PSRPG             = 2  // bits 0..11: PG/text subtitle stream, 1-based (0xfff = none); bits 16..27: PiP subtitle stream; bit 31: subtitles shown; bit 30: PiP subtitles shown
	PSRAngle          = 3  // selected angle, 1-based
	PSRTitle          = 4  // current title, 1-based (0xffff while first play runs)
	PSRChapter        = 5  // current chapter, 1-based (0xffff when unknown)
	PSRPlaylist       = 6  // current playlist number
	PSRPlayItem       = 7  // current play item
	PSRTime           = 8  // presentation time, 45 kHz
	PSRNavTimer       = 9  // navigation timer, seconds
	PSRButton         = 10 // selected button id (0xffff = none)
	PSRPage           = 11 // current menu page id
	PSRStyle          = 12 // user style number
	PSRParental       = 13 // parental level (the viewer's age; 255 = no limit)
	PSRSecondary      = 14 // bits 0..7: secondary audio; bits 8..15: secondary video; bits 30, 31: shown
	PSRAudioCap       = 15 // player audio capability
	PSRAudioLang      = 16 // preferred audio language, three letters packed
	PSRPGLang         = 17 // preferred subtitle language
	PSRMenuLang       = 18 // preferred menu language
	PSRCountry        = 19 // country code
	PSRRegion         = 20 // region: 1 = A, 2 = B, 4 = C
	PSROutputPref     = 21 // bit 0: the user prefers 3D output
	PSRStereoStatus   = 22 // bit 0: 3D output is on now
	PSRDisplayCap     = 23 // the display's 3D abilities and width
	PSR3DCap          = 24
	PSRUHDCap         = 25
	PSRUHDDisplayCap  = 26
	PSRHDRPref        = 27
	PSRSDRConv        = 28
	PSRVideoCap       = 29 // bit 0: secondary video in HD; bit 1: 25 and 50 Hz video
	PSRTextCap        = 30 // text subtitle abilities
	PSRProfile        = 31 // player profile (bits 16..23) and version (bits 0..15)
	PSRBackupTitle    = 36
	PSRBackupChapter  = 37
	PSRBackupPlaylist = 38
	PSRBackupItem     = 39
	PSRBackupTime     = 40
	PSRBackupButton   = 42
	PSRBackupPage     = 43
	PSRBackupStyle    = 44
)

// Player profiles as PSR31 carries them. A disc's program reads this to
// decide what to offer: 3D, BD-Live, secondary video.
const (
	Profile1v10 = 0x00000100 // the first players
	Profile1v11 = 0x00010110 // Bonus View: secondary streams and local storage
	Profile2v20 = 0x00030200 // BD-Live: network access
	Profile3v20 = 0x00080200 // audio-only players
	Profile5v24 = 0x00130240 // 3D
	Profile6v30 = 0x00000300 // Ultra HD
)

// RegisterOptions set the player's identity: languages, region, 3D.
type RegisterOptions struct {
	AudioLang, SubtitleLang, MenuLang string // three-letter codes, e.g. "eng"
	Country                           string // two letters, e.g. "US"
	Region                            int    // 1 = A, 2 = B, 4 = C; 0 = A
	Parental                          int    // 0 = no limit
	Stereo3D                          bool   // the display can show 3D
	UHD                               bool   // an Ultra HD player (for Ultra HD discs, which ask)
}

// NewRegisters makes a player's register file for the options. Registers
// nobody has chosen yet start at the "none" values the format defines: a disc
// sees 0xff for the audio stream until a stream is selected, and its program
// can tell a first visit to a menu from a later one that way.
func NewRegisters(o RegisterOptions) *Registers {
	r := &Registers{}
	r.PSR[PSRIGStream] = 1
	r.PSR[PSRAudio] = 0xff
	r.PSR[PSRPG] = 0x0fff0fff
	r.PSR[PSRAngle] = 1
	r.PSR[PSRTitle] = 0xffff
	r.PSR[PSRChapter] = 0xffff
	r.PSR[PSRButton] = 0xffff
	r.PSR[PSRStyle] = 0xff
	r.PSR[PSRSecondary] = 0xffff
	r.PSR[PSRBackupTitle], r.PSR[PSRBackupChapter] = 0xffff, 0xffff
	r.PSR[PSRBackupButton], r.PSR[PSRBackupStyle] = 0xffff, 0xff
	r.PSR[PSRAudioLang] = packLang(orDefault(o.AudioLang, "eng"))
	r.PSR[PSRPGLang] = packLang(orDefault(o.SubtitleLang, "eng"))
	r.PSR[PSRMenuLang] = packLang(orDefault(o.MenuLang, "eng"))
	c := orDefault(o.Country, "US")
	if len(c) < 2 {
		c = "US"
	}
	r.PSR[PSRCountry] = uint32(c[0])<<8 | uint32(c[1])
	r.PSR[PSRRegion] = uint32(orDefaultInt(o.Region, 1))
	r.PSR[PSRParental] = uint32(orDefaultInt(o.Parental, 255))
	// Every disc audio format can be converted, so all count as surround-capable.
	r.PSR[PSRAudioCap] = 0xaaaa
	r.PSR[PSRVideoCap] = 0x03
	r.PSR[PSRTextCap] = 0x1ffff
	r.PSR[PSRProfile] = Profile2v20
	for i := 48; i <= 61; i++ {
		r.PSR[i] = 0xffffffff // characteristic text: everything can be shown
	}
	switch {
	case o.UHD:
		r.PSR[PSRUHDCap], r.PSR[PSRUHDDisplayCap], r.PSR[PSRHDRPref] = 0xffffffff, 0xffffffff, 0xffffffff
		r.PSR[PSRProfile] = Profile6v30
	case o.Stereo3D:
		r.PSR[PSROutputPref] = 1
		r.PSR[PSRDisplayCap] = 0x0f
		r.PSR[PSR3DCap] = 0xffffffff
		r.PSR[PSRProfile] = Profile5v24
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
		in.Opt = int(w & 31)
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
			if in.Opt == 0x10 {
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
	// NavTimerOff: SET_NV_TIMER with a time of 0 cancelled the timer.
	NavTimerOff bool
}

// NavTimer is SET_NV_TIMER: run a movie object after some seconds (at most
// 300).
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
	MaxSteps int // 0 = 1000000; a disc's program cannot loop for ever
	// Button: the commands belong to a menu button, not a movie object. Some
	// commands only mean something there (LINK, TERMINATE_PL, POPUP_OFF,
	// stills), and changing the page ends the button's commands.
	Button bool
}

// Run executes commands from pc until one changes the player's course, or
// the end. Compare commands that are false skip the next command.
func (r *Registers) Run(cmds []Command, pc int, o RunOptions, fx *Effects) Result {
	max := o.MaxSteps
	if max <= 0 {
		max = 1000000
	}
	for steps := 0; ; steps++ {
		if pc < 0 || pc >= len(cmds) {
			return Result{Kind: ResultEnd, PC: pc, Item: -1, Mark: -1}
		}
		if steps > max {
			return Result{Kind: ResultError, PC: pc, Err: fmt.Errorf("the disc's program ran %d commands without playing anything", max)}
		}
		in := Decode(cmds[pc])
		dst, src := r.fetch(in, in.Dst, in.ImmDst), r.fetch(in, in.Src, in.ImmSrc)
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
					return Result{Kind: ResultPlay, Playlist: int(dst), Item: operandIndex(src), Mark: -1, PC: next}
				case 2:
					return Result{Kind: ResultPlay, Playlist: int(dst), Item: -1, Mark: operandIndex(src), PC: next}
				case 3:
					if o.Button {
						return Result{Kind: ResultTerminate, PC: next, Item: -1, Mark: -1}
					}
				case 4:
					if o.Button {
						return Result{Kind: ResultLinkItem, Item: operandIndex(dst), Mark: -1, PC: next}
					}
				case 5:
					if o.Button {
						return Result{Kind: ResultLinkMark, Mark: operandIndex(dst), Item: -1, PC: next}
					}
				}
			}
		case GroupCompare:
			var ok bool
			switch in.Opt {
			case 1:
				ok = dst&^src != 0 // a bit of the first operand that the second lacks
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
				case 3: // arithmetic saturates: a counter never wraps round
					v = saturate(uint64(dst) + uint64(src))
				case 4:
					if dst > src {
						v = dst - src
					}
				case 5:
					v = saturate(uint64(dst) * uint64(src))
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
					v = r.random(src)
				case 9:
					v = dst & src
				case 10:
					v = dst | src
				case 11:
					v = dst ^ src
				case 12:
					v = dst | shl1(src)
				case 13:
					v = dst &^ shl1(src)
				case 14:
					if src < 32 {
						v = dst << src
					}
				case 15:
					if src < 32 {
						v = dst >> src
					}
				default:
					v = dst
				}
				r.set(in.Dst, v)
			case 1:
				r.setSystem(in.Opt, dst, src, fx, o.Button)
				if o.Button && in.Opt == 3 {
					// Changing the page or button ends a button's commands.
					return Result{Kind: ResultEnd, PC: len(cmds), Item: -1, Mark: -1}
				}
			}
		}
		pc = next
	}
}

// fetch resolves an operand. A register operand normally means the
// register's value, but the stream and button-page system commands keep
// the operand's flag bits and take only the numbers from general-purpose
// registers named in the low bits (that is how discs choose a chapter page
// or an audio track from a computed value).
func (r *Registers) fetch(in Instr, v uint32, imm bool) uint32 {
	if imm {
		return v
	}
	if in.Group == GroupSet && in.Sub == 1 {
		switch in.Opt {
		case 1, 6, 11: // SET_STREAM, SET_SEC_STREAM, SET_STREAM_SS: two numbers, one per half
			return v&0xf000f000 | (r.GPR[(v>>16)&0xfff]&0xfff)<<16 | r.GPR[v&0xfff]&0xfff
		case 3: // SET_BUTTON_PAGE: one number
			return v&0xc0000000 | r.GPR[v&0xfff]&0x3fffffff
		}
	}
	return r.get(v)
}

// get reads an operand register. A reference with stray bits set is not a
// register, and reads as zero.
func (r *Registers) get(ref uint32) uint32 {
	if ref&0x80000000 != 0 {
		if ref&0x7fffff80 != 0 {
			return 0
		}
		return r.PSR[ref&0x7f]
	}
	if ref&0xfffff000 != 0 {
		return 0
	}
	return r.GPR[ref]
}

// set writes a general-purpose register; player status registers can only
// change through SET_SYSTEM commands, so writes to them are ignored.
func (r *Registers) set(ref, v uint32) {
	if ref&0xfffff000 != 0 {
		return
	}
	r.GPR[ref] = v
}

// operandIndex is a play item or mark number; a value with the top bit set
// means none was given.
func operandIndex(v uint32) int {
	if v >= 0x80000000 {
		return -1
	}
	return int(v)
}

func saturate(v uint64) uint32 {
	if v > 0xffffffff {
		return 0xffffffff
	}
	return uint32(v)
}

func shl1(n uint32) uint32 {
	if n > 31 {
		return 0
	}
	return 1 << n
}

// random is RND: 1..n.
func (r *Registers) random(n uint32) uint32 {
	if n == 0 {
		return 1
	}
	if r.Rand != nil {
		return uint32(r.Rand()*float64(n))%n + 1
	}
	return rand.Uint32N(n) + 1
}

// setPG writes the subtitle stream and whether subtitles show, leaving the
// picture-in-picture half of the register alone.
func (r *Registers) setPG(stream uint32, shown bool) {
	v := r.PSR[PSRPG]&^0x80000fff | stream&0xfff
	if shown {
		v |= 1 << 31
	}
	r.PSR[PSRPG] = v
}

// setSystem applies a SET_SYSTEM command: the registers it changes directly,
// and effects the player carries out.
func (r *Registers) setSystem(opt int, dst, src uint32, fx *Effects, button bool) {
	if fx == nil {
		fx = &Effects{}
	}
	switch opt {
	case 1: // SET_STREAM: dst = audio (bit 31 valid, bits 16..27) and PG (bit 15 valid, bits 0..11, bit 14 shown); src = IG (bit 31 valid, bits 16..23) and angle (bit 15 valid, bits 0..7)
		r.setStreams(dst, src)
		if src&0x8000 != 0 {
			r.PSR[PSRAngle] = src & 0xff
		}
		fx.StreamChange = true
	case 2: // SET_NV_TIMER: dst = the movie object, src = seconds (0 cancels, at most 300)
		obj, secs := dst&0xffff, src&0xffff
		switch {
		case secs == 0:
			r.PSR[PSRNavTimer] = 0
			fx.NavTimer, fx.NavTimerOff = nil, true
		case secs <= 300:
			r.PSR[PSRNavTimer] = secs
			fx.NavTimer, fx.NavTimerOff = &NavTimer{Seconds: secs, Object: int(obj)}, false
		}
	case 3: // SET_BUTTON_PAGE: dst = button (bit 31 valid, bits 0..15); src = page (bit 31 valid, bit 30 page effect off, bits 0..7)
		bp := &ButtonPage{}
		if dst&0x80000000 != 0 {
			bp.SetButton, bp.Button = true, dst&0xffff
			r.PSR[PSRButton] = bp.Button
		}
		if src&0x80000000 != 0 {
			bp.SetPage, bp.Page = true, src&0xff
			bp.PageEffectOff = src&0x40000000 != 0
			r.PSR[PSRPage] = bp.Page
		}
		fx.ButtonPage = bp
	case 4:
		if button {
			fx.EnableButton = append(fx.EnableButton, dst)
		}
	case 5:
		if button {
			fx.DisableButton = append(fx.DisableButton, dst)
		}
	case 6: // SET_SEC_STREAM: dst = secondary video (bit 31 valid, bits 0..7; bit 23 valid, bits 16..19 size; bit 30 shown); src = secondary audio (bit 31 valid, bits 16..23; bit 30 shown), PiP subtitles (bit 15 valid, bits 0..11; bit 13 shown)
		v := r.PSR[PSRSecondary]
		if dst&0x80000000 != 0 {
			v = v&^0xff00 | (dst&0xff)<<8
		}
		if dst&0x00800000 != 0 {
			v = v&^0x0f000000 | (dst>>16&15)<<24
		}
		if src&0x80000000 != 0 {
			v = v&^0xff | src>>16&0xff
		}
		v = v&^0xc0000000 | (dst&0x40000000)<<1 | src&0x40000000
		r.PSR[PSRSecondary] = v
		if src&0x8000 != 0 {
			r.PSR[PSRPG] = r.PSR[PSRPG]&^0x0fff0000 | (src&0xfff)<<16
		}
		r.PSR[PSRPG] = r.PSR[PSRPG]&^(1<<30) | (src&0x2000)<<17
		fx.StreamChange = true
	case 7:
		if button {
			fx.PopupOff = true
		}
	case 8:
		if button {
			on := true
			fx.Still = &on
		}
	case 9:
		if button {
			off := false
			fx.Still = &off
		}
	case 10: // SET_OUTPUT_MODE: dst bit 0 asks for 3D, which only a 3D player can do
		if r.PSR[PSRProfile]&Profile5v24 != Profile5v24 {
			return
		}
		want := dst&1 != 0
		r.PSR[PSRStereoStatus] = r.PSR[PSRStereoStatus]&^1 | b2u(want)
		fx.OutputMode3D = &want
	case 11: // SET_STREAM_SS: as SET_STREAM, for 3D stream numbers, only during 3D output
		if r.PSR[PSRStereoStatus]&1 != 0 {
			r.setStreams(dst, src)
			fx.StreamChange = true
		}
	case 0x10:
		r.PSR[103] = dst // not documented; kept so a disc can read it back
	}
}

// setStreams is the part SET_STREAM and SET_STREAM_SS share.
func (r *Registers) setStreams(dst, src uint32) {
	if dst&0x80000000 != 0 {
		r.PSR[PSRAudio] = (dst >> 16) & 0xfff
	}
	if dst&0x8000 != 0 {
		r.setPG(dst&0xfff, dst&0x4000 != 0)
	}
	if src&0x80000000 != 0 {
		r.PSR[PSRIGStream] = (src >> 16) & 0xff
	}
}

func b2u(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}
