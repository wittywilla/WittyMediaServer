// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/wittywilla/wittymodule"
)

// minorRange is a pre-1.0 limit such as "0.6.x"; the module API number decides whether it can run.
var minorRange = regexp.MustCompile(`^0\.\d+\.x$`)

func widen(max string) string {
	if minorRange.MatchString(max) {
		return "0.x"
	}
	return max
}

// supports checks a manifest, letting modules made for any 0.x server run on this one when the API
// fits. A version that fits only because of that is made for an older server (olderServer): it can
// run, but only someone who was told so and said go ahead installs it. Nothing automatic does.
func supports(man *wittymodule.Manifest, server string) error {
	c := *man
	c.MaxServer = widen(c.MaxServer)
	return c.Supports(server, MinAPI, API)
}

// olderServer reports a version whose own range stops below this server and which fits only once
// widened: it was made for an older server, and the pages, settings and features this one expects
// of it may be missing. madeFor is the range it was made for, e.g. "0.6.x". A minServer above this
// server is never widened, so a version made for a newer server is refused, not this.
func olderServer(man *wittymodule.Manifest, server string) (madeFor string, ok bool) {
	if man == nil || man.Supports(server, MinAPI, API) == nil || supports(man, server) != nil {
		return "", false
	}
	return man.MaxServer, true
}

// olderServerListed is olderServer for a version as a catalogue lists it.
func olderServerListed(name string, v *wittymodule.CatalogVersion, server string) (string, bool) {
	return olderServer(&wittymodule.Manifest{Name: name, Version: v.Version, API: v.API, MinServer: v.MinServer, MaxServer: v.MaxServer}, server)
}

// seriesOf is a version as people say which release it belongs to: "0.6.x" and "0.6.2" are "0.6".
func seriesOf(v string) string {
	v = strings.TrimSuffix(v, ".x")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if parts := strings.Split(v, "."); len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return v
}

// OlderServerWarning is what is said before a version made for an older server is installed, and
// while one is in use: which server it was made for, which this is, and what that means.
func OlderServerWarning(name, version, madeFor, server string) string {
	return fmt.Sprintf("%s %s was made for WittyMediaServer %s; this server is %s. Pages, settings and features from %s will be missing, and it may not work.",
		name, version, seriesOf(madeFor), server, seriesOf(server))
}

// ErrOlderServer refuses a version made for an older server that nobody confirmed. Installing it
// needs the request to say so (confirmOlder), after a person has read Warning.
type ErrOlderServer struct {
	Module  string `json:"module"`
	Name    string `json:"name"`
	Version string `json:"version"`
	MadeFor string `json:"madeFor"`
	Server  string `json:"server"`
	Warning string `json:"warning"`
}

func (e *ErrOlderServer) Error() string {
	return e.Warning + " It is installed only if you confirm."
}

func newErrOlderServer(id, name, version, madeFor, server string) *ErrOlderServer {
	return &ErrOlderServer{Module: id, Name: name, Version: version, MadeFor: madeFor, Server: server, Warning: OlderServerWarning(name, version, madeFor, server)}
}
