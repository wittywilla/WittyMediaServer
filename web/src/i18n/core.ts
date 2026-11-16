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

/** Fills each {name} with vars[name]; numbers are written the language's way. A {name} with no value stays as it is. */
export function fill(text: string, vars: Vars | undefined, locale: string): string {
  if (!vars) return text
  return text.replace(/\{(\w+)\}/g, (whole, name: string) => {
    const v = vars[name]
    if (v === undefined || v === null) return whole
    return typeof v === 'number' ? formatNumber(v, locale) : v
  })
}

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

/** The languages written right to left. */
export const RTL = new Set(['ar', 'he', 'fa', 'ur'])
export const isRtl = (code: string) => RTL.has(code)

/** The {names} a message uses, sorted: a translation has to use the same ones. */
export function placeholders(text: string): string[] {
  return [...new Set([...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]))].sort()
}
