// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, ApiError, profileName, setToken, type HouseholdProfile, type User } from '../api'
import { IconLock } from '../icons'
import { Avatar } from '../pages/AuthPage'
import { useApp } from '../state'
import { Button } from '../ui/Button'
import SpecPage from '../spec/SpecPage'
import { switchSpec } from '../spec/specs/auth'
import { isSubmitKey } from '../spec/specs/submit'
import { Dialog } from '../ui/Overlay'
import './switchprofile.css'

// Switching profile on a device that is already signed in: "Who's watching?" without signing out
// first. The server signs this device in as the profile chosen, asking for what signing in asks
// for (its password or PIN, its code), and ends the sign-in it came from. Where the server doesn't
// show its profiles to this device, switching is signing out, as it always was.

export function useSwitchProfile() {
  const { setUser } = useApp()
  const nav = useNavigate()
  const [open, setOpen] = useState(false)
  const signOut = async () => { try { await api.post('/logout') } catch { /* signed out here all the same */ } nav('/', { replace: true }); setUser(null) }
  const start = async () => {
    try {
      const r = await api.get<{ profiles: HouseholdProfile[] }>('/profiles/household')
      if (r.profiles.length > 1) { setOpen(true); return }
    } catch { /* an older server: sign out instead */ }
    signOut()
  }
  const dialog = open ? <SwitchProfile onClose={() => setOpen(false)} onSignOut={signOut} /> : null
  return { start, dialog }
}

function SwitchProfile({ onClose, onSignOut }: { onClose: () => void; onSignOut: () => void }) {
  const nav = useNavigate()
  const [profiles, setProfiles] = useState<HouseholdProfile[] | null>(null)
  const [chosen, setChosen] = useState<HouseholdProfile | null>(null)
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [needCode, setNeedCode] = useState(false)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    api.get<{ profiles: HouseholdProfile[] }>('/profiles/household').then((r) => setProfiles(r.profiles)).catch((e) => { setProfiles([]); setErr((e as Error).message) })
  }, [])
  const go = async (p: HouseholdProfile, pw: string) => {
    setErr(''); setBusy(true)
    try {
      const r = await api.post<{ token: string; user: User }>('/profiles/switch', { name: p.name, password: pw, code, device: navigator.userAgent.slice(0, 80) })
      setToken(r.token)
      onClose()
      nav('/', { replace: true })
      // Start the page afresh as the new profile, rather than handing it the page as it is.
      // What the profile before had on screen, had fetched, or was still playing in the
      // corner is not the new one's to see, and a request still on its way would be answered
      // (and written in a parent's report) as the new profile's.
      window.location.reload()
    } catch (e) {
      if (e instanceof ApiError && e.data?.twoFactor) {
        if (needCode) setErr(e.message)
        setNeedCode(true); setCode('')
      } else {
        setErr((e as Error).message || 'That didn’t work.')
        setPassword(''); setNeedCode(false); setCode('')
        setTimeout(() => document.querySelector<HTMLElement>('[data-field="password"]')?.focus())
      }
    } finally { setBusy(false) }
  }
  const pick = (p: HouseholdProfile) => {
    if (p.current) { onClose(); return }
    setErr(''); setPassword(''); setCode(''); setNeedCode(false)
    if (p.hasPassword) setChosen(p)
    else go(p, '')
  }
  if (chosen) {
    const name = profileName(chosen)
    return (
      <Dialog title={`Switch to ${name}`} size="sm" onClose={onClose}
        actions={<><Button variant="ghost" onClick={() => { setChosen(null); setErr('') }}>Back</Button><Button variant="primary" busy={busy} disabled={needCode ? !code.trim() : !password} onClick={() => go(chosen, password)}>Switch</Button></>}>
        <div onKeyDown={(e) => { if (isSubmitKey(e)) { e.preventDefault(); if (!busy && (needCode ? code.trim() : password)) go(chosen, password) } }}>
          <div className="sw-chosen"><Avatar name={name} avatar={chosen.avatar} /> <b>{name}</b></div>
          <SpecPage page={switchSpec(chosen.kind, needCode, password, code, err)} embedded onField={(id, v) => { if (id === 'code') setCode(String(v)); else setPassword(String(v)) }} />
        </div>
      </Dialog>
    )
  }
  return (
    <Dialog title="Who’s watching?" onClose={onClose}
      actions={<><Button variant="ghost" onClick={onSignOut}>Sign in with another account</Button><Button onClick={onClose}>Cancel</Button></>}>
      {!profiles ? <div className="loading-box" role="status" aria-label="Loading"><div className="spinner" /></div> : (
        <ul className="sw-grid" role="list">
          {profiles.map((p, i) => (
            <li key={p.id}>
              <button type="button" className={'sw-tile' + (p.current ? ' current' : '')} autoFocus={i === 0} disabled={busy} aria-current={p.current || undefined} onClick={() => pick(p)}>
                <Avatar name={profileName(p)} avatar={p.avatar} />
                <span className="sw-name">{profileName(p)}</span>
                <span className="sw-sub">
                  {p.current ? 'Watching now' : p.kind === 'kids' ? 'Kids' : p.kind === 'guest' ? 'Guest' : ''}
                  {!p.current && p.hasPassword && <IconLock size={14} />}
                  {!p.current && p.hasPassword && <span className="sr-only">, asks for a password or PIN</span>}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {err && <div className="error" role="alert">{err}</div>}
    </Dialog>
  )
}
