// Finding settings by name, and that every place a result jumps to still exists. Run with:
//   node --experimental-strip-types --test web/src/pages/settings/searchIndex.test.mjs
import assert from 'node:assert/strict'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { test } from 'node:test'
import { ENTRIES, norm, searchSettings, sectionId } from './searchIndex.ts'

const src = new URL('../../', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1')
const files = []
const walk = (d) => { for (const f of readdirSync(d)) { const p = join(d, f); if (statSync(p).isDirectory()) walk(p); else if (p.endsWith('.tsx')) files.push(readFileSync(p, 'utf8')) } }
walk(src)
const all = files.join('\n')
const settingsPage = readFileSync(join(src, 'pages/SettingsPage.tsx'), 'utf8')

const PAGES = [...settingsPage.matchAll(/\{ to: '([^']+)', label: '([^']+)'[^}]*hint: '([^']+)'/g)].map(([, to, label, hint]) => ({ to, label, hint }))
const found = (q, pages = PAGES) => searchSettings(q, pages).map((r) => r.to)

test('the menu is read from SettingsPage', () => {
  assert.ok(PAGES.length > 20)
  assert.ok(PAGES.some((p) => p.to === 'appearance'))
})

test('every entry is on a page in the menu', () => {
  for (const e of ENTRIES) assert.ok(PAGES.some((p) => p.to === e.page), `${e.title}: no page ${e.page}`)
})

test('every section an entry jumps to still has that title', () => {
  for (const e of ENTRIES) {
    if (e.section) assert.ok(all.includes(`title="${e.section}"`), `no section titled “${e.section}” (for ${e.title})`)
    if (e.at) assert.ok(all.includes(`id="${e.at}"`), `nothing has the id ${e.at} (for ${e.title})`)
  }
})

test('section anchors are plain words', () => {
  assert.equal(sectionId('Name and logo'), 'name-and-logo')
  assert.equal(sectionId('Moved a drive?'), 'moved-a-drive')
  assert.equal(sectionId('Other people’s two-factor sign-in'), 'other-peoples-two-factor-sign-in')
})

test('either spelling of colour finds the colours', () => {
  assert.equal(norm('Color'), norm('Colour'))
  for (const q of ['colour', 'color', 'colors', 'accent']) assert.ok(found(q).includes('/settings/appearance#colours'), q)
})

test('themes, dark mode and the TV guide look are found', () => {
  assert.equal(found('theme')[0], '/settings/appearance#theme')
  assert.ok(found('dark mode').includes('/settings/appearance#theme'))
  assert.ok(found('tv guide').includes('/settings/appearance#wittytv'))
  assert.ok(found('teletext').includes('/settings/appearance#wittytv'))
  assert.ok(found('big screen').includes('/settings/appearance#motion-and-big-screens'))
  assert.ok(found('menu').includes('/settings/appearance#navigation'))
})

test('pages are found by their name and what they are for', () => {
  assert.equal(found('libraries')[0], '/settings/libraries')
  assert.ok(found('posters').includes('/settings/metadata'))
  assert.ok(found('two factor').includes('/settings/security#two-factor-sign-in'))
})

test('every word has to match, from the start of a word', () => {
  assert.deepEqual(found('theme zzzz'), [])
  assert.ok(!found('heme').length)
  assert.deepEqual(found('   '), [])
})

test('settings on pages someone can’t open aren’t offered', () => {
  const mine = PAGES.filter((p) => ['users', 'security', 'appearance'].includes(p.to))
  assert.ok(found('tuners', mine).length === 0)
  assert.ok(found('theme', mine).length > 0)
})

test('module pages can be found too', () => {
  const r = searchSettings('weather', PAGES, [{ to: '/m/weather/settings', title: 'Weather', where: 'From modules' }])
  assert.equal(r[0].to, '/m/weather/settings')
})
