// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The photo map's arithmetic, kept apart from drawing so it can be tested
// (mapMath.test.mjs). Positions are Web Mercator as a share of the world's
// width from the top left, as online maps and the server's grouping use:
// 256 pixels across the world at zoom 0, twice that at each step.

export const TILE = 256
export const MAX_ZOOM = 19
const MERC_LAT = 85.05112878

export type View = { x: number; y: number; z: number } // the centre, and the zoom
export type Size = { w: number; h: number }
export type Box = { south: number; west: number; north: number; east: number } // west > east crosses the date line

export const mercX = (lon: number) => (lon + 180) / 360
export const mercY = (lat: number) => {
  const r = (Math.min(MERC_LAT, Math.max(-MERC_LAT, lat)) * Math.PI) / 180
  return Math.min(1, Math.max(0, 0.5 - Math.log(Math.tan(Math.PI / 4 + r / 2)) / (2 * Math.PI)))
}
export const lonOf = (x: number) => x * 360 - 180
export const latOf = (y: number) => (Math.atan(Math.sinh((0.5 - y) * 2 * Math.PI)) * 180) / Math.PI
export const wrapLon = (lon: number) => ((((lon + 180) % 360) + 360) % 360) - 180
export const scaleOf = (z: number) => TILE * 2 ** z // pixels across the world

// Zoomed right out, the whole world fits the shorter side.
export const minZoom = (s: Size) => Math.max(0, Math.log2(Math.min(s.w, s.h) / TILE))

// Keeps the zoom in range, the poles from scrolling away (the world is centred
// when it's shorter than the map), and x near the world drawn first, as the
// map goes round and round sideways.
export function clampView(v: View, s: Size): View {
  const z = Math.min(MAX_ZOOM, Math.max(minZoom(s), v.z))
  const half = s.h / 2 / scaleOf(z)
  const y = half >= 0.5 ? 0.5 : Math.min(1 - half, Math.max(half, v.y))
  const x = v.x - Math.floor(v.x)
  return { x, y, z }
}

// The view after zooming by dz around a point on the map (px, py from its top
// left), which stays under the pointer.
export function zoomAt(v: View, s: Size, px: number, py: number, dz: number): View {
  const z = Math.min(MAX_ZOOM, Math.max(minZoom(s), v.z + dz))
  const a = scaleOf(v.z), b = scaleOf(z)
  const wx = v.x + (px - s.w / 2) / a, wy = v.y + (py - s.h / 2) / a
  return clampView({ x: wx - (px - s.w / 2) / b, y: wy - (py - s.h / 2) / b, z }, s)
}

// The view that shows a box, with room around it.
export function fitView(b: Box, s: Size, maxZoom = 14): View {
  const east = b.east < b.west ? b.east + 360 : b.east
  const x1 = mercX(b.west), x2 = mercX(east), y1 = mercY(b.north), y2 = mercY(b.south)
  const room = 0.8
  const zx = Math.log2((s.w * room) / (TILE * Math.max(x2 - x1, 1e-9)))
  const zy = Math.log2((s.h * room) / (TILE * Math.max(y2 - y1, 1e-9)))
  return clampView({ x: (x1 + x2) / 2, y: (y1 + y2) / 2, z: Math.min(zx, zy, maxZoom) }, s)
}

// The grouping zoom asked of the server: groups for the zoom below are never
// closer than a pin's width, and only spread out as the map zooms in further.
export const groupZoom = (z: number) => Math.max(0, Math.floor(z + 1e-6))

// What to ask the server for: the part in view and half a screen around it,
// snapped outwards to a grid of 512 pixels so small moves ask the same
// question (and the answer is reused). Longitudes wrap; a box round the whole
// world is -180 to 180.
export function viewBox(v: View, s: Size): Box {
  const zg = groupZoom(v.z)
  const scale = scaleOf(v.z)
  const step = 512 / scaleOf(zg)
  const mx = s.w / scale, my = s.h / scale // a screen, in world widths
  const left = Math.floor((v.x - mx) / step) * step, right = Math.ceil((v.x + mx) / step) * step
  const top = Math.max(0, Math.floor((v.y - my) / step) * step), bottom = Math.min(1, Math.ceil((v.y + my) / step) * step)
  const north = top <= 0 ? 90 : latOf(top), south = bottom >= 1 ? -90 : latOf(bottom)
  if (right - left >= 1) return { south, west: -180, north, east: 180 }
  const west = wrapLon(lonOf(left)), east = wrapLon(lonOf(right))
  return { south, west, north, east: east === -180 && west > -180 ? 180 : east }
}

export const boxParam = (b: Box) => [b.south, b.west, b.north, b.east].map((n) => +n.toFixed(5)).join(',')

// The copies of the world in view: the map repeats sideways, so a place near
// the date line shows on both sides of it when zoomed out.
export function worldCopies(v: View, s: Size, margin = 0): number[] {
  const scale = scaleOf(v.z)
  const from = Math.floor(v.x - (s.w / 2 + margin) / scale), to = Math.floor(v.x + (s.w / 2 + margin) / scale)
  const out: number[] = []
  for (let k = from; k <= to && out.length < 12; k++) out.push(k)
  return out
}

// Where a point of the map is on screen, in the copy of the world k.
export const toScreen = (v: View, s: Size, x: number, y: number, k = 0) => {
  const scale = scaleOf(v.z)
  return { left: (x + k - v.x) * scale + s.w / 2, top: (y - v.y) * scale + s.h / 2 }
}

// An online map's square: {z}, {x} and {y}, and {s} for providers that share
// the work between servers.
export const tileUrl = (template: string, z: number, x: number, y: number) =>
  template.replace('{z}', String(z)).replace('{x}', String(x)).replace('{y}', String(y)).replace('{s}', 'abc'[(x + y) % 3]).replace('{r}', '')

// "48.86° N, 2.35° E": what a place is called until places have names.
export function placeName(lat: number, lon: number): string {
  const f = (n: number) => Math.abs(n).toFixed(Math.abs(n) >= 100 ? 1 : 2)
  return `${f(lat)}° ${lat >= 0 ? 'N' : 'S'}, ${f(wrapLon(lon))}° ${wrapLon(lon) >= 0 ? 'E' : 'W'}`
}
