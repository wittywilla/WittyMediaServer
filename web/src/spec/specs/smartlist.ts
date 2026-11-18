// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { anchored, Cc, choice, mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecField, SpecPageData, SpecSection } from '../types.ts'
import { t, tk, tn } from '../../sdui/words.ts'

// The collection dialog (canvas page "smartlist"): name, kind, the rule of a smart playlist with a live count, and
// sharing. Ids: name, desc, kind:manual, kind:smart, type:<library type>, genres, search, artist, yearFrom, yearTo,
// ratingMin, days, watched, sort, order, limit, liked, episodes, shared.
export interface RuleView {
  libraryTypes?: string[]; search?: string; artist?: string; yearFrom?: number; yearTo?: number; ratingMin?: number; addedWithinDays?: number
  watched?: '' | 'yes' | 'no' | 'started'; sort?: string; desc?: boolean; limit?: number; liked?: boolean; episodes?: boolean
}
export interface CollectionView { name: string; desc: string; kind: 'manual' | 'smart'; canChooseKind: boolean; rule: RuleView; genresText: string; shared: boolean; canShare: boolean; preview: { count: number } | null; error: string }

// Names marked with tk: shown with t(name).
export const LIBRARY_TYPES: [string, string][] = [['movies', tk('Movies')], ['shows', tk('TV shows')], ['anime', tk('Anime')], ['discs', tk('Discs')], ['videos', tk('Home videos')], ['music', tk('Music')], ['musicvideos', tk('Music videos')], ['audiobooks', tk('Audiobooks')], ['podcasts', tk('Podcasts')], ['books', tk('Books')], ['channels', tk('Creator videos')], ['shorts', tk('Vertical videos')], ['photos', tk('Photos')]]
const WATCHED = [{ value: '', label: tk('Either') }, { value: 'no', label: tk('Not watched') }, { value: 'started', label: tk('Started') }, { value: 'yes', label: tk('Watched') }]
const SORTS = [{ value: 'added', label: tk('Date added') }, { value: 'title', label: tk('Title') }, { value: 'year', label: tk('Year') }, { value: 'rating', label: tk('Rating') }, { value: 'runtime', label: tk('Length') }, { value: 'played', label: tk('Most played') }, { value: 'random', label: tk('Shuffle') }]
const ORDER = [{ value: 'd', label: tk('Highest or newest first') }, { value: 'a', label: tk('Lowest or oldest first') }]
const shown = (list: { value: string; label: string }[]) => list.map((o) => ({ value: o.value, label: t(o.label) }))

// A number box's text back to a number, or nothing when empty.
export const num = (v: string | number): number | undefined => (v === '' ? undefined : Number(v))

export function collectionSpec(v: CollectionView): SpecPageData {
  const r = v.rule
  const s: SpecSection[] = [secF(t('Name'), [
    { id: 'name', kind: 'text', label: t('Name'), value: v.name, required: true, autoFocus: true },
    { id: 'desc', kind: 'text', label: t('Description'), value: v.desc },
  ])]
  if (v.canChooseKind) s.push(choice(secR(t('Kind'), [
    { id: 'kind:manual', title: t('A list you fill'), on: v.kind === 'manual' },
    { id: 'kind:smart', title: t('Smart playlist'), on: v.kind === 'smart' },
  ])))
  if (v.kind === 'smart') {
    s.push(secP(t('From these kinds of library (none means all)'), LIBRARY_TYPES.map(([k, l]) => ({ ...Cc(t(l), !!r.libraryTypes?.includes(k)), id: 'type:' + k }))))
    const f: SpecField[] = [
      { id: 'genres', kind: 'text', label: t('Genres (any)'), value: v.genresText, placeholder: t('Comedy, Drama') },
      { id: 'search', kind: 'text', label: t('Words in the title'), value: r.search || '' },
      { id: 'artist', kind: 'text', label: t('Artist or creator'), value: r.artist || '' },
      { id: 'yearFrom', kind: 'number', label: t('From year'), value: r.yearFrom ?? '', min: 1800, max: 3000 },
      { id: 'yearTo', kind: 'number', label: t('To year'), value: r.yearTo ?? '', min: 1800, max: 3000 },
      { id: 'ratingMin', kind: 'number', label: t('Rating at least'), value: r.ratingMin ?? '', min: 0, max: 10, step: 0.5 },
      { id: 'days', kind: 'number', label: t('Added in the last (days)'), value: r.addedWithinDays ?? '', min: 0 },
      { id: 'watched', kind: 'select', label: t('Watched'), value: r.watched || '', options: shown(WATCHED) },
      { id: 'sort', kind: 'select', label: t('Sort by'), value: r.sort || 'added', options: shown(SORTS) },
      { id: 'order', kind: 'select', label: t('Order'), value: r.desc ? 'd' : 'a', options: shown(ORDER) },
      { id: 'limit', kind: 'number', label: t('At most'), value: r.limit ?? 100, min: 1, max: 500 },
      { id: 'liked', kind: 'switch', label: t('Only what I liked'), value: !!r.liked },
      { id: 'episodes', kind: 'switch', label: t('Include TV episodes'), value: !!r.episodes },
    ]
    s.push(secF(t('Everything that matches'), f))
    // The count's note carries the anchor smartlist-count: CollectionsPage puts the preview posters under it.
    if (v.preview) s.push(anchored(secN('', tn('{count} match', '{count} matches', v.preview.count)), 'smartlist-count'))
  }
  if (v.canShare) s.push(secF(t('Sharing'), [{ id: 'shared', kind: 'switch', label: t('Share with the other profiles'), value: v.shared, hint: t('Each sees what their own limits allow.') }]))
  if (v.error) s.push(secN('', v.error, true))
  return mk('smartlist', t('Collections · Smart playlist'), v.name || (v.canChooseKind ? t('New collection') : t('Edit collection')), '', s, [], '')
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
