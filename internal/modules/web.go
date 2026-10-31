package modules

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

// A web bundle is held to the same signature as a helper, because it runs with the
// access of whoever is signed in. The server names exactly the files that were signed
// and serves nothing else; the page's own content security policy, which allows only
// this site's scripts, still applies to all of them.

// webFiles are a bundle's signed names and hashes. Each file is hashed again when it is
// served: browsers keep these for a year, so that is rare, and it means a file changed
// after the last start is never sent to someone's browser. tag names what was signed, and
// is in the address, so one version number from two publishers can't be mixed up in a
// browser's cache.
type webFiles struct {
	dir   string
	tag   string
	files map[string]string
}

func loadWebFiles(dir string, man *wittymodule.Manifest, keys []ed25519.PublicKey) (*webFiles, error) {
	want, err := signedFiles(dir, man, keys)
	if err != nil {
		return nil, err
	}
	sums, err := os.ReadFile(filepath.Join(dir, signedList))
	if err != nil {
		return nil, err
	}
	return &webFiles{dir: dir, tag: sum(sums)[:12], files: want}, nil
}

// WebBundle says where the browser finds a bundle in use.
type WebBundle struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Entry   string `json:"entry"`           // the script, under Base
	Style   string `json:"style,omitempty"` // a stylesheet, under Base
	Base    string `json:"base"`
	API     int    `json:"api"`
}

// WebBundles lists the bundles in use instead of the built-in copies. A module not
// listed uses the copy in the app itself, which is the ordinary case.
func (m *Manager) WebBundles() []WebBundle {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []WebBundle{}
	for _, id := range m.order {
		e := m.mods[id]
		if e.web == nil {
			continue
		}
		out = append(out, WebBundle{ID: id, Version: e.man.Version, Entry: e.man.Entry, Style: e.man.Style, API: e.man.API,
			Base: "/api/modules/" + id + "/web/" + e.web.address(e.man.Version) + "/"})
	}
	return out
}

func (w *webFiles) address(ver string) string { return ver + "~" + w.tag }

// ErrNoFile: the bundle has no such file, or it isn't the version in use.
var ErrNoFile = errors.New("no such file")

// WebFile finds a file of a bundle in use, or refuses. rel comes from the address, so
// it is matched against the signed names and never used as a path until it has been.
// Only the version in use is served; a tab holding an older one falls back to the
// app's own copy.
func (m *Manager) WebFile(id, ver, rel string) (path, hash string, err error) {
	m.mu.Lock()
	e := m.mods[id]
	if e == nil || e.web == nil || e.web.address(e.man.Version) != ver {
		m.mu.Unlock()
		return "", "", ErrNoFile
	}
	w, version := e.web, e.man.Version
	m.mu.Unlock()
	want, ok := w.files[rel]
	if !ok || rel == wittymodule.ManifestFile || strings.HasPrefix(rel, ".") {
		return "", "", ErrNoFile
	}
	p := filepath.Join(w.dir, filepath.FromSlash(rel))
	got, herr := update.FileSHA256(p)
	st, serr := os.Lstat(p)
	if herr != nil || serr != nil || got != want || !st.Mode().IsRegular() {
		m.failed(id, e, version, fmt.Errorf("%w: %s is different", errUntrusted, rel))
		return "", "", ErrNoFile
	}
	return p, want, nil
}

// ProgramCommand finds a program module's version in use, like Program, ready to start: a
// fork's goes through the launcher, since a program from another publisher can't be
// trusted to limit itself. It can write only to the folder after its -save argument, and
// open devices, since a game core may need a graphics card.
func (m *Manager) ProgramCommand(id string) (path string, args []string, version string, ok bool) {
	exe, version, ok := m.Program(id)
	if !ok {
		return "", nil, "", false
	}
	m.mu.Lock()
	fork := m.mods[id].state.Source != nil
	m.mu.Unlock()
	if !fork {
		return exe, nil, version, true
	}
	const strict = SandboxStrict | SandboxDevices | SandboxSaveArg
	path, args = launchWith(m.cfg.Launcher, exe, 0, strict, nil)
	if sandboxAvailable && goos() == "linux" && m.cfg.Launcher == "" {
		return "", nil, "", false
	}
	return path, args, version, true
}

// Program finds a program module's version in use: the path to run, checked against
// its signature just now, and that version. ok is false when the built-in copy is in
// use, and the caller uses that.
func (m *Manager) Program(id string) (path, version string, ok bool) {
	m.mu.Lock()
	e := m.mods[id]
	if e == nil || e.dir == "" || e.man.Delivery != wittymodule.DeliveryProgram {
		m.mu.Unlock()
		return "", "", false
	}
	man, dir, keys := e.man, e.dir, m.keysFor(e)
	m.mu.Unlock()
	if err := verifyInstalled(dir, man, keys); err != nil {
		m.failed(id, e, man.Version, err)
		return "", "", false
	}
	return filepath.Join(dir, man.EntryFile(goos())), man.Version, true
}
