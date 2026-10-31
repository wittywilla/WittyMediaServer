// Command wittydvd-module is WittyDVD as a helper process, one for each open disc, so
// a new version of the engine can reach a server without a new server. See WittyModule
// for the protocol and package wire for what is said over it.
package main

import (
	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittydvd/wire"
	"github.com/wittywilla/wittymodule"
)

func main() { wittymodule.Main(wittydvd.ManifestJSON, new(wire.Helper).Handle) }
