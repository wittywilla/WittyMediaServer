// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useRef, useState } from 'react'
import { api, serverBase, setToken, type User } from '../api'
import { useApp } from '../state'
import { isBackKey } from '../ui/spatial'
import { approveLink, browserName, clock, onlyThisDevice, pinGroups, serverClockOffset, typedAddress } from './pairing'
import { QrCode } from './QrCode'
import '../pages/pair.css'

type Started = {
  sessionId: string; deviceSecret: string; pin: string; pinExpiresAt: number; period: number
  qr: string; code: string; approveUrl: string; expiresAt: number; expiresInSec: number; intervalSec: number
}
type Ended = 'expired' | 'denied' | 'locked' | 'failed'

const ENDINGS: Record<Ended, string> = {
  expired: 'That took too long, so the code has expired.',
  denied: 'The sign-in was refused on the other device.',
  locked: 'Too many wrong PINs were typed, so this code was ended.',
  failed: "This device couldn't ask the server for a code.",
}

// This device signs in by being approved from one that already is: it shows a QR
// code that opens the approval page on a phone, and a PIN that changes every 30
// seconds, which the person types there. The device's secret stays in this
// component's memory and goes to the server only in a header.
export function PairDevice({ onBack }: { onBack: () => void }) {
  const { setUser } = useApp()
  const [s, setS] = useState<Started | null>(null)
  const [pin, setPin] = useState({ pin: '', until: 0 })
  // The server's clock, which its times are by: this device's own may be minutes out (a TV's
  // often is), and a countdown by it would run early, late, or ask for a new PIN without end.
  const offset = useRef(0)
  const clockNow = () => Date.now() / 1000 + offset.current
  const [now, setNow] = useState(clockNow)
  const [ended, setEnded] = useState<Ended | null>(null)
  const [why, setWhy] = useState('')
  const live = useRef(true)
  // What the approval page will call this device, so the person can check it's this one.
  const name = browserName(navigator.userAgent)

  const ask = useCallback(async (path: string, secret: string) => {
    const res = await fetch(`${serverBase()}/api/auth/pair/${path}`, { headers: { 'X-WMS-Pair-Secret': secret }, credentials: 'same-origin', cache: 'no-store' })
    return { status: res.status, data: await res.json().catch(() => ({})) as any }
  }, [])

  const start = useCallback(async () => {
    setEnded(null); setWhy(''); setS(null)
    try {
      const r = await api.post<Started>('/auth/pair', { device: name, platform: 'web' })
      if (!live.current) return
      offset.current = serverClockOffset(r.expiresAt, r.expiresInSec, Date.now() / 1000)
      setNow(clockNow()); setS(r); setPin({ pin: r.pin, until: r.pinExpiresAt })
    } catch (e: any) { setWhy(e.message || ''); setEnded('failed') }
  }, [name])
  useEffect(() => { live.current = true; start(); return () => { live.current = false } }, [start])

  // A remote's Back, or Esc, goes back to the other ways of signing in: on a TV there may be
  // nothing else to press it with.
  useEffect(() => {
    const key = (e: KeyboardEvent) => { if ((e.key === 'Escape' || isBackKey(e)) && !e.defaultPrevented) { e.preventDefault(); onBack() } }
    window.addEventListener('keydown', key)
    return () => window.removeEventListener('keydown', key)
  }, [onBack])

  // The clock, for the two countdowns.
  useEffect(() => { const t = setInterval(() => setNow(clockNow()), 500); return () => clearInterval(t) }, [])

  // Its five minutes are up, whether or not the server could be asked.
  useEffect(() => { if (s && !ended && now >= s.expiresAt) setEnded('expired') }, [s, ended, now])

  // A new PIN when the one shown runs out.
  useEffect(() => {
    if (!s || ended || now < pin.until) return
    let stop = false
    ask(`${s.sessionId}/pin`, s.deviceSecret).then((r) => {
      if (stop || !live.current) return
      if (r.status === 200) setPin({ pin: r.data.pin, until: Math.max(Number(r.data.pinExpiresAt) || 0, now + 2) }) // never asked more often than every two seconds
      else setPin((p) => ({ ...p, until: now + 3 })) // answered or gone: the poll below says which
    }).catch(() => setPin((p) => ({ ...p, until: now + 3 })))
    return () => { stop = true }
  }, [s, ended, now >= pin.until]) // eslint-disable-line react-hooks/exhaustive-deps

  // Has someone answered?
  useEffect(() => {
    if (!s || ended) return
    const t = setInterval(async () => {
      try {
        const r = await ask(s.sessionId, s.deviceSecret)
        if (!live.current) return
        const st = r.data?.status
        if (r.status === 200 && st === 'approved') { clearInterval(t); setToken(r.data.token); setUser(r.data.user as User) }
        else if (st === 'denied' || st === 'locked' || st === 'expired') { clearInterval(t); setEnded(st) }
      } catch { /* the network blinked: the next poll tries again */ }
    }, Math.max(2, s.intervalSec) * 1000)
    return () => clearInterval(t)
  }, [s, ended, ask, setUser])

  if (ended) {
    return (
      <div className="auth-card pair-device ended" role="alert">
        <h2>Sign in with your phone</h2>
        <p>{ENDINGS[ended]} {why}</p>
        <div className="panel-actions"><button className="btn primary" autoFocus onClick={start}>Get a new code</button><button className="btn ghost" onClick={onBack}>Use a password instead</button></div>
      </div>
    )
  }
  if (!s) return <div className="auth-card pair-device ended" role="status" aria-label="Getting a code"><div className="spinner" /></div>
  const pinLeft = Math.max(0, Math.ceil(pin.until - now))
  const left = s.expiresAt - now
  // A web address a phone's camera opens: the server's own when it knows it, else where this
  // device reached it. Without either (an app that isn't on a web address), the code for the apps.
  const link = approveLink(s.approveUrl, serverBase() || window.location.origin)
  const address = typedAddress(link)
  const local = !!link && onlyThisDevice(link)
  return (
    <div className="auth-card pair-device">
      <h2>Sign in with your phone</h2>
      <div className="pair-device-body">
        <div className="pair-qr">
          <QrCode text={link || s.qr} label="QR code that opens the approval page" />
          {address && !local && <p className="pair-address">or open <b>{address}</b></p>}
        </div>
        <div className="pair-how">
          <ol className="pair-steps">
            {local
              ? <li>On a device that’s already signed in, open <b>Sign in a TV</b> and type <b className="pair-inline">{s.code}</b>. (A phone can’t use the picture: this device reached the server as <b>{new URL(link).hostname}</b>, an address that only works here.)</li>
              : <li>Scan this with your phone’s camera. Or open <b>Sign in a TV</b> on a device that’s already signed in, and type <b className="pair-inline">{s.code}</b>.</li>}
            <li>Check it names this device, <b>{name}</b>, then type this PIN there.</li>
          </ol>
          <p className="pair-code" aria-live="polite" aria-atomic="true" aria-label={`PIN ${pin.pin.split('').join(' ')}`}>{pinGroups(pin.pin)}</p>
          <progress className="pair-countdown" max={s.period} value={pinLeft} aria-label="Time until the PIN changes" />
          <p className="panel-note">A new PIN in {pinLeft} s. This code lasts another {clock(left)}.</p>
        </div>
      </div>
      <div className="panel-actions"><button className="btn ghost" autoFocus onClick={onBack}>Use a password instead</button></div>
    </div>
  )
}
