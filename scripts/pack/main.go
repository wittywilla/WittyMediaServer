// Command pack writes a release archive (.zip or .tar.gz) of a directory with
// correct Unix permissions on any build machine. Executables are recognised
// by their content (ELF, Mach-O, PE, "#!" scripts), because Windows and
// MSYS cannot record the executable bit for Linux/macOS/FreeBSD binaries.
//
//	go run ./scripts/pack <dir> <out.zip|out.tar.gz>
//
// The archive contains <dir>'s base name as its top-level folder.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var magics = [][]byte{
	{0x7f, 'E', 'L', 'F'},                              // ELF (Linux, FreeBSD)
	{0xcf, 0xfa, 0xed, 0xfe}, {0xce, 0xfa, 0xed, 0xfe}, // Mach-O 64/32 little endian
	{0xfe, 0xed, 0xfa, 0xcf}, {0xfe, 0xed, 0xfa, 0xce}, // Mach-O big endian
	{0xca, 0xfe, 0xba, 0xbe}, // Mach-O universal
	{'M', 'Z'},               // Windows PE (harmless, keeps .exe marked)
	{'#', '!'},               // scripts
}

func executable(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 4)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	for _, m := range magics {
		if bytes.HasPrefix(head, m) {
			return true
		}
	}
	return false
}

type entry struct {
	rel  string // slash path inside the archive
	full string
	dir  bool
	mode int64
}

func collect(dir string) ([]entry, error) {
	base := filepath.Base(dir)
	var out []entry
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		name := path.Join(base, filepath.ToSlash(rel))
		if d.IsDir() {
			out = append(out, entry{rel: name + "/", full: p, dir: true, mode: 0o755})
			return nil
		}
		mode := int64(0o644)
		if executable(p) {
			mode = 0o755
		}
		out = append(out, entry{rel: name, full: p, mode: mode})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out, err
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/pack <dir> <out.zip|out.tar.gz>")
		os.Exit(2)
	}
	dir, out := os.Args[1], os.Args[2]
	entries, err := collect(dir)
	if err != nil {
		fail(err)
	}
	f, err := os.Create(out + ".part")
	if err != nil {
		fail(err)
	}
	mtime := time.Now()
	switch {
	case strings.HasSuffix(out, ".zip"):
		zw := zip.NewWriter(f)
		for _, e := range entries {
			h := &zip.FileHeader{Name: e.rel, Method: zip.Deflate, Modified: mtime}
			if e.dir {
				h.Method = zip.Store
				h.SetMode(fs.ModeDir | 0o755)
			} else {
				h.SetMode(fs.FileMode(e.mode))
			}
			w, err := zw.CreateHeader(h)
			if err != nil {
				fail(err)
			}
			if !e.dir {
				copyFile(w, e.full)
			}
		}
		check(zw.Close())
	case strings.HasSuffix(out, ".tar.gz"), strings.HasSuffix(out, ".tgz"):
		gz, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
		tw := tar.NewWriter(gz)
		for _, e := range entries {
			h := &tar.Header{Name: e.rel, Mode: e.mode, ModTime: mtime, Format: tar.FormatPAX}
			if e.dir {
				h.Typeflag = tar.TypeDir
			} else {
				st, err := os.Stat(e.full)
				if err != nil {
					fail(err)
				}
				h.Typeflag, h.Size = tar.TypeReg, st.Size()
			}
			check(tw.WriteHeader(h))
			if !e.dir {
				copyFile(tw, e.full)
			}
		}
		check(tw.Close())
		check(gz.Close())
	default:
		fail(fmt.Errorf("unknown archive type: %s", out))
	}
	check(f.Close())
	check(os.Rename(out+".part", out))
}

func copyFile(w io.Writer, p string) {
	src, err := os.Open(p)
	if err != nil {
		fail(err)
	}
	defer src.Close()
	if _, err := io.Copy(w, src); err != nil {
		fail(err)
	}
}

func check(err error) {
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "pack:", err)
	os.Exit(1)
}
