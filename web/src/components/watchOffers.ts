// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Where a title can be watched, as GET /api/media/{id}/where-to-watch answers: what this
// library has of it (in which quality, and every version) beside the outside services that
// offer it in a region, grouped by how (docs/METADATA.md, Where to watch). This turns the
// answer into what the title's page shows, and decides what each kind of account is told
// when there are no services to show. Nothing here asks the server anything.

export type WatchVersion = { itemId: number; label?: string; kind: string; quality?: string; width?: number; height?: number; container?: string; videoCodec?: string; sizeBytes?: number }
export type WatchOffer = { providerId?: number; key: string; name: string; type: string; logo?: string; priority?: number }
export type WatchAnswer = {
  inLibrary: boolean; itemId?: number; showId?: number
  local?: { itemId?: number; showId?: number; title?: string; quality?: string; versions?: WatchVersion[] | null } | null
  region: string; tmdb?: { type: string; tmdbId: string } | null; source?: string; link?: string
  groups?: { type: string; providers: WatchOffer[] }[] | null; providers?: WatchOffer[] | null
  reason?: string; attribution?: string; fetchedAt?: number; cached?: boolean
}

// The groups in the order they are shown, with the words for each: what someone already
// pays for first, then free, then paid.
export const WATCH_GROUPS: { type: string; title: string }[] = [
  { type: 'stream', title: 'Stream' }, { type: 'free', title: 'Free' }, { type: 'ads', title: 'Free with adverts' },
  { type: 'rent', title: 'Rent' }, { type: 'buy', title: 'Buy' },
]

export type WatchProvider = { key: string; name: string; logo: string }
export type WatchCopy = { itemId: number; name: string; detail: string }
// note: why there are no services, when the person should be told. "no-key" is only ever
// for an administrator, who is pointed at where the key goes.
export type WatchNote = '' | 'no-key' | 'not-matched' | 'unavailable' | 'nowhere'
export type WatchView = {
  quality: string; copies: WatchCopy[]
  groups: { type: string; title: string; providers: WatchProvider[] }[]
  link: string; attribution: string; region: string; asOf: number; note: WatchNote
}

// An address from outside is only used when it is a plain https one.
export function httpsUrl(u?: string): string {
  if (!u) return ''
  try { const p = new URL(u); return p.protocol === 'https:' ? p.href : '' } catch { return '' }
}

const CODECS: Record<string, string> = { h264: 'H.264', hevc: 'HEVC', h265: 'HEVC', av1: 'AV1', vp9: 'VP9', mpeg2video: 'MPEG-2', mpeg4: 'MPEG-4', vc1: 'VC-1' }

export function sizeWords(bytes?: number): string {
  if (!bytes || bytes <= 0) return ''
  if (bytes >= 1e9) return (bytes / 1e9).toFixed(bytes >= 1e10 ? 0 : 1) + ' GB'
  return Math.max(1, Math.round(bytes / 1e6)) + ' MB'
}

// copyDetail: one version in a line: 1080p · HEVC · MKV · 4.2 GB.
export function copyDetail(v: WatchVersion): string {
  const disc = v.kind.startsWith('dvd') || v.kind.startsWith('bluray')
  const codec = v.videoCodec ? CODECS[v.videoCodec.toLowerCase()] || v.videoCodec.toUpperCase() : ''
  return [v.quality, disc ? (v.kind.endsWith('-iso') ? 'Disc image' : 'Disc folder') : codec, disc ? '' : (v.container || '').split(',')[0].toUpperCase(), sizeWords(v.sizeBytes)].filter(Boolean).join(' · ')
}

// watchView: null when there is nothing worth a section: no services, nothing to say about
// why, and a single copy (an ordinary account on a server with no key is shown nothing,
// unless the title has several versions to choose between).
export function watchView(a: WatchAnswer | null, who: { admin: boolean }): WatchView | null {
  if (!a) return null
  const versions = a.local?.versions || []
  const copies: WatchCopy[] = versions.map((v, i) => ({ itemId: v.itemId, name: v.label || (i === 0 ? 'This copy' : `Version ${i + 1}`), detail: copyDetail(v) }))
  const byType = new Map((a.groups || []).map((g) => [g.type, g.providers || []]))
  const groups = WATCH_GROUPS.map((g) => ({
    ...g,
    providers: (byType.get(g.type) || []).filter((p) => p.key && p.name).map((p) => ({ key: p.key, name: p.name, logo: httpsUrl(p.logo) })),
  })).filter((g) => g.providers.length > 0)
  let note: WatchNote = ''
  if (!groups.length) {
    if (a.reason === 'no-api-key') note = who.admin ? 'no-key' : ''
    else if (a.reason === 'not-matched') note = who.admin ? 'not-matched' : ''
    else if (a.reason === 'unavailable') note = 'unavailable'
    else if (!a.reason) note = 'nowhere'
  }
  const quality = a.inLibrary ? a.local?.quality || '' : ''
  if (!groups.length && !note && copies.length < 2) return null
  return { quality, copies: a.inLibrary ? copies : [], groups, link: groups.length ? httpsUrl(a.link) : '', attribution: groups.length || note === 'nowhere' ? a.attribution || '' : '', region: a.region, asOf: groups.length ? a.fetchedAt || 0 : 0, note }
}
