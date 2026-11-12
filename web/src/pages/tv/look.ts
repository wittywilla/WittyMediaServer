// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useState } from 'react'
import { api } from '../../api'
import { useApp } from '../../state'
import { currentLook } from '../../theme'
import { chooseTheme, contrast, FALLBACK_APP, FONT_STACKS, isHex, readableOn, SHAPES, themeById, themeTokens, upgradeSaved, type AppColours, type Colours, type Font, type OwnerLook, type Saved, type Shape, type Slot, type Suggestion, type ThemeId, type Tokens } from './tvThemes'

// How WittyTV looks for this viewer, on every TV screen. It's their own, kept with their account
// so it follows them from device to device. What they haven't chosen comes from the app theme's
// suggestion and then the owner's starting look: chooseTheme in tvThemes.ts is what decides.

export { contrast, readableOn } from './tvThemes'
export type PreviewMode = 'highlight' | 'last' | 'off'
export type PreviewSide = 'right' | 'left'
// Where the preview's controls are: in a bar under the picture, or over it only while it has
// focus or the pointer, which leaves a plain picture with its corner to resize it by.
export type PreviewBar = 'bar' | 'over'
export type Look = {
  theme: ThemeId
  shape: Shape
  font: Font
  // Where the theme came from: what the viewer chose, what the app theme suggests, or what
  // the server's owner set for new viewers.
  source: 'you' | 'app' | 'owner'
  colours: Colours // this theme's overrides
  tokens: Tokens
  bg: string // the background as a plain colour, for working things out from it
  custom: { shape: Shape; font: Font }
  density: 'compact' | 'normal' | 'large'
  hours: 0 | 1.5 | 2 | 3 | 4 // 0 = what fits the screen
  nowLine: boolean
  progress: boolean // how far into each programme, on the cells on now
  subtitles: boolean // episode names in the cells
  logos: boolean
  art: boolean // the programme's picture behind the top of the page
  preview: PreviewMode
  previewSide: PreviewSide
  // How wide the preview was asked to be. Left out, it's worked out from the window and the
  // theme. The page holds either to what fits.
  previewW?: number
  previewSound: boolean
  previewBar: PreviewBar
  surfaces: { bg: 'dark' | 'light'; hi: 'dark' | 'light' }
}

const SETTINGS: Required<Pick<Saved, 'density' | 'hours' | 'nowLine' | 'progress' | 'subtitles' | 'logos' | 'art' | 'preview' | 'previewSound' | 'previewBar'>> = {
  density: 'normal', hours: 0, nowLine: true, progress: true, subtitles: true, logos: true, art: true, preview: 'highlight', previewSound: false, previewBar: 'bar',
}

// The sizes the preview snaps to, and their names. Any size between works too.
export const PREVIEW_SIZES = [{ w: 220, name: 'Small' }, { w: 320, name: 'Medium' }, { w: 460, name: 'Large' }, { w: 640, name: 'Extra large' }, { w: 880, name: 'Huge' }]
export const previewSizeName = (w: number) => PREVIEW_SIZES.find((s) => Math.abs(s.w - w) < 8)?.name || `${Math.round(w)} pixels`

const EVENT = 'wms-tv-look'
const OLD_KEY = 'wms.tv.look'
const OWNER_KEY = 'wms.tv.owner'
let userKey = OLD_KEY

// ------------------------------------------------------------ the owner's starting look

function savedOwner(): OwnerLook {
  try { return JSON.parse(localStorage.getItem(OWNER_KEY) || '{}') as OwnerLook } catch { return {} }
}
let owner: OwnerLook = savedOwner()
let ownerAsked = false

export function setOwnerLook(o: OwnerLook) {
  if (owner.theme === o.theme) return
  owner = o
  try { localStorage.setItem(OWNER_KEY, JSON.stringify(o)) } catch { /* it's asked for again next time */ }
  window.dispatchEvent(new Event(EVENT))
}

// Every screen follows it, not only the guide. One row of the channel list is the cheapest way to hear it.
function askOwner() {
  if (ownerAsked) return
  ownerAsked = true
  api.get<OwnerLook>('/guide?list=1&limit=1').then((g) => setOwnerLook({ theme: g.theme })).catch(() => { ownerAsked = false })
}
export const forgetOwnerLook = () => { ownerAsked = false; askOwner() }

// ------------------------------------------------------------ what's saved

function read(): Saved {
  let raw: Record<string, unknown> = {}
  try { raw = JSON.parse(localStorage.getItem(userKey) || localStorage.getItem(OLD_KEY) || '{}') } catch { /* nothing usable */ }
  return upgradeSaved(raw)
}

let saveTimer: ReturnType<typeof setTimeout> | undefined
// While a change is on its way to the account, this device's copy is the newer one.
let saving = 0
function saveToAccount(saved: Saved) {
  clearTimeout(saveTimer)
  if (!saving) saving = 1
  saveTimer = setTimeout(() => {
    // The server keeps an account's interface choices whole, and others have some there too.
    api.get<Record<string, unknown>>('/me/ui').then((u) => api.put('/me/ui', { ...u, tvLook: saved })).catch(() => { /* still saved on this device */ }).finally(() => { saving = 0 })
  }, 800)
}

function store(next: Saved) {
  try { localStorage.setItem(userKey, JSON.stringify(next)) } catch { /* ignore */ }
  window.dispatchEvent(new Event(EVENT))
}

export function setLook(patch: Partial<Saved>) {
  const next = { ...read(), ...patch }
  store(next)
  saveToAccount(next)
}

export function resetLook() {
  store({})
  saveToAccount({})
}

// Back to following the app theme's suggestion and the owner. The viewer's other choices stay.
export function forgetTheme() {
  const next = read()
  delete next.theme
  store(next)
  saveToAccount(next)
}

// A size back to being worked out from the window.
export function forgetPreviewSize() {
  const next = read()
  delete next.previewW
  store(next)
  saveToAccount(next)
}

export function pickTheme(id: ThemeId, from?: Look) {
  const saved = read()
  const patch: Partial<Saved> = { theme: id }
  // Custom starts from what's on the screen, so there's nothing to set up to begin with.
  if (id === 'custom' && !saved.custom && from) {
    patch.custom = { shape: from.shape, font: from.font }
    if (Object.keys(from.colours).length) patch.themeColours = { ...saved.themeColours, custom: from.colours }
  }
  setLook(patch)
}

// One colour of a theme, or undefined to go back to following.
export function setThemeColour(id: ThemeId, slot: Slot, value: string | undefined) {
  const saved = read()
  const mine = { ...saved.themeColours?.[id] }
  if (value) mine[slot] = value
  else delete mine[slot]
  setLook({ themeColours: { ...saved.themeColours, [id]: mine } })
}

export function clearThemeColours(id: ThemeId) {
  const saved = read()
  const rest = { ...saved.themeColours }
  delete rest[id]
  setLook({ themeColours: rest })
}

// ------------------------------------------------------------ reading the page

// What the current app theme suggests, set in its CSS: --tv-theme.
export function themeSuggestion(): Suggestion {
  const css = getComputedStyle(document.documentElement)
  const one = (name: string) => css.getPropertyValue(name).trim() || undefined
  // The look (experience) the app is wearing suggests one too (docs/plans/DESIGN-2.md 7.1, `tvTheme`); it comes before a theme's own.
  return { theme: currentLook()?.tvTheme || one('--tv-theme') }
}

const toHex = (raw: string, fallback: string): string => {
  const v = raw.trim()
  if (isHex(v)) return v
  const short = /^#([0-9a-f])([0-9a-f])([0-9a-f])$/i.exec(v)
  if (short) return '#' + short[1] + short[1] + short[2] + short[2] + short[3] + short[3]
  const m = /^rgba?\(\s*(\d+)[ ,]+(\d+)[ ,]+(\d+)/.exec(v)
  if (m) return '#' + m.slice(1, 4).map((x) => Number(x).toString(16).padStart(2, '0')).join('')
  return fallback
}

// The app theme's colours as plain colours, for what has to be worked out from them.
export function appColours(): AppColours {
  const css = getComputedStyle(document.documentElement)
  const get = (name: string, fallback: string) => toHex(css.getPropertyValue(name), fallback)
  const f = FALLBACK_APP
  return { bg: get('--bg', f.bg), bg2: get('--bg-2', f.bg2), bg3: get('--bg-3', f.bg3), bg4: get('--bg-4', f.bg4), fg: get('--fg', f.fg), fg2: get('--fg-2', f.fg2), accent: get('--accent', f.accent), accentText: get('--accent-text', f.accentText), accentFg: get('--accent-fg', f.accentFg), border: get('--border-solid', f.border) }
}

function resolve(saved: Saved, suggestion: Suggestion, ownerLook: OwnerLook, app: AppColours): Look {
  const chosen = chooseTheme(saved, suggestion, ownerLook)
  const theme = chosen.theme
  const custom = saved.custom || chosen.custom || { shape: 'modern' as Shape, font: '' as Font }
  const shape = theme === 'custom' ? custom.shape : themeById(theme)!.shape
  const colours = saved.themeColours?.[theme] || chosen.colours?.[theme] || {}
  const tokens = themeTokens(theme, colours, app)
  const plain = (name: string, followed: string) => { const v = tokens[name]; return isHex(v) ? v : followed }
  const bg = plain('--ctv-bg', app.bg)
  const hi = plain('--ctv-hi', theme === 'modern' || theme === 'bigpicture' ? app.fg : app.accent)
  const dark = (c: string) => (readableOn(c) === '#ffffff' ? 'dark' : 'light') as 'dark' | 'light'
  return {
    ...SETTINGS, ...saved, theme, shape, font: theme === 'custom' ? custom.font : '', source: chosen.source, colours, tokens, bg, custom,
    previewSide: saved.previewSide || SHAPES[shape].side, previewBar: saved.previewBar === 'over' ? 'over' : 'bar', surfaces: { bg: dark(bg), hi: dark(hi) },
  }
}

export function useLook(): Look {
  const { user } = useApp()
  userKey = user ? `${OLD_KEY}.${user.id}` : OLD_KEY
  const [saved, setSaved] = useState(read)
  const [ownerLook, setOwner] = useState(owner)
  useEffect(() => {
    const on = () => { setSaved(read()); setOwner(owner) }
    on()
    window.addEventListener(EVENT, on)
    window.addEventListener('storage', on)
    return () => { window.removeEventListener(EVENT, on); window.removeEventListener('storage', on) }
  }, [user?.id])
  // What this account chose on another device.
  useEffect(() => {
    if (!user) return
    let on = true
    askOwner()
    api.get<{ tvLook?: Record<string, unknown> }>('/me/ui').then((u) => { if (on && u.tvLook && !saving) store(upgradeSaved(u.tvLook)) }).catch(() => { /* this device's copy will do */ })
    return () => { on = false }
  }, [user?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  // Read on each drawing, and cheap: the look only changes identity when one of these really has.
  const suggestion = themeSuggestion(), app = appColours()
  const key = JSON.stringify([saved, suggestion, ownerLook, app])
  return useMemo(() => resolve(saved, suggestion, ownerLook, app), [key]) // eslint-disable-line react-hooks/exhaustive-deps
}

// What the guide's root element and the player's need, so every TV screen is dressed the same.
export function themeRoot(look: Look): { 'data-shape': Shape; 'data-tvtheme': ThemeId; 'data-previewbar': PreviewBar; style: React.CSSProperties } {
  const style = { ...look.tokens } as Record<string, string>
  if (look.font) style['--ctv-font'] = FONT_STACKS[look.font]
  return { 'data-shape': look.shape, 'data-tvtheme': look.theme, 'data-previewbar': look.previewBar, style: style as React.CSSProperties }
}

// An accent pushed far enough from the background to be seen, whatever was picked.
function legibleAccent(accent: string, bg: string): string {
  const away = readableOn(bg)
  const parts = (h: string) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16))
  let a = accent
  for (let i = 0; i < 10 && contrast(a, bg) < 4.5; i++) {
    const x = parts(a), y = parts(away)
    a = '#' + x.map((v, n) => Math.round(v + (y[n] - v) * 0.15).toString(16).padStart(2, '0')).join('')
  }
  return a
}

// What a logo sits on, so it comes in the version that can be read: a highlighted row, in the
// themes that colour one, is the highlight's colour; anything else the guide's background.
export function surfaceOf(look: Look, selected = false): 'dark' | 'light' {
  return selected && SHAPES[look.shape].rowHighlight ? look.surfaces.hi : look.surfaces.bg
}

// A channel's own colour, for what belongs to that channel.
export function channelVars(accent: string | undefined, bg = '#000000'): React.CSSProperties {
  if (!isHex(accent)) return {}
  const a = legibleAccent(accent, isHex(bg) ? bg : '#000000')
  return { '--ctv-accent': a, '--ctv-channel': a, '--ctv-channel-text': readableOn(a) } as React.CSSProperties
}

// ------------------------------------------------------------ the last channel

const LAST = 'wms.tv.last'
export const lastChannel = () => { try { return localStorage.getItem(LAST) || '' } catch { return '' } }
export const previousChannel = () => { try { return localStorage.getItem(LAST + '.before') || '' } catch { return '' } }
// The channel just watched, for the guide to come back to. Only for this visit: a guide
// opened afresh starts at the top as before.
let watched = ''
export const justWatched = () => watched
export const forgetJustWatched = () => { watched = '' }
export const rememberChannel = (key: string) => {
  watched = key
  try {
    const was = localStorage.getItem(LAST)
    if (was && was !== key) localStorage.setItem(LAST + '.before', was)
    localStorage.setItem(LAST, key)
  } catch { /* ignore */ }
}
