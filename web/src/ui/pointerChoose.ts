// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// "A pointer only points": how a row or grid of cards with a chosen one (a details panel beside it, or the chosen card
// drawn larger) answers a mouse, a remote, a pad and the keyboard alike.
//
// - Arrows, a remote and a pad move the focus, and the focus chooses (the row's own onFocus does that).
// - A pointer passing over a card only shows a hover style. It never chooses: when it did, crossing other cards on the
//   way to the details' buttons changed the details, and a chosen card that grows moved the layout under the pointer,
//   so a click landed on another card (WittyPlay #91; Recent lost the fix in #558).
// - A click on a card that isn't the chosen one chooses it, and nothing opens.
// - A click on the chosen card, a double-click, or Enter (a click with no mouse behind it: detail 0) opens it.
//
// Whether the card pressed was already the chosen one is read as the press begins, before the focus the press brings
// has chosen it.

/** clickOpens: whether a click should go on and open the card, rather than only choose it. */
export function clickOpens(detail: number, wasChosen: boolean): boolean {
  return detail === 0 || detail > 1 || wasChosen
}

type ClickLike = { detail: number; preventDefault(): void; stopPropagation(): void }

/**
 * pointerChooser makes the two handlers each card gets: put them on the card's wrapper in the capture phase
 * (onPointerDownCapture, onClickCapture) so a link or button inside opens only when it should.
 */
export function pointerChooser(chosen: () => string | null | undefined, choose: (id: string) => void) {
  let pressed: string | null = null
  return (id: string) => ({
    onPointerDownCapture: () => { pressed = chosen() === id ? id : null },
    onClickCapture: (e: ClickLike) => {
      const opens = clickOpens(e.detail, pressed === id)
      pressed = null
      if (opens) return
      e.preventDefault()
      e.stopPropagation()
      choose(id)
    },
  })
}
