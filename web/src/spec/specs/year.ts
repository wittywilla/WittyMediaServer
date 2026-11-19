// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, R, mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecPageData, SpecRow, SpecSection } from '../types.ts'
import { t, tk, tn } from '../../sdui/words.ts'

// Your year (canvas key "year"): this profile's year from the server's own record (/me/review). Field id: year.
// Chip id: clear.
export interface Entry { id?: number; showId?: number; name: string; sub?: string; seconds: number }
export interface Review {
  year: number; seconds: number; items: number; days: number; longestRun: number; busiestDay?: number; busiestSeconds?: number
  byType: Entry[]; byMonth: number[]; topTitles: Entry[]; topShows: Entry[]; topArtists: Entry[]; topCreators: Entry[]; topGenres: Entry[]; years: number[]
}

// Marked with tk: shown with t().
export const TYPE_NAMES: Record<string, string> = { movies: tk('Movies'), shows: tk('TV shows'), anime: tk('Anime'), discs: tk('Discs'), videos: tk('Home videos'), music: tk('Music'), musicvideos: tk('Music videos'), audiobooks: tk('Audiobooks'), podcasts: tk('Podcasts'), channels: tk('Creator videos'), shorts: tk('Vertical videos'), radio: tk('Radio'), photos: tk('Photo clips') }
export const MONTHS = [tk('Jan'), tk('Feb'), tk('Mar'), tk('Apr'), tk('May'), tk('Jun'), tk('Jul'), tk('Aug'), tk('Sep'), tk('Oct'), tk('Nov'), tk('Dec')]

export function hours(sec: number): string {
  if (sec < 3600) return t('{minutes} min', { minutes: Math.round(sec / 60) })
  const h = sec / 3600
  return t('{hours} h', { hours: h < 10 ? h.toFixed(1) : Math.round(h) })
}

// 20240104 as "4 January".
export function dayName(d?: number): string {
  if (!d) return ''
  return new Date(Date.UTC(Math.floor(d / 10000), Math.floor(d / 100) % 100 - 1, d % 100)).toLocaleDateString([], { timeZone: 'UTC', day: 'numeric', month: 'long' })
}

const top = (list: Entry[], link?: (e: Entry) => string | null): SpecRow[] => list.map((e, i) => ({ ...R(e.name, hours(e.seconds), i === 0), ...(link?.(e) ? { to: link(e) as string } : {}) }))

export function yearSpec(r: Review): SpecPageData {
  const sections: SpecSection[] = []
  const year = String(r.year)
  if (r.years.length > 1) sections.push(secF('', [{ id: 'year', kind: 'select', label: t('Year'), value: year, options: r.years.map((y) => ({ value: String(y), label: String(y) })) }]))
  if (r.seconds === 0) {
    sections.push(secN('', t('As you watch and listen, this fills up. It’s worked out on this server from your profile’s own record, and only you can see it.')))
    return mk('year', t('You · Your year'), t('Nothing yet for {year}', { year }), t('Totals, streak, biggest day, months and top titles.'), sections)
  }
  sections.push(secR(t('Totals'), [
    R(t('Watching and listening'), hours(r.seconds), true),
    R(r.items === 1 ? t('Title') : t('Titles'), r.items.toLocaleString('en')),
    R(r.days === 1 ? t('Day') : t('Days'), String(r.days)),
    R(t('Longest run'), tn('{count} day in a row', '{count} days in a row', r.longestRun)),
    ...(r.busiestDay ? [R(t('Biggest day'), `${hours(r.busiestSeconds || 0)} · ${dayName(r.busiestDay)}`)] : []),
  ]))
  const peak = r.byMonth.reduce((a, v, i) => (v > r.byMonth[a] ? i : a), 0)
  sections.push(secN(t('Month by month'), t('Busiest month: {month}, {time}.', { month: t(MONTHS[peak]), time: hours(r.byMonth[peak] || 0) })))
  const lists: [string, Entry[], ((e: Entry) => string | null)?][] = [
    [t('What you spent it on'), r.byType.map((e) => ({ ...e, name: TYPE_NAMES[e.name] ? t(TYPE_NAMES[e.name]) : e.name }))],
    [t('Shows'), r.topShows, (e) => (e.showId ? `/show/${e.showId}` : null)],
    [t('Movies and videos'), r.topTitles, (e) => (e.id ? `/item/${e.id}` : null)],
    [t('Artists'), r.topArtists], [t('Creators'), r.topCreators], [t('Genres'), r.topGenres],
  ]
  for (const [h, list, link] of lists) if (list.length) sections.push(secR(h, top(list, link)))
  sections.push(secP('', [{ ...Cc(t('Clear my history')), id: 'clear' }]))
  return mk('year', t('You · Your year'), t('Your {year}', { year }), t('Totals, streak, biggest day, months and top titles.'), sections)
}
