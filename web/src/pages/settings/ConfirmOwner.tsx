import { useId, useState, type ReactNode } from 'react'
import { useApp } from '../../state'
import { Button, Dialog } from '../../ui'

// Your password, and code if you use one, before a change to which code runs on this server.
export function OwnerFields({ password, code, setPassword, setCode, autoFocus }: { password: string; code: string; setPassword: (s: string) => void; setCode: (s: string) => void; autoFocus?: boolean }) {
  const { user } = useApp()
  return (
    <>
      <label className="set-field">Your password
        <input className="input" type="password" autoFocus={autoFocus} autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
      </label>
      {user?.twoFactor && <label className="set-field">Code from your authenticator app
        <input className="input" inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} />
      </label>}
    </>
  )
}

export type OwnerAsk = { title: string; body: ReactNode; action: string; run: (password: string, code: string) => Promise<void> }

// Stays open on a refusal, so a mistyped password can be typed again.
export function ConfirmOwner({ ask, onClose }: { ask: OwnerAsk; onClose: () => void }) {
  const { user } = useApp()
  const formId = useId()
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [err, setErr] = useState('')
  const [working, setWorking] = useState(false)
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (working) return
    setWorking(true); setErr('')
    try { await ask.run(password, code); onClose() } catch (x: any) { setErr(x.message); setCode(''); setWorking(false) }
  }
  return (
    <Dialog alert size="sm" title={ask.title} onClose={onClose}
      actions={<>
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button variant="primary" type="submit" form={formId} busy={working} disabled={!password || (!!user?.twoFactor && !code)}>{ask.action}</Button>
      </>}>
      <form id={formId} className="ui-dialog-form" onSubmit={submit}>
        <div className="ms-ask">{ask.body}</div>
        <OwnerFields autoFocus password={password} code={code} setPassword={setPassword} setCode={setCode} />
        {err && <p className="error" role="alert">{err}</p>}
      </form>
    </Dialog>
  )
}
