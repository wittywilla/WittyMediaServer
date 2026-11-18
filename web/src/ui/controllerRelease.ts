// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Leaving controller styling when the mouse or a finger takes over, without moving the page under the pointer.
//
// `using-controller` makes the page bigger (the root type size, the gutters, the card sizes). It used to come off on
// the first mouse press, so the page re-laid out between the press and the release and the click landed on whatever
// had moved under the pointer, on every page. Now it comes off only when the pointer is MOVED (so the person sees the
// settled page before aiming), or, for a press with no movement before it (a finger, or a click without moving), only
// AFTER the click has been delivered. The first click always hits what was under the pointer. A screen set up for
// across the room (ten-foot) keeps the class: it never leaves it.

export type Html = { classList: { contains(c: string): boolean; remove(c: string): void } }
export type Win = { addEventListener(type: string, fn: (e: any) => void, o?: unknown): void }

export const CLASS = 'using-controller'

export function installControllerRelease(win: Win, html: Html, schedule: (fn: () => void, ms: number) => unknown = setTimeout) {
  let armed = false
  const release = () => { if (!html.classList.contains('ten-foot')) html.classList.remove(CLASS) }
  // A mouse that really moved: the layout settles before the person aims at anything.
  win.addEventListener('pointermove', (e) => {
    if (e.pointerType === 'mouse' && (e.movementX || e.movementY)) release()
  }, { passive: true })
  // A press never changes the layout itself; it only notes that the pointer is in use.
  win.addEventListener('pointerdown', () => { armed = true }, { passive: true, capture: true })
  // After the click has been delivered (a task later), the styling goes.
  win.addEventListener('click', () => { if (armed) { armed = false; schedule(release, 0) } }, { passive: true })
  // A press that never becomes a click (a drag, a cancelled touch): still release, a moment later.
  const noClick = () => { if (armed) schedule(() => { if (armed) { armed = false; release() } }, 600) }
  win.addEventListener('pointerup', noClick, { passive: true })
  win.addEventListener('pointercancel', noClick, { passive: true })
}
