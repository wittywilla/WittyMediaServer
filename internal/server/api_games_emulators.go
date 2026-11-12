// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
	"github.com/wittywilla/wittymodule/service"
)

// Playing a game in a standalone emulator (PCSX2, PPSSPP and the like), which WittyPlay runs in
// a container or a sandbox of its own and streams. The module does the running; what's here is
// the part only the server can do: deciding that this account may play this game, finding its
// file, and handing WittyPlay exactly that file. A browser never names a path, and never
// reaches the module's start itself. See docs/GAMING.md, Standalone emulators.

// emulatorOffer is an emulator set up to play a game's console, as WittyPlay describes it.
type emulatorOffer struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	Stream       string `json:"stream"`
	Experimental bool   `json:"experimental,omitempty"`
	NeedsBios    bool   `json:"needsBios,omitempty"`
	Ready        bool   `json:"ready"`
	Why          string `json:"why,omitempty"`
}

// emulatorLaunch is what the server asks WittyPlay to start. Field names are the module's.
type emulatorLaunch struct {
	UserID, ItemID int64
	Title          string
	Console        string
	ROM            string
	SystemDir      string
	Emulator       string
}

// emulatorSession is a started game as WittyPlay answers, with paths on its own routes.
type emulatorSession struct {
	ID       string   `json:"id"`
	Backend  string   `json:"backend"`
	Emulator string   `json:"emulator"`
	Name     string   `json:"name"`
	Connect  string   `json:"connect,omitempty"`
	KasmURL  string   `json:"kasmUrl,omitempty"`
	Stop     string   `json:"stop"`
	Pause    string   `json:"pause"`
	Resume   string   `json:"resume"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
	FPS      int      `json:"fps"`
	Players  int      `json:"players"`
	Notices  []string `json:"notices"`
}

type emulatorRefusal struct {
	Code string `json:"code"`
	Text string `json:"error"`
	Busy bool   `json:"busy,omitempty"`
}

type emulatorResult struct {
	Session *emulatorSession `json:"session,omitempty"`
	Refusal *emulatorRefusal `json:"refusal,omitempty"`
}

// callWittyPlay is how the server asks the module something; a test stands in for it.
var callWittyPlay = func(ctx context.Context, name string, args []any, out []any) error {
	return service.Call(ctx, wittyPlayID, name, nil, nil, args, out)
}

// emulatorConsole finds a console by its id. WittyPlay holds the table; a test stands in for it.
var emulatorConsole = wittyplay.ByID

func (s *Server) gameEmulatorRoutes(m routes) {
	a := s.requireUser
	m.Handle("GET /api/games/{id}/emulators", a(s.handleGameEmulators))
	m.Handle("POST /api/games/{id}/play/emulator", a(s.handleGamePlayEmulator))
}

// emulatorGame is a game this account may see, on a console of this server's, whose file is
// there. The answer for anything else is "not found", whatever the reason, so the route says
// nothing about games an account's limits hide.
func (s *Server) emulatorGame(r *http.Request) (*library.Item, *wittyplay.Console, bool) {
	it, ok := s.gameItem(r)
	if !ok || it.Console == library.RigConsole {
		return nil, nil, false
	}
	console := emulatorConsole(it.Console)
	if console == nil || (console.Hidden && !s.testingGames()) {
		return nil, nil, false
	}
	return it, console, true
}

// handleGameEmulators lists the standalone emulators set up to play this game. It's what a
// game's page asks before offering "Play with ...", so it never fails for a game the account
// can see: with WittyPlay missing, its backend off, or nothing installed, the list is empty.
func (s *Server) handleGameEmulators(w http.ResponseWriter, r *http.Request) {
	it, console, ok := s.emulatorGame(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	out := []emulatorOffer{}
	if service.Running != nil && service.Running(wittyPlayID) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		var got []emulatorOffer
		if err := callWittyPlay(ctx, "docker.Offers", []any{console.ID, s.games().systemDir()}, []any{&got}); err != nil {
			s.Log.DebugContext(ctx, "games: asking WittyPlay which emulators play a game", "cat", "emulators", "item", it.ID, "err", err)
		} else if got != nil {
			out = got
		}
	}
	writeJSON(w, 200, map[string]any{"emulators": out})
}

// handleGamePlayEmulator starts a game in a standalone emulator. The account's limits were
// checked when the game was loaded; the file is the library's own, checked again here; and the
// module is told only that file, this account and the emulator's name.
func (s *Server) handleGamePlayEmulator(w http.ResponseWriter, r *http.Request) {
	it, console, ok := s.emulatorGame(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	var b struct {
		Emulator string `json:"emulator"`
		Device   string `json:"device"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !emulatorID(b.Emulator) {
		writeErr(w, 400, "emulator: which one?")
		return
	}
	ctx := r.Context()
	u := userFrom(r)
	g := s.games()
	systemDir := g.systemDir()
	if err := os.MkdirAll(systemDir, 0o755); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if _, err := os.Stat(it.Path); err != nil || !s.Store.LibraryFile(ctx, it.Path) {
		writeErr(w, 404, "the game file is missing")
		return
	}
	if _, err := s.configureGames(ctx); err != nil {
		writeErr(w, 503, err.Error())
		return
	}
	// Firmware left in a folder beside the games is brought to the BIOS folder first, as for a core.
	s.findBIOSNear(ctx, it.Path)
	rom, cleanup, err := g.prepareGame(ctx, s.Store, it, console)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		writeErr(w, 422, err.Error())
		return
	}
	done := func() {
		if cleanup != nil {
			cleanup()
		}
	}
	// Starting can mean pulling an image: minutes, the first time.
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Minute)
	defer cancel()
	var res emulatorResult
	err = callWittyPlay(cctx, "docker.Launch", []any{emulatorLaunch{UserID: u.ID, ItemID: it.ID, Title: it.Title, Console: console.ID, ROM: rom,
		SystemDir: systemDir, Emulator: b.Emulator}}, []any{&res})
	switch {
	case err != nil:
		done()
		if writeModuleMissing(w, err) {
			return
		}
		s.Log.WarnContext(ctx, "could not start a game in an emulator", "cat", "emulators", "module", wittyPlayID, "item", it.ID, "emulator", b.Emulator, "err", err)
		writeErr(w, 500, "This game could not start. "+err.Error())
		return
	case res.Refusal != nil:
		done()
		// The refusal's words name what is missing (a BIOS's or firmware's file name), never what is in a file.
		s.Log.WarnContext(ctx, "an emulator refused to start a game", "cat", "emulators", "module", wittyPlayID, "item", it.ID, "console", console.ID,
			"emulator", b.Emulator, "code", res.Refusal.Code, "reason", res.Refusal.Text, "busy", res.Refusal.Busy)
		status := 409
		if res.Refusal.Busy {
			status = 503
		}
		writeJSON(w, status, map[string]any{"error": res.Refusal.Text, "code": res.Refusal.Code})
		return
	case res.Session == nil || res.Session.ID == "":
		done()
		s.Log.WarnContext(ctx, "WittyPlay answered without a game", "cat", "emulators", "module", wittyPlayID, "item", it.ID, "emulator", b.Emulator)
		writeErr(w, 500, "This game could not start: WittyPlay answered without a game.")
		return
	}
	sess := res.Session
	ended := make(chan struct{})
	// Whose it is, for as long as it runs: see emulator_sessions.go.
	game := emulatorGame{id: sess.ID, session: sessionOf(r), user: u.ID, item: it.ID}
	s.emus.add(&game)
	started := time.Now()
	logging.Go("emulator session end", func() {
		why := waitForEmulator(sess.ID)
		s.emus.remove(sess.ID)
		close(ended)
		done()
		s.Log.Info("an emulator session ended", "cat", "emulators", "module", wittyPlayID, "session", sess.ID, "item", it.ID, "emulator", sess.Emulator,
			"played", time.Since(started).Round(time.Second).String(), "why", why)
	}, "cat", "emulators", "session", sess.ID)
	if accessFor(u) != nil {
		logging.Go("emulator limits", func() { s.watchEmulator(game, ended) }, "cat", "emulators", "session", sess.ID)
	}
	s.streams.game(sess.ID, u.ID, u.Name, it.Title, ended, func() GameView {
		return GameView{Core: sess.Name, Width: sess.Width, Height: sess.Height}
	})
	logging.Go("emulator play time", func() { s.recordEmulatorTime(ended, u.ID, it.ID) }, "cat", "emulators", "session", sess.ID)
	if err := s.Store.NoteGamePlayed(ctx, u.ID, it.ID); err != nil {
		s.Log.WarnContext(ctx, "recording a play", "cat", "emulators", "session", sess.ID, "err", err)
	}
	s.Log.InfoContext(ctx, "a game started in an emulator", "cat", "emulators", "module", wittyPlayID, "session", sess.ID, "item", it.ID, "console", console.ID, "game", it.Title, "emulator", sess.Emulator, "backend", sess.Backend,
		"device", deviceName(b.Device, r))
	// The module's paths are its own; a client reaches them under this server's.
	under := func(p string) string {
		if p == "" || !strings.HasPrefix(p, "/") {
			return ""
		}
		return "/api/v1/modules/" + wittyPlayID + p
	}
	sess.Connect, sess.KasmURL, sess.Stop, sess.Pause, sess.Resume = under(sess.Connect), under(sess.KasmURL), under(sess.Stop), under(sess.Pause), under(sess.Resume)
	if sess.Notices == nil {
		sess.Notices = []string{}
	}
	writeJSON(w, 201, sess)
}

// emulatorID says a name could be an emulator's: what the module's own list allows.
func emulatorID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for i, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || i > 0 && (c == '-' || c == '_')) {
			return false
		}
	}
	return true
}

// waitForEmulator returns when a game in an emulator has ended. The module going away ends it
// too: an emulator doesn't outlive the module that watches it.
func waitForEmulator(id string) (why string) {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		var over bool
		err := callWittyPlay(ctx, "stream.Session.Wait", []any{id}, []any{&over})
		cancel()
		switch {
		case over:
			return "ended"
		case err != nil && !errors.Is(err, context.DeadlineExceeded):
			// WittyPlay went, or forgot the session: either way it is over here.
			return "lost: " + err.Error()
		}
	}
}

// recordEmulatorTime adds the time played to what "Continue playing" shows, as for a core.
func (s *Server) recordEmulatorTime(ended <-chan struct{}, userID, itemID int64) {
	start := time.Now()
	<-ended
	ctx := context.Background()
	played := time.Since(start).Seconds()
	var total float64
	_ = s.DB.QueryRowContext(ctx, `SELECT position_sec FROM progress WHERE user_id = ? AND item_id = ?`, userID, itemID).Scan(&total)
	if err := s.Store.SetProgress(ctx, userID, itemID, library.Progress{PositionSec: total + played}); err != nil {
		s.Log.Warn("could not save time played", "cat", "emulators", "item", itemID, "err", err)
	}
}
