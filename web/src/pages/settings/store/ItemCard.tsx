// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { ApiError } from '../../../api'
import { toast } from '../../../state'
import { Button, Dialog, Progress } from '../../../ui'
import { FailureDialog, itemPath, ModuleIcon, StateTags } from './parts'
import { categoryTitle, currentRun, failureOf, installModule, isBusy, mb, PHASE, type Failure, type RunItem, type StoreItem } from './storeApi'

// One module, with the one thing to do next, wherever it's shown. What can be done comes from
// what the server says: nothing here decides whether something fits.

// actionFor names the next step, or nothing when there isn't one to take from a list. Anything
// from another store goes through its own page, because that's where its warning is.
export function actionFor(it: StoreItem): { label: string; aria: string } | null {
  const m = it.status
  if (it.kind === 'in-development' || it.kind === 'refused' || it.kind === 'fork') return null
  if (m && isBusy(m)) return null
  if (m?.installed) {
    if (it.failure && m.latest) return { label: 'Try again', aria: `Try again: update ${it.name}` }
    return m.latest && !m.pin ? { label: 'Update', aria: `Update ${it.name} to ${m.latest.version}` } : null
  }
  if (it.fit.ok || it.kind === 'server') return { label: it.failure ? 'Try again' : 'Install', aria: `${it.failure ? 'Try again: install' : 'Install'} ${it.name}${it.version ? ` ${it.version}` : ''}` }
  return null
}

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))

// useInstall presses the button: it asks first when a module brings others with it, follows the
// change, and opens the failure dialogue by itself if the change goes wrong. It follows the run
// rather than the page's view, since a small module can come and go between two looks at the page.
// While it does, the button stays where it is, so the focus isn't dropped and a remote stays put.
export function useInstall(it: StoreItem, onChanged: () => void, repairRoute: string, focusAfter?: () => void) {
  const [plan, setPlan] = useState<RunItem[] | null>(null)
  const [failure, setFailure] = useState<Failure | null>(null)
  const [working, setWorking] = useState(false)
  const going = useRef(false)
  const alive = useRef(true)
  useEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  const follow = async () => {
    for (;;) {
      await wait(400)
      if (!alive.current) return
      const r = await currentRun().catch(() => undefined)
      onChanged()
      if (!r || r.finished) return r
    }
  }
  const run = async (confirm: boolean) => {
    if (going.current) return
    going.current = true
    setPlan(null); setWorking(true)
    try {
      await installModule(it.id, confirm)
      onChanged()
      const r = await follow()
      if (!alive.current) return
      const failed = r?.items.find((x) => x.failure)
      if (failed?.failure) setFailure(failed.failure)
      else if (r) { toast(`${it.name} is ready`); setTimeout(() => focusAfter?.(), 0) }
    } catch (e) {
      if (e instanceof ApiError && e.data?.code === 'plan') setPlan(e.data.plan as RunItem[])
      else setFailure(failureOf(e, it.name))
    } finally { going.current = false; if (alive.current) setWorking(false) }
  }
  const dialogs = (
    <>
      {plan && <PlanDialog item={it} plan={plan} onClose={() => setPlan(null)} onYes={() => run(true)} />}
      {failure && <FailureDialog failure={failure} name={it.name} moduleId={it.id} repairRoute={repairRoute} onClose={() => setFailure(null)} onRetry={() => run(false)} />}
    </>
  )
  return { press: () => run(false), working, dialogs }
}

// The button for the next step, which stays put while that step runs.
export function ActionButton({ it, press, working, primary }: { it: StoreItem; press: () => void; working: boolean; primary?: boolean }) {
  const act = actionFor(it)
  const inProgress = working || isBusy(it.status)
  if (!act && !inProgress) return null
  const label = inProgress ? 'Working…' : act!.label + (primary && act!.label === 'Install' && it.size ? ` · ${mb(it.size)}` : '')
  return (
    <Button size={primary ? 'md' : 'sm'} variant={primary ? 'primary' : 'default'} className={primary ? undefined : 'st-card-act'} aria-busy={inProgress || undefined} aria-disabled={inProgress || undefined}
      aria-label={inProgress ? `Working… ${it.name}` : primary ? undefined : act!.aria} onClick={inProgress ? undefined : press}>{label}</Button>
  )
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
  const link = useRef<HTMLAnchorElement>(null)
  // The button goes once it's installed, and the focus shouldn't go with it.
  const { press, working, dialogs } = useInstall(item, onChanged, repairRoute, () => { if (document.activeElement === document.body) link.current?.focus() })
  const label = [item.name, item.official || item.kind === 'server' ? '' : `from ${item.storeName}`].filter(Boolean).join(', ')
  return (
    <article className={'st-card' + (item.official || item.kind === 'server' ? '' : ' third')} aria-label={label}>
      <Link ref={link} className="st-card-link" to={itemPath(item)}>
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
        <ActionButton it={item} press={press} working={working} />
      </div>
      <InPlaceProgress it={item} />
      {dialogs}
    </article>
  )
}
