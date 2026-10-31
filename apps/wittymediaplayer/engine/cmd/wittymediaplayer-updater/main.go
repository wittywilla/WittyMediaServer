// wittymediaplayer-updater looks for and installs newer versions of WittyMediaPlayer's
// disc modules. It's the only part of the player with network code, and it runs only when
// the viewer asks. Everything it fetches has to carry the project's signature.
//
//	wittymediaplayer-updater --check
//	wittymediaplayer-updater --install <module> <version>
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"

	"github.com/wittywilla/wittymediaplayer/engine/internal/disc"
	"github.com/wittywilla/wittymediaplayer/engine/internal/modupdate"
)

// Version is the player's version, set by the build.
var Version = "0.1.0"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	m := disc.NewManager(Version)
	c := modupdate.New(m)
	enc := json.NewEncoder(os.Stdout)
	args := os.Args[1:]
	switch {
	case len(args) == 1 && args[0] == "--check":
		offers := []modupdate.Offer{}
		for _, mod := range m.Modules {
			offers = append(offers, c.Check(ctx, mod))
		}
		_ = enc.Encode(map[string]any{"offers": offers})
	case len(args) == 3 && args[0] == "--install":
		mod, ok := m.Module(args[1])
		if !ok {
			fail(enc, fmt.Errorf("%q isn't one of this player's modules", args[1]))
		}
		if err := c.Install(ctx, mod, args[2]); err != nil {
			fail(enc, err)
		}
		_ = enc.Encode(map[string]any{"installed": args[2]})
	default:
		fmt.Fprintln(os.Stderr, "wittymediaplayer-updater --check | --install <module> <version>")
		os.Exit(2)
	}
}

func fail(enc *json.Encoder, err error) {
	_ = enc.Encode(map[string]any{"error": err.Error()})
	os.Exit(1)
}
