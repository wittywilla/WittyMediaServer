// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// The parts of translation that need no browser: looking a message up, filling in its {names}, and picking the plural
// form. index.ts adds loading the catalogues and following the server's language; tests use this file on its own.
//
// Messages are keyed by their English text, so English needs no catalogue and anything not translated yet still shows,
// in English. A catalogue maps the English to the translation; a message with a count may give each plural form of
// the language (Intl.PluralRules' names: zero, one, two, few, many, other).

export type PluralForms = Partial<Record<Intl.LDMLPluralRule, string>>
export type Message = string | PluralForms
export type Catalogue = Record<string, Message>
export type Vars = Record<string, string | number | null | undefined>

/**
 * Fills each {name} with vars[name]; numbers are written the language's way. A {name} with no value stays as it is.
 * In a language written right to left, what's filled in is isolated (U+2068 … U+2069), so a Latin title, a file
 * name or "2 hr 10 min" keeps its own order inside the sentence instead of having its digits and dashes moved.
 */
export function fill(text: string, vars: Vars | undefined, locale: string): string {
  if (!vars) return text
  const rtl = isRtl(locale)
  return text.replace(/\{(\w+)\}/g, (whole, name: string) => {
    const v = vars[name]
    if (v === undefined || v === null) return whole
    const s = typeof v === 'number' ? formatNumber(v, locale) : v
    return rtl && s && needsIsolating(s) ? FSI + s + PDI : s
  })
}

/** First strong isolate and pop directional isolate: the plain-text <bdi>. */
export const FSI = '\u2068'
export const PDI = '\u2069'
// Left-to-right letters, or digits with something around them that the bidi rules would move.
const ltrOrMixed = /[A-Za-z\u00C0-\u024F\u0370-\u03FF\u0400-\u04FF]|\d[^\d]|[^\d]\d/
const needsIsolating = (s: string) => !s.startsWith(FSI) && ltrOrMixed.test(s)

/** Text wrapped so it keeps its own direction inside a sentence in any language (for text not filled in by t()). */
export const isolate = (s: string) => (s && !s.startsWith(FSI) ? FSI + s + PDI : s)

const pluralCache = new Map<string, Intl.PluralRules>()
/** The plural form a count takes in a language: 'one', 'few', 'other' and so on. */
export function pluralForm(count: number, locale: string): Intl.LDMLPluralRule {
  let rules = pluralCache.get(locale)
  if (!rules) {
    try { rules = new Intl.PluralRules(locale) } catch { rules = new Intl.PluralRules('en') }
    pluralCache.set(locale, rules)
  }
  return rules.select(count)
}

/** Picks the form for count from a catalogue entry, falling back to 'other', then to the English. */
export function pickForm(msg: Message | undefined, count: number, locale: string, englishOne: string, englishOther: string): string {
  if (typeof msg === 'string') return msg
  if (msg) {
    const form = msg[pluralForm(count, locale)] ?? msg.other
    if (form) return form
  }
  return pluralForm(count, 'en') === 'one' ? englishOne : englishOther
}

/** Looks up one English message in a catalogue; the English itself when there's no translation. */
export function lookup(cat: Catalogue, english: string): string {
  const m = cat[english]
  if (typeof m === 'string' && m) return m
  if (m && typeof m === 'object' && m.other) return m.other
  return english
}

const numberCache = new Map<string, Intl.NumberFormat>()
export function formatNumber(n: number, locale: string, opts?: Intl.NumberFormatOptions): string {
  const key = opts ? `${locale}|${JSON.stringify(opts)}` : locale
  let f = numberCache.get(key)
  if (!f) {
    try { f = new Intl.NumberFormat(locale, opts) } catch { f = new Intl.NumberFormat('en', opts) }
    numberCache.set(key, f)
  }
  return f.format(n)
}

/**
 * Translates text the server has already filled in ("Welcome, Ana"), from catalogue keys that still have their
 * {names} ("Welcome, {user}"): each such key becomes a pattern, and what each {name} stood for is put into the
 * translation. Keys with almost no words of their own ("{kind} {number}") are left out, so they can't match
 * just anything. The longest key that matches wins. Returns undefined when nothing matches.
 */
export function templateMatcher(cat: Catalogue, locale: string, plurals: Record<string, string> = {}, all: Catalogue = cat): (text: string) => string | undefined {
  // What a {name} stood for may be a message of its own ("… {why}" with the reason in English): it's translated too,
  // one level down, so a sentence the server built from two of its messages comes out whole.
  const inner = (v: string, depth: number): string => {
    if (v in cat) return lookup(cat, v)
    return depth > 0 ? match(v, depth - 1) ?? v : v
  }
  const isName = (p: string) => /^\{\w+\}$/.test(p)
  type Entry = { re: RegExp; names: string[]; key: string; msg: Message | undefined; count?: { one: string; other: string } }
  const list: Entry[] = []
  const add = (pattern: string, key: string, msg: Message | undefined, count?: Entry['count']) => {
    const parts = pattern.split(/(\{\w+\})/).filter((p) => p !== '')
    const names = parts.filter(isName).map((p) => p.slice(1, -1))
    const words = parts.filter((p) => !isName(p)).join('').replace(/[^\p{L}\p{N}]/gu, '')
    if (!names.length || words.length < 4) return
    const source = parts.map((p) => (isName(p) ? (count && p === '{count}' ? '(\\d[\\d,.]*)' : '(.+?)') : p.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))).join('')
    list.push({ re: new RegExp(`^${source}$`, 's'), names, key, msg, count })
  }
  for (const key of Object.keys(cat)) if (key.includes('{') && !Object.values(plurals).includes(key)) add(key, key, cat[key])
  // A count the server put in its words ("1 album", "3 albums", from i18n.N): either English form, and the language's
  // form for that count, from wherever in the app's catalogues the message is.
  for (const [one, other] of Object.entries(plurals)) {
    if (!(other in all)) continue
    add(one, other, all[other], { one, other })
    add(other, other, all[other], { one, other })
  }
  list.sort((a, b) => b.key.length - a.key.length)
  function match(text: string, depth: number): string | undefined {
    for (const { re, names, key, msg, count } of list) {
      const m = re.exec(text)
      if (!m) continue
      const vars: Vars = {}
      names.forEach((n, i) => { vars[n] = inner(m[i + 1], depth) })
      if (count) {
        const n = Number(String(vars.count).replace(/[,.]/g, ''))
        vars.count = n
        return fill(pickForm(msg, n, locale, count.one, count.other), vars, locale)
      }
      return fill(lookup({ [key]: msg as Message }, key), vars, locale)
    }
    return undefined
  }
  return (text) => match(text, 1)
}

/** The languages written right to left. */
export const RTL = new Set(['ar', 'he', 'fa', 'ur'])
export const isRtl = (code: string) => RTL.has(code)

/** The {names} a message uses, sorted: a translation has to use the same ones. */
export function placeholders(text: string): string[] {
  return [...new Set([...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]))].sort()
}
