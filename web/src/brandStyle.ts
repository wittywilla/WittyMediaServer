// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The server's name with a colour for every letter (docs/plans/DESIGN-2.md 7.6): the editor works on one colour per
// character ("cells"), the server keeps them as spans (internal/brandname/colors.go, the `brandSpans` setting) and sends
// the name back as parts ready to draw. Nothing here touches the page, so it is tested (brandStyle.test.mjs).
import { accentMask, words } from './brandName.ts'

export type BrandColor = { kind: 'fg' | 'accent' | 'token' | 'static' | 'gradient'; token?: string; hex?: string; stops?: string[]; angle?: number }
export type BrandSpan = { from: number; to: number; color: BrandColor; bold?: boolean }
export type StyledPart = { text: string; accent?: boolean; color?: BrandColor; bold?: boolean }

export const MAX_SPANS = 40
export const FG: BrandColor = { kind: 'fg' }
export const ACCENT: BrandColor = { kind: 'accent' }
export const token = (t: string): BrandColor => ({ kind: 'token', token: t })
export const fixed = (hex: string): BrandColor => ({ kind: 'static', hex: hex.toLowerCase() })
export const gradient = (stops: string[], angle = 90): BrandColor => ({ kind: 'gradient', stops: stops.map((s) => s.toLowerCase()), angle })

export const sameColor = (a: BrandColor, b: BrandColor) => JSON.stringify(a) === JSON.stringify(b)
export const isHex = (v: string) => /^#[0-9a-fA-F]{6}$/.test(v)

// What each theme-following swatch is called, and the colour token it is.
export const THEME_SWATCHES: { label: string; color: BrandColor }[] = [
  { label: 'Text', color: FG },
  { label: 'Accent', color: ACCENT },
  { label: 'Success', color: token('ok') },
  { label: 'Warning', color: token('warn') },
  { label: 'Danger', color: token('danger') },
]
export const FIXED_SWATCHES = ['#b4491f', '#1e9be0', '#3ddc84', '#ffd84a', '#ff4fa3']
export const GRADIENT_SWATCHES: BrandColor[] = [gradient(['#0a6aa8', '#1e9be0', '#3aa84a']), gradient(['#ff4fa3', '#ffd84a'])]
const RAINBOW = ['#ff4040', '#ffff00', '#00ff00', '#00ffff', '#ff00ff']

export const nameLength = (name: string) => Array.from(name).length

// The colour of every character: from the saved spans (characters between them are the text colour).
export function cellsFromSpans(n: number, spans: BrandSpan[]): BrandColor[] {
  const cells: BrandColor[] = Array.from({ length: n }, () => FG)
  for (const s of spans) for (let i = s.from; i < s.to && i < n; i++) cells[i] = s.color
  return cells
}

// The colour of every character of an automatic or quick colouring ('auto', 'one', 'pick:...').
export function cellsFromColouring(name: string, colouring: string): BrandColor[] {
  return accentMask(name, colouring).map((on) => (on ? ACCENT : FG))
}

// The spans the server keeps. The text colour is the default and is not written, except when that is all there is: the
// server draws its automatic colouring for a name with no spans.
export function spansFromCells(cells: BrandColor[]): BrandSpan[] {
  const out: BrandSpan[] = []
  let start = 0
  for (let i = 1; i <= cells.length; i++) {
    if (i === cells.length || !sameColor(cells[i], cells[start])) {
      if (cells[start].kind !== 'fg') out.push({ from: start, to: i, color: cells[start] })
      start = i
    }
  }
  if (!out.length && cells.length) return [{ from: 0, to: cells.length, color: FG }]
  return out
}

// How many spans a set of cells needs (the server takes MAX_SPANS).
export const spanCount = (cells: BrandColor[]) => spansFromCells(cells).length

// The name split into runs of one colour.
export function partsFromCells(name: string, cells: BrandColor[]): StyledPart[] {
  const r = Array.from(name)
  const out: StyledPart[] = []
  let start = 0
  for (let i = 1; i <= r.length; i++) {
    if (i === r.length || !sameColor(cells[i] || FG, cells[start] || FG)) {
      const c = cells[start] || FG
      out.push({ text: r.slice(start, i).join(''), ...(c.kind === 'accent' ? { accent: true } : {}), color: c })
      start = i
    }
  }
  return out
}

// Presets: whole-name colourings to start from.
export type Preset = { id: string; label: string; cells: (name: string) => BrandColor[] }
export const PRESETS: Preset[] = [
  { id: 'auto', label: 'Accent on every other word', cells: (n) => cellsFromColouring(n, 'auto') },
  { id: 'two', label: 'Two-tone', cells: (n) => { const w = words(n); const cut = w.length > 1 ? w[1][0] : Math.ceil(nameLength(n) / 2); return Array.from(n).map((_, i) => (i < cut ? FG : ACCENT)) } },
  { id: 'one', label: 'One colour', cells: (n) => Array.from(n).map(() => ACCENT) },
  { id: 'plain', label: 'Plain', cells: (n) => Array.from(n).map(() => FG) },
  { id: 'rainbow', label: 'Rainbow letters', cells: (n) => { let k = 0; return Array.from(n).map((c) => (/\s/u.test(c) ? FG : fixed(RAINBOW[k++ % RAINBOW.length]))) } },
  { id: 'gradient', label: 'Gradient', cells: (n) => Array.from(n).map(() => GRADIENT_SWATCHES[0]) },
]
export const presetLabel = (name: string) => (name ? `Accent on “${wordAt(name, 1) || name}”` : 'Accent on every other word')
function wordAt(name: string, k: number) { const w = words(name)[k]; return w ? Array.from(name).slice(w[0], w[1]).join('') : '' }

// Words of the name as ranges of characters, for choosing a whole word.
export const nameWords = words

// ---------------------------------------------------------------- the mark

export type MarkKind = 'letter' | 'initials' | 'text' | 'image'
export type Mark = { kind: MarkKind; text?: string }

// What the badge shows: the first letter, the initials of its words (three at most) or the owner's own characters.
export function markShown(name: string, m: Mark): string {
  const r = Array.from(name.trim())
  if (m.kind === 'text') return (m.text || '').trim()
  if (m.kind === 'initials') return words(name.trim()).slice(0, 3).map(([a]) => r[a]).join('').toUpperCase()
  return (r[0] || '').toUpperCase()
}

export const markSetting = (m: Mark) => (m.kind === 'letter' ? '' : JSON.stringify(m.kind === 'text' ? { kind: 'text', text: (m.text || '').trim() } : { kind: m.kind }))
export function markFromSetting(raw: string): Mark {
  try { const m = JSON.parse(raw || '{}'); if (m && ['letter', 'initials', 'text', 'image'].includes(m.kind)) return { kind: m.kind, ...(m.kind === 'text' ? { text: String(m.text || '') } : {}) } } catch { /* the first letter */ }
  return { kind: 'letter' }
}

// ---------------------------------------------------------------- contrast

export type RGB = [number, number, number]
// parseColour reads #rgb, #rrggbb, rgb() and color(srgb ...): what a browser's computed colour looks like.
export function parseColour(v: string): RGB | null {
  const s = v.trim().toLowerCase()
  let m = /^#([0-9a-f]{3})$/.exec(s)
  if (m) return [...m[1]].map((c) => parseInt(c + c, 16)) as RGB
  m = /^#([0-9a-f]{6})/.exec(s)
  if (m) return [parseInt(m[1].slice(0, 2), 16), parseInt(m[1].slice(2, 4), 16), parseInt(m[1].slice(4, 6), 16)]
  m = /^rgba?\(\s*([\d.]+)[ ,]+([\d.]+)[ ,]+([\d.]+)/.exec(s)
  if (m) return [Number(m[1]), Number(m[2]), Number(m[3])]
  m = /^color\(srgb\s+([\d.]+)\s+([\d.]+)\s+([\d.]+)/.exec(s)
  if (m) return [Number(m[1]) * 255, Number(m[2]) * 255, Number(m[3]) * 255]
  return null
}
const lin = (c: number) => { const x = c / 255; return x <= 0.03928 ? x / 12.92 : Math.pow((x + 0.055) / 1.055, 2.4) }
export const luminance = ([r, g, b]: RGB) => 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b)
export function contrast(a: RGB, b: RGB): number {
  const x = luminance(a), y = luminance(b)
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05)
}

// A colour's own look as text, for a given theme: `resolve` turns a token name into a colour string of the page.
export function colourStops(c: BrandColor, resolve: (token: string) => string): RGB[] {
  const one = (s: string) => { const p = parseColour(s); return p ? [p] : [] }
  switch (c.kind) {
    case 'static': return one(c.hex || '')
    case 'gradient': return (c.stops || []).flatMap(one)
    case 'accent': return one(resolve('brandAccent'))
    case 'token': return one(resolve(c.token || ''))
    default: return one(resolve('fg'))
  }
}

export type Warning = { text: string; surface: string; ratio: number; need: number }
// The parts of the name that are hard to read on a surface: words need 4.5 to 1 (WCAG 2.2 AA). A gradient is as weak as its weakest stop.
export function nameWarnings(parts: StyledPart[], surfaces: { name: string; colour: string }[], resolve: (token: string) => string): Warning[] {
  const out: Warning[] = []
  for (const p of parts) {
    if (!p.text.trim()) continue
    const stops = colourStops(p.color || FG, resolve)
    if (!stops.length) continue
    for (const s of surfaces) {
      const bg = parseColour(s.colour)
      if (!bg) continue
      const ratio = Math.min(...stops.map((c) => contrast(c, bg)))
      if (ratio < 4.5) out.push({ text: p.text.trim(), surface: s.name, ratio: Math.round(ratio * 10) / 10, need: 4.5 })
    }
  }
  return out
}

// The CSS token behind a colour token's name (ok -> --ok, accentText -> --accent-text).
export const cssVarOf = (t: string) => '--' + t.replace(/[A-Z]/g, (c) => '-' + c.toLowerCase())

// How a part is drawn, as inline style: nothing for the text colour and the accent (the stylesheet draws those).
export function partStyle(c: BrandColor | undefined): Record<string, string> | undefined {
  if (!c) return undefined
  switch (c.kind) {
    case 'token': return c.token ? { color: `var(${cssVarOf(c.token)})` } : undefined
    case 'static': return c.hex ? { color: c.hex } : undefined
    case 'gradient': return c.stops?.length ? { backgroundImage: `linear-gradient(${c.angle ?? 90}deg, ${c.stops.join(', ')})`, WebkitBackgroundClip: 'text', backgroundClip: 'text', color: 'transparent', WebkitTextFillColor: 'transparent' } : undefined
    default: return undefined
  }
}
