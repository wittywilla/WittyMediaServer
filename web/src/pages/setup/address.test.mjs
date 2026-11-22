// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { bestAddress, onlyHere } from './address.ts'

const at = (origin) => ({ origin, hostname: new URL(origin).hostname })

test('setup always ends with an address (#536)', () => {
  // Listening on this computer only, opened at a .localhost name: the address it answers on, and where it will be.
  const local = bestAddress(at('http://setuppolish.localhost:19561'), { lanUrls: [], lanGuess: 'http://192.168.1.20:19561', localOnly: true })
  assert.deepEqual(local, { url: 'http://setuppolish.localhost:19561', reachable: false, later: 'http://192.168.1.20:19561' })
  // Even with nothing from the server.
  assert.equal(bestAddress(at('http://localhost:9797'), null).url, 'http://localhost:9797')
  assert.equal(bestAddress(at('http://127.0.0.1:9797'), { lanUrls: [] }).reachable, false)
})

test('the home-network address when the page was opened at localhost', () => {
  const b = bestAddress(at('http://localhost:9797'), { lanUrls: ['http://192.168.1.20:9797', 'http://172.20.0.1:9797'], lanGuess: 'http://192.168.1.20:9797' })
  assert.deepEqual(b, { url: 'http://192.168.1.20:9797', reachable: true, also: 'http://172.20.0.1:9797' })
})

test('the address in use when other devices can use it too', () => {
  const b = bestAddress(at('http://mediapc.lan:9797'), { lanUrls: ['http://192.168.1.20:9797'] })
  assert.deepEqual(b, { url: 'http://mediapc.lan:9797', reachable: true, also: 'http://192.168.1.20:9797' })
  assert.equal(bestAddress(at('http://192.168.1.20:9797'), { lanUrls: ['http://192.168.1.20:9797'] }).also, undefined)
})

test('names only this computer answers to', () => {
  for (const h of ['localhost', '127.0.0.1', '127.1.2.3', '[::1]', 'rc.localhost', 'A.LOCALHOST']) assert.ok(onlyHere(h), h)
  for (const h of ['192.168.1.20', 'mediapc.lan', 'localhost.example.com', '1127.0.0.1']) assert.ok(!onlyHere(h), h)
})
