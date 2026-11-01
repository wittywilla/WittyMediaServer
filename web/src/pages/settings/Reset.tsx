import { useEffect, useState } from 'react'
import { fmtDateTime, api, setToken } from '../../api'
import { Spinner } from '../../components/common'
import { toast } from '../../state'
import { PageHead, Section } from './ui'

// Reset: back to a new install, or back to a copy an earlier reset kept. The server
// does it as it starts up, so this page waits for it to come back.

type Copy = { name: string; at: number; bytes: number }
type View = {
  dataDir: string; outside: string[]; blocked?: string; copies: Copy[]; canRestart: boolean; twoFactor: boolean
  pending?: { mode: string }
  last?: { ok: boolean; mode: string; copy?: string; error?: string; notes?: string[]; at: string }
}

const size = (n: number) => (n >= 1 << 30 ? `${(n / (1 << 30)).toFixed(1)} GB` : `${Math.max(1, Math.round(n / (1 << 20)))} MB`)

// Waits for the server to go away and come back, then starts the page again.
async function waitForRestart(setStatus: (s: string) => void) {
  const answers = async () => {
    try {
      const r = await fetch('/api/status', { cache: 'no-store' })
      return r.ok
    } catch { return false }
  }
  const started = Date.now()
  let wentAway = false
  while (Date.now() - started < 120000) {
    await new Promise((ok) => setTimeout(ok, 1000))
    const up = await answers()
    if (!up) { wentAway = true; continue }
    if (wentAway || Date.now() - started > 8000) {
      setToken('')
      window.location.href = '/'
      return
    }
  }
  setStatus('The server hasn’t come back. Start it again by hand: the reset happens as it starts.')
}

function Secrets({ twoFactor, password, code, setPassword, setCode }: { twoFactor: boolean; password: string; code: string; setPassword: (s: string) => void; setCode: (s: string) => void }) {
  return (
    <>
      <label className="oobe-field">Your password
        <input className="input" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
      </label>
      {twoFactor && (
        <label className="oobe-field">Code from your authenticator app
          <input className="input" inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} />
        </label>
      )}
    </>
  )
}

export default function ResetSettings() {
  const [view, setView] = useState<View | null>(null)
  const [mode, setMode] = useState<'keep' | 'delete'>('keep')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [typed, setTyped] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [waiting, setWaiting] = useState('')
  const [picked, setPicked] = useState<{ copy: string; action: 'restore' | 'delete' } | null>(null)
  const load = () => api.get<View>('/reset').then(setView).catch((e) => setErr(e.message))
  useEffect(() => { void load() }, [])
  if (!view) return err ? <div className="error" role="alert">{err}</div> : <Spinner />

  const restarted = (r: { restarting: boolean }, what: string) => {
    if (!r.restarting) {
      setWaiting(`${what} This server can’t restart itself: stop it and start it again, and it happens then.`)
      return
    }
    setWaiting(`${what} The server is restarting, which can take a minute.`)
    void waitForRestart(setWaiting)
  }
  const go = async (e: React.FormEvent) => {
    e.preventDefault(); setErr(''); setBusy(true)
    try {
      const r = await api.post<{ restarting: boolean }>('/reset', { password, code, confirm: typed, delete: mode === 'delete' })
      restarted(r, mode === 'delete' ? 'Deleting everything.' : 'Resetting.')
    } catch (x: any) { setErr(x.message); setBusy(false) }
  }
  const onCopy = async (e: React.FormEvent) => {
    e.preventDefault(); if (!picked) return
    setErr(''); setBusy(true)
    try {
      if (picked.action === 'restore') {
        restarted(await api.post<{ restarting: boolean }>('/reset/restore', { password, code, copy: picked.copy }), 'Bringing that copy back.')
        return
      }
      await api.post('/reset/copies/delete', { password, code, copy: picked.copy })
      toast('Copy deleted'); setPicked(null); setPassword(''); setCode(''); setBusy(false); void load()
    } catch (x: any) { setErr(x.message); setBusy(false) }
  }
  const cancel = async () => {
    try { await api.del('/reset/request'); toast('The reset was cancelled'); void load() } catch (x: any) { setErr(x.message) }
  }

  if (waiting) {
    return (
      <>
        <PageHead title="Reset this server" />
        <Section title="Please wait"><div role="status" className="set-prose"><p>{waiting}</p><p>This page moves on by itself when it’s done.</p></div></Section>
      </>
    )
  }
  const deleting = mode === 'delete'
  const ready = password !== '' && typed === 'RESET' && !busy && (!view.twoFactor || code !== '')
  return (
    <>
      <PageHead title="Reset this server" desc="Puts this server back the way it was first installed, so you can set it up from the start." />
      {view.last && !view.last.ok && <div className="error" role="alert">The last reset didn’t happen, and nothing was changed: {view.last.error}</div>}
      {view.last?.ok && (view.last.notes || []).map((n) => <p key={n} className="set-note" role="status">{n}</p>)}
      {view.pending && (
        <div className="set-note bad" role="status">
          A reset is waiting: it happens the next time this server starts. <button className="btn sm" onClick={cancel}>Cancel it</button>
        </div>
      )}
      <Section title="What a reset does" desc="Read this first. It applies to everyone who uses this server, not only you.">
        <div className="set-prose">
          <p><b>These go away:</b></p>
          <ul>
            <li>Every account and profile, and every device’s sign-in. Apps have to connect again.</li>
            <li>The list of libraries, and everything known about what’s in them: watch progress, favourites, ratings, playlists, collections and notes.</li>
            <li>All settings and API keys, Remote Access, TV tuners, channels and scheduled recordings.</li>
            <li><b>Game saves and saved states</b>, controller profiles, game cores and custom cores.</li>
            <li>Installed module versions, logs, reports, and the cache of posters and converted video.</li>
            <li><b>Recordings</b>, if they’re kept in the data folder, which is where they go unless you chose another.</li>
          </ul>
          <p><b>These stay, whichever you choose:</b> your media files and game files, wherever they are, anything saved beside them such as posters and info files, the program itself and its config file.</p>
          <p>Data folder: <code>{view.dataDir}</code></p>
          {view.outside.length > 0 && <p>These folders are set up elsewhere. What’s in them is moved into a <code>before-reset</code> folder inside each, and never deleted, so that’s yours to clear: {view.outside.map((o) => <code key={o}>{o} </code>)}</p>}
          {!view.canRestart && <p>This server can’t restart itself. Once you’ve asked, stop it and start it again, and the reset happens then.</p>}
          <p>Anyone watching or playing now is interrupted. To set the server up again from another device, the setup code is shown on the server’s console.</p>
        </div>
      </Section>
      {view.blocked ? (
        <Section title="This server can’t be reset as it’s set up"><div className="error" role="alert">{view.blocked}</div></Section>
      ) : (
        <Section title="Reset">
          <form className="form full" onSubmit={go}>
            <fieldset className="set-fieldset">
              <legend>What happens to what’s there now</legend>
              <label className="set-radio"><input type="radio" name="reset-mode" checked={!deleting} onChange={() => setMode('keep')} /> <span><b>Keep a copy</b> (recommended). It’s all moved into a folder inside the data folder, and you can bring it back from this page. It still takes up disk space.</span></label>
              <label className="set-radio"><input type="radio" name="reset-mode" checked={deleting} onChange={() => setMode('delete')} /> <span><b>Delete everything for good</b>, including copies from earlier resets. This can’t be undone.</span></label>
            </fieldset>
            {deleting && <div className="set-note bad" role="alert">Nothing is kept, and this can’t be undone: game saves, watch progress and any recordings in the data folder are gone for good.</div>}
            <Secrets twoFactor={view.twoFactor} password={password} code={code} setPassword={setPassword} setCode={setCode} />
            <label className="oobe-field">Type RESET to confirm
              <input className="input" autoComplete="off" spellCheck={false} value={typed} onChange={(e) => setTyped(e.target.value)} />
            </label>
            {err && !picked && <div className="error" role="alert">{err}</div>}
            <div className="actions"><button className="btn danger" disabled={!ready}>{deleting ? 'Delete everything and reset' : 'Reset this server'}</button></div>
          </form>
        </Section>
      )}
      {view.copies.length > 0 && (
        <Section title="Copies from earlier resets" desc="Bringing one back replaces what’s on this server now. What’s there now is kept as another copy.">
          <ul className="set-list">
            {view.copies.map((c) => (
              <li key={c.name}>
                <span><b>{fmtDateTime(c.at * 1000)}</b> <span className="muted">{size(c.bytes)}</span></span>{' '}
                <button className="btn sm" onClick={() => { setErr(''); setPassword(''); setCode(''); setPicked({ copy: c.name, action: 'restore' }) }}>Bring back</button>{' '}
                <button className="btn sm ghost" onClick={() => { setErr(''); setPassword(''); setCode(''); setPicked({ copy: c.name, action: 'delete' }) }}>Delete</button>
              </li>
            ))}
          </ul>
          {picked && (
            <form className="form full" onSubmit={onCopy}>
              <p>{picked.action === 'restore' ? 'Bring this copy back, and restart the server?' : 'Delete this copy for good? This can’t be undone.'}</p>
              <Secrets twoFactor={view.twoFactor} password={password} code={code} setPassword={setPassword} setCode={setCode} />
              {err && <div className="error" role="alert">{err}</div>}
              <div className="actions">
                <button type="button" className="btn ghost" onClick={() => { setPicked(null); setErr('') }}>Cancel</button>
                <button className={'btn ' + (picked.action === 'delete' ? 'danger' : 'primary')} disabled={busy || !password || (view.twoFactor && !code)}>{picked.action === 'restore' ? 'Bring it back' : 'Delete it'}</button>
              </div>
            </form>
          )}
        </Section>
      )}
    </>
  )
}
