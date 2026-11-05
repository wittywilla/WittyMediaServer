import { createContext, useContext, useEffect, useState } from 'react'
import { api, mediaUrl, posterUrl, type GameConsole, type Item } from '../../api'

// What the game pages share: the one request for the home screen, what a game needs in
// order to play, and where links go, which depends on whether couch mode is on.

export type GameSaveRef = { slot: number; updatedAt: number; hasThumb: boolean }
export type GameContinue = { item: Item; plays: number; lastPlayed: number; save?: GameSaveRef }
export type GameShelf = { kind: 'genre' | 'series'; name: string; count: number; items: Item[] }
export type GameHome = {
  hero: Item[]; continue: GameContinue[]; recent: Item[]; favourites: Item[]; favouriteIds: number[]; mostPlayed: Item[]; shelves: GameShelf[]
}
export type GameCheck = {
  ready: boolean; reason?: 'core' | 'bios' | 'gpu' | 'unavailable' | 'unknown'; message?: string; file?: string; console?: string; consoleName?: string
  core?: { id: string; name: string; license: string }; needsGpu?: boolean; gpuMissing?: string
  // Plays, drawn by the server's processor as it has no graphics card games can use; notice is the speed caveat.
  software?: boolean; notice?: string
  bios: { file: string; for: string; required: boolean; found: boolean }[]; biosFolder?: string
}

// A page back from a game shows what it showed at once, and then catches up.
const homes = new Map<number, GameHome>()
export const cachedHome = (lib: number) => homes.get(lib) || null
export const getHome = (lib: number) => api.get<GameHome>(`/games/home?library=${lib}`).then((h) => { homes.set(lib, h); return h })
export const forgetHome = (lib: number) => { homes.delete(lib) }

// Covers come as smaller copies, as big as they'll be seen on this screen.
const dense = (window.devicePixelRatio || 1) * window.innerWidth > 2000
export const cardCover = (it: Item) => `${posterUrl(it)}&w=${dense ? 480 : 320}`
export const heroCover = (it: Item) => `${posterUrl(it)}&w=${dense ? 800 : 480}`

export const saveThumb = (id: number, slot: number, v: number) => {
  const u = mediaUrl(`/api/games/${id}/saves/${slot}/thumb`)
  return u + (u.includes('?') ? '&' : '?') + `v=${v}`
}

// A save's picture: an old console's few pixels are kept sharp when enlarged, and a
// modern one's many are left to the browser to scale smoothly.
export const crispIfSmall = (e: { currentTarget: HTMLImageElement }) => { if (e.currentTarget.naturalWidth < 512) e.currentTarget.classList.add('crisp') }

export const fmtPlayed = (sec: number) => {
  const h = Math.floor(sec / 3600), m = Math.round((sec % 3600) / 60)
  return h > 0 ? `${h} h ${m} min` : `${Math.max(1, m)} min`
}

export const fmtAgo = (unix: number) => {
  const s = Date.now() / 1000 - unix
  if (s < 90) return 'just now'
  if (s < 3600) return `${Math.round(s / 60)} minutes ago`
  if (s < 86400) { const h = Math.round(s / 3600); return h === 1 ? '1 hour ago' : `${h} hours ago` }
  if (s < 86400 * 2) return 'yesterday'
  if (s < 86400 * 30) return `${Math.round(s / 86400)} days ago`
  return new Date(unix * 1000).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

// ---------------------------------------------------------------- where links go

export type GameLinks = {
  couch: boolean
  item: (id: number) => string
  library: (lib: number, query?: string) => string
  play: (id: number, query?: string) => string
}
const plain: GameLinks = {
  couch: false,
  item: (id) => `/item/${id}`,
  library: (lib, query) => `/library/${lib}${query ? '?' + query : ''}`,
  play: (id, query) => `/game/${id}${query ? '?' + query : ''}`,
}
export const GameLinksContext = createContext<GameLinks>(plain)
export const useGameLinks = () => useContext(GameLinksContext)

// Whether couch mode is on survives going into a game and back, for this tab only.
const COUCH_KEY = 'wms.couchMode'
export const couchOn = () => { try { return sessionStorage.getItem(COUCH_KEY) === '1' } catch { return false } }
export const setCouchOn = (on: boolean) => { try { if (on) sessionStorage.setItem(COUCH_KEY, '1'); else sessionStorage.removeItem(COUCH_KEY) } catch { /* it just won't come back after a game */ } }

// ---------------------------------------------------------------- the colour of a cover

// A cover's main colour, from a very small copy of it. It costs one small picture and
// no blur, so a cheap TV can afford a page tinted with it.
const tints = new Map<string, [number, number, number] | null>()

function averageColour(img: HTMLImageElement): [number, number, number] | null {
  const size = 16
  const c = document.createElement('canvas')
  c.width = c.height = size
  const g = c.getContext('2d', { willReadFrequently: true })
  if (!g) return null
  g.drawImage(img, 0, 0, size, size)
  let data: Uint8ClampedArray
  try { data = g.getImageData(0, 0, size, size).data } catch { return null } // a picture from another address
  let r = 0, gr = 0, b = 0, w = 0
  for (let i = 0; i < data.length; i += 4) {
    const R = data[i], G = data[i + 1], B = data[i + 2]
    const max = Math.max(R, G, B), min = Math.min(R, G, B)
    // The colourful parts count for more: a white box with a red logo reads as red.
    const weight = 1 + ((max - min) / 255) * 6
    r += R * weight; gr += G * weight; b += B * weight; w += weight
  }
  return w ? [r / w, gr / w, b / w] : null
}

function hsl([r, g, b]: [number, number, number]): [number, number, number] {
  r /= 255; g /= 255; b /= 255
  const max = Math.max(r, g, b), min = Math.min(r, g, b), l = (max + min) / 2, d = max - min
  if (!d) return [0, 0, l]
  const s = d / (1 - Math.abs(2 * l - 1))
  const h = max === r ? ((g - b) / d + 6) % 6 : max === g ? (b - r) / d + 2 : (r - g) / d + 4
  return [h * 60, s, l]
}

// useCoverTint gives a colour for the page behind a cover: deep on a dark theme, pale on
// a light one, so that text stays readable on it either way.
export function useCoverTint(url: string | null): string | null {
  const [tint, setTint] = useState<[number, number, number] | null>(null)
  useEffect(() => {
    if (!url) { setTint(null); return }
    if (tints.has(url)) { setTint(tints.get(url)!); return }
    let stale = false
    const img = new Image()
    img.decoding = 'async'
    img.onload = () => { const t = averageColour(img); tints.set(url, t); if (!stale) setTint(t) }
    img.onerror = () => { tints.set(url, null); if (!stale) setTint(null) }
    img.src = url
    return () => { stale = true }
  }, [url])
  if (!tint) return null
  const [h, s] = hsl(tint)
  return isDarkBackground()
    ? `hsl(${Math.round(h)} ${Math.round(Math.min(s, 0.62) * 100)}% 26%)`
    : `hsl(${Math.round(h)} ${Math.round(Math.min(s, 0.7) * 100)}% 84%)`
}

function isDarkBackground() {
  const v = getComputedStyle(document.documentElement).getPropertyValue('--bg-rgb').trim().split(/\s+/).map(Number)
  if (v.length < 3 || v.some(Number.isNaN)) return true
  return 0.2126 * v[0] + 0.7152 * v[1] + 0.0722 * v[2] < 128
}

export const consoleMap = (list: GameConsole[]) => new Map(list.map((c) => [c.id, c]))
