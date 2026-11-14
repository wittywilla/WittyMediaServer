#!/usr/bin/env node
// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Writes what WittyModule's web kit takes from this server's web app (module API 5):
// kit/web/tokens.json, the design tokens (the CSS custom properties on :root in
// web/src/styles.css) a module's stylesheet uses instead of copying colours, so a module
// looks exactly like the app and follows every theme. Run it after changing those tokens.
//
//   node scripts/export-module-kit.mjs [WittyModule folder]     (default: ../WittyModule)
//   node scripts/export-module-kit.mjs --check [folder]          fail if the kit's copy differs
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const args = process.argv.slice(2)
const check = args[0] === '--check'
if (check) args.shift()
const kit = resolve(args[0] || [join(here, '../../WittyModule'), join(here, '../../../WittyModule')].find((d) => existsSync(join(d, 'kit'))) || '')

export function tokens(css) {
  const root = /:root\s*\{([\s\S]*?)\n\}/.exec(css)
  if (!root) throw new Error('styles.css has no :root block')
  const out = {}
  for (const m of root[1].matchAll(/(--[a-z0-9-]+)\s*:\s*([^;]+);/g)) out[m[1]] = m[2].trim().replace(/\s+/g, ' ')
  return out
}

const css = readFileSync(join(here, '../web/src/styles.css'), 'utf8')
const doc = JSON.stringify({
  $comment: 'Written by WittyMediaServer scripts/export-module-kit.mjs from web/src/styles.css. Use these as var(--name) in a module stylesheet; never copy their values. Themes change them at run time.',
  webHostAPI: 5,
  tokens: tokens(css),
}, null, 2) + '\n'
const file = join(kit, 'kit', 'web', 'tokens.json')
if (check) {
  if (!existsSync(file) || readFileSync(file, 'utf8') !== doc) { console.error(`${file} differs from this server's tokens: run node scripts/export-module-kit.mjs`); process.exit(1) }
  console.log('the kit has this server\'s tokens')
} else {
  writeFileSync(file, doc)
  console.log(`wrote ${Object.keys(JSON.parse(doc).tokens).length} tokens to ${file}`)
}
