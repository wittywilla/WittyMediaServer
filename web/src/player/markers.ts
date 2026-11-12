// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Where a video's skippable parts are, as GET /api/media/{id}/markers answers: from what
// an administrator set, a .skip.json or .edl beside the file, chapter names, what comparing
// the season's episodes found, and the show's own intro. This picks what to offer now.

export type Range = { startSec: number; endSec: number }
export type Markers = { intro?: Range | null; recap?: Range | null; credits?: Range | null; commercials?: Range[] | null }
export type Skip = { kind: 'intro' | 'recap' | 'credits' | 'commercial'; label: string; to: number }

const LABEL: Record<Skip['kind'], string> = { intro: 'Skip intro', recap: 'Skip recap', credits: 'Skip credits', commercial: 'Skip adverts' }

// skipAt is the part playing at now, if skipping it is worth a button: more than a second
// of it is left, and for credits there is something after them (credits that run to the
// end are what "next episode" is for). null when there is nothing to skip.
export function skipAt(m: Markers | null, now: number, duration: number): Skip | null {
  if (!m) return null
  const inside = (r?: Range | null) => !!r && r.endSec > r.startSec && now >= r.startSec && now < r.endSec - 1
  if (inside(m.recap)) return { kind: 'recap', label: LABEL.recap, to: m.recap!.endSec }
  if (inside(m.intro)) return { kind: 'intro', label: LABEL.intro, to: m.intro!.endSec }
  for (const c of m.commercials || []) if (inside(c)) return { kind: 'commercial', label: LABEL.commercial, to: c.endSec }
  if (inside(m.credits) && duration > 0 && m.credits!.endSec < duration - 5) return { kind: 'credits', label: LABEL.credits, to: m.credits!.endSec }
  return null
}

// ---------------------------------------------------------------- skipping by itself

// What a profile has asked to be skipped without pressing anything. It is kept with the
// account's other choices (/me/ui, as "autoSkip"), so it follows the person to every device.
export type AutoSkipPrefs = { intro: boolean; recap: boolean; credits: boolean; adverts: boolean }
export const NO_AUTO_SKIP: AutoSkipPrefs = { intro: false, recap: false, credits: false, adverts: false }
export const AUTO_SKIP_KEY = 'autoSkip'

// autoSkipPrefs reads the stored choice, whatever is there: anything but true is off.
export function autoSkipPrefs(raw: unknown): AutoSkipPrefs {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  return { intro: r.intro === true, recap: r.recap === true, credits: r.credits === true, adverts: r.adverts === true }
}
export const anyAutoSkip = (p: AutoSkipPrefs) => p.intro || p.recap || p.credits || p.adverts

const SKIPPED: Record<Skip['kind'], string> = { intro: 'Skipped intro', recap: 'Skipped recap', credits: 'Skipped credits', commercial: 'Skipped adverts' }
export const skippedLabel = (kind: Skip['kind']) => SKIPPED[kind]

type Part = { kind: Skip['kind']; key: string; startSec: number; endSec: number }

// The parts of a video in the order skipAt offers them, each with a name of its own.
function partsOf(m: Markers): Part[] {
  const out: Part[] = []
  const add = (kind: Skip['kind'], r?: Range | null) => { if (r && r.endSec > r.startSec) out.push({ kind, key: `${kind}:${r.startSec}:${r.endSec}`, startSec: r.startSec, endSec: r.endSec }) }
  add('recap', m.recap)
  add('intro', m.intro)
  for (const c of m.commercials || []) add('commercial', c)
  add('credits', m.credits)
  return out
}

// What the player remembers between looks at the clock: where the picture was last, a jump
// into a part that hasn't been understood yet, and the parts that are not to be skipped
// (again): each is skipped once at most.
export type AutoSkipState = { last: number | null; pending: { key: string; back: boolean } | null; spent: string[] }
export const autoSkipStart = (): AutoSkipState => ({ last: null, pending: null, spent: [] })

// next: instead of a jump, go on to the next episode (credits with an episode after them).
export type AutoSkipAct = { kind: Skip['kind']; key: string; label: string; from: number; to: number; next: boolean }

// party: "follow" is someone in a watch party who may not control it, "control" someone who may.
export type AutoSkipWhere = { duration: number; hasNext: boolean; party: 'none' | 'control' | 'follow'; maxStep?: number }

// autoSkipStep is called with each new position while the picture is really playing, and
// says whether to skip now. The rules:
//  - a part is skipped when playing runs into it, or starts inside it, and only if the
//    profile asked for that kind;
//  - each part is skipped once: after an Undo, it plays;
//  - seeking back into a part means someone wants to see it, so it plays, and is not
//    skipped later either. A jump forward into one is skipped once playing carries on there
//    (a jump is only believed when the next position agrees with it: while a stream is
//    being swapped for another, one position can be nonsense);
//  - in a watch party only someone who may control it skips, as that seeks for everyone,
//    and going on to the next episode is left to the room;
//  - credits go on to the next episode when there is one; without one they are skipped
//    only if something comes after them.
export function autoSkipStep(state: AutoSkipState, m: Markers | null, prefs: AutoSkipPrefs, now: number, where: AutoSkipWhere): { state: AutoSkipState; act: AutoSkipAct | null } {
  const last = state.last
  const step = now - (last ?? now)
  const natural = last == null || (step >= 0 && step <= (where.maxStep ?? 3))
  const stay = (pending: AutoSkipState['pending'] = null, spent = state.spent) => ({ state: { last: now, pending, spent }, act: null })
  if (!m || where.party === 'follow' || !anyAutoSkip(prefs)) return stay()
  const wanted = (k: Skip['kind']) => (k === 'commercial' ? prefs.adverts : prefs[k])
  const part = partsOf(m).find((p) => wanted(p.kind) && now >= p.startSec && now < p.endSec - 1)
  if (!part) return stay()
  if (state.spent.includes(part.key)) return stay()
  if (!natural) return stay({ key: part.key, back: step < 0 })
  // The same position again (the screen was only drawn again) confirms nothing.
  if (state.pending?.key === part.key && step === 0 && last != null) return stay(state.pending)
  // Playing on inside a part that was jumped into: back means leave it be, forward means skip.
  if (state.pending?.key === part.key && state.pending.back) return stay(null, [...state.spent, part.key])
  let next = false
  if (part.kind === 'credits') {
    if (where.hasNext) {
      if (where.party !== 'none') return stay()
      next = true
    } else if (!(where.duration > 0 && part.endSec < where.duration - 5)) return stay()
  }
  const act: AutoSkipAct = { kind: part.kind, key: part.key, label: SKIPPED[part.kind], from: now, to: part.endSec, next }
  // The jump is the player's own: the next position follows on from where it lands.
  return { state: { last: next ? now : part.endSec, pending: null, spent: [...state.spent, part.key] }, act }
}

// ---------------------------------------------------------------- setting them by hand

// What GET /api/media/{id}/markers answers beyond the ranges: where each came from.
export type MarkerAnswer = Markers & { source?: string; sources?: Record<string, string> | null }
export type RangeText = { start: string; end: string }
export type MarkerForm = { intro: RangeText; recap: RangeText; credits: RangeText; commercials: RangeText[] }
export type MarkerPut = { reset: true } | { intro?: PutRange; recap?: PutRange; credits?: PutRange; commercials?: PutRange[] }
type PutRange = { start: number; end?: number }

const SOURCE_WORDS: Record<string, string> = {
  manual: 'Set by hand', 'skip.json': 'From a .skip.json beside the file', edl: 'From an .edl beside the file',
  chapters: 'From the file’s chapters', detected: 'Found by comparing episodes', show: 'The show’s own intro',
}
// sourceWords: where a kind of marker came from, in words ('' when there is none).
export const markerSourceWords = (m: MarkerAnswer | null, kind: 'intro' | 'recap' | 'credits' | 'commercials') => SOURCE_WORDS[m?.sources?.[kind] || ''] || ''

// clockText writes seconds as m:ss (h:mm:ss past an hour), keeping tenths when there are any.
export function clockText(sec: number): string {
  const t = Math.round(sec * 10) / 10
  const whole = Math.floor(t)
  const tenth = Math.round((t - whole) * 10)
  const h = Math.floor(whole / 3600), m = Math.floor((whole % 3600) / 60), s = whole % 60
  return (h ? `${h}:${String(m).padStart(2, '0')}` : `${m}`) + `:${String(s).padStart(2, '0')}` + (tenth ? `.${tenth}` : '')
}

// clockSeconds reads 90, 1:30, 1:02:03 or 1:30.5. null for nothing typed, NaN for anything else.
export function clockSeconds(text: string): number | null {
  const t = text.trim()
  if (!t) return null
  if (!/^\d+(:\d{1,2}){0,2}(\.\d+)?$/.test(t)) return NaN
  return t.split(':').reduce((a, x) => a * 60 + Number(x), 0)
}

const rangeText = (r?: Range | null): RangeText => (r && r.endSec > r.startSec ? { start: clockText(r.startSec), end: clockText(r.endSec) } : { start: '', end: '' })
const blank = (r: RangeText) => !r.start.trim() && !r.end.trim()
const same = (a: RangeText, b: RangeText) => a.start.trim() === b.start.trim() && a.end.trim() === b.end.trim()

// markerForm is what the edit dialog shows: every range as text, wherever it came from.
export function markerForm(m: MarkerAnswer | null): MarkerForm {
  return { intro: rangeText(m?.intro), recap: rangeText(m?.recap), credits: rangeText(m?.credits), commercials: (m?.commercials || []).map(rangeText).filter((r) => !blank(r)) }
}

const NAMES = { intro: 'the intro', recap: 'the recap', credits: 'the credits', commercials: 'an advert break' } as const

function readRange(r: RangeText, what: string, openEnd: boolean): PutRange | string {
  const start = clockSeconds(r.start) ?? 0
  const end = clockSeconds(r.end)
  if (Number.isNaN(start) || Number.isNaN(end)) return `Type the times for ${what} as minutes and seconds, such as 1:05.`
  if (end == null) return openEnd ? { start } : `Say where ${what} ends.`
  if (end <= start) return `The end of ${what} has to come after its start.`
  return { start, end }
}

// markerPut turns the form into the body of PUT /api/media/{id}/markers. What is set by
// hand is kept whole on the server, so the body holds every kind that was already set by
// hand or has been changed here; a kind left as the files have it is left out, so it keeps
// following the files. A hand-set kind that has been emptied goes back to the files, and
// with nothing left set by hand the body is {reset: true}. error is what to tell the person.
export function markerPut(saved: MarkerAnswer | null, form: MarkerForm): { body?: MarkerPut; error?: string } {
  const was = markerForm(saved)
  const byHand = (k: string) => saved?.sources?.[k] === 'manual'
  const body: Exclude<MarkerPut, { reset: true }> = {}
  for (const k of ['intro', 'recap', 'credits'] as const) {
    if (!byHand(k) && same(form[k], was[k])) continue
    if (blank(form[k])) continue
    const r = readRange(form[k], NAMES[k], k === 'credits')
    if (typeof r === 'string') return { error: r }
    body[k] = r
  }
  const breaks = form.commercials.filter((r) => !blank(r))
  const changed = breaks.length !== was.commercials.length || breaks.some((r, i) => !same(r, was.commercials[i]))
  if ((byHand('commercials') || changed) && breaks.length) {
    const list: PutRange[] = []
    for (const b of breaks) {
      const r = readRange(b, NAMES.commercials, false)
      if (typeof r === 'string') return { error: r }
      list.push(r)
    }
    body.commercials = list.sort((a, b) => a.start - b.start)
  }
  return { body: Object.keys(body).length ? body : { reset: true } }
}

// anyByHand: something is set by hand, so there is something to go back from.
export const anyByHand = (m: MarkerAnswer | null) => Object.values(m?.sources || {}).includes('manual')
