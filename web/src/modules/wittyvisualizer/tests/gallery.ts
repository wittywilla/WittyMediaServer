// A page for looking at effects without a server: tests/browser.mjs drives it. Every effect
// runs from generated music, at a fixed step, so the same input gives the same picture.
import { Analyzer, FFT_SIZE } from '../analysis.ts'
import { Palettes } from '../engine.ts'
import { GlRenderer, createGl } from '../gl.ts'
import { GUARD, GUARD_CALM } from '../safety.ts'
import { BUILTIN_EFFECTS, PALETTES } from '../shaders.ts'
// @ts-expect-error plain JavaScript, shared with the unit tests
import { fill, song } from './synth.mjs'

const canvas = document.getElementById('gl') as HTMLCanvasElement
const gl = createGl(canvas)!
const r = new GlRenderer(gl)
const left = new Float32Array(FFT_SIZE), right = new Float32Array(FFT_SIZE)

const ready = async (id: string) => {
  const e = BUILTIN_EFFECTS.find((x) => x.id === id)!
  for (;;) {
    const p = r.prepare(e)
    if (p) { if (!p.ok) throw new Error(`${id}: ${p.error}`); return }
    await new Promise((d) => setTimeout(d, 10))
  }
}

type Options = { seconds?: number; fps?: number; calm?: boolean; gen?: (t: number) => number[]; intensity?: number; onFrame?: () => void; rate?: number }
function run(id: string, b: string | null, o: Options) {
  const fps = o.fps ?? 60, secs = o.seconds ?? 6, gen = o.gen ?? song, calm = !!o.calm
  const a = new Analyzer()
  const pal = new Palettes(PALETTES, ((s) => () => (s = (s * 48271) % 2147483647) / 2147483647)(1234))
  let time = 0, flow = 0
  const dt = 1 / fps
  for (let f = 0; f < secs * fps; f++) {
    fill(gen, f * dt, 44100, left, right)
    a.process(left, right, 44100, dt)
    time += dt
    flow += dt * (0.3 + 0.75 * a.frame.level + 0.5 * a.frame.beat)
    pal.step(dt, a.frame.beatCount, calm)
    r.draw({ audio: a.frame, time, flow, dt, intensity: o.intensity ?? 1, calm, palette: pal.out, guard: calm ? GUARD_CALM : GUARD, a: id, b, mix: b ? Math.min(1, f / (secs * fps)) : 0, mixStyle: 0 })
    o.onFrame?.()
  }
}

const api = {
  ids: BUILTIN_EFFECTS.map((e) => e.id),
  gpu: r.label,
  software: r.software,
  // All compile on this device?
  async compileAll() {
    const out: Record<string, string> = {}
    for (const e of BUILTIN_EFFECTS) { try { const t = performance.now(); await ready(e.id); out[e.id] = `ok ${Math.round(performance.now() - t)} ms` } catch (err) { out[e.id] = String(err) } }
    return out
  },
  // A sheet of every effect, as a PNG data address.
  async sheet(w: number, h: number, seconds: number, ids?: string[]) {
    const list = ids ?? api.ids
    const cols = Math.ceil(Math.sqrt(list.length * 1.2)), rows = Math.ceil(list.length / cols)
    const out = document.createElement('canvas')
    out.width = cols * w; out.height = rows * (h + 16)
    const c = out.getContext('2d')!
    c.fillStyle = '#222'; c.fillRect(0, 0, out.width, out.height)
    c.font = '12px sans-serif'
    r.setSize(w, h)
    for (let i = 0; i < list.length; i++) {
      await ready(list[i])
      run(list[i], null, { seconds })
      const x = (i % cols) * w, y = Math.floor(i / cols) * (h + 16)
      c.drawImage(canvas, x, y + 16)
      c.fillStyle = '#fff'; c.fillText(list[i], x + 4, y + 12)
      r.release(list[i])
    }
    return out.toDataURL('image/png')
  },
  // How long frames really take, drawn back to back and waited for.
  async speed(id: string, w: number, h: number, frames: number) {
    await ready(id)
    r.setSize(w, h)
    run(id, null, { seconds: 1 })
    const pixel = new Uint8Array(4)
    const a = new Analyzer()
    fill(song, 1, 44100, left, right)
    a.process(left, right, 44100, 1 / 60)
    const pal = new Palettes(PALETTES)
    const began = performance.now()
    for (let f = 0; f < frames; f++) {
      r.draw({ audio: a.frame, time: f / 60, flow: f / 60, dt: 1 / 60, intensity: 1, calm: false, palette: pal.out, guard: GUARD, a: id, b: null, mix: 0, mixStyle: 0 })
      gl.readPixels(0, 0, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, pixel)
    }
    const ms = (performance.now() - began) / frames
    r.release(id)
    return Math.round(ms * 10) / 10
  },
  // What a worst-case strobe of a picture does: a full-screen shader that swings with the
  // beat is the hardest thing an effect could do, so a recording of the screen is tested.
  async strobe(hz: number, calm: boolean) {
    const src = `/*wittyvisualizer {"format":1,"id":"strobe","name":"Strobe","feedback":false} */\nvec4 effect(vec2 uv) { return vec4(vec3(step(0.5, fract(time * ${hz.toFixed(3)}))), 1.0); }`
    const e = { id: 'strobe', name: 'Strobe', description: '', tags: [], cost: 1 as const, motion: 'lively' as const, feedback: false, quads: 0, source: src.slice(src.indexOf('*/') + 2), builtin: false }
    for (;;) { const p = r.prepare(e); if (p) { if (!p.ok) throw new Error(p.error); break } await new Promise((d) => setTimeout(d, 10)) }
    r.setSize(64, 36)
    const pixels = new Uint8Array(64 * 36 * 4)
    const lum = (v: number) => Math.pow(v / 255, 2.2)
    const values: number[] = [], times: number[] = []
    let n = 0
    run('strobe', null, { seconds: 10, calm, onFrame() {
      gl.readPixels(0, 0, 64, 36, gl.RGBA, gl.UNSIGNED_BYTE, pixels)
      let s = 0
      for (let i = 0; i < pixels.length; i += 4) s += 0.2126 * lum(pixels[i]) + 0.7152 * lum(pixels[i + 1]) + 0.0722 * lum(pixels[i + 2])
      values.push(s / (64 * 36)); times.push(n++ / 60)
    } })
    r.release('strobe')
    return { values, times }
  },
}
;(window as unknown as { gallery: typeof api }).gallery = api
document.title = 'ready'
