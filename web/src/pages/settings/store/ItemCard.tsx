import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ApiError } from '../../../api'
import { toast } from '../../../state'
import { Button, Dialog, Progress } from '../../../ui'
import { FailureDialog, itemPath, ModuleIcon, StateTags } from './parts'
import { categoryTitle, failureOf, installModule, isBusy, mb, PHASE, type Failure, type RunItem, type StoreItem } from './storeApi'

// One module, with the one thing to do next, wherever it's shown. What can be done comes from
// what the server says: nothing here decides whether something fits.

// actionFor names the next step, or nothing when there isn't one to take from a list. Anything
// from another store goes through its own page, because that's where its warning is.
export function actionFor(it: StoreItem): { label: string; aria: string } | null {
  const m = it.status
  if (it.kind === 'in-development' || it.kind === 'refused' || it.kind === 'fork') return null
  if (m && isBusy(m)) return null
  if (m?.installed) {
    if (it.failure && m.latest) return { label: 'Try again', aria: `Try updating ${it.name} again` }
    return m.latest && !m.pin ? { label: 'Update', aria: `Update ${it.name} to ${m.latest.version}` } : null
  }
  if (it.fit.ok || it.kind === 'server') return { label: it.failure ? 'Try again' : 'Install', aria: `${it.failure ? 'Try installing' : 'Install'} ${it.name}${it.version ? ` ${it.version}` : ''}` }
  return null
}

// useInstall presses the button: it asks first when a module brings others with it, follows the
// change, and opens the failure dialogue by itself if the change goes wrong.
export function useInstall(it: StoreItem, onChanged: () => void, repairRoute: string) {
  const [plan, setPlan] = useState<RunItem[] | null>(null)
  const [failure, setFailure] = useState<Failure | null>(null)
  const [watching, setWatching] = useState(false)
  const [starting, setStarting] = useState(false)
  const busy = isBusy(it.status)
  const wasBusy = useWasTrue(busy)
  useEffect(() => {
    if (!watching || busy || !wasBusy.current) return
    setWatching(false); wasBusy.current = false
    if (it.failure) setFailure(it.failure)
    else toast(`${it.name} is ready`)
  }, [watching, busy, it.failure, it.name, wasBusy])
  const run = async (confirm: boolean) => {
    if (starting) return
    setStarting(true); setPlan(null)
    try {
      await installModule(it.id, confirm)
      setWatching(true)
      onChanged()
    } catch (e) {
      if (e instanceof ApiError && e.data?.code === 'plan') setPlan(e.data.plan as RunItem[])
      else setFailure(failureOf(e, it.name))
    } finally { setStarting(false) }
  }
  const dialogs = (
    <>
      {plan && <PlanDialog item={it} plan={plan} onClose={() => setPlan(null)} onYes={() => run(true)} />}
      {failure && <FailureDialog failure={failure} name={it.name} moduleId={it.id} repairRoute={repairRoute} onClose={() => setFailure(null)} onRetry={() => run(false)} />}
    </>
  )
  return { press: () => run(false), starting, dialogs }
}

// Remembers that something was true since the last time this was reset.
function useWasTrue(v: boolean) {
  const [box] = useState(() => ({ current: false }))
  if (v) box.current = true
  return box
}

// What else comes with a module, explained before anything happens.
function PlanDialog({ item, plan, onClose, onYes }: { item: StoreItem; plan: RunItem[]; onClose: () => void; onYes: () => void }) {
  const others = plan.filter((p) => p.id !== item.id)
  return (
    <Dialog title={`${item.name} needs ${others.length === 1 ? 'something else' : 'other modules'}`} onClose={onClose} size="sm"
      description={`${item.name} is built on ${others.length === 1 ? 'a module that isn’t' : 'modules that aren’t'} on this server yet, so ${others.length === 1 ? 'it' : 'they'} will be installed first, in this order.`}
      actions={<><Button variant="ghost" autoFocus onClick={onClose}>Cancel</Button><Button variant="primary" onClick={onYes}>Install {plan.length} modules</Button></>}>
      <ol className="st-plan">
        {plan.map((p) => <li key={p.id}><b>{p.name}</b>{p.to && <span className="ms-ver"> {p.to}</span>}{p.why && <span className="muted"> {p.why}</span>}</li>)}
      </ol>
    </Dialog>
  )
}

// Progress in place, while a module is being changed.
export function InPlaceProgress({ it }: { it: StoreItem }) {
  const m = it.status
  if (!m || !isBusy(m)) return null
  const known = m.phase === 'downloading' && !!m.total
  return (
    <div className="st-inplace" role="status">
      <span>{PHASE[m.phase]}{known ? `, ${mb(m.done)} of ${mb(m.total)}` : '…'}</span>
      <Progress label={`${it.name}: ${PHASE[m.phase]}`} value={known ? m.done : undefined} max={m.total || undefined} />
    </div>
  )
}

export function ItemCard({ item, onChanged, repairRoute }: { item: StoreItem; onChanged: () => void; repairRoute: string }) {
  const { press, starting, dialogs } = useInstall(item, onChanged, repairRoute)
  const act = actionFor(item)
  const label = [item.name, item.official || item.kind === 'server' ? '' : `from ${item.storeName}`].filter(Boolean).join(', ')
  return (
    <article className={'st-card' + (item.official || item.kind === 'server' ? '' : ' third')} aria-label={label}>
      <Link className="st-card-link" to={itemPath(item)}>
        <ModuleIcon item={item} />
        <span className="st-card-text">
          <b className="st-card-name">{item.name}</b>
          <span className="st-card-by">{item.publisher?.name || 'Part of this server'} · {categoryTitle(item.group)}</span>
          {item.summary && <span className="st-card-sum">{item.summary}</span>}
        </span>
      </Link>
      <div className="st-card-foot">
        <StateTags item={item} />
        {!item.official && item.kind !== 'server' && <span className="st-card-store">{item.storeName} ({item.storeHost})</span>}
        {act && <Button size="sm" className="st-card-act" busy={starting} aria-label={act.aria} onClick={press}>{act.label}</Button>}
      </div>
      <InPlaceProgress it={item} />
      {dialogs}
    </article>
  )
}
