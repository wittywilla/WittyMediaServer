// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, mk, secP, secR } from '../model.ts'
import type { SpecPageData, SpecRow, SpecSection } from '../types.ts'

// The games library's "Files that didn't become games" list (canvas keys "gamereview" and "gameattn"): one row
// per file with its reason, and the choices beside it. Ids: add.<id>, other.<id>, ignore.<id>, forget.<id>,
// pick.<id>.<console>, closepick, bulk.<console>. The tabs and the dialog frame are the page's own.
export interface ReviewFile { id: number; path: string; name: string; kind: string; reason: string; suggest: { console: string; short: string }[]; itemId?: number }
export type ReviewTab = 'needs' | 'blocked' | 'left' | 'ignored'

export const KIND_WORDS: Record<string, string> = {
  bios: 'BIOS', update: 'Updates', dlc: 'Extra content', utility: 'Utilities', audio: 'Music', data: 'Pieces of a game', other: 'Other files', duplicate: 'Copies', ignored: 'Not games',
}
export const kindWord = (k: string) => KIND_WORDS[k] || k

// Folders that are plainly one console's, with no headers: one choice for the lot.
export function bulkGroups(items: ReviewFile[]): { console: string; name: string; ids: number[] }[] {
  const groups = new Map<string, { console: string; name: string; ids: number[] }>()
  for (const it of items) {
    const s = it.suggest[0]
    if (!s) continue
    const g = groups.get(s.console) || { console: s.console, name: s.short, ids: [] }
    g.ids.push(it.id)
    groups.set(s.console, g)
  }
  return [...groups.values()].filter((g) => g.ids.length > 1)
}

export function reviewSpec(tab: ReviewTab, items: ReviewFile[], opts: { busy: boolean; consoles: { id: string; name: string }[]; choosing?: number | null }): SpecPageData {
  const s: SpecSection[] = []
  if (tab === 'needs') {
    const g = bulkGroups(items)
    if (g.length) s.push(secP('Many at once', g.map((x) => ({ ...Cc(`Add all ${x.ids.length} as ${x.name}`), id: `bulk.${x.console}`, label: `Add all ${x.ids.length} as ${x.name} games`, disabled: opts.busy }))))
  }
  const rows: SpecRow[] = items.map((it) => {
    const first = it.suggest[0]
    const sub = [it.path !== it.name ? it.path : '', it.reason].filter(Boolean).join(' · ')
    const row: SpecRow = { id: `file.${it.id}`, title: it.name, sub, ...(tab === 'left' ? { value: kindWord(it.kind) } : {}) }
    if (tab === 'blocked') return it.itemId ? { ...row, to: `/item/${it.itemId}`, value: 'Open the game' } : row
    if (tab === 'ignored') return { ...row, actions: [{ id: `forget.${it.id}`, title: 'Take it back', label: `Take back saying ${it.name} isn’t a game`, disabled: opts.busy }] }
    if (it.kind === 'duplicate') return row
    const actions = []
    if (first) actions.push({ id: `add.${it.id}`, title: `Add as ${first.short}`, label: `${it.name} is a ${first.short} game`, disabled: opts.busy })
    actions.push({ id: `other.${it.id}`, title: 'Another console', label: `Another console for ${it.name}`, disabled: opts.busy })
    if (tab === 'needs') actions.push({ id: `ignore.${it.id}`, title: 'Not a game', label: `${it.name} is not a game`, disabled: opts.busy })
    return { ...row, on: opts.choosing === it.id, actions }
  })
  if (rows.length) s.push(secR('', rows))
  if (opts.choosing != null) {
    const f = items.find((i) => i.id === opts.choosing)
    if (f) s.push(secP(`Console for ${f.name}`, [...opts.consoles.map((c) => ({ ...Cc(c.name), id: `pick.${f.id}.${c.id}`, disabled: opts.busy })), { ...Cc('Cancel'), id: 'closepick' }]))
  }
  return mk('gamereview', 'Games · Review', 'Needs a look', '', s)
}
