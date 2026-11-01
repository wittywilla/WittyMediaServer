import { useEffect, useState } from 'react'
import { api } from '../../api'
import { useApp } from '../../state'

// How the guide looks for this viewer. The server only sets what someone starts
// with, and a theme can suggest a look; everything here is each viewer's own
// choice, kept with their account so it follows them from device to device.

export type Layout = 'classic' | 'streaming'
export type Colours = 'cable' | 'satellite' | 'midnight' | 'theme' | 'custom'
export type PreviewMode = 'highlight' | 'last' | 'off'
export type PreviewSide = 'right' | 'left'
export type Look = {
  layout: Layout
  colours: Colours
  customBg: string // for colours = custom
  customAccent: string
  density: 'compact' | 'normal' | 'large'
  hours: 0 | 1.5 | 2 | 3 | 4 // 0 = what fits the screen
  nowLine: boolean
  progress: boolean // how far into each programme, on the cells on now
  subtitles: boolean // episode names in the cells
  logos: boolean
  art: boolean // the programme's picture behind the top of the page
  preview: PreviewMode
  previewSide: PreviewSide
  previewW: number // how wide the preview is asked to be, in pixels; the page holds it to what fits
  previewSound: boolean
}

export const DEFAULT_LOOK: Look = {
  layout: 'classic', colours: 'cable', customBg: '#10203a', customAccent: '#ffb300', density: 'normal', hours: 0, nowLine: true, progress: true, subtitles: true, logos: true, art: true,
  preview: 'highlight', previewSide: 'right', previewW: 320, previewSound: false,
}

// The sizes the preview snaps to, and their names. Any size between works too.
export const PREVIEW_SIZES = [{ w: 220, name: 'Small' }, { w: 320, name: 'Medium' }, { w: 460, name: 'Large' }, { w: 640, name: 'Extra large' }]
export const PREVIEW_MIN = 180
export const previewSizeName = (w: number) => PREVIEW_SIZES.find((s) => Math.abs(s.w - w) < 8)?.name || `${Math.round(w)} pixels`

const EVENT = 'wms-tv-look'
const OLD_KEY = 'wms.tv.look'
let userKey = OLD_KEY

// Reads what was saved, understanding what earlier versions saved.
function read(): Partial<Look> {
  let raw: Record<string, unknown> = {}
  try { raw = JSON.parse(localStorage.getItem(userKey) || localStorage.getItem(OLD_KEY) || '{}') } catch { /* nothing usable */ }
  return upgrade(raw)
}

function upgrade(raw: Record<string, unknown>): Partial<Look> {
  const out = { ...raw } as Partial<Look> & Record<string, unknown>
  if (typeof raw.previewPos === 'string' && !raw.previewSide) out.previewSide = raw.previewPos === 'tl' || raw.previewPos === 'bl' ? 'left' : 'right'
  if (typeof raw.previewSize === 'string' && !raw.previewW) out.previewW = ({ s: 220, m: 320, l: 460 } as Record<string, number>)[raw.previewSize] || 320
  delete out.previewPos; delete out.previewSize; delete out.previewX; delete out.previewY
  return out
}

let saveTimer: ReturnType<typeof setTimeout> | undefined
function saveToAccount(saved: Partial<Look>) {
  clearTimeout(saveTimer)
  saveTimer = setTimeout(() => {
    // The server keeps an account's interface choices whole, and others have some there too.
    api.get<Record<string, unknown>>('/me/ui').then((u) => api.put('/me/ui', { ...u, tvLook: saved })).catch(() => { /* still saved on this device */ })
  }, 800)
}

function store(next: Partial<Look>) {
  try { localStorage.setItem(userKey, JSON.stringify(next)) } catch { /* ignore */ }
  window.dispatchEvent(new Event(EVENT))
}

export function setLook(patch: Partial<Look>) {
  const next = { ...read(), ...patch }
  store(next)
  saveToAccount(next)
}

export function resetLook() {
  store({})
  saveToAccount({})
}

// What the current theme suggests, set in its CSS: --tv-layout and --tv-colours.
export function themeSuggestion(): { layout?: Layout; colours?: Colours } {
  const css = getComputedStyle(document.documentElement)
  const layout = css.getPropertyValue('--tv-layout').trim(), colours = css.getPropertyValue('--tv-colours').trim()
  const out: { layout?: Layout; colours?: Colours } = {}
  if (layout === 'classic' || layout === 'streaming') out.layout = layout
  if (colours === 'cable' || colours === 'satellite' || colours === 'midnight' || colours === 'theme') out.colours = colours
  return out
}

// serverLayout and serverColours are what the owner chose as the starting look.
export function useLook(serverLayout?: Layout, serverColours?: Colours): Look {
  const { user } = useApp()
  userKey = user ? `${OLD_KEY}.${user.id}` : OLD_KEY
  const [saved, setSaved] = useState(read)
  useEffect(() => {
    const on = () => setSaved(read())
    on()
    window.addEventListener(EVENT, on)
    window.addEventListener('storage', on)
    return () => { window.removeEventListener(EVENT, on); window.removeEventListener('storage', on) }
  }, [user?.id])
  // What this account chose on another device.
  useEffect(() => {
    if (!user) return
    let on = true
    api.get<{ tvLook?: Record<string, unknown> }>('/me/ui').then((u) => { if (on && u.tvLook) store(upgrade(u.tvLook)) }).catch(() => { /* this device's copy will do */ })
    return () => { on = false }
  }, [user?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  const theme = themeSuggestion()
  return { ...DEFAULT_LOOK, ...(serverLayout ? { layout: serverLayout } : {}), ...(serverColours ? { colours: serverColours } : {}), ...theme, ...saved }
}

// ------------------------------------------------------------- custom colours

const rgb = (hex: string) => [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16))
const toHex = (c: number[]) => '#' + c.map((v) => Math.round(Math.max(0, Math.min(255, v))).toString(16).padStart(2, '0')).join('')
const mix = (a: string, b: string, t: number) => { const x = rgb(a), y = rgb(b); return toHex(x.map((v, i) => v + (y[i] - v) * t)) }
const lum = (hex: string) => { const f = (v: number) => { const s = v / 255; return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4 }; const [r, g, b] = rgb(hex); return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b) }
export const contrast = (a: string, b: string) => { const x = lum(a), y = lum(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05) }
export const readableOn = (bg: string) => (contrast(bg, '#ffffff') >= contrast(bg, '#000000') ? '#ffffff' : '#000000')

// An accent pushed far enough from the background to be seen, whatever was picked.
function legibleAccent(accent: string, bg: string): string {
  const away = readableOn(bg)
  let a = accent
  for (let i = 0; i < 10 && contrast(a, bg) < 4.5; i++) a = mix(a, away, 0.15)
  return a
}

const isHex = (v: unknown): v is string => typeof v === 'string' && /^#[0-9a-fA-F]{6}$/.test(v)

// The guide's tokens worked out from two colours: every pairing has AA contrast,
// because the text colour comes from the surface it is on.
export function customVars(look: Look): React.CSSProperties {
  if (look.colours !== 'custom' || !isHex(look.customBg) || !isHex(look.customAccent)) return {}
  const bg = look.customBg
  const text = readableOn(bg)
  const lift = (t: number) => mix(bg, text, t)
  const accent = legibleAccent(look.customAccent, bg)
  return {
    '--ctv-bg': bg, '--ctv-panel': lift(0.06), '--ctv-panel-2': lift(0.12), '--ctv-head': lift(0.14), '--ctv-cell': lift(0.14), '--ctv-cell-2': lift(0.1),
    '--ctv-live': mix(lift(0.14), accent, 0.22), '--ctv-live-2': mix(lift(0.1), accent, 0.22), '--ctv-ch': lift(0.06), '--ctv-dim': lift(0.06), '--ctv-line': lift(0.34),
    '--ctv-text': text, '--ctv-soft': mix(text, bg, 0.2), '--ctv-hi': accent, '--ctv-hi-text': readableOn(accent), '--ctv-accent': accent,
    '--ctv-key-a': accent, '--ctv-key-b': accent, '--ctv-key-c': accent, '--ctv-key-text': readableOn(accent), '--ctv-err': accent,
  } as React.CSSProperties
}

// What a logo sits on, so it comes in the version that can be read: a highlighted row
// in the modern look is the highlight's colour, anything else the guide's background.
export function surfaceOf(look: Look, selected = false): 'dark' | 'light' {
  const token = (name: string) => getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  let colour = ''
  if (selected && look.layout === 'streaming') {
    if (look.colours === 'theme') colour = token('--accent')
    else if (look.colours === 'custom') colour = legibleAccent(look.customAccent, look.customBg)
    else return 'light'
  } else if (look.colours === 'theme') colour = token('--bg')
  else if (look.colours === 'custom') colour = look.customBg
  return isHex(colour) && readableOn(colour) === '#000000' ? 'light' : 'dark'
}

// A channel's own colour, for what belongs to that channel.
export function channelVars(accent: string | undefined, bg = '#000000'): React.CSSProperties {
  if (!isHex(accent)) return {}
  const a = legibleAccent(accent, bg)
  return { '--ctv-accent': a, '--ctv-channel': a, '--ctv-channel-text': readableOn(a) } as React.CSSProperties
}

// ------------------------------------------------------------ the last channel

const LAST = 'wms.tv.last'
export const lastChannel = () => { try { return localStorage.getItem(LAST) || '' } catch { return '' } }
export const previousChannel = () => { try { return localStorage.getItem(LAST + '.before') || '' } catch { return '' } }
export const rememberChannel = (key: string) => {
  try {
    const was = localStorage.getItem(LAST)
    if (was && was !== key) localStorage.setItem(LAST + '.before', was)
    localStorage.setItem(LAST, key)
  } catch { /* ignore */ }
}
