// Command wittybluray-module is WittyBluray as a helper process, so a new version
// can reach a server without a new server. See WittyModule for the protocol, and
// package helper for what it answers.
package main

import (
	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittybluray/helper"
	"github.com/wittywilla/wittymodule"
)

func main() {
	h := &helper.Helper{}
	wittymodule.Main(wittybluray.ManifestJSON, h.Handle)
}
