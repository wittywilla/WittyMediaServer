// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build race

package modules

const raceEnabled = true // the race detector maps far more memory than a sandboxed helper may have
