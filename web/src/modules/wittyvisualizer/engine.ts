// What the visualizer decides, with no drawing in it: which effect comes next, when it
// changes, how colours move, and what resolution the machine can carry. Pure, so it can be
// tested, and qt/wvengine.cpp is the same in C++.

export type Settings = {
  notice: boolean // has read the notice about flashing
  on: boolean // shows with the full player
  effect: string | null // the last one, or a pinned one when cycling is off
  random: boolean
  cycleSec: number // 0 stays on one effect
  favourites: string[]
  favouritesOnly: boolean
  intensity: number // 0.3 to 1.5
  sensitivity: number // 0.25 to 2
  calm: boolean | null // null follows the system's reduced motion setting
  scale: number // 0 adapts by itself; otherwise the share of full resolution
  stats: boolean
}

export const DEFAULT_SETTINGS: Settings = {
  notice: false, on: false, effect: null, random: true, cycleSec: 30, favourites: [], favouritesOnly: false,
  intensity: 1, sensitivity: 1, calm: null, scale: 0, stats: false,
}

export const CYCLE_CHOICES = [0, 10, 15, 30, 60, 120, 300]
export const SCALE_CHOICES = [0, 1, 0.75, 0.5, 0.33]

const inRange = (v: unknown, lo: number, hi: number, fallback: number) => (typeof v === 'number' && Number.isFinite(v) ? Math.min(hi, Math.max(lo, v)) : fallback)

// Settings come back from a server and from storage, so they're read with care.
export function normalizeSettings(v: unknown): Settings {
  const s = (typeof v === 'object' && v !== null ? v : {}) as Record<string, unknown>
  const d = DEFAULT_SETTINGS
  return {
    notice: s.notice === true,
    on: s.on === true,
    effect: typeof s.effect === 'string' && s.effect.length <= 40 ? s.effect : d.effect,
    random: typeof s.random === 'boolean' ? s.random : d.random,
    cycleSec: CYCLE_CHOICES.includes(s.cycleSec as number) ? (s.cycleSec as number) : d.cycleSec,
    favourites: Array.isArray(s.favourites) ? s.favourites.filter((f) => typeof f === 'string' && f.length <= 40).slice(0, 64) : [],
    favouritesOnly: s.favouritesOnly === true,
    intensity: inRange(s.intensity, 0.3, 1.5, d.intensity),
    sensitivity: inRange(s.sensitivity, 0.25, 2, d.sensitivity),
    calm: typeof s.calm === 'boolean' ? s.calm : null,
    scale: SCALE_CHOICES.includes(s.scale as number) ? (s.scale as number) : d.scale,
    stats: s.stats === true,
  }
}

export type CycleItem = { id: string; motion: 'gentle' | 'lively'; cost: number }

// The effects that are in the running right now.
export function eligible(all: CycleItem[], s: Pick<Settings, 'favourites' | 'favouritesOnly'>, calm: boolean, weak: boolean) {
  let list = all
  if (calm) list = list.filter((e) => e.motion === 'gentle')
  if (s.favouritesOnly && s.favourites.length) {
    const f = list.filter((e) => s.favourites.includes(e.id))
    if (f.length) list = f
  }
  if (weak) {
    const cheap = list.filter((e) => e.cost < 3)
    if (cheap.length) list = cheap
  }
  return list.length ? list : all
}

// Moves through effects. In random order it plays each one before any comes round again,
// and never the same one twice in a row. A history makes "previous" really go back.
export class Cycle {
  private queue: string[] = []
  private past: string[] = []
  current: string | null = null
  private rand: () => number
  constructor(rand: () => number = Math.random) { this.rand = rand }

  private shuffled(ids: string[]) {
    const a = ids.filter((id) => id !== this.current)
    for (let i = a.length - 1; i > 0; i--) { const j = Math.floor(this.rand() * (i + 1)); const t = a[i]; a[i] = a[j]; a[j] = t }
    return a
  }

  next(ids: string[], random: boolean): string {
    if (!ids.length) return this.current || ''
    let n: string
    if (random) {
      this.queue = this.queue.filter((id) => ids.includes(id))
      if (!this.queue.length) this.queue = this.shuffled(ids)
      n = this.queue.shift() ?? ids[0]
    } else {
      const i = this.current === null ? -1 : ids.indexOf(this.current)
      n = ids[(i + 1) % ids.length]
    }
    if (this.current !== null) { this.past.push(this.current); if (this.past.length > 32) this.past.shift() }
    this.current = n
    return n
  }

  previous(ids: string[], random: boolean): string {
    if (!ids.length) return this.current || ''
    if (random) {
      while (this.past.length) {
        const p = this.past.pop()!
        if (ids.includes(p) && p !== this.current) { if (this.current) this.queue.unshift(this.current); this.current = p; return p }
      }
    }
    const i = this.current === null ? 0 : ids.indexOf(this.current)
    const n = ids[(Math.max(i, 0) - 1 + ids.length) % ids.length]
    this.current = n
    return n
  }

  select(id: string) {
    if (this.current !== null && this.current !== id) this.past.push(this.current)
    this.current = id
  }
}

// Holds the frame rate: fewer pixels first, and fewer frames only when that isn't enough.
// It watches the time between frames rather than the time a frame takes, as that's the
// only measure every browser and every graphics driver gives honestly.
export class Budget {
  scale: number
  frameGap = 0 // the least seconds between frames, 0 for as fast as the screen goes
  fps = 0
  private avg = 1 / 60
  private held = 0
  private calmFor = 0
  private counted = 0
  private countedFor = 0
  readonly floor: number
  readonly ceiling: number

  constructor(start: number, floor: number, ceiling: number) { this.scale = start; this.floor = floor; this.ceiling = ceiling }

  // Returns true when the resolution or the frame rate changed.
  frame(dt: number): boolean {
    this.counted++
    this.countedFor += dt
    if (this.countedFor >= 1) { this.fps = this.counted / this.countedFor; this.counted = 0; this.countedFor = 0 }
    // A long gap is a tab coming back or a window being dragged, not the visualizer.
    if (dt > 0.5) return false
    this.avg += (dt - this.avg) * 0.08
    this.held += dt
    if (this.held < 1.2) return false
    const slow = this.avg > 1 / 45, fast = this.avg < 1 / 54
    if (slow) {
      this.calmFor = 0
      this.held = 0
      if (this.scale > this.floor + 1e-3) {
        // A bigger step the further behind it is.
        this.scale = Math.max(this.floor, this.scale * (this.avg > 1 / 25 ? 0.7 : 0.85))
        return true
      }
      if (this.frameGap < 1 / 31) { this.frameGap = 1 / 30; return true }
      return false
    }
    if (fast && this.frameGap === 0) {
      this.calmFor += dt
      if (this.calmFor > 6 && this.scale < this.ceiling - 1e-3) {
        this.calmFor = 0
        this.held = 0
        this.scale = Math.min(this.ceiling, this.scale * 1.12)
        return true
      }
    }
    return false
  }
}

export type PaletteDef = { id: string; a: number[]; b: number[]; c: number[]; d: number[] }

// Moves between colour families with the music: a new one every so many beats, or after a
// while without any, always as a slow blend. Calm mode moves less often.
export class Palettes {
  private from: PaletteDef
  private to: PaletteDef
  private t = 1
  private beatsAtChange = 0
  private since = 0
  private list: PaletteDef[]
  private rand: () => number
  readonly out = { a: [0, 0, 0], b: [0, 0, 0], c: [0, 0, 0], d: [0, 0, 0] }
  constructor(list: PaletteDef[], rand: () => number = Math.random) {
    this.list = list
    this.rand = rand
    this.from = this.to = list[Math.floor(rand() * list.length)]
    this.blend()
  }
  private blend() {
    const k = this.t * this.t * (3 - 2 * this.t)
    for (const key of ['a', 'b', 'c', 'd'] as const) {
      for (let i = 0; i < 3; i++) this.out[key][i] = this.from[key][i] + (this.to[key][i] - this.from[key][i]) * k
    }
  }
  step(dt: number, beatCount: number, calm: boolean) {
    this.since += dt
    const everyBeats = calm ? 64 : 24, everySec = calm ? 60 : 25
    if (this.t >= 1 && (beatCount - this.beatsAtChange >= everyBeats || this.since >= everySec)) {
      this.beatsAtChange = beatCount
      this.since = 0
      this.from = this.to
      let n = this.to
      for (let i = 0; i < 8 && n === this.to; i++) n = this.list[Math.floor(this.rand() * this.list.length)]
      this.to = n
      this.t = 0
    }
    if (this.t < 1) {
      this.t = Math.min(1, this.t + dt / (calm ? 6 : 2.5))
      this.blend()
    }
  }
}
