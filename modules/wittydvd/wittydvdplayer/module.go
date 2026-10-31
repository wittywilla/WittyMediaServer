package wittydvdplayer

import _ "embed"

// Manifest is this module's manifest, built into the helper and into a player's
// built-in copy, so they can't disagree. Its minServer and maxServer are the versions
// of WittyMediaPlayer it works with.
//
//go:embed module.json
var Manifest []byte
