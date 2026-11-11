// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !linux && !windows

package modules

// forkLimits is what this system can really take away from a fork's helper.
const forkLimits = "This system can't box in a fork's helper, so forks aren't let in here. Settings, Modules says what limits a module has."
