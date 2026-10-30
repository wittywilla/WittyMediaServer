import { A, B, DOWN, L, L2, L3, LEFT, R, R2, R3, RIGHT, SELECT, START, UP, X, Y } from './consoles'

// Turns every controller into one RetroPad state per player, and sends a state
// when it changes. Nothing else in the player knows what a gamepad is.

export const MAX_PLAYERS = 4
const AXIS_THRESHOLD = 0.5
const DEADZONE = 0.15

export type PadBinding = { b: number } | { a: number; d: 1 | -1 } // a button, or an axis direction
export type PadMap = Record<number, PadBinding>
export type KeyMap = Record<number, string> // RetroPad id -> KeyboardEvent.code

// By position, as libretro expects: bottom button is RetroPad B, right is A, and
// so on. This is the W3C standard mapping.
export const STANDARD_MAP: PadMap = {
  [B]: { b: 0 }, [A]: { b: 1 }, [Y]: { b: 2 }, [X]: { b: 3 }, [L]: { b: 4 }, [R]: { b: 5 }, [L2]: { b: 6 }, [R2]: { b: 7 },
  [SELECT]: { b: 8 }, [START]: { b: 9 }, [L3]: { b: 10 }, [R3]: { b: 11 }, [UP]: { b: 12 }, [DOWN]: { b: 13 }, [LEFT]: { b: 14 }, [RIGHT]: { b: 15 },
}

export const DEFAULT_KEYS: KeyMap = {
  [UP]: 'ArrowUp', [DOWN]: 'ArrowDown', [LEFT]: 'ArrowLeft', [RIGHT]: 'ArrowRight',
  [B]: 'KeyZ', [A]: 'KeyX', [Y]: 'KeyA', [X]: 'KeyS', [L]: 'KeyQ', [R]: 'KeyW', [L2]: 'KeyE', [R2]: 'KeyR',
  [START]: 'Enter', [SELECT]: 'ShiftRight',
}

// Browser storage can be blocked or empty; losing a preference is never an error.
export function load<T>(key: string): T | null {
  try { const v = localStorage.getItem(key); return v ? JSON.parse(v) as T : null } catch { return null }
}
export function save(key: string, value: unknown) {
  try { if (value === null) localStorage.removeItem(key); else localStorage.setItem(key, JSON.stringify(value)) } catch { /* not kept */ }
}

const padKey = (consoleId: string, padId: string) => `wittyplay.pad.${consoleId}.${padId.slice(0, 80)}`
const keysKey = (consoleId: string) => `wittyplay.keys.${consoleId}`

export const loadPadMap = (consoleId: string, padId: string) => load<PadMap>(padKey(consoleId, padId))
export const savePadMap = (consoleId: string, padId: string, m: PadMap | null) => save(padKey(consoleId, padId), m)
export const loadKeyMap = (consoleId: string): KeyMap => load<KeyMap>(keysKey(consoleId)) || DEFAULT_KEYS
export const saveKeyMap = (consoleId: string, m: KeyMap | null) => save(keysKey(consoleId), m)

export type PadInfo = { index: number; id: string; name: string; standard: boolean; port: number; hasMap: boolean; rumble: boolean }

// Device names are often a long string with vendor numbers in it.
export const padName = (id: string) => id.replace(/\s*\(.*?\)\s*/g, ' ').replace(/^[0-9a-f]{4}-[0-9a-f]{4}-/i, '').trim() || 'Gamepad'

type PadSlot = { port: number; map: PadMap | null }

export class InputManager {
  onChange?: () => void // the list of controllers changed
  private send: (msg: Uint8Array) => void = () => {}
  private consoleId: string
  private keys: KeyMap
  private keyOf = new Map<string, number>()
  private held = new Set<number>() // keyboard
  private touchMask = 0
  private touchPort = 0
  private keyboardPort = 0
  private pads = new Map<number, PadSlot>()
  private last: Uint8Array[] = []
  private raf = 0
  private beat = 0
  private enabled = true
  private polling = false

  constructor(consoleId: string) {
    this.consoleId = consoleId
    this.keys = loadKeyMap(consoleId)
    this.indexKeys()
    for (let i = 0; i < MAX_PLAYERS; i++) this.last.push(new Uint8Array(11))
  }

  private indexKeys() {
    this.keyOf.clear()
    for (const [id, code] of Object.entries(this.keys)) this.keyOf.set(code, Number(id))
  }

  setSender(send: (msg: Uint8Array) => void) { this.send = send }

  // While a menu is open the game gets nothing, and everything held is let go.
  setEnabled(on: boolean) {
    if (this.enabled === on) return
    this.enabled = on
    if (!on) { this.held.clear(); this.touchMask = 0 }
    this.flush(true)
  }

  start() {
    window.addEventListener('keydown', this.keydown)
    window.addEventListener('keyup', this.keyup)
    window.addEventListener('blur', this.blur)
    window.addEventListener('gamepadconnected', this.padEvent)
    window.addEventListener('gamepaddisconnected', this.padEvent)
    this.polling = true
    const tick = () => { if (!this.polling) return; this.poll(); this.raf = requestAnimationFrame(tick) }
    this.raf = requestAnimationFrame(tick)
    // A full state now and then corrects anything lost on an unreliable channel,
    // and keeps polling pads when animation frames stop in a background tab.
    this.beat = window.setInterval(() => { this.poll(); this.flush(true) }, 250)
    this.poll()
  }

  stop() {
    this.polling = false
    cancelAnimationFrame(this.raf)
    clearInterval(this.beat)
    window.removeEventListener('keydown', this.keydown)
    window.removeEventListener('keyup', this.keyup)
    window.removeEventListener('blur', this.blur)
    window.removeEventListener('gamepadconnected', this.padEvent)
    window.removeEventListener('gamepaddisconnected', this.padEvent)
  }

  // ---- what's connected ----

  private rawPads(): Gamepad[] {
    try { return (navigator.getGamepads?.() || []).filter((p): p is Gamepad => !!p && p.connected !== false) } catch { return [] } // blocked by policy
  }

  private padEvent = () => { this.poll(); this.onChange?.() }

  listPads(): PadInfo[] {
    return this.rawPads().map((p) => {
      const slot = this.slot(p)
      return { index: p.index, id: p.id, name: padName(p.id), standard: p.mapping === 'standard', port: slot.port, hasMap: !!slot.map, rumble: !!(p as any).vibrationActuator }
    })
  }

  // A pad takes the next player nobody else is on.
  private slot(p: Gamepad): PadSlot {
    let s = this.pads.get(p.index)
    if (!s) {
      const used = new Set([...this.pads.values()].map((x) => x.port))
      let port = 0
      while (used.has(port) && port < MAX_PLAYERS - 1) port++
      s = { port, map: loadPadMap(this.consoleId, p.id) }
      this.pads.set(p.index, s)
      this.onChange?.()
    }
    return s
  }

  setPadPort(index: number, port: number) {
    const s = this.pads.get(index)
    if (s) { s.port = port; this.flush(true); this.onChange?.() }
  }
  get keyboardPlayer() { return this.keyboardPort }
  setKeyboardPort(port: number) { this.keyboardPort = port; this.flush(true); this.onChange?.() }
  get touchPlayer() { return this.touchPort }
  setTouchPort(port: number) { this.touchPort = port; this.flush(true); this.onChange?.() }

  reloadMaps() {
    this.keys = loadKeyMap(this.consoleId)
    this.indexKeys()
    for (const p of this.rawPads()) {
      const s = this.pads.get(p.index)
      if (s) s.map = loadPadMap(this.consoleId, p.id)
    }
  }

  // ---- keyboard ----

  private keydown = (e: KeyboardEvent) => {
    if (!this.enabled || e.ctrlKey || e.altKey || e.metaKey) return
    const id = this.keyOf.get(e.code)
    if (id === undefined) return
    e.preventDefault()
    if (e.repeat) return
    this.held.add(id)
    this.flush()
  }
  private keyup = (e: KeyboardEvent) => {
    const id = this.keyOf.get(e.code)
    if (id !== undefined && this.held.delete(id)) { e.preventDefault(); this.flush() }
  }
  private blur = () => { if (this.held.size) { this.held.clear(); this.flush() } }

  // ---- touch ----

  setTouch(mask: number) {
    if (mask !== this.touchMask) { this.touchMask = mask; this.flush() }
  }

  // ---- the state of every player ----

  private poll() { if (this.flush()) this.onActivity?.() }
  onActivity?: () => void

  private flush(force = false): boolean {
    const buttons = [0, 0, 0, 0]
    const axes = [[0, 0, 0, 0], [0, 0, 0, 0], [0, 0, 0, 0], [0, 0, 0, 0]]
    if (this.enabled) {
      for (const id of this.held) buttons[this.keyboardPort] |= 1 << id
      buttons[this.touchPort] |= this.touchMask
      for (const p of this.rawPads()) {
        const s = this.slot(p)
        buttons[s.port] |= readButtons(p, s.map)
        const a = readAxes(p)
        if (a) axes[s.port] = a
      }
    }
    let sent = false
    for (let port = 0; port < MAX_PLAYERS; port++) {
      const msg = new Uint8Array(11)
      const v = new DataView(msg.buffer)
      msg[0] = port
      v.setUint16(1, buttons[port], true)
      for (let i = 0; i < 4; i++) v.setInt16(3 + i * 2, axes[port][i], true)
      const same = this.last[port].every((x, i) => x === msg[i])
      if (same && !force) continue
      if (same && force && port > 0 && buttons[port] === 0 && msg.every((x, i) => i === 0 || x === 0)) continue // idle players need no heartbeat
      this.last[port] = msg
      this.send(msg)
      sent = true
    }
    return sent
  }

  rumble(port: number, strength: number) {
    let done = false
    for (const p of this.rawPads()) {
      if (this.slot(p).port !== port) continue
      const act = (p as any).vibrationActuator
      if (act?.playEffect) {
        act.playEffect('dual-rumble', { duration: 200, strongMagnitude: strength / 0xffff, weakMagnitude: strength / 0xffff }).catch(() => {})
        done = true
      }
    }
    if (!done && port === this.touchPort && strength > 0 && (navigator as any).userActivation?.hasBeenActive !== false) navigator.vibrate?.(30)
  }
}

export function readButtons(p: Gamepad, map: PadMap | null): number {
  let mask = 0
  const m = map || STANDARD_MAP
  for (const [id, bind] of Object.entries(m)) {
    if (isPressed(p, bind)) mask |= 1 << Number(id)
  }
  // The left stick also steers the D-pad unless someone chose otherwise, because
  // most retro consoles have nothing else to steer with.
  if (!map) {
    const x = p.axes[0] || 0, y = p.axes[1] || 0
    if (x < -AXIS_THRESHOLD) mask |= 1 << LEFT
    if (x > AXIS_THRESHOLD) mask |= 1 << RIGHT
    if (y < -AXIS_THRESHOLD) mask |= 1 << UP
    if (y > AXIS_THRESHOLD) mask |= 1 << DOWN
  }
  return mask
}

function isPressed(p: Gamepad, bind: PadBinding): boolean {
  if ('b' in bind) {
    const b = p.buttons[bind.b]
    return !!b && (b.pressed || b.value > 0.5)
  }
  const v = p.axes[bind.a] || 0
  return bind.d > 0 ? v > AXIS_THRESHOLD : v < -AXIS_THRESHOLD
}

function readAxes(p: Gamepad): number[] | null {
  if (p.axes.length < 2) return null
  const conv = (v = 0) => (Math.abs(v) < DEADZONE ? 0 : Math.round(Math.max(-1, Math.min(1, v)) * 32767))
  return [conv(p.axes[0]), conv(p.axes[1]), conv(p.axes[2]), conv(p.axes[3])]
}

// The first thing pressed on any pad, for the remap screen. baseline holds what
// was already held when listening began.
export type PadSnapshot = { buttons: boolean[]; axes: number[] }
export function snapshotPad(p: Gamepad): PadSnapshot {
  return { buttons: p.buttons.map((b) => b.pressed || b.value > 0.5), axes: [...p.axes] }
}
export function findPress(p: Gamepad, base: PadSnapshot): PadBinding | null {
  for (let i = 0; i < p.buttons.length; i++) {
    if ((p.buttons[i].pressed || p.buttons[i].value > 0.5) && !base.buttons[i]) return { b: i }
  }
  for (let i = 0; i < p.axes.length; i++) {
    const v = p.axes[i]
    if (Math.abs(v) > 0.6 && Math.abs((base.axes[i] || 0) - v) > 0.5) return { a: i, d: v > 0 ? 1 : -1 }
  }
  return null
}

export const describeBinding = (b?: PadBinding) => (!b ? 'Not set' : 'b' in b ? `Button ${b.b}` : `Axis ${b.a} ${b.d > 0 ? '+' : '−'}`)
// "KeyZ" -> "Z", "ArrowUp" -> "Up"
export const describeKey = (code?: string) => (!code ? 'Not set' : code.replace(/^Key/, '').replace(/^Digit/, '').replace(/^Arrow/, ''))
