package library

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
	"github.com/wittywilla/wittymediaserver/internal/playback"
)

// Subtitle files beside a video, read when they're needed rather than kept in the
// database: dropping a file in the folder is enough for it to show up.

// SubtitleFile is one subtitle file that belongs to a video.
type SubtitleFile struct {
	Path     string
	Format   string // the registry's id
	Codec    string // FFmpeg's name, as for a track inside a file
	Language string
	Title    string
	Forced   bool
	SDH      bool
	Default  bool
}

// Text: the track converts to WebVTT. Otherwise it's pictures, and burned in.
func (s SubtitleFile) Text() bool { return mediatypes.IsTextSubtitle(s.Codec) }

const (
	maxSubtitleFiles  = 64
	maxSubtitleFolder = 4096 // entries looked through in one folder
)

// subtitleFolders are where downloads and rips keep them.
var subtitleFolders = []string{"subs", "subtitles", "sub", "subtitle"}

var subtitleCodecs = map[string]string{"srt": "subrip", "ass": "ass", "ssa": "ssa", "vtt": "webvtt", "sub": "microdvd", "sbv": "subrip", "ttml": "ttml", "dfxp": "dfxp",
	"smi": "sami", "lrc": "text", "mks": "subrip", "scc": "eia_608", "mpl2": "mpl2", "stl": "stl", "jss": "jacosub", "realtext": "realtext",
	"sup": "hdmv_pgs_subtitle", "idx": "dvd_subtitle"}

// FindSubtitles lists the subtitle files of a video: beside it with the same name
// ("Film.en.forced.srt"), and in a Subs folder beside it. A folder of subtitles counts
// whole when it's named after the video, or when this is the only video beside it.
func FindSubtitles(video string) []SubtitleFile {
	dir := filepath.Dir(video)
	base := filepath.Base(video)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	entries := listFolder(dir)
	var out []SubtitleFile
	videos := 0
	for _, e := range entries {
		if !e.IsDir() && mediatypes.IsVideo(e.Name()) {
			videos++
		}
	}
	add := func(folder, name, rest string) {
		if len(out) >= maxSubtitleFiles {
			return
		}
		if sf := readSubtitleName(filepath.Join(folder, name), rest); sf != nil {
			out = append(out, *sf)
		}
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		if !plainMode(e.Type()) || !mediatypes.IsSubtitle(name) || skippedVobSub(entries, name) {
			continue
		}
		if rest, ok := subtitleSuffix(name, stem); ok {
			add(dir, name, rest)
		}
	}
	for _, e := range entries {
		if !e.IsDir() || !isSubtitleFolder(e.Name()) {
			continue
		}
		folder := filepath.Join(dir, e.Name())
		inner := listFolder(folder)
		// Subs/<Film name>/English.srt
		for _, x := range inner {
			if x.IsDir() && strings.EqualFold(x.Name(), stem) {
				sub := filepath.Join(folder, x.Name())
				files := listFolder(sub)
				for _, f := range files {
					if !f.IsDir() && plainMode(f.Type()) && mediatypes.IsSubtitle(f.Name()) && !skippedVobSub(files, f.Name()) {
						add(sub, f.Name(), strings.TrimSuffix(f.Name(), filepath.Ext(f.Name())))
					}
				}
			}
		}
		for _, f := range inner {
			name := f.Name()
			if f.IsDir() || !plainMode(f.Type()) || !mediatypes.IsSubtitle(name) || skippedVobSub(inner, name) {
				continue
			}
			if rest, ok := subtitleSuffix(name, stem); ok {
				add(folder, name, rest)
			} else if videos == 1 {
				add(folder, name, strings.TrimSuffix(name, filepath.Ext(name)))
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func isSubtitleFolder(name string) bool {
	low := strings.ToLower(name)
	for _, f := range subtitleFolders {
		if low == f {
			return true
		}
	}
	return false
}

func listFolder(dir string) []os.DirEntry {
	fh, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer fh.Close()
	entries, _ := fh.ReadDir(maxSubtitleFolder)
	return entries
}

// subtitleSuffix: the name is this video's, with anything after it left over.
func subtitleSuffix(name, stem string) (string, bool) {
	s := strings.TrimSuffix(name, filepath.Ext(name))
	if len(s) < len(stem) || !strings.EqualFold(s[:len(stem)], stem) {
		return "", false
	}
	rest := s[len(stem):]
	if rest == "" {
		return "", true
	}
	if !strings.ContainsRune("._- [(", rune(rest[0])) {
		return "", false // "Film 2.srt" isn't a subtitle of "Film"
	}
	return rest[1:], true
}

// skippedVobSub: this .sub is the data of the .idx beside it, which is what's listed.
func skippedVobSub(entries []os.DirEntry, name string) bool {
	if !strings.EqualFold(filepath.Ext(name), ".sub") {
		return false
	}
	want := strings.TrimSuffix(name, filepath.Ext(name)) + ".idx"
	for _, e := range entries {
		if strings.EqualFold(e.Name(), want) {
			return true
		}
	}
	return false
}

// readSubtitleName works out a track's language and kind from what follows the
// video's name: "en", "eng.forced", "English SDH", "pt-BR.default".
func readSubtitleName(path, rest string) *SubtitleFile {
	f := mediatypes.Find(path, mediatypes.Subtitle)
	if f == nil {
		return nil
	}
	if f.Confirm && !mediatypes.Confirms(f, readHead(path)) {
		return nil
	}
	sf := &SubtitleFile{Path: path, Format: f.ID, Codec: subtitleCodecs[f.ID]}
	if sf.Codec == "" {
		return nil
	}
	var label []string
	for _, w := range strings.FieldsFunc(rest, func(r rune) bool { return strings.ContainsRune(`._ []()`, r) }) {
		switch low := strings.ToLower(w); low {
		case "forced", "foreign":
			sf.Forced = true
		case "sdh", "hi", "cc", "hearing":
			sf.SDH = true
		case "default":
			sf.Default = true
		default:
			if sf.Language == "" {
				if lang := playback.LanguageFromWord(low); lang != "" {
					sf.Language = lang
					continue
				}
			}
			label = append(label, w)
		}
	}
	sf.Title = strings.Join(label, " ")
	return sf
}
