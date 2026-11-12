// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF, secN, secR } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'

// Editing many titles together (canvas page "bulkedit"). Section 0 is a note under which the page puts its genre and
// tag token fields (`after`), section 1 holds Year and the rating switch (the rating chooser goes `after` it when on),
// section 2 the locks. Field ids: year, setRating, lock.
export interface BulkView { count: number; year: string; setRating: boolean; lock: string; error: string; result: { done: number; failed: string[] } | null }

export const LOCK_OPTIONS = [{ value: '', label: 'Leave them as they are' }, { value: 'lock', label: 'Lock every field' }, { value: 'unlock', label: 'Unlock every field' }]

export function bulkTitle(count: number): string { return `Edit ${count} ${count === 1 ? 'title' : 'titles'} together` }

export function bulkSpec(v: BulkView): SpecPageData {
  const title = bulkTitle(v.count)
  if (v.result) {
    const s: SpecSection[] = [secN('', `Changed ${v.result.done}.${v.result.failed.length ? ` ${v.result.failed.length} couldn't be changed.` : ''}`, !v.result.failed.length)]
    if (v.result.failed.length) s.push(secR('Not changed', v.result.failed.slice(0, 10).map((f) => ({ title: f }))))
    if (v.error) s.push(secN('', v.error, true))
    return mk('bulkedit', 'Library · Select mode', title, '', s, [], '')
  }
  const s: SpecSection[] = [
    secN('Genres and tags', 'Only what you fill in changes. What you change is locked on each title, so nothing changes it back.'),
    secF('Details', [
      { id: 'year', kind: 'text', label: 'Year', value: v.year, hint: 'Four digits' },
      { id: 'setRating', kind: 'switch', label: 'Set the content rating', value: v.setRating },
    ]),
    secF('Locks', [{ id: 'lock', kind: 'select', label: 'Locks', value: v.lock, options: LOCK_OPTIONS }]),
  ]
  if (v.error) s.push(secN('', v.error, true))
  return mk('bulkedit', 'Library · Select mode', title, '', s, [], '')
}

// What gets sent: the page's own words in, the server's request out.
export function bulkRequest(v: { year: string; setRating: boolean; rating: string; lock: string }): { set: Record<string, string>; lock: boolean; unlock: boolean; nothing: (more: boolean) => boolean } {
  const set: Record<string, string> = {}
  if (v.year.trim()) set.year = v.year.trim()
  if (v.setRating) set.contentRating = v.rating
  return { set, lock: v.lock === 'lock', unlock: v.lock === 'unlock', nothing: (tokens) => !tokens && !v.year.trim() && !v.setRating && !v.lock }
}
