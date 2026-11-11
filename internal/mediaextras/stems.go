package mediaextras

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/metafolder"
)

// Stems are the separated parts of a song: the voice alone, the music without it, and so
// on, for karaoke and practice. This server doesn't separate songs itself. It finds what a
// tool such as Demucs or Spleeter made, or what an administrator uploads.

// MaxStem is the largest stem taken as an upload. A lossless five-minute stem is about 50 MB.
const MaxStem = 200 << 20

// Multitrack is the name a Native Instruments .stem.mp4 file is listed under: one file
// holding every part as its own audio track.
const Multitrack = "multitrack"

// StemNames are the parts, in the order they're listed.
var StemNames = []string{"vocals", "instrumental", "drums", "bass", "other"}

// stemAliases are what tools and people call the parts.
var stemAliases = map[string]string{
	"vocals": "vocals", "vocal": "vocals", "voice": "vocals", "acapella": "vocals", "a_cappella": "vocals", "acappella": "vocals",
	"instrumental": "instrumental", "accompaniment": "instrumental", "karaoke": "instrumental", "no_vocals": "instrumental",
	"novocals": "instrumental", "no-vocals": "instrumental", "inst": "instrumental", "minus_vocals": "instrumental",
	"drums": "drums", "drum": "drums", "bass": "bass", "other": "other",
}

// StemName is the part a name stands for ("accompaniment" is "instrumental"), or "".
// Only these names are ever used to make a path.
func StemName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == Multitrack {
		return Multitrack
	}
	return stemAliases[s]
}

// StemExts are the kinds of file a stem can be.
var StemExts = []string{".flac", ".wav", ".m4a", ".mp3", ".ogg", ".opus", ".aac", ".webm"}

func stemExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range StemExts {
		if e == ext {
			return true
		}
	}
	return false
}

// StemMIME is the type a stem is sent with, by its name only.
func StemMIME(p string) string {
	if strings.HasSuffix(strings.ToLower(p), ".stem.mp4") {
		return "audio/mp4"
	}
	switch strings.ToLower(filepath.Ext(p)) {
	case ".flac":
		return "audio/flac"
	case ".wav":
		return "audio/wav"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a", ".mp4":
		return "audio/mp4"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".aac":
		return "audio/aac"
	case ".webm":
		return "audio/webm"
	}
	return "application/octet-stream"
}

// AudioExt says from its first bytes what kind of audio file this is, whatever its name
// says: ".flac", ".wav", ".ogg", ".m4a", ".aac", ".mp3" or ".webm", or "" for anything else.
func AudioExt(b []byte) string {
	switch {
	case len(b) >= 4 && bytes.Equal(b[:4], []byte("fLaC")):
		return ".flac"
	case len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WAVE")):
		return ".wav"
	case len(b) >= 4 && bytes.Equal(b[:4], []byte("OggS")):
		return ".ogg"
	case isMP4(b):
		return ".m4a"
	case isWebM(b):
		return ".webm"
	case len(b) >= 3 && bytes.Equal(b[:3], []byte("ID3")):
		return ".mp3"
	case len(b) >= 2 && b[0] == 0xFF && b[1]&0xF6 == 0xF0: // ADTS: layer bits are 00
		return ".aac"
	case len(b) >= 2 && b[0] == 0xFF && b[1]&0xE0 == 0xE0 && b[1]&0x06 != 0: // an MPEG audio frame
		return ".mp3"
	}
	return ""
}

// MultitrackExt takes only an MP4, for a .stem.mp4.
func MultitrackExt(b []byte) string {
	if isMP4(b) {
		return ".mp4"
	}
	return ""
}

// Stem is one part that was found.
type Stem struct {
	Name       string `json:"name"`
	Path       string `json:"-"`
	Size       int64  `json:"size"`
	MIME       string `json:"mime"`
	Multitrack bool   `json:"multitrack,omitempty"`
	// Source: upload (an administrator's) or sidecar (beside the song).
	Source string `json:"source"`
	// Uploaded is true for a file in the server's own folder, false beside the media.
	Uploaded bool `json:"-"`
}

// maxDirEntries bounds how much of one folder is read.
const maxDirEntries = 5000

func readDir(dir string) []os.DirEntry {
	f, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer f.Close()
	entries, _ := f.ReadDir(maxDirEntries)
	return entries
}

// FindStems lists the stems of a song. uploads is the item's folder of uploads (or ""),
// checked first; then, beside the song "Song.flac":
//
//	Song.vocals.flac, Song.instrumental.m4a ...     the song's name, a dot, the part
//	Song.stems/vocals.wav ...                        a folder named after the song
//	stems/Song/vocals.wav, stems/Song.vocals.wav     an album's stems folder
//	Song.stem.mp4                                    a Native Instruments file, all parts in one
//
// Demucs' two-stem output (no_vocals) counts as instrumental. Each part is listed once,
// from the first place it's found. Only ordinary files count: links are left alone.
func FindStems(mediaPath, uploads string) []Stem {
	found := map[string]Stem{}
	add := func(name, p, source string, uploaded bool) {
		if name == "" || found[name].Path != "" {
			return
		}
		fi, err := os.Lstat(p)
		if err != nil || !plainFileInfo(fi) {
			return
		}
		found[name] = Stem{Name: name, Path: p, Size: fi.Size(), MIME: StemMIME(p), Multitrack: name == Multitrack, Source: source, Uploaded: uploaded}
	}
	if uploads != "" {
		dir := filepath.Join(uploads, "stems")
		for _, e := range readDir(dir) {
			n := e.Name()
			if strings.HasPrefix(n, ".") {
				continue
			}
			if strings.EqualFold(n, Multitrack+".stem.mp4") {
				add(Multitrack, filepath.Join(dir, n), "upload", true)
				continue
			}
			if stemExt(n) {
				add(StemName(strings.TrimSuffix(n, filepath.Ext(n))), filepath.Join(dir, n), "upload", true)
			}
		}
	}
	dir := filepath.Dir(mediaPath)
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	lbase := strings.ToLower(base)
	// Song.<part>.<ext>, and Song.stem.mp4, in the song's own folder.
	byName := func(d string, source string) {
		for _, e := range readDir(d) {
			n := e.Name()
			ln := strings.ToLower(n)
			if !strings.HasPrefix(ln, lbase+".") || e.IsDir() {
				continue
			}
			if ln == lbase+".stem.mp4" {
				add(Multitrack, filepath.Join(d, n), source, false)
				continue
			}
			if !stemExt(n) {
				continue
			}
			part := strings.TrimSuffix(n[len(base)+1:], filepath.Ext(n))
			add(StemName(part), filepath.Join(d, n), source, false)
		}
	}
	// <folder>/<part>.<ext>
	inFolder := func(d string, source string) {
		for _, e := range readDir(d) {
			n := e.Name()
			if e.IsDir() || strings.HasPrefix(n, ".") || !stemExt(n) {
				continue
			}
			add(StemName(strings.TrimSuffix(n, filepath.Ext(n))), filepath.Join(d, n), source, false)
		}
	}
	byName(dir, "sidecar")
	inFolder(filepath.Join(dir, base+".stems"), "sidecar")
	inFolder(filepath.Join(dir, "stems", base), "sidecar")
	byName(filepath.Join(dir, "stems"), "sidecar")
	out := make([]Stem, 0, len(found))
	for _, s := range found {
		out = append(out, s)
	}
	order := map[string]int{Multitrack: len(StemNames)}
	for i, n := range StemNames {
		order[n] = i
	}
	sort.Slice(out, func(i, j int) bool { return order[out[i].Name] < order[out[j].Name] })
	return out
}

func plainFileInfo(fi os.FileInfo) bool { return metafolder.PlainFile(fi) }
