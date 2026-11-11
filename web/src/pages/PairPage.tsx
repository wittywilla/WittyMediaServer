// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { HelpLink } from '../components/HelpLink'
import { Link, useSearchParams } from 'react-router-dom'
import { api, ApiError, type User, type UserView } from '../api'
import { useApp } from '../state'
import './pair.css'

type Invite = { code: string; expiresInSec: number; user: User }
type Asking = { code: string; device: string; platform: string; ip: string; profiles: { id: number; name: string; kind: string }[]; needsPin?: boolean }

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

  const look = async (c: string) => {
    setErr(''); setBusy(true)
    try { const a = await api.post<Asking>('/pair/info', { code: c }); setAsking(a); setWho(user?.id || 0); setPin(''); setPinErr('') } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  // A code in the address only fills in the form's first step: the question
  // below is still asked.
  useEffect(() => { const c = params.get('code'); if (c) look(c) }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const needsPin = !!asking?.needsPin
  const pinReady = !needsPin || pin.length === PIN_LENGTH
  const approve = async () => {
    if (!pinReady) { setPinErr(`Type the ${PIN_LENGTH}-digit PIN the TV shows now.`); return }
    setErr(''); setPinErr(''); setBusy(true)
    try {
      setDone(await api.post<{ device: string; user: User }>('/pair/approve', needsPin ? { code: asking!.code, userId: who, pin } : { code: asking!.code, userId: who }))
    } catch (e: any) {
      const reason = e instanceof ApiError ? e.data?.reason : undefined
      if (needsPin && reason === 'wrong-pin') {
        // The question stays open: the PIN changes every 30 seconds, so another try is normal.
        const left = e.data?.triesLeft
        setPinErr(typeof left === 'number' && left > 0 ? `${e.message} ${left === 1 ? 'One more try' : `${left} more tries`} before the sign-in ends.` : e.message)
        setPin('')
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
  const getCode = async () => {
    setErr(''); setBusy(true)
    try { const i = await api.post<Invite>('/pair/invite', { userId: who }); setInv(i); setLeft(i.expiresInSec) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }

  if (user && user.kind !== 'adult' && !user.isAdmin) {
    return <><div className="toolbar"><h1>Sign in a TV</h1></div><p className="muted">An adult profile has to do this.</p></>
  }
  if (done) {
    return (
      <>
        <div className="toolbar"><h1>Sign in a TV</h1></div>
        <div className="panel pair-ask" role="status">
          {done === 'denied'
            ? <p>The sign-in was refused. The TV has been told.</p>
            : <p><b>{done.device}</b> is now signed in as <b>{done.user.name}</b>. It will continue by itself in a moment.</p>}
          <div className="panel-actions"><Link className="btn" to="/" autoFocus>Back to home</Link></div>
        </div>
      </>
    )
  }
  const profiles = (list: { id: number; name: string; kind: string }[]) => list.map((p) => <option key={p.id} value={p.id}>{p.name}{p.kind !== 'adult' ? ` (${p.kind === 'kids' ? 'Kids' : 'Guest'})` : ''}</option>)
  return (
    <>
      <div className="toolbar"><h1>Sign in a TV</h1></div>
      {asking ? (
        <div className="panel pair-ask">
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
                <input className="input pair-pin" value={pin} autoFocus inputMode="numeric" pattern="[0-9]*" autoComplete="one-time-code"
                  maxLength={PIN_LENGTH} placeholder="123456" spellCheck={false} disabled={busy}
                  aria-describedby={pinErr ? 'pair-pin-help pair-pin-error' : 'pair-pin-help'} aria-invalid={pinErr ? true : undefined}
                  onChange={(e) => { setPin(e.target.value.replace(/\D/g, '').slice(0, PIN_LENGTH)); setPinErr('') }}
                  onKeyDown={(e) => { if (e.key === 'Enter' && pin.length === PIN_LENGTH && !busy) approve() }} />
                <span id="pair-pin-help" className="panel-note">The {PIN_LENGTH} digits under the QR code. They change every 30 seconds, so type the ones shown now.</span>
              </label>
            )}
            {pinErr && <div id="pair-pin-error" className="error" role="alert">{pinErr}</div>}
            {err && <div className="error" role="alert">{err}</div>}
          </div>
          <div className="panel-actions">
            <button className="btn primary" autoFocus={!needsPin} disabled={busy || !pinReady} onClick={approve}>Sign it in</button>
            <button className="btn ghost" disabled={busy} onClick={deny}>Refuse</button>
          </div>
        </div>
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
          <div className="panel">
            <h2>Your TV asks for a code</h2>
            {inv ? (
              <div role="status">
                <p className="pair-code">{inv.code}</p>
                <p className="panel-note">Type this on the TV to sign it in as {inv.user.name}. It works once, for another {Math.floor(left / 60)}:{String(left % 60).padStart(2, '0')}.</p>
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
            <div className="panel-actions"><button className="btn" disabled={busy} onClick={getCode}>{inv ? 'Get a new code' : 'Get a code'}</button></div>
          </div>
        </div>
      )}
      <p className="muted"><HelpLink to="apps#sign-in-with-a-code">How signing in with a code works</HelpLink></p>
      {print && <p className="pair-print muted">This server’s fingerprint is <b>{print}</b>. If the app shows one, the two should match.</p>}
    </>
  )
}
