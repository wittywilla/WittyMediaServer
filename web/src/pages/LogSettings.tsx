import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api } from '../api'
import { Spinner } from '../components/common'
import { IconRefresh } from '../icons'
import { toast } from '../state'
import Bundle from './settings/logs/Bundle'
import Crashes, { CrashBanner } from './settings/logs/Crashes'
import Files from './settings/logs/Files'
import LogModules from './settings/logs/LogModules'
import type { Health, LogsView } from './settings/logs/types'
import Viewer from './settings/logs/Viewer'
import WhatToLog from './settings/logs/WhatToLog'
import './settings/logs/logs.css'

// Settings → WittyLog: reading the log as it's written, how much each part of the
// server and each module records, the files, crash reports and the bundle for a
// bug report. The address says which tab, session or module, so a link can jump
// straight to one: /settings/logs?session=abc, /settings/logs?module=wittydvd.

const TABS = [
  { id: 'viewer', label: 'Viewer' },
  { id: 'levels', label: 'What to log' },
  { id: 'modules', label: 'Modules' },
  { id: 'files', label: 'Files' },
  { id: 'crashes', label: 'Crash reports' },
  { id: 'bundle', label: 'Support bundle' },
]

// Logging itself in trouble, in plain words.
function healthProblems(h: Health): string[] {
  const out: string[] = []
  if (h.error) out.push(`The log can't be written to its folder: ${h.error}. Until that's fixed, lines are kept only in memory and on the console, and the server tries again every 30 seconds. Check the folder under Files.`)
  if (h.lowDisk) out.push('The disk holding the logs is nearly full. To save room only warnings and errors are being written, and old logs are being removed.')
  if (h.lost) out.push(`${h.lost.toLocaleString()} lines could not be written to the log.`)
  if (h.dropped) out.push(`${h.dropped.toLocaleString()} lines were left out because the disk could not keep up.`)
  return out
}

export default function LogSettings() {
  const [params] = useSearchParams()
  const [view, setView] = useState<LogsView | null>(null)
  const [health, setHealth] = useState<Health | null>(null)
  const [err, setErr] = useState('')
  const asked = params.get('tab') || ''
  const tab = params.get('session') || params.get('module') ? 'viewer' : TABS.some((t) => t.id === asked) ? asked : 'viewer'

  const load = useCallback(() => {
    setErr('')
    return api.get<LogsView>('/logs').then((v) => { setView(v); setHealth(v.health) }).catch((e) => setErr(e.message))
  }, [])
  useEffect(() => { void load() }, [load])

  // The viewer's every answer carries this; only a change is worth drawing.
  const lastHealth = useRef('')
  const onHealth = useCallback((h: Health) => {
    const s = JSON.stringify(h)
    if (s !== lastHealth.current) { lastHealth.current = s; setHealth(h) }
  }, [])
  const onView = useCallback((v: LogsView) => { setView(v); setHealth(v.health) }, [])

  const markSeen = useCallback((id: string) => {
    api.post(`/logs/crashes/${encodeURIComponent(id)}/seen`).then(() => {
      setView((v) => v && { ...v, crashes: v.crashes.map((c) => (c.id === id ? { ...c, seen: true } : c)) })
    }).catch((e) => toast(e.message))
  }, [])

  // On a phone the tabs scroll sideways: keep the open one in sight, for a deep link.
  useEffect(() => {
    const a = document.querySelector<HTMLElement>('.lg-tabs a.active')
    if (a?.parentElement) a.parentElement.scrollLeft = a.offsetLeft - a.parentElement.offsetLeft - 16
  }, [tab, !view])

  if (err && !view) return <div className="error" role="alert">{err}<div className="lg-note-actions"><button type="button" className="btn sm" onClick={() => void load()}><IconRefresh /> Try again</button></div></div>
  if (!view) return <Spinner />
  const problems = health ? healthProblems(health) : []
  const stopRecording = () => { api.put<LogsView>('/logs', { recordMinutes: 0 }).then(onView).catch((e) => toast(e.message)) }
  return (
    <>
      <CrashBanner crashes={view.crashes} onSeen={markSeen} />
      {view.recordingUntil && (
        <div className="lg-banner" role="status">
          <div className="lg-banner-text">
            <b>Recording everything until {new Date(view.recordingUntil).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}</b>
            <p>Try to make the problem happen now. Logging goes back to your usual settings by itself.</p>
          </div>
          <div className="lg-banner-actions"><button type="button" className="btn sm" onClick={stopRecording}>Stop now</button></div>
        </div>
      )}
      {problems.length > 0 && (
        <div className="lg-banner warn" role="status">
          <div className="lg-banner-text">
            <b>Logging has a problem</b>
            {problems.map((p) => <p key={p}>{p}</p>)}
          </div>
        </div>
      )}
      <nav className="tabs lg-tabs" aria-label="WittyLog">
        {TABS.map((t) => (
          <Link key={t.id} to={t.id === 'viewer' ? '?' : `?tab=${t.id}`} className={tab === t.id ? 'active' : ''} aria-current={tab === t.id ? 'page' : undefined}>{t.label}</Link>
        ))}
      </nav>
      {tab === 'viewer' && <Viewer categories={view.categories} modules={view.modules} onHealth={onHealth} />}
      {tab === 'levels' && <WhatToLog view={view} onView={onView} />}
      {tab === 'modules' && <LogModules view={view} onView={onView} />}
      {tab === 'files' && <Files view={view} onView={onView} reload={() => void load()} />}
      {tab === 'crashes' && <Crashes crashes={view.crashes} reload={() => void load()} onSeen={markSeen} />}
      {tab === 'bundle' && <Bundle />}
    </>
  )
}
