// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, ApiError, isNativeShell, profileName, setToken, type User } from '../api'
import ConnectServer from '../components/ConnectServer'
import { PairDevice } from '../components/PairDevice'
import { VersionTag } from '../components/common'
import { StandaloneCard } from '../components/StandaloneCard'
import SpecPage from '../spec/SpecPage'
import { handledIds } from '../spec/model'
import { codeSpec, signInSpec } from '../spec/specs/auth'
import { isSubmitKey } from '../spec/specs/submit'
import { useApp } from '../state'
import { Avatar } from '../ui/Avatar'
import { useTvMode } from '../tvmode/mode'
import { t } from '../i18n'

type Profile = { id: number; name: string; displayName?: string; kind: 'adult' | 'kids' | 'guest'; hasPassword: boolean; avatar?: string }

// Sign-in. On the home network this is a "Who's watching?" screen with a
// tile per profile; elsewhere (or when an admin chose it) the classic name
// and password form. The server decides which, see GET /api/profiles.
export default function AuthPage() {
  const { version, setUser } = useApp()
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
  if (pairing) return <PairDevice onBack={() => setPairing(false)} />
  // The other ways in, as text buttons at the card's foot (docs/DESIGN.md, "Standalone pages").
  const withPhone = <button type="button" className="sa-text-btn" onClick={() => setPairing(true)}>{t('Sign in with your phone')}</button>
  const changeServer = isNativeShell() && <button type="button" className="sa-text-btn" onClick={() => setChanging(true)}>{t('Change server')}</button>
  const foot = <><VersionTag version={version} /> · <Link to="/away">{t('Connect a device')}</Link> · <Link to="/help">{t('Help')}</Link> · <Link to="/legal">{t('Legal')}</Link></>

  if (style === 'profiles' && !chosen && !other) {
    return (
      <StandaloneCard wide className="who-card" foot={foot}>
        <h1>{t('Who’s watching?')}</h1>
        <ul className="who-grid" role="list">
          {profiles!.map((p, i) => (
            <li key={p.id}>
              <button type="button" className="who-tile" autoFocus={i === 0} onClick={() => pick(p)}>
                <Avatar name={profileName(p)} avatar={p.avatar} big />
                <span className="who-name">{profileName(p)}</span>
                {tv && profileName(p) !== p.name && <span className="who-user">@{p.name}</span>}
                {p.kind !== 'adult' && <span className={'who-kind kind-' + p.kind}>{p.kind === 'kids' ? t('Kids') : t('Guest')}</span>}
              </button>
            </li>
          ))}
        </ul>
        {tapErr && <div className="error" role="alert">{tapErr}</div>}
        <div className="sa-links">
          <button type="button" className="sa-text-btn" onClick={() => setOther(true)}>{t('Sign in with another account')}</button>
          {withPhone}
          {changeServer}
        </div>
      </StandaloneCard>
    )
  }
  return (
    <StandaloneCard foot={foot}>
      <PasswordForm profile={chosen} onBack={style === 'profiles' ? () => { setChosen(null); setOther(false) } : undefined} />
      {(!chosen || changeServer) && <div className="sa-links">{!chosen && withPhone}{changeServer}</div>}
    </StandaloneCard>
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
      setErr(e.message || t('Sign-in failed'))
      setNeedCode(false)
      setCode('')
      setPassword('')
      setTimeout(() => document.querySelector<HTMLElement>('[data-field="password"]')?.focus())
    } finally { setBusy(false) }
  }
  // The card shows the server's logo and name; a chosen profile adds its picture.
  const head = profile
    ? <div className="auth-profile"><Avatar name={profileName(profile)} avatar={profile.avatar} big />{tv && profileName(profile) !== profile.name && <span className="who-user">@{profile.name}</span>}</div>
    : null
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
