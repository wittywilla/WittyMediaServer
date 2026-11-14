// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { keepDataChoice } from './store/keepData'
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { HelpLink } from '../../components/HelpLink'
import { Link, useLocation, useSearchParams } from 'react-router-dom'
import { fmtDateTime, api, ApiError } from '../../api'
import { LIB_TYPES } from '../../components/AddLibrary'
import { Spinner } from '../../components/common'
import { IconCheck, IconDown, IconInfo, IconPlug, IconPlus, IconRefresh, IconShield, IconTv, libIcon } from '../../icons'
import { shelves, type LibraryKind, type ModuleEvent, type ModuleReport, type ModuleStatus, type VersionInfo, type VersionList } from '../../moduleStore'
import { toast, useApp } from '../../state'
import SpecPage from '../../spec/SpecPage'
import { modulesTopSpec } from '../../spec/specs/modules'
import { PageHead, Row, Section, Switch } from './ui'
import './modules.css'
import LegalNote from '../../components/LegalNote'
import { LinkButton } from '../../ui'
import { ChangeNotes, FailureNotice, ModuleIcon, ModuleLogDialog, PolicyNote, SandboxNote, UpdateAllPanel, useStoreView, WhatsNew } from './store/parts'
import { checkAll, failureForStatus, type Failure } from './store/storeApi'
import { olderOf, type OlderVersion } from './store/olderServer'
import { useOlderAsk } from './store/OlderServerDialog'
import { Dialog } from '../../ui/Overlay'
import { ConfirmOwner, OwnerFields, type OwnerAsk } from './ConfirmOwner'
import { OverrideRecordBox, OverrideTags, PendingNotice, StatusNotice, useOverrideDialog, type OverrideAsk } from './store/overrideParts'
import { needsOverrideOf, overrideLines, releaseStatus, statusWarns, type NeedsOverride } from './store/overrides'
import { InstallFromFile } from './store/InstallFromFile'
import { ModuleAudit } from './store/ModuleAudit'
import { useFeatures } from '../../sdui/useSdui'

// The module store: what this server can do, by what it is for, to add and take away,
// with the versions, pins and forks of the ones that have releases of their own. Nothing
// on this page goes online unless an admin presses Check now or turned the automatic
// check on; a version list only reads what the last check found.

const PHASE: Record<string, string> = { downloading: 'Downloading', verifying: 'Checking the signature', installing: 'Installing' }
const busy = (m: ModuleStatus) => !!PHASE[m.phase]
const mb = (n = 0) => (n / 1048576).toFixed(1)
const when = (ms: number) => fmtDateTime(ms)
const day = (ms: number) => new Date(ms).toLocaleDateString(undefined, { dateStyle: 'medium' })
const kindName = (t: string) => LIB_TYPES.find((x) => x.type === t)?.name || t
const needsLook = (m: ModuleStatus) => m.state === 'fallback' || m.phase === 'failed' || !!m.withdrawn || !!m.notice || (m.installed && !!m.olderServer)
const severity = (s: string) => s === 'security' ? 'for security reasons' : 'because it was broken'
const pinWords = (pin: string) => `version ${pin}`
const moduleIcon = (m: ModuleStatus) => m.libraries?.length ? libIcon(m.libraries[0]) : m.group === 'tv' ? <IconTv /> : <IconPlug />
// A release's address comes unsigned from GitHub, so only a plain link to GitHub is used.
const safeUrl = (u?: string) => u && /^https:\/\/github\.com\//.test(u) ? u : ''

type Filter = 'all' | 'installed' | 'absent' | 'updates'
const FILTERS: { id: Filter; label: string; test: (m: ModuleStatus) => boolean }[] = [
  { id: 'all', label: 'All', test: () => true },
  { id: 'installed', label: 'Installed', test: (m) => m.installed },
  { id: 'absent', label: 'Not installed', test: (m) => !m.installed },
  { id: 'updates', label: 'Updates', test: (m) => m.installed && !!m.latest },
]

// What happened to a module, in words.
function eventWords(e: ModuleEvent): string {
  const v = e.version ? `version ${e.version}` : ''
  switch (e.kind) {
    case 'added': return 'Added to this server'
    case 'removed': return 'Removed from this server'
    case 'failed': return `${v || 'A version'} failed${e.detail ? `: ${e.detail}` : ''}`
    case 'taken-out-of-use': return `${v || 'A version'} was taken out of use${e.detail ? `: ${e.detail}` : ''}`
    case 'put-back': return `${v || 'A version'} was put back in use${e.detail ? `: ${e.detail}` : ''}`
    case 'rolled-back': return `Went back from ${v || 'a version'}`
    case 'pinned': return `Kept on ${v || 'this version'}`
    case 'unpinned': return 'Automatic updates can move it again'
    case 'switched': return `Switched ${e.detail || `to ${v}`}`
    case 'source-changed': return `Publisher changed: ${e.detail || ''}`
    case 'withdrawn': return `${v || 'A version'} was withdrawn by its publisher${e.detail ? `: ${e.detail}` : ''}`
    default: return [e.kind.replace(/-/g, ' '), v, e.detail].filter(Boolean).join(', ')
  }
}

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

type Ask = { title: string; body: ReactNode; action: string; danger?: boolean; run: () => Promise<void> | void }

// A question before something that can't be taken back with one press. The dialog's
// keyboard handling comes from useDialogs, as for every .modal.
function Confirm({ ask, onClose }: { ask: Ask; onClose: () => void }) {
  const [working, setWorking] = useState(false)
  const yes = async () => { setWorking(true); try { await ask.run() } finally { setWorking(false); onClose() } }
  return (
    <Dialog title={ask.title} onClose={onClose} className="set-modal" actions={<><button className="btn ghost" autoFocus onClick={onClose}>Cancel</button>
          <button className={'btn ' + (ask.danger ? 'danger' : 'primary')} disabled={working} onClick={yes}>{ask.action}</button></>}>
        <div className="ms-ask">{ask.body}</div>
    </Dialog>
  )
}


// A module installed past the rules that vouch for its publisher: what it says about itself is unchecked.
const unvouched = (m: { override?: { kinds?: string[] } | null }) => !!m.override?.kinds?.some((k) => k === 'unsigned' || k === 'untrusted-key' || k === 'custom')

export default function ModulesSettings() {
  const [r, setR] = useState<ModuleReport | null>(null)
  const [failed, setFailed] = useState('')
  const [checking, setChecking] = useState(false)
  const [settings, setSettings] = useState<Record<string, string> | null>(null)
  const [filter, setFilter] = useState<Filter>('installed')
  const [open, setOpen] = useState<string | null>(null)
  const [hi, setHi] = useState('')
  const [ask, setAsk] = useState<Ask | null>(null)
  const [ownerAsk, setOwnerAsk] = useState<OwnerAsk | null>(null)
  const [refused, setRefused] = useState<{ id: string; why: string } | null>(null) // why it couldn't be removed
  const [versionsOf, setVersionsOf] = useState<ModuleStatus | null>(null)
  const [params] = useSearchParams()
  const loc = useLocation()
  const target = params.get('add') || loc.hash.slice(1)
  const reached = useRef('')
  const [hostReady, setHostReady] = useState(false)
  const store = useStoreView()
  const [logOf, setLogOf] = useState<ModuleStatus | null>(null)
  const features = useFeatures()
  const { user } = useApp()

  useEffect(() => {
    api.get<ModuleReport>('/modules').then(setR).catch((e) => setFailed(e.message))
    api.get<Record<string, string>>('/settings').then(setSettings).catch(() => {})
    api.get<{ ready: boolean }>('/couch').then((m) => setHostReady(m.ready)).catch(() => {})
  }, [])

  // While something installs, follow it.
  const active = !!r?.modules.some(busy)
  useEffect(() => {
    if (!active) return
    const t = setInterval(() => { api.get<ModuleReport>('/modules').then(setR).catch(() => {}) }, 1000)
    return () => clearInterval(t)
  }, [active])

  // A link from Add Library lands on its module: open, marked, and with the focus.
  useEffect(() => {
    if (!r || !target || reached.current === target || !r.modules.some((m) => m.id === target)) return
    reached.current = target
    setFilter('all'); setOpen(target); setHi(target)
  }, [r, target])
  useEffect(() => {
    if (!hi) return
    const row = document.getElementById('mod-' + hi)
    row?.scrollIntoView({ block: 'center' })
    row?.querySelector<HTMLElement>('.ms-head')?.focus({ preventScroll: true })
  }, [hi])

  // After a change that took away the button that had the focus.
  const focusRow = (id: string) => setTimeout(() => {
    const now = document.activeElement
    if (!now || now === document.body) document.getElementById('mod-' + id)?.querySelector<HTMLElement>('.ms-head')?.focus()
  }, 50)

  // Pressed again while it runs, it does nothing. It isn't disabled, as that would drop
  // the focus, and a remote would have to find its way back.
  const check = async () => {
    if (checking) return
    setChecking(true)
    try {
      store.setView(await checkAll())
      const n = await api.get<ModuleReport>('/modules')
      setR(n)
      if (n.checkError) toast('The check didn’t work. What happened is on the page.')
      else if (!n.modules.some((m) => m.latest)) toast('Every module is up to date')
    } catch (e: any) { toast(e.message) } finally { setChecking(false) }
  }
  const save = async (k: string, v: boolean) => {
    const old = settings
    setSettings((s) => ({ ...(s || {}), [k]: v ? '1' : '0' }))
    try { await api.put('/settings', { [k]: v ? '1' : '0' }) } catch (e: any) { setSettings(old); toast(e.message) }
  }
  // Runs one action on a module; the answer is always the new report.
  const act = async (call: () => Promise<ModuleReport>, done?: string) => {
    try { setR(await call()); if (done) toast(done); void store.reload() } catch (e: any) { toast(e.message) }
  }
  const older = useOlderAsk()
  // Adding asks first when the only version was made for an older server: only Install anyway
  // sends confirmOlder.
  const add = async (m: ModuleStatus) => {
    try { setR(await api.post<ModuleReport>(`/modules/${m.id}/add`)); toast(`${m.name} is being installed`); void store.reload() } catch (e: any) {
      const old = olderOf(e)
      if (!old) { toast(e.message); focusRow(m.id); return }
      const yes = await older.ask(old)
      if (old.every((o) => yes.includes(o.module))) await act(() => api.post<ModuleReport>(`/modules/${m.id}/add`, { confirmOlder: true }), `${m.name} is being installed`)
      else toast(`${m.name} wasn’t installed`)
    }
    focusRow(m.id)
  }
  const remove = (m: ModuleStatus) => { const keep = keepDataChoice(m.name); setAsk({
    title: `Remove ${m.name}?`, action: 'Remove', danger: true,
    body: <>
      <p>{m.name} stops showing in the app and stops checking for updates.{m.libraries?.length ? ` New libraries of these kinds can’t be made: ${m.libraries.map(kindName).join(', ')}.` : ''} Your files and watch history stay as they are.</p>
      <p>Every version of it that was downloaded is deleted, which frees the disk space{m.source ? ', and the fork it comes from is forgotten' : ''}. You can install it again from the store at any time.</p>
      {!!m.requiredBy?.length && <p>{m.requiredBy.join(', ')} {m.requiredBy.length === 1 ? 'is' : 'are'} built on it.</p>}
      {keep.node}
    </>,
    run: async () => {
      setRefused(null)
      try { setR(await api.post<ModuleReport>(`/modules/${m.id}/remove${keep.query()}`)); toast(`${m.name} removed`); focusRow(m.id) }
      catch (e: any) { setRefused({ id: m.id, why: e.message }) }
    },
  }) }
  const over = useOverrideDialog()
  // A version the usual rules refuse, installed with an override: the dialog sends it again.
  const overrideInstall = (m: ModuleStatus, version: string, need: NeedsOverride) => over.open(overrideAskFor(m, version, need, (n) => { setR(n); void store.reload(); focusRow(m.id) }))
  const update = async (m: ModuleStatus) => {
    if (!m.latest) return
    const version = m.latest.version
    const install = (confirm: boolean) => api.post<ModuleReport>(`/modules/${m.id}/install`, { version, confirm })
    try { setR(await install(false)); focusRow(m.id) } catch (e: any) {
      const need = needsOverrideOf(e)
      if (need) overrideInstall(m, version, need)
      else if (e instanceof ApiError && e.data?.code === 'confirm') setAsk({ title: `Update ${m.name} to ${version}?`, body: <p>{e.data.warning}</p>, action: 'Update anyway', run: async () => {
        try { setR(await install(true)); focusRow(m.id) } catch (x) { const n = needsOverrideOf(x); if (n) overrideInstall(m, version, n); else toast(x instanceof Error ? x.message : String(x)) }
      } })
      else toast(e.message)
    }
  }
  // A release held after its download, waiting for the go-ahead.
  const goAhead = (m: ModuleStatus) => { if (m.pending) overrideInstall(m, m.pending.version, { ...m.pending.needsOverride, version: m.pending.version, pending: true }) }
  const rollback = (m: ModuleStatus) => setOwnerAsk({
    title: m.previous ? `Go back to version ${m.previous}?` : `Stop using ${m.name} ${m.version}?`, action: m.previous ? 'Go back' : 'Stop using it',
    body: <p>{m.name} stops using version {m.version}, and that version is marked as taken out of use, so automatic updates won’t bring it back. You can still choose it yourself.{m.previous ? ' An older version may lack security fixes, so this asks for your password.' : ' There’s no earlier version here, so it won’t be working until you install another. This asks for your password.'}</p>,
    run: async (password, code) => { setR(await api.post<ModuleReport>(`/modules/${m.id}/rollback`, { password, code })); toast(m.previous ? `${m.name} went back to version ${m.previous}` : `${m.name} is no longer in use`); focusRow(m.id) },
  })
  const pin = (m: ModuleStatus) => act(() => m.pin ? api.del<ModuleReport>(`/modules/${m.id}/pin`) : api.post<ModuleReport>(`/modules/${m.id}/pin`), m.pin ? `${m.name} can update again` : `${m.name} stays on this version`)
  const setAuto = (m: ModuleStatus, on: boolean) => act(() => api.put<ModuleReport>(`/modules/${m.id}/autoupdate`, { on }), on ? `${m.name} can update by itself` : `${m.name} will wait for you to update it`)
  const toLatest = (m: ModuleStatus) => act(() => api.post<ModuleReport>(`/modules/${m.id}/latest`), `${m.name} is going back to the latest`)
  const official = (m: ModuleStatus) => setAsk({
    title: 'Go back to the official version?', action: 'Go back',
    body: <p>{m.name} goes back to the releases of <b>{m.stockRepo}</b>. Versions downloaded from {m.source?.repo} are deleted, and it isn’t on this server until you install the official one.</p>,
    run: async () => { await act(() => api.del<ModuleReport>(`/modules/${m.id}/source`), `${m.name} is back to the official version`); focusRow(m.id) },
  })

  if (failed) return (
    <div>
      <PageHead title="Modules" />
      <div className="set-card set-empty big"><IconPlug size={34} /><b>Modules aren’t available</b><p>{failed}</p></div>
    </div>
  )
  if (!r) return <div><PageHead title="Modules" /><Spinner /></div>
  const canUpload = !!features?.has('module-upload')
  const canAudit = !!features?.has('module-audit') && !!user?.isAdmin

  // Until it is set, the automatic check follows the one for server updates.
  const autoCheck = (settings?.moduleUpdateCheck || settings?.updateCheck || '1') === '1'
  const held = r.holdUntil && r.holdUntil > Date.now() ? r.holdUntil : 0
  const test = FILTERS.find((f) => f.id === filter)!.test
  const shown = r.modules.filter(test)
  const core = shown.filter((m) => m.core)
  const groups = shelves(shown.filter((m) => !m.core))
  const nameOf = (id: string) => r.modules.find((m) => m.id === id)?.name || id
  const row = (m: ModuleStatus) => (
    <ModuleRow key={m.id} m={m} onGoAhead={() => goAhead(m)} onOverride={(n) => { if (n) setR(n); else api.get<ModuleReport>('/modules').then(setR).catch(() => {}); void store.reload() }} isOpen={open === m.id} marked={hi === m.id} checkedAt={r.checkedAt} nameOf={nameOf} refused={refused?.id === m.id ? refused.why : ''}
      onToggle={() => setOpen(open === m.id ? null : m.id)} onReport={setR}
      onAdd={() => add(m)} onRemove={() => remove(m)} onUpdate={() => update(m)} onVersions={() => setVersionsOf(m)}
      onPin={() => pin(m)} onRollback={() => rollback(m)} onOfficial={() => official(m)} onAuto={(on) => setAuto(m, on)} onLatest={() => toLatest(m)} onLog={() => setLogOf(m)} repairRoute={store.view?.repairRoute || '/settings/repair'} serverFailure={store.view?.items.find((i) => i.official && i.id === m.id)?.failure} serverAuto={settings?.moduleAutoInstall === '1' && autoCheck} kinds={r.libraries || []} />
  )

  return (
    <div className="ms-page">
      <PageHead title="Modules" desc="What this server can do, one piece at a time. Add what you want and remove what you don’t."
        actions={<LinkButton variant="primary" to="/settings/modules/store" icon={<IconPlug />}>Browse the store</LinkButton>} />

      {store.view && <WhatsNew view={store.view} />}

      <SpecPage embedded page={modulesTopSpec({
        autoCheck, autoInstall: settings?.moduleAutoInstall === '1', loaded: !!settings, checkedAt: r.checkedAt ? when(r.checkedAt) : '', held: held ? when(held) : '', checkError: r.checkError || '', checking,
        server: r.server, api: String(r.minApi === r.api ? r.api : `${r.minApi} to ${r.api}`),
        filters: FILTERS.map((f) => ({ id: f.id, label: f.label, count: r.modules.filter(f.test).length })), filter,
      })} handled={new Set(['check', ...FILTERS.map((f) => 'filter:' + f.id)])}
        onAction={(id) => { if (id === 'check') void check(); else if (id.startsWith('filter:')) setFilter(id.slice(7) as Filter) }}
        onField={(id, v) => { if (id === 'autoCheck') save('moduleUpdateCheck', !!v); else if (id === 'autoInstall') save('moduleAutoInstall', !!v) }}
        after={{ 1: (
          <>
            {store.view && <UpdateAllPanel view={store.view} onChanged={() => { void store.reload(); api.get<ModuleReport>('/modules').then(setR).catch(() => {}) }} />}
            {store.view && <PolicyNote view={store.view} />}
            <SandboxNote sandbox={r.sandbox} />
          </>
        ) }} />

      {shown.length === 0 && (
        <div className="set-card set-empty">
          <b>{filter === 'updates' ? 'No updates known' : filter === 'absent' ? 'Every module is on this server' : filter === 'installed' ? 'No modules yet' : 'Nothing here'}</b>
          {filter === 'updates' && <p>{r.checkedAt ? `Nothing newer was found when the server last looked, ${when(r.checkedAt)}.` : 'The server hasn’t looked yet.'}</p>}
          {filter === 'installed' && <p>The server works without any. <Link to="/settings/modules/store">Browse the store</Link> to add what you want.</p>}
          {filter === 'updates' && <button className="btn sm" onClick={check} aria-busy={checking}><IconRefresh /> {checking ? 'Checking…' : 'Check now'}</button>}
        </div>
      )}

      {groups.map((g) => <Section key={g.id} id={'shelf-' + g.id} title={g.title}>{g.modules.map(row)}</Section>)}

      {core.length > 0 && (
        <div className="ms-quiet">
          <Section title="What other modules are built on" desc="These stay for as long as something uses them, and can’t be removed on their own.">{core.map(row)}</Section>
        </div>
      )}

      {canUpload ? (
        <Section id="custom-modules" title="Custom modules" actions={<InstallFromFile onInstalled={() => { api.get<ModuleReport>('/modules').then(setR).catch(() => {}); void store.reload() }} />}
          desc="Install a module from its release archive: one you built, or one someone gave you. The server checks the file and says what it found before anything is installed. Replacing an existing module with someone’s fork is done from that module above, under Advanced.">
          <p className="ms-note">A module is someone’s code running on your server, so only install one from a publisher you trust. Anything unsigned, signed by a key this server doesn’t trust, or new to this server needs an override, gets no permissions you don’t tick, and never updates by itself. <HelpLink to="module-store#installing-from-a-file">Installing from a file</HelpLink></p>
        </Section>
      ) : filter === 'all' && (
        <div className="ms-quiet">
          <Section title="Custom modules" actions={<span className="set-pill">In development</span>}
            desc="Adding a wholly new kind of module isn’t available yet. Replacing an existing module with someone’s fork is: open that module above, then Advanced.">
            <p className="ms-note">When it comes, the same rule will apply: a module is someone’s code running on your server, so only take one from a publisher you trust.</p>
          </Section>
        </div>
      )}

      {canAudit && <ModuleAudit nameOf={nameOf} stamp={r} />}
      {over.dialog}
      {older.dialog}

      {ownerAsk && <ConfirmOwner ask={ownerAsk} onClose={() => setOwnerAsk(null)} />}
      {versionsOf && <VersionsDialog m={r.modules.find((m) => m.id === versionsOf.id) || versionsOf} onClose={() => setVersionsOf(null)} onReport={setR} />}
      {ask && <Confirm ask={ask} onClose={() => setAsk(null)} />}
      {logOf && <ModuleLogDialog moduleId={logOf.id} name={logOf.name} onClose={() => setLogOf(null)} />}

      <Section title="Installed separately" desc="A program of its own that runs beside this server, under its own license. The server works without it.">
        <div className="mod-list">
          <div className="mod-row">
            <span className="mod-name"><b>WittyCouch Host</b><span className="muted">Lets people play on Game PCs in the page, built on Moonlight’s own code. License: AGPL-3.0-or-later, with Moonlight-derived parts under the GPL-3.0. <a href="https://github.com/wittywilla/WittyCouchHost" target="_blank" rel="noreferrer noopener">Source</a></span></span>
            <span className="mod-ver"><span className="mod-local">{hostReady ? 'Installed' : 'Not installed'}</span></span>
          </div>
        </div>
      </Section>
      <LegalNote kind="modules" className="set-legal" />
    </div>
  )
}

function Tag({ tone, icon, children }: { tone?: 'ok' | 'warn' | 'bad' | 'new' | 'off'; icon?: ReactNode; children: ReactNode }) {
  return <span className={'ms-tag' + (tone ? ' ' + tone : '')}>{icon}{children}</span>
}

function Tags({ m }: { m: ModuleStatus }) {
  return (
    <span className="ms-tags">
      {!m.installed ? <Tag tone="off">{m.comesInside ? 'Comes with other modules' : m.unavailable ? 'No build here' : 'Not installed'}</Tag>
        : busy(m) ? <Tag tone="new">{PHASE[m.phase]}</Tag>
        : needsLook(m) ? <Tag tone="bad" icon={<IconInfo size={12} />}>Needs a look</Tag>
        : m.latest ? <Tag tone="new">Update available</Tag>
        : <Tag tone="ok" icon={<IconCheck size={12} />}>Installed</Tag>}
      {m.pending && <Tag tone="warn">Needs your go-ahead</Tag>}
      {m.installed && m.olderServer && <Tag tone="warn" icon={<IconInfo size={12} />}>Made for an older server</Tag>}
      {!m.installed && !m.latest && m.olderOffer && <Tag tone="warn">Only for an older server</Tag>}
      {m.installed && m.pin && <Tag>Pinned</Tag>}
      {m.source && <Tag tone="warn" icon={<IconShield size={12} />}>Other publisher</Tag>}
      <OverrideTags override={m.override} status={m.releaseStatus} />
    </span>
  )
}

type RowProps = {
  m: ModuleStatus; isOpen: boolean; marked: boolean; checkedAt?: number; nameOf: (id: string) => string; refused: string
  onToggle: () => void; onReport: (r: ModuleReport) => void
  onAdd: () => void; onRemove: () => void; onUpdate: () => void; onVersions: () => void; onPin: () => void; onRollback: () => void; onOfficial: () => void
  onAuto: (on: boolean) => void; onLatest: () => void; onLog: () => void; repairRoute: string; serverFailure?: Failure; serverAuto: boolean; kinds: LibraryKind[]
  onGoAhead: () => void; onOverride: (r?: ModuleReport) => void
}

// One module: a line to scan, and everything else when it is opened.
function ModuleRow(p: RowProps) {
  const { m, isOpen, marked } = p
  const detail = useId()
  return (
    <article id={'mod-' + m.id} className={'ms-mod' + (marked ? ' marked' : '') + (m.installed ? '' : ' absent')} aria-label={m.name}>
      <div className="ms-row">
        <button type="button" className="ms-head" aria-expanded={isOpen} aria-controls={isOpen ? detail : undefined} onClick={p.onToggle}>
          {m.icon ? <ModuleIcon item={{ icon: m.icon, name: m.name, official: true }} size={36} /> : <span className="ms-ic">{moduleIcon(m)}</span>}
          <span className="ms-title">
            <span className="ms-name"><b>{m.name}</b>{m.installed && <span className="ms-ver">{m.version}</span>}{m.license && <span className="ms-ver">{m.license}</span>}<Tags m={m} /></span>
            {m.summary && <span className="ms-sum">{m.summary}</span>}
          </span>
          <span className="ms-chev"><IconDown size={20} /></span>
        </button>
        {!m.installed && !m.unavailable && !m.comesInside && <button type="button" className={'btn sm' + (marked ? ' primary' : '')} onClick={p.onAdd} aria-label={`Install ${m.name}`}><IconPlus /> <span className="ms-add-word">Install</span></button>}
        {m.installed && m.latest && !m.pin && !busy(m) && !isOpen && <button type="button" className="btn sm" onClick={p.onUpdate} aria-label={`Update ${m.name} to ${m.latest.version}`}>Update</button>}
      </div>
      {busy(m) && (
        <div className="upd-progress ms-progress" role="status">
          <span>{PHASE[m.phase]}{m.phase === 'downloading' && m.total ? ` · ${mb(m.done)} of ${mb(m.total)} MB` : ''}…</span>
          <progress value={m.phase === 'downloading' && m.total ? m.done : undefined} max={m.total || undefined} aria-label={`${m.name}: ${PHASE[m.phase]}`} />
        </div>
      )}
      {isOpen && <ModuleDetail {...p} id={detail} />}
    </article>
  )
}

// A kind of library a module brings. When others bring it too it says so, since they share
// one kind, and that kind can have a name of this server's own, so what is on screen can
// always be told apart from whatever else appears here one day.
function BroughtKind({ type, m, kinds, onReport }: { type: string; m: ModuleStatus; kinds: LibraryKind[]; onReport: (r: ModuleReport) => void }) {
  const kind = kinds.find((k) => k.type === type)
  const usual = kindName(type)
  const [editing, setEditing] = useState(false)
  const [text, setText] = useState(kind?.label || '')
  const [err, setErr] = useState('')
  const [busyNaming, setBusyNaming] = useState(false)
  const others = (kind?.modules || []).filter((x) => x.id !== m.id).map((x) => x.name)
  const save = async (label: string) => {
    setBusyNaming(true); setErr('')
    const taken = Object.fromEntries(LIB_TYPES.map((x) => [x.type, x.name]))
    try { onReport(await api.put<ModuleReport>(`/modules/libraries/${type}/label`, { label, taken })); setEditing(false); setText(label) }
    catch (e: any) { setErr(e.message) } finally { setBusyNaming(false) }
  }
  return (
    <div className="ms-kind">
      <b>{kind?.label || usual}</b>{kind?.label && <span className="muted"> (usually {usual})</span>}
      {others.length > 0 && (
        <p className="ms-note">Also brought by {others.join(', ')}. They share this kind of library, so any one of them is enough to make one, and each plays what it knows.{' '}
          {!editing && <button type="button" className="btn sm ghost" onClick={() => setEditing(true)} aria-label={`Rename the kind ${kind?.label || usual}`}>Rename</button>}
        </p>
      )}
      {editing && (
        <form className="ms-kind-form" onSubmit={(e) => { e.preventDefault(); save(text) }}>
          <label>Call it, on this server
            <input className="input" autoFocus maxLength={40} value={text} placeholder={usual} onChange={(e) => setText(e.target.value)} />
          </label>
          <div className="ms-actions">
            <button type="submit" className="btn sm" disabled={busyNaming}>Save</button>
            {kind?.label && <button type="button" className="btn sm ghost" disabled={busyNaming} onClick={() => save('')}>Use the usual name</button>}
            <button type="button" className="btn sm ghost" onClick={() => { setEditing(false); setErr(''); setText(kind?.label || '') }}>Cancel</button>
          </div>
          <p className="muted">This only changes what the kind is called when you add a library. Libraries keep their own names.</p>
          {err && <p className="error" role="alert">{err}</p>}
        </form>
      )}
    </div>
  )
}

function ModuleDetail({ id, m, checkedAt, nameOf, refused, onReport, kinds, onRemove, onUpdate, onVersions, onPin, onRollback, onOfficial, onAuto, onLatest, onLog, repairRoute, serverFailure, serverAuto, onGoAhead, onOverride }: RowProps & { id: string }) {
  const working = busy(m)
  const failure = serverFailure || failureForStatus(m)
  const events = [...(m.events || [])].reverse().slice(0, 8)
  const latest = m.latest ? m.latest.version : checkedAt ? 'This is the newest known' : 'Not checked yet'
  return (
    <div className="ms-detail" id={id}>
      <PendingNotice m={m} onGo={onGoAhead} onDropped={onOverride} />
      {m.override && <OverrideRecordBox id={m.id} name={m.name} record={m.override} disabled={working} onRemoved={onOverride} />}
      {statusWarns(releaseStatus(m.releaseStatus)) && <StatusNotice status={m.releaseStatus} note={m.statusNote} name={m.name} installed={m.installed} />}
      {m.source && <PublisherNote m={m} onOfficial={onOfficial} disabled={working} />}
      {refused && <p className="error" role="alert">{refused} <Link to="/settings/libraries">Go to Libraries</Link></p>}
      {failure && <FailureNotice failure={failure} name={m.name} moduleId={m.id} repairRoute={repairRoute} onRetry={m.latest ? onUpdate : undefined} />}
      {m.withdrawn && <p className="error">Version {m.withdrawn.version} was withdrawn {severity(m.withdrawn.severity)}: {m.withdrawn.reason}</p>}
      {m.installed && m.olderServer && m.olderWarning && (
        <div className="warn-box ms-older" role="note">
          <b>Made for an older server</b>
          <p>{m.olderWarning}</p>
          <div className="ms-actions">
            {m.latest && !working && <button type="button" className="btn sm primary" onClick={onUpdate}>Update to {m.latest.version}</button>}
            {!working && <button type="button" className={'btn sm' + (m.latest ? ' ghost' : '')} onClick={onRemove}>Remove {m.name}</button>}
          </div>
        </div>
      )}
      {!m.installed && !m.latest && m.olderOffer && <p className="warn-box">No version of {m.name} for this server is published yet. {m.olderOffer.reason} Installing it asks you first.</p>}
      {m.lastFailure && !failure && <p className="warn-box">The last problem it had: {m.lastFailure}</p>}

      <dl className="ms-facts">
        {m.installed && <div><dt>In use</dt><dd>{m.version}</dd></div>}
        {m.installed && m.updatable && <div><dt>Latest</dt><dd>{latest}</dd></div>}
        {!m.installed && <div><dt>Version</dt><dd>{m.latest?.version || 'None available yet'}</dd></div>}
        <div><dt>Works with</dt><dd>Servers {m.minServer} to {m.maxServer}</dd></div>
        {/* For a module nothing here checked (unsigned, an untrusted key, one this server doesn't know), the repository is only what its own file claims. */}
        <div className="wide"><dt>{m.source ? 'Official repository' : unvouched(m) ? 'Says it comes from' : 'Repository'}</dt><dd><a href={`https://github.com/${m.stockRepo || m.repo}`} target="_blank" rel="noreferrer">{m.stockRepo || m.repo}</a>{unvouched(m) && <span className="ms-note"> Nothing checked this: it’s what the module’s own file says.</span>}</dd></div>
        {!!m.libraries?.length && <div className="wide"><dt>Brings these libraries</dt><dd>{m.libraries.map((t) => <BroughtKind key={t} type={t} m={m} kinds={kinds} onReport={onReport} />)}</dd></div>}
        {!!m.needs?.length && <div className="wide"><dt>What it needs</dt><dd>{m.needs.join(', ')}</dd></div>}
        {!!m.requires?.length && <div className="wide"><dt>Built on</dt><dd>{m.requires.map(nameOf).join(', ')}</dd></div>}
        {!!m.requiredBy?.length && <div className="wide"><dt>What’s built on it</dt><dd>{m.requiredBy.join(', ')}</dd></div>}
      </dl>

      <div className="ms-how">
        <b>{m.updatable ? 'Updates on its own' : 'Updates with the server'}</b>
        {m.updatable
          ? <>
              {m.updates && <p>{m.updates}</p>}
              {m.stays && <p><span className="ms-how-sub">Only a new server changes:</span> {m.stays}</p>}
            </>
          : <p>{m.stays || 'This is part of the server, so a new server brings its new versions.'}</p>}
      </div>

      {m.unavailable && <p className="ms-note">{m.unavailable}</p>}
      {m.latest && !working && m.installed && (
        <div className="ms-latest">
          <b>Version {m.latest.version} is available</b>
          <span className="muted">Works with servers {m.latest.minServer} to {m.latest.maxServer}{m.latest.prerelease ? ' · pre-release' : ''}{m.latest.size ? ` · ${mb(m.latest.size)} MB` : ''}</span>
          {m.latest.blocked && <span className="ms-bad">This version was taken out of use before: {m.latest.blocked}</span>}
          <ChangeNotes text={m.latest.notes} />
        </div>
      )}
      {m.tooNew && <p className="ms-note">Version {m.tooNew.version} exists but doesn’t fit this server. {m.tooNew.reason}</p>}
      {m.installed && m.pin && <p className="ms-note">Kept on {pinWords(m.pin)} on purpose. Automatic updates leave it alone, though a version withdrawn for security reasons still goes out of use.</p>}

      {m.installed && m.updatable && (
        <Row label="Update this module by itself" dim={!!m.source || !!m.pin || !!m.override || !serverAuto}
          desc={m.override ? 'It was installed with an override, so it never updates by itself. Remove the override, or choose a version that needs none, to let it update again.'
            : m.source ? 'A module from another publisher never updates by itself: you choose each version.'
            : m.pin ? 'It is pinned, so automatic updates leave it alone whatever this says.'
            : !serverAuto ? 'Automatic updates are off for the whole server, above, so this changes nothing until they are on.'
            : m.autoUpdate ? 'Installs new versions when they appear, if they work with this server.'
            : 'It still looks for new versions and shows them here, and waits for you to install them.'}>
          <Switch label={`Update ${m.name} by itself`} checked={m.autoUpdate && !m.override} disabled={working || !!m.source || !!m.override} onChange={onAuto} />
        </Row>
      )}

      <div className="ms-actions">
        {m.installed && m.pin && m.pin !== m.latest?.version && m.latest && <button type="button" className="btn sm" disabled={working} onClick={onLatest}>Go back to the latest</button>}
        {m.installed && m.latest && !m.pin && <button type="button" className="btn sm" disabled={working} onClick={onUpdate}>Update to {m.latest.version}</button>}
        {m.installed && m.updatable && <button type="button" className="btn sm" disabled={working} onClick={onVersions}>Versions</button>}
        {m.installed && m.updatable && !m.override && <button type="button" className="btn sm ghost" disabled={working} onClick={onPin}>{m.pin ? 'Unpin' : 'Pin this version'}</button>}
        {m.installed && m.canRollback && <button type="button" className="btn sm ghost" disabled={working} onClick={onRollback}>{m.previous ? `Go back to version ${m.previous}` : 'Stop using this version'}</button>}
        {m.installed && !m.core && <button type="button" className="btn sm ghost danger-hover" disabled={working} onClick={onRemove}>Remove</button>}
        <Link className="btn sm ghost" to={`/settings/modules/store/${m.id}`} aria-label={`The store's page for ${m.name}`}>Store page</Link>
        <button type="button" className="btn sm ghost" onClick={onLog} aria-label={`Show the log for ${m.name}`}>Show log</button>
        <Link className="btn sm ghost" to={`/settings/logs?module=${m.id}`} aria-label={`Logging for ${m.name}`}>Logging</Link>
      </div>

      {events.length > 0 && (
        <details className="ms-more">
          <summary>Recent activity</summary>
          <ul className="ms-events">
            {events.map((e, i) => <li key={i}><time dateTime={new Date(e.at).toISOString()}>{when(e.at)}</time><span>{cap(eventWords(e))}</span></li>)}
          </ul>
        </details>
      )}

      {m.installed && !m.source && (m.noFork || m.updatable) && (
        <details className="ms-more">
          <summary>Advanced</summary>
          <p className="muted"><HelpLink to="module-store#using-someones-fork">What using a fork means</HelpLink></p>
          <div className="ms-fork">
            <b>Use someone’s fork of this module</b>
            {m.noFork ? <p className="ms-note">{m.noFork}</p> : <ForkForm m={m} onReport={onReport} />}
          </div>
        </details>
      )}
    </div>
  )
}

// A module that comes from someone else says so, and how to stop.
function PublisherNote({ m, onOfficial, disabled }: { m: ModuleStatus; onOfficial: () => void; disabled: boolean }) {
  const s = m.source!
  return (
    <div className="warn-box ms-publisher">
      <strong>From another publisher: <a href={`https://github.com/${s.repo}`} target="_blank" rel="noreferrer">{s.repo}</a></strong>
      <span>This isn’t the official {m.name}, and nobody here has reviewed it. Keep it only if you trust whoever publishes it. Everything else works as for the official one, and this server won’t install its updates by itself.</span>
      <dl className="ms-facts">
        <div className="wide"><dt>Key fingerprint</dt><dd className="mono">{s.fingerprint || 'Unreadable'}</dd></div>
        <div className="wide"><dt>Added</dt><dd>{s.addedBy ? `by ${s.addedBy}, ` : ''}{when(s.addedAt)}</dd></div>
      </dl>
      <div><button type="button" className="btn sm" disabled={disabled} onClick={onOfficial}>Go back to the official version</button></div>
    </div>
  )
}

// Two steps, because the warning is the server's and comes with the key's fingerprint:
// Check shows it, and only then can someone say they trust the publisher.
function ForkForm({ m, onReport }: { m: ModuleStatus; onReport: (r: ModuleReport) => void }) {
  const [repo, setRepo] = useState('')
  const [key, setKey] = useState('')
  const [warning, setWarning] = useState('')
  const [trust, setTrust] = useState(false)
  const [err, setErr] = useState('')
  const [working, setWorking] = useState(false)
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const warned = useRef<HTMLDivElement>(null)
  const checkBtn = useRef<HTMLButtonElement>(null)
  // The warning comes up where the Check button was: read from its start.
  useEffect(() => { if (warning) warned.current?.focus() }, [warning])
  const edit = (set: (v: string) => void) => (e: React.ChangeEvent<HTMLInputElement>) => { set(e.target.value); setWarning(''); setTrust(false); setErr('') }
  const check = async (e: React.FormEvent) => {
    e.preventDefault()
    if (working) return
    setWorking(true); setErr(''); setTrust(false)
    try {
      // Without confirm the server changes nothing; it can only answer with the warning.
      onReport(await api.put<ModuleReport>(`/modules/${m.id}/source`, { repo, key }))
    } catch (e: any) {
      if (e instanceof ApiError && e.data?.code === 'confirm') setWarning(e.data.warning)
      else { setWarning(''); setErr(e.message) }
    } finally { setWorking(false) }
  }
  const over = useOverrideDialog()
  const saved = (r: ModuleReport) => {
    onReport(r)
    toast(`${m.name} now comes from ${repo.trim()}`)
    setTimeout(() => document.getElementById('mod-' + m.id)?.querySelector<HTMLElement>('.ms-head')?.focus(), 50)
  }
  const confirm = async () => {
    setWorking(true); setErr('')
    try { saved(await api.put<ModuleReport>(`/modules/${m.id}/source`, { repo, key, confirm: true, password, code })) }
    catch (e: any) {
      // On some systems a fork needs an override as well (on macOS, a weaker sandbox).
      const need = needsOverrideOf(e)
      if (need) over.open({ id: m.id, name: m.name, need, send: async (body) => saved(await api.put<ModuleReport>(`/modules/${m.id}/source`, { repo, key, confirm: true, ...body })) })
      else setErr(e.message)
      setCode(''); setWorking(false)
    }
  }
  const { user } = useApp()
  const fingerprint = /fingerprint is ([0-9a-f ]+)\./.exec(warning)?.[1]
  return (
    <form className="ms-fork-form" onSubmit={check}>
      <p className="ms-note">The module is then treated the same as the official one, but its releases come from that repository and must be signed with that key. Only take a fork from someone you trust.</p>
      <label className="set-field">Repository
        <input className="input" value={repo} onChange={edit(setRepo)} placeholder="someone/WittyDVD" spellCheck={false} autoComplete="off" autoCapitalize="off" />
      </label>
      <label className="set-field">The publisher’s public key
        <input className="input mono" value={key} onChange={edit(setKey)} placeholder="44 characters, from wittymodule keygen" spellCheck={false} autoComplete="off" autoCapitalize="off" />
      </label>
      {err && <p className="error" role="alert">{err}</p>}
      {!warning && <div><button type="submit" ref={checkBtn} className="btn sm" disabled={!repo.trim() || !key.trim()} aria-busy={working}>{working ? 'Checking…' : 'Check'}</button></div>}
      {warning && (
        <div className="warn-box ms-fork-warn" role="alert" tabIndex={-1} ref={warned}>
          <span>{warning}</span>
          {fingerprint && <span className="ms-print"><span className="muted">Key fingerprint</span><code>{fingerprint}</code></span>}
          <label className="ms-trust"><input type="checkbox" checked={trust} onChange={(e) => setTrust(e.target.checked)} /> I trust this publisher</label>
          <OwnerFields password={password} code={code} setPassword={setPassword} setCode={setCode} />
          <div className="ms-actions">
            <button type="button" className="btn sm danger" disabled={!trust || working || !password || (!!user?.twoFactor && !code)} onClick={confirm}>{working ? 'Saving…' : 'Confirm'}</button>
            <button type="button" className="btn sm ghost" disabled={working} onClick={() => { setWarning(''); setTrust(false); setTimeout(() => checkBtn.current?.focus(), 0) }}>Cancel</button>
          </div>
        </div>
      )}
      {over.dialog}
    </form>
  )
}

// Every version the last check knew of, explained, without going online.
function VersionsDialog({ m, onClose, onReport }: { m: ModuleStatus; onClose: () => void; onReport: (r: ModuleReport) => void }) {
  const [list, setList] = useState<VersionList | null>(null)
  const [err, setErr] = useState('')
  const [checking, setChecking] = useState(false)
  const [pending, setPending] = useState<{ v: VersionInfo; warning: string } | null>(null)
  const [needPassword, setNeedPassword] = useState<VersionInfo | null>(null)
  const over = useOverrideDialog()
  // Versions made for an older server that someone chose Install anyway for, in its pop-up.
  const older = useOlderAsk()
  const olderOk = useRef<Set<string>>(new Set())
  const load = () => api.get<VersionList>(`/modules/${m.id}/versions`).then((l) => { setList(l); setErr('') }).catch((e) => setErr(e.message))
  useEffect(() => { void load() }, [m.id])
  // Check now is the only thing here that goes online, and someone has to press it.
  const check = async () => {
    if (checking) return
    setChecking(true)
    try {
      const n = await api.post<ModuleReport>('/modules/check')
      onReport(n)
      if (n.checkError) setErr(n.checkError)
      await load()
    } catch (e: any) { setErr(e.message) } finally { setChecking(false) }
  }
  const backToButton = (ver: string) => setTimeout(() => document.getElementById('ver-' + ver)?.focus(), 50)
  const post = (v: VersionInfo, confirm: boolean, secrets?: { password: string; code: string }) =>
    api.post<ModuleReport>(`/modules/${m.id}/install`, { version: v.version, confirm, ...(olderOk.current.has(v.version) ? { confirmOlder: true } : {}), ...secrets })
  const run = async (v: VersionInfo, confirm: boolean) => {
    try {
      onReport(await post(v, confirm))
      onClose()
    } catch (e: any) {
      const need = needsOverrideOf(e)
      const old = olderOf(e)
      if (need) over.open(overrideAskFor(m, v.version, need, (n) => { onReport(n); onClose() }))
      else if (old && !olderOk.current.has(v.version)) { if (await askOlder(v, old)) await run(v, true); else backToButton(v.version) }
      else if (e instanceof ApiError && e.data?.code === 'confirm') setPending({ v, warning: e.data.warning })
      else if (e instanceof ApiError && e.data?.code === 'password-needed') setNeedPassword(v)
      else { setErr(e.message); backToButton(v.version) }
    }
  }
  // The pop-up for a version made for an older server says everything its warning says, so Install
  // anyway there answers both.
  const askOlder = async (v: VersionInfo, list: OlderVersion[]) => {
    const yes = await older.ask(list.map((o) => ({ ...o, warning: v.warning || o.warning })))
    if (!list.every((o) => yes.includes(o.module))) return false
    olderOk.current.add(v.version)
    return true
  }
  const choose = async (v: VersionInfo) => {
    if (v.olderServer) {
      if (await askOlder(v, [{ module: m.id, name: m.name, version: v.version, madeFor: v.madeFor || v.maxServer, server: '', warning: v.warning || '' }])) void run(v, true)
      else backToButton(v.version)
      return
    }
    if (v.warning) setPending({ v, warning: v.warning }); else void run(v, false)
  }
  const pinAct = async (call: () => Promise<ModuleReport>) => {
    try { onReport(await call()); await load() } catch (e: any) { setErr(e.message) }
  }
  const pin = list?.pin ?? m.pin
  const none = !!list && list.versions.length === 0
  return (
    <>
      <Dialog title={`${m.name} versions`} size="wide" className="set-modal ms-versions" onClose={onClose}>

          <div className="ms-vers-top">
            <span>{pin
              ? <><b>Pinned</b> to {pinWords(pin)}. Automatic updates leave it alone.</>
              : 'Not pinned, so automatic updates can move it if you turned them on.'}
              {list?.checkedAt ? ` Last looked ${when(list.checkedAt)}.` : ' The server hasn’t looked yet.'}
            </span>
            <span className="ms-actions">
              {pin && <button type="button" className="btn sm" onClick={() => pinAct(() => api.post<ModuleReport>(`/modules/${m.id}/latest`))}>Return to latest</button>}
              <button type="button" className="btn sm ghost" aria-busy={checking} onClick={check}><IconRefresh /> {checking ? 'Checking…' : 'Check now'}</button>
            </span>
          </div>
          {busy(m) && <p className="ms-note" role="status">{PHASE[m.phase]}…</p>}
          {m.source && <p className="warn-box">These releases come from another publisher: {m.source.repo}.</p>}
          {err && <p className="error" role="alert">{err}</p>}
          {!list && !err && <div role="status" className="muted">Reading the list…</div>}
          {none && <p className="ms-note">No versions are known so far. {list?.checkedAt ? 'There were none when the server last looked.' : 'Press Check now to look.'}</p>}

          {list && (
            <ul className="ms-vers">
              {list.versions.map((v) => <VersionRow key={v.version} v={v} working={busy(m)} onChoose={() => choose(v)} />)}
            </ul>
          )}
      </Dialog>
      {pending && <Confirm ask={{ title: `Use ${m.name} ${pending.v.version}?`, body: <p>{pending.warning}</p>, action: 'Install anyway', run: () => run(pending.v, true) }}
        onClose={() => { setPending(null); backToButton(pending.v.version) }} />}
      {needPassword && <ConfirmOwner ask={{ title: `Go back to ${m.name} ${needPassword.version}?`, body: <p>An older version may lack security fixes, so this asks for your password.</p>, action: 'Go back',
        run: async (password, code) => { onReport(await post(needPassword, true, { password, code })); onClose() } }}
        onClose={() => { setNeedPassword(null); backToButton(needPassword.version) }} />}
      {over.dialog}
      {older.dialog}
    </>
  )
}

function VersionRow({ v, working, onChoose }: { v: VersionInfo; working: boolean; onChoose: () => void }) {
  const link = safeUrl(v.url)
  const can = !v.inUse && (v.allowed || !!v.override)
  return (
    <li className={'ms-ver-row' + (v.inUse ? ' on' : '') + (v.withdrawn ? ' withdrawn' : '') + (!v.inUse && !v.allowed ? ' no' : '')}>
      <div className="ms-ver-head">
        <div className="ms-ver-title">
          <b>{v.version}</b>
          {v.published ? <span className="muted">{day(v.published)}</span> : null}
          <span className="ms-tags">
            {v.inUse && <Tag tone="ok" icon={<IconCheck size={12} />}>In use</Tag>}
            {v.installed && !v.inUse && <Tag>Installed</Tag>}
            {v.prerelease && <Tag>Pre-release</Tag>}
            {v.older && <Tag>Older</Tag>}
            {v.olderServer && <Tag tone="warn" icon={<IconInfo size={12} />}>Made for an older server</Tag>}
            {v.withdrawn && <Tag tone="bad" icon={<IconInfo size={12} />}>Withdrawn</Tag>}
          </span>
        </div>
        {!v.inUse && <button type="button" id={'ver-' + v.version} className="btn sm" disabled={!can || working} onClick={onChoose} aria-label={`${v.installed ? 'Switch to' : 'Install'} version ${v.version}${!v.allowed && v.override ? ', with an override' : v.olderServer ? ', made for an older server: asks first' : ''}`}>{!v.allowed && v.override ? 'Install anyway…' : v.installed ? 'Switch to this' : v.olderServer ? 'Install…' : 'Install'}</button>}
      </div>
      <div className="muted ms-ver-fit">Works with servers {v.minServer} to {v.maxServer}{v.size ? ` · ${mb(v.size)} MB` : ''}{link && <> · <a href={link} target="_blank" rel="noreferrer">On GitHub</a></>}</div>
      {v.withdrawn && <p className="ms-bad">Withdrawn {severity(v.withdrawn.severity)}: {v.withdrawn.reason}</p>}
      {v.override && !v.allowed && <p className="ms-why">Needs an override: {overrideLines(v.override).map((l) => l.reason || l.title).join(' ')}</p>}
      {v.refused && !v.override && !/withdrawn/i.test(v.refused) && <p className="ms-why">Can’t be chosen: {v.refused}</p>}
      {v.blocked && !v.withdrawn && <p className="ms-why">Taken out of use before: {v.blocked}</p>}
      <details className="ms-more">
        <summary>Release notes</summary>
        <ChangeNotes text={v.notes} />
      </details>
    </li>
  )
}

// What the override dialog sends for a version of an installed module: the same install request,
// confirmed, with the password, code and override beside the version.
function overrideAskFor(m: ModuleStatus, version: string, need: NeedsOverride, done: (r: ModuleReport) => void): OverrideAsk {
  return {
    id: m.id, name: `${m.name} ${version}`, need: { ...need, version: need.version || version },
    send: async (body) => {
      const r = await api.post<ModuleReport>(`/modules/${m.id}/install`, { version, confirm: true, ...body })
      toast(`${m.name} ${version} is being installed`)
      done(r)
    },
  }
}
