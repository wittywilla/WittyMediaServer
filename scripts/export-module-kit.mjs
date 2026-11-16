#!/usr/bin/env node
// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Writes what WittyModule's web kit takes from this server's web app (module API 5):
// kit/web/app-exports.json, what each of the app's parts a bundle may use (web/src/moduleApp.ts)
// exports as values, for the builder's stand-ins of @wms/app/<path>; and
// kit/web/tokens.json, the design tokens (the CSS custom properties on :root in
// web/src/styles.css) a module's stylesheet uses instead of copying colours, so a module
// looks exactly like the app and follows every theme. Run it after changing those tokens.
//
//   node scripts/export-module-kit.mjs [WittyModule folder]     (default: ../WittyModule)
//   node scripts/export-module-kit.mjs --check [folder]          fail if the kit's copy differs
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
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
// The value exports of each shared part, as the bundler sees them: everything a part imports is
// left out, so only its own exports and re-exports count, and types leave nothing behind.
async function appExports() {
  const { build } = await import(createRequire(join(here, '../web/package.json')).resolve('vite'))
  const src = join(here, '../web/src')
  const shared = [...readFileSync(join(src, 'moduleApp.ts'), 'utf8').matchAll(/^ {2}'([^']+)': m\d+,$/gm)].map((m) => m[1])
  const input = Object.fromEntries(shared.map((p) => [p.replace(/\//g, '__'), ['.tsx', '.ts'].map((e) => join(src, p + e)).find(existsSync)]))
  const res = await build({
    configFile: false, logLevel: 'silent', publicDir: false,
    build: { write: false, minify: false, cssCodeSplit: false, rollupOptions: { input, preserveEntrySignatures: 'strict', external: (id, importer) => !!importer } },
  })
  const out = {}
  for (const o of [res].flat().flatMap((r) => r.output)) {
    if (o.type === 'chunk' && o.isEntry) out[o.name.replace(/__/g, '/')] = [...o.exports].sort()
  }
  return Object.fromEntries(shared.map((p) => [p, out[p] || []]))
}

const appDoc = JSON.stringify({ $comment: 'Written by WittyMediaServer scripts/export-module-kit.mjs from web/src/moduleApp.ts: what a bundle may import as @wms/app/<path>, and the values each exports.', webHostAPI: 5, app: await appExports() }, null, 2) + '\n'
const files = { [join(kit, 'kit', 'web', 'tokens.json')]: doc, [join(kit, 'kit', 'web', 'app-exports.json')]: appDoc }
for (const [file, want] of Object.entries(files)) {
  if (check) {
    if (!existsSync(file) || readFileSync(file, 'utf8') !== want) { console.error(`${file} differs from this server's: run node scripts/export-module-kit.mjs`); process.exit(1) }
  } else {
    writeFileSync(file, want)
    console.log(`wrote ${file}`)
  }
}
if (check) console.log('the kit has this server\'s tokens and app exports')
