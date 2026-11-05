import { serverBase } from './api'

// Themes are ZIPs on the server (see docs/THEMES.md). The server default can
// be overridden per device; the choice lives in localStorage.

export type ThemeMeta = {
  id: string; name: string; description?: string; author?: string; version?: string; scheme: 'dark' | 'light'; builtin?: boolean
  preview: { bg?: string; surface?: string; text?: string; accent?: string }; size: number; modTime: number
  colors?: Record<string, string>; radius?: number
}

// Themes that used to ship, and today's look for each. The server knows these too.
const RETIRED: Record<string, string> = {
  dark: 'default', 'dark-purple': 'default', 'dark-pink': 'default', 'dark-blue': 'midnight', 'dark-red': 'cinema', 'dark-orange': 'cinema', 'dark-gold': 'cinema', 'dark-green': 'evergreen', 'dark-teal': 'evergreen',
  'light-blue': 'light', 'light-purple': 'light', 'light-pink': 'light', 'light-teal': 'light', 'light-red': 'paper', 'light-orange': 'paper', 'light-gold': 'paper', 'light-green': 'paper',
}
export const currentThemeId = (id: string) => RETIRED[id] || id

const KEY = 'wms.theme'
const CUSTOM_CSS = 'wms.customTheme'

// A look made in the theme maker just for this device: it's only colours, kept here
// as the stylesheet it became, so it applies before anything else has loaded.
export const CUSTOM_THEME = 'custom'
export const customThemeCss = () => { try { return localStorage.getItem(CUSTOM_CSS) || '' } catch { return '' } }
export const setCustomThemeCss = (css: string) => { try { localStorage.setItem(CUSTOM_CSS, css) } catch { /* it lasts until the page reloads */ } }
function applyCustom(css: string) {
  document.querySelectorAll('link[data-wms-theme]').forEach((l) => l.remove())
  let st = document.getElementById('wms-custom-theme') as HTMLStyleElement | null
  if (!st) { st = document.createElement('style'); st.id = 'wms-custom-theme'; document.head.appendChild(st) }
  st.textContent = css
  document.documentElement.dataset.theme = CUSTOM_THEME
}

export const deviceTheme = (): string => { try { return currentThemeId(localStorage.getItem(KEY) || '') } catch { return '' } }
export const setDeviceTheme = (id: string) => { try { id ? localStorage.setItem(KEY, id) : localStorage.removeItem(KEY) } catch { /* ignore */ } }

let wanted = ''

// 'auto' follows the device's light or dark setting, using the two built-in looks.
export const AUTO_THEME = 'auto'
const lightDevice = typeof window !== 'undefined' && window.matchMedia ? window.matchMedia('(prefers-color-scheme: light)') : null
const resolveTheme = (id: string) => (id === AUTO_THEME ? (lightDevice?.matches ? 'light' : 'default') : currentThemeId(id))

// applyTheme loads a theme's CSS after the built-in stylesheet. The new sheet
// is added next to the current one and replaces it only once it has loaded,
// so switching never flashes an unstyled page, and a sheet that failed to
// load is fetched again the next time the theme is picked (instead of the
// click silently doing nothing because the link already pointed at it).
export function applyTheme(wantedId: string, version = '') {
  if (lightDevice) lightDevice.onchange = wantedId === AUTO_THEME ? () => applyTheme(AUTO_THEME, version) : null
  const id = resolveTheme(wantedId)
  if (id === CUSTOM_THEME && customThemeCss()) { wanted = ''; applyCustom(customThemeCss()); return }
  document.getElementById('wms-custom-theme')?.remove()
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

let brand = 'WittyMediaServer'
let page = ''

export function applyBrand(name: string) {
  brand = name || 'WittyMediaServer'
  applyPageTitle(page)
}

// applyPageTitle puts the page's own name before the server's in the window's title, so browser
// tabs, history and a screen reader's list of windows can tell the pages apart.
export function applyPageTitle(name: string) {
  page = name
  document.title = name && name !== brand ? `${name} · ${brand}` : brand
}
