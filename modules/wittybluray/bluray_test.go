package wittybluray

import (
	"os"
	"testing"
)

func TestDecode(t *testing.T) {
	cases := []struct {
		cmd  Command
		want string
	}{
		{Command{0x3180000a, 1, 0}, "SET_OUTPUT_MODE 1"},
		{Command{0x48400200, 0xffe, 3}, "EQ GPR4094, 3"},
		{Command{0x20810000, 8, 0}, "GOTO 8"},
		{Command{0x22800000, 2, 0}, "PLAY_PL 2"},
		{Command{0x42820000, 3, 0xfff}, "PLAY_PL_PM 3, GPR4095"},
		{Command{0x50400001, 0xec, 1}, "MOVE GPR236, 1"},
		{Command{0x50000001, 1, 0x80000001}, "MOVE GPR1, PSR1"},
		{Command{0x21810000, 0, 0}, "JUMP_TITLE 0"},
		{Command{0x51800001, 0x8001, 0}, "SET_STREAM 32769, GPR0"},
		{Command{0x00000000, 0, 0}, "NOP"},
	}
	for _, c := range cases {
		if got := Decode(c.cmd).String(); got != c.want {
			t.Errorf("%08x: got %q, want %q", c.cmd.Op, got, c.want)
		}
	}
}

func TestRun(t *testing.T) {
	// A program like a disc's: branch on a register, play, loop.
	cmds := []Command{
		{0x50400001, 10, 3},  // MOVE GPR10, 3
		{0x48400200, 10, 3},  // EQ GPR10, 3
		{0x20810000, 4, 0},   // GOTO 4
		{0x22800000, 99, 0},  // PLAY_PL 99 (skipped)
		{0x50400001, 11, 7},  // MOVE GPR11, 7
		{0x50400003, 11, 5},  // ADD GPR11, 5
		{0x48400700, 11, 20}, // LT GPR11, 20
		{0x42c10000, 7, 2},   // PLAY_PL_PI 7, 2
		{0x21810000, 0, 0},   // JUMP_TITLE 0
	}
	r := NewRegisters(RegisterOptions{})
	res := r.Run(cmds, 0, RunOptions{}, nil)
	if res.Kind != ResultPlay || res.Playlist != 7 || res.Item != 2 || res.PC != 8 {
		t.Fatalf("got %+v", res)
	}
	if r.GPR[11] != 12 {
		t.Fatalf("GPR11 = %d", r.GPR[11])
	}
	res = r.Run(cmds, res.PC, RunOptions{}, nil)
	if res.Kind != ResultJumpTitle || res.Title != 0 {
		t.Fatalf("got %+v", res)
	}
	// A program that never plays anything stops instead of hanging.
	loop := []Command{{0x20810000, 0, 0}}
	if res := r.Run(loop, 0, RunOptions{MaxSteps: 100}, nil); res.Kind != ResultError {
		t.Fatalf("loop: got %+v", res)
	}
	// PSRs cannot be written by SET commands, only by SET_SYSTEM.
	r.Run([]Command{{0x50400001, 0x80000001, 5}}, 0, RunOptions{}, nil)
	if r.PSR[PSRAudio] != 1 {
		t.Fatal("a MOVE changed a PSR")
	}
	var fx Effects
	r.Run([]Command{{0x51c00001, 0x80050000 | 0x8000 | 0x4000 | 2, 0x80030000 | 0x8000 | 1}}, 0, RunOptions{}, &fx)
	// SET_BUTTON_PAGE with register operands: the flags come from the operand,
	// the button and page numbers from the named registers (a chapter page
	// chosen from the current chapter).
	r.GPR[10], r.GPR[11] = 7, 3
	var bp Effects
	r.Run([]Command{{0x51000003, 0x8000000a, 0x8000000b}}, 0, RunOptions{}, &bp)
	if bp.ButtonPage == nil || !bp.ButtonPage.SetButton || bp.ButtonPage.Button != 7 || !bp.ButtonPage.SetPage || bp.ButtonPage.Page != 3 {
		t.Fatalf("SET_BUTTON_PAGE from registers: %+v", bp.ButtonPage)
	}
	if r.PSR[PSRAudio] != 5 || r.PSR[PSRPG]&0xfff != 2 || r.PSR[PSRPG]>>31 != 1 || r.PSR[PSRIGStream] != 3 || r.PSR[PSRAngle] != 1 || !fx.StreamChange {
		t.Fatalf("SET_STREAM: audio %d pg %#x ig %d angle %d", r.PSR[PSRAudio], r.PSR[PSRPG], r.PSR[PSRIGStream], r.PSR[PSRAngle])
	}
}

// TestRealDisc checks the parsers against a decrypted Blu-ray when
// WMS_TEST_BD points at an image or a BDMV folder (skipped otherwise).
func TestRealDisc(t *testing.T) {
	p := os.Getenv("WMS_TEST_BD")
	if p == "" {
		t.Skip("set WMS_TEST_BD to a decrypted Blu-ray image or folder")
	}
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Encrypted {
		t.Fatal("the test disc is reported as encrypted")
	}
	main := d.MainPlaylist()
	if main == nil || d.PlaylistDuration(main) < 60 {
		t.Fatal("no main playlist")
	}
	for _, c := range d.Clips {
		ext, err := d.ClipExtents(c.Name)
		if err != nil {
			t.Errorf("clip %s: %v", c.Name, err)
			continue
		}
		var size int64
		for _, e := range ext {
			size += e.Length
		}
		if size != c.Size() {
			t.Errorf("clip %s: %d bytes in the image, %d packets in its clip information", c.Name, size, c.Packets)
		}
		for _, m := range c.EPMaps {
			for i := 1; i < len(m.Entries); i++ {
				a, b := m.Entries[i-1], m.Entries[i]
				if b.PTS < a.PTS || b.SPN <= a.SPN {
					t.Errorf("clip %s PID %#x: entry %d (%d, %d) after (%d, %d) is out of order", c.Name, m.PID, i, b.PTS, b.SPN, a.PTS, a.SPN)
					break
				}
				if b.SPN >= c.Packets {
					t.Errorf("clip %s: entry %d packet %d is past the end (%d)", c.Name, i, b.SPN, c.Packets)
					break
				}
			}
		}
	}
	// The main feature starts at its first play item's IN time.
	it := main.Items[0]
	c := d.Clip(it.Clip)
	if c == nil {
		t.Fatal("main clip missing")
	}
	if e, ok := c.SPNAt(c.VideoPID(), it.In); !ok || e.PTS > it.In || ticks45(it.In-e.PTS) > 10 {
		t.Errorf("entry point for IN %d: %+v %v", it.In, e, ok)
	}
	// A navigator run reaches a playlist.
	nav := NewNav(d, NavOptions{})
	st := nav.Start()
	for i := 0; i < 10 && st.Kind == StepPlay && st.Playlist != nil && st.Playlist != main && !st.Menu; i++ {
		st = nav.PlaylistEnded()
	}
	if st.Kind != StepPlay {
		t.Fatalf("first play did not reach a playlist: %+v", st)
	}
}
