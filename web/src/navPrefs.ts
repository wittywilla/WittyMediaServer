import { useEffect, useState } from 'react'

// Where the main navigation sits on this device: the sidebar, a bar along
// the top, or both. Phones always also get the bottom tab bar.
export type NavStyle = 'side' | 'top' | 'both'

const KEY = 'wms.nav'
const EVENT = 'wms-nav-change'

export function getNavStyle(): NavStyle {
  try {
    const v = localStorage.getItem(KEY)
    return v === 'top' || v === 'both' ? v : 'side'
  } catch { return 'side' }
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
