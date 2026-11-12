// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, A2, mk, secP } from '../model.ts'
import type { SpecChip, SpecPageData, SpecSection } from '../types.ts'

// The library's filter panel (canvas key "filters"): what is chosen, then every facet the library has as chips with
// their counts. What's chosen lives in the address; ids tell the panel what was pressed:
//   chosen:<key>   take a filter off        f:<key>:<value>   choose or clear a value        more:<key>   show them all
//   clear, save, done                        the page's own buttons
export const FILTER_KEYS = ['genre', 'studio', 'network', 'kind', 'disc', 'decade', 'year', 'content', 'minRating', 'resolution', 'audio', 'subtitle', 'format', 'tag', 'edition', 'series', 'watched', 'platform', 'search']

export const FILTER_LABELS: Record<string, string> = {
  genre: 'Category', studio: 'Studio', network: 'Network', kind: 'Type', disc: 'On disc', decade: 'Decade', year: 'Year', content: 'Age rating', minRating: 'Score', resolution: 'Picture',
  audio: 'Audio language', subtitle: 'Subtitles', format: 'Sound', tag: 'Tag', edition: 'Edition', series: 'Collection', watched: 'Watched', platform: 'Listed on', search: 'Words',
}

// The words for a chosen value. `lang` turns a language code into its name.
export function facetLabel(k: string, v: string, lang: (code: string) => string = (c) => c): string {
  if (k === 'audio' || k === 'subtitle') return lang(v)
  if (k === 'minRating') return `${v} or more`
  if (k === 'decade') return `${v}s`
  if (k === 'watched') return v === 'yes' ? 'Watched' : 'Not watched yet'
  if (k === 'kind') return ({ video: 'Video files', dvd: 'DVDs', bluray: 'Blu-rays', disc: 'DVDs and Blu-rays', show: 'TV shows' } as Record<string, string>)[v] || v
  if (k === 'disc') return ({ movie: 'Films', tv: 'TV series', music: 'Music' } as Record<string, string>)[v] || v
  if (k === 'resolution') return ({ '4k': '4K', '1080p': '1080p', '720p': '720p', sd: 'SD' } as Record<string, string>)[v] || v
  return v
}

export interface FacetList { value: string; label?: string; count: number }
export interface FilterFacets { genres: FacetList[]; studios: FacetList[]; networks: FacetList[]; decades: FacetList[]; years: FacetList[]; content: FacetList[]; resolutions: FacetList[]; audio: FacetList[]; subtitles: FacetList[]; formats: FacetList[]; tags: FacetList[]; editions: FacetList[]; kinds: FacetList[]; platforms: FacetList[] }

type Group = [string, string, keyof FilterFacets, number] // key, heading, facets field, how many before "N more"
const FIRST: Group[] = [
  ['genre', 'Category', 'genres', 24], ['kind', 'Type', 'kinds', 24], ['studio', 'Studio', 'studios', 16], ['network', 'Network', 'networks', 16], ['platform', 'Listed on', 'platforms', 24],
  ['decade', 'Decade', 'decades', 24], ['year', 'Year', 'years', 10], ['content', 'Age rating', 'content', 24],
]
const LAST: Group[] = [
  ['resolution', 'Picture', 'resolutions', 24], ['format', 'Sound', 'formats', 24], ['audio', 'Audio language', 'audio', 24], ['subtitle', 'Subtitles', 'subtitles', 24], ['edition', 'Edition', 'editions', 24], ['tag', 'Tags', 'tags', 24],
]

export interface FilterSpecInput { facets: FilterFacets | null; params: Record<string, string>; expanded?: ReadonlySet<string>; lang?: (code: string) => string; platformName?: (key: string) => string | undefined }

export function filterSpec(i: FilterSpecInput): SpecPageData {
  const lang = i.lang || ((c: string) => c)
  const on = FILTER_KEYS.filter((k) => i.params[k])
  const sections: SpecSection[] = []
  if (on.length) {
    sections.push(secP('Chosen', on.map((k): SpecChip => ({ id: 'chosen:' + k, title: `${FILTER_LABELS[k]}: ${(k === 'platform' && i.platformName?.(i.params[k])) || facetLabel(k, i.params[k], lang)} ✕`, on: true }))))
  }
  const group = ([k, h, field, limit]: Group) => {
    const list = ((i.facets?.[field] as FacetList[] | undefined) || []).filter((f) => !(k === 'format' && f.value === 'none'))
    if (!list.length) return
    const cur = (i.params[k] || '').toLowerCase()
    const shown = i.expanded?.has(k) ? list : list.slice(0, limit)
    const chips: SpecChip[] = shown.map((f) => ({ id: `f:${k}:${f.value}`, title: `${f.label || facetLabel(k, f.value, lang)} (${f.count})`, on: cur === f.value.toLowerCase() }))
    if (shown.length < list.length) chips.push({ id: 'more:' + k, title: `${list.length - shown.length} more` })
    sections.push(secP(h, chips))
  }
  FIRST.forEach(group)
  sections.push(secP('Score', ['9', '8', '7', '6'].map((n) => ({ id: `f:minRating:${n}`, title: `${n} or more`, on: i.params.minRating === n }))))
  sections.push(secP('Watched', [['no', 'Not watched yet'], ['yes', 'Watched']].map(([v, l]) => ({ id: `f:watched:${v}`, title: l, on: i.params.watched === v }))))
  LAST.forEach(group)
  return mk('filters', 'Library · Filters', 'Narrow this library', 'Pick any. Chosen filters appear as chips above the grid.', sections,
    [{ ...A1('Done'), id: 'done' }, { ...A2('Clear all'), id: 'clear', disabled: !on.length }, { ...A2('Save this view'), id: 'save', disabled: !on.length }], '')
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
