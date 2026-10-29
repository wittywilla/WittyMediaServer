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

// applyTheme loads a theme's CSS after the built-in stylesheet.
export function applyTheme(id: string, version = '') {
  let link = document.getElementById('wms-theme') as HTMLLinkElement | null
  if (!link) {
    link = document.createElement('link')
    link.id = 'wms-theme'
    link.rel = 'stylesheet'
    document.head.appendChild(link)
  }
  const href = `${serverBase()}/api/themes/${encodeURIComponent(id || 'default')}/theme.css${version ? '?v=' + version : ''}`
  if (link.getAttribute('href') !== href) link.setAttribute('href', href)
  document.documentElement.dataset.theme = id || 'default'
}

export function applyBrand(name: string) {
  document.title = name || 'WittyMediaServer'
}
