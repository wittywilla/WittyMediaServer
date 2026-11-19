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
import { t, tn, tk, tx, date } from '../../i18n'

// The module store: what this server can do, by what it is for, to add and take away,
// with the versions, pins and forks of the ones that have releases of their own. Nothing
// on this page goes online unless an admin presses Check now or turned the automatic
// check on; a version list only reads what the last check found.

const PHASE: Record<string, string> = { downloading: tk('Downloading'), verifying: tk('Checking the signature'), installing: tk('Installing') }
const busy = (m: ModuleStatus) => !!PHASE[m.phase]
const mb = (n = 0) => (n / 1048576).toFixed(1)
const when = (ms: number) => fmtDateTime(ms)
const day = (ms: number) => date(ms, { dateStyle: 'medium' })
const kindName = (type: string) => tx(LIB_TYPES.find((x) => x.type === type)?.name || type)
const needsLook = (m: ModuleStatus) => m.state === 'fallback' || m.phase === 'failed' || !!m.withdrawn || !!m.notice || (m.installed && !!m.olderServer)
const moduleIcon = (m: ModuleStatus) => m.libraries?.length ? libIcon(m.libraries[0]) : m.group === 'tv' ? <IconTv /> : <IconPlug />
// A release's address comes unsigned from GitHub, so only a plain link to GitHub is used.
const safeUrl = (u?: string) => u && /^https:\/\/github\.com\//.test(u) ? u : ''

type Filter = 'all' | 'installed' | 'absent' | 'updates'
const FILTERS: { id: Filter; label: string; test: (m: ModuleStatus) => boolean }[] = [
  { id: 'all', label: tk('All'), test: () => true },
  { id: 'installed', label: tk('Installed'), test: (m) => m.installed },
  { id: 'absent', label: tk('Not installed'), test: (m) => !m.installed },
  { id: 'updates', label: tk('Updates'), test: (m) => m.installed && !!m.latest },
]

// What happened to a module, in words.
function eventWords(e: ModuleEvent): string {
  const fill = { version: e.version, detail: e.detail }
  // The English is marked with tk below; one of the four is shown, translated.
  const four = (vd: string, v: string, d: string, none: string) => t(e.version ? (e.detail ? vd : v) : (e.detail ? d : none), fill)
  switch (e.kind) {
    case 'added': return t('Added to this server')
    case 'removed': return t('Removed from this server')
    case 'failed': return four(tk('Version {version} failed: {detail}'), tk('Version {version} failed'), tk('A version failed: {detail}'), tk('A version failed'))
    case 'taken-out-of-use': return four(tk('Version {version} was taken out of use: {detail}'), tk('Version {version} was taken out of use'), tk('A version was taken out of use: {detail}'), tk('A version was taken out of use'))
    case 'put-back': return four(tk('Version {version} was put back in use: {detail}'), tk('Version {version} was put back in use'), tk('A version was put back in use: {detail}'), tk('A version was put back in use'))
    case 'rolled-back': return e.version ? t('Went back from version {version}', fill) : t('Went back from a version')
    case 'pinned': return e.version ? t('Kept on version {version}', fill) : t('Kept on this version')
    case 'unpinned': return t('Automatic updates can move it again')
    case 'switched': return e.detail ? t('Switched {detail}', fill) : t('Switched to version {version}', fill)
    case 'source-changed': return t('Publisher changed: {detail}', { detail: e.detail || '' })
    case 'withdrawn': return four(tk('Version {version} was withdrawn by its publisher: {detail}'), tk('Version {version} was withdrawn by its publisher'), tk('A version was withdrawn by its publisher: {detail}'), tk('A version was withdrawn by its publisher'))
    default: return [e.kind.replace(/-/g, ' '), e.version ? `version ${e.version}` : '', e.detail].filter(Boolean).join(', ')
  }
}

// Why a version was withdrawn, in a whole sentence.
const withdrawnWords = (w: { version?: string; severity: string; reason: string }, named: boolean) => named
  ? (w.severity === 'security' ? t('Version {version} was withdrawn for security reasons: {reason}', { version: w.version, reason: w.reason }) : t('Version {version} was withdrawn because it was broken: {reason}', { version: w.version, reason: w.reason }))
  : (w.severity === 'security' ? t('Withdrawn for security reasons: {reason}', { reason: w.reason }) : t('Withdrawn because it was broken: {reason}', { reason: w.reason }))

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

type Ask = { title: string; body: ReactNode; action: string; danger?: boolean; run: () => Promise<void> | void }

// A question before something that can't be taken back with one press. The dialog's
// keyboard handling comes from useDialogs, as for every .modal.
function Confirm({ ask, onClose }: { ask: Ask; onClose: () => void }) {
  const [working, setWorking] = useState(false)
  const yes = async () => { setWorking(true); try { await ask.run() } finally { setWorking(false); onClose() } }
  return (
    <Dialog title={ask.title} onClose={onClose} className="set-modal" actions={<><button className="btn ghost" autoFocus onClick={onClose}>{t('Cancel')}</button>
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
  const store = useStoreView()
  const [logOf, setLogOf] = useState<ModuleStatus | null>(null)
  const features = useFeatures()
  const { user } = useApp()

  useEffect(() => {
    api.get<ModuleReport>('/modules').then(setR).catch((e) => setFailed(e.message))
    api.get<Record<string, string>>('/settings').then(setSettings).catch(() => {})
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
      if (n.checkError) toast(t('The check didn’t work. What happened is on the page.'))
      else if (!n.modules.some((m) => m.latest)) toast(t('Every module is up to date'))
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
    try { setR(await api.post<ModuleReport>(`/modules/${m.id}/add`)); toast(t('{name} is being installed', { name: m.name })); void store.reload() } catch (e: any) {
      const old = olderOf(e)
      if (!old) { toast(e.message); focusRow(m.id); return }
      const yes = await older.ask(old)
      if (old.every((o) => yes.includes(o.module))) await act(() => api.post<ModuleReport>(`/modules/${m.id}/add`, { confirmOlder: true }), t('{name} is being installed', { name: m.name }))
      else toast(t('{name} wasn’t installed', { name: m.name }))
    }
    focusRow(m.id)
  }
  const remove = (m: ModuleStatus) => { const keep = keepDataChoice(m.name); setAsk({
    title: t('Remove {name}?', { name: m.name }), action: t('Remove'), danger: true,
    body: <>
      <p>{t('{name} stops showing in the app and stops checking for updates.', { name: m.name })}{m.libraries?.length ? ' ' + t('New libraries of these kinds can’t be made: {kinds}.', { kinds: m.libraries.map(kindName).join(', ') }) : ''} {t('Your files and watch history stay as they are.')}</p>
      <p>{m.source ? t('Every version of it that was downloaded is deleted, which frees the disk space, and the fork it comes from is forgotten.') : t('Every version of it that was downloaded is deleted, which frees the disk space.')} {t('You can install it again from the store at any time.')}</p>
      {!!m.requiredBy?.length && <p>{tn('{names} is built on it.', '{names} are built on it.', m.requiredBy.length, { names: m.requiredBy.join(', ') })}</p>}
      {keep.node}
    </>,
    run: async () => {
      setRefused(null)
      try { setR(await api.post<ModuleReport>(`/modules/${m.id}/remove${keep.query()}`)); toast(t('{name} removed', { name: m.name })); focusRow(m.id) }
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
      else if (e instanceof ApiError && e.data?.code === 'confirm') setAsk({ title: t('Update {name} to {version}?', { name: m.name, version }), body: <p>{tx(e.data.warning)}</p>, action: t('Update anyway'), run: async () => {
        try { setR(await install(true)); focusRow(m.id) } catch (x) { const n = needsOverrideOf(x); if (n) overrideInstall(m, version, n); else toast(x instanceof Error ? x.message : String(x)) }
      } })
      else toast(e.message)
    }
  }
  // A release held after its download, waiting for the go-ahead.
  const goAhead = (m: ModuleStatus) => { if (m.pending) overrideInstall(m, m.pending.version, { ...m.pending.needsOverride, version: m.pending.version, pending: true }) }
  const rollback = (m: ModuleStatus) => setOwnerAsk({
    title: m.previous ? t('Go back to version {version}?', { version: m.previous }) : t('Stop using {name} {version}?', { name: m.name, version: m.version }), action: m.previous ? t('Go back') : t('Stop using it'),
    body: <p>{t('{name} stops using version {version}, and that version is marked as taken out of use, so automatic updates won’t bring it back. You can still choose it yourself.', { name: m.name, version: m.version })} {m.previous ? t('An older version may lack security fixes, so this asks for your password.') : t('There’s no earlier version here, so it won’t be working until you install another. This asks for your password.')}</p>,
    run: async (password, code) => { setR(await api.post<ModuleReport>(`/modules/${m.id}/rollback`, { password, code })); toast(m.previous ? t('{name} went back to version {version}', { name: m.name, version: m.previous }) : t('{name} is no longer in use', { name: m.name })); focusRow(m.id) },
  })
  const pin = (m: ModuleStatus) => act(() => m.pin ? api.del<ModuleReport>(`/modules/${m.id}/pin`) : api.post<ModuleReport>(`/modules/${m.id}/pin`), m.pin ? t('{name} can update again', { name: m.name }) : t('{name} stays on this version', { name: m.name }))
  const setAuto = (m: ModuleStatus, on: boolean) => act(() => api.put<ModuleReport>(`/modules/${m.id}/autoupdate`, { on }), on ? t('{name} can update by itself', { name: m.name }) : t('{name} will wait for you to update it', { name: m.name }))
  const toLatest = (m: ModuleStatus) => act(() => api.post<ModuleReport>(`/modules/${m.id}/latest`), t('{name} is going back to the latest', { name: m.name }))
  const official = (m: ModuleStatus) => setAsk({
    title: t('Go back to the official version?'), action: t('Go back'),
    body: <p>{t('{name} goes back to the releases of {repo}. Versions downloaded from {fork} are deleted, and it isn’t on this server until you install the official one.', { name: m.name, repo: m.stockRepo, fork: m.source?.repo })}</p>,
    run: async () => { await act(() => api.del<ModuleReport>(`/modules/${m.id}/source`), t('{name} is back to the official version', { name: m.name })); focusRow(m.id) },
  })

  if (failed) return (
    <div>
      <PageHead title={tk('Modules')} />
      <div className="set-card set-empty big"><IconPlug size={34} /><b>{t('Modules aren’t available')}</b><p>{failed}</p></div>
    </div>
  )
  if (!r) return <div><PageHead title={tk('Modules')} /><Spinner /></div>
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
      <PageHead title={tk('Modules')} desc={t('What this server can do, one piece at a time. Add what you want and remove what you don’t.')}
        actions={<LinkButton variant="primary" to="/settings/modules/store" icon={<IconPlug />}>{t('Browse the store')}</LinkButton>} />

      {store.view && <WhatsNew view={store.view} />}

      <SpecPage embedded page={modulesTopSpec({
        autoCheck, autoInstall: settings?.moduleAutoInstall === '1', loaded: !!settings, checkedAt: r.checkedAt ? when(r.checkedAt) : '', held: held ? when(held) : '', checkError: r.checkError || '', checking,
        server: r.server, api: String(r.minApi === r.api ? r.api : t('{from} to {to}', { from: r.minApi, to: r.api })),
        filters: FILTERS.map((f) => ({ id: f.id, label: tx(f.label), count: r.modules.filter(f.test).length })), filter,
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
          <b>{filter === 'updates' ? t('No updates known') : filter === 'absent' ? t('Every module is on this server') : filter === 'installed' ? t('No modules yet') : t('Nothing here')}</b>
          {filter === 'updates' && <p>{r.checkedAt ? t('Nothing newer was found when the server last looked, {when}.', { when: when(r.checkedAt) }) : t('The server hasn’t looked yet.')}</p>}
          {filter === 'installed' && <p>{t('The server works without any.')} <Link to="/settings/modules/store">{t('Browse the store')}</Link> {t('to add what you want.')}</p>}
          {filter === 'updates' && <button className="btn sm" onClick={check} aria-busy={checking}><IconRefresh /> {checking ? t('Checking…') : t('Check now')}</button>}
        </div>
      )}

      {groups.map((g) => <Section key={g.id} id={'shelf-' + g.id} title={g.title}>{g.modules.map(row)}</Section>)}

      {core.length > 0 && (
        <div className="ms-quiet">
          <Section title={tk('What other modules are built on')} desc={t('These stay for as long as something uses them, and can’t be removed on their own.')}>{core.map(row)}</Section>
        </div>
      )}

      {canUpload ? (
        <Section id="custom-modules" title={tk('Custom modules')} actions={<InstallFromFile onInstalled={() => { api.get<ModuleReport>('/modules').then(setR).catch(() => {}); void store.reload() }} />}
          desc={t('Install a module from its release archive: one you built, or one someone gave you. The server checks the file and says what it found before anything is installed. Replacing an existing module with someone’s fork is done from that module above, under Advanced.')}>
          <p className="ms-note">{t('A module is someone’s code running on your server, so only install one from a publisher you trust. Anything unsigned, signed by a key this server doesn’t trust, or new to this server needs an override, gets no permissions you don’t tick, and never updates by itself.')} <HelpLink to="module-store#installing-from-a-file">{t('Installing from a file')}</HelpLink></p>
        </Section>
      ) : filter === 'all' && (
        <div className="ms-quiet">
          <Section title={tk('Custom modules')} actions={<span className="set-pill">{t('In development')}</span>}
            desc={t('Adding a wholly new kind of module isn’t available yet. Replacing an existing module with someone’s fork is: open that module above, then Advanced.')}>
            <p className="ms-note">{t('When it comes, the same rule will apply: a module is someone’s code running on your server, so only take one from a publisher you trust.')}</p>
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
      {!m.installed ? <Tag tone="off">{m.comesInside ? t('Comes with other modules') : m.unavailable ? t('No build here') : t('Not installed')}</Tag>
        : busy(m) ? <Tag tone="new">{tx(PHASE[m.phase])}</Tag>
        : needsLook(m) ? <Tag tone="bad" icon={<IconInfo size={12} />}>{t('Needs a look')}</Tag>
        : m.latest ? <Tag tone="new">{t('Update available')}</Tag>
        : <Tag tone="ok" icon={<IconCheck size={12} />}>{t('Installed')}</Tag>}
      {m.pending && <Tag tone="warn">{t('Needs your go-ahead')}</Tag>}
      {m.installed && m.olderServer && <Tag tone="warn" icon={<IconInfo size={12} />}>{t('Made for an older server')}</Tag>}
      {!m.installed && !m.latest && m.olderOffer && <Tag tone="warn">{t('Only for an older server')}</Tag>}
      {m.installed && m.pin && <Tag>{t('Pinned')}</Tag>}
      {m.source && <Tag tone="warn" icon={<IconShield size={12} />}>{t('Other publisher')}</Tag>}
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
        {!m.installed && !m.unavailable && !m.comesInside && <button type="button" className={'btn sm' + (marked ? ' primary' : '')} onClick={p.onAdd} aria-label={t('Install {name}', { name: m.name })}><IconPlus /> <span className="ms-add-word">{t('Install')}</span></button>}
        {m.installed && m.latest && !m.pin && !busy(m) && !isOpen && <button type="button" className="btn sm" onClick={p.onUpdate} aria-label={t('Update {name} to {version}', { name: m.name, version: m.latest.version })}>{t('Update')}</button>}
      </div>
      {busy(m) && (
        <div className="upd-progress ms-progress" role="status">
          <span>{tx(PHASE[m.phase])}{m.phase === 'downloading' && m.total ? ' · ' + t('{done} of {total} MB', { done: mb(m.done), total: mb(m.total) }) : ''}…</span>
          <progress value={m.phase === 'downloading' && m.total ? m.done : undefined} max={m.total || undefined} aria-label={`${m.name}: ${tx(PHASE[m.phase])}`} />
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
      <b>{kind?.label || usual}</b>{kind?.label && <span className="muted"> {t('(usually {name})', { name: usual })}</span>}
      {others.length > 0 && (
        <p className="ms-note">{t('Also brought by {names}. They share this kind of library, so any one of them is enough to make one, and each plays what it knows.', { names: others.join(', ') })}{' '}
          {!editing && <button type="button" className="btn sm ghost" onClick={() => setEditing(true)} aria-label={t('Rename the kind {name}', { name: kind?.label || usual })}>{t('Rename')}</button>}
        </p>
      )}
      {editing && (
        <form className="ms-kind-form" onSubmit={(e) => { e.preventDefault(); save(text) }}>
          <label>{t('Call it, on this server')}
            <input className="input" autoFocus maxLength={40} value={text} placeholder={usual} onChange={(e) => setText(e.target.value)} />
          </label>
          <div className="ms-actions">
            <button type="submit" className="btn sm" disabled={busyNaming}>{t('Save')}</button>
            {kind?.label && <button type="button" className="btn sm ghost" disabled={busyNaming} onClick={() => save('')}>{t('Use the usual name')}</button>}
            <button type="button" className="btn sm ghost" onClick={() => { setEditing(false); setErr(''); setText(kind?.label || '') }}>{t('Cancel')}</button>
          </div>
          <p className="muted">{t('This only changes what the kind is called when you add a library. Libraries keep their own names.')}</p>
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
  const latest = m.latest ? m.latest.version : checkedAt ? t('This is the newest known') : t('Not checked yet')
  return (
    <div className="ms-detail" id={id}>
      <PendingNotice m={m} onGo={onGoAhead} onDropped={onOverride} />
      {m.override && <OverrideRecordBox id={m.id} name={m.name} record={m.override} disabled={working} onRemoved={onOverride} />}
      {statusWarns(releaseStatus(m.releaseStatus)) && <StatusNotice status={m.releaseStatus} note={m.statusNote} name={m.name} installed={m.installed} />}
      {m.source && <PublisherNote m={m} onOfficial={onOfficial} disabled={working} />}
      {refused && <p className="error" role="alert">{tx(refused)} <Link to="/settings/libraries">{t('Go to Libraries')}</Link></p>}
      {failure && <FailureNotice failure={failure} name={m.name} moduleId={m.id} repairRoute={repairRoute} onRetry={m.latest ? onUpdate : undefined} />}
      {m.withdrawn && <p className="error">{withdrawnWords(m.withdrawn, true)}</p>}
      {m.installed && m.olderServer && m.olderWarning && (
        <div className="warn-box ms-older" role="note">
          <b>{t('Made for an older server')}</b>
          <p>{tx(m.olderWarning)}</p>
          <div className="ms-actions">
            {m.latest && !working && <button type="button" className="btn sm primary" onClick={onUpdate}>{t('Update to {version}', { version: m.latest.version })}</button>}
            {!working && <button type="button" className={'btn sm' + (m.latest ? ' ghost' : '')} onClick={onRemove}>{t('Remove {name}', { name: m.name })}</button>}
          </div>
        </div>
      )}
      {!m.installed && !m.latest && m.olderOffer && <p className="warn-box">{t('No version of {name} for this server is published yet.', { name: m.name })} {tx(m.olderOffer.reason)} {t('Installing it asks you first.')}</p>}
      {m.lastFailure && !failure && <p className="warn-box">{t('The last problem it had: {problem}', { problem: m.lastFailure })}</p>}

      <dl className="ms-facts">
        {m.installed && <div><dt>{t('In use')}</dt><dd>{m.version}</dd></div>}
        {m.installed && m.updatable && <div><dt>{t('Latest')}</dt><dd>{latest}</dd></div>}
        {!m.installed && <div><dt>{t('Version')}</dt><dd>{m.latest?.version || t('None available yet')}</dd></div>}
        <div><dt>{t('Works with')}</dt><dd>{t('Servers {min} to {max}', { min: m.minServer, max: m.maxServer })}</dd></div>
        {/* For a module nothing here checked (unsigned, an untrusted key, one this server doesn't know), the repository is only what its own file claims. */}
        <div className="wide"><dt>{m.source ? t('Official repository') : unvouched(m) ? t('Says it comes from') : t('Repository')}</dt><dd><a href={`https://github.com/${m.stockRepo || m.repo}`} target="_blank" rel="noreferrer">{m.stockRepo || m.repo}</a>{unvouched(m) && <span className="ms-note"> {t('Nothing checked this: it’s what the module’s own file says.')}</span>}</dd></div>
        {!!m.libraries?.length && <div className="wide"><dt>{t('Brings these libraries')}</dt><dd>{m.libraries.map((t) => <BroughtKind key={t} type={t} m={m} kinds={kinds} onReport={onReport} />)}</dd></div>}
        {!!m.needs?.length && <div className="wide"><dt>{t('What it needs')}</dt><dd>{m.needs.join(', ')}</dd></div>}
        {!!m.requires?.length && <div className="wide"><dt>{t('Built on')}</dt><dd>{m.requires.map(nameOf).join(', ')}</dd></div>}
        {!!m.requiredBy?.length && <div className="wide"><dt>{t('What’s built on it')}</dt><dd>{m.requiredBy.join(', ')}</dd></div>}
      </dl>

      <div className="ms-how">
        <b>{m.updatable ? t('Updates on its own') : t('Updates with the server')}</b>
        {m.updatable
          ? <>
              {m.updates && <p>{tx(m.updates)}</p>}
              {m.stays && <p><span className="ms-how-sub">{t('Only a new server changes:')}</span> {tx(m.stays)}</p>}
            </>
          : <p>{m.stays ? tx(m.stays) : t('This is part of the server, so a new server brings its new versions.')}</p>}
      </div>

      {m.unavailable && <p className="ms-note">{tx(m.unavailable)}</p>}
      {m.latest && !working && m.installed && (
        <div className="ms-latest">
          <b>{t('Version {version} is available', { version: m.latest.version })}</b>
          <span className="muted">{t('Works with servers {min} to {max}', { min: m.latest.minServer, max: m.latest.maxServer })}{m.latest.prerelease ? ' · ' + t('pre-release') : ''}{m.latest.size ? ' · ' + t('{size} MB', { size: mb(m.latest.size) }) : ''}</span>
          {m.latest.blocked && <span className="ms-bad">{t('This version was taken out of use before: {reason}', { reason: m.latest.blocked })}</span>}
          <ChangeNotes text={m.latest.notes} />
        </div>
      )}
      {m.tooNew && <p className="ms-note">{t('Version {version} exists but doesn’t fit this server.', { version: m.tooNew.version })} {tx(m.tooNew.reason)}</p>}
      {m.installed && m.pin && <p className="ms-note">{t('Kept on version {version} on purpose. Automatic updates leave it alone, though a version withdrawn for security reasons still goes out of use.', { version: m.pin })}</p>}

      {m.installed && m.updatable && (
        <Row label={t('Update this module by itself')} dim={!!m.source || !!m.pin || !!m.override || !serverAuto}
          desc={m.override ? t('It was installed with an override, so it never updates by itself. Remove the override, or choose a version that needs none, to let it update again.')
            : m.source ? t('A module from another publisher never updates by itself: you choose each version.')
            : m.pin ? t('It is pinned, so automatic updates leave it alone whatever this says.')
            : !serverAuto ? t('Automatic updates are off for the whole server, above, so this changes nothing until they are on.')
            : m.autoUpdate ? t('Installs new versions when they appear, if they work with this server.')
            : t('It still looks for new versions and shows them here, and waits for you to install them.')}>
          <Switch label={t('Update {name} by itself', { name: m.name })} checked={m.autoUpdate && !m.override} disabled={working || !!m.source || !!m.override} onChange={onAuto} />
        </Row>
      )}

      <div className="ms-actions">
        {m.installed && m.pin && m.pin !== m.latest?.version && m.latest && <button type="button" className="btn sm" disabled={working} onClick={onLatest}>{t('Go back to the latest')}</button>}
        {m.installed && m.latest && !m.pin && <button type="button" className="btn sm" disabled={working} onClick={onUpdate}>{t('Update to {version}', { version: m.latest.version })}</button>}
        {m.installed && m.updatable && <button type="button" className="btn sm" disabled={working} onClick={onVersions}>{t('Versions')}</button>}
        {m.installed && m.updatable && !m.override && <button type="button" className="btn sm ghost" disabled={working} onClick={onPin}>{m.pin ? t('Unpin') : t('Pin this version')}</button>}
        {m.installed && m.canRollback && <button type="button" className="btn sm ghost" disabled={working} onClick={onRollback}>{m.previous ? t('Go back to version {version}', { version: m.previous }) : t('Stop using this version')}</button>}
        {m.installed && !m.core && <button type="button" className="btn sm ghost danger-hover" disabled={working} onClick={onRemove}>{t('Remove')}</button>}
        <Link className="btn sm ghost" to={`/settings/modules/store/${m.id}`} aria-label={t('The store’s page for {name}', { name: m.name })}>{t('Store page')}</Link>
        <button type="button" className="btn sm ghost" onClick={onLog} aria-label={t('Show the log for {name}', { name: m.name })}>{t('Show log')}</button>
        <Link className="btn sm ghost" to={`/settings/logs?module=${m.id}`} aria-label={t('Logging for {name}', { name: m.name })}>{t('Logging')}</Link>
      </div>

      {events.length > 0 && (
        <details className="ms-more">
          <summary>{t('Recent activity')}</summary>
          <ul className="ms-events">
            {events.map((e, i) => <li key={i}><time dateTime={new Date(e.at).toISOString()}>{when(e.at)}</time><span>{cap(eventWords(e))}</span></li>)}
          </ul>
        </details>
      )}

      {m.installed && !m.source && (m.noFork || m.updatable) && (
        <details className="ms-more">
          <summary>{t('Advanced')}</summary>
          <p className="muted"><HelpLink to="module-store#using-someones-fork">{t('What using a fork means')}</HelpLink></p>
          <div className="ms-fork">
            <b>{t('Use someone’s fork of this module')}</b>
            {m.noFork ? <p className="ms-note">{tx(m.noFork)}</p> : <ForkForm m={m} onReport={onReport} />}
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
      <strong>{t('From another publisher:')} <a href={`https://github.com/${s.repo}`} target="_blank" rel="noreferrer">{s.repo}</a></strong>
      <span>{t('This isn’t the official {name}, and nobody here has reviewed it. Keep it only if you trust whoever publishes it. Everything else works as for the official one, and this server won’t install its updates by itself.', { name: m.name })}</span>
      <dl className="ms-facts">
        <div className="wide"><dt>{t('Key fingerprint')}</dt><dd className="mono">{s.fingerprint || t('Unreadable')}</dd></div>
        <div className="wide"><dt>{t('Added')}</dt><dd>{s.addedBy ? t('by {name}, {when}', { name: s.addedBy, when: when(s.addedAt) }) : when(s.addedAt)}</dd></div>
      </dl>
      <div><button type="button" className="btn sm" disabled={disabled} onClick={onOfficial}>{t('Go back to the official version')}</button></div>
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
    toast(t('{name} now comes from {repo}', { name: m.name, repo: repo.trim() }))
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
      <p className="ms-note">{t('The module is then treated the same as the official one, but its releases come from that repository and must be signed with that key. Only take a fork from someone you trust.')}</p>
      <label className="set-field">{t('Repository')}
        <input className="input" value={repo} onChange={edit(setRepo)} placeholder="someone/WittyDVD" spellCheck={false} autoComplete="off" autoCapitalize="off" />
      </label>
      <label className="set-field">{t('The publisher’s public key')}
        <input className="input mono" value={key} onChange={edit(setKey)} placeholder={t('44 characters, from wittymodule keygen')} spellCheck={false} autoComplete="off" autoCapitalize="off" />
      </label>
      {err && <p className="error" role="alert">{err}</p>}
      {!warning && <div><button type="submit" ref={checkBtn} className="btn sm" disabled={!repo.trim() || !key.trim()} aria-busy={working}>{working ? t('Checking…') : t('Check')}</button></div>}
      {warning && (
        <div className="warn-box ms-fork-warn" role="alert" tabIndex={-1} ref={warned}>
          <span>{tx(warning)}</span>
          {fingerprint && <span className="ms-print"><span className="muted">{t('Key fingerprint')}</span><code>{fingerprint}</code></span>}
          <label className="ms-trust"><input type="checkbox" checked={trust} onChange={(e) => setTrust(e.target.checked)} /> {t('I trust this publisher')}</label>
          <OwnerFields password={password} code={code} setPassword={setPassword} setCode={setCode} />
          <div className="ms-actions">
            <button type="button" className="btn sm danger" disabled={!trust || working || !password || (!!user?.twoFactor && !code)} onClick={confirm}>{working ? t('Saving…') : t('Confirm')}</button>
            <button type="button" className="btn sm ghost" disabled={working} onClick={() => { setWarning(''); setTrust(false); setTimeout(() => checkBtn.current?.focus(), 0) }}>{t('Cancel')}</button>
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
      <Dialog title={t('{name} versions', { name: m.name })} size="wide" className="set-modal ms-versions" onClose={onClose}>

          <div className="ms-vers-top">
            <span>{pin
              ? t('Pinned to version {version}. Automatic updates leave it alone.', { version: pin })
              : t('Not pinned, so automatic updates can move it if you turned them on.')}
              {' '}{list?.checkedAt ? t('Last looked {when}.', { when: when(list.checkedAt) }) : t('The server hasn’t looked yet.')}
            </span>
            <span className="ms-actions">
              {pin && <button type="button" className="btn sm" onClick={() => pinAct(() => api.post<ModuleReport>(`/modules/${m.id}/latest`))}>{t('Return to latest')}</button>}
              <button type="button" className="btn sm ghost" aria-busy={checking} onClick={check}><IconRefresh /> {checking ? t('Checking…') : t('Check now')}</button>
            </span>
          </div>
          {busy(m) && <p className="ms-note" role="status">{tx(PHASE[m.phase])}…</p>}
          {m.source && <p className="warn-box">{t('These releases come from another publisher: {repo}.', { repo: m.source.repo })}</p>}
          {err && <p className="error" role="alert">{err}</p>}
          {!list && !err && <div role="status" className="muted">{t('Reading the list…')}</div>}
          {none && <p className="ms-note">{t('No versions are known so far.')} {list?.checkedAt ? t('There were none when the server last looked.') : t('Press Check now to look.')}</p>}

          {list && (
            <ul className="ms-vers">
              {list.versions.map((v) => <VersionRow key={v.version} v={v} working={busy(m)} onChoose={() => choose(v)} />)}
            </ul>
          )}
      </Dialog>
      {pending && <Confirm ask={{ title: t('Use {name} {version}?', { name: m.name, version: pending.v.version }), body: <p>{tx(pending.warning)}</p>, action: t('Install anyway'), run: () => run(pending.v, true) }}
        onClose={() => { setPending(null); backToButton(pending.v.version) }} />}
      {needPassword && <ConfirmOwner ask={{ title: t('Go back to {name} {version}?', { name: m.name, version: needPassword.version }), body: <p>{t('An older version may lack security fixes, so this asks for your password.')}</p>, action: t('Go back'),
        run: async (password, code) => { onReport(await post(needPassword, true, { password, code })); onClose() } }}
        onClose={() => { setNeedPassword(null); backToButton(needPassword.version) }} />}
      {over.dialog}
      {older.dialog}
    </>
  )
}

// What a version's button says to a screen reader.
function versionLabel(v: VersionInfo): string {
  const version = v.version
  if (!v.allowed && v.override) return v.installed ? t('Switch to version {version}, with an override', { version }) : t('Install version {version}, with an override', { version })
  if (v.olderServer) return v.installed ? t('Switch to version {version}, made for an older server: asks first', { version }) : t('Install version {version}, made for an older server: asks first', { version })
  return v.installed ? t('Switch to version {version}', { version }) : t('Install version {version}', { version })
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
            {v.inUse && <Tag tone="ok" icon={<IconCheck size={12} />}>{t('In use')}</Tag>}
            {v.installed && !v.inUse && <Tag>{t('Installed')}</Tag>}
            {v.prerelease && <Tag>{t('Pre-release')}</Tag>}
            {v.older && <Tag>{t('Older')}</Tag>}
            {v.olderServer && <Tag tone="warn" icon={<IconInfo size={12} />}>{t('Made for an older server')}</Tag>}
            {v.withdrawn && <Tag tone="bad" icon={<IconInfo size={12} />}>{t('Withdrawn')}</Tag>}
          </span>
        </div>
        {!v.inUse && <button type="button" id={'ver-' + v.version} className="btn sm" disabled={!can || working} onClick={onChoose} aria-label={versionLabel(v)}>{!v.allowed && v.override ? t('Install anyway…') : v.installed ? t('Switch to this') : v.olderServer ? t('Install…') : t('Install')}</button>}
      </div>
      <div className="muted ms-ver-fit">{t('Works with servers {min} to {max}', { min: v.minServer, max: v.maxServer })}{v.size ? ' · ' + t('{size} MB', { size: mb(v.size) }) : ''}{link && <> · <a href={link} target="_blank" rel="noreferrer">{t('On GitHub')}</a></>}</div>
      {v.withdrawn && <p className="ms-bad">{withdrawnWords(v.withdrawn, false)}</p>}
      {v.override && !v.allowed && <p className="ms-why">{t('Needs an override: {reasons}', { reasons: overrideLines(v.override).map((l) => l.reason || l.title).join(' ') })}</p>}
      {v.refused && !v.override && !/withdrawn/i.test(v.refused) && <p className="ms-why">{t('Can’t be chosen: {reason}', { reason: v.refused })}</p>}
      {v.blocked && !v.withdrawn && <p className="ms-why">{t('Taken out of use before: {reason}', { reason: v.blocked })}</p>}
      <details className="ms-more">
        <summary>{t('Release notes')}</summary>
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
      toast(t('{name} {version} is being installed', { name: m.name, version }))
      done(r)
    },
  }
}
