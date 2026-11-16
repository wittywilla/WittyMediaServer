// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Finds the English messages in a source file: the first text given to t(), tk() and tx() and both texts given to tn(),
// when they're written out in quotes. The check script and the tests use it.
const str = String.raw`'((?:[^'\\]|\\.)*)'|"((?:[^"\\]|\\.)*)"|\x60((?:[^\x60\\$]|\\.)*)\x60`
const one = new RegExp(String.raw`(?<![\w.$])t[kx]?\(\s*(?:${str})`, 'g')
const two = new RegExp(String.raw`(?<![\w.$])tn\(\s*(?:${str})\s*,\s*(?:${str})`, 'g')

const unescape = (s: string) => s.replace(/\\(u\{[0-9a-fA-F]+\}|u[0-9a-fA-F]{4}|x[0-9a-fA-F]{2}|.)/g, (_, e: string) =>
  e[0] === 'u' ? String.fromCodePoint(parseInt(e.replace(/[u{}]/g, ''), 16)) : e[0] === 'x' ? String.fromCharCode(parseInt(e.slice(1), 16)) : e === 'n' ? '\n' : e === 't' ? '\t' : e)

export function messagesIn(source: string): string[] {
  const out: string[] = []
  for (const m of source.matchAll(one)) out.push(unescape(m[1] ?? m[2] ?? m[3] ?? ''))
  for (const m of source.matchAll(two)) out.push(unescape(m[1] ?? m[2] ?? m[3] ?? ''), unescape(m[4] ?? m[5] ?? m[6] ?? ''))
  return out.filter(Boolean)
}

/** The catalogue keys in a source file: like messagesIn, but a count's message is keyed by its plural English only. */
export function keysIn(source: string): string[] {
  const out: string[] = []
  for (const m of source.matchAll(one)) out.push(unescape(m[1] ?? m[2] ?? m[3] ?? ''))
  for (const m of source.matchAll(two)) out.push(unescape(m[4] ?? m[5] ?? m[6] ?? ''))
  return out.filter(Boolean)
}
