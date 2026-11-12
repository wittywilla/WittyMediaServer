// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../../api'
import { toast } from '../../state'
import { Button, Field, Input } from '../../ui'
import QrCode from '../settings/QrCode'

// Two-step sign-in, offered on the account page and carried out the moment the account exists (it needs one): a QR code for an
// authenticator app, the six digits it shows, then the recovery codes, shown once. Skipping is always fine; Settings, Security has
// the same thing later.
export function twoStepCode(v: string) { return v.replace(/\s/g, '') }

export function TwoStep({ password, onDone }: { password: string; onDone: () => void }) {
  const [setup, setSetup] = useState<{ secret: string; uri: string } | null>(null)
  const [code, setCode] = useState('')
  const [recovery, setRecovery] = useState<string[] | null>(null)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => { api.post<{ secret: string; uri: string }>('/me/2fa/setup', { password }).then(setSetup).catch((e: Error) => setErr(e.message)) }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const turnOn = async () => {
    setBusy(true); setErr('')
    try { setRecovery((await api.post<{ recovery: string[] }>('/me/2fa/confirm', { code: twoStepCode(code) })).recovery) } catch (e) { setErr((e as Error).message) } finally { setBusy(false) }
  }
  if (recovery) {
    return (
      <section>
        <h1>Two-step sign-in is on</h1>
        <div className="rem-recovery" role="alert">
          <b>Save these recovery codes now.</b>
          <p>Each works once, in place of a code, if you lose your phone. They won’t be shown again.</p>
          <ul>{recovery.map((c) => <li key={c}><code>{c}</code></li>)}</ul>
          <Button size="sm" onClick={() => navigator.clipboard.writeText(recovery.join('\n')).then(() => toast('Copied')).catch(() => toast('Select them and copy by hand'))}>Copy them</Button>
        </div>
        <div className="oobe-actions su-actions"><span /><Button variant="primary" size="big" onClick={onDone}>I saved them: continue</Button></div>
      </section>
    )
  }
  return (
    <form onSubmit={(e) => { e.preventDefault(); void turnOn() }}>
      <h1>Set up two-step sign-in</h1>
      <p className="lead">Signing in also asks for a 6-digit code from an app on your phone, so a stolen password isn’t enough. Any authenticator app works.</p>
      {setup ? (
        <div className="su-twostep">
          <QrCode text={setup.uri} label="QR code for your authenticator app" size={190} />
          <ol>
            <li>In your authenticator app, add an account and scan this code.</li>
            <li>Can’t scan? Choose to enter a key, and type: <code>{setup.secret.replace(/(.{4})/g, '$1 ').trim()}</code></li>
            <li>Enter the 6 digits it shows.</li>
          </ol>
          <Field label="Code from your app"><Input autoFocus inputMode="numeric" autoComplete="one-time-code" maxLength={7} value={code} onChange={(e) => setCode(e.target.value)} /></Field>
        </div>
      ) : !err && <p className="muted">Getting it ready…</p>}
      {err && <div className="error su-gap" role="alert">{err}</div>}
      <div className="oobe-actions su-actions">
        <Button variant="ghost" onClick={onDone}>Skip for now</Button>
        <Button type="submit" variant="primary" size="big" busy={busy} disabled={twoStepCode(code).length !== 6}>Turn on</Button>
      </div>
    </form>
  )
}
