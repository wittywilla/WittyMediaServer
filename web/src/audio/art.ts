// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'

// A colour from the artwork, for what's behind it. Read once per picture from a tiny copy,
// so it costs one small draw, and kept for the page's lifetime. A picture from another
// address can't be read (the browser won't allow it): those simply get the neutral colour.

const known = new Map<string, string>()
const NEUTRAL = '120 120 135'

function pick(img: HTMLImageElement): string {
  const size = 16
  const c = document.createElement('canvas')
  c.width = c.height = size
  const g = c.getContext('2d', { willReadFrequently: true })
  if (!g) return NEUTRAL
  g.drawImage(img, 0, 0, size, size)
  let data: Uint8ClampedArray
  try { data = g.getImageData(0, 0, size, size).data } catch { return NEUTRAL }
  // The most colourful bright part, as a plain average of a poster is usually grey mud.
  let r = 0, gr = 0, b = 0, weight = 0, rAll = 0, gAll = 0, bAll = 0
  for (let i = 0; i < data.length; i += 4) {
    const R = data[i], G = data[i + 1], B = data[i + 2]
    const hi = Math.max(R, G, B), lo = Math.min(R, G, B)
    const w = (hi - lo + 8) * (hi + 24)
    r += R * w; gr += G * w; b += B * w; weight += w
    rAll += R; gAll += G; bAll += B
  }
  if (!weight) return NEUTRAL
  let R = r / weight, G = gr / weight, B = b / weight
  // Lifted to a usable brightness, and never so dark it vanishes on black.
  const peak = Math.max(R, G, B, 1)
  const lift = Math.min(2.2, 190 / peak)
  R *= lift; G *= lift; B *= lift
  return `${Math.round(Math.min(255, R))} ${Math.round(Math.min(255, G))} ${Math.round(Math.min(255, B))}`
}

export function useArtColour(url: string | null | undefined) {
  const [rgb, setRgb] = useState(() => (url && known.get(url)) || NEUTRAL)
  useEffect(() => {
    if (!url) { setRgb(NEUTRAL); return }
    const have = known.get(url)
    if (have) { setRgb(have); return }
    let gone = false
    const img = new Image()
    img.onload = () => {
      const v = pick(img)
      if (known.size > 400) known.clear()
      known.set(url, v)
      if (!gone) setRgb(v)
    }
    img.src = url
    return () => { gone = true }
  }, [url])
  return rgb
}
