package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	_ "image/jpeg" // the formats a person's own image may be in
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittyplay"
	"github.com/wittywilla/wittyplay/ipc"
	"github.com/wittywilla/wittyplay/patch"
	"github.com/wittywilla/wittyplay/stream"
	_ "golang.org/x/image/webp"
)

// A person's settings and own files for their games: what they've chosen, at which
// level, and how a running game hears about it. docs/GAMING.md has the reasoning.

func (s *Server) gameOptionRoutes(m *http.ServeMux) {
	a := s.requireUser
	m.Handle("GET /api/games/settings/schema", a(s.handleGameSettingsSchema))
	m.Handle("GET /api/games/settings", a(s.handleGameSettingsGet))
	m.Handle("PUT /api/games/settings", a(s.handleGameSettingsPut))
	m.Handle("POST /api/games/settings/validate", a(s.handleGameSettingsValidate))
	m.Handle("GET /api/games/recent", a(s.handleGamesRecent))
	m.Handle("GET /api/game-bezels", a(s.handleBezels))
	m.Handle("POST /api/game-bezels", a(s.handleBezelAdd))
	m.Handle("GET /api/game-bezels/{bezel}", a(s.handleBezelImage))
	m.Handle("DELETE /api/game-bezels/{bezel}", a(s.handleBezelDelete))
	m.Handle("GET /api/games/{id}/core-options", a(s.handleGameCoreOptions))
	m.Handle("GET /api/games/{id}/meta", a(s.handleGameMeta))
	m.Handle("PUT /api/games/{id}/meta", a(s.handleGameMetaPut))
	m.Handle("GET /api/games/{id}/cheats", a(s.handleGameCheats))
	m.Handle("PUT /api/games/{id}/cheats", a(s.handleGameCheatsPut))
	m.Handle("POST /api/games/{id}/cheats/import", a(s.handleGameCheatsImport))
	m.Handle("GET /api/games/{id}/patches", a(s.handleGamePatches))
	m.Handle("POST /api/games/{id}/patches", a(s.handleGamePatchAdd))
	m.Handle("PUT /api/games/{id}/patches/{patch}", a(s.handleGamePatchPut))
	m.Handle("DELETE /api/games/{id}/patches/{patch}", a(s.handleGamePatchDelete))
	m.Handle("GET /api/games/{id}/saves/{slot}/file", a(s.handleSaveExport))
	m.Handle("PUT /api/games/{id}/saves/{slot}/file", a(s.handleSaveImport))
	m.Handle("GET /api/games/{id}/battery", a(s.handleBatteryExport))
	m.Handle("PUT /api/games/{id}/battery", a(s.handleBatteryImport))
	m.Handle("GET /api/play/{sid}/stats", a(s.handlePlayStats))
	m.Handle("GET /api/play/{sid}/screenshot", a(s.handlePlayScreenshot))
}

// gameLimits is what this server allows, whatever anyone's settings say.
func (s *Server) gameLimits(ctx context.Context) wittyplay.Limits {
	// A small machine has less to spare, unless its administrator says otherwise.
	speed, rewind := 4.0, int64(32)
	if runtime.NumCPU() < 4 {
		speed, rewind = 2, 16
	}
	if v, err := strconv.ParseFloat(s.DB.GetSetting(ctx, "gamingMaxSpeed", ""), 64); err == nil && v >= 1 {
		speed = min(v, ipc.SpeedMax)
	}
	if v, err := strconv.ParseInt(s.DB.GetSetting(ctx, "gamingRewindMB", ""), 10, 64); err == nil && v >= 0 {
		rewind = min(v, ipc.RewindBytesMax>>20)
	}
	return wittyplay.Limits{MaxSpeed: speed, RewindBytes: rewind << 20, RunAhead: s.DB.GetSetting(ctx, "gamingRunAhead", "1") != "0"}
}

const gameDefaultsKey = "gamingDefaults"

func gameDefaultsSetting(console string) string {
	if console == "" {
		return gameDefaultsKey
	}
	return gameDefaultsKey + "." + console
}

// serverGameLayer reads what an administrator set for everyone.
func (s *Server) serverGameLayer(ctx context.Context, console string) *wittyplay.Layer {
	l := &wittyplay.Layer{}
	if raw := s.DB.GetSetting(ctx, gameDefaultsSetting(console), ""); raw != "" {
		if json.Unmarshal([]byte(raw), l) != nil {
			return &wittyplay.Layer{}
		}
	}
	return l
}

// gameLayers reads every level that applies, least specific first.
func (s *Server) gameLayers(ctx context.Context, userID int64, console string, itemID int64) []wittyplay.NamedLayer {
	out := []wittyplay.NamedLayer{{Level: wittyplay.LevelServer, Layer: s.serverGameLayer(ctx, "")}}
	if console != "" {
		out = append(out, wittyplay.NamedLayer{Level: wittyplay.LevelServerConsole, Layer: s.serverGameLayer(ctx, console)})
	}
	add := func(level, c string, id int64) {
		l, err := s.Store.GameLayer(ctx, userID, c, id)
		if err != nil {
			s.Log.Warn("reading game settings", "err", err)
			l = &wittyplay.Layer{}
		}
		out = append(out, wittyplay.NamedLayer{Level: level, Layer: l})
	}
	add(wittyplay.LevelPerson, "", 0)
	if console != "" {
		add(wittyplay.LevelConsole, console, 0)
	}
	if itemID != 0 {
		add(wittyplay.LevelGame, "", itemID)
	}
	return out
}

// applyGameSettings gives a game about to start what its player has chosen, unless
// they asked for safe mode: then it gets nothing but defaults, and a way to start.
func (s *Server) applyGameSettings(ctx context.Context, o *stream.Options, it *library.Item, console *wittyplay.Console, coreID string, safe bool) {
	lim := s.gameLimits(ctx)
	o.MaxSpeed = lim.MaxSpeed
	o.SafeMode = safe
	o.OnCoreOptions = func(list []ipc.Option) { s.keepCoreOptions(coreID, list) }
	if safe {
		return
	}
	res := wittyplay.Resolve(s.gameLayers(ctx, o.UserID, console.ID, it.ID))
	control := wittyplay.ControlFor(res.Values, lim)
	o.Control = &control
	o.CoreOptions = res.CoreOptions
	o.NoAutoSave = res.Values["states.autoSave"] == false
	if meta, err := s.Store.GetGameMeta(ctx, o.UserID, it.ID); err == nil {
		o.Cheats = wittyplay.CheatsForCore(meta.Cheats)
	}
	rom, cleanup, notices := s.games().patchROM(ctx, s.Store, o.UserID, it.ID, o.ROM, o.Cleanup)
	o.ROM, o.Cleanup, o.Notices = rom, cleanup, append(o.Notices, notices...)
}

func (s *Server) keepCoreOptions(coreID string, list []ipc.Option) {
	out := make([]wittyplay.OptionInfo, 0, len(list))
	for _, o := range list {
		out = append(out, optionInfo(o))
	}
	if err := s.Store.SetCoreOptionList(context.Background(), coreID, out); err != nil {
		s.Log.Warn("keeping a core's options", "err", err)
	}
}

func optionInfo(o ipc.Option) wittyplay.OptionInfo {
	return wittyplay.OptionInfo{Key: o.Key, Label: o.Label, Choices: o.Choices, Default: o.Choices[0]}
}

func (s *Server) handleGameSettingsSchema(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"settings": wittyplay.Settings, "presets": wittyplay.BuiltinPresets, "levels": []string{wittyplay.LevelServer, wittyplay.LevelServerConsole, wittyplay.LevelPerson, wittyplay.LevelConsole, wittyplay.LevelGame}, "maxSlot": stream.MaxSlot, "undoSlot": stream.SlotUndo, "prevAutoSlot": stream.SlotPrevAuto})
}

// gameScope works out which console and game a request is about, from ?item= or ?console=.
func (s *Server) gameScope(r *http.Request, consoleQ string, itemQ int64) (console string, itemID int64, ok bool) {
	if itemQ != 0 {
		it, err := s.visibleItem(r, itemQ)
		if err != nil || it.Kind != "game" {
			return "", 0, false
		}
		return it.Console, it.ID, true
	}
	if consoleQ != "" && s.gameConsole(consoleQ) == nil {
		return "", 0, false
	}
	return consoleQ, 0, true
}

func (s *Server) settingsView(ctx context.Context, userID int64, console string, itemID int64) map[string]any {
	layers := s.gameLayers(ctx, userID, console, itemID)
	res := wittyplay.Resolve(layers)
	byLevel := map[string]*wittyplay.Layer{}
	for _, l := range layers {
		byLevel[l.Level] = l.Layer
	}
	return map[string]any{"resolved": res, "layers": byLevel, "limits": s.gameLimits(ctx), "console": console, "item": itemID}
}

func (s *Server) handleGameSettingsGet(w http.ResponseWriter, r *http.Request) {
	console, itemID, ok := s.gameScope(r, r.URL.Query().Get("console"), int64(qInt(r, "item", 0)))
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, s.settingsView(r.Context(), userFrom(r).ID, console, itemID))
}

// handleGameSettingsPut changes one level: a patch (null removes a value), a whole
// settings file, or a reset. Anyone may change their own; only an administrator can
// change what the server gives everyone.
func (s *Server) handleGameSettingsPut(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Level       string             `json:"level"`
		Console     string             `json:"console"`
		Item        int64              `json:"item"`
		Values      map[string]any     `json:"settings"`
		CoreOptions map[string]*string `json:"coreOptions"`
		Document    *string            `json:"document"`
		Reset       bool               `json:"reset"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, wittyplay.MaxCustomSize*2)
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "these settings can't be read")
		return
	}
	ctx := r.Context()
	u := userFrom(r)
	console, itemID, ok := s.gameScope(r, b.Console, b.Item)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	read := func() (*wittyplay.Layer, error) { return nil, nil }
	write := func(*wittyplay.Layer) error { return nil }
	switch b.Level {
	case wittyplay.LevelServer, wittyplay.LevelServerConsole:
		if !u.IsAdmin {
			writeErr(w, 403, "only an administrator can change what everyone starts with")
			return
		}
		key := gameDefaultsSetting("")
		if b.Level == wittyplay.LevelServerConsole {
			if console == "" {
				writeErr(w, 400, "which console?")
				return
			}
			key = gameDefaultsSetting(console)
		}
		read = func() (*wittyplay.Layer, error) {
			l := &wittyplay.Layer{}
			if raw := s.DB.GetSetting(ctx, key, ""); raw != "" {
				_ = json.Unmarshal([]byte(raw), l)
			}
			return l, nil
		}
		write = func(l *wittyplay.Layer) error {
			data, err := json.Marshal(l)
			if err != nil {
				return err
			}
			return s.DB.SetSetting(ctx, key, string(data))
		}
	case wittyplay.LevelPerson, wittyplay.LevelConsole, wittyplay.LevelGame:
		c, id := "", int64(0)
		switch b.Level {
		case wittyplay.LevelConsole:
			if console == "" {
				writeErr(w, 400, "which console?")
				return
			}
			c = console
		case wittyplay.LevelGame:
			if itemID == 0 {
				writeErr(w, 400, "which game?")
				return
			}
			id = itemID
		}
		read = func() (*wittyplay.Layer, error) { return s.Store.GameLayer(ctx, u.ID, c, id) }
		write = func(l *wittyplay.Layer) error { return s.Store.SetGameLayer(ctx, u.ID, c, id, l) }
	default:
		writeErr(w, 400, "unknown level")
		return
	}
	layer := &wittyplay.Layer{}
	ignored := []wittyplay.Issue{}
	switch {
	case b.Reset:
	case b.Document != nil:
		parsed, rep := wittyplay.ParseCustom([]byte(*b.Document), s.customInput(ctx, r, console, itemID))
		if !rep.OK() {
			writeJSON(w, 422, map[string]any{"error": "This settings file has mistakes, so nothing was changed.", "report": rep})
			return
		}
		layer, ignored = parsed, rep.Ignored
	default:
		cur, err := read()
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		layer = cur
		if ig := layer.Patch(b.Values, b.CoreOptions); ig != nil {
			ignored = ig
		}
	}
	if err := write(layer); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.applyToRunningGame(ctx, u.ID, console, itemID)
	view := s.settingsView(ctx, u.ID, console, itemID)
	view["ignored"] = ignored
	writeJSON(w, 200, view)
}

// applyToRunningGame tells this person's running game what's changed, so that a
// setting changed from a menu takes effect at once. A change for some other game, or
// for a console it isn't, isn't its concern.
func (s *Server) applyToRunningGame(ctx context.Context, userID int64, console string, itemID int64) {
	if s.gameSvc == nil {
		return
	}
	sess := s.gameSvc.mgr.ForUser(userID)
	if sess == nil || (itemID != 0 && sess.ItemID() != itemID) {
		return
	}
	it, err := s.Store.Item(ctx, sess.ItemID())
	if err != nil || (console != "" && it.Console != console) {
		return
	}
	res := wittyplay.Resolve(s.gameLayers(ctx, userID, it.Console, it.ID))
	if sess.Safe() {
		return
	}
	sess.SetAutoSave(res.Values["states.autoSave"] != false)
	if err := sess.Apply(wittyplay.ControlFor(res.Values, s.gameLimits(ctx)), res.CoreOptions); err != nil {
		s.Log.Debug("applying game settings", "err", err)
	}
}

// customInput gathers what's known about the core a settings file is for.
func (s *Server) customInput(ctx context.Context, r *http.Request, console string, itemID int64) wittyplay.CustomInput {
	if itemID == 0 && console == "" {
		return wittyplay.CustomInput{}
	}
	list, _ := s.coreOptionsFor(ctx, r, console, itemID)
	return wittyplay.CustomInput{Offered: list}
}

// coreOptionsFor finds what the core for a console offers: from the running game, then
// what was kept, then by asking the core. It returns which core that was, too.
func (s *Server) coreOptionsFor(ctx context.Context, r *http.Request, console string, itemID int64) ([]wittyplay.OptionInfo, string) {
	coreID, corePath := s.coreForConsole(ctx, console)
	if coreID == "" {
		return nil, ""
	}
	if sess := s.gameSvcSession(userFrom(r).ID, itemID); sess != nil {
		sess.WaitOptions(ctx)
		if opts := sess.CoreOptions(); len(opts) > 0 {
			out := make([]wittyplay.OptionInfo, 0, len(opts))
			for _, o := range opts {
				out = append(out, optionInfo(o))
			}
			return out, coreID
		}
	}
	if list, ok := s.Store.CoreOptionList(ctx, coreID); ok {
		return list, coreID
	}
	if corePath == "" || s.configureGames(ctx) != nil {
		return nil, coreID
	}
	g := s.games()
	opts, err := g.mgr.ProbeOptions(ctx, corePath, filepath.Join(g.dir, "system"), filepath.Join(g.dir, "tmp"))
	if err != nil {
		s.Log.Debug("asking a core for its options", "core", coreID, "err", err)
		return nil, coreID
	}
	if len(opts) == 0 {
		return nil, coreID // this core only says with a game loaded
	}
	s.keepCoreOptions(coreID, opts)
	out := make([]wittyplay.OptionInfo, 0, len(opts))
	for _, o := range opts {
		out = append(out, optionInfo(o))
	}
	return out, coreID
}

func (s *Server) gameSvcSession(userID, itemID int64) *stream.Session {
	if s.gameSvc == nil {
		return nil
	}
	sess := s.gameSvc.mgr.ForUser(userID)
	if sess == nil || (itemID != 0 && sess.ItemID() != itemID) {
		return nil
	}
	return sess
}

// coreForConsole is the core a console's games use here.
func (s *Server) coreForConsole(ctx context.Context, console string) (id, path string) {
	if console == wittyplay.TestConsole {
		return "test", ipc.TestCore
	}
	c := wittyplay.ByID(console)
	if c == nil {
		return "", ""
	}
	core := s.games().cores.ForConsole(c, s.preferredCores(ctx)[console])
	if core == nil {
		return "", ""
	}
	return core.ID, core.Path
}

func (s *Server) handleGameSettingsValidate(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Document string `json:"document"`
		Console  string `json:"console"`
		Item     int64  `json:"item"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, wittyplay.MaxCustomSize*2)
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "this can't be read")
		return
	}
	console, itemID, ok := s.gameScope(r, b.Console, b.Item)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	layer, rep := wittyplay.ParseCustom([]byte(b.Document), s.customInput(r.Context(), r, console, itemID))
	out := map[string]any{"ok": rep.OK(), "report": rep}
	if layer != nil {
		out["layer"] = layer
	}
	writeJSON(w, 200, out)
}

// handleGameCoreOptions lists what this game's core offers, what's chosen, and anything
// this person has set that it won't take.
func (s *Server) handleGameCoreOptions(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	ctx := r.Context()
	u := userFrom(r)
	list, core := s.coreOptionsFor(ctx, r, it.Console, it.ID)
	res := wittyplay.Resolve(s.gameLayers(ctx, u.ID, it.Console, it.ID))
	running := map[string]string{}
	if sess := s.gameSvcSession(u.ID, it.ID); sess != nil {
		for _, o := range sess.CoreOptions() {
			running[o.Key] = o.Value
		}
	}
	ignored := wittyplay.CheckOptions(res.CoreOptions, list)
	if list == nil {
		list = []wittyplay.OptionInfo{}
	}
	if ignored == nil {
		ignored = []wittyplay.Issue{}
	}
	writeJSON(w, 200, map[string]any{"core": core, "known": len(list) > 0, "options": list, "values": res.CoreOptions, "sources": res.CoreSources, "running": running, "ignored": ignored, "controls": res.Controls})
}

// ---- a person's list of games ----

func (s *Server) handleGamesRecent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := min(max(qInt(r, "limit", 24), 1), 200)
	rows, err := s.Store.GameList(ctx, userFrom(r).ID, r.URL.Query().Get("favourites") == "1", limit*2)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	type entry struct {
		Item       *library.Item `json:"item"`
		Favourite  bool          `json:"favourite"`
		Plays      int           `json:"plays"`
		LastPlayed int64         `json:"lastPlayed"`
	}
	out := []entry{}
	for _, e := range rows {
		if len(out) >= limit {
			break
		}
		it, err := s.visibleItem(r, e.ItemID)
		if err != nil || it.Kind != "game" {
			continue // no longer theirs to see
		}
		out = append(out, entry{it, e.Favourite, e.Plays, e.LastPlayed})
	}
	writeJSON(w, 200, out)
}

const maxGameNote = 8 << 10

func (s *Server) handleGameMeta(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	m, err := s.Store.GetGameMeta(r.Context(), userFrom(r).ID, it.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, statErr := os.Stat(filepath.Join(s.games().saveDir(userFrom(r).ID, it.ID), "save.srm"))
	m.HasBattery = statErr == nil
	writeJSON(w, 200, m)
}

func (s *Server) handleGameMetaPut(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	var b struct {
		Favourite *bool   `json:"favourite"`
		Note      *string `json:"note"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	id := userFrom(r).ID
	if b.Note != nil && len(*b.Note) > maxGameNote {
		writeErr(w, 400, "a note can be 8 KB at most")
		return
	}
	if b.Favourite != nil {
		if err := s.Store.SetGameFavourite(ctx, id, it.ID, *b.Favourite); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if b.Note != nil {
		if err := s.Store.SetGameNote(ctx, id, it.ID, *b.Note); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	m, err := s.Store.GetGameMeta(ctx, id, it.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, statErr := os.Stat(filepath.Join(s.games().saveDir(id, it.ID), "save.srm"))
	m.HasBattery = statErr == nil
	writeJSON(w, 200, m)
}

// ---- cheats ----

func (s *Server) handleGameCheats(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	m, err := s.Store.GetGameMeta(r.Context(), userFrom(r).ID, it.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, m.Cheats)
}

func (s *Server) handleGameCheatsPut(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	var b struct {
		Cheats []wittyplay.Cheat `json:"cheats"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, wittyplay.MaxCheatFile)
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "this list can't be read")
		return
	}
	clean, err := wittyplay.CleanCheats(b.Cheats)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	id := userFrom(r).ID
	if err := s.Store.SetGameCheats(r.Context(), id, it.ID, clean); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if sess := s.gameSvcSession(id, it.ID); sess != nil {
		_ = sess.SetCheats(wittyplay.CheatsForCore(clean))
	}
	writeJSON(w, 200, clean)
}

// handleGameCheatsImport reads a person's own cheat file and says what's in it. Nothing
// is kept until they save the list.
func (s *Server) handleGameCheatsImport(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.gameItem(r); !ok {
		writeErr(w, 404, "not found")
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, wittyplay.MaxCheatFile))
	if err != nil {
		writeErr(w, 413, "this file is too large to be a cheat list")
		return
	}
	list, skipped, err := wittyplay.ParseCheatFile(b)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"cheats": list, "skipped": skipped})
}

// ---- patches ----

const maxGamePatches = 16

var uploadNameRe = regexp.MustCompile(`[^\p{L}\p{N} ._()\[\]-]+`)

// cleanUploadName makes a person's file name safe to show. It's only ever a label: files
// are stored under numbers.
func cleanUploadName(name, fallback string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.TrimSpace(uploadNameRe.ReplaceAllString(name, ""))
	if len(name) > 120 {
		name = name[:120]
	}
	if name == "" || name == "." {
		return fallback
	}
	return name
}

func (g *gameService) patchDir(userID, itemID int64) string {
	return filepath.Join(g.saveDir(userID, itemID), "patches")
}

func patchFile(dir string, id int64) string {
	return filepath.Join(dir, "p"+strconv.FormatInt(id, 10)+".patch")
}

func (s *Server) handleGamePatches(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	list, err := s.Store.GamePatches(r.Context(), userFrom(r).ID, it.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleGamePatchAdd(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	ctx := r.Context()
	id := userFrom(r).ID
	existing, err := s.Store.GamePatches(ctx, id, it.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(existing) >= maxGamePatches {
		writeErr(w, 409, fmt.Sprintf("a game can have %d patches at most; remove one first", maxGamePatches))
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, patch.MaxPatch))
	if err != nil {
		writeErr(w, 413, fmt.Sprintf("this patch is too large: %d MB at most", patch.MaxPatch>>20))
		return
	}
	kind, err := patch.Detect(b)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	name := cleanUploadName(r.URL.Query().Get("name"), "patch."+kind)
	dir := s.games().patchDir(id, it.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	pid, err := s.Store.AddGamePatch(ctx, id, it.ID, library.GamePatch{Name: name, Kind: kind, Size: int64(len(b))})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := writeFileAtomic(patchFile(dir, pid), b); err != nil {
		_, _ = s.Store.RemoveGamePatch(ctx, id, it.ID, pid)
		writeErr(w, 500, err.Error())
		return
	}
	list, _ := s.Store.GamePatches(ctx, id, it.ID)
	writeJSON(w, 201, list)
}

func (s *Server) handleGamePatchPut(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	pid, ok := pathInt(r, "patch")
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	var b struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	found, err := s.Store.SetGamePatchEnabled(r.Context(), userFrom(r).ID, it.ID, pid, b.Enabled)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !found {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleGamePatchDelete(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	pid, ok := pathInt(r, "patch")
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	id := userFrom(r).ID
	found, err := s.Store.RemoveGamePatch(r.Context(), id, it.ID, pid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !found {
		writeErr(w, 404, "not found")
		return
	}
	_ = os.Remove(patchFile(s.games().patchDir(id, it.ID), pid))
	writeJSON(w, 200, map[string]any{"ok": true})
}

// patchROM makes a patched copy of a game for this session when its player has patches
// turned on. The original is never touched. A patch that won't apply leaves the game
// unpatched and playable, with a notice saying so.
func (g *gameService) patchROM(ctx context.Context, store *library.Store, userID, itemID int64, rom string, cleanup func()) (string, func(), []string) {
	list, err := store.GamePatches(ctx, userID, itemID)
	if err != nil {
		return rom, cleanup, nil
	}
	var on []library.GamePatch
	for _, p := range list {
		if p.Enabled {
			on = append(on, p)
		}
	}
	if len(on) == 0 {
		return rom, cleanup, nil
	}
	ext := strings.ToLower(filepath.Ext(rom))
	switch ext {
	case ".cue", ".m3u", ".gdi", ".chd", ".pbp", ".iso":
		return rom, cleanup, []string{"Patches only work on games that are a single file, so this one started unpatched."}
	}
	st, err := os.Stat(rom)
	if err != nil || st.Size() > patch.MaxOutput {
		return rom, cleanup, []string{"This game is too large to patch, so it started unpatched."}
	}
	data, err := os.ReadFile(rom)
	if err != nil {
		return rom, cleanup, []string{"This game couldn't be read to patch it, so it started unpatched."}
	}
	dir := g.patchDir(userID, itemID)
	var notices []string
	applied := 0
	for _, p := range on {
		b, err := os.ReadFile(patchFile(dir, p.ID))
		if err == nil {
			var out []byte
			if out, err = patch.Apply(data, b); err == nil {
				data = out
				applied++
				continue
			}
		}
		notices = append(notices, fmt.Sprintf("The patch %s wasn’t applied: %v.", p.Name, err))
	}
	if applied == 0 {
		return rom, cleanup, notices
	}
	tmpRoot := filepath.Join(g.dir, "tmp")
	if err := os.MkdirAll(tmpRoot, 0o700); err != nil {
		return rom, cleanup, append(notices, "There was nowhere to put the patched game, so it started unpatched.")
	}
	tmp, err := os.MkdirTemp(tmpRoot, "rom-")
	if err != nil {
		return rom, cleanup, append(notices, "There was nowhere to put the patched game, so it started unpatched.")
	}
	out := filepath.Join(tmp, "game"+ext)
	if err := os.WriteFile(out, data, 0o600); err != nil {
		_ = os.RemoveAll(tmp)
		return rom, cleanup, append(notices, "The patched game couldn't be written, so it started unpatched.")
	}
	next := func() {
		_ = os.RemoveAll(tmp)
		if cleanup != nil {
			cleanup()
		}
	}
	return out, next, notices
}

func writeFileAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".up-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ---- images around the picture ----

// A person's image is a hostile file until it's been decoded and written out again.
// Only the decoded pixels are kept, as a PNG this server made, under a number. An SVG,
// or anything else with instructions in it, never gets past the decoder.
const (
	maxBezelUpload = 16 << 20
	maxBezelPixels = 16 << 20
	maxBezelSide   = 8192
	maxBezels      = 16
)

// Decoding one at a time keeps a burst of big images from using up a small server.
var bezelDecode sync.Mutex

func (g *gameService) bezelFile(userID, id int64) string {
	return filepath.Join(g.dir, "bezels", "u"+strconv.FormatInt(userID, 10), "b"+strconv.FormatInt(id, 10)+".png")
}

func (s *Server) handleBezels(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.GameBezels(r.Context(), userFrom(r).ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

// reencodeImage decodes what a person sent and returns a clean PNG of it.
func reencodeImage(b []byte) (out []byte, width, height int, err error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, 0, 0, errors.New("use a PNG, JPEG or WebP image")
	}
	switch format {
	case "png", "jpeg", "webp":
	default:
		return nil, 0, 0, errors.New("use a PNG, JPEG or WebP image")
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxBezelSide || cfg.Height > maxBezelSide || cfg.Width*cfg.Height > maxBezelPixels {
		return nil, 0, 0, fmt.Errorf("this image is too large: %d megapixels at most", maxBezelPixels>>20)
	}
	bezelDecode.Lock()
	defer bezelDecode.Unlock()
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, 0, 0, errors.New("this image is damaged")
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), cfg.Width, cfg.Height, nil
}

func (s *Server) handleBezelAdd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := userFrom(r).ID
	existing, err := s.Store.GameBezels(ctx, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(existing) >= maxBezels {
		writeErr(w, 409, fmt.Sprintf("you can keep %d images at most; remove one first", maxBezels))
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBezelUpload))
	if err != nil {
		writeErr(w, 413, fmt.Sprintf("this image is too large: %d MB at most", maxBezelUpload>>20))
		return
	}
	clean, width, height, err := reencodeImage(b)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	bid, err := s.Store.AddGameBezel(ctx, id, library.GameBezel{Name: cleanUploadName(r.URL.Query().Get("name"), "Image"), Width: width, Height: height, Size: int64(len(clean))})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	path := s.games().bezelFile(id, bid)
	err = os.MkdirAll(filepath.Dir(path), 0o700)
	if err == nil {
		err = writeFileAtomic(path, clean)
	}
	if err != nil {
		_, _ = s.Store.RemoveGameBezel(ctx, id, bid)
		writeErr(w, 500, err.Error())
		return
	}
	list, _ := s.Store.GameBezels(ctx, id)
	writeJSON(w, 201, map[string]any{"id": bid, "bezels": list})
}

func (s *Server) handleBezelImage(w http.ResponseWriter, r *http.Request) {
	bid, ok := pathInt(r, "bezel")
	id := userFrom(r).ID
	if !ok || !s.Store.HasGameBezel(r.Context(), id, bid) {
		writeErr(w, 404, "not found")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, s.games().bezelFile(id, bid))
}

func (s *Server) handleBezelDelete(w http.ResponseWriter, r *http.Request) {
	bid, ok := pathInt(r, "bezel")
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	id := userFrom(r).ID
	found, err := s.Store.RemoveGameBezel(r.Context(), id, bid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !found {
		writeErr(w, 404, "not found")
		return
	}
	_ = os.Remove(s.games().bezelFile(id, bid))
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---- saves in and out ----

// A state file on its way out carries a header saying where it came from, so an import
// can check it arrived whole. Plain state files from elsewhere are accepted too: a
// state is only ever read by a core in its sandbox.
var stateFileMagic = []byte("WPSTATE1")

// Limits on imports.
const (
	maxImportState = 256 << 20
	maxImportSRM   = 64 << 20
	maxStateHeader = 4 << 10
)

type stateHeader struct {
	Console string `json:"console"`
	Core    string `json:"core,omitempty"`
	Title   string `json:"title,omitempty"`
	At      int64  `json:"at"`
	Size    int64  `json:"size"`
	CRC32   uint32 `json:"crc32"`
}

func slotCore(dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil || len(b) > 4096 {
		return ""
	}
	var m stream.SlotMeta
	if json.Unmarshal(b, &m) != nil {
		return ""
	}
	return m.Core
}

// parseStateFile reads an exported state, checking everything it says. A file without
// the header is taken as a plain state, which no header could vouch for anyway.
func parseStateFile(b []byte) (body []byte, hdr *stateHeader, err error) {
	if !bytes.HasPrefix(b, stateFileMagic) {
		return b, nil, nil
	}
	rest := b[len(stateFileMagic):]
	if len(rest) < 4 {
		return nil, nil, errors.New("this state file is cut short")
	}
	n := binary.LittleEndian.Uint32(rest)
	if n > maxStateHeader || int(n) > len(rest)-4 {
		return nil, nil, errors.New("this state file's header is damaged")
	}
	var h stateHeader
	if err := json.Unmarshal(rest[4:4+n], &h); err != nil {
		return nil, nil, errors.New("this state file's header is damaged")
	}
	body = rest[4+n:]
	if int64(len(body)) != h.Size {
		return nil, nil, errors.New("this state file is cut short")
	}
	if crc32.ChecksumIEEE(body) != h.CRC32 {
		return nil, nil, errors.New("this state file is damaged: its checksum doesn't match")
	}
	return body, &h, nil
}

func (s *Server) handleSaveExport(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	slot, err := strconv.Atoi(r.PathValue("slot"))
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	name, err := stream.SlotName(slot)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	dir := s.games().saveDir(userFrom(r).ID, it.ID)
	path := filepath.Join(dir, name+".state")
	st, err := os.Stat(path)
	if err != nil || st.Size() > ipc.MaxBlob {
		writeErr(w, 404, "not found")
		return
	}
	body, err := os.ReadFile(path)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	h, _ := json.Marshal(stateHeader{Console: it.Console, Core: slotCore(dir, name), Title: it.Title, At: st.ModTime().Unix(), Size: int64(len(body)), CRC32: crc32.ChecksumIEEE(body)})
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.wpstate"`, attachmentName(it.Title, name)))
	var head bytes.Buffer
	head.Write(stateFileMagic)
	_ = binary.Write(&head, binary.LittleEndian, uint32(len(h)))
	head.Write(h)
	_, _ = w.Write(head.Bytes())
	_, _ = w.Write(body)
}

var attachmentRe = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)

func attachmentName(title, part string) string {
	t := strings.TrimSpace(attachmentRe.ReplaceAllString(title, ""))
	if len(t) > 80 {
		t = t[:80]
	}
	if t == "" {
		t = "game"
	}
	return t + " " + part
}

// saveRoom says whether a person has space for a file in a game's folder.
func saveRoom(dir string, size int64) bool {
	var n int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !info.IsDir() {
			n += info.Size()
		}
	}
	return n+size <= stream.MaxSaveBytes
}

func (s *Server) handleSaveImport(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	slot, err := strconv.Atoi(r.PathValue("slot"))
	if err != nil || slot < 1 || slot > stream.MaxSlot {
		writeErr(w, 400, fmt.Sprintf("a state can be imported into slots 1 to %d", stream.MaxSlot))
		return
	}
	name, _ := stream.SlotName(slot)
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImportState))
	if err != nil {
		writeErr(w, 413, fmt.Sprintf("this file is too large: %d MB at most", maxImportState>>20))
		return
	}
	body, hdr, err := parseStateFile(b)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	if len(body) == 0 {
		writeErr(w, 422, "this file is empty")
		return
	}
	warning := ""
	if hdr == nil {
		warning = "This file has no header to check. If it came from a different core or game, it won’t load."
	} else if hdr.Console != it.Console {
		writeErr(w, 422, "this state is from another console's game")
		return
	} else if hdr.Title != it.Title {
		warning = "This state was saved from " + truncateText(hdr.Title, 80) + ". If that’s a different game, it won’t load."
	}
	dir := s.games().saveDir(userFrom(r).ID, it.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !saveRoom(dir, int64(len(body))) {
		writeErr(w, 409, stream.ErrSaveFull.Error())
		return
	}
	if err := writeFileAtomic(filepath.Join(dir, name+".state"), body); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = os.Remove(filepath.Join(dir, name+".png")) // the old picture isn't of this
	core := ""
	if hdr != nil {
		core = hdr.Core
	}
	meta, _ := json.Marshal(stream.SlotMeta{Core: core})
	_ = writeFileAtomic(filepath.Join(dir, name+".json"), meta)
	writeJSON(w, 200, map[string]any{"ok": true, "warning": warning})
}

func truncateText(v string, n int) string {
	if len(v) > n {
		return v[:n]
	}
	return v
}

func (s *Server) handleBatteryExport(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	path := filepath.Join(s.games().saveDir(userFrom(r).ID, it.ID), "save.srm")
	if _, err := os.Stat(path); err != nil {
		writeErr(w, 404, "this game has no battery save yet")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.srm"`, attachmentName(it.Title, "save")))
	http.ServeFile(w, r, path)
}

// handleBatteryImport replaces a game's battery save. It's refused while that person is
// playing it, because the running game would write over it anyway. The old one is kept
// beside it.
func (s *Server) handleBatteryImport(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	id := userFrom(r).ID
	if s.gameSvcSession(id, it.ID) != nil {
		writeErr(w, 409, "leave the game first: it would write over this")
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImportSRM))
	if err != nil {
		writeErr(w, 413, fmt.Sprintf("this file is too large: %d MB at most", maxImportSRM>>20))
		return
	}
	if len(b) == 0 {
		writeErr(w, 422, "this file is empty")
		return
	}
	dir := s.games().saveDir(id, it.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !saveRoom(dir, int64(len(b))) {
		writeErr(w, 409, stream.ErrSaveFull.Error())
		return
	}
	path := filepath.Join(dir, "save.srm")
	if old, err := os.ReadFile(path); err == nil {
		_ = writeFileAtomic(filepath.Join(dir, "save.srm.bak"), old)
	}
	if err := writeFileAtomic(path, b); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---- from a running game ----

func (s *Server) handlePlayStats(w http.ResponseWriter, r *http.Request) {
	sess := s.session(r)
	if sess == nil {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, sess.Stats())
}

func (s *Server) handlePlayScreenshot(w http.ResponseWriter, r *http.Request) {
	sess := s.session(r)
	if sess == nil {
		writeErr(w, 404, "not found")
		return
	}
	b, err := sess.Screenshot()
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.png"`, attachmentName(sess.Info().Title, "screenshot")))
	_, _ = w.Write(b)
}
