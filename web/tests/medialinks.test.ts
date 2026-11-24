// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Where a sign-in may go in an address: only to the server an app outside it talks to, never on
// the server's own page (its cookie carries it) and never to another server.
import { before, describe, test } from 'node:test'
import assert from 'node:assert/strict'

const store = new Map<string, string>()
const g = globalThis as any
g.localStorage = { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => store.set(k, String(v)), removeItem: (k: string) => store.delete(k) }
g.window = g.window || { location: { protocol: 'http:', origin: 'http://192.168.1.10:9797' } }

let api: typeof import('../src/api')
before(async () => { api = await import('../src/api') })

describe('mediaUrl', () => {
  test('the server\'s own page: no token, over plain HTTP too', () => {
    store.clear()
    api.setToken('secret-session')
    assert.equal(api.mediaUrl('/api/items/1/poster'), '/api/items/1/poster')
    assert.equal(api.mediaUrlQ('/api/items/1/poster', 'v=2'), '/api/items/1/poster?v=2')
  })
  test('an app talking to a server elsewhere: the token, for that server only', () => {
    store.clear()
    api.setToken('secret-session')
    api.setServerBase('http://192.168.1.10:9797')
    assert.equal(api.mediaUrl('/api/items/1/poster'), 'http://192.168.1.10:9797/api/items/1/poster?token=secret-session')
    assert.equal(api.mediaUrl('http://192.168.1.10:9797/api/hls/x/master.m3u8?a=1'), 'http://192.168.1.10:9797/api/hls/x/master.m3u8?a=1&token=secret-session')
    assert.equal(api.mediaUrl('https://images.example/poster.jpg'), 'https://images.example/poster.jpg')
    assert.equal(api.mediaUrl('http://192.168.1.10:9798/x'), 'http://192.168.1.10:9798/x')
  })
  test('signed out: nothing added', () => {
    store.clear()
    api.setServerBase('http://192.168.1.10:9797')
    assert.equal(api.mediaUrl('/api/items/1/poster'), 'http://192.168.1.10:9797/api/items/1/poster')
  })
})
