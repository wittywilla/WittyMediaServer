package library

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"
)

// Game rigs: a person's own gaming PC, whose games are listed as a games library. See
// docs/GAME-RIGS.md. A rig's games are ordinary items, which is what makes every access
// rule apply to them: library limits, age limits and per-title choices, in every list.
// Their paths are made up and name nothing on this machine.

// RigConsole marks a games library, and its items, as belonging to a gaming PC.
const RigConsole = "rig"

// IsRig reports whether a library belongs to a gaming PC, so that nothing scans it.
func (l *Library) IsRig() bool { return l.Type == "games" && l.Console == RigConsole }

// RigPath is the path of a rig's game, unique in the table and meaningless on disk.
func RigPath(rigID int64, appID int) string {
	return "rig://" + strconv.FormatInt(rigID, 10) + "/" + strconv.Itoa(appID)
}

// CreateRigLibrary makes the library that holds a rig's games.
func (s *Store) CreateRigLibrary(ctx context.Context, name string) (*Library, error) {
	lib, err := s.CreateLibrary(ctx, name, "games", nil)
	if err != nil {
		return nil, err
	}
	if err := s.SetLibraryConsole(ctx, lib.ID, RigConsole); err != nil {
		_ = s.DeleteLibrary(ctx, lib.ID)
		return nil, err
	}
	lib.Console = RigConsole
	return lib, nil
}

// SaveRigGame adds a game to a rig's library, or updates it, and returns its item ID.
// Details from elsewhere stay as they are. The title is always the PC's, as that's the
// name Moonlight needs.
func (s *Store) SaveRigGame(ctx context.Context, libraryID int64, path, title, tagline, poster string) (int64, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM items WHERE library_id = ? AND path = ?`, libraryID, path).Scan(&id)
	if err == nil {
		it, err := s.Item(ctx, id)
		if err != nil {
			return 0, err
		}
		it.Title, it.SortTitle, it.Tagline, it.Poster = title, SortTitle(title), tagline, poster
		return id, s.upsertItem(ctx, it)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	it := &Item{LibraryID: libraryID, Kind: "game", Path: path, Title: title, SortTitle: SortTitle(title), Tagline: tagline, Poster: poster, Console: RigConsole}
	if err := s.upsertItem(ctx, it); err != nil {
		return 0, err
	}
	return it.ID, nil
}

// rigEnriched marks a rig's game as already looked up online, found or not, so that
// refreshing a rig's games doesn't ask again each time.
const rigEnriched = "wittysunshine"

// EnrichRigGame fills a game's empty details from Wikidata, once, as a scan does for a game
// file. The title stays the PC's, and box art isn't taken from there.
func (s *Store) EnrichRigGame(ctx context.Context, itemID int64) error {
	it, err := s.Item(ctx, itemID)
	if err != nil {
		return err
	}
	if it.Overview != "" || it.Provider != "" {
		return nil
	}
	title := it.Title
	wctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	err = (&Wikidata{Client: SafeClient}).EnrichGame(wctx, it)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if it.Provider == "" {
		it.Provider = rigEnriched
	}
	it.Title, it.SortTitle = title, SortTitle(title)
	return s.upsertItem(ctx, it)
}

// RemoveRigGame takes a game out of a rig's library.
func (s *Store) RemoveRigGame(ctx context.Context, itemID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM items WHERE id = ? AND console = ?`, itemID, RigConsole)
	return err
}

// VisibleRigGames counts the games of a rig's library this account may see.
func (s *Store) VisibleRigGames(ctx context.Context, libraryID int64, a *Access) (int, error) {
	clause, args := a.ItemClause()
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM items i WHERE i.library_id = ? AND i.missing = 0 AND `+clause,
		append([]any{libraryID}, args...)...).Scan(&n)
	return n, err
}
