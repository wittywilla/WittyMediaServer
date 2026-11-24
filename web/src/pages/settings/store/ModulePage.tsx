// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, ApiError, getToken, serverBase } from '../../../api'
import { BackLink } from '../../../components/common'
import { IconCheck, IconInfo, IconShield } from '../../../icons'
import type { ModuleReport, VersionInfo } from '../../../moduleStore'
import { toast } from '../../../state'
import { date, t } from '../../../i18n'
import { keepDataChoice } from './keepData'
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
    try { setD(await itemDetail(store, id)); setError(null) } catch (e) { setError(failureOf(e, t('This module'))) }
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
      <BackLink to="/settings/modules/store" label={t('Module store')} />
      <Notice tone="danger" title={error.title} action={<Button size="sm" onClick={load}>{t('Try again')}</Button>}>{error.message}</Notice>
    </div>
  )
  if (!d) return <div className="ms-page st-page"><BackLink to="/settings/modules/store" label={t('Module store')} /><Spinner /></div>
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
    const keep = keepDataChoice(it.name)
    if (!await ask({ title: t('Remove {name}?', { name: it.name }), body: <><p>{t('{name} stops working and stops looking for updates. Your files and what you’ve watched, read or played stay as they are. You can install it again any time.', { name: it.name })}</p>{keep.node}</>, confirm: t('Remove'), danger: true })) return
    await call(() => api.post<ModuleReport>(`/modules/${it.id}/remove${keep.query()}`), t('{name} removed', { name: it.name }))
  }
  const toggle = (on: boolean) => call(() => api.post<ModuleReport>(`/modules/${it.id}/${on ? 'enable' : 'disable'}`), on ? t('{name} turned on', { name: it.name }) : t('{name} turned off', { name: it.name }))
  const rollback = () => {
    setOwnerAsk({
      title: m?.previous ? t('Go back to version {version}?', { version: m.previous }) : t('Stop using {name} {version}?', { name: it.name, version: m?.version }), action: m?.previous ? t('Go back') : t('Stop using it'),
      body: <p>{t('{name} stops using version {version}, and automatic updates won’t bring that version back. You can still choose it yourself.', { name: it.name, version: m?.version })} {m?.previous ? t('An older version may lack security fixes, so this asks for your password.') : t('There’s no earlier version here, so it won’t be working until you install another. This asks for your password.')}</p>,
      run: async (password, code) => { await api.post<ModuleReport>(`/modules/${it.id}/rollback`, { password, code }); toast(m?.previous ? t('{name} went back to version {version}', { name: it.name, version: m.previous }) : t('{name} is no longer in use', { name: it.name })); reload() },
    })
  }
  const official = async () => {
    if (!await ask({ title: t('Go back to the official version?'), body: t('What came from this store is deleted, and {name} isn’t on this server until you install the official one, which is one press from its page.', { name: it.name }), confirm: t('Go back') })) return
    await call(() => api.del<ModuleReport>(`/modules/${it.id}/source`), t('{name} is back to the official version', { name: it.name }))
  }
  const over = useOverrideDialog()
  const permissionWords = Object.fromEntries(d.permissions.map((p) => [p.name, p.words]))
  // A version the usual rules refuse, with an override: the same request again, confirmed.
  const overrideVersion = (version: string, need: NeedsOverride) => over.open({
    id: it.id, name: `${it.name} ${version}`, need: { ...need, version: need.version || version }, permissionWords,
    send: async (body) => { await api.post<ModuleReport>(`/modules/${it.id}/install`, { version, confirm: true, ...body }); toast(t('{name} {version} is being installed', { name: it.name, version })); reload() },
  })
  const goAhead = () => { if (m?.pending) overrideVersion(m.pending.version, { ...m.pending.needsOverride, version: m.pending.version, pending: true }) }
  const chooseVersion = async (v: VersionInfo) => {
    const install = (confirm: boolean, secrets?: { password: string; code: string }) => api.post<ModuleReport>(`/modules/${it.id}/install`, { version: v.version, confirm, ...secrets })
    try { await install(false); reload() } catch (e) {
      const need = needsOverrideOf(e)
      if (need) { overrideVersion(v.version, need); return }
      if (e instanceof ApiError && e.data?.code === 'confirm') {
        if (!await ask({ title: t('Use {name} {version}?', { name: it.name, version: v.version }), body: e.data.warning, confirm: t('Install anyway') })) return
        try { await install(true); reload() } catch (x) {
          const n = needsOverrideOf(x)
          if (n) overrideVersion(v.version, n)
          else if (x instanceof ApiError && x.data?.code === 'password-needed') setOwnerAsk({
            title: t('Go back to {name} {version}?', { name: it.name, version: v.version }), action: t('Go back'),
            body: <p>{t('An older version may lack security fixes, so this asks for your password.')}</p>,
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
      <BackLink to="/settings/modules/store" label={t('Module store')} />

      <header className="st-mod-head">
        <ModuleIcon item={it} size={96} />
        <div className="st-mod-title">
          <h1 id="st-module-h" tabIndex={-1}>{it.name}</h1>
          <p className="st-mod-by">{it.publisher?.url ? <a href={it.publisher.url} target="_blank" rel="noreferrer noopener">{it.publisher.name}</a> : it.publisher?.name || t('Part of this server')} · {categoryTitle(it.group)}</p>
          <StateTags item={it} />
        </div>
        <div className="st-mod-act">
          <ActionButton it={it} press={press} working={installing} primary />
          {it.kind === 'fork' && !usingThisFork && it.fit.ok && <Button variant="danger" onClick={() => setForkAsk(true)}>{t('Use this fork…')}</Button>}
        </div>
      </header>

      <InPlaceProgress it={it} />
      {dialogs}
      {over.dialog}

      {m && <PendingNotice m={m} onGo={goAhead} onDropped={reload} />}
      <StatusNotice status={it.releaseStatus || m?.releaseStatus} note={it.statusNote || m?.statusNote} name={it.name} installed={installedHere} />
      {(it.override || m?.override) && <OverrideRecordBox id={it.id} name={it.name} record={(it.override || m?.override)!} disabled={working} onRemoved={reload} />}

      {third && (
        <Notice tone="warn" title={t('Third-party: from {store} ({host})', { store: it.storeName, host: it.storeHost })}>
          {t('Nobody at this project has reviewed this. It comes from a store you added, and whoever runs that store holds the key it’s signed with.')} <Link to="/settings/modules/stores">{t('Manage stores')}</Link>
          {d.fingerprint && <span className="st-print">{t('Key fingerprint')} <code>{d.fingerprint}</code></span>}
        </Notice>
      )}
      {it.lookalike && <Notice tone="danger" title={t('This uses the name of another module')}>{t('It calls itself {name}, the name of a module from this project, and it isn’t that module. Don’t assume it does what that one does.', { name: it.name })}</Notice>}
      {it.kind === 'in-development' && <Notice title={t('A module this server doesn’t know')} action={<InstallFromFile onInstalled={reload} />}>{it.why} {t('If you have its release archive and trust whoever made it, it can be installed from the file, with an override.')}</Notice>}
      {it.kind === 'refused' && <Notice tone="danger" title={t('This can’t be used')}>{it.why}</Notice>}
      {!it.fit.ok && it.kind !== 'in-development' && it.kind !== 'refused' && <Notice tone={it.fit.kind === 'library' ? undefined : 'warn'} title={it.fit.kind === 'too-old' ? t('Built for an older server') : it.fit.kind === 'older-server' ? t('Made for an older server') : it.fit.kind === 'too-new' ? t('Needs a newer server') : it.fit.kind === 'library' ? t('Comes with other modules') : t('Can’t be installed here')}>{it.fit.why}{it.fit.kind === 'older-server' && !installedHere && ` ${t('Installing it asks you first.')}`}</Notice>}
      {m?.olderServer && m.olderWarning && it.fit.kind !== 'older-server' && <Notice tone="warn" title={t('Made for an older server')}>{m.olderWarning}</Notice>}
      {it.failure && <FailureNotice failure={it.failure} name={it.name} moduleId={it.id} repairRoute={repair} onRetry={act ? press : undefined} />}
      {m?.notice && !it.failure && <Notice tone="warn">{m.notice}</Notice>}

      {it.description || it.summary ? <section className="st-mod-sec" aria-labelledby="st-about"><h2 id="st-about">{t('About')}</h2>{(it.description || it.summary || '').split(/\n{2,}/).map((p, i) => <p key={i}>{p}</p>)}</section> : null}

      {!!it.screenshots?.length && (
        <section className="st-mod-sec" aria-labelledby="st-shots">
          <h2 id="st-shots">{t('Screenshots')}</h2>
          <div className="st-shots" tabIndex={0} role="group" aria-label={t('Screenshots of {name}', { name: it.name })} onKeyDown={shotKeys}>
            {it.screenshots.map((src, i) => <Screenshot key={src} src={src} alt={t('{name}, screenshot {n} of {total}', { name: it.name, n: i + 1, total: it.screenshots!.length })} />)}
          </div>
        </section>
      )}

      <WhatItAdds ui={it.ui || m?.uiAdds} integration={installedHere && m?.integrationAdds ? m.integrationAdds : it.integration || m?.integrationAdds} id={it.id} libraries={it.libraries} name={it.name} />

      <section className="st-mod-sec" aria-labelledby="st-new">
        <h2 id="st-new">{t('What’s new')}{(m?.latest?.version || it.version) ? <span className="ms-ver"> {m?.latest?.version || it.version}</span> : null}</h2>
        {it.released && !m?.latest && <p className="muted st-news-day">{t('Released {day}', { day: dayOf(it.released) })}</p>}
        <ChangeNotes text={news} developer />
      </section>

      <section className="st-mod-sec" aria-labelledby="st-asks">
        <h2 id="st-asks">{t('What it asks for')}</h2>
        <SpecPage bare page={permissionsSpec(d.permissions)} />
        <p className="muted">{t('Whatever it asks for, a module can read what this server’s account can read, and the server’s own limits apply to it.')} <Link to="/help/module-store#what-limits-a-module">{t('See what limits a module.')}</Link></p>
      </section>

      <section className="st-mod-sec" aria-labelledby="st-facts">
        <h2 id="st-facts">{t('Details')}</h2>
        <SpecPage bare page={detailsSpec([
          { title: installedHere ? t('In use') : t('Version'), value: installedHere ? m!.version : it.version || t('Not released yet') },
          ...(installedHere && m!.latest ? [{ title: t('Newest'), value: m!.latest.version }] : []),
          ...(it.size ? [{ title: t('Download'), value: mb(it.size) }] : []),
          { title: t('Publisher'), value: it.publisher?.name || t('This server') },
          ...(moduleKindTitle(itemModuleKind(it)) ? [{ title: t('Kind'), value: moduleKindTitle(itemModuleKind(it))! }] : []),
          ...(it.license || m?.license ? [{ title: t('Licence'), value: (it.license || m?.license)! }] : []),
          ...(it.released ? [{ title: t('Released'), value: dayOf(it.released) }] : []),
          ...(it.minServer ? [{ title: t('Fits servers'), value: it.fit.ok ? t('{min} to {max}, which includes this server', { min: it.minServer, max: it.maxServer }) : t('{min} to {max}', { min: it.minServer, max: it.maxServer }) }] : []),
          ...(it.platforms?.length ? [{ title: t('Works on'), value: it.platforms.map(platformName).join(', ') }] : []),
          ...(it.needs?.length ? [{ title: t('What it needs'), value: it.needs.join(', ') }] : []),
          ...(it.repo ? [{ title: t('Source'), value: `github.com/${it.repo}` }] : []),
        ], d.builtOn.map((r) => ({ id: r.id, name: r.installed ? t('{name} (installed)', { name: r.name }) : r.name, to: itemPath({ official: true, store: 'official', id: r.id, kind: 'official' }) })))} />
        {(it.updates || it.stays) && (
          <div className="ms-how">
            <b>{t('What an update changes')}</b>
            {it.updates && <p>{it.updates}</p>}
            {it.stays && <p><span className="ms-how-sub">{t('Only a new server changes:')}</span> {it.stays}</p>}
          </div>
        )}
        <PolicyNote view={{ policy: d.policy, server: d.server }} />
      </section>

      {(installedHere || d.versions.length > 0) && (
        <section className="st-mod-sec" aria-labelledby="st-manage">
          <h2 id="st-manage">{t('Manage')}</h2>
          <SpecPage bare page={managePage} handled={handledIds(managePage)} after={{ 0: installedHere ? <NoticesButton id={it.id} name={it.name} /> : null }}
            onAction={(id) => { if (id === 'rollback') rollback(); else if (id === 'off') void toggle(false); else if (id === 'on') void toggle(true); else if (id === 'remove') void remove(); else if (id === 'official') void official(); else if (id === 'log') setLogOpen(true) }} />
          {m?.core && <p className="muted">{t('Other modules are built on this one, so it stays.')}</p>}
          {d.versions.length > 0 && (
            <details className="ms-more">
              <summary>{t('All versions ({count})', { count: d.versions.length })}</summary>
              <ul className="ms-vers st-vers">
                {d.versions.map((v) => (
                  <li key={v.version} className={'ms-ver-row' + (v.inUse ? ' on' : '') + (v.withdrawn ? ' withdrawn' : '') + (!v.inUse && !v.allowed ? ' no' : '')}>
                    <div className="ms-ver-head">
                      <div className="ms-ver-title">
                        <b>{v.version}</b>
                        {v.published ? <span className="muted">{date(v.published, { dateStyle: 'medium' })}</span> : null}
                        <span className="ms-tags">
                          {v.inUse && <Tag tone="ok" icon={<IconCheck size={12} />}>{t('In use')}</Tag>}
                          {v.prerelease && <Tag>{t('Pre-release')}</Tag>}
                          {v.withdrawn && <Tag tone="bad" icon={<IconInfo size={12} />}>{t('Withdrawn')}</Tag>}
                        </span>
                      </div>
                      {!v.inUse && installedHere && (!third || usingThisFork) && <Button size="sm" disabled={!(v.allowed || v.override) || working} aria-label={!v.allowed && v.override ? t('Use version {version}, with an override', { version: v.version }) : t('Use version {version}', { version: v.version })} onClick={() => chooseVersion(v)}>{!v.allowed && v.override ? t('Install anyway…') : v.installed ? t('Switch to this') : t('Install')}</Button>}
                    </div>
                    <div className="muted ms-ver-fit">{t('Fits servers {min} to {max}', { min: v.minServer, max: v.maxServer })}{v.size ? ` · ${mb(v.size)}` : ''}</div>
                    {v.override && !v.allowed && <p className="ms-why">{t('Needs an override:')} {overrideLines(v.override).map((l) => l.reason || l.title).join(' ')}</p>}
                    {v.refused && !v.override && <p className="ms-why">{v.refused}</p>}
                    <ChangeNotes text={v.notes} developer />
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
      setText(r.ok ? await r.text() : t('This version lists no third-party licences.'))
    } catch { setText(t('These couldn’t be read just now.')) }
  }
  return (
    <>
      <Button size="sm" variant="ghost" onClick={open}>{t('Third-party licences')}</Button>
      {text !== null && (
        <Dialog title={t('{name}: third-party licences', { name })} size="wide" onClose={() => setText(null)} actions={<Button variant="primary" onClick={() => setText(null)}>{t('Close')}</Button>}>
          <pre className="st-log st-notices" tabIndex={0} aria-label={t('Third-party licences')}>{text}</pre>
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
  const done = () => { toast(t('Installing {name} from {store}', { name: it.name, store: it.storeName })); onDone(); onClose() }
  if (need) return <OverrideDialog id={it.id} name={it.name} need={need} permissionWords={Object.fromEntries(d.permissions.map((p) => [p.name, p.words]))} onClose={onClose}
    onSubmit={async (body) => { await useFork(it.store, it.id, true, d.fingerprint, body); done() }} />
  if (owner) return <ConfirmOwner onClose={() => setOwner(false)} ask={{
    title: t('Replace {name}?', { name: it.name }), action: t('Replace {name}', { name: it.name }),
    body: <p>{t('This changes which code runs on this server, so it asks for your password. The official version is deleted first, so if this one won’t install, {name} is off until you try again or go back to the official version.', { name: it.name })}</p>,
    run: async (password, code) => {
      try { await useFork(it.store, it.id, true, d.fingerprint, { password, code }); done() } catch (e) {
        const n = needsOverrideOf(e)
        if (!n) throw e
        setOwner(false); setNeed(n)
      }
    },
  }} />
  return (
    <Dialog title={t('Replace {name} with this store’s version?', { name: it.name })} onClose={onClose} size="md" alert
      actions={<><Button variant="ghost" autoFocus onClick={onClose}>{t('Cancel')}</Button><Button variant="danger" disabled={!trust} onClick={() => setOwner(true)}>{t('Replace {name}', { name: it.name })}</Button></>}>
      <div className="st-warn-body">
        <IconShield size={28} />
        <p>{d.forkWarning}</p>
        {d.fingerprint && <p className="st-print">{t('Key fingerprint')} <code>{d.fingerprint}</code></p>}
        <label className="ms-trust"><input type="checkbox" checked={trust} onChange={(e) => setTrust(e.target.checked)} /> {t('I trust {store} ({host})', { store: it.storeName, host: it.storeHost })}</label>
      </div>
    </Dialog>
  )
}
