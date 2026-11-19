// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useSyncExternalStore } from 'react'
import { tvClientOf, type TvClient } from './levels'

// TV mode: the website laid out and driven like the Apple TV and Android TV apps (docs/plans/TV-MODE.md).
// It's per device, kept in this browser. `?tv=1` in the address turns it on and `?tv=0` off, for a TV
// browser with no easy way to reach Settings; the desktop app's --tv and Plasma Bigscreen send `?tv=1`.
// It took the place of Big screen mode, so someone who had that on finds this on.

const KEY = 'wms.tvmode'
const OLD_KEY = 'wms.tenfoot'

let on = false
const listeners = new Set<() => void>()

function stored(): boolean {
  try {
    const v = localStorage.getItem(KEY)
    if (v !== null) return v === '1'
    return localStorage.getItem(OLD_KEY) === '1'
  } catch { return false }
}

function apply(next: boolean) {
  on = next
  const html = document.documentElement
  // ten-foot: the larger type and stronger focus every page already has for across the room.
  html.classList.toggle('tv-mode', next)
  html.classList.toggle('ten-foot', next)
  if (next) html.classList.add('using-controller')
  else html.classList.remove('using-controller')
  listeners.forEach((l) => l())
}

export const getTvMode = (): boolean => on

export function setTvMode(next: boolean) {
  try { localStorage.setItem(KEY, next ? '1' : '0'); localStorage.removeItem(OLD_KEY) } catch { /* for this visit only */ }
  apply(next)
}

// What kind of client this is, as the apps say with ?client=tv (Android TV, Plasma Bigscreen) or
// ?client=desktop: kept on this device, since the address carries it only on the first page.
const CLIENT_KEY = 'wms.tvclient'
// Set when someone left TV mode to change something, so the page can offer the way back.
const RETURN_KEY = 'wms.tvmode.return'

function storedClient(): string | null {
  try { return localStorage.getItem(CLIENT_KEY) } catch { return null }
}

// tvClient: whether TV mode here is a TV device's, with nowhere else to go, or a computer's (levels.ts).
export function tvClient(): TvClient {
  const shell = typeof (window as unknown as { wittyClient?: { moonlight?: unknown } }).wittyClient?.moonlight === 'object'
  return tvClientOf({ client: storedClient(), ua: navigator.userAgent, shell })
}

// leaveTvMode: the normal layout, at this same page, with the way back offered there.
export function leaveTvMode() {
  try { sessionStorage.setItem(RETURN_KEY, '1') } catch { /* the Appearance switch still brings it back */ }
  setTvMode(false)
}
export function backToTvMode() {
  try { sessionStorage.removeItem(RETURN_KEY) } catch { /* nothing kept */ }
  setTvMode(true)
}
export function cameFromTvMode(): boolean {
  try { return sessionStorage.getItem(RETURN_KEY) === '1' } catch { return false }
}
export function forgetTvReturn() {
  try { sessionStorage.removeItem(RETURN_KEY) } catch { /* nothing kept */ }
}

export function installTvMode() {
  const client = new URLSearchParams(location.search).get('client')
  if (client === 'tv' || client === 'desktop') { try { localStorage.setItem(CLIENT_KEY, client) } catch { /* for this visit */ } }
  const asked = new URLSearchParams(location.search).get('tv')
  if (asked === '1' || asked === '0') setTvMode(asked === '1')
  else if (stored()) apply(true)
}

const subscribe = (l: () => void) => { listeners.add(l); return () => { listeners.delete(l) } }

export function useTvMode(): boolean {
  return useSyncExternalStore(subscribe, getTvMode)
}
