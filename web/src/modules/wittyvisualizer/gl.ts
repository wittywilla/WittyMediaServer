import { AUDIO_ROWS, SPECTRUM_BINS } from './analysis.ts'
import type { EffectFile } from './format.ts'
import { effectFrag, fullscreenVert, guardFrag, presentFrag, quadFrag, quadVert } from './shaders.ts'

// Draws with WebGL 2. Every effect draws into a texture of its own; the guard and present
// passes then turn those into the picture, so nothing reaches the screen without passing
// the flash guard.

export type DrawInput = {
  audio: { texture: Uint8Array; historyRow: number; bass: number; mid: number; treble: number; level: number; beat: number; beatCount: number; beatAges: number[] }
  time: number; flow: number; dt: number
  intensity: number; calm: boolean
  palette: { a: number[]; b: number[]; c: number[]; d: number[] }
  guard: { deadBand: number; rise: number }
  a: string
  b: string | null // the effect coming in
  mix: number
  mixStyle: number
}

export interface Renderer {
  readonly kind: 'webgl2' | 'canvas'
  readonly software: boolean
  readonly label: string
  // Starts compiling an effect without stopping the picture. Null while working.
  prepare(e: EffectFile): { ok: true } | { ok: false; error: string } | null
  supports(id: string): boolean
  setSize(w: number, h: number): void
  draw(f: DrawInput): void
  release(id: string): void
  dispose(): void
}

const SOFTWARE = /swiftshader|llvmpipe|softpipe|basic render|microsoft basic|warp\b|software/i

const GUARD_W = 8, GUARD_H = 5

type Prog = { program: WebGLProgram; shaders: WebGLShader[] }
type Pending = Prog & { started: number }
type Effect = { info: EffectFile; main: Prog; quad: Prog | null; seed: number }
type Target = { w: number; h: number; tex: WebGLTexture[]; fbo: WebGLFramebuffer[]; at: number }

export class GlRenderer implements Renderer {
  readonly kind = 'webgl2' as const
  readonly software: boolean
  readonly label: string
  private gl: WebGL2RenderingContext
  private parallel: { COMPLETION_STATUS_KHR: number } | null
  private vao: WebGLVertexArrayObject
  private ubo: WebGLBuffer
  private audio: WebGLTexture
  private uniforms = new Float32Array(44)
  private guardProg: Prog
  private presentProg: Prog
  private quadFragShader: WebGLShader
  private fullVert: WebGLShader
  private guardTex: WebGLTexture[] = []
  private guardFbo: WebGLFramebuffer[] = []
  private guardAt = 0
  private effects = new Map<string, Effect>()
  private pending = new Map<string, { info: EffectFile; main: Pending; quad: Pending | null }>()
  private targets = new Map<string, Target>()
  private w = 2
  private h = 2
  private historySent = -1
  private black: WebGLTexture

  constructor(gl: WebGL2RenderingContext) {
    this.gl = gl
    const dbg = gl.getExtension('WEBGL_debug_renderer_info')
    this.label = dbg ? String(gl.getParameter(dbg.UNMASKED_RENDERER_WEBGL)) : 'WebGL 2'
    this.software = SOFTWARE.test(this.label)
    this.parallel = gl.getExtension('KHR_parallel_shader_compile')
    gl.disable(gl.DEPTH_TEST)
    gl.disable(gl.CULL_FACE)
    gl.disable(gl.DITHER)
    this.vao = gl.createVertexArray()!
    gl.bindVertexArray(this.vao)
    this.ubo = gl.createBuffer()!
    gl.bindBuffer(gl.UNIFORM_BUFFER, this.ubo)
    gl.bufferData(gl.UNIFORM_BUFFER, this.uniforms.byteLength, gl.DYNAMIC_DRAW)
    gl.bindBufferBase(gl.UNIFORM_BUFFER, 0, this.ubo)
    gl.pixelStorei(gl.UNPACK_ALIGNMENT, 1)

    this.audio = gl.createTexture()!
    gl.activeTexture(gl.TEXTURE1)
    gl.bindTexture(gl.TEXTURE_2D, this.audio)
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.R8, SPECTRUM_BINS, AUDIO_ROWS, 0, gl.RED, gl.UNSIGNED_BYTE, null)
    this.filters(gl.LINEAR)

    this.black = gl.createTexture()!
    gl.activeTexture(gl.TEXTURE3)
    gl.bindTexture(gl.TEXTURE_2D, this.black)
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, 1, 1, 0, gl.RGBA, gl.UNSIGNED_BYTE, new Uint8Array([0, 0, 0, 255]))
    this.filters(gl.NEAREST)

    for (let i = 0; i < 2; i++) {
      const t = gl.createTexture()!
      gl.bindTexture(gl.TEXTURE_2D, t)
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, GUARD_W, GUARD_H, 0, gl.RGBA, gl.UNSIGNED_BYTE, null)
      this.filters(gl.LINEAR)
      const f = gl.createFramebuffer()!
      gl.bindFramebuffer(gl.FRAMEBUFFER, f)
      gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, t, 0)
      this.guardTex.push(t)
      this.guardFbo.push(f)
    }
    gl.bindFramebuffer(gl.FRAMEBUFFER, null)

    this.fullVert = this.shader(gl.VERTEX_SHADER, fullscreenVert())
    this.quadFragShader = this.shader(gl.FRAGMENT_SHADER, quadFrag())
    this.guardProg = this.link(this.fullVert, this.shader(gl.FRAGMENT_SHADER, guardFrag()))
    this.presentProg = this.link(this.fullVert, this.shader(gl.FRAGMENT_SHADER, presentFrag()))
    for (const p of [this.guardProg, this.presentProg]) {
      if (!gl.getProgramParameter(p.program, gl.LINK_STATUS)) throw new Error('the picture shaders would not compile: ' + gl.getProgramInfoLog(p.program))
      this.bindNames(p.program)
    }
  }

  private filters(f: number) {
    const gl = this.gl
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, f)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, f)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
  }

  private shader(type: number, source: string) {
    const gl = this.gl
    const s = gl.createShader(type)!
    gl.shaderSource(s, source)
    gl.compileShader(s)
    return s
  }

  private link(vs: WebGLShader, fs: WebGLShader): Prog {
    const gl = this.gl
    const program = gl.createProgram()!
    gl.attachShader(program, vs)
    gl.attachShader(program, fs)
    gl.linkProgram(program)
    return { program, shaders: [vs, fs] }
  }

  private bindNames(program: WebGLProgram) {
    const gl = this.gl
    gl.useProgram(program)
    const block = gl.getUniformBlockIndex(program, 'Frame')
    if (block !== gl.INVALID_INDEX) gl.uniformBlockBinding(program, block, 0)
    const set = (name: string, unit: number) => { const l = gl.getUniformLocation(program, name); if (l) gl.uniform1i(l, unit) }
    set('iAudio', 1); set('iPrev', 2); set('iA', 2); set('iB', 3); set('iGuard', 4)
  }

  supports() { return true }

  prepare(e: EffectFile): { ok: true } | { ok: false; error: string } | null {
    const gl = this.gl
    if (this.effects.has(e.id)) return { ok: true }
    let p = this.pending.get(e.id)
    if (!p) {
      const main = { ...this.link(this.fullVert, this.shader(gl.FRAGMENT_SHADER, effectFrag(e))), started: performance.now() }
      const quad = e.quads ? { ...this.link(this.shader(gl.VERTEX_SHADER, quadVert(e)), this.quadFragShader), started: main.started } : null
      p = { info: e, main, quad }
      this.pending.set(e.id, p)
    }
    const done = (q: Pending | null) => !q || !this.parallel || gl.getProgramParameter(q.program, this.parallel.COMPLETION_STATUS_KHR)
    if (!done(p.main) || !done(p.quad)) return null
    this.pending.delete(e.id)
    for (const q of [p.main, p.quad]) {
      if (q && !gl.getProgramParameter(q.program, gl.LINK_STATUS)) {
        const log = gl.getProgramInfoLog(q.program) || q.shaders.map((s) => gl.getShaderInfoLog(s)).join(' ')
        return { ok: false, error: log.slice(0, 600) || 'it would not compile' }
      }
    }
    this.bindNames(p.main.program)
    if (p.quad) this.bindNames(p.quad.program)
    this.effects.set(e.id, { info: e, main: p.main, quad: p.quad, seed: Math.random() })
    return { ok: true }
  }

  setSize(w: number, h: number) {
    w = Math.max(2, Math.round(w)); h = Math.max(2, Math.round(h))
    if (w === this.w && h === this.h) return
    this.w = w; this.h = h
    const canvas = this.gl.canvas as HTMLCanvasElement
    canvas.width = w; canvas.height = h
  }

  private target(id: string, feedback: boolean): Target {
    const gl = this.gl
    let t = this.targets.get(id)
    if (t && t.w === this.w && t.h === this.h) return t
    const old = t
    const n = feedback ? 2 : 1
    t = { w: this.w, h: this.h, tex: [], fbo: [], at: 0 }
    gl.activeTexture(gl.TEXTURE5)
    for (let i = 0; i < n; i++) {
      const tx = gl.createTexture()!
      gl.bindTexture(gl.TEXTURE_2D, tx)
      gl.texStorage2D(gl.TEXTURE_2D, 1, gl.RGBA8, this.w, this.h)
      this.filters(gl.LINEAR)
      const f = gl.createFramebuffer()!
      gl.bindFramebuffer(gl.FRAMEBUFFER, f)
      gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, tx, 0)
      gl.clearColor(0, 0, 0, 0)
      gl.clear(gl.COLOR_BUFFER_BIT)
      t.tex.push(tx); t.fbo.push(f)
    }
    if (old) {
      // Keep the picture a feedback effect is building on through a change of resolution.
      gl.bindFramebuffer(gl.READ_FRAMEBUFFER, old.fbo[old.at])
      gl.bindFramebuffer(gl.DRAW_FRAMEBUFFER, t.fbo[0])
      gl.blitFramebuffer(0, 0, old.w, old.h, 0, 0, t.w, t.h, gl.COLOR_BUFFER_BIT, gl.LINEAR)
      this.freeTarget(old)
    }
    this.targets.set(id, t)
    return t
  }

  private freeTarget(t: Target) {
    for (const x of t.tex) this.gl.deleteTexture(x)
    for (const x of t.fbo) this.gl.deleteFramebuffer(x)
  }

  private sendUniforms(f: DrawInput, seed: number) {
    const u = this.uniforms, a = f.audio, p = f.palette
    u[0] = f.time; u[1] = f.flow; u[2] = f.dt; u[3] = a.beatCount
    u[4] = a.bass; u[5] = a.mid; u[6] = a.treble; u[7] = a.level
    u[8] = a.beatAges[0]; u[9] = a.beatAges[1]; u[10] = a.beatAges[2]; u[11] = a.beatAges[3]
    u[12] = this.w; u[13] = this.h; u[14] = this.w / this.h; u[15] = a.historyRow
    u[16] = f.intensity; u[17] = f.calm ? 1 : 0; u[18] = seed; u[19] = a.beat * (f.calm ? 0.35 : 1) * f.intensity
    u[20] = 0; u[21] = 1; u[22] = f.guard.deadBand; u[23] = f.guard.rise
    u[24] = f.mix; u[25] = f.mixStyle
    u.set(p.a, 28); u.set(p.b, 32); u.set(p.c, 36); u.set(p.d, 40)
    const gl = this.gl
    gl.bufferSubData(gl.UNIFORM_BUFFER, 0, u)
  }

  private drawEffect(id: string, f: DrawInput) {
    const gl = this.gl
    const e = this.effects.get(id)
    if (!e) return
    const t = this.target(id, e.info.feedback)
    const out = e.info.feedback ? 1 - t.at : 0
    gl.bindFramebuffer(gl.FRAMEBUFFER, t.fbo[out])
    gl.viewport(0, 0, this.w, this.h)
    this.sendUniforms(f, e.seed)
    gl.activeTexture(gl.TEXTURE2)
    gl.bindTexture(gl.TEXTURE_2D, e.info.feedback ? t.tex[t.at] : this.black)
    gl.useProgram(e.main.program)
    gl.drawArrays(gl.TRIANGLES, 0, 3)
    if (e.quad) {
      gl.enable(gl.BLEND)
      gl.blendFunc(gl.ONE, gl.ONE)
      gl.useProgram(e.quad.program)
      gl.drawArrays(gl.TRIANGLES, 0, e.info.quads * 6)
      gl.disable(gl.BLEND)
    }
    if (e.info.feedback) t.at = out
  }

  private texOf(id: string | null) {
    const t = id ? this.targets.get(id) : undefined
    return t ? t.tex[t.at] : this.black
  }

  draw(f: DrawInput) {
    const gl = this.gl, a = f.audio
    gl.activeTexture(gl.TEXTURE1)
    gl.bindTexture(gl.TEXTURE_2D, this.audio)
    gl.texSubImage2D(gl.TEXTURE_2D, 0, 0, 0, SPECTRUM_BINS, 4, gl.RED, gl.UNSIGNED_BYTE, a.texture)
    if (a.historyRow !== this.historySent) {
      // Only the row that's new: it's all a frame ever changes.
      gl.texSubImage2D(gl.TEXTURE_2D, 0, 0, 4 + a.historyRow, SPECTRUM_BINS, 1, gl.RED, gl.UNSIGNED_BYTE, a.texture, (4 + a.historyRow) * SPECTRUM_BINS)
      this.historySent = a.historyRow
    }

    this.drawEffect(f.a, f)
    if (f.b) this.drawEffect(f.b, f)

    this.sendUniforms(f, 0)
    gl.activeTexture(gl.TEXTURE2)
    gl.bindTexture(gl.TEXTURE_2D, this.texOf(f.a))
    gl.activeTexture(gl.TEXTURE3)
    gl.bindTexture(gl.TEXTURE_2D, this.texOf(f.b))

    // The guard: reads what's about to be shown and last frame's memory.
    const next = 1 - this.guardAt
    gl.bindFramebuffer(gl.FRAMEBUFFER, this.guardFbo[next])
    gl.viewport(0, 0, GUARD_W, GUARD_H)
    gl.activeTexture(gl.TEXTURE4)
    gl.bindTexture(gl.TEXTURE_2D, this.guardTex[this.guardAt])
    gl.useProgram(this.guardProg.program)
    gl.drawArrays(gl.TRIANGLES, 0, 3)
    this.guardAt = next

    gl.bindFramebuffer(gl.FRAMEBUFFER, null)
    gl.viewport(0, 0, this.w, this.h)
    gl.bindTexture(gl.TEXTURE_2D, this.guardTex[this.guardAt])
    gl.useProgram(this.presentProg.program)
    gl.drawArrays(gl.TRIANGLES, 0, 3)
  }

  // Frees an effect's pictures once it has left the screen; its program is kept, as most
  // effects come round again.
  release(id: string) {
    const t = this.targets.get(id)
    if (t) { this.freeTarget(t); this.targets.delete(id) }
  }

  dispose() {
    this.gl.getExtension('WEBGL_lose_context')?.loseContext()
    this.effects.clear(); this.pending.clear(); this.targets.clear()
  }
}

// A throwaway context for trying out an effect someone added, away from the real one, so a
// shader that upsets the driver costs nothing but itself. Draws it small and times it.
export function trialCompile(e: EffectFile): Promise<string | null> {
  return new Promise((resolve) => {
    const canvas = document.createElement('canvas')
    canvas.width = 64; canvas.height = 36
    const gl = canvas.getContext('webgl2', { alpha: false, antialias: false, depth: false, stencil: false, failIfMajorPerformanceCaveat: false })
    if (!gl) { resolve('This device can\'t try the effect: it has no WebGL 2.'); return }
    let r: GlRenderer
    try { r = new GlRenderer(gl) } catch (err) { resolve((err as Error).message); return }
    const finish = (why: string | null) => { r.dispose(); resolve(why) }
    const started = performance.now()
    const poll = () => {
      const p = r.prepare(e)
      if (p === null) {
        if (performance.now() - started > 8000) finish('It took too long to compile.')
        else setTimeout(poll, 30)
        return
      }
      if (!p.ok) { finish('It would not compile: ' + p.error); return }
      r.setSize(64, 36)
      const audio = { texture: new Uint8Array(SPECTRUM_BINS * AUDIO_ROWS).fill(128), historyRow: 0, bass: 0.5, mid: 0.5, treble: 0.5, level: 0.5, beat: 1, beatCount: 3, beatAges: [0, 1, 2, 3] }
      const flat = { a: [0.5, 0.5, 0.5], b: [0.5, 0.5, 0.5], c: [1, 1, 1], d: [0, 0.33, 0.67] }
      const before = performance.now()
      r.draw({ audio, time: 1, flow: 1, dt: 1 / 60, intensity: 1, calm: false, palette: flat, guard: { deadBand: 0.04, rise: 0.15 }, a: e.id, b: null, mix: 0, mixStyle: 0 })
      gl.readPixels(0, 0, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, new Uint8Array(4))
      const ms = performance.now() - before
      // 64 by 36 is 1/900th of 1080p: this is far too slow for any machine at full size.
      finish(ms > 200 ? `It's too slow: one tiny frame took ${Math.round(ms)} ms.` : null)
    }
    poll()
  })
}

export function createGl(canvas: HTMLCanvasElement) {
  return canvas.getContext('webgl2', { alpha: false, antialias: false, depth: false, stencil: false, preserveDrawingBuffer: false, powerPreference: 'default' })
}
