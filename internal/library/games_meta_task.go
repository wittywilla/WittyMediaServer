// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// What finding out about games is doing, in the activity panel.
const (
	phaseLocal    = "Names from the local list"
	phaseOnline   = "Asking online sources and fetching box art"
	phasePictures = "Fetching box art"
)

// gameMetaTask reports one round of the lookup worker. The worker wakes every few minutes,
// so nothing shows until a round has found something to do. Pictures go on beside the
// details, so either can be the first to find some. A nil registry does nothing, as ever.
type gameMetaTask struct {
	g  *GameMeta
	mu sync.Mutex
	h  *tasks.Handle
	// What the panel's bar counts: the stage's games. Pictures count once they're all that's left.
	phase       string
	done, total int64
}

// stage starts the next stage, saying how much it has ahead of it.
func (t *gameMetaTask) stage(ctx context.Context, phase string) {
	total := t.g.Store.gameWorkAhead(ctx, phase)
	t.mu.Lock()
	defer t.mu.Unlock()
	if total == 0 && t.h != nil {
		return // nothing left: the bar stays as the last stage left it, full
	}
	t.phase, t.done, t.total = phase, 0, total
	if t.h != nil {
		t.h.Phase(phase)
		t.h.Progress(0, total)
	}
}

// started gives the task, starting it if this is the first there's been to do.
func (t *gameMetaTask) started() *tasks.Handle {
	if t.h == nil {
		t.h = t.g.Tasks.Start(tasks.Spec{Type: tasks.TypeGames, Title: "Finding out about games", Phase: t.phase, Unit: "games", Pausable: true})
	}
	return t.h
}

// did says n games of this stage have been dealt with.
func (t *gameMetaTask) did(n int) {
	if t == nil || n == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.done += int64(n)
	t.total = max(t.total, t.done)
	t.started().Progress(t.done, t.total)
}

// picture says n games have had their pictures fetched. It only moves the bar once they're
// all that's left, as they aren't the same games the details are counting.
func (t *gameMetaTask) picture(n int) {
	if n == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	h := t.started()
	if t.phase == phasePictures {
		t.done += int64(n)
		t.total = max(t.total, t.done)
		h.Progress(t.done, t.total)
	}
}

// wait holds the worker while someone has paused it.
func (t *gameMetaTask) wait(ctx context.Context) error {
	t.mu.Lock()
	h := t.h
	t.mu.Unlock()
	return h.Wait(ctx)
}

func (t *gameMetaTask) finish(ctx context.Context, failed error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.h == nil {
		return
	}
	if ctx.Err() == nil && failed == nil {
		t.h.Result(gameMetaResult(t.g.Status(ctx)))
	}
	t.h.Finish(failed)
}

// gameMetaResult says what's known about the games now, for the activity history.
func gameMetaResult(st GameMetaStatus) string {
	out := fmt.Sprintf("Recognised %d of %d games, %d of them by checksum. %d have box art.", st.Matched, st.Total, st.ByChecksum, st.WithPoster)
	if st.Pending > 0 {
		out += fmt.Sprintf(" %d are still waiting for an online source, and they'll be asked later.", st.Pending)
	}
	return out
}

// gameWorkAhead counts what a stage has to do, for its bar.
func (s *Store) gameWorkAhead(ctx context.Context, phase string) int64 {
	where := `m.status = 'pending'`
	switch phase {
	case phaseLocal:
		where += ` AND NOT EXISTS (SELECT 1 FROM game_meta_sources s WHERE s.item_id = m.item_id AND s.source = 'libretro')`
	case phasePictures:
		where = `m.art_wanted = 1`
	}
	var n sql.NullInt64
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM game_meta m JOIN items i ON i.id = m.item_id WHERE i.missing = 0 AND i.kind = 'game' AND `+hideLinked+` AND `+where).Scan(&n)
	return n.Int64
}
