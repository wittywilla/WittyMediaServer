// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Something over the page that Back closes: a phone's Back gesture, the browser's Back button
// and a mouse's back button, not only Esc and a remote's Back key (a11y.tsx). While it's open,
// one entry in the history carries the page's own state with a mark of its own, so the router
// sees the same place; the full audio player and WittyPlay's game menu do the same. Closing it
// any other way (its ×, Esc, a tap outside) takes the entry back out, so Back then does what it
// did before. Something opened over it with an entry of its own goes first.
//
// useBackCloses in Overlay.tsx is this for a component.

export type HistoryLike = { readonly state: unknown; pushState(data: unknown, unused: string): void; back(): void }
export type PopTarget = { addEventListener(type: 'popstate', f: () => void): void; removeEventListener(type: 'popstate', f: () => void): void }

const marked = (state: unknown, mark: string) => !!state && typeof state === 'object' && (state as Record<string, unknown>)[mark] === true

// openBackEntry: for the time something is open. close runs when Back takes the entry away. The
// function it returns is for when it closes any other way.
export function openBackEntry(history: HistoryLike, target: PopTarget, mark: string, close: () => void): () => void {
  const state = history.state && typeof history.state === 'object' ? history.state : {}
  history.pushState({ ...state, [mark]: true }, '')
  let gone = false
  const pop = () => {
    // Still ours: only something over this went.
    if (gone || marked(history.state, mark)) return
    gone = true
    target.removeEventListener('popstate', pop)
    close()
  }
  target.addEventListener('popstate', pop)
  return () => {
    target.removeEventListener('popstate', pop)
    if (gone) return
    gone = true
    if (marked(history.state, mark)) history.back()
  }
}
