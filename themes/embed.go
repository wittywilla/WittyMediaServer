// Package themes holds the built-in interface themes. Each folder is one
// theme (theme.json + theme.css); at start-up the server packs every folder
// into <data>/themes/<id>.zip, the same format as user-made themes.
package themes

import "embed"

//go:embed */theme.json */theme.css
var Builtin embed.FS
