// Writes generated music as a WAV file, for trying the visualizer on a real server:
//   node tests/make-audio.mjs out.wav [seconds] [bpm] [strobe]
// "strobe" makes it as hostile as music can be: full-range bursts ten times a second.
import { closeSync, openSync, writeSync } from 'node:fs'
import { song } from './synth.mjs'

const [out, seconds = '45', bpm = '120', mode] = process.argv.slice(2)
const rate = 44100, frames = Math.round(Number(seconds) * rate)
const gen = mode === 'strobe'
  ? (t) => { const on = (t * 10) % 1 < 0.5 ? 0.9 : 0; const n = Math.sin(t * 12345.678) * 43758.5453; const v = (n - Math.floor(n) - 0.5) * 2 * on + Math.sin(t * 2 * Math.PI * 60) * on * 0.8; return [v * 0.6, v * 0.6] }
  : (t) => song(t, Number(bpm))

const head = Buffer.alloc(44)
head.write('RIFF', 0); head.writeUInt32LE(36 + frames * 4, 4); head.write('WAVEfmt ', 8); head.writeUInt32LE(16, 16)
head.writeUInt16LE(1, 20); head.writeUInt16LE(2, 22); head.writeUInt32LE(rate, 24); head.writeUInt32LE(rate * 4, 28)
head.writeUInt16LE(4, 32); head.writeUInt16LE(16, 34); head.write('data', 36); head.writeUInt32LE(frames * 4, 40)
const fd = openSync(out, 'w')
writeSync(fd, head)
const chunk = Buffer.alloc(rate * 4)
for (let at = 0; at < frames; at += rate) {
  const n = Math.min(rate, frames - at)
  for (let i = 0; i < n; i++) {
    const [l, r] = gen((at + i) / rate)
    chunk.writeInt16LE(Math.max(-32767, Math.min(32767, Math.round(l * 26000))), i * 4)
    chunk.writeInt16LE(Math.max(-32767, Math.min(32767, Math.round(r * 26000))), i * 4 + 2)
  }
  writeSync(fd, chunk, 0, n * 4)
}
closeSync(fd)
