// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecPageData, SpecRow, SpecSection } from '../types.ts'
import { t } from '../../sdui/words.ts'
import { bidi } from '../../i18n'

// Fix matches (canvas keys "fixmatches" and "editmatch"): the review's tabs and tools, and the opened title's
// panel with its suggestions. The list of titles and the keys stay the page's own. Ids: tab:<key>, search, find,
// stop, sure; in the panel edit, renumber, leave, query, go, look:<n>, accept:<n>.

export function tabsSpec(tabs: [string, string][], counts: Record<string, number> | undefined, current: string): SpecPageData {
  return mk('fixmatches-tabs', t('Library · Review'), t('Fix matches'), '', [
    secP(t('Which titles'), tabs.map(([k, l]) => ({ id: 'tab:' + k, title: counts && k !== 'removed' && counts[k] != null ? t('{tab} {count}', { tab: l, count: counts[k] }) : l, on: k === current }))),
  ])
}

export interface Job { running: boolean; done: number; total: number; current?: string }

export function toolsSpec(search: string, job: Job | null, sure: number): SpecPageData {
  const chips = job?.running
    ? [{ id: 'stop', title: t('Stop') }]
    : [{ id: 'find', title: t('Find suggestions for all') }, ...(sure > 1 ? [{ id: 'sure', title: t('Choose the {count} that are 90% sure', { count: sure }) }] : [])]
  const sections: SpecSection[] = [
    secF('', [{ id: 'search', kind: 'text', label: t('Find a title in this list'), placeholder: t('Find a title'), value: search }]),
    secP('', chips),
  ]
  if (job?.running) sections.push(secN('', job.current ? t('Finding suggestions: {done} of {total}, {title}', { done: job.done, total: job.total, title: job.current }) : t('Finding suggestions: {done} of {total}', { done: job.done, total: job.total })))
  return mk('fixmatches-tools', t('Library · Review'), t('Fix matches'), '', sections)
}

export interface Line { title: string; year?: number | string; kind?: string; rating?: number; score: number; source: string; sourceName: string }
export interface DetailIn {
  state: string; target: string; reasons: string[]; matchedWith?: string; matchedUrl?: string
  query: string; busy: boolean; err: string; warnings: string[]; results: boolean; list: Line[]
}

export function detailSpec(d: DetailIn): SpecPageData {
  const sections: SpecSection[] = []
  const chips = [{ id: 'edit', title: t('Edit details') }]
  if (d.target === 'show') chips.push({ id: 'renumber', title: t('Renumber episodes') })
  if (d.state === 'unmatched' || d.state === 'low') chips.push({ id: 'leave', title: d.state === 'low' ? t('It’s right: stop asking') : t('It has no match') })
  sections.push(secP('', chips))
  const why: SpecRow[] = []
  if (d.matchedWith) why.push({ title: t('Matched with'), value: d.matchedWith, ...(d.matchedUrl ? { href: d.matchedUrl } : {}) })
  for (const w of d.reasons) why.push({ title: w })
  if (why.length) sections.push(secR(t('How it was matched'), why))
  sections.push(secF('', [{ id: 'query', kind: 'text', label: t('Search for the right title, or paste an id or address'), placeholder: t('A name, or an id or address from TMDB, IMDb, TheTVDB, TVmaze or Wikidata'), value: d.query, disabled: d.busy }]))
  sections.push(secP('', [{ id: 'go', title: t('Search'), disabled: d.busy }]))
  if (d.err) sections.push(secN('', d.err, true))
  for (const w of d.warnings) sections.push(secN('', w, true))
  if (d.list.length) {
    sections.push(secR(d.results ? t('Results') : t('Suggestions'), d.list.map((c, i): SpecRow => ({
      id: 'cand:' + i, title: i < 6 ? t('{number}. {title}', { number: i + 1, title: c.title }) : c.title, value: t('{percent}% · {source}', { percent: Math.round(c.score), source: c.sourceName }),
      sub: [c.year, c.kind === 'show' ? t('Show') : t('Film'), c.rating ? t('Rated {rating}', { rating: c.rating.toFixed(1) }) : ''].filter(Boolean).map(bidi).join(' · '),
      actions: [{ id: `look:${i}`, title: t('Look'), label: t('Look at {title}', { title: c.title }) }, { id: `accept:${i}`, title: t('Accept'), label: t('Accept {title}', { title: c.title }) }],
    }))))
  } else if (!d.busy && d.state !== 'manual' && d.state !== 'none') {
    sections.push(secN('', t('No source has anything by this name. Try another name, or an id, above.')))
  }
  return mk('editmatch', t('Library · Fix matches'), t('Match'), '', sections)
}
