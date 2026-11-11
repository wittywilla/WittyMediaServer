// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package channels

import (
	"context"
	"database/sql"
	"math/rand/v2"
	"slices"
	"strconv"
	"time"
)

// Resetting a channel: its schedule thrown away, what is on now included, and
// made again from its current definition and the titles that match it now.

// ResetResult says what a reset did to the reminders people had set on the
// channel. Library channels can't be recorded, so there are no recordings to
// say anything about.
type ResetResult struct {
	Kept    int `json:"kept"`    // the programme still airs when it did
	Moved   int `json:"moved"`   // it airs at another time now, and the reminder went with it
	Dropped int `json:"dropped"` // it no longer airs in the guide's two weeks
	// DroppedTitles names what no longer airs, up to six (a note shows five,
	// and a sixth says there are others), for the note
	// shown to whoever reset the channel.
	DroppedTitles []string `json:"droppedTitles,omitempty"`
}

// reminderLook is how far ahead a reminder's programme is looked for: as far
// as the guide goes.
const reminderLook = 15 * 24 * time.Hour

// Reset makes the channel's schedule again as if it had always run with its
// current definition: tune in and you join today's new schedule part-way
// through, as with any channel. Each show carries on from where it had got to
// at the start of today, unless startOver, which starts every show again and
// picks a new shuffle. Reminders follow their programme to its new time, or go
// when it no longer airs.
func (s *Store) Reset(ctx context.Context, id int64, startOver bool) (*ResetResult, error) {
	c, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	today := DayNumber(now, s.loc())
	if !startOver {
		// Where each show had got to is kept with today's schedule, so
		// today's has to have been worked out.
		if _, err := s.Day(ctx, c, today); err != nil {
			return nil, err
		}
	}
	if err := s.reset(ctx, id, today, startOver); err != nil {
		return nil, err
	}
	if c, err = s.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.followReminders(ctx, c, now)
}

func (s *Store) reset(ctx context.Context, id int64, today int, startOver bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	cursors := map[string]*cursor{}
	if row, ok := s.readDay(ctx, id, today); ok && !startOver && row.BeforeCursors() != nil {
		cursors = row.BeforeCursors()
	}
	if startOver {
		c.Seed = rand.Uint64() >> 1
	}
	rev := c.Rev + 1
	if _, err := s.DB.ExecContext(ctx, `UPDATE channels SET rev = ?, seed = ?, anchor_day = ? WHERE id = ?`, rev, int64(c.Seed), today, id); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM channel_days WHERE channel_id = ?`, id); err != nil {
		return err
	}
	c.Rev, c.AnchorDay = rev, today
	before := State{Cursors: cloneCursors(cursors)}
	progs, next, err := GenerateDay(&c.Definition, c.Seed, today, s.loc(), State{Cursors: cursors}, s.loader(ctx), nil)
	if err != nil {
		return err
	}
	return s.writeDay(ctx, c, today, progs, before, next)
}

// followReminders goes through every account's reminders for programmes on
// this channel still to start. A programme is the same one when its title and
// episode are, as the guide shows them.
func (s *Store) followReminders(ctx context.Context, c *Channel, now time.Time) (*ResetResult, error) {
	out := &ResetResult{}
	key := "lib-" + strconv.FormatInt(c.ID, 10)
	type rem struct {
		id, user   int64
		title, sub string
		start      int64
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, title, sub, start FROM tv_reminders WHERE channel = ? AND start > ? ORDER BY start`, key, now.Unix())
	if err != nil {
		return nil, err
	}
	var list []rem
	for rows.Next() {
		var r rem
		if err := rows.Scan(&r.id, &r.user, &r.title, &r.sub, &r.start); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(list) == 0 {
		return out, err
	}
	sched, err := s.Window(ctx, c, now, now.Add(reminderLook))
	if err != nil {
		return nil, err
	}
	cells := GuideCells(sched)
	// One account's two reminders for a title that airs twice (a film, say)
	// don't both move to its first new airing.
	taken := map[[2]int64]bool{}
	for _, r := range list {
		var at *Programme
		for i := range cells {
			p := &cells[i]
			if p.Kind == KindSlate || p.Title != r.title || p.Sub != r.sub || p.Start <= now.Unix() {
				continue
			}
			if p.Start == r.start {
				at = p
				break
			}
			if at == nil && !taken[[2]int64{r.user, p.Start}] {
				at = p
			}
		}
		switch {
		case at != nil && at.Start == r.start:
			out.Kept++
			_, err = s.DB.ExecContext(ctx, `UPDATE tv_reminders SET stop = ? WHERE id = ?`, at.End, r.id)
		case at != nil:
			var res sql.Result
			res, err = s.DB.ExecContext(ctx, `UPDATE OR IGNORE tv_reminders SET start = ?, stop = ? WHERE id = ?`, at.Start, at.End, r.id)
			if n, _ := rowsAffected(res, err); n == 0 && err == nil {
				// That account already has one for the new time.
				_, err = s.DB.ExecContext(ctx, `DELETE FROM tv_reminders WHERE id = ?`, r.id)
			}
			taken[[2]int64{r.user, at.Start}] = true
			out.Moved++
		default:
			_, err = s.DB.ExecContext(ctx, `DELETE FROM tv_reminders WHERE id = ?`, r.id)
			out.Dropped++
			if len(out.DroppedTitles) < 6 && !slices.Contains(out.DroppedTitles, r.title) {
				out.DroppedTitles = append(out.DroppedTitles, r.title)
			}
		}
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func rowsAffected(res sql.Result, err error) (int64, error) {
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
