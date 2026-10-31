import type { DisplayValues } from './display'

// Shortcuts: from a keyboard, or a controller by holding one button and pressing
// another. Both work on RetroPad buttons as the input layer has already mapped them, so
// a remapped pad's shortcuts follow its mapping.

export type HotAction = 'fastForward' | 'rewind' | 'slow' | 'quickSave' | 'quickLoad' | 'pause' | 'advance' | 'screenshot' | 'menu' | 'mute' | 'osd'

// Held down while wanted; the rest happen once at the press.
export const HOLD_ACTIONS: HotAction[] = ['fastForward', 'rewind', 'slow']

const BIT: Record<string, number> = { b: 0, y: 1, select: 2, start: 3, up: 4, down: 5, left: 6, right: 7, a: 8, x: 9, l: 10, r: 11, l2: 12, r2: 13, l3: 14, r3: 15 }
const PAD_ACTIONS: HotAction[] = ['fastForward', 'rewind', 'quickSave', 'quickLoad', 'slow', 'pause', 'advance', 'screenshot', 'menu']
const KEY_ACTIONS: HotAction[] = ['fastForward', 'rewind', 'quickSave', 'quickLoad', 'slow', 'pause', 'advance', 'screenshot', 'mute', 'osd']

export class Hotkeys {
  enabled = true
  private keyOf = new Map<string, HotAction>()
  private padOf: { action: HotAction; bit: number }[] = []
  private modifier = 1 << BIT.select
  private combos = true
  private held = new Set<HotAction>()
  private lastMask = 0
  private swallowed = 0

  constructor(private fire: (a: HotAction, down: boolean) => void, private keyInUse: (code: string) => boolean) {}

  configure(v: DisplayValues) {
    this.keyOf.clear()
    for (const a of KEY_ACTIONS) {
      const code = v['hotkeys.key.' + a]
      if (typeof code === 'string' && code) this.keyOf.set(code, a)
    }
    this.padOf = []
    for (const a of PAD_ACTIONS) {
      const name = v['hotkeys.pad.' + a]
      if (typeof name === 'string' && BIT[name] !== undefined) this.padOf.push({ action: a, bit: 1 << BIT[name] })
    }
    this.modifier = 1 << (BIT[String(v['hotkeys.padModifier'])] ?? BIT.select)
    this.combos = v['hotkeys.padCombos'] !== false
  }

  start() {
    window.addEventListener('keydown', this.down)
    window.addEventListener('keyup', this.up)
    window.addEventListener('blur', this.letGo)
  }

  stop() {
    window.removeEventListener('keydown', this.down)
    window.removeEventListener('keyup', this.up)
    window.removeEventListener('blur', this.letGo)
    this.letGo()
  }

  private press(a: HotAction, on: boolean) {
    if (on) {
      if (this.held.has(a)) return
      this.held.add(a)
    } else {
      if (!this.held.delete(a)) return
      if (!HOLD_ACTIONS.includes(a)) return
    }
    this.fire(a, on)
  }

  // Leaving the window while holding a key must not leave a game fast-forwarding.
  private letGo = () => { for (const a of [...this.held]) this.press(a, false) }

  private down = (e: KeyboardEvent) => {
    if (!this.enabled || e.ctrlKey || e.altKey || e.metaKey) return
    const a = this.keyOf.get(e.code)
    if (!a || this.keyInUse(e.code)) return
    e.preventDefault()
    if (!e.repeat) this.press(a, true)
  }

  private up = (e: KeyboardEvent) => {
    const a = this.keyOf.get(e.code)
    if (a) this.press(a, false)
  }

  // A player's buttons on their way to the game. While the shortcut button is held,
  // anything assigned to a shortcut does that instead of reaching the game, and stays
  // kept from it until let go, however the two are released.
  filter(mask: number): number {
    if (!this.combos || !this.enabled) return mask
    const held = (mask & this.modifier) !== 0
    if (held) {
      for (const { action, bit } of this.padOf) {
        const on = (mask & bit) !== 0, was = (this.lastMask & bit) !== 0
        if (on && !was) {
          this.swallowed |= bit
          this.press(action, true)
        }
      }
    }
    for (const { action, bit } of this.padOf) {
      if ((this.swallowed & bit) !== 0 && (mask & bit) === 0) {
        this.swallowed &= ~bit
        this.press(action, false)
      }
    }
    this.lastMask = mask
    return mask & ~this.swallowed
  }
}
