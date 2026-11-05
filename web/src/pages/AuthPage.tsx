import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, ApiError, isNativeShell, profileName, setToken, type User } from '../api'
import ConnectServer from '../components/ConnectServer'
import { Brand, BrandMark, VersionTag } from '../components/common'
import { IconBack } from '../icons'
import { useApp } from '../state'
import { avatarColor } from '../ui/avatarColor'
import { useTvMode } from '../tvmode/mode'

type Profile = { id: number; name: string; displayName?: string; kind: 'adult' | 'kids' | 'guest'; hasPassword: boolean }

// Sign-in. On the home network this is a "Who's watching?" screen with a
// tile per profile; elsewhere (or when an admin chose it) the classic name
// and password form. The server decides which, see GET /api/profiles.
export default function AuthPage() {
  const { brandName, version, setUser } = useApp()
  // On a TV the username shows under a profile name that differs from it, as in the apps.
  const tv = useTvMode()
  const [tapErr, setTapErr] = useState('')
  // Kids and Guest profiles without a password sign in with one tap.
  const pick = async (p: Profile) => {
    if (p.hasPassword) { setChosen(p); return }
    setTapErr('')
    try {
      const r = await api.post<{ token: string; user: User }>('/login', { name: p.name, password: '', device: navigator.userAgent.slice(0, 80) })
      setToken(r.token)
      setUser(r.user)
    } catch (e: any) { setTapErr(e.message) }
  }
  const [profiles, setProfiles] = useState<Profile[] | null>(null)
  const [style, setStyle] = useState<'profiles' | 'classic' | null>(null)
  const [chosen, setChosen] = useState<Profile | null>(null)
  const [other, setOther] = useState(false)
  const [changing, setChanging] = useState(false)
  useEffect(() => {
    api.get<{ style: 'profiles' | 'classic'; profiles?: Profile[] }>('/profiles')
      .then((r) => { setStyle(r.profiles?.length ? r.style : 'classic'); setProfiles(r.profiles || []) })
      .catch(() => setStyle('classic'))
  }, [])
  if (!style) return <div className="center"><div className="spinner" /></div>
  if (changing) return <ConnectServer onCancel={() => setChanging(false)} />
  const changeServer = isNativeShell() && <button type="button" className="btn ghost sm" onClick={() => setChanging(true)}>Change server</button>

  if (style === 'profiles' && !chosen && !other) {
    return (
      <div className="who">
        <div className="who-brand"><BrandMark /><Brand name={brandName} /></div>
        <h1>Who's watching?</h1>
        <ul className="who-grid" role="list">
          {profiles!.map((p, i) => (
            <li key={p.id}>
              <button type="button" className="who-tile" autoFocus={i === 0} onClick={() => pick(p)}>
                <Avatar name={profileName(p)} big />
                <span className="who-name">{profileName(p)}</span>
                {tv && profileName(p) !== p.name && <span className="who-user">@{p.name}</span>}
                {p.kind !== 'adult' && <span className={'who-kind kind-' + p.kind}>{p.kind === 'kids' ? 'Kids' : 'Guest'}</span>}
              </button>
            </li>
          ))}
        </ul>
        {tapErr && <div className="error" role="alert">{tapErr}</div>}
        <button type="button" className="btn ghost" onClick={() => setOther(true)}>Sign in with another account</button>
        {changeServer}
        <div className="auth-version"><VersionTag version={version} /> · <Link to="/help">Help</Link> · <Link to="/legal">Legal</Link></div>
      </div>
    )
  }
  return (
    <div className="center">
      <PasswordForm profile={chosen} onBack={style === 'profiles' ? () => { setChosen(null); setOther(false) } : undefined} />
      {changeServer}
      <div className="auth-version"><VersionTag version={version} /> · <Link to="/help">Help</Link> · <Link to="/legal">Legal</Link></div>
    </div>
  )
}

function PasswordForm({ profile, onBack }: { profile: Profile | null; onBack?: () => void }) {
  const { setUser, brandName, plainConnection } = useApp()
  const tv = useTvMode()
  const [name, setName] = useState(profile?.name || '')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  // The password was right and this account also wants a code.
  const [needCode, setNeedCode] = useState(false)
  const [code, setCode] = useState('')
  const pwRef = useRef<HTMLInputElement | null>(null)
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setErr('')
    setBusy(true)
    try {
      const r = await api.post<{ token: string; user: User }>('/login', { name, password, code, device: navigator.userAgent.slice(0, 80) })
      setToken(r.token)
      setUser(r.user)
    } catch (e: any) {
      if (e instanceof ApiError && e.data?.twoFactor) {
        if (needCode) setErr(e.message)
        setNeedCode(true)
        setCode('')
        return
      }
      setErr(e.message || 'Sign-in failed')
      setNeedCode(false)
      setCode('')
      setPassword('')
      pwRef.current?.focus()
    } finally { setBusy(false) }
  }
  if (needCode) {
    return (
      <form className="auth-card" onSubmit={submit}>
        <button type="button" className="ibtn auth-back" onClick={() => { setNeedCode(false); setErr(''); setPassword('') }} aria-label="Back to the password"><IconBack /></button>
        <div className="brand"><BrandMark /><Brand name={brandName} /></div>
        <p>Enter the 6-digit code from your authenticator app.</p>
        <div className="form">
          <label><span className="sr-only">Code</span>
            <input className="input" autoFocus inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} />
          </label>
          <small className="muted">Lost your phone? Enter one of your recovery codes instead.</small>
          {err && <div className="error" role="alert">{err}</div>}
          <button className="btn primary" disabled={busy || !code.trim()} style={{ justifyContent: 'center' }}>{busy ? 'Checking…' : 'Sign in'}</button>
        </div>
      </form>
    )
  }
  return (
    <form className="auth-card" onSubmit={submit}>
      {onBack && <button type="button" className="ibtn auth-back" onClick={onBack} aria-label="Back to profiles"><IconBack /></button>}
      {profile
        ? <div className="auth-profile"><Avatar name={profileName(profile)} big /><h2>{profileName(profile)}</h2>{tv && profileName(profile) !== profile.name && <span className="who-user">@{profile.name}</span>}</div>
        : <div className="brand"><BrandMark /><Brand name={brandName} /></div>}
      <p>{profile ? 'Enter your password or PIN.' : 'Sign in to your media server.'}</p>
      {plainConnection && <p className="auth-plain" role="alert"><b>This connection isn’t encrypted.</b> Your password can be read or changed by anyone between this device and the server. The owner allowed this. Sign in only on a network you trust.</p>}
      <div className="form">
        {!profile && <label>Username<input className="input" autoFocus value={name} onChange={(e) => setName(e.target.value)} autoComplete="username" /></label>}
        <label>{profile ? <span className="sr-only">Password</span> : 'Password'}
          <input ref={pwRef} className="input" type="password" autoFocus={!!profile} value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
        </label>
        {err && <div className="error" role="alert">{err}</div>}
        <button className="btn primary" disabled={busy || !name || !password}>{busy ? 'Signing in…' : 'Sign in'}</button>
        <small className="muted"><Link to="/help/accounts#if-you-forget-your-password">Forgot your password?</Link></small>
      </div>
    </form>
  )
}

// Avatar: the profile's initial on a colour picked from its name, so each
// person keeps the same colour on every device (see ui/avatarColor).
export function Avatar({ name, big }: { name: string; big?: boolean }) {
  return <span className={'avatar-tile' + (big ? ' big' : '')} style={{ background: avatarColor(name) }} aria-hidden="true">{name.trim()[0]?.toUpperCase() || '?'}</span>
}
