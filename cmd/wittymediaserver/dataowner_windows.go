// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

// dataOwnerOK has nothing to check on Windows, where new files take the folder's permissions.
func dataOwnerOK(string) error { return nil }

func ownerOf(string) string { return "" }
