// Package cores finds libretro cores on this machine and installs them on an
// administrator's request. A core is someone else's native code, so downloads
// are held to one host, one list of names and one file.
package cores

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittyplay"
)

// Core is a core file on this machine.
type Core struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	License       string `json:"license"` // "" when we don't know this core
	Path          string `json:"-"`
	Source        string `json:"source"` // installed | retroarch | system
	SHA256        string `json:"sha256,omitempty"`
	Installed     int64  `json:"installedAt,omitempty"`
	NonCommercial bool   `json:"nonCommercial,omitempty"`
}

// Manager looks in one folder it owns, plus folders other programs own.
type Manager struct {
	Dir    string // installs go here; an administrator may also place files here
	Client *http.Client
	Extra  []string // folders to read cores from; DefaultExtra if nil
}

const (
	downloadHost = "buildbot.libretro.com"
	maxDownload  = 256 << 20
)

var validID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// libraryExt is this platform's shared library extension.
func libraryExt() string {
	switch runtime.GOOS {
	case "windows":
		return ".dll"
	case "darwin":
		return ".dylib"
	}
	return ".so"
}

func fileName(id string) string { return id + "_libretro" + libraryExt() }

// DefaultExtra are the folders RetroArch and system packages keep cores in.
// Cores there were put there by someone else and are only ever read.
func DefaultExtra() []string {
	home, _ := os.UserHomeDir()
	var out []string
	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("APPDATA"); d != "" {
			out = append(out, filepath.Join(d, "RetroArch", "cores"))
		}
		if d := os.Getenv("ProgramFiles"); d != "" {
			out = append(out, filepath.Join(d, "RetroArch", "cores"))
		}
		out = append(out, `C:\RetroArch-Win64\cores`, `C:\RetroArch\cores`)
	case "darwin":
		if home != "" {
			out = append(out, filepath.Join(home, "Library", "Application Support", "RetroArch", "cores"))
		}
	default:
		if home != "" {
			out = append(out, filepath.Join(home, ".config", "retroarch", "cores"))
		}
		out = append(out, "/usr/lib/libretro", "/usr/lib64/libretro", "/usr/local/lib/libretro", "/usr/lib/x86_64-linux-gnu/libretro", "/usr/lib/aarch64-linux-gnu/libretro")
	}
	return out
}

// List returns the cores here. A core installed through us wins over the same
// core elsewhere.
func (m *Manager) List() []Core {
	seen := map[string]bool{}
	out := []Core{}
	read := func(dir, source string) {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			name := e.Name()
			id, ok := strings.CutSuffix(name, "_libretro"+libraryExt())
			if !ok || e.IsDir() || !validID.MatchString(id) || seen[id] {
				continue
			}
			seen[id] = true
			c := Core{ID: id, Name: id, Path: filepath.Join(dir, name), Source: source, NonCommercial: wittyplay.IsNonCommercial(id)}
			if known, ok := wittyplay.CoreLicense(id); ok {
				c.Name, c.License = known.Name, known.License
			}
			if source == "installed" {
				c.SHA256, c.Installed = m.record(id)
			}
			out = append(out, c)
		}
	}
	read(m.Dir, "installed")
	extra := m.Extra
	if extra == nil {
		extra = DefaultExtra()
	}
	for _, dir := range extra {
		src := "system"
		if strings.Contains(strings.ToLower(dir), "retroarch") {
			src = "retroarch"
		}
		read(dir, src)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Find returns a core by name, or nil.
func (m *Manager) Find(id string) *Core {
	for _, c := range m.List() {
		if c.ID == id {
			return &c
		}
	}
	return nil
}

// ForConsole picks the core to play a console with: the one asked for if it's
// here, else the first in the console's list that is.
func (m *Manager) ForConsole(c *wittyplay.Console, preferred string) *Core {
	all := m.List()
	find := func(id string) *Core {
		for i := range all {
			if all[i].ID == id {
				return &all[i]
			}
		}
		return nil
	}
	if preferred != "" {
		if got := find(preferred); got != nil {
			return got
		}
	}
	for _, want := range c.Cores {
		if got := find(want.ID); got != nil {
			return got
		}
	}
	return nil
}

// CanDownload says whether this platform has prebuilt cores to download.
func CanDownload() bool { return platformPath() != "" }

func platformPath() string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		return "windows/x86_64"
	case "linux/amd64":
		return "linux/x86_64"
	case "linux/arm64":
		return "linux/aarch64"
	case "linux/arm":
		return "linux/armv7-neon-hf"
	case "darwin/amd64":
		return "apple/osx/x86_64"
	case "darwin/arm64":
		return "apple/osx/arm64"
	}
	return ""
}

// Offered says whether a core is on our download list. The list only holds
// cores whose licences allow it (wittyplay.Consoles).
func Offered(id string) bool {
	for i := range wittyplay.Consoles {
		for _, c := range wittyplay.Consoles[i].Cores {
			if c.ID == id {
				return true
			}
		}
	}
	return false
}

// DownloadURL is where a core comes from. It is built only from our own table
// and platform, never from a request.
func DownloadURL(id string) (string, error) {
	if !Offered(id) {
		return "", fmt.Errorf("%q is not a core this server offers", id)
	}
	p := platformPath()
	if p == "" {
		return "", errors.New("there are no prebuilt cores for this platform; install them with your system's packages or place them in the cores folder")
	}
	return "https://" + downloadHost + "/nightly/" + p + "/latest/" + fileName(id) + ".zip", nil
}

type record struct {
	SHA256      string `json:"sha256"`
	Source      string `json:"source"`
	InstalledAt int64  `json:"installedAt"`
}

func (m *Manager) record(id string) (string, int64) {
	b, err := os.ReadFile(filepath.Join(m.Dir, id+".core.json"))
	if err != nil {
		return "", 0
	}
	var r record
	if json.Unmarshal(b, &r) != nil {
		return "", 0
	}
	return r.SHA256, r.InstalledAt
}

// Install downloads one core: from one host, over HTTPS, with its size capped,
// taking exactly one file from the archive and writing it in one step.
func (m *Manager) Install(ctx context.Context, id string) (*Core, error) {
	url, err := DownloadURL(id)
	if err != nil {
		return nil, err
	}
	if !validID.MatchString(id) {
		return nil, errors.New("bad core name")
	}
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		return nil, err
	}
	body, err := m.fetch(ctx, url)
	if err != nil {
		return nil, err
	}
	lib, err := extractOne(body, fileName(id))
	if err != nil {
		return nil, fmt.Errorf("unpacking %s: %w", id, err)
	}
	sum := sha256.Sum256(lib)
	if err := writeFile(filepath.Join(m.Dir, fileName(id)), lib, 0o755); err != nil {
		return nil, err
	}
	rec, _ := json.Marshal(record{SHA256: hex.EncodeToString(sum[:]), Source: url, InstalledAt: time.Now().Unix()})
	if err := writeFile(filepath.Join(m.Dir, id+".core.json"), rec, 0o644); err != nil {
		return nil, err
	}
	return m.Find(id), nil
}

func (m *Manager) fetch(ctx context.Context, url string) ([]byte, error) {
	client := *http.DefaultClient
	if m.Client != nil {
		client = *m.Client
	}
	client.Timeout = 5 * time.Minute
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if r.URL.Scheme != "https" || r.URL.Hostname() != downloadHost || len(via) > 3 {
			return errors.New("the download tried to go somewhere else")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the download server answered %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxDownload {
		return nil, errors.New("the download is larger than any core should be")
	}
	return b, nil
}

// extractOne takes the named file from the top of a zip and ignores the rest.
func extractOne(data []byte, name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		if f.UncompressedSize64 > maxDownload {
			return nil, errors.New("the core is larger than any core should be")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		out, err := io.ReadAll(io.LimitReader(rc, maxDownload+1))
		if err != nil {
			return nil, err
		}
		if len(out) > maxDownload {
			return nil, errors.New("the core is larger than any core should be")
		}
		return out, nil
	}
	return nil, errors.New("the archive does not hold the expected file")
}

func writeFile(path string, b []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".core-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Remove deletes a core we installed. Cores elsewhere belong to whoever put them
// there.
func (m *Manager) Remove(id string) error {
	if !validID.MatchString(id) {
		return errors.New("bad core name")
	}
	if err := os.Remove(filepath.Join(m.Dir, fileName(id))); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(m.Dir, id+".core.json"))
	return nil
}
