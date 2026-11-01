// Moves music up or down in key without changing its speed. A delay line with two reading
// points that drift apart, each fading in and out so they never click: the classic way, and
// light enough for a phone or a cheap TV. It can warble a little on sustained notes at large
// shifts, which is why the player keeps to six semitones and says so.
const GRAIN = 2048 // about 46 ms at 44.1 kHz
const SIZE = GRAIN * 4

class WmsPitch extends AudioWorkletProcessor {
  static get parameterDescriptors() {
    return [{ name: 'ratio', defaultValue: 1, minValue: 0.5, maxValue: 2, automationRate: 'k-rate' }]
  }

  constructor() {
    super()
    this.lines = [new Float32Array(SIZE), new Float32Array(SIZE)]
    this.write = 0
    this.phase = 0
  }

  process(inputs, outputs, parameters) {
    const input = inputs[0]
    const output = outputs[0]
    const ratio = parameters.ratio[0]
    if (!output.length) return true
    const n = output[0].length
    if (!input.length) {
      for (const ch of output) ch.fill(0)
      return true
    }
    const shift = 1 - ratio // how fast the reading points drift, in samples per sample
    let write = this.write
    let phase = this.phase
    for (let i = 0; i < n; i++) {
      const a = phase
      const b = (phase + GRAIN / 2) % GRAIN
      // Each point fades as it reaches the end of its grain: raised cosine, summing to one.
      const ga = 0.5 - 0.5 * Math.cos((2 * Math.PI * a) / GRAIN)
      const gb = 0.5 - 0.5 * Math.cos((2 * Math.PI * b) / GRAIN)
      for (let c = 0; c < output.length; c++) {
        const line = this.lines[Math.min(c, 1)]
        const from = input[Math.min(c, input.length - 1)]
        line[write] = from[i]
        output[c][i] = ratio === 1 ? from[i] : read(line, write - a - 1) * ga + read(line, write - b - 1) * gb
      }
      write = (write + 1) % SIZE
      phase += shift
      if (phase >= GRAIN) phase -= GRAIN
      else if (phase < 0) phase += GRAIN
    }
    this.write = write
    this.phase = phase
    return true
  }
}

function read(line, at) {
  let p = at % SIZE
  if (p < 0) p += SIZE
  const i = Math.floor(p)
  const f = p - i
  return line[i] * (1 - f) + line[(i + 1) % SIZE] * f
}

registerProcessor('wms-pitch', WmsPitch)
