// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package photoengine

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymodule"
)

const photosID = "wittyphotos"

// PhotoEngine reads what photos say about themselves through the WittyPhotos version in
// use. With none, a scan carries on and the photos have only what their files say on the
// outside. It is library.PhotoEngine.
type PhotoEngine struct{ M *modules.Manager }

type pathParams struct {
	Path string `json:"path"`
}

// ReadFile reads a photo's EXIF.
func (p PhotoEngine) ReadFile(path string) (*library.PhotoInfo, error) {
	var in library.PhotoInfo
	if _, err := p.M.Call(photosID, "read", pathParams{path}, &in); err != nil {
		return nil, noEngine(err)
	}
	return &in, nil
}

// Dimensions reads a picture's size from its header.
func (p PhotoEngine) Dimensions(path string) (w, h int) {
	var d struct{ W, H int }
	if _, err := p.M.Call(photosID, "dimensions", pathParams{path}, &d); err != nil {
		return 0, 0
	}
	return d.W, d.H
}

// RawPreview returns the JPEG a camera stored in a RAW file.
func (p PhotoEngine) RawPreview(path string) ([]byte, error) {
	b, err := p.M.Call(photosID, "preview", pathParams{path}, nil)
	var ce *wittymodule.CallError
	if errors.As(err, &ce) && ce.Code == "no-preview" {
		return nil, library.ErrNoRawPreview
	}
	return b, noEngine(err)
}

// noEngine says the module isn't there in the way the library knows.
func noEngine(err error) error {
	var missing *modules.ErrMissing
	if errors.As(err, &missing) {
		return library.ErrNoPhotoEngine
	}
	return err
}

// IsRaw knows the formats the version in use lists in its signed manifest, so scanning a
// folder never has to start a helper.
func (p PhotoEngine) IsRaw(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return false
	}
	for _, e := range p.M.Capability(photosID, "rawExtensions") {
		if e == ext {
			return true
		}
	}
	return false
}
