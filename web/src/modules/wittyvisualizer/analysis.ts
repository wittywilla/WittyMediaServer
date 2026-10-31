// Listening: turns the last moment of sound into what effects draw from. Pure sums with
// no browser in them, so it runs under a test, and qt/wvanalysis.cpp is the same thing in
// C++ (tests/fixtures/analysis-golden.json keeps the two honest).
//
// It takes plain samples rather than the browser's own frequency data because that's what
// the desktop app has too, and it makes both sides agree on what "bass" means.

export const FFT_SIZE = 2048
export const SPECTRUM_BINS = 512
export const WAVE_POINTS = 512
export const HISTORY_ROWS = 64
// Spectrum, left, right, peaks, then the history.
export const AUDIO_ROWS = 4 + HISTORY_ROWS

const LOW_HZ = 40, HIGH_HZ = 16000
// Faster than this isn't heard as a beat, and it would only feed the flash guard.
export const MIN_BEAT_GAP = 0.24

export type AudioFrame = {
  texture: Uint8Array // SPECTRUM_BINS wide, AUDIO_ROWS high, one byte each
  historyRow: number // where the newest history row is, 0 to HISTORY_ROWS - 1
  bass: number; mid: number; treble: number; level: number // 0 to 1
  beat: number // 1 at a beat, falling away
  beatCount: number
  beatAges: [number, number, number, number] // seconds since the last four, newest first
  isSilent: boolean
}

// The transform: in place, radix 2. Allocated once.
export class Fft {
  readonly n: number
  private cos: Float64Array
  private sin: Float64Array
  private rev: Uint32Array
  constructor(n: number) {
    this.n = n
    this.cos = new Float64Array(n / 2)
    this.sin = new Float64Array(n / 2)
    for (let i = 0; i < n / 2; i++) { this.cos[i] = Math.cos((2 * Math.PI * i) / n); this.sin[i] = Math.sin((2 * Math.PI * i) / n) }
    const bits = Math.log2(n)
    this.rev = new Uint32Array(n)
    for (let i = 0; i < n; i++) {
      let r = 0
      for (let b = 0; b < bits; b++) if (i & (1 << b)) r |= 1 << (bits - 1 - b)
      this.rev[i] = r
    }
  }
  run(re: Float64Array, im: Float64Array) {
    const n = this.n
    for (let i = 0; i < n; i++) {
      const j = this.rev[i]
      if (j > i) { let t = re[i]; re[i] = re[j]; re[j] = t; t = im[i]; im[i] = im[j]; im[j] = t }
    }
    for (let size = 2; size <= n; size *= 2) {
      const half = size / 2, step = n / size
      for (let start = 0; start < n; start += size) {
        for (let k = 0; k < half; k++) {
          const w = k * step, a = start + k, b = a + half
          const tr = re[b] * this.cos[w] + im[b] * this.sin[w]
          const ti = im[b] * this.cos[w] - re[b] * this.sin[w]
          re[b] = re[a] - tr; im[b] = im[a] - ti
          re[a] += tr; im[a] += ti
        }
      }
    }
  }
}

const clamp01 = (v: number) => (v < 0 ? 0 : v > 1 ? 1 : v)

// One band's loudness against its own recent peak, so a quiet recording still moves
// everything, and silence stays still.
class Band {
  value = 0
  private peak = 1e-4
  update(power: number, dt: number, sensitivity: number) {
    const amp = Math.sqrt(power)
    this.peak = Math.max(amp, this.peak * Math.exp(-dt / 6), 0.0015)
    const target = clamp01((amp / this.peak) * sensitivity)
    const tau = target > this.value ? 0.02 : 0.2
    this.value += (target - this.value) * (1 - Math.exp(-dt / tau))
  }
}

export class Analyzer {
  private fft = new Fft(FFT_SIZE)
  private window = new Float64Array(FFT_SIZE)
  private re = new Float64Array(FFT_SIZE)
  private im = new Float64Array(FFT_SIZE)
  private mono = new Float64Array(FFT_SIZE)
  private mags = new Float64Array(FFT_SIZE / 2)
  private smooth = new Float64Array(SPECTRUM_BINS)
  private peaks = new Float64Array(SPECTRUM_BINS)
  private bandBass = new Band()
  private bandMid = new Band()
  private bandTreble = new Band()
  private bandLevel = new Band()
  private lows: Float64Array // the novelty of the last second and a half, as (time, value)
  private lowsAt: Float64Array
  private lowsHead = 0
  private lastLow = 0
  private lastLowInput = 0
  private clock = 0
  private lastBeat = -10
  private specPeak = 1e-3
  private binLo = new Float64Array(SPECTRUM_BINS)
  private binHi = new Float64Array(SPECTRUM_BINS)
  private rate = 0
  private beats = [60, 60, 60, 60]
  readonly frame: AudioFrame

  // How much the music moves things (0.25 to 2), and how easily a beat is called.
  sensitivity = 1
  // Longer between beats, in calm mode.
  minBeatGap = MIN_BEAT_GAP

  constructor() {
    for (let i = 0; i < FFT_SIZE; i++) this.window[i] = 0.5 - 0.5 * Math.cos((2 * Math.PI * i) / (FFT_SIZE - 1))
    this.lows = new Float64Array(128)
    this.lowsAt = new Float64Array(128).fill(-100)
    this.frame = {
      texture: new Uint8Array(SPECTRUM_BINS * AUDIO_ROWS),
      historyRow: 0,
      bass: 0, mid: 0, treble: 0, level: 0, beat: 0, beatCount: 0,
      beatAges: [60, 60, 60, 60],
      isSilent: true,
    }
  }

  private setRate(rate: number) {
    this.rate = rate
    const hzPerBin = rate / FFT_SIZE
    const top = Math.min(HIGH_HZ, rate * 0.48)
    for (let i = 0; i < SPECTRUM_BINS; i++) {
      this.binLo[i] = (LOW_HZ * Math.pow(top / LOW_HZ, i / SPECTRUM_BINS)) / hzPerBin
      this.binHi[i] = (LOW_HZ * Math.pow(top / LOW_HZ, (i + 1) / SPECTRUM_BINS)) / hzPerBin
    }
  }

  // left and right hold at least FFT_SIZE samples, the newest last.
  process(left: Float32Array, right: Float32Array, rate: number, dtSec: number) {
    if (rate !== this.rate) this.setRate(rate)
    const dt = Math.max(0.001, Math.min(0.25, dtSec))
    this.clock += dt
    const f = this.frame
    const off = left.length - FFT_SIZE

    let energy = 0
    for (let i = 0; i < FFT_SIZE; i++) {
      const m = 0.5 * (left[off + i] + right[off + i])
      this.mono[i] = m
      energy += m * m
      this.re[i] = m * this.window[i]
      this.im[i] = 0
    }
    this.fft.run(this.re, this.im)
    // Windowing keeps half the power; this scales a full-scale sine to about 1.
    const norm = 4 / FFT_SIZE
    for (let i = 0; i < FFT_SIZE / 2; i++) this.mags[i] = Math.hypot(this.re[i], this.im[i]) * norm

    f.isSilent = energy / FFT_SIZE < 1e-8

    this.bands(dt)
    this.spectrumRow(dt, f.isSilent)
    this.waves(left, right, off)
    this.detectBeat(dt)

    f.bass = this.bandBass.value; f.mid = this.bandMid.value; f.treble = this.bandTreble.value; f.level = this.bandLevel.value
    f.beat *= Math.exp(-dt / 0.16)
    for (let i = 0; i < 4; i++) { this.beats[i] += dt; f.beatAges[i] = this.beats[i] }
  }

  private bandPower(fromHz: number, toHz: number) {
    const per = this.rate / FFT_SIZE
    const a = Math.max(1, Math.round(fromHz / per)), b = Math.min(FFT_SIZE / 2 - 1, Math.round(toHz / per))
    let p = 0
    for (let i = a; i <= b; i++) p += this.mags[i] * this.mags[i]
    return p
  }

  private bands(dt: number) {
    const s = this.sensitivity
    const bass = this.bandPower(20, 180), mid = this.bandPower(180, 2000), treble = this.bandPower(2000, 16000)
    this.bandBass.update(bass, dt, s)
    this.bandMid.update(mid, dt, s)
    this.bandTreble.update(treble, dt, s)
    this.bandLevel.update(bass + mid + treble, dt, s)
    this.lastLowInput = bass
  }

  private spectrumRow(dt: number, silent: boolean) {
    const f = this.frame, row = f.texture, w = SPECTRUM_BINS
    const fall = 1 - Math.exp(-dt / 0.14)
    // The loudest bin sets the scale, slowly, so that bars use the height they have.
    let top = 0
    for (let i = 0; i < w; i++) {
      const lo = this.binLo[i], hi = this.binHi[i]
      let v = 0
      if (hi - lo < 1) {
        // Below the transform's resolution: read between its bins.
        const c = (lo + hi) * 0.5, i0 = Math.floor(c), t = c - i0
        v = this.mags[i0] * (1 - t) + this.mags[Math.min(i0 + 1, FFT_SIZE / 2 - 1)] * t
      } else {
        for (let k = Math.floor(lo); k < Math.min(Math.ceil(hi), FFT_SIZE / 2); k++) if (this.mags[k] > v) v = this.mags[k]
      }
      // Music has less energy up high; a tilt of 3 dB an octave evens that out.
      const octave = i / w * Math.log2(HIGH_HZ / LOW_HZ)
      v *= Math.pow(2, octave * 0.5)
      if (v > top) top = v
      this.re[i] = v
    }
    this.specPeak = Math.max(top, this.specPeak * Math.exp(-dt / 8), 0.002)
    const rangeDb = 52
    const h = (f.historyRow + 1) % HISTORY_ROWS
    f.historyRow = h
    const histAt = (4 + h) * w
    for (let i = 0; i < w; i++) {
      const db = silent ? -120 : 20 * Math.log10(Math.max(this.re[i], 1e-9) / this.specPeak)
      const target = clamp01(1 + db / rangeDb) * Math.min(1.15, this.sensitivity)
      let s = this.smooth[i]
      s = target > s ? target : s + (target - s) * fall
      this.smooth[i] = s
      this.peaks[i] = Math.max(s, this.peaks[i] - 0.35 * dt)
      const byte = Math.round(clamp01(s) * 255)
      row[i] = byte
      row[3 * w + i] = Math.round(clamp01(this.peaks[i]) * 255)
      row[histAt + i] = byte
    }
  }

  // The waves are lined up on a rising edge, so a steady note stands still on a scope.
  private waves(left: Float32Array, right: Float32Array, off: number) {
    const row = this.frame.texture, w = SPECTRUM_BINS
    const span = WAVE_POINTS * 2 // about 23 ms
    let start = FFT_SIZE - span
    for (let i = FFT_SIZE - span; i > 1 && i > FFT_SIZE - span - 600; i--) {
      if (this.mono[i - 1] <= 0 && this.mono[i] > 0) { start = i; break }
    }
    for (let i = 0; i < WAVE_POINTS; i++) {
      const at = off + start + i * 2
      row[w + i] = Math.round(clamp01(left[at] * 0.5 + 0.5) * 255)
      row[2 * w + i] = Math.round(clamp01(right[at] * 0.5 + 0.5) * 255)
    }
  }

  // A beat is a jump in the low end's loudness that stands out from its own recent jumps.
  private detectBeat(dt: number) {
    const low = Math.log10(this.lastLowInput + 1e-6) * 10 // dB
    const jump = Math.max(0, low - this.lastLow)
    this.lastLow += (low - this.lastLow) * (1 - Math.exp(-dt / 0.03))
    const lows = this.lows, at = this.lowsAt
    let n = 0, sum = 0, sq = 0
    for (let i = 0; i < lows.length; i++) {
      if (this.clock - at[i] > 1.5) continue
      n++; sum += lows[i]; sq += lows[i] * lows[i]
    }
    const mean = n ? sum / n : 0
    const spread = n ? Math.sqrt(Math.max(0, sq / n - mean * mean)) : 0
    lows[this.lowsHead] = jump
    at[this.lowsHead] = this.clock
    this.lowsHead = (this.lowsHead + 1) % lows.length
    const threshold = mean + (1.8 / this.sensitivity) * spread + 1.5 / this.sensitivity
    const loudEnough = low > -48
    if (jump > threshold && loudEnough && n > 6 && this.clock - this.lastBeat >= this.minBeatGap) {
      this.lastBeat = this.clock
      const f = this.frame
      f.beat = 1
      f.beatCount++
      this.beats.pop()
      this.beats.unshift(0)
    }
  }
}
