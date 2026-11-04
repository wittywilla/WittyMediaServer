// Package appcontrol knows about Windows' Smart App Control: whether it's on, whether the
// programs and libraries about to be installed are signed so that it would let them run, and
// how to say so when it blocks one. On other systems it has nothing to say.
//
// Smart App Control lets a program or DLL load only if Microsoft's reputation service knows it,
// or it's signed by a publisher Windows trusts. There's no exception for one program, and once
// turned off it can't be turned on again without reinstalling Windows, so nothing here ever
// suggests turning it off.
package appcontrol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Smart App Control's states, as Windows Security names them.
const (
	Off        = "off"
	On         = "on"
	Evaluation = "evaluation" // watching, not blocking yet: it turns itself on or off later
)

// State is Smart App Control's state on this computer, or "" where there's no such thing (not
// Windows, or a Windows without it).
func State() string { return currentState() }

// currentState is a seam for tests, which can't turn Smart App Control on.
var currentState = state

// Words says the state for people.
func Words(s string) string {
	switch s {
	case On:
		return "On"
	case Evaluation:
		return "Evaluation: Windows is watching which programs run, and will turn it on or off by itself"
	case Off:
		return "Off"
	}
	return ""
}

// IsPE reports whether the file is a Windows program or library, by its first bytes rather than
// its name, so a renamed DLL isn't missed.
func IsPE(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var head [64]byte
	if _, err := io.ReadFull(f, head[:]); err != nil || head[0] != 'M' || head[1] != 'Z' {
		return false
	}
	off := int64(binary.LittleEndian.Uint32(head[0x3c:]))
	if off < 64 || off > 1<<20 {
		return false
	}
	var sig [4]byte
	if _, err := f.ReadAt(sig[:], off); err != nil {
		return false
	}
	return bytes.Equal(sig[:], []byte("PE\x00\x00"))
}

// Unsigned lists the Windows programs and libraries under dir (or dir itself, if it's a file)
// that Windows' own check doesn't find signed by a publisher it trusts, as paths relative to dir.
// Elsewhere it lists nothing.
func Unsigned(dir string) ([]string, error) {
	if !canVerify {
		return nil, nil
	}
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() || !IsPE(p) {
			return nil
		}
		if verify(p) != nil {
			rel, rerr := filepath.Rel(dir, p)
			if rerr != nil || rel == "." {
				rel = filepath.Base(p)
			}
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// Blocked is the answer when Smart App Control would block, or has blocked, a file.
type Blocked struct {
	What  string   // "this module", "this core"
	Files []string // the files it would block, or did
	Ran   bool     // Windows refused to start it, rather than a check before installing
	Fix   string   // what to do instead
}

func (b *Blocked) Error() string {
	files := b.Files
	more := ""
	if len(files) > 3 {
		more = fmt.Sprintf(" and %d more", len(files)-3)
		files = files[:3]
	}
	names := strings.Join(files, ", ") + more
	var s string
	if b.Ran {
		s = "Windows' Smart App Control blocked " + names + ": it isn't signed by a publisher Windows trusts."
	} else {
		s = "Windows' Smart App Control is on, and " + names + " in " + b.What + " isn't signed by a publisher Windows trusts, so Windows would block it. Nothing was installed."
	}
	if b.Fix != "" {
		s += " " + b.Fix
	}
	return s + " Turning Smart App Control off isn't the answer: Windows can't turn it back on without being reinstalled. Help, Troubleshooting, Smart App Control has more."
}

// CheckInstall answers before files are installed: a *Blocked when Smart App Control is on and
// any program or library under dir isn't signed, otherwise nil. While Smart App Control is only
// in evaluation, it blocks nothing, so neither does this.
func CheckInstall(dir, what, fix string) error {
	if State() != On {
		return nil
	}
	files, err := Unsigned(dir)
	if err != nil {
		return fmt.Errorf("checking the signatures of %s: %w", what, err)
	}
	if len(files) == 0 {
		return nil
	}
	return &Blocked{What: what, Files: files, Fix: fix}
}

// Explain turns a failure to start the program at path into a *Blocked when Windows refused it
// for its application control policy, which is what Smart App Control does. Other errors are
// returned as they are.
func Explain(err error, path, fix string) error {
	if err == nil || !policyBlock(err) {
		return err
	}
	var b *Blocked
	if errors.As(err, &b) {
		return err
	}
	return &Blocked{Files: []string{filepath.Base(path)}, Ran: true, Fix: fix}
}
