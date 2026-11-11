// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package mediaextras

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Karaoke versions the server made by itself (the approximate pair, and parts a separator
// wrote) are a cache: they can always be made again. So they are kept under a size cap,
// and the ones sung least recently go first. What an administrator uploaded, and anything
// beside the media, is never counted and never removed.

// ApproxFull is the whole song, encoded in the same run as the approximate instrumental, so
// an app can blend the two sample for sample whatever format the song itself is in.
const ApproxFull = "full"

// usedMark is the file, in an item's folder, whose time is when its karaoke was last used.
const usedMark = ".karaoke-used"

// ApproxPairArgs are FFmpeg's arguments, after the input, that make the approximate
// instrumental (see ApproxArgs) and the whole song beside it, both stereo AAC of the same
// length and encoder delay.
func ApproxPairArgs(instrumental, full string) []string {
	const filter = "[0:a:0]aformat=channel_layouts=stereo,asplit=3[a][b][f];" +
		"[a]pan=stereo|c0=c0-c1|c1=c0-c1,highpass=f=120[side];" +
		"[b]pan=stereo|c0=0.5*c0+0.5*c1|c1=0.5*c0+0.5*c1,lowpass=f=120[bass];" +
		"[side][bass]amix=inputs=2:normalize=0[out]"
	enc := []string{"-vn", "-sn", "-dn", "-map_metadata", "-1", "-c:a", "aac", "-b:a", "160k", "-movflags", "+faststart", "-f", "mp4"}
	args := []string{"-filter_complex", filter, "-map", "[out]"}
	args = append(append(args, enc...), instrumental)
	args = append(args, "-map", "[f]")
	return append(append(args, enc...), full)
}

// CompactArgs are FFmpeg's arguments, after the input, that turn one separated part into
// AAC: a separator writes WAV, at 50 MB a part for a five-minute song, and four of those
// are more than a phone should download to sing one song.
func CompactArgs(out string) []string {
	return []string{"-map", "0:a:0", "-vn", "-sn", "-dn", "-map_metadata", "-1", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", "-f", "mp4", out}
}

// TouchKaraoke notes that an item's karaoke was used now, at most once an hour.
func TouchKaraoke(itemDir string) {
	p := filepath.Join(itemDir, usedMark)
	now := time.Now()
	if fi, err := os.Lstat(p); err == nil {
		if now.Sub(fi.ModTime()) < time.Hour {
			return
		}
		_ = os.Chtimes(p, now, now)
		return
	}
	if fi, err := os.Lstat(itemDir); err != nil || !fi.IsDir() {
		return
	}
	_ = os.WriteFile(p, nil, 0o644)
}

// madeFiles are the files in an item's folder the server made by itself.
func madeFiles(itemDir string) (files []string, bytes int64, newest time.Time) {
	add := func(p string) {
		fi, err := os.Lstat(p)
		if err != nil || !fi.Mode().IsRegular() {
			return
		}
		files = append(files, p)
		bytes += fi.Size()
		if fi.ModTime().After(newest) {
			newest = fi.ModTime()
		}
	}
	for _, e := range readDir(filepath.Join(itemDir, ApproxDir)) {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			add(filepath.Join(itemDir, ApproxDir, e.Name()))
		}
	}
	stems := filepath.Join(itemDir, "stems")
	if made := Separated(stems); len(made) > 0 {
		for _, e := range readDir(stems) {
			n := e.Name()
			if !e.IsDir() && !strings.HasPrefix(n, ".") && made[StemName(strings.TrimSuffix(n, filepath.Ext(n)))] {
				add(filepath.Join(stems, n))
			}
		}
	}
	return files, bytes, newest
}

// maxCacheItems bounds how many items' folders one pass looks at.
const maxCacheItems = 20000

// KaraokeUsage is how many bytes of karaoke the server has made and kept under root, and
// for how many songs.
func KaraokeUsage(root string) (bytes int64, songs int) {
	for i, e := range readDirN(root, maxCacheItems) {
		if i >= maxCacheItems || !e.IsDir() {
			continue
		}
		if _, b, _ := madeFiles(filepath.Join(root, e.Name())); b > 0 {
			bytes += b
			songs++
		}
	}
	return bytes, songs
}

// EvictKaraoke removes what the server made for the songs sung least recently until what
// is left is at most limit bytes. keep is an item's folder name that stays whatever its
// age: the one just made. It answers how many songs were cleared and the bytes freed.
func EvictKaraoke(root string, limit int64, keep string) (songs int, freed int64) {
	type entry struct {
		dir   string
		files []string
		bytes int64
		used  time.Time
	}
	var all []entry
	var total int64
	for _, e := range readDirN(root, maxCacheItems) {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		files, b, newest := madeFiles(dir)
		if b == 0 {
			continue
		}
		total += b
		if e.Name() == keep {
			continue
		}
		used := newest
		if fi, err := os.Lstat(filepath.Join(dir, usedMark)); err == nil && fi.ModTime().After(used) {
			used = fi.ModTime()
		}
		all = append(all, entry{dir, files, b, used})
	}
	if total <= limit {
		return 0, 0
	}
	sort.Slice(all, func(i, j int) bool { return all[i].used.Before(all[j].used) })
	for _, e := range all {
		if total <= limit {
			break
		}
		for _, f := range e.files {
			_ = os.Remove(f)
		}
		// The list of separated parts goes with them; the folders go if that left them empty.
		_ = os.Remove(filepath.Join(e.dir, "stems", SeparatedList))
		_ = os.Remove(filepath.Join(e.dir, usedMark))
		_ = os.Remove(filepath.Join(e.dir, ApproxDir))
		_ = os.Remove(filepath.Join(e.dir, "stems"))
		_ = os.Remove(e.dir)
		total -= e.bytes
		freed += e.bytes
		songs++
	}
	return songs, freed
}

func readDirN(dir string, n int) []os.DirEntry {
	f, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer f.Close()
	entries, _ := f.ReadDir(n)
	return entries
}

// separatorPart is the part a separator's own file name stands for, for tools that don't
// name files plainly: audio-separator writes "Song_(Vocals)_model.flac".
func separatorPart(base string) string {
	if n := StemName(base); n != "" {
		return n
	}
	i := strings.LastIndex(base, "_(")
	if i < 0 {
		return ""
	}
	rest := base[i+2:]
	j := strings.Index(rest, ")")
	if j < 0 {
		return ""
	}
	return StemName(rest[:j])
}
