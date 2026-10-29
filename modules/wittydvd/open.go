package wittydvd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/wittywilla/wittydvd/isofs"
)

// MaxIFOBytes bounds how much of an IFO we read.
const MaxIFOBytes = 16 << 20

// Space names a VOB address space.
type Space struct {
	Domain Domain
	VTS    int
}

// Source abstracts an ISO image or a VIDEO_TS folder.
type Source interface {
	ReadIFO(name string) ([]byte, error)
	// VOBExtents returns the byte extents (in the backing file(s)) of the VOB
	// address space, in order. Each extent names its file.
	VOBExtents(sp Space) ([]FileExtent, error)
	Close() error
	Describe() string
}

// FileExtent is a byte range in a named file.
type FileExtent struct {
	File   string `json:"file"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
}

// Opened is a DVD ready for navigation and playback.
type Opened struct {
	Disc   *Disc
	Source Source
	Label  string
	Format string

	mu      sync.Mutex
	spaces  map[Space][]FileExtent
	readers map[Space]*spaceReader
	scans   map[scanKey]*MenuPicture
	subs    map[subsKey]*CellSubs
	ranges  map[rangesKey][]SectorRange
}

// scanKey identifies a menu scan whose result does not depend on viewer
// state (the default button is applied afterwards with WithDefault).
type scanKey struct {
	sp          Space
	first, last uint32
	palette     [16]uint32
	video       VideoAttr
	subp        SubpControl
	hasSubp     bool
	wide        bool
	maxHops     int
}

// Open opens an ISO/IMG image or a VIDEO_TS directory.
func Open(p string) (*Opened, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	var src Source
	label := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	format := "folder"
	if st.IsDir() {
		src, err = openFolder(p)
	} else {
		var img *isofs.Disc
		img, err = isofs.Open(p)
		if err == nil {
			src = &imageSource{img: img}
			if img.Label != "" {
				label = img.Label
			}
			format = img.Format
		}
	}
	if err != nil {
		return nil, err
	}
	vmgBytes, err := src.ReadIFO("VIDEO_TS.IFO")
	if err != nil {
		src.Close()
		return nil, fmt.Errorf("dvd: %w", err)
	}
	vmg, err := ParseVMG(vmgBytes)
	if err != nil {
		src.Close()
		return nil, err
	}
	d := &Disc{VMG: vmg, VTS: map[int]*VTS{}}
	count := vmg.NumTitleSets
	for _, t := range vmg.Titles {
		if t.VTS > count {
			count = t.VTS
		}
	}
	if count > maxTitleSets {
		count = maxTitleSets
	}
	for n := 1; n <= count; n++ {
		b, err := src.ReadIFO(fmt.Sprintf("VTS_%02d_0.IFO", n))
		if err != nil {
			continue
		}
		v, err := ParseVTS(b, n)
		if err != nil {
			continue
		}
		d.VTS[n] = v
	}
	return &Opened{Disc: d, Source: src, Label: label, Format: format, spaces: map[Space][]FileExtent{}, readers: map[Space]*spaceReader{}, scans: map[scanKey]*MenuPicture{}}, nil
}

// Close releases the reader file handles and the source.
func (o *Opened) Close() error {
	o.mu.Lock()
	for _, r := range o.readers {
		r.Close()
	}
	o.readers = map[Space]*spaceReader{}
	o.mu.Unlock()
	return o.Source.Close()
}

// Extents returns the VOB address space extents (cached).
func (o *Opened) Extents(sp Space) ([]FileExtent, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.extentsLocked(sp)
}

func (o *Opened) extentsLocked(sp Space) ([]FileExtent, error) {
	if e, ok := o.spaces[sp]; ok {
		return e, nil
	}
	e, err := o.Source.VOBExtents(sp)
	if err != nil {
		return nil, err
	}
	o.spaces[sp] = e
	return e, nil
}

// SpaceFor maps a step to its VOB address space.
func SpaceFor(domain Domain, vts int) Space {
	switch domain {
	case DomainFP, DomainVMGM:
		return Space{Domain: DomainVMGM}
	case DomainVTSM:
		return Space{Domain: DomainVTSM, VTS: vts}
	}
	return Space{Domain: DomainVTS, VTS: vts}
}

// RangeExtents maps sectors [first,last] of a space onto file byte extents.
func (o *Opened) RangeExtents(sp Space, first, last uint32) ([]FileExtent, error) {
	all, err := o.Extents(sp)
	if err != nil {
		return nil, err
	}
	return sliceExtents(all, int64(first)*Sector, int64(last-first+1)*Sector), nil
}

func sliceExtents(all []FileExtent, off, length int64) []FileExtent {
	var out []FileExtent
	for _, e := range all {
		if length <= 0 {
			break
		}
		if off >= e.Length {
			off -= e.Length
			continue
		}
		take := e.Length - off
		if take > length {
			take = length
		}
		out = append(out, FileExtent{File: e.File, Offset: e.Offset + off, Length: take})
		length -= take
		off = 0
	}
	return out
}

// Reader returns a SectorReader over a space. Readers are shared and keep
// their file handles open until the disc is closed.
func (o *Opened) Reader(sp Space) (SectorReader, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if r, ok := o.readers[sp]; ok {
		return r, nil
	}
	ext, err := o.extentsLocked(sp)
	if err != nil {
		return nil, err
	}
	if len(ext) == 0 {
		return nil, errors.New("dvd: VOB missing")
	}
	r := &spaceReader{ext: ext, files: map[string]*os.File{}}
	o.readers[sp] = r
	return r, nil
}

// Menu scans a cell for its highlight information, caching the result per
// disc so revisiting a menu (or moving the highlight) never touches the disk.
func (o *Opened) Menu(sp Space, first, last uint32, opt MenuScanOptions) (*MenuPicture, error) {
	k := scanKey{sp: sp, first: first, last: last, palette: opt.Palette, video: opt.Video, wide: opt.Wide, maxHops: opt.MaxHops}
	if opt.SubpControl != nil {
		k.subp, k.hasSubp = *opt.SubpControl, true
	}
	o.mu.Lock()
	pic, ok := o.scans[k]
	o.mu.Unlock()
	if !ok {
		rd, err := o.Reader(sp)
		if err != nil {
			return nil, err
		}
		pic, err = ScanMenu(rd, first, last, opt)
		if err != nil {
			return nil, err
		}
		o.mu.Lock()
		if len(o.scans) > 512 {
			o.scans = map[scanKey]*MenuPicture{}
		}
		o.scans[k] = pic
		o.mu.Unlock()
	}
	return pic.WithDefault(opt.NavButton), nil
}

type spaceReader struct {
	ext   []FileExtent
	mu    sync.Mutex
	files map[string]*os.File
}

func (r *spaceReader) file(name string) (*os.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if f, ok := r.files[name]; ok {
		return f, nil
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	r.files[name] = f
	return f, nil
}

func (r *spaceReader) ReadSectors(first uint32, count int) ([]byte, error) {
	want := int64(count) * Sector
	parts := sliceExtents(r.ext, int64(first)*Sector, want)
	out := make([]byte, want)
	got := 0
	for _, p := range parts {
		f, err := r.file(p.File)
		if err != nil {
			return nil, err
		}
		// Read straight into the result: no intermediate copy per extent.
		n, err := f.ReadAt(out[got:got+int(p.Length)], p.Offset)
		got += n
		if err != nil && !errors.Is(err, io.EOF) {
			return out[:got], err
		}
		if int64(n) < p.Length {
			break
		}
	}
	return out[:got], nil
}

// Close releases file handles held by the reader.
func (r *spaceReader) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, f := range r.files {
		f.Close()
	}
	r.files = map[string]*os.File{}
}

// ---------------------------------------------------------------------------
// Sources
// ---------------------------------------------------------------------------

type imageSource struct{ img *isofs.Disc }

func (s *imageSource) Describe() string { return s.img.Path }
func (s *imageSource) Close() error     { return s.img.Close() }

func (s *imageSource) ReadIFO(name string) ([]byte, error) {
	b, err := s.img.ReadFile("VIDEO_TS/"+name, MaxIFOBytes)
	if err != nil && strings.HasSuffix(name, ".IFO") {
		// Fall back to the backup copy.
		b, err = s.img.ReadFile("VIDEO_TS/"+strings.TrimSuffix(name, ".IFO")+".BUP", MaxIFOBytes)
	}
	return b, err
}

func vobNames(sp Space) []string {
	switch sp.Domain {
	case DomainVMGM:
		return []string{"VIDEO_TS.VOB"}
	case DomainVTSM:
		return []string{fmt.Sprintf("VTS_%02d_0.VOB", sp.VTS)}
	}
	var out []string
	for k := 1; k <= 9; k++ {
		out = append(out, fmt.Sprintf("VTS_%02d_%d.VOB", sp.VTS, k))
	}
	return out
}

func (s *imageSource) VOBExtents(sp Space) ([]FileExtent, error) {
	var out []FileExtent
	for _, name := range vobNames(sp) {
		f, ok := s.img.Find("VIDEO_TS/" + name)
		if !ok {
			break
		}
		for _, e := range f.Extents {
			out = append(out, FileExtent{File: s.img.Path, Offset: e.Offset, Length: e.Length})
		}
	}
	return out, nil
}

type folderSource struct{ dir string }

func openFolder(p string) (Source, error) {
	dir := p
	if strings.ToUpper(filepath.Base(p)) != "VIDEO_TS" {
		dir = filepath.Join(p, "VIDEO_TS")
	}
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("dvd: no VIDEO_TS folder in %s", p)
	}
	return &folderSource{dir: dir}, nil
}

func (s *folderSource) Describe() string { return s.dir }
func (s *folderSource) Close() error     { return nil }

func (s *folderSource) find(name string) (string, bool) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) {
			return filepath.Join(s.dir, e.Name()), true
		}
	}
	return "", false
}

func (s *folderSource) ReadIFO(name string) ([]byte, error) {
	p, ok := s.find(name)
	if !ok {
		if p, ok = s.find(strings.TrimSuffix(name, ".IFO") + ".BUP"); !ok {
			return nil, fmt.Errorf("%s not found", name)
		}
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, MaxIFOBytes))
}

func (s *folderSource) VOBExtents(sp Space) ([]FileExtent, error) {
	var out []FileExtent
	for _, name := range vobNames(sp) {
		p, ok := s.find(name)
		if !ok {
			break
		}
		st, err := os.Stat(p)
		if err != nil {
			break
		}
		out = append(out, FileExtent{File: p, Offset: 0, Length: st.Size()})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Title summaries (for the library and the "Play main feature" shortcut)
// ---------------------------------------------------------------------------

// TitleSummary is a flattened view of one title.
type TitleSummary struct {
	Number   int         `json:"number"`
	VTS      int         `json:"vts"`
	VTSTitle int         `json:"vtsTitle"`
	Duration float64     `json:"durationSec"`
	Chapters []float64   `json:"chapters"` // chapter start offsets in seconds
	Angles   int         `json:"angles"`
	Audio    []AudioAttr `json:"audio"`
	Subp     []SubpAttr  `json:"subtitles"`
	Video    VideoAttr   `json:"video"`
	Cells    []TitleCell `json:"-"`
}

// TitleCell is one cell of a flattened title with its PGC.
type TitleCell struct {
	PGCN  int
	CellN int
	Cell  Cell
	Start float64
}

// Titles flattens every title (following PGC chains in program order).
func (d *Disc) Titles() []TitleSummary {
	var out []TitleSummary
	for _, t := range d.VMG.Titles {
		if ts := d.Title(t.Number); ts != nil {
			out = append(out, *ts)
		}
	}
	return out
}

// Title flattens one title by number.
func (d *Disc) Title(number int) *TitleSummary { return d.TitleAngle(number, 1) }

// TitleAngle flattens a title for one camera angle (1-based); cells inside
// angle blocks are replaced by that angle's cell.
func (d *Disc) TitleAngle(number, angle int) *TitleSummary {
	if number < 1 || number > len(d.VMG.Titles) {
		return nil
	}
	t := d.VMG.Titles[number-1]
	v := d.VTS[t.VTS]
	if v == nil || t.VTSTitle < 1 || t.VTSTitle > len(v.PTT) {
		return nil
	}
	ts := &TitleSummary{Number: number, VTS: t.VTS, VTSTitle: t.VTSTitle, Angles: t.Angles, Audio: v.Audio, Subp: v.Subp, Video: v.Video}
	ptts := v.PTT[t.VTSTitle-1]
	if len(ptts) == 0 {
		return ts
	}
	// Walk PGCs starting from the first chapter's PGC, following next_pgcn.
	seen := map[int]bool{}
	pgcn := ptts[0].PGCN
	chapterAt := map[[2]int]bool{}
	for _, p := range ptts {
		chapterAt[[2]int{p.PGCN, p.PGN}] = true
	}
	for pgcn >= 1 && pgcn <= len(v.PGCs) && !seen[pgcn] {
		seen[pgcn] = true
		p := v.PGCs[pgcn-1].PGC
		for n := 1; n >= 1 && n <= len(p.Cells); n = nextCellN(p, n) {
			k := angleCell(p, n, angle)
			c := p.Cells[k-1]
			pg := programOf(p, n)
			if chapterAt[[2]int{pgcn, pg}] && (len(p.Programs) == 0 || p.Programs[pg-1] == n) {
				ts.Chapters = append(ts.Chapters, ts.Duration)
			}
			ts.Cells = append(ts.Cells, TitleCell{PGCN: pgcn, CellN: k, Cell: c, Start: ts.Duration})
			ts.Duration += c.Duration
		}
		if p.NextPGCN == 0 {
			break
		}
		pgcn = p.NextPGCN
	}
	return ts
}

// MainTitle guesses the feature: the longest title.
func (d *Disc) MainTitle() *TitleSummary {
	ts := d.Titles()
	if len(ts) == 0 {
		return nil
	}
	sort.SliceStable(ts, func(i, j int) bool { return ts[i].Duration > ts[j].Duration })
	return &ts[0]
}
