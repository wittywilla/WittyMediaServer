package library

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
)

// Game rigs: a person's own gaming PC, whose games are listed as a games library. See
// docs/GAME-RIGS.md. A rig's games are ordinary items, which is what makes every access
// rule apply to them: library limits, age limits and per-title choices, in every list.
// Their paths are made up and name nothing on this machine.

const (
	// RigConsole marks a games library, and its items, as belonging to a gaming PC.
	RigConsole = "rig"
	// RigArtPrefix starts the name of every picture that came from a PC.
	RigArtPrefix = "cache:rig-"
)

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
// name Moonlight needs. A nil poster means the PC couldn't say just now, so what's there
// stays, and a picture someone chose by hand is never replaced by the PC's.
func (s *Store) SaveRigGame(ctx context.Context, libraryID int64, path, title, tagline string, poster *string) (int64, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM items WHERE library_id = ? AND path = ?`, libraryID, path).Scan(&id)
	if err == nil {
		it, err := s.Item(ctx, id)
		if err != nil {
			return 0, err
		}
		it.Title, it.SortTitle, it.Tagline = title, SortTitle(title), tagline
		if poster != nil && (it.Poster == "" || strings.HasPrefix(it.Poster, RigArtPrefix)) {
			it.Poster = *poster
		}
		return id, s.upsertItem(ctx, it)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	it := &Item{LibraryID: libraryID, Kind: "game", Path: path, Title: title, SortTitle: SortTitle(title), Tagline: tagline, Console: RigConsole}
	if poster != nil {
		it.Poster = *poster
	}
	if err := s.upsertItem(ctx, it); err != nil {
		return 0, err
	}
	return it.ID, nil
}

// EnrichRigGame queues a game for the metadata sources, as a scan does for a game file.
// The title stays the PC's: it counts as set by hand, so sources leave it alone.
func (s *Store) EnrichRigGame(ctx context.Context, itemID int64) error {
	it, err := s.Item(ctx, itemID)
	if err != nil {
		return err
	}
	if it.Overview != "" || it.Provider != "" {
		return nil
	}
	return s.enqueueGame(ctx, it, []string{"title"})
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
