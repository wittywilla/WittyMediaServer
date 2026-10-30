import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, type User } from '../api'
import { useApp } from '../state'

type Asking = { code: string; device: string; platform: string; ip: string; profiles: { id: number; name: string; kind: string }[] }

// Signing a TV in: the TV shows a short code, and it's typed (or arrives in
// the address, from a QR code) here. Nothing happens until the person has seen
// which device is asking and said yes, so a code from someone else's screen
// can't sign that someone in by accident.
export default function PairPage() {
  const { user } = useApp()
  const [params] = useSearchParams()
  const [code, setCode] = useState(params.get('code') || '')
  const [asking, setAsking] = useState<Asking | null>(null)
  const [who, setWho] = useState<number>(user?.id || 0)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState<{ device: string; user: User } | 'denied' | null>(null)

  const look = async (c: string) => {
    setErr(''); setBusy(true)
    try { const a = await api.post<Asking>('/pair/info', { code: c }); setAsking(a); setWho(user?.id || 0) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  // A code in the address only fills in the form's first step: the question
  // below is still asked.
  useEffect(() => { const c = params.get('code'); if (c) look(c) }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const approve = async () => {
    setErr(''); setBusy(true)
    try { setDone(await api.post<{ device: string; user: User }>('/pair/approve', { code: asking!.code, userId: who })) } catch (e: any) { setErr(e.message); setAsking(null) } finally { setBusy(false) }
  }
  const deny = async () => {
    setErr(''); setBusy(true)
    try { await api.post('/pair/deny', { code: asking!.code }); setDone('denied') } catch (e: any) { setErr(e.message); setAsking(null) } finally { setBusy(false) }
  }

  if (user && user.kind !== 'adult' && !user.isAdmin) {
    return <div className="toolbar"><h1>Sign in a TV</h1><p className="muted">An adult profile has to do this.</p></div>
  }
  if (done) {
    return (
      <>
        <div className="toolbar"><h1>Sign in a TV</h1></div>
        <div role="status">
          {done === 'denied'
            ? <p>The sign-in was refused. The TV has been told.</p>
            : <p><b>{done.device}</b> is now signed in as <b>{done.user.name}</b>. It will continue by itself in a moment.</p>}
          <p><Link className="btn" to="/" autoFocus>Back to home</Link></p>
        </div>
      </>
    )
  }
  return (
    <>
      <div className="toolbar"><h1>Sign in a TV</h1></div>
      {!asking ? (
        <form className="form" onSubmit={(e) => { e.preventDefault(); if (code.trim()) look(code) }}>
          <p className="muted">Open the app on your TV and choose to sign in. Type the code it shows here.</p>
          <label>Code from the TV
            <input className="input" value={code} onChange={(e) => setCode(e.target.value)} placeholder="ABCD-EFGH" autoFocus
              autoComplete="off" autoCapitalize="characters" spellCheck={false} maxLength={12} />
          </label>
          {err && <div className="error" role="alert">{err}</div>}
          <div className="actions"><button className="btn primary" disabled={busy || !code.trim()}>Continue</button></div>
        </form>
      ) : (
        <div className="form">
          <p><b>{asking.device}</b>{asking.platform ? ` (${asking.platform})` : ''} at {asking.ip} wants to sign in to this server.</p>
          <p className="muted">Only continue if you are looking at that code on your own TV right now.</p>
          {asking.profiles.length > 1 && (
            <label>Sign it in as
              <select className="select" value={who} onChange={(e) => setWho(Number(e.target.value))}>
                {asking.profiles.map((p) => <option key={p.id} value={p.id}>{p.name}{p.kind !== 'adult' ? ` (${p.kind === 'kids' ? 'Kids' : 'Guest'})` : ''}</option>)}
              </select>
            </label>
          )}
          {err && <div className="error" role="alert">{err}</div>}
          <div className="actions">
            <button className="btn primary" autoFocus disabled={busy} onClick={approve}>Sign it in</button>
            <button className="btn ghost" disabled={busy} onClick={deny}>Refuse</button>
          </div>
        </div>
      )}
    </>
  )
}
