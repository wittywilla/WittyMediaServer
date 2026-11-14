// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// background runs fn on a goroutine of its own, as the server's own work: a request started it,
// and nobody waits for the answer. Close tells it to stop, by ending ctx, and waits for it, so the
// database can close with nothing still using it. After Close nothing starts, and this says false.
func (s *Server) background(where string, fn func(ctx context.Context), attrs ...any) bool {
	s.bgMu.Lock()
	defer s.bgMu.Unlock()
	if s.bgClosed {
		return false
	}
	if s.bgCtx == nil {
		s.bgCtx, s.bgStop = context.WithCancel(context.Background())
	}
	ctx := s.bgCtx
	s.bgWork.Add(1)
	logging.Go(where, func() {
		defer s.bgWork.Done()
		fn(ctx)
	}, attrs...)
	return true
}

// Close ends the server's own background work and waits for it, then Live TV's, then the activity
// still on its way to the database. The database is closed after it, by whoever opened it. The
// server calls it once it has stopped answering; a test, when it ends, so nothing writes into its
// folder as that is removed. It can be called more than once.
func (s *Server) Close() {
	s.bgMu.Lock()
	s.bgClosed = true
	if s.bgStop != nil {
		s.bgStop()
	}
	s.bgMu.Unlock()
	s.bgWork.Wait()
	if s.raArchive != nil {
		s.raArchive.Close()
	}
	if s.TV != nil {
		s.TV.Close() // the server closed it already as it stopped; a test's is closed here
	}
	s.Tasks.Settle()
}
