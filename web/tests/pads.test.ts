// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Controller presses as the app's navigation sees them, on a simulated machine: the real code
// in tv.ts, with time, the timers and the pad stepped by hand. Animation frames never run,
// which is the point: the browser was seen holding them back for up to half a second.
import { after, before, beforeEach, describe, test } from 'node:test'
import assert from 'node:assert/strict'

type Pad = { id: string; index: number; connected: boolean; mapping: string; timestamp: number; axes: number[]; buttons: { pressed: boolean; value: number }[] }

const UP = 12, DOWN = 13, VIEW = 8, MENU = 9, A = 0

let clock = 0
let pads: (Pad | null)[] = []
let timers: { fn: () => void; every: number; next: number; id: number }[] = []
let framesAsked = 0
let keys: string[] = []
let nextId = 1
let lateBy = 0 // how late a loaded machine runs each timer, at worst
let lateTurn = 0
const listeners = new Map<string, ((e: any) => void)[]>()
const classes = new Set<string>()
let inGameNow = false

// Runs simulated time forward, firing timers as they fall due.
function run(ms: number) {
  const end = clock + ms
  for (;;) {
    const t = timers.reduce<(typeof timers)[number] | null>((a, b) => (!a || b.next < a.next ? b : a), null)
    if (!t || t.next > end) break
    clock = Math.max(clock, t.next)
    t.next = clock + t.every + (lateBy ? (lateTurn++ % 2) * lateBy : 0)
    t.fn()
  }
  clock = end
}

const makePad = (): Pad => ({ id: 'Test pad (STANDARD GAMEPAD)', index: 0, connected: true, mapping: 'standard', timestamp: 0, axes: [0, 0, 0, 0], buttons: Array.from({ length: 17 }, () => ({ pressed: false, value: 0 })) })
const set = (b: number, down: boolean) => { const p = pads[0]!; p.buttons[b].pressed = down; p.buttons[b].value = down ? 1 : 0; p.timestamp = clock }
function tap(b: number, hold: number, rest: number) {
  set(b, true); run(hold)
  set(b, false); run(rest)
}
const fire = (name: string) => { for (const f of listeners.get(name) || []) f({}) }

before(async () => {
  const g = globalThis as any
  g.window = g
  g.addEventListener = (n: string, f: (e: any) => void) => { listeners.set(n, [...(listeners.get(n) || []), f]) }
  g.scrollTo = () => {}; g.scrollBy = () => {}
  Object.defineProperty(g, 'navigator', { configurable: true, value: { getGamepads: () => pads } })
  Object.defineProperty(g, 'performance', { configurable: true, value: { now: () => clock } })
  g.setInterval = (fn: () => void, every: number) => { const id = nextId++; timers.push({ fn, every, next: clock + every, id }); return id }
  g.clearInterval = (id: number) => { timers = timers.filter((t) => t.id !== id) }
  g.requestAnimationFrame = () => { framesAsked++; return 1 }
  g.location = { pathname: '/' }
  g.KeyboardEvent = class { key: string; defaultPrevented = true; constructor(_: string, o: { key: string }) { this.key = o.key } }
  g.document = {
    hidden: false,
    body: { dispatchEvent: (e: { key: string }) => { keys.push(e.key); return true } },
    activeElement: null,
    documentElement: { dataset: {}, classList: { add: (c: string) => classes.add(c), remove: (c: string) => classes.delete(c), contains: (c: string) => classes.has(c) } },
    querySelector: (q: string) => (q.includes('.wp-playing') && inGameNow ? {} : null),
  }
  const { installTVInput } = await import('../src/tv')
  installTVInput()
})

beforeEach(() => {
  // Unplugging stops the timer and forgets everything held, ready for the next test.
  pads = []
  run(50)
  assert.equal(timers.length, 0, 'unplugged, nothing keeps reading')
  keys = []; framesAsked = 0; lateBy = 0; inGameNow = false
  ;(globalThis as any).document.hidden = false
  pads = [makePad()]
  fire('gamepadconnected')
})

after(() => { pads = [] })

describe('a controller in the menus', () => {
  // 33ms is a very quick tap, two frames of a 60Hz screen. 16ms is one, shorter than a person
  // manages, and the timer still sees it on a machine that is keeping time.
  for (const hold of [16, 33, 50, 80]) {
    test(`every ${hold}ms press counts while frames have stopped`, () => {
      for (let i = 0; i < 40; i++) tap(i % 2 ? UP : DOWN, hold, 160)
      assert.equal(keys.length, 40)
      assert.equal(framesAsked, 0, 'reading the pad needs no frames')
    })
  }

  test('a 33ms press counts on a loaded machine, whose timers run late', () => {
    lateBy = 13 // the worst measured in a browser with half the cores busy
    for (let i = 0; i < 200; i++) tap(i % 2 ? UP : DOWN, 33, 60 + (i % 7))
    assert.equal(keys.length, 200)
  })

  test('two quick presses of one button stay two', () => {
    tap(DOWN, 33, 33)
    tap(DOWN, 33, 33)
    assert.deepEqual(keys, ['ArrowDown', 'ArrowDown'])
  })

  test('a direction held down repeats, and a button does not', () => {
    set(DOWN, true); run(419)
    assert.equal(keys.length, 1, 'one press, then a pause')
    run(120)
    assert.equal(keys.length, 2, 'then it repeats')
    set(DOWN, false); run(30)
    keys = []
    set(A, true); run(2000)
    assert.deepEqual(keys, ['Enter'])
  })
})

describe('coming out of a game', () => {
  test('buttons still held as its menu opens are not presses in that menu', () => {
    inGameNow = true
    set(VIEW, true); set(MENU, true); run(50)
    assert.deepEqual(keys, [], 'playing, the buttons are the game\'s')
    inGameNow = false // the two together opened the menu, and they're still down
    run(500)
    assert.deepEqual(keys, [])
    set(VIEW, false); set(MENU, false); run(50)
    tap(MENU, 40, 40)
    assert.deepEqual(keys, ['F6'], 'pressed again, it counts')
  })

  test('a direction held through it does not start repeating', () => {
    inGameNow = true
    set(DOWN, true); run(50)
    inGameNow = false
    run(1000)
    assert.deepEqual(keys, [])
  })
})

test('a hidden page ignores the controller', () => {
  (globalThis as any).document.hidden = true
  tap(A, 100, 100)
  assert.deepEqual(keys, [])
})

describe('which button confirms', () => {
  // The ids browsers give, from Chromium and Firefox on Linux with the kernel's own drivers, and
  // Firefox elsewhere, which may leave the vendor's leading zero off.
  const pressA = (id: string, mapping: string, b: number) => {
    pads = [{ ...makePad(), id, mapping, buttons: Array.from({ length: 18 }, () => ({ pressed: false, value: 0 })) }]
    keys = []
    tap(b, 80, 200)
    return keys
  }
  for (const id of ['Microsoft X-Box 360 pad (STANDARD GAMEPAD Vendor: 045e Product: 028e)', 'Sony Interactive Entertainment DualSense Wireless Controller (STANDARD GAMEPAD Vendor: 054c Product: 0ce6)', '054c-09cc-Sony Interactive Entertainment Wireless Controller'])
    test(`the bottom button on ${id.split(' (')[0]}`, () => assert.deepEqual(pressA(id, 'standard', 0), ['Enter']))
  for (const id of ['Nintendo Switch Pro Controller (STANDARD GAMEPAD Vendor: 057e Product: 2009)', '057e-2009-Nintendo Switch Pro Controller', '57e-2009-Wireless Gamepad', '057e-2008-Nintendo Switch Combined Joy-Cons'])
    test(`the right-hand button on ${id}`, () => {
      assert.deepEqual(pressA(id, 'standard', 1), ['Enter'])
      assert.deepEqual(pressA(id, 'standard', 0), ['Escape'])
    })
  test('a pad whose product id only looks like Nintendo\'s is not swapped', () => {
    assert.deepEqual(pressA('Some Pad (STANDARD GAMEPAD Vendor: 1234 Product: 057e)', 'standard', 0), ['Enter'])
  })

  test('two Joy-Cons that joycond puts together, with no layout from the browser', () => {
    const id = 'Nintendo Switch Combined Joy-Cons (Vendor: 057e Product: 2008)'
    assert.deepEqual(pressA(id, '', 1), ['Enter'], 'A')
    assert.deepEqual(pressA(id, '', 0), ['Escape'], 'B')
    for (const [b, k] of [[14, 'ArrowUp'], [15, 'ArrowDown'], [16, 'ArrowLeft'], [17, 'ArrowRight'], [10, 'F6'], [2, '/']] as const)
      assert.deepEqual(pressA(id, '', b), [k], `button ${b}`)
    assert.deepEqual(pressA(id, '', 9), [], 'Minus alone does nothing in the app')
    assert.deepEqual(pressA(id, '', 4), [], 'Capture does nothing')
  })
})

test('Android lays a pad out the standard way, even one it gives no name to, such as a DualSense', () => {
  const nav = (globalThis as any).navigator
  const say = (userAgent: string) => Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { getGamepads: nav.getGamepads, userAgent } })
  pads = [{ ...makePad(), mapping: '' }]
  keys = []
  say('Mozilla/5.0 (Windows NT 10.0)')
  tap(DOWN, 80, 200)
  assert.deepEqual(keys, [], 'elsewhere, an unnamed layout says nothing about where its D-pad is')
  say('Mozilla/5.0 (Linux; Android 16) Chrome/143')
  tap(DOWN, 80, 200)
  assert.deepEqual(keys, ['ArrowDown'])
  Object.defineProperty(globalThis, 'navigator', { configurable: true, value: nav })
})

describe('Up with nothing above, for an app that has a bar of its own over the page', () => {
  // One button alone on the page, as the topmost row is when Up is pressed on it.
  const button = (dialog: object | null = null) => ({
    tagName: 'BUTTON', isContentEditable: false, parentElement: null,
    closest: (q: string) => (dialog && q.includes('[role=dialog]') ? dialog : null),
    matches: () => false, hasAttribute: () => false,
    getBoundingClientRect: () => ({ left: 90, top: 24, right: 290, bottom: 90, width: 200, height: 66 }),
  })
  let sent: string[] = []
  const press = (key: string, target: object) => {
    const e = { key, target, altKey: false, ctrlKey: false, metaKey: false, defaultPrevented: false, preventDefault() { this.defaultPrevented = true }, stopImmediatePropagation() {} }
    for (const f of listeners.get('keydown') || []) f(e)
    return e
  }
  const page = (target: object, host: string | undefined, dialog: object | null = null) => {
    const g = globalThis as any
    g.document.activeElement = target
    g.document.querySelectorAll = () => [target]
    if (dialog) (dialog as any).querySelectorAll = () => [target]
    g.document.documentElement.dataset.appUp = host
    g.getComputedStyle = () => ({ visibility: 'visible', pointerEvents: 'auto', opacity: '1', overflowX: 'visible', overflowY: 'visible' })
    g.innerHeight = 1080
    g.location.assign = (to: string) => { sent.push(to) }
  }
  beforeEach(() => { sent = [] })
  after(() => {
    const g = globalThis as any
    g.document.activeElement = null
    delete g.document.documentElement.dataset.appUp
  })

  test('the page tells the app, at the address it gave', () => {
    const b = button()
    page(b, '/.wittymedia-app-up')
    const e = press('ArrowUp', b)
    assert.deepEqual(sent, ['/.wittymedia-app-up'])
    assert.ok(e.defaultPrevented)
  })

  test('only Up: down, left and right with nothing that way stay on the page', () => {
    const b = button()
    page(b, '/.wittymedia-app-up')
    for (const k of ['ArrowDown', 'ArrowLeft', 'ArrowRight']) press(k, b)
    assert.deepEqual(sent, [])
  })

  test('with no app around the page, nothing happens', () => {
    const b = button()
    page(b, undefined)
    press('ArrowUp', b)
    assert.deepEqual(sent, [])
  })

  test('a dialog keeps its arrows', () => {
    const dialog = {}
    const b = button(dialog)
    page(b, '/.wittymedia-app-up', dialog)
    press('ArrowUp', b)
    assert.deepEqual(sent, [])
  })
})
