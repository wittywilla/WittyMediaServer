// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What WittyPlay Online's pieces work out, with nothing of the page in it (playModel.test.mjs).

// Presence, from the badge the source sends (docs/GAMING-API.md): playing and online are the same bright dot, away is amber, the rest is dim.
export const presenceClass = (badge: string): 'on' | 'away' | 'off' => ({ playing: 'on', online: 'on', away: 'away' } as Record<string, 'on' | 'away'>)[badge.trim().toLowerCase()] || 'off'

// The item the detail panel is about: the one focused, else the first.
export function focusedOf<T extends { id: string }>(items: T[], focusId: string): T | undefined {
  return items.find((c) => c.id === focusId) || items[0]
}
