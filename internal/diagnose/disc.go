package diagnose

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wittywilla/wittycd"
	"github.com/wittywilla/wittydisc/isofs"
)

// DiscOptions choose the long parts of a disc check.
type DiscOptions struct {
	Crawl  bool // press every menu button
	Damage bool // read everything
}

// SubmitDisc queues a check of a disc at a path that has already passed
// Roots.Resolve. Nothing here looks at where the path is from.
func (m *Manager) SubmitDisc(path string, o DiscOptions) (*Job, error) {
	base := filepath.Base(path)
	scrub := []scrubRule{{path: path, with: "this disc"}, {path: filepath.Dir(path), with: "its folder"}}
	return m.Submit(KindDisc, base, map[string]bool{"crawl": o.Crawl, "damage": o.Damage}, scrub, func(ctx context.Context, j *Job) {
		checkDisc(ctx, j, path, o)
	})
}

// discFile is one file of a disc: inside an image, or on disk for a disc folder.
type discFile struct {
	Name string // its path on the disc, with forward slashes
	Size int64
	Exts []isofs.Extent // an image's: byte ranges in the image
	Real string         // a folder's: where it is on disk. Never reported.
}

// discSource is what can be said about a disc before an engine sees it.
type discSource struct {
	Path    string
	IsDir   bool
	Format  string // udf, iso9660 or folder
	Label   string
	Size    int64
	Files   []discFile
	HasDVD  bool
	HasBD   bool
	img     *isofs.Disc
	byUpper map[string]*discFile
}

func (s *discSource) close() {
	if s.img != nil {
		_ = s.img.Close() // only read from
	}
}

// file finds a file by its path on the disc, whatever the case.
func (s *discSource) file(name string) *discFile { return s.byUpper[strings.ToUpper(name)] }

// read reads a file of the disc, at most max bytes.
func (s *discSource) read(name string, max int64) ([]byte, error) {
	f := s.file(name)
	if f == nil {
		return nil, fs.ErrNotExist
	}
	if s.img != nil {
		return s.img.ReadFile(f.Name, max)
	}
	h, err := os.Open(f.Real)
	if err != nil {
		return nil, err
	}
	defer h.Close()
	return io.ReadAll(io.LimitReader(h, max))
}

// maxFolderFiles bounds a disc folder's walk: real discs hold hundreds of files.
const maxFolderFiles = 50000

// discFolders are the folders a video disc keeps at its top level.
var discFolders = map[string]bool{"VIDEO_TS": true, "AUDIO_TS": true, "BDMV": true, "BDAV": true, "AACS": true, "BDSVM": true, "CERTIFICATE": true, "PRIVATE": true, "AVCHD": true}

func openSource(path string) (*discSource, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	s := &discSource{Path: path, IsDir: st.IsDir(), byUpper: map[string]*discFile{}}
	if !st.IsDir() {
		s.Size = st.Size()
		img, err := isofs.Open(path)
		if err != nil {
			return s, err
		}
		s.img, s.Format, s.Label = img, img.Format, img.Label
		for _, f := range img.Files() {
			if f.IsDir {
				continue
			}
			s.Files = append(s.Files, discFile{Name: f.Path, Size: f.Size, Exts: f.Extents})
		}
	} else {
		s.Format = "folder"
		root, starts := path, []string{}
		if up := strings.ToUpper(filepath.Base(path)); up == "VIDEO_TS" || up == "BDMV" || up == "BDAV" {
			root = filepath.Dir(path)
			starts = []string{path}
		} else {
			// Only a disc's own folders count: this may be a folder that also
			// holds much else, and a check must not wander through all of it.
			ents, _ := os.ReadDir(path)
			for _, e := range ents {
				if e.IsDir() && discFolders[strings.ToUpper(e.Name())] {
					starts = append(starts, filepath.Join(path, e.Name()))
				}
			}
		}
		for _, start := range starts {
			_ = filepath.WalkDir(start, func(p string, e fs.DirEntry, err error) error {
				if err != nil || e.IsDir() {
					return nil
				}
				if !e.Type().IsRegular() || len(s.Files) >= maxFolderFiles {
					return nil
				}
				info, err := e.Info()
				if err != nil {
					return nil
				}
				rel, err := filepath.Rel(root, p)
				if err != nil {
					return nil
				}
				s.Files = append(s.Files, discFile{Name: filepath.ToSlash(rel), Size: info.Size(), Real: p})
				s.Size += info.Size()
				return nil
			})
		}
		slices.SortFunc(s.Files, func(a, b discFile) int { return strings.Compare(a.Name, b.Name) })
	}
	for i := range s.Files {
		f := &s.Files[i]
		up := strings.ToUpper(f.Name)
		s.byUpper[up] = f
		switch {
		case strings.HasPrefix(up, "VIDEO_TS/"):
			s.HasDVD = true
		case strings.HasPrefix(up, "BDMV/"), strings.HasPrefix(up, "BDAV/"), strings.Contains(up, "AVCHD/BDMV/"):
			s.HasBD = true
		}
	}
	return s, nil
}

func bytesText(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

func hms(sec float64) string {
	s := int(sec + 0.5)
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
}

func formatName(f string) string {
	switch f {
	case "udf":
		return "UDF"
	case "iso9660":
		return "ISO 9660"
	}
	return f
}

// barSpans shares out the progress bar among the long parts of a check.
func barSpans(o DiscOptions) (crawlFrom, crawlTo, damageFrom float64) {
	switch {
	case o.Crawl && o.Damage:
		return 0.2, 0.45, 0.45
	case o.Damage:
		return 0.2, 0.2, 0.2
	}
	return 0.2, 0.98, 0.98
}

// checkDisc is the whole of a disc check.
func checkDisc(ctx context.Context, j *Job, path string, o DiscOptions) {
	j.Step("Looking at what this is", 0.02)
	if wittycd.IsImageName(path) && checkCD(ctx, j, path) {
		return
	}
	var src *discSource
	var err error
	if !j.Engine("reading the disc's file list", func() { src, err = openSource(path) }) {
		return
	}
	if err != nil {
		if wittycd.MayBeRaw(path) && checkCD(ctx, j, path) {
			return
		}
		reportUnreadable(j, path, err)
		return
	}
	defer src.close()
	j.SetLabel(src.Label)
	if !src.HasDVD && !src.HasBD {
		if looksLikeGameDisc(src) {
			j.Add(Finding{Severity: SevInfo, Title: "This looks like a game disc", What: fmt.Sprintf("It's readable (%s, %s, %d files), and has what a game console's disc has rather than a video structure, so there are no films or menus here to check.", kindWord(src), bytesText(src.Size), len(src.Files)),
				Fix: "To play it, put it in a Games library."})
			return
		}
		what := fmt.Sprintf("It's readable (%s, %s, %d files), but has no DVD or Blu-ray structure: no VIDEO_TS or BDMV folder.", kindWord(src), bytesText(src.Size), len(src.Files))
		j.Add(Finding{Severity: SevProblem, Title: "This isn't a DVD or Blu-ray", What: what,
			Fix: "Pick the disc image, or the folder that holds VIDEO_TS or BDMV. If this is a copy of a video disc, it was made in a way that left the video structure out."})
		return
	}
	switch {
	case src.HasDVD && src.HasBD:
		j.Add(Finding{Severity: SevInfo, Title: "This disc holds both a DVD and a Blu-ray", What: "Both structures are on it. They're checked one after the other."})
		checkBluray(ctx, j, src, o)
		if ctx.Err() == nil {
			checkDVD(ctx, j, src, o)
		}
	case src.HasBD:
		checkBluray(ctx, j, src, o)
	default:
		checkDVD(ctx, j, src, o)
	}
}

// looksLikeGameDisc spots a console's boot file at the top of a disc.
func looksLikeGameDisc(s *discSource) bool {
	for i := range s.Files {
		switch strings.ToUpper(strings.TrimPrefix(s.Files[i].Name, "/")) {
		case "SYSTEM.CNF", "UMD_DATA.BIN", "PSP_GAME/PARAM.SFO", "PS3_DISC.SFB", "DEFAULT.XEX", "IP.BIN":
			return true
		}
	}
	return false
}

func kindWord(s *discSource) string {
	if s.IsDir {
		return "a folder"
	}
	return "a " + formatName(s.Format) + " image"
}

// reportUnreadable explains a file that isn't a disc image at all, looking at
// its first bytes to say what it might be instead.
func reportUnreadable(j *Job, path string, err error) {
	st, serr := os.Stat(path)
	if serr != nil {
		j.Add(Finding{Severity: SevProblem, Title: "This can't be opened", What: "The server got this answer: " + plainErr(serr) + ".",
			Fix: "Check that the drive is connected and that the server's account may read it."})
		return
	}
	f := Finding{Severity: SevProblem, Title: "This isn't a disc image the server can read",
		What:    "It has neither a UDF nor an ISO 9660 file system where a disc image has one.",
		Fix:     "If it's a copy of a disc, make it again as a plain .iso image.",
		Details: []string{"Size: " + bytesText(st.Size()), "The engine said: " + plainErr(err)}}
	if st.Size() < 64*1024 {
		f.What = "It's far too small to be a disc image, which suggests the copy didn't finish."
	} else if h, oerr := os.Open(path); oerr == nil {
		defer h.Close()
		if hint := sniffImage(h, st.Size()); hint != "" {
			f.What = hint
		}
	}
	j.Add(f)
}

// sniffImage recognises a few things that aren't plain 2048-byte images, from
// structure bytes only.
func sniffImage(h io.ReaderAt, size int64) string {
	raw := make([]byte, 2352)
	if n, _ := h.ReadAt(raw, 16*2352); n == 2352 {
		// A raw sector starts with 12 sync bytes, and the volume descriptor's
		// mark follows the sector's header.
		if raw[0] == 0 && raw[1] == 0xff && raw[11] == 0 && (string(raw[17:22]) == "CD001" || string(raw[25:30]) == "CD001") {
			return "It looks like a raw dump with 2352-byte sectors, not a plain image with 2048-byte sectors. The DVD and Blu-ray engines read only plain images. CD games are often kept this way: if it's a game, put it in a Games library."
		}
	}
	head := make([]byte, 64*1024)
	n, _ := h.ReadAt(head, 0)
	zero := true
	for _, c := range head[:n] {
		if c != 0 {
			zero = false
			break
		}
	}
	first := make([]byte, 2048)
	if _, err := h.ReadAt(first, 16*2048); err == nil && zero && !hasVolumeMark(first) {
		return "The start of the file is all zeros and there is no file system mark where one should be. The copy may have failed at the very beginning."
	}
	if size%2048 != 0 {
		return "Its size isn't a whole number of 2048-byte sectors, so it isn't a plain disc image, or the copy was cut short."
	}
	return ""
}

func hasVolumeMark(sector []byte) bool {
	id := string(sector[1:6])
	return id == "CD001" || id == "BEA01" || id == "NSR02" || id == "NSR03" || id == "TEA01"
}

// volumeSectors reads how many sectors an image says it has, from its file
// system's own words: the ISO 9660 volume size, and the end of the UDF partition.
// It reads only those structures. 0 when neither says.
func volumeSectors(h io.ReaderAt, imageSize int64) int64 {
	b := make([]byte, 2048)
	var claim int64
	if _, err := h.ReadAt(b, 16*2048); err == nil && string(b[1:6]) == "CD001" && b[0] == 1 {
		claim = int64(binary.LittleEndian.Uint32(b[80:84]))
	}
	for _, at := range []int64{256, imageSize/2048 - 1, imageSize/2048 - 257} {
		if at < 0 {
			continue
		}
		if _, err := h.ReadAt(b, at*2048); err != nil || binary.LittleEndian.Uint16(b[0:2]) != 2 {
			continue
		}
		mvds := int64(binary.LittleEndian.Uint32(b[20:24]))
		count := int64(binary.LittleEndian.Uint32(b[16:20])) / 2048
		for i := int64(0); i < count && i < 64; i++ {
			if _, err := h.ReadAt(b, (mvds+i)*2048); err != nil {
				break
			}
			tag := binary.LittleEndian.Uint16(b[0:2])
			if tag == 8 {
				break
			}
			if tag == 5 {
				if end := int64(binary.LittleEndian.Uint32(b[188:192])) + int64(binary.LittleEndian.Uint32(b[192:196])); end > claim {
					claim = end
				}
			}
		}
		break
	}
	return claim
}
