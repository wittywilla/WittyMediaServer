import { padKey, type ControlsData } from './consoles'
import {
  compile, effectiveKeys, effectiveMap, effectiveOptions, knownPad, menuHeld, newReading, noteHat, parseModel, readPad, resolveProfile, standingOf,
  STICK_LX_NEG, TARGETS, type Compiled, type ControllerData, type KeyMap, type PadLike, type PadMap, type Profile, type Reading, type Standing,
} from './mapping'

// Turns every controller into one RetroPad state per player, and sends a state
// when it changes. Nothing else in the player knows what a gamepad is.
//
// Pads are read in step with the screen, which is as often as browsers update them,
// and only while one is connected and the page can be seen. With none connected, the
// keyboard and touch work from their own events, and the only cost is the heartbeat.

export const MAX_PLAYERS = 4
const HEARTBEAT_MS = 250
const SEATS_KEY = 'wittyplay.seats'
// A motor's effect runs for this long, and is renewed while the game wants it, so a
// lost stop can't leave a pad shaking for good.
const RUMBLE_MS = 1000

// Browser storage can be blocked or empty; losing a preference is never an error.
// Only this device's own choices live here. Controller layouts are on the server.
export function load<T>(key: string): T | null {
  try { const v = localStorage.getItem(key); return v ? JSON.parse(v) as T : null } catch { return null }
}
export function save(key: string, value: unknown) {
  try { if (value === null) localStorage.removeItem(key); else localStorage.setItem(key, JSON.stringify(value)) } catch { /* not kept */ }
}

// This device's name for itself, which touch layouts are saved under.
export function deviceId(): string {
  let id = load<string>('wittyplay.device')
  if (!id) {
    id = 'web-' + Math.random().toString(36).slice(2, 12)
    save('wittyplay.device', id)
  }
  return id
}

// TV browsers and web views report no pointer at all, and some name themselves.
export const onTV = () => (window.matchMedia?.('(pointer: none)').matches ?? false) || /\b(TV|AFT[A-Z]*|BRAVIA|GoogleTV|SmartTV|CrKey|Tizen|Web0S)\b/.test(navigator.userAgent)

// Whether this browser can show us gamepads at all, for saying what to do about it.
// Browsers keep them for secure pages (https or localhost); some say so plainly, and
// some just report none, so a plain http page with no pad gets the advice either way.
export type PadSupport = 'ok' | 'insecure' | 'none' | 'blocked'
export function padSupport(): PadSupport {
  if (typeof navigator.getGamepads !== 'function') return window.isSecureContext === false ? 'insecure' : 'none'
  try { navigator.getGamepads() } catch { return window.isSecureContext === false ? 'insecure' : 'blocked' }
  return window.isSecureContext === false ? 'insecure' : 'ok'
}

export type PadInfo = {
  index: number; id: string; name: string; model: string; standard: boolean; standing: Standing; style: string; port: number
  rumble: boolean; profile: Profile | null; hasMap: boolean; raw: boolean
}

// Device names are often a long string with vendor numbers in it.
export const padName = (id: string) => parseModel(id).name

type PadSlot = {
  seat: string // what its player is remembered under
  index: number; id: string; model: string; port: number
  standing: Standing; style: string; raw: boolean; layoutName: string
  map: PadMap; profile: Profile | null; compiled: Compiled; reading: Reading; lastStamp: number
}

type Motors = { strong: number; weak: number; timer: number }

export class InputManager {
  onChange?: () => void // the list of controllers changed
  onActivity?: () => void // a state went out
  onMenu?: () => void // the menu combination was pressed
  onFrame?: () => void // pads were read; for lighting up a picture
  private send: (msg: Uint8Array) => void = () => {}
  private controls: ControlsData
  private known: ControllerData | null
  private game = 0
  private targets = new Set<number>()
  private keyOf = new Map<string, number>()
  private remoteOf = new Map<string, number>()
  private tv = onTV()
  private pulseMask = 0
  private held = new Set<number>() // keyboard
  private touchMask = 0
  private touchX = 0
  private touchY = 0
  private touchPort = 0
  private keyboardPort = 0
  private keyboardTurbo = 0
  private keyboardHz = 12
  private slots = new Map<number, PadSlot>() // by the browser's index for each pad
  private last: Uint8Array[] = []
  private scratch = new Uint8Array(13)
  private scratchView = new DataView(this.scratch.buffer)
  private raf = 0
  private beat = 0
  private enabled = true
  private polling = false
  private point = new Uint8Array([0xff, 0, 0, 0, 0, 0]) // the touch screen's state
  private pointRepeats = 0
  private hidden = false
  private capturing = false
  private menuDown = false
  private motors = new Map<number, Motors>()
  private seats: Record<string, number> = {}
  // One frame's sums, kept to save allocating them sixty times a second.
  private sumB = new Int32Array(MAX_PLAYERS)
  private sumA = new Float64Array(MAX_PLAYERS * 4)
  private sumT = new Float64Array(MAX_PLAYERS * 2)

  constructor(controls: ControlsData, known: ControllerData | null = null, game = 0) {
    this.controls = controls
    this.known = known
    this.game = game
    this.seats = load<Record<string, number>>(SEATS_KEY) || {}
    this.indexKeys()
    for (let i = 0; i < MAX_PLAYERS; i++) this.last.push(new Uint8Array(13))
  }

  private indexKeys() {
    this.targets = new Set(this.controls.targets.map((t) => t.id))
    this.keyOf.clear()
    this.remoteOf.clear()
    const p = resolveProfile(this.controls.profiles, 'keyboard', 'keyboard', this.controls.console, this.game, false)
    const keys = p || !this.controls.saved.keyboard ? effectiveKeys(this.controls.defaults.keyboard, this.targets, p) : this.controls.saved.keyboard
    for (const [id, code] of Object.entries(keys)) this.keyOf.set(code, Number(id))
    for (const [id, key] of Object.entries(this.controls.defaults.remote)) this.remoteOf.set(key, Number(id))
    const o = effectiveOptions(this.controls.defaults.options, p)
    this.keyboardTurbo = 0
    for (const id of o.turbo || []) this.keyboardTurbo |= 1 << id
    this.keyboardHz = o.turboHz || 12
  }

  // Which keys do what right now, for the mapping screen.
  keyMap(): KeyMap {
    const out: KeyMap = {}
    for (const [code, id] of this.keyOf) out[id] = code
    return out
  }

  // The button a key means. A TV remote is a controller of its own, and remotes
  // often send a key name with no code.
  private keyButton(e: KeyboardEvent): number | undefined {
    if (this.tv || !e.code) { const id = this.remoteOf.get(e.key); if (id !== undefined) return id }
    return this.keyOf.get(e.code || e.key)
  }

  // Presses a button for a moment: the game menu's Start and Select, for controllers
  // that have neither.
  pulse(id: number) {
    this.pulseMask |= 1 << id
    this.flush()
    setTimeout(() => { this.pulseMask &= ~(1 << id); this.flush() }, 200)
  }

  setSender(send: (msg: Uint8Array) => void) { this.send = send }

  // The touch screen. x and y run from 0 to 1 across the picture. Sent at once, and
  // repeated on the beat because the channel may lose it: while held, and a few times
  // after, since a lost release would hold the stylus down.
  setPointer(x: number, y: number, pressed: boolean) {
    const at = (v: number) => Math.round(Math.max(0, Math.min(1, v)) * 65534) - 32767
    const view = new DataView(this.point.buffer)
    this.point[1] = pressed ? 1 : 0
    view.setInt16(2, at(x), true)
    view.setInt16(4, at(y), true)
    this.pointRepeats = 4
    this.send(this.point)
  }

  private beatPointer() {
    if (this.point[1] === 0 && this.pointRepeats <= 0) return
    if (this.point[1] === 0) this.pointRepeats--
    this.send(this.point)
  }

  // While a menu is open the game gets nothing, and everything held is let go.
  setEnabled(on: boolean) {
    if (this.enabled === on) return
    this.enabled = on
    if (!on) { this.held.clear(); this.touchMask = 0; this.touchX = 0; this.touchY = 0; this.stopRumble(); if (this.point[1]) { this.point[1] = 0; this.pointRepeats = 4; this.send(this.point) } }
    this.flush(true)
  }

  // While someone is choosing a button, nothing they press means anything else.
  setCapturing(on: boolean) { this.capturing = on }

  start() {
    window.addEventListener('keydown', this.keydown)
    window.addEventListener('keyup', this.keyup)
    window.addEventListener('blur', this.blur)
    window.addEventListener('gamepadconnected', this.padEvent)
    window.addEventListener('gamepaddisconnected', this.padEvent)
    document.addEventListener('visibilitychange', this.visibility)
    this.polling = true
    this.hidden = document.hidden
    // A full state now and then corrects anything lost on an unreliable channel. It
    // also finds pads in browsers that don't fire the connection events.
    this.beat = window.setInterval(() => { this.poll(); this.flush(true); this.beatPointer() }, HEARTBEAT_MS)
    this.poll()
  }

  stop() {
    this.polling = false
    cancelAnimationFrame(this.raf)
    this.raf = 0
    clearInterval(this.beat)
    this.stopRumble()
    window.removeEventListener('keydown', this.keydown)
    window.removeEventListener('keyup', this.keyup)
    window.removeEventListener('blur', this.blur)
    window.removeEventListener('gamepadconnected', this.padEvent)
    window.removeEventListener('gamepaddisconnected', this.padEvent)
    document.removeEventListener('visibilitychange', this.visibility)
  }

  // Runs while there's something to read. With nothing connected, or nobody looking,
  // no frames are asked for at all.
  private tick = () => {
    this.raf = 0
    if (!this.polling) return
    this.poll()
  }

  private wantFrames() {
    return !this.hidden && (this.slots.size > 0 || (this.keyboardTurbo !== 0 && this.held.size > 0))
  }

  // ---- what's connected ----

  private rawPads(): Gamepad[] {
    try { return (navigator.getGamepads?.() || []).filter((p): p is Gamepad => !!p && p.connected !== false) } catch { return [] } // blocked by policy
  }

  private padEvent = () => { this.poll(); this.onChange?.() }

  private visibility = () => {
    this.hidden = document.hidden
    if (this.hidden) {
      // Browsers stop updating pads for a hidden page, so whatever was held would stay
      // held for as long as it's away.
      this.held.clear()
      this.stopRumble()
    }
    this.poll()
    this.flush(true)
  }

  listPads(): PadInfo[] {
    return this.rawPads().map((p) => {
      const s = this.slot(p)
      return {
        index: p.index, id: p.id, name: s.standing === 'known' || !parseModel(p.id).hasIds ? (knownPad(this.known, s.model)?.name || parseModel(p.id).name) : parseModel(p.id).name, model: s.model,
        standard: p.mapping === 'standard', standing: s.standing, style: s.style, port: s.port, raw: s.raw,
        rumble: !!((p as any).vibrationActuator || (p as any).hapticActuators?.length), profile: s.profile, hasMap: !!s.profile || s.layoutName === 'legacy',
      }
    })
  }

  // A pad keeps the player it had last time, if nobody else is on it; or takes the next
  // nobody else is on.
  private slot(p: PadLike): PadSlot {
    const cur = this.slots.get(p.index)
    if (cur) {
      if (cur.id === p.id) return cur
      this.slots.delete(p.index) // another pad has taken its place
    }
    const model = parseModel(p.id).key
    let nth = 0
    for (const s of this.slots.values()) if (s.model === model) nth++
    const seat = model + '#' + nth
    const used = new Set([...this.slots.values()].map((x) => x.port))
    let port = this.seats[seat]
    if (port === undefined || used.has(port) || port < 0 || port >= MAX_PLAYERS) {
      port = 0
      while (used.has(port) && port < MAX_PLAYERS - 1) port++
    }
    const s = { seat, index: p.index, id: p.id, model, port, reading: newReading(), lastStamp: -1 } as PadSlot
    this.bind(s, p)
    this.slots.set(p.index, s)
    this.onChange?.()
    return s
  }

  // Works out what a pad's buttons mean: its standing, its profile, and the list to read.
  private bind(s: PadSlot, p: PadLike, preview?: Profile | null) {
    const { standing, layout, known } = standingOf(p, this.known, navigator.userAgent)
    s.standing = standing
    s.style = known?.style || 'generic'
    s.layoutName = ''
    const raw = standing === 'unknown'
    const profile = preview !== undefined ? preview : resolveProfile(this.controls.profiles, 'gamepad', s.model, this.controls.console, this.game, raw)
    const legacy = !profile ? this.controls.saved.gamepad[padKey(p.id)] : undefined
    s.profile = profile
    s.raw = raw && (!!profile?.raw || !profile)
    let map: PadMap
    let useLayout = layout
    if (profile) {
      map = effectiveMap(this.controls.defaults.gamepad, this.targets, profile)
      if (profile.raw) useLayout = null
    } else if (legacy) {
      map = legacy
      s.layoutName = 'legacy'
      useLayout = null
    } else {
      map = this.controls.defaults.gamepad
    }
    s.map = map
    s.compiled = compile(p, map, useLayout, effectiveOptions(this.controls.defaults.options, profile))
    s.lastStamp = -1
  }

  // Tries out a profile on one pad, before it's saved.
  previewProfile(index: number, profile: Profile | null) {
    for (const p of this.rawPads()) if (p.index === index) this.bind(this.slot(p), p, profile)
  }

  // Who's on which player. Moving onto someone swaps with them, so nobody ends up
  // sharing by accident.
  setPadPort(index: number, port: number) {
    const mine = this.slots.get(index)
    if (!mine) return
    for (const s of this.slots.values()) if (s !== mine && s.port === port) { s.port = mine.port; this.seats[s.seat] = s.port }
    mine.port = port
    this.seats[mine.seat] = port
    save(SEATS_KEY, this.seats)
    this.flush(true)
    this.onChange?.()
  }
  get keyboardPlayer() { return this.keyboardPort }
  setKeyboardPort(port: number) { this.keyboardPort = port; this.flush(true); this.onChange?.() }
  get touchPlayer() { return this.touchPort }
  setTouchPort(port: number) { this.touchPort = port; this.flush(true); this.onChange?.() }

  setControls(c: ControlsData) {
    this.controls = c
    this.indexKeys()
    for (const p of this.rawPads()) {
      const s = this.slots.get(p.index)
      if (s && s.id === p.id) this.bind(s, p)
    }
  }

  // What a pad is doing right now, for the mapping screen.
  reading(index: number): Reading | null {
    return this.slots.get(index)?.reading ?? null
  }

  // ---- keyboard ----

  private keydown = (e: KeyboardEvent) => {
    if (!this.enabled || this.capturing || e.ctrlKey || e.altKey || e.metaKey) return
    const id = this.keyButton(e)
    if (id === undefined) return
    e.preventDefault()
    if (e.repeat) return
    this.held.add(id)
    this.flush()
    if (this.keyboardTurbo & (1 << id)) this.poll()
  }
  private keyup = (e: KeyboardEvent) => {
    const id = this.keyButton(e)
    if (id !== undefined && this.held.delete(id)) { e.preventDefault(); this.flush() }
  }
  // Keys go to whichever window has the focus, so letting go of one elsewhere is never
  // heard of here. Gamepads aren't focused: they carry on, so a second screen still works.
  private blur = () => { if (this.held.size) { this.held.clear(); this.flush() } }

  // ---- touch ----

  setTouch(mask: number) {
    if (mask !== this.touchMask) { this.touchMask = mask; this.flush() }
  }

  // The on-screen stick, -1 to 1 each way.
  setTouchStick(x: number, y: number) {
    if (x !== this.touchX || y !== this.touchY) { this.touchX = x; this.touchY = y; this.flush() }
  }

  // ---- the state of every player ----

  // Sees each player's buttons before the game does, and can keep some back: shortcuts.
  filter?: (port: number, mask: number) => number

  // A key some button already answers to, which a shortcut then leaves alone.
  isBound(code: string) { return this.keyOf.has(code) }

  private poll() {
    if (this.readPads()) this.flush()
    if (this.polling && !this.raf && this.wantFrames()) this.raf = requestAnimationFrame(this.tick)
  }

  // Reads everything connected. True if anything may have changed.
  private readPads(): boolean {
    if (this.hidden) return true
    let pads: readonly (Gamepad | null)[]
    try { pads = navigator.getGamepads?.() || [] } catch { pads = [] }
    const now = performance.now()
    let changed = false, seen = 0, menu = false
    for (let i = 0; i < pads.length; i++) {
      const p = pads[i]
      if (!p || p.connected === false) continue
      seen++
      const s = this.slot(p)
      if (s.standing !== 'standard' && noteHat(p)) this.bind(s, p)
      // Nothing new since last time, and nothing timed to do: no need to read it again.
      const stamp = p.timestamp || 0
      const turbo = s.compiled.turbo !== 0 && (s.reading.mask !== 0 || s.compiled.prevMask !== 0)
      if (stamp === 0 || stamp !== s.lastStamp || turbo) {
        s.lastStamp = stamp
        readPad(s.compiled, p, now, s.reading)
        changed = true
      }
      if (!this.capturing && menuHeld(s.compiled, p)) menu = true
    }
    if (seen !== this.slots.size) { this.forgetGone(pads); changed = true }
    if (menu !== this.menuDown) {
      this.menuDown = menu
      if (menu) this.onMenu?.()
    }
    if (changed) this.onFrame?.()
    return changed || this.keyboardTurbo !== 0 || this.pulseMask !== 0
  }

  // A pad that's gone lets go of everything: its player is simply no longer pressing.
  private forgetGone(pads: readonly (Gamepad | null)[]) {
    const here = new Set<number>()
    for (const p of pads) if (p && p.connected !== false) here.add(p.index)
    let gone = false
    for (const k of [...this.slots.keys()]) if (!here.has(k)) { this.slots.delete(k); gone = true }
    if (gone) this.onChange?.()
  }

  // Puts every player's state together, and sends what's changed. Allocates nothing
  // unless a state has to go out.
  private flush(force = false): boolean {
    const b = this.sumB, a = this.sumA, t = this.sumT
    b.fill(0); a.fill(0); t.fill(0)
    if (this.enabled && !this.hidden) {
      for (const id of this.held) this.press(this.keyboardPort, id)
      if (this.keyboardTurbo && Math.floor((performance.now() * this.keyboardHz * 2) / 1000) % 2) b[this.keyboardPort] &= ~this.keyboardTurbo
      for (let id = 0; id < TARGETS; id++) if (this.touchMask & (1 << id)) this.press(this.touchPort, id)
      for (let id = 0; id < TARGETS; id++) if (this.pulseMask & (1 << id)) this.press(0, id)
      if (this.touchX || this.touchY) { a[this.touchPort * 4] = this.touchX; a[this.touchPort * 4 + 1] = this.touchY }
      for (const s of this.slots.values()) {
        const r = s.reading, n = s.port * 4
        b[s.port] |= r.mask
        if (Math.abs(r.lx) > Math.abs(a[n])) a[n] = r.lx
        if (Math.abs(r.ly) > Math.abs(a[n + 1])) a[n + 1] = r.ly
        if (Math.abs(r.rx) > Math.abs(a[n + 2])) a[n + 2] = r.rx
        if (Math.abs(r.ry) > Math.abs(a[n + 3])) a[n + 3] = r.ry
        if (r.l2 > t[s.port * 2]) t[s.port * 2] = r.l2
        if (r.r2 > t[s.port * 2 + 1]) t[s.port * 2 + 1] = r.r2
      }
      if (this.filter) for (let port = 0; port < MAX_PLAYERS; port++) b[port] = this.filter(port, b[port])
    }
    let sent = false
    const msg = this.scratch, v = this.scratchView
    for (let port = 0; port < MAX_PLAYERS; port++) {
      const n = port * 4
      msg[0] = port
      v.setUint16(1, b[port] & 0xffff, true)
      for (let i = 0; i < 4; i++) v.setInt16(3 + i * 2, Math.round(a[n + i] * 32767), true)
      msg[11] = Math.round(t[port * 2] * 255)
      msg[12] = Math.round(t[port * 2 + 1] * 255)
      const prev = this.last[port]
      let same = true
      for (let i = 0; i < 13 && same; i++) same = prev[i] === msg[i]
      if (same && !force) continue
      if (same && force && port > 0) {
        let idle = true
        for (let i = 1; i < 13 && idle; i++) idle = msg[i] === 0
        if (idle) continue // idle players need no heartbeat
      }
      prev.set(msg)
      this.send(msg)
      sent = true
    }
    if (sent) this.onActivity?.()
    return sent
  }

  // One thing pressed, whatever pressed it: a button, or a stick pushed all the way.
  private press(port: number, id: number) {
    if (id < 16) { this.sumB[port] |= 1 << id; return }
    const half = id - STICK_LX_NEG
    const axis = port * 4 + (half >> 1)
    this.sumA[axis] = half & 1 ? 1 : -1
  }

  // ---- rumble ----

  // effect 0 is the strong motor and 1 the weak one, each 0 to 65535.
  rumble(port: number, strength: number, effect = 0) {
    let m = this.motors.get(port)
    if (!m) { m = { strong: 0, weak: 0, timer: 0 }; this.motors.set(port, m) }
    const level = Math.max(0, Math.min(1, strength / 0xffff))
    if (effect === 1) m.weak = level
    else m.strong = level
    this.applyRumble(port, m)
  }

  private applyRumble(port: number, m: Motors) {
    clearTimeout(m.timer)
    m.timer = 0
    if (this.hidden || !this.enabled) return
    let done = false
    const on = m.strong > 0 || m.weak > 0
    for (const p of this.rawPads()) {
      const s = this.slots.get(p.index)
      if (!s || s.port !== port) continue
      done = true
      if (s.compiled.noRumble) continue
      const act = (p as any).vibrationActuator
      if (act?.playEffect) {
        if (on) act.playEffect('dual-rumble', { duration: RUMBLE_MS, strongMagnitude: m.strong, weakMagnitude: m.weak }).catch(() => {})
        else if (act.reset) act.reset().catch?.(() => {})
        else act.playEffect('dual-rumble', { duration: 0, strongMagnitude: 0, weakMagnitude: 0 }).catch(() => {})
      } else if (on) {
        (p as any).hapticActuators?.[0]?.pulse?.(Math.max(m.strong, m.weak), RUMBLE_MS)?.catch?.(() => {})
      }
    }
    if (on && done) m.timer = window.setTimeout(() => this.applyRumble(port, m), RUMBLE_MS - 100)
    if (!done && on && port === this.touchPort && (navigator as any).userActivation?.hasBeenActive !== false) navigator.vibrate?.(30)
  }

  private stopRumble() {
    for (const [port, m] of this.motors) {
      if (!m.strong && !m.weak) { clearTimeout(m.timer); continue }
      m.strong = 0
      m.weak = 0
      const wasEnabled = this.enabled, wasHidden = this.hidden
      this.enabled = true
      this.hidden = false
      this.applyRumble(port, m)
      this.enabled = wasEnabled
      this.hidden = wasHidden
    }
  }

  // A short buzz to tell which pad is which.
  testRumble(index: number) {
    for (const p of this.rawPads()) {
      if (p.index !== index) continue
      const act = (p as any).vibrationActuator
      if (act?.playEffect) act.playEffect('dual-rumble', { duration: 300, strongMagnitude: 0.8, weakMagnitude: 0.8 }).catch(() => {})
      else (p as any).hapticActuators?.[0]?.pulse?.(0.8, 300)?.catch?.(() => {})
    }
  }
}
