// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What each player reports to a remote (docs/REMOTE-PROTOCOL.md, "State"), as plain rules
// with nothing from the page so they can be tested (playerStates.test.mjs). Each player
// says honestly what it has: a station has no length, a disc's menu no place, a book no clock.
import type { RemoteState } from './device'
import type { PlayerKind } from './capabilities'
import { mirror, reported } from './queue.ts'

type Report = Omit<RemoteState, 'v'>
type Sound = { volume: number; muted: boolean }

// ---------------------------------------------------------------- the audio player

export type AudioTold = {
  queue: { id: number }[]; index: number; current: { id: number } | null; playing: boolean; retrying: boolean
  duration: number; rate: number; sound: Sound; live: boolean; cdActive: boolean
}

// Which of the protocol's players the audio player is being, from what it has loaded.
export const audioPlayerKind = (p: { cdActive: boolean; live: boolean }): PlayerKind => (p.cdActive ? 'cd' : p.live ? 'radio' : 'audio')

// audioState: music, audiobooks, podcasts, stations and a CD. A station has no length and
// one speed. A CD is one item (the disc): its tracks aren't library items, so no queue is
// reported, and the place and length are the track's.
export function audioState(p: AudioTold, positionSec: number, disc: { id: number; trackLen: number } | null): Report {
  const cur = p.current
  if (!cur) return { status: 'idle' }
  const st: Report = {
    status: p.playing ? 'playing' : p.retrying ? 'buffering' : 'paused', itemId: p.cdActive && disc ? disc.id : cur.id,
    volume: p.sound.volume, muted: p.sound.muted, rate: p.live || p.cdActive ? 1 : p.rate, screen: 'music',
  }
  if (!p.live) { st.positionSec = Math.max(0, positionSec); st.durationSec = p.cdActive && disc ? disc.trackLen : p.duration }
  if (!p.cdActive) Object.assign(st, reported(mirror(p.queue.map((q) => q.id), p.index)))
  return st
}

// ---------------------------------------------------------------- discs

type DiscTrack = { n: number; language: string }
export type DiscTold = {
  itemId: number; stopped: boolean; waiting: boolean
  // What the session is showing: nothing yet, a menu (moving or still), or a title.
  pres: null | { kind: string; media?: { type: string }; title?: { durationSec: number; audioStream: number; subtitleStream: number; audio: DiscTrack[]; subtitles: DiscTrack[] } }
}
type Media = { paused: boolean; currentTime: number; playbackRate: number; volume: number; muted: boolean }

// discState: in a menu nothing has a length or a place; in a title the disc's own stream
// numbers are the track ids.
export function discState<T extends DiscTrack>(d: DiscTold & { pres: null | { title?: { audio: T[]; subtitles: T[] } } }, v: Media | null, label: (t: T) => string): Report {
  const title = d.pres?.kind === 'title' ? d.pres.title : undefined
  const base = { itemId: d.itemId, volume: v?.volume, muted: v?.muted }
  if (!d.pres || !v) return { ...base, status: 'buffering', screen: 'player' }
  if (!title) return { ...base, status: d.pres.media?.type === 'hls' && v.paused ? 'paused' : 'playing', screen: 'disc-menu' }
  return {
    ...base, screen: 'player',
    status: d.stopped || v.paused ? 'paused' : d.waiting ? 'buffering' : 'playing',
    positionSec: v.currentTime, durationSec: title.durationSec, rate: v.playbackRate || 1,
    audio: String(title.audioStream), subtitle: title.subtitleStream < 0 ? 'off' : String(title.subtitleStream),
    audioTracks: (title.audio as T[]).map((a) => ({ id: String(a.n), label: label(a), lang: a.language })),
    subtitleTracks: (title.subtitles as T[]).map((s) => ({ id: String(s.n), label: label(s), lang: s.language })),
  }
}

// ---------------------------------------------------------------- live TV

type TrackOpt = { id: number; label: string; active?: boolean }
export type TvTold = {
  // What the server said is on, where in it the channel was then, and when it said so (seconds).
  now: { itemId?: number; start: number; end: number } | null | undefined
  positionSec: number
  serverTime: number
  behindSec: number // how far behind live this viewer is
  ready: boolean
  audio: TrackOpt[]
  subtitles: TrackOpt[]
}

// tvState: a channel isn't a library item. When the programme on now is one (a channel made
// from the library), that item and how far in it is; otherwise no item, which the server
// lists as idle with this screen.
export function tvState(t: TvTold, v: { paused: boolean; volume: number; muted: boolean } | null, nowSec: number): Report {
  const st: Report = { status: !v || !t.ready ? 'buffering' : v.paused ? 'paused' : 'playing', volume: v?.volume, muted: v?.muted, rate: 1, screen: 'tv' }
  const p = t.now
  if (p?.itemId && p.end > p.start) {
    st.itemId = p.itemId
    st.durationSec = p.end - p.start
    st.positionSec = Math.max(0, Math.min(st.durationSec, t.positionSec + (nowSec - t.serverTime) - t.behindSec))
  }
  if (t.audio.length) { st.audioTracks = t.audio.map((a) => ({ id: String(a.id), label: a.label })); st.audio = String(t.audio.find((a) => a.active)?.id ?? t.audio[0].id) }
  if (t.subtitles.length) { st.subtitleTracks = t.subtitles.map((s) => ({ id: String(s.id), label: s.label })); st.subtitle = String(t.subtitles.find((s) => s.active)?.id ?? 'off') }
  return st
}

// ---------------------------------------------------------------- books, pictures, games

// bookState: no time and nothing that plays, so `paused`, with the reader's place and length
// as the reader counts them (pages, or chapters for an ebook): the same numbers the server
// keeps as its progress. `place` is null until the reader has said.
export function bookState(itemId: number, place: { position: number; total: number } | null): Report {
  return { status: 'paused', itemId, positionSec: place?.position || 0, durationSec: place?.total || 0, screen: 'reader' }
}

// photoState: `playing` while a slideshow runs. No place, no length.
export const photoState = (itemId: number, slideshow: boolean): Report => ({ status: slideshow ? 'playing' : 'paused', itemId, screen: 'photos' })

export const gameState = (itemId: number): Report => ({ status: 'playing', itemId, screen: 'game' })
