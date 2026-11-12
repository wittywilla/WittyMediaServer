// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What WittyPlay Online's pieces work out, with nothing of the page in it (playModel.test.mjs).

// Presence, from the badge the source sends (docs/GAMING-API.md): playing and online are the same bright dot, away is amber, the rest is dim.
export const presenceClass = (badge: string): 'on' | 'away' | 'off' => ({ playing: 'on', online: 'on', away: 'away' } as Record<string, 'on' | 'away'>)[badge.trim().toLowerCase()] || 'off'

// The item the detail panel is about: the one focused, else the first.
export function focusedOf<T extends { id: string }>(items: T[], focusId: string): T | undefined {
  return items.find((c) => c.id === focusId) || items[0]
}

// The dot for a badge: the catalogue's own tones for the person style (badgeTones: ok, warn, muted) when the card carries them, else the
// names this app knows.
export function dotFor(badge: string, tones?: Record<string, unknown>): 'on' | 'away' | 'off' {
  const tone = tones?.[badge.trim()] ?? tones?.[badge.trim().toLowerCase()]
  if (tone === 'ok') return 'on'
  if (tone === 'warn') return 'away'
  if (typeof tone === 'string') return 'off'
  return presenceClass(badge)
}
