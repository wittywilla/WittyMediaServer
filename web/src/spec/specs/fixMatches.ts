// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecPageData, SpecRow, SpecSection } from '../types.ts'

// Fix matches (canvas keys "fixmatches" and "editmatch"): the review's tabs and tools, and the opened title's
// panel with its suggestions. The list of titles and the keys stay the page's own. Ids: tab:<key>, search, find,
// stop, sure; in the panel edit, renumber, leave, query, go, look:<n>, accept:<n>.

export function tabsSpec(tabs: [string, string][], counts: Record<string, number> | undefined, current: string): SpecPageData {
  return mk('fixmatches-tabs', 'Library · Review', 'Fix matches', '', [
    secP('Which titles', tabs.map(([k, l]) => ({ id: 'tab:' + k, title: l + (counts && k !== 'removed' && counts[k] != null ? ` ${counts[k]}` : ''), on: k === current }))),
  ])
}

export interface Job { running: boolean; done: number; total: number; current?: string }

export function toolsSpec(search: string, job: Job | null, sure: number): SpecPageData {
  const chips = job?.running
    ? [{ id: 'stop', title: 'Stop' }]
    : [{ id: 'find', title: 'Find suggestions for all' }, ...(sure > 1 ? [{ id: 'sure', title: `Choose the ${sure} that are 90% sure` }] : [])]
  const sections: SpecSection[] = [
    secF('', [{ id: 'search', kind: 'text', label: 'Find a title in this list', placeholder: 'Find a title', value: search }]),
    secP('', chips),
  ]
  if (job?.running) sections.push(secN('', `Finding suggestions: ${job.done} of ${job.total}${job.current ? `, ${job.current}` : ''}`))
  return mk('fixmatches-tools', 'Library · Review', 'Fix matches', '', sections)
}

export interface Line { title: string; year?: number | string; kind?: string; rating?: number; score: number; source: string; sourceName: string }
export interface DetailIn {
  state: string; target: string; reasons: string[]; matchedWith?: string; matchedUrl?: string
  query: string; busy: boolean; err: string; warnings: string[]; results: boolean; list: Line[]
}

export function detailSpec(d: DetailIn): SpecPageData {
  const sections: SpecSection[] = []
  const chips = [{ id: 'edit', title: 'Edit details' }]
  if (d.target === 'show') chips.push({ id: 'renumber', title: 'Renumber episodes' })
  if (d.state === 'unmatched' || d.state === 'low') chips.push({ id: 'leave', title: d.state === 'low' ? 'It’s right: stop asking' : 'It has no match' })
  sections.push(secP('', chips))
  const why: SpecRow[] = []
  if (d.matchedWith) why.push({ title: 'Matched with', value: d.matchedWith, ...(d.matchedUrl ? { href: d.matchedUrl } : {}) })
  for (const w of d.reasons) why.push({ title: w })
  if (why.length) sections.push(secR('How it was matched', why))
  sections.push(secF('', [{ id: 'query', kind: 'text', label: 'Search for the right title, or paste an id or address', placeholder: 'A name, or an id or address from TMDB, IMDb, TheTVDB, TVmaze or Wikidata', value: d.query, disabled: d.busy }]))
  sections.push(secP('', [{ id: 'go', title: 'Search', disabled: d.busy }]))
  if (d.err) sections.push(secN('', d.err, true))
  for (const w of d.warnings) sections.push(secN('', w, true))
  if (d.list.length) {
    sections.push(secR(d.results ? 'Results' : 'Suggestions', d.list.map((c, i): SpecRow => ({
      id: 'cand:' + i, title: (i < 6 ? `${i + 1}. ` : '') + c.title, value: `${Math.round(c.score)}% · ${c.sourceName}`,
      sub: [c.year, c.kind === 'show' ? 'Show' : 'Film', c.rating ? `Rated ${c.rating.toFixed(1)}` : ''].filter(Boolean).join(' · '),
      actions: [{ id: `look:${i}`, title: 'Look', label: `Look at ${c.title}` }, { id: `accept:${i}`, title: 'Accept', label: `Accept ${c.title}` }],
    }))))
  } else if (!d.busy && d.state !== 'manual' && d.state !== 'none') {
    sections.push(secN('', 'No source has anything by this name. Try another name, or an id, above.'))
  }
  return mk('editmatch', 'Library · Fix matches', 'Match', '', sections)
}

