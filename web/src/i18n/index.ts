// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useSyncExternalStore } from 'react'
import { UI_LANGUAGES, uiLanguageFor } from '../languages'
import { fill, formatNumber, isolate, isRtl, lookup, pickForm, templateMatcher, type Catalogue, type Message, type Vars } from './core'
// The server's counted words (i18n.N in Go): its English for one, to the catalogue key, its English for more.
import plurals from './server-plurals.json'

// The app's words in the language the server is set to (the uiLanguage setting, chosen on setup's first screen).
//
// How to use it, anywhere in the app:
//   t ('Sign in')                                 a message
//   t ('Hello, {name}', { name })                 with something filled in
//   tn ('{count} film', '{count} films', n)       with a count: the right plural form for the language
//   tk ('Movies')                                 marks text kept in a list made when the file loads: it's
//                                                 translated where it's shown, with t(label) or tx(label)
// (written here with a space so the check script doesn't count these examples)
// The English is the key. Catalogues are in locales/<language>/<part>.json, one file per part of the app, and only the
// chosen language's are loaded. Changing the language redraws the whole app (main.tsx keys it on the language), so a
// component needs nothing else to follow it; useLanguage() is there for the few that format things themselves.

export { formatNumber, isRtl } from './core'
export type { Vars } from './core'

const files = import.meta.glob<{ default: Catalogue }>('./locales/*/*.json')

let current = 'en'
let catalogue: Catalogue = {}
const listeners = new Set<() => void>()

const known = (code: string) => UI_LANGUAGES.includes(code)

/** The language the app is in now: a code from UI_LANGUAGES in languages.ts (two letters, or pt-BR / pt-PT). */
export const language = () => current

/** A message in the app's language. */
export function t(english: string, vars?: Vars): string {
  return fill(current === 'en' ? english : lookup(catalogue, english), vars, current)
}

/** A message with a count in it: the English singular and plural, and the count ({count} in the text). */
export function tn(one: string, other: string, count: number, vars?: Vars): string {
  const all = { count, ...vars }
  if (current === 'en') return fill(count === 1 ? one : other, all, 'en')
  return fill(pickForm(catalogue[other] as Message | undefined, count, current, one, other), all, current)
}

/**
 * A message whose English means more than one thing ("Network": a broadcaster, or the home network). English shows
 * the text as it is; a catalogue can translate it for this context under "context|English", and otherwise the plain
 * English's translation is used.
 */
export function tc(context: string, english: string, vars?: Vars): string {
  if (current === 'en') return fill(english, vars, 'en')
  const key = `${context}|${english}`
  return fill(key in catalogue ? lookup(catalogue, key) : lookup(catalogue, english), vars, current)
}

/** tx() with a context, for text kept elsewhere (a menu group's saved name): see tc(). */
export function txc(context: string, text: string | undefined | null): string {
  if (!text) return ''
  if (current === 'en') return text
  const key = `${context}|${text}`
  return key in catalogue ? lookup(catalogue, key) : tx(text)
}

/**
 * A part of a line put together from several ("Spice 2 · 2019 Hits · 3:20"), as text: in a right-to-left language it is
 * isolated (U+2068 … U+2069), so a Latin title or "2 hr 10 min" keeps its own order and its place in the line. Use it as
 * parts.map(bidi).join(' · '). In other languages the text is as it was. See bidi.css for whole elements.
 */
export const bidi = (part: unknown): string => {
  const s = part === null || part === undefined || part === false ? '' : String(part)
  return isRtl(current) ? isolate(s) : s
}

/** Marks English text to be translated later, where it's shown, with t(). */
export const tk = <S extends string>(english: S): S => english

/** Translates text that may have come from somewhere marked with tk(), or from the server: unknown text stays as it is. */
export function tx(text: string | undefined | null): string {
  if (!text) return ''
  if (current === 'en') return text
  const exact = lookup(catalogue, text)
  if (exact !== text) return exact
  return fromServer(text)
}

// Text the server filled in before sending ("Welcome, Ana"): matched against the server catalogue's keys that still
// have their {names}. Remembered, as the same labels are drawn again and again.
let matcher: ((text: string) => string | undefined) | null = null
let serverCat: Catalogue = {}
const seen = new Map<string, string>()
function fromServer(text: string): string {
  const known = seen.get(text)
  if (known !== undefined) return known
  matcher ??= templateMatcher(serverCat, current, plurals, catalogue)
  const out = matcher(text) ?? text
  if (seen.size > 5000) seen.clear()
  seen.set(text, out)
  return out
}

/** Loads a language's catalogues and switches the app to it. An unknown or empty code is English. */
// loadedFor: the language whose catalogues were last loaded (or tried: a part that failed to load stays English rather
// than being asked for again, which would redraw the app over and over). asked: the latest language asked for, so a
// slower earlier load can't win over it.
let loadedFor = 'en'
let asked = ''

export async function setLanguage(code: string): Promise<void> {
  const wanted = uiLanguageFor(code)
  const next = known(wanted) ? wanted : 'en'
  asked = next
  if (next === current && next === loadedFor) { applyDocument(next); return }
  let merged: Catalogue = {}
  let server: Catalogue = {}
  if (next !== 'en') {
    const prefix = `./locales/${next}/`
    const mine = Object.entries(files).filter(([p]) => p.startsWith(prefix))
    const parts = await Promise.all(mine.map(([, load]) => load().then((m) => m.default).catch(() => ({} as Catalogue))))
    merged = Object.assign({}, ...parts)
    server = parts[mine.findIndex(([p]) => p.endsWith('/server.json'))] ?? {}
  }
  if (asked !== next) return
  catalogue = merged
  serverCat = server
  matcher = null
  seen.clear()
  current = next
  loadedFor = next
  try { localStorage.setItem(KEY, next) } catch { /* storage off: the server's status says it again next time */ }
  applyDocument(next)
  listeners.forEach((f) => f())
}

function applyDocument(code: string) {
  const html = document.documentElement
  html.lang = code
  html.dir = isRtl(code) ? 'rtl' : 'ltr'
}

const KEY = 'wms.uiLanguage'

/** The language to start in before the server has said: the last one this browser used, else the browser's own. */
export function startingLanguage(): string {
  try {
    const kept = uiLanguageFor(localStorage.getItem(KEY))
    if (kept && known(kept)) return kept
  } catch { /* storage off */ }
  try {
    const code = uiLanguageFor(navigator.language)
    if (known(code)) return code
  } catch { /* no usable browser language */ }
  return 'en'
}

/** The app's language, redrawing the component when it changes. */
export function useLanguage(): string {
  return useSyncExternalStore((f) => { listeners.add(f); return () => { listeners.delete(f) } }, () => current)
}

/** Listens for the language changing (module host API). */
export function onLanguage(f: (code: string) => void): () => void {
  const g = () => f(current)
  listeners.add(g)
  return () => { listeners.delete(g) }
}

/** A number, written the language's way. */
export const num = (n: number, opts?: Intl.NumberFormatOptions) => formatNumber(n, current, opts)

/** A date and/or time, written the language's way. */
export function date(d: Date | number | string, opts: Intl.DateTimeFormatOptions = { dateStyle: 'medium' }): string {
  const v = d instanceof Date ? d : new Date(d)
  if (Number.isNaN(v.getTime())) return ''
  try { return new Intl.DateTimeFormat(current, opts).format(v) } catch { return v.toLocaleString() }
}

/** "3 days ago", "in 2 hours" in the language. */
export function relative(value: number, unit: Intl.RelativeTimeFormatUnit): string {
  try { return new Intl.RelativeTimeFormat(current, { numeric: 'auto' }).format(value, unit) } catch { return `${value} ${unit}` }
}

/** "a, b and c" in the language. */
export function list(items: string[], type: 'conjunction' | 'disjunction' = 'conjunction'): string {
  try { return new Intl.ListFormat(current, { type }).format(items) } catch { return items.join(', ') }
}

/** A language's own name for itself ("Deutsch"), for language pickers. */
export function nativeName(code: string): string {
  try { return new Intl.DisplayNames([code], { type: 'language' }).of(code) || code } catch { return code }
}

/** A language's name in the app's language ("German" in English, "Allemand" in French). */
export function languageNameHere(code: string): string {
  try { return new Intl.DisplayNames([current], { type: 'language' }).of(code) || code } catch { return code }
}
