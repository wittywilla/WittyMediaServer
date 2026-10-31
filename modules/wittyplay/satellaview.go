package wittyplay

import (
	"path/filepath"
	"strings"
)

// Satellaview games were broadcast to a receiver on the Super Famicom. Those dumped
// like cartridges run on any SNES core. A .bs file holds only what was broadcast, so
// it runs from the receiver's own cartridge, and only on this core.
const (
	SatellaviewCore = "mesen-s"
	SatellaviewBIOS = "BS-X.bin"
)

// IsSatellaview says whether a SNES file needs the receiver.
func IsSatellaview(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".bs")
}
