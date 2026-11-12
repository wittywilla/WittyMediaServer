// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// The rules of karaoke from a song's separate parts: how loud each part plays for where the
// sliders are, what is remembered, what the screen says, and when the parts have wandered
// from the song's own clock. No sound and no page here, so it's tested on its own. The same
// rules are in the phone and TV apps: docs/CLIENT-API.md, "Karaoke that sets itself up".

export type Stem = { id: string; label: string; role: 'vocals' | 'music' | 'full' | string; url: string; mime?: string; defaultGain: number }
export type KaraokeAnswer = {
  state: 'ready' | 'preparing' | 'unavailable' | string
  quality: string
  mix: 'parts' | 'blend' | string
  stems: Stem[]
  version: string
  improving: boolean
  progress: number
  canImprove?: boolean
  reason?: string
  message?: string
  pollSec: number
}

// Where the sliders are: 0 to 1 by stem id. `vocals` is the big slider in both mixes.
export type Levels = Record<string, number>

export const clamp01 = (n: number) => (Number.isFinite(n) ? Math.min(1, Math.max(0, n)) : 1)

const voiceStem = (a: Pick<KaraokeAnswer, 'mix' | 'stems'>) => a.stems.find((s) => s.role === (a.mix === 'blend' ? 'full' : 'vocals'))

// What a slider starts at: the server's default for that stem.
export function defaultLevel(a: Pick<KaraokeAnswer, 'mix' | 'stems'>, id: string) {
  const st = id === 'vocals' ? voiceStem(a) : a.stems.find((s) => s.id === id)
  return clamp01(st?.defaultGain ?? 1)
}

export const levelOf = (a: Pick<KaraokeAnswer, 'mix' | 'stems'>, levels: Levels, id: string) => (id in levels ? clamp01(levels[id]) : defaultLevel(a, id))

// The sliders under More: one a stem, or Vocals and Music for a blend.
export function sliders(a: Pick<KaraokeAnswer, 'mix' | 'stems'>): { id: string; label: string }[] {
  if (a.mix === 'blend') return [{ id: 'vocals', label: 'Vocals' }, { id: 'instrumental', label: 'Music' }]
  return a.stems.filter((s) => s.role !== 'full').map((s) => ({ id: s.role === 'vocals' ? 'vocals' : s.id, label: s.label || s.id }))
}

// How loud each stem plays, by stem id. In a blend the whole song and the instrumental are
// crossed in a straight line: they hold the same music, so the two always add up to it.
export function gainsFor(a: Pick<KaraokeAnswer, 'mix' | 'stems'>, levels: Levels, sing: boolean): Record<string, number> {
  const v = sing ? 0 : levelOf(a, levels, 'vocals')
  const out: Record<string, number> = {}
  if (a.mix === 'blend') {
    const m = levelOf(a, levels, 'instrumental')
    for (const s of a.stems) out[s.id] = s.role === 'full' ? m * v : m * (1 - v)
    return out
  }
  for (const s of a.stems) out[s.id] = s.role === 'vocals' ? v : s.role === 'full' ? 0 : levelOf(a, levels, s.id)
  return out
}

// Can this answer be played at all: a blend needs its two, parts need a voice and some music.
export function playable(a: KaraokeAnswer | null): a is KaraokeAnswer {
  if (!a || a.state !== 'ready' || !Array.isArray(a.stems)) return false
  if (a.mix === 'blend') return a.stems.length === 2 && a.stems.some((s) => s.role === 'full') && a.stems.some((s) => s.role === 'music')
  return a.mix === 'parts' && a.stems.some((s) => s.role === 'vocals') && a.stems.some((s) => s.role === 'music')
}

// What's kept, by account: where the sliders were. Sing isn't: it starts off.
export const levelsKey = (userId: number | string | undefined) => `wms.karaoke.levels.${userId ?? 0}`
export function encodeLevels(l: Levels) {
  const out: Levels = {}
  for (const [k, v] of Object.entries(l)) if (/^[a-z]{1,16}$/.test(k) && Number.isFinite(v)) out[k] = Math.round(clamp01(v) * 100) / 100
  return JSON.stringify(out)
}
export function decodeLevels(text: string | null | undefined): Levels {
  try {
    const raw = JSON.parse(text || '{}')
    const out: Levels = {}
    if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
      for (const [k, v] of Object.entries(raw)) if (/^[a-z]{1,16}$/.test(k) && typeof v === 'number' && Number.isFinite(v)) out[k] = clamp01(v)
    }
    return out
  } catch { return {} }
}

export const qualityName = (a: KaraokeAnswer) => (a.quality === 'stems' ? 'Separated' : 'Quick version')
const pct = (n: number) => (n >= 0 && n <= 100 ? ` ${Math.round(n)}%` : '')

// The one line that says where things stand, or '' when there's nothing to say.
export function statusLine(a: KaraokeAnswer | null, fallback = ''): string {
  if (fallback) return fallback
  if (!a) return ''
  if (a.state === 'preparing') return `Preparing karaoke…${pct(a.progress)}`
  if (a.state === 'unavailable') return a.reason === 'not-made' ? '' : a.message || 'There\'s no karaoke version of this song.'
  if (a.improving) return `Preparing a better version…${pct(a.progress)}`
  return ''
}

// When to ask the server again, in milliseconds; 0 for never. A failed ask backs off.
export function pollDelay(a: KaraokeAnswer | null, failures: number): number {
  if (failures > 0) return Math.min(30000, 3000 * 2 ** Math.min(failures, 4))
  if (!a || !a.pollSec || a.pollSec < 0) return 0
  return Math.min(60, Math.max(1, a.pollSec)) * 1000
}

// Keeping the parts with the song. The parts were started at `startCtx` on the audio
// engine's clock from `startAt` seconds into the song; the song's own player says it's at
// `songAt`. More than DRIFT apart, they're started again from where the song is.
export const DRIFT = 0.08
export const LEAD = 0.06 // how far ahead a start is scheduled, so every part starts on the same sample
export function partsAt(startAt: number, startCtx: number, ctxNow: number, rate = 1) {
  return startAt + Math.max(0, ctxNow - startCtx) * rate
}
export function drifted(startAt: number, startCtx: number, ctxNow: number, songAt: number, rate = 1) {
  if (ctxNow < startCtx) return false // not started yet
  return Math.abs(partsAt(startAt, startCtx, ctxNow, rate) - songAt) > DRIFT
}
// Where a start scheduled now begins in the parts, and when on the engine's clock.
export function startPlan(songAt: number, ctxNow: number, rate = 1) {
  return { when: ctxNow + LEAD, offset: Math.max(0, songAt + LEAD * rate) }
}

// The most sound held decoded at once. A longer song, or one played faster or slower, has
// its parts streamed instead (streamedParts), each by a player of its own kept with the song.
export const MAX_SECONDS = 15 * 60

// Whether the parts are streamed rather than decoded whole: decoded parts start on the same
// sample but can't change speed without changing key, and hold the whole song in memory.
export function streamedParts(rate: number, durationSec: number): boolean {
  return rate !== 1 || (Number.isFinite(durationSec) && durationSec > MAX_SECONDS)
}

// Keeping a streamed part with the song: the song is `ahead` seconds in front of the part
// (behind when negative). Close enough is left alone; a little way off is caught up by
// playing up to 5% faster or slower, which no one hears; further off is a jump.
export const STREAM_CLOSE = 0.03
export const STREAM_JUMP = 0.3
export function streamStep(rate: number, ahead: number): { seek: boolean; rate: number } {
  if (!Number.isFinite(ahead) || Math.abs(ahead) > STREAM_JUMP) return { seek: true, rate }
  if (Math.abs(ahead) <= STREAM_CLOSE) return { seek: false, rate }
  const nudge = Math.max(-0.05, Math.min(0.05, ahead * 0.5))
  return { seek: false, rate: rate * (1 + nudge) }
}
