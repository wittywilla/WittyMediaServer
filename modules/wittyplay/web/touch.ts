import { DOWN, LEFT, RIGHT, UP, type Control, type ControlsData, type Layout, type Orientation } from './consoles'

// On-screen controls. Layouts come from the server, and all numbers in them are
// fractions of the play area, so a layout survives a change of window size. What
// is left here is what only a touch screen needs: turning fingers into buttons.

export function layoutFor(c: ControlsData, o: Orientation): Layout {
  return { ...c.defaults.touch[o], ...(c.saved.touch[o] || {}) }
}

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
