// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The web app's download budget (#643): for each route in bundle-budget.json, the scripts and
// stylesheets a first visit to it fetches before anything is drawn, compressed as the server sends
// them. That is the app's entry with everything it imports, plus the route's own page code with
// everything that imports. Code fetched later (a dialog when it opens, the player when a title plays,
// pdf.js for a book) isn't counted against the route.
//
//   node scripts/bundle-budget.mjs            build into a temporary folder, report, fail when over
//   node scripts/bundle-budget.mjs --json     the same, and print the figures as JSON
//   WMS_BUDGET_DIST=dist node scripts/...     use a build made with --manifest instead of building
//
// When a route goes over: look at what it now imports (the report lists the largest files), and
// fetch what isn't needed at first with lazy() or import(). Raise a budget only when the extra is
// worth it on a slow TV or phone, and say why in the pull request.
import { execFileSync } from 'node:child_process'
import { existsSync, mkdtempSync, readFileSync, rmSync, statSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const web = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const budget = JSON.parse(readFileSync(path.join(web, 'bundle-budget.json'), 'utf8'))
const asJSON = process.argv.includes('--json')

let dist = process.env.WMS_BUDGET_DIST ? path.resolve(web, process.env.WMS_BUDGET_DIST) : ''
let temp = ''
if (!dist) {
  temp = mkdtempSync(path.join(tmpdir(), 'wms-budget-'))
  dist = temp
  const vite = path.join(web, 'node_modules', 'vite', 'bin', 'vite.js')
  execFileSync(process.execPath, [vite, 'build', '--manifest', '--outDir', dist, '--emptyOutDir', '--logLevel', 'error'], { cwd: web, stdio: 'inherit' })
}

try {
  const manifestFile = path.join(dist, '.vite', 'manifest.json')
  if (!existsSync(manifestFile)) throw new Error(`no ${manifestFile}: build with --manifest, or leave WMS_BUDGET_DIST out`)
  const m = JSON.parse(readFileSync(manifestFile, 'utf8'))
  // As sent: the build keeps the .gz of anything over 1 KB and the file itself otherwise.
  const size = (f) => {
    for (const p of [path.join(dist, f + '.gz'), path.join(dist, f)]) if (existsSync(p)) return statSync(p).size
    return 0
  }
  // A route's code is a source file, or (for code the bundler put in a shared chunk because it is
  // both imported and fetched lazily) "chunk:Name", the chunk named after its main file.
  const resolve = (k) => {
    if (!k.startsWith('chunk:')) return k
    const hit = Object.keys(m).find((x) => x.startsWith('_' + k.slice(6) + '-') && x.endsWith('.js'))
    if (!hit) throw new Error(`no shared chunk named ${k.slice(6)} in the build: a route in bundle-budget.json names a file that moved?`)
    return hit
  }
  const closure = (keys) => {
    const seen = new Set()
    const stack = keys.map(resolve)
    while (stack.length) {
      const k = stack.pop()
      if (seen.has(k)) continue
      if (!m[k]) throw new Error(`${k} isn't in the build: a route in bundle-budget.json names a file that moved?`)
      seen.add(k)
      for (const i of m[k].imports || []) stack.push(i)
    }
    return seen
  }
  const weigh = (keys) => {
    let js = 0, css = 0
    const files = []
    const cssSeen = new Set()
    for (const k of keys) {
      const n = size(m[k].file)
      js += n
      files.push({ file: m[k].file, bytes: n })
      for (const c of m[k].css || []) {
        if (cssSeen.has(c)) continue
        cssSeen.add(c)
        css += size(c)
      }
    }
    files.sort((a, b) => b.bytes - a.bytes)
    return { js, css, chunks: keys.size, files }
  }

  const entry = Object.keys(m).find((k) => m[k].isEntry)
  const shell = closure([entry])
  const kb = (n) => (n / 1024).toFixed(1) + ' KB'
  const over = []
  const out = { routes: {}, largestChunk: null }
  for (const [route, r] of Object.entries(budget.routes)) {
    const w = weigh(closure([entry, ...(r.code || [])]))
    out.routes[route] = { js: w.js, css: w.css, chunks: w.chunks }
    if (w.js > r.js) over.push(`${route}: ${kb(w.js)} of scripts on a first visit, over its budget of ${kb(r.js)}. Largest: ${w.files.slice(0, 5).map((f) => `${f.file} ${kb(f.bytes)}`).join(', ')}`)
    if (w.css > r.css) over.push(`${route}: ${kb(w.css)} of stylesheets on a first visit, over its budget of ${kb(r.css)}`)
  }
  // Any one file, wherever it's fetched: a big new dependency shows here even when it is lazy.
  let largest = { file: '', bytes: 0 }
  for (const v of Object.values(m)) {
    if (!/\.js$/.test(v.file) || (budget.chunkExempt || []).some((x) => v.file.includes(x))) continue
    const n = size(v.file)
    if (n > largest.bytes) largest = { file: v.file, bytes: n }
  }
  out.largestChunk = largest
  if (largest.bytes > budget.chunkMax) over.push(`${largest.file} is ${kb(largest.bytes)}, over the ${kb(budget.chunkMax)} any one script may be (pdf.js and hls.js are exempt in chunkExempt)`)

  if (asJSON) console.log(JSON.stringify(out, null, 2))
  else {
    console.log(`Shell: ${shell.size} files. Per route, first visit (compressed, as sent):`)
    console.log('| Route | Scripts | Budget | Stylesheets | Budget | Files |')
    console.log('|---|---|---|---|---|---|')
    for (const [route, w] of Object.entries(out.routes)) console.log(`| ${route} | ${kb(w.js)} | ${kb(budget.routes[route].js)} | ${kb(w.css)} | ${kb(budget.routes[route].css)} | ${w.chunks} |`)
    console.log(`Largest script fetched anywhere (pdf.js and hls.js aside): ${largest.file} ${kb(largest.bytes)} (budget ${kb(budget.chunkMax)})`)
  }
  if (over.length) {
    console.error('\nOVER THE WEB BUDGET (web/bundle-budget.json):')
    for (const o of over) console.error('  - ' + o)
    console.error('\nFetch what a page doesn’t need at first with lazy() or import(); raise a budget only when it is worth it on a slow TV or phone, and say why in the pull request.')
    process.exitCode = 1
  }
} finally {
  if (temp) rmSync(temp, { recursive: true, force: true })
}
