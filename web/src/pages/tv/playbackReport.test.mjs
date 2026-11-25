// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The playback report reads as it should and carries nothing that signs in or finds the server.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { formatReport } from './playbackReport.ts'

const base = {
  when: '2026-11-25 10:00', version: '0.8.7-alpha', channel: '651 NAS TV', via: 'p2p', away: true, choice: 'h720-b3000', autoStep: 2, autoStepText: '720p 3000 kbit/s',
  encoder: 'libx264', gpu: '', clock: 42.5, buffered: [[12, 52.1]], paused: false, readyState: 4, frames: { total: 1000, dropped: 3 },
  size: '1280x720', browser: 'Chrome 142',
  frags: [{ at: 4100, sn: 0, cc: 0, bytes: 600 * 1024, durationSec: 2, loadMs: 1100, firstByteMs: 60 }, { at: 6100, sn: 1, cc: 0, bytes: 500 * 1024, durationSec: 2, loadMs: 900, firstByteMs: 55 }],
  stalls: [{ at: 12000 }], autoChanges: [{ at: 9000, from: 0, to: 2, why: 'the link' }],
}

test('the report says the connection, quality, encoder, buffers, stalls and every segment', () => {
  const r = formatReport(base)
  for (const want of ['Server: 0.8.7-alpha', 'Connection: p2p, away from home', 'Auto step 2: 720p 3000 kbit/s', 'Encoder: libx264', '9.6 s buffered ahead', 'Stalls: 1 at 12.0 s', 'step 0→2 (the link)', 'Segments: 2, stream 2253 kbit/s']) {
    assert.ok(r.includes(want), `lacks ${want}:\n${r}`)
  }
  assert.equal(r.split('\n').filter((l) => /^\s+\d+\.\d\s+\d+\s/.test(l)).length, 2, 'a line for each segment')
})

test('nothing that signs in or finds the server', () => {
  const r = formatReport(base)
  assert.ok(!/token|secret|password|https?:\/\/|12D3Koo|\/api\//i.test(r), r)
})

test('before anything has loaded', () => {
  const r = formatReport({ ...base, frags: [], stalls: [], autoChanges: [], autoStep: null, choice: '', buffered: [] })
  assert.ok(r.includes('Segments: none loaded yet') && r.includes('Quality: full') && r.includes('Buffered: nothing'), r)
})
