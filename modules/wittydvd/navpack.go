package wittydvd

import (
	"encoding/binary"
)

const packStart = 0x000001ba

// Button is one highlight button from a PCI.
type Button struct {
	N           int     `json:"n"`
	Group       int     `json:"group"`
	X0, Y0      int     `json:"-"`
	X1, Y1      int     `json:"-"`
	Up, Down    int     `json:"-"`
	Left, Right int     `json:"-"`
	Auto        bool    `json:"auto"`
	Cmd         Command `json:"-"`
	ColorScheme int     `json:"-"`
	Select      Look    `json:"-"`
	Action      Look    `json:"-"`
}

// Look is a color+contrast table for pixel codes 0..3.
type Look struct {
	Color    [4]int
	Contrast [4]int
}

// ButtonGroup describes one of up to three button groups (per display mode).
type ButtonGroup struct {
	N    int    `json:"n"`
	Type int    `json:"type"`
	Mode string `json:"mode"` // normal | letterbox | panscan
}

// PCI is the parsed presentation control information of a NAV pack.
type PCI struct {
	NavLBN         uint32
	VOBUStartPTM   uint32
	VOBUEndPTM     uint32
	HLStatus       int // 0 none, 1 new, 2 same, 3 same but commands differ
	HLStartPTM     uint32
	HLEndPTM       uint32
	SelectEndPTM   uint32
	Groups         []ButtonGroup
	ButtonCount    int
	FirstButton    int
	NumericButtons int
	ForcedSelect   int
	ForcedAction   int
	Buttons        []Button
	Schemes        [3]struct{ Select, Action Look }
}

// DSI is the parsed data search information of a NAV pack.
type DSI struct {
	SCR     uint32
	LBN     uint32
	VOBUEnd uint32 // relative end address: VOBU spans LBN..LBN+VOBUEnd
	VOBID   int
	CellID  int
}

// packet is a PES packet pulled out of a pack.
type packet struct {
	id   byte
	sub  int // substream id for 0xbd/0xbf, else -1
	data []byte
	pts  int64 // -1 when absent
}

func readPTS(b []byte) int64 {
	return int64(b[0]>>1&7)<<30 | int64(binary.BigEndian.Uint16(b[1:])>>1)<<15 | int64(binary.BigEndian.Uint16(b[3:])>>1)
}

// PackScrambled reports CSS scrambling on the first PES of a pack.
func PackScrambled(pack []byte) bool {
	if len(pack) < 20 || binary.BigEndian.Uint32(pack) != packStart || pack[4]&0xc0 != 0x40 {
		return false
	}
	p := 14 + int(pack[13]&7)
	for p+9 <= len(pack) {
		if pack[p] != 0 || pack[p+1] != 0 || pack[p+2] != 1 {
			return false
		}
		id := pack[p+3]
		l := int(binary.BigEndian.Uint16(pack[p+4:]))
		if id == 0xbb || id == 0xbe || id == 0xbf { // system header, padding, nav: no scrambling flag
			p += 6 + l
			continue
		}
		return pack[p+6]&0x30 != 0
	}
	return false
}

func packPackets(pack []byte) []packet {
	if len(pack) < 14 || binary.BigEndian.Uint32(pack) != packStart || pack[4]&0xc0 != 0x40 {
		return nil
	}
	var out []packet
	p := 14 + int(pack[13]&7)
	for p+6 <= len(pack) {
		if pack[p] != 0 || pack[p+1] != 0 || pack[p+2] != 1 {
			break
		}
		id := pack[p+3]
		if id == 0xb9 || id == 0xba {
			break
		}
		end := p + 6 + int(binary.BigEndian.Uint16(pack[p+4:]))
		if end > len(pack) {
			end = len(pack)
		}
		switch {
		case id == 0xbf:
			if p+7 <= end {
				out = append(out, packet{id: id, sub: int(pack[p+6]), data: pack[p+7 : end], pts: -1})
			}
		case id == 0xbd || (id >= 0xc0 && id <= 0xef):
			if p+9 > end {
				break
			}
			flags := pack[p+7]
			start := p + 9 + int(pack[p+8])
			pts := int64(-1)
			if flags&0x80 != 0 && p+14 <= end {
				pts = readPTS(pack[p+9:])
			}
			if start < end {
				if id == 0xbd {
					out = append(out, packet{id: id, sub: int(pack[start]), data: pack[start+1 : end], pts: pts})
				} else {
					out = append(out, packet{id: id, sub: -1, data: pack[start:end], pts: pts})
				}
			}
		}
		p = end
	}
	return out
}

func nibbles(b []byte) [4]int {
	// Byte layout: hi byte = codes 3,2 ; lo byte = codes 1,0 (each a nibble, high nibble first).
	return [4]int{int(b[1] & 0x0f), int(b[1] >> 4), int(b[0] & 0x0f), int(b[0] >> 4)}
}

func parseLook(b []byte) Look {
	return Look{Color: nibbles(b[0:2]), Contrast: nibbles(b[2:4])}
}

const (
	pciHLISS    = 0x60
	pciHLISPTM  = 0x62
	pciHLIEPTM  = 0x66
	pciBTNSEPTM = 0x6a
	pciBTNMD    = 0x6e
	pciBTNOFN   = 0x70
	pciBTNNS    = 0x71
	pciNSLBTNNS = 0x72
	pciFOSLBTNN = 0x74
	pciFOACBTNN = 0x75
	pciBTNCOLIT = 0x76
	pciBTNIT    = 0x8e
	btnSize     = 18
	maxButtons  = 36
	pciMin      = pciBTNIT + maxButtons*btnSize
)

// ParsePCI parses a PCI substream payload.
func ParsePCI(b []byte) *PCI {
	if len(b) < pciMin {
		return nil
	}
	p := &PCI{
		NavLBN:         binary.BigEndian.Uint32(b[0:]),
		VOBUStartPTM:   binary.BigEndian.Uint32(b[0x0c:]),
		VOBUEndPTM:     binary.BigEndian.Uint32(b[0x10:]),
		HLStatus:       int(binary.BigEndian.Uint16(b[pciHLISS:]) & 3),
		HLStartPTM:     binary.BigEndian.Uint32(b[pciHLISPTM:]),
		HLEndPTM:       binary.BigEndian.Uint32(b[pciHLIEPTM:]),
		SelectEndPTM:   binary.BigEndian.Uint32(b[pciBTNSEPTM:]),
		FirstButton:    int(b[pciBTNOFN]),
		NumericButtons: int(b[pciNSLBTNNS]),
		ForcedSelect:   int(b[pciFOSLBTNN]),
		ForcedAction:   int(b[pciFOACBTNN]),
	}
	md := binary.BigEndian.Uint16(b[pciBTNMD:])
	groupCount := int(md>>12) & 3
	for g := 0; g < groupCount; g++ {
		typ := int(md>>(8-4*uint(g))) & 7
		mode := "normal"
		if typ&4 != 0 {
			mode = "panscan"
		} else if typ&2 != 0 {
			mode = "letterbox"
		}
		p.Groups = append(p.Groups, ButtonGroup{N: g + 1, Type: typ, Mode: mode})
	}
	for i := 0; i < 3; i++ {
		o := pciBTNCOLIT + i*8
		p.Schemes[i].Select = parseLook(b[o : o+4])
		p.Schemes[i].Action = parseLook(b[o+4 : o+8])
	}
	p.ButtonCount = int(b[pciBTNNS])
	if p.ButtonCount > maxButtons {
		p.ButtonCount = maxButtons
	}
	// Status 2 ("same as the previous VOBU") still carries a full copy of the
	// button table, and a cell that loops or starts mid-menu may never see a
	// status 1 pack, so parse the buttons whenever highlights are active.
	if p.HLStatus != 0 {
		perGroup := maxButtons
		if groupCount > 1 {
			perGroup = maxButtons / groupCount
		}
		groups := groupCount
		if groups < 1 {
			groups = 1
		}
		for g := 0; g < groups; g++ {
			for i := 0; i < p.ButtonCount && i < perGroup; i++ {
				o := pciBTNIT + (g*perGroup+i)*btnSize
				bb := b[o : o+btnSize]
				btn := Button{
					N: i + 1, Group: g + 1,
					X0: int(bb[0]&0x3f)<<4 | int(bb[1]>>4),
					X1: int(bb[1]&3)<<8 | int(bb[2]),
					Y0: int(bb[3]&0x3f)<<4 | int(bb[4]>>4),
					Y1: int(bb[4]&3)<<8 | int(bb[5]),
					Up: int(bb[6] & 0x3f), Down: int(bb[7] & 0x3f), Left: int(bb[8] & 0x3f), Right: int(bb[9] & 0x3f),
					Auto:        bb[3]>>6 != 0,
					ColorScheme: int(bb[0] >> 6),
				}
				copy(btn.Cmd[:], bb[10:18])
				if btn.ColorScheme >= 1 {
					btn.Select = p.Schemes[btn.ColorScheme-1].Select
					btn.Action = p.Schemes[btn.ColorScheme-1].Action
				}
				p.Buttons = append(p.Buttons, btn)
			}
		}
	}
	return p
}

// ParseDSI parses a DSI substream payload.
func ParseDSI(b []byte) *DSI {
	if len(b) < 0x20 {
		return nil
	}
	return &DSI{
		SCR:     binary.BigEndian.Uint32(b[0:]),
		LBN:     binary.BigEndian.Uint32(b[4:]),
		VOBUEnd: binary.BigEndian.Uint32(b[8:]),
		VOBID:   int(binary.BigEndian.Uint16(b[0x18:])),
		CellID:  int(b[0x1b]),
	}
}

// NavPack is the PCI+DSI found at the start of a VOBU.
type NavPack struct {
	Sector uint32 // relative to the scan start
	PCI    *PCI
	DSI    *DSI
}

// SPUUnit is a complete sub-picture unit with its presentation time.
type SPUUnit struct {
	Stream int
	Sector uint32
	PTS    int64
	Bytes  []byte
}

// StreamScan is the result of scanning a run of VOB sectors.
type StreamScan struct {
	Navs      []NavPack
	SPUs      []SPUUnit
	Scrambled bool
}

type openUnit struct {
	sector uint32
	pts    int64
	size   int
	parts  [][]byte
	have   int
}

// PackScanner walks MPEG-PS packs incrementally and pulls out NAV packs and
// complete SPU units. Units may span calls to Feed, so a long cell can be
// read in chunks without holding it all in memory.
type PackScanner struct {
	Out *StreamScan
	// Streams limits SPU collection to these sub-picture stream numbers
	// (bit n = stream n); 0 collects every stream.
	Streams uint32
	// NoNav skips PCI/DSI parsing except for the first NAV pack.
	NoNav bool
	open  map[int]*openUnit
}

// NewPackScanner returns a scanner with an empty result.
func NewPackScanner() *PackScanner {
	return &PackScanner{Out: &StreamScan{}, open: map[int]*openUnit{}}
}

func (ps *PackScanner) begin(stream int, sector uint32, pts int64, data []byte) {
	if len(data) < 2 {
		return
	}
	size := int(binary.BigEndian.Uint16(data))
	if size < 4 {
		return
	}
	u := &openUnit{sector: sector, pts: pts, size: size}
	ps.open[stream] = u
	ps.finish(stream, u, data, sector)
}

func (ps *PackScanner) finish(stream int, u *openUnit, data []byte, sector uint32) {
	want := u.size - u.have
	take := data
	if len(take) > want {
		take = data[:want]
	}
	// Copy: the caller's buffer is reused between chunks.
	u.parts = append(u.parts, append([]byte(nil), take...))
	u.have += len(take)
	if u.have < u.size {
		return
	}
	all := make([]byte, 0, u.size)
	for _, p := range u.parts {
		all = append(all, p...)
	}
	ps.Out.SPUs = append(ps.Out.SPUs, SPUUnit{Stream: stream, Sector: u.sector, PTS: u.pts, Bytes: all})
	delete(ps.open, stream)
	if rest := data[len(take):]; len(rest) > 0 {
		ps.begin(stream, sector, -1, rest)
	}
}

// Feed scans whole sectors of buf; firstSector labels the results.
func (ps *PackScanner) Feed(buf []byte, firstSector uint32) {
	for s := 0; (s+1)*Sector <= len(buf); s++ {
		pack := buf[s*Sector : (s+1)*Sector]
		sector := firstSector + uint32(s)
		if PackScrambled(pack) {
			ps.Out.Scrambled = true
			for k := range ps.open {
				delete(ps.open, k)
			}
			continue
		}
		var pci *PCI
		var dsi *DSI
		for _, pk := range packPackets(pack) {
			switch {
			case pk.id == 0xbf && pk.sub == 0:
				if !ps.NoNav || len(ps.Out.Navs) == 0 {
					pci = ParsePCI(pk.data)
				}
			case pk.id == 0xbf && pk.sub == 1:
				if !ps.NoNav || len(ps.Out.Navs) == 0 {
					dsi = ParseDSI(pk.data)
				}
			case pk.id == 0xbd && pk.sub >= 0x20 && pk.sub <= 0x3f:
				stream := pk.sub & 0x1f
				if ps.Streams != 0 && ps.Streams&(1<<uint(stream)) == 0 {
					continue
				}
				u := ps.open[stream]
				switch {
				case u != nil && pk.pts >= 0:
					delete(ps.open, stream)
					ps.begin(stream, sector, pk.pts, pk.data)
				case u != nil:
					ps.finish(stream, u, pk.data, sector)
				case pk.pts >= 0:
					ps.begin(stream, sector, pk.pts, pk.data)
				}
			}
		}
		if pci != nil || dsi != nil {
			ps.Out.Navs = append(ps.Out.Navs, NavPack{Sector: sector, PCI: pci, DSI: dsi})
		}
	}
}

// ScanSectors walks MPEG-PS packs and pulls out NAV packs and SPU units.
// firstSector is only used to label results.
func ScanSectors(buf []byte, firstSector uint32) *StreamScan {
	ps := NewPackScanner()
	ps.Feed(buf, firstSector)
	return ps.Out
}
