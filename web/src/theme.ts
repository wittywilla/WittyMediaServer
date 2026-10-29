import { serverBase } from './api'

// Themes are ZIPs on the server (see docs/THEMES.md). The server default can
// be overridden per device; the choice lives in localStorage.

export type ThemeMeta = {
  id: string; name: string; description?: string; author?: string; version?: string; scheme: 'dark' | 'light'; builtin?: boolean
  preview: { bg?: string; surface?: string; text?: string; accent?: string }; size: number; modTime: number
}

const KEY = 'wms.theme'

export const deviceTheme = (): string => { try { return localStorage.getItem(KEY) || '' } catch { return '' } }
export const setDeviceTheme = (id: string) => { try { id ? localStorage.setItem(KEY, id) : localStorage.removeItem(KEY) } catch { /* ignore */ } }

let wanted = ''

// applyTheme loads a theme's CSS after the built-in stylesheet. The new sheet
// is added next to the current one and replaces it only once it has loaded,
// so switching never flashes an unstyled page, and a sheet that failed to
// load is fetched again the next time the theme is picked (instead of the
// click silently doing nothing because the link already pointed at it).
export function applyTheme(id: string, version = '') {
  const href = `${serverBase()}/api/themes/${encodeURIComponent(id || 'default')}/theme.css${version ? '?v=' + version : ''}`
  wanted = href
  document.documentElement.dataset.theme = id || 'default'
  const links = Array.from(document.querySelectorAll<HTMLLinkElement>('link[data-wms-theme]'))
  const cur = links.find((l) => l.getAttribute('href') === href)
  if (cur && (cur.sheet || cur.dataset.state === 'loading')) {
    if (cur.sheet) links.forEach((l) => { if (l !== cur) l.remove() })
    return
  }
  cur?.remove()
  const link = document.createElement('link')
  link.rel = 'stylesheet'
  link.dataset.wmsTheme = '1'
  link.dataset.state = 'loading'
  link.onload = () => {
    link.dataset.state = 'ok'
    if (wanted !== href) { link.remove(); return } // a newer pick won
    link.id = 'wms-theme'
    document.querySelectorAll<HTMLLinkElement>('link[data-wms-theme]').forEach((l) => { if (l !== link) l.remove() })
  }
  link.onerror = () => link.remove()
  link.href = href
  document.head.appendChild(link)
}

export function applyBrand(name: string) {
  document.title = name || 'WittyMediaServer'
}
