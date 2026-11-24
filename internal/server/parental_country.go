// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// ratingCountryChanged: the owner chose whose certificates titles keep. From now on new
// metadata keeps that country's, and the ages of titles already saved are worked out again,
// as the same letters can mean another age there. That runs in the background: a big
// library takes a few seconds.
func (s *Server) ratingCountryChanged(ctx context.Context, code, by string) {
	library.SetRatingCountry(code)
	logging.Audit(ctx, s.Log, "rating country changed", "cat", "profiles", "country", code, "by", by)
	go func() {
		if err := s.Store.RecomputeAges(context.WithoutCancel(ctx)); err != nil {
			s.Log.Warn("could not work out the titles' ages again", "cat", "profiles", "err", err)
		}
		s.forgetRules()
	}()
}
