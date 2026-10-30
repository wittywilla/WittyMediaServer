package wittydvd

import (
	"encoding/binary"
	"testing"
)

// navSector builds a NAV pack: pack header, then the PCI and DSI private
// stream 2 packets.
func navSector(pci, dsi []byte) []byte {
	b := make([]byte, Sector)
	copy(b, []byte{0, 0, 1, 0xba, 0x44})
	p := 14
	put := func(sub byte, data []byte) {
		copy(b[p:], []byte{0, 0, 1, 0xbf})
		binary.BigEndian.PutUint16(b[p+4:], uint16(len(data)+1))
		b[p+6] = sub
		copy(b[p+7:], data)
		p += 7 + len(data)
	}
	put(0, pci)
	put(1, dsi)
	return b
}

type sectors [][]byte

func (s sectors) ReadSectors(first uint32, count int) ([]byte, error) {
	var out []byte
	for i := 0; i < count && int(first)+i < len(s); i++ {
		out = append(out, s[int(first)+i]...)
	}
	return out, nil
}

func TestNavPackFields(t *testing.T) {
	pci := make([]byte, pciMin)
	binary.BigEndian.PutUint32(pci[0x0c:], 1000)
	binary.BigEndian.PutUint32(pci[0x10:], 46000)
	binary.BigEndian.PutUint32(pci[0x14:], 4600) // a still picture ends the sequence
	dsi := make([]byte, 0x3fa)
	binary.BigEndian.PutUint32(dsi[8:], 40)
	binary.BigEndian.PutUint32(dsi[dsiNextVideo:], 0xbfffffff)
	sc := ScanSectors(navSector(pci, dsi), 0)
	if len(sc.Navs) != 1 {
		t.Fatalf("navs: %+v", sc.Navs)
	}
	n := sc.Navs[0]
	if n.PCI.SeqEndPTM != 4600 || n.DSI.VOBUEnd != 40 || n.DSI.FirstRefEnd != 0 || !n.DSI.NoVideoAhead {
		t.Fatalf("PCI %+v DSI %+v", n.PCI, n.DSI)
	}
}

func TestHighlightTiming(t *testing.T) {
	// One VOBU of a menu cell: buttons from 0.5 s, selection ends at 8 s
	// with button 2 forced, highlight ends at 10 s.
	const start = 90000
	pci := make([]byte, pciMin)
	binary.BigEndian.PutUint32(pci[0x0c:], start)
	binary.BigEndian.PutUint32(pci[0x10:], start+90000)
	pci[pciHLISS+1] = 1
	binary.BigEndian.PutUint32(pci[pciHLISPTM:], start+45000)
	binary.BigEndian.PutUint32(pci[pciHLIEPTM:], start+10*90000)
	binary.BigEndian.PutUint32(pci[pciBTNSEPTM:], start+8*90000)
	pci[pciBTNMD] = 0x10
	pci[pciBTNNS] = 2
	pci[pciFOACBTNN] = 2
	for i := 0; i < 2; i++ {
		o := pciBTNIT + i*btnSize
		pci[o+2], pci[o+5] = 100, 100                    // x 0..100, y 0..100
		copy(pci[o+10:], []byte{0x30, 0x02, 0, 0, 0, 1}) // JumpTT 1
	}
	dsi := make([]byte, 0x3fa)
	pic, err := ScanMenu(sectors{navSector(pci, dsi)}, 0, 0, MenuScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pic.Buttons) != 2 || pic.ButtonsFromSec != 0.5 || pic.ButtonsUntilSec != 8 || pic.ForcedAction != 2 || pic.ActionAtSec != 8 {
		t.Fatalf("timing: from %.2f until %.2f forced %d at %.2f (%d buttons)", pic.ButtonsFromSec, pic.ButtonsUntilSec, pic.ForcedAction, pic.ActionAtSec, len(pic.Buttons))
	}

	// "Until the end" (0xffffffff) leaves the buttons up for the whole cell.
	binary.BigEndian.PutUint32(pci[pciHLIEPTM:], 0xffffffff)
	binary.BigEndian.PutUint32(pci[pciBTNSEPTM:], 0xffffffff)
	pic, _ = ScanMenu(sectors{navSector(pci, dsi)}, 0, 0, MenuScanOptions{})
	if pic.ButtonsUntilSec != 0 || pic.ForcedAction != 0 {
		t.Fatalf("open-ended: until %.2f forced %d", pic.ButtonsUntilSec, pic.ForcedAction)
	}
}

func TestButtonGroups(t *testing.T) {
	// Two groups: 16:9 (type 1) and letterbox (type 2), 18 buttons each. The
	// player shows the whole 16:9 frame, so it uses the first group.
	pci := make([]byte, pciMin)
	pci[pciHLISS+1] = 1
	binary.BigEndian.PutUint16(pci[pciBTNMD:], 2<<12|1<<8|2<<4)
	pci[pciBTNNS] = 1
	pci[pciBTNIT+2] = 200                    // group 1: x 0..200
	pci[pciBTNIT+18*btnSize+2] = 150         // group 2: x 0..150
	copy(pci[pciBTNIT+10:], []byte{0x30, 2}) // a command, so the button is not inert
	copy(pci[pciBTNIT+18*btnSize+10:], []byte{0x30, 2})
	p := ParsePCI(pci)
	if len(p.Groups) != 2 || p.Groups[0].Mode != "normal" || p.Groups[1].Mode != "letterbox" || len(p.Buttons) != 2 {
		t.Fatalf("groups %+v buttons %d", p.Groups, len(p.Buttons))
	}
	wide := VideoAttr{Wide: true}
	pic, _ := ScanMenu(sectors{navSector(pci, make([]byte, 0x3fa))}, 0, 0, MenuScanOptions{Video: wide, Wide: true})
	if len(pic.Buttons) != 1 || pic.Buttons[0].X1 != 200 {
		t.Fatalf("16:9 group: %+v", pic.Buttons)
	}
	pic, _ = ScanMenu(sectors{navSector(pci, make([]byte, 0x3fa))}, 0, 0, MenuScanOptions{Video: wide})
	if len(pic.Buttons) != 1 || pic.Buttons[0].X1 != 150 {
		t.Fatalf("letterbox group: %+v", pic.Buttons)
	}
}
