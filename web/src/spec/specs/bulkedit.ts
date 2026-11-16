// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF, secN, secR } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'
import { t, tk, tn } from '../../sdui/words.ts'

// Editing many titles together (canvas page "bulkedit"). Section 0 is a note under which the page puts its genre and
// tag token fields (`after`), section 1 holds Year and the rating switch (the rating chooser goes `after` it when on),
// section 2 the locks. Field ids: year, setRating, lock.
export interface BulkView { count: number; year: string; setRating: boolean; lock: string; error: string; result: { done: number; failed: string[] } | null }

// Labels marked with tk: shown with t(label).
export const LOCK_OPTIONS = [{ value: '', label: tk('Leave them as they are') }, { value: 'lock', label: tk('Lock every field') }, { value: 'unlock', label: tk('Unlock every field') }]

export function bulkTitle(count: number): string { return tn('Edit {count} title together', 'Edit {count} titles together', count) }

export function bulkSpec(v: BulkView): SpecPageData {
  const title = bulkTitle(v.count)
  if (v.result) {
    const failed = v.result.failed.length
    const s: SpecSection[] = [secN('', failed ? t('Changed {done}. {failed} couldn\'t be changed.', { done: v.result.done, failed }) : t('Changed {done}.', { done: v.result.done }), !failed)]
    if (failed) s.push(secR(t('Not changed'), v.result.failed.slice(0, 10).map((f) => ({ title: f }))))
    if (v.error) s.push(secN('', v.error, true))
    return mk('bulkedit', t('Library · Select mode'), title, '', s, [], '')
  }
  const s: SpecSection[] = [
    secN(t('Genres and tags'), t('Only what you fill in changes. What you change is locked on each title, so nothing changes it back.')),
    secF(t('Details'), [
      { id: 'year', kind: 'text', label: t('Year'), value: v.year, hint: t('Four digits') },
      { id: 'setRating', kind: 'switch', label: t('Set the content rating'), value: v.setRating },
    ]),
    secF(t('Locks'), [{ id: 'lock', kind: 'select', label: t('Locks'), value: v.lock, options: LOCK_OPTIONS.map((o) => ({ value: o.value, label: t(o.label) })) }]),
  ]
  if (v.error) s.push(secN('', v.error, true))
  return mk('bulkedit', t('Library · Select mode'), title, '', s, [], '')
}

// What gets sent: the page's own words in, the server's request out.
export function bulkRequest(v: { year: string; setRating: boolean; rating: string; lock: string }): { set: Record<string, string>; lock: boolean; unlock: boolean; nothing: (more: boolean) => boolean } {
  const set: Record<string, string> = {}
  if (v.year.trim()) set.year = v.year.trim()
  if (v.setRating) set.contentRating = v.rating
  return { set, lock: v.lock === 'lock', unlock: v.lock === 'unlock', nothing: (tokens) => !tokens && !v.year.trim() && !v.setRating && !v.lock }
}
