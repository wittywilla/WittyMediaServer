// What the analyzer says about a known piece of generated music, at fixed points. The web
// test compares with this file, and so does the desktop app's (qt/tests), so the two copies
// of the analysis can't drift apart. After a deliberate change: node tests/golden.mjs write
import { writeFileSync } from 'node:fs'
import { Analyzer, FFT_SIZE, SPECTRUM_BINS } from '../analysis.ts'
import { fill, song } from './synth.mjs'

export function goldenRun() {
  const a = new Analyzer()
  const left = new Float32Array(FFT_SIZE), right = new Float32Array(FFT_SIZE)
  const out = []
  const round = (v) => Math.round(v * 1e3) / 1e3
  for (let frame = 0; frame < 240; frame++) {
    fill(song, frame / 60, 48000, left, right)
    a.process(left, right, 48000, 1 / 60)
    if (frame % 30 === 29) {
      const f = a.frame, t = f.texture
      out.push({
        frame, bass: round(f.bass), mid: round(f.mid), treble: round(f.treble), level: round(f.level), beat: round(f.beat), beatCount: f.beatCount, historyRow: f.historyRow,
        spectrum: [8, 64, 128, 256, 384, 500].map((i) => t[i]), left: [0, 100, 300, 500].map((i) => t[SPECTRUM_BINS + i]), right: [0, 100, 300, 500].map((i) => t[2 * SPECTRUM_BINS + i]), peaks: [64, 256].map((i) => t[3 * SPECTRUM_BINS + i]),
      })
    }
  }
  return out
}

if (process.argv[2] === 'write') {
  const rows = goldenRun().map((r) => '  ' + JSON.stringify(r)).join(',\n')
  writeFileSync(new URL('./fixtures/analysis-golden.json', import.meta.url), `[\n${rows}\n]\n`)
}
