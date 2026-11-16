// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What GET /api/logs and its neighbours return, and the small things the log
// pages share. See internal/logging and internal/server/api_logs.go.
import { date, num, t, tk, tn } from '../../../i18n'

export type LogAttr = { k: string; v: string }
export type Explanation = { id: string; title: string; what: string; fix: string }
export type LogRecord = {
  seq: number; t: string; level: string; cat: string; session?: string; req?: string; module?: string; topic?: string
  msg: string; attrs?: LogAttr[]; explain?: Explanation
}
export type LogTopic = { id: string; label: string; description: string; on: boolean }
export type LogModule = {
  id: string; name: string; version: string; delivery?: string; new?: boolean; missing?: boolean; firstSeen: string
  level: string; topics: LogTopic[]; lines: number; warnings: number; errors: number
}
export type Health = {
  writing: boolean; error?: string; since?: string; lost?: number; dropped?: number; quieted?: number
  lowDisk?: boolean; totalSize?: number; maxSize: number; keepDays: number
}
export type EntriesPage = { entries: LogRecord[]; last: number; more: boolean; partial: boolean; health?: Health }
export type Category = { id: string; label: string; description: string; default: string }
// A kind of work whose lines share one ID on the server, in modules and in the apps.
export type SessionKind = { id: string; label: string; prefix: string; cats?: string[] }
export type LogFile = { name: string; size: number; modified: string }
export type Crash = { id: string; kind: string; time: string; summary: string; version?: string; size: number; seen: boolean; parts?: string[] }
export type LogsView = {
  dir: string; level: string; categories: Category[]; levels: Record<string, string>; on: string[]
  files: LogFile[]; canOpen: boolean; health: Health; crashes: Crash[]; modules: LogModule[]; recordingUntil?: string
  flight: boolean; trace: boolean; kinds?: SessionKind[]
}
export type BundlePart = { name: string; description: string; size: number }
export type BundlePreview = {
  parts: BundlePart[]; total: number; sample: string[]; sanitize: boolean; days: number
  scanned: number; replaced?: Record<string, number>; changed?: { before: string; after: string }[]
}

// The words for a level. Colour is only ever added to these. Shown with tx(); a
// copied line keeps the English.
export const LEVEL_WORDS: Record<string, string> = { debug: tk('Debug'), info: tk('Info'), warn: tk('Warning'), error: tk('Error'), crash: tk('Crash') }

export function fmtSize(n = 0): string {
  const one = { minimumFractionDigits: 1, maximumFractionDigits: 1 }
  if (n >= 1 << 30) return t('{size} GB', { size: num(n / (1 << 30), one) })
  if (n >= 1 << 20) return t('{size} MB', { size: num(n / (1 << 20), one) })
  return t('{size} KB', { size: num(Math.max(n ? 1 : 0, Math.round(n / 1024))) })
}

export const fmtDate = (when: string) => date(when, { dateStyle: 'medium', timeStyle: 'short' })

export function fmtAge(when: string): string {
  const min = Math.round((Date.now() - new Date(when).getTime()) / 60000)
  if (min < 2) return t('just now')
  if (min < 90) return tn('{count} minute ago', '{count} minutes ago', min)
  const h = Math.round(min / 60)
  if (h < 36) return tn('{count} hour ago', '{count} hours ago', h)
  const d = Math.round(h / 24)
  return tn('{count} day ago', '{count} days ago', d)
}

// A long text in a box: the arrows scroll it until they reach an end, and then
// move on, so a TV remote can read all of it and still get out.
export function scrollKeys(e: React.KeyboardEvent<HTMLElement>) {
  const el = e.currentTarget
  const step = Math.max(60, el.clientHeight * 0.6)
  if (e.key === 'ArrowDown' && el.scrollTop + el.clientHeight < el.scrollHeight - 1) { el.scrollTop += step; e.preventDefault() }
  else if (e.key === 'ArrowUp' && el.scrollTop > 0) { el.scrollTop -= step; e.preventDefault() }
}

// The clipboard API exists only on secure pages, and a server at home usually
// isn't one.
export async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard) { await navigator.clipboard.writeText(text); return true }
  } catch { /* try the old way */ }
  const was = document.activeElement as HTMLElement | null
  const ta = document.createElement('textarea')
  ta.value = text
  ta.setAttribute('readonly', '')
  ta.style.position = 'fixed'
  ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.select()
  let ok = false
  try { ok = document.execCommand('copy') } catch { /* not allowed */ }
  ta.remove()
  was?.focus({ preventScroll: true })
  return ok
}
