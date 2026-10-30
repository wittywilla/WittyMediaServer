package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const maxEntries = 5000

// Extract unpacks a release package (.tar.gz, or else a zip, which may
// follow a program as in the Windows setup) into dest, without
// its top-level folder. Links, absolute paths and paths that climb out of
// dest are refused; anything a package must never bring (see SkipPath) is
// left out. At most maxTotal bytes are written.
func Extract(archive, dest string, maxTotal int64) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	x := &extractor{dest: dest, left: maxTotal}
	if strings.HasSuffix(archive, ".tar.gz") || strings.HasSuffix(archive, ".tgz") {
		return x.tarGz(archive)
	}
	// A zip, or a program with a zip appended (the Windows setup).
	return x.zip(archive)
}

type extractor struct {
	dest  string
	top   string
	left  int64
	count int
}

// rel turns an archive name into a path inside the package, or "" for the
// top-level folder itself.
func (x *extractor) rel(name string) (string, error) {
	if strings.ContainsAny(name, "\\:\x00") {
		return "", fmt.Errorf("unsafe name in package: %q", name)
	}
	clean := path.Clean(strings.TrimPrefix(name, "./"))
	if clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("unsafe name in package: %q", name)
	}
	top, rest, _ := strings.Cut(clean, "/")
	if x.top == "" {
		x.top = top
	} else if top != x.top {
		return "", errors.New("the package has more than one top-level folder")
	}
	if rest != "" && !filepath.IsLocal(filepath.FromSlash(rest)) {
		return "", fmt.Errorf("unsafe name in package: %q", name)
	}
	return rest, nil
}

func (x *extractor) count1() error {
	x.count++
	if x.count > maxEntries {
		return errors.New("the package has too many files")
	}
	return nil
}

func (x *extractor) write(rel string, mode os.FileMode, r io.Reader) error {
	if rel == "" {
		return errors.New("the package has a file outside its folder")
	}
	if SkipPath(rel) {
		return nil
	}
	p := filepath.Join(x.dest, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	perm := os.FileMode(0o644)
	if mode&0o111 != 0 {
		perm = 0o755
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, x.left+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	x.left -= n
	if x.left < 0 {
		return errors.New("the package is larger than expected")
	}
	return nil
}

func (x *extractor) zip(archive string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if err := x.count1(); err != nil {
			return err
		}
		rel, err := x.rel(f.Name)
		if err != nil {
			return err
		}
		mode := f.Mode()
		switch {
		case mode.IsDir():
			continue
		case !mode.IsRegular():
			return fmt.Errorf("the package holds a link or special file: %q", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = x.write(rel, mode, rc)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (x *extractor) tarGz(archive string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := x.count1(); err != nil {
			return err
		}
		rel, err := x.rel(h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
			if err := x.write(rel, os.FileMode(h.Mode), tr); err != nil {
				return err
			}
		default:
			return fmt.Errorf("the package holds a link or special file: %q", h.Name)
		}
	}
}
