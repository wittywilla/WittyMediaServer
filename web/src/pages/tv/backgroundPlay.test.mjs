// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { keepsPlaying, letGo, readChoice, resumeOnShow, sayStillHere } from './backgroundPlay.ts'

test('a device that hasn\'t chosen keeps playing, unless the server says new devices stop', () => {
  assert.equal(keepsPlaying(null, undefined), true)
  assert.equal(keepsPlaying(null, true), true)
  assert.equal(keepsPlaying(null, false), false)
})

test('a device\'s own choice wins over the server\'s', () => {
  assert.equal(keepsPlaying('1', false), true)
  assert.equal(keepsPlaying('0', true), false)
  assert.equal(keepsPlaying('0', undefined), false)
})

test('only "1" and "0" are choices', () => {
  assert.equal(readChoice('1'), '1')
  assert.equal(readChoice('0'), '0')
  for (const v of [null, undefined, '', 'true', 'on', '2']) assert.equal(readChoice(v), null)
})

test('the channel is let go only when hidden and not kept', () => {
  assert.equal(letGo(false, false), false)
  assert.equal(letGo(false, true), false)
  assert.equal(letGo(true, true), false)
  assert.equal(letGo(true, false), true)
})

test('a hidden player that keeps playing says it is still there', () => {
  assert.equal(sayStillHere(true, true, 'tun-1'), true)
  assert.equal(sayStillHere(true, true, ''), false)
  assert.equal(sayStillHere(true, false, 'tun-1'), false)
  assert.equal(sayStillHere(false, true, 'tun-1'), false)
})

test('shown again, only a pause the viewer didn\'t ask for is undone', () => {
  assert.equal(resumeOnShow(true, true, false), true)
  assert.equal(resumeOnShow(true, true, true), false)
  assert.equal(resumeOnShow(false, true, false), false)
  assert.equal(resumeOnShow(true, false, false), false)
})
