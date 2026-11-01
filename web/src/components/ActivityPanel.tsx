import { useEffect, useId, useState } from 'react'
import { STATE_NAMES, clearTaskHistory, fmtAgo, fmtCount, fmtDuration, fmtRate, taskAction, taskHistory, typeName, useActivity, type Task } from '../activity'
import { IconCheck, IconClose, IconInfo, IconPause, IconPlay, IconRefresh, IconSettings, IconStop, IconTrash } from '../icons'
import { Button, Chip, Chips, EmptyState, IconButton, LinkButton, LoadError, Notice, Progress, Sheet, Spinner, Tabs, ask, toast } from '../ui'

// The panel behind the activity button: what's going on, library by library, and what
// happened lately, with errors a person can act on. A sheet on phones.

type Tab = 'now' | 'history'

export default function ActivityPanel({ onClose }: { onClose: () => void }) {
  const a = useActivity()
  const [tab, setTab] = useState<Tab>('now')
  const panel = useId()
  const going = a.tasks.length
  const count = (n: number) => (n ? <span className="act-tab-n">{n}</span> : null)
  return (
    <Sheet title="Activity" onClose={onClose} className="act-sheet" actions={a.access.readOnly ? undefined : (
      <LinkButton to="/settings/activity" variant="ghost" size="sm" icon={<IconSettings size={16} />} onClick={onClose}>Activity settings</LinkButton>
    )}>
      <Tabs label="Activity" value={tab} onChange={setTab} panelId={panel} items={[{ value: 'now', label: 'Now', badge: count(going) }, { value: 'history', label: 'History' }]} />
      <div id={panel} role="tabpanel" className="act-body">
        {!a.connected && a.loaded && <p className="act-offline" role="status">Reconnecting to the server. This may be out of date.</p>}
        {a.access.readOnly && <p className="act-readonly">You can see what the server is doing here. Only an administrator can change it.</p>}
        {tab === 'now' ? <Now tasks={a.tasks} /> : <History recent={a.recent} readOnly={a.access.readOnly} />}
      </div>
    </Sheet>
  )
}

// Grouped by library, with the server's own work after them.
function Now({ tasks }: { tasks: Task[] }) {
  if (!tasks.length) return <EmptyState icon={<IconCheck />} title="Nothing going on" text="Library scans and other background work show up here while they run." />
  const groups = new Map<string, Task[]>()
  for (const t of tasks) {
    const name = t.libraryName || ''
    groups.set(name, [...(groups.get(name) ?? []), t])
  }
  const named = [...groups.keys()].filter(Boolean).sort((x, y) => x.localeCompare(y))
  return (
    <>
      {[...named, ...(groups.has('') ? [''] : [])].map((name) => {
        const list = groups.get(name)!
        const known = list.filter((t) => t.percent >= 0)
        const pct = known.length ? known.reduce((n, t) => n + t.percent, 0) / known.length : -1
        return (
          <section key={name} className="act-group" aria-label={name || 'This server'}>
            <h3><span>{name || 'This server'}</span>{pct >= 0 && <span className="act-group-pct">{Math.round(pct)}%</span>}</h3>
            <ul className="act-list">{list.map((t) => <li key={t.id}><LiveTask task={t} /></li>)}</ul>
          </section>
        )
      })}
    </>
  )
}

function useAct(t: Task) {
  const [busy, setBusy] = useState(false)
  const run = async (action: 'pause' | 'resume' | 'cancel' | 'retry') => {
    setBusy(true)
    try { await taskAction(t.id, action) } catch (e) { toast((e as Error).message) } finally { setBusy(false) }
  }
  return { busy, run }
}

function LiveTask({ task: t }: { task: Task }) {
  const { busy, run } = useAct(t)
  const facts = [fmtCount(t), fmtRate(t), t.etaSec ? `about ${fmtDuration(t.etaSec)} left` : ''].filter(Boolean)
  const waiting = t.state === 'queued'
  return (
    <article className="act-task">
      <div className="act-task-head">
        <div>
          <div className="act-title">{t.title}</div>
          <div className="act-sub">{[typeName(t.type), t.source, t.by && `started by ${t.by}`, `started ${fmtAgo(t.startedAt)}`].filter(Boolean).join(' · ')}</div>
        </div>
        {/* Asked to pause, and carrying on to the next point it can stop at. */}
        <span className={'act-state ' + t.state}>{t.state === 'running' && t.canResume ? 'Pausing…' : STATE_NAMES[t.state]}</span>
      </div>
      {t.state === 'paused' || waiting
        ? <Progress label={t.title} value={t.percent >= 0 ? t.percent : 0} />
        : <Progress label={t.title} value={t.percent >= 0 ? t.percent : undefined} showValue />}
      {t.phase && <div className="act-phase">{t.phase}</div>}
      {facts.length > 0 && <div className="act-facts">{facts.join(' · ')}</div>}
      {t.item && <div className="act-item" title={t.path || t.item}>{t.item}</div>}
      {t.path && t.path !== t.item && <div className="act-path">{t.path}</div>}
      {!!t.problemCount && <Problems task={t} />}
      {(t.canPause || t.canResume || t.canCancel) && (
        <div className="act-controls">
          {t.canPause && <Button size="sm" disabled={busy} icon={<IconPause size={16} />} aria-label={`Pause ${t.title}`} onClick={() => run('pause')}>Pause</Button>}
          {t.canResume && <Button size="sm" disabled={busy} icon={<IconPlay size={16} />} aria-label={`Resume ${t.title}`} onClick={() => run('resume')}>Resume</Button>}
          {t.canCancel && <Button size="sm" variant="ghost" disabled={busy} icon={<IconStop size={16} />} aria-label={`Cancel ${t.title}`} onClick={() => run('cancel')}>Cancel</Button>}
        </div>
      )}
    </article>
  )
}

function Problems({ task: t }: { task: Task }) {
  const n = t.problemCount ?? 0
  const words = n === 1 ? '1 problem' : `${n.toLocaleString()} problems`
  if (!t.problems?.length) return <div className="act-problem-n"><IconInfo size={16} /> {words}</div>
  const kept = t.problems.length
  return (
    <details className="act-problems">
      <summary><IconInfo size={16} /> {words}</summary>
      <ul>
        {t.problems.map((p, i) => (
          <li key={i}>
            {p.item && <div className="act-path">{p.item}</div>}
            <div>{p.message}{p.count && p.count > 1 ? ` (${p.count.toLocaleString()} times)` : ''}</div>
          </li>
        ))}
      </ul>
      {n > kept && <p className="act-more">The first {kept} are shown here. WittyLog has the rest.</p>}
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
  if (failed) return <LoadError what="The history" onRetry={() => load()} />
  if (!rows) return <Spinner />
  const clearAll = async () => {
    if (!await ask({ title: 'Clear the activity history?', body: 'What happened stops being listed here. WittyLog keeps its own record, and anything going on now carries on.', confirm: 'Clear history', danger: true })) return
    try { await clearTaskHistory(); setRows([]); setMore(false) } catch (e) { toast((e as Error).message) }
  }
  return (
    <>
      <div className="act-history-bar">
        <Chips label="Show">
          <Chip selected={!only} onClick={() => setOnly(false)}>All</Chip>
          <Chip selected={only} onClick={() => setOnly(true)}>Failed</Chip>
        </Chips>
        {!readOnly && rows.length > 0 && <Button size="sm" variant="ghost" icon={<IconTrash size={16} />} onClick={clearAll}>Clear history</Button>}
      </div>
      {rows.length === 0
        ? <EmptyState icon={<IconCheck />} title={only ? 'Nothing has failed' : 'No history yet'} text={only ? 'Anything that fails is listed here with what to do about it.' : 'Finished work is listed here.'} />
        : <ul className="act-list">{rows.map((t) => <li key={t.id}><PastTask task={t} readOnly={readOnly} onRemoved={() => setRows((r) => r!.filter((x) => x.id !== t.id))} /></li>)}</ul>}
      {more && <div className="act-load-more"><Button size="sm" onClick={() => load(rows[rows.length - 1].finishedAt)}>Show more</Button></div>}
    </>
  )
}

function PastTask({ task: t, readOnly, onRemoved }: { task: Task; readOnly: boolean; onRemoved: () => void }) {
  const { busy, run } = useAct(t)
  const took = t.finishedAt && t.startedAt ? t.finishedAt - t.startedAt : 0
  const remove = async () => {
    try { await clearTaskHistory(t.id); onRemoved() } catch (e) { toast((e as Error).message) }
  }
  return (
    <article className="act-task">
      <div className="act-task-head">
        <div>
          <div className="act-title">{t.title}</div>
          <div className="act-sub">{[t.finishedAt ? fmtAgo(t.finishedAt) : '', took > 0 ? `took ${fmtDuration(took)}` : '', t.by && `started by ${t.by}`].filter(Boolean).join(' · ')}</div>
        </div>
        <span className={'act-state ' + t.state}>
          {t.state === 'done' ? <IconCheck size={14} /> : t.state === 'failed' ? <IconClose size={14} /> : t.state === 'cancelled' ? <IconStop size={14} /> : null}
          {STATE_NAMES[t.state]}
        </span>
      </div>
      {t.result && <div className="act-facts">{t.result}</div>}
      {t.error && <Notice tone="danger" title={t.error}>{t.fix}</Notice>}
      {t.detail && <details className="act-detail"><summary>Technical details</summary><code>{t.detail}</code></details>}
      {!!t.problemCount && <Problems task={t} />}
      {!readOnly && (
        <div className="act-controls">
          {t.canRetry && <Button size="sm" disabled={busy} icon={<IconRefresh size={16} />} aria-label={`Try ${t.title} again`} onClick={() => run('retry')}>{t.type === 'scan' ? 'Scan again' : 'Try again'}</Button>}
          <IconButton size="sm" label={`Remove ${t.title} from the history`} onClick={remove}><IconTrash size={16} /></IconButton>
        </div>
      )}
    </article>
  )
}
