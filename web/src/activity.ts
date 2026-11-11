// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useSyncExternalStore } from 'react'
import { announce } from './a11y'
import { ApiError, api, getToken, serverBase } from './api'

// What the server is doing in the background, kept up to date by one stream. The stream
// is open only while a tab is showing: a browser allows few connections per server, and
// a tab left in the background shouldn't hold one. docs/TASKS.md has the API.

export type TaskState = 'queued' | 'running' | 'paused' | 'failed' | 'done' | 'cancelled'
export type TaskProblem = { item?: string; message: string; count?: number; at: number }
export type Task = {
  id: string; type: string; title: string; state: TaskState; phase?: string; libraryId?: number; libraryName?: string
  // The phases it goes through, and which it's on from 1: the bar then covers all of them.
  steps?: string[]; step?: number
  item?: string; path?: string; done: number; total: number; unit?: string; percent: number; rate?: number; etaSec?: number
  startedAt: number; updatedAt: number; finishedAt?: number; by?: string; source?: string; result?: string; error?: string; fix?: string; detail?: string
  problemCount?: number; problems?: TaskProblem[]; canPause: boolean; canResume: boolean; canCancel: boolean; canRetry: boolean
}
export type ActivityAccess = { enabled: boolean; visible: boolean; readOnly: boolean }
export type ActivitySummary = { active: number; queued: number; paused: number; percent: number }
export type ActivityView = { loaded: boolean; connected: boolean; access: ActivityAccess; tasks: Task[]; recent: Task[]; summary: ActivitySummary; hidden: boolean; unseenProblems: number }

const NOBODY: ActivityAccess = { enabled: false, visible: false, readOnly: true }
const IDLE: ActivitySummary = { active: 0, queued: 0, paused: 0, percent: -1 }
const SEEN_KEY = 'wms-activity-seen'

let view: ActivityView = { loaded: false, connected: false, access: NOBODY, tasks: [], recent: [], summary: IDLE, hidden: false, unseenProblems: 0 }
const listeners = new Set<() => void>()
let abort: AbortController | null = null
let retry: ReturnType<typeof setTimeout> | null = null
let watchdog: ReturnType<typeof setTimeout> | null = null
let attempts = 0
let panelOpen = false
const spoken = new Map<string, { at: number; pct: number }>()

function seenAt() { try { return Number(localStorage.getItem(SEEN_KEY)) || 0 } catch { return 0 } }

function publish(next: Partial<ActivityView>) {
  view = { ...view, ...next }
  view.unseenProblems = view.recent.filter((t) => t.state === 'failed' && (t.finishedAt ?? 0) > seenAt()).length
  listeners.forEach((l) => l())
}

// Someone has looked at the panel, so what failed so far no longer needs pointing out.
export function markProblemsSeen() {
  try { localStorage.setItem(SEEN_KEY, String(Math.floor(Date.now() / 1000))) } catch { /* it just shows again */ }
  publish({})
}

// Starts, ends and failures are read out as they happen. While the panel is open, progress
// is too, by quarters and no more than twice a minute. Nothing else is, however fast it moves.
function speak(before: Task[], changed: Task[]) {
  const had = new Set(before.map((t) => t.id))
  for (const t of changed) {
    if (t.state === 'failed') announce(`${t.title} failed. ${t.error || ''}`)
    else if (t.state === 'done') announce(`${t.title} finished.`)
    else if (t.state === 'cancelled') announce(`${t.title} cancelled.`)
    else if (!had.has(t.id)) { announce(`${t.title} started.`); spoken.set(t.id, { at: Date.now(), pct: 0 }) }
    else if (panelOpen && t.percent >= 0) {
      const was = spoken.get(t.id) ?? { at: 0, pct: 0 }
      if (t.percent - was.pct >= 25 && Date.now() - was.at >= 30000) {
        announce(`${t.title}, ${Math.round(t.percent)} percent.`)
        spoken.set(t.id, { at: Date.now(), pct: t.percent })
      }
    }
    if (t.state === 'failed' || t.state === 'done' || t.state === 'cancelled') spoken.delete(t.id)
  }
}

const isFinished = (t: Task) => t.state === 'failed' || t.state === 'done' || t.state === 'cancelled'

function merge(update: { tasks?: Task[]; gone?: string[]; summary?: ActivitySummary }) {
  const changed = update.tasks ?? []
  const gone = new Set([...(update.gone ?? []), ...changed.map((t) => t.id)])
  const tasks = view.tasks.filter((t) => !gone.has(t.id) || changed.some((c) => c.id === t.id && !isFinished(c)))
    .map((t) => changed.find((c) => c.id === t.id) ?? t)
  for (const c of changed) if (!isFinished(c) && !tasks.some((t) => t.id === c.id)) tasks.push(c)
  const recent = [...changed.filter(isFinished).reverse(), ...view.recent.filter((t) => !gone.has(t.id))].slice(0, 40)
  speak(view.tasks, changed)
  publish({ tasks, recent, summary: update.summary ?? view.summary })
}

function petWatchdog(ctl: AbortController) {
  if (watchdog) clearTimeout(watchdog)
  // The server says something at least every 25 seconds. A connection a phone or a proxy
  // has quietly dropped says nothing at all, so this is the only way to notice.
  watchdog = setTimeout(() => ctl.abort(), 60000)
}

async function readStream(ctl: AbortController) {
  const headers: Record<string, string> = {}
  const tok = getToken()
  if (tok) headers.Authorization = 'Bearer ' + tok
  const res = await fetch(serverBase() + '/api/system/tasks/stream', { headers, credentials: 'same-origin', signal: ctl.signal })
  if (!res.ok) throw new ApiError(res.status, 'activity stream refused')
  if (!res.body) throw new ApiError(0, 'this browser cannot read a stream')
  const reader = res.body.getReader()
  const text = new TextDecoder()
  let buffer = ''
  petWatchdog(ctl)
  publish({ connected: true })
  attempts = 0
  for (;;) {
    const { value, done } = await reader.read()
    if (done) return
    petWatchdog(ctl)
    buffer += text.decode(value, { stream: true })
    let end: number
    while ((end = buffer.indexOf('\n\n')) >= 0) {
      const frame = buffer.slice(0, end)
      buffer = buffer.slice(end + 2)
      let event = '', data = ''
      for (const line of frame.split('\n')) {
        if (line.startsWith('event: ')) event = line.slice(7)
        else if (line.startsWith('data: ')) data += line.slice(6)
      }
      if (!data) continue
      const msg = JSON.parse(data)
      if (event === 'snapshot') publish({ loaded: true, access: msg.access, tasks: msg.tasks, recent: msg.recent, summary: msg.summary })
      else if (event === 'update') merge(msg)
    }
  }
}

function stopStream() {
  abort?.abort()
  abort = null
  if (retry) { clearTimeout(retry); retry = null }
  if (watchdog) { clearTimeout(watchdog); watchdog = null }
}

// Asks once whether this account gets to see any of this. A plain request, which is always
// answered, so an account without access opens no stream.
const tabHidden = () => document.visibilityState === 'hidden'

async function connect() {
  stopStream()
  if (tabHidden() || !listeners.size) return
  const ctl = new AbortController()
  abort = ctl
  try {
    const first = await api.get<{ access: ActivityAccess; tasks: Task[]; recent: Task[]; summary: ActivitySummary }>('/system/tasks')
    if (ctl.signal.aborted) return
    publish({ loaded: true, access: first.access, tasks: first.tasks, recent: first.recent, summary: first.summary })
    if (!first.access.visible) { abort = null; return }
    await readStream(ctl)
  } catch (e) {
    if (e instanceof ApiError && e.status === 403) { publish({ access: { ...view.access, visible: false }, connected: false }); abort = null; return }
  }
  if (ctl.signal.aborted && abort !== ctl) return
  publish({ connected: false })
  if (tabHidden() || !listeners.size) return
  attempts++
  retry = setTimeout(connect, Math.min(30000, 1000 * 2 ** Math.min(attempts, 5)))
}

const onVisibility = () => { if (tabHidden()) { stopStream(); publish({ connected: false }) } else connect() }

function subscribe(listener: () => void) {
  if (!listeners.size) {
    document.addEventListener('visibilitychange', onVisibility)
    listeners.add(listener)
    connect()
    loadHidden()
  } else listeners.add(listener)
  return () => {
    listeners.delete(listener)
    if (!listeners.size) {
      document.removeEventListener('visibilitychange', onVisibility)
      stopStream()
      view = { ...view, loaded: false, connected: false, access: NOBODY, tasks: [], recent: [], summary: IDLE }
    }
  }
}

export const useActivity = () => useSyncExternalStore(subscribe, () => view)
export const setPanelOpen = (open: boolean) => { panelOpen = open }
// The owner changed who sees it: ask again rather than wait for a reconnect.
export const refreshActivity = () => { if (listeners.size) connect() }

// ---------------------------------------------------------------- each person's own choice

function loadHidden() {
  api.get<{ activityHidden?: boolean }>('/me/ui').then((u) => publish({ hidden: !!u.activityHidden })).catch(() => {})
}

// The server stores these choices whole, so what's there is read first and only this changes.
export async function setActivityHidden(hidden: boolean) {
  const ui = await api.get<Record<string, unknown>>('/me/ui')
  await api.put('/me/ui', { ...ui, activityHidden: hidden })
  publish({ hidden })
}

// ---------------------------------------------------------------- controls and history

export const taskAction = (id: string, action: 'pause' | 'resume' | 'cancel' | 'retry') => api.post(`/system/tasks/${encodeURIComponent(id)}/${action}`)
export const taskHistory = (q: { before?: number; limit?: number; state?: string }) => {
  const p = new URLSearchParams()
  if (q.before) p.set('before', String(q.before))
  if (q.state) p.set('state', q.state)
  p.set('limit', String(q.limit ?? 30))
  return api.get<{ tasks: Task[] }>('/system/tasks/history?' + p).then((r) => r.tasks)
}
export async function clearTaskHistory(id?: string) {
  await api.del('/system/tasks/history' + (id ? '/' + encodeURIComponent(id) : ''))
  publish({ recent: id ? view.recent.filter((t) => t.id !== id) : [] })
}

// ---------------------------------------------------------------- saying it in words

const TYPE_NAMES: Record<string, string> = {
  scan: 'Library scan', analysis: 'File analysis', metadata: 'Metadata', thumbnails: 'Thumbnails', intros: 'Intro detection', games: 'Game details',
  module: 'Module', backup: 'Backup', transcode: 'Transcode', convert: 'Library transcoding', tdarr: 'Tdarr', update: 'Server update', guide: 'TV guide', recording: 'Recording', diagnose: 'WittyDiagnoser', other: 'Background work',
}
export const typeName = (t: string) => TYPE_NAMES[t] ?? 'Background work'

export const STATE_NAMES: Record<TaskState, string> = { queued: 'Waiting', running: 'Running', paused: 'Paused', failed: 'Failed', done: 'Finished', cancelled: 'Cancelled' }

export function fmtBytes(n: number) {
  if (n < 1024) return `${Math.round(n)} B`
  const units = ['KB', 'MB', 'GB', 'TB', 'PB']
  let v = n / 1024, i = 0
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return `${v >= 100 ? Math.round(v) : v.toFixed(1)} ${units[i]}`
}

export function fmtDuration(sec: number) {
  if (sec < 60) return `${Math.max(1, Math.round(sec))} sec`
  if (sec < 3600) return `${Math.round(sec / 60)} min`
  const h = Math.floor(sec / 3600), m = Math.round((sec % 3600) / 60)
  return m ? `${h} hr ${m} min` : `${h} hr`
}

export function fmtAgo(unix: number) {
  const s = Date.now() / 1000 - unix
  if (s < 45) return 'just now'
  if (s < 86400) return `${fmtDuration(s)} ago`
  return new Date(unix * 1000).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

export function fmtCount(t: Task) {
  if (t.unit === 'bytes') return t.total > 0 ? `${fmtBytes(t.done)} of ${fmtBytes(t.total)}` : fmtBytes(t.done)
  const unit = t.unit ? ' ' + t.unit : ''
  if (t.total > 0) return `${t.done.toLocaleString()} of ${t.total.toLocaleString()}${unit}`
  return t.done > 0 ? `${t.done.toLocaleString()}${unit}` : ''
}

export function fmtRate(t: Task) {
  if (!t.rate || t.rate <= 0) return ''
  if (t.unit === 'bytes') return `${fmtBytes(t.rate)}/s`
  const unit = t.unit || 'items'
  return t.rate >= 10 ? `${Math.round(t.rate)} ${unit}/s` : t.rate >= 0.1 ? `${t.rate.toFixed(1)} ${unit}/s` : `${Math.round(t.rate * 60)} ${unit}/min`
}

export function buttonLabel(a: ActivityView) {
  const { active, queued, paused, percent } = a.summary
  const going = active - paused
  const parts: string[] = []
  if (going > 0) parts.push(`${going} running`)
  if (paused > 0) parts.push(`${paused} paused`)
  if (queued > 0) parts.push(`${queued} waiting`)
  if (percent >= 0 && going > 0) parts.push(`${Math.round(percent)}% done`)
  if (a.unseenProblems > 0) parts.push(`${a.unseenProblems} failed`)
  return parts.length ? `Activity: ${parts.join(', ')}` : 'Activity: nothing going on'
}
