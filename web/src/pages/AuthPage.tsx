import { useEffect, useRef, useState } from 'react'
import { api, setToken, type User } from '../api'
import { Brand, BrandMark, VersionTag } from '../components/common'
import { IconBack } from '../icons'
import { useApp } from '../state'

type Profile = { id: number; name: string; kind: 'adult' | 'kids' | 'guest'; hasPassword: boolean }

// Sign-in. On the home network this is a "Who's watching?" screen with a
// tile per profile; elsewhere (or when an admin chose it) the classic name
// and password form. The server decides which, see GET /api/profiles.
export default function AuthPage() {
  const { brandName, version, setUser } = useApp()
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
  useEffect(() => {
    api.get<{ style: 'profiles' | 'classic'; profiles?: Profile[] }>('/profiles')
      .then((r) => { setStyle(r.profiles?.length ? r.style : 'classic'); setProfiles(r.profiles || []) })
      .catch(() => setStyle('classic'))
  }, [])
  if (!style) return <div className="center"><div className="spinner" /></div>

  if (style === 'profiles' && !chosen && !other) {
    return (
      <div className="who">
        <div className="who-brand"><BrandMark /><Brand name={brandName} /></div>
        <h1>Who's watching?</h1>
        <ul className="who-grid" role="list">
          {profiles!.map((p, i) => (
            <li key={p.id}>
              <button type="button" className="who-tile" autoFocus={i === 0} onClick={() => pick(p)}>
                <Avatar name={p.name} big />
                <span className="who-name">{p.name}</span>
                {p.kind !== 'adult' && <span className={'who-kind kind-' + p.kind}>{p.kind === 'kids' ? 'Kids' : 'Guest'}</span>}
              </button>
            </li>
          ))}
        </ul>
        {tapErr && <div className="error" role="alert">{tapErr}</div>}
        <button type="button" className="btn ghost" onClick={() => setOther(true)}>Sign in with another account</button>
        <div className="auth-version"><VersionTag version={version} /></div>
      </div>
    )
  }
  return (
    <div className="center">
      <PasswordForm profile={chosen} onBack={style === 'profiles' ? () => { setChosen(null); setOther(false) } : undefined} />
      <div className="auth-version"><VersionTag version={version} /></div>
    </div>
  )
}

function PasswordForm({ profile, onBack }: { profile: Profile | null; onBack?: () => void }) {
  const { setUser, brandName } = useApp()
  const [name, setName] = useState(profile?.name || '')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const pwRef = useRef<HTMLInputElement | null>(null)
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setErr('')
    setBusy(true)
    try {
      const r = await api.post<{ token: string; user: User }>('/login', { name, password, device: navigator.userAgent.slice(0, 80) })
      setToken(r.token)
      setUser(r.user)
    } catch (e: any) {
      setErr(e.message || 'Sign-in failed')
      setPassword('')
      pwRef.current?.focus()
    } finally { setBusy(false) }
  }
  return (
    <form className="auth-card" onSubmit={submit}>
      {onBack && <button type="button" className="ibtn auth-back" onClick={onBack} aria-label="Back to profiles"><IconBack /></button>}
      {profile
        ? <div className="auth-profile"><Avatar name={profile.name} big /><h2>{profile.name}</h2></div>
        : <div className="brand"><BrandMark /><Brand name={brandName} /></div>}
      <p>{profile ? 'Enter your password or PIN.' : 'Sign in to your media server.'}</p>
      <div className="form">
        {!profile && <label>Name<input className="input" autoFocus value={name} onChange={(e) => setName(e.target.value)} autoComplete="username" /></label>}
        <label>{profile ? <span className="sr-only">Password</span> : 'Password'}
          <input ref={pwRef} className="input" type="password" autoFocus={!!profile} value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
        </label>
        {err && <div className="error" role="alert">{err}</div>}
        <button className="btn primary" disabled={busy || !name || !password}>{busy ? 'Signing in…' : 'Sign in'}</button>
      </div>
    </form>
  )
}

// Avatar: the profile's initial on a colour picked from its name, so each
// person keeps the same colour on every device.
// All dark enough for the white initial (4.5:1).
const AVATAR_COLORS = ['#c9302f', '#b8500d', '#99690a', '#2e7d3c', '#0e7c6f', '#0a6fb8', '#3e5bd0', '#7f46b8', '#bd2f85', '#8d6444']
export function Avatar({ name, big }: { name: string; big?: boolean }) {
  let h = 0
  for (const c of name) h = (h * 31 + c.charCodeAt(0)) >>> 0
  return <span className={'avatar-tile' + (big ? ' big' : '')} style={{ background: AVATAR_COLORS[h % AVATAR_COLORS.length] }} aria-hidden="true">{name.trim()[0]?.toUpperCase() || '?'}</span>
}
