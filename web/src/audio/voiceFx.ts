// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Karaoke's voice settings: what can be done to the singer's part and to the music under it
// once a song is in separate parts. The rules only: the controls, their ranges, the presets,
// what's kept and how it reads back. No sound here, so it's tested on its own; fxChain.ts
// turns these into audio nodes.

export type FxId =
  | 'lowCut' | 'bass' | 'presence' | 'air' | 'compress' | 'pan' | 'double'
  | 'echo' | 'echoTime' | 'echoRepeats' | 'reverb' | 'room'
  | 'musicBass' | 'musicTreble' | 'pocket'
export type Fx = Record<FxId, number>

// Labels are marked with tk() and translated where they're shown; fxValue takes the app's t() (English here, so the tests need
// nothing else). No i18n import: node tests load this file.
const tk = <S extends string>(english: S): S => english
type Say = (english: string, vars?: Record<string, string | number | null | undefined>) => string
const english: Say = (s, v) => s.replace(/\{(\w+)\}/g, (m, k: string) => (v && v[k] != null ? String(v[k]) : m))

type Control = { id: FxId; label: string; min: number; max: number; step: number; unit: string; help: string }
export type FxGroup = { title: string; controls: Control[] }

// Every control, in the order the panel shows them. Off is each one's default, so a song sounds
// as it was made until someone changes something.
export const FX_GROUPS: FxGroup[] = [
  { title: tk('Voice tone'), controls: [
    { id: 'lowCut', label: tk('Low cut'), min: 20, max: 400, step: 10, unit: 'Hz', help: tk('Takes out rumble and breath below this') },
    { id: 'bass', label: tk('Warmth'), min: -12, max: 12, step: 1, unit: 'dB', help: tk('The low end of the voice') },
    { id: 'presence', label: tk('Presence'), min: -12, max: 12, step: 1, unit: 'dB', help: tk('Brings the words forward') },
    { id: 'air', label: tk('Air'), min: -12, max: 12, step: 1, unit: 'dB', help: tk('Brightness and breath on top') },
  ] },
  { title: tk('Voice dynamics and place'), controls: [
    { id: 'compress', label: tk('Compression'), min: 0, max: 100, step: 5, unit: '%', help: tk('Evens out loud and quiet lines') },
    { id: 'pan', label: tk('Pan'), min: -100, max: 100, step: 5, unit: '', help: tk('Left or right of centre') },
    { id: 'double', label: tk('Doubling'), min: 0, max: 100, step: 5, unit: '%', help: tk('A second, slightly late voice, for a fuller sound') },
  ] },
  { title: tk('Voice echo and reverb'), controls: [
    { id: 'echo', label: tk('Echo'), min: 0, max: 100, step: 5, unit: '%', help: tk('How loud the echo is') },
    { id: 'echoTime', label: tk('Echo time'), min: 60, max: 900, step: 20, unit: 'ms', help: tk('How long before it comes back') },
    { id: 'echoRepeats', label: tk('Echo repeats'), min: 0, max: 80, step: 5, unit: '%', help: tk('How much each echo echoes again') },
    { id: 'reverb', label: tk('Reverb'), min: 0, max: 100, step: 5, unit: '%', help: tk('The sound of a room') },
    { id: 'room', label: tk('Room size'), min: 0.3, max: 5, step: 0.1, unit: 's', help: tk('From a small room to a hall') },
  ] },
  { title: tk('Background Music'), controls: [
    { id: 'musicBass', label: tk('Bass'), min: -12, max: 12, step: 1, unit: 'dB', help: tk('The low end of the music') },
    { id: 'musicTreble', label: tk('Treble'), min: -12, max: 12, step: 1, unit: 'dB', help: tk('The top of the music') },
    { id: 'pocket', label: tk('Room for the voice'), min: 0, max: 12, step: 1, unit: 'dB', help: tk('Dips the music where voices sit, so the voice comes through') },
  ] },
]

export const FX_CONTROLS = FX_GROUPS.flatMap((g) => g.controls)

export const FX_OFF: Fx = {
  lowCut: 20, bass: 0, presence: 0, air: 0, compress: 0, pan: 0, double: 0,
  echo: 0, echoTime: 320, echoRepeats: 30, reverb: 0, room: 1.6,
  musicBass: 0, musicTreble: 0, pocket: 0,
}

// Starting points; any control can be moved after choosing one.
export const FX_PRESETS: { id: string; name: string; fx: Partial<Fx> }[] = [
  { id: 'off', name: tk('As recorded'), fx: {} },
  { id: 'clear', name: tk('Clear voice'), fx: { lowCut: 100, presence: 4, air: 2, compress: 40, pocket: 4 } },
  { id: 'studio', name: tk('Studio'), fx: { lowCut: 80, bass: 1, presence: 3, air: 3, compress: 55, reverb: 18, room: 1.1, double: 15 } },
  { id: 'hall', name: tk('Concert hall'), fx: { lowCut: 80, presence: 2, compress: 30, reverb: 45, room: 3.6 } },
  { id: 'stadium', name: tk('Stadium'), fx: { lowCut: 100, presence: 3, compress: 50, echo: 25, echoTime: 420, echoRepeats: 35, reverb: 40, room: 4.5 } },
  { id: 'slapback', name: tk('Slapback echo'), fx: { lowCut: 90, presence: 2, compress: 35, echo: 35, echoTime: 120, echoRepeats: 10 } },
  { id: 'radio', name: tk('Old radio'), fx: { lowCut: 380, bass: -8, presence: 8, air: -12, compress: 70 } },
  { id: 'duet', name: tk('Duet spread'), fx: { double: 60, pan: -30, reverb: 15, room: 1.4 } },
]

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))

// Each value within its control's range, anything unknown or broken back to off.
export function cleanFx(raw: unknown): Fx {
  const out = { ...FX_OFF }
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return out
  const r = raw as Record<string, unknown>
  for (const c of FX_CONTROLS) {
    const v = r[c.id]
    if (typeof v === 'number' && Number.isFinite(v)) out[c.id] = clamp(v, c.min, c.max)
  }
  return out
}

export const presetFx = (id: string): Fx => cleanFx({ ...FX_OFF, ...(FX_PRESETS.find((p) => p.id === id)?.fx || {}) })

// The preset these settings are, or '' once something's been moved off one.
export function whichPreset(fx: Fx): string {
  for (const p of FX_PRESETS) {
    const want = presetFx(p.id)
    if (FX_CONTROLS.every((c) => want[c.id] === fx[c.id])) return p.id
  }
  return ''
}

export const isOff = (fx: Fx) => FX_CONTROLS.every((c) => fx[c.id] === FX_OFF[c.id])

// Kept by account on the device, as the Mix's levels are.
export const fxKey = (userId: number | string | undefined) => `wms.karaoke.fx.${userId ?? 0}`
export function decodeFx(text: string | null | undefined): Fx {
  try { return cleanFx(JSON.parse(text || '{}')) } catch { return { ...FX_OFF } }
}
export const encodeFx = (fx: Fx) => JSON.stringify(cleanFx(fx))

export function fxValue(c: Pick<Control, 'unit' | 'id'>, v: number, t: Say = english): string {
  if (c.id === 'pan') return v === 0 ? t('Centre') : v < 0 ? t('{n}% left', { n: Math.abs(v) }) : t('{n}% right', { n: Math.abs(v) })
  if (c.id === 'lowCut' && v <= 20) return t('Off')
  if (c.unit === 'dB') return `${v > 0 ? '+' : ''}${v} dB`
  if (c.unit === 's') return `${v.toFixed(1)} s`
  return `${v}${c.unit === '%' ? '%' : c.unit ? ' ' + c.unit : ''}`
}

// What the nodes are set to, from the controls.
//   - compression: threshold from 0 to -40 dB and ratio from 1 to 8 as the control goes up, with
//     make-up gain so turning it up doesn't just make it quieter.
//   - echo and reverb are sends: the dry voice stays at full, the wet one is added.
export function fxNodes(fx: Fx) {
  const c = fx.compress / 100
  return {
    lowCutHz: fx.lowCut,
    bassDb: fx.bass, presenceDb: fx.presence, airDb: fx.air,
    threshold: c > 0 ? -40 * c : 0, ratio: 1 + 7 * c, makeUp: Math.pow(10, (12 * c * 0.5) / 20),
    pan: fx.pan / 100,
    double: (fx.double / 100) * 0.7,
    echo: (fx.echo / 100) * 0.8, echoTime: fx.echoTime / 1000, echoFeedback: fx.echoRepeats / 100,
    reverb: (fx.reverb / 100) * 0.9, room: fx.room,
    musicBassDb: fx.musicBass, musicTrebleDb: fx.musicTreble, pocketDb: fx.pocket ? -fx.pocket : 0,
  }
}
