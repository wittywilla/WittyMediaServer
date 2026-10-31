package modules

import (
	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittydisc"
	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittyintegratedfilebrowser"
	"github.com/wittywilla/wittymoonlight"
	"github.com/wittywilla/wittyphotos"
	"github.com/wittywilla/wittyplay"
	"github.com/wittywilla/wittysunshine"
	"github.com/wittywilla/wittytuner"
)

// GoManifests are the manifests of the Go modules compiled into this server. Only
// modules listed here (and the web ones) are ever accepted: a release of anything
// else has nowhere to go.
func GoManifests() [][]byte {
	return [][]byte{wittybluray.ManifestJSON, wittydisc.ManifestJSON, wittydvd.ManifestJSON, wittyintegratedfilebrowser.ManifestJSON,
		wittymoonlight.ManifestJSON, wittyphotos.ManifestJSON, wittyplay.ManifestJSON, wittysunshine.ManifestJSON, wittytuner.ManifestJSON}
}
