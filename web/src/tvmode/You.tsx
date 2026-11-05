import { useEffect, useState, type CSSProperties, type ReactNode } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, profileName } from '../api'
import {
  IconActivity, IconBook, IconCopy, IconDownload, IconDrive, IconEdit, IconFile, IconGamepad, IconGlobe, IconHelp, IconImage, IconInfo, IconLibrary,
  IconLock, IconLogout, IconNetwork, IconPalette, IconPlay, IconPlug, IconRefresh, IconRight, IconShield, IconStar, IconTag, IconTune, IconTv, IconUsers, IconWrench,
} from '../icons'
import { Avatar } from '../pages/AuthPage'
import { useApp } from '../state'
import { AUTO_THEME, deviceTheme, type ThemeMeta } from '../theme'
import { setTvMode } from './mode'

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
    if (id === AUTO_THEME) setThemeName('Automatic')
    else api.get<{ themes: ThemeMeta[] }>('/themes').then((r) => setThemeName(r.themes.find((t) => t.id === id)?.name || '')).catch(() => {})
  }, [grownUp, serverTheme])
  if (!user) return null

  const admin = user.isAdmin
  const games = libraries.some((l) => l.type === 'games')
  const shown = profileName(user)
  const role = admin ? 'Administrator' : user.kind === 'kids' ? 'Kids' : user.kind === 'guest' ? 'Guest' : 'Adult'
  const signOut = async () => { try { await api.post('/logout') } catch { /* signed out here all the same */ } nav('/', { replace: true }); setUser(null) }
  const leave = () => { setTvMode(false); nav('/', { replace: true }) }

  const only = <T,>(on: boolean, rows: T[]) => (on ? rows : [])
  const groups: Group[] = [
    { key: 'account', label: 'Account', rows: [
      { key: 'names', label: 'Name and username', icon: <IconEdit />, tint: '#0a6fb8', to: '/settings/users', value: shown },
      ...only(grownUp, [{ key: 'password', label: 'Password', icon: <IconLock />, tint: '#3e5bd0', to: '/settings/users' }]),
      ...only(grownUp && twoStep !== null, [{ key: 'twostep', label: 'Two-step sign-in', icon: <IconShield />, tint: '#2e7d3c', to: '/settings/security', value: twoStep ? 'On' : 'Off' }]),
      { key: 'devices', label: 'Your devices', icon: <IconTv />, tint: '#7f46b8', to: '/settings/security', value: devices === null ? undefined : String(devices) },
    ] },
    { key: 'household', label: 'Household', rows: only(grownUp, [
      { key: 'profiles', label: 'Profiles and parental limits', icon: <IconUsers />, tint: '#b8500d', to: '/settings/users' },
    ]) },
    { key: 'yours', label: 'Yours', rows: [
      { key: 'year', label: 'Your year', icon: <IconStar />, tint: '#c9302f', to: '/review' },
      { key: 'collections', label: 'Collections and playlists', icon: <IconLibrary />, tint: '#0e7c6f', to: '/collections' },
      { key: 'share', label: 'Share links', icon: <IconCopy />, tint: '#0a6fb8', to: '/settings/sharing' },
      { key: 'activity', label: 'Activity', icon: <IconActivity />, tint: '#99690a', to: '/settings/activity' },
      ...only(games, [
        { key: 'gamesettings', label: 'Game settings', icon: <IconGamepad />, tint: '#7f46b8', to: '/settings/games' },
        { key: 'controllers', label: 'Controllers', icon: <IconGamepad />, tint: '#3e5bd0', to: '/controllers' },
      ]),
    ] },
    { key: 'device', label: 'On this device', rows: [
      { key: 'theme', label: 'Theme', icon: <IconPalette />, tint: '#0e7c6f', to: '/settings/appearance', value: themeName || undefined },
    ] },
    { key: 'server', label: 'Server', rows: only(admin, [
      { key: 'libraries', label: 'Libraries', icon: <IconLibrary />, tint: QUIET, to: '/settings/libraries' },
      { key: 'metadata', label: 'Metadata and artwork', icon: <IconImage />, tint: QUIET, to: '/settings/metadata' },
      { key: 'fix', label: 'Fix matches', icon: <IconTag />, tint: QUIET, to: '/fix-matches' },
      { key: 'playback', label: 'Playback and discs', icon: <IconPlay />, tint: QUIET, to: '/settings/playback' },
      { key: 'livetv', label: 'Live TV and tuners', icon: <IconTv />, tint: QUIET, to: '/settings/channels' },
      { key: 'gaming', label: 'Consoles and cores', icon: <IconGamepad />, tint: QUIET, to: '/settings/gaming' },
      { key: 'gamepcs', label: 'Game PCs', icon: <IconNetwork />, tint: QUIET, to: '/settings/couch' },
      { key: 'transcoding', label: 'Live transcoding', icon: <IconTune />, tint: QUIET, to: '/settings/transcoding' },
      { key: 'libtranscoding', label: 'Library transcoding', icon: <IconRefresh />, tint: QUIET, to: '/settings/library-transcoding' },
      { key: 'remote', label: 'Remote access', icon: <IconGlobe />, tint: QUIET, to: '/settings/remote' },
      { key: 'appearance', label: 'Appearance and branding', icon: <IconPalette />, tint: QUIET, to: '/settings/appearance' },
      { key: 'rigs', label: 'WittyRigs', icon: <IconDrive />, tint: QUIET, to: '/settings/wittyrigs' },
    ]) },
    { key: 'maintenance', label: 'Maintenance', rows: only(admin, [
      { key: 'logs', label: 'WittyLog', icon: <IconFile />, tint: QUIET, to: '/settings/logs' },
      { key: 'diagnoser', label: 'WittyDiagnoser', icon: <IconActivity />, tint: QUIET, to: '/settings/diagnoser' },
      { key: 'modules', label: 'Modules', icon: <IconPlug />, tint: QUIET, to: '/settings/modules' },
      { key: 'backup', label: 'Backup', icon: <IconDownload />, tint: QUIET, to: '/settings/backup' },
      { key: 'repair', label: 'Repair', icon: <IconWrench />, tint: QUIET, to: '/settings/repair' },
      { key: 'reset', label: 'Reset', icon: <IconRefresh />, tint: QUIET, to: '/settings/reset', danger: true },
    ]) },
    { key: 'help', label: 'Help and about', rows: [
      { key: 'help', label: 'Help', icon: <IconHelp />, tint: '#99690a', to: '/help' },
      { key: 'about', label: 'About this server', icon: <IconInfo />, tint: QUIET, to: '/settings/about', value: version || undefined },
      { key: 'legal', label: 'Legal', icon: <IconBook />, tint: QUIET, to: '/settings/legal' },
      { key: 'licences', label: 'Licences', icon: <IconBook />, tint: QUIET, to: '/settings/about#licenses' },
    ] },
  ]

  return (
    <div className="tvm-you-page">
      <h1 className="sr-only">You</h1>
      <section className="tvm-you-head" aria-label={shown}>
        <Avatar name={shown || '?'} big />
        <h2>{shown}</h2>
        <p>@{user.name} · {role}</p>
        <div className="tvm-you-actions" data-nav-group>
          <button type="button" className="btn" onClick={signOut}><IconUsers /> Switch profile</button>
          <button type="button" className="btn" onClick={leave}>Leave TV mode</button>
          <button type="button" className="btn danger" onClick={signOut}><IconLogout /> Sign out</button>
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
                  {r.value && <span className="tvm-row-value">{r.value}</span>}
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
