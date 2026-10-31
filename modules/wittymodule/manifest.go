// Package wittymodule is what a WittyMediaServer plugin needs to be a project of
// its own: the manifest, the rules for version ranges, the protocol a helper
// process speaks, and the names of a release's files. It has no dependencies, so
// a plugin that uses it still has none of its own.
package wittymodule

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// The module API: the contract between the server and its plugins. A plugin
// names the version it was built for, and a server takes MinAPI to API. Adding
// to the contract raises API only; breaking it raises MinAPI too, which disables
// older plugins, so that is done rarely.
const (
	API    = 1
	MinAPI = 1
)

// How a release of a plugin reaches the server.
const (
	DeliveryHelper  = "helper"  // a program for each platform, run as a separate process
	DeliveryWeb     = "web"     // a bundle the browser loads
	DeliveryBuiltin = "builtin" // not converted yet: it updates with the server
)

// ManifestFile is the manifest's name in a plugin's folder and in a release.
const ManifestFile = "module.json"

// Manifest describes one version of a plugin.
type Manifest struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Repo      string `json:"repo"` // owner/name on GitHub, where its releases are
	Delivery  string `json:"delivery"`
	Entry     string `json:"entry,omitempty"` // the helper program, without .exe
	API       int    `json:"api"`
	MinServer string `json:"minServer"`
	MaxServer string `json:"maxServer"`
	// Capabilities are facts the server needs without starting the helper, such
	// as "rawExtensions". Signed along with everything else.
	Capabilities map[string][]string `json:"capabilities,omitempty"`
}

var (
	idRe    = regexp.MustCompile(`^[a-z][a-z0-9]{1,63}$`)
	entryRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	repoRe  = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`)
)

// ParseManifest reads and checks a manifest.
func ParseManifest(b []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("unreadable manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate checks that a manifest makes sense on its own.
func (m *Manifest) Validate() error {
	name := m.ID
	if name == "" {
		name = "the module"
	}
	switch {
	case !idRe.MatchString(m.ID):
		return fmt.Errorf("manifest: %q is not a module id (lowercase letters and digits)", m.ID)
	case m.Name == "":
		return fmt.Errorf("manifest of %s: no name", name)
	case !Valid(m.Version):
		return fmt.Errorf("manifest of %s: %q is not a version", name, m.Version)
	case !repoRe.MatchString(m.Repo):
		return fmt.Errorf("manifest of %s: %q is not a GitHub repository (owner/name)", name, m.Repo)
	case m.API < 1:
		return fmt.Errorf("manifest of %s: api must be 1 or more", name)
	case !Valid(m.MinServer):
		return fmt.Errorf("manifest of %s: minServer %q is not a version", name, m.MinServer)
	}
	if _, _, ok := parseMax(m.MaxServer); !ok {
		return fmt.Errorf("manifest of %s: maxServer %q is not a version or a series such as 0.5.x", name, m.MaxServer)
	}
	if !atMost(m.MinServer, m.MaxServer) {
		return fmt.Errorf("manifest of %s: minServer %s is past maxServer %s", name, m.MinServer, m.MaxServer)
	}
	switch m.Delivery {
	case DeliveryHelper:
		if !entryRe.MatchString(m.Entry) {
			return fmt.Errorf("manifest of %s: entry %q is not a program name", name, m.Entry)
		}
	case DeliveryWeb, DeliveryBuiltin:
	default:
		return fmt.Errorf("manifest of %s: delivery %q is not helper, web or builtin", name, m.Delivery)
	}
	return nil
}

// Supports says, in words an owner can read, why this version can't run on a
// server of the given version and module API range; nil when it can.
func (m *Manifest) Supports(server string, minAPI, maxAPI int) error {
	if !Valid(server) {
		return fmt.Errorf("this server's version (%q) can't be compared, so it uses its built-in %s", server, m.Name)
	}
	if Compare(server, m.MinServer) < 0 {
		return fmt.Errorf("%s %s needs WittyMediaServer %s or newer, and this server is %s", m.Name, m.Version, m.MinServer, server)
	}
	if !atMost(server, m.MaxServer) {
		return fmt.Errorf("%s %s supports WittyMediaServer up to %s, and this server is %s", m.Name, m.Version, m.MaxServer, server)
	}
	if m.API > maxAPI {
		return fmt.Errorf("%s %s needs a newer server: it uses module API %d, and this server has up to %d", m.Name, m.Version, m.API, maxAPI)
	}
	if m.API < minAPI {
		return fmt.Errorf("%s %s is too old for this server: it uses module API %d, and this server needs at least %d", m.Name, m.Version, m.API, minAPI)
	}
	return nil
}

// EntryFile is the helper program's file name on a system.
func (m *Manifest) EntryFile(goos string) string {
	if goos == "windows" {
		return m.Entry + ".exe"
	}
	return m.Entry
}

// Target is a platform a helper is built for.
type Target struct{ GOOS, GOARCH string }

// Targets are the platforms the server ships to, as in scripts/build-release.sh.
var Targets = []Target{{"windows", "amd64"}, {"windows", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"},
	{"linux", "amd64"}, {"linux", "arm64"}, {"linux", "arm"}, {"linux", "386"}, {"freebsd", "amd64"}, {"freebsd", "arm64"}}

// The names of a release's files.

// TopFolder is the one folder inside a release's archive.
func TopFolder(id, ver, goos, goarch string) string {
	label := goarch
	if goos == "linux" && goarch == "arm" {
		label = "armv7"
	}
	return id + "-" + strings.TrimPrefix(ver, "v") + "-" + goos + "-" + label
}

// ArchiveName is the release archive for a platform.
func ArchiveName(id, ver, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return TopFolder(id, ver, goos, goarch) + ext
}

// SumsName is the signed list of a release's hashes; its signature adds .sig.
func SumsName(id, ver string) string {
	return id + "-" + strings.TrimPrefix(ver, "v") + "-SHA256SUMS.txt"
}

// ManifestName is the copy of the manifest attached to a release, so it can be
// judged before anything large is downloaded.
func ManifestName(id, ver string) string {
	return id + "-" + strings.TrimPrefix(ver, "v") + "-module.json"
}
