package wittydvd

import "math"

// Title runs.
//
// A feature is usually one recording cut into cells at its chapters: the cells
// follow each other on the disc, in one VOB, and their timestamps carry on from
// one to the next. The cut falls between pictures, not between sounds: the first
// VOBU of a cell also holds the sound of the last quarter of a second or so,
// which was stored ahead of its picture, and an audio frame can lie across the
// cut. A player that reads on notices none of this. Reading each cell as an input
// of its own does: the sound frame across the cut is lost, and an input is put on
// the timeline by its earliest packet, which is that early sound, so the picture
// lands late by a different amount in every cell. Frames are then repeated or
// dropped at each chapter. So a title is read in runs: stretches with nothing to
// put together, each placed on the timeline by its first picture.

// TitleRun is a stretch of a title that can be read straight through.
type TitleRun struct {
	Extents []FileExtent
	// Duration is how long the run plays, in seconds.
	Duration float64
	// StartPTS is the presentation time of its first picture (90 kHz); 0 when
	// the disc doesn't say.
	StartPTS uint32
	// Lead is how long before its first picture the run's earliest packet lies,
	// in seconds: the sound stored ahead of the picture (see scanLead).
	Lead float64
	// FirstCell and Cells say which of the given cells it plays.
	FirstCell, Cells int
}

const (
	navSectors = 1 // a NAV pack is one sector, the first of its VOBU
	// leadSectors is how much of a run's start is searched for its earliest
	// packet: every stream shows up well within a second of a DVD's bit rate.
	leadSectors = 512
	// runPTSGap is how far (90 kHz) the times of two cells may differ for one to
	// count as carrying on from the other: a frame or two of authoring slack.
	runPTSGap = 9000
)

// navPTM reads the NAV pack at a sector, if there is one.
func navPTM(rd SectorReader, sector uint32) *PCI {
	buf, err := rd.ReadSectors(sector, navSectors)
	if err != nil || len(buf) < Sector {
		return nil
	}
	if n := ScanSectors(buf, sector).Navs; len(n) > 0 {
		return n[0].PCI
	}
	return nil
}

// scanLead reads a stretch's first packets for how long before the given first
// picture (90 kHz) the earliest of them lies. Sound counts, and sub-pictures only
// when they lie just ahead of it, which is how a demuxer places an input.
func scanLead(buf []byte, first uint32) float64 {
	seen := map[int]bool{}
	var earliest, sub int64 = -1, -1
	for off := 0; off+Sector <= len(buf); off += Sector {
		for _, p := range packPackets(buf[off : off+Sector]) {
			if p.pts < 0 {
				continue
			}
			key := int(p.id)<<8 | (p.sub & 0xff)
			if seen[key] {
				continue
			}
			seen[key] = true
			switch {
			case p.id == 0xbd && p.sub >= 0x20 && p.sub <= 0x3f:
				if sub < 0 || p.pts < sub {
					sub = p.pts
				}
			case earliest < 0 || p.pts < earliest:
				earliest = p.pts
			}
		}
	}
	if sub >= 0 && (earliest < 0 || (sub < earliest && earliest-sub < 90000)) {
		earliest = sub
	}
	if earliest < 0 || earliest >= int64(first) {
		return 0
	}
	return float64(int64(first)-earliest) / 90000
}

// TitleRuns groups the cells of a title (see Disc.Title) into runs. Cells join a
// run when they follow each other on the disc in one VOB and their times carry
// on; interleaved cells (angles, seamless branching) and anything the disc marks
// as a new time base stand alone.
func (o *Opened) TitleRuns(sp Space, cells []TitleCell) ([]TitleRun, error) {
	rd, err := o.Reader(sp)
	if err != nil {
		return nil, err
	}
	return titleRuns(cells, func(c Cell) ([]FileExtent, error) { return o.CellExtents(sp, c) }, func(c Cell) (start, end uint32) {
		if rs, err := o.CellRanges(sp, c); err == nil && len(rs) > 0 {
			if pci := navPTM(rd, rs[0].First); pci != nil {
				start = pci.VOBUStartPTM
			}
		}
		if plainCell(c) && c.LastVOBUStart >= c.FirstSector {
			if pci := navPTM(rd, c.LastVOBUStart); pci != nil {
				end = pci.VOBUEndPTM
			}
		}
		return start, end
	}, func(c Cell, start uint32) float64 {
		rs, err := o.CellRanges(sp, c)
		if err != nil || len(rs) == 0 || start == 0 {
			return 0
		}
		count := min(leadSectors, int(rs[0].Last-rs[0].First)+1)
		buf, err := rd.ReadSectors(rs[0].First, count)
		if err != nil {
			return 0
		}
		return scanLead(buf, start)
	})
}

func plainCell(c Cell) bool { return !c.Interleaved && c.BlockType != 1 }

// titleRuns is TitleRuns with the disc reads handed in: times gives the time of
// a cell's first picture and of its end (90 kHz), 0 when the disc doesn't say.
func titleRuns(cells []TitleCell, extents func(Cell) ([]FileExtent, error), times func(Cell) (start, end uint32), lead func(c Cell, start uint32) float64) ([]TitleRun, error) {
	var out []TitleRun
	var prev Cell      // the last cell of the run being built
	var endPTS uint32  // where it ends
	var ifo float64    // the disc's own length for the run so far
	finish := func() { // settles the run's length once its last cell is known
		if len(out) == 0 {
			return
		}
		r := &out[len(out)-1]
		r.Duration = ifo
		// The timestamps are what the pictures keep to, and an NTSC cell's
		// stated length is only close. An end that disagrees widely is no use
		// (a cell may end in VOBUs with no picture): the stated length stands.
		if r.StartPTS > 0 && endPTS > r.StartPTS {
			pts := float64(endPTS-r.StartPTS) / 90000
			if d := math.Abs(pts - ifo); d < 1 && d < ifo/100 {
				r.Duration = pts
			}
		}
	}
	for i, c := range cells {
		ext, err := extents(c.Cell)
		if err != nil {
			return nil, err
		}
		if len(ext) == 0 {
			continue // nothing to read, so nothing on the timeline either
		}
		start, end := times(c.Cell)
		join := len(out) > 0 && plainCell(c.Cell) && plainCell(prev) && !c.Cell.STCDiscontinuity &&
			prev.VOBID == c.Cell.VOBID && prev.LastSector+1 == c.Cell.FirstSector
		if join && endPTS > 0 && start > 0 && (start > endPTS+runPTSGap || start+runPTSGap < endPTS) {
			join = false // the next picture isn't where this one ended
		}
		if join {
			r := &out[len(out)-1]
			last := &r.Extents[len(r.Extents)-1]
			if last.File == ext[0].File && last.Offset+last.Length == ext[0].Offset {
				last.Length += ext[0].Length
				ext = ext[1:]
			}
			r.Extents = append(r.Extents, ext...)
			r.Cells = i - r.FirstCell + 1
			ifo += c.Cell.Duration
		} else {
			finish()
			out = append(out, TitleRun{Extents: append([]FileExtent(nil), ext...), StartPTS: start, Lead: lead(c.Cell, start), FirstCell: i, Cells: 1})
			ifo = c.Cell.Duration
		}
		prev, endPTS = c.Cell, end
	}
	finish()
	return out, nil
}
