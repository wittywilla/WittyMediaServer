// Package cd is what the server knows of a CD image: the shapes WittyCD answers in, and how a file's
// name says it may be one. WittyCD itself is a module, reached through a helper process (internal/modules).
// These travel as JSON, so a field a newer version adds is simply not read here.

package cd

import (
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// sheetText turns the bytes of a text sheet into a string: a byte-order mark is
// believed, then valid UTF-8, and anything else is read as Windows Western, which
// is what most old rips were written in.
func sheetText(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0xef && b[1] == 0xbb && b[2] == 0xbf:
		return strings.ToValidUTF8(string(b[3:]), "�")
	case len(b) >= 2 && b[0] == 0xff && b[1] == 0xfe:
		return utf16Text(b[2:], false)
	case len(b) >= 2 && b[0] == 0xfe && b[1] == 0xff:
		return utf16Text(b[2:], true)
	case utf8.Valid(b):
		return string(b)
	}
	var s strings.Builder
	s.Grow(len(b))
	for _, c := range b {
		if c >= 0x80 && c < 0xa0 {
			s.WriteRune(cp1252[c-0x80])
		} else {
			s.WriteRune(rune(c))
		}
	}
	return s.String()
}

func utf16Text(b []byte, big bool) string {
	var s strings.Builder
	s.Grow(len(b) / 2)
	var hi rune
	for i := 0; i+1 < len(b); i += 2 {
		v := rune(b[i]) | rune(b[i+1])<<8
		if big {
			v = rune(b[i])<<8 | rune(b[i+1])
		}
		switch {
		case v >= 0xd800 && v < 0xdc00:
			hi = v
			continue
		case v >= 0xdc00 && v < 0xe000 && hi != 0:
			v = 0x10000 + (hi-0xd800)<<10 + (v - 0xdc00)
		case v >= 0xd800 && v < 0xe000:
			v = utf8.RuneError
		}
		hi = 0
		s.WriteRune(v)
	}
	return s.String()
}

var cp1252 = [32]rune{'€', '�', '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', '�', 'Ž', '�',
	'�', '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', '�', 'ž', 'Ÿ'}

func readSheet(path string) ([]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("can't read %q: %w", filepath.Base(path), err)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q isn't a file", ErrNotCD, filepath.Base(path))
	}
	if st.Size() > maxSheet {
		return nil, fmt.Errorf("%w: %q is far too large to be a sheet", ErrNotCD, filepath.Base(path))
	}
	return os.ReadFile(path)
}

// sameNameAudio finds the audio beside a sheet when the name inside it is stale.
func sameNameAudio(sheet string) string {
	base := strings.TrimSuffix(sheet, filepath.Ext(sheet))
	return firstExisting(base+".bin", base+".wav", base+".img", base+".flac", base+".BIN", base+".WAV", base+".IMG", base+".FLAC")
}

const (
	// SectorBytes is one sector of CD audio: 1/75 of a second.
	SectorBytes = 2352
	// SectorSamples is the stereo samples in a sector.
	SectorSamples = 588
	// SampleBytes is one stereo sample: two 16-bit values.
	SampleBytes = 4
	// SampleRate is what every audio CD plays at.
	SampleRate = 44100
	// FramesPerSecond is sectors per second, the F of MM:SS:FF.
	FramesPerSecond = 75
	// LeadIn is the 2 seconds before LBA 0, in sectors: MM:SS:FF counts them on a
	// real disc, and disc IDs add them to every offset.
	LeadIn = 150

	// MaxSectors is the longest disc there can be: 99:59:74.
	MaxSectors = 450000
	// MaxTracks is the most a disc's table of contents can hold.
	MaxTracks  = 99
	maxIndexes = 100
	// maxSheet bounds any text or chunk read whole from a sheet or footer.
	maxSheet = 4 << 20
)

// Errors a caller may want to tell apart. Each reads as a sentence.
var (
	// ErrNotCD: not a CD image at all, such as a DVD or a data disc's ISO.
	ErrNotCD = errors.New("this isn't an image of an audio CD")
	// ErrNoAudio: a CD image with only data tracks.
	ErrNoAudio = errors.New("this CD image holds only data tracks, so there is nothing to play")
	// ErrUnsupported: a CD image in a form this version can't read.
	ErrUnsupported = errors.New("this CD image is stored in a form WittyCD can't read")
)

// Index is an index point inside a track.
type Index struct {
	Number int `json:"number"`
	LBA    int `json:"lba"`
}

// Track is one track of the table of contents, data tracks included.
type Track struct {
	Number int `json:"number"`
	// Audio is false for a data track, which is listed and never played.
	Audio bool   `json:"audio"`
	Mode  string `json:"mode"` // AUDIO, MODE1/2352 and so on
	// Pregap is where index 0 starts, Start where index 1 starts and End where
	// the next track's pregap starts, all in sectors from LBA 0.
	Pregap int `json:"pregap"`
	Start  int `json:"start"`
	End    int `json:"end"`
	// Indexes above 1.
	Indexes     []Index `json:"indexes,omitempty"`
	PreEmphasis bool    `json:"preEmphasis,omitempty"`
	CopyAllowed bool    `json:"copyAllowed,omitempty"`
	FourChannel bool    `json:"fourChannel,omitempty"`
	ISRC        string  `json:"isrc,omitempty"`
	Session     int     `json:"session,omitempty"`
	Text        Text    `json:"text"`
}

// Text is what a disc says about itself or about one track.
type Text struct {
	Title      string `json:"title,omitempty"`
	Performer  string `json:"performer,omitempty"`
	Songwriter string `json:"songwriter,omitempty"`
	Composer   string `json:"composer,omitempty"`
	Arranger   string `json:"arranger,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Warning is something worth telling the owner about an image that still plays.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// IsImageName says whether a file's name marks it as a CD image's sheet or
// container. .iso, .img and .bin may be one too, which only opening them tells.
func IsImageName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".cue", ".toc", ".ccd", ".nrg", ".chd":
		return true
	}
	return false
}

// MayBeRaw says whether a file with this name might be bare CD audio.
func MayBeRaw(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".iso", ".img", ".bin":
		return true
	}
	return false
}

func firstExisting(paths ...string) string {
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// TOC is a table of contents as disc ID services take it: audio tracks only, with
// the 2-second lead-in added to every offset.
type TOC struct {
	First, Last int
	LeadOut     int
	Offsets     []int // of tracks First to Last
}

// MusicBrainzID is the disc's ID in MusicBrainz: 28 characters, from the SHA-1 of
// the table of contents written in hexadecimal.
func (t TOC) MusicBrainzID() string {
	if t.Last == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(804)
	fmt.Fprintf(&b, "%02X%02X%08X", t.First, t.Last, t.LeadOut)
	for i := 1; i <= 99; i++ {
		v := 0
		if n := i - t.First; n >= 0 && n < len(t.Offsets) {
			v = t.Offsets[n]
		}
		fmt.Fprintf(&b, "%08X", v)
	}
	sum := sha1.Sum([]byte(b.String()))
	s := base64.StdEncoding.EncodeToString(sum[:])
	return strings.NewReplacer("+", ".", "/", "_", "=", "-").Replace(s)
}

// LookupString is the toc= value of a MusicBrainz lookup, which also finds a disc
// nobody has attached that exact ID to. A release is looked up by its track count
// from 1, so a first track above 1 (a data track first) is told as it is.
func (t TOC) LookupString() string {
	parts := []string{strconv.Itoa(t.First), strconv.Itoa(t.Last), strconv.Itoa(t.LeadOut)}
	for _, o := range t.Offsets {
		parts = append(parts, strconv.Itoa(o))
	}
	return strings.Join(parts, "+")
}

// CDDBID is the older eight-digit disc ID. It is only shown, for matching a disc
// against other tools' logs; nothing here looks it up.
func (t TOC) CDDBID() string {
	if len(t.Offsets) == 0 {
		return ""
	}
	sum := 0
	for _, o := range t.Offsets {
		for s := o / FramesPerSecond; s > 0; s /= 10 {
			sum += s % 10
		}
	}
	length := t.LeadOut/FramesPerSecond - t.Offsets[0]/FramesPerSecond
	return fmt.Sprintf("%08x", uint32(sum%255)<<24|uint32(length)<<8|uint32(len(t.Offsets)))
}

var _ = os.ErrNotExist

var (
	cueFileLine = regexp.MustCompile(`(?i)^\s*FILE\s+"?(.+?)"?\s+[A-Z0-9]+\s*$`)
	tocFileLine = regexp.MustCompile(`(?i)^\s*(?:AUDIOFILE|DATAFILE|FILE)\s+"([^"]+)"`)
)

// SheetFiles lists the files a sheet plays, resolved beside it, without opening
// any of them, and says whether the sheet is really a disc image. It is cheap
// enough for a scan to ask about every sheet it finds.
//
// A sheet is not treated as an image when its audio is already a set of separately
// compressed files, one to a track: those are tagged songs, and the sheet is just
// a rip's notes. The same goes for a sheet naming compressed audio WittyCD can't
// read. A .ccd, .nrg or .chd holds its own audio, and is always an image.
func SheetFiles(path string) (files []string, image bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cue":
	case ".toc":
	default:
		return nil, IsImageName(path)
	}
	raw, err := readSheet(path)
	if err != nil {
		return nil, false
	}
	line := cueFileLine
	if strings.EqualFold(filepath.Ext(path), ".toc") {
		line = tocFileLine
	}
	dir := filepath.Dir(path)
	seen := map[string]bool{}
	compressed, named := 0, 0
	for _, l := range strings.Split(sheetText(raw), "\n") {
		m := line.FindStringSubmatch(strings.TrimRight(l, "\r"))
		if m == nil {
			continue
		}
		named++
		p, err := resolveBeside(dir, m[1])
		if err != nil {
			if q := sameNameAudio(path); q != "" {
				p = q
			} else {
				continue
			}
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		switch ext := strings.ToLower(filepath.Ext(p)); {
		case ext == ".flac":
			compressed++
		case foreignAudioExt(ext):
			if ForeignAudio == nil {
				return nil, false
			}
			compressed++
		}
		files = append(files, p)
	}
	if named == 0 || compressed > 1 {
		return nil, false
	}
	// A sheet whose files are all missing is still an image, and opening it says so.
	return files, true
}

// resolveBeside finds a file a sheet names. A sheet is someone else's text, so it
// can only ever reach files in or under its own folder: a name that is absolute
// or climbs out is looked for by its last part beside the sheet, which also
// rescues the many rips that recorded the full path on the ripper's PC.
func resolveBeside(dir, name string) (string, error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, `\`, "/"))
	if name == "" || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("a sheet names a file with no usable name")
	}
	rel := filepath.FromSlash(name)
	if !filepath.IsLocal(rel) || strings.Contains(name, ":") {
		last := name[strings.LastIndexAny(name, "/:")+1:]
		if last == "" || last == "." || last == ".." {
			return "", fmt.Errorf("a sheet names %q, which can't be used", name)
		}
		rel = filepath.FromSlash(last)
	}
	p := filepath.Join(dir, rel)
	st, err := os.Lstat(p)
	if err != nil || st.IsDir() {
		// Different letter case: a rip from Windows on a case-sensitive disk.
		if p = matchName(filepath.Dir(p), filepath.Base(p)); p == "" {
			return "", fmt.Errorf("the file %q this image needs isn't in its folder: %w", filepath.Base(rel), os.ErrNotExist)
		}
		if st, err = os.Lstat(p); err != nil {
			return "", err
		}
		if st.IsDir() {
			return "", fmt.Errorf("the file %q this image needs isn't in its folder: %w", filepath.Base(rel), os.ErrNotExist)
		}
	}
	// Any part of the way may be a link, not only the file's own name.
	if !staysInside(dir, p) {
		return "", fmt.Errorf("a sheet names %q, a link to somewhere outside its folder", name)
	}
	return p, nil
}

func staysInside(dir, p string) bool {
	root, err1 := filepath.EvalSymlinks(dir)
	target, err2 := filepath.EvalSymlinks(p)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(root, target)
	return err == nil && filepath.IsLocal(rel)
}

func matchName(dir, base string) string {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) > 20000 {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(e.Name(), base) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// foreignAudioExt: a sound file only ForeignAudio can read.
func foreignAudioExt(ext string) bool {
	switch ext {
	case ".mp3", ".ogg", ".oga", ".opus", ".m4a", ".m4b", ".aac", ".ape", ".wv", ".wma", ".tta", ".mpc", ".dsf", ".dff", ".tak", ".mka", ".ac3", ".dts", ".alac":
		return true
	}
	return false
}
