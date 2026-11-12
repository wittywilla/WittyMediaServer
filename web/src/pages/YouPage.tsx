// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState, type CSSProperties, type ReactNode } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { api, isNativeShell, profileName, serverBase, setServerBase } from '../api'
import { Avatar } from './AuthPage'
import { Brand, BrandMark, UpdateDot } from '../components/common'
import {
  IconActivity, IconBolt, IconBook, IconBrush, IconChannels, IconContrast, IconDevices, IconDownload, IconDrive, IconFile, IconGamepad, IconGlobe, IconHelp,
  IconImage, IconInfo, IconKey, IconLibrary, IconLink, IconLock, IconLogout, IconPalette, IconPerson, IconPlay, IconPlug, IconRefresh, IconRight, IconServer,
  IconShield, IconSparkles, IconStack, IconTune, IconTv, IconUsers, IconWand, IconWrench,
} from '../icons'
import { useMedia } from '../shell/useNav'
import { useSignOut } from '../shell/Account'
import { useSwitchProfile } from '../shell/SwitchProfile'
import { useApp } from '../state'
import { featureOff, useMyLimits } from '../myLimits'
import { settingsPageHidden } from '../profileWords'
import { AUTO_THEME, CUSTOM_THEME, deviceTheme } from '../theme'
import { ask } from '../ui'
import './you.css'

// You, on a phone: the person, their account, this device's choices and, for an
// administrator, the server, as the apps' You page is (WittyMediaClient's
// docs/APP-DESIGN.md, You and settings). Each row opens the website's own page for it.
// A wider screen has the account menu and Settings instead, so it goes to Profiles.

// Each row's colour, as the apps have them. The owner's tools are all the quiet one.
const TINT = {
  account: '#0a6fb8', password: '#3e5bd0', twoStep: '#2e7d3c', devices: '#7f46b8', tv: '#bd2f85', family: '#b8500d',
  year: '#c9302f', teal: '#0e7c6f', gold: '#99690a', quiet: '#5d6474', danger: 'var(--danger)',
}

type RowProps = { to: string; icon: ReactNode; tint: string; label: ReactNode; value?: ReactNode; danger?: boolean }

function Row({ to, icon, tint, label, value, danger }: RowProps) {
  return (
    <Link to={to} className={'you-row' + (danger ? ' danger' : '')}>
      <span className="you-tile" style={{ '--tint': tint } as CSSProperties} aria-hidden="true">{icon}</span>
      <span className="you-row-text">{label}</span>
      {value != null && value !== '' && <span className="you-row-value">{value}</span>}
      <IconRight className="you-go" />
    </Link>
  )
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="you-group" aria-label={title}>
      <h2 className="you-caption">{title}</h2>
      <div className="you-card">{children}</div>
    </section>
  )
}

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

      <Group title="Account">
        <Row to="/settings/users" icon={<IconPerson />} tint={TINT.account} label="Name and username" value={shown} />
        {adult && <Row to="/settings/users" icon={<IconKey />} tint={TINT.password} label="Password" />}
        {adult && twoStep !== null && <Row to="/settings/security" icon={<IconLock />} tint={TINT.twoStep} label="Two-step sign-in" value={twoStep === undefined ? '' : twoStep ? 'On' : 'Off'} />}
        <Row to="/settings/security" icon={<IconDevices />} tint={TINT.devices} label="Your devices" value={devices ?? ''} />
      </Group>
      {adult && (
        <Group title="Household">
          <Row to="/pair" icon={<IconTv />} tint={TINT.tv} label="Sign in a TV" />
          <Row to="/away" icon={<IconTv />} tint={TINT.tv} label="From away" />
          <Row to="/settings/users" icon={<IconUsers />} tint={TINT.family} label="Profiles and parental limits" />
        </Group>
      )}
      <Group title="Yours">
        <Row to="/review" icon={<IconSparkles />} tint={TINT.year} label="Your year" />
        <Row to="/collections" icon={<IconStack />} tint={TINT.teal} label="Collections and playlists" />
        {/* A Kids or Guest profile makes no share links and has no activity button to set: no rows that lead nowhere. */}
        {!settingsPageHidden('sharing', user) && <Row to="/settings/sharing" icon={<IconLink />} tint={TINT.account} label="Share links" />}
        {!settingsPageHidden('activity', user) && <Row to="/settings/activity" icon={<IconActivity />} tint={TINT.gold} label="Activity" />}
        {games && !gamesOff && <Row to="/settings/games" icon={<IconContrast />} tint={TINT.devices} label="Game settings" />}
        {games && <Row to="/controllers" icon={<IconGamepad />} tint={TINT.password} label="Controllers" />}
      </Group>
      <Group title="On this device">
        <Row to="/settings/appearance#theme" icon={<IconPalette />} tint={TINT.teal} label="Theme" value={themeName} />
      </Group>
      {user.isAdmin && <>
        <Group title="Server">
          <Row to="/settings/libraries" icon={<IconLibrary />} tint={TINT.quiet} label="Libraries" />
          <Row to="/settings/metadata" icon={<IconImage />} tint={TINT.quiet} label="Metadata and artwork" />
          <Row to="/fix-matches" icon={<IconWand />} tint={TINT.quiet} label="Fix matches" />
          <Row to="/settings/playback" icon={<IconPlay />} tint={TINT.quiet} label="Playback and discs" />
          <Row to="/settings/channels" icon={<IconChannels />} tint={TINT.quiet} label="Live TV and tuners" />
          <Row to="/settings/gaming" icon={<IconGamepad />} tint={TINT.quiet} label="Consoles and cores" />
          <Row to="/settings/couch" icon={<IconDrive />} tint={TINT.quiet} label="Game PCs" />
          <Row to="/settings/transcoding" icon={<IconBolt />} tint={TINT.quiet} label="Live transcoding" />
          <Row to="/settings/library-transcoding" icon={<IconRefresh />} tint={TINT.quiet} label="Library transcoding" />
          <Row to="/settings/remote" icon={<IconGlobe />} tint={TINT.quiet} label="Remote access" />
          <Row to="/settings/appearance" icon={<IconBrush />} tint={TINT.quiet} label="Appearance and branding" />
          <Row to="/settings/wittyrigs" icon={<IconServer />} tint={TINT.quiet} label="WittyRigs" />
        </Group>
        <Group title="Maintenance">
          <Row to="/settings/logs" icon={<IconFile />} tint={TINT.quiet} label="WittyLog" />
          <Row to="/settings/diagnoser" icon={<IconTune />} tint={TINT.quiet} label="WittyDiagnoser" />
          <Row to="/settings/modules" icon={<IconPlug />} tint={TINT.quiet} label="Modules" />
          <Row to="/settings/backup" icon={<IconDownload />} tint={TINT.quiet} label="Backup" />
          <Row to="/settings/repair" icon={<IconWrench />} tint={TINT.quiet} label="Repair" />
          <Row to="/settings/reset" icon={<IconRefresh />} tint={TINT.danger} label="Reset" danger />
        </Group>
      </>}
      <Group title="Help and about">
        <Row to="/help" icon={<IconHelp />} tint={TINT.gold} label="Help" />
        <Row to={newer ? '/settings/about#updates' : '/settings/about'} icon={<IconInfo />} tint={TINT.quiet} label={<>About this server<UpdateDot /></>} value={newer ? 'Update available' : version} />
        <Row to="/legal" icon={<IconShield />} tint={TINT.quiet} label="Legal" />
        <Row to="/settings/about#licenses" icon={<IconBook />} tint={TINT.quiet} label="Licences" />
      </Group>

      <section className="you-group" aria-label="This server">
        <h2 className="you-caption">This server</h2>
        <div className="you-server">
          <BrandMark />
          <div className="you-server-text">
            <b className="you-server-name"><Brand name={brandName} /></b>
            <span className="you-row-value">{address}</span>
            {print && <small>Server fingerprint <span className="you-print">{print}</span></small>}
          </div>
        </div>
        {/* Only an app that keeps its own pages can go to another server. */}
        {isNativeShell() && <button type="button" className="btn you-wide" onClick={changeServer}><IconServer /> Change server</button>}
        <button type="button" className="btn you-wide you-out" onClick={out}><IconLogout /> Sign out</button>
      </section>
    </div>
  )
}
