// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, mk, secR } from '../model.ts'
import type { SpecPageData, SpecRow, SpecSection } from '../types.ts'

// Save to a list (canvas key "savelist"). The Collections page itself is the server's own page (SDUI).

// Save to a list, for one film, show or video: the lists it can go in, ticked where it is. Watch later is offered
// until it exists.
export function saveSpec(lists: { id: number; name: string }[], holding: readonly number[], hasLater: boolean): SpecPageData {
  const rows: SpecRow[] = lists.map((c) => ({ id: 'list:' + c.id, title: c.name, value: holding.includes(c.id) ? '✓' : undefined, on: holding.includes(c.id) }))
  if (!hasLater) rows.unshift({ id: 'later', title: 'Watch later', sub: 'A list made for you', on: false })
  const sections: SpecSection[] = rows.length ? [secR('Lists', rows)] : []
  return mk('savelist', 'Save to a list', 'Save to', 'Tick the lists it should be in.', sections, [{ ...A1('Done'), id: 'done' }], '')
}
