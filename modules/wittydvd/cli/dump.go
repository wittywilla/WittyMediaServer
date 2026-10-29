package cli

import (
	"fmt"

	"github.com/wittywilla/wittydvd"
)

// dvdDumpPGC prints the commands of one PGC: dvd pgc <image> <domain> <vts> <pgcn>
func dvdDumpPGC(o *wittydvd.Opened, domain string, vts, pgcn int) int {
	var list []wittydvd.PGCEntry
	switch domain {
	case "vmgm":
		list = o.Disc.VMG.Menus[0].PGCs
	case "vtsm":
		list = o.Disc.VTS[vts].Menus[0].PGCs
	case "vts":
		list = o.Disc.VTS[vts].PGCs
	case "fp":
		list = []wittydvd.PGCEntry{{PGC: o.Disc.VMG.FirstPlay}}
		pgcn = 1
	}
	if pgcn < 1 || pgcn > len(list) {
		fmt.Println("no such pgc")
		return 1
	}
	p := list[pgcn-1].PGC
	fmt.Printf("%s vts=%d pgc=%d cells=%d programs=%v next=%d prev=%d up=%d still=%d dur=%.1fs\n", domain, vts, pgcn, len(p.Cells), p.Programs, p.NextPGCN, p.PrevPGCN, p.GoUpPGCN, p.StillTime, p.Duration)
	for i, c := range p.Pre {
		fmt.Printf("  pre  %2d: %s\n", i+1, wittydvd.Decode(c))
	}
	for i, c := range p.Cells {
		fmt.Printf("  cell %2d: sectors %d..%d dur=%.2fs still=%d cmd=%d block=%d/%d\n", i+1, c.FirstSector, c.LastSector, c.Duration, c.StillTime, c.Command, c.BlockMode, c.BlockType)
	}
	for i, c := range p.CellCmds {
		fmt.Printf("  cellcmd %2d: %s\n", i+1, wittydvd.Decode(c))
	}
	for i, c := range p.Post {
		fmt.Printf("  post %2d: %s\n", i+1, wittydvd.Decode(c))
	}
	return 0
}
