package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// Collections and smart playlists gather things from any library. A collection
// is a list someone made; a smart one is a saved rule, worked out each time it is
// opened, so it stays current on its own. They belong to one account unless shared;
// either way each account sees only what its own limits allow.

// CollectionRule is what a smart playlist asks for. Everything is a whitelisted
// field turned into SQL with arguments; nothing in it reaches a statement as text.
type CollectionRule struct {
	Libraries       []int64  `json:"libraries,omitempty"`
	LibraryTypes    []string `json:"libraryTypes,omitempty"` // movies, music, ...
	Genres          []string `json:"genres,omitempty"`       // any of these
	Tags            []string `json:"tags,omitempty"`         // creator video tags, any of these
	Search          string   `json:"search,omitempty"`
	Artist          string   `json:"artist,omitempty"`
	YearFrom        int      `json:"yearFrom,omitempty"`
	YearTo          int      `json:"yearTo,omitempty"`
	RatingMin       float64  `json:"ratingMin,omitempty"`
	RuntimeMinSec   float64  `json:"runtimeMinSec,omitempty"`
	RuntimeMaxSec   float64  `json:"runtimeMaxSec,omitempty"`
	Watched         string   `json:"watched,omitempty"` // yes | no | started
	Liked           bool     `json:"liked,omitempty"`
	AddedWithinDays int      `json:"addedWithinDays,omitempty"`
	Sort            string   `json:"sort,omitempty"` // added | title | year | rating | runtime | random | played
	Desc            bool     `json:"desc,omitempty"`
	Limit           int      `json:"limit,omitempty"` // up to 500
	Episodes        bool     `json:"episodes,omitempty"`
}

// maxSmartItems bounds a smart playlist, whatever its rule asks.
const maxSmartItems = 500

// Clean checks a rule; its errors are safe to show.
func (r *CollectionRule) Clean() error {
	r.Search, r.Artist = strings.TrimSpace(r.Search), strings.TrimSpace(r.Artist)
	if len(r.Libraries) > 100 || len(r.LibraryTypes) > 30 || len(r.Genres) > 50 || len(r.Tags) > 50 || len(r.Search) > 200 || len(r.Artist) > 200 {
		return ErrBadInput("that rule is too long")
	}
	switch r.Watched {
	case "", "yes", "no", "started":
	default:
		return ErrBadInput("watched must be yes, no or started")
	}
	switch r.Sort {
	case "", "added", "title", "year", "rating", "runtime", "random", "played":
	default:
		return ErrBadInput("that sort isn't available")
	}
	if r.Limit < 0 || r.Limit > maxSmartItems {
		r.Limit = maxSmartItems
	}
	if r.AddedWithinDays < 0 {
		r.AddedWithinDays = 0
	}
	return nil
}

// where turns a rule into a condition on items aliased "i".
func (r *CollectionRule) where(userID int64) (string, []any) {
	w := []string{"1=1"}
	var args []any
	anyOf := func(col string, vals []string) {
		if len(vals) == 0 {
			return
		}
		parts := make([]string, len(vals))
		for i, v := range vals {
			parts[i] = "('|' || IFNULL(" + col + ", '') || '|') LIKE ?"
			args = append(args, "%|"+strings.ReplaceAll(v, "|", "")+"|%")
		}
		w = append(w, "("+strings.Join(parts, " OR ")+")")
	}
	if len(r.Libraries) > 0 {
		w = append(w, "i.library_id IN ("+ids(r.Libraries)+")")
	}
	if len(r.LibraryTypes) > 0 {
		q := make([]string, len(r.LibraryTypes))
		for i, t := range r.LibraryTypes {
			q[i] = "?"
			args = append(args, t)
		}
		w = append(w, "i.library_id IN (SELECT id FROM libraries WHERE type IN ("+strings.Join(q, ",")+"))")
	}
	anyOf("i.genres", r.Genres)
	if len(r.Tags) > 0 {
		sub := len(w)
		anyOf("v.tags", r.Tags)
		w[sub] = "i.id IN (SELECT v.item_id FROM video_meta v WHERE " + w[sub] + ")"
	}
	if r.Search != "" {
		like := "%" + r.Search + "%"
		w = append(w, "(i.title LIKE ? OR i.overview LIKE ?)")
		args = append(args, like, like)
	}
	if r.Artist != "" {
		like := "%" + r.Artist + "%"
		w = append(w, "(i.artist LIKE ? OR i.album_artist LIKE ? OR i.studio LIKE ?)")
		args = append(args, like, like, like)
	}
	if r.YearFrom > 0 {
		w = append(w, "i.year >= ?")
		args = append(args, r.YearFrom)
	}
	if r.YearTo > 0 {
		w = append(w, "i.year <= ?")
		args = append(args, r.YearTo)
	}
	if r.RatingMin > 0 {
		w = append(w, "i.rating >= ?")
		args = append(args, r.RatingMin)
	}
	if r.RuntimeMinSec > 0 {
		w = append(w, "i.runtime_sec >= ?")
		args = append(args, r.RuntimeMinSec)
	}
	if r.RuntimeMaxSec > 0 {
		w = append(w, "i.runtime_sec <= ?")
		args = append(args, r.RuntimeMaxSec)
	}
	switch r.Watched {
	case "yes":
		w = append(w, "i.id IN (SELECT item_id FROM progress WHERE user_id = ? AND watched = 1)")
		args = append(args, userID)
	case "no":
		w = append(w, "i.id NOT IN (SELECT item_id FROM progress WHERE user_id = ? AND watched = 1)")
		args = append(args, userID)
	case "started":
		w = append(w, "i.id IN (SELECT item_id FROM progress WHERE user_id = ? AND watched = 0)")
		args = append(args, userID)
	}
	if r.Liked {
		w = append(w, "i.id IN (SELECT item_id FROM item_reactions WHERE user_id = ? AND value > 0)")
		args = append(args, userID)
	}
	if r.AddedWithinDays > 0 {
		w = append(w, "i.added_at >= ?")
		args = append(args, db.Now()-int64(r.AddedWithinDays)*86400)
	}
	if !r.Episodes {
		w = append(w, "i.show_id IS NULL")
	}
	return strings.Join(w, " AND "), args
}

func (r *CollectionRule) order(userID int64) (string, []any) {
	dir := ""
	if r.Desc {
		dir = " DESC"
	}
	switch r.Sort {
	case "title":
		return "i.sort_title COLLATE NOCASE" + dir + ", i.id", nil
	case "year":
		return "IFNULL(i.year, 0)" + dir + ", i.id", nil
	case "rating":
		return "IFNULL(i.rating, 0)" + dir + ", i.id", nil
	case "runtime":
		return "IFNULL(i.runtime_sec, 0)" + dir + ", i.id", nil
	case "random":
		return "RANDOM()", nil
	case "played":
		return "(SELECT IFNULL(SUM(seconds), 0) FROM play_log p WHERE p.item_id = i.id AND p.user_id = ?)" + dir + ", i.id", []any{userID}
	}
	return "i.added_at" + dir + ", i.id", nil
}

// Collection is a list or a smart playlist.
type Collection struct {
	ID          int64           `json:"id"`
	OwnerID     int64           `json:"ownerId"`
	Owner       string          `json:"owner"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Kind        string          `json:"kind"` // manual | smart
	Rule        *CollectionRule `json:"rule,omitempty"`
	Shared      bool            `json:"shared"`
	System      string          `json:"system,omitempty"` // watchlater
	Count       int             `json:"count"`
	CoverID     int64           `json:"coverId"`
	CoverShow   int64           `json:"coverShowId,omitempty"`
	UpdatedAt   int64           `json:"updatedAt"`
}

const collectionCols = `c.id, c.owner_id, IFNULL(u.name, ''), c.name, c.description, c.kind, c.rule, c.shared, c.system, c.updated_at`

func scanCollection(sc interface{ Scan(...any) error }) (*Collection, error) {
	var c Collection
	var rule string
	if err := sc.Scan(&c.ID, &c.OwnerID, &c.Owner, &c.Name, &c.Description, &c.Kind, &rule, &c.Shared, &c.System, &c.UpdatedAt); err != nil {
		return nil, err
	}
	if c.Kind == "smart" && rule != "" {
		var r CollectionRule
		if json.Unmarshal([]byte(rule), &r) == nil {
			c.Rule = &r
		}
	}
	return &c, nil
}

// Collections lists what an account can see: its own and those shared with everyone.
func (s *Store) Collections(ctx context.Context, userID int64, access *Access) ([]*Collection, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+collectionCols+` FROM collections c LEFT JOIN users u ON u.id = c.owner_id
		WHERE c.owner_id = ? OR c.shared = 1 ORDER BY c.system DESC, c.name COLLATE NOCASE LIMIT 2000`, userID)
	if err != nil {
		return nil, err
	}
	out := []*Collection{}
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, c := range out {
		if c.Kind == "smart" {
			if c.Rule == nil {
				continue
			}
			c.Count, c.CoverID, err = s.smartSummary(ctx, c.Rule, userID, access)
		} else {
			c.Count, c.CoverID, c.CoverShow, err = s.manualSummary(ctx, c.ID, access)
		}
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) smartSummary(ctx context.Context, r *CollectionRule, userID int64, access *Access) (count int, cover int64, err error) {
	w, args := r.where(userID)
	clause, cargs := access.ItemClause()
	all := append(append([]any{}, args...), cargs...)
	var c sql.NullInt64
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*), MAX(i.id) FROM items i WHERE i.missing = 0 AND `+w+` AND `+clause, all...).Scan(&count, &c)
	if r.Limit > 0 && count > r.Limit {
		count = r.Limit
	}
	return count, c.Int64, err
}

func (s *Store) manualSummary(ctx context.Context, id int64, access *Access) (count int, cover, coverShow int64, err error) {
	clause, cargs := access.ItemClause()
	var c sql.NullInt64
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*), MAX(i.id) FROM collection_items x JOIN items i ON i.id = x.item_id WHERE x.collection_id = ? AND i.missing = 0 AND `+clause, append([]any{id}, cargs...)...).Scan(&count, &c)
	if err != nil {
		return 0, 0, 0, err
	}
	sc, sargs := access.ShowClause()
	var shows int
	var cs sql.NullInt64
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*), MAX(s.id) FROM collection_shows x JOIN shows s ON s.id = x.show_id WHERE x.collection_id = ? AND s.missing = 0 AND `+sc, append([]any{id}, sargs...)...).Scan(&shows, &cs)
	return count + shows, c.Int64, cs.Int64, err
}

// CollectionByID loads a collection an account may see.
func (s *Store) CollectionByID(ctx context.Context, id, userID int64, isAdmin bool) (*Collection, error) {
	c, err := scanCollection(s.DB.QueryRowContext(ctx, `SELECT `+collectionCols+` FROM collections c LEFT JOIN users u ON u.id = c.owner_id WHERE c.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !c.Shared && c.OwnerID != userID && !isAdmin {
		return nil, ErrNotFound
	}
	return c, nil
}

// SaveCollection creates a collection (ID 0) or changes one.
func (s *Store) SaveCollection(ctx context.Context, c *Collection) error {
	rule := ""
	if c.Kind == "smart" {
		if c.Rule == nil {
			return ErrBadInput("a smart playlist needs a rule")
		}
		if err := c.Rule.Clean(); err != nil {
			return err
		}
		b, err := json.Marshal(c.Rule)
		if err != nil {
			return err
		}
		rule = string(b)
	} else {
		c.Kind, c.Rule = "manual", nil
	}
	now := db.Now()
	c.UpdatedAt = now
	if c.ID == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO collections(owner_id, name, description, kind, rule, shared, system, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			c.OwnerID, c.Name, c.Description, c.Kind, rule, c.Shared, c.System, now, now)
		if err != nil {
			return err
		}
		c.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE collections SET name = ?, description = ?, rule = ?, shared = ?, updated_at = ? WHERE id = ?`, c.Name, c.Description, rule, c.Shared, now, c.ID)
	return err
}

// DeleteCollection removes a collection; what is in it stays.
func (s *Store) DeleteCollection(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM collections WHERE id = ?`, id)
	return err
}

// WatchLater is an account's own list, made the first time it is used.
func (s *Store) WatchLater(ctx context.Context, userID int64) (*Collection, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM collections WHERE owner_id = ? AND system = 'watchlater'`, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		c := &Collection{OwnerID: userID, Name: "Watch later", Kind: "manual", System: "watchlater"}
		if err := s.SaveCollection(ctx, c); err != nil {
			return nil, err
		}
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	return s.CollectionByID(ctx, id, userID, true)
}

// CollectionItems is one page of what a collection holds, for an account.
func (s *Store) CollectionItems(ctx context.Context, c *Collection, userID int64, access *Access, limit, offset int) ([]*Item, []*Show, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	clause, cargs := access.ItemClause()
	var q string
	var args []any
	if c.Kind == "smart" {
		if c.Rule == nil {
			return []*Item{}, nil, nil
		}
		w, wargs := c.Rule.where(userID)
		order, oargs := c.Rule.order(userID)
		// A smart playlist's own limit is part of the rule.
		if c.Rule.Limit > 0 {
			if offset >= c.Rule.Limit {
				return []*Item{}, nil, nil
			}
			if offset+limit > c.Rule.Limit {
				limit = c.Rule.Limit - offset
			}
		}
		q = `SELECT ` + itemCols + ` FROM items i WHERE i.missing = 0 AND ` + w + ` AND ` + clause + ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
		args = append(append(append(args, wargs...), cargs...), oargs...)
	} else {
		q = `SELECT ` + itemCols + ` FROM collection_items x JOIN items i ON i.id = x.item_id WHERE x.collection_id = ? AND i.missing = 0 AND ` + clause + ` ORDER BY x.added_at, x.item_id LIMIT ? OFFSET ?`
		args = append(append(args, c.ID), cargs...)
	}
	rows, err := s.DB.QueryContext(ctx, q, append(args, limit, offset)...)
	if err != nil {
		return nil, nil, err
	}
	items := []*Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			rows.Close()
			return nil, nil, err
		}
		items = append(items, it)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if err := s.AttachProgress(ctx, userID, items); err != nil {
		return nil, nil, err
	}
	shows := []*Show{}
	if c.Kind != "smart" && offset == 0 {
		all, err := s.Shows(ctx, ShowQuery{UserID: userID, Access: access, InCollection: c.ID})
		if err != nil {
			return nil, nil, err
		}
		shows = all
	}
	return items, shows, nil
}

// PreviewRule is what a rule would find, before saving it.
func (s *Store) PreviewRule(ctx context.Context, r *CollectionRule, userID int64, access *Access) ([]*Item, int, error) {
	if err := r.Clean(); err != nil {
		return nil, 0, err
	}
	count, _, err := s.smartSummary(ctx, r, userID, access)
	if err != nil {
		return nil, 0, err
	}
	items, _, err := s.CollectionItems(ctx, &Collection{Kind: "smart", Rule: r}, userID, access, 12, 0)
	return items, count, err
}

// AddToCollection adds items (and shows) that the account may see, checked by the caller.
func (s *Store) AddToCollection(ctx context.Context, id int64, items, shows []int64) error {
	now := db.Now()
	for _, x := range items {
		if _, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO collection_items(collection_id, item_id, added_at) VALUES(?,?,?)`, id, x, now); err != nil {
			return err
		}
	}
	for _, x := range shows {
		if _, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO collection_shows(collection_id, show_id, added_at) VALUES(?,?,?)`, id, x, now); err != nil {
			return err
		}
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE collections SET updated_at = ? WHERE id = ?`, now, id)
	return err
}

// RemoveFromCollection takes items and shows out of a collection.
func (s *Store) RemoveFromCollection(ctx context.Context, id int64, items, shows []int64) error {
	for _, x := range items {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM collection_items WHERE collection_id = ? AND item_id = ?`, id, x); err != nil {
			return err
		}
	}
	for _, x := range shows {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM collection_shows WHERE collection_id = ? AND show_id = ?`, id, x); err != nil {
			return err
		}
	}
	return nil
}

// CollectionsHolding lists which of an account's lists hold an item or show.
func (s *Store) CollectionsHolding(ctx context.Context, userID, itemID, showID int64) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id FROM collections c WHERE c.owner_id = ? AND (
		EXISTS(SELECT 1 FROM collection_items x WHERE x.collection_id = c.id AND x.item_id = ?) OR
		EXISTS(SELECT 1 FROM collection_shows x WHERE x.collection_id = c.id AND x.show_id = ?))`, userID, itemID, showID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

// NextInCollection is what comes after an item when a list is played through.
func (s *Store) NextInCollection(ctx context.Context, c *Collection, userID, after int64, access *Access) *int64 {
	items, _, err := s.CollectionItems(ctx, c, userID, access, 200, 0)
	if err != nil {
		return nil
	}
	for i, it := range items {
		if it.ID == after && i+1 < len(items) {
			return &items[i+1].ID
		}
	}
	return nil
}
