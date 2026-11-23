// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// How a module's bundle is loaded once and held, apart from React and the network so it can be
// tested. What is kept is the module itself: whether it has what one caller wants (a page, a
// view, a reader) is that caller's question, asked each time. Only a bundle that won't load or
// won't run is reported as failing: an address naming a page a module doesn't have is a page
// not found, and never counts against the module.

export type Loaded = Record<string, unknown>

// bundleCache loads each bundle once, whoever asks first, and reports it if loading fails.
export function bundleCache<B>(key: (b: B) => string, load: (b: B) => Promise<Loaded>, report: (b: B, why: unknown) => void) {
  const held = new Map<string, Promise<Loaded>>()
  return (b: B): Promise<Loaded> => {
    let p = held.get(key(b))
    if (!p) {
      p = load(b)
      p.catch((why) => report(b, why))
      held.set(key(b), p)
    }
    return p
  }
}

// What a module offers for one ask: absent when it's loaded but has no such thing.
export type Picked<T> = { found: true; value: T } | { found: false }

// pickFrom asks a loaded module for one thing, with no say over the module's health.
export function pickFrom<T>(m: Loaded, pick: (m: Loaded) => unknown, ok: (x: unknown) => boolean): Picked<T> {
  let x: unknown
  try { x = pick(m) } catch { return { found: false } }
  return ok(x) ? { found: true, value: x as T } : { found: false }
}

// A page's id from an address, as a module names them: anything else is no page at all, and
// never a property of the bundle (constructor, __proto__).
export const pageOf = (pages: unknown, id: string): unknown =>
  pages && typeof pages === 'object' && Object.prototype.hasOwnProperty.call(pages, id) ? (pages as Record<string, unknown>)[id] : undefined
