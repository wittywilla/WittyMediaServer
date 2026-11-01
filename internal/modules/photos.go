package modules

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymodule"
)

const photosID = "wittyphotos"

// PhotoEngine reads what photos say about themselves through the WittyPhotos version in
// use. With none, a scan carries on and the photos have only what their files say on the
// outside. It is library.PhotoEngine.
type PhotoEngine struct{ M *Manager }

type pathParams struct {
	Path string `json:"path"`
}

// ReadFile reads a photo's EXIF.
func (p PhotoEngine) ReadFile(path string) (*library.PhotoInfo, error) {
	var in library.PhotoInfo
	_, err, handled := p.M.call(photosID, "read", pathParams{path}, &in)
	if !handled {
		return nil, library.ErrNoPhotoEngine
	}
	if err != nil {
		return nil, err
	}
	return &in, nil
}

// Dimensions reads a picture's size from its header.
func (p PhotoEngine) Dimensions(path string) (w, h int) {
	var d struct{ W, H int }
	if _, err, handled := p.M.call(photosID, "dimensions", pathParams{path}, &d); handled && err == nil {
		return d.W, d.H
	}
	return 0, 0
}

// RawPreview returns the JPEG a camera stored in a RAW file.
func (p PhotoEngine) RawPreview(path string) ([]byte, error) {
	b, err, handled := p.M.call(photosID, "preview", pathParams{path}, nil)
	if !handled {
		return nil, library.ErrNoPhotoEngine
	}
	var ce *wittymodule.CallError
	if errors.As(err, &ce) && ce.Code == "no-preview" {
		return nil, library.ErrNoRawPreview
	}
	return b, err
}

// IsRaw knows the formats the version in use lists in its signed manifest, so scanning a
// folder never has to start a helper.
func (p PhotoEngine) IsRaw(name string) bool {
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
