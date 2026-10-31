import { HISTORY_ROWS, SPECTRUM_BINS, WAVE_POINTS } from './analysis.ts'
import type { EffectFile } from './format.ts'
import type { DrawInput, Renderer } from './gl.ts'
import { FlashGuard, GUARD, GUARD_CALM, flashMetric } from './safety.ts'

// For a device with no WebGL 2: the effects that translate well to a plain canvas, drawn
// by hand. They keep the ids of their shader versions, so a person's choice carries over.
// The flash guard runs here too, on the whole picture at once.

const css = (r: number, g: number, b: number, a = 1) => `rgba(${r * 255 | 0},${g * 255 | 0},${b * 255 | 0},${a})`
const SUPPORTED = new Set(['skyline', 'phosphor', 'sundial', 'warpstars', 'cascade', 'molten'])

export class CanvasRenderer implements Renderer {
  readonly kind = 'canvas' as const
  readonly software = true
  readonly label = 'Plain canvas'
  private ctx: CanvasRenderingContext2D
  private probe: CanvasRenderingContext2D
  private guard = new FlashGuard(GUARD)
  private plasma: CanvasRenderingContext2D
  private plasmaData: ImageData
  private w = 2
  private h = 2

  constructor(private canvas: HTMLCanvasElement) {
    this.ctx = canvas.getContext('2d', { alpha: false })!
    const small = (w: number, h: number, read: boolean) => {
      const c = document.createElement('canvas')
      c.width = w; c.height = h
      return c.getContext('2d', { willReadFrequently: read })!
    }
    this.probe = small(8, 5, true)
    this.plasma = small(80, 45, false)
    this.plasmaData = this.plasma.createImageData(80, 45)
  }

  supports(id: string) { return SUPPORTED.has(id) }
  prepare(e: EffectFile) { return SUPPORTED.has(e.id) ? { ok: true as const } : { ok: false as const, error: 'This effect needs WebGL 2.' } }
  release() { /* nothing is held */ }
  dispose() { /* nothing is held */ }

  setSize(w: number, h: number) {
    w = Math.max(2, Math.round(w)); h = Math.max(2, Math.round(h))
    if (w === this.w && h === this.h) return
    this.w = w; this.h = h
    this.canvas.width = w; this.canvas.height = h
  }

  private color(f: DrawInput, t: number) {
    const p = f.palette, c = (i: number) => Math.max(0, Math.min(1, p.a[i] + p.b[i] * Math.cos(2 * Math.PI * (p.c[i] * t + p.d[i]))))
    return [c(0), c(1), c(2)]
  }

  draw(f: DrawInput) {
    const { ctx, w, h } = this
    const id = f.b && f.mix > 0.5 ? f.b : f.a
    if (id === 'molten') this.molten(f)
    else {
      ctx.fillStyle = id === 'phosphor' ? 'rgba(2,8,5,0.35)' : id === 'warpstars' ? 'rgba(0,0,5,0.5)' : '#020308'
      ctx.fillRect(0, 0, w, h)
      if (id === 'phosphor') this.scope(f)
      else if (id === 'sundial') this.dial(f)
      else if (id === 'warpstars') this.stars(f)
      else if (id === 'cascade') this.waterfall(f)
      else this.bars(f)
    }
    // A blend between effects, as a dip through dark.
    const dip = f.b ? 1 - Math.abs(f.mix * 2 - 1) : 0
    if (dip > 0) { ctx.fillStyle = `rgba(0,0,0,${dip})`; ctx.fillRect(0, 0, w, h) }

    const limits = f.calm ? GUARD_CALM : GUARD
    this.guard.deadBand = limits.deadBand
    this.guard.rise = limits.rise
    this.probe.drawImage(this.canvas, 0, 0, 8, 5)
    const d = this.probe.getImageData(0, 0, 8, 5).data
    let r = 0, g = 0, b = 0
    for (let i = 0; i < d.length; i += 4) { r += d[i]; g += d[i + 1]; b += d[i + 2] }
    const n = (d.length / 4) * 255
    const gain = this.guard.step(flashMetric(r / n, g / n, b / n), f.dt)
    if (gain < 1) { ctx.fillStyle = `rgba(0,0,0,${1 - Math.pow(gain, 1 / 2.2)})`; ctx.fillRect(0, 0, w, h) }
  }

  private bars(f: DrawInput) {
    const { ctx, w, h } = this, tex = f.audio.texture
    const bars = 40, gap = w / bars * 0.18, bw = w / bars - gap, base = h * 0.94
    for (let i = 0; i < bars; i++) {
      const at = Math.floor(((i + 0.5) / bars) * SPECTRUM_BINS)
      const v = tex[at] / 255, cap = tex[3 * SPECTRUM_BINS + at] / 255
      const c = this.color(f, i / bars * 0.65 + f.flow * 0.025)
      ctx.fillStyle = css(c[0], c[1], c[2])
      const x = i * (bw + gap) + gap / 2
      ctx.fillRect(x, base - v * base * 0.95, bw, v * base * 0.95)
      ctx.fillStyle = '#fff5e0'
      ctx.fillRect(x, base - cap * base * 0.95 - 3, bw, 2)
    }
  }

  private scope(f: DrawInput) {
    const { ctx, w, h } = this, tex = f.audio.texture
    for (const [row, y, col] of [[1, 0.7, '#5cff8c'], [2, 0.3, '#5cc0ff']] as const) {
      ctx.strokeStyle = col
      ctx.lineWidth = Math.max(1, h / 240)
      ctx.beginPath()
      for (let i = 0; i < WAVE_POINTS; i += 2) {
        const v = tex[row * SPECTRUM_BINS + i] / 255 * 2 - 1
        const px = (i / WAVE_POINTS) * w, py = h * (1 - y) - v * h * 0.2
        if (i) ctx.lineTo(px, py); else ctx.moveTo(px, py)
      }
      ctx.stroke()
    }
  }

  private dial(f: DrawInput) {
    const { ctx, w, h } = this, tex = f.audio.texture
    const cx = w / 2, cy = h / 2, u = h / 2, rays = 72
    ctx.lineWidth = Math.max(1, u / 60)
    for (let i = 0; i < rays; i++) {
      const folded = Math.abs((((i + 0.5) / rays) * 2) % 1 * 2 - 1)
      const v = tex[Math.floor(folded * (SPECTRUM_BINS - 1))] / 255
      const ang = (i / rays) * Math.PI * 2 + f.flow * 0.045
      const c = this.color(f, folded * 0.7 + f.flow * 0.03)
      ctx.strokeStyle = css(c[0], c[1], c[2])
      ctx.beginPath()
      ctx.moveTo(cx + Math.cos(ang) * u * 0.28, cy + Math.sin(ang) * u * 0.28)
      ctx.lineTo(cx + Math.cos(ang) * u * (0.32 + v * 0.65), cy + Math.sin(ang) * u * (0.32 + v * 0.65))
      ctx.stroke()
    }
  }

  private stars(f: DrawInput) {
    const { ctx, w, h } = this
    const c = this.color(f, f.flow * 0.02)
    ctx.strokeStyle = css(0.5 + c[0] / 2, 0.5 + c[1] / 2, 0.5 + c[2] / 2)
    ctx.lineWidth = Math.max(1, h / 480)
    const hash = (n: number) => { const v = Math.sin(n * 127.1) * 43758.5453; return v - Math.floor(v) }
    const stretch = 0.012 + 0.05 * f.audio.bass * f.intensity
    ctx.beginPath()
    for (let i = 0; i < 180; i++) {
      const x = (hash(i * 1.31) - 0.5) * 2.4, y = (hash(i * 2.77) - 0.5) * 2.4
      let z = hash(i * 0.73) - f.flow * 0.14
      z = Math.max(z - Math.floor(z), 0.01)
      const sx = (X: number, Z: number) => w / 2 + (X / Z) * h * 0.3, sy = (Y: number, Z: number) => h / 2 + (Y / Z) * h * 0.3
      ctx.moveTo(sx(x, z), sy(y, z))
      ctx.lineTo(sx(x, z + stretch), sy(y, z + stretch))
    }
    ctx.stroke()
  }

  private waterfall(f: DrawInput) {
    const { ctx, w, h } = this, tex = f.audio.texture
    const cols = 48, rows = HISTORY_ROWS, cw = w / cols, ch = h / rows
    for (let r = 0; r < rows; r++) {
      const row = 4 + ((f.audio.historyRow - r + HISTORY_ROWS) % HISTORY_ROWS)
      for (let i = 0; i < cols; i++) {
        const x = Math.abs((i + 0.5) / cols - 0.5) * 2
        const v = tex[row * SPECTRUM_BINS + Math.floor(x * 0.92 * SPECTRUM_BINS)] / 255
        if (v < 0.25) continue
        const c = this.color(f, v * 0.8 + f.flow * 0.02)
        ctx.fillStyle = css(c[0], c[1], c[2], v * v)
        ctx.fillRect(i * cw, r * ch, cw + 0.5, ch + 0.5)
      }
    }
  }

  private molten(f: DrawInput) {
    const d = this.plasmaData.data, W = 80, H = 45, t = f.flow * 0.42, asp = this.w / this.h
    const swell = (0.28 + 0.5 * f.audio.bass * f.intensity) * 0.5
    const p = f.palette, off = f.flow * 0.03
    for (let y = 0; y < H; y++) {
      for (let x = 0; x < W; x++) {
        let px = (x / W - 0.5) * 2 * asp * 1.3, py = (y / H - 0.5) * 2 * 1.3
        const qx = px + swell * Math.sin(py * 2.1 + t), qy = py + swell * Math.cos(px * 1.9 - t * 0.8)
        px = qx; py = qy
        const v = (Math.sin(px * 2.8 + t) + Math.sin(py * 3.4 - t * 1.3) + Math.sin((px + py) * 2.2 + t * 0.7)) / 3 * 0.5 + 0.5
        const k = 0.42 + 0.5 * (0.5 + 0.5 * Math.sin(v * 18.85))
        const i = (y * W + x) * 4, u = v * 1.1 + off
        d[i] = 255 * k * (p.a[0] + p.b[0] * Math.cos(6.283 * (p.c[0] * u + p.d[0])))
        d[i + 1] = 255 * k * (p.a[1] + p.b[1] * Math.cos(6.283 * (p.c[1] * u + p.d[1])))
        d[i + 2] = 255 * k * (p.a[2] + p.b[2] * Math.cos(6.283 * (p.c[2] * u + p.d[2])))
        d[i + 3] = 255
      }
    }
    this.plasma.putImageData(this.plasmaData, 0, 0)
    this.ctx.imageSmoothingEnabled = true
    this.ctx.drawImage(this.plasma.canvas, 0, 0, this.w, this.h)
  }
}
