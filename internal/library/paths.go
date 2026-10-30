package library

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
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

// resolvesInside: path, with links followed, is a plain file in one of roots.
func resolvesInside(path string, roots []string) bool {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		// Some network and virtual drives can't be resolved. A link would have
		// been followed above, so what is left is checked as it stands.
		st, err := os.Lstat(path)
		return err == nil && plainMode(st.Mode())
	}
	st, err := os.Stat(real)
	if err != nil || !plainMode(st.Mode()) {
		return false
	}
	resolved := make([]string, 0, len(roots))
	for _, r := range roots {
		if rr, err := filepath.EvalSymlinks(r); err == nil {
			r = rr
		}
		resolved = append(resolved, r)
	}
	return inRoots(real, resolved)
}

// plainMode: a file with contents, not a folder, link, pipe or device. (Cloud
// placeholder files on Windows report as irregular and count as files.)
func plainMode(m fs.FileMode) bool {
	return m&(fs.ModeDir|fs.ModeSymlink|fs.ModeNamedPipe|fs.ModeSocket|fs.ModeDevice|fs.ModeCharDevice) == 0
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

// readPlain reads path unless it is a link or special file.
func readPlain(path string) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !plainMode(st.Mode()) {
		return nil, fs.ErrInvalid
	}
	return os.ReadFile(path)
}

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
