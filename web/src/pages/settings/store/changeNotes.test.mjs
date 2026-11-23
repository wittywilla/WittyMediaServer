// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What's new shows people the plain part of a module's notes (#549). Run with:
//   node --experimental-strip-types --test web/src/pages/settings/store/changeNotes.test.mjs
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { hasNews, splitNotes } from './changeNotes.ts'

test('headings: What’s new for people, Developer notes folded away', () => {
  const s = splitNotes("### What's new\n- Friends can invite each other into a game.\n\n### Developer notes\n- `POST /games/online/friends/{user}/invite`\n- `wittyplay.invite.accept`\n")
  assert.equal(s.people, '- Friends can invite each other into a game.')
  assert.match(s.developer, /POST \/games/)
  assert.doesNotMatch(s.people, /`|POST|wittyplay\./)
})

test('only a Developer notes heading: everything else is for people', () => {
  const s = splitNotes('- Plays faster on small machines.\n\n## Developer notes\n- host/runner.go keeps one decoder.')
  assert.equal(s.people, '- Plays faster on small machines.')
  assert.equal(s.developer, '- host/runner.go keeps one decoder.')
  const bold = splitNotes('- Plays faster.\n\n**Developer notes**\n- `x.go`')
  assert.equal(bold.people, '- Plays faster.')
  assert.equal(bold.developer, '- `x.go`')
})

test('an entry from before: the bold lead of each bullet is the plain part', () => {
  const old = "- **A virtual gamepad plays a game in the tests.** `web/tests/virtualpad.test.ts` stubs the browser's Gamepad API (`navigator.getGamepads`).\n- **Friends: invites, and batteries.** Someone playing invites a friend (`POST /games/online/friends/{user}/invite`)."
  const s = splitNotes(old)
  assert.equal(s.people, '- A virtual gamepad plays a game in the tests.\n- Friends: invites, and batteries.')
  assert.equal(s.developer, old)
  assert.doesNotMatch(s.people, /`|\/games\/|\.ts\b|\.go\b/)
})

test('plain bullets stay; ones naming code are developer notes', () => {
  const s = splitNotes('- Covers load faster.\n- `scan.go` reads each folder once.')
  assert.equal(s.people, '- Covers load faster.')
  assert.match(s.developer, /scan\.go/)
  const plain = splitNotes('- Covers load faster.')
  assert.equal(plain.developer, '')
  assert.equal(plain.people, '- Covers load faster.')
})

test('a release only about the README’s pictures is no news', () => {
  const s = splitNotes('- **The README’s pictures are retaken from WittyMediaServer 0.8.0-alpha.**')
  assert.equal(hasNews(s), false)
  assert.match(s.developer, /README/)
  assert.equal(hasNews(splitNotes('')), false)
  assert.equal(hasNews(splitNotes('### What’s new\n- Faster.')), true)
})
