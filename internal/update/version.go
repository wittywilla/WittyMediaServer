// Package update finds, verifies and installs new releases of
// WittyMediaServer from its GitHub releases.
//
// Nothing is installed unless the release's checksum list carries a valid
// Ed25519 signature from a key built into this program, and the downloaded
// archive matches its checksum. Installing replaces the program's files
// next to the running ones and never touches the data folder.
package update

import "github.com/wittywilla/wittymodule"

// Compare orders two versions: -1 if a is older than b, 0 if equal, 1 if
// newer. A pre-release (0.4.0-alpha) comes before its release (0.4.0).
// Versions that cannot be read sort before everything else.
func Compare(a, b string) int { return wittymodule.Compare(a, b) }

// Valid reports whether s is a version this package can compare.
func Valid(s string) bool { return wittymodule.Valid(s) }
