// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { NavLink, useNavigate } from 'react-router-dom'
import { api, cornerName, profileName } from '../api'
import { IconDevices, IconDownload, IconFull, IconGamepad, IconHelp, IconLogout, IconPalette, IconSettings, IconStar, IconTv, IconUsers } from '../icons'
import { Avatar } from '../pages/AuthPage'
import { UpdateDot } from '../components/common'
import { useApp } from '../state'
import { Menu, type MenuEntry } from '../ui/Menu'
import { setTvMode } from '../tvmode/mode'
import { openPlayOn, openWatchParty } from '../remote/RemoteHost'
import { useRemoteAllowed } from '../remote/allowed'
import { useSwitchProfile } from './SwitchProfile'

// Signing out: back to "Who's watching?". Switching profile without signing out is SwitchProfile.
export function useSignOut() {
  const { setUser } = useApp()
  const nav = useNavigate()
  return async () => { try { await api.post('/logout') } catch { /* ignore */ } nav('/', { replace: true }); setUser(null) }
}

// What the account menu holds, in the top bar's corner.
function useAccount() {
  const { user, libraries, update } = useApp()
  const nav = useNavigate()
  const signOut = useSignOut()
  const sw = useSwitchProfile()
  const may = useRemoteAllowed()
  const items: MenuEntry[] = [
    { key: 'year', label: 'Your year', icon: <IconStar />, to: '/review' },
    { key: 'look', label: 'Appearance', icon: <IconPalette />, to: '/settings/appearance' },
    // Your other devices and what they're playing; and watching with other people.
    // Either can be turned off for a profile, and then isn't offered.
    ...(may.playOn ? [{ key: 'playon', label: 'Play on another device', icon: <IconDevices />, onSelect: () => openPlayOn() }] : []),
    ...(may.party ? [{ key: 'party', label: 'Watch party', icon: <IconUsers />, onSelect: openWatchParty }] : []),
    // The Apple TV and Android TV layout, for a screen across the room.
    { key: 'tvmode', label: 'TV mode', icon: <IconFull />, onSelect: () => { setTvMode(true); nav('/') } },
    ...(libraries.some((l) => l.type === 'games') ? [{ key: 'pads', label: 'Controllers', icon: <IconGamepad />, to: '/controllers' }] : []),
    { key: 'settings', label: <>Settings<UpdateDot /></>, text: 'Settings', icon: <IconSettings />, to: '/settings' },
    // Admins only: a newer version of the server, and the way to it.
    ...(update && user?.isAdmin ? [{ key: 'update', label: `Update available: ${update}`, icon: <IconDownload />, to: '/settings/about#updates' }] : []),
    { key: 'help', label: 'Help', icon: <IconHelp />, to: '/help' },
    ...(user?.kind === 'adult' || user?.isAdmin ? [{ key: 'pair', label: 'Sign in a TV', icon: <IconTv />, to: '/pair' }] : []),
    { kind: 'separator', key: 'sep' },
    { key: 'switch', label: 'Switch profile', icon: <IconUsers />, onSelect: sw.start },
    { key: 'out', label: 'Sign out', icon: <IconLogout />, onSelect: signOut },
  ]
  const role = user?.isAdmin ? 'Administrator' : user?.kind === 'kids' ? 'Kids profile' : user?.kind === 'guest' ? 'Guest' : 'Profile'
  const shown = user ? profileName(user) : ''
  const head = <><b>{shown}</b><span>{user && user.name !== shown ? `${user.name} · ${role}` : role}</span></>
  return { user, items, head, shown, switching: sw.dialog }
}

// ProfileMenu: the avatar in the corner, with settings, switching profiles
// (back to "Who's watching?") and signing out.
export function ProfileMenu() {
  const { user, items, head, shown, switching } = useAccount()
  const corner = user ? cornerName(user) : ''
  return (
    <div className="profile-menu">
      <Menu label="Account" items={items} head={head} trigger={(props) => (
        <button type="button" {...props} className="avatar-btn" aria-label={`${corner}: account menu`}>
          <span className="avatar-name">{corner}</span>
          <Avatar name={shown || '?'} avatar={user?.avatar} />
        </button>
      )} />
      {switching}
    </div>
  )
}

// The phone's You tab: the You page (pages/YouPage.tsx), as in the apps.
export function YouTab() {
  const { user } = useApp()
  const shown = user ? profileName(user) : ''
  return (
    <NavLink to="/you" className="tab-you" aria-label={`You, ${shown}`}>
      <Avatar name={shown || '?'} avatar={user?.avatar} /><UpdateDot />
    </NavLink>
  )
}
