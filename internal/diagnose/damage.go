package diagnose

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"
	"time"
)

// The damage scan reads everything on the disc from start to end, once. It works
// on raw bytes, and looks at structure only: which sectors can't be read, and
// whether the stream's framing is where it should be. Bytes of a scrambled stream
// are read the same way and never decrypted, and nothing read is kept.

const (
	scanChunk     = 1 << 20
	scanSector    = 2048
	tsUnit        = 192
	maxSpots      = 50
	zeroRunMin    = 16      // sectors of nothing: 32 KB. A disc's video never has that.
	maxUnreadable = 1 << 20 // sectors. Past this (2 GB), carrying on tells nothing new.
)

// damageOptions shape a scan.
type damageOptions struct {
	from float64 // where on the progress bar it starts
	// where says what a place in a file is used for ("the main title").
	where func(name string, off int64) string
	// packs: check VOB packs. scrambled and ts say what the streams are.
	packs     bool
	scrambled bool
	ts        bool
}

type fileKind int

const (
	kindOther fileKind = iota
	kindVOB
	kindTS
)

func kindOf(name string) fileKind {
	up := strings.ToUpper(name)
	switch {
	case strings.HasSuffix(up, ".VOB"):
		return kindVOB
	case strings.Contains(up, "/STREAM/") && !strings.Contains(up, "/SSIF/") &&
		(strings.HasSuffix(up, ".M2TS") || strings.HasSuffix(up, ".MTS") || strings.HasSuffix(up, ".M2T")):
		return kindTS
	}
	return kindOther
}

// spot is a stretch of something wrong in a file.
type spot struct {
	file  string
	where string
	at    int64
	count int64
}

// tally keeps the first few spots, merged when they touch, and counts the rest.
type tally struct {
	total     int64
	stretches int64
	spots     []spot
	lastFile  string
	lastEnd   int64
}

func (t *tally) add(file string, at, count int64, where func() string) {
	t.total += count
	if file == t.lastFile && at == t.lastEnd {
		t.lastEnd = at + count
		if n := len(t.spots); n > 0 && t.spots[n-1].file == file && t.spots[n-1].at+t.spots[n-1].count == at {
			t.spots[n-1].count += count
		}
		return
	}
	t.stretches++
	t.lastFile, t.lastEnd = file, at+count
	if len(t.spots) < maxSpots {
		t.spots = append(t.spots, spot{file: file, where: where(), at: at, count: count})
	}
}

func (t *tally) lines(unit string) []string {
	out := make([]string, 0, len(t.spots))
	for _, s := range t.spots {
		line := fmt.Sprintf("%s, at %s %d: %s", path.Base(s.file), unit, s.at, plural(int(s.count), unit, unit+"s"))
		if s.where != "" {
			line += " (" + s.where + ")"
		}
		out = append(out, line)
	}
	return out
}

// region is where a file's data lies in what's being read.
type region struct {
	start, end int64
	logical    int64 // where in the file this stretch starts
	name       string
	kind       fileKind
}

// imageRegions turns files' extents into stretches of an image in disc order.
// Files on crafted discs can overlap: the first to claim a byte keeps it.
func imageRegions(files []discFile, size int64) []region {
	var out []region
	for i := range files {
		f := &files[i]
		var logical int64
		for _, e := range f.Exts {
			end := min(e.Offset+e.Length, size)
			if e.Offset < size && end > e.Offset {
				out = append(out, region{start: e.Offset, end: end, logical: logical, name: f.Name, kind: kindOf(f.Name)})
			}
			logical += e.Length
		}
	}
	slices.SortStableFunc(out, func(a, b region) int {
		switch {
		case a.start < b.start:
			return -1
		case a.start > b.start:
			return 1
		}
		return 0
	})
	var kept []region
	var edge int64
	for _, r := range out {
		if r.end <= edge {
			continue
		}
		if r.start < edge {
			r.logical += edge - r.start
			r.start = edge
		}
		kept = append(kept, r)
		edge = r.end
	}
	return kept
}

type damageScan struct {
	ctx  context.Context
	j    *Job
	o    damageOptions
	buf  []byte
	zero []byte

	done, total int64
	gaveUp      bool
	bytesRead   int64

	unread, zeros, packs, unsync tally

	zeroFile  string
	zeroStart int64
	zeroLen   int64
	cursor    int
	regions   []region
}

func (s *damageScan) whereFn(name string, off int64) func() string {
	return func() string {
		if s.o.where == nil {
			return ""
		}
		return s.o.where(name, off)
	}
}

// endZeroRun records a run of empty sectors once it's over.
func (s *damageScan) endZeroRun() {
	if s.zeroLen >= zeroRunMin {
		s.zeros.add(s.zeroFile, s.zeroStart, s.zeroLen, s.whereFn(s.zeroFile, s.zeroStart*scanSector))
	}
	s.zeroLen = 0
}

// feed looks at data from a file, which starts at a place in the file.
func (s *damageScan) feed(name string, kind fileKind, logical int64, data []byte) {
	if kind == kindOther {
		return
	}
	if kind == kindTS && !s.o.scrambled {
		// A unit's sync byte is 4 bytes in, after its timestamp. In a scrambled
		// stream most of them are noise, so it's checked only on clear ones.
		i := int((tsUnit + 4 - logical%tsUnit) % tsUnit)
		for ; i < len(data); i += tsUnit {
			if data[i] != 0x47 {
				s.unsync.add(name, (logical+int64(i))/tsUnit, 1, s.whereFn(name, logical+int64(i)))
			}
		}
	}
	// Sector by sector: whole empty sectors, and on a DVD each sector's pack start.
	i := int((scanSector - logical%scanSector) % scanSector)
	for ; i+scanSector <= len(data); i += scanSector {
		sector := (logical + int64(i)) / scanSector
		b := data[i : i+scanSector]
		if bytes.Equal(b, s.zero) {
			if s.zeroLen > 0 && (s.zeroFile != name || s.zeroStart+s.zeroLen != sector) {
				s.endZeroRun()
			}
			if s.zeroLen == 0 {
				s.zeroFile, s.zeroStart = name, sector
			}
			s.zeroLen++
			continue
		}
		if s.zeroLen > 0 {
			s.endZeroRun()
		}
		if kind == kindVOB && s.o.packs && (b[0] != 0 || b[1] != 0 || b[2] != 1 || b[3] != 0xba) {
			s.packs.add(name, sector, 1, s.whereFn(name, sector*scanSector))
		}
	}
}

// feedImage shares a chunk of an image out to the files it holds.
func (s *damageScan) feedImage(off int64, data []byte) {
	end := off + int64(len(data))
	for s.cursor < len(s.regions) && s.regions[s.cursor].end <= off {
		s.cursor++
	}
	for i := s.cursor; i < len(s.regions); i++ {
		r := &s.regions[i]
		if r.start >= end {
			break
		}
		a, b := max(off, r.start), min(end, r.end)
		s.feed(r.name, r.kind, r.logical+a-r.start, data[a-off:b-off])
	}
}

func (s *damageScan) badImageSector(off int64) {
	i, found := slices.BinarySearchFunc(s.regions, off, func(r region, at int64) int {
		switch {
		case r.end <= at:
			return -1
		case r.start > at:
			return 1
		}
		return 0
	})
	if !found {
		s.unread.add("outside any file", off/scanSector, 1, func() string { return "" })
		return
	}
	r := &s.regions[i]
	logical := r.logical + off - r.start
	s.unread.add(r.name, logical/scanSector, 1, s.whereFn(r.name, logical))
}

// readAll reads a stretch of an open file in order. A chunk that fails is read
// again a sector at a time, to find out exactly what can't be read.
func (s *damageScan) readAll(h io.ReaderAt, start, end int64, onData func(off int64, b []byte), onBad func(off int64)) {
	for off := start; off < end; {
		if s.ctx.Err() != nil || s.gaveUp {
			return
		}
		began := time.Now()
		n := min(int64(scanChunk), end-off)
		got, err := h.ReadAt(s.buf[:n], off)
		switch {
		case err == nil || int64(got) == n || errors.Is(err, io.EOF):
			if got > 0 {
				s.bytesRead += int64(got)
				onData(off, s.buf[:got])
			}
			if int64(got) < n {
				s.done += end - off
				return // the file is shorter than it should be: reported from sizes
			}
		default:
			for at := off; at < off+n; at += scanSector {
				if s.ctx.Err() != nil {
					return
				}
				m := min(int64(scanSector), off+n-at)
				g, e := h.ReadAt(s.buf[:m], at)
				if e != nil && int64(g) < m && !errors.Is(e, io.EOF) {
					onBad(at)
					continue
				}
				if g > 0 {
					s.bytesRead += int64(g)
					onData(at, s.buf[:g])
				}
			}
			if s.unread.total >= maxUnreadable {
				s.gaveUp = true
			}
		}
		off += n
		s.done += n
		if s.total > 0 {
			s.j.Progress(s.o.from + (1-s.o.from)*0.98*float64(s.done)/float64(s.total))
		}
		if !s.j.Rest(s.ctx, time.Since(began)) {
			return
		}
	}
}

// scanDamage reads the whole disc and reports what it found.
func scanDamage(ctx context.Context, j *Job, src *discSource, o damageOptions) {
	j.Step("Reading the whole disc to look for damage", o.from)
	s := &damageScan{ctx: ctx, j: j, o: o, buf: make([]byte, scanChunk), zero: make([]byte, scanSector)}
	began := time.Now()
	var short int64
	var claim int64
	var past []string
	opened := 0
	unopened := 0
	if src.IsDir {
		for i := range src.Files {
			s.total += src.Files[i].Size
		}
		for i := range src.Files {
			f := &src.Files[i]
			if ctx.Err() != nil || s.gaveUp {
				break
			}
			h, err := os.Open(f.Real)
			if err != nil {
				unopened++
				s.unread.add(f.Name, 0, max(f.Size/scanSector, 1), func() string { return "" })
				s.done += f.Size
				continue
			}
			opened++
			kind := kindOf(f.Name)
			s.readAll(h, 0, f.Size, func(off int64, b []byte) { s.feed(f.Name, kind, off, b) },
				func(off int64) { s.unread.add(f.Name, off/scanSector, 1, s.whereFn(f.Name, off)) })
			s.endZeroRun()
			_ = h.Close() // only read from
		}
	} else {
		h, err := os.Open(src.Path)
		if err != nil {
			j.Add(Finding{Severity: SevProblem, Title: "The disc image can't be opened for reading", What: "The server got this answer: " + plainErr(err) + "."})
			return
		}
		defer h.Close()
		s.total = src.Size
		s.regions = imageRegions(src.Files, src.Size)
		claim = volumeSectors(h, src.Size)
		if have := src.Size / scanSector; claim > have {
			short = claim - have
		}
		for i := range src.Files {
			f := &src.Files[i]
			var far int64
			for _, e := range f.Exts {
				far = max(far, e.Offset+e.Length)
			}
			if far > src.Size {
				past = append(past, path.Base(f.Name))
			}
		}
		s.readAll(h, 0, src.Size, s.feedImage, s.badImageSector)
		s.endZeroRun()
	}
	if ctx.Err() != nil {
		j.Add(Finding{Severity: SevInfo, Title: "The damage scan stopped early", What: fmt.Sprintf("You stopped it after reading %s of %s. Nothing can be said about the rest.", bytesText(s.done), bytesText(s.total)), Details: s.damageDetails()})
		return
	}
	bad := false
	if short > 0 {
		bad = true
		j.Add(Finding{Severity: SevProblem, Title: "This disc image is incomplete",
			What: fmt.Sprintf("Its own file system says it has %s, but the file holds only %s. The copy stopped early, or the file was cut short.", bytesText(claim*scanSector), bytesText(src.Size)),
			Fix:  "Make a new copy of the disc, and check there's room for all of it on the drive."})
	}
	if len(past) > 0 {
		bad = true
		sev := SevWarn
		what := "These files run past the end of the image. Files with impossible sizes are sometimes added on purpose as copy protection; if the disc plays, ignore this."
		if short > 0 {
			sev = SevProblem
			what = "These files run past the end of the image, because the copy is incomplete. They won't play right."
		}
		list, more := clip(past, len(past), maxSpots)
		j.Add(Finding{Severity: sev, Title: plural(len(past), "file runs past the end of the image", "files run past the end of the image"), What: what, Details: list, More: more})
	}
	if s.unread.total > 0 {
		bad = true
		what := fmt.Sprintf("%s (%s) couldn't be read at all. Those parts of the picture or the menus won't play.", plural(int(s.unread.total), "sector", "sectors"), bytesText(s.unread.total*scanSector))
		if s.gaveUp {
			what += " So much failed that the scan stopped early."
		}
		j.Add(Finding{Severity: SevProblem, Title: "Parts of this disc can't be read", What: what,
			Fix:     "This is usually the drive itself. Copy what you can to another drive soon and check the drive under Storage. For an optical disc, clean it and try again.",
			Details: s.unread.lines("sector"), More: int(s.unread.stretches) - len(s.unread.spots)})
	}
	if s.zeros.total > 0 {
		bad = true
		j.Add(Finding{Severity: SevWarn, Title: "Long stretches of the video are empty",
			What:    fmt.Sprintf("%s hold nothing but zeros, in %s. Copying tools often write zeros where they couldn't read the disc, so a picture may break up there. Some discs carry filler in places that never play, so what matters is whether it's used.", plural(int(s.zeros.total), "sector", "sectors"), plural(int(s.zeros.stretches), "stretch", "stretches")),
			Fix:     "If it plays badly at those places, make a new copy from a clean disc.",
			Details: s.zeros.lines("sector"), More: int(s.zeros.stretches) - len(s.zeros.spots)})
	}
	if s.packs.total > 0 {
		bad = true
		j.Add(Finding{Severity: SevWarn, Title: "Parts of the DVD video aren't in the right format",
			What:    fmt.Sprintf("%s of the video files don't start the way MPEG data must (in %s). A broken copy looks like this, and so does the filler some copy protection adds.", plural(int(s.packs.total), "sector", "sectors"), plural(int(s.packs.stretches), "stretch", "stretches")),
			Fix:     "If it plays badly at those places, make a new copy.",
			Details: s.packs.lines("sector"), More: int(s.packs.stretches) - len(s.packs.spots)})
	}
	if s.unsync.total > 0 {
		bad = true
		j.Add(Finding{Severity: SevWarn, Title: "Parts of the Blu-ray video have lost their rhythm",
			What:    fmt.Sprintf("%s of the stream files don't carry the mark each one must, in %s. That means a damaged or badly made copy.", plural(int(s.unsync.total), "packet", "packets"), plural(int(s.unsync.stretches), "stretch", "stretches")),
			Fix:     "If it plays badly at those places, make a new copy.",
			Details: s.unsync.lines("packet"), More: int(s.unsync.stretches) - len(s.unsync.spots)})
	}
	if !bad {
		took := time.Since(began)
		det := s.damageDetails()
		if took > time.Second {
			det = append(det, fmt.Sprintf("Read at %.0f MB/s", float64(s.bytesRead)/(1<<20)/took.Seconds()))
		}
		j.Add(Finding{Severity: SevOK, Title: "No damage found", What: "Every sector could be read, and the video has the structure it should.", Details: det})
	}
}

func (s *damageScan) damageDetails() []string {
	return []string{bytesText(s.bytesRead) + " read"}
}
