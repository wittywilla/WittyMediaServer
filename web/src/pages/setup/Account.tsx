// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { api, setToken, type User } from '../../api'
import { DEFAULT_NAME } from '../../brandName'
import { legalAckText, useLegalPath } from '../../components/LegalNote'
import { useApp } from '../../state'
import { Button, Field, Input } from '../../ui'
import { TwoStep } from './TwoStep'
import { Customize, StepActions, StepHead, TickRow, WhereIsTheCode } from './parts'
import { t } from '../../i18n'

// Everything is checked when Create account is pressed, with the reason beside the field, rather
// than by a greyed-out button: a remote can't land on a button that's off, and it says nothing about why.
type Problems = { name?: string; pw?: string; pw2?: string; code?: string; ack?: string }

function problemsWith(name: string, pw: string, pw2: string, needCode: boolean, code: string, ack: boolean): Problems {
  const p: Problems = {}
  if (!name.trim()) p.name = t('Type a name to sign in with.')
  if (pw.length < 8) p.pw = t('Your password needs at least 8 characters.')
  else if (pw !== pw2) p.pw2 = t('The two passwords are different. Type it again.')
  if (needCode && !code.trim()) p.code = t('Type the setup code. Where to find it is just below.')
  if (!ack) p.ack = t('Tick this to carry on.')
  return p
}

// Step 2: the administrator's account and the server's name, on one page. The name people see and two-step sign-in are
// under Customize. In advanced setup those two are on the page, and the server's name, colours and logo get a page next.
export function Account({ back, onDone, advanced }: { back: () => void; onDone: (u: User, token: string) => void; advanced?: boolean }) {
  const { setBrandName } = useApp()
  const [name, setName] = useState('')
  const [server, setServer] = useState('')
  // What people at home call this profile: optional, and the sign-in name when empty.
  const [shown, setShown] = useState('')
  const [pw, setPw] = useState('')
  const [pw2, setPw2] = useState('')
  const [show, setShow] = useState(false)
  const [code, setCode] = useState('')
  const [needCode, setNeedCode] = useState(false)
  // One tick, once per server: a click is harder to miss than a line of text.
  const [ack, setAck] = useState(false)
  const [problems, setProblems] = useState<Problems>({})
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  // Two-step sign-in wanted: it is set up as soon as the account exists, before setup goes on.
  const [two, setTwo] = useState(false)
  const [made, setMade] = useState<{ user: User; token: string } | null>(null)
  const form = useRef<HTMLFormElement>(null)
  const legal = useLegalPath('your-content')
  useEffect(() => { api.get<{ setupCodeRequired?: boolean }>('/status').then((s) => setNeedCode(!!s.setupCodeRequired)).catch(() => {}) }, [])
  const level = pw.length >= 12 ? 3 : pw.length >= 8 ? 2 : 0
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setErr('')
    const p = problemsWith(name, pw, pw2, needCode, code, ack)
    setProblems(p)
    if (Object.keys(p).length) {
      requestAnimationFrame(() => form.current?.querySelector<HTMLElement>('[aria-invalid=true], .su-ack.wrong button')?.focus())
      return
    }
    setBusy(true)
    try {
      const r = await api.post<{ token: string; user: User }>('/setup', { name: name.trim(), password: pw, setupCode: code.trim(), acknowledgedLegal: ack, device: navigator.userAgent.slice(0, 80) })
      setToken(r.token)
      // Neither of these is worth stopping for: both are in Settings.
      if (shown.trim()) await api.put(`/users/${r.user.id}/profile`, { displayName: shown.trim() }).catch(() => {})
      if (server.trim() && server.trim() !== DEFAULT_NAME) {
        await api.put('/settings', { brandName: server.trim(), brandColouring: 'auto' }).then(() => setBrandName(server.trim(), 'auto')).catch(() => {})
      }
      if (two) { setMade({ user: r.user, token: r.token }); setBusy(false) } else onDone(r.user, r.token)
    } catch (e: any) { setErr(e.message); setBusy(false) }
  }
  if (made) return <TwoStep password={pw} onDone={() => onDone(made.user, made.token)} />
  const more = (
    <>
      <Field label={t('Name people see')} hint={t('Optional. Shown around the app instead of the name you sign in with.')}><Input value={shown} onChange={(e) => setShown(e.target.value)} maxLength={40} autoComplete="nickname" /></Field>
      <TickRow checked={two} onChange={setTwo} title={t('Two-step sign-in')} text={t('Signing in also asks for a code from an app on your phone, so a stolen password isn’t enough. You set it up right after the account is made.')} />
    </>
  )
  return (
    <form ref={form} onSubmit={submit} noValidate className="su-narrow">
      <StepHead title={t('Create your account')} lead={t('You’ll use it to sign in and look after the server.')} />
      <div className="oobe-form su-form">
        <Field label={t('Name to sign in with')} error={problems.name}><Input autoFocus value={name} onChange={(e) => setName(e.target.value)} autoComplete="username" autoCapitalize="off" spellCheck={false} /></Field>
        <Field label={t('Password')} hint={<span className={'oobe-strength l' + level}><i /><i /><i /><span>{[t('At least 8 characters'), '', t('Good'), t('Strong')][level]}</span></span>} error={problems.pw}>
          <Input type={show ? 'text' : 'password'} value={pw} onChange={(e) => setPw(e.target.value)} autoComplete="new-password" />
        </Field>
        <Field label={t('Type it again')} error={problems.pw2}><Input type={show ? 'text' : 'password'} value={pw2} onChange={(e) => setPw2(e.target.value)} autoComplete="new-password" /></Field>
        <TickRow plain checked={show} onChange={setShow} title={t('Show what I’m typing')} />
        {advanced && more}
        {!advanced && <Field label={t('Server name')}><Input value={server} maxLength={40} placeholder={DEFAULT_NAME} onChange={(e) => setServer(e.target.value)} autoComplete="off" /></Field>}
        {needCode && (
          <>
            <Field label={t('Setup code')} error={problems.code}><Input className="mono" value={code} onChange={(e) => setCode(e.target.value)} autoComplete="off" autoCapitalize="characters" spellCheck={false} /></Field>
            <WhereIsTheCode />
          </>
        )}
        <div className={'su-ack' + (problems.ack ? ' wrong' : '')}>
          <TickRow checked={ack} onChange={setAck} title={legalAckText()} />
          {problems.ack && <div className="ui-field-error" role="alert">{problems.ack}</div>}
          <a href={legal} target="_blank" rel="noreferrer">{t('Read the legal notes (opens in a new tab)')}</a>
        </div>
      </div>
      {err && <div className="error su-gap" role="alert">{err}</div>}
      <StepActions back={back}>
        {!advanced && <Customize title={t('Account options')}>{more}</Customize>}
        <Button type="submit" variant="primary" size="big" busy={busy}>{busy ? t('Creating…') : t('Create account')}</Button>
      </StepActions>
    </form>
  )
}
