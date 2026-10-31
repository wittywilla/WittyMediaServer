package wittyplay

import (
	"os"
	"path/filepath"
	"strings"
)

// Satellaview games were broadcast to a receiver on the Super Famicom. They run
// from the receiver's own cartridge, and with no such thing every core shows a
// blank picture rather than an error, so the server has to know one when it sees it.
const (
	SatellaviewCore = "mesen-s"
	SatellaviewBIOS = "BS-X.bin"
)

// IsSatellaview says whether a SNES file is a broadcast game. Its header sits where
// a cartridge's does, with different fields: where a cartridge has the end of its
// title, this has a zero, and where a cartridge gives its save memory size, this
// gives the map mode and speed. A file with neither shape is taken as a cartridge,
// which at worst leaves it showing a blank picture.
func IsSatellaview(path string) bool {
	if strings.EqualFold(filepath.Ext(path), ".bs") {
		return true
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false
	}
	var skip int64
	if st.Size()%1024 == 512 { // a copier's header
		skip = 512
	}
	var h [0x20]byte
	for _, at := range []int64{0x7FC0, 0xFFC0} {
		if _, err := f.ReadAt(h[:], at+skip); err == nil && satellaviewHeader(h[:]) {
			return true
		}
	}
	return false
}

func satellaviewHeader(h []byte) bool {
	maker, titleEnd, starts, month, day, mapMode, fileType := h[0x1A], h[0x14], h[0x15], h[0x16], h[0x17], h[0x18], h[0x19]
	return (maker == 0x33 || maker == 0xFF) && titleEnd == 0 &&
		mapMode&0xCE == 0 && mapMode&0x30 != 0 && fileType&0x4F == 0 &&
		starts|month|day != 0
}
