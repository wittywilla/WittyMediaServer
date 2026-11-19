// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useId, useState } from 'react'
import { STATE_NAMES, clearTaskHistory, fmtAgo, fmtCount, fmtDuration, fmtRate, taskAction, taskHistory, typeName, useActivity, type Task } from '../activity'
import { IconCheck, IconClose, IconInfo, IconPause, IconPlay, IconRefresh, IconSettings, IconStop, IconTrash } from '../icons'
import { useApp } from '../state'
import SpecPage from '../spec/SpecPage'
import { handledIds, withIds } from '../spec/model'
import { activitySpec } from '../spec/specs/activity'
import { Button, EmptyState, IconButton, LinkButton, LoadError, Notice, Progress, Sheet, Spinner, Tabs, ask, toast } from '../ui'
import { t, tn, tx } from '../i18n'

// The panel behind the activity button: what's going on, library by library, and what
// happened lately, with errors a person can act on. A sheet on phones.

type Tab = 'now' | 'history'

export default function ActivityPanel({ onClose }: { onClose: () => void }) {
  const a = useActivity()
  const { update, user } = useApp()
  const [tab, setTab] = useState<Tab>('now')
  const panel = useId()
  const going = a.tasks.length
  const notices = activitySpec({ tab, connected: a.connected, loaded: a.loaded, readOnly: a.access.readOnly, update: update || '', isAdmin: !!user?.isAdmin, failedOnly: false, canClear: false, part: 'notices' })
  const upAt = notices.sections.findIndex((x) => x.h === t('Update available'))
  const count = (n: number) => (n ? <span className="act-tab-n">{n}</span> : null)
  return (
    <Sheet title={t('Activity')} onClose={onClose} className="act-sheet" actions={a.access.readOnly ? undefined : (
      <LinkButton to="/settings/activity" variant="ghost" size="sm" icon={<IconSettings size={16} />} onClick={onClose}>{t('Activity settings')}</LinkButton>
    )}>
      <Tabs label={t('Activity')} value={tab} onChange={setTab} panelId={panel} items={[{ value: 'now', label: t('Now'), badge: count(going) }, { value: 'history', label: t('History') }]} />
      <div id={panel} role="tabpanel" className="act-body">
        {notices.sections.length > 0 && <SpecPage embedded page={notices} after={upAt >= 0 ? { [upAt]: <LinkButton to="/settings/about#updates" size="sm" onClick={onClose}>{t('What’s new')}</LinkButton> } : undefined} />}
        {tab === 'now' ? <Now tasks={a.tasks} /> : <History recent={a.recent} readOnly={a.access.readOnly} />}
      </div>
    </Sheet>
  )
}

// Grouped by library, with the server's own work after them.
function Now({ tasks }: { tasks: Task[] }) {
  if (!tasks.length) return <EmptyState icon={<IconCheck />} title={t('Nothing going on')} text={t('Library scans and other background work show up here while they run.')} />
  const groups = new Map<string, Task[]>()
  for (const task of tasks) {
    const name = task.libraryName || ''
    groups.set(name, [...(groups.get(name) ?? []), task])
  }
  const named = [...groups.keys()].filter(Boolean).sort((x, y) => x.localeCompare(y))
  return (
    <>
      {[...named, ...(groups.has('') ? [''] : [])].map((name) => {
        const list = groups.get(name)!
        const known = list.filter((task) => task.percent >= 0)
        const pct = known.length ? known.reduce((n, task) => n + task.percent, 0) / known.length : -1
        return (
          <section key={name} className="act-group" aria-label={name || t('This server')}>
            <h3><span>{name || t('This server')}</span>{pct >= 0 && <span className="act-group-pct">{Math.round(pct)}%</span>}</h3>
            <ul className="act-list">{list.map((task) => <li key={task.id}><LiveTask task={task} /></li>)}</ul>
          </section>
        )
      })}
    </>
  )
}

function useAct(task: Task) {
  const [busy, setBusy] = useState(false)
  const run = async (action: 'pause' | 'resume' | 'cancel' | 'retry') => {
    setBusy(true)
    try { await taskAction(task.id, action) } catch (e) { toast((e as Error).message) } finally { setBusy(false) }
  }
  return { busy, run }
}

function LiveTask({ task }: { task: Task }) {
  const { busy, run } = useAct(task)
  const facts = [fmtCount(task), fmtRate(task), task.etaSec ? t('about {time} left', { time: fmtDuration(task.etaSec) }) : ''].filter(Boolean)
  const waiting = task.state === 'queued'
  return (
    <article className="act-task">
      <div className="act-task-head">
        <div>
          <div className="act-title">{task.title}</div>
          <div className="act-sub">{[tx(typeName(task.type)), task.source, task.by && t('started by {name}', { name: task.by }), t('started {when}', { when: fmtAgo(task.startedAt) })].filter(Boolean).join(' · ')}</div>
        </div>
        {/* Asked to pause, and carrying on to the next point it can stop at. */}
        <span className={'act-state ' + task.state}>{task.state === 'running' && task.canResume ? t('Pausing…') : tx(STATE_NAMES[task.state])}</span>
      </div>
      {task.state === 'paused' || waiting
        ? <Progress label={task.title} value={task.percent >= 0 ? task.percent : 0} />
        : <Progress label={task.title} value={task.percent >= 0 ? task.percent : undefined} showValue />}
      {!!task.steps?.length && <Steps task={task} />}
      {task.phase && task.phase !== task.steps?.[(task.step ?? 0) - 1] && <div className="act-phase">{tx(task.phase)}</div>}
      {facts.length > 0 && <div className="act-facts">{facts.join(' · ')}</div>}
      {task.item && <div className="act-item" title={task.path || task.item}>{task.item}</div>}
      {task.path && task.path !== task.item && <div className="act-path">{task.path}</div>}
      {!!task.problemCount && <Problems task={task} />}
      {(task.canPause || task.canResume || task.canCancel) && (
        <div className="act-controls">
          {task.canPause && <Button size="sm" disabled={busy} icon={<IconPause size={16} />} aria-label={t('Pause {title}', { title: task.title })} onClick={() => run('pause')}>{t('Pause')}</Button>}
          {task.canResume && <Button size="sm" disabled={busy} icon={<IconPlay size={16} />} aria-label={t('Resume {title}', { title: task.title })} onClick={() => run('resume')}>{t('Resume')}</Button>}
          {task.canCancel && <Button size="sm" variant="ghost" disabled={busy} icon={<IconStop size={16} />} aria-label={t('Cancel {title}', { title: task.title })} onClick={() => run('cancel')}>{t('Cancel')}</Button>}
        </div>
      )}
    </article>
  )
}

// The phases of one bar, so it's plain why it doesn't start again at each.
function Steps({ task }: { task: Task }) {
  const on = task.step ?? 0
  return (
    <ol className="act-steps" aria-label={t('Steps')}>
      {task.steps!.map((s, i) => (
        <li key={i} className={i + 1 < on ? 'done' : i + 1 === on ? 'on' : undefined} aria-current={i + 1 === on ? 'step' : undefined}>
          {i + 1 < on && <IconCheck size={14} />}{tx(s)}{i + 1 < on && <span className="sr-only">{t(', done')}</span>}
        </li>
      ))}
    </ol>
  )
}

function Problems({ task }: { task: Task }) {
  const n = task.problemCount ?? 0
  const words = tn('{count} problem', '{count} problems', n)
  if (!task.problems?.length) return <div className="act-problem-n"><IconInfo size={16} /> {words}</div>
  const kept = task.problems.length
  return (
    <details className="act-problems">
      <summary><IconInfo size={16} /> {words}</summary>
      <ul>
        {task.problems.map((p, i) => (
          <li key={i}>
            {p.item && <div className="act-path">{p.item}</div>}
            <div>{tx(p.message)}{p.count && p.count > 1 ? ' ' + t('({count} times)', { count: p.count }) : ''}</div>
          </li>
        ))}
      </ul>
      {n > kept && <p className="act-more">{t('The first {count} are shown here. WittyLog has the rest.', { count: kept })}</p>}
    </details>
  )
}

const PAGE = 30

function History({ recent, readOnly }: { recent: Task[]; readOnly: boolean }) {
  const [rows, setRows] = useState<Task[] | null>(null)
  const [failed, setFailed] = useState(false)
  const [more, setMore] = useState(false)
  const [only, setOnly] = useState(false)
  const load = (before?: number) => {
    setFailed(false)
    taskHistory({ before, limit: PAGE, state: only ? 'failed' : undefined }).then((list) => {
      setRows((was) => (before && was ? [...was, ...list] : list))
      setMore(list.length === PAGE)
    }).catch(() => setFailed(true))
  }
  // Again whenever something finishes, which is when there's something new to read.
  const newest = recent[0]?.id
  useEffect(() => { load() }, [only, newest]) // eslint-disable-line react-hooks/exhaustive-deps
  if (failed) return <LoadError what={t('The history')} onRetry={() => load()} />
  if (!rows) return <Spinner />
  const clearAll = async () => {
    if (!await ask({ title: t('Clear the activity history?'), body: t('What happened stops being listed here. WittyLog keeps its own record, and anything going on now carries on.'), confirm: t('Clear history'), danger: true })) return
    try { await clearTaskHistory(); setRows([]); setMore(false) } catch (e) { toast((e as Error).message) }
  }
  const bar = activitySpec({ tab: 'history', connected: true, loaded: true, readOnly, update: '', isAdmin: false, failedOnly: only, canClear: !readOnly && rows.length > 0, part: 'filter' })
  const barIds = handledIds(withIds(bar))
  return (
    <>
      <SpecPage embedded page={bar} handled={barIds} onAction={(id) => { if (id === 'show:all') setOnly(false); else if (id === 'show:failed') setOnly(true); else if (id === 'clear') void clearAll() }} />
      {rows.length === 0
        ? <EmptyState icon={<IconCheck />} title={only ? t('Nothing has failed') : t('No history yet')} text={only ? t('Anything that fails is listed here with what to do about it.') : t('Finished work is listed here.')} />
        : <ul className="act-list">{rows.map((task) => <li key={task.id}><PastTask task={task} readOnly={readOnly} onRemoved={() => setRows((r) => r!.filter((x) => x.id !== task.id))} /></li>)}</ul>}
      {more && <div className="act-load-more"><Button size="sm" onClick={() => load(rows[rows.length - 1].finishedAt)}>{t('Show more')}</Button></div>}
    </>
  )
}

function PastTask({ task, readOnly, onRemoved }: { task: Task; readOnly: boolean; onRemoved: () => void }) {
  const { busy, run } = useAct(task)
  const took = task.finishedAt && task.startedAt ? task.finishedAt - task.startedAt : 0
  const remove = async () => {
    try { await clearTaskHistory(task.id); onRemoved() } catch (e) { toast((e as Error).message) }
  }
  return (
    <article className="act-task">
      <div className="act-task-head">
        <div>
          <div className="act-title">{task.title}</div>
          <div className="act-sub">{[task.finishedAt ? fmtAgo(task.finishedAt) : '', took > 0 ? t('took {time}', { time: fmtDuration(took) }) : '', task.by && t('started by {name}', { name: task.by })].filter(Boolean).join(' · ')}</div>
        </div>
        <span className={'act-state ' + task.state}>
          {task.state === 'done' ? <IconCheck size={14} /> : task.state === 'failed' ? <IconClose size={14} /> : task.state === 'cancelled' ? <IconStop size={14} /> : null}
          {tx(STATE_NAMES[task.state])}
        </span>
      </div>
      {task.result && <div className="act-facts">{task.result}</div>}
      {task.error && <Notice tone="danger" title={tx(task.error)}>{tx(task.fix)}</Notice>}
      {task.detail && <details className="act-detail"><summary>{t('Technical details')}</summary><code>{task.detail}</code></details>}
      {!!task.problemCount && <Problems task={task} />}
      {!readOnly && (
        <div className="act-controls">
          {task.canRetry && <Button size="sm" disabled={busy} icon={<IconRefresh size={16} />} aria-label={t('Try {title} again', { title: task.title })} onClick={() => run('retry')}>{task.type === 'scan' ? t('Scan again') : t('Try again')}</Button>}
          <IconButton size="sm" label={t('Remove {title} from the history', { title: task.title })} onClick={remove}><IconTrash size={16} /></IconButton>
        </div>
      )}
    </article>
  )
}
