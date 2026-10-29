package wittydvd

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
)

// Register file of the DVD virtual machine.
type Registers struct {
	GPRM      [16]uint16
	Counter   [16]bool // GPRM counter mode (increments once per second)
	SPRM      [24]uint16
	clockFrac float64
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

func code2(s string) uint16 {
	if len(s) != 2 {
		return 0xffff
	}
	return uint16(s[0])<<8 | uint16(s[1])
}

// RegisterOptions seeds player preferences.
type RegisterOptions struct {
	MenuLanguage     string
	AudioLanguage    string
	SubtitleLanguage string
	Region           int // 1-8
	Angle            int // initial camera angle (0 = 1)
	Rand             func() float64
}

// NewRegisters builds a register file with spec defaults.
func NewRegisters(o RegisterOptions) *Registers {
	r := &Registers{Rand: o.Rand}
	if r.Rand == nil {
		r.Rand = rand.Float64
	}
	r.SPRM[SprmMenuLang] = code2("en")
	r.SPRM[SprmAudio] = 15
	r.SPRM[SprmSubp] = 62
	r.SPRM[SprmAngle] = 1
	if o.Angle > 1 && o.Angle <= 9 {
		r.SPRM[SprmAngle] = uint16(o.Angle)
	}
	r.SPRM[SprmTTN] = 1
	r.SPRM[SprmVTSTTN] = 1
	r.SPRM[SprmPTTN] = 1
	r.SPRM[SprmHLBTNN] = 1 << 10
	r.SPRM[SprmCountry] = code2("US")
	r.SPRM[SprmParental] = 15
	r.SPRM[SprmVideoMode] = 3 << 10 // 16:9 capable, letterbox preferred
	r.SPRM[SprmAudioCap] = (1 << 14) | (1 << 12) | (1 << 11)
	r.SPRM[SprmAudioLang] = 0xffff
	r.SPRM[SprmSubpLang] = 0xffff
	r.SPRM[SprmRegion] = 1
	if o.MenuLanguage != "" {
		r.SPRM[SprmMenuLang] = code2(o.MenuLanguage)
	}
	if o.AudioLanguage != "" {
		r.SPRM[SprmAudioLang] = code2(o.AudioLanguage)
	}
	if o.SubtitleLanguage != "" {
		r.SPRM[SprmSubpLang] = code2(o.SubtitleLanguage)
	}
	if o.Region >= 1 && o.Region <= 8 {
		r.SPRM[SprmRegion] = 1 << (o.Region - 1)
	}
	return r
}

// Clone copies the register file.
func (r *Registers) Clone() *Registers {
	c := *r
	return &c
}

// Tick advances the wall clock: counter GPRMs increment, the navigation timer
// counts down. Returns true when the nav timer just expired.
func (r *Registers) Tick(seconds float64) bool {
	if seconds < 0 {
		seconds = 0
	}
	total := r.clockFrac + seconds
	whole := int(total)
	r.clockFrac = total - float64(whole)
	if whole == 0 {
		return false
	}
	for i := range r.GPRM {
		if r.Counter[i] {
			r.GPRM[i] = uint16((int(r.GPRM[i]) + whole) % 65536)
		}
	}
	before := r.SPRM[SprmNavTimer]
	if before > 0 {
		if int(before) <= whole {
			r.SPRM[SprmNavTimer] = 0
		} else {
			r.SPRM[SprmNavTimer] = before - uint16(whole)
		}
	}
	return before > 0 && r.SPRM[SprmNavTimer] == 0
}

func (r *Registers) read(o operand) uint16 {
	if !o.isReg {
		return o.val
	}
	if o.reg < 16 {
		return r.GPRM[o.reg]
	}
	if o.reg >= 0x80 && o.reg < 0x80+24 {
		return r.SPRM[o.reg-0x80]
	}
	return 0
}

// ---------------------------------------------------------------------------
// Instruction decoding
// ---------------------------------------------------------------------------

type operand struct {
	isReg bool
	reg   int
	val   uint16
}

func regOp(r byte) operand   { return operand{isReg: true, reg: int(r)} }
func valOp(v uint16) operand { return operand{val: v} }

func (o operand) String() string {
	if !o.isReg {
		return fmt.Sprint(o.val)
	}
	if o.reg < 16 {
		return fmt.Sprintf("g%d", o.reg)
	}
	return fmt.Sprintf("s%d", o.reg-0x80)
}

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

var linkNames = map[LinkOp]string{
	LinkTopCell: "LinkTopCell", LinkNextCell: "LinkNextCell", LinkPrevCell: "LinkPrevCell", LinkTopPG: "LinkTopPG",
	LinkNextPG: "LinkNextPG", LinkPrevPG: "LinkPrevPG", LinkTopPGC: "LinkTopPGC", LinkNextPGC: "LinkNextPGC",
	LinkPrevPGC: "LinkPrevPGC", LinkGoUpPGC: "LinkGoUpPGC", LinkTailPGC: "LinkTailPGC", LinkRSM: "RSM",
	LinkPGCN: "LinkPGCN", LinkPTTN: "LinkPTTN", LinkPGN: "LinkPGN", LinkCN: "LinkCN",
}

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

const (
	setNone setOp = iota
	setMov
	setSwp
	setAdd
	setSub
	setMul
	setDiv
	setMod
	setRnd
	setAnd
	setOr
	setXor
)

var setText = [...]string{"", "=", "<->", "+=", "-=", "*=", "/=", "%=", "=rnd", "&=", "|=", "^="}

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

var cmpText = [...]string{"", "&", "==", "!=", ">=", ">", "<=", "<"}

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

const (
	spNone specialOp = iota
	spNop
	spGoto
	spBreak
	spSetTmpPML
)

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
			return &Link{Op: LinkPGCN, N: int(u16(6))}, ""
		case 5:
			return &Link{Op: LinkPTTN, N: int(b[6]&3)<<8 | int(b[7]), Button: button}, ""
		case 6:
			return &Link{Op: LinkPGN, N: int(b[7]), Button: button}, ""
		case 7:
			return &Link{Op: LinkCN, N: int(b[7]), Button: button}, ""
		}
		return nil, fmt.Sprintf("link command %d invalid", cmd)
	}
	jumpOf := func(cmd int) (*Jump, string) {
		sel := b[5] >> 6
		menu := int(b[5] & 0x0f)
		pgcn := int(u16(2))
		switch cmd {
		case 1:
			return &Jump{Op: JumpExit}, ""
		case 2:
			return &Jump{Op: JumpTT, TTN: int(b[5])}, ""
		case 3:
			return &Jump{Op: JumpVTSTT, TTN: int(b[5])}, ""
		case 5:
			return &Jump{Op: JumpVTSPTT, TTN: int(b[5]), PTTN: int(b[2]&3)<<8 | int(b[3])}, ""
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
					j.VTS, j.TTN = int(b[4]), int(b[3])
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

// String renders a human readable form (for diagnostics).
func (in Instr) String() string {
	if in.Invalid != "" {
		return "invalid: " + in.Invalid
	}
	var parts []string
	cond := ""
	if in.cmp != cmpNone {
		cond = fmt.Sprintf("if (%s %s %s) ", in.cmpA, cmpText[in.cmp], in.cmpB)
	}
	switch in.sp {
	case spNop:
		parts = append(parts, "NOP")
	case spGoto:
		parts = append(parts, fmt.Sprintf("Goto %d", in.spLin))
	case spBreak:
		parts = append(parts, "Break")
	case spSetTmpPML:
		parts = append(parts, fmt.Sprintf("SetTmpPML %d, %d", in.spLvl, in.spLin))
	}
	if in.set != setNone {
		parts = append(parts, fmt.Sprintf("g%d %s %s", in.setD, setText[in.set], in.setS))
	}
	switch in.sys {
	case sysSetSTN:
		s := "SetSTN"
		if in.sysA.reg != -1 {
			s += " audio=" + in.sysA.String()
		}
		if in.sysB.reg != -1 {
			s += " subp=" + in.sysB.String()
		}
		if in.sysC.reg != -1 {
			s += " angle=" + in.sysC.String()
		}
		parts = append(parts, s)
	case sysSetNVTMR:
		parts = append(parts, fmt.Sprintf("SetNVTMR %s pgc %d", in.sysA, in.sysN))
	case sysSetGPRMMD:
		mode := "register"
		if in.sysCt {
			mode = "counter"
		}
		parts = append(parts, fmt.Sprintf("SetGPRMMD g%d = %s (%s)", in.sysN, in.sysA, mode))
	case sysSetAMXMD:
		parts = append(parts, "SetAMXMD "+in.sysA.String())
	case sysSetHLBTNN:
		parts = append(parts, "SetHL_BTNN "+in.sysA.String())
	}
	if in.jump != nil {
		parts = append(parts, in.jump.String())
	}
	if in.link != nil {
		parts = append(parts, in.link.String())
	}
	if len(parts) == 0 {
		parts = append(parts, "NOP")
	}
	return cond + strings.Join(parts, "; ")
}

func (l *Link) String() string {
	s := linkNames[l.Op]
	if l.Op >= LinkPGCN {
		s += fmt.Sprintf(" %d", l.N)
	}
	if l.Button != 0 {
		s += fmt.Sprintf(" (button %d)", l.Button)
	}
	return s
}

func (j *Jump) String() string {
	menu := map[int]string{2: "title", 3: "root", 4: "subpicture", 5: "audio", 6: "angle", 7: "chapter"}
	where := func() string {
		switch {
		case j.Domain == DomainFP:
			return "FP"
		case j.HasPGCN:
			return fmt.Sprintf("VMGM pgc %d", j.PGCN)
		case j.Domain == DomainVTSM:
			s := "VTSM"
			if j.VTS != 0 {
				s += fmt.Sprintf(" vts %d title %d", j.VTS, j.TTN)
			}
			return s + " menu " + menu[j.Menu]
		}
		return "VMGM menu " + menu[j.Menu]
	}
	switch j.Op {
	case JumpExit:
		return "Exit"
	case JumpTT:
		return fmt.Sprintf("JumpTT %d", j.TTN)
	case JumpVTSTT:
		return fmt.Sprintf("JumpVTS_TT %d", j.TTN)
	case JumpVTSPTT:
		return fmt.Sprintf("JumpVTS_PTT %d chapter %d", j.TTN, j.PTTN)
	case JumpSS:
		return "JumpSS " + where()
	}
	s := "CallSS " + where()
	if j.RsmCell != 0 {
		s += fmt.Sprintf(" (resume cell %d)", j.RsmCell)
	}
	return s
}

// ---------------------------------------------------------------------------
// Execution
// ---------------------------------------------------------------------------

// ResultKind says how a command list ended.
type ResultKind int

const (
	ResultEnd   ResultKind = iota // ran off the end
	ResultBreak                   // Break
	ResultExit                    // Exit
	ResultLink
	ResultJump
	ResultStop // invalid command / runaway
)

// Result of running a command list.
type Result struct {
	Kind   ResultKind
	Link   *Link
	Jump   *Jump
	Reason string
	Steps  int
}

type stepKind int

const (
	stNext stepKind = iota
	stGoto
	stBreak
	stExit
	stLink
	stJump
)

func compare(op cmpOp, a, b uint16) bool {
	switch op {
	case cmpBC:
		return a&b != 0
	case cmpEQ:
		return a == b
	case cmpNE:
		return a != b
	case cmpGE:
		return a >= b
	case cmpGT:
		return a > b
	case cmpLE:
		return a <= b
	case cmpLT:
		return a < b
	}
	return true
}

func clamp16(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > 65535 {
		return 65535
	}
	return uint16(v)
}

func (r *Registers) doSet(in *Instr, res *Result) {
	d := r.GPRM[in.setD]
	s := r.read(in.setS)
	var v uint16
	switch in.set {
	case setMov:
		v = s
	case setSwp:
		r.GPRM[in.setD] = s
		if in.setS.isReg && in.setS.reg < 16 {
			r.GPRM[in.setS.reg] = d
		}
		return
	case setAdd:
		v = clamp16(int(d) + int(s))
	case setSub:
		v = clamp16(int(d) - int(s))
	case setMul:
		v = clamp16(int(d) * int(s))
	case setDiv:
		if s == 0 {
			v = 65535
		} else {
			v = d / s
		}
	case setMod:
		if s == 0 {
			v = 0
		} else {
			v = d % s
		}
	case setRnd:
		if s == 0 {
			v = 0
		} else {
			n := int(r.Rand()*float64(s)) + 1
			if n > int(s) {
				n = int(s)
			}
			v = uint16(n)
		}
	case setAnd:
		v = d & s
	case setOr:
		v = d | s
	case setXor:
		v = d ^ s
	}
	r.GPRM[in.setD] = v
}

func (r *Registers) setSPRM(i int, v uint16, _ *Result) {
	r.SPRM[i] = v
}

func (r *Registers) doSystem(in *Instr, res *Result) {
	switch in.sys {
	case sysSetSTN:
		if in.sysA.reg != -1 {
			r.setSPRM(SprmAudio, r.read(in.sysA), res)
		}
		if in.sysB.reg != -1 {
			r.setSPRM(SprmSubp, r.read(in.sysB), res)
		}
		if in.sysC.reg != -1 {
			r.setSPRM(SprmAngle, r.read(in.sysC), res)
		}
	case sysSetNVTMR:
		r.setSPRM(SprmNavTimer, r.read(in.sysA), res)
		r.setSPRM(SprmNavPGCN, uint16(in.sysN), res)
	case sysSetGPRMMD:
		r.GPRM[in.sysN] = r.read(in.sysA)
		r.Counter[in.sysN] = in.sysCt
	case sysSetAMXMD:
		r.setSPRM(SprmAudioMix, r.read(in.sysA), res)
	case sysSetHLBTNN:
		r.setSPRM(SprmHLBTNN, r.read(in.sysA), res)
	}
}

// exec runs one instruction.
func (r *Registers) exec(in *Instr, res *Result) (stepKind, int) {
	ok := func() bool {
		return in.cmp == cmpNone || compare(in.cmp, r.read(in.cmpA), r.read(in.cmpB))
	}
	var link *Link
	switch in.when {
	case whenLink:
		if in.set != setNone {
			r.doSet(in, res)
		}
		if ok() {
			link = in.link
		}
	case whenSetAndLink:
		if !ok() {
			return stNext, 0
		}
		if in.set != setNone {
			r.doSet(in, res)
		}
		link = in.link
	case whenSet:
		if ok() && in.set != setNone {
			r.doSet(in, res)
		}
		link = in.link
	default:
		if !ok() {
			return stNext, 0
		}
		if in.set != setNone {
			r.doSet(in, res)
		}
		if in.sys != sysNone {
			r.doSystem(in, res)
		}
		switch in.sp {
		case spGoto:
			return stGoto, in.spLin
		case spBreak:
			return stBreak, 0
		case spSetTmpPML:
			r.setSPRM(SprmParental, uint16(in.spLvl), res)
			return stGoto, in.spLin
		}
		if in.jump != nil {
			if in.jump.Op == JumpExit {
				return stExit, 0
			}
			res.Jump = in.jump
			return stJump, 0
		}
		link = in.link
	}
	if link == nil {
		return stNext, 0
	}
	if link.Button != 0 {
		r.setSPRM(SprmHLBTNN, uint16(link.Button)<<10, res)
	}
	res.Link = link
	return stLink, 0
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

const maxVMSteps = 10000

// RunOptions tune Run.
type RunOptions struct {
	Start int  // 0-based first instruction
	Once  bool // run exactly one instruction (cell commands, button commands)
}

// Run executes a command list against the registers.
func (r *Registers) Run(cmds []Command, o RunOptions) Result {
	res := Result{Kind: ResultEnd}
	if len(cmds) == 0 {
		return res
	}
	pc := o.Start
	for steps := 0; ; steps++ {
		if steps >= maxVMSteps {
			res.Kind, res.Reason = ResultStop, "command loop ran away"
			return res
		}
		if pc < 0 || pc >= len(cmds) {
			res.Steps = steps
			return res
		}
		in := decodeCached(cmds[pc])
		if in.Invalid != "" {
			res.Kind, res.Reason = ResultStop, fmt.Sprintf("command %d: %s", pc+1, in.Invalid)
			return res
		}
		k, arg := r.exec(in, &res)
		res.Steps = steps + 1
		switch k {
		case stNext:
			pc++
		case stGoto:
			pc = arg - 1 // lines are 1-based
		case stBreak:
			res.Kind = ResultBreak
			return res
		case stExit:
			res.Kind = ResultExit
			return res
		case stLink:
			res.Kind = ResultLink
			return res
		case stJump:
			res.Kind = ResultJump
			return res
		}
		if o.Once {
			return res
		}
	}
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
