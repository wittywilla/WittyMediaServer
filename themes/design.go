// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package themes

import _ "embed"

// TokensJSON is themes/tokens.json, the design tokens as data (written by
// scripts/design-tokens.py from web/src/styles.css). The theme engine serves the parts
// clients need from it (internal/themes/design.go).
//
//go:embed tokens.json
var TokensJSON []byte
