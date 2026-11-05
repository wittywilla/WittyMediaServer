package server

import (
	"context"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
	"github.com/wittywilla/wittymediaserver/internal/play/stream"
)

// The games home screen in one answer, so a TV app and the web page show the same thing
// from one request. Everything goes through the same access rules as any other list.

const (
	homeShelfGames = 24 // games on a shelf
	homeShelves    = 6  // shelves of each kind, genre and series
	homeHero       = 15 // covers in the hero
)

type gameHomeSave struct {
	Slot      int   `json:"slot"`
	UpdatedAt int64 `json:"updatedAt"`
	HasThumb  bool  `json:"hasThumb"`
}

type gameHomeContinue struct {
	Item       *library.Item `json:"item"`
	Plays      int           `json:"plays"`
	LastPlayed int64         `json:"lastPlayed"`
	Save       *gameHomeSave `json:"save,omitempty"` // where it would resume from
}

type gameHomeShelf struct {
	Kind  string          `json:"kind"` // genre, or series
	Name  string          `json:"name"`
	Count int             `json:"count"`
	Items []*library.Item `json:"items"`
}

// GET /api/games/home?library=
func (s *Server) handleGamesHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lib := int64(qInt(r, "library", 0))
	if lib == 0 {
		writeErr(w, 400, "which library?")
		return
	}
	u := userFrom(r)
	access := s.access(r)
	list := func(q library.ItemQuery) []*library.Item {
		q.LibraryID, q.Kind, q.Access, q.Brief, q.NoTotal = lib, "game", access, true, true
		items, _, err := s.Store.Items(ctx, q)
		if err != nil {
			s.Log.WarnContext(ctx, "games home: a list failed", "cat", "games", "err", err)
			return []*library.Item{}
		}
		return items
	}
	// Someone's own lists, in their own order, with what they can no longer see left out.
	mine := func(entries []library.GameListEntry) []*library.Item {
		ids := make([]int64, 0, len(entries))
		for _, e := range entries {
			ids = append(ids, e.ItemID)
		}
		found := map[int64]*library.Item{}
		if len(ids) > 0 {
			for _, it := range list(library.ItemQuery{IDs: ids, Limit: len(ids)}) {
				found[it.ID] = it
			}
		}
		out := make([]*library.Item, 0, len(ids))
		for _, id := range ids {
			if it := found[id]; it != nil {
				out = append(out, it)
			}
		}
		return out
	}

	played, err := s.Store.GameList(ctx, u.ID, false, 60)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	favs, err := s.Store.GameList(ctx, u.ID, true, 60)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	most, err := s.Store.GameMostPlayed(ctx, u.ID, 60)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	playedItems := mine(played)
	entryOf := map[int64]library.GameListEntry{}
	for _, e := range played {
		entryOf[e.ItemID] = e
	}
	saves := s.games().saveDir
	cont := []gameHomeContinue{}
	for _, it := range playedItems {
		if len(cont) >= 12 {
			break
		}
		e := entryOf[it.ID]
		cont = append(cont, gameHomeContinue{Item: it, Plays: e.Plays, LastPlayed: e.LastPlayed, Save: resumeSave(saves(u.ID, it.ID))})
	}

	favItems := capItems(mine(favs), homeShelfGames)
	mostItems := capItems(mine(most), homeShelfGames)
	recent := list(library.ItemQuery{Sort: "added", Desc: true, Limit: homeShelfGames})
	rated := list(library.ItemQuery{Sort: "rating", Desc: true, MinRating: 0.1, Limit: 60})

	shelves := []gameHomeShelf{}
	if facets, err := s.Store.GameFacets(ctx, lib, "", access); err == nil {
		for i, g := range facets.Genres {
			if i >= homeShelves || g.Count < 4 {
				break
			}
			shelves = append(shelves, gameHomeShelf{Kind: "genre", Name: g.Value, Count: g.Count, Items: list(library.ItemQuery{Genre: g.Value, Sort: "popular", Limit: homeShelfGames})})
		}
		for i, f := range facets.Series {
			if i >= homeShelves || f.Count < 2 {
				break
			}
			shelves = append(shelves, gameHomeShelf{Kind: "series", Name: f.Value, Count: f.Count, Items: list(library.ItemQuery{Series: f.Value, Sort: "oldest", Limit: homeShelfGames})})
		}
	}

	hero := pickHero(cont, favItems, rated, recent)

	// Everything shown here, once, for what's been played and how far.
	all := append(append(append(append([]*library.Item{}, hero...), playedItems...), favItems...), append(mostItems, recent...)...)
	for _, sh := range shelves {
		all = append(all, sh.Items...)
	}
	_ = s.Store.AttachProgress(ctx, u.ID, all)
	s.forViewer(r, all...)

	favIDs := make([]int64, 0, len(favs))
	for _, it := range mine(favs) {
		favIDs = append(favIDs, it.ID)
	}
	writeJSON(w, 200, map[string]any{
		"hero": hero, "continue": cont, "recent": recent, "favourites": favItems, "favouriteIds": favIDs, "mostPlayed": mostItems, "shelves": shelves,
	})
}

func capItems(items []*library.Item, n int) []*library.Item {
	if len(items) > n {
		return items[:n]
	}
	return items
}

// resumeSave looks for the automatic save among a game's files.
func resumeSave(dir string) *gameHomeSave {
	name, err := stream.SlotName(0)
	if err != nil {
		return nil
	}
	st, err := os.Stat(filepath.Join(dir, name+".state"))
	if err != nil {
		return nil
	}
	_, thumb := os.Stat(filepath.Join(dir, name+".png"))
	return &gameHomeSave{Slot: 0, UpdatedAt: st.ModTime().Unix(), HasThumb: thumb == nil}
}

// pickHero chooses the covers at the top of the page: what someone's playing, what they
// love, then the best of the rest, which turns over each day so it doesn't go stale.
func pickHero(cont []gameHomeContinue, favs, rated, recent []*library.Item) []*library.Item {
	out := make([]*library.Item, 0, homeHero)
	seen := map[int64]bool{}
	add := func(it *library.Item, limit int) {
		if it.Poster != "" && !seen[it.ID] && len(out) < limit {
			seen[it.ID] = true
			out = append(out, it)
		}
	}
	for _, c := range cont {
		add(c.Item, 3)
	}
	for _, it := range favs {
		add(it, 6)
	}
	shuffled := append([]*library.Item{}, rated...)
	day := time.Now().Unix() / 86400
	rand.New(rand.NewSource(day)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	for _, it := range shuffled {
		add(it, homeHero-2)
	}
	for _, it := range recent {
		add(it, homeHero)
	}
	return out
}

type gameCheckBIOS struct {
	File     string `json:"file"`
	For      string `json:"for"`
	Required bool   `json:"required"`
	Found    bool   `json:"found"`
}

// GET /api/games/{id}/check says what a game needs and whether this server has it, in
// time for the page to say so rather than the Play button finding out.
func (s *Server) handleGameCheck(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	console := wittyplay.ByID(it.Console)
	if console == nil {
		writeJSON(w, 200, map[string]any{"ready": false, "reason": "unknown", "bios": []gameCheckBIOS{}})
		return
	}
	writeJSON(w, 200, s.gameCheck(r.Context(), it, console))
}

func (s *Server) gameCheck(ctx context.Context, it *library.Item, console *wittyplay.Console) map[string]any {
	g := s.games()
	systemDir := g.systemDir()
	// As pressing Play does: files left in a BIOS folder beside the games count.
	if len(console.BIOS) > 0 {
		for _, want := range console.Cores {
			_ = wittyplay.CollectBIOS(console, want.ID, it.Path, systemDir)
		}
	}
	bios := make([]gameCheckBIOS, 0, len(console.BIOS))
	for _, b := range console.BIOS {
		_, err := os.Stat(filepath.Join(systemDir, filepath.FromSlash(b.File)))
		bios = append(bios, gameCheckBIOS{File: b.File, For: b.For, Required: b.Required, Found: err == nil})
	}
	out := map[string]any{"console": console.ID, "consoleName": console.Name, "bios": bios, "biosFolder": wittyplay.BIOSFolderName}
	if console.ID == wittyplay.TestConsole {
		out["ready"] = true
		return out
	}
	if console.Unavailable != "" && !s.customFor(ctx, console.ID) {
		out["ready"], out["reason"], out["message"] = false, "unavailable", console.Unavailable
		return out
	}
	pick := s.pickCore(ctx, console, it.ID, systemDir)
	switch {
	case pick.core != nil:
		out["ready"], out["core"] = true, map[string]any{"id": pick.core.ID, "name": pick.core.Name, "license": pick.core.License}
		out["needsGpu"] = !pick.software && s.coreDrawsOnGPU(*pick.core)
		if pick.software {
			// It plays, drawn by the processor: how it may run, before anyone presses Play.
			out["software"], out["notice"] = true, softwareNotice
		}
	case pick.noGPU != "":
		// Not on this server, and why: the Play button would only be turned away.
		out["ready"], out["reason"], out["message"] = false, "gpu", gpuMessage(console.Name, pick.noGPU)
		out["needsGpu"], out["gpuMissing"] = true, pick.noGPU
	case pick.blocked != nil:
		out["ready"], out["reason"], out["message"] = false, "bios", wittyplay.BIOSHelp(pick.blocked.ID, console.Name)
		out["file"] = wittyplay.MissingBIOS(pick.blocked.ID, systemDir)
	default:
		out["ready"], out["reason"], out["message"] = false, "core", "No core is installed for "+console.Name+"."
	}
	return out
}

// gameCountsEverywhere adds up each console's games across every library.
func (s *Server) gameCountsEverywhere(ctx context.Context) map[string]int {
	out := map[string]int{}
	libs, err := s.Store.Libraries(ctx)
	if err != nil {
		return out
	}
	for _, l := range libs {
		if l.Type != "games" {
			continue
		}
		counts, err := s.Store.GameCounts(ctx, l.ID, nil)
		if err != nil {
			continue
		}
		for id, n := range counts {
			out[id] += n
		}
	}
	return out
}
