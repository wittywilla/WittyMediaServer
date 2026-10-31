// How a controller's raw state becomes a console's buttons. No browser objects in
// here, so tests can feed it simulated pads. Which profile applies mirrors Resolve in
// modules/wittyplay/profiles.go; testdata/resolve.json keeps the two the same.

export const SELECT = 2, START = 3, UP = 4, DOWN = 5, LEFT = 6, RIGHT = 7, L2 = 12, R2 = 13
// Everything a controller can press: 16 buttons, then each stick's four directions.
export const STICK_LX_NEG = 16, TARGETS = 24

// Hat directions, as stored.
export const HAT_UP = 1, HAT_RIGHT = 2, HAT_DOWN = 3, HAT_LEFT = 4

export type Binding =
  | { b: number } // a button
  | { a: number; d: 1 | -1; r?: -1 | 0 | 1 } // one direction of an axis; r: where a trigger on an axis rests
  | { h: number; d: 1 | 2 | 3 | 4 } // one direction of a D-pad reported as a single axis
export type PadMap = Record<number, Binding | null>
export type KeyMap = Record<number, string>

export type ProfileOptions = { deadzone?: number; threshold?: number; stickDpad?: boolean; turbo?: number[]; turboHz?: number; noRumble?: boolean; calibration?: Record<number, [number, number, number]> }
export type Profile = {
  id: number; name: string; kind: 'gamepad' | 'keyboard'; controller: string; controllerName?: string; console?: string; game?: number; gameTitle?: string
  raw?: boolean; active: boolean; map?: PadMap; keys?: KeyMap; options: ProfileOptions; updatedAt?: number
}

export type RawLayout = { buttons: (Binding | null)[]; axes: number[] }
export type KnownPad = { id: string; name: string; style: string; layout?: string; linuxLayout?: string }
export type ControllerData = { layouts: Record<string, RawLayout>; pads: KnownPad[] }

// What's read of a browser's Gamepad, so that a test can make one.
export type PadLike = {
  id: string; index: number; mapping: string; connected?: boolean; timestamp?: number
  buttons: ArrayLike<{ pressed: boolean; value: number }>; axes: ArrayLike<number>
}

// ---- which model is this ----

// Browsers describe a pad in different ways: "Name (STANDARD GAMEPAD Vendor: 054c
// Product: 0ce6)", "Name (Vendor: 054c Product: 0ce6)", or "054c-ce6-Name".
const ID_LONG = /vendor:\s*([0-9a-f]{1,4})\s+product:\s*([0-9a-f]{1,4})/i
const ID_SHORT = /^([0-9a-f]{1,4})-([0-9a-f]{1,4})-/i

export function parseModel(id: string): { key: string; name: string; hasIds: boolean } {
  const m = ID_LONG.exec(id) || ID_SHORT.exec(id)
  const name = id.replace(/\s*\(.*?\)\s*/g, ' ').replace(ID_SHORT, '').trim() || 'Gamepad'
  if (m) return { key: `${m[1].toLowerCase().padStart(4, '0')}:${m[2].toLowerCase().padStart(4, '0')}`, name, hasIds: true }
  return { key: 'id:' + (name.slice(0, 116) || 'Gamepad'), name, hasIds: false }
}

export const isLinux = (ua: string) => /Linux/.test(ua) && !/Android/.test(ua)

export function knownPad(data: ControllerData | null, key: string): KnownPad | undefined {
  return data?.pads.find((p) => p.id === key)
}

// Unknown pads get first-to-last order, which is right more often than not, and the
// D-pad is found by looking at it. People are told to check.
const GENERIC: RawLayout = {
  axes: [0, 1, 2, 3],
  buttons: [{ b: 0 }, { b: 1 }, { b: 2 }, { b: 3 }, { b: 4 }, { b: 5 }, { b: 6 }, { b: 7 }, { b: 8 }, { b: 9 }, { b: 10 }, { b: 11 }],
}

export type Standing = 'standard' | 'known' | 'unknown'

// What's known about a pad: whether the browser put it in standard order, or we can,
// or neither. layout is null when its numbers are already standard.
export function standingOf(pad: PadLike, data: ControllerData | null, ua: string): { standing: Standing; layout: RawLayout | null; known?: KnownPad } {
  const { key } = parseModel(pad.id)
  const known = knownPad(data, key)
  if (pad.mapping === 'standard') return { standing: 'standard', layout: null, known }
  const name = known && ((isLinux(ua) && known.linuxLayout) || known.layout)
  const layout = name ? data?.layouts[name] : undefined
  if (layout) return { standing: 'known', layout, known }
  return { standing: 'unknown', layout: GENERIC, known }
}

// ---- D-pads that are axes ----

// A hat: one axis, -1 at up and on round clockwise in eight steps, and well outside
// that range at rest. Returns the four directions as bits: 1 up, 2 right, 4 down, 8 left.
export function hatBits(v: number): number {
  if (v === undefined || Math.abs(v) > 1.05) return 0
  const pos = Math.round((v + 1) * 3.5) & 7
  return [1, 3, 2, 6, 4, 12, 8, 9][pos]
}

const looksLikeHatAtRest = (v: number) => Math.abs(v) > 1.05

// Only a hat at rest gives itself away, and a pad can first be seen with its D-pad held
// (browsers show a pad once something on it is pressed), so what's seen is remembered.
const hats = new Set<string>()

// Notes a pad whose last axis shows itself to be a hat. True the first time, when what
// was worked out for its D-pad needs doing again.
export function noteHat(pad: PadLike): boolean {
  const n = pad.axes.length
  if (n === 0 || !looksLikeHatAtRest(pad.axes[n - 1])) return false
  const key = pad.index + '/' + pad.id
  if (hats.has(key)) return false
  hats.add(key)
  return true
}

// Where a pad with no D-pad buttons has its D-pad, as the four directions.
function findDpad(pad: PadLike): (Binding | null)[] {
  const n = pad.axes.length
  noteHat(pad)
  if (hats.has(pad.index + '/' + pad.id)) {
    return [{ h: n - 1, d: HAT_UP }, { h: n - 1, d: HAT_DOWN }, { h: n - 1, d: HAT_LEFT }, { h: n - 1, d: HAT_RIGHT }]
  }
  if (n >= 2) return [{ a: n - 1, d: -1 }, { a: n - 1, d: 1 }, { a: n - 2, d: -1 }, { a: n - 2, d: 1 }]
  return [null, null, null, null]
}

// ---- compiled mappings: a flat list, read once a frame without allocating ----

const SRC_BUTTON = 0, SRC_AXIS = 1, SRC_HAT = 2, HAT_BIT = [0, 1, 2, 4, 8]

type Source = { kind: number; i: number; d: number; r: number }
type Entry = { target: number; src: Source }

function toSource(b: Binding): Source {
  if ('b' in b) return { kind: SRC_BUTTON, i: b.b, d: 0, r: 0 }
  if ('h' in b) return { kind: SRC_HAT, i: b.h, d: HAT_BIT[b.d], r: 0 }
  return { kind: SRC_AXIS, i: b.a, d: b.d, r: b.r || 0 }
}

// A binding in standard numbers, as this particular pad really reports it.
export function throughLayout(b: Binding, pad: PadLike, layout: RawLayout | null): Binding | null {
  if (!layout) return b
  if ('b' in b) {
    const raw = layout.buttons[b.b]
    if (raw) return raw
    if (b.b >= 12 && b.b <= 15) return findDpad(pad)[b.b - 12]
    return null
  }
  if ('a' in b) {
    const raw = b.a < layout.axes.length ? layout.axes[b.a] : -1
    return raw < 0 ? null : { a: raw, d: b.d, r: b.r }
  }
  return b
}

export type Reading = {
  mask: number // bits 0 to 15: buttons, whichever way they were pressed
  lx: number; ly: number; rx: number; ry: number // -1 to 1
  l2: number; r2: number // 0 to 1
  held: Uint8Array // per target, for lighting up a picture
}
export const newReading = (): Reading => ({ mask: 0, lx: 0, ly: 0, rx: 0, ry: 0, l2: 0, r2: 0, held: new Uint8Array(TARGETS) })

export type Compiled = {
  entries: Entry[]
  deadzone: number
  threshold: number
  stickDpad: Source[] | null // x and y of the left stick, when it also steers the D-pad
  turbo: number // a bit per button
  turboHz: number
  calibration: Record<number, [number, number, number]> | null
  noRumble: boolean
  menu: (Source | null)[] // Select, Start and Home in standard numbers, for opening the menu
  prevMask: number // the last mask, for not flickering around the press point
}

export function compile(pad: PadLike, map: PadMap, layout: RawLayout | null, o: ProfileOptions): Compiled {
  const entries: Entry[] = []
  for (const [k, b] of Object.entries(map)) {
    if (!b) continue
    const raw = throughLayout(b, pad, layout)
    if (raw) entries.push({ target: Number(k), src: toSource(raw) })
  }
  const axis = (n: number) => { const r = throughLayout({ a: n, d: 1 }, pad, layout); return r ? toSource(r) : null }
  const x = axis(0), y = axis(1)
  const menuSrc = (n: number) => { const r = throughLayout({ b: n }, pad, layout); return r ? toSource(r) : null }
  let turbo = 0
  for (const id of o.turbo || []) turbo |= 1 << id
  return {
    entries, deadzone: o.deadzone ?? 0.15, threshold: o.threshold ?? 0.5, stickDpad: o.stickDpad && x && y ? [x, y] : null,
    turbo, turboHz: o.turboHz ?? 12, calibration: o.calibration && Object.keys(o.calibration).length ? o.calibration : null, noRumble: !!o.noRumble,
    menu: [menuSrc(8), menuSrc(9), menuSrc(16)], prevMask: 0,
  }
}

function calibrated(v: number, c: [number, number, number] | undefined): number {
  if (!c) return v
  const out = v < c[1] ? (v - c[1]) / (c[1] - c[0]) : (v - c[1]) / (c[2] - c[1])
  return Math.max(-1, Math.min(1, out))
}

// How far a source is pressed, 0 to 1.
export function sourceValue(s: Source, pad: PadLike, cal: Compiled['calibration']): number {
  if (s.kind === SRC_BUTTON) {
    const b = pad.buttons[s.i]
    return !b ? 0 : b.value > 0 ? b.value : b.pressed ? 1 : 0
  }
  const v = pad.axes[s.i]
  if (v === undefined) return 0
  if (s.kind === SRC_HAT) return hatBits(v) & s.d ? 1 : 0
  if (s.r !== 0) {
    // Some browsers report a trigger at exactly 0 until it's first touched, and that
    // mustn't read as half pressed.
    if (v === 0) return 0
    return Math.max(0, Math.min(1, s.r < 0 ? (v + 1) / 2 : (1 - v) / 2))
  }
  const c = cal ? calibrated(v, cal[s.i]) : v
  return Math.max(0, s.d > 0 ? c : -c)
}

// A stick's two axes as one: nothing inside the dead zone, then the whole travel from
// its edge, so a stick reaches full speed and small movements still count.
function radial(r: Reading, xKey: 'lx' | 'rx', yKey: 'ly' | 'ry', dz: number) {
  const x = r[xKey], y = r[yKey]
  const mag = Math.hypot(x, y)
  if (mag <= dz) { r[xKey] = 0; r[yKey] = 0; return }
  const scale = (Math.min(1, mag) - dz) / (1 - dz) / mag
  r[xKey] = x * scale
  r[yKey] = y * scale
}

// Reads a pad into a Reading. Allocates nothing: this runs every frame.
export function readPad(c: Compiled, pad: PadLike, now: number, out: Reading) {
  let mask = 0, lxn = 0, lxp = 0, lyn = 0, lyp = 0, rxn = 0, rxp = 0, ryn = 0, ryp = 0, l2 = 0, r2 = 0
  const prev = c.prevMask, thr = c.threshold, cal = c.calibration, held = out.held
  held.fill(0)
  for (let n = 0; n < c.entries.length; n++) {
    const e = c.entries[n], t = e.target
    const v = sourceValue(e.src, pad, cal)
    if (t < 16) {
      // A little easier to keep held than to press, so a value near the point doesn't flicker.
      if (v > (prev & (1 << t) ? thr * 0.8 : thr)) { mask |= 1 << t; held[t] = 1 }
      if (t === L2 && v > l2) l2 = v
      else if (t === R2 && v > r2) r2 = v
    } else {
      if (v > thr) held[t] = 1
      switch (t) {
        case 16: if (v > lxn) lxn = v; break
        case 17: if (v > lxp) lxp = v; break
        case 18: if (v > lyn) lyn = v; break
        case 19: if (v > lyp) lyp = v; break
        case 20: if (v > rxn) rxn = v; break
        case 21: if (v > rxp) rxp = v; break
        case 22: if (v > ryn) ryn = v; break
        case 23: if (v > ryp) ryp = v; break
      }
    }
  }
  out.lx = lxp - lxn; out.ly = lyp - lyn; out.rx = rxp - rxn; out.ry = ryp - ryn
  out.l2 = l2; out.r2 = r2
  radial(out, 'lx', 'ly', c.deadzone)
  radial(out, 'rx', 'ry', c.deadzone)
  // The left stick steering the D-pad too, for consoles that have nothing else to steer with.
  if (c.stickDpad) {
    let x = sourceSigned(c.stickDpad[0], pad, cal), y = sourceSigned(c.stickDpad[1], pad, cal)
    if (Math.hypot(x, y) <= c.deadzone) { x = 0; y = 0 }
    const hold = (bit: number, v: number) => { if (v > (prev & (1 << bit) ? thr * 0.8 : thr)) { mask |= 1 << bit; held[bit] = 1 } }
    hold(LEFT, -x); hold(RIGHT, x); hold(UP, -y); hold(DOWN, y)
  }
  c.prevMask = mask
  if (c.turbo && mask & c.turbo) {
    if (Math.floor((now * c.turboHz * 2) / 1000) % 2) mask &= ~c.turbo
  }
  out.mask = mask
}

function sourceSigned(s: Source, pad: PadLike, cal: Compiled['calibration']): number {
  const v = pad.axes[s.i]
  if (v === undefined) return 0
  return cal ? calibrated(v, cal[s.i]) : v
}

// Whether the menu combination is held: Select with Start, or Home.
export function menuHeld(c: Compiled, pad: PadLike): boolean {
  const on = (s: Source | null) => !!s && sourceValue(s, pad, null) > 0.5
  return (on(c.menu[0]) && on(c.menu[1])) || on(c.menu[2])
}

// ---- pressing to choose ----

export type Snapshot = { buttons: boolean[]; axes: number[] }
const down = (b: { pressed: boolean; value: number } | undefined) => !!b && (b.pressed || b.value > 0.5)

export function snapshot(pad: PadLike): Snapshot {
  const buttons: boolean[] = []
  for (let i = 0; i < pad.buttons.length; i++) buttons.push(down(pad.buttons[i]))
  return { buttons, axes: Array.from(pad.axes as ArrayLike<number>) }
}

// Forgets what's been let go since listening began, so that the very button a person
// pressed to get here can be the answer, once they've released it and pressed it again.
export function noteReleases(pad: PadLike, base: Snapshot) {
  for (let i = 0; i < base.buttons.length; i++) if (base.buttons[i] && !down(pad.buttons[i])) base.buttons[i] = false
}

// The first new press, in raw numbers. base holds what was already held or at rest when
// listening began, so nothing a person was already holding is taken for an answer.
export function findPress(pad: PadLike, base: Snapshot): Binding | null {
  for (let i = 0; i < pad.buttons.length; i++) {
    if (down(pad.buttons[i]) && !base.buttons[i]) return { b: i }
  }
  for (let i = 0; i < pad.axes.length; i++) {
    const v = pad.axes[i], rest = base.axes[i] ?? 0
    if (looksLikeHatAtRest(rest)) { // a hat at rest
      const bits = hatBits(v)
      if (bits & 1) return { h: i, d: HAT_UP }
      if (bits & 2) return { h: i, d: HAT_RIGHT }
      if (bits & 4) return { h: i, d: HAT_DOWN }
      if (bits & 8) return { h: i, d: HAT_LEFT }
      continue
    }
    if (Math.abs(v - rest) < 0.5) continue
    if (Math.abs(rest) > 0.8) { // a trigger at one end, travelling towards the other
      if (Math.abs(v - rest) > 0.6) return { a: i, d: rest < 0 ? 1 : -1, r: rest < 0 ? -1 : 1 }
      continue
    }
    if (Math.abs(v) > 0.6) return { a: i, d: v > 0 ? 1 : -1 }
  }
  return null
}

// The console's defaults written in a pad's own numbers, for a pad whose buttons we can
// only guess at: a profile for it has to say where everything is.
export function rawFromDefaults(defaults: PadMap, pad: PadLike): PadMap {
  const out: PadMap = {}
  for (const [k, b] of Object.entries(defaults)) {
    if (!b) continue
    const raw = throughLayout(b, pad, GENERIC)
    if (raw) out[Number(k)] = raw
  }
  return out
}

// A raw press in the numbers a profile stores: standard ones if this pad has a layout
// that reaches the press, and raw otherwise (the profile is then marked raw).
export function toStandard(b: Binding, layout: RawLayout | null, pad: PadLike): Binding | null {
  if (!layout) return b
  const same = (x: Binding | null, y: Binding) => !!x && JSON.stringify(canon(x)) === JSON.stringify(canon(y))
  for (let i = 0; i < 17; i++) {
    const raw = throughLayout({ b: i }, pad, layout)
    if (same(raw, b)) return { b: i }
  }
  if ('a' in b) {
    const n = layout.axes.indexOf(b.a)
    if (n >= 0) return { a: n, d: b.d }
  }
  return null
}

const canon = (b: Binding) => ('b' in b ? { b: b.b } : 'h' in b ? { h: b.h, d: b.d } : { a: b.a, d: b.d })

// ---- which profile applies ----

export function resolveProfile(list: Profile[], kind: string, controller: string, consoleId: string, game: number, raw: boolean): Profile | null {
  let best: Profile | null = null, bestRank = 0
  for (const p of list) {
    if (!p.active || p.kind !== kind) continue
    if (kind === 'gamepad' && !!p.raw !== raw) continue
    if (p.controller && p.controller !== controller) continue
    if (p.console && p.console !== consoleId) continue
    if (p.game && p.game !== game) continue
    const named = !!p.controller
    const rank = p.game ? (named ? 6 : 5) : p.console ? (named ? 4 : 3) : named ? 2 : 1
    if (rank > bestRank || (rank === bestRank && (p.updatedAt || 0) > (best?.updatedAt || 0))) { best = p; bestRank = rank }
  }
  return best
}

// What each target is pressed by: the console's default, changed by the profile. A
// profile of raw numbers starts from nothing, as the defaults use standard numbers.
export function effectiveMap(defaults: PadMap, known: Set<number>, p: Profile | null): PadMap {
  const out: PadMap = p?.raw ? {} : { ...defaults }
  for (const [k, b] of Object.entries(p?.map || {})) {
    const id = Number(k)
    if (b === null) delete out[id]
    else if (known.has(id)) out[id] = b
  }
  return out
}

// A key does one thing: one the profile uses leaves the default it would also press.
export function effectiveKeys(defaults: KeyMap, known: Set<number>, p: Profile | null): KeyMap {
  const out: KeyMap = { ...defaults }
  for (const [k, code] of Object.entries(p?.keys || {})) {
    const id = Number(k)
    if (!known.has(id)) continue
    for (const [o, v] of Object.entries(out)) if (v === code) delete out[Number(o)]
    out[id] = code
  }
  return out
}

export function effectiveOptions(defaults: ProfileOptions, p: Profile | null): ProfileOptions {
  if (!p) return defaults
  const o = p.options || {}
  return { ...defaults, deadzone: o.deadzone || defaults.deadzone, threshold: o.threshold || defaults.threshold, stickDpad: o.stickDpad ?? defaults.stickDpad, turboHz: o.turboHz || defaults.turboHz, turbo: o.turbo, noRumble: o.noRumble, calibration: o.calibration }
}

// Which targets share a source: a button that does two things is allowed, but shown.
export function conflicts(map: PadMap): Map<number, number[]> {
  const seen = new Map<string, number[]>()
  for (const [k, b] of Object.entries(map)) {
    if (!b) continue
    const key = JSON.stringify(canon(b))
    seen.set(key, [...(seen.get(key) || []), Number(k)])
  }
  const out = new Map<number, number[]>()
  for (const ids of seen.values()) {
    if (ids.length < 2) continue
    for (const id of ids) out.set(id, ids.filter((x) => x !== id))
  }
  return out
}

export function keyConflicts(map: KeyMap): Map<number, number[]> {
  const asPad: PadMap = {}
  const codes = [...new Set(Object.values(map))]
  for (const [k, c] of Object.entries(map)) asPad[Number(k)] = { b: codes.indexOf(c) }
  return conflicts(asPad)
}

// ---- words ----

const STD_NAMES: Record<string, string[]> = {
  xbox: ['A', 'B', 'X', 'Y', 'LB', 'RB', 'LT', 'RT', 'View', 'Menu', 'Left stick click', 'Right stick click', 'D-pad up', 'D-pad down', 'D-pad left', 'D-pad right', 'Home'],
  playstation: ['Cross', 'Circle', 'Square', 'Triangle', 'L1', 'R1', 'L2', 'R2', 'Share', 'Options', 'L3', 'R3', 'D-pad up', 'D-pad down', 'D-pad left', 'D-pad right', 'Home'],
  nintendo: ['B', 'A', 'Y', 'X', 'L', 'R', 'ZL', 'ZR', 'Minus', 'Plus', 'Left stick click', 'Right stick click', 'D-pad up', 'D-pad down', 'D-pad left', 'D-pad right', 'Home'],
  generic: ['Bottom button', 'Right button', 'Left button', 'Top button', 'Left bumper', 'Right bumper', 'Left trigger', 'Right trigger', 'Select', 'Start', 'Left stick click', 'Right stick click', 'D-pad up', 'D-pad down', 'D-pad left', 'D-pad right', 'Home'],
}
const STICK_NAMES = [['Left stick left', 'Left stick right'], ['Left stick up', 'Left stick down'], ['Right stick left', 'Right stick right'], ['Right stick up', 'Right stick down']]
const HAT_NAMES = ['', 'D-pad up', 'D-pad right', 'D-pad down', 'D-pad left']

export function describeSource(b: Binding | null | undefined, style = 'generic', raw = false): string {
  if (!b) return 'Not set'
  if ('h' in b) return HAT_NAMES[b.d]
  if (raw) return 'b' in b ? `Button ${b.b}` : `Axis ${b.a} ${b.d > 0 ? 'plus' : 'minus'}`
  if ('b' in b) return (STD_NAMES[style] || STD_NAMES.generic)[b.b] || `Button ${b.b}`
  const name = STICK_NAMES[b.a]?.[b.d > 0 ? 1 : 0]
  return name || `Axis ${b.a} ${b.d > 0 ? 'plus' : 'minus'}`
}

// "KeyZ" -> "Z", "ArrowUp" -> "Up"
export const describeKey = (code?: string) => (!code ? 'Not set' : code.replace(/^Key/, '').replace(/^Digit/, '').replace(/^Arrow/, ''))
