// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Lists the app's English messages that a language has no translation for, and translations whose {names} don't match
// the English. Run from web/:  node scripts/i18n-check.mjs [language] [--keys]
//   --keys   prints every message found in the source, one per line (for writing a catalogue)
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { messagesIn } from '../src/i18n/extract.ts'

const src = new URL('../src/', import.meta.url).pathname.replace(/^\/(\w:)/, '$1')
const walk = (d) => readdirSync(d).flatMap((f) => { const p = join(d, f); return statSync(p).isDirectory() ? walk(p) : /\.(tsx?|mts)$/.test(f) ? [p] : [] })
const keys = new Set()
for (const f of walk(src)) for (const k of messagesIn(readFileSync(f, 'utf8'))) keys.add(k)
const args = process.argv.slice(2)
if (args.includes('--keys')) { for (const k of [...keys].sort()) console.log(k); process.exit(0) }
const dir = join(src, 'i18n', 'locales')
const only = args.find((a) => !a.startsWith('-'))
let bad = 0
for (const lang of readdirSync(dir).filter((l) => !only || l === only)) {
  const cat = Object.assign({}, ...readdirSync(join(dir, lang)).map((f) => JSON.parse(readFileSync(join(dir, lang, f), 'utf8'))))
  const missing = [...keys].filter((k) => !(k in cat))
  console.log(`${lang}: ${keys.size - missing.length} of ${keys.size} translated`)
  if (only) for (const k of missing) console.log('  missing:', k)
}
console.log(`${keys.size} messages in the source`)
process.exit(bad)
