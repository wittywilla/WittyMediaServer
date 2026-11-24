// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package notify keeps each profile's notifications: new episodes, recordings that finished,
// scans that failed, and updates for modules and the server. They stay on the server, read by
// the bell in each app. An administrator may also have their own notifications sent on to a
// channel they set up and own (email through their SMTP server, or a webhook); nothing else
// ever leaves. docs/NOTIFICATIONS.md has the whole of it.
package notify

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"
)

// Kind is what a notification is about. Each profile chooses which kinds it gets.
type Kind string

const (
	Episodes   Kind = "episodes"   // new episodes of a show the profile can open
	Recordings Kind = "recordings" // a recording the profile set up finished or failed
	Scans      Kind = "scans"      // a library scan failed (administrators)
	Modules    Kind = "modules"    // modules have updates (administrators)
	Server     Kind = "server"     // a new version of the server (administrators)
)

// Kinds is every kind, in the order the choices are shown.
var Kinds = []Kind{Episodes, Recordings, Scans, Modules, Server}

// AdminOnly says whether only administrators get a kind.
func AdminOnly(k Kind) bool { return k == Scans || k == Modules || k == Server }

// Valid says whether k is a kind this server knows.
func Valid(k Kind) bool {
	for _, x := range Kinds {
		if x == k {
			return true
		}
	}
	return false
}

// Note is one notification as its profile sees it.
type Note struct {
	ID    int64  `json:"id"`
	Kind  Kind   `json:"kind"`
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	// Link is a path in the app ("/show/12"), never an address elsewhere.
	Link string `json:"link,omitempty"`
	At   int64  `json:"at"` // unix seconds
	Read bool   `json:"read"`
}

// DB is what the store needs from the server's database.
type DB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const (
	keepPerUser = 200
	keepDays    = 90
	maxTitle    = 200
	maxBody     = 600
	maxLink     = 300
)

// Hub stores notifications and hands new ones to Out.
type Hub struct {
	DB  DB
	Log *slog.Logger
	// Out, if set, is told of each notification as it's stored, to send it on to the
	// profile's own channels. It must not block.
	Out func(userID int64, n Note)
	now func() time.Time
}

// New makes a hub.
func New(db DB, log *slog.Logger) *Hub {
	if log == nil {
		log = slog.Default()
	}
	return &Hub{DB: db, Log: log, now: time.Now}
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// safeLink keeps links inside the app: a path, with no scheme or host.
func safeLink(s string) string {
	if !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") || strings.ContainsAny(s, "\\\r\n") || len(s) > maxLink {
		return ""
	}
	return s
}

// Send stores n for each profile in to that hasn't turned its kind off. dedupe, if not
// empty, stops the same thing being said twice to one profile (a version found again at
// the next check, say). It returns how many profiles got it.
func (h *Hub) Send(ctx context.Context, to []int64, n Note, dedupe string) int {
	if h == nil || len(to) == 0 || !Valid(n.Kind) {
		return 0
	}
	n.Title, n.Body, n.Link = clip(n.Title, maxTitle), clip(n.Body, maxBody), safeLink(n.Link)
	if n.Title == "" {
		return 0
	}
	n.At, n.Read = h.now().Unix(), false
	dedupe = clip(dedupe, 200)
	sent := 0
	seen := map[int64]bool{}
	for _, uid := range to {
		if uid <= 0 || seen[uid] {
			continue
		}
		seen[uid] = true
		if !h.Wants(ctx, uid, n.Kind) {
			continue
		}
		res, err := h.DB.ExecContext(ctx, `INSERT OR IGNORE INTO notifications(user_id, kind, title, body, link, dedupe, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, uid, string(n.Kind), n.Title, n.Body, n.Link, dedupe, n.At)
		if err != nil {
			h.Log.Warn("could not keep a notification", "cat", "notify", "account", uid, "err", err)
			continue
		}
		if c, _ := res.RowsAffected(); c == 0 {
			continue // already said
		}
		id, _ := res.LastInsertId()
		sent++
		h.prune(ctx, uid)
		if h.Out != nil {
			one := n
			one.ID = id
			h.Out(uid, one)
		}
	}
	return sent
}

func (h *Hub) prune(ctx context.Context, uid int64) {
	_, err := h.DB.ExecContext(ctx, `DELETE FROM notifications WHERE user_id = ? AND (created_at < ? OR id NOT IN
		(SELECT id FROM notifications WHERE user_id = ? ORDER BY id DESC LIMIT ?))`,
		uid, h.now().Add(-keepDays*24*time.Hour).Unix(), uid, keepPerUser)
	if err != nil {
		h.Log.Warn("could not trim old notifications", "cat", "notify", "err", err)
	}
}

// List returns a profile's notifications, newest first, older than before (an id) if it's set.
func (h *Hub) List(ctx context.Context, uid, before int64, limit int) ([]Note, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if before <= 0 {
		before = 1<<62 - 1
	}
	rows, err := h.DB.QueryContext(ctx, `SELECT id, kind, title, body, link, created_at, read_at FROM notifications
		WHERE user_id = ? AND id < ? ORDER BY id DESC LIMIT ?`, uid, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		var n Note
		var kind string
		var readAt int64
		if err := rows.Scan(&n.ID, &kind, &n.Title, &n.Body, &n.Link, &n.At, &readAt); err != nil {
			return nil, err
		}
		n.Kind, n.Read = Kind(kind), readAt > 0
		out = append(out, n)
	}
	return out, rows.Err()
}

// Count says how many a profile hasn't read, and the newest one's id (0 for none), which is
// all a bell needs to ask for while it waits.
func (h *Hub) Count(ctx context.Context, uid int64) (unread int, latest int64, err error) {
	err = h.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(read_at = 0), 0), COALESCE(MAX(id), 0) FROM notifications WHERE user_id = ?`, uid).Scan(&unread, &latest)
	return
}

// MarkRead marks some (or, with no ids, all) of a profile's notifications read.
func (h *Hub) MarkRead(ctx context.Context, uid int64, ids []int64) error {
	now := h.now().Unix()
	if len(ids) == 0 {
		_, err := h.DB.ExecContext(ctx, `UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at = 0`, now, uid)
		return err
	}
	if len(ids) > 200 {
		return errors.New("too many at once")
	}
	for _, id := range ids {
		if _, err := h.DB.ExecContext(ctx, `UPDATE notifications SET read_at = ? WHERE user_id = ? AND id = ? AND read_at = 0`, now, uid, id); err != nil {
			return err
		}
	}
	return nil
}

// Clear forgets one (id > 0) or all of a profile's notifications.
func (h *Hub) Clear(ctx context.Context, uid, id int64) error {
	var err error
	if id > 0 {
		_, err = h.DB.ExecContext(ctx, `DELETE FROM notifications WHERE user_id = ? AND id = ?`, uid, id)
	} else {
		_, err = h.DB.ExecContext(ctx, `DELETE FROM notifications WHERE user_id = ?`, uid)
	}
	return err
}

// Wants says whether a profile gets a kind: every kind is on until the profile turns it off.
func (h *Hub) Wants(ctx context.Context, uid int64, k Kind) bool {
	var n int
	if err := h.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM notify_muted WHERE user_id = ? AND kind = ?`, uid, string(k)).Scan(&n); err != nil {
		return true
	}
	return n == 0
}

// Choice is one kind and whether a profile gets it.
type Choice struct {
	Kind Kind `json:"kind"`
	On   bool `json:"on"`
}

// Choices are a profile's kinds, for an administrator or not.
func (h *Hub) Choices(ctx context.Context, uid int64, admin bool) []Choice {
	out := []Choice{}
	for _, k := range Kinds {
		if AdminOnly(k) && !admin {
			continue
		}
		out = append(out, Choice{Kind: k, On: h.Wants(ctx, uid, k)})
	}
	return out
}

// SetChoices saves a profile's choices. Kinds it doesn't name are left as they were.
func (h *Hub) SetChoices(ctx context.Context, uid int64, list []Choice) error {
	for _, c := range list {
		if !Valid(c.Kind) {
			return errors.New("unknown kind: " + string(c.Kind))
		}
	}
	for _, c := range list {
		var err error
		if c.On {
			_, err = h.DB.ExecContext(ctx, `DELETE FROM notify_muted WHERE user_id = ? AND kind = ?`, uid, string(c.Kind))
		} else {
			_, err = h.DB.ExecContext(ctx, `INSERT OR IGNORE INTO notify_muted(user_id, kind) VALUES (?, ?)`, uid, string(c.Kind))
		}
		if err != nil {
			return err
		}
	}
	return nil
}
