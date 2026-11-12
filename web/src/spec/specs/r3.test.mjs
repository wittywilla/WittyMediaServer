// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { problems, reachable, withIds } from '../model.ts'
import { detailSpec, tabsSpec, toolsSpec } from './fixMatches.ts'
import { brandSpec } from './brand.ts'
import { unreachableSpec } from './unreachable.ts'
import { youSpec } from './you.ts'

const ok = (p) => assert.deepEqual(problems(withIds(p)), [])

test('fix matches: tabs carry counts and mark the current one', () => {
  const p = tabsSpec([['review', 'Needs a look'], ['removed', 'Removed']], { review: 4, removed: 9 }, 'review')
  ok(p)
  assert.deepEqual(p.sections[0].items.map((c) => [c.id, c.title, c.on]), [['tab:review', 'Needs a look 4', true], ['tab:removed', 'Removed', false]])
})

test('fix matches: tools change with a running job and offer the sure ones only when several', () => {
  const idle = toolsSpec('abc', null, 3)
  ok(idle)
  assert.equal(idle.sections[0].items[0].value, 'abc')
  assert.deepEqual(idle.sections[1].items.map((c) => c.id), ['find', 'sure'])
  assert.deepEqual(toolsSpec('', null, 1).sections[1].items.map((c) => c.id), ['find'])
  const run = toolsSpec('', { running: true, done: 2, total: 9, current: 'Low Tide' }, 0)
  assert.deepEqual(run.sections[1].items.map((c) => c.id), ['stop'])
  assert.match(run.sections[2].text, /2 of 9, Low Tide/)
})

test('fix matches: the open title lists suggestions with Look and Accept, numbered to six', () => {
  const list = Array.from({ length: 7 }, (_, i) => ({ title: 'T' + i, year: 2000 + i, kind: 'movie', score: 90.4, source: 'tmdb', sourceName: 'TMDB' }))
  const p = detailSpec({ state: 'low', target: 'item', reasons: ['Year differs'], matchedWith: 'TMDB', matchedUrl: 'https://x.test', query: '', busy: false, err: '', warnings: [], results: false, list })
  ok(p)
  const sug = p.sections.find((s) => s.h === 'Suggestions')
  assert.equal(sug.items.length, 7)
  assert.equal(sug.items[0].title, '1. T0')
  assert.equal(sug.items[6].title, 'T6')
  assert.deepEqual(sug.items[0].actions.map((a) => a.id), ['look:0', 'accept:0'])
  assert.equal(sug.items[0].value, '90% · TMDB')
  assert.equal(p.sections[0].items.find((c) => c.id === 'leave').title, 'It’s right: stop asking')
  assert.equal(p.sections[1].items[0].href, 'https://x.test')
})

test('fix matches: no results says so, errors and warnings are notes, shows can be renumbered', () => {
  const p = detailSpec({ state: 'unmatched', target: 'show', reasons: [], query: 'x', busy: false, err: 'Boom', warnings: ['Slow'], results: true, list: [] })
  ok(p)
  const kinds = p.sections.map((s) => s.kind + ':' + (s.text || s.h))
  assert.ok(kinds.includes('note:Boom') && kinds.includes('note:Slow'))
  assert.ok(p.sections.at(-1).text.startsWith('No source has anything'))
  assert.ok(p.sections[0].items.some((c) => c.id === 'renumber'))
  const busy = detailSpec({ state: 'manual', target: 'item', reasons: [], query: '', busy: true, err: '', warnings: [], results: false, list: [] })
  assert.ok(!busy.sections.some((s) => s.kind === 'note'))
  assert.equal(busy.sections.find((s) => s.kind === 'chips' && s.items[0].id === 'go').items[0].disabled, true)
})

test('brand: only the controls that apply', () => {
  assert.deepEqual(brandSpec('studio', 'Harbour', false, false).sections, [])
  const p = brandSpec('network', 'Harbour', true, true)
  ok(p)
  assert.deepEqual(p.sections[0].items.map((c) => c.id), ['all-libraries', 'change-logo'])
  assert.equal(p.eyebrow, 'Network · Harbour')
})

test('unreachable: counts down, disables Try again while trying, helps with a link', () => {
  const p = unreachableSpec({ address: 'http://x:1', trying: false, seconds: 1, tries: 3 })
  ok(p)
  assert.equal(p.lead, 'Trying again in 1 second.')
  assert.equal(p.sections[0].items[1].value, '3 times')
  assert.equal(p.actions[1].to, '/help')
  assert.equal(unreachableSpec({ address: 'a', trying: true, seconds: 5, tries: 1 }).actions[0].disabled, true)
  assert.deepEqual(reachable(withIds(p), new Set(['retry'])).length, 2)
})

const you = { shown: 'Willow', adult: true, isAdmin: true, twoStep: true, devices: 3, games: true, gamesOff: false, sharingHidden: false, activityHidden: false, themeName: 'Witty', newer: false, version: '0.8.0', address: 'http://s', print: 'ab12', native: false }

test('you: an administrator sees everything, every row goes somewhere and has an icon', () => {
  const p = youSpec(you)
  ok(p)
  assert.deepEqual(p.sections.map((s) => s.h), ['Account', 'Household', 'Yours', 'On this device', 'Server', 'Maintenance', 'Help and about', 'This server', ''])
  for (const s of p.sections.slice(0, 7)) for (const r of s.items) { assert.ok(r.to, r.title); assert.ok(r.icon, r.title) }
  assert.equal(p.sections[0].items[2].value, 'On')
  assert.equal(p.sections.at(-1).items.map((c) => c.id).join(), 'sign-out')
})

test('you: kids and guests get fewer rows, an update turns About into the update link, native apps can change server', () => {
  const kid = youSpec({ ...you, adult: false, isAdmin: false, twoStep: undefined, games: false, sharingHidden: true, activityHidden: true, native: true, newer: true })
  ok(kid)
  assert.deepEqual(kid.sections.map((s) => s.h), ['Account', 'Yours', 'On this device', 'Help and about', 'This server', ''])
  assert.equal(kid.sections[0].items.length, 2)
  assert.deepEqual(kid.sections[1].items.map((r) => r.title), ['Your year', 'Collections and playlists'])
  const about = kid.sections[3].items[1]
  assert.deepEqual([about.to, about.value], ['/settings/about#updates', 'Update available'])
  assert.deepEqual(kid.sections.at(-1).items.map((c) => c.id), ['change-server', 'sign-out'])
  assert.equal(youSpec({ ...you, twoStep: null }).sections[0].items.length, 4 - 1 + 0)
})
