// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What this app says about the device with every SDUI request (docs/SDUI.md, "What a
// client sends"), worked out from plain facts so it can be tested.

import { CATALOG } from './types.ts'

export type DeviceFacts = { width: number; height: number; tvMode: boolean; desktopShell: boolean; touch: boolean; pointer: boolean; reducedMotion: boolean; hdr: boolean; locale: string; uhd: boolean }

export type Device = { platform: string; formFactor: string; deviceClass?: string; viewport?: string; capabilities: string[]; locale: string; views: string[] }

// The app's own views a NativeView can ask for: all four are in the web app.
export const VIEWS = ['guide', 'photoTimeline', 'photoMap', 'coverFlow']

export function breakpoint(width: number): string {
  return width < 600 ? 'compact' : width < 840 ? 'medium' : width < 1200 ? 'expanded' : 'large'
}

// TV mode is a television whatever the window; a narrow window is a phone and a touch
// screen of middling width a tablet, so each gets the sizes made for it; the rest is the
// web (or the desktop app around it).
export function describe(f: DeviceFacts): Device {
  const platform = f.desktopShell ? 'desktop' : 'web'
  const capabilities = [f.touch && 'touch', f.pointer && 'pointer', 'keyboard', f.hdr && 'hdr', f.reducedMotion && 'reducedMotion'].filter(Boolean) as string[]
  const base = { platform, capabilities, locale: f.locale, views: VIEWS }
  if (f.tvMode) return { ...base, formFactor: 'tv', deviceClass: f.uhd ? '4k' : 'hd' }
  const viewport = `${Math.max(1, Math.round(f.width))}x${Math.max(1, Math.round(f.height))}`
  const formFactor = f.width < 600 ? 'phone' : f.touch && !f.pointer && f.width < 1200 ? 'tablet' : f.desktopShell ? 'desktop' : 'web'
  return { ...base, formFactor, viewport }
}

// The part of the device a rendered page depends on: when it changes, ask again.
export const deviceKey = (d: Device, width: number) => [d.platform, d.formFactor, d.deviceClass || '', d.formFactor === 'tv' ? '' : breakpoint(width), d.capabilities.includes('reducedMotion') ? 'rm' : ''].join('|')

export function query(d: Device, extra: Record<string, string | undefined> = {}): string {
  const q = new URLSearchParams()
  q.set('catalog', String(CATALOG))
  q.set('platform', d.platform)
  q.set('formFactor', d.formFactor)
  if (d.deviceClass) q.set('deviceClass', d.deviceClass)
  if (d.viewport) q.set('viewport', d.viewport)
  if (d.views.length) q.set('views', d.views.join(','))
  if (d.capabilities.length) q.set('capabilities', d.capabilities.join(','))
  if (d.locale) q.set('locale', d.locale)
  for (const [k, v] of Object.entries(extra)) if (v) q.set(k, v)
  return q.toString()
}
