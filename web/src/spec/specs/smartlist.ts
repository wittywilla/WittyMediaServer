// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, choice, mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecField, SpecPageData, SpecSection } from '../types.ts'

// The collection dialog (canvas page "smartlist"): name, kind, the rule of a smart playlist with a live count, and
// sharing. Ids: name, desc, kind:manual, kind:smart, type:<library type>, genres, search, artist, yearFrom, yearTo,
// ratingMin, days, watched, sort, order, limit, liked, episodes, shared.
export interface RuleView {
  libraryTypes?: string[]; search?: string; artist?: string; yearFrom?: number; yearTo?: number; ratingMin?: number; addedWithinDays?: number
  watched?: '' | 'yes' | 'no' | 'started'; sort?: string; desc?: boolean; limit?: number; liked?: boolean; episodes?: boolean
}
export interface CollectionView { name: string; desc: string; kind: 'manual' | 'smart'; canChooseKind: boolean; rule: RuleView; genresText: string; shared: boolean; canShare: boolean; preview: { count: number } | null; error: string }

export const LIBRARY_TYPES: [string, string][] = [['movies', 'Movies'], ['shows', 'TV shows'], ['anime', 'Anime'], ['discs', 'Discs'], ['videos', 'Home videos'], ['music', 'Music'], ['musicvideos', 'Music videos'], ['audiobooks', 'Audiobooks'], ['podcasts', 'Podcasts'], ['books', 'Books'], ['channels', 'Creator videos'], ['shorts', 'Vertical videos'], ['photos', 'Photos']]
const WATCHED = [{ value: '', label: 'Either' }, { value: 'no', label: 'Not watched' }, { value: 'started', label: 'Started' }, { value: 'yes', label: 'Watched' }]
const SORTS = [{ value: 'added', label: 'Date added' }, { value: 'title', label: 'Title' }, { value: 'year', label: 'Year' }, { value: 'rating', label: 'Rating' }, { value: 'runtime', label: 'Length' }, { value: 'played', label: 'Most played' }, { value: 'random', label: 'Shuffle' }]
const ORDER = [{ value: 'd', label: 'Highest or newest first' }, { value: 'a', label: 'Lowest or oldest first' }]

// A number box's text back to a number, or nothing when empty.
export const num = (v: string | number): number | undefined => (v === '' ? undefined : Number(v))

export function collectionSpec(v: CollectionView): SpecPageData {
  const r = v.rule
  const s: SpecSection[] = [secF('Name', [
    { id: 'name', kind: 'text', label: 'Name', value: v.name, required: true, autoFocus: true },
    { id: 'desc', kind: 'text', label: 'Description', value: v.desc },
  ])]
  if (v.canChooseKind) s.push(choice(secR('Kind', [
    { id: 'kind:manual', title: 'A list you fill', on: v.kind === 'manual' },
    { id: 'kind:smart', title: 'Smart playlist', on: v.kind === 'smart' },
  ])))
  if (v.kind === 'smart') {
    s.push(secP('From these kinds of library (none means all)', LIBRARY_TYPES.map(([k, l]) => ({ ...Cc(l, !!r.libraryTypes?.includes(k)), id: 'type:' + k }))))
    const f: SpecField[] = [
      { id: 'genres', kind: 'text', label: 'Genres (any)', value: v.genresText, placeholder: 'Comedy, Drama' },
      { id: 'search', kind: 'text', label: 'Words in the title', value: r.search || '' },
      { id: 'artist', kind: 'text', label: 'Artist or creator', value: r.artist || '' },
      { id: 'yearFrom', kind: 'number', label: 'From year', value: r.yearFrom ?? '', min: 1800, max: 3000 },
      { id: 'yearTo', kind: 'number', label: 'To year', value: r.yearTo ?? '', min: 1800, max: 3000 },
      { id: 'ratingMin', kind: 'number', label: 'Rating at least', value: r.ratingMin ?? '', min: 0, max: 10, step: 0.5 },
      { id: 'days', kind: 'number', label: 'Added in the last (days)', value: r.addedWithinDays ?? '', min: 0 },
      { id: 'watched', kind: 'select', label: 'Watched', value: r.watched || '', options: WATCHED },
      { id: 'sort', kind: 'select', label: 'Sort by', value: r.sort || 'added', options: SORTS },
      { id: 'order', kind: 'select', label: 'Order', value: r.desc ? 'd' : 'a', options: ORDER },
      { id: 'limit', kind: 'number', label: 'At most', value: r.limit ?? 100, min: 1, max: 500 },
      { id: 'liked', kind: 'switch', label: 'Only what I liked', value: !!r.liked },
      { id: 'episodes', kind: 'switch', label: 'Include TV episodes', value: !!r.episodes },
    ]
    s.push(secF('Everything that matches', f))
    if (v.preview) s.push(secN('', `${v.preview.count} ${v.preview.count === 1 ? 'match' : 'matches'}`))
  }
  if (v.canShare) s.push(secF('Sharing', [{ id: 'shared', kind: 'switch', label: 'Share with the other profiles', value: v.shared, hint: 'Each sees what their own limits allow.' }]))
  if (v.error) s.push(secN('', v.error, true))
  return mk('smartlist', 'Collections · Smart playlist', v.name || (v.canChooseKind ? 'New collection' : 'Edit collection'), '', s, [], '')
}

// A field change, applied to the rule: returns the new rule (the page keeps genres as typed text apart from it).
export function applyRuleField<T extends RuleView>(rule: T, id: string, value: string | number | boolean): T {
  const n = typeof value === 'boolean' ? undefined : num(value as string | number)
  switch (id) {
    case 'search': return { ...rule, search: String(value) }
    case 'artist': return { ...rule, artist: String(value) }
    case 'yearFrom': return { ...rule, yearFrom: n }
    case 'yearTo': return { ...rule, yearTo: n }
    case 'ratingMin': return { ...rule, ratingMin: n }
    case 'days': return { ...rule, addedWithinDays: n }
    case 'limit': return { ...rule, limit: n }
    case 'watched': return { ...rule, watched: String(value) as RuleView['watched'] }
    case 'sort': return { ...rule, sort: String(value) } as T
    case 'order': return { ...rule, desc: value === 'd' }
    case 'liked': return { ...rule, liked: !!value }
    case 'episodes': return { ...rule, episodes: !!value }
    default: return rule
  }
}
export const genresOf = (text: string) => text.split(',').map((s) => s.trim()).filter(Boolean)
export const toggleType = <T extends RuleView>(rule: T, type: string): T => ({ ...rule, libraryTypes: rule.libraryTypes?.includes(type) ? rule.libraryTypes.filter((x) => x !== type) : [...(rule.libraryTypes || []), type] })
