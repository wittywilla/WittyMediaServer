// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, R, mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecPageData, SpecRow, SpecSection } from '../types.ts'

// Your year (canvas key "year"): this profile's year from the server's own record (/me/review). Field id: year.
// Chip id: clear.
export interface Entry { id?: number; showId?: number; name: string; sub?: string; seconds: number }
export interface Review {
  year: number; seconds: number; items: number; days: number; longestRun: number; busiestDay?: number; busiestSeconds?: number
  byType: Entry[]; byMonth: number[]; topTitles: Entry[]; topShows: Entry[]; topArtists: Entry[]; topCreators: Entry[]; topGenres: Entry[]; years: number[]
}

export const TYPE_NAMES: Record<string, string> = { movies: 'Movies', shows: 'TV shows', anime: 'Anime', discs: 'Discs', videos: 'Home videos', music: 'Music', musicvideos: 'Music videos', audiobooks: 'Audiobooks', podcasts: 'Podcasts', channels: 'Creator videos', shorts: 'Vertical videos', radio: 'Radio', photos: 'Photo clips' }
export const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

export function hours(sec: number): string {
  if (sec < 3600) return `${Math.round(sec / 60)} min`
  const h = sec / 3600
  return `${h < 10 ? h.toFixed(1) : Math.round(h).toLocaleString('en')} h`
}

// 20240104 as "4 January".
export function dayName(d?: number): string {
  if (!d) return ''
  return new Date(Date.UTC(Math.floor(d / 10000), Math.floor(d / 100) % 100 - 1, d % 100)).toLocaleDateString([], { timeZone: 'UTC', day: 'numeric', month: 'long' })
}

const top = (list: Entry[], link?: (e: Entry) => string | null): SpecRow[] => list.map((e, i) => ({ ...R(e.name, hours(e.seconds), i === 0), ...(link?.(e) ? { to: link(e) as string } : {}) }))

export function yearSpec(r: Review): SpecPageData {
  const sections: SpecSection[] = []
  if (r.years.length > 1) sections.push(secF('', [{ id: 'year', kind: 'select', label: 'Year', value: String(r.year), options: r.years.map((y) => ({ value: String(y), label: String(y) })) }]))
  if (r.seconds === 0) {
    sections.push(secN('', 'As you watch and listen, this fills up. It’s worked out on this server from your profile’s own record, and only you can see it.'))
    return mk('year', 'You · Your year', `Nothing yet for ${r.year}`, 'Totals, streak, biggest day, months and top titles.', sections)
  }
  sections.push(secR('Totals', [
    R('Watching and listening', hours(r.seconds), true),
    R(r.items === 1 ? 'Title' : 'Titles', r.items.toLocaleString('en')),
    R(r.days === 1 ? 'Day' : 'Days', String(r.days)),
    R('Longest run', `${r.longestRun} ${r.longestRun === 1 ? 'day' : 'days'} in a row`),
    ...(r.busiestDay ? [R('Biggest day', `${hours(r.busiestSeconds || 0)} · ${dayName(r.busiestDay)}`)] : []),
  ]))
  const peak = r.byMonth.reduce((a, v, i) => (v > r.byMonth[a] ? i : a), 0)
  sections.push(secN('Month by month', `Busiest month: ${MONTHS[peak]}, ${hours(r.byMonth[peak] || 0)}.`))
  const lists: [string, Entry[], ((e: Entry) => string | null)?][] = [
    ['What you spent it on', r.byType.map((e) => ({ ...e, name: TYPE_NAMES[e.name] || e.name }))],
    ['Shows', r.topShows, (e) => (e.showId ? `/show/${e.showId}` : null)],
    ['Movies and videos', r.topTitles, (e) => (e.id ? `/item/${e.id}` : null)],
    ['Artists', r.topArtists], ['Creators', r.topCreators], ['Genres', r.topGenres],
  ]
  for (const [h, list, link] of lists) if (list.length) sections.push(secR(h, top(list, link)))
  sections.push(secP('', [{ ...Cc('Clear my history'), id: 'clear' }]))
  return mk('year', 'You · Your year', `Your ${r.year}`, 'Totals, streak, biggest day, months and top titles.', sections)
}
