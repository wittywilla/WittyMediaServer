package diagnose

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

// Nothing a fix changes is lost. Before the first fix, the database is copied to a folder
// in <data>/backups, and anything a fix would remove moves there instead of being deleted.
// The newest few of these folders are kept.

const (
	safetyPrefix = "before-repair-"
	safetyKeep   = 5
)

// BackupsDir is where safety copies go.
func BackupsDir(data string) string { return filepath.Join(data, "backups") }

// FixContext is what one batch of fixes shares.
type FixContext struct {
	env     *DoctorEnv
	copyDir string
	// Option is the other way of fixing a person chose, if any.
	Option  string
	restart bool
}

// ensureCopy makes the safety copy, the first time it's needed.
func (f *FixContext) ensureCopy(ctx context.Context) error {
	if f.copyDir != "" {
		return nil
	}
	e := f.env
	root := BackupsDir(e.Data)
	if err := os.MkdirAll(root, 0o750); err != nil {
		return err
	}
	stamp := e.now().UTC().Format("20060102-150405")
	dir := filepath.Join(root, safetyPrefix+stamp)
	for n := 1; ; n++ {
		err := os.Mkdir(dir, 0o750)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrExist) || n > 50 {
			return err
		}
		dir = filepath.Join(root, fmt.Sprintf("%s%s-%d", safetyPrefix, stamp, n))
	}
	if err := copyDatabase(ctx, e.Data, dir); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	// What else a fix may rewrite.
	for _, name := range []string{filepath.Join("modules", "state.json"), "reset-request.json"} {
		_ = copyFile(filepath.Join(e.Data, name), filepath.Join(dir, "files", name))
	}
	f.copyDir = dir
	pruneSafetyCopies(root, safetyKeep, dir)
	return nil
}

// copyDatabase prefers a clean, consistent copy from SQLite itself. A damaged file can't
// give one, so then the files themselves are copied, as they are. There being no database
// yet is fine: there's nothing to keep.
func copyDatabase(ctx context.Context, data, dir string) error {
	src := filepath.Join(data, "wms.db")
	st, err := os.Stat(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	// Room for the copy, and a little over.
	if free := sysinfo.FreeBytes(data); free > 0 && free < st.Size()+st.Size()/5 {
		return fmt.Errorf("the disk has %d MB free and the database is %d MB", free>>20, st.Size()>>20)
	}
	d, err := safeOpen(data, false)
	if err == nil {
		_, err = d.ExecContext(ctx, `VACUUM INTO ?`, filepath.Join(dir, "wms.db"))
		_ = d.Close()
	}
	if err == nil {
		return nil
	}
	_ = os.Remove(filepath.Join(dir, "wms.db"))
	for _, name := range []string{"wms.db", "wms.db-wal", "wms.db-shm"} {
		if err := copyFile(filepath.Join(data, name), filepath.Join(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s couldn't be copied: %w", name, err)
		}
	}
	return nil
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
		return err
	}
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// pruneSafetyCopies removes the oldest, and never the one just made. The stamp in a name
// sorts by age.
func pruneSafetyCopies(root string, keep int, current string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	var names []string
	for _, d := range entries {
		if d.IsDir() && strings.HasPrefix(d.Name(), safetyPrefix) {
			names = append(names, d.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for n, name := range names {
		if n < keep || filepath.Join(root, name) == current {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, name))
	}
}

// SafetyCopies lists the kept copies, newest first.
func SafetyCopies(data string) []string {
	entries, err := os.ReadDir(BackupsDir(data))
	if err != nil {
		return nil
	}
	var names []string
	for _, d := range entries {
		if d.IsDir() && strings.HasPrefix(d.Name(), safetyPrefix) {
			names = append(names, d.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names
}

// Remove moves a file or folder into the safety copy, keeping where it came from
// recognisable. A fix must use this instead of deleting anything.
func (f *FixContext) Remove(p string) error {
	if f.copyDir == "" {
		return errors.New("no safety copy was made")
	}
	rel := relInside(f.env.Data, p)
	dst := filepath.Join(f.copyDir, "removed", rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	err := os.Rename(p, dst)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	// Another drive: copy, and only then remove.
	st, serr := os.Lstat(p)
	if serr != nil {
		return serr
	}
	if st.IsDir() {
		if err := copyTree(p, dst); err != nil {
			return err
		}
		return os.RemoveAll(p)
	}
	if err := copyFile(p, dst); err != nil {
		return err
	}
	return os.Remove(p)
}

// relInside is p as a name under the safety copy: relative to the data folder, or, for a
// folder set elsewhere, its own name under "outside".
func relInside(data, p string) string {
	if rel, err := filepath.Rel(data, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return rel
	}
	return filepath.Join("outside", filepath.Base(filepath.Dir(p)), filepath.Base(p))
}

func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(to, rel), 0o750)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(p, filepath.Join(to, rel))
	})
}

// Writes a fix runs in a transaction of its own, on a connection of its own.
func withWrite(ctx context.Context, e *DoctorEnv, fn func(d *db.DB) error) error {
	d, err := e.writeDB()
	if err != nil {
		return err
	}
	defer d.Close()
	return fn(d)
}

// olderThan: a file old enough that nothing can still be writing it.
func olderThan(info fs.FileInfo, now time.Time, age time.Duration) bool {
	return now.Sub(info.ModTime()) > age
}
