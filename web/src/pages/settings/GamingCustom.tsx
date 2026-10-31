import { useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { api, ApiError, getToken, serverBase, type GameConsole } from '../../api'
import { Spinner } from '../../components/common'
import { toast } from '../../state'
import { getConsoles } from '../Games'
import { Row, Section, Switch } from './ui'

// Settings, Gaming: an administrator's own cores and consoles. A core is a program
// that runs on the server, so the page says so before anything else, asks for the
// password, shows what the test-load found, and is plain about what nobody has checked.
// The server enforces all of it (internal/server/api_games_custom.go); this page only
// explains it.

type Firmware = { file: string; desc?: string; required: boolean; present: boolean }
type Report = {
  id: string; file: string; size: number; sha256: string; arch: string; binary: { matches: boolean }; name: string; version: string; extensions: string[]
  needFullPath?: boolean; noGame?: boolean; options?: number; needsGpu: 'yes' | 'maybe' | 'no'; gpuWhy?: string; bios: Firmware[]; biosKnown: boolean
  license: string; licenseSource?: string; nonCommercial?: boolean; hasInfo?: boolean; system?: string; notes?: string; assets?: number; systemFiles?: number
  ignored?: string[]; sandboxNote?: string; warnings: string[]; consoles: string[]; source: string; sourceUrl?: string
}
type Version = { sha256: string; version: string; addedAt: number; current: boolean }
export type CustomCore = {
  id: string; name: string; version: string; sha256: string; size: number; license: string; licenseSource?: string; nonCommercial?: boolean
  consoles: string[]; defaultFor: string[]; note?: string; source: string; sourceUrl?: string; addedBy: string; addedAt: number; updatedAt: number
  report: Report; state: { state: 'ok' | 'changed' | 'missing' | 'link'; found?: string }; versions: Version[]
  plays: { starts: number; failures: number; seconds: number }; running: number
}
type ConsoleInput = { id?: string; name: string; short: string; extensions: string[]; aliases?: string[]; bios?: { file: string; for: string; required: boolean }[]; padLike?: string; cores?: string[] }
type ConsoleView = ConsoleInput & { games: number; libraries: number }
type Placed = { key: string; name: string; folder: string; ours: boolean; size: number; refuse?: string }
type Audit = { at: number; user: string; action: string; core?: string; detail?: string }
type Option = { id: string; name: string }
export type CustomStatus = {
  enabled: boolean; locked?: boolean; canEnable: boolean; sandbox: string; haveRunner: boolean; platform: string; libraryExt: string; maxUploadMB: number
  coresDir: string; systemDir: string; cores: CustomCore[]; consoles: ConsoleView[]; placed: Placed[]; audit: Audit[]; padChoices: Option[]; otherCores: Option[]
}
type Staged = { token: string; report: Report; replaces?: string }

const fmtSize = (n: number) => (n >= 1 << 20 ? `${(n / (1 << 20)).toFixed(1)} MB` : `${Math.max(1, Math.round(n / 1024))} KB`)
const fmtHours = (sec: number) => (sec >= 3600 ? `${(sec / 3600).toFixed(1)} hours` : `${Math.round(sec / 60)} minutes`)

// What to say about a licence. It's only ever a claim, so it says whose.
export function licenseText(c: { license?: string; licenseSource?: string; nonCommercial?: boolean }): string {
  const known = c.license || 'Unknown license'
  const whose = c.licenseSource === 'info' ? ', according to its info file' : c.licenseSource === 'table' ? ', going by its name' : c.licenseSource === 'admin' ? ', as you entered it' : ''
  if (c.nonCommercial) return `Non-commercial use only${c.license && !/^non-?commercial$/i.test(c.license) ? ` (${c.license})` : ''}${whose}`
  return known + whose
}

// Multipart with a progress bar. The password goes first, so a wrong one stops the
// upload before it starts.
function upload<T>(path: string, form: FormData, onProgress: (fraction: number) => void): Promise<T> {
  return new Promise((resolve, reject) => {
    const x = new XMLHttpRequest()
    x.open('POST', serverBase() + '/api' + path)
    const tok = getToken()
    if (tok) x.setRequestHeader('Authorization', 'Bearer ' + tok)
    x.upload.onprogress = (e) => { if (e.lengthComputable) onProgress(e.loaded / e.total) }
    x.onload = () => {
      let data: any = null
      try { data = x.responseText ? JSON.parse(x.responseText) : null } catch { data = { error: x.responseText } }
      if (x.status >= 200 && x.status < 300) resolve(data as T)
      else reject(new ApiError(x.status, data?.error || x.statusText, data))
    }
    x.onerror = () => reject(new ApiError(0, "The upload stopped. Check that the server is still running and try again."))
    x.send(form)
  })
}

function Warning({ children }: { children: ReactNode }) {
  return <div className="custom-warning" role="note">{children}</div>
}

function Fail({ children }: { children: ReactNode }) {
  return <div className="custom-fail" role="alert">{children}</div>
}

// ---- turning it on ----

function PasswordDialog({ title, children, action, onSubmit, onClose }: { title: string; children?: ReactNode; action: string; onSubmit: (password: string) => Promise<void>; onClose: () => void }) {
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const go = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setErr('')
    try { await onSubmit(password) } catch (x: any) { setErr(x.message); setBusy(false) }
  }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <form className="modal" role="dialog" aria-label={title} onClick={(e) => e.stopPropagation()} onSubmit={go}>
        <h2>{title}</h2>
        {children}
        <label className="custom-field">Your password
          <input className="input" type="password" autoComplete="current-password" autoFocus value={password} onChange={(e) => setPassword(e.target.value)} />
        </label>
        {err && <Fail>{err}</Fail>}
        <div className="actions">
          <button type="button" className="btn ghost" onClick={onClose}>Cancel</button>
          <button className="btn primary" disabled={busy || !password}>{busy ? 'Working…' : action}</button>
        </div>
      </form>
    </div>
  )
}

const runsAsCode = 'A core is a program, and it runs on this server whenever someone plays. It always runs boxed in, in its own process, never inside the server itself. A box can have holes, though, so only add cores you trust.'

// ---- adding a core ----

function FactList({ r }: { r: Report }) {
  const required = r.bios.filter((b) => b.required)
  return (
    <dl className="gaming-facts custom-facts">
      <dt>It calls itself</dt><dd><b>{r.name}</b></dd>
      <dt>Version</dt><dd>{r.version || 'Not stated'}</dd>
      <dt>Opens</dt><dd>{r.extensions.length ? r.extensions.map((e) => '.' + e).join(', ') : 'Not stated'}</dd>
      <dt>Built for</dt><dd>{r.arch} <span className="custom-ok">✓ matches this server</span></dd>
      <dt>BIOS files</dt>
      <dd>{r.biosKnown
        ? (r.bios.length
          ? <ul className="custom-bios">{r.bios.map((b) => <li key={b.file}><code>{b.file}</code> {b.required ? 'required' : 'optional'}{b.desc ? `, ${b.desc}` : ''}: {b.present ? 'in the BIOS folder' : <b>not here yet</b>}</li>)}</ul>
          : 'None, according to its info file')
        : 'Unknown: only an info file would say'}{required.some((b) => !b.present) ? <><br /><span className="muted">You supply these yourself.</span></> : null}</dd>
      <dt>Graphics card</dt><dd>{r.needsGpu === 'yes' ? 'Yes' : r.needsGpu === 'maybe' ? 'Maybe' : "Doesn't look like it"}{r.gpuWhy ? <><br /><span className="muted">{r.gpuWhy}</span></> : null}</dd>
      <dt>Size</dt><dd>{fmtSize(r.size)}</dd>
      <dt>SHA-256</dt><dd><code className="custom-sha">{r.sha256}</code></dd>
      <dt>Info file</dt><dd>{r.hasInfo ? 'Yes' : 'None'}{r.system ? `, for ${r.system}` : ''}</dd>
      {(r.assets || r.systemFiles) ? <><dt>Support files</dt><dd>{r.assets || 0} in its own folder, {r.systemFiles || 0} for the BIOS folder</dd></> : null}
      {r.options ? <><dt>Settings</dt><dd>{r.options} of its own</dd></> : null}
      {r.sourceUrl ? <><dt>From</dt><dd className="custom-url">{r.sourceUrl}</dd></> : null}
    </dl>
  )
}

function ConsoleChecks({ consoles, assigned, defaults, suggested, extensions, onAssign, onDefault }: {
  consoles: GameConsole[]; assigned: string[]; defaults: string[]; suggested: string[]; extensions: string[]
  onAssign: (list: string[]) => void; onDefault: (list: string[]) => void
}) {
  const order = useMemo(() => [...consoles].sort((a, b) => Number(suggested.includes(b.id)) - Number(suggested.includes(a.id))), [consoles, suggested])
  return (
    <ul className="custom-consoles">
      {order.map((c) => {
        const on = assigned.includes(c.id)
        const fits = extensions.length === 0 || [...c.extensions, ...(c.shared || [])].some((e: string) => extensions.includes(e.replace(/^\./, '')))
        return (
          <li key={c.id}>
            <label>
              <input type="checkbox" checked={on} onChange={(e) => { onAssign(e.target.checked ? [...assigned, c.id] : assigned.filter((x) => x !== c.id)); if (!e.target.checked) onDefault(defaults.filter((x) => x !== c.id)) }} />
              <span>{c.name}{suggested.includes(c.id) && <span className="pill gaming-rec">Fits</span>}{on && !fits && <small className="muted"> doesn’t list this console’s file types</small>}</span>
            </label>
            {on && (
              <label className="custom-default">
                <input type="checkbox" checked={defaults.includes(c.id)} onChange={(e) => onDefault(e.target.checked ? [...defaults, c.id] : defaults.filter((x) => x !== c.id))} />
                Use by default
              </label>
            )}
          </li>
        )
      })}
    </ul>
  )
}

type Source = 'upload' | 'url' | 'placed'

function AddCoreDialog({ status, replaces, onClose, onDone, onReload }: { status: CustomStatus; replaces?: CustomCore; onClose: () => void; onDone: (c: CustomCore) => void; onReload: () => Promise<void> }) {
  const [source, setSource] = useState<Source>('upload')
  const [file, setFile] = useState<File | null>(null)
  const [url, setUrl] = useState('')
  const [sha, setSha] = useState('')
  const [picked, setPicked] = useState('')
  const [password, setPassword] = useState('')
  const [phase, setPhase] = useState<'source' | 'working' | 'review'>('source')
  const [progress, setProgress] = useState<number | null>(null)
  const [err, setErr] = useState('')
  const [staged, setStaged] = useState<Staged | null>(null)
  const [consoles, setConsoles] = useState<GameConsole[]>([])
  const [assigned, setAssigned] = useState<string[]>(replaces?.consoles || [])
  const [defaults, setDefaults] = useState<string[]>(replaces?.defaultFor || [])
  const [name, setName] = useState('')
  const [id, setId] = useState('')
  const [license, setLicense] = useState('')
  const [ack, setAck] = useState(false)
  const [newConsole, setNewConsole] = useState(false)
  const [saving, setSaving] = useState(false)
  useEffect(() => { getConsoles(true).then(setConsoles).catch(() => {}) }, [])
  const usable = status.placed.filter((p) => !p.refuse)

  const test = async (e: FormEvent) => {
    e.preventDefault()
    setErr('')
    setPhase('working')
    setProgress(source === 'upload' ? 0 : null)
    try {
      let res: Staged
      if (source === 'upload') {
        if (!file) throw new Error('Choose a file first.')
        const form = new FormData()
        form.append('password', password) // first, so nothing is read from someone without it
        if (replaces) form.append('replaces', replaces.id)
        if (sha.trim()) form.append('sha256', sha.trim())
        form.append('file', file, file.name)
        res = await upload<Staged>('/gaming/custom/staged/upload', form, (f) => setProgress(f >= 1 ? null : f))
      } else if (source === 'url') {
        res = await api.post<Staged>('/gaming/custom/staged/url', { password, url: url.trim(), sha256: sha.trim(), replaces: replaces?.id })
      } else {
        res = await api.post<Staged>('/gaming/custom/staged/placed', { password, key: picked, replaces: replaces?.id })
      }
      setStaged(res)
      setName(res.report.name)
      setId(res.report.id)
      setLicense(res.report.licenseSource === 'info' || res.report.licenseSource === 'table' ? '' : res.report.license)
      if (!replaces) setAssigned(res.report.consoles)
      setPassword('')
      setPhase('review')
    } catch (x: any) {
      setErr(x.message)
      setPhase('source')
    }
  }
  const cancel = async () => {
    if (staged) await api.del(`/gaming/custom/staged/${staged.token}`).catch(() => {})
    onClose()
  }
  const approve = async () => {
    if (!staged) return
    setSaving(true)
    setErr('')
    try {
      const body: Record<string, unknown> = { acknowledged: true, name, consoles: assigned, defaultFor: defaults }
      if (license.trim()) body.license = license.trim()
      if (!replaces) body.id = id
      onDone(await api.post<CustomCore>(`/gaming/custom/staged/${staged.token}/approve`, body))
    } catch (x: any) { setErr(x.message); setSaving(false) }
  }
  const verb = replaces ? 'Replace' : 'Add'
  const title = replaces ? `Replace ${replaces.name}` : 'Add a custom core'

  if (phase === 'review' && staged) {
    const r = staged.report
    const shown = { license: license.trim() || r.license, licenseSource: license.trim() ? 'admin' : r.licenseSource, nonCommercial: r.nonCommercial || /non.?commercial|\bnc\b/i.test(license) }
    return (
      <div className="modal-backdrop">
        <div className="modal custom-modal" role="dialog" aria-label={title}>
          <h2>{title}</h2>
          <p className="custom-passed"><span className="custom-ok">✓</span> It loaded in the sandbox and answered as a libretro core. That shows it’s a core, not that it’s safe or that it will play your games well.</p>
          <FactList r={r} />
          {r.warnings.map((w) => <Warning key={w}>{w}</Warning>)}
          {r.sandboxNote && <Warning>Part of the sandbox isn’t available on this machine: {r.sandboxNote.replace(/^sandbox: /, '')}</Warning>}
          <div className="custom-license">
            <h3>License</h3>
            <p><span className={'pill' + (shown.nonCommercial ? ' custom-nc' : '')}>{licenseText(shown)}</span></p>
            <p className="muted">This is third-party software that WittyMediaServer hasn’t checked, and it isn’t offered or supported by this project. What it’s licensed for is between you and its authors.</p>
            <label className="custom-field">Correct or fill this in, if you know better
              <input className="input" value={license} placeholder={r.license || 'For example GPL-3.0'} maxLength={200} onChange={(e) => setLicense(e.target.value)} />
            </label>
          </div>
          <div className="custom-row2">
            <label className="custom-field">Name shown
              <input className="input" value={name} maxLength={120} onChange={(e) => setName(e.target.value)} />
            </label>
            {!replaces && (
              <label className="custom-field">Name on this server
                <input className="input" value={id} maxLength={64} pattern="[a-z0-9][a-z0-9_\-]*" title="Small letters, numbers, dashes and underscores" onChange={(e) => setId(e.target.value)} />
              </label>
            )}
          </div>
          <h3>Which consoles is it for?</h3>
          <p className="muted custom-note">Other cores stay available, for each console and for each game. No console fits? <button type="button" className="linkish" onClick={() => setNewConsole(true)}>Make a custom console.</button></p>
          <ConsoleChecks consoles={consoles} assigned={assigned} defaults={defaults} suggested={r.consoles} extensions={r.extensions} onAssign={setAssigned} onDefault={setDefaults} />
          <label className="custom-ack">
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
            <span>I understand this runs as code on this server, in its sandbox, whenever someone plays with it.</span>
          </label>
          {err && <Fail>{err}</Fail>}
          <div className="actions">
            <button className="btn ghost" onClick={cancel}>Cancel</button>
            <button className="btn primary" disabled={!ack || saving} onClick={approve}>{saving ? 'Saving…' : `${verb} this core`}</button>
          </div>
        </div>
        {newConsole && <ConsoleDialog status={status} onClose={() => setNewConsole(false)} onDone={async (c) => {
          setNewConsole(false)
          await onReload()
          setConsoles(await getConsoles(true))
          if (c?.id) setAssigned((a) => [...a, c.id!])
        }} />}
      </div>
    )
  }

  const ready = !!password && (source === 'upload' ? !!file : source === 'url' ? !!url.trim() : !!picked)
  return (
    <div className="modal-backdrop" onClick={phase === 'working' ? undefined : onClose}>
      <form className="modal custom-modal" role="dialog" aria-label={title} onClick={(e) => e.stopPropagation()} onSubmit={test}>
        <h2>{title}</h2>
        {phase === 'working' ? (
          <div className="custom-working" role="status">
            <Spinner />
            <p>{progress !== null ? `Uploading: ${Math.round(progress * 100)}%` : source === 'url' ? 'Downloading it, then testing it in the sandbox…' : 'Testing it in the sandbox…'}</p>
            {progress !== null && <progress value={progress} max={1} aria-label="Upload progress" />}
            <p className="muted">A core gets up to a minute to load. If it crashes or hangs, only the test stops.</p>
          </div>
        ) : (
          <>
            <Warning><b>This adds a program to your server.</b> {runsAsCode}</Warning>
            <p className="muted custom-note">This server is <b>{status.platform}</b>, so it needs a core that ends in <code>{status.libraryExt}</code>, or a <code>.zip</code> holding one. A core’s <code>.info</code> file can go in the zip beside it, with <code>assets/</code> and <code>system/</code> folders for any files it needs.</p>
            <div className="custom-tabs" role="tablist">
              <button type="button" role="tab" aria-selected={source === 'upload'} className={source === 'upload' ? 'on' : ''} onClick={() => setSource('upload')}>Upload a file</button>
              <button type="button" role="tab" aria-selected={source === 'url'} className={source === 'url' ? 'on' : ''} onClick={() => setSource('url')}>Web address</button>
              <button type="button" role="tab" aria-selected={source === 'placed'} className={source === 'placed' ? 'on' : ''} onClick={() => setSource('placed')}>In the cores folder</button>
            </div>
            {source === 'upload' && (
              <label className="custom-field">File (up to {status.maxUploadMB} MB)
                <input className="input" type="file" accept={`${status.libraryExt},.zip`} onChange={(e) => setFile(e.target.files?.[0] || null)} />
              </label>
            )}
            {source === 'url' && (
              <label className="custom-field">Web address
                <input className="input" type="url" inputMode="url" placeholder="https://…" value={url} onChange={(e) => setUrl(e.target.value)} />
                <small className="muted">It has to be https, on the public internet. This server can’t be sent to your own network this way.</small>
              </label>
            )}
            {source !== 'placed' && (
              <label className="custom-field">SHA-256 it should have (optional)
                <input className="input" value={sha} spellCheck={false} placeholder="If the place you got it from publishes one" onChange={(e) => setSha(e.target.value)} />
              </label>
            )}
            {source === 'placed' && (
              <div className="custom-field">
                {status.placed.length === 0
                  ? <p className="muted">Nothing here yet. Put the file in <code>{status.coresDir}</code>, with its <code>.info</code> file beside it if it has one, then open this again.</p>
                  : (
                    <ul className="custom-placed">
                      {status.placed.map((p) => (
                        <li key={p.key}>
                          <label className={p.refuse ? 'dim' : ''}>
                            <input type="radio" name="placed" disabled={!!p.refuse} checked={picked === p.key} onChange={() => setPicked(p.key)} />
                            <span><b>{p.name}</b><small className="muted">{p.size ? fmtSize(p.size) + ' · ' : ''}{p.folder}{p.ours ? '' : ' (stays where it is; a copy is added)'}</small>{p.refuse && <small className="custom-refuse">{p.refuse}</small>}</span>
                          </label>
                        </li>
                      ))}
                    </ul>
                  )}
                {usable.length > 0 && <small className="muted">Once approved, a file in the cores folder moves into the server’s own store.</small>}
              </div>
            )}
            <label className="custom-field">Your password
              <input className="input" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
            </label>
            {err && <Fail><b>That core wasn’t added.</b> {err}</Fail>}
            <div className="actions">
              <button type="button" className="btn ghost" onClick={onClose}>Cancel</button>
              <button className="btn primary" disabled={!ready}>Test this core</button>
            </div>
          </>
        )}
      </form>
    </div>
  )
}

// ---- a core that's here ----

const stateText: Record<string, string> = {
  changed: 'This core’s file has changed since you approved it, so it won’t run. If you didn’t do this, something else has written to your data folder: treat that seriously.',
  missing: 'This core’s file is missing, so it can’t run.',
  link: 'This core’s file has been replaced with a link, so it won’t run.',
}

function CoreCard({ core, status, onReload, onReplace }: { core: CustomCore; status: CustomStatus; onReload: () => Promise<void>; onReplace: () => void }) {
  const [editing, setEditing] = useState(false)
  const [consoles, setConsoles] = useState<GameConsole[]>([])
  const [assigned, setAssigned] = useState(core.consoles)
  const [defaults, setDefaults] = useState(core.defaultFor)
  const [license, setLicense] = useState(core.licenseSource === 'admin' ? core.license : '')
  const [note, setNote] = useState(core.note || '')
  const [dialog, setDialog] = useState<null | 'reapprove' | { rollback: Version } | 'remove' | { files: File }>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  const r = core.report
  const ok = core.state.state === 'ok'
  const startEdit = () => {
    setAssigned(core.consoles); setDefaults(core.defaultFor); setLicense(core.licenseSource === 'admin' ? core.license : ''); setNote(core.note || '')
    getConsoles(true).then(setConsoles).catch(() => {})
    setEditing(true)
  }
  const save = async () => {
    try {
      await api.put(`/gaming/custom/cores/${core.id}`, { consoles: assigned, defaultFor: defaults, license: license.trim(), note })
      toast('Saved')
      setEditing(false)
      await onReload()
    } catch (e: any) { toast(e.message) }
  }
  const remove = async () => {
    try {
      await api.del(`/gaming/custom/cores/${core.id}`)
      toast(`${core.name} removed. Saves are untouched.`)
      setDialog(null)
      await onReload()
    } catch (e: any) { toast(e.message); setDialog(null) }
  }
  return (
    <article className={'custom-core' + (ok ? '' : ' bad')}>
      <header>
        <div>
          <h3>{core.name}{core.version && <span className="muted"> {core.version}</span>}</h3>
          <small className="muted">{core.id}, added by {core.addedBy} on {new Date(core.addedAt * 1000).toLocaleDateString()}, {fmtSize(core.size)}</small>
        </div>
        <span className="pill custom-third">Third-party, unverified</span>
      </header>
      {!ok && <Fail>{stateText[core.state.state]}{core.state.found && <><br />Approved: <code className="custom-sha">{core.sha256}</code><br />Found: <code className="custom-sha">{core.state.found}</code></>}</Fail>}
      <p className="custom-lic"><span className={'pill' + (core.nonCommercial ? ' custom-nc' : '')}>{licenseText(core)}</span></p>
      <p className="custom-for">{core.consoles.length ? <>For {core.consoles.map((c) => {
        const name = status.padChoices.find((p) => p.id === c)?.name || status.consoles.find((p) => p.id === c)?.name || c
        return <span key={c} className="custom-tag">{name}{core.defaultFor.includes(c) ? ' (default)' : ''}</span>
      })}</> : <b>Not assigned to any console yet.</b>}</p>
      {core.plays.starts > 0 && <p className="muted custom-note">Started {core.plays.starts} {core.plays.starts === 1 ? 'time' : 'times'}, {fmtHours(core.plays.seconds)} played{core.plays.failures > 0 ? `, ${core.plays.failures} ended in a failure` : ''}.</p>}
      {core.note && <p className="custom-note">{core.note}</p>}
      <details>
        <summary>What it reports</summary>
        <FactList r={{ ...r, sha256: core.sha256, version: core.version }} />
        {r.warnings.map((w) => <Warning key={w}>{w}</Warning>)}
        {core.versions.length > 1 && (
          <>
            <h4>Versions kept</h4>
            <ul className="custom-versions">
              {core.versions.map((v) => (
                <li key={v.sha256}>
                  <span>{v.version || 'No version given'}<br /><code className="custom-sha">{v.sha256.slice(0, 16)}…</code> <small className="muted">{new Date(v.addedAt * 1000).toLocaleDateString()}</small></span>
                  {v.current ? <span className="muted">In use</span> : <button className="btn sm ghost" onClick={() => setDialog({ rollback: v })}>Go back to this</button>}
                </li>
              ))}
            </ul>
          </>
        )}
      </details>
      {editing && (
        <div className="custom-edit">
          <h4>Consoles</h4>
          <ConsoleChecks consoles={consoles} assigned={assigned} defaults={defaults} suggested={r.consoles} extensions={r.extensions} onAssign={setAssigned} onDefault={setDefaults} />
          <label className="custom-field">License
            <input className="input" value={license} maxLength={200} placeholder={r.license || 'Unknown license'} onChange={(e) => setLicense(e.target.value)} />
          </label>
          <label className="custom-field">Your notes on how it plays
            <textarea className="input" rows={2} maxLength={2000} value={note} onChange={(e) => setNote(e.target.value)} />
          </label>
          <div className="actions">
            <button className="btn ghost sm" onClick={() => setEditing(false)}>Cancel</button>
            <button className="btn primary sm" onClick={save}>Save</button>
          </div>
        </div>
      )}
      {!editing && (
        <div className="custom-actions">
          {!ok && core.state.state === 'changed' && <button className="btn sm primary" onClick={() => setDialog('reapprove')}>Test and approve again</button>}
          <button className="btn sm" onClick={startEdit} disabled={!status.enabled}>Consoles and notes</button>
          <button className="btn sm" onClick={onReplace} disabled={!status.enabled}>Replace</button>
          <button className="btn sm" onClick={() => fileRef.current?.click()} disabled={!status.enabled || !ok}>Add files</button>
          <input ref={fileRef} type="file" hidden accept=".info,.zip" aria-label={`Add files to ${core.name}`} onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ''; if (f) setDialog({ files: f }) }} />
          <button className="btn sm ghost" onClick={() => setDialog('remove')}>Remove</button>
        </div>
      )}
      {dialog === 'remove' && (
        <div className="modal-backdrop" onClick={() => setDialog(null)}>
          <div className="modal" role="dialog" aria-label={`Remove ${core.name}`} onClick={(e) => e.stopPropagation()}>
            <h2>Remove {core.name}?</h2>
            <p>Its file and any versions kept go. <b>Nobody’s saves are touched:</b> they belong to the game, and another core for the same console can read them if it uses the same format.</p>
            {core.running > 0 && <Warning>A game is playing with this core. Stop it first.</Warning>}
            <div className="actions">
              <button className="btn ghost" onClick={() => setDialog(null)}>Cancel</button>
              <button className="btn primary" disabled={core.running > 0} onClick={remove}>Remove it</button>
            </div>
          </div>
        </div>
      )}
      {dialog === 'reapprove' && (
        <PasswordDialog title={`Approve ${core.name} again?`} action="Test and approve" onClose={() => setDialog(null)} onSubmit={async (password) => {
          await api.post(`/gaming/custom/cores/${core.id}/reapprove`, { password })
          toast('Tested and approved')
          setDialog(null)
          await onReload()
        }}>
          <Warning>Only do this if you changed this file yourself. {runsAsCode}</Warning>
        </PasswordDialog>
      )}
      {dialog && typeof dialog === 'object' && 'rollback' in dialog && (
        <PasswordDialog title="Go back to this version?" action="Test and go back" onClose={() => setDialog(null)} onSubmit={async (password) => {
          await api.post(`/gaming/custom/cores/${core.id}/rollback`, { password, sha256: dialog.rollback.sha256 })
          toast('Back on that version')
          setDialog(null)
          await onReload()
        }}>
          <p>{core.name} goes back to {dialog.rollback.version || 'that version'}. Its test-load runs again first. Save states made with a newer version may not load in an older one; battery saves usually do.</p>
        </PasswordDialog>
      )}
      {dialog && typeof dialog === 'object' && 'files' in dialog && (
        <PasswordDialog title={`Add ${dialog.files.name}?`} action="Add" onClose={() => setDialog(null)} onSubmit={async (password) => {
          const form = new FormData()
          form.append('password', password)
          form.append('file', dialog.files, dialog.files.name)
          const res = await upload<{ kept: string[] }>(`/gaming/custom/cores/${core.id}/files`, form, () => {})
          toast(res.kept?.length ? `Added. ${res.kept.length} already in the BIOS folder and left alone.` : 'Added')
          setDialog(null)
          await onReload()
        }}>
          <p>An <code>.info</code> file describes the core. A <code>.zip</code> can hold <code>assets/</code> for the core’s own folder and <code>system/</code> for the BIOS folder, where nothing already there is ever overwritten.</p>
        </PasswordDialog>
      )}
    </article>
  )
}

// ---- custom consoles ----

const splitList = (v: string) => v.split(/[\s,]+/).map((x) => x.trim()).filter(Boolean)
const splitLines = (v: string) => v.split('\n').map((x) => x.trim()).filter(Boolean)

function ConsoleDialog({ status, existing, onClose, onDone }: { status: CustomStatus; existing?: ConsoleView; onClose: () => void; onDone: (c?: ConsoleInput) => void }) {
  const [tab, setTab] = useState<'form' | 'json'>('form')
  const [name, setName] = useState(existing?.name || '')
  const [short, setShort] = useState(existing?.short || '')
  const [exts, setExts] = useState((existing?.extensions || []).join(', '))
  const [aliases, setAliases] = useState((existing?.aliases || []).join('\n'))
  const [bios, setBios] = useState((existing?.bios || []).map((b) => b.file + (b.required ? ' (required)' : '')).join('\n'))
  const [pad, setPad] = useState(existing?.padLike || '')
  const [cores, setCores] = useState<string[]>(existing?.cores || [])
  const [json, setJson] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const fromForm = (): ConsoleInput => ({
    name, short, extensions: splitList(exts), aliases: splitLines(aliases), padLike: pad || undefined, cores,
    bios: splitLines(bios).map((line) => ({ file: line.replace(/\s*\(required\)$/i, ''), for: 'BIOS for this console', required: /\(required\)$/i.test(line) })),
  })
  const toTab = (t: 'form' | 'json') => {
    if (t === 'json') setJson(JSON.stringify(fromForm(), null, 2))
    else {
      try {
        const v = JSON.parse(json) as ConsoleInput
        setName(v.name || ''); setShort(v.short || ''); setExts((v.extensions || []).join(', ')); setAliases((v.aliases || []).join('\n'))
        setBios((v.bios || []).map((b) => b.file + (b.required ? ' (required)' : '')).join('\n')); setPad(v.padLike || ''); setCores(v.cores || [])
      } catch { setErr('That JSON can’t be read, so the form wasn’t changed.'); return }
    }
    setErr('')
    setTab(t)
  }
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setErr('')
    let body: ConsoleInput
    try { body = tab === 'json' ? JSON.parse(json) : fromForm() } catch (x: any) { setErr('That JSON can’t be read: ' + x.message); return }
    setBusy(true)
    try {
      const out = existing ? await api.put<ConsoleInput>(`/gaming/custom/consoles/${existing.id}`, body) : await api.post<ConsoleInput>('/gaming/custom/consoles', body)
      toast(existing ? 'Saved' : `${body.name} added`)
      onDone(out)
    } catch (x: any) { setErr(x.message); setBusy(false) }
  }
  const title = existing ? `Edit ${existing.name}` : 'Add a custom console'
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <form className="modal custom-modal" role="dialog" aria-label={title} onClick={(e) => e.stopPropagation()} onSubmit={submit}>
        <h2>{title}</h2>
        <p className="muted custom-note">For a system our list doesn’t have, so a core for it has a console to belong to. It then shows up when adding a library, like any other console.</p>
        <div className="custom-tabs" role="tablist">
          <button type="button" role="tab" aria-selected={tab === 'form'} className={tab === 'form' ? 'on' : ''} onClick={() => toTab('form')}>Form</button>
          <button type="button" role="tab" aria-selected={tab === 'json'} className={tab === 'json' ? 'on' : ''} onClick={() => toTab('json')}>JSON</button>
        </div>
        {tab === 'form' ? (
          <>
            <div className="custom-row2">
              <label className="custom-field">Name<input className="input" required maxLength={80} value={name} placeholder="Fantasy Box 3000" onChange={(e) => setName(e.target.value)} /></label>
              <label className="custom-field">Short name<input className="input" required maxLength={24} value={short} placeholder="FBox" onChange={(e) => setShort(e.target.value)} /></label>
            </div>
            <label className="custom-field">File extensions
              <input className="input" required value={exts} placeholder=".fbx, .fb3" onChange={(e) => setExts(e.target.value)} />
              <small className="muted">One another console already uses only counts in a library set to this console, or a folder named after it.</small>
            </label>
            <label className="custom-field">Other folder names that mean this console (optional, one a line)
              <textarea className="input" rows={2} value={aliases} onChange={(e) => setAliases(e.target.value)} />
            </label>
            <label className="custom-field">BIOS file names (optional, one a line; add “(required)” after any it can’t run without)
              <textarea className="input" rows={2} value={bios} spellCheck={false} onChange={(e) => setBios(e.target.value)} />
            </label>
            <label className="custom-field">Controller
              <select className="select" value={pad} onChange={(e) => setPad(e.target.value)}>
                <option value="">A full modern pad</option>
                {status.padChoices.map((p) => <option key={p.id} value={p.id}>Like {p.name}</option>)}
              </select>
            </label>
            {status.otherCores.length > 0 && (
              <details>
                <summary>Cores from this server’s list to offer for it (optional)</summary>
                <ul className="custom-consoles">
                  {status.otherCores.map((c) => (
                    <li key={c.id}><label><input type="checkbox" checked={cores.includes(c.id)} onChange={(e) => setCores(e.target.checked ? [...cores, c.id] : cores.filter((x) => x !== c.id))} /><span>{c.name}</span></label></li>
                  ))}
                </ul>
              </details>
            )}
          </>
        ) : (
          <label className="custom-field">One console, as JSON
            <textarea className="input custom-json" rows={14} spellCheck={false} value={json} onChange={(e) => setJson(e.target.value)} />
          </label>
        )}
        {err && <Fail>{err}</Fail>}
        <div className="actions">
          <button type="button" className="btn ghost" onClick={onClose}>Cancel</button>
          <button className="btn primary" disabled={busy}>{busy ? 'Saving…' : existing ? 'Save' : 'Add console'}</button>
        </div>
      </form>
    </div>
  )
}

function ConsolesPart({ status, onReload }: { status: CustomStatus; onReload: () => Promise<void> }) {
  const [editing, setEditing] = useState<ConsoleView | 'new' | null>(null)
  const remove = async (c: ConsoleView) => {
    if (c.games > 0 && !window.confirm(`${c.games} games are sorted under ${c.name}. They'll drop out of their library at the next scan unless another console takes them. Remove it?`)) return
    try { await api.del(`/gaming/custom/consoles/${c.id}`); toast(`${c.name} removed`); await onReload() } catch (e: any) { toast(e.message) }
  }
  return (
    <Section title="Custom consoles" desc="For a system our list doesn’t have, so a custom core has a console to belong to." actions={<button className="btn sm" onClick={() => setEditing('new')}>Add a console</button>}>
      {status.consoles.length === 0
        ? <p className="muted">None yet.</p>
        : (
          <ul className="custom-console-list">
            {status.consoles.map((c) => (
              <li key={c.id}>
                <span><b>{c.name}</b> <small className="muted">{c.extensions.join(', ')}{c.games ? `, ${c.games} ${c.games === 1 ? 'game' : 'games'}` : ''}</small></span>
                <span className="custom-actions">
                  <button className="btn sm ghost" onClick={() => setEditing(c)} aria-label={`Edit ${c.name}`}>Edit</button>
                  <button className="btn sm ghost" onClick={() => remove(c)} aria-label={`Remove ${c.name}`}>Remove</button>
                </span>
              </li>
            ))}
          </ul>
        )}
      {editing && <ConsoleDialog status={status} existing={editing === 'new' ? undefined : editing} onClose={() => setEditing(null)} onDone={async () => { setEditing(null); await onReload() }} />}
    </Section>
  )
}

// ---- the section ----

const auditWords: Record<string, string> = {
  'turned-on': 'Custom cores turned on', 'turned-off': 'Custom cores turned off', added: 'Added', replaced: 'Replaced', changed: 'Changed', removed: 'Removed',
  'add-refused': 'Refused', 'test-load-failed': 'Failed its test-load', 'test-load-passed': 'Passed its test-load', reapproved: 'Approved again', 'reapprove-failed': 'Failed when approving again',
  'rolled-back': 'Went back a version', 'rollback-failed': 'Failed going back a version', 'files-added': 'Files added', 'use-failed': 'Failed during play', 'file-changed': 'File changed!',
  'file-missing': 'File missing', 'file-link': 'File became a link!', 'password-refused': 'Wrong password', 'console-default': 'Console default set', 'game-core': 'Game’s core set',
  'console-added': 'Console added', 'console-changed': 'Console changed', 'console-removed': 'Console removed',
}

export default function CustomCoresSection({ onChanged }: { onChanged: () => void }) {
  const [st, setSt] = useState<CustomStatus | null>(null)
  const [adding, setAdding] = useState<{ replaces?: CustomCore } | null>(null)
  const [enabling, setEnabling] = useState(false)
  const load = async () => {
    try { setSt(await api.get<CustomStatus>('/gaming/custom')) } catch (e: any) { toast(e.message) }
  }
  const reload = async () => { await load(); onChanged() }
  useEffect(() => { load() }, [])
  if (!st) return null
  const turnOff = async () => {
    try { await api.put('/gaming/custom/enabled', { enabled: false }); toast('Custom cores turned off'); await reload() } catch (e: any) { toast(e.message) }
  }
  return (
    <>
      <Section title="Custom cores" desc="Your own libretro cores, for consoles or versions this server doesn’t offer. They’re third-party software, and this project hasn’t checked them.">
        <Row label="Allow custom cores" desc={<>Off by default on every server. {st.locked ? 'Whoever runs this machine has kept this off, and it can’t be changed from here.' : !st.canEnable ? `This system can’t box a core in properly (${st.sandbox}), so they can’t be used here.` : `A core runs as a program on this server, boxed in by ${st.sandbox}. Off stops them from running; nothing is deleted.`}</>}>
          <Switch checked={st.enabled} disabled={!st.canEnable && !st.enabled} label="Allow custom cores" onChange={(v) => { if (v) setEnabling(true); else turnOff() }} />
        </Row>
        {st.enabled && !st.haveRunner && <Warning>The game runner is missing from this install, so a new core can’t be tested. Custom cores can’t be added until it’s back.</Warning>}
        {st.enabled && (
          <div className="custom-add">
            <button className="btn primary" onClick={() => setAdding({})} disabled={!st.haveRunner}>Add a core</button>
          </div>
        )}
        {st.cores.length > 0 && !st.enabled && <p className="muted">These stay here while custom cores are off, and don’t run. You can still remove them.</p>}
        <div className="custom-list">
          {st.cores.map((c) => <CoreCard key={c.id + c.sha256} core={c} status={st} onReload={reload} onReplace={() => setAdding({ replaces: c })} />)}
        </div>
        {st.audit.length > 0 && (
          <details className="custom-audit">
            <summary>What’s happened ({st.audit.length})</summary>
            <ul>
              {st.audit.map((a, i) => (
                <li key={i}>
                  <small className="muted">{new Date(a.at * 1000).toLocaleString()}</small>
                  <span><b>{auditWords[a.action] || a.action}</b>{a.core ? ` ${a.core}` : ''} <small className="muted">by {a.user}</small>{a.detail && <small className="custom-detail">{a.detail}</small>}</span>
                </li>
              ))}
            </ul>
          </details>
        )}
      </Section>
      {st.enabled && <ConsolesPart status={st} onReload={reload} />}
      {adding && (
        <AddCoreDialog status={st} replaces={adding.replaces} onClose={() => setAdding(null)} onReload={load} onDone={async (c) => {
          setAdding(null)
          toast(adding.replaces ? `${c.name} replaced. Saves are untouched.` : `${c.name} added`)
          await reload()
        }} />
      )}
      {enabling && (
        <PasswordDialog title="Allow custom cores?" action="Turn on" onClose={() => setEnabling(false)} onSubmit={async (password) => {
          await api.put('/gaming/custom/enabled', { enabled: true, password })
          toast('Custom cores turned on')
          setEnabling(false)
          await reload()
        }}>
          <Warning>{runsAsCode}</Warning>
          <p className="muted">Only administrators can add one, and it takes your password each time. Kids and guest profiles never see any of this.</p>
        </PasswordDialog>
      )}
    </>
  )
}

// A console's core, for its card in Settings, Gaming.
export function ConsoleCoreSelect({ console: c, onChanged }: { console: GameConsole; onChanged: () => void }) {
  const list = c.installed
  if (!list || list.length < 2) return null
  const value = c.defaultCore || ''
  return (
    <label className="custom-field custom-select">Plays with
      <select className="select" value={value} onChange={async (e) => {
        try { await api.put(`/gaming/consoles/${c.id}/core`, { core: e.target.value }); toast('Saved'); onChanged() } catch (x: any) { toast(x.message) }
      }}>
        <option value="">First one here</option>
        {list.map((x) => <option key={x.id} value={x.id}>{x.name}{x.source === 'custom' ? ' (custom)' : ''}</option>)}
      </select>
    </label>
  )
}

// A game's own core, on its page, for administrators only.
type GameCoreOption = { id: string; name: string; source: string; license: string; nonCommercial?: boolean; starts: number; failures: number; seconds: number; lastError?: string; best?: boolean }
export function GameCorePicker({ itemId }: { itemId: number }) {
  const [info, setInfo] = useState<{ choice: string; effective: string; options: GameCoreOption[] } | null>(null)
  const load = () => api.get<{ choice: string; effective: string; options: GameCoreOption[] }>(`/gaming/games/${itemId}/core`).then(setInfo).catch(() => {})
  useEffect(() => { load() }, [itemId])
  if (!info || info.options.length < 2) return null
  const set = async (core: string) => {
    try { await api.put(`/gaming/games/${itemId}/core`, { core }); toast('Saved'); await load() } catch (e: any) { toast(e.message) }
  }
  const shown = info.options.find((o) => o.id === (info.choice || info.effective))
  return (
    <details className="custom-gamecore">
      <summary>Core: {shown?.name || 'none'}{info.choice ? ' (just for this game)' : ''}</summary>
      <label className="custom-field">Play this game with
        <select className="select" value={info.choice} onChange={(e) => set(e.target.value)}>
          <option value="">The console’s core</option>
          {info.options.map((o) => <option key={o.id} value={o.id}>{o.name}{o.source === 'custom' ? ' (custom)' : ''}{o.best ? ' (has gone best here)' : ''}</option>)}
        </select>
      </label>
      <ul className="custom-gamecore-stats">
        {info.options.filter((o) => o.starts > 0).map((o) => (
          <li key={o.id}><b>{o.name}</b>: {o.starts} {o.starts === 1 ? 'start' : 'starts'}, {fmtHours(o.seconds)}{o.failures ? `, ${o.failures} failed` : ''}{o.lastError ? <small className="muted custom-detail">Last problem: {o.lastError}</small> : null}</li>
        ))}
      </ul>
    </details>
  )
}
