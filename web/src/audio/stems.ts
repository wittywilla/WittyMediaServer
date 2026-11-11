// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Karaoke with a song's real parts ("stems"): the rules, with no sound in them. What a
// song has, which choices that gives the singer, which parts play for each choice and how
// loud, where the music is while they play, and where to pick the song up when going back
// to the ordinary mix. stemEngine.ts makes the sound; this decides. docs/LYRICS-KARAOKE.md
// says it for people.

export const STEM_NAMES = ['vocals', 'instrumental', 'drums', 'bass', 'other'] as const
export type StemName = (typeof STEM_NAMES)[number]

// One part, as GET /media/{id}/stems lists it.
export type StemInfo = { name: string; url: string; mime: string; size: number; multitrack?: boolean; source: string; approximate?: boolean }
export type SeparationWay = { available: boolean; reason?: string; task?: string; note?: string; error?: string }
export type StemsAnswer = { stems: StemInfo[]; names: string[]; separation: { approximate: SeparationWay; external: SeparationWay } }

export const STEM_LABELS: Record<StemName, string> = { vocals: 'Voice', instrumental: 'Music without the voice', drums: 'Drums', bass: 'Bass', other: 'Everything else' }

const isStemName = (n: string): n is StemName => (STEM_NAMES as readonly string[]).includes(n)

// ---------------------------------------------------------------- what a song has

export type Sources = {
  vocals: boolean; instrumental: boolean; drums: boolean; bass: boolean; other: boolean
  approximate: boolean // the instrumental is FFmpeg's guess, not a real stem
  multitrack: boolean // a Native Instruments .stem.mp4 is listed
}

export function sourcesOf(stems: StemInfo[]): Sources {
  const s: Sources = { vocals: false, instrumental: false, drums: false, bass: false, other: false, approximate: false, multitrack: false }
  for (const st of stems) {
    if (st.multitrack || st.name === 'multitrack') { s.multitrack = true; continue }
    if (!isStemName(st.name)) continue
    s[st.name] = true
    if (st.name === 'instrumental' && st.approximate) s.approximate = true
  }
  return s
}

// The parts of sources that are there, in the order they're shown.
export const present = (s: Sources): StemName[] => STEM_NAMES.filter((n) => s[n])

// What stands for "the music without the voice". Drums, bass and the rest together are the
// whole of it, and give a level each, so they're used when all three are there. Two of
// the three would leave a hole, so then it is the instrumental as one part.
export type Backing = 'parts' | 'instrumental' | 'none'
export function backingOf(s: Sources): Backing {
  if (s.drums && s.bass && s.other) return 'parts'
  if (s.instrumental) return 'instrumental'
  return 'none'
}

// ---------------------------------------------------------------- what the singer can choose

// on: the song as it is. lower and lowest: the browser's own trick of cancelling the middle
// of the mix, as before. lower and off, with stems: the real voice part turned down, or
// left out. instrumental: there is music without the voice but no voice part to bring
// back in (the approximate instrumental, or a karaoke track someone supplied).
export type ChoiceId = 'on' | 'lower' | 'lowest' | 'off' | 'instrumental'
export type VoiceChoice = {
  id: ChoiceId; name: string
  engine: 'mix' | 'stems' // the ordinary player, or the parts played together
  vocalCut: number // for the ordinary player: how much of the middle to cancel, 0 to 1
  approximate?: boolean
}

const ON: VoiceChoice = { id: 'on', name: 'Voice on', engine: 'mix', vocalCut: 0 }
const CUT_LOWER: VoiceChoice = { id: 'lower', name: 'Voice lower', engine: 'mix', vocalCut: 0.6 }
const CUT_LOWEST: VoiceChoice = { id: 'lowest', name: 'Voice lowest', engine: 'mix', vocalCut: 1 }

// usable: this browser could load and decode the parts. Until it has tried, that's taken
// to be so; once it has failed, the song is treated as one without stems.
export function voiceChoices(s: Sources, usable = true): VoiceChoice[] {
  const backing = usable ? backingOf(s) : 'none'
  if (backing !== 'none' && s.vocals) {
    return [ON, { id: 'lower', name: 'Voice lower', engine: 'stems', vocalCut: 0 }, { id: 'off', name: 'Voice off', engine: 'stems', vocalCut: 0 }]
  }
  const out = [ON, CUT_LOWER, CUT_LOWEST]
  if (backing !== 'none') out.push({ id: 'instrumental', name: s.approximate && backing === 'instrumental' ? 'Instrumental (approximate)' : 'Instrumental', engine: 'stems', vocalCut: 0, approximate: s.approximate && backing === 'instrumental' })
  return out
}

// The next song may not offer what this one did. The nearest thing is chosen: no voice
// stays as little voice as can be had, and anything else that's missing goes back to the song.
export function carryChoice(wanted: ChoiceId, choices: VoiceChoice[]): ChoiceId {
  const has = (id: ChoiceId) => choices.some((c) => c.id === id)
  if (has(wanted)) return wanted
  const nearest: Partial<Record<ChoiceId, ChoiceId[]>> = { off: ['instrumental', 'lowest'], instrumental: ['off', 'lowest'], lowest: ['off', 'instrumental'] }
  return (nearest[wanted] || []).find(has) || 'on'
}

// ---------------------------------------------------------------- how loud each part is

// A level is where a slider is, 0 to 1. Each part that's there has one.
export type Levels = Record<StemName, number>
export const VOICE_LOWER_LEVEL = 0.5 // a quarter of its strength: 12 dB down

const clamp01 = (v: number) => (Number.isFinite(v) ? Math.min(1, Math.max(0, v)) : 0)

// Where the sliders start for a choice. The music without the voice plays in full, by the
// parts when all three are there and otherwise as the one instrumental, never both: that
// would play the music twice. A part that isn't playing starts at nothing, and can be
// brought up by hand (more drums on top, say).
export function defaultLevels(s: Sources, choice: ChoiceId): Levels {
  const backing = backingOf(s)
  const parts = backing === 'parts' ? 1 : 0
  return {
    vocals: !s.vocals || choice === 'off' || choice === 'instrumental' ? 0 : choice === 'lower' ? VOICE_LOWER_LEVEL : 1,
    instrumental: backing === 'instrumental' ? 1 : 0,
    drums: parts, bass: parts, other: parts,
  }
}

// A slider's place to what the sound is multiplied by. Squared, as loudness is heard:
// half-way down the slider sounds about half as loud (12 dB down), not barely different.
export const gainFor = (level: number) => { const l = clamp01(level); return l * l }

// What each part that's there is multiplied by. Parts that aren't there aren't named.
export function stemGains(s: Sources, levels: Levels): Partial<Record<StemName, number>> {
  const out: Partial<Record<StemName, number>> = {}
  for (const n of present(s)) out[n] = gainFor(levels[n])
  return out
}

// Whether the sliders are still where the choice put them.
export function levelsAreDefault(s: Sources, choice: ChoiceId, levels: Levels): boolean {
  const d = defaultLevels(s, choice)
  return present(s).every((n) => Math.abs(d[n] - clamp01(levels[n])) < 0.005)
}

// ---------------------------------------------------------------- what to fetch

// Decoded sound is big: two channels of 32-bit samples. A browser tab that takes a
// gigabyte for one song gets closed by a phone, so there is a budget.
export const DECODED_BUDGET_BYTES = 640 * 1024 * 1024
export const decodedBytes = (seconds: number, sampleRate = 48000) => Math.max(0, seconds) * sampleRate * 2 * 4

// Which parts to fetch and decode. All that are there when they fit, so every slider is
// live; otherwise only the ones that will be heard; and if even those are too much, none,
// with ok false: the song is too long for this.
export function stemsToLoad(s: Sources, levels: Levels, seconds: number, budget = DECODED_BUDGET_BYTES): { ok: boolean; names: StemName[]; all: boolean } {
  const each = decodedBytes(seconds)
  const all = present(s)
  if (all.length * each <= budget) return { ok: all.length > 0, names: all, all: true }
  const heard = all.filter((n) => levels[n] > 0)
  if (heard.length > 0 && heard.length * each <= budget) return { ok: true, names: heard, all: false }
  return { ok: false, names: [], all: false }
}

// ---------------------------------------------------------------- where the music is

// Every part starts at the same moment on the same clock, from the same place in the song:
// that is all that keeps them together, and it keeps them together exactly. lead is a
// moment ahead, so the start can be scheduled rather than be late.
export const START_LEAD_SEC = 0.06
export function startPlan(now: number, position: number, duration: number): { when: number; offset: number } {
  return { when: now + START_LEAD_SEC, offset: carryPosition(position, duration) }
}

// Where the song is, in seconds, while the parts play. The clock says what has been handed
// to the sound card; what is heard is behind that by the output's delay (a Bluetooth
// speaker's can be a quarter of a second), and the words follow what is heard. Before the
// scheduled start it is still at the place it starts from.
export function enginePosition(st: { offset: number; startedAt: number; playing: boolean }, now: number, outputLatency: number, duration: number): number {
  if (!st.playing) return st.offset
  const heard = now - (Number.isFinite(outputLatency) && outputLatency > 0 ? Math.min(outputLatency, 1) : 0)
  const at = st.offset + Math.max(0, heard - st.startedAt)
  return duration > 0 ? Math.min(at, duration) : at
}

// The place to carry from one way of playing to the other: the same second of the song,
// kept inside it. Right at the end it stays a moment short, so the song that is taking
// over ends by itself and the queue moves on as it always does.
export const END_GUARD_SEC = 0.05
export function carryPosition(position: number, duration: number): number {
  if (!Number.isFinite(position) || position <= 0) return 0
  if (!(duration > 0)) return position
  return Math.min(position, Math.max(0, duration - END_GUARD_SEC))
}

// ---------------------------------------------------------------- a multitrack file

// A Native Instruments .stem.mp4 holds five sound tracks: the whole mix first, then four
// parts, which by the format's custom are drums, bass, the rest and the voice. A file can
// name its tracks otherwise, and then the names win.
export type TrackPlan = { mix: number; parts: Partial<Record<StemName, number>> } | null

export function multitrackPlan(labels: string[]): TrackPlan {
  if (labels.length < 5) return null
  const parts: Partial<Record<StemName, number>> = {}
  const taken = new Set<number>([0])
  const named: [StemName, RegExp][] = [['vocals', /voc|voice|sing|acap/i], ['drums', /drum|perc|beat/i], ['bass', /bass/i]]
  for (const [name, re] of named) {
    const i = labels.findIndex((l, n) => n > 0 && n < 5 && !taken.has(n) && re.test(l))
    if (i > 0) { parts[name] = i; taken.add(i) }
  }
  const custom: StemName[] = ['drums', 'bass', 'other', 'vocals']
  // Whatever wasn't named takes the places left, in the format's order.
  const free = [1, 2, 3, 4].filter((n) => !taken.has(n))
  for (const name of custom) if (parts[name] === undefined && free.length) parts[name] = free.shift()
  return { mix: 0, parts }
}

// Which tracks are switched on. A browser can only switch a track on or off, so a part
// plays in full or not at all. With every part on it is the whole mix, which is the one
// track made to be heard alone.
export function tracksEnabled(plan: NonNullable<TrackPlan>, levels: Levels, count: number): boolean[] {
  const on = new Array<boolean>(count).fill(false)
  const parts = Object.entries(plan.parts) as [StemName, number][]
  if (parts.every(([name]) => levels[name] > 0)) { on[plan.mix] = true; return on }
  for (const [name, i] of parts) if (i < count) on[i] = levels[name] > 0
  return on
}

// ---------------------------------------------------------------- saying why not

// Why the server can't make something for this song, in words.
export function reasonWords(reason: string | undefined, what: 'approximate' | 'external'): string {
  switch (reason) {
    case 'no-ffmpeg': return 'This server has no FFmpeg, which is what makes it.'
    case 'not-configured': return 'No stem separator is set up on this server. Its owner names one where the server starts.'
    case 'admin-only': return 'An administrator can have the server\'s separator split this song into real stems.'
    case 'not-a-song': return 'Only songs have stems.'
    case 'not-a-file': return 'This song isn\'t a file in a library, so there is nothing to make stems from.'
    default: return what === 'approximate' ? 'This server can\'t make one for this song.' : 'This server can\'t separate this song.'
  }
}

export function fmtSize(bytes: number): string {
  if (!(bytes > 0)) return ''
  return bytes >= 1024 * 1024 ? `${(bytes / 1024 / 1024).toFixed(bytes >= 10 * 1024 * 1024 ? 0 : 1)} MB` : `${Math.max(1, Math.round(bytes / 1024))} KB`
}
