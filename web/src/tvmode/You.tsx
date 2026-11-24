// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState, type CSSProperties, type ReactNode } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, profileName } from '../api'
import {
  IconActivity, IconBook, IconCopy, IconDownload, IconEdit, IconFile, IconGamepad, IconGlobe, IconHelp, IconImage, IconInfo, IconLibrary,
  IconLock, IconLogout, IconPalette, IconPlay, IconPlug, IconRefresh, IconRight, IconShield, IconStar, IconTag, IconTune, IconTv, IconUsers, IconWrench,
} from '../icons'
import { Avatar } from '../pages/AuthPage'
import { t } from '../i18n'
import { useApp } from '../state'
import { featureOff, useMyLimits } from '../myLimits'
import { settingsPageHidden } from '../profileWords'
import { AUTO_THEME, deviceTheme, type ThemeMeta } from '../theme'
import { setTvMode } from './mode'
import { useSwitchProfile } from '../shell/SwitchProfile'
import { withParentPin } from '../shell/ParentPin'
import { tvScreenFor } from './levelTable'
import { tvElsewhere } from './OnAComputer'

// You, as on Apple TV: the person on the left third, and on the right two-thirds the groups of the apps'
// You page (WittyMediaClient docs/APP-DESIGN.md, You and settings), each row opening the website's own page
// for it. Rows the server would refuse are left out, not greyed.

type Row = { key: string; label: string; icon: ReactNode; tint: string; to: string; value?: string; danger?: boolean }
type Group = { key: string; label: string; rows: Row[] }

// The quiet colour of the owner's tools.
const QUIET = '#5d6474'

export default function You() {
  const { user, setUser, libraries, version, serverTheme } = useApp()
  const nav = useNavigate()
  const [twoStep, setTwoStep] = useState<boolean | null>(null)
  const [devices, setDevices] = useState<number | null>(null)
  const [themeName, setThemeName] = useState('')
  const grownUp = user?.isAdmin || user?.kind === 'adult'
  useEffect(() => {
    if (grownUp) api.get<{ on: boolean }>('/me/2fa').then((r) => setTwoStep(!!r.on)).catch(() => setTwoStep(null))
    api.get<unknown[]>('/sessions').then((r) => setDevices(r.length)).catch(() => {})
    const id = deviceTheme() || serverTheme
    if (id === AUTO_THEME) setThemeName(t('Automatic'))
    else api.get<{ themes: ThemeMeta[] }>('/themes').then((r) => setThemeName(r.themes.find((theme) => theme.id === id)?.name || '')).catch(() => {})
  }, [grownUp, serverTheme])
  const gamesOff = featureOff(useMyLimits(), 'games')
  if (!user) return null

  const admin = user.isAdmin
  const games = libraries.some((l) => l.type === 'games')
  const shown = profileName(user)
  const role = admin ? t('Administrator') : user.kind === 'kids' ? t('Kids') : user.kind === 'guest' ? t('Guest') : t('Adult')
  const signOut = async () => {
    try { if ((await withParentPin((pin) => api.post('/logout', pin ? { pin } : {}))).cancelled) return } catch { /* signed out here all the same */ }
    nav('/', { replace: true }); setUser(null)
  }
  const leave = () => { setTvMode(false); nav('/', { replace: true }) }
  const sw = useSwitchProfile()

  const only = <T,>(on: boolean, rows: T[]) => (on ? rows : [])
  const groups: Group[] = [
    { key: 'account', label: t('Account'), rows: [
      { key: 'names', label: t('Name and username'), icon: <IconEdit />, tint: '#0a6fb8', to: '/settings/users', value: shown },
      ...only(grownUp, [{ key: 'password', label: t('Password'), icon: <IconLock />, tint: '#3e5bd0', to: '/settings/users' }]),
      ...only(grownUp && twoStep !== null, [{ key: 'twostep', label: t('Two-step sign-in'), icon: <IconShield />, tint: '#2e7d3c', to: '/settings/security', value: twoStep ? t('On') : t('Off') }]),
      { key: 'devices', label: t('Your devices'), icon: <IconTv />, tint: '#7f46b8', to: '/settings/security', value: devices === null ? undefined : String(devices) },
    ] },
    { key: 'household', label: t('Household'), rows: only(grownUp, [
      { key: 'profiles', label: t('Profiles and parental limits'), icon: <IconUsers />, tint: '#b8500d', to: '/settings/users' },
    ]) },
    { key: 'yours', label: t('Yours'), rows: [
      { key: 'year', label: t('Your year'), icon: <IconStar />, tint: '#c9302f', to: '/review' },
      { key: 'collections', label: t('Collections and playlists'), icon: <IconLibrary />, tint: '#0e7c6f', to: '/collections' },
      // A Kids or Guest profile makes no share links and has no activity button to set: no rows that lead nowhere.
      ...only(!settingsPageHidden('sharing', user), [{ key: 'share', label: t('Share links'), icon: <IconCopy />, tint: '#0a6fb8', to: '/settings/sharing' }]),
      ...only(!settingsPageHidden('activity', user), [{ key: 'activity', label: t('Activity'), icon: <IconActivity />, tint: '#99690a', to: '/settings/activity' }]),
      ...only(games && !gamesOff, [
        { key: 'gamesettings', label: t('Game settings'), icon: <IconGamepad />, tint: '#7f46b8', to: '/settings/games' },
        { key: 'controllers', label: t('Controllers'), icon: <IconGamepad />, tint: '#3e5bd0', to: '/controllers' },
      ]),
    ] },
    { key: 'device', label: t('On this device'), rows: [
      { key: 'theme', label: t('Theme'), icon: <IconPalette />, tint: '#0e7c6f', to: '/settings/appearance', value: themeName || undefined },
    ] },
    { key: 'server', label: t('Server'), rows: only(admin, [
      { key: 'libraries', label: t('Libraries'), icon: <IconLibrary />, tint: QUIET, to: '/manage/libraries' },
      { key: 'metadata', label: t('Metadata and artwork'), icon: <IconImage />, tint: QUIET, to: '/settings/metadata' },
      { key: 'fix', label: t('Fix matches'), icon: <IconTag />, tint: QUIET, to: '/fix-matches' },
      { key: 'playback', label: t('Playback'), icon: <IconPlay />, tint: QUIET, to: '/settings/playback' },
      { key: 'livetv', label: t('Live TV and tuners'), icon: <IconTv />, tint: QUIET, to: '/settings/channels' },
      { key: 'gaming', label: t('Consoles and cores'), icon: <IconGamepad />, tint: QUIET, to: '/settings/gaming' },
      { key: 'transcoding', label: t('Live transcoding'), icon: <IconTune />, tint: QUIET, to: '/settings/transcoding' },
      { key: 'libtranscoding', label: t('Library transcoding'), icon: <IconRefresh />, tint: QUIET, to: '/settings/library-transcoding' },
      { key: 'remote', label: t('Remote access'), icon: <IconGlobe />, tint: QUIET, to: '/settings/remote' },
      { key: 'appearance', label: t('Appearance and branding'), icon: <IconPalette />, tint: QUIET, to: '/settings/appearance' },
    ]) },
    { key: 'maintenance', label: t('Maintenance'), rows: only(admin, [
      { key: 'logs', label: 'WittyLog', icon: <IconFile />, tint: QUIET, to: '/settings/logs' },
      { key: 'diagnoser', label: 'WittyDiagnoser', icon: <IconActivity />, tint: QUIET, to: '/settings/diagnoser' },
      { key: 'modules', label: t('Modules'), icon: <IconPlug />, tint: QUIET, to: '/settings/modules' },
      { key: 'backup', label: t('Backup'), icon: <IconDownload />, tint: QUIET, to: '/settings/backup' },
      { key: 'repair', label: t('Repair'), icon: <IconWrench />, tint: QUIET, to: '/settings/repair' },
      { key: 'reset', label: t('Reset'), icon: <IconRefresh />, tint: QUIET, to: '/settings/reset', danger: true },
    ]) },
    { key: 'help', label: t('Help and about'), rows: [
      { key: 'help', label: t('Help'), icon: <IconHelp />, tint: '#99690a', to: '/help' },
      { key: 'about', label: t('About this server'), icon: <IconInfo />, tint: QUIET, to: '/settings/about', value: version || undefined },
      { key: 'legal', label: t('Legal'), icon: <IconBook />, tint: QUIET, to: '/settings/legal' },
      { key: 'licences', label: t('Licences'), icon: <IconBook />, tint: QUIET, to: '/settings/about#licenses' },
    ] },
  ]

  return (
    <div className="tvm-you-page">
      <h1 className="sr-only">{t('You')}</h1>
      <section className="tvm-you-head" aria-label={shown}>
        <Avatar name={shown || '?'} avatar={user.avatar} big />
        <h2>{shown}</h2>
        <p>@{user.name} · {role}</p>
        <div className="tvm-you-actions" data-nav-group>
          <button type="button" className="btn" onClick={sw.start}><IconUsers /> {t('Switch profile')}</button>
          {sw.dialog}
          <button type="button" className="btn" onClick={leave}>{t('Leave TV mode')}</button>
          <button type="button" className="btn danger" onClick={signOut}><IconLogout /> {t('Sign out')}</button>
        </div>
      </section>
      <div className="tvm-you-groups">
        {groups.filter((g) => g.rows.length).map((g, gi) => (
          <section key={g.key} className="tvm-group" aria-label={g.label}>
            <h3>{g.label}</h3>
            <div className="tvm-rows" data-nav-group>
              {g.rows.map((r, ri) => (
                <Link key={r.key} to={r.to} className={'tvm-row' + (r.danger ? ' danger' : '')} data-tv-first={(gi === 0 && ri === 0) || undefined}>
                  <span className="tvm-row-ic" style={{ '--tint': r.danger ? 'var(--danger)' : r.tint } as CSSProperties} aria-hidden="true">{r.icon}</span>
                  <span className="tvm-row-name">{r.label}</span>
                  {(r.value || tvScreenFor(r.to)?.level === 'desktopOnly') && <span className="tvm-row-value">{r.value || tvElsewhere()}</span>}
                  <IconRight className="tvm-row-go" />
                </Link>
              ))}
            </div>
          </section>
        ))}
      </div>
    </div>
  )
}
