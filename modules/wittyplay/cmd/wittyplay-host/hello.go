package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/wittywilla/wittyplay"
)

// A server checks a new version by starting it with this flag: it says who it is, in
// WittyModule's words, and leaves when the server closes the pipe. It comes before
// main, which has its own flags, and before any sandbox or core. Written out here so
// the runner needs nothing more than it did.
func init() {
	if len(os.Args) != 2 || os.Args[1] != "--wms-helper" {
		return
	}
	var m struct {
		ID      string `json:"id"`
		Version string `json:"version"`
		API     int    `json:"api"`
	}
	if err := json.Unmarshal(wittyplay.ManifestJSON, &m); err != nil {
		fmt.Fprintln(os.Stderr, "wittyplay-host:", err)
		os.Exit(1)
	}
	b, _ := json.Marshal(map[string]any{"module": m.ID, "version": m.Version, "api": m.API})
	if _, err := os.Stdout.Write(append(b, '\n')); err != nil {
		os.Exit(1)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}
