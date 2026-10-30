// Package wittymediaserver holds this module's go.mod, so the server can
// report the versions of the engines it requires (Settings, About).
package wittymediaserver

import _ "embed"

//go:embed go.mod
var GoMod string
