package wittyplay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"sync/atomic"
	"unicode"
)

// Custom consoles let an administrator use a core for a system the table above
// doesn't know. Everything in one comes from a person's form or a pasted JSON file,
// so it is checked as untrusted input here, once, whichever way it arrived.
//
// They are kept apart from Consoles on purpose: the table is what this project
// offers and has checked the licences of, and Offered and CoreLicense read only it.

// CustomPrefix starts every custom console's ID, so one can never collide with a
// console added to the table in a later version.
const CustomPrefix = "custom-"

// MaxCustomConsoleJSON is the most JSON one console may be sent as.
const MaxCustomConsoleJSON = 64 << 10

// MaxCustomConsoles bounds what the scanner and every console list has to go through.
const MaxCustomConsoles = 200

// CustomConsole is one as a person or a JSON file describes it. Cores names cores
// from this server's own list to offer for it, besides custom cores assigned to it.
type CustomConsole struct {
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name"`
	Short      string   `json:"short"`
	Extensions []string `json:"extensions"`
	Aliases    []string `json:"aliases,omitempty"`
	BIOS       []BIOS   `json:"bios,omitempty"`
	PadLike    string   `json:"padLike,omitempty"` // a console whose controller to copy; a whole RetroPad if empty
	Cores      []string `json:"cores,omitempty"`
}

var (
	customIDRe    = regexp.MustCompile(`^custom-[a-z0-9][a-z0-9-]{0,47}$`)
	customExtRe   = regexp.MustCompile(`^\.[a-z0-9][a-z0-9_+-]{0,15}$`)
	customCoreRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	biosSegmentRe = regexp.MustCompile(`^[^\x00-\x1f<>:"|?*\\/]{1,120}$`)
)

// ParseCustomConsole reads one console from JSON. Unknown fields are mistakes worth
// saying so about, rather than something quietly dropped.
func ParseCustomConsole(raw []byte) (*CustomConsole, error) {
	if len(raw) > MaxCustomConsoleJSON {
		return nil, errors.New("this is too long to be one console")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var c CustomConsole
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("this isn't a console as JSON: %v", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("this holds more than one console; add them one at a time")
	}
	return &c, nil
}

// Validate checks a custom console against everything else and returns it ready to
// use. id is the console being edited, or "" for a new one. existing is the other
// custom consoles.
func (in *CustomConsole) Validate(id string, existing []Console) (*Console, error) {
	name, err := cleanLabel("name", in.Name, 80)
	if err != nil {
		return nil, err
	}
	short, err := cleanLabel("short name", in.Short, 24)
	if err != nil {
		return nil, err
	}
	others := append(append([]Console(nil), Consoles...), existing...)
	for i := range others {
		c := &others[i]
		if c.ID == id || c.Hidden {
			continue
		}
		if normalise(c.Name) == normalise(name) {
			return nil, fmt.Errorf("%q is already a console here", name)
		}
		if normalise(c.Short) == normalise(short) && strings.HasPrefix(c.ID, CustomPrefix) {
			return nil, fmt.Errorf("another custom console is already called %q", short)
		}
	}
	if id == "" {
		if id = in.ID; id != "" {
			if !customIDRe.MatchString(id) {
				return nil, fmt.Errorf("a custom console's ID must look like %sname", CustomPrefix)
			}
		} else {
			id = newCustomID(short, others)
		}
		for i := range others {
			if others[i].ID == id {
				return nil, fmt.Errorf("the ID %q is already taken", id)
			}
		}
	}
	if normalise(short) == "" || normalise(name) == "" {
		return nil, errors.New("a console's name needs some letters or numbers")
	}
	out := &Console{ID: id, Name: name, Short: short, PadLike: in.PadLike}

	if len(in.Extensions) == 0 {
		return nil, errors.New("list at least one file extension, such as .abc")
	}
	if len(in.Extensions) > 64 {
		return nil, errors.New("that's too many file extensions")
	}
	seen := map[string]bool{}
	for _, raw := range in.Extensions {
		ext := strings.ToLower(strings.TrimSpace(raw))
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		if !customExtRe.MatchString(ext) {
			return nil, fmt.Errorf("%q isn't a file extension: use letters and numbers, like .abc", raw)
		}
		if archiveExtension(ext) {
			return nil, fmt.Errorf("%s is an archive, which game libraries open anyway; list what's inside", ext)
		}
		if seen[ext] {
			continue
		}
		seen[ext] = true
		// An extension another console already uses can't sort a file by itself,
		// so this console never takes files from one in the table.
		claimed := false
		for i := range others {
			if others[i].ID != id && others[i].Accepts(ext) {
				claimed = true
				break
			}
		}
		if claimed {
			out.Shared = append(out.Shared, ext)
		} else {
			out.Extensions = append(out.Extensions, ext)
		}
	}

	if len(in.Aliases) > 32 {
		return nil, errors.New("that's too many folder names")
	}
	aliases := map[string]bool{}
	add := func(a string) { aliases[a] = true }
	add(normalise(name))
	add(normalise(short))
	for _, raw := range in.Aliases {
		if strings.IndexFunc(raw, badLabelRune) >= 0 || len(raw) > 80 {
			return nil, errors.New("a folder name has characters that can't be used")
		}
		a := normalise(raw)
		if a == "" {
			return nil, fmt.Errorf("the folder name %q needs some letters or numbers", raw)
		}
		for i := range others {
			if others[i].ID != id && contains(others[i].Aliases, a) {
				return nil, fmt.Errorf("the folder name %q already means %s", raw, others[i].Name)
			}
		}
		add(a)
	}
	for i := range others {
		if others[i].ID == id {
			continue
		}
		for _, a := range others[i].Aliases {
			delete(aliases, a) // its own name yields to a console already using it
		}
	}
	for a := range aliases {
		out.Aliases = append(out.Aliases, a)
	}
	sortStrings(out.Aliases)

	if len(in.BIOS) > 32 {
		return nil, errors.New("that's too many BIOS files")
	}
	for _, b := range in.BIOS {
		file := strings.TrimSpace(b.File)
		if !safeBIOSPath(file) {
			return nil, fmt.Errorf("%q can't be a BIOS file name: it has to be a plain name inside the BIOS folder", b.File)
		}
		what, err := cleanLabel("BIOS description", b.For, 200)
		if err != nil && strings.TrimSpace(b.For) != "" {
			return nil, err
		}
		out.BIOS = append(out.BIOS, BIOS{File: file, For: what, Required: b.Required})
	}

	if in.PadLike != "" {
		if _, ok := pads[in.PadLike]; !ok || in.PadLike == TestConsole {
			return nil, fmt.Errorf("%q isn't a console whose controller can be copied", in.PadLike)
		}
	}
	if len(in.Cores) > 32 {
		return nil, errors.New("that's too many cores")
	}
	for _, id := range in.Cores {
		if !customCoreRe.MatchString(id) {
			return nil, fmt.Errorf("%q isn't the name of a core", id)
		}
	}
	return out, nil
}

// CustomConsoleInput turns a console back into what describes it, to edit or export.
func CustomConsoleInput(c *Console, cores []string) CustomConsole {
	return CustomConsole{ID: c.ID, Name: c.Name, Short: c.Short, Extensions: append(append([]string{}, c.Extensions...), c.Shared...),
		Aliases: c.Aliases, BIOS: c.BIOS, PadLike: c.PadLike, Cores: cores}
}

func newCustomID(short string, others []Console) string {
	base := CustomPrefix + strings.Trim(slug(short), "-")
	if len(base) > 48 {
		base = strings.TrimRight(base[:48], "-")
	}
	if base == CustomPrefix {
		base += "console"
	}
	id := base
	for n := 2; ; n++ {
		taken := false
		for i := range others {
			taken = taken || others[i].ID == id
		}
		if !taken {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	return b.String()
}

func cleanLabel(what, v string, limit int) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("the %s is empty", what)
	}
	if len([]rune(v)) > limit {
		return "", fmt.Errorf("the %s is longer than %d characters", what, limit)
	}
	if strings.IndexFunc(v, badLabelRune) >= 0 {
		return "", fmt.Errorf("the %s has characters that can't be used", what)
	}
	return v, nil
}

// badLabelRune refuses control and invisible formatting characters, which can make
// text read as something else, and angle brackets, in case a client ever forgets
// that this is text.
func badLabelRune(r rune) bool {
	return r == '<' || r == '>' || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == unicode.ReplacementChar
}

// safeBIOSPath allows folders, as dc/dc_boot.bin needs, and nothing that climbs out
// of the BIOS folder or means something special to Windows.
func safeBIOSPath(p string) bool {
	if p == "" || len(p) > 200 || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return false
	}
	if path.Clean(p) != p {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "." || seg == ".." || strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " ") || !biosSegmentRe.MatchString(seg) {
			return false
		}
	}
	return true
}

func archiveExtension(ext string) bool {
	switch ext {
	case ".zip", ".7z", ".rar", ".gz", ".tgz", ".tar":
		return true
	}
	return false
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// The published list: the table followed by the custom consoles, replaced whole so
// a scan on another goroutine sees one or the other.
var published atomic.Pointer[[]Console]

// SetCustomConsoles replaces the custom consoles.
func SetCustomConsoles(list []Console) {
	all := make([]Console, 0, len(Consoles)+len(list))
	all = append(append(all, Consoles...), list...)
	published.Store(&all)
}

// All is every console, the table's first. Don't change what it returns.
func All() []Console {
	if p := published.Load(); p != nil {
		return *p
	}
	return Consoles
}

// CustomConsoles is the custom consoles now in use.
func CustomConsoles() []Console { return All()[len(Consoles):] }

// IsCustomConsole says whether an ID belongs to a custom console.
func IsCustomConsole(id string) bool { return strings.HasPrefix(id, CustomPrefix) }

// NonCommercialName says whether a core's own name for itself is one of the cores
// whose licence forbids commercial use. A file can be called anything, so what it
// reports gets looked at as well as what it's called.
func NonCommercialName(name string) bool {
	n := normalise(name)
	if n == "" {
		return false
	}
	for id := range nonCommercial {
		if normalise(id) == n {
			return true
		}
	}
	switch n {
	case "finalburnneo", "fbalpha", "finalburnalpha":
		return true
	}
	return strings.HasPrefix(n, "snes9x") || strings.HasPrefix(n, "mame2000") || strings.HasPrefix(n, "mame2003") || strings.HasPrefix(n, "mame2010")
}

// NonCommercialLicense says whether licence text forbids commercial use.
func NonCommercialLicense(text string) bool {
	n := normalise(text)
	return strings.Contains(n, "noncommercial") || strings.Contains(n, "notforcommercial") || strings.Contains(n, "ccbync")
}
