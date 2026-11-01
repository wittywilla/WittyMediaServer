package modules

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittyphotos"
)

const photosID = "wittyphotos"

// PhotoEngine reads what photos say about themselves through the WittyPhotos
// version in use, and through the built-in copy when that is the one in use. A
// file a helper dies on isn't read here as well: whatever killed the helper would
// kill the server, which the helper is there to prevent. The scan carries on
// without its details. It is library.PhotoEngine.
type PhotoEngine struct{ M *Manager }

// ErrHelperFailed: a helper stopped working on this file.
var ErrHelperFailed = errors.New("the helper stopped on this file")

type pathParams struct {
	Path string `json:"path"`
}

// useBuiltin: the helper's limits may be why it can't read a file, so the server
// tries the file itself.
func useBuiltin(err error) bool {
	var ce *wittymodule.CallError
	return errors.As(err, &ce) && ce.Code == "denied"
}

// ReadFile reads a photo's EXIF.
func (p PhotoEngine) ReadFile(path string) (*wittyphotos.Info, error) {
	var in wittyphotos.Info
	helper := p.M.helperRunning(photosID)
	_, err, handled := p.M.call(photosID, "read", pathParams{path}, &in)
	if helper && !handled {
		return nil, ErrHelperFailed
	}
	if !handled || useBuiltin(err) {
		return wittyphotos.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	return &in, nil
}

// Dimensions reads a picture's size from its header.
func (p PhotoEngine) Dimensions(path string) (w, h int) {
	var d struct{ W, H int }
	helper := p.M.helperRunning(photosID)
	_, err, handled := p.M.call(photosID, "dimensions", pathParams{path}, &d)
	switch {
	case handled && err == nil:
		return d.W, d.H
	case helper && !handled:
		return 0, 0
	}
	return wittyphotos.Dimensions(path)
}

// RawPreview returns the JPEG a camera stored in a RAW file.
func (p PhotoEngine) RawPreview(path string) ([]byte, error) {
	helper := p.M.helperRunning(photosID)
	b, err, handled := p.M.call(photosID, "preview", pathParams{path}, nil)
	if helper && !handled {
		return nil, ErrHelperFailed
	}
	if !handled || useBuiltin(err) {
		return wittyphotos.RawPreview(path)
	}
	var ce *wittymodule.CallError
	if errors.As(err, &ce) && ce.Code == "no-preview" {
		return nil, wittyphotos.ErrNoPreview
	}
	return b, err
}

// IsRaw knows the built-in copy's formats and any a newer version adds, from its
// signed manifest, so scanning a folder never has to start a helper.
func (p PhotoEngine) IsRaw(name string) bool {
	if wittyphotos.IsRaw(name) {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return false
	}
	for _, e := range p.M.capability(photosID, "rawExtensions") {
		if e == ext {
			return true
		}
	}
	return false
}

// capability is a fact from the manifest of the version in use.
func (m *Manager) capability(id, name string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.mods[id]; e != nil {
		return e.man.Capabilities[name]
	}
	return nil
}
