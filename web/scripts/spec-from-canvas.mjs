// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Reads the `gp` table of docs/design-2/canvas/Exp.dc.html (the 124 "Every feature, drawn" pages, in the order of
// the boards GB2_1 to GB2_6) and writes src/spec/gaps.json in the shape of src/spec/types.ts.
//   node scripts/spec-from-canvas.mjs          writes it
//   node scripts/spec-from-canvas.mjs --check  fails if the file is not what the canvas gives (CI)
import { readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('../../', import.meta.url))
const dir = root + 'docs/design-2/canvas/'
const out = fileURLToPath(new URL('../src/spec/gaps.json', import.meta.url))
const lines = readFileSync(dir + 'Exp.dc.html', 'utf8').split('\n')
const start = lines.findIndex((l) => l.startsWith('    const gp = {'))
const end = lines.findIndex((l, i) => i > start && l.startsWith('    };'))
if (start < 0 || end < 0) throw new Error('the gp table was not found in Exp.dc.html')
const order = []
for (let i = 1; i <= 6; i++) {
  const m = readFileSync(dir + `GB2_${i}.dc.html`, 'utf8').match(/const d = (\[\[.*?\]\]);/s)
  if (!m) throw new Error(`GB2_${i}: no page list`)
  for (const a of JSON.parse(m[1])) order.push(a[0])
}
// The canvas draws with its own colours; none of them are kept. A card's tint pair is (bg, bg2) and its chosen
// state is `st`, which the stand-in marks.
const stub = `
const E = new Proxy({}, { get: () => '' });
const phone = false, plat = 'desktop';
const R = (title, value, on, sub) => ({ title, ...(value ? { value } : {}), ...(sub ? { sub } : {}), on: !!on });
const Cc = (title, on) => ({ title, on: !!on });
const pgCardSt = (on) => on ? 'on' : '';
const A1 = (title) => ({ title, kind: 'primary' }), A2 = (title) => ({ title, kind: 'secondary' });
const card = (c) => ({ title: c.t, ...(c.s ? { sub: c.s } : {}), ...(c.bg ? { bg: c.bg } : {}), ...(c.bg2 ? { bg2: c.bg2 } : {}), on: c.st === 'on' });
const secC = (h, items) => ({ kind: 'cards', h, items: items.map(card) });
const secR = (h, items) => ({ kind: 'rows', h, items });
const secP = (h, items) => ({ kind: 'chips', h, items });
const secN = (h, text, tone) => ({ kind: 'note', h, text, tone: !!tone });
const mk = (eyebrow, title, lead, sections, actions, foot) => ({ eyebrow, title, lead, sections, actions: actions || [], foot: foot || '' });
const TH = new Proxy({}, { get: (t, k) => ({ name: String(k), bg: '#111', bg2: '#222', bg3: '#222', acc: '#7955ff', dark: true }) });
const uiList = [], thList = [], uiCards = () => [], themeCards = () => [], fx = [];
`
const gp = new Function(stub + lines.slice(start, end + 1).join('\n') + '\nreturn gp;')()
const pages = order.map((key) => {
  if (!gp[key]) throw new Error('no gp entry for ' + key)
  const { eyebrow, title, lead, sections, actions, foot } = gp[key]
  return { key, eyebrow, title, lead, sections, actions, foot }
})
const text = JSON.stringify(pages, null, 1) + '\n'
if (process.argv.includes('--check')) {
  if (readFileSync(out, 'utf8') !== text) { console.error('src/spec/gaps.json is out of date: run node scripts/spec-from-canvas.mjs'); process.exit(1) }
  console.log('gaps.json matches the canvas:', pages.length, 'pages')
} else { writeFileSync(out, text); console.log('wrote', pages.length, 'pages') }
