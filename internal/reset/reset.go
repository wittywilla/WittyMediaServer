// Package reset puts a server back the way it was first installed, or back to a copy
// kept by an earlier reset. A running server has its files open, so asking and doing
// are apart: a request is written, and the next start carries it out, before anything
// opens the database.
//
// Only what is inside the server's own folders is touched, never a library's files.
package reset

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	requestFile = "reset-request.json"
	resultFile  = "reset-result.json"
	copyPrefix  = "before-reset-"
	stampFormat = "20060102-150405"
)

// What a request does. Modes other than these are refused.
const (
	ModeKeepCopy = "keep-copy" // everything moves into a copy inside the data folder
	ModeDelete   = "delete"    // everything goes for good, earlier copies too
	ModeRestore  = "restore"   // a copy comes back; what's there now becomes a new copy
)

var copyName = regexp.MustCompile(`^` + copyPrefix + `\d{8}-\d{6}$`)

// Folders are the places a server keeps its own things.
type Folders struct {
	Data    string
	Others  []string // cache, transcode and artwork folders, which may be set elsewhere
	Program string
	Config  string // the config file, which stays
}

// Request is what the next start is to do.
type Request struct {
	Mode    string    `json:"mode"`
	Copy    string    `json:"copy"`              // the copy to make, or for restore the one to bring back
	Save    string    `json:"save,omitempty"`    // restore: the copy to make of what's there now
	Outside []string  `json:"outside,omitempty"` // folders set elsewhere to clear
	At      time.Time `json:"at"`
}

// Result is how the last one went.
type Result struct {
	OK    bool      `json:"ok"`
	Mode  string    `json:"mode"`
	Copy  string    `json:"copy,omitempty"`
	Error string    `json:"error,omitempty"`
	Notes []string  `json:"notes,omitempty"`
	At    time.Time `json:"at"`
}

// Copy is one kept by an earlier reset.
type Copy struct {
	Name  string `json:"name"`
	At    int64  `json:"at"`
	Bytes int64  `json:"bytes"`
}

// heldOpen may be open while a reset happens, by whatever started the server and keeps
// its output: the tray app, for one. They move if they can, and stay behind if not.
var heldOpen = map[string]bool{"logs": true, "wms.log": true}

// retryFor is how long a file that's in use gets. The process before this one may
// still be closing its database, and a helper may still be on its way out.
var retryFor = 30 * time.Second

func within(parent, path string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func real(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	// A path may not exist yet: what's above it decides where it would be.
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

// Check says whether this install can be reset as it's set up, and which folders set
// elsewhere can go with it. Anyone's media inside the data folder, or the program
// itself, stops it: those aren't the server's to move.
func Check(f Folders, libraries []string) (outside []string, err error) {
	data := real(f.Data)
	if f.Program != "" && within(data, real(f.Program)) {
		return nil, errors.New("the program is inside the data folder, so the data folder can't be cleared. Set a data folder of its own")
	}
	if f.Config != "" && within(data, real(f.Config)) {
		return nil, fmt.Errorf("the config file %s is inside the data folder, so the data folder can't be cleared. Move it out first", f.Config)
	}
	libs := make([]string, 0, len(libraries))
	for _, l := range libraries {
		p := real(l)
		if within(data, p) {
			return nil, fmt.Errorf("the library folder %s is inside the data folder, so resetting would move your media. Move it out first", l)
		}
		libs = append(libs, p)
	}
	seen := map[string]bool{}
next:
	for _, o := range f.Others {
		p := real(o)
		if o == "" || seen[p] || within(data, p) || (f.Program != "" && within(p, real(f.Program))) || (f.Config != "" && within(p, real(f.Config))) || within(p, data) {
			continue
		}
		for _, l := range libs {
			if within(p, l) || within(l, p) {
				continue next
			}
		}
		seen[p] = true
		outside = append(outside, p)
	}
	sort.Strings(outside)
	return outside, nil
}

// Ask writes a request for the next start. A request already waiting is replaced.
func Ask(data string, r Request) error {
	switch r.Mode {
	case ModeKeepCopy, ModeDelete:
		r.Copy = copyPrefix + r.At.UTC().Format(stampFormat)
	case ModeRestore:
		if !copyName.MatchString(r.Copy) {
			return errors.New("no such copy")
		}
		if st, err := os.Stat(filepath.Join(data, r.Copy)); err != nil || !st.IsDir() {
			return errors.New("no such copy")
		}
		r.Save = copyPrefix + r.At.UTC().Format(stampFormat)
	default:
		return fmt.Errorf("no such kind of reset: %q", r.Mode)
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

// Cancel takes back a request that hasn't been carried out, and says if there was one.
func Cancel(data string) (bool, error) {
	err := os.Remove(filepath.Join(data, requestFile))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Pending is the request waiting, or nil.
func Pending(data string) *Request {
	b, err := os.ReadFile(filepath.Join(data, requestFile))
	if err != nil {
		return nil
	}
	var r Request
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return &r
}

// Last says how the last one went, or nil.
func Last(data string) *Result {
	b, err := os.ReadFile(filepath.Join(data, resultFile))
	if err != nil {
		return nil
	}
	var r Result
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return &r
}

// Copies are those kept in the data folder, newest first.
func Copies(data string) []Copy {
	out := []Copy{}
	entries, err := os.ReadDir(data)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() || !copyName.MatchString(e.Name()) {
			continue
		}
		c := Copy{Name: e.Name()}
		if t, err := time.ParseInLocation(stampFormat, strings.TrimPrefix(e.Name(), copyPrefix), time.UTC); err == nil {
			c.At = t.Unix()
		}
		_ = filepath.WalkDir(filepath.Join(data, e.Name()), func(_ string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if i, err := d.Info(); err == nil {
					c.Bytes += i.Size()
				}
			}
			return nil
		})
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out
}

// DeleteCopy removes one kept copy for good.
func DeleteCopy(data, name string) error {
	if !copyName.MatchString(name) {
		return errors.New("no such copy")
	}
	p := filepath.Join(data, name)
	if st, err := os.Stat(p); err != nil || !st.IsDir() {
		return errors.New("no such copy")
	}
	return os.RemoveAll(p)
}

// rename waits out a file that's still in use.
func rename(from, to string) error {
	deadline := time.Now().Add(retryFor)
	for {
		err := os.Rename(from, to)
		if err == nil || errors.Is(err, fs.ErrNotExist) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// moveAll moves what's in a folder into another, except what skip says stays, and
// undoes it all if anything can't move. It carries on from a start that was cut off.
func moveAll(from, to string, skip func(name string) bool) error {
	if err := os.MkdirAll(to, 0o750); err != nil {
		return err
	}
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	var moved []string
	for _, e := range entries {
		if skip(e.Name()) {
			continue
		}
		if err := rename(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
			for _, name := range moved {
				_ = os.Rename(filepath.Join(to, name), filepath.Join(from, name))
			}
			_ = os.Remove(to)
			return fmt.Errorf("%s couldn't be moved: %w", e.Name(), err)
		}
		moved = append(moved, e.Name())
	}
	return nil
}

// moveHeldOpen moves what it can of what may be held open, file by file, with no waiting.
func moveHeldOpen(from, to string) {
	for name := range heldOpen {
		src, dst := filepath.Join(from, name), filepath.Join(to, name)
		st, err := os.Stat(src)
		if err != nil {
			continue
		}
		if !st.IsDir() {
			_ = os.Rename(src, dst)
			continue
		}
		entries, err := os.ReadDir(src)
		if err != nil || os.MkdirAll(dst, 0o750) != nil {
			continue
		}
		for _, e := range entries {
			_ = os.Rename(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
		}
	}
}

// Apply carries out a waiting request, if there is one. It returns nil when there was
// none. On failure the install is left as it was, and the server starts as usual.
func Apply(f Folders) *Result {
	r := Pending(f.Data)
	if r == nil {
		if _, err := os.Stat(filepath.Join(f.Data, requestFile)); err == nil {
			_ = os.Remove(filepath.Join(f.Data, requestFile)) // not one this version can read
		}
		return nil
	}
	res := &Result{Mode: r.Mode, Copy: r.Copy, At: time.Now().UTC()}
	notCopy := func(keep ...string) func(string) bool {
		return func(name string) bool {
			if name == requestFile || name == requestFile+".tmp" || heldOpen[name] || copyName.MatchString(name) {
				return true
			}
			for _, k := range keep {
				if name == k {
					return true
				}
			}
			return false
		}
	}
	var err error
	if _, cerr := Check(Folders{Data: f.Data, Program: f.Program, Config: f.Config}, nil); cerr != nil {
		err = cerr
	} else {
		switch r.Mode {
		case ModeKeepCopy, ModeDelete:
			err = moveAll(f.Data, filepath.Join(f.Data, r.Copy), notCopy())
			if err == nil {
				moveHeldOpen(f.Data, filepath.Join(f.Data, r.Copy))
				clearOutside(r, res)
			}
		case ModeRestore:
			from := filepath.Join(f.Data, r.Copy)
			if !copyName.MatchString(r.Copy) {
				err = errors.New("no such copy")
				break
			}
			if _, serr := os.Stat(from); serr != nil {
				err = errors.New("that copy isn't there any more")
				break
			}
			if err = moveAll(f.Data, filepath.Join(f.Data, r.Save), notCopy()); err != nil {
				break
			}
			if err = moveAll(from, f.Data, func(name string) bool { return heldOpen[name] }); err != nil {
				_ = moveAll(filepath.Join(f.Data, r.Save), f.Data, func(string) bool { return false })
				break
			}
			moveHeldOpen(f.Data, filepath.Join(f.Data, r.Save))
			moveHeldOpen(from, f.Data)
			// Old logs that couldn't come back aren't worth keeping a folder for.
			for name := range heldOpen {
				_ = os.RemoveAll(filepath.Join(from, name))
			}
			_ = os.Remove(from)
			res.Notes = append(res.Notes, "What was here before is kept as "+r.Save+".")
		default:
			err = fmt.Errorf("no such kind of reset: %q", r.Mode)
		}
	}
	_ = os.Remove(filepath.Join(f.Data, requestFile))
	if err != nil {
		res.Error = err.Error()
	} else {
		res.OK = true
		if r.Mode == ModeDelete {
			res.Copy = ""
			for _, c := range Copies(f.Data) {
				if derr := DeleteCopy(f.Data, c.Name); derr != nil {
					res.Notes = append(res.Notes, c.Name+" couldn't be deleted: "+derr.Error())
				}
			}
		}
	}
	if b, jerr := json.MarshalIndent(res, "", "  "); jerr == nil {
		_ = os.WriteFile(filepath.Join(f.Data, resultFile), b, 0o600)
	}
	return res
}

// clearOutside moves the contents of folders set elsewhere aside. Those are never
// deleted, whatever was asked: someone chose those places, and may keep more there.
func clearOutside(r *Request, res *Result) {
	for _, dir := range r.Outside {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) == 0 {
			continue
		}
		name := copyPrefix + r.At.UTC().Format(stampFormat)
		if err := moveAll(dir, filepath.Join(dir, name), func(n string) bool { return copyName.MatchString(n) }); err != nil {
			res.Notes = append(res.Notes, dir+": "+err.Error())
			continue
		}
		res.Notes = append(res.Notes, "What was in "+dir+" is now in its "+name+" folder, and is yours to delete.")
	}
}
