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

// The consoles of a games row (each card's props.console), most games first, for the bar over it: none when the row holds fewer than
// two, where a bar would choose nothing.
export function consolesOf(items: { props?: Record<string, unknown> }[]): { name: string; count: number }[] {
  const n = new Map<string, number>()
  for (const it of items) { const c = it.props?.console; if (typeof c === 'string' && c) n.set(c, (n.get(c) || 0) + 1) }
  if (n.size < 2) return []
  return [...n].map(([name, count]) => ({ name, count })).sort((a, b) => b.count - a.count || a.name.localeCompare(b.name))
}

// The row's cards on one console, or all of them with none chosen.
export const onConsole = <T extends { props?: Record<string, unknown> }>(items: T[], name: string): T[] => (name ? items.filter((it) => it.props?.console === name) : items)
