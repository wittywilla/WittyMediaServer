package wittydvd

import "testing"

// The parsers read whatever is on a disc image, so no input may crash them.
// `go test -fuzz=FuzzParsers` explores further; the seeds run with every test.

func FuzzParsers(f *testing.F) {
	f.Add(append([]byte("DVDVIDEO-VMG"), make([]byte, 0x200)...), uint8(0))
	f.Add(append([]byte("DVDVIDEO-VTS"), make([]byte, 0x200)...), uint8(1))
	f.Add(make([]byte, 0x3d4), uint8(2))
	f.Add([]byte{0x00, 0x20, 0x00, 0x10, 0x00, 0x00, 0x00, 0x10, 0x01, 0xff}, uint8(4))
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
		}
	})
}
