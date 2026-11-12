// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { HelpLink } from '../components/HelpLink'
import { Link, useSearchParams } from 'react-router-dom'
import { api, ApiError, type User, type UserView } from '../api'
import { clock, tidyCode } from '../components/pairing'
import { useApp } from '../state'
import './pair.css'

type Invite = { code: string; expiresInSec: number; user: User }
type Asking = { code: string; device: string; platform: string; ip: string; profiles: { id: number; name: string; kind: string; needsPassword?: boolean }[]; needsPin?: boolean; twoFactor?: boolean }

// The way on, given the focus when it appears: a link takes no autoFocus, and a remote or a
// keyboard should land on it. Defined once, so drawing the page again doesn't take the focus back.
const focusOnce = (el: HTMLElement | null) => { el?.focus() }

// A device that started with secure pairing shows a 6-digit PIN that changes every 30 seconds.
const PIN_LENGTH = 6

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
  const [pin, setPin] = useState('')
  const [pinErr, setPinErr] = useState('')
  // Signing a device in as an administrator asks for the administrator's password again:
  // a session left open on a phone can't hand out the whole server.
  const [password, setPassword] = useState('')
  const [otp, setOtp] = useState('')
  // After a wrong PIN or password the field to type again has the focus, so a phone's
  // keyboard is already up for it.
  const pinRef = useRef<HTMLInputElement | null>(null)
  const passwordRef = useRef<HTMLInputElement | null>(null)
  // Which of them, once the answer is in and the fields can be typed in again.
  const refocus = useRef<'pin' | 'password' | null>(null)
  useEffect(() => {
    if (busy || !refocus.current) return
    ;(refocus.current === 'pin' ? pinRef : passwordRef).current?.focus()
    refocus.current = null
  }, [busy])

  const look = async (typed: string) => {
    const c = tidyCode(typed)
    setCode(c); setErr(''); setBusy(true)
    try { const a = await api.post<Asking>('/pair/info', { code: c }); setAsking(a); setWho(user?.id || 0); setPin(''); setPinErr(''); setPassword(''); setOtp('') } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  // A code in the address only fills in the form's first step: the question
  // below is still asked.
  useEffect(() => { const c = params.get('code'); if (c) look(c) }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const needsPin = !!asking?.needsPin
  const needsPassword = !!user?.isAdmin && who === user.id
  // The server says whether this person's password comes with a code from their authenticator.
  const twoFactor = !!(asking ? asking.twoFactor ?? user?.twoFactor : user?.twoFactor)
  const pinReady = (!needsPin || pin.length === PIN_LENGTH) && (!needsPassword || (password !== '' && (!twoFactor || otp.trim() !== '')))
  const confirm = needsPassword ? { password, otp: otp.trim() } : {}
  const approve = async () => {
    if (!pinReady) { setPinErr(needsPin && pin.length !== PIN_LENGTH ? `Type the ${PIN_LENGTH}-digit PIN the TV shows now.` : 'Type your password to sign a device in as an administrator.'); return }
    setErr(''); setPinErr(''); setBusy(true)
    try {
      setDone(await api.post<{ device: string; user: User }>('/pair/approve', needsPin ? { code: asking!.code, userId: who, pin, ...confirm } : { code: asking!.code, userId: who, ...confirm }))
    } catch (e: any) {
      const reason = e instanceof ApiError ? e.data?.reason : undefined
      if (e instanceof ApiError && e.status === 403 && needsPassword && reason !== 'locked') {
        // The password or code wasn't right: the question stays open.
        setPinErr(e.message); setPassword(''); setOtp('')
        refocus.current = 'password'
      } else if (needsPin && reason === 'wrong-pin') {
        // The question stays open: the PIN changes every 30 seconds, so another try is normal.
        const left = e.data?.triesLeft
        setPinErr(typeof left === 'number' && left > 0 ? `${e.message} ${left === 1 ? 'One more try' : `${left} more tries`} before the sign-in ends.` : e.message)
        setPin('')
        refocus.current = 'pin'
      } else if (reason === 'locked') {
        setErr('Too many wrong PINs were typed, so this sign-in has ended. Start again on the TV.'); setAsking(null)
      } else if (reason === 'expired' || reason === 'used' || reason === 'not-found' || (e instanceof ApiError && e.status === 410)) {
        setErr(e.message || 'This sign-in has expired. Start again on the TV.'); setAsking(null)
      } else {
        setErr(e.message); setAsking(null)
      }
    } finally { setBusy(false) }
  }
  const deny = async () => {
    setErr(''); setBusy(true)
    try { await api.post('/pair/deny', { code: asking!.code }); setDone('denied') } catch (e: any) { setErr(e.message); setAsking(null) } finally { setBusy(false) }
  }

  // Which way round: here the code comes from this page, to type on the device.
  const [inv, setInv] = useState<Invite | null>(null)
  const [left, setLeft] = useState(0)
  const [choices, setChoices] = useState<{ id: number; name: string; kind: string }[]>([])
  useEffect(() => {
    api.get<UserView[]>('/users').then((us) => setChoices(us.filter((u) => !u.isAdmin && u.id !== user?.id))).catch(() => {})
    api.get<{ fingerprint: string }>('/capabilities').then((c) => setPrint(c.fingerprint)).catch(() => {})
  }, [user?.id])
  const [print, setPrint] = useState('')
  useEffect(() => {
    if (left <= 0) { if (inv) setInv(null); return }
    const t = setTimeout(() => setLeft(left - 1), 1000)
    return () => clearTimeout(t)
  }, [left]) // eslint-disable-line react-hooks/exhaustive-deps
  // What went wrong getting a code, said beside that button and not under the other form.
  const [invErr, setInvErr] = useState('')
  const getCode = async () => {
    setInvErr(''); setBusy(true)
    try { const i = await api.post<Invite>('/pair/invite', { userId: who, ...confirm }); setInv(i); setLeft(i.expiresInSec); setPassword(''); setOtp('') } catch (e: any) {
      setInvErr(e.message)
      if (e instanceof ApiError && e.status === 403 && needsPassword) { setPassword(''); setOtp('') }
    } finally { setBusy(false) }
  }

  if (user && user.kind !== 'adult' && !user.isAdmin) {
    return (
      <>
        <div className="toolbar"><h1>Sign in a TV</h1></div>
        <div className="panel pair-ask" role="status">
          <p>Signing in a TV is for grown-ups. Ask an adult to do it from their own profile.</p>
          <div className="panel-actions"><Link className="btn" to="/">Back to home</Link></div>
        </div>
      </>
    )
  }
  if (done) {
    return (
      <>
        <div className="toolbar"><h1>Sign in a TV</h1></div>
        <div className="panel pair-ask" role="status">
          {done === 'denied'
            ? <p>The sign-in was refused. The TV has been told.</p>
            : <p><b>{done.device}</b> is now signed in as <b>{done.user.name}</b>. It will continue by itself in a moment.</p>}
          <div className="panel-actions"><Link className="btn" to="/" ref={focusOnce}>Back to home</Link></div>
        </div>
      </>
    )
  }
  const profiles = (list: { id: number; name: string; kind: string }[]) => list.map((p) => <option key={p.id} value={p.id}>{p.name}{p.kind !== 'adult' ? ` (${p.kind === 'kids' ? 'Kids' : 'Guest'})` : ''}</option>)
  return (
    <>
      <div className="toolbar"><h1>Sign in a TV</h1></div>
      {asking ? (
        <form className="panel pair-ask" onSubmit={(e) => { e.preventDefault(); if (!busy) approve() }}>
          <h2>Is this your TV?</h2>
          <p><b>{asking.device}</b>{asking.platform ? ` (${asking.platform})` : ''} at {asking.ip} wants to sign in to this server.</p>
          <p className="panel-note">Only continue if you are looking at that code on your own TV right now.</p>
          <div className="form full">
            {asking.profiles.length > 1 && (
              <label>Sign it in as
                <select className="select" value={who} onChange={(e) => setWho(Number(e.target.value))}>{profiles(asking.profiles)}</select>
              </label>
            )}
            {needsPin && (
              <label>PIN on the TV
                <input ref={pinRef} className="input pair-pin" value={pin} autoFocus inputMode="numeric" pattern="[0-9]*" autoComplete="one-time-code"
                  maxLength={PIN_LENGTH} placeholder="123456" spellCheck={false} disabled={busy}
                  aria-describedby={pinErr ? 'pair-pin-help pair-pin-error' : 'pair-pin-help'} aria-invalid={pinErr ? true : undefined}
                  onChange={(e) => { setPin(e.target.value.replace(/\D/g, '').slice(0, PIN_LENGTH)); setPinErr('') }} />
                <span id="pair-pin-help" className="panel-note">The {PIN_LENGTH} digits under the QR code. They change every 30 seconds, so type the ones shown now.</span>
              </label>
            )}
            {needsPassword && (
              <label>Your password
                <input ref={passwordRef} className="input" type="password" value={password} autoComplete="current-password" disabled={busy} onChange={(e) => { setPassword(e.target.value); setPinErr('') }} />
                <span className="panel-note">This device would be an administrator’s, so your password is asked for again. Pick a Kids or Guest profile above to skip it.</span>
              </label>
            )}
            {needsPassword && twoFactor && (
              <label>Code from your authenticator app
                <input className="input" value={otp} inputMode="numeric" autoComplete="one-time-code" disabled={busy} onChange={(e) => { setOtp(e.target.value); setPinErr('') }} />
                <span className="panel-note">Or one of your recovery codes.</span>
              </label>
            )}
            {pinErr && <div id="pair-pin-error" className="error" role="alert">{pinErr}</div>}
            {err && <div className="error" role="alert">{err}</div>}
          </div>
          <div className="panel-actions">
            <button type="submit" className="btn primary" autoFocus={!needsPin && !needsPassword} disabled={busy || !pinReady}>Sign it in</button>
            <button type="button" className="btn ghost" disabled={busy} onClick={deny}>Refuse</button>
          </div>
        </form>
      ) : (
        <div className="pair-grid">
          <form className="panel" onSubmit={(e) => { e.preventDefault(); if (code.trim()) look(code) }}>
            <h2>Your TV shows a code</h2>
            <p className="panel-note">Type the code from its screen here. You’ll see which device is asking before anything happens.</p>
            <div className="form full">
              <label>Code from the TV
                <input className="input" value={code} onChange={(e) => setCode(e.target.value)} placeholder="ABCD-EFGH" autoFocus
                  autoComplete="off" autoCapitalize="characters" spellCheck={false} maxLength={12} />
              </label>
              {err && <div className="error" role="alert">{err}</div>}
            </div>
            <div className="panel-actions"><button className="btn primary" disabled={busy || !code.trim()}>Continue</button></div>
          </form>
          <form className="panel" onSubmit={(e) => { e.preventDefault(); if (!busy && !(needsPassword && !inv && password === '')) getCode() }}>
            <h2>Your TV asks for a code</h2>
            {inv ? (
              <div role="status">
                <p className="pair-code">{inv.code}</p>
                <p className="panel-note">Type this on the TV to sign it in as {inv.user.name}. It works once, for another {clock(left)}.</p>
              </div>
            ) : (
              <p className="panel-note">Get a code here and type it on the TV. Anyone who sees it could use it, so get it when you’re ready.</p>
            )}
            {choices.length > 0 && !inv && (
              <div className="form full">
                <label>Sign it in as
                  <select className="select" value={who} onChange={(e) => setWho(Number(e.target.value))}>
                    <option value={user?.id}>{user?.name}</option>
                    {profiles(choices)}
                  </select>
                </label>
              </div>
            )}
            {needsPassword && !inv && (
              <div className="form full">
                <label>Your password
                  <input className="input" type="password" value={password} autoComplete="current-password" disabled={busy} onChange={(e) => setPassword(e.target.value)} />
                  <span className="panel-note">A code for an administrator’s account asks for your password again.</span>
                </label>
                {twoFactor && <label>Code from your authenticator app<input className="input" value={otp} inputMode="numeric" autoComplete="one-time-code" disabled={busy} onChange={(e) => setOtp(e.target.value)} /></label>}
              </div>
            )}
            {invErr && <div className="error" role="alert">{invErr}</div>}
            <div className="panel-actions"><button type="submit" className="btn" disabled={busy || (needsPassword && !inv && password === '')}>{inv ? 'Get a new code' : 'Get a code'}</button></div>
          </form>
        </div>
      )}
      <p className="muted"><HelpLink to="apps#sign-in-with-a-code">How signing in with a code works</HelpLink></p>
      {print && <p className="pair-print muted">This server’s fingerprint is <b>{print}</b>. If the app shows one, the two should match.</p>}
    </>
  )
}
