// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import "context"

// ApplyShowInfoForTest lets the server's tests run what a scan does with show.wittyinfo.
func (s *Scanner) ApplyShowInfoForTest(ctx context.Context, sh *Show) { s.applyShowInfo(ctx, sh) }
