import { useState } from 'react'
import { api, setToken, type User } from '../api'
import { useApp } from '../state'
import { Brand, VersionTag } from '../components/common'

export default function AuthPage({ setup }: { setup: boolean }) {
  const { setUser, brandName, version } = useApp()
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setErr('')
    if (setup && password !== confirm) { setErr('Passwords do not match'); return }
    setBusy(true)
    try {
      const r = await api.post<{ token: string; user: User }>(setup ? '/setup' : '/login', { name, password, device: navigator.userAgent.slice(0, 80) })
      setToken(r.token)
      setUser(r.user)
    } catch (e: any) {
      setErr(e.message || 'Failed')
    } finally { setBusy(false) }
  }

  return (
    <div className="center">
      <form className="auth-card" onSubmit={submit}>
        <div className="brand"><img src="/icon.svg" alt="" /><Brand name={brandName} /></div>
        <p>{setup ? 'Welcome! Create the administrator account to get started.' : 'Sign in to your media server.'}</p>
        <div className="form">
          <label>Name<input className="input" autoFocus value={name} onChange={(e) => setName(e.target.value)} autoComplete="username" /></label>
          <label>Password<input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete={setup ? 'new-password' : 'current-password'} /></label>
          {setup && <label>Confirm password<input className="input" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} /></label>}
          {err && <div className="error">{err}</div>}
          <button className="btn primary" disabled={busy} style={{ justifyContent: 'center' }}>{setup ? 'Create account' : 'Sign in'}</button>
        </div>
      </form>
      <div className="auth-version"><VersionTag version={version} /></div>
    </div>
  )
}
