package library

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Where library files may be read from, and how they are written. Everything
// here assumes the folders can hold anything: whoever can drop files into a
// shared folder must not get to read or overwrite the rest of the machine by
// leaving a link or a crafted sidecar there.

// Roots lists every library folder.
func (s *Store) Roots(ctx context.Context) []string {
	rows, err := s.DB.QueryContext(ctx, `SELECT path FROM library_paths`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if rows.Scan(&p) == nil {
			out = append(out, p)
		}
	}
	return out
}

func inRoots(path string, roots []string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)
	for _, root := range roots {
		if rel, err := filepath.Rel(root, clean); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

// InLibraries reports whether a path is inside a library folder, by its name
// alone. It touches nothing, so a path from an untrusted file can be checked
// without the machine contacting the server a network path names.
func (s *Store) InLibraries(ctx context.Context, path string) bool {
	return inRoots(path, s.Roots(ctx))
}

// LibraryFile reports whether path is a file inside a library folder once
// links are followed.
func (s *Store) LibraryFile(ctx context.Context, path string) bool {
	roots := s.Roots(ctx)
	if !inRoots(path, roots) {
		return false
	}
	return resolvesInside(path, roots)
}

// ErrNotInLibrary: the path isn't a plain file inside a library folder.
var ErrNotInLibrary = errors.New("not a file in a library")

// OpenLibraryFile opens a file inside a library folder. What it hands back is what was
// checked: a check followed by an open by name can be raced by anyone who can rename files
// in the folder, so the opening itself can't leave the library. Serve from this handle.
func (s *Store) OpenLibraryFile(ctx context.Context, path string) (*os.File, fs.FileInfo, error) {
	return openInRoots(path, s.Roots(ctx))
}

func openInRoots(path string, roots []string) (*os.File, fs.FileInfo, error) {
	if !inRoots(path, roots) {
		return nil, nil, ErrNotInLibrary
	}
	f, err := openThroughRoot(path, roots)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		// os.Root won't follow an absolute link, even to a place inside the library, or a link to
		// another library. Those are allowed, so where it leads is worked out, and that place is
		// then opened the same way: if it changes meanwhile, this still can't leave the library.
		real, within, ok := resolveInside(path, roots)
		if !ok {
			return nil, nil, ErrNotInLibrary
		}
		f, err = openThroughRoot(real, within)
		if err != nil && !errors.Is(err, fs.ErrNotExist) && runtime.GOOS == "windows" {
			// Windows' cloud-file placeholders are reparse points os.Root won't pass through,
			// and they aren't links. No link is left in real by now.
			f, err = os.Open(real)
		}
	}
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil || st.IsDir() || !plainMode(st.Mode()) {
		f.Close()
		return nil, nil, ErrNotInLibrary
	}
	return f, st, nil
}

// openThroughRoot opens path by way of the root that holds it, without blocking on a pipe.
func openThroughRoot(path string, roots []string) (*os.File, error) {
	root := rootOf(path, roots)
	if root == "" {
		return nil, ErrNotInLibrary
	}
	rel, err := filepath.Rel(root, filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return r.OpenFile(rel, os.O_RDONLY|openNoBlock, 0)
}

// rootOf is the folder of roots that holds path, by name.
func rootOf(path string, roots []string) string {
	for _, root := range roots {
		if inRoots(path, []string{root}) {
			return root
		}
	}
	return ""
}

// resolvesInside: path, with links followed, is a plain file in one of roots.
func resolvesInside(path string, roots []string) bool {
	_, _, ok := resolveInside(path, roots)
	return ok
}

// resolveInside finds where path really is. within is roots as they really are.
func resolveInside(path string, roots []string) (real string, within []string, ok bool) {
	real, err := filepath.EvalSymlinks(path)
	var st fs.FileInfo
	if err == nil {
		st, err = os.Stat(real)
	} else {
		// A junction, or a network or virtual drive: EvalSymlinks can't always get through these.
		// Below the library folder each step is looked at, and anything that can't be told apart
		// from a link stops here rather than passing as a file.
		if real, err = walkLinks(path, roots, 0); err == nil {
			st, err = os.Lstat(real)
			// No links are left, so this can only settle short names and case.
			if rr, rerr := filepath.EvalSymlinks(real); rerr == nil {
				real = rr
			}
		}
	}
	if err != nil || !plainMode(st.Mode()) {
		return "", nil, false
	}
	// As they were given and as they really are: a path that can't be resolved stays as given.
	within = make([]string, 0, 2*len(roots))
	for _, r := range roots {
		if rr, err := filepath.EvalSymlinks(r); err == nil && rr != r {
			within = append(within, rr)
		}
		within = append(within, r)
	}
	return real, within, inRoots(real, within)
}

// maxLinks is how many links one path may lead through.
const maxLinks = 16

// walkLinks follows path below the root that holds it, one step at a time. Since Go 1.23 a
// junction isn't reported as a link: it and a cloud file's placeholder both report as
// irregular, and the difference is that a junction can be read as a link. A path that leads
// out of roots comes back as where it leads, for the caller to refuse.
func walkLinks(path string, roots []string, depth int) (string, error) {
	if depth > maxLinks {
		return "", ErrNotInLibrary
	}
	clean := filepath.Clean(path)
	root := rootOf(clean, roots)
	if root == "" {
		return clean, nil
	}
	rel, err := filepath.Rel(root, clean)
	if err != nil {
		return "", err
	}
	cur := filepath.Clean(root)
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		if part == "." {
			continue
		}
		next := filepath.Join(cur, part)
		st, err := os.Lstat(next)
		if err != nil {
			return "", err
		}
		if st.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			if to, err := os.Readlink(next); err == nil {
				if !filepath.IsAbs(to) {
					to = filepath.Join(cur, to)
				}
				return walkLinks(filepath.Join(append([]string{to}, parts[i+1:]...)...), roots, depth+1)
			}
		}
		cur = next
	}
	return cur, nil
}

// plainMode: a file with contents, not a folder, link, pipe or device. (Cloud
// placeholder files on Windows report as irregular and count as files.)
func plainMode(m fs.FileMode) bool {
	return m&(fs.ModeDir|fs.ModeSymlink|fs.ModeNamedPipe|fs.ModeSocket|fs.ModeDevice|fs.ModeCharDevice) == 0
}

// plainFile: there is a file at path, not a folder, pipe or device.
func plainFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && plainMode(st.Mode())
}

// mediaEntry says whether a scan may index a directory entry under root: a
// plain file, or a link to one inside the same folder. A link leading out of
// the library would let anyone who can write there read any file on this
// machine as "a movie"; pipes and devices would hang whatever opens them.
func mediaEntry(root, p string, d fs.DirEntry) bool {
	if d.Type()&fs.ModeSymlink != 0 {
		return resolvesInside(p, []string{root})
	}
	return plainMode(d.Type())
}

// isPicture: the bytes start like a raster image (never SVG or HTML).
func isPicture(b []byte) bool {
	switch http.DetectContentType(b) {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp":
		return true
	}
	return len(b) > 12 && string(b[4:8]) == "ftyp" && (string(b[8:12]) == "avif" || string(b[8:12]) == "avis")
}

// writeBeside replaces path with data. It writes a new file and moves it into
// place, so a link left at path is replaced and never written through.
func writeBeside(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".wms-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, 0o644)
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// How much of a file beside the media is read. Real ones are kilobytes.
const (
	maxNFO     = 4 << 20
	maxSideDoc = 1 << 20
)

// ErrTooLarge: a file beside the media is bigger than any real one of its kind.
var ErrTooLarge = errors.New("too large")

// readSmall reads a file beside the media, which can be anything: a sparse file the size of
// the disk, a link to a device, or a pipe that nothing will ever write to, which would hold the
// scan for good. It reads only a plain file of at most limit bytes, opened without waiting,
// and a link isn't one. Scans come across these files by themselves, with nobody asking.
func readSmall(path string, limit int64) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !plainMode(st.Mode()) {
		return nil, fs.ErrInvalid
	}
	if st.Size() > limit {
		return nil, ErrTooLarge
	}
	f, err := os.OpenFile(path, os.O_RDONLY|openNoBlock, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// It may have been swapped for something else since.
	if st, err = f.Stat(); err != nil || !plainMode(st.Mode()) {
		return nil, fs.ErrInvalid
	}
	if st.Size() > limit {
		return nil, ErrTooLarge
	}
	var b bytes.Buffer
	b.Grow(int(st.Size()) + bytes.MinRead)
	n, err := b.ReadFrom(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if n > limit {
		return nil, ErrTooLarge
	}
	return b.Bytes(), nil
}

// readPlain reads path unless it is a link or special file, or too large to be an NFO.
func readPlain(path string) ([]byte, error) { return readSmall(path, maxNFO) }

// artRef checks a picture reference from a sidecar file: a web address, or a
// file named relative to the sidecar. Absolute paths are dropped: a sidecar
// must not point the server at other files on this machine or, on Windows, at
// a network share (just opening one sends the server's credentials there).
func artRef(ref, dir string) string {
	ref = strings.TrimSpace(ref)
	switch {
	case ref == "":
		return ""
	case webURL(ref) != "":
		return ref
	case strings.Contains(ref, ":") || strings.HasPrefix(ref, `\`) || strings.HasPrefix(ref, "/") || filepath.IsAbs(ref):
		return ""
	}
	p := filepath.Join(dir, filepath.FromSlash(ref))
	if !inRoots(p, []string{dir}) {
		return ""
	}
	return p
}
