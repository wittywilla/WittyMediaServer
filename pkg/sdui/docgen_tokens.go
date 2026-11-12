// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"fmt"
	"sort"
	"strings"
)

// tokensMarkdown is the reference's section on the theme tokens each component and card
// style is drawn with (tokens.go).
func tokensMarkdown(c *Catalogue) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f, a...) }
	pairs := func(m map[string]string) string {
		if len(m) == 0 {
			return ""
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]string, len(keys))
		for i, k := range keys {
			out[i] = k + " `" + m[k] + "`"
		}
		return strings.Join(out, ", ")
	}
	w("### Theme tokens\n\n")
	w("Every colour, typeface and corner comes from the theme: colours from `GET /api/v1/theme` `tokens` (with a node's scope `tokens` over them), corners from its `shape`, typefaces and weights from its `type`, sizes from `layout.text`. A renderer has none of its own. Each component and card style has `tokens` in the catalogue: which colour token each part is drawn in, which text role each piece of text takes, and which shape token rounds each part. [THEMES.md](THEMES.md#sdui-tokens) explains them.\n\n")
	if c.Theme == nil {
		return b.String()
	}
	w("Text roles: ")
	roles := make([]string, 0, len(c.Theme.TypeRoles))
	for r := range c.Theme.TypeRoles {
		roles = append(roles, r)
	}
	sort.Strings(roles)
	for i, r := range roles {
		if i > 0 {
			w(", ")
		}
		w("`%s` (%s family, %s weight)", r, c.Theme.TypeRoles[r].Family, c.Theme.TypeRoles[r].Weight)
	}
	w(".\n\nColour roles:\n\n")
	for _, r := range c.Theme.ColorRoles {
		w("- `%s`: %s.\n", r.Value, r.Label)
	}
	w("\nShape tokens:\n\n")
	for _, r := range c.Theme.ShapeTokens {
		w("- `%s`: %s.\n", r.Value, r.Label)
	}
	w("\n| Component | Colours | Text | Shape |\n|---|---|---|---|\n")
	for _, comp := range c.Components {
		if comp.Tokens == nil {
			continue
		}
		w("| `%s` | %s | %s | %s |\n", comp.Type, pairs(comp.Tokens.Colors), pairs(comp.Tokens.Type), pairs(comp.Tokens.Shape))
	}
	w("\n| Card style | Colours | Text | Shape |\n|---|---|---|---|\n")
	for _, s := range c.CardStyles {
		if s.Tokens == nil {
			continue
		}
		w("| `%s` | %s | %s | %s |\n", s.Style, pairs(s.Tokens.Colors), pairs(s.Tokens.Type), pairs(s.Tokens.Shape))
	}
	w("\n")
	return b.String()
}
