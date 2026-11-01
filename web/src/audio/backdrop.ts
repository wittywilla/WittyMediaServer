import { SoundListener } from './sound'

// The player's own moving pictures, drawn behind the artwork: Bars, Waves and Drift. They
// are the ones that can be on by default, because of what they aren't: no flashes, nothing
// that changes the whole screen's brightness, so no warning is needed before showing them.
// One canvas at low resolution, at most 30 frames a second, and it stops when hidden.

export type NativeStyle = 'bars' | 'waves' | 'drift'
export const NATIVE_STYLES: { id: NativeStyle; name: string }[] = [{ id: 'bars', name: 'Bars' }, { id: 'waves', name: 'Waves' }, { id: 'drift', name: 'Drift' }]

const BARS = 46 // on each side of the centre together
const MOTES = 14

type Mote = { x: number; y: number; r: number; vx: number; vy: number; band: number }

export class Backdrop {
  private canvas: HTMLCanvasElement
  private g: CanvasRenderingContext2D
  private listener: SoundListener
  private raf = 0
  private last = 0
  private freq = new Uint8Array(1024)
  private levels = new Float32Array(BARS)
  private motes: Mote[] = []
  private sprite: HTMLCanvasElement | null = null
  private t = 0
  private w = 0
  private h = 0
  style: NativeStyle = 'bars'
  slow = false
  rgb = '150 150 165'
  // Paused music settles, and then nothing is drawn at all until it plays again.
  active = true
  private calm = false
  private idle = false
  private sizer: ResizeObserver

  constructor(canvas: HTMLCanvasElement, media: () => HTMLMediaElement | null) {
    this.canvas = canvas
    this.g = canvas.getContext('2d', { alpha: true, desynchronized: true })!
    this.listener = new SoundListener(media)
    this.calm = matchMedia('(prefers-reduced-motion: reduce)').matches
    this.sizer = new ResizeObserver(() => this.resize())
    this.sizer.observe(canvas)
    this.resize()
    document.addEventListener('visibilitychange', this.onVisible)
    for (let i = 0; i < MOTES; i++) {
      this.motes.push({ x: Math.random(), y: Math.random(), r: 0.12 + Math.random() * 0.2, vx: (Math.random() - 0.5) * 0.02, vy: (Math.random() - 0.5) * 0.02, band: i % 6 })
    }
    this.start()
  }

  private resize() {
    const r = this.canvas.getBoundingClientRect()
    // Soft shapes on black: half the pixels of a large screen look the same and cost a quarter.
    const scale = Math.min(1, 960 / Math.max(1, r.width))
    this.w = Math.max(1, Math.round(r.width * scale))
    this.h = Math.max(1, Math.round(r.height * scale))
    this.canvas.width = this.w
    this.canvas.height = this.h
    this.sprite = null
  }

  private onVisible = () => { if (document.hidden) this.stop(); else this.start() }

  start() {
    if (!this.raf && !document.hidden) { this.last = performance.now(); this.raf = requestAnimationFrame(this.tick) }
  }

  stop() {
    if (this.raf) cancelAnimationFrame(this.raf)
    this.raf = 0
  }

  destroy() {
    this.stop()
    this.sizer.disconnect()
    document.removeEventListener('visibilitychange', this.onVisible)
    this.listener.close()
  }

  // A soft round light, drawn once and stamped many times: far cheaper than a blur.
  private mote() {
    if (this.sprite) return this.sprite
    const c = document.createElement('canvas')
    c.width = c.height = 128
    const g = c.getContext('2d')!
    const grad = g.createRadialGradient(64, 64, 0, 64, 64, 64)
    grad.addColorStop(0, `rgb(${this.rgb} / .5)`)
    grad.addColorStop(0.5, `rgb(${this.rgb} / .16)`)
    grad.addColorStop(1, `rgb(${this.rgb} / 0)`)
    g.fillStyle = grad
    g.fillRect(0, 0, 128, 128)
    this.sprite = c
    return c
  }

  setColour(rgb: string) {
    if (rgb !== this.rgb) { this.rgb = rgb; this.sprite = null }
  }

  private tick = (now: number) => {
    this.raf = requestAnimationFrame(this.tick)
    const gap = this.slow || this.calm ? 1000 / 20 : 1000 / 30
    if (now - this.last < gap - 2) return
    const dt = Math.min(0.2, (now - this.last) / 1000)
    this.last = now
    const heard = this.active && this.listener.read(this.freq)
    this.measure(heard, dt, now)
    // Settled and paused: what's on screen is already right, so nothing is drawn.
    if (!this.active && this.levels.every((v) => v < 0.004)) {
      if (this.idle) return
      this.idle = true
    } else this.idle = false
    this.t += dt * (this.slow || this.calm ? 0.35 : 1)
    this.g.clearRect(0, 0, this.w, this.h)
    if (this.style === 'bars') this.drawBars()
    else if (this.style === 'waves') this.drawWaves()
    else this.drawDrift(dt)
  }

  // Levels from low to high, on a scale that spreads the music out across the bars.
  private measure(heard: boolean, dt: number, now: number) {
    const n = BARS / 2
    const rise = this.slow || this.calm ? 2.5 : 14
    const fall = this.slow || this.calm ? 1 : 3.2
    for (let i = 0; i < n; i++) {
      let v = 0
      if (heard) {
        const a = Math.floor(2 * Math.pow(170, i / n))
        const b = Math.max(a + 1, Math.floor(2 * Math.pow(170, (i + 1) / n)))
        let peak = 0
        for (let k = a; k < b && k < this.freq.length; k++) peak = Math.max(peak, this.freq[k])
        // The top of the range is quieter in nearly all music: lift it to match.
        v = Math.pow((peak / 255) * (1 + (i / n) * 0.45), 3)
      } else if (this.active) {
        // Nothing to read here (another address, an iPhone): a slow swell, and not a pretence.
        v = 0.18 + 0.1 * Math.sin(now / 1400 + i * 0.5) + 0.05 * Math.sin(now / 700 + i)
      }
      const old = this.levels[i]
      this.levels[i] = v > old ? old + (v - old) * Math.min(1, dt * rise) : old + (v - old) * Math.min(1, dt * fall)
    }
  }

  private drawBars() {
    const { g, w, h } = this
    const n = BARS / 2
    const gap = w / BARS
    const bw = gap * 0.62
    const base = h * 0.62
    const tall = h * 0.46
    const grad = g.createLinearGradient(0, base - tall, 0, base)
    grad.addColorStop(0, `rgb(${this.rgb} / .55)`)
    grad.addColorStop(1, `rgb(${this.rgb} / 0)`)
    g.fillStyle = grad
    // From the centre out, each side a mirror of the other. The edges fade away.
    for (let i = 0; i < n; i++) {
      const v = 0.12 + this.levels[i] * 0.88
      const bh = Math.min(tall, v * tall)
      const off = (i + 0.5) * gap
      g.globalAlpha = 1 - (i / n) * 0.72
      g.fillRect(w / 2 + off - bw / 2, base - bh, bw, bh)
      g.fillRect(w / 2 - off - bw / 2, base - bh, bw, bh)
    }
    g.globalAlpha = 1
  }

  private drawWaves() {
    const { g, w, h } = this
    const low = this.band(0, 4), mid = this.band(4, 12), high = this.band(12, 23)
    const layers = [{ a: 0.2 + low * 0.3, f: 1.2, s: 0.5, y: 0.62, o: 0.3 }, { a: 0.12 + mid * 0.28, f: 2.1, s: -0.7, y: 0.6, o: 0.22 }, { a: 0.07 + high * 0.22, f: 3.4, s: 1, y: 0.58, o: 0.16 }]
    for (const l of layers) {
      g.beginPath()
      g.moveTo(0, h)
      for (let x = 0; x <= w; x += 8) {
        const u = x / w
        const env = Math.sin(u * Math.PI) * 0.8 + 0.2
        const y = h * l.y - env * h * 0.42 * l.a * (0.6 + 0.4 * Math.sin(u * l.f * 6.283 + this.t * l.s * 1.4)) - h * 0.02 * Math.sin(u * 3 + this.t * l.s)
        g.lineTo(x, y)
      }
      g.lineTo(w, h)
      g.closePath()
      const grad = g.createLinearGradient(0, h * 0.2, 0, h * 0.8)
      grad.addColorStop(0, `rgb(${this.rgb} / ${l.o})`)
      grad.addColorStop(1, `rgb(${this.rgb} / 0)`)
      g.fillStyle = grad
      g.fill()
    }
  }

  private drawDrift(dt: number) {
    const { g, w, h } = this
    const sprite = this.mote()
    const push = this.slow || this.calm ? 0.35 : 1
    g.globalCompositeOperation = 'lighter'
    for (const m of this.motes) {
      const level = this.levels[Math.min(this.levels.length - 1, m.band * 3)] || 0
      m.x += m.vx * dt * push * (1 + level * 2)
      m.y += m.vy * dt * push * (1 + level * 2)
      if (m.x < -0.2) m.x = 1.2; else if (m.x > 1.2) m.x = -0.2
      if (m.y < -0.2) m.y = 1.2; else if (m.y > 1.2) m.y = -0.2
      const size = (m.r + level * 0.1) * Math.max(w, h)
      g.globalAlpha = 0.45 + Math.min(0.45, level * 0.5)
      g.drawImage(sprite, m.x * w - size / 2, m.y * h - size / 2, size, size)
    }
    g.globalAlpha = 1
    g.globalCompositeOperation = 'source-over'
  }

  private band(a: number, b: number) {
    let s = 0
    for (let i = a; i < b; i++) s += this.levels[i] || 0
    return s / Math.max(1, b - a)
  }
}
