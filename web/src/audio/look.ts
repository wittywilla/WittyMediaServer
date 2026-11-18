// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useSyncExternalStore } from 'react'
import { api } from '../api'

// How a song's album art looks, in the full player, the bar and karaoke (the Look panel, LookPanel.tsx). One look for
// every song, and any song can have its own, which wins. Kept with the music player's other choices in /me/ui
// (musicPlayer.look and musicPlayer.songLooks), written by the full player; the bar reads it from here.

// art: the cover's shape. vinyl and cd spin while the music plays; tilt leans towards the pointer; drift pans and
// zooms slowly across the picture.
export type ArtStyle = 'square' | 'circle' | 'vinyl' | 'cd' | 'polaroid' | 'tilt' | 'drift'
// backdrop: what is behind the player when neither the Canvas video nor the visualizer is: a glow of the cover's
// colour, the cover blurred to fill the screen, the whole cover dimmed, or nothing.
export type Backdrop = 'glow' | 'blur' | 'cover' | 'plain'
// canvasPlace: where a song's Canvas video plays, when Canvas is chosen: behind everything, in the cover's place
// (cropped to its shape, in every view), or both.
export type CanvasPlace = 'behind' | 'art' | 'both'
export type Look = { art: ArtStyle; backdrop: Backdrop; canvasPlace: CanvasPlace; reflection: boolean; pulse: boolean }
// A song's own look: only what was changed for it. show: Canvas on or off for this song alone.
export type SongLook = Partial<Look> & { show?: 'art' | 'canvas' }

export const DEFAULT_LOOK: Look = { art: 'square', backdrop: 'glow', canvasPlace: 'behind', reflection: true, pulse: false }

export const ART_STYLES: { id: ArtStyle; name: string; note: string }[] = [
  { id: 'square', name: 'Cover', note: 'The cover as it is' },
  { id: 'circle', name: 'Round', note: 'Cut to a circle' },
  { id: 'vinyl', name: 'Record', note: 'On a record that turns as it plays' },
  { id: 'cd', name: 'Disc', note: 'Printed on a spinning disc' },
  { id: 'polaroid', name: 'Snapshot', note: 'An instant photo, with the song’s name' },
  { id: 'tilt', name: 'Float', note: 'Leans towards the pointer' },
  { id: 'drift', name: 'Drift', note: 'Pans slowly across the picture' },
]
export const BACKDROPS: { id: Backdrop; name: string }[] = [
  { id: 'glow', name: 'Glow' }, { id: 'blur', name: 'Blurred cover' }, { id: 'cover', name: 'Whole cover' }, { id: 'plain', name: 'Plain' },
]
export const CANVAS_PLACES: { id: CanvasPlace; name: string }[] = [
  { id: 'behind', name: 'Behind everything' }, { id: 'art', name: 'In the cover’s place' }, { id: 'both', name: 'Both' },
]

// The most songs that keep a look of their own; the oldest go first past it, so the setting can't grow without end.
export const MAX_SONG_LOOKS = 400

const pick = <T extends string>(v: unknown, ok: readonly { id: T }[]): T | undefined => (ok.some((o) => o.id === v) ? (v as T) : undefined)

// readLook: a look from saved data, anything unknown left out (a song's) or at the default (everyone's).
export function readSongLook(v: unknown): SongLook {
  const s = (typeof v === 'object' && v !== null ? v : {}) as Record<string, unknown>
  const out: SongLook = {}
  const art = pick(s.art, ART_STYLES), bd = pick(s.backdrop, BACKDROPS), cp = pick(s.canvasPlace, CANVAS_PLACES)
  if (art) out.art = art
  if (bd) out.backdrop = bd
  if (cp) out.canvasPlace = cp
  if (typeof s.reflection === 'boolean') out.reflection = s.reflection
  if (typeof s.pulse === 'boolean') out.pulse = s.pulse
  if (s.show === 'art' || s.show === 'canvas') out.show = s.show
  return out
}
export const readLook = (v: unknown): Look => ({ ...DEFAULT_LOOK, ...readSongLook(v) })
export function readSongLooks(v: unknown): Record<string, SongLook> {
  const out: Record<string, SongLook> = {}
  if (typeof v !== 'object' || v === null) return out
  for (const [k, x] of Object.entries(v).slice(-MAX_SONG_LOOKS)) if (/^\d+$/.test(k)) { const l = readSongLook(x); if (Object.keys(l).length) out[k] = l }
  return out
}

// lookFor: the look a song has, its own over everyone's.
export function lookFor(all: Look, songs: Record<string, SongLook>, id: number | undefined): Look & { show?: 'art' | 'canvas'; own: boolean } {
  const own = id != null ? songs[String(id)] : undefined
  return { ...all, ...(own || {}), own: !!own && Object.keys(own).length > 0 }
}

// setSongLook: a song's look changed (or, with null, forgotten), keeping the newest MAX_SONG_LOOKS.
export function setSongLook(songs: Record<string, SongLook>, id: number, patch: SongLook | null): Record<string, SongLook> {
  const key = String(id)
  const next = { ...songs }
  const now = patch === null ? undefined : { ...(next[key] || {}), ...patch }
  delete next[key] // re-added last: the newest
  if (now && Object.keys(now).length) next[key] = now
  const keys = Object.keys(next)
  for (const k of keys.slice(0, Math.max(0, keys.length - MAX_SONG_LOOKS))) delete next[k]
  return next
}

// The store the bar and karaoke read: the full player sets it as it changes, and the first reader loads it.
type State = { look: Look; songs: Record<string, SongLook>; canvas: boolean; viz: boolean }
let state: State = { look: DEFAULT_LOOK, songs: {}, canvas: true, viz: false }
const listeners = new Set<() => void>()
let loading: Promise<void> | null = null
export function publishLook(s: Partial<State>) {
  state = { ...state, ...s }
  listeners.forEach((f) => f())
}
function loadOnce() {
  loading ??= api.get<{ musicPlayer?: { look?: unknown; songLooks?: unknown; canvas?: unknown; viz?: unknown } }>('/me/ui')
    .then((u) => publishLook({ look: readLook(u.musicPlayer?.look), songs: readSongLooks(u.musicPlayer?.songLooks), canvas: u.musicPlayer?.canvas !== false, viz: u.musicPlayer?.viz === true }))
    .catch(() => { /* the defaults stay */ })
}
const subscribe = (f: () => void) => { listeners.add(f); loadOnce(); return () => { listeners.delete(f) } }
export const useLookState = () => useSyncExternalStore(subscribe, () => state)
