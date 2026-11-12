// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// TV mode's tabs and what Back and the bumpers do there, as plain rules with nothing from the page,
// so they can be tested (nav.test.mjs). docs/plans/TV-MODE.md has the contract; TvFrame.tsx uses them.

// A tab of the top bar. `screen` names one of the app's own screens (profiles: You, with the profile's picture).
export type TabId = string
export type TvTab = { id: TabId; path: string; label: string; icon?: string; screen?: string }

// The top bar as TV mode has it against a server that doesn't describe its own menu: five on the left, You
// pinned to the right end of the same row. A server with server-driven pages sends the bar (tabsFromMenu).
export const TV_TABS: TvTab[] = [
  { id: 'home', path: '/', label: 'Home', icon: 'home' },
  { id: 'libraries', path: '/libraries', label: 'Libraries', icon: 'library' },
  { id: 'games', path: '/games', label: 'Games', icon: 'game' },
  { id: 'live', path: '/tv', label: 'Live TV', icon: 'tv' },
  { id: 'search', path: '/search', label: 'Search', icon: 'search' },
  { id: 'you', path: '/you', label: 'You', screen: 'profiles' },
]

// tabsFromMenu: the bar from a server's menu for a television (GET /sdui/navigation): its primary entries in
// order, then More when the menu places anything there (moreTabs lists them on its own screen), then the ones
// placed at the end. Settings is reached from You on a television, as it always was, unless the menu has no You
// to reach it from.
export const MORE_PATH = '/more'
export function tabsFromMenu(items: MenuEntry[]): TvTab[] {
  const usable = usableEntries(items)
  const tab = (it: MenuEntry): TvTab => ({ id: it.id, path: it.to, label: it.label, icon: it.icon, screen: it.screen })
  const more = usable.filter((it) => it.placement === 'more' && it.id !== 'search')
  const bar = usable.filter((it) => (it.placement !== 'end' && it.placement !== 'more') || it.id === 'search').map(tab)
  if (more.length) bar.push({ id: 'more', path: MORE_PATH, label: 'More', icon: 'more' })
  return [...bar, ...usable.filter((it) => it.placement === 'end' && it.id !== 'search').map(tab)]
}

// moreTabs: what the More screen lists, in the menu's order.
export function moreTabs(items: MenuEntry[]): TvTab[] {
  return usableEntries(items).filter((it) => it.placement === 'more' && it.id !== 'search').map((it) => ({ id: it.id, path: it.to, label: it.label, icon: it.icon, screen: it.screen }))
}

type MenuEntry = { id: string; label: string; to: string; icon?: string; placement?: string; screen?: string }
function usableEntries(items: MenuEntry[]): MenuEntry[] {
  const hasYou = items.some((it) => it.screen === 'profiles')
  return items.filter((it) => it.to.startsWith('/') && !(it.screen === 'settings' && hasYou))
}

// tabRootOf: the tab whose first screen this is, or null for a screen opened from a tab, which has the
// whole screen and no bar.
export function tabRootOf(pathname: string, tabs: TvTab[] = TV_TABS): TabId | null {
  const path = pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname
  return tabs.find((t) => t.path === path)?.id ?? null
}

// stepTab: the tab beside this one, for LB and RB; null at either end, which doesn't wrap round.
export function stepTab(from: TabId, step: -1 | 1, tabs: TvTab[] = TV_TABS): TabId | null {
  const i = tabs.findIndex((t) => t.id === from)
  return i < 0 ? null : tabs[i + step]?.id ?? null
}

export const tabPath = (id: TabId, tabs: TvTab[] = TV_TABS) => (tabs.find((t) => t.id === id) || tabs[0]).path

// What Back does:
//   history  a screen opened from a tab: back to where it was opened
//   bar      inside a tab: up to the bar, on the chosen tab's pill
//   home     the bar, on another tab: to Home, on Home's pill
//   leave    the bar, on Home: out to whatever is around the page
export type BackStep = 'history' | 'bar' | 'home' | 'leave'

// `first` is the bar's first tab, where it isn't called home.
export function backStep(at: { tab: TabId | null; inBar: boolean; first?: TabId }): BackStep {
  if (!at.tab) return 'history'
  if (!at.inBar) return 'bar'
  return at.tab === (at.first ?? 'home') ? 'leave' : 'home'
}

// The keys TV mode answers to, beside the arrows and OK that the page's own navigation has.
export type TvKey = 'back' | 'prevTab' | 'nextTab' | null

// The Back button, as each kind of remote reports it (webOS and Tizen send a key code with no name), Escape,
// which Plasma Bigscreen and HDMI-CEC remotes send for Back, and Backspace outside a text field.
export function tvKey(e: { key: string; keyCode?: number }, typing: boolean): TvKey {
  if (e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack' || e.keyCode === 461 || e.keyCode === 10009) return 'back'
  if (e.key === 'Backspace') return typing ? null : 'back'
  if (typing) return null
  if (e.key === 'PageUp') return 'prevTab'
  if (e.key === 'PageDown') return 'nextTab'
  return null
}
