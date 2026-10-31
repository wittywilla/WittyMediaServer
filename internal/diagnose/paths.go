package diagnose

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Where a disc check may look: inside the library folders or the data folder,
// and nowhere else. Every answer is decided here, on the server, after cleaning
// the path and resolving links in both the root and the target. A path that's
// outside is refused before anything on the disk is touched, so this can't be
// used to find out what exists elsewhere.

// ErrOutside is the plain refusal for anything outside the allowed places.
var ErrOutside = errors.New("WittyDiagnoser can only look inside your library folders and the server's data folder. Pick something from the list.")

// ErrMissing: the path is in an allowed place but can't be opened.
var ErrMissing = errors.New("That can't be found, or the server isn't allowed to open it.")

// ErrLinkOut: a link inside a disc folder leads out of the allowed places.
var ErrLinkOut = errors.New("A link in that folder leads outside your library folders and the data folder, so WittyDiagnoser won't follow it.")

// Roots are the places a disc check may look.
type Roots struct {
	raw      []string // cleaned, as configured
	resolved []string // with links followed
}

// NewRoots prepares a list of allowed folders. One that's unavailable right now
// still counts as written, but nothing resolves into it until it's back.
func NewRoots(folders []string) *Roots {
	r := &Roots{}
	for _, f := range folders {
		if f == "" || strings.ContainsRune(f, 0) {
			continue
		}
		f = filepath.Clean(f)
		if !filepath.IsAbs(f) {
			continue
		}
		r.raw = append(r.raw, f)
		if real, err := evalLinks(f); err == nil {
			r.resolved = append(r.resolved, real)
		}
	}
	return r
}

// Within says whether target is root or inside it: /media/a does not hold
// /media/ab, and on Windows case doesn't matter. Both paths should be clean.
func Within(root, target string) bool {
	if root == "" || target == "" {
		return false
	}
	rel, err := filepath.Rel(root, target) // ignores case on Windows
	if err != nil {
		return false
	}
	return rel == "." || filepath.IsLocal(rel)
}

// Overlap says whether one folder holds the other.
func Overlap(a, b string) bool { return Within(a, b) || Within(b, a) }

// looksWrong catches paths that are tricks before they're treated as paths: NUL
// bytes, Windows device and long-path namespaces, alternate data streams and
// paths relative to a drive. These are refused even when they'd resolve inside.
func looksWrong(p string) bool {
	if p == "" || strings.ContainsRune(p, 0) {
		return true
	}
	if runtime.GOOS != "windows" {
		return false
	}
	s := strings.ReplaceAll(p, "/", `\`)
	for _, pre := range []string{`\\?\`, `\\.\`, `\??\`} {
		if strings.HasPrefix(s, pre) {
			return true
		}
	}
	// The only colon a Windows path has is the drive's. Others are streams.
	if i := strings.IndexByte(s, ':'); i >= 0 && i != 1 || strings.Count(s, ":") > 1 {
		return true
	}
	return !strings.HasPrefix(s, `\\`) && !(len(s) >= 3 && s[1] == ':' && s[2] == '\\')
}

func (r *Roots) rawWithin(p string) bool {
	for _, root := range r.raw {
		if Within(root, p) {
			return true
		}
	}
	for _, root := range r.resolved {
		if Within(root, p) {
			return true
		}
	}
	return false
}

func (r *Roots) resolvedWithin(p string) bool {
	for _, root := range r.resolved {
		if Within(root, p) {
			return true
		}
	}
	return false
}

// Resolve checks a path from a request and gives the real one, with links
// followed. Anything outside the allowed places is ErrOutside.
func (r *Roots) Resolve(input string) (string, error) {
	if looksWrong(input) {
		return "", ErrOutside
	}
	clean := filepath.Clean(input)
	if !filepath.IsAbs(clean) || !r.rawWithin(clean) {
		return "", ErrOutside
	}
	real, err := evalLinks(clean)
	if err != nil {
		return "", ErrMissing
	}
	if !r.resolvedWithin(real) {
		return "", ErrOutside
	}
	if r.irregularOnTheWay(real) {
		return "", ErrOutside
	}
	return real, nil
}

// irregularOnTheWay looks at every folder from the root down to the target for
// reparse points that aren't links (Windows junctions and mounted folders):
// following links doesn't see through those, so a junction in a library could
// otherwise lead anywhere.
func (r *Roots) irregularOnTheWay(real string) bool {
	for _, root := range r.resolved {
		if !Within(root, real) {
			continue
		}
		rel, err := filepath.Rel(root, real)
		if err != nil || rel == "." {
			return false
		}
		p := root
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			p = filepath.Join(p, part)
			st, err := os.Lstat(p)
			if err != nil || st.Mode()&fs.ModeIrregular != 0 && r.junctionLeadsOut(p) {
				return true
			}
		}
		return false
	}
	return true
}

// junctionLeadsOut looks behind a reparse point. Those that can't be read as a
// link are things like cloud-file placeholders, which are ordinary files.
func (r *Roots) junctionLeadsOut(p string) bool {
	if _, err := os.Readlink(p); err != nil {
		return false
	}
	real, err := evalLinks(p)
	return err != nil || !r.resolvedWithin(real)
}

// evalLinks is filepath.EvalSymlinks that also sees through Windows junctions and
// mounted folders. It treats those as ordinary folders at the end of a path, and
// gives up at them in the middle ("not a directory"). A library on a mounted drive
// is ordinary, and a junction leading out must be seen as out.
func evalLinks(p string) (string, error) {
	if runtime.GOOS != "windows" {
		return filepath.EvalSymlinks(p)
	}
	return evalWindowsLinks(p, 0)
}

func evalWindowsLinks(p string, depth int) (string, error) {
	if depth > 16 {
		return "", errors.New("too many links")
	}
	vol := filepath.VolumeName(p)
	cur := vol + string(filepath.Separator)
	for _, part := range strings.Split(strings.Trim(p[len(vol):], string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		next := filepath.Join(cur, part)
		st, err := os.Lstat(next)
		if err != nil {
			return "", err
		}
		if st.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			// What can't be read as a link is some other reparse point, such as a
			// cloud file's placeholder: an ordinary file or folder.
			if to, err := os.Readlink(next); err == nil {
				if !filepath.IsAbs(to) {
					to = filepath.Join(cur, to)
				}
				if cur, err = evalWindowsLinks(filepath.Clean(to), depth+1); err != nil {
					return "", err
				}
				continue
			}
		}
		cur = next
	}
	// No links are left: this only settles short names and case.
	if real, err := filepath.EvalSymlinks(cur); err == nil {
		return real, nil
	}
	return cur, nil
}

// CheckInnerLinks refuses a disc folder with a link inside that leads out of the
// allowed places: the engines would follow it. Regular links that stay inside
// are fine.
func (r *Roots) CheckInnerLinks(dir string) error {
	const maxEntries = 20000
	seen := 0
	var walk func(p string, depth int) error
	walk = func(p string, depth int) error {
		ents, err := os.ReadDir(p)
		if err != nil {
			return nil // unreadable: the check reports that itself
		}
		for _, e := range ents {
			seen++
			if seen > maxEntries || depth > 6 {
				return nil
			}
			full := filepath.Join(p, e.Name())
			switch {
			case e.Type()&fs.ModeSymlink != 0:
				real, err := evalLinks(full)
				if err != nil {
					continue // a broken link leads nowhere
				}
				if !r.resolvedWithin(real) {
					return ErrLinkOut
				}
			case e.Type()&fs.ModeIrregular != 0:
				if r.junctionLeadsOut(full) {
					return ErrLinkOut
				}
			case e.IsDir():
				if err := walk(full, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(dir, 0)
}

// plainErr takes the path out of a file error, leaving only what went wrong.
func plainErr(err error) string {
	if err == nil {
		return ""
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.Err.Error()
	}
	return err.Error()
}
