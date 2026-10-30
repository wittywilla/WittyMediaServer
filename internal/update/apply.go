package update

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// StageDir is the folder inside the program folder where an update is
// downloaded and unpacked (on the same drive, so files move instantly).
const StageDir = ".update"

// oldList records the replaced files that are removed on the next start.
const oldList = "old-files.txt"

// skipPath: files a package must never bring into the program folder. The
// data folder holds the library, accounts and progress of a copy run in
// place, and wms.json is its owner's configuration.
func SkipPath(rel string) bool {
	rel = filepath.ToSlash(rel)
	top, _, _ := strings.Cut(rel, "/")
	return strings.EqualFold(top, "data") || strings.EqualFold(top, StageDir) || strings.EqualFold(rel, "wms.json") ||
		strings.HasSuffix(strings.ToLower(rel), ".old")
}

// within reports whether p is dir or inside it.
func within(p, dir string) bool {
	if dir == "" {
		return false
	}
	p, dir = filepath.Clean(p), filepath.Clean(dir)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		// Case-insensitive file systems: DATA\x is data\x.
		p, dir = strings.ToLower(p), strings.ToLower(dir)
	}
	r, err := filepath.Rel(dir, p)
	return err == nil && (r == "." || filepath.IsLocal(r))
}

// realPath resolves short (8.3) names, links and relative parts so paths
// can be compared; a path that does not exist is only made absolute.
func realPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

type move struct{ rel, src, dst, old string }

// Swap moves every file under staged into dir. A file it replaces is first
// renamed to <name>.old (Windows lets a running program be renamed, not
// overwritten) and deleted on the next start by Cleanup. Nothing inside
// protect, the data folder or wms.json is ever written, and files that are
// not in the package are left alone. If a step fails, every file is put
// back as it was.
func Swap(dir, staged string, protect []string) ([]string, error) {
	var plan []move
	err := filepath.WalkDir(staged, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("unexpected file in the update: %s", p)
		}
		rel, err := filepath.Rel(staged, p)
		if err != nil || !filepath.IsLocal(rel) {
			return fmt.Errorf("unexpected file in the update: %s", p)
		}
		if SkipPath(rel) {
			return nil
		}
		dst := filepath.Join(dir, rel)
		for _, pr := range protect {
			if within(dst, pr) {
				return fmt.Errorf("the update would write %s, inside %s; nothing was changed", dst, pr)
			}
		}
		if st, err := os.Lstat(dst); err == nil && !st.Mode().IsRegular() {
			return fmt.Errorf("%s is not a plain file; nothing was changed", dst)
		}
		plan = append(plan, move{rel: rel, src: p, dst: dst})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(plan) == 0 {
		return nil, errors.New("the update holds no files")
	}

	done := 0
	rollback := func() {
		for i := done - 1; i >= 0; i-- {
			m := plan[i]
			if m.old != "" {
				_ = os.Rename(m.dst, m.src) // the new file back to staging
				_ = os.Rename(m.old, m.dst)
			} else {
				_ = os.Rename(m.dst, m.src)
			}
		}
	}
	for i := range plan {
		m := &plan[i]
		if err := os.MkdirAll(filepath.Dir(m.dst), 0o755); err != nil {
			rollback()
			return nil, err
		}
		if _, err := os.Lstat(m.dst); err == nil {
			old, err := oldName(m.dst)
			if err != nil {
				rollback()
				return nil, err
			}
			if err := os.Rename(m.dst, old); err != nil {
				rollback()
				return nil, fmt.Errorf("could not set aside %s: %w", m.dst, err)
			}
			m.old = old
		}
		if err := os.Rename(m.src, m.dst); err != nil {
			if m.old != "" {
				_ = os.Rename(m.old, m.dst)
			}
			rollback()
			return nil, fmt.Errorf("could not put %s in place: %w", m.dst, err)
		}
		done++
	}
	var olds []string
	for _, m := range plan {
		if m.old != "" {
			if r, err := filepath.Rel(dir, m.old); err == nil {
				olds = append(olds, filepath.ToSlash(r))
			}
		}
	}
	return olds, nil
}

// oldName finds a free <file>.old name, removing a leftover one when it is
// no longer in use.
func oldName(p string) (string, error) {
	for i := 0; i < 10; i++ {
		c := p + ".old"
		if i > 0 {
			c = p + "." + strconv.Itoa(i) + ".old"
		}
		if _, err := os.Lstat(c); errors.Is(err, fs.ErrNotExist) {
			return c, nil
		}
		if os.Remove(c) == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("could not set aside %s: too many old copies are still in use", p)
}

// RecordOld adds replaced files to the list Cleanup removes.
func RecordOld(dir string, olds []string) error {
	stage := filepath.Join(dir, StageDir)
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(stage, oldList), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	for _, o := range olds {
		fmt.Fprintln(f, o)
	}
	return f.Close()
}

// Cleanup runs at start: it deletes the files an update set aside (those
// still in use stay listed for next time) and anything left in the staging
// folder by an update that did not finish. It only ever deletes *.old files
// it recorded and its own staging folder.
func Cleanup(dir string) {
	stage := filepath.Join(dir, StageDir)
	st, err := os.Lstat(stage)
	if err != nil || !st.IsDir() {
		return
	}
	var keep []string
	if f, err := os.Open(filepath.Join(stage, oldList)); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			rel := filepath.FromSlash(strings.TrimSpace(sc.Text()))
			if rel == "" || !filepath.IsLocal(rel) || !strings.HasSuffix(rel, ".old") || within(filepath.Join(dir, rel), filepath.Join(dir, "data")) {
				continue
			}
			p := filepath.Join(dir, rel)
			if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
				keep = append(keep, filepath.ToSlash(rel))
			}
		}
		f.Close()
	}
	entries, _ := os.ReadDir(stage)
	for _, e := range entries {
		if e.Name() != oldList {
			_ = os.RemoveAll(filepath.Join(stage, e.Name()))
		}
	}
	if len(keep) == 0 {
		_ = os.Remove(filepath.Join(stage, oldList))
		_ = os.Remove(stage)
		return
	}
	_ = os.WriteFile(filepath.Join(stage, oldList), []byte(strings.Join(keep, "\n")+"\n"), 0o644)
}
