// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../../../api'
import { t, tn } from '../../../i18n'
import { toast } from '../../../state'
import { Button, Dialog } from '../../../ui'
import { compat, type Compat } from './compat'
import { FailureNotice, ModuleIcon, type StoreViewState } from './parts'
import { failureOf, installModule, isBusy, startUpdateAll, type Failure, type StoreItem } from './storeApi'
import { InPlaceProgress } from './ItemCard'

// The installed modules that need a choice before they can run on this server, and the choice:
// Update, Switch or Remove. Built once and used in two places: at the top of the store's Installed
// tab, and as the mandatory Modules step of post-update setup (0.FuturePlans/0.9.0/UPDATE-OOBE.md
// §2), which is this list and nothing else. The states and words are the store's (compat.ts,
// MODULE-API-0.9.md §3a).
//
// Remove never deletes data here: the module goes, its settings and saves stay for when it comes
// back. Wiping data is a separate choice on the module's own row in Settings.

// The states that need a choice. A module that runs, even with a notice, isn't one.
const CHOICE = new Set(['needs-update', 'not-ready', 'withdrawn', 'blocked'])
export type ModuleChoice = { item: StoreItem; c: Compat }

// choicesOf: every installed module that needs a choice, most urgent first (security first).
export function choicesOf(items: StoreItem[], server: string): ModuleChoice[] {
  const out: ModuleChoice[] = []
  for (const item of items) {
    if (!item.status?.installed || item.kind === 'server') continue
    if (item.kind === 'refused') { out.push({ item, c: { state: 'blocked', ok: false, tone: 'bad', tag: t('Refused'), line: item.why || t('This server’s rules refuse {name}.', { name: item.name }) } }); continue }
    const c = compat(item, server)
    if (CHOICE.has(c.state)) out.push({ item, c })
  }
  const rank = (x: ModuleChoice) => (x.item.status?.withdrawn?.severity === 'security' ? 0 : x.c.state === 'withdrawn' ? 1 : x.c.state === 'needs-update' ? 2 : 3)
  return out.sort((a, b) => rank(a) - rank(b) || a.item.name.localeCompare(b.item.name))
}

// The one main action a row offers, besides Remove.
function mainAction(x: ModuleChoice): { label: string; aria: string } | null {
  const latest = x.item.status?.latest?.version
  if (!latest) return null
  if (x.c.state === 'withdrawn') return { label: t('Switch to {version}', { version: latest }), aria: t('Switch {name} to {version}', { name: x.item.name, version: latest }) }
  return { label: t('Update'), aria: t('Update {name} to {version}', { name: x.item.name, version: latest }) }
}

export function ModuleChoiceRow({ choice, store, offline }: { choice: ModuleChoice; store: StoreViewState; offline?: boolean }) {
  const { item, c } = choice
  const [working, setWorking] = useState(false)
  const [failure, setFailure] = useState<Failure | null>(null)
  const [asking, setAsking] = useState(false)
  const act = mainAction(choice)
  const busy = working || isBusy(item.status)
  const update = async () => {
    setFailure(null); setWorking(true)
    try { await installModule(item.id, true); await store.reload() } catch (e) { setFailure(failureOf(e, item.name)) } finally { setWorking(false) }
  }
  const remove = async () => {
    setAsking(false); setFailure(null); setWorking(true)
    try { await api.post(`/modules/${encodeURIComponent(item.id)}/remove`); toast(t('{name} removed. What it saved is kept.', { name: item.name })); await store.reload() }
    catch (e) { setFailure(failureOf(e, item.name)) } finally { setWorking(false) }
  }
  // Offline, nothing newer can be known: say so, and Remove still finishes the choice.
  const line = offline && !act ? t('Can’t check for updates right now.') : c.line
  return (
    <li className="st-choice" aria-label={item.name}>
      <div className="st-choice-head">
        <ModuleIcon item={item} size={40} />
        <div className="st-choice-text">
          <b>{item.name}</b> <span className="ms-ver">{item.status?.version}</span> <span className={'ms-tag ' + c.tone}>{c.tag}</span>
          <p>{line}</p>
        </div>
        <div className="st-choice-act">
          {act && <Button size="sm" variant="primary" aria-label={act.aria} aria-busy={busy || undefined} onClick={busy ? undefined : update}>{busy ? t('Working…') : act.label}</Button>}
          {offline && !act && <Button size="sm" onClick={() => void store.check()} aria-busy={store.checking || undefined}>{t('Try again')}</Button>}
          <Button size="sm" variant={act ? 'ghost' : 'primary'} aria-label={t('Remove {name}', { name: item.name })} onClick={busy ? undefined : () => setAsking(true)}>{t('Remove')}</Button>
        </div>
      </div>
      <InPlaceProgress it={item} />
      {failure && <FailureNotice failure={failure} name={item.name} moduleId={item.id} repairRoute={store.view?.repairRoute || '/settings/repair'} onRetry={act ? update : undefined} />}
      {asking && (
        <Dialog title={t('Remove {name}?', { name: item.name })} size="sm" onClose={() => setAsking(false)}
          actions={<><Button variant="ghost" autoFocus onClick={() => setAsking(false)}>{t('Cancel')}</Button><Button variant="danger" onClick={remove}>{t('Remove')}</Button></>}>
          <p>{t('{name}’s settings and saved data are kept. If you install it again later, it picks them up.', { name: item.name })}</p>
        </Dialog>
      )}
    </li>
  )
}

// The list, with Update all at the top and how many are left. onLeft tells a host (the setup step)
// how many still need a choice, so it can hold Continue until there are none.
export function ModuleChoiceList({ store, title, onLeft, showEmpty }: { store: StoreViewState; title?: string; onLeft?: (n: number) => void; showEmpty?: boolean }) {
  const view = store.view
  const [starting, setStarting] = useState(false)
  const list = view ? choicesOf(view.items, view.server) : []
  const left = view ? list.length : -1
  useEffect(() => { if (left >= 0) onLeft?.(left) }, [left, onLeft])
  if (!view) return null
  if (!list.length) return showEmpty ? <p className="st-choices-none" role="status">{t('All your modules are ready for this version.')}</p> : null
  const offline = !!view.checkError || view.stores.some((s) => s.official && s.problem?.kind === 'network')
  const updatable = list.filter((x) => mainAction(x)).length
  const updateAll = async () => { setStarting(true); try { await startUpdateAll(); await store.reload() } catch (e) { toast(failureOf(e, t('Update all')).message) } finally { setStarting(false) } }
  return (
    <section className="set-card st-choices" aria-labelledby="st-choices-h">
      <header className="set-card-head">
        <div><h2 id="st-choices-h">{title || tn('{count} module needs your choice', '{count} modules need your choice', list.length)}</h2>
          <p role="status">{tn('{count} module still needs a choice', '{count} modules still need a choice', list.length)}</p></div>
        {updatable > 1 && <Button size="sm" variant="primary" aria-busy={starting || undefined} onClick={starting ? undefined : updateAll}>{t('Update all')}</Button>}
      </header>
      <ul className="set-card-body st-choice-list">{list.map((x) => <ModuleChoiceRow key={x.item.key} choice={x} store={store} offline={offline} />)}</ul>
    </section>
  )
}
