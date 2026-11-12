// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { problems, reachable, withIds } from '../model.ts'
import { helpSpec } from './help.ts'
import { INSTALL, moduleMissingSpec } from './moduleMissing.ts'

const groups = [{ title: 'Start here', guides: [{ file: 'GETTING_STARTED', title: 'Getting started', about: 'From installing.' }, { file: 'PLAYING', title: 'Watching', about: 'The player.' }] }, { title: 'Your media', guides: [{ file: 'TV', title: 'TV shows', about: 'Seasons.' }] }]
const slug = (f) => f.toLowerCase().replace(/_/g, '-')

test('help: every guide is a link, in group order, and the page is drawable', () => {
  const p = helpSpec(groups, '0.8.0-alpha', slug)
  assert.deepEqual(problems(withIds(p)), [])
  assert.equal(p.key, 'help')
  assert.equal(p.title, 'Guides')
  assert.deepEqual(p.sections.map((s) => s.h), ['Start', 'Start here', 'Your media'])
  assert.equal(p.sections[1].items[0].to, '/help/getting-started')
  assert.equal(p.sections[1].items[0].sub, 'From installing.')
  assert.match(p.lead, /0\.8\.0-alpha/)
  assert.equal(reachable(withIds(p), new Set()).length, 3 + 3)
})

const base = { ids: ['karaoke'], admin: true, mods: [{ id: 'karaoke', name: 'Karaoke', summary: 'Sing along.' }], adding: false, error: '' }

test('module missing: an admin gets an install button and the store', () => {
  const p = moduleMissingSpec(base)
  assert.deepEqual(problems(withIds(p)), [])
  assert.equal(p.title, 'This needs Karaoke')
  assert.equal(p.actions[0].id, INSTALL + 'karaoke')
  assert.equal(p.actions[0].kind, 'primary')
  assert.ok(p.sections.some((s) => s.kind === 'chips' && s.items.some((c) => c.to === '/settings/modules')))
  assert.ok(p.sections.some((s) => s.kind === 'note' && s.text === 'Karaoke: Sing along.'))
})

test('module missing: others are told who can install, with no button', () => {
  const p = moduleMissingSpec({ ...base, admin: false, mods: null })
  assert.equal(p.actions.length, 0)
  assert.ok(p.sections.some((s) => s.kind === 'note' && /administrator/.test(s.text)))
  assert.ok(!p.sections.some((s) => s.kind === 'chips' && s.items.some((c) => c.title === 'Open the store')))
})

test('module missing: installing shows progress and no button; a build problem hides it too', () => {
  const busy = moduleMissingSpec({ ...base, mods: [{ id: 'karaoke', name: 'Karaoke', phase: 'downloading', done: 1, total: 4 }] })
  assert.equal(busy.actions.length, 0)
  assert.ok(busy.sections.some((s) => s.kind === 'note' && /Installing Karaoke: 25%/.test(s.text)))
  const none = moduleMissingSpec({ ...base, mods: [{ id: 'karaoke', name: 'Karaoke', unavailable: 'Not built for this system.' }], error: 'Nope' })
  assert.equal(none.actions.length, 0)
  assert.ok(none.sections.some((s) => s.kind === 'note' && s.text === 'Nope'))
})

test('module missing: several modules, one primary; the what text wins as the title', () => {
  const p = moduleMissingSpec({ ...base, what: 'Games are not set up', ids: ['a', 'b'], mods: null, adding: true })
  assert.equal(p.title, 'Games are not set up')
  assert.deepEqual(p.actions.map((a) => a.kind), ['primary', 'secondary'])
  assert.ok(p.actions.every((a) => a.disabled))
  assert.deepEqual(problems(withIds(p)), [])
})
