import { useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useApp } from '../../state'

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
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [err, setErr] = useState('')
  const [working, setWorking] = useState(false)
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setWorking(true); setErr('')
    try { await ask.run(password, code); onClose() } catch (x: any) { setErr(x.message); setCode(''); setWorking(false) }
  }
  return createPortal(
    <div className="modal-backdrop" onClick={onClose}>
      <form className="modal set-modal" role="alertdialog" aria-label={ask.title} onClick={(e) => e.stopPropagation()} onSubmit={submit}>
        <h2>{ask.title}</h2>
        <div className="ms-ask">{ask.body}</div>
        <OwnerFields autoFocus password={password} code={code} setPassword={setPassword} setCode={setCode} />
        {err && <p className="error" role="alert">{err}</p>}
        <div className="actions">
          <button type="button" className="btn ghost" onClick={onClose}>Cancel</button>
          <button className="btn primary" disabled={working || !password || (!!user?.twoFactor && !code)}>{ask.action}</button>
        </div>
      </form>
    </div>, document.body)
}
