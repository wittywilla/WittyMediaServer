// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { fmtDateTime, getToken, serverBase } from '../../../api'
import { copyText } from '../../../gallery'
import { IconCheck, IconClose, IconInfo, IconPlug, IconShield } from '../../../icons'
import { toast } from '../../../state'
import { Button, Dialog, LinkButton, Notice, Progress } from '../../../ui'
import { ConfirmOwner } from '../ConfirmOwner'
import {
  anyBusy, changelogBlocks, dayOf, dismissRun, failureOf, mb, moduleLog, NOTHING_NOTABLE, PHASE, showsRun, startUpdateAll, storeView, undoRun,
  type Failure, type ModuleLog, type Run, type RunItem, type SandboxInfo, type StoreItem, type StoreView,
} from './storeApi'
import '../modules.css'
import './store.css'
import { OverrideTags } from './overrideParts'
import { itemReleaseStatus } from './overrides'

// ------------------------------------------------------------------- staying up to date

// useStoreView is the store's data. It asks again only while something is being changed, so a
// page left open costs nothing.
export function useStoreView() {
  const [view, setView] = useState<StoreView | null>(null)
  const [error, setError] = useState<Failure | null>(null)
  const load = useCallback(async () => {
    try { const v = await storeView(); setView(v); setError(null); return v } catch (e) { setError(failureOf(e, 'The store')); return null }
  }, [])
  useEffect(() => { void load() }, [load])
  const busy = anyBusy(view)
  useEffect(() => {
    if (!busy) return
    const t = setInterval(() => { void load() }, 1000)
    return () => clearInterval(t)
  }, [busy, load])
  return { view, setView, error, reload: load, busy }
}

// ------------------------------------------------------------------- labels

// A picture from the store's pages, which are for admins. It's asked for with a header, as an
// address with a token in it ends up in logs. Empty until it has come, and if it never does.
export function usePicture(src?: string) {
  const [url, setUrl] = useState('')
  useEffect(() => {
    setUrl('')
    if (!src) return
    let gone = false, made = ''
    const tok = getToken()
    fetch(serverBase() + src, tok ? { headers: { Authorization: 'Bearer ' + tok } } : undefined)
      .then((r) => (r.ok ? r.blob() : null))
      .then((b) => { if (b && !gone) { made = URL.createObjectURL(b); setUrl(made) } })
      .catch(() => {})
    return () => { gone = true; if (made) URL.revokeObjectURL(made) }
  }, [src])
  return url
}

// A module's picture: its own icon, served by this server, or one drawn from its name. Either
// way it decorates, as the name is always beside it.
export function ModuleIcon({ item, size = 56 }: { item: Pick<StoreItem, 'icon' | 'name' | 'official'>; size?: number }) {
  const [broken, setBroken] = useState(false)
  const url = usePicture(item.icon)
  if (url && !broken) return <img className="st-icon" src={url} alt="" width={size} height={size} onError={() => setBroken(true)} />
  const letter = (item.name.replace(/^Witty/, '') || item.name).charAt(0).toUpperCase()
  return <span className="st-icon st-icon-drawn" aria-hidden="true" style={{ width: size, height: size, fontSize: size * 0.46 }}>{letter || <IconPlug />}</span>
}

export function Tag({ tone, icon, children }: { tone?: 'ok' | 'warn' | 'bad' | 'new' | 'off'; icon?: ReactNode; children: ReactNode }) {
  return <span className={'ms-tag' + (tone ? ' ' + tone : '')}>{icon}{children}</span>
}

// Says where it comes from: anything not from this project is labelled wherever it appears.
export function OriginTag({ item }: { item: StoreItem }) {
  if (item.official || item.kind === 'server') return null
  return <Tag tone="warn" icon={<IconShield size={12} />}>Third-party</Tag>
}

// Why it can't be installed, in a word or two. The whole reason is on its page.
function FitTag({ kind }: { kind: string }) {
  if (kind === 'no-version') return <Tag tone="off">Not available</Tag>
  if (kind === 'library') return <Tag tone="off">Comes with other modules</Tag>
  if (kind === 'no-build') return <Tag tone="off">Not for this computer</Tag>
  if (kind === 'withdrawn') return <Tag tone="bad" icon={<IconInfo size={12} />}>Withdrawn</Tag>
  return <Tag tone="bad" icon={<IconInfo size={12} />}>{kind === 'too-old' ? 'Built for an older server' : 'Needs a newer server'}</Tag>
}

// What state it's in, in words. Each state also has its own wording or icon, never only a colour.
export function StateTags({ item }: { item: StoreItem }) {
  const m = item.status
  return (
    <span className="ms-tags">
      <OriginTag item={item} />
      {item.kind === 'in-development' && <Tag tone="off">In development</Tag>}
      {item.kind === 'refused' && <Tag tone="bad" icon={<IconClose size={12} />}>Can’t be used</Tag>}
      {m && PHASE[m.phase] && <Tag tone="new">{PHASE[m.phase]}</Tag>}
      {m?.installed && !PHASE[m.phase] && m.enabled === false && <Tag tone="off">Turned off</Tag>}
      {m?.installed && !PHASE[m.phase] && m.enabled !== false && (item.failure ? <Tag tone="bad" icon={<IconInfo size={12} />}>Needs a look</Tag> : m.latest ? <Tag tone="new">Update available</Tag> : <Tag tone="ok" icon={<IconCheck size={12} />}>Installed</Tag>)}
      {!m?.installed && item.kind !== 'in-development' && item.kind !== 'refused' && !item.fit.ok && <FitTag kind={item.fit.kind} />}
      {m?.pin && <Tag>Pinned</Tag>}
      {item.official && m?.source && <Tag tone="warn" icon={<IconShield size={12} />}>Other publisher</Tag>}
      {m?.pending && <Tag tone="warn">Needs your go-ahead</Tag>}
      <OverrideTags override={item.override || m?.override} status={itemReleaseStatus(item)} />
    </span>
  )
}

// ------------------------------------------------------------------- what's new

// A version's changelog, as text. Nothing in it can become markup: it's only ever text nodes.
// With nothing in it but headings, such as the version's number, there's no news either.
export function ChangeNotes({ text }: { text?: string }) {
  const blocks = changelogBlocks(text || '')
  if (!blocks.some((b) => b.kind !== 'h')) return <p className="st-notes-none">{NOTHING_NOTABLE}</p>
  return (
    <div className="st-notes">
      {blocks.map((b, i) => b.kind === 'ul'
        ? <ul key={i}>{b.lines.map((l, j) => <li key={j}>{l}</li>)}</ul>
        : b.kind === 'h' ? <h4 key={i}>{b.lines[0]}</h4> : <p key={i}>{b.lines.join(' ')}</p>)}
    </div>
  )
}

// The latest news across what's installed, newest first, with an update's news first of all.
export function WhatsNew({ view, limit = 5 }: { view: StoreView; limit?: number }) {
  const installed = view.items.filter((i) => i.status?.installed && i.kind !== 'server')
  // An update is news even when it says nothing about itself.
  const withNews = installed.filter((i) => i.status?.latest || (i.changelog || '').trim())
  const order = (i: StoreItem) => (i.status?.latest ? 1 : 0)
  const shown = [...withNews].sort((a, b) => order(b) - order(a) || (b.released || '').localeCompare(a.released || '')).slice(0, limit)
  return (
    <section className="set-card st-news" aria-labelledby="st-news-h">
      <header className="set-card-head"><div><h2 id="st-news-h">What’s new</h2><p>The latest from your modules.</p></div></header>
      <div className="set-card-body">
        {shown.length === 0 && <div className="st-news-item"><b>Nothing major to report</b><p className="st-notes-none">{NOTHING_NOTABLE}</p></div>}
        {shown.map((i) => {
          const update = i.status?.latest
          return (
            <article key={i.key} className="st-news-item" aria-label={i.name}>
              <div className="st-news-head">
                <ModuleIcon item={i} size={32} />
                <Link to={itemPath(i)}><b>{i.name}</b></Link>
                <span className="ms-ver">{update ? update.version : i.version}</span>
                {update ? <Tag tone="new">Update available</Tag> : i.released && <span className="muted st-news-day">{dayOf(i.released)}</span>}
                <OriginTag item={i} />
              </div>
              <ChangeNotes text={update?.notes || i.changelog} />
            </article>
          )
        })}
      </div>
    </section>
  )
}

export const itemPath = (i: Pick<StoreItem, 'official' | 'store' | 'id' | 'kind'>) => i.official || i.kind === 'server' ? `/settings/modules/store/${i.id}` : `/settings/modules/store/${i.store}/${i.id}`

// ------------------------------------------------------------------- the policy

export function PolicyNote({ view }: { view: Pick<StoreView, 'policy' | 'server'> }) {
  return (
    <details className="ms-more st-policy">
      <summary>Which modules run on this server?</summary>
      <p>{view.policy}</p>
      <p className="muted">This server is version {view.server}.</p>
    </details>
  )
}

// What a module's helper is really held to here, whenever that's short of a full sandbox.
export function SandboxNote({ sandbox }: { sandbox?: SandboxInfo }) {
  if (!sandbox || sandbox.level === 'full') return null
  return <Notice tone={sandbox.level === 'partial' ? 'info' : 'warn'} title={sandbox.title}>{sandbox.words}</Notice>
}

// ------------------------------------------------------------------- when something fails

// The whole story of a failure: what happened, what to try, and everything behind it for anyone
// helping, including the module's own log lines, with a button to copy all of it.
export function FailureDialog({ failure, moduleId, name, onClose, repairRoute = '/settings/repair', onRetry }: { failure: Failure; moduleId?: string; name: string; onClose: () => void; repairRoute?: string; onRetry?: () => void }) {
  return (
    <Dialog title={failure.title} onClose={onClose} size="md" className="st-failure" description={failure.message}
      actions={<>
        <Button variant="ghost" onClick={onClose}>Close</Button>
        {onRetry && <Button variant="primary" onClick={() => { onClose(); onRetry() }}>Try again</Button>}
      </>}>
      <FailureBody failure={failure} moduleId={moduleId} name={name} repairRoute={repairRoute} />
    </Dialog>
  )
}

export function FailureBody({ failure, moduleId, name, repairRoute }: { failure: Failure; moduleId?: string; name: string; repairRoute: string }) {
  return (
    <div className="st-fail-body">
      {failure.try.length > 0 && <>
        <h3>What to try</h3>
        <ol>{failure.try.map((t, i) => <li key={i}>{t}</li>)}</ol>
      </>}
      <DetailsAndLog failure={failure} moduleId={moduleId} name={name} />
      <p className="st-fail-links">
        {failure.repair && <Link to={repairRoute}>Open the repair tools</Link>}
        {moduleId && <Link to={`/settings/logs?module=${encodeURIComponent(moduleId)}`}>Logging for {name}</Link>}
      </p>
    </div>
  )
}

// Show details: loaded when asked for, so a failure costs nothing until someone looks.
export function DetailsAndLog({ failure, moduleId, name, open }: { failure?: Failure; moduleId?: string; name: string; open?: boolean }) {
  const [log, setLog] = useState<ModuleLog | null>(null)
  const [state, setState] = useState<'idle' | 'loading' | 'failed'>('idle')
  const asked = useRef(false)
  const fetchLog = () => {
    if (!moduleId || asked.current) return
    asked.current = true
    setState('loading')
    moduleLog(moduleId).then((l) => { setLog(l); setState('idle') }).catch(() => setState('failed'))
  }
  useEffect(() => { if (open) fetchLog() }, [open]) // eslint-disable-line react-hooks/exhaustive-deps
  const text = [failure?.detail ? `What happened: ${failure.detail}` : '', log?.text || ''].filter(Boolean).join('\n\n')
  const copy = async () => { toast((await copyText(`${name}\n${text}`)) ? 'Details copied' : 'That couldn’t be copied here. Select the text instead.') }
  return (
    <details className="ms-more st-details" open={open} onToggle={(e) => { if ((e.target as HTMLDetailsElement).open) fetchLog() }}>
      <summary>Show details</summary>
      <div className="st-details-body">
        {failure?.detail && <pre className="st-log" tabIndex={0} aria-label="What happened">{failure.detail}</pre>}
        {moduleId && <h4>This module’s log</h4>}
        {state === 'loading' && <p className="muted" role="status">Reading the log…</p>}
        {state === 'failed' && <p className="muted">The log couldn’t be read. It’s also in Settings, WittyLog.</p>}
        {log && (log.lines.length
          ? <pre className="st-log" tabIndex={0} aria-label={`Log lines for ${name}`}>{log.text}</pre>
          : <p className="muted">It hasn’t logged anything lately.</p>)}
        <div className="ms-actions"><Button size="sm" onClick={copy} disabled={!text}>Copy details</Button></div>
      </div>
    </details>
  )
}

// A module's log, on its own.
export function ModuleLogDialog({ moduleId, name, onClose }: { moduleId: string; name: string; onClose: () => void }) {
  return (
    <Dialog title={`${name} log`} onClose={onClose} size="wide" description="What this module has said and done lately. Tokens and passwords are taken out before it reaches this page."
      actions={<><LinkButton variant="ghost" to={`/settings/logs?module=${encodeURIComponent(moduleId)}`}>Logging settings</LinkButton><Button variant="primary" onClick={onClose}>Close</Button></>}>
      <DetailsAndLog moduleId={moduleId} name={name} open />
    </Dialog>
  )
}

// A failure in place: a line that says what happened, and the button for the rest.
export function FailureNotice({ failure, name, moduleId, repairRoute, onRetry }: { failure: Failure; name: string; moduleId?: string; repairRoute: string; onRetry?: () => void }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <Notice tone={failure.kind === 'key-changed' ? 'danger' : 'warn'} title={failure.title} action={<Button size="sm" onClick={() => setOpen(true)}>What to do</Button>}>{failure.message}</Notice>
      {open && <FailureDialog failure={failure} name={name} moduleId={moduleId} repairRoute={repairRoute} onClose={() => setOpen(false)} onRetry={onRetry} />}
    </>
  )
}

// ------------------------------------------------------------------- update all and installs

const RUN_WORDS: Record<RunItem['state'], string> = { queued: 'Waiting', working: 'Working', done: 'Done', failed: 'Failed', skipped: 'Skipped', undone: 'Taken back' }

function RunLine({ it, onExplain }: { it: RunItem; onExplain: (it: RunItem) => void }) {
  const going = it.state === 'working'
  return (
    <li className={'st-run-line ' + it.state}>
      <span className="st-run-state" aria-hidden="true">{it.state === 'done' ? <IconCheck size={16} /> : it.state === 'failed' ? <IconClose size={16} /> : it.state === 'working' ? <span className="ui-btn-spin" /> : <span className="st-run-dot" />}</span>
      <span className="st-run-text">
        <b>{it.name}</b>
        {it.to && <span className="ms-ver">{it.from ? `${it.from} → ` : ''}{it.to}</span>}
        <span className="st-run-word">{going && it.phase ? PHASE[it.phase] || RUN_WORDS.working : RUN_WORDS[it.state]}{going && it.phase === 'downloading' && it.total ? `, ${mb(it.done)} of ${mb(it.total)}` : ''}</span>
        {it.why && it.state !== 'failed' && <span className="muted st-run-why">{it.why}</span>}
        {it.failure && it.state === 'failed' && <span className="st-run-why">{it.failure.title}</span>}
      </span>
      {it.state === 'failed' && it.failure && <Button size="sm" onClick={() => onExplain(it)} aria-label={`What happened to ${it.name}`}>What happened</Button>}
      {going && <div className="st-run-bar"><Progress label={`${it.name}: ${it.phase ? PHASE[it.phase] : 'working'}`} value={it.phase === 'downloading' && it.total ? it.done : undefined} max={it.total || undefined} /></div>}
    </li>
  )
}

function summaryWords(r: Run): string {
  const parts: string[] = []
  const what = r.kind === 'install' ? 'installed' : 'updated'
  if (r.undone) return 'The updates were taken back.'
  parts.push(`${r.updated} ${what}`)
  if (r.failed) parts.push(`${r.failed} failed`)
  if (r.skipped) parts.push(`${r.skipped} skipped`)
  return parts.join(', ') + '.'
}

// Everything about a run: who's waiting, who's going, how it went, and taking it back.
export function RunPanel({ run, onChanged, repairRoute }: { run: Run; onChanged: () => void; repairRoute: string }) {
  const [explain, setExplain] = useState<RunItem | null>(null)
  const [undoAsk, setUndoAsk] = useState(false)
  const headId = useId()
  const finished = !!run.finished
  const done = run.items.filter((i) => !['queued', 'working'].includes(i.state)).length
  const dismiss = async () => { try { await dismissRun() } finally { onChanged() } }
  return (
    <div className={'st-run' + (finished && run.failed ? ' has-failed' : '')} role="group" aria-labelledby={headId}>
      <div className="st-run-head">
        <h3 id={headId}>{finished ? (run.failed ? 'Finished, with problems' : 'All done') : run.kind === 'install' ? 'Installing' : 'Updating your modules'}</h3>
        <span role="status" className="st-run-count">{finished ? summaryWords(run) : `${done} of ${run.items.length}`}</span>
      </div>
      {!finished && <Progress label="Progress overall" value={done} max={run.items.length} />}
      <ul className="st-run-list">{run.items.map((it) => <RunLine key={it.id} it={it} onExplain={setExplain} />)}</ul>
      {finished && (
        <div className="ms-actions">
          {run.canUndo && <Button size="sm" onClick={() => setUndoAsk(true)}>Take these updates back</Button>}
          <Button size="sm" variant="ghost" onClick={dismiss}>Dismiss</Button>
          <span className="muted st-run-when">Finished {fmtDateTime(run.finished!)}</span>
        </div>
      )}
      {explain?.failure && <FailureDialog failure={explain.failure} name={explain.name} moduleId={explain.id} repairRoute={repairRoute} onClose={() => setExplain(null)} />}
      {undoAsk && <ConfirmOwner onClose={() => setUndoAsk(false)} ask={{
        title: 'Take these updates back?', action: 'Take them back',
        body: <p>These modules go back to the versions they were on. An older version may lack security fixes, so this asks for your password.</p>,
        run: async (password, code) => { await undoRun({ password, code }); toast('Those updates were taken back'); onChanged() },
      }} />}
    </div>
  )
}

// The button everyone looks for, with how many it would update, and the run once it starts.
export function UpdateAllPanel({ view, onChanged }: { view: StoreView; onChanged: () => void }) {
  const [starting, setStarting] = useState(false)
  const [failure, setFailure] = useState<Failure | null>(null)
  const running = !!view.run && !view.run.finished
  const n = view.updates
  // The button stays pressable-looking only while there's something to do, and is never truly
  // disabled, as that would drop the focus and leave a remote lost on the page.
  const idle = n === 0 || running
  const go = async () => {
    if (starting || idle) return
    setStarting(true)
    try { await startUpdateAll(); onChanged() } catch (e) { setFailure(failureOf(e, 'Update All')) } finally { setStarting(false) }
  }
  return (
    <div className="st-updateall">
      <div className="st-updateall-top">
        <div role="status">
          <b className="st-updateall-count">{running ? 'Updating…' : n > 1 ? `${n} updates available` : n === 1 ? '1 update available' : view.checkedAt ? 'Every module is up to date' : 'No updates known yet'}</b>
          {view.checkedAt ? <span className="muted"> Last checked {fmtDateTime(view.checkedAt)}.</span> : <span className="muted"> The server hasn’t looked yet.</span>}
        </div>
        <Button variant={n > 0 && !running ? 'primary' : 'default'} className={idle ? 'st-idle' : undefined} aria-busy={starting || running || undefined} aria-disabled={idle || undefined} onClick={go}>{n > 0 ? `Update All Modules (${n})` : 'Update All Modules'}</Button>
      </div>
      {showsRun(view.run) && <RunPanel run={view.run!} onChanged={onChanged} repairRoute={view.repairRoute} />}
      {failure && <FailureDialog failure={failure} name="Update All" repairRoute={view.repairRoute} onClose={() => setFailure(null)} />}
    </div>
  )
}

