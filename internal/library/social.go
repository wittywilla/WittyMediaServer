package library

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// What accounts do with items: like, comment, watch and share. All of it stays
// on this server and belongs to one account; deleting the account removes it.

// ---------------------------------------------------------------------------
// Likes
// ---------------------------------------------------------------------------

// SetReaction saves an account's like (1), dislike (-1) or neither (0).
func (s *Store) SetReaction(ctx context.Context, userID, itemID int64, value int) error {
	if value == 0 {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM item_reactions WHERE user_id = ? AND item_id = ?`, userID, itemID)
		return err
	}
	if value > 0 {
		value = 1
	} else {
		value = -1
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO item_reactions(user_id, item_id, value, created_at) VALUES(?,?,?,?)
		ON CONFLICT(user_id, item_id) DO UPDATE SET value = excluded.value, created_at = excluded.created_at`, userID, itemID, value, db.Now())
	return err
}

// ---------------------------------------------------------------------------
// Comments
// ---------------------------------------------------------------------------

// Comment is one comment on an item. Imported ones came with an archive and are
// shown with the name they had there; they can't be edited.
type Comment struct {
	ID        int64  `json:"id"`
	ItemID    int64  `json:"itemId"`
	ParentID  *int64 `json:"parentId,omitempty"`
	UserID    *int64 `json:"userId,omitempty"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	Imported  bool   `json:"imported"`
	Likes     int64  `json:"likes,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	EditedAt  *int64 `json:"editedAt,omitempty"`
}

// MaxCommentRunes is the longest comment accepted.
const MaxCommentRunes = 2000

// CleanComment trims a comment and drops control characters, keeping line
// breaks. It returns "" for an empty one or one that is too long.
func CleanComment(body string) string {
	body = strings.TrimSpace(strings.ToValidUTF8(body, ""))
	body = strings.Map(func(r rune) rune {
		if r == '\n' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, strings.ReplaceAll(body, "\r\n", "\n"))
	if body == "" || len([]rune(body)) > MaxCommentRunes {
		return ""
	}
	return body
}

// Comments lists an item's comments: those of accounts on this server first,
// then those that came with the archive by their likes.
func (s *Store) Comments(ctx context.Context, itemID int64, limit, offset int) ([]Comment, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM item_comments WHERE item_id = ?`, itemID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id, c.item_id, c.parent_id, c.user_id, CASE WHEN c.imported = 1 THEN c.author ELSE IFNULL(u.name, 'Former profile') END,
		c.body, c.imported, c.likes, c.created_at, c.edited_at FROM item_comments c LEFT JOIN users u ON u.id = c.user_id
		WHERE c.item_id = ? ORDER BY c.imported, CASE WHEN c.imported = 1 THEN -c.likes ELSE -c.created_at END, c.id LIMIT ? OFFSET ?`, itemID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var c Comment
		var parent, user, edited sql.NullInt64
		if err := rows.Scan(&c.ID, &c.ItemID, &parent, &user, &c.Author, &c.Body, &c.Imported, &c.Likes, &c.CreatedAt, &edited); err != nil {
			return nil, 0, err
		}
		if parent.Valid {
			c.ParentID = &parent.Int64
		}
		if user.Valid {
			c.UserID = &user.Int64
		}
		if edited.Valid {
			c.EditedAt = &edited.Int64
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// AddComment saves a comment by an account. A reply must answer a comment on
// the same item.
func (s *Store) AddComment(ctx context.Context, userID, itemID int64, parentID *int64, body string) (*Comment, error) {
	if parentID != nil {
		var on int64
		if err := s.DB.QueryRowContext(ctx, `SELECT item_id FROM item_comments WHERE id = ?`, *parentID).Scan(&on); err != nil || on != itemID {
			return nil, ErrNotFound
		}
	}
	now := db.Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO item_comments(item_id, user_id, parent_id, body, created_at) VALUES(?,?,?,?,?)`, itemID, userID, parentID, body, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	var name string
	_ = s.DB.QueryRowContext(ctx, `SELECT name FROM users WHERE id = ?`, userID).Scan(&name)
	return &Comment{ID: id, ItemID: itemID, ParentID: parentID, UserID: &userID, Author: name, Body: body, CreatedAt: now}, nil
}

// CommentOwner says who wrote a comment (0 for an imported one) and on what.
func (s *Store) CommentOwner(ctx context.Context, id int64) (userID, itemID int64, err error) {
	var u sql.NullInt64
	err = s.DB.QueryRowContext(ctx, `SELECT user_id, item_id FROM item_comments WHERE id = ?`, id).Scan(&u, &itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, ErrNotFound
	}
	return u.Int64, itemID, err
}

// EditComment replaces a comment's text.
func (s *Store) EditComment(ctx context.Context, id int64, body string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE item_comments SET body = ?, edited_at = ? WHERE id = ? AND imported = 0`, body, db.Now(), id)
	return err
}

// DeleteComment removes a comment and its replies.
func (s *Store) DeleteComment(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM item_comments WHERE id = ?`, id)
	return err
}

// maxImportedComments keeps a rarely-read list from growing without bound.
const maxImportedComments = 500

// importComments replaces the comments that came with an archive.
func (s *Store) importComments(ctx context.Context, itemID int64, in []sideComment) error {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM item_comments WHERE item_id = ? AND imported = 1`, itemID); err != nil {
		return err
	}
	sort.SliceStable(in, func(a, b int) bool {
		var x, y int64
		if in[a].LikeCount != nil {
			x = *in[a].LikeCount
		}
		if in[b].LikeCount != nil {
			y = *in[b].LikeCount
		}
		return x > y
	})
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	n := 0
	for _, c := range in {
		body := CleanComment(c.Text)
		if body == "" {
			continue
		}
		var likes, at int64
		if c.LikeCount != nil {
			likes = *c.LikeCount
		}
		if c.Timestamp != nil {
			at = int64(*c.Timestamp)
		}
		author := strings.TrimSpace(c.Author)
		if len(author) > 120 {
			author = author[:120]
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO item_comments(item_id, author, body, imported, likes, created_at) VALUES(?,?,?,1,?,?)`, itemID, author, body, likes, at); err != nil {
			return err
		}
		if n++; n >= maxImportedComments {
			break
		}
	}
	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Watching
// ---------------------------------------------------------------------------

// ViewEvent is what a player reports when a viewer moves on from a video.
type ViewEvent struct {
	Seconds   float64 `json:"seconds"`
	Completed bool    `json:"completed"`
	Skipped   bool    `json:"skipped"`
}

// RecordView adds one play to an item's numbers for an account.
func (s *Store) RecordView(ctx context.Context, userID, itemID int64, e ViewEvent) error {
	if e.Seconds < 0 {
		e.Seconds = 0
	}
	if e.Seconds > 24*3600 {
		e.Seconds = 24 * 3600
	}
	complete, skip := 0, 0
	if e.Completed {
		complete = 1
	}
	if e.Skipped {
		skip = 1
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO item_stats(user_id, item_id, plays, seconds, completes, skips, last_at) VALUES(?,?,1,?,?,?,?)
		ON CONFLICT(user_id, item_id) DO UPDATE SET plays = plays + 1, seconds = seconds + excluded.seconds, completes = completes + excluded.completes,
		skips = skips + excluded.skips, last_at = excluded.last_at`, userID, itemID, e.Seconds, complete, skip, db.Now())
	if err != nil {
		return err
	}
	return s.LogPlay(ctx, userID, itemID, e.Seconds)
}

// dayNumber is the day of the server's calendar, as YYYYMMDD.
func dayNumber(t time.Time) int {
	y, m, d := t.Date()
	return y*10000 + int(m)*100 + d
}

// LogPlay adds time to an account's daily record, which Year in review reads.
func (s *Store) LogPlay(ctx context.Context, userID, itemID int64, seconds float64) error {
	if seconds <= 0 {
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO play_log(user_id, item_id, day, seconds) VALUES(?,?,?,?)
		ON CONFLICT(user_id, item_id, day) DO UPDATE SET seconds = seconds + excluded.seconds`, userID, itemID, dayNumber(time.Now()), seconds)
	return err
}

// ClearWatchHistory forgets what an account watched: its numbers, its daily
// record and its likes stay apart from the media itself.
func (s *Store) ClearWatchHistory(ctx context.Context, userID int64) error {
	for _, q := range []string{`DELETE FROM item_stats WHERE user_id = ?`, `DELETE FROM play_log WHERE user_id = ?`} {
		if _, err := s.DB.ExecContext(ctx, q, userID); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Links to share
// ---------------------------------------------------------------------------

// ShareLink lets someone without an account see one item or album. Only a hash
// of the link's secret is stored, so the link itself is shown once, when made.
type ShareLink struct {
	ID            int64  `json:"id"`
	UserID        int64  `json:"userId"`
	Owner         string `json:"owner,omitempty"`
	Kind          string `json:"kind"` // item | album
	TargetID      int64  `json:"targetId"`
	Title         string `json:"title"`
	AllowDownload bool   `json:"allowDownload"`
	ExpiresAt     *int64 `json:"expiresAt"`
	Views         int64  `json:"views"`
	CreatedAt     int64  `json:"createdAt"`
	Token         string `json:"token,omitempty"` // only when just created
}

func hashShareToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// CreateShare makes a link and returns it with its secret.
func (s *Store) CreateShare(ctx context.Context, l ShareLink) (*ShareLink, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	tok := base64.RawURLEncoding.EncodeToString(raw)
	now := db.Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO share_links(token_hash, user_id, kind, target_id, title, allow_download, expires_at, created_at) VALUES(?,?,?,?,?,?,?,?)`,
		hashShareToken(tok), l.UserID, l.Kind, l.TargetID, l.Title, l.AllowDownload, l.ExpiresAt, now)
	if err != nil {
		return nil, err
	}
	l.ID, _ = res.LastInsertId()
	l.CreatedAt, l.Token = now, tok
	return &l, nil
}

// ShareByToken finds a link that is still valid.
func (s *Store) ShareByToken(ctx context.Context, tok string) (*ShareLink, error) {
	if tok == "" || len(tok) > 128 {
		return nil, ErrNotFound
	}
	var l ShareLink
	var exp sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT id, user_id, kind, target_id, title, allow_download, expires_at, views, created_at FROM share_links WHERE token_hash = ?`, hashShareToken(tok)).Scan(
		&l.ID, &l.UserID, &l.Kind, &l.TargetID, &l.Title, &l.AllowDownload, &exp, &l.Views, &l.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if exp.Valid {
		if exp.Int64 <= db.Now() {
			return nil, ErrNotFound
		}
		l.ExpiresAt = &exp.Int64
	}
	return &l, nil
}

// CountShareView adds one to a link's opens.
func (s *Store) CountShareView(ctx context.Context, id int64) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE share_links SET views = views + 1 WHERE id = ?`, id)
}

// Shares lists the links an account made (all of them for an administrator).
func (s *Store) Shares(ctx context.Context, userID int64, all bool) ([]ShareLink, error) {
	w, args := "l.user_id = ?", []any{userID}
	if all {
		w, args = "1=1", nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT l.id, l.user_id, IFNULL(u.name, ''), l.kind, l.target_id, l.title, l.allow_download, l.expires_at, l.views, l.created_at
		FROM share_links l LEFT JOIN users u ON u.id = l.user_id WHERE `+w+` ORDER BY l.created_at DESC LIMIT 1000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ShareLink{}
	for rows.Next() {
		var l ShareLink
		var exp sql.NullInt64
		if err := rows.Scan(&l.ID, &l.UserID, &l.Owner, &l.Kind, &l.TargetID, &l.Title, &l.AllowDownload, &exp, &l.Views, &l.CreatedAt); err != nil {
			return nil, err
		}
		if exp.Valid {
			l.ExpiresAt = &exp.Int64
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ShareOwner says who made a link.
func (s *Store) ShareOwner(ctx context.Context, id int64) (int64, error) {
	var u int64
	err := s.DB.QueryRowContext(ctx, `SELECT user_id FROM share_links WHERE id = ?`, id).Scan(&u)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return u, err
}

// DeleteShare ends a link.
func (s *Store) DeleteShare(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM share_links WHERE id = ?`, id)
	return err
}

// PurgeExpiredShares drops links past their date.
func (s *Store) PurgeExpiredShares(ctx context.Context) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM share_links WHERE expires_at IS NOT NULL AND expires_at <= ?`, db.Now())
}

// topKeys returns the keys with the highest counts, ties by name.
func topKeys(count map[string]int, limit int) []string {
	out := make([]string, 0, len(count))
	for k := range count {
		out = append(out, k)
	}
	sort.Slice(out, func(a, b int) bool {
		if count[out[a]] != count[out[b]] {
			return count[out[a]] > count[out[b]]
		}
		return out[a] < out[b]
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
