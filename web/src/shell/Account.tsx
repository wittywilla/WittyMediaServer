import { NavLink, useNavigate } from 'react-router-dom'
import { api, cornerName, profileName } from '../api'
import { IconGamepad, IconHelp, IconLogout, IconPalette, IconSettings, IconStar, IconTv, IconUsers } from '../icons'
import { Avatar } from '../pages/AuthPage'
import { UpdateDot } from '../components/common'
import { useApp } from '../state'
import { Menu, type MenuEntry } from '../ui/Menu'

// Signing out, which is also how a profile is switched: back to "Who's watching?".
export function useSignOut() {
  const { setUser } = useApp()
  const nav = useNavigate()
  return async () => { try { await api.post('/logout') } catch { /* ignore */ } nav('/', { replace: true }); setUser(null) }
}

// What the account menu holds, in the top bar's corner.
function useAccount() {
  const { user, libraries } = useApp()
  const signOut = useSignOut()
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
  return { user, items, head, shown }
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

// The phone's You tab: the You page (pages/YouPage.tsx), as in the apps.
export function YouTab() {
  const { user } = useApp()
  const shown = user ? profileName(user) : ''
  return (
    <NavLink to="/you" className="tab-you" aria-label={`You, ${shown}`}>
      <Avatar name={shown || '?'} /><UpdateDot />
    </NavLink>
  )
}
