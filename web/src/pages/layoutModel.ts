// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What the layout editor works out, with nothing of the page in it (layoutModel.test.mjs): which zone of the picture a
// region is drawn in, where it may be dropped, and the presets.

export type Region = { id: string; label: string; positions?: string[]; locked?: boolean; styles?: string[] }
export type Placement = { position?: string; visible?: boolean; style?: string }
export type Placements = Record<string, Placement>
export type Zone = 'top' | 'start' | 'end' | 'bottom' | 'main'

export const DEVICES = [
  { id: 'tv', label: 'TV', ratio: '16 / 9' },
  { id: 'desktop', label: 'Desktop', ratio: '16 / 10' },
  { id: 'tablet', label: 'Tablet', ratio: '4 / 3' },
  { id: 'phone', label: 'Phone', ratio: '9 / 16' },
  { id: 'headset', label: 'Headset', ratio: '16 / 9' },
] as const
export type DeviceId = typeof DEVICES[number]['id']

// The form factor the server knows this window as, as one of the five buttons ("web" is a desktop).
export const deviceOf = (formFactor: string): DeviceId => (formFactor === 'web' ? 'desktop' : (DEVICES.find((d) => d.id === formFactor)?.id || 'desktop'))

const SIDE_NAV = new Set(['start', 'end'])
// The placement a region has: the experience's and the edit over it.
export const merged = (base: Placements, edit: Placements, id: string): Placement => ({ ...(base[id] || {}), ...(edit[id] || {}) })

// zoneOf: where a region's tile is drawn for a position. 'nav' is wherever the menu is, 'dock' wherever the Dock is.
export function zoneOf(id: string, position: string | undefined, nav: string | undefined, dock: string | undefined): Zone {
  const p = position || ''
  const at = (q: string | undefined): Zone => (q === 'start' ? 'start' : q === 'end' ? 'end' : q === 'bottom' ? 'bottom' : 'top')
  if (p === 'nav') return at(nav || 'start')
  if (p === 'dock') return dock === 'side' ? 'end' : dock === 'bottom' ? 'bottom' : 'top'
  switch (id) {
    case 'hero': return 'main'
    case 'legend': return 'bottom'
    case 'dock': return p === 'side' ? 'end' : p === 'bottom' ? 'bottom' : 'top'
    case 'brand': return p === 'start' ? 'start' : 'top'
    case 'nav': return at(p || 'start')
    default: return p === 'floating' ? 'main' : SIDE_NAV.has(p) ? at(p) : 'top'
  }
}

// Presets: whole arrangements. Each says what it sets; positions a region doesn't have are left alone.
export type Preset = { id: string; label: string; set: Placements }
export const LAYOUT_PRESETS: Preset[] = [
  { id: 'left', label: 'Menu on the left', set: { nav: { position: 'start' }, brand: { position: 'start' }, search: { position: 'nav' }, profile: { position: 'nav' } } },
  { id: 'top', label: 'Menu on top', set: { nav: { position: 'top' }, brand: { position: 'top' }, search: { position: 'top' }, profile: { position: 'top' } } },
  { id: 'right', label: 'Menu on the right', set: { nav: { position: 'end' }, brand: { position: 'top' }, search: { position: 'top' }, profile: { position: 'nav' } } },
  { id: 'bottom', label: 'Menu at the bottom', set: { nav: { position: 'bottom' }, brand: { position: 'top' }, search: { position: 'top' }, profile: { position: 'top' } } },
  { id: 'calm', label: 'Calm: no clock or key legend', set: { clock: { visible: false }, legend: { visible: false } } },
  { id: 'dockbottom', label: 'Controls at the bottom', set: { dock: { position: 'bottom' }, clock: { position: 'dock' } } },
]

export function applyPreset(regions: Region[], edit: Placements, p: Preset): Placements {
  const out: Placements = { ...edit }
  for (const [id, pl] of Object.entries(p.set)) {
    const r = regions.find((x) => x.id === id)
    if (!r) continue
    const next: Placement = { ...(out[id] || {}) }
    if (pl.position !== undefined && r.positions?.includes(pl.position)) next.position = pl.position
    if (pl.visible !== undefined && !(r.locked && pl.visible === false)) next.visible = pl.visible
    if (pl.style !== undefined && r.styles?.includes(pl.style)) next.style = pl.style
    out[id] = next
  }
  return out
}

// What is different from the look's own, in a few words for the Save button's note.
export const changedRegions = (edit: Placements) => Object.entries(edit).filter(([, p]) => p.position !== undefined || p.visible !== undefined || p.style !== undefined).map(([id]) => id)
