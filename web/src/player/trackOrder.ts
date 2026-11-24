// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The order of the subtitle and sound track lists: Off first, then the tracks in this device's
// languages, then the ones picked lately, then the rest by name. And the search that narrows a long
// list. No i18n import (node tests load it): words come from the caller.

export type Track = { id: number; title: string; sub?: string; detail?: string; active: boolean; lang?: string }
export type Group = 'off' | 'device' | 'recent' | 'other'

// "eng", "ger", "en-GB" and "EN" are all "en". Empty for "und" and nothing.
export function baseLang(code?: string | null): string {
  const c = (code || '').trim().replace('_', '-')
  if (!c || /^(und|unknown|zxx|mul)$/i.test(c)) return ''
  try { return Intl.getCanonicalLocales(c)[0].split('-')[0].toLowerCase() } catch { return c.split('-')[0].toLowerCase() }
}

const RECENT_MAX = 5
const key = (kind: string) => `wms.tracks.recent.${kind}`

export function recentLangs(kind: string): string[] {
  try { const v = JSON.parse(localStorage.getItem(key(kind)) || '[]'); return Array.isArray(v) ? v.filter((x) => typeof x === 'string') : [] } catch { return [] }
}

export function rememberLang(kind: string, code?: string | null) {
  const b = baseLang(code)
  if (!b) return
  try { localStorage.setItem(key(kind), JSON.stringify([b, ...recentLangs(kind).filter((x) => x !== b)].slice(0, RECENT_MAX))) } catch { /* storage off */ }
}

// Sorts the list into its groups. device: the languages the device and the app are set to, most wanted first.
export function orderTracks(tracks: Track[], device: string[], recent: string[], collator = new Intl.Collator()): { track: Track; group: Group }[] {
  const dev = [...new Set(device.map(baseLang).filter(Boolean))]
  const rec = [...new Set(recent.map(baseLang).filter(Boolean))].filter((l) => !dev.includes(l))
  const rank = (t: Track): [Group, number] => {
    if (t.id < 0) return ['off', 0]
    const b = baseLang(t.lang)
    const d = dev.indexOf(b)
    if (b && d >= 0) return ['device', d]
    const r = rec.indexOf(b)
    if (b && r >= 0) return ['recent', r]
    return ['other', 0]
  }
  const order: Group[] = ['off', 'device', 'recent', 'other']
  return tracks
    .map((track, i) => { const [group, n] = rank(track); return { track, group, n, i } })
    .sort((a, b) => order.indexOf(a.group) - order.indexOf(b.group) || a.n - b.n
      || (a.group === 'other' ? collator.compare(a.track.title, b.track.title) : 0) || a.i - b.i)
    .map(({ track, group }) => ({ track, group }))
}

// The search: every word typed must start a word of the title, its note or its code ("spa", "sdh", "forced").
export function matchTrack(t: Track, query: string): boolean {
  const q = query.trim().toLowerCase().split(/\s+/).filter(Boolean)
  if (!q.length) return true
  const words = [t.title, t.sub, t.detail, t.lang].filter(Boolean).join(' ').toLowerCase().split(/[\s·,()/-]+/).filter(Boolean)
  return q.every((w) => words.some((x) => x.startsWith(w)))
}

// A search box is offered from this many tracks.
export const SEARCH_FROM = 9
