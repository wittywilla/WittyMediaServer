import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, cornerName, profileName } from '../api'
import { IconGamepad, IconHelp, IconLogout, IconPalette, IconSettings, IconStar, IconTv, IconUsers } from '../icons'
import { Avatar } from '../pages/AuthPage'
import { UpdateDot } from '../components/common'
import { useApp } from '../state'
import { Menu, type MenuEntry } from '../ui/Menu'
import { RowSheet, Rows, SheetRow } from '../ui/RowSheet'

// What the account menu holds, in the top bar's corner and behind the phone's You tab.
function useAccount() {
  const { user, setUser, libraries } = useApp()
  const nav = useNavigate()
  const signOut = async () => { try { await api.post('/logout') } catch { /* ignore */ } nav('/', { replace: true }); setUser(null) }
  const items: MenuEntry[] = [
    { key: 'year', label: 'Your year', icon: <IconStar />, to: '/review' },
    { key: 'look', label: 'Appearance', icon: <IconPalette />, to: '/settings/appearance' },
    ...(libraries.some((l) => l.type === 'games') ? [{ key: 'pads', label: 'Controllers', icon: <IconGamepad />, to: '/controllers' }] : []),
    { key: 'settings', label: <>Settings<UpdateDot /></>, text: 'Settings', icon: <IconSettings />, to: '/settings' },
    { key: 'help', label: 'Help', icon: <IconHelp />, to: '/help' },
    ...(user?.kind === 'adult' || user?.isAdmin ? [{ key: 'pair', label: 'Sign in a TV', icon: <IconTv />, to: '/pair' }] : []),
    { kind: 'separator', key: 'sep' },
    { key: 'switch', label: 'Switch profile', icon: <IconUsers />, onSelect: signOut },
    { key: 'out', label: 'Sign out', icon: <IconLogout />, onSelect: signOut },
  ]
  const role = user?.isAdmin ? 'Administrator' : user?.kind === 'kids' ? 'Kids profile' : user?.kind === 'guest' ? 'Guest' : 'Profile'
  const shown = user ? profileName(user) : ''
  const head = <><b>{shown}</b><span>{user && user.name !== shown ? `${user.name} · ${role}` : role}</span></>
  // The apps' words for the same thing (You's header).
  const roleName = user?.isAdmin ? 'Administrator' : user?.kind === 'kids' ? 'Kids' : user?.kind === 'guest' ? 'Guest' : 'Adult'
  return { user, items, head, shown, roleName, signOut }
}

// ProfileMenu: the avatar in the corner, with settings, switching profiles
// (back to "Who's watching?") and signing out.
export function ProfileMenu() {
  const { user, items, head, shown } = useAccount()
  const corner = user ? cornerName(user) : ''
  return (
    <div className="profile-menu">
      <Menu label="Account" items={items} head={head} trigger={(props) => (
        <button type="button" {...props} className="avatar-btn" aria-label={`${corner}: account menu`}>
          <span className="avatar-name">{corner}</span>
          <Avatar name={shown || '?'} />
        </button>
      )} />
    </div>
  )
}

// The phone's You tab: a sheet from the bottom, as the apps' You page begins: the person,
// with Switch profile, then the same places as the account menu.
export function YouTab() {
  const { user, items, shown, roleName, signOut } = useAccount()
  const [open, setOpen] = useState(false)
  const places = items.filter((it): it is Exclude<MenuEntry, { kind: 'separator' | 'heading' }> => it.kind !== 'separator' && it.kind !== 'heading' && it.key !== 'switch' && it.key !== 'out')
  return (
    <>
      <button type="button" className="tab-you" aria-label={`You, ${shown}`} aria-haspopup="dialog" aria-expanded={open} onClick={() => setOpen(true)}>
        <Avatar name={shown || '?'} /><UpdateDot />
      </button>
      {open && user && (
        <RowSheet title="You" className="you-sheet" onClose={() => setOpen(false)} head={
          <div className="you-head">
            <Avatar name={shown || '?'} />
            <b className="you-name">{shown}</b>
            <span className="you-sub">@{user.name} · {roleName}</span>
            <button type="button" className="btn" onClick={signOut}><IconUsers /> Switch profile</button>
          </div>
        }>
          <Rows label="Places">
            {places.map((it) => <SheetRow key={it.key} icon={it.icon} label={it.label} to={it.to} onSelect={it.onSelect} />)}
          </Rows>
          <Rows><SheetRow icon={<IconLogout />} label="Sign out" danger onSelect={signOut} /></Rows>
        </RowSheet>
      )}
    </>
  )
}
