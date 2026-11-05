package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"slices"

	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
	"github.com/wittywilla/wittymediaserver/internal/play/cores"
)

// The core picker: every core that can play a console, installed or not, with what each is good
// at, where it runs and how it draws, whether it needs a BIOS, and whether it can be installed
// here. One answer for a console's default (GET /gaming/consoles/{console}/core) and for one game
// (GET /gaming/games/{id}/core, beside its older options). Choosing one that isn't installed
// installs it first (POST /gaming/cores/{id}/install, with GET for its progress), then sets it
// with the PUT beside each GET, which only takes an installed core.

// coreChoice is one core in the picker.
type coreChoice struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	License       string `json:"license"`
	NonCommercial bool   `json:"nonCommercial,omitempty"`
	// Installed: it's on this server, and Source says from where (installed, retroarch, system or
	// custom). CanInstall: it isn't, and installing it here would work.
	Installed  bool   `json:"installed"`
	Source     string `json:"source,omitempty"`
	CanInstall bool   `json:"canInstall,omitempty"`
	// Recommended is the core this server would pick for the console: the first in its list that's
	// published for this platform and can draw here.
	Recommended bool `json:"recommended,omitempty"`
	// Strength is accuracy, speed or balanced, beside the console's other cores, and About is a
	// line on what it's good at (core_notes.go). Neither is there for a custom core.
	Strength string `json:"strength,omitempty"`
	About    string `json:"about,omitempty"`
	// Support is where it runs and how it draws, here and on every platform, as in
	// GET /games/consoles. Not for a custom core.
	Support *coreSupport `json:"support,omitempty"`
	// BIOS, when it won't start without a file of the owner's own: any one of Files, and whether
	// this server has one.
	BIOS *coreBIOSNeed `json:"bios,omitempty"`
	// For one game: how the core has gone on it.
	Starts    int     `json:"starts,omitempty"`
	Failures  int     `json:"failures,omitempty"`
	Seconds   float64 `json:"seconds,omitempty"`
	LastError string  `json:"lastError,omitempty"`
	Best      bool    `json:"best,omitempty"`
}

type coreBIOSNeed struct {
	Files []string `json:"files"`
	Found bool     `json:"found"`
}

// coreChoicesView is the picker's answer.
type coreChoicesView struct {
	Console string `json:"console"`
	// Choice is what was chosen at this level: the game's own core, or the console's default; ""
	// for none. ConsoleDefault, for a game, is its console's.
	Choice         string `json:"choice"`
	ConsoleDefault string `json:"consoleDefault,omitempty"`
	// Effective is what plays now, after a missing BIOS or graphics card is taken into account;
	// "" when nothing would.
	Effective   string       `json:"effective"`
	CanDownload bool         `json:"canDownload"`
	Choices     []coreChoice `json:"choices"`
}

// coreChoices lists a console's cores for the picker; itemID is 0 for the console's default.
func (s *Server) coreChoices(ctx context.Context, console *wittyplay.Console, itemID int64) coreChoicesView {
	g := s.games()
	systemDir := g.systemDir()
	out := coreChoicesView{Console: console.ID, CanDownload: cores.CanDownload(), Choices: []coreChoice{}}
	prefs := s.preferredCores(ctx)
	if itemID != 0 {
		out.Choice, out.ConsoleDefault = s.gameCoreChoice(ctx, itemID), prefs[console.ID]
	} else {
		out.Choice = prefs[console.ID]
	}
	if pick := s.pickCore(ctx, console, itemID, systemDir); pick.core != nil {
		out.Effective = pick.core.ID
	}
	have := map[string]cores.Core{}
	for _, c := range g.cores.List() {
		if !c.Unapproved {
			have[c.ID] = c
		}
	}
	noGPU, asked := "", false
	recommended := false
	for _, want := range console.Cores {
		gpuCore := wittyplay.Traits(want.ID).GPU
		if gpuCore && !asked {
			noGPU, asked = s.gpuMissing(ctx), true
		}
		support := coreSupportFor(want.ID, gpuCore, noGPU)
		c := coreChoice{ID: want.ID, Name: want.Name, License: want.License, Support: &support}
		if got, ok := have[want.ID]; ok {
			c.Installed, c.Source = true, got.Source
		}
		playsHere := support.Here.Build && support.Here.Draws != drawsNone
		c.CanInstall = !c.Installed && out.CanDownload && playsHere && cores.Offered(want.ID)
		if playsHere && !recommended {
			c.Recommended, recommended = true, true
		}
		if n := coreNoteFor(want.ID); n != nil {
			c.Strength, c.About = n.Strength, n.About
		}
		c.BIOS = coreBIOS(want.ID, systemDir)
		out.Choices = append(out.Choices, c)
	}
	// An administrator's own cores for this console, which are installed by being added.
	for _, c := range g.cores.List() {
		if c.Source != cores.SourceCustom || c.Unapproved || !slices.Contains(c.Consoles, console.ID) || consoleListsCore(console, c.ID) {
			continue
		}
		out.Choices = append(out.Choices, coreChoice{ID: c.ID, Name: c.Name, License: c.License, NonCommercial: c.NonCommercial, Installed: true, Source: c.Source})
	}
	if itemID != 0 {
		options := make([]GameCoreOption, len(out.Choices))
		for i, c := range out.Choices {
			options[i].ID = c.ID
		}
		s.markBestCore(ctx, itemID, options)
		for i, o := range options {
			c := &out.Choices[i]
			c.Starts, c.Failures, c.Seconds, c.LastError, c.Best = o.Starts, o.Failures, o.Seconds, o.LastError, o.Best && c.Installed
		}
	}
	return out
}

// coreBIOS is the BIOS a core won't start without, from its traits in the module or its note
// here, and whether this server has one; nil for a core that starts without.
func coreBIOS(id, systemDir string) *coreBIOSNeed {
	if rule := wittyplay.Traits(id).RequiredBIOS; rule != nil {
		return &coreBIOSNeed{Files: []string{rule.Example}, Found: wittyplay.MissingBIOS(id, systemDir) == ""}
	}
	n := coreNoteFor(id)
	if n == nil || len(n.BIOS) == 0 {
		return nil
	}
	need := &coreBIOSNeed{Files: n.BIOS}
	for _, f := range n.BIOS {
		if st, err := os.Stat(filepath.Join(systemDir, filepath.FromSlash(f))); err == nil && st.Mode().IsRegular() {
			need.Found = true
			break
		}
	}
	return need
}

// GET /api/gaming/consoles/{console}/core: the console's cores for the picker, and its default.
func (s *Server) handleConsoleCoreGet(w http.ResponseWriter, r *http.Request) {
	console := s.gameConsole(r.PathValue("console"))
	if console == nil {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, s.coreChoices(r.Context(), console, 0))
}

// GET /api/gaming/cores/{id}/install: how far an install of this core has got, while one is
// under way: bytes received and the download's size (0 when the download server didn't say).
// A module from before this was asked answers as though nothing were installing.
func (s *Server) handleCoreInstallProgress(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !coreIDRe.MatchString(id) {
		writeErr(w, 404, "not found")
		return
	}
	got, total, ok := s.games().cores.InstallProgress(id)
	writeJSON(w, 200, map[string]any{"installing": ok, "received": got, "total": total})
}
