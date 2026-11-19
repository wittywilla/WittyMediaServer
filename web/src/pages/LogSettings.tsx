// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api } from '../api'
import { Spinner } from '../components/common'
import { IconRefresh } from '../icons'
import SpecPage from '../spec/SpecPage'
import { logsSpec } from '../spec/specs/logs'
import { toast } from '../state'
import { date, num, t, tk, tx } from '../i18n'
import Bundle from './settings/logs/Bundle'
import Crashes, { CrashBanner } from './settings/logs/Crashes'
import Files from './settings/logs/Files'
import LogModules from './settings/logs/LogModules'
import type { Crash, Health, LogsView } from './settings/logs/types'
import Viewer from './settings/logs/Viewer'
import WhatToLog from './settings/logs/WhatToLog'
import './settings/logs/logs.css'

// Settings → WittyLog: reading the log as it's written, how much each part of the
// server and each module records, the files, crash reports and the download for a
// bug report, which every tab points to. The address says which tab, session or module, so a link can jump
// straight to one: /settings/logs?session=abc, /settings/logs?module=wittydvd.

const TABS = [
  { id: 'viewer', label: tk('Viewer') },
  { id: 'levels', label: tk('What to log') },
  { id: 'modules', label: tk('Modules') },
  { id: 'files', label: tk('Files') },
  { id: 'crashes', label: tk('Crash reports') },
  { id: 'bundle', label: tk('Download logs') },
]

// Logging itself in trouble, in plain words.
function healthProblems(h: Health): string[] {
  const out: string[] = []
  if (h.error) out.push(t("The log can't be written to its folder: {error}. Until that's fixed, lines are kept only in memory and on the console, and the server tries again every 30 seconds. Check the folder under Files.", { error: h.error }))
  if (h.lowDisk) out.push(t('The disk holding the logs is nearly full. To save room only warnings and errors are being written, and old logs are being removed.'))
  if (h.lost) out.push(t('{count} lines could not be written to the log.', { count: num(h.lost) }))
  if (h.dropped) out.push(t('{count} lines were left out because the disk could not keep up.', { count: num(h.dropped) }))
  return out
}

export default function LogSettings() {
  const [params] = useSearchParams()
  const [view, setView] = useState<LogsView | null>(null)
  const [health, setHealth] = useState<Health | null>(null)
  const [err, setErr] = useState('')
  const [saving, setSaving] = useState(false)
  const navigate = useNavigate()
  const asked = params.get('tab') || ''
  const tab = params.get('session') || params.get('module') ? 'viewer' : TABS.some((x) => x.id === asked) ? asked : 'viewer'

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

  if (err && !view) return <div className="error" role="alert">{err}<div className="lg-note-actions"><button type="button" className="btn sm" onClick={() => void load()}><IconRefresh /> {t('Try again')}</button></div></div>
  if (!view) return <Spinner />
  const problems = health ? healthProblems(health) : []
  const stopRecording = () => { api.put<LogsView>('/logs', { recordMinutes: 0 }).then(onView).catch((e) => toast(e.message)) }
  // "Something just went wrong": keep the last minutes before they're gone.
  const saveNow = async () => {
    setSaving(true)
    try {
      const c = await api.post<Crash>('/logs/save')
      await load()
      toast(t('Saved. It’s under Crash reports, and in the logs you download.'))
      navigate(`?tab=crashes&crash=${encodeURIComponent(c.id)}`)
    } catch (e: any) { toast(e.message) } finally { setSaving(false) }
  }
  return (
    <>
      <CrashBanner crashes={view.crashes} onSeen={markSeen} />
      <SpecPage embedded page={logsSpec({ recordingUntil: view.recordingUntil ? date(view.recordingUntil, { hour: 'numeric', minute: '2-digit' }) : '', problems, flight: !!view.flight, showDownload: tab !== 'bundle', saving })}
        handled={new Set(['stop', 'save'])} onAction={(id) => { if (id === 'stop') stopRecording(); else void saveNow() }} />
      <nav className="tabs lg-tabs" aria-label="WittyLog">
        {TABS.map((x) => (
          <Link key={x.id} to={x.id === 'viewer' ? '?' : `?tab=${x.id}`} className={tab === x.id ? 'active' : ''} aria-current={tab === x.id ? 'page' : undefined}>{tx(x.label)}</Link>
        ))}
      </nav>
      {tab === 'viewer' && <Viewer categories={view.categories} modules={view.modules} kinds={view.kinds || []} onHealth={onHealth} />}
      {tab === 'levels' && <WhatToLog view={view} onView={onView} />}
      {tab === 'modules' && <LogModules view={view} onView={onView} />}
      {tab === 'files' && <Files view={view} onView={onView} reload={() => void load()} />}
      {tab === 'crashes' && <Crashes crashes={view.crashes} reload={() => void load()} onSeen={markSeen} />}
      {tab === 'bundle' && <Bundle />}
    </>
  )
}
