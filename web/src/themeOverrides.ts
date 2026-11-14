// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// The server's theme engine (GET /api/theme, docs/THEMES.md "The theme engine") says which
// colours an administrator changed: for the whole interface, and for the pages that belong
// to one module. This is the part of it that needs no page: what to set, and where a page
// belongs. theme.ts puts the result on <html>.

import type { StyledPart } from './brandStyle.ts'
export type BrandMarkData = { kind: 'letter' | 'initials' | 'text' | 'image'; text?: string; imageUrl?: string; shown: string }
export type RequiredValue = { value: string; by?: string; at?: string; scope: string }
export type SceneLayer = { type: 'gradient' | 'radial' | 'shape' | 'image'; angle?: number; stops?: { at: number; color: string }[]; x?: number; y?: number; w?: number; h?: number; color?: string; fade?: number; kind?: 'circle' | 'hill'; size?: number; file?: string }
export type Finish = { glass?: { hi: string; lo: string; edge: string; shadow: string }; gloss?: string[] }
export type Look = { finish?: Finish; scene?: { layers?: SceneLayer[] }; experience: string; theme: string; requiredExperience?: RequiredValue; requiredTheme?: RequiredValue; tvTheme?: string; effects?: Record<string, string>; shape?: { radius?: number; radiusMultiple?: number; cardSet?: string }; type?: string; density?: string; motion?: Record<string, unknown> }
export type ResolvedTheme = {
  id: string
  source: 'server' | 'user' | 'required' | 'trial'
  // The finished look: the experience and theme in force and what an administrator requires (docs/THEMES.md).
  look?: Look
  // The server's name as the theme engine draws it; the mark is the badge for small spaces (the rail's header).
  brand?: { mark?: BrandMarkData; parts?: StyledPart[]; warnings?: { text: string; surface: string; ratio: number; need: number }[] }
  overrides: Record<string, string>
  // The whole palette.
  tokens: Record<string, string>
  // A module's: the administrator's overrides alone, and (docs/SDUI-MODULES.md, "Theme") the
  // whole palette with the module's own colours and those overrides over it.
  modules: Record<string, { overrides: Record<string, string>; tokens?: Record<string, string>; personal?: Record<string, string> }>
  // The profile's own colours (PUT /theme/personal): over everything, everywhere, whatever
  // theme this device shows (docs/THEMES.md, "A profile's own colours").
  personal?: Record<string, string>
  // Text an override left hard to read, for the settings page: scope is "global" or
  // "module:<id>", variant the look ("dark" or "light") when the theme has two.
  warnings?: { scope: string; variant?: string; text: string; surface: string; ratio: number; need: number }[]
}

// A colour as the server sends it: #RRGGBB or #RRGGBBAA, and nothing else gets near CSS.
const COLOUR = /^#[0-9a-f]{6}([0-9a-f]{2})?$/i

// The one colour whose CSS variable isn't its name respelled (scripts/design-tokens.py has the same table).
const VAR_NAMES: Record<string, string> = { accentHover: '--accent-2' }

// tokenVar is a theme.json colour name as its CSS variable: accentFg is --accent-fg, bg2 is --bg-2.
export const tokenVar = (name: string) => VAR_NAMES[name] || '--' + name.replace(/([a-z])([A-Z0-9])/g, '$1-$2').toLowerCase()

// These two also exist as "R G B", for see-through shades made from them.
const RGB_TWIN: Record<string, string> = { bg: '--bg-rgb', accent: '--accent-rgb' }

// moduleForPath is the module a page belongs to, "" for the server's own pages.
export function moduleForPath(path: string): string {
  const m = /^\/m\/([a-z][a-z0-9_-]*)\//.exec(path)
  if (m) return m[1]
  if (path === '/tv' || path.startsWith('/tv/')) return 'wittytv'
  if (path.startsWith('/game/') || path === '/controllers' || path === '/couch' || path.startsWith('/couch/') || path.startsWith('/rigs/') || path.startsWith('/big')) return 'wittyplay'
  return ''
}

// overrideDeclarations are the CSS variables to set: the global overrides when the theme on
// screen is the one they were chosen for, then the module's over them when the page is a
// module's. Anything that isn't a plain colour is left out.
export function overrideDeclarations(t: ResolvedTheme | null, module: string, globalApplies: boolean): [string, string][] {
  if (!t) return []
  const merged: Record<string, string> = {}
  if (globalApplies) Object.assign(merged, t.overrides || {})
  if (module) {
    // The colours the module itself would like: where its palette differs from everyone's.
    const own = t.modules?.[module]?.tokens || {}
    for (const [name, value] of Object.entries(own)) if (t.tokens[name] !== value) merged[name] = value
    Object.assign(merged, t.modules?.[module]?.overrides || {})
  }
  Object.assign(merged, t.personal || {})
  if (module) Object.assign(merged, t.modules?.[module]?.personal || {})
  const out: [string, string][] = []
  for (const [name, value] of Object.entries(merged)) {
    if (!/^[a-z][A-Za-z0-9]*$/.test(name) || !COLOUR.test(value)) continue
    out.push([tokenVar(name), value])
    if (RGB_TWIN[name]) out.push([RGB_TWIN[name], [1, 3, 5].map((i) => parseInt(value.slice(i, i + 2), 16)).join(' ')])
  }
  return out
}

// A finish colour from a theme file (an imported one included): #hex or a plain rgb/rgba/hsl/hsla, nothing that could fetch or break out.
const FINISH_COLOUR = /^(#[0-9a-f]{3,8}|(?:rgb|hsl)a?\(\s*[\d.]+%?(?:\s*[,/]?\s*[\d.]+%?){2,3}\s*\))$/i
export const safeColour = (c: unknown): string => (typeof c === 'string' && FINISH_COLOUR.test(c.trim()) ? c.trim() : '')

// themeQuery is what GET /theme is asked with: this device's name for its own look, and the theme this
// device wears when it chose one, so the server answers with that theme's scene and finish rather than
// the server's (#S8: Paper on a dark sky). "auto" asks for the one the device shows now; a theme made on
// this device ("custom") isn't the server's, so nothing is asked for it and it draws no scene.
export function themeQuery(device: string, own: string, prefersLight: boolean): string {
  const q = new URLSearchParams()
  if (device) q.set('device', device)
  const theme = own === 'auto' ? (prefersLight ? 'light' : 'default') : own
  if (theme && theme !== 'custom') q.set('theme', theme)
  const s = q.toString()
  return s ? '?' + s : ''
}
