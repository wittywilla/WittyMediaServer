// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, mk, secP } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'

// A games library's two sheets: Filter and sort (canvas "filters") and Jump to a letter (canvas "letterpick").

export interface FilterGroup { name: string; title: string; options: { value: string; label: string }[] }

// One chips section per group, the chosen value on; choosing it again clears it. Ids: <group>:<value>.
export function gameFiltersSpec(sort: string, sorts: { id: string; label: string }[], groups: FilterGroup[], now: Record<string, string>): SpecPageData {
  const sections: SpecSection[] = [secP('Sort', sorts.map((s) => ({ ...Cc(s.label, sort === s.id), id: 'sort:' + s.id })))]
  for (const g of groups) if (g.options.length) sections.push(secP(g.title, g.options.map((o) => ({ ...Cc(o.label, (now[g.name] || '') === o.value), id: g.name + ':' + o.value }))))
  return mk('filters', 'Library · Filters', 'Filter and sort', '', sections)
}

// The first id part is the group, the rest the value (a value may hold colons).
export const splitFilterId = (id: string): [string, string] => { const i = id.indexOf(':'); return [id.slice(0, i), id.slice(i + 1)] }

export function letterSpec(letters: { label: string; offset: number; count: number }[]): SpecPageData {
  return mk('letterpick', 'Library · Letter', 'Jump to a letter', '', [secP('', letters.map((l) => ({ ...Cc(l.label), id: 'letter:' + l.offset })))])
}
