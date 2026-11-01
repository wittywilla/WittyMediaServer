import { useEffect, useState } from 'react'

// Where the main navigation sits on this device: the sidebar, a bar along
// the top, or both. Phones always also get the bottom tab bar. A device with no
// choice of its own follows what the owner set for everyone.
export type NavStyle = 'side' | 'top' | 'both'

const KEY = 'wms.nav'
const COLLAPSED = 'wms.navCollapsed'
const EVENT = 'wms-nav-change'

let ownerStyle: NavStyle = 'side'

export function setOwnerNavStyle(v: NavStyle | undefined) {
  ownerStyle = v === 'top' || v === 'both' ? v : 'side'
  window.dispatchEvent(new Event(EVENT))
}

export function getNavStyle(): NavStyle {
  try {
    const v = localStorage.getItem(KEY)
    return v === 'top' || v === 'both' || v === 'side' ? v : ownerStyle
  } catch { return ownerStyle }
}

export function setNavStyle(v: NavStyle) {
  try { localStorage.setItem(KEY, v) } catch { /* ignore */ }
  window.dispatchEvent(new Event(EVENT))
}

export function useNavStyle(): NavStyle {
  const [v, setV] = useState(getNavStyle)
  useEffect(() => {
    const on = () => setV(getNavStyle())
    window.addEventListener(EVENT, on)
    window.addEventListener('storage', on)
    return () => { window.removeEventListener(EVENT, on); window.removeEventListener('storage', on) }
  }, [])
  return v
}

// Whether the side menu is folded away on this device. Wide screens remember it.
export function getNavCollapsed(): boolean {
  try { return localStorage.getItem(COLLAPSED) === '1' } catch { return false }
}

export function setNavCollapsed(v: boolean) {
  try { localStorage.setItem(COLLAPSED, v ? '1' : '0') } catch { /* ignore */ }
}
