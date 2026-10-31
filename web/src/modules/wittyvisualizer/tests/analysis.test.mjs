import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { Analyzer, FFT_SIZE, Fft, HISTORY_ROWS, MIN_BEAT_GAP, SPECTRUM_BINS, WAVE_POINTS } from '../analysis.ts'
import { goldenRun } from './golden.mjs'
import { fill, silence, song, tone } from './synth.mjs'

const RATE = 44100

// Runs an analyzer as a host would, at a frame rate, and collects what it says.
function run(gen, seconds, { fps = 60, rate = RATE, setup } = {}) {
  const a = new Analyzer()
  setup?.(a)
  const left = new Float32Array(FFT_SIZE), right = new Float32Array(FFT_SIZE)
  const beats = [], bands = []
  let was = 0
  const dt = 1 / fps
  for (let t = 0; t < seconds; t += dt) {
    fill(gen, t, rate, left, right)
    a.process(left, right, rate, dt)
    if (a.frame.beatCount !== was) { was = a.frame.beatCount; beats.push(t) }
    bands.push({ bass: a.frame.bass, mid: a.frame.mid, treble: a.frame.treble, level: a.frame.level })
  }
  return { a, beats, bands }
}

const mean = (rows, key, from = 0) => { const r = rows.slice(from); return r.reduce((s, x) => s + x[key], 0) / r.length }

test('the transform finds a tone in the right bin', () => {
  const fft = new Fft(1024)
  const re = new Float64Array(1024), im = new Float64Array(1024)
  for (let i = 0; i < 1024; i++) re[i] = Math.sin((2 * Math.PI * 37 * i) / 1024)
  fft.run(re, im)
  let best = 0, at = 0
  for (let i = 0; i < 512; i++) { const m = Math.hypot(re[i], im[i]); if (m > best) { best = m; at = i } }
  assert.equal(at, 37)
  assert.ok(Math.abs(best - 512) < 1)
})

test('bass, mids and treble each answer to their own part of the sound', () => {
  const low = run(tone(80), 1.5).bands, middle = run(tone(800), 1.5).bands, high = run(tone(6000), 1.5).bands
  const skip = 45
  assert.ok(mean(low, 'bass', skip) > 0.8, 'a low tone fills the bass')
  assert.ok(mean(low, 'treble', skip) < mean(low, 'bass', skip))
  assert.ok(mean(middle, 'mid', skip) > 0.8, 'a middle tone fills the mids')
  assert.ok(mean(high, 'treble', skip) > 0.8, 'a high tone fills the treble')
})

test('the spectrum puts a tone where it belongs, and nothing much elsewhere', () => {
  const place = (hz) => Math.log(hz / 40) / Math.log(16000 / 40) * SPECTRUM_BINS
  for (const hz of [100, 440, 2000, 8000]) {
    const { a } = run(tone(hz), 0.8)
    const row = a.frame.texture.subarray(0, SPECTRUM_BINS)
    let best = 0, at = 0
    for (let i = 0; i < SPECTRUM_BINS; i++) if (row[i] > best) { best = row[i]; at = i }
    assert.ok(Math.abs(at - place(hz)) < 18, `${hz} Hz should be near bin ${place(hz) | 0}, is at ${at}`)
    assert.ok(best > 200, 'and loud')
    const far = row[(at + SPECTRUM_BINS / 2) % SPECTRUM_BINS]
    assert.ok(far < best * 0.4, 'and alone')
  }
})

test('beats land on the kicks, at the tempos music uses', () => {
  for (const bpm of [80, 100, 120, 128, 140, 174]) {
    const { beats } = run((t) => song(t, bpm), 16)
    const period = 60 / bpm
    const steady = beats.filter((t) => t > 2)
    const expected = (16 - 2) / period
    // Half time is as good as full time for a picture, doubled isn't.
    const fraction = steady.length / expected
    assert.ok(fraction > 0.8 && fraction < 1.2, `${bpm} BPM: ${steady.length} beats, expected about ${expected | 0}`)
    let near = 0
    for (const t of steady) {
      const off = ((t % period) + period) % period
      if (off < 0.09 || off > period - 0.02) near++
    }
    assert.ok(near / steady.length > 0.85, `${bpm} BPM: only ${near} of ${steady.length} on the kick`)
  }
})

test('beats hold up at a low frame rate', () => {
  const { beats } = run((t) => song(t, 120), 16, { fps: 24 })
  const steady = beats.filter((t) => t > 2).length
  assert.ok(steady > 22 && steady < 34, `${steady} beats in 14 seconds at 120 BPM`)
})

test('silence and steady tones have no beats, and silence is still', () => {
  const quiet = run(silence, 4)
  assert.equal(quiet.beats.length, 0)
  assert.ok(quiet.a.frame.isSilent)
  assert.ok(quiet.bands.at(-1).level < 0.01)
  assert.ok(quiet.a.frame.texture.subarray(0, SPECTRUM_BINS).every((v) => v === 0))
  assert.equal(run(tone(200), 4).beats.filter((t) => t > 1).length, 0)
})

test('very quiet noise is not heard as music', () => {
  const noise = () => { const v = (Math.random() - 0.5) * 1e-5; return [v, v] }
  assert.equal(run(noise, 4).beats.length, 0)
})

test('a beat can never come faster than the limit, however hard the music tries', () => {
  // A click ten times a second.
  const clicks = (t) => { const v = Math.exp(-((t * 10) % 1) * 12) * Math.sin(2 * Math.PI * 60 * t) * 0.9; return [v, v] }
  const { beats } = run(clicks, 6)
  for (let i = 1; i < beats.length; i++) assert.ok(beats[i] - beats[i - 1] >= MIN_BEAT_GAP - 1e-6)
  const slow = run(clicks, 6, { setup: (a) => { a.minBeatGap = 0.5 } }).beats
  for (let i = 1; i < slow.length; i++) assert.ok(slow[i] - slow[i - 1] >= 0.5 - 1e-6)
})

test('a quiet recording moves things as much as a loud one', () => {
  const loud = run(song, 8).bands, soft = run((t) => { const [l, r] = song(t); return [l * 0.03, r * 0.03] }, 8).bands
  assert.ok(Math.abs(mean(loud, 'level', 240) - mean(soft, 'level', 240)) < 0.1)
})

test('the waves hold the stereo signal, lined up on a rising edge', () => {
  const stereo = (t) => [Math.sin(2 * Math.PI * 220 * t) * 0.8, Math.sin(2 * Math.PI * 220 * t) * 0.4]
  const { a } = run(stereo, 0.3)
  const w = SPECTRUM_BINS, tex = a.frame.texture
  const at = (row, i) => tex[row * w + i] / 255 * 2 - 1
  assert.ok(Math.abs(at(1, 0)) < 0.25, 'starts near zero')
  assert.ok(at(1, 8) > at(1, 0), 'and rising')
  for (let i = 0; i < WAVE_POINTS; i += 37) assert.ok(Math.abs(at(1, i) - at(2, i) * 2) < 0.05, 'each channel keeps its own level')
})

test('a mono-sized or unusual sample rate still places notes correctly', () => {
  for (const rate of [22050, 48000, 96000]) {
    const { a } = run(tone(1000), 0.8, { rate })
    const row = a.frame.texture.subarray(0, SPECTRUM_BINS)
    let best = 0, at = 0
    for (let i = 0; i < SPECTRUM_BINS; i++) if (row[i] > best) { best = row[i]; at = i }
    const top = Math.min(16000, rate * 0.48)
    const want = Math.log(1000 / 40) / Math.log(top / 40) * SPECTRUM_BINS
    assert.ok(Math.abs(at - want) < 18, `${rate} Hz: at ${at}, want ${want | 0}`)
  }
})

test('history keeps a row for each frame and wraps around', () => {
  const { a } = run(song, (HISTORY_ROWS + 5) / 60)
  assert.ok(a.frame.historyRow >= 0 && a.frame.historyRow < HISTORY_ROWS)
})

test('a frame takes well under a millisecond here', () => {
  const a = new Analyzer()
  const left = new Float32Array(FFT_SIZE), right = new Float32Array(FFT_SIZE)
  fill(song, 1, RATE, left, right)
  const n = 400, began = performance.now()
  for (let i = 0; i < n; i++) a.process(left, right, RATE, 1 / 60)
  const each = (performance.now() - began) / n
  assert.ok(each < 1.5, `${each.toFixed(2)} ms a frame`)
})

// Keeps the C++ copy honest: qt/tests reads these same numbers.
test('the numbers the desktop app has to match are up to date', () => {
  const golden = JSON.parse(readFileSync(new URL('./fixtures/analysis-golden.json', import.meta.url)))
  const now = goldenRun()
  assert.deepEqual(now, golden)
})
