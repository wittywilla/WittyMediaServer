// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Live TV's Auto quality, fed with fragments as a 5 Mbit/s peer-to-peer link gave them (measured on the
// 0.8.6 image at 2 CPUs: 2-second segments of 1.2 to 1.6 MB, each taking about 2.2 s to come).
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { autoStep, LADDER, PEER_START, rates, startStep, stepChoice, UP_AFTER_SEC } from './liveAuto.ts'

const frag = (mb, sec, loadMs) => ({ bytes: mb * 1024 * 1024, durationSec: sec, loadMs })

test('a step is the choice the server reads', () => {
  assert.equal(stepChoice(0), '')
  assert.equal(stepChoice(PEER_START), 'h720-b3000')
  assert.equal(stepChoice(99), `h${LADDER.at(-1).height}-b${LADDER.at(-1).kbps}`)
})

test('peer to peer starts lower, unless this device has learned better', () => {
  assert.equal(startStep(null, 'http'), 0)
  assert.equal(startStep(null, 'p2p'), PEER_START)
  assert.equal(startStep(null, 'p2p-relay'), PEER_START)
  assert.equal(startStep(1, 'p2p'), 1)
})

test('a stream faster than the link steps down (the owner\'s 5 Mbit/s case)', () => {
  const frags = [frag(1.4, 2, 2300), frag(1.3, 2, 2250), frag(1.5, 2, 2500)]
  const r = rates(frags)
  assert.ok(r.stream > 5e6 && r.link < 5e6, JSON.stringify(r))
  assert.equal(autoStep(0, { frags, stalls: 0, sinceChangeSec: 10 }), 'down')
})

test('a stream well within the link is kept, and stalls step down whatever the numbers', () => {
  const ok = [frag(0.7, 2, 300), frag(0.7, 2, 320), frag(0.7, 2, 310)]
  assert.equal(autoStep(PEER_START, { frags: ok, stalls: 0, sinceChangeSec: 10 }), 'keep')
  assert.equal(autoStep(PEER_START, { frags: ok, stalls: 2, sinceChangeSec: 10 }), 'down')
})

test('up only with plenty of room, after a while, and never below the bottom', () => {
  const fast = [frag(0.7, 2, 50), frag(0.7, 2, 50), frag(0.7, 2, 50)] // about 117 Mbit/s
  assert.equal(autoStep(PEER_START, { frags: fast, stalls: 0, sinceChangeSec: 10 }), 'keep')
  assert.equal(autoStep(PEER_START, { frags: fast, stalls: 0, sinceChangeSec: UP_AFTER_SEC }), 'up')
  const slow = [frag(0.2, 2, 3000), frag(0.2, 2, 3000), frag(0.2, 2, 3000)]
  assert.equal(autoStep(LADDER.length - 1, { frags: slow, stalls: 5, sinceChangeSec: 10 }), 'keep')
  assert.equal(autoStep(0, { frags: [frag(1, 2, 100)], stalls: 0, sinceChangeSec: 0 }), 'keep', 'too few fragments to tell')
})
