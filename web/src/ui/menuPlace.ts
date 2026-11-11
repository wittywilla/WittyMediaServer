// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Where a menu opens (ui/Menu.tsx), as arithmetic only, so it can be tested and ported.

export type Box = { left: number; top: number; right: number; bottom: number }
export type Size = { width: number; height: number }
export type Place = { left: number; top: number; maxHeight: number }

const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(v, Math.max(lo, hi)))

// placeMenu: below its button, or above when there's more room there, lined up with the button's
// start or end, and always wholly on screen, however tall or wide it is or wherever the button is.
// With no button to measure (it isn't laid out), it opens at the top of the screen on that side
// rather than not at all.
export function placeMenu(anchor: Box | null, panel: Size, view: Size, align: 'start' | 'end', edge = 8): Place {
  const a = anchor ?? (align === 'end'
    ? { left: view.width - edge, right: view.width - edge, top: edge, bottom: edge }
    : { left: edge, right: edge, top: edge, bottom: edge })
  const below = view.height - a.bottom - edge * 2, above = a.top - edge * 2
  const up = panel.height > below && above > below
  const maxHeight = Math.max(Math.min(120, view.height - edge * 2), Math.min(view.height - edge * 2, up ? above : below))
  const height = Math.min(panel.height, maxHeight)
  const left = align === 'end' ? a.right - panel.width : a.left
  const top = up ? a.top - height - edge : a.bottom + edge
  return { left: clamp(left, edge, view.width - panel.width - edge), top: clamp(top, edge, view.height - height - edge), maxHeight }
}

// movedAway: whether the page moved a menu's button since it opened, which is when a scroll should
// close the menu. A list scrolling elsewhere, or a scroll event with no movement (Firefox sends
// some), leaves it where it is, and so leaves the menu open.
export function movedAway(then: Box | null, now: Box | null, slack = 2) {
  if (!then || !now) return true
  return Math.abs(now.top - then.top) > slack || Math.abs(now.left - then.left) > slack
}
