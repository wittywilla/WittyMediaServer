package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/gamearchive"
	"github.com/wittywilla/wittymediaserver/internal/gameid"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittyplay"
)

// Everything about putting a games library right: the list of files a scan wasn't sure
// about, how an import is going, and fixing one game. Files aren't touched by any of it:
// each choice is kept beside the library, and the next scan goes by it. See docs/GAMING.md.

func (s *Server) gameReviewRoutes(m *http.ServeMux) {
	m.Handle("GET /api/libraries/{id}/games/review", s.requireAdmin(s.handleGameReview))
	m.Handle("POST /api/libraries/{id}/games/review/apply", s.requireAdmin(s.handleGameReviewApply))
	m.Handle("GET /api/libraries/{id}/games/import", s.requireAdmin(s.handleGameImport))
	m.Handle("GET /api/games/{id}/versions", s.requireUser(s.handleGameVersions))
	m.Handle("PUT /api/games/{id}/version", s.requireAdmin(s.handleGameVersionPick))
	m.Handle("POST /api/games/{id}/split", s.requireAdmin(s.handleGameSplit))
	m.Handle("PUT /api/games/{id}/console", s.requireAdmin(s.handleGameConsoleFix))
	m.Handle("POST /api/games/{id}/not-a-game", s.requireAdmin(s.handleGameNotAGame))
}

func (s *Server) gamesLibrary(w http.ResponseWriter, r *http.Request) *library.Library {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil
	}
	lib, err := s.Store.Library(r.Context(), id)
	if err != nil || lib.Type != "games" {
		writeErr(w, 404, "library not found")
		return nil
	}
	return lib
}

// GET /api/libraries/{id}/games/review?kind=needs|blocked|left|ignored&limit=&offset=
func (s *Server) handleGameReview(w http.ResponseWriter, r *http.Request) {
	lib := s.gamesLibrary(w, r)
	if lib == nil {
		return
	}
	kind := r.URL.Query().Get("kind")
	switch kind {
	case "", "needs", "blocked", "left", "ignored", "duplicate":
	default:
		writeErr(w, 400, "no such list")
		return
	}
	out, err := s.Store.GameReviewList(r.Context(), lib, kind, qInt(r, "limit", 200), qInt(r, "offset", 0))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

// POST /api/libraries/{id}/games/review/apply {ids: [...], action: "add"|"ignore"|"forget", console}
//
// add says these are games for console, ignore says they aren't games, and forget takes back
// whatever was said. A scan follows, and it only has to read what changed.
func (s *Server) handleGameReviewApply(w http.ResponseWriter, r *http.Request) {
	lib := s.gamesLibrary(w, r)
	if lib == nil {
		return
	}
	var b struct {
		IDs     []int64 `json:"ids"`
		Action  string  `json:"action"`
		Console string  `json:"console"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(b.IDs) == 0 || len(b.IDs) > 5000 {
		writeErr(w, 400, "say which files, up to 5,000 at a time")
		return
	}
	switch b.Action {
	case "add":
		if c := wittyplay.ByID(b.Console); c == nil || (c.Hidden && !s.testingGames()) {
			writeErr(w, 400, "no such console")
			return
		}
	case "ignore", "forget":
		b.Console = ""
	default:
		writeErr(w, 400, "the action is add, ignore or forget")
		return
	}
	done := 0
	for _, id := range b.IDs {
		if _, err := s.Store.DecideGameFile(r.Context(), lib, id, b.Action, b.Console); err != nil {
			if errors.Is(err, library.ErrNotFound) {
				continue
			}
			writeErr(w, 500, err.Error())
			return
		}
		done++
	}
	if done > 0 {
		s.Scanner.Start(lib.ID)
	}
	writeJSON(w, 200, map[string]any{"applied": done})
}

// GET /api/libraries/{id}/games/import: how far a library's import has got, in one answer.
func (s *Server) handleGameImport(w http.ResponseWriter, r *http.Request) {
	lib := s.gamesLibrary(w, r)
	if lib == nil {
		return
	}
	review, err := s.Store.GameReviewList(r.Context(), lib, "none", 1, 0)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	counts, err := s.Store.GameCounts(r.Context(), lib.ID, nil)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"scanning": s.Scanner.Snapshot(lib.ID) != nil,
		"scan":     s.Scanner.Snapshot(lib.ID),
		"lookup":   s.GameMeta.Status(r.Context()),
		"review":   review.Counts,
		"games":    counts,
	})
}

// gameVersion is one way to play a game, for choosing between them.
type gameVersion struct {
	// ID is the version's own item, when it's a file of its own.
	ID int64 `json:"id,omitempty"`
	// Entry is its file in the game's archive, when it's one of the versions in there.
	Entry string `json:"entry,omitempty"`
	Label string `json:"label"`
	// Current: what Play starts. Altered: a bad dump, a hack or a translation.
	Current bool `json:"current,omitempty"`
	Altered bool `json:"altered,omitempty"`
}

// GET /api/games/{id}/versions: its other versions, in files of their own and inside its
// archive, and its discs.
func (s *Server) handleGameVersions(w http.ResponseWriter, r *http.Request) {
	it := s.visibleGame(w, r)
	if it == nil {
		return
	}
	ctx := r.Context()
	out := map[string]any{}
	head := it.ID
	if l := s.Store.LinkOf(ctx, it.ID); l != nil && l.Link == library.LinkVersion && l.ParentItem != nil {
		head = *l.ParentItem
	}
	var versions []gameVersion
	if head != it.ID {
		if parent, err := s.visibleItem(r, head); err == nil {
			versions = append(versions, gameVersion{ID: parent.ID, Label: parent.Tagline})
		}
	} else {
		versions = append(versions, gameVersion{ID: it.ID, Label: it.Tagline, Current: true})
	}
	kids, links, err := s.Store.AttachedTo(ctx, library.TargetItem, head, library.LinkVersion)
	if err == nil {
		for i, v := range kids {
			if !s.access(r).AllowsItem(v, nil) {
				continue
			}
			label := links[i].Label
			if label == "" {
				label = v.Tagline
			}
			versions = append(versions, gameVersion{ID: v.ID, Label: label, Current: v.ID == it.ID})
		}
	}
	out["versions"] = versions
	if discs, err := s.Store.GameDiscs(ctx, it.ID); err == nil && len(discs) > 0 {
		out["discs"] = discs
	}
	// What's inside a set, which is only a choice if there's a choice to make. Only an
	// administrator can make it, and answering means opening the archive in a worker.
	if userFrom(r).IsAdmin && gamearchive.IsArchive(it.Path) {
		if inner := s.archiveVersions(ctx, it); len(inner) > 1 {
			out["inside"] = inner
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) archiveVersions(ctx context.Context, it *library.Item) []gameVersion {
	console := wittyplay.ByID(it.Console)
	if console == nil {
		return nil
	}
	l, err := gamearchive.List(it.Path)
	if err != nil {
		return nil
	}
	d, err := s.Store.GameDetails(ctx, it.ID)
	if err != nil {
		return nil
	}
	current := d.Entry
	if current == "" {
		if plan, err := gamearchive.Choose(l, console, filepath.Base(it.Path)); err == nil && len(plan.Want) > 0 {
			current = plan.Want[0].Name
		}
	}
	var out []gameVersion
	for _, e := range l.Entries {
		x := strings.ToLower(path.Ext(e.Name))
		if !console.Accepts(x) {
			continue
		}
		switch x {
		case ".cue", ".gdi", ".m3u", ".ccd", ".toc":
			return nil // a disc, with its tracks: nothing to choose between
		}
		base := path.Base(e.Name)
		q := gameid.QualityOf(base)
		out = append(out, gameVersion{Entry: e.Name, Label: strings.TrimSuffix(base, path.Ext(base)), Current: e.Name == current, Altered: q >= gameid.QualityTranslated})
	}
	return out
}

// PUT /api/games/{id}/version {entry}: which file in an archive to play. Empty goes back
// to choosing by itself.
func (s *Server) handleGameVersionPick(w http.ResponseWriter, r *http.Request) {
	it := s.visibleGame(w, r)
	if it == nil {
		return
	}
	var b struct {
		Entry string `json:"entry"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.Entry != "" {
		// Only something this archive holds, and that this console can run.
		ok := false
		for _, v := range s.archiveVersions(r.Context(), it) {
			ok = ok || v.Entry == b.Entry
		}
		if !ok {
			writeErr(w, 400, "this game's archive has no such file")
			return
		}
	}
	if err := s.Store.PinGameEntry(r.Context(), it, b.Entry); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.GameMeta.Kick()
	writeJSON(w, 200, map[string]any{"entry": b.Entry})
}

// POST /api/games/{id}/split: a version becomes a game of its own, and stays one.
func (s *Server) handleGameSplit(w http.ResponseWriter, r *http.Request) {
	it := s.visibleGame(w, r)
	if it == nil {
		return
	}
	lib, err := s.Store.Library(r.Context(), it.LibraryID)
	if err != nil {
		writeErr(w, 404, "library not found")
		return
	}
	if err := s.Store.SplitGameVersion(r.Context(), lib, it); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// PUT /api/games/{id}/console {console}: this file is really for another console.
func (s *Server) handleGameConsoleFix(w http.ResponseWriter, r *http.Request) {
	it := s.visibleGame(w, r)
	if it == nil {
		return
	}
	var b struct {
		Console string `json:"console"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if c := wittyplay.ByID(b.Console); c == nil || (c.Hidden && !s.testingGames()) {
		writeErr(w, 400, "no such console")
		return
	}
	if err := s.Store.DecideGamePath(r.Context(), it, "add", b.Console); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Scanner.Start(it.LibraryID)
	writeJSON(w, 200, map[string]any{"console": b.Console})
}

// POST /api/games/{id}/not-a-game: this isn't a game. Its file stays where it is, and it
// goes to the list of what's been left out, from where it can be brought back.
func (s *Server) handleGameNotAGame(w http.ResponseWriter, r *http.Request) {
	it := s.visibleGame(w, r)
	if it == nil {
		return
	}
	if err := s.Store.DecideGamePath(r.Context(), it, "ignore", ""); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Scanner.Start(it.LibraryID)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// prepareGame gives the file a core should open: one disc or cartridge, or a list of a
// game's discs. Those can be files, archives, or some of each.
func (g *gameService) prepareGame(ctx context.Context, store *library.Store, it *library.Item, console *wittyplay.Console) (string, func(), error) {
	discs, err := store.GameDiscs(ctx, it.ID)
	if err != nil || len(discs) < 2 || !console.Accepts(".m3u") {
		return g.prepareROM(ctx, it.Path, console, store.GameEntry(ctx, it.ID))
	}
	return g.prepareDiscs(ctx, discs, console)
}

// prepareDiscs makes a folder holding a playlist of a game's discs, and the discs or
// links to them. A core reads a playlist's names relative to itself, and can't follow one
// across drives, so what's beside it has to be real, or as near as the system allows.
func (g *gameService) prepareDiscs(ctx context.Context, discs []library.GameDisc, console *wittyplay.Console) (rom string, cleanup func(), err error) {
	var ends []func()
	finish := func() {
		for _, end := range ends {
			end()
		}
	}
	defer func() {
		if err != nil {
			finish()
		}
	}()
	tmpRoot := filepath.Join(g.dir, "tmp")
	if err = os.MkdirAll(tmpRoot, 0o700); err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp(tmpRoot, "rom-")
	if err != nil {
		return "", nil, err
	}
	ends = append(ends, func() { _ = os.RemoveAll(dir) })
	var list strings.Builder
	for _, d := range discs {
		one, release, err := g.prepareROM(ctx, d.Path, console, "")
		if err != nil {
			return "", nil, err
		}
		if release != nil {
			ends = append(ends, release)
		}
		sub := "disc" + strconv.Itoa(d.Disc)
		if err := os.Mkdir(filepath.Join(dir, sub), 0o700); err != nil {
			return "", nil, err
		}
		files := []string{one}
		switch strings.ToLower(filepath.Ext(one)) {
		case ".cue", ".gdi", ".ccd", ".toc":
			if sheet, err := gameid.ReadSheet(one); err == nil {
				files = append(files, sheet.Files...)
			}
		}
		for _, f := range files {
			if err := linkOrCopy(f, filepath.Join(dir, sub, filepath.Base(f))); err != nil {
				return "", nil, err
			}
		}
		list.WriteString(sub + "/" + filepath.Base(one) + "\n")
	}
	m3u := filepath.Join(dir, "game.m3u")
	if err := os.WriteFile(m3u, []byte(list.String()), 0o600); err != nil {
		return "", nil, err
	}
	var once bool
	return m3u, func() {
		if !once {
			once = true
			finish()
		}
	}, nil
}

// linkOrCopy puts a file in a second place as cheaply as this system allows. A hard link
// works on the same drive, and a symbolic link where the account may make them. A copy is
// what's left: it costs the time and the room, for as long as the game is running.
func linkOrCopy(from, to string) error {
	if _, err := os.Lstat(to); err == nil {
		return nil // a track two sheets name
	}
	if os.Link(from, to) == nil {
		return nil
	}
	if os.Symlink(from, to) == nil {
		return nil
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
