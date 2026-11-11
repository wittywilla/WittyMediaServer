// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package mediaextras

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Making stems, for a song that came without any. There are two ways, and neither is
// built into the server:
//
//   - a separator the owner installed (Demucs, Spleeter, anything that takes a song and
//     writes its parts into a folder). These are neural networks with models of hundreds
//     of megabytes that want a fast processor or a graphics card, so the server neither
//     ships one nor runs one unless the owner names it on the machine itself;
//   - what FFmpeg alone can do: take the left channel from the right. Whatever is the same
//     in both, which in most studio recordings is the lead voice, cancels. It is quick
//     enough for a Raspberry Pi and it is approximate: it also removes anything else
//     mixed to the centre, leaves reverb and backing voices, turns the music mono, and
//     does nothing useful for mono or live recordings. It is always labelled as such.

// ApproxDir is the folder, in an item's folder of uploads, the approximate instrumental
// is kept in: apart from real stems, so it never passes for one.
const ApproxDir = "approx"

// ApproxExt is what the approximate instrumental is encoded as.
const ApproxExt = ".m4a"

// ApproxArgs are FFmpeg's arguments, after the input, that make the approximate
// instrumental: the difference of the two channels above 120 Hz, with the bass (which is
// mixed to the centre too, and which a song sounds hollow without) put back from the sum.
// Both output channels carry the same left-minus-right: with left-minus-right in one and
// right-minus-left in the other, as the trick is often written, a phone's single speaker
// adds the two and plays silence. The input must be stereo.
func ApproxArgs(out string) []string {
	const filter = "[0:a:0]aformat=channel_layouts=stereo,asplit=2[a][b];" +
		"[a]pan=stereo|c0=c0-c1|c1=c0-c1,highpass=f=120[side];" +
		"[b]pan=stereo|c0=0.5*c0+0.5*c1|c1=0.5*c0+0.5*c1,lowpass=f=120[bass];" +
		"[side][bass]amix=inputs=2:normalize=0[out]"
	return []string{"-filter_complex", filter, "-map", "[out]", "-vn", "-sn", "-dn", "-map_metadata", "-1",
		"-c:a", "aac", "-b:a", "160k", "-movflags", "+faststart", "-f", "mp4", out}
}

// ErrSeparator: the configured command can't be used.
var ErrSeparator = errors.New("the stem separator's command needs both {input} and {output}")

// SeparatorCommand turns the owner's command line into a program and its arguments for one
// song. The line is split into words the way a shell would (quotes keep spaces together)
// but no shell runs it: a song called "$(rm -rf ~).flac" is one argument and nothing more.
// {input} is the song and {output} an empty folder for the parts; both must be there.
func SeparatorCommand(words []string, input, output string) (string, []string, error) {
	if len(words) == 0 {
		return "", nil, ErrSeparator
	}
	hasIn, hasOut := false, false
	args := make([]string, 0, len(words)-1)
	for i, w := range words {
		if strings.Contains(w, "{input}") {
			hasIn = true
		}
		if strings.Contains(w, "{output}") {
			hasOut = true
		}
		w = strings.ReplaceAll(strings.ReplaceAll(w, "{input}", input), "{output}", output)
		if i > 0 {
			args = append(args, w)
		}
	}
	if !hasIn || !hasOut || strings.Contains(words[0], "{") {
		return "", nil, ErrSeparator
	}
	return words[0], args, nil
}

// maxSeparatorEntries bounds how much of a separator's output folder is looked at.
const maxSeparatorEntries = 500

// CollectStems finds the parts a separator wrote under dir, however deep it put them
// (Demucs writes <model>/<song>/vocals.wav, Spleeter <song>/accompaniment.wav), and moves
// each into dst as <part><ext>: only ordinary files, whose first bytes are audio, of at
// most MaxStem bytes, named for a part (see StemName). It answers the parts kept. The type
// in the name is what the bytes say, not what the separator called the file.
func CollectStems(dir, dst string) ([]string, error) {
	type found struct{ name, path, ext string }
	var parts []found
	seen, entries := map[string]bool{}, 0
	var walk func(d string, depth int)
	walk = func(d string, depth int) {
		for _, e := range readDir(d) {
			if entries++; entries > maxSeparatorEntries {
				return
			}
			p := filepath.Join(d, e.Name())
			if e.IsDir() {
				if depth < 4 && !strings.HasPrefix(e.Name(), ".") {
					walk(p, depth+1)
				}
				continue
			}
			name := StemName(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
			if name == "" || name == Multitrack || seen[name] {
				continue
			}
			f, fi, err := OpenPlain(p)
			if err != nil {
				continue
			}
			head := make([]byte, 64)
			n, _ := io.ReadFull(f, head)
			f.Close()
			ext := AudioExt(head[:n])
			if ext == "" || fi.Size() > MaxStem || fi.Size() == 0 {
				continue
			}
			seen[name] = true
			parts = append(parts, found{name, p, ext})
		}
	}
	walk(dir, 0)
	if len(parts) == 0 {
		return nil, errors.New("the separator finished but wrote no parts this server knows: it looks for vocals, no_vocals or accompaniment, drums, bass and other")
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return nil, err
	}
	var kept []string
	for _, p := range parts {
		// One file per part: an earlier one of another type goes.
		for _, ext := range StemExts {
			if old := filepath.Join(dst, p.name+ext); plainExists(old) {
				_ = os.Remove(old)
			}
		}
		to := filepath.Join(dst, p.name+p.ext)
		if err := moveFile(p.path, to); err != nil {
			return kept, err
		}
		kept = append(kept, p.name)
	}
	return kept, nil
}

// moveFile renames, or copies when the two are on different disks.
func moveFile(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	in, _, err := OpenPlain(from)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(to), ".wms-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, io.LimitReader(in, MaxStem+1)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), to)
}

// SeparatedList is the file, in an item's stems folder, that names the parts a separator
// made (one per line), so they can be told from ones an administrator uploaded.
const SeparatedList = ".separated"

// Separated reads which parts in an item's stems folder a separator made.
func Separated(stemsDir string) map[string]bool {
	out := map[string]bool{}
	b, err := os.ReadFile(filepath.Join(stemsDir, SeparatedList))
	if err != nil || len(b) > 4096 {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		if n := StemName(line); n != "" {
			out[n] = true
		}
	}
	return out
}
