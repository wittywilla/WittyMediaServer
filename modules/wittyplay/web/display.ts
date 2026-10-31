// The picture's last step, in the browser: scanlines, a CRT's curve and glow, an LCD's
// grid, colour. The server knows nothing about it, so it costs a low-end server nothing.
//
// With no effect turned on there's nothing here at all: the video element shows itself,
// and that's the fastest path there is. An effect draws each new frame onto a canvas
// the moment it arrives, using a small shader of our own. Where WebGL isn't there, or
// goes away, plain CSS does what it can: colour, scanlines and the grid.

export type DisplayValues = Record<string, unknown>
type Media = HTMLVideoElement | HTMLCanvasElement

const num = (v: DisplayValues, k: string, d = 0) => { const n = v[k]; return typeof n === 'number' ? n : d }

const VERT = `attribute vec2 p; varying vec2 uv;
void main() { uv = vec2(p.x * .5 + .5, .5 - p.y * .5); gl_Position = vec4(p, 0., 1.); }`

// Sharp sampling blends only across the seam between two game pixels. The curve bends
// the sampling position, not the picture. Scanlines darken between a pixel row's middle
// and its edge, and bright rows stay wider, as a beam did.
const FRAG = `precision mediump float;
varying vec2 uv;
uniform sampler2D tex, prev;
uniform vec2 src, screen;
uniform vec4 crop;
uniform float sharp, blend, scan, curve, mask, glow, vignette, grid, flicker, bright, contrast, sat, gamma;
vec2 bend(vec2 u) {
  if (curve <= 0.) return u;
  vec2 c = u * 2. - 1.;
  vec2 k = c.yx * c.yx * curve * .18;
  return (c * (1. + k)) * .5 + .5;
}
vec3 fetch(sampler2D t, vec2 u) {
  if (sharp > .5) {
    vec2 px = u * src;
    vec2 scale = max(screen / src, vec2(1.));
    vec2 near = floor(px) + .5;
    u = (near + clamp((px - near) * scale, -.5, .5)) / src;
  }
  return texture2D(t, u).rgb;
}
void main() {
  vec2 u = bend(uv);
  if (u.x < 0. || u.x > 1. || u.y < 0. || u.y > 1.) { gl_FragColor = vec4(0., 0., 0., 1.); return; }
  vec2 cu = crop.xy + u * crop.zw;
  vec3 c = fetch(tex, cu);
  if (blend > 0.) c = mix(c, fetch(prev, cu), blend);
  if (glow > 0.) {
    vec2 d = 1.5 / src;
    vec3 g = texture2D(tex, cu + vec2(d.x, 0.)).rgb + texture2D(tex, cu - vec2(d.x, 0.)).rgb + texture2D(tex, cu + vec2(0., d.y)).rgb + texture2D(tex, cu - vec2(0., d.y)).rgb;
    c += g * .25 * glow * .6;
  }
  float l = dot(c, vec3(.299, .587, .114));
  if (scan > 0.) {
    float scale = screen.y / src.y;
    float y = fract(cu.y * src.y) - .5;
    float w = mix(.28, .46, clamp(l, 0., 1.));
    float beam = exp(-(y * y) / (2. * w * w));
    float s = scan * smoothstep(1., 2., scale);
    c *= mix(1., beam * (1. + .35 * s), s);
  }
  if (grid > 0.) {
    vec2 f = fract(cu * src);
    vec2 e = min(f, 1. - f);
    float w = clamp(1.2 * src.y / screen.y, .02, .5);
    float g = smoothstep(0., w, e.x) * smoothstep(0., w, e.y);
    c *= mix(1. - grid * .7, 1., g);
  }
  if (mask > 0.) {
    float m = mod(gl_FragCoord.x, 3.);
    vec3 t = m < 1. ? vec3(1., .7, .7) : m < 2. ? vec3(.7, 1., .7) : vec3(.7, .7, 1.);
    c *= mix(vec3(1.), t * 1.15, mask);
  }
  if (vignette > 0.) {
    vec2 v = u * (1. - u);
    c *= mix(1., clamp(pow(v.x * v.y * 16., .35), 0., 1.), vignette);
  }
  if (flicker > 0.) c *= 1. - flicker * .03;
  c = (c - .5) * contrast + .5;
  c *= bright;
  c = mix(vec3(dot(c, vec3(.299, .587, .114))), c, sat);
  if (gamma != 1.) c = pow(max(c, vec3(0.)), vec3(1. / gamma));
  gl_FragColor = vec4(c, 1.);
}`

// The settings that need the shader, rather than CSS.
const SHADER_KEYS = ['display.scanlines', 'display.curve', 'display.mask', 'display.glow', 'display.vignette', 'display.lcdGrid', 'display.blend', 'display.flicker']

export const reducedMotion = () => window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false

export function wantsShader(v: DisplayValues): boolean {
  if (SHADER_KEYS.some((k) => num(v, k) > 0)) return true
  return num(v, 'display.gamma', 1) !== 1
}

// Colour through CSS: free, and works on anything.
function cssFilter(v: DisplayValues): string {
  const b = num(v, 'display.brightness', 1), c = num(v, 'display.contrast', 1), s = num(v, 'display.saturation', 1)
  if (b === 1 && c === 1 && s === 1) return ''
  return `brightness(${b}) contrast(${c}) saturate(${s})`
}

// Scanlines and the grid without a shader, as patterns laid over the picture.
function cssOverlay(v: DisplayValues, srcW: number, srcH: number, picW: number, picH: number): string {
  const layers: string[] = []
  const lineH = picH / Math.max(1, srcH), cellW = picW / Math.max(1, srcW)
  const scan = num(v, 'display.scanlines')
  if (scan > 0 && lineH >= 2) layers.push(`repeating-linear-gradient(to bottom, transparent 0, transparent ${lineH / 2}px, rgba(0,0,0,${scan * 0.75}) ${lineH / 2}px, rgba(0,0,0,${scan * 0.75}) ${lineH}px)`)
  const grid = num(v, 'display.lcdGrid')
  if (grid > 0 && lineH >= 2 && cellW >= 2) {
    const a = `rgba(0,0,0,${grid * 0.6})`
    layers.push(`repeating-linear-gradient(to bottom, ${a} 0, ${a} 1px, transparent 1px, transparent ${lineH}px)`, `repeating-linear-gradient(to right, ${a} 0, ${a} 1px, transparent 1px, transparent ${cellW}px)`)
  }
  return layers.join(', ')
}

export type DisplayMode = 'plain' | 'shader' | 'css'

export class DisplayEngine {
  // Says when the drawing path changes, so that a menu can be honest about it.
  onMode?: (m: DisplayMode) => void
  private canvas: HTMLCanvasElement | null = null
  private overlay: HTMLDivElement | null = null
  private gl: WebGLRenderingContext | null = null
  private prog: WebGLProgram | null = null
  private u: Record<string, WebGLUniformLocation | null> = {}
  private tex: [WebGLTexture | null, WebGLTexture | null] = [null, null]
  private newest = 0
  private haveFrame = false
  private media: Media | null = null
  private values: DisplayValues = {}
  private srcW = 256
  private srcH = 224
  private picW = 256
  private picH = 224
  private cropX = 0
  private cropY = 0
  private mode: DisplayMode = 'plain'
  private raf = 0
  private vfc = 0
  private running = false
  private lost = false
  private failed = false

  constructor(private stage: HTMLElement) {}

  // The element that draws frames: a video in the usual case, a canvas on the fallbacks.
  attach(media: Media) {
    this.media = media
    this.apply()
  }

  // Everything that affects the picture. Safe to call often: it does only what changed.
  update(values: DisplayValues, src: { width: number; height: number }, size: { picW: number; picH: number }) {
    this.values = values
    if (src.width > 0 && src.height > 0) {
      this.srcW = src.width
      this.srcH = src.height
    }
    this.picW = size.picW
    this.picH = size.picH
    this.cropX = num(values, 'display.cropX') / 100
    this.cropY = num(values, 'display.cropY') / 100
    this.apply()
  }

  get currentMode() { return this.mode }

  private apply() {
    const media = this.media
    if (!media) return
    const smooth = this.values['display.scaling'] === 'smooth'
    const pixel = this.values['display.scaling'] === 'pixel'
    media.style.imageRendering = smooth ? 'auto' : 'pixelated'
    let want: DisplayMode = 'plain'
    if (wantsShader(this.values)) want = this.failed || this.lost ? 'css' : 'shader'
    if (want === 'shader' && !this.ensureGL()) want = 'css'
    this.setMode(want)
    // The shader does colour itself. Everywhere else CSS does it.
    media.style.filter = this.mode === 'shader' ? '' : cssFilter(this.values)
    if (this.mode === 'shader') {
      this.resize()
      this.setFilter(smooth || !pixel)
      this.draw(true)
    }
    this.placeOverlay()
  }

  private setMode(m: DisplayMode) {
    if (m === this.mode && (m !== 'shader' || this.running)) return
    this.mode = m
    if (m === 'shader') {
      this.canvas!.style.display = 'block'
      if (this.media) this.media.style.opacity = '0' // it has to go on playing to give us frames
      this.start()
    } else {
      this.stop()
      if (this.canvas) this.canvas.style.display = 'none'
      if (this.media) this.media.style.opacity = ''
    }
    this.onMode?.(m)
  }

  private placeOverlay() {
    const css = this.mode === 'css' ? cssOverlay(this.values, this.srcW, this.srcH, this.picW, this.picH) : ''
    if (!css) {
      this.overlay?.remove()
      this.overlay = null
      return
    }
    if (!this.overlay) {
      this.overlay = document.createElement('div')
      this.overlay.className = 'wp-fx-overlay'
      this.stage.appendChild(this.overlay)
    }
    this.overlay.style.backgroundImage = css
  }

  private ensureGL(): boolean {
    if (this.gl) return true
    if (this.failed) return false
    try {
      const canvas = document.createElement('canvas')
      canvas.className = 'wp-media wp-gl'
      canvas.setAttribute('aria-hidden', 'true')
      const gl = canvas.getContext('webgl', { alpha: false, antialias: false, depth: false, stencil: false, desynchronized: true, powerPreference: 'low-power' } as WebGLContextAttributes) as WebGLRenderingContext | null
      if (!gl) throw new Error('no WebGL')
      const compile = (kind: number, text: string) => {
        const s = gl.createShader(kind)!
        gl.shaderSource(s, text)
        gl.compileShader(s)
        if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) throw new Error(gl.getShaderInfoLog(s) || 'shader')
        return s
      }
      const prog = gl.createProgram()!
      gl.attachShader(prog, compile(gl.VERTEX_SHADER, VERT))
      gl.attachShader(prog, compile(gl.FRAGMENT_SHADER, FRAG))
      gl.linkProgram(prog)
      if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) throw new Error(gl.getProgramInfoLog(prog) || 'link')
      gl.useProgram(prog)
      gl.bindBuffer(gl.ARRAY_BUFFER, gl.createBuffer())
      gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]), gl.STATIC_DRAW)
      const p = gl.getAttribLocation(prog, 'p')
      gl.enableVertexAttribArray(p)
      gl.vertexAttribPointer(p, 2, gl.FLOAT, false, 0, 0)
      for (const n of ['tex', 'prev', 'src', 'screen', 'crop', 'sharp', 'blend', 'scan', 'curve', 'mask', 'glow', 'vignette', 'grid', 'flicker', 'bright', 'contrast', 'sat', 'gamma']) this.u[n] = gl.getUniformLocation(prog, n)
      this.tex = [gl.createTexture(), gl.createTexture()]
      for (const t of this.tex) {
        gl.bindTexture(gl.TEXTURE_2D, t)
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
      }
      canvas.addEventListener('webglcontextlost', (e) => {
        e.preventDefault()
        this.lost = true
        this.gl = null
        this.apply() // down to CSS until it comes back
      })
      canvas.addEventListener('webglcontextrestored', () => {
        this.lost = false
        canvas.remove()
        this.canvas = null
        this.apply()
      })
      this.gl = gl
      this.prog = prog
      this.canvas = canvas
      this.stage.appendChild(canvas)
      return true
    } catch {
      this.failed = true
      return false
    }
  }

  private setFilter(linear: boolean) {
    const gl = this.gl
    if (!gl) return
    const f = linear ? gl.LINEAR : gl.NEAREST
    for (const t of this.tex) {
      gl.bindTexture(gl.TEXTURE_2D, t)
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, f)
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, f)
    }
  }

  private resize() {
    const c = this.canvas
    if (!c) return
    // What's drawn is what's seen, up to a limit that keeps a phone or an old TV quick.
    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    const w = Math.min(Math.round(this.picW * dpr), 2560), h = Math.min(Math.round(this.picH * dpr), 1600)
    if (c.width !== w || c.height !== h) {
      c.width = w
      c.height = h
      this.gl?.viewport(0, 0, w, h)
    }
  }

  private start() {
    if (this.running) return
    this.running = true
    this.schedule()
  }

  private stop() {
    this.running = false
    cancelAnimationFrame(this.raf)
    const m = this.media as HTMLVideoElement | null
    if (this.vfc && m && 'cancelVideoFrameCallback' in m) (m as any).cancelVideoFrameCallback(this.vfc)
    this.vfc = 0
  }

  // A video says when it has a new frame, so a frame is drawn as soon as it's decoded
  // and nothing is done in between. A canvas doesn't, so it's looked at once per display
  // refresh, at most.
  private schedule() {
    if (!this.running) return
    const m = this.media as any
    if (m && m.tagName === 'VIDEO' && m.requestVideoFrameCallback) {
      this.vfc = m.requestVideoFrameCallback(() => { this.draw(false); this.schedule() })
      return
    }
    this.raf = requestAnimationFrame(() => { this.draw(false); this.schedule() })
  }

  private ready(): boolean {
    const m = this.media as any
    if (!m) return false
    return m.tagName === 'VIDEO' ? m.videoWidth > 0 && m.readyState >= 2 : m.width > 0
  }

  // keep: redraw the frame already uploaded, because only the settings have changed.
  private draw(keep: boolean) {
    const gl = this.gl
    if (!gl || !this.prog || !this.media) return
    if (keep && !this.ready()) return
    if (gl.isContextLost()) return
    try {
      const v = this.values
      const blend = num(v, 'display.blend')
      if (!keep || !this.haveFrame) {
        if (this.ready()) {
          if (blend > 0) this.newest ^= 1 // the older texture holds the frame before this
          gl.activeTexture(gl.TEXTURE0)
          gl.bindTexture(gl.TEXTURE_2D, this.tex[this.newest])
          gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGB, gl.RGB, gl.UNSIGNED_BYTE, this.media as any)
          this.haveFrame = true
        }
      }
      gl.activeTexture(gl.TEXTURE0)
      gl.bindTexture(gl.TEXTURE_2D, this.tex[this.newest])
      gl.activeTexture(gl.TEXTURE1)
      gl.bindTexture(gl.TEXTURE_2D, this.tex[this.newest ^ 1])
      const u = this.u
      gl.uniform1i(u.tex, 0)
      gl.uniform1i(u.prev, 1)
      gl.uniform2f(u.src, this.srcW, this.srcH)
      // On this path the crop happens here, so that a curve bends what's actually seen.
      const keepW = 1 - 2 * this.cropX, keepH = 1 - 2 * this.cropY
      gl.uniform4f(u.crop, this.cropX, this.cropY, keepW, keepH)
      gl.uniform2f(u.screen, this.canvas!.width / keepW, this.canvas!.height / keepH)
      gl.uniform1f(u.sharp, v['display.scaling'] === 'pixel' ? 0 : v['display.scaling'] === 'smooth' ? 0 : 1)
      gl.uniform1f(u.blend, blend)
      gl.uniform1f(u.scan, num(v, 'display.scanlines'))
      gl.uniform1f(u.curve, num(v, 'display.curve'))
      gl.uniform1f(u.mask, num(v, 'display.mask'))
      gl.uniform1f(u.glow, num(v, 'display.glow'))
      gl.uniform1f(u.vignette, num(v, 'display.vignette'))
      gl.uniform1f(u.grid, num(v, 'display.lcdGrid'))
      // A slow wobble of brightness, never more than a small part of it, and never for
      // anyone whose device asks for less motion.
      const fl = reducedMotion() ? 0 : num(v, 'display.flicker')
      gl.uniform1f(u.flicker, fl > 0 ? fl * (0.5 + 0.5 * Math.sin(performance.now() / 70)) : 0)
      gl.uniform1f(u.bright, num(v, 'display.brightness', 1))
      gl.uniform1f(u.contrast, num(v, 'display.contrast', 1))
      gl.uniform1f(u.sat, num(v, 'display.saturation', 1))
      gl.uniform1f(u.gamma, num(v, 'display.gamma', 1))
      gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4)
    } catch {
      // A browser that refuses a frame (a tainted or closing source) falls back rather than break the game.
      this.failed = true
      this.gl = null
      this.apply()
    }
  }

  // A picture of what's on screen, effects and all, or the plain frame without them.
  capture(): Promise<Blob | null> {
    return new Promise((resolve) => {
      const m = this.media as any
      if (!m) return resolve(null)
      try {
        if (this.mode === 'shader' && this.canvas) {
          this.draw(false)
          this.canvas.toBlob((b) => resolve(b), 'image/png')
          return
        }
        const w = m.videoWidth || m.width, h = m.videoHeight || m.height
        if (!w || !h) return resolve(null)
        const c = document.createElement('canvas')
        c.width = w
        c.height = h
        c.getContext('2d')!.drawImage(m, 0, 0)
        c.toBlob((b) => resolve(b), 'image/png')
      } catch { resolve(null) }
    })
  }

  destroy() {
    this.stop()
    this.canvas?.remove()
    this.overlay?.remove()
    if (this.media) {
      this.media.style.opacity = ''
      this.media.style.filter = ''
      this.media.style.imageRendering = ''
    }
    const ext = this.gl?.getExtension('WEBGL_lose_context')
    ext?.loseContext()
    this.gl = null
  }
}
