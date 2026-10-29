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
	Preview     struct {
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
	if len(data) > maxZipBytes {
		return nil, fmt.Errorf("theme ZIP is larger than %d MB", maxZipBytes>>20)
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
	id := strings.ToLower(meta.ID)
	if id == "" {
		id = strings.ToLower(strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName)))
	}
	id = regexp.MustCompile(`[^a-z0-9._-]+`).ReplaceAllString(id, "-")
	if !idRe.MatchString(id) {
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
	return &meta, nil
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
