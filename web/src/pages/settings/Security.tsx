import { useCallback, useEffect, useState } from 'react'
import { fmtDateTime, api, type Session, type UserView } from '../../api'
import { IconCheck, IconTrash } from '../../icons'
import { toast, useApp } from '../../state'
import QrCode from './QrCode'
import { PageHead, Row, Section } from './ui'
import './remote.css'

// Settings → Sign-in & security, for everyone: the devices signed in to this
// account, and two-factor sign-in for adults.

const where: Record<string, string> = { machine: 'This computer', home: 'At home', vpn: 'Away, through your VPN', internet: 'Away, over the internet' }

const when = (t: number) => (t ? fmtDateTime(t * 1000) : 'not yet')

export default function Security() {
  const { user } = useApp()
  return (
    <>
      <PageHead title="Sign-in & security" desc="Where you are signed in, and how your account is protected." />
      <Devices />
      {user?.kind === 'adult' && <TwoFactor />}
      {user?.isAdmin && <ResetOthers />}
    </>
  )
}

function Devices() {
  const [list, setList] = useState<Session[] | null>(null)
  const load = useCallback(() => api.get<Session[]>('/sessions').then(setList).catch((e) => toast(e.message)), [])
  useEffect(() => { load() }, [load])
  const end = async (s: Session) => {
    try { await api.del(`/sessions/${s.id}`); toast('Signed out on that device'); load() } catch (e: any) { toast(e.message) }
  }
  const endOthers = async () => {
    if (!confirm('Sign out everywhere except on this device?')) return
    try { await api.del('/sessions'); toast('Signed out on your other devices'); load() } catch (e: any) { toast(e.message) }
  }
  const others = (list || []).filter((s) => !s.current).length
  return (
    <Section title="Your devices" desc="Anything you don't recognise? Sign it out, then change your password."
      actions={others > 0 ? <button type="button" className="btn ghost sm" onClick={endOthers}>Sign out the others</button> : undefined}>
      <ul className="rem-sessions">
        {(list || []).map((s) => (
          <li key={s.id}>
            <div>
              <b>{s.device || 'Unknown device'}</b>
              {s.current && <span className="set-badge">This device</span>}
              {s.streaming && <span className="set-pill live">Streaming</span>}
              <small>{where[s.where] || 'Not used since signing in'}{s.ip ? ` · ${s.ip}` : ''} · last used {when(s.lastSeen)}</small>
            </div>
            {!s.current && <button type="button" className="btn sm ghost danger-hover" onClick={() => end(s)} aria-label={`Sign out on ${s.device || 'this device'}`}><IconTrash /> Sign out</button>}
          </li>
        ))}
      </ul>
    </Section>
  )
}

type Setup = { secret: string; uri: string }

function TwoFactor() {
  const { user, setUser } = useApp()
  const [state, setState] = useState<{ on: boolean; recoveryLeft: number } | null>(null)
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [setup, setSetup] = useState<Setup | null>(null)
  const [recovery, setRecovery] = useState<string[] | null>(null)
  const [turningOff, setTurningOff] = useState(false)
  const [err, setErr] = useState('')
  const load = useCallback(() => api.get<{ on: boolean; recoveryLeft: number }>('/me/2fa').then(setState).catch(() => {}), [])
  useEffect(() => { load() }, [load])
  const run = async (f: () => Promise<void>) => { setErr(''); try { await f() } catch (e: any) { setErr(e.message) } }
  const start = () => run(async () => { setSetup(await api.post<Setup>('/me/2fa/setup', { password })); setPassword('') })
  const confirmCode = () => run(async () => {
    const r = await api.post<{ recovery: string[] }>('/me/2fa/confirm', { code })
    setRecovery(r.recovery); setSetup(null); setCode(''); await load()
    if (user) setUser({ ...user, twoFactor: true })
  })
  const disable = () => run(async () => {
    await api.post('/me/2fa/disable', { password, code })
    setTurningOff(false); setPassword(''); setCode(''); await load(); toast('Two-factor sign-in is off')
    if (user) setUser({ ...user, twoFactor: false })
  })
  if (!state) return null
  return (
    <Section title="Two-factor sign-in" desc="Signing in also asks for a 6-digit code from an app on your phone, so a stolen password isn't enough. It is optional, and worth having if you use Remote Access. Any authenticator app works.">
      {recovery && (
        <div className="rem-recovery" role="alert">
          <b>Save these recovery codes now.</b>
          <p>Each works once, in place of a code, if you lose your phone. They won't be shown again.</p>
          <ul>{recovery.map((c) => <li key={c}><code>{c}</code></li>)}</ul>
          <div className="rem-actions">
            <button type="button" className="btn sm" onClick={() => navigator.clipboard.writeText(recovery.join('\n')).then(() => toast('Copied')).catch(() => toast('Select them and copy by hand'))}>Copy them</button>
            <button type="button" className="btn primary sm" onClick={() => setRecovery(null)}>I saved them</button>
          </div>
        </div>
      )}
      {state.on && !turningOff && (
        <Row label={<><IconCheck size={14} /> On for your account</>} desc={`${state.recoveryLeft} recovery ${state.recoveryLeft === 1 ? 'code' : 'codes'} left.`}>
          <button type="button" className="btn sm ghost" onClick={() => { setErr(''); setTurningOff(true) }}>Turn off</button>
        </Row>
      )}
      {state.on && turningOff && (
        <form className="set-form rem-form" onSubmit={(e) => { e.preventDefault(); disable() }}>
          <label>Your password<input className="input" type="password" autoComplete="current-password" autoFocus value={password} onChange={(e) => setPassword(e.target.value)} /></label>
          <label>A code from your app, or a recovery code<input className="input" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} /></label>
          <div className="rem-actions">
            <button type="button" className="btn ghost sm" onClick={() => setTurningOff(false)}>Keep it on</button>
            <button className="btn danger sm" disabled={!password || !code}>Turn off</button>
          </div>
        </form>
      )}
      {!state.on && !setup && !recovery && (
        <form className="set-form rem-form" onSubmit={(e) => { e.preventDefault(); start() }}>
          <label>Your password, to begin<input className="input" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} /></label>
          <div className="rem-actions"><button className="btn primary sm" disabled={!password}>Set up</button></div>
        </form>
      )}
      {setup && (
        <form className="rem-test" onSubmit={(e) => { e.preventDefault(); confirmCode() }}>
          <QrCode text={setup.uri} label="QR code for your authenticator app" size={190} />
          <div className="set-form rem-form">
            <ol>
              <li>In your authenticator app, add an account and scan this code.</li>
              <li>Can't scan? Choose to enter a key, and type: <code className="rem-key">{setup.secret.replace(/(.{4})/g, '$1 ').trim()}</code></li>
              <li>Enter the 6 digits it shows:</li>
            </ol>
            <label><span className="sr-only">Code from your app</span><input className="input" inputMode="numeric" autoComplete="one-time-code" autoFocus maxLength={7} value={code} onChange={(e) => setCode(e.target.value)} /></label>
            <div className="rem-actions">
              <button type="button" className="btn ghost sm" onClick={() => { setSetup(null); setCode('') }}>Cancel</button>
              <button className="btn primary sm" disabled={code.replace(/\s/g, '').length !== 6}>Turn on</button>
            </div>
            <small>Other devices signed in to your account are signed out when this turns on.</small>
          </div>
        </form>
      )}
      {err && <p className="error" role="alert">{err}</p>}
    </Section>
  )
}

// For someone who lost their phone and their recovery codes.
function ResetOthers() {
  const { user } = useApp()
  const [users, setUsers] = useState<UserView[]>([])
  const load = useCallback(() => api.get<UserView[]>('/users').then((l) => setUsers(l.filter((u) => u.twoFactor && !u.isAdmin && u.id !== user?.id))).catch(() => {}), [user])
  useEffect(() => { load() }, [load])
  if (users.length === 0) return null
  const reset = async (u: UserView) => {
    if (!confirm(`Turn off two-factor sign-in for ${u.name}? Do this only if you know it's really them who lost their phone.`)) return
    try { await api.del(`/users/${u.id}/2fa`); toast(`Turned off for ${u.name}`); load() } catch (e: any) { toast(e.message) }
  }
  return (
    <Section title="Other people's two-factor sign-in" desc="For someone who lost their phone and their recovery codes. An administrator's own can only be reset on the server itself, with: wms reset-2fa">
      {users.map((u) => (
        <Row key={u.id} label={u.name}><button type="button" className="btn sm ghost" onClick={() => reset(u)}>Turn off</button></Row>
      ))}
    </Section>
  )
}
