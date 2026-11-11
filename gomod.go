// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package wittymediaserver holds this module's go.mod, so the server can
// report the versions of the engines it requires (Settings, About).
package wittymediaserver

import _ "embed"

//go:embed go.mod
var GoMod string
