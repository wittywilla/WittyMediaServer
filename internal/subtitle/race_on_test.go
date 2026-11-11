// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build race

package subtitle

// The race detector runs code 15 to 30 times slower, so time limits stretch to match.
const timeScale = 20
