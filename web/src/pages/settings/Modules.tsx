import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { Link, useLocation, useSearchParams } from 'react-router-dom'
import { api, ApiError } from '../../api'
import { LIB_TYPES } from '../../components/AddLibrary'
import { Spinner } from '../../components/common'
import { IconCheck, IconClose, IconDown, IconInfo, IconPlug, IconPlus, IconRefresh, IconShield, IconTv, libIcon } from '../../icons'
import { shelves, type LibraryKind, type ModuleEvent, type ModuleReport, type ModuleStatus, type VersionInfo, type VersionList } from '../../moduleStore'
import { toast } from '../../state'
import { PageHead, Row, Section, Switch } from './ui'
import './modules.css'
import LegalNote from '../../components/LegalNote'

// The module store: what this server can do, by what it is for, to add and take away,
// with the versions, pins and forks of the ones that have releases of their own. Nothing
// on this page goes online unless an admin presses Check now or turned the automatic
// check on; a version list only reads what the last check found.

const PHASE: Record<string, string> = { downloading: 'Downloading', verifying: 'Checking the signature', installing: 'Installing' }
const busy = (m: ModuleStatus) => !!PHASE[m.phase]
const mb = (n = 0) => (n / 1048576).toFixed(1)
const when = (ms: number) => new Date(ms).toLocaleString()
const day = (ms: number) => new Date(ms).toLocaleDateString()
const kindName = (t: string) => LIB_TYPES.find((x) => x.type === t)?.name || t
const needsLook = (m: ModuleStatus) => m.state === 'fallback' || m.phase === 'failed' || !!m.withdrawn || !!m.notice
const backTo = (m: ModuleStatus) => m.previous ? `version ${m.previous}` : `the built-in copy (${m.builtinVersion})`
const severity = (s: string) => s === 'security' ? 'for security reasons' : 'because it was broken'
const pinWords = (pin: string) => pin === 'builtin' ? 'the built-in copy' : `version ${pin}`
const moduleIcon = (m: ModuleStatus) => m.libraries?.length ? libIcon(m.libraries[0]) : m.group === 'tv' ? <IconTv /> : <IconPlug />
// A release's address comes unsigned from GitHub, so only a plain link to GitHub is used.
const safeUrl = (u?: string) => u && /^https:\/\/github\.com\//.test(u) ? u : ''

type Filter = 'all' | 'installed' | 'absent' | 'updates'
const FILTERS: { id: Filter; label: string; test: (m: ModuleStatus) => boolean }[] = [
  { id: 'all', label: 'All', test: () => true },
  { id: 'installed', label: 'Installed', test: (m) => m.installed },
  { id: 'absent', label: 'Not added', test: (m) => !m.installed },
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
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal set-modal" role="alertdialog" onClick={(e) => e.stopPropagation()}>
        <h2>{ask.title}</h2>
        <div className="ms-ask">{ask.body}</div>
        <div className="actions">
          <button className="btn ghost" autoFocus onClick={onClose}>Cancel</button>
          <button className={'btn ' + (ask.danger ? 'danger' : 'primary')} disabled={working} onClick={yes}>{ask.action}</button>
        </div>
      </div>
    </div>
  )
}

export default function ModulesSettings() {
  const [r, setR] = useState<ModuleReport | null>(null)
  const [failed, setFailed] = useState('')
  const [checking, setChecking] = useState(false)
  const [settings, setSettings] = useState<Record<string, string> | null>(null)
  const [filter, setFilter] = useState<Filter>('all')
  const [open, setOpen] = useState<string | null>(null)
  const [hi, setHi] = useState('')
  const [ask, setAsk] = useState<Ask | null>(null)
  const [refused, setRefused] = useState<{ id: string; why: string } | null>(null) // why it couldn't be removed
  const [versionsOf, setVersionsOf] = useState<ModuleStatus | null>(null)
  const [params] = useSearchParams()
  const loc = useLocation()
  const target = params.get('add') || loc.hash.slice(1)
  const reached = useRef('')
  const [hostReady, setHostReady] = useState(false)

  useEffect(() => {
    api.get<ModuleReport>('/modules').then(setR).catch((e) => setFailed(e.message))
    api.get<Record<string, string>>('/settings').then(setSettings).catch(() => {})
    api.get<{ ready: boolean }>('/moonlight').then((m) => setHostReady(m.ready)).catch(() => {})
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
      const n = await api.post<ModuleReport>('/modules/check')
      setR(n)
      if (n.checkError) toast(n.checkError)
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
    try { setR(await call()); if (done) toast(done) } catch (e: any) { toast(e.message) }
  }
  const add = async (m: ModuleStatus) => {
    await act(() => api.post<ModuleReport>(`/modules/${m.id}/add`), `${m.name} added. It works straight away.`)
    focusRow(m.id)
  }
  const remove = (m: ModuleStatus) => setAsk({
    title: `Remove ${m.name}?`, action: 'Remove', danger: true,
    body: <>
      <p>{m.name} stops showing in the app and stops checking for updates.{m.libraries?.length ? ` New libraries of these kinds can’t be made: ${m.libraries.map(kindName).join(', ')}.` : ''} Your files, watch history and settings stay as they are.</p>
      <p>Its code stays on the server, as part of the server, so this doesn’t free up disk space. Any versions of it that were downloaded are deleted{m.source ? ', and the fork it comes from is forgotten' : ''}. You can add it back any time.</p>
      {!!m.requiredBy?.length && <p>{m.requiredBy.join(', ')} {m.requiredBy.length === 1 ? 'is' : 'are'} built on it.</p>}
    </>,
    run: async () => {
      setRefused(null)
      try { setR(await api.post<ModuleReport>(`/modules/${m.id}/remove`)); toast(`${m.name} removed`); focusRow(m.id) }
      catch (e: any) { setRefused({ id: m.id, why: e.message }) }
    },
  })
  const update = async (m: ModuleStatus) => {
    if (!m.latest) return
    const version = m.latest.version
    const install = (confirm: boolean) => api.post<ModuleReport>(`/modules/${m.id}/install`, { version, confirm })
    try { setR(await install(false)); focusRow(m.id) } catch (e: any) {
      if (e instanceof ApiError && e.data?.code === 'confirm') setAsk({ title: `Update ${m.name} to ${version}?`, body: <p>{e.data.warning}</p>, action: 'Update anyway', run: async () => { await act(() => install(true)); focusRow(m.id) } })
      else toast(e.message)
    }
  }
  const rollback = (m: ModuleStatus) => setAsk({
    title: `Go back to ${backTo(m)}?`, action: 'Go back',
    body: <p>{m.name} stops using version {m.version}, and that version is marked as taken out of use, so automatic updates won’t bring it back. You can still choose it yourself.</p>,
    run: async () => { await act(() => api.post<ModuleReport>(`/modules/${m.id}/rollback`), `${m.name} went back to ${backTo(m)}`); focusRow(m.id) },
  })
  const pin = (m: ModuleStatus) => act(() => m.pin ? api.del<ModuleReport>(`/modules/${m.id}/pin`) : api.post<ModuleReport>(`/modules/${m.id}/pin`), m.pin ? `${m.name} can update again` : `${m.name} stays on this version`)
  const setAuto = (m: ModuleStatus, on: boolean) => act(() => api.put<ModuleReport>(`/modules/${m.id}/autoupdate`, { on }), on ? `${m.name} can update by itself` : `${m.name} will wait for you to update it`)
  const toLatest = (m: ModuleStatus) => act(() => api.post<ModuleReport>(`/modules/${m.id}/latest`), `${m.name} is going back to the latest`)
  const official = (m: ModuleStatus) => setAsk({
    title: 'Go back to the official version?', action: 'Go back',
    body: <p>{m.name} goes back to the releases of <b>{m.stockRepo}</b>. Versions downloaded from {m.source?.repo} are deleted, and it uses its built-in copy until you choose another.</p>,
    run: async () => { await act(() => api.del<ModuleReport>(`/modules/${m.id}/source`), `${m.name} is back to the official version`); focusRow(m.id) },
  })

  if (failed) return (
    <div>
      <PageHead title="Modules" />
      <div className="set-card set-empty big"><IconPlug size={34} /><b>Modules aren’t available</b><p>{failed}</p></div>
    </div>
  )
  if (!r) return <div><PageHead title="Modules" /><Spinner /></div>

  // Until it is set, the automatic check follows the one for server updates.
  const autoCheck = (settings?.moduleUpdateCheck || settings?.updateCheck || '1') === '1'
  const held = r.holdUntil && r.holdUntil > Date.now() ? r.holdUntil : 0
  const test = FILTERS.find((f) => f.id === filter)!.test
  const shown = r.modules.filter(test)
  const core = shown.filter((m) => m.core)
  const groups = shelves(shown.filter((m) => !m.core))
  const nameOf = (id: string) => r.modules.find((m) => m.id === id)?.name || id
  const row = (m: ModuleStatus) => (
    <ModuleRow key={m.id} m={m} isOpen={open === m.id} marked={hi === m.id} checkedAt={r.checkedAt} nameOf={nameOf} refused={refused?.id === m.id ? refused.why : ''}
      onToggle={() => setOpen(open === m.id ? null : m.id)} onReport={setR}
      onAdd={() => add(m)} onRemove={() => remove(m)} onUpdate={() => update(m)} onVersions={() => setVersionsOf(m)}
      onPin={() => pin(m)} onRollback={() => rollback(m)} onOfficial={() => official(m)} onAuto={(on) => setAuto(m, on)} onLatest={() => toLatest(m)} serverAuto={settings?.moduleAutoInstall === '1' && autoCheck} kinds={r.libraries || []} />
  )

  return (
    <div className="ms-page">
      <PageHead title="Modules" desc="What this server can do, one piece at a time. Add what you want and remove what you don’t. All of it is already part of the server, so adding a module is instant and downloads nothing." />
      <LegalNote kind="modules" className="set-legal" />

      <Section title="Updates" desc="Some modules can update without a new server. Each update is signed, this server installs only what it can verify, and it takes effect straight away, with no restart."
        actions={<button className="btn ghost sm" onClick={check} aria-busy={checking}><IconRefresh /> {checking ? 'Checking…' : 'Check now'}</button>}>
        <Row label="Look for updates automatically" desc={`Asks GitHub twice a day. When this is off, the server only asks when you press Check now.${r.checkedAt ? ` Last checked ${when(r.checkedAt)}.` : ' It hasn’t asked yet.'}`}>
          <Switch label="Look for updates automatically" checked={autoCheck} disabled={!settings} onChange={(v) => save('moduleUpdateCheck', v)} />
        </Row>
        <Row label="Install updates automatically" desc="Installs what the check finds, if it works with this server. A version that fails is taken back out by itself. Pinned modules and forks are left alone." dim={!autoCheck}>
          <Switch label="Install updates automatically" checked={settings?.moduleAutoInstall === '1'} disabled={!settings || !autoCheck} onChange={(v) => save('moduleAutoInstall', v)} />
        </Row>
        {!!held && <p className="upd-msg">GitHub asked this network to wait, so the server won’t ask again until {when(held)}. Everything keeps working as it is.</p>}
        {r.checkError && <p className="upd-msg warn">The last check didn’t work: {r.checkError} Everything keeps working as it is.</p>}
        <p className="ms-note">This server is version {r.server} and has module API {r.minApi === r.api ? r.api : `${r.minApi} to ${r.api}`}.</p>
      </Section>

      <div className="chips ms-filter" role="group" aria-label="Show">
        {FILTERS.map((f) => {
          const n = r.modules.filter(f.test).length
          return <button key={f.id} className={'chip' + (filter === f.id ? ' active' : '')} aria-pressed={filter === f.id} onClick={() => setFilter(f.id)}>{f.label}{f.id !== 'all' && ` · ${n}`}</button>
        })}
      </div>

      {shown.length === 0 && (
        <div className="set-card set-empty">
          <b>{filter === 'updates' ? 'No updates known' : filter === 'absent' ? 'Every module is on this server' : 'Nothing here'}</b>
          {filter === 'updates' && <p>{r.checkedAt ? `Nothing newer was found when the server last asked, ${when(r.checkedAt)}.` : 'The server hasn’t asked GitHub yet.'}</p>}
          {filter === 'updates' && <button className="btn sm" onClick={check} aria-busy={checking}><IconRefresh /> {checking ? 'Checking…' : 'Check now'}</button>}
        </div>
      )}

      {groups.map((g) => <Section key={g.id} id={'shelf-' + g.id} title={g.title}>{g.modules.map(row)}</Section>)}

      {core.length > 0 && (
        <div className="ms-quiet">
          <Section title="Built into everything" desc="Other modules are built on these, so they stay and can’t be removed.">{core.map(row)}</Section>
        </div>
      )}

      {filter === 'all' && (
        <div className="ms-quiet">
          <Section title="Custom modules" actions={<span className="set-pill">In development</span>}
            desc="Adding a wholly new kind of module isn’t available yet. Replacing an existing module with someone’s fork is: open that module above, then Advanced.">
            <p className="ms-note">When it comes, the same rule will apply: a module is someone’s code running on your server, so only take one from a publisher you trust.</p>
          </Section>
        </div>
      )}

      {versionsOf && <VersionsDialog m={r.modules.find((m) => m.id === versionsOf.id) || versionsOf} onClose={() => setVersionsOf(null)} onReport={setR} />}
      {ask && <Confirm ask={ask} onClose={() => setAsk(null)} />}

      <Section title="Installed separately" desc="A program of its own that runs beside this server, under its own license. The server works without it.">
        <div className="mod-list">
          <div className="mod-row">
            <span className="mod-name"><b>WittyMoonlight Host</b><span className="muted">Lets people play on Game PCs in the page, built on Moonlight’s own code. License: GPL-3.0. <a href="https://github.com/wittywilla/WittyMoonlightHost" target="_blank" rel="noreferrer noopener">Source</a></span></span>
            <span className="mod-ver"><span className="mod-local">{hostReady ? 'Installed' : 'Not installed'}</span></span>
          </div>
        </div>
      </Section>
    </div>
  )
}

function Tag({ tone, icon, children }: { tone?: 'ok' | 'warn' | 'bad' | 'new' | 'off'; icon?: ReactNode; children: ReactNode }) {
  return <span className={'ms-tag' + (tone ? ' ' + tone : '')}>{icon}{children}</span>
}

function Tags({ m }: { m: ModuleStatus }) {
  return (
    <span className="ms-tags">
      {!m.installed ? <Tag tone="off">Not added</Tag>
        : busy(m) ? <Tag tone="new">{PHASE[m.phase]}</Tag>
        : needsLook(m) ? <Tag tone="bad" icon={<IconInfo size={12} />}>Needs a look</Tag>
        : m.latest ? <Tag tone="new">Update available</Tag>
        : <Tag tone="ok" icon={<IconCheck size={12} />}>Installed</Tag>}
      {m.installed && m.pin && <Tag>Pinned</Tag>}
      {m.source && <Tag tone="warn" icon={<IconShield size={12} />}>Other publisher</Tag>}
    </span>
  )
}

type RowProps = {
  m: ModuleStatus; isOpen: boolean; marked: boolean; checkedAt?: number; nameOf: (id: string) => string; refused: string
  onToggle: () => void; onReport: (r: ModuleReport) => void
  onAdd: () => void; onRemove: () => void; onUpdate: () => void; onVersions: () => void; onPin: () => void; onRollback: () => void; onOfficial: () => void
  onAuto: (on: boolean) => void; onLatest: () => void; serverAuto: boolean; kinds: LibraryKind[]
}

// One module: a line to scan, and everything else when it is opened.
function ModuleRow(p: RowProps) {
  const { m, isOpen, marked } = p
  const detail = useId()
  return (
    <article id={'mod-' + m.id} className={'ms-mod' + (marked ? ' marked' : '') + (m.installed ? '' : ' absent')} aria-label={m.name}>
      <div className="ms-row">
        <button type="button" className="ms-head" aria-expanded={isOpen} aria-controls={isOpen ? detail : undefined} onClick={p.onToggle}>
          <span className="ms-ic">{moduleIcon(m)}</span>
          <span className="ms-title">
            <span className="ms-name"><b>{m.name}</b>{m.installed && <span className="ms-ver">{m.version}</span>}{m.license && <span className="ms-ver">{m.license}</span>}<Tags m={m} /></span>
            {m.summary && <span className="ms-sum">{m.summary}</span>}
          </span>
          <span className="ms-chev"><IconDown size={20} /></span>
        </button>
        {!m.installed && <button type="button" className={'btn sm' + (marked ? ' primary' : '')} onClick={p.onAdd} aria-label={`Add ${m.name}`}><IconPlus /> <span className="ms-add-word">Add</span></button>}
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

function ModuleDetail({ id, m, checkedAt, nameOf, refused, onReport, kinds, onRemove, onUpdate, onVersions, onPin, onRollback, onOfficial, onAuto, onLatest, serverAuto }: RowProps & { id: string }) {
  const working = busy(m)
  const events = [...(m.events || [])].reverse().slice(0, 8)
  const latest = m.latest ? m.latest.version : checkedAt ? 'This is the newest known' : 'Not checked yet'
  return (
    <div className="ms-detail" id={id}>
      {m.source && <PublisherNote m={m} onOfficial={onOfficial} disabled={working} />}
      {refused && <p className="error" role="alert">{refused} <Link to="/settings/libraries">Go to Libraries</Link></p>}
      {m.notice && <p className="error" role="status">{m.notice}</p>}
      {m.withdrawn && <p className="error">Version {m.withdrawn.version} was withdrawn {severity(m.withdrawn.severity)}: {m.withdrawn.reason}</p>}
      {m.phase === 'failed' && <p className="error" role="status">The update wasn’t installed, and nothing changed: {m.error}</p>}
      {m.lastFailure && m.phase !== 'failed' && <p className="warn-box">The last problem it had: {m.lastFailure}</p>}

      <dl className="ms-facts">
        {m.installed && <div><dt>In use</dt><dd>{m.version}{m.updatable && m.state !== 'installed' ? ' (the built-in copy)' : ''}</dd></div>}
        {m.installed && m.updatable && <div><dt>Latest</dt><dd>{latest}</dd></div>}
        {!m.installed && <div><dt>Version</dt><dd>{m.builtinVersion}</dd></div>}
        <div><dt>Works with</dt><dd>Servers {m.minServer} to {m.maxServer}</dd></div>
        <div className="wide"><dt>{m.source ? 'Official repository' : 'Repository'}</dt><dd><a href={`https://github.com/${m.stockRepo || m.repo}`} target="_blank" rel="noreferrer">{m.stockRepo || m.repo}</a></dd></div>
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

      {m.latest && !working && m.installed && (
        <div className="ms-latest">
          <b>Version {m.latest.version} is available</b>
          <span className="muted">Works with servers {m.latest.minServer} to {m.latest.maxServer}{m.latest.prerelease ? ' · pre-release' : ''}{m.latest.size ? ` · ${mb(m.latest.size)} MB` : ''}</span>
          {m.latest.blocked && <span className="ms-bad">This version was taken out of use before: {m.latest.blocked}</span>}
        </div>
      )}
      {m.tooNew && <p className="ms-note">Version {m.tooNew.version} exists but doesn’t fit this server. {m.tooNew.reason}</p>}
      {m.installed && m.pin && <p className="ms-note">Kept on {pinWords(m.pin)} on purpose. Automatic updates leave it alone, though a version withdrawn for security reasons still goes out of use.</p>}

      {m.installed && m.updatable && (
        <Row label="Update this module by itself" dim={!!m.source || !!m.pin || !serverAuto}
          desc={m.source ? 'A module from another publisher never updates by itself: you choose each version.'
            : m.pin ? 'It is pinned, so automatic updates leave it alone whatever this says.'
            : !serverAuto ? 'Automatic updates are off for the whole server, above, so this changes nothing until they are on.'
            : m.autoUpdate ? 'Installs new versions when they appear, if they work with this server.'
            : 'It still looks for new versions and shows them here, and waits for you to install them.'}>
          <Switch label={`Update ${m.name} by itself`} checked={m.autoUpdate} disabled={working || !!m.source} onChange={onAuto} />
        </Row>
      )}

      <div className="ms-actions">
        {m.installed && m.pin && m.pin !== m.latest?.version && (m.latest || m.pin === 'builtin' || m.version !== m.builtinVersion) && <button type="button" className="btn sm" disabled={working} onClick={onLatest}>Go back to the latest</button>}
        {m.installed && m.latest && !m.pin && <button type="button" className="btn sm" disabled={working} onClick={onUpdate}>Update to {m.latest.version}</button>}
        {m.installed && m.updatable && <button type="button" className="btn sm" disabled={working} onClick={onVersions}>Versions</button>}
        {m.installed && m.updatable && <button type="button" className="btn sm ghost" disabled={working} onClick={onPin}>{m.pin ? 'Unpin' : 'Pin this version'}</button>}
        {m.installed && m.canRollback && <button type="button" className="btn sm ghost" disabled={working} onClick={onRollback}>Go back to {backTo(m)}</button>}
        {m.installed && !m.core && <button type="button" className="btn sm ghost danger-hover" disabled={working} onClick={onRemove}>Remove</button>}
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
  const confirm = async () => {
    setWorking(true); setErr('')
    try {
      onReport(await api.put<ModuleReport>(`/modules/${m.id}/source`, { repo, key, confirm: true }))
      toast(`${m.name} now comes from ${repo.trim()}`)
      setTimeout(() => document.getElementById('mod-' + m.id)?.querySelector<HTMLElement>('.ms-head')?.focus(), 50)
    }
    catch (e: any) { setErr(e.message); setWorking(false) }
  }
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
          <div className="ms-actions">
            <button type="button" className="btn sm danger" disabled={!trust || working} onClick={confirm}>{working ? 'Saving…' : 'Confirm'}</button>
            <button type="button" className="btn sm ghost" disabled={working} onClick={() => { setWarning(''); setTrust(false); setTimeout(() => checkBtn.current?.focus(), 0) }}>Cancel</button>
          </div>
        </div>
      )}
    </form>
  )
}

// Every version the last check knew of, explained, without going online.
function VersionsDialog({ m, onClose, onReport }: { m: ModuleStatus; onClose: () => void; onReport: (r: ModuleReport) => void }) {
  const [list, setList] = useState<VersionList | null>(null)
  const [err, setErr] = useState('')
  const [checking, setChecking] = useState(false)
  const [pending, setPending] = useState<{ v: VersionInfo; warning: string } | null>(null)
  const load = () => api.get<VersionList>(`/modules/${m.id}/versions`).then((l) => { setList(l); setErr('') }).catch((e) => setErr(e.message))
  useEffect(() => { void load() }, [m.id])
  // Check now is the only thing here that asks GitHub, and someone has to press it.
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
  const run = async (v: VersionInfo, confirm: boolean) => {
    try {
      const n = v.builtin
        ? await api.post<ModuleReport>(`/modules/${m.id}/builtin`)
        : await api.post<ModuleReport>(`/modules/${m.id}/install`, { version: v.version, confirm })
      onReport(n)
      onClose()
    } catch (e: any) {
      if (e instanceof ApiError && e.data?.code === 'confirm') setPending({ v, warning: e.data.warning })
      else { setErr(e.message); backToButton(v.version) }
    }
  }
  const choose = (v: VersionInfo) => { if (v.warning) setPending({ v, warning: v.warning }); else void run(v, false) }
  const pinAct = async (call: () => Promise<ModuleReport>) => {
    try { onReport(await call()); await load() } catch (e: any) { setErr(e.message) }
  }
  const pin = list?.pin ?? m.pin
  const onlyBuiltin = !!list && list.versions.every((v) => v.builtin)
  return (
    <>
      <div className="modal-backdrop" onClick={onClose}>
        <div className="modal set-modal wide ms-versions" role="dialog" aria-label={`${m.name} versions`} onClick={(e) => e.stopPropagation()}>
          <div className="add-lib-modal-head"><h2>{m.name} versions</h2><button type="button" className="btn sm ghost square" onClick={onClose} aria-label="Close"><IconClose /></button></div>

          <div className="ms-vers-top">
            <span>{pin
              ? <><b>Pinned</b> to {pinWords(pin)}. Automatic updates leave it alone.</>
              : 'Not pinned, so automatic updates can move it if you turned them on.'}
              {list?.checkedAt ? ` Last asked GitHub ${when(list.checkedAt)}.` : ' The server hasn’t asked GitHub yet.'}
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
          {onlyBuiltin && <p className="ms-note">Only the built-in copy is known so far. {list?.checkedAt ? 'GitHub had no other releases of this module when the server last asked.' : 'Press Check now to ask GitHub what else there is.'}</p>}

          {list && (
            <ul className="ms-vers">
              {list.versions.map((v) => <VersionRow key={v.version} v={v} working={busy(m)} onChoose={() => choose(v)} />)}
            </ul>
          )}
        </div>
      </div>
      {pending && <Confirm ask={{ title: `Use ${m.name} ${pending.v.version}?`, body: <p>{pending.warning}</p>, action: 'Install anyway', run: () => run(pending.v, true) }}
        onClose={() => { setPending(null); backToButton(pending.v.version) }} />}
    </>
  )
}

function VersionRow({ v, working, onChoose }: { v: VersionInfo; working: boolean; onChoose: () => void }) {
  const link = safeUrl(v.url)
  const can = !v.inUse && v.allowed
  return (
    <li className={'ms-ver-row' + (v.inUse ? ' on' : '') + (v.withdrawn ? ' withdrawn' : '') + (!v.inUse && !v.allowed ? ' no' : '')}>
      <div className="ms-ver-head">
        <div className="ms-ver-title">
          <b>{v.version}</b>
          {v.published ? <span className="muted">{day(v.published)}</span> : null}
          <span className="ms-tags">
            {v.inUse && <Tag tone="ok" icon={<IconCheck size={12} />}>In use</Tag>}
            {v.builtin && <Tag>Built in</Tag>}
            {v.installed && !v.inUse && <Tag>Installed</Tag>}
            {v.prerelease && <Tag>Pre-release</Tag>}
            {v.older && <Tag>Older</Tag>}
            {v.withdrawn && <Tag tone="bad" icon={<IconInfo size={12} />}>Withdrawn</Tag>}
          </span>
        </div>
        {!v.inUse && <button type="button" id={'ver-' + v.version} className="btn sm" disabled={!can || working} onClick={onChoose} aria-label={`${v.builtin || v.installed ? 'Switch to' : 'Install'} version ${v.version}`}>{v.builtin || v.installed ? 'Switch to this' : 'Install'}</button>}
      </div>
      <div className="muted ms-ver-fit">Works with servers {v.minServer} to {v.maxServer}{v.size ? ` · ${mb(v.size)} MB` : ''}{link && <> · <a href={link} target="_blank" rel="noreferrer">On GitHub</a></>}</div>
      {v.withdrawn && <p className="ms-bad">Withdrawn {severity(v.withdrawn.severity)}: {v.withdrawn.reason}</p>}
      {v.refused && !/withdrawn/i.test(v.refused) && <p className="ms-why">Can’t be chosen: {v.refused}</p>}
      {v.blocked && !v.withdrawn && <p className="ms-why">Taken out of use before: {v.blocked}</p>}
      {v.notes && (
        <details className="ms-more">
          <summary>Release notes <span className="muted">(from GitHub, unsigned)</span></summary>
          {/* Plain text on purpose: notes aren't signed, so they must never become markup. */}
          <div className="ms-notes">{v.notes}</div>
        </details>
      )}
    </li>
  )
}
