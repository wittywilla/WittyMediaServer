// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, mk, secN, secR } from '../model.ts'
import type { SpecPageData, SpecRow, SpecSection } from '../types.ts'

// Collections (canvas keys "collections" and "savelist"). The shape below is what the server's /collections answers.
export interface CollectionLine { id: number; ownerId: number; owner: string; name: string; kind: 'manual' | 'smart'; shared: boolean; count: number; system?: string }

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

// The list of lists: yours and shared ones, then the smart playlists, each a link to its page. `canMake` adds New.
export function collectionsSpec(list: CollectionLine[], userId: number | undefined, canMake: boolean): SpecPageData {
  const line = (c: CollectionLine): SpecRow => ({
    id: 'c' + c.id, title: c.name, to: `/collections/${c.id}`,
    value: c.kind === 'smart' ? plural(c.count, 'match', 'match') : plural(c.count, 'title', 'titles'),
    sub: c.ownerId !== userId ? `From ${c.owner}` : c.shared ? 'Shared with the other profiles' : undefined,
  })
  const lists = list.filter((c) => c.kind !== 'smart').map(line)
  const smart = list.filter((c) => c.kind === 'smart').map(line)
  const sections: SpecSection[] = []
  if (lists.length) sections.push(secR('Lists', lists))
  if (smart.length) sections.push(secR('Smart playlists', smart))
  if (!sections.length) sections.push(secN('', canMake ? 'Gather movies, shows, albums and more from any library, or save a rule, such as unwatched movies from the 90s, as a smart playlist that keeps itself up to date.' : 'There are no collections for your profile to see yet.'))
  return mk('collections', 'Collections', 'Your lists', 'Lists you fill, and smart playlists that fill themselves.', sections, canMake ? [{ ...A1('New list or smart playlist'), id: 'new' }] : [], '')
}

// Save to a list, for one film, show or video: the lists it can go in, ticked where it is. Watch later is offered
// until it exists.
export function saveSpec(lists: { id: number; name: string }[], holding: readonly number[], hasLater: boolean): SpecPageData {
  const rows: SpecRow[] = lists.map((c) => ({ id: 'list:' + c.id, title: c.name, value: holding.includes(c.id) ? '✓' : undefined, on: holding.includes(c.id) }))
  if (!hasLater) rows.unshift({ id: 'later', title: 'Watch later', sub: 'A list made for you', on: false })
  const sections: SpecSection[] = rows.length ? [secR('Lists', rows)] : []
  return mk('savelist', 'Save to a list', 'Save to', 'Tick the lists it should be in.', sections, [{ ...A1('Done'), id: 'done' }], '')
}
