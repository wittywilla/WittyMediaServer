// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, fmtDateTime, setToken } from '../../api'
import { Spinner } from '../../components/common'
import { toast } from '../../state'
import { Tabs } from '../../ui'
import SpecPage from '../../spec/SpecPage'
import { copiesSpec, remapSpec, repairResultsSpec, repairTopSpec } from '../../spec/specs/repair'
import FindingCard, { type Finding } from './FindingCard'
import { PageHead, Section } from './ui'
import './repair.css'

// Repair: the same doctor as `wittymediaserver doctor`, for someone who'd rather not open a terminal. It
// looks first and says what it found and what each fix does, and nothing changes until
// someone presses a fix. A safety copy is made first. The server does the work, so this page
// can be left and come back to.

type Area = 'data' | 'metadata' | 'modules' | 'install' | 'features'
type Safety = 'safe' | 'careful' | 'manual'
type Issue = Finding & {
  key: string; check: string; area: Area; safety: Safety; steps?: string[]; count?: number; exclusive?: boolean
  input?: string; notChecked?: boolean; options?: { id: string; label: string; what: string; network?: boolean }[]
}
type Report = { version: string; at: number; tookMs: number; data: string; serverRunning: boolean; checks: { id: string }[]; issues: Issue[]; problems: number; warnings: number; notes: number; fixable: number }
type FixResult = { key: string; title: string; ok: boolean; done?: string; error?: string; copy?: string; restart?: boolean }
type View = {
  running?: string; phase?: string; fraction: number; report?: Report; results: FixResult[]; restart: boolean; canRestart: boolean; twoFactor: boolean; error?: string; copies: string[]
  lastRepair?: { ok: boolean; kept?: string; error?: string; at: string }
}
type RemapPlan = { from: string; to: string; roots: number; rows: number; checked: number; found: number; sample: string[]; notFound: string[]; canApply: boolean; why?: string; applied?: boolean; copy?: string }

const POLL_MS = 1200
const AREAS: { id: Area; title: string; desc: string }[] = [
  { id: 'data', title: 'The database and data folder', desc: 'Everything the server knows is kept here.' },
  { id: 'metadata', title: 'Metadata', desc: 'Titles, matches, artwork and the files beside your media.' },
  { id: 'modules', title: 'Modules', desc: 'What’s installed, whether it’s whole, and whether it’s signed.' },
  { id: 'install', title: 'The install', desc: 'FFmpeg, folders, disk space, the port and the firewall.' },
  { id: 'features', title: 'Features', desc: 'Peer to peer from away, modules under an override, emulators and their permissions, karaoke’s separator and channels failing their checks.' },
]
const SAFETY_WORDS: Record<Safety, string> = { safe: 'Safe', careful: 'Backed up first', manual: 'You’ll need to do this yourself' }
const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

// The tabs of this page. Metadata files joins here, once there's more than checks to show.
const TABS = [{ value: 'check' as const, label: 'Check and fix' }, { value: 'files' as const, label: 'Metadata files' }]

function countsLine(r: Report) {
  const part = (n: number, one: string, many: string) => (n ? `${n} ${n === 1 ? one : many}` : '')
  const line = [part(r.problems, 'problem', 'problems'), part(r.warnings, 'warning', 'warnings'), part(r.notes, 'note', 'notes')].filter(Boolean).join(', ')
  return line ? `Found ${line}.` : 'Everything looks fine.'
}

// Waits for the server to go away and come back, then starts the page again.
async function waitForRestart(say: (s: string) => void) {
  const answers = async () => {
    try { return (await fetch('/api/status', { cache: 'no-store' })).ok } catch { return false }
  }
  const started = Date.now()
  let wentAway = false
  while (Date.now() - started < 120000) {
    await new Promise((ok) => setTimeout(ok, 1000))
    const up = await answers()
    if (!up) { wentAway = true; continue }
    if (wentAway || Date.now() - started > 8000) { setToken(''); window.location.href = '/'; return }
  }
  say('The server hasn’t come back. Start it again by hand: the repair finishes as it starts.')
}

const MetadataFolder = lazy(() => import('./MetadataFolder'))

export default function Repair() {
  const [params, setParams] = useSearchParams()
  const tab = params.get('tab') === 'files' ? 'files' as const : 'check' as const
  const [view, setView] = useState<View | null>(null)
  const [err, setErr] = useState('')
  const [waiting, setWaiting] = useState('')
  const [remapFrom, setRemapFrom] = useState('')
  const remapBox = useRef<HTMLDivElement>(null)
  const summary = useRef<HTMLParagraphElement>(null)
  const wasRunning = useRef(false)

  const load = useCallback(() => api.get<View>('/repair').then((v) => {
    setView(v); setErr('')
    // Just finished: the keyboard goes to the answer, and a screen reader reads it.
    if (wasRunning.current && !v.running) requestAnimationFrame(() => summary.current?.focus())
    wasRunning.current = !!v.running
  }).catch((e) => setErr(message(e))), [])
  useEffect(() => { void load() }, [load])

  // Asks only while something runs and someone can see the answer.
  const running = !!view?.running
  useEffect(() => {
    if (!running) return
    const tick = () => { if (document.visibilityState === 'visible') void load() }
    const t = setInterval(tick, POLL_MS)
    document.addEventListener('visibilitychange', tick)
    return () => { clearInterval(t); document.removeEventListener('visibilitychange', tick) }
  }, [running, load])

  const start = async () => {
    setErr('')
    try { setView(await api.post<View>('/repair/check', {})); wasRunning.current = true } catch (e) { setErr(message(e)) }
  }
  const fix = useCallback(async (body: { keys?: string[]; allSafe?: boolean; option?: string; password?: string; code?: string }) => {
    setErr('')
    const v = await api.post<View>('/repair/fix', body)
    setView(v); wasRunning.current = true
  }, [])
  const restart = async () => {
    try {
      const r = await api.post<{ restarting: boolean }>('/repair/restart', {})
      if (!r.restarting) { setWaiting('This server can’t restart itself. Stop it and start it again, and the repair finishes then.'); return }
      setWaiting('The server is restarting to finish the repair, which can take a minute.')
      void waitForRestart(setWaiting)
    } catch (e) { toast(message(e)) }
  }
  const moved = (from: string) => {
    setRemapFrom(from)
    requestAnimationFrame(() => { remapBox.current?.scrollIntoView({ block: 'start' }); remapBox.current?.querySelector('input')?.focus() })
  }

  if (waiting) {
    return (
      <>
        <PageHead title="Repair" />
        <Section title="Please wait"><div role="status" className="set-prose"><p>{waiting}</p><p>This page moves on by itself when it’s done.</p></div></Section>
      </>
    )
  }
  if (!view) return err ? <div className="error" role="alert">{err}</div> : <Spinner />
  const rep = view.report
  const safeCount = rep ? rep.issues.filter((i) => i.safety === 'safe' && !i.notChecked).length : 0
  const copy = view.results.find((r) => r.copy)?.copy
  const topPage = repairTopSpec({
    lastRepair: view.lastRepair && { ...view.lastRepair, at: fmtDateTime(new Date(view.lastRepair.at).getTime()) },
    running: view.running || '', phase: view.phase || '', checked: rep ? fmtDateTime(rep.at) : '', error: view.error || '',
    safeCount, hasReport: !!rep, restart: view.restart, canRestart: view.canRestart,
  })
  const tabs = <Tabs value={tab} onChange={(v) => setParams(v === 'check' ? {} : { tab: v }, { replace: true })} items={TABS} label="Repair" />
  if (tab === 'files') {
    return (
      <>
        <PageHead title="Repair" desc="Fix what the server knows about a film or show by hand, or let it find and fix problems for you." />
        {tabs}
        <Suspense fallback={<Spinner />}><MetadataFolder /></Suspense>
      </>
    )
  }
  return (
    <>
      <PageHead title="Repair" desc="Finds and fixes problems with your metadata, the database, modules and this install, and works from the command line too if the server won’t start. It only looks until you press a fix, and a safety copy is made first." />
      {tabs}
      <SpecPage embedded page={topPage} handled={new Set(['check', 'allsafe', 'restart'])} onAction={(id) => { if (id === 'check') void start(); else if (id === 'allsafe') void fix({ allSafe: true }).catch((e) => setErr(message(e))); else if (id === 'restart') void restart() }}
        after={{ [topPage.sections.findIndex((x) => x.h === 'Check this server')]: <>
          {running && <progress className="dgn-bar" max={1} value={view.fraction} aria-label="How far along it is" />}
          {err && <div className="error" role="alert">{err}</div>}
          {rep && !running && <p ref={summary} className="rp-summary" tabIndex={-1} role="status">{countsLine(rep)} {rep.fixable > 0 ? `${rep.fixable} can be fixed from here.` : ''}</p>}
        </> }} />
      {view.results.length > 0 && <SpecPage embedded page={repairResultsSpec(view.results, copy)} />}
      {rep && AREAS.map((a) => {
        const list = rep.issues.filter((i) => i.area === a.id)
        if (!list.length) return null
        return (
          <Section key={a.id} title={a.title} desc={a.desc}>
            <div className="dgn-cards">
              {list.map((i) => <IssueCard key={i.key} issue={i} view={view} busy={running} onFix={fix} onMoved={moved} />)}
            </div>
          </Section>
        )
      })}
      {rep && rep.issues.length === 0 && !running && <div className="set-card set-empty"><b>Nothing to report</b><p>{rep.checks.length} checks looked at this server and found nothing wrong.</p></div>}
      <div ref={remapBox} className="rp-anchor"><Remap from={remapFrom} setFrom={setRemapFrom} twoFactor={view.twoFactor} onDone={() => void start()} /></div>
      {view.copies.length > 0 && <SpecPage embedded page={copiesSpec(view.copies)} />}
    </>
  )
}

function IssueCard({ issue: i, view, busy, onFix, onMoved }: { issue: Issue; view: View; busy: boolean; onFix: (b: { keys: string[]; option?: string; password?: string; code?: string }) => Promise<void>; onMoved: (from: string) => void }) {
  const [asking, setAsking] = useState<{ option?: string } | null>(null)
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [err, setErr] = useState('')
  const fixable = i.safety !== 'manual' && !i.notChecked
  const cancelAsk = () => { setAsking(null); setErr(''); setPassword(''); setCode('') }
  const confirm =(e: React.FormEvent) => {
    e.preventDefault(); setErr('')
    onFix({ keys: [i.key], option: asking?.option, password, code }).catch((x) => setErr(message(x)))
  }
  const press = (option?: string) => {
    // A safe fix can't lose anything, so it just happens; anything else asks who's asking.
    if (i.safety === 'safe' && !option) { onFix({ keys: [i.key] }).catch((x) => setErr(message(x))); return }
    setAsking({ option })
  }
  const badge = i.notChecked ? <span className="rp-safety">Not checked</span> : <span className={'rp-safety ' + i.safety}>{SAFETY_WORDS[i.safety]}</span>
  return (
    <FindingCard f={i} fixLabel={i.safety === 'manual' ? 'What to do' : 'What the fix does'} badge={badge}>
      {!!i.steps?.length && <ol className="rp-steps">{i.steps.map((s, n) => <li key={n}>{s}</li>)}</ol>}
      {err && !asking && <div className="error" role="alert">{err}</div>}
      {i.exclusive && fixable && <p className="rp-when">This finishes the next time the server starts.</p>}
      {(fixable || i.input === 'remap') && !asking && (
        <div className="rp-actions">
          {fixable && <button type="button" className="btn" disabled={busy} onClick={() => press()}>Fix this</button>}
          {i.options?.map((o) => <button key={o.id} type="button" className="btn ghost" disabled={busy} title={o.what} onClick={() => press(o.id)}>{o.label}</button>)}
          {i.input === 'remap' && <button type="button" className="btn" onClick={() => onMoved(i.details?.[0] || '')}>Tell it where the folder went</button>}
        </div>
      )}
      {asking && (
        <form className="form full rp-confirm" onSubmit={confirm} onKeyDown={(e) => { if (e.key === 'Escape') { e.stopPropagation(); cancelAsk() } }}>
          <p>{asking.option ? i.options?.find((o) => o.id === asking.option)?.what : 'This changes data, so it asks for your password. A safety copy is made first.'}</p>
          <label className="oobe-field">Your password
            <input className="input" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} autoFocus />
          </label>
          {view.twoFactor && (
            <label className="oobe-field">Code from your authenticator app
              <input className="input" inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} />
            </label>
          )}
          {err && <div className="error" role="alert">{err}</div>}
          <div className="actions">
            <button type="button" className="btn ghost" onClick={cancelAsk}>Cancel</button>
            <button className="btn primary" disabled={busy || !password || (view.twoFactor && !code)}>Go ahead</button>
          </div>
        </form>
      )}
    </FindingCard>
  )
}

function Remap({ from, setFrom, twoFactor, onDone }: { from: string; setFrom: (s: string) => void; twoFactor: boolean; onDone: () => void }) {
  const [to, setTo] = useState('')
  const [plan, setPlan] = useState<RemapPlan | null>(null)
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  useEffect(() => { setPlan(null); setErr('') }, [from, to])
  const look = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true); setErr('')
    try { setPlan(await api.post<RemapPlan>('/repair/remap', { from, to })) } catch (x) { setErr(message(x)) } finally { setBusy(false) }
  }
  const apply = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true); setErr('')
    try {
      const p = await api.post<RemapPlan>('/repair/remap', { from, to, apply: true, password, code })
      setPlan(p); setPassword(''); setCode('')
      if (p.applied) { toast('The folder was moved. Scan the library to pick up anything new.'); onDone() }
    } catch (x) { setErr(message(x)) } finally { setBusy(false) }
  }
  const onField = (id: string, v: string | number | boolean | File) => { const t = String(v); if (id === 'from') setFrom(t); else if (id === 'to') setTo(t); else if (id === 'password') setPassword(t); else if (id === 'code') setCode(t) }
  const onAction = (id: string) => { if (id === 'look') void look({ preventDefault() {} } as React.FormEvent); else if (id === 'move') void apply({ preventDefault() {} } as React.FormEvent) }
  return (
    <SpecPage embedded handled={new Set(['look', 'move'])} onAction={onAction} onField={onField}
      page={remapSpec({ from, to, busy, twoFactor, password, code, error: err, plan })} />
  )
}
