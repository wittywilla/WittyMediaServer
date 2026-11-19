// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package config

// systemDocuments: other systems keep Documents in the home folder, or say otherwise in
// user-dirs.dirs, which DocumentsDir reads.
func systemDocuments() string { return "" }
