// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The rules for the four zones, with nothing from the page (zonesModel.test.mjs).

export type Zone = 'bar' | 'stage' | 'dock' | 'layer'

// cycleZone: where the zone key (Start, F6) goes from here: Stage to the Dock, the Dock to the Bar, the Bar back
// to Stage, skipping a zone this screen doesn't have. From nowhere it goes to the Stage.
export function cycleZone(from: Zone | null, present: Zone[]): Zone | null {
  const order: Zone[] = ['stage', 'dock', 'bar']
  const have = order.filter((z) => present.includes(z))
  if (!have.length) return null
  const i = from ? order.indexOf(from) : -1
  for (let n = 1; n <= order.length; n++) {
    const z = order[(i + n + order.length) % order.length]
    if (have.includes(z)) return z
  }
  return null
}

// Back unwinds the screen from the inside out (section 3): a Layer first, then the Dock, then the Stage up to the
// Bar, then the Bar to the first tab, then out. A screen opened from a tab has no Bar and goes back in history.
export type Unwind = 'layer' | 'dock' | 'bar' | 'home' | 'leave' | 'history'
export function backLadder(at: { layer: boolean; dock: boolean; tab: string | null; inBar: boolean; first?: string }): Unwind {
  if (at.layer) return 'layer'
  if (at.dock) return 'dock'
  if (!at.tab) return 'history'
  if (!at.inBar) return 'bar'
  return at.tab === (at.first ?? 'home') ? 'leave' : 'home'
}
