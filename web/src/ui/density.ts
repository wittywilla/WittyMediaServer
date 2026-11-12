// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// How closely things are spaced on this device: comfortable (the default), compact or spacious
// (docs/plans/DESIGN-2.md section 2, `density`). Kept on this device and written to <html data-density>, which
// design2.css answers to; touch screens keep their full-size targets whatever is chosen.
import { useEffect, useState } from 'react'

export type Density = 'comfortable' | 'compact' | 'spacious'
export const DENSITIES: Density[] = ['comfortable', 'compact', 'spacious']
const KEY = 'wms.density'
const EVENT = 'wms-density'

export const asDensity = (v: unknown): Density => (v === 'compact' || v === 'spacious' ? v : 'comfortable')

export function getDensity(): Density {
  try { return asDensity(localStorage.getItem(KEY)) } catch { return 'comfortable' }
}

function apply(d: Density) {
  if (d === 'comfortable') delete document.documentElement.dataset.density
  else document.documentElement.dataset.density = d
}

export function setDensity(d: Density) {
  try { localStorage.setItem(KEY, d) } catch { /* for this visit only */ }
  apply(d)
  window.dispatchEvent(new Event(EVENT))
}

export function installDensity() {
  apply(getDensity())
  window.addEventListener('storage', () => apply(getDensity()))
}

export function useDensity(): Density {
  const [d, set] = useState(getDensity)
  useEffect(() => {
    const on = () => set(getDensity())
    window.addEventListener(EVENT, on)
    window.addEventListener('storage', on)
    return () => { window.removeEventListener(EVENT, on); window.removeEventListener('storage', on) }
  }, [])
  return d
}
