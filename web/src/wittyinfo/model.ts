// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// The .wittyinfo editor's model (docs/formats/wittyinfo.md): what the editor shows and
// changes, and the checks the server makes, said early. Nothing here draws anything, so it
// is shared by the server's editor, the player's marking bar and the wittyinfo program's
// page, and tested on its own (model.test.mjs).

import { clockSeconds, clockText, type Markers } from '../player/markers.ts'

// Marks words for translation; they're translated where they're shown, with tx(). (A local
// mark, so the tests can load this file without the app's i18n layer.)
const tk = <S extends string>(english: S): S => english

export type SegType = 'intro' | 'recap' | 'credits' | 'preview' | 'postcredits' | 'commercial' | 'custom'
export type SegAction = 'button' | 'auto' | 'mark'
export type FilterCategory = 'violence' | 'language' | 'nudity' | 'sex' | 'drugs' | 'frightening' | 'flashing' | 'discrimination' | 'other'
export type FilterAction = 'skip' | 'mute' | 'warn'

// What the file holds. Fields the editor doesn't know stay in `rest` and go back unchanged.
export type Seg = { id?: string; type: SegType; start: number; end?: number | null; label?: string; action?: SegAction; note?: string; [k: string]: unknown }
export type Scene = { id?: string; title: string; start: number; thumbnailAt?: number | null; image?: string; description?: string; [k: string]: unknown }
export type Filter = { id?: string; category: FilterCategory; severity?: number; start: number; end: number; action?: FilterAction; label?: string; [k: string]: unknown }
export type Meta = { title?: string; originalTitle?: string; sortTitle?: string; edition?: string; year?: number; overview?: string; tagline?: string; genres?: string[]; contentRating?: string; episodeKind?: string; [k: string]: unknown }
export type Playback = { startAt?: number; audioLanguage?: string; subtitleLanguage?: string; subtitleMode?: string; volumeDb?: number; aspectRatio?: string; [k: string]: unknown }
// Translation: the file's text in one language, naming parts by id (docs/formats/wittyinfo.md).
export type TextPart = { label?: string; title?: string; note?: string; description?: string; [k: string]: unknown }
export type Translation = {
  segments?: Record<string, TextPart>; scenes?: Record<string, TextPart>; filters?: Record<string, TextPart>
  metadata?: Meta; notes?: string; lyrics?: Lyrics; [k: string]: unknown
}
export type LyricLine = { start: number; end?: number; text: string; singer?: string; words?: { start: number; text: string }[]; [k: string]: unknown }
export type Lyrics = { language?: string; plain?: string; lines?: LyricLine[]; instrumental?: boolean; [k: string]: unknown }
export type Sub = { file: string; language?: string; title?: string; forced?: boolean; sdh?: boolean; [k: string]: unknown }
export type Manifest = {
  format: 'wittyinfo'; version: number; scope?: string; season?: number
  language?: string; translations?: Record<string, Translation>; subtitles?: Sub[]
  identity?: { durationSec: number; [k: string]: unknown }
  authorship?: { author?: string; created?: string; updated?: string; tool?: { name: string; version?: string }; [k: string]: unknown }
  segments?: Seg[]; absent?: SegType[]; scenes?: Scene[]; filters?: Filter[]
  metadata?: Meta; artwork?: Record<string, string | string[]>; playback?: Playback; notes?: string
  [k: string]: unknown
}
export type Problem = { where: string; what: string }
export type Match = { result: 'exact' | 'same-cut' | 'unchecked' | 'other-cut' | 'forced' | 'defaults'; usable: boolean; message?: string; deltaSec?: number }

// What GET /api/wittyinfo/{target}/{id} answers (the wittyinfo program answers the same).
export type InfoAnswer = {
  kind: string; id: number; season?: number | null
  info: Manifest | null; origin: '' | 'stored' | 'file'; path?: string; fileFound: boolean; fileError?: string; problems?: Problem[]
  match?: Match | null; force: boolean; files: { name: string; size: number; url?: string }[]; canWrite: boolean
  identity?: Manifest['identity'] | null; markers?: (Markers & { sources?: Record<string, string>; source?: string }) | null
  defaults?: Record<string, { origin: string; segments: number; error?: string }>
  chapters?: { start: number; title: string }[]; durationSec?: number
  // The newest format version this server writes, and whether the file follows a later one.
  version?: number; newer?: boolean
  // The wittyinfo program only.
  standalone?: boolean; video?: string; videoName?: string
}

export const SEG_TYPES: { value: SegType; label: string; hint: string }[] = [
  { value: 'intro', label: tk('Intro'), hint: tk('The opening titles or theme song.') },
  { value: 'recap', label: tk('Recap'), hint: tk('“Previously on…”') },
  { value: 'credits', label: tk('Credits'), hint: tk('The end credits. Leave the end empty if they run to the end.') },
  { value: 'preview', label: tk('Preview'), hint: tk('A look at the next episode.') },
  { value: 'postcredits', label: tk('After the credits'), hint: tk('A scene after the credits. Skipping the credits stops here.') },
  { value: 'commercial', label: tk('Advert break'), hint: tk('Adverts left in a recording.') },
  { value: 'custom', label: tk('Other part'), hint: tk('Anything else to skip: give it a name.') },
]
export const segLabel = (t: string) => SEG_TYPES.find((x) => x.value === t)?.label || t

export const ACTIONS: { value: SegAction; label: string; hint: string }[] = [
  { value: 'button', label: tk('Skip button'), hint: tk('Offers a button while it plays.') },
  { value: 'auto', label: tk('Skip by itself'), hint: tk('Skips it without a press, for anyone who allows that in their settings.') },
  { value: 'mark', label: tk('Only mark it'), hint: tk('Shows where it is, with no button.') },
]
export const defaultAction = (t: SegType): SegAction => (t === 'postcredits' ? 'mark' : 'button')
export const actOf = (s: Seg): SegAction => s.action || defaultAction(s.type)

export const FILTER_CATEGORIES: { value: FilterCategory; label: string }[] = [
  { value: 'violence', label: tk('Violence') }, { value: 'language', label: tk('Strong language') }, { value: 'nudity', label: tk('Nudity') },
  { value: 'sex', label: tk('Sex') }, { value: 'drugs', label: tk('Drugs') }, { value: 'frightening', label: tk('Frightening scenes') },
  { value: 'flashing', label: tk('Flashing lights') }, { value: 'discrimination', label: tk('Discrimination') }, { value: 'other', label: tk('Other') },
]
export const filterLabel = (c: string) => FILTER_CATEGORIES.find((x) => x.value === c)?.label || c
export const SEVERITIES = [{ value: 0, label: tk('Not said') }, { value: 1, label: tk('Mild') }, { value: 2, label: tk('Moderate') }, { value: 3, label: tk('Strong') }]

// The same limits as the server (pkg/wittyinfo); the server checks again.
export const LIMITS = { segments: 1000, scenes: 1000, filters: 5000, short: 200, long: 20000, maxTime: 7 * 24 * 3600 }

// ---------------------------------------------------------------- the draft

// Draft is what the editor changes: the manifest's parts, with the rest kept as it was.
export type Draft = {
  segments: Seg[]; absent: SegType[]; scenes: Scene[]; filters: Filter[]
  metadata: Meta; playback: Playback; notes: string; artwork: Record<string, string[]>
  language: string; translations: Record<string, Translation>; subtitles: Sub[]
  rest: Record<string, unknown>
}

const KNOWN = new Set(['segments', 'absent', 'scenes', 'filters', 'metadata', 'playback', 'notes', 'artwork', 'language', 'translations', 'subtitles'])

// newId gives a part an id of its own, as writers do: translations name parts by it.
export const newId = (prefix: 'seg' | 'scene' | 'filter') => `${prefix}-${Math.random().toString(16).slice(2, 10)}`
const withIds = <T extends { id?: string }>(l: T[], prefix: 'seg' | 'scene' | 'filter') => l.map((x) => (x.id ? x : { ...x, id: newId(prefix) }))

// draftOf takes a manifest apart for editing; with none, a new one starts from what is in
// force now (so editing starts from the times the player already uses, not from nothing).
export function draftOf(info: Manifest | null, now?: InfoAnswer['markers'] | null): Draft {
  const rest: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(info || {})) if (!KNOWN.has(k)) rest[k] = v
  if (!info && now?.segments) {
    return { segments: now.segments.map((s) => ({ type: s.kind as SegType, start: s.startSec, end: s.endSec, ...(s.label ? { label: s.label } : {}) })), absent: [], scenes: [], filters: [], metadata: {}, playback: {}, notes: '', artwork: {}, language: '', translations: {}, subtitles: [], rest }
  }
  return {
    segments: (info?.segments || []).map((s) => ({ ...s })), absent: [...(info?.absent || [])], scenes: (info?.scenes || []).map((s) => ({ ...s })),
    filters: (info?.filters || []).map((f) => ({ ...f })), metadata: { ...(info?.metadata || {}) }, playback: { ...(info?.playback || {}) },
    notes: info?.notes || '', artwork: picturesOf(info?.artwork), language: info?.language || '',
    translations: JSON.parse(JSON.stringify(info?.translations || {})), subtitles: (info?.subtitles || []).map((x) => ({ ...x })), rest,
  }
}

// picturesOf reads a manifest's artwork, where a role names one picture or a list of them,
// the one shown first, as lists.
export function picturesOf(art?: Record<string, string | string[]>): Record<string, string[]> {
  const out: Record<string, string[]> = {}
  for (const [role, v] of Object.entries(art || {})) {
    const list = (Array.isArray(v) ? v : [v]).filter((x) => typeof x === 'string' && x)
    if (list.length) out[role] = list
  }
  return out
}

const tidy = <T extends Record<string, unknown>>(o: T): T | undefined => {
  const out: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(o)) {
    if (v === '' || v == null || (Array.isArray(v) && v.length === 0)) continue
    out[k] = v
  }
  return Object.keys(out).length ? out as T : undefined
}
const byStart = <T extends { start: number }>(l: T[]) => [...l].sort((a, b) => a.start - b.start)

// manifestOf puts a draft back together as the manifest PUT sends.
export function manifestOf(d: Draft): Manifest {
  const m: Manifest = { ...(d.rest as object), format: 'wittyinfo', version: Math.max(1, Number((d.rest as { version?: number }).version) || 1) } as Manifest
  // Every part has an id once it has a translation to keep with it.
  const ided = Object.keys(d.translations).length > 0
  if (d.segments.length) m.segments = byStart(ided ? withIds(d.segments, 'seg') : d.segments).map((s) => tidy({ ...s, end: s.end ?? undefined }) as Seg)
  if (d.absent.length) m.absent = [...d.absent]
  if (d.scenes.length) m.scenes = byStart(ided ? withIds(d.scenes, 'scene') : d.scenes).map((s) => tidy({ ...s, thumbnailAt: s.thumbnailAt ?? undefined }) as Scene)
  if (d.filters.length) m.filters = byStart(ided ? withIds(d.filters, 'filter') : d.filters).map((f) => tidy({ ...f, severity: f.severity || undefined }) as Filter)
  const meta = tidy(d.metadata)
  if (meta) m.metadata = meta
  const pb = tidy(d.playback)
  if (pb) m.playback = pb
  if (d.notes.trim()) m.notes = d.notes
  const art: Record<string, string | string[]> = {}
  for (const [role, list] of Object.entries(d.artwork)) if (list.length) art[role] = list.length === 1 ? list[0] : [...list]
  if (Object.keys(art).length) m.artwork = art
  if (d.language) m.language = d.language
  if (d.subtitles.length) m.subtitles = d.subtitles.map((x) => ({ ...x }))
  const trs: Record<string, Translation> = {}
  for (const [lang, tr] of Object.entries(d.translations)) {
    const clean = cleanTranslation(tr)
    if (clean) trs[lang] = clean
  }
  if (Object.keys(trs).length) m.translations = trs
  return m
}

// cleanTranslation drops what is empty from a translation; undefined when nothing is left.
function cleanTranslation(tr: Translation): Translation | undefined {
  const out: Translation = {}
  for (const [k, v] of Object.entries(tr)) {
    if (k === 'segments' || k === 'scenes' || k === 'filters') {
      const parts: Record<string, TextPart> = {}
      for (const [id, p] of Object.entries((v || {}) as Record<string, TextPart>)) {
        const t = tidy({ ...p })
        if (t) parts[id] = t
      }
      if (Object.keys(parts).length) out[k] = parts
    } else if (k === 'metadata') {
      const t = tidy({ ...(v as Meta) })
      if (t) out.metadata = t
    } else if (k === 'notes') {
      if (typeof v === 'string' && v.trim()) out.notes = v
    } else if (v != null) out[k] = v
  }
  return Object.keys(out).length ? out : undefined
}

// setText changes one text of a translation: a part's (kind and id) or the details' or notes.
export function setText(d: Draft, lang: string, kind: 'segments' | 'scenes' | 'filters' | 'metadata' | 'notes', id: string, field: string, value: string): Draft {
  const tr: Translation = { ...(d.translations[lang] || {}) }
  if (kind === 'notes') tr.notes = value
  else if (kind === 'metadata') tr.metadata = { ...(tr.metadata || {}), [field]: value }
  else tr[kind] = { ...(tr[kind] || {}), [id]: { ...((tr[kind] || {})[id] || {}), [field]: value } }
  return { ...d, translations: { ...d.translations, [lang]: tr } }
}

// ensureIds gives every part an id, so a translation can name it.
export const ensureIds = (d: Draft): Draft => ({ ...d, segments: withIds(d.segments, 'seg'), scenes: withIds(d.scenes, 'scene'), filters: withIds(d.filters, 'filter') })

// ---------------------------------------------------------------- languages

// LANGUAGE_CODES: every ISO 639-1 language and the three-letter codes media often has alone,
// as pkg/langtag (and WittyMediaPlayer's QLocale) know them. Names come from the browser, in
// the app's language (languageLabel).
export const LANGUAGE_CODES = ('aa ab ae af ak am an ar as av ay az ba be bg bh bi bm bn bo br bs ca ce ch co cr cs cu cv cy da de dv dz ee el en eo es et eu fa ff fi fj fo fr fy ga gd gl gn gu gv ha he hi ho hr ht hu hy hz ia id ie ig ii ik io is it iu ja jv ka kg ki kj kk kl km kn ko kr ks ku kv kw ky la lb lg li ln lo lt lu lv mg mh mi mk ml mn mr ms mt my na nb nd ne ng nl nn no nr nv ny oc oj om or os pa pi pl ps pt qu rm rn ro ru rw sa sc sd se sg si sk sl sm sn so sq sr ss st su sv sw ta te tg th ti tk tl tn to tr ts tt tw ty ug uk ur uz ve vi vo wa wo xh yi yo za zh zu ' +
  'ast ceb cmn fil gsw haw hmn kab mni nan nds sco tlh yue wuu').split(' ')
// Tags people often want that are a language with a region or script.
export const COMMON_TAGS = ['en-GB', 'en-US', 'es-419', 'es-ES', 'fr-CA', 'pt-BR', 'pt-PT', 'zh-Hans', 'zh-Hant', 'sr-Latn', 'sr-Cyrl']

const LANG_RE = /^[A-Za-z]{2,8}([-_][A-Za-z0-9]{1,8})*$/
export const validLanguage = (tag: string) => tag.length <= 35 && LANG_RE.test(tag) && !(tag.length > 3 && !/[-_]/.test(tag))

// canonicalLanguage writes a tag in its usual form, as the server does: "pt_br" is "pt-BR".
export function canonicalLanguage(tag: string): string {
  if (!validLanguage(tag)) return tag
  const parts = tag.split(/[-_]/)
  return [parts[0].toLowerCase(), ...parts.slice(1).map((p) => (/^[A-Za-z]{4}$/.test(p) ? p[0].toUpperCase() + p.slice(1).toLowerCase() : /^([A-Za-z]{2}|\d{3})$/.test(p) ? p.toUpperCase() : p.toLowerCase()))].join('-')
}

// languageLabel names a tag in the language the app is in ("Deutsch (Österreich)" or
// "German (Austria)"), and keeps the code when the browser can't name it.
export function languageLabel(tag: string, inLanguage = 'en'): string {
  if (!tag) return ''
  try {
    const n = new Intl.DisplayNames([inLanguage, 'en'], { type: 'language', fallback: 'none' }).of(canonicalLanguage(tag))
    return n && n.toLowerCase() !== tag.toLowerCase() ? `${n} (${canonicalLanguage(tag)})` : canonicalLanguage(tag)
  } catch { return tag }
}

// changed: the draft differs from what was loaded.
export const changed = (a: Draft, b: Draft) => JSON.stringify(manifestOf(a)) !== JSON.stringify(manifestOf(b))

// ---------------------------------------------------------------- checks

const ctrl = /[\u0000-\u0008\u000b-\u001f\u007f]/
function text(where: string, s: string | undefined, max: number, out: Problem[]) {
  if (!s) return
  if ([...s].length > max) out.push({ where, what: `longer than ${max} characters` })
  else if (ctrl.test(s)) out.push({ where, what: 'holds control characters' })
}

// check says what the server would refuse, in the same words, so it shows before Save.
// durationSec: the video's length, if known: a part past the end is only a warning.
export function check(d: Draft, scope: 'item' | 'season' | 'show' = 'item'): Problem[] {
  const out: Problem[] = []
  const time = (where: string, v: number | null | undefined) => {
    if (v == null) return
    if (!Number.isFinite(v) || v < 0 || v > LIMITS.maxTime) out.push({ where, what: 'a time has to be between 0 and 7 days' })
  }
  if (d.segments.length > LIMITS.segments) out.push({ where: 'segments', what: `more than ${LIMITS.segments} segments` })
  byStart(d.segments).forEach((s, i) => {
    const w = `segments[${i}]`
    time(w + '.start', s.start)
    time(w + '.end', s.end)
    if (s.end != null && !(s.end > s.start)) out.push({ where: w + '.end', what: 'has to come after the start' })
    text(w + '.label', s.label, LIMITS.short, out)
  })
  if (scope !== 'item' && d.scenes.length) out.push({ where: 'scenes', what: `scenes belong to one video, not to a ${scope}` })
  if (scope !== 'item' && d.filters.length) out.push({ where: 'filters', what: `content filters belong to one video, not to a ${scope}` })
  byStart(d.scenes).forEach((s, i) => {
    time(`scenes[${i}].start`, s.start)
    time(`scenes[${i}].thumbnailAt`, s.thumbnailAt)
    text(`scenes[${i}].title`, s.title, LIMITS.short, out)
  })
  byStart(d.filters).forEach((f, i) => {
    time(`filters[${i}].start`, f.start)
    time(`filters[${i}].end`, f.end)
    if (!(f.end > f.start)) out.push({ where: `filters[${i}].end`, what: 'has to come after the start' })
  })
  for (const k of ['title', 'originalTitle', 'sortTitle', 'edition', 'tagline', 'contentRating'] as const) text('metadata.' + k, d.metadata[k] as string, LIMITS.short, out)
  text('metadata.overview', d.metadata.overview, LIMITS.long, out)
  if (d.metadata.year != null && (d.metadata.year < 1800 || d.metadata.year > 3000)) out.push({ where: 'metadata.year', what: 'has to be a year' })
  text('notes', d.notes, LIMITS.long, out)
  if (d.language && !validLanguage(d.language)) out.push({ where: 'language', what: 'is not a language code' })
  const seen = new Set<string>()
  for (const [lang, tr] of Object.entries(d.translations)) {
    if (!validLanguage(lang)) out.push({ where: `translations.${lang}`, what: 'is not a language code' })
    const c = canonicalLanguage(lang)
    if (seen.has(c)) out.push({ where: `translations.${lang}`, what: 'the same language twice' })
    seen.add(c)
    for (const kind of ['segments', 'scenes', 'filters'] as const) for (const [id, p] of Object.entries(tr[kind] || {})) {
      text(`translations.${lang}.${kind}.${id}.label`, p.label, LIMITS.short, out)
      text(`translations.${lang}.${kind}.${id}.title`, p.title, LIMITS.short, out)
    }
    text(`translations.${lang}.metadata.overview`, tr.metadata?.overview, LIMITS.long, out)
    text(`translations.${lang}.notes`, tr.notes, LIMITS.long, out)
  }
  return out
}

// problemWords: "Segment 3 (intro): has to come after the start", for a person.
export function problemWords(p: Problem, d?: Draft): string {
  const m = /^(segments|scenes|filters)\[(\d+)\]\.?(\w*)$/.exec(p.where)
  if (!m) return p.where ? `${p.where}: ${p.what}` : p.what
  const n = Number(m[2])
  const list: { start: number }[] = d ? byStart<{ start: number }>(m[1] === 'segments' ? d.segments : m[1] === 'scenes' ? d.scenes : d.filters) : []
  const item = list[n] as Seg | Scene | Filter | undefined
  const what = m[1] === 'segments' ? `Part ${n + 1}${item ? ` (${segLabel((item as Seg).type)})` : ''}` : m[1] === 'scenes' ? `Scene ${n + 1}` : `Filter ${n + 1}`
  const field = { start: 'start', end: 'end', label: 'name', title: 'title', thumbnailAt: 'picture time' }[m[3]] || m[3]
  return `${what}${field ? `, ${field}` : ''}: ${p.what}`
}

// ---------------------------------------------------------------- time

export { clockText, clockSeconds }

// readTime reads what was typed in a time box: null for nothing, NaN for nonsense.
export const readTime = (s: string) => clockSeconds(s)

// FRAME is a frame's length when the video's rate isn't known (24 frames a second).
export const FRAME = 1 / 24
const round3 = (v: number) => Math.round(v * 1000) / 1000
export const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))

// Edge is which part of a range a drag or a key moves.
export type Edge = 'start' | 'end' | 'both'

// moveRange moves a range's start, end or both by delta seconds, keeping it inside the video
// and at least a tenth of a second long. An open end ("to the end") stays open unless moved.
export function moveRange<T extends { start: number; end?: number | null }>(r: T, edge: Edge, delta: number, duration: number): T {
  const max = duration > 0 ? duration : LIMITS.maxTime
  const end = r.end ?? (edge === 'end' ? max : null)
  if (edge === 'start') return { ...r, start: round3(clamp(r.start + delta, 0, (end ?? max) - 0.1)) }
  if (edge === 'end') return { ...r, end: round3(clamp((end as number) + delta, r.start + 0.1, max)) }
  const len = end == null ? 0 : end - r.start
  const start = round3(clamp(r.start + delta, 0, max - Math.max(len, 0.1)))
  return { ...r, start, ...(end == null ? {} : { end: round3(start + len) }) }
}

// markAt sets a range's start or end to a moment, swapping them if they would cross.
export function markAt<T extends { start: number; end?: number | null }>(r: T, edge: 'start' | 'end', at: number): T {
  at = round3(Math.max(0, at))
  if (edge === 'start') return r.end != null && at >= r.end ? { ...r, start: r.end, end: Math.max(at, r.end + 0.1) } : { ...r, start: at }
  return at <= r.start ? { ...r, start: at, end: r.start } : { ...r, end: at }
}

// newSegment starts a part at a moment, a sensible length for its kind, inside the video.
export function newSegment(type: SegType, at: number, duration: number): Seg {
  const len = { intro: 60, recap: 45, credits: 90, preview: 30, postcredits: 60, commercial: 120, custom: 30 }[type]
  const max = duration > 0 ? duration : at + len
  const start = round3(clamp(at, 0, Math.max(0, max - 1)))
  const seg: Seg = { type, start, end: round3(Math.min(max, start + len)) }
  if (type === 'credits' && duration > 0 && max - start <= len) seg.end = null
  return seg
}

// percent places a moment along the timeline.
export const percent = (t: number, duration: number) => (duration > 0 ? clamp((t / duration) * 100, 0, 100) : 0)

// timeAt turns a place along the timeline (0 to 1) into a moment, rounded to a tenth.
export const timeAt = (frac: number, duration: number) => Math.round(clamp(frac, 0, 1) * duration * 10) / 10

// ---------------------------------------------------------------- what wins

// SOURCE_RANK: the order of trust (docs/formats/wittyinfo.md, What wins), most trusted first.
export const SOURCE_RANK = ['wittyinfo', 'manual', 'skip.json', 'edl', 'wittyinfo-season', 'wittyinfo-show', 'chapters', 'detected', 'show']

// overrides lists, for each kind this file gives, what it takes the place of now: the
// editor shows "replaces: the file's chapters".
export function inForce(m: InfoAnswer['markers'] | null | undefined): { kind: string; source: string; startSec: number; endSec: number }[] {
  return (m?.segments || []).map((s) => ({ kind: s.kind, source: s.source, startSec: s.startSec, endSec: s.endSec }))
}

// ---------------------------------------------------------------- content filters

// What a profile does about each kind of content (Settings, Playback). Kept with the
// account's other choices as "contentFilters".
export type FilterChoice = 'off' | 'warn' | 'mute' | 'skip'
export type FilterPrefs = Partial<Record<FilterCategory, FilterChoice>> & { minSeverity?: number }
export const FILTER_PREFS_KEY = 'contentFilters'

export function filterPrefs(raw: unknown): FilterPrefs {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const out: FilterPrefs = {}
  for (const c of FILTER_CATEGORIES) {
    const v = r[c.value]
    if (v === 'warn' || v === 'mute' || v === 'skip') out[c.value] = v
  }
  const s = Number(r.minSeverity)
  if (s >= 1 && s <= 3) out.minSeverity = s
  return out
}

// filterAt is what to do now: the strongest choice among the filters playing (skip, then
// mute, then warn), and the filter it is for. A filter milder than the profile cares about
// is left alone; one whose strength isn't said counts.
export function filterAt(filters: Filter[] | null | undefined, prefs: FilterPrefs, now: number): { act: Exclude<FilterChoice, 'off'>; filter: Filter } | null {
  let best: { act: Exclude<FilterChoice, 'off'>; filter: Filter } | null = null
  const rank = { warn: 1, mute: 2, skip: 3 }
  for (const f of filters || []) {
    if (!(now >= f.start && now < f.end)) continue
    const choice = prefs[f.category]
    if (!choice || choice === 'off') continue
    if (prefs.minSeverity && f.severity && f.severity < prefs.minSeverity) continue
    if (!best || rank[choice] > rank[best.act]) best = { act: choice, filter: f }
  }
  return best
}

// ---------------------------------------------------------------- preview

// draftMarkers is a draft as the player reads markers, so it can try them before saving:
// the parts as they would be, with an open end running to the end of the video.
export function draftMarkers(d: Draft, duration: number): Markers {
  const segments = byStart(d.segments).map((s) => ({
    kind: s.type, startSec: s.start, endSec: s.end ?? (duration > 0 ? duration : s.start + 1), label: s.label, action: actOf(s), source: 'wittyinfo',
  })).filter((s) => s.endSec > s.startSec)
  const pc = segments.find((s) => s.kind === 'postcredits')
  return { segments, postCredits: pc ? { startSec: pc.startSec, endSec: pc.endSec } : null }
}
