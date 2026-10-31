package library

import (
	"context"
	"sync/atomic"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// Filling in network facts: shows scanned before channels existed, or whose
// metadata came from a file, may not know their network or the slot they
// aired in. This asks the metadata services about only those shows, without
// rescanning anything. A show that was asked and has no answer is not asked
// again (its air_time goes from NULL to empty).

// FactsProgress is how far a fill has got.
type FactsProgress struct {
	Running bool  `json:"running"`
	Total   int64 `json:"total"`
	Done    int64 `json:"done"`
	Found   int64 `json:"found"`
}

var factsRun struct {
	on                 atomic.Bool
	total, done, found atomic.Int64
}

// NetworkFactsProgress reads the state of the current or last fill.
func NetworkFactsProgress() FactsProgress {
	return FactsProgress{Running: factsRun.on.Load(), Total: factsRun.total.Load(), Done: factsRun.done.Load(), Found: factsRun.found.Load()}
}

// FillNetworkFacts starts the fill in the background, unless one is running.
func (s *Scanner) FillNetworkFacts() bool {
	if !factsRun.on.CompareAndSwap(false, true) {
		return false
	}
	logging.Go("network facts", func() {
		defer factsRun.on.Store(false)
		if err := s.fillNetworkFacts(context.Background()); err != nil {
			s.Log.Warn("could not fill in network facts", "err", err)
		}
	}, "cat", "library")
	return true
}

func (s *Scanner) fillNetworkFacts(ctx context.Context) error {
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT id FROM shows WHERE missing = 0 AND air_time IS NULL ORDER BY sort_title COLLATE NOCASE`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	factsRun.total.Store(int64(len(ids)))
	factsRun.done.Store(0)
	factsRun.found.Store(0)
	src := s.tvSource()
	for _, id := range ids {
		sh, err := s.Store.Show(ctx, id, 0)
		if err == nil {
			if m := s.lookupFacts(ctx, sh, src); m != nil {
				if s.saveFacts(ctx, sh, m) {
					factsRun.found.Add(1)
				}
			} else {
				_, _ = s.Store.DB.ExecContext(ctx, `UPDATE shows SET air_time = '' WHERE id = ? AND air_time IS NULL`, id)
			}
		}
		factsRun.done.Add(1)
	}
	return nil
}

func (s *Scanner) lookupFacts(ctx context.Context, sh *Show, src SeriesSource) *SeriesMeta {
	if t := s.sourceNamed(sh.Provider); t != nil && sh.ProviderID != "" {
		m, err := t.Series(ctx, sh.ProviderID)
		if err == nil {
			return m
		}
		s.Log.DebugContext(ctx, "network facts lookup failed", "source", t.Name(), "show", sh.Title, "err", err)
	}
	if src == nil || sh.Provider != "" {
		return nil
	}
	m, err := src.FindSeries(ctx, sh.Title, sh.Year)
	if err != nil {
		s.Log.DebugContext(ctx, "network facts lookup failed", "source", src.Name(), "show", sh.Title, "err", err)
		return nil
	}
	return m
}

// saveFacts fills only what is missing: an owner's edits and NFO files win.
func (s *Scanner) saveFacts(ctx context.Context, sh *Show, m *SeriesMeta) bool {
	network := sh.Network
	if network == "" {
		network = m.Network
	}
	if _, err := s.Store.DB.ExecContext(ctx, `UPDATE shows SET network = ?, air_days = ?, air_time = ? WHERE id = ?`, network, joinList(m.AirDays), m.AirTime, sh.ID); err != nil {
		return false
	}
	return network != sh.Network || m.AirTime != ""
}
