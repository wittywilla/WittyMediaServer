// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'

// A browser's storage, for this device's defaults.
const mem = () => {
  const store = new Map()
  return { store, getItem: (k) => (store.has(k) ? store.get(k) : null), setItem: (k, v) => store.set(k, String(v)), removeItem: (k) => store.delete(k) }
}
globalThis.localStorage = mem()
globalThis.sessionStorage = mem()
const q = await import('./quality.ts')

const PRESETS = [
  { name: '1080p', height: 1080, kbps: 8000 }, { name: '720p', height: 720, kbps: 4000 },
  { name: '480p', height: 480, kbps: 1500 }, { name: '360p', height: 360, kbps: 800 },
]

test('a choice reads and writes as the server does', () => {
  assert.equal(q.formatChoice(q.parseChoice('b4000,h720')), 'h720-b4000')
  assert.equal(q.formatChoice(q.parseChoice('h1080-b8000-vh264-a192-c2')), 'h1080-b8000-vh264-a192-c2')
  assert.equal(q.parseChoice(''), null, 'Auto is no choice')
  assert.equal(q.parseChoice('auto'), null)
  assert.equal(q.formatChoice(q.parseChoice('original')), 'original')
  assert.equal(q.formatChoice(q.parseChoice('h99999-vav1-c3')), 'original', 'out of range is dropped')
})

test('the menu marks the entry a choice is', () => {
  assert.equal(q.entryOf('', PRESETS), 'auto')
  assert.equal(q.entryOf('original', PRESETS), 'original')
  assert.equal(q.entryOf(q.presetChoice(PRESETS[1]), PRESETS), '720p')
  assert.equal(q.entryOf('h720-b3000', PRESETS), 'custom', 'a size at another rate is the advanced menu’s')
  assert.equal(q.entryOf('h720-b4000-c2', PRESETS), 'custom')
  assert.equal(q.describe('h720-b4000', PRESETS), '720p')
  assert.equal(q.describe('h900-b2500', PRESETS), '900p · 2.5 Mbps')
})

test('data use in words', () => {
  assert.equal(q.rate(8000), '8 Mbps')
  assert.equal(q.rate(1500), '1.5 Mbps')
  assert.equal(q.rate(800), '800 kbps')
  assert.equal(q.rate(20000), '20 Mbps')
  assert.equal(q.perHour(8000), '3.6 GB an hour')
  assert.equal(q.perHour(800), '360 MB an hour')
  assert.equal(q.sizeName(800, 1920), '1080p', 'a wide film is named by its width')
  assert.equal(q.sizeName(2160, 3840), '4K')
})

test('this device keeps a default for home and for away', () => {
  assert.equal(q.deviceChoice(), '', 'nothing chosen: Auto')
  q.setDeviceDefault('home', 'h1080-b8000')
  q.setDeviceDefault('away', 'b1500-h480')
  assert.equal(q.deviceChoice(), 'h1080-b8000', 'home until the server says otherwise')
  q.noteAway(true)
  assert.equal(q.here(), 'away')
  assert.equal(q.deviceChoice(), 'h480-b1500')
  q.noteAway(false)
  q.setDeviceDefault('home', '')
  q.setDeviceDefault('away', '')
  assert.equal(localStorage.getItem('wms.quality'), null, 'Auto everywhere keeps nothing')
})

test('only requests that start a stream carry the choice', () => {
  assert.ok(q.startsStream('POST', '/items/12/dvd'))
  assert.ok(q.startsStream('GET', '/tv/lib-3/tune?viewer=abc'))
  assert.ok(q.startsStream('GET', '/tv/tun-3/catchup?start=1&end=2&viewer=x'))
  assert.ok(!q.startsStream('GET', '/items/12'))
  assert.ok(!q.startsStream('POST', '/items/12/progress'))
  assert.ok(!q.startsStream('GET', '/tv/lib-3/leave'))
})

test('stream information says what plays and how', () => {
  const gpu = { encoder: 'h264_nvenc', gpuEncode: true, gpuDecode: true, gpuToneMap: false }
  const lines = q.streamLines('transcode', { width: 1280, height: 720, videoCodec: 'h264', audioCodec: 'aac', audioChannels: 2, audioKbps: 192, kbps: 3808, encoder: 'h264_nvenc', work: gpu })
  assert.deepEqual(lines, ['1280×720 (720p)', '3.8 Mbps at most', 'H.264 · AAC stereo 192 kbps', 'Converting', 'GPU: the graphics chip decodes, resizes and encodes (NVIDIA NVENC)'])
  assert.equal(q.workLine({ ...gpu, gpuDecode: false }), 'GPU: the graphics chip encodes (NVIDIA NVENC); the processor decodes')
  assert.equal(q.workLine({ ...gpu, gpuToneMap: true }), 'GPU: the graphics chip decodes, resizes, maps HDR and encodes (NVIDIA NVENC)')
  assert.equal(q.workLine({ encoder: 'libx264', gpuEncode: false, gpuDecode: false, gpuToneMap: false }), 'CPU: the processor converts it (x264)')
  const direct = q.streamLines('direct', { width: 1920, height: 1080, videoCodec: 'hevc', audioCodec: 'eac3', audioChannels: 6, kbps: 9000 })
  assert.deepEqual(direct, ['1920×1080 (1080p)', '9 Mbps', 'HEVC · Dolby Digital Plus 5.1', 'Direct play'])
  // What the player sees wins over what was planned.
  assert.equal(q.streamLines('transcode', { width: 1280, height: 720 }, { width: 960, height: 540 })[0], '960×540 (540p)')
})
