import { useEffect, useRef, useState } from 'react'
import { api, type User } from '../../api'
import { legalAckText, useLegalPath } from '../../components/LegalNote'
import { Button, Field, Input } from '../../ui'
import { StepActions, StepHead, TickRow, WhereIsTheCode } from './parts'

// Everything is checked when Create account is pressed, with the reason beside the field, rather
// than by a greyed-out button: a remote can't land on a button that's off, and it says nothing about why.
type Problems = { name?: string; pw?: string; pw2?: string; code?: string; ack?: string }

function problemsWith(name: string, pw: string, pw2: string, needCode: boolean, code: string, ack: boolean): Problems {
  const p: Problems = {}
  if (!name.trim()) p.name = 'Type a name to sign in with.'
  if (pw.length < 8) p.pw = 'Your password needs at least 8 characters.'
  else if (pw !== pw2) p.pw2 = 'The two passwords are different. Type it again.'
  if (needCode && !code.trim()) p.code = 'Type the setup code. Where to find it is just below.'
  if (!ack) p.ack = 'Tick this to carry on.'
  return p
}

export function Account({ back, onDone }: { back: () => void; onDone: (u: User, token: string) => void }) {
  const [name, setName] = useState('')
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
      onDone(r.user, r.token)
    } catch (e: any) { setErr(e.message); setBusy(false) }
  }
  return (
    <form ref={form} onSubmit={submit} noValidate>
      <StepHead title="Create your account" lead="This account looks after the server: its libraries, people and settings. You can add a profile for everyone else later." />
      <div className="oobe-form su-form">
        <Field label="Name to sign in with" error={problems.name}><Input autoFocus value={name} onChange={(e) => setName(e.target.value)} autoComplete="username" autoCapitalize="off" spellCheck={false} /></Field>
        <Field label="Password" hint={<span className={'oobe-strength l' + level}><i /><i /><i /><span>{['At least 8 characters', '', 'Good', 'Strong'][level]}</span></span>} error={problems.pw}>
          <Input type={show ? 'text' : 'password'} value={pw} onChange={(e) => setPw(e.target.value)} autoComplete="new-password" />
        </Field>
        <Field label="Type it again" error={problems.pw2}><Input type={show ? 'text' : 'password'} value={pw2} onChange={(e) => setPw2(e.target.value)} autoComplete="new-password" /></Field>
        <TickRow checked={show} onChange={setShow} title="Show what I’m typing" />
        {needCode && (
          <>
            <Field label="Setup code" error={problems.code}><Input className="mono" value={code} onChange={(e) => setCode(e.target.value)} autoComplete="off" autoCapitalize="characters" spellCheck={false} /></Field>
            <WhereIsTheCode />
          </>
        )}
        <div className={'su-ack' + (problems.ack ? ' wrong' : '')}>
          <TickRow checked={ack} onChange={setAck} title={legalAckText()} />
          {problems.ack && <div className="ui-field-error" role="alert">{problems.ack}</div>}
          <a href={legal} target="_blank" rel="noreferrer">Read the legal notes (opens in a new tab)</a>
        </div>
      </div>
      {err && <div className="error su-gap" role="alert">{err}</div>}
      <StepActions back={back}>
        <Button type="submit" variant="primary" size="big" busy={busy}>{busy ? 'Creating…' : 'Create account'}</Button>
      </StepActions>
    </form>
  )
}
