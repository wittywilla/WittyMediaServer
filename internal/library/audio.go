package library

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	wittycd "github.com/wittywilla/wittymediaserver/internal/cd"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
	"github.com/wittywilla/wittymediaserver/internal/midi"
)

// Music, audiobooks and podcasts: audio files are indexed from their tags
// (falling back to folder and file names), with cover art from the folder
// or the picture embedded in the file. Audiobooks use the album tag (or the
// folder) as the book and the artist as the author.

// IsAudioLibrary reports whether a library type holds audio files.
func IsAudioLibrary(t string) bool { return t == "music" || t == "audiobooks" || t == "podcasts" }

func discoverAudio(root string) ([]candidate, error) {
	var out, sheets, raw []candidate
	var bad unreadable
	var unnamed sniffBudget
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			bad.note(err)
			return nil // unreadable folders are skipped, not fatal
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") || strings.EqualFold(name, "@eaDir")) {
				return filepath.SkipDir
			}
			return nil
		}
		if wittycd.IsImageName(name) && !strings.HasPrefix(name, "._") {
			if st, err := d.Info(); err == nil && mediaEntry(root, p, d) {
				sheets = append(sheets, candidate{path: p, kind: CDKind, size: st.Size(), mtime: st.ModTime().Unix()})
			}
			return nil
		}
		if wittycd.MayBeRaw(name) && !strings.HasPrefix(name, "._") {
			if st, err := d.Info(); err == nil && st.Size()%wittycd.SectorBytes == 0 && mediaEntry(root, p, d) {
				raw = append(raw, candidate{path: p, kind: CDKind, size: st.Size(), mtime: st.ModTime().Unix()})
			}
			return nil
		}
		if strings.HasPrefix(name, "._") {
			return nil
		}
		st, err := d.Info()
		if err != nil || !mediaEntry(root, p, d) {
			return nil
		}
		if mediatypes.IsPlaylist(name) {
			out = append(out, candidate{path: p, kind: playlistKind, size: st.Size(), mtime: st.ModTime().Unix()})
			return nil
		}
		format := ""
		if !mediatypes.IsAudio(name) || !nameHolds(p, name, mediatypes.Audio) {
			f := unnamed.sniff(p, name, st.Size(), mediatypes.Audio)
			if f == nil {
				return nil
			}
			format = f.ID
		}
		out = append(out, candidate{path: p, kind: "audio", size: st.Size(), mtime: st.ModTime().Unix(), format: format})
		return nil
	})
	// CD images, and what their sheets take out of the list of songs.
	images, claimed := cdCandidates(sheets)
	out = dropClaimed(out, claimed, images)
	raw = dropClaimed(raw, claimed, images)
	return append(append(out, images...), raw...), bad.with(err)
}

// tag reads a tag case-insensitively from the format, then the audio stream.
func tag(tags []map[string]string, keys ...string) string {
	for _, m := range tags {
		for k, v := range m {
			for _, want := range keys {
				if strings.EqualFold(k, want) && strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			}
		}
	}
	return ""
}

// num parses "3", "03" or "3/12".
func num(v string) *int {
	if i := strings.IndexByte(v, '/'); i >= 0 {
		v = v[:i]
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		return nil
	}
	return &n
}

// "01 - Title", "1. Title", "01 Title" -> track 1, "Title".
var trackPrefixRe = regexp.MustCompile(`^(\d{1,3})(?:[ ._-]+|\.\s*)(.+)$`)

func (s *Scanner) indexAudio(ctx context.Context, lib *Library, c candidate, covers map[string]string) error {
	if k := c.unchanged(); k != nil {
		if k.missing {
			return s.Store.restore(ctx, k.id)
		}
		return nil // unchanged
	}
	existing, err := s.Store.itemByPath(ctx, lib.ID, c.path)
	if err == nil && existing.MTime == c.mtime && existing.SizeBytes == c.size {
		if existing.Missing {
			return s.Store.restore(ctx, existing.ID)
		}
		return nil // unchanged
	}
	it := &Item{LibraryID: lib.ID, Kind: "audio", Path: c.path, SizeBytes: c.size, MTime: c.mtime}
	if existing != nil {
		it.ID, it.AddedAt = existing.ID, existing.AddedAt
	}
	dir := filepath.Dir(c.path)
	base := strings.TrimSuffix(filepath.Base(c.path), filepath.Ext(c.path))

	if isMIDI(c) {
		// FFmpeg doesn't read MIDI: its length and names come from the notes themselves.
		if info, err := midi.ReadFile(c.path); err == nil {
			it.RuntimeSec, it.Title = info.Seconds, info.Title
		} else {
			s.minorFailure(ctx, "could not read a file's details", err, "file", c.path)
		}
		it.Container, it.VideoCodec = "midi", "midi"
	}
	pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	var p *ffmpeg.Probe
	perr := errNoProbe
	if !isMIDI(c) {
		p, perr = s.Tools.ProbeFile(pctx, c.path)
	}
	cancel()
	embeddedArt := false
	if perr == nil {
		it.RuntimeSec = p.Duration()
		if it.RuntimeSec <= 0 {
			// A bare stream, or a song that only says how long once it's been played.
			mctx, done := context.WithTimeout(ctx, 2*time.Minute)
			it.RuntimeSec = s.Tools.MeasureDuration(mctx, c.path)
			done()
		}
		it.Container = strings.Split(p.Format.FormatName, ",")[0]
		tags := []map[string]string{p.Format.Tags}
		for _, st := range p.Streams {
			switch st.CodecType {
			case "audio":
				if it.VideoCodec == "" {
					it.VideoCodec = st.CodecName // the audio codec, for the direct-play decision
					tags = append(tags, st.Tags)
				}
			case "video":
				if st.Disposition["attached_pic"] == 1 {
					embeddedArt = true
				}
			}
		}
		it.Title = tag(tags, "title")
		it.Artist = tag(tags, "artist", "author")
		it.Album = tag(tags, "album")
		it.AlbumArtist = tag(tags, "album_artist", "album artist", "albumartist")
		it.Track = num(tag(tags, "track", "tracknumber"))
		it.DiscNo = num(tag(tags, "disc", "discnumber"))
		it.Overview = tag(tags, "comment", "description", "synopsis")
		it.recIDs = recordingIDs(tags)
		if g := tag(tags, "genre"); g != "" {
			it.Genres = []string{g}
		}
		if y := tag(tags, "date", "year", "originaldate"); len(y) >= 4 {
			if n, err := strconv.Atoi(y[:4]); err == nil && n > 1000 {
				it.Year = &n
			}
		}
	}
	// Names from the folder layout when tags are missing:
	// Artist/Album/01 - Title.ext (music), Author/Book/01 Chapter.ext (books).
	if it.Title == "" {
		it.Title = base
		if m := trackPrefixRe.FindStringSubmatch(base); m != nil {
			it.Title = strings.TrimSpace(m[2])
			if it.Track == nil {
				it.Track = num(m[1])
			}
		}
	}
	if it.Album == "" && !samePath(dir, lib.Paths...) {
		it.Album = filepath.Base(dir)
	}
	if it.AlbumArtist == "" {
		if it.Artist != "" {
			it.AlbumArtist = it.Artist
		} else if parent := filepath.Dir(dir); !samePath(parent, lib.Paths...) && !samePath(dir, lib.Paths...) {
			it.AlbumArtist = filepath.Base(parent)
		}
	}
	if it.Artist == "" {
		it.Artist = it.AlbumArtist
	}
	it.SortTitle = SortTitle(it.Title)

	// Cover: one per folder+album, shared by its tracks.
	key := dir + "|" + it.Album
	cover, seen := covers[key]
	if !seen {
		cover = albumCover(dir)
		if cover == "" && embeddedArt {
			h := sha1.Sum([]byte(key))
			name := "a-" + hex.EncodeToString(h[:10]) + ".jpg"
			out := filepath.Join(s.ImagesDir, name)
			if st, err := os.Stat(out); err == nil && st.Size() > 0 {
				cover = "cache:" + name
			} else {
				cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				if s.Tools.Still(cctx, ffmpeg.FileInput(c.path), 0, 600, out) == nil {
					if st, err := os.Stat(out); err == nil && st.Size() > 0 {
						cover = "cache:" + name
					}
				}
				cancel()
			}
		}
		covers[key] = cover
	}
	it.Poster = cover
	if err := s.Store.upsertItem(ctx, it); err != nil {
		return err
	}
	return s.saveRecordingIDs(ctx, lib, it)
}

var errNoProbe = errors.New("not read by FFmpeg")

func isMIDI(c candidate) bool {
	if c.format != "" {
		return c.format == "midi"
	}
	f := mediatypes.Find(c.path, mediatypes.Audio)
	return f != nil && f.ID == "midi"
}

func samePath(p string, roots ...string) bool {
	for _, r := range roots {
		if strings.EqualFold(filepath.Clean(p), filepath.Clean(r)) {
			return true
		}
	}
	return false
}

var coverNameRe = regexp.MustCompile(`(?i)^(cover|folder|front|album|albumart.*|poster|artwork)\.(jpe?g|jfif|png|webp|gif|avif|bmp|tbn)$`)

// albumCover picks the largest cover image in an album folder: some players
// leave a 200 px Folder.jpg next to a bigger
// AlbumArt_..._Large.jpg.
func albumCover(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	best, size := "", int64(0)
	for _, e := range entries {
		if e.IsDir() || !coverNameRe.MatchString(e.Name()) {
			continue
		}
		if info, err := e.Info(); err == nil && info.Size() > size {
			best, size = filepath.Join(dir, e.Name()), info.Size()
		}
	}
	return best
}

// MIDIAudio makes the sound of a MIDI file, once, and keeps it as FLAC in the cache:
// a browser plays that as it is, and can seek in it.
func (s *Scanner) MIDIAudio(ctx context.Context, it *Item) (string, error) {
	h := sha1.Sum([]byte(it.Path + "|" + strconv.FormatInt(it.MTime, 10) + "|" + strconv.FormatInt(it.SizeBytes, 10)))
	dir := filepath.Join(s.CacheDir, "midi")
	out := filepath.Join(dir, hex.EncodeToString(h[:12])+".flac")
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out, nil
	}
	midiRender.Lock()
	defer midiRender.Unlock()
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out, nil
	}
	song, err := midi.ReadFile(it.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp := out + ".tmp.flac"
	defer os.Remove(tmp)
	rctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := ffmpeg.Command(rctx, s.Tools.FFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-f", "s16le", "-ar", strconv.Itoa(midi.SampleRate), "-ac", "2",
		"-protocol_whitelist", "pipe", "-i", "pipe:0", "-c:a", "flac", tmp)
	in, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	rerr := song.Render(in)
	in.Close()
	if err := cmd.Wait(); err != nil {
		return "", err
	}
	if rerr != nil {
		return "", rerr
	}
	if err := os.Rename(tmp, out); err != nil {
		return "", err
	}
	trimCache(dir, maxConvertedCache)
	return out, nil
}

var midiRender sync.Mutex
