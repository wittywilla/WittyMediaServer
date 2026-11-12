// A helper for the tests. Unlike a real one it reads its manifest from beside
// itself, so one build can play any version, and capabilities["test"] says how to
// misbehave.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/wittywilla/wittymodule"
)

func main() {
	exe, _ := os.Executable()
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(exe), wittymodule.ManifestFile))
	if err != nil {
		os.Exit(9)
	}
	m, err := wittymodule.ParseManifest(raw)
	if err != nil {
		os.Exit(9)
	}
	do := m.Capabilities["test"]
	if slices.Contains(do, "die-at-start") {
		os.Exit(3)
	}
	if slices.Contains(do, "slow-hello") {
		time.Sleep(1500 * time.Millisecond) // a machine too busy to start it on time
	}
	if slices.Contains(do, "wrong-version") {
		m.Version = "9.9.9"
		raw, _ = json.Marshal(m)
	}
	wittymodule.Main(raw, func(method string, params json.RawMessage) (any, []byte, error) {
		switch {
		case slices.Contains(do, "crash"):
			os.Exit(4)
		case slices.Contains(do, "hang"):
			time.Sleep(time.Hour)
		case slices.Contains(do, "garbage"):
			os.Stdout.WriteString("this is not the protocol\n")
		}
		if slices.Contains(do, "one-call") {
			// A helper that answers one call and leaves, as one serving a stream does.
			defer time.AfterFunc(100*time.Millisecond, func() { os.Exit(0) })
		}
		switch method {
		case "read":
			return map[string]any{"Make": "fake " + m.Version}, nil, nil
		case "dimensions":
			return map[string]int{"w": 11, "h": 22}, nil, nil
		case "preview":
			if slices.Contains(do, "no-preview") {
				return nil, nil, &wittymodule.CallError{Code: "no-preview", Message: "no embedded preview"}
			}
			return nil, []byte("jpeg from " + m.Version), nil
		case "sdui.items":
			// A module's data source: one card per page asked for, saying what it was asked.
			var req struct {
				Source   map[string]any               `json:"source"`
				Offset   int                          `json:"offset"`
				Viewer   struct{ ProfileKind string } `json:"viewer"`
				Settings map[string]any               `json:"settings"`
			}
			_ = json.Unmarshal(params, &req)
			return map[string]any{"items": []map[string]any{{"id": fmt.Sprint("g", req.Offset), "kind": "game", "title": fmt.Sprint(req.Source["type"], " for ", req.Viewer.ProfileKind, " ", req.Settings["household"]),
				"route": "/page/wittyphotos", "images": map[string]string{"square": "/api/v1/sdui/modules/wittyphotos/images/g.png"}}}, "total": 1}, nil, nil
		case "sdui.image":
			return nil, []byte("\x89PNG\r\n\x1a\n" + m.Version), nil
		case "sandbox":
			// What WittyPlay's helper does on a Mac with each game: start it in a sandbox of its own.
			out, err := exec.Command("/usr/bin/sandbox-exec", "-p", "(version 1)(allow default)(deny network*)", "/usr/bin/true").CombinedOutput()
			if err != nil {
				return nil, nil, &wittymodule.CallError{Code: "sandbox", Message: err.Error() + ": " + string(out)}
			}
			return map[string]bool{"ok": true}, nil, nil
		}
		return nil, nil, &wittymodule.CallError{Code: "unknown-method", Message: "no such thing"}
	})
}
