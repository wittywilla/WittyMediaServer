// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, ApiError } from '../api'
import { QrCode } from '../components/QrCode'
import { toast, useApp } from '../state'
import { Button } from '../ui'
import { Field, Input } from '../ui/Field'

// From away (docs/P2P.md; canvas pages `connect` and `away`): using this server from outside the home network, with nothing set up
// on the router. Three ways in, each one plain: a link or QR code from here for a phone, tablet or TV app to join with; the code
// a TV or other device away from home is showing, typed in here; or an address typed into the app. Anyone may read it; the
// parts that make an invite or take a code need someone signed in (and an administrator's password for an administrator's).
type P2P = { enabled?: boolean; on?: boolean }
type Invite = { link: string; expiresInSec: number }

export default function AwayPage() {
  const { user } = useApp()
  const [on, setOn] = useState<boolean | null>(null)
  const [join, setJoin] = useState(false)
  useEffect(() => {
    if (!user) return
    api.get<P2P>('/p2p').then((p) => setOn(p.enabled ?? p.on ?? true)).catch(() => setOn(null))
    api.get<{ features?: string[] }>('/capabilities').then((c) => setJoin(!!c.features?.includes('p2p-join'))).catch(() => {})
  }, [user])
  return (
    <div className="away">
      <h1>From away</h1>
      <p className="away-lead">Use this server away from home, on a phone, a tablet or a TV, with nothing to set up on your router. The device finds this server by its connection ID and the connection is private all the way.</p>
      {!user && (
        <section className="away-card" aria-labelledby="away-in">
          <h2 id="away-in">Connect a device from away</h2>
          <ol className="away-steps">
            <li>In the WittyMedia app, choose <b>Connect to a remote server</b>.</li>
            <li>Pick <b>Join with a link or code</b>, <b>Scan a QR code</b> or <b>Enter an address</b>.</li>
            <li>Someone who is signed in to this server makes the link or QR code, or types in the code your device shows. <Link to="/login">Sign in</Link> here to do it.</li>
          </ol>
        </section>
      )}
      {user && on === false && (
        <div className="ui-notice warn" role="status"><div className="ui-notice-text"><strong>From away is off</strong><span>{user.isAdmin ? <>Turn it on in <Link to="/settings/remote">Settings, Remote Access</Link>.</> : 'An administrator can turn it on in Settings, Remote Access.'}</span></div></div>
      )}
      {user && on !== false && (
        <>
          <InviteCard />
          {join && <CodeCard />}
        </>
      )}
      <section className="away-card" aria-labelledby="away-addr">
        <h2 id="away-addr">Enter an address</h2>
        <p>An app can also be told this server’s address by hand: the one in <Link to={user ? '/settings/remote' : '/login'}>Settings, Remote Access</Link>, such as <code>https://media.example.org</code>.</p>
      </section>
    </div>
  )
}

// A link and a QR code for one device (docs/P2P.md, "Invites"): good once, for fifteen minutes.
function InviteCard() {
  const { user } = useApp()
  const [inv, setInv] = useState<Invite | null>(null)
  const [need, setNeed] = useState(false)
  const [password, setPassword] = useState('')
  const [otp, setOtp] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const make = async () => {
    setErr(''); setBusy(true)
    try { setInv(await api.post<Invite>('/p2p/invite', { userId: user!.id, ...(need ? { password, otp } : {}) })) }
    catch (e) {
      if (e instanceof ApiError && e.status === 403 && (e.data as { reason?: string } | undefined)?.reason === 'password-required') { setNeed(true); setErr('Type your password again to make an invite that signs in as an administrator.') } else setErr((e as Error).message)
    } finally { setBusy(false) }
  }
  return (
    <section className="away-card" aria-labelledby="away-link">
      <h2 id="away-link">Join with a link or scan a QR code</h2>
      <p>Make a link for one device. Open it on the device, or scan the code with its camera. It works once and lasts fifteen minutes.</p>
      {need && (
        <div className="away-fields">
          <Field label="Your password"><Input type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} /></Field>
          {user?.twoFactor && <Field label="Your sign-in code"><Input inputMode="numeric" autoComplete="one-time-code" value={otp} onChange={(e) => setOtp(e.target.value)} /></Field>}
        </div>
      )}
      {err && <p role="alert" className="away-err">{err}</p>}
      <Button variant="primary" busy={busy} onClick={() => void make()}>{inv ? 'Make another' : 'Make a link'}</Button>
      {inv && (
        <div className="away-invite">
          <QrCode text={inv.link} label="QR code for joining this server from away" />
          <div>
            <p><code className="away-link-text">{inv.link}</code></p>
            <Button size="sm" onClick={() => { void navigator.clipboard?.writeText(inv.link).then(() => toast('Link copied', { tone: 'ok' })).catch(() => toast('Copy it by hand')) }}>Copy the link</Button>
          </div>
        </div>
      )}
    </section>
  )
}

// A TV or other device away from home shows a code; someone signed in types it in here.
function CodeCard() {
  const { user } = useApp()
  const [code, setCode] = useState('')
  const [need, setNeed] = useState(false)
  const [password, setPassword] = useState('')
  const [otp, setOtp] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [done, setDone] = useState(false)
  const send = async () => {
    setErr(''); setBusy(true); setDone(false)
    try { await api.post('/p2p/join', { code: code.trim().toUpperCase(), userId: user!.id, ...(need ? { password, otp } : {}) }); setDone(true); setCode(''); toast('The device is joining', { tone: 'ok' }) }
    catch (e) {
      if (e instanceof ApiError && e.status === 403 && (e.data as { reason?: string } | undefined)?.reason === 'password-required') { setNeed(true); setErr('Type your password again to sign a device in as an administrator.') } else setErr((e as Error).message)
    } finally { setBusy(false) }
  }
  return (
    <section className="away-card" aria-labelledby="away-code">
      <h2 id="away-code">A device that shows a code</h2>
      <p>A TV away from home shows a code such as <code>K7QM-2XHD</code>. Type it here and the TV finds this server and signs in as you.</p>
      <form onSubmit={(e) => { e.preventDefault(); if (code.trim()) void send() }}>
        <div className="away-fields">
          <Field label="The code on the screen"><Input value={code} onChange={(e) => setCode(e.target.value)} autoCapitalize="characters" autoComplete="off" spellCheck={false} /></Field>
          {need && <Field label="Your password"><Input type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} /></Field>}
          {need && user?.twoFactor && <Field label="Your sign-in code"><Input inputMode="numeric" autoComplete="one-time-code" value={otp} onChange={(e) => setOtp(e.target.value)} /></Field>}
        </div>
        {err && <p role="alert" className="away-err">{err}</p>}
        {done && <p role="status">Sent. The device signs in within a moment.</p>}
        <Button variant="primary" type="submit" busy={busy} disabled={!code.trim()}>Connect the device</Button>
      </form>
    </section>
  )
}
