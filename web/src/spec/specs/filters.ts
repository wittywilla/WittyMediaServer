// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, A2, mk, secP } from '../model.ts'
import type { SpecChip, SpecPageData, SpecSection } from '../types.ts'
import { t, tk } from '../../sdui/words.ts'

// The library's filter panel (canvas key "filters"): what is chosen, then every facet the library has as chips with
// their counts. What's chosen lives in the address; ids tell the panel what was pressed:
//   chosen:<key>   take a filter off        f:<key>:<value>   choose or clear a value        more:<key>   show them all
//   clear, save, done                        the page's own buttons
export const FILTER_KEYS = ['genre', 'studio', 'network', 'kind', 'disc', 'decade', 'year', 'content', 'minRating', 'resolution', 'audio', 'subtitle', 'format', 'tag', 'edition', 'series', 'watched', 'platform', 'search']

// Marked with tk: show them with t(FILTER_LABELS[key]).
export const FILTER_LABELS: Record<string, string> = {
  genre: tk('Category'), studio: tk('Studio'), network: tk('TV network'), kind: tk('Type'), disc: tk('On disc'), decade: tk('Decade'), year: tk('Year'), content: tk('Age rating'), minRating: tk('Score'), resolution: tk('Picture'),
  audio: tk('Audio language'), subtitle: tk('Subtitles'), format: tk('Sound'), tag: tk('Tag'), edition: tk('Edition'), series: tk('Collection'), watched: tk('Watched'), platform: tk('Listed on'), search: tk('Words'),
}

// The words for a chosen value. `lang` turns a language code into its name.
export function facetLabel(k: string, v: string, lang: (code: string) => string = (c) => c): string {
  if (k === 'audio' || k === 'subtitle') return lang(v)
  if (k === 'minRating') return t('{score} or more', { score: v })
  if (k === 'decade') return t('{decade}s', { decade: v })
  if (k === 'watched') return v === 'yes' ? t('Watched') : t('Not watched yet')
  if (k === 'kind') return ({ video: t('Video files'), dvd: t('DVDs'), bluray: t('Blu-rays'), disc: t('DVDs and Blu-rays'), show: t('TV shows') } as Record<string, string>)[v] || v
  if (k === 'disc') return ({ movie: t('Films'), tv: t('TV series'), music: t('Music') } as Record<string, string>)[v] || v
  if (k === 'resolution') return ({ '4k': '4K', '1080p': '1080p', '720p': '720p', sd: 'SD' } as Record<string, string>)[v] || v
  return v
}

export interface FacetList { value: string; label?: string; count: number }
export interface FilterFacets { genres: FacetList[]; studios: FacetList[]; networks: FacetList[]; decades: FacetList[]; years: FacetList[]; content: FacetList[]; resolutions: FacetList[]; audio: FacetList[]; subtitles: FacetList[]; formats: FacetList[]; tags: FacetList[]; editions: FacetList[]; kinds: FacetList[]; platforms: FacetList[] }

type Group = [string, string, keyof FilterFacets, number] // key, heading (tk, shown with t), facets field, how many before "N more"
const FIRST: Group[] = [
  ['genre', tk('Category'), 'genres', 24], ['kind', tk('Type'), 'kinds', 24], ['studio', tk('Studio'), 'studios', 16], ['network', tk('TV network'), 'networks', 16], ['platform', tk('Listed on'), 'platforms', 24],
  ['decade', tk('Decade'), 'decades', 24], ['year', tk('Year'), 'years', 10], ['content', tk('Age rating'), 'content', 24],
]
const LAST: Group[] = [
  ['resolution', tk('Picture'), 'resolutions', 24], ['format', tk('Sound'), 'formats', 24], ['audio', tk('Audio language'), 'audio', 24], ['subtitle', tk('Subtitles'), 'subtitles', 24], ['edition', tk('Edition'), 'editions', 24], ['tag', tk('Tags'), 'tags', 24],
]

export interface FilterSpecInput { facets: FilterFacets | null; params: Record<string, string>; expanded?: ReadonlySet<string>; lang?: (code: string) => string; platformName?: (key: string) => string | undefined }

export function filterSpec(i: FilterSpecInput): SpecPageData {
  const lang = i.lang || ((c: string) => c)
  const on = FILTER_KEYS.filter((k) => i.params[k])
  const sections: SpecSection[] = []
  if (on.length) {
    sections.push(secP(t('Chosen'), on.map((k): SpecChip => ({ id: 'chosen:' + k, title: t('{filter}: {value} ✕', { filter: t(FILTER_LABELS[k]), value: (k === 'platform' && i.platformName?.(i.params[k])) || facetLabel(k, i.params[k], lang) }), on: true }))))
  }
  const group = ([k, h, field, limit]: Group) => {
    const list = ((i.facets?.[field] as FacetList[] | undefined) || []).filter((f) => !(k === 'format' && f.value === 'none'))
    if (!list.length) return
    const cur = (i.params[k] || '').toLowerCase()
    const shown = i.expanded?.has(k) ? list : list.slice(0, limit)
    const chips: SpecChip[] = shown.map((f) => ({ id: `f:${k}:${f.value}`, title: t('{value} ({count})', { value: f.label || facetLabel(k, f.value, lang), count: f.count }), on: cur === f.value.toLowerCase() }))
    if (shown.length < list.length) chips.push({ id: 'more:' + k, title: t('{count} more', { count: list.length - shown.length }) })
    sections.push(secP(t(h), chips))
  }
  FIRST.forEach(group)
  sections.push(secP(t('Score'), ['9', '8', '7', '6'].map((n) => ({ id: `f:minRating:${n}`, title: t('{score} or more', { score: n }), on: i.params.minRating === n }))))
  sections.push(secP(t('Watched'), [['no', t('Not watched yet')], ['yes', t('Watched')]].map(([v, l]) => ({ id: `f:watched:${v}`, title: l, on: i.params.watched === v }))))
  LAST.forEach(group)
  return mk('filters', t('Library · Filters'), t('Narrow this library'), t('Pick any. Chosen filters appear as chips above the grid.'), sections,
    [{ ...A1(t('Done')), id: 'done' }, { ...A2(t('Clear all')), id: 'clear', disabled: !on.length }, { ...A2(t('Save this view')), id: 'save', disabled: !on.length }], '')
}

// What pressing an id means: [key, value] to set ('' takes it off), show more of a facet, or a verb.
export type FilterPress = { set: [string, string] } | { more: string } | { verb: 'clear' | 'save' | 'done' } | null
export function pressFilter(id: string, params: Record<string, string>): FilterPress {
  if (id === 'clear' || id === 'save' || id === 'done') return { verb: id }
  if (id.startsWith('chosen:')) return { set: [id.slice(7), ''] }
  if (id.startsWith('more:')) return { more: id.slice(5) }
  const m = /^f:([^:]+):(.*)$/.exec(id)
  if (!m) return null
  const [, k, v] = m
  return { set: [k, (params[k] || '').toLowerCase() === v.toLowerCase() ? '' : v] }
}
