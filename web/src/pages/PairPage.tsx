// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { HelpLink } from '../components/HelpLink'
import { useSearchParams } from 'react-router-dom'
import { api, ApiError, type User, type UserView } from '../api'
import { clock, tidyCode } from '../components/pairing'
import { useApp } from '../state'
import SpecPage from '../spec/SpecPage'
import { handledIds } from '../spec/model'
import type { SpecPageData } from '../spec/types'
import { pairAdultsOnlySpec, pairAskSpec, pairDoneSpec, pairEnterSpec, PIN_LENGTH } from '../spec/specs/pair'
import { isSubmitKey } from '../spec/specs/submit'
import './pair.css'

type Invite = { code: string; expiresInSec: number; user: User }
type Asking = { code: string; device: string; platform: string; ip: string; profiles: { id: number; name: string; kind: string; needsPassword?: boolean }[]; needsPin?: boolean; twoFactor?: boolean }

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
  // Which of them, once the answer is in and the fields can be typed in again.
  const refocus = useRef<'pin' | 'password' | null>(null)
  useEffect(() => {
    if (busy || !refocus.current) return
    document.querySelector<HTMLElement>(`[data-field="${refocus.current === 'pin' ? 'pin' : 'password'}"]`)?.focus()
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

  const input = (page: SpecPageData, run: () => void) => (
    <div onKeyDown={(e) => { if (isSubmitKey(e)) { e.preventDefault(); if (!busy) run() } }}>
      <SpecPage page={page} handled={handledIds(page)} onAction={(id) => { if (!busy) press(id) }} onField={field} />
    </div>
  )
  const press = (id: string) => {
    if (id === 'approve') approve()
    else if (id === 'deny') deny()
    else if (id === 'continue') { if (code.trim()) look(code) }
    else if (id === 'getcode') { if (!(needsPassword && !inv && password === '')) getCode() }
  }
  const field = (id: string, v: string | number | boolean | File) => {
    const t = String(v)
    if (id === 'code') setCode(t)
    else if (id === 'who' || id === 'who2') setWho(Number(t))
    else if (id === 'pin') { setPin(t); setPinErr('') }
    else if (id === 'password' || id === 'password2') { setPassword(t); setPinErr('') }
    else if (id === 'otp' || id === 'otp2') { setOtp(t); setPinErr('') }
  }
  if (user && user.kind !== 'adult' && !user.isAdmin) return <SpecPage page={pairAdultsOnlySpec()} />
  if (done) return <SpecPage page={pairDoneSpec(done === 'denied' ? 'denied' : { device: done.device, name: done.user.name })} />
  let body: ReactNode
  if (asking) {
    body = input(pairAskSpec({
      device: asking.device, platform: asking.platform, ip: asking.ip, profiles: asking.profiles, who, needsPin, pin, needsPassword, password, twoFactor, otp, busy, pinError: pinErr, error: err,
    }), approve)
  } else {
    body = input(pairEnterSpec({
      code, busy, error: err, invite: inv ? { code: inv.code, name: inv.user.name, left: clock(left) } : null, inviteError: invErr,
      choices: choices.map((c) => ({ id: c.id, name: c.name, kind: c.kind })), selfId: user?.id || 0, selfName: user?.name || '', who,
      needsPassword, password, twoFactor, otp, fingerprint: print,
    }), () => { if (code.trim()) look(code) })
  }
  return (
    <>
      {body}
      <p className="muted pair-help"><HelpLink to="apps#sign-in-with-a-code">How signing in with a code works</HelpLink></p>
    </>
  )
}
