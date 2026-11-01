// What a browser sends to a PC: controllers, keyboard, mouse. Each message is a type byte and a
// body, in the protocol in modules/wittycouchhost/PROTOCOL.md. Written from that protocol
// and from the browser's own APIs.

const CMD_PAD = 65, CMD_MOVE = 66, CMD_BUTTON = 67, CMD_KEY = 68, CMD_SCROLL = 69, CMD_POSITION = 72, CMD_HSCROLL = 74

export type Send = (m: Uint8Array, reliable: boolean) => void

// Buttons as the PC's controller has them, by the position of each on a standard pad.
const BUTTONS: [number, number][] = [
  [0, 0x1000], [1, 0x2000], [2, 0x4000], [3, 0x8000], // A B X Y
  [4, 0x0100], [5, 0x0200], // shoulders
  [8, 0x0020], [9, 0x0010], // back, start
  [10, 0x0040], [11, 0x0080], // stick clicks
  [12, 0x0001], [13, 0x0002], [14, 0x0004], [15, 0x0008], // D-pad
  [16, 0x0400], // the logo button
]
const MENU_COMBO = 0x0010 | 0x0020 | 0x0100 | 0x0200 // Start, Back and both shoulders
const DEAD_ZONE = 0.08
export const MAX_PADS = 4

function stick(v: number): number {
  const a = Math.abs(v) < DEAD_ZONE ? 0 : v
  return Math.max(-32767, Math.min(32767, Math.round(a * 32767)))
}

type PadState = { buttons: number; lt: number; rt: number; lx: number; ly: number; rx: number; ry: number }

export function readPad(p: Gamepad): PadState {
  let buttons = 0
  for (const [i, flag] of BUTTONS) if (p.buttons[i]?.pressed) buttons |= flag
  const trig = (i: number) => Math.round((p.buttons[i]?.value ?? 0) * 255)
  return {
    buttons, lt: trig(6), rt: trig(7), lx: stick(p.axes[0] ?? 0), rx: stick(p.axes[2] ?? 0),
    // A browser's up is negative, and the PC's is positive.
    ly: stick(-(p.axes[1] ?? 0)), ry: stick(-(p.axes[3] ?? 0)),
  }
}

export function padMessage(index: number, mask: number, s: PadState): Uint8Array {
  const b = new ArrayBuffer(19)
  const d = new DataView(b)
  d.setUint8(0, CMD_PAD)
  d.setUint8(1, index)
  d.setUint8(2, 0)
  d.setUint16(3, mask, true)
  d.setUint32(5, s.buttons, true)
  d.setUint8(9, s.lt)
  d.setUint8(10, s.rt)
  d.setInt16(11, s.lx, true)
  d.setInt16(13, s.ly, true)
  d.setInt16(15, s.rx, true)
  d.setInt16(17, s.ry, true)
  return new Uint8Array(b)
}

const same = (a: PadState | undefined, b: PadState) => !!a && a.buttons === b.buttons && a.lt === b.lt && a.rt === b.rt && a.lx === b.lx && a.ly === b.ly && a.rx === b.rx && a.ry === b.ry

// Polls controllers while a stream shows. Every state is complete, so one lost on the way is
// put right by the next. A controller coming or going takes its slot with it.
export class Pads {
  private last = new Map<number, PadState>()
  private lastSent = 0
  private timer = 0
  private menuSince = 0
  private paused = false
  connected = 0
  constructor(private send: Send, private onMenu: () => void, private onChange: (n: number) => void) {}
  start() { this.timer = window.setInterval(() => this.poll(), 8) }
  stop() { clearInterval(this.timer) }
  // While paused the game hears nothing, and starts from nothing held down.
  setPaused(on: boolean) {
    if (on === this.paused) return
    this.paused = on
    if (!on) return
    this.last.clear()
    const mask = (1 << this.connected) - 1
    for (let i = 0; i < this.connected; i++) this.send(padMessage(i, mask, { buttons: 0, lt: 0, rt: 0, lx: 0, ly: 0, rx: 0, ry: 0 }), true)
  }
  private poll() {
    if (document.hidden || !navigator.getGamepads) return
    const pads = [...navigator.getGamepads()].filter((p): p is Gamepad => !!p && p.connected).slice(0, MAX_PADS)
    const mask = pads.reduce((m, _, i) => m | (1 << i), 0)
    if (pads.length !== this.connected) {
      // A pad that's gone needs one last message, so nothing stays held down on the PC.
      for (let i = pads.length; i < this.connected; i++) this.send(padMessage(i, mask, { buttons: 0, lt: 0, rt: 0, lx: 0, ly: 0, rx: 0, ry: 0 }), true)
      this.connected = pads.length
      this.onChange(pads.length)
    }
    const now = performance.now()
    const keepAlive = now - this.lastSent > 100
    pads.forEach((p, i) => {
      const s = readPad(p)
      if (i === 0 && (s.buttons & MENU_COMBO) === MENU_COMBO) {
        if (!this.menuSince) this.menuSince = now
        else if (now - this.menuSince > 800) { this.menuSince = 0; this.onMenu() }
      } else if (i === 0) this.menuSince = 0
      if (!this.paused && (!same(this.last.get(i), s) || keepAlive)) {
        this.send(padMessage(i, mask, s), false)
        this.last.set(i, s)
        this.lastSent = now
      }
    })
  }
  rumble(index: number, low: number, high: number) {
    const pad = navigator.getGamepads?.()[index] as (Gamepad & { vibrationActuator?: { playEffect?(t: string, o: object): Promise<unknown> } }) | null
    pad?.vibrationActuator?.playEffect?.('dual-rumble', { duration: 250, strongMagnitude: low / 65535, weakMagnitude: high / 65535 })?.catch(() => {})
  }
}

// Windows virtual key codes, by the physical key the browser names: they are what the PC expects,
// and they're the same whatever keyboard layout this device has.
const VK: Record<string, number> = {
  Backspace: 0x08, Tab: 0x09, Enter: 0x0d, ShiftLeft: 0xa0, ShiftRight: 0xa1, ControlLeft: 0xa2, ControlRight: 0xa3, AltLeft: 0xa4, AltRight: 0xa5,
  Pause: 0x13, CapsLock: 0x14, Escape: 0x1b, Space: 0x20, PageUp: 0x21, PageDown: 0x22, End: 0x23, Home: 0x24, ArrowLeft: 0x25, ArrowUp: 0x26, ArrowRight: 0x27, ArrowDown: 0x28,
  PrintScreen: 0x2c, Insert: 0x2d, Delete: 0x2e, MetaLeft: 0x5b, MetaRight: 0x5c, ContextMenu: 0x5d,
  Numpad0: 0x60, Numpad1: 0x61, Numpad2: 0x62, Numpad3: 0x63, Numpad4: 0x64, Numpad5: 0x65, Numpad6: 0x66, Numpad7: 0x67, Numpad8: 0x68, Numpad9: 0x69,
  NumpadMultiply: 0x6a, NumpadAdd: 0x6b, NumpadSubtract: 0x6d, NumpadDecimal: 0x6e, NumpadDivide: 0x6f, NumpadEnter: 0x0d, NumLock: 0x90, ScrollLock: 0x91,
  Semicolon: 0xba, Equal: 0xbb, Comma: 0xbc, Minus: 0xbd, Period: 0xbe, Slash: 0xbf, Backquote: 0xc0, BracketLeft: 0xdb, Backslash: 0xdc, BracketRight: 0xdd, Quote: 0xde, IntlBackslash: 0xe2,
}
for (let i = 0; i < 26; i++) VK['Key' + String.fromCharCode(65 + i)] = 65 + i
for (let i = 0; i < 10; i++) VK['Digit' + i] = 48 + i
for (let i = 1; i <= 24; i++) VK['F' + i] = 0x6f + i

// Keys that share a code on the PC, told apart by this flag.
const EXTENDED = new Set(['NumpadEnter', 'ArrowLeft', 'ArrowUp', 'ArrowRight', 'ArrowDown', 'Insert', 'Delete', 'Home', 'End', 'PageUp', 'PageDown', 'NumpadDivide', 'ControlRight', 'AltRight'])

export function vkFor(code: string): number | undefined {
  return VK[code]
}

export function keyMessage(code: string, down: boolean, e: { shiftKey: boolean; ctrlKey: boolean; altKey: boolean; metaKey: boolean }): Uint8Array | null {
  const vk = VK[code]
  if (vk === undefined) return null
  const mods = (e.shiftKey ? 1 : 0) | (e.ctrlKey ? 2 : 0) | (e.altKey ? 4 : 0) | (e.metaKey ? 8 : 0) | (EXTENDED.has(code) ? 0x10 : 0)
  return new Uint8Array([CMD_KEY, vk & 0xff, 0x80, down ? 3 : 4, mods])
}

const i16 = (v: number) => Math.max(-32768, Math.min(32767, Math.round(v)))

export function moveMessage(dx: number, dy: number): Uint8Array {
  const d = new DataView(new ArrayBuffer(5))
  d.setUint8(0, CMD_MOVE)
  d.setInt16(1, i16(dx), true)
  d.setInt16(3, i16(dy), true)
  return new Uint8Array(d.buffer)
}

export function positionMessage(x: number, y: number, w: number, h: number): Uint8Array {
  const d = new DataView(new ArrayBuffer(9))
  d.setUint8(0, CMD_POSITION)
  d.setInt16(1, i16(x), true)
  d.setInt16(3, i16(y), true)
  d.setInt16(5, i16(w), true)
  d.setInt16(7, i16(h), true)
  return new Uint8Array(d.buffer)
}

// A browser numbers buttons 0 left, 1 middle, 2 right, 3 back, 4 forward. The PC starts at 1.
export function buttonMessage(button: number, down: boolean): Uint8Array {
  return new Uint8Array([CMD_BUTTON, down ? 7 : 8, [1, 2, 3, 4, 5][button] ?? 1])
}

export function scrollMessage(amount: number, horizontal: boolean): Uint8Array {
  const d = new DataView(new ArrayBuffer(3))
  d.setUint8(0, horizontal ? CMD_HSCROLL : CMD_SCROLL)
  d.setInt16(1, i16(amount), true)
  return new Uint8Array(d.buffer)
}

// The on-screen controller's buttons, for a touch screen with no pad.
export const TOUCH_BUTTONS = {
  a: 0x1000, b: 0x2000, x: 0x4000, y: 0x8000, lb: 0x0100, rb: 0x0200, start: 0x0010, back: 0x0020, up: 0x0001, down: 0x0002, left: 0x0004, right: 0x0008,
}
