// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { httpsWords, timeZoneChoices } from './network.ts'

test('encrypted connections are said in plain words', () => {
  assert.match(httpsWords({ mode: 'off' }), /^Off\./)
  assert.match(httpsWords({ mode: 'acme', running: true, port: 9443, cert: { names: ['tv.example.com'], daysLeft: 1 } }), /port 9443 for tv.example.com, the certificate good for 1 more day\./)
  assert.match(httpsWords({ mode: 'acme', error: 'no certificate' }), /not working: no certificate/)
  assert.equal(httpsWords(null), 'Not read yet.')
})

test('the computer\'s own zone is first, and a saved zone is never lost', () => {
  const list = timeZoneChoices('America/Toronto', 'Mars/Olympus', ['Europe/Paris', 'America/Toronto'])
  assert.deepEqual(list[0], { value: '', label: 'The computer’s own (America/Toronto)' })
  assert.ok(list.some((z) => z.value === 'Mars/Olympus'))
  assert.ok(list.some((z) => z.value === 'Europe/Paris'))
})
