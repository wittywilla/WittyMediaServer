// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/tuner"
)

// pickyEngine makes sources whose channels open or not as their locator says: "dead"
// never opens, anything else gives a few packets. It remembers what it was asked to open.
type pickyEngine struct {
	fakeEngine
	mu     sync.Mutex
	opened []tuner.Channel
}

func (e *pickyEngine) New(context.Context, string, tuner.Config, tuner.Env) (tuner.Source, error) {
	return &pickySource{e: e}, nil
}

type pickySource struct{ e *pickyEngine }

func (s *pickySource) Info() tuner.Info { return tuner.Info{Driver: "fake", MaxStreams: 1} }
func (s *pickySource) Channels(context.Context) ([]tuner.Channel, error) {
	return nil, nil
}
func (s *pickySource) Scan(context.Context, tuner.ScanOptions, func(tuner.Progress)) ([]tuner.Channel, error) {
	return nil, nil
}
func (s *pickySource) Close() error { return nil }
func (s *pickySource) Open(_ context.Context, ch tuner.Channel) (*tuner.Stream, error) {
	s.e.mu.Lock()
	s.e.opened = append(s.e.opened, ch)
	s.e.mu.Unlock()
	if ch.Locator == "dead" {
		return nil, errors.New("the stream could not be reached")
	}
	return &tuner.Stream{Reader: io.NopCloser(bytes.NewReader(make([]byte, 188*100)))}, nil
}

// pickyLineup is two tuners, one channel on each that works and one that doesn't.
func pickyLineup(t *testing.T) (*Manager, *pickyEngine) {
	t.Helper()
	old := engine
	t.Cleanup(func() { SetEngine(old) })
	e := &pickyEngine{}
	SetEngine(e)
	m := bareManager(t)
	for _, q := range []string{
		`INSERT INTO tuners(id, driver, name, options, created_at) VALUES(1, 'fake', 'Provider A', '{}', 1), (2, 'fake', 'Provider B', '{}', 1)`,
		`INSERT INTO tuner_channels(id, tuner_id, source_id, number, name, locator, epg_id) VALUES
		   (1, 1, 'a1', '1', 'Harbour News', 'dead', 'news.test'),
		   (2, 2, 'b1', '101', 'Harbour News HD', 'ok', 'news.test'),
		   (3, 1, 'a2', '2', 'Pine Films', 'ok', ''),
		   (4, 2, 'b2', '102', 'pine films', 'dead', '')`,
	} {
		if _, err := m.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return m, e
}

func TestBackupsAreTriedWhenAChannelWontOpen(t *testing.T) {
	m, e := pickyLineup(t)
	ctx := context.Background()
	if _, err := m.Open(ctx, 1); err == nil {
		t.Fatal("a dead channel with no backups opened")
	}
	if c, _ := m.Channel(ctx, 1); c.Health != HealthFailing || c.HealthMsg == "" {
		t.Fatalf("its failure wasn't kept: %q %q", c.Health, c.HealthMsg)
	}
	if err := m.Merge(ctx, 1, []int64{2}); err != nil {
		t.Fatal(err)
	}
	o, err := m.Open(ctx, 1)
	if err != nil {
		t.Fatalf("the backup wasn't tried: %v", err)
	}
	defer o.Release()
	if o.Channel.ID != 2 {
		t.Fatalf("playing %d, want the backup", o.Channel.ID)
	}
	if last := e.opened[len(e.opened)-1]; last.Locator != "ok" {
		t.Fatalf("opened %+v", last)
	}
	// The backup is out of the guide, and counted on its main channel.
	list, _ := m.GuideChannels(ctx)
	for _, c := range list {
		if c.ID == 2 {
			t.Fatal("a backup is in the guide")
		}
	}
	chs, _ := m.Channels(ctx, 1)
	for _, c := range chs {
		if c.ID == 1 && c.Backups != 1 {
			t.Fatalf("backups counted %d", c.Backups)
		}
	}
	// One level only: what backs up a backup moves to the main channel.
	if err := m.Merge(ctx, 3, []int64{1}); err != nil {
		t.Fatal(err)
	}
	if c, _ := m.Channel(ctx, 2); c.MergedInto != 3 {
		t.Fatalf("a backup's backup points at %d", c.MergedInto)
	}
	if err := m.Merge(ctx, 2, []int64{4}); err == nil {
		t.Fatal("merged into a backup")
	}
}

func TestCheckingChannels(t *testing.T) {
	m, _ := pickyLineup(t)
	ctx := context.Background()
	if err := m.CheckChannel(ctx, 3); err != nil {
		t.Fatal(err)
	}
	if err := m.CheckChannel(ctx, 4); err == nil {
		t.Fatal("a dead channel passed")
	}
	if c, _ := m.Channel(ctx, 3); c.Health != HealthOK || c.HealthAt == 0 {
		t.Fatalf("%+v", c)
	}
	// A whole tuner, in the background.
	if err := m.StartHealth(2); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		st := m.Health(2)
		if st != nil && !st.Running {
			if st.Total != 2 || st.Done != 2 || st.OK != 1 || st.Failing != 1 {
				t.Fatalf("%+v", st)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("still running: %+v", st)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestDuplicatesAndBulkEdits(t *testing.T) {
	m, _ := pickyLineup(t)
	ctx := context.Background()
	if err := m.CheckChannel(ctx, 2); err != nil {
		t.Fatal(err)
	}
	groups, err := m.Duplicates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].Reason != "same guide id" || groups[1].Reason != "same name" {
		t.Fatalf("%+v", groups)
	}
	// The copy that works, in HD, is the one to keep.
	if groups[0].Suggested != 2 {
		t.Fatalf("suggested %d", groups[0].Suggested)
	}
	if err := m.Renumber(ctx, []int64{4, 3}, "900"); err != nil {
		t.Fatal(err)
	}
	if c, _ := m.Channel(ctx, 3); c.Number != "901" {
		t.Fatalf("renumbered to %s", c.Number)
	}
	if err := m.SetEnabled(ctx, []int64{3, 4}, false); err != nil {
		t.Fatal(err)
	}
	if list, _ := m.GuideChannels(ctx); len(list) != 2 {
		t.Fatalf("%d in the guide", len(list))
	}
}

func TestArchiveNeedsCatchup(t *testing.T) {
	m, e := pickyLineup(t)
	ctx := context.Background()
	now := time.Now().Unix()
	if _, err := m.OpenArchive(ctx, 3, now-3600, now-1800); !errors.Is(err, ErrNoArchive) {
		t.Fatalf("a channel with no archive: %v", err)
	}
	if _, err := m.DB.Exec(`UPDATE tuner_channels SET catchup_days = 2, catchup = 'shift' WHERE id = 3`); err != nil {
		t.Fatal(err)
	}
	if _, err := m.OpenArchive(ctx, 3, now-3*86400, 0); !errors.Is(err, ErrNoArchive) {
		t.Fatalf("older than the archive keeps: %v", err)
	}
	if _, err := m.OpenArchive(ctx, 3, now+600, 0); !errors.Is(err, ErrNoArchive) {
		t.Fatalf("in the future: %v", err)
	}
	o, err := m.OpenArchive(ctx, 3, now-3600, now-1800)
	if err != nil {
		t.Fatal(err)
	}
	o.Release()
	if last := e.opened[len(e.opened)-1]; last.ArchiveStart != now-3600 || last.ArchiveEnd != now-1800 || last.Catchup != "shift" || last.CatchupDays != 2 {
		t.Fatalf("the driver was asked %+v", last)
	}
}

func TestSeriesRecordingOptions(t *testing.T) {
	m, _ := pickyLineup(t)
	ctx := context.Background()
	m.Folder = func() string { return t.TempDir() }
	if _, err := m.SchedulePadded(ctx, 3, 1, "Film", "", time.Now().Unix()+60, time.Now().Unix()+3600, 61, 0); err == nil {
		t.Fatal("over an hour early")
	}
	if _, err := m.AddRuleWith(ctx, 3, 1, "Quiz", RuleOptions{Keep: -1}); err == nil {
		t.Fatal("a negative keep")
	}
	// Guide data: the same episode twice on this channel, another on the other copy of it.
	base := time.Now().Truncate(time.Hour).Unix() + 3600
	for _, q := range []struct {
		ch         string
		at         int64
		season, ep int
	}{{"pine", base, 1, 1}, {"pine", base + 86400, 1, 1}, {"pine", base + 2*86400, 1, 2}, {"other", base + 3600, 1, 3}} {
		if _, err := m.DB.Exec(`INSERT INTO epg_programmes(channel, start, stop, title, season, episode) VALUES(?,?,?,?,?,?)`, q.ch, q.at, q.at+1800, "Quiz", q.season, q.ep); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.DB.Exec(`UPDATE tuner_channels SET epg_id = 'pine' WHERE id = 3; UPDATE tuner_channels SET epg_id = 'other' WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	rule, err := m.AddRuleWith(ctx, 3, 1, "quiz", RuleOptions{NewOnly: true, PadAfter: 5})
	if err != nil {
		t.Fatal(err)
	}
	m.applyRules(ctx)
	count := func() (n int) {
		_ = m.DB.QueryRow(`SELECT COUNT(*) FROM recordings WHERE rule_id = ?`, rule.ID).Scan(&n)
		return n
	}
	if n := count(); n != 2 {
		t.Fatalf("new episodes only on its channel: %d recordings", n)
	}
	var pad int
	_ = m.DB.QueryRow(`SELECT pad_after FROM recordings WHERE rule_id = ? LIMIT 1`, rule.ID).Scan(&pad)
	if pad != 5 {
		t.Fatalf("padding %d", pad)
	}
	// Any channel brings in the episode shown elsewhere, once.
	if err := m.UpdateRule(ctx, rule.ID, RuleOptions{NewOnly: true, AnyChannel: true}); err != nil {
		t.Fatal(err)
	}
	m.applyRules(ctx)
	if n := count(); n != 3 {
		t.Fatalf("any channel: %d recordings", n)
	}
	rules, _ := m.Rules(ctx)
	if len(rules) != 1 || !rules[0].AnyChannel || !rules[0].NewOnly {
		t.Fatalf("%+v", rules)
	}
	// Both tuners stream one channel at a time: of two recordings at once on tuner 1, the one
	// that starts second finds no tuner free.
	if _, err := m.Schedule(ctx, 3, 1, "Late film", "", base+60, base+1800); err != nil {
		t.Fatal(err)
	}
	recs, _ := m.Recordings(ctx)
	conflicts := 0
	for _, r := range recs {
		if r.Conflict {
			conflicts++
		}
		if r.RuleID != nil && r.RuleTitle != "quiz" {
			t.Fatalf("rule title %q", r.RuleTitle)
		}
	}
	if conflicts != 1 {
		t.Fatalf("%d conflicts", conflicts)
	}
}

func TestSeriesKeepsTheNewest(t *testing.T) {
	m, _ := pickyLineup(t)
	ctx := context.Background()
	folder := t.TempDir()
	m.Folder = func() string { return folder }
	rule, err := m.AddRuleWith(ctx, 3, 1, "News", RuleOptions{Keep: 2})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for i := range 4 {
		p := filepath.Join(folder, "News "+string(rune('a'+i))+".ts")
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		res, err := m.DB.Exec(`INSERT INTO recordings(channel_id, title, start, stop, status, path, rule_id, created_at) VALUES(3, 'News', ?, ?, 'done', ?, ?, 1)`,
			int64(1000+i*100), int64(1050+i*100), p, rule.ID)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}
	m.keepNewest(ctx, ids[3])
	var n int
	_ = m.DB.QueryRow(`SELECT COUNT(*) FROM recordings WHERE rule_id = ?`, rule.ID).Scan(&n)
	left, _ := os.ReadDir(folder)
	if n != 2 || len(left) != 2 {
		t.Fatalf("%d rows and %d files left", n, len(left))
	}
}
