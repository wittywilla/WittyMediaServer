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
  // 'installable': no separator yet, but an administrator could install one in Settings (a cleaner version).
  separator?: string
  pollSec: number
}

// Where the sliders are: 0 to 1 by slider id (MIX).
export type Levels = Record<string, number>

export const clamp01 = (n: number) => (Number.isFinite(n) ? Math.min(1, Math.max(0, n)) : 1)

const voiceStem = (a: Pick<KaraokeAnswer, 'mix' | 'stems'>) => a.stems.find((s) => s.role === (a.mix === 'blend' ? 'full' : 'vocals'))

// The three sliders, the same whatever version is playing: the singer, everything else (every other part
// together, so the voice can stand out), and the whole song.
export type MixId = 'vocals' | 'music' | 'track'
export const MIX: readonly { id: MixId; label: string }[] = [{ id: 'vocals', label: 'Voice' }, { id: 'music', label: 'Background Music' }, { id: 'track', label: 'Track' }]

// What a slider starts at: the server's default for the voice, and full for the others.
export function defaultLevel(a: Pick<KaraokeAnswer, 'mix' | 'stems'>, id: string) {
  return id === 'vocals' ? clamp01(voiceStem(a)?.defaultGain ?? 1) : 1
}

export const levelOf = (a: Pick<KaraokeAnswer, 'mix' | 'stems'>, levels: Levels, id: string) => (id in levels ? clamp01(levels[id]) : defaultLevel(a, id))

// What can be hidden from karaoke's bar: the Mix's three sliders, the key, the words' timing and the Voice settings button.
export const KAR_PARTS: readonly { id: string; label: string }[] = [...MIX, { id: 'key', label: 'Key' }, { id: 'timing', label: 'Timing of the words' }, { id: 'fx', label: 'Voice settings' }]

export function sliders(_a?: Pick<KaraokeAnswer, 'mix' | 'stems'>) { return MIX }

// How loud each stem plays, by stem id, for Voice v, Background Music m and Track t.
//   - Parts: the voice at v, every other part at m (times the server's own level for it), all of it times t.
//   - A blend (the whole song, and the music without the voice): the whole song at v and the music at m - v,
//     all of it times t. The music is in both, so it adds up to v + (m - v) = m, and the voice to v. With the
//     music turned down below the voice, the music is taken away from the whole song: what's left is the voice.
export function gainsFor(a: Pick<KaraokeAnswer, 'mix' | 'stems'>, levels: Levels, sing: boolean): Record<string, number> {
  const v = sing ? 0 : levelOf(a, levels, 'vocals')
  const m = levelOf(a, levels, 'music')
  const t = levelOf(a, levels, 'track')
  const out: Record<string, number> = {}
  if (a.mix === 'blend') {
    for (const s of a.stems) out[s.id] = s.role === 'full' ? t * v : t * (m - v)
    return out
  }
  for (const s of a.stems) out[s.id] = s.role === 'vocals' ? t * v : s.role === 'full' ? 0 : t * m * clamp01(s.defaultGain ?? 1)
  return out
}

// The same mix as two sends a stem: how much of it goes to the voice's chain and how much to the music's (voiceFx.ts).
// Parts: the vocals stem to the voice, the others to the music. A blend has no voice of its own, so the voice is the
// whole song less the music: `full` to the voice at t·v and `instrumental` to it at −t·v, and `instrumental` to the music
// at t·m. Added up it is gainsFor exactly, so with every voice setting off it sounds the same as before.
export type Route = { voice: number; music: number }
export function routesFor(a: Pick<KaraokeAnswer, 'mix' | 'stems'>, levels: Levels, sing: boolean): Record<string, Route> {
  const v = sing ? 0 : levelOf(a, levels, 'vocals')
  const m = levelOf(a, levels, 'music')
  const t = levelOf(a, levels, 'track')
  const out: Record<string, Route> = {}
  for (const s of a.stems) {
    if (a.mix === 'blend') out[s.id] = s.role === 'full' ? { voice: t * v, music: 0 } : { voice: -t * v, music: t * m }
    else out[s.id] = s.role === 'vocals' ? { voice: t * v, music: 0 } : s.role === 'full' ? { voice: 0, music: 0 } : { voice: 0, music: t * m * clamp01(s.defaultGain ?? 1) }
  }
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
