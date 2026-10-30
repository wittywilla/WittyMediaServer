import { DOWN, LEFT, RIGHT, UP, padFor } from './consoles'
import { load, save } from './input'

// On-screen controls: where each one sits, saved per console, per device and
// per orientation. All numbers are fractions of the play area, so a layout
// survives a change of window size.

export type ControlKind = 'dpad' | 'face' | 'shoulder' | 'meta'
export type Control = { key: string; id: number; label: string; kind: ControlKind }
export type Placement = { x: number; y: number; size: number; opacity: number; hidden?: boolean }
export type Layout = Record<string, Placement>
export type Orientation = 'landscape' | 'portrait'

export const DPAD_KEY = 'dpad'

export function controlList(consoleId: string): Control[] {
  const p = padFor(consoleId)
  const k = (b: { id: number; label: string }, kind: ControlKind): Control => ({ key: 'b' + b.id, id: b.id, label: b.label, kind })
  return [
    { key: DPAD_KEY, id: -1, label: 'D-pad', kind: 'dpad' },
    ...p.face.map((b) => k(b, 'face')),
    ...(p.shoulder || []).map((b) => k(b, 'shoulder')),
    ...p.meta.map((b) => k(b, 'meta')),
  ]
}

const OPACITY = 0.55

// Sizes are a fraction of the play area's shorter side, so a button stays the
// size of a thumb on any screen.
export function defaultLayout(consoleId: string, o: Orientation): Layout {
  const pad = padFor(consoleId)
  const out: Layout = {}
  const land = o === 'landscape'
  const fy = land ? 0.62 : 0.72 // height of the D-pad and main buttons
  out[DPAD_KEY] = { x: land ? 0.13 : 0.24, y: fy, size: land ? 0.36 : 0.3, opacity: OPACITY }
  const cx = land ? 0.87 : 0.76
  const s = land ? 0.15 : 0.125 // main button size
  const dx = land ? 0.085 : 0.15, dy = land ? 0.17 : 0.085
  const face = pad.face
  face.forEach((b, i) => {
    let x = cx, y = fy
    if (face.length === 2) { x = cx + (i === 0 ? -dx * 0.75 : dx * 0.75); y = fy + (i === 0 ? dy * 0.4 : -dy * 0.4) }
    else if (face.length === 3) { x = cx + (i - 1) * dx * 1.0; y = fy + (i === 1 ? dy * 0.2 : i === 0 ? dy * 0.5 : -dy * 0.1) }
    else if (face.length >= 4) {
      // A diamond, listed as left, top, bottom, right.
      const pos = [[-dx, 0], [0, -dy], [0, dy], [dx, 0]][i] || [0, 0]
      x = cx + pos[0]; y = fy + pos[1]
    }
    out['b' + b.id] = { x, y, size: s, opacity: OPACITY }
  })
  const shoulders = pad.shoulder || []
  const top = land ? 0.14 : 0.56
  shoulders.forEach((b, i) => {
    const left = i % 2 === 0
    const row = Math.floor(i / 2)
    if (shoulders.length === 3) { // three in a row above the right-hand buttons
      out['b' + b.id] = { x: cx + (i - 1) * dx * 0.95, y: top + (land ? 0.1 : 0), size: s * 0.8, opacity: OPACITY }
      return
    }
    out['b' + b.id] = { x: (left ? 0.1 : 0.9) + (left ? 1 : -1) * row * (land ? 0.11 : 0.18), y: top, size: s * 0.8, opacity: OPACITY }
  })
  const meta = pad.meta
  meta.forEach((b, i) => {
    const offset = meta.length === 1 ? 0 : i === 0 ? -1 : 1
    out['b' + b.id] = { x: 0.5 + offset * (land ? 0.065 : 0.11), y: land ? 0.91 : 0.93, size: s * 0.72, opacity: OPACITY }
  })
  return out
}

const key = (consoleId: string, o: Orientation) => `wittyplay.touch.${consoleId}.${o}`

export function loadLayout(consoleId: string, o: Orientation): Layout {
  const def = defaultLayout(consoleId, o)
  const saved = load<Layout>(key(consoleId, o))
  if (!saved) return def
  // Anything missing (a console gained a button) comes from the default, and
  // nonsense from storage is put back within range.
  const out: Layout = {}
  for (const k of Object.keys(def)) {
    const p = saved[k]
    out[k] = p && [p.x, p.y, p.size, p.opacity].every((v) => typeof v === 'number' && isFinite(v))
      ? { x: clamp(p.x, 0, 1), y: clamp(p.y, 0, 1), size: clamp(p.size, 0.05, 0.6), opacity: clamp(p.opacity, 0.1, 1), hidden: !!p.hidden }
      : def[k]
  }
  return out
}

export const saveLayout = (consoleId: string, o: Orientation, l: Layout | null) => save(key(consoleId, o), l)

export const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))

// Which directions a point on the D-pad means. The centre is a dead zone, and the
// corners are diagonals.
export function dpadMask(dx: number, dy: number): number {
  const dist = Math.hypot(dx, dy)
  if (dist < 0.18) return 0
  const a = Math.atan2(dy, dx) // -PI..PI, 0 is right, clockwise on screen
  const sector = Math.round(a / (Math.PI / 4)) // -4..4
  switch (sector) {
    case 0: return 1 << RIGHT
    case 1: return (1 << RIGHT) | (1 << DOWN)
    case 2: return 1 << DOWN
    case 3: return (1 << DOWN) | (1 << LEFT)
    case 4: case -4: return 1 << LEFT
    case -3: return (1 << LEFT) | (1 << UP)
    case -2: return 1 << UP
    case -1: return (1 << UP) | (1 << RIGHT)
  }
  return 0
}

export type Rect = { w: number; h: number }

// The mask for a set of finger positions, in pixels within the play area. A finger
// counts for a button slightly larger than it looks, and a finger between two close
// buttons presses both, which is how people press them together on real pads.
export function touchMask(points: { x: number; y: number }[], controls: Control[], layout: Layout, r: Rect): number {
  const unit = Math.min(r.w, r.h)
  let mask = 0
  for (const pt of points) {
    for (const c of controls) {
      const p = layout[c.key]
      if (!p || p.hidden) continue
      const cx = p.x * r.w, cy = p.y * r.h
      const half = (p.size * unit) / 2
      if (c.kind === 'dpad') {
        const dx = (pt.x - cx) / half, dy = (pt.y - cy) / half
        if (Math.hypot(dx, dy) <= 1.35) mask |= dpadMask(dx, dy)
      } else if (Math.hypot(pt.x - cx, pt.y - cy) <= half * 1.18) {
        mask |= 1 << c.id
      }
    }
  }
  return mask
}
