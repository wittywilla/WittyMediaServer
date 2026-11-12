// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { HARDWARE, crfWords, transcodeSpec } from './transcode.ts'
import { problems, reachable, withIds } from '../model.ts'

const v = { directPlay: true, maxHeight: '0', audioCodec: 'aac', mode: 'easy', profile: 'balanced', preset: 'veryfast', crf: 21, maxBitrate: '', audioBitrate: '', videoArgs: '', outputArgs: '' }
const ids = (p) => p.sections.filter((s) => s.kind !== 'note').flatMap((s) => s.items.map((i) => i.id))

test('transcode easy: profiles are one choice, settings search anchors kept', () => {
  const p = transcodeSpec(v)
  assert.deepEqual(p.sections.filter((s) => s.anchor).map((s) => [s.h, s.anchor]), [['Playback', 'playback'], [HARDWARE, 'hardware-encoders'], ['Picture quality', 'picture-quality']])
  const prof = p.sections.at(-1)
  assert.equal(prof.choice, true)
  assert.deepEqual(prof.items.map((r) => [r.id, r.on]), [['profile:speed', false], ['profile:balanced', true], ['profile:quality', false]])
  assert.deepEqual(problems(withIds(p)), [])
})

test('transcode advanced and expert add their fields', () => {
  assert.deepEqual(ids(transcodeSpec({ ...v, mode: 'advanced' })).filter((i) => ['x264Preset', 'crf', 'maxBitrate', 'audioBitrate'].includes(i)), ['x264Preset', 'crf', 'maxBitrate', 'audioBitrate'])
  const e = transcodeSpec({ ...v, mode: 'expert', crf: 23 })
  assert.ok(ids(e).includes('ffmpegVideoArgs') && ids(e).includes('ffmpegOutputArgs'))
  assert.match(e.sections.find((s) => s.kind === 'fields' && s.items.some((f) => f.id === 'crf')).items[1].label, /23 \(good\)/)
  assert.deepEqual(problems(withIds(e)), [])
  assert.ok(reachable(withIds(e), new Set()).includes('crf'))
})

test('crf words', () => { assert.equal(crfWords(18), 'looks the same as the source'); assert.equal(crfWords(30), 'small files') })
