// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// TV mode's tabs and what Back and the bumpers do there, as plain rules with nothing from the page,
// so they can be tested (nav.test.mjs). docs/plans/TV-MODE.md has the contract; TvFrame.tsx uses them.

export type TabId = 'home' | 'libraries' | 'games' | 'live' | 'search' | 'you'

// The top bar, in its order: five on the left, You pinned to the right end of the same row.
export const TV_TABS: { id: TabId; path: string; label: string }[] = [
  { id: 'home', path: '/', label: 'Home' },
  { id: 'libraries', path: '/libraries', label: 'Libraries' },
  { id: 'games', path: '/games', label: 'Games' },
  { id: 'live', path: '/tv', label: 'Live TV' },
  { id: 'search', path: '/search', label: 'Search' },
  { id: 'you', path: '/you', label: 'You' },
]

// tabRootOf: the tab whose first screen this is, or null for a screen opened from a tab, which has the
// whole screen and no bar.
export function tabRootOf(pathname: string): TabId | null {
  const path = pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname
  return TV_TABS.find((t) => t.path === path)?.id ?? null
}

// stepTab: the tab beside this one, for LB and RB; null at either end, which doesn't wrap round.
export function stepTab(from: TabId, step: -1 | 1): TabId | null {
  const i = TV_TABS.findIndex((t) => t.id === from)
  return TV_TABS[i + step]?.id ?? null
}

export const tabPath = (id: TabId) => TV_TABS.find((t) => t.id === id)!.path

// What Back does:
//   history  a screen opened from a tab: back to where it was opened
//   bar      inside a tab: up to the bar, on the chosen tab's pill
//   home     the bar, on another tab: to Home, on Home's pill
//   leave    the bar, on Home: out to whatever is around the page
export type BackStep = 'history' | 'bar' | 'home' | 'leave'

export function backStep(at: { tab: TabId | null; inBar: boolean }): BackStep {
  if (!at.tab) return 'history'
  if (!at.inBar) return 'bar'
  return at.tab === 'home' ? 'leave' : 'home'
}

// The keys TV mode answers to, beside the arrows and OK that the page's own navigation has.
export type TvKey = 'back' | 'prevTab' | 'nextTab' | 'options' | null

// The Back button, as each kind of remote reports it (webOS and Tizen send a key code with no name), Escape,
// which Plasma Bigscreen and HDMI-CEC remotes send for Back, and Backspace outside a text field.
export function tvKey(e: { key: string; keyCode?: number }, typing: boolean): TvKey {
  if (e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack' || e.keyCode === 461 || e.keyCode === 10009) return 'back'
  if (e.key === 'Backspace') return typing ? null : 'back'
  if (typing) return null
  if (e.key === 'PageUp') return 'prevTab'
  if (e.key === 'PageDown') return 'nextTab'
  if (e.key === 'ContextMenu') return 'options'
  return null
}
