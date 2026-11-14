// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package photoengine

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymodule/service"
)

const photosID = "wittyphotos"

// PhotoEngine reads what photos say about themselves through the WittyPhotos version in
// use, calls on its service (module API 5: read, dimensions, preview, each given the file's
// path). With none, a scan carries on and the photos have only what their files say on the
// outside. It is library.PhotoEngine.
type PhotoEngine struct{ M *modules.Manager }

// callTime bounds one call: reading a header, or a RAW file's preview.
const callTime = 30 * time.Second

func (p PhotoEngine) call(name, path string, out ...any) error {
	ctx, cancel := context.WithTimeout(context.Background(), callTime)
	defer cancel()
	return service.Call(ctx, photosID, name, nil, nil, []any{path}, out)
}

// ReadFile reads a photo's EXIF.
func (p PhotoEngine) ReadFile(path string) (*library.PhotoInfo, error) {
	var in library.PhotoInfo
	if err := p.call("read", path, &in); err != nil {
		return nil, noEngine(err)
	}
	return &in, nil
}

// Dimensions reads a picture's size from its header.
func (p PhotoEngine) Dimensions(path string) (w, h int) {
	if err := p.call("dimensions", path, &w, &h); err != nil {
		return 0, 0
	}
	return w, h
}

// RawPreview returns the JPEG a camera stored in a RAW file.
func (p PhotoEngine) RawPreview(path string) ([]byte, error) {
	var b []byte
	err := p.call("preview", path, &b)
	var ce *service.CallError
	if errors.As(err, &ce) && ce.Code == "no-preview" {
		return nil, library.ErrNoRawPreview
	}
	if err != nil {
		return nil, noEngine(err)
	}
	return b, nil
}

// noEngine says the module isn't there in the way the library knows.
func noEngine(err error) error {
	var missing *modules.ErrMissing
	if errors.As(err, &missing) || errors.Is(err, service.ErrNoService) {
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
