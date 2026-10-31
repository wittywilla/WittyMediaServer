package modules

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wittywilla/wittymodule"
)

// A web bundle is held to the same signature as a helper, because it runs with the
// access of whoever is signed in. The server names exactly the files that were signed
// and serves nothing else; the page's own content security policy, which allows only
// this site's scripts, still applies to all of them.

type webFile struct {
	hash    string
	size    int64
	modTime time.Time
}

// webFiles are a bundle's files as they were when last checked. A later change to one
// is seen from its size and date without hashing it again.
type webFiles struct {
	dir   string
	files map[string]webFile
}

func loadWebFiles(dir string, man *wittymodule.Manifest, keys []ed25519.PublicKey) (*webFiles, error) {
	want, err := signedFiles(dir, man, keys)
	if err != nil {
		return nil, err
	}
	w := &webFiles{dir: dir, files: map[string]webFile{}}
	for rel, h := range want {
		st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		w.files[rel] = webFile{hash: h, size: st.Size(), modTime: st.ModTime()}
	}
	return w, nil
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
			Base: "/api/modules/" + id + "/web/" + e.man.Version + "/"})
	}
	return out
}

// ErrNoFile: the bundle has no such file, or it isn't the version in use.
var ErrNoFile = errors.New("no such file")

// WebFile finds a file of a bundle in use, or refuses. rel comes from the address, so
// it is matched against the signed names and never used as a path until it has been.
// Only the version in use is served; a tab holding an older one falls back to the
// app's own copy.
func (m *Manager) WebFile(id, ver, rel string) (path, hash string, err error) {
	m.mu.Lock()
	e := m.mods[id]
	if e == nil || e.web == nil || e.man.Version != ver {
		m.mu.Unlock()
		return "", "", ErrNoFile
	}
	w := e.web
	m.mu.Unlock()
	f, ok := w.files[rel]
	if !ok || rel == wittymodule.ManifestFile || strings.HasPrefix(rel, ".") {
		return "", "", ErrNoFile
	}
	p := filepath.Join(w.dir, filepath.FromSlash(rel))
	st, serr := os.Stat(p)
	if serr != nil || st.Size() != f.size || !st.ModTime().Equal(f.modTime) || !st.Mode().IsRegular() {
		bad := fmt.Errorf("%w: %s is different", errUntrusted, rel)
		m.failed(id, e, ver, bad)
		return "", "", ErrNoFile
	}
	return p, f.hash, nil
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
