import { useSyncExternalStore } from 'react'

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

export function installTvMode() {
  const asked = new URLSearchParams(location.search).get('tv')
  if (asked === '1' || asked === '0') setTvMode(asked === '1')
  else if (stored()) apply(true)
}

const subscribe = (l: () => void) => { listeners.add(l); return () => { listeners.delete(l) } }

export function useTvMode(): boolean {
  return useSyncExternalStore(subscribe, getTvMode)
}
