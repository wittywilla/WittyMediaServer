// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Signing a device in from another (docs/ACCOUNTS.md "Signing in with a code"): the parts
// of both pages that need no page. What this device is called on the approval page, the
// address a phone opens, and how the time and the PIN are written.

// browserName is the browser and system an address bar's user agent reveals, in the words
// the server uses for the list of signed-in devices (browserName in internal/server/
// api_auth.go), so a browser is called the same thing while it asks and once it's in.
export function browserName(ua: string): string {
  const has = (s: string) => ua.includes(s)
  const browser = has('Edg/') ? 'Edge' : has('OPR/') ? 'Opera' : has('Firefox/') ? 'Firefox' : has('Chrome/') ? 'Chrome' : has('Safari/') ? 'Safari' : ''
  const system = has('Android') ? 'Android' : has('iPhone') || has('iPad') ? 'iOS' : has('Windows') ? 'Windows' : has('Mac OS X') ? 'macOS' : has('Linux') ? 'Linux' : ''
  if (browser && system) return `${browser} on ${system}`
  return browser || system || 'Web browser'
}

// approveLink is the whole address of the approval page for a QR code. The server gives it
// whole when it knows its own address; otherwise it gives a path, and the address this
// device reached the server at goes in front. "" when neither makes an address.
export function approveLink(approveUrl: string, origin: string): string {
  if (/^https?:\/\//i.test(approveUrl)) return approveUrl
  if (!approveUrl.startsWith('/') || !/^https?:\/\//i.test(origin)) return ''
  return origin.replace(/\/+$/, '') + approveUrl
}

// typedAddress is that page as someone would type it: no scheme for plain http's usual
// case, no code (the page asks for it).
export function typedAddress(link: string): string {
  try {
    const u = new URL(link)
    return (u.protocol === 'https:' ? 'https://' : '') + u.host + u.pathname.replace(/\/+$/, '')
  } catch { return '' }
}

// onlyThisDevice: the address means "this very machine" (localhost), so a phone that scans
// it gets nowhere. The code typed on a device that's already signed in still works.
export function onlyThisDevice(link: string): boolean {
  try {
    const h = new URL(link).hostname.toLowerCase()
    return h === 'localhost' || h.endsWith('.localhost') || h === '[::1]' || /^127\.\d+\.\d+\.\d+$/.test(h)
  } catch { return false }
}

// clock: seconds as m:ss, never below 0:00.
export function clock(seconds: number): string {
  const s = Math.max(0, Math.ceil(seconds))
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

// pinGroups: a PIN in two halves, which is how people read six digits aloud.
export function pinGroups(pin: string): string {
  const half = Math.ceil(pin.length / 2)
  return pin.length > 4 ? `${pin.slice(0, half)} ${pin.slice(half)}` : pin
}

// tidyCode: a code as typed, the way the device shows it. Letters become capitals and an
// 8-letter code gets its dash; digits (the 6-digit kind) and anything else are left alone
// for the server to judge.
export function tidyCode(typed: string): string {
  const letters = typed.toUpperCase().replace(/[^A-Z0-9]/g, '')
  if (/^[A-Z]{8}$/.test(letters)) return letters.slice(0, 4) + '-' + letters.slice(4)
  return typed.trim().toUpperCase()
}

// serverClockOffset: how far the server's clock is ahead of this device's, in seconds, from
// an answer that gives a time both as a moment (expiresAt, by the server's clock) and as a
// wait from now (expiresInSec). An answer without the wait gives 0: this device's clock is used.
export function serverClockOffset(expiresAt: number, expiresInSec: number | undefined, localNow: number): number {
  if (!Number.isFinite(expiresAt) || !Number.isFinite(expiresInSec) || !expiresInSec || expiresInSec <= 0) return 0
  return expiresAt - expiresInSec - localNow
}
