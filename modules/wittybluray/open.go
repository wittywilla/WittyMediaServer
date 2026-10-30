package wittybluray

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/wittywilla/wittydisc/isofs"
)

// Opening a disc: an .iso/.img image or a folder that holds BDMV. The
// disc's program (index, movie objects), playlists and clip information are
// read at once; the stream files are only ever described as byte extents,
// never copied.

// MaxStructureBytes bounds how much of any .bdmv/.mpls/.clpi file is read.
const MaxStructureBytes = 16 << 20

// FileExtent is a byte range of a stream file: inside the image, or a file
// on disk when the disc is a folder.
type FileExtent struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
}

// Source abstracts an image or a BDMV folder.
type Source interface {
	ReadFile(name string, max int64) ([]byte, error) // name is relative to the disc root, e.g. BDMV/index.bdmv
	Extents(name string) ([]FileExtent, error)
	List(dir string) ([]string, error) // file names in a folder of the disc
	Describe() string
	Close() error
}

// Disc is an opened Blu-ray.
type Disc struct {
	Source       Source
	Label        string
	Index        *Index
	MovieObjects []MovieObject
	Playlists    map[int]*Playlist
	Clips        map[string]*Clip
	// Encrypted is set when the disc carries AACS (an AACS folder, or
	// scrambled packets in its streams). The engine never decrypts
	// anything: such discs are refused for playback.
	Encrypted bool
	// Stereo3D: the disc has 3D (MVC) content: an SSIF folder, a dependent
	// view stream, or the index's 3D flag.
	Stereo3D bool
	// BDJ: the disc's first play, top menu or a title is a BD-J (Java)
	// program, which this engine cannot run; such entries are played as
	// plain titles where possible.
	BDJ bool
}

// Open opens an image file or a disc folder (the folder that contains BDMV,
// or BDMV itself).
func Open(p string) (*Disc, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	var src Source
	if st.IsDir() {
		dir := p
		if strings.EqualFold(filepath.Base(dir), "BDMV") {
			dir = filepath.Dir(dir)
		}
		if _, err := os.Stat(filepath.Join(dir, "BDMV", "index.bdmv")); err != nil {
			return nil, fmt.Errorf("%s: no BDMV/index.bdmv (not a Blu-ray folder)", p)
		}
		src = &folderSource{dir: dir}
	} else {
		img, err := isofs.Open(p)
		if err != nil {
			return nil, err
		}
		if _, ok := img.Find("BDMV/index.bdmv"); !ok {
			img.Close()
			return nil, fmt.Errorf("%s: no BDMV/index.bdmv (not a Blu-ray image)", p)
		}
		src = &imageSource{img: img}
	}
	d := &Disc{Source: src, Playlists: map[int]*Playlist{}, Clips: map[string]*Clip{}}
	if err := d.load(); err != nil {
		src.Close()
		return nil, err
	}
	return d, nil
}

// Close releases the image.
func (d *Disc) Close() error { return d.Source.Close() }

func (d *Disc) load() error {
	if s, ok := d.Source.(*imageSource); ok {
		d.Label = s.img.Label
	} else if s, ok := d.Source.(*folderSource); ok {
		d.Label = filepath.Base(s.dir)
	}
	b, err := d.Source.ReadFile("BDMV/index.bdmv", MaxStructureBytes)
	if err != nil {
		return fmt.Errorf("index.bdmv: %w", err)
	}
	if d.Index, err = ParseIndex(b); err != nil {
		return err
	}
	if b, err = d.Source.ReadFile("BDMV/MovieObject.bdmv", MaxStructureBytes); err == nil {
		if d.MovieObjects, err = ParseMovieObjects(b); err != nil {
			return err
		}
	}
	names, _ := d.Source.List("BDMV/PLAYLIST")
	for _, n := range names {
		num, ok := structureNumber(n, ".mpls")
		if !ok {
			continue
		}
		b, err := d.Source.ReadFile("BDMV/PLAYLIST/"+n, MaxStructureBytes)
		if err != nil {
			continue
		}
		pl, err := ParsePlaylist(num, b)
		if err != nil {
			continue // one damaged playlist should not stop the disc
		}
		d.Playlists[num] = pl
	}
	names, _ = d.Source.List("BDMV/CLIPINF")
	for _, n := range names {
		if _, ok := structureNumber(n, ".clpi"); !ok {
			continue
		}
		b, err := d.Source.ReadFile("BDMV/CLIPINF/"+n, MaxStructureBytes)
		if err != nil {
			continue
		}
		c, err := ParseClip(strings.TrimSuffix(n, filepath.Ext(n)), b)
		if err != nil {
			continue
		}
		d.Clips[c.Name] = c
	}
	if len(d.Playlists) == 0 {
		return errors.New("the disc has no playlists")
	}
	d.BDJ = d.Index.FirstPlay.Kind == ObjectBDJ || d.Index.TopMenu.Kind == ObjectBDJ
	for _, t := range d.Index.Titles {
		if t.Object.Kind == ObjectBDJ {
			d.BDJ = true
		}
	}
	if ssif, _ := d.Source.List("BDMV/STREAM/SSIF"); len(ssif) > 0 {
		d.Stereo3D = true
	}
	if d.Index.Stereo3D {
		d.Stereo3D = true
	}
	for _, pl := range d.Playlists {
		for _, sp := range pl.SubPaths {
			if sp.Type == 8 || sp.Type == 9 {
				d.Stereo3D = true
			}
		}
	}
	if aacs, _ := d.Source.List("AACS"); len(aacs) > 0 {
		d.Encrypted = true
	}
	if !d.Encrypted {
		d.Encrypted = d.streamsScrambled()
	}
	return nil
}

// streamsScrambled looks at the start of the main clips for packets with
// the scrambling bits set (AACS without an AACS folder, or a damaged copy).
func (d *Disc) streamsScrambled() bool {
	checked := 0
	for _, pl := range d.sortedPlaylists() {
		if len(pl.Items) == 0 {
			continue
		}
		ext, err := d.Source.Extents("BDMV/STREAM/" + pl.Items[0].Clip + ".m2ts")
		if err != nil || len(ext) == 0 {
			continue
		}
		b, err := readExtent(ext[0], 192*256)
		if err != nil {
			continue
		}
		scrambled := 0
		for i := 0; i+192 <= len(b); i += 192 {
			if b[i+4] == 0x47 && b[i+7]&0xc0 != 0 {
				scrambled++
			}
		}
		if scrambled > 16 {
			return true
		}
		if checked++; checked >= 3 {
			break
		}
	}
	return false
}

func readExtent(e FileExtent, n int64) ([]byte, error) {
	f, err := os.Open(e.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if n > e.Length {
		n = e.Length
	}
	b := make([]byte, n)
	got, err := f.ReadAt(b, e.Offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return b[:got], nil
}

// Playlist returns a playlist by number.
func (d *Disc) Playlist(n int) *Playlist { return d.Playlists[n] }

// Clip returns a clip's information by name.
func (d *Disc) Clip(name string) *Clip { return d.Clips[name] }

// ClipExtents returns the byte extents of a clip's stream file.
func (d *Disc) ClipExtents(name string) ([]FileExtent, error) {
	return d.Source.Extents("BDMV/STREAM/" + name + ".m2ts")
}

// sortedPlaylists lists playlists by number.
func (d *Disc) sortedPlaylists() []*Playlist {
	out := make([]*Playlist, 0, len(d.Playlists))
	for _, pl := range d.Playlists {
		out = append(out, pl)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

// PlaylistDuration is the playlist's length in seconds.
func (d *Disc) PlaylistDuration(pl *Playlist) float64 {
	var t float64
	for i := range pl.Items {
		t += pl.Items[i].Duration()
	}
	return t
}

// MainPlaylist guesses the feature: the longest playlist whose clips are
// not all shared with a longer one (so a "play all" of the extras does not
// win), with a video stream.
func (d *Disc) MainPlaylist() *Playlist {
	var best *Playlist
	var bestLen float64
	for _, pl := range d.sortedPlaylists() {
		if len(pl.Items) == 0 || len(pl.Items[0].Video) == 0 {
			continue
		}
		if l := d.PlaylistDuration(pl); l > bestLen {
			best, bestLen = pl, l
		}
	}
	return best
}

func structureNumber(name, ext string) (int, bool) {
	if !strings.EqualFold(filepath.Ext(name), ext) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSuffix(name, filepath.Ext(name)))
	return n, err == nil
}

// ---------------------------------------------------------------- sources

type imageSource struct{ img *isofs.Disc }

func (s *imageSource) Describe() string { return s.img.Path }
func (s *imageSource) Close() error     { return s.img.Close() }
func (s *imageSource) ReadFile(name string, max int64) ([]byte, error) {
	return s.img.ReadFile(name, max)
}
func (s *imageSource) Extents(name string) ([]FileExtent, error) {
	f, ok := s.img.Find(name)
	if !ok || f.IsDir {
		return nil, fmt.Errorf("%s: not on this disc", name)
	}
	out := make([]FileExtent, 0, len(f.Extents))
	for _, e := range f.Extents {
		out = append(out, FileExtent{Path: s.img.Path, Offset: e.Offset, Length: e.Length})
	}
	return out, nil
}
func (s *imageSource) List(dir string) ([]string, error) {
	dir = strings.ToUpper(strings.Trim(dir, "/")) + "/"
	var out []string
	for _, f := range s.img.Files() {
		up := strings.ToUpper(f.Path)
		if f.IsDir || !strings.HasPrefix(up, dir) || strings.Contains(up[len(dir):], "/") {
			continue
		}
		out = append(out, f.Path[len(dir):])
	}
	sort.Strings(out)
	return out, nil
}

type folderSource struct{ dir string }

func (s *folderSource) Describe() string { return s.dir }
func (s *folderSource) Close() error     { return nil }
func (s *folderSource) path(name string) string {
	return filepath.Join(s.dir, filepath.FromSlash(name))
}
func (s *folderSource) ReadFile(name string, max int64) ([]byte, error) {
	f, err := os.Open(s.path(name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, max))
}
func (s *folderSource) Extents(name string) ([]FileExtent, error) {
	st, err := os.Stat(s.path(name))
	if err != nil {
		return nil, err
	}
	return []FileExtent{{Path: s.path(name), Offset: 0, Length: st.Size()}}, nil
}
func (s *folderSource) List(dir string) ([]string, error) {
	ents, err := os.ReadDir(s.path(dir))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}
