// What GET /api/logs and its neighbours return, and the small things the log
// pages share. See internal/logging and internal/server/api_logs.go.

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
export type LogFile = { name: string; size: number; modified: string }
export type Crash = { id: string; kind: string; time: string; summary: string; version?: string; size: number; seen: boolean }
export type LogsView = {
  dir: string; level: string; categories: Category[]; levels: Record<string, string>; on: string[]
  files: LogFile[]; canOpen: boolean; health: Health; crashes: Crash[]; modules: LogModule[]
}
export type BundlePart = { name: string; description: string; size: number }
export type BundlePreview = { parts: BundlePart[]; total: number; sample: string[]; redact: boolean; days: number }

// The words for a level. Colour is only ever added to these.
export const LEVEL_WORDS: Record<string, string> = { debug: 'Debug', info: 'Info', warn: 'Warning', error: 'Error', crash: 'Crash' }

export function fmtSize(n = 0): string {
  if (n >= 1 << 30) return (n / (1 << 30)).toFixed(1) + ' GB'
  if (n >= 1 << 20) return (n / (1 << 20)).toFixed(1) + ' MB'
  return Math.max(n ? 1 : 0, Math.round(n / 1024)) + ' KB'
}

export const fmtDate = (t: string) => new Date(t).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })

export function fmtAge(t: string): string {
  const min = Math.round((Date.now() - new Date(t).getTime()) / 60000)
  if (min < 2) return 'just now'
  if (min < 90) return `${min} minutes ago`
  const h = Math.round(min / 60)
  if (h < 36) return `${h} hours ago`
  return `${Math.round(h / 24)} days ago`
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
