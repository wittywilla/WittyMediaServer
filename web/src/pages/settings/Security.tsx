// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useMemo, useState } from 'react'
import { fmtDateTime, api, type Session, type UserView } from '../../api'
import { toast, useApp } from '../../state'
import SpecPage from '../../spec/SpecPage'
import { handledIds } from '../../spec/model'
import { devicesSpec, twoFactorSpec, type TwoFactorState } from '../../spec/specs/security'
import { PageHead, Row, Section } from './ui'
import './remote.css'
import { ask } from '../../ui/Overlay'

// Settings → Sign-in & security, for everyone: the devices signed in to this
// account, and two-factor sign-in for adults.

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
  const page = useMemo(() => devicesSpec(list || [], when), [list])
  const handled = useMemo(() => handledIds(page), [page])
  const act = async (id: string) => {
    if (id === 'others') {
      if (!await ask({ title: 'Sign out everywhere except on this device?', confirm: 'Sign out the others', danger: true })) return
      try { await api.del('/sessions'); toast('Signed out on your other devices'); load() } catch (e: any) { toast(e.message) }
    } else if (id.startsWith('out:')) {
      try { await api.del(`/sessions/${id.slice(4)}`); toast('Signed out on that device'); load() } catch (e: any) { toast(e.message) }
    }
  }
  return (
    <Section title="Your devices" desc="Anything you don't recognise? Sign it out, then change your password.">
      <SpecPage embedded page={page} handled={handled} onAction={act} />
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
  const st: TwoFactorState | null = !state ? null
    : recovery ? { mode: 'recovery', codes: recovery }
    : setup ? { mode: 'setup', code, uri: setup.uri, secret: setup.secret }
    : state.on ? (turningOff ? { mode: 'turningOff', password, code } : { mode: 'on', recoveryLeft: state.recoveryLeft })
    : { mode: 'begin', password }
  const page = useMemo(() => (st ? twoFactorSpec(st, err) : null), [st && JSON.stringify(st), err]) // eslint-disable-line react-hooks/exhaustive-deps
  const handled = useMemo(() => (page ? handledIds(page) : undefined), [page])
  if (!state || !st || !page) return null
  const primary: Record<string, string> = { turningOff: 'turnOff', begin: 'start', setup: 'confirm', recovery: 'saved' }
  const act = (id: string) => {
    if (id === 'off') { setErr(''); setTurningOff(true) }
    else if (id === 'keepOn') setTurningOff(false)
    else if (id === 'turnOff') void disable()
    else if (id === 'start') void start()
    else if (id === 'confirm') void confirmCode()
    else if (id === 'cancel') { setSetup(null); setCode('') }
    else if (id === 'saved') setRecovery(null)
    else if (id === 'copy' && recovery) navigator.clipboard.writeText(recovery.join('\n')).then(() => toast('Copied')).catch(() => toast('Select them and copy by hand'))
  }
  const field = (id: string, v: string | number | boolean | File) => { if (id === 'password') setPassword(String(v)); else if (id === 'code') setCode(String(v)) }
  return (
    <Section title="Two-factor sign-in" desc="Signing in also asks for a 6-digit code from an app on your phone, so a stolen password isn't enough. It is optional, and worth having if you use Remote Access. Any authenticator app works.">
      <form onSubmit={(e) => { e.preventDefault(); const id = primary[st.mode]; if (id && !page.actions.find((a) => a.id === id)?.disabled) act(id) }}>
        <SpecPage embedded page={page} handled={handled} onAction={act} onField={field} />
        <button type="submit" hidden tabIndex={-1} aria-hidden="true" />
      </form>
      {err && st.mode !== 'setup' && <p className="error" role="alert">{err}</p>}
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
    if (!await ask({ title: `Turn off two-factor sign-in for ${u.name}?`, body: 'Do this only if you know it’s really them who lost their phone.', confirm: 'Turn it off', danger: true })) return
    try { await api.del(`/users/${u.id}/2fa`); toast(`Turned off for ${u.name}`); load() } catch (e: any) { toast(e.message) }
  }
  return (
    <Section title="Other people's two-factor sign-in" desc="For someone who lost their phone and their recovery codes. An administrator's own can only be reset on the server itself, with: wittymediaserver reset-2fa">
      {users.map((u) => (
        <Row key={u.id} label={u.name}><button type="button" className="btn sm ghost" onClick={() => reset(u)}>Turn off</button></Row>
      ))}
    </Section>
  )
}
