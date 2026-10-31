// Command wittyphotos-module is WittyPhotos as a helper process, so a new version
// can reach a server without a new server. See WittyModule for the protocol.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"

	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittyphotos"
)

func main() { wittymodule.Main(wittyphotos.ManifestJSON, handle) }

// Dims is the answer to "dimensions".
type dims struct {
	W int `json:"w"`
	H int `json:"h"`
}

func handle(method string, params json.RawMessage) (any, []byte, error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Path == "" {
		return nil, nil, &wittymodule.CallError{Code: "bad-call", Message: "no path was given"}
	}
	switch method {
	case "read":
		in, err := wittyphotos.ReadFile(p.Path)
		if err != nil {
			return nil, nil, coded(err)
		}
		return in, nil, nil
	case "dimensions":
		w, h := wittyphotos.Dimensions(p.Path)
		return dims{w, h}, nil, nil
	case "preview":
		b, err := wittyphotos.RawPreview(p.Path)
		if err != nil {
			return nil, nil, coded(err)
		}
		return nil, b, nil
	}
	return nil, nil, &wittymodule.CallError{Code: "unknown-method", Message: fmt.Sprintf("this version has no %q", method)}
}

// coded names the errors the server acts on.
func coded(err error) error {
	code := ""
	switch {
	case errors.Is(err, wittyphotos.ErrNoPreview):
		code = "no-preview"
	case errors.Is(err, wittyphotos.ErrNoExif):
		code = "no-exif"
	case errors.Is(err, fs.ErrPermission):
		code = "denied" // the helper's limits may be why, so the server tries itself
	}
	return &wittymodule.CallError{Code: code, Message: err.Error()}
}
