import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, ApiError, mediaUrl } from '../../../api'
import { BackLink } from '../../../components/common'
import { IconCheck, IconInfo, IconShield } from '../../../icons'
import type { ModuleReport, VersionInfo } from '../../../moduleStore'
import { toast } from '../../../state'
import { ask, Button, Dialog, Notice, Spinner } from '../../../ui'
import { actionFor, ActionButton, InPlaceProgress, useInstall } from './ItemCard'
import { ChangeNotes, FailureDialog, FailureNotice, itemPath, ModuleIcon, ModuleLogDialog, PolicyNote, StateTags, Tag } from './parts'
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
  const rollback = async () => {
    const to = m?.previous ? `version ${m.previous}` : 'the version before'
    if (!await ask({ title: `Go back to ${to}?`, body: `${it.name} stops using version ${m?.version}, and automatic updates won't bring that version back. You can still choose it yourself.`, confirm: 'Go back' })) return
    await call(() => api.post<ModuleReport>(`/modules/${it.id}/rollback`), `${it.name} went back`)
  }
  const official = async () => {
    if (!await ask({ title: 'Go back to the official version?', body: `What came from this store is deleted, and ${it.name} isn’t on this server until you install the official one, which is one press from its page.`, confirm: 'Go back' })) return
    await call(() => api.del<ModuleReport>(`/modules/${it.id}/source`), `${it.name} is back to the official version`)
  }
  const chooseVersion = async (v: VersionInfo) => {
    const install = (confirm: boolean) => api.post<ModuleReport>(`/modules/${it.id}/install`, { version: v.version, confirm })
    try { await install(false); reload() } catch (e) {
      if (e instanceof ApiError && e.data?.code === 'confirm') {
        if (await ask({ title: `Use ${it.name} ${v.version}?`, body: e.data.warning, confirm: 'Install anyway' })) await call(() => install(true))
      } else setFailure(failureOf(e, it.name))
    }
  }

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

      {third && (
        <Notice tone="warn" title={`Third-party: from ${it.storeName} (${it.storeHost})`}>
          Nobody at this project has reviewed this. It comes from a store you added, and whoever runs that store holds the key it’s signed with. <Link to="/settings/modules/stores">Manage stores</Link>
          {d.fingerprint && <span className="st-print">Key fingerprint <code>{d.fingerprint}</code></span>}
        </Notice>
      )}
      {it.lookalike && <Notice tone="danger" title="This uses the name of another module">It calls itself {it.name}, the name of a module from this project, and it isn’t that module. Don’t assume it does what that one does.</Notice>}
      {it.kind === 'in-development' && <Notice title="Custom modules are in development">{it.why}</Notice>}
      {it.kind === 'refused' && <Notice tone="danger" title="This can’t be used">{it.why}</Notice>}
      {!it.fit.ok && it.kind !== 'in-development' && it.kind !== 'refused' && <Notice tone="warn" title={it.fit.kind === 'too-old' ? 'Built for an older server' : it.fit.kind === 'too-new' ? 'Needs a newer server' : 'Can’t be installed here'}>{it.fit.why}</Notice>}
      {it.failure && <FailureNotice failure={it.failure} name={it.name} moduleId={it.id} repairRoute={repair} onRetry={act ? press : undefined} />}
      {m?.notice && !it.failure && <Notice tone="warn">{m.notice}</Notice>}

      {it.description || it.summary ? <section className="st-mod-sec" aria-labelledby="st-about"><h2 id="st-about">About</h2>{(it.description || it.summary || '').split(/\n{2,}/).map((p, i) => <p key={i}>{p}</p>)}</section> : null}

      {!!it.screenshots?.length && (
        <section className="st-mod-sec" aria-labelledby="st-shots">
          <h2 id="st-shots">Screenshots</h2>
          <div className="st-shots" tabIndex={0} role="group" aria-label={`Screenshots of ${it.name}`}>
            {it.screenshots.map((src, i) => <img key={src} src={mediaUrl(src)} alt={`${it.name}, screenshot ${i + 1} of ${it.screenshots!.length}`} loading="lazy" />)}
          </div>
        </section>
      )}

      <section className="st-mod-sec" aria-labelledby="st-new">
        <h2 id="st-new">What’s new{(m?.latest?.version || it.version) ? <span className="ms-ver"> {m?.latest?.version || it.version}</span> : null}</h2>
        {it.released && !m?.latest && <p className="muted st-news-day">Released {dayOf(it.released)}</p>}
        <ChangeNotes text={news} />
      </section>

      <section className="st-mod-sec" aria-labelledby="st-asks">
        <h2 id="st-asks">What it asks for</h2>
        {d.permissions.length ? (
          <ul className="st-perms">{d.permissions.map((p) => <li key={p.name}><IconInfo size={16} /><span><b>{p.name}</b> {p.words}</span></li>)}</ul>
        ) : <p>No special permissions.</p>}
        <p className="muted">Whatever it asks for, a module can read what this server’s account can read, and the server’s own limits apply to it. See <a href="https://github.com/wittywilla/WittyMediaServer/blob/master/docs/MODULES.md#the-helper-protocol-module-api-1" target="_blank" rel="noreferrer noopener">what limits a module</a>.</p>
      </section>

      <section className="st-mod-sec" aria-labelledby="st-facts">
        <h2 id="st-facts">Details</h2>
        <dl className="ms-facts">
          <div><dt>{installedHere ? 'In use' : 'Version'}</dt><dd>{installedHere ? m!.version : it.version || 'Not released yet'}</dd></div>
          {installedHere && m!.latest && <div><dt>Newest</dt><dd>{m!.latest.version}</dd></div>}
          {!!it.size && <div><dt>Download</dt><dd>{mb(it.size)}</dd></div>}
          <div><dt>Publisher</dt><dd>{it.publisher?.name || 'This server'}</dd></div>
          {(it.license || m?.license) && <div><dt>Licence</dt><dd>{it.license || m?.license}</dd></div>}
          {it.released && <div><dt>Released</dt><dd>{dayOf(it.released)}</dd></div>}
          {it.minServer && <div className="wide"><dt>Fits servers</dt><dd>{it.minServer} to {it.maxServer}{it.fit.ok ? ', which includes this server' : ''}</dd></div>}
          {!!it.platforms?.length && <div className="wide"><dt>Works on</dt><dd>{it.platforms.map(platformName).join(', ')}</dd></div>}
          {d.builtOn.length > 0 && <div className="wide"><dt>Built on</dt><dd>{d.builtOn.map((r, i) => <span key={r.id}>{i > 0 && ', '}<Link to={itemPath({ official: true, store: 'official', id: r.id, kind: 'official' })}>{r.name}</Link>{r.installed ? ' (installed)' : ''}</span>)}</dd></div>}
          {!!it.needs?.length && <div className="wide"><dt>What it needs</dt><dd>{it.needs.join(', ')}</dd></div>}
          {it.repo && <div className="wide"><dt>Source</dt><dd><a href={`https://github.com/${it.repo}`} target="_blank" rel="noreferrer noopener">{it.repo}</a></dd></div>}
        </dl>
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
          <div className="ms-actions">
            {installedHere && m!.canRollback && <Button size="sm" disabled={working} onClick={rollback}>{m!.previous ? `Go back to ${m!.previous}` : 'Go back to the version before'}</Button>}
            {installedHere && !m!.core && m!.enabled !== false && <Button size="sm" disabled={working} onClick={() => toggle(false)}>Turn off</Button>}
            {installedHere && m!.enabled === false && <Button size="sm" disabled={working} onClick={() => toggle(true)}>Turn on</Button>}
            {installedHere && !m!.core && m!.delivery !== 'builtin' && <Button size="sm" variant="ghost" className="danger-hover" disabled={working} onClick={remove}>Remove</Button>}
            {usingThisFork && <Button size="sm" disabled={working} onClick={official}>Go back to the official version</Button>}
            <Button size="sm" variant="ghost" onClick={() => setLogOpen(true)}>Show log</Button>
          </div>
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
                        {v.published ? <span className="muted">{new Date(v.published).toLocaleDateString()}</span> : null}
                        <span className="ms-tags">
                          {v.inUse && <Tag tone="ok" icon={<IconCheck size={12} />}>In use</Tag>}
                          {v.prerelease && <Tag>Pre-release</Tag>}
                          {v.withdrawn && <Tag tone="bad" icon={<IconInfo size={12} />}>Withdrawn</Tag>}
                        </span>
                      </div>
                      {!v.inUse && installedHere && (!third || usingThisFork) && <Button size="sm" disabled={!v.allowed || working} aria-label={`Use version ${v.version}`} onClick={() => chooseVersion(v)}>{v.installed ? 'Switch to this' : 'Install'}</Button>}
                    </div>
                    <div className="muted ms-ver-fit">Fits servers {v.minServer} to {v.maxServer}{v.size ? ` · ${mb(v.size)}` : ''}</div>
                    {v.refused && <p className="ms-why">{v.refused}</p>}
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
    </div>
  )
}

// Replacing a module this server knows with a store's version of it: its own warning each time.
function ForkDialog({ d, onClose, onDone }: { d: ItemDetail; onClose: () => void; onDone: () => void }) {
  const it = d.item
  const [trust, setTrust] = useState(false)
  const [working, setWorking] = useState(false)
  const [failure, setFailure] = useState<Failure | null>(null)
  const go = async () => {
    setWorking(true)
    try { await useFork(it.store, it.id, true); toast(`${it.name} now comes from ${it.storeName}`); onDone(); onClose() } catch (e) { setFailure(failureOf(e, it.name)); setWorking(false) }
  }
  if (failure) return <FailureDialog failure={failure} name={it.name} moduleId={it.id} repairRoute="/settings/repair" onClose={onClose} />
  return (
    <Dialog title={`Replace ${it.name} with this store’s version?`} onClose={onClose} size="md" alert
      actions={<><Button variant="ghost" autoFocus onClick={onClose}>Cancel</Button><Button variant="danger" aria-busy={working || undefined} disabled={!trust} onClick={working ? undefined : go}>Replace {it.name}</Button></>}>
      <div className="st-warn-body">
        <IconShield size={28} />
        <p>{d.forkWarning}</p>
        {d.fingerprint && <p className="st-print">Key fingerprint <code>{d.fingerprint}</code></p>}
        <label className="ms-trust"><input type="checkbox" checked={trust} onChange={(e) => setTrust(e.target.checked)} /> I trust {it.storeName} ({it.storeHost})</label>
      </div>
    </Dialog>
  )
}
