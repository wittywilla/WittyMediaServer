package wittybluray

import "testing"

// The parsers read whatever is on a disc image, so no input may crash them.
// `go test -fuzz=FuzzParsers` explores further; the seeds run with every test.

func FuzzParsers(f *testing.F) {
	f.Add([]byte("INDX0200"), uint8(0))
	f.Add([]byte("MOBJ0200\x00\x00\x00\x00"), uint8(1))
	f.Add([]byte("MPLS0200\x00\x00\x00\x3a\x00\x00\x00\x00"), uint8(2))
	f.Add([]byte("HDMV0200\x00\x00\x00\x00\x00\x00\x00\x00"), uint8(3))
	f.Add([]byte{0x16, 0x00, 0x10}, uint8(4))
	f.Add([]byte{0x15, 0x00, 0x05, 0x00, 0x01}, uint8(5))
	f.Fuzz(func(t *testing.T, b []byte, which uint8) {
		switch which % 7 {
		case 0:
			_, _ = ParseIndex(b)
		case 1:
			_, _ = ParseMovieObjects(b)
		case 2:
			_, _ = ParsePlaylist(1, b)
		case 3:
			_, _ = ParseClip("00001", b)
		case 4:
			m := &Menu{}
			_ = m.parseIC(b)
		case 5:
			_, _ = parsePDS(b, true)
			_, _ = parseCompObject(b)
			_ = parseWindows(b)
		case 6:
			_, _ = decodeRLE(b, 64, 64)
		}
	})
}
