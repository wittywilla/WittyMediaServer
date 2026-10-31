import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { afterEach, before, beforeEach, describe, test } from 'node:test'
import type { ControlsData } from '../consoles'
import {
  compile, conflicts, describeSource, effectiveKeys, effectiveMap, effectiveOptions, findPress, hatBits, menuHeld, newReading, noteHat, parseModel, rawFromDefaults, readPad, resolveProfile, snapshot, standingOf, toStandard,
  type ControllerData, type PadLike, type PadMap, type Profile,
} from '../mapping'

// Simulated pads, fed to the real code. Run with: npm run test:play (in web/)

const button = (pressed = false, value = pressed ? 1 : 0) => ({ pressed, value })
function makePad(o: Partial<PadLike> & { nb?: number; na?: number } = {}): PadLike & { buttons: { pressed: boolean; value: number }[]; axes: number[] } {
  const buttons = Array.from({ length: o.nb ?? 17 }, () => button())
  const axes = Array.from({ length: o.na ?? 4 }, () => 0)
  return { id: 'Test pad (STANDARD GAMEPAD Vendor: 054c Product: 0ce6)', index: 0, mapping: 'standard', connected: true, timestamp: 1, ...o, buttons, axes } as any
}

const STANDARD: PadMap = { 0: { b: 0 }, 1: { b: 2 }, 2: { b: 8 }, 3: { b: 9 }, 4: { b: 12 }, 5: { b: 13 }, 6: { b: 14 }, 7: { b: 15 }, 8: { b: 1 }, 9: { b: 3 }, 10: { b: 4 }, 11: { b: 5 }, 12: { b: 6 }, 13: { b: 7 } }
const STICKS: PadMap = { ...STANDARD, 16: { a: 0, d: -1 }, 17: { a: 0, d: 1 }, 18: { a: 1, d: -1 }, 19: { a: 1, d: 1 }, 20: { a: 2, d: -1 }, 21: { a: 2, d: 1 }, 22: { a: 3, d: -1 }, 23: { a: 3, d: 1 } }
const read = (pad: PadLike, map: PadMap, opts = {}, layout = null as any, now = 0) => { const c = compile(pad, map, layout, opts); const r = newReading(); readPad(c, pad, now, r); return { r, c } }

describe('reading a pad', () => {
  test('buttons by position, and analog triggers by how far', () => {
    const p = makePad()
    p.buttons[0] = button(true)
    p.buttons[6] = button(true, 0.25)
    const { r } = read(p, STANDARD, { threshold: 0.2 })
    assert.ok(r.mask & (1 << 0))
    assert.ok(r.mask & (1 << 12))
    assert.equal(r.l2, 0.25)
    assert.equal(r.r2, 0)
  })

  test('a trigger only counts past the press point, and stays held a little below it', () => {
    const p = makePad()
    p.buttons[7] = button(false, 0.45)
    const c = compile(p, STANDARD, null, { threshold: 0.5 })
    const r = newReading()
    readPad(c, p, 0, r)
    assert.equal(r.mask & (1 << 13), 0)
    p.buttons[7] = button(false, 0.55)
    readPad(c, p, 0, r)
    assert.ok(r.mask & (1 << 13), 'pressed')
    p.buttons[7] = button(false, 0.45)
    readPad(c, p, 0, r)
    assert.ok(r.mask & (1 << 13), 'still held: no flicker around the point')
    p.buttons[7] = button(false, 0.3)
    readPad(c, p, 0, r)
    assert.equal(r.mask & (1 << 13), 0, 'let go')
  })

  test('sticks: nothing in the dead zone, full travel outside it, in a circle', () => {
    const p = makePad()
    p.axes[0] = 0.1; p.axes[1] = 0.1
    let { r } = read(p, STICKS, { deadzone: 0.15 })
    assert.equal(r.lx, 0)
    assert.equal(r.ly, 0)
    p.axes[0] = 1; p.axes[1] = 0
    r = read(p, STICKS, { deadzone: 0.15 }).r
    assert.equal(Math.round(r.lx * 1000), 1000)
    p.axes[0] = 0.2; p.axes[1] = 0
    r = read(p, STICKS, { deadzone: 0.15 }).r
    assert.ok(r.lx > 0 && r.lx < 0.1, 'just past the edge is a small value, not a jump')
    // A stick on the diagonal keeps its direction.
    p.axes[0] = -0.5; p.axes[1] = 0.5
    r = read(p, STICKS, { deadzone: 0.15 }).r
    assert.ok(Math.abs(r.lx + r.ly) < 1e-9)
    assert.ok(r.lx < 0 && r.ly > 0)
  })

  test('the left stick steers the D-pad only if asked', () => {
    const p = makePad()
    p.axes[0] = -0.9; p.axes[1] = 0.9
    let { r } = read(p, STANDARD, { stickDpad: true })
    assert.ok(r.mask & (1 << 6) && r.mask & (1 << 5), 'left and down')
    assert.equal(r.lx, 0, 'a console with no stick is sent none')
    r = read(p, STICKS, { stickDpad: false }).r
    assert.equal(r.mask & (1 << 6), 0)
    assert.ok(r.lx < 0)
  })

  test('stick to button, and button to stick', () => {
    const p = makePad()
    p.axes[2] = 0.9
    p.buttons[15] = button(true)
    const { r } = read(p, { 8: { a: 2, d: 1 }, 17: { b: 15 } }, { threshold: 0.5, deadzone: 0.15 })
    assert.ok(r.mask & (1 << 8), 'a stick pressed a button')
    assert.equal(Math.round(r.lx), 1, 'a button pushed a stick all the way')
    assert.ok(r.held[17])
  })

  test('turbo repeats while held, at its rate, and only what was asked', () => {
    const p = makePad()
    p.buttons[0] = button(true)
    p.buttons[1] = button(true)
    const c = compile(p, STANDARD, null, { turbo: [0], turboHz: 10 })
    const r = newReading()
    const seen = new Set<boolean>()
    for (let t = 0; t < 1000; t += 16) {
      readPad(c, p, t, r)
      seen.add(!!(r.mask & 1))
      assert.ok(r.mask & (1 << 8), 'the other stays held')
    }
    assert.equal(seen.size, 2, 'on and off')
    readPad(c, p, 0, r)
    const first = !!(r.mask & 1)
    readPad(c, p, 50, r) // half of one press at 10 a second
    assert.notEqual(!!(r.mask & 1), first)
  })

  test('calibration centres a stick that rests off centre', () => {
    const p = makePad()
    p.axes[0] = 0.2
    const cal = { 0: [-0.8, 0.2, 0.9] as [number, number, number] }
    let { r } = read(p, STICKS, { calibration: cal, deadzone: 0.1 })
    assert.equal(r.lx, 0, 'at its own rest it reads as centred')
    p.axes[0] = 0.9
    r = read(p, STICKS, { calibration: cal, deadzone: 0.1 }).r
    assert.equal(Math.round(r.lx * 100), 100, 'its own far end reads as all the way')
  })

  test('a missing button or axis reads as nothing', () => {
    const p = makePad({ nb: 2, na: 0 })
    const { r } = read(p, STICKS, {})
    assert.equal(r.mask, 0)
    assert.equal(r.lx, 0)
  })

  test('the menu combination, for pads in any order', () => {
    const p = makePad()
    const c = compile(p, STANDARD, null, {})
    assert.equal(menuHeld(c, p), false)
    p.buttons[8] = button(true)
    assert.equal(menuHeld(c, p), false)
    p.buttons[9] = button(true)
    assert.equal(menuHeld(c, p), true)
    const q = makePad()
    q.buttons[16] = button(true)
    assert.equal(menuHeld(compile(q, STANDARD, null, {}), q), true)
  })
})

describe('non-standard pads', () => {
  const known: ControllerData = {
    layouts: {
      x: { axes: [0, 1, 3, 4], buttons: [{ b: 0 }, { b: 1 }, { b: 2 }, { b: 3 }, { b: 4 }, { b: 5 }, { a: 2, d: 1, r: -1 }, { a: 5, d: 1, r: -1 }, { b: 6 }, { b: 7 }, { b: 9 }, { b: 10 }, { a: 7, d: -1 }, { a: 7, d: 1 }, { a: 6, d: -1 }, { a: 6, d: 1 }, { b: 8 }] },
      d: { axes: [0, 1, 2, 3], buttons: [{ b: 1 }, { b: 2 }, { b: 0 }, { b: 3 }, { b: 4 }, { b: 5 }, { b: 6 }, { b: 7 }, { b: 8 }, { b: 9 }, { b: 10 }, { b: 11 }, null, null, null, null, { b: 12 }] },
    },
    pads: [
      { id: '045e:028e', name: 'Xbox 360 pad', style: 'xbox', linuxLayout: 'x' },
      { id: '046d:c216', name: 'Pad in D mode', style: 'generic', layout: 'd' },
    ],
  }
  const LINUX = 'Mozilla/5.0 (X11; Linux x86_64)', WIN = 'Mozilla/5.0 (Windows NT 10.0)'

  test('browsers name the same model differently', () => {
    assert.equal(parseModel('DualSense Wireless Controller (STANDARD GAMEPAD Vendor: 054c Product: 0ce6)').key, '054c:0ce6')
    assert.equal(parseModel('054c-0ce6-DualSense Wireless Controller').key, '054c:0ce6')
    assert.equal(parseModel('54c-ce6-DualSense').key, '054c:0ce6')
    assert.equal(parseModel('USB Gamepad (Vendor: 0079 Product: 0006)').name, 'USB Gamepad')
    assert.equal(parseModel('Xbox 360 Controller (XInput STANDARD GAMEPAD)').key, 'id:Xbox 360 Controller')
    assert.equal(parseModel('xinput').key, 'id:xinput')
  })

  test('what a browser calls standard is left alone', () => {
    const p = makePad({ id: '045e-028e-Pad', mapping: 'standard' })
    const s = standingOf(p, known, LINUX)
    assert.equal(s.standing, 'standard')
    assert.equal(s.layout, null)
    assert.equal(s.known?.style, 'xbox')
  })

  test('a known pad on Linux: triggers and the D-pad are axes', () => {
    const p = makePad({ id: '045e-028e-Microsoft X-Box 360 pad', mapping: '', nb: 11, na: 8 })
    const s = standingOf(p, known, LINUX)
    assert.equal(s.standing, 'known')
    p.axes[2] = -1; p.axes[5] = -1 // triggers at rest
    p.axes[7] = 1 // D-pad down
    p.buttons[1] = button(true) // right face button
    p.axes[3] = 0.9 // right stick X
    const { r } = read(p, STICKS, { threshold: 0.5 }, s.layout)
    assert.ok(r.mask & (1 << 5), 'D-pad down')
    assert.ok(r.mask & (1 << 8), 'A, by position')
    assert.equal(r.mask & (1 << 12), 0, 'a trigger at rest is not pressed')
    assert.ok(r.rx > 0)
    // Fully pressed, then half.
    p.axes[2] = 1
    assert.ok(read(p, STICKS, {}, s.layout).r.mask & (1 << 12))
    p.axes[2] = 0.1
    const half = read(p, STICKS, {}, s.layout).r
    assert.ok(half.l2 > 0.5 && half.l2 < 0.6)
  })

  test('a trigger reading exactly 0 before it is first touched is not half pressed', () => {
    const p = makePad({ id: '045e-028e-Pad', mapping: '', nb: 11, na: 8 })
    const s = standingOf(p, known, LINUX)
    const { r } = read(p, STICKS, {}, s.layout)
    assert.equal(r.mask & (1 << 12), 0)
    assert.equal(r.l2, 0)
  })

  test('the same model is not known without a layout for this system', () => {
    const p = makePad({ id: '045e-028e-Pad', mapping: '', nb: 11, na: 8 })
    assert.equal(standingOf(p, known, WIN).standing, 'unknown')
  })

  test('a pad in D mode: faces reordered, and a hat found by looking', () => {
    const p = makePad({ id: 'Pad (Vendor: 046d Product: c216)', mapping: '', nb: 13, na: 10 })
    p.axes[9] = 3.2857 // a hat at rest
    const s = standingOf(p, known, WIN)
    assert.equal(s.standing, 'known')
    p.buttons[1] = button(true) // the second button is the bottom one here
    let { r } = read(p, STANDARD, {}, s.layout)
    assert.ok(r.mask & (1 << 0), 'RetroPad B')
    p.buttons[1] = button()
    p.axes[9] = -1 // hat up
    r = read(p, STANDARD, {}, s.layout).r
    assert.equal(r.mask, 1 << 4, 'only up')
    p.axes[9] = -0.7143 // up and right
    r = read(p, STANDARD, {}, s.layout).r
    assert.equal(r.mask, (1 << 4) | (1 << 7))
    p.axes[9] = 3.2857
    r = read(p, STANDARD, {}, s.layout).r
    assert.equal(r.mask, 0)
  })

  test('a pad first seen with its hat held still finds it, once it comes to rest', () => {
    const p = makePad({ id: '2222-3333-Hat pad', mapping: '', nb: 13, na: 10 })
    p.axes[9] = 0.1429 // down, and the browser only now shows the pad
    const s = standingOf(p, known, WIN)
    const c = compile(p, STANDARD, s.layout, {})
    p.axes[9] = 3.2857
    assert.equal(noteHat(p), true, 'it shows itself')
    assert.equal(noteHat(p), false, 'and is only news once')
    const again = compile(p, STANDARD, s.layout, {})
    assert.notDeepEqual(again.entries, c.entries)
    p.axes[9] = 0.1429
    const r = newReading()
    readPad(again, p, 0, r)
    assert.equal(r.mask, 1 << 5, 'down, and held down during a rebuild stays right')
  })

  test('hat readings round the clock, and at rest', () => {
    assert.deepEqual([-1, -0.7143, -0.4286, -0.1429, 0.1429, 0.4286, 0.7143, 1].map(hatBits), [1, 3, 2, 6, 4, 12, 8, 9])
    assert.equal(hatBits(3.2857), 0)
    assert.equal(hatBits(1.2857), 0)
  })

  test('an unknown pad gets a guess, with its D-pad on its last axes', () => {
    const p = makePad({ id: '1234-5678-Mystery', mapping: '', nb: 10, na: 2 })
    const s = standingOf(p, known, WIN)
    assert.equal(s.standing, 'unknown')
    p.axes[1] = -1
    p.buttons[0] = button(true)
    const { r } = read(p, STANDARD, {}, s.layout)
    assert.ok(r.mask & (1 << 4), 'up')
    assert.ok(r.mask & (1 << 0), 'first button')
    // Its defaults, in its own numbers, ready to go in a profile.
    const raw = rawFromDefaults(STANDARD, p)
    assert.deepEqual(raw[4], { a: 1, d: -1 })
    assert.deepEqual(raw[0], { b: 0 })
  })
})

describe('pressing to choose', () => {
  test('the first new press, ignoring what was already held', () => {
    const p = makePad()
    p.buttons[3] = button(true)
    const base = snapshot(p)
    assert.equal(findPress(p, base), null)
    p.buttons[5] = button(true)
    assert.deepEqual(findPress(p, base), { b: 5 })
  })

  test('a stick, once it has moved far enough', () => {
    const p = makePad()
    const base = snapshot(p)
    p.axes[1] = -0.3
    assert.equal(findPress(p, base), null)
    p.axes[1] = -0.8
    assert.deepEqual(findPress(p, base), { a: 1, d: -1 })
  })

  test('a trigger on an axis remembers where it rests', () => {
    const p = makePad({ na: 6, mapping: '' })
    p.axes[2] = -1
    const base = snapshot(p)
    p.axes[2] = 0.2
    assert.deepEqual(findPress(p, base), { a: 2, d: 1, r: -1 })
  })

  test('a hat is taken as a hat', () => {
    const p = makePad({ na: 10, mapping: '' })
    p.axes[9] = 3.2857
    const base = snapshot(p)
    p.axes[9] = 0.1429
    assert.deepEqual(findPress(p, base), { h: 9, d: 3 })
  })

  test('a known pad is stored in standard numbers, so a profile works in any browser', () => {
    const layout = { axes: [0, 1, 3, 4], buttons: [{ b: 0 }, { b: 1 }, { b: 3 }, { b: 2 }, { b: 4 }, { b: 5 }, { a: 2, d: 1 as const, r: -1 as const }] }
    const p = makePad({ mapping: '', na: 8 })
    assert.deepEqual(toStandard({ b: 3 }, layout, p), { b: 2 })
    assert.deepEqual(toStandard({ a: 2, d: 1, r: -1 }, layout, p), { b: 6 })
    assert.deepEqual(toStandard({ a: 3, d: -1 }, layout, p), { a: 2, d: -1 })
  })
})

describe('which profile applies', () => {
  test('the same answers as the server', () => {
    const f = JSON.parse(readFileSync(new URL('../../testdata/resolve.json', import.meta.url), 'utf8'))
    for (const c of f.cases) {
      const got = resolveProfile(f.profiles, c.kind, c.controller, c.console, c.game, !!c.raw)
      assert.equal(got?.id ?? 0, c.want, c.why)
    }
  })

  test('a profile changes only what it says; null clears', () => {
    const p = { map: { 0: { b: 7 }, 2: null, 20: { b: 1 } } } as unknown as Profile
    const m = effectiveMap(STANDARD, new Set(Object.keys(STANDARD).map(Number)), p)
    assert.deepEqual(m[0], { b: 7 })
    assert.equal(m[2], undefined)
    assert.deepEqual(m[8], { b: 1 })
    assert.equal(m[20], undefined, 'not on this console')
  })

  test('a key does one thing', () => {
    const k = effectiveKeys({ 0: 'KeyZ', 8: 'KeyX' }, new Set([0, 8]), { keys: { 0: 'KeyX' } } as unknown as Profile)
    assert.deepEqual(k, { 0: 'KeyX' })
  })

  test('options fall back to the console', () => {
    const d = { deadzone: 0.15, threshold: 0.5, stickDpad: true, turboHz: 12 }
    assert.equal(effectiveOptions(d, null).deadzone, 0.15)
    const o = effectiveOptions(d, { options: { deadzone: 0.3, stickDpad: false } } as unknown as Profile)
    assert.equal(o.deadzone, 0.3)
    assert.equal(o.stickDpad, false)
    assert.equal(o.threshold, 0.5)
  })

  test('a shared source is a conflict, shown both ways', () => {
    const c = conflicts({ 0: { b: 1 }, 8: { b: 1 }, 1: { a: 0, d: 1 }, 9: { a: 0, d: -1 }, 2: null })
    assert.deepEqual(c.get(0), [8])
    assert.deepEqual(c.get(8), [0])
    assert.equal(c.get(1), undefined, 'the two directions of an axis are different things')
  })

  test('names by what a controller calls its buttons', () => {
    assert.equal(describeSource({ b: 0 }, 'xbox'), 'A')
    assert.equal(describeSource({ b: 0 }, 'playstation'), 'Cross')
    assert.equal(describeSource({ b: 0 }, 'nintendo'), 'B')
    assert.equal(describeSource({ b: 0 }, 'generic'), 'Bottom button')
    assert.equal(describeSource({ a: 1, d: -1 }), 'Left stick up')
    assert.equal(describeSource({ h: 9, d: 1 }), 'D-pad up')
    assert.equal(describeSource({ b: 3 }, 'generic', true), 'Button 3')
    assert.equal(describeSource(null), 'Not set')
  })
})

// ---- the manager, with a simulated browser around it ----

describe('InputManager', () => {
  let InputManager: typeof import('../input').InputManager
  let pads: (ReturnType<typeof makePad> | null)[] = []
  let frames: (() => void)[] = []
  let events: Map<string, Set<(e: any) => void>>
  let sent: Uint8Array[] = []
  const fire = (name: string, e: any = {}) => { for (const f of [...(events.get(name) || [])]) f(e) }
  const frame = () => { const f = frames; frames = []; for (const cb of f) cb() }

  const controls = (): ControlsData => ({
    console: 'snes', pad: { art: 'four' }, controls: [], targets: Object.keys(STICKS).map((id) => ({ id: Number(id), label: id, group: 'face' })),
    defaults: { gamepad: STICKS, keyboard: { 4: 'ArrowUp', 0: 'KeyZ', 18: 'KeyI' }, remote: {}, touch: { landscape: {}, portrait: {} }, options: { deadzone: 0.15, threshold: 0.5, stickDpad: false, turboHz: 12 } },
    saved: { gamepad: {}, keyboard: null, touch: {} }, game: 0, profiles: [], background: { version: 0, fit: 'cover', x: 50, y: 50, dim: 50 },
  })
  const state = (port: number) => {
    for (let i = sent.length - 1; i >= 0; i--) {
      if (sent[i][0] === port) {
        const v = new DataView(sent[i].buffer, sent[i].byteOffset)
        return { buttons: v.getUint16(1, true), lx: v.getInt16(3, true), ly: v.getInt16(5, true), rx: v.getInt16(7, true), ry: v.getInt16(9, true), l2: sent[i][11], r2: sent[i][12] }
      }
    }
    return null
  }
  // Whatever a test does, its manager's timers stop with it.
  let live: InstanceType<typeof InputManager>[] = []
  afterEach(() => { for (const m of live) m.stop(); live = [] })
  const make = (c = controls()) => {
    const m = new InputManager(c)
    m.setSender((msg) => sent.push(new Uint8Array(msg)))
    live.push(m)
    return m
  }

  before(async () => {
    const g = globalThis as any
    Object.defineProperty(g, 'navigator', { configurable: true, value: { userAgent: 'test', getGamepads: () => pads } })
    g.window = g
    g.document = {
      hidden: false,
      addEventListener: (n: string, f: any) => { if (!events.has('doc:' + n)) events.set('doc:' + n, new Set()); events.get('doc:' + n)!.add(f) },
      removeEventListener: (n: string, f: any) => events.get('doc:' + n)?.delete(f),
    }
    g.addEventListener = (n: string, f: any) => { if (!events.has(n)) events.set(n, new Set()); events.get(n)!.add(f) }
    g.removeEventListener = (n: string, f: any) => events.get(n)?.delete(f)
    g.requestAnimationFrame = (cb: () => void) => { frames.push(cb); return frames.length }
    g.cancelAnimationFrame = () => { frames = [] }
    g.matchMedia = () => ({ matches: false })
    g.localStorage = { getItem: () => null, setItem: () => {}, removeItem: () => {} }
    events = new Map()
    InputManager = (await import('../input')).InputManager
  })
  beforeEach(() => { pads = []; frames = []; sent = []; events = new Map(); (globalThis as any).document.hidden = false })

  test('with no pad connected, no frames are asked for', () => {
    const m = make()
    m.start()
    assert.equal(frames.length, 0)
    m.stop()
  })

  test('hot-plugging: frames start, and stop again with nothing left stuck', () => {
    const m = make()
    let changes = 0
    m.onChange = () => { changes++ }
    m.start()
    const p = makePad()
    pads = [p]
    fire('gamepadconnected')
    assert.ok(changes > 0)
    assert.equal(frames.length, 1, 'polling started')
    p.buttons[0] = button(true); p.timestamp = 2
    frame()
    assert.equal(state(0)!.buttons, 1)
    pads = []
    fire('gamepaddisconnected')
    assert.equal(state(0)!.buttons, 0, 'a pad unplugged while held lets go')
    frame()
    assert.equal(frames.length, 0, 'polling stopped')
    m.stop()
  })

  test('a pad the browser never announces is still found', async () => {
    const m = make()
    m.start()
    pads = [makePad()]
    await new Promise((r) => setTimeout(r, 300)) // one heartbeat
    assert.equal(m.listPads().length, 1)
    assert.equal(frames.length, 1)
    m.stop()
  })

  test('several pads get several players, and a swap keeps them apart', () => {
    const m = make()
    m.start()
    const a = makePad({ index: 0 }), b = makePad({ index: 1, id: 'Other (Vendor: 045e Product: 028e)' })
    pads = [a, b]
    fire('gamepadconnected')
    const before = m.listPads()
    assert.deepEqual(before.map((p) => p.port), [0, 1])
    b.buttons[0] = button(true); b.timestamp = 2
    frame()
    assert.equal(state(1)!.buttons, 1)
    assert.equal(state(0)?.buttons ?? 0, 0)
    m.setPadPort(1, 0)
    assert.deepEqual(m.listPads().map((p) => p.port), [1, 0], 'swapped')
    assert.equal(state(0)!.buttons, 1)
    assert.equal(state(1)!.buttons, 0)
    m.stop()
  })

  test('a different pad at the same place is a new pad', () => {
    const m = make()
    m.start()
    pads = [makePad({ index: 0 })]
    fire('gamepadconnected')
    pads = [makePad({ index: 0, id: 'Another (Vendor: 1111 Product: 2222)' })]
    fire('gamepaddisconnected')
    fire('gamepadconnected')
    const l = m.listPads()
    assert.equal(l.length, 1)
    assert.equal(l[0].model, '1111:2222')
    assert.equal(l[0].port, 0)
    m.stop()
  })

  test('a state goes out once per change; an unchanged pad costs nothing', () => {
    const m = make()
    m.start()
    const p = makePad()
    pads = [p]
    fire('gamepadconnected')
    p.buttons[0] = button(true); p.timestamp = 2
    frame()
    const n = sent.length
    frame(); frame(); frame()
    assert.equal(sent.length, n, 'nothing new to say')
    // An unchanged timestamp means the state isn't even read again.
    p.buttons[1] = button(true)
    frame()
    assert.equal(sent.length, n)
    p.timestamp = 3
    frame()
    assert.ok(sent.length > n)
    m.stop()
  })

  test('analog sticks and triggers reach the message', () => {
    const m = make()
    m.start()
    const p = makePad()
    pads = [p]
    fire('gamepadconnected')
    p.axes[0] = 1; p.axes[3] = -1
    p.buttons[6] = button(true, 0.5)
    p.timestamp = 2
    frame()
    const s = state(0)!
    assert.equal(s.lx, 32767)
    assert.equal(s.ry, -32767)
    assert.equal(s.l2, 128)
    assert.equal(sent[sent.length - 1].length, 13)
    m.stop()
  })

  test('a keyboard key can push a stick', () => {
    const m = make()
    m.start()
    fire('keydown', { code: 'KeyI', key: 'i', preventDefault() {} })
    assert.equal(state(0)!.ly, -32767)
    fire('keyup', { code: 'KeyI', key: 'i', preventDefault() {} })
    assert.equal(state(0)!.ly, 0)
    m.stop()
  })

  test('leaving the page lets go of everything, and stops reading', () => {
    const m = make()
    m.start()
    const p = makePad()
    pads = [p]
    fire('gamepadconnected')
    p.buttons[0] = button(true); p.timestamp = 2
    frame()
    fire('keydown', { code: 'KeyZ', key: 'z', preventDefault() {} })
    assert.ok(state(0)!.buttons & 1)
    ;(globalThis as any).document.hidden = true
    fire('doc:visibilitychange')
    assert.equal(state(0)!.buttons, 0)
    frame()
    assert.equal(frames.length, 0, 'no frames for a hidden page')
    ;(globalThis as any).document.hidden = false
    fire('doc:visibilitychange')
    assert.equal(frames.length, 1, 'and back again')
    m.stop()
  })

  test('losing the window lets go of keys, which may be released elsewhere', () => {
    const m = make()
    m.start()
    fire('keydown', { code: 'KeyZ', key: 'z', preventDefault() {} })
    assert.equal(state(0)!.buttons, 1)
    fire('blur')
    assert.equal(state(0)!.buttons, 0)
    m.stop()
  })

  test('menus: the game gets nothing, the combination still works, and capture silences it', () => {
    const m = make()
    let menus = 0
    m.onMenu = () => { menus++ }
    m.start()
    const p = makePad()
    pads = [p]
    fire('gamepadconnected')
    p.buttons[8] = button(true); p.buttons[9] = button(true); p.timestamp = 2
    frame()
    assert.equal(menus, 1, 'opened')
    frame()
    assert.equal(menus, 1, 'once per press')
    m.setEnabled(false)
    p.buttons[0] = button(true); p.timestamp = 3
    frame()
    assert.equal(state(0)!.buttons, 0, 'the game gets nothing')
    p.buttons[8] = button(); p.buttons[9] = button(); p.timestamp = 4
    frame()
    m.setCapturing(true)
    p.buttons[8] = button(true); p.buttons[9] = button(true); p.timestamp = 5
    frame()
    assert.equal(menus, 1, 'not while choosing a button')
    m.stop()
  })

  test('a profile applies to its model, and a preview applies at once', () => {
    const c = controls()
    c.profiles = [{ id: 1, name: 'Mine', kind: 'gamepad', controller: '054c:0ce6', console: 'snes', active: true, map: { 0: { b: 3 } }, options: { deadzone: 0.4 } }]
    const m = make(c)
    m.start()
    const p = makePad()
    pads = [p]
    fire('gamepadconnected')
    assert.equal(m.listPads()[0].profile?.name, 'Mine')
    p.buttons[3] = button(true); p.timestamp = 2
    frame()
    assert.ok(state(0)!.buttons & 1, 'remapped')
    m.previewProfile(0, null)
    p.timestamp = 3
    frame()
    assert.equal(state(0)!.buttons & 1, 0, 'back to the default')
    assert.ok(state(0)!.buttons & (1 << 9), 'top is X')
    m.stop()
  })

  test('rumble: each motor, renewed while wanted, stopped on leaving', async () => {
    const m = make()
    m.start()
    const effects: any[] = []
    let resets = 0
    const p = makePad() as any
    p.vibrationActuator = { playEffect: (_: string, o: any) => { effects.push(o); return Promise.resolve() }, reset: () => { resets++; return Promise.resolve() } }
    pads = [p]
    fire('gamepadconnected')
    m.rumble(0, 0xffff, 0)
    assert.equal(effects.at(-1).strongMagnitude, 1)
    assert.equal(effects.at(-1).weakMagnitude, 0)
    m.rumble(0, 0x8000, 1)
    assert.equal(effects.at(-1).strongMagnitude, 1)
    assert.ok(Math.abs(effects.at(-1).weakMagnitude - 0.5) < 0.01)
    assert.equal(m.listPads()[0].rumble, true)
    m.rumble(0, 0, 0)
    m.rumble(0, 0, 1)
    assert.ok(resets > 0, 'stopped')
    m.rumble(0, 0xffff, 0)
    const n = resets
    m.stop()
    assert.ok(resets > n, 'stops with the game')
  })

  test('a browser that blocks gamepads is no error', () => {
    const g = globalThis as any
    const was = g.navigator
    Object.defineProperty(g, 'navigator', { configurable: true, value: { userAgent: 'test', getGamepads: () => { throw new Error('blocked') } } })
    const m = make()
    m.start()
    assert.deepEqual(m.listPads(), [])
    m.stop()
    Object.defineProperty(g, 'navigator', { configurable: true, value: was })
  })
})
