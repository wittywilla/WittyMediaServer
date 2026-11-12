// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, ApiError, isNativeShell, profileName, setToken, type User } from '../api'
import ConnectServer from '../components/ConnectServer'
import { PairDevice } from '../components/PairDevice'
import { Brand, BrandMark, VersionTag } from '../components/common'
import SpecPage from '../spec/SpecPage'
import { handledIds } from '../spec/model'
import { codeSpec, signInSpec } from '../spec/specs/auth'
import { isSubmitKey } from '../spec/specs/submit'
import { useApp } from '../state'
import { avatarColor } from '../ui/avatarColor'
import { AvatarShape, parseAvatar } from '../ui/avatars'
import { useTvMode } from '../tvmode/mode'

type Profile = { id: number; name: string; displayName?: string; kind: 'adult' | 'kids' | 'guest'; hasPassword: boolean; avatar?: string }

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
  // Signing in by being approved from a phone: a QR code and a PIN, no password typed here.
  const [pairing, setPairing] = useState(false)
  useEffect(() => {
    api.get<{ style: 'profiles' | 'classic'; profiles?: Profile[] }>('/profiles')
      .then((r) => { setStyle(r.profiles?.length ? r.style : 'classic'); setProfiles(r.profiles || []) })
      .catch(() => setStyle('classic'))
  }, [])
  if (!style) return <div className="center"><div className="spinner" /></div>
  if (changing) return <ConnectServer onCancel={() => setChanging(false)} />
  if (pairing) return <div className="center"><PairDevice onBack={() => setPairing(false)} /></div>
  const withPhone = <button type="button" className="btn ghost" onClick={() => setPairing(true)}>Sign in with your phone</button>
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
                <Avatar name={profileName(p)} avatar={p.avatar} big />
                <span className="who-name">{profileName(p)}</span>
                {tv && profileName(p) !== p.name && <span className="who-user">@{p.name}</span>}
                {p.kind !== 'adult' && <span className={'who-kind kind-' + p.kind}>{p.kind === 'kids' ? 'Kids' : 'Guest'}</span>}
              </button>
            </li>
          ))}
        </ul>
        {tapErr && <div className="error" role="alert">{tapErr}</div>}
        <button type="button" className="btn ghost" onClick={() => setOther(true)}>Sign in with another account</button>
        {withPhone}
        {changeServer}
        <div className="auth-version"><VersionTag version={version} /> · <Link to="/help">Help</Link> · <Link to="/legal">Legal</Link></div>
      </div>
    )
  }
  return (
    <div className="center">
      <PasswordForm profile={chosen} onBack={style === 'profiles' ? () => { setChosen(null); setOther(false) } : undefined} />
      {!chosen && withPhone}
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
  const submit = async () => {
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
      setTimeout(() => document.querySelector<HTMLElement>('[data-field="password"]')?.focus())
    } finally { setBusy(false) }
  }
  const logo = <div className="brand"><BrandMark /><Brand name={brandName} /></div>
  const head = profile
    ? <div className="auth-profile"><Avatar name={profileName(profile)} avatar={profile.avatar} big />{tv && profileName(profile) !== profile.name && <span className="who-user">@{profile.name}</span>}</div>
    : logo
  const act = (id: string) => {
    if (id === 'back') { if (needCode) { setNeedCode(false); setErr(''); setPassword('') } else onBack?.() }
    else if (!busy) submit()
  }
  const field = (id: string, v: string | number | boolean | File) => { const t = String(v); if (id === 'name') setName(t); else if (id === 'password') setPassword(t); else if (id === 'code') setCode(t) }
  const page = needCode
    ? codeSpec(brandName, code, busy, err)
    : signInSpec({ brand: brandName, profile: profile ? profileName(profile) : null, kind: profile?.kind, name, password, busy, error: err, plain: plainConnection, hasBack: !!onBack })
  return (
    <div className="auth-spec" onKeyDown={(e) => { if (isSubmitKey(e)) { e.preventDefault(); if (!busy) submit() } }}>
      {head}
      <SpecPage page={page} handled={handledIds(page)} onAction={act} onField={field} />
    </div>
  )
}

// Avatar: the profile's initial on a colour picked from its name, so each
// person keeps the same colour on every device (see ui/avatarColor).
// A profile that chose a picture shows that instead (ui/avatars).
export function Avatar({ name, big, avatar }: { name: string; big?: boolean; avatar?: string }) {
  const chosen = parseAvatar(avatar)
  if (chosen) return <span className={'avatar-tile' + (big ? ' big' : '')} style={{ background: chosen.background }} aria-hidden="true"><AvatarShape shape={chosen.shape} /></span>
  return <span className={'avatar-tile' + (big ? ' big' : '')} style={{ background: avatarColor(name) }} aria-hidden="true">{name.trim()[0]?.toUpperCase() || '?'}</span>
}
