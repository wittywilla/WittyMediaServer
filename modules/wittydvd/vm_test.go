package wittydvd

import "testing"

// run executes a command list with Rnd fixed to the middle of its range.
func run(t *testing.T, r *Registers, cmds ...Command) Result {
	t.Helper()
	r.Rand = func() float64 { return 0.5 }
	return r.Run(cmds, RunOptions{})
}

func TestVMArithmetic(t *testing.T) {
	cases := []struct {
		name string
		prog []Command
		reg  int
		want uint16
	}{
		// Type 3, immediate: 0x7s = 011 1 ssss, b3 = destination, b4-5 = value.
		{"mov", []Command{cmd(0x71, 0, 0, 1, 0x01, 0x2c)}, 1, 300},
		{"add saturates", []Command{cmd(0x71, 0, 0, 1, 0xfd, 0xe8), cmd(0x73, 0, 0, 1, 0x03, 0xe8)}, 1, 65535},
		{"sub floors at 0", []Command{cmd(0x71, 0, 0, 1, 0, 5), cmd(0x74, 0, 0, 1, 0, 9)}, 1, 0},
		{"mul saturates", []Command{cmd(0x71, 0, 0, 1, 0x10, 0), cmd(0x75, 0, 0, 1, 0x10, 0)}, 1, 65535},
		{"div", []Command{cmd(0x71, 0, 0, 1, 0, 17), cmd(0x76, 0, 0, 1, 0, 5)}, 1, 3},
		{"div by zero", []Command{cmd(0x71, 0, 0, 1, 0, 17), cmd(0x76, 0, 0, 1, 0, 0)}, 1, 65535},
		{"mod", []Command{cmd(0x71, 0, 0, 1, 0, 17), cmd(0x77, 0, 0, 1, 0, 5)}, 1, 2},
		{"and", []Command{cmd(0x71, 0, 0, 1, 0, 0x0f), cmd(0x79, 0, 0, 1, 0, 0x3c)}, 1, 0x0c},
		{"or", []Command{cmd(0x71, 0, 0, 1, 0, 0x0f), cmd(0x7a, 0, 0, 1, 0, 0x30)}, 1, 0x3f},
		{"xor", []Command{cmd(0x71, 0, 0, 1, 0, 0x0f), cmd(0x7b, 0, 0, 1, 0, 0x3c)}, 1, 0x33},
		// Rnd gives 1..n; rand fixed at 0.5 picks the middle.
		{"rnd", []Command{cmd(0x78, 0, 0, 1, 0, 6)}, 1, 4},
		// Register source: b5 names the register (type 3, I=0).
		{"mov from register", []Command{cmd(0x71, 0, 0, 2, 0, 40), cmd(0x61, 0, 0, 1, 0, 2)}, 1, 40},
		{"read SPRM", []Command{cmd(0x61, 0, 0, 1, 0, 0x80+SprmParental)}, 1, 15},
	}
	for _, c := range cases {
		r := NewRegisters(RegisterOptions{})
		if res := run(t, r, c.prog...); res.Kind != ResultEnd {
			t.Errorf("%s: %+v", c.name, res)
		}
		if got := r.GPRM[c.reg]; got != c.want {
			t.Errorf("%s: g%d = %d, want %d", c.name, c.reg, got, c.want)
		}
	}
}

func TestVMSwap(t *testing.T) {
	r := NewRegisters(RegisterOptions{})
	run(t, r, cmd(0x71, 0, 0, 1, 0, 7), cmd(0x71, 0, 0, 2, 0, 9), cmd(0x62, 0, 0, 1, 0, 2))
	if r.GPRM[1] != 9 || r.GPRM[2] != 7 {
		t.Fatalf("swap: g1=%d g2=%d", r.GPRM[1], r.GPRM[2])
	}
}

func TestVMCombinedCommands(t *testing.T) {
	// Type 4: set, then compare the result and link. g1 = 5; if (g1 == 5) LinkTailPGC.
	r := NewRegisters(RegisterOptions{})
	res := run(t, r, cmd(0x91, 0xa1, 0, 5, 0, 5, 0, 0x0d))
	if r.GPRM[1] != 5 || res.Kind != ResultLink || res.Link.Op != LinkTailPGC {
		t.Fatalf("type 4: g1=%d %+v", r.GPRM[1], res)
	}
	// Type 5, immediate set: if (g2 == g3) { g1 = 7; LinkTailPGC }. Compare
	// registers are b4 and b5 when the set value is immediate.
	for _, eq := range []bool{true, false} {
		r := NewRegisters(RegisterOptions{})
		r.GPRM[2], r.GPRM[3] = 4, 4
		if !eq {
			r.GPRM[3] = 5
		}
		res := run(t, r, cmd(0xb1, 0x21, 0, 7, 2, 3, 0, 0x0d))
		if eq != (r.GPRM[1] == 7) || eq != (res.Kind == ResultLink) {
			t.Fatalf("type 5 eq=%v: g1=%d %+v", eq, r.GPRM[1], res)
		}
	}
	// Type 6: the compare only gates the set; the link always happens.
	r = NewRegisters(RegisterOptions{})
	r.GPRM[2], r.GPRM[3] = 1, 2
	res = run(t, r, cmd(0xd1, 0x21, 0, 7, 2, 3, 0, 0x0d))
	if r.GPRM[1] != 0 || res.Kind != ResultLink {
		t.Fatalf("type 6: g1=%d %+v", r.GPRM[1], res)
	}
	// Type 5, register set: if (g3 == 9) { g1 = g2 } with b2 = source and
	// b3 = compare register, b4-5 = compare value.
	r = NewRegisters(RegisterOptions{})
	r.GPRM[2], r.GPRM[3] = 11, 9
	run(t, r, cmd(0xa1, 0xa1, 2, 3, 0, 9, 0, 0))
	if r.GPRM[1] != 11 {
		t.Fatalf("type 5 register: g1=%d", r.GPRM[1])
	}
}

func TestVMSystemCommands(t *testing.T) {
	r := NewRegisters(RegisterOptions{})
	// SetSTN audio=2 subp=5 (on), angle unchanged.
	run(t, r, cmd(0x51, 0, 0, 0x82, 0xc5, 0))
	if r.SPRM[SprmAudio] != 2 || r.SPRM[SprmSubp] != 0x45 || r.SPRM[SprmAngle] != 1 {
		t.Fatalf("SetSTN: %v", r.SPRM[:4])
	}
	// SetHL_BTNN 3.
	run(t, r, cmd(0x56, 0, 0, 0, 0x0c, 0))
	if r.SPRM[SprmHLBTNN] != 3<<10 {
		t.Fatalf("SetHL_BTNN: %#x", r.SPRM[SprmHLBTNN])
	}
	// SetNVTMR 30 s, PGC 5: expires after 30 whole seconds, not before.
	run(t, r, cmd(0x52, 0, 0, 30, 0, 5))
	if r.SPRM[SprmNavTimer] != 30 || r.SPRM[SprmNavPGCN] != 5 {
		t.Fatalf("SetNVTMR: %d %d", r.SPRM[SprmNavTimer], r.SPRM[SprmNavPGCN])
	}
	if r.Tick(29.6) || !r.Tick(0.5) {
		t.Fatal("nav timer expiry")
	}
	// SetGPRMMD g3 = 10 in counter mode: it counts wall-clock seconds.
	run(t, r, cmd(0x53, 0, 0, 10, 0, 0x83))
	r.Tick(2.5)
	if r.GPRM[3] != 12 {
		t.Fatalf("counter after 2.5s: %d", r.GPRM[3])
	}
	r.Tick(0.5)
	if r.GPRM[3] != 13 || !r.Counter[3] {
		t.Fatalf("counter after 3s: %d", r.GPRM[3])
	}
	// Back to register mode: the value stays and stops counting.
	run(t, r, cmd(0x53, 0, 0, 0, 0, 0x03), cmd(0x71, 0, 0, 3, 0, 13))
	r.Tick(5)
	if r.GPRM[3] != 13 || r.Counter[3] {
		t.Fatalf("register mode: %d", r.GPRM[3])
	}
	// SetAMXMD (karaoke mixing) only records the mode.
	run(t, r, cmd(0x54, 0, 0, 0, 0x04, 0x04))
	if r.SPRM[SprmAudioMix] != 0x0404 {
		t.Fatalf("SetAMXMD: %#x", r.SPRM[SprmAudioMix])
	}
}

func TestVMSpecialCommands(t *testing.T) {
	// SetTmpPML 4 then go to line 3, skipping line 2.
	r := NewRegisters(RegisterOptions{})
	run(t, r, cmd(0x00, 0x03, 0, 0, 0, 0, 4, 3), cmd(0x71, 0, 0, 1, 0, 1), cmd(0x71, 0, 0, 2, 0, 2))
	if r.SPRM[SprmParental] != 4 || r.GPRM[1] != 0 || r.GPRM[2] != 2 {
		t.Fatalf("SetTmpPML: s13=%d g1=%d g2=%d", r.SPRM[SprmParental], r.GPRM[1], r.GPRM[2])
	}
	// Break ends the list; nothing after it runs.
	r = NewRegisters(RegisterOptions{})
	res := run(t, r, cmd(0x00, 0x02), cmd(0x71, 0, 0, 1, 0, 1))
	if res.Kind != ResultBreak || r.GPRM[1] != 0 {
		t.Fatalf("Break: %+v g1=%d", res, r.GPRM[1])
	}
	// Compare with an SPRM operand: if (s20 & 1) Goto 3.
	r = NewRegisters(RegisterOptions{Region: 1})
	run(t, r, cmd(0x00, 0x91, 0, 0x80+SprmRegion, 0, 1, 0, 3), cmd(0x71, 0, 0, 1, 0, 1), cmd(0x71, 0, 0, 2, 0, 2))
	if r.GPRM[1] != 0 || r.GPRM[2] != 2 {
		t.Fatalf("region compare: g1=%d g2=%d", r.GPRM[1], r.GPRM[2])
	}
}

func TestVMReservedBits(t *testing.T) {
	cases := []struct {
		in   Command
		want string
	}{
		// The reserved top bits of title, program and PGC numbers are ignored.
		{cmd(0x30, 0x02, 0, 0, 0, 0x85, 0, 0), "JumpTT 5"},
		{cmd(0x20, 0x04, 0, 0, 0, 0, 0x80, 0x03), "LinkPGCN 3"},
		{cmd(0x20, 0x06, 0, 0, 0, 0, 0, 0x82), "LinkPGN 2"},
		{cmd(0x30, 0x06, 0x80, 0x07, 0, 0xc0, 0, 0), "JumpSS VMGM pgc 7"},
		{cmd(0x30, 0x06, 0, 0x82, 0x03, 0x83, 0, 0), "JumpSS VTSM vts 3 title 2 menu root"},
		// A register that does not exist makes the command invalid.
		{cmd(0x71, 0, 0, 1, 0, 0, 0, 0), "g1 = 0"},
		{cmd(0x61, 0, 0, 1, 0, 0x20, 0, 0), "invalid: register 0x20 does not exist"},
		{cmd(0xe0), "invalid: command type 7 unknown"},
	}
	for _, c := range cases {
		if got := Decode(c.in).String(); got != c.want {
			t.Errorf("%x: got %q want %q", c.in, got, c.want)
		}
	}
	if res := NewRegisters(RegisterOptions{}).Run([]Command{cmd(0xe0)}, RunOptions{}); res.Kind != ResultStop {
		t.Fatalf("invalid command must stop: %+v", res)
	}
}

func TestVideoModeRegister(t *testing.T) {
	r := NewRegisters(RegisterOptions{})
	if aspect, mode := r.SPRM[SprmVideoMode]>>10&3, r.SPRM[SprmVideoMode]>>8&3; aspect != 3 || mode != 0 {
		t.Fatalf("SPRM14 = %#x: want a 16:9 display in normal mode", r.SPRM[SprmVideoMode])
	}
}
