// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Which steps first-time setup shows, and what it says it did. Run with:
//   node --experimental-strip-types --test web/src/pages/setup/plan.test.mjs
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { canMake, currentStep, DRAWN, drawnNumber, describeOutcome, GAMES_LINE, neighbours, offerable, onAnotherMachine, pathOf, planSteps, RECOMMENDED, STEPS, stepFromPath, updateChoiceFrom, updateSettings } from './plan.ts'

const facts = (over = {}) => ({ signedIn: true, store: true, modules: [], kinds: [], ...over })
const ids = (f) => planSteps(f).map((s) => s.id)
const advanced = (over = {}) => facts({ advanced: true, ...over })

test('quick setup is six steps, one page each, with nothing optional on a page of its own', () => {
  assert.deepEqual(DRAWN.map((d) => d.id), ['welcome', 'account', 'modules', 'media', 'look', 'ready'])
  assert.deepEqual(ids(facts({ signedIn: false, kinds: null })), ['welcome', 'account', 'modules', 'media', 'look', 'done'])
  assert.deepEqual(ids(facts()), ['modules', 'media', 'look', 'done'])
  assert.equal(ids(facts({ askData: true }))[0], 'storage', 'where data is kept comes first, on a desktop that asks')
})

test('modules are chosen before the media and the look', () => {
  const list = ids(facts())
  assert.ok(list.indexOf('modules') < list.indexOf('media') && list.indexOf('media') < list.indexOf('look'))
  assert.ok(!ids(facts({ store: false })).includes('modules'), 'a server with no store goes on without it')
})

test('advanced setup has a page for every choice, each with the step it belongs with', () => {
  const out = ids(advanced({ signedIn: false, kinds: null }))
  assert.deepEqual(out.slice(0, 3), ['welcome', 'locale', 'account'])
  const list = ids(advanced({ modules: ['wittytuner', 'wittyplay', 'wittydvd'] }))
  assert.deepEqual(list, ['name', 'people', 'modules', 'media', 'tv', 'games', 'discs', 'details', 'logos', 'music', 'karaoke', 'listings', 'look', 'colours', 'tvguide', 'network', 'remote', 'done'])
  for (const id of list) {
    const s = STEPS.find((x) => x.id === id)
    assert.ok(s.advanced || ids(facts({ modules: ['wittytuner', 'wittyplay', 'wittydvd'] })).includes(id), `${id} is in quick setup or marked advanced`)
  }
})

test('a module\'s pages only appear when the module is on this server', () => {
  assert.ok(!ids(advanced()).includes('games') && ids(advanced({ modules: ['wittyplay'] })).includes('games'))
  assert.ok(!ids(advanced()).includes('tv') && ids(facts({ modules: ['wittytuner'] })).includes('tv'))
  assert.ok(!ids(advanced()).includes('tvguide') && ids(advanced({ modules: ['wittytuner'] })).includes('tvguide'))
  assert.ok(ids(advanced({ modules: ['wittybluray'] })).includes('discs'))
  assert.ok(!ids(advanced({ modules: ['wittycd'] })).includes('discs'), 'an audio CD has no menu to ask about')
  assert.ok(!ids(facts({ modules: ['wittydvd'] })).includes('discs'), 'quick setup keeps the disc defaults')
})

test('lyrics, karaoke and streaming lists are asked only where they can be used', () => {
  const noMusic = { kinds: [{ type: 'music', available: false }] }
  assert.ok(ids(advanced()).includes('music') && !ids(advanced(noMusic)).includes('music') && !ids(advanced(noMusic)).includes('karaoke'))
  const off = (t) => ({ type: t, available: false })
  assert.ok(!ids(advanced({ kinds: [off('movies'), off('shows')] })).includes('listings'))
  assert.equal(canMake(facts({ kinds: null }), 'movies'), true)
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
  assert.equal(currentStep(inn, 'account', true).id, 'modules', 'an account is only made once')
  assert.equal(currentStep(inn, 'games', true).id, 'modules', 'a step for a module that isn\'t there')
})

test('a page only advanced setup has, or one from before 0.8.3, opens where its question is now', () => {
  const quick = planSteps(facts())
  assert.equal(currentStep(quick, 'theme', true).id, 'look')
  assert.equal(currentStep(quick, 'logos', true).id, 'done')
  assert.equal(currentStep(quick, 'remote', true).id, 'done')
  const adv = planSteps(advanced({ modules: ['wittytuner'] }))
  assert.equal(currentStep(adv, 'theme', true).id, 'colours')
  assert.equal(currentStep(adv, 'wittytv', true).id, 'tvguide')
  assert.equal(currentStep(planSteps(advanced({ signedIn: false, kinds: null })), 'language', false).id, 'locale')
})

test('Back goes to the page before, but never back to making an account', () => {
  const inn = planSteps(facts())
  assert.equal(neighbours(inn, 'modules').back, undefined)
  assert.equal(neighbours(inn, 'media').back.id, 'modules')
  assert.equal(neighbours(planSteps(advanced()), 'name').back, undefined)
  const out = planSteps(facts({ signedIn: false, kinds: null }))
  assert.equal(neighbours(out, 'account').back.id, 'welcome')
})

test('the last page lists only what was chosen', () => {
  assert.deepEqual(describeOutcome({ libraries: [], modules: [] }), ['No libraries yet'])
  assert.deepEqual(describeOutcome({ libraries: ['Movies', 'Games'], modules: ['WittyPlay'] }), ['Libraries: Movies and Games', 'Modules: WittyPlay'])
})

test('the step number counts the six steps, and the pages of one step share its number', () => {
  const steps = planSteps(advanced({ modules: ['wittyplay', 'wittytuner'] }))
  assert.deepEqual(drawnNumber(planSteps(facts({ signedIn: false, kinds: null })), 'welcome'), { at: 1, of: 6 })
  assert.deepEqual(drawnNumber(planSteps(facts()), 'media'), { at: 4, of: 6 })
  assert.deepEqual(drawnNumber(steps, 'tv'), { at: 4, of: 6, page: { at: 2, of: 8 } })
  assert.deepEqual(drawnNumber(steps, 'colours'), { at: 5, of: 6, page: { at: 2, of: 3 } })
  assert.deepEqual(drawnNumber(steps, 'done'), { at: 6, of: 6, page: { at: 3, of: 3 } })
})

test('the games line is word for word what was asked for', () => {
  assert.equal(GAMES_LINE.replace(/’/g, "'"), "Point it at your games folder and leave it. Anything it can't prove is listed for you to look at.")
})
