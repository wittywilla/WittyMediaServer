// Package themes manages interface themes. A theme is a ZIP file in the
// themes directory containing at least:
//
//	theme.json   {"name": "...", "scheme": "dark"|"light", ...}
//	theme.css    CSS loaded after the built-in stylesheet
//
// plus any images or fonts theme.css references with relative URLs. The ZIP
// file name (without .zip) is the theme id. See docs/THEMES.md.
package themes

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/wittywilla/wittymediaserver/internal/zipsafe"
)

// Meta is theme.json.
type Meta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Author      string `json:"author,omitempty"`
	Version     string `json:"version,omitempty"`
	Scheme      string `json:"scheme"` // dark | light
	Builtin     bool   `json:"builtin,omitempty"`
	// Colors and Radius are every colour and the corner radius as data, so the
	// app can show a theme without loading it. Written by scripts/design-tokens.py.
	Colors  map[string]string `json:"colors,omitempty"`
	Radius  float64           `json:"radius,omitempty"`
	Preview struct {
		BG      string `json:"bg,omitempty"`
		Surface string `json:"surface,omitempty"`
		Text    string `json:"text,omitempty"`
		Accent  string `json:"accent,omitempty"`
	} `json:"preview"`
	Size    int64 `json:"size"`
	ModTime int64 `json:"modTime"`
}

const (
	maxZipBytes  = 25 << 20
	maxFileBytes = 10 << 20
	maxFiles     = 200
)

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// Allowed file types inside a theme and how they are served.
var contentTypes = map[string]string{
	".css": "text/css; charset=utf-8", ".json": "application/json",
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp", ".gif": "image/gif",
	".avif": "image/avif", ".svg": "image/svg+xml",
	".woff2": "font/woff2", ".woff": "font/woff", ".ttf": "font/ttf", ".otf": "font/otf",
	".txt": "text/plain; charset=utf-8", ".md": "text/plain; charset=utf-8",
}

// retired: themes that used to ship, and which of today's looks replaces each, so
// a server or device still set to one keeps working.
var retired = map[string]string{
	"dark": "default", "dark-purple": "default", "dark-pink": "default",
	"dark-blue": "midnight", "dark-red": "cinema", "dark-orange": "cinema", "dark-gold": "cinema",
	"dark-green": "evergreen", "dark-teal": "evergreen",
	"light-blue": "light", "light-purple": "light", "light-pink": "light", "light-teal": "light",
	"light-red": "paper", "light-orange": "paper", "light-gold": "paper", "light-green": "paper",
}

// Manager reads themes from a directory.
type Manager struct {
	Dir string
	mu  sync.Mutex
}

// ErrNotFound is returned for unknown themes or files.
var ErrNotFound = errors.New("theme not found")

// Install writes the built-in themes as ZIPs. An existing ZIP is replaced
// only when it is an older built-in version (a user's edited copy saved
// under the same name without "builtin": true is kept).
func (m *Manager) Install(src fs.FS) error {
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		return err
	}
	dirs, err := fs.ReadDir(src, ".")
	if err != nil {
		return err
	}
	// Built-in copies of retired themes go. A theme someone made under one of those
	// names isn't marked built-in, and stays.
	for id := range retired {
		p := filepath.Join(m.Dir, id+".zip")
		if cur, err := m.readMeta(p); err == nil && cur.Builtin {
			_ = os.Remove(p)
		}
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		id := d.Name()
		raw, err := fs.ReadFile(src, id+"/theme.json")
		if err != nil {
			continue
		}
		var meta Meta
		if json.Unmarshal(raw, &meta) != nil {
			continue
		}
		dst := filepath.Join(m.Dir, id+".zip")
		if cur, err := m.readMeta(dst); err == nil && (!cur.Builtin || cur.Version == meta.Version) {
			continue
		}
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		files, _ := fs.ReadDir(src, id)
		for _, f := range files {
			b, err := fs.ReadFile(src, id+"/"+f.Name())
			if err != nil {
				continue
			}
			w, err := zw.Create(f.Name())
			if err != nil {
				return err
			}
			if _, err := w.Write(b); err != nil {
				return err
			}
		}
		if err := zw.Close(); err != nil {
			return err
		}
		if err := writeAtomic(dst, buf.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func writeAtomic(dst string, b []byte) error {
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func (m *Manager) readMeta(zipPath string) (*Meta, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	raw, err := readZipFile(&zr.Reader, "theme.json")
	if err != nil {
		return nil, err
	}
	var meta Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("theme.json: %w", err)
	}
	st, err := os.Stat(zipPath)
	if err == nil {
		meta.Size, meta.ModTime = st.Size(), st.ModTime().Unix()
	}
	return &meta, nil
}

func readZipFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			if f.UncompressedSize64 > maxFileBytes {
				return nil, fmt.Errorf("%s is too large", name)
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, maxFileBytes))
		}
	}
	return nil, ErrNotFound
}

// List returns every valid theme, built-ins first, then by name.
func (m *Manager) List() []Meta {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries, _ := os.ReadDir(m.Dir)
	var out []Meta
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.EqualFold(filepath.Ext(name), ".zip") {
			continue
		}
		id := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
		if !idRe.MatchString(id) {
			continue
		}
		meta, err := m.readMeta(filepath.Join(m.Dir, name))
		if err != nil {
			continue
		}
		meta.ID = id
		if meta.Name == "" {
			meta.Name = id
		}
		if meta.Scheme != "light" {
			meta.Scheme = "dark"
		}
		out = append(out, *meta)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ID == "default" || out[j].ID == "default" {
			return out[i].ID == "default"
		}
		if out[i].Builtin != out[j].Builtin {
			return out[i].Builtin
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (m *Manager) zipPath(id string) (string, error) {
	id = strings.ToLower(id)
	if !idRe.MatchString(id) {
		return "", ErrNotFound
	}
	p := filepath.Join(m.Dir, id+".zip")
	if _, err := os.Stat(p); err != nil {
		if to, ok := retired[id]; ok {
			return m.zipPath(to)
		}
		return "", ErrNotFound
	}
	return p, nil
}

// File returns one file of a theme with its content type.
func (m *Manager) File(id, name string) ([]byte, string, error) {
	name = path.Clean("/" + name)[1:]
	ct, ok := contentTypes[strings.ToLower(path.Ext(name))]
	if !ok || name == "" {
		return nil, "", ErrNotFound
	}
	p, err := m.zipPath(id)
	if err != nil {
		return nil, "", err
	}
	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, "", err
	}
	defer zr.Close()
	b, err := readZipFile(&zr.Reader, name)
	return b, ct, err
}

// ZipPath returns the theme's file for downloading.
func (m *Manager) ZipPath(id string) (string, error) { return m.zipPath(id) }

// Add validates an uploaded theme ZIP and stores it as <id>.zip. The id comes
// from theme.json "id" or else the uploaded file name.
func (m *Manager) Add(data []byte, fileName string) (*Meta, error) {
	meta, err := parseTheme(data)
	if err != nil {
		return nil, err
	}
	id := strings.ToLower(meta.ID)
	if id == "" {
		id = strings.ToLower(strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName)))
	}
	id = regexp.MustCompile(`[^a-z0-9._-]+`).ReplaceAllString(id, "-")
	if !idRe.MatchString(id) || !filepath.IsLocal(id+".zip") {
		return nil, errors.New(`choose a theme id (theme.json "id") of letters, digits, dots, dashes or underscores`)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, err := m.readMeta(filepath.Join(m.Dir, id+".zip")); err == nil && cur.Builtin {
		return nil, fmt.Errorf("%q is a built-in theme; give your theme a different id", id)
	}
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		return nil, err
	}
	if err := writeAtomic(filepath.Join(m.Dir, id+".zip"), data); err != nil {
		return nil, err
	}
	meta.ID = id
	return meta, nil
}

// parseTheme checks a ZIP by the rules for a theme, and reads its theme.json.
func parseTheme(data []byte) (*Meta, error) {
	if len(data) > maxZipBytes {
		return nil, fmt.Errorf("theme ZIP is larger than %d MB", maxZipBytes>>20)
	}
	if err := zipsafe.Check(bytes.NewReader(data), int64(len(data)), zipsafe.Limits{MaxEntries: 1000, MaxDirectory: 1 << 20}); err != nil {
		return nil, errors.New("not a ZIP file: themes must be packed as .zip")
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("not a ZIP file: themes must be packed as .zip")
	}
	if len(zr.File) > maxFiles {
		return nil, fmt.Errorf("theme has more than %d files", maxFiles)
	}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		clean := path.Clean("/" + f.Name)[1:]
		if clean != f.Name || strings.Contains(f.Name, `\`) {
			return nil, fmt.Errorf("unsafe file name in ZIP: %q", f.Name)
		}
		if _, ok := contentTypes[strings.ToLower(path.Ext(f.Name))]; !ok {
			return nil, fmt.Errorf("file type not allowed in a theme: %s (allowed: css, json, images, fonts, txt, md)", f.Name)
		}
		if f.UncompressedSize64 > maxFileBytes {
			return nil, fmt.Errorf("%s is larger than %d MB", f.Name, maxFileBytes>>20)
		}
	}
	raw, err := readZipFile(zr, "theme.json")
	if err != nil {
		return nil, errors.New("theme.json is missing from the top level of the ZIP")
	}
	if _, err := readZipFile(zr, "theme.css"); err != nil {
		return nil, errors.New("theme.css is missing from the top level of the ZIP")
	}
	var meta Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("theme.json: %w", err)
	}
	if strings.TrimSpace(meta.Name) == "" {
		return nil, errors.New(`theme.json needs a "name"`)
	}
	if meta.Builtin {
		return nil, errors.New(`remove "builtin": true from theme.json (it is reserved for the themes that ship with WittyMediaServer)`)
	}
	return &meta, nil
}

// CheckZip says whether a ZIP would be accepted as a theme someone uploads, by the same
// rules, without keeping it.
func CheckZip(data []byte) error {
	_, err := parseTheme(data)
	return err
}

// Remove deletes a user theme (built-ins are recreated at start-up, so they
// cannot be removed).
func (m *Manager) Remove(id string) error {
	p, err := m.zipPath(id)
	if err != nil {
		return err
	}
	if meta, err := m.readMeta(p); err == nil && meta.Builtin {
		return errors.New("built-in themes cannot be removed")
	}
	return os.Remove(p)
}

// Custom is a theme made in the app's theme maker: colours and corners, no files.
type Custom struct {
	Name   string            `json:"name"`
	Scheme string            `json:"scheme"`
	Colors map[string]string `json:"colors"` // CSS variable name without the dashes -> #rrggbb
	Radius int               `json:"radius"`
}

// customVars is everything the maker sets, in the order it's written out.
var customVars = []string{"bg", "bg-2", "bg-3", "bg-4", "fg", "fg-2", "fg-3", "accent", "accent-2", "accent-fg", "accent-text", "control-line", "danger", "ok", "warn"}

var plainHex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// AddCustom turns a made theme into an ordinary theme ZIP. Only colours that are
// exactly #rrggbb get through, so nothing else can reach the stylesheet.
func (m *Manager) AddCustom(c Custom) (*Meta, error) {
	name := strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, c.Name)), " ")
	if name == "" || len([]rune(name)) > 40 {
		return nil, errors.New("a name of up to 40 characters is required")
	}
	if c.Scheme != "dark" && c.Scheme != "light" {
		return nil, errors.New("a theme is dark or light")
	}
	if c.Radius < 0 || c.Radius > 24 {
		return nil, errors.New("corners are 0 to 24 pixels")
	}
	var css strings.Builder
	fmt.Fprintf(&css, ":root {\n  color-scheme: %s;\n", c.Scheme)
	for _, k := range customVars {
		v := c.Colors[k]
		if !plainHex.MatchString(v) {
			return nil, fmt.Errorf("%s needs to be a colour like #336699", k)
		}
		fmt.Fprintf(&css, "  --%s: %s;\n", k, v)
	}
	rgb := func(h string) string {
		var r, g, b int
		_, _ = fmt.Sscanf(h, "#%02x%02x%02x", &r, &g, &b)
		return fmt.Sprintf("%d %d %d", r, g, b)
	}
	fmt.Fprintf(&css, "  --bg-rgb: %s;\n  --accent-rgb: %s;\n  --radius: %dpx;\n}\n", rgb(c.Colors["bg"]), rgb(c.Colors["accent"]), c.Radius)

	id := "custom-" + strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(name), "-"), "-")
	if id == "custom-" {
		id = "custom-theme"
	}
	if len(id) > 64 {
		id = strings.TrimRight(id[:64], "-")
	}
	meta := Meta{ID: id, Name: name, Author: "Made in the app", Scheme: c.Scheme, Radius: float64(c.Radius), Colors: map[string]string{}}
	meta.Preview.BG, meta.Preview.Surface, meta.Preview.Text, meta.Preview.Accent = c.Colors["bg"], c.Colors["bg-3"], c.Colors["fg"], c.Colors["accent"]
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string][]byte{"theme.json": raw, "theme.css": []byte(css.String())} {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(body); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return m.Add(buf.Bytes(), id+".zip")
}
