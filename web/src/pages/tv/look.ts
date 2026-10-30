import { useEffect, useState } from 'react'

// How the guide looks for this viewer on this device. The server only sets
// what someone starts with: everything here is each viewer's own choice.

export type Layout = 'classic' | 'streaming'
export type Colours = 'cable' | 'satellite' | 'theme'
export type PreviewMode = 'highlight' | 'last' | 'off'
export type PreviewPos = 'tr' | 'tl' | 'br' | 'bl' | 'custom'
export type Look = {
  layout: Layout
  colours: Colours
  density: 'compact' | 'normal' | 'large'
  hours: 0 | 1.5 | 2 | 3 | 4 // 0 = what fits the screen
  nowLine: boolean
  progress: boolean // how far into each programme, on the cells on now
  subtitles: boolean // episode names in the cells
  logos: boolean
  preview: PreviewMode
  previewPos: PreviewPos
  previewSize: 's' | 'm' | 'l'
  previewX: number // custom position: % of the guide, from the left
  previewY: number // and from the top
  previewSound: boolean
}

export const DEFAULT_LOOK: Look = {
  layout: 'classic', colours: 'cable', density: 'normal', hours: 0, nowLine: true, progress: true, subtitles: true, logos: true,
  preview: 'highlight', previewPos: 'tr', previewSize: 'm', previewX: 70, previewY: 2, previewSound: false,
}

const KEY = 'wms.tv.look'
const EVENT = 'wms-tv-look'

function read(): Partial<Look> {
  try { return JSON.parse(localStorage.getItem(KEY) || '{}') } catch { return {} }
}

export function setLook(patch: Partial<Look>) {
  try { localStorage.setItem(KEY, JSON.stringify({ ...read(), ...patch })) } catch { /* ignore */ }
  window.dispatchEvent(new Event(EVENT))
}

export function resetLook() {
  try { localStorage.removeItem(KEY) } catch { /* ignore */ }
  window.dispatchEvent(new Event(EVENT))
}

// serverLayout and serverColours are what the owner chose as the starting look.
export function useLook(serverLayout?: Layout, serverColours?: Colours): Look {
  const [saved, setSaved] = useState(read)
  useEffect(() => {
    const on = () => setSaved(read())
    window.addEventListener(EVENT, on)
    window.addEventListener('storage', on)
    return () => { window.removeEventListener(EVENT, on); window.removeEventListener('storage', on) }
  }, [])
  return { ...DEFAULT_LOOK, ...(serverLayout ? { layout: serverLayout } : {}), ...(serverColours ? { colours: serverColours } : {}), ...saved }
}

export const lastChannel = () => { try { return localStorage.getItem('wms.tv.last') || '' } catch { return '' } }
export const rememberChannel = (key: string) => { try { localStorage.setItem('wms.tv.last', key) } catch { /* ignore */ } }
