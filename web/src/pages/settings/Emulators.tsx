// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState, type RefObject } from 'react'
import { api, fmtDateTime, profileName, type UserView } from '../../api'
import { LoadError, Spinner } from '../../components/common'
import { WebLink } from '../../components/WebLink'
import { IconCheck, IconInfo, IconRefresh } from '../../icons'
import { toast } from '../../state'
import { ask, Dialog, Input, Notice, Progress, Select, Switch } from '../../ui'
import { useConsoles } from '../games/consoles'
import { emulatorsApi } from '../games/emulatorsApi'
import { fmtBytes, hostOf, joinArgs, splitArgs, type OwnProgram, jobProgress, requiresWords, same, sinceWords, wholeSettings, type BackendSettings, type CatalogueEntry, type DiagnosticCheck, type Diagnostics, type DockerStatus, type RunningSession, type Values } from '../games/emulatorsLogic'
import { SchemaForm } from '../games/SchemaForm'
import { PageHead, Row, SaveBar, Section } from './ui'
import '../games/games-ui.css'
import '../games/emulators.css'

const EmulatorSettingsDialog = lazy(() => import('../games/EmulatorSettings'))

// Settings, Standalone emulators. Some consoles have no core, only an emulator that is a whole
// program of its own. WittyPlay can run those in a container (or, on Linux, a sandbox without
// one) and stream them like any other game. This page is where an administrator turns that on,
// sees whether this computer is up to it, and installs the emulators.
//
// An emulator is someone else's program, so nothing is installed without being asked for, with
// its licence in view first.

const DOCS = 'https://github.com/wittywilla/WittyPlay/blob/master/docs/DOCKER-BACKEND.md'
const JOB_MS = 1000 // an install is asked after once a second, never faster
const SESSIONS_MS = 5000

// Whether a part of the page is there to be looked at: on the screen, in a tab that's showing.
function useVisible(ref: RefObject<HTMLElement | null>) {
  const [on, setOn] = useState(false)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    let inView = typeof IntersectionObserver === 'undefined'
    const update = () => setOn(inView && !document.hidden)
    const watch = typeof IntersectionObserver === 'undefined' ? null : new IntersectionObserver((entries) => { inView = entries.some((e) => e.isIntersecting); update() })
    watch?.observe(el)
    document.addEventListener('visibilitychange', update)
    update()
    return () => { watch?.disconnect(); document.removeEventListener('visibilitychange', update) }
  }, [ref])
  return on
}

// Asks again every ms, counted from when the last answer came, so a slow server is never asked
// faster than it answers. Nothing is asked while on is false.
function usePoll(run: () => Promise<unknown>, ms: number, on: boolean) {
  const latest = useRef(run)
  latest.current = run
  useEffect(() => {
    if (!on) return
    let timer = 0, live = true
    const wait = () => { timer = window.setTimeout(() => { latest.current().catch(() => {}).finally(() => { if (live) wait() }) }, ms) }
    wait()
    return () => { live = false; window.clearTimeout(timer) }
  }, [ms, on])
}

const CHECK_WORDS: Record<DiagnosticCheck['state'], string> = { ok: 'Fine.', warn: 'Worth a look.', fail: 'Not working.', off: 'Off.' }

export default function EmulatorsSettings() {
  const page = useRef<HTMLDivElement>(null)
  const [status, setStatus] = useState<DockerStatus | null>(null)
  const [checks, setChecks] = useState<Diagnostics | null>(null)
  const [list, setList] = useState<CatalogueEntry[] | null>(null)
  const [listError, setListError] = useState('')
  const [failed, setFailed] = useState(false)
  const [refreshing, setRefreshing] = useState(false)

  const loadList = useCallback(() => emulatorsApi.catalogue().then((l) => { setList(l || []); setListError('') }).catch((e) => { setListError(e?.message || 'The list couldn’t be loaded.') }), [])
  const loadStatus = useCallback(async () => {
    try { setStatus(await emulatorsApi.status()); setFailed(false) } catch { setFailed(true) }
    // The checks are a help: the page stands without them.
    await emulatorsApi.diagnostics().then(setChecks).catch(() => {})
  }, [])
  const refresh = useCallback(async () => {
    setRefreshing(true)
    await Promise.all([loadStatus(), loadList()])
    setRefreshing(false)
  }, [loadStatus, loadList])
  useEffect(() => { void refresh() }, [refresh])

  if (failed && !status) return <LoadError what="This page" onRetry={() => { void refresh() }} />
  if (!status) return <Spinner />
  return (
    <div ref={page}>
      <PageHead title="Standalone emulators" desc="Some consoles are played by an emulator that is a whole program of its own, not a core: Xbox, PlayStation 3 and Wii U, and others where you’d rather. This server can run those too, boxed in on this computer or in a container, and stream them like any other game. It’s off until you turn it on." />
      <Section title="Where emulators run" desc="Whether this computer can run emulators, and what would run them."
        actions={<button type="button" className="btn sm" disabled={refreshing} onClick={() => { void refresh() }}><IconRefresh size={16} /> {refreshing ? 'Checking…' : 'Refresh'}</button>}>
        <StatusFacts status={status} checks={checks} />
      </Section>
      <BackendForm onSaved={refresh} />
      <Catalogue list={list} error={listError} reload={loadList} setList={setList} page={page} />
      <OwnPrograms />
      <Sessions />
    </div>
  )
}

function StatusFacts({ status, checks }: { status: DockerStatus; checks: Diagnostics | null }) {
  const docker = status.available
    ? [status.apiVersion ? `API ${status.apiVersion}` : '', [status.os, status.arch].filter(Boolean).join(' '), status.rootless === true ? 'rootless' : status.rootless === false ? 'runs as root' : ''].filter(Boolean).join(' · ') || 'Reachable'
    : null
  return (
    <>
      <dl className="emu-facts">
        <dt>Standalone emulators</dt>
        <dd><span className={'gm-pill ' + (status.enabled ? 'ok' : 'off')}>{status.enabled ? <IconCheck size={14} /> : <IconInfo size={14} />} {status.enabled ? 'On' : 'Off'}</span></dd>
        <dt>A game would run in</dt>
        <dd>{status.runtime === 'docker' ? 'A Docker container' : status.runtime === 'host' ? 'This computer’s own desktop, boxed in by its own sandbox' : 'The local sandbox, without Docker'}</dd>
        <dt>Docker</dt>
        <dd>
          {status.available ? <><b>Reachable</b>{docker && docker !== 'Reachable' ? ` · ${docker}` : ''}</> : <b>Can’t be reached</b>}
          {status.host && <small>At <code>{status.host}</code></small>}
          {status.error && <small>{status.error}</small>}
        </dd>
        <dt>Local sandbox</dt>
        <dd>{status.local.available ? 'Available' : 'Not available here'}{status.local.why && <small>{status.local.why}</small>}</dd>
        {status.desktop && <><dt>On this computer’s desktop</dt>
          <dd>{status.desktop.available ? 'Available' : 'Not available here'}{status.desktop.why && <small>{status.desktop.why}</small>}</dd></>}
        <dt>Containers running</dt>
        <dd>{status.containers}</dd>
        {checks && <><dt>This computer</dt><dd>{[[checks.host.os, checks.host.arch].filter(Boolean).join(' '), checks.host.cpus ? `${checks.host.cpus} processor ${checks.host.cpus === 1 ? 'thread' : 'threads'}` : '', checks.host.memoryMB ? `${checks.host.memoryMB >= 1024 ? `${+(checks.host.memoryMB / 1024).toFixed(1)} GB` : `${checks.host.memoryMB} MB`} of memory` : ''].filter(Boolean).join(' · ')}</dd></>}
      </dl>
      {checks && checks.checks.length > 0 && (
        <ul className="gm-checks" aria-label="Checks">
          {checks.checks.map((c) => (
            <li key={c.id} className={c.state === 'ok' ? 'ok' : c.state === 'fail' ? 'no' : c.state}>
              {c.state === 'ok' ? <IconCheck size={16} /> : <IconInfo size={16} />}
              <span><b>{c.label}</b><small>{CHECK_WORDS[c.state] || ''} {c.detail}</small></span>
            </li>
          ))}
        </ul>
      )}
    </>
  )
}

// The backend's own settings, drawn from what the module says they are and saved whole.
function BackendForm({ onSaved }: { onSaved: () => Promise<unknown> }) {
  const [data, setData] = useState<BackendSettings | null>(null)
  const [draft, setDraft] = useState<Values>({})
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [failed, setFailed] = useState('')
  const load = useCallback(() => emulatorsApi.settings().then((d) => { setData(d); setFailed('') }).catch((e) => setFailed(e?.message || 'These settings couldn’t be loaded.')), [])
  useEffect(() => { void load() }, [load])

  const saved = data?.settings || {}
  const now = useMemo(() => ({ ...saved, ...draft }), [saved, draft])
  const dirty = !!data && !same(wholeSettings(data.schema, saved, draft), wholeSettings(data.schema, saved, {}))
  const change = (key: string, value: unknown) => { setError(''); setDraft((d) => ({ ...d, [key]: value })) }
  const save = async () => {
    if (!data) return
    setBusy(true)
    setError('')
    try {
      const kept = await emulatorsApi.saveSettings(wholeSettings(data.schema, saved, draft))
      setData({ ...data, settings: kept || {} })
      setDraft({})
      toast('Settings saved')
      // What can run, and what's installable, follows from these.
      void onSaved()
    } catch (e: any) {
      setError(e?.message || 'That couldn’t be saved.')
    } finally { setBusy(false) }
  }
  // The switch that turns it all on comes first, with what turning it on means beside it.
  const first = data?.schema.filter((f) => f.key === 'docker.enabled') || []
  // A container's own settings mean nothing to an emulator on this computer, and are left out.
  const runtime = now['docker.runtime'] ?? data?.schema.find((f) => f.key === 'docker.runtime')?.default
  const containerOnly = ['docker.network', 'docker.internal', 'docker.publishLoopback', 'docker.gpu', 'docker.images']
  const rest = data?.schema.filter((f) => f.key !== 'docker.enabled' && (runtime === 'docker' || !containerOnly.includes(f.key))) || []
  const form = (schema: BackendSettings['schema']) => <SchemaForm schema={schema} values={now} onChange={change} under={(f) => f.default} disabled={busy} />
  return (
    <Section title="How emulators run" desc="What an emulator may use, and how its picture reaches you.">
      {failed ? <Notice tone="danger" action={<button type="button" className="btn sm" onClick={() => { void load() }}>Try again</button>}>{failed}</Notice>
        : !data ? <Spinner />
        : (
          <>
            {first.length > 0 && form(first)}
            {runtime === 'docker' && <div className="emu-warning">
              <Notice tone="warn" title="Access to Docker is the same as root on this computer">
                <p>Whoever can reach Docker’s socket can do anything on this computer that its administrator can. Turning this on gives this server that reach, so only turn it on for a server you run yourself. Rootless Docker, or Podman, is the safer way: the emulators then run without root at all. <WebLink href={DOCS}>How the Docker backend works, and how to set up rootless Docker</WebLink>.</p>
              </Notice>
            </div>}
            {rest.length > 0 && form(rest)}
            {error && <div className="emu-form-error"><Notice tone="danger">{error}</Notice></div>}
            <SaveBar dirty={dirty} busy={busy} save={() => { void save() }} discard={() => { setDraft({}); setError('') }} />
          </>
        )}
    </Section>
  )
}

function Catalogue({ list, error, reload, setList, page }: { list: CatalogueEntry[] | null; error: string; reload: () => Promise<unknown>; setList: (f: (l: CatalogueEntry[] | null) => CatalogueEntry[] | null) => void; page: RefObject<HTMLElement | null> }) {
  const consoles = useConsoles()
  const [confirm, setConfirm] = useState<CatalogueEntry | null>(null)
  const [settings, setSettings] = useState<CatalogueEntry | null>(null)
  const [busy, setBusy] = useState('')
  const [problems, setProblems] = useState<Record<string, string>>({})
  const visible = useVisible(page)
  const problem = (id: string, text: string) => setProblems((p) => ({ ...p, [id]: text }))

  // Installs under way are asked after once a second, one at a time, while the page is in view.
  const running = (list || []).filter((e) => e.job?.state === 'running').map((e) => e.id)
  usePoll(async () => {
    let finished = false
    for (const id of running) {
      try {
        const job = await emulatorsApi.installJob(id)
        if (job.state !== 'running') finished = true
        setList((l) => l && l.map((e) => (e.id === id ? { ...e, job } : e)))
      } catch { finished = true } // no longer there to ask about: the list says what became of it
    }
    if (finished) await reload()
  }, JOB_MS, visible && running.length > 0)

  const install = async (e: CatalogueEntry) => {
    setConfirm(null)
    setBusy(e.id)
    problem(e.id, '')
    try {
      // The licence that was on the screen goes with the request.
      const job = await emulatorsApi.install(e.id, e.license)
      setList((l) => l && l.map((x) => (x.id === e.id ? { ...x, job } : x)))
    } catch (err: any) { problem(e.id, err?.message || 'The install couldn’t start.') } finally { setBusy('') }
  }
  const cancel = async (e: CatalogueEntry) => {
    setBusy(e.id)
    try { await emulatorsApi.cancelInstall(e.id); await reload() } catch (err: any) { problem(e.id, err?.message || 'That couldn’t be cancelled.') } finally { setBusy('') }
  }
  const remove = async (e: CatalogueEntry) => {
    if (!await ask({ title: `Remove ${e.name}?`, body: 'The emulator is taken off this server. Your games, saves and BIOS files stay where they are, and it can be installed again.', confirm: 'Remove', danger: true })) return
    setBusy(e.id)
    problem(e.id, '')
    try { await emulatorsApi.remove(e.id); toast(`${e.name} removed`); await reload() } catch (err: any) { problem(e.id, err?.message || 'It couldn’t be removed.') } finally { setBusy('') }
  }
  const short = (id: string) => consoles.find((c) => c.id === id)?.short || id.toUpperCase()

  return (
    <Section title="Emulators" desc="Each is its own project’s program, under its own licence. Installing one downloads it from that project. No games or BIOS files come with any of them.">
      {error && <Notice tone="danger" action={<button type="button" className="btn sm" onClick={() => { void reload() }}>Try again</button>}>{error}</Notice>}
      {!list && !error && <Spinner />}
      {list && list.length === 0 && <p className="muted">There are no emulators in this server’s catalogue.</p>}
      {list && list.length > 0 && (
        <div className="gm-setups">
          {list.map((e) => {
            const job = e.job?.state === 'running' ? e.job : null
            const progress = job ? jobProgress(job) : null
            const needs = requiresWords(e.requires)
            const working = busy === e.id
            return (
              <article key={e.id} className="gm-setup" aria-label={e.name}>
                <header className="gm-setup-head">
                  <div className="gm-setup-name"><h3>{e.name}</h3><small>Version {e.version}{e.systems.length > 0 && ` · ${e.systems.map(short).join(', ')}`}</small></div>
                  {e.installed
                    ? <span className={'gm-pill ' + (e.runnable ? 'ok' : 'warn')}>{e.runnable ? <IconCheck size={14} /> : <IconInfo size={14} />} {e.runnable ? 'Installed' : 'Installed, can’t run yet'}</span>
                    : <span className="gm-pill off"><IconInfo size={14} /> Not installed</span>}
                </header>
                {(e.updateAvailable || e.experimental || e.nonCommercial || e.needsBios) && (
                  <div className="emu-card-tags">
                    {e.updateAvailable && <span className="gm-pill warn"><IconInfo size={14} /> Update available</span>}
                    {e.experimental && <span className="gm-pill">Experimental</span>}
                    {e.nonCommercial && <span className="gm-pill">Non-commercial licence</span>}
                    {e.needsBios && <span className="gm-pill">Needs a BIOS file</span>}
                  </div>
                )}
                <div className="emu-card-meta">
                  <span>Licence: {e.licenseUrl ? <WebLink href={e.licenseUrl}>{e.license}</WebLink> : e.license}</span>
                  {needs && <span>{needs}</span>}
                  {e.downloadBytes ? <span>Download: {fmtBytes(e.downloadBytes)}</span> : null}
                  {e.installed && <span>Installed: version {e.installed.version}{e.installed.installedAt ? `, ${fmtDateTime(e.installed.installedAt * 1000)}` : ''}</span>}
                  <span>{e.stream === 'kasmvnc' ? 'Shown through its own desktop' : 'Streamed like any other game'}</span>
                </div>
                {!e.installable && e.why && <p className="emu-hold" role="note">{e.why}</p>}
                {e.installed && !e.runnable && e.whyNot && <p className="emu-hold" role="note">{e.whyNot}</p>}
                {job && progress && (
                  <div className="emu-job" role="group" aria-label={`Installing ${e.name}`}>
                    <Progress label={`Installing ${e.name}`} value={progress.value} max={progress.max} />
                    <div className="emu-job-line">
                      <span aria-live="off">{progress.text}</span>
                      <button type="button" className="btn sm ghost" disabled={working} onClick={() => { void cancel(e) }} aria-label={`Cancel installing ${e.name}`}>Cancel</button>
                    </div>
                  </div>
                )}
                {e.job?.state === 'failed' && <p className="emu-problem" role="alert">{e.job.error || 'The install failed.'}</p>}
                {e.job?.state === 'cancelled' && !problems[e.id] && <p className="muted">The install was cancelled.</p>}
                {problems[e.id] && <p className="emu-problem" role="alert">{problems[e.id]}</p>}
                {e.bios && e.bios.length > 0 && (
                  <ul className="gm-checks" aria-label={`BIOS files for ${e.name}`}>
                    {e.bios.map((b) => (
                      <li key={b.name} className={b.present ? 'ok' : 'info'}>
                        {b.present ? <IconCheck size={16} /> : <IconInfo size={16} />}
                        <span><code>{b.name}</code><small>{b.present ? 'Found.' : 'Not found yet.'}{b.note ? ` ${b.note}` : ''}</small></span>
                      </li>
                    ))}
                  </ul>
                )}
                {!job && (
                  <div className="emu-card-actions">
                    {!e.installed && e.installable && <button type="button" className="btn sm primary" disabled={working} onClick={() => setConfirm(e)} aria-label={`Install ${e.name}`}>Install</button>}
                    {e.installed && e.updateAvailable && e.installable && <button type="button" className="btn sm primary" disabled={working} onClick={() => setConfirm(e)} aria-label={`Update ${e.name}`}>Update</button>}
                    {e.installed && <button type="button" className="btn sm" disabled={working} onClick={() => setSettings(e)} aria-label={`${e.name} settings`}>Settings</button>}
                    {e.installed && <button type="button" className="btn sm ghost" disabled={working} onClick={() => { void remove(e) }} aria-label={`Remove ${e.name}`}>{working ? 'Working…' : 'Remove'}</button>}
                  </div>
                )}
              </article>
            )
          })}
        </div>
      )}
      {confirm && (
        <Dialog title={`${confirm.installed ? 'Update' : 'Install'} ${confirm.name}?`} onClose={() => setConfirm(null)}
          actions={<><button type="button" className="btn ghost" onClick={() => setConfirm(null)}>Cancel</button><button type="button" className="btn primary" autoFocus onClick={() => { void install(confirm) }}>{confirm.installed ? 'Update' : 'Install'}</button></>}>
          <dl className="emu-licence">
            <dt>Emulator</dt><dd>{confirm.name}, version {confirm.version}</dd>
            <dt>Licence</dt><dd>{confirm.licenseUrl ? <WebLink href={confirm.licenseUrl}>{confirm.license}</WebLink> : confirm.license}{confirm.nonCommercial ? ', which doesn’t allow commercial use' : ''}</dd>
            <dt>Its project</dt><dd><WebLink href={confirm.upstream}>{hostOf(confirm.upstream) || confirm.upstream}</WebLink></dd>
            {confirm.downloadBytes ? <><dt>Download</dt><dd>{fmtBytes(confirm.downloadBytes)}</dd></> : null}
          </dl>
          <p>{confirm.name} is a separate program, under its own licence: its authors’ work, not part of this server. It’s downloaded from its own project, and runs boxed in, in a container or a sandbox, whenever someone plays.</p>
          <p>No games and no BIOS files come with it. Those are yours to supply.</p>
          {confirm.experimental && <p className="muted">It’s marked experimental here: expect rough edges.</p>}
        </Dialog>
      )}
      {settings && <Suspense fallback={null}><EmulatorSettingsDialog emulator={{ id: settings.id, name: settings.name }} adminOnly onClose={() => setSettings(null)} /></Suspense>}
    </Section>
  )
}

// Games running in an emulator now, looked at again every few seconds while they're on screen.
function Sessions() {
  const box = useRef<HTMLDivElement>(null)
  const visible = useVisible(box)
  const [rows, setRows] = useState<RunningSession[] | null>(null)
  const [error, setError] = useState('')
  const [names, setNames] = useState<Map<number, string>>(new Map())
  const [stopping, setStopping] = useState('')
  const load = useCallback(() => emulatorsApi.sessions().then((r) => { setRows(r || []); setError('') }).catch((e) => { setError(e?.message || 'The list couldn’t be loaded.') }), [])
  useEffect(() => { void load() }, [load])
  usePoll(load, SESSIONS_MS, visible)
  // Whose game it is, by name. Without the list of people, a number will do.
  useEffect(() => { api.get<UserView[]>('/users').then((us) => setNames(new Map(us.map((u) => [u.id, profileName(u)])))).catch(() => {}) }, [])
  const stop = async (s: RunningSession) => {
    const whose = s.mine ? 'your' : `${names.get(s.userId) || 'someone'}’s`
    if (!await ask({ title: `Stop ${s.title}?`, body: `This ends ${whose} game in ${s.name}. Anything the game hasn’t saved itself is lost.`, confirm: 'Stop the game', danger: true })) return
    setStopping(s.id)
    try { await emulatorsApi.stop(s.id); toast('Stopped') } catch (e: any) { toast(e?.message || 'It couldn’t be stopped') } finally { setStopping(''); void load() }
  }
  const now = Date.now()
  return (
    <Section title="Running now" desc="Games being played in an emulator at the moment.">
      <div ref={box}>
        {error && !rows && <Notice tone="danger" action={<button type="button" className="btn sm" onClick={() => { void load() }}>Try again</button>}>{error}</Notice>}
        {!rows && !error && <Spinner />}
        {rows && rows.length === 0 && <p className="muted">Nothing is running.</p>}
        {rows && rows.length > 0 && (
          <ul className="emu-sessions">
            {rows.map((s) => (
              <li key={s.id}>
                <span className="emu-session-what">
                  <b>{s.title}</b>
                  <small>{s.mine ? 'You' : names.get(s.userId) || `Account ${s.userId}`} · {s.name} · {s.backend}</small>
                  <small>Started {fmtDateTime(s.started)} ({sinceWords(s.started, now)}) · {s.viewers === 1 ? '1 device watching' : `${s.viewers} devices watching`}</small>
                </span>
                <span className="emu-session-side">
                  <span className={'gm-pill ' + (s.state === 'paused' ? 'off' : 'ok')}>{s.state === 'paused' ? 'Paused' : 'Running'}</span>
                  <button type="button" className="btn sm" disabled={stopping === s.id} onClick={() => { void stop(s) }} aria-label={`Stop ${s.title}`}>{stopping === s.id ? 'Stopping…' : 'Stop'}</button>
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Section>
  )
}

// Emulators an administrator installed on this server themselves: added by their path, run in
// the same sandbox as any other. The only way to play the Switch, for which WittyPlay offers no
// emulator, and how a FreeBSD server uses its own packages.
function OwnPrograms() {
  const consoles = useConsoles()
  const [list, setList] = useState<OwnProgram[] | null>(null)
  const [max, setMax] = useState(32)
  const [failed, setFailed] = useState('')
  const [editing, setEditing] = useState<{ at: number; p: OwnProgram } | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [catalogue, setCatalogue] = useState<CatalogueEntry[]>([])
  const load = useCallback(() => emulatorsApi.own().then((r) => { setList(r.own || []); setMax(r.max || 32); setFailed('') }).catch((e) => setFailed(e?.message || 'Your programs couldn’t be loaded.')), [])
  useEffect(() => { void load(); emulatorsApi.catalogue().then((l) => setCatalogue(l || [])).catch(() => {}) }, [load])
  const save = async (next: OwnProgram[]) => {
    setBusy(true)
    setError('')
    try {
      const r = await emulatorsApi.saveOwn(next)
      setList(r.own || [])
      setEditing(null)
      toast('Saved')
    } catch (e: any) {
      setError(e?.message || 'That couldn’t be saved.')
    } finally { setBusy(false) }
  }
  const remove = async (at: number) => {
    if (!list) return
    if (!(await ask({ title: `Remove ${list[at].name}?`, body: 'Its program stays where it is. People’s settings for it are kept.', confirm: 'Remove' }))) return
    await save(list.filter((_, i) => i !== at))
  }
  const name = (id: string) => consoles.find((c) => c.id === id)?.short || id
  return (
    <Section title="Your own emulator programs" desc="An emulator you installed on this server yourself, from wherever you chose. It runs in the same sandbox as the others, on this computer, never in a container. WittyPlay downloads nothing for it and checks no hash: it’s yours."
      actions={list && list.length < max ? <button type="button" className="btn sm" onClick={() => { setError(''); setEditing({ at: -1, p: { name: '', systems: [], program: '', args: [] } }) }}>Add a program</button> : undefined}>
      {failed ? <Notice tone="danger" action={<button type="button" className="btn sm" onClick={() => { void load() }}>Try again</button>}>{failed}</Notice>
        : !list ? <Spinner />
        : !list.length ? <p className="muted">None yet. This is how Switch games play, as WittyPlay offers no Switch emulator, and how an emulator from your system’s packages is used.</p>
        : (
          <ul className="emu-sessions">
            {list.map((p, i) => (
              <li key={p.id || i}>
                <span><b>{p.name}</b><small className="muted">{p.systems.map(name).join(', ')} · <code>{p.program}</code>{p.like ? ` · settings of ${catalogue.find((c) => c.id === p.like)?.name || p.like}` : ''}</small></span>
                <span>
                  <button type="button" className="btn sm" onClick={() => { setError(''); setEditing({ at: i, p }) }}>Change</button>
                  <button type="button" className="btn sm ghost" disabled={busy} onClick={() => { void remove(i) }}>Remove</button>
                </span>
              </li>
            ))}
          </ul>
        )}
      {editing && list && (
        <OwnProgramDialog program={editing.p} consoles={consoles} catalogue={catalogue} busy={busy} error={error} onClose={() => setEditing(null)}
          onSave={(p) => { void save(editing.at < 0 ? [...list, p] : list.map((x, i) => (i === editing.at ? p : x))) }} />
      )}
    </Section>
  )
}

function OwnProgramDialog({ program, consoles, catalogue, busy, error, onClose, onSave }: {
  program: OwnProgram; consoles: { id: string; name: string }[]; catalogue: CatalogueEntry[]; busy: boolean; error: string
  onClose: () => void; onSave: (p: OwnProgram) => void
}) {
  const [p, setP] = useState<OwnProgram>(program)
  const [args, setArgs] = useState(joinArgs(program.args))
  const ok = p.name.trim() !== '' && p.program.trim() !== '' && p.systems.length > 0
  const toggle = (id: string) => setP((x) => ({ ...x, systems: x.systems.includes(id) ? x.systems.filter((s) => s !== id) : [...x.systems, id] }))
  return (
    <Dialog title={program.name ? `Change ${program.name}` : 'Add your own emulator program'} size="wide" onClose={onClose}
      actions={<><button type="button" className="btn ghost" onClick={onClose}>Cancel</button><button type="button" className="btn primary" disabled={!ok || busy} onClick={() => onSave({ ...p, args: splitArgs(args) })}>{busy ? 'Saving…' : 'Save'}</button></>}>
      <div className="emu-dialog-body emu-form">
        <Row label="Name"><Input aria-label="Name" value={p.name} maxLength={60} onChange={(e) => setP({ ...p, name: e.target.value })} /></Row>
        <Row label="The program" desc="Its full path on this server, such as /usr/local/bin/rpcs3 or C:\Emulators\MyEmulator\emulator.exe.">
          <Input aria-label="The program’s full path" value={p.program} spellCheck={false} autoComplete="off" onChange={(e) => setP({ ...p, program: e.target.value })} />
        </Row>
        <Row label="A build of" desc="If it’s one of the emulators WittyPlay knows, from another source: its settings and controllers are set up the same way.">
          <Select aria-label="A build of" value={p.like || ''} onChange={(e) => setP({ ...p, like: e.target.value || undefined })}>
            <option value="">None of these</option>
            {catalogue.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
          </Select>
        </Row>
        <Row label="Its command line" desc="What comes after the program. {game} is the game’s file, {config} the folder its settings go in, {bios} the BIOS folder. Without {game}, the game goes last." stack>
          <Input aria-label="Its command line" value={args} spellCheck={false} autoComplete="off" placeholder="--fullscreen {game}" onChange={(e) => setArgs(e.target.value)} />
        </Row>
        <Row label="Consoles it plays" stack>
          <ul className="emu-map">
            {consoles.map((c) => <li key={c.id}><span>{c.name}</span><Switch checked={p.systems.includes(c.id)} onChange={() => toggle(c.id)} label={c.name} /></li>)}
          </ul>
        </Row>
        {error && <div className="emu-form-error"><Notice tone="danger">{error}</Notice></div>}
      </div>
    </Dialog>
  )
}
