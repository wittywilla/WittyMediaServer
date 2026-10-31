import { Analyzer, FFT_SIZE, MIN_BEAT_GAP } from './analysis.ts'
import { CanvasRenderer } from './canvas2d.ts'
import { Budget, Cycle, DEFAULT_SETTINGS, Palettes, eligible, normalizeSettings, type Settings } from './engine.ts'
import { parseEffect, type EffectFile } from './format.ts'
import { GlRenderer, createGl, trialCompile, type Renderer } from './gl.ts'
import { GUARD, GUARD_CALM } from './safety.ts'
import { BUILTIN_EFFECTS, PALETTES } from './shaders.ts'
import { openTap, type AudioTap, type TapState } from './tap.ts'
import type { VisualizerHost } from './host.ts'

// One visualizer for the whole page: one canvas, which moves between the places it's
// shown (so nothing restarts when the full player opens), and one loop, which stops
// completely whenever it can't be seen.

const CUSTOM_KEY = 'wms.visualizer.custom'
const MAX_CUSTOM = 8
const LONG_EDGE = 1920 // past this, more pixels cost more than they show
const TRANSITION = 2.4

export type Place = 'panel' | 'mini' | 'full'

export type Status = {
  running: boolean
  effect: string
  tap: TapState
  tapWhy: string
  renderer: string
  rendererDetail: string
  fps: number
  width: number
  height: number
  scale: number
  frameMs: number
  beats: number
  problem: string
}

const calmBySystem = () => { try { return matchMedia('(prefers-reduced-motion: reduce)').matches } catch { return false } }

export class Session {
  settings: Settings = DEFAULT_SETTINGS
  loaded = false
  status: Status = { running: false, effect: '', tap: 'closed', tapWhy: '', renderer: '', rendererDetail: '', fps: 0, width: 0, height: 0, scale: 1, frameMs: 0, beats: 0, problem: '' }
  effects: EffectFile[] = BUILTIN_EFFECTS
  canvas: HTMLCanvasElement

  private host: VisualizerHost
  private listeners = new Set<() => void>()
  private renderer: Renderer | null = null
  private analyzer = new Analyzer()
  private cycle = new Cycle()
  private palettes = new Palettes(PALETTES)
  private budget = new Budget(1, 0.25, 1)
  private tap: AudioTap | null = null
  private media: (() => HTMLMediaElement | null) | null = null
  private left = new Float32Array(FFT_SIZE)
  private right = new Float32Array(FFT_SIZE)
  private raf = 0
  private lastFrame = 0
  private time = 0
  private flow = 0
  private fromId = '' // what's on screen
  private toId: string | null = null // what it's blending into
  private mix = 0
  private mixStyle = 0
  private mixFor = TRANSITION
  private shownFor = 0
  private wanted = false // someone wants it to run
  private visible = true // on screen, and its tab is too
  private saveTimer = 0
  private place: Place = 'panel'
  private frames = 0
  private statsAt = 0
  private frameCost = 0
  private observer: IntersectionObserver | null = null
  private sizer: ResizeObserver | null = null
  private slowFrames = 0

  constructor(host: VisualizerHost) {
    this.host = host
    this.canvas = this.makeCanvas()
    this.loadCustom()
    document.addEventListener('visibilitychange', this.onVisibility)
  }

  private makeCanvas() {
    const c = document.createElement('canvas')
    c.className = 'wv-canvas'
    c.setAttribute('aria-hidden', 'true')
    c.tabIndex = -1
    c.addEventListener('webglcontextlost', (e) => {
      e.preventDefault()
      if (c === this.canvas && this.renderer) this.onContextLost()
    })
    c.addEventListener('webglcontextrestored', () => { if (c === this.canvas && this.wanted) this.resumeLoop() })
    return c
  }

  // --- what the interface reads and changes

  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn) } }
  private emit() { this.listeners.forEach((l) => l()) }

  async load() {
    if (this.loaded) return
    try { this.settings = normalizeSettings(await this.host.loadSettings()) } catch { this.settings = DEFAULT_SETTINGS }
    this.loaded = true
    this.applySensitivity()
    this.emit()
  }

  get calm() { return this.settings.calm ?? calmBySystem() }

  update(patch: Partial<Settings>) {
    this.settings = normalizeSettings({ ...this.settings, ...patch })
    this.applySensitivity()
    if ('scale' in patch) this.resetBudget()
    // Save after the person has stopped moving a slider.
    clearTimeout(this.saveTimer)
    this.saveTimer = window.setTimeout(() => this.host.saveSettings(this.settings), 500)
    this.emit()
  }

  private applySensitivity() {
    this.analyzer.sensitivity = this.settings.sensitivity
    this.analyzer.minBeatGap = this.calm ? 0.5 : MIN_BEAT_GAP
  }

  private inRunning() {
    const usable = this.effects.filter((e) => !this.renderer || this.renderer.supports(e.id))
    const weak = this.renderer?.software || (this.budget.frameGap > 0 && this.budget.scale <= this.budget.floor + 1e-3)
    return eligible(usable, this.settings, this.calm, !!weak)
  }

  next() { this.change(this.cycle.next(this.inRunning().map((e) => e.id), this.settings.random)) }
  previous() { this.change(this.cycle.previous(this.inRunning().map((e) => e.id), this.settings.random)) }
  select(id: string) {
    if (!this.effects.some((e) => e.id === id)) return
    this.cycle.select(id)
    this.change(id)
    this.update({ effect: id })
  }

  toggleFavourite(id: string) {
    const f = this.settings.favourites
    this.update({ favourites: f.includes(id) ? f.filter((x) => x !== id) : [...f, id] })
  }

  // --- effects a person adds

  private loadCustom() {
    let list: string[] = []
    try { list = JSON.parse(localStorage.getItem(CUSTOM_KEY) || '[]') } catch { /* private window */ }
    if (!Array.isArray(list)) return
    const added: EffectFile[] = []
    for (const text of list.slice(0, MAX_CUSTOM)) {
      const r = typeof text === 'string' ? parseEffect(text, false) : null
      if (r?.ok && !BUILTIN_EFFECTS.some((b) => b.id === r.effect.id)) added.push(r.effect)
    }
    this.effects = [...BUILTIN_EFFECTS, ...added]
  }

  // Checks a file and tries it, away from the real picture. Returns why not, or null.
  async addCustom(text: string): Promise<string | null> {
    const r = parseEffect(text, false)
    if (!r.ok) return r.error
    if (BUILTIN_EFFECTS.some((e) => e.id === r.effect.id)) return `The name "${r.effect.id}" belongs to a built-in effect. Give yours a different "id".`
    const others = this.effects.filter((e) => !e.builtin && e.id !== r.effect.id)
    if (others.length >= MAX_CUSTOM) return `You can add up to ${MAX_CUSTOM} effects. Remove one first.`
    const why = await trialCompile(r.effect)
    if (why) return why
    this.effects = [...BUILTIN_EFFECTS, ...others, r.effect]
    this.saveCustom(text, r.effect.id)
    this.emit()
    return null
  }

  private saveCustom(text: string | null, id: string) {
    try {
      const list: string[] = JSON.parse(localStorage.getItem(CUSTOM_KEY) || '[]')
      const keep = list.filter((t) => { const p = parseEffect(t, false); return p.ok && p.effect.id !== id })
      localStorage.setItem(CUSTOM_KEY, JSON.stringify(text ? [...keep, text] : keep))
    } catch { /* private window: it lasts until the page closes */ }
  }

  removeCustom(id: string) {
    this.effects = this.effects.filter((e) => e.builtin || e.id !== id)
    this.saveCustom(null, id)
    if (this.fromId === id || this.toId === id) this.next()
    this.emit()
  }

  // --- turning on and off

  // Starts it, listening to this element. Nothing runs before this is called.
  start(media: () => HTMLMediaElement | null) {
    this.media = media
    this.wanted = true
    this.resumeLoop()
    this.emit()
  }

  // Call this from a press or a click, for browsers that hold sound back until one.
  async unlock() {
    await this.tap?.resume()
    this.status = { ...this.status, tap: this.tap?.state() ?? 'closed' }
    this.emit()
  }

  // Stops everything and lets go of the graphics card.
  stop() {
    this.wanted = false
    this.pauseLoop()
    // A context told to let go can't be used again, so the next time gets a new canvas.
    const r = this.renderer
    this.renderer = null
    this.observer?.disconnect()
    this.sizer?.disconnect()
    this.observer = this.sizer = null
    r?.dispose()
    this.canvas.remove()
    this.canvas = this.makeCanvas()
    this.visible = true
    this.host.saveSettings(this.settings)
    this.status = { ...this.status, running: false, tap: 'closed' }
    this.emit()
  }

  // Moves the canvas to where it's being shown.
  attach(el: HTMLElement, place: Place) {
    this.place = place
    if (this.canvas.parentElement !== el) el.prepend(this.canvas)
    this.observer?.disconnect()
    this.sizer?.disconnect()
    this.observer = new IntersectionObserver((list) => {
      const on = list[list.length - 1].isIntersecting
      if (on !== this.visible) { this.visible = on; on ? this.resumeLoop() : this.pauseLoop() }
    })
    this.observer.observe(this.canvas)
    this.sizer = new ResizeObserver(() => this.sizeCanvas())
    this.sizer.observe(el)
    this.resetBudget()
    this.sizeCanvas()
  }

  private onVisibility = () => {
    if (document.hidden) this.pauseLoop()
    else if (this.wanted && this.visible) this.resumeLoop()
  }

  private makeRenderer(): Renderer {
    const gl = createGl(this.canvas)
    if (gl) {
      try { return new GlRenderer(gl) } catch (e) { this.status = { ...this.status, problem: (e as Error).message } }
      // A canvas that has given itself to WebGL can't draw any other way.
      const fresh = this.makeCanvas()
      this.canvas.replaceWith(fresh)
      this.canvas = fresh
      return new CanvasRenderer(fresh)
    }
    return new CanvasRenderer(this.canvas)
  }

  private resetBudget() {
    const chosen = this.settings.scale
    const software = this.renderer?.software ?? false
    const start = chosen || (software ? 0.33 : 1)
    this.budget = new Budget(start, chosen || (software ? 0.2 : 0.25), chosen || 1)
    if (this.place === 'mini') this.budget = new Budget(1, 1, 1)
    if (software || this.place === 'mini') this.budget.frameGap = this.place === 'mini' ? 1 / 20 : 1 / 30
    this.sizeCanvas()
  }

  private sizeCanvas() {
    const r = this.renderer
    if (!r) return
    const box = this.canvas.parentElement?.getBoundingClientRect()
    if (!box || box.width < 1 || box.height < 1) return
    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    const fit = Math.min(1, LONG_EDGE / Math.max(box.width * dpr, box.height * dpr))
    const k = dpr * fit * this.budget.scale
    r.setSize(box.width * k, box.height * k)
  }

  private resumeLoop() {
    if (!this.wanted || this.raf || document.hidden || !this.visible) return
    if (!this.renderer) {
      this.renderer = this.makeRenderer()
      this.resetBudget()
      this.status = { ...this.status, renderer: this.renderer.kind, rendererDetail: this.renderer.label }
      if (this.fromId) this.prepareFor(this.fromId)
    }
    if (!this.tap) {
      this.tap = openTap(this.media?.() ?? null)
      this.status = { ...this.status, tap: this.tap.state(), tapWhy: this.tap.why() }
    }
    if (!this.fromId) {
      const saved = this.settings.effect
      const pinned = saved && this.settings.cycleSec === 0 && this.inRunning().some((e) => e.id === saved)
      this.fromId = pinned ? saved : this.cycle.next(this.inRunning().map((e) => e.id), this.settings.random)
      if (pinned) this.cycle.select(saved)
      this.status = { ...this.status, effect: this.fromId }
    }
    this.status = { ...this.status, running: true }
    this.lastFrame = performance.now()
    this.statsAt = this.lastFrame
    this.frames = 0
    this.raf = requestAnimationFrame(this.tick)
  }

  private pauseLoop() {
    if (this.raf) cancelAnimationFrame(this.raf)
    this.raf = 0
    // Closing the tap stops the browser's audio work too. It's opened again on the way back.
    this.tap?.close()
    this.tap = null
    this.status = { ...this.status, running: false }
  }

  private onContextLost() {
    this.pauseLoop()
    // A shader someone added is the likeliest reason, so it doesn't get a second go.
    const suspect = this.effects.find((e) => !e.builtin && (e.id === this.fromId || e.id === this.toId))
    if (suspect) {
      this.removeCustom(suspect.id)
      this.status = { ...this.status, problem: `"${suspect.name}" upset the graphics driver, so it has been removed.` }
    }
    this.renderer = null
    this.emit()
  }

  private prepareFor(id: string) {
    const e = this.effects.find((x) => x.id === id)
    if (!e || !this.renderer) return null
    const r = this.renderer.prepare(e)
    if (r && !r.ok) {
      this.status = { ...this.status, problem: `${e.name} couldn't start: ${r.error}` }
      this.effects = this.effects.filter((x) => x.id !== id)
      if (!e.builtin) this.saveCustom(null, id)
      this.emit()
    }
    return r
  }

  private change(id: string) {
    if (!id) return
    if (this.toId) this.finishMix()
    if (id === this.fromId) return
    this.toId = id
    this.mix = 0
    this.mixStyle = Math.floor(Math.random() * 4)
    this.mixFor = this.calm ? 4 : TRANSITION
    this.shownFor = 0
    this.status = { ...this.status, effect: id }
    this.emit()
  }

  private finishMix() {
    if (!this.toId) return
    const gone = this.fromId
    this.fromId = this.toId
    this.toId = null
    this.mix = 0
    this.renderer?.release(gone)
  }

  private tick = (now: number) => {
    this.raf = requestAnimationFrame(this.tick)
    const r = this.renderer, tap = this.tap
    if (!r || !tap) return
    const gap = this.budget.frameGap
    const since = (now - this.lastFrame) / 1000
    if (gap && since < gap - 0.002) return
    this.lastFrame = now
    const dt = Math.max(0.001, Math.min(0.25, since))
    const began = performance.now()

    tap.read(this.left, this.right)
    this.analyzer.process(this.left, this.right, tap.rate(), dt)
    const a = this.analyzer.frame
    const calm = this.calm, intensity = this.settings.intensity
    this.time += dt * (calm ? 0.6 : 1)
    this.flow += dt * (0.3 + 0.75 * a.level * intensity + 0.5 * a.beat) * (calm ? 0.45 : 1)
    this.palettes.step(dt, a.beatCount, calm)

    // Time for the next one?
    this.shownFor += dt
    const cycle = this.settings.cycleSec
    if (cycle > 0 && !this.toId && this.shownFor >= cycle) this.next()

    if (this.prepareFor(this.fromId) === null) return
    let incoming: string | null = null
    if (this.toId) {
      const ready = this.prepareFor(this.toId)
      if (ready?.ok) {
        incoming = this.toId
        this.mix = Math.min(1, this.mix + dt / this.mixFor)
      } else if (ready && !ready.ok) {
        this.toId = null
      }
    }
    r.draw({
      audio: a, time: this.time, flow: this.flow, dt, intensity, calm,
      palette: this.palettes.out,
      guard: calm ? GUARD_CALM : GUARD,
      a: this.fromId, b: incoming, mix: this.mix * this.mix * (3 - 2 * this.mix), mixStyle: this.mixStyle,
    })
    if (this.toId && this.mix >= 1) this.finishMix()

    if (this.place !== 'mini' && this.budget.frame(dt)) this.sizeCanvas()
    // Someone's effect that's far too slow for this machine has to go, whatever the scale.
    this.watchSlow(dt)

    this.frames++
    this.frameCost += performance.now() - began
    if (now - this.statsAt >= 1000) {
      const canvas = this.canvas
      const was = this.status
      const fps = (this.frames * 1000) / (now - this.statsAt)
      const ms = this.frameCost / Math.max(1, this.frames)
      this.statsAt = now
      this.frames = 0
      this.frameCost = 0
      const tapState = tap.state()
      this.status = { ...was, fps: Math.round(fps), width: canvas.width, height: canvas.height, scale: this.budget.scale, frameMs: Math.round(ms * 10) / 10, beats: a.beatCount, tap: tapState, tapWhy: tap.why() }
      this.emit()
    }
  }

  private watchSlow(dt: number) {
    const e = this.effects.find((x) => x.id === (this.toId ?? this.fromId))
    if (!e || e.builtin) return
    this.slowFrames = dt >= 0.2 ? this.slowFrames + 1 : 0
    if (this.slowFrames > 20) {
      this.slowFrames = 0
      this.status = { ...this.status, problem: `"${e.name}" was too slow on this device, so it has been skipped.` }
      this.cycle.select(e.id)
      this.next()
      this.effects = this.effects.filter((x) => x.id !== e.id)
      this.emit()
    }
  }
}
