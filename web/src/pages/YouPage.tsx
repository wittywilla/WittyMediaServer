// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Navigate } from 'react-router-dom'
import { api, isNativeShell, profileName, serverBase, setServerBase } from '../api'
import { Avatar } from './AuthPage'
import { Brand, BrandMark } from '../components/common'
import { IconUsers } from '../icons'
import { useMedia } from '../shell/useNav'
import { useSignOut } from '../shell/Account'
import { useSwitchProfile } from '../shell/SwitchProfile'
import { useApp } from '../state'
import { featureOff, useMyLimits } from '../myLimits'
import { settingsPageHidden } from '../profileWords'
import { AUTO_THEME, CUSTOM_THEME, deviceTheme } from '../theme'
import { ask } from '../ui'
import SpecPage from '../spec/SpecPage'
import { handledIds } from '../spec/model'
import { youSpec } from '../spec/specs/you'
import './you.css'

// You, on a phone: the person, their account, this device's choices and, for an
// administrator, the server, as the apps' You page is (WittyMediaClient's
// docs/APP-DESIGN.md, You and settings). Each row opens the website's own page for it.
// A wider screen has the account menu and Settings instead, so it goes to Profiles.

export default function YouPage() {
  const phone = useMedia('(max-width: 760px)')
  const { user, libraries, version, brandName, serverTheme, update } = useApp()
  const signOut = useSignOut()
  const sw = useSwitchProfile()
  const [twoStep, setTwoStep] = useState<boolean | null | undefined>(undefined) // null: the server doesn't offer it
  const [devices, setDevices] = useState<number | null>(null)
  const [print, setPrint] = useState('')
  const [themes, setThemes] = useState<{ id: string; name: string }[]>([])
  const adult = !!user && (user.isAdmin || !user.kind || user.kind === 'adult')
  useEffect(() => {
    if (!phone || !user) return
    let gone = false
    if (adult) api.get<{ on: boolean }>('/me/2fa').then((r) => { if (!gone) setTwoStep(r.on) }).catch(() => { if (!gone) setTwoStep(null) })
    api.get<unknown[]>('/sessions').then((l) => { if (!gone) setDevices(l.length) }).catch(() => {})
    api.get<{ fingerprint?: string }>('/capabilities').then((c) => { if (!gone) setPrint(c.fingerprint || '') }).catch(() => {})
    api.get<{ themes: { id: string; name: string }[] }>('/themes').then((r) => { if (!gone) setThemes(r.themes) }).catch(() => {})
    return () => { gone = true }
  }, [phone, user?.id, adult]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!phone) return <Navigate to="/settings/users" replace />
  const gamesOff = featureOff(useMyLimits(), 'games')
  if (!user) return null

  const shown = profileName(user)
  const role = user.isAdmin ? 'Administrator' : user.kind === 'kids' ? 'Kids' : user.kind === 'guest' ? 'Guest' : 'Adult'
  const games = libraries.some((l) => l.type === 'games' && l.console !== 'rig')
  // An administrator with a newer version waiting is taken to it, as the account menu does.
  const newer = !!update && user.isAdmin
  const mine = deviceTheme()
  const themeName = mine === AUTO_THEME ? 'Automatic' : mine === CUSTOM_THEME ? 'Your own' : themes.find((t) => t.id === (mine || serverTheme))?.name || ''
  const address = (serverBase() || window.location.origin).replace(/\/+$/, '')
  const out = async () => { if (await ask({ title: 'Sign out?', confirm: 'Sign out', danger: true })) signOut() }
  const changeServer = () => { setServerBase(''); window.location.reload() }

  const page = youSpec({
    shown, adult, isAdmin: !!user.isAdmin, twoStep, devices, games, gamesOff,
    sharingHidden: settingsPageHidden('sharing', user), activityHidden: settingsPageHidden('activity', user),
    themeName, newer, version, address, print, native: isNativeShell(),
  })
  const act = (id: string) => { if (id === 'sign-out') void out(); else if (id === 'change-server') changeServer() }

  return (
    <div className="you">
      <h1 className="sr-only">You</h1>
      <header className="you-head">
        <Avatar name={shown || '?'} avatar={user.avatar} big />
        <div className="you-name">{shown}</div>
        <div className="you-sub">@{user.name} · {role}</div>
        <button type="button" className="btn you-switch" onClick={sw.start}><IconUsers /> Switch profile</button>
        {sw.dialog}
      </header>
      <SpecPage embedded page={page} handled={handledIds(page)} onAction={act}
        after={{ [page.sections.length - 2]: (
          <div className="you-server">
            <BrandMark />
            <div className="you-server-text"><b className="you-server-name"><Brand name={brandName} /></b></div>
          </div>
        ) }} />
    </div>
  )
}
