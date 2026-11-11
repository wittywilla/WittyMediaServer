package metafolder

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// DirName is the folder inside the data folder.
const DirName = "metadata"

// Limits on walking a folder someone may have filled with anything.
const (
	MaxFiles = 500_000
	maxWalk  = 6
	// StateFile says which files are as this server last wrote or applied them, so a
	// file nobody touched can't undo a change made since, and an export never writes over
	// someone's unsaved work. It starts with a dot, so it isn't part of the folder's files.
	StateFile = ".exported.json"
	maxState  = 128 << 20
)

// Entry is one thing found in the folder.
type Entry struct {
	Rel  string // from the folder, with forward slashes
	Abs  string
	Size int64
	Kind string // json, nfo, picture
}

// IsCanvasName says a file is a title's looping video ("My Film [item-12]-canvas.mp4"),
// which the server plays behind the title and this folder's own check leaves alone.
func IsCanvasName(name string) bool {
	low := strings.ToLower(name)
	return strings.HasSuffix(low, "-canvas.mp4") || strings.HasSuffix(low, "-canvas.webm")
}

// ignoredName: what editors and systems leave around, and says nothing.
func ignoredName(name string) bool {
	low := strings.ToLower(name)
	switch low {
	case "readme.txt", "thumbs.db", "desktop.ini":
		return true
	}
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "~") || strings.HasSuffix(name, "~") || strings.HasSuffix(low, ".bak") || strings.HasSuffix(low, ".tmp") || strings.HasSuffix(low, ".swp")
}

// Walk goes through a folder without following anything that leads elsewhere: links,
// junctions and other reparse points are reported, and never entered. The folder itself
// may be reached through a link, since that's the owner's own setup.
func Walk(root string, each func(Entry)) []Problem {
	var ps []Problem
	count := 0
	full := false
	var walk func(abs, rel string, depth int)
	walk = func(abs, rel string, depth int) {
		if full {
			return
		}
		entries, err := os.ReadDir(abs)
		if err != nil {
			ps = append(ps, Problem{File: relOr(rel), Msg: "This folder can't be read: " + plainFileError(err), Severity: SevError})
			return
		}
		for _, e := range entries {
			name := e.Name()
			childRel := name
			if rel != "" {
				childRel = rel + "/" + name
			}
			if ignoredName(name) {
				continue
			}
			fi, err := e.Info()
			if err != nil {
				continue // gone since it was listed
			}
			child := filepath.Join(abs, name)
			switch {
			case fi.Mode()&os.ModeSymlink != 0 || isReparsePoint(fi):
				ps = append(ps, Problem{File: childRel, Msg: "This is a link to somewhere else, which isn't followed. Put a real file here instead", Severity: SevWarning})
			case fi.IsDir():
				if depth >= maxWalk {
					ps = append(ps, Problem{File: childRel, Msg: "This folder is nested too deeply, so it was left out", Severity: SevWarning})
					continue
				}
				walk(child, childRel, depth+1)
			case !fi.Mode().IsRegular():
				ps = append(ps, Problem{File: childRel, Msg: "This isn't an ordinary file, so it was left out", Severity: SevWarning})
			default:
				ext := extOf(name)
				kind := ""
				switch {
				case ext == ".json":
					kind = "json"
				case ext == ".nfo":
					kind = "nfo"
				case pictureNames[ext]:
					kind = "picture"
				case IsCanvasName(name):
					continue // a looping video for the title, read by the server's media extras
				case ext == ".svg":
					ps = append(ps, Problem{File: childRel, Msg: "SVG pictures aren't accepted, because they can hold scripts. Use a JPEG, PNG, WebP or GIF", Severity: SevWarning})
					continue
				default:
					ps = append(ps, Problem{File: childRel, Msg: "This folder only uses .json and .nfo files and pictures, so this one is ignored", Severity: SevWarning})
					continue
				}
				if count++; count > MaxFiles {
					full = true
					ps = append(ps, Problem{File: relOr(rel), Msg: fmt.Sprintf("This folder has more than %d files, and the rest weren't looked at", MaxFiles), Severity: SevError})
					return
				}
				each(Entry{Rel: childRel, Abs: child, Size: fi.Size(), Kind: kind})
			}
		}
	}
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return []Problem{{File: ".", Msg: "This folder can't be read: " + plainFileError(err), Severity: SevError}}
	}
	walk(root, "", 0)
	return ps
}

func relOr(rel string) string {
	if rel == "" {
		return "."
	}
	return rel
}

// plainFileError leaves out the path, which the problem already gives.
func plainFileError(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

// ReadDoc reads and checks one entry of the folder.
func ReadDoc(e Entry) (*Doc, []Problem) {
	b, err := ReadEntry(e)
	if err != nil {
		return &Doc{File: e.Rel, Kind: e.Kind, Fields: map[string]string{}, Art: map[string]Art{}, Lines: map[string]int{}},
			[]Problem{{File: e.Rel, Msg: "This file can't be read: " + plainFileError(err), Severity: SevError}}
	}
	return ParseBytes(e, b)
}

// ReadEntry reads an entry's bytes, within the limit for its kind.
func ReadEntry(e Entry) ([]byte, error) {
	max := int64(MaxJSON)
	if e.Kind == "nfo" {
		max = MaxNFO
	}
	return ReadPlain(e.Abs, max)
}

// ParseBytes checks an entry's contents, already read. A file that doesn't say what it's
// for is a problem here and not in the parsers, which also read NFOs beside media that
// have no id.
func ParseBytes(e Entry, b []byte) (*Doc, []Problem) {
	var d *Doc
	var ps []Problem
	if e.Kind == "nfo" {
		d, ps = ParseNFO(e.Rel, b)
	} else {
		d, ps = ParseJSON(e.Rel, b)
	}
	if !HasErrors(ps) && (d.ID == 0 || d.Target == "") {
		ps = append(ps, Problem{File: e.Rel, Msg: "This file doesn't say which title it's for. Its name has to end in [item-123] or [show-22], or it needs \"target\" and \"id\" inside, as an export writes", Severity: SevError})
	}
	return d, ps
}

// CheckDir reads everything in a folder and says what's wrong with it, and doesn't need
// a database: it works on a copy, or when the server can't start. The pictures a file
// names, and any that lie around, are checked to be real pictures.
func CheckDir(root string) []Problem {
	ps, _ := CheckDirCount(root)
	return ps
}

// CheckDirCount is CheckDir, and how many files there were.
func CheckDirCount(root string) ([]Problem, int) {
	var out []Problem
	files := 0
	targets := map[string]string{}
	walkProblems := Walk(root, func(e Entry) {
		files++
		if e.Kind == "picture" {
			if _, _, err := ReadPicture(e.Abs); err != nil {
				out = append(out, Problem{File: e.Rel, Msg: "This picture can't be used: " + plainFileError(err), Severity: SevError})
			}
			return
		}
		d, ps := ReadDoc(e)
		out = append(out, ps...)
		if HasErrors(ps) {
			return
		}
		key := d.Target + "-" + fmt.Sprint(d.ID)
		if other, dup := targets[key]; dup {
			out = append(out, Problem{File: e.Rel, Line: d.Lines["id"], Field: "id", Msg: "This file and " + other + " describe the same title. Delete one of them", Severity: SevError})
		} else {
			targets[key] = e.Rel
		}
		dir := path.Dir(e.Rel)
		for f, a := range d.Art {
			if a.File == "" {
				continue
			}
			rel := path.Join(dir, a.File)
			if _, _, err := ReadPicture(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
				out = append(out, Problem{File: e.Rel, Line: d.Lines[f], Field: f, Msg: "\"" + f + "\" names " + a.File + ", which can't be used: " + plainFileError(err), Severity: SevError})
			}
		}
	})
	out = append(out, walkProblems...)
	SortProblems(out)
	return out, files
}

// SortProblems puts the worst first, then by file and line.
func SortProblems(ps []Problem) {
	sort.SliceStable(ps, func(i, j int) bool {
		a, b := ps[i], ps[j]
		if a.Severity != b.Severity {
			return a.Severity == SevError
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
}

// HashOf is what the state file remembers of a file.
func HashOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:16])
}

// State is the record of what this server last wrote or applied: file to hash.
type State map[string]string

// LoadState reads the record. A missing or damaged one is an empty one, which only means
// every file counts as edited.
func LoadState(root string) State {
	s := State{}
	b, err := ReadPlain(filepath.Join(root, StateFile), maxState)
	if err != nil {
		return s
	}
	if json.Unmarshal(b, &s) != nil {
		return State{}
	}
	return s
}

// Save writes the record.
func (s State) Save(root string) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(map[string]string(s)); err != nil {
		return err
	}
	return WriteAtomic(root, StateFile, buf.Bytes())
}

// SafeDir makes a folder under root, and refuses if any step on the way is a link or
// isn't a folder, so nothing can be written outside by planting one.
func SafeDir(root, rel string) (string, error) {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return "", err
	}
	abs := root
	if rel == "" || rel == "." {
		return abs, nil
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." {
			return "", errors.New("that isn't a folder inside this one")
		}
		abs = filepath.Join(abs, part)
		fi, err := os.Lstat(abs)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if err := os.Mkdir(abs, 0o750); err != nil {
				return "", err
			}
		case err != nil:
			return "", err
		case fi.Mode()&os.ModeSymlink != 0 || isReparsePoint(fi) || !fi.IsDir():
			return "", fmt.Errorf("%s is a link or a file, not a folder, so nothing is written there", part)
		}
	}
	return abs, nil
}

// WriteAtomic writes a file under root at a path with forward slashes: a temporary file
// that's renamed over it, so a file is never half written, and a link planted where it
// goes is replaced rather than written through.
func WriteAtomic(root, rel string, data []byte) error {
	dir, err := SafeDir(root, path.Dir(rel))
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".wms-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_ = os.Chmod(name, 0o640)
	return os.Rename(name, filepath.Join(dir, path.Base(rel)))
}

// Field is one line of a file an export writes.
type Field struct {
	Name  string
	Value any
}

// Encode writes a title's file: fields in a fixed order, one a line, as plain as JSON
// gets, so it's easy to read and a small edit is a small difference.
func Encode(list []Field) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, f := range list {
		var v bytes.Buffer
		enc := json.NewEncoder(&v)
		enc.SetEscapeHTML(false)
		enc.SetIndent("  ", "  ")
		if err := enc.Encode(f.Value); err != nil {
			return nil, err
		}
		k, err := json.Marshal(f.Name)
		if err != nil {
			return nil, err
		}
		b.WriteString("  ")
		b.Write(k)
		b.WriteString(": ")
		b.Write(bytes.TrimRight(v.Bytes(), "\n"))
		if i < len(list)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	return b.Bytes(), nil
}

// FileNameFor is a title's file, from its name and id.
func FileNameFor(name, target string, id int64) string {
	return fmt.Sprintf("%s [%s-%d].json", SafeName(name), target, id)
}

// FolderNameFor is a show's folder, from its name and id.
func FolderNameFor(name string, id int64) string {
	return fmt.Sprintf("%s [show-%d]", SafeName(name), id)
}

// DroppedPicture looks for a picture someone put beside a file for one field, without
// naming it in the file: "My Film [item-1]-poster.jpg", or "poster.jpg" beside show.json.
// It answers the file's name, or "".
func DroppedPicture(dirAbs, docBase, field string) string {
	stem := strings.TrimSuffix(docBase, path.Ext(docBase))
	prefix := stem + "-" + field
	if docBase == "show.json" {
		prefix = field
	}
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif"} {
		name := prefix + ext
		if fi, err := os.Lstat(filepath.Join(dirAbs, name)); err == nil && PlainFile(fi) {
			return name
		}
	}
	return ""
}

// Readme is written at the top of the folder, for whoever opens it.
const Readme = `WittyMediaServer metadata folder
================================

This folder holds one plain text file for each film, show and episode, so you can
fix what the server knows about them with any text editor. Nothing here changes
anything until you apply it.

HOW TO FIX SOMETHING

  1. Open the file for the title, in Notepad, TextEdit or any text editor.
  2. Change what's wrong, and save it. Keep the "quotes" and the commas.
  3. In Settings > Repair, press "Check what would change". On the command
     line: wittymediaserver metadata apply
  4. Read what it says it will change, and press Apply. On the command line:
     wittymediaserver metadata apply --apply

Anything you change this way is locked, so no scan or lookup changes it back. Every
change can be undone under Fix matches > History.

WHAT'S WHERE

  Movies/My Film (2001) [item-123].json        one film
  Movies/My Film (2001) [item-123]-poster.jpg  a picture for it, if you put one here
  Shows/A Show [show-22]/show.json             one show
  Shows/A Show [show-22]/poster.jpg            a picture for the show
  Shows/A Show [show-22]/S01E01 Pilot [item-311].json  one episode

The number in [square brackets] is how the server knows which title a file is for. Leave
it alone, along with "target", "id" and "path". You can rename the rest of the file.

WHAT YOU CAN CHANGE

  title, sortTitle, year, overview, tagline, genres, tags, contentRating, rating,
  studios, network, status, director, edition, aired, season, episode, episodeEnd,
  poster, backdrop, banner, logo, locked, provider, providerId

Text goes in "quotes". A number has none. A list goes in [square brackets]. Delete a
line to leave that thing as it is. Empty "quotes" clear it, except for a title.

A picture is a web address (http or https), or the plain name of a JPEG, PNG, WebP or GIF
in the same folder as the file.

WHEN IT SAYS SOMETHING IS WRONG

Messages name the file and the line, such as

  Movies/My Film (2001) [item-123].json:7: A comma is missing at the end of the line
  before this one

Look at that line, and the one before it. A file that's wrong is left out, and the others
still apply. Delete a file and export again to start that one over.

SAFE BY DESIGN

HTML, scripts, hidden characters and links are taken out of anything read from here, and
pictures are checked to be real pictures. There's nothing you can break here that a
new export won't put right.

More help: Settings > Repair, or Help in the app: The metadata folder.
`
