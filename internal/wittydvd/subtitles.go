package wittydvd

// Sub-pictures of title cells, extracted for client-side subtitle rendering.
// Rendering subtitles in the player (instead of burning them into the video)
// means switching or hiding them is instant and never restarts the stream,
// and forced subtitles can be shown on their own.

// CellSubs are the sub-picture units of one cell, all streams.
type CellSubs struct {
	// BasePTM is the presentation time of the cell's first VOBU; unit PTS
	// minus BasePTM is the offset into the cell.
	BasePTM uint32
	Units   []SPUUnit
}

type subsKey struct {
	sp          Space
	first, last uint32
}

const maxCachedSubCells = 96

// CellSubpictures reads a cell once and returns its sub-picture units
// (cached per disc). The read is sequential in 1 MiB chunks; the cost is a
// single pass over the cell's bytes.
func (o *Opened) CellSubpictures(sp Space, cell Cell) (*CellSubs, error) {
	first, last := cell.FirstSector, cell.LastSector
	k := subsKey{sp, first, last}
	o.mu.Lock()
	if o.subs == nil {
		o.subs = map[subsKey]*CellSubs{}
	}
	if c, ok := o.subs[k]; ok {
		o.mu.Unlock()
		return c, nil
	}
	o.mu.Unlock()
	rd, err := o.Reader(sp)
	if err != nil {
		return nil, err
	}
	// Interleaved cells (angle blocks) are read VOBU run by VOBU run so the
	// other angles' subtitles are not mixed in.
	ranges, err := o.CellRanges(sp, cell)
	if err != nil {
		return nil, err
	}
	ps := NewPackScanner()
	ps.NoNav = true
	const chunk = 512
	for _, r := range ranges {
		for at := r.First; at <= r.Last; at += chunk {
			n := chunk
			if at+uint32(n)-1 > r.Last {
				n = int(r.Last - at + 1)
			}
			buf, err := rd.ReadSectors(at, n)
			if err != nil {
				return nil, err
			}
			if len(buf) < Sector {
				break
			}
			ps.Feed(buf, at)
			if ps.Out.Scrambled {
				break
			}
		}
	}
	c := &CellSubs{Units: ps.Out.SPUs}
	for _, nv := range ps.Out.Navs {
		if nv.PCI != nil {
			c.BasePTM = nv.PCI.VOBUStartPTM
			break
		}
	}
	o.mu.Lock()
	if len(o.subs) >= maxCachedSubCells {
		for key := range o.subs { // drop an arbitrary entry
			delete(o.subs, key)
			break
		}
	}
	o.subs[k] = c
	o.mu.Unlock()
	return c, nil
}

// Purpose names an audio stream's code extension (empty for plain audio).
func (a AudioAttr) Purpose() string {
	switch a.Extension {
	case 2:
		return "Visually impaired"
	case 3:
		return "Commentary"
	case 4:
		return "Commentary 2"
	}
	return ""
}

// Purpose names a sub-picture stream's code extension.
func (s SubpAttr) Purpose() string {
	switch s.Extension {
	case 2:
		return "Large"
	case 3:
		return "Children"
	case 5:
		return "Captions"
	case 6:
		return "Large captions"
	case 7:
		return "Children's captions"
	case 9:
		return "Forced"
	case 13:
		return "Commentary"
	case 14:
		return "Large commentary"
	case 15:
		return "Children's commentary"
	}
	return ""
}
