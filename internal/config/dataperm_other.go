// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package config

// makePrivate: the folder's mode, 0750, already says it.
func makePrivate(string) error { return nil }
