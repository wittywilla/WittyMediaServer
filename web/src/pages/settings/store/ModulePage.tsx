// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, ApiError, getToken, serverBase } from '../../../api'
import { BackLink } from '../../../components/common'
import { IconCheck, IconInfo, IconShield } from '../../../icons'
import type { ModuleReport, VersionInfo } from '../../../moduleStore'
import { toast } from '../../../state'
import { ask, Button, Dialog, Notice, Spinner } from '../../../ui'
import { ConfirmOwner, type OwnerAsk } from '../ConfirmOwner'
import { actionFor, ActionButton, InPlaceProgress, useInstall } from './ItemCard'
import { ChangeNotes, FailureDialog, FailureNotice, itemPath, ModuleIcon, ModuleLogDialog, PolicyNote, StateTags, Tag, usePicture } from './parts'
import { OverrideRecordBox, PendingNotice, StatusNotice, useOverrideDialog, WhatItAdds } from './overrideParts'
import { itemModuleKind, moduleKindTitle, needsOverrideOf, overrideLines, type NeedsOverride } from './overrides'
import { InstallFromFile } from './InstallFromFile'
import SpecPage from '../../../spec/SpecPage'
import { handledIds } from '../../../spec/model'
import { detailsSpec, manageSpec, permissionsSpec } from '../../../spec/specs/moduleDetail'
import { OverrideDialog } from './OverrideDialog'
import { categoryTitle, dayOf, failureOf, isBusy, itemDetail, mb, platformName, useFork, type Failure, type ItemDetail } from './storeApi'

// A module's own page: everything needed to decide, and everything that can be done to it.

export default function ModulePage() {
  const params = useParams()
  const store = params.store || 'official'
  const id = params.id || ''
  const [d, setD] = useState<ItemDetail | null>(null)
  const [error, setError] = useState<Failure | null>(null)
  const load = useCallback(async () => {
    try { setD(await itemDetail(store, id)); setError(null) } catch (e) { setError(failureOf(e, 'This module')) }
  }, [store, id])
  useEffect(() => { void load() }, [load])
  const busy = isBusy(d?.item.status)
  useEffect(() => {
    if (!busy) return
    const t = setInterval(() => { void load() }, 1000)
    return () => clearInterval(t)
  }, [busy, load])
  useEffect(() => { document.getElementById('st-module-h')?.focus({ preventScroll: true }) }, [id])

  if (error && !d) return (
    <div className="ms-page st-page">
      <BackLink to="/settings/modules/store" label="Module store" />
      <Notice tone="danger" title={error.title} action={<Button size="sm" onClick={load}>Try again</Button>}>{error.message}</Notice>
    </div>
  )
  if (!d) return <div className="ms-page st-page"><BackLink to="/settings/modules/store" label="Module store" /><Spinner /></div>
  return <Body d={d} reload={load} />
}

function Body({ d, reload }: { d: ItemDetail; reload: () => void }) {
  const it = d.item
  const m = it.status
  const repair = '/settings/repair'
  const { press, working: installing, dialogs } = useInstall(it, reload, repair, () => { if (document.activeElement === document.body) document.getElementById('st-module-h')?.focus() })
  const [logOpen, setLogOpen] = useState(false)
  const [failure, setFailure] = useState<Failure | null>(null)
  const [forkAsk, setForkAsk] = useState(false)
  const [ownerAsk, setOwnerAsk] = useState<OwnerAsk | null>(null)
  const act = actionFor(it)
  const third = !it.official && it.kind !== 'server'
  const installedHere = !!m?.installed
  const working = isBusy(m)
  const usingThisFork = third && m?.source?.store === it.store
  const news = m?.latest?.notes || it.changelog

  const call = async (run: () => Promise<unknown>, done?: string) => {
    try { await run(); if (done) toast(done); reload() } catch (e) { setFailure(failureOf(e, it.name)) }
  }
  const remove = async () => {
    if (!await ask({ title: `Remove ${it.name}?`, body: `${it.name} stops working and stops looking for updates. Your files and what you've watched, read or played stay as they are. You can install it again any time.`, confirm: 'Remove', danger: true })) return
    await call(() => api.post<ModuleReport>(`/modules/${it.id}/remove`), `${it.name} removed`)
  }
  const toggle = (on: boolean) => call(() => api.post<ModuleReport>(`/modules/${it.id}/${on ? 'enable' : 'disable'}`), on ? `${it.name} turned on` : `${it.name} turned off`)
  const rollback = () => {
    setOwnerAsk({
      title: m?.previous ? `Go back to version ${m.previous}?` : `Stop using ${it.name} ${m?.version}?`, action: m?.previous ? 'Go back' : 'Stop using it',
      body: <p>{it.name} stops using version {m?.version}, and automatic updates won’t bring that version back. You can still choose it yourself.{m?.previous ? ' An older version may lack security fixes, so this asks for your password.' : ' There’s no earlier version here, so it won’t be working until you install another. This asks for your password.'}</p>,
      run: async (password, code) => { await api.post<ModuleReport>(`/modules/${it.id}/rollback`, { password, code }); toast(m?.previous ? `${it.name} went back to version ${m.previous}` : `${it.name} is no longer in use`); reload() },
    })
  }
  const official = async () => {
    if (!await ask({ title: 'Go back to the official version?', body: `What came from this store is deleted, and ${it.name} isn’t on this server until you install the official one, which is one press from its page.`, confirm: 'Go back' })) return
    await call(() => api.del<ModuleReport>(`/modules/${it.id}/source`), `${it.name} is back to the official version`)
  }
  const over = useOverrideDialog()
  const permissionWords = Object.fromEntries(d.permissions.map((p) => [p.name, p.words]))
  // A version the usual rules refuse, with an override: the same request again, confirmed.
  const overrideVersion = (version: string, need: NeedsOverride) => over.open({
    id: it.id, name: `${it.name} ${version}`, need: { ...need, version: need.version || version }, permissionWords,
    send: async (body) => { await api.post<ModuleReport>(`/modules/${it.id}/install`, { version, confirm: true, ...body }); toast(`${it.name} ${version} is being installed`); reload() },
  })
  const goAhead = () => { if (m?.pending) overrideVersion(m.pending.version, { ...m.pending.needsOverride, version: m.pending.version, pending: true }) }
  const chooseVersion = async (v: VersionInfo) => {
    const install = (confirm: boolean, secrets?: { password: string; code: string }) => api.post<ModuleReport>(`/modules/${it.id}/install`, { version: v.version, confirm, ...secrets })
    try { await install(false); reload() } catch (e) {
      const need = needsOverrideOf(e)
      if (need) { overrideVersion(v.version, need); return }
      if (e instanceof ApiError && e.data?.code === 'confirm') {
        if (!await ask({ title: `Use ${it.name} ${v.version}?`, body: e.data.warning, confirm: 'Install anyway' })) return
        try { await install(true); reload() } catch (x) {
          const n = needsOverrideOf(x)
          if (n) overrideVersion(v.version, n)
          else if (x instanceof ApiError && x.data?.code === 'password-needed') setOwnerAsk({
            title: `Go back to ${it.name} ${v.version}?`, action: 'Go back',
            body: <p>An older version may lack security fixes, so this asks for your password.</p>,
            run: async (password, code) => { await install(true, { password, code }); reload() },
          })
          else setFailure(failureOf(x, it.name))
        }
      } else setFailure(failureOf(e, it.name))
    }
  }

  const managePage = manageSpec({ installed: installedHere, canRollback: !!m?.canRollback, previous: m?.previous || undefined, core: !!m?.core, enabled: m?.enabled !== false, builtin: m?.delivery === 'builtin', usingFork: usingThisFork, working })
  return (
    <div className="ms-page st-page st-module">
      <BackLink to="/settings/modules/store" label="Module store" />

      <header className="st-mod-head">
        <ModuleIcon item={it} size={96} />
        <div className="st-mod-title">
          <h1 id="st-module-h" tabIndex={-1}>{it.name}</h1>
          <p className="st-mod-by">{it.publisher?.url ? <a href={it.publisher.url} target="_blank" rel="noreferrer noopener">{it.publisher.name}</a> : it.publisher?.name || 'Part of this server'} · {categoryTitle(it.group)}</p>
          <StateTags item={it} />
        </div>
        <div className="st-mod-act">
          <ActionButton it={it} press={press} working={installing} primary />
          {it.kind === 'fork' && !usingThisFork && it.fit.ok && <Button variant="danger" onClick={() => setForkAsk(true)}>Use this fork…</Button>}
        </div>
      </header>

      <InPlaceProgress it={it} />
      {dialogs}
      {over.dialog}

      {m && <PendingNotice m={m} onGo={goAhead} onDropped={reload} />}
      <StatusNotice status={it.releaseStatus || m?.releaseStatus} note={it.statusNote || m?.statusNote} name={it.name} installed={installedHere} />
      {(it.override || m?.override) && <OverrideRecordBox id={it.id} name={it.name} record={(it.override || m?.override)!} disabled={working} onRemoved={reload} />}

      {third && (
        <Notice tone="warn" title={`Third-party: from ${it.storeName} (${it.storeHost})`}>
          Nobody at this project has reviewed this. It comes from a store you added, and whoever runs that store holds the key it’s signed with. <Link to="/settings/modules/stores">Manage stores</Link>
          {d.fingerprint && <span className="st-print">Key fingerprint <code>{d.fingerprint}</code></span>}
        </Notice>
      )}
      {it.lookalike && <Notice tone="danger" title="This uses the name of another module">It calls itself {it.name}, the name of a module from this project, and it isn’t that module. Don’t assume it does what that one does.</Notice>}
      {it.kind === 'in-development' && <Notice title="A module this server doesn’t know" action={<InstallFromFile onInstalled={reload} />}>{it.why} If you have its release archive and trust whoever made it, it can be installed from the file, with an override.</Notice>}
      {it.kind === 'refused' && <Notice tone="danger" title="This can’t be used">{it.why}</Notice>}
      {!it.fit.ok && it.kind !== 'in-development' && it.kind !== 'refused' && <Notice tone={it.fit.kind === 'library' ? undefined : 'warn'} title={it.fit.kind === 'too-old' ? 'Built for an older server' : it.fit.kind === 'older-server' ? 'Made for an older server' : it.fit.kind === 'too-new' ? 'Needs a newer server' : it.fit.kind === 'library' ? 'Comes with other modules' : 'Can’t be installed here'}>{it.fit.why}{it.fit.kind === 'older-server' && !installedHere && ' Installing it asks you first.'}</Notice>}
      {m?.olderServer && m.olderWarning && it.fit.kind !== 'older-server' && <Notice tone="warn" title="Made for an older server">{m.olderWarning}</Notice>}
      {it.failure && <FailureNotice failure={it.failure} name={it.name} moduleId={it.id} repairRoute={repair} onRetry={act ? press : undefined} />}
      {m?.notice && !it.failure && <Notice tone="warn">{m.notice}</Notice>}

      {it.description || it.summary ? <section className="st-mod-sec" aria-labelledby="st-about"><h2 id="st-about">About</h2>{(it.description || it.summary || '').split(/\n{2,}/).map((p, i) => <p key={i}>{p}</p>)}</section> : null}

      {!!it.screenshots?.length && (
        <section className="st-mod-sec" aria-labelledby="st-shots">
          <h2 id="st-shots">Screenshots</h2>
          <div className="st-shots" tabIndex={0} role="group" aria-label={`Screenshots of ${it.name}`} onKeyDown={shotKeys}>
            {it.screenshots.map((src, i) => <Screenshot key={src} src={src} alt={`${it.name}, screenshot ${i + 1} of ${it.screenshots!.length}`} />)}
          </div>
        </section>
      )}

      <WhatItAdds ui={it.ui || m?.uiAdds} name={it.name} />

      <section className="st-mod-sec" aria-labelledby="st-new">
        <h2 id="st-new">What’s new{(m?.latest?.version || it.version) ? <span className="ms-ver"> {m?.latest?.version || it.version}</span> : null}</h2>
        {it.released && !m?.latest && <p className="muted st-news-day">Released {dayOf(it.released)}</p>}
        <ChangeNotes text={news} />
      </section>

      <section className="st-mod-sec" aria-labelledby="st-asks">
        <h2 id="st-asks">What it asks for</h2>
        <SpecPage bare page={permissionsSpec(d.permissions)} />
        <p className="muted">Whatever it asks for, a module can read what this server’s account can read, and the server’s own limits apply to it. See <Link to="/help/module-store#what-limits-a-module">what limits a module</Link>.</p>
      </section>

      <section className="st-mod-sec" aria-labelledby="st-facts">
        <h2 id="st-facts">Details</h2>
        <SpecPage bare page={detailsSpec([
          { title: installedHere ? 'In use' : 'Version', value: installedHere ? m!.version : it.version || 'Not released yet' },
          ...(installedHere && m!.latest ? [{ title: 'Newest', value: m!.latest.version }] : []),
          ...(it.size ? [{ title: 'Download', value: mb(it.size) }] : []),
          { title: 'Publisher', value: it.publisher?.name || 'This server' },
          ...(moduleKindTitle(itemModuleKind(it)) ? [{ title: 'Kind', value: moduleKindTitle(itemModuleKind(it))! }] : []),
          ...(it.license || m?.license ? [{ title: 'Licence', value: (it.license || m?.license)! }] : []),
          ...(it.released ? [{ title: 'Released', value: dayOf(it.released) }] : []),
          ...(it.minServer ? [{ title: 'Fits servers', value: `${it.minServer} to ${it.maxServer}${it.fit.ok ? ', which includes this server' : ''}` }] : []),
          ...(it.platforms?.length ? [{ title: 'Works on', value: it.platforms.map(platformName).join(', ') }] : []),
          ...(it.needs?.length ? [{ title: 'What it needs', value: it.needs.join(', ') }] : []),
          ...(it.repo ? [{ title: 'Source', value: `github.com/${it.repo}` }] : []),
        ], d.builtOn.map((r) => ({ id: r.id, name: r.installed ? `${r.name} (installed)` : r.name, to: itemPath({ official: true, store: 'official', id: r.id, kind: 'official' }) })))} />
        {(it.updates || it.stays) && (
          <div className="ms-how">
            <b>What an update changes</b>
            {it.updates && <p>{it.updates}</p>}
            {it.stays && <p><span className="ms-how-sub">Only a new server changes:</span> {it.stays}</p>}
          </div>
        )}
        <PolicyNote view={{ policy: d.policy, server: d.server }} />
      </section>

      {(installedHere || d.versions.length > 0) && (
        <section className="st-mod-sec" aria-labelledby="st-manage">
          <h2 id="st-manage">Manage</h2>
          <SpecPage bare page={managePage} handled={handledIds(managePage)} after={{ 0: installedHere ? <NoticesButton id={it.id} name={it.name} /> : null }}
            onAction={(id) => { if (id === 'rollback') rollback(); else if (id === 'off') void toggle(false); else if (id === 'on') void toggle(true); else if (id === 'remove') void remove(); else if (id === 'official') void official(); else if (id === 'log') setLogOpen(true) }} />
          {m?.core && <p className="muted">Other modules are built on this one, so it stays.</p>}
          {d.versions.length > 0 && (
            <details className="ms-more">
              <summary>All versions ({d.versions.length})</summary>
              <ul className="ms-vers st-vers">
                {d.versions.map((v) => (
                  <li key={v.version} className={'ms-ver-row' + (v.inUse ? ' on' : '') + (v.withdrawn ? ' withdrawn' : '') + (!v.inUse && !v.allowed ? ' no' : '')}>
                    <div className="ms-ver-head">
                      <div className="ms-ver-title">
                        <b>{v.version}</b>
                        {v.published ? <span className="muted">{new Date(v.published).toLocaleDateString(undefined, { dateStyle: 'medium' })}</span> : null}
                        <span className="ms-tags">
                          {v.inUse && <Tag tone="ok" icon={<IconCheck size={12} />}>In use</Tag>}
                          {v.prerelease && <Tag>Pre-release</Tag>}
                          {v.withdrawn && <Tag tone="bad" icon={<IconInfo size={12} />}>Withdrawn</Tag>}
                        </span>
                      </div>
                      {!v.inUse && installedHere && (!third || usingThisFork) && <Button size="sm" disabled={!(v.allowed || v.override) || working} aria-label={`Use version ${v.version}${!v.allowed && v.override ? ', with an override' : ''}`} onClick={() => chooseVersion(v)}>{!v.allowed && v.override ? 'Install anyway…' : v.installed ? 'Switch to this' : 'Install'}</Button>}
                    </div>
                    <div className="muted ms-ver-fit">Fits servers {v.minServer} to {v.maxServer}{v.size ? ` · ${mb(v.size)}` : ''}</div>
                    {v.override && !v.allowed && <p className="ms-why">Needs an override: {overrideLines(v.override).map((l) => l.reason || l.title).join(' ')}</p>}
                    {v.refused && !v.override && <p className="ms-why">{v.refused}</p>}
                    <ChangeNotes text={v.notes} />
                  </li>
                ))}
              </ul>
            </details>
          )}
        </section>
      )}

      {logOpen && <ModuleLogDialog moduleId={it.id} name={it.name} onClose={() => setLogOpen(false)} />}
      {failure && <FailureDialog failure={failure} name={it.name} moduleId={it.id} repairRoute={repair} onClose={() => setFailure(null)} />}
      {forkAsk && <ForkDialog d={d} onClose={() => setForkAsk(false)} onDone={reload} />}
      {ownerAsk && <ConfirmOwner ask={ownerAsk} onClose={() => setOwnerAsk(null)} />}
    </div>
  )
}

// Along the row by arrows, as a list of pictures isn't a list of stops. At an end the arrow is
// left alone, so a remote can move on.
function shotKeys(e: React.KeyboardEvent<HTMLDivElement>) {
  const by = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0
  const el = e.currentTarget
  if (!by || (by < 0 ? el.scrollLeft <= 1 : el.scrollLeft + el.clientWidth >= el.scrollWidth - 1)) return
  e.preventDefault(); e.stopPropagation()
  el.scrollBy({ left: by * el.clientWidth * 0.8, behavior: 'auto' })
}

function Screenshot({ src, alt }: { src: string; alt: string }) {
  const url = usePicture(src)
  return url ? <img src={url} alt={alt} /> : null
}

// What a module brought with it under other licences, if anything. Asked for only when pressed.
function NoticesButton({ id, name }: { id: string; name: string }) {
  const [text, setText] = useState<string | null>(null)
  const open = async () => {
    try {
      const r = await fetch(`${serverBase()}/api/modules/${encodeURIComponent(id)}/notices`, { headers: { Authorization: 'Bearer ' + getToken() } })
      setText(r.ok ? await r.text() : 'This version lists no third-party licences.')
    } catch { setText('These couldn’t be read just now.') }
  }
  return (
    <>
      <Button size="sm" variant="ghost" onClick={open}>Third-party licences</Button>
      {text !== null && (
        <Dialog title={`${name}: third-party licences`} size="wide" onClose={() => setText(null)} actions={<Button variant="primary" onClick={() => setText(null)}>Close</Button>}>
          <pre className="st-log st-notices" tabIndex={0} aria-label="Third-party licences">{text}</pre>
        </Dialog>
      )}
    </>
  )
}

// Replacing a module this server knows with a store's version of it: its own warning each time.
function ForkDialog({ d, onClose, onDone }: { d: ItemDetail; onClose: () => void; onDone: () => void }) {
  const it = d.item
  const [trust, setTrust] = useState(false)
  const [owner, setOwner] = useState(false)
  // On some systems a fork needs an override too (on macOS, a weaker sandbox): the same request,
  // with the override beside the password.
  const [need, setNeed] = useState<NeedsOverride | null>(null)
  const done = () => { toast(`Installing ${it.name} from ${it.storeName}`); onDone(); onClose() }
  if (need) return <OverrideDialog id={it.id} name={it.name} need={need} permissionWords={Object.fromEntries(d.permissions.map((p) => [p.name, p.words]))} onClose={onClose}
    onSubmit={async (body) => { await useFork(it.store, it.id, true, d.fingerprint, body); done() }} />
  if (owner) return <ConfirmOwner onClose={() => setOwner(false)} ask={{
    title: `Replace ${it.name}?`, action: `Replace ${it.name}`,
    body: <p>This changes which code runs on this server, so it asks for your password. The official version is deleted first, so if this one won’t install, {it.name} is off until you try again or go back to the official version.</p>,
    run: async (password, code) => {
      try { await useFork(it.store, it.id, true, d.fingerprint, { password, code }); done() } catch (e) {
        const n = needsOverrideOf(e)
        if (!n) throw e
        setOwner(false); setNeed(n)
      }
    },
  }} />
  return (
    <Dialog title={`Replace ${it.name} with this store’s version?`} onClose={onClose} size="md" alert
      actions={<><Button variant="ghost" autoFocus onClick={onClose}>Cancel</Button><Button variant="danger" disabled={!trust} onClick={() => setOwner(true)}>Replace {it.name}</Button></>}>
      <div className="st-warn-body">
        <IconShield size={28} />
        <p>{d.forkWarning}</p>
        {d.fingerprint && <p className="st-print">Key fingerprint <code>{d.fingerprint}</code></p>}
        <label className="ms-trust"><input type="checkbox" checked={trust} onChange={(e) => setTrust(e.target.checked)} /> I trust {it.storeName} ({it.storeHost})</label>
      </div>
    </Dialog>
  )
}
