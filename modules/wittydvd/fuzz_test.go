package wittydvd

import (
	"testing"
	"time"
)

// The parsers read whatever is on a disc image, so no input may crash them.
// `go test -fuzz=FuzzParsers` explores further; the seeds run with every test.

func FuzzParsers(f *testing.F) {
	f.Add(append([]byte("DVDVIDEO-VMG"), make([]byte, 0x200)...), uint8(0))
	f.Add(append([]byte("DVDVIDEO-VTS"), make([]byte, 0x200)...), uint8(1))
	f.Add(testVMG(), uint8(0)) // with a parental management table
	f.Add(testVTS([3]int{2, 1, 1<<2 | 1}, [3]int{1, 2, 3<<2 | 1}), uint8(1))
	f.Add(make([]byte, 0x3d4), uint8(2))
	f.Add([]byte{0x00, 0x20, 0x00, 0x10, 0x00, 0x00, 0x00, 0x10, 0x01, 0xff}, uint8(4))
	f.Add(spuUnit(spuPix, append([]byte{0, 0}, spuSetup(0x07, 0x00, 16, 0x00, 0x14, 0x10, 0x15, 0x00, 0x0c, 0x00, 0x30, 0x00, 0x80, 0x0f, 0xff, 0xff, 0xff, 0x01, 0xff)...), []byte{0, 9, 0x02, 0xff}), uint8(4))
	f.Fuzz(func(t *testing.T, b []byte, which uint8) {
		switch which % 5 {
		case 0:
			_, _ = ParseVMG(b)
		case 1:
			_, _ = ParseVTS(b, 1)
		case 2:
			_ = ParsePCI(b)
		case 3:
			_ = ParseDSI(b)
			_ = ScanSectors(b, 0)
		case 4:
			if s, err := DecodeSPU(b); err == nil && s != nil {
				var pal [16]uint32
				_ = s.Render(pal, nil)
			}
			frames, _ := DecodeSPUFrames(b)
			for _, s := range frames {
				var pal [16]uint32
				_ = s.Render(pal, nil)
			}
		}
	})
}

// Running a disc's programs must always come back, however they are written,
// and no state a disc can reach may crash the navigator.
func FuzzNav(f *testing.F) {
	f.Add(testVMG(), testVTS([3]int{2, 1, 1<<2 | 1}, [3]int{1, 2, 3<<2 | 1}), uint8(3))
	f.Add(append([]byte("DVDVIDEO-VMG"), make([]byte, 0x200)...), append([]byte("DVDVIDEO-VTS"), make([]byte, 0x200)...), uint8(0))
	f.Fuzz(func(t *testing.T, vmgB, vtsB []byte, script uint8) {
		vmg, err := ParseVMG(vmgB)
		if err != nil {
			return
		}
		d := &Disc{VMG: vmg, VTS: map[int]*VTS{}}
		if v, err := ParseVTS(vtsB, 1); err == nil {
			d.VTS[1] = v
		}
		n := NewNav(d, NavOptions{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			n.Start()
			_ = d.Titles()
			_ = d.MainTitle()
			for i := uint8(0); i < script%8; i++ {
				n.CellEnded()
				n.PressButton(int(script), nil)
				n.SelectButton(int(script))
				n.MenuKey("title")
				n.GoUp()
				n.OpenMenu("root", int(script))
				n.Resume()
				n.PlayTitle(int(script), 1)
				n.NextChapter()
				n.PrevChapter()
				n.NavTimerFired()
				n.Tick(1)
				_ = n.SyncTitleCell(int(script), int(script))
			}
		}()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Fatal("a disc's program never came back")
		}
	})
}
