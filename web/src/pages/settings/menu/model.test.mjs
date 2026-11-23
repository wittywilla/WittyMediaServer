// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { holds, menuGroups, rowFor, rowLevel, rowSummary } from './model.ts'

const read = (f) => JSON.parse(readFileSync(new URL('../../../../../pkg/sdui/data/' + f, import.meta.url), 'utf8'))
const menu = read('settingsmenu.json')
const tv = read('tvlevels.json')
const who = (over = {}) => ({
  admin: true, adult: true, tv: false, twoStep: true, games: true, app: false, servers: false, fromAway: false,
  looks: true, layoutEditor: true, moduleSettings: true, remote: true, parties: true, off: new Set(), ...over,
})
const web = (r) => !!r.path
const titles = (gs) => gs.map((g) => [g.title, g.rows.map((r) => r.title)])

test('the groups are the Android phone app\'s, in its order', () => {
  assert.deepEqual(menu.groups.map((g) => g.title), ['Account', 'Household', 'Yours', 'Modules', 'On this device', 'Server', 'Maintenance', 'Help and about'])
  // Every row says how it opens and has an icon for every platform.
  for (const g of menu.groups) for (const r of g.rows) {
    assert.ok(r.path || r.native, r.id)
    assert.ok(r.icon.android && r.icon.apple && r.icon.web, r.id)
  }
})

test('an administrator on the web sees every row with a page', () => {
  const gs = menuGroups(menu, who(), web)
  const all = gs.flatMap((g) => g.rows.map((r) => r.id))
  for (const id of ['names', 'password', 'twoStep', 'devices', 'signInTv', 'profiles', 'quality', 'network', 'dataFolder', 'remote', 'pages', 'reset', 'licences']) assert.ok(all.includes(id), id)
  // The app's own screens aren't the web's.
  for (const id of ['topMenu', 'servers', 'away', 'appLog']) assert.ok(!all.includes(id), id)
})

test('module settings and the module store are side by side, in their own group', () => {
  const gs = menuGroups(menu, who(), web)
  assert.deepEqual(gs.find((g) => g.id === 'modules').rows.map((r) => r.title), ['Module settings', 'Module store'])
  assert.ok(!gs.find((g) => g.id === 'maintenance').rows.some((r) => r.path === '/settings/modules'))
  // Someone who isn't an administrator sees the settings alone.
  assert.deepEqual(menuGroups(menu, who({ admin: false }), web).find((g) => g.id === 'modules').rows.map((r) => r.id), ['moduleSettings'])
})

test('the desktop app adds its own rows', () => {
  const desktop = new Set(['servers', 'away', 'appLog'])
  const gs = menuGroups(menu, who({ app: true, servers: true, fromAway: true }), (r) => !!r.path || desktop.has(r.native))
  const device = gs.find((g) => g.id === 'device').rows.map((r) => r.id)
  assert.deepEqual(device, ['look', 'editLayout', 'theme', 'servers', 'away', 'playingHere'])
  assert.equal(gs.at(-1).rows.at(-1).id, 'appLog')
})

test('kids and guests get fewer rows, and nothing the server would refuse', () => {
  const kid = menuGroups(menu, who({ admin: false, adult: false, twoStep: false, games: false, off: new Set(['sharing', 'activity']) }), web)
  assert.deepEqual(kid.map((g) => g.id), ['account', 'yours', 'modules', 'device', 'help'])
  assert.deepEqual(kid[0].rows.map((r) => r.id), ['names', 'devices'])
  assert.deepEqual(kid[1].rows.map((r) => r.id), ['yourYear', 'collections', 'quality', 'remoteControl', 'watchParty'])
})

test('a TV signs in no TVs, and a condition nobody knows keeps a row out', () => {
  const gs = menuGroups(menu, who({ tv: true }), web)
  assert.ok(!gs.flatMap((g) => g.rows).some((r) => r.id === 'signInTv'))
  assert.equal(holds('someday', who()), false)
  assert.equal(holds('notOff:games', who({ off: new Set(['games']) })), false)
})

test('what a TV may change: rows keep their place and say where to change them', () => {
  const level = (id) => rowLevel(tv, menu.groups.flatMap((g) => g.rows).find((r) => r.id === id))
  assert.equal(level('libraries'), 'desktopOnly')
  assert.equal(level('nameAndLogo'), 'desktopOnly')
  assert.equal(level('editLayout'), 'desktopOnly')
  assert.equal(level('dataFolder'), 'desktopOnly')
  assert.equal(level('playback'), 'view')
  assert.equal(level('quality'), 'full')
  assert.equal(level('theme'), 'full')
  const row = (id) => menu.groups.flatMap((g) => g.rows).find((r) => r.id === id)
  assert.equal(rowSummary(tv, row('remote')), 'remote')
  assert.equal(rowSummary(tv, row('libraries')), 'libraries')
})

test('the menu beside a page marks the page it is', () => {
  const gs = menuGroups(menu, who(), web)
  assert.equal(rowFor(gs, '/settings/remote').id, 'remote')
  assert.equal(rowFor(gs, '/settings/users').id, 'names')
  assert.equal(rowFor(gs, '/settings/pages/navigation').id, 'pages')
  assert.equal(rowFor(gs, '/nowhere'), null)
})

test('every row\'s words are in the clients\' table of words too', () => {
  assert.equal(titles(menuGroups(menu, who(), web)).length, 8)
  for (const g of menu.groups) { assert.match(g.key, /^group[A-Z]/); for (const r of g.rows) assert.match(r.key, /^row[A-Z]/) }
})
