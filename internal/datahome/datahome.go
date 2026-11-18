// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package datahome chooses and moves the folder the server keeps its settings and data in.
//
// Where the folder is comes from config (resolveDefault): on a desktop install, the pointer that
// setup writes. Moving it is like a reset: a request waits in the data folder, the server restarts,
// and the next start does the work before anything opens a file. It copies, checks the copy, points
// at it, and only then deals with the old folder: kept until someone deletes it (Settings), or
// cleared at once in first-time setup, when there's nothing in it yet worth keeping.
package datahome

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/layout"
)

const (
	requestFile = "move-request.json"
	// ResultFile is the last move's result: in the new folder when it worked, in the old one when it didn't.
	ResultFile = "move-result.json"
	// MovedFile, in the new folder, says where the old copy is and what was in it.
	MovedFile = "moved-from.json"
	// NoteFile, in an old copy that was kept, says where everything went.
	NoteFile = "MOVED.txt"
	// marker is in a folder while a copy is being made in it: a copy cut short is ours to clear.
	marker = ".wms-moving"
)

// left are never copied: the running server's own files, a move's own files, and temporary video.
var left = map[string]bool{
	"instance.lock": true, "instance.json": true, "instance.json.tmp": true,
	requestFile: true, requestFile + ".tmp": true, ResultFile: true, MovedFile: true, NoteFile: true, marker: true,
	"transcode": true,
	// On a Mac the config folder and the old data folder are the same folder: the pointer and the
	// config file stay where they are read from.
	config.PointerName: true, config.PointerName + ".tmp": true, config.FileName: true,
}

// Request is a move waiting for the next start.
type Request struct {
	To      string    `json:"to"`
	Choice  string    `json:"choice"`  // documents, install or custom
	KeepOld bool      `json:"keepOld"` // false in first-time setup: the old folder is cleared
	By      string    `json:"by,omitempty"`
	At      time.Time `json:"at"`
}

// Result is how the last move went.
type Result struct {
	OK    bool      `json:"ok"`
	From  string    `json:"from"`
	To    string    `json:"to"`
	Error string    `json:"error,omitempty"`
	Files int       `json:"files"`
	Bytes int64     `json:"bytes"`
	At    time.Time `json:"at"`
}

// Moved is the old copy a move kept.
type Moved struct {
	From    string    `json:"from"`
	Entries []string  `json:"entries"` // what was copied out of it, and is all that deleting it removes
	Kept    bool      `json:"kept"`
	At      time.Time `json:"at"`
}

// Ask writes a request for the next start. A request already waiting is replaced.
func Ask(data string, r Request) error {
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	}
	r.To = filepath.Clean(r.To)
	if p := CheckTarget(data, r.To, 0); p != "" {
		return errors.New(p)
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(data, requestFile+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(data, requestFile))
}

// Pending is the move waiting, or nil.
func Pending(data string) *Request {
	var r Request
	if !readJSON(filepath.Join(data, requestFile), &r) || r.To == "" {
		return nil
	}
	return &r
}

// Cancel takes back a move that hasn't happened, and says if there was one.
func Cancel(data string) (bool, error) {
	err := os.Remove(filepath.Join(data, requestFile))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Last is the result of the last move that started from or arrived in data, or nil.
func Last(data string) *Result {
	var r Result
	if !readJSON(filepath.Join(data, ResultFile), &r) {
		return nil
	}
	return &r
}

// OldCopy is the copy a move left behind, while it's still there.
func OldCopy(data string) *Moved {
	var m Moved
	if !readJSON(filepath.Join(data, MovedFile), &m) || m.From == "" || !m.Kept {
		return nil
	}
	if st, err := os.Stat(m.From); err != nil || !st.IsDir() {
		return nil
	}
	return &m
}

func readJSON(path string, v any) bool {
	b, err := os.ReadFile(path)
	if err != nil || len(b) > 1<<20 {
		return false
	}
	return json.Unmarshal(b, v) == nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// within says path is parent or inside it. Windows and macOS don't mind case.
func within(parent, path string) bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		parent, path = strings.ToLower(parent), strings.ToLower(path)
	}
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// real is a path with its links followed, as far as it exists.
func real(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	rest := ""
	for p := abs; ; {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, rest)
		}
		up := filepath.Dir(p)
		if up == p {
			return abs
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = up
	}
}

// Same says two paths are the same folder.
func Same(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ra, rb := real(a), real(b)
	return within(ra, rb) && within(rb, ra)
}

// existing is the nearest folder at or above p that's there.
func existing(p string) string {
	for {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
		up := filepath.Dir(p)
		if up == p {
			return ""
		}
		p = up
	}
}

// Writable says the server's account can make files at p, or in the nearest folder above it
// that's there, which is where it would make p. It leaves nothing behind.
func Writable(p string) error {
	at := existing(p)
	if at == "" {
		return errors.New("neither the folder nor any folder above it is there")
	}
	f, err := os.CreateTemp(at, ".wms-write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// CheckTarget says, in a sentence, why to can't take the data now in data: "" when it can. need
// is how much room the data takes, 0 to leave room out of it.
func CheckTarget(data, to string, need int64) string {
	if to == "" || !filepath.IsAbs(to) {
		return "Type the folder's full path, such as " + example() + "."
	}
	to = filepath.Clean(to)
	if filepath.Dir(to) == to {
		return "Choose a folder, not the top of a drive."
	}
	rd, rt := real(data), real(to)
	switch {
	case Same(rd, rt):
		return "Your data is already in this folder."
	case within(rd, rt):
		return "That folder is inside the current data folder. Choose one outside it."
	case within(rt, rd):
		return "The current data folder is inside that folder. Choose an empty folder of its own."
	}
	// An installed package's folder is the program's: only its data folder may be in it. A program
	// built from source, or one file run from anywhere, has no folder of its own to keep clear.
	prog := real(config.ProgramDir())
	if within(rt, prog) || (layout.Current().Kind != layout.KindSource && within(prog, rt) && !Same(rt, real(config.InstallDataDir()))) {
		return "That's the program's own folder. Choose the install folder option, or a folder outside it."
	}
	if st, err := os.Stat(to); err == nil {
		if !st.IsDir() {
			return "There's a file with that name. Choose a folder."
		}
		ents, err := os.ReadDir(to)
		if err != nil {
			return "That folder can't be read: " + err.Error()
		}
		ours := false
		for _, e := range ents {
			if e.Name() == marker {
				ours = true
			}
		}
		if len(ents) > 0 && !ours {
			if _, err := os.Stat(filepath.Join(to, "wms.db")); err == nil {
				return "That folder already holds a WittyMediaServer data folder. Choose an empty folder, or a new one."
			}
			return "That folder has files in it. Choose an empty folder, or type a new one to make."
		}
	}
	if err := Writable(to); err != nil {
		return "The server can't write there: " + plainErr(err)
	}
	if need > 0 {
		if free := FreeBytes(to); free > 0 && free < need+need/10+64<<20 {
			return fmt.Sprintf("There isn't room there: your data needs about %s, and %s is free.", Size(need), Size(free))
		}
	}
	return ""
}

func example() string {
	switch runtime.GOOS {
	case "windows":
		return `D:\WittyMediaServer`
	case "darwin":
		return "/Users/you/WittyMediaServer"
	}
	return "/home/you/WittyMediaServer"
}

func plainErr(err error) string {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return "its account isn't allowed to."
	case errors.Is(err, fs.ErrNotExist):
		return "that drive or folder isn't there."
	}
	return err.Error()
}

// Size is a number of bytes in words.
func Size(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

// Sized is Footprint, remembered for half a minute: the page asks again as someone types a folder,
// and a big artwork cache takes a while to walk.
func Sized(data string) (files int, bytes int64) {
	sizedMu.Lock()
	defer sizedMu.Unlock()
	if sized.dir == data && time.Since(sized.at) < 30*time.Second {
		return sized.files, sized.bytes
	}
	files, bytes = Footprint(data)
	sized.dir, sized.at, sized.files, sized.bytes = data, time.Now(), files, bytes
	return files, bytes
}

var (
	sizedMu sync.Mutex
	sized   struct {
		dir   string
		at    time.Time
		files int
		bytes int64
	}
)

// Footprint is how much a move copies: everything but temporary video and the running server's
// own files.
func Footprint(data string) (files int, bytes int64) {
	ents, err := os.ReadDir(data)
	if err != nil {
		return 0, 0
	}
	for _, e := range ents {
		if left[e.Name()] {
			continue
		}
		_ = filepath.WalkDir(filepath.Join(data, e.Name()), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			if !d.IsDir() {
				if info, err := d.Info(); err == nil {
					files++
					bytes += info.Size()
				}
			}
			return nil
		})
	}
	return files, bytes
}
