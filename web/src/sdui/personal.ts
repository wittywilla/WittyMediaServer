// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// "Customise this page": what a profile changed about a page, worked out from the list
// the server sent (document.personal) and the changes made to it, as plain data
// (docs/SDUI.md, "Personalisation"). The page shows the rows and PUTs `body`.

import type { Personal, PersonalInfo, PersonalSection } from './types.ts'

export type Draft = { rows: PersonalSection[]; density: string; cardSize: Record<string, string>; defaultTab: Record<string, string> }

export const DENSITIES = ['compact', 'comfortable', 'spacious']
export const CARD_SIZES = ['s', 'm', 'l']

export const allows = (info: PersonalInfo | undefined, what: string) => !!info?.editable && (info.allow || []).includes(what)

export function draftOf(info: PersonalInfo | undefined): Draft {
  return {
    rows: (info?.sections || []).map((s) => ({ ...s })),
    density: info?.density || 'comfortable',
    cardSize: { ...(info?.cardSize || {}) },
    defaultTab: { ...(info?.defaultTab || {}) },
  }
}

// Pinned rows come first, in the order they were pinned; a locked row never moves.
function settle(rows: PersonalSection[]): PersonalSection[] {
  const free = rows.filter((r) => !r.locked)
  const ordered = [...free.filter((r) => r.pinned), ...free.filter((r) => !r.pinned)]
  let k = 0
  return rows.map((r) => (r.locked ? r : ordered[k++]))
}

export function move(d: Draft, id: string, to: number): Draft {
  const from = d.rows.findIndex((r) => r.id === id)
  if (from < 0 || d.rows[from].locked) return d
  const target = Math.max(0, Math.min(d.rows.length - 1, to))
  if (target === from || d.rows[target].locked) return d
  // A pinned row moves among the pinned ones, and the others among themselves.
  if (!!d.rows[target].pinned !== !!d.rows[from].pinned) return d
  const rows = [...d.rows]
  rows.splice(target, 0, ...rows.splice(from, 1))
  return { ...d, rows: settle(rows) }
}

export function setHidden(d: Draft, id: string, hidden: boolean): Draft {
  return { ...d, rows: d.rows.map((r) => (r.id === id && !r.locked ? { ...r, hidden: hidden || undefined, pinned: hidden ? undefined : r.pinned } : r)) }
}

export function setPinned(d: Draft, id: string, pinned: boolean): Draft {
  const row = d.rows.find((r) => r.id === id)
  if (!row || row.locked) return d
  const changed = { ...row, pinned: pinned || undefined, hidden: pinned ? undefined : row.hidden }
  // Newly pinned goes after the ones pinned before it; unpinned goes to the top of the rest.
  // Locked rows keep their places throughout.
  const free = d.rows.filter((r) => !r.locked && r.id !== id)
  const lastPinned = free.reduce((n, r, i) => (r.pinned ? i : n), -1)
  free.splice(lastPinned + 1, 0, changed)
  let k = 0
  return { ...d, rows: settle(d.rows.map((r) => (r.locked ? r : free[k++]))) }
}

export function setCardSize(d: Draft, id: string, size: string): Draft {
  const cardSize = { ...d.cardSize }
  if (CARD_SIZES.includes(size) && size !== 'm') cardSize[id] = size; else delete cardSize[id]
  return { ...d, cardSize }
}

export function setDefaultTab(d: Draft, tabs: string, tab: string): Draft {
  const defaultTab = { ...d.defaultTab }
  if (tab) defaultTab[tabs] = tab; else delete defaultTab[tabs]
  return { ...d, defaultTab }
}

export const setDensity = (d: Draft, density: string): Draft => ({ ...d, density: DENSITIES.includes(density) ? density : 'comfortable' })

// body: what to PUT. Only what the page allows is sent.
export function body(d: Draft, info: PersonalInfo | undefined): Personal {
  const out: Personal = {}
  if (allows(info, 'reorder')) out.order = d.rows.filter((r) => !r.locked && !r.pinned).map((r) => r.id)
  if (allows(info, 'hide')) out.hidden = d.rows.filter((r) => r.hidden && !r.locked).map((r) => r.id)
  if (allows(info, 'pin')) out.pinned = d.rows.filter((r) => r.pinned && !r.locked).map((r) => r.id)
  if (allows(info, 'density') && d.density !== 'comfortable') out.density = d.density
  if (allows(info, 'cardSize') && Object.keys(d.cardSize).length) out.cardSize = d.cardSize
  if (allows(info, 'defaultTab') && Object.keys(d.defaultTab).length) out.defaultTab = d.defaultTab
  return out
}

export const same = (a: Draft, b: Draft) => JSON.stringify(a) === JSON.stringify(b)

// fromOldHome: Home's order and hidden rows as the web app kept them before pages were
// server-driven (/me/ui `home`), as an arrangement. The built-in Home's sections have the
// same IDs, and the three "continue" rows had already become "jump".
export function fromOldHome(old: unknown): Personal | null {
  if (!old || typeof old !== 'object') return null
  const o = old as { v?: number; order?: unknown; hidden?: unknown }
  const ids = (v: unknown) => (Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [])
  const hidden = [...new Set(ids(o.hidden).map((h) => (h.startsWith('continue') ? 'jump' : h)))]
  const order = o.v === 2 ? ids(o.order) : []
  if (!hidden.length && !order.length) return null
  return { ...(order.length ? { order } : {}), ...(hidden.length ? { hidden } : {}) }
}

// fromOldNav: the main menu's order and hidden entries from the web app's own `nav`
// choice, in the navigation's IDs ("lib:3" was a library; it is "lib-3" now).
export function fromOldNav(old: unknown): Personal | null {
  if (!old || typeof old !== 'object') return null
  const o = old as { order?: unknown; hidden?: unknown }
  const id = (s: string) => (s.startsWith('lib:') ? 'lib-' + s.slice(4) : s === 'tv' ? 'livetv' : s)
  const ids = (v: unknown) => (Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string').map(id) : [])
  const order = ids(o.order), hidden = ids(o.hidden).filter((h) => h !== 'settings')
  if (!order.length && !hidden.length) return null
  return { ...(order.length ? { order } : {}), ...(hidden.length ? { hidden } : {}) }
}
