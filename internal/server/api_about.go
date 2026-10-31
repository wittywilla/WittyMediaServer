package server

import (
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	wms "github.com/wittywilla/wittymediaserver"
	"github.com/wittywilla/wittymediaserver/internal/licenses"
)

// The engines built into this server, with the versions it was built
// against, for Settings, About. A module built from this repository's own
// copy (not a downloaded release) is marked local.

type moduleInfo struct {
	Name    string `json:"name"`
	Module  string `json:"module"`
	Version string `json:"version"`
	Local   bool   `json:"local,omitempty"`
	Role    string `json:"role"`
}

var engines = []moduleInfo{
	{Name: "WittyDVD", Module: "github.com/wittywilla/wittydvd", Role: "DVD engine: menus, titles and navigation"},
	{Name: "WittyBluray", Module: "github.com/wittywilla/wittybluray", Role: "Blu-ray engine: menus, playlists, 4K and HDR"},
	{Name: "WittyDisc", Module: "github.com/wittywilla/wittydisc", Role: "Disc image reader (UDF and ISO 9660)"},
	{Name: "WittyPlay", Module: "github.com/wittywilla/wittyplay", Role: "Game streaming: runs games through libretro cores and streams them to the browser"},
	{Name: "WittySunshine", Module: "github.com/wittywilla/wittysunshine", Role: "Advanced: streaming demanding games hosted on this server, through Sunshine as a separate program"},
	{Name: "WittyMoonlight", Module: "github.com/wittywilla/wittymoonlight", Role: "A Moonlight client in the page, for playing on any Sunshine server"},
	{Name: "WittyIntegratedFileBrowser", Module: "github.com/wittywilla/wittyintegratedfilebrowser", Role: "The built-in folder browser: drives and folders, from any device"},
	{Name: "WittyTuner", Module: "github.com/wittywilla/wittytuner", Role: "TV tuners and IPTV: network tuners, playlists, DVB, capture cards, SAT>IP and backends"},
}

var (
	modulesOnce sync.Once
	modules     []moduleInfo
)

func builtModules() []moduleInfo {
	modulesOnce.Do(func() {
		deps := map[string]*debug.Module{}
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, d := range bi.Deps {
				deps[d.Path] = d
			}
		}
		for _, m := range engines {
			if d := deps[m.Module]; d != nil {
				m.Version = strings.TrimPrefix(d.Version, "v")
				m.Local = d.Replace != nil && d.Replace.Version == "" || d.Version == "(devel)"
			}
			if m.Version == "" || m.Version == "(devel)" || m.Version == "0.0.0" {
				// Built from the workspace's own copy: the version go.mod requires.
				m.Version, m.Local = "dev", true
				if v := regexp.MustCompile(regexp.QuoteMeta(m.Module) + `\s+v(\S+)`).FindStringSubmatch(wms.GoMod); v != nil {
					m.Version = v[1]
				}
			}
			modules = append(modules, m)
		}
	})
	return modules
}

// GET /api/licenses: everything in this server that someone else made, with its license.
func (s *Server) handleLicenses(w http.ResponseWriter, r *http.Request) {
	all := licenses.All()
	out := make([]licenses.Component, len(all))
	copy(out, all)
	// The versions really in this program, which a list made earlier can't know.
	versions := map[string]string{}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			versions[d.Path] = strings.TrimPrefix(d.Version, "v")
		}
	}
	for i := range out {
		out[i].Version = versions[out[i].Name]
	}
	writeJSON(w, 200, map[string]any{"groups": licenses.Groups, "components": out})
}

// GET /api/licenses/{id}: one component's full license text.
func (s *Server) handleLicenseText(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	text, ok := licenses.Text(id)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, map[string]any{"text": text})
}

// GET /api/about: the server's version and its engines (any signed-in user).
func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"version": s.Version, "modules": builtModules()})
}
