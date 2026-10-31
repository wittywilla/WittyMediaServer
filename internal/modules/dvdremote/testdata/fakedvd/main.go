// A WittyDVD helper for the tests. Unlike a real one it reads its manifest from beside
// itself, so it can play any version, and it dies at the first button press.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/wittywilla/wittydvd/wire"
	"github.com/wittywilla/wittymodule"
)

func main() {
	exe, _ := os.Executable()
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(exe), wittymodule.ManifestFile))
	if err != nil {
		os.Exit(9)
	}
	h := new(wire.Helper)
	wittymodule.Main(raw, func(method string, params json.RawMessage) (any, []byte, error) {
		if method == wire.MethodPress {
			os.Exit(4)
		}
		return h.Handle(method, params)
	})
}
