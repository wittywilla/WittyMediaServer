// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The guide's rules that don't need the page: which set of channels it shows, how it is cut into
// pages, what dialling a number does, when a programme can be watched from its start, and the
// sums behind putting a set's channels in order. Nothing here is imported from elsewhere, so it
// is tested on its own (guideLogic.test.mjs). docs/CHANNELS.md, "The guide", says what it does.

// ------------------------------------------------------------------ sets

export type SetKind = 'all' | 'favourites' | 'custom' | 'source' | 'group'
// One entry of GET /api/tv/sets: which channels the guide shows, in which order.
export type TvSet = { id: string; kind: SetKind; name: string; count: number; shared?: boolean; editable?: boolean; mine?: boolean; driver?: string }

export const ALL_SET = 'all'

// The set to show: the one this device chose, while the server still has it; otherwise all channels.
export function chooseSet(saved: string | null | undefined, sets: readonly TvSet[]): string {
  if (saved && sets.some((s) => s.id === saved)) return saved
  return ALL_SET
}

// The set before or after this one, going round at either end (C, [ and ], a remote's colour keys).
export function stepSet(sets: readonly TvSet[], current: string, dir: 1 | -1): string {
  if (!sets.length) return ALL_SET
  const i = sets.findIndex((s) => s.id === current)
  if (i < 0) return sets[dir > 0 ? 0 : sets.length - 1].id
  return sets[(i + dir + sets.length) % sets.length].id
}

export type SetSection = { title: string; sets: TvSet[] }

// The picker's sections, in the server's order, with only the sets whose name has the words typed.
export function sectionsOf(sets: readonly TvSet[], q = ''): SetSection[] {
  const words = q.trim().toLowerCase().split(/\s+/).filter(Boolean)
  const hit = (s: TvSet) => words.every((w) => s.name.toLowerCase().includes(w))
  const of = (title: string, test: (s: TvSet) => boolean): SetSection => ({ title, sets: sets.filter((s) => test(s) && hit(s)) })
  return [
    of('Channels', (s) => s.kind === 'all' || s.kind === 'favourites'),
    of('Your sets', (s) => s.kind === 'custom' && s.mine !== false),
    of('Shared sets', (s) => s.kind === 'custom' && s.mine === false),
    of('Sources', (s) => s.kind === 'source'),
    of('Groups', (s) => s.kind === 'group'),
  ].filter((x) => x.sets.length > 0)
}

// The sets this person can put a channel in.
export const editableSets = (sets: readonly TvSet[]) => sets.filter((s) => s.kind === 'custom' && s.editable)

// "set:7" is 7 in the address of /api/tv/sets/{id}.
export const setNumber = (id: string) => id.replace(/^set:/, '')

// ------------------------------------------------------------------ pages

// How many rows at a time: 0 is all of them, scrolled through.
export type PageSize = 0 | 25 | 50 | 75 | 100
export const PAGE_SIZES: readonly PageSize[] = [25, 50, 75, 100, 0]
export const pageSizeName = (n: PageSize) => (n ? String(n) : 'All')

export function readPageSize(v: unknown): PageSize {
  const n = Number(v)
  return (PAGE_SIZES as readonly number[]).includes(n) ? (n as PageSize) : 0
}

// The first row of the page that holds this row.
export function pageStartOf(row: number, size: PageSize): number {
  if (!size || row <= 0) return 0
  return Math.floor(row / size) * size
}

export function pageCount(total: number, size: PageSize): number {
  if (total <= 0) return 0
  return size ? Math.ceil(total / size) : 1
}

// How many rows the page starting there really has: the last one can be short.
export function rowsOnPage(start: number, size: PageSize, total: number): number {
  if (!size) return Math.max(0, total)
  return Math.max(0, Math.min(size, total - start))
}

// The row a page key lands on: the same place on the page before or after, or as near as there is.
export function stepPage(row: number, size: PageSize, total: number, dir: 1 | -1): number {
  if (total <= 0) return 0
  if (!size) return row
  const start = pageStartOf(row, size) + dir * size
  if (start < 0) return 0
  if (start >= total) return total - 1
  return Math.min(total - 1, start + (row - pageStartOf(row, size)))
}

// "Channels 51–100 of 10,423".
export function pageLabel(start: number, size: PageSize, total: number, fmt: (n: number) => string = (n) => n.toLocaleString()): string {
  if (total <= 0) return 'No channels'
  if (!size) return `${fmt(total)} ${total === 1 ? 'channel' : 'channels'}`
  const last = start + rowsOnPage(start, size, total)
  return `Channels ${fmt(start + 1)}–${fmt(last)} of ${fmt(total)}`
}

// ------------------------------------------------------------------ dialling

export type DialOutcome = { kind: 'here'; row: number } | { kind: 'elsewhere'; row: number } | { kind: 'none' }

// Whether a number not found among the channels shown is worth looking for among all of them:
// only when something narrows what's shown.
export const widerLookup = (setId: string, q: string, inView: number) => inView < 0 && (setId !== ALL_SET || q.trim() !== '')

// What dialling comes to: the row in what's shown; or, in all channels, a row to offer to switch to.
export function dialOutcome(inView: number, inAll: number | null): DialOutcome {
  if (inView >= 0) return { kind: 'here', row: inView }
  if (inAll != null && inAll >= 0) return { kind: 'elsewhere', row: inAll }
  return { kind: 'none' }
}

// ------------------------------------------------------------------ catch-up

type CatchupChannel = { kind?: string; catchupDays?: number; encrypted?: string } | null | undefined
type CatchupProgramme = { kind?: string; start: number; end: number } | null | undefined

// "Watch from the start": a programme that has started, on a channel whose source keeps an archive,
// no further back than it keeps.
export function canCatchup(ch: CatchupChannel, p: CatchupProgramme, now: number): boolean {
  if (!ch || !p || ch.encrypted || !ch.catchupDays || ch.catchupDays <= 0) return false
  if (p.kind !== 'broadcast') return false
  return p.start <= now && p.start >= now - ch.catchupDays * 86400
}

// The one player's name for a stream from a programme's start: the channel, and the programme's times.
export const catchupKey = (key: string, start: number, end: number) => `${key}@${Math.floor(start)}-${Math.floor(end)}`
export function parseCatchupKey(k: string): { key: string; start: number; end: number } | null {
  const m = /^(.+)@(\d+)-(\d+)$/.exec(k)
  return m ? { key: m[1], start: Number(m[2]), end: Number(m[3]) } : null
}
export const channelOf = (k: string) => parseCatchupKey(k)?.key ?? k

// ------------------------------------------------------------------ putting channels in order

// The list with one entry moved to another place.
export function moveItem<T>(list: readonly T[], from: number, to: number): T[] {
  const out = list.slice()
  if (from < 0 || from >= out.length) return out
  const at = Math.max(0, Math.min(out.length - 1, to))
  const [it] = out.splice(from, 1)
  out.splice(at, 0, it)
  return out
}

// In or out of a set: added at the end, so what was there keeps its order.
export function toggleIn(list: readonly string[], key: string, on: boolean): string[] {
  const has = list.includes(key)
  if (on) return has ? list.slice() : [...list, key]
  return has ? list.filter((k) => k !== key) : list.slice()
}

// Every one of these that isn't in the set yet, at the end, in the order given.
export function addAll(list: readonly string[], keys: readonly string[]): string[] {
  const have = new Set(list)
  const out = list.slice()
  for (const k of keys) if (!have.has(k)) { have.add(k); out.push(k) }
  return out
}

export function removeAll(list: readonly string[], keys: readonly string[]): string[] {
  const gone = new Set(keys)
  return list.filter((k) => !gone.has(k))
}

type Findable = { number: string; name: string; group?: string }
// The channels with that typed: a number it starts with, or words in its name or group.
export function filterChannels<T extends Findable>(list: readonly T[], q: string): T[] {
  const text = q.trim().toLowerCase()
  if (!text) return list.slice()
  return list.filter((c) => c.number.startsWith(text) || c.name.toLowerCase().includes(text) || (c.group || '').toLowerCase().includes(text))
}

// ------------------------------------------------------------------ what this device remembers

export type GuidePrefs = { set?: string; pageSize: PageSize }

// Kept per server and account, as the guide's look is per account.
export const prefsKey = (server: string, user: number | string | null | undefined) => `wms.tv.guide.${server || 'here'}.${user ?? 'none'}`

export function readGuidePrefs(raw: string | null | undefined): GuidePrefs {
  let v: Record<string, unknown> = {}
  try { v = raw ? JSON.parse(raw) : {} } catch { v = {} }
  if (!v || typeof v !== 'object') v = {}
  return { set: typeof v.set === 'string' && v.set ? v.set : undefined, pageSize: readPageSize(v.pageSize) }
}
