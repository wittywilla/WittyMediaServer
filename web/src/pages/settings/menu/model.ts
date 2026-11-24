// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { partFor, screenFor, type TvLevel, type TvLevelTable } from '../../../tvmode/levels.ts'

// The settings menu (pkg/sdui/data/settingsmenu.json, docs/SETTINGS-MENU.md): the Android phone app's You page,
// which the web, its TV mode, the desktop app and every other client draw the same. These are its rules with
// nothing from the page, so they can be tested (model.test.mjs); table.ts holds the file itself.

export type MenuRow = {
  id: string; key: string; title: string; icon: { android: string; apple: string; web: string }; tint: string
  native?: string; path?: string; sdui?: boolean; part?: string; when?: string[]; value?: string; summary?: string
}
export type MenuGroup = { id: string; key: string; title: string; rows: MenuRow[] }
export type MenuTable = { note: string; conditions: string[]; values: string[]; groups: MenuGroup[] }

// Who is looking and on what, as far as the rows care: each condition a row may name.
export type MenuWho = {
  admin: boolean; adult: boolean; tv: boolean; twoStep: boolean; games: boolean
  // The page is inside one of the apps (the desktop app): rows for the app itself.
  app: boolean
  // The app keeps a list of servers, and can reach a server from away.
  servers: boolean; fromAway: boolean
  looks: boolean; layoutEditor: boolean; moduleSettings: boolean; remote: boolean; parties: boolean
  // What this profile's limits turn off (GET /me/limits) and pages hidden from this kind of profile.
  off: ReadonlySet<string>
}

export function holds(cond: string, who: MenuWho): boolean {
  if (cond.startsWith('notOff:')) return !who.off.has(cond.slice(7))
  switch (cond) {
    case 'notTv': return !who.tv
    case 'admin': case 'adult': case 'tv': case 'twoStep': case 'games': case 'app': case 'servers': case 'fromAway':
    case 'looks': case 'layoutEditor': case 'moduleSettings': case 'remote': case 'parties':
      return who[cond]
    default: return false // a condition this page doesn't know yet: the row stays out rather than lead nowhere
  }
}

// The groups this person sees, in the table's order. `opens` says whether this client can open a row (its own
// screen for `native`, or a page); a group left with no rows is left out.
export function menuGroups(table: MenuTable, who: MenuWho, opens: (row: MenuRow) => boolean): MenuGroup[] {
  return table.groups
    .map((g) => ({ ...g, rows: g.rows.filter((r) => (r.when || []).every((c) => holds(c, who)) && opens(r)) }))
    .filter((g) => g.rows.length > 0)
}

// What a television may do with a row: its part's level, else its page's (sdui.SettingsRowLevel).
export function rowLevel(tv: TvLevelTable, row: MenuRow): TvLevel {
  if (row.part) return partFor(tv, row.part)?.level || 'full'
  if (!row.path) return 'full'
  return screenFor(tv, row.path)?.level || 'full'
}

// The summary a television shows on a row it can't change: the row's own, else its page's in the TV table.
export function rowSummary(tv: TvLevelTable, row: MenuRow): string {
  return row.summary || (row.path ? screenFor(tv, row.path)?.summary || '' : '')
}

// The row a settings address belongs to: the one whose page is that address, for the menu beside a page to
// mark where it is. The longest match wins, so /settings/users#your-account and /settings/users both mark
// /settings/users's first row.
export function rowFor(groups: MenuGroup[], pathname: string): MenuRow | null {
  let best: MenuRow | null = null
  for (const g of groups) for (const r of g.rows) {
    const p = (r.path || '').split('#')[0]
    if (!p) continue
    if ((pathname === p || pathname.startsWith(p + '/')) && (!best || p.length > (best.path || '').split('#')[0].length)) best = r
  }
  return best
}
