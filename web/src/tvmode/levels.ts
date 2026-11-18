// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// How much of each screen a television offers (docs/TV-MODE.md). The table is the server's
// (pkg/sdui/data/tvlevels.json, the catalogue's tvLevels), so the web app's TV mode and every app's
// TV screens follow the same list. These are its rules with nothing from the page, so they can be
// tested (levels.test.mjs); levelTable.ts holds the table itself.

export type TvLevel = 'full' | 'view' | 'desktopOnly'
export type TvScreen = { id: string; path: string; level: TvLevel; title: string; summary?: string }
export type TvPart = { id: string; level: TvLevel; title: string }
export type TvLevelTable = { levels: string[]; default: string; note: string; noteDesktop?: string; screens: TvScreen[]; parts: TvPart[] }

// pathMatches: ":name" is one part of the address, and "*" at the end is the rest of it, or nothing.
export function pathMatches(pattern: string, path: string): boolean {
  const pp = pattern.replace(/^\/+|\/+$/g, '').split('/')
  const trimmed = path.replace(/^\/+|\/+$/g, '')
  const ap = trimmed ? trimmed.split('/') : []
  for (let i = 0; i < pp.length; i++) {
    if (pp[i] === '*' && i === pp.length - 1) return true
    if (i >= ap.length) return false
    if (pp[i].startsWith(':')) { if (!ap[i]) return false; continue }
    if (pp[i] !== ap[i]) return false
  }
  return ap.length === pp.length
}

// screenFor: the screen an address is, or null for one that is full on a television. The first
// match wins, as on the server (sdui.TVScreenFor).
export function screenFor(table: TvLevelTable, path: string): TvScreen | null {
  const clean = path.split(/[?#]/)[0]
  return table.screens.find((s) => pathMatches(s.path, clean)) || null
}

export function partFor(table: TvLevelTable, id: string): TvPart | null {
  return table.parts.find((p) => p.id === id) || null
}

// strictest: of two levels, the one that offers less (a module's own level and the table's).
export function strictest(a: TvLevel | undefined, b: TvLevel | undefined): TvLevel {
  const rank = { full: 0, view: 1, desktopOnly: 2 }
  const x = a && a in rank ? a : 'full'
  const y = b && b in rank ? b : 'full'
  return rank[x] >= rank[y] ? x : y
}

// pageAddress: the address to give for a page, to open it on a computer or a phone. The address
// the television uses may be the desktop app's own (localhost) or an app's, which nothing else
// can open, so then the server's first address on the home network is given instead.
export function pageAddress(origin: string, path: string, home: string[]): string {
  let host = ''
  try { host = new URL(origin).hostname } catch { /* not an address */ }
  const local = !host || host === 'localhost' || host.endsWith('.localhost') || host === '127.0.0.1' || host === '[::1]' || host === '::1'
  const base = (local && home[0]) || origin
  return base.replace(/\/+$/, '') + (path.startsWith('/') ? path : '/' + path)
}

// Which wording TV mode uses (docs/TV-MODE.md). A TV device (the Android TV and Apple TV apps, Plasma
// Bigscreen, a TV's own browser) has nowhere else to go, so it names the WittyMediaClient app on a computer and
// shows the address and a QR code. A computer or a phone in TV mode can simply leave it. Decided by what the
// client says it is, never by the size of the window:
//   client  "tv" or "desktop", from ?client= (the apps add it), remembered on this device
//   ua      the browser's own name: a TV's browser says so
//   shell   the WittyMediaClient desktop app's own window
export type TvClient = 'tvDevice' | 'computer'
export const TV_BROWSER = /\b(SmartTV|SMART-TV|Tizen|Web0S|WebOS|NetCast|BRAVIA|GoogleTV|Android TV|AFT[A-Z]+|CrKey|HbbTV|Roku|PlayStation|Xbox)\b/i
export function tvClientOf(f: { client?: string | null; ua?: string; shell?: boolean }): TvClient {
  if (f.client === 'tv') return 'tvDevice'
  if (f.client === 'desktop' || f.shell) return 'computer'
  return TV_BROWSER.test(f.ua || '') ? 'tvDevice' : 'computer'
}
