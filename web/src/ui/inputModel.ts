// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The one input model (docs/plans/DESIGN-2.md section 3) as plain rules with nothing from the page, so they can
// be tested (inputModel.test.mjs) and copied into the apps: which action a key press is, which input the person
// is using right now, and the key legend for it. tv.ts answers the actions; ui/KeyLegend.tsx draws the legend.

export type Action = 'layout' | 'select' | 'back' | 'options' | 'search' | 'tabPrev' | 'tabNext' | 'pageUp' | 'pageDown' | 'top' | 'dock' | 'help'
export type InputKind = 'keys' | 'pad' | 'remote' | 'touch'
export type PadFamily = 'xbox' | 'playstation' | 'nintendo' | 'generic'

type KeyEvent = { key: string; keyCode?: number; ctrlKey?: boolean; metaKey?: boolean; shiftKey?: boolean; altKey?: boolean }

// shortcut: what a key press means in the app, or null for none (the page's own handling, such as arrows and
// Enter, takes it). Letters and punctuation never count while a field has the keys, except Ctrl or Cmd+K,
// which is search from anywhere.
export function shortcut(e: KeyEvent, typing: boolean): Action | null {
  const mod = !!(e.ctrlKey || e.metaKey)
  if (mod && !e.altKey) {
    if (e.key.toLowerCase() === 'k') return 'search'
    if (e.shiftKey && e.key.toLowerCase() === 'l') return 'layout'
    if (e.key === '[') return 'tabPrev'
    if (e.key === ']') return 'tabNext'
    return null
  }
  if (e.altKey) return e.key === 'ArrowLeft' ? 'back' : null
  if (e.key === 'ContextMenu' || (e.key === 'F10' && e.shiftKey)) return 'options'
  if (e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack' || e.keyCode === 461 || e.keyCode === 10009) return 'back'
  if (e.key === 'F6') return 'dock'
  if (typing) return null
  switch (e.key) {
    case '/': return 'search'
    case '?': return 'help'
    case '.': return 'options'
    case 'Backspace': return 'back'
    case 'Home': return 'top'
    case 'PageUp': return 'pageUp'
    case 'PageDown': return 'pageDown'
  }
  return null
}

// nextKind: which input is in use after an event of this sort. A pad or a touch is believed at once; a key
// press is a remote when the screen is set up for across the room and the key is one a remote has.
export type InputEvent = 'key' | 'pad' | 'pointer' | 'touch'
export function nextKind(prev: InputKind, ev: InputEvent, o: { tenFoot?: boolean; key?: string } = {}): InputKind {
  if (ev === 'pad') return 'pad'
  if (ev === 'touch') return 'touch'
  if (ev === 'pointer') return prev === 'touch' ? 'touch' : prev === 'pad' || prev === 'remote' ? 'keys' : prev
  // A key: a pad's own presses arrive as keys too, from tv.ts, and must not turn it into a keyboard.
  if (prev === 'pad' && o.key !== undefined && PAD_ONLY.has(o.key)) return 'pad'
  return o.tenFoot ? 'remote' : 'keys'
}
const PAD_ONLY = new Set(['Ctrl+[', 'Ctrl+]'])

// A name for a controller by its id, to pick the right glyphs.
export function padFamily(id: string): PadFamily {
  if (/054c|dualshock|dualsense|playstation|wireless controller \(standard gamepad vendor: 054c/i.test(id)) return 'playstation'
  if (/nintendo|pro controller|joy-?con|vendor: 057e|^0?57e-/i.test(id)) return 'nintendo'
  if (/xbox|045e|xinput|microsoft/i.test(id)) return 'xbox'
  return 'generic'
}

export type LegendItem = { action: Action; label: string; glyph: string }

const PAD_GLYPHS: Record<PadFamily, Record<string, string>> = {
  xbox: { select: 'A', back: 'B', options: 'X', search: 'Y', tab: 'LB / RB', page: 'LT / RT', top: 'R3', dock: 'Menu', help: 'View + Menu' },
  playstation: { select: 'Cross', back: 'Circle', options: 'Square', search: 'Triangle', tab: 'L1 / R1', page: 'L2 / R2', top: 'R3', dock: 'Options', help: 'Create + Options' },
  nintendo: { select: 'A', back: 'B', options: 'Y', search: 'X', tab: 'L / R', page: 'ZL / ZR', top: 'R stick', dock: '+', help: '- and +' },
  generic: { select: 'A', back: 'B', options: 'X', search: 'Y', tab: 'LB / RB', page: 'LT / RT', top: 'R3', dock: 'Start', help: 'Back + Start' },
}
const KEY_GLYPHS: Record<string, string> = { select: 'Enter', back: 'Esc', options: '.', search: '/', tab: 'Ctrl [ ]', page: 'PgUp / PgDn', top: 'Home', dock: 'F6', help: '?' }
const REMOTE_GLYPHS: Record<string, string> = { select: 'OK', back: 'Back', options: 'Hold OK', search: 'Search', tab: 'CH +/-', page: 'CH +/-', top: 'Up Up', dock: 'Menu', help: 'Hold Menu' }

const ORDER: [string, string, Action][] = [
  ['select', 'Select', 'select'], ['back', 'Back', 'back'], ['options', 'Options', 'options'], ['search', 'Search', 'search'],
  ['tab', 'Tabs', 'tabNext'], ['page', 'Page', 'pageDown'], ['dock', 'Dock', 'dock'], ['help', 'Keys', 'help'],
]

// legend: the hint row for this input, in a fixed order, so the same thing is always in the same place. Touch has
// none: nothing here shows without a keyboard, pad or remote.
export function legend(kind: InputKind, family: PadFamily = 'generic', o: { scope?: 'screen' | 'list' } = {}): LegendItem[] {
  if (kind === 'touch') return []
  const glyphs = kind === 'pad' ? PAD_GLYPHS[family] : kind === 'remote' ? REMOTE_GLYPHS : KEY_GLYPHS
  return ORDER
    // A list is paged and has nothing to tab through; a screen has tabs, and its Dock.
    .filter(([id]) => !(id === 'page' && o.scope !== 'list'))
    .map(([id, label, action]) => ({ action, label, glyph: glyphs[id] }))
}
