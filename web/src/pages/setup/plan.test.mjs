// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Which steps first-time setup shows, and what it says it did. Run with:
//   node --experimental-strip-types --test web/src/pages/setup/plan.test.mjs
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { ALL_DRAWN, canMake, canSkip, currentStep, drawnNumber, describeOutcome, GAMES_LINE, neighbours, offerable, onAnotherMachine, pathOf, planSteps, RECOMMENDED, STEPS, stepFromPath, stepNumber, updateChoiceFrom, updateSettings } from './plan.ts'

const facts = (over = {}) => ({ signedIn: true, store: true, modules: [], kinds: [], ...over })
const ids = (f) => planSteps(f).map((s) => s.id)

test('someone with no account starts at the welcome page, then makes one', () => {
  const steps = planSteps(facts({ signedIn: false, kinds: null }))
  assert.deepEqual(steps.slice(0, 2).map((s) => s.id), ['welcome', 'account'])
  assert.equal(steps.at(-1).id, 'done')
})

test('once there is an account, the first two steps are gone', () => {
  const list = ids(facts())
  assert.ok(!list.includes('welcome') && !list.includes('account'))
  assert.equal(list[0], 'name')
})

test('a module\'s step only appears when the module is on this server', () => {
  assert.ok(!ids(facts()).includes('games'))
  assert.ok(ids(facts({ modules: ['wittyplay'] })).includes('games'))
  assert.ok(!ids(facts()).includes('discs'))
  assert.ok(ids(facts({ modules: ['wittydvd'] })).includes('discs'))
  assert.ok(ids(facts({ modules: ['wittybluray'] })).includes('discs'))
  assert.ok(!ids(facts({ modules: ['wittycd'] })).includes('discs'), 'an audio CD has no menu to ask about')
})

test('live TV is offered only where WittyTuner is', () => {
  assert.ok(!ids(facts()).includes('tv'))
  const list = ids(facts({ modules: ['wittytuner'] }))
  assert.ok(list.includes('tv') && list.indexOf('tv') < list.indexOf('done'))
})

test('a server with no store skips choosing modules, and goes on working', () => {
  const list = ids(facts({ store: false }))
  assert.ok(!list.includes('modules'))
  assert.ok(list.includes('media') && list.includes('done'))
})

test('lyrics are asked only where music can be played', () => {
  assert.ok(ids(facts()).includes('music'), 'a kind no module claims can be made')
  assert.ok(ids(facts({ kinds: [{ type: 'music', available: true }] })).includes('music'))
  assert.ok(!ids(facts({ kinds: [{ type: 'music', available: false }] })).includes('music'))
})

test('streaming lists are asked only where movies or shows can be made', () => {
  const off = (t) => ({ type: t, available: false })
  assert.ok(ids(facts()).includes('listings'))
  assert.ok(ids(facts({ kinds: [off('movies')] })).includes('listings'))
  assert.ok(!ids(facts({ kinds: [off('movies'), off('shows')] })).includes('listings'))
  assert.equal(canMake(facts({ kinds: null }), 'movies'), true)
})

test('every step that asks something says where to change it later, and that page exists', () => {
  const routes = readFileSync(new URL('../SettingsPage.tsx', import.meta.url), 'utf8')
  const labels = [...routes.matchAll(/to: '([a-z-]+)', label: '([^']+)'/g)].map((m) => [m[1], m[2]])
  for (const s of STEPS) {
    if (s.id === 'welcome' || s.id === 'account' || s.id === 'done') continue
    assert.ok(s.later, `${s.id} has nowhere to change it later`)
    const page = s.later.to.replace('/settings/', '').split('#')[0]
    assert.ok(routes.includes(`path="${page}"`) || routes.includes(`path="${page}/*"`), `no settings page ${page}`)
    const label = labels.find(([to]) => to === page)?.[1]
    assert.ok(s.later.where.startsWith('Settings, ' + label), `${s.id}: "${s.later.where}" isn't what the menu calls it: ${label}`)
  }
})

test('the sections the later notes name are really on the page', () => {
  const file = (name) => readFileSync(new URL(name, import.meta.url), 'utf8')
  assert.match(file('../settings/MetadataOptional.tsx'), /id="logos"/)
  assert.match(file('../settings/MetadataOptional.tsx'), /id="listings"/)
  assert.match(file('../MusicOnline.tsx'), /id="lyrics"/)
})

test('an address picks its step, and one that can\'t be shown goes to the first', () => {
  const out = planSteps(facts({ signedIn: false, kinds: null }))
  assert.equal(stepFromPath(pathOf('look')), 'look')
  assert.equal(stepFromPath('/'), '')
  assert.equal(stepFromPath('/setup/look/extra'), '')
  assert.equal(currentStep(out, 'account', false).id, 'account')
  assert.equal(currentStep(out, 'look', false).id, 'welcome', 'nothing past the account without one')
  const inn = planSteps(facts())
  assert.equal(currentStep(inn, 'look', true).id, 'look')
  assert.equal(currentStep(inn, 'account', true).id, 'name', 'an account is only made once')
  assert.equal(currentStep(inn, 'games', true).id, 'name', 'a step for a module that isn\'t there')
})

test('Back goes to the step before, but never back to making an account', () => {
  const inn = planSteps(facts())
  assert.equal(neighbours(inn, 'name').back, undefined)
  assert.equal(neighbours(inn, 'media').back.id, 'modules')
  assert.deepEqual(ids(facts()).slice(0, 4), ['name', 'look', 'theme', 'modules'])
  assert.equal(neighbours(inn, 'media').next.id, inn[inn.findIndex((s) => s.id === 'media') + 1].id)
  const out = planSteps(facts({ signedIn: false, kinds: null }))
  assert.equal(neighbours(out, 'account').back.id, 'welcome')
})

test('the three update answers are the two settings, there and back', () => {
  for (const c of ['ask', 'auto', 'never']) assert.equal(updateChoiceFrom(updateSettings(c)), c)
  assert.deepEqual(updateSettings('auto'), { moduleUpdateCheck: '1', moduleAutoInstall: '1' })
  assert.deepEqual(updateSettings('never'), { moduleUpdateCheck: '0', moduleAutoInstall: '0' })
  assert.equal(updateChoiceFrom({}), 'ask', 'installing by itself is off until chosen')
  assert.equal(updateChoiceFrom({ updateCheck: '0' }), 'never', 'follows the server\'s own check until set')
})

test('what\'s inside other modules isn\'t offered on its own', () => {
  const items = [
    { id: 'a', official: true, fit: { kind: 'ok' } },
    { id: 'b', official: true, fit: { kind: 'no-version' } },
    { id: 'wittyaudio', official: true, kind: 'server', fit: { kind: 'ok' } },
    { id: 'wittycouchhost', official: true, fit: { kind: 'ok' } },
    { id: RECOMMENDED[0], official: true, core: true, fit: { kind: 'no-version' } },
    { id: 'core', official: true, core: true, fit: { kind: 'ok' } },
    { id: 'lib', official: true, delivery: 'library', fit: { kind: 'library' } },
    { id: 'theirs', official: false, kind: 'fork', fit: { kind: 'ok' } },
  ]
  assert.deepEqual(offerable(items).map((i) => i.id), ['a', 'b', RECOMMENDED[0]])
})

test('a share on another machine is seen however it\'s typed', () => {
  for (const p of ['\\\\nas\\media', '//nas/media', '  \\\\nas\\media', '\\\\?\\UNC\\nas\\x']) assert.equal(onAnotherMachine(p), true, p)
  for (const p of ['D:\\Movies', '/mnt/nas/media', 'Z:\\', '']) assert.equal(onAnotherMachine(p), false, p)
})

test('the last page says what\'s so, and leaves out what this server never asked', () => {
  const plain = { libraries: [], modules: [], logos: '', lyrics: false, artists: false, listings: false, region: '', updates: 'ask' }
  const lines = describeOutcome(plain, planSteps(facts())).map((l) => l.text)
  assert.ok(lines.includes('No libraries yet'))
  assert.ok(lines.includes('Logos: plain names, nothing downloaded'))
  assert.ok(lines.includes('Streaming lists: off'))
  const full = describeOutcome({ libraries: ['Movies', 'Games'], modules: ['WittyPlay'], logos: 'yes', lyrics: true, artists: true, listings: true, region: 'Canada', updates: 'auto' }, planSteps(facts())).map((l) => l.text)
  assert.ok(full.includes('Libraries: Movies and Games'))
  assert.ok(full.includes('Modules: WittyPlay'))
  assert.ok(full.includes('Module updates: installed by themselves'))
  assert.ok(full.includes('Logos: downloading from Wikimedia Commons'))
  assert.ok(full.includes('Lyrics: looked up online when someone opens them'))
  assert.ok(full.includes('Streaming lists: on, for Canada'))
  const noMusic = describeOutcome(plain, planSteps(facts({ kinds: [{ type: 'music', available: false }], store: false }))).map((l) => l.text)
  assert.ok(!noMusic.some((t) => t.startsWith('Lyrics')) && !noMusic.some((t) => t.startsWith('Module')))
})

test('the step number doesn\'t go back to 1 after making an account', () => {
  const out = planSteps(facts({ signedIn: false, kinds: null }))
  const inn = planSteps(facts())
  assert.deepEqual(stepNumber(1, out.length, false), { at: 2, of: out.length })
  assert.deepEqual(stepNumber(0, inn.length, true), { at: 3, of: out.length })
})

test('the games line is word for word what was asked for', () => {
  assert.equal(GAMES_LINE.replace(/’/g, "'"), "Point it at your games folder and leave it. Anything it can't prove is listed for you to look at.")
})

test('there are always fourteen drawn steps, and the pages of Details and artwork share one number', () => {
  const steps = planSteps(facts({ modules: ['wittyplay', 'wittydvd', 'wittytuner'] }))
  assert.equal(ALL_DRAWN.length, 14)
  assert.deepEqual(drawnNumber(steps, 'details'), { at: 8, of: 14, page: { at: 1, of: 4 } })
  assert.deepEqual(drawnNumber(steps, 'listings'), { at: 8, of: 14, page: { at: 4, of: 4 } })
  assert.deepEqual(drawnNumber(steps, 'tv'), { at: 9, of: 14 })
  assert.deepEqual(drawnNumber(steps, 'done'), { at: 14, of: 14 })
  assert.deepEqual(drawnNumber(planSteps(facts()), 'done'), { at: 14, of: 14 }, 'the total doesn\'t change with the modules')
})

test('every step that says where to change it later can be passed, the last cannot', () => {
  const byId = Object.fromEntries(STEPS.map((s) => [s.id, s]))
  assert.ok(canSkip(byId.look) && canSkip(byId.theme) && canSkip(byId.name) && canSkip(byId.media))
  assert.ok(!canSkip(byId.done) && !canSkip(byId.welcome))
})
