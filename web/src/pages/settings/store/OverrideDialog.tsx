// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useId, useRef, useState } from 'react'
import { IconShield } from '../../../icons'
import { useApp } from '../../../state'
import { Button, Dialog } from '../../../ui'
import { HelpLink } from '../../../components/HelpLink'
import './overrides.css'
import { friction, needsOverrideOf, refusalStatus, overrideLines, overrideRequest, refusalKind, severityOf, stillNeeded, type NeedsOverride, type OverrideAnswers } from './overrides'

// The override: installing something the usual rules refuse, knowingly. It names every rule being
// bypassed with the server's own words, has each one ticked, offers the permissions the module asks
// for one by one (none ticked), asks for the password and code again, and from severity 3 has the
// module's ID typed. The more serious it is, the heavier it looks, in the design's warning tones.
// A refusal is shown in place, next to what caused it, and the dialog stays open to correct it.

export type OverrideBody = ReturnType<typeof overrideRequest>

export function OverrideDialog({ id, name, need: first, permissionWords, onSubmit, onClose }: {
  id: string; name: string; need: NeedsOverride
  permissionWords?: Record<string, string>
  // Sends the request. It throws the server's refusal, which this dialog explains.
  onSubmit: (body: OverrideBody) => Promise<void>
  onClose: () => void
}) {
  const { user } = useApp()
  const twoFactor = !!user?.twoFactor
  const uid = useId()
  const [need, setNeed] = useState(first)
  const [a, setA] = useState<OverrideAnswers>({ accepted: [], password: '', code: '', typed: '', grants: [] })
  const [pwErr, setPwErr] = useState('')
  const [err, setErr] = useState('')
  const [working, setWorking] = useState(false)
  const pwRef = useRef<HTMLInputElement>(null)
  const errRef = useRef<HTMLParagraphElement>(null)
  const sev = severityOf(need)
  const f = friction(sev)
  const lines = overrideLines(need)
  const missing = stillNeeded(need, a, id, twoFactor)
  const perms = need.permissions || []
  const set = (k: keyof OverrideAnswers, v: string | string[]) => setA((x) => ({ ...x, [k]: v }))
  const toggle = (k: 'accepted' | 'grants', v: string, on: boolean) => setA((x) => ({ ...x, [k]: on ? [...x[k].filter((y) => y !== v), v] : x[k].filter((y) => y !== v) }))
  useEffect(() => { if (err) errRef.current?.focus() }, [err])

  const submit = async (e: React.FormEvent) => {
    // A dialog opened from inside another form is still inside it to React: keep the submit here.
    e.preventDefault(); e.stopPropagation()
    if (working || missing.length) return
    setWorking(true); setErr(''); setPwErr('')
    try {
      await onSubmit(overrideRequest(need, a))
      onClose()
    } catch (x) {
      const kind = refusalKind(x)
      const message = x instanceof Error ? x.message : String(x)
      if (kind === 'password') {
        setPwErr(message || 'That password isn’t right.')
        setA((y) => ({ ...y, password: '', code: '' }))
        setTimeout(() => pwRef.current?.focus(), 0)
      } else if (kind === 'override') {
        // The server found more to accept (or less): start the ticks again from what it says now.
        const n = needsOverrideOf(x)!
        setNeed(n)
        setA((y) => ({ ...y, accepted: [], grants: y.grants.filter((g) => (n.permissions || []).includes(g)) }))
        setErr(refusalStatus(x) === 400 && message ? `${message} Read the warnings again and tick each one.` : 'What this needs has changed. Read the warnings again and tick each one.')
      } else setErr(message)
      setWorking(false)
    }
  }

  return (
    <Dialog alert size="md" className={`ov-dialog ov-sev-${f.level}`} title={`Install ${name} anyway?`} onClose={onClose}
      description={`${lines.length === 1 ? 'One rule that normally stops this install' : `${lines.length} rules that normally stop this install`} would be bypassed. Severity ${f.level} of 4.`}
      actions={<>
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button variant={f.tone === 'danger' ? 'danger' : 'primary'} type="submit" form={uid + 'f'} busy={working} disabled={missing.length > 0} aria-describedby={uid + 'm'}>{f.button}</Button>
      </>}>
      <form id={uid + 'f'} className="ui-dialog-form ov-form" onSubmit={submit} noValidate>
        <div className={'ov-banner ' + f.tone}>
          <IconShield size={f.level >= 3 ? 28 : 22} />
          <div>
            <strong>{f.heading}</strong>
            {need.warning && !lines.some((l) => l.reason === need.warning) && <p>{need.warning}</p>}
            {need.version && <p className="ov-small">Version {need.version}{need.pending ? ', already downloaded and checked, waiting for your go-ahead' : ''}.</p>}
          </div>
        </div>

        <fieldset className="ov-set">
          <legend>Tick each one to say you accept it</legend>
          {lines.map((l, i) => (
            <label key={l.kind} className="ov-check">
              {/* The first warning has the focus, so a long dialog opens at its top. Ticking it alone does nothing. */}
              <input type="checkbox" autoFocus={i === 0} checked={a.accepted.includes(l.kind)} onChange={(e) => toggle('accepted', l.kind, e.target.checked)} />
              <span><b>{l.title}</b>{l.reason && <>{' '}<span className="ov-reason">{l.reason}</span></>}</span>
            </label>
          ))}
        </fieldset>

        {perms.length > 0 && (
          <fieldset className="ov-set">
            <legend>Permissions to give it</legend>
            <p className="ov-small">It gets none of these unless you tick them. Give only what you’re sure it needs.</p>
            {perms.map((p) => (
              <label key={p} className="ov-check">
                <input type="checkbox" checked={a.grants.includes(p)} onChange={(e) => toggle('grants', p, e.target.checked)} />
                <span><b>{p}</b>{permissionWords?.[p] && <>{' '}<span className="ov-reason">{permissionWords[p]}</span></>}</span>
              </label>
            ))}
          </fieldset>
        )}

        {f.typed && (
          <label className="set-field">To confirm, type the module’s ID: <code className="ov-id">{id}</code>
            <input className="input mono" value={a.typed} onChange={(e) => set('typed', e.target.value)} autoComplete="off" autoCapitalize="off" spellCheck={false}
              aria-invalid={!!a.typed && a.typed.trim() !== id ? true : undefined} />
          </label>
        )}

        <label className="set-field">Your password
          <input ref={pwRef} className="input" type="password" autoComplete="current-password" value={a.password} onChange={(e) => { set('password', e.target.value); setPwErr('') }}
            aria-invalid={pwErr ? true : undefined} aria-describedby={pwErr ? uid + 'p' : undefined} />
        </label>
        {twoFactor && <label className="set-field">Code from your authenticator app
          <input className="input" inputMode="numeric" autoComplete="one-time-code" value={a.code} onChange={(e) => set('code', e.target.value)} aria-invalid={pwErr ? true : undefined} aria-describedby={pwErr ? uid + 'p' : undefined} />
        </label>}
        {pwErr && <p className="error" id={uid + 'p'} role="alert">{pwErr}</p>}
        {err && <p className="error" role="alert" tabIndex={-1} ref={errRef}>{err}</p>}

        <p className="ov-small" id={uid + 'm'} aria-live="polite">{missing.length ? `To go on, ${missing.join(', ')}.` : 'Ready.'} This is written to the module audit log. <HelpLink newTab to="module-store#installing-past-a-rule">What overrides mean</HelpLink></p>
      </form>
    </Dialog>
  )
}
