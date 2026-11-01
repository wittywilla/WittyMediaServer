import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, fmtDateTime, setToken } from '../../api'
import { Spinner } from '../../components/common'
import { toast } from '../../state'
import { Notice, Tabs } from '../../ui'
import FindingCard, { type Finding } from './FindingCard'
import { PageHead, Section } from './ui'
import './repair.css'

// Repair: the same doctor as `wms doctor`, for someone who'd rather not open a terminal. It
// looks first and says what it found and what each fix does, and nothing changes until
// someone presses a fix. A safety copy is made first. The server does the work, so this page
// can be left and come back to.

type Area = 'data' | 'metadata' | 'modules' | 'install'
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
      {view.lastRepair && (
        <div className="rp-note">
          {view.lastRepair.ok
            ? <Notice tone="ok" title="The repaired database is in use">It took over on {fmtDateTime(new Date(view.lastRepair.at).getTime())}. The old file is kept in <code>{view.lastRepair.kept}</code>.</Notice>
            : <Notice tone="danger" title="The repaired database couldn’t take over">{view.lastRepair.error} The old database is still in use. Run the check again.</Notice>}
        </div>
      )}
      <Section title="Check this server" desc={<>Your media files are never changed or removed. To check a disc or a drive’s speed and health, use <Link to="/settings/diagnoser">WittyDiagnoser</Link>.</>}>
        <div className="rp-start">
          <button type="button" className={'btn big' + (safeCount > 0 ? '' : ' primary')} onClick={start} disabled={running}>{view.running === 'check' ? 'Checking…' : rep ? 'Check again' : 'Check this server'}</button>
          {rep && !running && <span className="rp-when">Last checked {fmtDateTime(rep.at)}</span>}
        </div>
        {running && <progress className="dgn-bar" max={1} value={view.fraction} aria-label="How far along it is" />}
        <p className="dgn-phase" role="status" aria-live="polite">{running ? view.phase || 'Working…' : ''}</p>
        {err && <div className="error" role="alert">{err}</div>}
        {view.error && <div className="error" role="alert">{view.error}</div>}
        {rep && !running && (
          <>
            <p ref={summary} className="rp-summary" tabIndex={-1} role="status">{countsLine(rep)} {rep.fixable > 0 ? `${rep.fixable} can be fixed from here.` : ''}</p>
            {safeCount > 0 && (
              <div className="rp-allsafe">
                <button type="button" className="btn primary" onClick={() => void fix({ allSafe: true }).catch((e) => setErr(message(e)))}>Fix everything that’s safe ({safeCount})</button>
                <span className="rp-when">These can’t lose anything. Fixes that change data ask you first, one at a time.</span>
              </div>
            )}
          </>
        )}
      </Section>
      {view.restart && (
        <div className="rp-note">
          <Notice tone="warn" title="A restart finishes the repair" action={<button type="button" className="btn" onClick={restart}>Restart now</button>}>
            The new database takes over when the server next starts, and anyone watching is interrupted. {view.canRestart ? '' : 'This server can’t restart itself: stop it and start it again.'}
          </Notice>
        </div>
      )}
      {view.results.length > 0 && <Results results={view.results} copy={copy} />}
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
      {view.copies.length > 0 && (
        <Section title="Safety copies" desc="Made before a repair. The newest five are kept, in the backups folder inside the data folder. What a repair removed is in each one’s removed folder.">
          <ul className="rp-copies">{view.copies.map((c) => <li key={c} className="mono">{c}</li>)}</ul>
        </Section>
      )}
    </>
  )
}

function Results({ results, copy }: { results: FixResult[]; copy?: string }) {
  return (
    <Section title="What was done">
      <ul className="rp-results" role="status">
        {results.map((r) => (
          <li key={r.key} className={r.ok ? 'ok' : 'bad'}>
            <b>{r.ok ? 'Fixed' : 'Not fixed'}: {r.title}</b>
            <span>{r.ok ? r.done : r.error}</span>
          </li>
        ))}
      </ul>
      {copy && <p className="rp-copyline">Safety copy: <code>{copy}</code></p>}
    </Section>
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
  return (
    <Section title="Moved a drive?" desc="If a library’s folder is now somewhere else, this changes where the server looks, and everything it knows about those titles stays: watch progress, matches and your edits. Your files aren’t touched.">
      <form className="form full" onSubmit={look}>
        <label className="oobe-field">Where it was
          <input className="input mono" value={from} onChange={(e) => setFrom(e.target.value)} placeholder="D:\Movies" spellCheck={false} autoComplete="off" />
        </label>
        <label className="oobe-field">Where it is now
          <input className="input mono" value={to} onChange={(e) => setTo(e.target.value)} placeholder="E:\Movies" spellCheck={false} autoComplete="off" />
        </label>
        {err && !plan && <div className="error" role="alert">{err}</div>}
        <div className="actions"><button className="btn" disabled={busy || !from.trim() || !to.trim()}>See what would change</button></div>
      </form>
      {plan && (
        <div className="rp-plan" role="status">
          {plan.why && <div className="error">{plan.why}</div>}
          {plan.applied && <p className="set-note ok">Done. Safety copy: <code>{plan.copy}</code></p>}
          {!plan.applied && plan.roots > 0 && <p>{plan.roots === 1 ? 'One library folder changes' : `${plan.roots} library folders change`}, and about {plan.rows.toLocaleString()} values in all. {plan.checked > 0 && `It looked for ${plan.checked.toLocaleString()} files, and ${plan.found.toLocaleString()} are in the new place.`}</p>}
          {(plan.sample.length > 0 || plan.notFound.length > 0) && !plan.applied && (
            <details>
              <summary>Some of the files</summary>
              <ul className="rp-paths">
                {plan.sample.map((s) => <li key={s}>Found: <span className="mono">{s}</span></li>)}
                {plan.notFound.map((s) => <li key={s}>Not found: <span className="mono">{s}</span></li>)}
              </ul>
            </details>
          )}
          {plan.canApply && !plan.applied && (
            <form className="form full" onSubmit={apply}>
              <label className="oobe-field">Your password
                <input className="input" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
              </label>
              {twoFactor && (
                <label className="oobe-field">Code from your authenticator app
                  <input className="input" inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} />
                </label>
              )}
              {err && <div className="error" role="alert">{err}</div>}
              <div className="actions"><button className="btn primary" disabled={busy || !password || (twoFactor && !code)}>Move it</button></div>
            </form>
          )}
        </div>
      )}
    </Section>
  )
}
