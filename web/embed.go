// Package web embeds the built single-page client (web/dist).
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

//go:embed src/modules/*/module.json
var moduleManifests embed.FS

// ModuleManifests returns the manifests of the web modules in this client.
func ModuleManifests() [][]byte {
	files, err := fs.Glob(moduleManifests, "src/modules/*/module.json")
	if err != nil {
		panic(err)
	}
	out := make([][]byte, 0, len(files))
	for _, f := range files {
		b, err := moduleManifests.ReadFile(f)
		if err != nil {
			panic(err)
		}
		out = append(out, b)
	}
	return out
}

// Dist returns the built client as a filesystem rooted at dist/.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
