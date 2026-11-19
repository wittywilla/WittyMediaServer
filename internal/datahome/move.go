// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package datahome

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

// FreeBytes is the room on the drive p is, or would be, on.
func FreeBytes(p string) int64 {
	at := existing(p)
	if at == "" {
		return 0
	}
	return sysinfo.FreeBytes(at)
}

// Progress is how far a move is, for the page that waits for it.
type Progress struct {
	Done  atomic.Int64
	Total atomic.Int64
}

// Apply carries out a move waiting in data, before the server opens anything there. It says
// whether the data is now somewhere else, and where. A move that fails leaves everything where it
// was, says why in data's ResultFile, and isn't tried again.
func Apply(data string, prog *Progress) (moved bool, to string, res *Result) {
	req := Pending(data)
	if req == nil {
		return false, "", nil
	}
	res = &Result{From: data, To: req.To, At: time.Now().UTC()}
	fail := func(err error) (bool, string, *Result) {
		res.Error = err.Error()
		_ = writeJSON(filepath.Join(data, ResultFile), res)
		_, _ = Cancel(data)
		return false, "", res
	}
	if p := CheckTarget(data, req.To, 0); p != "" {
		return fail(errors.New(p))
	}
	files, total := Footprint(data)
	if free := FreeBytes(req.To); free > 0 && free < total+64<<20 {
		return fail(fmt.Errorf("there isn't room in %s: the data needs %s, and %s is free", req.To, Size(total), Size(free)))
	}
	if prog != nil {
		prog.Total.Store(total)
	}
	made := !dirExists(req.To)
	if err := os.MkdirAll(req.To, 0o750); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(req.To, marker), nil, 0o600); err != nil {
		return fail(err)
	}
	undo := func(err error) (bool, string, *Result) {
		clearCopy(req.To, made)
		return fail(err)
	}
	ents, err := os.ReadDir(data)
	if err != nil {
		return undo(err)
	}
	var copied []string
	for _, e := range ents {
		if left[e.Name()] {
			continue
		}
		if err := copyTree(filepath.Join(data, e.Name()), filepath.Join(req.To, e.Name()), prog); err != nil {
			return undo(fmt.Errorf("copying %s: %w", e.Name(), err))
		}
		copied = append(copied, e.Name())
	}
	for _, name := range copied {
		if err := verifyTree(filepath.Join(data, name), filepath.Join(req.To, name)); err != nil {
			return undo(fmt.Errorf("the copy didn't match, so nothing was changed: %w", err))
		}
	}
	sort.Strings(copied)
	m := Moved{From: data, Entries: copied, Kept: req.KeepOld, At: res.At}
	if err := writeJSON(filepath.Join(req.To, MovedFile), m); err != nil {
		return undo(err)
	}
	// From here on the new folder is the one: the pointer says so, and the copy is no longer partial.
	if err := config.WritePointer(config.Pointer{DataDir: req.To, Choice: req.Choice, At: res.At}); err != nil {
		return undo(fmt.Errorf("the new place couldn't be saved in %s: %w", config.PointerFile(), err))
	}
	_ = os.Remove(filepath.Join(req.To, marker))
	res.OK, res.Files, res.Bytes = true, files, total
	_ = writeJSON(filepath.Join(req.To, ResultFile), res)
	_, _ = Cancel(data)
	_ = os.Remove(filepath.Join(data, ResultFile))
	if req.KeepOld {
		note := fmt.Sprintf("WittyMediaServer copied this folder to\r\n\r\n    %s\r\n\r\non %s, and uses that folder now.\r\n\r\n"+
			"This is the old copy, kept in case you need it. Delete it from Settings, Data folder, once you're happy, or delete the\r\n"+
			"folder yourself. Nothing reads it any more.\r\n", req.To, res.At.Local().Format("2 January 2006 at 15:04"))
		_ = os.WriteFile(filepath.Join(data, NoteFile), []byte(note), 0o600)
	} else {
		removeEntries(data, append(copied, "transcode"))
	}
	return true, req.To, res
}

// DeleteOld deletes the old copy a move kept: only what was copied out of it, and the folder
// itself once it's empty. Anything else someone put there stays.
func DeleteOld(data string) (*Moved, error) {
	m := OldCopy(data)
	if m == nil {
		return nil, errors.New("there's no old copy")
	}
	if Same(m.From, data) || within(real(m.From), real(data)) || within(real(data), real(m.From)) {
		return nil, errors.New("the old copy overlaps the data folder, so it was left alone")
	}
	removeEntries(m.From, append(m.Entries, leftBehind...))
	m.Kept = false
	if err := writeJSON(filepath.Join(data, MovedFile), m); err != nil {
		return m, err
	}
	return m, nil
}

// Tidy finishes clearing an old folder that a move was meant to clear, when something still had a
// file in it open at the time (the tray app's console log, on Windows).
func Tidy(data string) {
	var m Moved
	if !readJSON(filepath.Join(data, MovedFile), &m) || m.Kept || m.From == "" || !dirExists(m.From) {
		return
	}
	if Same(m.From, data) || within(real(m.From), real(data)) || within(real(data), real(m.From)) {
		return
	}
	removeEntries(m.From, append(m.Entries, leftBehind...))
}

// leftBehind are the old folder's files that weren't copied, and go with it: temporary video, the
// note, an earlier move's own records, and the lock the server that made the move held until it
// stopped, which nothing holds now.
var leftBehind = []string{"transcode", NoteFile, MovedFile, ResultFile, "instance.lock", "instance.json"}

// removeEntries deletes the named entries of dir, and dir once nothing else is in it.
func removeEntries(dir string, names []string) {
	for _, n := range names {
		if n == "" || n != filepath.Base(n) || n == "." || n == ".." || n == config.PointerName || n == config.FileName {
			continue
		}
		_ = os.RemoveAll(filepath.Join(dir, n))
	}
	_ = os.Remove(dir) // only when empty
}

// clearCopy takes away a copy cut short.
func clearCopy(to string, made bool) {
	ents, err := os.ReadDir(to)
	if err != nil {
		return
	}
	for _, e := range ents {
		_ = os.RemoveAll(filepath.Join(to, e.Name()))
	}
	if made {
		_ = os.Remove(to)
	}
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// copyTree copies a file or folder. Links aren't followed or copied: nothing the server makes is one.
func copyTree(src, dst string, prog *Progress) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return nil
		case d.IsDir():
			return os.MkdirAll(target, 0o750)
		case !d.Type().IsRegular():
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if err := copyFile(p, target, info); err != nil {
			return err
		}
		if prog != nil {
			prog.Done.Add(info.Size())
		}
		return nil
	})
}

func copyFile(src, dst string, info fs.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}

// verifyTree checks every file arrived whole: the same size, and for the database and the
// settings, the same bytes.
func verifyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		a, err := d.Info()
		if err != nil {
			return err
		}
		b, err := os.Stat(target)
		if err != nil {
			return fmt.Errorf("%s is missing", rel)
		}
		if a.Size() != b.Size() {
			return fmt.Errorf("%s is %d bytes, not %d", filepath.Join(filepath.Base(src), rel), b.Size(), a.Size())
		}
		switch filepath.Base(p) {
		case "wms.db", "wms.db-wal", "settings.ini":
			ha, err := hashFile(p)
			if err != nil {
				return err
			}
			hb, err := hashFile(target)
			if err != nil {
				return err
			}
			if !bytes.Equal(ha, hb) {
				return fmt.Errorf("%s differs", filepath.Base(p))
			}
		}
		return nil
	})
}

func hashFile(p string) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
