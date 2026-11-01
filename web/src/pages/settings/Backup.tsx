import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { ApiError, fmtDateTime, getToken, serverBase, setToken } from '../../api'
import { Spinner } from '../../components/common'
import { IconDownload, IconLock, IconTrash } from '../../icons'
import { toast } from '../../state'
import { Button, Field, Input, Notice, Progress, Segmented, Select, Tabs } from '../../ui'
import { PageHead, Row, Section, Switch } from './ui'
import './backup.css'

// Settings, Backup: a ZIP of the server's data and metadata, and bringing one back. The
// server does the work, so this page can be left and come back to. Restoring is also what
// first-time setup offers, so it's a part of its own, with no account to ask for there.

type Part = { id: string; label: string; desc: string; default: boolean; needs?: string[]; files?: number; bytes?: number; rows?: number; hasFiles?: boolean }
type Info = { name: string; size: number; created: string; auto: boolean; protected: boolean; version?: string }
type Schedule = { frequency: 'off' | 'daily' | 'weekly'; hour: number; weekday: number; keep: number; parts: string[]; protected: boolean }
type Inspection = {
  name: string; size: number; serverVersion: string; created: string; protected: boolean; older: boolean; parts: Part[]
  accounts: { name: string; admin: boolean; kind?: string }[]; libraries: { name: string; type: string; folders: string[]; items: number }[]
  addresses: string[]; dropped: string[]; handEdited: string[]; problems: string[]; notes: string[]; needed: number
}
type Job = { kind?: 'create' | 'inspect' | 'restore'; running: boolean; phase?: string; done: number; total: number; item?: string; error?: string; needsPassword?: boolean; refused?: boolean; file?: string; result?: string; problems?: string[]; inspection?: Inspection; restarting?: boolean }
type Last = { ok: boolean; mode: string; name?: string; safety?: string; error?: string; notes?: string[]; at: string }
type View = {
  parts: Part[]; files: Info[]; folder: string; schedule: Schedule; lastAuto?: number; lastError?: string; job: Job; pending?: { mode: string; at: string; name?: string }
  last?: Last; safety: { name: string; at: string }[]; canRestart: boolean; twoFactor: boolean; firstRun: boolean
}

const POLL_MS = 1000
const size = (n = 0) => (n >= 1 << 30 ? `${(n / (1 << 30)).toFixed(1)} GB` : n >= 1 << 20 ? `${(n / (1 << 20)).toFixed(n < 10 << 20 ? 1 : 0)} MB` : n >= 1024 ? `${Math.round(n / 1024)} KB` : `${n} bytes`)
const WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

// The requests of this page. In first-time setup there's no sign-in, and from another
// network the setup code goes along.
function client(setupCode?: string) {
  const call = async <T,>(method: string, path: string, body?: unknown): Promise<T> => {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    const tok = getToken()
    if (tok) headers.Authorization = 'Bearer ' + tok
    if (setupCode) headers['X-Setup-Code'] = setupCode
    let r: Response
    try {
      r = await fetch(serverBase() + '/api' + path, { method, headers, credentials: 'same-origin', body: body === undefined ? undefined : JSON.stringify(body) })
    } catch { throw new ApiError(0, "Can't reach your server.") }
    const text = await r.text()
    let json: any = null
    try { json = text ? JSON.parse(text) : null } catch { /* not JSON */ }
    if (!r.ok) throw new ApiError(r.status, json?.error || text || r.statusText)
    return json as T
  }
  return call
}

// An upload shows how far along it is, which fetch can't.
function upload(file: File, setupCode: string | undefined, onProgress: (done: number) => void): Promise<string> {
  return new Promise((ok, no) => {
    const x = new XMLHttpRequest()
    x.open('POST', serverBase() + '/api/backup/restore/upload')
    const tok = getToken()
    if (tok) x.setRequestHeader('Authorization', 'Bearer ' + tok)
    if (setupCode) x.setRequestHeader('X-Setup-Code', setupCode)
    x.setRequestHeader('Content-Type', 'application/zip')
    x.upload.onprogress = (e) => onProgress(e.loaded)
    x.onerror = () => no(new Error("The upload didn't finish. Check the connection and try again."))
    x.onload = () => {
      let json: any = null
      try { json = JSON.parse(x.responseText) } catch { /* not JSON */ }
      if (x.status >= 200 && x.status < 300 && json?.file) ok(json.file)
      else no(new Error(json?.error || 'The upload was refused.'))
    }
    x.send(file)
  })
}

// Waits for the server to go away and come back, then starts the app again.
async function waitForRestart(say: (s: string) => void) {
  const answers = async () => { try { return (await fetch(serverBase() + '/api/status', { cache: 'no-store' })).ok } catch { return false } }
  const started = Date.now()
  let wentAway = false
  while (Date.now() - started < 300000) {
    await new Promise((ok) => setTimeout(ok, 1000))
    const up = await answers()
    if (!up) { wentAway = true; continue }
    if (wentAway || Date.now() - started > 12000) {
      setToken('')
      window.location.href = '/'
      return
    }
  }
  say('The server hasn’t come back. Start it again by hand: the restore happens as it starts.')
}

function useJob(call: ReturnType<typeof client>) {
  const [view, setView] = useState<View | null>(null)
  const [err, setErr] = useState('')
  const load = useCallback(() => call<View>('GET', '/backup').then((v) => { setView(v); setErr(''); return v }).catch((e) => { setErr(message(e)); return null }), [call])
  useEffect(() => { void load() }, [load])
  const running = !!view?.job.running
  useEffect(() => {
    if (!running) return
    const tick = () => { if (document.visibilityState === 'visible') void load() }
    const t = setInterval(tick, POLL_MS)
    document.addEventListener('visibilitychange', tick)
    return () => { clearInterval(t); document.removeEventListener('visibilitychange', tick) }
  }, [running, load])
  return { view, err, load, setView }
}

function JobProgress({ job, onCancel }: { job: Job; onCancel?: () => void }) {
  const pct = job.total > 0 ? Math.min(100, (job.done / job.total) * 100) : undefined
  return (
    <div className="bk-progress">
      <p className="bk-phase" role="status" aria-live="polite">{job.phase || 'Working…'}</p>
      <Progress value={pct} label={job.phase || 'Working'} />
      {job.total > 0 && <p className="bk-quiet">{size(job.done)} of about {size(job.total)}. You can leave this page while it runs.</p>}
      {onCancel && <div className="bk-actions"><Button onClick={onCancel}>Stop</Button></div>}
    </div>
  )
}

// What a part holds, in words: its files and its rows.
const holds = (p: Part) => {
  const words = [p.rows ? `${p.rows.toLocaleString()} ${p.rows === 1 ? 'entry' : 'entries'}` : '', p.files ? `${p.files.toLocaleString()} ${p.files === 1 ? 'file' : 'files'}, ${size(p.bytes)}` : ''].filter(Boolean)
  return words.join(', ') || 'nothing in it'
}

function PartChecks({ parts, chosen, setChosen, legend, sizes, inside }: { parts: Part[]; chosen: Set<string>; setChosen: (s: Set<string>) => void; legend: string; sizes?: Record<string, number>; inside?: boolean }) {
  const flip = (p: Part, on: boolean) => {
    const n = new Set(chosen)
    if (on) { n.add(p.id); (p.needs || []).forEach((x) => n.add(x)) } else {
      n.delete(p.id)
      // Whatever needs this one can't stay on without it.
      parts.filter((o) => (o.needs || []).includes(p.id)).forEach((o) => n.delete(o.id))
    }
    setChosen(n)
  }
  return (
    <fieldset className="bk-parts">
      <legend className="sr-only">{legend}</legend>
      {parts.map((p) => {
        const needs = (p.needs || []).map((id) => parts.find((x) => x.id === id)?.label).filter(Boolean)
        const known = sizes?.[p.id]
        return (
          <label key={p.id} className="bk-part">
            <input type="checkbox" checked={chosen.has(p.id)} onChange={(e) => flip(p, e.target.checked)} />
            <span className="bk-part-text">
              <span className="bk-part-head"><b>{p.label}</b>{inside ? <span className="bk-size">{holds(p)}</span> : known !== undefined && <span className="bk-size">{known > 0 ? `about ${size(known)}` : 'empty'}</span>}</span>
              <span className="bk-quiet">{p.desc}{needs.length > 0 && ` Needs ${needs.join(' and ')} too.`}</span>
            </span>
          </label>
        )
      })}
    </fieldset>
  )
}

function Passwords({ password, again, setPassword, setAgain, min = 8 }: { password: string; again: string; setPassword: (s: string) => void; setAgain: (s: string) => void; min?: number }) {
  const short = password !== '' && password.length < min
  const differs = again !== '' && again !== password
  return (
    <div className="bk-fields">
      <Field label="Password" hint={`At least ${min} characters. If you lose it, nobody can open the backup.`} error={short ? 'That’s too short.' : undefined}>
        <Input type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} />
      </Field>
      <Field label="The same again" error={differs ? 'These aren’t the same.' : undefined}>
        <Input type="password" autoComplete="new-password" value={again} onChange={(e) => setAgain(e.target.value)} />
      </Field>
    </div>
  )
}

// ---------------------------------------------------------------- making one

function MakeBackup({ view, reload, call }: { view: View; reload: () => void; call: ReturnType<typeof client> }) {
  const [chosen, setChosen] = useState(() => new Set(view.parts.filter((p) => p.default).map((p) => p.id)))
  const [sizes, setSizes] = useState<Record<string, number> | null>(null)
  const [protect, setProtect] = useState(true)
  const [password, setPassword] = useState('')
  const [again, setAgain] = useState('')
  const [err, setErr] = useState('')
  useEffect(() => {
    let gone = false
    call<{ parts: { id: string; bytes: number }[] }>('GET', '/backup/estimate').then((r) => {
      if (!gone) setSizes(Object.fromEntries(r.parts.map((p) => [p.id, p.bytes])))
    }).catch(() => {})
    return () => { gone = true }
  }, [call])
  const total = sizes ? [...chosen].reduce((n, id) => n + (sizes[id] || 0), 0) : null
  const job = view.job
  const busy = job.running
  const answer = useRef<HTMLDivElement>(null)
  const was = useRef(false)
  useEffect(() => {
    if (was.current && !busy) requestAnimationFrame(() => answer.current?.focus())
    was.current = busy
  }, [busy])
  const ready = chosen.size > 0 && !busy && (!protect || (password.length >= 8 && password === again))
  const start = async () => {
    setErr('')
    try {
      await call('POST', '/backup/create', { parts: [...chosen], password: protect ? password : '' })
      setPassword(''); setAgain(''); reload()
    } catch (e) { setErr(message(e)) }
  }
  const cancel = () => { call('POST', '/backup/cancel').then(reload).catch((e) => toast(message(e))) }
  return (
    <>
      <Section title="What to include" desc="Your media files are never in a backup, however large or small it is. Neither are modules, game cores, BIOS files, logs or sign-ins.">
        <PartChecks parts={view.parts} chosen={chosen} setChosen={setChosen} legend="What to include" sizes={sizes || undefined} />
        <p className="bk-total" aria-live="polite">{total === null ? 'Working out how large this is…' : <>About <b>{size(total)}</b> before compression.</>}</p>
      </Section>
      <Section title="Protect it with a password" desc="Real encryption: AES-256, with the key made from your password. It also proves nobody has changed the backup.">
        <Row label="Protect with a password" desc={protect ? 'Everything comes back: API keys, two-factor sign-in, share links and this server’s identity.' : undefined}>
          <Switch checked={protect} onChange={setProtect} label="Protect with a password" />
        </Row>
        {protect ? <Passwords password={password} again={again} setPassword={setPassword} setAgain={setAgain} /> : (
          <Notice tone="warn" title="Without a password, secrets are left out">
            API keys, two-factor secrets, share links and logins in addresses aren’t in the file, so you add them again after a restore. Account passwords are in it as hashes, and titles and file names can be read: keep it private. Anything can be changed in a file with no password, so a restore also leaves out settings holding web addresses.
          </Notice>
        )}
      </Section>
      <Section title="Make the backup">
        {busy && job.kind === 'create' ? <JobProgress job={job} onCancel={cancel} /> : (
          <div ref={answer} tabIndex={-1} className="bk-answer">
            {job.kind === 'create' && job.result && (
              <Notice tone={job.problems?.length ? 'warn' : 'ok'} title={job.result}>
                {job.file && <p>It’s below, as <code>{job.file}</code>. A copy on another drive is what saves you if this one fails, so download it.</p>}
                {!!job.problems?.length && <ul className="bk-list">{job.problems.slice(0, 20).map((p) => <li key={p}>{p}</li>)}</ul>}
              </Notice>
            )}
            {job.kind === 'create' && job.error && <Notice tone="danger" title="No backup was made">{job.error}</Notice>}
            {err && <div role="alert"><Notice tone="danger">{err}</Notice></div>}
            {busy && <p className="bk-quiet" role="status">Something else is running. This can start when it’s finished.</p>}
            <div className="bk-actions"><Button variant="primary" disabled={!ready} onClick={start}>Make a backup now</Button></div>
          </div>
        )}
      </Section>
    </>
  )
}

function FileList({ view, reload, call, onRestore }: { view: View; reload: () => void; call: ReturnType<typeof client>; onRestore: (name: string) => void }) {
  const [asking, setAsking] = useState('')
  const [deleting, setDeleting] = useState(false) // what the password asked for below is for
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [err, setErr] = useState('')
  const startAsking = (name: string, del: boolean) => { setAsking(name); setDeleting(del); setPassword(''); setCode(''); setErr('') }
  const download = async () => {
    setErr('')
    try {
      const r = await call<{ url: string }>('POST', `/backup/files/${encodeURIComponent(asking)}/link`, { password, code })
      const a = document.createElement('a')
      a.href = serverBase() + r.url
      a.download = asking
      document.body.appendChild(a)
      a.click()
      a.remove()
      setAsking(''); setPassword(''); setCode('')
    } catch (e) { setErr(message(e)) }
  }
  const remove = async () => {
    setErr('')
    try {
      await call('DELETE', `/backup/files/${encodeURIComponent(asking)}`, { password, code })
      setAsking(''); setPassword(''); setCode('')
      toast('Backup deleted'); reload()
    } catch (e) { setErr(message(e)) }
  }
  return (
    <Section title="Backups on this server" desc={<>In <code>{view.folder}</code>. They stay through a reset that keeps a copy, and stay on the same drive as everything else, so download one to keep it somewhere safer.</>}>
      {view.files.length === 0 ? <p className="bk-quiet">None yet.</p> : (
        <ul className="bk-files">
          {view.files.map((b) => (
            <li key={b.name} className="bk-file">
              <div className="bk-file-text">
                <b>{fmtDateTime(b.created)}</b>
                <span className="bk-quiet">{size(b.size)} · {b.protected ? 'Password protected' : 'Secrets left out'}{b.auto ? ' · Scheduled' : ''}{b.version ? ` · Version ${b.version}` : ''}</span>
                <small className="mono bk-name">{b.name}</small>
              </div>
              <div className="bk-file-actions">
                <Button onClick={() => startAsking(b.name, false)} icon={<IconDownload />}>Download</Button>
                <Button onClick={() => onRestore(b.name)}>Restore</Button>
                <Button variant="ghost" onClick={() => startAsking(b.name, true)} icon={<IconTrash />} aria-label={`Delete ${b.name}`}>Delete</Button>
              </div>
              {asking === b.name && (
                <form className="bk-pw" onSubmit={(e) => { e.preventDefault(); void (deleting ? remove() : download()) }}>
                  <p className="bk-quiet">{deleting ? `${b.name} (${size(b.size)}) is deleted for good. A backup is the way back after something goes wrong, so this asks for your password.` : 'A backup holds your accounts, so taking one off the server asks for your password.'}</p>
                  <Field label="Your password"><Input type="password" autoComplete="current-password" autoFocus value={password} onChange={(e) => setPassword(e.target.value)} /></Field>
                  {view.twoFactor && <Field label="Code from your authenticator app"><Input inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} /></Field>}
                  {err && <Notice tone="warn" title={deleting ? 'It wasn’t deleted' : 'It can’t be downloaded yet'}>{err}</Notice>}
                  <div className="bk-actions">
                    <Button type="submit" variant={deleting ? 'danger' : 'primary'} disabled={password === '' || (view.twoFactor && code === '')}>{deleting ? 'Delete' : 'Download'}</Button>
                    <Button onClick={() => setAsking('')}>Cancel</Button>
                  </div>
                </form>
              )}
            </li>
          ))}
        </ul>
      )}
    </Section>
  )
}

// ---------------------------------------------------------------- the schedule

function ScheduleForm({ view, reload, call }: { view: View; reload: () => void; call: ReturnType<typeof client> }) {
  const [s, setS] = useState(view.schedule)
  const [chosen, setChosen] = useState(() => new Set(view.schedule.parts))
  const [password, setPassword] = useState('')
  const [again, setAgain] = useState('')
  const [changePw, setChangePw] = useState(false)
  const [current, setCurrent] = useState('')
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const on = s.frequency !== 'off'
  const fewer = s.keep < view.schedule.keep // that removes backups, and asks for your password
  const badPw = (changePw || !s.protected) && password !== '' && (password.length < 8 || password !== again)
  const save = async () => {
    setBusy(true); setErr('')
    try {
      await call('PUT', '/backup/schedule', { ...s, parts: [...chosen], password: changePw || !s.protected ? password : '', current, code })
      setPassword(''); setAgain(''); setChangePw(false); setCurrent(''); setCode(''); toast('Schedule saved'); reload()
    } catch (e) { setErr(message(e)) } finally { setBusy(false) }
  }
  const clearPw = async () => {
    try { await call('PUT', '/backup/schedule', { ...s, parts: [...chosen], clearPassword: true }); setS({ ...s, protected: false }); toast('Scheduled backups no longer have a password'); reload() } catch (e) { setErr(message(e)) }
  }
  return (
    <>
      <Section title="Back up by itself" desc="The backup is made in the background at the time you choose. If the server was off, it’s made soon after it next starts.">
        <Row label="How often">
          <Segmented<Schedule['frequency']> label="How often" value={s.frequency} onChange={(v) => setS({ ...s, frequency: v })} options={[{ value: 'off', label: 'Off' }, { value: 'daily', label: 'Every day' }, { value: 'weekly', label: 'Every week' }]} />
        </Row>
        {on && (
          <>
            {s.frequency === 'weekly' && (
              <Row label="On"><Select aria-label="On which day" value={String(s.weekday)} onChange={(e) => setS({ ...s, weekday: Number(e.target.value) })} options={WEEKDAYS.map((d, i) => ({ value: String(i), label: d }))} /></Row>
            )}
            <Row label="At"><Select aria-label="At what time" value={String(s.hour)} onChange={(e) => setS({ ...s, hour: Number(e.target.value) })} options={Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, '0')}:00` }))} /></Row>
            <Row label="Keep the newest" desc="Older scheduled backups are deleted. Ones you make yourself never are.">
              <Input className="bk-keep" type="number" inputMode="numeric" min={1} max={365} aria-label="How many to keep" value={s.keep} onChange={(e) => setS({ ...s, keep: Math.max(1, Math.min(365, Number(e.target.value) || 1)) })} />
            </Row>
            {fewer && (
              <div className="bk-pw">
                <p className="bk-quiet">Keeping fewer deletes backups at the next run, so this asks for your password.</p>
                <Field label="Your password"><Input type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} /></Field>
                {view.twoFactor && <Field label="Code from your authenticator app"><Input inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} /></Field>}
              </div>
            )}
          </>
        )}
        {view.lastAuto ? <p className="bk-quiet">Last made {fmtDateTime(view.lastAuto * 1000)}.</p> : null}
        {view.lastError && <Notice tone="danger" title="The last scheduled backup failed">{view.lastError}</Notice>}
      </Section>
      {on && (
        <>
          <Section title="What to include">
            <PartChecks parts={view.parts} chosen={chosen} setChosen={setChosen} legend="What each scheduled backup includes" />
          </Section>
          <Section title="Password" desc="This server keeps a key made from the password, not the password itself. It opens these backups, as anyone who has this server’s database could anyway.">
            {s.protected && !changePw ? (
              <div className="bk-actions">
                <span className="bk-quiet"><IconLock size={16} /> Scheduled backups are protected.</span>
                <Button onClick={() => setChangePw(true)}>Change the password</Button>
                <Button variant="ghost" onClick={() => void clearPw()}>Remove it</Button>
              </div>
            ) : <Passwords password={password} again={again} setPassword={setPassword} setAgain={setAgain} />}
            {!s.protected && !changePw && password === '' && <p className="bk-quiet">With no password, secrets are left out of these backups.</p>}
          </Section>
        </>
      )}
      {err && <div role="alert"><Notice tone="danger">{err}</Notice></div>}
      <div className="bk-actions bk-save"><Button variant="primary" busy={busy} disabled={busy || badPw || (on && chosen.size === 0)} onClick={save}>Save</Button></div>
    </>
  )
}

// ---------------------------------------------------------------- bringing one back

function ListBlock({ title, items, tone }: { title: string; items: string[]; tone?: 'warn' }) {
  if (!items.length) return null
  return (
    <div className={'bk-block' + (tone ? ' warn' : '')}>
      <h3>{title}</h3>
      <ul className="bk-list">{items.slice(0, 50).map((s) => <li key={s}>{s}</li>)}</ul>
      {items.length > 50 && <p className="bk-quiet">…and {(items.length - 50).toLocaleString()} more.</p>}
    </div>
  )
}

// RestoreFlow: choose a file, check it, see what's in it, choose what to take, agree. It's
// the same in Settings and in first-time setup, where firstRun means no account to ask for.
export function RestoreFlow({ firstRun, setupCode, initial, onBusy }: { firstRun?: boolean; setupCode?: string; initial?: string; onBusy?: (b: boolean) => void }) {
  const call = useMemo(() => client(setupCode), [setupCode])
  const { view, err: loadErr, load, setView } = useJob(call)
  const [file, setFile] = useState(initial || '')
  const [password, setPassword] = useState('')
  const [chosen, setChosen] = useState<Set<string>>(new Set())
  const [accountPw, setAccountPw] = useState('')
  const [code, setCode] = useState('')
  const [typed, setTyped] = useState('')
  const [err, setErr] = useState('')
  const [uploading, setUploading] = useState<{ done: number; total: number } | null>(null)
  const [waiting, setWaiting] = useState('')
  const seen = useRef('')
  const fileInput = useRef<HTMLInputElement>(null)
  const heading = useRef<HTMLHeadingElement>(null)
  const inspection = view?.job.inspection
  useEffect(() => { onBusy?.(!!uploading || !!view?.job.running || !!waiting) }, [uploading, view?.job.running, waiting, onBusy])
  // A new inspection: everything it holds starts ticked.
  useEffect(() => {
    if (!inspection) return
    const key = inspection.name + inspection.created
    if (seen.current !== key) {
      seen.current = key
      setChosen(new Set(inspection.parts.map((p) => p.id)))
      requestAnimationFrame(() => heading.current?.focus())
    }
  }, [inspection])
  useEffect(() => { if (initial) setFile(initial) }, [initial])
  // Stage done: the server restarts, and this waits for it.
  const restarting = !!view?.job.restarting
  useEffect(() => {
    if (!restarting || waiting) return
    setWaiting('The server is restarting to bring your data in. This can take a minute.')
    void waitForRestart(setWaiting)
  }, [restarting, waiting])

  // On a new server the restart can come and go between two looks at the job, and afterwards this
  // page may not ask about it any more, as there are accounts now. So while one runs, it watches
  // the status anyone can read, and starts the app again when the server has been away or is set up.
  const inProgress = !!view?.job.running
  useEffect(() => {
    if (!firstRun || !inProgress) return
    let wentAway = false
    const t = setInterval(() => {
      fetch(serverBase() + '/api/status', { cache: 'no-store' }).then((r) => r.json()).then((st: { setupRequired?: boolean }) => {
        if (!wentAway && st.setupRequired) return
        setToken('')
        window.location.href = '/'
      }).catch(() => { wentAway = true })
    }, 1000)
    return () => clearInterval(t)
  }, [firstRun, inProgress])

  const check = async (name: string, pw: string) => {
    setErr('')
    try {
      await call('POST', '/backup/restore/inspect', { file: name, password: pw })
      await load()
    } catch (e) { setErr(message(e)) }
  }
  const pick = async (f: File) => {
    setErr(''); setUploading({ done: 0, total: f.size })
    try {
      const name = await upload(f, setupCode, (done) => setUploading({ done, total: f.size }))
      setFile(name)
      setUploading(null)
      await check(name, password)
    } catch (e) { setErr(message(e)); setUploading(null) }
  }
  const cancelJob = () => { call('POST', '/backup/cancel').then(load).catch((e) => toast(message(e))) }
  const start = async () => {
    setErr('')
    try {
      await call('POST', '/backup/restore/start', { parts: [...chosen], confirm: typed, password: accountPw, code })
      setAccountPw(''); setCode('')
      await load()
    } catch (e) { setErr(message(e)) }
  }
  const undoPending = async () => {
    try { await call('DELETE', '/backup/restore/request'); toast('The restore was cancelled'); setView(null); await load() } catch (e) { toast(message(e)) }
  }

  if (!view) return loadErr ? <div role="alert"><Notice tone="danger">{loadErr}</Notice></div> : <Spinner />
  if (waiting) return <div className="bk-progress" role="status"><p className="bk-phase">{waiting}</p><p className="bk-quiet">This page moves on by itself. Everyone signs in again afterwards.</p><Progress label="Waiting for the server" /></div>

  const job = view.job
  const running = job.running && job.kind !== 'create'
  const stage = job.running && job.kind === 'restore'
  const ready = chosen.size > 0 && typed === 'RESTORE' && !job.running && (firstRun || (accountPw !== '' && (!view.twoFactor || code !== '')))
  const needs = inspection ? [...chosen].flatMap((id) => (inspection.parts.find((p) => p.id === id)?.needs || []).filter((n) => !chosen.has(n))) : []
  return (
    <>
      {view.pending && (
        <Notice tone="warn" title="A restore is waiting" action={<Button onClick={undoPending}>Cancel it</Button>}>
          It happens the next time this server starts. {view.canRestart ? '' : 'This server can’t restart itself: stop it and start it again.'}
        </Notice>
      )}
      <div className="bk-choose">
        <input ref={fileInput} className="sr-only" type="file" accept=".zip,application/zip" aria-label="Choose a backup file" onChange={(e) => { const f = e.target.files?.[0]; if (f) void pick(f); e.target.value = '' }} disabled={!!uploading || job.running} />
        <Button variant={view.files.length ? undefined : 'primary'} onClick={() => fileInput.current?.click()} disabled={!!uploading || job.running}>Choose a backup file</Button>
        {view.files.length > 0 && (
          <Select aria-label="Or one already on this server" value={file} disabled={!!uploading || job.running} onChange={(e) => { setFile(e.target.value); if (e.target.value) void check(e.target.value, password) }}
            options={[{ value: '', label: 'Or one already on this server…' }, ...view.files.map((b) => ({ value: b.name, label: `${fmtDateTime(b.created)}, ${size(b.size)}${b.protected ? ', protected' : ''}` }))]} />
        )}
      </div>
      {uploading && <div className="bk-progress" role="status" aria-live="polite"><p className="bk-phase">Uploading {size(uploading.done)} of {size(uploading.total)}</p><Progress value={(uploading.done / Math.max(1, uploading.total)) * 100} label="Uploading" /></div>}
      {running && <JobProgress job={job} onCancel={cancelJob} />}
      {!running && job.kind === 'inspect' && job.needsPassword && (
        <form className="bk-pw" onSubmit={(e) => { e.preventDefault(); void check(job.file || file, password) }}>
          <Field label={password ? 'That password doesn’t open this backup. Try again' : 'This backup is protected. Enter its password'}>
            <Input type="password" autoComplete="off" autoFocus value={password} onChange={(e) => setPassword(e.target.value)} />
          </Field>
          <Button type="submit" variant="primary" disabled={!password}>Open it</Button>
        </form>
      )}
      {!running && !job.needsPassword && job.error && job.kind !== 'create' && (
        <div role="alert"><Notice tone="danger" title={job.refused ? 'This backup was refused, and nothing was changed' : 'That didn’t work, and nothing was changed'}>{job.error}</Notice></div>
      )}
      {err && <div role="alert"><Notice tone="danger">{err}</Notice></div>}
      {inspection && !stage && !job.needsPassword && (
        <div className="bk-preview">
          <h2 className="bk-h" ref={heading} tabIndex={-1}>What’s in it</h2>
          <p>A backup from <b>{fmtDateTime(inspection.created)}</b>, made by version {inspection.serverVersion}, {inspection.protected ? 'protected with a password' : 'with no password'}. Every file in it checked out.{inspection.older ? ' It’s from an older version, and is brought up to date as it’s restored.' : ''}</p>
          <PartChecks parts={inspection.parts} chosen={chosen} setChosen={setChosen} legend="What to bring back" inside />
          {needs.length > 0 && <Notice tone="warn">Some of what you’ve chosen needs parts you’ve left out.</Notice>}
          <p className="bk-quiet">Anything you untick stays as it is on this server now, unless it depends on something you’re replacing: watch history, say, can’t stay if the library is replaced, as its numbers would point at other titles.</p>
          {inspection.accounts.length > 0 && chosen.has('users') && (
            <div className="bk-block">
              <h3>Who can sign in afterwards</h3>
              <ul className="bk-list">{inspection.accounts.map((a) => <li key={a.name}><b>{a.name}</b>{a.admin ? ' (administrator)' : ''}</li>)}</ul>
              <p className="bk-quiet">These replace the accounts here, with the passwords they had. If you don’t recognise an administrator here, stop: whoever made this backup could sign in.</p>
            </div>
          )}
          {inspection.libraries.length > 0 && chosen.has('library') && (
            <div className="bk-block">
              <h3>Folders the server will use</h3>
              <ul className="bk-list">{inspection.libraries.map((l) => <li key={l.name}><b>{l.name}</b> ({l.items.toLocaleString()} titles): <span className="mono">{l.folders.join(', ') || 'no folders'}</span></li>)}</ul>
            </div>
          )}
          <ListBlock title="Addresses it would contact" items={inspection.addresses} />
          <ListBlock title="Left out, on purpose" items={inspection.dropped} tone="warn" />
          <ListBlock title="Metadata files changed by hand" items={inspection.handEdited} />
          <ListBlock title="Problems in those files" items={inspection.problems} tone="warn" />
          <ListBlock title="What happens to sign-ins, keys and modules" items={inspection.notes} />
          <div className="bk-block">
            <h3>Bring it back</h3>
            <p>What’s on this server now is moved to a safety copy first, and goes back by itself if anything fails. Anything playing is interrupted.{view.canRestart ? ' The server restarts.' : ' This server can’t restart itself: stop it and start it again, and it happens then.'}</p>
            <div className="bk-fields">
              {!firstRun && <Field label="Your password"><Input type="password" autoComplete="current-password" value={accountPw} onChange={(e) => setAccountPw(e.target.value)} /></Field>}
              {!firstRun && view.twoFactor && <Field label="Code from your authenticator app"><Input inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} /></Field>}
              <Field label="Type RESTORE to confirm"><Input autoComplete="off" spellCheck={false} value={typed} onChange={(e) => setTyped(e.target.value)} /></Field>
            </div>
            <div className="bk-actions"><Button variant="danger" disabled={!ready || needs.length > 0} onClick={start}>Restore this backup</Button></div>
          </div>
        </div>
      )}
    </>
  )
}

function LastRestore({ view, reload, call }: { view: View; reload: () => void; call: ReturnType<typeof client> }) {
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [undoing, setUndoing] = useState(false)
  const [err, setErr] = useState('')
  const [waiting, setWaiting] = useState('')
  const last = view.last
  const copy = view.safety[0]
  if (!last && !copy) return null
  const undo = async () => {
    setErr('')
    try {
      const r = await call<{ restarting: boolean }>('POST', '/backup/undo', { password, code, copy: copy.name })
      if (r.restarting) { setWaiting('The server is restarting to put things back.'); void waitForRestart(setWaiting) } else { setWaiting('Stop this server and start it again, and it happens then.'); reload() }
    } catch (e) { setErr(message(e)) }
  }
  return (
    <Section title="The last restore">
      {last && !last.ok && <div role="alert"><Notice tone="danger" title="The last restore didn’t work">{last.error}</Notice></div>}
      {last?.ok && <Notice tone="ok" title={last.mode === 'undo' ? 'Put back as it was' : last.mode === 'recovery' ? 'Put back after an interrupted restore' : 'Restored'}>{fmtDateTime(last.at)}{last.name ? ` from ${last.name}` : ''}. {(last.notes || []).join(' ')}</Notice>}
      {waiting && <p role="status" className="bk-phase">{waiting}</p>}
      {copy && !waiting && (
        <>
          <p className="bk-quiet">A safety copy from {fmtDateTime(copy.at)} holds what the last restore replaced.</p>
          {undoing ? (
            <form className="bk-pw" onSubmit={(e) => { e.preventDefault(); void undo() }}>
              <Field label="Your password"><Input type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} /></Field>
              {view.twoFactor && <Field label="Code from your authenticator app"><Input inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} /></Field>}
              {err && <div role="alert"><Notice tone="danger">{err}</Notice></div>}
              <div className="bk-actions"><Button variant="ghost" onClick={() => setUndoing(false)}>Keep things as they are</Button><Button type="submit" variant="danger" disabled={!password}>Put it back</Button></div>
            </form>
          ) : <div className="bk-actions"><Button onClick={() => setUndoing(true)}>Undo the last restore</Button></div>}
        </>
      )}
    </Section>
  )
}

export default function BackupSettings() {
  const call = useMemo(() => client(), [])
  const { view, err, load } = useJob(call)
  const [tab, setTab] = useState<'make' | 'restore' | 'schedule'>('make')
  const [initial, setInitial] = useState('')
  useEffect(() => { if (view?.job.running && view.job.kind !== 'create') setTab('restore') }, [view?.job.running, view?.job.kind])
  if (!view) return err ? <div role="alert"><Notice tone="danger">{err}</Notice></div> : <Spinner />
  return (
    <>
      <PageHead title="Backup" desc="A ZIP of your data and metadata, for when something goes wrong or you move to another machine. Bring one back here, or from first-time setup after a reset." />
      <Tabs<'make' | 'restore' | 'schedule'> value={tab} onChange={setTab} label="Backup" panelId="bk-panel" items={[{ value: 'make', label: 'Back up' }, { value: 'restore', label: 'Restore' }, { value: 'schedule', label: 'Schedule' }]} />
      <div id="bk-panel" role="tabpanel">
        {tab === 'make' && (
          <>
            <MakeBackup view={view} reload={() => void load()} call={call} />
            <FileList view={view} reload={() => void load()} call={call} onRestore={(n) => { setInitial(n); setTab('restore') }} />
          </>
        )}
        {tab === 'restore' && (
          <>
            <Section title="Restore a backup" desc={<>Bring back a backup from this server or from a file. Every file is checked before anything changes, and archives that don’t pass are refused. To start again first, see <Link to="/settings/reset">Reset</Link>.</>}>
              <RestoreFlow initial={initial} />
            </Section>
            <LastRestore view={view} reload={() => void load()} call={call} />
          </>
        )}
        {tab === 'schedule' && <ScheduleForm view={view} reload={() => void load()} call={call} />}
      </div>
    </>
  )
}
