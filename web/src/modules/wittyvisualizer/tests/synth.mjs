// Generated music for tests: nothing here comes from a recording.

// A kick on every beat, a bass line, a chord that moves, and hats, with a wide stereo image.
export function song(t, bpm = 120) {
  const period = 60 / bpm
  const beat = t / period
  const since = Math.max(0, t) % period // seconds since the last kick
  const kick = Math.sin(2 * Math.PI * (48 * since + (110 / 28) * (1 - Math.exp(-since * 28)))) * Math.exp(-since * 11) * 0.9
  const bar = Math.floor(beat / 4) % 4
  const root = [55, 65.4, 49, 73.4][(bar + 4) % 4]
  const bass = Math.sin(2 * Math.PI * root * t) * 0.2 * (0.6 + 0.4 * Math.exp(-((beat * 2) % 1) * 3))
  const chord = (Math.sin(2 * Math.PI * root * 4 * t) + Math.sin(2 * Math.PI * root * 5 * t + 1) + Math.sin(2 * Math.PI * root * 6 * t + 2)) * 0.07
  const lead = Math.sin(2 * Math.PI * root * 12 * t + 3 * Math.sin(2 * Math.PI * 3 * t)) * 0.05 * (0.5 + 0.5 * Math.sin((2 * Math.PI * beat) / 8))
  const hat = (Math.sin(t * 91345.7) * Math.sin(t * 73123.3) + Math.sin(t * 40123.9) * 0.5) * Math.exp(-((beat * 2) % 1) * 18) * 0.13
  // A wash of notes across the range, each swelling at its own pace, as real music has.
  let wash = 0, washR = 0
  for (let i = 0; i < 18; i++) {
    const hz = 90 * Math.pow(1.3, i)
    const swell = 0.5 + 0.5 * Math.sin(t * (0.9 + i * 0.37) + i * 2.1)
    const v = Math.sin(2 * Math.PI * hz * t + i) * swell * swell * 0.11 / (1 + i * 0.22)
    wash += v
    washR += i % 2 ? v : -v * 0.6
  }
  // And a little hiss, which real recordings have between the notes.
  const hiss = (x) => { const v = Math.sin(x * 12345.678) * 43758.5453; return (v - Math.floor(v) - 0.5) * 0.05 }
  return [kick + bass + chord + lead + hat * 0.7 + wash + hiss(t), kick + bass + chord * 0.8 - lead + hat + washR + hiss(t + 7)]
}

export const silence = () => [0, 0]
export const tone = (hz, amp = 0.5) => (t) => { const v = Math.sin(2 * Math.PI * hz * t) * amp; return [v, v] }

// Fills two buffers with what a host would hand the analyser at time t.
export function fill(gen, t, rate, left, right) {
  const n = left.length
  for (let i = 0; i < n; i++) {
    const [l, r] = gen(t - (n - i) / rate)
    left[i] = l
    right[i] = r
  }
}
