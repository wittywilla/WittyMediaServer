package gamearchive

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// A .cue, .gdi or .m3u from an archive names other files. They have to be in the
// game's folder, or the core fails with nothing useful to say, and a hostile one may
// name anything at all. So each name is matched to a file that really arrived, and
// the list is rewritten if what it says isn't exactly that: dumps made on Windows
// often differ in case, or keep a whole path, and that fails on Linux.

const maxDescriptor = 1 << 20

var (
	cueLineRe = regexp.MustCompile(`(?i)^(\s*FILE\s+)("[^"]+"|\S+)(\s+[A-Z0-9]+\s*)$`)
	gdiLineRe = regexp.MustCompile(`^(\s*\d+\s+\d+\s+\d+\s+\d+\s+)("[^"]+"|\S+)(\s+\d+\s*)$`)
)

// checkReferences checks the file the core opens and whatever it lists. dir is the
// game's folder, files everything that arrived there.
func checkReferences(dir, entry string, files []string) error {
	switch strings.ToLower(path.Ext(entry)) {
	case ".m3u":
		discs, err := fixList(dir, entry, files, func(line string) (string, string, string, bool) {
			t := strings.TrimSpace(line)
			if t == "" || strings.HasPrefix(t, "#") {
				return "", "", "", false
			}
			name, label, _ := strings.Cut(t, "|")
			if label != "" {
				label = "|" + label
			}
			return "", name, label, true
		})
		if err != nil {
			return err
		}
		for _, d := range discs {
			// Never followed: one that names itself would go round until the server crashed.
			if strings.EqualFold(path.Ext(d), ".m3u") {
				return fmt.Errorf("the m3u in this archive names another list, %q, and only discs can be listed", path.Base(d))
			}
			if err := checkReferences(dir, d, files); err != nil {
				return err
			}
		}
	case ".cue":
		_, err := fixList(dir, entry, files, matcher(cueLineRe))
		return err
	case ".gdi":
		_, err := fixList(dir, entry, files, matcher(gdiLineRe))
		return err
	}
	return nil
}

func matcher(re *regexp.Regexp) func(string) (string, string, string, bool) {
	return func(line string) (string, string, string, bool) {
		m := re.FindStringSubmatch(line)
		if m == nil {
			return "", "", "", false
		}
		return m[1], m[2], m[3], true
	}
}

// fixList rewrites one list so every line names a file that's there, and returns those.
func fixList(dir, list string, files []string, parse func(string) (before, name, after string, ok bool)) ([]string, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(filepath.FromSlash(list))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxDescriptor+1))
	f.Close()
	if err != nil {
		return nil, err
	}
	if len(raw) > maxDescriptor {
		return nil, ErrTooBig
	}
	eol := "\n"
	if bytes.Contains(raw, []byte("\r\n")) {
		eol = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	var named []string
	changed := false
	for i, line := range lines {
		before, name, after, ok := parse(line)
		if !ok {
			continue
		}
		quoted := strings.HasPrefix(name, `"`) && strings.HasSuffix(name, `"`) && len(name) >= 2
		bare := strings.Trim(name, `"`)
		found := findFile(path.Dir(list), bare, files)
		if found == "" {
			return nil, fmt.Errorf("the %s in this archive names %q, which isn't in the archive", strings.ToLower(path.Ext(list)), path.Base(strings.ReplaceAll(bare, `\`, "/")))
		}
		named = append(named, found)
		rel := relativeTo(path.Dir(list), found)
		if rel != bare {
			if quoted || strings.Contains(rel, " ") {
				rel = `"` + rel + `"`
			}
			lines[i] = before + rel + after
			changed = true
		}
	}
	if !changed {
		return named, nil
	}
	return named, writeInRoot(root, list, []byte(strings.Join(lines, eol)))
}

// findFile matches a name in a list to a file that arrived: the exact one, the same
// ignoring case, or one with the same name anywhere in the game's folder when the
// list holds a path from somewhere else.
func findFile(dir, name string, files []string) string {
	want := path.Clean(path.Join(dir, strings.ReplaceAll(name, `\`, "/")))
	lower := strings.ToLower(want)
	for _, f := range files {
		if f == want {
			return f
		}
	}
	for _, f := range files {
		if strings.ToLower(f) == lower {
			return f
		}
	}
	base := strings.ToLower(path.Base(strings.ReplaceAll(name, `\`, "/")))
	found := ""
	for _, f := range files {
		if strings.ToLower(path.Base(f)) == base {
			if found != "" {
				return "" // more than one: it can't be known which
			}
			found = f
		}
	}
	return found
}

func relativeTo(dir, file string) string {
	if dir == "." {
		return file
	}
	rel, err := filepath.Rel(filepath.FromSlash(dir), filepath.FromSlash(file))
	if err != nil {
		return file
	}
	return filepath.ToSlash(rel)
}

func writeInRoot(root *os.Root, name string, data []byte) error {
	tmp := name + ".tmp"
	f, err := root.OpenFile(filepath.FromSlash(tmp), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = root.Remove(filepath.FromSlash(tmp))
		return err
	}
	return root.Rename(filepath.FromSlash(tmp), filepath.FromSlash(name))
}

// writePlaylist writes the list a core opens for a set of discs.
func writePlaylist(dir, entry string, discs []string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if _, err := root.Stat(filepath.FromSlash(entry)); err == nil {
		return errors.New("the playlist's name is taken")
	}
	return writeInRoot(root, entry, []byte(strings.Join(discs, "\n")+"\n"))
}
