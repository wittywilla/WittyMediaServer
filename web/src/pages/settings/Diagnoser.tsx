// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { fmtDateTime, api } from '../../api'
import DownloadButton from '../../components/DownloadButton'
import { Spinner } from '../../components/common'
import { IconDisc, IconDrive, IconFile, IconFolder, IconUp } from '../../icons'
import { toast } from '../../state'
import FindingCard, { SEVERITY_WORDS as WORDS, type Finding, type Severity } from './FindingCard'
import { PageHead, Row, Section, Switch } from './ui'
import SpecPage from '../../spec/SpecPage'
import { discSpec, pastSpec, storageSpec } from '../../spec/specs/diagnoser'
import './diagnoser.css'
import { Tabs } from '../../ui/Choice'

// WittyDiagnoser, the disk diagnoser: checks a DVD or Blu-ray, or the folders the
// server uses, and says in plain words what it found. It only reads, and the
// server does the work, so this page can be left and come back to. Discs are
// picked with the page's own browser, which shows only what the server allows.

type Job = {
  id: string; kind: 'disc' | 'storage'; state: 'queued' | 'running' | 'done' | 'cancelled'
  subject: string; label?: string; queued: number; started?: number; finished?: number
  worst?: Severity; counts?: Partial<Record<Severity, number>>; findings?: Finding[]
  phase?: string; fraction: number; position?: number; saved: boolean
}
type Folder = { id: string; name: string; path: string; role: string }
type Targets = { libraries: { id: number; name: string; type: string; folders: Folder[] }[]; data: Folder[]; discs: { id: number; title: string; kind: string; path: string; library: number }[] }
type Entry = { name: string; path: string; dir: boolean; disc?: string; kind?: string; size?: number }
type Listing = { path: string; parent: string | null; roots?: { name: string; path: string; kind: string }[]; entries: Entry[]; truncated?: boolean }

const POLL_MS = 1500
const active = (j?: Job | null) => !!j && (j.state === 'queued' || j.state === 'running')
const gb = (n = 0) => (n >= 1 << 30 ? `${(n / (1 << 30)).toFixed(1)} GB` : `${Math.max(1, Math.round(n / (1 << 20)))} MB`)

function countsLine(c: Job['counts']) {
  const n = (k: Severity) => c?.[k] || 0
  const part = (k: Severity, one: string, many: string) => (n(k) ? `${n(k)} ${n(k) === 1 ? one : many}` : '')
  return [part('problem', 'problem', 'problems'), part('warn', 'warning', 'warnings'), part('info', 'note', 'notes'), part('ok', 'thing fine', 'things fine')].filter(Boolean).join(', ') || 'Nothing to report'
}

export default function Diagnoser() {
  const [tab, setTab] = useState<'disc' | 'storage'>('disc')
  const [jobs, setJobs] = useState<Job[] | null>(null)
  const [jobsErr, setJobsErr] = useState('')
  const [current, setCurrent] = useState('')
  const panel = useRef<HTMLDivElement>(null)

  const loadJobs = useCallback(() => api.get<{ jobs: Job[] }>('/diagnose/jobs').then((r) => { setJobs(r.jobs); setJobsErr(''); return r.jobs }).catch((e) => { setJobsErr(e.message); return [] as Job[] }), [])
  // Back on the page while a check runs: pick it up again.
  useEffect(() => { void loadJobs().then((l) => { const run = l.find(active); if (run) setCurrent(run.id) }) }, [loadJobs])

  const started = (j: Job) => { setCurrent(j.id); void loadJobs(); requestAnimationFrame(() => panel.current?.scrollIntoView({ block: 'nearest' })) }
  const open = (id: string) => { setCurrent(id); requestAnimationFrame(() => panel.current?.scrollIntoView({ block: 'start' })) }

  return (
    <>
      <PageHead title="WittyDiagnoser" desc={<>The disk diagnoser: checks your discs and your drives, and changes nothing. Pick a DVD, Blu-ray or CD image, or the folders the server uses, and it says what it finds in plain words. It never gets around copy protection. To fix the server’s own data, use <Link to="/settings/repair">Repair</Link>.</>} />
      <Tabs label="What to check" value={tab} onChange={setTab} panelId="dgn-panel" items={[{ value: 'disc', label: 'A disc' }, { value: 'storage', label: 'Storage' }]} />
      <div id="dgn-panel" role="tabpanel" aria-label={tab === 'disc' ? 'A disc' : 'Storage'}>
        {tab === 'disc' ? <DiscChoice onStarted={started} /> : <StorageChoice onStarted={started} />}
      </div>
      <div ref={panel} className="dgn-anchor">
        {current && <JobPanel id={current} onChanged={loadJobs} />}
      </div>
      <PastReports jobs={jobs} err={jobsErr} current={current} onOpen={open} onRetry={loadJobs} />
    </>
  )
}

// ---------------------------------------------------------------- a disc

function DiscChoice({ onStarted }: { onStarted: (j: Job) => void }) {
  const [targets, setTargets] = useState<Targets | null>(null)
  const [path, setPath] = useState('')
  const [browsing, setBrowsing] = useState(false)
  const [crawl, setCrawl] = useState(true)
  const [damage, setDamage] = useState(false)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  useEffect(() => { api.get<Targets>('/diagnose/targets').then(setTargets).catch(() => {}) }, [])
  const pick = (p: string) => { setPath(p); setBrowsing(false); setErr('') }
  const start = async () => {
    setBusy(true); setErr('')
    try { onStarted(await api.post<Job>('/diagnose/disc', { path, crawl, damage })) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const known = targets?.discs || []
  const page = discSpec({ path, knownDiscs: known.map((d) => ({ title: d.title, path: d.path })), crawl, damage, busy, error: err, browsing })
  return (
    <SpecPage embedded page={page} handled={new Set(['browse', 'start'])}
      onAction={(id) => { if (id === 'browse') setBrowsing((b) => !b); else if (id === 'start') void start() }}
      onField={(id, v) => { if (id === 'known') { if (v) pick(String(v)) } else if (id === 'crawl') setCrawl(!!v); else if (id === 'damage') setDamage(!!v) }}
      after={{ 0: browsing ? <DiscBrowser onPick={pick} /> : null }} />
  )
}

// The server's own view of its folders, limited to the places a check may look.
function DiscBrowser({ onPick }: { onPick: (path: string) => void }) {
  const [at, setAt] = useState('')
  const [list, setList] = useState<Listing | null>(null)
  const [err, setErr] = useState('')
  const ul = useRef<HTMLUListElement>(null)
  const focus = useRef(false)
  useEffect(() => {
    let gone = false
    setList(null); setErr('')
    api.get<Listing>('/diagnose/browse' + (at ? `?path=${encodeURIComponent(at)}` : ''))
      .then((l) => { if (!gone) setList(l) })
      .catch((e) => { if (!gone) setErr(e.message) })
    return () => { gone = true }
  }, [at])
  // Moving between folders keeps the keyboard or the remote in the list.
  useEffect(() => { if (list && focus.current) { focus.current = false; ul.current?.querySelector('button')?.focus() } }, [list])
  const go = (p: string) => { focus.current = true; setAt(p) }
  const upTo = list?.parent ?? ''
  const showRoots = !at && list?.roots
  return (
    <div className="dgn-browser">
      <div className="dgn-browser-head">
        {at && <button type="button" className="btn sm" onClick={() => go(upTo)}><IconUp /> {list?.parent ? 'Up' : 'All places'}</button>}
        <span className="mono dgn-where">{at || 'Your libraries and the data folder'}</span>
      </div>
      {err && <div className="error dgn-err" role="alert">{err} {at && <button type="button" className="btn sm" onClick={() => go('')}>Back to the start</button>}</div>}
      {!err && !list && <div className="dgn-browser-empty"><Spinner /></div>}
      {list && (
        <ul className="dgn-entries" ref={ul}>
          {showRoots && list.roots!.map((r) => (
            <li key={r.path}><button type="button" className="dgn-entry" onClick={() => go(r.path)}><IconDrive size={18} /><span className="dgn-entry-name">{r.name}</span><small>{r.kind === 'data' ? 'Data folder' : 'Library'}</small></button></li>
          ))}
          {!showRoots && list.entries.map((e) => {
            const disc = e.dir ? e.disc : e.kind === 'disc' ? 'image' : ''
            return (
              <li key={e.path}>
                <button type="button" className={'dgn-entry' + (disc ? ' disc' : '')} onClick={() => (disc ? onPick(e.path) : go(e.path))}>
                  {disc ? <IconDisc size={18} /> : e.dir ? <IconFolder size={18} /> : <IconFile size={18} />}
                  <span className="dgn-entry-name">{e.name}</span>
                  <small>{e.disc === 'dvd' ? 'DVD folder: choose' : e.disc === 'bluray' ? 'Blu-ray folder: choose' : e.kind === 'disc' ? `${gb(e.size)}: choose` : ''}</small>
                </button>
              </li>
            )
          })}
          {!showRoots && list.entries.length === 0 && <li className="dgn-browser-empty">No folders or disc images here.</li>}
          {showRoots && !list.roots!.length && <li className="dgn-browser-empty">Add a library first, under Libraries.</li>}
        </ul>
      )}
      {list?.truncated && <p className="set-row-desc dgn-trunc">This folder holds too much to list in full.</p>}
    </div>
  )
}

// ---------------------------------------------------------------- storage

function StorageChoice({ onStarted }: { onStarted: (j: Job) => void }) {
  const [targets, setTargets] = useState<Targets | null>(null)
  const [err, setErr] = useState('')
  const [chosen, setChosen] = useState<Set<string>>(new Set())
  const [readTest, setReadTest] = useState(true)
  const [writeTest, setWriteTest] = useState(false)
  const [walk, setWalk] = useState(true)
  const [busy, setBusy] = useState(false)
  useEffect(() => { api.get<Targets>('/diagnose/targets').then(setTargets).catch((e) => setErr(e.message)) }, [])
  const all = targets ? [...targets.libraries.flatMap((l) => l.folders), ...targets.data] : []
  const flip = (id: string) => setChosen((s) => { const n = new Set(s); if (n.has(id)) n.delete(id); else n.add(id); return n })
  const start = async () => {
    setBusy(true); setErr('')
    try { onStarted(await api.post<Job>('/diagnose/storage', { targets: [...chosen], readTest, writeTest, walk })) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const folders = targets ? [...targets.libraries.flatMap((l) => l.folders).map((f) => ({ ...f, group: 'library' as const })), ...targets.data.map((f) => ({ ...f, group: 'data' as const }))] : []
  if (!targets) return err ? <div className="error dgn-err" role="alert">{err}</div> : <div className="dgn-browser-empty"><Spinner /></div>
  return (
    <SpecPage embedded page={storageSpec({ folders, chosen, walk, readTest, writeTest, busy, error: err })}
      handled={new Set([...all.map((f) => 'dir:' + f.id), 'start', 'all'])}
      onAction={(id) => {
        if (id === 'start') void start()
        else if (id === 'all') setChosen(chosen.size === all.length ? new Set() : new Set(all.map((f) => f.id)))
        else if (id.startsWith('dir:')) flip(id.slice(4))
      }}
      onField={(id, v) => { if (id === 'walk') setWalk(!!v); else if (id === 'read') setReadTest(!!v); else if (id === 'write') setWriteTest(!!v) }} />
  )
}

// ---------------------------------------------------------------- one check

function JobPanel({ id, onChanged }: { id: string; onChanged: () => void }) {
  const [job, setJob] = useState<Job | null>(null)
  const [err, setErr] = useState('')
  const [hide, setHide] = useState(false)
  const [stopping, setStopping] = useState(false)
  const wasActive = useRef(false)
  const load = useCallback(() => api.get<Job>(`/diagnose/jobs/${encodeURIComponent(id)}`).then((j) => {
    setJob(j); setErr('')
    if (wasActive.current && !active(j)) onChanged() // it has just finished: it's in the list now
    wasActive.current = active(j)
  }).catch((e) => setErr(e.message)), [id, onChanged])
  useEffect(() => { setJob(null); setStopping(false); wasActive.current = false; void load() }, [load])

  // Asks only while the check runs and someone can see the answer.
  const running = active(job)
  useEffect(() => {
    if (!running) return
    const tick = () => { if (document.visibilityState === 'visible') void load() }
    const t = setInterval(tick, POLL_MS)
    document.addEventListener('visibilitychange', tick)
    return () => { clearInterval(t); document.removeEventListener('visibilitychange', tick) }
  }, [running, load])

  const cancel = async () => {
    setStopping(true)
    try { setJob(await api.del<Job>(`/diagnose/jobs/${encodeURIComponent(id)}`)) } catch (e: any) { toast(e.message); setStopping(false) }
  }
  if (err && !job) return <Section title="This check"><div className="error" role="alert">{err}</div></Section>
  if (!job) return <div className="dgn-browser-empty"><Spinner /></div>
  const when = job.finished ? fmtDateTime(job.finished) : ''
  // Spoken when it changes: the words, never each percent.
  const said = job.state === 'queued' ? `Waiting for another check to finish${job.position ? ` (number ${job.position} in line)` : ''}.`
    : job.state === 'running' ? (stopping ? 'Stopping…' : job.phase || 'Working…')
    : job.state === 'cancelled' ? `Stopped. ${countsLine(job.counts)}.`
    : `Finished. ${countsLine(job.counts)}.`
  const report = (format: string) => `/diagnose/jobs/${encodeURIComponent(id)}/report?format=${format}${hide ? '&redact=1' : ''}`
  return (
    <Section title={job.subject + (job.kind === 'storage' ? ' checked' : '')} desc={job.label && !running ? `Label on the disc: ${job.label}` : when ? `Checked ${when}` : undefined}
      actions={running && <button type="button" className="btn sm" onClick={cancel} disabled={stopping}>Cancel</button>}>
      <p className="dgn-phase" role="status" aria-live="polite">{said}</p>
      {running && <progress className="dgn-bar" max={1} value={job.fraction} aria-label="How far along the check is" />}
      {job.state === 'cancelled' && <p className="set-row-desc">You stopped this check, so this is only what it found up to then.</p>}
      <div className="dgn-cards">
        {(job.findings || []).map((f, i) => <FindingCard key={i} f={f} />)}
      </div>
      {job.saved && (
        <div className="dgn-download">
          <Row label="Hide disc name and file names" desc="For a copy to send to someone. Reports never hold full paths or anything from the disc itself.">
            <Switch checked={hide} onChange={setHide} label="Hide disc name and file names" />
          </Row>
          <div className="dgn-download-btns">
            <DownloadButton className="btn" path={report('txt')} name={`${id}.txt`}>Download report</DownloadButton>
            <DownloadButton className="btn ghost" path={report('json')} name={`${id}.json`}>As JSON</DownloadButton>
          </div>
        </div>
      )}
    </Section>
  )
}

// ---------------------------------------------------------------- past reports

function PastReports({ jobs, err, current, onOpen, onRetry }: { jobs: Job[] | null; err: string; current: string; onOpen: (id: string) => void; onRetry: () => void }) {
  if (err) return <div className="error dgn-err" role="alert">{err} <button type="button" className="btn sm" onClick={onRetry}>Try again</button></div>
  if (!jobs) return <div className="dgn-browser-empty"><Spinner /></div>
  const list = jobs.map((j) => ({ id: j.id, kind: j.kind, subject: j.subject, running: active(j), state: j.state, worst: j.worst, finished: j.finished ? fmtDateTime(j.finished) : undefined }))
  return <SpecPage embedded page={pastSpec(list, current, WORDS)} handled={new Set(jobs.map((j) => 'job:' + j.id))} onAction={(id) => onOpen(id.slice(4))} />
}
