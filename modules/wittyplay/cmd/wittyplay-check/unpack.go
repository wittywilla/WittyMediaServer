package main

import (
	"archive/zip"
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittyplay"
)

func isArchive(ext string) bool { return ext == ".zip" || ext == ".7z" }

// listedFiles reads the files a .cue or .m3u names, which hides them from the
// game list as a library scan does.
func listedFiles(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	m3u := strings.EqualFold(filepath.Ext(path), ".m3u")
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if m3u {
			if line != "" && !strings.HasPrefix(line, "#") {
				out = append(out, line)
			}
			continue
		}
		if len(line) > 5 && strings.EqualFold(line[:5], "FILE ") {
			if a := strings.IndexByte(line, '"'); a >= 0 {
				if b := strings.LastIndexByte(line, '"'); b > a {
					out = append(out, line[a+1:b])
				}
			}
		}
	}
	return out
}

// unpack returns the file a core should open. Archives are unpacked in the run's
// own folder; the original folder is only ever read.
func unpack(c *config, path string, cn *wittyplay.Console, dir string) (string, func(), error) {
	none := func() {}
	ext := strings.ToLower(filepath.Ext(path))
	if !isArchive(ext) {
		return path, none, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", none, err
	}
	cleanup := func() { removeAll(dir) }
	var err error
	if ext == ".zip" {
		err = unzip(path, dir)
	} else {
		err = un7z(c, path, dir)
	}
	if err != nil {
		cleanup()
		return "", none, err
	}
	rom, err := bestGame(dir, cn)
	if err != nil {
		cleanup()
		return "", none, err
	}
	return rom, cleanup, nil
}

const maxUnpacked = 16 << 30

func unzip(path, dir string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()
	var total uint64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		total += f.UncompressedSize64
		if total > maxUnpacked {
			return errors.New("this archive is too large")
		}
		// Names from an archive become nothing but a base name in a folder of ours.
		out, err := os.Create(filepath.Join(dir, filepath.Base(filepath.FromSlash(f.Name))))
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func un7z(c *config, path, dir string) error {
	prog := c.sevenZip
	if prog == "" {
		for _, n := range []string{"7z", "7zz", "7za"} {
			if p, err := exec.LookPath(n); err == nil {
				prog = p
				break
			}
		}
	}
	if prog == "" {
		return errors.New("no 7-Zip program found; use -7z, or point the tool at unpacked copies")
	}
	cmd := exec.Command(prog, "e", "-y", "-bd", "-o"+dir, path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("7-Zip: %v: %s", err, truncate(string(out), 200))
	}
	return nil
}

// bestGame picks what to open from an unpacked archive: a list of discs before a
// cue sheet, and a cue sheet before the tracks it names.
func bestGame(dir string, cn *wittyplay.Console) (string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	rank := func(ext string) int {
		switch ext {
		case ".m3u":
			return 0
		case ".cue", ".gdi", ".ccd", ".toc":
			return 1
		case ".chd", ".pbp", ".iso", ".cso":
			return 2
		}
		return 3
	}
	var found []string
	for _, e := range ents {
		if !e.IsDir() && cn.Accepts(strings.ToLower(filepath.Ext(e.Name()))) {
			found = append(found, e.Name())
		}
	}
	if len(found) == 0 {
		return "", errors.New("this archive holds no game for " + cn.Name)
	}
	sort.Slice(found, func(i, j int) bool {
		ri, rj := rank(strings.ToLower(filepath.Ext(found[i]))), rank(strings.ToLower(filepath.Ext(found[j])))
		if ri != rj {
			return ri < rj
		}
		return found[i] < found[j]
	})
	return filepath.Join(dir, found[0]), nil
}

// removeAll tries again for a moment, because Windows holds a dead process's
// files open for a short while.
func removeAll(dir string) {
	for i := 0; i < 20; i++ {
		if os.RemoveAll(dir) == nil {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}
