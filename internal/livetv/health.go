// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/pace"
	"github.com/wittywilla/wittymediaserver/internal/tuner"
)

// Long IPTV playlists are full of channels that have stopped working, and of the same
// station two or three times over from different sources. This is what keeps that
// manageable: a health check of each channel, remembered; backups, which hide one copy
// behind another and are tried when it won't open; and the edits an owner makes to many
// channels at once.

// What a channel's last check or tune found.
const (
	HealthOK      = "ok"
	HealthFailing = "failing"
)

// noteHealth keeps what opening a channel showed. Being busy says nothing about a channel,
// and an encrypted one is labelled as that instead.
func (m *Manager) noteHealth(id int64, err error) {
	health, msg := HealthOK, ""
	switch {
	case err == nil:
	case errors.Is(err, ErrBusy), errors.Is(err, context.Canceled), tuner.IsEncrypted(err):
		return
	default:
		health, msg = HealthFailing, PublicError(err)
		if len(msg) > 300 {
			msg = msg[:300]
		}
	}
	if _, err := m.DB.Exec(`UPDATE tuner_channels SET health = ?, health_at = ?, health_msg = ? WHERE id = ?`, health, time.Now().Unix(), msg, id); err != nil {
		m.Log.Warn("could not save a channel's health", "channel", id, "err", err)
	}
}

// backupsOf lists a channel's backups, the ones that worked last time first.
func (m *Manager) backupsOf(ctx context.Context, id int64) ([]int64, error) {
	rows, err := m.DB.QueryContext(ctx, `SELECT c.id FROM tuner_channels c JOIN tuners t ON t.id = c.tuner_id
		WHERE c.merged_into = ? AND t.enabled = 1 ORDER BY c.health = 'failing', c.health != 'ok', c.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var b int64
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// worthFailingOver: an error that another copy of the station might not have.
func worthFailingOver(ctx context.Context, err error) bool {
	return err != nil && ctx.Err() == nil && !errors.Is(err, ErrNotFound)
}

// --------------------------------------------------------------------------
// Checking channels
// --------------------------------------------------------------------------

// checkTime is how long one channel may take to show it works.
const checkTime = 20 * time.Second

// CheckChannel opens a channel the way watching it would, makes sure something
// arrives, and keeps the answer. It takes a tuner while it runs, within the tuner's limit.
func (m *Manager) CheckChannel(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, checkTime)
	defer cancel()
	o, err := m.openOne(ctx, id)
	if err != nil {
		return err
	}
	defer o.Release()
	err = m.probe(ctx, o.Stream)
	if ctx.Err() != nil && err != nil {
		err = errors.New("nothing arrived from it in time")
	}
	m.noteHealth(id, err)
	return err
}

// probe reads the start of a stream: packets from a tuner, or what FFmpeg finds in a
// stream it would read itself.
func (m *Manager) probe(ctx context.Context, st *tuner.Stream) error {
	if st.Reader != nil {
		buf := make([]byte, 188*64)
		done := make(chan error, 1)
		go func() {
			_, err := io.ReadFull(st.Reader, buf)
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf("the stream stopped at once: %w", err)
			}
			return nil
		case <-ctx.Done():
			_ = st.Reader.Close() // unblocks the read
			<-done
			return ctx.Err()
		}
	}
	if m.Tools == nil || m.Tools.FFprobe == "" {
		return nil // opening it is all that can be checked
	}
	args := append([]string{"-v", "error", "-hide_banner"}, st.Input...)
	args = append(args, "-show_entries", "stream=codec_type", "-of", "csv=p=0")
	cmd := exec.CommandContext(ctx, m.Tools.FFprobe, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("the stream couldn't be read: %s", lastLine(errb.String()))
	}
	if !strings.Contains(out.String(), "video") && !strings.Contains(out.String(), "audio") {
		return errors.New("the stream has no picture or sound")
	}
	return nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// HealthState is how a tuner's channel check is going.
type HealthState struct {
	Running bool  `json:"running"`
	Done    int   `json:"done"`
	Total   int   `json:"total"`
	OK      int   `json:"ok"`
	Failing int   `json:"failing"`
	Started int64 `json:"started,omitempty"`
	stop    context.CancelFunc
}

// StartHealth checks every enabled channel of a tuner, a few at a time and in the
// background. Starting it again while it runs changes nothing.
func (m *Manager) StartHealth(tunerID int64) error {
	t, err := m.Tuner(m.stopCtx, tunerID)
	if err != nil {
		return err
	}
	list, err := m.Channels(m.stopCtx, tunerID)
	if err != nil {
		return err
	}
	var ids []int64
	for _, c := range list {
		if c.Enabled && c.Encrypted == "" {
			ids = append(ids, c.ID)
		}
	}
	m.mu.Lock()
	if m.health == nil {
		m.health = map[int64]*HealthState{}
	}
	if st := m.health[tunerID]; st != nil && st.Running {
		m.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(pace.Background(m.stopCtx))
	st := &HealthState{Running: true, Total: len(ids), Started: time.Now().Unix(), stop: cancel}
	m.health[tunerID] = st
	m.mu.Unlock()
	// Two at a time at most, and never more than the tuner allows, so a check never
	// takes the last tuner from someone watching.
	workers := 2
	if t.MaxStreams > 0 {
		workers = min(workers, max(1, t.MaxStreams-1))
	}
	started := m.background("channel check", func(stop context.Context) {
		defer cancel()
		defer context.AfterFunc(stop, cancel)()
		next := make(chan int64)
		done := make(chan struct{})
		for range workers {
			go func() {
				defer func() { done <- struct{}{} }()
				for id := range next {
					err := m.CheckChannel(ctx, id)
					m.mu.Lock()
					st.Done++
					if err == nil {
						st.OK++
					} else if !errors.Is(err, ErrBusy) && ctx.Err() == nil {
						st.Failing++
					}
					m.mu.Unlock()
				}
			}()
		}
		for _, id := range ids {
			if pace.Yield(ctx) != nil {
				break
			}
			select {
			case next <- id:
			case <-ctx.Done():
			}
			if ctx.Err() != nil {
				break
			}
		}
		close(next)
		for range workers {
			<-done
		}
		m.mu.Lock()
		st.Running = false
		m.mu.Unlock()
	}, "cat", "tv", "tuner", tunerID)
	if !started {
		cancel()
		m.mu.Lock()
		st.Running = false
		m.mu.Unlock()
		return errors.New("the server is stopping")
	}
	return nil
}

// Health is how a tuner's check is going, or nil if none has run.
func (m *Manager) Health(tunerID int64) *HealthState {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.health[tunerID]
	if st == nil {
		return nil
	}
	cp := *st
	cp.stop = nil
	return &cp
}

// StopHealth ends a tuner's check.
func (m *Manager) StopHealth(tunerID int64) {
	m.mu.Lock()
	st := m.health[tunerID]
	m.mu.Unlock()
	if st != nil && st.stop != nil {
		st.stop()
	}
}

// --------------------------------------------------------------------------
// Many channels at once
// --------------------------------------------------------------------------

// MaxBulk is how many channels one bulk edit may name.
const MaxBulk = 20000

// inList runs fn over ids in chunks, since SQLite limits the arguments of one statement.
func inList(ids []int64, fn func(marks string, args []any) error) error {
	for len(ids) > 0 {
		n := min(len(ids), 500)
		args := make([]any, n)
		for i, id := range ids[:n] {
			args[i] = id
		}
		if err := fn(strings.TrimSuffix(strings.Repeat("?,", n), ","), args); err != nil {
			return err
		}
		ids = ids[n:]
	}
	return nil
}

// SetEnabled turns these channels on or off.
func (m *Manager) SetEnabled(ctx context.Context, ids []int64, on bool) error {
	return inList(ids, func(marks string, args []any) error {
		_, err := m.DB.ExecContext(ctx, `UPDATE tuner_channels SET enabled = ? WHERE id IN (`+marks+`)`, append([]any{boolInt(on)}, args...)...)
		return err
	})
}

// Renumber numbers these channels from start upwards, in the order given. The numbers are
// the owner's own, so they stay through every new channel list.
func (m *Manager) Renumber(ctx context.Context, ids []int64, start string) error {
	n, err := strconv.Atoi(strings.TrimSpace(start))
	if err != nil || n < 0 || n > 99999999-len(ids) {
		return errors.New("start from a whole number, such as 100")
	}
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.PrepareContext(ctx, `UPDATE tuner_channels SET number_set = ? WHERE id = ?`)
	if err != nil {
		return err
	}
	defer st.Close()
	for i, id := range ids {
		if _, err := st.ExecContext(ctx, strconv.Itoa(n+i), id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Merge makes the others backups of into: hidden from the guide, and tried in turn when into
// won't open. Backups of the others become backups of into, so there's only ever one level.
func (m *Manager) Merge(ctx context.Context, into int64, others []int64) error {
	c, err := m.Channel(ctx, into)
	if err != nil {
		return err
	}
	if c.MergedInto != 0 {
		return fmt.Errorf("%s is itself a backup: choose the channel it backs up", c.Name)
	}
	var ids []int64
	for _, id := range others {
		if id != into {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return errors.New("choose at least one other channel to be its backup")
	}
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	err = inList(ids, func(marks string, args []any) error {
		if _, err := tx.ExecContext(ctx, `UPDATE tuner_channels SET merged_into = ? WHERE merged_into IN (`+marks+`)`, append([]any{into}, args...)...); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE tuner_channels SET merged_into = ? WHERE id IN (`+marks+`)`, append([]any{into}, args...)...)
		return err
	})
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Unmerge brings these channels back as channels of their own.
func (m *Manager) Unmerge(ctx context.Context, ids []int64) error {
	return inList(ids, func(marks string, args []any) error {
		_, err := m.DB.ExecContext(ctx, `UPDATE tuner_channels SET merged_into = NULL WHERE id IN (`+marks+`)`, args...)
		return err
	})
}

// unorphanBackups brings back backups whose channel has gone, so a channel never stays
// hidden behind one that no longer exists.
func (m *Manager) unorphanBackups(ctx context.Context) {
	if _, err := m.DB.ExecContext(ctx, `UPDATE tuner_channels SET merged_into = NULL
		WHERE merged_into IS NOT NULL AND merged_into NOT IN (SELECT id FROM tuner_channels)`); err != nil {
		m.Log.WarnContext(ctx, "could not bring back channels whose main copy has gone", "cat", "tv", "err", err)
	}
}

// backupCounts is how many backups each channel has.
func (m *Manager) backupCounts(ctx context.Context) (map[int64]int, error) {
	rows, err := m.DB.QueryContext(ctx, `SELECT merged_into, COUNT(*) FROM tuner_channels WHERE merged_into IS NOT NULL GROUP BY merged_into`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// DupChannel is one channel of a group of likely duplicates.
type DupChannel struct {
	ID      int64  `json:"id"`
	TunerID int64  `json:"tunerId"`
	Tuner   string `json:"tuner"`
	Number  string `json:"number"`
	Name    string `json:"name"`
	Health  string `json:"health,omitempty"`
	HD      bool   `json:"hd,omitempty"`
}

// DupGroup is channels that look like the same station.
type DupGroup struct {
	Reason    string       `json:"reason"` // same guide id | same name
	Channels  []DupChannel `json:"channels"`
	Suggested int64        `json:"suggested"`
}

// maxDupGroups keeps the answer to a size a page can show.
const maxDupGroups = 500

// Duplicates finds channels, across every tuner, that look like one station: the same guide
// id, or failing that the same name once case, spaces and an "HD" are set aside. Channels
// already backing another up, and encrypted ones, are left out.
func (m *Manager) Duplicates(ctx context.Context) ([]DupGroup, error) {
	rows, err := m.DB.QueryContext(ctx, `SELECT `+channelCols+` FROM tuner_channels c JOIN tuners t ON t.id = c.tuner_id
		WHERE c.enabled = 1 AND c.merged_into IS NULL`)
	if err != nil {
		return nil, err
	}
	var list []*Channel
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if c.Encrypted == "" {
			list = append(list, c)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	byGuide := map[string][]*Channel{}
	for _, c := range list {
		if k := c.GuideKey(); k != "" {
			byGuide[strings.ToLower(k)] = append(byGuide[strings.ToLower(k)], c)
		}
	}
	taken := map[int64]bool{}
	var out []DupGroup
	add := func(reason string, g []*Channel) {
		if len(g) < 2 || len(out) >= maxDupGroups {
			return
		}
		sort.SliceStable(g, func(i, j int) bool { return dupRank(g[i]) < dupRank(g[j]) })
		d := DupGroup{Reason: reason, Suggested: g[0].ID}
		for _, c := range g {
			taken[c.ID] = true
			d.Channels = append(d.Channels, DupChannel{ID: c.ID, TunerID: c.TunerID, Tuner: c.sourceName, Number: c.Number, Name: c.Name, Health: c.Health, HD: c.HD})
		}
		out = append(out, d)
	}
	for _, k := range sortedKeys(byGuide) {
		add("same guide id", byGuide[k])
	}
	byName := map[string][]*Channel{}
	for _, c := range list {
		if k := normalizeName(c.Name); k != "" && !taken[c.ID] {
			byName[k] = append(byName[k], c)
		}
	}
	for _, k := range sortedKeys(byName) {
		add("same name", byName[k])
	}
	return out, nil
}

// dupRank puts the copy to keep first: one that works, in HD, from the first tuner added.
func dupRank(c *Channel) string {
	h := map[string]string{HealthOK: "0", "": "1", HealthFailing: "2"}[c.Health]
	return fmt.Sprintf("%s%d%012d%012d", h, 1-boolInt(c.HD), c.TunerID, c.ID)
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
