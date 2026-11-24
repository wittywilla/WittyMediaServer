// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { getToken, serverBase } from '../../../api'
import { date, t, tk, tn, tx } from '../../../i18n'
import { copyText } from '../../../gallery'
import { IconCheck, IconClose, IconInfo, IconPlug, IconShield } from '../../../icons'
import { toast } from '../../../state'
import { Button, Dialog, LinkButton, Notice, Progress } from '../../../ui'
import { ConfirmOwner } from '../ConfirmOwner'
import {
  anyBusy, changelogBlocks, checkAll, dayOf, dismissRun, failureOf, mb, moduleLog, NOTHING_NOTABLE, PHASE, showsRun, startUpdateAll, storeView, undoRun,
  type Failure, type ModuleLog, type Run, type RunItem, type SandboxInfo, type StoreItem, type StoreView,
} from './storeApi'
import '../modules.css'
import './store.css'
import { OverrideTags } from './overrideParts'
import { itemReleaseStatus } from './overrides'
import { hasNews, splitNotes } from './changeNotes'
import { compat } from './compat'

// ------------------------------------------------------------------- staying up to date

// useStoreView is the store's data. It asks again only while something is being changed, so a
// page left open costs nothing.
export function useStoreView() {
  const [view, setView] = useState<StoreView | null>(null)
  const [error, setError] = useState<Failure | null>(null)
  const load = useCallback(async () => {
    try { const v = await storeView(); setView(v); setError(null); return v } catch (e) { setError(failureOf(e, t('The store'))); return null }
  }, [])
  useEffect(() => { void load() }, [load])
  const busy = anyBusy(view)
  useEffect(() => {
    if (!busy) return
    const t = setInterval(() => { void load() }, 1000)
    return () => clearInterval(t)
  }, [busy, load])
  // Check now, for every tab: asks the stores again and says how it went.
  const [checking, setChecking] = useState(false)
  const [checkFailure, setCheckFailure] = useState<Failure | null>(null)
  const check = useCallback(async () => {
    setChecking(true)
    try {
      const v = await checkAll()
      setView(v); setError(null)
      if (v.checkError || v.stores.some((s) => s.problem)) toast(t('The check didn’t all work. What happened is on the page.'))
      else toast(v.updates ? tn('{count} update found', '{count} updates found', v.updates) : t('Every module is up to date'))
    } catch (e) { setCheckFailure(failureOf(e, t('The check'))) } finally { setChecking(false) }
  }, [])
  return { view, setView, error, reload: load, busy, check, checking, checkFailure, clearCheckFailure: () => setCheckFailure(null) }
}
export type StoreViewState = ReturnType<typeof useStoreView>

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
  return <Tag tone="warn" icon={<IconShield size={12} />}>{t('Third-party')}</Tag>
}

// Why it can't be installed, in a word or two. The whole reason is on its page.
function FitTag({ item, server }: { item: StoreItem; server: string }) {
  const c = compat(item, server)
  return <Tag tone={c.tone === 'ok' ? 'ok' : c.tone} icon={c.tone === 'bad' || c.tone === 'warn' ? <IconInfo size={12} /> : undefined}>{c.tag}</Tag>
}

// What state it's in, in words. Each state also has its own wording or icon, never only a colour.
export function StateTags({ item, server = '' }: { item: StoreItem; server?: string }) {
  const m = item.status
  return (
    <span className="ms-tags">
      <OriginTag item={item} />
      {item.kind === 'in-development' && <Tag tone="off">{t('In development')}</Tag>}
      {item.kind === 'refused' && <Tag tone="bad" icon={<IconClose size={12} />}>{t('Can’t be used')}</Tag>}
      {m && PHASE[m.phase] && <Tag tone="new">{PHASE[m.phase]}</Tag>}
      {m?.installed && !PHASE[m.phase] && m.enabled === false && <Tag tone="off">{t('Turned off')}</Tag>}
      {m?.installed && !PHASE[m.phase] && m.enabled !== false && (item.failure ? <Tag tone="bad" icon={<IconInfo size={12} />}>{t('Needs a look')}</Tag> : m.latest ? <Tag tone="new">{t('Update available')}</Tag> : <Tag tone="ok" icon={<IconCheck size={12} />}>{t('Installed')}</Tag>)}
      {!m?.installed && item.kind !== 'in-development' && item.kind !== 'refused' && !item.fit.ok && <FitTag item={item} server={server} />}
      {m?.installed && m.olderServer && <FitTag item={item} server={server} />}
      {m?.installed && m.withdrawn && !m.olderServer && <FitTag item={item} server={server} />}
      {m?.pin && <Tag>{t('Pinned')}</Tag>}
      {item.official && m?.source && <Tag tone="warn" icon={<IconShield size={12} />}>{t('Other publisher')}</Tag>}
      {m?.pending && <Tag tone="warn">{t('Needs your go-ahead')}</Tag>}
      <OverrideTags override={item.override || m?.override} status={itemReleaseStatus(item)} />
    </span>
  )
}

// ------------------------------------------------------------------- what's new

// A version's changelog, as text. Nothing in it can become markup: it's only ever text nodes.
// With nothing in it but headings, such as the version's number, there's no news either.
// People see the plain part; file names, API paths and the like are developer notes (#549),
// folded away on a module's page (developer) and left out of the overview.
export function ChangeNotes({ text, developer }: { text?: string; developer?: boolean }) {
  const split = splitNotes(text)
  return (
    <>
      {hasNews(split) ? <NoteBlocks text={split.people} /> : <p className="st-notes-none">{tx(NOTHING_NOTABLE)}</p>}
      {developer && split.developer && (
        <details className="ms-more st-dev-notes">
          <summary>{t('Developer notes')}</summary>
          <NoteBlocks text={split.developer} />
        </details>
      )}
    </>
  )
}

function NoteBlocks({ text }: { text: string }) {
  const blocks = changelogBlocks(text)
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
  // An update is news even when it says nothing about itself. A release that only retook the
  // README's pictures, or has only developer notes, isn't (#553).
  const withNews = installed.filter((i) => i.status?.latest || hasNews(splitNotes(i.changelog)))
  const order = (i: StoreItem) => (i.status?.latest ? 1 : 0)
  const shown = [...withNews].sort((a, b) => order(b) - order(a) || (b.released || '').localeCompare(a.released || '')).slice(0, limit)
  return (
    <section className="set-card st-news" aria-labelledby="st-news-h">
      <header className="set-card-head"><div><h2 id="st-news-h">{t('What’s new')}</h2><p>{t('The latest from your modules.')}</p></div></header>
      <div className="set-card-body">
        {shown.length === 0 && <div className="st-news-item"><b>{t('Nothing major to report')}</b><p className="st-notes-none">{tx(NOTHING_NOTABLE)}</p></div>}
        {shown.map((i) => {
          const update = i.status?.latest
          return (
            <article key={i.key} className="st-news-item" aria-label={i.name}>
              <div className="st-news-head">
                <ModuleIcon item={i} size={32} />
                <Link to={itemPath(i)}><b>{i.name}</b></Link>
                <span className="ms-ver">{update ? update.version : i.version}</span>
                {update ? <Tag tone="new">{t('Update available')}</Tag> : i.released && <span className="muted st-news-day">{dayOf(i.released)}</span>}
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

// The Updates tab's list: each module with an update waiting, the version it goes to, and what that
// version brings, in its own words.
export function UpdateNotes({ view }: { view: StoreView }) {
  const list = view.items.filter((i) => i.status?.installed && i.status.latest)
  if (!list.length) return null
  return (
    <section className="set-card st-news st-update-notes" aria-labelledby="st-upd-h">
      <header className="set-card-head"><div><h2 id="st-upd-h">{t('What the updates bring')}</h2></div></header>
      <div className="set-card-body">
        {list.map((i) => (
          <article key={i.key} className="st-news-item" aria-label={i.name}>
            <div className="st-news-head">
              <ModuleIcon item={i} size={32} />
              <Link to={itemPath(i)}><b>{i.name}</b></Link>
              <span className="ms-ver">{t('{from} to {to}', { from: i.status!.version, to: i.status!.latest!.version })}</span>
              {i.status!.latest!.size ? <span className="muted">{mb(i.status!.latest!.size)}</span> : null}
              <OriginTag item={i} />
            </div>
            <ChangeNotes text={i.status!.latest!.notes} />
          </article>
        ))}
      </div>
    </section>
  )
}

export const itemPath = (i: Pick<StoreItem, 'official' | 'store' | 'id' | 'kind'>) => i.official || i.kind === 'server' ? `/settings/modules/store/${i.id}` : `/settings/modules/store/${i.store}/${i.id}`

// ------------------------------------------------------------------- the policy

export function PolicyNote({ view }: { view: Pick<StoreView, 'policy' | 'server'> }) {
  return (
    <details className="ms-more st-policy">
      <summary>{t('Which modules run on this server?')}</summary>
      <p>{view.policy}</p>
      <p className="muted">{t('This server is version {version}.', { version: view.server })}</p>
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
        <Button variant="ghost" onClick={onClose}>{t('Close')}</Button>
        {onRetry && <Button variant="primary" onClick={() => { onClose(); onRetry() }}>{t('Try again')}</Button>}
      </>}>
      <FailureBody failure={failure} moduleId={moduleId} name={name} repairRoute={repairRoute} />
    </Dialog>
  )
}

export function FailureBody({ failure, moduleId, name, repairRoute }: { failure: Failure; moduleId?: string; name: string; repairRoute: string }) {
  return (
    <div className="st-fail-body">
      {failure.try.length > 0 && <>
        <h3>{t('What to try')}</h3>
        <ol>{failure.try.map((t, i) => <li key={i}>{t}</li>)}</ol>
      </>}
      <DetailsAndLog failure={failure} moduleId={moduleId} name={name} />
      <p className="st-fail-links">
        {failure.repair && <Link to={repairRoute}>{t('Open the repair tools')}</Link>}
        {moduleId && <Link to={`/settings/logs?module=${encodeURIComponent(moduleId)}`}>{t('Logging for {name}', { name })}</Link>}
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
  const text = [failure?.detail ? t('What happened: {detail}', { detail: failure.detail }) : '', log?.text || ''].filter(Boolean).join('\n\n')
  const copy = async () => { toast((await copyText(`${name}\n${text}`)) ? t('Details copied') : t('That couldn’t be copied here. Select the text instead.')) }
  return (
    <details className="ms-more st-details" open={open} onToggle={(e) => { if ((e.target as HTMLDetailsElement).open) fetchLog() }}>
      <summary>{t('Show details')}</summary>
      <div className="st-details-body">
        {failure?.detail && <pre className="st-log" tabIndex={0} aria-label={t('What happened')}>{failure.detail}</pre>}
        {moduleId && <h4>{t('This module’s log')}</h4>}
        {state === 'loading' && <p className="muted" role="status">{t('Reading the log…')}</p>}
        {state === 'failed' && <p className="muted">{t('The log couldn’t be read. It’s also in Settings, WittyLog.')}</p>}
        {log && (log.lines.length
          ? <pre className="st-log" tabIndex={0} aria-label={t('Log lines for {name}', { name })}>{log.text}</pre>
          : <p className="muted">{t('It hasn’t logged anything lately.')}</p>)}
        <div className="ms-actions"><Button size="sm" onClick={copy} disabled={!text}>{t('Copy details')}</Button></div>
      </div>
    </details>
  )
}

// A module's log, on its own.
export function ModuleLogDialog({ moduleId, name, onClose }: { moduleId: string; name: string; onClose: () => void }) {
  return (
    <Dialog title={t('{name} log', { name })} onClose={onClose} size="wide" description={t('What this module has said and done lately. Tokens and passwords are taken out before it reaches this page.')}
      actions={<><LinkButton variant="ghost" to={`/settings/logs?module=${encodeURIComponent(moduleId)}`}>{t('Logging settings')}</LinkButton><Button variant="primary" onClick={onClose}>{t('Close')}</Button></>}>
      <DetailsAndLog moduleId={moduleId} name={name} open />
    </Dialog>
  )
}

// A failure in place: a line that says what happened, and the button for the rest.
export function FailureNotice({ failure, name, moduleId, repairRoute, onRetry }: { failure: Failure; name: string; moduleId?: string; repairRoute: string; onRetry?: () => void }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <Notice tone={failure.kind === 'key-changed' ? 'danger' : 'warn'} title={failure.title} action={<Button size="sm" onClick={() => setOpen(true)}>{t('What to do')}</Button>}>{failure.message}</Notice>
      {open && <FailureDialog failure={failure} name={name} moduleId={moduleId} repairRoute={repairRoute} onClose={() => setOpen(false)} onRetry={onRetry} />}
    </>
  )
}

// ------------------------------------------------------------------- update all and installs

const RUN_WORDS: Record<RunItem['state'], string> = { queued: tk('Waiting'), working: tk('Working'), done: tk('Done'), failed: tk('Failed'), skipped: tk('Skipped'), undone: tk('Taken back') }

function RunLine({ it, onExplain }: { it: RunItem; onExplain: (it: RunItem) => void }) {
  const going = it.state === 'working'
  return (
    <li className={'st-run-line ' + it.state}>
      <span className="st-run-state" aria-hidden="true">{it.state === 'done' ? <IconCheck size={16} /> : it.state === 'failed' ? <IconClose size={16} /> : it.state === 'working' ? <span className="ui-btn-spin" /> : <span className="st-run-dot" />}</span>
      <span className="st-run-text">
        <b>{it.name}</b>
        {it.to && <span className="ms-ver">{it.from ? `${it.from} → ` : ''}{it.to}</span>}
        <span className="st-run-word">{going && it.phase === 'downloading' && it.total
          ? t('{phase}, {done} of {total}', { phase: PHASE.downloading, done: mb(it.done), total: mb(it.total) })
          : going && it.phase ? PHASE[it.phase] || tx(RUN_WORDS.working) : tx(RUN_WORDS[it.state])}</span>
        {it.why && it.state !== 'failed' && <span className="muted st-run-why">{it.why}</span>}
        {it.failure && it.state === 'failed' && <span className="st-run-why">{it.failure.title}</span>}
      </span>
      {it.state === 'failed' && it.failure && <Button size="sm" onClick={() => onExplain(it)} aria-label={t('What happened to {name}', { name: it.name })}>{t('What happened')}</Button>}
      {going && <div className="st-run-bar"><Progress label={`${it.name}: ${(it.phase && PHASE[it.phase]) || tx(RUN_WORDS.working)}`} value={it.phase === 'downloading' && it.total ? it.done : undefined} max={it.total || undefined} /></div>}
    </li>
  )
}

function summaryWords(r: Run): string {
  const parts: string[] = []
  if (r.undone) return t('The updates were taken back.')
  parts.push(r.kind === 'install' ? tn('{count} installed', '{count} installed', r.updated) : tn('{count} updated', '{count} updated', r.updated))
  if (r.failed) parts.push(tn('{count} failed', '{count} failed', r.failed))
  if (r.skipped) parts.push(tn('{count} skipped', '{count} skipped', r.skipped))
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
        <h3 id={headId}>{finished ? (run.failed ? t('Finished, with problems') : t('All done')) : run.kind === 'install' ? t('Installing') : t('Updating your modules')}</h3>
        <span role="status" className="st-run-count">{finished ? summaryWords(run) : t('{done} of {total}', { done, total: run.items.length })}</span>
      </div>
      {!finished && <Progress label={t('Progress overall')} value={done} max={run.items.length} />}
      <ul className="st-run-list">{run.items.map((it) => <RunLine key={it.id} it={it} onExplain={setExplain} />)}</ul>
      {finished && (
        <div className="ms-actions">
          {run.canUndo && <Button size="sm" onClick={() => setUndoAsk(true)}>{t('Take these updates back')}</Button>}
          <Button size="sm" variant="ghost" onClick={dismiss}>{t('Dismiss')}</Button>
          <span className="muted st-run-when">{t('Finished {when}', { when: date(run.finished!, { dateStyle: 'medium', timeStyle: 'short' }) })}</span>
        </div>
      )}
      {explain?.failure && <FailureDialog failure={explain.failure} name={explain.name} moduleId={explain.id} repairRoute={repairRoute} onClose={() => setExplain(null)} />}
      {undoAsk && <ConfirmOwner onClose={() => setUndoAsk(false)} ask={{
        title: t('Take these updates back?'), action: t('Take them back'),
        body: <p>{t('These modules go back to the versions they were on. An older version may lack security fixes, so this asks for your password.')}</p>,
        run: async (password, code) => { await undoRun({ password, code }); toast(t('Those updates were taken back')); onChanged() },
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
    try { await startUpdateAll(); onChanged() } catch (e) { setFailure(failureOf(e, t('Update All'))) } finally { setStarting(false) }
  }
  return (
    <div className="st-updateall">
      <div className="st-updateall-top">
        <div role="status">
          <b className="st-updateall-count">{running ? t('Updating…') : n > 0 ? tn('{count} update available', '{count} updates available', n) : view.checkedAt ? t('Every module is up to date') : t('No updates known yet')}</b>
          {view.checkedAt ? <span className="muted"> {t('Last checked {when}.', { when: date(view.checkedAt, { dateStyle: 'medium', timeStyle: 'short' }) })}</span> : <span className="muted"> {t('The server hasn’t looked yet.')}</span>}
        </div>
        {/* Every module up to date and no run to show: no button that does nothing (#553). After a run it
            stays, quiet, so the focus a remote left on it isn't dropped. */}
        {(n > 0 || running || showsRun(view.run)) && <Button variant={n > 0 && !running ? 'primary' : 'default'} className={idle ? 'st-idle' : undefined} aria-busy={starting || running || undefined} aria-disabled={idle || undefined} onClick={go}>{n > 0 ? t('Update All Modules ({count})', { count: n }) : t('Update All Modules')}</Button>}
      </div>
      {showsRun(view.run) && <RunPanel run={view.run!} onChanged={onChanged} repairRoute={view.repairRoute} />}
      {failure && <FailureDialog failure={failure} name={t('Update All')} repairRoute={view.repairRoute} onClose={() => setFailure(null)} />}
    </div>
  )
}

