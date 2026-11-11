// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useRef, useState } from 'react'
import { api, serverBase, setToken, type User } from '../api'
import { useApp } from '../state'
import { QrCode } from './QrCode'
import '../pages/pair.css'

type Started = {
  sessionId: string; deviceSecret: string; pin: string; pinExpiresAt: number; period: number
  qr: string; code: string; approveUrl: string; expiresAt: number; intervalSec: number
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
  const [now, setNow] = useState(() => Date.now() / 1000)
  const [ended, setEnded] = useState<Ended | null>(null)
  const [why, setWhy] = useState('')
  const live = useRef(true)

  const ask = useCallback(async (path: string, secret: string) => {
    const res = await fetch(`${serverBase()}/api/auth/pair/${path}`, { headers: { 'X-WMS-Pair-Secret': secret }, credentials: 'same-origin', cache: 'no-store' })
    return { status: res.status, data: await res.json().catch(() => ({})) as any }
  }, [])

  const start = useCallback(async () => {
    setEnded(null); setWhy(''); setS(null)
    try {
      const r = await api.post<Started>('/auth/pair', { device: navigator.userAgent.slice(0, 60), platform: 'web' })
      if (!live.current) return
      setS(r); setPin({ pin: r.pin, until: r.pinExpiresAt })
    } catch (e: any) { setWhy(e.message || ''); setEnded('failed') }
  }, [])
  useEffect(() => { live.current = true; start(); return () => { live.current = false } }, [start])

  // The clock, for the two countdowns.
  useEffect(() => { const t = setInterval(() => setNow(Date.now() / 1000), 500); return () => clearInterval(t) }, [])

  // A new PIN when the one shown runs out.
  useEffect(() => {
    if (!s || ended || now < pin.until) return
    let stop = false
    ask(`${s.sessionId}/pin`, s.deviceSecret).then((r) => {
      if (stop || !live.current) return
      if (r.status === 200) setPin({ pin: r.data.pin, until: r.data.pinExpiresAt })
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
      <div className="auth-card pair-device" role="alert">
        <h2>Sign in with your phone</h2>
        <p>{ENDINGS[ended]} {why}</p>
        <div className="panel-actions"><button className="btn primary" autoFocus onClick={start}>Get a new code</button><button className="btn ghost" onClick={onBack}>Back</button></div>
      </div>
    )
  }
  if (!s) return <div className="auth-card pair-device"><div className="spinner" /></div>
  const pinLeft = Math.max(0, Math.ceil(pin.until - now))
  const left = Math.max(0, Math.ceil(s.expiresAt - now))
  // A web address a phone's camera opens. Without one (the server doesn't know its own
  // address), the code for the apps.
  const link = /^https?:\/\//.test(s.approveUrl) ? s.approveUrl : s.qr
  return (
    <div className="auth-card pair-device">
      <h2>Sign in with your phone</h2>
      <ol className="pair-steps">
        <li>Scan this with your phone’s camera, or open <b>Sign in a TV</b> on a device that’s already signed in and type <b className="pair-inline">{s.code}</b>.</li>
        <li>Check it names this device, then type this PIN there.</li>
      </ol>
      <QrCode text={link} label="QR code that opens the approval page" />
      <p className="pair-code" aria-live="polite" aria-label={`PIN ${pin.pin.split('').join(' ')}`}>{pin.pin.slice(0, 3)} {pin.pin.slice(3)}</p>
      <progress className="pair-countdown" max={s.period} value={pinLeft} aria-label="Time until the PIN changes" />
      <p className="panel-note">A new PIN in {pinLeft} s. This code lasts another {Math.floor(left / 60)}:{String(left % 60).padStart(2, '0')}.</p>
      <div className="panel-actions"><button className="btn ghost" autoFocus onClick={onBack}>Use a password instead</button></div>
    </div>
  )
}
